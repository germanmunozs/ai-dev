# Tasks: Agent Catalog Expansion

**Input**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/catalog.md` and `quickstart.md` in this directory.

**Tests**: mandatory (constitution IV), written before the code they cover. Each test task names the requirement or success criterion it proves. **No test may start a real coding agent** (SC-005): every agent is a fake executable on a private `PATH`, or a fake `system.Executor` / `system.Launcher`.

## Phase 1: Setup

- [X] T001 Confirm the baseline on `feature/013-agent-catalog`: run `gofmt -l .`, `go vet ./...` and `go test ./...`; record the result here. List every test that asserts a catalog of exactly two agents, or doctor's nine checks, with `git grep -n -E 'len\((report\.)?Agents\) != 2|claude-code-tool|codex-cli-tool|Nine|nine' -- '*_test.go'`. Record the list here; these are the expectations the Required Amendments allow to change (SC-007).

## Phase 2: Foundational (blocks every story)

- [X] T002 [P] Tests in `internal/agent/registry_test.go`, written before T003 (F013-FR-002, FR-005, contracts/catalog.md):
  - `Catalog()` returns exactly, in order, `claude-code` "Claude Code" `claude`, `codex-cli` "OpenAI Codex CLI" `codex`, `factory-droid` "Factory Droid" `droid`, `gemini-cli` "Gemini CLI" `gemini`, `kiro-cli` "Kiro CLI" `kiro-cli`, `mistral-vibe` "Mistral Vibe" `vibe`, `opencode` "OpenCode" `opencode`, and `qwen-code` "Qwen Code" `qwen`;
  - every identifier matches `^[a-z0-9]+(-[a-z0-9]+)*$` and is distinct;
  - after the first two, identifiers are strictly ascending;
  - `Executable()` returns the executable of each entry.

  Update the existing order and length expectations in the same file.
- [X] T003 Implement in `internal/agent/agent.go` and `internal/agent/registry.go`: constants `FactoryDroid`, `GeminiCLI`, `KiroCLI`, `MistralVibe`, `OpenCode`, `QwenCode`; the six catalog entries in the order of T002; `func (a Agent) Executable() string`. Keep `Catalog`, `Known`, `Resolve`, `NewRegistry` and `List` unchanged in signature and behavior.
- [X] T004 [P] Tests in `internal/execution/adapters/invocation_test.go`, written before T005 (data-model.md Invocation, F013-FR-006):
  - `AfterEndOfOptions` with `Mode` `["exec"]` gives `exec`, `--`, then the instruction;
  - `AttachedOption` with `Option` `--prompt` gives `--prompt=` followed by the instruction, as one argument, with no `--`;
  - `Classify(0)` is `agent_succeeded`, and `Classify(1)`, `Classify(2)` and `Classify(130)` are `agent_failed`;
  - the instruction is never modified: spaces, newlines, `=`, quotes and Unicode are passed through.
- [X] T005 Implement `internal/execution/adapters/invocation.go`: `type Form uint8` with `AfterEndOfOptions` and `AttachedOption`; `type Invocation struct{ Executable string; Mode []string; Form Form; Option string }` implementing `execution.Adapter`. `Command` returns a fresh slice on each call, so callers cannot alias `Mode`. Comment the two forms with 002 FR-010 and F013-FR-006.

**Checkpoint:** catalog identity and the generic adapter are proven; no agent is wired yet.

## Phase 3: User Story 1, see every supported agent and which are installed (P1) 🎯 MVP

**Goal**: `ai-dev agents` lists the eight agents in order with their availability. **Independent test**: fake executables for a subset of agents; text and JSON listings.

- [X] T006 [P] [US1] Tests in `internal/app/agents_test.go`, written before T007 (US1 scenarios 1–3, 002 FR-002, FR-003): with a fake executor that finds only `gemini` and `vibe`, the text and JSON listings report the eight agents in catalog order; only `gemini-cli` and `mistral-vibe` are `available`; two runs give identical output; no launcher call is made. Update the existing length-2 expectations here and in `cmd/ai-dev/main_test.go` to the full catalog.
- [X] T007 [US1] Make T006 pass. The listing already iterates `agent.NewRegistry(...).List()`, so no change should be needed beyond T003. If any code path enumerates agents another way, change it to `agent.Catalog()` (F013-FR-012).

**Checkpoint:** detection of the new agents works end to end.

## Phase 4: User Story 2, run any catalog agent with an instruction (P1)

**Goal**: `ai-dev agent run <id> <instruction>` starts exactly the published invocation for every agent. **Independent test**: fake executables that print their arguments.

- [X] T008 [P] [US2] Catalog guard tests in `internal/execution/adapters/catalog_test.go`, written before T010 (research R-07, SC-003, SC-004, F013-FR-007):
  - **drift:** the keys of `Invocations` equal the identifiers of `agent.Catalog()`, and each `Invocation.Executable` equals that agent's `Executable()`;
  - **exact vectors from contracts/catalog.md:** `claude --print -- I`, `codex exec -- I`, `droid exec -- I`, `gemini --prompt=I`, `kiro-cli chat --no-interactive -- I`, `vibe --prompt=I`, `opencode run -- I` and `qwen --prompt=I`;
  - **instruction safety:** for every agent and for each of `-x a b`, `--help`, `-h`, `--version`, `-p`, `--yolo`, `--dangerously-skip-permissions`, `--`, `-` and `a=b`:
    - with `AfterEndOfOptions`, the last argument is the instruction and the only `--` comes right before it;
    - with `AttachedOption`, the last argument is the option, `=` and the instruction, and no argument is `--`;
  - **option shape:** every `AttachedOption` row has an `Option` that starts with `--`, and every `AfterEndOfOptions` row has an empty `Option` (data-model.md);
  - **denylist:** no argument other than the instruction contains, case-insensitively, `yolo`, `dangerous`, `skip-permission`, `bypass`, `trust`, `approve`, `allow-all`, `auto`, `force`, `full-auto`, `accept-license`, `sandbox` or `permission`;
  - **self-checks:** a fixture invocation with `--yolo` in `Mode` makes the denylist assertion fail, and one with a missing catalog row makes the drift assertion fail.

  Move the cases of `injection_test.go`, `claude_test.go` and `codex_test.go` here, then delete those three files.
- [X] T009 [P] [US2] End-to-end test in `cmd/ai-dev/agentcatalog_test.go`, written before T010 and T011 (SC-002, SC-003, US2 scenarios 1–4):
  - build the binary;
  - put one fake script per catalog executable in a temporary directory. Each fake prints `ARG:<arg>` per argument and exits with `$FAKE_EXIT` (default 0);
  - run with `PATH=<fakes>:/usr/bin:/bin`:
    - for every agent, `ai-dev agent run <id> "-x a b"` prints exactly the contract's vector, reports `agent_succeeded` and exits 0;
    - `ai-dev agent run opencode --help` passes `--help` as the instruction;
    - `FAKE_EXIT=3` gives `agent_failed` and exit 1;
    - removing the `qwen` fake gives `execution_failed` (unavailable) for `qwen-code`;
    - `--format json` keeps the report alone on standard output.
- [X] T010 [US2] Implement `internal/execution/adapters/catalog.go`: `var Invocations = map[agent.ID]Invocation{…}` with the eight rows of contracts/catalog.md. Each row has a comment naming its evidence in research R-05. Delete `internal/execution/adapters/claude.go` and `codex.go`, and update the package comment.
- [X] T011 [US2] In `internal/app/agentrun.go`, build `execution.Runner.Adapters` from `adapters.Invocations`, copied into a fresh map so the package table cannot be mutated through the runner.
- [X] T012 [US2] Update `internal/app/agentrun_test.go` and `internal/execution/runner_test.go` wherever they construct `adapters.Claude{}` or `adapters.Codex{}`: use `adapters.Invocations[agent.ClaudeCode]` and `adapters.Invocations[agent.CodexCLI]`, without changing what each test asserts.

**Checkpoint:** every catalog agent runs exactly as 002 specifies; the two existing vectors are unchanged.

## Phase 5: User Story 3, configure any catalog agent for a role (P2)

**Goal**: 011, 003 and 007 accept the new agents through the project configuration; built-in defaults are unchanged. **Independent test**: a configuration choosing new agents.

- [X] T013 [P] [US3] Tests, written before any change (F013-FR-010, FR-011, US3):
  - in `internal/app/setup_test.go` or `cmd/ai-dev/setup_test.go`: `setup apply --agent gemini-cli --agent factory-droid --role specification=gemini-cli --role planning=gemini-cli --role implementation=factory-droid --role review=gemini-cli --role remediation=factory-droid` writes the configuration; `agent select implementation` then reports `agent=factory-droid source=project`;
  - without a configuration, every role still selects `claude-code` or `codex-cli` with `source=built_in`;
  - a configuration written before this feature, choosing only `claude-code` and `codex-cli`, stays valid and byte-identical after `setup show`;
  - `setup show` lists the eight agents.
- [X] T014 [US3] Make T013 pass. No code change is expected beyond T003, because 011 derives from `agent.Catalog()`. Any failure points to an enumeration that must move to the catalog (F013-FR-012).
- [X] T015 [P] [US3] Help and completion tests in `internal/app/help_test.go` (help) and `internal/app/completion_test.go` (completion scripts), and a cockpit check in `internal/app/cockpit_test.go` (F013-FR-012):
  - `ai-dev agent run --help` and `ai-dev help agent select` list the eight identifiers in catalog order;
  - the zsh and bash completion scripts complete the eight identifiers after `agent run` and in `setup apply --role ROLE=`;
  - the cockpit's Agents area shows the eight agents.

  Fix any enumeration that does not come from `agent.Catalog()`.

**Checkpoint:** the new agents can be assigned to roles and appear wherever agents are listed.

## Phase 6: User Story 4, doctor reports agent readiness for any catalog agent (P2)

**Goal**: `coding-agent-tool` replaces the two agent tool checks (Amendment A2). **Independent test**: doctor with zero, one and several agents.

- [X] T016 [P] [US4] Tests in `internal/checks/codingagent_test.go`, written before T017 (F013-FR-009, research R-08, contracts/catalog.md):
  - no catalog executable found → `Fail`, explanation `No supported coding agent was found on PATH.`, remediation `Install a supported coding agent; run ai-dev agents to see the list.`;
  - only `vibe` found → `Pass`, name `Coding agent: mistral-vibe`, empty explanation and remediation (implementation finding, 2026-11-23);
  - `claude`, `qwen` and `droid` found → name `Coding agent: claude-code, factory-droid, qwen-code`, in catalog order;
  - a panicking executor → `Fail`, as `ToolCheck` does;
  - the check's identifier is `coding-agent-tool`, its name `Coding agent` when failing, its area `Environment`.
- [X] T017 [US4] Implement `internal/checks/codingagent.go`: `NewCodingAgentCheck(executor system.Executor)` over `agent.NewRegistry(executor).List()`.
- [X] T018 [P] [US4] Update `internal/app/doctor_test.go`, written before T019 (A2, SC-006, SC-007):
  - the ordered identifiers become `git-tool`, `speckit-tool`, `coding-agent-tool`, `git-repository`, `speckit-initialized`, `constitution`, `claude-integration`, `codex-integration`;
  - the single-failure table replaces the two tool rows with one `coding-agent-tool` row that removes every agent executable;
  - with only a `vibe` executable, doctor is Ready if every other check passes, and the text report shows `[PASS] Coding agent: mistral-vibe`;
  - the "all failures" test reports eight checks; rename it from "Nine" to "Eight".

  Update `internal/checks/tool_test.go` only if it names the removed identifiers.
- [X] T019 [US4] Update `internal/app/doctor.go`: eight checks in the order of T018, with `coding-agent-tool` third. Change the comment from "nine" to "eight".

**Checkpoint:** doctor passes with any one catalog agent.

## Phase 7: User Story 5, the admission record explains every candidate (P3)

**Goal**: the record is complete, auditable and reachable from the public documentation. **Independent test**: review against SC-001 and F013-FR-004.

- [X] T020 [US5] Check `research.md` R-05 against SC-001 and F013-FR-004:
  - every candidate of the research input appears exactly once;
  - each admitted row has a source for each of the seven criteria, a version and the date;
  - each exclusion names a criterion or the missing evidence;
  - the owner decisions of 2026-11-22 are recorded.

  Fix any gap in `research.md` only. The record is documentation, and no code depends on it.

## Phase 8: Polish and cross-cutting

- [X] T021 [P] Documentation in `README.md` and `README.es.md`, kept in step (CONTRIBUTING):
  - the supported-agents list gives the eight agents with identifiers and executables;
  - the doctor table has eight checks, with `coding-agent-tool` described;
  - a short "Adding an agent" paragraph links to `specs/013-agent-catalog/research.md` (admission criteria and record) and states that GitHub Copilot CLI, Amp, Goose, Forge, Grok Build, IBM Bob and DeepSeek Harness are excluded because their executable names collide with other widely used programs (criterion 6, research R-04).
- [X] T022 [P] `CHANGELOG.md` and `CHANGELOG.es.md`, under `[Unreleased]`:
  - **Added:** Factory Droid, Gemini CLI, Kiro CLI, Mistral Vibe, OpenCode and Qwen Code;
  - **Changed:** `ai-dev doctor` replaces `claude-code-tool` and `codex-cli-tool` with `coding-agent-tool` (eight checks), and machine readers of the doctor report must update; agent lists show the full catalog.
- [X] T023 Quality gates:
  - `gofmt -l .`, `go vet ./...`, `GOOS=linux go vet ./...`, `go test ./...` and `go test -race ./internal/execution/... ./internal/app ./cmd/ai-dev`;
  - `staticcheck ./...` and `govulncheck ./...` at the CI-pinned versions;
  - cross-builds for darwin and linux;
  - run quickstart.md by hand with fake agents only.

  Record the results here. Confirm `git grep -n -E 'adapters\.(Claude|Codex)\b'` finds nothing.
- [X] T024 Independent review before the PR (constitution VIII). It covers:
  - F013-FR-006 and FR-007 against each row of `catalog.go`;
  - the denylist's completeness;
  - that no code path runs an agent outside `agent run`;
  - SC-005;
  - F013-FR-013: no new input (configuration, environment variable or file) can name an executable, a path or an argument; the invocation table is fixed at build time;
  - SC-008: adding one more admissible agent would change only `internal/agent/registry.go`, `internal/execution/adapters/catalog.go`, its admission entry and the table-driven test expectations.

  Record the findings and their resolution here.

## Dependencies

- T001 → T002–T005 (foundational) → stories.
- US1 (T006–T007) needs T003.
- US2 (T008–T012) needs T003 and T005.
- US3 (T013–T015) needs T003. It is independent of US2 except for help text that mentions `agent run`.
- US4 (T016–T019) needs T003.
- US5 (T020) needs nothing in code.
- T021–T024 come last.

## Parallel examples

- After T003 and T005: T006 (US1), T008 (US2), T013 and T015 (US3), and T016 (US4) are test tasks on different files.
- T021 and T022 are independent documentation files.

## Implementation strategy

1. **MVP:** Phases 1–3 (the catalog is visible), then Phase 4. Running new agents is the owner's goal, so US1 and US2 ship together.
2. Then US4 (doctor), because it changes a machine-readable report and must land with the CHANGELOG note.
3. US3 and US5 mostly confirm existing behavior and close the record.

## Implementation record (2026-11-23)

- **T001 baseline:** `gofmt -l .` empty, `go vet ./...` and `go test ./...` passed on `feature/013-agent-catalog` before any change. Tests asserting two agents or nine checks:
  - `internal/app/agents_test.go:78` and `cmd/ai-dev/main_test.go:92`: two agents;
  - `internal/app/doctor_test.go:116,161,162,195` and `cmd/ai-dev/main_test.go:80`: nine checks.
  
  Also found during the run, all expectations of an agent list that follows the catalog (SC-007):
  - two-agent lookups in `setup_test.go`, `agentselect_test.go` and `workflowguide_test.go`;
  - completion lists in `completion_test.go`;
  - the listing writer in `internal/report/agentlist_test.go`;
  - `setup_wizard_test.go`'s "unknown number" (`3` became a valid agent, now `9`).
  
  `internal/checks/tool_test.go` and `internal/report/text_test.go` use the old identifiers only as generic fixtures and are unchanged.
- **T007, T014:** no production change was needed. Listing, setup, selection, help, completion and the cockpit already derive from `agent.Catalog()`.
- **T012:** no test constructed the removed adapters directly. Only `internal/app/agentrun.go` did; it now uses `catalogAdapters()`.
- **T016–T019, implementation finding:** 001's report contract keeps a passing check's explanation empty, and the text report prints it only on failure. The owner chose to name the agents in the check's name (spec Clarifications, 2026-11-23): `[PASS] Coding agent: mistral-vibe`.
- **T020:** R-05 lists every candidate of the research input exactly once. Each admitted row has evidence for the seven criteria, with version and date. Each exclusion names its criterion or the missing evidence. The owner decisions of 2026-11-22 are recorded.
- **T023 quality gates (2026-11-23, macOS):**
  - `gofmt -l .` empty;
  - `go vet ./...` and `GOOS=linux go vet ./...` passed;
  - `go test -count=1 ./...` passed;
  - `go test -race -count=1` passed on `internal/execution/...`, `internal/agent`, `internal/checks`, `internal/app` and `cmd/ai-dev`;
  - `staticcheck@v0.8.2 ./...` passed, also with `GOOS=darwin`;
  - `govulncheck@v1.10.0 ./...`: "No vulnerabilities found.";
  - cross-builds passed for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64, windows/amd64 and freebsd/amd64;
  - `git grep -n -E 'adapters\.(Claude|Codex)\b'` finds nothing.
  
  The quickstart was run by hand with fake agents only; every row behaved as written.
- **Unverified here:** the Linux test run itself (CI will run it). The help line for `setup apply --role` now lists 40 `ROLE=AGENT` pairs. That is correct but long; it is noted for the review (T024).

## Independent review (T024, 2026-11-23)

The reviewer was a separate agent. It had no write access to the repository and ran no real agent; the only binary it ran was its own build, with an empty `PATH`. Its checks (`gofmt`, `go vet`, and the tests of the affected packages) passed. It found no CRITICAL or HIGH findings. The owner authorized fixing all findings ("si, aplica todas las correcciones").

| # | Severity | Finding | Resolution |
|---|---|---|---|
| M1 | MEDIUM | The guard tests used only a substring denylist. Short flags (`-y`, `-s danger-full-access`), `--allowed-tools`, `--add-dir` and `--yes` would pass, and there was no allowlist of mode selectors despite R-07 | Fixed: `catalog_test.go` now requires every mode token to be in `allowedModeTokens` and every prompt option to be in `allowedOptions`. It rejects short options and values inside `Mode`. The denylist is extended with `danger`, `allow`, `unsafe`, `yes`, `add-dir`, `include-dir`, `settings`, `output` and `json`, and there are self-check fixtures for each case |
| M2 | MEDIUM | 001's `contracts/cli-contract.md` and `data-model.md` still listed nine permanent identifiers, and 013 said 001's report contract was unchanged | Fixed: an A2 note in both 001 artifacts retires `claude-code-tool` and `codex-cli-tool` and adds `coding-agent-tool`. The 013 spec now says "report shape" |
| M3 | MEDIUM | The end-to-end tests kept `/usr/bin:/bin` on `PATH`, so a real agent in a system directory could run when its fake was removed (SC-005) | Fixed: `PATH` holds only the fakes (`#!/bin/sh`, builtins only), and removed fakes are matched exactly |
| L1 | LOW | `Invocations` was an exported mutable map, and the runner's copy was shallow | Fixed: the table is unexported. `Invocations()` returns a deep copy, with a test that a caller cannot change the table |
| L2 | LOW | The README architecture text still said "claude/codex adapters" (EN and ES), and the README omitted that Grok Build and IBM Bob also fail criterion 3 | Fixed in both languages |
| L3 | LOW | The 013 contract said doctor names agents in its "explanation", and 001's amendment record lacked the 2026-11-23 decision | Fixed |
| L4 | LOW | Adding an agent touches more than the files listed (the identifier constant, documentation, non-table tests) | Fixed: the contract's "Adding an agent later" list names every file. `main_test.go` and `completion_test.go` derive their expectations from the shared catalog lists |
| L5 | LOW | The `setup apply --role` help line lists 40 `ROLE=AGENT` pairs | Accepted: it complies with 010 (allowed values listed when fixed). A compact rendering would need a 010 clarification and is left for a later change |
| L6 | LOW | `TestCanonicalIDsRemainSeparateFromDisplayLabels` could panic on a length mismatch, and the instruction-safety test had no negative fixture | Fixed: a length guard was added, and `instructionProblem` has fixtures for a second `--` and for an unknown form |
| L7 | LOW | The doctor check's name varies with the environment | Accepted: 001 FR-008 keys automation off `id`, which is stable. The CHANGELOG documents it |

Gates after the fixes are recorded below.

- **Gates after the fixes (2026-11-23, macOS):**
  - `gofmt -l .` empty;
  - `go vet ./...` and `GOOS=linux go vet ./...` passed;
  - `go test -count=1 ./...` passed;
  - `go test -race -count=1` passed on `internal/execution/...`, `internal/agent`, `internal/checks` and `cmd/ai-dev`;
  - `staticcheck@v0.8.2` passed, also with `GOOS=darwin`;
  - `govulncheck@v1.10.0`: "No vulnerabilities found.".
