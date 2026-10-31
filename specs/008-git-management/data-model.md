# Data Model: Feature 008 — Git Management

**Date**: 2026-10-11  
**Normative source**: [spec.md](./spec.md)

All JSON decoders are strict and bounded. Unknown members, duplicate keys, invalid UTF-8 where UTF-8 is required, floating-point values, excessive size/depth/count, and unstable representations are rejected. Device/inode and unsafe-range integers use unsigned decimal strings.

## 1. Identifier types

| Type | Representation | Rules |
|---|---|---|
| `PlanID` | opaque string/UUID | Metadata identity only; excluded from canonical plan digest. |
| `PlanDigest` | `sha256:<64 lowercase hex>` | SHA-256 of canonical covered plan fields. |
| `InspectionID` | opaque string/UUID | Observation correlation only; excluded from canonical plan digest. |
| `AuthorizationID` | canonical UUID supplied by operator | Unique exclusive-create key; not derived solely from plan digest. |
| `ApplyID` | generated canonical UUID | Fresh for every minimally valid apply invocation. |
| `OperationID` | deterministic string | Zero-based ordinal plus SHA-256 of canonical operation content; excludes runtime IDs/tokens/temp paths. |
| `ProjectIdentityDigest` | `sha256:<64 lowercase hex>` | Physical path + NUL + decimal device + NUL + decimal inode. |
| `ContentDigest` | `sha256:<64 lowercase hex>` | Exact authorized bytes. |
| `BlobOID` | Git object-format OID | SHA-1 for new repositories; existing repository format preserved. |

IDs are not interchangeable and do not prove a human identity.

## 2. PhysicalProjectIdentity

```text
PhysicalProjectIdentity
  physical_path_utf8
  reversible_path_bytes
  device_decimal
  inode_decimal
  digest
```

Validation:

- Physical path is canonical and derived without following ambiguous components.
- Device/inode match `0|[1-9][0-9]*` and remain strings at all magnitudes.
- Digest uses the exact formula in the specification.
- mtime, ctime, directory size/count, Git state, and contents are excluded.
- Identity remains stable across `git init`; inability to prove stability yields `project_identity_unavailable`.

## 3. GitToolIdentity

```text
GitToolIdentity
  invocation_mode          launcher | direct
  invocation_path
  invocation_path_digest
  invocation_device
  invocation_inode
  effective_executable_path
  effective_executable_digest
  version
  exec_path
  capabilities[]
  environment_selectors{}
```

On macOS, launcher and effective executable are distinct when applicable. `DEVELOPER_DIR`/xcode-select context is frozen. Apply requires exact identity and capability equality; no fallback is allowed.

## 4. GitInspection

```text
GitInspection
  schema_version
  inspection_id
  inspected_at
  project_identity
  repository_kind
  repository_state
  git_dir?
  common_dir?
  object_format?
  ref_format?
  head
  history_present
  refs[]
  index
  worktree
  remotes_redacted[]
  git_flow_evidence
  effective_configuration[]
  attribute_sources[]
  ignore_sources[]
  ignore_evaluations[]
  conversion_settings
  linked_worktrees[]
  submodules[]
  filesystem_capabilities
  tool_identity
  warnings[]
  incompatibilities[]
```

Enums:

- `repository_kind`: `absent | normal | bare | linked_worktree | submodule | nested | ancestor | inaccessible | ambiguous`.
- `head.state`: `unborn | attached | detached | invalid`; attached also carries exact symbolic-ref bytes.
- `index`: presence, digest, empty flag, lock state, staged/conflicted/type/mode entries.
- `worktree`: unstaged/untracked/ignored entries and reversible path identities.

Inspection never grants mutation authority. Sensitive remote/config values are redacted before entering the entity.

Observed paths use unpadded RFC 4648 base64url in `reversible_path_bytes_base64url` plus an optional UTF-8 display field; neither field is silently normalized.

### AttributeEvaluation

```text
AttributeEvaluation
  reversible_path_bytes_base64url
  display_path_utf8?
  ordered_source_digests[]
  matched_rules[]
  effective_attributes
  effective_conversion_config
  external_filter_required
  byte_identity_proven
  incompatibility_reason?
```

It is produced entirely in process. Unknown grammar, ambiguous/unreadable sources, active configured code, or unproved conversion equivalence is incompatible.

### IgnoreEvaluation

```text
IgnoreEvaluation
  reversible_path_bytes_base64url
  display_path_utf8?
  ordered_source_digests[]
  matched_rules[]
  ignored
  winning_rule_source?
  equivalence_proven
  incompatibility_reason?
```

It is produced in process from safely parsed system/global/XDG/local configuration, `core.excludesFile`, `.git/info/exclude`, and root-to-leaf `.gitignore` sources. Negation, directory-relative matching, escapes and precedence are explicit. Git subprocess output may corroborate only an inert supported profile and never overrides a failed equivalence proof.

## 5. ImmutableIntent

```text
ImmutableIntent
  project_id
  strategy
  git_flow_dialect?
  branch_parameters
  development_base_oid?
  frozen_production_ref?
  frozen_production_tip_oid?
  prefix_parameters
  initialize_repository
  object_format
  ref_format
  tool_identity
  project_config_bytes_digest
  initial_commit
  author
  committer
  author_timestamp
  committer_timestamp
  commit_message_bytes
  authorized_files[]
  required_capabilities[]
```

Rules:

- `strategy`: exactly `basic | git-flow | github-flow | trunk-based`.
- `project_id` is the preserved or newly generated UUIDv4 used to reproduce exact `.ai-dev/config.json` bytes and is canonical-plan covered.
- No branch name or strategy is inferred.
- Git Flow requires explicit `avh | git-flow-next` for a new configuration.
- New repositories freeze `sha1`, `files`, and an explicit initial branch.
- Identity, timestamps/offsets, exact message bytes, and file approvals are frozen at plan time and not reread from Git config during apply.
- Existing-history development creation requires all three development/production fields, exact object-format match, accessible commit equality, target-absent CAS and authorization of its operation ID.

## 6. AuthorizedFile

```text
AuthorizedFile
  path_utf8
  reversible_path_bytes_base64url
  mode
  content_sha256
  blob_oid
  ignored
  approval_kind          normal | ignored
  individually_approved
  observed_identity
  observed_metadata
```

Rules:

- Literal regular files only; no glob, directory, absolute path, traversal, symlink component, control, NUL, or `|`.
- `mode` is an authorized Git mode; `.ai-dev/config.json` is mandatory and `100644` for initial commit.
- Approval tuple is exactly `PATH|MODE|sha256:BYTES_DIGEST|blob:BLOB_OID`.
- Ignored files require the ignored approval form.
- The complete approval set equals the allowlist exactly.

## 7. PreconditionSnapshot

```text
PreconditionSnapshot
  project_identity
  repository_identity
  repository_kind
  head_and_history
  refs[]
  index_state
  worktree_path_identities[]
  effective_config_with_origins[]
  project_config_identity
  git_flow_state
  attribute_and_conversion_sources[]
  ignore_and_configuration_sources[]
  worktree_and_submodule_state
  filesystem_and_lock_state
  tool_identity
```

The snapshot is complete and digest-covered. It distinguishes:

- incompatible state present during planning (`repository_state_incompatible`);
- drift against the authorized snapshot before mutation (`plan_stale`);
- change after successful immediate revalidation (`concurrent_change`).

`filesystem_and_lock_state` contains one profile per project/Git dir, state/audit root, plan-output parent and prepared-index location:

```text
FilesystemCapabilityProfile
  role
  goos
  filesystem_type
  mount_identity
  device_decimal
  inode_decimal
  mount_flags[]
  local
  read_only
  profile_version
  exclusive_create
  advisory_lock
  file_sync
  directory_sync
  atomic_same_filesystem_rename
  atomic_replace
  stable_identity
  production_supported
  probe_evidence_digest?
```

## 8. DerivedOperation

```text
DerivedOperation
  operation_id
  ordinal
  kind
  exact_argv[]?
  environment_profile?
  target_identity
  preconditions[]
  affected_paths[]
  affected_refs[]
  affected_config_keys[]
  expected_effects[]
  expected_postconditions[]
  failure_outcomes[]
  partial_effect_classification
  recovery_instruction
  temporary_resource_roles[]
```

Allowed operation kinds are a closed planner-owned enum, including audit writes, authorization transitions, apply-lock actions, deterministic repository init, individual Git config writes, `create_project_config_directory`, atomic project-config write, initial-commit group, initial/development ref CAS, prepared-index construction/installation, verification, and owned-resource release. `create_project_config_directory` has an absent-or-compatible-directory precondition and is followed by a barrier that revalidates directory identity, mode and the still-absent config target before any config temporary is created.

Concrete apply IDs, owner tokens, PIDs, audit timestamps, and temporary paths are excluded from canonical arguments. An operation from an input file is never dispatched directly.

## 9. GitInitializationPlan

```text
GitInitializationPlan
  schema_version          ai-dev.git-init-plan/v1
  plan_id                 excluded metadata
  generated_at            excluded metadata
  expires_at?             excluded metadata
  inspection_id           excluded metadata
  inspected_at            excluded metadata
  immutable_intent        covered
  precondition_snapshot   covered
  derived_operations[]    covered
  expected_postconditions[] covered
  prohibited_operations[] covered
  partial_effect_points[] covered
  recovery_policy         covered
  plan_digest
```

Canonical form:

- RFC 8785 UTF-8 JSON.
- Unknown/duplicate keys and floats forbidden.
- Covered fields are exactly those listed as covered.
- Approval tuples in `GitPlanOutput` are a deterministic public projection of `immutable_intent.authorized_files[]`; they are not an additional `GitInitializationPlan` field and do not expand the exact covered-field list in F008-FR-172.
- Excluded metadata cannot affect authority, operation, precondition, or postcondition.
- Regeneration requires byte-identical canonical covered form, not semantic similarity.

`prohibited_operations` contains at least push/fetch/pull/merge/rebase/tag/remote mutation/branch delete-or-rename/reset/checkout/switch/stash/gc/amend/restore/submodule or linked-worktree mutation/hooks/`git flow feature finish`.

## 10. GitStrategyReport

```text
GitStrategyReport
  schema_version
  project_identity_digest
  selected_strategy
  source
  git_flow_dialect?
  branch_parameters
  detected_evidence[]
  compatibility
  pending_dependencies[]
  warnings[]
  grants_mutation_authority = false
```

The report contains neither credentials, authorization, reusable plan, nor mutation capability. Feature 007 may consume it read-only.

## 11. ProjectConfig

```text
ProjectConfig (.ai-dev/config.json)
  schema_version
  project_id
  strategy
  strategy_parameters
```

Rules:

- Canonical strategy names only; `trunk-based` has no aliases.
- No credentials or audit/authorization data.
- Deterministic JSON bytes rendered at plan time.
- Existing valid equivalent content is a no-op; divergent content is `configuration_conflict`.
- `project_id` is a cryptographically generated UUIDv4 frozen at planning and preserved from an existing valid config. It is not derived from local physical identity, so a clone preserves project identity without sharing local authorization state.

## 12. GitAuthorization

```text
GitAuthorization
  schema_version
  authorization_id
  plan_digest
  project_identity
  authorized_operation_ids[]
  strategy_and_dialect
  authorized_refs_and_config
  initial_commit_authorized
  approved_files[]
  declared_operator
  issued_at
  expires_at?
  state
  state_version
  owner_apply_id?
  terminal_result_digest?
  previous_authorization_id?
  previous_apply_id?
  previous_plan_digest?
  immutable_transition_history[]
```

State machine:

```text
issued
  └─ claimed
      ├─ issued                  clean pre-mutation release
      ├─ claimed                 proven orphan reassignment
      └─ mutation_started
          ├─ applied
          ├─ failed
          ├─ partially_applied
          └─ state_indeterminate
```

Every transition is CAS over authorization ID, current state, version, and owner. Terminal states never transition. A `claimed` authorization with durable `mutation_intent` is consumed and cannot be reassigned or retried even though intent alone does not prove Git mutation. `mutation_started` cannot transfer or authorize a retry. `failed` is terminal and only valid after mutation started plus proof of zero effects across the entire attempt.

A later attempt against consumed `claimed` returns `refused/authorization_consumed`. The original owner that proves `claimed→mutation_started` did not occur returns `refused/audit_unavailable`; if the CAS outcome cannot be established reliably it returns `state_indeterminate/audit_failed_after_mutation`. Both exit 1, prohibit subsequent Git mutation/retry, preserve the last verified durable state, and create only new linked reconciliation records.

`terminal_result_digest` is absent before closure and is set only by the owner-only terminal CAS from `mutation_started`. The CAS compares authorization ID, expected state/version and owner apply ID, then atomically binds the prepared-result digest; it does not compare with a nonexistent prior digest. Later attempts may verify the bound value. Reconciliation and unrelated authorizations never replace it.

## 13. GitApplyAttempt and audit records

```text
GitApplyAttempt
  schema_version
  apply_id
  authorization_id?
  declared_plan_digest?
  project_identity?
  started_at
  invocation_mode
  result_status?
  result_reason?
```

Audit event variants:

- `attempt_created`
- `authorization_issued`
- `authorization_transition`
- `mutation_intent`
- `operation_started`
- `operation_completed | operation_failed | operation_indeterminate`
- `resource_release_started`
- `resource_release_result`
- `prepared_terminal_result`
- `terminal_authorization_cas`

Each record is immutable, sequenced, fully written/fsynced before no-replace publication, directory-fsynced, and contains only non-sensitive metadata. A prepared terminal result is conclusive only when a successful terminal CAS references its digest. Truncated/unpublished temporaries are not events; an intent without a conclusive result is indeterminate.

## 14. GitApplyResult

```text
GitApplyResult
  schema_version
  apply_id
  authorization_id?
  plan_id?
  plan_digest?
  status
  reason?
  secondary_failures[]
  prior_authorization_state?
  before_snapshot_digest?
  after_snapshot_digest?
  completed_operation_ids[]
  failed_operation_id?
  not_started_operation_ids[]
  indeterminate_operation_ids[]
  verified_effects[]
  commit_oid?
  index_tree_oid?
  development_branch_created?
  audit_status
  warnings[]
  findings[]
  recovery_required
```

```text
SecondaryFailure
  code                    resource_release_failed
  phase                   resource_release
  resource_role           apply_lock | index_lock | config_temporary | prepared_index
  operation_id?
  failure_class
  ownership_verified
  audit_record_digest?      latest relevant durable release record; absent if that record was not persisted

SecurityFinding
  reversible_path_bytes_base64url
  display_path_utf8?
  category
```

Neither structure contains secret bytes, content, concrete temporary paths, tokens or descriptors. `SecurityFinding` identifies a high-risk category without the match. Secondary failures are byte-equivalent in the prepared terminal record and rendered result.

`status` enum:

- `applied`
- `already_applied`
- `refused`
- `failed`
- `partially_applied`
- `state_indeterminate`

`reason` is the primary cause and is restricted to the closed catalogue in `spec.md`. Success may omit it or use the informational `development_branch_pending_initial_history`. Every release failure is a separate `SecondaryFailure`, ordered by planned release ordinal and resource role, including when it is the only failure. If release is the only failure it is `resource_release_failed`; otherwise the earlier reason is preserved. Pre-mutation domain refusal exits 1 except the explicit usage/output cases that exit 2. Post-mutation status is based on all effects, not the causal reason alone. Any non-empty secondary-failure list exits 1 even when status is `applied`.

## 15. Git audit/store layout

Logical layout (names are roles, not permission to expose concrete temp paths in a plan):

```text
<state-root>/
├── environment/              validated private parent; the two leaves below MUST remain absent
│   ├── nonexistent-home-v1
│   └── nonexistent-xdg-v1
├── locks/by-device-inode/<sha256-v1-device-inode>/apply.lock
└── projects/<project-identity-digest>/
    ├── authorizations/<authorization-id>.json
    └── attempts/<apply-id>/
        ├── attempt.json
        ├── events/<sequence>-<event-kind>.json
        └── result.json
```

Directories are `0700`; records are `0600`. Apply-lock existence is not ownership. Authorization replacement is atomic and version-checked. Event/result absence is meaningful and never repaired automatically.

`<state-root>` is exactly the canonical Git state root: `$HOME/Library/Application Support/ai-dev/state/git` on macOS; on Linux, absolute `$XDG_STATE_HOME/ai-dev/git`, or `$HOME/.local/state/ai-dev/git` when XDG_STATE_HOME is unset. A relative XDG_STATE_HOME is invalid. No consumer appends another `git` segment.

The environment leaf names are fixed lookup paths, not directories to create: HOME is `<state-root>/environment/nonexistent-home-v1` and XDG_CONFIG_HOME is `<state-root>/environment/nonexistent-xdg-v1`. Their common parent is private and validated; both leaves are checked absent before and after each allowed Git invocation and create no per-attempt cleanup role. The resolved absolute state root and exact reversible bytes of both paths are frozen in the environment profile/precondition snapshot, covered by the canonical plan and `plan_digest`, and included with the environment-profile version in each applicable `operation_id`; execution MUST use those same bytes.

The lock key excludes path and is `sha256("ai-dev-lock-v1\x00" || device-decimal || "\x00" || inode-decimal)`. Four concepts remain separate: the persistent advisory-lock file provides mutual exclusion; `PhysicalProjectIdentity` identifies the project; lock metadata is diagnostic; `GitAuthorization.state` controls mutation authority. Physical aliases of the same proven live directory contend on the same file.

An existing persistent lock file is reusable when it is opened safely, its descriptor identity/mode/ownership are valid, a nonblocking advisory lock proves no active holder, the project plus lock resource are freshly reverified, and the namespace key matches the current device/inode. Its existence, absent historical metadata, a changed documentary path, and a prior nonterminal/indeterminate authorization do not by themselves block reuse. A changed canonical path changes the documentary identity/partition and requires a new independent plan/snapshot/digest/authorization even though the same physical lock namespace may be reused. Diagnostic metadata MAY be rebuilt under the held lock by publishing a new durable audit record from those current proofs; historical records are never replaced.

For a recycled device/inode, the current physical identity creates its own documentary partition and authorization lineage. Active ownership, corrupt metadata that prevents current verification, ambiguous identity, or namespace mismatch produces `lock_conflict`; prior history is not required merely to reuse a currently verified free lock. The persistent file is never automatically deleted. Lock reuse grants no authorization and cannot reactivate a consumed authorization; record partitioning remains keyed by the full current physical identity digest.

### Output schemas

```text
GitInspectOutput
  schema_version
  inspection
  strategy_report?
  warnings[]
  incompatibilities[]

GitPlanOutput
  schema_version
  plan_id
  plan_digest
  canonical_plan_path
  strategy_report
  ordered_operations[]
  approval_tuples[]
  warnings[]

GitPlanRejectionOutput
  schema_version
  status                  refused
  reason
  exit_code               1 | 2
  findings[]
  warnings[]
  retryable               false

GitApplyOutput
  schema_version
  result                 GitApplyResult
```

All JSON modes emit exactly one object. A plan rejection contains no partial plan, operation, authorization, raw configuration, source content, credential, secret, or unsafe path detail. Plan approval tuples use exact normal/ignored CLI forms, contain no content, and are derived solely from canonical `immutable_intent.authorized_files[]`. Text output renders the same validated model.

### Bounded content model

The enforced maxima are: plan JSON 8 MiB, nesting 32, 16,384 members/items, 1 MiB per JSON string, 4,096 authorized files, 8,192 operations, 16,384 postconditions, 4,096 path bytes plus platform `NAME_MAX`, 64 MiB per authorized file, 512 MiB aggregate authorized bytes, 1 MiB commit message, 1 MiB per configuration/attribute/ignore source, 16 MiB aggregate such sources, and 4 MiB per captured stdout/stderr stream. Exact-boundary values are accepted. Structural/JSON/count/path excess yields `plan_invalid`; configuration/attribute/ignore source excess yields `attributes_incompatible`; authorized-file or aggregate-byte excess yields `repository_state_incompatible`; commit-message excess yields `commit_message_invalid`. Captured-output excess uses the closed reason of the source or operation being evaluated. These checks occur before authorization/mutation whenever the input is available; an overflow discovered after mutation retains its causal reason and uses effect-based status.

## 16. ConventionalCommitValidation

```text
ConventionalCommitValidation
  schema_version
  valid
  type?
  scope?
  description?
  body_lines[]
  footers[]
  breaking_change_recognized
  diagnostics[]
```

The validator distinguishes syntactic validity from breaking recognition. It accepts uppercase characters in type, recognizes `BREAKING CHANGE` and `BREAKING-CHANGE`, tests lowercase `breaking-change` explicitly, and accepts `Refs #123` footer syntax according to the normative corpus.

## 17. Relationships and lifecycle

```text
GitInspection ──freezes──► PreconditionSnapshot
       │                         │
       └── + ImmutableIntent ───► GitInitializationPlan
                                      │ plan_digest
                                      ▼
                               GitAuthorization
                                      │ claim/CAS
                                      ▼
                               GitApplyAttempt
                                      │ events/effects
                                      ▼
                                GitApplyResult
```

- One plan may be inspected many times but one authorization permits at most one mutation sequence.
- Every apply invocation has one attempt record, including refusals and already-applied checks.
- Reconciliation creates a new plan and authorization linked to, but never modifying, prior records.
- Human Gate and Workflow State entities have no relationship that grants Git authority.
