# Tasks: Agent Role Selection

**Input**: Design documents from `/specs/003-agent-role-selection/`
**Prerequisites**: `plan.md` (required), `spec.md` (required, user stories), `research.md`,
`data-model.md`, `contracts/agent-select.md`, `contracts/selection-policy.md`,
`quickstart.md`

**Tests**: Included. The constitution's Testable by Design principle and this feature's
own `plan.md` "Test Strategy" / "Quality Gates" sections require automated coverage for
every outcome, every usage-failure class, and both execution-mechanism no-call proofs, so
test tasks are treated as required deliverables, not optional additions.

**Organization**: Tasks are grouped by user story (spec.md P1/P2/P3) so each story's
acceptance scenarios can be verified independently once the shared foundation exists.
Because this feature's entire outcome model is produced by one function
(`selection.Selector.Select`, called through one `internal/app.RunAgentSelect`), the
Foundational phase below builds that complete, already-correct pipeline; each User Story
phase then adds that story's specific acceptance-criteria tests and CLI-level wiring
verification against it — it does not add new production branches.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Every task includes the exact file path(s) it touches

## Path Conventions

Single Go module CLI (existing layout — no new top-level directory):
`cmd/ai-dev/`, `internal/app/`, `internal/role/` (new), `internal/selection/` (new),
`internal/report/`, `internal/agent/`, `internal/execution/`, `internal/system/`.

---

## Phase 1: Setup

**Purpose**: Confirm a clean starting point. No new dependency, scaffolding tool, or
project structure is needed — this feature extends the existing `cmd/` + `internal/`
layout in place (plan.md "Project Structure").

- [X] T001 Establish baseline: from the repository root, run `gofmt -l .`, `go vet ./...`,
  `go build ./cmd/ai-dev`, and `go test ./...`; confirm all pass cleanly and that
  `go.mod`/`go.sum` show no pending changes, before any code in this feature is written
  (plan.md "Quality Gates"; research.md §9 — stdlib only, no new dependency).

**Checkpoint**: Baseline green. Foundational work may begin.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the complete, correct four-outcome pipeline — `internal/role`,
`internal/selection`, `internal/report`'s role-selection renderer, `internal/app`'s
`RunAgentSelect`, and the one-line dispatcher addition — exactly as specified in
`data-model.md`, `contracts/selection-policy.md`, and `contracts/agent-select.md`. No
production branch is added or changed by any later user-story phase; those phases add
tests and CLI-level verification against this pipeline.

**⚠️ CRITICAL**: No user story task can begin until this phase is complete — every story
exercises this same pipeline.

- [X] T002 [P] Create `internal/role/role.go`: package `role`; `type Role string`; the
  five canonical constants `Specification Role = "specification"`,
  `Planning Role = "planning"`, `Implementation Role = "implementation"`,
  `Review Role = "review"`, `Remediation Role = "remediation"`; and
  `func Recognized(value string) (Role, bool)` that matches `value` **exactly and
  case-sensitively** against those five constants with no normalization, aliasing,
  abbreviation, or fuzzy matching (FR-002), returning `("", false)` for any non-match.
  This package MUST import nothing from `internal/agent` (data-model.md "Development
  Role").

- [X] T003 Create `internal/selection/outcome.go` (depends on T002: `Report.RequestedRole`
  is typed `role.Role`, so `role.go` must already declare that type): package `selection`;
  `type Outcome string`; the four canonical constants
  `RoleSelected Outcome = "role_selected"`, `RoleUnsupported Outcome = "role_unsupported"`,
  `AgentUnavailable Outcome = "agent_unavailable"`,
  `SelectionInconsistent Outcome = "selection_inconsistent"`; and the `Report` struct
  exactly per data-model.md: `SuppliedRole string`, `RequestedRole role.Role`,
  `SelectedAgentID agent.ID`, `MappedAgentID agent.ID`, `Outcome Outcome`. Field-presence
  constraint to satisfy (data-model.md, verbatim): "`role_selected`: SuppliedRole ✓,
  RequestedRole ✓, SelectedAgentID ✓, MappedAgentID —; `role_unsupported`: SuppliedRole ✓,
  RequestedRole —, SelectedAgentID —, MappedAgentID —; `agent_unavailable`: SuppliedRole ✓,
  RequestedRole ✓, SelectedAgentID —, MappedAgentID ✓; `selection_inconsistent`:
  SuppliedRole ✓, RequestedRole ✓, SelectedAgentID —, MappedAgentID ✓." `SelectedAgentID`
  and `MappedAgentID` must never both be non-empty on the same `Report`.

- [X] T004 Create `internal/selection/policy.go` (depends on T002 for `role.Role`):
  package `selection`; unexported
  `func mappedAgent(r role.Role) (agent.ID, bool)` implemented as an **exhaustive
  `switch`** (not a map, not a struct field, not any other mutable value) with exactly
  these five cases per FR-003's fixed table: `role.Specification -> agent.ClaudeCode`,
  `role.Planning -> agent.ClaudeCode`, `role.Implementation -> agent.CodexCLI`,
  `role.Review -> agent.ClaudeCode`, `role.Remediation -> agent.CodexCLI`; `default:
  return "", false`. No exported accessor, setter, constructor parameter, configuration
  file, environment variable, or plugin hook may expose or mutate this mapping from
  outside this file (plan.md "Selection Policy Immutability").

- [X] T005 Create `internal/selection/selector.go` (depends on T002, T003, T004): package
  `selection`; `type Selector struct { registry agent.Registry }` (this is the *only*
  field — no policy value is ever held on `Selector`); `func NewSelector(registry
  agent.Registry) Selector`; method `func (s Selector) Select(suppliedRole string)
  Report` implementing exactly this lifecycle (contracts/selection-policy.md): (1) call
  `role.Recognized(suppliedRole)` — if not recognized, return
  `Report{SuppliedRole: suppliedRole, Outcome: RoleUnsupported}` immediately with **no**
  registry access; (2) call `mappedAgent(recognizedRole)` for the target `agent.ID`;
  (3) return `resolveAgainstRegistry(suppliedRole, recognizedRole, target, s.registry)`.
  Also add the unexported helper
  `func resolveAgainstRegistry(suppliedRole string, requested role.Role, target agent.ID, registry agent.Registry) Report`
  that scans `registry.List()` for an `AgentStatus` whose `Agent.ID == target`: no match
  → `Report{SuppliedRole: suppliedRole, RequestedRole: requested, MappedAgentID: target, Outcome: SelectionInconsistent}`;
  match with `Availability == agent.Unavailable` →
  `Report{..., MappedAgentID: target, Outcome: AgentUnavailable}`; match with
  `Availability == agent.Available` →
  `Report{..., SelectedAgentID: target, Outcome: RoleSelected}`. Neither method may
  import or reference `internal/execution`, `system.Launcher`, or `system.Executor`.

- [X] T006 Create `internal/report/roleselection.go` (depends on T003 for
  `selection.Report`/`selection.Outcome`): package `report`; mirror the structural
  pattern of `internal/report/executionreport.go` exactly:
  `func WriteRoleSelectionText(w io.Writer, r selection.Report) error`,
  `func WriteRoleSelectionJSON(w io.Writer, r selection.Report) error`, and an unexported
  `func validateRoleSelectionReport(r selection.Report) error` (mirroring
  `validateExecutionReport`) that rejects any `Report` whose field presence does not
  match T003's table, returning a descriptive error rather than rendering a malformed
  report. Text format, one line, exactly per `contracts/agent-select.md`:
  `ai-dev: outcome=role_selected role=<canonical> agent=<selected-id>`;
  `ai-dev: outcome=role_unsupported role="<raw-supplied-value>"` (quoted, no `agent=`);
  `ai-dev: outcome=agent_unavailable role=<canonical> agent=<mapped-id>`;
  `ai-dev: outcome=selection_inconsistent role=<canonical> agent=<mapped-id>`. JSON
  format, one `json.NewEncoder`-written object per line, with `omitempty` on
  `requested_role`, `selected_agent_id`, and `mapped_agent_id` wire fields so an absent
  field is a fully omitted key (not `null`, not `""`) — exact field-presence rules per
  `contracts/agent-select.md`'s Machine-readable output section.

- [X] T007 Create `internal/app/agentselect.go` (depends on T005, T006): package `app`;
  `func RunAgentSelect(args []string, registry agent.Registry, stdout, stderr io.Writer) int`
  — **no `system.Executor`, no `system.Launcher`, no `context.Context` parameter**
  (plan.md "Dependency Boundary"). Behavior: `flag.NewFlagSet("agent select",
  flag.ContinueOnError)` with `flags.SetOutput(io.Discard)`; `--format` string flag,
  default `"text"`; usage failure (exit `2`, usage text
  `Usage: ai-dev agent select [--format text|json] <role>` on stderr, empty stdout, zero
  registry access, no `Report` emitted) when: no role token; the role token is empty or
  whitespace-only (`strings.TrimSpace`); more than one positional after flags; an
  unparseable flag; or `--format` is neither `text` nor `json`. Otherwise construct
  `selection.NewSelector(registry).Select(role)`, write it via
  `report.WriteRoleSelectionText`/`WriteRoleSelectionJSON` per `--format` to `stdout`; on
  a write error, print a diagnostic to `stderr` (mirroring
  `RunAgentsList`/`RunDoctor`'s `fmt.Fprintf(stderr, "write ...: %v\n", err)` pattern)
  and return `2` — never reclassify the failure as any of the four outcomes (plan.md
  "Report Validation / Write Failure"). Otherwise return `0` if
  `report.Outcome == selection.RoleSelected`, else `1`.

- [X] T008 Modify `internal/app/doctor.go` (depends on T007): inside the existing
  `if args[0] == "agent" { ... }` block, replace the single
  `if len(args) < 2 || args[1] != "run" { ...; return 2 }` condition with two
  independent checks per plan.md "Dispatcher Change":
  `if len(args) >= 2 && args[1] == "run" { return RunAgentRun(ctx, args[2:], executor, launcher, stdin, readerIsInteractive(stdin), "", stdout, stderr) }`
  (byte-for-byte the same call and arguments as today), then
  `if len(args) >= 2 && args[1] == "select" { registry := agent.NewRegistry(executor); return RunAgentSelect(args[2:], registry, stdout, stderr) }`,
  falling back to `fmt.Fprintln(stderr, "Usage: ai-dev agent <run|select> [options]")` and
  `return 2` for a missing or unrecognized agent subcommand. Add the
  `"ai-dev/internal/agent"` import if not already present in this file. Do **not**
  change `app.Run`'s signature, the `doctor`/`agents` dispatch branches, `RunDoctor`, or
  any other line in this file.

**Checkpoint**: `ai-dev agent select [--format text|json] <role>` is fully wired and
produces all four outcomes correctly. User story phases below add acceptance-criteria
tests against this pipeline; they do not add new production logic.

---

## Phase 3: User Story 1 - Determine which agent is selected for a role that is ready to use (Priority: P1)

*This phase is an internal development milestone (the feature's happy path only) — it is
**not** the Human implementation checkpoint. See Phase 6's checkpoint after T036 for the
first point at which this feature may be presented for Human Task Gate review.*

**Goal**: A recognized role whose mapped agent is currently locally available reports
`role_selected` with that agent's canonical identity, deterministically, without
executing anything or touching the filesystem — for each of the five roles
independently.

**Independent Test**: Request selection for a role whose mapped agent is available;
confirm the report names exactly that agent, reports `role_selected`, repeats
identically on a second call, and that no process is invoked and nothing on disk
changes.

### Tests for User Story 1

- [X] T009 [US1] In `internal/selection/selector_test.go`, add a table-driven test
  (e.g. `TestSelectorReportsRoleSelectedForEveryCanonicalRoleWithAnAvailableAgent`)
  asserting, for each of the five canonical roles against a fake-backed `agent.Registry`
  in which both `claude-code` and `codex-cli` are available: `Outcome == RoleSelected`,
  `SelectedAgentID` equals the role's FR-003 target (`specification`→`claude-code`,
  `planning`→`claude-code`, `implementation`→`codex-cli`, `review`→`claude-code`,
  `remediation`→`codex-cli`), and `MappedAgentID == ""`.

- [X] T010 [US1] In `internal/selection/selector_test.go` (same file as T009 — apply
  after it, not concurrently), add `TestSelectorSelectIsDeterministic` — call `Select`
  twice with the same `suppliedRole` against the same fake-backed registry state and
  assert the two `Report` values are `reflect.DeepEqual` (FR-011).

- [X] T011 [P] [US1] In `internal/report/roleselection_test.go`, add
  `TestWriteRoleSelectionTextAndJSONForRoleSelected` asserting the exact text line
  (`ai-dev: outcome=role_selected role=implementation agent=codex-cli`) and exact JSON
  object (`supplied_role`, `requested_role`, `selected_agent_id`, `outcome` present; no
  `mapped_agent_id` key) per `contracts/agent-select.md`'s examples.

### Implementation for User Story 1

*(No new production code — T009–T011 verify Foundational's existing implementation.)*

- [X] T012 [US1] Create `internal/app/agentselect_test.go` with
  `TestRunAgentSelectRoleSelectedAcrossAllFiveRoles`: dispatch
  `ai-dev agent select <role>` and `ai-dev agent select --format json <role>` through
  `Run` (per FR Acceptance Scenario 4 — every role resolved independently, none chains
  into another) for all five roles, using a fake `system.Executor` reporting both
  `claude`/`codex` as found; assert exit code `0` and the exact text/JSON output for
  each; call the same request twice to also confirm determinism at the CLI boundary
  (SC-002).

- [X] T013 [US1] In `internal/app/agentselect_test.go`, add
  `TestRunAgentSelectRoleSelectedDoesNotMutateFilesystem`: create a controlled temporary
  directory (`os.MkdirTemp` plus a small fixed file tree — never the developer's real
  repository), call `t.Chdir(tempDir)` so that directory is the **active working
  directory of the `RunAgentSelect` invocation under test** (making the snapshot
  meaningful rather than incidental — `t.Chdir` restores the original working directory
  automatically when the test ends, so the real repository is never left altered), take
  a full file-tree snapshot, invoke a `role_selected` `RunAgentSelect` call, snapshot
  again, and assert byte-identical (FR-024, SC-009). Introduce no production
  filesystem/working-directory abstraction — `RunAgentSelect` already takes no `workdir`
  parameter; `t.Chdir` is a test-only mechanism.

- [X] T014 [US1] Manually run `quickstart.md` §3 against `go build ./cmd/ai-dev`'s
  output binary: `./ai-dev agent select specification` and
  `./ai-dev agent select --format json specification` with `claude` on `PATH`; confirm
  the documented expected output and `echo $?` is `0`.

**Checkpoint**: User Story 1's happy path is functionally complete and independently
verified in isolation. This is an internal development milestone only, **not** a safe,
deployable, or Human-Task-Gate-ready state: public failure behavior (`role_unsupported`,
`agent_unavailable`, `selection_inconsistent`, every usage-failure class — including the
explicit empty-role case), report-write-failure handling, both no-execution proofs, and
001/002 regression protection are all still unverified at this point (Phases 4–6 below).

---

## Phase 4: User Story 2 - Understand clearly when a requested role is not recognized (Priority: P2)

**Goal**: A syntactically valid but non-canonical role value reports `role_unsupported`
with no agent identity and no availability check performed; a missing or
whitespace-only role value is a distinct usage failure, never `role_unsupported`.

**Independent Test**: Request selection with a role name outside the five canonical
roles; confirm `role_unsupported`, no agent identity, zero registry discovery. Separately
confirm a missing/whitespace-only role is reported as a usage failure, not
`role_unsupported`.

### Tests for User Story 2

- [X] T015 [P] [US2] In `internal/selection/selector_test.go`, add
  `TestSelectorReportsRoleUnsupportedWithoutAnyRegistryAccess`: assert
  `Select("Implementation")` (wrong case) and `Select("nonsense")` both return
  `Outcome == RoleUnsupported`, `SuppliedRole` preserved exactly as given,
  `RequestedRole`/`SelectedAgentID`/`MappedAgentID` all empty; use a
  `recordingExecutor`-backed registry and assert its `LookPath` call count is `0` (FR-007
  "MUST NOT attempt any agent availability check").

- [X] T016 [P] [US2] In `internal/report/roleselection_test.go`, add
  `TestWriteRoleSelectionTextAndJSONForRoleUnsupported` asserting
  `ai-dev: outcome=role_unsupported role="Implementation"` (quoted raw value, no
  `agent=`) and the JSON object containing only `supplied_role` and `outcome` — no
  `requested_role`, `selected_agent_id`, or `mapped_agent_id` key.

### Implementation for User Story 2

*(No new production code — T015–T016 verify Foundational's existing implementation.)*

- [X] T017 [US2] In `internal/app/agentselect_test.go`, add
  `TestRunAgentSelectRoleUnsupported`: dispatch `ai-dev agent select Specification` and
  `ai-dev agent select nonsense` (text and json) through `Run`; assert exit code `1`,
  the correct output, and zero `LookPath` calls on a `recordingExecutor` wired into
  `Run`'s executor parameter.

- [X] T018 [US2] In `internal/app/agentselect_test.go`, add
  `TestRunAgentSelectUsageFailuresAreDistinctFromRoleUnsupported` (mirrors
  `TestRunAgentsListUsageFailuresDoNotDiscover` /
  `TestRunAgentRunSeparatesUsageFailuresFromRecognizedExecutionFailures`) as a
  table-driven test covering, at minimum: `ai-dev agent select` (no role token at all —
  **preserve this existing missing-role case**), `ai-dev agent select ""` (an
  **explicit, syntactically-supplied but empty role value** — FR-016's "empty" half,
  distinct from both the missing-role case above and the whitespace-only case below; this
  is the case the independent review found was not explicitly exercised), `ai-dev agent
  select "   "` (whitespace-only role value — **preserve this existing case**, FR-016's
  "whitespace-only" half), `ai-dev agent select implementation extra` (extra
  positional), `ai-dev agent select --bogus implementation` (unknown flag),
  `ai-dev agent select --format yaml implementation` (invalid `--format` value), and
  `ai-dev agent select --format` (the `--format` flag supplied with **no value at all** —
  a distinct malformed-flag-syntax case from the invalid-value case above; the flag
  parse itself fails before any positional is even considered). For **every** case in
  this table, assert all of the following (FR-015, FR-016):
  - the outcome is a usage failure, not `role_unsupported` and not any other of the four
    semantic outcomes;
  - process completion uses the already-approved usage-failure exit code `2` (the same
    convention `doctor`/`agents`/`agent run` already use — no new numeric code);
  - a usage diagnostic is written to stderr;
  - stdout is exactly empty (no partial or full report line/object);
  - no semantic selection report (`selection.Report`) is ever constructed or rendered —
    no `outcome=`/`"outcome"` token of any kind appears in stdout or stderr;
  - zero `agent.Registry`/`LookPath` discovery activity occurs (assert a
    `recordingExecutor`'s `LookPath` call count is `0`);
  - no agent is executed: the same shared fake `system.Launcher` used elsewhere in this
    file (see T030) records zero `Launch` calls for every case in this table.

- [X] T019 [US2] In `internal/app/agentselect_test.go`, add
  `TestRunAgentSelectRoleUnsupportedAndUsageFailureDoNotMutateFilesystem`: create a
  controlled temporary directory and `t.Chdir(tempDir)` into it (same mechanism and
  same real-repository-safety guarantee as T013), snapshot its file tree, run a
  `role_unsupported` invocation and a representative usage failure (e.g. the `""` role
  case from T018) against that same active working directory, snapshot again after
  each, and assert byte-identical in both cases (FR-024, SC-009). No production
  filesystem abstraction is introduced.

- [X] T020 [US2] Manually run `quickstart.md` §4 (unrecognized role) and §7 (usage
  failures) against the built binary; confirm documented exit codes and output.

**Checkpoint**: User Stories 1 AND 2 both work independently.

---

## Phase 5: User Story 3 - Understand clearly when a recognized role's mapped agent cannot currently be used (Priority: P3)

**Goal**: A recognized role whose mapped agent is a supported-but-unavailable coding
agent reports `agent_unavailable` (never `role_selected`); the reserved
`selection_inconsistent` integrity outcome is fully verified at all three coverage
layers without ever being reachable through the public CLI; no failure outcome resembles
`role_selected`; nothing further is attempted after any non-`role_selected` outcome; and
selection never executes or launches a process under any outcome.

**Independent Test**: Request a role mapped to an agent not on `PATH`; confirm
`agent_unavailable` naming both role and mapped agent, no selected identity. Separately,
confirm `selection_inconsistent`'s three-layer coverage (`internal/selection`,
`internal/report`, `internal/app` via a direct `RunAgentSelect` call with
`agent.Registry{}`) and that it stays unreachable through the public CLI.

### Tests for User Story 3

- [X] T021 [US3] In `internal/selection/selector_test.go`, add
  `TestResolveAgainstRegistryReportsAgentUnavailable`: fake-backed registry where the
  mapped target exists but `Availability == Unavailable` → `Outcome == AgentUnavailable`,
  `MappedAgentID` set to the target, `SelectedAgentID == ""` (FR-008).

- [X] T022 [US3] In `internal/selection/selector_test.go` (same file as T021 — apply in
  order, not concurrently), add the exact test from `contracts/selection-policy.md`:
  `TestResolveAgainstRegistryReportsSelectionInconsistentForAnUnregisteredTarget` —
  calls `resolveAgainstRegistry("implementation", role.Implementation,
  agent.ID("not-a-real-agent"), agent.NewRegistry(fakeExecutor))` directly; asserts
  `Outcome == SelectionInconsistent`, `MappedAgentID == "not-a-real-agent"`,
  `SelectedAgentID == ""`. This test must not touch `mappedAgent` or any mutable policy
  value (none exists).

- [X] T023 [US3] In `internal/selection/selector_test.go` (same file as T021/T022 —
  apply in order, not concurrently), add
  `TestMappedAgentProductionEntriesAllResolveAgainstTheRealRegistry`: for all five
  `mappedAgent` cases, assert the returned target is found in the real
  `internal/agent.NewRegistry(...).List()` output — proving `selection_inconsistent` is
  unreachable under the approved production mapping (SC-010's "0%" half).

- [X] T024 [P] [US3] In `internal/report/roleselection_test.go`, add
  `TestWriteRoleSelectionTextAndJSONForAgentUnavailableAndSelectionInconsistent`:
  construct `selection.Report{..., Outcome: AgentUnavailable}` and
  `selection.Report{..., Outcome: SelectionInconsistent}` directly (never through the
  CLI) and assert both render `role=`/`agent=` (mapped, never presented as selected) in
  text and `mapped_agent_id` present / `selected_agent_id` absent in JSON, per
  `contracts/agent-select.md`'s tables. This is rendering/field-contract coverage only —
  it does not assert stream placement or exit codes (those belong to T026 below).
  Additionally, in the same file, add a deterministic table-driven test
  `TestWriteRoleSelectionRejectsInvalidReportsBeforeWriting` covering, for **both**
  `WriteRoleSelectionText` and `WriteRoleSelectionJSON`, invalid `selection.Report`
  values built directly as Go struct literals (no new production seam — this exercises
  the `validateRoleSelectionReport` helper T006 already specifies): a `RoleSelected`
  report missing `SelectedAgentID` (a field required by that outcome per T003's
  presence table); a `RoleUnsupported` report that carries a non-empty `RequestedRole`
  (a field prohibited for that outcome); a `RoleUnsupported` **or** `AgentUnavailable`/
  `SelectionInconsistent` report that carries a non-empty `SelectedAgentID` (a selected
  identity present where prohibited); a report with both `SelectedAgentID` and
  `MappedAgentID` non-empty at once (the invariant that a selected agent is never also
  reported as merely mapped); and a report whose `Outcome` is not one of the four
  canonical values (e.g. `selection.Outcome("bogus")`). For every case, assert the
  writer returns a non-nil error and — because `validateRoleSelectionReport` runs as the
  very first statement in both writers, strictly before any `Fprintf`/`Encode` call, per
  T006's design (mirroring `executionreport.go`'s `validateExecutionReport` ordering) —
  assert the destination `io.Writer` received **zero bytes** for each of these specific,
  controlled invalid-`Report` cases. This zero-bytes assertion is justified only by that
  specific ordering guarantee; it is not a claim that every possible writer failure
  leaves zero bytes written (see T029's narrower, differently-justified zero-bytes
  claim). Do not introduce any new exported constructor, flag, or production
  configurability to build these invalid `Report` values — plain struct literals in the
  test file are sufficient since `Report`'s fields are already exported.

### Implementation for User Story 3

*(No new production code — T021–T024 verify Foundational's existing implementation.)*

- [X] T025 [US3] In `internal/app/agentselect_test.go`, add
  `TestRunAgentSelectAgentUnavailable`: dispatch `ai-dev agent select implementation`
  through `Run` with a fake executor reporting `codex` not found; assert exit code `1`
  and the correct text/json output.

- [X] T026 [US3] In `internal/app/agentselect_test.go`, add the application-level
  `selection_inconsistent` coverage from `plan.md` "Application-Level
  `selection_inconsistent` Coverage" as two tests:
  `TestRunAgentSelectSelectionInconsistentTextMode` — call
  `RunAgentSelect([]string{"implementation"}, agent.Registry{}, &stdout, &stderr)`
  directly (not through `Run`); assert `stdout` is exactly one line with
  `outcome=selection_inconsistent role=implementation agent=codex-cli`, no
  selected-agent identity, `stderr` is empty, and the returned `int` is `1`.
  `TestRunAgentSelectSelectionInconsistentJSONMode` — call `RunAgentSelect([]string{
  "--format", "json", "implementation"}, agent.Registry{}, &stdout, &stderr)`; assert
  `stdout` decodes as exactly one JSON object with `"outcome":"selection_inconsistent"`,
  `"requested_role":"implementation"`, `"mapped_agent_id":"codex-cli"`, no
  `"selected_agent_id"` key; `stderr` empty; returned `int` is `1`. Use the Go zero value
  `agent.Registry{}` exactly — no new exported constructor, field, or test hook.

- [X] T027 [US3] In `internal/app/agentselect_test.go`, add
  `TestNoFailureOutcomeResemblesRoleSelected`: for `role_unsupported`,
  `agent_unavailable`, and `selection_inconsistent` (the latter via T026's
  `agent.Registry{}` call), assert the rendered text/JSON never contains
  `outcome=role_selected` / `"outcome":"role_selected"` and never carries a selected-agent
  identity (FR-010).

- [X] T028 [US3] In `internal/app/agentselect_test.go`, add
  `TestNoFurtherAutomaticActionAfterNonSelectedOutcome`. There is no `Registry.List()`
  call-count seam in production (and none is added — `agent.Registry` gains no new
  method or counter solely for this test); the only observable evidence available,
  consistent with 002's existing `LookPath`-only availability semantics and the exact
  pattern `internal/agent/registry_test.go` already uses
  (`recordingExecutor.lookups []string`), is the **sequence of `LookPath` calls**. Assert
  exactly:
  - `role_unsupported`: a `recordingExecutor` wired into the request records **zero**
    `LookPath` calls (`Select` returns before any registry access — FR-007).
  - A normal supported-role selection that reaches `resolveAgainstRegistry` against the
    real, fixed two-entry production registry (`agent_unavailable`, and `role_selected`
    for cross-check): the `recordingExecutor`'s recorded `LookPath` calls are **exactly**
    `["claude", "codex"]`, in that order — one complete, deterministic scan of
    `Registry.List()`'s fixed iteration order (mirroring
    `TestRegistryListIsFixedOrderedAndUsesOnlyLocalDiscovery`'s own expectation), proving
    neither zero scans nor more than one scan occurred.
  - `selection_inconsistent` via the `agent.Registry{}` seam (T026): **zero** `LookPath`
    calls, because the empty registry's `nil` `agents` field makes `List()`'s loop
    iterate zero times and never dereference the zero-valued `executor` field (plan.md
    "Application-Level `selection_inconsistent` Coverage").
  In every case, also assert no additional attempt, alternate-agent choice, or retry
  occurs within the same `RunAgentSelect` call (FR-023, SC-008) — evidenced by these
  exact, bounded `LookPath` sequences rather than an unobservable call-count claim.

- [X] T029 [US3] In `internal/app/agentselect_test.go`, add
  `TestRunAgentSelectReportWriteFailureIsNotReclassified`: pass an always-erroring
  `io.Writer` as `stdout`; assert the returned code is `2`, a diagnostic is written to
  `stderr`, and the failure is never rendered as any of the four outcomes — in
  particular never as `selection_inconsistent` (plan.md "Report Validation / Write
  Failure"). This test verifies zero report bytes reached `stdout` for this specific,
  controlled always-fails-immediately case only; it does not assert a general atomic-
  rollback guarantee.

- [X] T030 [US3] In `internal/app/agentselect_test.go`, add
  `TestRunAgentSelectNeverLaunchesOrExecutes` implementing both behavioral proofs from
  `contracts/selection-policy.md`, using **two different, independently-genuine positive
  controls** — `agent run` never calls `system.Executor.Run` (confirmed by reading
  `internal/execution/runner.go`: `Runner.Run` only calls `Registry.Resolve`,
  `Registry.List()` — `LookPath` only — and `Launcher.Launch`; it never calls
  `Executor.Run`), so `agent run` cannot serve as a positive control for `Executor.Run`,
  and no production code is changed merely to manufacture one:
  - **`system.Launcher.Launch` positive control**: wire the *same* fake `system.Launcher`
    instance `agent run`'s existing tests use into `Run`; dispatch
    `role_selected`/`role_unsupported`/`agent_unavailable`/every usage-failure case
    through it and assert the launcher's launch count stays `0` throughout; then, in the
    same test, confirm that same launcher's count increases for an `agent run` call —
    proving the fake actually detects a launch.
  - **`system.Executor.Run` positive control**: use `ai-dev doctor`'s existing
    `git-repo` check (`internal/checks/gitrepo.go`, which genuinely calls
    `executor.Run(ctx, "git", "rev-parse", "--is-inside-work-tree")` as part of the
    already-existing `doctor` command — no code changes needed to obtain this) as the
    positive control instead of `agent run`: wire the *same* `recordingExecutor` into
    `Run`; dispatch `role_selected`/`role_unsupported`/`agent_unavailable`/every
    usage-failure `agent select` case through it and assert the executor's `Run` call
    count stays `0` throughout; then, in the same test, dispatch `ai-dev doctor` through
    that same `Run` with the same `recordingExecutor` and confirm its `Run` call count
    increases — proving the fake actually detects an `Executor.Run` call, using an
    application path (`doctor`) that genuinely exercises it.
  For every `agent select` dispatch in this test, require **both** the launcher's
  `Launch` count and the executor's `Run` count remain zero. Do **not** wire a fake
  `Launcher`/`Executor` into T026's `selection_inconsistent` test: its zero-execution
  property already follows unconditionally from `RunAgentSelect`'s signature (no such
  parameters exist to wire — this is the structural proof, preserved unchanged), and
  adding one would introduce an execution dependency the function does not have.

- [X] T031 [US3] In `internal/app/agentselect_test.go`, add
  `TestRunAgentSelectAgentUnavailableDoesNotMutateFilesystem`: create a controlled
  temporary directory and `t.Chdir(tempDir)` into it (same mechanism as T013/T019),
  snapshot its file tree, invoke an `agent_unavailable` `RunAgentSelect` call against
  that same active working directory, snapshot again, and assert byte-identical
  (FR-024, SC-009). No production filesystem abstraction is introduced.

- [X] T032 [US3] Manually run `quickstart.md` §5 (agent_unavailable), §6
  (`selection_inconsistent` not naturally reachable through the public CLI — confirm no
  flag/env var reproduces it), §8 (never launches an agent — `ps` before/after), and §9
  (filesystem untouched via `git status --porcelain`) against the built binary.

**Checkpoint**: All three user stories are independently functional; the full
four-outcome model is verified end-to-end.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Confirm regression protection, run every declared quality gate, and perform
the security/scope audit — none of these add new behavior.

- [X] T033 [P] Regression: run `go test ./internal/app/... ./cmd/ai-dev/...` and confirm
  every existing test — `TestRunFormatAndUsageContract`,
  `TestRunExitCodeIsFormatIndependent`, `TestRunDoctorReadyIsDeterministicAndOrdered`,
  `TestRunReadyTextExitZero`, `TestRunDoctorEachSingleFailureIsIsolatedAndExitOne`,
  `TestRunDoctorOutsideRepositoryAndAllFailuresStillReportsNine` (exactly nine ordered
  doctor checks) in `internal/app/doctor_test.go`; every test in
  `internal/app/agents_test.go`; every test in `internal/app/agentrun_test.go`; and both
  tests in `cmd/ai-dev/main_test.go` — passes unmodified (plan.md "Regression
  Protection").

- [X] T034 [P] Regression: run
  `go test ./internal/agent/... ./internal/execution/... ./internal/report/...` and
  confirm every existing test in `internal/agent/registry_test.go`,
  `internal/agent/agent_test.go`, `internal/execution/runner_test.go`,
  `internal/report/executionreport_test.go`, `internal/report/agentlist_test.go`,
  `internal/report/json_test.go`, and `internal/report/text_test.go` passes unmodified.

- [X] T035 Run every gate in `plan.md` "Quality Gates": `gofmt -l .` (no output),
  `go vet ./...` (no diagnostics), `go test ./...` (all pass), `go test -count=1 ./...`
  (all pass, no cache reliance), `go build ./cmd/ai-dev` (clean build, `go.mod`
  unchanged), `git diff --check` (no whitespace errors).

- [X] T036 Perform the security/scope audit checklist from `plan.md` "Quality Gates"
  against the finished diff: no new third-party dependency; `internal/role` and
  `internal/selection` import neither `internal/execution` nor `system.Launcher`;
  `RunAgentSelect`'s parameters contain no `system.Executor`/`system.Launcher`/process-
  supervision `context.Context`; no authentication/network probe; no permission/sandbox/
  approval configuration touched; no secret/token/credential/environment value in any
  code path or fixture; no routing/retry/fallback/chaining/background-execution/
  persistence/dynamic-plugin/MCP-orchestration behavior introduced.

**Human Implementation Checkpoint (T001–T036)**: This — not Phase 3's happy-path
milestone — is the first point at which this feature may be presented for independent
Human Task Gate review. By T036, all of the following are verified: all three user
stories (`role_selected`, `role_unsupported`, `agent_unavailable`); all four canonical
outcomes including `selection_inconsistent`'s three-layer coverage; complete
usage-failure coverage (including the explicit empty-role case); report-write-failure
behavior (including the invalid-report validation coverage); both no-execution proofs
(`Launcher.Launch` and `Executor.Run`, each with a genuine positive control); meaningful
filesystem non-mutation evidence (snapshotting the invocation's own active working
directory) across `role_selected`, `role_unsupported`, a usage failure, and
`agent_unavailable`; full 001/002 regression protection (T033–T034); and every automated
quality gate plus the security/scope audit (T035–T036). This checkpoint does not itself
constitute Human Task Gate approval — that remains a separate, explicit human decision.

- [X] T037 Run the complete `quickstart.md` end-to-end (§1 through §9) against the
  final built `ai-dev` binary as a last acceptance pass, including the optional "real
  tool" checks. This is the final manual acceptance step, performed *after* the Human
  Implementation Checkpoint above (it depends on T001–T036 all being complete, so it is
  not parallel with them or with anything else), not a prerequisite to reaching it.

**Checkpoint**: Feature complete, fully regression-safe, every declared gate green, and
manual quickstart acceptance performed.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup. **Blocks every user story** — it builds
  the entire four-outcome pipeline (`internal/role`, `internal/selection`,
  `internal/report`'s renderer, `internal/app.RunAgentSelect`, and the dispatcher
  branch) that all three stories verify.
- **User Stories (Phase 3–5)**: All depend only on Foundational completion. They are
  behaviorally independent (each exercises a distinct outcome branch of the same
  already-complete `Select`/`RunAgentSelect` pipeline) and may be done in any order or
  in parallel by different people — **but** T009/T010/T011 (US1), T015–T016 (US2), and
  T021–T023/T024 (US3) append to the *same* three test files
  (`internal/selection/selector_test.go`, `internal/report/roleselection_test.go`,
  `internal/app/agentselect_test.go`) as each other, so within each file, apply the
  story tasks in ascending task-ID order (or have one person own a given file) to avoid
  merge conflicts even though the underlying acceptance criteria are independent.
- **Polish (Phase 6)**: Depends on all three user stories being complete. Its
  **Human Implementation Checkpoint** (after T036, before the final T037 manual pass —
  see Phase 6) is the first point at which this feature may be presented for Human Task
  Gate review; Phase 3's own end-of-phase checkpoint is an internal milestone only.

### User Story Dependencies

- **User Story 1 (P1)**: No dependency on US2/US3 beyond Foundational.
- **User Story 2 (P2)**: No dependency on US1/US3 beyond Foundational.
- **User Story 3 (P3)**: No dependency on US1/US2 beyond Foundational.

### Within Each Phase

- Foundational: **T002 has no prerequisite; T003 depends on T002** (`Report.RequestedRole`
  is typed `role.Role`, so `role.go` must exist first) — they are not parallel. T002–T004
  before T005 (imports); T005 and T006 before T007; T007 before T008.
- Each user story: its "Tests" tasks (package-level, `internal/selection`/
  `internal/report`) verify Foundational's already-correct branches first; its
  `internal/app` tasks then verify the same behavior at the CLI/application boundary;
  its manual-quickstart task runs last.

### Parallel Opportunities

- T002 (Foundational) has no dependency and no same-file sibling — parallel-safe on its
  own; T003 depends on it and is not parallel with it.
- T011 (US1, `internal/report/roleselection_test.go`) is parallel-safe relative to the
  sequential T009 → T010 pair (both edit `internal/selection/selector_test.go` and must
  be applied in order, not concurrently).
- T015 and T016 (US2, different files, no cross-dependency).
- T024 (US3, `internal/report/roleselection_test.go`) is parallel-safe relative to the
  sequential T021 → T022 → T023 chain (all three edit `internal/selection/selector_test.go`
  and must be applied in order, not concurrently).
- T033 and T034 (Polish — independent read-only regression-verification passes; neither
  edits a file the other touches). T037 is **not** parallel with them or with anything
  else: it depends on the T001–T036 Human Implementation Checkpoint being complete (see
  Phase 6), so it always runs last, strictly after T033–T036.
- Different user stories' non-shared-file tasks (e.g., T012 and T017) may be worked by
  different people in parallel once Foundational lands, subject to the shared-file notes
  above.

---

## Parallel Example: User Story 1

```bash
# T011 (a different file) can run alongside the sequential T009 -> T010 pair
# (both edit internal/selection/selector_test.go and must not run concurrently):
Task: "internal/selection/selector_test.go: role_selected for all five roles (T009)"
Task: "  then, same file: Select determinism test (T010)"
Task: "internal/report/roleselection_test.go: role_selected text/json rendering (T011)"
```

---

## Implementation Strategy

### Happy-Path Milestone First (User Story 1 Only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational (CRITICAL — this is most of the feature's actual
   code; it must be fully correct before any story can be verified). Note T003's
   dependency on T002 above.
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: run `quickstart.md` §3 and `go test ./internal/selection/... ./internal/report/... ./internal/app/...`.
5. This is an internal development milestone, not a deployable or Human-Task-Gate-ready
   state (see Phase 3's corrected checkpoint note): `ai-dev agent select <role>`
   correctly reports `role_selected` for an available mapped agent, but public failure
   behavior, report-failure handling, no-execution proofs, filesystem non-mutation
   evidence, and regression protection are not yet verified.

### Incremental Delivery

1. Setup + Foundational → the complete pipeline exists and builds/vets cleanly.
2. Add User Story 1 → validate independently → happy-path milestone (not yet a Human
   Task Gate checkpoint).
3. Add User Story 2 → validate independently (role_unsupported + usage-failure
   distinction, including the explicit empty-role case).
4. Add User Story 3 → validate independently (agent_unavailable +
   selection_inconsistent's three-layer coverage + both no-execution proofs).
5. Polish: regression + quality gates + security/scope audit + full quickstart → **this
   is the Human Implementation Checkpoint** (after T036; see Phase 6).

### Parallel Team Strategy

With multiple contributors:

1. One person completes Setup + Foundational (it is the load-bearing, tightly-coupled
   core; splitting it further adds coordination cost without real parallelism benefit).
2. Once Foundational lands:
   - Contributor A: User Story 1 (`internal/app/agentselect_test.go`'s role_selected
     tests, T012–T014).
   - Contributor B: User Story 2 (T017–T020).
   - Contributor C: User Story 3 (T025–T032, the largest phase — covers both
     `agent_unavailable` and the full `selection_inconsistent` application-level
     coverage plus the no-execution/report-write-failure/no-further-action proofs).
3. Coordinate on the three shared test files per the "Dependencies & Execution Order"
   note above before merging.

---

## Constitution / Testability Re-check (Post Task-Remediation)

*This is a task-level re-check only — `plan.md`'s own Constitution Check tables are not
edited by this remediation, since none of the corrections below change architecture,
behavior, or contracts; they correct task text, dependency/parallel labeling, and test
coverage precision.*

- **Testable by Design**: Strengthened. T018 now explicitly covers the empty-role usage
  case (previously only implied). T024 now covers invalid-report rejection, not only
  valid-report rendering. T028 now asserts an actually-observable signal (`LookPath`
  sequences) instead of an uncountable `Registry.List()` call count. T030 now uses two
  genuinely correct positive controls (`agent run` for `Launcher.Launch`, `doctor` for
  `Executor.Run`) instead of one that could never pass. T013/T019/T031 now make the
  filesystem snapshot meaningful by chdir'ing the invocation under test into the
  snapshotted directory. No acceptance scenario in `spec.md` lost coverage.
- **Security by Design**: Unchanged and still satisfied — no task in this remediation
  adds a new dependency, a new execution capability, a new exported production seam, a
  configuration surface, or any authentication/network probe. `agent.Registry{}`
  remains the only `selection_inconsistent` seam; `t.Chdir` and the corrected `doctor`/
  `agent run` positive controls are test-only mechanisms with no production-code change.
- **Reproducibility and Automation**: Unchanged and still satisfied — every corrected
  task remains a version-controlled, automated `go test` assertion (or, for T014/T020/
  T032/T037, a documented manual `quickstart.md` step); none requires live Claude Code
  or OpenAI/Codex CLI access or undocumented local state.

No complexity exception is introduced by this remediation — every change either removes
an incorrect claim (the impossible `Executor.Run`-via-`agent run` positive control, the
uncountable `List()` assertion, the tautological filesystem snapshot, the
"deployable MVP" claim) or adds precision to an already-planned test (the explicit
empty-role case, invalid-report validation, explicit `[P]`/dependency correctness).
Nothing new was added to `internal/role`, `internal/selection`, `internal/report`, or
`internal/app` beyond what `plan.md` already specifies.

---

## Notes

- [P] tasks touch different files with no dependency on an incomplete task.
- [Story] labels (US1/US2/US3) map every user-story-phase task to its spec.md priority
  for traceability; Setup/Foundational/Polish tasks carry no story label by design.
- This feature adds **zero** new production abstractions beyond what `plan.md`
  specifies: no map-based policy, no `Selector` policy field, no exported test-override
  constructor, no new `agent.Registry` method — `agent.Registry{}` (the Go zero value)
  is the entire `selection_inconsistent` application-level seam (T026), and it requires
  no change to `internal/agent`.
- Every automated test in this feature uses a fake/local `system.Executor`/
  `system.Launcher` and requires no live Claude Code or OpenAI/Codex CLI service access.
- Commit after each task or logical group; stop at any checkpoint to validate a story
  independently before continuing.
- Avoid: vague assertions, testing the same outcome twice for no reason, and any
  production code change outside Phase 2 (Foundational) — every later phase is
  test/verification-only against that already-complete pipeline.
- This remediation pass (Human Task Gate review follow-up) is task-text, dependency, and
  test-coverage cleanup only: it changed no architecture, no behavioral contract, and no
  planning artifact other than this file. The task count remains **37** (T001–T037); the
  five-role model, the fixed immutable `mappedAgent` production mapping, the four
  canonical outcomes, the three-layer `selection_inconsistent` coverage model,
  `Registry.List()` reuse, `LookPath`-only availability, the structural no-execution
  boundary, "no fallback/no further action," report-failure-as-a-distinct-category,
  001/002 regression protection, the final automated quality gates, the security/scope
  audit, and the "no live Claude/OpenAI dependency, no auth/network probes, no
  routing/retry/chaining/background/plugins/MCP/configuration" guarantees are all
  unchanged.
- Phase 3's end-of-phase checkpoint is an internal development milestone only. The first
  Human implementation checkpoint is defined after T036 (Phase 6) — see that phase for
  the full composition. This remediation does not claim Human Task Gate approval.
