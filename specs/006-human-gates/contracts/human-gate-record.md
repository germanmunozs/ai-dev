# Contract: Workflow State Record with Human Gate history, and its commit protocol

Implements spec FR-014–FR-025 (persistence, atomicity, stale state, evidence). The rationale is in
[research.md §3–§7](../research.md). This contract extends, and does not change, 004's
[workflow-record.md](../../004-workflow-state/contracts/workflow-record.md): every reading rule there
remains authoritative and unchanged.

## Files in `specs/<feature-id>/`

| Name | Written by | Read by | Notes |
|---|---|---|---|
| `workflow-state.json` | `internal/gatestore` only | 004 (`ResolveRecord`), and the store under the lock | Authoritative state plus `human_gates` history |
| `.workflow-state.<hex>.tmp` | `internal/gatestore` only, during a commit | nobody | Renamed over the record, or removed. A crash leftover is ignored by 004, 005, and git (the `*.tmp` rule), and never blocks anything. |

There is **no lock file**. Mutual exclusion is a kernel `flock(2)` advisory lock on the feature
directory's own descriptor, which exists only while that descriptor is open
([research §5](../research.md)). No other path is ever created, modified, or removed. Every operation
goes through an `os.Root` opened at the repository root, so no path can escape the repository.

## Record schema (as written by 006)

```json
{
  "state": "<canonical to_state>",
  "human_gates": [
    {
      "feature_id": "<id>",
      "decision": "approval",
      "approver": "<verbatim>",
      "from_state": "<canonical>",
      "to_state": "<canonical>",
      "decided_at": "<RFC 3339 UTC, seconds>",
      "reason": "<verbatim, only when supplied>"
    }
  ]
}
```

- The value of `state` equals the `to_state` of the **last** `human_gates` element that 006 wrote.
- The `state` member is written exactly once, with exactly that name and a canonical value. It is
  therefore resolved by 004's unchanged reader to the target (FR-015, FR-019).
- `human_gates` is append-only. Earlier elements are preserved byte-for-byte as raw JSON values;
  re-indentation changes whitespace only.
- Any other top-level member found in an existing record is preserved verbatim and in its original
  position.
- The file is encoded as two-space-indented JSON, followed by a trailing newline.

## Build rules (`existing`, `to`, `entry`) → bytes

| Existing record | Result |
|---|---|
| Absent | `{"state": to, "human_gates": [entry]}` |
| Present and 004-valid, with no `human_gates` member | Members in order with `state` replaced, then `"human_gates": [entry]` appended |
| Present and 004-valid, with exactly one `human_gates` member that is a JSON array | Members in order, with `state` replaced and `entry` appended to the array |
| `human_gates` present but not an array (object, string, `null`, …) | **Refuse**: `Failed` (`persistence_failed`), nothing written (D-2) |
| `human_gates` member duplicated | **Refuse**: `Failed` |
| A member whose name differs from `human_gates` only by case (`strings.EqualFold`) | **Refuse**: `Failed` |

The elements of `human_gates` are not schema-validated; they are preserved, never interpreted.

## Platform policy

Persistence is supported on **macOS and Linux, on local filesystems** (spec FR-037 and SC-014;
[research §13](../research.md)).
The POSIX store is selected only by
`(darwin && !ios) || (linux && !android)`. Its exact complement,
`!((darwin && !ios) || (linux && !android))`, selects the fallback. On every other platform,
including Windows, iOS, and Android, `Store.Commit` performs no filesystem access and returns
`Failed`, giving `persistence_failed`, with the stderr warning
`ai-dev: warning: human gate persistence is supported only on macOS and Linux`.

## Commit protocol (`Store.Commit(featureID, expected, entry)`, macOS/Linux)

Every step operates on `dir = workflow.FeatureDir(featureID)` inside the `os.Root`.

| # | Step | On failure |
|---|---|---|
| 0 | `os.OpenRoot(repoRoot)` | `Failed` |
| 1 | `dirFile := root.Open(dir)` (read-only, `O_CLOEXEC`) | `Failed` |
| 2 | `flock(dirFile, LOCK_EX\|LOCK_NB)` | `EWOULDBLOCK` (an active holder) gives `Conflict`. **Any other error gives `Failed`** (ambiguous, fail closed). Nothing is created or removed, and there is no retry. |
| 3 | Bind the lock to the path: `root.Lstat(dir)` must be a real directory (not a symlink), and `os.SameFile(dirFile.Stat(), lstat)` must hold | Otherwise `Conflict` (the feature directory was replaced) |
| 4 | Re-validate with 004 on `root.FS()`: `FeatureExists`, then `ResolveRecord` | Feature gone, record invalid, or `state != expected` gives `Conflict` |
| 5 | Read the raw record bytes (`ErrNotExist` means absent); `Lstat` for the permission bits | A read error gives `Failed` |
| 6 | Build the new bytes (rules above) | A refusal gives `Failed` |
| 7 | `OpenFile(dir/.workflow-state.<16 hex>.tmp, O_WRONLY\|O_CREATE\|O_EXCL, perm)`, then write, `Sync`, and `Close` | `Failed`, and the tmp file is removed (best effort) |
| 8 | **Commit point**: `Rename(tmp, dir/workflow-state.json)` | `Failed`, and the tmp file is removed (best effort) |
| 9 | Post-commit: `dirFile.Sync()` (the directory flush, P-3) | A warning only; the status stays `Committed` |
| — | Always, deferred after step 1 succeeded: `dirFile.Close()`, which releases the lock | A warning only; the status is unchanged. The kernel releases the lock at process exit in any case. |

**Guarantees**:
- Before step 8 returns, the complete prior record is the authoritative record. After it returns,
  the complete new record is. No reader can observe anything in between (FR-016, FR-020).
- `Committed` is returned only after step 8 succeeded (FR-019).
- `Conflict` and `Failed` leave `workflow-state.json` byte-identical, and leave no file behind. A
  tmp file can remain only if the process dies between steps 7 and 8, or if its best-effort removal
  fails (FR-018, FR-021).
- **No persistent lock state exists.** A holder that exits, crashes, or is killed releases the lock
  in the kernel. The next request acquires it immediately, so recovery is automatic, deterministic,
  and never performed by ai-dev. A post-commit failure (step 9 or `Close`) therefore never blocks a
  later gate ([research §5](../research.md), case analysis).
- **An ambiguous lock fails closed.** Any `flock` error other than `EWOULDBLOCK` gives `Failed`, and
  ai-dev never removes, breaks, or overrides a lock.
- No step follows a symlink:
  - step 3 rejects a replaced or symlinked directory;
  - step 4 uses 004's no-follow `Lstat`;
  - step 7 uses `O_EXCL` with a random name;
  - step 8 replaces a directory entry rather than writing through it;
  - `os.Root` rejects escapes.
- Of N concurrent ai-dev commits from the same `expected`, at most one returns `Committed`, and every
  other returns `Conflict` (FR-017, SC-006). A loser either gets `EWOULDBLOCK` at step 2, or acquires
  the lock after the winner and fails re-validation at step 4.

**Explicitly not guaranteed** (residual risks, for acceptance in plan.md):
- Protection against a writer that ignores the advisory lock (a hand edit or another tool). Its
  write is lost only if it lands between step 4's read and step 8's `rename`; it is then replaced by
  a record built from step 4's bytes. This race is documented precisely in research §5.
- Durability of the rename across power loss before step 9 completes (P-3). After such a loss, the
  complete prior pair may be observed, never a partial one.
- Exclusion across *hosts* on network filesystems whose `flock` is host-local. Such filesystems are
  unsupported (research §13).

## Test seams

- **Builder**: a table covering every build-rule row, plus unknown-member preservation (order and
  bytes). A property test, over all 004-permitted transitions and a randomized sequence of up to 10
  appends, asserts the following:
  - `workflow.ResolveRecord` over `fstest.MapFS{record}` returns the target, with `invalid == false`;
  - the earlier elements are byte-equal after `json.Compact`;
  - the element count increases by exactly 1.
- **Store** (`//go:build (darwin && !ios) || (linux && !android)`): on a real `t.TempDir()` repository, through
  `gatestore.New(dir)`:
  - first commit with no record; append; conflict on changed state; conflict when the record became
    invalid; `Failed` on malformed `human_gates`;
  - `Failed` on a read-only feature directory (non-root; skipped otherwise), with the prior record
    byte-identical;
  - no tmp file is left after any status;
  - a regular file named `.workflow-state.lock` in the directory (a leftover from the rejected
    lock-file design, or created by a user) is ignored, and the commit succeeds.
- **Lock: active holder** (R1):
  - in-process, a test opens the feature directory and holds `flock` on it; `Commit` returns
    `Conflict`, with the record byte-identical and no tmp file;
  - cross-process, a helper process started with `os/exec` and an environment-variable gate in the
    test binary holds the lock and signals readiness over a pipe; `Commit` returns `Conflict`.
- **Lock: dead holder, automatic recovery** (R2): the helper process from the previous item is
  SIGKILLed and reaped. The next `Commit` then returns `Committed` immediately, with no ai-dev action
  and no human action.
- **Lock: ambiguous, fail closed** (R3): the `ops.flock` seam returns `ENOLCK`, `EOPNOTSUPP`,
  `EINVAL`, and `EINTR` in turn. Each gives `Failed`, with the record byte-identical, nothing
  written, nothing removed, and no second `flock` call (no retry).
- **Lock binding**: the `ops` seam makes `Lstat(dir)` report a different directory identity, then a
  symlink. Each gives `Conflict`, with nothing written.
- **Crash during commit** (R4): a helper process runs `Commit` with a test-only pause hook (an
  unexported `ops.afterStep` callback that blocks on a pipe). It is SIGKILLed at each of these
  points:
  - after the lock is acquired;
  - after the tmp file is written;
  - after the rename;
  - after the directory `Sync`.

  After each kill: 004 resolves either the complete prior state (kills before the rename) or the
  complete new state with exactly one appended entry (kills after the rename), never `state_invalid`.
  A following `Commit` from the then-current state returns `Committed`, so the feature is not
  poisoned. A leftover tmp file is ignored by 004 and by `sdd.MissingArtifacts`.
- **Post-commit failures** (R5): the `ops` seam makes `dirFile.Sync()` fail, then `Close()` fail,
  then both. Each gives `Committed` plus the expected warnings, the record is the new state, and an
  immediate second `Commit` from the new state returns `Committed`.
- **Fault injection before the commit point**: failures at `OpenRoot`, at the directory open, and
  at the read, build, tmp open, write, `Sync`, `Close`, and rename steps. Each gives `Failed`, with
  the record byte-identical and the tmp file removed.
- **Concurrency**:
  - (a) In-process: 16 goroutines commit from the same `expected`, giving exactly 1 `Committed` and
    15 `Conflict`, with one appended element. Run with `-race`, ×20.
  - (b) Multi-process: the built `ai-dev` binary is started 8 times concurrently against a temp
    repository, giving exactly one exit 0 / `transition_applied`, with the others exit 1 /
    `state_conflict`. Run ×20.
- **Unsupported-platform store**: the behavior lives in an untagged file, so it is unit-tested on
  every host; the constructor is selected by
  `//go:build !((darwin && !ios) || (linux && !android))`. It returns `Failed` plus the platform
  warning, and a recording root shows zero filesystem calls. Structural `go list` checks prove that
  macOS/Linux select the POSIX store and iOS/Android/Windows select the fallback; representative
  cross-builds prove those selections compile. Native Windows runtime validation remains H-7
  `PENDING — DEFERRED` and is not a support claim.
