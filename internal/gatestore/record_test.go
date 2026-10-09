package gatestore

import (
	"bytes"
	"encoding/json"
	"testing"
	"testing/fstest"
	"time"

	"ai-dev/internal/gate"
	"ai-dev/internal/workflow"
)

func testEvidence(from, to workflow.State, reason string, reasonSet bool) gate.Evidence {
	return gate.Evidence{FeatureID: "f", Decision: gate.Approval, Approver: "a\n\"λ", FromState: from, ToState: to, DecidedAt: time.Date(2026, 10, 7, 12, 34, 56, 999, time.FixedZone("x", 3600)), Reason: reason, ReasonSet: reasonSet}
}

func TestBuildRecordRules(t *testing.T) {
	entry := testEvidence(workflow.Draft, workflow.Specified, "", false)
	cases := []struct {
		name     string
		existing string
		exists   bool
		wantErr  bool
	}{
		{"absent", "", false, false},
		{"no history", `{"note":{"state":"nested"},"state":"draft","x":1}`, true, false},
		{"append", `{"state":"draft","human_gates":[{"old":true}],"x":1}`, true, false},
		{"object", `{"state":"draft","human_gates":{}}`, true, true},
		{"string", `{"state":"draft","human_gates":"x"}`, true, true},
		{"number", `{"state":"draft","human_gates":1}`, true, true},
		{"null", `{"state":"draft","human_gates":null}`, true, true},
		{"duplicate", `{"state":"draft","human_gates":[],"human_gates":[]}`, true, true},
		{"case variant", `{"state":"draft","Human_Gates":[]}`, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := buildRecord([]byte(tc.existing), tc.exists, workflow.Specified, entry)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v out=%s", err, out)
			}
			if tc.wantErr {
				return
			}
			if len(out) == 0 || out[len(out)-1] != '\n' || !json.Valid(out) {
				t.Fatalf("bad output %q", out)
			}
			fixture := fstest.MapFS{"specs/f": {Mode: 0o755 | 1<<31}, "specs/f/workflow-state.json": {Data: out, Mode: 0o644}}
			state, invalid := workflow.ResolveRecord(fixture, "f")
			if invalid || state != workflow.Specified {
				t.Fatalf("state=%s invalid=%v output=%s", state, invalid, out)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(out, &object); err != nil {
				t.Fatal(err)
			}
			var history []json.RawMessage
			if err := json.Unmarshal(object["human_gates"], &history); err != nil {
				t.Fatal(err)
			}
			if tc.name == "append" {
				if len(history) != 2 || !bytes.Equal(compactJSON(history[0]), compactJSON([]byte(`{"old":true}`))) {
					t.Fatalf("history=%s", object["human_gates"])
				}
			} else if len(history) != 1 {
				t.Fatalf("history len=%d", len(history))
			}
			var encoded map[string]any
			if err := json.Unmarshal(history[len(history)-1], &encoded); err != nil {
				t.Fatal(err)
			}
			if len(encoded) != 6 || encoded["decided_at"] != "2026-10-07T11:34:56Z" || encoded["approver"] != "a\n\"λ" {
				t.Fatalf("entry=%v", encoded)
			}
			if tc.name == "no history" {
				if _, ok := object["note"]; !ok {
					t.Fatal("unknown member lost")
				}
				if _, ok := object["x"]; !ok {
					t.Fatal("unknown member lost")
				}
			}
		})
	}
}

func TestBuildRecordReasonAndAppendSequence(t *testing.T) {
	var data []byte
	exists := false
	states := []workflow.State{workflow.Draft, workflow.Specified, workflow.Planned, workflow.Tasked, workflow.Implemented, workflow.Approved}
	for i := 1; i < len(states); i++ {
		entry := testEvidence(states[i-1], states[i], "ok", true)
		out, err := buildRecord(data, exists, states[i], entry)
		if err != nil {
			t.Fatal(err)
		}
		data, exists = out, true
		var object map[string]json.RawMessage
		json.Unmarshal(data, &object)
		var history []map[string]any
		json.Unmarshal(object["human_gates"], &history)
		if len(history) != i || history[i-1]["reason"] != "ok" || len(history[i-1]) != 7 {
			t.Fatalf("history=%v", history)
		}
	}
}

func compactJSON(data []byte) []byte {
	var out bytes.Buffer
	_ = json.Compact(&out, data)
	return out.Bytes()
}
