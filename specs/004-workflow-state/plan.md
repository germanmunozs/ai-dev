# Implementation Plan: Workflow State

**Branch**: `004-workflow-state` | **Date**: 2026-10-02 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-workflow-state/spec.md`

## Summary

Add a vendor-neutral workflow-state capability with two strictly read-only, evaluate-and-report
commands: `ai-dev workflow state [--format text|json] <feature-id>` (report a feature's current
Workflow State) and `ai-dev workflow transition [--format text|json] <feature-id> <target-state>`
(evaluate, without applying, whether a candidate transition would be valid).

`internal/workflow` (new) defines the seven canonical `State` values, the fixed Transition Rule
Set (`Allowed(from, to State) bool`, an exhaustive `switch`, not a map — same pattern as
`internal/selection`'s `mappedAgent`), feature-existence and Workflow-State-Record resolution
(`FeatureExists`, `ResolveRecord`), and the two evaluation entry points (`EvaluateState`,
`EvaluateTransition`) that produce `StateReport`/`TransitionReport` values. `EvaluateTransition`'s
own control flow *is* the proof of the approved FR-029 precedence: candidate-target canonicality
is checked and can return before raw feature-ID validation and `FeatureExists`/`ResolveRecord` —
which perform the only filesystem I/O in this feature — are ever called. Before either resolver
constructs a path, the raw feature ID must identify exactly one directory name below `specs/`;
neither a feature-directory symlink nor a record symlink is followed.

Both functions take a `fs.FS` (the same `root` parameter `app.Run` already threads through to
`RunDoctor`) and nothing else: `io/fs.FS` exposes no write method at all, so FR-014's
zero-filesystem-mutation guarantee is a property of the Go type system, not just of test
coverage — the same "architectural guarantee, not just behavioral" pattern 003's Dependency
Boundary established for `system.Launcher`/`system.Executor`.

`internal/app` gains two new files (`workflowstate.go`, `workflowtransition.go`) following the
exact shape of `agentselect.go`. `internal/report` gains two new files
(`workflowstate.go`, `workflowtransition.go`) following the exact shape of `roleselection.go`,
including a `Valid()`-gated defensive-rendering check mirroring `selection.Report.Valid()`.
`doctor.go`'s `Run` gains one new `args[0] == "workflow"` branch, reusing the `root fs.FS`
parameter it already has — no change to `Run`'s signature.

No existing file's behavior, signature, or test assertions for `doctor`, `agents`, `agent run`,
or `agent select` change.

## Technical Context

**Language/Version**: Go 1.27.1 (matches existing `go.mod`; no change)

**Primary Dependencies**: Go standard library only (`flag`, `io/fs`, `path`, `encoding/json`,
`bytes`, `errors`, `strings`, `fmt`, `io`) — no new module dependency, consistent with 001/002/003 and
spec Assumptions ("left open as long as observable behavior is satisfied")

**Storage**: A Workflow State Record is a JSON file, `workflow-state.json`, colocated inside a
feature's own `specs/<feature-id>/` directory, containing `{"state": "<one of the seven
canonical values>"}`. This is the plan-level filename/schema decision the specification
deliberately deferred (spec Assumptions); no mechanism in this feature creates, writes, or
updates this file — it is read-only from this feature's perspective, exactly as FR-027 and the
spec's evaluate-and-report-only decision require

**Testing**: `go test` with the standard library `testing` package and `testing/fstest.MapFS`
(the existing `root fs.FS` test-fixture mechanism already used by `doctor_test.go` and
`agentselect_test.go`'s `readyFS()`), following the white-box-in-package convention established
by `internal/role`/`internal/selection`. Coverage includes: all seven canonical states and exact
case-sensitive recognition; the complete Transition Rule Set (every valid edge, every invalid
skip/backward/self/terminal-origin pair); all six outcome values across both commands; the four
precedence combinations from FR-029 (including a recording-`fs.FS` proof that
`target_unsupported` never calls `Open`); every usage-failure class; text/JSON report contracts
per the FR-019 field tables; stream placement and completion-code semantics; determinism;
zero-filesystem-mutation evidence (reusing `agentselect_test.go`'s `snapshotDir`/
`controlledTempDir` helpers); raw feature-ID containment tests for `..`, `../x`, `a/../../x`,
`/etc/passwd`, and `a/b` plus valid control `004-workflow-state`; and symlink fixtures for both
the feature directory and record. No live Claude Code or Codex CLI service access is required.

**Target Platform**: Same as existing `ai-dev` — no platform-specific code; no process
supervision of any kind is performed by this feature

**Project Type**: Single Go module CLI (matches existing `cmd/`, `internal/` layout — no new
top-level project structure)

**Performance Goals**: Not a performance-sensitive feature; one request performs at most two
`fs.ReadLinkFS.Lstat` calls (feature directory and record), one `fs.ReadFile`, and one report write

**Constraints**: Must never create, modify, or delete any Workflow State Record or other
filesystem content, regardless of outcome (FR-014, FR-020) — enforced structurally by `fs.FS`
having no write method; must never invoke a coding agent or perform role selection (FR-015); must
not change the observable behavior of `doctor`, `agents`, `agent run`, or `agent select`
(FR-026); must implement the fixed FR-029 precedence exactly, including the "no record read once
`target_unsupported` is determined" guarantee (FR-011); must validate a raw feature ID before any
path construction and must not follow feature-directory or record symlinks; must remain
deterministic (FR-018)

**Scale/Scope**: Seven fixed states, ten fixed transition edges, six outcome values across two
commands, one request produces exactly one report, no concurrency/coordination requirements
(spec Edge Cases)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design below.*

| Principle | Assessment |
|---|---|
| I. Specification First | Spec underwent three rounds of independent review (clarification, I-1/I-2/I-3/M-1–M-4, P-1) and the final re-review assessed it READY; this plan implements exactly its behavioral contract, including the FR-029 precedence. PASS |
| II. Architecture Before Framework | `internal/workflow` follows directly from the spec's own entities (Workflow State, Transition Rule Set, Workflow State Record, the two report types) rather than from a preferred tool; the `fs.FS`-only dependency follows directly from FR-014's read-only requirement, not from a library preference. PASS |
| III. Simplicity and YAGNI | No new abstraction beyond what FR-004/FR-029 require: one exhaustive-switch transition function (no map, mirroring `internal/selection`'s `mappedAgent`), two small evaluation functions, two CLI subcommands reusing the existing `args[0] == "workflow"` dispatch pattern and the `root fs.FS` parameter `Run` already has. No persistence, no write path, no plugin system. PASS |
| IV. Testable by Design | Every FR has a corresponding test seam (see Contracts' "Test seams" sections and Technical Context above); the FR-029 precedence is provable both structurally (function call order) and behaviorally (a recording `fs.FS` wrapper proving zero `Open` calls for `target_unsupported`). The explicit C-1 matrix proves raw identifier containment before I/O, while symlink fixtures and exhaustive `Valid()` tables make the remaining security and report invariants directly testable. PASS |
| V. Security by Design | FR-014/FR-020 are enforced by construction: `EvaluateState`/`EvaluateTransition` take only an `fs.FS`, an interface with no write method. Before any `path.Join`, the raw feature identifier must satisfy `fs.ValidPath`, must not be `.`, and must contain no `/`, so it represents exactly one name below `specs/`; invalid values become `feature_unknown` with zero filesystem access. Feature and record inspection requires `fs.ReadLinkFS` and calls its no-follow `Lstat`; unsupported filesystems are rejected rather than falling back to link-following `fs.Stat`. Reports contain only caller-supplied identifiers/targets, canonical state tokens, and fixed outcomes—never environment data. The security/scope audit gate checks these properties. PASS |
| VI. Observability by Design | Not a backend/distributed system; N/A beyond the report itself. |
| VII. Reproducibility and Automation | Quality gates below are the same scripted, version-controlled commands 001/002/003 already use (`gofmt`, `go vet`, `go test`, `go build`, `git diff --check`); none require a live service. PASS |
| VIII. AI-Assisted Engineering | This plan is itself a version-controlled artifact produced ahead of implementation, per the standard Spec Kit flow. PASS |
| IX. Human Authority | Plan does not assert Human Plan Gate approval; it is a proposal awaiting the normal review before `tasks`/implementation proceed. PASS |

No violations requiring a Complexity Tracking entry.

## Quality Gates

All gates are automated, run locally, and require no live Claude Code or OpenAI/Codex CLI
service access — they exercise only the Go toolchain and this repository's own `fstest.MapFS`-
backed test suite.

| Gate | Command | Pass condition |
|---|---|---|
| Formatting | `gofmt -l .` | No output. |
| Vet | `go vet ./...` | Successful completion, no diagnostics. |
| Unit/integration tests | `go test ./...` | All tests pass. |
| Tests without cache | `go test -count=1 ./...` | All tests pass without relying on the test cache. |
| Build | `go build ./cmd/ai-dev` | Successful build, no new `go.mod` dependency. |
| Whitespace | `git diff --check` | No whitespace errors. |

### Security / scope audit gate

A manual code-review checklist run against the diff this feature introduces, verifying:

- No new third-party dependency (`go.mod`/`go.sum` unchanged).
- `internal/workflow` imports nothing from `internal/agent`, `internal/role`, `internal/selection`,
  `internal/execution`, or `internal/system` — it has no way to reach an agent, a role, or a
  process.
- `EvaluateState`/`EvaluateTransition`/`FeatureExists`/`ResolveRecord` accept only `fs.FS` plus
  plain strings — no `io.Writer`-based mutation path, no `os` package call that could create,
  rename, or delete a file. Raw feature IDs are checked before every path construction.
- No authentication step or network probe anywhere in `internal/workflow`, the new `internal/app`
  files, or the new `internal/report` files — the only I/O is no-follow
  `fs.ReadLinkFS.Lstat`/`fs.ReadFile` against the caller-supplied `root`, plus writes to the
  caller-supplied `stdout`/`stderr`.
- The C-1 invalid-ID table returns `feature_unknown` with zero filesystem access, and the same IDs
  paired with an unsupported transition target return `target_unsupported` before identifier
  validation; no joined or cleaned path is ever used as the security check.
- `specs/<feature-id>` and `workflow-state.json` symlinks are rejected and never followed; a
  filesystem lacking `fs.ReadLinkFS` is rejected instead of using `fs.Lstat`'s following fallback.
- No secret, token, credential, or environment value appears in any code path or test fixture.
- No routing, retry, fallback, chaining, background execution, or persistence is introduced —
  `EvaluateState`/`EvaluateTransition` are single synchronous, stateless calls producing one
  report each.

## Dependency Boundary

`EvaluateState` and `EvaluateTransition` take a plain `io/fs.FS` — the Go standard library's
read-only filesystem interface, whose only method is `Open(name string) (fs.File, error)`. There
is no `Write`, `Create`, `Remove`, or `Rename` method anywhere on this interface; `internal/workflow`
therefore has no language-level way to mutate anything it reads, independent of any test
assertion. `app.Run` already holds a `fs.FS` (`root`, used today for `RunDoctor`); the new
`workflow` dispatch branch passes that same value straight through — no new parameter is added to
`Run`, and no new dependency is introduced anywhere in the call chain.

```text
app.Run
  -> RunWorkflowState(args, root, stdout, stderr)   /  RunWorkflowTransition(args, root, stdout, stderr)
  -> workflow.EvaluateState(root, featureID)        /  workflow.EvaluateTransition(root, featureID, suppliedTarget)
  -> workflow.FeatureExists(root, featureID)  (raw-ID validation, then fs.ReadLinkFS.Lstat)
  -> workflow.ResolveRecord(root, featureID)  (raw-ID validation, no-follow Lstat, then fs.ReadFile)
                                               -- only reached per the FR-029 order
```

## Feature-ID Containment and Symlinks

The containment rule is a small predicate over the raw value, evaluated before any
`path.Join`/`path.Clean` call:

```go
func validFeatureID(featureID string) bool {
	return featureID != "." && fs.ValidPath(featureID) && !strings.Contains(featureID, "/")
}
```

This is validation, not sanitization: the input is never trimmed, cleaned, rewritten, or split.
Whitespace-only input has already failed usage validation in `internal/app`; any other value that
fails this predicate is a recognized request whose feature cannot resolve, so evaluators return
the existing `feature_unknown` outcome. `FeatureExists` returns false before filesystem access.
`ResolveRecord` repeats the predicate defensively before it constructs its record path.

For transition evaluation this check belongs to FR-029 step 3 (feature existence), not step 1:
an unsupported candidate target still returns `target_unsupported` at step 2 before feature-ID
validation and before all filesystem access.

| Raw feature ID | Structural result | State request / canonical-target transition | Unsupported-target transition | Filesystem access before outcome |
|---|---|---|---|---|
| `..` | invalid | `feature_unknown` | `target_unsupported` | none |
| `../x` | invalid | `feature_unknown` | `target_unsupported` | none |
| `a/../../x` | invalid | `feature_unknown` | `target_unsupported` | none |
| `/etc/passwd` | invalid | `feature_unknown` | `target_unsupported` | none |
| `a/b` | invalid | `feature_unknown` | `target_unsupported` | none |
| `004-workflow-state` | valid | continue to `specs/004-workflow-state` lookup | `target_unsupported` | lookup only for state/canonical-target branch |

Symlinks are deliberately not followed. Both resolvers require the supplied filesystem to
implement `fs.ReadLinkFS`, then call that interface's `Lstat` directly. `FeatureExists` accepts
only a real directory at `specs/<feature-id>`; a symlink produces `feature_unknown`.
`ResolveRecord` treats a missing record as `draft`, and a symlink or other non-regular entry as
`state_invalid`; only a regular file is read. If `root` lacks `fs.ReadLinkFS`, `FeatureExists`
returns false (and defensive direct `ResolveRecord` returns invalid), so the implementation never
uses the package-level `fs.Lstat` fallback that can follow links. Production `os.DirFS(".")` and
the planned `fstest.MapFS` fixtures both implement `fs.ReadLinkFS`. This protects the
feature-controlled path components without introducing a general filesystem sandbox. The
repository root and its top-level `specs` directory remain the existing trusted `fs.FS` boundary.
Atomic protection against an unrelated process swapping an entry after `Lstat` is outside this
feature's stated concurrency requirements.

## Transition Rule Set as Code

Mirroring `internal/selection`'s `mappedAgent` (an unexported, exhaustive `switch`, not a
mutable map), the fixed FR-004 table is represented as code with no data structure that could be
mutated, persisted, or exposed for external configuration:

```go
// internal/workflow/transition.go
package workflow

// Allowed reports whether the fixed Transition Rule Set (FR-004) contains a
// directed entry from `from` to `to`. This is the complete FR-004 table,
// expressed as code, not data — there is no map, slice, or struct literal
// that could be mutated, and no exported way to reconfigure it.
func Allowed(from, to State) bool {
	switch from {
	case Draft:
		return to == Specified || to == Abandoned
	case Specified:
		return to == Planned || to == Abandoned
	case Planned:
		return to == Tasked || to == Abandoned
	case Tasked:
		return to == Implemented || to == Abandoned
	case Implemented:
		return to == Approved || to == Abandoned
	default: // Approved, Abandoned: terminal, no outgoing entries (FR-003)
		return false
	}
}
```

## Precedence Implementation (FR-029)

The fixed precedence — usage validation, candidate-target canonicality, feature existence,
Workflow State Record resolution, Transition Rule Set evaluation — is implemented as the literal,
top-to-bottom control flow of one function, not as configuration or a separate ordering table
that could drift from the code it describes:

```go
// internal/workflow/evaluate.go
package workflow

// EvaluateTransition resolves one Transition Evaluation Request under the
// fixed precedence order (FR-029). Candidate-target canonicality (step 2) is
// checked and can return before FeatureExists/ResolveRecord (steps 3-4) —
// the only filesystem I/O in this feature — are ever called; usage
// validation (step 1) happens in the caller (RunWorkflowTransition) before
// this function is invoked at all.
func EvaluateTransition(root fs.FS, featureID, suppliedTarget string) TransitionReport {
	target, ok := Recognized(suppliedTarget)
	if !ok {
		return TransitionReport{FeatureID: featureID, SuppliedTarget: suppliedTarget, Outcome: TargetUnsupported}
	}
	// FeatureExists performs raw feature-ID validation before constructing a path.
	if !FeatureExists(root, featureID) {
		return TransitionReport{FeatureID: featureID, Outcome: FeatureUnknown}
	}
	state, invalid := ResolveRecord(root, featureID)
	if invalid {
		return TransitionReport{FeatureID: featureID, Outcome: StateInvalid}
	}
	if Allowed(state, target) {
		return TransitionReport{FeatureID: featureID, CurrentState: state, SuppliedTarget: suppliedTarget, Target: target, Outcome: TransitionValid}
	}
	return TransitionReport{FeatureID: featureID, CurrentState: state, SuppliedTarget: suppliedTarget, Target: target, Outcome: TransitionInvalid}
}

// EvaluateState resolves one state-reporting request. It is unaffected by
// FR-029 (no candidate target): feature existence, then record resolution —
// the same order it would have used before FR-029 existed.
func EvaluateState(root fs.FS, featureID string) StateReport {
	if !FeatureExists(root, featureID) {
		return StateReport{FeatureID: featureID, Outcome: FeatureUnknown}
	}
	state, invalid := ResolveRecord(root, featureID)
	if invalid {
		return StateReport{FeatureID: featureID, Outcome: StateInvalid}
	}
	return StateReport{FeatureID: featureID, State: state, Terminal: Terminal(state), Outcome: StateReported}
}
```

`ResolveRecord` reuses `Recognized` (the same exact-match, case-sensitive function used for
candidate-target validation) to decide whether the record's stored state string counts — an
absent, empty, whitespace-only, wrong-case, or otherwise non-exact value is rejected by the same
single code path that rejects an unsupported candidate target, so I-1's exact-matching rule
cannot drift between the two call sites:

```go
// internal/workflow/record.go
package workflow

const RecordFileName = "workflow-state.json"

// FeatureExists reports whether featureID resolves to a feature root
// recognizes as existing: a real, non-symlink specs/<featureID> directory
// (FR-008). The raw value is validated before path.Join.
func FeatureExists(root fs.FS, featureID string) bool {
	if !validFeatureID(featureID) {
		return false
	}
	lstatRoot, ok := root.(fs.ReadLinkFS)
	if !ok {
		return false // no safe no-follow inspection is available
	}
	info, err := lstatRoot.Lstat(path.Join("specs", featureID))
	return err == nil && info.IsDir() && info.Mode()&fs.ModeSymlink == 0
}

// ResolveRecord determines an existing feature's current Workflow State by
// reading its Workflow State Record (FR-005, FR-006, FR-007). Callers must
// have already confirmed FeatureExists.
func ResolveRecord(root fs.FS, featureID string) (state State, invalid bool) {
	if !validFeatureID(featureID) {
		return "", true // defensive: callers normally stop at FeatureExists
	}
	lstatRoot, ok := root.(fs.ReadLinkFS)
	if !ok {
		return "", true // do not fall back to link-following Stat behavior
	}
	recordPath := path.Join("specs", featureID, RecordFileName)
	info, err := lstatRoot.Lstat(recordPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Draft, false // FR-005: record absent -> initial state
	}
	if err != nil {
		return "", true // FR-007: I/O failure
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", true // FR-007: symlink/non-regular record is present but invalid
	}
	data, err := fs.ReadFile(root, recordPath)
	if err != nil {
		return "", true // FR-007: read failure
	}
	value, ok := stateField(data)
	if !ok {
		return "", true // FR-007: unparseable, or state field absent, duplicated, or not a string
	}
	matched, ok := Recognized(value) // FR-006/FR-007: exact, case-sensitive; rejects empty/whitespace/non-exact
	if !ok {
		return "", true
	}
	return matched, false
}
```

`stateField` (I-1 remediation, human decision) walks the top-level JSON object with
`json.Decoder` tokens instead of `json.Unmarshal` into a struct, because `encoding/json` matches
struct field names case-insensitively (including Unicode folding, e.g. `ſtate`) and resolves
duplicate members last-key-wins. The member name must be exactly `state`; a member that equals
`state` only under case folding (`State`, `STATE`, `ſtate`), or a second exact `state` member,
makes the record invalid. Other members are ignored, and the `state` value must be a JSON string.

## Report Field-Presence Contract (FR-019)

`StateReport` and `TransitionReport` each carry a `Valid()` method, mirroring
`selection.Report.Valid()`, encoding the approved per-outcome field tables as code so the report
writers (`internal/report`) reject a malformed report at render time rather than emit it:

- **Common**: `FeatureID` must be non-empty/non-whitespace for every report. Outcomes outside the
  capability's canonical set are rejected. A structurally invalid `FeatureID` is allowed only for
  `feature_unknown`, and for transition `target_unsupported` because FR-029 has not resolved the
  feature at that point.
- **StateReport / `state_reported`**: `FeatureID` structurally valid; `State` canonical;
  `Terminal == Terminal(State)`. The JSON wire type uses `*bool` so required `false` is rendered.
- **StateReport / `feature_unknown`**: `State == ""`, `Terminal == false`; identifier may be
  structurally invalid or valid-but-absent. **`state_invalid`**: same prohibited fields, but the
  identifier must be structurally valid because feature existence was established first.
- **TransitionReport / `transition_valid` or `transition_invalid`**: structurally valid
  `FeatureID`; canonical `CurrentState` and `Target`; `SuppliedTarget == string(Target)` exactly;
  and `Allowed(CurrentState, Target)` must respectively be true or false.
- **TransitionReport / `target_unsupported`**: non-empty/non-whitespace, noncanonical
  `SuppliedTarget`; `CurrentState == ""`, `Target == ""`; feature ID need not be structurally
  valid due to FR-029 precedence.
- **TransitionReport / `feature_unknown`**: `CurrentState == ""`, `SuppliedTarget == ""`,
  `Target == ""`; identifier may be invalid or valid-but-absent. **`state_invalid`**: the same
  three fields are prohibited, and `FeatureID` must be structurally valid.

Tests construct every valid combination and mutate each required/prohibited field independently,
including noncanonical states/outcomes, terminal/state mismatch, supplied/canonical-target
mismatch, Allowed/outcome mismatch, blank required strings, and fields populated where prohibited.
Both text and JSON writers must reject every invalid report before normal rendering.

## Text Rendering of Feature Identifiers

Every human-readable report renders the feature identifier Go-quoted with `%q`, for all six
outcomes of both commands (I-2 remediation, human decision). Because `target_unsupported` echoes an
identifier that FR-029 has not structurally validated, and because a valid directory name may
itself contain spaces, quotes, backslashes, or newlines, unquoted rendering could split a report
across lines or forge an apparent outcome (FR-013). Quoting keeps every report on exactly one line
with the real outcome token first. JSON rendering is unchanged.

## Report Validation / Write Failure

Following the existing pure-report-command convention (`RunDoctor`, `RunAgentsList`,
`RunAgentSelect`): if report validation or the write itself fails after an outcome has already
been determined, `ai-dev` completes unsuccessfully (exit `2`), writes a diagnostic to stderr, and
does not reclassify the failure as any of the six workflow outcomes. No new outcome is introduced
for this case.

## Regression Protection

This feature must not change the observable behavior of anything built by 001, 002, or 003. The
existing regression suite is the gate; this plan adds only one new, narrowly additive dispatcher
branch:

- **`doctor`, `agents`, `agent run`, `agent select`**: `doctor_test.go`, `agents_test.go`,
  `agentrun_test.go`, `agentselect_test.go`'s existing assertions are unchanged;
  `RunDoctor`/`RunAgentsList`/`RunAgentRun`/`RunAgentSelect` are not modified.
- **Canonical agent IDs, role identifiers, selection outcomes**: `internal/agent`,
  `internal/role`, `internal/selection` are not imported by `internal/workflow` and are not
  modified.
- **`cmd/ai-dev/main_test.go`**: unchanged; `main.go`'s call to `app.Run` needs no new argument.
- **Human Authority (`agent_succeeded != implemented != approved`)**: `internal/workflow` has no
  import of `internal/execution` or `internal/selection`, so it has no way to read a Coding Agent
  Execution Outcome or a Role Selection Outcome, let alone derive a Workflow State from one — this
  is a structural, not just behavioral, guarantee.

## Project Structure

### Documentation (this feature)

```text
specs/004-workflow-state/
├── plan.md                        # This file ($speckit-plan command output)
├── research.md                    # Phase 0 output
├── data-model.md                  # Phase 1 output
├── quickstart.md                  # Phase 1 output
├── contracts/                     # Phase 1 output
│   ├── workflow-state.md
│   ├── workflow-transition.md
│   └── workflow-record.md
├── checklists/
│   └── requirements.md            # Already present (Human Specification Gate input) — unchanged
└── tasks.md                       # Phase 2 output ($speckit-tasks command — NOT created by $speckit-plan)
```

### Source Code (repository root)

```text
cmd/ai-dev/
├── main.go                        # UNCHANGED — app.Run's signature does not change
└── main_test.go                   # UNCHANGED assertions; MAY gain new dispatch cases (additive only)

internal/app/
├── doctor.go                      # MODIFIED: one new `if args[0] == "workflow"` branch added
│                                   #   beside the existing "agents"/"agent" branches; the
│                                   #   top-level usage string is updated to name "workflow" as a
│                                   #   fourth subcommand in BOTH existing occurrences: the
│                                   #   len(args)==0 branch and the args[0]!="doctor" fallback.
│                                   #   Both must remain byte-identical to each other. RunDoctor
│                                   #   and every other branch are otherwise unchanged; no
│                                   #   dispatcher refactor is introduced.
├── doctor_test.go                 # UNCHANGED (regression gate)
├── agents.go / agents_test.go     # UNCHANGED (regression gate)
├── agentrun.go / agentrun_test.go # UNCHANGED (regression gate)
├── agentselect.go / agentselect_test.go # UNCHANGED (regression gate)
├── workflowstate.go               # NEW: RunWorkflowState(args []string, root fs.FS, stdout,
│                                   #   stderr io.Writer) int
├── workflowstate_test.go          # NEW
├── workflowtransition.go          # NEW: RunWorkflowTransition(args []string, root fs.FS,
│                                   #   stdout, stderr io.Writer) int
└── workflowtransition_test.go     # NEW

internal/workflow/
├── state.go                       # NEW: State type, seven canonical constants, Recognized,
│                                   #   Terminal
├── state_test.go                  # NEW
├── transition.go                  # NEW: Allowed(from, to State) bool — exhaustive switch
├── transition_test.go             # NEW
├── record.go                      # NEW: RecordFileName, FeatureExists, ResolveRecord
├── record_test.go                 # NEW
├── outcome.go                     # NEW: Outcome type, six constants, StateReport,
│                                   #   TransitionReport
├── evaluate.go                    # NEW: EvaluateState, EvaluateTransition (FR-029 precedence)
├── evaluate_test.go                # NEW — includes the recording-fs.FS proof that
│                                    #   target_unsupported never calls Open
└── validate.go / validate_test.go  # NEW: StateReport.Valid(), TransitionReport.Valid()

internal/report/
├── roleselection.go / roleselection_test.go # UNCHANGED (regression gate)
├── workflowstate.go                 # NEW: WriteWorkflowStateText, WriteWorkflowStateJSON
├── workflowstate_test.go            # NEW
├── workflowtransition.go            # NEW: WriteWorkflowTransitionText, WriteWorkflowTransitionJSON
└── workflowtransition_test.go       # NEW

internal/agent/, internal/role/, internal/selection/, internal/execution/  # UNCHANGED —
    # not imported by internal/workflow; no agent, role, or execution capability is reachable
    # from this feature
internal/checks/, internal/doctor/, internal/instruction/, internal/system/ # UNCHANGED
```

**Structure Decision**: Single Go module, extending the existing flat `cmd/` + `internal/<domain>`
layout — no new top-level directory. `internal/workflow` is a new, narrow domain package
analogous to `internal/role`: a vendor-neutral model (Workflow State, Transition Rule Set,
Workflow State Record resolution) with zero dependency on `internal/agent`, `internal/role`, or
`internal/selection`, so there is no import path by which a coding-agent or role concept could
leak into it, matching the spec's "conceptually separate from agent execution, agent
availability, role selection" requirement at the architecture level.

## Complexity Tracking

*No entries — Constitution Check recorded no violations requiring justification.*

## Constitution Check — Post-Design

*Re-checked after Phase 1 design (data-model.md, contracts/, quickstart.md below).*

| Principle | Assessment |
|---|---|
| I. Specification First | Unchanged: design artifacts implement exactly the approved FR/outcome/precedence contracts; no new behavior was introduced during design. PASS |
| II. Architecture Before Framework | Confirmed by `data-model.md` and the contracts: every type and function traces directly to a spec entity or FR; `internal/workflow`'s isolation from `internal/agent`/`internal/role`/`internal/selection` is a design-time property, not an afterthought. PASS |
| III. Simplicity and YAGNI | `contracts/workflow-record.md`'s file-format decision (one JSON file, one field) is the minimum needed to satisfy FR-027; no schema versioning, no migration path, no configuration was added beyond what reading one fixed field requires. PASS |
| IV. Testable by Design | `quickstart.md` demonstrates every outcome end-to-end through the real CLI binary and now takes its before/after filesystem snapshots at the runnable point of comparison. The four FR-029 combinations, C-1 invalid-ID matrix, symlink policy, exhaustive report-validity combinations, and "no record read" proof each have explicit automated seams. PASS |
| V. Security by Design | Raw feature IDs are validated before path construction, traversal/nested/absolute inputs produce `feature_unknown` with zero filesystem access, FR-029 still returns `target_unsupported` earlier, and feature/record inspection requires `fs.ReadLinkFS` so symlinks are rejected rather than followed. A filesystem that cannot provide no-follow metadata is rejected. Together with the read-only `fs.FS` dependency and scope-audit checklist, this closes C-1 without adding a sanitizer framework or write capability. PASS |
| VI. Observability by Design | Unchanged; N/A. |
| VII. Reproducibility and Automation | Unchanged; Quality Gates above apply unmodified to the designed file set. PASS |
| VIII. AI-Assisted Engineering | Unchanged. PASS |
| IX. Human Authority | Unchanged: this plan does not claim Human Plan Gate approval, does not generate `tasks.md`, and does not implement code. PASS |

No complexity exception required.
