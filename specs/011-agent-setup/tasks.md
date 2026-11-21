# Tasks: Agent Detection and Role Configuration

**Input**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/` and `quickstart.md` in this directory.

**Tests**: mandatory (constitution IV), written before the code they cover. Each test task names the requirement it proves.

## Phase 1: Setup

- [ ] T001 Add read-only catalog views to `internal/agent/registry.go` (R-06): `Catalog() []Agent` returns a copy of the fixed set in catalog order, and `Known(id ID) bool` reports exact, case-sensitive membership. Neither takes an executor or probes availability. Tests first in `internal/agent/registry_test.go`: order, copy semantics (mutating the result does not change the catalog), `Known` for both identifiers, `Claude-Code`, `""` and `gemini`.

## Phase 2: Foundational (blocks every story)

- [ ] T002 [P] Tests in `internal/projectconfig/load_test.go`, written before T003, using `fstest.MapFS` and real temporary directories through `os.DirFS` (contracts/config-file.md):
  - `Absent` when `.ai-dev` or `.ai-dev/config.json` does not exist;
  - `Valid` for the canonical file and for the same content with other whitespace;
  - one fixture per reason code, asserting the exact code and quoted value: `directory_not_regular` (symlinked `.ai-dev`, `.ai-dev` as a file), `file_not_regular` (symlink to a valid file, a directory, a FIFO), `cannot_verify_symlinks` (an `fs.FS` without `ReadLinkFS`), `too_large` (65537 bytes), `malformed_json` (empty, `[]`, trailing `{}`), `duplicate_key` (top level and inside `roles`), `unknown_field`, `missing_field`, `wrong_type` (including `null` and a number in `agents`), `unsupported_schema`, `no_agents`, `duplicate_agent`, `unknown_agent` (`gemini`, `Claude-Code`), `unknown_role`, `missing_role`, `agent_not_chosen`;
  - several content violations in one file are all reported, in the contract's order;
  - an untrusted value containing a newline, `$`, a backquote and `!` is rendered in 004's shell-inert quoted form.
- [ ] T003 Implement `internal/projectconfig` (R-02, data-model.md):
  - `config.go`: `FileName = ".ai-dev/config.json"`, `SchemaVersion = "ai-dev.config/v1"`, `MaxSize = 65536`; `Config` with unexported fields; `Reason{Code, Value string}`; `State` (`Absent`, `Valid`, `Invalid`); `New(agents []agent.ID, roles map[role.Role]agent.ID) (Config, []Reason)`; accessors `Agents()`, `AgentFor(role.Role)`, `Chosen(agent.ID)`;
  - `load.go`: `Load(root fs.FS) (Config, State, []Reason)` with `Lstat` through `fs.ReadLinkFS`, the 65537-byte read limit, a token walk for duplicate keys and trailing data, then strict field and closed-set validation. Rules, verbatim from data-model.md: "`schema_version` exactly `ai-dev.config/v1`"; "`agents` one or more; distinct; each a catalog identifier"; "`roles` exactly the keys `specification`, `planning`, `implementation`, `review`, `remediation`; each value one of `agents`"; "no other field, no `null`, no duplicate key, no trailing data; at most 64 KiB".
  - Reuse the shell-inert quoting of `internal/report/quote.go` by moving the pure function it needs, or a copy with its tests, so `projectconfig` does not import `report`.
- [ ] T004 [P] Tests in `internal/projectconfig/encode_test.go`, written before T005: the canonical bytes of contracts/config-file.md exactly; agents in catalog order whatever order `New` received; roles in contract order; trailing newline; `Load(Encode(c))` round-trips to an equal `Config`; `New` rejects every content violation of T002 with the same codes, while `duplicate_role` is detected by `setup apply` while parsing options (T024), because a map cannot hold a role twice.
- [ ] T005 Implement `(Config) Encode() []byte` in `internal/projectconfig/encode.go` (R-04), encoding roles through a struct with one field per role so the order is fixed.
- [ ] T006 [P] Tests in `internal/configstore/store_posix_test.go` and `internal/configstore/unsupported_test.go`, written before T007, mirroring `internal/gatestore`:
  - creates `.ai-dev/` (0o755) and `config.json` (0o644) when absent; replaces an existing file keeping its permission bits; leaves no temporary file;
  - refuses a symlinked `.ai-dev`, a symlinked `config.json`, and a `config.json` that is a directory, writing nothing through them;
  - a swap of `.ai-dev` between the check and the open is detected (`os.SameFile`);
  - fault injection through `ops` at open, write, sync, close and rename: the previous file is byte-identical and the temporary file is removed; a directory-sync failure after rename reports success with a warning;
  - on an unsupported platform build, `Write` always fails with the macOS/Linux warning.
- [ ] T007 Implement `internal/configstore` (R-03): `store.go` (`New(dir string) Store`, `Store.Write(data []byte) (Status, []string)`, `Status` = `Written | Failed`), `write_posix.go` with build tags `(darwin && !ios) || (linux && !android)` and injectable `ops`, `unsupported.go` and `new_other.go`. Temporary name `.ai-dev/.config.<16 hex>.tmp`, `O_CREATE|O_EXCL`.
- [ ] T008 [P] Boundary test `internal/configstore/boundary_test.go` (R-03): walking every non-test `.go` file, no file outside `internal/configstore/` combines a write-capable call (`WriteFile`, `Rename`, `Create`, `OpenFile` with a write flag, `Mkdir`, `MkdirAll`) with a reference to `config.json`, `.ai-dev` or `projectconfig.FileName`; only `cmd/ai-dev/main.go` imports `internal/configstore`; self-check fixtures prove the checker flags a violation and accepts a read-only open.
- [ ] T009 [P] Tests in `internal/selection/assignment_test.go`, written before T010 (R-01, A1):
  - `BuiltIn()` resolves every role exactly as 003 FR-003, with source `built_in`, and every existing selector test still passes through `NewSelector`;
  - `FromProject(cfg)` resolves to the configured agents with source `project`: `role_selected` when available, `agent_unavailable` when not;
  - `InvalidProject()` yields `configuration_invalid` for every recognized role, with no agent, source `project`;
  - an unrecognized role is `role_unsupported` under all three, carrying the assignment's source;
  - `selection_inconsistent` is impossible under `FromProject` (F011-FR-018);
  - `Report.Valid()`: a `built_in` report is still checked against `mappedAgent`; a `project` report is valid only with the outcome's field presence and a catalog agent; `configuration_invalid` is valid only with `RequestedRole`, no agent and source `project`.
- [ ] T010 Implement `internal/selection/assignment.go` (`Source`, `Assignment`, `BuiltIn`, `FromProject`, `InvalidProject`), `NewSelectorFor(registry, Assignment)`, outcome `ConfigurationInvalid` and `Report.Source` in `outcome.go`, and the `Valid()` changes in `validate.go`. `NewSelector(registry)` returns `NewSelectorFor(registry, BuiltIn())`. Update the package comments that say the mapping cannot be substituted, citing A1.
- [ ] T011 Shared write decision in `internal/app/setupwrite.go`, tests first in `internal/app/setupwrite_test.go`: `decideWrite(root fs.FS, store configstore-like interface, cfg projectconfig.Config, replace bool) (outcome, warnings)` returns `configuration_written`, `configuration_unchanged` (existing bytes equal `cfg.Encode()`), `configuration_exists` (different or invalid, `replace` false) or `persistence_failed`. The store is an interface in `internal/app` so tests use a fake and `internal/app` never imports `configstore`.
- [ ] T012 Wire the store: `Dependencies.ConfigStore` in `internal/app/app.go` (nil-safe: a nil store reports `persistence_failed`), and `configstore.New(".")` in `cmd/ai-dev/main.go`.

**Checkpoint:** the configuration can be read, validated, encoded and written; selection resolves through an `Assignment`. No command uses them yet.

## Phase 3: User Story 1, see detected agents and the assignment (P1) 🎯 MVP

**Goal**: `ai-dev setup show` in text and JSON. **Independent test**: run it with no configuration, a valid one and an invalid one, each agent available or not; nothing is written and no agent starts.

- [ ] T013 [P] [US1] Report tests in `internal/report/setup_test.go`, written before T014 (contracts/setup-cli.md): the exact text and compact JSON for `not_configured`, `configured` (with `chosen_agent_unavailable`), and `configuration_invalid` (reasons in order, no `source`, `roles: []`); `warnings` and `reasons` always arrays; schema `ai-dev.setup/v1`; no executable path in either form.
- [ ] T014 [US1] Implement `internal/report/setup.go`: a `SetupReport` value (command, outcome, source, agents with availability and chosen, roles, warnings, reasons), `WriteSetupText`, `WriteSetupJSON`, and a `Valid()` that rejects field combinations the contract forbids.
- [ ] T015 [P] [US1] Command tests in `internal/app/setup_test.go`, written before T016: `RunSetupShow` with a fake executor and `fstest.MapFS` for each outcome and exit code (0, 0, 1); `--format json` writes exactly one object and nothing else on stdout; usage failures (positional argument, unknown option, `--format yaml`) exit 2 with `Usage: ai-dev setup show [--format text|json]`; the fake executor records only `LookPath` calls (F011-FR-001, F011-FR-013).
- [ ] T016 [US1] Implement `RunSetupShow(args, executor, root, stdout, stderr)` in `internal/app/setup.go`, and the `setup` routing in `internal/app/app.go`: `setup show` and `setup apply` routes, the group usage `Usage: ai-dev setup [show|apply] [options]` for an unknown subcommand.

**Checkpoint:** US1 works alone.

## Phase 4: User Story 4, selection and guidance follow the assignment (P1)

**Goal**: `agent select` and `workflow guide` use the project assignment (A1, A2). **Independent test**: for each role and each source, compare outcomes, agents and `source`.

- [ ] T017 [P] [US4] Tests, written before T018 (contracts/selection-and-guide.md):
  - `internal/report/roleselection_test.go`: every text line gains ` source=…`; JSON gains `"source"`; `configuration_invalid` renders `role` and `source` only;
  - `internal/guide/guide_test.go` and `templates_test.go`: availability `configuration_invalid` with no agent and source `project`; its templates are transition and approve only; `Valid()` accepts it and rejects an agent with it;
  - `internal/report/guide_test.go`: ` source=…` after `availability=`; `agent=` omitted for `configuration_invalid`; JSON `"source"` and no `"agent"`;
  - `internal/app/agentselect_test.go` and `internal/app/workflowguide_test.go`: no configuration gives today's output plus `source=built_in`; a valid configuration assigning `implementation` to `claude-code` gives `agent=claude-code source=project`; an invalid one gives `configuration_invalid` (agent select exit 1; guide exit unchanged).
- [ ] T018 [US4] Implement: `availabilityOf` and `Valid` in `internal/guide/guide.go`, `Templates` in `internal/guide/templates.go`, `Report.Source`; the text and JSON changes in `internal/report/roleselection.go` and `internal/report/guide.go`; in `internal/app/agentselect.go` and `internal/app/workflowguide.go`, load `projectconfig.Load(root)` and choose `BuiltIn()`, `FromProject(cfg)` or `InvalidProject()`. `RunAgentSelect` gains the `root fs.FS` parameter; update its call in `app.go`.
- [ ] T019 [US4] Update every existing test that pins a 003 or 007 report line or JSON object to carry `source=built_in` (SC-005), and nothing else: `internal/report`, `internal/app`, `internal/guide`, `cmd/ai-dev`. List the files touched in the PR.

**Checkpoint:** with no configuration, the 003 and 007 suites pass with only the `source` expectation added.

## Phase 5: User Story 2, the assistant (P1)

**Goal**: `ai-dev setup` chooses agents and roles and writes only after `yes`. **Independent test**: drive it in a pseudo-terminal through each path.

- [ ] T020 [P] [US2] Unit tests in `internal/app/setup_wizard_test.go`, written before T021, with an `io.Pipe` input and `interactive=true` (contracts/setup-cli.md transcript):
  - full flow by numbers and by identifiers, `yes` → `configuration_written`, the fake store receives the canonical bytes;
  - an unavailable chosen agent prints the warning and is accepted;
  - with a valid existing configuration, each prompt shows `[current: …]` and empty answers keep it → `configuration_unchanged`, nothing written;
  - with an invalid existing configuration, its reasons are printed and an empty answer is invalid;
  - `no`, `y`, end of input, and a cancelled context → `setup_cancelled`, exit 1, nothing written;
  - an unknown number, an unknown identifier, no agent chosen, or a role answered with an unchosen agent → exit 2, nothing written;
  - `interactive=false` → `interactive_terminal_required`, exit 2, and the executor is never called (no detection);
  - `--format json` or any argument → usage failure.
- [ ] T021 [US2] Implement `RunSetupWizard(ctx, args, executor, root, store, stdin, interactive, stdout, stderr)` in `internal/app/setup_wizard.go` (R-05): line reads in a goroutine raced against `ctx.Done()`, the literal `yes`, `decideWrite` for the result; route bare `ai-dev setup` in `app.go` with `interactive = isTerminal(stdin) && isTerminal(stdout)` (extend `isTerminal` to accept the `*os.File` behind stdout).
- [ ] T022 [US2] Pseudo-terminal test `internal/app/setup_wizard_pty_test.go` (build tags as `completion_pty_test.go`), reusing its `openPTY`: run the built binary under a pty with fake agents on `PATH`, complete the flow with `yes`, check the file; repeat with `no` and with Ctrl-C (`\x03`), checking the directory is byte-identical (SC-003).

## Phase 6: User Story 3, configure without the assistant (P2)

**Goal**: `ai-dev setup apply`. **Independent test**: complete, incomplete and contradictory option sets, with and without an existing file.

- [ ] T023 [P] [US3] Tests in `internal/app/setup_test.go`, written before T024:
  - the quickstart command → `configuration_written`, then again → `configuration_unchanged`;
  - a different set without `--replace` → `configuration_exists`, exit 1, file unchanged; with `--replace` → written; an invalid existing file without `--replace` → `configuration_exists`;
  - `configuration_rejected` listing every reason for: a missing role, `duplicate_role`, `agent_not_chosen`, `unknown_agent`, `unknown_role`, `no_agents`, `duplicate_agent`;
  - an unavailable chosen agent → written, with `chosen_agent_unavailable:<id>`;
  - store failure → `persistence_failed`, exit 1;
  - usage failures: `--role implementation` (no `=`), a positional argument, `--format yaml` → exit 2 with the apply usage line;
  - JSON mode: one object with `"command":"apply"`.
- [ ] T024 [US3] Implement `RunSetupApply(args, executor, root, store, stdout, stderr)` in `internal/app/setup.go`: repeatable `--agent` and `--role` through `flag.Func`, `duplicate_role` detection, `projectconfig.New`, `decideWrite`, the setup report.
- [ ] T025 [P] [US3] Subprocess test `cmd/ai-dev/setup_test.go` against the built binary in a temporary directory with fake `claude` and `codex` on `PATH` that append to a log: `setup show`, `setup apply`, `agent select`, `workflow guide` and `setup show --format json` all run, and the log stays empty (SC-002); two `setup apply` runs produce byte-identical files (SC-006).

## Phase 7: Help, completion and documentation

- [ ] T026 Tests first, then catalog entries in `internal/app/catalog.go` and `catalog_test.go` (F011-FR-020, A3, A4):
  - `setup` (command and group, `changes`: "the project configuration file `.ai-dev/config.json`", after confirmation), `setup show` (`--format`, changes nothing), `setup apply` (`--agent` with the catalog identifiers, repeatable; `--role` with the ten `ROLE=AGENT` values generated from `role` and `agent.Catalog()`, repeatable; `--replace`; `--format`);
  - `TestCatalogCoversRoutedPaths` gains the three paths and the `setup` group; `TestGroupUsageNamesEverySubcommand` covers `setup`;
  - option and usage equality (F010-FR-009, SC-002) pass for the new commands.
- [ ] T027 Update `topLevelUsage` in `internal/app/app.go` to `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|version|help|completion> [options]`, the general help in `internal/app/help.go`, and every test pinning them; completion tests check `ai-dev setup apply --role ` offers the ten pairs in bash and zsh.
- [ ] T028 [P] Update `README.md` (feature table row 011, the setup commands, the configuration file and that it should be committed, the effect on `agent select` and `workflow guide`) and `CHANGELOG.md` under `[Unreleased]` → `Added`.

## Phase 8: Polish

- [ ] T029 Run the quality gates of plan.md: `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `go build ./cmd/ai-dev`; run quickstart.md by hand and record the results here.
- [ ] T030 Independent review by a different agent (constitution VIII), with the security focus of research R-02 and R-03; record implementing agent, reviewer, findings and resolution here and in the PR. The reviewer must not run a real coding agent.

## Dependencies

- T001 → T002/T003 → T004/T005; T006/T007/T008 independent of T002–T005; T009/T010 need T003; T011 needs T003, T005; T012 needs T007, T011.
- US1 (T013–T016) needs Phase 2. US4 (T017–T019) needs T010 and T003. US2 (T020–T022) needs T011, T012, T014. US3 (T023–T025) needs T011, T012, T014, T016.
- Phase 7 needs US1–US3 routes. Phase 8 last.

## Parallel examples

- Phase 2: T002, T004, T006, T008 and T009 are test files in different packages and can be written together.
- After Phase 2: US1 (T013–T016) and US4 (T017–T019) touch different files and can proceed in parallel.
- T025 and T028 can run alongside T026.

## Implementation strategy

1. **MVP:** Phases 1–3 (`setup show`), which is read-only and lets a user see detection and the assignment in effect.
2. Phase 4, so a configuration written by hand already takes effect safely, with fail-closed validation.
3. Phases 5 and 6, the two ways to write the configuration.
4. Phases 7 and 8, then the PR.
