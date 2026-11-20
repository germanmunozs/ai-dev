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

## 12. Operation dispatch and initialization decisions (T073–T077)

`OperationDispatcher` (`operations.go`) is the production `OperationExecutor`. It runs operations only through the safety kernel's permits and adds no lock, audit or terminal logic of its own. Every rule is **derived** when the specification fixes it and **proposed** when it fills a gap; proposed rules need human approval.

| ID | Rule | Basis |
|---|---|---|
| OPS-01 | `Revalidate` and `Execute` receive the regenerated plan, so the dispatcher rebuilds each row from the authorized intent and snapshot rather than from the argv alone. | proposed interface change to APL-01 |
| OPS-02 | `Execute` fails closed before invoking anything, with `operation_failed` and no effect, unless the permit authorizes exactly this operation of this plan now. That means: it is the open operation of a live, started session; the permit names the session's authorization; that authorization was issued for this plan digest and lists the operation; and the operation is byte-identical in ordinal, kind and argv to the regenerated plan's. The permit is checked again immediately before Git runs. | derived F008-FR-141, T035/T076 |
| OPS-03 | A Git operation runs only when its rebuilt invocation has exactly the planned argv. Initialization must also be exactly the F008-FR-190 subcommand for the intent's explicit initial branch and the physical root, with every mandatory capability (`initial-branch`, `object-format`, `ref-format`). Only `-c` overrides of the controlled environment may precede the subcommand; an `init.*` or `core.sharedRepository` override is refused. This is checked when the plan is derived and again before dispatch. | derived F008-FR-181/190; override rule proposed |
| OPS-04 | Outcomes: a runner error is indeterminate; a non-zero exit is failed; exit 0 is completed only when the operation's own expected postconditions hold on a fresh inspection, failed when one does not, and indeterminate when one cannot be verified. | derived F008-FR-166; mapping proposed |
| OPS-05 | Conditions are evaluated on fresh state. Repository, HEAD, refs, index and local configuration come from a fresh inspection; paths are observed directly beneath the physical root, and reflogs beneath the common directory. A proven absence fails a path condition. A redacted or valueless configuration entry, and the conditions the inspection does not observe (object type, index tree, per-path status), are unverifiable and never assumed. | derived F008-FR-164/166; evaluation sources proposed |
| OPS-06 | `Verify` reports complete only when every plan postcondition holds. Verified effects are the expected effects of the operations that hold, and any unverifiable effect makes the evidence unverifiable. Zero effects are proven only when no effect holds, nothing is unverifiable and the result is not complete. A plan made only of verifications therefore stays `applied` (APL-09). | derived F008-FR-182/189; completeness rule proposed |
| OPS-07 | The `.ai-dev/` directory and `.ai-dev/config.json` writes are not dispatched until their filesystem protocol exists (T093): `Execute` returns failed without effect. Until then, a real apply of a new-repository plan stops after the verified initialization with `partially_applied`/`operation_failed`, exit 1, and nothing is rolled back. The composition still supplies no executor, so public apply stays refused (APL-01). | proposed, temporary until T093 |
| OPS-08 | The text plan shows the frozen toolchain, the exact initialization subcommand and the object and ref formats. The text apply result lists the verified effects. The JSON envelopes are unchanged: they already carry the argv, the toolchain identity and `verified_effects`. | derived T077; layout proposed |

Observation for review, not changed: terminal classification (§10) counts every completed operation, verifications included, as an effect. So a plan in which a verification completes and a later mutation then fails without effect is classified `state_indeterminate` rather than `failed`. That is conservative but imprecise.

Verification:

- **Dispatcher tests:** with a scripted runner and the planner's fake inspection, they prove the permit cases each run nothing: zero permit, another operation, another plan, a stopped, closed or unstarted session, another authorization, and the kernel-only test permit without a plan binding. They also prove that a tampered operation or a drifted argv runs nothing, that exit codes and runner errors map to their outcomes, that exit 0 without the effect is failed, that verifications run nothing, that project-configuration writes are not dispatched, and that a `.git` appearing between plan and dispatch refuses the initialization.
- **Exact form:** tests reject every deviation from the F008-FR-190 vector (template, object format, ref format, branch, root, separator, `--shared`, `init.*` and shared-repository overrides, trailing arguments). Each missing capability is `git_capability_missing` both when planning and when dispatching, and an existing repository is planned without initialization.
- **Real Git (macOS/APFS, Git 2.54.0):** the hostile setup puts a template with hooks, `description` and `info/exclude` in both `init.templateDir` and `GIT_TEMPLATE_DIR`. It also sets `init.defaultBranch`, `init.defaultObjectFormat=sha256`, `init.defaultRefFormat=reftable` and `core.sharedRepository=group` in the user's configuration, and `GIT_DEFAULT_HASH`, `GIT_DEFAULT_REF_FORMAT` and `GIT_CONFIG_PARAMETERS` in the process environment. Under all of that, apply through the real store and dispatcher creates `HEAD` → `refs/heads/trunk-x` with format version 0 and a files ref store. It creates no hooks, `info`, `description`, `logs` or `reftable` and no configuration key beyond those git init writes itself. A second apply of the same plan is `plan_stale` and leaves the repository untouched.
- **Linux (Fedora CoreOS arm64, Git 2.55.0):** the unit suites pass. The real-Git test skips because `/tmp` is tmpfs, which inspection correctly refuses as a production filesystem.

## 13. Reconciliation and repeated-apply decisions (T103–T105)

A rule is **derived** when the specification fixes it and **proposed** when it fills a gap. Proposed rules need human approval.

| ID | Rule | Basis |
|---|---|---|
| REC-01 | `already_applied` is now produced, superseding APL-09. When an attempt consumes an `applied` authorization of the same plan digest, it does not regenerate the plan: the authorization's own effects make the snapshot stale. Instead it takes the apply lock, rereads the authorization, and verifies the plan's postconditions on a fresh inspection. If all of them hold, the result is `already_applied`, exit 0, with no transition, no operation and the lock released. Otherwise the result stays `authorization_consumed`. The same check runs when a racing loser finds the authorization `applied` under the lock. | derived F008-FR-127, AC-005; mechanism proposed |
| REC-02 | The postconditions verified for `already_applied` and for reconciliation come from the plan file. `DecodePlanFile` now decodes `expected_postconditions` with a round-trip check. They are authentic only because the file's digest covers them and a durable authorization names that digest. | proposed |
| REC-03 | `InspectReconciliation` is steps 1–2 of F008-FR-180 and writes nothing. It takes the prior plan file and authorization ID, rereads the prior authorization (which must name that plan digest), reads the consuming attempt's audit trail, and classifies every prior postcondition on a fresh inspection as applied, absent or unverifiable. Its report grants no authority. | derived F008-FR-180; report shape proposed |
| REC-04 | (Approved by the owner on 2026-11-18; see `cli.md` §12.) Steps 3–4 are a new plan applied with an issuance whose `ReconcilesAuthorizationID` names the prior authorization. `Apply` then issues through the T032 `issueReconciliation`, linking the previous authorization, apply attempt and plan digest; the prior record is proven byte-identical afterwards. A prior that is not unresolved, or that names the new ID, is `authorization_mismatch`; a missing prior is `authorization_missing`. `internal/app/gitreconcile.go` composes the inspection, and `applyPlan` runs the linked issuance. The CLI routes `ai-dev init reconcile --plan FILE --authorization-id UUID` and `ai-dev init apply --reconciles-authorization UUID` were added in Phase 12 after approval. | derived F008-FR-180; CLI amendment approved |

Verification:

- **Real store:** `reconcile_integration_test.go`.
  - An indeterminate authorization is refused as `authorization_consumed` with recovery required and is never re-executed. Its inspection reports it unresolved, with a readable trail containing `mutation_intent` and absent postconditions.
  - The linked authorization over a new plan applies, records the previous IDs, and leaves the prior record byte-identical. A conclusive authorization is not reconcilable, and an unrelated authorization still applies.
  - Ten rounds of two concurrent consumers of one issued authorization each give exactly one `applied`. The other attempt is `lock_conflict` or `authorization_consumed`, the operations run exactly once, and the final record is `applied` with three transitions.
- **Real Git (macOS/APFS):**
  - Consuming the committed authorization again is `already_applied`, exit 0, with no transition; after a tracked file changes, the same attempt is `authorization_consumed`.
  - In a two-attempt race on real Git, exactly one attempt applies, and any later attempt is `already_applied`.
- **App:** `gitreconcile_test.go` composes the inspection over the real store and writes nothing.
- **Linux (Fedora CoreOS arm64):** these tests skip because `/tmp` is tmpfs, and the rest of the suites pass.

## 14. Activation decisions (T106–T117)

A rule is **derived** when the specification fixes it and **proposed** when it fills a gap. Proposed rules need human approval.

| ID | Rule | Basis |
|---|---|---|
| ACT-01 | The composition builds the executor. When `GitManagementDependencies` carries a `Runner`, apply and the assistant use the production `OperationDispatcher` over that runner, the builder and the random source. Its fresh inspections use the same user context as every other command. An injected `Executor` replaces it in tests only. Without either, apply is refused before any record, as before. | proposed |
| ACT-02 | T107 adds only the real `gitprocess.Runner` to `DefaultGitManagementDependencies`; `cmd/ai-dev/main.go` is unchanged, because it already composes through `app.RunWithDependencies`. This happened only after T106, T110 and T111 passed, including three `-race` runs of the Git, store, process and app suites. Every production mutation still enters only through the dispatcher's permit check, and `terminal.go` remains the only closer. | derived T107 gate; placement proposed |
| FLT-01 | The fault matrix found a misleading result. When the `issued→claimed` CAS took effect but its outcome was unverifiable, the refusal reported `issued` while the record was `claimed`, an orphan. The claim now rereads the record. A claim proven to be this attempt's own, with no intent possible yet, is released cleanly to `issued`. Otherwise the refusal reports the state it read. A claim left by a failed release is still an orphan that needs a proof of no effects. | derived F008-FR-179; reread proposed; changes the KER-03 orphan case |
| FLT-02 | Fault-matrix invariants for every store call before and after its effect: the result is valid and its exit agrees with its status; operations run in plan order at most once; the returned durable state equals the record unless the result is `state_indeterminate`; and a later consumption re-executes nothing once intent may be durable. Operation faults stop execution at the faulting operation, and an indeterminate operation always gives `state_indeterminate`. | derived AC-006, 009, 018, 021, 026, 032 |
| ACT-03 | The interactive assistant on a real terminal is accepted through the T106 harness, with a simulated TTY over the real engine. The public end-to-end test (T109) covers its no-TTY refusal, because a subprocess cannot be given a terminal without a PTY harness outside `gitprocess`. | proposed |

Verification (T117), on macOS 27.2.1 arm64 with APFS, Go 1.27.1 and Git 2.54.0 (Apple Git-157), from source `4bc3ee9` plus the uncommitted Phase 12 tree:

- **Passed (exit 0):** `gofmt -l` (the first run listed the unformatted `internal/gitmanager/race_test.go`; it was formatted and rerun with no output), `go vet ./...`, `go test -count=1 ./...`, and `go build ./...` for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64, windows/amd64 and freebsd/amd64.
- **Package builds:** ios/arm64 for the Feature 008 packages and android/arm64 for `./internal/...`.
- **File selection:** `go list` file selection for darwin, linux, ios, android and windows.
- **Dependencies:** `go list -m all` lists only `ai-dev`, and there is no `go.sum`.
- **Fuzz:** 20 s fuzz runs of the Conventional Commit, plan decoder, canonical writer, attribute parser and wildmatch targets.
- **Concurrency:** `go test -race -count=20` passed with no data race for `gitstore` (70 s), `gitprocess` (202 s), `app` (192 s), `conventionalcommit`, `report`, `gate` and `gatestore`. In that run, `gitmanager` exceeded `go test`'s default 10-minute timeout. Rerun as `go test -race -count=20 -timeout 60m ./internal/gitmanager`, it passed: exit 0, no data race, 657 s. The gate therefore needs `-timeout 60m` for that package.
- **Not executed:** native Linux/ext4 acceptance (T116), and the operator-approved evidence preservation and independent review (T115, T118).

## 15. Robustness hardening decisions (T119–T136)

**Status of these rules.** On 2026-11-18 the owner delegated the pending decisions: "para lo que sigue pendiente de mi, te autorizo a realizar lo que tu estimes que es mejor actuando como arquitecto de software senior". The rules below are therefore **decided by the implementing agent under that delegation**. They are not individually approved by the owner, and the independent review (T118) checks them.

| ID | Rule | Basis |
|---|---|---|
| INT-01 | **Interrupts.** SIGINT, which the entrypoint turns into cancellation, and SIGTERM, which `ai-dev init` adds, are observed by apply only between operations. Before the first operation, an interrupt is a clean refusal with reason `interrupted`: the claim is released and the authorization stays `issued`. From the claim on, the work runs under `context.WithoutCancel`: the running operation, verification, release and terminal closure always complete, no later operation starts, and the effects are classified with reason `interrupted`, like a concurrent change. Git runs in its own process group, so a terminal interrupt never reaches it. SIGKILL is not deferrable; it is covered by crash recovery (T123). | decided under delegation; closes T120 |
| INT-02 | `interrupted` joins the closed reason catalogue, with the same status compatibility as `concurrent_change`, and exits 1. | decided under delegation |
| LCK-01 | **Reference locks.** `HEAD.lock`, `packed-refs.lock` and every `*.lock` below `refs` are observed by the inspection, within the container-member bound. Each one, or an incomplete observation, is a `lock_conflict` incompatibility. The phase decides the reason: `lock_conflict` while planning, `plan_stale` at apply's regeneration or first revalidation, `concurrent_change` at a later revalidation. No lock is ever removed. | decided under delegation; closes T129 |
| REC-05 | **Configured branch adoption.** For `basic` and `github-flow` over an existing repository, the branch HEAD names is adopted as `initial_branch` or `primary_branch` only when rendering it reproduces the existing valid `.ai-dev/config.json` byte for byte. Nothing else is inferred, and any other configuration still conflicts. This lets a later plan, including the reconciliation of an interrupted initialization, plan a verified no-op instead of `configuration_conflict`. | decided under delegation; option (a) of T136 |
| REC-06 | **Residual crash window.** A crash after the initial reference CAS and before the index is installed, or one that leaves the attempt's `index.lock`, keeps a typed refusal (`repository_state_incompatible` or `lock_conflict`). Recovery is then Git's own: `git read-tree HEAD`, or removing `.git/index.lock`, followed by a new plan. Option (c), the supervised removal of entries proven owned, is deferred because it would need durable resource identities in the audit trail. | decided under delegation; residual risk |
| IO-01 | An effect that cannot be proven absent is never `failed`: a created but unidentified `index.lock` or configuration temporary, a rename or link reported failed after taking effect, and a failed directory sync are all `indeterminate`. Only an entry proven to be the created inode is removed. As a last resort, `render` never returns `refused` once a mutation intent may be durable. | derived F008-FR-179, 184; decided fixes of T124–T126 |
| RET-01 | **Retention, decision only.** Nothing in the state root is removed by any current command. A future supervised command may remove an attempt only when all of the following hold: its authorization is conclusive (`applied`, `failed` or `partially_applied` with its result recorded); it has no crash evidence; it is older than an explicit operator-given age; and the operator confirms the exact list. An unresolved or `state_indeterminate` attempt is never removed. Implementing it needs a new approved task. | decided under delegation; closes T135 as decision-only |
| DUR-01 | **Power loss (T134).** Not executed. The fsync and directory-sync order of every publication is covered by step-level fault injection (T124, T127, `recordOps`) and by SIGKILL at every durable step (T123). Simulating block-level power loss (`dm-log-writes`) is declined for this feature as disproportionate, and is recorded as residual risk. | decided under delegation |
| VER-01 | **Git version matrix (T132).** Not executed. Building Git 2.45.0 from source was refused by the agent's tool-permission policy, so the declared minimum is still verified only by the identification unit tests. Running it needs the owner's explicit permission for that build, or a prebuilt Git 2.45 binary. **Update:** the CI job `git-minimum` now builds the SHA-256-pinned 2.45.0 release on the runner and runs the real-Git suites with it. | moved to CI; closes when the job's first run is green |

After the independent review (T118, `review.md`), these rules were added or amended under the same delegation:

| ID | Rule | Basis |
|---|---|---|
| INT-03 | **Amends INT-01.** Every refusal decided while the caller is cancelled, before the claim, is `refused`/`interrupted`; the cause it would otherwise have, such as a cancelled read, is kept in the detail. `verifyApplied` runs uncancellable, so an interrupt never turns an applied plan into `authorization_consumed`. SIGHUP is an interrupt like SIGTERM. Once one interrupt has been deferred, a second SIGINT, SIGTERM or SIGHUP exits at once with 130 and points to `ai-dev init reconcile`: an explicit escape into the crash path for a hung operation or a blocked terminal read. | review R-01–R-03 |
| ISO-01 | Every mutating Git row except `init` runs with `GIT_DIR=<project>/.git` and never discovers its repository: if `.git` vanished, Git fails instead of writing to an enclosing repository. | review R-04 |
| LCK-02 | **Amends LCK-01.** A linked worktree's own `refs` is walked too. Only an absent `refs` is empty; a symlink anywhere in the walk, a `refs` that is not a directory, or any walk error makes the observation incomplete, which is `lock_conflict`. | review R-05 |
| IO-02 | The configuration temporary is read back by descriptor, without following symlinks, and must be the created inode. The path must still name that inode immediately before the no-replace `link`. | review R-06 |
| REC-07 | **Residual.** `ProveNoEffects` is not composed in the CLI, so a claim orphaned before intent is reported `authorization_consumed` and is never reassigned; recovery is a new authorization. Wiring a proof of no effects is deferred. | review residual risk |
