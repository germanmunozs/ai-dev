---

description: "Implementation tasks for Coding Agent Execution"
---

# Tasks: Coding Agent Execution

**Input**: Design documents from `/specs/002-coding-agent-execution/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Automated tests are required by the specification, plan, and project constitution. Write each listed behavioral test before the implementation task that satisfies it and confirm the new test fails for the expected reason.

**Architecture boundaries**: `internal/execution` owns the vendor-neutral lifecycle; `internal/execution/adapters` owns Claude Code and Codex CLI command construction/classification; `internal/instruction` owns instruction-source validation; the new narrow `system.Launcher` sits beside the unchanged `system.Executor`; the initial registry is fixed/static. Do not add routing, retries, fallback, workflows, persistence, authentication probing/management, MCP orchestration, a plugin system, or third-party Go dependencies.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel after its stated dependencies complete because it changes different files
- **[Story]**: User story served by the task (`US1`, `US2`, or `US3`)
- Every task names its affected file(s) or repository area and its dependencies

## Phase 1: Setup (Deterministic Test Infrastructure)

**Purpose**: Establish reusable, deterministic subprocess fixtures before process supervision, adapters, or CLI wiring depend on them.

- [X] T001 Create POSIX test fixtures that emit distinct flushed stdout/stderr pre-exit markers, block until an explicit release, expose their PID for child-termination assertions, and support caller-selected normal/non-zero exits in `internal/system/testdata/streaming-agent.sh` and `internal/system/testdata/exit-agent.sh` (dependencies: none)

---

## Phase 2: Foundational (Blocking Generic Abstractions)

**Purpose**: Build and test the vendor-neutral input and process boundaries shared by every user story.

**CRITICAL**: Complete this phase before registry, vendor-adapter, or CLI work begins.

- [X] T002 [P] Add table-driven failing tests for argument-only input, non-interactive stdin through EOF, exact preservation of non-empty text, missing input, interactive stdin without an argument, zero-length input, whitespace-only input, and ambiguous argument plus non-interactive stdin in `internal/instruction/instruction_test.go` (dependencies: T001)
- [X] T003 Implement `Resolve(arg, hasArg, stdin, stdinIsInteractive)` and its stable validation errors in `internal/instruction/instruction.go`, without probing the terminal or invoking a process (depends on T002)
- [X] T004 [P] Define the narrow `system.Spec`, `system.ExitStatus`, `system.Handle`, and `system.Launcher` contracts in `internal/system/launcher.go`; do not modify `internal/system/executor.go` or `system.Executor` (dependencies: T001)
- [X] T005 Add failing real-subprocess tests for working-directory propagation, direct argument passing, normal versus abnormal termination, handled cancellation with child termination before `Wait` returns, and live stdout/stderr streaming using the pre-exit marker/release fixture in `internal/system/oslauncher_test.go` (depends on T001, T004)
- [X] T006 Implement `system.OSLauncher` with `os/exec.CommandContext`, direct writer wiring, interrupt forwarding, bounded `Cmd.WaitDelay`, and accurate normal/abnormal `ExitStatus` in `internal/system/oslauncher.go` (depends on T005)
- [X] T007 [P] Define vendor-neutral `execution.Request`, the exact `agent_succeeded`/`agent_failed`/`execution_failed` `Outcome` constants, `execution.Report`, and the `execution.Adapter` contract in `internal/execution/request.go`, `internal/execution/outcome.go`, and `internal/execution/adapter.go` (dependencies: T001)

**Checkpoint**: Instruction validation and the generic process-launching boundary are independently tested; no vendor or CLI behavior has been introduced.

---

## Phase 3: User Story 1 - Discover Which Coding Agents Are Available (Priority: P1) MVP

**Goal**: List the fixed supported-agent set with stable canonical IDs, display labels, and availability derived only from local executable discovery, in deterministic text and JSON forms.

**Independent Test**: With a fake `system.Executor` that finds only `claude`, run the listing twice and verify both outputs list `claude-code` then `codex-cli`, report only the corresponding `available`/`unavailable` states, remain identical, and never launch either executable.

### Tests for User Story 1

- [X] T008 [US1] Add failing registry tests for immutable canonical IDs `claude-code` and `codex-cli`, separate display labels, fixed/static two-agent membership, stable ordering, repeated deterministic results, `Resolve`, and availability based solely on `system.Executor.LookPath` for `claude`/`codex` with no `Run`, authentication, or readiness probe in `internal/agent/agent_test.go` and `internal/agent/registry_test.go` (depends on T004, T007)
- [X] T009 [P] [US1] Add failing renderer tests for ordered text rows and a single JSON object whose availability values are exactly `available` or `unavailable` in `internal/report/agentlist_test.go` (depends on T007)
- [X] T010 [US1] Add failing command tests for default text, explicit text/JSON, supported-agent completeness, deterministic ordering, machine-readable stdout isolation, success when all agents are unavailable, and usage failures that leave stdout empty and perform no discovery in `internal/app/agents_test.go` (depends on T008, T009)

### Implementation for User Story 1

- [X] T011 [US1] Implement `agent.ID`, the two stable constants, `Agent`, `Availability`, and `AgentStatus` in `internal/agent/agent.go`, plus the fixed two-entry `Registry.List`/`Registry.Resolve` using only `system.Executor.LookPath` in `internal/agent/registry.go` (depends on T008)
- [X] T012 [P] [US1] Implement deterministic text and `encoding/json` agent-list rendering in `internal/report/agentlist.go` without changing the existing doctor renderers `internal/report/json.go` or `internal/report/text.go` (depends on T009, T011)
- [X] T013 [US1] Implement `RunAgentsList` with `flag.NewFlagSet`, text/JSON selection, usage exit `2`, and no execution side effects in `internal/app/agents.go` (depends on T010, T011, T012)
- [X] T014 [US1] Extend top-level dispatch for `agents` and thread the already-planned stdin and `system.Launcher` parameters through `internal/app/doctor.go` and `cmd/ai-dev/main.go`, while leaving `RunDoctor`, `internal/checks/`, `internal/doctor/`, and the existing doctor renderer behavior unchanged (depends on T006, T013)
- [X] T015 [US1] Update only the mechanical `app.Run` call signatures and add listing dispatch coverage in `internal/app/doctor_test.go` and `cmd/ai-dev/main_test.go`, retaining the existing doctor assertions and nine-check expectations unchanged (depends on T014)

**Checkpoint**: User Story 1 is independently usable in text and JSON modes and performs discovery only.

---

## Phase 4: User Story 2 - Run an Explicitly Chosen Agent (Priority: P2)

**Goal**: Run exactly one explicitly selected, available agent in the current directory with an argument or stdin instruction; stream native output live; handle cancellation; and emit one final canonical outcome.

**Independent Test**: With deterministic fake discovery/launch boundaries, run each supported canonical ID with both instruction sources and verify one process receives the exact instruction and workdir, live output reaches the required streams, cancellation stops the child, and the final report is `agent_succeeded` only for a normal successful exit.

### Tests for User Story 2

- [X] T016 [P] [US2] Add failing Claude adapter tests for the exact one-shot command vector, unmodified shell-significant/multiline instruction, current workdir, zero/non-zero exit classification, and absence of permission-elevation, sandbox-bypass, auto-approval, authentication, retry, or fallback flags in `internal/execution/adapters/claude_test.go` (depends on T007, T011)
- [X] T017 [P] [US2] Add failing Codex adapter tests for the exact one-shot command vector, unmodified shell-significant/multiline instruction, current workdir, zero/non-zero exit classification, and absence of permission-elevation, sandbox-bypass, auto-approval, authentication, retry, or fallback flags in `internal/execution/adapters/codex_test.go` (depends on T007, T011)
- [X] T018 [US2] Add failing runner tests for exact request/workdir propagation, exactly one launch, `agent_succeeded` on a normal adapter-success exit, handled context cancellation producing `execution_failed`, waiting until the fake child is stopped, and no retry/fallback/second-agent invocation in `internal/execution/runner_test.go` (depends on T006, T007, T011)
- [X] T019 [P] [US2] Add failing execution-report tests for a visibly delimited final human record on stderr, exact canonical outcome text, requested canonical agent ID, optional success explanation, and one isolated JSON object with `agent_id`, `outcome`, and `explanation` in `internal/report/executionreport_test.go` (depends on T007)
- [X] T020 [US2] Add failing application tests for argument instruction input, non-interactive stdin instruction input, equivalent request construction, exact current-directory propagation, one selected agent only, human stdout/stderr byte preservation and per-stream order, JSON-mode routing of both agent streams to stderr, machine-readable stdout isolation, final canonical outcome ordering after completion, successful exit code `0`, and handled cancellation exit `1` in `internal/app/agentrun_test.go` (depends on T003, T018, T019)

### Implementation for User Story 2

- [X] T021 [P] [US2] Implement the Claude Code adapter in `internal/execution/adapters/claude.go`, using the vendor's confirmed one-shot/non-interactive invocation from its CLI help and no flags that relax permissions, approvals, or sandboxing (depends on T016)
- [X] T022 [P] [US2] Implement the Codex CLI adapter in `internal/execution/adapters/codex.go`, using the vendor's confirmed one-shot/non-interactive invocation from its CLI help and no flags that relax permissions, approvals, or sandboxing (depends on T017)
- [X] T023 [US2] Implement the vendor-neutral `execution.Runner` resolve/check/build/launch/wait/classify/report lifecycle and fixed adapter map in `internal/execution/runner.go`, with no vendor-specific branches, retry, fallback, routing, workflow state, or background execution (depends on T018, T021, T022)
- [X] T024 [P] [US2] Implement human and JSON execution-report writers in `internal/report/executionreport.go`, keeping final reports separate from agent output and never incorporating agent output or environment values into the report (depends on T019)
- [X] T025 [US2] Implement `RunAgentRun` instruction resolution, terminal-state input, output routing, current-workdir request, `execution.Runner` invocation, final rendering, and outcome-to-exit-code mapping in `internal/app/agentrun.go` (depends on T003, T020, T023, T024)
- [X] T026 [US2] Extend the `agent run [--format text|json] <agent-id> [instruction]` dispatch and wire `signal.NotifyContext` plus `system.OSLauncher` from the executable entry point in `internal/app/doctor.go` and `cmd/ai-dev/main.go` (depends on T014, T025)
- [X] T027 [US2] Add an executable-level cancellation regression proving the handled interrupt stops the fixture child before exit and reports `execution_failed` when reporting remains possible in `cmd/ai-dev/main_test.go` (depends on T001, T026)

**Checkpoint**: User Story 2 completes one explicit agent execution with live output, cancellation supervision, and a final report; it performs no automatic selection or orchestration.

---

## Phase 5: User Story 3 - Distinguish Execution Failure from Agent Failure (Priority: P3)

**Goal**: Make every recognized failure unambiguous while keeping malformed syntax outside the execution outcome model.

**Independent Test**: Exercise unsupported, unavailable, launch-error, abnormal termination, and normal non-zero fixtures plus malformed CLI forms; verify recognized failures emit exactly one appropriate `execution_failed` or `agent_failed` report and exit `1`, while usage failures emit no report, leave stdout empty, invoke no process, and exit `2`.

### Tests for User Story 3

- [X] T028 [US3] Add failing lifecycle tests for unsupported agent, unavailable agent based only on local discovery, missing adapter invariant handling, process launch failure, wait/communication failure, abnormal termination, normal non-zero `agent_failed`, and normal zero `agent_succeeded`, asserting exact canonical outcomes, non-empty failure explanations, requested-ID preservation, and zero launches for pre-launch rejection in `internal/execution/runner_test.go` (depends on T023)
- [X] T029 [US3] Add failing command tests for missing, zero-length, and whitespace-only instructions; interactive stdin; ambiguous argument plus stdin; unsupported agent; unavailable agent; process launch failure; `agent_failed`; `execution_failed`; and absence of `agent_succeeded` from every failure report in `internal/app/agentrun_test.go` (depends on T025)
- [X] T030 [US3] Add failing usage-contract tests for missing agent ID, too many positionals, unknown flags, and invalid format versus recognized validation/execution failures, asserting exit `2` plus usage/no report/no launch for the former and exit `1` plus one final report for the latter in `internal/app/agentrun_test.go` (depends on T029)
- [X] T031 [P] [US3] Add failing renderer tests that require non-empty explanations for `agent_failed` and `execution_failed`, exact final canonical outcome values, unsupported requested-ID round-tripping, and no success-like text in failure records in `internal/report/executionreport_test.go` (depends on T024)

### Implementation for User Story 3

- [X] T032 [US3] Complete runner rejection and termination classification in `internal/execution/runner.go` so unsupported/unavailable/launch/wait/abnormal cases are `execution_failed`, normal non-zero completion is `agent_failed`, and adapters never classify abnormal exits (depends on T028)
- [X] T033 [US3] Complete recognized-request validation, usage separation, failure explanations, final reporting, and unsuccessful exit mapping in `internal/app/agentrun.go` and `internal/app/doctor.go` without launching on validation or usage failures (depends on T029, T030, T032)
- [X] T034 [US3] Enforce canonical failure rendering and requested-ID preservation in `internal/report/executionreport.go` for both human and JSON modes (depends on T031, T033)

**Checkpoint**: All three outcomes and usage failures are deterministically distinguishable in both output modes.

---

## Phase 6: Polish & Cross-Cutting Quality Gates

**Purpose**: Prove architecture boundaries, regression safety, and reproducible repository health without expanding scope.

- [X] T035 Add a final cross-package contract test audit covering both canonical IDs, deterministic listing, local-discovery-only availability, every instruction-source case, all three outcomes, launch/cancellation/child-termination behavior, stream routing/isolation, pre-exit live markers, final canonical reports, and usage-versus-recognized failures across `internal/agent/*_test.go`, `internal/instruction/*_test.go`, `internal/execution/*_test.go`, `internal/system/*_test.go`, `internal/report/*_test.go`, and `internal/app/*_test.go` (depends on T015, T027, T034)
- [X] T036 Verify the existing environment-doctor regression suite still asserts exactly nine ordered checks and unchanged behavior in `internal/app/doctor_test.go` and `cmd/ai-dev/main_test.go`; make no changes to `internal/checks/`, `internal/doctor/`, `internal/report/json.go`, `internal/report/text.go`, `internal/system/executor.go`, or `internal/system/executor_test.go` (depends on T035)
- [X] T037 Verify `go.mod` has no new `require` block and audit `internal/execution/`, `internal/execution/adapters/`, `internal/instruction/`, `internal/system/`, and `internal/app/` for the prohibited plugin, routing, retry/fallback, workflow-state, authentication-management, MCP, privilege-elevation, and secret-output behaviors (depends on T036)
- [X] T038 Run `gofmt -l .` from repository root `./` and require empty output (depends on T037)
- [X] T039 Run `go vet ./...` from repository root `./` and require exit code `0` (depends on T038)
- [X] T040 Run `go test ./...` from repository root `./` and require exit code `0`, including the live-marker and existing doctor nine-check regressions (depends on T039)
- [X] T041 Run `go build ./...` from repository root `./` and require exit code `0` (depends on T040)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 — Setup**: Starts immediately.
- **Phase 2 — Foundational**: Depends on T001 and blocks all user stories.
- **Phase 3 — US1**: Depends on the generic types and boundaries from Phase 2. It establishes the fixed registry required by execution.
- **Phase 4 — US2**: Depends on US1's registry plus all foundational boundaries. Vendor adapters and CLI execution wiring start only after those abstractions and fixtures exist.
- **Phase 5 — US3**: Depends on US2's end-to-end successful execution path, then hardens every failure and usage boundary.
- **Phase 6 — Polish**: Depends on all three user stories and runs the final reproducible gates serially.

### User Story Dependency Graph

```text
Setup T001
  └─ Foundational T002-T007
       └─ US1 T008-T015 (fixed registry + listing)
            └─ US2 T016-T027 (explicit execution + streaming/cancellation)
                 └─ US3 T028-T034 (failure distinction + usage contract)
                      └─ Quality gates T035-T041
```

The stories are independently testable at their checkpoints, but implementation order is intentionally sequential because US2 consumes US1's fixed registry and US3 validates the failure behavior of US2's lifecycle.

### Within Each User Story

- Write the listed tests first and confirm they fail for the expected missing behavior.
- Complete models/contracts before services or command wiring.
- Keep vendor adapters behind `execution.Adapter`; do not branch on vendor inside `execution.Runner`.
- Complete library/application behavior before executable-level integration tests.
- Do not advance past a checkpoint until that story's independent test passes.

### Parallel Opportunities

- After T001, T002, T004, and T007 affect separate packages and can proceed in parallel.
- In US1, renderer tests T009 can proceed alongside registry tests T008; renderer implementation T012 can proceed after its own prerequisites while command work waits on both.
- In US2, Claude and Codex adapter test/implementation pairs T016/T021 and T017/T022 can proceed in parallel; execution-report work T019/T024 can proceed alongside runner work T018/T023.
- In US3, command failure tests T029-T030 and renderer tests T031 can proceed in parallel after their stated prerequisites.

---

## Parallel Examples

### User Story 1

```text
Task T008: Registry identity, ordering, resolution, and local-discovery tests in internal/agent/
Task T009: Listing text/JSON renderer tests in internal/report/
```

### User Story 2

```text
Task T016 → T021: Claude Code adapter tests and implementation in internal/execution/adapters/claude_*.go
Task T017 → T022: Codex CLI adapter tests and implementation in internal/execution/adapters/codex_*.go
Task T019 → T024: Final execution-report tests and implementation in internal/report/executionreport*.go
```

### User Story 3

```text
Task T029 → T030: Recognized failure and usage-versus-execution command tests in internal/app/agentrun_test.go
Task T031: Failure renderer tests in internal/report/executionreport_test.go (parallel with T029-T030)
```

---

## Implementation Strategy

### MVP First (User Story 1)

1. Complete T001-T007 to establish deterministic fixtures and generic boundaries.
2. Complete T008-T015 for the fixed supported-agent listing.
3. Stop and run the US1 independent test in both text and JSON modes.

### Incremental Delivery

1. **US1**: Fixed registry and deterministic listing; no execution side effects.
2. **US2**: One explicit agent invocation with exact input, live streams, cancellation, and final success reporting.
3. **US3**: Complete failure classification and usage separation.
4. **Final**: Architecture audit, nine-check doctor regression, formatting, vet, tests, and build.

## Notes

- `[P]` means only the specifically identified files can be worked independently after prerequisites; shared-file edits must be coordinated.
- Availability is only `LookPath` discovery and must never probe authentication, network access, or readiness.
- Each recognized request can launch at most one selected process; no automatic routing, retries, fallback, chaining, or persistence is permitted.
- The exact vendor one-shot arguments must be pinned in adapter tests at implementation time from the corresponding installed CLI help; no permission/sandbox bypass flag is acceptable.
- Shell fixture tests follow the approved POSIX scope and the existing Windows skip convention; this task list makes no broader cross-platform claim.
- No task records or asserts human approval.
