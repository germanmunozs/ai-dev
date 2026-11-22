package app

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/projectconfig"
	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/role"
)

// fakeConfigStore records writes; fail makes every write fail.
type fakeConfigStore struct {
	writes [][]byte
	fail   bool
}

func (s *fakeConfigStore) Write(data []byte) (bool, []string) {
	if s.fail {
		return false, []string{"ai-dev: warning: injected write failure"}
	}
	s.writes = append(s.writes, append([]byte(nil), data...))
	return true, nil
}

func testConfig(t *testing.T, implementation agent.ID) projectconfig.Config {
	t.Helper()
	config, reasons := projectconfig.New([]agent.ID{agent.ClaudeCode, agent.CodexCLI}, map[role.Role]agent.ID{
		role.Specification: agent.ClaudeCode, role.Planning: agent.ClaudeCode, role.Implementation: implementation,
		role.Review: agent.ClaudeCode, role.Remediation: agent.CodexCLI,
	})
	if len(reasons) != 0 {
		t.Fatal(reasons)
	}
	return config
}

func configFS(content string) fstest.MapFS {
	return fstest.MapFS{projectconfig.FileName: &fstest.MapFile{Data: []byte(content), Mode: 0o644}}
}

// decideWrite: unchanged compares content, exists needs replace, and a
// store failure is persistence_failed (F011-FR-011, contracts/setup-cli.md).
func TestDecideWrite(t *testing.T) {
	requested := testConfig(t, agent.CodexCLI)
	other := testConfig(t, agent.ClaudeCode)
	reformatted := strings.Join(strings.Fields(string(requested.Encode())), "")
	for _, test := range []struct {
		name    string
		root    fstest.MapFS
		replace bool
		fail    bool
		want    report.SetupOutcome
		written bool
	}{
		{"absent", fstest.MapFS{}, false, false, report.ConfigurationWritten, true},
		{"equal canonical", configFS(string(requested.Encode())), false, false, report.ConfigurationUnchanged, false},
		{"equal reformatted", configFS(reformatted), false, false, report.ConfigurationUnchanged, false},
		{"equal with replace", configFS(reformatted), true, false, report.ConfigurationUnchanged, false},
		{"different", configFS(string(other.Encode())), false, false, report.ConfigurationExists, false},
		{"different with replace", configFS(string(other.Encode())), true, false, report.ConfigurationWritten, true},
		{"invalid", configFS("{}"), false, false, report.ConfigurationExists, false},
		{"invalid with replace", configFS("{}"), true, false, report.ConfigurationWritten, true},
		{"store failure", fstest.MapFS{}, false, true, report.PersistenceFailed, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeConfigStore{fail: test.fail}
			got, _ := decideWrite(test.root, store, requested, test.replace)
			if got != test.want {
				t.Fatalf("outcome = %s, want %s", got, test.want)
			}
			if test.written != (len(store.writes) == 1) || (test.written && string(store.writes[0]) != string(requested.Encode())) {
				t.Fatalf("writes = %q", store.writes)
			}
		})
	}
	if got, warnings := decideWrite(fstest.MapFS{}, nil, requested, false); got != report.PersistenceFailed || len(warnings) == 0 {
		t.Fatalf("nil store: %s %v", got, warnings)
	}
}
