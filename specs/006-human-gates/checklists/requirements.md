# Specification Quality Checklist: Human Gates

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-10-05
**Last validated**: 2026-10-07
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- **Initial draft (2026-10-05)**: twelve open clarifications, C-1 to C-12. Result: 13/16.
- **Human clarification decisions applied (2026-10-06)**: all twelve decisions are recorded in the Clarifications section (Session 2026-10-06) and propagated through the following sections:
  - Responsibility Boundary, including a new Residual trust boundary subsection (C-12);
  - User Stories 1–5 and their acceptance scenarios;
  - Edge Cases;
  - FR-001 to FR-036, renumbered;
  - Key Entities;
  - SC-001 to SC-013;
  - Assumptions.

  All provisional and "pending" text was removed.
- **Precedence (FR-026)** is derived from approved sources only:
  - usage and decision validity come first, by the 004/005 usage-failure precedent;
  - 004 FR-029 order follows: target canonicality, feature recognition, state resolution, then rule lookup;
  - the 005 artifact check comes after 004 validity. C-8 requires an invalid replay to report `transition_invalid`, and 005 FR-006 forbids artifact inspection before recognition and state resolution succeed;
  - the stale-state check comes at commit, as C-7 requires ("before committing");
  - the report comes last.
- **Remaining open clarification, C-13** (4 marker sites: the Clarifications section, the Edge Cases, FR-011, and FR-026 step 1): how to classify an `abandonment` whose target is **non-canonical**.
  - The approved decisions force canonical decision/target mismatches to be usage failures. The fixed C-11 vocabulary has no outcome for them, and reusing `transition_invalid` or `target_unsupported` would change 004's semantics.
  - For a non-canonical abandonment target, usage failure (exit 2) and `target_unsupported` (exit 1) are both consistent with C-1 to C-12. The choice is therefore a genuine product decision and was not invented.
- **Derived rules to confirm at the gate (no marker)**. Each follows from approved precedent:
  - a supplied but empty or whitespace-only reason is a usage failure for either decision type, following 004 FR-020/FR-021 and "no normalization";
  - existing evidence that cannot be preserved yields `persistence_failed`, following C-6 ("must not erase prior evidence") and C-7;
  - the artifact precondition reflects artifact presence when it is evaluated, because C-7 covers the authoritative Workflow State only.
- **Anti-pattern verification (2026-10-06)**: each item was checked by searching the spec and reading the matches.
  - `transition_valid` is never authorization (FR-007, US2 AS2, SC-002).
  - Artifacts and agent success never imply approval (FR-006, US2).
  - 004's Transition Rule Set is referenced, not restated; no origin/target table exists in this spec.
  - 005's Required Artifacts table is referenced (FR-015), not restated.
  - There is no pre-execution authorization (FR-002, FR-008).
  - There is no deny or rejected decision type or state (FR-008).
  - There is no idempotent success and no `already_applied` outcome (FR-022, US2 AS4, SC-007).
  - No authenticated identity is claimed (FR-033, the Residual trust boundary, FR-027, SC-009).
  - No atomic-write mechanism is prescribed (FR-016–FR-018, Assumptions).
  - Partial persistence is forbidden (FR-016, SC-005).
  - The behavior of 001–005 is unchanged (FR-035, SC-013).
- **Content Quality note**: references to standard output and standard error, exit codes, regular files, and symbolic links describe the observable CLI and filesystem contract, as in specs 002–005.
- **Result after the C-12 round (iteration 3)**: 15/16. The only failing item was the open C-13 marker.
- **C-13 and derived rules applied (2026-10-06)**: C-13 = A (usage failure, exit 2). Decision-type consistency is usage validation and runs before target canonicality. D-1, D-2, and D-3 were confirmed as human decisions and recorded in a second Clarifications session. The four C-13 locations were updated consistently, plus the two dependent lists:
  - the Clarifications session;
  - the Edge Cases (two entries, including the contrast case: an `approval` with a non-canonical target gets `target_unsupported`);
  - FR-011;
  - FR-026 steps 1–2;
  - the FR-030 list;
  - User Story 5, acceptance scenario 3.

  The Assumptions entry on artifact timing now matches D-3 wording.
- **Defect fixed during the final review**: FR-026 said "Nothing is persisted before step 5", but persistence was step 7. With the new numbering, it now says step 9.
- **Final cross-spec consistency review against 004 and 005**: no contradictions found.
  - **Precedence**: FR-026 steps 3–6 are exactly 004 FR-029 steps 2–5. A non-canonical `approval` target never triggers a feature or record read, matching 004 FR-011 and SC-003.
  - **Usage failures**: steps 1–2 precede all domain checks, following the 004 FR-020 and 005 FR-019 precedent. A whitespace-only target is a usage failure, as in 004 FR-021.
  - **Reused outcomes**: `feature_unknown`, `state_invalid`, `target_unsupported`, and `transition_invalid` are produced under exactly 004's conditions. 006's field-presence rules for these outcomes match 004 FR-019. For example, `target_unsupported` carries the target as supplied but no current state, and `feature_unknown` and `state_invalid` carry no target. 006 adds only its own decision type and approver fields.
  - **Terminal origins and the same-state replay** produce `transition_invalid`, as in 004 FR-012.
  - **Artifacts**: `artifacts_missing` is a distinct 006 token. 005's `artifacts_inconsistent` and its meaning are untouched. The 005 Required Artifacts Rule (FR-010/FR-011) is referenced, including its fixed order, and never restated. No artifact is inspected before recognition and state resolution, as 005 FR-006 requires.
  - **Write path**: 004 FR-014 and 005 FR-004 (read-only) are unchanged. 004 FR-027 explicitly leaves the write mechanism to a later feature, and FR-019 requires persisted state to stay readable under 004's unchanged record rules. A symlinked or unreadable record yields `state_invalid` at step 5, before any write, consistent with the 004 record contract.
  - **Exit codes**: the 0/1/2 convention is unchanged. 004 and 005 exit codes are unaffected.
- **Anti-pattern verification (re-run)**:
  - `transition_valid` appears only as a non-authorizing signal;
  - no pre-execution authorization, deny, idempotent success, or `already_applied`;
  - no authenticated-identity claim;
  - no atomic-write mechanism;
  - partial persistence is forbidden;
  - 004 and 005 tables are not restated;
  - 001–005 behavior is unchanged (FR-035, SC-013).
- **Final result (iteration 4)**: **16/16 items pass.** No [NEEDS CLARIFICATION] markers remain. The specification is ready for the Human Specification Gate.
- **Product clarification P-6 (2026-10-07)**, raised at the Human Plan Gate and applied narrowly:
  - **Spec changes**:
    - a new Clarifications session, 2026-10-07;
    - two new Edge Cases (unsupported platform; filesystem where the primitives cannot be established safely);
    - FR-018, which now cross-references FR-037;
    - a note on FR-026 step 8: where FR-037 applies, `persistence_failed` is produced before any stale-state check or write;
    - a new FR-037 (platform and filesystem scope);
    - a platform-applicability note under Measurable Outcomes;
    - a new SC-014;
    - Assumptions: a platform-scope entry, plus two out-of-scope items (Windows persistence, distributed/network-filesystem coordination).
  - **Unchanged**: no new outcome; C-1–C-13, D-1–D-3, FR-016–FR-023 atomicity and stale-state guarantees, Human Authority, the precedence order of steps 1–7, and every refusal outcome.
  - **Semantic effect**: the commit-dependent success criteria (SC-001, SC-005–SC-008, SC-012, and the post-commit clause of SC-010) now carry a platform-applicability qualifier only. No criterion's meaning changed on supported platforms.
- **Final result (iteration 5)**: 16/16 items pass.
  - Markers: 0.
  - Requirements: 37 FRs and 14 SCs, all testable. FR-037 is covered by SC-014 and the new edge cases.
  - Implementation leakage: none (no flock, fsync, `os.Root`, or OS API names appear in the spec).
- **Post-checklist traceability note (2026-10-08; no checklist decision rewritten):** the later
  **platform-audit D-1** and **platform-audit D-2** identifiers are distinct from the Derived rules
  D-1 and D-2 approved on 2026-10-06. By human decision / human-approved scope amendment, the human
  authorized the corrected platform constraints, macOS/Linux-only persistence scope, H-7 as
  `PENDING — DEFERRED` and non-blocking for that scope, and preservation of FR-037/SC-014. Claude's
  technical PASS is independent evidence, not human authorization. The checklist remains 16/16;
  this note does not approve the Human Implementation Gate or accept renewed H-3 evidence.
- **Later H-3/D-2 acceptance record (2026-10-08; prior note preserved):** the first independent D-2
  evidence audit returned FAIL with BLK-1; after correction, the follow-up returned
  `D-2 EVIDENCE PASS` with no blockers. The responsible human subsequently approved D-2 and accepted
  the renewed H-3 macOS/Linux evidence, preserving NB-1 through NB-4 and the known limitations.
  Claude supplied technical review evidence and did not issue the human approval. H-7 remains
  `PENDING — DEFERRED`; the Human Implementation Gate remains `NO APROBADO`. Checklist markers
  remain unchanged at 16/16.
- **Post-acceptance snapshot clarification:** manifest `595b861b…` identifies the accepted snapshot
  before the documentation changes that recorded the human decision and the later audit
  corrections. Go production code and tests remain identical to that snapshot; the current
  documentation differs. This clarification changes no checklist marker or historical decision.
