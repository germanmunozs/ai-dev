// Package cockpit is the presentation of "ai-dev tui": a Bubble Tea model
// over plain data. It imports no domain package, writes no file, starts no
// process of its own and holds no authority (F012-FR-017): every fact comes
// from a Source and every action is a command handed to a Runner, both
// composed by internal/app.
package cockpit

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// FeatureRow is one feature as the existing capabilities report it
// (data-model.md). The words are the capabilities' own values, sanitized.
type FeatureRow struct {
	// Name is the directory name, sanitized for display; ID is the raw
	// name, used only as a command argument.
	Name Text
	ID   string
	// State is the Workflow State, or the 004 outcome when there is none.
	State Text
	// Next is the 005 outcome, with the activity for step_recommended.
	Next Text
	// Role, Agent, Availability and Source are 007 and 011's, for
	// step_recommended only. AgentID is the raw agent identifier.
	Role, Agent, Availability, Source Text
	AgentID                           string
	// Detail is the text of "workflow state" and "workflow guide".
	Detail []Text
	// Recognized: the state is one of the canonical values. Terminal: it
	// is approved or abandoned.
	Recognized, Terminal bool
	// StepRecommended and AgentAvailable decide "run agent" (F012-FR-011).
	StepRecommended, AgentAvailable bool
}

// Source evaluates the read-only views. Each method is called once per
// refresh in its own tea.Cmd, so the areas load concurrently; none may
// write, start a coding agent or contact the network (F012-FR-007).
type Source interface {
	LoadFeatures(ctx context.Context) ([]FeatureRow, error)
	LoadEnvironment(ctx context.Context) ([]Text, error)
	LoadAgents(ctx context.Context) ([]Text, error)
	LoadRepository(ctx context.Context) ([]Text, error)
}

// Runner hands the terminal to an ai-dev command (F012-FR-008).
type Runner interface {
	// Command returns the command for args, the words after "ai-dev".
	Command(args []string) tea.ExecCommand
	// Preview is the command line shown in the review.
	Preview(args []string) string
}

// CommandExit is the error a Runner's command returns when the command
// exited with a non-zero code, or was terminated by a signal.
type CommandExit struct {
	Code     int
	Signaled bool
}

func (e CommandExit) Error() string {
	if e.Signaled {
		return "the command was terminated by a signal"
	}
	return fmt.Sprintf("the command exited with code %d", e.Code)
}

// SignalMsg is an interrupt or termination signal forwarded by
// internal/app, which disables Bubble Tea's own handler (R-04).
type SignalMsg struct{ Terminate bool }

// loadStatus is the state of one area's evaluation.
type loadStatus int

const (
	loading loadStatus = iota
	ready
	failed
)

// String gives every status as a word (F012-FR-019).
func (s loadStatus) String() string {
	switch s {
	case ready:
		return "ready"
	case failed:
		return "failed"
	default:
		return "evaluating…"
	}
}

// area is one line-oriented view: Environment, Agents or Repository.
type area struct {
	status loadStatus
	lines  []Text
	err    Text
}
