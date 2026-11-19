---

description: "Dependency-ordered implementation tasks for Feature 007 guided mode"
---

# Tasks: Feature 007 — Workflow Orchestration (Guided Mode)

**Input**: Design documents from `specs/007-workflow-orchestration/`: `plan.md`, `spec.md` (revision 3), `research.md`, `data-model.md`, `contracts/workflow-guide.md`, `quickstart.md`.

**Scope**: Guided mode only, F007-FR-001 to F007-FR-015 and SC-001 to SC-007. The owner authorized it under G-IMPLEMENT on 2026-11-19. The specification's deferred autonomous section is not normative, and no task touches it.

**Tests**: Mandatory (constitution IV). In every phase the test tasks come before their implementation, and each test is first confirmed to fail for the missing behavior.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: May run in parallel: it touches a different file and depends on no incomplete task of the same phase.
- **[Story]**: The user story the task serves (US1, US2).
- Every task names its file.

## Path Conventions

This is the single Go project already in use. The domain goes in `internal/guide/`, rendering in `internal/report/`, wiring in `internal/app/`, and the subprocess tests in `cmd/ai-dev/`.

---

## Phase 1: Setup

**Purpose**: Create the package and its dependency boundary before any behavior exists.

- [X] T001 Create the `internal/guide` package documentation in `internal/guide/doc.go`. It states that guide composes `sdd.Evaluate` and the 003 selection by reference (R-01), starts no process, writes nothing, and reads no standard input.
- [X] T002 Add a structural boundary test in `internal/guide/boundary_test.go` that parses every non-test file of `internal/guide`.
  - **Allowed imports:** only `io/fs`, `strings`, `unicode` and `ai-dev/internal/{sdd,selection,agent,role,workflow}`.
  - **Forbidden imports:** `os`, `os/exec`, `net`, `net/http`, `syscall`, and `ai-dev/internal/{system,execution,instruction,gate,gatestore,gitmanager,gitprocess,gitstore,app,report}`.
  - **Forbidden call:** `agent.NewRegistry`. The registry is built by `internal/app` from the injected executor (plan, Dependency rule).

  Because `os` is forbidden, guide cannot read the environment (`os.Getenv`) or any configuration file, so no configuration or environment value can enable anything (US2 acceptance 2). Include negative fixtures that prove each forbidden import fails the test (R-07, SC-002).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the pure model US1 renders: the identifier safety rule and the Command Template.

**⚠️ CRITICAL**: US1 cannot start until this phase is complete.

- [X] T003 [P] Write tests for the template-safe identifier rule in `internal/guide/safety_test.go`, following the data model ("none of its runes is in Unicode category Cc or Cf and none is U+2028 or U+2029").
  - **Unsafe:** NUL, newline, tab, U+007F, U+202E, U+200B, U+FEFF, U+2028 and U+2029.
  - **Safe:** `a;b`, `$(x)`, `-f`, `it's`, `a b`, `ñandú` and `007-workflow-orchestration`.
- [X] T004 Implement `TemplateSafe(featureID string) bool` in `internal/guide/safety.go` using `unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' '`. T003 must pass.
- [X] T005 [P] Write tests for the Command Template model in `internal/guide/templates_test.go`.
  - **Template sets:** the kinds and order for each outcome and availability, exactly as in the data-model table:
    - `step_recommended` with `available`: `select`, `run`, `transition`, `approve`;
    - with `unavailable`: `select`, `transition`, `approve`;
    - with `inconsistent`: `transition`, `approve`;
    - `workflow_terminal`: `state`;
    - every other outcome: none.
  - **Words per kind,** exactly as in "Words by kind".
  - **`HumanSlots`:** it lists the `human` words in order of appearance (`run`: `instruction`; `transition`: `target-state`; `approve`: `approver`, `target-state`).
  - **No reason word:** no template contains a word for `--reason`.
- [X] T006 Implement the Word kinds (`fixed`, `token`, `feature`, `human`), the `Template{Kind, Label, Words, HumanSlots}` type, the fixed labels of `contracts/workflow-guide.md`, and `Templates(outcome, availability, role, agent)` in `internal/guide/templates.go`. T005 must pass.

**Checkpoint**: The pure model is complete and tested, with no rendering and no I/O.

---

## Phase 3: User Story 1 — Guided next-action report (Priority: P1) 🎯 MVP

**Goal**: `ai-dev workflow guide <feature-id>` reports:
- 005's outcome, state, activity and role;
- the role's mapped agent and its availability;
- the command templates and the non-authorization statement.

It executes, writes and derives nothing.

**Independent Test**: For MapFS fixtures in every Workflow State, the invalid cases and the hostile names, the outcome equals `workflow next`'s, the rendered templates satisfy SC-004, no `Run` call happens, and the tree digest is unchanged.

### Tests for User Story 1 ⚠️

> Write these tests first and confirm they fail before implementing T013–T019.

- [X] T007 [P] [US1] SC-001 equivalence tests in `internal/guide/guide_test.go`. Use MapFS fixtures for all seven Workflow States, an unknown feature, an invalid record and missing artifacts. For each, assert that `guide.Evaluate(root, id, selector)` returns the same `Outcome`, `State`, `Activity`, `Role` and `Missing` as `sdd.Evaluate(root, id)`, and that `Agent`, `Availability` and `Templates` are present only for the outcomes the data model allows. Evaluate each fixture twice and assert deep equality (F007-FR-009).
- [X] T008 [P] [US1] SC-003 availability tests in `internal/guide/guide_test.go`, with a fake `Selector` returning each 003 outcome (R-02):
  - `role_selected` → `available` with `SelectedAgentID`;
  - `agent_unavailable` → `unavailable` with `MappedAgentID`;
  - `selection_inconsistent` → `inconsistent` with `MappedAgentID`;
  - `role_unsupported` → `Valid()` is false.

  Also assert that no other agent ever appears, and that the selector is called with exactly the 005 role.
- [X] T009 [P] [US1] Template-safety integration test in `internal/guide/guide_test.go`: an identifier containing U+202E or U+200B yields the full report with an empty `Templates`.
- [X] T010 [P] [US1] Rendering tests in `internal/report/guide_test.go`, against `contracts/workflow-guide.md`:
  - **Text:** the exact first-line forms for all five outcomes, with `feature=%q` escaping. The template lines, labels and closing reminder appear only when a human slot is present.
  - **JSON:** one object with `schema_version` `ai-dev.workflow-guide/v1`; the fields present or omitted per outcome; `templates` always present (`[]` when empty); `human_slots`; `optional_flags` `["--reason"]` on `approve`.
  - **Statement (SC-005, SC-007):** `statement` and the text `note` are present for every outcome.
- [X] T011 [P] [US1] SC-004 shell-paste tests in `internal/report/guide_test.go`.
  - **Setup:** for the hostile identifiers `a;b`, `$(x)`, `-f`, `it's`, `a b` and `ñandú`, render every template. Put a sentinel `ai-dev` script on a temporary `PATH`; it appends its argv to a log file.
  - **Filled:** with every `(slot)` replaced by a quoted value, `sh -c`, `bash -c` and `zsh -c` each invoke the sentinel with the identifier as exactly one argument after `--`.
  - **One slot unfilled:** for each human slot separately, with only that slot unfilled, the sentinel is never invoked and the temporary directory's entries are unchanged. Also test with a file named after the slot present in the directory.
  - **Shells:** skip `zsh` when it is not installed, with `t.Skip`; `sh` and `bash` are required.
  - **No derived values:** no rendered command contains a value for `instruction`, `approver`, `target-state` or `reason`.
- [X] T012 [P] [US1] Command tests in `internal/app/workflowguide_test.go`:
  - **Usage failures (005 FR-019):** none or two identifiers, a whitespace identifier, an unknown option, `--format yaml`. Each gives exit 2, empty stdout and the usage line on stderr.
  - **Exit codes:** they equal `RunWorkflowNext`'s for the same fixtures.
  - **No side effects (SC-002):** the fake executor records zero `Run` calls and only `LookPath`; the stdin reader fails the test if read; the fixture tree digest is unchanged.
  - **Output failure:** a failing writer gives exit 2 with a stderr diagnostic.

### Implementation for User Story 1

- [X] T013 [US1] Implement `Report`, `Availability` (`available` | `unavailable` | `inconsistent`), the `Selector` interface (`Select(string) selection.Report`), `Evaluate(root fs.FS, featureID string, selector Selector) Report` and `(Report) Valid() bool` in `internal/guide/guide.go`.
  - **005:** call `sdd.Evaluate`.
  - **003:** for `step_recommended` only, call `selector.Select(string(role))` and map it per R-02.
  - **Templates:** attach `Templates(...)` only when `TemplateSafe(featureID)`, and set the fixed `Statement`.

  T007–T009 must pass.
- [X] T014 [US1] Implement the shell rendering of Words in `internal/report/guide.go`:
  - `fixed` and `token` words are rendered verbatim;
  - `feature` words become one POSIX single-quoted word with `'` written as `'\''` (R-04);
  - `human(name)` words are rendered unquoted as `(name)` (R-03).

  Also add `RenderCommand(guide.Template) string`.
- [X] T015 [US1] Implement `WriteGuideText(w io.Writer, r guide.Report) error` in `internal/report/guide.go`, following the contract: the first line, the indented `label: command` lines, and the reminder line about `(slot)` and the optional `--reason` (006 FR-010). It refuses an invalid report. T010 and T011 text cases must pass.
- [X] T016 [US1] Implement `WriteGuideJSON(w io.Writer, r guide.Report) error` in `internal/report/guide.go`, emitting exactly one object per the contract. It refuses an invalid report. The T010 JSON cases must pass.
- [X] T017 [US1] Implement `RunWorkflowGuide(args []string, root fs.FS, executor system.Executor, stdout, stderr io.Writer) int` in `internal/app/workflowguide.go`, mirroring `runWorkflowNext`:
  - `brokenPipeAsError`, the `flag` parsing, and the usage line `Usage: ai-dev workflow guide [--format text|json] <feature-id>`;
  - the selector `selection.NewSelector(agent.NewRegistry(executor))`;
  - the exit codes of 005 FR-018: 0 for `step_recommended` and `workflow_terminal`, 1 for the other outcomes, 2 for usage and output failures.

  It never touches stdin. T012 must pass.
- [X] T018 [US1] Route `workflow guide` in `internal/app/app.go` to `RunWorkflowGuide(args[2:], root, executor, stdout, stderr)`, next to `next`. Leave the `workflow` usage line (`Usage: ai-dev workflow <state|transition> [options]`) and the top-level usage line unchanged (F007-FR-013).
- [X] T019 [US1] Subprocess end-to-end test in `cmd/ai-dev/workflowguide_test.go`. Build the binary and run `workflow guide` and `workflow guide --format json` against a temporary repository fixture with `spec.md` in `specified`. Assert the exit code, stdout and an empty stderr, and that the fixture tree digest is unchanged. Run it with `PATH` containing neither `claude` nor `codex`, and assert availability `unavailable` and that no `run` template is shown.

**Checkpoint**: US1 is complete. `quickstart.md` scenarios 1–5 pass.

---

## Phase 4: User Story 2 — No autonomous capability exists (Priority: P1)

**Goal**: No command or option of ai-dev can plan, authorize or start an agent chain, and every existing usage failure stays unchanged.

**Independent Test**: Non-existent `workflow` subcommands and top-level commands are usage failures with exit 2 and unchanged usage text, and `workflow guide` rejects any extra option.

### Tests for User Story 2 ⚠️

- [ ] T020 [P] [US2] Subprocess test in `cmd/ai-dev/workflowguide_test.go`:
  - `workflow run x`, `workflow chain x`, `workflow orchestrate x` and `orchestrate x` each exit 2 with exactly the existing usage line and an empty stdout;
  - `workflow guide --autonomous x` and `workflow guide --execute x` are usage failures (F007-FR-014);
  - none of them creates or changes a file in the fixture tree.
- [ ] T021 [P] [US2] Regression assertion in `internal/app/workflowguide_test.go` that `RunWithDependencies` prints exactly `Usage: ai-dev workflow <state|transition> [options]` for an unknown `workflow` subcommand, and the unchanged top-level usage for an unknown command (F007-FR-013).

### Implementation for User Story 2

- [ ] T022 [US2] Confirm that T018 added no other route or flag. If T020 or T021 fail, correct `internal/app/app.go` or `internal/app/workflowguide.go`, without adding any autonomous surface.

**Checkpoint**: US2 is complete. `quickstart.md` scenario 6 passes.

---

## Phase 5: Polish & Cross-Cutting Concerns

- [ ] T023 [P] Document `ai-dev workflow guide` in `README.md`:
  - add it to the command reference table (read-only);
  - change the 007 status row to "guided mode implemented; autonomous mode deferred";
  - add a short section on the templates and the `(slot)` rule.
- [ ] T024 Run the plan's Quality Gates and record the exact commands and results:
  - `gofmt -l .` and `go vet ./...`, plus `GOOS=linux go vet ./...`;
  - the six-platform cross-build;
  - `go test -count=1 ./...`;
  - `go test -race -count=3 ./internal/guide ./internal/report ./internal/app ./cmd/ai-dev`.
- [ ] T025 Run `quickstart.md` scenarios 1–7 against a fresh build, and confirm that `git status` is unchanged after each read-only command.
- [ ] T026 Independent review of the implementation diff by a separate read-only agent. Record the implementing and reviewing agents, the findings and the resolutions in the PR description (constitution VIII). Fix every blocking or high finding before requesting merge.
- [ ] T027 Open the PR to `dev`, with CI green on macOS and Ubuntu. The owner decides the merge (constitution IX).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none.
- **Foundational (Phase 2)**: depends on Phase 1 and blocks US1.
- **US1 (Phase 3)**: depends on Phase 2.
- **US2 (Phase 4)**: depends on T018, because it verifies the routing T018 adds. Its tests can be written in parallel with US1.
- **Polish (Phase 5)**: depends on US1 and US2.

### Within Each User Story

- Tests come first and must fail before implementation.
- The domain comes before rendering, rendering before command wiring, and wiring before the subprocess tests: T013, then T014–T016, then T017–T018, then T019.

### Parallel Opportunities

- T003 and T005 touch different files.
- T007–T012 are test-only, and all can be written in parallel. T007–T009 share `guide_test.go` and T010–T011 share `report/guide_test.go`, so write each group sequentially if one person does it.
- T020 and T021 can be written alongside US1's tests.

## Parallel Example: User Story 1

```text
T007–T009 internal/guide/guide_test.go  |  T010–T011 internal/report/guide_test.go  |  T012 internal/app/workflowguide_test.go
```

## Implementation Strategy

### MVP First

1. Phases 1 and 2: boundary, safety rule and template model.
2. Phase 3 (US1): the guidance report, end to end.
3. **Stop and validate** with quickstart scenarios 1–5.
4. Phase 4 (US2): prove that no autonomous surface exists.
5. Phase 5: gates, quickstart, independent review and PR.

### Traceability

| Requirement | Tasks |
|---|---|
| F007-FR-001, FR-011 | T012, T017, T018, T019 |
| F007-FR-002 | T007, T013 |
| F007-FR-003 | T005, T011 (no derived value) |
| F007-FR-004 | T008, T013 |
| F007-FR-005 | T005, T006, T010, T011, T015, T016 |
| F007-FR-006 | T003, T004, T009, T011, T014 |
| F007-FR-007 | T002, T012, T019 |
| F007-FR-008 | T010, T015, T016 |
| F007-FR-009 | T007 (repeated evaluation, deep equality), T012 |
| F007-FR-010 | T010, T016 |
| F007-FR-012 | T010, T011 (no environment-derived value) |
| F007-FR-013 | T018, T021 |
| F007-FR-014 | T002 (no environment or configuration reads), T020, T022 |
| F007-FR-015 | T002 (only the owners' packages may be imported), T007 |

## Notes

- The specification's deferred autonomous section is not normative. No task implements, stubs or flags it.
- [P] tasks touch different files and have no unmet dependency.
- Commit after each phase checkpoint, when the owner requests it.
