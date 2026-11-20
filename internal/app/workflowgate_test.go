package app

import (
	"bytes"
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

type fakeGateStore struct {
	status   gate.CommitStatus
	warnings []string
	calls    int
	feature  string
	expected workflow.State
	entry    gate.Evidence
}

func (s *fakeGateStore) Commit(feature string, expected workflow.State, entry gate.Evidence) (gate.CommitStatus, []string) {
	s.calls++
	s.feature = feature
	s.expected = expected
	s.entry = entry
	return s.status, s.warnings
}

func appGateFixture(state workflow.State, files ...string) fstest.MapFS {
	m := fstest.MapFS{"specs/f": {Mode: fs.ModeDir | 0o755}, "specs/f/workflow-state.json": {Data: []byte(`{"state":"` + string(state) + `"}`), Mode: 0o644}}
	for _, name := range files {
		m["specs/f/"+name] = &fstest.MapFile{Mode: 0o644}
	}
	return m
}

func TestWorkflowGateUsageFailures(t *testing.T) {
	for _, decision := range []gate.Decision{gate.Approval, gate.Abandonment} {
		verb := "approve"
		target := "specified"
		base := []string{"--approver", "a", "f", target}
		usage := "Usage: ai-dev workflow approve [--format text|json] --approver <identity> [--reason <text>] <feature-id> <target-state>\n"
		if decision == gate.Abandonment {
			verb, target = "abandon", "abandoned"
			base = []string{"--approver", "a", "--reason", "r", "f", target}
			usage = "Usage: ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>\n"
		}
		cases := []struct {
			name string
			args []string
		}{
			{"unknown flag", []string{"--unknown", "x", "--approver", "a", "f", target}},
			{"bad format", append([]string{"--format", "yaml"}, base...)},
			{"missing format value", []string{"--format"}},
			{"zero positional", base[:len(base)-2]},
			{"one positional", base[:len(base)-1]},
			{"three positional", append(append([]string{}, base...), "extra")},
			{"whitespace id", append(append([]string{}, base[:len(base)-2]...), " ", target)},
			{"whitespace target", append(append([]string{}, base[:len(base)-1]...), " ")},
			{"approver missing", func() []string {
				if decision == gate.Abandonment {
					return []string{"--reason", "r", "f", target}
				}
				return []string{"f", target}
			}()},
			{"approver repeated", append([]string{"--approver", "a"}, base...)},
			{"approver empty", append([]string{"--approver", ""}, base[2:]...)},
			{"approver whitespace", append([]string{"--approver", " "}, base[2:]...)},
			{"reason repeated", func() []string {
				if decision == gate.Abandonment {
					return []string{"--approver", "a", "--reason", "r", "--reason", "again", "f", target}
				}
				return []string{"--approver", "a", "--reason", "r", "--reason", "again", "f", target}
			}()},
			{"reason empty", []string{"--approver", "a", "--reason", "", "f", target}},
			{"reason whitespace", []string{"--approver", "a", "--reason", " ", "f", target}},
		}
		if decision == gate.Abandonment {
			cases = append(cases, struct {
				name string
				args []string
			}{"reason missing", []string{"--approver", "a", "f", target}})
		}
		for _, tc := range cases {
			t.Run(verb+"/"+tc.name, func(t *testing.T) {
				store := &fakeGateStore{status: gate.Committed}
				root := &recordingRootFS{inner: appGateFixture(workflow.Draft, "spec.md")}
				var out, errOut bytes.Buffer
				code := runWorkflowGate(decision, tc.args, root, store, time.Now, &out, &errOut)
				if code != 2 || out.Len() != 0 || errOut.String() != usage || store.calls != 0 || len(root.calls) != 0 {
					t.Fatalf("code=%d out=%q err=%q store=%d fs=%v", code, out.String(), errOut.String(), store.calls, root.calls)
				}
			})
		}
	}
}

func TestWorkflowGateC13(t *testing.T) {
	cases := []struct {
		decision gate.Decision
		target   string
		wantCode int
		want     string
		wantErr  string
		calls    int
	}{
		{gate.Abandonment, "abandoned", 0, "transition_applied", "", 1},
		{gate.Abandonment, "Abandoned", 2, "", "abandonment requires target \"abandoned\"\nUsage: ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>\n", 0},
		{gate.Abandonment, "planned", 2, "", "abandonment requires target \"abandoned\"\nUsage: ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>\n", 0},
		{gate.Abandonment, "reviewing", 2, "", "abandonment requires target \"abandoned\"\nUsage: ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>\n", 0},
		{gate.Approval, "abandoned", 2, "", "use \"ai-dev workflow abandon\" to abandon a feature\nUsage: ai-dev workflow approve [--format text|json] --approver <identity> [--reason <text>] <feature-id> <target-state>\n", 0},
		{gate.Approval, "Abandoned", 1, "target_unsupported", "", 0},
		{gate.Approval, "reviewing", 1, "target_unsupported", "", 0},
		{gate.Approval, "planned", 0, "transition_applied", "", 1},
	}
	for _, tc := range cases {
		t.Run(string(tc.decision)+"_"+tc.target, func(t *testing.T) {
			store := &fakeGateStore{status: gate.Committed}
			args := []string{"--approver", "a"}
			if tc.decision == gate.Abandonment {
				args = append(args, "--reason", "stop")
			}
			args = append(args, "f", tc.target)
			root := &recordingRootFS{inner: appGateFixture(workflow.Specified, "spec.md", "plan.md")}
			var out, errOut bytes.Buffer
			code := runWorkflowGate(tc.decision, args, root, store, func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }, &out, &errOut)
			combined := out.String() + errOut.String()
			if code != tc.wantCode || (tc.want != "" && !strings.Contains(combined, tc.want)) || errOut.String() != tc.wantErr || store.calls != tc.calls || (tc.wantCode == 2 && out.Len() != 0) {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
			}
			if (tc.wantCode == 2 || tc.want == "target_unsupported") && len(root.calls) != 0 {
				t.Fatalf("filesystem calls=%v", root.calls)
			}
		})
	}
}

func TestWorkflowGateOutcomesWarningsAndOutputFailure(t *testing.T) {
	for _, tc := range []struct {
		status gate.CommitStatus
		code   int
		token  string
	}{{gate.Committed, 0, "transition_applied"}, {gate.Conflict, 1, "state_conflict"}, {gate.Failed, 1, "persistence_failed"}} {
		store := &fakeGateStore{status: tc.status, warnings: []string{"warning"}}
		var out, errOut bytes.Buffer
		code := runWorkflowGate(gate.Approval, []string{"--approver", "alice", "f", "specified"}, appGateFixture(workflow.Draft, "spec.md"), store, time.Now, &out, &errOut)
		if code != tc.code || !strings.Contains(out.String(), tc.token) || errOut.String() != "warning\n" {
			t.Fatalf("status=%v code=%d out=%q err=%q", tc.status, code, out.String(), errOut.String())
		}
	}
	store := &fakeGateStore{status: gate.Committed}
	var errOut bytes.Buffer
	code := runWorkflowGate(gate.Approval, []string{"--approver", "a", "f", "specified"}, appGateFixture(workflow.Draft, "spec.md"), store, time.Now, errWriter{}, &errOut)
	if code != 2 || store.calls != 1 || !strings.Contains(errOut.String(), "write workflow gate report:") {
		t.Fatalf("code=%d calls=%d err=%q", code, store.calls, errOut.String())
	}
}

func TestWorkflowGateEnvironmentNeverSuppliesIdentity(t *testing.T) {
	for _, key := range []string{"USER", "LOGNAME", "GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME", "EMAIL"} {
		t.Setenv(key, "SECRET_SENTINEL")
	}
	for _, tc := range []struct {
		status gate.CommitStatus
		code   int
		want   string
	}{{gate.Committed, 0, "transition_applied"}, {gate.Conflict, 1, "state_conflict"}, {gate.Failed, 1, "persistence_failed"}} {
		store := &fakeGateStore{status: tc.status}
		var out, errOut bytes.Buffer
		code := runWorkflowGate(gate.Approval, []string{"--approver", "explicit", "f", "specified"}, appGateFixture(workflow.Draft, "spec.md"), store, func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) }, &out, &errOut)
		if code != tc.code || !strings.Contains(out.String(), tc.want) || errOut.Len() != 0 || store.calls != 1 {
			t.Fatalf("status=%v code=%d out=%q err=%q calls=%d", tc.status, code, out.String(), errOut.String(), store.calls)
		}
		if store.feature != "f" || store.expected != workflow.Draft || store.entry.Approver != "explicit" || strings.Contains(out.String()+errOut.String()+store.entry.Approver, "SECRET_SENTINEL") {
			t.Fatalf("feature=%q expected=%q entry=%+v", store.feature, store.expected, store.entry)
		}
	}
}

func TestWorkflowGateMissingApproverIgnoresEnvironmentIdentity(t *testing.T) {
	for _, key := range []string{"USER", "LOGNAME", "GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME", "EMAIL"} {
		t.Setenv(key, "SECRET_SENTINEL_"+key)
	}
	cases := []struct {
		name     string
		decision gate.Decision
		args     []string
		usage    string
	}{
		{
			name:     "approve",
			decision: gate.Approval,
			args:     []string{"f", "specified"},
			usage:    "Usage: ai-dev workflow approve [--format text|json] --approver <identity> [--reason <text>] <feature-id> <target-state>\n",
		},
		{
			name:     "abandon",
			decision: gate.Abandonment,
			args:     []string{"--reason", "stop", "f", "abandoned"},
			usage:    "Usage: ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := &recordingRootFS{inner: appGateFixture(workflow.Draft, "spec.md")}
			store := &fakeGateStore{status: gate.Committed}
			var out, errOut bytes.Buffer
			code := runWorkflowGate(tc.decision, tc.args, root, store, time.Now, &out, &errOut)
			combined := out.String() + errOut.String()
			for _, key := range []string{"USER", "LOGNAME", "GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME", "EMAIL"} {
				if strings.Contains(combined, "SECRET_SENTINEL_"+key) {
					t.Fatalf("environment identity appeared for %s: %q", key, combined)
				}
			}
			if code != 2 || out.Len() != 0 || errOut.String() != tc.usage || store.calls != 0 || len(root.calls) != 0 {
				t.Fatalf("code=%d out=%q err=%q store=%d fs=%v", code, out.String(), errOut.String(), store.calls, root.calls)
			}
		})
	}
}

func TestRunWorkflowGateNilStoreAndDispatcherUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	stdin := &workflowNextFailReader{t: t}
	root := &recordingRootFS{inner: appGateFixture(workflow.Draft, "spec.md")}
	code := RunWithGateStore(context.Background(), []string{"workflow", "approve", "--approver", "explicit", "f", "specified"}, &fakeExecutor{}, &appRunLauncher{}, root, nil, stdin, &out, &errOut)
	if code != 2 || out.Len() != 0 || errOut.String() != "ai-dev: internal error: human gate store unavailable\n" || stdin.called || len(root.calls) != 0 {
		t.Fatalf("code=%d out=%q err=%q stdin=%v fs=%v", code, out.String(), errOut.String(), stdin.called, root.calls)
	}
	out.Reset()
	errOut.Reset()
	code = RunWithGateStore(context.Background(), []string{"workflow", "bogus"}, &fakeExecutor{}, &appRunLauncher{}, appGateFixture(workflow.Draft), nil, stdin, &out, &errOut)
	if code != 2 || out.Len() != 0 || errOut.String() != "Usage: ai-dev workflow <state|transition> [options]\n" || stdin.called {
		t.Fatalf("code=%d out=%q err=%q stdin=%v", code, out.String(), errOut.String(), stdin.called)
	}
}

func TestWorkflowGateEveryNonPersistenceOutcomeExitsOne(t *testing.T) {
	cases := []struct {
		name, id, target, record, want string
		files                          []string
	}{
		{"target unsupported", "missing", "reviewing", "", "target_unsupported", nil},
		{"feature unknown", "missing", "specified", "", "feature_unknown", nil},
		{"state invalid", "f", "specified", `{"state":"bad"}`, "state_invalid", []string{"spec.md"}},
		{"transition invalid", "f", "planned", `{"state":"draft"}`, "transition_invalid", []string{"spec.md", "plan.md"}},
		{"artifacts missing", "f", "specified", `{"state":"draft"}`, "artifacts_missing", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := fstest.MapFS{}
			if tc.id == "f" {
				fixture["specs/f"] = &fstest.MapFile{Mode: fs.ModeDir | 0o755}
				fixture["specs/f/workflow-state.json"] = &fstest.MapFile{Data: []byte(tc.record), Mode: 0o644}
				for _, file := range tc.files {
					fixture["specs/f/"+file] = &fstest.MapFile{Mode: 0o644}
				}
			}
			store := &fakeGateStore{status: gate.Committed}
			var out, errOut bytes.Buffer
			code := runWorkflowGate(gate.Approval, []string{"--approver", "a", tc.id, tc.target}, fixture, store, time.Now, &out, &errOut)
			if code != 1 || !strings.Contains(out.String(), "outcome="+tc.want) || errOut.Len() != 0 || store.calls != 0 {
				t.Fatalf("code=%d out=%q err=%q calls=%d", code, out.String(), errOut.String(), store.calls)
			}
		})
	}
}

func TestWorkflowGateInvalidReportIsInternalError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWorkflowGateWithEvaluator(gate.Approval, []string{"--approver", "a", "f", "specified"}, appGateFixture(workflow.Draft, "spec.md"), &fakeGateStore{}, time.Now, &out, &errOut,
		func(fs.FS, gate.Request, gate.Store, func() time.Time) (gate.Report, []string) {
			return gate.Report{FeatureID: "f"}, nil
		})
	if code != 2 || out.Len() != 0 || errOut.String() != "ai-dev: internal error: workflow gate report cannot be rendered for feature \"f\"\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestWorkflowGateWarningsFollowReportAndDeterminism(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.FixedZone("offset", 3600)) }
	run := func() (int, string) {
		store := &fakeGateStore{status: gate.Committed, warnings: []string{"warning one", "warning two"}}
		var combined bytes.Buffer
		code := runWorkflowGate(gate.Approval, []string{"--approver", "a", "f", "specified"}, appGateFixture(workflow.Draft, "spec.md"), store, now, &combined, &combined)
		return code, combined.String()
	}
	firstCode, first := run()
	secondCode, second := run()
	if firstCode != 0 || secondCode != 0 || first != second {
		t.Fatalf("first=(%d,%q) second=(%d,%q)", firstCode, first, secondCode, second)
	}
	reportAt := strings.Index(first, "outcome=transition_applied")
	warningOneAt := strings.Index(first, "warning one")
	warningTwoAt := strings.Index(first, "warning two")
	if reportAt < 0 || warningOneAt < reportAt || warningTwoAt < warningOneAt {
		t.Fatalf("ordering=%q", first)
	}
}
