# Contract: `ai-dev workflow guide`

## Synopsis

```text
ai-dev workflow guide [--format text|json] <feature-id>
```

The usage line, printed on standard error for a usage failure, is:

```text
Usage: ai-dev workflow guide [--format text|json] <feature-id>
```

## Usage failures (exit 2, nothing on standard output)

These are 005 FR-019's conditions applied to this command:
- no identifier, or more than one;
- an identifier that is empty or contains only whitespace;
- an unknown option, or a `--format` value other than `text` or `json`.

## Outcomes and exit codes (005 FR-018, unchanged)

| Outcome | Exit |
|---|---|
| `step_recommended` | 0 |
| `workflow_terminal` | 0 |
| `artifacts_inconsistent` | 1 |
| `feature_unknown` | 1 |
| `state_invalid` | 1 |
| usage failure | 2 |
| output failure (cannot render or write) | 2 |

## Human-readable report (`--format text`, default)

The report goes to standard output, with the first line first. Example for feature `f` in state `specified`, with Claude Code installed:

```text
ai-dev: outcome=step_recommended feature="f" state=specified activity=plan role=planning agent=claude-code availability=available note="options for the human only; not authorization. Running an agent or a review is not approval; approval is your own Human Gate decision after completing the activity"
  # select the agent for this role (003)
  ai-dev agent select -- planning
  # run the agent (002)
  ai-dev agent run -- claude-code )instruction(
  # after completing the activity, check the transition (004)
  ai-dev workflow transition -- 'f' )target-state(
  # then record your decision (006)
  ai-dev workflow approve --approver )approver( -- 'f' )target-state(
  # replace each )slot( before running; --reason )reason( is optional for approval (006 FR-010)
```

The first-line forms for the other outcomes follow. The same fixed note closes each line, shown as `note="…"`:

```text
ai-dev: outcome=workflow_terminal feature="f" state=approved note="…"
  # report the state (004)
  ai-dev workflow state -- 'f'
ai-dev: outcome=artifacts_inconsistent feature="f" state=planned missing=plan.md note="…"
ai-dev: outcome=feature_unknown feature="f" note="…"
ai-dev: outcome=state_invalid feature="f" note="…"
```

The `feature="…"` field uses Go `%q` escaping (004 FR-028), while templates use the shell quoting of F007-FR-006. A human slot is always written `)name(`, including in the reminder line. No line of the report contains a quoted placeholder.

Each template's label is a shell comment on its own line, above its command, so pasting a whole line, or the whole report, never runs a label word. Only the command lines are meant to be pasted. The status line renders the identifier in 004 FR-028's escaped, quoted form, which since 004 Amendment 5 also backslash-escapes `$`, backquotes and `!`. Pasting it runs nothing the identifier contains, although it is a report and not a command.

The rendering rules:
- **When template lines are omitted:** for the outcomes that have none, and for an identifier that is not template-safe.
- **Which template lines appear:** they follow the data model. `run` appears only when availability is `available`, and `select` is omitted when it is `inconsistent`.
- **The slot reminder:** the closing reminder line appears only when at least one template has a human slot.

## Machine-readable report (`--format json`)

Standard output holds exactly one JSON object with a trailing newline:

```json
{
  "schema_version": "ai-dev.workflow-guide/v1",
  "feature_id": "f",
  "outcome": "step_recommended",
  "current_state": "specified",
  "activity": "plan",
  "role": "planning",
  "agent": "claude-code",
  "agent_availability": "available",
  "templates": [
    {"kind": "select", "label": "select the agent for this role (003)", "command": "ai-dev agent select -- planning", "human_slots": []},
    {"kind": "run", "label": "run the agent (002)", "command": "ai-dev agent run -- claude-code )instruction(", "human_slots": ["instruction"]},
    {"kind": "transition", "label": "after completing the activity, check the transition (004)", "command": "ai-dev workflow transition -- 'f' )target-state(", "human_slots": ["target-state"]},
    {"kind": "approve", "label": "then record your decision (006)", "command": "ai-dev workflow approve --approver )approver( -- 'f' )target-state(", "human_slots": ["approver", "target-state"], "optional_flags": ["--reason"]}
  ],
  "statement": "options for the human only; not authorization. Running an agent or a review is not approval; approval is your own Human Gate decision after completing the activity"
}
```

Which fields appear for each outcome:
- `current_state`, `activity`, `role` and `missing_artifacts` follow 005's JSON exactly: present when 005 presents them, and omitted otherwise.
- `agent` and `agent_availability` appear only for `step_recommended`.
- `templates` is always present. It is empty (`[]`) for outcomes without templates and for an identifier that is not template-safe.
- `statement` is always present.

## Invariants

- **No effects:** no process is started, and no file, record, store or Git state is written. Neither standard input nor the network is used.
- **Nothing derived:** no field or template contains a target state, an approver, a reason or an instruction derived from any source.
- **Unfilled templates do nothing:** pasting a template with any `)slot(` unfilled into sh, dash, bash or zsh, under any of the option sets in research R-03, executes nothing and changes no file.
- **Identifier quoting:** pasting a filled template passes the feature identifier as exactly one positional argument after `--`.
