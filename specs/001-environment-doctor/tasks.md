---

description: "Implementation tasks for Environment Doctor"
---

# Tasks: Environment Doctor

**Input**: Design documents from `/specs/001-environment-doctor/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`, and `.specify/memory/constitution.md`

**Tests**: Automated tests are required by the feature request, specification, implementation plan, and project constitution. Within each story, write the listed tests first and confirm they fail for the expected reason before implementing the corresponding behavior.

**Organization**: Tasks are grouped by user story so each story produces an independently testable increment while preserving the approved package boundaries (`internal/doctor`, `internal/checks`, `internal/system`, `internal/report`, and `internal/app`).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel with other tasks marked `[P]` in the same ready task group because it changes different files and has no dependency on an incomplete peer task
- **[Story]**: Maps the task to User Story 1, 2, or 3 from `spec.md`
- Every task names the file path it creates, changes, validates, or uses as its executable source of truth

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the approved standard-library-only Go module and pinned toolchain baseline.

- [X] T001 Initialize the standard-library-only module and pin the approved Go 1.27.1 language/toolchain baseline in `go.mod`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Implement the technology-agnostic readiness model and the sole process boundary required by every story.

**⚠️ CRITICAL**: No user story implementation begins until this phase is complete.

- [X] T002 [P] Write failing table-driven tests for `Status`, `CheckResult`, `Check`, `Report`, and `RunAll` in `internal/doctor/check_test.go`, preserving these data-model constraints verbatim: `Status` has "exactly two values" (`Pass`, `Fail`); `ID` is a "Stable, kebab-case identifier" that "Never changes meaning once shipped"; `Status` is "Pass or Fail"; `Explanation` is "Required and non-empty when Status == Fail" and "empty when Pass"; `Remediation` is "Required and non-empty when Status == Fail" and "Empty when Pass"; `Checks` has "One entry per executed Check, in the order they were run"; and `Overall` is `Pass` "iff every entry in Checks is Pass"
- [X] T003 Implement the technology-agnostic `Status`, `CheckResult`, `Check`, `Report`, and non-short-circuiting `RunAll` domain model in `internal/doctor/check.go` until `internal/doctor/check_test.go` passes, without importing Git-, Spec Kit-, Claude-, or Codex-specific knowledge
- [X] T004 [P] Write failing tests for `Executor.LookPath` and fixed-argument command execution, including stdout and error propagation, in `internal/system/executor_test.go`
- [X] T005 Implement the narrow `Executor` interface and real `os/exec`-backed implementation in `internal/system/executor.go`, keeping all external process lookup and invocation behind this boundary

**Checkpoint**: Domain aggregation and process execution are independently testable; concrete checks may now be built against fakes.

---

## Phase 3: User Story 1 — Check readiness before starting work (Priority: P1) 🎯 MVP

**Goal**: In a fully ready environment and repository, run all nine checks in stable order, show every check as passing in human-readable text, report `Overall: Ready`, and return success deterministically.

**Independent Test**: With fake process results and an in-memory fully-ready repository filesystem, invoke the doctor twice and confirm both runs contain exactly the nine stable IDs in contract order, every status is Pass, human output lists every Name/Area/result, both overall results are Ready, and both exit codes are `0`.

### Tests for User Story 1

- [X] T006 [P] [US1] Write failing table-driven Pass-path tests for the reusable tool lookup mechanism configured as Git (`git-tool`/`git`), Spec Kit (`speckit-tool`/`specify`), Claude Code (`claude-code-tool`/`claude`), and Codex CLI (`codex-cli-tool`/`codex`) in `internal/checks/tool_test.go`
- [X] T007 [P] [US1] Write failing Pass-path tests proving `GitRepoCheck` delegates working-tree detection to the fake `system.Executor` with exactly `git rev-parse --is-inside-work-tree`, accepts trimmed `true`, and does not inspect `.git` directly in `internal/checks/gitrepo_test.go`
- [X] T008 [P] [US1] Write failing Pass-path tests for parsing a valid scaffold manifest and accepting every listed repository-relative file when it exists and is non-empty in `internal/checks/manifest_test.go`
- [X] T009 [P] [US1] Write failing Pass-path tests that the Spec Kit initialization check uses only `.specify/integrations/speckit.manifest.json` and ignores coding-agent installation records/files in `internal/checks/speckit_test.go`
- [X] T010 [P] [US1] Write failing Pass-path tests for a non-empty `.specify/memory/constitution.md` with no `\[[A-Z][A-Z0-9_]*\]` placeholder and non-blank `**Version**:` and `**Ratified**:` values in `internal/checks/constitution_test.go`
- [X] T011 [P] [US1] Write failing Pass-path tests for the reusable Claude and Codex integration mechanism, including membership in `.specify/integration.json`, valid manifests, non-empty required files, and customized content that differs from recorded SHA-256 values in `internal/checks/integration_test.go`

### Implementation for User Story 1

- [X] T012 [P] [US1] Implement reusable `ToolCheck` lookup and Pass results for all four environment tools through `system.Executor.LookPath` in `internal/checks/tool.go`
- [X] T013 [P] [US1] Implement `GitRepoCheck` in `internal/checks/gitrepo.go` by invoking `git rev-parse --is-inside-work-tree` exclusively through `system.Executor`, passing only on zero exit with trimmed stdout equal to `true`
- [X] T014 [P] [US1] Implement the shared read-only manifest parser/file validator in `internal/checks/manifest.go` using `io/fs.FS`, enforcing the verbatim data-model rule that `files` is the "authoritative list of paths" that "exist and are non-empty" while deliberately not comparing recorded SHA-256 values
- [X] T015 [US1] Implement the core-only Spec Kit initialization check over `.specify/integrations/speckit.manifest.json` in `internal/checks/speckit.go` using the `ValidateManifestFiles` function from T014
- [X] T016 [P] [US1] Implement the approved structural constitution heuristic in `internal/checks/constitution.go`: file exists, is non-empty, has no unresolved `\[[A-Z][A-Z0-9_]*\]` token, and has non-blank `**Version**:` and `**Ratified**:` markers
- [X] T017 [US1] Implement reusable `ManifestIntegrationCheck` for Claude and Codex in `internal/checks/integration.go` using T014 and the verbatim record constraint that `<name>` "requires `<name>` to be present in `installed_integrations`", without byte-for-byte hash validation
- [X] T018 [P] [US1] Write failing ready-fixture orchestration tests asserting all nine stable IDs, names, areas, order, Pass statuses, deterministic repeat results, and overall Ready in `internal/app/doctor_test.go`
- [X] T019 [P] [US1] Write failing human-report tests asserting Environment/Repository grouping, all nine Name/Area/PASS entries, and the final `Overall: Ready` line in `internal/report/text_test.go`
- [X] T020 [US1] Implement the plain-text `fmt`/`text/tabwriter` renderer for the ready report in `internal/report/text.go` until `internal/report/text_test.go` passes
- [X] T021 [US1] Assemble exactly these checks in contract order—`git-tool`, `speckit-tool`, `claude-code-tool`, `codex-cli-tool`, `git-repository`, `speckit-initialized`, `constitution`, `claude-integration`, `codex-integration`—using the injected Executor and repository-rooted `io/fs.FS`, call `doctor.RunAll`, render text, and map Ready to exit `0` in `internal/app/doctor.go`
- [X] T022 [US1] Wire the `doctor` subcommand to the real Executor, current-directory `os.DirFS`, stdout/stderr, and `os.Exit` boundary in `cmd/ai-dev/main.go`

**Checkpoint**: User Story 1 is independently runnable as the MVP: a fully-ready fixture produces nine PASS results, `Overall: Ready`, deterministic output, and exit `0`.

---

## Phase 4: User Story 2 — Diagnose and fix a specific missing prerequisite (Priority: P2)

**Goal**: Every failed prerequisite names what is missing or invalid, gives a concise corrective action, and cannot prevent any other check from completing.

**Independent Test**: Use fakes/`fstest.MapFS` to fail each prerequisite once in isolation, then fail all nine together; confirm all nine results are still present, only affected checks change in isolated cases, every Fail has non-empty Explanation and Remediation, text output shows both fields, and overall is Not Ready.

### Tests for User Story 2

- [X] T023 [P] [US2] Add failing tests for each of the four undiscoverable binaries, Executor lookup errors, non-empty tool-specific diagnosis/remediation, and panic-to-Fail containment in `internal/checks/tool_test.go`
- [X] T024 [P] [US2] Add failing tests for Git command-not-found, non-zero exit, `false`, empty/malformed output, no-commit `true`, and panic-to-Fail containment while still using the Executor abstraction in `internal/checks/gitrepo_test.go`
- [X] T025 [P] [US2] Add failing tests for missing/malformed manifests, absent/empty listed files, aggregated affected-file diagnostics, safe relative paths, and panic-to-Fail containment in `internal/checks/manifest_test.go`
- [X] T026 [P] [US2] Add failing tests for absent/incomplete core `.specify` scaffold parts and prove agent integration files never affect `speckit-initialized` in `internal/checks/speckit_test.go`
- [X] T027 [P] [US2] Add failing tests for a missing, empty, placeholder-bearing, versionless, and ratification-date-less constitution plus non-empty diagnosis/remediation and panic-to-Fail containment in `internal/checks/constitution_test.go`
- [X] T028 [P] [US2] Add failing Claude/Codex cases for absent/malformed `.specify/integration.json`, missing `installed_integrations` membership, absent/malformed integration manifest, missing/empty required files, independent results, permitted customization, and panic-to-Fail containment in `internal/checks/integration_test.go`

### Implementation for User Story 2

- [X] T029 [P] [US2] Complete `ToolCheck` Fail behavior and local panic recovery so every lookup failure returns tool-specific non-empty Explanation and Remediation in `internal/checks/tool.go`
- [X] T030 [P] [US2] Complete `GitRepoCheck` Fail behavior and local panic recovery for command lookup/invocation/output failures, explaining that Git could not establish a working tree while preserving the fixed Executor call in `internal/checks/gitrepo.go`
- [X] T031 [US2] Complete manifest and Spec Kit scaffold failure aggregation, invalid-path handling, local panic recovery, and actionable diagnostics in `internal/checks/manifest.go` and `internal/checks/speckit.go`
- [X] T032 [P] [US2] Complete constitution failure classification, local panic recovery, and actionable diagnosis/remediation for every required structural condition in `internal/checks/constitution.go`
- [X] T033 [US2] Complete reusable Claude/Codex installation-record and required-file failure handling, local panic recovery, affected integration/file/requirement naming, and independent results in `internal/checks/integration.go`
- [X] T034 [P] [US2] Add failing aggregation tests proving `RunAll` preserves input order, executes later checks after earlier Fail results, produces exactly one result per check, and derives Not Ready for any Fail in `internal/doctor/check_test.go`
- [X] T035 [P] [US2] Add failing scenario tests for each single-check failure, outside-repository state, absent `.specify`, all nine failures together, unaffected-check stability, and exit `1` in `internal/app/doctor_test.go`
- [X] T036 [P] [US2] Add failing text-renderer tests requiring every FAIL entry to include legible non-empty Explanation and Remediation followed by `Overall: Not Ready` in `internal/report/text_test.go`
- [X] T037 [US2] Complete the human-readable failure renderer without colors or interactive/TUI dependencies in `internal/report/text.go`
- [X] T038 [US2] Integrate non-short-circuiting failure results and Not Ready exit `1` while keeping repository-dependent failures independent of the four environment-tool checks in `internal/app/doctor.go`

**Checkpoint**: User Story 2 independently demonstrates every documented Fail path, actionable human output, all-check completion, and Not Ready behavior without real tool installations.

---

## Phase 5: User Story 3 — Gate automated workflows on readiness (Priority: P3)

**Goal**: Offer stable JSON selected by `--format json` and process exit behavior `0 = Ready`, `1 = Not Ready`, `2 = usage error`, identically across output formats.

**Independent Test**: Invoke the app with ready and failing fake reports in both formats, validate JSON fields and exact nine-ID order against `contracts/doctor-report.schema.json`, confirm scripts need only `overall`, `checks[].id`, and `checks[].status`, and assert exit codes `0`, `1`, and `2` including no check execution on usage errors.

### Tests for User Story 3

- [X] T039 [P] [US3] Write failing JSON contract tests that decode output and enforce `overall`, exactly nine ordered `checks`, stable ID enum values, `name`, `area`, `status`, `explanation`, and `remediation`, with empty diagnostic strings on Pass and non-empty strings on Fail, using `specs/001-environment-doctor/contracts/doctor-report.schema.json` as the source of truth in `internal/report/json_test.go`
- [X] T040 [P] [US3] Write failing CLI orchestration tests for default text, `--format text`, `--format json`, no positional arguments, invalid/unknown flags and formats, usage on stderr, no check execution on usage errors, and format-independent `0`/`1`/`2` codes in `internal/app/doctor_test.go`

### Implementation for User Story 3

- [X] T041 [US3] Implement deterministic UTF-8 JSON serialization with the contract field names and `pass`/`fail`, `ready`/`not_ready` mappings in `internal/report/json.go`, conforming to `specs/001-environment-doctor/contracts/doctor-report.schema.json`
- [X] T042 [US3] Implement stdlib `flag` parsing for `ai-dev doctor [--format text|json]`, renderer selection, usage errors before check execution, and exit mapping `0 = Ready`, `1 = Not Ready`, `2 = usage error` in `internal/app/doctor.go`
- [X] T043 [US3] Add subprocess-level command contract tests covering `doctor`, both formats, invalid flags/formats, positional arguments, stdout/stderr routing, and observable process exits `0`, `1`, and `2` using temporary repository fixtures and fake PATH executables—never real Git, Spec Kit, Claude Code, or Codex CLI installations—in `cmd/ai-dev/main_test.go`

**Checkpoint**: User Story 3 provides a stable automation contract without parsing human prose and preserves the same readiness exit semantics in text and JSON modes.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Make local reproduction explicit, verify the integrated acceptance surface, and then finish with every concrete quality gate declared by the approved plan.

- [X] T044 [P] Document the pinned Go prerequisite and exact local build, run, format, vet, and automated-test commands plus their pass conditions in `specs/001-environment-doctor/quickstart.md`
- [X] T045 Execute and reconcile every Ready, isolated-failure, all-fail, outside-repository, repeat-run determinism, text, JSON, and cleanup scenario in `specs/001-environment-doctor/quickstart.md` without weakening `specs/001-environment-doctor/spec.md`, `specs/001-environment-doctor/contracts/cli-contract.md`, or `specs/001-environment-doctor/contracts/doctor-report.schema.json`
- [X] T046 Run the declared Formatting gate `gofmt -l .` from repository root `./` and pass only when output is empty
- [X] T047 Run the declared Static analysis gate `go vet ./...` from repository root `./` and pass only when it exits `0` with no reported diagnostics
- [X] T048 Run the declared Automated tests gate `go test ./...` from repository root `./` and pass only when it exits `0`, every unit test passes, and every check's Pass and Fail paths are covered through fakes without requiring real Git, Spec Kit, Claude Code, or Codex CLI installations
- [X] T049 Run the declared Build gate `GOOS=darwin GOARCH=arm64 go build ./...` and then `go build ./...` from repository root `./`; pass only when both exit `0` and the resulting `cmd/ai-dev/main.go` program is runnable

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: Starts immediately.
- **Foundational (Phase 2)**: Depends on T001 and blocks every user story.
- **User Story 1 (Phase 3)**: Depends on Phase 2; supplies the complete ready-path MVP and all nine concrete checks.
- **User Story 2 (Phase 4)**: Depends on User Story 1 because it completes the failure behavior of the same nine checks and human renderer.
- **User Story 3 (Phase 5)**: Depends on User Story 1's complete report/check set; its JSON renderer and flag tests may proceed in parallel with User Story 2, but final Not Ready integration validation uses both stories.
- **Polish (Phase 6)**: Depends on all selected stories. T044 can begin once commands stabilize; T045 follows all implementation; T046 → T047 → T048 → T049 are the final ordered quality gates.

### User Story Dependency Graph

```text
Setup → Foundation → US1 (MVP) → US2 ─┐
                            └──→ US3 ─┴→ Polish → gofmt → vet → test → build
```

### Within Each User Story

- Write each listed test first and confirm it fails for the intended missing behavior.
- Implement reusable mechanisms before their thin story-specific configuration/wiring.
- Keep `internal/doctor` technology-agnostic and all process calls in `internal/system`.
- Run every check regardless of earlier Fail results; preserve the nine-check contract order.
- Complete the story checkpoint before treating that story as deliverable.

### Parallel Opportunities

- T002 and T004 can run in parallel; T003 follows T002 and T005 follows T004.
- US1 Pass-path tests T006–T011 can run in parallel. After their tests exist, T012, T013, T014, and T016 can run in parallel; T015 and T017 wait only for T014. T018 and T019 can run in parallel after the check implementations, while T020 and T021 follow their respective tests.
- US2 failure tests T023–T028 can run in parallel. T029, T030, and T032 can then run in parallel; T031 follows the manifest tests, and T033 follows integration/manifest behavior. T034–T036 can run in parallel after concrete failure behavior stabilizes.
- After US1, US3's T039 and T040 can run in parallel with one another and with US2 work. T041 follows T039; T042 follows T040 and T041.
- T044 changes only documentation and can run in parallel with late test hardening after the documented commands and behavior are stable.

---

## Parallel Example: User Story 1

```text
Task T006: ToolCheck ready-path tests in internal/checks/tool_test.go
Task T007: GitRepoCheck Executor-delegation tests in internal/checks/gitrepo_test.go
Task T008: Manifest validator ready-path tests in internal/checks/manifest_test.go
Task T010: Constitution ready-path tests in internal/checks/constitution_test.go
Task T011: Integration ready-path tests in internal/checks/integration_test.go
```

## Parallel Example: User Story 2

```text
Task T023: Tool failure tests in internal/checks/tool_test.go
Task T024: Git repository failure tests in internal/checks/gitrepo_test.go
Task T025: Manifest failure tests in internal/checks/manifest_test.go
Task T027: Constitution failure tests in internal/checks/constitution_test.go
Task T028: Integration failure tests in internal/checks/integration_test.go
```

## Parallel Example: User Story 3

```text
Task T039: JSON/schema contract tests in internal/report/json_test.go
Task T040: CLI format/exit contract tests in internal/app/doctor_test.go
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Setup and Foundational phases.
2. Write and fail the US1 tests.
3. Implement all nine ready-path checks, text reporting, orchestration, and entry point.
4. Stop at the US1 checkpoint and verify nine PASS results, Ready, deterministic repeat output, and exit `0`.

### Incremental Delivery

1. Deliver US1 as the single-command, fully-ready human-facing MVP.
2. Add US2's independent, actionable diagnostics and all-failure resilience; re-run US1 tests.
3. Add US3's JSON/schema compatibility surface and `0`/`1`/`2` automation contract; re-run US1 and US2 tests.
4. Reproduce quickstart scenarios, then execute the final four quality gates in order.

### Parallel Team Strategy

1. Complete the shared module, domain, and Executor foundation first.
2. Within US1 and US2, assign different check/test file pairs to different implementers according to the `[P]` markers.
3. Once US1 is stable, JSON/report work for US3 may proceed alongside US2 failure-path work because the files do not overlap, except that `internal/app/doctor.go` integration remains ordered.
4. Merge at story checkpoints and run all existing tests before beginning the next dependent integration task.

---

## Notes

- `[P]` never permits bypassing an explicit prerequisite stated in a task or dependency section.
- The nine checks are fixed and always reported; no plugin registry or dynamic discovery is introduced.
- Git working-tree detection always asks Git through `system.Executor`; no `.git` filesystem heuristic is permitted.
- Filesystem checks use `io/fs.FS` and `testing/fstest.MapFS`; tests do not mutate the developer's real repository.
- The command is read-only, makes no network calls, starts no daemon, stores no state, and does not inspect Claude/Codex authentication.
- Exact manifest-content hash equality, tool version checks, cross-platform support beyond `darwin/arm64`, and new third-party dependencies remain out of scope.
- Commit after each task or coherent task group and preserve the approved architecture and contracts.
