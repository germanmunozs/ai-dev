# Data Model: Agent Catalog Expansion

## Agent (internal/agent, existing, extended)

| Field | Rule |
|---|---|
| `ID` | `^[a-z0-9]+(-[a-z0-9]+)*$`, unique, immutable once published (F013-FR-002) |
| `DisplayLabel` | Non-empty; never used as an identifier |
| executable | The name looked up on `PATH`. Unexported; read through `Executable()` (new accessor, for the drift test) |

The catalog is an ordered, build-time slice. Its order is `claude-code`, `codex-cli`, then ascending `ID` (F013-FR-005). `Catalog()`, `Known()`, `NewRegistry().List()` and `Resolve()` keep their signatures and semantics.

## Invocation (internal/execution/adapters, new)

| Field | Rule |
|---|---|
| `Executable` | Equals the catalog executable of the same agent |
| `Mode` | Zero or more fixed arguments that select the non-interactive mode, for example `--print`, `exec`, `run`, or `chat --no-interactive`. Never an option that relaxes approvals, selects a model or format, or names a file (F013-FR-007) |
| `Form` | `AfterEndOfOptions` or `AttachedOption` (F013-FR-006) |
| `Option` | Required when `Form` is `AttachedOption` (for example `--prompt`); empty otherwise. Must start with `--` |

`Command(workdir, instruction)` returns `Executable` and the arguments:
- for `AfterEndOfOptions`: `Mode…`, then `--`, then `instruction`;
- for `AttachedOption`: `Mode…`, then `Option + "=" + instruction`.

`Classify(0)` is `agent_succeeded`; any other code is `agent_failed`.

`Invocations` is a `map[agent.ID]Invocation` with exactly one entry per catalog agent.

## Coding Agent Readiness (internal/checks, new)

A `doctor.Check` with:
- identifier `coding-agent-tool`;
- name "Coding agent";
- area `Environment`;
- `Pass` when at least one catalog agent is available, with the available identifiers in catalog order;
- `Fail` otherwise, with the remediation pointing to `ai-dev agents`.

## Unchanged

- The project configuration `.ai-dev/agents.json` (schema `ai-dev.agents/v1`): any catalog identifier is valid.
- The selection policy of 003: the built-in assignment is unchanged.
- The execution `Report` and its outcomes.
- The JSON schemas `ai-dev.agents/v1` (listing), `ai-dev.setup/v1` and the doctor report. The doctor report lists one check fewer, with the new identifier.
