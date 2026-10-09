---

description: "Task list for 006-human-gates implementation"
---

# Tasks: Human Gates

**Input**: Design documents from `/specs/006-human-gates/`

**Prerequisites**:
- plan.md
- spec.md
- research.md
- data-model.md
- quickstart.md
- contracts/workflow-gate.md
- contracts/human-gate-record.md
- contracts/sdd-missingartifacts.md
- checklists/requirements.md
- `.specify/memory/constitution.md`

This task list was generated only after the human stated, in the controlling session, explicit
approval of the Human Specification Gate (including clarifications C-1–C-13, D-1–D-3, and P-6) and
of the Human Plan Gate (P-1 remediated, P-2–P-6). This file is not an approval record, and it does
not approve any Human Gate, including the Human Task Gate.

**Tests**: Included. The plan's Project Structure and Test Strategy name every test file below as
a required deliverable, and constitution principle IV applies. Contract-establishing tests come
before the production code they constrain, and are expected to fail or not compile until that
code lands. Story-phase tests are integration and verification tests over the foundational
implementation.

**Organization**: Spec user stories:
- US1 (P1) accept and advance;
- US2 (P1) no automated signal approves;
- US3 (P2) refusals;
- US4 (P2) stale state, persistence failure, crash and concurrency;
- US5 (P3) abandonment.

The layers are:
- `internal/sdd` (one approved wrapper);
- `internal/gate` (domain and control flow, no writes);
- `internal/gatestore` (the sole Workflow State writer);
- `internal/report`;
- `internal/app`;
- `cmd/ai-dev`.

## Binding decisions carried from spec and plan (do not re-decide during implementation)

- **Commands**:
  ```text
  ai-dev workflow approve [--format text|json] --approver <identity> [--reason <text>] <feature-id> <target-state>
  ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>
  ```
  - `--approver` is required and may be set only once.
  - `--reason` may be set at most once. It is required for `abandon`. When supplied it must be
    non-empty and non-whitespace (D-1).
  - No stdin, no prompt, no 007 behavior.
- **Precedence** (spec FR-026, C-13):
  1. usage validation;
  2. decision-type consistency;
  3. target canonicality;
  4. feature recognition;
  5. current-state resolution;
  6. 004 transition validation;
  7. 005 target-artifact check (`approval` only);
  8. stale-state check, with lock and binding (FR-037 failures here);
  9. all-or-nothing persistence;
  10. report.

  Steps 1–2 perform zero filesystem access and zero store calls. Steps 3–6 are exactly one call to
  `workflow.EvaluateTransition`. Step 7 is exactly `sdd.MissingArtifacts(root, id, target)`.
- **C-13**:
  - `abandon` with a target other than exactly `abandoned` (including `Abandoned`) is a usage
    failure (exit 2).
  - `approve` with exactly `abandoned` is a usage failure (exit 2).
  - Any other `approve` target proceeds to canonicality, so `Abandoned` and `reviewing` give
    `target_unsupported` (exit 1).
- **Outcomes**: exactly `transition_applied`, `feature_unknown`, `state_invalid`,
  `target_unsupported`, `transition_invalid`, `artifacts_missing`, `state_conflict`, and
  `persistence_failed`. The four 004 tokens are defined as `Outcome(workflow.X)`.
- **Exit codes**:
  - 0 only when this invocation committed (`transition_applied`);
  - 1 for every other outcome;
  - 2 for usage and output failures.

  An output failure after a commit exits 2 and is never rolled back.
- **Record and evidence**: `specs/<id>/workflow-state.json` holds the top-level `state` plus an
  append-only `human_gates` array. Each element has exactly the keys `feature_id`, `decision`,
  `approver`, `from_state`, `to_state`, `decided_at` (RFC 3339, UTC, seconds), and `reason`
  (present iff supplied).
  - Unknown top-level members are preserved verbatim and in position (P-4).
  - A `human_gates` member that is not an array, is duplicated, or has an `EqualFold` variant gives
    `persistence_failed` (D-2).
- **Commit protocol** (P-1 as approved; contracts/human-gate-record.md), macOS/Linux only:
  1. `os.OpenRoot(repo)`.
  2. `root.Open(dir)`.
  3. `flock(LOCK_EX|LOCK_NB)`:
     - `EWOULDBLOCK` or `EAGAIN` (held) gives `Conflict`;
     - any other error (ambiguous) gives `Failed`;
     - one call, never retried.
  4. Bind: no-follow `Lstat(dir)` must be a real directory and `os.SameFile` must hold, otherwise
     `Conflict`.
  5. Re-validate with `workflow.FeatureExists` and `workflow.ResolveRecord` on `root.FS()`:
     missing, invalid, or a state ≠ expected gives `Conflict`.
  6. Read the raw record.
  7. Build.
  8. Write `.workflow-state.<16 hex>.tmp` with `O_EXCL`, then write, `Sync`, and `Close`.
  9. `Rename`: **the commit point**.
  10. Post-commit: `dirFile.Sync()` (warning only).
  11. Deferred: `dirFile.Close()` (warning only).

  There is **no lock file**. ai-dev never removes, breaks, or retries a lock.
- **Platform** (P-6, spec FR-037/SC-014): persistence is supported only on macOS and Linux local
  filesystems. The POSIX store uses `(darwin && !ios) || (linux && !android)` and its fallback uses
  `!((darwin && !ios) || (linux && !android))`. Every other platform—including Windows, iOS, and
  Android—compiles an unsupported-platform store that returns `Failed` with the warning
  `ai-dev: warning: human gate persistence is supported only on macOS and Linux` and performs zero
  filesystem access. Persistence support outside macOS/Linux is never claimed.
- **Trust boundary** (C-12): the approver identity is a verbatim, auditable assertion. Text reports
  always say "approver identity is not verified". No report or evidence claims authentication. The
  approver is never taken from the environment, git, or the OS.
- **Wiring** (P-2): `app.Run` keeps its signature and delegates to `app.RunWithGateStore(...)` with a
  `nil` store. A `nil` store gives an internal error (exit 2) for `approve` and `abandon`, with zero
  filesystem access. `main` passes `gatestore.New(".")`. The existing line
  `Usage: ai-dev workflow <state|transition> [options]` is unchanged.
- **005 reuse** (P-5): add only `func MissingArtifacts(root fs.FS, featureID string, state
  workflow.State) []string { return missingArtifacts(root, featureID, state) }`. Never call
  `sdd.Evaluate`.
- **`internal/workflow` is not modified.** No existing test file is modified. `go.mod` and `go.sum`
  are unchanged.
- **Selector allow-list reconciliation** (internal detail, no product effect): `gate.Report.Valid()`
  uses `workflow.FeatureDir` and `workflow.Recognized`, as data-model.md requires. The
  `internal/gate` boundary test therefore allows exactly these `workflow` selectors:
  - `EvaluateTransition`, `FeatureDir`, `Recognized`;
  - `State`, `Outcome`;
  - the seven state constants;
  - the outcomes `FeatureUnknown`, `StateInvalid`, `TargetUnsupported`, `TransitionInvalid`, and
    `TransitionValid`.

  It forbids `Allowed`, `FeatureExists`, `ResolveRecord`, `EvaluateState`, and `Terminal` in
  `internal/gate`. The only exception is that these remain allowed in `internal/gatestore` for the
  under-lock re-validation. This reconciles research §12's shorter list with data-model.md's
  `Valid()` table.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel. It is used only when the task writes a different file from every
  other task it could run alongside, and has no dependency on an incomplete task.
- **[Story]**: US1–US5. It is omitted for Setup, Foundational, and Polish tasks.
- Every task names its exact file path or command scope.

## Path Conventions

Single Go module: `cmd/ai-dev/` plus `internal/<domain>/`. Two new packages are added:
`internal/gate/` and `internal/gatestore/`. No other new directory is added.

---

## Phase 1: Setup

**Purpose**: Confirm a clean baseline and the toolchain primitives that the approved plan relies on.
No file changes in this phase.

- [X] T001 Confirm the baseline on branch `feature/006-human-gates`, without changing any file:
  - `go.mod` declares `go 1.27.1`;
  - `gofmt -l .` prints nothing;
  - `go vet ./...` succeeds;
  - `go test -count=1 ./...` passes;
  - `git status --porcelain` shows only `specs/006-human-gates/`.
- [X] T002 Confirm the primitives, without changing any file:
  - `go doc os.Root.Rename`, `go doc os.Root.FS`, and `go doc os.Root.OpenFile` resolve;
  - `go doc syscall.Flock` resolves, and `GOOS=linux go doc syscall.Flock` resolves;
  - `GOOS=windows go doc syscall.Flock` reports no such symbol, which confirms that unsupported
    platforms require a separately selected fallback store;
  - `.gitignore` contains `*.tmp`.

  Depends on T001.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The complete shared implementation:
- the 005 wrapper;
- the `gate` domain and the step 3–9 control flow;
- the record builder;
- the unsupported-platform and POSIX stores;
- the report writer;
- the CLI steps 1–2;
- the dispatcher and `main` wiring;
- the boundary tests.

Every user story verifies behavior on top of this phase.

**⚠️ CRITICAL**: No user-story task may begin until this phase is complete.

### 005 extension: `sdd.MissingArtifacts` (contracts/sdd-missingartifacts.md, P-5)

- [X] T003 [P] Write `internal/sdd/missingartifacts_test.go`.
  - **Reuse** the existing package test helpers `sddFixture`, `recordingSDDFS`, and
    `faultReadDirFS` (from `internal/sdd/rules_test.go`) and `caseInsensitiveFS` (from
    `internal/sdd/casefs_test.go`). Declare no equivalent type.
  - **Equivalence table:** for each of the seven canonical states, over every directory fixture
    `rules_test.go` uses (all present, each subset missing, a case variant `Plan.md`, a directory
    named `plan.md`, a symlink named `plan.md`, a `ReadDir` fault, a structurally invalid ID),
    `MissingArtifacts(...)` equals `missingArtifacts(...)`.
  - **Calls:** through `recordingSDDFS`, a call with a state that has requirements performs exactly
    one `ReadDir` on `workflow.FeatureDir(id)` and no `Open`, `Lstat`, or `ReadLink`. `draft` and
    `abandoned` return `nil` with zero calls.
  - The test does not compile until T004.
- [X] T004 Add exactly
  `func MissingArtifacts(root fs.FS, featureID string, state workflow.State) []string { return missingArtifacts(root, featureID, state) }`,
  with a doc comment, to `internal/sdd/rules.go`.
  - Add no import, and change nothing else.
  - Run `go test -count=1 ./internal/sdd/`: every existing test, including `TestPackageBoundary`,
    passes unchanged, and T003 passes.
  - Depends on T003.

### `internal/gate` domain model (data-model.md)

- [X] T005 [P] Write `internal/gate/model_test.go` (package `gate`).
  - **Tokens:**
    - `FeatureUnknown`, `StateInvalid`, `TargetUnsupported`, and `TransitionInvalid` equal
      `Outcome(workflow.FeatureUnknown)` and so on, byte for byte;
    - `TransitionApplied` is `"transition_applied"`, `ArtifactsMissing` is `"artifacts_missing"`,
      `StateConflict` is `"state_conflict"`, and `PersistenceFailed` is `"persistence_failed"`;
    - `Approval` is `"approval"` and `Abandonment` is `"abandonment"`;
    - `CommitStatus` values: `Committed` is `1`, `Conflict` is `2`, and `Failed` is `3`.
      "The zero value `CommitStatus(0)` is invalid."
  - **`Report.Valid()`:** a table covering every row of data-model.md's `Report.Valid()` invariants,
    with positive and negative cases:
    - whitespace `FeatureID`, `Decision`, or `Approver` is invalid for every outcome;
    - `transition_applied` with `SuppliedTarget != string(Target)` is invalid;
    - `transition_applied` for `Abandonment` with `Target != abandoned`, or with no reason, is
      invalid;
    - `transition_applied` for `Approval` with `Target == abandoned` is invalid;
    - `Reason` set on any non-applied outcome is invalid;
    - `artifacts_missing` with an empty `Missing`, or with `Abandonment`, is invalid;
    - `target_unsupported` with a canonical supplied target, a non-empty `FromState` or `Target`,
      or `Abandonment` is invalid;
    - `feature_unknown` with any optional field set is invalid;
    - `state_invalid` with a structurally invalid ID is invalid;
    - an unknown outcome is invalid.
  - The test does not compile until T006.
- [X] T006 Create the domain files, exactly per data-model.md.
  - `internal/gate/decision.go`: `Decision` with `Approval` and `Abandonment`, and `Request` with
    `FeatureID`, `Decision`, `SuppliedTarget`, `Approver`, `Reason`, and `ReasonSet`.
  - `internal/gate/outcome.go`: `Outcome`, with the eight constants and the four reused ones defined
    as `Outcome(workflow.X)`, and `Report` with `FeatureID`, `Decision`, `Approver`, `FromState`,
    `SuppliedTarget`, `Target`, `Reason`, `ReasonSet`, `Missing`, and `Outcome`.
  - `internal/gate/evidence.go`: `Evidence` with `FeatureID`, `Decision`, `Approver`, `FromState`,
    `ToState`, `DecidedAt time.Time`, `Reason`, and `ReasonSet`.
  - `internal/gate/store.go`: `CommitStatus` (`Committed = iota + 1`, `Conflict`, `Failed`) and
    `Store` with `Commit(featureID string, expected workflow.State, entry Evidence) (CommitStatus, []string)`.
  - `internal/gate/validate.go`: `Report.Valid()`.

  Imports are limited to `io/fs` (if needed), `strings`, `time`, and `ai-dev/internal/workflow`.
  T005 must pass. Depends on T005.
- [X] T007 [P] Write `internal/gate/boundary_test.go`, an AST test in the style of
  `internal/sdd/boundary_test.go`. Over every non-test `.go` file in `internal/gate`:
  - **Imports:** exactly `io/fs`, `strings`, `time`, `ai-dev/internal/workflow`, and
    `ai-dev/internal/sdd`. Forbidden: `os`, `syscall`, `os/exec`, `net`, and any of
    `ai-dev/internal/{agent,execution,selection,role,system,gatestore,report,app}`.
  - **`sdd` selectors:** only `MissingArtifacts`.
  - **`workflow` selectors:** only the reconciled allow-list in Binding decisions.
  - **Statements:** no `go` statement.
  - **Self-check:** include known-bad snippets (`sdd.Evaluate`, `workflow.Allowed`, `import "os"`,
    a `go` statement, `workflow.ResolveRecord`) that must be rejected, and one known-good snippet
    that must be accepted.

  Depends on T006.
- [X] T008 [P] Write `internal/gate/evaluate_test.go` for
  `Evaluate(root fs.FS, req Request, store Store, now func() time.Time) (Report, []string)`.
  - **Helpers declared in this file** (the `workflow` test helpers are in another package):
    - `recordingGateFS`, which wraps an `fstest.MapFS`, implements and forwards `fs.ReadLinkFS`
      and `fs.ReadDirFS`, and records ordered `Open:`, `Lstat:`, `ReadLink:`, and `ReadDir:`
      calls;
    - `fakeStore`, which records each `Commit` call and returns scripted
      `(CommitStatus, []string)` values.
  - **Steps 3–6:** for each 004 outcome:
    - `target_unsupported` (`Planned`, `reviewing`, with the feature absent) makes **zero**
      filesystem calls;
    - `feature_unknown` covers `..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`, and a symlinked
      directory;
    - `state_invalid` covers the 004 invalid-record kinds;
    - `transition_invalid` covers skip, backward, same-state, `approved` origin, and `abandoned`
      origin.

    Each reports the identical token, makes **zero** store calls, and gives a report that is
    `Valid()`.
  - **Step 7:**
    - `approval` with a missing target artifact gives `artifacts_missing`, with `Missing` in 005
      order and zero store calls;
    - `MissingArtifacts` is reached only after `transition_valid` and is called with the
      **target** state; the recorded `ReadDir` happens after the 004 calls;
    - a request that is both 004-invalid and missing artifacts gives `transition_invalid` with
      zero `ReadDir` calls (SC-004);
    - `abandonment` makes zero `ReadDir` calls in every case.
  - **Steps 8–9:**
    - the store is called exactly once, with `expected` equal to the 004 current state and an
      `Evidence` whose fields equal the request (approver verbatim; reason iff `ReasonSet`;
      `FromState` current; `ToState` target; `DecidedAt` equal to the injected clock's `.UTC()`);
    - `Committed` gives `transition_applied`;
    - `Conflict` gives `state_conflict`;
    - `Failed` gives `persistence_failed`;
    - `CommitStatus(0)` gives `persistence_failed`;
    - warnings are returned unchanged.
  - **Determinism:** identical inputs and clock give identical reports.
  - **Never authorization:** `transition_valid` never appears as an outcome.

  Depends on T006.
- [X] T009 Implement `internal/gate/evaluate.go`: `Evaluate` exactly per data-model.md's
  evaluation flow.
  - Call `workflow.EvaluateTransition(root, req.FeatureID, req.SuppliedTarget)` once, and map
    non-valid outcomes to the same token.
  - For `Approval` only, call `sdd.MissingArtifacts(root, req.FeatureID, tr.Target)`.
  - Take `now().UTC()` once, after step 7.
  - Call `store.Commit(req.FeatureID, tr.CurrentState, entry)`, and map the status.

  Steps 1–2 are not performed here. T005, T007, and T008 must pass. Depends on T004 and T008.

### `internal/gatestore` (contracts/human-gate-record.md, research §4–§7, §13)

- [X] T010 [P] Write `internal/gatestore/record_test.go` for the pure builder
  `buildRecord(existing []byte, exists bool, to workflow.State, entry gate.Evidence) ([]byte, error)`.
  - **Build-rules table**, every row:
    - absent gives exactly `{"state": to, "human_gates": [entry]}`;
    - present with no `human_gates` keeps the members in order, replaces `state`, and appends
      `"human_gates": [entry]` at the end;
    - present with a `human_gates` array appends the entry, with earlier elements byte-equal after
      `json.Compact`;
    - `human_gates` as an object, string, number, or `null` gives an error;
    - a duplicated `human_gates` gives an error;
    - `Human_Gates` or `HUMAN_GATES` gives an error.
  - **Preservation:** unknown members such as `"note"` and `"x":{"state":"nested"}` are kept
    verbatim and in position (P-4).
  - **Entry encoding:**
    - the keys are exactly `feature_id`, `decision`, `approver`, `from_state`, `to_state`,
      `decided_at`, and `reason`, in that order;
    - `reason` is absent when `ReasonSet` is false;
    - `decided_at` uses `time.RFC3339` in UTC with second precision;
    - an approver containing a newline, quotes, or Unicode round-trips verbatim.
  - **Output format:** two-space indentation plus a trailing newline.
  - **Property test** over all 004-permitted transitions, with randomized sequences of 1–10 appends:
    - `workflow.ResolveRecord` over `fstest.MapFS{"specs/f": dir, "specs/f/workflow-state.json": out}`
      returns `(to, false)`;
    - the element count increases by exactly 1;
    - every earlier element is byte-equal after compaction.

  Depends on T006.
- [X] T011 Implement `internal/gatestore/record.go`, the pure builder per research §7.
  - Walk the top-level members with `json.Decoder`, keeping each member's raw value.
  - Replace the single exact `state` member, and append to `human_gates` or add it.
  - Return an error for every D-2 refusal.
  - Re-encode, then apply `json.Indent` with two spaces plus `"\n"`.

  No I/O. T010 must pass. Depends on T010.
- [X] T012 [P] Write `internal/gatestore/unsupported_test.go` (untagged, so it runs on every host).
  - `unsupportedStore{}.Commit("f", workflow.Draft, entry)` returns `gate.Failed` and exactly the
    warnings
    `[]string{"ai-dev: warning: human gate persistence is supported only on macOS and Linux"}`.
  - Over a `t.TempDir()` repository containing `specs/f/`, a `snapshotDir`-style before and after
    comparison (declare a local helper; the `app` helper is in another package) shows zero changes.
  - The type holds no directory and performs no filesystem call (FR-037, SC-014).

  Depends on T006.
- [X] T013 Create `internal/gatestore/unsupported.go` (untagged): the `unsupportedStore` type, plus
  the exported-to-package constant for the warning text. Also create
  `internal/gatestore/new_other.go` with
  `//go:build !((darwin && !ios) || (linux && !android))`:
  `func newPlatformStore(dir string) gate.Store { return unsupportedStore{} }`. T012 must pass on
  this host. Depends on T012.
- [X] T014 [P] Write `internal/gatestore/store_posix_test.go` with
  `//go:build (darwin && !ios) || (linux && !android)`. All
  cases run on a real `t.TempDir()` repository through `New(dir)`:
  - first commit with no record gives `Committed`, and
    `workflow.EvaluateState(os.DirFS(dir), "f")` reports the target;
  - a second commit appends, and the first element is byte-equal;
  - unknown top-level members are preserved;
  - `expected` ≠ the on-disk state gives `Conflict`, with the record byte-identical;
  - a record made invalid gives `Conflict`;
  - a symlinked record gives `Conflict`;
  - a malformed `human_gates` gives `Failed`, with the record byte-identical;
  - a read-only feature directory (chmod `0555`; skipped when `os.Geteuid() == 0`) gives `Failed`,
    with the record byte-identical;
  - no `*.tmp` remains after any status;
  - **a pre-existing regular file named `.workflow-state.lock` is ignored, and the commit gives
    `Committed`**;
  - the record's permission bits are preserved across the replace;
  - the only paths that differ after a commit are `specs/f/workflow-state.json`.

  Depends on T011 and T013.
- [X] T015 Create `internal/gatestore/ops.go` (untagged): the unexported `ops` struct of function
  fields that the commit uses, as the test seam. It contains:
  - `openRoot`, `openDir`, `lstat`, `flock`, `readFile`, `openTmp`, `rename`, `remove`, `syncDir`,
    and `closeDir`;
  - `afterStep func(step string)`, a no-op by default, which tests use as a pause hook.

  The defaults are assigned in T017. There is no `syscall` import here. Depends on T013.
- [X] T016 Create `internal/gatestore/lock_posix.go` with
  `//go:build (darwin && !ios) || (linux && !android)`. It is the
  **only** file in the module (outside tests) that imports `syscall`. `flockDir(f *os.File) error`
  calls `syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)` exactly once, and
  `classifyLock(err) lockResult` maps:
  - `nil` → `acquired`;
  - `syscall.EWOULDBLOCK` or `syscall.EAGAIN` → `held`;
  - anything else, including `EINTR`, `ENOLCK`, `EOPNOTSUPP`, `ENOTSUP`, `EINVAL`, and `EBADF` →
    `ambiguous`.

  There is no retry or loop. Depends on T015.
- [X] T017 Create `internal/gatestore/store.go` (untagged): the package doc, the fixed names, and
  `func New(dir string) gate.Store { return newPlatformStore(dir) }`, with **no I/O in `New`**. Also
  create `internal/gatestore/commit_posix.go` with
  `//go:build (darwin && !ios) || (linux && !android)`: `newPlatformStore`, `posixStore`,
  `defaultOps()`, and `Commit`, implementing the contract's commit protocol table **exactly**:
  1. **Open:** `OpenRoot`, then `Open(dir)`. An error gives `Failed`.
  2. **Lock:** `flockDir`.
     - `held` gives `Conflict`;
     - `ambiguous` gives `Failed`;
     - nothing is created or removed.
  3. **Bind:** `Lstat(dir)` must be a directory and not a symlink, and `os.SameFile` must hold.
     Otherwise `Conflict`.
  4. **Re-validate:** `workflow.FeatureExists` and `workflow.ResolveRecord` on `root.FS()`. Missing,
     invalid, or ≠ expected gives `Conflict`.
  5. **Read:** read the raw record; `ErrNotExist` means absent; take the permission bits from
     `Lstat`. A read error gives `Failed`.
  6. **Build:** `buildRecord`. An error gives `Failed`.
  7. **Write the tmp file:** `OpenFile(dir/.workflow-state.<16 hex from crypto/rand>.tmp,
     O_WRONLY|O_CREATE|O_EXCL, perm)`, then write, `Sync`, and `Close`. An error gives `Failed`,
     and the tmp file is removed best-effort.
  8. **Commit point:** `Rename(tmp, dir/workflow-state.json)`. An error gives `Failed`, and the tmp
     file is removed best-effort.
  9. **Post-commit:** `dirFile.Sync()`. An error is the warning
     `ai-dev: warning: committed, but directory flush failed: <err>`.
  10. **Deferred:** `dirFile.Close()`. An error is a warning (`ai-dev: warning: … close …`).

  `ops.afterStep` is called after the lock, after the tmp write, after the rename, and after the
  directory sync, with the step names `locked`, `tmp-written`, `renamed`, and `dir-synced`. Every
  operation goes through `*os.Root`. There are no goroutines. T014 must pass on this host.
  Depends on T011, T014, T015, and T016.
- [X] T018 [P] Write `internal/gatestore/boundary_test.go` (an AST test).
  - **Over `internal/gatestore` non-test files:**
    - imports are exactly the research §12 allow-list: `bytes`, `crypto/rand`, `encoding/hex`,
      `encoding/json`, `errors`, `fmt`, `io/fs`, `os`, `path`, `strings`, `time`,
      `ai-dev/internal/gate`, and `ai-dev/internal/workflow`, plus `syscall` **only** in
      `lock_posix.go`, whose build constraint must be
      `(darwin && !ios) || (linux && !android)`;
    - no package-level `os.Create`, `os.WriteFile`, `os.OpenFile`, `os.Rename`, `os.Remove`,
      `os.Mkdir`, or `os.Chmod` (writes go only through `*os.Root` methods);
    - no `os.Getenv`, `os.Environ`, `os/exec`, or `net`, and no `go` statement.
  - **Repository-wide over non-test files, enforcing the sole write path (FR-003):**
    - `ai-dev/internal/gatestore` is imported only by `cmd/ai-dev/main.go`;
    - `ai-dev/internal/gate` is imported only by `internal/gatestore`, `internal/report`, and, in
      `internal/app`, only by `workflowgate.go` and `doctor.go`;
    - no file in `internal/{workflow,sdd,role,selection,execution,agent,checks,doctor,instruction,system}`
      imports either package;
    - no non-test file outside `internal/gatestore` contains a write-capable call (`WriteFile`,
      `OpenFile` with write flags, `Rename`, or `Create`) in combination with
      `RecordFileName` or `"workflow-state.json"`.
  - **Platform store selection (P-6, FR-037, SC-014), parsed with `go/build/constraint`:**
    - `new_other.go`'s build constraint is exactly
      `!((darwin && !ios) || (linux && !android))`, and its
      `newPlatformStore` body is exactly one `return unsupportedStore{}`;
    - `newPlatformStore` is declared in exactly two non-test files: `new_other.go` and
      `commit_posix.go`, and `commit_posix.go`'s build constraint is exactly
      `(darwin && !ios) || (linux && !android)`;
    - the two constraints are complementary: for each GOOS in a fixed list (`darwin`, `linux`,
      `ios` (also satisfies `darwin`), `android` (also satisfies `linux`), `windows`, `freebsd`,
      `openbsd`, `netbsd`, `dragonfly`, `solaris`, `illumos` (also satisfies `solaris`), `aix`, `plan9`, `js`, `wasip1`),
      exactly one of the two files is selected; `commit_posix.go` is selected only for macOS and
      Linux, while `new_other.go` is selected for iOS, Android, Windows, and every other GOOS;
    - every non-test file that references `posixStore`, `defaultOps`, `flockDir`, `classifyLock`,
      or `syscall` has the constraint exactly `(darwin && !ios) || (linux && !android)`.
      `new_other.go` and the untagged files (`store.go`, `ops.go`, `unsupported.go`, `record.go`)
      reference none of them;
    - no non-test file in `internal/gatestore` uses a GOOS or GOARCH file-name suffix, so selection
      is decided only by these `//go:build` lines.

    This is structural evidence of the selection. It does not execute the unsupported store on a
    foreign OS; T012 covers its semantics, T035 the app mapping, and T049 the Windows cross-build.
  - **Self-check:** known-bad and known-good snippets, including a `new_other.go` variant that
    returns a `posixStore` and a variant with a non-complementary constraint.

  Every test function in this file is named `TestBoundary…`, so T044's `-run Boundary` runs it.
  Depends on T017.

  **Post-completion platform-audit amendment D-1 (2026-10-08; human decision / human-approved scope
  amendment):** The human expressly authorized D-1, macOS and Linux as the only currently supported
  persistence platforms, H-7 as `PENDING — DEFERRED` and non-blocking for that scope, and
  preservation of FR-037/SC-014's complete fail-closed guarantees. T013, T014, T016, T017, and T018
  remain `[X]`. Their normative build constraints are amended above because Go's iOS target also
  satisfies `darwin` and Android also satisfies `linux`. The corrected expressions are exact
  complements, restrict the POSIX store to macOS/Linux, and route iOS, Android, Windows, and every
  other unsupported platform to the fail-closed fallback. The boundary test rejects the former
  broad expressions and verifies effective selection with `go list` for macOS, Linux, iOS, Android,
  and Windows. Representative target-compilation checks and the preserved T049 Windows procedure
  passed. For iOS, this records package/file selection and compilation, not a linked, installable,
  or runnable iOS executable. Claude's independent technical review returned PASS as evidence; it
  is not the authority for the amendment. This human decision does not approve the Human
  Implementation Gate, accept renewed H-3 evidence, or authorize merge.

### Report writer (contracts/workflow-gate.md)

- [X] T019 [P] Write `internal/report/workflowgate_test.go`.
  - Reuse `decodeOneWorkflowJSON` and `assertExactWorkflowJSON` from the existing `internal/report`
    tests. Declare no equivalent.
  - **All eight outcomes, in text and JSON:**
    - **Text:** exactly the contract's line, one line, with `%q` for the feature, approver,
      supplied target (`target_unsupported` only), and reason. The notes are
      `transition applied on the decision asserted by the named approver; approver identity is not verified`
      and `no transition was applied`.
    - **JSON:** exactly the contract's key table: `feature_id`, `outcome`, `decision`, and
      `approver` always; `from_state`, `supplied_target`, `target`, `reason`, and
      `missing_artifacts` per outcome.
  - **Injection:** feature and approver values containing `\n`, `"`, and
    `outcome=transition_applied` stay on one line.
  - **Forbidden phrases:** no output contains `authenticated`, `verified approver`,
    `identity verified`, or `approved by`.
  - **Invalid reports:** an invalid `Report` returns an error and writes nothing.

  Depends on T006.
- [X] T020 Implement `internal/report/workflowgate.go`: `WriteWorkflowGateText` and
  `WriteWorkflowGateJSON`, each rejecting `!Valid()`. T019 must pass. Depends on T019.

### CLI steps 1–2, rendering, exit codes, dispatch, wiring

- [X] T021 [P] Write `internal/app/workflowgate_test.go` for
  `runWorkflowGate(decision gate.Decision, args []string, root fs.FS, store gate.Store, now func() time.Time, stdout, stderr io.Writer) int`.
  - **Reuse** the existing helpers `recordingRootFS` (workflowstate_test.go), `errWriter`
    (agentselect_test.go), `workflowNextFailReader` (workflownext_test.go), `readyFS`, and
    `assertExactlyOneTextLine`. The `fakeGateStore` is declared here.
  - **Every usage row** in the contract's table, for both verbs. Each must give exit 2, empty
    stdout, the verb's usage line on stderr, **zero** `recordingRootFS` calls, and **zero** store
    calls. The rows:
    - unknown flag; `--format` with a bad or missing value;
    - 0, 1, or 3 positional arguments;
    - a whitespace ID or target;
    - `--approver` missing, repeated, empty, or whitespace;
    - `--reason` repeated, empty, or whitespace (D-1);
    - `abandon` without `--reason`.
  - **Full C-13 matrix:** `approve` and `abandon` × {`abandoned`, `Abandoned`, `planned`,
    `reviewing`}:
    - `abandon abandoned` reaches the store;
    - `abandon Abandoned`, `abandon planned`, and `abandon reviewing` give exit 2 with the
      stderr explanation `abandonment requires target "abandoned"`;
    - `approve abandoned` gives exit 2 with `use "ai-dev workflow abandon" to abandon a feature`;
    - `approve Abandoned` and `approve reviewing` give `target_unsupported`, exit 1, with zero
      store calls;
    - `approve planned` proceeds.
  - **Exit codes per outcome:** 0 only for `Committed`, otherwise 1.
  - **Warnings:** store warnings go to stderr **after** the report, and never change the exit code.
  - **Output failure after commit:** with an `errWriter` as stdout after `Committed`, the result is
    exit 2, the store is called **exactly once**, and stderr contains
    `write workflow gate report:`.
  - **Internal error:** a report that is not `Valid()` gives exit 2.
  - **Identity sources (FR-006, FR-033, FR-034):** use `t.Setenv` to set `USER`, `LOGNAME`,
    `GIT_AUTHOR_NAME`, `GIT_COMMITTER_NAME`, and `EMAIL` to sentinels. They never appear in stdout,
    stderr, or `Evidence`.
  - **Determinism:** a fixed clock gives byte-identical output.
  - **Dispatcher, via `Run` with a `workflowNextFailReader` stdin:**
    - `Run(... "workflow","approve" ...)` (nil store) gives exit 2,
      `ai-dev: internal error: human gate store unavailable`, zero filesystem calls, and stdin is
      never read;
    - `Run(... "workflow","bogus")` still prints exactly
      `Usage: ai-dev workflow <state|transition> [options]`.

  Depends on T009 and T020.
- [X] T022 Implement `internal/app/workflowgate.go`:
  - `RunWorkflowGate(decision, args, root, store, stdout, stderr) int`, which uses `time.Now`, and
    `runWorkflowGate`, which takes the injected `now`;
  - a set-once `flag.Value` type for `--approver` and `--reason` that records whether it was set;
  - steps 1–2 exactly per the contract, then `gate.Evaluate`, then `Report.Valid()`, then
    rendering, the exit code, and warnings;
  - call `brokenPipeAsError()` at entry and `defer` the release, as `runWorkflowNext` does.

  No stdin. T021 must pass, except the dispatcher cases, which need T023. Depends on T021.
- [X] T023 Edit `internal/app/doctor.go`.
  - Add `RunWithGateStore(ctx, args, executor, launcher, root, store gate.Store, stdin, stdout, stderr) int`
    containing the existing dispatcher body, plus the `workflow approve` and `workflow abandon`
    arms. Each arm checks for a `nil` store first, and in that case prints the internal error and
    exits 2 with no filesystem access.
  - Make `Run` a one-line delegation with a `nil` store.
  - Leave every existing string and arm unchanged.
  - T021 passes fully, and every existing `internal/app` test passes unchanged.

  Depends on T022.
- [X] T024 Edit `cmd/ai-dev/main.go` to call
  `app.RunWithGateStore(ctx, os.Args[1:], system.OSExecutor{}, system.OSLauncher{}, os.DirFS("."), gatestore.New("."), os.Stdin, os.Stdout, os.Stderr)`.
  `go build ./...` must succeed, and the existing `cmd/ai-dev` tests must pass. Depends on T017 and
  T023.

**Checkpoint**: `go test -count=1 ./...` passes on macOS. All foundational tests (T003, T005, T007,
T008, T010, T012, T014, T018, T019, T021) pass.

---

## Phase 3: User Story 1 — Accept completed work and advance one step (Priority: P1) 🎯 MVP

**Goal**: A human `approval` persists exactly one 004 transition with durable evidence (FR-001,
FR-014, FR-019, FR-024, FR-025; SC-001, SC-008, SC-012).

**Independent Test**: A `draft` feature with `spec.md` is approved to `specified`, and so on up to
`approved`. After each step, `workflow state` reports the target and the history has grown by one,
with earlier entries unchanged.

- [X] T025 [P] [US1] Write `internal/app/workflowgate_apply_test.go`. It uses `RunWithGateStore`
  with `os.DirFS(repo)` and `gatestore.New(repo)` on a `controlledTempDir` repository, and reuses
  `snapshotDir`.
  - **Walk:** for each step from `draft` to `approved`, adding the required artifacts beforehand:
    - `transition_applied`, exit 0, with exact text and exact JSON, in separate runs on fresh
      repositories;
    - then `RunWorkflowState` reports the target and its terminal flag;
    - `RunWorkflowNext` reports the expected activity, or `workflow_terminal` after `approved`.
  - **Evidence:** the `human_gates` element fields are exact, `reason` is present iff supplied, and
    earlier elements are byte-equal.
  - **Repository diff:** only `specs/<id>/workflow-state.json` differs; `spec.md`, `plan.md`,
    `tasks.md`, and other features are unchanged.
  - **Text:** the line says "asserted" and "approver identity is not verified".

  Depends on T024.
- [X] T026 [P] [US1] Create `cmd/ai-dev/workflowgate_contract_test.go` with
  `//go:build (darwin && !ios) || (linux && !android)`.
  - **Helpers:** reuse `buildWorkflowNextBinary` and `runCommand`. Declare
    `makeWorkflowGateRepo(t, features map[string][]string) string` here; it is the shared helper
    for T036, T037, and T041.
  - **Real-binary walk:** `draft` → `specified` → `planned` → `tasked` → `implemented` →
    `approved`. Each step gives exit 0 and exact stdout. After each step, `workflow state` and
    `workflow next` agree, and `workflow-state.json` contains the cumulative history.
  - **JSON:** each step's report decodes to exactly one object with the contract's keys.

  Depends on T024.
- [X] T027 [US1] Run
  `go test -count=1 ./internal/app/ ./cmd/ai-dev/ -run 'WorkflowGateApply|WorkflowGateContract'`.
  Fix any defect **only** in `internal/gate/evaluate.go`, `internal/gatestore/record.go`,
  `internal/gatestore/commit_posix.go`, `internal/report/workflowgate.go`, or
  `internal/app/workflowgate.go`. Never weaken a test, and never change a contract. Depends on T025
  and T026.

**Checkpoint**: MVP. Human approval persists state and evidence end to end on macOS.

---

## Phase 4: User Story 2 — No automated signal can stand in for human acceptance (Priority: P1)

**Goal**: FR-002, FR-003, FR-006, FR-007, and FR-022; SC-002 and SC-007. 006 is the sole write
path.

**Independent Test**: Snapshot the repository, run every 001–005 command, and confirm the
repository is byte-identical with no evidence.

- [X] T028 [P] [US2] Write `internal/app/authority_test.go`. The fixture repository uses
  `controlledTempDir`, with feature `f` in `specified` (a record containing no `human_gates`) and
  `spec.md` present. Take a snapshot with `snapshotDir`.
  - **Run through `RunWithGateStore`** with the real store:
    - `doctor`, `agents`, `agent select implementation`;
    - `agent run claude-code "x"` with a launcher fake declared in this file
      (`authorityLauncher`), which reports success (`agent_succeeded`) and creates
      `specs/f/plan.md` and `specs/f/tasks.md`;
    - `workflow state f`, `workflow transition f planned` (`transition_valid`), and
      `workflow next f` (`step_recommended`).
  - **Assert:** `workflow-state.json` is byte-identical, there is still no `human_gates`, and the
    state is still `specified`. The only new paths are the two files the fake agent created.
  - **Replay (US2 AS4, SC-007):** approve `draft`→`specified` on a fresh repository (applied), then
    submit the identical request again. The result is `transition_invalid`, exit 1, with the record
    byte-identical and the history length unchanged.

  Depends on T024.
- [X] T029 [US2] Run `go test -count=1 ./internal/app/ -run Authority` and
  `go test -count=1 ./internal/gate/ ./internal/gatestore/ -run Boundary`.
  - Fix any defect **only** in the 006 files `internal/gate/*.go`, `internal/gatestore/*.go`, and
    `internal/app/workflowgate.go`.
  - If a 001–005 path writes state, stop and report it to the human instead of editing 001–005
    production code.

  Depends on T028.

**Checkpoint**: The authority invariant and the sole write path are proven.

---

## Phase 5: User Story 3 — Refuse invalid gate requests without changing anything (Priority: P2)

**Goal**: FR-012–FR-015, FR-021, and FR-026; SC-003 and SC-004.

**Independent Test**: For each refusal class, the outcome and exit code are distinct, and the
repository is byte-identical.

- [X] T030 [P] [US3] Write `internal/app/workflowgate_refusal_test.go`, with the real store on a
  `controlledTempDir` repository and `snapshotDir` before and after.
  - **`feature_unknown`:** `..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`, an absent feature, and
    a symlinked feature directory (the link is not followed and nothing is created).
  - **`state_invalid`:** every invalid-record kind in the 004 workflow-record contract table:
    - malformed JSON, non-object JSON, an absent or `null` `state`;
    - case-variant members (`State`, `ſtate`), a duplicate `state`, a non-string `state`;
    - an empty, whitespace, or wrong-case value;
    - a symlinked record, a directory named `workflow-state.json`, an unreadable record (non-root).

    The record must never be repaired.
  - **`transition_invalid`:** skip, backward, same-state, target `draft`, `approved` origin, and
    `abandoned` origin.
  - **`target_unsupported`:** `Planned` and `reviewing`, including with the feature absent, with a
    recording root proving zero feature lookups.
  - **`artifacts_missing`:** `planned`→`tasked` without `plan.md`; the case variant `Plan.md`; a
    directory named `plan.md`; a symlink named `plan.md`. The missing list follows 005 order.
  - **SC-004:** a request that is both 004-invalid and missing artifacts gives `transition_invalid`.
  - **FR-026 two-violation table:** each fixture violates two steps, and the earlier step wins.
    `recordingRootFS` proves that no later-step filesystem access happens.
  - **Every case:** exit 1, the report never contains `transition_applied`, the note says no
    transition was applied, the repository is byte-identical, and no `human_gates` appears.

  Depends on T024.
- [X] T031 [US3] Run `go test -count=1 ./internal/app/ -run Refusal`. Fix any defect **only** in
  `internal/gate/evaluate.go`, `internal/app/workflowgate.go`, or
  `internal/report/workflowgate.go`. Depends on T030.

**Checkpoint**: Every refusal is non-mutating and correctly ordered.

---

## Phase 6: User Story 4 — Truthful outcome when state is stale or persistence fails (Priority: P2)

**Goal**: FR-016–FR-018, FR-020, FR-023, FR-031, and FR-037 (ambiguous primitives); SC-005,
SC-006, and SC-010. This covers the full approved lock, crash, and concurrency matrix (P-1, R1–R5).

**Independent Test**: Inject competing holders, kills, failures, and races. The record is always
the complete prior pair or the complete new pair, and the feature is never blocked afterwards.

- [X] T032 [P] [US4] Write `internal/gatestore/lock_posix_test.go` with
  `//go:build (darwin && !ios) || (linux && !android)`.
  It declares the package's **helper-process entry point**: `TestGatestoreHelperProcess`, gated by
  the environment variable `GATESTORE_HELPER`. Its modes are `hold <dir>` (open the feature
  directory, `flock` it, print `ready`, then block) and
  `commit-pause <repo> <id> <expected> <step>` (run `New(repo).Commit` with `ops.afterStep`
  printing `paused:<step>` and blocking at the named step). Test files may import `syscall`. The
  cases:
  - **R1, same-process holder:** the test opens `specs/f` and holds `flock`. `Commit` gives
    `Conflict`, with the record byte-identical and no `*.tmp`.
  - **R1, separate-process holder:** start `hold` and wait for `ready`. `Commit` gives `Conflict`.
  - **R2, SIGKILL recovery:** kill that helper with `SIGKILL` and `Wait()` for it. The **immediately
    following** `Commit` gives `Committed`, with no ai-dev or human cleanup.
  - **R3, ambiguous lock:** inject `ops.flock` returning `syscall.ENOLCK`, `syscall.EOPNOTSUPP`,
    `syscall.EINVAL`, and `syscall.EINTR`, in turn. Each gives `Failed`, with the record
    byte-identical, nothing created or removed, and a call counter showing **exactly one** `flock`
    call (no retry).
  - **Binding, directory replacement:** inject `ops.lstat` returning a different directory's
    `FileInfo`. The result is `Conflict`, with nothing written.
  - **Binding, symlink swap:** from `ops.afterStep("locked")`, rename `specs/f` away and put a
    symlink at `specs/f` that points to a copy. The result is `Conflict`, with nothing written to
    either location.

  Depends on T024.
- [X] T033 [US4] Write `internal/gatestore/crash_posix_test.go` with
  `//go:build (darwin && !ios) || (linux && !android)`.
  It reuses `TestGatestoreHelperProcess` from T032 and declares no second helper entry point.
  - **Crash matrix:** for each step in `locked`, `tmp-written`, `renamed`, and `dir-synced`:
    1. start `commit-pause` and wait for `paused:<step>`;
    2. send `SIGKILL` and `Wait()`;
    3. assert that `workflow.ResolveRecord` gives the **complete prior** state for `locked` and
       `tmp-written`, and the **complete new** state with exactly one appended entry for `renamed`
       and `dir-synced`, and never `invalid`;
    4. assert that the next `Commit` from the then-current state gives `Committed` immediately
       (not poisoned);
    5. assert that any leftover `.workflow-state.*.tmp` is ignored by `workflow.ResolveRecord` and
       by `sdd.MissingArtifacts`.
  - **Post-commit failures (R5):** inject a failing `ops.syncDir`, then a failing `ops.closeDir`,
    then both. Each gives `Committed` plus the expected warnings, with the record at the new state.
    An **immediate second** `Commit` from the new state gives `Committed`.
  - **Pre-commit fault injection**, for each of `openRoot`, `openDir`, `readFile`, the build refusal
    (malformed `human_gates`), `openTmp`, the tmp write, the tmp `Sync`, the tmp `Close`, and
    `rename`: the result is `Failed`, the record is **byte-identical** to before, and no `*.tmp`
    remains.

  Depends on T032.
- [X] T034 [P] [US4] Write `internal/gatestore/concurrency_posix_test.go` with
  `//go:build (darwin && !ios) || (linux && !android)`.
  - **Goroutines:** 16 goroutines `Commit` from the same `expected` on one repository, released
    together by a start barrier. Exactly 1 gives `Committed` and 15 give `Conflict`; `human_gates`
    has exactly one new element; `workflow.ResolveRecord` gives the target.
  - **Real state change:** the on-disk record is advanced by a prior `Commit` before a second commit
    with the old `expected`. The second gives `Conflict`, with nothing written.
  - Designed to run under `-race -count=20` (T047).

  Depends on T024.
- [X] T035 [P] [US4] Write `internal/app/workflowgate_failure_test.go`, with the real store on a
  `controlledTempDir` repository and the shell-level outcomes:
  - a read-only feature directory (non-root) gives `persistence_failed`, exit 1, with the note "no
    transition was applied" and a byte-identical repository;
  - a malformed `human_gates` record gives `persistence_failed`, exit 1, byte-identical;
  - a state changed between evaluation and commit (injected with a store wrapper that edits the
    record and then delegates) gives `state_conflict`, exit 1;
  - **unsupported platform (SC-014, the app-level mapping):** inject a local fake `gate.Store` that
    behaves like the unsupported-platform store. It returns `gate.Failed` with exactly the warning
    `ai-dev: warning: human gate persistence is supported only on macOS and Linux`, and records
    whether it was called. The real store's selection is covered by T018, its semantics by T012,
    and its cross-build by T049.
    - Every request that passes steps 1–7 gives `persistence_failed`, exit 1, with the warning on
      stderr after the report. It never gives `transition_applied` or `state_conflict`, and the
      repository is byte-identical.
    - Requests stopped at steps 1–7 (usage failures, `target_unsupported`, `feature_unknown`,
      `transition_invalid`, `artifacts_missing`) give their normal outcomes, and the fake is never
      called.
  Depends on T024.
- [X] T036 [P] [US4] Write `cmd/ai-dev/workflowgate_concurrency_test.go` with
  `//go:build (darwin && !ios) || (linux && !android)`.
  - Reuse `buildWorkflowNextBinary`, `runCommand`, and T026's `makeWorkflowGateRepo`.
  - Run 20 iterations. In each, 8 concurrent real `ai-dev workflow approve --approver p<n> f specified`
    processes run against a fresh repository. Exactly one gives exit 0 / `transition_applied`, and 7
    give exit 1 / `state_conflict`. The final record has exactly one `human_gates` element, and
    `workflow state` gives `specified`.

  Depends on T026.
- [X] T037 [P] [US4] Create `cmd/ai-dev/workflowgate_brokenpipe_test.go` with
  `//go:build (darwin && !ios) || (linux && !android)`.
  - **Setup:** build the binary, then create `r, w, _ := os.Pipe()` and `r.Close()` **before**
    `cmd.Start`, with `cmd.Stdout = w`.
  - **Check:** `workflow approve --approver a f specified` is **not signaled**, exits 2, and its
    stderr contains `write workflow gate report:`.
  - **Afterwards:** `workflow state f` reports `specified`, and the record has one history element
    (the commit stands, FR-023 and FR-031). Repeat with `--format json`.

  Depends on T026.
- [X] T038 [US4] Run
  `go test -count=1 ./internal/gatestore/ ./internal/app/ ./cmd/ai-dev/ -run 'Lock|Crash|Concurren|Failure|BrokenPipe'`.
  Fix any defect **only** in `internal/gatestore/commit_posix.go`,
  `internal/gatestore/lock_posix.go`, `internal/gatestore/ops.go`, or
  `internal/app/workflowgate.go`, without changing the approved protocol. Depends on T033, T034,
  T035, T036, and T037.

**Checkpoint**: The P-1 lock (R1–R5), crash, post-commit, and concurrency matrix is fully green on
macOS.

---

## Phase 7: User Story 5 — Explicitly abandon a feature (Priority: P3)

**Goal**: FR-008, FR-010, FR-011, C-3, and C-13 (US5 AS1–AS5).

**Independent Test**: An abandonment from each non-terminal state, with no artifacts and a reason,
gives `abandoned`, with evidence `decision: abandonment` and the verbatim reason.

- [X] T039 [P] [US5] Write `internal/app/workflowgate_abandon_test.go`, with the real store on a
  `controlledTempDir` repository.
  - **Each non-terminal origin, with no artifacts at all:** `abandon --approver b --reason r f abandoned`
    gives `transition_applied`, exit 0. The evidence has `decision` `"abandonment"` and the reason
    verbatim. 004 then gives `abandoned` with terminal `true`, and 005 gives `workflow_terminal`.
  - **No artifact lookup:** a `recordingRootFS` shows zero `ReadDir` calls for abandonment.
  - **Terminal origin:** gives `transition_invalid`.
  - **Reason missing, empty, or whitespace:** exit 2 with zero filesystem access.
  - **Reports:** text and JSON for `approval` and `abandonment` differ in `decision`.

  Depends on T024.
- [X] T040 [P] [US5] Create `cmd/ai-dev/workflowgate_c13_test.go` with
  `//go:build (darwin && !ios) || (linux && !android)`. It reuses
  `buildWorkflowNextBinary`, `runCommand`, and T026's
  `makeWorkflowGateRepo`.
  - **Real-binary full C-13 matrix:** `approve` and `abandon` × {`abandoned`, `Abandoned`,
    `planned`, `reviewing`} against a `specified` feature with `spec.md` and `plan.md`.
  - **Assert per cell:** the exact exit code (0, 1, or 2), empty stdout for exit 2, the exact
    stderr explanation lines, and a byte-identical repository for every non-applied cell.

  Depends on T026.
- [X] T041 [US5] Run `go test -count=1 ./internal/app/ ./cmd/ai-dev/ -run 'Abandon|C13'`. Fix any
  defect **only** in `internal/app/workflowgate.go` or `internal/gate/evaluate.go`. Depends on T039
  and T040.

**Checkpoint**: All five user stories are green on macOS.

---

## Phase 8: Polish, Quality Gates, and Acceptance

**Purpose**: Regression, security and scope audit, every plan quality gate, the platform gates, the
required Linux gate, and the final manual acceptance.

- [X] T042 Run the 001–005 regression:
  `go test -count=1 -v ./internal/doctor/... ./internal/checks/... ./internal/agent/... ./internal/execution/... ./internal/instruction/... ./internal/system/... ./internal/role/... ./internal/selection/... ./internal/workflow/... ./internal/sdd/... ./internal/report/... ./internal/app/... ./cmd/ai-dev/...`.
  - All existing tests pass, including 004's and 005's boundary and contract tests.
  - `git diff --name-status HEAD -- '*_test.go'` lists only **added** (`A`/`??`) test files, and no
    modified pre-existing test file (FR-035, SC-013).
  - `git diff --quiet HEAD -- internal/workflow/` (unchanged).

  Depends on T027, T029, T031, T038, and T041.
- [X] T043 [P] Run the security and scope audit over the 006 diff (plan.md checklist). Confirm:
  - writes appear only in `internal/gatestore`, and all of them go through `*os.Root`;
  - no symlink is followed;
  - the lock is only `flock(LOCK_EX|LOCK_NB)` on the directory descriptor: no lock file, and no
    lock removed, broken, or retried;
  - `EWOULDBLOCK` or `EAGAIN` (lock held) maps to `state_conflict`; every other `flock` error is
    ambiguous and maps to `persistence_failed`; there is no retry;
  - `syscall` appears only in `lock_posix.go`
    (`(darwin && !ios) || (linux && !android)`);
  - the approver comes never from the environment, git, or the OS;
  - there is no stdin read, no prompt, no 007 behavior, no goroutine outliving a request, no retry,
    wait, or background work;
  - agent execution, role selection, and `sdd.Evaluate` are never invoked;
  - there is no authentication claim (C-12);
  - stdout is empty on usage failure;
  - 001–005 outputs are unchanged;
  - `go.mod` and `go.sum` are unchanged.

  Also record the accepted residual boundaries as **not** addressed by code: the external or manual
  writer window between the re-validation and the rename; network-filesystem and cross-machine
  coordination; a live hung holder giving `state_conflict`; the P-3 power-loss window; the C-12
  identity assertion.

  Depends on T042.
- [X] T044 [P] Run the boundary gate:
  `go test -count=1 -v ./internal/gate/ ./internal/gatestore/ ./internal/sdd/ -run Boundary`. All
  allow-lists, the sole-write-path checks, and the self-check snippets pass. Depends on T042.
- [X] T045 [P] Run the format and vet gates: `gofmt -l .` prints nothing, and `go vet ./...`
  succeeds. Depends on T042.
- [X] T046 [P] Run the test gates: `go test ./...` and `go test -count=1 ./...` pass. Depends on
  T042.
- [X] T047 [P] Run the race and concurrency gates:
  `go test -count=1 -race ./...` and
  `go test -race -count=20 ./internal/gate/ ./internal/gatestore/` pass. Depends on T042.
- [X] T048 [P] Run the subprocess contract gate: `go test -count=1 -v ./cmd/ai-dev/`. The real-binary
  walk, the C-13 matrix, the 8-process ×20 race, and the broken-pipe tests all pass. Depends on
  T042.
- [X] T049 [P] Run the unsupported-platform build gate (P-6, FR-037):
  Save the following as a separate Bash script and execute that script from the repository root
  with **Bash 4.4 or later** (macOS `/bin/bash` 3.2 is unsupported). Do not paste its setup lines
  into an interactive shell. The module path is `ai-dev` (verified in `go.mod`):

  ```bash
  set -euo pipefail
  export GOOS=windows GOARCH=amd64

  # Stage 1: Cross-build all production packages.
  go build ./...

  # Stage 2: Vet all packages except the two with incompatible legacy tests.
  all_packages=$(go list ./...)
  vet_packages=$(printf '%s\n' "$all_packages" |
    grep -v -x -e 'ai-dev/cmd/ai-dev' -e 'ai-dev/internal/system')
  mapfile -t packages <<<"$vet_packages"
  test "${#packages[@]}" -eq "$(( $(printf '%s\n' "$all_packages" | wc -l) - 2 ))"

  go vet "${packages[@]}"

  # Stage 3: Vet Windows-selected production files in the two excluded packages.
  for package in ./cmd/ai-dev ./internal/system; do
    file_list=$(go list -f '{{range .GoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}' "$package")
    test -n "$file_list"
    mapfile -t files <<<"$file_list"
    go vet "${files[@]}"
  done
  ```

  This replaces the original `GOOS=windows GOARCH=amd64 go vet ./...` step: its failure
  predates Feature 006 and was independently reproduced against clean HEAD `666abd7`. The two
  incompatible legacy test references are `cmd/ai-dev/main_test.go:171` (`syscall.Kill`) and
  `internal/system/oslauncher_test.go:164` (`syscall.Kill`). Stage 1 cross-builds every production
  package for Windows. Stage 2 vets every other package, including its Windows-compatible tests.
  Stage 3 vets the Windows-selected production files from `cmd/ai-dev` and `internal/system`,
  selected by `go list` so files excluded by Windows build constraints are not included. The two
  legacy test files are outside Feature 006's modification scope under FR-035 and T042.
  This procedure does not waive production-code validation or exclude any Feature 006 test that
  the original Windows command could have analyzed. `set -euo pipefail` makes the gate fail if any
  stage fails. This is a human-approved amendment to T049's validation procedure, not a change to
  Feature 006 functional requirements, security guarantees, or the human authority model. It does
  not claim Windows persistence support. Depends on T042.
  **Final verification evidence:** Bash 5.3.20; the three-stage Windows validation exited 0
  (`GOOS=windows GOARCH=amd64`): Stage 1 built all production packages, Stage 2 vetted 14
  packages, and Stage 3 vetted `cmd/ai-dev/main.go` plus the three `internal/system` production
  files. Negative tests confirmed failed `go list` and `go vet` operations fail closed. Claude's
  independent final review resolved D-1 through D-4.
- [X] T050 [P] Run the build and hygiene gates:
  - `go build -o "$(mktemp -d)/ai-dev" ./cmd/ai-dev` succeeds **outside the repository tree**;
  - `git diff --quiet HEAD -- go.mod go.sum`;
  - `git diff --check` reports nothing;
  - `git status --porcelain` shows no binary, `*.tmp`, or `.workflow-state.lock` inside the
    repository.

  Depends on T042.
- [X] T051 **REQUIRED pre-merge Linux gate (human/environment).** On a real Linux host or container
  with Go 1.27.1, on a local filesystem (ext4, xfs, or tmpfs, not a bind mount on a network
  filesystem), run `go test -count=1 ./...` and
  `go test -race -count=20 ./internal/gate/ ./internal/gatestore/`. For example, use the podman
  command in quickstart §1, with the repository copied into the container's local filesystem.
  - The lock (R1–R5), crash, post-commit, concurrency, broken-pipe, and C-13 tests must all pass.
  - Record the command, kernel version (`uname -sr`), filesystem type (`stat -f -c %T`), and full
    output in the PR.
  - A macOS run does **not** satisfy this task. It must not be marked complete without Linux
    evidence.

  Depends on T043–T050.

  **Final verification evidence:** Run on an independent UTM virtual machine with Ubuntu Server
  24.04.5 ARM64, kernel `Linux 6.8.0-146-generic`, and Go `go1.27.1 linux/arm64`. The repository
  was `/home/german/ai-dev` on local `ext4`; `TMPDIR` was
  `/home/german/ai-dev-test-tmp`, also on local `ext4`. The run used non-root user `german`
  (UID 1000) with `GOOS=linux`, `GOARCH=arm64`, and `CGO_ENABLED=1`.

  `go test -count=1 ./...` passed all 16 packages with exit 0. Full output:

  ```text
  ok  	ai-dev/cmd/ai-dev	4.742s
  ok  	ai-dev/internal/agent	0.003s
  ok  	ai-dev/internal/app	0.084s
  ok  	ai-dev/internal/checks	0.003s
  ok  	ai-dev/internal/doctor	0.003s
  ok  	ai-dev/internal/execution	0.005s
  ok  	ai-dev/internal/execution/adapters	0.005s
  ok  	ai-dev/internal/gate	0.008s
  ok  	ai-dev/internal/gatestore	0.103s
  ok  	ai-dev/internal/instruction	0.004s
  ok  	ai-dev/internal/report	0.007s
  ok  	ai-dev/internal/role	0.002s
  ok  	ai-dev/internal/sdd	0.012s
  ok  	ai-dev/internal/selection	0.005s
  ok  	ai-dev/internal/system	0.031s
  ok  	ai-dev/internal/workflow	0.004s
  EXIT_CODE=0
  ```

  `go test -race -count=20 ./internal/gate/ ./internal/gatestore/` passed both packages with exit
  0. Full output:

  ```text
  ok  	ai-dev/internal/gate	1.076s
  ok  	ai-dev/internal/gatestore	3.889s
  EXIT_CODE=0
  ```

  The reviewed macOS copies of the logs are
  `/tmp/ai-dev-t051-evidence/t051-linux-full-test.log` (SHA-256
  `7d8370a6c97690dc0f6874622153d3778ce45cf08fbac929d951fd24445f021f`) and
  `/tmp/ai-dev-t051-evidence/t051-linux-race-test.log` (SHA-256
  `5e928e8bc0c092f06fd1146d8439acefea1896e152c6041f481149cfde2d410f`).
- [X] T052 **Final manual acceptance (human).** Run `specs/006-human-gates/quickstart.md` end to end
  (§1–§4) with the binary built outside the repository and against scratch repositories only:
  - the walk from `draft` to `approved` and through `abandoned`;
  - the replay;
  - `artifacts_missing`;
  - the C-13 cells;
  - the leftover `.workflow-state.lock` being irrelevant;
  - the concurrency, lock-recovery, and broken-pipe test commands;
  - the authority and boundary checks;
  - cleanup.

  **This is a human acceptance task. It must not be marked complete during automated
  implementation, and completing it is not a Human Gate approval.** Depends on T051.

  **Human acceptance evidence (2026-10-08):** The user explicitly accepted T052 after reviewing
  satisfactory evidence for:
  - Go validation (`gofmt`, `go vet`, the full test suite, and the race detector);
  - manual transitions, all seven C-13 rejection cases (7/7), abandonment, and terminal states;
  - the SC-012 audit and compatibility with the old lock file;
  - recovery, concurrency, BrokenPipe, authority, and component-boundary checks; and
  - comparison of `git status --porcelain` with the initial state, with no differences.

  **Complementary evidence status:**

  **Scope-authorization trace (2026-10-08):** The human decision / human-approved scope amendment
  authorizes D-1, the macOS/Linux-only persistence scope, deferred non-blocking H-7, and unchanged
  FR-037/SC-014 guarantees. Claude's technical PASS is independent evidence, not human
  authorization. T052's acceptance is unchanged. The D-1 scope record itself approved no Human
  Gate or renewed H-3 evidence; the later D-2/H-3 decision is recorded separately below.

  - **H-3 (Linux) — PASS — D-2 RENEWED EVIDENCE HUMAN-ACCEPTED (macOS/Linux only).** The earlier
    H-3 PASS remains part of the history: Claude's independent second review on 2026-10-08 returned
    H-3 PASS with no blocking findings for the source tree before the platform-audit D-1
    build-constraint correction; the prior findings B-1, B-2, B-3, and B-4 were resolved. Verified
    historical evidence:
    1. Ubuntu Linux ARM64 with Go 1.27.1, CGO enabled, non-root user (UID 1000), and repository and
       `TMPDIR` on a local ext4 filesystem.
    2. The macOS and Linux source manifests each contain 248 files, compare byte-for-byte equal,
       and both have SHA-256
       `99ff24ca8f2ad456b9dd86a876e43b3d5d7cd0ed6a5e44b65f885363b8a2a0f5`.
    3. `go test -count=1 ./...` passed all 16 packages with exit 0, and
       `go test -race -count=20 ./internal/gate/ ./internal/gatestore/` passed both packages with
       exit 0.
    4. The durable evidence intended for the PR is stored at
       `~/ai-dev-evidence/feature-006/h3-linux/` in these eight files:
       `h3-linux-environment.txt`, `h3-linux-full-test.log`, `h3-linux-race-test.log`,
       `source-manifest-linux.txt`, `source-manifest-macos.txt`, `h3-manifest-method.txt`,
       `generate-macos-manifest.py`, and `SHA256SUMS`. The externally calculated SHA-256 of
       `SHA256SUMS` is
       `d79364f158c6a4542c2fb0fde6ff7e8a49d126b5f9a0cd9bfdbe40918308fc72`.

    Attaching this historical evidence to the PR remains **PENDING** until a PR exists and the
    attachment is performed. Because D-1 changed platform-selection code after these manifests and
    logs were captured, renewed macOS/Linux source manifests, SHA-256 hashes, Linux full-suite and
    race logs, and durable PR evidence for the corrected source tree were recorded as **PENDING**.
    That pending state and the historical PASS are preserved here as the pre-renewal record.

    **D-2 renewal chronology and accepted evidence (2026-10-08):**
    1. Claude's first independent D-2 evidence audit returned **FAIL** with blocking finding BLK-1.
    2. The evidence package was corrected to address BLK-1. Claude's follow-up audit returned
       **D-2 EVIDENCE PASS**, with BLK-1 resolved and no blocking findings.
    3. The responsible human—not Claude—then explicitly approved D-2 and accepted the renewed H-3
       evidence for macOS/Linux, with the known limitations and the non-blocking observations NB-1,
       NB-2, NB-3, and NB-4 preserved.
    4. The accepted external package is
       `/Users/german/ai-dev-evidence/feature-006/h3-linux-d2-renewal-2026-10-08/`. The externally
       verified SHA-256 of `SHA256SUMS` is
       `696e623eb98a67058644bf4935de363acf91b20a3382f7073c6cf21c42616b03`.
    5. `source-manifest-macos.txt` and `source-manifest-linux.txt` each contain 248 files, compare
       byte-for-byte equal, and both have SHA-256
       `595b861b041b8f93b10479432206a8a69ba50d5bd8778bcb2ffd1cb6121aa21f`.
    6. On Ubuntu Linux ARM64, Go 1.27.1 with CGO enabled, non-root UID 1000, and local ext4,
       `go test -count=1 ./...` passed all 16 packages with exit 0, and
       `go test -race -count=20 ./internal/gate/ ./internal/gatestore/` passed both packages with
       exit 0.

    **Non-blocking observations (preserved separately from evidence limitations):**
    - **NB-1:** The D-2 package does not include `generate-macos-manifest.py`; the documented
      manifest methodology remains reproducible.
    - **NB-2:** The source manifest includes six files under `.idea/`.
    - **NB-3:** `synchronized-files-check.log` does not record the `rsync` flags used; byte-content
      equivalence is instead supported by the matching canonical macOS/Linux manifests.
    - **NB-4:** `h3-manifest-method.txt` and `sync-files.txt` do not mention the second test
      execution.

    **Accepted evidence limitations:**
    1. The original `rsync` dry-run and transfer logs do not exist; no `sync-dry-run.log` or
       `sync.log` is claimed.
    2. The race-detector output does not by itself prove `-race -count=20`: the command is recorded
       in the log, but there is no independent iteration counter.
    3. The operator's script recorded the association between the test logs and manifest
       `595b861b041b8f93b10479432206a8a69ba50d5bd8778bcb2ffd1cb6121aa21f`; the independent auditor
       did not directly recalculate the Linux tree during the audited execution.
    4. Claude had no direct access to the `server-test` host and audited the evidence received.
    5. The renewed D-2 package and the historical H-3 package have not yet been attached to a PR.
       Attaching both packages remains **PENDING** until a PR exists and the attachment is actually
       performed.

    `sync-files.txt` records the 20 synchronized paths. `synchronized-files-check.log` is a
    subsequent read-only rsync verification with exit 0, not a reproduction of the original
    transfer logs; its remaining itemizations concern directory metadata. The documentation
    changes that record the human acceptance and this correction necessarily postdate the captured
    source manifests. The external evidence package is not regenerated or modified: the accepted
    manifest identity `595b861b…` identifies the accepted snapshot before these later documentation
    changes. Go production code and tests remain byte-identical to that snapshot, while the current
    documentation no longer does.

    **H-3/D-2 documentation-audit chronology (2026-10-08):**
    1. Claude's initial independent documentation audit returned
       **H-3/D-2 DOCUMENTATION FAIL** because of BLK-A and BLK-B.
    2. A subsequent documentary correction completed the missing observations and limitations and
       restored the explicit pending status of attaching both evidence packages to a future PR.
    3. Claude's second independent documentation audit returned
       **H-3/D-2 DOCUMENTATION PASS**, with no blocking findings.
    4. The final non-blocking observations were:
       - **NB-c:** the historical D-2 text needed to retain explicitly that attaching the renewed
         D-2 package and the historical H-3 package to the PR remains pending until a PR exists.
       - **NB-d:** the D-2 risk row in `plan.md` needed to state the same pending attachment
         condition explicitly.
       Both documentary observations are closed by this correction; the underlying attachment
       remains **PENDING** and is not represented as completed.

    The acceptance-documentation patch is identified by SHA-256
    `4ad907907f3709be74de6c65ad841eb469d21c54c0fd3109076ce64b8ba4f623`; the subsequent correction
    patch is identified by SHA-256
    `0bc4903bf2a87c0643e77e436e3ef9062b2310dddfc4f185ab485c94c358650c`. These hashes identify the
    reviewed patches only: this record does not claim that either patch or either audit dictamen has
    been archived durably, and durable preservation remains pending if the artifacts still exist
    only under `/tmp`.

    Claude issued the technical documentation-audit findings. The responsible human's acceptance
    of H-3/D-2 was a distinct, earlier decision and is not replaced, inferred, or reissued by either
    audit.
  - **H-7 (Windows) — PENDING — DEFERRED.** This evidence is non-blocking for Feature 006's current
    macOS/Linux-only persistence scope. It is not PASS, does not establish native Windows support,
    and preserves the following validation criteria for future closure on a real Windows host:
    1. Run the Windows binary against a temporary repository.
    2. Submit a valid request that reaches persistence and verify `persistence_failed`, exit code
       1, and the warning
       `ai-dev: warning: human gate persistence is supported only on macOS and Linux`; verify that
       neither `transition_applied` nor `state_conflict` is reported.
    3. Compare a recursive before/after content-and-path manifest of the temporary repository and
       verify byte-for-byte equality, with no new lock file (including
       `.workflow-state.lock`).
    4. Verify at least one refusal before persistence with the outcome and exit code required by
       the C-13 contract.
    5. Record the Windows version, `go version`, `go env GOFLAGS`, and the Windows binary's SHA-256
       (for example, with `Get-FileHash -Algorithm SHA256`). Preserve the command output and
       manifests as durable evidence intended for attachment to the PR.

    Reactivate H-7 before distributing or claiming Windows Human Gate persistence support, before
    changing the Windows fallback selection, warning, outcome, or zero-write guarantee, or when
    native Windows validation becomes a release requirement. Reactivation requires a real Windows
    host and completion of all five criteria above; cross-compilation alone is insufficient.

  T052 remains accepted; D-1, D-2, and these complementary-evidence statuses do not change its
  status.

  **Human Implementation Gate: `APPROVED — macOS/Linux scope` (human decision, 2026-10-08).**
  After reviewing the formal gate criteria and the previously accepted H-3/D-2 evidence, the
  responsible human for Feature 006 explicitly approved the implementation for the supported
  macOS/Linux scope. H-7 remains `PENDING — DEFERRED`; native Windows behavior remains unvalidated,
  no complete multiplatform validation is claimed, and FR-037/SC-014 retain their full fail-closed
  contracts. This later decision preserves the history above: completing T052 did not itself
  approve a Human Gate. It does not approve H-7 or authorize a PR, merge, release, or another Human
  Gate.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup**: T001, then T002.
- **Foundational** (blocks all stories):

  | Area | Order |
  |---|---|
  | sdd wrapper | T003, then T004 |
  | gate domain | T005, then T006. T006 is followed by T007, T008, T010, T012, and T019 (all [P]). T008 and T004 come before T009. |
  | gatestore | T010, then T011. T012, then T013. T011 and T013 come before T014. T013, then T015, then T016. T011, T014, T015, and T016 come before T017, which comes before T018. |
  | report | T019, then T020 |
  | app | T009 and T020 come before T021, then T022, then T023 |
  | main | T017 and T023 come before T024 |

- **User stories** all depend on T024 (and T026 where shared helpers are needed). In priority
  order:
  - US1: T025 and T026, then T027;
  - US2: T028, then T029;
  - US3: T030, then T031;
  - US4: T032, then T033; T034, T035, T036, and T037 alongside; all before T038;
  - US5: T039 and T040, then T041.

  The stories are independent of each other except for the shared `cmd/ai-dev` helper from T026,
  which T036, T037, and T040 depend on.
- **Polish**: T042 depends on every story's green task (T027, T029, T031, T038, T041). T043–T050
  are [P] after T042. T051 comes after T043–T050. T052 is last.

### Same-File Ordering (never parallel)

| File | Writer task(s) |
|---|---|
| `internal/sdd/rules.go` | T004 only |
| `internal/gate/evaluate.go` | T009, then fixes only in T027, T031, and T041 (sequential through the story order) |
| `internal/gatestore/commit_posix.go`, `store.go` | T017, then fixes only in T027 and T038 |
| `internal/gatestore/lock_posix.go`, `ops.go` | T016 and T015, then fixes only in T038 |
| `internal/gatestore/record.go` | T011, then fixes only in T027 |
| `internal/report/workflowgate.go` | T020, then fixes only in T027 and T031 |
| `internal/app/workflowgate.go` | T022, then fixes only in T027, T029, T031, T038, and T041 |
| `internal/app/doctor.go` | T023 only |
| `cmd/ai-dev/main.go` | T024 only |
| `cmd/ai-dev/workflowgate_contract_test.go` | T026 only. Its helper `makeWorkflowGateRepo` is reused, not edited, by T036, T037, and T040. |
| `internal/gatestore/lock_posix_test.go` | T032 only. `TestGatestoreHelperProcess` is reused by T033. |

Every other file is written by exactly one task. The green and fix tasks (T027, T029, T031, T038,
T041) are never [P], because they may edit shared production files. Run them strictly in the order
listed.

### Parallel Opportunities

- **After T006**: T007, T008, T010, T012, and T019, all different new files. T003 can run from the
  start.
- **US phases, after T024**: the test-writing tasks T025, T028, T030, T032, T034, T035, and T039
  (each a distinct new file), plus T026. After T026: T036, T037, and T040.
- **Polish**: T043–T050.

---

## Requirement Coverage

| Requirement | Tasks |
|---|---|
| FR-001 | T021, T022, T025, T026 |
| FR-002 | T008, T028, T043 |
| FR-003 | T018, T023, T028, T044 |
| FR-004 | T007, T018, T043 |
| FR-005 | T021, T043 |
| FR-006 | T008, T021, T028 |
| FR-007 | T008, T028 |
| FR-008 | T005, T006, T021, T039 |
| FR-009 | T021, T022 |
| FR-010 | T021, T039 |
| FR-011 | T021, T040 |
| FR-012 | T008, T030 |
| FR-013 | T008, T009, T030 |
| FR-014 | T014, T030 |
| FR-015 | T003, T004, T008, T030, T039 |
| FR-016 | T010, T014, T017, T033 |
| FR-017 | T014, T017, T032, T034, T035, T036 |
| FR-018 | T014, T033, T035 |
| FR-019 | T010, T014, T025 |
| FR-020 | T014, T018, T032, T033 |
| FR-021 | T030, T035 |
| FR-022 | T028 |
| FR-023 | T021, T033, T037 |
| FR-024 | T010, T025, T039 |
| FR-025 | T010, T014, T025 |
| FR-026 | T008, T021, T030 |
| FR-027 | T005, T019, T020 |
| FR-028 | T019 |
| FR-029 | T021, T025, T035 |
| FR-030 | T021, T039 |
| FR-031 | T021, T037 |
| FR-032 | T008, T021 |
| FR-033 | T019, T021, T043 |
| FR-034 | T021, T043 |
| FR-035 | T021 (unchanged dispatcher line), T042 |
| FR-036 | T007, T042 |
| FR-037 | T012, T013, T016, T018 (selection), T032 (R3), T035, T044, T049, T051; D-1 narrows selection to macOS/Linux without weakening fail-closed behavior |
| SC-001 | T025, T026 |
| SC-002 | T028 |
| SC-003 | T030 |
| SC-004 | T008, T030 |
| SC-005 | T033 |
| SC-006 | T034, T036 |
| SC-007 | T028 |
| SC-008 | T010, T025 |
| SC-009 | T019 |
| SC-010 | T021, T037 |
| SC-011 | T008, T021 |
| SC-012 | T025 |
| SC-013 | T042 |
| SC-014 | T018 (five-platform selection) → T012 (store semantics) → T035 (app mapping) → T049 (Windows cross-build); T044 runs T018; native H-7 is deferred |

| Plan decision | Implementation | Tests and gates |
|---|---|---|
| P-1 (flock lock, binding, re-validation, commit protocol) | T015, T016, T017 | T014, T032, T033, T034, T036, T043, T047, T051 |
| P-2 (`RunWithGateStore` seam) | T023, T024 | T021 (nil store, unchanged dispatcher) |
| P-3 (durability boundary) | T017 (step 9 warning) | T033 (`dir-synced`, `syncDir` failure) |
| P-4 (preserve unknown members) | T011 | T010, T014 |
| P-5 (`sdd.MissingArtifacts`) | T004 | T003, T007, T008 |
| P-6 (platform policy) | T013, T016, T017 (D-1 build constraints) | T018 (selection), T012, T035, T049, T044, T051; renewed H-3 evidence human-accepted under D-2 on 2026-10-08 |

---

## Implementation Strategy

### Suggested MVP Scope

Phases 1–3: Setup, Foundational, and US1. That delivers `workflow approve` persisting state and
evidence on macOS. US2 (also P1) should follow immediately, before any release, because it proves
the authority invariant.

### Incremental Delivery

Foundational → US1 → US2 → US3 → US4 → US5 → Polish. T051 and T052 are complete. Platform-audit
D-2 and the renewed H-3 macOS/Linux evidence were human-accepted on 2026-10-08. This does not
by itself authorize merge or approve the Human Implementation Gate. In a separate later human
decision on 2026-10-08, the Human Implementation Gate became `APPROVED — macOS/Linux scope`.
H-7 remains `PENDING — DEFERRED`, native Windows behavior remains unvalidated, and merge remains
separately unauthorized.

## Notes

- Never mark T051 or T052 complete without the stated human or Linux evidence.
- Completing any task is not a Human Gate approval.
- Do not modify `internal/workflow/`, any pre-existing test file, `go.mod`, or `go.sum`. Do not
  commit unless the human asks.
