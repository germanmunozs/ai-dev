//go:build (darwin && !ios) || (linux && !android)

package gatestore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

// The history reader of 006 Amendment A3 declares its own copy of the wire
// format; committing through the real store and reading back keeps both
// sides equal (015 research R-05).
func TestHistoryRoundTrip(t *testing.T) {
	repo := t.TempDir()
	for _, feature := range []string{"approved", "abandoned"} {
		if err := os.MkdirAll(filepath.Join(repo, "specs", feature), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store := New(repo)
	decidedAt := time.Date(2026, 11, 26, 9, 30, 15, 123456789, time.FixedZone("x", -5*3600))
	commits := map[string][]gate.Evidence{
		"approved": {
			{FeatureID: "approved", Decision: gate.Approval, Approver: "ana@example.com",
				FromState: workflow.Draft, ToState: workflow.Implemented, DecidedAt: decidedAt},
			{FeatureID: "approved", Decision: gate.Approval, Approver: "ana@example.com",
				FromState: workflow.Implemented, ToState: workflow.Approved, DecidedAt: decidedAt.Add(time.Hour)},
		},
		"abandoned": {
			{FeatureID: "abandoned", Decision: gate.Abandonment, Approver: "bo \"λ\"",
				FromState: workflow.Draft, ToState: workflow.Abandoned, DecidedAt: decidedAt,
				Reason: "no \"longer\"\nneeded — ñ", ReasonSet: true},
		},
	}
	for feature, entries := range commits {
		for _, entry := range entries {
			if status, warnings := store.Commit(feature, entry.FromState, entry); status != gate.Committed {
				t.Fatalf("commit %s %s: status=%v warnings=%v", feature, entry.ToState, status, warnings)
			}
		}
	}
	for feature, want := range commits {
		got, ok := gate.ReadHistory(os.DirFS(repo), feature)
		if !ok || len(got) != len(want) {
			t.Fatalf("%s: ReadHistory = %v, %v; want %d entries", feature, got, ok, len(want))
		}
		for i := range want {
			w := want[i]
			w.DecidedAt = w.DecidedAt.Truncate(time.Second).UTC() // the store records RFC 3339 seconds in UTC
			g := got[i]
			if g.FeatureID != w.FeatureID || g.Decision != w.Decision || g.Approver != w.Approver ||
				g.FromState != w.FromState || g.ToState != w.ToState || !g.DecidedAt.Equal(w.DecidedAt) ||
				g.Reason != w.Reason || g.ReasonSet != w.ReasonSet {
				t.Errorf("%s entry %d = %+v; want %+v", feature, i, g, w)
			}
		}
	}
}
