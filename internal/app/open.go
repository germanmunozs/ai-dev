package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/cockpit"
	"github.com/germanmunozs/ai-dev/internal/editor"
	"github.com/germanmunozs/ai-dev/internal/system"
)

const openUsage = "Usage: ai-dev open [--format text|json] [--line N] [--column N] -- <path>"

const openSchema = "ai-dev.open/v1"

// openDependencies are what ai-dev open reads from its environment, so the
// tests can supply each one.
type openDependencies struct {
	Getenv        func(string) string
	Getwd         func() (string, error)
	LookPath      func(string) (string, error)
	CheckEndpoint func(string) error
	OpenParent    func(string, editor.Location) error
	// Terminals: standard input and output are both terminals.
	Terminals bool
	Launcher  system.Launcher
}

func defaultOpenDependencies(deps Dependencies) openDependencies {
	return openDependencies{
		Getenv:        os.Getenv,
		Getwd:         os.Getwd,
		LookPath:      deps.Executor.LookPath,
		CheckEndpoint: editor.CheckEndpoint,
		OpenParent:    editor.OpenInParent,
		Terminals:     isTerminal(deps.Stdin) && writerIsTerminal(deps.Stdout),
		Launcher:      deps.Launcher,
	}
}

// openResult is one outcome of contracts/open.md.
type openResult struct {
	Outcome  string
	Reason   string
	Target   string
	Message  string
	ExitCode *int
	loc      editor.Location
}

// RunOpen opens a project file in Neovim (F014-FR-001 to FR-005): in the
// parent instance named by $NVIM, with no fallback when it is refused, or
// else in a foreground nvim that takes over the terminal. It never searches
// for instances, never starts an editor in the background, and sends the
// editor no command built from text.
func RunOpen(ctx context.Context, args []string, deps openDependencies, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := newFlagSet("open")
	format := flags.String("format", "text", "output format: text or json")
	line := flags.Int("line", 1, "line, from 1")
	column := flags.Int("column", 1, "column, from 1")
	// "--" is required, so a path is never read as an option (owner
	// decision A1, 2026-11-24).
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 || len(args) < 2 || args[len(args)-2] != "--" ||
		(*format != "text" && *format != "json") || *line < 1 || *column < 1 {
		fmt.Fprintln(stderr, openUsage)
		return 2
	}
	path := flags.Arg(0)
	result := openFile(ctx, deps, path, *line, *column, stdin, stdout, stderr)
	// An absolute path is never echoed, not even the one typed (F014-FR-025).
	if result.Reason != editor.ReasonAbsolutePath {
		result.loc.Rel = path
	}
	if *format == "json" {
		return writeOpenJSON(result, stdout, stderr)
	}
	return writeOpenText(result, stdout, stderr)
}

func openFile(ctx context.Context, deps openDependencies, path string, line, column int, stdin io.Reader, stdout, stderr io.Writer) openResult {
	root, err := deps.Getwd()
	if err != nil {
		return openResult{Outcome: "path_refused", Reason: editor.ReasonNotFound}
	}
	loc, err := editor.Resolve(root, path, line, column)
	var pathErr *editor.PathError
	if errors.As(err, &pathErr) {
		return openResult{Outcome: "path_refused", Reason: pathErr.Reason}
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return openResult{Outcome: "editor_unavailable", Reason: editor.ReasonPlatformUnsupported, loc: loc}
	}
	if endpoint := deps.Getenv("NVIM"); endpoint != "" {
		err := deps.CheckEndpoint(endpoint)
		if err == nil {
			err = deps.OpenParent(endpoint, loc)
		}
		var endpointErr *editor.EndpointError
		switch {
		case err == nil:
			return openResult{Outcome: "opened", Target: "parent", loc: loc}
		case errors.As(err, &endpointErr):
			result := openResult{Outcome: "editor_refused", Reason: endpointErr.Reason, loc: loc}
			if endpointErr.Reason == editor.ReasonEditorError {
				// Neovim's message may name the absolute path; the report
				// shows the path as given (F014-FR-025).
				result.Message = cockpit.Sanitize(strings.ReplaceAll(endpointErr.Message, loc.Abs, path)).String()
			}
			return result
		default:
			return openResult{Outcome: "editor_refused", Reason: editor.ReasonNoAnswer, loc: loc}
		}
	}
	if !deps.Terminals {
		return openResult{Outcome: "editor_unavailable", Reason: "no_terminal", loc: loc}
	}
	nvim, err := deps.LookPath("nvim")
	if err != nil {
		return openResult{Outcome: "editor_unavailable", Reason: "nvim_not_found", loc: loc}
	}
	handle, err := deps.Launcher.Launch(ctx, system.Spec{Name: nvim, Args: editor.ForegroundArgs(loc), Stdin: stdin, Stdout: stdout, Stderr: stderr})
	if err != nil {
		return openResult{Outcome: "editor_failed", Target: "foreground", loc: loc}
	}
	status, _ := handle.Wait()
	if status.Exited && status.ExitCode == 0 {
		return openResult{Outcome: "opened", Target: "foreground", loc: loc}
	}
	result := openResult{Outcome: "editor_failed", Target: "foreground", loc: loc}
	if status.Exited {
		code := status.ExitCode
		result.ExitCode = &code
	}
	return result
}

var openReasonText = map[string]string{
	editor.ReasonAbsolutePath:        "is an absolute path; give a path relative to the project",
	editor.ReasonNotFound:            "does not exist",
	editor.ReasonOutsideProject:      "is outside the project",
	editor.ReasonNotRegularFile:      "is not a regular file",
	editor.ReasonNotLocalSocket:      "$NVIM does not name a local socket",
	editor.ReasonNotSocket:           "$NVIM does not name a socket",
	editor.ReasonNotOwned:            "$NVIM does not name a local socket owned by you",
	editor.ReasonNoAnswer:            "the Neovim named by $NVIM does not answer",
	editor.ReasonNotNeovim:           "the program named by $NVIM is not Neovim",
	editor.ReasonVersionUnsupported:  "the Neovim named by $NVIM is older than 0.11.7",
	editor.ReasonPlatformUnsupported: "ai-dev open supports macOS and Linux only",
	"no_terminal":                    "$NVIM is not set and standard input and output are not a terminal",
	"nvim_not_found":                 "$NVIM is not set and nvim is not on the search path",
}

func writeOpenText(r openResult, stdout, stderr io.Writer) int {
	path := cockpit.Sanitize(r.loc.Rel).String()
	switch r.Outcome {
	case "opened":
		if r.Target == "parent" {
			fmt.Fprintf(stdout, "Opened %s at line %d, column %d in the parent Neovim.\n", path, r.loc.Line, r.loc.Column)
		}
		return 0
	case "path_refused":
		if path == "" {
			path = "the path"
		}
		fmt.Fprintf(stderr, "Refused: %s %s (%s).\n", path, openReasonText[r.Reason], r.Reason)
	case "editor_refused", "editor_unavailable":
		text := openReasonText[r.Reason]
		if r.Reason == editor.ReasonEditorError {
			text = "Neovim refused to open " + path + ": " + r.Message
		}
		fmt.Fprintf(stderr, "Refused: %s (%s). Nothing was opened.\n", text, r.Reason)
	case "editor_failed":
		if r.ExitCode != nil {
			fmt.Fprintf(stderr, "Neovim exited with code %d (editor_failed).\n", *r.ExitCode)
		} else {
			fmt.Fprintln(stderr, "Neovim could not be started or did not exit normally (editor_failed).")
		}
	}
	return 1
}

func writeOpenJSON(r openResult, stdout, stderr io.Writer) int {
	wire := struct {
		SchemaVersion string `json:"schema_version"`
		Outcome       string `json:"outcome"`
		Reason        string `json:"reason,omitempty"`
		Target        string `json:"target,omitempty"`
		Path          string `json:"path"`
		Line          int    `json:"line,omitempty"`
		Column        int    `json:"column,omitempty"`
		ExitCode      *int   `json:"exit_code,omitempty"`
		Message       string `json:"message,omitempty"`
	}{SchemaVersion: openSchema, Outcome: r.Outcome, Reason: r.Reason, Target: r.Target, Path: r.loc.Rel,
		ExitCode: r.ExitCode, Message: r.Message}
	if slices.Contains([]string{"opened", "editor_failed"}, r.Outcome) {
		wire.Line, wire.Column = r.loc.Line, r.loc.Column
	}
	if err := json.NewEncoder(stdout).Encode(wire); err != nil {
		fmt.Fprintf(stderr, "write open report: %v\n", err)
		return 2
	}
	if r.Outcome == "opened" {
		return 0
	}
	return 1
}
