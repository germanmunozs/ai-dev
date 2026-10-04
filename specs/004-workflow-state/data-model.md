# Data Model: Workflow State

Concrete representations of spec.md's Key Entities, with the plan-level storage decision from
`research.md` filled in.

## State

```go
type State string

const (
	Draft       State = "draft"
	Specified   State = "specified"
	Planned     State = "planned"
	Tasked      State = "tasked"
	Implemented State = "implemented"
	Approved    State = "approved"
	Abandoned   State = "abandoned"
)

func Recognized(value string) (State, bool)  // exact, case-sensitive; no normalization (FR-002, FR-006)
func Terminal(s State) bool                  // true only for Approved, Abandoned (FR-003)
```

Corresponds to spec's **Workflow State** entity. Exactly seven values; `Terminal` is a pure
function of the value, not a stored flag, so it cannot drift from FR-003.

## Transition Rule Set

```go
func Allowed(from, to State) bool // FR-004 — see plan.md "Transition Rule Set as Code"
```

Corresponds to spec's **Transition Rule Set** entity. Ten valid directed edges total: five
forward steps (`draft→specified→planned→tasked→implemented→approved`) plus five `*→abandoned`
edges from every non-terminal state. No entry originates from `Approved` or `Abandoned`. Not
represented as data (no map/slice) — see plan.md for the rationale shared with
`internal/selection`'s `mappedAgent`.

## Feature Identifier

Represented as a plain `string` wherever it appears (`featureID` parameter name), exactly as
`internal/selection` represents `SuppliedRole` as a plain string before recognition. It has no
fixed enumeration, but it does have one structural rule before resolution: the raw value must
satisfy `fs.ValidPath(featureID)`, must not equal `.`, and must contain no `/`. This means it is
exactly one relative directory name below `specs/`, never an absolute path, traversal, or nested
component. Validation occurs before any `path.Join`/`path.Clean` operation.

An invalid structural value is not a usage failure when it is non-empty/non-whitespace; it is a
recognized request that resolves to `feature_unknown`. Under FR-029, an unsupported transition
target still returns `target_unsupported` before this feature-resolution check and before any
filesystem access. A valid value exists only when the supplied filesystem supports
`fs.ReadLinkFS` and `specs/<value>` is a real, non-symlink directory (`FeatureExists`); lack of
no-follow inspection support is rejected rather than falling back to link-following behavior.

## Workflow State Record

- **Location**: `specs/<feature-id>/workflow-state.json` (research.md §1; FR-027).
- **Schema**: `{"state": "<string>"}` — the only field this feature reads or defines.
- **Go representation**: no record struct. An unexported `stateField` helper inside `record.go`
  walks the top-level JSON object with `json.Decoder` tokens, matching the member name `state`
  exactly and case-sensitively; a case-folding variant of `state` or a duplicated `state` member
  makes the record invalid (I-1 remediation; `json.Unmarshal` into a struct would fold case and
  resolve duplicates last-key-wins, so it is deliberately not used).
- Absent file → `Draft` (FR-005). Present but unreadable/unparseable/absent-field/empty/
  whitespace-only/non-canonical field → `invalid` (FR-007) — see `record.go` in plan.md.
- A symlink at `specs/<feature-id>` is not an existing feature. A symlink at
  `workflow-state.json` is a present-but-invalid record; neither is followed.
- No Go type or function in this feature writes this file (FR-027's "this feature does not define
  any mechanism that creates, writes, or updates a Workflow State Record").

## Outcome

```go
type Outcome string

const (
	StateReported     Outcome = "state_reported"
	FeatureUnknown    Outcome = "feature_unknown"
	StateInvalid      Outcome = "state_invalid"
	TransitionValid   Outcome = "transition_valid"
	TransitionInvalid Outcome = "transition_invalid"
	TargetUnsupported Outcome = "target_unsupported"
)
```

One shared type for both capabilities, since `FeatureUnknown`/`StateInvalid` are the identical
token in both outcome sets per spec FR-013 — a single type avoids two independently-maintained
string literals for the same value.

## StateReport

Corresponds to spec's **Workflow State Report** entity.

```go
type StateReport struct {
	FeatureID string
	State     State
	Terminal  bool
	Outcome   Outcome
}
```

| Outcome | FeatureID | State | Terminal |
|---|---|---|---|
| `state_reported` | required | required | required (rendered even when `false`) |
| `feature_unknown` | required | absent | absent |
| `state_invalid` | required | absent | absent |

`StateReport.Valid()` enforces these combinations directly:

- `FeatureID` is always non-empty/non-whitespace.
- `state_reported`: `FeatureID` is structurally valid; `State` is canonical; `Terminal` equals
  `Terminal(State)` exactly.
- `feature_unknown`: `State == ""` and `Terminal == false`; `FeatureID` may be structurally valid
  but absent from the filesystem, or structurally invalid and therefore deliberately unresolved.
- `state_invalid`: `FeatureID` is structurally valid; `State == ""` and `Terminal == false`.
- Every other outcome is rejected.

## TransitionReport

Corresponds to spec's **Transition Evaluation Result** entity.

```go
type TransitionReport struct {
	FeatureID      string
	CurrentState   State
	SuppliedTarget string
	Target         State
	Outcome        Outcome
}
```

| Outcome | FeatureID | CurrentState | SuppliedTarget | Target |
|---|---|---|---|---|
| `transition_valid` | required | required | required | required |
| `transition_invalid` | required | required | required | required |
| `target_unsupported` | required | absent | required (echoed exactly as supplied, FR-011) | absent |
| `feature_unknown` | required | absent | absent | absent |
| `state_invalid` | required | absent | absent | absent |

This table is the literal source for both `TransitionReport.Valid()` (internal/workflow) and the
two report writers' field-presence behavior (internal/report) — see plan.md "Report Field-Presence
Contract".

`TransitionReport.Valid()` additionally enforces:

- `FeatureID` is always non-empty/non-whitespace.
- `transition_valid`/`transition_invalid`: `FeatureID` is structurally valid; `CurrentState` and
  `Target` are canonical; `SuppliedTarget == string(Target)`; and `Allowed(CurrentState, Target)`
  is respectively true/false.
- `target_unsupported`: `SuppliedTarget` is non-empty/non-whitespace and noncanonical;
  `CurrentState == ""` and `Target == ""`. `FeatureID` may be structurally invalid because FR-029
  returns this outcome before feature resolution.
- `feature_unknown`: `CurrentState == ""`, `SuppliedTarget == ""`, and `Target == ""`;
  `FeatureID` may be structurally valid-but-absent or structurally invalid.
- `state_invalid`: `FeatureID` is structurally valid and `CurrentState == ""`,
  `SuppliedTarget == ""`, `Target == ""`.
- Every outcome outside the transition capability's five canonical outcomes is rejected.

## Relationships

- `EvaluateState`/`EvaluateTransition` are the only producers of `StateReport`/`TransitionReport`.
- Neither report type, nor any type in this file, references `agent.ID` or `role.Role` — this
  package has no import of `internal/agent`, `internal/role`, or `internal/selection`, matching
  the spec's "conceptually separate from agent execution, agent availability, role selection"
  requirement at the type level.
