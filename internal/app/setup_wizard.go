package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/projectconfig"
	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/role"
	"github.com/germanmunozs/ai-dev/internal/system"
)

// setupWizard is the interactive assistant of "ai-dev setup" (F011-FR-010,
// research R-05). It follows the init wizard: numbered choices, the literal
// "yes" to confirm, nothing written before it.
type setupWizard struct {
	ctx   context.Context
	lines <-chan lineRead
	out   io.Writer
}

type lineRead struct {
	line string
	err  error
}

// readLines delivers stdin line by line from one goroutine, so a prompt can
// be abandoned when the context is cancelled (Ctrl-C) instead of staying
// blocked in a read.
func readLines(r io.Reader) <-chan lineRead {
	lines := make(chan lineRead)
	go func() {
		defer close(lines)
		reader := bufio.NewReader(r)
		for {
			line, err := reader.ReadString('\n')
			if line != "" || err != nil {
				lines <- lineRead{line, err}
			}
			if err != nil {
				return
			}
		}
	}()
	return lines
}

// ask prints a prompt and returns the trimmed answer. End of input, even
// right after an answer typed without Enter, and a cancelled context cancel
// the assistant, so "yes" followed by Ctrl-D writes nothing (review L2).
func (w *setupWizard) ask(prompt string) (string, error) {
	fmt.Fprint(w.out, prompt)
	select {
	case <-w.ctx.Done():
		fmt.Fprintln(w.out)
		return "", errWizardCancelled
	case read, ok := <-w.lines:
		if !ok || read.err != nil {
			return "", errWizardCancelled
		}
		return strings.TrimSpace(read.line), nil
	}
}

// RunSetupWizard implements "ai-dev setup". Without an interactive terminal
// on stdin and stdout it refuses before detecting anything.
func RunSetupWizard(ctx context.Context, args []string, executor system.Executor, root fs.FS, store ConfigStore, stdin io.Reader, interactive bool, stdout, stderr io.Writer) int {
	flags := newFlagSet("setup")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(stderr, setupWizardUsage)
		return 2
	}
	if !interactive {
		fmt.Fprintln(stderr, "ai-dev: setup: refused: interactive_terminal_required; use ai-dev setup apply")
		return 2
	}
	w := &setupWizard{ctx: ctx, lines: readLines(stdin), out: stdout}
	config, err := w.choose(executor, root)
	switch {
	case errors.Is(err, errWizardCancelled):
		fmt.Fprintln(stdout, "Cancelled. Nothing was changed.")
		return writeSetupReport(report.SetupReport{Command: "setup", Outcome: report.SetupCancelled, Agents: setupAgents(executor, nil)}, "text", stdout, stderr)
	case err != nil:
		fmt.Fprintln(stderr, "ai-dev: setup: invalid input; nothing was changed")
		return 2
	}
	outcome, warnings := decideWrite(root, store, config, true)
	for _, warning := range warnings {
		fmt.Fprintln(stderr, warning)
	}
	if outcome == report.PersistenceFailed {
		return writeSetupReport(report.SetupReport{Command: "setup", Outcome: outcome, Agents: setupAgents(executor, nil)}, "text", stdout, stderr)
	}
	return writeSetupReport(configuredReport("setup", outcome, executor, config), "text", stdout, stderr)
}

// choose asks for the agents, then one agent per role, then confirmation.
func (w *setupWizard) choose(executor system.Executor, root fs.FS) (projectconfig.Config, error) {
	current, state, reasons := projectconfig.Load(root)
	if state == projectconfig.Invalid {
		fmt.Fprintln(w.out, "The existing configuration is invalid and will not be used as a starting point:")
		for _, reason := range reasons {
			fmt.Fprint(w.out, report.SetupReasonLine(report.SetupReason{Code: reason.Code, Value: reason.Value}))
		}
	}
	keep := state == projectconfig.Valid

	statuses := agent.NewRegistry(executor).List()
	fmt.Fprintln(w.out, "Agents detected in the catalog:")
	for i, status := range statuses {
		fmt.Fprintf(w.out, "  %d. %s (%s): %s\n", i+1, status.Agent.ID, status.Agent.DisplayLabel, status.Availability)
	}
	prompt := "Which agents does this project work with? (numbers or ids, comma-separated)"
	if keep {
		prompt += " [current: " + joinIDs(current.Agents()) + "]"
	}
	answer, err := w.ask(prompt + ": ")
	if err != nil {
		return projectconfig.Config{}, err
	}
	var chosen []agent.ID
	if answer == "" && keep {
		chosen = current.Agents()
	} else {
		catalog := make([]agent.ID, len(statuses))
		for i, status := range statuses {
			catalog[i] = status.Agent.ID
		}
		if chosen, err = pickMany(answer, catalog); err != nil {
			return projectconfig.Config{}, err
		}
	}
	chosen = inCatalogOrder(chosen, statuses)
	for _, status := range statuses {
		if slices.Contains(chosen, status.Agent.ID) && status.Availability == agent.Unavailable {
			fmt.Fprintf(w.out, "warning: %s is not available on this machine; it can still be configured\n", status.Agent.ID)
		}
	}

	options := make([]string, len(chosen))
	for i, id := range chosen {
		options[i] = fmt.Sprintf("%d %s", i+1, id)
	}
	roles := map[role.Role]agent.ID{}
	for _, r := range projectconfig.Roles() {
		prompt := fmt.Sprintf("Agent for %s (%s)", r, strings.Join(options, ", "))
		was, _ := current.AgentFor(r)
		keepRole := keep && slices.Contains(chosen, was)
		if keepRole {
			prompt += " [current: " + string(was) + "]"
		}
		answer, err := w.ask(prompt + ": ")
		if err != nil {
			return projectconfig.Config{}, err
		}
		if answer == "" && keepRole {
			roles[r] = was
			continue
		}
		picked, err := pickMany(answer, chosen)
		if err != nil || len(picked) != 1 {
			return projectconfig.Config{}, errWizardInvalid
		}
		roles[r] = picked[0]
	}
	config, problems := projectconfig.New(chosen, roles)
	if len(problems) != 0 {
		return projectconfig.Config{}, errWizardInvalid
	}

	fmt.Fprintf(w.out, "This configuration will be written to %s:\n  agents: %s\n", projectconfig.FileName, joinIDs(config.Agents()))
	for _, r := range projectconfig.Roles() {
		id, _ := config.AgentFor(r)
		fmt.Fprintf(w.out, "  %s: %s\n", r, id)
	}
	answer, err = w.ask("Write it? [yes/no] ")
	if err != nil {
		return projectconfig.Config{}, err
	}
	if answer != "yes" {
		return projectconfig.Config{}, errWizardCancelled
	}
	return config, nil
}

// pickMany resolves a comma-separated answer of 1-based numbers or exact
// identifiers against options. Empty, unknown and repeated choices are
// invalid.
func pickMany(answer string, options []agent.ID) ([]agent.ID, error) {
	if strings.TrimSpace(answer) == "" {
		return nil, errWizardInvalid
	}
	var picked []agent.ID
	for _, field := range strings.Split(answer, ",") {
		field = strings.TrimSpace(field)
		var id agent.ID
		if n, err := strconv.Atoi(field); err == nil && n >= 1 && n <= len(options) {
			id = options[n-1]
		} else if slices.Contains(options, agent.ID(field)) {
			id = agent.ID(field)
		} else {
			return nil, errWizardInvalid
		}
		if slices.Contains(picked, id) {
			return nil, errWizardInvalid
		}
		picked = append(picked, id)
	}
	return picked, nil
}

func inCatalogOrder(ids []agent.ID, statuses []agent.AgentStatus) []agent.ID {
	var ordered []agent.ID
	for _, status := range statuses {
		if slices.Contains(ids, status.Agent.ID) {
			ordered = append(ordered, status.Agent.ID)
		}
	}
	return ordered
}

func joinIDs(ids []agent.ID) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = string(id)
	}
	return strings.Join(names, ", ")
}
