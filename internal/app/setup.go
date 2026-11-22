package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/projectconfig"
	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/role"
	"github.com/germanmunozs/ai-dev/internal/selection"
	"github.com/germanmunozs/ai-dev/internal/system"
)

const (
	setupGroupUsage  = "Usage: ai-dev setup [show|apply] [options]"
	setupWizardUsage = "Usage: ai-dev setup"
	setupShowUsage   = "Usage: ai-dev setup show [--format text|json]"
	setupApplyUsage  = "Usage: ai-dev setup apply --agent AGENT... --role ROLE=AGENT... [--replace] [--format text|json]"
)

// loadAssignment reads the project configuration and returns the role
// assignment in effect: built-in without a configuration, the project's
// when valid, and invalid otherwise (F011-FR-015, FR-016).
func loadAssignment(root fs.FS) (selection.Assignment, projectconfig.Config, projectconfig.State, []projectconfig.Reason) {
	config, state, reasons := projectconfig.Load(root)
	switch state {
	case projectconfig.Valid:
		return selection.FromProject(config), config, state, nil
	case projectconfig.Invalid:
		return selection.InvalidProject(), config, state, reasons
	default:
		return selection.BuiltIn(), config, state, nil
	}
}

// setupAgents detects every catalog agent by executable lookup only
// (F011-FR-001). chosen marks the agents of a configuration in effect.
func setupAgents(executor system.Executor, chosen func(agent.ID) bool) []report.SetupAgent {
	var agents []report.SetupAgent
	for _, status := range agent.NewRegistry(executor).List() {
		agents = append(agents, report.SetupAgent{
			ID:           string(status.Agent.ID),
			Label:        status.Agent.DisplayLabel,
			Availability: status.Availability.String(),
			Chosen:       chosen != nil && chosen(status.Agent.ID),
		})
	}
	return agents
}

func setupRoles(assignment selection.Assignment) []report.SetupRole {
	var roles []report.SetupRole
	for _, r := range projectconfig.Roles() {
		id, _ := assignment.AgentFor(r)
		roles = append(roles, report.SetupRole{Role: string(r), Agent: string(id)})
	}
	return roles
}

func setupReasons(reasons []projectconfig.Reason) []report.SetupReason {
	var out []report.SetupReason
	for _, reason := range reasons {
		out = append(out, report.SetupReason{Code: reason.Code, Value: reason.Value})
	}
	return out
}

// configuredReport is the report of a configuration in effect, for show
// (configured) and for a successful write.
func configuredReport(command string, outcome report.SetupOutcome, executor system.Executor, config projectconfig.Config) report.SetupReport {
	return report.SetupReport{
		Command: command, Outcome: outcome, Source: string(selection.SourceProject),
		Agents: setupAgents(executor, config.Chosen),
		Roles:  setupRoles(selection.FromProject(config)),
	}
}

// setupExitCode follows F011-FR-012.
func setupExitCode(outcome report.SetupOutcome) int {
	switch outcome {
	case report.NotConfigured, report.Configured, report.ConfigurationWritten, report.ConfigurationUnchanged:
		return 0
	default:
		return 1
	}
}

func writeSetupReport(r report.SetupReport, format string, stdout, stderr io.Writer) int {
	var err error
	if format == "json" {
		err = report.WriteSetupJSON(stdout, r)
	} else {
		err = report.WriteSetupText(stdout, r)
	}
	if err != nil {
		fmt.Fprintf(stderr, "ai-dev: setup: write report: %v\n", err)
		return 2
	}
	return setupExitCode(r.Outcome)
}

// RunSetupShow implements "ai-dev setup show": detected agents and the
// assignment in effect. It writes nothing (F011-FR-009).
func RunSetupShow(args []string, executor system.Executor, root fs.FS, stdout, stderr io.Writer) int {
	flags := newFlagSet("setup show")
	format := flags.String("format", "text", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") {
		fmt.Fprintln(stderr, setupShowUsage)
		return 2
	}
	return writeSetupReport(setupShowReport(executor, root), *format, stdout, stderr)
}

// setupShowReport is the report of "setup show", also the cockpit's Agents
// view (Feature 012).
func setupShowReport(executor system.Executor, root fs.FS) report.SetupReport {
	assignment, config, state, reasons := loadAssignment(root)
	switch state {
	case projectconfig.Valid:
		return configuredReport("show", report.Configured, executor, config)
	case projectconfig.Invalid:
		return report.SetupReport{Command: "show", Outcome: report.ConfigurationInvalid, Agents: setupAgents(executor, nil), Reasons: setupReasons(reasons)}
	default:
		return report.SetupReport{Command: "show", Outcome: report.NotConfigured, Source: string(selection.SourceBuiltIn),
			Agents: setupAgents(executor, nil), Roles: setupRoles(assignment)}
	}
}

// RunSetupApply implements "ai-dev setup apply": the configuration from
// explicit options, written without prompting (F011-FR-011).
func RunSetupApply(args []string, executor system.Executor, root fs.FS, store ConfigStore, stdout, stderr io.Writer) int {
	flags := newFlagSet("setup apply")
	format := flags.String("format", "text", "")
	replace := flags.Bool("replace", false, "")
	var agents []agent.ID
	roles := map[role.Role]agent.ID{}
	var duplicates []report.SetupReason
	flags.Func("agent", "", func(value string) error {
		agents = append(agents, agent.ID(value))
		return nil
	})
	flags.Func("role", "", func(value string) error {
		name, id, ok := strings.Cut(value, "=")
		if !ok || name == "" || id == "" {
			return errors.New("want ROLE=AGENT")
		}
		r := role.Role(name)
		if _, seen := roles[r]; seen {
			duplicates = append(duplicates, report.SetupReason{Code: "duplicate_role", Value: name})
			return nil
		}
		roles[r] = agent.ID(id)
		return nil
	})
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") {
		fmt.Fprintln(stderr, setupApplyUsage)
		return 2
	}
	config, problems := projectconfig.New(agents, roles)
	if reasons := append(duplicates, setupReasons(problems)...); len(reasons) != 0 {
		return writeSetupReport(report.SetupReport{Command: "apply", Outcome: report.ConfigurationRejected, Agents: setupAgents(executor, nil), Reasons: reasons}, *format, stdout, stderr)
	}
	outcome, warnings := decideWrite(root, store, config, *replace)
	for _, warning := range warnings {
		fmt.Fprintln(stderr, warning)
	}
	switch outcome {
	case report.ConfigurationWritten, report.ConfigurationUnchanged:
		return writeSetupReport(configuredReport("apply", outcome, executor, config), *format, stdout, stderr)
	default:
		return writeSetupReport(report.SetupReport{Command: "apply", Outcome: outcome, Agents: setupAgents(executor, nil)}, *format, stdout, stderr)
	}
}
