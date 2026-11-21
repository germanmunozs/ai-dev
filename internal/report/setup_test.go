package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func setupAgents(codexAvailable, chosen bool) []SetupAgent {
	codex := "unavailable"
	if codexAvailable {
		codex = "available"
	}
	return []SetupAgent{
		{ID: "claude-code", Label: "Claude Code", Availability: "available", Chosen: chosen},
		{ID: "codex-cli", Label: "OpenAI Codex CLI", Availability: codex, Chosen: chosen},
	}
}

func setupRoles(implementation string) []SetupRole {
	return []SetupRole{
		{Role: "specification", Agent: "claude-code"}, {Role: "planning", Agent: "claude-code"},
		{Role: "implementation", Agent: implementation}, {Role: "review", Agent: "claude-code"},
		{Role: "remediation", Agent: "codex-cli"},
	}
}

func renderSetup(t *testing.T, r SetupReport) (string, map[string]any) {
	t.Helper()
	var text, js bytes.Buffer
	if err := WriteSetupText(&text, r); err != nil {
		t.Fatal(err)
	}
	if err := WriteSetupJSON(&js, r); err != nil {
		t.Fatal(err)
	}
	if strings.Count(js.String(), "\n") != 1 {
		t.Fatalf("JSON is not one compact line: %q", js.String())
	}
	var object map[string]any
	if err := json.Unmarshal(js.Bytes(), &object); err != nil {
		t.Fatal(err)
	}
	return text.String(), object
}

// contracts/setup-cli.md: the exact text and JSON of each show outcome.
func TestSetupReportConfigured(t *testing.T) {
	text, object := renderSetup(t, SetupReport{Command: "show", Outcome: Configured, Source: "project", Agents: setupAgents(false, true), Roles: setupRoles("codex-cli")})
	want := `ai-dev: outcome=configured source=project
  agent claude-code availability=available chosen=yes
  agent codex-cli availability=unavailable chosen=yes warning=chosen_agent_unavailable
  role specification agent=claude-code
  role planning agent=claude-code
  role implementation agent=codex-cli
  role review agent=claude-code
  role remediation agent=codex-cli
`
	if text != want {
		t.Fatalf("text:\n%s\nwant:\n%s", text, want)
	}
	if object["schema_version"] != SetupSchemaVersion || object["command"] != "show" || object["outcome"] != "configured" || object["source"] != "project" {
		t.Fatalf("JSON header: %v", object)
	}
	if warnings := object["warnings"].([]any); len(warnings) != 1 || warnings[0] != "chosen_agent_unavailable:codex-cli" {
		t.Fatalf("warnings: %v", warnings)
	}
	if reasons := object["reasons"].([]any); len(reasons) != 0 {
		t.Fatalf("reasons: %v", reasons)
	}
	agent := object["agents"].([]any)[1].(map[string]any)
	if agent["id"] != "codex-cli" || agent["label"] != "OpenAI Codex CLI" || agent["availability"] != "unavailable" || agent["chosen"] != true {
		t.Fatalf("agent: %v", agent)
	}
	if roles := object["roles"].([]any); len(roles) != 5 || roles[2].(map[string]any)["agent"] != "codex-cli" {
		t.Fatalf("roles: %v", roles)
	}
}

func TestSetupReportNotConfigured(t *testing.T) {
	text, object := renderSetup(t, SetupReport{Command: "show", Outcome: NotConfigured, Source: "built_in", Agents: setupAgents(false, false), Roles: setupRoles("codex-cli")})
	if !strings.HasPrefix(text, "ai-dev: outcome=not_configured source=built_in\n  agent claude-code availability=available chosen=no\n  agent codex-cli availability=unavailable chosen=no\n  role specification") {
		t.Fatalf("text:\n%s", text)
	}
	if strings.Contains(text, "warning=") || len(object["warnings"].([]any)) != 0 {
		t.Fatal("an unchosen unavailable agent produced a warning")
	}
}

// configuration_invalid: no source, no roles, every reason in order, and
// untrusted values in the 004 shell-inert quoted form (F011 R-02).
func TestSetupReportConfigurationInvalid(t *testing.T) {
	text, object := renderSetup(t, SetupReport{Command: "show", Outcome: ConfigurationInvalid, Agents: setupAgents(true, false), Reasons: []SetupReason{
		{Code: "unsupported_schema", Value: "x"}, {Code: "unknown_agent", Value: "gem\nini $(id) `id` !!"}, {Code: "no_agents"},
	}})
	want := "ai-dev: outcome=configuration_invalid\n" +
		"  agent claude-code availability=available chosen=no\n" +
		"  agent codex-cli availability=available chosen=no\n" +
		"  reason unsupported_schema \"x\"\n" +
		"  reason unknown_agent \"gem\\nini \\$(id) \\`id\\` \\!\\!\"\n" +
		"  reason no_agents\n"
	if text != want {
		t.Fatalf("text:\n%s\nwant:\n%s", text, want)
	}
	if _, ok := object["source"]; ok {
		t.Fatal("configuration_invalid carries a source")
	}
	if roles := object["roles"].([]any); len(roles) != 0 {
		t.Fatalf("roles: %v", roles)
	}
	reasons := object["reasons"].([]any)
	if len(reasons) != 3 || reasons[1].(map[string]any)["value"] != "gem\nini $(id) `id` !!" {
		t.Fatalf("reasons: %v", reasons)
	}
	if _, ok := reasons[2].(map[string]any)["value"]; ok {
		t.Fatal("an empty reason value is rendered")
	}
}

// No report form ever contains an executable path (F011-FR-014): the report
// has no field for one, and labels are catalog labels.
func TestSetupReportRejectsImpossibleCombinations(t *testing.T) {
	for name, r := range map[string]SetupReport{
		"unknown command":           {Command: "run", Outcome: Configured, Source: "project", Agents: setupAgents(true, true), Roles: setupRoles("codex-cli")},
		"unknown outcome":           {Command: "show", Outcome: "approved", Agents: setupAgents(true, true)},
		"configured without source": {Command: "show", Outcome: Configured, Agents: setupAgents(true, true), Roles: setupRoles("codex-cli")},
		"invalid with roles":        {Command: "show", Outcome: ConfigurationInvalid, Agents: setupAgents(true, false), Roles: setupRoles("codex-cli"), Reasons: []SetupReason{{Code: "no_agents"}}},
		"invalid without reasons":   {Command: "show", Outcome: ConfigurationInvalid, Agents: setupAgents(true, false)},
		"no agents":                 {Command: "show", Outcome: NotConfigured, Source: "built_in", Roles: setupRoles("codex-cli")},
		"bad availability":          {Command: "show", Outcome: NotConfigured, Source: "built_in", Agents: []SetupAgent{{ID: "claude-code", Label: "Claude Code", Availability: "/usr/bin/claude"}}, Roles: setupRoles("codex-cli")},
		"configured nothing chosen": {Command: "show", Outcome: Configured, Source: "project", Agents: setupAgents(true, false), Roles: setupRoles("codex-cli")},
		"show with apply outcome":   {Command: "show", Outcome: ConfigurationWritten, Source: "project", Agents: setupAgents(true, true), Roles: setupRoles("codex-cli")},
	} {
		if err := WriteSetupText(&bytes.Buffer{}, r); err == nil {
			t.Errorf("%s: rendered", name)
		}
	}
}
