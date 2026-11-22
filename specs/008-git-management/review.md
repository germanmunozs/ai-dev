# Feature 008 — Independent Review Record (T118)

**Date:** 2026-11-18. **Branch:** `feature/008-git-management`. **Constitution:** principle VIII (AI-Assisted Engineering).

## Roles and separation

| Role | Identity |
|---|---|
| Owner | German Munoz |
| Implementing agent | Claude Opus 5.5 (`claude-opus-5-5`), the session that implemented Phases 1–13 |
| Independent reviewing agent | A separate Claude agent started with no context from the implementing session, read-only, instructed to be adversarial and to confirm every finding by reading code or running a test |

**Separation exception.** The reviewer is a different agent instance, but the same model family and the same tooling host. It shared no conversation context, was not allowed to modify files or run mutating Git commands, and ran its mutation experiments only on a scratch copy, which it deleted afterwards. Separation by a different human or a different model is not claimed.

**Authority.** The owner asked for this review and delegated the pending decisions on 2026-11-18 ("te autorizo a realizar lo que tu estimes que es mejor actuando como arquitecto de software senior"). This record grants no Human Gate, Workflow State transition, merge or remote authority by itself. The local Git Flow finish of this feature was requested separately by the owner in the same message.

## Scope

Security and recovery of Git operations:
- Git process isolation: `internal/gitprocess`.
- The safety kernel and the sole terminal closure: `safety.go`, `terminal.go`, `apply.go`, `recovery.go`, `reconcile.go`.
- The index.lock protocol and the no-overwrite configuration publication, with the I/O seam: `index_posix.go`, `projectconfig_posix.go`, `fileops_posix.go`.
- The durable store: `internal/gitstore`.
- The Phase 13 changes:
  - interrupt policy INT-01;
  - reference-lock observation LCK-01;
  - configured-branch adoption REC-05.
- Secret handling.
- Whether the new tests prove what they claim.

## Findings and resolutions

| ID | Sev | Finding | Resolution | Evidence |
|---|---|---|---|---|
| R-01 | medium | `TestRealEngineInterrupts` accepted `applied` from any point and also passed with INT-01's between-operation check removed. No test ran `Apply` with a cancelled context. | **Fixed.** `TestApplyInterruptPolicy` drives `Apply` deterministically: a cancel before the claim, during the first operation and during the last one, and every operation must receive a context that is not cancelled. The real test now also interrupts at issuance, at the apply lock and at the claim CAS. It accepts `applied` only from the last operation's start on, and requires both a clean refusal and an interrupted mutation. | Removing the check fails 100 of the real cases and the unit test. Removing `WithoutCancel` fails the unit test. With the code in place: 168 real cases, 18 refused, 138 `partially_applied`/`interrupted`, and 12 `applied`, all within the last operation. |
| R-02 | low | Interrupts before the claim were reported as drift (`plan_stale` or `concurrent_change`), and `verifyApplied` could turn an applied plan into `authorization_consumed`. | **Fixed.** Every refusal while the caller is cancelled becomes `refused`/`interrupted`, with the original reason kept in the detail. An explicit check now runs before the claim, and `verifyApplied` runs under `WithoutCancel`. | The `before the claim` unit case and the 18 real pre-claim refusals, each followed by a successful later consumption. |
| R-03 | low | Once SIGTERM was captured, a hung Git or a blocked terminal read could no longer be stopped by SIGINT or SIGTERM. | **Fixed.** After an interrupt has been deferred, a second SIGINT, SIGTERM or SIGHUP exits at once with 130 and prints a pointer to `ai-dev init reconcile`; that is the crash path. SIGHUP (terminal closed) is now an interrupt too. | `TestRealEngineSecondInterruptExits`: exit 130, then the crash checks hold and the linked reconciliation is applied. SIGHUP was added to the 168-case matrix. |
| R-04 | low | The mutating rows discovered their repository from the project root, so a vanished `.git` could redirect a write to an enclosing repository. | **Fixed.** Every non-`init` mutating row runs with `GIT_DIR=<project>/.git`. | `TestWriteRowsNeverDiscoverAnEnclosingRepository`. Without `GIT_DIR`, `config --local` writes to the parent. |
| R-05 | low | `referenceLocks` could report "no locks" from an incomplete walk: symlinked `refs` or subdirectories, a linked worktree's own `refs`, or an entry that vanished during the walk. | **Fixed.** Only an absent `refs` directory is empty. A symlink, a non-directory `refs` or any walk error makes the observation incomplete, which is `lock_conflict`. A linked worktree's `refs` is walked too. | `TestReferenceLockObservation`, five cases. |
| R-06 | low | The configuration was read back by path, following symlinks, and linked without proving that the path was still the created inode. | **Fixed.** The read-back is by descriptor with `O_NOFOLLOW` and must be the created inode, and the path is checked against that inode again immediately before `link`. | `TestProjectConfigNotPublishedAfterSwap`. A swap after the check publishes nothing and leaves the foreign file. |
| R-07 | low | The reference-lock check in `Revalidate` was never exercised. | **Fixed.** The integration test calls `Revalidate` for every operation with a lock present. | Removing the check makes the test fail. |

**Verdict of the reviewer:** "No blocking findings." All seven findings are resolved, each with a test shown to fail without its fix.

## Areas the reviewer found sound

- **Git isolation:** exact argv with no shell; a fully explicit environment; every hook, credential, editor, pager, SSH, protocol, fsmonitor and GPG override; neutralized filter drivers; bounded capture that ends as indeterminate; the private HOME/XDG leaf proven absent; and T130's live-hook control.
- **Process group:** Git runs in its own group, and the apply-lock descriptor is close-on-exec.
- **Kernel:**
  - the claim is reread when its outcome is unverifiable;
  - intent is durable before `mutation_started`;
  - the claim is released only while no intent can be durable;
  - there is a single closer with a bound-digest CAS, and no rollback or retry;
  - `classifyEffects` never claims an unproven success or failure;
  - the `render` fallback is indeterminate after intent;
  - the store ignores cancellation.
- **Index protocol, durable store, configured-branch adoption and secret redaction.**

## Residual risks (accepted under the owner's delegation)

On 2026-11-20 the owner, German Munoz, explicitly confirmed REC-06 and REC-07 as accepted risks, in the words "REC-06 y REC-07 mantenerlos como riesgo aceptado". The rationale: both fail closed and never report a false success, and closing them would need durable resource identities in the audit trail and a proof of no effects inside the safety kernel, for convenience only. This is a declared, unauthenticated assertion.

- **SIGQUIT and SIGKILL:** SIGQUIT keeps Go's default dump-and-exit, and SIGKILL cannot be deferred. Both take the crash path, which T123 covers at every durable step.
- **Orphaned claim (REC-07):** `ProveNoEffects` is not wired in the CLI composition. A claim orphaned by a crash before intent is never reassigned: that authorization stays `authorization_consumed`, and a new one must be issued. That is safe, but less convenient.
- **Interrupt during the first `begin`:** an interrupt delivered while the first operation's `mutation_intent` and CAS are being written lets that operation run. This is consistent with "observed only between operations".
- **Same-user concurrent tampering:** a TOCTOU window remains between `owned()` and the rename of the index lock. R-04 and R-06 narrowed it but did not close it.
- **Residual crash window (REC-06):** after the initial reference CAS and before the index is installed, recovery needs Git's own commands.
- **Git 2.45 minimum (VER-01):** executed after this review by the CI job `git-minimum` (first green run: job 136054532762, commit `5f2733c`; see T132).
- **Block-level power loss (DUR-01):** not executed. On 2026-11-20 the owner confirmed closing T134 as declined ("si, bórralas y arranca con el 8 y el 9"); this is a declared, unauthenticated assertion. Publication durability rests on the fsync and directory-sync order, covered by step-level fault injection and SIGKILL at every durable step.
