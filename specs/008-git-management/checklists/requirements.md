# Requirements Quality Checklist: Feature 008 — Git Management

**Purpose**: Validate completeness, clarity, consistency, traceability, and testability of the Feature 008 specification before independent SDD review.

**Created**: 2026-10-10

**Feature**: [spec.md](../spec.md)

**Review Ownership**: This is a requirements-quality artifact. Checked items confirm specification quality only; they do not indicate implementation or test completion.

## Content Quality

- [x] CHK001 The specification contains no unresolved template placeholders or clarification markers.
- [x] CHK002 The scope, exclusions, authority boundaries, supported platforms, and deferred Windows behavior are explicit.
- [x] CHK003 User stories are prioritized and independently testable.
- [x] CHK004 Acceptance criteria are stated as proposed tests and are not represented as executed.
- [x] CHK005 Residual MVP risks and limitations are explicit, including the same-user security limitation.

## Normative Coverage

- [x] CHK006 F008-FR-001 through F008-FR-192 appear exactly once in the active requirements ledger.
- [x] CHK007 The recovered precedence chain and SHA-256 provenance are recorded.
- [x] CHK008 Explicit replacements from Steps 34 and 36 and later harmonizations of FR-035/187 and FR-169/184 are declared without silently discarding cumulative guarantees.
- [x] CHK009 Additions from Steps 38 and 40 are preserved without renumbering earlier requirements.
- [x] CHK010 Feature 007 remains reserved and receives only a non-authoritative read-only `GitStrategyReport`.
- [x] CHK011 Features 001–006, Workflow State, and Human Gate authority remain outside Feature 008 mutation authority.

## Final Step 41 Amendments

- [x] CHK012 N-E fixes the exact new-repository argv with `--ref-format=files` and `git_capability_missing`.
- [x] CHK013 P-1 requires global preflight, including intended config bytes, before the first mutation whenever applicable and always before blobs.
- [x] CHK014 P-2 classifies outcomes using all effects of the attempt, not blob creation alone.
- [x] CHK015 P-3 gives F008-FR-190 plus N-E precedence over the abbreviated F008-FR-181 example.
- [x] CHK016 P-4 uses canonical development/output reasons, separates `GitApplyResult.status` from `reason`, and maps each status to the durable authorization state.

## Safety and Verifiability

- [x] CHK017 Inspection/planning read-only guarantees and the explicitly external `plan --output` write are distinguishable.
- [x] CHK018 Authorization issuance, all terminal states including `failed`, durable transitions, concurrency, idempotency, scoped reconciliation, audit ordering, and lock ownership are specified with verifiable outcomes.
- [x] CHK019 The single initial-commit flow covers global preflight, exact bytes, secret screening, no filters/hooks/signing, ref CAS, autonomous index installation, and postcondition verification.
- [x] CHK020 Strategy freedom, complete AVH/git-flow-next key profiles, explicit branches, non-atomic configuration writes, and the separate development-ref CAS are testable and internally consistent.

## Step 43 Audit Corrections

- [x] CHK021 F-01 is resolved: project identity uses only canonical physical path, decimal device and decimal inode with one explicit digest formula.
- [x] CHK022 F-02 is resolved: any unverifiable post-mutation effect has precedence and yields `state_indeterminate`; fully verifiable incomplete effects yield `partially_applied`.
- [x] CHK023 F-03, n-02 and c-01 remain resolved: status, reason, durable authorization state and exceptional `mutation_started` persistence are mapped explicitly; repository-state precedence distinguishes initial incompatibility, authorized-snapshot drift and post-revalidation concurrency while preserving effect-based classification after mutation.
- [x] CHK024 F-04 is resolved: all approved AVH and git-flow-next keys and prohibited hook keys are present.
- [x] CHK025 F-05, N-01, r-01 and c-01 are resolved: the closed reason vocabulary and normative mapping define phase-based precedence among `repository_state_incompatible`, `plan_stale` and `concurrent_change` without changing their approved exit codes or other rejection categories.
- [x] CHK026 m-01–m-03 are resolved: state/retry scope, later-source harmonizations and the exact P-1 applicability condition are stated.
- [x] CHK027 m-04 is resolved: every FR range maps to acceptance criteria with observable expected outcomes; all tests remain proposed.
- [x] CHK028 m-05–m-07 are resolved: `--project-dir` is canonical, the prepared-index/13-step lock protocol is normative, and audit record terms are defined.
- [x] CHK029 Residual risks from Steps 38–41 include non-expiring issued authorization, partial external artifact and future Unicode incompatibility.
- [x] CHK030 No source file, implementation, earlier feature, Workflow State or Human Gate is modified or authorized by this correction.

## Notes

- The original 30 checklist items were reevaluated against the Step 50 microcorrected draft on 2026-10-11; Step 55 adds CHK031–CHK036 without rewriting that history.
- No implementation, plan, task list, or executed test evidence is implied by these markers.
- Independent review remains required before task generation or implementation.

## Step 55 Plan-Audit Corrections

- [x] CHK031 P-06 amends FR-046 and the CLI with explicit `--development-base-oid`, production-tip equality, canonical coverage, authorized operation and c-01 behavior without inference.
- [x] CHK032 P-07 defines primary reason plus structured `secondary_failures[]`, exit 1, release audit, prepared terminal result and digest-bound terminal CAS.
- [x] CHK033 P-01–P-05 are design-complete: acyclic Go DAG, path-free device/inode lock namespace, normative 13-step index order, exact Git operation matrix and in-process conservative attribute evaluator.
- [x] CHK034 m-01–m-13 have concrete design decisions, including output schemas, option matrix, reversible base64url paths, APFS/ext4 profiles, no-replace audit publication, portable project ID, hard limits, Git Flow adoption, state roots and idempotency tests.
- [x] CHK035 AC-001–030 are preserved; AC-031 and AC-032 verify the two authorized normative amendments. All remain proposed and unexecuted.
- [x] CHK036 At Step 55, the active ledger contained exactly F008-FR-001–192 once each; N-E, P-1–P-4, c-01, closed reasons and authority boundaries remained intact under the expressly authorized P-06/P-07 consequences. Step 57 reevaluation is recorded separately below.

## Step 57 Focused Corrections

- [x] CHK037 N-01 and N-02 are explicit: every release failure is a deterministic secondary entry with exit 1, and every development-base condition has one unambiguous reason/exit mapping.
- [x] CHK038 N-03 is explicit: system/global/XDG/local configuration, attributes and ignores are evaluated in process; unsafe configured code never executes; unproved equivalence rejects before mutation.
- [x] CHK039 N-04–N-12, M-1/M-2/M-3 and Step 63 L-1/L-2 are consistent: current advisory/descriptor/project/namespace proofs suffice for free-lock reuse without historical path/authorization evidence; rename/move remains usable; recycled tuples receive independent documentary authority; consumed authority is never reused; `<state-root>` has exactly one final `git`, environment leaves are direct children under `environment/`, and identical path bytes/profile version feed plan, digest, operation IDs and execution.
- [x] CHK040 N-13 records Steps 55, 55-B and 57 as express user decisions without inventing authenticated identity, signature, timestamp or unavailable artifact; AC-001–032 remain proposed and unexecuted.

## Reevaluation

All CHK001–CHK040 were reevaluated after the Step 63 L-1/L-2 correction. The checklist asserts documentary consistency only. M-1/M-2/M-3 remain closed; lock reuse is aligned with FR-071/168/179/180, and the state/environment root is canonical without `git/git`. Independent review remains pending; the specification remains Draft and no readiness for task generation or implementation is asserted.
