# Contract: `ai-dev workflow state`

Implements spec capability FR-001/FR-005–FR-009/FR-019 (state-reporting).

## Usage

```text
ai-dev workflow state [--format text|json] <feature-id>
```

- `--format`: `text` (default) or `json`. Any other value is a usage failure.
- Exactly one positional argument, `<feature-id>`. Zero, or more than one (after flag parsing),
  is a usage failure.
- A `<feature-id>` consisting only of whitespace is a usage failure (not `feature_unknown`),
  per FR-020.

## Usage failures

| Condition | Behavior |
|---|---|
| No `<feature-id>` | `Usage: ai-dev workflow state [--format text|json] <feature-id>` to stderr, exit `2`, no report, no `fs.FS` access. |
| Whitespace-only `<feature-id>` | Same as above. |
| Extra positional args | Same as above. |
| Unknown flag | Same as above. |
| `--format` not `text`/`json` | Same as above. |

Usage failures perform zero `fs.FS` access (no `FeatureExists`/`ResolveRecord` call) and zero
filesystem mutation.

## Outcomes

| Outcome | Trigger | Exit code |
|---|---|---|
| `state_reported` | `<feature-id>` resolves to an existing feature and its current Workflow State is determinable (recorded or defaulted to `draft`). | `0` |
| `feature_unknown` | `<feature-id>` does not resolve to an existing feature (`specs/<feature-id>` is not a directory). | `1` |
| `state_invalid` | The feature exists but its Workflow State Record cannot be matched to exactly one canonical value (unreadable, unparseable, absent/empty/whitespace state field, or non-exact value). | `1` |

Exit code is `0` if and only if the outcome is `state_reported`; `1` for `feature_unknown`/
`state_invalid`; `2` for a usage failure or a report-write failure (see plan.md "Report
Validation / Write Failure").

A non-empty, non-whitespace feature identifier that is not exactly one safe directory name below
`specs/`—including `..`, `../x`, `a/../../x`, `/etc/passwd`, or `a/b`—is a recognized request with
outcome `feature_unknown`. The raw identifier is rejected before path construction and before any
`fs.FS` access; no usage failure or new workflow outcome is introduced.

## Text format

```text
ai-dev: outcome=state_reported feature=<id as supplied, quoted> state=<state> terminal=<true|false>
ai-dev: outcome=feature_unknown feature=<id as supplied, quoted>
ai-dev: outcome=state_invalid feature=<id as supplied, quoted>
```

One line, newline-terminated, written to stdout. The feature identifier is rendered Go-quoted
with `%q` for **every** outcome (human decision, I-2 remediation): any identifier — including one
containing spaces, newlines, quotes, backslashes, or other special characters — stays inside the
single report line in escaped form and cannot introduce a second line or an apparent outcome
token. `state`/`terminal` use `%s`/`%t` because `State` values are one of the seven canonical
tokens once this far. The JSON format is unaffected by this rule.

## JSON format

```json
{"feature_id": "<id>", "state": "<state>", "terminal": true, "outcome": "state_reported"}
{"feature_id": "<id>", "outcome": "feature_unknown"}
{"feature_id": "<id>", "outcome": "state_invalid"}
```

- `feature_id` and `outcome` are always present.
- `state` is present (one of the seven tokens) if and only if the outcome is `state_reported`.
- `terminal` is present (as JSON `true`/`false`, never omitted for a falsy value) if and only if
  the outcome is `state_reported`.
- Exactly one JSON object is written to stdout; no other content accompanies it.

## Test seams

- `internal/workflow`: `EvaluateState` unit-tested directly against an `fstest.MapFS` for all
  three outcomes, including a record naming each of the seven states, a missing record (→
  `draft`), and every `state_invalid` sub-case from spec SC-010 (absent/empty/whitespace/non-exact
  state field, unparseable JSON, unreadable file).
  The same package tests the raw feature-ID rule before path construction with invalid cases
  `..`, `../x`, `a/../../x`, `/etc/passwd`, and `a/b` (each → `feature_unknown`, zero filesystem
  access) plus `004-workflow-state` as the valid positive control.
- `internal/report`: `WriteWorkflowStateText`/`WriteWorkflowStateJSON` unit-tested by constructing
  `workflow.StateReport` values directly for all three outcomes, asserting the exact field-presence
  table above, including a `state_reported` fixture with `State: workflow.Planned` to prove
  `terminal:false` is rendered, not omitted.
- `internal/app`: `RunWorkflowState` tested through `Run` (dispatcher-level) and directly
  (unit-level) for: every usage failure (zero `fs.FS` access asserted via a recording wrapper);
  all three outcomes with text and JSON; filesystem non-mutation (reusing `agentselect_test.go`'s
  `snapshotDir`/`controlledTempDir`); determinism (repeated identical calls, byte-identical
  output); a failing-writer test (always-erroring `io.Writer`, exit `2`, no outcome
  reclassification).
