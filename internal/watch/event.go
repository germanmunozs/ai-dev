// Package watch implements ai-dev watch (Feature 015): it observes the
// project and reports each change to a computed result as one event, in the
// ai-dev.watch/v1 contract. It is read-only: the results come from the
// commands' own computations through Source, and nothing here writes, locks
// or starts a process (F015-FR-019, F015-FR-020).
package watch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SchemaVersion identifies the event contract (F015-FR-002).
const SchemaVersion = "ai-dev.watch/v1"

// Type is an event type of contracts/watch.md.
type Type string

const (
	Snapshot             Type = "snapshot"
	FeatureAdded         Type = "feature_added"
	FeatureRemoved       Type = "feature_removed"
	WorkflowStateChanged Type = "workflow_state_changed"
	ArtifactsChanged     Type = "artifacts_changed"
	NextStepChanged      Type = "next_step_changed"
	AgentsConfigChanged  Type = "agents_config_changed"
	EnvironmentChanged   Type = "environment_changed"
	Heartbeat            Type = "heartbeat"
	ResyncRequired       Type = "resync_required"
	WatchStopped         Type = "watch_stopped"
)

// Event is one unit of output. Its seq is assigned when it is written
// (owner decision 2, research R-06).
type Event struct {
	Type    Type
	Time    time.Time
	Project string
	Data    any
}

// Artifact is the metadata of one artifact: never its content (F015-FR-021).
type Artifact struct {
	Name     string `json:"name"`
	Status   string `json:"status"` // present, absent, not_regular_file
	Size     *int64 `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

// FeatureData is one Observed Feature; State and Next are the bytes of
// "workflow state --format json" and "workflow next --format json".
type FeatureData struct {
	Feature   string          `json:"feature"`
	State     json.RawMessage `json:"state"`
	Artifacts []Artifact      `json:"artifacts"`
	Next      json.RawMessage `json:"next"`
}

type SnapshotData struct {
	Features    []FeatureData   `json:"features"`
	Setup       json.RawMessage `json:"setup"`
	Environment json.RawMessage `json:"environment"`
}

type FeatureRef struct {
	Feature string `json:"feature"`
}

// HumanGate is a recorded Human Gate history entry, as recorded
// (F015-FR-008).
type HumanGate struct {
	Decision  string  `json:"decision"`
	Approver  string  `json:"approver"`
	FromState string  `json:"from_state"`
	ToState   string  `json:"to_state"`
	DecidedAt string  `json:"decided_at"`
	Reason    *string `json:"reason,omitempty"`
}

// StateChange reports a new state with the outcome of "workflow state",
// never a Human Gate outcome; HumanGate is nil unless F015-FR-008 attaches
// the recorded evidence.
type StateChange struct {
	Feature   string          `json:"feature"`
	From      *string         `json:"from"`
	To        *string         `json:"to"`
	Outcome   string          `json:"outcome"`
	State     json.RawMessage `json:"state"`
	HumanGate *HumanGate      `json:"human_gate"`
}

type ArtifactsChange struct {
	Feature   string     `json:"feature"`
	Artifacts []Artifact `json:"artifacts"`
}

type NextChange struct {
	Feature string          `json:"feature"`
	Next    json.RawMessage `json:"next"`
}

// SetupChange carries the bytes of "setup show --format json".
type SetupChange struct {
	Setup json.RawMessage `json:"setup"`
}

type CheckChange struct {
	ID          string  `json:"id"`
	From        *string `json:"from"`
	To          *string `json:"to"`
	Remediation string  `json:"remediation"`
}

// EnvironmentChange carries the changed checks and the bytes of
// "doctor --format json".
type EnvironmentChange struct {
	Checks      []CheckChange   `json:"checks"`
	Environment json.RawMessage `json:"environment"`
}

type Resync struct {
	Reason string `json:"reason"` // queue_overflow, watch_overflow
}

type Stopped struct {
	Reason string `json:"reason"` // signal, project_gone, watch_unavailable, internal_error
	Detail string `json:"detail,omitempty"`
	Remedy string `json:"remedy,omitempty"`
}

type envelope struct {
	SchemaVersion string `json:"schema_version"`
	Seq           int64  `json:"seq"`
	Type          Type   `json:"type"`
	Time          string `json:"time"`
	Project       string `json:"project"`
	Data          any    `json:"data"`
}

const timeLayout = "2006-01-02T15:04:05.000Z07:00"

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

// EncodeJSON renders e as one NDJSON line (F015-FR-001). Embedded command
// objects are compacted onto the line but otherwise kept as they are; HTML
// characters are not escaped, so the commands' strings stay unchanged.
func EncodeJSON(e Event, seq int64) []byte {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(envelope{SchemaVersion: SchemaVersion, Seq: seq, Type: e.Type, Time: formatTime(e.Time), Project: e.Project, Data: e.Data})
	if err != nil {
		// Only an invalid embedded object can fail; report it rather than
		// emit a broken line.
		out.Reset()
		_ = encoder.Encode(envelope{SchemaVersion: SchemaVersion, Seq: seq, Type: WatchStopped, Time: formatTime(e.Time), Project: e.Project,
			Data: Stopped{Reason: "internal_error", Detail: fmt.Sprintf("event %s cannot be encoded: %v", e.Type, err)}})
	}
	return out.Bytes()
}

// EncodeText renders e as one readable line (F015-FR-004). Every string that
// comes from the project goes through quote, the shell-inert form of 004
// FR-028; values that ai-dev defines (types, states, outcomes) do not.
func EncodeText(e Event, seq int64, quote func(string) string) []byte {
	fields := []string{"ai-dev:", "event=" + string(e.Type), fmt.Sprintf("seq=%d", seq), "time=" + formatTime(e.Time)}
	add := func(key, value string) { fields = append(fields, key+"="+value) }
	switch d := e.Data.(type) {
	case SnapshotData:
		add("project", quote(e.Project))
		add("features", fmt.Sprint(len(d.Features)))
		add("setup", member(d.Setup, "outcome"))
		add("environment", member(d.Environment, "overall"))
	case FeatureData:
		add("feature", quote(d.Feature))
		add("state", stateSummary(d.State))
	case FeatureRef:
		add("feature", quote(d.Feature))
	case StateChange:
		add("feature", quote(d.Feature))
		add("from", orNone(d.From))
		add("to", orNone(d.To))
		add("outcome", d.Outcome)
		if d.HumanGate == nil {
			add("human_gate", "none")
			add("note", quote("no recorded human gate for this change"))
		} else {
			add("human_gate", d.HumanGate.Decision)
			add("approver", quote(d.HumanGate.Approver))
			if d.HumanGate.Reason != nil {
				add("reason", quote(*d.HumanGate.Reason))
			}
		}
	case ArtifactsChange:
		add("feature", quote(d.Feature))
		for _, a := range d.Artifacts {
			add(a.Name, a.Status)
		}
	case NextChange:
		add("feature", quote(d.Feature))
		add("outcome", member(d.Next, "outcome"))
		if activity := member(d.Next, "activity"); activity != "" {
			add("activity", activity)
			add("role", member(d.Next, "role"))
		}
	case SetupChange:
		add("outcome", member(d.Setup, "outcome"))
		if source := member(d.Setup, "source"); source != "" {
			add("source", source)
		}
	case EnvironmentChange:
		add("overall", member(d.Environment, "overall"))
		for _, c := range d.Checks {
			add(c.ID, orNone(c.From)+">"+orNone(c.To))
		}
	case Resync:
		add("reason", d.Reason)
	case Stopped:
		add("reason", d.Reason)
		if d.Detail != "" {
			add("detail", quote(d.Detail))
		}
		if d.Remedy != "" {
			add("remedy", quote(d.Remedy))
		}
	}
	return []byte(strings.Join(fields, " ") + "\n")
}

func orNone(value *string) string {
	if value == nil {
		return "none"
	}
	return *value
}

// member returns a top-level string member of a command's JSON object, or
// "" when it is absent. Values the commands define (outcomes, states,
// activities) never need quoting; anything else is replaced by "?".
func member(raw json.RawMessage, name string) string {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return ""
	}
	var value string
	if json.Unmarshal(object[name], &value) != nil {
		return ""
	}
	for _, r := range value {
		if !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return "?"
		}
	}
	return value
}

func stateSummary(raw json.RawMessage) string {
	if state := member(raw, "state"); state != "" {
		return state
	}
	return member(raw, "outcome")
}
