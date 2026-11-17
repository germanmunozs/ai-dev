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
| `ObjectOID` | Git object-format OID | Nominal identifier for commits, trees, gitlinks and observed HEAD objects; SHA-1 or SHA-256 according to the frozen repository object format. It is not interchangeable with `BlobOID` and does not resolve or prove object existence. |

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
  sequencer_state
  git_dir?
  common_dir?
  object_format?
  ref_format?
  head                                   HeadObservation (includes history_present)
  refs_observation_state
  refs[]                                 RefObservation
  index                                  IndexObservation
  config_lock_state                      absent | present | unobservable
  worktree_observation_state
  worktree_changes[]                     WorktreeChange
  worktree_path_identities[]             WorktreePathIdentity
  configuration_observation_state
  effective_configuration[]              ConfigEntry
  project_config                         ProjectConfigObservation
  remotes_observation_state
  remotes_redacted[]
  git_flow_evidence
  attribute_sources_observation_state
  attribute_sources[]
  ignore_sources_observation_state
  ignore_sources[]
  ignore_evaluations_observation_state
  ignore_evaluations[]
  conversion_settings
  linked_worktrees_observation_state
  linked_worktrees[]
  submodules_observation_state
  submodules[]
  filesystem_capabilities_observation_state
  filesystem_capabilities
  tool_identity
  warnings[]
  incompatibilities[]                    Incompatibility
```

Enums:

- `repository_kind`: `absent | normal | bare | linked_worktree | submodule | nested | ancestor | inaccessible | ambiguous`.
- `head.state`: `unborn | attached | detached | invalid`; attached also carries exact symbolic-ref bytes.
- `index`: presence, digest, empty flag, lock state, staged/conflicted/type/mode entries.
- `worktree`: unstaged/untracked/ignored entries and reversible path identities.

The typed shapes of `sequencer_state`, `head`, `refs`, `index`, `worktree_*`, `effective_configuration`, `project_config` and `incompatibilities` are defined in [Typed repository observation](#typed-repository-observation-schema-103b). Every member listed above except the inspection metadata (`schema_version`, `inspection_id`, `inspected_at`), `remotes_*`, `tool_identity`, `warnings` and `incompatibilities` forms the `RepositoryObservation` that a `PreconditionSnapshot` freezes.

Inspection never grants mutation authority. Sensitive remote/config values are redacted before entering the entity.

Observed paths use unpadded RFC 4648 base64url in `reversible_path_bytes_base64url` plus an optional UTF-8 display field; neither field is silently normalized.

The nine structured inspection groups are mandatory members of the inspection schema, with the following types and cardinalities. Every collection-valued group has the companion `ObservationState` shown in the schema so an empty list remains unambiguous; singular evidence types carry their observation state internally.

| Group | Type | Cardinality | Empty/absence semantics |
|---|---|---:|---|
| `remotes_redacted` | `RemoteRedacted` | `0..N` | Empty means a completed observation found no remotes; observation failure is represented separately, never by an ambiguous empty list. |
| `git_flow_evidence` | `GitFlowEvidence` | exactly `1` | Uses an explicit classification, including `absent` and `unobservable`. |
| `attribute_sources` | `AttributeSource` | `0..N` | Each applicable source, including an absent expected source, carries an explicit read state. |
| `ignore_sources` | `IgnoreSource` | `0..N` | Each applicable source carries an explicit read state and domain-specific precedence. |
| `ignore_evaluations` | `IgnoreEvaluation` | `0..N` | Empty means no candidate evaluation was required after a complete source observation. |
| `conversion_settings` | `ConversionSettings` | exactly `1` | Explicit risk and proof state distinguish no effective conversion from unknown or incompatible conversion. |
| `linked_worktrees` | `LinkedWorktree` | `0..N` | Empty means none were found after a complete observation. |
| `submodules` | `Submodule` | `0..N` | Empty means none were declared/found after a complete observation. |
| `filesystem_capabilities` | `FilesystemCapabilityProfile` | `1..N` when inspect can establish the required locations | Missing or unobservable profiles are explicit incompatibilities, not fabricated values. |

### Shared observation and path types

```text
ObservationState
  not_applicable
  not_observed
  observed_empty
  observed
  absent
  error

ObservedPath
  reversible_path_bytes_base64url
  display_path_utf8?

InspectedSourceIdentity
  scope
  reversible_path?
  physical_identity_digest?
  content_digest?
  read_state
  observed_size_decimal?
  precedence
```

`ObservationState` is closed. `absent` means the relevant location or declaration was conclusively checked and does not exist; `not_applicable` means the concept does not apply to this repository state; `not_observed` means inspection was not attempted or could not reach the observation boundary; `observed_empty` means observation completed and produced no entries; `observed` means data is present; and `error` means an attempted observation failed. `not_observed` and `error` require a warning or incompatibility explaining why planning cannot rely on the missing evidence. Empty collections MUST NOT simultaneously mean absent, not observed and error.

`ObservedPath` preserves exact path bytes. Its display value is present exactly when the bytes are valid UTF-8 and then MUST equal those bytes without normalization. Physical identity is present only when descriptor-safe observation established it. Source state is closed as `absent | read | unreadable | ambiguous | unsupported`; `read` requires the applicable identity and content digest, while `absent` prohibits a fabricated digest.

### RemoteRedacted

```text
RemoteRedacted
  name
  fetch_urls[]
  push_urls[]
  source_scope
  source_digest
  observation_state

RedactedRemoteURL
  ordinal
  display_redacted
  credential_material_detected
  redaction_state       not_needed | redacted | opaque
  original_digest?
```

Remote names are non-empty and contain no control characters. Fetch and push URL lists preserve observed order and multiplicity. The model MUST NOT retain an original URL, credentials, userinfo, tokens, or unredacted sensitive fragments. A URL that cannot be redacted with demonstrable safety is represented only by a fixed opaque marker and `redaction_state=opaque`.

`original_digest` is optional, is not an anonymization mechanism and MUST NOT be populated merely to hide the original URL. It MAY be present only when a concrete later contract requires stable correlation and the digest cannot reveal or correlate sensitive information; otherwise it is absent. Exact rendering syntax remains deferred to T046. No model needs to retain a secret in order to produce a redacted observation.

An empty fetch or push list means that direction was observed without a configured URL. `not_observed` and `error` are not represented as empty success. Remote data exposed through reports, errors, logs or serialization is limited to this already-redacted entity.

### GitFlowEvidence

```text
GitFlowEvidence
  observation_state
  classification
  dialect?
  entries[]
  production_branch?
  development_branch?
  prefixes[]
  instance_bases[]
  source_digests[]
  issues[]
```

The closed classification is:

```text
absent | complete_avh | complete_git_flow_next | partial | mixed |
contradictory | unrecognized | unobservable
```

`absent` requires completed observation and no `gitflow.*` entries. `complete_avh` and `complete_git_flow_next` require the exact corresponding normative profile, recognized dialect, required production/development branches and complete prefix/profile evidence. `partial` contains a proper subset of one recognizable profile. `mixed` contains keys from incompatible profiles. `contradictory` contains mutually inconsistent values. `unrecognized` contains observed `gitflow.*` data that cannot be classified safely. `unobservable` pairs with `not_observed` or `error` and records an issue.

Entries preserve exact safely parsed key/value evidence, origin and source digest in deterministic key/origin order. Instance `.base` entries are preserved separately. Evidence is informational: it does not select a strategy, grant authorization, execute Git Flow or turn partial/mixed/unrecognized evidence into `strategy_conflict` unless planning explicitly selects or adopts Git Flow.

### AttributeSource and IgnoreSource

The two source types are nominally distinct and reuse `InspectedSourceIdentity`; their values are not interchangeable.

```text
AttributeSource
  identity
  kind                  system_attributes | global_attributes_explicit |
                        global_attributes_default | repository_gitattributes |
                        info_attributes | post_init_intent

IgnoreSource
  identity
  kind                  core_excludes_file | info_exclude |
                        repository_gitignore | post_init_intent
  base_directory?
  directory_depth?
```

Attribute source precedence is system, explicit/default global, root-to-leaf repository `.gitattributes`, then existing-repository `info/attributes`. A repository `.gitattributes` source records its reversible path and directory-relative precedence. `post_init_intent` is used only for known intended values where the normative planning contract requires them; it MUST NOT fabricate a currently observed filesystem source.

Ignore sources preserve the independently defined ignore semantics. `repository_gitignore` requires a base directory and directory depth; `core_excludes_file` records its configuration origin; and `info_exclude` records Git-directory identity. Root-to-leaf precedence, negation and directory-relative interpretation remain specific to ignores and MUST NOT be reused as attribute semantics. `IgnoreEvaluation.ordered_source_digests` and `winning_rule_source` MUST refer to the corresponding ordered `IgnoreSource` observations.

No source type implements parsing or pattern evaluation. `unreadable`, `ambiguous` and `unsupported` are fail-closed source states and must be reflected in inspection incompatibilities.

### ConversionSettings

```text
ConversionSettings
  observation_state
  settings[]
  filter_drivers[]
  external_code_required
  byte_identity_proven
  risk                  none | neutral | may_transform |
                        external_code | unknown | incompatible
  source_digests[]
  incompatibility_reason?

EffectiveConversionSetting
  key
  configured_value_redacted?
  effective_value_redacted?
  source_scope
  source_digest
  interpretation_state
```

Settings are typed but extensible: the structure MUST cover the normative `core.autocrlf`, `core.eol`, `core.safecrlf`, `core.checkRoundtripEncoding`, `core.attributesFile`, `filter.*`, `working-tree-encoding`, `ident`, `text`, `eol` and `crlf` mechanisms without treating that list as permission to ignore another relevant conversion mechanism. Configured and effective values remain distinct and retain origin evidence.

`byte_identity_proven=true` is invalid with `external_code_required=true` or risk `may_transform`, `external_code`, `unknown` or `incompatible`. `risk=incompatible` requires `incompatibility_reason=attributes_incompatible`. `none` means complete observation found no relevant setting; `neutral` means every effective transformation was proven byte-neutral. Filters and configured commands are never executed to populate this model.

### LinkedWorktree

```text
LinkedWorktree
  observation_state
  worktree_path
  git_dir_path?
  common_dir_identity?
  head                  HeadObservation (SCHEMA-103B-01)
  availability          available | missing | locked |
                        inaccessible | ambiguous
  locked
  lock_reason_redacted?
  prunable?
```

Available worktrees require a reversible worktree path and verifiable association with the common directory. `head` follows the `HeadObservation` rules: attached HEAD requires exact reversible symbolic-ref bytes and a commit `ObjectOID`. Lock reasons are already redacted and must not expose tokens, credentials or unsafe paths. `prunable` is an observation only and does not authorize pruning. The entity conveys no linked-worktree mutation capability.

### Submodule

```text
Submodule
  observation_state
  name
  path
  recorded_commit_oid
  url_redacted?
  declaration_digest
  initialization_state  declared_uninitialized | initialized_matching |
                        initialized_mismatched | missing |
                        inaccessible | ambiguous
  observed_path_identity?
  observed_head_oid?
  observed_repository_kind?
```

The declaration in the superproject is distinct from filesystem observation. A valid declaration requires a name, reversible path, gitlink `ObjectOID` and declaration digest. `initialized_matching` requires a verified physical identity and observed HEAD equal to the recorded gitlink. `initialized_mismatched` requires both OIDs and inequality. `declared_uninitialized` prohibits an observed path identity or HEAD. URL data, when present, uses `RedactedRemoteURL` and never retains its original value.

Submodule inspection is non-recursive and performs no network access or mutation. Missing, inaccessible and ambiguous states remain distinct.

### Typed repository observation (SCHEMA-103B)

These shapes replace free-text members. They are decided from F008-FR-022, F008-FR-025 (staged, unstaged, untracked, ignored, conflicted and mode/type enumeration), F008-FR-112 (index, ref and HEAD preconditions of the initial commit), F008-FR-124/125 (authorized-path status and preservation of unauthorized paths), F008-FR-140 (each operation's type, position, preconditions, affected paths, refs and configuration keys, postconditions, failure outcomes, partial effects and recovery instruction), F008-FR-162 (HEAD), F008-FR-172 (snapshot contents) and the operation matrix in `plan.md`. Values marked **proposed** in the traceability table of `contracts/inspection-plan.md` §13 have no literal normative source; their acceptance status is recorded there.

```text
SequencerState          none | merge | rebase | cherry_pick | revert |
                        bisect | am | unknown

HeadObservation
  state                 unborn | attached | detached | invalid
  symbolic_ref?         ObservedRef
  commit_oid?           ObjectOID
  history_present

ObservedRef             same shape as ObservedPath; bytes start with refs/

RefObservation
  name                  ObservedRef
  target_oid            ObjectOID
  peeled_oid?           ObjectOID

IndexObservation
  presence              absent | present | unreadable
  version?              2 | 3 | 4
  entry_count_decimal?
  content_digest?
  lock_state            absent | present | unobservable
  extensions[]          four-byte extension signatures
  changes[]             IndexChange { path, kind: staged | conflicted |
                                      type_changed | mode_changed }

WorktreeChange          { path, kind: unstaged | untracked | ignored |
                                      type_changed | mode_changed }

WorktreePathIdentity
  path                  ObservedPath
  identity              device_decimal, inode_decimal, mode, size
  content_digest?       required for a regular file, prohibited otherwise

ConfigEntry
  ordinal
  key
  value_present
  value_redacted?
  value_redaction       not_needed | redacted | opaque
  scope                 system | xdg | global | local | worktree | command
  origin?               ObservedPath; absent only for command scope
  source_digest?        absent only for command scope

ProjectConfigObservation
  state                 absent | valid | invalid | unreadable
  path                  ObservedPath
  identity?             present for valid and invalid
  content_digest?       present for valid and invalid
  project_id?           present exactly for valid

Incompatibility         { reason, subject? }
```

Rules:

- `sequencer_state` is observed in process from the Git directory without invoking Git. It is `none` for an absent repository. Under the resolution of O-103B-1, any value other than `none`, including `unknown`, makes every plan inapplicable: planning rejects with `repository_state_incompatible`, and a value first observed during apply before the first mutation is snapshot drift classified as `plan_stale` under c-01. Inspection still reports the value. The model records it; the rejection belongs to the planner and classifier (T055/T056).
- HEAD: `attached` requires `symbolic_ref` and `commit_oid` and implies `history_present`; `detached` requires `commit_oid`, prohibits `symbolic_ref` and implies `history_present`; `unborn` prohibits `commit_oid` and may carry the symbolic ref of the unborn branch; `invalid` prohibits `commit_oid`.
- Refs are the bounded `show-ref -d` snapshot excluding HEAD (HEAD is `head`), unique and ordered by raw name bytes.
- Index and worktree change lists are ordered by raw path bytes, then kind, without duplicates. An absent or unreadable index carries no version, count, digest, extensions or changes. The index is empty exactly when it is absent or present with count `0`; an empty index has no changes.
- `worktree_path_identities` cover every authorized file, the project-config target and every `worktree_changes` path, so preservation of unauthorized paths (F008-FR-125) is verifiable. They are ordered by raw path bytes and unique.
- `effective_configuration` lists every safely parsed entry in Git resolution order with sensitive values redacted by the same rules as remote URLs. Ordinals are consecutive from zero. A key is `section[.subsection].name` with lowercase section and name.
- `config_lock_state` reports `$GIT_DIR/config.lock` and is `absent` for an absent repository; it uses the same closed catalogue as `index.lock_state`.
- A `PreconditionSnapshot` requires every observation to be reliable: no group in `not_observed` or `error`, no `unreadable` index or project config, no `unobservable` index or config lock, no `unobservable` Git Flow evidence and no `unknown` conversion risk caused by failed observation.

### DerivedOperation conditions (SCHEMA-103B)

```text
Condition
  kind
  subject?
  expected?
```

`kind` is closed. Each kind fixes whether `subject` and `expected` are present and their formats:

| Kind | Subject | Expected |
|---|---|---|
| `repository_absent` | — | — |
| `repository_kind` | — | repository kind |
| `head_state` | — | HEAD state |
| `head_symbolic_ref` | ref | — |
| `ref_absent` | ref | — |
| `ref_target` | ref | object OID |
| `reflog_absent` | ref | — |
| `object_type` | object OID | `blob` \| `tree` \| `commit` |
| `path_absent` | path | — |
| `path_identity` | path | `device:inode` decimals |
| `path_content` | path | content digest |
| `path_mode` | path | octal mode |
| `index_absent` | — | — |
| `index_empty` | — | — |
| `index_tree` | — | object OID |
| `index_lock_absent` | — | — |
| `config_absent` | config key | — |
| `config_value` | config key | redacted value |
| `path_status_clean` | path | — |

Paths and refs are encoded as unpadded base64url of their exact bytes. Preconditions, expected effects, expected postconditions and verified effects all use `Condition`.

`DerivedOperation` additionally uses: `affected_refs[]` as `ObservedRef`; `failure_outcomes[]` as closed-catalogue reasons; `partial_effect_classification` as `none | atomic | partial_possible`; and `recovery_instruction` as `not_applicable | supervised_reconciliation`. A `verification` operation has no expected effects and classification `none`; every other kind has at least one expected effect and one failure outcome. `recovery_instruction` is `not_applicable` exactly when the classification is `none`.

`GitInitializationPlan` uses: `prohibited_operations[]` as the complete closed catalogue of §9, ordered and unique; `partial_effect_points[]` as the `operation_id`s of plan operations whose classification is not `none`; and `recovery_policy` as the single value `supervised_reconciliation`.

### Defensive-copy contract

All model constructors copy caller-owned slices, maps, byte buffers, pointers to mutable values and nested structures containing them. Accessors return copies rather than aliases. Composition of an inspection into another model must copy or preserve immutable ownership so later caller mutation cannot change a previously validated value. `Validate` checks the entity's current complete state and does not rely on a constructor-only sentinel or fabricated observation. This contract is limited to model integrity under T008 and does not require unrelated architectural refactoring.

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
  observed_identity      device_decimal, inode_decimal, mode, size of the regular file read
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
  repository_observation        RepositoryObservation frozen from GitInspection (§4)
  environment_profile
    version
    state_root                  ObservedPath
    home                        ObservedPath = state_root + /environment/nonexistent-home-v1
    xdg_config_home             ObservedPath = state_root + /environment/nonexistent-xdg-v1
  development_base?
    base_oid                    ObjectOID
    object_format
    production_ref              ObservedRef
    production_tip_oid          ObjectOID, equal to base_oid
  tool_identity
```

The F008-FR-172 snapshot members map onto this shape as follows (SCHEMA-103B-08):

| F008-FR-172 member | Snapshot location |
|---|---|
| project/repository identity | `project_identity`; `repository_observation.repository_kind`, `git_dir`, `common_dir`, `object_format`, `ref_format`, `sequencer_state` |
| HEAD/history | `repository_observation.head` |
| refs | `repository_observation.refs[]` |
| relevant effective configuration with origin and scope | `repository_observation.effective_configuration[]` |
| index state | `repository_observation.index` |
| worktree identities/digests | `repository_observation.worktree_changes[]` and `worktree_path_identities[]` |
| project configuration identity | `repository_observation.project_config` |
| Git Flow state | `repository_observation.git_flow_evidence` |
| attribute/conversion sources | `repository_observation.attribute_sources[]`, `ignore_sources[]`, `ignore_evaluations[]`, `conversion_settings` |
| worktree/submodule status | `repository_observation.linked_worktrees[]`, `submodules[]` |
| filesystem/lock state | `repository_observation.filesystem_capabilities[]` (one profile per role); `index.lock_state` for `index.lock`; `config_lock_state` (`absent \| present \| unobservable`) for `$GIT_DIR/config.lock` |
| toolchain selectors | `tool_identity`, `environment_profile` |

The persistent apply lock is not a snapshot member (SCHEMA-103B-16): it is attempt-time exclusion state governed by `contracts/authorization-audit.md` §5, and freezing it into a covered plan would make every plan stale while any apply holds the lock. Remote observations are not snapshot members: F008-FR-172 does not list them, and they never influence a planned operation. A snapshot accepts only a reliable observation (see Typed repository observation). `environment_profile` freezes the exact bytes required by plan L-2. `development_base` is present exactly when the immutable intent requests F008-FR-046 creation and carries the same values.

The snapshot is complete and digest-covered. It distinguishes:

- incompatible state present during planning (`repository_state_incompatible`);
- drift against the authorized snapshot before mutation (`plan_stale`);
- change after successful immediate revalidation (`concurrent_change`).

`filesystem_capabilities` contains at most one profile per role — `project` (project and Git directory), `state_root` (state/audit root), `plan_output_parent` and `prepared_index`:

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
  stdin_digest?                  ContentDigest of the exact stdin bytes (O-T011-1)
  target_identity
  preconditions[]                Condition
  affected_paths[]               ObservedPath
  affected_refs[]                ObservedRef
  affected_config_keys[]
  expected_effects[]             Condition
  expected_postconditions[]      Condition
  failure_outcomes[]             Reason
  partial_effect_classification  none | atomic | partial_possible
  recovery_instruction           not_applicable | supervised_reconciliation
  temporary_resource_roles[]
```

Condition kinds and the rules linking these members are defined in [DerivedOperation conditions](#derivedoperation-conditions-schema-103b).

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

Canonical encoding rules (T011). Each rule is **derived** when a cited source fixes it and **proposed** when it only fixes an encoding the sources leave open:

| ID | Rule | Basis |
|---|---|---|
| CANON-01 | Member names are the snake_case names of this data model. An absent optional member (`?`) is omitted; a collection is always present, as `[]` when empty; a map member is a JSON object. | proposed |
| CANON-02 | An integer up to 9007199254740991 is a JSON number; above that it is a decimal string. Device, inode, size-decimal and entry-count members are always decimal strings. | derived, F008-FR-171 |
| CANON-03 | `ObservedPath` and `ObservedRef` both use `reversible_path_bytes_base64url` and optional `display_path_utf8`. Exact byte members (`commit_message_bytes_base64url`) are unpadded base64url. | proposed |
| CANON-04 | A frozen timestamp is RFC 3339 with whole seconds and the frozen UTC offset; sub-second precision cannot be encoded, because a Git date cannot carry it. | proposed |
| CANON-05 | A `FileIdentity` encodes as `device_decimal`, `inode_decimal`, `file_type` (`regular \| directory \| symlink \| other`), `permissions_octal` (four octal digits) and `size`. | proposed |
| CANON-06 | `operation_id` is `<ordinal>:sha256:<hex>` over the canonical object of `kind`, `exact_argv`, `target_identity`, `preconditions`, `expected_effects` and `partial_effect_classification`. An operation with argv also binds `environment_profile`, the exact `environment` path bytes of the snapshot and, when Git reads stdin, `stdin_digest`. | derived, F008-FR-173, plan L-2 and O-T011-1 |
| CANON-07 | `plan_digest` is `sha256:<hex>` over the canonical object of exactly the eight covered members. A plan whose stored operation IDs or digest differ from their recomputation is invalid. | derived, F008-FR-172 |
| CANON-08 | A decoder accepts only input that is already the exact canonical serialization of its value, within the F008-FR-051 limits, and rejects duplicate members, fractions, exponents, unsafe integers, invalid UTF-8 and lone surrogates. Unknown or missing members are rejected against a closed member set; schema-level plan decoding belongs to T055. | derived, research §3 and F008-FR-050–051 |

Resolved item O-T011-1 (2026-11-17, owner instruction to apply the agent's recommendation): `DerivedOperation` carries an optional `stdin_digest`, the `sha256:` digest of the exact bytes the operation feeds to Git on stdin (blob content, tree encoding, commit message). It is present only on an operation with `exact_argv`, and it is part of the `operation_id` content, so the authorized operation binds the exact input bytes as `plan.md` requires. T018 and T086 populate it.

`prohibited_operations` contains at least push/fetch/pull/merge/rebase/tag/remote mutation/branch delete-or-rename/reset/checkout/switch/stash/gc/amend/restore/submodule or linked-worktree mutation/hooks/`git flow feature finish`. Under SCHEMA-103B-11 these form the closed catalogue `push | fetch | pull | merge | rebase | tag_mutation | remote_mutation | branch_delete | branch_rename | reset | checkout | switch | stash | gc | amend | automatic_restore | submodule_mutation | linked_worktree_mutation | git_flow_hooks | git_flow_feature_finish`, and every plan lists all of them exactly once in that order.

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
  authorized_refs[]                ObservedRef (SCHEMA-103B-14)
  authorized_config_keys[]
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
  verified_effects[]              Condition
  commit_oid?
  index_tree_oid?
  development_branch_created?
  audit_status
  warnings[]
  findings[]                      SecurityFinding
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

The enforced maxima are: plan JSON 8 MiB, nesting 32, 16,384 members/items, 1 MiB per JSON string, 4,096 authorized files, 8,192 operations, 16,384 postconditions in total across the plan-level list and every operation's expected postconditions, 4,096 path bytes plus platform `NAME_MAX`, 64 MiB per authorized file, 512 MiB aggregate authorized bytes, 1 MiB commit message, 1 MiB per configuration/attribute/ignore source, 16 MiB aggregate such sources, and 4 MiB per captured stdout/stderr stream. Exact-boundary values are accepted. Structural/JSON/count/path excess yields `plan_invalid`; configuration/attribute/ignore source excess yields `attributes_incompatible`; authorized-file or aggregate-byte excess yields `repository_state_incompatible`; commit-message excess yields `commit_message_invalid`. Captured-output excess uses the closed reason of the source or operation being evaluated. These checks occur before authorization/mutation whenever the input is available; an overflow discovered after mutation retains its causal reason and uses effect-based status.

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
