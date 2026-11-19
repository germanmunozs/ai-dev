# Feature Specification: Human Gates

**Feature Branch**: `feature/006-human-gates`

**Created**: 2026-10-05

**Status**: Draft

**Input**: User description: "Feature 006 — Human Gates. Introduce the smallest useful Human Gate capability: explicitly recording a human gate decision and, when authorized by that decision, applying and persisting the corresponding workflow-state transition defined by 004. Core invariant: agent_succeeded != workflow transition != human approval. An agent result, artifact existence, SDD recommendation, role selection, or valid transition must never by itself constitute human approval or authorization. The capability must be human-initiated and must never infer approval from previous commands, agent success, generated artifacts, current workflow state, or any other automated signal. 006 must not create a second source of truth for states, transition rules, roles, agent mappings, next-step rules, or artifact requirements owned by 002–005; must not invoke Claude Code, Codex, role selection, agent execution, or workflow-next; no orchestration, retries, fallback, multi-agent conversations, background execution, implicit approval, or interactive prompts. Preserve 001–005 observable behavior. Specification only."

## Clarifications

### Session 2026-10-06 (Human clarification decisions C-1 – C-12)

- Q (C-1): What does a Human Gate authorize? → A: A Human Gate is a **post-activity acceptance decision**. It records the human's explicit acceptance of a completed SDD activity and authorizes exactly one corresponding Workflow State transition. It does not authorize, trigger, or imply permission to execute any activity (human decision).
- Q (C-2): Is the target state explicit or derived? → A: Every request MUST explicitly supply the intended canonical target Workflow State. 006 never derives, guesses, recommends, or substitutes a target. Transition validity is governed exclusively by 004 (human decision).
- Q (C-3): How is abandonment authorized? → A: Through a separate Human Gate decision type, `abandonment`, distinct from `approval`. It explicitly targets `abandoned`, MUST include a non-empty human-supplied reason, and the transition MUST be valid under 004 (human decision).
- Q (C-4): Can approval reach canonical `approved`? → A: Yes. An explicit `approval` from `implemented` may target `approved`, subject to 004 validation, through the same approval mechanism as every other forward transition. There is no separate final-approval gate type. A reason is optional for `approval` and mandatory for `abandonment` (human decision).
- Q (C-5): Is every transition gated, and is 006 the only write path? → A: Every Workflow State transition persisted by ai-dev requires an explicit Human Gate decision. 006 is ai-dev's sole workflow-state write path; 002, 003, 004, 005, and every read-only capability never persist a transition. This does not claim to prevent manual edits to repository files made outside ai-dev (human decision).
- Q (C-6): What evidence is retained? → A: Every successfully applied transition retains durable Human Gate evidence, consisting of:
  - the feature identity;
  - the decision type;
  - the explicitly supplied approver identity;
  - the actual from-state;
  - the explicit target (to-state);
  - the decision timestamp;
  - the reason, when supplied.

  Later successful decisions never erase earlier evidence. This specification defines the observable evidence contract only, not its physical storage representation, and preserves compatibility with 004's Workflow State Record semantics (human decision).
- Q (C-7): What atomicity and stale-state guarantees apply? → A: The transition and its evidence form one logical, all-or-nothing persistence operation. Before committing, 006 ensures the authoritative Workflow State has not changed from the state the request was evaluated against. If it changed, 006 reports `state_conflict` and persists nothing. A persistence failure is never reported as success and leaves no partially committed state/evidence pair. A report or output failure after a successful commit does not roll back or invalidate the committed decision. The mechanism is a planning concern (human decision).
- Q (C-8): How are replayed requests handled? → A: With no special treatment. 006 resolves the current state and delegates validity to 004 exactly as for a new request. A target that is no longer valid yields 004's `transition_invalid`, and nothing is persisted. There is no idempotent success and no `already_applied` outcome (human decision).
- Q (C-9): Are denials persisted? → A: No. Only `approval` and `abandonment` exist as decision types. A human who does not accept the work simply submits no approval. System refusals and failures are reported only; they never create evidence or mutate state. There is no `deny` or `rejected` decision type or state (human decision).
- Q (C-10): How does 006 relate to 005's artifact consistency? → A: For `approval`, artifact consistency for the explicit target state is a precondition. It uses 005's authoritative Required Artifacts Rule by reference, without copying it and without invoking `workflow next`. Missing required artifacts produce `artifacts_missing` and no mutation. `abandonment` requires no completion artifacts (human decision).
- Q (C-11): Which outcomes and exit codes apply? → A: The outcomes are `transition_applied`, `feature_unknown`, `state_invalid`, `target_unsupported`, `transition_invalid`, `artifacts_missing`, `state_conflict`, and `persistence_failed`, with 004's reused semantics preserved exactly. The exit codes are:
  - exit 0 only when this invocation persisted both the transition and its evidence;
  - exit 1 for domain refusals and failures;
  - exit 2 for usage failures and report/output failures. A post-commit output failure may therefore exit 2 even though the decision was durably applied (human decision).
- Q (C-12): What is the human identity and trust boundary? → A: Every request is a distinct explicit operation and MUST include a non-empty, human-supplied approver identity, which is persisted as evidence.
  - ai-dev does not authenticate the approver, and does not prove that the approver is a human or that the process was launched by that person.
  - The approver identity is an auditable assertion, not an authentication claim.
  - No automated signal may supply or imply approval.
  - Strong authentication, and preventing an external coding agent from deliberately invoking the command, are out of scope and are documented as a residual trust boundary.
  - 006 never claims stronger verification than it provides (human decision).

### Session 2026-10-06 (Human clarification decision C-13 and confirmation of derived rules)

- Q (C-13): How is a decision type / target mismatch classified? → A: As a **usage failure (exit 2)**. Decision-type consistency is part of request and usage validation, and it runs before target canonicality. The rules are:
  1. An `abandonment` MUST have target exactly `abandoned`. Any other supplied target is a usage failure, including case or other non-exact variants such as `Abandoned`. It is never reported as `target_unsupported` or `transition_invalid`.
  2. An `approval` MUST NOT target `abandoned`. An `approval` whose target is exactly `abandoned` is a usage failure. Abandoning a feature requires the distinct `abandonment` decision type.
  3. After decision-type consistency succeeds, normal target canonicality applies. A target that reaches canonical-target evaluation and is not one of 004's exact canonical values reports `target_unsupported` (exit 1), with 004's meaning unchanged.

  (human decision)
- Q (Derived rule D-1): How is a supplied but empty or whitespace-only reason treated? → A: As a usage failure, for either decision type. A non-empty reason remains mandatory for `abandonment` and optional for `approval` (human decision).
- Q (Derived rule D-2): What happens if existing Human Gate evidence cannot be preserved as C-6 requires? → A: Report `persistence_failed` and persist nothing (human decision).
- Q (Derived rule D-3): What does the artifact check, and C-7's stale-state protection, cover? → A: Artifact consistency reflects the required artifacts that exist when the artifact check runs. C-7's stale-state protection covers the authoritative Workflow State specifically. It is not a filesystem-wide concurrency guarantee (human decision).

### Session 2026-10-07 (Product clarification P-6, raised at the Human Plan Gate)

- Q (P-6): On which platforms and filesystems is Human Gate persistence supported? → A: On **macOS and Linux local filesystems** where the required locking and atomic-replacement primitives are available. On unsupported platforms, including Windows, the Human Gate commands may perform the already-defined validation and evaluation steps, but before any persistence they MUST fail closed with the existing `persistence_failed` outcome (exit 1), with no Workflow State or Human Gate evidence mutation. Feature 006 does not claim Windows Human Gate persistence support. Cross-machine or distributed coordination on network filesystems is outside Feature 006's scope. A filesystem on which the required locking or persistence primitives cannot be established safely MUST fail closed rather than silently weaken the Human Gate guarantees. Windows persistence support and distributed or network-filesystem coordination require future, explicitly scoped work (human decision).

### Session 2026-10-08 (Platform-audit amendment D-1)

- **Authority record (2026-10-08): human decision / human-approved scope amendment.** The human
  responsible for Feature 006 expressly authorized the platform-audit amendment D-1, macOS and
  Linux as the only currently supported persistence platforms, H-7 as `PENDING — DEFERRED` and
  non-blocking for the macOS/Linux scope, and preservation of the complete fail-closed guarantees
  in FR-037 and SC-014. This authorization is distinct from the **Derived rule D-1** recorded on
  2026-10-06; neither decision is rewritten. Claude's technical PASS for the corrected build
  constraints is independent review evidence, not the source of this human authorization. This
  scope amendment does not approve the Human Implementation Gate, authorize merge, or accept
  renewed H-3 evidence.
- Q (platform-audit D-1): Does the macOS/Linux persistence scope include iOS or Android through Go's inherited
  build tags? → A: **No.** macOS and Linux are the only currently supported persistence platforms.
  iOS, Android, Windows, and every other platform use the fail-closed unsupported-platform path.
  The implementation constraints are `(darwin && !ios) || (linux && !android)` for the POSIX store
  and `!((darwin && !ios) || (linux && !android))` for its fallback. This correction does not weaken
  FR-037 or SC-014. Native Windows validation is deferred, not completed or approved; reactivating
  Windows persistence support requires separately scoped work and native validation (human
  decision / human-approved scope amendment, 2026-10-08).

### Session 2026-10-08 (Human acceptance of platform-audit D-2 / renewed H-3 evidence)

- **Authority record: human decision.** After the initial independent D-2 evidence review returned
  FAIL with BLK-1, the evidence was corrected and the follow-up independent audit returned
  **D-2 EVIDENCE PASS**, with BLK-1 resolved and no blocking findings. The responsible human then
  explicitly approved platform-audit D-2 and accepted the renewed H-3 evidence for the evidenced
  macOS/Linux source tree, with the known limitations and non-blocking observations NB-1 through
  NB-4.
  The accepted external package is
  `/Users/german/ai-dev-evidence/feature-006/h3-linux-d2-renewal-2026-10-08/`; the SHA-256 of its
  `SHA256SUMS` is `696e623eb98a67058644bf4935de363acf91b20a3382f7073c6cf21c42616b03`,
  and the byte-identical macOS/Linux source-manifest identity is
  `595b861b041b8f93b10479432206a8a69ba50d5bd8778bcb2ffd1cb6121aa21f`.
- The `595b861b…` manifest identifies the accepted snapshot before the documentation changes that
  recorded this human decision and the later audit corrections. Go production code and tests remain
  identical to that snapshot; the current documentation differs.
- This human acceptance is limited to D-2 and renewed H-3 evidence for the supported macOS/Linux
  scope. H-7 remains `PENDING — DEFERRED`; no native Windows validation is claimed. The Human
  Implementation Gate remains `NO APROBADO`; no merge, PR, release, or other Human Gate is approved
  by inference.

### Session 2026-10-08 (Human Implementation Gate approval)

- **Authority record: human decision.** After reviewing the formal gate criteria and the previously
  accepted H-3/D-2 evidence, the responsible human for Feature 006 explicitly approved the
  implementation for the supported macOS/Linux scope.
- **Human Implementation Gate: `APPROVED — macOS/Linux scope`.** H-7 remains
  `PENDING — DEFERRED`, native Windows behavior remains unvalidated, and no complete
  multiplatform validation is claimed. FR-037 and SC-014 retain their full fail-closed contracts
  for Windows and every other unsupported platform. This approval does not approve H-7 and does
  not by itself authorize a PR, merge, release, or another Human Gate.

### Session 2026-11-19 (Feature 007 cross-feature Amendment 4)

- **Authority record (2026-11-19): human decision / human-approved cross-feature amendment.** German Munoz, repository owner, approved Amendments 1–4, of which this is Amendment 4 (cross-references) of Feature 007 (`specs/007-workflow-orchestration/spec.md`, section Required Amendments) in the words "si, apruebo las enmiendas 1 a 4, aplícalas". The approver identity is a declared, auditable assertion, not an authenticated identity. The amendment adds a permission for an explicit Feature 007 guidance request only; it changes no observable behavior of this feature and approves no Feature 007 implementation, plan or autonomous capability.

## Responsibility Boundary *(context for all requirements)*

A **Human Gate** is the point at which ai-dev requires an explicit human acceptance decision before
it changes a feature's persisted Workflow State. This feature adds ai-dev's only Workflow State
write path, and that path can be reached only through such a decision.

| Concern | Owner | 006's relationship |
|---|---|---|
| Coding-agent identity, discovery, availability, execution, and execution outcomes (`agent_succeeded`, …) | 002 | None. 006 never discovers, lists, or executes an agent, and never reads or consumes an execution outcome. |
| Development roles, the role → agent mapping, role-selection outcomes | 003 | None. 006 never selects a role or reports an agent. |
| Workflow State vocabulary, terminality, Transition Rule Set, Workflow State Record location and reading semantics, feature recognition, transition-evaluation precedence | 004 | Used by reference. 006 determines recognition, the current state, and transition validity with exactly 004's semantics, and persists state that 004's unchanged reader resolves to the authorized target. It never copies, restates, extends, or overrides 004's rules. |
| Required Artifacts Rule, Next-Step Table, SDD activities, next-step report | 005 | 006 uses only the Required Artifacts Rule, by reference, as an `approval` precondition for the target state. It never invokes next-step evaluation and never consults the Next-Step Table. |
| **Recording a human acceptance or abandonment decision; persisting the one transition it authorizes; retaining Human Gate evidence** | **006** | Owned here. |
| Interactive or guided command experience, prompts, menus | 007 Workflow Orchestration (guided report; autonomous mode deferred) | Out of scope. 006 is a single non-interactive request and report. |

### Six distinct concepts

These concepts are deliberately separate. None of them implies another.

| Concept | Question it answers | Owner | Mutates anything? |
|---|---|---|---|
| Observing state | "What is the feature's current Workflow State?" | 004 state reporting | No |
| Evaluating transition validity | "Do the rules permit moving from the current state to X?" | 004 transition evaluation | No |
| Recommending an activity | "Which SDD activity corresponds to the current state?" | 005 next-step evaluation | No |
| **Authorizing** | "Has a human explicitly accepted the completed work, or decided to abandon, for exactly this transition?" | **006**, only from the explicit Human Gate Request | No, by itself |
| **Persisting a transition** | "Change the feature's authoritative Workflow State." | **006**, only when authorized and valid | Yes |
| **Recording approval evidence** | "Who asserted which decision, from which state to which state, when, and why?" | **006**, in the same all-or-nothing operation as persisting | Yes |

Core invariant: **`agent_succeeded` ≠ `transition_valid` ≠ `step_recommended` ≠ human acceptance.**
Only a Human Gate decision can cause persistence, and only together with 004 validity (and, for
`approval`, 005 target-artifact consistency).

### Residual trust boundary

ai-dev **does not authenticate** the approver. The approver identity is an **auditable assertion**
supplied by whoever invokes the command, recorded verbatim, and reviewable by humans through
version control. ai-dev cannot distinguish a human from a coding agent or script that deliberately
invokes the Human Gate command, and it does not attempt to.

The controls that remain are:
- the command is distinct and explicit, and nothing in ai-dev invokes it;
- coding agents launched through 002 keep their native permission prompts (002 FR-016);
- the evidence is version-controllable and is reviewed by humans, for example in a pull request.

Strong authentication, and preventing an external agent from invoking the command, are outside 006's
scope.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Accept completed work and advance the feature by one step (Priority: P1)

A human developer has reviewed the outcome of a feature's completed SDD activity (for example its
specification) and records their explicit acceptance, naming the target Workflow State and their own
identity. The feature's Workflow State advances to that target, and durable evidence of the decision
is retained.

**Why this priority**: This is the entire purpose of the feature. Without it, every feature reads as
`draft` forever (004 FR-005, FR-027), and human acceptance cannot be recorded through ai-dev in an
auditable way.

**Independent Test**: For a recognized feature in each non-terminal state, with the target state's
required artifacts present, submit an `approval` naming the target that 004 permits. Confirm all of
the following:
- 004's unchanged state reporting then reports the target;
- the evidence contains every C-6 field;
- no agent, role selection, or next-step evaluation was invoked.

**Acceptance Scenarios**:

1. **Given** a recognized feature with no Workflow State Record (state `draft`) and `spec.md` present, **When** a human submits an `approval` with target `specified` and approver identity `alice`, **Then** ai-dev reports `transition_applied` with from-state `draft`, target `specified`, and approver `alice`, and exits 0. After that, 004's state reporting reports `state_reported` with state `specified`.
2. **Given** a recognized feature in state `specified`, `planned`, or `tasked`, with the artifacts that 005's Required Artifacts Rule requires for the target, **When** a human submits an `approval` naming a target that 004 permits from that state, **Then** the outcome is `transition_applied`, and 004 then reports that target.
3. **Given** a recognized feature in state `implemented` with the artifacts 005 requires for `approved`, **When** a human submits an `approval` with target `approved`, **Then** the outcome is `transition_applied`. 004 then reports `approved` with terminal `true`, and 005 reports `workflow_terminal`.
4. **Given** a `transition_applied` outcome, **When** the Human Gate evidence is inspected, **Then** it contains the feature identity, decision type `approval`, the approver identity exactly as supplied, the actual from-state, the explicit target, the decision timestamp, and the reason if one was supplied.
5. **Given** a feature that has advanced through several `transition_applied` decisions, **When** the evidence is inspected, **Then** the evidence of every earlier decision is still present and unaltered.
6. **Given** a `transition_applied` outcome, **When** the repository is inspected, **Then** the only content that changed belongs to that feature's authoritative Workflow State and Human Gate evidence. `spec.md`, `plan.md`, `tasks.md`, other features, and everything outside the feature's Spec Kit feature directory are unchanged, and nothing was staged or committed.
7. **Given** a request in machine-readable mode, **When** it completes, **Then** standard output contains exactly one machine-readable report with the feature identifier, the outcome, and the fields FR-027 requires, and no human-readable text.
8. **Given** a `transition_applied` outcome in human-readable mode, **When** the human reads it, **Then** it is a single line that shows the exact outcome token and the quoted feature identifier and approver. It presents the approver as asserted, never as authenticated or verified.

---

### User Story 2 - No automated signal can stand in for human acceptance (Priority: P1)

A developer runs a coding agent that reports `agent_succeeded`, requests a role selection, receives
`step_recommended` from 005 or `transition_valid` from 004, or sees an agent generate `plan.md`. They
need certainty that none of this approved anything or changed the feature's Workflow State.

**Why this priority**: This is the core invariant of the feature and of the constitution's Human
Authority principle. If it is violated, the feature does harm.

**Independent Test**: Snapshot a feature's Workflow State and evidence. Run every capability of
001–005 against the feature, including a successful agent execution that creates artifacts. Confirm
that the state and evidence are unchanged.

**Acceptance Scenarios**:

1. **Given** a feature in state `specified`, **When** a coding agent executed through 002 reports `agent_succeeded`, **Then** the Workflow State and evidence are unchanged, and 004 still reports `specified`.
2. **Given** a feature in state `specified`, **When** 005 reports `step_recommended` and 004 reports `transition_valid` for target `planned`, **Then** no Human Gate evidence is created and no transition is persisted.
3. **Given** a feature in state `specified` whose directory gains `plan.md` and `tasks.md`, **When** any capability other than the Human Gate capability runs, **Then** the Workflow State stays `specified`.
4. **Given** an `approval` from `draft` to `specified` that was already applied, **When** the identical request is submitted again, **Then** 006 resolves the current state `specified`, and 004 deems `specified` → `specified` invalid. The outcome is `transition_invalid` with exit 1, and no state or evidence is persisted. No idempotent success is reported.
5. **Given** any capability of 001–005, **When** it is invoked with any input, **Then** it never persists a transition or creates Human Gate evidence, and its approved observable behavior is unchanged.

---

### User Story 3 - Refuse invalid gate requests without changing anything (Priority: P2)

A human submits a gate request that cannot be honored: an unknown feature, an invalid recorded state,
a non-canonical target, a transition 004 does not permit, a terminal feature, or missing target
artifacts. The human needs a distinct non-success answer, and the guarantee that nothing was written,
repaired, or guessed.

**Why this priority**: A gate is only trustworthy if every refusal leaves the repository exactly as
it was.

**Independent Test**: Submit each class of invalid request against fixtures. Confirm the distinct
outcome, exit code 1 (or 2 for usage failures), and byte-identical repository content.

**Acceptance Scenarios**:

1. **Given** a feature identifier that 004 does not recognize, including `..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`, or a symbolic link in place of a feature directory, **When** an `approval` with a canonical target is submitted, **Then** the outcome is `feature_unknown`. No directory, state, or evidence is created, and no link is followed.
2. **Given** a recognized feature whose Workflow State Record 004 classifies as invalid, **When** a request with any canonical target is submitted, **Then** the outcome is `state_invalid`. The record is not repaired, overwritten, or defaulted.
3. **Given** a feature in state `specified`, **When** an `approval` names target `tasked` (a skip), `draft` (backward), or `specified` (the same state), **Then** the outcome is `transition_invalid`, and nothing is written.
4. **Given** an `approval` whose target is not one of 004's canonical values (for example `Planned` or `reviewing`), **When** it is submitted, **Then** the outcome is `target_unsupported` under 004's precedence. Feature existence is not checked, and no record is read.
5. **Given** a feature in terminal state `approved` or `abandoned`, **When** an `approval` or `abandonment` is submitted, **Then** the outcome is `transition_invalid`, and the feature is not reopened.
6. **Given** a feature in state `planned` with `spec.md` present and `plan.md` absent, **When** an `approval` with target `tasked` is submitted, **Then** the outcome is `artifacts_missing`, the missing artifacts are listed in 005's fixed order, and nothing is written.
7. **Given** a feature in state `specified` with `spec.md` and `plan.md` absent, **When** an `approval` with target `tasked` (invalid under 004) is submitted, **Then** the outcome is `transition_invalid`, not `artifacts_missing`, because 004 validity is evaluated first.
8. **Given** any refusal in this story, **When** it completes, **Then** the report never resembles `transition_applied`, says that no transition was applied, and leaves the repository byte-identical. No Human Gate evidence is created.

---

### User Story 4 - Truthful outcome when state is stale or persistence fails (Priority: P2)

A human submits a valid, authorized request. Before it commits, however, the feature's state changes
(through another request or a manual edit), or the persistence operation fails. The human needs to
know truthfully that the decision did not take effect, and the state/evidence pair must never be left
partially committed.

**Why this priority**: A write path that can misreport its own result undermines every later reading
of Workflow State.

**Independent Test**: Inject a state change between evaluation and commit, and inject persistence
failures at every point. Confirm the outcome, the exit code, and that the state/evidence pair is
either the complete prior pair or the complete new pair.

**Acceptance Scenarios**:

1. **Given** a request evaluated against state `specified`, **When** the authoritative state changes before commit, **Then** the outcome is `state_conflict` with exit 1, and no state or evidence is persisted.
2. **Given** two concurrent valid requests from the same from-state, **When** both complete, **Then** at most one reports `transition_applied`, and every other reports `state_conflict` and persists nothing.
3. **Given** a valid request whose persistence operation fails, **When** it completes, **Then** the outcome is `persistence_failed` with exit 1. The report says no transition was applied, and 004 still reports the original state.
4. **Given** a persistence operation interrupted at any point, **When** the feature is later read, **Then** 004 reports either the complete prior state, with no evidence of this request, or the complete new state with its evidence. It never reports a state without its evidence, evidence without its state, or `state_invalid` caused by the interruption.
5. **Given** a `transition_applied` outcome whose report cannot then be written, **When** the request completes, **Then** it exits 2. The committed transition and evidence remain in effect, unchanged and not rolled back, and the caller learns the state by re-observing through 004.

---

### User Story 5 - Explicitly abandon a feature (Priority: P3)

A human decides that a non-terminal feature will not continue. They submit an `abandonment` decision
with target `abandoned` and a reason, so that 004 reports `abandoned` and 005 reports
`workflow_terminal`.

**Why this priority**: 004 makes `abandoned` reachable from every non-terminal state, but no ai-dev
path can persist it today. The feature still delivers value without it.

**Independent Test**: For a feature in each non-terminal state, submit an `abandonment` with a reason.
Confirm that 004 reports `abandoned` and that the evidence contains decision type `abandonment` and
the reason.

**Acceptance Scenarios**:

1. **Given** a recognized feature in any non-terminal state, regardless of which artifacts are present, **When** a human submits an `abandonment` with target `abandoned`, an approver identity, and a non-empty reason, **Then** the outcome is `transition_applied` with exit 0, and the evidence records decision type `abandonment` and the reason exactly as supplied.
2. **Given** an `abandonment` with a missing, empty, or whitespace-only reason, **When** it is submitted, **Then** it is a usage failure with exit 2, and nothing is read or written.
3. **Given** an `abandonment` whose target is anything other than exactly `abandoned` (for example `planned`, `Abandoned`, or `reviewing`), or an `approval` whose target is exactly `abandoned`, **When** it is submitted, **Then** it is a usage failure with exit 2, never `target_unsupported` or `transition_invalid`, and nothing is read or written.
4. **Given** a feature in a terminal state, **When** an `abandonment` is submitted, **Then** the outcome is `transition_invalid`, and nothing is written.
5. **Given** any `abandonment` report, **When** it is compared with an `approval` report, **Then** the decision type distinguishes them in both report forms.

---

### Edge Cases

- **Usage failures** (FR-030), each with exit 2, empty standard output, a diagnostic on standard error, and nothing read or written:
  - a missing decision type, or one that is not exactly `approval` or `abandonment`;
  - a missing, empty, or whitespace-only feature identifier, target, or approver identity;
  - an `abandonment` without a non-empty reason;
  - a supplied but empty or whitespace-only reason, for either decision type;
  - an `approval` targeting `abandoned`;
  - an `abandonment` whose target is anything other than exactly `abandoned`, canonical or not (C-13);
  - extra arguments, unknown options, or an unsupported output format.
- **An `abandonment` with a non-exact target** (for example `Abandoned`, `reviewing`, or `planned`): usage failure (exit 2), never `target_unsupported` or `transition_invalid`, because decision-type consistency precedes target canonicality (C-13).
- **An `approval` with a non-canonical target that is not exactly `abandoned`** (for example `Abandoned` or `Planned`): this passes decision-type consistency and reaches target canonicality, so the outcome is `target_unsupported` (exit 1), with 004's meaning.
- **The approver identity contains newlines, quotes, or control characters**: it is retained exactly as supplied and rendered escaped and quoted in human-readable output, so it cannot split the report or fake an outcome.
- **The approver identity looks like a name, an e-mail address, or anything else**: it is never validated, looked up, or authenticated. It is an auditable assertion.
- **The request comes from a script or a coding agent's shell**: ai-dev neither prompts nor reads standard input, and it does not refuse. This is the documented residual trust boundary.
- **The same successful request is resubmitted**: `transition_invalid` (C-8). There is no idempotent success and no new evidence.
- **The Workflow State Record was edited by hand outside ai-dev**: 004 reads it as it reads any record. 006 does not detect or repair hand edits, and does not verify that earlier states were reached through gates. A record that 004 classifies as invalid produces `state_invalid` and is never overwritten.
- **The authoritative state changes between evaluation and commit**: `state_conflict`, and nothing is persisted.
- **Required target artifacts are removed after the artifact precondition passed but before commit**: the precondition reflects artifact presence when it was evaluated. C-7's stale-state protection covers the authoritative Workflow State, not artifact presence.
- **Existing Human Gate evidence cannot be read or preserved**: the operation cannot complete without risking the erasure of earlier evidence, so the outcome is `persistence_failed` and nothing is persisted.
- **The feature directory is not writable, or the disk is full**: `persistence_failed`, and the prior state/evidence pair is unchanged.
- **The request runs on an unsupported platform, including Windows, iOS, or Android** (P-6, FR-037): steps 1–7 behave exactly as on a supported platform, so usage failures and the refusal outcomes are reported normally. A request that reaches persistence gets `persistence_failed` (exit 1), with nothing written. It never gets `transition_applied` or `state_conflict`.
- **The feature directory is on a filesystem where exclusive coordination or atomic replacement cannot be established safely** (for example, a network or FUSE filesystem that rejects the required locking): `persistence_failed`, and nothing is written. Coordination between gates running on different machines over a shared network filesystem is outside this feature's scope (P-6).
- **A symbolic link where state or evidence would be written**: it is never written through or replaced. Under 004, a symlinked record is `state_invalid`.
- **Existing features (001–005) currently read as `draft`**: bringing one to `approved` takes one explicit `approval` per transition. There is no bulk, backfill, or skip-ahead path.
- **The target is `draft`**: 004 has no transition into `draft`, so the outcome is `transition_invalid`. There is no reset, rollback, or backward move.
- **An `abandonment` from a state whose completion artifacts are missing**: allowed, because `abandonment` has no artifact precondition.
- **An output failure after commit**: exit 2. The transition and evidence remain in effect, and exit 2 never implies that nothing was persisted.
- **004 later adds a Workflow State or transition, or 005 changes its Required Artifacts Rule**: 006 follows both by reference with no change of its own.
- **A human wants to reopen an `approved` or `abandoned` feature**: out of scope. 004 defines no transition from a terminal state.

## Requirements *(mandatory)*

### Functional Requirements

**Capability and authority boundary**

- **FR-001**: The system MUST provide a Human Gate capability: a single, explicit, non-interactive request through which a human records exactly one Human Gate decision for exactly one identified feature. When the request is accepted, the capability persists exactly the one Workflow State transition that the decision authorizes.
- **FR-002**: A Human Gate decision MUST be a post-activity acceptance or abandonment decision. It MUST NOT authorize, trigger, schedule, or imply permission to execute any SDD activity, coding agent, or role selection.
- **FR-003**: The Human Gate capability MUST be ai-dev's sole Workflow State write path. Every Workflow State transition persisted by ai-dev MUST result from an explicit Human Gate decision. No capability of 001–005 MAY persist a transition, create Human Gate evidence, or invoke the Human Gate capability, whether directly, as a side effect, or by chaining. This requirement does not claim to prevent manual edits made outside ai-dev.
- **FR-004**: The Human Gate capability MUST NOT discover, list, check the availability of, launch, or execute any coding agent (002). It MUST NOT invoke role selection (003) or next-step evaluation (005). It MUST NOT schedule, chain, retry, fall back, or continue any work in the background.
- **FR-005**: The Human Gate capability MUST NOT read standard input, prompt, or wait for interactive input.
- **FR-006**: The only source of a Human Gate decision, and of every decision field (decision type, target, approver identity, reason), MUST be the explicit content of the Human Gate Request. The system MUST NOT infer, derive, default, or accept a decision or any decision field from any of the following:
  - a coding-agent outcome (002), including `agent_succeeded`;
  - a role-selection outcome (003);
  - a next-step report (005), including `step_recommended`;
  - a transition-evaluation result (004), including `transition_valid`;
  - artifact presence or content;
  - the current or any previous Workflow State;
  - any previous Human Gate decision or command;
  - the environment, operating-system, or version-control identity;
  - an exit code;
  - any other automated signal.
- **FR-007**: A transition being valid under 004 MUST be treated as necessary but never sufficient for persistence. The system MUST persist a transition only when an explicit Human Gate decision authorizes it and every precondition in FR-008 through FR-015 holds.

**Decision types and request content**

- **FR-008**: The system MUST support exactly two Human Gate decision types, and no others:
  - `approval`: the human accepts the completed SDD activity, and the feature moves to the explicitly supplied target;
  - `abandonment`: the human abandons the feature, and the feature moves to `abandoned`.

  The system MUST NOT define a deny, reject, final-approval, or pre-execution decision type, and MUST NOT introduce any Workflow State.
- **FR-009**: Every Human Gate Request MUST explicitly supply all of the following. A request missing any of them is a usage failure (FR-030).
  - exactly one feature identifier;
  - exactly one decision type;
  - exactly one target Workflow State;
  - a non-empty approver identity.

  The system MUST NOT derive, guess, recommend, default, or substitute the target or the approver.
- **FR-010**: A reason MUST be optional for `approval` and mandatory and non-empty for `abandonment`. A supplied reason MUST be retained exactly as supplied. A supplied reason that is empty or whitespace-only MUST be a usage failure for either decision type.
- **FR-011**: Decision-type consistency MUST be checked as part of usage validation, before target canonicality (C-13). The following are usage failures (FR-030); neither MAY be reported as `target_unsupported` or `transition_invalid`:
  - an `abandonment` whose target is anything other than exactly `abandoned`, compared exactly and case-sensitively, including non-exact variants such as `Abandoned`;
  - an `approval` whose target is exactly `abandoned`.

  Any other `approval` target proceeds to target canonicality (FR-013), where a value that is not one of 004's exact canonical Workflow State values produces `target_unsupported`, with 004's meaning unchanged.

**Feature recognition, current state, and transition validity (reuse of 004)**

- **FR-012**: The system MUST determine feature recognition and the current Workflow State with exactly the observable semantics of 004's approved state-reporting capability. This includes the safe single-directory-name identifier rule, rejecting symbolic-link feature directories without following them, and 004's record-reading rules. Where those semantics yield `feature_unknown` or `state_invalid`, the system MUST report the same token with the same meaning.
- **FR-013**: The system MUST determine transition validity solely through 004's Transition Rule Set, with exactly 004's transition-evaluation semantics and precedence (004 FR-029). That covers:
  - exact, case-sensitive target canonicality, where a non-canonical target is `target_unsupported`;
  - feature recognition;
  - state resolution;
  - rule lookup, where a canonical target that is not permitted (including from a terminal state, a skip, a backward move, or the same state) is `transition_invalid`.

  The system MUST NOT copy, restate, extend, relax, override, or reinterpret 004's rules or terminality. It MUST NOT provide any way to reopen, roll back, or reset a feature.
- **FR-014**: The system MUST NOT repair, rewrite, overwrite, default, or guess any Workflow State Record that 004 classifies as invalid. For a feature that 004 does not recognize, it MUST NOT create a feature directory, state, or evidence.

**Artifact precondition (reuse of 005)**

- **FR-015**: For `approval` only, after 004 has found the transition valid, the system MUST require that 005's Required Artifacts Rule (005 FR-010) is satisfied for the explicit target state. It MUST use that rule by reference, with 005's exact case-sensitive names, regular-file requirement, no-symlink-follow semantics, and fixed missing-artifact order, and MUST NOT copy, restate, or redefine it. If any required artifact is missing, the outcome MUST be `artifacts_missing`, listing the missing artifacts, and nothing is persisted. The system MUST NOT invoke 005's next-step capability, consult the Next-Step Table, or read artifact content. `abandonment` MUST NOT be subject to this precondition.

**Persistence and atomicity**

- **FR-016**: The persisted transition and its Human Gate evidence MUST form one logical, all-or-nothing persistence operation. After any request, including a failed or interrupted one, the observable result MUST be either the complete prior state together with the prior evidence, or the complete new state together with the prior evidence plus this decision's evidence. A state without its evidence, evidence without its state, a partially written record, and a record that 004 would classify as invalid because of the request MUST never be observable.
- **FR-017**: Before committing, the system MUST ensure that the authoritative Workflow State is still the state the request was evaluated against. If the state has changed, the outcome MUST be `state_conflict`, and nothing is persisted. Of several concurrent requests from the same from-state, at most one MAY report `transition_applied`.
- **FR-018**: When the persistence operation cannot be completed, including when existing evidence cannot be preserved or when FR-037 applies, the outcome MUST be `persistence_failed`. The system MUST report that no transition was applied, persist nothing, leave the prior state/evidence pair observable unchanged, and not retry.
- **FR-019**: After a successful commit, 004's unchanged state-reporting capability MUST report `state_reported` with the target state, and 004's terminal flag and 005's next-step evaluation MUST reflect that state under their own approved semantics. Persisted state MUST remain resolvable under 004's approved Workflow State Record semantics with no change to them.
- **FR-020**: The system MUST persist only within the identified feature's existing Spec Kit feature directory, and only the feature's authoritative Workflow State and its Human Gate evidence. It MUST NOT:
  - create, modify, or delete `spec.md`, `plan.md`, `tasks.md`, or any other artifact;
  - write to any other feature, or outside the repository;
  - create a feature directory;
  - write through or replace a symbolic link;
  - stage, commit, push, or otherwise operate on version control.
- **FR-021**: Every request that does not end in `transition_applied` MUST leave all repository and filesystem content byte-identical to its prior state. System refusals and failures MUST be reported only, and MUST never create Human Gate evidence or mutate Workflow State.
- **FR-022**: A repeated or replayed request MUST receive no special treatment. It MUST be evaluated against the current state exactly as a new request. A target that is no longer valid under 004 MUST produce `transition_invalid`, with nothing persisted. The system MUST NOT report idempotent success or any `already_applied`-style outcome.
- **FR-023**: Once a commit has completed, no subsequent report or output failure MAY roll back, invalidate, or reclassify the committed decision.

**Evidence**

- **FR-024**: Every applied transition MUST retain durable Human Gate evidence containing:
  - the feature identity;
  - the decision type;
  - the approver identity, exactly as supplied;
  - the actual from-state, meaning the state the request was evaluated against and confirmed at commit;
  - the explicit target (to-state);
  - the decision timestamp;
  - the reason, exactly as supplied, when one was supplied.

  The evidence MUST be retained within the feature's Spec Kit feature directory, so that it can be version-controlled and reviewed by humans. Its physical storage representation is a planning concern.
- **FR-025**: Successful later decisions MUST NOT erase, alter, or reorder earlier Human Gate evidence. The system MUST NOT fabricate, default, or infer any evidence field, and evidence MUST contain only the fields defined in FR-024.

**Outcomes, precedence, and reporting**

- **FR-026**: Every request MUST be processed in the following fixed order. Processing stops at the first step that produces a result, and no later step's check, read, or write is performed. Nothing is persisted before step 9.
  1. **Usage and request validation** (FR-009, FR-010, FR-030). This produces a usage failure (exit 2) and performs no filesystem access.
  2. **Decision-type consistency** (FR-011, C-13). This produces a usage failure (exit 2) and performs no filesystem access. It never produces `target_unsupported` or `transition_invalid`.
  3. **Target canonicality** (004 FR-011, FR-029 step 2), which can produce `target_unsupported`.
  4. **Feature recognition** (004 FR-008), which can produce `feature_unknown`.
  5. **Current-state resolution** (004 FR-005–FR-007), which can produce `state_invalid`.
  6. **004 transition validation** (004 FR-012), which can produce `transition_invalid`.
  7. **005-owned target-artifact consistency**, for `approval` only (FR-015), which can produce `artifacts_missing`.
  8. **Stale-state check** (FR-017), which can produce `state_conflict`. Where FR-037 applies (an unsupported platform, or primitives that cannot be established safely), this step instead produces `persistence_failed`, before any stale-state check or write.
  9. **All-or-nothing persistence** (FR-016, FR-018), which can produce `persistence_failed` or `transition_applied`.
  10. **Report**.

  Steps 1–2 are usage validation, following the 004 FR-020 and 005 FR-019 precedent that usage failures precede every domain check. Steps 3–6 are 004's own transition-evaluation precedence (004 FR-029 steps 2–5), used by reference and unchanged. Step 7 follows step 6 because C-8 requires an invalid replayed target to report `transition_invalid`, and because 005 likewise inspects no artifact before recognition and state resolution have succeeded (005 FR-006). Steps 8–9 together form the commit (C-7). An output failure (FR-031) can occur only after step 9, and never changes the outcome.
- **FR-027**: Every outcome of a recognized request MUST be exactly one of `transition_applied`, `feature_unknown`, `state_invalid`, `target_unsupported`, `transition_invalid`, `artifacts_missing`, `state_conflict`, or `persistence_failed`. `feature_unknown`, `state_invalid`, `target_unsupported`, and `transition_invalid` MUST carry exactly 004's meanings. `artifacts_missing` refers to the explicit target state and is distinct from 005's `artifacts_inconsistent`. No non-success outcome may resemble `transition_applied`.

  The system MUST provide an explicitly selectable machine-readable report in addition to the human-readable presentation. In machine-readable mode, standard output MUST contain exactly one report and no human-readable text. The feature identifier (exactly as supplied), the outcome, the decision type, and the approver identity (exactly as supplied) MUST always be present. The other fields follow this table:

  | Outcome | From-state | Target as supplied | Canonical target | Reason | Missing artifacts |
  |---|---|---|---|---|---|
  | `transition_applied` | REQUIRED | REQUIRED | REQUIRED | REQUIRED if supplied, otherwise absent | PROHIBITED |
  | `transition_invalid`, `state_conflict`, `persistence_failed` | REQUIRED | REQUIRED | REQUIRED | PROHIBITED | PROHIBITED |
  | `artifacts_missing` | REQUIRED | REQUIRED | REQUIRED | PROHIBITED | REQUIRED (non-empty) |
  | `target_unsupported` | PROHIBITED | REQUIRED | PROHIBITED | PROHIBITED | PROHIBITED |
  | `feature_unknown`, `state_invalid` | PROHIBITED | PROHIBITED | PROHIBITED | PROHIBITED | PROHIBITED |

  No report MAY contain a coding-agent identity, an agent availability or execution outcome, a role-selection result, an SDD activity, a recommendation, or any field or text that describes the approver as authenticated or verified.
- **FR-028**: The normal report MUST be written to standard output. Each human-readable report MUST:
  - occupy exactly one line;
  - display the exact outcome token;
  - render the feature identifier and the approver identity in the escaped, quoted form 004 requires (004 FR-028);
  - for `transition_applied`, state that the transition was applied on the decision asserted by the named approver;
  - for every other outcome, state that no transition was applied.

  Usage-failure and output-failure diagnostics MUST go to standard error only.
- **FR-029**: The command MUST complete with these exit codes:

  | Result | Exit code |
  |---|---|
  | `transition_applied`, persisted by this invocation | 0 |
  | `feature_unknown`, `state_invalid`, `target_unsupported`, `transition_invalid`, `artifacts_missing`, `state_conflict`, `persistence_failed` | 1 |
  | Usage failure (FR-030) | 2 |
  | Report or output failure (FR-031) | 2 |

  Exit code 0 MUST occur only when this invocation persisted both the transition and its evidence. Exit code 2 after a post-commit output failure MUST NOT be presented as meaning that nothing was persisted.

**Invalid input and output failure**

- **FR-030**: Each of the following is a usage failure outside the outcome model:
  - a missing, empty, or whitespace-only feature identifier, target, or approver identity;
  - a missing decision type, or a decision type that is not exactly `approval` or `abandonment`;
  - an `abandonment` with a missing, empty, or whitespace-only reason;
  - a supplied but empty or whitespace-only reason for either decision type;
  - an `approval` targeting `abandoned`;
  - an `abandonment` whose target is anything other than exactly `abandoned`, including non-canonical variants such as `Abandoned` (C-13);
  - extra arguments or an unrecognized option;
  - an unsupported output-format value;
  - any syntax that cannot be parsed into a recognized request.

  For a usage failure, the system MUST NOT access the filesystem, persist anything, or write to standard output. It MUST report the problem on standard error and exit 2. A structurally invalid but non-empty feature identifier is not a usage failure; it produces `feature_unknown` (FR-012).
- **FR-031**: When an outcome has been determined but the report cannot be rendered, written, or delivered, the system MUST treat this as an output failure. It MUST NOT reclassify the outcome, roll back a committed decision, retry, or fall back to another output. It MUST report the problem on standard error when possible and exit 2. Callers MUST re-observe the state through 004 to learn whether a transition was persisted.

**Determinism, trust boundary, security, compatibility**

- **FR-032**: For an identical request and identical prior content (Workflow State, evidence, and feature-directory entries), the outcome and every reported value MUST be identical, except for the decision timestamp. The system MUST NOT use scoring, ranking, randomness, prompt-based inference, learning, or any other nondeterministic mechanism.
- **FR-033**: The system MUST treat the approver identity as an auditable assertion only. It MUST NOT authenticate, validate, look up, or enrich the approver identity. It MUST NOT claim, in any output, evidence, or documentation, that the approver was authenticated or verified as a human, or that the process was launched by that person. The residual trust boundary MUST be documented as stated in the Responsibility Boundary.
- **FR-034**: The system MUST NOT emit any credential, token, secret, or sensitive environment value in any output or evidence. It MUST NOT alter, elevate, bypass, or interfere with any coding agent's permissions, sandbox, approval model, or authentication, or any credential configuration.
- **FR-035**: This feature MUST NOT change the approved observable behavior of 001's environment doctor, 002's listing and execution, 003's role selection, 004's state reporting and transition evaluation, or 005's next-step evaluation. It MUST NOT redefine their vocabularies, mappings, rules, outcomes, record semantics, or Required Artifacts Rule. 004 and 005 observe state persisted by 006 only through their unchanged, approved semantics.
- **FR-036**: Adding a coding agent (002) or a development role (003) MUST NOT change any outcome of this feature. A future Workflow State or transition (004), or a change to the Required Artifacts Rule (005), MUST take effect in 006 only through 006's by-reference use of those rules.

**Platform and filesystem scope (P-6)**

- **FR-037**: Human Gate persistence (FR-026 steps 8–9) MUST be supported on macOS and Linux local filesystems where the required primitives are available: exclusive coordination between concurrent Human Gate invocations (FR-017), and atomic replacement of the state/evidence pair (FR-016). All of the following MUST hold:
  - **Unsupported platforms, including Windows, iOS, and Android**: the system MAY perform steps 1–7 exactly as defined. It MUST NOT attempt persistence, and MUST report `persistence_failed` (exit 1) for every request that reaches step 8. It MUST NOT mutate any Workflow State or Human Gate evidence, and MUST NOT report `transition_applied` or `state_conflict`.
  - **Primitives that cannot be established safely**: on any filesystem where the required exclusive coordination or atomic replacement cannot be established safely for a request, the system MUST fail closed with `persistence_failed`, persisting nothing. It MUST NOT proceed with weaker guarantees.
  - **No support claims**: the system MUST NOT claim Human Gate persistence support for Windows,
    iOS, Android, or any platform other than macOS and Linux, or cross-machine coordination of Human
    Gates on network filesystems. These are outside this feature's scope and require future,
    explicitly scoped work.

  This requirement introduces no new outcome. It does not alter FR-016–FR-023, C-1–C-13, or any refusal outcome produced by steps 1–7.

### Key Entities

- **Human Gate**: The point at which ai-dev requires an explicit human decision before changing a feature's persisted Workflow State.
- **Human Gate Request**: One explicit, non-interactive request containing a feature identifier, a decision type, an explicit target, a non-empty approver identity, a reason (optional for `approval`, mandatory for `abandonment`), and an optional output-format choice.
- **Human Gate Decision**: Either an `approval` (post-activity acceptance) or an `abandonment`. Each authorizes exactly one transition, and only from the request's own content. It is never pre-execution authorization, never a denial, and never inferred.
- **Approver Identity**: A non-empty value supplied by the requester and recorded verbatim. It is an auditable assertion, not an authenticated identity.
- **Human Gate Evidence**: The durable, append-only, version-controllable record of an applied decision. It holds the feature, the decision type, the approver, the from-state, the to-state, the timestamp, and the reason when supplied. It is persisted all-or-nothing together with the transition, and its storage representation is a planning concern.
- **Human Gate Report**: The result of one request: the feature identifier, exactly one outcome, and the fields FR-027 requires, in human-readable and machine-readable form.
- **Reused by reference, not redefined**: Workflow State, terminality, the Transition Rule Set, the Workflow State Record, feature recognition, and transition-evaluation precedence (004); the Required Artifacts Rule (005). Coding-agent outcomes (002) and development roles (003) are never consumed.

## Success Criteria *(mandatory)*

### Measurable Outcomes

**Platform applicability (P-6, FR-037)**: success criteria whose measurement requires a committed transition are measured on supported platforms, meaning macOS and Linux local filesystems. These are SC-001, SC-005, SC-006, SC-007, SC-008, SC-012, and the post-commit clause of SC-010. All other criteria apply on every platform. SC-014 defines the required behavior on unsupported platforms.

- **SC-001**: For fixtures covering every transition 004 permits from each non-terminal state, 100% of valid requests (an `approval` with the target's required artifacts present, or an `abandonment` with a reason) report `transition_applied` with exit 0. 004 then reports exactly the target, and the evidence contains every field FR-024 requires.
- **SC-002**: For fixtures in which a coding agent reports `agent_succeeded`, 005 reports `step_recommended`, 004 reports `transition_valid`, artifacts are added, or any 001–005 capability is invoked, 0% result in a changed Workflow State or new Human Gate evidence.
- **SC-003**: For fixtures covering each refusal class, 100% of requests produce the expected outcome and exit code, and 100% leave the repository byte-identical with no evidence created. The classes are:
  - unknown feature, including `..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`, and a symlinked directory;
  - an invalid record;
  - an unsupported target;
  - a skip-ahead, backward, same-state, or terminal-origin transition;
  - missing target artifacts;
  - every usage-failure condition.
- **SC-004**: For fixtures in which a request is both invalid under 004 and missing target artifacts, 100% report `transition_invalid`, and 0% report `artifacts_missing`.
- **SC-005**: Under injected persistence failures and interruptions at every point, 0% of requests report `transition_applied` or exit 0. In 100% of subsequent reads, the state/evidence pair is the complete prior pair or the complete new pair, and 0% of reads report `state_invalid` because of the failed request.
- **SC-006**: Across repeated trials of concurrent requests from the same from-state, and of state changes injected between evaluation and commit, at most one request per trial reports `transition_applied`. Every other request reports `state_conflict` and persists nothing.
- **SC-007**: 100% of replayed, previously successful requests report `transition_invalid` with exit 1, and add no evidence.
- **SC-008**: After any sequence of successful decisions on a feature, the evidence of 100% of them is present and unaltered, and 0% of persisted states lack evidence written by the same request.
- **SC-009**: From the machine-readable report alone, a caller can determine every field FR-027 requires. 0% of reports contain a prohibited field, or any wording that describes the approver as authenticated or verified. 100% of normal reports go to standard output, and 100% of diagnostics go to standard error.
- **SC-010**: 100% of requests complete with the exit code FR-029 defines. 0% of requests that persisted nothing exit 0. 100% of post-commit output failures exit 2 with the committed decision still in effect.
- **SC-011**: Repeating a request against identical prior content produces identical outcomes and reported values in 100% of trials, except for the decision timestamp. In 0% of requests does the system read standard input or prompt.
- **SC-012**: A human reviewer can determine, from the version-controlled feature directory alone, which asserted approver decided each applied transition, of which type, from which state to which state, when, and why (when a reason was given).
- **SC-013**: 100% of the existing acceptance scenarios of 001–005 remain unchanged in observable behavior once this feature is present.
- **SC-014**: On an unsupported platform, 100% of requests that pass steps 1–7 report `persistence_failed` with exit 1, and 0% report `transition_applied` or `state_conflict`. 100% leave the repository byte-identical. Requests stopped at steps 1–7 produce the same usage failures and refusal outcomes as on a supported platform. On a supported platform, 100% of requests for which the required exclusive coordination cannot be established report `persistence_failed` and persist nothing.

## Assumptions

- The constitution's definition of human approval governs: "MUST identify the human approver and decision and MUST be recorded in a version-controlled artifact or an auditable review system". 006 records the decision and the asserted approver in the repository. Committing and reviewing that record remain human actions outside this feature.
- Feature identifier, feature recognition, Workflow State Record location and reading semantics, and transition-evaluation precedence are exactly those approved in 004. 004 explicitly left the write mechanism to a later feature (004 FR-027), and 005 anticipated that 006 introduces it.
- The artifact precondition reflects the required artifacts that exist when the artifact check runs (D-3). C-7's stale-state protection covers the authoritative Workflow State specifically. This specification makes no filesystem-wide concurrency guarantee.
- Human Gate persistence is supported on macOS and Linux local filesystems only (P-6, FR-037). Elsewhere, including on Windows, iOS, and Android, the commands fail closed with `persistence_failed` before any persistence.
- 006 does not verify consistency between pre-existing evidence and the current state, or that earlier states were reached through gates. Manual edits outside ai-dev are neither prevented nor detected.
- The following are out of scope:
  - reopening terminal features, rollback, backward moves, and reset;
  - bulk or backfill transitions;
  - multi-approver or quorum gates, delegation, and approval expiry;
  - authentication or signatures;
  - preventing an external agent from invoking the command;
  - notifications;
  - version-control operations;
  - review-result tracking and remediation loops;
  - any orchestration with 002, 003, or 005;
  - interactive or guided behavior (007 Workflow Orchestration: guided report; autonomous mode deferred);
  - Windows Human Gate persistence support (future, explicitly scoped work);
  - cross-machine or distributed coordination of Human Gates on network filesystems (future, explicitly scoped work).
- The following are planning concerns, left open as long as the observable behavior defined here is satisfied:
  - command syntax (conceptually similar to `ai-dev workflow approve <feature-id> <target> …` and a corresponding abandonment form);
  - the programming language;
  - serialization;
  - the evidence storage representation;
  - the mechanisms that achieve atomicity and stale-state detection.
- No new numeric exit code is introduced beyond the existing 0/1/2 convention.
