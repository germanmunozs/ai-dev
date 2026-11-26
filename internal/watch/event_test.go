package watch

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/germanmunozs/ai-dev/internal/report"
)

var eventTime = time.Date(2026, 11, 26, 10, 2, 8, 120_456_789, time.FixedZone("x", 3600))

const testProject = "/Users/ana/dev/app"

func ptr[T any](v T) *T { return &v }

func sampleEvents() map[Type]Event {
	state := json.RawMessage(`{"feature_id":"004-login","state":"approved","terminal":true,"outcome":"state_reported"}`)
	next := json.RawMessage(`{"feature_id":"004-login","outcome":"workflow_terminal","current_state":"approved"}`)
	setup := json.RawMessage(`{"schema_version":"ai-dev.setup/v1","command":"show","outcome":"configured","source":"project","agents":[],"roles":[],"warnings":[],"reasons":[]}`)
	env := json.RawMessage(`{"overall":"ready","checks":[]}`)
	artifacts := []Artifact{
		{Name: "spec.md", Status: "present", Size: ptr(int64(12)), Modified: "2026-11-26T09:02:08.123456789Z"},
		{Name: "plan.md", Status: "absent"},
		{Name: "tasks.md", Status: "not_regular_file"},
	}
	feature := FeatureData{Feature: "004-login", State: state, Artifacts: artifacts, Next: next}
	mk := func(t Type, data any) Event { return Event{Type: t, Time: eventTime, Project: testProject, Data: data} }
	return map[Type]Event{
		Snapshot:             mk(Snapshot, SnapshotData{Features: []FeatureData{feature}, Setup: setup, Environment: env}),
		FeatureAdded:         mk(FeatureAdded, feature),
		FeatureRemoved:       mk(FeatureRemoved, FeatureRef{Feature: "004-login"}),
		WorkflowStateChanged: mk(WorkflowStateChanged, StateChange{Feature: "004-login", From: ptr("implemented"), To: ptr("approved"), Outcome: "state_reported", State: state, HumanGate: &HumanGate{Decision: "approval", Approver: "ana@example.com", FromState: "implemented", ToState: "approved", DecidedAt: "2026-11-25T14:32:07Z", Reason: ptr("Revisado")}}),
		ArtifactsChanged:     mk(ArtifactsChanged, ArtifactsChange{Feature: "004-login", Artifacts: artifacts}),
		NextStepChanged:      mk(NextStepChanged, NextChange{Feature: "004-login", Next: next}),
		AgentsConfigChanged:  mk(AgentsConfigChanged, SetupChange{Setup: setup}),
		EnvironmentChanged:   mk(EnvironmentChanged, EnvironmentChange{Checks: []CheckChange{{ID: "git-tool", From: ptr("pass"), To: ptr("fail"), Remediation: "Install Git."}}, Environment: env}),
		Heartbeat:            mk(Heartbeat, struct{}{}),
		ResyncRequired:       mk(ResyncRequired, Resync{Reason: "queue_overflow"}),
		WatchStopped:         mk(WatchStopped, Stopped{Reason: "watch_unavailable", Detail: "inotify watch limit reached (fs.inotify.max_user_watches)", Remedy: "sudo sysctl fs.inotify.max_user_watches=524288"}),
	}
}

func TestEncodeJSONEnvelope(t *testing.T) {
	events := sampleEvents()
	if len(events) != 11 {
		t.Fatalf("sample covers %d types, want 11", len(events))
	}
	for typ, event := range events {
		line := EncodeJSON(event, 7)
		if !bytes.HasSuffix(line, []byte("\n")) || bytes.Count(line, []byte("\n")) != 1 {
			t.Errorf("%s: not one line: %q", typ, line)
		}
		prefix := `{"schema_version":"ai-dev.watch/v1","seq":7,"type":"` + string(typ) + `","time":"2026-11-26T09:02:08.120Z","project":"/Users/ana/dev/app","data":`
		if !strings.HasPrefix(string(line), prefix) {
			t.Errorf("%s: envelope = %s\nwant prefix %s", typ, line, prefix)
		}
		if !json.Valid(line) {
			t.Errorf("%s: invalid JSON %s", typ, line)
		}
	}
}

func TestEncodeJSONData(t *testing.T) {
	events := sampleEvents()
	data := func(typ Type) string {
		line := string(EncodeJSON(events[typ], 1))
		i := strings.Index(line, `"data":`)
		return strings.TrimSuffix(line[i+len(`"data":`):], "}\n")
	}
	want := map[Type]string{
		Snapshot:             `{"features":[{"feature":"004-login","state":{"feature_id":"004-login","state":"approved","terminal":true,"outcome":"state_reported"},"artifacts":[{"name":"spec.md","status":"present","size":12,"modified":"2026-11-26T09:02:08.123456789Z"},{"name":"plan.md","status":"absent"},{"name":"tasks.md","status":"not_regular_file"}],"next":{"feature_id":"004-login","outcome":"workflow_terminal","current_state":"approved"}}],"setup":{"schema_version":"ai-dev.setup/v1","command":"show","outcome":"configured","source":"project","agents":[],"roles":[],"warnings":[],"reasons":[]},"environment":{"overall":"ready","checks":[]}}`,
		FeatureRemoved:       `{"feature":"004-login"}`,
		WorkflowStateChanged: `{"feature":"004-login","from":"implemented","to":"approved","outcome":"state_reported","state":{"feature_id":"004-login","state":"approved","terminal":true,"outcome":"state_reported"},"human_gate":{"decision":"approval","approver":"ana@example.com","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z","reason":"Revisado"}}`,
		NextStepChanged:      `{"feature":"004-login","next":{"feature_id":"004-login","outcome":"workflow_terminal","current_state":"approved"}}`,
		AgentsConfigChanged:  `{"setup":{"schema_version":"ai-dev.setup/v1","command":"show","outcome":"configured","source":"project","agents":[],"roles":[],"warnings":[],"reasons":[]}}`,
		EnvironmentChanged:   `{"checks":[{"id":"git-tool","from":"pass","to":"fail","remediation":"Install Git."}],"environment":{"overall":"ready","checks":[]}}`,
		Heartbeat:            `{}`,
		ResyncRequired:       `{"reason":"queue_overflow"}`,
		WatchStopped:         `{"reason":"watch_unavailable","detail":"inotify watch limit reached (fs.inotify.max_user_watches)","remedy":"sudo sysctl fs.inotify.max_user_watches=524288"}`,
	}
	for typ, w := range want {
		if got := data(typ); got != w {
			t.Errorf("%s data =\n%s\nwant\n%s", typ, got, w)
		}
	}
}

func TestEncodeJSONNullsAndNewlines(t *testing.T) {
	manual := Event{Type: WorkflowStateChanged, Time: eventTime, Project: testProject, Data: StateChange{
		Feature: "x\ny", Outcome: "state_invalid", State: json.RawMessage("{\"feature_id\":\"x\\ny\",\n\"outcome\":\"state_invalid\"}\n"),
	}}
	line := string(EncodeJSON(manual, 1))
	if !strings.Contains(line, `"from":null,"to":null`) || !strings.Contains(line, `"human_gate":null`) {
		t.Errorf("nulls missing: %s", line)
	}
	if strings.Count(line, "\n") != 1 {
		t.Errorf("embedded newline kept: %q", line)
	}
	noReason := Event{Type: WorkflowStateChanged, Time: eventTime, Project: testProject, Data: StateChange{
		Feature: "f", From: ptr("implemented"), To: ptr("approved"), Outcome: "state_reported", State: json.RawMessage(`{}`),
		HumanGate: &HumanGate{Decision: "approval", Approver: "a", FromState: "implemented", ToState: "approved", DecidedAt: "2026-11-25T14:32:07Z"},
	}}
	if got := string(EncodeJSON(noReason, 1)); strings.Contains(got, `"reason"`) {
		t.Errorf("reason present without a recorded reason: %s", got)
	}
	html := Event{Type: FeatureRemoved, Time: eventTime, Project: testProject, Data: FeatureRef{Feature: "<a&b>"}}
	if got := string(EncodeJSON(html, 1)); !strings.Contains(got, `"feature":"<a&b>"`) {
		t.Errorf("HTML characters escaped: %s", got)
	}
}

func TestEncodeText(t *testing.T) {
	events := sampleEvents()
	quote := report.ShellInertQuote
	want := map[Type]string{
		Snapshot:             `ai-dev: event=snapshot seq=1 time=2026-11-26T09:02:08.120Z project="/Users/ana/dev/app" features=1 setup=configured environment=ready`,
		FeatureAdded:         `ai-dev: event=feature_added seq=1 time=2026-11-26T09:02:08.120Z feature="004-login" state=approved`,
		FeatureRemoved:       `ai-dev: event=feature_removed seq=1 time=2026-11-26T09:02:08.120Z feature="004-login"`,
		WorkflowStateChanged: `ai-dev: event=workflow_state_changed seq=1 time=2026-11-26T09:02:08.120Z feature="004-login" from=implemented to=approved outcome=state_reported human_gate=approval approver="ana@example.com" reason="Revisado"`,
		ArtifactsChanged:     `ai-dev: event=artifacts_changed seq=1 time=2026-11-26T09:02:08.120Z feature="004-login" spec.md=present plan.md=absent tasks.md=not_regular_file`,
		NextStepChanged:      `ai-dev: event=next_step_changed seq=1 time=2026-11-26T09:02:08.120Z feature="004-login" outcome=workflow_terminal`,
		AgentsConfigChanged:  `ai-dev: event=agents_config_changed seq=1 time=2026-11-26T09:02:08.120Z outcome=configured source=project`,
		EnvironmentChanged:   `ai-dev: event=environment_changed seq=1 time=2026-11-26T09:02:08.120Z overall=ready git-tool=pass>fail`,
		Heartbeat:            `ai-dev: event=heartbeat seq=1 time=2026-11-26T09:02:08.120Z`,
		ResyncRequired:       `ai-dev: event=resync_required seq=1 time=2026-11-26T09:02:08.120Z reason=queue_overflow`,
		WatchStopped:         `ai-dev: event=watch_stopped seq=1 time=2026-11-26T09:02:08.120Z reason=watch_unavailable detail="inotify watch limit reached (fs.inotify.max_user_watches)" remedy="sudo sysctl fs.inotify.max_user_watches=524288"`,
	}
	for typ, w := range want {
		if got := string(EncodeText(events[typ], 1, quote)); got != w+"\n" {
			t.Errorf("%s text =\n%q\nwant\n%q", typ, got, w+"\n")
		}
	}
	none := Event{Type: WorkflowStateChanged, Time: eventTime, Project: testProject, Data: StateChange{
		Feature: "005-x", From: ptr("planned"), To: ptr("tasked"), Outcome: "state_reported", State: json.RawMessage(`{}`)}}
	if got, w := string(EncodeText(none, 5, quote)), `ai-dev: event=workflow_state_changed seq=5 time=2026-11-26T09:02:08.120Z feature="005-x" from=planned to=tasked outcome=state_reported human_gate=none note="no recorded human gate for this change"`+"\n"; got != w {
		t.Errorf("no evidence text =\n%q\nwant\n%q", got, w)
	}
	invalid := Event{Type: WorkflowStateChanged, Time: eventTime, Project: testProject, Data: StateChange{
		Feature: "f", From: ptr("planned"), Outcome: "state_invalid", State: json.RawMessage(`{}`)}}
	if got := string(EncodeText(invalid, 1, quote)); !strings.Contains(got, " from=planned to=none outcome=state_invalid ") {
		t.Errorf("invalid state text = %q", got)
	}
}

func TestEncodeTextQuotesProjectStrings(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quote-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Name, Input, Want string }
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		event := Event{Type: WorkflowStateChanged, Time: eventTime, Project: testProject, Data: StateChange{
			Feature: v.Input, From: ptr("implemented"), To: ptr("approved"), Outcome: "state_reported", State: json.RawMessage(`{}`),
			HumanGate: &HumanGate{Decision: "approval", Approver: v.Input, FromState: "implemented", ToState: "approved", DecidedAt: "2026-11-25T14:32:07Z"},
		}}
		got := string(EncodeText(event, 1, report.ShellInertQuote))
		if !strings.Contains(got, " feature="+v.Want+" ") || !strings.Contains(got, " approver="+v.Want+"\n") {
			t.Errorf("%s: text %q does not carry %s", v.Name, got, v.Want)
		}
		if strings.Count(got, "\n") != 1 {
			t.Errorf("%s: text is not one line: %q", v.Name, got)
		}
	}
}
