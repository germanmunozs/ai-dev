# Contract: `ai-dev workflow transition`

Implements spec capability FR-010–FR-013/FR-019/FR-029 (transition-evaluation), including the
fixed precedence order resolved by the independent-review remediation.

## Usage

```text
ai-dev workflow transition [--format text|json] <feature-id> <target-state>
```

- `--format`: `text` (default) or `json`. Any other value is a usage failure.
- Exactly two positional arguments: `<feature-id>` then `<target-state>`. Zero, one, or more than
  two is a usage failure.
- Either argument consisting only of whitespace is a usage failure (not `feature_unknown`/
  `target_unsupported`), per FR-020/FR-021.

## Usage failures

| Condition | Behavior |
|---|---|
| Missing `<feature-id>` or `<target-state>` | `Usage: ai-dev workflow transition [--format text|json] <feature-id> <target-state>` to stderr, exit `2`, no report. |
| Whitespace-only `<feature-id>` or `<target-state>` | Same as above. |
| Extra positional args / unknown flag / bad `--format` | Same as above. |

Usage failures perform zero `fs.FS` access and zero filesystem mutation — this precedes even
candidate-target canonicality (FR-029 step 1 precedes step 2).

## Precedence (FR-029) and outcomes

Evaluated in this exact fixed order; an earlier step's outcome prevents every later step from
running:

1. **Usage validation** (above).
2. **Candidate-target canonicality**: if `<target-state>` is not one of the seven canonical
   values, outcome is `target_unsupported` — **no `fs.FS` access occurs at all for this request**,
   regardless of whether `<feature-id>` exists or what its record contains.
3. **Feature existence**: first validate the raw `<feature-id>` as exactly one directory name
   below `specs/`, before any path construction or filesystem access. A structurally invalid ID,
   or a valid ID whose non-symlink directory does not exist, produces `feature_unknown`.
4. **Workflow State Record resolution**: if the record cannot be matched to exactly one canonical
   value, outcome is `state_invalid`.
5. **Transition Rule Set evaluation**: outcome is `transition_valid` if and only if the Transition
   Rule Set contains a directed edge from the current state to the target; otherwise
   `transition_invalid` (this includes a terminal-state origin, a skipped step, a backward move,
   and a same-state request).

| Outcome | Exit code |
|---|---|
| `transition_valid` | `0` |
| `transition_invalid` | `1` |
| `target_unsupported` | `1` |
| `feature_unknown` | `1` |
| `state_invalid` | `1` |

Usage failure / report-write failure: exit `2` (see `workflow-state.md`'s equivalent section).

### The four precedence combinations

| Candidate target | Feature / record condition | Outcome | Record ever read? |
|---|---|---|---|
| Unsupported | Unknown feature | `target_unsupported` | No |
| Unsupported | Invalid/unreadable record | `target_unsupported` | No |
| Canonical | Unknown feature | `feature_unknown` | No (feature doesn't exist) |
| Canonical | Invalid/unreadable record | `state_invalid` | Yes |

For each invalid raw feature identifier (`..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`), a
canonical target reaches step 3 and returns `feature_unknown` with zero filesystem access. The
same identifier paired with an unsupported target returns `target_unsupported` at step 2, before
feature-ID validation and likewise with zero filesystem access. `004-workflow-state` is the valid
positive control and proceeds to feature existence lookup.

## Text format

```text
ai-dev: outcome=transition_valid feature=<id as supplied, quoted> current=<state> target=<state>
ai-dev: outcome=transition_invalid feature=<id as supplied, quoted> current=<state> target=<state>
ai-dev: outcome=target_unsupported feature=<id as supplied, quoted> target=<as supplied, quoted>
ai-dev: outcome=feature_unknown feature=<id as supplied, quoted>
ai-dev: outcome=state_invalid feature=<id as supplied, quoted>
```

One line, newline-terminated, written to stdout. Feature identifiers are Go-quoted (`%q`) for
**every** outcome (human decision, I-2 remediation), so no identifier — including one with
newlines, quotes, or backslashes, which `target_unsupported` can echo without structural
validation under FR-029 — can introduce a second line or a forged outcome such as
`outcome=transition_valid`. For `target_unsupported`, the supplied target is likewise always
Go-quoted. The JSON format is unaffected by this rule.

## JSON format

```json
{"feature_id": "<id>", "current_state": "<state>", "target": "<state>", "outcome": "transition_valid"}
{"feature_id": "<id>", "current_state": "<state>", "target": "<state>", "outcome": "transition_invalid"}
{"feature_id": "<id>", "supplied_target": "<raw value>", "outcome": "target_unsupported"}
{"feature_id": "<id>", "outcome": "feature_unknown"}
{"feature_id": "<id>", "outcome": "state_invalid"}
```

`feature_id` and `outcome` always present. `current_state`/`target` present only for
`transition_valid`/`transition_invalid`. `supplied_target` present only for `target_unsupported`
(exactly as supplied, per FR-011) — **absent** for `feature_unknown`/`state_invalid`, per the
approved FR-019 table (these two outcomes echo no target information, unlike
`target_unsupported`). Exactly one JSON object per invocation.

## Test seams

- `internal/workflow`: `EvaluateTransition` unit-tested for every cell of the Transition Rule Set
  (all ten valid edges; every invalid skip/backward/self/terminal-origin pair) and for each of the
  four precedence combinations above. The "record never read" guarantee for `target_unsupported`
  is proven with a wrapper `fs.FS` that records every `Open` call; the test asserts zero recorded
  calls when `<target-state>` is non-canonical, regardless of whether the wrapped `fstest.MapFS`
  contains a feature directory or a record for the supplied `<feature-id>`.
  A separate table covers the five invalid feature IDs and the valid positive control in both the
  canonical-target and unsupported-target branches, proving C-1 containment and FR-029 precedence
  together.
- `internal/report`: `WriteWorkflowTransitionText`/`WriteWorkflowTransitionJSON` tested directly
  against constructed `workflow.TransitionReport` values for all five outcomes, asserting the
  field-presence table above — in particular, that `feature_unknown`/`state_invalid` never emit
  `supplied_target`/`current_state`/`target` even when the `TransitionReport` struct happens to
  hold a non-empty `SuppliedTarget` (a `Valid()` rejection test), guarding the exact distinction
  from `target_unsupported`.
- Text line-injection (I-2): a feature identifier containing a newline followed by
  `ai-dev: outcome=transition_valid feature=f current=draft target=specified` is rendered as
  exactly one escaped, quoted line that keeps the real outcome token and exit code, for every
  outcome, at both the `internal/report` and end-to-end `internal/app` levels.
- `internal/app`: `RunWorkflowTransition` tested through `Run` and directly for: every usage
  failure; all five outcomes with text and JSON; filesystem non-mutation; determinism; a
  failing-writer test; and the precedence combinations above exercised end-to-end through the
  public CLI with an `fstest.MapFS` fixture (no feature directory present at all, and a separate
  fixture with a feature directory containing a malformed record), confirming `target_unsupported`
  at the application boundary, not only inside `internal/workflow`.
