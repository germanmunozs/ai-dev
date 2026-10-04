package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"strings"
)

// RecordFileName is the Workflow State Record's file name inside a feature's
// own specs/<feature-id>/ directory (FR-027).
const RecordFileName = "workflow-state.json"

// validFeatureID reports whether the raw featureID names exactly one
// directory below specs/. This is validation, not sanitization: the value is
// never trimmed, cleaned, or rewritten, and it is checked before any
// path.Join.
func validFeatureID(featureID string) bool {
	return featureID != "." && fs.ValidPath(featureID) && !strings.Contains(featureID, "/")
}

// FeatureExists reports whether featureID resolves to a feature root
// recognizes as existing: a real, non-symlink specs/<featureID> directory
// (FR-008). The raw value is validated before path.Join.
func FeatureExists(root fs.FS, featureID string) bool {
	if !validFeatureID(featureID) {
		return false
	}
	lstatRoot, ok := root.(fs.ReadLinkFS)
	if !ok {
		return false // no safe no-follow inspection is available
	}
	info, err := lstatRoot.Lstat(path.Join("specs", featureID))
	return err == nil && info.IsDir() && info.Mode()&fs.ModeSymlink == 0
}

// ResolveRecord determines an existing feature's current Workflow State by
// reading its Workflow State Record (FR-005, FR-006, FR-007). Callers must
// have already confirmed FeatureExists. It never writes, creates, deletes,
// or renames anything.
func ResolveRecord(root fs.FS, featureID string) (state State, invalid bool) {
	if !validFeatureID(featureID) {
		return "", true // defensive: callers normally stop at FeatureExists
	}
	lstatRoot, ok := root.(fs.ReadLinkFS)
	if !ok {
		return "", true // do not fall back to link-following Stat behavior
	}
	recordPath := path.Join("specs", featureID, RecordFileName)
	info, err := lstatRoot.Lstat(recordPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Draft, false // FR-005: record absent -> initial state
	}
	if err != nil {
		return "", true // FR-007: I/O failure
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", true // FR-007: symlink/non-regular record is present but invalid
	}
	data, err := fs.ReadFile(root, recordPath)
	if err != nil {
		return "", true // FR-007: read failure
	}
	value, ok := stateField(data)
	if !ok {
		return "", true // FR-007: unparseable, or state field absent, duplicated, or not a string
	}
	matched, ok := Recognized(value) // FR-006/FR-007: exact, case-sensitive; rejects empty/whitespace/non-exact
	if !ok {
		return "", true
	}
	return matched, false
}

// stateField extracts the record's top-level "state" member. Unlike
// json.Unmarshal into a struct, the member name is matched exactly and
// case-sensitively and nothing is resolved last-key-wins: a duplicated
// "state" member, or any case variant of it ("State", "STATE", "ſtate"),
// makes the record invalid. Other members are ignored. It reports false when
// data is not a single valid JSON object or "state" is absent, duplicated,
// accompanied by a case variant, or not a string.
func stateField(data []byte) (string, bool) {
	if !json.Valid(data) {
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return "", false
	}
	var value *string
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return "", false
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return "", false
		}
		name, _ := token.(string)
		if name != "state" {
			if strings.EqualFold(name, "state") {
				return "", false // case variant of "state" ("State", "STATE", "ſtate"): ambiguous
			}
			continue
		}
		if value != nil {
			return "", false // duplicated "state" member
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return "", false
		}
		value = &s
	}
	if value == nil {
		return "", false
	}
	return *value, true
}
