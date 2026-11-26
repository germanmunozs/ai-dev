package gate

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/germanmunozs/ai-dev/internal/workflow"
)

// historyMember is the record member that holds the Human Gate history.
const historyMember = "human_gates"

// historyFields are the members of one history entry, as
// internal/gatestore/record.go writes them; reason is optional.
var historyFields = []string{"feature_id", "decision", "approver", "from_state", "to_state", "decided_at", "reason"}

// ReadHistory reads, read-only, the Human Gate history of featureID's
// Workflow State Record, so that reports can present each decision as
// recorded (006 Amendment A3, F015-FR-008). A missing record, or one without
// a history, has an empty history. It reports false for a structurally
// invalid feature identifier, a record that is a symbolic link or not a
// regular file, and any record or entry that does not match the format the
// Human Gate store writes. It never writes, creates, deletes or renames
// anything.
func ReadHistory(root fs.FS, featureID string) ([]Evidence, bool) {
	dir, ok := workflow.FeatureDir(featureID)
	if !ok {
		return nil, false
	}
	lstatRoot, ok := root.(fs.ReadLinkFS)
	if !ok {
		return nil, false // no safe no-follow inspection is available
	}
	recordPath := path.Join(dir, workflow.RecordFileName)
	info, err := lstatRoot.Lstat(recordPath)
	if errors.Is(err, fs.ErrNotExist) {
		return []Evidence{}, true
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	data, err := fs.ReadFile(root, recordPath)
	if err != nil {
		return nil, false
	}
	members, ok := objectMembers(data, []string{historyMember})
	if !ok {
		return nil, false
	}
	raw, present := members[historyMember]
	if !present {
		return []Evidence{}, true
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil || items == nil {
		return nil, false // not an array, or null
	}
	entries := make([]Evidence, 0, len(items))
	for _, item := range items {
		entry, ok := historyEntry(item)
		if !ok {
			return nil, false
		}
		entries = append(entries, entry)
	}
	return entries, true
}

// historyEntry decodes one entry with every required member present and
// valid, as Feature 006 records it.
func historyEntry(data []byte) (Evidence, bool) {
	members, ok := objectMembers(data, historyFields)
	if !ok {
		return Evidence{}, false
	}
	text := func(name string) (string, bool) {
		raw, present := members[name]
		if !present {
			return "", false
		}
		var value string
		return value, json.Unmarshal(raw, &value) == nil
	}
	var entry Evidence
	var fields [6]string
	for i, name := range historyFields[:6] {
		value, ok := text(name)
		if !ok {
			return Evidence{}, false
		}
		fields[i] = value
	}
	entry.FeatureID, entry.Approver = fields[0], fields[2]
	switch Decision(fields[1]) {
	case Approval, Abandonment:
		entry.Decision = Decision(fields[1])
	default:
		return Evidence{}, false
	}
	if entry.FromState, ok = workflow.Recognized(fields[3]); !ok {
		return Evidence{}, false
	}
	if entry.ToState, ok = workflow.Recognized(fields[4]); !ok {
		return Evidence{}, false
	}
	decidedAt, err := time.Parse(time.RFC3339, fields[5])
	if err != nil {
		return Evidence{}, false
	}
	entry.DecidedAt = decidedAt.UTC()
	if _, present := members["reason"]; present {
		if entry.Reason, ok = text("reason"); !ok {
			return Evidence{}, false
		}
		entry.ReasonSet = true
	}
	return entry, true
}

// objectMembers decodes data as one JSON object. Member names are matched
// exactly: a duplicated member, or a case variant of one of the known names,
// makes the object invalid, as for the record's "state" member
// (workflow.ResolveRecord). Unknown members are kept and ignored by callers.
func objectMembers(data []byte, known []string) (map[string]json.RawMessage, bool) {
	if !json.Valid(data) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, false
	}
	members := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		name, _ := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, false
		}
		if _, duplicated := members[name]; duplicated {
			return nil, false
		}
		for _, k := range known {
			if name != k && strings.EqualFold(name, k) {
				return nil, false
			}
		}
		members[name] = raw
	}
	return members, true
}
