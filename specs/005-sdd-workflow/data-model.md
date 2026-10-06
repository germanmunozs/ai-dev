# Data Model: SDD Workflow (005)

All 005-owned types live in the new package `internal/sdd`. They reuse, and never redefine:
- `workflow.State` and its seven constants, `workflow.StateReport`, `workflow.EvaluateState`, and
  `workflow.FeatureDir` (004);
- `role.Role` and its constants (003).

## Reused entities (owned elsewhere)

| Entity | Owner | Used by 005 as |
|---|---|---|
| `workflow.State` (7 canonical values) | 004 | Key of the Next-Step Table and of the Required Artifacts rule; value of `Report.State` |
| Terminality (`workflow.Terminal`, surfaced as `StateReport.Terminal`) | 004 | Sole input to step 5 (terminal check) |
| Feature recognition and record resolution (`workflow.EvaluateState`) | 004 | Steps 2–3, called unchanged |
| Feature directory location (`workflow.FeatureDir`, new I/O-free export, research §2) | 004 | Directory listed in step 4, obtained only after `state_reported` |
| `role.Role` (`specification`, `planning`, `implementation`, `review`) | 003 | Value of `Report.Role`, by reference. `remediation` is never used. |

005 holds no transition data. It never references `workflow.Allowed`, `EvaluateTransition`, or
`TransitionReport`.

## Activity (005)

```go
type Activity string

const (
	ActivitySpecify   Activity = "specify"
	ActivityPlan      Activity = "plan"
	ActivityTasks     Activity = "tasks"
	ActivityImplement Activity = "implement"
	ActivityReview    Activity = "review"
)
```

There are exactly five values. None of them equals any of the seven Workflow State tokens. A test
asserts that the two sets are disjoint, so a report can never contain a second state token through
the activity field (FR-003).

## Next-Step Table (005, FR-007)

This is an unexported exhaustive switch, `nextStep(workflow.State) (Activity, role.Role, bool)`:

| State | Activity | Role |
|---|---|---|
| `draft` | `specify` | `role.Specification` |
| `specified` | `plan` | `role.Planning` |
| `planned` | `tasks` | `role.Planning` |
| `tasked` | `implement` | `role.Implementation` |
| `implemented` | `review` | `role.Review` |
| any other value | — | — (returns `false`) |

There is no target-state output. The tests require all of the following:
- `!workflow.Terminal(s)` ⇔ `nextStep(s)` returns `ok`, across all seven canonical states;
- no entry yields `role.Remediation`;
- the table is exactly the five rows above.

## Artifact names and Required Artifacts rule (005, FR-010)

```go
const (
	SpecArtifact  = "spec.md"
	PlanArtifact  = "plan.md"
	TasksArtifact = "tasks.md"
)
```

This is an unexported switch, `requiredArtifacts(workflow.State) []string`. It returns a new slice
in canonical order each time:

| State | Required |
|---|---|
| `draft` | `nil` |
| `specified` | `spec.md` |
| `planned` | `spec.md`, `plan.md` |
| `tasked`, `implemented`, `approved` | `spec.md`, `plan.md`, `tasks.md` |
| `abandoned` | `nil` |

### Presence rule (research §3)

Given the directory `dir` from `workflow.FeatureDir`, presence is decided as follows:
- If `fs.ReadDir(root, dir)` returns an error, every required artifact is missing.
- Otherwise a required name is present if and only if some entry satisfies both
  `entry.Name() == name` (exact byte comparison) and `entry.Type().IsRegular()`.
- Missing names are reported in canonical order. When nothing is required, no listing is
  performed.

## Outcome (005, FR-013)

```go
type Outcome string

const (
	StepRecommended       Outcome = "step_recommended"
	WorkflowTerminal      Outcome = "workflow_terminal"
	ArtifactsInconsistent Outcome = "artifacts_inconsistent"
	FeatureUnknown        Outcome = "feature_unknown" // same token and meaning as 004
	StateInvalid          Outcome = "state_invalid"   // same token and meaning as 004
)
```

`FeatureUnknown` and `StateInvalid` are 005-typed constants whose string values are tested to
equal `string(workflow.FeatureUnknown)` and `string(workflow.StateInvalid)` (FR-005). There are no
other outcomes. Usage and output failures are not outcomes.

## Report (005, Next-Step Report)

```go
type Report struct {
	FeatureID string         // as supplied (JSON coerces invalid UTF-8 to U+FFFD; research §8)
	State     workflow.State // observed current state (when required)
	Activity  Activity       // step_recommended only
	Role      role.Role      // step_recommended only
	Missing   []string       // artifacts_inconsistent only, non-empty, canonical order
	Outcome   Outcome
}
```

### `Report.Valid()` invariants (the FR-016 field table as code)

These checks apply to every report:
- `FeatureID` is not blank.
- `Outcome` is one of the five values.

Per outcome:

| Outcome | `State` | `Activity` / `Role` | `Missing` | Extra |
|---|---|---|---|---|
| `step_recommended` | canonical, `!Terminal` | must equal `nextStep(State)` | empty | `FeatureDir(FeatureID)` ok |
| `workflow_terminal` | canonical, `Terminal` | empty | empty | `FeatureDir` ok |
| `artifacts_inconsistent` | canonical | empty | non-empty; strictly increasing canonical-order subset of `requiredArtifacts(State)` | `FeatureDir` ok |
| `feature_unknown` | empty | empty | empty | ID may be structurally invalid |
| `state_invalid` | empty | empty | empty | `FeatureDir` ok |

`RunWorkflowNext` checks `Valid()` before writing. An invalid report is an FR-020 output failure
of the "cannot be rendered" subcase (research §6, §9):
- nothing goes to stdout;
- the diagnostic `ai-dev: internal error: … cannot be rendered …` goes to stderr;
- the exit code is 2.

It is never one of the five outcomes and never a separate exit-code category. The writers also
call `Valid()` and refuse an invalid report, as defense in depth. A writer I/O error, including a
broken pipe (research §11), is the "cannot be written or delivered" subcase of the same output
failure: also exit 2, with the diagnostic `write workflow next report: …`.

## Evaluation flow (precedence, FR-014)

```text
RunWorkflowNext (internal/app) → runWorkflowNext
  0. release := brokenPipeAsError(); defer release()          (Unix only; no-op elsewhere)
  1. usage validation ── fail → stderr usage, exit 2 (no fs access)
  └─ sdd.Evaluate(root, featureID)
       2–3. sr := workflow.EvaluateState(root, featureID)
            feature_unknown → Report{feature_unknown}
            state_invalid   → Report{state_invalid}
       4.   missing := missingArtifacts(root, featureID, sr.State)   // FeatureDir + one ReadDir
            len(missing) > 0 → Report{artifacts_inconsistent, State, Missing}
       5.   sr.Terminal → Report{workflow_terminal, State}
       6.   nextStep(sr.State) → Report{step_recommended, State, Activity, Role}
       (no table entry for a non-terminal state → Report{} with no outcome)
  (SIGPIPE → EPIPE handling active for the whole runWorkflowNext call, research §11)
  !r.Valid() → nothing on stdout, stderr "ai-dev: internal error: …", exit 2   (output failure: cannot be rendered)
  write report to stdout ── error/EPIPE → stderr "write workflow next report: …", exit 2 (output failure: cannot be written/delivered)
  exit 0 for step_recommended / workflow_terminal; exit 1 otherwise
```

There is no state machine. 005 has no state transitions of its own.
