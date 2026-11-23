// Package adapters holds the verified non-interactive invocation of every
// catalog agent (Feature 013, contracts/catalog.md). Each row passes only the
// mode selector and the instruction; the evidence for each row is in
// specs/013-agent-catalog/research.md, R-05.
package adapters

import "github.com/germanmunozs/ai-dev/internal/agent"

// invocations has exactly one row per catalog agent. It is fixed at build
// time and reachable only through Invocations, which returns copies.
var invocations = map[agent.ID]Invocation{
	// Claude Code's print mode keeps its native permissions (002 Amendment 1).
	agent.ClaudeCode: {Executable: "claude", Mode: []string{"--print"}, Form: AfterEndOfOptions},
	// Codex CLI's exec mode keeps its native sandbox and approvals (002 Amendment 1).
	agent.CodexCLI: {Executable: "codex", Mode: []string{"exec"}, Form: AfterEndOfOptions},
	// droid exec is read-only unless --auto is given; commander honors "--".
	agent.FactoryDroid: {Executable: "droid", Mode: []string{"exec"}, Form: AfterEndOfOptions},
	// Headless --prompt; an ask decision is denied; yargs-parser binds --prompt=.
	agent.GeminiCLI: {Executable: "gemini", Form: AttachedOption, Option: "--prompt"},
	// Untrusted tools are denied without --trust-*; clap honors "--".
	agent.KiroCLI: {Executable: "kiro-cli", Mode: []string{"chat", "--no-interactive"}, Form: AfterEndOfOptions},
	// Programmatic mode denies tool calls that need approval; clap binds --prompt=.
	agent.MistralVibe: {Executable: "vibe", Form: AttachedOption, Option: "--prompt"},
	// run rejects permission requests without --auto; it reads its message from argv["--"].
	agent.OpenCode: {Executable: "opencode", Mode: []string{"run"}, Form: AfterEndOfOptions},
	// Headless runs default to Ask Permissions; yargs-parser binds --prompt=.
	agent.QwenCode: {Executable: "qwen", Form: AttachedOption, Option: "--prompt"},
}

// Invocations returns a deep copy of the table, so no caller can change the
// verified invocations, not even through a Mode slice.
func Invocations() map[agent.ID]Invocation {
	result := make(map[agent.ID]Invocation, len(invocations))
	for id, invocation := range invocations {
		invocation.Mode = append([]string(nil), invocation.Mode...)
		result[id] = invocation
	}
	return result
}
