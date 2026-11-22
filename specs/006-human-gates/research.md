# Research: Human Gates (006)

**Input**: [spec.md](./spec.md), approved at the Human Specification Gate. Each section records a
planning decision: the decision, its rationale, and the alternatives considered. None of them changes
an approved product semantic.

## §1 Reuse of 004 for precedence steps 3–6

**Decision**: Steps 3–6 of FR-026 (target canonicality → feature recognition → current-state
resolution → transition validation) are performed by one call to the existing, unchanged
`workflow.EvaluateTransition(root, featureID, suppliedTarget)`. 006 maps its outcome as follows:

| `EvaluateTransition` outcome | 006 action |
|---|---|
| `target_unsupported`, `feature_unknown`, `state_invalid`, `transition_invalid` | Report the same token (same `workflow.Outcome` value) and stop |
| `transition_valid` | Continue to step 7. This is a validity precondition only, never authorization (FR-007) |

**Rationale**: `EvaluateTransition` is 004's FR-029 implementation, so the approved order and data
access come for free:
- a non-canonical target returns before any `fs.FS` call;
- feature-ID validation runs before path construction;
- record reading follows the approved rules, with no symlink following.

Calling it means 006 contains **no** copy of the Transition Rule Set, the canonical vocabulary, or
the record-reading rules.

**Alternatives rejected**:
- Calling `Recognized`, `FeatureExists`, `ResolveRecord`, and `Allowed` separately: this
  re-implements FR-029's ordering in a second place, which risks divergence.
- Shelling out to `ai-dev workflow transition`: this adds process coupling and parsing.

## §2 Reuse of 005's Required Artifacts Rule: minimal API extension

**Decision**: Add one exported, I/O-equivalent wrapper to `internal/sdd`:

```go
// MissingArtifacts reports the required artifacts for state that are absent from featureID's
// feature directory, in 005's fixed order, under 005's exact presence rule.
func MissingArtifacts(root fs.FS, featureID string, state workflow.State) []string {
    return missingArtifacts(root, featureID, state)
}
```

006 calls it with the **explicit target state**, for `approval` only, after `EvaluateTransition`
returned `transition_valid`.

**Rationale**: The table (`requiredArtifacts`) and the presence rule (`missingArtifacts`: one
`fs.ReadDir`, an exact byte-equal name, `DirEntry.Type().IsRegular()`, I/O error counted as missing)
are 005-owned and unexported. A one-line wrapper exposes them without copying. This mirrors the
approved precedent of `workflow.FeatureDir` from 005's plan. The wrapper:
- changes no 005 behavior;
- does not call `sdd.Evaluate` or `nextStep`;
- does not touch the Next-Step Table.

**Alternatives rejected**:
- Calling `sdd.Evaluate` on a hypothetical target: this is impossible without faking state, and it
  would be "invoking workflow next".
- Copying the table into 006: this violates FR-015.
- Exporting `requiredArtifacts` only: 006 would then re-implement the presence rule.

## §3 Where Human Gate evidence lives

**Decision**: Evidence is embedded in the feature's existing Workflow State Record,
`specs/<feature-id>/workflow-state.json`, as an append-only top-level array member `human_gates`.
The top-level `state` member remains the 004 authority.

```json
{
  "state": "planned",
  "human_gates": [
    {"feature_id": "006-human-gates", "decision": "approval", "approver": "alice",
     "from_state": "draft", "to_state": "specified", "decided_at": "2026-10-06T14:03:22Z"},
    {"feature_id": "006-human-gates", "decision": "approval", "approver": "alice",
     "from_state": "specified", "to_state": "planned", "decided_at": "2026-10-07T09:12:05Z",
     "reason": "plan reviewed"}
  ]
}
```

**Rationale**:
- **Atomicity for free.** State and evidence are one file, so one atomic replace makes the new
  pair visible together, or not at all (FR-016). No second file, no two-phase protocol, and no
  journal are needed.
- **004 compatibility is already approved.** The 004 record contract states: "Any other field present
  in the object is ignored by this feature (forward-compatible with a future feature adding fields
  such as a timestamp or an audit trail, without requiring this feature's reader to change). Only
  top-level members are considered." `human_gates` is not a case variant of `state`, and the nested
  `from_state` and `to_state` names are never considered.
- **Version-controllable.** The evidence lives in the feature's directory, next to `spec.md`, and is
  reviewable in a pull request (FR-024, SC-012).

**Alternatives rejected**:
- A separate `human-gates.json` or JSON-lines log: two files cannot be replaced atomically together.
  This would need ordering plus recovery logic, and a crash would leave evidence without state, or
  state without evidence (FR-016 violation).
- Keeping only the latest decision in the record: this erases earlier evidence (C-6 violation).
- Git commits as evidence: version-control operations are out of scope (FR-020).

## §4 Atomic replace of the record

**Decision**: Use write-to-temporary, `fsync`, then `rename`, all through an `*os.Root` opened at
the repository root (§6). Inside `specs/<id>/`:
1. Create `.workflow-state.<16 random hex>.tmp` with `O_WRONLY|O_CREATE|O_EXCL`, mode `0644`, or the
   existing record's permission bits.
2. Write the complete new record bytes, then `File.Sync()`, then `Close()`.
3. `Root.Rename(tmp, "specs/<id>/workflow-state.json")`. **This is the commit point.**
4. Post-commit, best effort: `Sync()` the feature directory, using the descriptor that is already
   open and locked (§5), so the rename becomes durable.

On any failure before step 3, the temporary file is removed (best effort), and the outcome is
`persistence_failed`.

**Rationale**: `rename(2)` within one directory atomically replaces the destination on POSIX
filesystems. This holds on the supported platforms only (§13). A concurrent 004 reader therefore opens either the old file or the new one, never a
partial one (FR-020). The temporary file's name:
- is not `workflow-state.json`, so 004 never reads it;
- is not `spec.md`, `plan.md`, or `tasks.md`, so 005 never counts it;
- matches the repository's existing `*.tmp` `.gitignore` rule, so a crash leftover is not committed
  by accident.

**Durability boundary (decided here)**: success is reported only after the file data has been
`fsync`ed and the rename has returned. The directory `fsync` happens after the commit point.
- If it fails, the outcome stays `transition_applied` (FR-023), and a warning goes to stderr.
- If power is lost before the directory metadata reaches disk, the filesystem may come back with
  the **complete prior** record. That is still a consistent pair, never a partial one.

This residual risk is recorded in plan.md.

**Alternatives rejected**:
- In-place `O_TRUNC` writes: these expose partial content.
- `os.CreateTemp`: it cannot be scoped to an `os.Root`, so traversal safety would be lost.
- Directory `fsync` before reporting, with a "rollback" on failure: rolling back a visible rename is
  itself non-atomic, and FR-023 forbids invalidating a commit.

## §5 Stale-state detection and concurrency: a kernel advisory lock on the feature directory, plus re-validation

*Remediated after the Human Plan Gate review (2026-10-06). The earlier design used an
`O_CREAT|O_EXCL` lock **file**, which a killed process could leave behind. That would block the
feature until a human deleted it, so it was rejected (former P-1).*

### Requirements (from the Human Plan Gate review)

| ID | Requirement |
|---|---|
| R1 | A lock owned by an active competing invocation produces `state_conflict`. |
| R2 | A stale lock has a safe, deterministic recovery path. |
| R3 | An ambiguous lock fails closed: never auto-remove when ownership or liveness is unknown. |
| R4 | Recovery never weakens FR-017 (stale state) or FR-016 (all-or-nothing). |
| R5 | A post-commit failure (directory flush or lock release) never poisons the feature. |
| R6 | The smallest robust design for the supported platforms. |

### Alternatives compared

| # | Mechanism | Liveness proof | Stale recovery | Verdict |
|---|---|---|---|---|
| A | `O_EXCL` lock file plus manual removal (former P-1) | none | human deletes the file | **Rejected**: violates R2 and R5. A crash or a failed post-commit removal leaves a file that blocks every later gate. |
| B | `O_EXCL` lock file plus PID | `kill(pid, 0)` | remove if the PID is dead | **Rejected**: a PID alone is not proof. PIDs are reused, have no meaning across PID namespaces (containers) or hosts (shared filesystems), and `kill(pid, 0)` returns `EPERM` for other users. The result is ambiguous often, which under R3 means blocked often. |
| C | `O_EXCL` lock file plus PID plus process start time | PID plus a start-time match | remove on mismatch | **Rejected**: start time needs per-OS code (`/proc/<pid>/stat` on Linux, `sysctl kern.proc.pid` struct parsing on macOS). It still has no meaning across namespaces or hosts. Breaking the lock is a check-then-remove race: two recoverers can each remove and recreate, violating R4. |
| D | `O_EXCL` lock file plus timestamp lease (break after N seconds) | none | break after expiry | **Rejected**: a slow but live holder (for example, a stalled `fsync`) would have its lock broken, letting two writers proceed (R4 violation). A timestamp alone proves nothing. |
| E | Lock file plus heartbeat or lease renewal | renewal timestamps | break if not renewed | **Rejected**: needs a background goroutine (forbidden by FR-004 in spirit), and still relies on time thresholds plus a break race. |
| F | POSIX `fcntl(F_SETLK)` record lock | kernel | kernel releases on process exit | **Rejected**: the lock belongs to the *process*. Closing *any* descriptor of the file in that process releases it, and two goroutines never conflict, so in-process concurrency is unprotected. Linux's `F_OFD_SETLK` fixes this but is Linux-only. |
| G | **BSD `flock(2)` on the feature directory's own descriptor** | **the kernel**: the lock exists only while an open file description holding it exists | **automatic and immediate**: the kernel drops it when the holder closes it or the holder's process terminates for any reason (normal exit, panic, SIGKILL, OOM kill). Locks are not persistent, so none survives a reboot. | **Chosen.** |
| H | `flock(2)` on a dedicated persistent lock file | as G | as G | A viable fallback, but it adds a permanent file to every gated feature directory. G needs no file at all. |
| I | Windows `LockFileEx` | kernel | automatic | Not available in the Go standard library's `syscall` (needs `golang.org/x/sys/windows`), and Windows is outside 006's persistence platform policy (§13). |

### Chosen design (G)

All operations go through the store's `os.Root`, and only after steps 1–7 have passed.

1. **Open** the feature directory: `dirFile := root.Open(dir)`. Go opens it read-only with
   `O_CLOEXEC`, so no child process can ever inherit the lock (and the gate path spawns none).
2. **Try the lock** without blocking: `syscall.Flock(dirFile.Fd(), LOCK_EX|LOCK_NB)`.

   | Result | Meaning | Outcome |
   |---|---|---|
   | `nil` | Acquired | Continue |
   | `EWOULDBLOCK` (`EAGAIN`) | Another open file description holds the lock, so a holder is **demonstrably alive**. It is normally a concurrent ai-dev gate; any foreign holder is treated identically. | `state_conflict` (**R1**). Nothing is written or removed. |
   | Any other error (`ENOLCK`, `EOPNOTSUPP`/`ENOTSUP`, `EINVAL`, `EBADF`, `EINTR`, …) | Exclusivity **cannot be established** on this filesystem or at this moment | `persistence_failed` (**R3**, fail closed). Nothing is written or removed, and there is no retry. |

3. **Bind the lock to the path.** Compare `dirFile.Stat()` with `root.Lstat(dir)`. The entry must
   still be a real directory and not a symlink, and `os.SameFile` must hold. Otherwise the feature
   directory was replaced after recognition, so the authoritative state cannot be confirmed:
   `state_conflict`, with nothing written. This closes the gap where a replaced directory leaves
   the lock guarding an orphaned inode.
4. **Re-validate under the lock** with 004's unchanged `FeatureExists` and `ResolveRecord` on
   `root.FS()`. If the state is missing, invalid, or not equal to the evaluated state, the outcome
   is `state_conflict` (FR-017, unchanged).
5. **Build, write, and commit** (§4, §7): write the tmp file, then `Sync`, then `Close`, then
   `Rename` (the commit point). This is unchanged.
6. **Post-commit**: `dirFile.Sync()`. This is P-3's directory flush, done on the descriptor that is
   already open and locked. A failure is a warning only.
7. **Release**: `dirFile.Close()`, deferred on every path. Closing drops the lock. A `Close` error is
   a warning only. Even if `Close` were skipped or failed, the kernel drops the lock when the process
   exits.

### Why this is safe

- **R1**: `EWOULDBLOCK` is a kernel statement that a live open file description holds the lock.
- **R2**: there is no stale lock state to recover from. A dead holder's lock no longer exists, and
  the next request acquires it immediately. The kernel performs the recovery deterministically.
  ai-dev never decides that a lock is stale and never removes anything.
- **R3**: every error other than "held" fails closed with `persistence_failed`. ai-dev never deletes
  or overrides a lock, because there is no lock file to delete.
- **R4**: the lock only serializes. Correctness still comes from the re-validation under the lock
  (FR-017) and the single-rename commit (FR-016). A recovery that happens because a holder died
  cannot expose a partial pair: the dead holder either renamed, giving the complete new pair, or
  did not, giving the complete prior pair. Its `*.tmp` file is never read.
- **R5**: the case analysis follows.

### Case analysis: commit succeeded, then post-commit trouble (R5)

| Sequence | Report | Lock afterwards | Next gate for this feature |
|---|---|---|---|
| rename ok → `dirFile.Sync()` fails → `Close` ok | `transition_applied`, exit 0, plus a stderr warning (FR-023, P-3) | released by `Close` | proceeds normally against the **new** state |
| rename ok → `Sync` ok → `Close` returns an error | `transition_applied`, exit 0, plus a stderr warning | released no later than process exit | proceeds normally |
| rename ok → the report write fails (stdout closed) | exit 2 (FR-031) | released by the deferred `Close` | proceeds normally |
| rename ok → the process is killed before `Close` | (no report) | **released by the kernel at process death** | proceeds normally; `workflow state` shows the new state |
| killed after tmp write, before rename | (no report) | released by the kernel | proceeds normally against the **prior** state; the leftover `*.tmp` is ignored |
| killed while holding the lock before re-validation | (no report) | released by the kernel | proceeds normally |

No row leaves any artifact that blocks a later gate. A leftover `.workflow-state.<hex>.tmp` has a
random name created with `O_EXCL`, is never read by 004 or 005, is ignored by git (`*.tmp`), and
never collides with a later temporary file.

### Precise residual race: writers that ignore the protocol

`flock` is advisory. A process that writes `workflow-state.json` without taking the directory lock
(a hand edit, an editor, or another tool) is not excluded. The only window in which its write is
lost is between step 4's read and step 5's `rename` returning. In that window, its change is
replaced by a record built from the bytes read in step 4. Outside that window:
- an edit made before step 4 is seen, and gives either `state_conflict` or preserved content;
- an edit made after the rename is the external writer's own overwrite, which 004 then reads as it
  reads any record.

Making this compare-and-swap-safe would need an atomic content-conditional replace, which POSIX
filesystems do not provide. The spec already places such edits outside ai-dev's control (C-5).

## §6 Filesystem confinement: `os.Root`

**Decision**: The store opens `os.OpenRoot(<repository root>)` (`"."` in production, the same
directory `main` already passes as `os.DirFS(".")`). It performs every write, rename, remove, and
lock operation, plus the under-lock re-read (`root.FS()`), through that root, using the
slash-separated paths from `workflow.FeatureDir` plus fixed file names.

**Rationale**:
- `os.Root` (Go ≥ 1.24, with `Rename` in 1.25) refuses any path or symlink that escapes the root,
  so a write cannot leave the repository (FR-020).
- `Root.FS()` implements `fs.ReadLinkFS`, so 004's `FeatureExists` and `ResolveRecord` run
  unchanged on it, with no link following and no fallback.
- 004 has already validated the feature ID before any write path is built: `workflow.FeatureDir`
  is called only after `transition_valid`.

**Residual (narrowed by §5 step 3)**: the store binds the locked directory descriptor to the path
(`os.SameFile` plus a no-follow `Lstat`) before re-validation. A feature directory that was replaced
or symlinked after recognition therefore yields `state_conflict`. A replacement made *after* that
binding check, but before the `rename`, remains possible only for an actor with local write access
to the repository, which is outside the threat model, and `os.Root` still prevents any escape from
the repository. Not following links on the record itself is still guaranteed: `rename` replaces a
link entry rather than writing through it, and a symlinked record already fails 004's resolution
under the lock.

## §7 Building the new record without erasing anything

**Decision**: The record builder lives in `internal/gatestore` and is pure: it maps
`(existing []byte or none, newState, entry)` to `(record []byte or an error)`.
- **No existing record**: the result is `{"state": <to>, "human_gates": [<entry>]}`.
- **Existing record**: by the time the builder runs, it has already passed 004's resolution under the
  lock, so it is one JSON object with exactly one exact `state` member. The builder walks the
  top-level members in order with a `json.Decoder` (the same token-walking technique as 004's
  `stateField`), keeping every member's name and raw value:
  - `state`: its value is replaced by the new canonical state.
  - `human_gates`, exact name: it MUST be a JSON array. Its existing elements are kept as raw bytes,
    in order, and the new entry is appended.
  - Any other member is kept verbatim, in its position.
  - If no `human_gates` member exists (a hand-written record), one is appended at the end.
- **Refusals (D-2, so `persistence_failed`, and nothing is written)**:
  - `human_gates` is present but is not an array;
  - `human_gates` appears more than once;
  - a member name that differs from `human_gates` only by case (`strings.EqualFold`) is present.

  In each case the existing evidence cannot be preserved unambiguously.
- **Output**: the members are re-encoded and passed through `json.Indent` (two spaces), with a
  trailing newline. Indentation changes whitespace only, never a value. Earlier entries stay
  semantically unaltered, and their order is preserved (FR-025).

**Verification**: there is no runtime self-parse, which would duplicate 004's reader. Instead, a
table and property test runs every builder output through the **real** `workflow.ResolveRecord`
(over `fstest.MapFS`), asserting that it resolves to the target. For appends, it also asserts that
the earlier `human_gates` elements are byte-equal after compaction (FR-015, FR-019, FR-025).

**Alternatives rejected**:
- Unmarshalling into a struct and re-marshalling: this drops unknown members and reorders members.
- Rejecting records with unknown members: this would make a hand-annotated record ungateable for no
  product reason.

## §8 CLI shape

**Decision**: Two subcommands. The decision type is the subcommand, and the target is always an
explicit positional argument (C-2):

```text
ai-dev workflow approve [--format text|json] --approver <identity> [--reason <text>] <feature-id> <target-state>
ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>
```

- `--approver` and `--reason` use a set-once `flag.Value`. Supplying either flag twice is a usage
  failure, because Go's `flag` package is otherwise last-wins, which would silently discard human
  input.
- The value also records whether it was set, so that "supplied but empty or whitespace" (D-1) can be
  told apart from "not supplied".
- `--format` keeps the existing commands' behavior.
- Flags precede positionals, as in every existing ai-dev command.

**Rationale**:
- Distinct verbs make abandonment impossible to mistake for approval (US5 AS5).
- The explicit `<target-state>` on `abandon` keeps C-2 uniform and gives C-13 its input (`abandoned`
  exactly).

**Alternatives rejected**:
- `--decision approval|abandonment`: one verb for two meanings, which is easier to mistype.
- A target implied for `abandon`: this contradicts C-2.

## §9 Wiring the store without editing existing tests

**Decision**:
- Add `app.RunWithGateStore(ctx, args, executor, launcher, root, store, stdin, stdout, stderr)`,
  which contains the existing dispatcher plus the two new `workflow` subcommands.
- The existing `app.Run` keeps its exact signature and becomes a one-line call to
  `RunWithGateStore` with a `nil` store.
- `main` calls `RunWithGateStore` with `gatestore.New(".")`. `New` performs no I/O; the root is
  opened per commit.
- With a `nil` store, `workflow approve` and `workflow abandon` report
  `ai-dev: internal error: human gate store unavailable` on stderr, perform no filesystem access,
  and exit 2. That path is reachable only from tests that call `Run`.

**Rationale**: `Run` has 28 existing test call sites. Changing its signature would force
non-additive edits to approved 001–005 test files. All existing commands keep byte-identical
behavior. The dispatcher's existing `Usage: ai-dev workflow <state|transition> [options]` line is
left unchanged, as 005 left it (FR-035).

## §10 Output failure and signals

**Decision**:
- `RunWorkflowGate` calls the existing `brokenPipeAsError()` helper, as `workflow next` does. A
  closed stdout then surfaces as a write error, giving exit 2 with a stderr diagnostic, instead of
  SIGPIPE-killing the process (exit 141).
- After a commit, a write error never triggers any store call (FR-023, FR-031).
- `main` already installs `signal.NotifyContext(os.Interrupt)`, so SIGINT does not terminate the
  process mid-commit. The gate code ignores the context, so a started commit runs to completion and
  closes the locked directory descriptor.

**Residual**: SIGKILL, SIGTERM, or power loss during the commit can leave a leftover `*.tmp` file,
which is harmless (§5). The kernel releases the directory lock at process death, so no lock state
survives. In every case, the record is the complete prior pair or the complete new pair.

## §11 Timestamp and determinism

**Decision**:
- `decided_at` is `now().UTC()` formatted as RFC 3339 with second precision (`time.RFC3339`). It is
  taken once, after step 7 and before the commit.
- `now` is injected (`func() time.Time`): `time.Now` in production, a fixed clock in tests. Every
  other reported or recorded value is a pure function of the request and the prior content
  (FR-032).

## §12 Package placement and dependency boundary

**Decision**:

| Package | Owns | Direct imports, as an allow-list |
|---|---|---|
| `internal/gate` (new) | Decision types, request/report model, `Report.Valid()`, `Evaluate` (steps 3–9 control flow), the `Store` interface, the `Evidence` value | `io/fs`, `strings`, `time`, `ai-dev/internal/workflow`, `ai-dev/internal/sdd` |
| `internal/gatestore` (new) | The only ai-dev code that writes Workflow State: the directory lock, the under-lock re-validation, the record builder, and the atomic replace | `bytes`, `crypto/rand`, `encoding/hex`, `encoding/json`, `errors`, `fmt`, `io/fs`, `os`, `path`, `strings`, `ai-dev/internal/gate`, `ai-dev/internal/workflow`; plus `syscall` **only** in `lock_posix.go` (`//go:build (darwin && !ios) \|\| (linux && !android)`) |
| `internal/report` | New `workflowgate.go` renderer | (existing) plus `ai-dev/internal/gate` |
| `internal/app` | New `workflowgate.go` (usage steps 1–2, rendering, exit codes) plus the dispatcher | (existing) plus `ai-dev/internal/gate` |
| `cmd/ai-dev` | `main` constructs `gatestore.New(".")` | plus `ai-dev/internal/gatestore` |

AST boundary tests, in the style of 005's `boundary_test.go`, enforce the following:
- **`internal/gate`**:
  - it imports exactly its allow-list;
  - its only `sdd` selector is `MissingArtifacts`;
  - its only `workflow` selectors are `EvaluateTransition`, `State`, the canonical state and outcome
    constants, and `Abandoned`;
  - it never references `sdd.Evaluate` or anything from `agent`, `execution`, `selection`, `role`,
    or `system`.
- **`internal/gatestore`**:
  - it imports exactly its allow-list;
  - it is imported only by `cmd/ai-dev` and its own tests.
- **Sole write path (FR-003)**:
  - no non-test file in `internal/workflow`, `internal/sdd`, `internal/report`, `internal/role`,
    `internal/selection`, `internal/execution`, `internal/agent`, `internal/checks`, or
    `internal/doctor` imports `internal/gate` or `internal/gatestore`;
  - within `internal/app`, only `workflowgate.go` and the dispatcher reference `gate`.

The existing 004 and 005 boundary tests stay unchanged and keep passing. `internal/workflow` and
`internal/sdd` gain no new imports: the sdd wrapper uses only what `rules.go` already imports.

## §13 Supported-platform policy for Human Gate persistence

*Elevated to an approved product clarification (P-6) and recorded in spec FR-037 and SC-014 on
2026-10-07. This section records the technical rationale.*

**Decision**: Human Gate **persistence** (FR-026 steps 8–9) is supported on **macOS and Linux**, on
**local filesystems** that implement `flock(2)` and same-directory atomic `rename(2)`. Examples are
APFS and HFS+ on macOS, and ext4, xfs, btrfs, and tmpfs on Linux.
- **Code**: the real store (`lock_posix.go` and the commit path) builds only under
  `//go:build (darwin && !ios) || (linux && !android)`.
- **Fallback**: `new_other.go` uses the exact complement,
  `//go:build !((darwin && !ios) || (linux && !android))`.
- **Any other platform, including Windows, iOS, and Android**: `gatestore.New` returns an
  *unsupported-platform store*.
  - Its `Commit` performs **no filesystem access**. It returns `Failed` with the warning
    `ai-dev: warning: human gate persistence is supported only on macOS and Linux`.
  - The request therefore ends in `persistence_failed` with exit 1, and nothing is written.
- **Steps 1–7 on every platform**: usage, C-13, the 004 checks, and the 005 artifact check behave
  identically, so a caller still gets the true refusal outcomes.
- **ai-dev on Windows**: it continues to build, and every 001–005 command is unchanged.

**Platform-audit D-1 amendment (2026-10-08):** Go sets the `darwin` build tag for iOS and the
`linux` tag for Android. The earlier broad expressions therefore selected the POSIX store beyond
the approved macOS/Linux product scope. The corrected constraints above exclude those inherited
tags and remain exact complements. Claude's technical review returned PASS. This correction keeps
FR-037 and SC-014 fail-closed guarantees intact.

**Why Windows is not a 006 persistence target**: a *correct* Windows design needs three things
the standard library cannot reliably provide.
- **Locking**: it needs `LockFileEx` (from `golang.org/x/sys/windows`, a new dependency).
- **Atomic replace**: it needs a rename-with-POSIX-semantics (`SetFileInformationByHandle` with
  `FileRenameInfoEx` and `FILE_RENAME_FLAG_POSIX_SEMANTICS`) or `ReplaceFileW`. Go's `os.Rename`
  uses `MoveFileEx(MOVEFILE_REPLACE_EXISTING)`, which is not documented as atomic and fails while a
  reader holds the file open without `FILE_SHARE_DELETE`.
- **Verification**: a Windows test host. None is available in this repository; there is no CI and
  no Windows runner.

Accepting a non-atomic replace as a residual risk would violate FR-016/FR-020, so this plan does not
claim Windows support and fails closed there. A Windows strategy can be planned as a separate,
explicitly approved change.

**Network and FUSE filesystems**: they are not a supported target.
- If `flock` reports an error there, the outcome is `persistence_failed` (fail closed, §5).
- If a network filesystem silently grants host-local-only locks (for example, NFS mounted with
  `nolock`), concurrent writers on *different hosts* are not excluded. This is documented as a
  residual risk: concurrent multi-host gating on a shared filesystem is unsupported.

**Evidence for the chosen primitives**:
- **macOS**: verified empirically on macOS 27.0.1 with APFS and Go 1.27.1 (probe run 2026-10-06):
  - `flock(LOCK_EX|LOCK_NB)` on a directory descriptor obtained through `os.Root.Open` succeeds;
  - a second open file description in the *same process* gets `EWOULDBLOCK`, so in-process
    goroutines are excluded;
  - a child process holding the lock causes `EWOULDBLOCK`, and after the child is SIGKILLed, the
    lock is acquired immediately;
  - `Root.Rename` inside the locked directory succeeds;
  - `os.SameFile(dirFile.Stat(), root.Lstat(dir))` holds.
- **Linux**: the same semantics are documented by `flock(2)`: locks belong to the open file
  description and are released when the last descriptor closes. `flock(1)` from util-linux accepts
  directories. **Linux was not verified empirically during planning**, because the local Linux VM
  was not running. The plan therefore makes running the store test suite on Linux a **required
  pre-merge quality gate** (plan.md, Quality Gates).

**Platform-audit D-2 evidence status (2026-10-08):** H-3 is a historical PASS for the source tree
before the D-1 constraint correction. The existing environment, manifests, hashes, full-suite log,
and race log remain valid historical evidence, but they do not identify the corrected tree.
Renewed macOS/Linux SHA-256 manifests and renewed Linux full-suite and race logs are pending. H-7
native Windows evidence is `PENDING — DEFERRED`; it is not required to close the current
macOS/Linux scope and is not claimed as PASS.

**Later D-2 disposition (2026-10-08; history preserved):** the initial independent audit of the
renewed package returned FAIL with BLK-1. After BLK-1 was corrected, the follow-up audit returned
`D-2 EVIDENCE PASS` with no blockers. The responsible human then approved D-2 and accepted the
renewed H-3 macOS/Linux evidence; Claude supplied technical audit evidence and did not issue the
human approval. The accepted package is
`/Users/german/ai-dev-evidence/feature-006/h3-linux-d2-renewal-2026-10-08/`, with `SHA256SUMS`
SHA-256 `696e623eb98a67058644bf4935de363acf91b20a3382f7073c6cf21c42616b03` and matching 248-entry
macOS/Linux manifests whose SHA-256 is
`595b861b041b8f93b10479432206a8a69ba50d5bd8778bcb2ffd1cb6121aa21f`. NB-1 through NB-4 remain
non-blocking observations. H-7 remains `PENDING — DEFERRED`, with no native Windows validation, and
the Human Implementation Gate remains `NO APROBADO`.
The `595b861b…` identity belongs to the accepted snapshot before the later documentation changes
that record the human acceptance and post-acceptance audit corrections. Go production code and
tests remain identical to that snapshot; the current documentation differs.

**Later Human Implementation Gate decision (2026-10-08; prior history preserved):** the responsible
human for Feature 006 approved the implementation for the supported macOS/Linux scope after review
of the formal criteria and the previously accepted H-3/D-2 evidence. **Human Implementation Gate:
`APPROVED — macOS/Linux scope`.** H-7 remains `PENDING — DEFERRED`; native Windows behavior remains
unvalidated, no complete multiplatform validation is claimed, and FR-037/SC-014 retain their full
fail-closed guarantees. This decision does not approve H-7 or authorize merge or release.
