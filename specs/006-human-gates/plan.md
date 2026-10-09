# Implementation Plan: Human Gates

**Branch**: `feature/006-human-gates` | **Date**: 2026-10-06 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-human-gates/spec.md`. The Human Specification Gate
was approved by the human, with C-1–C-13 and D-1–D-3 decided.

## Summary

This plan adds ai-dev's only Workflow State write path, as two explicit, non-interactive commands:

```text
ai-dev workflow approve [--format text|json] --approver <identity> [--reason <text>] <feature-id> <target-state>
ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>
```

Each command records one human acceptance decision or one abandonment decision. Only when the
request passes every check does it persist exactly one 004 transition, together with durable,
append-only evidence of that decision.

**What 006 owns:**

| Concern | Where |
|---|---|
| Usage and decision-type consistency (C-13) | `internal/app` |
| Decision types, outcome model, and the control flow of steps 3–9 | new `internal/gate` package |
| The single piece of code that writes Workflow State | new `internal/gatestore` package |

**What 006 reuses unchanged:**

| Reused rule | How 006 uses it |
|---|---|
| 004's transition evaluation (FR-029 steps 2–5) | Called as a whole through the existing `workflow.EvaluateTransition` |
| 005's Required Artifacts rule | Called through a one-line exported wrapper, `sdd.MissingArtifacts` (minimal API extension, as with 005's `FeatureDir`) |

Neither the 004 rule set nor the 005 artifact table is copied.

**Persistence design:**
- **Evidence location.** Evidence is embedded in the existing `specs/<id>/workflow-state.json` as an
  append-only top-level `human_gates` array. 004's approved reader ignores unknown members, so it
  keeps resolving `state` unchanged.
- **Atomicity.** One `fsync`ed temporary file plus one `rename` makes state and evidence visible
  together, or not at all.
- **Stale-state protection.** A non-blocking kernel `flock(2)` on the feature directory's own
  descriptor serializes ai-dev writers. Under that lock, 004's own reader re-validates that the
  authoritative state still equals the evaluated state.
  - There is **no lock file**. The kernel releases the lock when the holder closes it or its process
    dies, so a crashed or post-commit-failed invocation can never leave the feature blocked.
  - An ambiguous lock error fails closed (remediated P-1, [research §5](./research.md)).
- **Platforms.** Persistence is supported on macOS and Linux (local filesystems). Every other
  platform fails closed with `persistence_failed` ([research §13](./research.md)).
- **Path safety.** All writes go through `os.Root`, so no path can escape the repository.

## Technical Context

**Language/Version**: Go 1.27.1, from the existing `go.mod`, unchanged.

**Primary Dependencies**: Go standard library only, with no new module. The new code relies on:
- `os.Root` (`OpenRoot`, `Open`, `OpenFile`, `Rename`, `Remove`, `FS`, `Lstat`, `ReadFile`), Go ≥ 1.25;
- `syscall.Flock` (darwin and linux only, behind a build tag);
- `encoding/json` token decoding;
- `crypto/rand` for the temporary-file name;
- `time`.

The per-package allow-lists are in [research §12](./research.md).

**Storage**:
- **Read**: the 004 Workflow State Record and one directory listing of `specs/<id>/`, through
  004/005 code.
- **Written**, only on commit and only inside `specs/<id>/`:
  - `workflow-state.json`, replaced atomically;
  - a transient `.workflow-state.<hex>.tmp`.

  See [contracts/human-gate-record.md](./contracts/human-gate-record.md).

**Testing**: `go test`, with white-box tests in each package, as in 001–005:
- `fstest.MapFS` fixtures, and recording and fault-injecting `fs.FS` wrappers;
- real `t.TempDir()` repositories for the store;
- `-race` in-process concurrency tests;
- multi-process concurrency and broken-pipe subprocess tests that use the built binary;
- an AST import and selector boundary test.

No agent or network access is involved.

**Target Platform**: **Human Gate persistence is supported on macOS and Linux only**, on local
filesystems that implement `flock(2)` and same-directory atomic `rename(2)` ([research §13](./research.md)).
- **Authority record (2026-10-08): human decision / human-approved scope amendment.** The human
  expressly authorized platform-audit D-1, the macOS/Linux-only persistence scope, H-7 as
  `PENDING — DEFERRED` and non-blocking for that scope, and preservation of FR-037/SC-014. Claude's
  technical PASS is independent review evidence and is not human authorization. This decision is
  not approval of the Human Implementation Gate, renewed H-3 evidence, merge, or release.
- On every other platform, including Windows, iOS, and Android:
  - ai-dev still builds, and 001–005 are unchanged;
  - `approve` and `abandon` run steps 1–7 identically;
  - every commit ends in `persistence_failed` (exit 1), with zero filesystem access in the store
    and a stderr notice.
- Windows atomic replace and locking are **not** claimed, and native Windows behavior has not been
  validated. A Windows strategy would need its own approved plan and completion of H-7.
- Network and FUSE filesystems are unsupported. A `flock` error there fails closed.
- Symlink, permission, SIGPIPE, lock-holder, and crash fixtures are macOS/Linux only, and skipped
  as root where permissions matter.

**Project Type**: A single Go module CLI (`cmd/`, `internal/`).

**Performance Goals**: Not performance-sensitive. A refusal performs at most:
- one `Lstat` of the feature directory;
- one `Lstat` and one read of the record;
- one `ReadDir`.

A commit adds:
- one directory open, non-blocking `flock`, and close;
- one re-validation;
- one temporary-file write and `fsync`;
- one rename;
- one directory `fsync`.

**Constraints**:
- No stdin, no prompts, no retries, and no background work.
- Nothing is persisted before step 9.
- Exit codes are 0, 1, and 2 only.
- 001–005 behavior is byte-identical.

**Scale/Scope**: One feature per request. The `human_gates` history grows by one element per applied
transition: at most 5 per feature. That is the 5 forward steps from `draft` to `approved`, or at
most 4 forward steps followed by one abandonment, because 004 permits no transition out of a
terminal state.

## Constitution Check (pre-design)

| Principle | Status | Evidence |
|---|---|---|
| I. Specification First | PASS | The spec is approved. The P-6 platform scope was raised as a product clarification and recorded in the spec (FR-037, SC-014) rather than decided in the plan. The plan introduces no product semantics beyond the spec. P-1–P-5 are approved. |
| II. Architecture Before Framework | PASS | Standard library only. The design follows from FR-016, FR-017, FR-020, and FR-025 (one file, so one atomic replace). |
| III. Simplicity and YAGNI | PASS, with justification | Two small packages; one transient tmp file beside the record; a kernel lock with no lock file. Each is justified in Complexity Tracking, with the simpler alternative stated. |
| IV. Testable by Design | PASS | Every FR and SC maps to automated tests (Test Strategy), including fault injection, concurrency, and subprocess tests. |
| V. Security by Design | PASS | Threats are enumerated below. `os.Root` confinement, no symlink following, and no secrets. Every residual risk has recorded human acceptance: the C-12 trust boundary (spec gate); P-3 power loss; the non-ai-dev writer race; network/multi-host coordination; a live hung holder; the Linux pre-merge gate (Human Plan Gate, 2026-10-07). The platform scope is a spec-level product decision (P-6, FR-037). |
| VI. Observability by Design | N/A (local CLI) | No backend or distributed component. Diagnosis comes from the outcome tokens, stderr diagnostics and warnings. No telemetry. |
| VII. Reproducibility and Automation | PASS | All gates are runnable with `go` commands (Quality Gates). No undocumented state. |
| VIII. AI-Assisted Engineering | PASS (process) | Implementation and the cross-agent review will be recorded in the PR, as for 001–005. |
| IX. Human Authority | PASS | This is the purpose of the feature: approval exists only as an explicit human-issued decision, it is never inferred, and evidence is recorded in version-controllable form. 006 claims no authentication (FR-033). |

## Project Structure

### Documentation (this feature)

```text
specs/006-human-gates/
├── spec.md                          # approved
├── plan.md                          # this file
├── research.md                      # §1–§12 planning decisions
├── data-model.md                    # entities, Report.Valid table, evaluation flow
├── quickstart.md                    # validation guide
├── checklists/requirements.md       # spec checklist (16/16)
└── contracts/
    ├── workflow-gate.md             # CLI: usage, outcomes, text/JSON, exit codes
    ├── human-gate-record.md         # record + human_gates schema, build rules, commit protocol
    └── sdd-missingartifacts.md      # minimal 005 API extension
```

### Source Code (repository root)

```text
cmd/ai-dev/
├── main.go                              # MODIFIED: call app.RunWithGateStore(..., gatestore.New("."), ...)
├── workflowgate_contract_test.go        # NEW: real-binary draft→approved walk, C-13 matrix, multi-process race
└── workflowgate_brokenpipe_test.go      # NEW (unix): closed stdout after commit → exit 2, commit kept

internal/gate/                           # NEW: domain + control flow, no OS writes
├── decision.go                          # Decision, Request
├── outcome.go                           # Outcome (4 reused as Outcome(workflow.X)), Report
├── evidence.go                          # Evidence value
├── store.go                             # Store interface, CommitStatus
├── evaluate.go                          # Evaluate: steps 3–9
├── validate.go                          # Report.Valid (FR-027 table)
└── *_test.go                            # evaluate/validate tests, boundary_test.go (AST allow-lists)

internal/gatestore/                      # NEW: sole Workflow State writer
├── store.go                             # New(dir) (selects posix or unsupported store), shared types
├── commit_posix.go                      # //go:build (darwin && !ios) || (linux && !android) — Commit: open dir → flock NB → bind → revalidate → build → tmp+fsync → rename → dir sync → close
├── lock_posix.go                        # //go:build (darwin && !ios) || (linux && !android) — flock wrapper (only file importing syscall)
├── new_other.go                         # //go:build !((darwin && !ios) || (linux && !android)) — New returns the unsupported-platform store
├── unsupported.go                       # untagged: unsupported-platform store (Failed + notice, zero FS access)
├── record.go                            # pure record builder (state replace + human_gates append)
├── ops.go                               # unexported ops seam (flock, open, lstat, rename, sync, close, afterStep hook)
└── *_test.go                            # builder; store on TempDir; lock holder/kill/ambiguous; crash at each step; post-commit; fault injection; -race; boundary

internal/sdd/rules.go                    # MODIFIED: + MissingArtifacts wrapper (one line)
internal/sdd/missingartifacts_test.go    # NEW: equivalence + single ReadDir

internal/report/workflowgate.go          # NEW: WriteWorkflowGateText / WriteWorkflowGateJSON
internal/report/workflowgate_test.go     # NEW

internal/app/workflowgate.go             # NEW: RunWorkflowGate (steps 1–2, render, exit codes, SIGPIPE helper)
internal/app/workflowgate_test.go        # NEW
internal/app/authority_test.go           # NEW: 001–005 commands never change record/evidence
internal/app/doctor.go                   # MODIFIED: Run → RunWithGateStore wrapper; +approve/abandon dispatch
```

No existing test file is edited. `internal/workflow` is not modified at all.

**Structure Decision**: This keeps the existing single-module layout. The domain (`gate`) is
separated from the OS writer (`gatestore`) so that the boundary tests can prove two things: the
domain performs no writes, and only one package writes Workflow State.

## Design

### Precedence as control flow (FR-026, exact C-13 order)

| Step | Where | Check | Result if it fails | Filesystem / store access |
|---|---|---|---|---|
| 1 | app | Flags and positionals; whitespace rules; set-once `--approver` and `--reason`; D-1; `abandon` requires `--reason` | usage failure, exit 2 | none |
| 2 | app | Decision-type consistency: `abandon` requires target exactly `abandoned`; `approve` requires target ≠ `abandoned` | usage failure, exit 2 | none |
| 3–6 | `gate.Evaluate` → `workflow.EvaluateTransition` | 004 FR-029 steps 2–5 | `target_unsupported`, `feature_unknown`, `state_invalid`, `transition_invalid`, exit 1 | 004's reads only |
| 7 | `gate.Evaluate` → `sdd.MissingArtifacts(root, id, target)` (approval only) | 005 rule for the target | `artifacts_missing`, exit 1 | one `ReadDir` |
| 8 | `gatestore.Commit` | Non-blocking `flock` on the feature directory: `EWOULDBLOCK` (active holder) gives conflict, and any other error gives a fail-closed `persistence_failed`. Then bind the lock to the path (replaced directory gives conflict). Then re-validate with 004 (state ≠ expected gives conflict). | `state_conflict` or `persistence_failed`, exit 1 | directory open, `flock`, 004 reads; no file created |
| 9 | `gatestore.Commit` | Build, then tmp + `fsync`, then `rename` (the commit point), then a post-commit directory flush, then close (which releases the lock) | `persistence_failed`, exit 1; otherwise `transition_applied`, exit 0 (post-commit failures are warnings only) | writes |
| 10 | app | Validate and render the report | output failure, exit 2. The commit stands, and the store is never called again. | stdout |

Because steps 1–2 run before step 3:
- `abandon … Abandoned` is a usage failure, not `target_unsupported`;
- `approve … Abandoned` reaches step 3 and gives `target_unsupported` (C-13 rule 3).

Step 7 follows step 6, so a replayed or invalid request reports `transition_invalid` even when
artifacts are missing (C-8, SC-004).

### Integration with 004 (reuse, not duplication)

- **Steps 3–6.** One call to `workflow.EvaluateTransition`. `transition_valid` is consumed only as a
  continue signal: it never becomes a report outcome, and it never stands in for the decision
  (FR-007).
- **Re-validation under the lock.** `workflow.FeatureExists` and `workflow.ResolveRecord` on
  `os.Root.FS()`, which implements `fs.ReadLinkFS`, so 004's no-follow path runs unchanged.
- **Write paths.** Built only from `workflow.FeatureDir(id)` plus the fixed names
  `workflow.RecordFileName` and `.workflow-state.<hex>.tmp`. 004 already validated the ID at step 4.
- **Written record.** It always contains exactly one exact `state` member with a canonical value.
  This is proven by tests that run the real `workflow.ResolveRecord` over the builder output, not by
  a second parser.
- **Unchanged.** `internal/workflow` is not modified, and its boundary and regression tests stay
  untouched.

### Integration with 005 (reuse, not duplication)

- A single new exported wrapper, `sdd.MissingArtifacts`
  ([contract](./contracts/sdd-missingartifacts.md)). 006 never calls `sdd.Evaluate`, `nextStep`, or
  `requiredArtifacts`, and never builds an artifact path itself.
- `abandonment` skips step 7 entirely (C-10).
- `artifacts_missing` is a distinct 006 token. 005's `artifacts_inconsistent` is untouched.

### Persistence, atomicity, stale state (FR-016–FR-023)

The full protocol is in [contracts/human-gate-record.md](./contracts/human-gate-record.md). In
summary:
1. **Lock (remediated P-1).** The store opens `specs/<id>/` through `os.Root` and takes
   `flock(LOCK_EX|LOCK_NB)` on that directory descriptor. No lock file exists.
   - `EWOULDBLOCK` means a demonstrably live holder, so the outcome is `state_conflict`.
   - Any other error means exclusivity cannot be established, so the outcome is
     `persistence_failed`. ai-dev never removes, breaks, or retries a lock.
   - A dead holder's lock is released by the kernel, so recovery is automatic and immediate.
   - The lock is then bound to the path: the directory must not have been replaced
     (`os.SameFile`).

   The rationale, the comparison of alternatives A–I, and the full case analysis are in
   [research §5](./research.md).
2. **Re-validate.** Under the lock, 004 resolves the record again. A missing feature, an invalid
   record, or a state that differs from the evaluated state gives `state_conflict` (FR-017).
3. **Build.** The pure builder replaces `state` and appends one `human_gates` element. All other
   bytes and members are preserved. A `human_gates` member that is not an array, is duplicated, or
   has a case variant gives `persistence_failed` (D-2).
4. **Write.** The bytes go to a tmp file created with `O_EXCL`, followed by `Sync` and `Close`.
5. **Commit.** `Rename(tmp, workflow-state.json)` is the only commit point. Any failure before it
   leaves the record byte-identical and gives `persistence_failed`.
6. **Post-commit.** `Sync` the locked directory descriptor (P-3), then `Close` it, which releases
   the lock. Their failures produce stderr warnings and never change `transition_applied` (FR-023).
   Neither can leave the feature blocked: if `Close` fails, or the process dies first, the kernel
   still releases the lock when the process exits.

**Platform scope.** Steps 1–6 above exist only under
`(darwin && !ios) || (linux && !android)`. Every other platform—including Windows, iOS, and
Android—compiles the fallback selected by
`!((darwin && !ios) || (linux && !android))`. It returns `Failed`, which gives
`persistence_failed` plus a stderr notice, and it touches no file (P-6,
[research §13](./research.md)).

### Evidence (FR-024, FR-025)

- **Fields.** Each `human_gates` element has exactly the keys `feature_id`, `decision`, `approver`,
  `from_state`, `to_state`, `decided_at`, and `reason`. `reason` is present if and only if one was
  supplied.
- **Approver.** Retained verbatim. It is never trimmed, looked up, or taken from the environment,
  git, or the OS (FR-006, FR-033).
- **Timestamp.** `decided_at` is UTC RFC 3339 from an injected clock.
- **History.** Append-only; the builder never rewrites an earlier element.

### Residual trust boundary (FR-033)

The following holds in implementation and documentation:
- The text report says, every time: "approver identity is not verified".
- No code path reads the approver from anywhere other than `--approver`.
- No ai-dev code invokes `workflow approve` or `workflow abandon`; the AST boundary tests enforce
  this.
- ai-dev cannot stop an external agent with shell access from running the command. This is the
  residual accepted at the spec gate (C-12).

### CLI and dispatcher

- **New entry point.** `app.RunWithGateStore` adds `workflow approve` and `workflow abandon` to the
  existing dispatcher. `app.Run` keeps its signature and delegates with a `nil` store (**P-2**). A
  `nil` store makes the gate commands report an internal error and exit 2, with no filesystem
  access.
- **Unchanged diagnostic.** The `Usage: ai-dev workflow <state|transition> [options]` line is left as
  it is, as 005 did (FR-035).
- **Signals.** `RunWorkflowGate` uses the existing `brokenPipeAsError()` helper, so a closed stdout
  gives exit 2, not 141. `main`'s existing `signal.NotifyContext` keeps SIGINT from terminating a
  commit. Any other termination, such as SIGKILL, releases the lock in the kernel (research §5,
  case analysis).

## Quality Gates

| Gate | Command / evidence | Pass condition |
|---|---|---|
| Format | `gofmt -l .` | no output |
| Vet | `go vet ./...` | exit 0 |
| Build | `go build ./...` | exit 0 |
| Tests | `go test ./...` | all packages ok. Every pre-existing test passes unedited. |
| Race / concurrency | `go test -race -count=20 ./internal/gate/ ./internal/gatestore/` | ok |
| Subprocess contract | `go test ./cmd/ai-dev/` | ok, including the multi-process race and broken-pipe tests |
| **Linux store gate (required pre-merge)** | `go test ./... && go test -race -count=20 ./internal/gate/ ./internal/gatestore/` on Linux (local ext4, xfs, or tmpfs), output recorded in the PR | ok, including the lock-holder, kill-recovery, crash, and post-commit tests. H-3 first passed for the pre-amendment source tree. Platform-audit D-2 then required renewal for the corrected tree; after an initial FAIL and BLK-1 correction, the independent follow-up returned PASS and the human accepted the renewed evidence on 2026-10-08. Attaching the renewed D-2 package (and the historical H-3 package) to the PR remains PENDING until a PR exists. |
| Unsupported-platform build (human-approved T049 amendment) | Execute the three-stage procedure in [tasks.md](./tasks.md) as a separate script from the repository root using Bash 4.4 or later; macOS `/bin/bash` 3.2 is unsupported. Stage 1 cross-builds all production packages; Stage 2 vets all packages except `ai-dev/cmd/ai-dev` and `ai-dev/internal/system` (including their Windows-compatible tests); Stage 3 vets Windows-selected production files in those two packages using `go list`. Checked command substitutions make any `go list` failure fatal. | All three stages exit 0; fallback store compiles where `syscall.Flock` is absent. The original Windows `go vet ./...` failure predates Feature 006 and was independently reproduced against clean HEAD `666abd7`: `cmd/ai-dev/main_test.go:171` and `internal/system/oslauncher_test.go:164` use `syscall.Kill`. These legacy test files are outside Feature 006's modification scope under FR-035 and T042. No production-code validation or analyzable Feature 006 tests are waived. `set -euo pipefail` fails the gate if any stage fails. This human-approved validation-procedure amendment does not change functional requirements, security guarantees, or the human authority model, and does not claim Windows persistence support. |
| Platform selection amendment D-1 | Structural boundary test plus `go list` selection checks and representative target-compilation checks for macOS, Linux, iOS, Android, and Windows | macOS and Linux select only the POSIX store; iOS, Android, Windows, and every other unsupported platform select only the fallback. For iOS, this evidence establishes package/file selection and compilation only; it does not claim a linked, installable, or runnable iOS executable. Claude's technical review returned PASS as independent evidence, not authorization. Native Windows runtime validation remains H-7 `PENDING — DEFERRED`. |
| Boundary | `go test ./internal/gate/ ./internal/gatestore/ ./internal/sdd/ -run Boundary` | allow-lists hold; sole write path holds |
| Regression | `git diff --stat develop -- '*_test.go'` | only new test files; no existing test file modified |
| Security / scope audit (manual review of the diff) | Reviewer checklist below | all items confirmed |
| Dependencies | `git diff develop -- go.mod go.sum` | empty |

**Procedure correction (2026-10-08):** the Regression and Dependencies gates above name `develop`,
the actual base of Feature 006; `main` does not exist in this repository. This is a documentary
correction to the recorded procedure, not a new execution of tests or validation commands.

**Security / scope audit checklist**:
- [ ] There are no writes outside `internal/gatestore`.
- [ ] Every write goes through `os.Root`.
- [ ] There is no symlink following.
- [ ] The approver is never sourced from the environment, git, or the OS.
- [ ] There is no stdin or prompt.
- [ ] There is no retry, wait, or background goroutine outliving the request.
- [ ] The lock is only `flock(LOCK_EX|LOCK_NB)` on the feature-directory descriptor. No lock file
      is created, and no lock is ever removed, broken, or retried.
- [ ] Only `EWOULDBLOCK` maps to `state_conflict`; every other `flock` error maps to `persistence_failed`.
- [ ] `syscall` is imported only in `lock_posix.go`, behind
      `//go:build (darwin && !ios) || (linux && !android)`.
- [ ] Agent execution, role selection, and `sdd.Evaluate` are never invoked.
- [ ] There is no "authenticated" or "verified" claim.
- [ ] Nothing is written to stdout on a usage failure.
- [ ] No existing 001–005 output changed.

## Test Strategy

The full seams are in each contract. Each test maps to the spec as follows:

| Spec target | Test(s) |
|---|---|
| US1, SC-001, FR-014, FR-019 | **gatestore + app**: for each of the 4 forward transitions from `draft` to `implemented`, plus `implemented` → `approved`, on a TempDir repository, get `transition_applied` / exit 0. Then `workflow.EvaluateState` reports the target, and `sdd.Evaluate` reflects it (next activity, or `workflow_terminal`). **cmd**: a real-binary walk from `draft` to `approved`. |
| US1 AS4–AS5, SC-008, SC-012, FR-024, FR-025 | **Builder**: property test over append sequences, where earlier elements are byte-equal after compaction. **Store**: element fields exactly match the request; `reason` is absent when not supplied; there are no extra keys. |
| US2, SC-002, FR-003, FR-006 | **`authority_test.go`**: snapshot a fixture repository. Run `doctor`, `agents`, `agent run` (with a fake launcher reporting `agent_succeeded` that creates `plan.md`), `agent select`, `workflow state`, `workflow transition` (`transition_valid`), and `workflow next` (`step_recommended`). The record must be byte-identical with no `human_gates` entry. **AST**: only `app/workflowgate.go` references `gate`, and only `cmd/ai-dev` imports `gatestore`. |
| US2 AS4, SC-007, FR-022 | A replayed identical request gives `transition_invalid` / exit 1; the record is byte-identical and the history length is unchanged. |
| US3, SC-003, FR-012–FR-015, FR-021 | **gate**: a table with a recording `fs.FS` and a fake store covering every refusal class. The structurally invalid IDs `..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`, a symlinked directory, every `state_invalid` record kind from 004's table, skip, backward, same-state, terminal, and missing artifacts. Each refusal is checked for **zero store calls** and the correct report. `target_unsupported` is checked for zero filesystem calls. |
| SC-004 | A request that is both 004-invalid and missing artifacts gives `transition_invalid`, with zero `ReadDir` calls. |
| US4, SC-005, FR-016–FR-018, FR-020 | **gatestore fault injection** at `OpenRoot`, the directory open, read, build, tmp write, `Sync`, `Close`, rename, the directory `Sync`, and the descriptor `Close`: status as specified; record byte-identical for every pre-commit failure; tmp file absent afterwards; warnings only after commit. Also a malformed `human_gates` record (D-2) and a read-only directory. |
| Lock R1 (active holder) | An in-process holder (the test holds `flock` on the directory), and a cross-process holder (a helper process holds it and signals over a pipe), each give `state_conflict`, with the record byte-identical and no file created. |
| Lock R2 (stale, automatic recovery) | The helper holder is SIGKILLed and reaped; the next `Commit` gives `Committed` with no ai-dev or human action. |
| Lock R3 (ambiguous, fail closed) | The `ops.flock` seam returns `ENOLCK`, `EOPNOTSUPP`, `EINVAL`, and `EINTR`: each gives `persistence_failed`, with nothing written or removed and exactly one `flock` call. A binding mismatch (different directory identity, or a symlink) gives `state_conflict`. |
| Lock R4 (crash during commit) | A helper process is SIGKILLed via the `ops.afterStep` pause hook after the lock, after the tmp write, after the rename, and after the directory `Sync`. 004 then resolves the complete prior state (kills before the rename) or the complete new state with exactly one appended entry (kills after it), never `state_invalid`, and the next `Commit` from the current state succeeds. |
| Lock R5 (post-commit leftovers) | The directory `Sync` fails, `Close` fails, or both: each gives `transition_applied` (exit 0) plus warnings, and an immediate second gate from the new state gives `transition_applied`. A regular file named `.workflow-state.lock` is ignored. |
| P-6 / FR-037 / SC-014 platform policy | The unsupported-platform store (untagged file, tested on every host) gives `Failed` plus the notice, with zero filesystem calls. `app` maps it to `persistence_failed` / exit 1. The `GOOS=windows` cross-build gate compiles it in. |
| SC-006, FR-017 | Concurrency: 16 goroutines (`-race`, ×20) and 8 processes (the real binary), giving exactly one `transition_applied`, with the rest `state_conflict` and one appended element. A state changed between step 6 and step 8 (injected through the fake store, and through the real store with the record edited before commit) gives `state_conflict`. |
| US5, FR-008–FR-011 | **app**: the C-13 matrix (`approve`/`abandon` × `abandoned`/`Abandoned`/`planned`/`reviewing`); `abandon` without a reason or with a whitespace reason → exit 2; `approve` with a whitespace reason → exit 2; abandonment from each non-terminal state with no artifacts present → `transition_applied`; abandonment from a terminal state → `transition_invalid`. |
| FR-026 | A step-order table in which each fixture violates two steps at once, asserting that the earlier step's result wins and that later steps make no filesystem or store calls. |
| FR-027–FR-029, SC-009, SC-010 | **report**: every outcome in text and JSON; the field-presence table is enforced by `Report.Valid`; `%q` quoting of an ID and approver containing a newline, quotes, or `outcome=transition_applied`; the forbidden-phrase scan. **app**: the exit code for each outcome; nothing on stdout for usage failures. |
| FR-031, US4 AS5 | A failing writer after `Committed` gives exit 2, the store is called exactly once, and the record stays committed. A Unix subprocess with a closed stdout gives exit 2, not 141, and `workflow state` then reports the target. |
| FR-005, SC-011 | stdin is a reader that fails the test if read. With a fixed clock, repeated runs over identical fixtures give byte-identical output and record bytes. |
| FR-033, FR-034 | No environment variable is consulted. A test sets `USER`, `GIT_AUTHOR_NAME`, and similar variables to sentinels and asserts that they never appear in output or evidence. |
| SC-013, FR-035 | The entire existing suite passes unedited, and the existing `cmd/ai-dev` contract tests keep passing. |
| `sdd.MissingArtifacts` | Equivalence with `missingArtifacts`; exactly one `ReadDir`; the existing sdd boundary test is unchanged. |

## Compatibility and Regression Strategy (001–005)

- **Untouched**: `internal/workflow`; all 004/005 commands, their outputs, and their exit codes;
  every existing test file.
- **Additive changes only**:
  - `internal/sdd`: one exported function;
  - `internal/app`: `Run` delegates through a wrapper, and two new verbs are added;
  - `cmd/ai-dev/main.go`: one call change.
- **Records written by 006** are read by 004's existing `ResolveRecord`, and by 005 through 004, with
  no change to either reader. Existing features with no record keep reading as `draft` until a human
  approves them.

## Plan decisions and their Human Plan Gate status

**Status (2026-10-07)**: P-1 (remediated), P-2, P-3, P-4, and P-5 are **approved**. P-6 was elevated to an approved **product clarification** and is now recorded in the spec (Clarifications, Session 2026-10-07; FR-037; SC-014).

These are implementation-level choices within the approved semantics. They are listed so that they
can be accepted explicitly.

- **P-1 (remediated after the Human Plan Gate review) — a kernel `flock(2)` on the feature
  directory, with no lock file** ([research §5](./research.md)):

  | Situation | Behavior |
  |---|---|
  | Active holder (`EWOULDBLOCK`) | `state_conflict` |
  | Holder exited or killed | The kernel has already released the lock, so the next request proceeds (automatic, deterministic recovery) |
  | Any other `flock` error (ambiguous) | `persistence_failed`. Nothing is written, removed, or retried. |
  | Post-commit flush or close failure | `transition_applied` plus a warning. The lock is released by `close`, or at process exit at the latest, so the feature is never poisoned. |

  Rejected alternatives: an `O_EXCL` lock file plus a human, a PID, a PID plus start time, a
  timestamp lease, or a heartbeat; `fcntl` record locks; a persistent lock file plus `flock`; and
  Windows `LockFileEx`. The comparison table is in research §5.
- **P-2 — `app.Run` keeps its signature and delegates to the new `RunWithGateStore` with a `nil`
  store.** This avoids editing 28 existing test call sites. As a consequence, `Run` itself reports
  an internal error (exit 2) for approve and abandon; only `main` wires the real store.
- **P-3 — Durability boundary.** Success is reported after the file `fsync` and the `rename`. The
  directory `fsync` is post-commit and best-effort. Power loss in that window may revert to the
  complete prior pair, never a partial one.
- **P-4 — Unknown top-level record members are preserved verbatim.** Malformed or ambiguous
  `human_gates` content refuses with `persistence_failed` (D-2) rather than being repaired.
- **P-5 — The minimal 005 API extension, `sdd.MissingArtifacts`.** It mirrors the approved
  `workflow.FeatureDir` precedent.
- **P-6 — Supported-platform policy. It is a product clarification, specified in spec FR-037 and
  SC-014; this plan implements it.** Human Gate persistence is supported on macOS and Linux local
  filesystems only. The POSIX store is selected by
  `(darwin && !ios) || (linux && !android)`; its exact complement selects the fallback. Every other
  platform, including Windows, iOS, and Android, builds and runs steps 1–7
  normally, but every commit ends in `persistence_failed` with zero writes and a stderr notice.
  - Windows atomic replace is **not** claimed and is not accepted as a residual risk. A correct
    Windows strategy needs a new dependency (`x/sys/windows`), different rename primitives, and a
    Windows test host, so it is deferred to a separately approved change ([research §13](./research.md)).
  - **Spec mapping**:
    - Spec FR-037 and the FR-026 step 8 note define the behavior.
    - The spec's Success Criteria applicability note limits the commit-dependent criteria (SC-001,
      SC-005–SC-008, SC-012, and the post-commit clause of SC-010) to supported platforms.
    - SC-014 defines the fail-closed behavior elsewhere.
    - FR-037's "primitives cannot be established safely" clause is implemented by mapping every
      `flock` error other than `EWOULDBLOCK` to `persistence_failed` (P-1).
  - **Platform-audit D-1 amendment (2026-10-08):** by human decision / human-approved scope
    amendment, macOS and Linux are the only supported persistence platforms, H-7 is deferred and
    non-blocking for that scope, and FR-037/SC-014 retain their complete fail-closed guarantees.
    Claude's technical PASS for the corrected, mutually exclusive constraints and five-platform
    selection checks is independent evidence, not authorization. This post-completion correction
    does not reopen T013–T018 or approve any Human Gate.
  - **Native Windows limitation:** H-7 is `PENDING — DEFERRED` and non-blocking for the current
    macOS/Linux scope. Reactivate it before distributing or claiming Windows persistence support,
    before changing the Windows fallback behavior, or when native Windows validation becomes a
    release requirement.

### Risks (residual; historical decisions and later amendments)

Rows whose status records acceptance on 2026-10-07 preserve the Human Plan Gate history as of that
date. Rows and annotations explicitly dated 2026-10-08 are later scope/evidence amendments; they do
not convert a historical acceptance into a new acceptance or approve the Human Implementation Gate.

| Risk | Impact | Mitigation | Status |
|---|---|---|---|
| A writer that ignores the advisory lock (a hand edit or another tool) writes the record between step 4's re-validation read and step 8's `rename` | That change is replaced by a record built from step 4's bytes | The window is the build plus the tmp write and `fsync` (milliseconds at most). Writes before it are detected (`state_conflict`) or preserved; writes after it win. Manual edits are outside ai-dev's control (C-5). The evidence history makes the overwrite visible in review. A content-conditional replace does not exist on POSIX. | Accepted (Human Plan Gate, 2026-10-07) |
| A live but hung ai-dev holder (for example, a stalled `fsync` on a failing disk) | Other gates for that feature report `state_conflict` while it lives | This is correct behavior: the holder is demonstrably alive (R1). Killing it releases the lock in the kernel, with no file to clean up. | Accepted by design |
| Network filesystems whose `flock` is host-local | Concurrent gates on *different hosts* are not excluded | Unsupported by policy (P-6). Locking errors fail closed. | Accepted (P-6 product clarification, spec FR-037) |
| Linux `flock`-on-directory semantics not verified empirically during planning (only macOS/APFS was probed) | A semantic difference would surface only on Linux | `flock(2)`/`flock(1)` documentation; the **required** Linux pre-merge gate runs the full lock and crash suite | Accepted (Human Plan Gate, 2026-10-07). The Linux run remained a required pre-merge quality gate. |
| Power loss before the directory `fsync` | The applied decision may be lost, and the complete prior pair observed | File data is fsynced before the rename. There is never a partial record. | Accepted (P-3) |
| Windows (and other non-macOS/Linux platforms) cannot persist Human Gates | `approve` and `abandon` always end in `persistence_failed` there | Fails closed with an explicit notice. A non-atomic replace is never attempted. A Windows strategy is deferred to its own approved plan. | Accepted (P-6 product clarification, Human Plan Gate, 2026-10-07; spec FR-037) |
| An agent or script invokes `workflow approve` | Approval asserted by a non-human | Accepted at the spec gate (C-12). The report always says the identity is not verified. The evidence is reviewable. | Accepted (C-12) |
| A leftover `*.tmp` after a crash | Clutter only | Ignored by 004, 005, and git (`*.tmp`) | Accepted |
| Platform-audit D-1 extension to iOS/Android and native Windows H-7 deferral (2026-10-08) | Go's inherited `darwin`/`linux` tags could otherwise select the POSIX store beyond the approved product scope; native Windows behavior remains unvalidated | Exact complementary constraints route iOS, Android, Windows, and all other unsupported platforms to the fail-closed fallback; preserved H-7 criteria define future native Windows validation | Human-approved scope amendment (2026-10-08). H-7 is `PENDING — DEFERRED`; this is not a new acceptance of the 2026-10-07 Windows risk. |
| Platform-audit D-2 evidence renewal after the build-constraint correction (2026-10-08) | The historical H-3 PASS identifies the pre-D-1 source tree, not the corrected tree proposed for closure | Regenerate reproducible macOS/Linux SHA-256 source manifests, rerun the Linux full and race suites, retain hashes, and attach durable evidence to the PR | **Initial status:** `EVIDENCE RENEWAL PENDING (D-2)`. **Later status, 2026-10-08:** initial independent audit FAIL; BLK-1 corrected; follow-up `D-2 EVIDENCE PASS`; renewed H-3 evidence human-accepted with NB-1–NB-4 and known limitations preserved. Attaching the renewed D-2 package and the historical H-3 package to the PR remains **PENDING** until a PR exists and the attachment is actually performed. |

**Post-2026-10-07 Linux annotation (2026-10-08):** H-3 obtained independent technical PASS for the
pre-D-1 source tree. That result resolves the original empirical uncertainty for that tree but does
not rewrite its 2026-10-07 acceptance record. D-2 separately requires renewed evidence after the
constraint correction; Claude's PASS is technical review evidence, not human authorization or
acceptance of the renewal.

**D-2 review and decision chronology (2026-10-08):** the first independent evidence audit returned
FAIL with BLK-1. The evidence package was corrected, and the follow-up audit returned
`D-2 EVIDENCE PASS`, with BLK-1 resolved and no blockers. The responsible human—not Claude—then
approved D-2 and accepted the renewed H-3 evidence for macOS/Linux. The accepted package is
`/Users/german/ai-dev-evidence/feature-006/h3-linux-d2-renewal-2026-10-08/`; its `SHA256SUMS` has
SHA-256 `696e623eb98a67058644bf4935de363acf91b20a3382f7073c6cf21c42616b03`, and both source
manifests have SHA-256 `595b861b041b8f93b10479432206a8a69ba50d5bd8778bcb2ffd1cb6121aa21f`.
NB-1 through NB-4 remain non-blocking observations and are not reclassified or erased. H-7 remains
`PENDING — DEFERRED`, and the Human Implementation Gate remains `NO APROBADO`.
The `595b861b…` manifest identifies the human-accepted snapshot before the later documentation
changes that record the acceptance and subsequent audit corrections. Go production code and tests
remain identical to that snapshot; the current documentation differs. Attaching the renewed D-2
package (and the historical H-3 package) to the PR remains PENDING until a PR exists.

**Later Human Implementation Gate decision (2026-10-08; prior history preserved):** the responsible
human for Feature 006 explicitly approved the implementation for the supported macOS/Linux scope.
**Human Implementation Gate: `APPROVED — macOS/Linux scope`.** The previously accepted H-3/D-2
evidence supports this limited scope. H-7 remains `PENDING — DEFERRED`, native Windows behavior is
not validated, and no complete multiplatform validation is claimed. FR-037 and SC-014 remain
unchanged and continue to require fail-closed behavior on every unsupported platform. This approval
does not approve H-7 or authorize a PR, merge, release, or another Human Gate.

## Constitution Check (post-design)

| Principle | Status | Note |
|---|---|---|
| I | PASS | No product semantics changed. C-13 precedence is implemented exactly (steps 1–2 in the app, before 004). |
| II | PASS | Standard library; the design is driven by FR-016, FR-017, FR-020, and FR-025. |
| III | PASS | Complexity is justified below. |
| IV | PASS | Every FR and SC has a mapped automated test. Fault injection and concurrency cover the risky paths. |
| V | PASS | Threats are covered: traversal (`os.Root`), symlinks (no-follow throughout, plus directory-identity binding), spoofed identity (C-12 residual), secrets (none emitted; environment sentinels tested). The approved P-1 removes persistent lock state, so no stale-lock or poisoning risk remains, and ambiguous lock errors fail closed (FR-037). Unsupported platforms fail closed with zero writes (FR-037, SC-014). All residual risks in the table above are accepted by the human. The Linux run remains a required pre-merge quality gate, not a planning-time check. |
| VI | N/A | Local CLI; stderr diagnostics only. |
| VII | PASS | All gates are reproducible via `go`. |
| VIII | PASS (process) | Cross-agent review to be recorded in the PR. |
| IX | PASS | Approval exists only from explicit human-issued commands and is recorded as version-controllable evidence. Nothing is inferred, and no authentication is claimed. |

## Complexity Tracking

| Added complexity | Why needed | Simpler alternative rejected because |
|---|---|---|
| New package `internal/gatestore`, separate from `internal/gate` | Proves by AST test that exactly one package writes Workflow State (FR-003) and that the domain is write-free | Folding it into `gate`: the boundary would then rely on review alone, not on an enforceable test |
| Kernel `flock` on the feature directory in the POSIX store selected by `(darwin && !ios) || (linux && !android)`, plus the exact-complement unsupported-platform store | FR-017 and SC-006: without a lock, two ai-dev processes can both pass re-validation and both rename, losing one decision's evidence. A kernel lock adds no persistent state, so it cannot go stale (Human Plan Gate requirements R1–R5). | Re-read without a lock: race. An `O_EXCL` lock file: stale locks poison the feature. PID, start time, or lease breaking: unprovable liveness, and break races. A cross-platform lock: needs `x/sys` and an unverifiable Windows path. |
| Temporary file plus `rename` | FR-016 and FR-020: no partial record may ever be observable | In-place write: partial content becomes visible on a crash |
| Order-preserving token-walk builder | FR-025 and P-4: earlier evidence and unknown members must survive unchanged | Struct unmarshal/marshal drops unknown members and can reorder them |
| `RunWithGateStore` wrapper | Avoids non-additive edits to 28 existing test call sites | Changing the `Run` signature would edit approved 001–005 test files |
