# Feature Specification: SDD Workflow

**Feature Branch**: `feature/005-sdd-workflow`

**Created**: 2026-10-04

**Status**: Draft

**Input**: User description: "Feature 005 — SDD Workflow. Define the SDD workflow capability that conceptually connects canonical workflow states (004: draft, specified, planned, tasked, implemented, approved, abandoned), canonical development roles (003: specification, planning, implementation, review, remediation), role-based agent selection (003), agent execution (002), Spec Kit artifacts and workflow state/transition evaluation (004) into a deterministic state-based development process. Define the responsibility boundary first; do not assume 005 invokes all capabilities. Preserve Human Authority: agent_succeeded != workflow transition != human approval. Do not implement Human Gates (006) or Interactive CLI (007). Do not redefine 002/003/004 vocabulary, mappings, rules or record semantics; no second source of truth. No irreversible automatic orchestration, implicit persistence, retries, fallback, background execution. Separate 'what should happen next' from 'authorizing it to happen'. The key open decision — whether 005 only inspects/evaluates/recommends or also executes the selected agent — must be raised as an explicit clarification, not assumed. Specification only."

## Clarifications

### Session 2026-10-04

- Q: Execution boundary — is 005 only an inspect/evaluate/recommend capability, or may it also invoke role selection (003) or agent execution (002)? → A: Evaluate and recommend only. 005 determines and reports the SDD activity, and its development role, that corresponds to a feature's current Workflow State. It never invokes the role-selection capability (003), never discovers, lists, or executes any coding agent (002), and never applies a workflow transition (004/006). Composing the recommendation with 003 or 002 is always a separate, explicit request by the caller (human decision).
- Q: In Workflow State `planned` the next activity is producing the task list, but 003 defines no "tasks" role. Which development role is reported for it? → A: The existing `planning` role. 003's role vocabulary is not extended (human decision).
- Q: In Workflow State `implemented`, which activity is reported, given that 004 records no review result? → A: The review activity with the `review` role only. Remediation is never derived from Workflow State; it remains reachable only through an explicit 003 role-selection request. No review artifact is read (human decision).
- Q: Must 005 check consistency between the Workflow State and the Spec Kit artifacts in the feature directory? → A: Yes, existence only. Each Workflow State requires the Spec Kit artifacts of the phases it implies are complete; a missing required artifact produces a distinct inconsistency outcome with no recommendation. Artifacts present "ahead" of the Workflow State are permitted (work awaiting a human decision) and never advance the recommendation. Artifact content is not inspected, and nothing is created or repaired (human decision).
- Q: Which outcomes complete successfully, and with which exit codes? → A: `step_recommended` → 0; `workflow_terminal` → 0; `artifacts_inconsistent`, `feature_unknown`, `state_invalid` → 1; usage failure → 2. `workflow_terminal` is a successful evaluation: 005 evaluated the feature and determined that no next activity exists because the workflow has ended. It is not an evaluation error. Exit 0 means only that 005's evaluation completed successfully. It never implies human approval: `approved` and `abandoned` are both terminal and both exit 0, and recommendation, transition, and human approval remain separate concepts (human decision, Human Specification Gate pre-review).

### Session 2026-10-04 (Independent Specification Review remediation)

- Q (F-01): Should 005 report or maintain a prospective target Workflow State for the recommended activity? → A: No. 005 neither maintains nor reports any target or next Workflow State, and it does not copy, derive, evaluate, or report any 004 transition. 004 remains the sole authority over workflow transitions. 005 answers "which SDD activity corresponds to the current state", never "which state the feature should transition to" (human decision).
- Q (F-02): May a report contain the canonical token `approved`? → A: Yes, as the observed current Workflow State resolved under 004's semantics. Observing `approved` is not approval. What is prohibited is any field or text asserting that 005 granted, recorded, inferred, or authorized approval, or performed a Human Gate (human decision).
- Q (F-03): How are structurally invalid, traversal, absolute, nested, or symlinked feature identifiers handled? → A: Exactly as 004's state-reporting capability handles them: they produce `feature_unknown`, and evaluation stops before any artifact inspection. 005 constructs no artifact path and inspects no artifact until 004-style feature recognition and state resolution have succeeded (human decision).
- Q (F-04): Are artifact names case-sensitive? → A: Yes. Only the exact names `spec.md`, `plan.md`, and `tasks.md` satisfy the requirement. Case variants such as `Plan.md`, `SPEC.md`, or `Tasks.md` do not. This exact matching belongs to 005, and the artifact check's result MUST be the same whether the host filesystem is case-sensitive or case-insensitive. Feature recognition and Workflow State resolution inherit 004's approved behavior exactly. 005 makes no cross-host reproducibility promise for behavior that belongs to 004 (human decision).
- Q (F-05): What happens when the final report cannot be rendered, written, or delivered? → A: Exit code 2. This is an output failure, not a new domain outcome (human decision).
- Q (F-06): Which stream carries the normal report? → A: Standard output, following 003/004. Usage-failure and output-failure diagnostics go to standard error (human decision).

## Responsibility Boundary *(context for all requirements)*

"SDD Workflow" in `ai-dev` is the deterministic association between each non-terminal Workflow State (004) and the single Spec-Driven Development activity that corresponds to it. The activities are the steps of the constitution's mandated Specification → Plan → Tasks → Implementation sequence, plus review, which the constitution defines as a quality-gate activity, and which is the SDD activity associated here with the `implemented` state. This feature makes that association observable through one read-only capability, **next-step evaluation**. Next-step evaluation answers "which SDD activity corresponds to this feature's current Workflow State, and under which development role?"

It answers *what should happen next*. It never decides, grants, or records that it *may* happen, and it never states which Workflow State should follow. Authorizing a step, applying a transition, and recording human approval belong to Feature 006 — Human Gates. Transition rules belong solely to 004.

| Concern | Owner | 005's relationship |
|---|---|---|
| Coding agent identity, discovery, availability, execution, execution outcomes | 002 | None. 005 never discovers, lists, or executes agents and never reads execution outcomes. |
| Development-role vocabulary; role → agent mapping; role-selection outcomes | 003 | 005 reports a canonical role token *by reference*. It never maps a role to an agent, never reports an agent identity, and never invokes selection. |
| Workflow State vocabulary, terminal states, Transition Rule Set, Workflow State Record location and reading semantics, feature recognition | 004 | 005 obtains feature recognition and the current Workflow State exactly as 004's state-reporting capability defines them. 005 never copies, derives, evaluates, applies, or reports transitions, and never writes the record. |
| Spec Kit artifacts (`spec.md`, `plan.md`, `tasks.md`) | Spec Kit / humans | 005 checks only that the required artifacts are present. It never creates, edits, generates, or validates their content. |
| Current state → SDD activity → development role association; state/artifact consistency; next-step report | **005** | Owned here. |
| Authorization, approval, applying/persisting transitions, gate records | 006 | Out of scope. 005's output is never authorization. |
| Interactive/guided command experience, prompts, menus | 007 | Out of scope. 005 is a single non-interactive request/report. |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Find out the next SDD activity for a feature (Priority: P1)

A developer or an automated caller wants to know, for a given feature, which SDD activity corresponds to its current Workflow State and which development role that activity belongs to. They want this without anything being run, selected, applied, or approved.

**Why this priority**: This is the whole purpose of the feature. It joins the existing state model (004) and role vocabulary (003) into one deterministic answer about the development process, and it never crosses into execution, transitions, or authorization.

**Independent Test**: For recognized features in each non-terminal Workflow State, with their required artifacts present, request next-step evaluation. Confirm that each report names exactly the expected activity and role, and reports no other Workflow State than the current one. Confirm that no agent, selection, transition, or filesystem change occurs.

**Acceptance Scenarios**:

1. **Given** a recognized feature with no Workflow State Record (state `draft`), **When** the developer requests next-step evaluation, **Then** `ai-dev` reports outcome `step_recommended` with current state `draft`, activity `specify`, and role `specification`.
2. **Given** a recognized feature in state `specified` with `spec.md` present, **When** next-step evaluation is requested, **Then** the outcome is `step_recommended` with activity `plan` and role `planning`.
3. **Given** a recognized feature in state `planned` with `spec.md` and `plan.md` present, **When** next-step evaluation is requested, **Then** the outcome is `step_recommended` with activity `tasks` and role `planning`.
4. **Given** a recognized feature in state `tasked` with `spec.md`, `plan.md`, and `tasks.md` present, **When** next-step evaluation is requested, **Then** the outcome is `step_recommended` with activity `implement` and role `implementation`.
5. **Given** a recognized feature in state `implemented` with `spec.md`, `plan.md`, and `tasks.md` present, **When** next-step evaluation is requested, **Then** the outcome is `step_recommended` with activity `review` and role `review`.
6. **Given** any `step_recommended` report, **When** the developer reads it, **Then** the human-readable report states visibly that it is a recommendation that does not authorize the activity or any transition. Neither form of the report contains a field or text asserting that 005 granted, recorded, inferred, or authorized approval, or performed a Human Gate. Neither form contains a selected agent, a target or next Workflow State, or a transition result.
7. **Given** any `step_recommended` outcome, **When** the request completes, **Then** the report is written to standard output and the command completes successfully with exit code 0.
8. **Given** identical feature identifier, Workflow State Record content, and artifact directory entries, **When** the request is repeated, **Then** every reported value is identical.
9. **Given** a request in machine-readable mode, **When** it completes, **Then** standard output contains exactly one machine-readable report exposing the feature identifier, the outcome, and the fields required for that outcome (FR-016), and no human-readable text.
10. **Given** any completed request, **When** the developer inspects the repository and filesystem afterward, **Then** nothing was created, modified, or deleted. No coding agent was discovered, listed, or executed. No role selection was performed. No transition was evaluated or applied. No Workflow State Record was written.

---

### User Story 2 - Recognize that a feature's workflow has ended (Priority: P2)

A developer or automated caller asks for the next step for a feature whose Workflow State is terminal (`approved` or `abandoned`). They need a clear answer that there is no next SDD activity, not a guessed one.

**Why this priority**: A terminal state is a legitimate, frequent answer. Without a distinct outcome, a caller could misread a terminal feature as needing more work, or misread a recommendation as permission to reopen it.

**Independent Test**: Request next-step evaluation for an `approved` feature (with required artifacts) and for an `abandoned` feature. Confirm that each reports `workflow_terminal` with no activity or role.

**Acceptance Scenarios**:

1. **Given** a recognized feature in state `approved` with `spec.md`, `plan.md`, and `tasks.md` present, **When** next-step evaluation is requested, **Then** `ai-dev` reports outcome `workflow_terminal` with current state `approved`, and no activity or role.
2. **Given** a recognized feature in state `abandoned`, regardless of which artifacts are present, **When** next-step evaluation is requested, **Then** `ai-dev` reports outcome `workflow_terminal` with current state `abandoned`, and no activity or role.
3. **Given** a `workflow_terminal` outcome, **When** the request completes, **Then** the command reports successful completion (exit code 0) and never suggests reopening, re-specifying, or any other activity.
4. **Given** a feature in state `approved` and another in state `abandoned`, **When** next-step evaluation completes for each, **Then** both report `workflow_terminal` with exit code 0, and the exit code does not distinguish `approved` from `abandoned`. The `approved` report contains the canonical token `approved` only as the observed current state. Neither report contains a field or text asserting that 005 granted, recorded, inferred, or authorized approval, or performed a Human Gate.

---

### User Story 3 - Detect a state/artifact inconsistency or an undeterminable state (Priority: P3)

A developer or automated caller asks for the next step for a feature whose Workflow State claims progress that the feature directory does not support (for example, `planned` with no `plan.md`). Or the caller names a feature that is not recognized, or one whose Workflow State Record is invalid. They need a distinct, non-guessed answer instead of a recommendation built on an untrustworthy basis.

**Why this priority**: A recommendation is only trustworthy when the state it is based on is determinable and consistent with the artifacts the state implies. This story guards correctness. It comes after the primary and terminal paths.

**Independent Test**: Request next-step evaluation for (a) a feature in state `tasked` with `tasks.md` missing, (b) an unrecognized feature identifier, and (c) a recognized feature with an invalid Workflow State Record. Confirm that each produces its own distinct outcome and no recommendation.

**Acceptance Scenarios**:

1. **Given** a recognized feature in state `planned` with `spec.md` present and `plan.md` absent, **When** next-step evaluation is requested, **Then** `ai-dev` reports outcome `artifacts_inconsistent` with current state `planned` and missing artifacts `plan.md`, and no activity or role.
2. **Given** a recognized feature in state `implemented` with none of `spec.md`, `plan.md`, or `tasks.md` present, **When** next-step evaluation is requested, **Then** the outcome is `artifacts_inconsistent`, and the missing artifacts are listed in the fixed order `spec.md`, `plan.md`, `tasks.md`.
3. **Given** a recognized feature in state `approved` with `tasks.md` absent, **When** next-step evaluation is requested, **Then** the outcome is `artifacts_inconsistent`, not `workflow_terminal`.
4. **Given** a recognized feature in state `specified` whose directory also contains `plan.md` and `tasks.md`, **When** next-step evaluation is requested, **Then** the outcome is `step_recommended` with activity `plan`. The extra artifacts neither cause an inconsistency nor change the recommendation.
5. **Given** a recognized feature in state `planned` whose directory contains `spec.md` and `Plan.md` but no entry named exactly `plan.md`, **When** next-step evaluation is requested on either a case-sensitive or a case-insensitive host filesystem, **Then** the outcome is `artifacts_inconsistent` with missing artifacts `plan.md` on both hosts.
6. **Given** a feature identifier that 004 does not recognize as an existing feature, **When** next-step evaluation is requested, **Then** the outcome is `feature_unknown` with no current state, and no artifact path is constructed or inspected.
7. **Given** a structurally invalid feature identifier such as `..`, `../x`, `a/../../x`, `/etc/passwd`, or `a/b`, **When** next-step evaluation is requested, **Then** the outcome is `feature_unknown`, as 004's state-reporting capability defines, and no artifact path is constructed or inspected.
8. **Given** a feature identifier naming a symbolic link located where a feature directory would be, **When** next-step evaluation is requested, **Then** the outcome is `feature_unknown`, as 004's state-reporting capability defines. The link is not followed, and no artifact path is constructed or inspected.
9. **Given** a recognized feature whose Workflow State Record is `state_invalid` under 004's rules, **When** next-step evaluation is requested, **Then** the outcome is `state_invalid` with no current state, and no artifact path is constructed or inspected. The state never defaults to `draft`.
10. **Given** an `artifacts_inconsistent`, `feature_unknown`, or `state_invalid` outcome, **When** the request completes, **Then** the report never resembles `step_recommended`, it is written to standard output, and the command completes unsuccessfully with exit code 1. Nothing is repaired, created, retried, or escalated automatically.

---

### Edge Cases

- **No feature identifier, an empty or whitespace-only identifier, extra positional arguments, an unknown option, or an unsupported output-format value**: this is a usage failure (FR-019). No evaluation takes place, no report is emitted, standard output stays empty, a diagnostic goes to standard error, and the exit code is 2.
- **A structurally invalid identifier (`..`, `../x`, `a/../../x`, an absolute path such as `/etc/passwd`, or a nested path such as `a/b`)**: this is a recognized request with outcome `feature_unknown`, as 004's state-reporting capability defines. No filesystem path is constructed from it, and no artifact is inspected.
- **A symbolic link where a feature directory would be**: `feature_unknown`, as 004 defines. The link is not followed, and no artifact is inspected.
- **The final report cannot be rendered, written, or delivered** (for example, standard output is closed): this is an output failure (FR-020). It exits with code 2 and is never reported as one of the five outcomes. A diagnostic goes to standard error when possible. The outcome is never reclassified, and the write is never retried.
- **A caller reads exit code 0 as "there is a next activity" or as approval**: neither reading is valid. Exit code 0 means only that the evaluation completed successfully. Both `step_recommended` and `workflow_terminal` return 0, so callers MUST inspect the outcome to learn whether a next activity exists. Exit code 0 never means approval, authorization, or a transition.
- **A coding agent (002) reported `agent_succeeded` for work on a feature**: the result does not change. 005's outcome depends only on the Workflow State Record and artifact presence. Agent success is never treated as a transition, a state change, or approval.
- **An agent or a human created `plan.md` while the feature is still `specified`**: the recommendation stays `plan`. Artifact presence never changes the Workflow State or the recommendation. Only a change to the Workflow State Record does, and 005 never makes that change.
- **A required artifact name exists only with different letter case** (`Plan.md`, `SPEC.md`, `Tasks.md`): it does not satisfy the requirement on any host, including hosts whose filesystem resolves names case-insensitively.
- **The required artifact name exists but is not a regular file** (for example, a directory or a symbolic link): it counts as missing. Symbolic links are never followed.
- **A required artifact exists but is empty or malformed**: it counts as present. Content is never inspected.
- **A required artifact exists but cannot be inspected because of a filesystem or I/O error**: it counts as missing, which produces `artifacts_inconsistent`. Presence is never assumed.
- **Remediation is needed after a review found problems**: 005 never reports the `remediation` role. Its state model holds no review result. A human can request remediation selection explicitly through 003.
- **Abandonment**: 005 never recommends abandonment. Abandonment is solely a human decision.
- **The same request runs concurrently in several processes**: each request is independent. No locking or coordination is required.
- **The Workflow State Record or artifacts change between two requests**: each request reflects the content at the time it is made. Determinism applies only to unchanged inputs.
- **004 later adds a new Workflow State, or 003 adds a new role**: the existing Next-Step Table entries and their outcomes stay the same. A new non-terminal state without a Next-Step Table entry must be handled by an amendment to this feature. 005 never guesses an activity for it.
- **The caller expects the selected agent to be run**: 005 never selects or runs an agent. The caller can pass the reported role to 003 selection, and then the selected agent to 002 execution, each as a separate explicit request.
- **The caller wants to know which state follows**: 005 does not answer this. Transition validity is evaluated only by 004's transition-evaluation capability, through a separate explicit request.
- **The caller treats `step_recommended` as permission to proceed**: it is not permission. Authorization is defined by 006 Human Gates. A `step_recommended` report is valid input for a human decision and is never a substitute for one.

## Requirements *(mandatory)*

### Functional Requirements

**Capability and boundary**

- **FR-001**: The system MUST provide a next-step evaluation capability. It takes exactly one explicitly supplied feature identifier, with no other required argument, and reports the SDD activity, if any, that corresponds to that feature's current Workflow State.
- **FR-002**: The capability MUST be strictly evaluate-and-report. It MUST NOT invoke the role-selection capability (003). It MUST NOT discover, list, check availability of, launch, authenticate, or execute any coding agent (002). It MUST NOT create, record, or assert that 005 granted, recorded, inferred, or authorized any human approval or authorization. Observing and reporting the canonical current state `approved`, as resolved under 004 semantics, remains permitted (FR-016).
- **FR-003**: The capability MUST NOT copy, derive, evaluate, apply, persist, record, or report any workflow transition, and MUST NOT report any target or next Workflow State. 004 remains the sole authority over workflow transitions. The only Workflow State value a report may contain is the feature's current Workflow State.
- **FR-004**: The system MUST NOT create, modify, or delete any file, directory, Workflow State Record, Spec Kit artifact, or other repository or filesystem content as part of any next-step evaluation request, regardless of outcome. The system MUST NOT persist anything between requests.

**Feature recognition and state determination (reuse of 004)**

- **FR-005**: The system MUST determine feature recognition and the current Workflow State with exactly the observable semantics of 004's approved state-reporting capability. It MUST NOT define an alternative identifier rule, record location, reading rule, default, or normalization. In particular:
  - An identifier that is not exactly one safe directory name below the Spec Kit features directory MUST produce `feature_unknown`. This includes `..`, traversal identifiers such as `../x` or `a/../../x`, absolute paths such as `/etc/passwd`, and nested paths such as `a/b`.
  - A symbolic link in place of a feature directory MUST produce `feature_unknown` and MUST NOT be followed.
  - When 004 semantics yield `feature_unknown` or `state_invalid`, next-step evaluation MUST report that same outcome token with the same meaning.
- **FR-006**: The system MUST NOT construct any artifact path and MUST NOT inspect any artifact until feature recognition and Workflow State determination (FR-005) have both succeeded for the request.

**Next-step association (owned by 005)**

- **FR-007**: The system MUST maintain the following fixed, built-in, deterministic Next-Step Table. It has exactly one entry for each non-terminal Workflow State defined by 004:

  | Current Workflow State | SDD Activity | Development Role (003 token) |
  |---|---|---|
  | `draft` | `specify` | `specification` |
  | `specified` | `plan` | `planning` |
  | `planned` | `tasks` | `planning` |
  | `tasked` | `implement` | `implementation` |
  | `implemented` | `review` | `review` |

  Workflow States that 004 defines as terminal (004 FR-003; currently `approved` and `abandoned`) MUST have no entry, and therefore no activity and no role. 004 is the normative authority on terminality; this feature does not define it. The table associates a current state with an activity only. It contains no target state, transition, or ordering of states. The system MUST NOT provide any capability to view separately, edit, persist, override, score, or configure this table.
- **FR-008**: Every Development Role value in the Next-Step Table MUST be a canonical role token defined by 003, used by reference without redefinition. The Next-Step Table MUST NOT contain, imply, or report any coding agent identity, and MUST NOT replicate any part of 003's role → agent mapping. The `remediation` role MUST NOT appear in the Next-Step Table.
- **FR-009**: The system MUST NOT recommend abandonment, and MUST NOT report more than one activity per request.

**Artifact consistency (owned by 005)**

- **FR-010**: The system MUST apply the following fixed Required Artifacts rule. Only the presence of these files directly inside the feature's Spec Kit feature directory is checked:

  | Workflow State | Required Artifacts |
  |---|---|
  | `draft` | *(none)* |
  | `specified` | `spec.md` |
  | `planned` | `spec.md`, `plan.md` |
  | `tasked`, `implemented`, `approved` | `spec.md`, `plan.md`, `tasks.md` |
  | `abandoned` | *(none)* |

  A required artifact is present only if both of these hold:
  - The feature directory contains an entry whose name is exactly the canonical artifact name, compared case-sensitively.
  - That entry is a regular file.

  An entry whose name differs only in letter case (for example `Plan.md`, `SPEC.md`, or `Tasks.md`) MUST NOT satisfy the requirement. This holds regardless of whether the host filesystem resolves names case-sensitively or case-insensitively. For identical directory entries, the presence result MUST be identical on both kinds of host.

  The following count as missing:
  - a non-regular entry (for example, a directory or symbolic link);
  - an entry that cannot be inspected because of a filesystem or I/O error.

  Symbolic links MUST NOT be followed. Artifact content MUST NOT be read or validated.
- **FR-011**: When one or more Required Artifacts for the current Workflow State are missing, the system MUST report outcome `artifacts_inconsistent`. The report MUST include the current Workflow State and the complete list of missing artifacts, in the fixed order `spec.md`, `plan.md`, `tasks.md`, and MUST NOT report an activity or role.
- **FR-012**: Artifacts present beyond those required for the current Workflow State MUST NOT cause `artifacts_inconsistent`, and MUST NOT change the reported activity or role. Artifact presence MUST NOT be used to infer, advance, or suggest a Workflow State.

**Outcomes and precedence**

- **FR-013**: Every outcome reported for a recognized request MUST be exactly one of: `step_recommended`, `workflow_terminal`, `artifacts_inconsistent`, `feature_unknown`, or `state_invalid`. No failure or terminal outcome may be presented in a way that resembles `step_recommended`. `step_recommended` and `workflow_terminal` are successful evaluations. `artifacts_inconsistent`, `feature_unknown`, and `state_invalid` are evaluation failures. A successful evaluation never signifies human approval, authorization, or a transition.
- **FR-014**: Every invocation MUST be processed in this fixed order. Processing stops at the first step that produces a result, and no later step's check or data access is performed:
  1. **Usage validation** (FR-019). This step decides whether the invocation is a recognized request. If it is not, the result is a usage failure and no later step runs.
  2. **Feature recognition** (FR-005). An unrecognized feature produces `feature_unknown`.
  3. **Workflow State determination** (FR-005). An invalid state produces `state_invalid`.
  4. **Required Artifacts check** (FR-010, FR-011). Missing artifacts produce `artifacts_inconsistent`.
  5. **Terminal check**. A Workflow State that 004 defines as terminal (004 FR-003; currently `approved` and `abandoned`) produces `workflow_terminal`.
  6. **Next-Step Table lookup** (FR-007). This produces `step_recommended`.

  Steps 2–6 apply only to a recognized request. An output failure (FR-020) can occur only after an outcome has been determined, and it never changes that outcome.
- **FR-015**: For `step_recommended`, the report MUST include the feature identifier, the current Workflow State, and the SDD activity and development role from the Next-Step Table entry for that state. For `workflow_terminal`, the report MUST include the current Workflow State, and MUST NOT include any activity or role.

**Reporting**

- **FR-016**: The system MUST provide an explicitly selectable machine-readable report in addition to a human-readable presentation, consistent with 002–004. In machine-readable mode, standard output MUST contain exactly one machine-readable report and no human-readable text. The feature identifier (exactly as supplied) and the outcome are always present. The other fields follow this table:

  | Outcome | Current state | Activity | Role | Missing artifacts |
  |---|---|---|---|---|
  | `step_recommended` | REQUIRED | REQUIRED | REQUIRED | PROHIBITED |
  | `workflow_terminal` | REQUIRED | PROHIBITED | PROHIBITED | PROHIBITED |
  | `artifacts_inconsistent` | REQUIRED | PROHIBITED | PROHIBITED | REQUIRED (non-empty) |
  | `feature_unknown` | PROHIBITED | PROHIBITED | PROHIBITED | PROHIBITED |
  | `state_invalid` | PROHIBITED | PROHIBITED | PROHIBITED | PROHIBITED |

  No other field is defined. A report MAY contain the canonical token `approved` (or any other canonical Workflow State token), but only as the observed current state resolved under 004 semantics. No report, in either form, MAY contain:
  - a field or text asserting that 005 granted, recorded, inferred, or authorized approval;
  - a field or text asserting that 005 performed a Human Gate;
  - a field representing a coding agent identity, agent availability, an execution outcome, a selection outcome, a transition-evaluation result, or a target or next Workflow State.
- **FR-017**: The normal report for every recognized request MUST be written to standard output, in both human-readable and machine-readable mode, following 003/004. Each human-readable report MUST:
  - occupy exactly one line;
  - visibly display the exact outcome token;
  - render the feature identifier in the same escaped, quoted form 004 requires (004 FR-028);
  - for `step_recommended`, also state visibly that the result is a recommendation that does not authorize the activity or any transition.

  Usage-failure diagnostics (FR-019) and output-failure diagnostics (FR-020) MUST go to standard error and never to standard output.
- **FR-018**: The command MUST complete with exactly the following exit codes, consistent with the existing `ai-dev` 0/1/2 convention used by 001–004:

  | Result | Completion | Exit code |
  |---|---|---|
  | `step_recommended` | successful | 0 |
  | `workflow_terminal` | successful | 0 |
  | `artifacts_inconsistent` | unsuccessful | 1 |
  | `feature_unknown` | unsuccessful | 1 |
  | `state_invalid` | unsuccessful | 1 |
  | usage failure (FR-019) | unsuccessful | 2 |
  | output failure (FR-020) | unsuccessful | 2 |

  Exit code 0 means only that next-step evaluation completed successfully. It does not, by itself, mean that a next activity exists, because both `step_recommended` and `workflow_terminal` return 0. Callers MUST inspect the outcome to learn whether an activity was recommended. Exit code 0 MUST NOT be presented, documented, or interpreted by this feature as human approval, authorization to perform an activity, or a workflow transition. The exit code MUST NOT distinguish `approved` from `abandoned`.

**Invalid input and output failure**

- **FR-019**: These conditions are usage failures outside the outcome model:
  - no feature identifier;
  - an empty or whitespace-only feature identifier;
  - extra positional arguments;
  - an unrecognized option;
  - an unsupported output-format value;
  - any syntax that cannot be parsed into a recognized request.

  A structurally invalid but non-empty, non-whitespace feature identifier is not a usage failure (see FR-005). For a usage failure, the system MUST report the problem on standard error and leave standard output empty. It MUST NOT perform any evaluation, read any Workflow State Record, or construct or inspect any artifact path. It MUST complete unsuccessfully with exit code 2.
- **FR-020**: When an outcome has been determined but the final report cannot be rendered, written, or delivered, the system MUST treat this as an output failure. This is not a domain outcome. The system MUST NOT report a sixth outcome, reclassify the determined outcome, retry the write, or fall back to another output. It MUST report the problem on standard error when possible and MUST complete unsuccessfully with exit code 2. On an output failure, the contents of standard output are not guaranteed; callers MUST rely on the exit code.
- **FR-021**: The capability MUST NOT read standard input, prompt, or wait for interactive input at any point.

**Determinism, authority, security, compatibility**

- **FR-022**: For identical feature identifier, Workflow State Record content (or its confirmed absence), and feature-directory entries, every reported value MUST be identical across repeated requests. The Required Artifacts check (FR-010) MUST also give identical results across hosts whose filesystems differ only in name case-sensitivity. Feature recognition and Workflow State determination are inherited unchanged from 004 (FR-005), so this feature imposes no additional cross-host requirement on them. The system MUST NOT use scoring, ranking, randomness, prompt-based inference, learning, or any other nondeterministic mechanism.
- **FR-023**: None of the following MAY be treated by this feature as, or reported as equivalent to, a workflow transition, a change of Workflow State, Human Gate approval, or authorization to perform any activity:
  - a coding-agent outcome (002);
  - a role-selection outcome (003);
  - artifact presence;
  - an observed `approved` state;
  - a `step_recommended` report;
  - exit code 0.
- **FR-024**: The system MUST require a separate, explicit request for next-step evaluation. It MUST NOT perform next-step evaluation as a side effect of any other capability. It MUST NOT trigger any other capability as a side effect of next-step evaluation, and MUST NOT schedule, chain, retry, fall back, or continue any work in the background.
- **FR-025**: The system MUST NOT emit any credential, token, secret, or sensitive environment value in any output.
- **FR-026**: This feature MUST NOT change the approved observable behavior of 001's environment doctor, 002's listing and execution, 003's role selection, or 004's state reporting and transition evaluation. It MUST NOT redefine their vocabularies, mappings, rules, outcomes, or record semantics.
- **FR-027**: Adding a coding agent (002) MUST NOT change any outcome or reported value of this feature. Adding a development role (003) or a Workflow State value (004) MUST NOT change the outcome or reported values for any already-defined Next-Step Table entry.

### Key Entities

- **SDD Activity**: One of five fixed tokens naming a Spec-Driven Development activity: `specify`, `plan`, `tasks`, `implement`, `review`. The first four are the steps of the constitution's Specification → Plan → Tasks → Implementation sequence. `review` is a quality-gate activity. These tokens are domain values owned by 005, independent of any coding agent. They correspond to the Spec Kit phases by name only, and 005 never invokes those phases.
- **Next-Step Table**: The fixed association (FR-007) from each non-terminal Workflow State to exactly one SDD Activity and one 003 Development Role, used by reference. It holds no target state and no transition knowledge.
- **Required Artifacts Rule**: The fixed association (FR-010) from each Workflow State to the Spec Kit artifacts, with exact case-sensitive names, whose presence that state implies.
- **Next-Step Evaluation Request**: One explicit, non-interactive request naming exactly one feature identifier, with an optional output-format choice.
- **Next-Step Report**: The result of one request: the feature identifier, exactly one outcome, and the outcome-dependent fields defined in FR-016, in human-readable and machine-readable form, written to standard output. It is a recommendation or diagnosis only, never an authorization.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With a single request, and without running, selecting, applying, or approving anything, a developer can learn the SDD activity and development role that correspond to the current state of any recognized feature in a consistent non-terminal state.
- **SC-002**: For fixtures covering all five non-terminal Workflow States with required artifacts present, 100% of requests report `step_recommended` with the exact activity and role defined in FR-007.
- **SC-003**: 100% of Next-Step Table entries use a canonical 004 non-terminal state as key and a canonical 003 role token as role. Zero entries reference a coding agent identity, the `remediation` role, or any target or next Workflow State. Across all fixtures, 0% of reports contain any Workflow State value other than the current state.
- **SC-004**: For fixtures covering every Workflow State that has Required Artifacts, each with every non-empty combination of missing required artifacts, 100% of requests report `artifacts_inconsistent` with exactly the missing artifacts in the fixed order. 0% of them report an activity.
- **SC-005**: For fixtures in which artifacts beyond the required set are present, 100% of requests report the same result as the corresponding fixture without those artifacts.
- **SC-006**: 100% of requests for `approved` (with required artifacts) and `abandoned` features report `workflow_terminal` with no activity or role.
- **SC-007**: The following produce the stated outcome in 100% of requests, and in every case zero artifact paths are constructed or inspected:
  - unrecognized features → `feature_unknown`;
  - the structurally invalid identifiers `..`, `../x`, `a/../../x`, `/etc/passwd`, and `a/b` → `feature_unknown`;
  - a symbolic link in place of a feature directory → `feature_unknown`;
  - features whose record 004 classifies as invalid → `state_invalid`.
- **SC-008**: 0% of requests, regardless of outcome, do any of the following: create, modify, or delete filesystem content; read standard input; discover or execute a coding agent; invoke role selection; evaluate or apply a transition.
- **SC-009**: 100% of repeated requests with unchanged inputs produce identical outcomes and reported values.
- **SC-010**: Across fixtures for every outcome, for usage failures, and for output failures, 100% of requests complete with the exit code FR-018 defines:
  - 0 for `step_recommended` and `workflow_terminal`;
  - 1 for `artifacts_inconsistent`, `feature_unknown`, and `state_invalid`;
  - 2 for usage and output failures.

  0% of reports, including `workflow_terminal` reports for `approved`, contain a field or text asserting that 005 granted, recorded, inferred, or authorized approval, or performed a Human Gate. The canonical `approved` token appearing as the observed current state does not violate this criterion.
- **SC-011**: From the machine-readable report alone, an automated caller can determine the feature identifier, the outcome, and every field FR-016 requires for that outcome. 0% of reports contain a field that FR-016 prohibits. 100% of normal reports are written to standard output. 100% of usage-failure and output-failure diagnostics are written to standard error, with an empty standard output for usage failures.
- **SC-012**: Consider fixtures in which a required artifact is present only under a case variant of its name (`Spec.md`, `PLAN.md`, `Tasks.md`). On both case-sensitive and case-insensitive host filesystems, 100% of these requests report `artifacts_inconsistent` listing the canonical name as missing, and the results are identical across the two hosts.
- **SC-013**: 100% of the existing acceptance scenarios of 001, 002, 003, and 004 remain unchanged in observable behavior once this feature is present.

## Assumptions

- The clarifications recorded above are explicit human decisions made during specification and during remediation of the Independent Specification Review.
- Feature identifier, feature recognition (including the rejection of structurally invalid identifiers and symlinked feature directories), Workflow State Record location, and record-reading semantics are exactly those of 004's approved state-reporting capability. A feature with no record is `draft`. Because no mechanism yet writes Workflow State Records (004 FR-027), every existing feature currently evaluates as `draft`. This is expected until a later feature (planned as 006) introduces a write path.
- Spec Kit artifact names are exactly `spec.md`, `plan.md`, and `tasks.md`, located directly inside the feature's Spec Kit feature directory, consistent with features 001–004. Name matching is case-sensitive by human decision. How this is achieved on case-insensitive hosts is a plan concern; only the observable result is specified here. This case-sensitivity applies only to 005's artifact names. Under 004's approved behavior, a feature identifier and the Workflow State Record name are resolved through the host filesystem's own lookup. 005 inherits that behavior unchanged and does not redefine it (FR-005, FR-026).
- 005 deliberately differs from the 003/004 pattern of "success only for the single positive outcome": `workflow_terminal` is also a successful evaluation (human decision). This changes no behavior of 003 or 004.
- Treating an output failure as exit code 2 matches the existing `ai-dev` convention for report-write failures in 004, and introduces no new numeric code.
- Remediation, re-opening terminal features, review-result tracking, multi-activity recommendations, abandonment recommendations, and any reporting of target or next states are out of scope.
- 006 (Human Gates) will own authorizing activities, applying and persisting transitions, and recording approvals. 007 (Interactive CLI) will own any guided or interactive experience. Either may consume the Next-Step Report as information, but this feature defines no hook, contract, or dependency on them.
- Command syntax (conceptually similar to `ai-dev workflow next <feature>`), programming language, and machine-readable serialization are plan concerns.
