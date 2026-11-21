package projectconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"slices"
)

// Load reads the project configuration from root, the project directory
// (F011-FR-004, FR-005). It follows no symbolic link: root must report
// symbolic links (fs.ReadLinkFS), or an existing configuration is Invalid.
func Load(root fs.FS) (Config, State, []Reason) {
	invalid := func(code string) (Config, State, []Reason) {
		return Config{}, Invalid, []Reason{{Code: code}}
	}
	links, ok := root.(fs.ReadLinkFS)
	if !ok {
		if _, err := fs.Stat(root, Dir); errors.Is(err, fs.ErrNotExist) {
			return Config{}, Absent, nil
		}
		return invalid("cannot_verify_symlinks")
	}
	info, err := links.Lstat(Dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Config{}, Absent, nil
	case err != nil:
		return invalid("unreadable")
	case !info.IsDir() || info.Mode()&fs.ModeSymlink != 0:
		return invalid("directory_not_regular")
	}
	info, err = links.Lstat(FileName)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Config{}, Absent, nil
	case err != nil:
		return invalid("unreadable")
	case !info.Mode().IsRegular():
		return invalid("file_not_regular")
	case info.Size() > MaxSize:
		return invalid("too_large")
	}
	file, err := root.Open(FileName)
	if err != nil {
		return invalid("unreadable")
	}
	defer file.Close()
	// The file can change between Lstat and Open; the read limit still
	// bounds what is decoded, and the content is validated either way.
	data, err := io.ReadAll(io.LimitReader(file, MaxSize+1))
	if err != nil {
		return invalid("unreadable")
	}
	if len(data) > MaxSize {
		return invalid("too_large")
	}
	config, reasons := Parse(data)
	if len(reasons) != 0 {
		return Config{}, Invalid, reasons
	}
	return config, Valid, nil
}

var topLevelFields = []string{"schema_version", "agents", "roles"}

// Parse validates the file content strictly: one JSON object, no trailing
// data, no duplicate key, exactly the known fields and types, then the
// content rules of F011-FR-005.
func Parse(data []byte) (Config, []Reason) {
	duplicates, ok := scanObject(data)
	if !ok {
		return Config{}, []Reason{{Code: "malformed_json"}}
	}
	var reasons []Reason
	for _, key := range duplicates {
		reasons = append(reasons, Reason{Code: "duplicate_key", Value: key})
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return Config{}, []Reason{{Code: "malformed_json"}}
	}
	var unknown []string
	for name := range fields {
		if !slices.Contains(topLevelFields, name) {
			unknown = append(unknown, name)
		}
	}
	slices.Sort(unknown)
	for _, name := range unknown {
		reasons = append(reasons, Reason{Code: "unknown_field", Value: name})
	}
	present := true
	for _, name := range topLevelFields {
		if _, ok := fields[name]; !ok {
			reasons = append(reasons, Reason{Code: "missing_field", Value: name})
			present = false
		}
	}
	if !present {
		return Config{}, reasons
	}
	var schema string
	var agents []string
	var roles map[string]string
	typed := true
	if !decodeString(fields["schema_version"], &schema) {
		reasons = append(reasons, Reason{Code: "wrong_type", Value: "schema_version"})
		typed = false
	}
	if !decodeStrings(fields["agents"], &agents) {
		reasons = append(reasons, Reason{Code: "wrong_type", Value: "agents"})
		typed = false
	}
	if !decodeStringMap(fields["roles"], &roles) {
		reasons = append(reasons, Reason{Code: "wrong_type", Value: "roles"})
		typed = false
	}
	if !typed {
		return Config{}, reasons
	}
	if schema != SchemaVersion {
		reasons = append(reasons, Reason{Code: "unsupported_schema", Value: schema})
	}
	return validate(agents, roles, reasons)
}

func isNull(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }

func decodeString(raw json.RawMessage, out *string) bool {
	return !isNull(raw) && json.Unmarshal(raw, out) == nil
}

func decodeStrings(raw json.RawMessage, out *[]string) bool {
	var items []json.RawMessage
	if isNull(raw) || json.Unmarshal(raw, &items) != nil {
		return false
	}
	*out = make([]string, len(items))
	for i, item := range items {
		if !decodeString(item, &(*out)[i]) {
			return false
		}
	}
	return true
}

func decodeStringMap(raw json.RawMessage, out *map[string]string) bool {
	var items map[string]json.RawMessage
	if isNull(raw) || json.Unmarshal(raw, &items) != nil {
		return false
	}
	*out = make(map[string]string, len(items))
	for key, item := range items {
		var value string
		if !decodeString(item, &value) {
			return false
		}
		(*out)[key] = value
	}
	return true
}

// scanObject checks that data is exactly one JSON object followed only by
// whitespace, and returns every key repeated within one object, in order of
// appearance. encoding/json would keep the last duplicate silently.
func scanObject(data []byte) ([]string, bool) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, false
	}
	var duplicates []string
	if !scanObjectBody(decoder, &duplicates) {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return duplicates, true
}

// scanObjectBody consumes an object after its opening brace.
func scanObjectBody(decoder *json.Decoder, duplicates *[]string) bool {
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, isKey := token.(string)
		if err != nil || !isKey {
			return false
		}
		if seen[key] {
			*duplicates = append(*duplicates, key)
		}
		seen[key] = true
		if !scanValue(decoder, duplicates) {
			return false
		}
	}
	token, err := decoder.Token()
	return err == nil && token == json.Delim('}')
}

func scanValue(decoder *json.Decoder, duplicates *[]string) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	switch token {
	case json.Delim('{'):
		return scanObjectBody(decoder, duplicates)
	case json.Delim('['):
		for decoder.More() {
			if !scanValue(decoder, duplicates) {
				return false
			}
		}
		token, err := decoder.Token()
		return err == nil && token == json.Delim(']')
	default:
		return true
	}
}
