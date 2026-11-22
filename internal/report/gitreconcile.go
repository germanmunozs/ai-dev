package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

// WriteGitReconciliationJSON writes exactly one reconciliation report.
func WriteGitReconciliationJSON(w io.Writer, r gitmanager.ReconciliationReport) error {
	data, err := gitmanager.EncodeReconciliationReport(r)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// WriteGitReconciliationText writes one stable block. It states the only
// way to continue and that the report grants nothing.
func WriteGitReconciliationText(w io.Writer, r gitmanager.ReconciliationReport) error {
	if _, err := gitmanager.EncodeReconciliationReport(r); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "authorization:  %s (%s, version %d)\n", r.Prior.AuthorizationID, r.Prior.State, r.Prior.StateVersion)
	fmt.Fprintf(&b, "plan digest:    %s\n", r.Prior.PlanDigest)
	if r.Prior.OwnerApplyID != "" {
		fmt.Fprintf(&b, "apply attempt:  %s\n", r.Prior.OwnerApplyID)
	}
	fmt.Fprintf(&b, "unresolved:     %t\n", r.Unresolved)
	switch {
	case !r.TrailReadable:
		b.WriteString("audit trail:    unreadable\n")
	default:
		events := make([]string, len(r.Trail.Events))
		for i, event := range r.Trail.Events {
			events[i] = string(event)
		}
		fmt.Fprintf(&b, "audit trail:    %s\n", strings.Join(events, ", "))
		if r.Trail.CrashEvidence {
			b.WriteString("crash evidence: unpublished temporaries are present\n")
		}
	}
	for _, group := range []struct {
		label      string
		conditions []gitmanager.Condition
	}{{"applied:", r.Applied}, {"absent:", r.Absent}, {"unverifiable:", r.Unverifiable}} {
		for _, condition := range group.conditions {
			fmt.Fprintf(&b, "%-15s %s\n", group.label, conditionText(condition))
		}
	}
	if r.Unresolved {
		b.WriteString("continue:       plan again and apply with --issue-authorization --reconciles-authorization " + string(r.Prior.AuthorizationID) + "\n")
	}
	b.WriteString("authority:      none; this report grants no mutation authority\n")
	_, err := io.WriteString(w, b.String())
	return err
}
