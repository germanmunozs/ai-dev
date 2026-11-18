# Authorization, Audit, and Concurrency Contract

## 1. Authority boundary

`GitAuthorization` is local supervised mutation authority. It is not authentication, a Human Gate, Workflow State, agent permission, merge approval, remote permission, or constitutional approval. `--authorized-by` is a declared label only.

## 2. Issuance

Issuance is allowed only during non-interactive apply with `--issue-authorization`, or through the same engine after explicit interactive confirmation. Exclusive creation binds:

- unique authorization ID;
- exact confirmed plan digest;
- freshly verified physical project identity;
- exact operation IDs, strategy/dialect, refs, config, and initial-commit decision;
- complete normal/ignored per-file approvals;
- declared operator;
- issue time and optional expiry.

Initial state is durably `issued` before any project/repository mutation. Existing ID is never overwritten, merged, or widened.

## 3. State machine

```text
issued
  └─ claimed
      ├─ issued                  clean release before mutation
      ├─ claimed                 proven orphan reassignment
      └─ mutation_started
          ├─ applied
          ├─ failed
          ├─ partially_applied
          └─ state_indeterminate
```

Every transition is a durable CAS over ID, current state, version, and owner apply ID.

- No `issued -> mutation_started` jump.
- Terminal states never transition.
- A `claimed` authorization with durable `mutation_intent` is consumed and cannot be reassigned or retried; intent alone does not prove Git mutation.
- `mutation_started` cannot transfer owners.
- `failed` requires a fresh controlled proof of zero effects across the attempt.
- A terminal-write failure may leave durable `mutation_started`; the process result is `state_indeterminate/audit_failed_after_mutation`.
- `terminal_result_digest` is absent before closure. The owner-only terminal CAS from `mutation_started` compares authorization ID, expected state/version and owner, then atomically binds the prepared-result digest; it never compares against a prior digest. Reconciliation and unrelated authorizations never overwrite it.

## 4. Apply-attempt ordering

1. Validate minimal input bounds.
2. Generate fresh apply ID.
3. Durable exclusive-create and fsync attempt record.
4. Resolve project and authorization.
5. Validate plan binding, scope, approvals, state, and expiry.
6. Audit any refusal before return.

Failure at step 3 returns `audit_unavailable` and permits no project/repository mutation.

A later invocation against original `claimed` with durable intent, `mutation_started`, or `state_indeterminate`:

- creates its own attempt/result audit;
- returns `refused/authorization_consumed`, exit 1;
- performs no planned mutation and no original authorization transition;
- records the unchanged original state and indeterminate prior outcome.

## 5. Apply lock

- Stored outside the repository at `locks/by-device-inode/<sha256("ai-dev-lock-v1\0"||device||"\0"||inode)>/apply.lock`; the key contains no path. Authorization/audit records remain under the full project-identity digest.
- The persistent lock file is the advisory exclusion resource, not authorization state. Its existence is expected and it MAY be reused after opening it without symlink following, verifying its regular-file identity/mode/ownership, successfully acquiring the advisory lock nonblocking, and freshly revalidating both the physical project identity and the opened lock resource through their descriptors.
- Physical path aliases and bind mounts share the namespace only when descriptor resolution proves the same live project object. A rename or move within the same filesystem does not block a free lock merely because the documentary path changed: exclusive advisory acquisition, verified lock descriptor/type/mode/ownership, freshly verified current project identity, and proof that the namespace key is derived from that current device/inode are sufficient provenance. The changed canonical path produces the corresponding current documentary partition and requires its own plan/snapshot/digest/authorization; it never revives authority bound to the old identity. No prior authorization history or path chain is required.
- If a device/inode tuple has been recycled for a different directory, the currently verified physical identity defines a new documentary partition and therefore requires its own plans, snapshots, digests and authorizations. If the current project identity, lock-resource identity, ownership, or namespace correspondence cannot be verified—including an indistinguishable recycled object—reuse fails closed with `lock_conflict`.
- Active advisory lock causes `lock_conflict`.
- A free persistent lock MUST NOT be rejected merely because an earlier authorization remains `claimed`, `mutation_started`, or `state_indeterminate`, because a path changed, or because historical lock metadata is absent. Lock metadata is diagnostic, not a second authorization authority. While holding the advisory lock, stale or absent diagnostic metadata MAY be reconstructed through a new durable audit record when the current project identity, lock resource and namespace correspondence are proven; historical audit/authorization records remain immutable. Corrupt metadata that prevents those current proofs, or indeterminate identity, fails closed.
- A pre-existing lock file is never deleted automatically. Advisory-lock success alone is insufficient without the identity and integrity checks above.
- Under lock, reread authorization/snapshot before CAS.
- Two processes cannot both claim the same state/version/owner tuple.
- Reusing the physical lock never reuses or reactivates an authorization. A new authorization has its own plan, snapshot, digest, scope and concurrency checks; the reconciliation procedure for an older consumed authorization remains available under FR-180.

## 6. Durable audit

Record order:

```text
attempt_created
[authorization_issued]
[authorization_transition: issued->claimed]
mutation_intent
[authorization_transition: claimed->mutation_started]
operation_started
operation_completed|failed|indeterminate
...
resource_release_started
resource_release_result(s)
prepared_terminal_result
authorization_terminal_cas(result_digest)
```

Each immutable record is written completely to a same-directory private temporary, file-fsynced, published without replacement, and directory-fsynced before the corresponding action. A truncated/unpublished temporary is not an event and is preserved as crash evidence. Audit excludes file/message contents, secrets, credentials, raw environment, unredacted URLs, temp paths, tokens and descriptors.

Before the first mutation, `mutation_intent` is durable, then `claimed→mutation_started` CAS is durable, then that operation's `operation_started` is durable. Intent-write failure permits no mutation and allows same-owner `claimed→issued` only after proving no durable intent and no effect. Once intent is durable, `claimed` is consumed and ineligible for orphan reassignment or retry. If the owner durably proves that the CAS did not occur, it returns `refused/audit_unavailable`, exit 1, leaves the authorization `claimed` with intent, and performs no Git mutation. If it cannot determine reliably whether the CAS occurred, it returns `state_indeterminate/audit_failed_after_mutation`, exit 1, performs no further Git mutation, and records uncertainty about authorization/audit state rather than claiming Git changed. Subsequent attempts always return `refused/authorization_consumed`, exit 1, with a new linked attempt record and no transition of the original authorization.

Failure semantics:

- Before mutation: `audit_unavailable`, refused, no mutation.
- After mutation: stop, `state_indeterminate/audit_failed_after_mutation`, no rollback.
- Intent without conclusive result: indeterminate and requires supervised reconciliation.
- A prepared terminal result is non-conclusive until the successful authorization CAS references its digest.

## 7. Owned-resource release

Release is a planned protocol, not rollback or arbitrary cleanup. It may release only current-attempt resources whose ownership is reverified:

- external apply lock;
- prepared index temporary;
- configuration temporary;
- `index.lock` created exclusively by this attempt.

For `index.lock`, ownership is the retained descriptor plus device/inode and apply ID. No token is embedded in index bytes. A replaced, missing, pre-existing, or foreign lock is never unlinked. Every failed release of an owned resource creates an independent deterministic `SecondaryFailure`, even when it is the sole failure, without changing effect-based status.

Release ordering and terminal closure:

1. classify all attempt effects and retain the primary reason;
2. write/fsync `resource_release_started`, revalidate ownership, release once, and publish `resource_release_result` for each non-apply resource;
3. perform the same protocol for the apply lock last;
4. build, publish and fsync one immutable prepared terminal result containing effect status, primary reason and every structured `secondary_failures[]` entry ordered by planned release ordinal/resource role;
5. CAS authorization from `mutation_started` to that status while matching the current owner/version and atomically binding `terminal_result_digest` to the prepared-result digest; no prior digest is compared;
6. only verified CAS success makes the record conclusive.

A crash before/during release leaves `mutation_started`. After apply-lock release, no new process can claim that state, so it can only audit `authorization_consumed`. A different authorization may acquire the lock, but it cannot satisfy the original owner/version CAS and must independently detect repository drift. A crash after prepared-result publication but before CAS leaves a non-conclusive record. CAS failure/unverifiability yields `state_indeterminate/audit_failed_after_mutation`; there is no automatic retry or authorization reuse. Every release failure is structured; release-only also sets primary `resource_release_failed`, while an earlier primary is preserved. Audit failure during release creates the resource entry using the digest of the latest relevant durable release record when one exists; if none was persisted the optional `audit_record_digest` is absent and MUST NOT be fabricated. Unverifiable release or terminal state forces conservative indeterminate classification. Any non-empty secondary list exits 1 without changing effect-based status.

If the terminal CAS outcome is unverifiable but a later controlled read finds a terminal durable state, that read is recorded by a new reconciliation attempt. It never changes the original invocation's `state_indeterminate/audit_failed_after_mutation` output or overwrites its prepared record.

## 8. Reconciliation

For an indeterminate attempt, the only permitted path is:

1. fresh read-only inspection;
2. identify applied, absent, and unverifiable effects;
3. generate a new canonical plan;
4. issue a new supervised authorization linked to prior authorization/apply/plan IDs.

Historical records remain immutable. No old authorization is reactivated; no automatic rollback, retry, repair, cleanup, or fallback occurs.

## 9. Store implementation decisions (T020–T023)

`internal/gitstore` now holds the platform selection, filesystem observation and probe, the private state layout, and the record primitives that authorization (T029) and audit (T030) build on. Each rule is **derived** when the plan or this contract fixes it and **proposed** when it fills a gap; proposed rules need human approval.

| ID | Rule | Basis |
|---|---|---|
| STO-01 | POSIX files compile exactly on `(darwin && !ios) \|\| (linux && !android)`; every other target compiles `unsupported_other.go`, which fails closed with `platform_unsupported`. The fail-closed implementations live in `unsupported.go`, compiled everywhere, so they are tested on every host. Platform selection is verified in process with `go/build`, because Feature 008 tests may not start processes outside `gitprocess`. | derived, plan *Platform split*; file layout proposed |
| STO-02 | Production filesystems are local, writable APFS on macOS and ext4 on Linux, with every primitive proven or certified. ext2 and ext3 are not production even though they share ext4's magic: Linux takes the exact type from `/proc/self/mountinfo` for the mount containing the path. tmpfs is recognized as test-only. Network and userspace types (NFS, CIFS/SMB, 9p, Ceph, GlusterFS, AFS, Lustre, GPFS, virtiofs, FUSE) are not local. | derived, plan m-07; type source and remote list proposed |
| STO-03 | The state-root probe exercises no-follow exclusive create, `0600` mode, file sync (`F_FULLFSYNC` on Darwin through Go), stable identity, advisory `flock` contention between two open file descriptions, rename, replace, no-replace `link`, and directory sync, in a private directory. It removes exactly the files it created, and its evidence digest covers each result. Project-side locations are never probed: they take certified primitives only for a production filesystem. | derived, plan m-07 |
| STO-04 | The state root and its `environment`, `locks` and `projects` directories, and each project partition with its `authorizations` and `attempts` directories, are mode `0700` and owned by the effective user; an existing directory with another mode, owner or type, or a symlink, is refused with `audit_unavailable`. Missing parents of the state root are created with `0700`. Both environment leaves must be absent when the store opens. | derived, data-model §15; parent creation proposed |
| STO-05 | Immutable publication writes a complete same-directory temporary (`.tmp-<16 hex>`, mode `0600`), syncs and closes it, hard-links it to the absent final name, verifies the final name is the same file, unlinks the temporary and syncs the directory. A failure before the link removes the owned temporary and publishes nothing; an existing final name returns `ErrRecordExists` and is untouched; a failed or unverifiable step after the link returns `ErrStoreOutcomeUnverifiable`. | derived, plan *Authorization, audit, and concurrency*; in-process temporary removal proposed |
| STO-06 | Record replacement is a compare-and-swap under an exclusive, blocking `flock` on the record's directory: the comparison sees the current bytes, and only when it accepts them is the record replaced through a synced temporary, `rename` and a directory sync. Records are read only as private regular files of at most 8 MiB. | derived for CAS and sync order; lock target, blocking and size bound proposed |
| STO-07 | Events are named `<20-digit sequence>-<kind>.json`. A listed event directory must hold only private regular files with valid names and kinds, numbered 1..n without gaps or duplicates; anything else rejects the whole directory with `ErrInconsistentEvents`. Temporaries are reported separately as crash evidence and are never treated as events or removed. | proposed naming and contiguity rule; non-event temporaries derived |

Verification: tests inject a fault at every publication and replacement step. A mutation check removed the CAS lock and the concurrent-writer test then failed with up to 15 winners against one expected value. The suite passed on macOS (APFS) and on Fedora CoreOS arm64, where `/tmp` is tmpfs and `/var` is xfs; the ext4 production path has not yet run on a real ext4 mount.

## 10. Safety kernel decisions (T024–T035)

The durable safety kernel is `internal/gitstore` (lock, authorization records, audit writer) plus `internal/gitmanager` `safety.go`, `recovery.go`, `release.go`, `terminal.go`, `reconcile.go`, `journal.go` and `records.go`. Every rule below is **proposed** unless marked derived; proposed rules need human approval.

| ID | Rule | Basis |
|---|---|---|
| STO-08 | Attempts live at `<state-root>/attempts/<apply-id>/` (`attempt.json`, `events/`, `result.json`), not under `projects/<digest>/`, because the attempt record must be durable before the project is resolved (§4 steps 3–4) and must hold a refusal whose project never resolves. Events still carry the project digest; authorizations stay under `projects/<digest>/authorizations/`. Record schemas: `ai-dev.git-authorization/v1`, `ai-dev.git-apply-attempt/v1`, `ai-dev.git-apply-result/v1`, `ai-dev.git-audit-event/v1`. | deviation from data-model §15 layout |
| STO-09 | `GitStore` gains `RecordAttemptResult` (the once-only `result.json`) and `ListAuditEvents` (an `AuditTrail` of event kinds plus `CrashEvidence` when unpublished temporaries exist), and the ports gain `ErrOwnedResourceReplaced`. `attempt.json` is the `attempt_created` record, so `AppendAuditEvent` refuses that kind and events start at sequence 1. | port extension |
| STO-10 | The store accepts only the next sequence of an attempt and only an order `CheckAuditEventOrder` allows: one `mutation_intent`; operations one at a time after intent, each with exactly one result; releases after operations, start then result; one prepared terminal result, then only its terminal CAS record, then nothing. A record claiming an effect with no started operation, or a closure with no prepared result, cannot be published. | derived from §6 order; grammar proposed |
| STO-11 | The lock file is opened without following symlinks and reused only as a private (`0600`), singly linked regular file owned by this user; it is never repaired or deleted. Under the held lock, the lock name must still be the held file and the project path, opened as a no-follow directory, must still be the identified device/inode. `holder.json` beside the lock is diagnostic only: rebuilt atomically from those proofs on every acquisition; if it cannot be rebuilt, acquisition fails closed with `lock_conflict`. Release unlocks the held descriptor and reports `ErrOwnedResourceReplaced` when the name no longer refers to it. | derived §5; checks and diagnostic file proposed |
| KER-01 | `mutation_intent` and each `operation_started` precede their action (derived). `authorization_transition` records are published after the verified CAS, recording a fact rather than an intent, so a crash between the two never leaves a record claiming a transition that did not happen. | derived + proposed |
| KER-02 | A claim whose owner trail proves no intent is an orphan. It is reassigned (`claimed→claimed`) only when the caller supplies a fresh read-only proof of no effects; without one, or when the proof fails, the attempt returns `refused/authorization_consumed` with `recovery_required=false`. A claim whose owner trail is unreadable or carries crash evidence is consumed and unresolved. | derived F008-FR-179 rule 3; reason choice proposed |
| KER-03 | A failed `claimed→mutation_started` CAS is `refused/audit_unavailable` only when a fresh reread still shows the expected claimed tuple; an unverifiable or contradicted outcome is `state_indeterminate/audit_failed_after_mutation`. Both report the last verified durable state (`claimed`) and require recovery. | derived F008-FR-179; proof method proposed |
| KER-04 | Effect classification: `applied` only when every authorized operation completed and every postcondition was verified with no failure cause; `failed` only with a proof of zero effects and no completed operation or verified effect; `partially_applied` for verified effects with an incomplete result; `state_indeterminate` for an unverifiable effect, an open or indeterminate operation, an audit failure after mutation, or contradictory evidence (zero-effect proof with effects, or a complete result with a failure cause). | derived F008-FR-155/189; contradiction handling proposed |
| KER-05 | Release order is ordinal then role, and the apply lock is always given the ordinal after every other resource. A release whose start record cannot be published is not performed (`release_audit_failed`, no digest); the flock is then held until the process exits. Failure classes: `ownership_unverified`, `release_failed`, `release_audit_failed`. A release audit failure after mutation makes the result `state_indeterminate/audit_failed_after_mutation`. | derived §7; classes and lock ordinal proposed |
| KER-06 | The prepared terminal record embeds the canonical result unchanged, and that result is also `result.json` and the rendered output. The `terminal_authorization_cas` record is written after the verified CAS; its failure cannot change the already conclusive result. A refusal whose `result.json` cannot be written becomes `refused/audit_unavailable` (`audit_status=unavailable`); after mutation, the output is kept. `audit_status` is `recorded`, `unavailable` or `audit_failed`. `recovery_required` is true for `state_indeterminate` and `partially_applied`. | proposed |
| KER-07 | Reconciliation accepts only an unresolved prior (claimed with intent or crash evidence, `mutation_started`, `state_indeterminate`) in the same project partition. It links `previous_authorization_id`, the prior `owner_apply_id` and `previous_plan_digest`, records `authorization_issued` with reason `state_indeterminate`, never transitions the prior record, and proves it byte-identical afterwards. | derived F008-FR-180; checks proposed |
| KER-08 | `MutationPermit` has only unexported fields and is built only in `safety.go`, which a structural test enforces. It is valid only for the open operation of a started, unstopped, unclosed session of the same authorization. The test-only constructor `newTestMutationPermit` exists in `safety_test.go`. | derived T035 |

Verification: tests inject faults before and after every durable step: attempt, claim, intent, start CAS, `operation_started`, operation result, each release record, prepared result, apply-lock release and terminal CAS. Each case then checks the result, the durable authorization, the lock and a later attempt's `authorization_consumed`. Mutation checks removed 15 guards in total (lock, CAS, grammar, intent, consumption, start-CAS proof, terminal CAS, release-audit rule, zero-effect proof, orphan proof, clean release, reconciliation guard, permit binding), and each made its test fail. One surviving mutation exposed a missing stale-permit assertion, which was added. The suite passed on macOS (APFS) and on Fedora CoreOS arm64 (tmpfs `/tmp`).

Observation for review, not changed: `GitApplyAttempt.InvocationMode` reuses the Git tool's `launcher|direct` enum, while data-model §13 `invocation_mode` more likely means interactive versus non-interactive apply.

## 11. Apply and assistant decisions (T060–T072)

The apply engine (`apply.go`) consumes the safety kernel (§10) and adds no lock, release, terminal CAS or consumption logic of its own. Every rule is **derived** when the specification fixes it and **proposed** when it fills a gap; proposed rules need human approval.

| ID | Rule | Basis |
|---|---|---|
| APL-01 | The engine runs operations only through an `OperationExecutor` port (`Revalidate`, `Execute` under a `MutationPermit`, `Verify`). Until the operation dispatcher exists (T076) and public wiring is enabled (T107), the composition supplies no executor: `ai-dev init apply` is then refused with exit 2 before any record is created, and the assistant exports the plan and stops before authorization. | derived T072/T107; refusal form proposed |
| APL-02 | Before issuance, apply decodes the untrusted plan (intent only), requires `--confirm-plan` to equal the file's digest, freshly inspects the project the file declares, and regenerates against that digest. Issuance comes after regeneration, because its scope is taken from the regenerated plan. All of this precedes the claim and every mutation. The declared project path is used only as the place to inspect; the regenerated digest proves it. | derived steps 3–7; order within the pre-mutation steps proposed |
| APL-03 | The first operation is revalidated before the claim, so drift there is `plan_stale` and leaves the authorization unconsumed. Every later operation is revalidated before its `operation_started`, and a failure there is `concurrent_change` with effect-based status. | derived F008-FR-058/164; placement proposed |
| APL-04 | Issuance input. A missing or non-UUIDv4 ID, a missing or control-bearing operator, or an expiry that is not a whole-second RFC 3339 time after now is `authorization_input_incomplete`. A missing approval or a missing commit confirmation is also `authorization_input_incomplete`. Extra, duplicate, malformed or differently classified approvals, and approvals without an initial commit, are `authorization_mismatch`. | derived F008-FR-159–161/178; reason split proposed |
| APL-05 | The authorization scope is every operation ID of the plan (verification operations included), plus the sorted union of affected refs and configuration keys. Consumption compares every member (digest, project, strategy, dialect, operations, refs, keys, commit decision, approvals) besides the digest check the kernel repeats under the lock. | derived F008-FR-152/167; verification inclusion proposed |
| APL-06 | Refusals before the claim are recorded through the kernel's session and result path, so there is one writer. The only unrecorded result is `refused/audit_unavailable`, when the attempt record itself could not be made durable. | derived F008-FR-154 |
| APL-07 | Apply JSON is `ai-dev.git-apply-output/v1` holding exactly the validated result; text renders the same result with structured release failures and redacted findings. | proposed schema name |
| APL-08 | The assistant has no defaults. Confirmations accept exactly `yes` or `no`; an empty or invalid answer exits 2; cancellation, EOF and a wrongly typed digest exit 1. The operator must type the full plan digest and a declared operator label. Only then is the authorization UUID drawn from the injected random source, and it is shown before issuance. A terminal is detected by reading its attributes (`TIOCGETA`/`TCGETS`); `/dev/null` and pipes are refused with `interactive_terminal_required`. | derived CLI §1; details proposed |
| APL-09 | A plan whose operations are all verifications still claims and closes its authorization, and returns `applied`; `already_applied` is not produced yet. | proposed, pending review |

Verification:

- **Engine tests:** with a fake store, and an executor that checks every permit, they cover issuance input, existing IDs, expiry, digest and scope binding, untrusted and stale plans, revalidation phases, effect classification, the Git Flow pending reason, attempt-record failure, and single consumption.
- **Integration with the real store:** the trail on disk is attempt, issuance, transitions, intent, the operation pairs, release, prepared result and terminal CAS, in that order. The terminal digest equals the prepared record's digest, refusals are recorded, and no credential from user configuration reaches any record.
- **Mutation checks:** two surviving mutations exposed missing direct tests for approvals and for non-digest scope members, which were added. The two guards that still survive (scope digest, empty issuance input) are each also enforced by the kernel or by operator validation.
- **Assistant tests:** on a scripted terminal they prove no inspection without a terminal, four strategies without preselection, plan display before the destination, no authority before the typed digest, and every refusal path.
- **Real binary:** under a pseudoterminal the assistant starts, and with `/dev/null` it is refused.
- **Linux:** the suites passed; the filesystem-dependent tests skip on tmpfs.
