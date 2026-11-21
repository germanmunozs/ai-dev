package gatestore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

type recordMember struct {
	name string
	raw  json.RawMessage
}

type evidenceRecord struct {
	FeatureID string         `json:"feature_id"`
	Decision  gate.Decision  `json:"decision"`
	Approver  string         `json:"approver"`
	FromState workflow.State `json:"from_state"`
	ToState   workflow.State `json:"to_state"`
	DecidedAt string         `json:"decided_at"`
	Reason    *string        `json:"reason,omitempty"`
}

func buildRecord(existing []byte, exists bool, to workflow.State, entry gate.Evidence) ([]byte, error) {
	entryWire := evidenceRecord{
		FeatureID: entry.FeatureID, Decision: entry.Decision, Approver: entry.Approver,
		FromState: entry.FromState, ToState: entry.ToState,
		DecidedAt: entry.DecidedAt.UTC().Format(time.RFC3339),
	}
	if entry.ReasonSet {
		reason := entry.Reason
		entryWire.Reason = &reason
	}
	entryJSON, err := json.Marshal(entryWire)
	if err != nil {
		return nil, fmt.Errorf("encode evidence: %w", err)
	}
	stateJSON, err := json.Marshal(to)
	if err != nil {
		return nil, fmt.Errorf("encode state: %w", err)
	}

	members := []recordMember{{name: "state", raw: stateJSON}}
	if exists {
		members, err = decodeRecordMembers(existing)
		if err != nil {
			return nil, err
		}
	}

	foundHistory := false
	for i := range members {
		switch members[i].name {
		case "state":
			members[i].raw = stateJSON
		case "human_gates":
			if foundHistory {
				return nil, fmt.Errorf("duplicate human_gates")
			}
			foundHistory = true
			var history []json.RawMessage
			if err := json.Unmarshal(members[i].raw, &history); err != nil || history == nil && string(bytes.TrimSpace(members[i].raw)) != "[]" {
				return nil, fmt.Errorf("human_gates is not an array")
			}
			history = append(history, json.RawMessage(entryJSON))
			members[i].raw, err = json.Marshal(history)
			if err != nil {
				return nil, fmt.Errorf("encode history: %w", err)
			}
		default:
			if strings.EqualFold(members[i].name, "human_gates") {
				return nil, fmt.Errorf("ambiguous human_gates member %q", members[i].name)
			}
		}
	}
	if !foundHistory {
		members = append(members, recordMember{name: "human_gates", raw: json.RawMessage("[" + string(entryJSON) + "]")})
	}

	var compact bytes.Buffer
	compact.WriteByte('{')
	for i, member := range members {
		if i > 0 {
			compact.WriteByte(',')
		}
		name, _ := json.Marshal(member.name)
		compact.Write(name)
		compact.WriteByte(':')
		compact.Write(member.raw)
	}
	compact.WriteByte('}')
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	pretty.WriteByte('\n')
	return pretty.Bytes(), nil
}

func decodeRecordMembers(data []byte) ([]recordMember, error) {
	if !json.Valid(data) {
		return nil, fmt.Errorf("invalid record JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("record is not an object")
	}
	var members []recordMember
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, fmt.Errorf("invalid member name")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		members = append(members, recordMember{name: name, raw: append(json.RawMessage(nil), raw...)})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return members, nil
}
