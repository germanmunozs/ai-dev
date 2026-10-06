package workflow

import (
	"io/fs"
	"path"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestFeatureDir(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../x", "a/../../x", "/etc/passwd", "a/b"} {
		t.Run("invalid "+id, func(t *testing.T) {
			if dir, ok := FeatureDir(id); dir != "" || ok {
				t.Fatalf("FeatureDir(%q) = (%q, %v), want (\"\", false)", id, dir, ok)
			}
		})
	}

	const id = "004-workflow-state"
	dir, ok := FeatureDir(id)
	if !ok || dir != path.Join("specs", id) {
		t.Fatalf("FeatureDir(%q) = (%q, %v), want (%q, true)", id, dir, ok, path.Join("specs", id))
	}
}

func TestFeaturePathRefactorPreservesOrderedCalls(t *testing.T) {
	const id = testFeature
	recordPath := "specs/" + id + "/" + RecordFileName
	featurePath := "specs/" + id

	t.Run("EvaluateState valid record", func(t *testing.T) {
		rec := &recordingFS{inner: featureFS(id, ptr(`{"state":"planned"}`))}
		got := EvaluateState(rec, id)
		if got.Outcome != StateReported || got.State != Planned || got.Terminal {
			t.Fatalf("EvaluateState = %+v, want planned state_reported", got)
		}
		wantCalls := []string{"Lstat:" + featurePath, "Lstat:" + recordPath, "Open:" + recordPath}
		if !reflect.DeepEqual(rec.calls, wantCalls) {
			t.Fatalf("calls = %v, want %v", rec.calls, wantCalls)
		}
	})

	t.Run("EvaluateTransition valid record", func(t *testing.T) {
		rec := &recordingFS{inner: featureFS(id, ptr(`{"state":"planned"}`))}
		got := EvaluateTransition(rec, id, "tasked")
		if got.Outcome != TransitionValid || got.CurrentState != Planned || got.Target != Tasked {
			t.Fatalf("EvaluateTransition = %+v, want planned to tasked transition_valid", got)
		}
		wantCalls := []string{"Lstat:" + featurePath, "Lstat:" + recordPath, "Open:" + recordPath}
		if !reflect.DeepEqual(rec.calls, wantCalls) {
			t.Fatalf("calls = %v, want %v", rec.calls, wantCalls)
		}
	})

	t.Run("absent record defaults draft", func(t *testing.T) {
		rec := &recordingFS{inner: featureFS(id, nil)}
		got := EvaluateState(rec, id)
		if got.Outcome != StateReported || got.State != Draft || got.Terminal {
			t.Fatalf("EvaluateState = %+v, want draft state_reported", got)
		}
		wantCalls := []string{"Lstat:" + featurePath, "Lstat:" + recordPath}
		if !reflect.DeepEqual(rec.calls, wantCalls) {
			t.Fatalf("calls = %v, want %v", rec.calls, wantCalls)
		}
	})

	t.Run("symlinked record is invalid without open", func(t *testing.T) {
		inner := fstest.MapFS{
			featurePath:                  &fstest.MapFile{Mode: fs.ModeDir | 0o755},
			recordPath:                   &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("target.json")},
			featurePath + "/target.json": &fstest.MapFile{Data: []byte(`{"state":"planned"}`)},
		}
		rec := &recordingFS{inner: inner}
		got := EvaluateState(rec, id)
		if got.Outcome != StateInvalid || got.State != "" {
			t.Fatalf("EvaluateState = %+v, want state_invalid", got)
		}
		wantCalls := []string{"Lstat:" + featurePath, "Lstat:" + recordPath}
		if !reflect.DeepEqual(rec.calls, wantCalls) {
			t.Fatalf("calls = %v, want %v", rec.calls, wantCalls)
		}
	})

	t.Run("symlinked feature is unknown", func(t *testing.T) {
		inner := fstest.MapFS{
			featurePath:         &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("real")},
			"specs/real":        &fstest.MapFile{Mode: fs.ModeDir | 0o755},
			"specs/real/unused": &fstest.MapFile{},
		}
		rec := &recordingFS{inner: inner}
		got := EvaluateState(rec, id)
		if got.Outcome != FeatureUnknown || got.State != "" {
			t.Fatalf("EvaluateState = %+v, want feature_unknown", got)
		}
		wantCalls := []string{"Lstat:" + featurePath}
		if !reflect.DeepEqual(rec.calls, wantCalls) {
			t.Fatalf("calls = %v, want %v", rec.calls, wantCalls)
		}
	})

	for _, invalidID := range []string{"..", "../x", "a/../../x", "/etc/passwd", "a/b"} {
		t.Run("invalid state "+invalidID, func(t *testing.T) {
			rec := &recordingFS{inner: featureFS(id, nil)}
			got := EvaluateState(rec, invalidID)
			if got.Outcome != FeatureUnknown || len(rec.calls) != 0 {
				t.Fatalf("EvaluateState = %+v, calls %v; want feature_unknown and zero calls", got, rec.calls)
			}
		})
		t.Run("invalid transition "+invalidID, func(t *testing.T) {
			rec := &recordingFS{inner: featureFS(id, nil)}
			got := EvaluateTransition(rec, invalidID, "planned")
			if got.Outcome != FeatureUnknown || len(rec.calls) != 0 {
				t.Fatalf("EvaluateTransition = %+v, calls %v; want feature_unknown and zero calls", got, rec.calls)
			}
		})
	}

	t.Run("filesystem without ReadLinkFS remains unknown", func(t *testing.T) {
		root := openOnlyFS{inner: featureFS(id, ptr(`{"state":"planned"}`))}
		got := EvaluateState(root, id)
		if got.Outcome != FeatureUnknown || got.State != "" {
			t.Fatalf("EvaluateState = %+v, want feature_unknown", got)
		}
	})
}

func TestFeaturePathRefactorDirectCallSequences(t *testing.T) {
	const id = testFeature
	featurePath, ok := FeatureDir(id)
	if !ok {
		t.Fatalf("FeatureDir(%q) = false", id)
	}
	recordPath := path.Join(featurePath, RecordFileName)

	rec := &recordingFS{inner: featureFS(id, ptr(`{"state":"planned"}`))}
	if !FeatureExists(rec, id) {
		t.Fatal("FeatureExists = false, want true")
	}
	if want := []string{"Lstat:" + featurePath}; !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("FeatureExists calls = %v, want %v", rec.calls, want)
	}

	rec.calls = nil
	state, invalid := ResolveRecord(rec, id)
	if state != Planned || invalid {
		t.Fatalf("ResolveRecord = (%q, %v), want (%q, false)", state, invalid, Planned)
	}
	if want := []string{"Lstat:" + recordPath, "Open:" + recordPath}; !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("ResolveRecord calls = %v, want %v", rec.calls, want)
	}
}
