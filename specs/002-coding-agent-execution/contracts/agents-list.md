# Contract: `ai-dev agents`

**Satisfies**: FR-001, FR-002, FR-003, FR-018 (listing half), FR-020, SC-001, SC-002, SC-006

## Command

```text
ai-dev agents [--format text|json]
```

- No positional arguments. `--format` defaults to `text`.
- Unknown flags, unknown positional arguments, or an unrecognized `--format` value are
  **usage failures**: exit code `2`, usage text on stderr, stdout empty, no agent
  discovery performed (mirrors the existing `doctor` usage contract exactly).

## Human-readable output (`--format text`, default)

Written to **stdout**. One line per supported agent, in registry (stable) order:

```text
claude-code   Claude Code       available
codex-cli     OpenAI Codex CLI  unavailable
```

- Column 1: canonical ID. Column 2: display label. Column 3: `available` or
  `unavailable`. Exact column alignment is not a contractual guarantee; the three
  values, their order, and the fact that both agents always appear are.

## Machine-readable output (`--format json`)

Written to **stdout**; nothing else is written to stdout. One JSON object, encoded the
same way `ai-dev doctor --format json` encodes its report (`json.NewEncoder`, one
object):

```json
{
  "agents": [
    {"id": "claude-code", "display_label": "Claude Code", "availability": "available"},
    {"id": "codex-cli", "display_label": "OpenAI Codex CLI", "availability": "unavailable"}
  ]
}
```

- `id`: the agent's immutable canonical identifier — always `claude-code` and
  `codex-cli` for this feature (FR-022).
- `availability`: exactly `"available"` or `"unavailable"` (FR-018) — no third value, no
  free-form text requiring parsing (SC-006).
- Both supported agents are always present, in the same order as the text form (FR-003).

## Exit codes

| Condition                          | Exit code |
|-------------------------------------|-----------|
| Recognized `agents` request (either format) | `0` |
| Usage failure (unrecognized flags/args)      | `2` |

Listing has no failure outcome of its own beyond usage failures — FR-002 makes
"unavailable" a normal, successfully-reported result, not an error.

## Test seams this contract implies

- Supported-agent discovery: fake `system.Executor` reporting both found.
- Unavailable agent: fake `system.Executor` reporting lookup failure for one/both.
- Determinism: same fake `system.Executor` state queried twice yields identical output
  (SC-002 / FR-003).
- Machine-readable isolation: stdout contains only the JSON object, nothing else.
