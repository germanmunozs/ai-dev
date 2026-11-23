package adapters

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/execution"
)

// Exact vectors of contracts/catalog.md, with I as the instruction. The first
// two are the 002 vectors and must never change.
var wantVectors = map[agent.ID][]string{
	agent.ClaudeCode:   {"claude", "--print", "--", "I"},
	agent.CodexCLI:     {"codex", "exec", "--", "I"},
	agent.FactoryDroid: {"droid", "exec", "--", "I"},
	agent.GeminiCLI:    {"gemini", "--prompt=I"},
	agent.KiroCLI:      {"kiro-cli", "chat", "--no-interactive", "--", "I"},
	agent.MistralVibe:  {"vibe", "--prompt=I"},
	agent.OpenCode:     {"opencode", "run", "--", "I"},
	agent.QwenCode:     {"qwen", "--prompt=I"},
}

// Instructions that look like options, subcommands or separators (SC-003).
var hostileInstructions = []string{
	"-x a b", "--help", "-h", "--version", "-p", "--yolo",
	"--dangerously-skip-permissions", "--dangerously-bypass-approvals-and-sandbox",
	"--sandbox=danger-full-access", "-c", "--", "-", "a=b", "--prompt=other",
}

// allowedModeTokens are the only mode selectors any row may use, and
// allowedOptions the only prompt options (research R-07: "only the mode
// selectors in the table may appear"). A new row with a new selector must
// add it here, in review, with its evidence.
var (
	allowedModeTokens = []string{"--print", "exec", "chat", "--no-interactive", "run"}
	allowedOptions    = []string{"--prompt"}
)

// relaxingFragments are argument fragments that would relax approvals,
// permissions, trust or sandboxing, or grant tools or directories (research
// R-07, F013-FR-007). They back up the allowlist for a reviewer's eye.
var relaxingFragments = []string{
	"yolo", "danger", "skip-permission", "bypass", "trust", "approve", "approval",
	"allow", "auto", "force", "full-auto", "accept-license", "sandbox", "permission",
	"unsafe", "yes", "add-dir", "include-dir", "settings",
}

// selectorFragments are options that pick a model, format, output, profile,
// configuration or file; the invocation carries only the mode selector.
var selectorFragments = []string{
	"model", "format", "output", "json", "profile", "config", "file", "mcp", "agent",
	"worktree", "remote", "enable", "background", "--bg",
}

// catalogDrift lists every mismatch between the catalog and the invocation
// table: missing or extra rows, and executables that differ.
func catalogDrift(catalog []agent.Agent, invocations map[agent.ID]Invocation) []string {
	var problems []string
	known := map[agent.ID]bool{}
	for _, a := range catalog {
		known[a.ID] = true
		invocation, ok := invocations[a.ID]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s has no invocation", a.ID))
			continue
		}
		if invocation.Executable != a.Executable() {
			problems = append(problems, fmt.Sprintf("%s runs %q, the catalog looks up %q", a.ID, invocation.Executable, a.Executable()))
		}
	}
	for id := range invocations {
		if !known[id] {
			problems = append(problems, fmt.Sprintf("invocation %s is not in the catalog", id))
		}
	}
	return problems
}

// unsafeArguments lists every argument, other than the instruction, that
// would relax the agent's protections or select more than its mode, and any
// shape problem of the instruction form.
func unsafeArguments(id agent.ID, invocation Invocation) []string {
	var problems []string
	switch invocation.Form {
	case AttachedOption:
		if !strings.HasPrefix(invocation.Option, "--") || len(invocation.Option) < 3 {
			problems = append(problems, fmt.Sprintf("%s: attached option %q does not start with --", id, invocation.Option))
		}
	case AfterEndOfOptions:
		if invocation.Option != "" {
			problems = append(problems, fmt.Sprintf("%s: option %q set for the end-of-options form", id, invocation.Option))
		}
	default:
		problems = append(problems, fmt.Sprintf("%s: unknown form %d", id, invocation.Form))
	}
	if invocation.Form == AttachedOption && !slices.Contains(allowedOptions, invocation.Option) {
		problems = append(problems, fmt.Sprintf("%s: prompt option %q is not in allowedOptions", id, invocation.Option))
	}
	for _, arg := range invocation.Mode {
		if !slices.Contains(allowedModeTokens, arg) {
			problems = append(problems, fmt.Sprintf("%s: mode token %q is not in allowedModeTokens", id, arg))
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			problems = append(problems, fmt.Sprintf("%s: mode token %q is a short option", id, arg))
		}
		if strings.Contains(arg, "=") {
			problems = append(problems, fmt.Sprintf("%s: mode token %q carries a value", id, arg))
		}
	}
	for _, arg := range append(slices.Clone(invocation.Mode), invocation.Option) {
		lower := strings.ToLower(arg)
		for _, fragment := range append(slices.Clone(relaxingFragments), selectorFragments...) {
			if strings.Contains(lower, fragment) {
				problems = append(problems, fmt.Sprintf("%s: argument %q contains %q", id, arg, fragment))
			}
		}
		if arg == "--" {
			problems = append(problems, fmt.Sprintf("%s: mode contains an end-of-options marker", id))
		}
	}
	return problems
}

func TestInvocationsMatchTheCatalog(t *testing.T) {
	if problems := catalogDrift(agent.Catalog(), Invocations()); len(problems) != 0 {
		t.Fatalf("catalog drift:\n%s", strings.Join(problems, "\n"))
	}
}

func TestInvocationsAreTheContractVectors(t *testing.T) {
	if len(wantVectors) != len(agent.Catalog()) {
		t.Fatalf("wantVectors has %d rows, the catalog %d", len(wantVectors), len(agent.Catalog()))
	}
	for id, want := range wantVectors {
		invocation, ok := Invocations()[id]
		if !ok {
			t.Errorf("%s has no invocation", id)
			continue
		}
		name, args := invocation.Command("/tmp/project with spaces", "I")
		if got := append([]string{name}, args...); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", id, got, want)
		}
	}
}

// An instruction that looks like an option is still the prompt: after the
// only "--", or attached to the prompt option in one argument (F013-FR-006,
// SC-003).
func TestInvocationsNeverParseTheInstructionAsAnOption(t *testing.T) {
	for id, invocation := range Invocations() {
		for _, instruction := range hostileInstructions {
			if problem := instructionProblem(invocation, instruction); problem != "" {
				t.Errorf("%s: %s", id, problem)
			}
		}
	}
}

// instructionProblem describes how args would expose the instruction to the
// agent's parser, or returns "" when the instruction is safe.
func instructionProblem(invocation Invocation, instruction string) string {
	_, args := invocation.Command("/project", instruction)
	n := len(args)
	switch invocation.Form {
	case AfterEndOfOptions:
		if n < 2 || args[n-1] != instruction || args[n-2] != "--" || slices.Index(args, "--") != n-2 {
			return fmt.Sprintf("instruction %q gives args %#v", instruction, args)
		}
	case AttachedOption:
		if n < 1 || args[n-1] != invocation.Option+"="+instruction || slices.Contains(args[:n-1], "--") {
			return fmt.Sprintf("instruction %q gives args %#v", instruction, args)
		}
	default:
		return fmt.Sprintf("unknown form %d", invocation.Form)
	}
	return ""
}

// No invocation relaxes approvals, permissions, trust or sandboxing, or
// selects anything beyond the non-interactive mode (F013-FR-007, SC-004).
func TestInvocationsCarryNoRelaxingOrSelectingOption(t *testing.T) {
	for id, invocation := range Invocations() {
		if problems := unsafeArguments(id, invocation); len(problems) != 0 {
			t.Errorf("%s", strings.Join(problems, "\n"))
		}
	}
}

func TestInvocationsClassifyExitCodesAsTheAgentsOwnResult(t *testing.T) {
	for id, invocation := range Invocations() {
		if got := invocation.Classify(0); got != execution.AgentSucceeded {
			t.Errorf("%s Classify(0) = %q", id, got)
		}
		if got := invocation.Classify(7); got != execution.AgentFailed {
			t.Errorf("%s Classify(7) = %q", id, got)
		}
	}
}

// The guards above must fire: these fixtures each break one rule.
func TestCatalogGuardsDetectViolations(t *testing.T) {
	catalog := agent.Catalog()
	missing := map[agent.ID]Invocation{}
	for id, invocation := range Invocations() {
		if id != agent.GeminiCLI {
			missing[id] = invocation
		}
	}
	if len(catalogDrift(catalog, missing)) == 0 {
		t.Error("a missing invocation row was not detected")
	}
	wrongExecutable := map[agent.ID]Invocation{}
	for id, invocation := range Invocations() {
		wrongExecutable[id] = invocation
	}
	wrongExecutable[agent.OpenCode] = Invocation{Executable: "oc", Mode: []string{"run"}, Form: AfterEndOfOptions}
	if len(catalogDrift(catalog, wrongExecutable)) == 0 {
		t.Error("an executable differing from the catalog was not detected")
	}
	extra := map[agent.ID]Invocation{"stranger": {Executable: "stranger", Form: AfterEndOfOptions}}
	for id, invocation := range Invocations() {
		extra[id] = invocation
	}
	if len(catalogDrift(catalog, extra)) == 0 {
		t.Error("an invocation outside the catalog was not detected")
	}
	if instructionProblem(Invocation{Executable: "x", Mode: []string{"exec", "--"}, Form: AfterEndOfOptions}, "-x") == "" {
		t.Error("a second end-of-options marker before the instruction was not detected")
	}
	if instructionProblem(Invocation{Executable: "x", Form: 9}, "-x") == "" {
		t.Error("an unknown form was not detected")
	}
	for _, bad := range []Invocation{
		{Executable: "x", Mode: []string{"-y"}, Form: AfterEndOfOptions},
		{Executable: "x", Mode: []string{"exec", "-s", "danger-full-access"}, Form: AfterEndOfOptions},
		{Executable: "x", Mode: []string{"--allowed-tools"}, Form: AfterEndOfOptions},
		{Executable: "x", Mode: []string{"run", "--sandbox=off"}, Form: AfterEndOfOptions},
		{Executable: "x", Mode: []string{"chat", "--yes"}, Form: AfterEndOfOptions},
		{Executable: "x", Form: AttachedOption, Option: "--message"},
		{Executable: "x", Mode: []string{"--yolo"}, Form: AfterEndOfOptions},
		{Executable: "x", Mode: []string{"exec", "--Dangerously-Skip-Permissions"}, Form: AfterEndOfOptions},
		{Executable: "x", Mode: []string{"--model", "m"}, Form: AfterEndOfOptions},
		{Executable: "x", Mode: []string{"--trust-all-tools"}, Form: AfterEndOfOptions},
		{Executable: "x", Form: AttachedOption, Option: "-p"},
		{Executable: "x", Form: AttachedOption, Option: ""},
		{Executable: "x", Form: AfterEndOfOptions, Option: "--prompt"},
		{Executable: "x", Mode: []string{"--"}, Form: AfterEndOfOptions},
	} {
		if len(unsafeArguments("fixture", bad)) == 0 {
			t.Errorf("unsafe invocation %#v was not detected", bad)
		}
	}
}

// The table cannot be changed through Invocations (F013-FR-013).
func TestInvocationsReturnsACopy(t *testing.T) {
	copied := Invocations()
	claude := copied[agent.ClaudeCode]
	claude.Mode[0] = "--yolo"
	copied[agent.ClaudeCode] = claude
	delete(copied, agent.CodexCLI)
	fresh := Invocations()
	if fresh[agent.ClaudeCode].Mode[0] != "--print" {
		t.Fatalf("a caller changed the table: %#v", fresh[agent.ClaudeCode])
	}
	if _, ok := fresh[agent.CodexCLI]; !ok {
		t.Fatal("a caller removed a row from the table")
	}
}
