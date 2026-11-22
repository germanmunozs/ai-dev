# Tasks: Terminal Cockpit

**Input**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/cockpit.md` and `quickstart.md` in this directory.

**Tests**: mandatory (constitution IV), written before the code they cover. Each test task names the requirement or success criterion it proves.

## Phase 1: Setup

- [ ] T001 Add the dependencies (research R-01): `go get charm.land/bubbletea/v2@<v> charm.land/lipgloss/v2@<v>` with the newest releases that are at least 14 days old on that day (on 2026-11-21: v2.1.1 and v2.0.6; check again with `go list -m -json <module>@<version>` and its `Time`). Then `go mod tidy` and `go mod verify`. Record in this file: each direct and transitive module, version, release date, and license read from its LICENSE file in the module cache. Every license must be MIT, BSD or Apache-2.0; stop and ask the owner otherwise.
- [ ] T002 Add a `govulncheck` step to the `static-analysis` job in `.github/workflows/ci.yml`: `GOBIN="$RUNNER_TEMP/bin" go install golang.org/x/vuln/cmd/govulncheck@v1.9.0` (14-day rule; recheck the date), then `"$RUNNER_TEMP/bin/govulncheck" ./...`, keeping `GOTOOLCHAIN: local` and `persist-credentials: false`. Run it locally too and record the result.

## Phase 2: Foundational (blocks every story)

- [ ] T003 [P] Tests in `internal/cockpit/sanitize_test.go`, written before T004 (F012-FR-016, SC-005, R-05): each class is rendered visibly and contains no raw byte of the sequence: CSI (`\x1b[2J`), OSC 52 clipboard (`\x1b]52;c;aGk=\x07`), OSC 8 hyperlink, OSC title, DCS, C1 controls (U+0085, U+009B), DEL, bidirectional controls (U+061C, U+200E, U+200F, U+202A–U+202E, U+2066–U+2069), a lone ESC, invalid UTF-8 (→ U+FFFD); a tab expands to spaces up to the next multiple of 8 columns (`"ab\tc"` → `"ab      c"`); ordinary text, accents and wide characters pass unchanged.
- [ ] T004 Implement `internal/cockpit/sanitize.go`: `type Text struct{ s string }`, `Sanitize(string) Text`, `(Text) String()`. Escape forms: `\x1b` for bytes below 0x20 (except tab, expanded to the next 8-column stop) and DEL, `\u0085` for C1 and bidirectional runes. No other constructor of a non-empty `Text`.
- [ ] T005 [P] Boundary test `internal/cockpit/boundary_test.go` (plan, R-03): no non-test file in `internal/cockpit` imports `os/exec`, `net`, `net/http`, `syscall`, or any `internal/` package (the cockpit consumes only the plain data of T006); none calls a write-capable function (`WriteFile`, `Create`, `OpenFile` with a write flag, `Mkdir`, `MkdirAll`, `Rename`, `Remove`); self-check fixtures prove each rule fires.
- [ ] T006 Define the ports and data in `internal/cockpit/data.go` (data-model.md): `Snapshot` (evaluatedAt; features []FeatureRow; environment, agents, repository as `Area` with `Status` "loading" | "ready" | "failed", lines []Text, err Text), `FeatureRow` (name Text, raw name string for arguments only, state, next, role, agent, availability, source, detail []Text, terminal, recognized, stepRecommended, agentAvailable bool), `Source` (`LoadFeatures`, `LoadEnvironment`, `LoadAgents`, `LoadRepository`, each `func(ctx) ...` so areas load concurrently), and `Runner` (`Command(args []string) tea.ExecCommand`; `Preview(args []string) string`, the quoted command line for the review, supplied by `internal/app` because the cockpit may not import `internal/report`).
- [ ] T007 [P] Tests in `internal/cockpit/actions_test.go`, written before T008 (contracts/cockpit.md, F012-FR-009, FR-011, SC-003): for each action, the argument vector for ordinary values and for values starting with `-`, containing spaces, `=`, quotes, newlines and `$(…)`, is exactly the contract's, one argument per value; `--reason=` is omitted for approve with an empty reason; availability and reason for each rule: run only for `step_recommended` with availability `available`; approve and abandon only for recognized, non-terminal features; setup, init and commit-validate always; required fields refuse empty values.
- [ ] T008 Implement `internal/cockpit/actions.go`: the six actions as data (id, title, scope, changes, fields, `available(FeatureRow) (bool, reason)`, `argv(FeatureRow, values) []string`).

**Checkpoint:** sanitizer, boundary, data and actions are proven; no screen yet.

## Phase 3: User Story 1, open the cockpit and find my way (P1) 🎯 MVP

**Goal**: `ai-dev tui` opens, navigates every area, shows help and palette, handles small terminals, and always restores the terminal. **Independent test**: pseudo-terminal run of the binary.

- [ ] T009 [P] [US1] Model tests in `internal/cockpit/model_test.go`, written before T010 (F012-FR-003, FR-004, FR-013, FR-014, FR-015, FR-019): `Tab`/`Shift-Tab` cycle Dashboard, Features, Environment, Agents, Repository, Help; `j`/`k`/arrows move and clamp; `/` filters and `Esc` clears; `:` and `Ctrl-P` open the palette listing every area and action with availability; `?` shows the current mode's keys and global keys; `q` quits from the top level, `Ctrl-C` quits outside `running`; a `WindowSizeMsg` below 80×24 renders only `Terminal too small: <c>×<r>, need 80×24.` and swallows confirmations; growing restores the previous mode; every status in the view is a word.
- [ ] T010 [US1] Implement `internal/cockpit/model.go`, `keys.go`, `view.go` (layout of contracts/cockpit.md: header with area and evaluation time, body, footer with keys) with Lip Gloss styles that never carry meaning alone.
- [ ] T011 [P] [US1] Command tests in `internal/app/cockpit_test.go`, written before T012 (F012-FR-001, contracts/cockpit.md): not interactive → the refusal line, exit 2, nothing loaded; any argument → `Usage: ai-dev tui`, exit 2; routing from `RunWithDependencies`.
- [ ] T012 [US1] Implement `internal/app/cockpit.go`: `RunCockpit`, the interactivity check (`isTerminal(stdin) && writerIsTerminal(stdout)`), the program with Bubble Tea's signal handler disabled and `SIGINT`/`SIGTERM` forwarded as messages (R-04); the program, the loaders and every launch run on `context.WithoutCancel(ctx)`, because `main` cancels `ctx` on every `SIGINT` (R-04); exit codes 0 for `q` and `Ctrl-C`, 130 for an external `SIGINT`, 143 for `SIGTERM` (contracts/cockpit.md); route `tui` in `internal/app/app.go`.
- [ ] T013 [US1] Pseudo-terminal test `internal/app/cockpit_pty_test.go` (build tags as `completion_pty_test.go`; reuse `openPTY`, `buildSetupBinary`) (F012-FR-002, SC-004, SC-005): run the binary as session leader; read the slave's terminal attributes before; open, wait for the dashboard, then quit by `q`, by `Ctrl-C`, by an external `SIGINT` and by `SIGTERM` to the process; after each, the attributes equal the originals, the output contains the alternate-screen exit and cursor-show sequences, and the exit code is 0, 0, 130 and 143 respectively. With a fixture `specs/` directory named with an OSC 52 sequence (`\x1b]52;c;aGk=\x07`) and an OSC 8 hyperlink, the raw terminal output never contains either sequence and does contain their visible escapes (R-05).

**Checkpoint:** the empty cockpit opens, navigates and restores the terminal.

## Phase 4: User Story 2, see where every feature stands (P1)

**Goal**: Features area and details from the existing reports. **Independent test**: compare with the commands for fixture projects.

- [ ] T014 [P] [US2] Tests in `internal/app/cockpit_test.go`, written before T015 (F012-FR-005, SC-002, R-02): for fixtures covering every Workflow State, missing artifacts, an invalid record, a terminal feature, a symlinked feature directory (excluded) and an invalid agent configuration, `LoadFeatures` returns rows whose fields match `workflow.EvaluateState`, `sdd.Evaluate` and `guide.Evaluate`, and whose detail lines equal the text of `ai-dev workflow state` and `ai-dev workflow guide` for that feature, line by line after sanitizing; a feature named with an OSC 52 sequence is sanitized in name and detail; with 50 fixture features, `LoadFeatures` completes in under 2 seconds (SC-001, second half; record the time here); when the project directory has been removed or made unreadable, `LoadFeatures` returns a failure, not an empty or stale list (edge case).
- [ ] T015 [US2] Implement the features source in `internal/app/cockpit.go`: list `specs/*` with `workflow.FeatureExists`, in name order; evaluate each feature with the assignment from `loadAssignment(root)` (as `workflow guide` does); render detail with `report.WriteWorkflowStateText` and `report.WriteGuideText`.
- [ ] T016 [P] [US2] Model tests, written before T017 (F012-FR-006, FR-007, US2 scenarios): the Features table shows name, state, next activity, agent, availability and source; `Enter` opens the detail; an invalid or unknown feature shows its outcome and no action; `r` and the return from an action reload every area; the header shows the evaluation time; an area still loading says `evaluating…` and keys keep working; an empty `specs/` shows how features are created; an area whose reload fails shows its error and none of its previous lines, so stale data never appears as current (edge case).
- [ ] T017 [US2] Implement the Features area, detail and refresh in `internal/cockpit/model.go` and `view.go`.
- [ ] T018 [P] [US2] Benchmark `internal/cockpit/bench_test.go` (SC-001, R-07): with 50 synthetic features, `Update` plus `View` per navigation key under 50 ms; record the result here.

## Phase 5: User Story 3, start an agent for the next step (P1)

**Goal**: run the next step's agent through the command. **Independent test**: fake agents on `PATH`.

- [ ] T019 [P] [US3] Model tests, written before T020 (F012-FR-008, FR-009, FR-010, FR-012, SC-006, R-06): "run agent" opens a form with an empty instruction; the review shows the fake runner's `Preview` of the argument vector, sanitized, and the fixed line of the contract; only `Tab` to the confirm button then `Enter` calls the fake runner, exactly once; `Esc` at form or review runs nothing; in `running`, every key is ignored and a second confirmation cannot start another command; the exec callback moves to `returned`, closes the review, and reloads; an unavailable action shows its reason and opens nothing. SC-006: for each area and mode, every key and every pair of keys of the keymap is sent; the fake runner records no command unless the sequence passed through the review's confirm.
- [ ] T020 [US3] Implement forms (`internal/cockpit/input.go`: single-line input with runes, Backspace, `Ctrl-U`, `Ctrl-A`/`Ctrl-E`, paste), the review and the running/returned modes in `internal/cockpit/model.go`.
- [ ] T021 [P] [US3] Tests in `internal/app/cockpit_test.go`, written before T022 (R-03, F012-FR-010): the runner launches `os.Executable()` with exactly the argument vector through a fake `system.Launcher`, with the terminal's stdin, stdout and stderr; after the process exits it prints `ai-dev tui: the command exited with code N. Press Enter to return.` and reads one line; a launch failure is reported and nothing else runs; the launch context is not cancelled when the context given to `RunCockpit` is cancelled (`context.WithoutCancel`, R-04); `Preview` quotes every argument with `report.ShellQuote` (values with `'`, spaces, newlines and a leading `-`).
- [ ] T022 [US3] Export `shellQuote` as `ShellQuote` in `internal/report/gitplan.go`, unchanged, and update its callers in `internal/report/gitplan.go` and `internal/report/guide.go` (existing report tests must pass unchanged); implement the runner (`tea.ExecCommand` over `system.Launcher`, launched on the detached context of T012) and `Preview` in `internal/app/cockpit.go`.
- [ ] T023 [US3] Extend `internal/app/cockpit_pty_test.go` (SC-003, SC-007): with a fake `claude` on `PATH` that prints its arguments, open a fixture project, select a feature whose next step maps to Claude Code, run the agent with the instruction `-x "a b"`, confirm; the fake prints exactly `--print -- -x "a b"`; the exit-code line appears; `Enter` returns to the cockpit on the same feature; the terminal is restored on quit. `SIGINT` sent to the foreground process group while the fake sleeps (R-04): the cockpit ignores it, the command handles it as it does outside the cockpit (its exit code is shown), and after `Enter` the cockpit resumes with every area reloaded successfully, not failed by a cancelled context.

## Phase 6: User Story 4, record a human decision (P1)

**Goal**: approve and abandon through the commands. **Independent test**: compare `workflow-state.json` with typed commands.

- [ ] T024 [P] [US4] Model tests, written before T025 (F012-FR-009, FR-011, US4 scenarios): approve asks approver, target state and reason with no value prefilled, and a hint listing the canonical states; abandon requires the reason; the review shows the exact vector; terminal features offer neither.
- [ ] T025 [US4] Implement the approve and abandon forms in `internal/cockpit/model.go` (actions from T008).
- [ ] T026 [US4] Extend `internal/app/cockpit_pty_test.go` (SC-003): approve a fixture feature through the cockpit and compare `workflow-state.json`, output and exit code with the same `ai-dev workflow approve` typed in a twin fixture; abandon likewise; an approve that the command refuses (an invalid target) leaves the record unchanged and the cockpit shows the command's output only.

## Phase 7: User Story 5, check the environment, agents and repository (P2)

- [ ] T027 [P] [US5] Tests in `internal/app/cockpit_test.go`, written before T028 (SC-002): `LoadEnvironment`, `LoadAgents` and `LoadRepository` return the lines of `report.WriteText(RunDoctor(...))`, the `setup show` text report and `report.WriteGitInspectionText(inspectProject(...))` for the same fixtures, sanitized; a failing inspection yields a failed area with its error; with a sentinel environment variable set (`AI_DEV_TEST_SECRET=<random>`), no loaded line of any area contains its value (F012-FR-018).
- [ ] T028 [US5] Implement those sources in `internal/app/cockpit.go`, and the Environment, Agents and Repository areas with the project actions (setup, init, commit-validate) in `internal/cockpit`.

## Phase 8: Help, catalog and documentation

- [ ] T029 Tests first, then: catalog entry `tui` in `internal/app/catalog.go` (description and changes of contracts/cockpit.md, no options); top-level usage gains `tui` in `internal/app/app.go`; the general help lists it; update every test pinning them, the completion candidate lists, and `TestCatalogCoversRoutedPaths` (A1, A2, F012-FR-020).
- [ ] T030 [P] Update `README.md` (feature table row 012, the cockpit section with keys and the actions it runs, the dependency policy), `CHANGELOG.md` (`Added`) and `SECURITY.md` (third-party dependencies: pinning, 14-day rule, govulncheck).

## Phase 9: Polish

- [ ] T031 Quality gates of plan.md: `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `staticcheck ./...` for linux and darwin, `govulncheck ./...`, `go mod verify`, builds for darwin, linux and windows; run quickstart.md by hand; record the results here, including SC-007 measured by a person if possible, marked unverified otherwise.
- [ ] T032 Independent review by a different agent with a fresh context (constitution VIII), focused on terminal injection, signals, double submission, argument vectors and the dependency set; it must not run a real coding agent; record implementer, reviewer, findings and resolution here and in the PR.

## Dependencies

- T001 → everything that imports the libraries (T006 onward). T002 is independent.
- T003 → T004; T005 independent; T006 after T004; T007 → T008 (needs T006).
- US1 (T009–T013) needs Phase 2. US2 (T014–T018) needs T010 and T012. US3 (T019–T023) needs T008, T010, T012. US4 (T024–T026) needs T020. US5 (T027–T028) needs T010, T012.
- Phase 8 after US1; Phase 9 last.

## Parallel examples

- Phase 2: T003, T005 and T007 are separate test files.
- After US1: US2's source tests (T014) and US3's runner tests (T021) touch different code and can run together; T027 likewise.

## Implementation strategy

1. **MVP:** Phases 1–3 plus US2 (T014–T018): a read-only cockpit that already replaces running `state`, `next` and `guide` for each feature.
2. US3 and US4: the actions through the commands.
3. US5, then help, docs, gates and review.
