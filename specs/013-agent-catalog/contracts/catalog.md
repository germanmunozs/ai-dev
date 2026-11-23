# Contract: Agent Catalog and Invocations

This is the public contract of Feature 013: the identifiers a person types and sees, and the exact process each one starts. `I` is the instruction, passed unmodified as the single argument shown.

## Catalog (F013-FR-001, FR-002, FR-005)

Order is normative: every listing (`ai-dev agents`, `setup show`, the assistant, help, completion, the cockpit, the name of doctor's passing agent check) uses it.

| # | Identifier | Label | Executable | Invocation |
|---|---|---|---|---|
| 1 | `claude-code` | Claude Code | `claude` | `claude --print -- I` |
| 2 | `codex-cli` | OpenAI Codex CLI | `codex` | `codex exec -- I` |
| 3 | `factory-droid` | Factory Droid | `droid` | `droid exec -- I` |
| 4 | `gemini-cli` | Gemini CLI | `gemini` | `gemini --prompt=I` |
| 5 | `kiro-cli` | Kiro CLI | `kiro-cli` | `kiro-cli chat --no-interactive -- I` |
| 6 | `mistral-vibe` | Mistral Vibe | `vibe` | `vibe --prompt=I` |
| 7 | `opencode` | OpenCode | `opencode` | `opencode run -- I` |
| 8 | `qwen-code` | Qwen Code | `qwen` | `qwen --prompt=I` |

Rows 1 and 2 are unchanged from 002. The admission evidence for each row is in [research.md](../research.md), R-05.

## Rules every row satisfies

- No argument other than the mode selector and the instruction (F013-FR-007). Specifically, ai-dev never passes:
  - `--yolo`, `--auto*`, `--force`, `--trust*`, `--allow-all*`, `--dangerously-*`, `--skip-permissions*` or `--approval-mode`;
  - `--sandbox` changes or `--accept-license`;
  - model, output-format, profile or file options.
- The instruction cannot become an option (F013-FR-006): it follows the only `--`, or it is attached to `--prompt=` in one argument.
- Process launch, streaming, interruption, outcomes (`agent_succeeded`, `agent_failed`, `execution_failed`), reports and exit codes are exactly those of 002 (F013-FR-008).
- Availability is executable lookup only. Nothing is started to list or detect agents.

## Doctor check `coding-agent-tool` (F013-FR-009)

| | Pass | Fail |
|---|---|---|
| When | at least one catalog executable is found on `PATH` | none is found |
| Name | `Coding agent: <id>, <id>, …` (catalog order) | `Coding agent` |
| Explanation | empty (001 contract) | `No supported coding agent was found on PATH.` |
| Remediation | — | `Install a supported coding agent; run ai-dev agents to see the list.` |

Doctor's checks, in order:
1. `git-tool`
2. `speckit-tool`
3. `coding-agent-tool`
4. `git-repository`
5. `speckit-initialized`
6. `constitution`
7. `claude-integration`
8. `codex-integration`

## Adding an agent later (SC-008)

1. Add its admission entry, with evidence for every criterion, to a research record. It is a new subsection when done under a later change.
2. Add its identifier constant in `internal/agent/agent.go`, its catalog row in `internal/agent/registry.go`, and its invocation row in `internal/execution/adapters/catalog.go`. A new mode selector or prompt option must also be added, in review, to the allowlist in `catalog_test.go`.
3. Add its row to the table-driven expectations: `wantCatalog`, `wantVectors`, `catalogIDs`, `catalogExecutables` and `catalogRuns`.
4. Update this table and the agent tables of `README.md` and `README.es.md`.

No command, outcome, schema or other feature's requirement changes.
