# Implementation Plan: Project Event Watch

**Branch**: `feature/015-watch` | **Date**: 2026-11-26 | **Spec**: [spec.md](spec.md)

## Summary

- `ai-dev watch` is a read-only, long-running command that reports each change to the project as one line, NDJSON in `json` format ([contracts/watch.md](contracts/watch.md)).
- It observes the project with the operating system's change notifications, through the standard library only: kqueue on macOS, inotify on Linux (R-01). Measured at rest with 100 features: 0.0015% CPU and 5.2 MB. An internal scan was measured at 0.93% CPU and rejected.
- After each notification and a fixed debounce window, it recomputes the affected results with the commands' own functions. It embeds their JSON unchanged and emits an event only when the bytes differ (R-03).
- A state change carries the recorded Human Gate evidence only under F015-FR-008. That evidence is read by a new read-only reader, `gate.ReadHistory` (A3, R-05).
- A new package, `internal/watch`, holds the loop and depends on nothing that writes state or runs processes. `internal/app` supplies the results through a port.

## Technical Context

- **Language and dependencies:** Go 1.27.4, standard library only (`syscall` for kqueue and inotify; `testing/synctest` in tests). `go.mod` is unchanged.
- **Storage:** none. `watch` writes nothing (F015-FR-019).
- **Testing:**
  - **`internal/watch`, under `testing/synctest`**, with a fake watcher, a fake source and a fake writer (R-09):
    - snapshot first, and no lost change during it (R-02);
    - debounce: five saves in 200 ms give one event, and the window is fixed;
    - heartbeat and environment revalidation, including `0`;
    - the `--feature` filter, and an unknown named feature;
    - F015-FR-008 evidence: an applied gate; a refused request (no event); a manual edit (null); an unreadable history (null); several entries within one window;
    - the per-feature event order;
    - queue overflow, then `resync_required`, then `snapshot`, with consecutive `seq` values;
    - signal, `project_gone`, `watch_unavailable` and `internal_error`, each with its exit reason and the 2 s final-write bound;
    - text and JSON rendering, with the shared quote vectors and hostile strings (SC-006).
  - **`internal/gate`:** `ReadHistory` on missing, valid, duplicated, case-variant, non-array, malformed-entry, symbolic-link and non-regular records, plus a fuzz target `FuzzReadHistory` that must never panic.
  - **`internal/gatestore`:** the round trip of R-05.
  - **`internal/fswatch`** (real file system, in `t.TempDir()`): atomic replace, in-place write, directory creation and removal, `specs` and `.ai-dev` created late, the project renamed or removed, symbolic links not followed, re-synchronization after a replace. Also, on Linux, the `ENOSPC` mapping, through an injected `InotifyAddWatch`, and `IN_Q_OVERFLOW` handling, through an injected event. Waits are bounded at 5 s.
  - **`internal/app`:** options, usage and exit codes; that the source is built only from `Root` and `Executor`; and the snapshot against the four commands' JSON for every fixture state of 001, 004, 005 and 011 (SC-007).
  - **Structural:** `go list -deps ./internal/watch ./internal/fswatch` excludes `gatestore`, `configstore`, `gitstore`, `gitmanager`, `gitprocess`, `execution`, `report`, `system` and `os/exec` (R-03). A recording executor shows that the source runs only `git rev-parse --is-inside-work-tree` (F015-FR-020).
  - **End-to-end** (the built binary as a subprocess, with fake agents only):
    - SC-001: 40 state changes (Human Gate approvals and edits of `state`), with the 95th percentile of their latency under 1 s;
    - SC-002: Human Gates applied and refused;
    - SC-003: at least 50 changes, then the project tree, except the test's own changes, compared byte for byte, with 0 writes;
    - SC-005: a pipe left unread while 1,100 features change at once, then read;
    - `SIGINT` and `SIGTERM` (exit 0), the project renamed and deleted (exit 1), and `--once`;
    - no artifact byte in the output (F015-FR-021).
- **Target platforms:** macOS and Linux. Elsewhere `watch` reports `watch_unavailable` (exit 1); `--once` works everywhere (R-07). The cross-builds for windows and freebsd compile the unavailable adapter.
- **Project type:** CLI.
- **Performance:**
  - SC-001 (1 s at p95) is measured by the end-to-end test.
  - SC-004 (under 0.5% CPU and 30 MB) is measured manually by the owner (quickstart step 6). It was already met by the prototype (R-01).
- **Constraints:**
  - Existing commands, outputs and cockpit behavior are unchanged except A1–A3.
  - No process other than the doctor checks' `git rev-parse` is started.
  - No real coding agent runs in tests.

## Constitution Check

| Principle | Status |
|---|---|
| I. Specification First | PASS: spec approved by the owner on 2026-11-26, with Amendments A1–A3 applied (`a36ce35`) |
| II. Architecture Before Framework | PASS: no framework. The mechanism follows from measurement against SC-001 and SC-004 (R-01); the port and adapter split follows from F015-FR-019 (R-03) |
| III. Simplicity and YAGNI | PASS with justified complexity: two platform adapters (about 300 lines) instead of an internal scan, because the scan fails SC-004 by measurement (R-01). Simpler alternatives considered: the scan, fsnotify (a dependency) and FSEvents (cgo). There is no daemon, socket, cache file or configurable queue |
| IV. Testable by Design | PASS: a deterministic loop under `testing/synctest`; real-file-system adapter tests on both platforms; end-to-end with the built binary and fake agents; a fuzz target for the new reader |
| V. Security by Design | PASS: threats and controls in R-10. Read-only is enforced by a structural test and a no-write test; symbolic links are never followed; output is encoded and quoted; memory is bounded. No new dependency |
| VI. Observability by Design | PASS: the command is itself an observation channel. `watch_unavailable` names the limit and its remedy; heartbeats show liveness; the text format is meant for debugging |
| VII. Reproducibility | PASS: everything runs with `go test`; no new CI tool; the Linux adapter runs in the existing `gates` job on ext4 |
| VIII. AI-Assisted Engineering | PASS: independent review before the PR, as in 012–014 |
| IX. Human Authority | PASS: no event carries a Human Gate outcome; evidence is shown as recorded and only under F015-FR-008; `watch` cannot write state (structural test) |

Post-design re-check: PASS. The only changes to existing behavior are `watch` in the catalog, help, usage and completion (A1, A2), and the new read-only `gate.ReadHistory` (A3), which no existing command calls.

## Design

- **`internal/gate/history.go`** (new): `ReadHistory(root fs.FS, featureID string) ([]Evidence, bool)` (R-05), with its own copy of the wire structure and the strict member rules of `workflow.ResolveRecord`.
- **`internal/fswatch`** (new; the adapter):
  - `fswatch.go`: the `Watcher` interface:
    - `Sync(dirs, files []string) error`;
    - `Events() <-chan Change`, where a `Change` is `{Path string, Kind: Changed|Gone|Overflow}`;
    - `Close() error`.
    - `Unavailable` errors carry a reason, a detail and a remedy.
  - `kqueue_darwin.go`: one `O_EVTONLY|O_SYMLINK|O_CLOEXEC` descriptor per path; `EV_ADD|EV_CLEAR` with `NOTE_WRITE|NOTE_EXTEND|NOTE_ATTRIB|NOTE_DELETE|NOTE_RENAME`; a reader goroutine; and reopening on inode change in `Sync`.
  - `inotify_linux.go`: directories only, with the mask of R-01; an `ENOSPC` or `EMFILE` limit becomes `Unavailable`; `IN_Q_OVERFLOW` becomes `Overflow`.
  - `unavailable_other.go`: `New` returns `Unavailable` with `platform_unsupported`.
- **`internal/watch`** (new; no I/O except reads through `fs.FS`):
  - `source.go`: the port `Source`:
    - `Features() ([]string, error)`;
    - `State(id) (json []byte, state string, outcome string)`;
    - `Next(id) []byte`;
    - `Setup() []byte`;
    - `Environment(ctx) (json []byte, statuses map[string]string, remediation map[string]string)`.
  - `picture.go`: the Project Picture, the Observed Feature with its artifacts, and the history length (data-model.md).
  - `diff.go`: a pure function from (previous, current) to events, in the contract's order.
  - `loop.go`: `Run(ctx, Options, Source, Watcher, Output) Result`: registration, snapshot, change keys and windows, tickers, overflow pause and resume, and stop reasons.
  - `queue.go`: the bounded queue and the writer goroutine, which assigns `seq` and does one `Write` per line.
  - `event.go`: the envelope and the data types of the contract, plus the text renderer, which takes the quote function as a value.
- **`internal/app/watch.go`** (new): `RunWatch(ctx, args, deps)`.
  - It parses and validates options (contracts/watch.md); on a usage error it prints the usage line and exits 2.
  - It builds the `Source` from `deps.Root` and `deps.Executor`, with `workflow.EvaluateState`, `sdd.Evaluate`, `setupShowReport`, `RunDoctor` and the `report` JSON writers.
  - It resolves the project's absolute path and identity.
  - It installs `signal.NotifyContext(SIGINT, SIGTERM)` and `brokenPipeAsError`, and creates the `fswatch` adapter (not for `--once`).
  - It runs the loop and maps the result to the exit code.
- **`internal/app/{app,catalog,help,completion}.go`:** route `watch`; add the catalog entry with help and completion of its options (`--format` values; `--feature` as free text, with no candidates); and add `watch` to the top-level usage and the general help (A1, A2).
- **Documentation:**
  - README and README.es: the command, the event contract with an example, the compatibility rules, how a client detects support, the recovery procedure, and the platform limits with their remedies.
  - CHANGELOG and CHANGELOG.es, under `[Unreleased]`.
  - SECURITY: the read-only boundary, and that events inform but never approve.

## Owner decisions on the plan (2026-11-26)

German Munoz, repository owner, confirmed these five interpretations on 2026-11-26 ("si, confirmo las cinco, haz el commit y ejecuta /speckit-tasks"). This is a declared, unauthenticated assertion.

1. **Scope of the structural test (decision 3).** It applies to `internal/watch` and `internal/fswatch`. `internal/app/watch.go` cannot be isolated by imports, because the `app` package links every command, so a test checks instead that it builds the source from `Root` and `Executor` only (R-03).
2. **`seq` at emission.** Discarded events consume no `seq`, so emitted lines are always consecutive (SC-006), and loss is signalled by `resync_required`, not by a gap (R-06).
3. **`--once` works on every platform**, because it registers no watch (R-07).
4. **A closed standard output** exits 2 with no event. On a signal, `watch` waits at most 2 s to write `watch_stopped`, then exits 0 anyway (R-06, R-07).
5. **A fixed debounce window** from the first notification, not one that restarts on every notification (R-04).

After `/speckit-analyze`, the owner approved the ten proposed fixes on 2026-11-26 ("si, aplica los diez cambios"), with the same caveat. The fixes are:

- **I1:** `--once` works on every platform, now stated in the spec (F015-FR-014, F015-FR-016).
- **U5:** no evidence when the previous observation could not read the history (F015-FR-008, R-05).
- **U1:** the queue overflow is tested end to end with 1,100 features changing at once, with no test-only option, variable or build tag (R-06).
- **U2:** `--feature` completes no candidates, like the other feature-identifier arguments.
- **C1:** a recording executor proves F015-FR-020.
- **A1:** SC-001 measures state changes only.
- **I2:** `seq` counts emitted events.
- **I3:** an unwritable standard output exits 2 (F015-FR-018).
- **I4:** `internal/system` is excluded from `internal/watch` (R-03).
- **U3:** the project's deletion is tested end to end.

## Project Structure

```text
specs/015-watch/
  plan.md, research.md, data-model.md, quickstart.md, contracts/watch.md
internal/gate/history.go, history_test.go, history_fuzz_test.go                      NEW
internal/gatestore/history_roundtrip_test.go                                          NEW
internal/fswatch/{fswatch,kqueue_darwin,inotify_linux,unavailable_other}.go, *_test.go NEW
internal/watch/{source,picture,diff,loop,queue,event}.go, *_test.go, deps_test.go     NEW
internal/app/watch.go, watch_test.go                                                  NEW
internal/app/{app,catalog,help,completion}.go                 MODIFIED (route, catalog, usage, help)
cmd/ai-dev/watch_e2e_test.go                                  NEW (built binary, SC-001/002/003/005)
.github/workflows/ci.yml                                      MODIFIED (FuzzReadHistory in the fuzz step)
tests pinning usage, general help, catalog, completion         UPDATED (A1, A2)
README.md, README.es.md, CHANGELOG.md, CHANGELOG.es.md, SECURITY.md   UPDATED
```

## Quality Gates

| Gate | How | Pass condition |
|---|---|---|
| Format | `gofmt -l .` | no output |
| Vet | `go vet ./...` and `GOOS=linux go vet ./...` | exit 0 |
| Tests | `go test -race ./...` in `gates` on `macos-latest` and `ubuntu-latest` | exit 0 |
| Fuzz | `go test -run '^$' -fuzz '^FuzzReadHistory$' -fuzztime 20s ./internal/gate` in CI's fuzz step | no failure |
| Static analysis | `staticcheck ./...` for linux and darwin (CI `static-analysis`) | exit 0 |
| Vulnerabilities | `govulncheck ./...` (CI `static-analysis`) | no reachable vulnerability |
| Dependencies | `go.mod` and `go.sum` unchanged | pass |
| Build | the existing cross-builds (darwin, linux, windows, freebsd), and `CGO_ENABLED=0 go build ./cmd/ai-dev` for darwin and linux as `release.yml` does | exit 0 |
| Read-only | the structural test and the SC-003 no-write test | pass |
| Performance | SC-001 in the end-to-end test; SC-004 manual by the owner | 1 s at p95; under 0.5% CPU and 30 MB, recorded in tasks.md |
| Regression | suites of Features 001–014 | pass with only the A1–A3 expectations updated (SC-008) |
