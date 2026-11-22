package projectconfig

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/agent"
)

const canonical = `{
  "schema_version": "ai-dev.agents/v1",
  "agents": [
    "claude-code",
    "codex-cli"
  ],
  "roles": {
    "specification": "claude-code",
    "planning": "claude-code",
    "implementation": "codex-cli",
    "review": "claude-code",
    "remediation": "codex-cli"
  }
}
`

func mapFS(content string) fstest.MapFS {
	return fstest.MapFS{FileName: &fstest.MapFile{Data: []byte(content), Mode: 0o644}}
}

func assertReasons(t *testing.T, root fs.FS, want ...Reason) {
	t.Helper()
	_, state, reasons := Load(root)
	if state != Invalid {
		t.Fatalf("state = %v, want Invalid (reasons %v)", state, reasons)
	}
	if !reflect.DeepEqual(reasons, want) {
		t.Fatalf("reasons = %#v, want %#v", reasons, want)
	}
}

// A missing directory or file is Absent, not Invalid (F011-FR-015).
func TestLoadAbsent(t *testing.T) {
	for name, root := range map[string]fs.FS{
		"no directory": fstest.MapFS{},
		"no file":      fstest.MapFS{Dir: &fstest.MapFile{Mode: fs.ModeDir | 0o755}},
		"real empty":   os.DirFS(t.TempDir()),
	} {
		if _, state, reasons := Load(root); state != Absent || len(reasons) != 0 {
			t.Errorf("%s: state=%v reasons=%v, want Absent", name, state, reasons)
		}
	}
}

func TestLoadValidCanonicalAndReformatted(t *testing.T) {
	for _, content := range []string{canonical, strings.Join(strings.Fields(canonical), "")} {
		config, state, reasons := Load(mapFS(content))
		if state != Valid || len(reasons) != 0 {
			t.Fatalf("state=%v reasons=%v", state, reasons)
		}
		if got := config.Agents(); !reflect.DeepEqual(got, []agent.ID{agent.ClaudeCode, agent.CodexCLI}) {
			t.Fatalf("agents = %v", got)
		}
		if agent, ok := config.AgentFor("implementation"); !ok || agent != "codex-cli" {
			t.Fatalf("implementation = %q %t", agent, ok)
		}
	}
}

// Each file-level rule of F011-FR-005 has its own reason (SC-004).
func TestLoadFileLevelReasons(t *testing.T) {
	t.Run("directory is a file", func(t *testing.T) {
		assertReasons(t, fstest.MapFS{Dir: &fstest.MapFile{Data: []byte("x")}}, Reason{Code: "directory_not_regular"})
	})
	t.Run("directory is a symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(t.TempDir(), "real")
		writeFile(t, filepath.Join(target, "agents.json"), canonical)
		if err := os.Symlink(target, filepath.Join(dir, Dir)); err != nil {
			t.Fatal(err)
		}
		assertReasons(t, os.DirFS(dir), Reason{Code: "directory_not_regular"})
	})
	t.Run("file is a symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(t.TempDir(), "agents.json")
		writeFile(t, target, canonical)
		mkdir(t, filepath.Join(dir, Dir))
		if err := os.Symlink(target, filepath.Join(dir, FileName)); err != nil {
			t.Fatal(err)
		}
		assertReasons(t, os.DirFS(dir), Reason{Code: "file_not_regular"})
	})
	t.Run("file is a directory", func(t *testing.T) {
		dir := t.TempDir()
		mkdir(t, filepath.Join(dir, FileName))
		assertReasons(t, os.DirFS(dir), Reason{Code: "file_not_regular"})
	})
	t.Run("file is a FIFO", func(t *testing.T) {
		dir := t.TempDir()
		mkdir(t, filepath.Join(dir, Dir))
		if err := syscall.Mkfifo(filepath.Join(dir, FileName), 0o644); err != nil {
			t.Skipf("mkfifo: %v", err)
		}
		assertReasons(t, os.DirFS(dir), Reason{Code: "file_not_regular"})
	})
	t.Run("filesystem cannot report symlinks", func(t *testing.T) {
		assertReasons(t, plainFS{mapFS(canonical)}, Reason{Code: "cannot_verify_symlinks"})
	})
	t.Run("too large", func(t *testing.T) {
		assertReasons(t, mapFS(strings.Repeat(" ", MaxSize+1)), Reason{Code: "too_large"})
	})
	t.Run("exactly the limit is read", func(t *testing.T) {
		content := canonical + strings.Repeat(" ", MaxSize-len(canonical))
		if _, state, reasons := Load(mapFS(content)); state != Valid {
			t.Fatalf("state=%v reasons=%v", state, reasons)
		}
	})
}

func TestLoadContentReasons(t *testing.T) {
	roles := `"roles":{"specification":"claude-code","planning":"claude-code","implementation":"codex-cli","review":"claude-code","remediation":"codex-cli"}`
	head := `"schema_version":"ai-dev.agents/v1","agents":["claude-code","codex-cli"]`
	for _, test := range []struct {
		name    string
		content string
		want    []Reason
	}{
		{"empty", ``, []Reason{{Code: "malformed_json"}}},
		{"array", `[]`, []Reason{{Code: "malformed_json"}}},
		{"truncated", `{"agents":`, []Reason{{Code: "malformed_json"}}},
		{"trailing object", `{` + head + `,` + roles + `}{}`, []Reason{{Code: "malformed_json"}}},
		{"duplicate top-level key", `{` + head + `,"agents":["claude-code"],` + roles + `}`, []Reason{
			{Code: "duplicate_key", Value: "agents"},
			// Content rules still apply to the decoded (last) value.
			{Code: "agent_not_chosen", Value: "implementation=codex-cli"}, {Code: "agent_not_chosen", Value: "remediation=codex-cli"},
		}},
		{"duplicate role key", `{` + head + `,"roles":{"specification":"claude-code","specification":"codex-cli","planning":"claude-code","implementation":"codex-cli","review":"claude-code","remediation":"codex-cli"}}`, []Reason{{Code: "duplicate_key", Value: "specification"}}},
		{"unknown field", `{` + head + `,` + roles + `,"command":"rm -rf /"}`, []Reason{{Code: "unknown_field", Value: "command"}}},
		{"missing fields", `{"schema_version":"ai-dev.agents/v1"}`, []Reason{{Code: "missing_field", Value: "agents"}, {Code: "missing_field", Value: "roles"}}},
		{"null agents", `{"schema_version":"ai-dev.agents/v1","agents":null,` + roles + `}`, []Reason{{Code: "wrong_type", Value: "agents"}}},
		{"number agent", `{"schema_version":"ai-dev.agents/v1","agents":["claude-code",7],` + roles + `}`, []Reason{{Code: "wrong_type", Value: "agents"}}},
		{"number schema", `{"schema_version":1,"agents":["claude-code","codex-cli"],` + roles + `}`, []Reason{{Code: "wrong_type", Value: "schema_version"}}},
		{"null role", `{` + head + `,"roles":{"specification":null,"planning":"claude-code","implementation":"codex-cli","review":"claude-code","remediation":"codex-cli"}}`, []Reason{{Code: "wrong_type", Value: "roles"}}},
		{"unsupported schema", `{"schema_version":"ai-dev.config/v2","agents":["claude-code","codex-cli"],` + roles + `}`, []Reason{{Code: "unsupported_schema", Value: "ai-dev.config/v2"}}},
		{"no agents", `{"schema_version":"ai-dev.agents/v1","agents":[],` + roles + `}`, []Reason{
			{Code: "no_agents"},
			{Code: "agent_not_chosen", Value: "specification=claude-code"}, {Code: "agent_not_chosen", Value: "planning=claude-code"},
			{Code: "agent_not_chosen", Value: "implementation=codex-cli"}, {Code: "agent_not_chosen", Value: "review=claude-code"},
			{Code: "agent_not_chosen", Value: "remediation=codex-cli"},
		}},
		{"duplicate agent", `{"schema_version":"ai-dev.agents/v1","agents":["claude-code","codex-cli","claude-code"],` + roles + `}`, []Reason{{Code: "duplicate_agent", Value: "claude-code"}}},
		{"unknown agents", `{"schema_version":"ai-dev.agents/v1","agents":["claude-code","codex-cli","gemini","Claude-Code"],` + roles + `}`, []Reason{{Code: "unknown_agent", Value: "gemini"}, {Code: "unknown_agent", Value: "Claude-Code"}}},
		{"unknown role", `{` + head + `,"roles":{"specification":"claude-code","planning":"claude-code","implementation":"codex-cli","review":"claude-code","remediation":"codex-cli","deploy":"codex-cli"}}`, []Reason{{Code: "unknown_role", Value: "deploy"}}},
		{"missing role", `{` + head + `,"roles":{"specification":"claude-code","planning":"claude-code","implementation":"codex-cli","review":"claude-code"}}`, []Reason{{Code: "missing_role", Value: "remediation"}}},
		{"agent not chosen", `{"schema_version":"ai-dev.agents/v1","agents":["claude-code"],` + roles + `}`, []Reason{{Code: "agent_not_chosen", Value: "implementation=codex-cli"}, {Code: "agent_not_chosen", Value: "remediation=codex-cli"}}},
		{"role names an unknown agent", `{` + head + `,"roles":{"specification":"gemini","planning":"claude-code","implementation":"codex-cli","review":"claude-code","remediation":"codex-cli"}}`, []Reason{{Code: "unknown_agent", Value: "gemini"}}},
		{"several violations in order", `{"schema_version":"x","agents":["gemini","gemini"],"roles":{"Review":"claude-code"}}`, []Reason{
			{Code: "unsupported_schema", Value: "x"}, {Code: "duplicate_agent", Value: "gemini"}, {Code: "unknown_agent", Value: "gemini"},
			{Code: "unknown_role", Value: "Review"},
			{Code: "missing_role", Value: "specification"}, {Code: "missing_role", Value: "planning"}, {Code: "missing_role", Value: "implementation"},
			{Code: "missing_role", Value: "review"}, {Code: "missing_role", Value: "remediation"},
		}},
	} {
		t.Run(test.name, func(t *testing.T) { assertReasons(t, mapFS(test.content), test.want...) })
	}
}

// Untrusted values are returned verbatim; the report layer quotes them.
func TestLoadReturnsUntrustedValuesVerbatim(t *testing.T) {
	hostile := "gem\nini $(id) `id` !!"
	content := `{"schema_version":"ai-dev.agents/v1","agents":["claude-code","gem\nini $(id) ` + "`id`" + ` !!"],"roles":{"specification":"claude-code","planning":"claude-code","implementation":"claude-code","review":"claude-code","remediation":"claude-code"}}`
	content = strings.Replace(content, "\n", `\n`, 1)
	assertReasons(t, mapFS(content), Reason{Code: "unknown_agent", Value: hostile})
}

type plainFS struct{ fs fs.FS }

func (p plainFS) Open(name string) (fs.File, error) { return p.fs.Open(name) }

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
