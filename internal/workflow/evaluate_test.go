package workflow

import (
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
)

func cloneMapFS(src fstest.MapFS) fstest.MapFS {
	out := fstest.MapFS{}
	for name, file := range src {
		copied := *file
		copied.Data = append([]byte(nil), file.Data...)
		out[name] = &copied
	}
	return out
}

func TestEvaluateStateReportsEveryCanonicalState(t *testing.T) {
	for _, s := range allStates {
		fsys := featureFS(testFeature, ptr(`{"state":"`+string(s)+`"}`))
		got := EvaluateState(fsys, testFeature)
		want := StateReport{FeatureID: testFeature, State: s, Terminal: Terminal(s), Outcome: StateReported}
		if got != want {
			t.Fatalf("EvaluateState = %+v, want %+v", got, want)
		}
	}
}

func TestEvaluateStateDefaultsAbsentRecordToDraft(t *testing.T) {
	got := EvaluateState(featureFS(testFeature, nil), testFeature)
	want := StateReport{FeatureID: testFeature, State: Draft, Terminal: false, Outcome: StateReported}
	if got != want {
		t.Fatalf("EvaluateState = %+v, want %+v", got, want)
	}
}

func TestEvaluateStateIsDeterministicAndDoesNotModifyFixture(t *testing.T) {
	fsys := featureFS(testFeature, ptr(`{"state":"approved"}`))
	before := cloneMapFS(fsys)
	first := EvaluateState(fsys, testFeature)
	second := EvaluateState(fsys, testFeature)
	if first != second {
		t.Fatalf("repeated calls differ: %+v vs %+v", first, second)
	}
	if !reflect.DeepEqual(before, fsys) {
		t.Fatal("fixture was modified by EvaluateState")
	}
}

func TestEvaluateTransitionCoversEveryRuleSetPair(t *testing.T) {
	for _, from := range allStates {
		for _, to := range allStates {
			fsys := featureFS(testFeature, ptr(`{"state":"`+string(from)+`"}`))
			got := EvaluateTransition(fsys, testFeature, string(to))
			outcome := TransitionInvalid
			if validEdges[[2]State{from, to}] {
				outcome = TransitionValid
			}
			want := TransitionReport{FeatureID: testFeature, CurrentState: from, SuppliedTarget: string(to), Target: to, Outcome: outcome}
			if got != want {
				t.Fatalf("EvaluateTransition(%q -> %q) = %+v, want %+v", from, to, got, want)
			}
		}
	}
}

func TestEvaluateTransitionNamedInvalidCases(t *testing.T) {
	cases := []struct {
		name     string
		from, to State
	}{
		{"skip-ahead", Specified, Tasked},
		{"backward", Implemented, Specified},
		{"same-state", Planned, Planned},
		{"terminal approved", Approved, Abandoned},
		{"terminal abandoned", Abandoned, Specified},
	}
	for _, tc := range cases {
		got := EvaluateTransition(featureFS(testFeature, ptr(`{"state":"`+string(tc.from)+`"}`)), testFeature, string(tc.to))
		if got.Outcome != TransitionInvalid {
			t.Fatalf("%s: outcome = %q, want transition_invalid", tc.name, got.Outcome)
		}
	}
}

func TestEvaluateTransitionUnsupportedTargetPerformsZeroFilesystemAccess(t *testing.T) {
	fixtures := map[string]fstest.MapFS{
		"feature and valid record": featureFS(testFeature, ptr(`{"state":"planned"}`)),
		"feature and bad record":   featureFS(testFeature, ptr(`not json`)),
		"no feature":               {},
	}
	for name, fixture := range fixtures {
		for _, target := range []string{"reviewing", "Planned", " planned "} {
			rec := &recordingFS{inner: fixture}
			got := EvaluateTransition(rec, testFeature, target)
			want := TransitionReport{FeatureID: testFeature, SuppliedTarget: target, Outcome: TargetUnsupported}
			if got != want {
				t.Fatalf("%s/%q: got %+v, want %+v", name, target, got, want)
			}
			if len(rec.calls) != 0 {
				t.Fatalf("%s/%q: target_unsupported accessed the filesystem: %v", name, target, rec.calls)
			}
		}
	}

	rec := &recordingFS{inner: featureFS(testFeature, ptr(`{"state":"planned"}`))}
	if got := EvaluateTransition(rec, testFeature, "tasked"); got.Outcome != TransitionValid {
		t.Fatalf("positive control outcome = %q", got.Outcome)
	}
	if len(rec.calls) == 0 {
		t.Fatal("positive control recorded no access; the zero-access proof would be vacuous")
	}
}

func TestEvaluateTransitionFourPrecedenceCombinations(t *testing.T) {
	invalidRecord := featureFS(testFeature, ptr(`{"state":"Planned"}`))
	cases := []struct {
		name    string
		fixture fstest.MapFS
		target  string
		want    TransitionReport
	}{
		{"unsupported + unknown feature", fstest.MapFS{}, "reviewing",
			TransitionReport{FeatureID: testFeature, SuppliedTarget: "reviewing", Outcome: TargetUnsupported}},
		{"unsupported + invalid record", invalidRecord, "reviewing",
			TransitionReport{FeatureID: testFeature, SuppliedTarget: "reviewing", Outcome: TargetUnsupported}},
		{"canonical + unknown feature", fstest.MapFS{}, "tasked",
			TransitionReport{FeatureID: testFeature, Outcome: FeatureUnknown}},
		{"canonical + invalid record", invalidRecord, "tasked",
			TransitionReport{FeatureID: testFeature, Outcome: StateInvalid}},
	}
	for _, tc := range cases {
		if got := EvaluateTransition(tc.fixture, testFeature, tc.target); got != tc.want {
			t.Fatalf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestEvaluateTransitionRawFeatureIDContainment(t *testing.T) {
	fixture := featureFS(testFeature, ptr(`{"state":"planned"}`))
	for _, id := range []string{"..", "../x", "a/../../x", "/etc/passwd", "a/b"} {
		rec := &recordingFS{inner: fixture}
		if got, want := EvaluateTransition(rec, id, "tasked"), (TransitionReport{FeatureID: id, Outcome: FeatureUnknown}); got != want {
			t.Fatalf("%q canonical: got %+v, want %+v", id, got, want)
		}
		if len(rec.calls) != 0 {
			t.Fatalf("%q canonical: accessed the filesystem: %v", id, rec.calls)
		}
		rec = &recordingFS{inner: fixture}
		if got, want := EvaluateTransition(rec, id, "reviewing"), (TransitionReport{FeatureID: id, SuppliedTarget: "reviewing", Outcome: TargetUnsupported}); got != want {
			t.Fatalf("%q unsupported: got %+v, want %+v", id, got, want)
		}
		if len(rec.calls) != 0 {
			t.Fatalf("%q unsupported: accessed the filesystem: %v", id, rec.calls)
		}
	}

	rec := &recordingFS{inner: fixture}
	if got := EvaluateTransition(rec, testFeature, "tasked"); got.Outcome != TransitionValid {
		t.Fatalf("valid control canonical: outcome = %q", got.Outcome)
	}
	if len(rec.calls) == 0 || rec.calls[0] != "Lstat:specs/004-workflow-state" {
		t.Fatalf("valid control canonical: accesses = %v, want first access Lstat:specs/004-workflow-state", rec.calls)
	}
	rec = &recordingFS{inner: fixture}
	if got := EvaluateTransition(rec, testFeature, "reviewing"); got.Outcome != TargetUnsupported {
		t.Fatalf("valid control unsupported: outcome = %q", got.Outcome)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("valid control unsupported: accessed the filesystem: %v", rec.calls)
	}
}

func TestEvaluateTransitionIsDeterministic(t *testing.T) {
	fsys := featureFS(testFeature, ptr(`{"state":"implemented"}`))
	for _, target := range []string{"approved", "draft", "reviewing"} {
		if first, second := EvaluateTransition(fsys, testFeature, target), EvaluateTransition(fsys, testFeature, target); first != second {
			t.Fatalf("repeated calls differ for %q: %+v vs %+v", target, first, second)
		}
	}
}

func TestFeatureUnknownForBothEvaluatorsPopulatesNoStateFields(t *testing.T) {
	fixtures := map[string]fstest.MapFS{
		"empty filesystem":      {},
		"other feature only":    featureFS("001-other", ptr(`{"state":"planned"}`)),
		"specs entry is a file": {"specs/" + testFeature: &fstest.MapFile{Data: []byte(`{"state":"planned"}`)}},
	}
	for name, fixture := range fixtures {
		if got, want := EvaluateState(fixture, testFeature), (StateReport{FeatureID: testFeature, Outcome: FeatureUnknown}); got != want {
			t.Fatalf("%s: EvaluateState = %+v, want %+v", name, got, want)
		}
		for _, target := range []string{"specified", "approved", "draft"} {
			if got, want := EvaluateTransition(fixture, testFeature, target), (TransitionReport{FeatureID: testFeature, Outcome: FeatureUnknown}); got != want {
				t.Fatalf("%s/%s: EvaluateTransition = %+v, want %+v", name, target, got, want)
			}
		}
	}
}

func TestStateInvalidPropagatesThroughBothEvaluatorsWithoutSubstitution(t *testing.T) {
	symlinked := featureFS(testFeature, nil)
	symlinked["specs/"+testFeature+"/"+RecordFileName] = &fstest.MapFile{Data: []byte("elsewhere.json"), Mode: 0o777 | fs.ModeSymlink}
	symlinked["specs/"+testFeature+"/elsewhere.json"] = &fstest.MapFile{Data: []byte(`{"state":"planned"}`)}
	fixtures := map[string]fstest.MapFS{
		"non-canonical value": featureFS(testFeature, ptr(`{"state":"reviewing"}`)),
		"malformed record":    featureFS(testFeature, ptr(`{"state":`)),
		"symlinked record":    symlinked,
	}
	for name, fixture := range fixtures {
		if got, want := EvaluateState(fixture, testFeature), (StateReport{FeatureID: testFeature, Outcome: StateInvalid}); got != want {
			t.Fatalf("%s: EvaluateState = %+v, want %+v", name, got, want)
		}
		for _, target := range allStates {
			if got, want := EvaluateTransition(fixture, testFeature, string(target)), (TransitionReport{FeatureID: testFeature, Outcome: StateInvalid}); got != want {
				t.Fatalf("%s/%s: EvaluateTransition = %+v, want %+v", name, target, got, want)
			}
		}
	}
}
