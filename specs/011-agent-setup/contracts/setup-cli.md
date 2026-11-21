# Contract: `ai-dev setup`

## Usage lines

| Command | Usage line (stderr on a usage failure, exit 2) |
|---|---|
| `ai-dev setup` | `Usage: ai-dev setup` |
| `ai-dev setup show` | `Usage: ai-dev setup show [--format text\|json]` |
| `ai-dev setup apply` | `Usage: ai-dev setup apply --agent AGENT... --role ROLE=AGENT... [--replace] [--format text\|json]` |

`ai-dev setup <unknown>` prints the `setup` group usage: `Usage: ai-dev setup [show|apply] [options]`. The top-level usage line becomes `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|version|help|completion> [options]`.

A usage failure includes: an unknown option, a positional argument, a malformed `--role` value (no `=`), `--format` other than `text` or `json`, and `--format` given to the assistant.

## `setup show`

Text, valid configuration (one line per agent in catalog order, then one per role in role order):

```text
ai-dev: outcome=configured source=project
  agent claude-code availability=available chosen=yes
  agent codex-cli availability=unavailable chosen=yes warning=chosen_agent_unavailable
  role specification agent=claude-code
  role planning agent=claude-code
  role implementation agent=codex-cli
  role review agent=claude-code
  role remediation agent=codex-cli
```

- `not_configured`: `source=built_in`, every agent `chosen=no`, the built-in roles.
- `configuration_invalid`: no `source` and no role lines; agents with `chosen=no`; then one `  reason <code>[ <value>]` line per reason (contracts/config-file.md).

JSON (`--format json`), exactly one object on stdout:

```json
{
  "schema_version": "ai-dev.setup/v1",
  "command": "show",
  "outcome": "configured",
  "source": "project",
  "agents": [
    {"id": "claude-code", "label": "Claude Code", "availability": "available", "chosen": true},
    {"id": "codex-cli", "label": "OpenAI Codex CLI", "availability": "unavailable", "chosen": true}
  ],
  "roles": [
    {"role": "specification", "agent": "claude-code"}
  ],
  "warnings": ["chosen_agent_unavailable:codex-cli"],
  "reasons": []
}
```

`source` is omitted and `roles` is `[]` for `configuration_invalid`. `warnings` and `reasons` are always arrays. The JSON is compact (one line), shown indented here.

## `setup apply`

- `--agent` is repeatable, at least once. `--role ROLE=AGENT` is repeatable; a valid set has exactly five, one per role.
- Validation uses the rules of contracts/config-file.md, applied to the options. Every violation is listed as a reason; `configuration_rejected` writes nothing.
- Then the existing file is loaded:
  - absent: write, `configuration_written`;
  - valid and equal in content to the requested configuration, whatever its formatting: `configuration_unchanged`, nothing written;
  - different, or invalid, without `--replace`: `configuration_exists`, nothing written;
  - different, or invalid, with `--replace`: write, `configuration_written`.
- A write failure is `persistence_failed`, and the previous file is untouched.

Text output: the `setup show` form for the resulting state, with the apply outcome on the first line (`ai-dev: outcome=configuration_written source=project`). For `configuration_rejected` and `configuration_exists`, the first line carries the outcome, followed by the agent lines and the reasons. JSON: the same object with `"command": "apply"`.

## `setup` (assistant)

Requires a terminal on standard input and standard output. Without one: `ai-dev: setup: refused: interactive_terminal_required; use ai-dev setup apply` on stderr, exit 2, before detection.

Transcript (no existing configuration):

```text
Agents detected in the catalog:
  1. claude-code (Claude Code): available
  2. codex-cli (OpenAI Codex CLI): unavailable
Which agents does this project work with? (numbers or ids, comma-separated): 1,2
warning: codex-cli is not available on this machine; it can still be configured
Agent for specification (1 claude-code, 2 codex-cli): 1
Agent for planning (1 claude-code, 2 codex-cli): 1
Agent for implementation (1 claude-code, 2 codex-cli): 2
Agent for review (1 claude-code, 2 codex-cli): 1
Agent for remediation (1 claude-code, 2 codex-cli): 2
This configuration will be written to .ai-dev/config.json:
  agents: claude-code, codex-cli
  specification: claude-code
  ...
Write it? [yes/no]
```

- With a valid existing configuration, each prompt ends with `[current: …]` and an empty answer keeps it.
- With an invalid existing configuration, its reasons are printed first and there is no preselection.
- Only `yes` writes. `no`, any other answer, end of input or an interrupt: `Cancelled. Nothing was changed.`, outcome `setup_cancelled`, exit 1.
- An invalid choice (unknown number or name, no agent chosen, an agent not chosen for a role): `ai-dev: setup: invalid input; nothing was changed` on stderr, exit 2.
- After `yes`, an existing configuration, valid or invalid, is replaced: the confirmation is the explicit decision, so the assistant needs no `--replace`. An equal configuration is `configuration_unchanged`.
- After writing: the `setup show` text for the new state, with outcome `configuration_written` (or `configuration_unchanged`).
- On platforms without terminal detection (other than macOS and Linux), the assistant always refuses with `interactive_terminal_required`, which also fails closed before any write; `setup apply` reports `persistence_failed` there.

## Exit codes

0: `not_configured`, `configured`, `configuration_written`, `configuration_unchanged`. 1: `configuration_invalid`, `configuration_rejected`, `configuration_exists`, `setup_cancelled`, `persistence_failed`. 2: usage and output failures.
