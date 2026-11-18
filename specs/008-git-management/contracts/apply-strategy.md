# Apply, Strategy, Initial Commit, and Result Contract

## 1. Apply engine input rule

The plan file is untrusted. Apply validates bounds/schema, extracts only immutable intent, performs fresh inspection, regenerates operations through the legitimate planner, and requires canonical byte equality plus the confirmed digest. Only regenerated typed operations can execute.

## 2. Apply sequence

1. Validate limits; generate `apply_id`; publish durable attempt record.
2. Resolve physical identity; issue or locate exact authorization and audit refusals.
3. Validate authorization/plan/project/scope/approval/expiry/state binding and complete file approvals.
4. Fresh inspect; compare authorized snapshot first; regenerate canonical plan. Drift before mutation is `plan_stale`.
5. Complete initial-commit global preflight when applicable.
6. Acquire device/inode external apply lock; reread; CAS `issued→claimed`.
7. Establish the sole global mutation order: init; ordered Git Flow keys; explicit `create_project_config_directory`; barrier revalidating directory identity/mode and absent config target; project config; initial-commit group; development-ref CAS. Execution starts only after the step-8 intent/state/start protocol.
8. Before the first mutation publish `mutation_intent`, CAS `claimed→mutation_started`, publish `operation_started`, execute once, publish result and verify. Intent-write failure permits clean same-owner release only after proving no durable intent/effect. Durable intent consumes `claimed` and forbids reassignment/retry: a proven failed CAS returns `refused/audit_unavailable`; an unverifiable CAS returns `state_indeterminate/audit_failed_after_mutation`; both exit 1 and prohibit any subsequent Git mutation. Later mutations repeat immediate revalidation/start/result/postcondition.
9. Stop at first failure; freshly inspect every planned effect and calculate provisional status/primary reason.
10. Publish release-started/result for each owned resource, append one deterministic `SecondaryFailure` for every failed release, release owned non-apply resources, and release apply lock last.
11. Publish/fsync immutable prepared terminal result including the complete ordered `secondary_failures[]`.
12. Owner-only CAS authorization to the effect-based terminal state by comparing expected state/version/owner and atomically binding `terminal_result_digest` to the prepared-result digest; no prior digest is compared, and only verified success makes it conclusive.
13. Render the same result. Any secondary failure returns exit 1 without changing status.

Stop at first failure. No rollback, retry, repair, substitution, strategy fallback, garbage collection, or cleanup of pre-existing resources.

## 3. Deterministic repository initialization

Exact argv for a new repository:

```text
git init --template= --object-format=sha1 --ref-format=files --initial-branch=<explicit-name> -- <physical-project-path>
```

It runs through the frozen controlled toolchain without shell. Missing option/semantic capability is `git_capability_missing`. Existing repositories are not reinitialized and retain their object format. This exact F008-FR-190 argv, including the N-E `--ref-format=files` extension, is normative and prevails over the abbreviated initialization example in F008-FR-181.

The literal initial branch `HEAD` is rejected during planning as `invalid_strategy`, exit 2. Ambient system/global/XDG `core.sharedRepository` is inspected but not replayed into the fixed private environment and cannot alter init. Any repository-local/worktree or deliberately replayed value capable of changing planned permissions/effects is rejected as `configuration_conflict` before mutation.

## 4. Initial-commit group

### Phase 1 — global preflight before first mutation

For every allowlisted file and intended in-memory `.ai-dev/config.json`:

- resolve literal path without symlink following;
- open descriptor and verify regular type/mode;
- read exact bytes;
- compare descriptor identity/metadata before/after;
- compare authorized SHA-256 and computed blob OID without writing;
- run high-risk secret screening;
- establish attribute/conversion/status equivalence;
- retain immutable approved bytes.

All files must pass before any blob or earlier project/repository mutation. Then a second all-file identity/metadata barrier runs before the first blob.

### Phase 2 — construction

1. Create and verify blobs without filters from retained bytes.
2. Build and verify exact tree.
3. Create unsigned commit from frozen identity/time/message bytes; exactly one final LF.
4. CAS initial ref from absent to commit.
5. Exclusively create descriptor-owned `.git/index.lock`, retain/verify its identity, and revalidate normal index.
6. Build and verify autonomous prepared index v2 outside repository.
7. Copy through retained descriptor, fsync, reverify ownership, rename to index and fsync Git directory.
8. Verify HEAD/ref/commit/parent/tree/index/status and preservation of unauthorized paths.

CAS failure leaves ref/index unchanged and may leave unreachable objects. Post-CAS index/verification failure preserves commit and is partial; no rollback.

## 5. Git Flow operations

- New Git Flow requires explicit `avh` or `git-flow-next`.
- Production and development differ; for next they cannot collide with `feature`, `bugfix`, `release`, `hotfix`, or `support` namespaces.
- Every profile key/value from `spec.md` is a separate ordered operation.
- Multi-key writes are non-atomic; stop after first failure and report prior writes as partial effects.
- Never write `gitflow.path.hooks` or `core.hooksPath`; never invoke `git flow`.
- Preserve topic-instance `.base` keys.
- Without sufficient history, record strategy but omit development ref and report `development_branch_pending_initial_history`.
- With authorized initial commit, development CAS is a separate later operation targeting that commit.
- In an existing repository with history and absent development, omission of `--development-base-oid` is `strategy_conflict`/exit 1. Malformed syntax, wrong algorithm or forbidden context is `invalid_strategy`/exit 2. A nonexistent/inaccessible commit or mismatch with frozen production is `strategy_conflict`/exit 1. Creation exists only for an explicitly supplied accessible commit equal to frozen production tip. The target-absent CAS operation ID is authorization-scoped; no OID is inferred, and later drift follows c-01.
- Complete equal existing profiles are adopted as no-ops; empty profiles may be initialized. Partial/mixed/unrecognized or complete-but-different selected profiles are `strategy_conflict` and are never normalized.

## 6. Prepared index

Construction environment:

```text
GIT_INDEX_FILE=<private external prepared-index path>
core.splitIndex=false
core.fsmonitor=false
core.untrackedCache=false
index.version=2
index.skipHash=false
```

Reject `extensions.worktreeConfig=true`; do not override it. Verify `DIRC`, version 2, exact entries/modes/OIDs, checksum, no `link` extension/shared index, and no additional entry before installation.

The sole 13-step protocol is: (1) confirm planned lock absence; (2) exclusive-create no-follow `index.lock`; (3) record descriptor/device/inode/mode/owner; (4) retain it open; (5) revalidate normal index; (6) build and verify the autonomous prepared index outside the repository; (7) copy its bytes through the retained descriptor; (8) fsync; (9) fstat/reverify identity and ownership; (10) rename owned lock to `index`; (11) fsync Git directory; (12) verify installed index and ordinary status; (13) on pre-rename abandonment, unlink only the still-matching owned inode. Never remove a missing, replaced, pre-existing or foreign lock.

## 7. Result classification

| Status | Required condition |
|---|---|
| `applied` | All authorized mutations and postconditions verified. |
| `already_applied` | Prior applied postconditions still hold; new attempt mutates nothing. |
| `refused` | This invocation starts no mutation. Existing consumed/terminal authorization may remain unchanged; this includes later attempts against `claimed` with durable intent and an owner that proves its intent-to-state CAS did not occur. |
| `failed` | Mutation started but fresh inspection proves zero planned effects across the entire attempt. Terminal/consumed. |
| `partially_applied` | At least one effect is verified and the complete result was not achieved; all relevant effects are verifiable. |
| `state_indeterminate` | Any relevant applied/absent/planned effect cannot be verified, terminal audit/CAS cannot be made durable, or the original owner cannot determine whether `claimed→mutation_started` occurred after durable intent. The latter is authorization/audit uncertainty, not proof of Git mutation. |

Reason is the primary causal field from the closed catalogue. Every failed release creates one deterministic structured `SecondaryFailure`; a release-only failure additionally uses `resource_release_failed`, while another primary cause is preserved. Neither changes effect-based status, but any non-empty secondary list forces exit 1. `output_failed` preserves durable status/state. Pre-mutation `repository_state_incompatible`, `plan_stale`, and `concurrent_change` are refused with exit 1 according to c-01.

Limit mapping is unique: plan/JSON/structure/count/path → `plan_invalid`; configuration/attribute/ignore sources → `attributes_incompatible`; authorized-file/aggregate bytes → `repository_state_incompatible`; commit message → `commit_message_invalid`; captured output → the reason of its evaluated source/operation. Exact boundaries pass. `SecondaryFailure.audit_record_digest` is optional only when the relevant release record was not persisted and MUST NOT be fabricated.

## 8. Strategy report

`GitStrategyReport` is read-only information with `grants_mutation_authority=false`. Feature 007 may consume it but receives no authorization, credentials, apply plan, or permission to invoke apply.

## 9. Platform behavior

- macOS/APFS and Linux/ext4 require native acceptance evidence.
- They are the only production profiles and require per-location stable identity, local writable mount, exclusive create, advisory lock, file/directory sync, atomic same-filesystem rename/replace and CAS. User-state probes are private; project inspect/plan never writes a capability probe inside the project.
- tmpfs, NFS, SMB, FUSE, overlayfs and unknown types are production `filesystem_unsupported`; tmpfs is permitted only for explicitly non-durability tests and is never acceptance evidence.
- Windows, iOS, Android, and all other unsupported plan/apply targets return fail-closed reasons before mutation.
- Limited inspect never claims native support or validation.

## 10. Initial-commit decisions (T078–T089)

The initial-commit group is planned by `allowlist.go`, `postconditions.go` and `commit.go`, run by the `OperationDispatcher`, and governed by the safety kernel like every other mutation. A rule is **derived** when the specification fixes it and **proposed** when it fills a gap. Proposed rules need human approval.

| ID | Rule | Basis |
|---|---|---|
| COM-01 | Every commit input is explicit. The message is the exact bytes of `--initial-commit-message-file`. Name, email and RFC 3339 date are given separately for author and committer, and dates must be whole seconds. Nothing is read from Git configuration. Commit options without `--initial-commit`, `--initial-commit` without a message file, an unreadable message file or a malformed date are usage errors (exit 2). An empty or unsafe identity, or a missing date, is `identity_invalid` (exit 1). | derived F008-FR-096–100; the MAY-read of F008-FR-097 is not used; exit mapping proposed |
| COM-02 | A name or email is UTF-8 without controls or `<`/`>`. It may not begin or end with a byte Git strips from identities (space and `.,:;<>"\'`), because Git would store other bytes than the approved ones. An email has exactly one `@` with non-empty parts and no whitespace, quotes, brackets or commas. | derived F008-FR-098–099; exact rule proposed |
| COM-03 | The message must be canonical (F008-FR-104: UTF-8, no NUL/CR, exactly one final LF) and a Conventional Commit. The pure validator of T100 is implemented now because the group depends on it. Its grammar: an ASCII-letter type of any case, an optional non-empty scope, an optional `!`, then `: ` and a description. A body follows one blank line. Footers are the last paragraph, introduced by `token: ` or `token #`, and a footer with an empty value is invalid. Only uppercase `BREAKING CHANGE`/`BREAKING-CHANGE` footers or `!` are breaking. | derived F008-FR-102–107; grammar choices proposed |
| COM-04 | The allowlist always contains `.ai-dev/config.json`, which may also be named. Includes are literal paths. A spelling that is not a literal relative UTF-8 path (glob, traversal, absolute, control, NUL, `|`, a `.git` component) or a symlink component is `path_unrepresentable`. An absent, non-regular, over-bound or changed-since-inspection file is `repository_state_incompatible`. Collisions use the inspection's equivalence rule, and a new repository on macOS is assumed to fold case and precompose Unicode, as git init configures it there. Conversion evidence must prove byte identity without external code, else `attributes_incompatible`. The Git mode is `100755` when the owner execute bit is set and `core.filemode` is true (the default), otherwise `100644`. | derived F008-FR-086–089, 183; reason mapping and post-init assumption proposed |
| COM-05 | `InspectRequest.Candidates` names the initial-commit paths. Inspection freezes the identity of each candidate and its ancestors and its ignore status, and loads the attribute sources that apply to it. Apply and the dispatcher re-inspect with the intent's candidates, so the `Inspect` ports of `ApplyDependencies` and `DispatcherDependencies` now take them. | proposed interface change |
| COM-06 | When `.ai-dev/config.json` is absent at planning, its approval is the in-memory rendering, with the placeholder identity device `0`/inode `0`, mode `0644` and the rendered size. Phase 1 screens those in-memory bytes. The Phase 2 barrier requires the file this attempt wrote, with exactly those bytes and Git mode `100644`. | derived F008-FR-085/191; placeholder proposed |
| COM-07 | The plan freezes every OID: blobs, trees and the commit are encoded in process. The group is four operations. `initial_commit` writes the objects, each `object_type` an effect. `initial_ref_cas` runs `update-ref` with an empty expected old value, reflog disabled, HEAD then attached. `prepared_index` sets the index to the root tree, makes each authorized path status-clean, and declares the `index_lock` and `prepared_index` roles. A final verification checks the ref, HEAD, the index tree, and the identity and content of every other frozen path. With Git Flow, the development branch is a separate CAS after the group, at the planned commit, and is not pending. | derived F008-FR-110–125, 188, 192; operation shape proposed |
| COM-08 | Plan-level postconditions describe the final state. A later condition replaces an earlier one about the same state that it contradicts: HEAD unborn→attached, ref or index absent→created, path absent→present. Before this, a plan with the `.ai-dev/` barrier also listed `path_absent` and `path_content` for the configuration together, so a completed configuration write could never verify as complete. | proposed; changes the digest of plans that carried a contradiction |
| COM-09 | `object_type` is verified in process from the loose object: inflated, header checked, rehashed. A missing object fails when no pack exists and is unverifiable otherwise; without a repository it fails. `index_tree` parses the index as an autonomous index and re-encodes the tree from its entries. `path_status_clean` means a present index and no index or worktree change for the path. `index_absent`/`index_empty` read the index file directly, because once HEAD has a commit, status reports an absent index as staged deletions. | proposed evaluation sources, extending OPS-05 |
| COM-10 | `OperationExecutor` gains `Preflight` (Phase 1). It runs after scope binding and before the first revalidation and the claim. A difference is `plan_stale` and a secret is `sensitive_content_suspected`; either way the authorization stays `issued`. The Phase 2 barrier runs inside `initial_commit` before the first blob. A difference there fails the operation with zero Git invocations, and the revalidation just before it reports `concurrent_change`. | derived F008-FR-191; interface change proposed (like OPS-01) |
| COM-11 | Secret screening is in process and bounded. It recognizes private keys, AWS key IDs and secret keys, GitHub, GitLab, Slack, Google, Stripe, Anthropic, OpenAI and npm tokens, and URL credentials. A finding records only the path and category. The plan text and text refusals state the scanner's limitation. | derived F008-FR-085, 091–093; category list proposed |
| COM-12 | The prepared index lives in a fresh `<state-root>/prepared/<random>/` directory with mode `0700`; the store now creates `prepared`. `read-tree` writes into it under the planned overrides. The lock and the prepared index are registered through the permit as owned resources, so their release is audited at closure. A lock is unlinked only while it is still the inode this attempt created, and a missing or replaced resource becomes a `resource_release_failed` secondary failure. An `index.lock` already present at dispatch fails the operation with `operation_failed`, not `lock_conflict`, because the executor reports outcomes only. The parser accepts the cache-tree extension and nothing else. | derived F008-FR-115–124, 157–158, 184; storage location and reason proposed |
| COM-13 | The standard library has no `openat` on Darwin. So an approved file is read by opening each component with `O_NOFOLLOW` (directories with `O_DIRECTORY`, the file non-blocking), then re-proving every component's identity after the read. | proposed; residual risk: a component swapped and restored between open and recheck |
| COM-14 | The text plan shows the commit OID, the initial ref, both identities with dates, every message line, one exact `--approve-file` or `--approve-ignored-file` argument per file (ignored files expressly), and the screening limitation. The apply result names the commit, the index tree and the development branch only as far as their effects were verified. | derived F008-FR-090, 097, 103; layout proposed |

Verification:

- **Unit:** the allowlist, message, identity, repository-state, secret, preflight, object encoding, index parser, `index.lock` protocol, prepared storage and dispatcher outcome tests in `allowlist_test.go`, `commit_test.go`, `secrets_test.go`, `commitflow_test.go` and `index_test.go`, plus the CLI plan cases in `cmd/ai-dev/init_plan_test.go`.
- **Real Git (macOS/APFS, Git 2.54.0):** `commitflow_integration_test.go` initializes and commits a project whose configuration already holds the exact rendering, so T093 is not needed. The result is `applied`, with HEAD on `refs/heads/main` at the planned commit OID. The stored commit has the frozen identities, dates and message, with no parent and no `gpgsig`. There is no reflog and no `index.lock`. Status shows only the unauthorized untracked file. Prepared storage is released, and a new authorization for the same plan is `plan_stale`. A secret in the last allowlisted file refuses planning with only its path and category, before `.git` exists.
- **Linux (Fedora CoreOS arm64, Git 2.55.0):** the unit suites pass; the three real-Git tests skip because `/tmp` is tmpfs.
- **Not verified:** a project whose configuration must be written by the same attempt (T093), a CAS lost to a concurrent writer and an index failure after the CAS against real Git, and native Linux/ext4.
