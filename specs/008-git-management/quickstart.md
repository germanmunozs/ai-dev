# Quickstart Validation Guide: Feature 008 — Git Management

**Status**: Proposed validation plan; no Feature 008 command or test below was executed during planning.  
**Supported acceptance targets**: macOS/APFS and Linux/ext4.  
**Normative source**: [spec.md](./spec.md)

This guide is for a future implemented Feature 008. Run it only in disposable directories and only after implementation authorization. It does not authorize Git mutation in the current repository.

## 1. Prerequisites

- Go 1.27.1 as declared by `go.mod`.
- A reviewed Feature 008 implementation and generated `tasks.md` from a later authorized phase.
- Git toolchain supporting the frozen capabilities required by the fixture.
- Local APFS on macOS or local ext4 on Linux for native acceptance.
- No containers are required or permitted as a substitute for native filesystem evidence.
- External evidence directory outside every test project and its `.git` directory.

Record before a run:

```text
uname / OS version
go version
go env GOOS GOARCH CGO_ENABLED GOFLAGS
git --version
git --exec-path
filesystem type for project and user-state roots
binary/source snapshot digest
```

## 2. Planned quality-gate commands

These commands are examples for the implementation phase and are not executed here:

```sh
gofmt -l <changed-go-files>
go vet ./...
go test -count=1 ./...
go test -race -count=20 ./internal/gitmanager/ ./internal/gitprocess/ ./internal/gitstore/ ./internal/app/
```

Platform/build selection must additionally use `go list` and representative builds for macOS, Linux, iOS package selection, Android package selection, and Windows fail-closed selection. Exact commands belong in the later task list after package names are final.

## 3. Fixture policy

Each integration scenario creates:

- a new disposable physical project directory;
- a separate disposable ai-dev user-state directory;
- a separate plan-output directory;
- sentinels for network, hooks, filters, helpers, fsmonitor, pager/editor, and prompts;
- before/after manifests of project, repository, index, refs, config, and audit roots as applicable.

Never use the ai-dev source repository as a mutation fixture. Never use `git add .` or any non-allowlisted staging command.

## 4. AC-001–032 verification matrix

| AC | Proposed fixture/action | Required observable result |
|---|---|---|
| AC-001 Read-only inspection | Inspect absent/normal/bare/nested/linked/submodule projects with hostile `GIT_*`, hooks, filters, fsmonitor, pager/editor, credential helper, and before/after manifests. | No configured executable runs; project/index/refs/config/audit bytes unchanged; complete redacted report. |
| AC-002 Canonical plan | Generate and regenerate the same plan; vary only excluded metadata, then vary frozen intent. Include official JCS vectors, duplicate/unknown keys, floats, unsafe integers, NFC/NFD. | Excluded metadata preserves digest; covered change alters digest; invalid/noncanonical input rejects; regenerated covered bytes are identical. |
| AC-003 Per-file approval | Omit, duplicate, add, alter mode/digest/OID, and swap normal/ignored approval forms. | `refused/authorization_input_incomplete` before authorization creation or mutation; exact set succeeds. |
| AC-004 Authorization issuance | Issue with complete inputs; repeat same ID; omit operator/scope/approval; make store unwritable. | First record durable `issued`; duplicate/insufficient input refuses without overwrite; unwritable store is `audit_unavailable`, zero mutation. |
| AC-005 Authorization concurrency | Start two apply attempts on one issued authorization with barriers; let the winner finish before the loser rereads; repeat against an already `applied` authorization. | One claimant progresses; a loser that verifies completed winner postconditions returns `already_applied`, exit 0, and mutates nothing; a repeated apply behaves identically; terminal state never returns to claimed. |
| AC-006 Crash/reconciliation | Crash after claimed without intent, after durable mutation intent, around the mutation-state CAS, and after individual operations; fail intent persistence; run owner and later apply paths; perform supervised reconciliation. | Only an intent-free proven orphan claim can be reassigned. Durable intent consumes `claimed`: a later invocation is `refused/authorization_consumed`; the owner is `refused/audit_unavailable` when CAS absence is proven and `state_indeterminate/audit_failed_after_mutation` when the CAS outcome is unknowable. All exit 1, perform no later Git mutation/retry, preserve the original state and require new linked plan/authorization. |
| AC-007 Deterministic init | Set hostile template/default branch/default ref format; plan/apply a new repo; use a tool fixture missing one required option. | Exact SHA-1/files/empty-template/explicit-branch argv and result; no template hook; missing capability is `git_capability_missing`; existing repo not reinitialized. |
| AC-008 Global preflight | Put a high-risk secret in the final allowlisted file and track Git object directory before/after. | `refused/sensitive_content_suspected`; zero blobs and, when initial-commit group exists, zero earlier project mutation. |
| AC-009 Effect classification | Inject first-operation no-effect failure, verified partial effect, and unverifiable result. | Respectively `failed`, `partially_applied`, `state_indeterminate`; classification considers all effects, not only blobs. |
| AC-010 Commit/index coherence | Successful initial commit; inject failure after ref CAS and before index install. | Success has exact tree/index and clean authorized paths; injected failure preserves commit and reports partial without rollback. |
| AC-011 Git Flow | Test AVH/next, complete/partial config, GitHub Flow selection with gitflow keys, no-history flow, and development CAS. | Exact strategy behavior; nonselection preserves keys; partial selected flow conflicts; no-history pending reason; later development CAS only when authorized. |
| AC-012 Paths | Use shell metacharacters, `|`, traversal, symlink component, case collisions, NFC/NFD pair, precompose incompatibility, and non-UTF-8 allowlist path. | Inputs remain literal; incompatible paths refuse before mutation with the exact path reason; no shell execution or silent normalization. |
| AC-013 Output safety | Plan output existing, inside project/Git dirs, and through symlink aliases; then safe external destination. | Unsafe/existing cases exit 2 and create nothing; safe destination is exclusive `0600`, complete and durable. |
| AC-014 Attributes and ignores | Before any `.git` exists, exercise system/global/XDG/core.attributesFile and nested attributes, plus `core.excludesFile`, `.git/info/exclude`, nested `.gitignore`, negation and precedence; include an invalid negative attribute pattern, unreadable/unsupported sources, conversions and external-code sentinels; differential-check only inert fixtures against frozen Git. | In-process results have ordered source identities/digests, winning rules and effective values; a globally ignored file requires ignored approval; no temporary repo or configured executable; supported inert results equal Git; any difference or unsupported/uncertain semantics is `attributes_incompatible`. |
| AC-015 Toolchain/index | Change launcher, effective binary, exec-path, capabilities, or `DEVELOPER_DIR`; enable worktreeConfig/split index. | Tool drift is `plan_stale`; worktreeConfig unsupported; installed prepared index remains autonomous v2 with no shared reference. |
| AC-016 Conventional Commits | Validate ordinary messages, uppercase type, `Refs #123`, `BREAKING CHANGE`, `BREAKING-CHANGE`, lowercase `breaking-change`, malformed messages. | Validity and breaking recognition match corpus; no Git/repository access; exits 0/1/2 exactly. |
| AC-017 Locks | Exercise: free same-project lock, active holder, rename/move within one filesystem, free lock after an indeterminate prior authorization, proven physical alias, recycled device/inode for another directory, absent historical metadata, and corrupt/indeterminate current identity; also replace index locks and fail owned release. | Current verified advisory/descriptor/project/namespace evidence reuses a free persistent file without deletion or path-history requirement; active or unverifiable current identity is `lock_conflict`; rename/move remains usable; recycled tuples create new documentary plans/authority; absent history alone does not reject; old authority is never reused; replaced resources are never removed. |
| AC-018 Audit failures | Make audit unavailable before mutation and fail event/result write after a controlled mutation. | Pre-mutation `audit_unavailable` with zero mutation; post-mutation `state_indeterminate/audit_failed_after_mutation`, no rollback. |
| AC-019 Platforms | Run native macOS/APFS and Linux/ext4 capability/crash suites; exercise tmpfs, NFS/SMB/FUSE/overlay/read-only/unknown fixtures and cross-select Windows/iOS/Android. | Only local APFS/ext4 can provide production evidence; every insufficient profile fails before mutation; tmpfs is test-only and cannot satisfy durability evidence. |
| AC-020 Regression/authority | Run all existing suites and structural imports; attempt to use Workflow/Human Gate outcome as Git authorization. | Features 001–006 unchanged; no forbidden dependency; Feature 007 receives report only; automated signals cannot authorize apply. |
| AC-021 Result/state mapping | Exercise every status/state pair, post-mutation audit failure, owner CAS outcomes after durable intent, and a new apply against original claimed-with-intent/mutation_started/indeterminate. | Exact table mapping; later attempt is audited `refused/authorization_consumed`, zero planned mutation, original state unchanged; owner proof/uncertainty maps exactly as AC-006 and does not claim Git mutation merely from intent. |
| AC-022 CLI contract | Table-test every option/mode, `--path`, formats, malformed approval, plan domain/usage refusals and every closed reason/exit category. | Only documented combinations accepted; success uses `GitPlanOutput`, refusal uses redacted `GitPlanRejectionOutput`; exactly one JSON object; status/reason/exit remain distinct and exact. |
| AC-023 Git Flow profiles | Capture every planned/written AVH and next key, instance `.base`, hook keys, and fault each write position. | AVH 8 keys; exact next profile; no hook key; `.base` preserved; first failure stops with auditable prior writes. |
| AC-024 Project identity | Capture identity before/after `git init`; test path alias and inode >2^53; move/remount fixture where controlled. | Stable across init; decimal strings exact; alias handled safely; changed path/device/inode invalidates prior binding. |
| AC-025 Prepared index | Parse produced index; replace `index.lock` during barriers; enable split/skipHash ambient settings. | Version 2, exact entries/OIDs/modes/checksum, no link/shared index; replaced lock not removed/installed. |
| AC-026 Audit records | Inspect modes/order/durability of attempt, intent, operation-started/result, final and release records across success/refusal/faults. | Records distinguish roles, precede actions as required, exclude secrets, and expose missing conclusive results. |
| AC-027 Identity/message | Inject CR/LF/NUL/control/header delimiters, invalid email, noncanonical line endings, ambient signing. | Unsafe identity/message refuses with exact reason before object creation; stored approved bytes exact; no `gpgsig`. |
| AC-028 Complete commit flow | Secret-last failure, successful exact flow, Git Flow development CAS, and terminal-write counter. | Zero blobs on preflight failure; exact commit/index on success; development occurs later; exactly one terminal result. |
| AC-029 Repository compatibility | Initial commit with history/ref/non-unborn HEAD/non-empty or conflicted index; invalid HEAD; inaccessible/ambiguous repo; plan without initial commit on existing history. | Incompatible initial-commit cases are `refused/repository_state_incompatible`, exit 1, zero blob/mutation; non-commit plan remains eligible. |
| AC-030 c-01 precedence | Present same mismatch initially, as apply snapshot drift, and after successful immediate revalidation; repeat last case before and after mutation. | Reasons are respectively `repository_state_incompatible`, `plan_stale`, `concurrent_change`; pre-mutation results refuse/exit 1/zero effects; post-mutation uses conservative effect status. |
| AC-031 Existing-history development | On existing Git Flow history with missing development, test omitted option, malformed OID, wrong algorithm, nonexistent/inaccessible commit, non-production-tip and correct base; then drift production and race target creation. | Omitted/inaccessible/mismatched base is `strategy_conflict` exit 1; malformed/wrong algorithm/forbidden context is `invalid_strategy` exit 2; only exact accessible production tip yields canonical target-absent CAS and authorization scope; later drift/concurrency follows c-01. |
| AC-032 Terminal/release protocol | Inject: release-only failure after applied operations; primary operation failure then release failure; two different release failures; release-audit failure; crashes before/during release, after apply-lock release, after prepared-result fsync and around terminal CAS. | Every failed release is its own deterministic structured entry identical in audit/output; sole release also sets `resource_release_failed`; prior primary is preserved; any non-empty set exits 1; status remains effect-based; audit uncertainty is indeterminate; unreferenced result never conclusive and authorization never reused/retried. |

## 5. Security sentinels

Future tests should use inert local sentinels that create a file if executed. Configure each as a Git hook, filter, fsmonitor, pager, editor, credential helper, askpass, diff/merge driver, transport helper, and signing command. After inspect/plan/apply, assert no sentinel file and no outbound-network attempt exists.

Do not place real credentials or secrets in fixtures. Use unmistakably synthetic test patterns.

Add structural and boundary cases:

- only `gitprocess` imports `os/exec`; `gitmanager` imports neither `gitprocess` nor `gitstore`; composition occurs in `app`;
- exact argv/environment/effects table is exercised row by row, including absence of reflogs and undeclared init paths/config;
- parser edges at and above 8 MiB plan, nesting 32, 16,384 members/items, 1 MiB strings, 4,096 files, 8,192 operations, 16,384 postconditions, 16-level includes and output caps;
- exact-boundary and one-byte-over fixtures for 64 MiB per authorized file, 512 MiB aggregate authorized bytes, 1 MiB commit message, 1 MiB per config/attribute/ignore source, 16 MiB aggregate sources and 4 MiB captured streams; assert respectively `repository_state_incompatible`, `commit_message_invalid`, `attributes_incompatible`, or the evaluated source/operation reason, while exact limits pass;
- safe Git-config parsing across system/global/XDG/local scopes, `core.excludesFile`-only ignores, nested negation/precedence, and a sentinel proving no configured command runs;
- Git reference/prefix values with leading dashes, controls, LF/CR, `@{-1}` and shell metacharacters; full-ref validation must reject or preserve them exactly without shell interpretation;
- hostile ambient global `core.sharedRepository` is reported but, because it is not replayed, does not alter or block deterministic init; a repository-local/worktree or deliberately replayed effective value that could alter effects rejects as `configuration_conflict` before mutation;
- fixed private HOME/XDG leaves remain absent before and after every allowed Git argv; real-user `safe.directory` is not replayed, and dubious ownership remains `dubious_ownership`;
- literal initial branch `HEAD` rejects during planning as `invalid_strategy`, exit 2, before authorization or mutation;
- raw paths round-trip through unpadded base64url without NFC/NFD normalization;
- exercise lock reuse independently: same project/free file succeeds; active holder is `lock_conflict`; rename/move in the same filesystem succeeds from current proofs without path history; a crash with prior indeterminate authority permits a separately validated new authorization while the old one remains consumed; physical aliases share exclusion; a recycled tuple for another verified directory gets a new documentary identity/authority; absent history alone does not reject; corrupt diagnostics that prevent current proof or indeterminate project/resource identity is `lock_conflict`. No case deletes the persistent file or reuses old authority;
- assert the canonical state roots and exact environment bytes: macOS `$HOME/Library/Application Support/ai-dev/state/git`, Linux absolute `$XDG_STATE_HOME/ai-dev/git` or fallback `$HOME/.local/state/ai-dev/git`, with HOME/XDG_CONFIG_HOME directly under `<state-root>/environment/`; verify there is no `git/git`, the leaves stay absent before/after every Git invocation, and plan digest/operation ID/executed environment all carry identical path bytes/profile version;
- immutable record crash before publication leaves no visible event; truncated temp requires reconciliation;
- existing `project_id` survives clone while local audit/authorization roots remain distinct;
- relative `XDG_STATE_HOME` is never resolved against CWD.

## 6. Crash/fault matrix

Inject failure immediately before and after:

- attempt record sync;
- authorization issuance/claim/mutation-start/terminal CAS;
- mutation-intent and operation-started sync;
- deterministic init;
- each Git Flow config key;
- project-config temp write/fsync/rename/dir-fsync;
- each blob/tree/commit step;
- initial ref CAS;
- prepared-index construction and verification;
- index.lock exclusive create/write/fsync/rename/dir-fsync;
- development ref CAS;
- final inspection/result/output;
- owned-resource release.
- prepared terminal result file/directory sync;
- terminal authorization CAS before syscall, after syscall with lost response, and verified success;

For every point, assert exact verified effects, status, reason, durable authorization state, audit ordering, not-started operations, and absence of rollback/retry.

## 7. Native evidence package

Preserve each native run outside the repository with:

- environment/toolchain report;
- source manifest and identity digest;
- exact commands and exit codes;
- full test and race logs;
- AC fixture result summary;
- file hashes and `SHA256SUMS` self-hash;
- known limitations and independent-review outcome.

Native evidence remains proposed until executed and reviewed. Windows evidence must not be described as native validation.

## 8. Completion criteria

Feature 008 implementation is not complete until:

- all applicable quality gates pass or have explicit human-approved exceptions;
- all AC-001–032 have reproducible evidence;
- macOS/APFS and Linux/ext4 native evidence is reviewed;
- no closed reason/status/exit or authority boundary diverges from `spec.md`;
- an independent review confirms FR-001–192 traceability;
- a human separately approves the implementation and merge decisions.
