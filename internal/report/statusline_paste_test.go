package report

import (
	"bytes"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/sdd"
	"github.com/germanmunozs/ai-dev/internal/selection"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

// TestStatusLinesPasteInert renders every 004-007 human-readable report
// with hostile supplied values (feature identifier, approver, reason and
// supplied target) and pastes the whole output into every shell mode:
// nothing may run and no file may change (004 FR-028, Amendment 5).
func TestStatusLinesPasteInert(t *testing.T) {
	const feature = "$(touch PWNED-feature)"
	const approver = "`touch PWNED-approver`"
	const reason = "$(touch PWNED-reason) ${HOME} !!"
	const target = "$(touch PWNED-target)"

	type rendered struct {
		name  string
		write func(*bytes.Buffer) error
	}
	var reports []rendered
	add := func(name string, write func(*bytes.Buffer) error) {
		reports = append(reports, rendered{name, write})
	}
	add("state reported", func(b *bytes.Buffer) error {
		return WriteWorkflowStateText(b, workflow.StateReport{FeatureID: feature, State: workflow.Planned, Outcome: workflow.StateReported})
	})
	add("state unknown", func(b *bytes.Buffer) error {
		return WriteWorkflowStateText(b, workflow.StateReport{FeatureID: feature, Outcome: workflow.FeatureUnknown})
	})
	add("transition valid", func(b *bytes.Buffer) error {
		return WriteWorkflowTransitionText(b, workflow.TransitionReport{FeatureID: feature, CurrentState: workflow.Specified, SuppliedTarget: "planned", Target: workflow.Planned, Outcome: workflow.TransitionValid})
	})
	add("transition target unsupported", func(b *bytes.Buffer) error {
		return WriteWorkflowTransitionText(b, workflow.TransitionReport{FeatureID: feature, SuppliedTarget: target, Outcome: workflow.TargetUnsupported})
	})
	add("next unknown", func(b *bytes.Buffer) error {
		return WriteWorkflowNextText(b, sdd.Report{FeatureID: feature, Outcome: sdd.FeatureUnknown})
	})
	for _, base := range gateReports() {
		r := base
		r.FeatureID, r.Approver = feature, approver
		if r.Outcome == gate.TargetUnsupported {
			r.SuppliedTarget = target
		}
		if r.Outcome == gate.TransitionApplied {
			r.Reason, r.ReasonSet = reason, true
		}
		add("gate "+string(r.Outcome), func(b *bytes.Buffer) error { return WriteWorkflowGateText(b, r) })
	}
	guideStep := guideReport(t, feature, "specified", selection.RoleSelected, false)
	add("guide step", func(b *bytes.Buffer) error { return WriteGuideText(b, guideStep) })

	shells := guideShells(t)
	sentinels := sentinelPath(t)
	for _, report := range reports {
		var text bytes.Buffer
		if err := report.write(&text); err != nil {
			t.Fatalf("%s: %v", report.name, err)
		}
		for name, shell := range shells {
			invocations, changed := paste(t, sentinels, shell, text.String())
			if changed {
				t.Fatalf("%s, %s: pasting the report changed files:\n%s", report.name, name, text.String())
			}
			for _, invocation := range invocations {
				if invocation[0] != "ai-dev" || len(invocation) < 2 || (invocation[1] != "agent" && invocation[1] != "workflow") {
					t.Fatalf("%s, %s: pasting the report ran %q:\n%s", report.name, name, invocation, text.String())
				}
			}
		}
	}
}
