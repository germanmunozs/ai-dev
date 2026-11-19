---

description: "Dependency-ordered implementation tasks for Feature 008 Git Management"
---

# Tasks: Feature 008 — Git Management

**Input**: Design documents from `specs/008-git-management/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Tests are mandatory because the specification requires automated verification for AC-001–AC-032, security boundaries, crash recovery, concurrency, and native filesystem behavior. Test tasks precede their corresponding implementation tasks.

**Organization**: Tasks are grouped by user story. Requirement and acceptance-criterion references identify the normative coverage; they do not create new requirements.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel because it targets different files and has no dependency on an incomplete task in the same phase.
- **[Story]**: Maps the task to a user story from `spec.md`.
- Every task names its intended file or directory.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish package boundaries and reusable test fixtures without changing Features 001–006.

- [x] T001 Create package skeletons and package documentation for `internal/gitmanager/`, `internal/gitprocess/`, `internal/gitstore/`, and `internal/conventionalcommit/` following the import DAG in `specs/008-git-management/plan.md`
- [x] T002 Define consumer-side `GitRunner`, `GitStore`, `Filesystem`, `Clock`, `RandomSource`, and `Reporter` ports in `internal/gitmanager/ports.go` before fakes, adapters, or composition (P-01)
  - **DEV-T002 (deviation accepted by owner instruction on 2026-11-17; the auditable approval is the owner's commit, see `contracts/inspection-plan.md` §13 *Acceptance status*):** After the pre-Phase 2 architecture review, `GitStore` and `Filesystem` expose typed intent-revealing methods instead of a generic `Execute` envelope, and the `Reporter` port was removed because rendering stays in `internal/report`. `plan.md` describes the resulting port set. The T005 fakes and the T006 composition bundle were updated to match; `cmd/ai-dev/main.go` now composes through `app.RunWithDependencies`, and the Feature 006 gate-import allowlist names `internal/app/app.go` instead of `internal/app/doctor.go`.
- [x] T003 Add structural import-boundary tests that permit only `internal/gitprocess` to import `os/exec` and prohibit `internal/gitmanager` from importing `internal/gitprocess` or `internal/gitstore` in `internal/gitmanager/boundary_test.go`
  - **DEV-T003 (approved deviation):** The `os/exec` import exception is restricted to `internal/gitmanager/testhelpers_test.go` for synthetic test infrastructure only; it does not extend to production code or other files. Residual risk: the exception depends on the exact filename.
- [x] T004 Add shared inert hostile-Git sentinels and repository fixture builders without real credentials in `internal/gitmanager/testdata/` and `internal/gitmanager/testhelpers_test.go`
  - **DEV-T004 (approved deviation):** The test-only harness includes `internal/gitmanager/sentinel_pty_posix_test.go` and `internal/gitmanager/sentinel_pty_other_test.go`, originally authorized in Step 75; its scope is exclusively testing.
- [x] T005 Add deterministic fault-injection implementations of the T002 ports in `internal/gitmanager/fakes_test.go`
- [x] T006 Add Feature 008 dependency composition interfaces while preserving existing `app.Run` and `app.RunWithGateStore` behavior in `internal/app/gitmanagement.go` and `internal/app/gitmanagement_test.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Implement shared types, validation, canonical encoding, platform paths, controlled process execution, and durable storage primitives required by every story.

**⚠️ CRITICAL**: No user-story implementation may begin until this phase is complete.

- [x] T007 Add table-driven tests for every closed enum, identifier distinction, status/reason/exit mapping, decimal-string device/inode, reversible base64url path rule, and every SCHEMA-103A/103B entity invariant (observation states, HEAD/ref/index/worktree/configuration/project-config observations, conditions, environment profile, development base, operation and plan catalogues) in `internal/gitmanager/model_test.go` (F008-FR-021, F008-FR-051–052, F008-FR-150–154, F008-FR-166, F008-FR-171, F008-FR-186, F008-FR-189; AC-009, AC-021–022, AC-024)
- [x] T008 Implement `PhysicalProjectIdentity`, `GitToolIdentity`, `GitInspection`, `ImmutableIntent`, `PreconditionSnapshot`, `DerivedOperation`, `GitInitializationPlan`, `GitAuthorization`, `GitApplyAttempt`, `GitApplyResult`, and `SecondaryFailure` with strict validation, together with the SCHEMA-103A/103B entities they are composed of (`RepositoryObservation`, `HeadObservation`, `RefObservation`, `IndexObservation`, `WorktreeChange`, `WorktreePathIdentity`, `ConfigEntry`, `ProjectConfigObservation`, `Incompatibility`, the nine structured evidence groups, `ObservedPath`/`ObservedRef`, `Condition`, `EnvironmentProfile`, `DevelopmentBase`), in `internal/gitmanager/model.go`
- [x] T009 [P] Add RFC 8785/JCS canonicalization vectors, duplicate/unknown-key rejection, unsafe-integer strings, NFC/NFD preservation, and deterministic `operation_id` tests in `internal/gitmanager/canonical_test.go` (F008-FR-050–058, F008-FR-152, F008-FR-171–173; AC-002, AC-012)
- [x] T010 [P] Create `FuzzPlanDecoder` for bounded allocation and strict schema rejection in `internal/gitmanager/plan_decode_fuzz_test.go` and `FuzzCanonicalWriter` for deterministic bytes/digests in `internal/gitmanager/canonical_fuzz_test.go` (F008-FR-050–056)
- [x] T011 Implement schema-restricted canonical JSON, plan digest calculation excluding only normative metadata and self-derived `plan_digest`, and deterministic operation IDs in `internal/gitmanager/canonical.go`
  - **DEV-T011 (deviation accepted by owner instruction on 2026-11-17; the auditable approval is the owner's commit):** The model-to-canonical mapping lives in `internal/gitmanager/canonical_model.go` beside `canonical.go`. The value-level strict decoder and the closed member-set reader are implemented here; schema-level decoding of an untrusted plan into the model is left to T055, which owns untrusted-plan handling. Encoding decisions CANON-01–08 and the resolution of O-T011-1 (an optional `stdin_digest` bound into the `operation_id`) are recorded in `data-model.md` §9.
- [x] T012 [P] Add exact-boundary and one-over tests for plan JSON, nesting, members, strings, paths, file counts, operation counts, postconditions, authorized bytes, commit messages, source bytes, include depth, and captured streams in `internal/gitmanager/limits_test.go` (F008-FR-051; AC-022)
- [x] T013 Implement streaming/bounded input accounting and the approved limit-to-reason mapping in `internal/gitmanager/limits.go`
  - **Note (T013):** `limits.go` holds the closed bound-to-reason catalogue, `CheckLimit`, streaming `Budget` accounting, `ReadBounded` and `CheckPathBytes`. The canonical decoder reports every exceeded bound as a `LimitError` mapped to `plan_invalid`, and the model enforces the bounds it can observe: path bytes, file count, per-file and aggregate authorized bytes, commit-message bytes, operation count and the plan-wide postcondition total. Source bytes, include depth and captured streams are enforced through `Budget` and `ReadBounded` by the readers that own them (T019, T042–T044); a captured-stream overflow carries the governing reason its caller supplies.
- [x] T014 [P] Add macOS/Linux state-root tests covering empty/unset/relative `XDG_STATE_HOME`, no `git/git`, fixed absent HOME/XDG leaves, and identical profile bytes for plan/digest/operation/execution in `internal/gitmanager/statepaths_test.go` (L-2; AC-019)
- [x] T015 Implement canonical state roots and fixed private Git environment paths—macOS `$HOME/Library/Application Support/ai-dev/state/git`, Linux absolute `$XDG_STATE_HOME/ai-dev/git` or `$HOME/.local/state/ai-dev/git`, with direct `environment/nonexistent-*-v1` leaves—in `internal/gitmanager/statepaths.go`; T015 computes these paths from the real user context, while `EnvironmentProfile` in `model.go` (T008) only validates the frozen bytes and their fixed leaves
- [x] T016 [P] Add exact-argv/no-shell, environment allowlist, hostile `GIT_*`, HOME/XDG absence, prompt/network/helper/hook/filter/fsmonitor/editor/pager/signing, bounded-output, and macOS launcher/`DEVELOPER_DIR` tests in `internal/gitprocess/runner_test.go` (F008-FR-012–018, F008-FR-130–132, F008-FR-155–156, F008-FR-163; AC-001, AC-015)
- [x] T017 [P] Add row-by-row tests for the approved Git operation matrix, exact argv, overrides, dynamic filter neutralization, sorted `-c` replay, `core.excludesFile`, `git config --` capability, private HOME/XDG, `GIT_CONFIG_NOSYSTEM=1`, rejection of unreproducible configuration, and frozen-environment equality in `internal/gitprocess/invocation_matrix_test.go` (F008-FR-163, F008-FR-175; AC-001, AC-015)
- [x] T018 Implement the closed operation-to-argv/environment builder exactly matching the `Exact Git operation matrix (P-04)` section of `specs/008-git-management/plan.md`, including its approved `update-ref`, `config`, `read-tree`, prepared-index, installation, verification, override, replay, argv, and environment rows without adding Git subcommands, in `internal/gitprocess/invocation.go`
- [x] T019 Implement frozen-toolchain discovery and exact `exec.CommandContext` Git runner with per-operation controlled profiles, `GIT_LITERAL_PATHSPECS=1`, no shell, no silent fallback, and bounded stdin/stdout/stderr in `internal/gitprocess/runner.go`
- [x] T020 [P] Add build-constraint and unsupported-platform tests for `(darwin && !ios) || (linux && !android)` and its complement in `internal/gitstore/boundary_test.go` (F008-FR-133–136; AC-019)
- [x] T021 Implement filesystem identity/capability ports plus APFS/ext4 production selectors, tmpfs test-only classification, and fail-closed unsupported fallback in `internal/gitstore/filesystem_posix.go`, `internal/gitstore/filesystem_other.go`, and `internal/gitmanager/filesystem.go`
- [x] T022 [P] Add atomic private-record publication, fsync ordering, permissions `0700`/`0600`, truncated-temp, and compare-and-swap fault tests in `internal/gitstore/record_test.go` (F008-FR-059–069, F008-FR-178–180; AC-018, AC-026)
- [x] T023 Add test-first coverage for the sole append-only audit writer in `internal/gitstore/audit_test.go`—record ordering, no-replace publication, durable write/fsync failures, inconsistent-record rejection, no false effect claims, and reconciliation evidence—and then implement no-replace immutable record publication plus version/owner-checked authorization replacement in `internal/gitstore/record.go` and `internal/gitstore/store.go`; the audit-writer tests MUST be written and confirmed to fail for the missing T030 behavior before T030 is implemented (F008-FR-059–069, F008-FR-178–180; AC-018, AC-026)
  - **DEV-T020–T023 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** Platform code is split into `filesystem_posix.go` plus per-OS `filesystem_darwin.go` and `filesystem_linux.go`, `record.go`/`record_posix.go` and `store.go`/`store_posix.go`; the single fallback is `unsupported_other.go` (instead of `filesystem_other.go`) with its implementations in the always-compiled `unsupported.go`. `audit_test.go` covers the event layer this task implements: naming, ordering, no-replace publication, durability faults, inconsistent-directory rejection and crash-evidence temporaries. Tests for record content, effect claims and reconciliation records are written first in T030, before its audit writer, because T030 defines those records. Decisions STO-01–07 are recorded in `contracts/authorization-audit.md` §9.

**Checkpoint**: Shared contracts, canonicalization, controlled Git execution, supported-platform selection, and durable storage are ready.

---

## Phase 3: Durable Safety Kernel (Blocks Every Git Mutation)

**Purpose**: Complete and test physical locks, authorization consumption, durable intent/CAS boundaries, crash recovery, owned-resource release, and one terminal closer before any Git mutation implementation or public mutating route.

- [x] T024 [P] Add persistent advisory-lock tests for free reuse, active holder, rename/move, aliases, recycled device/inode, diagnostics, consumed prior authority, and ambiguous identity in `internal/gitstore/lock_posix_test.go` (F008-FR-070–071, F008-FR-168, F008-FR-180; AC-017)
- [x] T025 [P] Add authorization CAS tests for `issued→claimed→mutation_started→terminal`, owner/version exclusivity, expiry, mismatch, terminal immutability, and consumed states in `internal/gitstore/authorization_test.go` (F008-FR-152–154, F008-FR-167–169, F008-FR-178–180; AC-004–006, AC-021)
- [x] T026 [P] Add crash/fault tests around attempt sync, claim, `mutation_intent`, mutation-start CAS, `operation_started`, release, prepared result, apply-lock release, and terminal CAS in `internal/gitmanager/recovery_test.go` (AC-006, AC-018, AC-021, AC-032)
- [x] T027 [P] Add release tests for sole, primary-plus-secondary, multiple failures, release-audit failure, replaced resources, deterministic order, and exit 1 in `internal/gitmanager/release_test.go` (F008-FR-169, F008-FR-184; AC-017, AC-032)
- [x] T028 Implement the path-free device/inode advisory-lock namespace, current physical proof, safe persistent-file reuse, diagnostic rebuild, and no automatic deletion in `internal/gitstore/lock_posix.go`
- [x] T029 Implement durable authorization issuance plus state/version/owner CAS transitions in `internal/gitstore/authorization.go`
- [x] T030 Implement append-only attempt, intent, operation, release, prepared-result, and terminal-CAS audit records in `internal/gitstore/audit.go`
- [x] T031 Implement consumed claimed-with-intent, `mutation_started`, terminal immutability, CAS uncertainty, and later-refusal semantics in `internal/gitmanager/recovery.go`
- [x] T032 Implement supervised reconciliation through new linked records without modifying history in `internal/gitmanager/reconcile.go`
- [x] T033 Implement planned owned-resource release and deterministic `secondary_failures[]` in `internal/gitmanager/release.go`
- [x] T034 Implement the sole terminal-closure owner—effect classification, prepared terminal result, apply-lock-last coordination, owner-only digest CAS, failed/uncertain CAS, and consumed authority—in `internal/gitmanager/terminal.go`
- [x] T035 Implement and test the safety facade that acquires the lock, claims authority, persists intent, performs mutation-start CAS, persists `operation_started`, and only then emits an internal non-forgeable `MutationPermit` bound to the authorization ID and exact operation ID in `internal/gitmanager/safety.go` and `internal/gitmanager/safety_test.go`; the production permit constructor MUST remain inaccessible to public adapters and a test-only constructor MUST exist only in `_test.go`
  - **DEV-T024–T035 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** The kernel also adds `internal/gitmanager/journal.go` (the per-attempt sequence owner and `CheckAuditEventOrder`), `records.go` (strict authorization, attempt and result codecs) and the tests `records_test.go`, `terminal_test.go` and `reconcile_test.go`. The `GitStore` port gains `RecordAttemptResult` and `ListAuditEvents`, and attempts move to `<state-root>/attempts/<apply-id>/`. Decisions STO-08–11 and KER-01–08 are recorded in `contracts/authorization-audit.md` §10. The audit record content and effect-claim tests promised by DEV-T020–T023 are in `records_test.go`, `terminal_test.go` and the store ordering test.

**Checkpoint**: Locks, durable authorization, recovery, release, and terminal closure are complete against fakes. Later mutation tasks MUST consume T034/T035 and MUST NOT reimplement them.

---

## Phase 4: User Story 1 — Inspect a project safely (Priority: P1) 🎯 MVP

**Goal**: Produce a complete read-only repository and strategy inspection under hostile configuration.

**Independent Test**: Inspect absent, normal, bare, nested, ancestor, linked-worktree, submodule, inaccessible, and ambiguous fixtures and prove zero project/index/ref/config/audit mutation.

### Tests for User Story 1

- [x] T036 [P] [US1] Add repository-kind, HEAD (`unborn|attached|detached|invalid`), history, index/worktree, ignored/conflict/mode/type, and redaction tests in `internal/gitmanager/inspect_test.go` (F008-FR-011–027, F008-FR-138–139, F008-FR-162; AC-001, AC-029)
- [x] T037 [P] [US1] Add descriptor-based root, symlink, ancestor/nested, alias, dubious ownership, non-UTF-8, NFC/NFD, case-folding, and inode-over-2^53 tests in `internal/gitmanager/path_test.go` (F008-FR-019–024, F008-FR-087–089, F008-FR-150–151, F008-FR-176, F008-FR-183, F008-FR-186; AC-012, AC-024)
- [x] T038 [P] [US1] Add safe in-process system/global/XDG/local config, includes, attributes, macros, conversions, ignores, negation, nested precedence, `core.excludesFile`, `info/attributes`, `info/exclude`, and fail-closed differential tests in `internal/gitmanager/configeval_test.go` (F008-FR-026, F008-FR-028–034, F008-FR-148, F008-FR-175; AC-014)
- [x] T039 [P] [US1] Create `FuzzAttributeParser` for matching, quoting, escapes, macros, precedence, and fail-closed unsupported grammar in `internal/gitmanager/attributes_fuzz_test.go` (F008-FR-029–034, F008-FR-175)
- [x] T040 [P] [US1] Add inspect CLI text/JSON contract and zero-side-effect subprocess tests in `cmd/ai-dev/init_inspect_test.go` (F008-FR-001, F008-FR-011; AC-001, AC-022)

### Implementation for User Story 1

- [x] T041 [US1] Implement descriptor-safe physical path resolution, physical/documentary identity, repository-kind classification, ownership checks, and reversible path encoding in `internal/gitmanager/path.go`
- [x] T042 [US1] Implement bounded in-process Git configuration reader with safe include resolution and no execution of configured commands in `internal/gitmanager/config.go`
- [x] T043 [US1] Implement Git-correct bounded attribute/conversion evaluator and byte-equivalence proof in `internal/gitmanager/attributes.go`
- [x] T044 [US1] Implement Git-correct bounded ignore evaluator for `.gitignore`, `.git/info/exclude`, and effective `core.excludesFile` in `internal/gitmanager/ignore.go`
- [x] T045 [US1] Implement read-only repository, HEAD, history, index/worktree, refs, Git Flow evidence, toolchain, and filesystem inspection in `internal/gitmanager/inspect.go`
- [x] T046 [US1] Add redacted `GitInspection` text/JSON rendering in `internal/report/gitinspection.go`
- [x] T047 [US1] Wire `ai-dev init inspect --project-dir PATH --format text|json` without audit-store mutation in `internal/app/gitinit.go` and `cmd/ai-dev/main.go`
  - **DEV-T036–T047 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** Beyond the listed files, the phase adds `wildmatch.go` (a port of Git's matcher shared by the three evaluators), `source.go`/`source_posix.go` (bounded source reads), `toolchain.go`, `gitflow.go`, `projectconfig.go`, `inspect_posix.go`, `inspect_output.go` and the path platform files in `internal/gitmanager`, plus `internal/gitprocess/reader.go` and `locate.go`, `internal/app/gitcompose.go` and the `GitReader` port. Ignore tests live in `configeval_test.go` with the configuration and attribute tests. The per-file `AttributeEvaluation` record is left to planning, which owns the allowlist. Submodule gitlinks and the state-root profile are not observed by inspection (INS-11, INS-13). Decisions INS-01–14 are recorded in `contracts/inspection-plan.md` §15.

**Checkpoint**: US1 independently satisfies AC-001 and the inspection portions of AC-012, AC-014, AC-015, AC-019, AC-022, AC-024, and AC-029.

---

## Phase 5: User Story 2 — Build a reviewable initialization plan (Priority: P1)

**Goal**: Generate one deterministic canonical plan for an explicit strategy and export it safely outside all project/Git areas.

**Independent Test**: Plan all four strategies, regenerate byte-identical covered content, classify initial incompatibility versus drift, and reject unsafe/existing output without writing.

### Tests for User Story 2

- [x] T048 [P] [US2] Add strategy/branch/prefix/ref validation tests, including no default, rejected `trunk`/`trunk_based`, literal `HEAD`, leading dashes, controls, `@{-1}`, shell metacharacters, and Git Flow reserved-name collisions in `internal/gitmanager/strategy_test.go` (F008-FR-035–049, F008-FR-146, F008-FR-170, F008-FR-181, F008-FR-187; AC-011, AC-023)
- [x] T049 [P] [US2] Add plan-time and canonicalization rejection tests for literal path byte `|` in `internal/gitmanager/plan_path_test.go` (F008-FR-087, F008-FR-177; AC-012)
- [x] T050 [P] [US2] Add plan regeneration, exact covered fields, prohibited-operation minimum, deterministic operation enumeration, c-01 precedence, rejection of every plan under an interrupted `sequencer_state` (O-103B-1), and untrusted-plan rejection tests in `internal/gitmanager/plan_test.go` (F008-FR-050–058, F008-FR-140–143, F008-FR-164–165, F008-FR-172–173; AC-002, AC-030)
- [x] T051 [P] [US2] Add output path alias/symlink/project/Git-area/existing-file, exclusive-create `0600`, fsync, partial external file, and APFS/ext4-profile tests in `internal/gitmanager/planoutput_test.go` (F008-FR-182; AC-013)
- [x] T052 [P] [US2] Add plan CLI option-matrix, `--project-dir`, rejection-envelope, exact exit 0/1/2, and structured output tests in `cmd/ai-dev/init_plan_test.go` (AC-022)

### Implementation for User Story 2

- [x] T053 [US2] Implement canonical strategy, branch, prefix, ref, object-format, and tool-capability validation in `internal/gitmanager/strategy.go`
- [x] T054 [US2] Implement immutable intent and complete precondition-snapshot construction, including frozen state-root/environment/config/attribute/ignore/toolchain identities in `internal/gitmanager/snapshot.go`
- [x] T055 [US2] Implement planner/regenerator that enumerates every mutation, precondition, effect, postcondition, partial point, recovery instruction, and prohibited operation in `internal/gitmanager/plan.go`
- [x] T056 [US2] Implement phase-aware `repository_state_incompatible` → `plan_stale` → `concurrent_change` classifier, including the O-103B-1 interrupted-sequencer rule, in `internal/gitmanager/classify.go`
- [x] T057 [US2] Implement safe external plan export with physical-parent validation, exclusive creation, restrictive mode, file/directory sync, and no overwrite in `internal/gitmanager/planoutput.go`
- [x] T058 [US2] Add `GitPlanOutput` and `GitPlanRejectionOutput` text/JSON rendering in `internal/report/gitplan.go`
- [x] T059 [US2] Wire `ai-dev init plan` and its approved option matrix without `--write-config` in `internal/app/gitinit.go` and `cmd/ai-dev/main.go`
  - **DEV-T048–T059 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** The initial-commit option group is refused with exit 2 until T083–T089 plan it with its allowlist and preflight (PLN-01). Planning of the Git Flow profiles, the project configuration rendering and the development-base CAS is done here; T093–T095 keep their apply-side execution and tests. The phase also adds `planfile.go` (plan file encoding and untrusted decoding), `plan_output_json.go`, `planoutput_posix.go`, `internal/app/gitplan.go`, and the `GitCommandBuilder` port with the `check_ref_format` read row. Decisions PLN-01–09 are recorded in `contracts/inspection-plan.md` §16.

**Checkpoint**: US2 independently satisfies AC-002, AC-013, AC-022, and planning aspects of AC-011, AC-014, AC-015, AC-024, AC-029, and AC-030.

---

## Phase 6: User Story 3 — Apply an explicitly authorized plan (Priority: P1)

**Goal**: Issue and consume a durable single-sequence authorization, audit every attempt before business return, and apply only regenerated authorized operations.

**Independent Test**: Exercise issuance, incomplete/existing IDs, expiry, mismatch, exact per-file approvals, concurrent claims, consumed states, and every durable status/reason mapping.

### Tests for User Story 3

- [x] T060 [P] [US3] Add issuance/scope/expiry/approval-set/declared-operator/existing-ID tests in `internal/gitmanager/authorization_test.go` (F008-FR-081–084, F008-FR-152–154, F008-FR-159–161, F008-FR-178; AC-003–004)
- [x] T061 [P] [US3] Extend the foundational T025 authorization tests through manager/store integration—without restating or reimplementing CAS invariants—in `internal/gitmanager/authorization_store_integration_test.go`, covering scope mismatch, consumption, expiry, and owner-visible outcomes (F008-FR-153, F008-FR-167–168, F008-FR-179; AC-005, AC-021)
- [x] T062 [P] [US3] Extend the test-first T023 writer coverage through manager/store integration—without creating another writer or restating its publication invariants—in `internal/gitmanager/audit_store_integration_test.go`, covering attempt-before-business-result, mutation intent, operation and release records, redaction, reconciliation evidence, and terminal digest binding through the T030 API (F008-FR-059–069, F008-FR-154, F008-FR-178–180; AC-018, AC-026)
- [x] T063 [P] [US3] Add apply CLI issue/consume forms, exact approvals, untrusted-plan regeneration, structured results, and exit-code tests in `cmd/ai-dev/init_apply_test.go` (F008-FR-050–058, F008-FR-159–161, F008-FR-167; AC-003–005, AC-022)

### Implementation for User Story 3

- [x] T064 [US3] Implement only authorization domain validation, exact plan/project/operation/file scope checks, and pre-issuance orchestration in `internal/gitmanager/authorization.go`; durable creation and every authorization state transition MUST be performed through the sole T029 API, and this task MUST NOT implement another CAS, durable issuer, append-only writer, or authorization state machine
- [x] T065 [US3] Integrate the manager authorization issuance/consumption flow with the existing T029 `GitStore` authorization API in `internal/gitmanager/authorization_store.go`; MUST NOT create a second CAS implementation or transition authority
- [x] T066 [US3] Integrate apply orchestration with the existing T030 append-only audit writer in `internal/gitmanager/audit_integration.go`; MUST NOT create a second audit writer, sequence owner, or publication protocol
- [x] T067 [US3] Implement the apply coordinator as a consumer of `safety.go` and the sole closer `terminal.go`; it MUST NOT implement lock, release, terminal CAS, or consumed-authority semantics in `internal/gitmanager/apply.go`
- [x] T068 [US3] Implement exact status/reason/durable-state/exit mapping and deterministic secondary-failure ordering in `internal/gitmanager/result.go`
- [x] T069 [US3] Add validated text/JSON apply rendering with one stdout object and redacted findings in `internal/report/gitapply.go`
- [x] T070 [P] [US3] Add simulated-TTY tests for four unselected strategies, explicit selection, complete plan display, digest-bound written confirmation, destination confirmation, UUID generation through injected `RandomSource` only after confirmation, cancellation, EOF, invalid input, no-TTY refusal, and zero premature authority in `internal/app/gitinit_wizard_test.go` (F008-FR-002–008, F008-FR-182; AC-004, AC-022)
- [x] T071 [US3] Implement the interactive assistant state machine with injected terminal, planner, issuer, apply engine, and `RandomSource` in `internal/app/gitinit_wizard.go`; no prompt, default, or authority semantics may differ from `contracts/cli.md`
- [x] T072 [US3] Integrate apply and assistant adapters against fake/nonmutating dispatch in `internal/app/gitinit.go`; defer real public mutation wiring until T107
  - **DEV-T060–T072 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** Public `ai-dev init apply` is refused with exit 2 before any record until an executor exists (APL-01). `cmd/ai-dev/init_apply_test.go` therefore verifies the option matrix, the refusal without state and the no-TTY assistant, while issue/consume flows run in `internal/app/gitapply_test.go` and `gitinit_wizard_test.go` with a nonmutating executor and the real store. The T061/T062 integration tests live in one external-package file, `apply_store_integration_test.go`. Added files: `result.go`, `audit_integration.go`, `authorization_store.go`, `internal/app/gitapply.go` and `terminal_*.go`. Decisions APL-01–09 are recorded in `contracts/authorization-audit.md` §11.

**Checkpoint**: US3 independently satisfies the authorization/result boundaries of AC-003–006, AC-018, AC-021–022, AC-026, and AC-032 against fake/nonmutating dispatch. Mutation-effect AC-009 and precedence AC-030 remain pending their later operation and integration phases.

---

## Phase 7: User Story 4 — Initialize a new repository deterministically (Priority: P1)

**Goal**: Initialize only an authorized new SHA-1/files-ref repository with explicit branch, empty template, frozen toolchain, and no ambient defaults.

**Independent Test**: Run the exact init operation under hostile template/default-branch/default-ref-format/shared-repository settings and verify capability failure is pre-mutation.

### Tests for User Story 4

- [x] T073 [P] [US4] Add exact argv, empty-template, explicit SHA-1/files-ref/branch, existing-repository no-reinit, missing-capability, and hostile ambient configuration tests in `internal/gitmanager/initrepo_test.go` (F008-FR-094–095, F008-FR-109, F008-FR-181, F008-FR-190; AC-007)
- [x] T074 [P] [US4] Add operation-level revalidation, declared-effects, no-reflog/no-implicit-config, postcondition inspection, and fail-closed missing/invalid/wrong-authorization-or-operation `MutationPermit` tests in `internal/gitmanager/operations_test.go`; prove production adapters cannot construct permits and use only an internal `_test.go` constructor in tests (F008-FR-140–143, F008-FR-164, F008-FR-166, F008-FR-174; AC-007, AC-009)

### Implementation for User Story 4

- [x] T075 [US4] Implement frozen capability validation and the exact argv `git init --template= --object-format=sha1 --ref-format=files --initial-branch=<name> -- <physical-path>` in `internal/gitmanager/initrepo.go`
- [x] T076 [US4] Implement typed exact-argv operation dispatch with immediate precondition revalidation and complete postcondition inspection in `internal/gitmanager/operations.go`; every production mutation entrypoint MUST require and validate the non-forgeable T035 `MutationPermit` against authorization and operation IDs, fail closed before invoking Git when absent/invalid/mismatched, and expose no public bypass
- [x] T077 [US4] Add initialization effects and toolchain details to strategy/apply reports in `internal/report/gitapply.go`
  - **DEV-T073–T077 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** Condition evaluation against fresh state lives in the added file `internal/gitmanager/conditions.go`. `OperationExecutor.Revalidate`/`Execute` now receive the regenerated plan (OPS-01). The real-Git T073 acceptance is the external-package `initrepo_integration_test.go`, because only an external test may compose `gitprocess`. It verifies effects through files rather than a Git subprocess, which the boundary test forbids in this package. T077 also renders the initialization in the plan text report (`internal/report/gitplan.go`). The dispatcher does not yet perform the `.ai-dev/` writes (OPS-07, T093). Decisions OPS-01–08 are recorded in `contracts/authorization-audit.md` §12.

**Checkpoint**: US4 independently satisfies AC-007, relevant AC-009/AC-015, and N-E/P-3 precedence.

---

## Phase 8: User Story 5 — Create an optional safe initial commit (Priority: P1)

**Goal**: Create exactly the approved files/objects/commit/tree and leave the normal index coherent, with global preflight before any mutation.

**Independent Test**: Fail on the last allowlisted secret with zero blobs, then complete the exact commit/ref/index flow and fault every CAS/index boundary.

### Tests for User Story 5

- [x] T078 [P] [US5] Add literal allowlist, mandatory config, normal/ignored approvals, safe-open metadata barriers, size limits, Unicode/case collisions, and ignored-global-file tests in `internal/gitmanager/allowlist_test.go` (F008-FR-080–090, F008-FR-159–161, F008-FR-183; AC-003, AC-008, AC-012)
- [x] T079 [P] [US5] Add high-risk synthetic-secret corpus, secret-last, finding-redaction, and scanner-limitation tests in `internal/gitmanager/secrets_test.go` (F008-FR-085, F008-FR-091–093, F008-FR-191; AC-008, AC-028)
- [x] T080 [P] [US5] Add identity/date/message-byte/signing and Conventional Commit integration tests for object creation in `internal/gitmanager/commit_test.go` (F008-FR-096–110; AC-016, AC-027)
- [x] T081 [P] [US5] Add blob/tree/commit verification, initial-ref CAS, unreachable-object reporting, index failure, unauthorized-file preservation, and exact final status tests in `internal/gitmanager/commitflow_test.go` (F008-FR-110–129, F008-FR-188–189, F008-FR-191–192; AC-009–010, AC-028–030)
- [x] T082 [P] [US5] Add autonomous index-v2 parser/checksum/entry/extension tests and all 13 `index.lock` ownership/race/replacement steps in `internal/gitmanager/index_test.go` (F008-FR-115–124, F008-FR-157–158, F008-FR-184, F008-FR-188; AC-010, AC-017, AC-025)

### Implementation for User Story 5

- [x] T083 [US5] Implement literal per-file approval validation, safe descriptor reads, metadata barriers, ignored-file reinforcement, and exact config inclusion in `internal/gitmanager/allowlist.go`
- [x] T084 [US5] Implement bounded in-process high-risk secret screening that records only path/category in `internal/gitmanager/secrets.go`
- [x] T085 [US5] Implement the two-phase global preflight over every allowlisted file and in-memory `.ai-dev/config.json`, prohibiting blobs until all files pass in `internal/gitmanager/preflight.go`
- [x] T086 [US5] Implement controlled no-filter blob hashing/writes, exact tree construction, unsigned commit creation, and object verification in `internal/gitmanager/commit.go`
- [x] T087 [US5] Implement absent-old initial-ref CAS and non-rollback partial-effect handling in `internal/gitmanager/refs.go`
- [x] T088 [US5] Implement autonomous prepared index-v2 construction/parsing and the descriptor-owned 13-step `index.lock` installation protocol in `internal/gitmanager/index.go`
- [x] T089 [US5] Implement complete HEAD/ref/commit/tree/index/status verification and preservation of unauthorized paths in `internal/gitmanager/postconditions.go`
  - **DEV-T078–T089 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** T097 (grammar corpus) and T100 (the validator) of US7 are done here, because the commit message must be a Conventional Commit; T098, T099 and T101 stay in Phase 10. Added files: `allowlist_posix.go` (descriptor reads), `index_posix.go`/`index_other.go` (the `index.lock` protocol and prepared storage), and the real-Git `commitflow_integration_test.go`. The initial-ref CAS (T087) is planned in `postconditions.go` and dispatched in `operations.go`, so no `refs.go` exists. `OperationExecutor` gains `Preflight`, and the `Inspect` ports take the commit candidates (COM-05, COM-10). Plan-level postconditions now supersede contradicted earlier ones (COM-08). The store creates `prepared`. Decisions COM-01–14 are recorded in `contracts/apply-strategy.md` §10.

**Checkpoint**: US5 independently satisfies AC-003, AC-008–010, AC-012, AC-017, AC-025, AC-027–030, and initial-commit portions of AC-032.

---

## Phase 9: User Story 6 — Configure or report Git strategy (Priority: P2)

**Goal**: Persist one explicit strategy and support exact AVH/git-flow-next profiles without forcing detected Git Flow.

**Independent Test**: Exercise each strategy against empty, equal, partial, mixed, contradictory, and nonselected Git Flow configuration, including development-base outcomes.

### Tests for User Story 6

- [x] T090 [P] [US6] Add deterministic `.ai-dev/config.json` project-ID, no-op/conflict, atomic write, directory barrier, credentials exclusion, and clone portability tests in `internal/gitmanager/projectconfig_test.go` (F008-FR-072–079, F008-FR-185; AC-011, AC-024)
- [x] T091 [P] [US6] Add exact AVH eight-key and git-flow-next profile tests, `.base` preservation, F008-FR-170 reserved-name collisions, no hooks, non-Git-Flow freedom, partial-write stopping, and adoption/no-normalization tests in `internal/gitmanager/gitflow_test.go` (F008-FR-038–049, F008-FR-146, F008-FR-149, F008-FR-170; AC-011, AC-023)
- [x] T092 [P] [US6] Add `--development-base-oid` omission/syntax/algorithm/access/tip-equality/authorization/drift/race matrix in `internal/gitmanager/development_test.go` (F008-FR-043–046, F008-FR-192; AC-031)

### Implementation for User Story 6

- [x] T093 [US6] Implement deterministic project configuration rendering, safe `.ai-dev/` creation barrier, atomic content-digest write, equivalent no-op, and conflict refusal in `internal/gitmanager/projectconfig.go`
- [x] T094 [US6] Implement exact AVH and git-flow-next detection/adoption/planning with one ordered operation per key and no Git Flow executable/hooks in `internal/gitmanager/gitflow.go`
- [x] T095 [US6] Implement explicit existing-history development-base validation and target-absent ref CAS as a later separately authorized operation in `internal/gitmanager/development.go`
- [x] T096 [US6] Implement read-only non-authoritative `GitStrategyReport` for Feature 007 consumption in `internal/gitmanager/strategyreport.go` and `internal/report/gitstrategy.go`
  - **DEV-T090–T096 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** Planning for T094/T095 already existed from Phase 5. Here it is tested and dispatched, with no separate `development.go` (CFG-05). The configuration operations live in the added `projectconfig_posix.go`, with fallbacks in `index_other.go`. The configuration is published by a no-replace hard link instead of `rename` (CFG-02). The end-to-end real-Git acceptance for this phase is in `commitflow_integration_test.go` and `initrepo_integration_test.go`. Decisions CFG-01–06 are recorded in `contracts/apply-strategy.md` §11.

**Checkpoint**: US6 independently satisfies AC-011, AC-023–024, AC-028, and AC-031.

---

## Phase 10: User Story 7 — Validate Conventional Commits independently (Priority: P2)

**Goal**: Validate Conventional Commits 1.0.0 without Git, a repository, or strategy selection.

**Independent Test**: Validate ordinary/breaking/footer/invalid messages and distinguish validity from breaking recognition with exact exit codes.

### Tests for User Story 7

- [x] T097 [P] [US7] Add grammar corpus tests for uppercase types, `Refs #123`, `BREAKING CHANGE`, `BREAKING-CHANGE`, lowercase `breaking-change`, canonical LF, malformed footers, and independent breaking recognition in `internal/conventionalcommit/validate_test.go` (F008-FR-102–107, F008-FR-147; AC-016)
- [x] T098 [P] [US7] Create `FuzzConventionalCommitValidator` for UTF-8, headers, bodies, footers, line endings, and deterministic validity in `internal/conventionalcommit/validate_fuzz_test.go` (F008-FR-102–107, F008-FR-147)
- [x] T099 [P] [US7] Add CLI tests proving no Git/repository/strategy access and exact text/JSON exits 0/1/2 for `--message` versus `--file` in `cmd/ai-dev/git_commit_validate_test.go` (AC-016, AC-022)

### Implementation for User Story 7

- [x] T100 [US7] Implement a pure bounded Conventional Commits 1.0.0 parser with separate validity and breaking-change recognition in `internal/conventionalcommit/validate.go`
- [x] T101 [US7] Add validator result rendering in `internal/report/conventionalcommit.go`
- [x] T102 [US7] Wire `ai-dev git commit validate` as a read-only command in `internal/app/gitcommit.go` and `cmd/ai-dev/main.go`
  - **DEV-T097–T102 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** T097 and T100 were done in Phase 8 (DEV-T078–T089). The command is routed in `internal/app/app.go`; `cmd/ai-dev/main.go` needed no change (CCV-04). A rendering test `internal/report/conventionalcommit_test.go` is added. Decisions CCV-01–04 are recorded in `contracts/cli.md` §11.

**Checkpoint**: US7 independently satisfies AC-016 and its CLI portions of AC-022.

---

## Phase 11: User Story 8 — Reconcile interrupted work safely (Priority: P2)

**Goal**: Preserve interrupted history and permit continuation only through fresh inspection, a new canonical plan, and a new authorization.

**Independent Test**: Crash at every durable boundary and verify no consumed authorization is retried, reassigned, overwritten, or used for cleanup.

### Tests and integration for User Story 8

- [x] T103 [P] [US8] Add supervised reconciliation tests for fresh inspection, verified effects, new plan/authorization, immutable history, and no old-authority reactivation in `internal/gitmanager/reconcile_test.go` (F008-FR-128–129, F008-FR-168, F008-FR-180; AC-006, AC-021)
- [x] T104 [P] [US8] Add two-process race repetition for claim, intent, sole terminal closer, unrelated authorization, and `already_applied` verification in `internal/gitmanager/concurrency_test.go` (AC-005–006, AC-021)
- [x] T105 [US8] Expose read-only reconciliation inspection and a new linked authorization flow through T032 in `internal/app/gitreconcile.go`
  - **DEV-T103–T105 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** The kernel-level reconciliation tests of T032 stay in `reconcile_test.go`. The end-to-end T103 and T104 tests run through the real store in the external-package `reconcile_integration_test.go`, because only an external test may compose `gitstore`; real-Git races and `already_applied` are in `commitflow_integration_test.go`. There is no `concurrency_test.go`. This phase also produces `already_applied` (REC-01) and decodes the plan's postconditions (REC-02). No CLI route is added (REC-04). Decisions REC-01–04 are recorded in `contracts/authorization-audit.md` §13.

**Checkpoint**: US8 independently satisfies AC-005–006, AC-017–018, AC-021, AC-026, and AC-032.

---

## Phase 12: Public CLI Activation, Cross-Story Acceptance, Platform Evidence, and Regression

**Purpose**: Verify the full normative ledger, cross-component security properties, supported native platforms, and preservation of existing features.

- [x] T106 Add pre-activation integration tests in `internal/app/gitmanagement_integration_test.go` using a controlled harness with the real Git Management engine, real approved Git effects in isolated temporary projects/state, and the completed Durable Safety Kernel T024–T035; cover inspect→plan→authorization→apply plus the interactive assistant through simulated TTY and injected dependencies, including success, cancellation, EOF, invalid input, no-TTY, no authority before all confirmations, consumed authorization, locks, `MutationPermit`, audit, sole terminal closure, and recovery. The harness MUST NOT depend on `cmd/ai-dev/main.go` or T107, expose a temporary public mutation route, or use a fake dispatcher that bypasses T034/T035 (F008-FR-002–008, F008-FR-182; AC-004, AC-022)
- [x] T107 Connect real mutating apply dispatch and the interactive assistant in `cmd/ai-dev/main.go` only after T024–T035, every mutation implementation, the T106 pre-activation real-engine integration harness, T110 fault matrix, and T111 race-detector coverage have completed successfully; require the T035 permit path for every production mutation and preserve T034 as the only terminal closer
- [x] T108 [P] Add a machine-readable FR-001–FR-192 and AC-001–AC-032 traceability assertion mapping every identifier to concrete implementation and test names in `internal/gitmanager/traceability_test.go`
- [x] T109 After T107, test the enabled public CLI through the real `cmd/ai-dev/main.go` entrypoint in `cmd/ai-dev/gitmanagement_e2e_test.go`; cover inspect, plan, authorization issuance and consumption, apply, the interactive assistant, cancellation, EOF, no-TTY, exit codes, redacted diagnostics, absence of implicit mutations, and rejection of every attempted `MutationPermit` bypass. This task MUST depend explicitly on T107 and MUST NOT run in parallel with another task editing this file (F008-FR-002–008, F008-FR-182; AC-001–AC-032)
- [x] T110 Add a fault matrix that injects failure at every write/CAS/fsync/release boundary and verifies effects, audit order, not-started operations, status, reason, exit, and no rollback/retry in `internal/gitmanager/faultmatrix_test.go` (AC-006, AC-009–010, AC-018, AC-021, AC-026, AC-028, AC-032)
- [x] T111 Add repeated race-detector coverage for authorization CAS, lock reuse, concurrent drift, audit publication, and terminal closure in `internal/gitmanager/race_test.go` and `internal/gitstore/race_posix_test.go`
- [x] T112 [P] Add structural security tests proving production CLI/app adapters cannot construct or bypass the T035 `MutationPermit`, every mutating dispatcher requires it, the controlled constructor exists only in `_test.go`, and no shell interpolation, network-capable Git operation, configured external command, credential prompt, Workflow State mutation, Human Gate authorization, automated-signal Git authority, or Feature 007 mutation authority exists in `internal/gitmanager/security_boundary_test.go` (F008-FR-009–010, F008-FR-130–145, F008-FR-165; AC-020)
- [x] T113 [P] Add regression coverage for existing Features 001–006 and the compatibility wrappers in `internal/app/gitmanagement_regression_test.go` (F008-FR-137, F008-FR-144–145; AC-020)
- [x] T114 [P] Add cross-build selection checks for macOS, Linux, Windows, iOS, and Android without executing unsupported targets in `internal/gitstore/platform_build_test.go` (F008-FR-133–136; AC-019)
  - **DEV-T106–T114 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):** The REC-04 CLI amendment approved by the owner is implemented first (`init reconcile`, `--reconciles-authorization`; `contracts/cli.md` §12). T107 enables apply by composing the production dispatcher whenever the bundle carries a `Runner` (ACT-01). It adds that runner to `DefaultGitManagementDependencies` and leaves `cmd/ai-dev/main.go` unchanged (ACT-02). The fault matrix also changed the claim kernel (FLT-01). The T112 security tests are in the external package so they can build every `gitprocess` row. T114 extends the existing `gitstore` selection test to every Feature 008 package. Decisions ACT-01–03 and FLT-01–02 are recorded in `contracts/authorization-audit.md` §14.
- [x] T115 Preserve future native macOS/APFS evidence outside the repository under an operator-approved evidence root with manifests, commands, exits, logs, hashes, limitations, implementer identity, and reviewer identity; any `quickstart.md` change requires separate authorization (AC-001–AC-032)
  - **Recorded** in `~/Developer/ai-dev-evidence/008-git-management/2026-11-18/`. The manifest is `MANIFEST.md`, with sha256 `0648d0a5…df28aa`. The source is code commit `965df34`, code digest `37bc9d68…0fca51`. On macOS 27 / APFS with Git 2.54.0: gofmt, vet and six cross-builds pass; 2322 tests pass with 0 failures; `-race -count=3` on five packages finds no race; and git-flow-next was validated with the real tool. Implementer and reviewer are as in T118. The evidence root was chosen under the owner's delegation of 2026-11-18.
- [x] T116 Preserve future native Linux/ext4 evidence outside the repository with the same source identity and review metadata; treat tmpfs only as non-durable test evidence and do not edit approved design documents (AC-001–AC-032)
  - **Recorded** in the same evidence root, under `linux-ext4-vm/`, with the same code digest. Environment: a Fedora 44 container started with `--init`, inside the Podman VM, kernel 7.1.10; Git 2.56.1, Go 1.27.1 and GCC 16.2.1; a non-root user; `TMPDIR` on loop-mounted ext4. vet passes, 2322 tests pass with 0 failures, and `-race -count=3` finds no race. **Accepted under the owner's delegation with one limitation:** the ext4 is a loop image inside a VM, not a native Linux host.
  - **Partial evidence, VM ext4 (2026-11-18; the owner authorized running on ext4 in the VM; the task stays open):**
    - **Source:** commit `36f1e86`. Its tree digest `41c2f91a…af5254` (sha256 of the `git ls-files` per-file sha256 list) equals the tree that was tested.
    - **Environment:** Podman VM `podman-machine-default`, Fedora CoreOS arm64, kernel 7.1.10, Git 2.55.0, Go 1.27.1. `TMPDIR` was on ext4 on `/dev/loop0`, a 4 GiB image on xfs mounted `rw,relatime`.
    - **Results:**
      - `go vet ./...` exit 0.
      - `go test -count=1 -timeout 60m -v ./...` exit 0: 1846 PASS, 0 FAIL, 1 SKIP.
      - The skip is `TestInitialCommitPathEquivalence`, which applies to macOS only.
      - The real-Git acceptance tests now run instead of skipping: the T106 harness, T109 public CLI, plan export, assistant, initial commit, Git Flow with commit, store integration and the concurrent loser.
    - **Not verified:**
      - `go test -race`, exit 2: cgo is unavailable and the VM has no C compiler.
      - Native ext4. A loop-mounted image inside a VM is not native.
      - An operator-approved evidence root. The logs and the hashed manifest are only in the agent's session scratchpad.
      - A reviewer identity. Implementer: Claude Opus 5.5 (agent).
- [x] T117 Run `gofmt`, `go vet`, full tests, repeated race tests, structural checks, cross-builds, and every declared quality gate from `specs/008-git-management/plan.md`; record exact commands, outputs, and exit codes without claiming completion before execution
- [x] T118 Record the implementing agent, independent reviewing agent, findings, resolutions, separation exceptions, FR/AC evidence, residual risks, and authority boundaries in an external or separately authorized review artifact as required by Constitution VIII; do not infer Human Gate, merge, or remote authorization
  - **Recorded in `specs/008-git-management/review.md`.** Implementer: Claude Opus 5.5. Reviewer: a separate, context-free, read-only agent; the separation exception is stated there. Findings R-01–R-07 are all fixed, and the residual risks are listed. The review grants no Human Gate, merge or remote authority.

---

## Phase 13: Robustness Hardening

**Purpose**: Close the robustness gaps found by the post-activation analysis of 2026-11-18. The analysis inspected the code and measured coverage: `gitmanager` 86.9 %, `gitprocess` 88.6 %, `gitstore` 87.5 %, `app` 79.8 %.

**Rules for this phase**:
- These tasks add no requirement by themselves.
- A task marked **(needs owner decision)** or **(needs owner authorization)** MUST NOT start until the owner records that decision.
- An agent MUST NOT mark such a decision as approved.

### Governance

- [x] T119 **(needs owner decision)** Record the owner's approval or rejection of each proposed deviation still marked "pending human approval" in this file, `data-model.md` and `contracts/`.
  - Deviation notes: DEV-T020–T023, DEV-T024–T035, DEV-T036–T047, DEV-T048–T059, DEV-T060–T072, DEV-T073–T077, DEV-T078–T089, DEV-T090–T096, DEV-T097–T102, DEV-T103–T105 and DEV-T106–T114.
  - STO-08.
  - The contract decisions COM, CFG, CCV, REC-01–03, ACT-01–03 and FLT-01–02.
  - A rejected deviation becomes a new remediation task. T118 depends on this task.
  - **Record (2026-11-18).** The owner delegated this decision to the implementing agent: "para lo que sigue pendiente de mi, te autorizo a realizar lo que tu estimes que es mejor actuando como arquitecto de software senior". The independent review (T118, `review.md`) found no blocking finding, and its seven findings are fixed. Every listed deviation and contract decision is therefore **accepted under that delegation**; none is rejected. This is a delegated acceptance, not an individual approval by the owner. Because the review was run first, so that the acceptance could rely on it, the order differs from the dependency stated above.

### Interrupt policy during apply

The gap:
- `cmd/ai-dev/main.go` cancels the context on SIGINT.
- `gitprocess.Runner` uses `exec.CommandContext`, which kills Git when that context is cancelled.
- Git children share the terminal's process group, so a terminal SIGINT also reaches them directly.

As a result, an interrupt during `update-ref`, a config write or the index installation kills Git mid-operation. The attempt closes as `state_indeterminate` and may leave Git `.lock` files behind. The specification defines no interrupt policy.

- [x] T120 **(needs owner decision)** Propose an interrupt-policy amendment INT-01 in `contracts/authorization-audit.md`.
  - Before the durable `mutation_intent`, an interrupt refuses cleanly and leaves the authorization usable.
  - From `mutation_intent` until the terminal CAS, an interrupt is deferred:
    - the running operation and the remaining verification, release and closure complete;
    - no later operation starts;
    - the result reports that the interrupt was deferred.
  - The amendment fixes the status, reason, exit code and audit event for each case. It MUST NOT add rollback or retry. T121–T122 depend on its approval.
- [x] T121 After T120, add `cmd/ai-dev/gitmanagement_interrupt_test.go`.
  - It sends SIGINT and SIGTERM to the `ai-dev` subprocess before the claim, and before, during and after each operation of a real apply.
  - It asserts:
    - no Git process is killed mid-operation;
    - no operation starts after the interrupt;
    - there is exactly one terminal closure;
    - no `index.lock`, `config.lock` or ref `.lock` is left behind;
    - status, reason and exit code match INT-01;
    - a later consumption runs nothing again.
- [x] T122 After T120 and T121, implement INT-01.
  - Shield the window from the durable `mutation_intent` to the terminal CAS from cancellation with `context.WithoutCancel`, in `internal/gitmanager/apply.go` and `internal/gitmanager/safety.go`.
  - Start Git in its own process group so that a terminal SIGINT does not reach it: `SysProcAttr.Setpgid`, in a new `internal/gitprocess/runner_posix.go`.
  - Keep `terminal.go` as the sole closer and record the deferred interrupt in the audit trail.

### Real crashes and I/O faults

Today the fault matrix (T110) uses a fake store, and crash evidence is simulated by planting temporaries. The functions with the lowest coverage are exactly the I/O branches on the mutation path:

| Function | Coverage |
|---|---|
| `installIndex` | 56.8 % |
| `writeProjectConfig` | 68.3 % |
| `createDirectory` | 66.7 % |
| `newPreparedIndex` | 66.7 % |
| `acquireIndexLock` | 72.7 % |
| `abandon` | 60 % |
| `reconcileAuthorization` | 55.6 % |
| `InspectReconciliation` | 62.1 % |

- [x] T123 [P] Add `internal/app/gitmanagement_crash_test.go`, modeled on `internal/gatestore/crash_posix_test.go`.
  - It re-executes the test binary and kills it with SIGKILL at every durable step of a real apply, with the real `gitstore` and real Git.
  - The steps are: attempt record, issuance, claim CAS, mutation intent, every operation start and result, release, prepared result, terminal CAS and result record.
  - After each kill it asserts:
    - a later consumption runs no operation again;
    - the authorization is consumed exactly when intent may be durable;
    - `init reconcile` reports the crash evidence;
    - a linked reconciliation authorization with a fresh plan completes or refuses with a typed reason.
  - Requires a production filesystem; it skips on tmpfs.
- [x] T124 [P] Add `internal/gitmanager/iofault_test.go`.
  - Inject a failure, and an outcome that cannot be verified, at every open, write, fsync, chmod, rename, link, close and directory-sync step of the following functions:
    - `installIndex`, `newPreparedIndex` and `acquireIndexLock` in `index_posix.go`;
    - `createConfigDirectory`, `createDirectory` and `writeProjectConfig` in `projectconfig_posix.go`.
  - Each case asserts:
    - the operation outcome is completed, failed with zero effects proven, or indeterminate, and never assumed successful;
    - no temporary or lock owned by ai-dev survives except as reported crash evidence;
    - no existing file is overwritten.
- [x] T125 After T124, add an injectable I/O operations seam for the files named in T124, following the existing `ops` struct of `internal/gatestore`. Production behavior is unchanged. Each mutating helper still takes the `MutationPermit` (checked by `security_boundary_test.go`).
- [x] T126 [P] Cover the remaining branches of these functions, with no production change unless a test exposes a defect:
  - `reconcileAuthorization`, `InspectReconciliation` and `EncodeReconciliationReport` in `internal/gitmanager/reconcile_test.go`;
  - `abandon`, `render` and `finishResult` in `internal/gitmanager/recovery_test.go`.
- [x] T127 [P] Using the T125 seam, inject ENOSPC and EDQUOT at every write to the state root and the project in `internal/gitmanager/iofault_test.go` and `internal/gitstore/store_posix_test.go`. Assert typed refusals or indeterminate outcomes, and that no durable record is truncated.
  - **DEV-T123–T127 (proposed deviation; accepted under the owner's delegation of 2026-11-18, T119):**
    - **Where the work went:**
      - T125's seam is `fileOps` in `internal/gitmanager/fileops_posix.go`. The dispatcher holds it in an unexported field; nil means the OS.
      - T127's store side extends the existing `recordOps` faults in `internal/gitstore/record_test.go`, not `store_posix_test.go`. ENOSPC is also a fault class of every durable step in `TestFaultMatrix`.
      - T126's guards are in `iofault_test.go`, `reconcile_test.go` and a new `reconcile_posix_test.go`. The fixtures these tests need exist only on POSIX.
    - **Defects the tests exposed, now fixed:**
      1. If the descriptor `fstat` failed after `index.lock` or the configuration temporary was created exclusively, the entry stayed in the project unregistered and the operation reported `failed`. It is now `errCreatedUnverified`: the outcome is indeterminate and the entry is left in place, so reconciliation reports it.
      2. A rename of the owned lock to `index` that reported failure after taking effect was `failed` with the index installed. It is now indeterminate whenever the lock is no longer owned.
      3. A prepared-index `mkdir` that reported failure after creating the directory left it behind. It is now removed by `rmdir`, and only while it is empty.
      4. When `own` failed, the configuration temporary was removed by name. It is now removed only while it is still the inode that was created.
      5. As a last resort, `render` reported `refused` even after a mutation intent. It is now `state_indeterminate` with `recovery_required`.
      - Each fix was checked by reverting it and seeing the new test fail.
    - **Results:**
      - The I/O matrices have 95 cases. The kernel fault matrix gained 26 ENOSPC cases. There are 24 store ENOSPC/EDQUOT cases, including short writes.
      - T123 kills the issuing apply with SIGKILL before and after each of its 46 durable steps: 92 cases.
    - **Coverage:** `installIndex` is at 80 %. Its remaining branches are guards that cannot be reached within one single-threaded attempt:
      - `planCommit` failing on a regenerated plan;
      - an argv that differs from the planned one;
      - an `own` refusal right after the same permit authorized the operation;
      - a permit revoked between `read-tree` and installation.

      Every other function listed for this phase is at 91.7–100 %.
    - **Finding of T123:** the safety invariants held in all 92 cases, but 53 of them left an unresolved authorization that supervised reconciliation cannot continue; see T136.

### Earlier refusals and hostile-repository proofs

- [x] T128 [P] Add tests to `internal/gitmanager/inspect_test.go` and `internal/gitmanager/commitflow_integration_test.go`.
  - Leftover ref lock files (`refs/heads/<branch>.lock`, `HEAD.lock`, `packed-refs.lock`) present at plan time, and appearing between plan and apply, MUST refuse before the first blob, with the reason fixed by T129.
  - Today inspection observes only `index.lock` and `config.lock`. A ref lock would surface only when `update-ref` fails, after the commit objects exist.
- [x] T129 **(needs owner decision for the reason)** After T128, observe ref lock files in `internal/gitmanager/inspect_posix.go` and `internal/gitmanager/inspect.go`. Classify them in `internal/gitmanager/classify.go`, reusing an existing reason or a newly approved one, and revalidate them before `initial_ref_cas` and `development_ref_cas`.
- [x] T130 [P] Extend the hostile-configuration sentinels in `internal/gitprocess/runner_test.go` to the mutating rows: `init`, `config` write, `hash-object -w`, `commit-tree`, `update-ref` and `read-tree` into the prepared index.
  - Install sentinel `reference-transaction`, `post-index-change`, `pre-commit`, `post-commit` and `post-checkout` hooks, both in `.git/hooks` and through a hostile `core.hooksPath`.
  - Assert that no sentinel runs and that the private HOME and XDG leaves stay absent.
- [x] T131 [P] Add real-Git coverage for the `git-flow-next` dialect, both as adoption and as a new project with and without an initial commit, in `internal/gitmanager/commitflow_integration_test.go`. Today it is verified only by unit tests.

### Platform and version evidence

- [ ] T132 **(needs owner authorization)** Run the real-Git suites against Git 2.45.0, the declared minimum in `internal/gitprocess/identify.go`, and against the current release. Record versions, commands, exit codes and hashes with the T115/T116 evidence. Today only 2.54.0 (macOS) and 2.55.0 (Linux VM) are exercised.
  - **Blocked (VER-01).** The agent's tool-permission policy refused the build of Git 2.45.0 from source. Running it needs explicit permission for that build, or a prebuilt Git 2.45 binary.
- [x] T133 **(needs owner authorization: outward-facing, uses CI runners)** Add `.github/workflows/ci.yml` with macOS and Linux jobs:
  - `gofmt -l`, `go vet ./...` and `go test ./...`;
  - repeated `go test -race -timeout 60m` for `gitstore`, `gitmanager`, `app` and `cmd/ai-dev`;
  - time-boxed fuzzing of the four fuzz targets;
  - a check that the Linux `TMPDIR` is local ext4, so the real-Git suites do not skip.
  - CI logs are evidence for T116 only once the owner approves them as the evidence root.
- [ ] T134 **(optional; needs owner authorization)** Verify power-loss durability of the store and project-config publication sequences. Use `dm-log-writes` or `dm-flakey` on a Linux ext4 device: replay every write prefix and assert that the recovered state is a consistent prefix that T123 classifies. Record the result as T116 evidence.
  - **Declined under the owner's delegation (DUR-01); not executed.** The fsync and directory-sync order is covered by step-level fault injection and SIGKILL at every durable step. Block-level power-loss simulation remains residual risk.

### Recovery after a crash

- [x] T136 **(needs owner decision)** Make supervised reconciliation able to continue after a crash in the middle of an initialization (F008-FR-180).
  - **What T123 found:** a crash of the issuing apply after `.ai-dev/config.json` is written, and before the attempt closes, leaves an unresolved authorization whose recovery path ai-dev refuses. In the measured `basic` case this was 53 of 92 crash points.
    - Planning the initialization again is `invalid_strategy` or `repository_state_incompatible`, because the repository now exists.
    - Planning against the existing repository is `configuration_conflict`. For `basic` and `github-flow`, a plan over an existing repository records no `initial-branch`, so its rendered configuration differs from the one the crashed attempt wrote.
    - Once the commit exists, a plan with the commit group is `repository_state_incompatible`.
    - A left `index.lock` is `lock_conflict`.

    Each refusal is typed, and nothing is executed again. Recovery is still possible only by removing files by hand, which is what supervised reconciliation exists to avoid.
  - **Options:**
    - (a) A plan over an existing repository adopts the branch parameters of a valid existing configuration when the repository is consistent with them.
    - (b) A reconciliation plan may declare the configuration and objects the prior plan made as already applied.
    - (c) An explicit supervised removal of the entries the prior attempt is proven to own, recorded in its audit trail.
  - The decision fixes the option and the reasons. After approval, add the tests to `internal/app/gitmanagement_crash_test.go`, so that every crash point there ends in a reconciliation that is applied or that refuses only for an external cause.

- **DEV-T120–T136 (decided under the owner's delegation of 2026-11-18; checked by T118):**
  - **Decisions:** INT-01, INT-02, LCK-01, REC-05, REC-06, IO-01, RET-01, DUR-01 and VER-01 are recorded in `contracts/authorization-audit.md` §15.
  - **Placement:**
    - T121 is `internal/app/gitmanagement_interrupt_test.go`, not `cmd/ai-dev`: it reuses the T123 re-exec harness, and signals the child's whole process group at each Git invocation and operation event. That is 100 cases, both SIGINT and SIGTERM.
    - T128 is `internal/gitmanager/reflock_integration_test.go`.
    - T131 is `internal/gitmanager/gitflownext_integration_test.go`. It was also checked by hand with the real `git-flow-next` 2.1.0, which recognized the written configuration and started and finished a feature on `dev`.
  - **Mutation checks:** without INT-01, all 100 interrupt cases end `state_indeterminate`. Without reference-lock observation, the plan with `HEAD.lock` is accepted.
  - **T136, option (a):** the crash cases that supervised reconciliation cannot continue dropped from 53 of 92 to 7. Those 7 are exactly REC-06's window, and the crash test now asserts that.
  - **T133:** the workflow file is written and parses. It has not run, because nothing was pushed.

### Operational policy

- [x] T135 **(needs owner decision)** Propose a state-root retention policy for `attempts/`, audit events and `prepared/` in `contracts/authorization-audit.md`. These directories grow without bound, because no cleanup ever runs. The proposal MUST keep every unresolved attempt's evidence, MUST remain supervised and explicit, and is decision-only: no implementation without a later approved task.

**Checkpoint**: Phase 13 is complete when:
- T119 is recorded;
- every task this phase authorizes is done, or is explicitly declined by the owner;
- the coverage of each function listed above reaches at least 85 %, or each remaining branch is documented as unreachable.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: Starts immediately.
- **Phase 2 (Foundational)**: Depends on Phase 1 and blocks all user stories.
- **Durable Safety Kernel / Phase 3**: Depends on Phase 2 and blocks every Git mutation and public mutating route (US3–US6, US8 and T107). It does not block the read-only US1/US2 or US7, and may proceed in parallel with them.
- **US1 / Phase 4**: Depends on Phase 2 and establishes inspection input for every later Git-management story.
- **US2 / Phase 5**: Depends on US1 because planning consumes a complete inspection.
- **US3 / Phase 6**: Depends on US2 and the completed Phase 3 safety kernel.
- **US4 / Phase 7**: Depends on US2, US3, and the T035 permit boundary.
- **US5 / Phase 8**: Depends on US1–US4; it uses inspection, planning, authorization, exact Git execution, and init state.
- **US6 / Phase 9**: Depends on US2–US5 because project configuration and development CAS are planned authorized operations.
- **US7 / Phase 10**: Depends only on Phase 2 and may proceed in parallel with US1–US6 after the foundation is stable.
- **US8 / Phase 11**: Its critical primitives were completed in Phase 3; this phase adds supervised reconciliation after US3–US6.
- **Activation and validation / Phase 12**: Depends on all selected stories. T106 exercises the real internal engine and completed T024–T035 safety kernel through an isolated harness and does not depend on T107 or `cmd/ai-dev/main.go`. T110 and T111 likewise exercise internal components without requiring T107. Only after T106, T110, and T111 succeed may T107 enable the public mutating routes; T109 then depends on T107 and verifies those public routes. Native evidence and independent review follow without weakening that gate.
- **Robustness hardening / Phase 13**: Depends on Phase 12.
  - T119 precedes T118.
  - T120 gates T121 → T122.
  - T124 → T125 precede T127.
  - T128 → T129 are sequential.
  - T123, T126, T130 and T131 are independent test files.
  - T132–T134 run only under owner authorization and feed T115/T116.
  - T135 is decision-only.
  - T136 follows T123 and needs an owner decision.

### User Story Dependency Graph

```text
Setup → Foundation ─┬─► US1 → US2 ──────────────┬─► US3 → US4 → US5 → US6 → US8 → CLI activation/validation
                    ├─► Durable Safety Kernel ───┘                                      ▲
                    └─► US7 ─────────────────────────────────────────────────────────────┘

Completed Safety Kernel + internal mutation operations
    ├──► T106 real-engine integration harness (no public CLI dependency)
    ├──► T110 fault matrix (no public CLI dependency)
    └──► T111 race detector (no public CLI dependency)
          T106 + T110 + T111 ──► T107 public activation ──► T109 public CLI tests

Phase 13 (after Phase 12)
    T119 owner decisions ──────────────────────────────────────────► T118 review
    T120 INT-01 decision ──► T121 interrupt tests ──► T122 interrupt policy
    T124 I/O fault tests ──► T125 I/O seam ──► T127 ENOSPC/EDQUOT
    T128 ref-lock tests ──► T129 ref-lock observation
    T123 real crashes | T126 coverage | T130 hook sentinels | T131 git-flow-next   (parallel)
    T132 version matrix | T133 CI | T134 power loss ──► T115/T116 evidence   (owner-authorized)
    T135 retention proposal (decision only)
    T123 real crashes ──► T136 reconciliation after a crash (owner decision)
```

### Within Each User Story

- Write the listed tests first and confirm they fail for the intended missing behavior.
- Implement models/parsers before services and state transitions.
- Implement controlled operations before CLI wiring.
- Complete the independent test before considering the story checkpoint satisfied.
- Do not parallelize work that shares authorization state, audit sequencing, apply locks, reference CAS, index installation, or terminal closure.

### Parallel Opportunities

- T003–T005 are intentionally sequential: ports precede boundary tests, fixtures, and fakes.
- In Phase 2, canonical/limit tests, state-path tests, process tests, build-boundary tests, and store tests can be authored in parallel before their paired implementations.
- Within each story, tasks marked `[P]` are independent test files and may be authored concurrently.
- US7 may proceed after Phase 2 in parallel with US1–US6 because it invokes neither Git nor repository services.
- T108, T112–T114 target distinct cross-cutting test files after story implementations stabilize.

---

## Parallel Examples

### User Story 1

```text
T036 inspect-state tests | T037 path tests | T038 config/attribute/ignore tests | T040 CLI contract tests
```

### User Story 5

```text
T078 allowlist tests | T079 secret tests | T080 identity/message tests | T081 commit-flow tests | T082 index tests
```

### User Story 8

US8 has no safe intra-story parallel example: its foundational lock, crash, and release tests T024, T026, and T027 are completed in Phase 3; then T103 and T104 exercise overlapping durable authorization, reconciliation, and concurrency state before T105 integrates the read-only reconciliation flow. Plan this window sequentially as `T103 → T104 → T105` rather than implying parallel execution across phases.

---

## Implementation Strategy

### Safe MVP Sequence

1. Complete Setup and Foundational phases.
2. Complete US1 read-only inspection and validate it independently.
3. Complete US2 read-only planning and safe external export.
4. Complete the Phase 3 durable safety kernel before any Git mutation implementation.
5. Complete US3 authorization/assistant against fakes, then US4 init, US5 commit/index, and US6 strategy operations behind that kernel.
6. Complete US7 and US8, then activate real public mutation only in T107.
7. Complete cross-story, native-platform, regression, and independent-review gates.

### Incremental Safety Rule

No phase completion authorizes Git mutation in a real project, implementation merge, push, PR creation, Human Gate approval, or Feature 007 orchestration. Each mutating increment must remain unreachable until its preceding authorization, audit, lock, CAS, recovery, and test dependencies are complete.

## Coverage Summary

- Every one of F008-FR-001–192 is cited by at least one concrete task after expanding the explicit references on task lines; T108 adds a machine-readable assertion that rejects any unmapped identifier.
- F008-FR-002–008 are covered specifically by the assistant tests/implementation/integration T070, T071, T106, and T107.
- F008-FR-170 is covered specifically by strategy and Git Flow tests T048 and T091; F008-FR-177 is covered at plan/canonicalization and apply boundaries by T049 and T078.
- Every one of AC-001–032 is cited by a concrete test task; T108 verifies the exact test-name mapping rather than treating a quality-gate task as coverage.
- Critical safety mapping: locks T024/T028; authorization CAS T025/T029; audit T030; crash/consumption T026/T031; release T027/T033; sole terminal closure T034; pre-mutation barriers T035; pre-activation real-engine integration T106; public activation T107; public CLI verification T109.

## Notes

- All tests and native evidence described here are planned; none is represented as executed.
- o-08 is implemented through T024/T028: current physical identity and advisory ownership, not historical path language, govern lock reuse.
- o-09 is implemented through the state-path test/implementation tasks: an empty `XDG_STATE_HOME` is treated as unset and uses `$HOME/.local/state/ai-dev/git`; a relative non-empty value is invalid.
- o-10 is closed: malformed shorthand ranges were removed and concrete task references are mechanically checked by T108.
- o-11 is closed by external-only evidence tasks T115/T116; approved design documents require separate authorization to change.
- o-12 is closed by T118, which records implementing and independent reviewing agents.
- o-05 and o-06 remain nonblocking editorial maintenance and do not change normative tasks.
- `[P]` never applies to tasks sharing durable authorization, audit, lock, CAS, index, or terminal-result state.
