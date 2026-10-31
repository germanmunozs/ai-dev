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
