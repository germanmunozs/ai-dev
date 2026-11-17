# Inspection and Plan Contract

## 1. Inspector port

Conceptual Go boundary:

```text
Inspector.Inspect(context, InspectRequest) -> GitInspection | error
```

`InspectRequest` contains only project path, output mode, and explicitly controlled tool/environment inputs. Inspection must not receive a mutation store or authorization.

Postconditions:

- Project/repository/index/ref/config/audit bytes are unchanged.
- No optional lock, hook, filter, fsmonitor, helper, pager, editor, prompt, network command, or configured executable runs.
- Physical project identity, tool identity, repository kind, HEAD/history, refs, index/worktree, configuration origins, attributes/conversions, Git Flow evidence, and filesystem capabilities are explicit.
- Secrets and remote userinfo are redacted before returning.

## 2. Planner port

```text
Planner.Plan(context, ImmutableIntent, GitInspection, EnvironmentProfile) -> GitInitializationPlan | GitPlanRejectionOutput
Planner.Regenerate(context, ImmutableIntent, GitInspection, EnvironmentProfile) -> GitInitializationPlan | GitApplyResult
```

`EnvironmentProfile` is resolved once by the host from the real user context (plan L-2) and frozen into the snapshot; the planner never reads HOME or XDG itself. The development base is carried by `ImmutableIntent` and copied into the snapshot. A planning refusal returns the redacted `GitPlanRejectionOutput` (data-model §15), not an apply result.

The planner is deterministic. It cannot read human identity/timestamps/message/strategy not present in `ImmutableIntent`. `Regenerate` produces the same covered canonical bytes when preconditions are unchanged.

During apply, the caller first compares fresh inspection with the authorized snapshot. An incompatibility caused by that drift is translated to `plan_stale` before `Regenerate`; the planner does not reclassify it as an initial `repository_state_incompatible`. Only a condition already present while generating/initially validating a plan uses the latter.

## 3. Plan schema and canonical form

- Schema: `ai-dev.git-init-plan/v1`.
- Encoding: UTF-8 JSON, canonicalized under RFC 8785.
- Covered fields: schema version, complete immutable intent, complete precondition snapshot, derived operations, expected postconditions, prohibited operations, partial-effect points, recovery policy.
- Excluded exactly: plan ID, generated/expiry times, inspection ID/time, audit timestamps.
- Excluded fields cannot affect authority or execution.
- Floats, duplicate/unknown members, ambiguous integers, and noncanonical values are invalid.
- Device/inode and integers outside IEEE-754 safe range are decimal strings.
- Regeneration compares canonical bytes, not decoded semantic equivalence.
- Limits are checked before allocation. Exact boundaries pass. Plan JSON/structure/member/string/path/count excess is `plan_invalid`; configuration/attribute/ignore source excess is `attributes_incompatible`; authorized-file/aggregate-byte excess is `repository_state_incompatible`; commit-message excess is `commit_message_invalid`. Captured-output excess uses the closed reason for the source or operation being evaluated. Checks occur before authorization/mutation whenever the input is available, and later discovery retains the causal reason with effect-based status.

## 4. Derived operation contract

Every mutation and mutation-supporting write is explicit. Each operation records:

- deterministic ID and order;
- operation kind and exact argv when Git is involved;
- target identities;
- paths/refs/config keys;
- complete preconditions;
- expected effects and postconditions;
- failure and partial-effect classifications;
- recovery instruction;
- logical temporary-resource roles.

No runtime apply ID, PID, token, audit time, or concrete temporary path enters the canonical operation. File-provided operations are never dispatched directly.

## 5. Safe output contract

Before exporting a plan:

1. resolve physical project and all prohibited Git/worktree/submodule areas;
2. resolve destination parent without unsafe symlinks/aliases;
3. prove destination is outside all prohibited areas;
4. reject an existing destination;
5. exclusive-create with mode `0600`;
6. write canonical complete bytes;
7. fsync file and, when supported, parent directory.

Unsafe destination returns `unsafe_output_path`; existing destination returns `output_exists`; both exit 2 and create no destination/temp file.

The destination parent must have a supported production filesystem profile. tmpfs and every non-APFS/non-ext4 profile reject; an allowed destination completes exclusive-create/file-sync/parent-sync capability checks as part of the explicit external write.

## 6. Reason precedence c-01

The classifier uses the phase where the condition is first established:

| Phase | Reason | Result before mutation |
|---|---|---|
| Plan generation or initial plan validation finds repository state intrinsically incompatible | `repository_state_incompatible` | `refused`, exit 1, zero blob/mutation |
| Apply fresh inspection/regeneration differs from authorized snapshot before first mutation | `plan_stale` | `refused`, exit 1, zero blob/mutation |
| State changes after successful immediate operation revalidation | `concurrent_change` | Before mutation: `refused`, exit 1. After mutation: retain reason and classify all effects. |

Final state similarity does not override phase precedence.

## 7. Strategy planning

- `basic`: explicit initial branch.
- `github-flow`: explicit primary/initial branch.
- `trunk-based`: explicit trunk/initial branch; no permanent auxiliary branches.
- `git-flow`: explicit production/development, initial=production for a new repository, explicit AVH or git-flow-next dialect.

Existing-history creation of a missing development ref additionally requires explicit `--development-base-oid ALGORITHM:HEX`. It must match repository object format, an accessible commit and the frozen production tip. The target-absent CAS is a distinct derived operation and its deterministic ID must be authorization-scoped; omission never causes OID inference.

For selected Git Flow with existing history and an absent development ref, omission is `strategy_conflict`/exit 1. Malformed syntax, an incompatible algorithm or forbidden context is `invalid_strategy`/exit 2. A nonexistent/inaccessible commit or mismatch with frozen production is `strategy_conflict`/exit 1. Only the exact valid production tip derives the operation; later drift follows c-01.

Detected Git Flow is informational unless Git Flow is selected/adopted. Non-Git-Flow selection never modifies `gitflow.*`. Git Flow operations use the exact profiles in `spec.md`, preserve instance `.base`, and expose every multi-key partial point.

Complete existing profiles are adopted only when every requested value is equal; those keys become verified no-ops. Empty profiles may be initialized. Partial/mixed/unrecognized or complete-but-different selected profiles are `strategy_conflict` and are not normalized.

## 8. In-process configuration, attribute, and ignore evaluation

Before Git status/diff or any initial-commit mutation, the inspector parses bounded configuration/attribute sources in process. Precedence is system, explicit/default global, root-to-leaf `.gitattributes`, then existing-repository `info/attributes`. New repositories have no local config/info source; expected post-init values are supplied from immutable intent/profile.

The parser supports approved Git matching, escapes, macros and states and returns ordered source digests, matched rules, effective values and a byte-identity proof per candidate. It invokes no Git/filter/hook/helper. Unknown grammar, include cycle/depth above 16, source above 1 MiB, aggregate sources above 16 MiB, unreadable/ambiguous input, active configured code or unproved conversion returns `attributes_incompatible`. Differential Git fixtures are test oracles only for compatible inert cases and never execute configured code.

The same bounded reader resolves effective system, global, XDG and local Git configuration without invoking Git configuration helpers or include commands. It evaluates ignore precedence in process across `core.excludesFile`, `.git/info/exclude`, and root-to-leaf `.gitignore` files, including negation, directory-only and anchored rules, recursive matching and escapes. It freezes every applicable source identity and digest plus the winning rule per candidate in the snapshot. An unreadable source, unsupported include/conditional syntax, ambiguous byte representation or mismatch with the supported inert Git semantics rejects before mutation. Ambient system/global/XDG `core.sharedRepository` is recorded but is not replayed into the private subprocess environment; only a repository-local/worktree or explicitly replayed value capable of changing planned permissions/effects is `configuration_conflict`.

Read-only Git subprocesses run only after that proof. `<state-root>` already includes the single final `git` segment and resolves to `$HOME/Library/Application Support/ai-dev/state/git` on macOS or absolute `$XDG_STATE_HOME/ai-dev/git` (fallback `$HOME/.local/state/ai-dev/git`) on Linux. The host resolves it from the real user context before building the subprocess environment; relative XDG state is invalid and real HOME/XDG are not exposed to Git. The frozen profile sets HOME to `<state-root>/environment/nonexistent-home-v1` and XDG_CONFIG_HOME to `<state-root>/environment/nonexistent-xdg-v1`, sets `GIT_CONFIG_NOSYSTEM=1` and null global/system config paths, and uses sorted exact `-c` replay of every supported semantic value required for ordinary status/ignore equivalence, including a descriptor-verified `core.excludesFile` path. The resolved root, exact reversible path bytes and profile version are snapshot/canonical-plan/digest/operation-ID inputs, and execution must match them byte-for-byte. Their private common parent is validated, the leaves are verified absent before/after every command, and the frozen toolchain must prove allowed commands do not create them; they are never per-attempt resources. Real HOME/XDG/system paths are inputs only to the in-process reader and remain frozen in the snapshot. Real-user `safe.directory` entries are not replayed or treated as proof of ownership: the inspector performs its own physical ownership check, and a controlled Git refusal remains `dubious_ownership`. If a value cannot be replayed while neutralizing executable configuration, the command is not run. Git status/diff output is compared with the in-process classification and any difference is `attributes_incompatible`. Global-only ignored files therefore retain ignored classification and require the ignored-file approval form.

## 9. Prohibited operation minimum

The plan always lists push, fetch, pull, merge, rebase, tag mutation, remote mutation, branch delete/rename, reset, checkout/switch, stash, gc, amend, automatic restore, submodule/linked-worktree mutation, Git Flow hooks, and `git flow feature finish` as prohibited.

## 10. Structured GitInspection evidence

`GitInspection` contains nine explicitly typed evidence groups. Successful empty observation, absence, non-applicability, lack of observation and observation failure are distinct states; an empty collection never substitutes for an error. Each collection-valued group has a companion `ObservationState`; singular `GitFlowEvidence` and `ConversionSettings` carry that state internally.

| Group | Contract | Inspector obligation |
|---|---|---|
| `remotes_redacted[]` | `RemoteRedacted` | Preserve names, fetch/push order, multiplicity and configuration provenance without retaining original URLs or sensitive material. |
| `git_flow_evidence` | `GitFlowEvidence` | Preserve observed profile evidence separately from strategy selection and planning decisions. |
| `attribute_sources[]` | `AttributeSource` | Preserve typed origin, reversible path, identity, digest, read state and attribute-specific precedence. |
| `ignore_sources[]` | `IgnoreSource` | Preserve typed origin, base directory, root-to-leaf precedence, identity, digest and read state. |
| `ignore_evaluations[]` | `IgnoreEvaluation` | Link each candidate evaluation to ordered source digests and, when ignored, its winning source. |
| `conversion_settings` | `ConversionSettings` | Distinguish configured/effective settings, provenance, external-code requirements, conversion risk and byte-identity proof. |
| `linked_worktrees[]` | `LinkedWorktree` | Preserve reversible paths, common-directory association, HEAD/lock/availability observations without granting mutation. |
| `submodules[]` | `Submodule` | Keep superproject declaration distinct from non-recursive filesystem observation. |
| `filesystem_capabilities[]` | `FilesystemCapabilityProfile` | Preserve one profile for each applicable location required by the plan/snapshot contract. |

The inspector does not fabricate values to make a structure complete. A source or location that cannot be observed uses the corresponding closed observation/read state and records the warning or incompatibility that prevents safe planning. Actual capture and evaluation remain assigned to T041–T045; exact redacted text/JSON rendering remains assigned to T046.

### 10.1 Redacted remotes

The inspector must redact before constructing `RemoteRedacted`. The model never receives or retains an original URL, credentials, userinfo, tokens or unredacted sensitive fragments. If safe redaction cannot be established, only a fixed opaque marker is stored. A digest of the original URL is optional, is not anonymization, is never required to hide a secret, and is omitted when it can reveal or correlate sensitive information. Fetch and push URLs remain separate ordered lists.

The exact display syntax is deferred to T046. That deferral does not weaken the current prohibition on secret or original-URL retention in model, report, error, log or serialized inspection data.

### 10.2 Git Flow evidence

The closed evidence classifications are `absent`, `complete_avh`, `complete_git_flow_next`, `partial`, `mixed`, `contradictory`, `unrecognized` and `unobservable`. Complete states require equality with the normative AVH or git-flow-next profile. Partial, mixed, contradictory and unrecognized states preserve their observed entries, source digests and issues rather than normalizing them.

Evidence does not choose a strategy, authorize a mutation or execute Git Flow. Partial or conflicting evidence is informational for Basic Git, GitHub Flow and Trunk-Based Development; it becomes a planning conflict only when Git Flow is explicitly selected or adopted under the strategy contract.

### 10.3 Attribute and ignore sources

`AttributeSource` and `IgnoreSource` are separate nominal types backed by a common source-identity component. Their catalogues and precedence rules are not interchangeable.

Attribute kinds cover system attributes, explicit/default global attributes, hierarchical repository `.gitattributes`, existing-repository `info/attributes` and known post-init intent where required by planning. Ignore kinds cover `core.excludesFile`, `info/exclude`, root-to-leaf `.gitignore` and known post-init intent where required. The common read-state catalogue is `absent`, `read`, `unreadable`, `ambiguous`, `unsupported`.

A read source has a descriptor-verified identity and content digest. An absent source has neither fabricated bytes nor digest. Unreadable, ambiguous and unsupported states fail closed. Ignore sources additionally preserve base directory and directory depth; ignore negation and directory-relative precedence must never be applied to attribute rules.

### 10.4 Conversion settings

`ConversionSettings` uses typed extensible entries and the closed risk catalogue `none`, `neutral`, `may_transform`, `external_code`, `unknown`, `incompatible`. It covers every currently specified conversion key and driver while allowing another relevant configured conversion mechanism to be represented rather than silently ignored.

Configured value, effective value, scope and source digest remain distinct. Byte identity cannot be declared proven when external code is required or risk is `may_transform`, `external_code`, `unknown` or `incompatible`. Incompatibility uses `attributes_incompatible`. No filter, command or configured executable runs to establish the model.

### 10.5 Object identity, worktrees and submodules

`ObjectOID` is the nominal SHA-1/SHA-256 object identifier for commits, trees, gitlinks and observed HEAD values. It is not interchangeable with `BlobOID`; validation checks syntax and frozen object format but does not resolve an object.

Linked-worktree availability is closed as `available`, `missing`, `locked`, `inaccessible`, `ambiguous`. Worktree paths and attached symbolic refs preserve exact reversible bytes. Lock and prunable fields are observations only and never mutation authority.

Submodule initialization is closed as `declared_uninitialized`, `initialized_matching`, `initialized_mismatched`, `missing`, `inaccessible`, `ambiguous`. The superproject declaration and gitlink are distinct from physical checkout identity and observed HEAD. Inspection is non-recursive and performs neither network access nor submodule mutation. Any submodule URL uses the same already-redacted representation as remote URLs.

### 10.6 Observation and defensive-copy semantics

`ObservationState` is closed as `not_applicable`, `not_observed`, `observed_empty`, `observed`, `absent`, `error`. These values have the exact semantics defined in `data-model.md` and must agree with group cardinality, warnings and incompatibilities.

Constructors copy all caller-owned slices, maps, bytes, mutable pointers and nested mutable collections. Accessors return defensive copies. Composing an inspection into a snapshot or plan cannot create aliases through which later mutation changes validated evidence. Validation applies to every instance regardless of constructor and cannot rely on sentinel strings or fabricated defaults.

## 11. Deferred schema decisions

- **SCHEMA-103A-08 — T046:** exact rendering syntax remains deferred. The prohibition on exposing original URLs, credentials, tokens, userinfo and other secrets is already normative.
- **SCHEMA-103A-09 — T012/T013:** additional collection/cardinality/depth/byte limits remain assigned to the bounded-input tasks. Existing limits in `spec.md` and `plan.md` remain fully applicable; no arbitrary new limit is introduced here.

These deferrals do not permit omission of a T008 field or replacement of explicit observation state with an ambiguous empty value.

## 12. Schema decision traceability

| ID | Contract | Prior normative source | Incorporated decision/invariant | Related task |
|---|---|---|---|---|
| SCHEMA-103A-01 | `RemoteRedacted` | F008-FR-027; `data-model.md` GitInspection | Original URLs and secrets never enter the model; unsafe redaction becomes an opaque marker; original digest is optional and restricted. | T008, T046 |
| SCHEMA-103A-02 | `GitFlowEvidence` | F008-FR-038–041, F008-FR-146/149; normative Git Flow profiles | Detailed closed classification; observed evidence remains separate from strategy selection and authorization. | T008, T045 |
| SCHEMA-103A-03 | `AttributeSource`, `IgnoreSource` | F008-FR-026, F008-FR-028–034, F008-FR-175 | Separate nominal types with shared identity/provenance and domain-specific precedence. | T008, T038, T042–T044 |
| SCHEMA-103A-04 | `ConversionSettings` | F008-FR-030–034, F008-FR-148/175 | Typed extensible settings plus closed conversion risk; no external code execution. | T008, T038, T043 |
| SCHEMA-103A-05 | `ObjectOID` | Repository object-format and GitInspection object observations | Nominal non-blob object identity for commits, trees, gitlinks and HEAD. | T008 |
| SCHEMA-103A-06 | `LinkedWorktree`, `Submodule` | F008-FR-022–023, F008-FR-130/165 | Closed availability/initialization states; reversible paths; read-only, non-recursive, no network. | T008, T036–T037, T045 |
| SCHEMA-103A-07 | `ObservationState` | Complete GitInspection and fail-closed inspection requirements | Explicit absence/not-applicable/not-observed/empty/data/error semantics; no ambiguous sentinels. | T008, T045 |
| SCHEMA-103A-08 | Rendering | T046 ownership of redacted report rendering | Deferred to T046; secret-exposure prohibition is current. | T046 |
| SCHEMA-103A-09 | Limits | F008-FR-051; bounded-input plan | Deferred to T012/T013 without weakening existing limits. | T012, T013 |

## 13. Schema decision round SCHEMA-103B

This round replaces every member that T008 still held as free text. The shapes are in `data-model.md` (§4 *Typed repository observation*, §4 *DerivedOperation conditions*, §7). A decision is **derived** when the cited source fixes its content and the round only fixes its encoding; it is **proposed** when no normative text fixes the value, and then requires human approval before T008 can be closed.

| ID | Member | Source | Decision | Basis |
|---|---|---|---|---|
| SCHEMA-103B-01 | `head` | F008-FR-162 | `HeadObservation` with state, optional symbolic ref, optional commit OID and `history_present`; consistency rules per state. | derived |
| SCHEMA-103B-02 | `repository_state` | data-model §4 lists the member with no definition | Renamed `sequencer_state`: closed `none \| merge \| rebase \| cherry_pick \| revert \| bisect \| am \| unknown`, observed in process. Any non-`none` value rejects every plan with `repository_state_incompatible` (O-103B-1 resolution below). | proposed |
| SCHEMA-103B-03 | `refs[]` | plan operation matrix row *Refs* (`show-ref --head -d`) | `RefObservation` with `ObservedRef` name, target and optional peeled `ObjectOID`; HEAD excluded; unique and ordered by raw bytes. | derived |
| SCHEMA-103B-04 | `index` | data-model §4 enum note; F008-FR-025; F008-FR-112 | `IndexObservation` with presence, version, entry count, digest, lock state, extension signatures and ordered changes `staged \| conflicted \| type_changed \| mode_changed`. | derived |
| SCHEMA-103B-05 | `worktree` | data-model §4 enum note; F008-FR-025; F008-FR-125; F008-FR-172 | `worktree_changes[]` (`unstaged \| untracked \| ignored \| type_changed \| mode_changed`) plus `worktree_path_identities[]` for authorized files, the project-config target and every changed path. | derived; the coverage set is proposed |
| SCHEMA-103B-06 | `effective_configuration[]` | F008-FR-172 ("with origin and scope"); plan *Local config read* row | `ConfigEntry` in resolution order with scope `system \| xdg \| global \| local \| worktree \| command`, origin, source digest and redacted value; the inspection keeps every safely parsed entry rather than a "relevant" subset. | derived; full-list choice is proposed |
| SCHEMA-103B-07 | project configuration identity | F008-FR-172; data-model §11 no-op/conflict rules | `ProjectConfigObservation` with `absent \| valid \| invalid \| unreadable`, identity, digest and preserved `project_id`. | derived |
| SCHEMA-103B-08 | snapshot composition | F008-FR-172; plan L-2; F008-FR-046 | Snapshot = frozen `RepositoryObservation` + `environment_profile` + optional `development_base` + tool identity; remotes excluded; only reliable observations accepted. | derived |
| SCHEMA-103B-09 | preconditions, effects, postconditions, verified effects | F008-FR-140; F008-FR-171; plan operation matrix | Typed `Condition` with the closed 19-kind catalogue of data-model §4. | catalogue proposed |
| SCHEMA-103B-10 | `failure_outcomes`, `partial_effect_classification`, `recovery_instruction` | F008-FR-140; spec *no automatic rollback/retry/repair* | Reasons from the closed catalogue; `none \| atomic \| partial_possible`; `not_applicable \| supervised_reconciliation`. | classification values proposed |
| SCHEMA-103B-11 | `prohibited_operations`, `partial_effect_points`, `recovery_policy` | §9 above; F008-FR-171 | Closed 20-value catalogue listed completely in fixed order; partial-effect points are the IDs of operations not classified `none`; recovery policy `supervised_reconciliation`. | catalogue spelling and point encoding proposed |
| SCHEMA-103B-12 | `incompatibilities[]` | F008-FR-011–027 fail-closed reporting | `Incompatibility { reason, subject? }` with a closed-catalogue reason. | derived |
| SCHEMA-103B-13 | `AuthorizedFile.observed_identity/metadata` | data-model §6 | One `observed_identity` holding device, inode, mode and size. | derived |
| SCHEMA-103B-14 | `authorized_refs_and_config` | spec *GitAuthorization* ("exact operation/file/config/ref scope") | Split into `authorized_refs[]` (`ObservedRef`) and `authorized_config_keys[]`; both may be empty when no ref or Git configuration is in scope. | derived |
| SCHEMA-103B-15 | filesystem profile roles | data-model §7 location list | `project \| state_root \| plan_output_parent \| prepared_index`, at most one profile per role. | spelling proposed |
| SCHEMA-103B-16 | "filesystem/lock state" | F008-FR-172; plan *Local config write* row (`config.lock` temporary) | Snapshot freezes filesystem profiles, `index.lock_state` and a new `config_lock_state` (`absent \| present \| unobservable`); the persistent apply lock is excluded as attempt-time state. | proposed |

### Resolved question

- **O-103B-1 (resolved 2026-11-17):** A `sequencer_state` other than `none`, including `unknown`, rejects every plan with `repository_state_incompatible`, exit 1, before authorization or mutation, whatever operations the plan contains. Interrupted merges, rebases and similar operations leave index, HEAD and refs in intermediate states that Feature 008 neither models nor may repair. A value first observed during apply before the first mutation is snapshot drift and follows c-01 as `plan_stale`. Inspect still reports the value without refusing. `spec.md` (rejection mapping and AC-029) and `data-model.md` §4 carry the rule; T050 and T056 test and implement it.

### Acceptance status

On 2026-11-17 the project owner instructed the implementing agent to apply its recommendation for analysis finding C1. That instruction accepts every decision marked **proposed** in the table above, the resolution of O-103B-1, and deviation DEV-T002 in `tasks.md`. A later instruction the same day accepted SCHEMA-103B-16 and the analysis remediations C2, F1, U1, U2, A1, F2 and D1, including the A1 change to the postcondition limit in `spec.md`. The agent recorded this text from those instructions, and it does not itself constitute approval: under Constitution IX, the auditable approval is the owner's own commit or pull-request review of these changes, which identifies the approver.

## 14. Git runner implementation decisions (T016–T019)

`internal/gitprocess` implements the operation matrix of `plan.md` (P-04) as a closed builder (`Build`, `BuildIdentify`), a runner (`Runner`) and toolchain identification (`Identify`). Each rule below is **derived** when `plan.md` or `spec.md` fixes it and **proposed** when it only fills a gap; proposed rules need human approval.

| ID | Rule | Basis |
|---|---|---|
| RUN-01 | The subprocess environment is built completely; nothing is inherited, not even `PATH`. It always contains `GIT_CONFIG_NOSYSTEM=1`, `GIT_TERMINAL_PROMPT=0`, `GCM_INTERACTIVE=never`, empty `GIT_ASKPASS`/`SSH_ASKPASS`, `GIT_PAGER=cat`, `PAGER=cat`, `GIT_EDITOR=true`, `GIT_SEQUENCE_EDITOR=true`, `GIT_LITERAL_PATHSPECS=1`, and the frozen HOME/XDG leaves; read-only rows add `GIT_OPTIONAL_LOCKS=0`. | derived, plan P-04 |
| RUN-02 | The "null global/system configuration paths" are `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_SYSTEM=/dev/null`; `GIT_NO_REPLACE_OBJECTS=1` implements "no replacement" for every row; `DEVELOPER_DIR` is the only accepted toolchain selector. | proposed |
| RUN-03 | `core.hooksPath` in the baseline vector is `<HOME leaf>/hooks`: beneath a leaf that must never exist, so it names a nonexistent directory without adding a path to freeze. | proposed |
| RUN-04 | Configuration replay is limited to `core.attributesfile`, `core.autocrlf`, `core.checkroundtripencoding`, `core.checkstat`, `core.eol`, `core.excludesfile`, `core.filemode`, `core.ignorecase`, `core.precomposeunicode`, `core.safecrlf`, `core.symlinks` and `core.trustctime`, sorted by key. The last four were added after review because a global `core.filemode` is common and would otherwise refuse ordinary plans. When `core.excludesFile` or `core.attributesFile` is unset, Git falls back to files under `$XDG_CONFIG_HOME/git/`, which the private leaf hides; the planner (T054) replays the default path explicitly when that file exists. Any other key, a duplicate, or a value with NUL or newline is refused with `attributes_incompatible`. Filter drivers are neutralized through sorted `clean`, `smudge`, `process` and `required=false` overrides; a driver name with `=`, `.`, NUL or newline is refused the same way. | proposed allowlist; ordering derived |
| RUN-05 | `Identify` claims `config-end-of-options` for every Git ≥ 2.45, because the separator is handled by Git's shared option parser, and the configuration-write row always passes `--` before key and value. Review verified this on Git 2.54; on 2.54 a leading-dash value also works without `--`. Only a toolchain without the capability refuses a leading-dash value, with `git_capability_missing`. | derived, plan matrix row *Local config write*; corrected after review |
| RUN-06 | Branch values reaching argv must be valid UTF-8 without control characters, a leading dash or `@{-`; a missing branch is `missing_required_branch` and an invalid one `invalid_strategy`. Author and committer reach `commit-tree` only through `GIT_AUTHOR_*`/`GIT_COMMITTER_*` with dates `@<unix> <±hhmm>`; an unusable identity or date is `identity_invalid`. | derived, plan P-04 and F008-FR-181 |
| RUN-07 | `Identify` requires Git 2.45.0 or later (C2) and claims exactly `config-end-of-options`, `initial-branch`, `object-format`, `path-format` and `ref-format`. Direct mode requires the invocation path to be the same file (device and inode) as `<exec-path>/git`, so a launcher can never pass as a direct executable. Launcher mode requires a different file, takes `<exec-path>/git` with symlinks resolved as the effective executable, and freezes `DEVELOPER_DIR` to the developer directory that contains the exec path; that frozen selector must resolve to the same exec path. All files are identified by device, inode and SHA-256, and only Darwin and Linux can identify them. | derived from F008-FR-155/156 (frozen invocation mode, launcher and `DEVELOPER_DIR`); corrected after review, which found the macOS shim accepted in direct mode |
| RUN-08 | Before and after every invocation the runner proves both private leaves absent; existence, or any error other than "does not exist", is `ErrEnvironmentLeafPresent`. | derived, plan L-2 |
| RUN-09 | Output beyond an invocation's limit (at most 4 MiB per stream) is discarded, the process is stopped, and the run returns `ErrCapturedOutputExceeded`. `GitRunResult` reports the bytes observed per stream so the manager applies `CheckCapturedStream` with the governing reason. A normal non-zero exit is a result, not a runner error. | derived, F008-FR-051; the two port fields extend the closed port set |
| RUN-10 | The status and diff rows carry `--ignore-submodules=all`, so Git never descends into a submodule and runs its unparsed configuration. Review demonstrated that the rows executed a submodule's configured clean filter. | owner-approved amendment of the `plan.md` matrix |

Verification: the matrix tests pin every row's exact argv and environment. The runner tests execute the real Git against a repository whose configuration routes pager, editor, credential helper, askpass, SSH, fsmonitor, hooks, external diff, filters, signing and an alias to sentinel scripts. A negative control proves those sentinels do fire when Git runs unprotected. A hostile submodule test, also with a negative control, covers RUN-10; it was confirmed to fail when the flag is removed. On 2026-11-17 the full suite ran on macOS (Git 2.54, Apple launcher) and, as cross-compiled linux/arm64 test binaries, on Fedora CoreOS 44 in the local podman VM (Git 2.55). `internal/gitprocess` passed 20 of 20 tests there with no skips, and `internal/gitmanager` 90 of 90. On Linux, direct-mode identification of `/usr/bin/git` succeeded and the hostile-configuration and submodule tests passed. Twelve tests elsewhere could not run because the VM has no Go toolchain: the binary-building tests of `cmd/ai-dev`, the `go list` platform-selection test of `gatestore`, and `TestOSExecutorLookPath`. Race detection was not run on Linux. This run is development evidence only; T116's preserved native evidence package is still required.
