---

description: "Task list for 004-workflow-state implementation"
---

# Tasks: Workflow State

**Input**: Design documents from `/specs/004-workflow-state/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/workflow-state.md,
contracts/workflow-transition.md, contracts/workflow-record.md, quickstart.md (this task list was
generated only after the human stated explicit approval of the Human Specification Gate and the
Human Plan Gate in the controlling session; this file is not itself an approval record and does not
approve any Human Gate, including the Human Task Gate)

**Tests**: Included. The plan's Project Structure explicitly names every test file below as a
required deliverable, and the constitution's Testable by Design / IV principle applies.

**Organization**: Tasks are grouped by user story (spec.md: US1 P1, US2 P2, US3 P3) so each can be
implemented and verified independently, per the project's established `internal/<domain>` +
`internal/app` + `internal/report` layering (mirroring `internal/role`/`internal/selection` from
003).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no unmet dependency)
- **[Story]**: US1, US2, or US3 — omitted for Setup/Foundational/Polish tasks
- Every task names its exact file path

## Path Conventions

Single Go module, existing `cmd/ai-dev/` + `internal/<domain>/` layout (see plan.md "Project
Structure"). No new top-level directory.

---

## Phase 1: Setup

**Purpose**: Confirm the existing toolchain is ready; this feature adds no new dependency.

- [X] T001 Confirm `go.mod` needs no change (Go stdlib only: `flag`, `io/fs`, `path`,
  `encoding/json`, `errors`, `strings`, `fmt`, `io` — plan.md Technical Context) and that
  `go build ./...`, `gofmt -l .`, and `go vet ./...` all succeed cleanly on the current tree
  before any new file is added.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared `internal/workflow` domain model — the canonical state vocabulary, the
fixed Transition Rule Set, the outcome/report types, Workflow State Record resolution (including
the C-1 feature-ID containment rule and the symlink no-follow rule), and the report-validity
invariants. Both `ai-dev workflow state` (US1) and `ai-dev workflow transition` (US2) depend on
this being complete and correct — none of it is divisible across user stories.

**⚠️ CRITICAL**: No user story task may begin until this phase is complete.

Within this phase, `[P]` marks a production file whose shape is already fully fixed by
`data-model.md`/`plan.md` and so can be drafted concurrently with its peers even though final
compilation requires its noted prerequisite to land first (the same drafting-time convention used
by the Tests-first sections in later phases); a `[P]`-free paired test file is always
sequential-after its own implementation file, never run concurrently with it.

- [X] T002 [P] Create `internal/workflow/state.go`: `type State string`; exactly seven canonical
  constants `Draft State = "draft"`, `Specified State = "specified"`, `Planned State = "planned"`,
  `Tasked State = "tasked"`, `Implemented State = "implemented"`, `Approved State = "approved"`,
  `Abandoned State = "abandoned"` (FR-002); `func Recognized(value string) (State, bool)` matching
  **exactly and case-sensitively, with no trimming, normalization, aliasing, case folding, or
  abbreviation** (FR-002/FR-006, data-model.md); `func Terminal(s State) bool` returning `true`
  only for `Approved` and `Abandoned` (FR-003) — `Implemented` MUST remain non-terminal.
- [X] T003 Create `internal/workflow/state_test.go` (depends on T002 — sequential, not drafted in
  parallel with it): table test asserting all seven
  `Recognized` matches succeed; `"Planned"`, `" planned "`, `""`, `"   "`, and an unknown string
  all fail with no normalization; `Terminal` is `true` only for `Approved`/`Abandoned` and `false`
  for all five other states including `Implemented`.
- [X] T004 [P] Create `internal/workflow/transition.go` (depends on T002 for the `State` type — may
  be drafted concurrently against data-model.md's fixed `State` definition, but will not compile
  until T002 lands): `func Allowed(from, to State) bool`, an
  exhaustive `switch` (no map, mirroring `internal/selection`'s `mappedAgent`) implementing exactly
  the FR-004 table from plan.md "Transition Rule Set as Code": `Draft→{Specified,Abandoned}`,
  `Specified→{Planned,Abandoned}`, `Planned→{Tasked,Abandoned}`, `Tasked→{Implemented,Abandoned}`,
  `Implemented→{Approved,Abandoned}`, and `default: return false` for `Approved`/`Abandoned`
  (terminal, no outgoing entries, FR-003).
- [X] T005 Create `internal/workflow/transition_test.go` (depends on T004 — sequential): table
  test covering all ten valid
  edges (`Allowed(from,to)==true`) and representative invalid cases — skip-ahead
  (`Specified→Tasked`), backward move (`Implemented→Specified`), same-state
  (`Planned→Planned`), and both terminal origins (`Approved→*`, `Abandoned→*`) — all
  `Allowed(...)==false`.
- [X] T006 [P] Create `internal/workflow/outcome.go` (depends on T002 for the `State` type — may be
  drafted concurrently against data-model.md's fixed `State` definition, but will not compile
  until T002 lands): `type Outcome string` with exactly six
  constants `StateReported = "state_reported"`, `FeatureUnknown = "feature_unknown"`,
  `StateInvalid = "state_invalid"`, `TransitionValid = "transition_valid"`,
  `TransitionInvalid = "transition_invalid"`, `TargetUnsupported = "target_unsupported"` (FR-013);
  `type StateReport struct { FeatureID string; State State; Terminal bool; Outcome Outcome }`;
  `type TransitionReport struct { FeatureID string; CurrentState State; SuppliedTarget string;
  Target State; Outcome Outcome }` (data-model.md).
- [X] T007 Create `internal/workflow/record.go` (depends on T002 — sequential, not `[P]`: it starts
  only after T002 has landed and is not drafted concurrently with any other task): `const RecordFileName =
  "workflow-state.json"`; `func validFeatureID(featureID string) bool { return featureID != "." &&
  fs.ValidPath(featureID) && !strings.Contains(featureID, "/") }` (plan.md "Feature-ID Containment
  and Symlinks" — this is validation, not sanitization: never trim/clean/rewrite the input);
  `func FeatureExists(root fs.FS, featureID string) bool` returning `false` immediately when
  `!validFeatureID(featureID)` or when `root` does not implement `fs.ReadLinkFS`, otherwise calling
  that interface's `Lstat(path.Join("specs", featureID))` and requiring
  `err == nil && info.IsDir() && info.Mode()&fs.ModeSymlink == 0`; `func ResolveRecord(root fs.FS,
  featureID string) (state State, invalid bool)` that repeats the same `validFeatureID`/
  `fs.ReadLinkFS` guard defensively, calls `Lstat` on `path.Join("specs", featureID,
  RecordFileName)`, returns `(Draft, false)` when that `Lstat` error `errors.Is(err,
  fs.ErrNotExist)`, returns `("", true)` for any other `Lstat` error or when the entry is a
  symlink or not a regular file, otherwise calls `fs.ReadFile` on the now-confirmed regular file,
  reads the top-level `state` string member (I-1 remediation: exact, case-sensitive member name;
  a case-folding variant or a duplicated `state` member is invalid — no `json.Unmarshal` into a
  struct, no last-key-wins), and returns `("", true)` unless `Recognized` succeeds — exactly per plan.md's `record.go` listing and contracts/workflow-record.md's Reading
  Rules table. No function in this file ever writes, creates, deletes, or renames anything.
- [X] T008 Create `internal/workflow/record_test.go` (depends on T007):
  - An `fstest.MapFS`-based table test covering every row of contracts/workflow-record.md's
    Reading Rules table: absent file → `(Draft, false)`; absent/`null`/empty/whitespace-only
    `state` field → invalid; non-exact case or padded value (e.g. `"Planned"`, `" planned "`) →
    invalid; non-canonical value → invalid; structurally unparseable JSON → invalid; a symlink or
    non-regular entry at the record path → invalid **without the content ever being read**; and a
    valid fixture with an unrecognized extra field, `{"state":"planned","extra":"ignored"}` →
    resolves successfully exactly as a plain `{"state":"planned"}` record would (`(Planned,
    false)`), proving unknown JSON fields are silently ignored per contracts/workflow-record.md's
    schema note and that no `json.Decoder.DisallowUnknownFields()`-style rejection is introduced.
  - Two genuine I/O-error-other-than-not-exist cases, each constructed via a small, test-only
    `fs.FS` + `fs.ReadLinkFS` wrapper (defined only in this test file, not a new production
    filesystem abstraction) around the `fstest.MapFS` fixture, each asserting `ResolveRecord`
    returns `("", true)`: (a) the wrapper injects a non-`fs.ErrNotExist` error from `Lstat` for the
    record path; (b) the wrapper's `Lstat` succeeds and reports a regular file for the record path,
    but its `Open` (and therefore `fs.ReadFile`) returns a non-`fs.ErrNotExist` error for that path
    — exercising the post-`Lstat` read-failure branch. Both are distinguished from the
    "non-regular entry" and "malformed JSON" rows they would otherwise be indistinguishable from
    using `fstest.MapFS` alone.
  - A capability-rejection case: a test-only wrapper that exposes **only** `fs.FS` (`Open`) and
    deliberately does **not** implement `fs.ReadLinkFS`, wrapping an `fstest.MapFS` that does
    contain a real `specs/004-workflow-state` directory and a valid record — asserting
    `FeatureExists(...) == false` and `ResolveRecord(...) == ("", true)`, proving such a filesystem
    is rejected rather than inspected through a link-following fallback (`fs.Stat`/`fs.Lstat`
    package helpers). No production abstraction or fallback is introduced by this test.
  - The C-1 raw-feature-ID table from plan.md "Feature-ID Containment and Symlinks" run through
    `validFeatureID`/`FeatureExists`/`ResolveRecord` directly: `"."`, `".."`, `"../x"`,
    `"a/../../x"`, `"/etc/passwd"`, and `"a/b"` each → `validFeatureID` returns `false`,
    `FeatureExists` returns `false`, **and** `ResolveRecord` returns `("", true)` — each of the two
    resolver calls with **zero** `fs.FS` method calls (prove via a call-recording wrapper that
    implements and forwards `fs.ReadLinkFS`, so a zero count is not caused by capability
    rejection); `""` → `validFeatureID` also returns `false`, `FeatureExists` returns `false`, and
    `ResolveRecord` returns `("", true)`, each with zero `fs.FS` method calls, asserted here only
    as defensive coverage of
    these functions in isolation — the real CLI path never reaches this input, since
    `RunWorkflowState`/`RunWorkflowTransition`'s usage validation (T013/T021) already intercepts
    an empty or whitespace-only feature-id as a usage failure before `workflow.EvaluateState`/
    `EvaluateTransition` is ever called; and `"004-workflow-state"` as the valid positive control
    proceeds to a real lookup — assert, via the same call-recording wrapper, that the **exact**
    path accessed is `specs/004-workflow-state`, not merely that some lookup occurred.
  - A symlinked `specs/<id>` directory (via `fstest.MapFS`) → `FeatureExists` returns `false`
    without the symlink being followed.
  - In addition to the `fstest.MapFS` coverage above (which remains required, not replaced), one
    real-filesystem test using `os.DirFS(t.TempDir())` and a real `os.Symlink` — mirroring
    `agentselect_test.go`'s `TestSnapshotDirDetectsSymlinkTargetChange` pattern, including its
    `t.Skipf` fallback where symlink creation is unsupported in the test environment —
    independently corroborating that `FeatureExists` treats a real symlinked feature directory as
    not existing and `ResolveRecord` treats a real symlinked record as invalid, without following
    either.
- [X] T009 [P] Create `internal/workflow/validate.go` (depends on T002, T004, T006, **and T007**
  for the unexported `validFeatureID` predicate — may be drafted concurrently with T002/T004/T006
  against data-model.md, but will not compile until T002, T004, T006, and T007 have all landed):
  `StateReport.Valid() bool` and `TransitionReport.Valid() bool` implementing exactly the
  per-outcome rules in data-model.md — `FeatureID` always non-empty/non-whitespace for every
  report; **structural `FeatureID` validity is checked by calling T007's existing
  `validFeatureID` (never a duplicated or redefined algorithm)** and is REQUIRED for
  `state_reported`, `state_invalid` (StateReport), `transition_valid`, `transition_invalid`, and
  `state_invalid` (TransitionReport); it is NOT required for `feature_unknown` (either report
  type), which may carry a structurally invalid non-empty `FeatureID`, nor for
  `target_unsupported`, which FR-029 resolves before any feature-ID validation;
  `state_reported` requires `State` canonical and `Terminal == Terminal(State)` exactly;
  `feature_unknown`/`state_invalid` (StateReport) require `State == ""` and `Terminal == false`;
  `transition_valid`/`transition_invalid` require canonical `CurrentState` and `Target`,
  `SuppliedTarget == string(Target)` exactly, and `Allowed(CurrentState, Target)` matching the
  outcome; `target_unsupported` requires a non-empty, non-whitespace, **non-canonical**
  `SuppliedTarget` with `CurrentState == ""` and `Target == ""`; `feature_unknown`/`state_invalid`
  (TransitionReport) require `CurrentState == ""`, `SuppliedTarget == ""`, `Target == ""`; every
  outcome outside each capability's own canonical set is rejected.
- [X] T010 Create `internal/workflow/validate_test.go` (depends on T009 and T007 — sequential, not
  run in parallel with T009): construct every
  valid `StateReport`/`TransitionReport` combination, then independently mutate each
  required/prohibited field (a noncanonical `State`/`CurrentState`/`Target`, a `Terminal`/`State`
  mismatch, a `SuppliedTarget`/`Target` mismatch, an `Allowed`/outcome mismatch, a blank
  `FeatureID`, a field populated where the outcome prohibits it, and an outcome outside the
  capability's canonical set), asserting `Valid()` rejects every mutation and accepts only the
  exact approved combinations. Structural-`FeatureID` cases: mutating an otherwise-valid
  `state_reported`, `state_invalid` (both report types), `transition_valid`, or
  `transition_invalid` report to each structurally invalid non-empty ID (`".."`, `"../x"`,
  `"a/../../x"`, `"/etc/passwd"`, `"a/b"`, `"."`) makes `Valid()` return `false`; the same IDs on an
  otherwise-valid `feature_unknown` report (both report types) and on an otherwise-valid
  `target_unsupported` report keep `Valid()` returning `true`.

**Checkpoint**: `internal/workflow` core is complete and independently unit-tested. Both user
stories below may now proceed.

---

## Phase 3: User Story 1 - Report the current workflow state of a feature (Priority: P1) 🎯 MVP

**Goal**: `ai-dev workflow state [--format text|json] <feature-id>` reports `state_reported`
(with the recorded or defaulted `draft` state and its terminal flag) for a recognized feature,
without applying or changing anything (spec FR-001, FR-005, FR-006, FR-009, FR-019, FR-022,
FR-028).

**Independent Test**: Build the binary, create a `specs/<id>` directory with no record (expect
`state_reported`/`draft`), then write a record naming each canonical state and confirm each is
reported exactly, including the terminal flag — all without writing anything back (quickstart.md
§§1–3).

### Tests for User Story 1 ⚠️ (write first; expect them to fail to compile/pass until the
Implementation tasks below exist)

- [X] T011 [P] [US1] Write the state-reporting cases in `internal/workflow/evaluate_test.go`
  (shared file: US2's T019 and US3's T027/T028 also write this file and are sequential-after T011;
  `[P]` here pairs T011 only with T012/T013) against the planned signature
  `func EvaluateState(root fs.FS, featureID string) StateReport`:
  an `fstest.MapFS` fixture naming each of the seven canonical states → `state_reported` with the
  matching `State`/`Terminal`; a feature directory with no record → `state_reported`/`Draft`/
  `Terminal==false` (FR-005); two identical calls against unchanged fixture content produce a
  byte-identical `StateReport` (FR-018); the fixture is unmodified after the call.
- [X] T012 [P] [US1] Write `internal/report/workflowstate_test.go` against the planned signatures
  `WriteWorkflowStateText(w io.Writer, r workflow.StateReport) error` and
  `WriteWorkflowStateJSON(w io.Writer, r workflow.StateReport) error`: exact text line
  `ai-dev: outcome=state_reported feature=<id, quoted> state=<state> terminal=<true|false>` and
  `ai-dev: outcome=feature_unknown|state_invalid feature=<id as supplied, quoted>` per
  contracts/workflow-state.md (feature Go-quoted for every outcome — I-2 remediation; originally
  `state_reported` was unquoted); exact JSON object per outcome, verified by decoding
  the first JSON value and then performing a second `Decoder.Decode` call requiring it return
  `io.EOF` (never `Decoder.More()`) to prove no trailing content, and by asserting the exact key
  set via an `assertExactJSONFields`-equivalent check (every required key present with the exact
  value, every prohibited/unexpected key absent) — the check compares exact keys **and exact typed
  values** over heterogeneous JSON values (e.g. `map[string]any`, so JSON boolean `false` is
  distinguishable from the string `"false"` and from an absent key); it MUST NOT be limited to
  the existing `map[string]string` helper signature, which cannot represent booleans — including
  a `State: workflow.Planned` fixture
  proving the JSON encodes `"terminal":false` rather than omitting the key (research.md §6); an
  invalid `StateReport` (e.g. `state_reported` with a noncanonical `State`) is rejected by both
  writers via `Valid()` rather than rendered.
- [X] T013 [P] [US1] Write `internal/app/workflowstate_test.go` against the planned signature
  `RunWorkflowState(args []string, root fs.FS, stdout, stderr io.Writer) int`:
  - Every usage-failure case — no feature-id, whitespace-only feature-id, extra positional
    argument, unknown flag, `--format` value other than `text`/`json`, and `--format` supplied
    with no following value (distinct from a bad value; mirrors `agentselect_test.go`'s
    `{"format flag missing value", []string{"agent", "select", "--format"}}`) — asserts exit `2`,
    empty stdout, a usage message on stderr, **zero** `fs.FS` access via a call-recording wrapper
    (mirroring `TestRunAgentSelectUsageFailuresAreDistinctFromRoleUnsupported`), and no semantic
    report rendered in either stream. The call-recording wrapper MUST implement and forward
    `fs.ReadLinkFS` (`Lstat`/`ReadLink`) when the wrapped filesystem supports it, recording those
    calls as well as `Open`, so production code never rejects the wrapper for lacking no-follow
    capability and a zero count genuinely means no access; a positive control through the same
    wrapper (a valid `state_reported` request) records a non-zero access count.
  - For **each** of the three outcomes (`state_reported`, `feature_unknown`, `state_invalid`) in
    both text and JSON mode: the correct exit code (`0` only for `state_reported`); the report on
    stdout with `stderr` empty; and a dedicated filesystem-non-mutation assertion for that specific
    outcome, reusing `snapshotDir`/`controlledTempDir` from `agentselect_test.go` — one assertion
    per outcome, mirroring `agentselect_test.go`'s separate
    `TestRunAgentSelect*DoesNotMutateFilesystem` tests, not a single generic check.
  - JSON-shape verification reuses the decode-twice-expect-`io.EOF` and exact-key-set techniques
    required by T012.
  - Determinism across repeated identical calls.
  - An always-erroring `io.Writer` asserts exit `2`, a stderr diagnostic, and no outcome
    reclassification.

### Implementation for User Story 1

- [X] T014 [US1] Implement `EvaluateState` in `internal/workflow/evaluate.go` (depends on T007,
  T006; makes T011 pass): call `FeatureExists`, returning `StateReport{FeatureID: featureID,
  Outcome: FeatureUnknown}` if false; otherwise call `ResolveRecord`, returning
  `StateReport{FeatureID: featureID, Outcome: StateInvalid}` if invalid; otherwise return
  `StateReport{FeatureID: featureID, State: state, Terminal: Terminal(state), Outcome:
  StateReported}` — exactly as in plan.md's "Precedence Implementation (FR-029)" listing.
- [X] T015 [US1] Implement `internal/report/workflowstate.go` (depends on T009; makes T012 pass):
  `WriteWorkflowStateText`/`WriteWorkflowStateJSON`, each calling `r.Valid()` first and returning
  an error if it fails; JSON wire type uses `*bool` for `terminal` (populated only for
  `state_reported`) so `omitempty` never drops a required `false` (research.md §6).
- [X] T016 [US1] Implement `RunWorkflowState` in `internal/app/workflowstate.go` (depends on T014,
  T015; makes T013 pass): `flag.NewFlagSet` with `--format` (default `text`); usage failure (exit
  `2`, no `fs.FS` access, no report) when `flags.Parse` errors, `flags.NArg() != 1`, `--format` is
  neither `text` nor `json`, or `strings.TrimSpace(flags.Arg(0)) == ""`; otherwise call
  `workflow.EvaluateState`, write the selected format, return `0` iff `Outcome == StateReported`,
  `1` for `FeatureUnknown`/`StateInvalid`, `2` with a stderr diagnostic (no reclassification) on a
  write error.
- [X] T017 [US1] Modify `internal/app/doctor.go`'s `Run`: add one `if args[0] == "workflow"` branch
  immediately after the existing `"agent"` branch, dispatching `len(args) >= 2 && args[1] ==
  "state"` to `RunWorkflowState(args[2:], root, stdout, stderr)`, falling through to `fmt.Fprintln(stderr,
  "Usage: ai-dev workflow <state|transition> [options]"); return 2` otherwise. Update **both**
  existing occurrences of the top-level usage string (the `len(args)==0` branch and the
  `args[0] != "doctor"` fallback) to list `workflow` as a fourth subcommand, keeping the two
  strings byte-identical to each other. Do not otherwise alter `RunDoctor` or the `agents`/`agent`
  branches; do not refactor the dispatcher's `if`-chain shape.
- [X] T018 [P] [US1] Add dispatcher-level assertions (in `internal/app/workflowstate_test.go`)
  proving `Run(ctx, []string{"workflow","state",...}, ...)` reaches `RunWorkflowState` with correct
  outcome/exit-code/stream placement, and that the two usage-string occurrences updated in T017 are
  byte-identical and both name `workflow`. Also assert top-level workflow dispatch usage failures:
  `Run(ctx, []string{"workflow"}, ...)` and `Run(ctx, []string{"workflow", "bogus"}, ...)` each exit
  `2`, write the `Usage: ai-dev workflow <state|transition> [options]` diagnostic to stderr, leave
  stdout empty of any semantic report, and perform zero `root` filesystem access (via the same
  `fs.ReadLinkFS`-forwarding call-recording wrapper as T013). Existing `doctor`/`agents`/`agent`
  behavior is preserved by leaving their tests unmodified (T032).

**Checkpoint**: `ai-dev workflow state` is fully functional, independently tested, and ships as
the MVP.

---

## Phase 4: User Story 2 - Evaluate whether a specific requested transition would be valid (Priority: P2)

**Goal**: `ai-dev workflow transition [--format text|json] <feature-id> <target-state>` evaluates,
without applying it, whether a candidate transition is valid — implementing the exact FR-029
precedence (usage validation → candidate-target canonicality → feature existence → record
resolution → Transition Rule Set evaluation) confirmed by two independent plan reviews (spec
FR-010–FR-013, FR-019, FR-029).

**Independent Test**: Against a feature already in a known state, request evaluation of a
reachable target (`transition_valid`), an unreachable one (`transition_invalid`, including
terminal-origin, skip-ahead, backward, and same-state cases), and a noncanonical target
(`target_unsupported` — including against an unknown feature and an invalid record, both
producing `target_unsupported` with zero filesystem access) — quickstart.md §§5–8.

### Tests for User Story 2 ⚠️ (write first)

- [X] T019 [P] [US2] Write the transition-evaluation cases in `internal/workflow/evaluate_test.go`
  (same file as US1's T011 — **depends on T011 and is sequential-after it, never edited
  concurrently with it**, even if US1 and US2 otherwise proceed in parallel; `[P]` here pairs
  T019 only with T020/T021) against the planned signature `func EvaluateTransition(root fs.FS, featureID, suppliedTarget
  string) TransitionReport`: all ten valid Transition Rule Set edges → `transition_valid`; every
  invalid skip-ahead, backward-move, same-state, and both terminal-origin (`Approved`,
  `Abandoned`) case → `transition_invalid`; a call-recording wrapper proving **zero** `fs.FS`
  method calls (`Open`, `Lstat`, `ReadLink`, or any other) occur whenever `suppliedTarget` is
  noncanonical, regardless of whether the wrapped fixture has a feature directory or record for
  the supplied id (FR-011/FR-029) — the wrapper MUST implement and forward `fs.ReadLinkFS` when the
  wrapped filesystem supports it (so it is never rejected for lacking no-follow capability before
  the behavior under test is reached), and a positive control through the same wrapper records
  non-zero access; the four-combination table from contracts/workflow-transition.md run directly
  against `EvaluateTransition` (unsupported target + unknown feature → `target_unsupported`;
  unsupported target + invalid/unreadable record → `target_unsupported`; canonical target +
  unknown feature → `feature_unknown`; canonical target + invalid/unreadable record →
  `state_invalid`); the C-1 raw-feature-ID table (`..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`)
  paired with a canonical target → `feature_unknown` with **zero** filesystem access, and paired
  with a noncanonical target → `target_unsupported` with zero filesystem access; the valid
  positive control `004-workflow-state` (fixture contains that real directory and a valid record)
  paired with a canonical target → proceeds to lookup (recorded access includes exactly
  `specs/004-workflow-state`) and a `transition_valid`/`transition_invalid` outcome, and paired with
  a noncanonical target → `target_unsupported` with zero filesystem access; determinism.
- [X] T020 [P] [US2] Write `internal/report/workflowtransition_test.go` against the planned
  signatures `WriteWorkflowTransitionText`/`WriteWorkflowTransitionJSON`: exact text/JSON per all
  five outcomes per contracts/workflow-transition.md (feature Go-quoted for every outcome — I-2
  remediation; originally unquoted for `transition_valid`/`transition_invalid`/
  `target_unsupported`; `target_unsupported`'s target always quoted;
  `current_state`/`target` present only for `transition_valid`/`transition_invalid`;
  `supplied_target` present only for `target_unsupported`); JSON verification for every outcome
  uses the same decode-twice-expect-`io.EOF` (never `Decoder.More()`) and exact-key-set
  (`assertExactJSONFields`-equivalent, exact keys and exact typed values over heterogeneous JSON
  values, not a `map[string]string`-only signature) techniques required by T012; a `Valid()`-rejection test
  proving `feature_unknown`/`state_invalid` never render `supplied_target`/`current_state`/`target`
  even if a malformed `TransitionReport` happens to carry a non-empty `SuppliedTarget`.
- [X] T021 [P] [US2] Write `internal/app/workflowtransition_test.go` against the planned signature
  `RunWorkflowTransition(args []string, root fs.FS, stdout, stderr io.Writer) int`:
  - Every usage-failure case — missing/whitespace feature-id or target, zero/multiple extra
    positionals, unknown flag, `--format` value other than `text`/`json`, and `--format` supplied
    with no following value (distinct from a bad value) — asserts exit `2`, empty stdout, a usage
    message on stderr, zero `fs.FS` access, and no semantic report rendered in either stream. The
    call-recording wrapper follows the same rule as T013/T019: it implements and forwards
    `fs.ReadLinkFS` when the wrapped filesystem supports it, records every method call, and has a
    positive control showing non-zero access for a valid request.
  - FR-029 step 1 before step 2: usage failures combined with a **non-canonical** target —
    `--format xml 004-workflow-state reviewing`, `004-workflow-state reviewing extra`, and
    `"   " reviewing` — each assert usage failure wins over `target_unsupported`: exit `2`, a usage
    diagnostic on stderr, empty stdout, no `target_unsupported` (or any other semantic) report in
    either stream, and zero filesystem access.
  - For **each** of the five outcomes in both text and JSON mode: the correct exit code (`0` only
    for `transition_valid`); the report on stdout with `stderr` empty; and a dedicated
    filesystem-non-mutation assertion for that specific outcome (not one generic check), reusing
    `snapshotDir`/`controlledTempDir`.
  - The two precedence combinations exercised end-to-end through the public CLI with an
    `fstest.MapFS` fixture containing no feature directory at all, and a separate fixture with a
    malformed record, each paired with a noncanonical target, confirming `target_unsupported` at
    the application boundary.
  - JSON-shape verification reuses the decode-twice-expect-`io.EOF` and exact-key-set techniques
    required by T020.
  - Determinism across repeated identical calls; an always-erroring `io.Writer` asserts exit `2`,
    a stderr diagnostic, and no outcome reclassification.

### Implementation for User Story 2

- [X] T022 [US2] Implement `EvaluateTransition` in `internal/workflow/evaluate.go` (same file as
  T014; depends on T014, T007, T006, T004; makes T019 pass): `Recognized(suppliedTarget)` first,
  returning `TransitionReport{FeatureID: featureID, SuppliedTarget: suppliedTarget, Outcome:
  TargetUnsupported}` immediately if it fails (no `FeatureExists`/`ResolveRecord` call at all);
  otherwise `FeatureExists` → `FeatureUnknown` if false; otherwise `ResolveRecord` → `StateInvalid`
  if invalid; otherwise `Allowed(state, target)` selects `TransitionValid`/`TransitionInvalid` —
  exactly per plan.md's listing.
- [X] T023 [US2] Implement `internal/report/workflowtransition.go` (depends on T009; makes T020
  pass): both writers call `r.Valid()` first and reject an invalid report before rendering.
- [X] T024 [US2] Implement `RunWorkflowTransition` in `internal/app/workflowtransition.go`
  (depends on T022, T023; makes T021 pass): two required positional arguments (`<feature-id>`,
  `<target-state>`); usage failure when parsing fails, `flags.NArg() != 2`, `--format` is invalid,
  or either positional is empty/whitespace-only; otherwise call `workflow.EvaluateTransition`,
  write the report, return `0` iff `TransitionValid`, `1` for the other four outcomes, `2` with a
  stderr diagnostic on a write failure.
- [X] T025 [US2] Extend the `"workflow"` dispatcher branch added in T017
  (`internal/app/doctor.go`) with `args[1] == "transition"` dispatching to
  `RunWorkflowTransition(args[2:], root, stdout, stderr)` (depends on T017, T024). No other
  dispatcher logic changes.
- [X] T026 [P] [US2] Add dispatcher-level assertions (in `internal/app/workflowtransition_test.go`)
  proving `Run(ctx, []string{"workflow","transition",...}, ...)` reaches `RunWorkflowTransition`
  correctly (depends on T025).

**Checkpoint**: Both `ai-dev workflow state` and `ai-dev workflow transition` are fully functional
and independently tested.

---

## Phase 5: User Story 3 - Understand clearly when a feature or its recorded state cannot be determined (Priority: P3)

**Goal**: Confirm — with dedicated, explicit test coverage across *both* commands — that an
unrecognized feature identifier always produces `feature_unknown` and a feature whose Workflow
State Record cannot be matched to a canonical value always produces `state_invalid`, never a
guessed or defaulted value, and never anything resembling a success outcome (spec FR-007, FR-008,
FR-013; User Story 3's own "Why this priority": *"prioritized after the core reporting and
evaluation paths because it is a correctness guard... rather than a separate capability"*).

**Independent Test**: Request state/transition against a nonexistent feature and against a feature
with a deliberately malformed record (wrong case, empty field, unparseable JSON, symlinked
record), confirming `feature_unknown`/`state_invalid` in both capabilities with no
`draft`/guessed substitution (quickstart.md §§2, 4).

**Note**: `FeatureExists`/`ResolveRecord`/`EvaluateState`/`EvaluateTransition` (Phases 2–4) already
implement `feature_unknown`/`state_invalid` completely — these outcomes are not separable from the
functions that produce `state_reported`/`transition_valid`. This phase adds the dedicated,
exhaustive test coverage the spec's own prioritization calls for; it is not expected to require
new production code. If any task below fails, it indicates a defect in the Phase 2–4
implementation, not a missing capability — fix the defect in `internal/workflow/record.go` or
`evaluate.go` rather than introducing a new outcome, a new usage-failure category, or any change
to the FR-029 precedence order.

### Tests for User Story 3

- [X] T027 [P] [US3] Extend `internal/workflow/evaluate_test.go` (depends on T011, T014, T019,
  T022 — sequential-after every earlier writer of this shared file) with
  dedicated `feature_unknown` coverage for **both** `EvaluateState` and `EvaluateTransition`
  against an unrecognized feature identifier, asserting no `State`/`Terminal` (StateReport) or
  `CurrentState`/`Target` (TransitionReport) is ever populated.
- [X] T028 [US3] Extend `internal/workflow/evaluate_test.go` (same file as T027 — sequential, not
  run in parallel with it; depends on T027, T008, T014, T022) with dedicated `state_invalid`
  **propagation** coverage for **both** `EvaluateState` and `EvaluateTransition`: one
  representative case from each distinct Reading Rules category already exhausted at the
  `FeatureExists`/`ResolveRecord` layer by T008 — a non-canonical value, a structurally malformed
  record, and a symlinked record — confirming the `state_invalid` outcome and its absent fields
  propagate correctly through both evaluators, and that neither function ever substitutes `draft`
  or a guessed canonical value. T008 remains the exhaustive source of truth for every Reading
  Rules sub-case at the resolver layer; this task does not re-enumerate every sub-case a second
  time.
- [X] T029 [P] [US3] Extend `internal/app/workflowstate_test.go` and
  `internal/app/workflowtransition_test.go` (depends on T016, T024, T017, T018, T025, T026 —
  requires both dispatcher wirings and is sequential-after T018/T026, the earlier writers of these
  same two files; never run concurrently with either) with end-to-end
  `feature_unknown`/`state_invalid` assertions through `Run`, confirming: exit code `1` in every
  case; text and JSON output never contain the substring `outcome=state_reported`,
  `outcome=transition_valid`, `"outcome":"state_reported"`, or `"outcome":"transition_valid"`.

### Verification for User Story 3

- [X] T030 [US3] Run T027–T029. If all pass, record that no production code change was needed
  beyond Phases 2–4. If any fails, fix the specific defect in `internal/workflow/record.go` or
  `internal/workflow/evaluate.go` only — do not add a new outcome, a new usage-failure category,
  or alter the FR-029 precedence order to make a failing test pass.

**Checkpoint**: All three user stories independently functional and verified; feature-complete per
spec.md.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final gates from plan.md's "Quality Gates" and "Security / scope audit gate",
applicable across all stories.

- [X] T031 [P] Run `gofmt -l .`, `go vet ./...`, `go build -o /tmp/ai-dev-004 ./cmd/ai-dev` (an
  explicit temporary output path — do not create an `ai-dev` binary in the repository root), and
  `git diff --check` across the full changed set; resolve any reported issue.
- [X] T032 [P] Run `go test ./...` and `go test -count=1 ./...`; confirm all tests pass, and that
  `internal/app/doctor_test.go`, `agents_test.go`, `agentrun_test.go`, `agentselect_test.go`,
  `internal/report/roleselection_test.go`, and `cmd/ai-dev/main_test.go`'s existing assertions are
  byte-for-byte unmodified and still pass (plan.md "Regression Protection" and "Project
  Structure"; `main_test.go` MAY only gain additive dispatch cases).
- [X] T033 (depends on T031, T032, T034 — the final manual step; run only after every automated
  gate and the security/scope audit have passed) Execute `specs/004-workflow-state/quickstart.md`
  end-to-end against a freshly built `ai-dev` binary in a scratch directory, confirming every
  documented outcome, exit code, and the `tar`/`cmp` filesystem non-mutation check in §9.
- [X] T034 [P] Perform the manual "Security / scope audit gate" checklist from plan.md, verifying
  every item:
  - No new `go.mod`/`go.sum` dependency.
  - No agent/role/selection/execution coupling: `internal/workflow` imports nothing from
    `internal/agent`, `internal/role`, `internal/selection`, `internal/execution`, or
    `internal/system`, and no workflow code path reaches an agent, role, selection, or process.
  - `EvaluateState`/`EvaluateTransition`/`FeatureExists`/`ResolveRecord` accept only `fs.FS` plus
    plain strings; production workflow logic makes no `os` filesystem call (no create, write,
    rename, delete, or direct `os` read) — its only filesystem I/O is no-follow
    `fs.ReadLinkFS.Lstat` and `fs.ReadFile` against the caller-supplied `root`, plus writes to the
    caller-supplied `stdout`/`stderr`.
  - No authentication step or network probe anywhere in `internal/workflow`, the new
    `internal/app` files, or the new `internal/report` files.
  - Raw feature IDs are checked (C-1 table) before every path construction; no joined or cleaned
    path is used as the security check; invalid IDs yield `feature_unknown` with zero filesystem
    access.
  - `target_unsupported` is resolved before feature-ID validation and before any filesystem
    access (FR-029 step 2 precedes step 3).
  - Production workflow resolution requires no-follow inspection capability and rejects
    filesystems lacking `fs.ReadLinkFS` rather than silently using a link-following fallback
    (`fs.Stat`/package-level `fs.Lstat`); `specs/<feature-id>` and `workflow-state.json` symlinks
    are rejected and never followed.
  - No routing, no retry, no fallback, no chaining, no background execution, and no persistence or
    write capability — `EvaluateState`/`EvaluateTransition` are single synchronous, stateless calls
    producing one report each.
  - No secret, token, credential, or environment value appears in any new code path or test
    fixture.

  This manual checklist MAY optionally be supplemented by a small, test-only automated
  import-graph check (e.g., a test asserting via `go list -deps` that `internal/workflow`'s import
  list excludes `internal/agent`/`role`/`selection`/`execution`/`system`); this is not required,
  introduces no production dependency or architecture change, and does not alter the approved
  plan's gate list.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup. **Blocks every user story** — `record.go`'s
  containment/symlink rules and the shared report/outcome types are not divisible across stories.
- **User Story 1 (Phase 3)**: Depends on Foundational only. Independently shippable (MVP).
- **User Story 2 (Phase 4)**: Depends on Foundational, plus three shared-file orderings with US1:
  `internal/workflow/evaluate_test.go` (T019 after T011), `internal/workflow/evaluate.go` (T022
  after T014), and the dispatcher's `"workflow"` branch in `internal/app/doctor.go` (T025 after
  T017). Those pairs are never edited concurrently, but
  US2 does not depend on US1's report/app files (`workflowstate.go`) and is independently testable
  once its own tasks are done.
- **User Story 3 (Phase 5)**: Depends on both Foundational and the `EvaluateState`/
  `EvaluateTransition` implementations from US1 (T014) and US2 (T022) — it adds test coverage to
  already-built behavior rather than new capability. T029 additionally depends on both dispatcher
  wirings (T017, T025) and on T018/T026, which write the same two `internal/app` test files.
- **Polish (Phase 6)**: Depends on all three user stories being complete.

### Within Each User Story

- Tests are written first and are expected to fail (to compile only once the referenced
  not-yet-existing function signatures exist) until their paired Implementation task is done.
- `internal/workflow` implementation before `internal/report` before `internal/app` before
  dispatcher wiring, per each story's task order above.

### Parallel Opportunities

- T002, T004, T006 (Foundational production files) may be drafted concurrently — each is fully
  specified by data-model.md/plan.md independently of the others' code — though T004 and T006
  will not compile until T002's `State` type lands; T009 may likewise be drafted concurrently with
  these three but will not compile until T002/T004/T006 **and T007** (its `validFeatureID`
  dependency) land. T003, T005, and T010 are each that same phase's paired test file and are
  **not** marked `[P]`: each is strictly sequential-after its own implementation task (T003 after
  T002, T005 after T004, T010 after T009 and T007) and is never run concurrently with it.
  T007/T008 (record.go/record_test.go) are a sequential pair and are **not** marked `[P]`: T007
  starts only after T002 has landed, T008 only after T007, given the critical
  C-1/symlink-containment logic they implement; no parallel claim is made for either.
- Within US1: T011, T012, T013 (tests, distinct files) can run in parallel. T018 depends on T017
  and has no parallel partner within US1 itself; it may run in parallel with US2's T026 once both
  T017 and T025 have landed (different files, both dependency chains independently satisfied at
  that point).
- Within US2: T019, T020, T021 (tests, distinct files) can run in parallel with each other. T019
  shares `internal/workflow/evaluate_test.go` with US1's T011 (and later US3's T027/T028), so it
  starts only after T011 is done and is never edited concurrently with T011. T026 depends on T025
  and has no parallel partner within US2 itself; see the T018/T026 cross-story note above.
- Within US3: T027 and T029 can run in parallel (distinct files from each other and from T028).
  T029 is never parallel with T018 or T026 (same files): it starts only after T017, T018, T025,
  and T026 have all landed.
  T028 is **not** marked `[P]`: it edits the same file as T027 (`internal/workflow/evaluate_test.go`)
  and is sequential-after it.
- T031, T032, T034 (Polish) can run in parallel; T033 is the final manual step and runs only after
  T031, T032, **and** T034 all succeed — it is not marked `[P]`.

---

## Parallel Example: Foundational Phase

```bash
# Draft together — distinct files; T004/T006 compile only once T002's State type lands:
Task: "Create internal/workflow/state.go"
Task: "Create internal/workflow/transition.go"
Task: "Create internal/workflow/outcome.go"
```

## Parallel Example: User Story 1 Tests

```bash
Task: "Write internal/workflow/evaluate_test.go state-reporting cases"
Task: "Write internal/report/workflowstate_test.go"
Task: "Write internal/app/workflowstate_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup.
2. Phase 2: Foundational (critical — blocks everything).
3. Phase 3: User Story 1 → `ai-dev workflow state` is a complete, shippable increment.
4. **STOP and VALIDATE**: run quickstart.md §§1–3 against the built binary.

### Incremental Delivery

1. Setup + Foundational → shared `internal/workflow` core ready.
2. Add User Story 1 → validate independently → MVP.
3. Add User Story 2 → validate independently (`ai-dev workflow transition` now works).
4. Add User Story 3 → hardening tests confirm `feature_unknown`/`state_invalid` correctness across
   both commands; fix Phase 2–4 code only if a gap is found.
5. Phase 6: Polish — full regression suite, quickstart, security/scope audit.

---

## Notes

- No task in this file introduces a write path, persistence, agent invocation, role selection,
  routing, retry, fallback, background execution, or configurable transition policy — none of
  these is part of the approved spec/plan, and none should be added while executing these tasks.
- If executing these tasks surfaces an apparent contradiction between spec.md/plan.md and what is
  described here, stop and report it rather than silently resolving it (per the approved
  workflow) — this applies to implementers of this file exactly as it applied to its generation.
- Commit after each task or logical group; verify tests fail (to compile or to pass) before their
  paired implementation task, per the Tests-first ordering above.
