package gate

import (
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	"github.com/germanmunozs/ai-dev/internal/workflow"
)

type fakeStore struct {
	status    CommitStatus
	warnings  []string
	calls     int
	featureID string
	expected  workflow.State
	entry     Evidence
}

func (s *fakeStore) Commit(featureID string, expected workflow.State, entry Evidence) (CommitStatus, []string) {
	s.calls++
	s.featureID = featureID
	s.expected = expected
	s.entry = entry
	return s.status, s.warnings
}

func gateFixture(state *workflow.State, files ...string) fstest.MapFS {
	dir := "specs/f"
	m := fstest.MapFS{dir: &fstest.MapFile{Mode: fs.ModeDir | 0o755}}
	if state != nil {
		m[dir+"/workflow-state.json"] = &fstest.MapFile{Data: []byte(`{"state":"` + string(*state) + `"}`), Mode: 0o644}
	}
	for _, name := range files {
		m[dir+"/"+name] = &fstest.MapFile{Mode: 0o644}
	}
	return m
}

func TestEvaluateRefusalsAndPrecedence(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.FixedZone("x", 3600)) }
	cases := []struct {
		name string
		root fs.FS
		req  Request
		want Outcome
	}{
		{"unsupported", gateFixture(nil), Request{FeatureID: "missing", Decision: Approval, Approver: "a", SuppliedTarget: "Planned"}, TargetUnsupported},
		{"unknown", gateFixture(nil), Request{FeatureID: "missing", Decision: Approval, Approver: "a", SuppliedTarget: "specified"}, FeatureUnknown},
		{"invalid", fstest.MapFS{"specs/f": {Mode: fs.ModeDir | 0o755}, "specs/f/workflow-state.json": {Data: []byte(`{"state":"bad"}`), Mode: 0o644}}, Request{FeatureID: "f", Decision: Approval, Approver: "a", SuppliedTarget: "planned"}, StateInvalid},
		{"transition invalid before artifacts", gateFixture(ptrState(workflow.Specified)), Request{FeatureID: "f", Decision: Approval, Approver: "a", SuppliedTarget: "tasked"}, TransitionInvalid},
		{"artifacts missing", gateFixture(ptrState(workflow.Specified)), Request{FeatureID: "f", Decision: Approval, Approver: "a", SuppliedTarget: "planned"}, ArtifactsMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{status: Committed}
			got, _ := Evaluate(tc.root, tc.req, store, now)
			if got.Outcome != tc.want || !got.Valid() {
				t.Fatalf("report=%+v", got)
			}
			if store.calls != 0 {
				t.Fatalf("store calls=%d", store.calls)
			}
		})
	}
}

func TestEvaluateStoreMappingsAndEvidence(t *testing.T) {
	fixed := time.Date(2026, 10, 7, 12, 0, 0, 123, time.FixedZone("x", 3600))
	req := Request{FeatureID: "f", Decision: Approval, Approver: " a\n ", SuppliedTarget: "planned", Reason: "ok", ReasonSet: true}
	root := gateFixture(ptrState(workflow.Specified), "spec.md", "plan.md")
	for _, tc := range []struct {
		status CommitStatus
		want   Outcome
	}{{Committed, TransitionApplied}, {Conflict, StateConflict}, {Failed, PersistenceFailed}, {0, PersistenceFailed}} {
		store := &fakeStore{status: tc.status, warnings: []string{"w"}}
		got, warnings := Evaluate(root, req, store, func() time.Time { return fixed })
		if got.Outcome != tc.want || !got.Valid() {
			t.Fatalf("status %d report=%+v", tc.status, got)
		}
		if store.calls != 1 || store.expected != workflow.Specified || store.featureID != "f" {
			t.Fatalf("store=%+v", store)
		}
		wantEntry := Evidence{FeatureID: "f", Decision: Approval, Approver: " a\n ", FromState: workflow.Specified, ToState: workflow.Planned, DecidedAt: fixed.UTC(), Reason: "ok", ReasonSet: true}
		if !reflect.DeepEqual(store.entry, wantEntry) || !reflect.DeepEqual(warnings, []string{"w"}) {
			t.Fatalf("entry=%+v warnings=%v", store.entry, warnings)
		}
	}
}

func TestEvaluateAbandonmentSkipsArtifacts(t *testing.T) {
	store := &fakeStore{status: Committed}
	req := Request{FeatureID: "f", Decision: Abandonment, Approver: "a", SuppliedTarget: "abandoned", Reason: "stop", ReasonSet: true}
	got, _ := Evaluate(gateFixture(ptrState(workflow.Planned)), req, store, time.Now)
	if got.Outcome != TransitionApplied || store.calls != 1 {
		t.Fatalf("report=%+v calls=%d", got, store.calls)
	}
}

func ptrState(s workflow.State) *workflow.State { return &s }
