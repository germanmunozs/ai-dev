package sdd

import (
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/role"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

type countingLookup struct {
	calls []workflow.State
	omit  workflow.State
}

func (c *countingLookup) lookup(state workflow.State) (Activity, role.Role, bool) {
	c.calls = append(c.calls, state)
	if state == c.omit {
		return "", "", false
	}
	return nextStep(state)
}

func TestEvaluateEveryStateAndOutcome(t *testing.T) {
	records := map[workflow.State]*string{workflow.Draft: nil}
	for _, state := range []workflow.State{workflow.Specified, workflow.Planned, workflow.Tasked, workflow.Implemented, workflow.Approved, workflow.Abandoned} {
		value := `{"state":"` + string(state) + `"}`
		records[state] = &value
	}
	cases := []struct {
		state    workflow.State
		files    []string
		outcome  Outcome
		activity Activity
		role     role.Role
	}{
		{workflow.Draft, nil, StepRecommended, ActivitySpecify, role.Specification},
		{workflow.Specified, []string{SpecArtifact}, StepRecommended, ActivityPlan, role.Planning},
		{workflow.Planned, []string{SpecArtifact, PlanArtifact}, StepRecommended, ActivityTasks, role.Planning},
		{workflow.Tasked, []string{SpecArtifact, PlanArtifact, TasksArtifact}, StepRecommended, ActivityImplement, role.Implementation},
		{workflow.Implemented, []string{SpecArtifact, PlanArtifact, TasksArtifact}, StepRecommended, ActivityReview, role.Review},
		{workflow.Approved, []string{SpecArtifact, PlanArtifact, TasksArtifact}, WorkflowTerminal, "", ""},
		{workflow.Abandoned, nil, WorkflowTerminal, "", ""},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			got := Evaluate(sddFixture(records[tc.state], tc.files...), sddFeatureID)
			if got.Outcome != tc.outcome || got.State != tc.state || got.Activity != tc.activity || got.Role != tc.role || len(got.Missing) != 0 {
				t.Fatalf("Evaluate = %+v", got)
			}
			if !got.Valid() {
				t.Fatalf("Evaluate returned invalid report %+v", got)
			}
		})
	}

	t.Run("feature unknown", func(t *testing.T) {
		got := Evaluate(fstest.MapFS{}, sddFeatureID)
		if got.Outcome != FeatureUnknown || !got.Valid() {
			t.Fatalf("Evaluate = %+v, want valid feature_unknown", got)
		}
	})
	t.Run("state invalid", func(t *testing.T) {
		record := `{"state":"Planned"}`
		got := Evaluate(sddFixture(&record), sddFeatureID)
		if got.Outcome != StateInvalid || !got.Valid() {
			t.Fatalf("Evaluate = %+v, want valid state_invalid", got)
		}
	})
	t.Run("artifacts inconsistent", func(t *testing.T) {
		record := `{"state":"implemented"}`
		got := Evaluate(sddFixture(&record), sddFeatureID)
		wantMissing := []string{SpecArtifact, PlanArtifact, TasksArtifact}
		if got.Outcome != ArtifactsInconsistent || got.State != workflow.Implemented || !reflect.DeepEqual(got.Missing, wantMissing) || !got.Valid() {
			t.Fatalf("Evaluate = %+v, want artifacts_inconsistent %v", got, wantMissing)
		}
	})
}

func TestEvaluatePrecedenceAndLookupCalls(t *testing.T) {
	t.Run("feature unknown", func(t *testing.T) {
		lookup := &countingLookup{}
		got := evaluate(fstest.MapFS{}, sddFeatureID, lookup.lookup)
		if got.Outcome != FeatureUnknown || len(lookup.calls) != 0 {
			t.Fatalf("Evaluate = %+v, lookup calls = %v", got, lookup.calls)
		}
	})
	for _, id := range []string{"..", "../x", "a/../../x", "/etc/passwd", "a/b"} {
		t.Run("invalid "+id, func(t *testing.T) {
			lookup := &countingLookup{}
			got := evaluate(sddFixture(nil), id, lookup.lookup)
			if got.Outcome != FeatureUnknown || len(lookup.calls) != 0 {
				t.Fatalf("Evaluate = %+v, lookup calls = %v", got, lookup.calls)
			}
		})
	}
	t.Run("symlinked feature", func(t *testing.T) {
		dir, _ := workflow.FeatureDir(sddFeatureID)
		root := &recordingSDDFS{inner: fstest.MapFS{dir: &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("real")}, "specs/real": &fstest.MapFile{Mode: fs.ModeDir}}}
		lookup := &countingLookup{}
		got := evaluate(root, sddFeatureID, lookup.lookup)
		if got.Outcome != FeatureUnknown || len(lookup.calls) != 0 || !reflect.DeepEqual(root.calls, []string{"Lstat:" + dir}) {
			t.Fatalf("Evaluate = %+v, lookup calls = %v, filesystem calls = %v", got, lookup.calls, root.calls)
		}
	})
	t.Run("state invalid before artifacts", func(t *testing.T) {
		record := `{`
		lookup := &countingLookup{}
		got := evaluate(sddFixture(&record), sddFeatureID, lookup.lookup)
		if got.Outcome != StateInvalid || len(lookup.calls) != 0 {
			t.Fatalf("Evaluate = %+v, lookup calls = %v", got, lookup.calls)
		}
	})
	t.Run("artifacts before terminal", func(t *testing.T) {
		record := `{"state":"approved"}`
		lookup := &countingLookup{}
		got := evaluate(sddFixture(&record, SpecArtifact, PlanArtifact), sddFeatureID, lookup.lookup)
		if got.Outcome != ArtifactsInconsistent || !reflect.DeepEqual(got.Missing, []string{TasksArtifact}) || len(lookup.calls) != 0 {
			t.Fatalf("Evaluate = %+v, lookup calls = %v", got, lookup.calls)
		}
	})
	for _, state := range []workflow.State{workflow.Approved, workflow.Abandoned} {
		t.Run("terminal "+string(state), func(t *testing.T) {
			record := `{"state":"` + string(state) + `"}`
			files := requiredArtifacts(state)
			lookup := &countingLookup{}
			got := evaluate(sddFixture(&record, files...), sddFeatureID, lookup.lookup)
			if got.Outcome != WorkflowTerminal || len(lookup.calls) != 0 {
				t.Fatalf("Evaluate = %+v, lookup calls = %v", got, lookup.calls)
			}
		})
	}
	for _, state := range []workflow.State{workflow.Draft, workflow.Specified, workflow.Planned, workflow.Tasked, workflow.Implemented} {
		t.Run("recommended "+string(state), func(t *testing.T) {
			var record *string
			if state != workflow.Draft {
				value := `{"state":"` + string(state) + `"}`
				record = &value
			}
			lookup := &countingLookup{}
			got := evaluate(sddFixture(record, requiredArtifacts(state)...), sddFeatureID, lookup.lookup)
			if got.Outcome != StepRecommended || !reflect.DeepEqual(lookup.calls, []workflow.State{state}) {
				t.Fatalf("Evaluate = %+v, lookup calls = %v", got, lookup.calls)
			}
		})
	}
	t.Run("ahead artifacts do not advance state", func(t *testing.T) {
		record := `{"state":"specified"}`
		got := Evaluate(sddFixture(&record, SpecArtifact, PlanArtifact, TasksArtifact), sddFeatureID)
		if got.Outcome != StepRecommended || got.Activity != ActivityPlan || got.State != workflow.Specified {
			t.Fatalf("Evaluate = %+v", got)
		}
	})
}

func TestEvaluateFilesystemCallOrder(t *testing.T) {
	dir, _ := workflow.FeatureDir(sddFeatureID)
	recordPath := dir + "/" + workflow.RecordFileName

	for _, id := range []string{"..", "../x", "a/../../x", "/etc/passwd", "a/b"} {
		rec := &recordingSDDFS{inner: sddFixture(nil)}
		if got := Evaluate(rec, id); got.Outcome != FeatureUnknown || len(rec.calls) != 0 {
			t.Fatalf("id %q: report=%+v calls=%v", id, got, rec.calls)
		}
	}

	t.Run("absent feature", func(t *testing.T) {
		rec := &recordingSDDFS{inner: fstest.MapFS{}}
		got := Evaluate(rec, sddFeatureID)
		want := []string{"Lstat:" + dir}
		if got.Outcome != FeatureUnknown || !reflect.DeepEqual(rec.calls, want) {
			t.Fatalf("report=%+v calls=%v want=%v", got, rec.calls, want)
		}
	})

	t.Run("state invalid", func(t *testing.T) {
		value := `{`
		rec := &recordingSDDFS{inner: sddFixture(&value)}
		got := Evaluate(rec, sddFeatureID)
		readDirs := 0
		for _, call := range rec.calls {
			if len(call) >= len("ReadDir:") && call[:len("ReadDir:")] == "ReadDir:" {
				readDirs++
			}
		}
		if got.Outcome != StateInvalid || readDirs != 0 {
			t.Fatalf("report=%+v calls=%v", got, rec.calls)
		}
	})

	for _, state := range []workflow.State{workflow.Draft, workflow.Abandoned} {
		t.Run("no listing "+string(state), func(t *testing.T) {
			var record *string
			if state == workflow.Abandoned {
				value := `{"state":"abandoned"}`
				record = &value
			}
			rec := &recordingSDDFS{inner: sddFixture(record)}
			got := Evaluate(rec, sddFeatureID)
			readDirs := 0
			for _, call := range rec.calls {
				if len(call) >= len("ReadDir:") && call[:len("ReadDir:")] == "ReadDir:" {
					readDirs++
				}
			}
			if got.Outcome != map[workflow.State]Outcome{workflow.Draft: StepRecommended, workflow.Abandoned: WorkflowTerminal}[state] || readDirs != 0 {
				t.Fatalf("report=%+v calls=%v", got, rec.calls)
			}
		})
	}

	for _, state := range []workflow.State{workflow.Specified, workflow.Planned, workflow.Tasked, workflow.Implemented, workflow.Approved} {
		t.Run("one listing "+string(state), func(t *testing.T) {
			value := `{"state":"` + string(state) + `"}`
			rec := &recordingSDDFS{inner: sddFixture(&value, requiredArtifacts(state)...)}
			_ = Evaluate(rec, sddFeatureID)
			readDirs, dirListings, recordLstats, recordReads := 0, 0, 0, 0
			for _, call := range rec.calls {
				if len(call) >= len("ReadDir:") && call[:len("ReadDir:")] == "ReadDir:" {
					readDirs++
				}
				if call == "ReadDir:"+dir {
					dirListings++
				}
				if call == "Lstat:"+recordPath {
					recordLstats++
				}
				if call == "ReadFile:"+recordPath {
					recordReads++
				}
			}
			if dirListings != 1 || readDirs != 1 {
				t.Fatalf("calls=%v, want exactly one ReadDir on %s", rec.calls, dir)
			}
			if recordLstats != 1 || recordReads != 1 {
				t.Fatalf("record access calls=%v", rec.calls)
			}
		})
	}
}

func TestEvaluateParityWithWorkflowState(t *testing.T) {
	invalidRecords := []string{
		`{}`, `{"state":null}`, `{"state":""}`, `{"state":"   "}`,
		`{"state":"Planned"}`, `{"state":" planned "}`, `{"state":"reviewing"}`, `not json`,
		``, `{"state":3}`, `"planned"`, `null`, `[{"state":"planned"}]`, `{"state":"planned"} {}`,
		`{"State":"approved"}`, `{"STATE":"planned"}`, `{"ſtate":"approved"}`, `{" state":"planned"}`,
		`{"state":"draft","State":"approved"}`, `{"State":"approved","state":"draft"}`,
		`{"state":"draft","state":"approved"}`, `{"state":"planned","state":"planned"}`,
		`{"state":null,"state":"planned"}`, `{"meta":{"state":"approved"}}`,
	}
	for _, record := range invalidRecords {
		root := sddFixture(&record)
		stateReport := workflow.EvaluateState(root, sddFeatureID)
		got := Evaluate(root, sddFeatureID)
		if stateReport.Outcome != workflow.StateInvalid || got.Outcome != StateInvalid {
			t.Fatalf("record %q: workflow=%+v sdd=%+v", record, stateReport, got)
		}
	}
	dir, _ := workflow.FeatureDir(sddFeatureID)
	for name, root := range map[string]fstest.MapFS{
		"symlinked record": {
			dir:                                 &fstest.MapFile{Mode: fs.ModeDir | 0o755},
			dir + "/" + workflow.RecordFileName: &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("target")},
			dir + "/target":                     &fstest.MapFile{Data: []byte(`{"state":"planned"}`)},
		},
		"directory record": {
			dir:                                 &fstest.MapFile{Mode: fs.ModeDir | 0o755},
			dir + "/" + workflow.RecordFileName: &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		},
	} {
		t.Run(name, func(t *testing.T) {
			stateReport := workflow.EvaluateState(root, sddFeatureID)
			got := Evaluate(root, sddFeatureID)
			if stateReport.Outcome != workflow.StateInvalid || got.Outcome != StateInvalid {
				t.Fatalf("workflow=%+v sdd=%+v", stateReport, got)
			}
		})
	}
	for _, id := range []string{"..", "../x", "a/../../x", "/etc/passwd", "a/b"} {
		root := sddFixture(nil)
		stateReport := workflow.EvaluateState(root, id)
		got := Evaluate(root, id)
		if stateReport.Outcome != workflow.FeatureUnknown || got.Outcome != FeatureUnknown {
			t.Fatalf("id %q: workflow=%+v sdd=%+v", id, stateReport, got)
		}
	}
	if string(FeatureUnknown) != string(workflow.FeatureUnknown) || string(StateInvalid) != string(workflow.StateInvalid) {
		t.Fatal("shared outcome tokens drifted from workflow")
	}
}

func TestEvaluateDeterministic(t *testing.T) {
	record := `{"state":"planned"}`
	root := sddFixture(&record, SpecArtifact, PlanArtifact)
	first := Evaluate(root, sddFeatureID)
	second := Evaluate(root, sddFeatureID)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated Evaluate differs: %+v vs %+v", first, second)
	}
}

func TestEvaluateTableMiss(t *testing.T) {
	record := `{"state":"planned"}`
	lookup := &countingLookup{omit: workflow.Planned}
	got := evaluate(sddFixture(&record, SpecArtifact, PlanArtifact), sddFeatureID, lookup.lookup)
	want := Report{FeatureID: sddFeatureID}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("table miss = %+v, want %+v", got, want)
	}
	if got.Valid() || got.Outcome == ArtifactsInconsistent || got.Outcome == StateInvalid {
		t.Fatalf("table miss was classified as an outcome: %+v", got)
	}
}
