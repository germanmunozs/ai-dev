# Tasks: Project Event Watch

**Input**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/watch.md` and `quickstart.md` in this directory.

**Tests**: mandatory (constitution IV), written before the code they cover. Each test task names the requirement or success criterion it proves.

- **No test may start a real coding agent.** Every agent is a fake executable on a private `PATH`, created like `makeFakePATH` in `cmd/ai-dev/main_test.go`.
- **Loop tests run inside `testing/synctest.Test`** (R-09), with fakes for `watch.Source`, `fswatch.Watcher` and the output writer, so they use no real time.
- **Adapter and end-to-end tests use the real file system** in `t.TempDir()`, with waits of at most 5 s for an expected event and a 1 s quiet period to assert that none comes.

**Owner decisions applied** (plan.md, 2026-11-26):

- The structural test covers `internal/watch` and `internal/fswatch`, and a separate test checks that `RunWatch` builds its source from `Root` and `Executor` only.
- `seq` is assigned at emission.
- `--once` works on every platform.
- A closed standard output exits 2 with no event, and a signal waits at most 2 s to write `watch_stopped`.
- The debounce window is fixed from the first notification.
- After `/speckit-analyze` (2026-11-26, "si, aplica los diez cambios"):
  - I1: `--once` on every platform;
  - U5: no evidence after an unreadable history;
  - U1: overflow tested with 1,100 features, with no test-only switch;
  - U2: `--feature` with no completion candidates;
  - C1: a recording executor for F015-FR-020;
  - A1: SC-001 over state changes;
  - I2: `seq` over emitted events;
  - I3: an unwritable output exits 2;
  - I4: `internal/system` excluded;
  - U3: deletion of the project tested end to end.
- After the second `/speckit-analyze` (2026-11-26, "si, aplica los tres cambios y haz el commit"):
  - U6: `--once` with an unreadable project;
  - C2: the full list of write calls and the `syscall.Open` flags;
  - I5: `inotifyInit`.

**Story order**: US2 (the snapshot) is built before US1, because every live event follows a snapshot (F015-FR-005). Both are P1, and the MVP is the two together.

## Phase 1: Setup

- [X] T001 Confirm the baseline on `feature/015-watch`: run `gofmt -l .`, `go vet ./...`, `GOOS=linux go vet ./...` and `go test ./...`, and record the result here.
  - List every test that pins the top-level usage line, the general help's command list, the command catalog or the completion command list. Use `git grep -n -E 'open\|version|"open", "version"' -- '*_test.go'`. At planning time these were:
    - `cmd/ai-dev/workflowguide_test.go:180`
    - `internal/app/catalog_test.go:50`
    - `internal/app/completion_test.go:75,84`
    - `internal/app/gitmanagement_regression_test.go:35-36`
    - `internal/app/help_test.go:61,102`
    - `internal/app/workflowguide_test.go:197-198`
    - `internal/app/workflowstate_test.go:268`
  - Record the list here. Only these expectations may change (A1, A2, SC-008).

  **Result (2026-11-26):** `gofmt -l .` empty; `go vet ./...` and `GOOS=linux go vet ./...` ok; `go test ./...` exit 0, every package ok. The pinned expectations are exactly the nine locations listed above.

## Phase 2: Foundational (blocks every story)

**The Human Gate history reader (A3, R-05)**

- [X] T002 [P] Tests `internal/gate/history_test.go`, written before T003 (F015-FR-008, R-05). Use an `fstest.MapFS`, plus `os.DirFS` over a temporary directory for the symbolic-link cases:
  - A missing record gives `([]Evidence{}, true)`.
  - A record with `state` and no `human_gates` gives an empty history, ok.
  - One approval entry, and one abandonment with a `reason`, decode to `Evidence` with `DecidedAt` parsed as RFC 3339 and `ReasonSet` true only when `reason` is present.
  - Not ok for:
    - a duplicated `human_gates`;
    - a case variant (`Human_Gates`) beside or instead of it;
    - `human_gates` that is not an array, or `null`;
    - an entry that is not an object;
    - an entry missing any of `feature_id`, `decision`, `approver`, `from_state`, `to_state`, `decided_at`;
    - a `decision` other than `approval` or `abandonment`;
    - an unrecognized state;
    - a `decided_at` that is not RFC 3339;
    - a non-string `reason`;
    - a record that is not a JSON object, or is invalid JSON;
    - a record that is a symbolic link, a directory or a FIFO;
    - a structurally invalid feature identifier (`..`, `a/b`).
- [X] T003 Implement `internal/gate/history.go`: `ReadHistory(root fs.FS, featureID string) ([]Evidence, bool)`.
  - Declare its own unexported wire structure, matching `internal/gatestore/record.go`'s `evidenceRecord` field for field.
  - Apply `workflow.FeatureDir` and the `Lstat` rules of `workflow.ResolveRecord` (read it first).
  - Decode the members with the same exact-name and duplicate rules as `stateField`.
  - It reads only through `fs.FS`. Comment with A3 and F015-FR-008.

  **Result (2026-11-26):** 006's boundary test, `internal/gate/boundary_test.go`, allows `gate` only `io/fs`, `strings`, `time`, `workflow` and `sdd`. Its allow lists gained `bytes`, `encoding/json`, `errors`, `path` and the selector `workflow.RecordFileName`, all read-only and used only by `ReadHistory` (A3). Its other rules are unchanged. 006's repository boundary test, `TestBoundarySoleWritePathImports` in `internal/gatestore/boundary_test.go`, lists the files that may import `gate`. It gained `internal/watch/picture.go`, which only calls `gate.ReadHistory` and holds no `gate.Store`.
- [X] T004 [P] Fuzz target `internal/gate/history_fuzz_test.go`: `FuzzReadHistory` writes the fuzzed bytes as `specs/f/workflow-state.json` in an `fstest.MapFS` and calls `ReadHistory`. It must never panic, and an ok result must never hold an entry with an empty `Decision`. Seed it with T002's valid and invalid records.
- [X] T005 [P] Test `internal/gatestore/history_roundtrip_test.go` (R-05):
  - Through the real POSIX store in a temporary project, commit an approval without a reason, then on another feature an abandonment with a reason containing quotes, a newline and Unicode.
  - `gate.ReadHistory` over `os.DirFS` of that project returns entries equal to the committed `gate.Evidence`, compared after truncating `DecidedAt` to seconds as the store records it.
  - Skip on platforms where the store is unsupported, as the existing store tests do.

**The watching port and the unavailable adapter**

- [X] T006 Create `internal/fswatch/fswatch.go`:
  - `type Kind int` with `Changed`, `Gone`, `Overflow`.
  - `type Change struct{ Path string; Kind Kind }`, where `Path` is the absolute path the adapter was asked to observe.
  - `type Watcher interface{ Sync(dirs, files []string) error; Events() <-chan Change; Close() error }`.
  - `type Unavailable struct{ Reason, Detail, Remedy string }` implementing `error`. Reasons: `platform_unsupported`, `watch_limit`, `open_file_limit`, `instance_limit`, `watch_failed`.
  - `func New() (Watcher, error)`, provided per platform.
- [X] T007 [P] Implement `internal/fswatch/unavailable_other.go` (`//go:build !darwin && !linux`): `New` returns `nil, Unavailable{Reason: "platform_unsupported", Detail: "watching is supported on macOS and Linux"}`. Add a test `internal/fswatch/unavailable_other_test.go` with the same build tag, and check that `GOOS=windows go vet ./internal/fswatch` and `GOOS=freebsd go build ./...` pass.

**The `internal/watch` package core**

- [X] T008 [P] Tests `internal/watch/event_test.go`, written before T009 (F015-FR-001 to FR-004, contracts/watch.md Envelope and Text format):
  - JSON encoding of every one of the eleven event types:
    - the member order is `schema_version`, `seq`, `type`, `time`, `project`, `data`;
    - `time` is UTC with milliseconds and `Z`;
    - the output is one line ending in `\n`, with no inner newline even when strings hold newlines;
    - embedded command objects (`json.RawMessage`) are kept byte for byte;
    - `human_gate` is `null` or the HumanGate object, with `reason` absent when not set;
    - `from` and `to` are `null` when not a recognized state.
  - Text rendering of each type, matching the contract's examples: every project string goes through the injected quote function. With `report.ShellInertQuote` passed in, every vector of `testdata/quote-vectors.json` used as a feature identifier and as an approver renders as the vector's `want`.
- [X] T009 Implement `internal/watch/event.go`: the `Event` type, the data types of contracts/watch.md, `EncodeJSON(e Event, seq int64) []byte` and `EncodeText(e Event, seq int64, quote func(string) string) []byte`.
- [X] T010 [P] Tests `internal/watch/queue_test.go`, written before T011, inside `synctest.Test` with a fake writer whose `Write` can be blocked and released (R-06, F015-FR-017, SC-005, owner decision 2):
  - Events are written in order with `seq` 1, 2, 3 and one `Write` per line.
  - A queue limit of 3 (unexported field), with the writer blocked and five pushes, discards the queue and leaves only `resync_required` (`queue_overflow`); `Push` then reports paused until the writer has written it.
  - The emitted `seq` values stay consecutive across the discard.
  - A `Write` error is reported once on the error channel, and every later `Push` is ignored.
  - `Flush(timeout)` returns true when everything queued is written, and false after 2 s with a writer that stays blocked (owner decision 4).
- [X] T011 Implement `internal/watch/queue.go`: `newQueue(w io.Writer, encode func(Event, int64) []byte, limit int)` with `Push(Event) (accepted bool)`, `Resumed() <-chan struct{}` (signalled after `resync_required` is written), `Err() <-chan error` and `Flush(time.Duration) bool`. The default limit is the constant `QueueLimit = 1000`.
- [X] T012 Create `internal/watch/source.go` with the port of plan.md Design:
  - `type Source interface{ Features() ([]string, error); State(id string) StateResult; Next(id string) []byte; Setup() []byte; Environment(ctx context.Context) EnvironmentResult }`.
  - `StateResult{JSON []byte; State string; Recognized bool; Outcome string}`.
  - `EnvironmentResult{JSON []byte; Checks []CheckStatus}`, where `CheckStatus{ID, Status, Remediation string}`.
  - Create `internal/watch/fake_test.go` with a scriptable `fakeSource` and a `fakeWatcher` (a channel of `fswatch.Change`, and a record of every `Sync`), shared by the loop tests.
- [X] T013 [P] Structural test `internal/watch/deps_test.go` (F015-FR-019, F015-FR-020, R-03, owner decision 1):
  - Run `go list -deps ./internal/watch ./internal/fswatch` from the module root.
  - Fail if the output contains `internal/gatestore`, `internal/configstore`, `internal/gitstore`, `internal/gitmanager`, `internal/gitprocess`, `internal/execution`, `internal/report`, `internal/system` or `os/exec`.
  - Also fail if any non-test file in these packages calls `os.Create`, `os.OpenFile` with a write flag, `os.WriteFile`, `os.Remove`, `os.RemoveAll`, `os.Rename`, `os.Mkdir`, `os.MkdirAll`, `os.Chmod`, `os.Chtimes`, `os.Chown`, `os.Lchown`, `os.Link`, `os.Symlink`, `os.Truncate` or `syscall.Flock`.
  - Also fail if any `syscall.Open` call's flags contain anything other than `O_EVTONLY`, `O_SYMLINK`, `O_CLOEXEC` or `O_RDONLY` (analysis C2).
  - Check all of this with a `go/parser` walk, like `internal/cockpit/boundary_test.go`.

**The command, catalog, help and completion (A1, A2)**

- [X] T014 Tests `internal/app/watch_test.go` (options part), written before T015 (contracts/watch.md Usage, F015-FR-018, F015-FR-023):
  - Each of these exits 2 with the usage line on standard error and nothing on standard output:
    - an unknown option, or an extra argument;
    - `--format xml`;
    - `--debounce 49ms` and `6s`;
    - `--heartbeat 4s` and `301s`;
    - `--environment-interval 4s` and `3601s`;
    - `--feature ..` and `--feature a/b`;
    - a malformed duration.
  - `--environment-interval 0` is accepted. Defaults are 250 ms, 15 s, 60 s and `text`. `--feature` repeats.
  - `RunWatch` builds its `watch.Source` only from `deps.Root` and `deps.Executor`: the source constructor takes exactly `(fs.FS, system.Executor)`, checked by a compile-time assignment to `func(fs.FS, system.Executor) watch.Source`, and `RunWatch` succeeds with every other `Dependencies` field nil (owner decision 1).
- [X] T015 Implement `internal/app/watch.go`: `RunWatch(ctx context.Context, args []string, deps Dependencies) int`, with option parsing, validation and the usage constant `watchUsage` from contracts/watch.md. Implement `newWatchSource(root fs.FS, executor system.Executor) watch.Source` (filled by T017), and leave the run step returning a not-implemented internal error until T020.
- [X] T016 Update the catalog, help, usage and completion (A1, A2, F015-FR-023):
  - `internal/app/catalog.go` gains the entry `watch`, after `open`:
    - description "Report project changes as they happen, read-only";
    - options `--format` (text, json), `--feature` (repeatable), `--debounce`, `--heartbeat`, `--environment-interval` and `--once`;
    - `changes: "nothing; reads the project and reports its changes"`;
    - `exitCodes: exitCodesDomain`.
  - `internal/app/app.go`: route `watch`, and set `topLevelUsage` to `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|open|watch|version|help|completion> [options]`.
  - Completion: `--feature` is a free-text value with no candidates, like the other feature-identifier arguments (analysis U2).
  - Update the expectations T001 listed, and only those. Add `{"ai-dev", "watch", "-"}` and `{"ai-dev", "watch", "--format", ""}` rows to `internal/app/completion_test.go`, and `watch` to the `help` loop in `internal/app/help_test.go:61`.

  **Result (2026-11-26):** `TestHelpCommandsAreRouted` (`internal/app/help_test.go`, Feature 010) runs every catalog command with no arguments. `watch` with no arguments runs until it is stopped, so the full suite hung there once the loop existed. That test now runs `watch --once`, which still proves the routing and that no process is launched. This is the only change to a 010 test beyond the expectations T001 listed.

**Checkpoint**: `go test ./...` passes. `ai-dev watch --help` prints the contract's usage. The structural test passes on an empty loop.

## Phase 3: User Story 2, a complete snapshot on connection (P1) 🎯 MVP part 1

**Goal**: the first event is a snapshot equal to the four commands' JSON, and `--once` emits only it. **Independent test**: fixture projects with features in several states, compared with `workflow state`, `workflow next`, `setup show` and `doctor` in JSON.

- [X] T017 [P] [US2] Tests `internal/app/watch_source_test.go`, written before T018 (F015-FR-006, SC-007):
  - For fixture projects covering every Workflow State, an invalid record, a symbolic-link feature directory (not listed), an absent `specs/`, a valid, an invalid and a missing `.ai-dev/agents.json`, and fake agents on a private `PATH` through the real `system.Executor`, check that:
    - `Features()` lists exactly what the cockpit's `LoadFeatures` lists, in name order;
    - `State(id).JSON` is byte-equal to `RunWorkflowState(["--format","json",id])` standard output;
    - `Next(id)` is byte-equal to `RunWorkflowNext`'s;
    - `Setup()` is byte-equal to `RunSetupShow(["--format","json"])`'s;
    - `Environment(ctx).JSON` is byte-equal to `ai-dev doctor --format json`'s, with `Checks` matching its statuses.
  - Also check an unknown feature identifier for `State` and `Next`.
- [X] T018 [US2] Implement `newWatchSource` in `internal/app/watch.go` with `workflow.FeatureExists`, `workflow.EvaluateState` with `report.WriteWorkflowStateJSON`, `sdd.Evaluate` with `report.WriteWorkflowNextJSON`, `setupShowReport` with its JSON writer, and `RunDoctor` with `report.WriteJSON`. Reuse the commands' functions; do not copy them.
- [X] T019 [P] [US2] Tests `internal/watch/snapshot_test.go`, written before T020, in `synctest.Test` (F015-FR-005, F015-FR-013, F015-FR-014, data-model.md Observed Feature):
  - The snapshot's `features` are in name order, with `state` and `next` the source's bytes.
  - `artifacts` always lists `spec.md`, `plan.md` and `tasks.md`, in that order: `present` with `size` and `modified` for a regular file, `absent`, and `not_regular_file` for a symbolic link (not followed) or a directory.
  - `--feature b --feature zz` gives only `b`, plus `zz` with the source's `feature_unknown` objects and the three artifacts `absent`.
  - `seq` is 1.
  - With `Once`, the result is exit 0 after exactly one event, and the watcher constructor is never called. A constructor that returns `Unavailable` still gives the snapshot and exit 0 (F015-FR-014, analysis I1). With an unreadable project directory, `Once` emits only `watch_stopped` (`watch_unavailable`) and gives exit 1 (analysis U6).
  - Without `Once`, `Sync` is called with the watch set of data-model.md before the source is first read (R-02). The order is checked by a log shared by the fake source and the fake watcher.
- [X] T020 [US2] Implement `internal/watch/picture.go` (the Project Picture, artifacts by `Lstat` through `fs.FS`, the history length through `gate.ReadHistory`, and the watch set) and the start of `internal/watch/loop.go`:
  - `Run(ctx, Options, Source, func() (fswatch.Watcher, error), root fs.FS, output io.Writer) Result`.
  - It registers watches unless `Once`, computes and queues the snapshot, and returns.
  - Wire it into `RunWatch`, with the absolute project path from `os.Getwd`, the JSON or text encoder, and `report.ShellInertQuote`.
  **Result (2026-11-26):** `Run` takes `(ctx, Options, Config)`, where `Config` holds the source, the watcher constructor, the root, the project path, the output, the encoder and the identity function. This is the same set of ports, grouped. contracts/watch.md said a named feature that does not exist has "empty artifacts", while its Artifact rule always lists the three names and this task asks for them `absent`; the contract now says "its three artifacts `absent`". The cockpit's `specs/*` listing moved into `listFeatures` in `internal/app/cockpit.go`, shared with the watch source, so the rule is not copied.

- [X] T021 [US2] End-to-end test `cmd/ai-dev/watch_e2e_test.go`, `TestWatchOnce`:
  - Build the binary as `TestCommandContractSubprocess` does, and use a fixture with three features (`specified`, `implemented`, invalid record) and fake agents.
  - `ai-dev watch --format json --once` prints exactly one line and exits 0.
  - The line's `data.features[*].state` and `next`, `data.setup` and `data.environment` are equal to the outputs of the same binary's `workflow state`, `workflow next`, `setup show` and `doctor` with `--format json`.
  - `--format text --once` prints one `event=snapshot` line.

**Checkpoint**: US2 works on its own with `--once`.

## Phase 4: User Story 1, a client learns of a state change at once (P1) 🎯 MVP part 2

**Goal**: a persisted state change becomes one `workflow_state_changed` within a second, with the evidence only under F015-FR-008. **Independent test**: a running `watch` and `ai-dev workflow approve`, a refused request and a manual edit in another process.

- [X] T022 [P] [US1] Tests `internal/fswatch/kqueue_darwin_test.go` (`//go:build darwin`), written before T023 (R-01):
  - In a temporary project, `Sync` with the feature directory and its `workflow-state.json`, then check each case:
    - an atomic replace (write a temporary file, rename it over) yields at least one `Changed` for the directory or the file within 5 s;
    - an in-place write of `spec.md` yields `Changed`;
    - after a replace, a second `Sync` with the same paths reopens the file (its inode changed), and a further in-place write of the new file yields `Changed`;
    - a symbolic link passed as a file is opened as the link itself: a write to its target outside the watch set yields nothing within 1 s;
    - `Close` closes every descriptor: the process's open-descriptor count, read from `/dev/fd`, returns to its value before `New`.
- [X] T023 [US1] Implement `internal/fswatch/kqueue_darwin.go` (`//go:build darwin`) per R-01:
  - `syscall.Kqueue`, and per path `syscall.Open(path, O_EVTONLY|O_SYMLINK|O_CLOEXEC, 0)`;
  - `EV_ADD|EV_CLEAR` with `NOTE_WRITE|NOTE_EXTEND|NOTE_ATTRIB|NOTE_DELETE|NOTE_RENAME`;
  - a reader goroutine blocked in `Kevent` that retries on `EINTR` and maps events to `Change` (`Gone` for `NOTE_DELETE` or `NOTE_RENAME` on a directory);
  - `Sync`, which adds, removes and reopens by device and inode;
  - `EMFILE` becomes `Unavailable{Reason: "open_file_limit", Remedy: "raise the open-file limit (ulimit -n)"}`.
- [X] T024 [P] [US1] Tests `internal/fswatch/inotify_linux_test.go` (`//go:build linux`), written before T025 (R-01):
  - The same atomic-replace and in-place-write cases as T022, through directory watches only; `Sync` ignores `files`.
  - A symbolic-link directory is not watched (`IN_DONT_FOLLOW|IN_ONLYDIR`).
  - Through an injected `addWatch` returning `ENOSPC`, `Sync` returns `Unavailable{Reason: "watch_limit"}`, with a detail naming `fs.inotify.max_user_watches` and a remedy `sudo sysctl fs.inotify.max_user_watches=<n>`; an injected `EMFILE` from `inotifyInit` gives `instance_limit` naming `fs.inotify.max_user_instances`.
  - An injected `IN_Q_OVERFLOW` record yields `Overflow`.
- [X] T025 [US1] Implement `internal/fswatch/inotify_linux.go` (`//go:build linux`) per R-01:
  - `syscall.InotifyInit1(IN_CLOEXEC)`, with `addWatch` and `inotifyInit` held in package variables for T024 (`init` is reserved in Go, analysis I5);
  - the mask `IN_CREATE|IN_DELETE|IN_MOVED_FROM|IN_MOVED_TO|IN_MODIFY|IN_CLOSE_WRITE|IN_ATTRIB|IN_DELETE_SELF|IN_MOVE_SELF|IN_DONT_FOLLOW|IN_ONLYDIR`;
  - a reader goroutine that parses `InotifyEvent` records and maps them to `Change` with the watched directory's path (`Gone` for `IN_DELETE_SELF` or `IN_MOVE_SELF`);
  - `Sync`, which adds and removes watches by path. Run this task's tests in CI's `gates` job on `ubuntu-latest`, since the owner's machine has no Linux environment (research.md R-01).
  **Result (2026-11-26):** the Linux adapter compiles and passes `GOOS=linux go vet`, but has not run on the owner's machine (no Linux environment). Its tests run for the first time in CI's `gates` job on `ubuntu-latest`.

- [X] T026 [P] [US1] Tests `internal/watch/state_test.go`, written before T027, in `synctest.Test` (F015-FR-007, F015-FR-008, F015-FR-010, R-04, SC-002, owner decision 5):
  - A `Changed` on a feature directory, with the source's state going from `implemented` to `approved` and the history growing by one approval entry to `approved`, yields exactly one `workflow_state_changed` after the 250 ms debounce. It has `from: "implemented"`, `to: "approved"`, `outcome: "state_reported"`, `state` equal to the source's bytes, and `human_gate` equal to that entry.
  - A notification with the state unchanged (a refused request) yields no event.
  - A state change with the history unchanged (a manual edit), or with an unreadable history, gives `human_gate: null`.
  - A history unreadable at the previous observation (−1) and readable now, with a last entry whose `to_state` matches the new state, still gives `human_gate: null`; the following change, with a new matching entry, carries it (analysis U5).
  - A new entry whose `to_state` differs from the new state gives null. Two entries appended within one window give the last one only if its `to_state` matches.
  - A record created from `draft` gives `from: "draft"`.
  - A change to `state_invalid` gives `to: null` and `outcome: "state_invalid"`.
  - Notifications at 0, 100 and 200 ms with a 250 ms debounce are evaluated once, at 250 ms (the window does not restart).
  - A notification that arrives while the snapshot is computed, for a change the snapshot already includes, yields no event; for one it does not include, it yields the event after the snapshot (R-02).
- [X] T027 [US1] Extend `internal/watch/loop.go` and add `internal/watch/diff.go`:
  - the change keys and fixed windows of data-model.md;
  - the evaluation of `feature:<id>`, with the F015-FR-008 rule on `HistoryLen`;
  - `Sync` after each evaluation;
  - the pure `diff(prev, cur Picture) []Event` for `workflow_state_changed`.
- [X] T028 [US1] End-to-end test in `cmd/ai-dev/watch_e2e_test.go`, `TestWatchHumanGates` (SC-002, US1 scenarios 1–4):
  - Start the built `ai-dev watch --format json` on a fixture, and read its standard output line by line.
  - Through the same binary, approve an `implemented` feature, then repeat the approval (refused, terminal), then run `workflow approve` with a missing artifact (refused), then edit another feature's `state` by hand.
  - Expect exactly one `workflow_state_changed` with the approval's evidence, none for the refusals, and one with `human_gate: null` for the edit. Stop it with `SIGTERM`.

**Checkpoint**: US1 and US2 form the MVP.

## Phase 5: User Story 3, changes to a feature's artifacts and next step (P2)

**Goal**: artifacts, next steps and features added or removed are reported, debounced. **Independent test**: create, rewrite and delete artifacts and feature directories while `watch` runs.

- [ ] T029 [P] [US3] Tests `internal/watch/artifacts_test.go`, written before T030, in `synctest.Test` (F015-FR-009, US3 scenarios 1–4):
  - Creating `plan.md` yields `artifacts_changed` with the three entries, then `next_step_changed` with the source's new `workflow next` bytes, in that order, in one evaluation.
  - A change in size or modification time alone yields `artifacts_changed` only.
  - Five saves within 200 ms yield one `artifacts_changed`.
  - A new directory under `specs` yields `feature_added` with the full Feature, and no per-feature events for it in the same evaluation; a removed one yields `feature_removed`.
  - `specs/` created after start is observed (the watch set gains it) and its features are added.
  - With `--feature a`, changes to `b` yield nothing.
  - No event contains an artifact's content: fixture artifacts hold a marker string, and the output is checked not to contain it (F015-FR-021).
- [ ] T030 [US3] Extend `diff.go` and `loop.go` with `artifacts_changed`, `next_step_changed`, the `specs` key, `feature_added` and `feature_removed`, and the order of contracts/watch.md.
- [ ] T031 [P] [US3] Adapter tests in `internal/fswatch/kqueue_darwin_test.go` and `internal/fswatch/inotify_linux_test.go`:
  - `mkdir specs/new` yields `Changed` for `specs`;
  - `rm -r` of a watched feature directory yields `Changed` for `specs`, and `Gone` for that directory;
  - `specs` created after `Sync` of the project directory yields `Changed` for the project directory.

  Extend T023 and T025 if a case fails.

## Phase 6: User Story 4, agent configuration and environment health (P2)

**Goal**: configuration and environment changes are reported. **Independent test**: `setup apply`, an invalid `agents.json`, and a fake agent removed from the fake executor's `PATH`.

- [ ] T032 [P] [US4] Tests `internal/watch/environment_test.go`, written before T033, in `synctest.Test` (F015-FR-012, US4 scenarios 1–3):
  - A `Changed` on `.ai-dev` or `agents.json` with new `Setup()` bytes yields `agents_config_changed` with those bytes.
  - Bytes with outcome `configuration_invalid` are reported as such, never replaced by defaults.
  - With `--environment-interval 60s`, a check going from `pass` to `fail` at the 60 s tick yields `environment_changed`, listing only that check with `from: "pass"`, `to: "fail"` and its remediation, plus the full environment.
  - The same tick re-evaluates `Setup()` (agent availability) and emits `agents_config_changed` if it changed.
  - A check that appears or disappears has `null` on that side.
  - With `--environment-interval 0` no tick occurs.
  - `heartbeat` is emitted every 15 s by default and every `--heartbeat`, with `data: {}`.
- [ ] T033 [US4] Extend `loop.go` and `diff.go` with the `setup` key, the revalidation ticker, the heartbeat ticker, `agents_config_changed` and `environment_changed`.
- [ ] T034 [US4] Tests in `internal/app/watch_test.go`:
  - With the real source over a fixture and a fake executor whose `LookPath` stops finding a fake agent between two calls, `Environment` and `Setup` change accordingly. This proves the revalidation path with the commands' own functions, and runs no agent.
  - **F015-FR-020 (analysis C1):** a recording executor wraps the fake one. Across `Features`, `State`, `Next`, `Setup` and two `Environment` calls, its only `Run` calls are `git rev-parse --is-inside-work-tree`, one per `Environment` call. `LookPath` calls are allowed; no other `Run` call is.

## Phase 7: User Story 5, predictable termination and recovery (P3)

**Goal**: signals, overflow, a vanished project and unavailable watching end or recover predictably. **Independent test**: signals, an unread pipe, the project removed, and an injected watch limit.

- [ ] T035 [P] [US5] Tests `internal/watch/stop_test.go`, written before T036, in `synctest.Test` (F015-FR-015 to FR-018, owner decisions 2 and 4):
  - Cancelling the context yields `watch_stopped` with `signal` as the last event, and exit 0. With a writer that stays blocked, `Run` returns exit 0 after 2 s.
  - A `Gone` on the project directory, or a project identity that changed (injected), yields `watch_stopped` with `project_gone`, and exit 1.
  - A watcher constructor returning `Unavailable` yields only `watch_stopped` (`watch_unavailable`, with the detail and remedy), no snapshot, and exit 1.
  - A `Sync` failing with `Unavailable` after start yields the same, and exit 1.
  - A watcher `Overflow` yields `resync_required` (`watch_overflow`) and then a new `snapshot`.
  - A queue overflow yields `resync_required` (`queue_overflow`), then, once the writer resumes, a `snapshot` computed after resumption, with the source state changed during the pause visible in it.
  - A source panic, recovered, yields `watch_stopped` with `internal_error`, and exit 2.
  - A writer error (closed output) yields exit 2 and no further `Write`.
- [ ] T036 [US5] Extend `loop.go`: the stop reasons, project identity (device and inode, injected through `Options` for tests), overflow pause and resume through the queue, the final `Flush(2 * time.Second)`, and panic recovery. Every exit path closes the watcher.
- [ ] T037 [US5] Wire signals and output errors in `internal/app/watch.go`: `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` and `brokenPipeAsError`. An unreadable project directory at start gives `watch_unavailable` with detail "the project directory cannot be read" and exit 1. Map `Result` to the exit codes of contracts/watch.md.
- [ ] T038 [US5] End-to-end tests in `cmd/ai-dev/watch_e2e_test.go` (US5 scenarios 1, 3 and 4, SC-005):
  - `TestWatchSignals`: `SIGINT` and `SIGTERM` each end with a `watch_stopped` `signal` line, and exit 0.
  - `TestWatchProjectGone`: in two subtests, renaming and deleting (`os.RemoveAll`) the project directory each end with `project_gone`, and exit 1 (analysis U3).
  - `TestWatchBlockedClient` (analysis U1: no test-only option, variable or build tag):
    1. Create a project with 1,100 features, each with `spec.md`.
    2. Start `watch --debounce 50ms` with its standard output on a pipe the test does not read.
    3. Rewrite every `spec.md` at once, which yields more than 1,000 events in one evaluation.
    4. Check that the process's RSS stays bounded (under 64 MB, measured with `ps -o rss=`).
    5. Read: the stream contains `resync_required` (`queue_overflow`) followed by a `snapshot` that lists all 1,100 features with their new artifacts.
  - `TestWatchClosedOutput`: closing the read end of the pipe gives exit 2.

## Phase 8: Polish and cross-cutting concerns

- [ ] T039 [P] End-to-end test `TestWatchLatency` in `cmd/ai-dev/watch_e2e_test.go` (SC-001):
  - 40 state changes, alternating Human Gate approvals on fresh fixture features and in-place edits of another feature's `state`, one every 300 ms (analysis A1).
  - Each latency runs from the end of the change (the `workflow approve` process exiting, or the write returning) to the matching `workflow_state_changed` line.
  - The 95th percentile is under 1 s. Log the percentiles.
- [ ] T040 [P] End-to-end test `TestWatchWritesNothing` (SC-003, F015-FR-019):
  - Before starting `watch`, record the project tree, every path with its mode, size, modification time and SHA-256.
  - Make at least 50 changes in a separate subtree, `specs/zz-*`, through the test.
  - Stop `watch`, and check that every recorded path outside that subtree is identical and that no new path appeared outside it.
- [ ] T041 [P] End-to-end test `TestWatchHostileNames` (SC-006, F015-FR-022): feature directories, approvers and reasons taken from `testdata/quote-vectors.json` and hostile strings (escape, OSC, bidirectional overrides, newlines). Check that:
  - every JSON line parses, with schema `ai-dev.watch/v1` and consecutive `seq`;
  - no byte below 0x20 other than `\n`, and no ESC, reaches standard output in `text` format.
- [ ] T042 [P] Add `FuzzReadHistory` to the fuzz step of `.github/workflows/ci.yml`: `go test -run '^$' -fuzz '^FuzzReadHistory$' -fuzztime 20s ./internal/gate`.
- [ ] T043 [P] Documentation in `README.md` and `README.es.md`:
  - `ai-dev watch` and its options;
  - the event contract with the example of contracts/watch.md;
  - the compatibility rules;
  - how a client detects support (`ai-dev watch --help`, or the version, from v0.4.0);
  - the recovery procedure (on `resync_required` or a gap, replace all state with the next snapshot);
  - the platform limits and their remedies;
  - that events inform but never approve.

  Also add `CHANGELOG.md` and `CHANGELOG.es.md` entries under `[Unreleased]`, and a `SECURITY.md` paragraph on the read-only boundary (F015-FR-024).
- [ ] T044 Run the quality gates of plan.md and record each result here:
  - `gofmt -l .`;
  - `go vet ./...` and `GOOS=linux go vet ./...`;
  - `go test -race ./...`;
  - the fuzz targets for 20 s;
  - `staticcheck ./...` for linux and darwin;
  - `govulncheck ./...`;
  - `go.mod` and `go.sum` unchanged;
  - the cross-builds, and `CGO_ENABLED=0 go build ./cmd/ai-dev` for darwin and linux;
  - the regression suites with only T001's expectations changed (SC-008).
- [ ] T045 Owner's manual check: quickstart.md steps 1–6, including SC-004 (under 0.5% CPU and under 30 MB with 100 features, measured with `ps`). Record the values, the date and the platform here.
- [ ] T046 Independent review before the PR (constitution VIII), as in 012–014. Record here the implementing agent, the reviewing agent, the findings and their resolution, and update the governing artifacts for any deviation.

## Dependencies

- **Phase 1** comes first, then **Phase 2**:
  - T002 → T003, then T004 and T005.
  - T006 → T007.
  - T008 → T009; T010 → T011; T012; T013.
  - T014 → T015 → T016.
- **Phase 3 (US2)** needs Phase 2: T017 → T018; T019 → T020 (needs T003, T009, T011, T012); then T021.
- **Phase 4 (US1)** needs US2: T022 → T023 and T024 → T025 can proceed in parallel with T026 → T027; then T028 needs T023 or T025 for the local platform, and T027.
- **Phase 5 (US3)** and **Phase 6 (US4)** need US1's loop (T027) and are independent of each other.
- **Phase 7 (US5)** needs T027. T038 needs T036 and T037.
- **Phase 8** needs every story. T044 to T046 come last, in order.

## Parallel examples

- Phase 2: T002, T006, T008, T010, T013 and T014 touch different files and can start together.
- US1: T022 (darwin adapter tests), T024 (linux adapter tests) and T026 (loop tests).
- US3 and US4: T029 and T032 can be written in parallel, though T030 and T033 both edit `loop.go` and `diff.go`, so they are applied one after the other.
- Polish: T039, T040, T041, T042 and T043.

## Implementation strategy

1. **MVP**: Phases 1–4, which give the snapshot and live state changes with the recorded evidence, on macOS and Linux. Stop for owner validation (quickstart steps 1 and 2) before Phase 5.
2. **Increment**: US3 and US4, in either order.
3. **Robustness**: US5.
4. **Close**: Phase 8, with the manual SC-004 measurement and the independent review before the PR.
