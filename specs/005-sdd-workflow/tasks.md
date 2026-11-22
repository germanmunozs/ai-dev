---

description: "Task list for 005-sdd-workflow implementation"
---

# Tasks: SDD Workflow

**Input**: Design documents from `/specs/005-sdd-workflow/`

**Prerequisites**:
- plan.md
- spec.md
- research.md
- data-model.md
- contracts/workflow-next.md
- contracts/next-step-rules.md
- contracts/workflow-featuredir.md
- quickstart.md
- `.specify/memory/constitution.md`

This task list was generated only after the human stated explicit approval of the Human
Specification Gate and the Human Plan Gate in the controlling session. This file is not an
approval record, and it does not approve any Human Gate, including the Human Task Gate.

**Tests**: Included. The plan's Project Structure and Test Strategy name every test file below as
a required deliverable, and constitution principle IV (Testable by Design) applies. Within each
phase, test tasks come before the implementation tasks they verify. Tests are written first and
are expected to fail, or not compile, until the matching implementation task lands.

**Organization**: The tasks follow spec.md's three user stories: US1 (P1) step_recommended, US2
(P2) workflow_terminal, and US3 (P3) artifacts_inconsistent, feature_unknown, and state_invalid.
They use the approved layering:
- `internal/sdd` (005 domain)
- `internal/workflow` (004, one minimal I/O-free extension)
- `internal/report`
- `internal/app`
- `cmd/ai-dev`

**Binding decisions carried from spec and plan (do not re-decide during implementation)**:
- 005 is evaluate-and-recommend only. It never executes or selects agents, never applies or
  persists transitions, never writes `workflow-state.json`, and never grants or records approval.
- Next-Step Table: `draft→specify→specification`, `specified→plan→planning`,
  `planned→tasks→planning`, `tasked→implement→implementation`, and `implemented→review→review`.
  There is no target state, no `abandoned`, and no `remediation`. 004 is the sole authority on
  transitions and terminality.
- There are exactly five outcomes. A table miss is an FR-020 output failure of the "cannot be
  rendered" kind. It is never a domain outcome.
- Exit codes:

  | Result | Exit code |
  |---|---|
  | `step_recommended`, `workflow_terminal` | 0 |
  | `artifacts_inconsistent`, `feature_unknown`, `state_invalid` | 1 |
  | usage failure, output failure | 2 |

  Exit 0 is never approval.
- Artifact presence is metadata-only: one `fs.ReadDir`, an exact byte-for-byte name, and a
  regular file. Content is never opened.
- SIGPIPE handling is installed only at entry to `runWorkflowNext` and released by `defer`. It has
  no global handler, no `signal.Ignore`, no `signal.Reset`, and no change to `cmd/ai-dev/main.go`.
  M-2 (no in-process post-`Stop` restoration test) is an accepted MINOR limitation.
- Existing tests MAY be extended only additively and with justification. No existing assertion may
  be removed or weakened.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel: a different file, and no dependency on an incomplete task.
- **[Story]**: US1, US2, or US3. It is omitted for Setup, Foundational, and Polish tasks.
- Every task names its exact file path or command scope.

## Path Conventions

Single Go module. The existing layout is `cmd/ai-dev/` plus `internal/<domain>/` (plan.md,
Project Structure). No new top-level directory is added.

---

## Phase 1: Setup

**Purpose**: Confirm a clean, passing baseline and record the pre-change behavior that the
isolation control will compare against. No file is changed in this phase.

- [X] T001 Confirm the baseline on branch `feature/005-sdd-workflow` without changing any file:
  - `go.mod` and `go.sum` need no change (Go stdlib only, per plan.md Technical Context);
  - `gofmt -l .` prints nothing;
  - `go vet ./...` succeeds;
  - `go test -count=1 ./...` passes on the current tree.
- [X] T002 Build the current tree outside the repository with
  `go build -o "$(mktemp -d)/ai-dev-baseline" ./cmd/ai-dev`. Confirm that
  `ai-dev-baseline workflow state <id>`, with its stdout connected to a pipe whose read end is
  already closed, is terminated by SIGPIPE on this host. That is the pre-005 004 behavior that
  the isolation control in T033 must keep. Remove the temporary binary afterwards and create no
  file in the repository tree.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared infrastructure that every user story needs:
- the 004 `FeatureDir` extension;
- the architectural boundary test;
- the complete `internal/sdd` domain model and evaluator, with all six precedence steps;
- the report-writer and CLI foundations, which are the `Valid()` gate, usage validation, the
  "cannot be rendered" path, SIGPIPE handling, and dispatch.

**⚠️ CRITICAL**: No user-story task may begin until this phase is complete.

### 004 extension: `workflow.FeatureDir` (contracts/workflow-featuredir.md, research §2)

- [X] T003 [P] Write the regression tests in the new file `internal/workflow/featuredir_test.go`.
  - **Helper reuse (M-01):** reuse the existing package helpers from
    `internal/workflow/record_test.go`:
    - `recordingFS`, which records ordered `"Open:<name>"`, `"Lstat:<name>"`, and
      `"ReadLink:<name>"` entries;
    - `openOnlyFS`, which deliberately omits `fs.ReadLinkFS`.

    Declare no equivalent type under another name. If an extra capability is genuinely needed,
    extend those helpers additively in `record_test.go` without removing or weakening any
    existing assertion.
  - **Order of work:** write steps 1–2 first and run them **green against the unmodified**
    `internal/workflow/record.go`, so the pre-refactor behavior is captured. Then add step 3,
    which does not compile until T004.
  1. **Full ordered call sequences (M-05).** Through `recordingFS` over `fstest.MapFS`, assert
     the exact ordered `calls` slice:

     | Case | Call | Expected ordered calls |
     |---|---|---|
     | Existing valid record (`{"state":"planned"}`) | `EvaluateState(id)` | `Lstat:specs/<id>`, `Lstat:specs/<id>/workflow-state.json`, `Open:specs/<id>/workflow-state.json` |
     | Existing valid record | `EvaluateTransition(id, "tasked")` | the same three calls |
     | Absent `workflow-state.json` | `EvaluateState(id)` | `Lstat:specs/<id>`, `Lstat:specs/<id>/workflow-state.json` (`draft`) |
     | Symlinked `workflow-state.json` | `EvaluateState(id)` | `Lstat:specs/<id>`, `Lstat:specs/<id>/workflow-state.json` (`state_invalid`; no `Open`; target not followed) |
     | Symlinked feature directory | `EvaluateState(id)` | `Lstat:specs/<id>` only (`feature_unknown`) |
     | Each C-1 invalid ID (`..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`) | `EvaluateState`, and `EvaluateTransition` with a canonical target | **zero** calls (`feature_unknown`) |
     | Any of these roots wrapped in `openOnlyFS` | `EvaluateState(id)` | `feature_unknown` |

     Also assert that `FeatureExists` and `ResolveRecord`, called directly, each produce the
     corresponding sub-sequence. The test records observed calls only. It never re-implements
     004 logic to compute expectations.
  2. Each case in step 1 also asserts the outcome and state, so error classification stays
     pinned across the refactor.
  3. **`FeatureDir` table and path equivalence:**
     - `""`, `.`, `..`, `../x`, `a/../../x`, `/etc/passwd`, and `a/b` → `("", false)`;
     - `004-workflow-state` → `(path.Join("specs", "004-workflow-state"), true)`;
     - for a valid ID, the `Lstat` paths recorded in step 1 equal `FeatureDir(id)` and
       `path.Join(FeatureDir(id), "workflow-state.json")`.
- [X] T004 Implement `func FeatureDir(featureID string) (dir string, ok bool)` in
  `internal/workflow/record.go`, then refactor `FeatureExists` and `ResolveRecord` to use it.
  Depends on T003.
  - `FeatureDir` validates **first** with the existing `validFeatureID`, unchanged. It returns
    `path.Join("specs", featureID), true`, or `"", false` for invalid IDs. It performs no I/O and
    never trims, cleans, or rewrites the input.
  - `FeatureExists` keeps the order validate → `fs.ReadLinkFS` assertion → `Lstat(dir)`.
  - `ResolveRecord` keeps validate → `fs.ReadLinkFS` assertion → `Lstat` / `ReadFile` on
    `path.Join(dir, RecordFileName)`.
  - No other line of `internal/workflow` changes: `Recognized`, `Terminal`, `Allowed`,
    `stateField`, `EvaluateState`, `EvaluateTransition`, `Valid()`, and the outcomes stay
    untouched.
  - Run `go test -count=1 ./internal/workflow/...`. Every pre-existing assertion and every T003
    test must pass. In particular, the ordered call sequences that T003 captured on the
    pre-refactor code must be identical after the refactor.

### Architectural boundary (research §4, plan test item 11)

- [X] T005 [P] Write the parser-based boundary test in the new file
  `internal/sdd/boundary_test.go`. It is test code only, with no production helper.
  - **`TestPackageBoundary`** parses every non-`_test.go` file in `internal/sdd` once with
    `go/parser` (full mode) and checks the following:
    - **Non-vacuity (M-06):** the set of scanned non-test files must equal **exactly**
      `{activity.go, outcome.go, rules.go, evaluate.go, validate.go}`. The test fails if:
      - zero files are scanned;
      - any of the five is missing;
      - any other non-test `.go` file is present.

      This test is therefore expected to fail until T006, T007, T009, T012, and T014 have all
      landed. T015 is the point where it must pass.
    - **Import allow-list:** imports must be exactly within `io/fs`, `strings`,
      `ai-dev/internal/workflow`, and `ai-dev/internal/role`. Anything else fails, including
      `os`, `os/exec`, `os/signal`, `net`, `syscall`, `path`, `path/filepath`, and the internal
      packages `agent`, `execution`, `selection`, `system`, `instruction`, `app`, `report`,
      `doctor`, and `checks`.
    - **Aliases:** for each file, the test maps local names to import paths. An explicit
      `ImportSpec.Name` is used as given; otherwise the local name is the last path element.
    - **Rejected import forms:** any dot (`.`) or blank (`_`) import of an `ai-dev/internal/...`
      package.
    - **Workflow selectors:** reject any selector, through any alias, on the workflow package
      that names `Allowed`, `EvaluateTransition`, `TransitionReport`, `TransitionValid`,
      `TransitionInvalid`, or `TargetUnsupported`.
    - **Role selectors:** permit, through any alias, only `Role`, `Specification`, `Planning`,
      `Implementation`, and `Review`. This rejects `Remediation`.
    - **No `"specs"` literal:** reject any string literal equal to `"specs"` or beginning with
      `"specs/"`.
  - **`TestPackageBoundarySelfCheck`** feeds the same checker in-memory snippets.
    - It must reject all twelve known-bad snippets:
      1. `os`
      2. `os/exec`
      3. `os/signal`
      4. `net`
      5. `syscall`
      6. `ai-dev/internal/selection`
      7. aliased `wf "ai-dev/internal/workflow"` with `wf.Allowed`
      8. aliased `r "ai-dev/internal/role"` with `r.Remediation`
      9. `role.Remediation`
      10. blank `_ "ai-dev/internal/selection"`
      11. dot `. "ai-dev/internal/workflow"`
      12. a `"specs"` string literal
    - It must accept one known-good snippet that uses only the four allowed imports,
      `workflow.FeatureDir`, `workflow.EvaluateState`, and `role.Planning`.

### Shared SDD model (data-model.md, contracts/next-step-rules.md)

- [X] T006 [P] Create `internal/sdd/activity.go`:
  - a package doc comment stating that the package is 005's read-only next-step domain, with no
    agent, selection, execution, or transition dependency;
  - `type Activity string`, with exactly `ActivitySpecify = "specify"`, `ActivityPlan = "plan"`,
    `ActivityTasks = "tasks"`, `ActivityImplement = "implement"`, and
    `ActivityReview = "review"`;
  - the artifact constants `SpecArtifact = "spec.md"`, `PlanArtifact = "plan.md"`, and
    `TasksArtifact = "tasks.md"`;
  - imports: none, or only the allow-list.
- [X] T007 [P] Create `internal/sdd/outcome.go`:
  - `type Outcome string`, with exactly `StepRecommended = "step_recommended"`,
    `WorkflowTerminal = "workflow_terminal"`,
    `ArtifactsInconsistent = "artifacts_inconsistent"`, `FeatureUnknown = "feature_unknown"`,
    and `StateInvalid = "state_invalid"`. There is no sixth constant and no `internal_error`.
  - `type Report struct { FeatureID string; State workflow.State; Activity Activity;
    Role role.Role; Missing []string; Outcome Outcome }`, as in data-model.md.
  - Report has no target, next state, agent, approval, or authorization field.
- [X] T008 [P] Write the rules tests in the new file `internal/sdd/rules_test.go`. Depends on
  T004, T006, and T007. Its tests exercise `missingArtifacts`, which calls
  `workflow.FeatureDir`, so they run only after T004.
  - **Shared test-helper owner for `internal/sdd` (M-01):** this file declares the package's
    shared test helpers. T010, T011, and T013 reuse them and must not declare equivalent names:
    - `recordingSDDFS`: wraps an `fs.FS` that implements `fs.ReadLinkFS` and `fs.ReadDirFS`, and
      records ordered `Open`, `Stat`, `Lstat`, `ReadLink`, `ReadFile`, and `ReadDir` calls.
    - `faultReadDirFS`: its `ReadDir` returns an injected error, optionally with partial entries.
    - `sddFixture(record *string, files ...string) fstest.MapFS`: builds `specs/<id>/` with an
      optional `workflow-state.json` and named regular files.
    - `sddFeatureID`: the fixture feature-ID constant.
  - **Next-Step Table:**
    - for each of the 7 canonical `workflow.State` values, `nextStep` returns `ok` if and only if
      `!workflow.Terminal(s)`;
    - exactly the 5 rows `draft→specify/Specification`, `specified→plan/Planning`,
      `planned→tasks/Planning`, `tasked→implement/Implementation`, and
      `implemented→review/Review`;
    - every returned role passes `role.Recognized`, and `role.Remediation` never appears;
    - activity tokens are disjoint from the 7 state tokens.
  - **Required Artifacts:** `requiredArtifacts` returns exactly the following, and each call
    returns a fresh slice (mutating a result does not affect the next call):

    | State | Required |
    |---|---|
    | `draft` | `nil` |
    | `specified` | `[spec.md]` |
    | `planned` | `[spec.md plan.md]` |
    | `tasked`, `implemented`, `approved` | `[spec.md plan.md tasks.md]` |
    | `abandoned` | `nil` |

  - **`missingArtifacts` on `fstest.MapFS`:**
    - every non-empty subset of missing required artifacts, for each state with requirements,
      yields exactly that list in canonical order;
    - artifacts beyond the requirement are ignored;
    - `Plan.md` does not satisfy `plan.md`;
    - a directory named `plan.md` is missing;
    - `ModeSymlink` entries named `spec.md` pointing to a file, dangling, or pointing to a
      directory are missing.
  - **Real filesystem** (`os.DirFS(t.TempDir())`, `os.Symlink`, skipped on Windows): the same
    symlink cases are missing.
  - **Fault injection:** a `ReadDir` wrapper returning an error, and one returning partial
    entries plus an error, both make all required artifacts missing.
  - **Permissions** (real filesystem, skipped on Windows or when running as root):
    - a feature directory with mode `0311` leaves all required artifacts missing;
    - a regular artifact with mode `000` counts as present, because file bytes are never probed.
  - **Recording filesystem**, across all fixtures:
    - zero `Open`, `Stat`, `Lstat`, `ReadLink`, or `ReadFile` calls on any artifact path;
    - exactly one `ReadDir`, on `specs/<id>`, for states with requirements;
    - zero `ReadDir` calls for `draft` and `abandoned`.
- [X] T009 Implement `internal/sdd/rules.go` as unexported exhaustive `switch` functions, with
  no map and no exported variable. Depends on T004 (it calls `workflow.FeatureDir` and must not
  run before that exists) and on T008.
  - `nextStep(workflow.State) (Activity, role.Role, bool)`: the 5 rows only, and `default`
    returns `false`. There is no target output.
  - `requiredArtifacts(workflow.State) []string`: the matrix above.
  - `missingArtifacts(root fs.FS, featureID string, state workflow.State) []string`:
    1. If nothing is required, return `nil` with no filesystem call.
    2. Get `dir, ok := workflow.FeatureDir(featureID)`. If `!ok`, return `required`.
    3. Call `entries, err := fs.ReadDir(root, dir)`. If `err != nil`, return `required`.
    4. Count a name as present only if `e.Name() == canonical` and `e.Type().IsRegular()`.
    5. Return the missing names in canonical order.
  - There is no per-artifact `Open`, `Stat`, `Lstat`, `ReadLink`, or `ReadFile`, and no
    `"specs"` literal.
- [X] T010 [P] Write the case tests in the new file `internal/sdd/casefs_test.go`. Depends on
  T008, because it reuses T008's helpers; the tests pass once T009 lands.
  - **Helpers (M-01):** reuse T008's `sddFixture`, `sddFeatureID`, and `recordingSDDFS`. This
    file owns only `caseInsensitiveFS`.
  - **Simulated (always runs):** a `caseInsensitiveFS` test wrapper over `fstest.MapFS`.
    `Open`, `Lstat`, and `ReadLink` resolve names case-insensitively, while `ReadDir` returns the
    stored names.
    - A self-check asserts that `Lstat("specs/f/plan.md")` resolves to the stored `Plan.md`.
    - Fixtures with `Plan.md`, `SPEC.md`, and `Tasks.md` report those canonical names as missing.
  - **Real host (always runs, no skip):** the same fixtures in `t.TempDir()` through `os.DirFS`.
    - The test probes the host's case-sensitivity (create `probe`, `Lstat` `PROBE`) and records it
      with `t.Log`.
    - It asserts the identical expected result on either kind of host.
- [X] T011 [P] Write the report-invariant tests in the new file `internal/sdd/validate_test.go`.
  Depends on T006 and T007.
  - **Helpers (M-01):** this file needs no filesystem helper. It declares only
    `Report` literals, and it must not redeclare any T008 helper name.
  - **Valid reports pass**, for every outcome, including:
    - `workflow_terminal` with `State: approved` and with `State: abandoned`;
    - `feature_unknown` with a structurally invalid ID.
  - **Fails, one mutation at a time:**
    - a required field blanked, or a prohibited field populated, for each row of the FR-016
      table;
    - `Activity` or `Role` differing from `nextStep(State)`;
    - a terminal `State` on `step_recommended`, or a non-terminal `State` on
      `workflow_terminal`;
    - a `Missing` list that is empty, unordered, duplicated, or not a subset of
      `requiredArtifacts(State)`;
    - a blank or whitespace `FeatureID`;
    - an unknown outcome, or an empty outcome;
    - `state_invalid` or any state-bearing outcome with a structurally invalid ID.
  - **Table-miss representation (M-07):** the evaluator's table-miss value is exactly
    `Report{FeatureID: "f"}`: only `FeatureID`, with no `State`, `Outcome`, `Activity`, `Role`, or
    `Missing`. It must fail `Valid()`.
  - **Additional validation-only case:** `Report{FeatureID: "f", State: workflow.Planned}` with
    no outcome must also fail `Valid()`. It is labeled in the test as an extra invalid-input
    check, **not** as a representation the evaluator produces.
- [X] T012 Implement `Report.Valid()` in `internal/sdd/validate.go` as the FR-016 table in code.
  Depends on T009 and T011.
  - Every report: non-blank `FeatureID` (`strings.TrimSpace`), and exactly one of the five
    outcomes.
  - Per outcome:

    | Outcome | Rule |
    |---|---|
    | `step_recommended` | canonical, non-terminal `State`; `Activity` and `Role` equal `nextStep(State)`; `Missing` empty; `FeatureDir` ok |
    | `workflow_terminal` | canonical, terminal `State` (`workflow.Terminal`); `Activity`, `Role`, and `Missing` empty; `FeatureDir` ok |
    | `artifacts_inconsistent` | canonical `State`; `Activity` and `Role` empty; `Missing` a non-empty, strictly increasing, canonical-order subset of `requiredArtifacts(State)`; `FeatureDir` ok |
    | `feature_unknown` | all optional fields empty; the ID may be structurally invalid |
    | `state_invalid` | all optional fields empty; `FeatureDir` ok |

- [X] T013 Write the evaluator tests in the new file `internal/sdd/evaluate_test.go`. Depends on
  T008 (shared helpers) and T012.
  - **Helpers (M-01):** reuse T008's `recordingSDDFS`, `sddFixture`, and `sddFeatureID`. This file
    owns only `countingLookup`, a test wrapper around the existing unexported `lookup` parameter
    of `evaluate(root, id, lookup)` that delegates to `nextStep` and counts its calls. **No new
    hook is added.**
  - One fixture per outcome and per state, through `Evaluate` on `fstest.MapFS`.
  - **Lookup observation (M-04),** through `evaluate(root, id, countingLookup)`:

    | Outcome | Lookup calls |
    |---|---|
    | `feature_unknown` (unknown, C-1 invalid, and symlinked-directory IDs) | exactly 0 |
    | `state_invalid` | exactly 0 |
    | `artifacts_inconsistent` (including `approved` with `tasks.md` missing) | exactly 0 |
    | `workflow_terminal` (`approved` complete, `abandoned`) | exactly 0 |
    | `step_recommended` (each of the 5 non-terminal states) | exactly 1, with the current state |
  - Precedence pairs:

    | Fixture | Expected |
    |---|---|
    | invalid record and missing artifacts | `state_invalid` |
    | `approved` with `tasks.md` missing | `artifacts_inconsistent` |
    | `abandoned` with no artifacts | `workflow_terminal` |
    | `draft` with no artifacts | `step_recommended` / `specify` |
    | `specified` plus `plan.md` and `tasks.md` | `step_recommended` / `plan` (SC-005) |

  - Recording-filesystem call logs:

    | Case | Calls |
    |---|---|
    | C-1 invalid IDs | zero calls |
    | absent feature | exactly 1 `Lstat`, 0 `ReadDir` |
    | `state_invalid` | 0 `ReadDir` |
    | `draft` and `abandoned` | 0 `ReadDir` |
    | states with requirements | exactly 1 `ReadDir`, on `FeatureDir(id)` |

  - A symlinked feature directory gives `feature_unknown` with 0 `ReadDir`.
  - **004 parity:** for every 004 `state_invalid` sub-case and every C-1 ID, 005's
    `feature_unknown` / `state_invalid` equals `workflow.EvaluateState`'s classification.
  - `string(FeatureUnknown) == string(workflow.FeatureUnknown)` and
    `string(StateInvalid) == string(workflow.StateInvalid)`.
  - **Determinism:** repeated calls are `reflect.DeepEqual`.
  - **Table miss (M-07):** use the unexported seam `evaluate(root, id, lookup)` with a lookup
    lacking `planned`, on a valid `planned` fixture with artifacts present. The result must be
    `reflect.DeepEqual` to `Report{FeatureID: id}`: only `FeatureID`, and no `State`, `Outcome`,
    `Activity`, `Role`, or `Missing`. It must fail `Valid()`, and it is not
    `artifacts_inconsistent`, `state_invalid`, or any other outcome.
- [X] T014 Implement `internal/sdd/evaluate.go`, following the plan.md "Precedence as control
  flow" sketch. Depends on T009, T012, and T013.
  - `func Evaluate(root fs.FS, featureID string) Report { return evaluate(root, featureID, nextStep) }`.
  - The unexported `evaluate(root, featureID, lookup)`:
    1. `sr := workflow.EvaluateState(root, featureID)` (steps 2–3). Map `feature_unknown` and
       `state_invalid` directly. Any other non-`state_reported` value returns `Report{FeatureID}`
       with no outcome.
    2. `missingArtifacts` (step 4) → `artifacts_inconsistent`.
    3. `sr.Terminal` (step 5, 004 terminality) → `workflow_terminal`.
    4. `lookup(sr.State)` (step 6) → `step_recommended`.
    5. A missing table entry returns exactly `Report{FeatureID: featureID}`, with no `State`,
       `Outcome`, `Activity`, `Role`, or `Missing`. This is the unified table-miss representation
       (M-07), and FR-007 makes it unreachable.
  - There is no transition API, no `"specs"` literal, and no goroutine.
- [X] T015 Run `go test -count=1 ./internal/sdd/...` over the completed package.
  Depends on T005 and T014.
  - `TestPackageBoundary` (T005) must now pass against the real non-test files in
    `internal/sdd/`. Its non-vacuity check (M-06) must confirm that it scanned exactly
    `activity.go`, `outcome.go`, `rules.go`, `evaluate.go`, and `validate.go`.
  - `TestPackageBoundarySelfCheck` must reject all twelve known-bad snippets and accept the
    known-good one.

### Report-writer foundation (contracts/workflow-next.md)

- [X] T016 [P] Write the shared report tests in the new file
  `internal/report/workflownext_test.go`. Depends on T007.
  - Both `WriteWorkflowNextText` and `WriteWorkflowNextJSON` reject every invalid `sdd.Report`
    with an error and **zero bytes written**. That includes the table-miss representation
    `sdd.Report{FeatureID: "f"}` (M-07).
  - A helper decodes exactly one JSON object with `json.Decoder`, and a second `Decode` must
    return `io.EOF`.
  - A helper scans output for forbidden wording and keys: `target`, `next_state`, `agent`,
    `gate`, `authorized`, `approved_by`, and `approval` / `authoriz` outside the fixed
    `step_recommended` disclaimer.
- [X] T017 Create `internal/report/workflownext.go`. Depends on T012 and T016.
  - `WriteWorkflowNextText(w io.Writer, r sdd.Report) error` and
    `WriteWorkflowNextJSON(w io.Writer, r sdd.Report) error`, each gated by `r.Valid()` before
    writing anything.
  - Text uses one `fmt.Fprintf` line with the prefix `ai-dev: outcome=%s feature=%q`.
  - JSON uses one `json.Encoder.Encode` of a wire struct with keys `feature_id`, `outcome`,
    `current_state` (omitempty), `activity` (omitempty), `role` (omitempty), and
    `missing_artifacts` (omitempty `[]string`). A prohibited key is absent, never `null`, `""`,
    or `[]`.
  - Per-outcome rendering is added in T024, T027, and T030. Until then, an outcome without a
    rendering branch returns an error.
  - There is no custom encoding.

### CLI foundation (research §6, §9, §11; contracts/workflow-next.md)

- [X] T018 [P] Create `internal/app/sigpipe_unix.go` with `//go:build unix`. It defines
  `func brokenPipeAsError() (release func())`, which creates `ch := make(chan os.Signal, 1)`,
  calls `signal.Notify(ch, syscall.SIGPIPE)`, and returns
  `func() { signal.Stop(ch) }`.
  - It does not drain `ch`, and uses no `signal.Ignore`, no `signal.Reset`, no `init`, and no
    goroutine.
  - A doc comment states that the effect is process-wide but limited in time to one
    `runWorkflowNext` call, under the documented precondition of one `runWorkflowNext` per
    process.
- [X] T019 [P] Create `internal/app/sigpipe_other.go` with `//go:build !unix`, containing the
  no-op `func brokenPipeAsError() (release func()) { return func() {} }`. It does not reference
  `syscall.SIGPIPE`.
- [X] T020 [P] Write the CLI foundation tests in the new file `internal/app/workflownext_test.go`.
  Depends on T007.
  - **Helper reuse (M-01):** reuse the existing package helpers in `internal/app`:
    - `recordingRootFS`, `workflowFixture`, `strPtr`, and `runDispatcher` from
      `internal/app/workflowstate_test.go`;
    - `fakeExecutor` from `internal/app/doctor_test.go`;
    - `appRunLauncher` from `internal/app/agentrun_test.go`;
    - `snapshotDir` and `controlledTempDir` from `internal/app/agentselect_test.go`.

    Declare no equivalent type or function under another name. `recordingRootFS` already
    observes listings, because `fs.ReadDir` falls back to the recorded `Open` when the root does
    not implement `fs.ReadDirFS`. Any new helper this file needs uses a `workflowNext` prefix,
    for example `workflowNextFixture`, to avoid collisions.
  - **Usage failures:**
    - each case is a usage failure:
      - no ID;
      - `""`;
      - a whitespace-only ID;
      - an extra positional argument;
      - an unknown flag;
      - `--format yaml`;
      - **`--format` with no value**, i.e. `workflow next --format` (M-02);
      - a flag after the positional;
    - each gives exit 2;
    - stderr is exactly
      `Usage: ai-dev workflow next [--format text|json] <feature-id>\n`;
    - stdout is empty, with no report;
    - `recordingRootFS` sees zero calls, and an evaluator injected through the
      `runWorkflowNext` seam that fails the test if called is never invoked, so nothing is
      evaluated.
  - `../x` is **not** a usage failure.
  - **"Cannot be rendered" output failure:** through the unexported seam
    `runWorkflowNext(args, root, stdout, stderr, evaluate)`, inject an evaluator that returns
    the unified table-miss representation `sdd.Report{FeatureID: "f"}` (M-07). Expect:
    - exit 2;
    - empty stdout;
    - stderr exactly
      `ai-dev: internal error: workflow next report cannot be rendered for feature "f"\n`;
    - no outcome token on either stream.
  - **Group usage unchanged:** through `Run`, the usage for `workflow` and `workflow bogus` is
    still exactly `Usage: ai-dev workflow <state|transition> [options]\n`. This is an additive
    assertion; the existing 004 test stays as it is.
  - `brokenPipeAsError()` returns a release function that can be called safely.
- [X] T021 Create `internal/app/workflownext.go`. Depends on T014, T017, T018, T019, and T020.
  - `func RunWorkflowNext(args []string, root fs.FS, stdout, stderr io.Writer) int` returns
    `runWorkflowNext(args, root, stdout, stderr, sdd.Evaluate)`.
  - The unexported `runWorkflowNext`:
    1. **First statements:** `release := brokenPipeAsError()` and `defer release()`.
    2. Parse with `flag.NewFlagSet("workflow next", flag.ContinueOnError)` and `SetOutput(io.Discard)`.
       Require `--format` to be `text` or `json`, `NArg() == 1`, and
       `strings.TrimSpace(id) != ""`. Otherwise write the usage line and return 2.
    3. `r := evaluate(root, id)`.
    4. If `!r.Valid()`, write the "cannot be rendered" diagnostic to stderr and return 2, with
       nothing written to stdout.
    5. Write the report with the text or JSON writer to stdout. On an error, write
       `write workflow next report: %v` to stderr and return 2. There is exactly one write
       attempt and no retry or fallback.
    6. Return 0 for `StepRecommended` and `WorkflowTerminal`, and 1 otherwise.
  - The function has no stdin, executor, launcher, or registry parameter.
- [X] T022 Add the single dispatch arm
  `if len(args) >= 2 && args[1] == "next" { return RunWorkflowNext(args[2:], root, stdout, stderr) }`
  inside the existing `args[0] == "workflow"` branch of `internal/app/doctor.go`. Depends on
  T021.
  - Keep `Usage: ai-dev workflow <state|transition> [options]` and every other string and
    branch byte-identical.
  - `Run`'s signature does not change.
  - Run `go test -count=1 ./internal/app/...`. T020 and every pre-existing assertion must pass.

**Checkpoint**: The domain, evaluator, writer gate, CLI shell, SIGPIPE handling, and dispatch are
done. User-story phases add per-outcome rendering and end-to-end verification.

---

## Phase 3: User Story 1 — Find out the next SDD step for a feature (Priority: P1) 🎯 MVP

**Goal**: For a recognized feature in any non-terminal state, with its required artifacts
present, `ai-dev workflow next` reports `step_recommended` with the exact activity and role and a
recommendation-only disclaimer, and exits 0. Nothing is run, selected, applied, or approved.

**Independent Test**: Run `ai-dev workflow next` in text and JSON for fixtures in `draft`,
`specified`, `planned`, `tasked`, and `implemented`, with their required artifacts. Each must
report `step_recommended` with the exact activity and role and exit 0. There must be no agent,
selection, or transition activity and no filesystem change (spec US1 AS1–AS10).

- [X] T023 [US1] Add `step_recommended` rendering tests to
  `internal/report/workflownext_test.go`. Depends on T016.
  - **Text:** the exact line
    `ai-dev: outcome=step_recommended feature=%q state=<s> activity=<a> role=<r> note="recommendation only; does not authorize this activity or any transition"\n`
    for each of the 5 non-terminal states.
  - **JSON:** the exact key set `{feature_id, outcome, current_state, activity, role}`, all
    strings, with `missing_artifacts` absent and a second `Decode` returning `io.EOF`.
  - No forbidden wording or keys.
  - **Valid-UTF-8 fidelity (P-06):** an ID containing `<`, `>`, `&`, and non-ASCII letters
    round-trips exactly through JSON, and renders `%q`-quoted on exactly one line in text.
- [X] T024 [US1] Implement `step_recommended` rendering in `internal/report/workflownext.go`
  (text line with the fixed disclaimer, plus JSON fields). Depends on T017 and T023.
- [X] T025 [US1] Add end-to-end US1 tests through `Run` to `internal/app/workflownext_test.go`.
  Depends on T022 and T024.
  - **Outcomes:** for each non-terminal state with its required artifacts, text and JSON give
    the exact stdout, empty stderr, and exit 0.
  - **Artifacts ahead of state:** `specified` with extra `plan.md` and `tasks.md` still reports
    `plan`.
  - **Determinism:** byte-identical output on repeated runs.
  - **Non-mutation:** a real temp tree, using the `snapshotDir` / `controlledTempDir` pattern
    from `internal/app/agentselect_test.go`, is unchanged afterwards, and no
    `workflow-state.json` is created.
  - **No execution or selection:** the dispatcher's `fakeExecutor` and `appRunLauncher` record
    zero lookups, runs, and launches.
  - **No stdin:** a stdin reader that fails the test if read is never touched.
  - **No secrets:** `t.Setenv("AI_DEV_TEST_SECRET", "s3cr3t-value")`, and the value never
    appears in any output.
  - **Output failure, "cannot be written":** an `io.Writer` that always errors and counts its
    calls gives exactly one write attempt, exit 2, `write workflow next report:` on stderr, and
    no reclassification.
  - **Dispatch:** `Run([]string{"workflow", "next", ...})` routes to the command.

**Checkpoint**: US1 is fully functional and testable on its own (MVP).

---

## Phase 4: User Story 2 — Recognize that a feature's workflow has ended (Priority: P2)

**Goal**: For features whose 004 state is terminal (`approved` with its required artifacts, or
`abandoned` with any artifacts), report `workflow_terminal` with the observed current state, no
activity or role, and exit 0. Nothing may imply approval.

**Independent Test**: An `approved` fixture with all three artifacts, and an `abandoned` fixture
with none, each report `workflow_terminal` with `current_state` and exit 0. The exit code does not
distinguish them, and neither output contains approval or authorization wording (spec US2
AS1–AS4).

- [X] T026 [US2] Add `workflow_terminal` rendering tests to `internal/report/workflownext_test.go`.
  Depends on T024, because it is the same writer under test.
  - **Text:** exactly `ai-dev: outcome=workflow_terminal feature=%q state=<approved|abandoned>\n`,
    with no disclaimer.
  - **JSON:** exactly the keys `{feature_id, outcome, current_state}`.
  - `current_state=approved` is accepted as an observed state.
  - No forbidden approval or authorization wording or keys.
- [X] T027 [US2] Implement `workflow_terminal` rendering in `internal/report/workflownext.go`.
  Depends on T024 and T026.
- [X] T028 [US2] Add end-to-end US2 tests through `Run` to `internal/app/workflownext_test.go`.
  Depends on T025 and T027.
  - **Real dependency on US1 (M-09):** besides same-file ordering after T025, the "callers must
    read the outcome" comparison below functionally requires US1's `step_recommended`
    rendering (T024) and end-to-end path (T025). The remaining US2 cases depend only on Phase 2
    and T027.
  - **Terminal outcomes:**
    - `approved` with `spec.md`, `plan.md`, and `tasks.md` → `workflow_terminal`, exit 0;
    - `abandoned` with no artifacts, and `abandoned` with arbitrary artifacts →
      `workflow_terminal`, exit 0.
  - **Exit code:** `approved` and `abandoned` give the same exit code. Their outputs differ only
    in the `current_state` value.
  - **Callers must read the outcome:** a test asserts that `step_recommended` and
    `workflow_terminal` both exit 0 and differ in `outcome`, so the exit code alone does not show
    whether an activity exists.
  - **Scan:** no output contains approval or authorization wording or keys.
  - **Non-mutation:** the snapshot is unchanged and no record is written.

**Checkpoint**: US2's terminal reporting works. Its exit-0 comparison case relies on US1 being
complete (see T028).

---

## Phase 5: User Story 3 — Detect a state/artifact inconsistency or an undeterminable state (Priority: P3)

**Goal**: `artifacts_inconsistent`, with the canonical-order missing list, takes precedence over
terminal handling. `feature_unknown` and `state_invalid` are inherited from 004 and lead to no
artifact listing. All three exit 1, and nothing is repaired.

**Independent Test**: Missing-artifact fixtures, including `approved` without `tasks.md` and the
`Plan.md` case variant, invalid or traversal or symlinked IDs, and invalid records each produce
their own outcome with exit 1 and no recommendation (spec US3 AS1–AS10).

- [X] T029 [US3] Add rendering tests to `internal/report/workflownext_test.go`. Depends on T026.
  - **`artifacts_inconsistent` text:**
    `ai-dev: outcome=artifacts_inconsistent feature=%q state=<s> missing=spec.md,plan.md`, in
    canonical order.
  - **`artifacts_inconsistent` JSON:** the keys `{feature_id, outcome, current_state,
    missing_artifacts}`, where `missing_artifacts` is a non-empty JSON array of strings in
    canonical order.
  - **`feature_unknown` and `state_invalid`:** text `ai-dev: outcome=<o> feature=%q`, and JSON
    with exactly the keys `{feature_id, outcome}`.
  - **Injection:** feature IDs containing each of the following are rendered as `feature_unknown`:
    - `\n`;
    - **`\t` (tab, M-03)**;
    - **`\r` (carriage return, M-03)**;
    - `"`;
    - `\\`;
    - spaces;
    - the literal `outcome=step_recommended`.

    For each:
    - the output is exactly one newline-terminated line;
    - it begins with `ai-dev: outcome=feature_unknown feature="`;
    - the feature value equals `fmt.Sprintf("%q", id)`, so tab and CR appear only as the escapes
      `\t` and `\r`, never as raw bytes;
    - after removing the `%q`-rendered ID, the line contains exactly one `outcome=`.
  - **Invalid UTF-8 (P-06):** an ID containing byte `0xff` yields U+FFFD in JSON `feature_id`
    and `\xff` in text. This pins the inherited limitation, with no custom encoding.
- [X] T030 [US3] Implement the `artifacts_inconsistent`, `feature_unknown`, and `state_invalid`
  rendering in `internal/report/workflownext.go`. Depends on T027 and T029.
- [X] T031 [US3] Add end-to-end US3 tests through `Run` to `internal/app/workflownext_test.go`.
  Depends on T028 and T030. Every case below exits 1.
  - **`artifacts_inconsistent`:**
    - `planned` without `plan.md` → missing `plan.md`;
    - `implemented` with no artifacts → `spec.md,plan.md,tasks.md`;
    - `approved` without `tasks.md` → `artifacts_inconsistent`, **not** `workflow_terminal`;
    - `planned` with only `Plan.md` → missing `plan.md`;
    - `tasked` with a symlink named `tasks.md` → missing `tasks.md`.
  - **`feature_unknown`** with zero `ReadDir` calls, for:
    - an unknown ID;
    - the IDs `..`, `../x`, `a/../../x`, `/etc/passwd`, and `a/b`, which also make zero
      filesystem calls;
    - a symlinked feature directory.
  - **`state_invalid`** with zero `ReadDir` calls, for the records `{"state":"Planned"}`,
    `{"state":""}`, and `{`. None of them defaults to `draft`.
  - **Failure outputs:** no failure output contains `outcome=step_recommended` outside the
    quoted ID, and none contains `"outcome":"step_recommended"`.
  - **Non-mutation:** the snapshot is unchanged. Nothing is repaired, created, or retried.

**Checkpoint**: All three user stories work independently.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Real-binary contract and broken-pipe proofs, the 001–004 regression, the security
audit, the quality gates, and the final manual acceptance.

- [X] T032 Create `cmd/ai-dev/workflownext_contract_test.go`. Depends on T031, so all
  production edits for the three user stories are complete.
  - **Shared binary-helper owner for `cmd/ai-dev` (M-01):** this file declares
    - `buildWorkflowNextBinary(t) string`, which builds `ai-dev` into `t.TempDir()`;
    - `makeWorkflowNextRepo(t, record *string, files ...string) string`, which creates a temp
      repository with `specs/f/`.

    It reuses the existing `runCommand` from `cmd/ai-dev/main_test.go` and does not redeclare it
    or any other existing helper (`makeReadyFixture`, `makeFakePATH`, `waitForFixturePID`).
  - Create temp repositories with `specs/<id>/` fixtures for every outcome.
  - For each outcome, in text and JSON, assert the exact stdout, the exact stderr, and the exit
    code (0/0/1/1/1).
  - For each usage-failure class, assert exit 2, the usage line on stderr, and empty stdout. The
    classes include **`workflow next --format` with no value** (M-02).
  - `approved` and `abandoned` both exit 0 with `workflow_terminal`.
- [X] T033 Create `cmd/ai-dev/workflownext_brokenpipe_test.go` with `//go:build unix`, for macOS
  and Linux. Depends on T031 (no production edit can still be in progress when it builds and
  runs the binary, M-09) and on T032 (shared helpers).
  - **Helpers (M-01):** reuse T032's `buildWorkflowNextBinary` and `makeWorkflowNextRepo`.
    Declare no equivalent helper.
  - Define `TestWorkflowNextBrokenPipe`.
  - **Setup:** build `ai-dev` with `buildWorkflowNextBinary`, using a `draft` fixture repository
    from `makeWorkflowNextRepo`.
    - Create `r, w, _ := os.Pipe()` and call `r.Close()` **before** `cmd.Start`.
    - Pass `w` (an `*os.File`) directly as `cmd.Stdout`, and capture stderr.
  - **Broken stdout:** for `workflow next f` and `workflow next --format json f`, assert
    `WaitStatus.Signaled() == false`, exit 2, and `write workflow next report:` on stderr.
  - **Broken stderr on usage failure:** run `workflow next` with no ID, with `cmd.Stderr` set to
    a second pipe whose read end is already closed. Assert that the process is not signaled and
    exits 2, with empty stdout.
  - **Isolation control:** `workflow state f` under the same broken stdout is still terminated
    by SIGPIPE (`Signaled() && Signal() == syscall.SIGPIPE`), matching the T002 baseline.
  - Never use `>&-` or `/dev/full`. Add no in-process post-`Stop` restoration test, because M-2
    is an accepted MINOR limitation.
- [X] T034 [P] Run the 001–004 regression and review its evidence. Depends on T032 and T033.
  - **Run:**
    `go test -count=1 -v ./internal/doctor/... ./internal/checks/... ./internal/agent/... ./internal/execution/... ./internal/instruction/... ./internal/system/... ./internal/role/... ./internal/selection/... ./internal/workflow/... ./internal/report/... ./internal/app/... ./cmd/ai-dev/...`.
  - **Confirm the named areas pass:** doctor, execution, role, selection, workflow state,
    workflow transition, the `FeatureDir` refactor (T003), and CLI dispatch, including 004's
    `TestRunWorkflowDispatchUsageFailures` and `TestRunTopLevelUsageStringsAreIdenticalAndNameWorkflow`.
  - **Review modified pre-existing tracked test files** with `git diff HEAD -- '*_test.go'`
    (informational, pre-commit). Every change must be additive and justified, with no assertion
    removed or weakened.
  - Compatibility is proven by passing assertions, not by file immutability. The plan's
    `git diff --diff-filter=M main...HEAD -- '*_test.go'` review is for the human on the
    committed branch before merge.
- [X] T035 [P] Run the security and scope audit over the 005 diff. Depends on T032 and T033.
  - **Audit scope (M-08):**
    - **New 005 production files**, audited whole: `internal/sdd/activity.go`,
      `internal/sdd/outcome.go`, `internal/sdd/rules.go`, `internal/sdd/evaluate.go`,
      `internal/sdd/validate.go`, `internal/report/workflownext.go`,
      `internal/app/workflownext.go`, `internal/app/sigpipe_unix.go`, and
      `internal/app/sigpipe_other.go`.
    - **Pre-existing files modified by 005**, audited **only on the 005 diff**:
      `internal/app/doctor.go` and `internal/workflow/record.go`. Use
      `git diff HEAD -- internal/app/doctor.go internal/workflow/record.go`, and inspect only the
      added or changed lines.
    - These two files legitimately already contain earlier features' code: `os`, `agent`,
      `system`, `checks`, `doctor` and `report` imports, and the 002 `agent run` / 003
      `agent select` dispatch. That pre-existing code is **not** a finding.
    - The expected `doctor.go` diff is exactly the one `next` dispatch arm. The expected
      `record.go` diff is the `FeatureDir` addition plus the `FeatureExists` and `ResolveRecord`
      path refactor.
    - Every check below applies to this scope only, so pre-existing code produces no false
      positives.
  - **Dependencies and `main.go`:**
    - `git diff --quiet HEAD -- go.mod go.sum cmd/ai-dev/main.go` (all unchanged).
  - **No writes, processes, network, or environment access,** in the new files and the 005 diff
    hunks:
    - none of `os.Create`, `os.WriteFile`, `os.Remove`, `os.Mkdir`, `os.Rename`, `os.OpenFile`,
      `os/exec`, `net`, `os.Getenv`, or `os.Environ` appears.
  - **Signal handling:**
    - `os/signal` and `syscall` appear only in `internal/app/sigpipe_unix.go`;
    - `brokenPipeAsError` is called only from `runWorkflowNext`;
    - there is no `signal.Ignore` and no `signal.Reset`.
  - **No background work:** no `go ` statement, goroutine, retry, fallback, routing, chaining,
    or persistence.
  - **No transition, agent, or approval logic:**
    - no `workflow.Allowed` or `EvaluateTransition` in `internal/sdd`;
    - no agent execution and no role selection;
    - no write of `workflow-state.json`;
    - no field or text granting, recording, inferring, or authorizing approval;
    - no task output approves a Human Gate or turns exit 0 into approval.
  - **Artifacts:** no artifact content read anywhere; presence comes only from `fs.ReadDir`.
  - **Coverage:** the C-1 and symlink IDs and the symlinked or unreadable artifact cases are
    covered by T008, T013, and T031.
- [X] T036 [P] Run the architectural boundary gate:
  `go test -count=1 -v ./internal/sdd -run 'TestPackageBoundary'`. Confirm it passes, that all
  twelve known-bad self-check snippets are rejected, and that the known-good snippet is accepted.
  Depends on T034 and T035.
- [X] T037 [P] Run the format and vet gates at the repository root: `gofmt -l .` must print
  nothing, and `go vet ./...` must succeed with no diagnostics. Depends on T034 and T035.
- [X] T038 [P] Run the test gates at the repository root: `go test ./...` and
  `go test -count=1 ./...` must both pass. Depends on T034 and T035.
- [X] T039 [P] Run the race gate at the repository root: `go test -count=1 -race ./...` must
  pass. Depends on T034 and T035.
- [X] T040 [P] Run the build gate **outside the repository tree**:
  `go build -o "$(mktemp -d)/ai-dev" ./cmd/ai-dev` must succeed. Confirm that `go.mod` and
  `go.sum` are unchanged, and that `git status --porcelain` shows no built binary inside the
  repository. Depends on T034 and T035.
- [X] T041 [P] Run the whitespace gate: `git diff --check` must report no errors. Depends on T034
  and T035.
- [X] T042 Manually run `specs/005-sdd-workflow/quickstart.md` end to end (sections 1–4), with
  the binary built outside the repository, against scratch repositories only.
  - Section 2 covers every row of the outcome table, the extra commands (`../x`, `/etc/passwd`,
    the symlinked directory, usage failures), and the broken-pipe check. The broken-pipe check
    expects `next exit=2`, and `state exit=141` as the isolation control.
  - Section 3 is the read-only snapshot.
  - **This is a human acceptance task. It must NOT be marked complete during automated
    implementation, and completing it is not a Human Gate approval.**
  - Depends on T032–T041.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)** has no dependencies: T001 → T002.
- **Foundational (Phase 2)** depends on Setup and blocks every user story. Its internal order:
  - T003 → T004.
  - T006 and T007 come before T008, T010, T011, T016, and T020.
  - T004 → T008 and T004 → T009, because both run code that calls `workflow.FeatureDir`.
  - T008 → T009.
  - T008 → T010 (shared helpers) and T008 → T013 (shared helpers).
  - T009 and T011 → T012 → T013 → T014.
  - T005 and T014 → T015.
  - T012 and T016 → T017.
  - T014, T017, T018, T019, and T020 → T021 → T022.
- **US1 (Phase 3)** depends on Phase 2 being complete.
- **US2 (Phase 4)** depends on Phase 2 and, **functionally**, on US1. T028's "callers must read
  the outcome" comparison needs US1's `step_recommended` rendering (T024) and end-to-end path
  (T025). Same-file ordering also applies: T026 follows T024 and T028 follows T025.
- **US3 (Phase 5)** depends on Phase 2. It depends on US2 only for same-file ordering: T029
  follows T026, T030 follows T027, and T031 follows T028.
- **Polish (Phase 6)** depends on all user stories:
  - T032 depends on T031;
  - T033 depends on T031 and T032, so no binary is built while production edits are in
    progress;
  - T034 and T035 depend on T032 and T033;
  - T036–T041 depend on T034 and T035;
  - T042 runs last and depends on T032–T041.

### User Story Dependencies

- **US1 (P1)** needs Phase 2 and nothing else. It is the MVP.
- **US2 (P2)** needs Phase 2, plus US1 for the single comparison case in T028. Every other US2
  case (terminal rendering and terminal outcomes) is testable with only Phase 2 and T027.
- **US3 (P3)** is functionally independent of US1 and US2, because the evaluator already handles
  every failure outcome in Phase 2. It is ordered after US2 only because of shared files.

### Same-File Ordering (never parallel)

| File | Task order |
|---|---|
| `internal/report/workflownext_test.go` | T016 → T023 → T026 → T029 |
| `internal/report/workflownext.go` | T017 → T024 → T027 → T030 |
| `internal/app/workflownext_test.go` | T020 → T025 → T028 → T031 |
| `internal/workflow/record.go` | T004 only |
| `internal/workflow/record_test.go` (pre-existing) | T003 only, and only if an additive helper extension is genuinely needed (M-01) |
| `cmd/ai-dev/workflownext_contract_test.go` | T032 only; its helpers are reused, not edited, by T033 |
| `internal/app/workflownext.go` | T021 only |
| `internal/app/doctor.go` | T022 only |

Every other file is written by exactly one task.

### Within Each Phase

- Tests come first: T003 before T004, T008 before T009, T011 before T012, T013 before T014,
  T016 before T017, T020 before T021, T023 before T024, T026 before T027, and T029 before T030.
- Domain comes before report, and report before CLI. Dispatch (T022) is the last foundational
  production change.

---

## Parallel Opportunities

| When | Parallel tasks |
|---|---|
| Phase 2 start | T003 (`featuredir_test.go`), T005 (`boundary_test.go`), T006 (`activity.go`), T007 (`outcome.go`), T018 (`sigpipe_unix.go`), T019 (`sigpipe_other.go`) |
| After T006 and T007 | T011 (`validate_test.go`), T016 (`report/workflownext_test.go`), and T020 (`app/workflownext_test.go`) |
| After T004, T006, and T007 | T008 (`rules_test.go`), alongside any of T011, T016, or T020 still running |
| After T008 | T010 (`casefs_test.go`), alongside any of T011, T016, or T020 still running |
| Phase 6 | none: T032 → T033 run in sequence, because T033 reuses T032's helpers and must not overlap production edits |
| After T033 | T034 (regression) and T035 (audit). Both are read-only and write no file. |
| After T034 and T035 | the read-only gates T036, T037, T038, T039, T040, and T041 |

Example for the Phase 2 kickoff, run together:

```text
T003 internal/workflow/featuredir_test.go
T005 internal/sdd/boundary_test.go
T006 internal/sdd/activity.go
T007 internal/sdd/outcome.go
T018 internal/app/sigpipe_unix.go
T019 internal/app/sigpipe_other.go
```

---

## Independent Test Criteria

- **US1**:
  - `go test ./internal/report -run WorkflowNext` and `go test ./internal/app -run WorkflowNext`
    pass for the US1 cases.
  - `ai-dev workflow next <id>` on the five non-terminal fixtures gives `step_recommended` with
    the exact activity and role, the disclaimer, and exit 0.
  - No agent or selection activity, no stdin read, no filesystem mutation, and exactly one write
    attempt.
- **US2**:
  - The `approved` (all three artifacts) and `abandoned` fixtures give `workflow_terminal` with
    `current_state` and exit 0.
  - The exit code is identical for both, and no approval or authorization wording appears.
  - `step_recommended` and `workflow_terminal` are distinguishable only through `outcome`.
- **US3**:
  - Missing-artifact fixtures, including `approved` without `tasks.md`, `Plan.md`-only, and a
    symlinked `tasks.md`, give `artifacts_inconsistent` with the canonical-order list and exit 1.
  - Invalid, traversal, absolute, nested, or symlinked IDs give `feature_unknown` with zero
    `ReadDir` calls and exit 1.
  - Invalid records give `state_invalid` with zero `ReadDir` calls and exit 1.
  - Text output is injection-safe.

---

## Implementation Strategy

### Suggested MVP Scope

Phase 1, Phase 2, and Phase 3 (US1). That gives a working `ai-dev workflow next` for every
consistent non-terminal feature, with:
- the full precedence order;
- the boundary test;
- SIGPIPE handling;
- the "cannot be rendered" path.

Stop and validate US1 independently before continuing.

### Incremental Delivery

1. Setup and Foundational: the domain, evaluator, and CLI shell are complete.
2. US1: `step_recommended`. Validate, which completes the MVP.
3. US2: `workflow_terminal`. Validate.
4. US3: `artifacts_inconsistent`, `feature_unknown`, and `state_invalid`. Validate.
5. Polish: the real-binary contract and broken-pipe tests, regression, audit, gates, and finally
   the manual quickstart (T042, human).

## Notes

- [P] means a different file with no dependency on an incomplete task. Same-file tasks are never
  [P].
- No task approves a Human Gate, infers or persists approval, turns exit 0 into approval, or
  executes the recommendation automatically.
- Do not commit, merge, or change branch as part of these tasks unless a human explicitly asks.
