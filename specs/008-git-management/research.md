# Research: Feature 008 — Git Management

**Date**: 2026-10-11  
**Status**: Phase 0 complete; no unresolved clarification  
**Normative source**: [spec.md](./spec.md)

This document records implementation-design decisions. It does not replace or relax any FR, AC, reason, status, exit code, or approved residual risk.

## R-01 — Package boundaries

**Decision**: Use `internal/gitmanager` for domain orchestration, models and consumer-owned ports; `internal/gitprocess` and `internal/gitstore` import `gitmanager` only to implement those ports and are never imported by it. `internal/app` is the composition root importing domain and implementations. `internal/conventionalcommit` is a leaf imported by the domain; rendering remains in `internal/report`, which may import result types but is not imported by the domain. A structural test enforces this DAG.

**Rationale**: These are distinct trust boundaries: untrusted CLI/plan input, externally configured Git execution, durable authority, and pure validation. The separation prevents Feature 008 from importing Human Gate authority and limits regression risk to existing process abstractions.

**Alternatives considered**:

- Put everything in `internal/app`: rejected because security, state-machine, and persistence logic would become inseparable from CLI parsing.
- Reuse `internal/gatestore`: rejected because Git authorization must remain independent from Feature 006 evidence.
- Extend `internal/system.Executor`: rejected because its inherited environment and text-only output are intentionally simpler than Feature 008 requires.

## R-02 — Existing CLI compatibility

**Decision**: Preserve `app.Run` and `app.RunWithGateStore` as compatibility wrappers and route new commands through an internal dependency bundle used by `cmd/ai-dev/main.go` and focused tests.

**Rationale**: Existing Features 001–006 and their tests depend on current entrypoints. A wrapper avoids a broad mechanical rewrite and permits injection of Git runner/store/filesystem ports.

**Alternatives considered**:

- Change every existing call site to a new signature: rejected as unnecessary regression surface.
- Construct OS dependencies directly inside each command: rejected because fault and concurrency tests require explicit seams.

## R-03 — Controlled Git process boundary

**Decision**: Implement a dedicated runner that receives an exact argv slice, physical directory, frozen tool identity, bounded stdin/stdout/stderr, and an explicit environment profile. Invoke `exec.CommandContext` directly and never a shell.

**Rationale**: The runner must remove inherited `GIT_*`, prevent aliases/hooks/filters/helpers/fsmonitor/pagers/editors/signing/network, preserve selected real configuration for inspection, and verify the frozen executable before apply. These controls cannot be represented by the existing `system.Executor` interface.

**Alternatives considered**:

- Shell out through `sh -c`: prohibited by FR-163.
- Use a pure-Go Git library: rejected because Feature 008 explicitly freezes and audits the installed Git toolchain and requires exact Git behavior.
- Erase HOME/XDG without first evaluating real scopes and replaying every supported semantic value: rejected because inspection must preserve ordinary effective behavior without executing configured code. The selected design reads real scopes in process, then gives Git a private environment plus proved-equivalent overrides.

## R-04 — Git environment profiles

**Decision**: Define immutable `inspect`, `plan`, `apply`, and `object` profiles. Each profile has a complete environment allowlist and a command/config-key allowlist. A bounded in-process reader first evaluates real frozen system/global/XDG/local configuration, attributes and ignores without executing configured code. `<state-root>` is the canonical Git state root defined by R-26 and already includes its final `git`; Git subprocesses use its fixed absent leaves `environment/nonexistent-home-v1` and `environment/nonexistent-xdg-v1` as HOME/XDG_CONFIG_HOME, verify them absent before/after, set `GIT_CONFIG_NOSYSTEM=1` plus null global/system paths, and replay only sorted proved-safe semantic overrides. The exact resolved paths/profile version are frozen in plan/digest/operation IDs and execution; otherwise Git is not run. The leaves are never created and are not per-attempt resources. Apply admits only planner-generated operations.

**Rationale**: “Sanitized environment” is insufficient unless the relationship between real configuration and safe execution is explicit. Profiles make every allowed difference testable.

**Alternatives considered**:

- One universal environment: rejected because inspect must see inputs that apply must not trust implicitly.
- Rely on `--no-optional-locks` alone: rejected because it does not disable filters, helpers, hooks, prompts, or external commands.

## R-05 — Git tool identity on macOS and Linux

**Decision**: Freeze invocation mode (`launcher` or `direct`), launcher/effective paths and digests where applicable, filesystem identity, `git --version`, `git --exec-path`, required option capabilities, and environment selectors such as `DEVELOPER_DIR`. Re-resolve them at apply and reject differences as `plan_stale`.

**Rationale**: `/usr/bin/git` on macOS can be a launcher whose effective implementation depends on Xcode selection. A launcher hash alone does not identify executed behavior.

**Alternatives considered**:

- Always call `/usr/bin/git`: rejected because it can silently select a different toolchain.
- Fall back to another Git when capability is absent: prohibited; the correct result is `git_capability_missing`.

## R-06 — Physical project and path identity

**Decision**: Resolve the project through descriptor-based, no-follow operations; represent the canonical physical path reversibly; derive identity solely from path plus decimal device/inode. Preserve raw observed path bytes alongside validated UTF-8 presentation where possible; reject allowlisted paths that are not reversible.

**Rationale**: This provides stable identity across `git init`, prevents symlink/ancestor confusion, and supports APFS NFC/NFD and case-collision checks without silent normalization.

**Alternatives considered**:

- Text path only: rejected because aliases can name the same or different filesystem object.
- Include mtime/ctime: rejected because initialization itself changes directory metadata.
- Normalize all paths to NFC: prohibited because the filesystem/Git representation may differ.

## R-07 — Canonical plan serialization

**Decision**: Implement a schema-restricted RFC 8785 canonical writer for the validated plan model, backed by official JCS vectors and project fixtures. Decode untrusted plans with size/depth/count limits, duplicate-key detection, unknown-field rejection, no floats, and fixed schema types before canonicalization.

**Rationale**: Generic `encoding/json` output alone is not a sufficient proof of RFC 8785 property ordering and escaping. The plan schema intentionally avoids floating-point normalization complexity and represents large integers as decimal strings.

**Alternatives considered**:

- Hash ordinary Go JSON: rejected because encoder settings and map/string behavior are not the normative JCS contract.
- Add an unreviewed JCS dependency: rejected during planning because the repository currently has no module dependencies and supply-chain review is required before adding one.
- Hash the original plan bytes: rejected because the file is untrusted and contains excluded metadata.

## R-08 — Plan model and operation derivation

**Decision**: Separate `ImmutableIntent`, `PreconditionSnapshot`, non-authoritative metadata, authorization, and apply audit. Generate `DerivedOperation` only from validated intent and fresh inspection; calculate operation IDs from ordinal plus canonical operation content and never from apply-time IDs or temporary paths.

**Rationale**: This closes direct dispatch, makes regeneration deterministic, and allows apply metadata to vary without changing authorized semantics.

**Alternatives considered**:

- Deserialize operations and execute them: prohibited by FR-053.
- Re-resolve identity/message/timestamps at apply: rejected because it changes the approved commit.

## R-09 — Authorization and audit persistence

**Decision**: Store authorization/audit records below the platform user state directory partitioned by project-identity digest. Store apply locks in a separate namespace keyed only by the SHA-256 of decimal device/inode, excluding path, so physical aliases contend. Use strict JSON/private modes, no-replace publication of complete immutable records, and temp/fsync/atomic-replace/fsync plus expected version/owner CAS for mutable authorization state.

**Rationale**: The design provides durable intent before mutation, append-only evidence, and concurrency control without putting authority inside the repository or claiming authenticated identity.

**Alternatives considered**:

- SQLite: rejected as unnecessary infrastructure and dependency for one local writer protocol.
- One append-only log as the sole source of truth: rejected because robust atomic authorization CAS and bounded recovery would be harder.
- Records inside `.git` or `.ai-dev`: prohibited by P-1 and the authority boundary.

## R-10 — Apply lock and authorization CAS

**Decision**: Use a persistent platform POSIX advisory-lock file keyed by project device/inode and never delete it automatically. Its existence is normal. Sufficient reuse evidence is current: safe open, descriptor identity/type/mode/ownership verification, successful nonblocking advisory acquisition, fresh physical-project verification, and proof that the namespace key matches the current device/inode. No authorization history or path chain is required. Physical aliases and rename/move within one filesystem remain usable when those current proofs hold. Stale/missing diagnostics may be reconstructed through a new durable audit record while preserving history. A recycled tuple belonging to a different verified directory creates a new documentary identity and independent plans/authorizations; ambiguous identity, namespace mismatch, or corrupt diagnostics that prevent current verification fails closed. Prior `claimed`, `mutation_started`, or `state_indeterminate` authorization state does not keep a physically free lock busy and does not prevent FR-180 reconciliation or a separately authorized plan. Under the acquired lock, reread the selected authorization and CAS state/version/owner through durable record replacement. The sole post-lock transition is P-07 terminal closure: only the existing `owner_apply_id` may CAS its `mutation_started` record by comparing expected owner/state/version and atomically binding `terminal_result_digest`; no prior digest is compared and no reconciler or different authorization can write the original result.

**Rationale**: Advisory lock ownership controls live concurrency; physical identity prevents namespace confusion; diagnostics support investigation but grant no authority; durable authorization CAS controls mutation rights. File existence and historical authorization state are not proof of an active lock owner, and lock reuse never reactivates consumed authority.

**Alternatives considered**:

- PID lock files: rejected because PID reuse and namespaces make stale detection unsafe.
- Delete “stale” locks automatically: prohibited.
- Repository-local lock: rejected because authority/lock state must remain external and repository initialization may not yet exist.

## R-11 — Audit record layout

**Decision**: Use one immutable attempt header and sequenced immutable event/result records. Publish a prepared terminal result before terminal authorization CAS; it becomes conclusive only when the owner-only CAS references its digest. Sequences needed after apply-lock release are reserved while locked; immutable publication remains exclusive and cannot overwrite the original attempt. Contents include hashes/paths/outcomes but no file bytes, credentials, raw environment, unredacted URLs, temp paths, tokens or descriptors.

**Rationale**: Separate records make write-ahead ordering and missing terminal results observable without in-place mutation. Durable directory synchronization establishes whether an event existed before the corresponding operation.

**Alternatives considered**:

- Rewrite one growing JSON document: rejected because a crash can corrupt all history.
- Log to stdout only: rejected because every attempt must be durable before business return.

## R-12 — Project configuration write

**Decision**: Render deterministic `.ai-dev/config.json` bytes in memory during planning/preflight. Apply writes an exclusive temp in the same directory, verifies expected current state/content, fsyncs, atomically renames, and fsyncs the directory. Equivalent existing content is a verified no-op; divergent content is `configuration_conflict`.

**Rationale**: This fulfills versionability and atomicity while ensuring the exact content is individually authorized when included in an initial commit.

**Alternatives considered**:

- Update in place: rejected because it exposes torn writes.
- Merge unknown divergent configuration: rejected because it silently widens scope.

## R-13 — Initial Git objects

**Decision**: Use controlled plumbing: `hash-object -w --stdin --no-filters` for approved bytes, an exact NUL-safe tree-construction input, and `commit-tree` with frozen author/committer environment and exact message stdin. Verify every returned object ID and object content/type. Update refs only with explicit old-value CAS.

**Rationale**: Plumbing avoids hooks, worktree updates, filters, `git add`, templates, editor, and implicit signing while producing standard objects.

**Alternatives considered**:

- `git add` plus `git commit`: rejected because it touches the normal index and admits hooks/filters/templates.
- Write loose object files directly: rejected because Git plumbing already provides hashing/compression/object-format behavior under a controlled toolchain.

## R-14 — Prepared index

**Decision**: Build a new version-2 index outside the repository through controlled `GIT_INDEX_FILE` and `read-tree` from the exact committed tree, with split/fsmonitor/untracked/sparse features disabled. Independently parse and verify header, entries, modes, OIDs, extensions, and checksum before copying into an exclusively owned `.git/index.lock` descriptor and renaming.

**Rationale**: The result must be autonomous and must not overwrite a concurrently changed normal index. Independent parsing verifies properties Git command success alone does not prove.

**Alternatives considered**:

- Run `git reset --mixed`: rejected because it is an implicit broader worktree/index operation and rollback-like command.
- Install the temporary index directly: rejected because ownership and same-directory lock/rename semantics must protect the normal index.

## R-15 — Git Flow configuration

**Decision**: Treat every approved AVH or git-flow-next key as one ordered operation with exact old/new preconditions. Preserve instance `.base` keys. Do not invoke `git flow`, create hooks, or alter `gitflow.*` when another strategy is selected. Development ref creation is a later CAS after a successful initial commit.

**Rationale**: Git config multi-key writes are non-atomic; operation-level audit and partial classification are necessary. The full profiles in `spec.md` are the only profiles implemented.

**Alternatives considered**:

- `git flow init`: prohibited and introduces executable/dialect behavior outside the plan.
- One opaque “configure Git Flow” operation: rejected because partial effects would not be auditable.

## R-16 — Reason precedence and result classification

**Decision**: Implement one phase-aware classifier. Initial incompatibility is `repository_state_incompatible`; authorized-snapshot drift before mutation is `plan_stale`; a change after successful immediate revalidation is `concurrent_change`. Once mutation starts, status derives from all attempt effects while retaining the causal reason.

**Rationale**: This directly implements c-01 and prevents the same final state from being classified without considering when it became observable.

**Alternatives considered**:

- Let each operation choose ad hoc: rejected because reason/status/exit would diverge.
- Always call mismatches concurrent: rejected because stale authorization and initial incompatibility have different semantics.

## R-17 — Secret screening

**Decision**: Use a bounded in-process registry of high-risk signatures (private-key headers and well-defined credential/token forms), record only rule identifier/category and path, and fail closed on a finding. Scan all approved bytes before any mutation when the initial-commit group exists.

**Rationale**: In-process checks avoid executing external scanners and prevent sensitive content from entering audit. The report explicitly states false negatives remain possible.

**Alternatives considered**:

- External scanner executable: rejected because it expands process/network/configuration authority.
- Entropy-only rejection: rejected because of excessive false positives and ambiguous policy.

## R-18 — Conventional Commits validator

**Decision**: Implement a pure parser for Conventional Commits 1.0.0 over UTF-8 message bytes. Treat syntax validity separately from recognized breaking-change footers, accept uppercase characters in types, and have ai-dev suggestions use lowercase.

**Rationale**: The command must run without Git/repository/strategy and must produce deterministic diagnostics.

**Alternatives considered**:

- Regular expression only: rejected because bodies, footer tokens/separators, and breaking markers require structured parsing.
- Git hook validation: prohibited because Feature 008 does not install or execute hooks.

## R-19 — Platform implementation

**Decision**: Put POSIX identity/locking/durability code behind `(darwin && !ios) || (linux && !android)` and a complementary fallback. Runtime filesystem probes still verify local semantics. Windows may expose limited inspect only and always rejects plan/apply before mutation.

**Rationale**: GOOS alone does not establish filesystem guarantees, and Darwin/Linux tags alone would accidentally select iOS/Android.

**Alternatives considered**:

- Treat all Unix targets as supported: rejected by the explicit platform scope.
- Require containers: prohibited and would not prove host filesystem behavior.

## R-20 — Verification strategy

**Decision**: Combine pure unit tests, fake-port fault tests, real temporary-repository integration tests, CLI subprocess contracts, race repetition, structural source checks, cross-builds, and separately preserved native APFS/ext4 evidence.

**Rationale**: No single layer can prove hostile environment isolation, binary index content, crash ordering, concurrency exclusion, platform selection, and real filesystem behavior.

**Alternatives considered**:

- Unit tests only: insufficient for process/fsync/rename/flock/CAS behavior.
- Native manual tests only: insufficiently reproducible and poor at exhaustive fault injection.

## R-21 — Pre-init configuration, attributes, conversions, and ignores

**Decision**: Implement the approved in-process evaluator. It reads bounded descriptor-verified system/global/XDG/local/project/info sources, safely resolves supported includes, parses Git-correct attribute matching/escapes/macros/precedence, and separately parses ignore negation/anchoring/per-directory precedence across `core.excludesFile`, `info/exclude` and `.gitignore`. Negative `.gitattributes` patterns are rejected because Git does not support them. It computes effective attributes and ignored status per candidate, freezes source identities/digests, and proves byte/status equivalence without invoking filters or configured commands. Existing-repository info sources are included; they are absent for a new repository. Unknown/unreadable/ambiguous or non-neutral semantics return `attributes_incompatible`.

**Alternatives considered**: A temporary Git repository was rejected because it adds external mutations and cleanup/audit state; unconditional rejection was rejected because it would unnecessarily eliminate compatible new projects.

## R-22 — Existing-history Git Flow development creation

**Decision**: `--development-base-oid ALGORITHM:HEX` is the sole explicit request. It is accepted only for Git Flow, existing history and an absent development ref; it must identify an accessible commit equal to the frozen production tip. In that repository state omission, an inaccessible/nonexistent commit, or a production-tip mismatch is `strategy_conflict`/exit 1; malformed syntax, wrong algorithm or forbidden context is `invalid_strategy`/exit 2. The exact ref CAS is a derived operation and its ID is authorization-scoped.

**Alternatives considered**: Inferring production tip was rejected by FR-046; a second boolean flag was unnecessary because presence of the OID option is itself an explicit request.

## R-23 — Terminal results and release failures

**Decision**: Preserve one primary `reason` and append one deterministic structured `SecondaryFailure` for every release failure, including a sole or multiple release failures. Release operations receive write-ahead/result audit. `audit_record_digest` names the latest relevant durable release record and is absent—not fabricated—if that record could not be persisted. A prepared immutable terminal result is fsynced before owner-only authorization CAS, and only the successful digest-binding CAS makes it conclusive. Any non-empty secondary list forces exit 1 without changing effect-based status.

**Rationale**: This represents simultaneous operation/release failures and eliminates a critical audit write after terminal CAS. A crash after apply-lock release is safe because `mutation_started` cannot be reclaimed.

## R-24 — Filesystem profiles

**Decision**: Production plan/apply supports only local APFS on macOS and local ext4 on Linux after per-location capability checks and native evidence. tmpfs is test-only and not durability evidence; NFS, SMB, FUSE, overlayfs and unknown types fail closed. User-state primitives are actively probed in private storage; project inspection never writes probes into the project.

## R-25 — Bounded parsers and reversible paths

**Decision**: Fix parser plus content limits in `plan.md`, including 64 MiB per authorized file, 512 MiB aggregate bytes and 1 MiB commit message; encode raw reversible path bytes as unpadded base64url plus optional UTF-8 display. Configuration include depth is bounded/cycle-detected and canonical-plan structures reject excess before allocation. Exact boundaries pass; excess maps uniquely to `plan_invalid` (plan structure/JSON/count/path), `attributes_incompatible` (configuration/attribute/ignore sources), `repository_state_incompatible` (authorized file/aggregate bytes), or `commit_message_invalid` (message). Bounded output overflow retains the reason of the evaluated source/operation.

## R-26 — Project identity and state roots

**Decision**: `project_id` is a frozen canonical `ImmutableIntent` UUIDv4 stored in versioned project config, independent of physical audit identity so clones remain portable and config bytes regenerate exactly. `<state-root>` means exactly `$HOME/Library/Application Support/ai-dev/state/git` on macOS and absolute `$XDG_STATE_HOME/ai-dev/git` or `$HOME/.local/state/ai-dev/git` on Linux. Relative XDG state is invalid and never resolved against CWD. `environment/`, `locks/` and `projects/` are direct children of this root; no consumer appends another `git`.

## R-27 — Git Flow adoption

**Decision**: A complete existing profile with exact requested values is adopted as verified no-ops; no keys means new ordered configuration. Partial/mixed/unrecognized or complete-but-different selected Git Flow is `strategy_conflict`. The MVP never normalizes existing Git Flow configuration.

## Resolved outcome

All Step 53 and Step 56 findings are resolved by the approved Step 54 decisions, Step 55 P-06/P-07 amendments, Step 55-B microcorrections and Step 57 N-01/N-02/N-03 decisions. No new authority, strategy, reason, platform, network capability, default, or dependency is introduced. The design requires independent re-audit; no implementation or task generation is authorized by this statement.
