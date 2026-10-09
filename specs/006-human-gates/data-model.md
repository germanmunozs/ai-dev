# Data Model: Human Gates (006)

Plan-level representation of the spec's Key Entities. Rationale is in [research.md](./research.md).

## Reused entities (owned elsewhere; not redefined)

| Entity | Owner | How 006 uses it |
|---|---|---|
| `workflow.State` and its seven canonical constants, `workflow.Terminal` | 004 | Values carried in requests, reports, and evidence. Never re-listed or re-validated by 006. |
| Transition Rule Set (`workflow.Allowed`), FR-029 precedence | 004 | Only through `workflow.EvaluateTransition`. 006 never calls `Allowed` directly. |
| `workflow.Outcome` values `feature_unknown`, `state_invalid`, `target_unsupported`, `transition_invalid` | 004 | Re-emitted with identical meaning. 006's outcome type reuses these exact token strings. |
| Workflow State Record (`specs/<id>/workflow-state.json`) and its reading rules, `FeatureExists`, `ResolveRecord`, `FeatureDir` | 004 | Read through 004's functions before the commit and under the lock. Written only by `internal/gatestore` in a form those reading rules resolve unchanged. |
| Required Artifacts Rule | 005 | Only through the new `sdd.MissingArtifacts` wrapper ([contract](./contracts/sdd-missingartifacts.md)). |

## Decision (006)

```go
type Decision string
const (
    Approval    Decision = "approval"
    Abandonment Decision = "abandonment"
)
```

These are exactly two values (FR-008). The CLI verb selects the value: `approve` gives `approval`,
and `abandon` gives `abandonment`. The value is never inferred.

## Request (006, Human Gate Request)

| Field | Type | Rule |
|---|---|---|
| `FeatureID` | string | As supplied. Must be non-whitespace (step 1). Structural validity is 004's job (step 4). |
| `Decision` | `Decision` | From the verb |
| `SuppliedTarget` | string | As supplied. Must be non-whitespace (step 1). C-13 consistency is checked at step 2. |
| `Approver` | string | `--approver`, set exactly once, non-whitespace, retained verbatim (FR-009, FR-033) |
| `Reason` | string plus a `ReasonSet` bool | `--reason`, set at most once. If set, it must be non-whitespace (D-1). It is required for `abandonment`. |

**Usage validation** (app, steps 1–2, no filesystem access) is complete before `gate.Evaluate` is
called. `gate.Evaluate` assumes that a request has passed it.

## Evidence (006, Human Gate Evidence entry)

| Field (JSON key) | Value |
|---|---|
| `feature_id` | `Request.FeatureID`, exactly as supplied. It is 004-valid at this point. |
| `decision` | `"approval"` or `"abandonment"` |
| `approver` | `Request.Approver`, verbatim |
| `from_state` | The current state from `EvaluateTransition`, confirmed equal under the lock |
| `to_state` | The canonical target |
| `decided_at` | `now().UTC()`, RFC 3339 with second precision |
| `reason` | `Request.Reason`, verbatim, present **if and only if** supplied |

There are no other keys (FR-025, FR-033). The full record layout is in
[human-gate-record.md](./contracts/human-gate-record.md).

## Outcome (006)

```go
type Outcome string
const (
    TransitionApplied Outcome = "transition_applied"
    FeatureUnknown    Outcome = "feature_unknown"    // == workflow.FeatureUnknown
    StateInvalid      Outcome = "state_invalid"      // == workflow.StateInvalid
    TargetUnsupported Outcome = "target_unsupported" // == workflow.TargetUnsupported
    TransitionInvalid Outcome = "transition_invalid" // == workflow.TransitionInvalid
    ArtifactsMissing  Outcome = "artifacts_missing"
    StateConflict     Outcome = "state_conflict"
    PersistenceFailed Outcome = "persistence_failed"
)
```

The four reused constants are defined as `Outcome(workflow.X)`, so the strings cannot drift. A test
asserts the equality.

## Store interface (006, persistence port)

```go
type CommitStatus int
const (
    Committed CommitStatus = iota + 1 // rename returned: this request's state and evidence are authoritative
    Conflict                          // active competing lock holder (EWOULDBLOCK), feature dir replaced, or state ≠ Expected: nothing written
    Failed                            // persistence could not complete (incl. ambiguous lock error, evidence not preservable, unsupported platform): nothing written
)

type Store interface {
    // Commit atomically replaces featureID's record with one whose state is entry.ToState and
    // whose human_gates history is the existing history plus entry, if and only if the
    // authoritative state still resolves (via 004) to expected. Warnings are post-commit or
    // cleanup diagnostics that never change the status.
    Commit(featureID string, expected workflow.State, entry Evidence) (CommitStatus, []string)
}
```

The zero value `CommitStatus(0)` is invalid. `gate.Evaluate` maps it to `persistence_failed`
defensively, and a test covers that.

## Report (006, Human Gate Report)

| Field | Type |
|---|---|
| `FeatureID`, `Decision`, `Approver` | always present |
| `FromState` | `workflow.State` |
| `SuppliedTarget` | string |
| `Target` | `workflow.State` (the canonical target) |
| `Reason` | string plus `ReasonSet` |
| `Missing` | `[]string` |
| `Outcome` | `Outcome` |

### `Report.Valid()` invariants (FR-027's field table as code)

| Outcome | FromState | SuppliedTarget | Target | Reason | Missing | Extra invariants |
|---|---|---|---|---|---|---|
| `transition_applied` | canonical | = `string(Target)` | canonical | iff supplied | empty | `FeatureDir(id)` ok. For `Abandonment`, `Target == abandoned` and the reason is set. For `Approval`, `Target != abandoned`. |
| `transition_invalid` | canonical | = `string(Target)` | canonical | absent | empty | `FeatureDir` ok |
| `state_conflict`, `persistence_failed` | canonical | = `string(Target)` | canonical | absent | empty | `FeatureDir` ok |
| `artifacts_missing` | canonical | = `string(Target)` | canonical | absent | non-empty | `Decision == Approval`, `FeatureDir` ok |
| `target_unsupported` | empty | non-whitespace, non-canonical | empty | absent | empty | `Decision == Approval` (C-13: an abandonment never reaches step 3 with a non-`abandoned` target) |
| `feature_unknown` | empty | empty | empty | absent | empty | none |
| `state_invalid` | empty | empty | empty | absent | empty | `FeatureDir` ok |

For every outcome, `FeatureID`, `Decision`, and `Approver` are non-whitespace. The renderers refuse
to emit an invalid report: the app reports an internal error and exits 2, as 005 does.

## Evaluation flow (FR-026)

```text
app (steps 1–2, no I/O)
  1  parse flags/positionals; whitespace / set-once / reason rules          → usage failure, exit 2
  2  decision-type consistency: abandon ⇒ target == "abandoned";
                                approve ⇒ target != "abandoned"            → usage failure, exit 2
gate.Evaluate(root, req, store, now)
  3–6  workflow.EvaluateTransition(root, id, target)                        → target_unsupported |
                                                                              feature_unknown |
                                                                              state_invalid |
                                                                              transition_invalid
  7    approval only: sdd.MissingArtifacts(root, id, target)                → artifacts_missing
       entry := Evidence{…, decided_at: now().UTC()}
  8–9  store.Commit(id, current, entry)                                     → state_conflict |
                                                                              persistence_failed |
                                                                              transition_applied
app
  10   validate + render report (stdout); write error → stderr, exit 2 (never touches the store)
       exit 0 iff transition_applied; else 1
```

No step writes before step 9. Steps 3–7 use the read-only `fs.FS` the app received, exactly as
`workflow state`, `workflow transition`, and `workflow next` do. Steps 8–9 use the store's own
`os.Root`.
