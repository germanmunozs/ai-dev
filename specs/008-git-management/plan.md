# Implementation Plan: Feature 008 — Git Management

**Branch**: `feature/008-git-management` | **Date**: 2026-10-11 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/008-git-management/spec.md`

## Summary

Add a supervised Git-initialization subsystem to the existing Go CLI. The subsystem separates read-only inspection, deterministic planning, durable Git authorization, and mutation application. It supports Basic Git, Git Flow, GitHub Flow, and Trunk-Based Development on macOS/APFS and Linux/ext4, fails closed elsewhere, and never derives Git authority from Workflow State or Human Gates.

The implementation is organized around a domain package (`internal/gitmanager`), a controlled exact-argv Git boundary (`internal/gitprocess`), an external durable authorization/audit store (`internal/gitstore`), an independent Conventional Commits validator, thin CLI adapters, and stable text/JSON reports. Every mutation is regenerated from validated intent, listed in `derived_operations`, audited before execution, immediately revalidated, and classified conservatively by verified effects.

## Technical Context

**Language/Version**: Go 1.27.1  
**Primary Dependencies**: Go standard library; the installed Git executable, version 2.45.0 or later, as a frozen external toolchain. 2.45.0 is the first release supporting `git init --ref-format`; the operation matrix also needs `rev-parse --path-format` (2.31) and `init --object-format` (2.29). The frozen capability probe remains authoritative: a Git that reports an older version or lacks a required capability returns `git_capability_missing` before planning. No Git Flow executable and no container runtime. Any new third-party dependency requires a separately reviewed plan amendment.  
**Storage**: Versioned `.ai-dev/config.json` inside the project; canonical plan exported outside the project; authorization, attempts, audit records, locks, and prepared temporary material under the per-user ai-dev state directory outside the repository.  
**Testing**: `go test`, race-enabled repeated concurrency tests, subprocess contract tests, fault-injection tests, structural boundary tests, cross-compilation, and native macOS/APFS plus Linux/ext4 acceptance evidence.  
**Target Platform**: macOS and Linux on compatible local filesystems. Windows and incompatible filesystems fail closed for plan/apply. iOS, Android, linked worktrees, submodules, and bare repositories are not plan/apply targets.  
**Project Type**: Single Go CLI with internal packages.  
**Performance Goals**: Inspection and planning remain bounded by repository/configuration enumeration and authorized-file reads; no network access; no unbounded recursion or input allocation. Safety and determinism take precedence over throughput.  
**Constraints**: Exact argv without shell; controlled environment; zero implicit operations; no filters/hooks/helpers/fsmonitor; canonical RFC 8785 plan; durable fsync boundaries; filesystem-identity locks; CAS state/ref transitions; autonomous index v2; no automatic rollback/retry/repair; closed result/reason vocabulary.  
**Scale/Scope**: One physical project per command; one authorization permits at most one mutation sequence; exact per-file allowlist; 192 functional requirements and 32 proposed acceptance criteria.

No unresolved clarification marker remains. Planning decisions below refine implementation structure without changing the normative specification.

## Constitution Check

*GATE: PASS before Phase 0; PASS again after Phase 1 design.*

| Principle / gate | Plan evidence | Result |
|---|---|---|
| I. Specification First | `spec.md` contains FR-001–192, AC-001–032, stable reasons, and c-01 precedence. This plan adds no behavior beyond them. | PASS |
| II. Architecture Before Framework | Boundaries follow trust, durability, and concurrency requirements; no framework selects the design. Consequential decisions are recorded in [research.md](./research.md). | PASS |
| III. Simplicity and YAGNI | Four focused domain/infrastructure packages are used; no daemon, service, plugin system, database, broker, container, or network integration is introduced. | PASS |
| IV. Testable by Design | [quickstart.md](./quickstart.md) maps all ACs to proposed validation and this plan declares executable quality gates. | PASS |
| V. Security by Design | Inputs are untrusted, secrets are not logged, Git is sandboxed by argv/environment policy, authority is scoped/durable, and mutations fail closed. | PASS |
| VI. Observability by Design | Durable local audit records cover attempts and operations without secrets. Distributed tracing is not applicable to a local offline CLI. | PASS |
| VII. Reproducibility and Automation | Toolchain identity, plan canonicalization, deterministic commit inputs, exact commands, platform evidence, and pass conditions are documented. | PASS |
| VIII. AI-Assisted Engineering | Versioned spec/plan/contracts are common to humans and agents; independent review remains required and tests are not represented as executed. | PASS |
| IX. Human Authority | Supervised Git authorization is explicitly distinct from authentication, Human Gates, merge approval, and production authority. | PASS |

No constitutional exception is requested.

## Architecture

### Boundaries and dependency direction

```text
cmd/ai-dev
    │
    ▼
internal/app ─────► internal/report
    │                    │
    ├──────────────┐     │ imports domain result types
    ▼              ▼     ▼
internal/gitprocess  internal/gitstore
    │ implements         │ implements
    └──────────────┬─────┘
                   ▼
          internal/gitmanager ─────► internal/conventionalcommit
```

`internal/gitmanager` owns all domain values plus the consumer-side ports `GitRunner`, `GitStore`, `Filesystem`, `Clock` and `RandomSource`. `GitStore` and `Filesystem` expose one intent-revealing method per contract step (`CreateAttempt`, `IssueAuthorization`, `LoadAuthorization`, `TransitionAuthorization`, `AppendAuditEvent`, `AcquireApplyLock`; `Identify`, `ReadRegular`, `CreateExclusive`, `CreateDirectory`) with typed requests and the sentinel outcomes `ErrAuthorizationExists`, `ErrAuthorizationMissing`, `ErrAuthorizationConflict`, `ErrApplyLockHeld` and `ErrStoreOutcomeUnverifiable`; neither port has a generic operation envelope, and later tasks extend the closed method set rather than adding one. There is no rendering port: Git commands return validated domain results that `internal/report` renders, as in Features 001–006. It imports only standard-library packages and the leaf `internal/conventionalcommit`. `internal/gitprocess` and `internal/gitstore` import `gitmanager` to implement those ports; `gitmanager` MUST NOT import either implementation. `internal/app` is the sole composition root that imports domain plus implementations and injects them. `internal/report` may import domain result types but is never imported by `gitmanager`. This one-way DAG is enforced by a structural import test and cannot form a Go cycle.

### Components

| Component | Responsibility | Normative coverage |
|---|---|---|
| `internal/app` Git command adapters | Parse the interactive assistant, inspect, plan, two apply modes, and validator; map result to exits 0/1/2; preserve existing command wrappers. | FR-001–010, FR-147, FR-159–161, FR-178, CLI contract |
| `internal/gitmanager` (`inspect.go`) | Resolve physical project identity; classify repository/HEAD/index/worktree; inspect configuration/attributes/toolchain without side effects; redact sensitive remote/config data. | FR-011–034, FR-130–139, FR-150–151, FR-155–156, FR-162, FR-175–176, FR-183, FR-186 |
| `internal/gitmanager` (`plan.go`) | Validate explicit strategy/branches, freeze intent/snapshot, derive every operation, canonicalize/digest, validate output target, and preserve reason precedence. | FR-035–58, FR-72–93, FR-138–149, FR-163–177, FR-181–187 |
| `internal/gitmanager` (`apply.go`) | Validate untrusted plan, issue/claim authorization, preflight, execute ordered operations once, verify postconditions, classify all effects, and reconcile safely. | FR-50–71, FR-80–129, FR-140–143, FR-152–154, FR-159–169, FR-178–180, FR-188–192 |
| `internal/gitprocess` | Identify the actual Git toolchain and run only approved exact argument vectors under a per-operation environment policy; capture bounded output without shell or network helpers. | FR-012–18, FR-29–34, FR-108–110, FR-130–132, FR-155–158, FR-163, FR-175, FR-190 |
| `internal/gitstore` | Store authorization CAS records, immutable histories, attempt/audit records, and filesystem-identity apply locks under private user state; provide POSIX implementation and safe unsupported fallback. | FR-59–71, FR-127–129, FR-152–154, FR-168–169, FR-178–180, FR-184–186 |
| `internal/conventionalcommit` | Parse and validate Conventional Commits 1.0.0 independently of Git and strategy selection. | FR-102–107, FR-147 |
| `internal/report` | Render exactly one validated text/JSON result; never turn a reason into a status or imply authorization. | FR-45, FR-66–69, FR-126, closed reason catalogue, GitApplyResult contract |

### Existing-code compatibility

- Keep `app.Run` and `app.RunWithGateStore` signatures as compatibility wrappers for existing tests and callers. `cmd/ai-dev/main.go` calls `app.RunWithDependencies` with the `app.Dependencies` bundle, which carries `GitManagementDependencies`; the router lives in `internal/app/app.go`, and the existing wrappers install an explicitly unavailable Git-management bundle unless tests provide one.
- Do not widen `system.Executor` or `system.Launcher`. Their current behavior is insufficient for controlled Git environment, bounded binary I/O, and stdin plumbing. `internal/gitprocess.Runner` is a separate narrow port so Features 001–006 retain behavior.
- Reuse established report validation and isolated JSON-output conventions, not Feature 006 authorization or storage semantics.
- Reuse lessons from `gatestore` (platform build constraints, injected operations, directory durability, fail-closed unsupported fallback) without importing or sharing Human Gate records.

### Safe Git execution

`gitprocess.Runner` accepts a typed operation, frozen `GitToolIdentity`, exact `[]string` argv, physical working directory, controlled stdin bytes, output bounds, and an environment profile. It invokes `exec.CommandContext` directly; no shell string exists.

Profiles are explicit:

- `inspect`: real HOME/XDG/system configuration is observable through the in-process evaluator; Git itself receives only the reconstructed controlled profile, while execution-capable settings are disabled and `GIT_OPTIONAL_LOCKS=0` prevents optional writes.
- `plan`: same in-process-observation/controlled-subprocess split plus plan-specific bounded reads.
- `apply`: only the frozen toolchain and operation-specific safe configuration are enabled. Network transports, prompts, credentials, aliases, hooks, filters, fsmonitor, pager/editor, signing, and external commands are disabled.
- `object`: supplies approved bytes to plumbing commands with `--no-filters` where supported and verifies returned OIDs.

The environment builder starts from an allowlist, removes inherited `GIT_*`, freezes/removes `DEVELOPER_DIR` as planned, sets `GIT_LITERAL_PATHSPECS=1` where applicable, disables terminal prompts, and records only non-sensitive selectors. Commands that cannot preserve the profile are rejected.

### Inspection and planning

Inspection proceeds through descriptor-based physical-root resolution before invoking Git. Platform-specific filesystem identity returns unsigned decimal device/inode strings. Repository kind, Git/common dirs, HEAD/history, refs, index/worktree changes, locks, configuration origins, attribute sources, Git Flow evidence, filesystem capabilities, and toolchain identity become one immutable `GitInspection`.

The planner is deterministic over validated `ImmutableIntent + PreconditionSnapshot`. It:

1. validates explicit strategy and branch inputs;
2. checks repository/platform/path/conversion compatibility;
3. renders intended `.ai-dev/config.json` in memory;
4. resolves and hashes the literal initial-commit allowlist when requested;
5. enumerates all mutations, preconditions, effects, postconditions, partial points, and recovery instructions;
6. creates deterministic `operation_id` values;
7. serializes the covered plan using the schema-restricted RFC 8785 canonical writer;
8. hashes the canonical bytes and writes the external plan using exclusive create, mode `0600`, fsync, and safe-path checks.

The canonical writer supports only the validated plan data model: no floats, no duplicate/unknown keys, decimal strings for unsafe integers, exact UTF-8 strings, deterministic UTF-16 property ordering, and RFC 8785 escaping. Official JCS vectors plus project-specific NFC/NFD and integer fixtures are required before use.

### Authorization, audit, and concurrency

The state root is selected by platform policy and is never inside the project. Authorization/audit records remain partitioned by `PhysicalProjectIdentity.digest`, but the persistent apply-lock file is separately keyed by `sha256("ai-dev-lock-v1\x00" || device-decimal || "\x00" || inode-decimal)` and therefore excludes path. The file is only the advisory exclusion resource: project identity, diagnostic lock metadata and durable authorization state remain separate. Bind-mount/path aliases proven to identify the same live physical directory contend on one file and are safe aliases.

Acquisition opens the persistent file safely, validates descriptor identity/mode/ownership, attempts the nonblocking advisory lock, then freshly revalidates project and lock-resource identities plus correspondence between the namespace key and current device/inode. Advisory contention or any unverifiable current proof is `lock_conflict`. These current proofs are sufficient: a free file remains reusable when its documentary path changed, historical diagnostics are absent, or an earlier authorization is `claimed`, `mutation_started` or `state_indeterminate`; those states constrain only that authorization. A changed canonical path creates the current documentary partition and independent plan/snapshot/digest/authorization even though the same physical lock namespace is reused. Stale/missing diagnostics may be reconstructed under the held lock through a new durable audit record without rewriting history. A recycled tuple used by a different currently verified directory starts a distinct documentary identity and authorization lineage; an indistinguishable or ambiguous reuse fails closed. The lock file is never automatically deleted. Directories are `0700`, records `0600`; strict schemas and bounded decoding apply.

- Authorization records are normally updated through external apply lock + state/version/owner CAS, temp file, file fsync, atomic replace, and directory fsync. The sole exception is the P-07 owner-only terminal CAS after planned apply-lock release: it compares expected `mutation_started`, version and `owner_apply_id`, then atomically binds the prepared `terminal_result_digest`; it does not compare with a prior digest. No other process or reconciler may transition that original authorization.
- Immutable event files are written completely to private same-directory temporaries, file-fsynced, and published without replacement through the capability-verified standard-library hard-link protocol (`os.Link` to an absent final name, verify identities, unlink only the owned temporary, then fsync the directory). No `x/sys` dependency or replace-capable rename is assumed. Truncated/unpublished temporaries are never interpreted as events and are preserved as crash evidence until supervised reconciliation.
- Every minimally valid invocation gets a durable attempt record before a business result.
- Mutation intent and each operation-started record are durable before mutation; operation-result follows immediately.
- Lock acquisition distinguishes an active holder (`lock_conflict`) from an existing reusable persistent lock file using advisory ownership plus fresh project/resource identity checks. Reuse grants no authority, does not depend on a prior authorization reaching terminal state, and never deletes the file.
- Resources created by the current attempt carry verifiable descriptor/inode/owner identity and are the only resources eligible for release.

Authorization transitions implement exactly FR-179. A new invocation against `mutation_started` or `state_indeterminate` only records its own refusal and returns `refused/authorization_consumed`; it does not transition or retry the original authorization.

### Apply and effect classification

Apply follows the 15-step sequence in `spec.md`. The implementation uses a single operation dispatcher over regenerated `DerivedOperation` values; there is no generic dispatch of file-provided operations.

Before the first mutation, the initial-commit global preflight validates all approved files and intended configuration bytes. Immediately before each mutation, its preconditions are checked again. The c-01 classifier is centralized and phase-aware:

- incompatible during plan/initial validation → `repository_state_incompatible`;
- authorized-snapshot drift before first mutation → `plan_stale`;
- change after successful immediate operation revalidation → `concurrent_change`.

Once mutation begins, the causal reason is retained but status derives from all effects: `failed` only for proven zero effects, `partially_applied` for one or more verified effects with incomplete result, and `state_indeterminate` whenever any relevant effect cannot be established.

### Initial commit and index

The initial commit is one authorized operation group:

1. global preflight over every approved path and in-memory config;
2. second metadata/identity barrier;
3. `hash-object --no-filters` from retained bytes and OID verification;
4. exact tree construction;
5. unsigned `commit-tree` using frozen author/committer/message bytes;
6. initial-ref CAS from absent to commit;
7. exclusively create and verify descriptor-owned `.git/index.lock`, then revalidate the normal index;
8. construct and verify the autonomous index-v2 outside the repository, copy it through the retained lock descriptor, fsync, revalidate ownership, rename to `index`, and fsync the Git directory;
9. HEAD/ref/commit/tree/index/status verification.

The prepared index is parsed independently before installation: `DIRC`, version 2, entry count, paths, modes, OIDs, checksum, and absence of `link`/shared-index dependencies. A Git Flow development-ref CAS is a distinct later operation. Neither operation performs rollback.

### In-process configuration, attributes, conversions, and ignores evaluator (P-05/N-03)

`gitmanager.ConfigEvaluator`, `AttributeEvaluator`, and `IgnoreEvaluator` perform no Git invocation. They read bounded, descriptor-verified system/global/XDG/local sources, resolve supported includes with depth/cycle checks, and never execute an include command, helper, hook, filter or configured executable. The attribute parser supports C-style quoting/escapes, directory-relative matching, recursive `**`, attribute states, top-level macros and last-match precedence; because Git forbids negative attribute patterns, a `!pattern` attribute rule is rejected as unsupported instead of treated as an ignore-style negation. For each candidate it evaluates, from lowest to highest precedence, frozen system attributes associated with the toolchain, explicit/default global attributes, the physical-root `.gitattributes` hierarchy, and—only for an existing repository—`$GIT_DIR/info/attributes`.

The ignore parser separately supports Git ignore negation, anchoring, directory-only rules, recursive matching, escapes and per-directory precedence. It resolves applicable `core.excludesFile`, `$GIT_DIR/info/exclude`, and root-to-leaf `.gitignore` sources. Every source identity/digest and winning candidate rule is frozen in the snapshot. `core.attributesFile`, `core.excludesFile`, and default system/global locations are discovered from the frozen toolchain installation and safely parsed configuration; inability to prove the location or semantics fails closed.

The outputs are per-file `AttributeEvaluation` and `IgnoreEvaluation` values containing reversible path bytes, ordered source digests, matched rules, effective values, external-code requirements and equivalence proofs. New projects treat repository-local config/info sources as absent and include known post-init values without creating `.git`. Matching neutral rules may pass. Unknown grammar, unreadable/ambiguous source, active code/encoding, or any conversion/ignore result whose ordinary Git equivalence cannot be proven returns `attributes_incompatible`. Differential tests compare only safe inert fixtures against frozen Git and never override a fail-closed evaluator decision. A globally ignored allowlisted file remains ignored and requires reinforced ignored-file approval.

### Exact Git operation matrix (P-04)

Every row is a closed typed operation. `<git>` is the frozen launcher/direct executable; arguments are separate `argv` elements, never shell text. All path-bearing commands set `GIT_LITERAL_PATHSPECS=1` and place paths after `--` where the subcommand supports it. The common safe environment sets `GIT_TERMINAL_PROMPT=0`, `GCM_INTERACTIVE=never`, `GIT_ASKPASS=`/`SSH_ASKPASS=`, `GIT_PAGER=cat`, `PAGER=cat`, `GIT_EDITOR=true`, `GIT_SEQUENCE_EDITOR=true`, `GIT_OPTIONAL_LOCKS=0` for read-only commands, and contains no inherited `GIT_*`. The in-process evaluator alone reads the real frozen HOME/XDG/system/local sources.

`<state-root>` always means the canonical Git state root, including its single final `git` segment: `$HOME/Library/Application Support/ai-dev/state/git` on macOS; on Linux, absolute `$XDG_STATE_HOME/ai-dev/git`, or `$HOME/.local/state/ai-dev/git` when XDG_STATE_HOME is unset. The host process resolves this root once from the real user context before constructing the private subprocess environment; relative XDG_STATE_HOME is invalid, and real HOME/XDG are never exposed to Git subprocesses. Every Git subprocess sets HOME to `<state-root>/environment/nonexistent-home-v1` and XDG_CONFIG_HOME to `<state-root>/environment/nonexistent-xdg-v1`; no additional `git` segment is appended. Both fixed leaf paths MUST be absent, their common physical parent MUST be private/validated, and they MUST be checked absent before and after every invocation. They are never created, vary neither by attempt nor repository, and therefore add no owned cleanup resource.

The resolved absolute state root, reversible bytes of both environment paths and environment-profile version are frozen in the precondition snapshot, included in canonical-plan/digest coverage and in each applicable deterministic `operation_id`; the runner MUST use those exact bytes and reject drift. The frozen toolchain capability evidence MUST demonstrate that every allowed argv treats the leaves as read-only lookup locations; otherwise plan/apply fail closed. Subprocesses also set `GIT_CONFIG_NOSYSTEM=1` and null global/system configuration paths. The planner replays as sorted exact `-c` arguments only safely supported semantic values needed for ordinary status/ignore equivalence, including a descriptor-verified `core.excludesFile` path when applicable. A value that cannot be replayed without executing code makes the command ineligible and plan/apply reject. Dynamic `filter.<driver>` overrides are generated from the safely parsed driver-name set and set clean/smudge/process to empty plus required=false; commands are not run when a matching non-neutral filter exists.

Configuration and attribute sources are parsed and classified in process before any repository-aware Git command. Git is never invoked to discover whether a configuration is unsafe. A read-only Git inspection row may run only after the parser proves its effective command/filter/hook/helper surface inert under the listed overrides; otherwise inspection reports the incompatibility and plan/apply reject.

The versioned baseline override vector, inserted before each subcommand as separate arguments where relevant, is: `-c core.fsmonitor=false`, `-c core.hooksPath=<frozen-nonexistent-safe-path>`, `-c credential.helper=`, `-c credential.interactive=never`, `-c core.askPass=`, `-c core.editor=true`, `-c sequence.editor=true`, `-c core.pager=cat`, `-c diff.external=`, `-c commit.gpgSign=false`, `-c tag.gpgSign=false`, `-c core.sshCommand=false`, and `-c protocol.allow=never`. The planner adds sorted empty `filter.<driver>.clean`, `.smudge`, `.process` and `required=false` overrides for every safely parsed driver. Unsupported override semantics are `git_capability_missing`; an invocation that would need network, a configured executable, or an active non-neutral transform is not run.

| Operation | Exact argv / stdin | Additional overrides | Declared effects and postconditions |
|---|---|---|---|
| Identify Git | `<git> --version`; `<git> --exec-path` | frozen `DEVELOPER_DIR`/launcher mode | No project write; exact tool identity/capabilities |
| Discover repo | `<git> rev-parse --path-format=absolute --git-dir --git-common-dir --show-toplevel` | inspect profile | No optional lock; physical outputs re-resolved |
| HEAD state | `<git> symbolic-ref -q HEAD`; `<git> rev-parse --verify HEAD^{commit}` | inspect profile | Classify unborn/attached/detached/invalid and history |
| Refs | `<git> show-ref --head -d` | inspect profile | Bounded ref/OID snapshot only |
| Status | `<git> status --porcelain=v2 -z --untracked-files=all --ignored=matching --no-ahead-behind --` | `core.fsmonitor=false`, safe filters, no pager | No refresh/write; NUL-safe bounded state |
| Worktree diff | `<git> diff --no-ext-diff --no-textconv --raw -z --no-renames --` | safe filters/external diff disabled | No external driver; exact raw entries |
| Index diff | `<git> diff --cached --no-ext-diff --no-textconv --raw -z --no-renames --` | safe filters/external diff disabled | No index refresh; exact cached entries |
| Validate ref | `<git> check-ref-format refs/heads/<name>` after rejecting NUL/control/LF/CR, leading `-`, `@{-N}` syntax and non-round-tripping bytes | no repository/config execution | Full ref validation; no previous-checkout shorthand |
| Validate object | `<git> cat-file -e <oid>^{commit}` then `<git> cat-file -t <oid>` | no replacement/object helper | Exact accessible commit and type |
| Initialize | `<git> init --template= --object-format=sha1 --ref-format=files --initial-branch=<name> -- <physical-root>` | frozen toolchain; templates/hooks disabled | Explicit `.git` tree, HEAD and local core config; exact created paths/keys are operation effects |
| Blob | `<git> hash-object -w --stdin --no-filters` | object profile; approved bytes stdin | One object; returned OID/content/type verified |
| Tree | in-process canonical Git-tree encoding, then `<git> hash-object -t tree -w --stdin` | object profile | One tree object; sorted entries and bytes independently verified |
| Commit | `<git> -c commit.gpgSign=false commit-tree <tree-oid>` | exact `GIT_AUTHOR_*`/`GIT_COMMITTER_*`; approved message stdin; no `-S` | One unsigned commit; headers/message/OID verified |
| Ref CAS | `<git> -c core.logAllRefUpdates=false update-ref --no-deref <ref> <new-oid> <expected-old-or-empty>` | apply profile | Exact CAS only; reflog suppressed and absence verified |
| Local config read | `<git> config --local --null --show-origin --show-scope --list` | inspect profile after safe config parsing | No mutation; sensitive values redacted |
| Local config write | `<git> config --local --replace-all -- <key> <value>` when the frozen Git capability probe confirms `--`; otherwise reject a leading-dash value and use the capability-tested separator-free argv | apply profile; one validated key/value per operation | `config.lock` temporary and exact key/value effect; stop at first failure |
| Prepared index | `<git> -c core.splitIndex=false -c core.fsmonitor=false -c core.untrackedCache=false -c index.version=2 -c index.skipHash=false read-tree <tree-oid>` | private `GIT_INDEX_FILE`; apply profile | Temporary index/lock roles only; parsed autonomous v2 result |
| Verify commit/tree | `<git> cat-file -t <oid>` and `<git> cat-file -p <oid>` | inspect profile; bounded binary-safe capture | Expected type/content/OID |
| Verify index/status | status/diff rows above plus independent index parser | safe read-only profile | Exact entries, clean authorized paths, unauthorized paths preserved |

`git init` implicit effects are not hidden: the operation enumerates every created Git path plus expected `HEAD`, repository format, `core.repositoryformatversion`, `core.filemode`, `core.bare`, `core.logAllRefUpdates`, and platform-derived `core.ignoreCase`/`core.precomposeUnicode` when Git writes them. `core.sharedRepository` is read in process from every real scope, but an ambient system/global/XDG value is neither replayed nor effective in the fixed private subprocess profile and therefore does not by itself block a new repository. A repository-local/worktree value, or any value deliberately replayed by the plan, that could alter permissions or undeclared effects is rejected before the affected mutation as `configuration_conflict`. Unexpected keys or paths fail postcondition verification. Ref CAS disables reflog creation; any unexpected reflog is a postcondition failure. `operation_id` hashes the exact typed operation, argv, stdin digest, environment-profile version, overrides, preconditions and effects—never runtime temp paths or apply IDs.

All user-controlled Git Flow prefixes and branch values reject NUL, C0/C1 controls, LF/CR, leading `-`, ambiguous/reversible-path failures and any byte sequence that cannot form the intended full ref. The literal initial branch `HEAD` is rejected during planning as `invalid_strategy`, exit 2, before authorization or mutation. Each planned topic prefix is validated by composing representative full `refs/heads/<prefix><leaf>` names and invoking the full-ref validation row; `@{-N}` is never accepted as a branch name. These validated bytes, exact argv and profile version are operation-ID inputs.

### Strategy inputs and existing-history development CAS (P-06, m-05, m-11)

| Strategy | Required | Forbidden |
|---|---|---|
| `basic` | explicit `--initial-branch` for new repo | all Git Flow/trunk options and `--development-base-oid` |
| `github-flow` | explicit `--initial-branch` primary branch for new repo | Git Flow/trunk options and `--development-base-oid` |
| `trunk-based` | explicit `--trunk-branch`; for new repo it is also the exact initial branch | Git Flow options, separate initial branch value, `--development-base-oid` |
| `git-flow` | production/development; new configuration also dialect and full prefix/tag parameters; new repo initial=production | trunk option; `--development-base-oid` except the FR-046 existing-history case |

For an existing Git Flow repository, a complete valid profile with byte-equal requested values is adopted without writes; equal keys are explicit verified no-ops. No Git Flow keys means a new configuration whose every write is planned. Partial/mixed/unrecognized keys, or a complete profile differing from requested values, returns `strategy_conflict`; the MVP does not normalize or reconfigure it. For existing history with absent development, omission of `--development-base-oid` is `strategy_conflict`/exit 1. Malformed syntax, algorithm mismatch or forbidden context is `invalid_strategy`/exit 2. A nonexistent/inaccessible commit or mismatch with frozen production is `strategy_conflict`/exit 1. Only an explicit accessible production-tip OID derives `CreateDevelopmentRef`; the CAS has `expected-old=absent` and its operation ID must be present in authorization scope.

### Definitive global apply ordering (m-01, m-02, m-03, P-07)

1. Enforce input limits; create/fsync attempt record.
2. Resolve project and issue/locate authorization; audit refusals.
3. Validate approvals and untrusted plan; fresh-inspect and compare the authorized snapshot before asking the planner to classify current state. During apply, a new incompatibility relative to that snapshot is `plan_stale`, not initial `repository_state_incompatible`.
4. Run global initial-commit preflight when applicable.
5. Acquire device/inode apply lock, reread, and CAS `issued→claimed`.
6. Establish the explicit mutation order when present: deterministic init; ordered Git Flow config keys; typed `create_project_config_directory`; post-mkdir identity/mode/config-target barrier; atomic project-config write; initial-commit group; existing/new development-ref CAS. Execution begins only under step 7.
7. Before the first mutation, fsync `mutation_intent`; CAS `claimed→mutation_started`; fsync `operation_started`; execute once. Intent-write failure returns `audit_unavailable` and permits same-owner `claimed→issued` only after proving no durable intent/effect. Once intent is durable, `claimed` is consumed and orphan reassignment/retry is forbidden. A proven failed CAS returns `refused/audit_unavailable`; an unverifiable CAS returns `state_indeterminate/audit_failed_after_mutation`; both exit 1, prohibit subsequent Git mutation, preserve the last verified state, and require linked supervised reconciliation.
8. Before every later mutation, revalidate, fsync `operation_started`, execute once, fsync its result, verify postconditions, and stop at first failure.
9. Reinspect all effects and calculate provisional effect-based status/primary reason.
10. Audit and release owned non-apply resources, then apply lock, with start/result records and one deterministic `SecondaryFailure` per failed release.
11. Write/fsync the immutable prepared terminal result including the complete ordered `secondary_failures[]`.
12. Perform the sole post-lock owner-only CAS from `mutation_started`, comparing expected state/version/owner and atomically binding `terminal_result_digest` to the prepared-result digest. No previous digest is compared. Only successful verified CAS makes it conclusive; otherwise return indeterminate without retry/reuse. Another authorization may acquire the released lock but can neither transition nor replace this original result.
13. Render the same result; any secondary failure forces exit 1 while status remains effect-based.

### Filesystem capability profiles (m-07)

Capability is evaluated independently for physical project/Git dir, user-state/audit root, plan-output parent and prepared-index storage. The snapshot freezes GOOS, filesystem type/magic, mount/device identity, flags, stable decimal device/inode, and capability-profile version.

- Production profiles are only local APFS on macOS and local ext4 on Linux.
- Read-only, remote, NFS, SMB, FUSE, overlayfs, tmpfs and unknown filesystems are `filesystem_unsupported` for production plan/apply.
- tmpfs is allowed only for explicitly labeled non-durability unit/integration fixtures and never supplies native acceptance evidence.
- Required primitives are no-follow exclusive create, stable descriptor/device/inode identity, advisory interprocess exclusion, `0600/0700`, file fsync (plus Darwin full-sync where required), directory fsync, same-filesystem atomic rename/replace, and versioned CAS.
- User-state probes use a private directory to verify these primitives and clean only owned probe resources. Project inspect/plan performs no probe write inside the project; it uses the supported-profile/mount checks plus native certification, then apply verifies every actual syscall and fails conservatively.
- Plan output is rejected before creation on an unsupported parent filesystem; successful exclusive creation, sync and parent sync complete its operational check.

### Bounded inputs, paths, schemas and persistent identities (m-04, m-06, m-08–m-10, m-12)

- Plan JSON: 8 MiB bytes, nesting 32, 16,384 object members/array items, 1 MiB per string.
- Allowlist/approvals: 4,096 files; path at most 4,096 bytes and platform `NAME_MAX` per component; 64 MiB per authorized file; 512 MiB aggregate authorized bytes; 1 MiB commit message; 16 MiB aggregate attribute/config/ignore source bytes and 1 MiB per source.
- Derived operations: 8,192; expected postconditions: 16,384 in total across the plan-level list and every operation's expected postconditions; captured stdout/stderr: 4 MiB each; config include depth: 16 with cycle rejection.

Exact limits are accepted. One-unit/byte-over outcomes are fixed: plan/JSON/structure/count/path → `plan_invalid`; configuration/attribute/ignore sources → `attributes_incompatible`; authorized-file/aggregate bytes → `repository_state_incompatible`; commit message → `commit_message_invalid`; bounded command output → the closed reason of the evaluated source/operation. Checks run before authorization or mutation whenever the content is available; later overflow preserves that cause and uses effect-based status.
- Reversible path bytes are unpadded RFC 4648 base64url in JSON plus a required UTF-8 display field when representable; no normalization occurs.
- `project_id` is a random UUIDv4 created at planning, frozen as a field of `ImmutableIntent`, canonical-plan covered and written to `.ai-dev/config.json`; an existing valid value is preserved. It is not derived from physical identity, so clones retain project identity without colliding with local audit partitions.
- macOS state root is `$HOME/Library/Application Support/ai-dev/state/git`; Linux is absolute `$XDG_STATE_HOME/ai-dev/git` or `$HOME/.local/state/ai-dev/git`. Relative `XDG_STATE_HOME` is never resolved against CWD: inspect warns and plan/apply returns `audit_unavailable` before mutation.
- Inspect, plan and apply outputs use the explicit schemas in `data-model.md`; secret findings expose only reversible authorized path and finding category. Plan output includes every approval tuple.
- Exclusive audit publication uses no-replace platform primitives; truncated/unpublished temporaries are non-events and trigger supervised reconciliation rather than repair.

### Step 55 closure matrix

| Finding | Design resolution | Status |
|---|---|---|
| P-01 | Consumer-owned ports and app composition establish an acyclic Go DAG. | RESOLVED |
| P-02 | Apply lock key is path-free device/inode hash; record partition remains identity digest. | RESOLVED |
| P-03 | Normative 13-step protocol acquires/verifies `index.lock` before prepared-index construction. | RESOLVED |
| P-04 | Closed exact Git operation/environment/effect matrix above. | RESOLVED |
| P-05 | In-process evaluator with conservative rejection and no temporary repository. | RESOLVED |
| P-06 | Explicit canonical `--development-base-oid` intent and authorized CAS. | RESOLVED |
| P-07 | Structured secondary failures and prepared-result/terminal-CAS protocol. | RESOLVED |
| m-01 | Apply compares authorized snapshot before phase-aware planner result translation. | RESOLVED |
| m-02 | Intent, mutation transition, operation-start and all failure points ordered. | RESOLVED |
| m-03 | One 13-step apply order includes init, Git Flow, mkdir/config, commit and development. | RESOLVED |
| m-04 | Inspect/plan/apply schemas and non-sensitive secret findings are explicit. | RESOLVED |
| m-05 | Strategy option matrix is explicit. | RESOLVED |
| m-06 | Reversible path JSON encoding is base64url without normalization. | RESOLVED |
| m-07 | APFS/ext4 production profiles; tmpfs test-only. | RESOLVED |
| m-08 | No-replace publication and truncated-record handling are explicit. | RESOLVED |
| m-09 | Portable UUIDv4 `project_id` is independent of local filesystem identity. | RESOLVED |
| m-10 | Numeric parsing/enumeration/output bounds are fixed. | RESOLVED |
| m-11 | Git Flow adoption/no-op/conflict semantics are explicit. | RESOLVED |
| m-12 | macOS/Linux state roots and relative XDG behavior are explicit. | RESOLVED |
| m-13 | Quickstart includes concurrent-loser and repeated-applied cases. | RESOLVED |

### Step 57 focused-correction matrix

| Finding | Design resolution | Status |
|---|---|---|
| N-01 | Every release failure is an ordered structured secondary failure; any non-empty set exits 1. | RESOLVED |
| N-02 | Omission/invalid/inaccessible/mismatched development base has an exact reason/exit mapping. | RESOLVED |
| N-03 | Configuration, attributes and ignores are evaluated in process; Git runs only under a proved-equivalent frozen profile. | RESOLVED |
| N-04 | Approval tuples are a public projection of canonical authorized files, not an extra covered plan field. | RESOLVED |
| N-05 | `terminal_result_digest` and owner-only post-lock CAS preserve single-writer closure and document the cross-authorization window. | RESOLVED |
| N-06 | Durable mutation intent precedes mutation-state CAS and forbids orphan reassignment/retry. | RESOLVED |
| N-07 | Plan refusals use the explicit redacted `GitPlanRejectionOutput`. | RESOLVED |
| N-08 | `project_id` is an immutable-intent field and reproduces project config bytes. | RESOLVED |
| N-09 | File, aggregate-byte and message limits are fixed with boundary behavior and the Step 59 closed reason mapping. | RESOLVED |
| N-10 | Full-ref and prefix validation plus capability-tested option separation prevent argument ambiguity. | RESOLVED |
| N-11 | Git-correct attribute pattern handling and frozen system-source discovery fail closed. | RESOLVED |
| N-12 | Real `core.sharedRepository` scopes are evaluated; only an effective/replayed value capable of changing planned effects rejects pre-mutation. | RESOLVED |
| N-13 | Steps 55, 55-B and 57 decisions are recorded without invented identity, signature or timestamp. | RESOLVED |

### Step 59 closure matrix

| Finding | Design resolution | Status |
|---|---|---|
| M-1 | Every bounded input category has one closed-catalogue reason; exact boundaries pass and post-mutation status remains effect-based. | RESOLVED |
| M-2 | Durable intent consumes `claimed`; later invocations refuse, while the owner distinguishes proven failed CAS from unverifiable CAS without further mutation. | RESOLVED |
| n-01 | Ambient `core.sharedRepository` is not replayed; only an effective value capable of changing effects rejects as `configuration_conflict`. | RESOLVED |
| n-02 | Safe physical aliases and proved inactive inode reuse are distinguished from unsafe or indeterminate collisions. | RESOLVED |
| n-03 | Terminal CAS compares owner/state/version and atomically binds, rather than pre-compares, the prepared-result digest. | RESOLVED |
| n-04 | Git subprocesses use fixed absent HOME/XDG leaves under the validated state root, with no per-attempt lifecycle. | RESOLVED |

### Step 61 lock-reuse correction

| Finding | Design resolution | Status |
|---|---|---|
| M-3 | Persistent lock-file reuse depends on advisory-holder absence plus fresh project/resource identity and metadata integrity; prior authorization state is independent, consumed authority is never reused, and FR-180 reconciliation/new authorization remain possible. | RESOLVED |

### Step 63 L-1/L-2 closure

| Finding | Design resolution | Status |
|---|---|---|
| L-1 | Current advisory acquisition plus verified project/lock descriptors and namespace correspondence are sufficient; path history is not required, rename/move remains usable, and recycled tuples create independent documentary authority. | RESOLVED |
| L-2 | `<state-root>` includes exactly one final `git`; `environment/` is directly beneath it and exact HOME/XDG path bytes are frozen in plan/digest/operation IDs and execution. | RESOLVED |

### Platform split

Production files that require POSIX identity, advisory locking, descriptor/inode ownership, or directory fsync use:

```go
//go:build (darwin && !ios) || (linux && !android)
```

The fallback uses:

```go
//go:build !((darwin && !ios) || (linux && !android))
```

The fallback may support limited inspect but returns `platform_unsupported` before plan/apply mutation. Filesystem capability checks can still reject supported GOOS targets with `filesystem_unsupported`.

## Project Structure

### Documentation (this feature)

```text
specs/008-git-management/
├── spec.md
├── checklists/requirements.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    ├── cli.md
    ├── inspection-plan.md
    ├── authorization-audit.md
    └── apply-strategy.md
```

### Source Code (repository root)

```text
cmd/ai-dev/
├── main.go
└── *_test.go

internal/app/
├── gitinit.go
├── gitcommit.go
└── *_test.go

internal/gitmanager/
├── model.go
├── reason.go
├── inspect.go
├── plan.go
├── canonical.go
├── strategy.go
├── apply.go
├── initialcommit.go
├── index.go
├── reconcile.go
├── platform_posix.go
├── platform_other.go
└── *_test.go

internal/gitprocess/
├── runner.go
├── environment.go
├── identify.go
└── *_test.go

internal/gitstore/
├── store.go
├── record.go
├── store_posix.go
├── lock_posix.go
├── store_other.go
└── *_test.go

internal/conventionalcommit/
├── validate.go
└── validate_test.go

internal/report/
├── gitinit.go
├── gitcommit.go
└── *_test.go
```

Exact filenames may be combined during task decomposition, but package ownership and dependency direction are normative plan decisions.

## FR-to-Design Traceability

Every identifier is listed explicitly; commas group requirements sharing one implementable design decision, not numeric ranges.

| Explicit requirements | Design owner and verification focus |
|---|---|
| FR-001, FR-002, FR-003, FR-004, FR-005, FR-006, FR-007, FR-008, FR-009, FR-010 | `app`/CLI modes, TTY, no default, shared engines and authority separation |
| FR-011, FR-012, FR-013, FR-014, FR-015, FR-016, FR-017, FR-018, FR-019, FR-020, FR-021, FR-022, FR-023, FR-024, FR-025, FR-026, FR-027 | Inspector/process profiles, repository/HEAD/path state and redaction |
| FR-028, FR-029, FR-030, FR-031, FR-032, FR-033, FR-034 | In-process attribute/config evaluator, byte-equivalence proof, conservative rejection |
| FR-035, FR-036, FR-037, FR-038, FR-039, FR-040, FR-041, FR-042, FR-043, FR-044, FR-045, FR-046, FR-047, FR-048, FR-049 | Strategy matrix, full Git Flow profiles, explicit development-base CAS, no hooks/`git flow` |
| FR-050, FR-051, FR-052, FR-053, FR-054, FR-055, FR-056, FR-057, FR-058 | Bounded strict plan decoder, canonical regeneration, digest and c-01 translation |
| FR-059, FR-060, FR-061, FR-062, FR-063, FR-064, FR-065, FR-066, FR-067, FR-068, FR-069, FR-070, FR-071 | State roots, no-replace records, audit ordering, device/inode lock and CAS |
| FR-072, FR-073, FR-074, FR-075, FR-076, FR-077, FR-078, FR-079 | UUID project config plus atomic write/no-op/conflict and filesystem profiles |
| FR-080, FR-081, FR-082, FR-083, FR-084, FR-085, FR-086, FR-087, FR-088, FR-089, FR-090, FR-091, FR-092, FR-093 | Initial-commit opt-in, exact approvals, paths and full preflight/scanner |
| FR-094, FR-095, FR-096, FR-097, FR-098, FR-099, FR-100, FR-101, FR-102, FR-103, FR-104, FR-105, FR-106, FR-107, FR-108, FR-109, FR-110, FR-111 | Object format, frozen identities/time/message, signatures/templates and exact plumbing |
| FR-112, FR-113, FR-114, FR-115, FR-116, FR-117, FR-118, FR-119, FR-120, FR-121, FR-122, FR-123, FR-124, FR-125, FR-126, FR-127, FR-128, FR-129 | Repository compatibility, CAS, 13-step index, results/idempotency/recovery |
| FR-130, FR-131, FR-132, FR-133, FR-134, FR-135, FR-136, FR-137 | No network/credentials, APFS/ext4 profiles, unsupported fallback and regressions |
| FR-138, FR-139, FR-140, FR-141, FR-142, FR-143, FR-144, FR-145, FR-146, FR-147, FR-148, FR-149 | Nested-repo refusal, explicit operations/failures, authority, strategy freedom and conversions |
| FR-150, FR-151, FR-152, FR-153, FR-154, FR-155, FR-156, FR-157, FR-158, FR-159, FR-160, FR-161 | Stable physical identity, independent IDs, toolchain, exact approvals and autonomous index |
| FR-162, FR-163, FR-164, FR-165, FR-166, FR-167, FR-168, FR-169, FR-170, FR-171, FR-172, FR-173, FR-174, FR-175, FR-176, FR-177 | HEAD, argv, revalidation, effect classification, terminal release, canonical JSON, Unicode/attributes |
| FR-178, FR-179, FR-180, FR-181, FR-182, FR-183, FR-184, FR-185, FR-186, FR-187, FR-188 | Issuance/state CAS/reconciliation, branch inputs, safe output, path/index ownership and unified flow |
| FR-189, FR-190, FR-191, FR-192 | Global result classification, exact init, two-phase preflight/build and later development CAS |

Field-level mapping lives in [data-model.md](./data-model.md), exact commands and interfaces in [contracts/](./contracts/), and every AC has an individual quickstart row.

## AC-to-Verification Traceability

All AC-001–032 are mapped individually to fixtures and expected results in [quickstart.md](./quickstart.md). The validation layers are:

- unit: parsers, canonicalization, state machines, reason precedence, validators;
- contract: CLI/text/JSON schemas, exact argv/env, report status/reason/exit mapping;
- integration: real temporary repositories with frozen Git and fault-injected filesystem/process ports;
- concurrency/crash: locks, CAS, orphan claims, post-mutation audit failure, replaced `index.lock`;
- native acceptance: macOS/APFS and Ubuntu/Linux ext4;
- structural regression: package/build constraints and authority boundaries for Features 001–007.

No acceptance criterion is represented as executed during planning.

## Quality Gates

These commands are planned evidence, not executed by this phase.

| Gate | Planned command/evidence | Pass condition |
|---|---|---|
| Formatting | `gofmt -l` over changed Go files | No output |
| Static analysis | `go vet ./...` | Exit 0 |
| Full tests | `go test -count=1 ./...` | Exit 0 |
| Concurrency | `go test -race -count=20` over Git-management, store, app, and affected authority packages | Exit 0; no race |
| Structural selection | `go list` for darwin, linux, ios, android, windows | POSIX/fallback files selected exactly as planned |
| Cross-build | CLI/package builds for darwin, linux, ios-package, android-package, windows | Builds succeed; unsupported targets select fail-closed code |
| Contract/fault tests | CLI subprocess, hostile env, crash points, CAS/lock/index faults | Exact status/reason/exit and no unplanned effects |
| Native macOS | APFS acceptance package and artifact hashes | AC matrix passes on supported macOS environment |
| Native Linux | ext4 acceptance package and artifact hashes | AC matrix passes on supported Linux environment |
| Security | secret fixtures, no-network sentinels, hooks/filter/helper/fsmonitor sentinels | No configured code/network executes; sensitive bytes absent from audit |
| Dependency hygiene | `go list -m all` and `go.mod` review | Only module `ai-dev`; no third-party module or `go.sum`. A new dependency requires the plan amendment stated in Technical Context, and `govulncheck` becomes mandatory from that point |
| Fuzz/property | `go test` fuzz targets for plan decoder/canonical writer, attribute parser and Conventional Commit parser | No panic/hang; canonical round-trip and bounded rejection invariants hold |
| Regression | existing Features 001–006 suites and authority boundary tests | No behavior/authority regression |

## Phase 0 Output

[research.md](./research.md) records decisions for package boundaries, canonical JSON, Git invocation, object/index construction, authorization/audit storage, filesystem locks, platform selection, Conventional Commits, secret screening, and verification strategy. All alternatives are resolved without changing the spec.

## Phase 1 Outputs

- [data-model.md](./data-model.md): entities, fields, validation, relationships, state transitions, and persistence boundaries.
- [contracts/cli.md](./contracts/cli.md): complete command/option/exit contract.
- [contracts/inspection-plan.md](./contracts/inspection-plan.md): inspection and canonical-plan interfaces.
- [contracts/authorization-audit.md](./contracts/authorization-audit.md): authorization state machine and durable record protocol.
- [contracts/apply-strategy.md](./contracts/apply-strategy.md): apply sequencing, reasons, Git strategies, initial commit, CAS, and index.
- [quickstart.md](./quickstart.md): proposed end-to-end validation guide with AC-001–032 mapping.

## Post-Design Constitution Re-check

PASS. Phase 1 retains specification-first sequencing, isolates new complexity at explicit trust/durability boundaries, provides objective validation for all ACs, introduces no network/service/container authority, and keeps Human Gates and Workflow State outside Git authorization. The later task list or PR must record the implementing and independent reviewing agents plus findings/resolution under Constitution VIII. No exception or new human decision is required.

## Complexity Tracking

| Complexity | Why needed | Simpler alternative rejected because |
|---|---|---|
| Separate `gitprocess` package | Git requires exact argv, frozen executable identity, controlled environment, bounded I/O, and no shell/configured-code execution. | Widening the general `system.Executor` would risk Features 001–003 and still mix ordinary process execution with a security boundary. |
| Separate `gitstore` package | Durable CAS authorization, append-only audit, fsync, external locks, and platform fallback form an independent persistence trust boundary. | Reusing `gatestore` would couple Git authority to Human Gate evidence, violating FR-009/144. |
| Schema-restricted RFC 8785 writer | Plan digests require byte-identical canonical form, special property ordering, exact strings, and rejection of floats/unknowns. | Plain `encoding/json` output is not a sufficient normative claim of RFC 8785 canonicalization. |
| Independent index parser/verifier | The installed index must be autonomous v2 with exact entries/checksum and no shared-index reference. | Trusting only Git command success cannot prove required binary postconditions or protect against ambient index settings. |
| In-process configuration, attribute, conversion and ignore evaluators | F008-FR-028–034 and F008-FR-175 require knowing effective conversions and ignore status without ever letting Git execute a configured filter, helper, hook or include command. Cost: a reimplementation of Git semantics that must match Git exactly or fail closed, so real configurations it cannot prove (for example unsupported include forms) are rejected as `attributes_incompatible`; it is the largest correctness and maintenance surface of the feature. | Running Git under an isolated environment and verifying status after the fact detects a divergence only once objects exist, and asking Git whether a configuration is unsafe is itself an invocation under that configuration. |
| Durable authorization, audit and apply-lock kernel | F008-FR-059–071 and F008-FR-178–180 require single-use authority, audit before every business result, and conservative classification after a crash. Cost: a CAS state machine, no-replace record publication, device/inode locks and a reconciliation path, all needing fault-injection and race evidence. | An in-process confirmation with no durable record cannot prove after a crash whether mutation began, and cannot stop a second process from consuming the same authorization. |
| Supported-filesystem allowlist (local APFS, local ext4) | F008-FR-133–136 require proven exclusive create, fsync, atomic rename and stable identity. Cost: plan/apply refuse overlayfs, tmpfs, NFS, SMB and FUSE, which excludes most containers and CI runners; only inspect works there. | Accepting any POSIX filesystem would assert durability the implementation cannot verify. |
| In-process high-risk secret screening | F008-FR-085 and F008-FR-091–093 require refusing an initial commit that contains suspected secrets without sending content to an external tool. Cost: a maintained pattern corpus with unavoidable false negatives and positives. | An external scanner is a configured executable and a new dependency, and would see file contents outside the audit boundary. |

**Scope decision (2026-11-17).** An architecture review before Phase 2 proposed reducing Feature 008 to an inspect/plan/init MVP that would defer the four rows added above. The project owner chose to retain the full specified scope, so those rows record complexity that Principle III requires to be explicit. This paragraph was written by the implementing agent from that instruction; the version-controlled approval is the owner's own commit or review of this change.
