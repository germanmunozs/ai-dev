# Tasks: Command Help and Shell Completion

**Input**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/` and `quickstart.md` in this directory.

**Tests**: mandatory (constitution IV), written before the code they cover.

## Phase 1: Setup

- [ ] T001 Add `internal/app/flags.go` (R-03):
  - `newFlagSet(name string) *flag.FlagSet` returns `flag.NewFlagSet(name, flag.ContinueOnError)` with output `io.Discard`;
  - `var flagSetObserver func(*flag.FlagSet)`, nil in production, is called with each new set when non-nil.

## Phase 2: Foundational (blocks both stories)

- [ ] T002 Replace each of the 16 `flag.NewFlagSet(...)` call sites in `internal/app` with `newFlagSet(...)`, keeping every name, flag, default and parse path. The files are:
  - `agents.go`, `agentrun.go`, `agentselect.go`, `app.go` (doctor), `version.go`;
  - `gitcommit.go`, `gitapply.go`, `gitinit.go` (init inspect), `gitinit_wizard.go`, `gitplan.go`, `gitreconcile.go`;
  - `workflowgate.go`, `workflownext.go`, `workflowguide.go`, `workflowstate.go`, `workflowtransition.go`.

  Remove the now-redundant `SetOutput(io.Discard)` calls. `go test ./internal/app ./cmd/ai-dev` must pass unchanged.
- [ ] T003 Extract every inline usage literal into a named constant with byte-identical text, so that the command and the catalog share it (R-02):
  - `doctor`, `agents`, `agent run`, `agent select`;
  - the `init` wizard;
  - `workflow state`, `transition`, `next` and `guide`.

  `workflowgate.go` gets `workflowGateUsage(decision gate.Decision) string`, used by both the command and the catalog. The group lines `Usage: ai-dev agent <run|select> [options]` and `Usage: ai-dev workflow <state|transition> [options]` become constants with unchanged text (F010-FR-010).
- [ ] T004 Tests in `internal/app/catalog_test.go`, written before T005:
  - **Coverage:** every routed command path is in the catalog, and nothing else is. The commands are the 18 paths of 009's `helpCommands` plus `completion`, and the group paths are `agent`, `workflow`, `init`, `git` and `git commit`.
  - **Option equality (F010-FR-009, SC-004):** for each non-group command, set `flagSetObserver`, run the command through `RunWithDependencies` with the single argument `--zz-undefined`, assert exit 2, and compare the recorded set's `VisitAll` names with the catalog's option names.
    - The test `Dependencies` carry a non-nil gate store, which is never used because parsing fails first. Without it, `workflow approve` and `workflow abandon` stop at "human gate store unavailable" before building their flag sets.
    - `help` and `completion` define no flag set; their catalog options are empty.
    - The rendered `-h, --help` line is not a catalog option (contracts/help.md).
  - **Usage equality (SC-002):** for each command, the catalog usage equals the first line of the command's stderr on that same `--zz-undefined` failure.
  - **Description:** each command description equals its general-help line.
  - **Safe words:** every embedded word (paths, option names, fixed values) matches `^[A-Za-z0-9._|-]+$`.
- [ ] T005 Implement `internal/app/catalog.go` from `data-model.md`. It holds `commandSpec`, `optionSpec`, `argSpec`, the ordered `catalog` and `lookup(args []string) (commandSpec, int, bool)`, which finds the longest path. `changes` is one of: "nothing", "workflow-state.json", "the repository and its project configuration", "the plan file only", or "whatever the agent does, under its own sandbox". It replaces `helpCommands`, and the 009 routing test in `help_test.go` iterates the catalog.

**Checkpoint:** the catalog is complete and proven equal to the commands' real options and usage lines.

## Phase 3: User Story 1, learn one command's options (P1) 🎯 MVP

**Goal**: `ai-dev <path> --help|-h` and `ai-dev help <path>` (contracts/help.md).

**Independent Test**: every catalog path answers help with exit 0, and the positions F010-FR-003 excludes are unchanged.

- [ ] T006 [US1] Tests in `internal/app/help_test.go`:
  - **Every path:** for every catalog path, `<path> --help`, `<path> -h`, `help <path>`, `--help <path>` and `-h <path>` give the same stdout, empty stderr and exit 0 (SC-001).
  - **Command help:** contains `ai-dev <path>: <description>`, the usage line, every option name, every fixed value, `Changes:` and `Exit codes:`.
  - **Group help:** lists exactly the child paths. `workflow` lists six, `git` lists `git commit`, and `init` lists its command help and its four subcommands.
  - **Exact goldens:** for `workflow state --help` and `workflow --help`, compare with contracts/help.md.
  - **Trailing arguments:** `workflow state --help extra` prints the same help, with exit 0.
- [ ] T007 [US1] Tests in `internal/app/help_test.go` for F010-FR-003 and F010-FR-004:
  - **Unchanged positions:** `agent run claude-code --help` launches the agent with instruction `--help`, through a recording launcher. `workflow approve --approver --help f planned` keeps `--help` as the approver. `workflow state -- --help` and `workflow state f --help` keep their current exit codes and stderr.
  - **No side effects:** help with `Dependencies` whose stdin, launcher, executor, root and gate store panic on any use, and whose Git dependencies are unavailable, still exits 0. This covers `init --help` with a non-terminal stdin and `agent run --help` with a non-empty pipe.
  - **Usage failures:** `help nope`, `help workflow state extra` and `help ""` exit 2, write `Usage: ai-dev help [command...]` alone on stderr, and leave stdout empty.
- [ ] T008 [US1] Implement help rendering in `internal/app/help.go`:
  - `writeCommandHelp` and `writeGroupHelp`, in the layouts of contracts/help.md;
  - `RunHelp(args)`, which looks the path up with `lookup` and requires all arguments to be consumed;
  - the general help, rendered from the catalog, with the new "Run 'ai-dev help <command>'…" line;
  - `helpUsage = "Usage: ai-dev help [command...]"`.
- [ ] T009 [US1] Add help interception to `internal/app/app.go` (R-01): before the `switch`, if `lookup` consumes `n` arguments and `args[n]` is `--help` or `-h`, render the help and return 0. Keep `help`, `--help` and `-h` as `args[0]` routed to `RunHelp(args[1:])`.
- [ ] T010 [US1] Update the expectations that pin the general-help text or `help extra`. These are in `internal/app/help_test.go` and in `cmd/ai-dev/version_test.go` if it asserts help text. Change nothing else (SC-005).

**Checkpoint:** US1 works on its own; `ai-dev help` and every `--help` are complete.

## Phase 4: User Story 2, shell completion (P2)

**Goal**: `ai-dev completion bash|zsh` (contracts/completion.md).

**Independent Test**: load each script in a real shell and complete the cases of contracts/completion.md.

- [ ] T011 [P] [US2] Tests in `internal/app/completion_test.go` for the command:
  - `completion bash` and `completion zsh` exit 0 with a non-empty script;
  - `completion`, `completion fish`, `completion bash extra` and `completion --zz` exit 2 with `Usage: ai-dev completion bash|zsh` alone on stderr;
  - two renders are byte-identical;
  - the script contains no value of `HOME`, `USER` or `PATH`, nor the working directory.
- [ ] T012 [P] [US2] bash tests in `internal/app/completion_test.go`. Run the script with `/bin/bash`, and with the `bash` on `PATH` when it differs, using `--norc --noprofile`. Set `COMP_WORDS` and `COMP_CWORD`, call `_ai_dev` and print `COMPREPLY`. Check these cases:
  - `ai-dev wo` gives `workflow`;
  - `ai-dev workflow ` gives the six commands;
  - `ai-dev workflow state --f` gives `--format`;
  - `--format ` gives `text json`;
  - `init plan --strategy ` gives the four strategies;
  - `agent run ` gives `claude-code codex-cli`;
  - `help init ` gives `apply inspect plan reconcile`;
  - `init plan --output ` gives the matching file names from `compgen -f`, in a temporary directory with known files;
  - `workflow state ` gives nothing: no file-name fallback (F010-FR-007);
  - `completion ` gives `bash zsh`.

  Assert that `/bin/bash` reports major version 3 on macOS, so bash 3.2 is really exercised. Skip with a message when a shell is missing.
- [ ] T013 [P] [US2] zsh tests in `internal/app/completion_test.go`:
  - `zsh -n` on the script;
  - the same cases as T012, run with stub `compadd`, `_files` and `compdef` functions that print their arguments, `words` and `CURRENT` set, and `_ai_dev` called.
  - `init plan --output ` calls `_files`.
  - Skip when zsh is missing.
- [ ] T014 [US2] Implement `internal/app/completion.go` (R-04):
  - `RunCompletion(args, stdout, stderr)`;
  - `bashScript()`, a single `_ai_dev` function in bash 3.2 syntax with `complete -o filenames -F _ai_dev ai-dev`; file options use `compgen -f -- "$cur"`, and `-o default` is not used;
  - `zshScript()`, a `#compdef ai-dev` function with `compdef _ai_dev ai-dev`.

  Both are rendered only from the catalog.
- [ ] T015 [US2] Add the `completion` route in `internal/app/app.go`. Set `topLevelUsage` to `Usage: ai-dev <doctor|agents|agent|workflow|init|git|version|help|completion> [options]` (F010-FR-010), and update the tests that pin the old line.

**Checkpoint:** both stories work independently.

## Phase 5: Polish

- [ ] T016 [P] Subprocess test in `cmd/ai-dev/help_test.go`:
  - the built binary answers `workflow state --help`, `help init plan`, `init --help </dev/null` and `completion bash`, with exit 0;
  - `echo x | ai-dev agent run --help` exits 0, and the pipe is left unread.
- [ ] T017 [P] Documentation:
  - **README:** the command reference mentions `--help` and `help <command>`, and adds `completion` with the install lines of contracts/completion.md. The index and the 009 status row are untouched, and a 010 status row is added.
  - **`specs/009-cli-help-version/spec.md`:** record the cross-feature amendments. F009-FR-004 is amended by F010-FR-005 (`help <command path>` is valid), and F009-FR-005 by F010-FR-010 (the top-level usage names `completion`). Add a note under each requirement and a Clarifications session.
- [ ] T018 Gates:
  - `gofmt -l .`;
  - `go vet ./...` and `GOOS=linux go vet ./...`;
  - staticcheck for darwin and linux;
  - `go test -count=1 ./...`;
  - `go test -race -count=3 ./internal/app ./cmd/ai-dev`;
  - the quickstart scenarios by hand.
- [ ] T019 Independent review (constitution VIII) of the diff against the spec and the contracts, then a PR to `dev` with CI green. The owner decides the merge.

## Dependencies

```text
T001 → T002 → T003 → T004 → T005 ─┬─► US1: T006, T007 → T008 → T009 → T010
                                  └─► US2: T011, T012, T013 → T014 → T015
US1 + US2 → T016, T017 → T018 → T019
```

- US1 and US2 depend only on the catalog (Phase 2) and can proceed in parallel.
- T011–T013 are separate test functions in one new file, written together.

## Parallel examples

- After T005: T006/T007 (US1 tests) alongside T011–T013 (US2 tests).
- After both stories: T016 and T017.

## Implementation strategy

- **MVP:** Phases 1–3. Per-command help alone closes the review finding's main gap.
- **Increment:** Phase 4 adds completion.
- Each phase ends with `go test ./internal/app ./cmd/ai-dev` green.
