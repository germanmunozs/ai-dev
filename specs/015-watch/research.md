# Research: Project Event Watch

**Feature**: 015 | **Date**: 2026-11-26 | **Plan**: [plan.md](plan.md)

No real coding agent ran during this research. The measurements ran on the owner's machine (macOS, Apple silicon, Go 1.27.4) with throwaway programs in the session's scratch directory, outside the repository.

## R-01: Watching mechanism

- **Decision:** the operating systems' change notifications through the standard library `syscall` package: kqueue on macOS and inotify on Linux. Every other platform gets an adapter that always reports `watch_unavailable`. There is no new module, and release builds stay `CGO_ENABLED=0`.
- **Measurements** (fixture: 100 features, each with `workflow-state.json`, `spec.md`, `plan.md` and `tasks.md`, plus `.ai-dev/agents.json`; 504 paths):

  | Mechanism | Idle CPU (one core) | Max RSS | First notification |
  |---|---|---|---|
  | `lstat` scan every 250 ms (20 s) | 0.93% | 10.4 MB | up to the period |
  | `lstat` scan every 100 ms (20 s) | 2.27% | 10.4 MB | up to the period |
  | kqueue, 504 vnodes (10 s idle) | 0.0015% (151 µs) | 5.2 MB | 0.11–1.13 ms |

- **Rationale:**
  - An internal scan fails SC-004 (under 0.5%) at the period SC-001 needs, so it is rejected by measurement. A 500 ms scan would sit near both limits at once: about 0.47% CPU, and 750 ms worst-case latency with the 250 ms debounce.
  - kqueue is idle when nothing changes and meets both criteria by two orders of magnitude.
  - `syscall` already has everything needed: `Kqueue`, `Kevent`, `SetKevent`, `EVFILT_VNODE`, the `NOTE_*` flags and `O_EVTONLY` on darwin; `InotifyInit1`, `InotifyAddWatch`, `InotifyRmWatch` and the `IN_*` masks on linux. `golang.org/x/sys` is not needed.
- **Observed kqueue events** (prototype, macOS):

  | Action | Events |
  |---|---|
  | Atomic replace of `workflow-state.json` (write a temporary file, rename it over) | the directory gets `NOTE_WRITE` twice; the old file gets `NOTE_DELETE` |
  | In-place write of `spec.md` | `NOTE_ATTRIB`, then `NOTE_WRITE\|NOTE_EXTEND` |
  | Five saves of `plan.md` 40 ms apart | ten events; the debounce groups them (R-04) |
  | `mkdir specs/200-new` | `specs` gets `NOTE_WRITE\|NOTE_LINK` |
  | `rm -r specs/001-feature` | each file and the directory get `NOTE_DELETE\|NOTE_LINK`; `specs` gets `NOTE_WRITE\|NOTE_LINK` |
  | Atomic replace of `.ai-dev/agents.json` | the same as the record replace |
  | Rename of the project directory | the project directory gets `NOTE_RENAME` |

- **How each case is handled:**
  - kqueue observes each vnode, so the macOS adapter opens one `O_EVTONLY|O_SYMLINK|O_CLOEXEC` descriptor for each watched directory and file. `O_EVTONLY` holds no lock and does not keep a volume busy. `O_SYMLINK` means a symbolic link is opened as itself, never followed.
  - inotify reports the events of a directory's entries, so the Linux adapter watches directories only, with `IN_DONT_FOLLOW|IN_ONLYDIR` and the mask `IN_CREATE|IN_DELETE|IN_MOVED_FROM|IN_MOVED_TO|IN_MODIFY|IN_CLOSE_WRITE|IN_ATTRIB|IN_DELETE_SELF|IN_MOVE_SELF`.
  - **Atomic renames:** an event only marks its feature (or the configuration) as changed. After the debounce, the results are recomputed from the files as they are and compared with the previous results, so a rename is one change (FR-007). The adapter then re-synchronizes its watch set: on macOS, a file whose inode changed is reopened.
  - **Directories created or removed:** the parent's event (`specs`, or the project directory for `specs` and `.ai-dev` themselves) triggers a new listing and a re-synchronization of the watch set.
  - **The project directory disappearing:** `NOTE_DELETE` or `NOTE_RENAME` on the project directory (macOS), or `IN_DELETE_SELF` or `IN_MOVE_SELF` on it (Linux). Every evaluation also checks that the project path still names the same directory (device and inode). Any of these gives `project_gone`.
  - **Limits:**
    - On Linux, `InotifyAddWatch` failing with `ENOSPC` means `fs.inotify.max_user_watches`; the remedy names `sysctl fs.inotify.max_user_watches=<n>`. `InotifyInit1` failing with `EMFILE` means `fs.inotify.max_user_instances`.
    - On macOS, `open` failing with `EMFILE` means the open-file limit. The Go runtime already raises the soft limit to the hard limit at start-up; the owner's machine has 1,048,576 and `kern.maxfilesperproc` 61,440, so 100 features need about 500 descriptors.
    - Each limit gives `watch_unavailable`, with the limit and its remedy.
  - **Queue overflow of the kernel:** `IN_Q_OVERFLOW` (Linux) gives `resync_required` with reason `watch_overflow`, then a new snapshot. kqueue merges events per vnode and has no overflow.
- **Verification limits:** the Linux adapter could not be run on the owner's machine: no Linux environment was available without downloading images. It is verified by the same tests in CI's `gates` job on `ubuntu-latest`, on ext4 (the job already checks the file system). The macOS adapter is verified locally and on `macos-latest`.
- **Alternatives considered:**
  - FSEvents: needs cgo and the CoreServices framework, so release builds would have to change. Rejected by the owner's constraint.
  - `github.com/fsnotify/fsnotify`: a new dependency for about 300 lines of adapter code. Rejected under the dependency policy.
  - `golang.org/x/sys/unix`: already an indirect module, but it adds nothing over `syscall` for these calls.
  - An internal scan: rejected by the measurement above.

## R-02: A snapshot that loses no change

- **Decision:** register the watches before reading anything. Every notification that arrives while the snapshot is being computed marks its key as changed. After the snapshot is emitted, the marked keys are evaluated against the snapshot's results, and only real differences become events.
- **Rationale:** a change made before registration is read by the snapshot. A change made after registration produces a notification, so it is either already visible to the snapshot (then its evaluation finds no difference and emits nothing) or it becomes an event. This is the F015-FR-005 guarantee, with no lock on the project.
- **Alternatives considered:** retrying the snapshot until two consecutive computations agree. That is unbounded under constant changes and still has a window between the last computation and registration.

## R-03: Reusing the existing computations (F015-FR-006), and the read-only boundary (F015-FR-019)

- **Finding:** `internal/report`, which renders every command's JSON, depends on `internal/execution` and `internal/gitmanager` (for the types of the agent-run and Git reports). A `watch` package that imports `report` would therefore depend on agent execution and the Git manager, against the owner's decision 3.
- **Decision:**
  - The new package `internal/watch` holds the loop, the diff, the queue and the event encoding. It depends only on `internal/workflow`, `internal/gate` (for the new read-only history reader, R-05), `internal/sdd` (artifact names) and the standard library.
  - It receives the commands' results through a port, `watch.Source`, implemented in `internal/app/watch.go` with the same functions the commands use:
    - `workflow.EvaluateState` and `report.WriteWorkflowStateJSON` for the state;
    - `sdd.Evaluate` and `report.WriteWorkflowNextJSON` for the next step;
    - `setupShowReport` and its JSON writer for the agents;
    - `RunDoctor` and `report.WriteJSON` for the environment;
    - the cockpit's `specs/*` listing rule (`workflow.FeatureExists`) for the features.
  - The port returns each command's JSON as bytes, and `watch` embeds them unchanged (`json.RawMessage`). The values are therefore the commands' own bytes (SC-007), and a change is detected by comparing bytes.
  - The app adapter receives only a read-only `fs.FS` and the `system.Executor` (`LookPath`, and `git rev-parse` through the doctor check), the same capabilities as `RunWorkflowState`, `RunSetupShow` and `RunDoctor`. It receives no gate store, configuration store, Git dependency or launcher.
- **Structural test:** `go list -deps ./internal/watch` must not contain `internal/gatestore`, `internal/configstore`, `internal/gitstore`, `internal/gitmanager`, `internal/gitprocess`, `internal/execution`, `internal/report` or `os/exec`. A second test checks that `RunWatch` builds its source from `deps.Root` and `deps.Executor` only.
- **Alternatives considered:**
  - Splitting `internal/report` into per-command packages: a large change to 001–014 code for no behavior change.
  - Reimplementing the JSON in `watch`: it would drift from the commands, against F015-FR-006.

## R-04: Debounce, heartbeat and revalidation

- **Decision:**
  - The debounce is a fixed window per key that starts at the first notification. Further notifications within the window are absorbed, and the key is evaluated when the window ends. A window that restarts on every notification was rejected, because a steady stream of saves could delay an event indefinitely.
  - Keys:
    - `feature:<id>`: the feature's state, artifacts, next step and Human Gate history;
    - `specs`: the listing of features;
    - `setup`: `.ai-dev/agents.json` and the agents;
    - `project`: the project directory itself.
  - The worst-case latency is the debounce plus one evaluation, 250 ms plus a few milliseconds by default, within SC-001's 1 s.
  - `heartbeat` uses a ticker of `--heartbeat`. Environment revalidation uses a ticker of `--environment-interval` (none when `0`); it evaluates the doctor checks and the setup report (agent availability) and emits only on a difference.

## R-05: Reading the Human Gate history (Amendment A3)

- **Decision:** add `gate.ReadHistory(root fs.FS, featureID string) (entries []gate.Evidence, ok bool)` in `internal/gate`, read-only through `fs.FS`. Its rules follow `workflow.ResolveRecord`:
  - The record is checked with `Lstat`, and a symbolic link or non-regular file gives not ok.
  - The record must be one JSON object with exactly one `human_gates` member, with no case variant.
  - That member must be an array of objects, each with `feature_id`, `decision` (`approval` or `abandonment`), `approver`, `from_state`, `to_state` (recognized states), `decided_at` (RFC 3339) and an optional `reason` string.
  - A missing record gives an empty history; anything else gives not ok.
- **Rule of F015-FR-008:** the loop keeps each feature's history length. On a state change, if the history now holds more entries and the last entry's `to_state` equals the new state, the event carries that entry. In every other case, including an unreadable history, it carries `human_gate: null`.
- **Wire-format agreement:** the wire structure is defined in `gatestore/record.go` (unexported), and `gate` cannot import `gatestore`, so the reader declares its own copy. A round-trip test in `internal/gatestore`:
  - commits approvals and abandonments, with and without a reason, through the real store;
  - reads them back with `gate.ReadHistory`;
  - requires equal entries.

  A change to either side fails that test.
- **Alternatives considered:**
  - A reader in `gatestore`: `watch` would then depend on the store that writes, against decision 3.
  - Moving the wire structure to `gate`: it changes 006's store code for no behavior change.

## R-06: The bounded queue and a blocked standard output

- **Decision:**
  - One writer goroutine writes each event line with one `Write` call. The loop puts events in a queue of at most 1,000.
  - `seq` is assigned by the writer at emission, so emitted lines always have consecutive `seq` values (SC-006).
  - When the loop must queue an event and the queue is full, it discards the queue, queues `resync_required` (`queue_overflow`), and stops producing events. When the writer has written `resync_required`, which means the client is reading again, the loop computes a new snapshot of the project as it is then, queues it, and resumes.
  - Memory is bounded by 1,000 events plus one snapshot (SC-005). The queue limit is a constant; tests set a smaller one through an unexported field.
- **A client that closes its end:** the write fails with `EPIPE`. `SIGPIPE` is turned into an error with the existing `brokenPipeAsError`. No event can be written any more, so `watch` exits 2 (an internal failure after start) without `watch_stopped`.
- **Alternatives considered:**
  - Non-blocking writes to standard output: they change the file status of a descriptor the client shares.
  - Discarding only the oldest events: the client could not know that its state is wrong.

## R-07: Signals and exit codes

- **Decision:**
  - `signal.NotifyContext` with `SIGINT` and `SIGTERM`. On cancellation, the loop queues `watch_stopped` (`signal`) and waits at most 2 s for the writer to write it, then exits 0. A client that is not reading does not keep `watch` alive.
  - The adapter's descriptors are closed on every exit path.
  - Exit codes follow F015-FR-018 (contracts/watch.md):
    - a usage error is reported on standard error, with the usage line, before any event;
    - a project directory that cannot be read at start gives `watch_stopped` with `watch_unavailable` and exit 1.
- **`--once`:** it computes and emits the snapshot without registering any watch, so it works on every platform, including those where watching is unavailable.

## R-08: Text format and quoting

- **Decision:** one line per event:

  ```text
  ai-dev: event=<type> seq=<n> time=<t> [feature=<q>] <key>=<value>...
  ```

  - Every string from the project (feature identifier, approver, reason, file name, project path) is passed through `report.ShellInertQuote`, the 004 FR-028 form.
  - The quoting function reaches `watch` as a function value from `internal/app`, so `watch` does not import `report`.
  - The shared vectors in `testdata/quote-vectors.json` are reused.

## R-09: Testing time without flakes

- **Decision:** two levels.
  - **The loop:** debounce, heartbeat, revalidation, snapshot ordering, overflow, signals and exit reasons are tested with `testing/synctest` (standard library since Go 1.25). Inside a bubble, `time.Sleep`, timers and tickers use a fake clock that advances only when every goroutine is blocked. With a fake watcher and a fake writer, these tests take no real time and cannot race the clock.
  - **The adapters and the built binary**, which use real kqueue or inotify, use real time with waits of up to 5 s for an expected event and a 1 s quiet period to assert that no event comes. SC-001 is measured by a test that makes 40 changes and checks the 95th percentile of their delivery latency against 1 s. The bound is four times the expected worst case, which leaves margin on shared CI runners.
  - Every timing test runs on `macos-latest` and `ubuntu-latest` in `gates`.

## R-10: Threats and controls

| Threat | Control |
|---|---|
| `watch` modifies state | The structural test (R-03); the app adapter receives only `fs.FS` and the executor; a no-write test over the project tree (SC-003) |
| An event read as an approval | No Human Gate outcome in any event (F015-FR-008); evidence only under R-05's rule; the documentation says that events inform |
| Hostile names in output | JSON encoding; shell-inert quoting in text (R-08); hostile-string vectors (SC-006) |
| Symbolic links leading outside the project | Never followed: `O_SYMLINK` and `IN_DONT_FOLLOW`, `Lstat` before every read, and 004's feature rule |
| Unbounded memory with a blocked client | The 1,000-event queue (R-06) |
| Descriptor or watch exhaustion | `watch_unavailable` with the limit and its remedy (R-01) |
| Content or secrets in events | Metadata only (F015-FR-021); setup and doctor JSON contain no secrets today; a test checks that no artifact byte appears in the output |
