# Contract: changes to `agent select` and `workflow guide` (Amendments A1, A2)

## `ai-dev agent select <role>`

Text lines gain ` source=<source>` at the end:

```text
ai-dev: outcome=role_selected role=implementation agent=codex-cli source=built_in
ai-dev: outcome=agent_unavailable role=implementation agent=claude-code source=project
ai-dev: outcome=role_unsupported role="Implementation" source=built_in
ai-dev: outcome=configuration_invalid role=implementation source=project
```

- `role_unsupported` carries the source of the assignment that would have applied: `built_in` without a configuration, `project` with one, valid or not.
- `configuration_invalid` names no agent. Its reasons are not repeated; `ai-dev setup show` lists them.

JSON gains `"source"`: `{"supplied_role":"implementation","requested_role":"implementation","outcome":"configuration_invalid","source":"project"}`.

Exit codes: 0 for `role_selected`, 1 for every other outcome including `configuration_invalid`, 2 for usage and output failures.

## `ai-dev workflow guide <feature>`

For `step_recommended`, the text line gains ` source=<source>` after `availability=…`, and JSON gains `"source"`. Other outcomes carry no agent, availability or source, as today.

With availability `configuration_invalid` there is no agent, so the `agent=` field is omitted from the line (and `"agent"` from JSON):

```text
ai-dev: outcome=step_recommended feature='011-x' state=planned activity=implementation role=implementation availability=configuration_invalid source=project note="…"
```

Templates for `configuration_invalid` are the transition and approval templates, as for `inconsistent`. Exit codes are unchanged (F007-FR-011).

## Unchanged

`ai-dev agents` and `ai-dev agent run` output and behavior (F011-FR-019).
