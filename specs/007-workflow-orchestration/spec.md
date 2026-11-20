# Feature Specification: Workflow Orchestration

**Feature Branch**: `feature/007-workflow-orchestration`

**Feature ID**: `007`

**Created**: 2026-11-19

**Status**: Draft, revision 3 — Amendments 1–4 approved and applied. The final independent review gave GO with conditions, and its findings S-27 to S-30 are fixed here. G-SPEC (guided) still needs the owner's confirmation in a version-controlled review. No implementation is authorized.

**Input**: User description: "Feature 007 — Workflow Orchestration. Formalize the consolidated draft (`feature-007-paquete-unificado-para-claude-DRAFT.md`, 2026-11-19) against the real contracts of Features 002–006 and the constitution. Guided read-only mode is normative; autonomous mode is specified but gated and disabled. Respect the approved architecture of OS-verifiable isolation and keep autonomous execution blocked until its security gates pass."

## Clarifications

### Session 2026-11-19

Every statement in this specification is one of:
- a fact verified in the repository;
- a human decision;
- a design this specification proposes;
- an open decision, marked **OPEN D-xx**.

**Normative scope.** Only the guided mode is normative. This specification defines **no autonomous capability**: no command, option, configuration, environment value or build variant of ai-dev may start, plan or authorize an autonomous chain. Section "Deferred: Autonomous Mode Design Inputs" is explicitly **not normative**. Approving this specification, or passing G-SPEC for the guided mode, does not ratify anything in that section.

**Decisions taken by the owner before this specification.** They were made outside the repository and come from the unversioned source draft. They are recorded here so that they become version-controlled.

- Q: What isolation principle applies to any future autonomous execution? → A: Agent processes run in a restricted workspace, isolated by the operating system in a way the system can verify. Authorization and checkpoints are outside the agent's reach. The concrete mechanism is not decided.
- Q: Which mode comes first? → A: Guided mode. Autonomous execution stays blocked until its own security gates pass and a human separately authorizes it.

**Decisions recorded on 2026-11-19.** Approver: German Munoz, repository owner. This was a **blanket acceptance** ("si, estoy de acuerdo") of the four options the implementing agent recommended in the conversation that produced this specification. The approver identity is a declared, auditable assertion, not an authenticated identity, as in Feature 008. The record takes effect when this file is committed or reviewed in a pull request.

- Q (D-01): How does guidance compose 003, 004 and 005? → A: By reference to their rules, with identical semantics, never by invoking their capabilities. This requires the narrow amendments 1–3 below, which the owner approved and which were applied on 2026-11-19.
- Q (D-02): Does guided mode execute agents? → A: No. It shows command templates that a human may complete and run; execution remains the human's explicit 002 request.
- Q (D-03): What is the command? → A: `ai-dev workflow guide <feature-id>`. The top-level usage text stays unchanged.
- Q (D-04): Who may authorize a future autonomous chain, and how? → A: Only through an interactive terminal, by typing the exact plan digest, with a declared actor label recorded verbatim, as in 006 and 008. This is a design input for the deferred autonomous mode, and is not normative here.

**Decisions recorded on 2026-11-19 after the independent review.** Same approver, same caveat about identity. These were explicit choices between alternatives presented to the owner ("si, opción (a) y modo autónomo no normativo, aplícalo"):

- Q (D-14): How does guidance present the 006 approval action, given that 005 never reports a target state (005 F-01, `specs/005-sdd-workflow/spec.md:23`) and 006 never derives one (006 C-2, `specs/006-human-gates/spec.md:16`)? → A: As a template whose target is the placeholder `<target-state>`, which the human supplies and may check with 004's transition evaluation. Guidance never derives, suggests or pre-fills a target state. (Note, revision 3: after review S-27, this placeholder is the `{target-state}` human slot of F007-FR-005, whose rendering the plan defines so that an unfilled paste invokes nothing.)
- Q (D-15): Is autonomous mode normative in this specification? → A: No. It moves to the non-normative section "Deferred: Autonomous Mode Design Inputs". This specification defines no autonomous capability.

**Facts verified in the repository** that this specification depends on:

- **007 cross-references.** 005 (lines 48 and 321) and 006 (lines 130 and 552) named Feature 007 "Interactive CLI" or "interactive or guided" until Amendment 4 renamed it on 2026-11-19.
- **Side-effect prohibitions.** Three requirements forbid evaluating a capability as a side effect of another: 003 FR-013 (`specs/003-agent-role-selection/spec.md:128`), 004 FR-017 (`specs/004-workflow-state/spec.md:145`) and 005 FR-024 (`specs/005-sdd-workflow/spec.md:273`). 005 and 006 reuse earlier features' rules by reference without invoking them (005 lines 43 and 144; 006 lines 127–128).
- **002 single execution.** 002 allows at most one agent invocation per human request, with no chaining (FR-024, line 131). It forbids role-based routing (FR-023, line 130) and requires the instruction unmodified (FR-010, line 117). It forbids relaxing the agent's approval or sandbox model, and requires every agent interaction to reach the developer unchanged (FR-016, line 123).
- **003 fixed mapping.** Specification, planning and review map to `claude-code`; implementation and remediation map to `codex-cli` (FR-003, line 103).
- **Workflow State Record location.** It lives inside the feature directory in the repository (004 FR-027, line 175). 006 records the approver as an unauthenticated, auditable assertion. Preventing an external agent from invoking the gate command is a residual trust boundary 006 leaves out of scope (lines 39–42, 161; FR-033, line 481).
- **Feature identifiers.** Any single path element other than `.` that contains no `/` is a valid identifier (`internal/workflow/record.go`, `validFeatureID`). That includes shell metacharacters and a leading `-`.
- **Option separator.** The existing commands `workflow state`, `workflow transition`, `workflow next`, `workflow approve`, `agent select` and `agent run` all accept `--` before their positional arguments, including an identifier that starts with `-`. This was verified on 2026-11-19 against a build of `dev` at `0e99128`.
- **Positive non-authorization statement.** 005 requires its human-readable report to say visibly that a recommendation does not authorize the activity or any transition (005 FR-017, lines 228–232).

### Session 2026-11-20 (004 Amendment 5 inherited)

- **Authority record (2026-11-20): human decision / human-approved cross-feature amendment.** German Munoz, repository owner, decided to amend 004 FR-028 in the words "Hallazgo de 004 FR-028 enmendar", after the finding was reported during the Feature 007 follow-up work. The approver identity is a declared, auditable assertion, not an authenticated identity.
- 004 FR-028's escaped, quoted form is now Go's double-quoted form with every `$`, backquote and `!` additionally backslash-escaped (004 Amendment 5). It applies by reference to the feature identifier its status line renders in 004's form (F007-FR-006). It resolves the status-line residual documented for the guidance report. No other requirement of this feature changes.

## Ownership Boundaries

| Capability | Owner | Feature 007 obligation |
|---|---|---|
| Agent listing, availability, single execution | 002 | Guidance never executes agents; it may show 002's run command as a template |
| Role → agent mapping | 003 | Used by reference; never overridden or substituted |
| Workflow States, transitions, Workflow State Record | 004 | Recognition, state and terminality used by reference. Transition evaluation is never performed by guidance; it is only offered as a command template |
| Next-Step Table, Required Artifacts Rule, SDD activities | 005 | Used by reference; information, never authorization |
| Human Gate decisions; sole Workflow State write path | 006 | Never invoked or written; offered only as a template with placeholders for every decision field |
| Every Git operation | 008 | None |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Guided next-action report (Priority: P1)

A developer asks for guidance on one feature. They see:
- its Workflow State;
- the recommended SDD activity and role;
- the agent that role maps to, and whether that agent is available;
- command templates for the next human actions.

Nothing is executed, written, derived for a decision, or approved.

**Why this priority**: It is the smallest useful composition of 003–005. It gives the developer the next step in one request, with no new authority or risk.

**Independent Test**: For fixtures in every Workflow State, every invalid or inconsistent case and hostile feature-directory names, run the guidance request. Compare its outcome with 005's for the same fixture, check every rendered template, and confirm that the project tree is unchanged and that no process started.

**Acceptance Scenarios**:

1. **Given** a recognized non-terminal feature whose required artifacts are present, **When** the developer requests guidance, **Then** the outcome is `step_recommended`. The report shows the state, activity, role, mapped agent, the agent's availability, the action templates of F007-FR-005, and the statement of F007-FR-008.
2. **Given** a feature in `approved` or `abandoned`, **When** guidance is requested, **Then** the outcome is `workflow_terminal`, and the only template is 004's state report.
3. **Given** an unknown feature, an invalid Workflow State Record, or missing required artifacts, **When** guidance is requested, **Then** the outcome is `feature_unknown`, `state_invalid` or `artifacts_inconsistent`, exactly as 005 would report. No activity, role, agent or template is reported.
4. **Given** any guidance request, **When** it completes, **Then**:
   - no agent process has started;
   - no file, record, store or Git state has changed;
   - no network contact has been attempted.
5. **Given** a role whose mapped agent is not available, **When** guidance is requested, **Then** the agent is reported as unavailable. The 002 run template is omitted, and no other agent is suggested.
6. **Given** a feature directory whose name contains shell metacharacters or starts with `-`, **When** guidance is requested, **Then** every template quotes the identifier so that pasting it runs exactly the intended `ai-dev` command, after `--`. An identifier with a control or format character (F007-FR-006) produces no templates.

---

### User Story 2 - No autonomous capability exists (Priority: P1)

Nothing in ai-dev can start, plan or authorize an autonomous chain. Autonomous execution stays blocked because the capability does not exist, not because a switch is off.

**Why this priority**: The owner decided to keep autonomy blocked until its gates pass. This story makes that observable without adding any surface.

**Independent Test**: Confirm that the only new command is `workflow guide`. Every other `ai-dev workflow` subcommand that does not exist must still be a usage failure with exit code 2, unchanged.

**Acceptance Scenarios**:

1. **Given** the built ai-dev, **When** any non-existent `workflow` or top-level subcommand is requested (for example `workflow run`, `workflow chain` or `orchestrate`), **Then** the existing usage failure occurs, with exit 2, and with no effect.
2. **Given** any configuration file or environment variable, **When** ai-dev runs, **Then** no autonomous behavior exists to enable.

---

### Edge Cases

- **The Workflow State Record changes during a guidance request:** the report reflects what was read, with 004's semantics. Nothing is retried.
- **The mapped agent is unavailable:** it is reported as unavailable, and the 002 run template is omitted.
- **The 003 mapping names an agent that is not in the supported-agent registry:** this is 003's `selection_inconsistent` condition. Availability is reported as `inconsistent`, and the 002 run template and the 003 selection template are omitted.
- **The report cannot be written after the outcome is determined:** output failure, exit 2, as in 005 FR-020.
- **Usage failures:** these are 005 FR-019's conditions, applied to `workflow guide` (exit 2).
- **A feature identifier with a control or format character** (Unicode Cc or Cf, U+2028 or U+2029): it is reported with 004 FR-028's escaping, and no templates are rendered.
- **The 005 Next-Step Table, the 003 mapping or the 004 states change in a later feature:** guidance follows them by reference, with no change of its own. It does not depend on 004's successor structure, because it never derives a target.

## Requirements *(mandatory)*

### Functional Requirements

- **F007-FR-001**: The system MUST provide one explicit, human-initiated, read-only guidance request for exactly one feature identifier: `ai-dev workflow guide <feature-id>`. Usage failures are 005 FR-019's conditions, applied to this command.
- **F007-FR-002**: Guidance MUST determine the following by reference to 004 and 005, with their exact semantics, case sensitivity and precedence:
  - feature recognition;
  - the current Workflow State and its terminality;
  - the Required Artifacts check;
  - the SDD activity and role.

  Its outcome MUST be exactly the one 005 reports for the same inputs: `step_recommended`, `workflow_terminal`, `artifacts_inconsistent`, `feature_unknown` or `state_invalid`.
- **F007-FR-003**: Guidance MUST NOT perform 004's transition evaluation. It MUST NOT derive, report, suggest or pre-fill a target Workflow State, nor any other Human Gate decision field.
- **F007-FR-004**: For `step_recommended` only, guidance MUST report the canonical agent identity the role maps to under 003 FR-003, and that agent's availability as `available`, `unavailable` or `inconsistent`. `available` and `unavailable` are determined as in 002 FR-002. `inconsistent` is the 003 FR-009 condition. This is information only. Guidance MUST NOT report a role-selection outcome of its own, an alternative agent, or a fallback.
- **F007-FR-005**: Guidance MUST list the next human actions as command templates. It MUST NOT execute any of them. The set and the order are fixed. In this table, `{name}` denotes a slot: it describes the template's content and is not its rendered text.

  | Outcome | Templates, in order |
  |---|---|
  | `step_recommended` | (1) 003 selection: `ai-dev agent select -- {role}`, unless availability is `inconsistent`. (2) 002 execution: `ai-dev agent run -- {agent} {instruction}`, only when availability is `available`. (3) After the human has completed the activity, 004 evaluation: `ai-dev workflow transition -- {feature} {target-state}`. (4) 006 approval: `ai-dev workflow approve --approver {approver} -- {feature} {target-state}` |
  | `workflow_terminal` | `ai-dev workflow state -- {feature}` |
  | any other outcome | none |

  The rendering rules:
  - **Filled slots.** `{role}` and `{agent}` are rendered as their canonical tokens. `{feature}` is rendered by F007-FR-006.
  - **Human slots.** `{instruction}`, `{approver}` and `{target-state}` are for the human to fill. Guidance MUST NOT fill them from any source, including the environment, Git configuration, the operating-system user or earlier requests.
  - **Unfilled templates do nothing.** A template is shown as a template to complete, never as a ready-to-run command. Each human slot MUST be rendered so that a template pasted into a POSIX shell with that slot still unfilled, whatever other slots are filled, does not invoke ai-dev or any agent and does not create, change or remove a file. The plan defines that rendering.
  - **Optional reason.** The approval template carries no `--reason`. Next to it, guidance MUST state that 006 FR-010 accepts an optional `--reason` for `approval`, without rendering a value for it.
- **F007-FR-006**: The feature identifier in every template MUST be rendered as one POSIX-shell single-quoted word, with each embedded `'` written as `'\''`, and placed after `--`, so that pasting the template passes it as exactly one positional argument. When the identifier contains a character of Unicode general category Cc (control) or Cf (format, including bidirectional overrides and zero-width characters), or U+2028 or U+2029, guidance MUST render no templates. In the human-readable report, the identifier itself MUST use 004 FR-028's escaped, quoted form.
- **F007-FR-007**: Guidance MUST NOT invoke the capabilities of 002, 003, 004, 005 or 006. It MUST reuse their rules by reference only, as 005 and 006 already do. It MUST NOT do any of the following:
  - launch, authenticate or contact any coding agent or network endpoint;
  - read standard input, prompt, or wait for interactive input;
  - create, modify or delete any file, record, store or Git state.
- **F007-FR-008**: Every human-readable guidance report MUST state visibly that the listed actions are options for the human, not authorization. It MUST say that running an agent or a review is not approval, and that approval is the human's own Human Gate decision after completing the activity. The machine-readable report MUST carry the same statement as a fixed field. No report MAY describe an action as approved or authorized, or describe an approver as authenticated.
- **F007-FR-009**: For identical inputs, every reported value MUST be identical across requests. The inputs are the Workflow State Record content (or its confirmed absence), the feature-directory entries, the supported-agent registry and local availability.
- **F007-FR-010**: Guidance MUST provide a human-readable report and an explicitly selectable machine-readable report. The machine-readable report MUST carry a versioned schema identifier, following the pattern of Feature 008's reports (for example `internal/report/conventionalcommit.go`), and its standard output MUST contain exactly one report. Usage and output diagnostics MUST go to standard error.
- **F007-FR-011**: Exit codes MUST follow 005 FR-018 exactly:
  - 0 for `step_recommended` and `workflow_terminal`;
  - 1 for `artifacts_inconsistent`, `feature_unknown` and `state_invalid`;
  - 2 for usage failures and output failures.

  Exit code 0 MUST NOT be presented as approval, authorization or a transition.
- **F007-FR-012**: No output MAY contain a credential, token, secret or sensitive environment value.
- **F007-FR-013**: The observable behavior of Features 001–006 and 008 MUST remain unchanged, including:
  - the top-level usage text;
  - the usage failure for non-existent `workflow` subcommands.
- **F007-FR-014**: This specification defines no autonomous capability. No command, option, configuration value, environment variable or build variant introduced under it MAY plan, authorize or start an agent chain, or invoke more than zero agents.
- **F007-FR-015**: Adding a coding agent, a development role or a Workflow State MUST change guidance only through the referenced owners' rules.

### Key Entities *(include if feature involves data)*

- **Guidance Report**: For one feature, it holds:
  - the 005 outcome and the Workflow State;
  - when one is recommended, the SDD activity and role;
  - the mapped agent and its availability;
  - the command templates;
  - the fixed non-authorization statement.

  It is never persisted.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Fixtures cover all seven Workflow States and every invalid or inconsistent case. In 100% of them, guidance reports the same outcome as 005 for the same fixture.
- **SC-002**: Across 100% of guidance requests:
  - zero processes are started;
  - zero changes appear in the project tree, compared by content digest before and after;
  - zero network-capable code paths, proven by the structural boundary test, and zero network attempts in the end-to-end test, which runs without agents on `PATH`.
- **SC-003**: In 100% of `step_recommended` fixtures, the reported agent equals 003's mapping and the availability equals 002's determination, or `inconsistent` for the 003 FR-009 condition. Zero reports substitute or suggest another agent.
- **SC-004**: Across 100% of fixtures, including hostile names (shell metacharacters, a leading `-`, quotes, spaces), pasting each rendered template into a POSIX shell passes the identifier to ai-dev as exactly one positional argument. Zero templates are rendered for identifiers with a control or format character, with fixtures that include U+202E and U+200B. Pasting a template with any placeholder unfilled invokes ai-dev or an agent in 0% of cases and changes zero files. Zero templates contain a derived value for `{target-state}`, `{approver}` or `{instruction}`. For each human slot separately, pasting the template with only that slot unfilled invokes nothing and changes zero files.
- **SC-005**: 100% of human-readable and machine-readable reports carry the non-authorization statement of F007-FR-008.
- **SC-006**: The regression suites of Features 001–006 and 008 pass unchanged, including the top-level usage text and the usage failure for non-existent `workflow` subcommands.
- **SC-007**: From the machine-readable report alone, a reader can find the outcome, state, activity, role, agent, availability, templates and statement in 100% of reports.

## Assumptions

- Guided mode needs no persistence and no new store. It reads only what 004 and 005 already read, plus the local availability 002 already determines.
- Required Amendments 1–4 are approved before guided mode is implemented. Without them, guidance cannot report the activity, role or agent and still comply with 003 FR-013, 004 FR-017 and 005 FR-024.
- No authenticated identity mechanism exists in ai-dev. Every human decision this feature records is a declared, auditable assertion, as in 006 and 008.
- The current 002 adapters pass the instruction as a positional argument without an option separator. The launcher inherits the caller's environment and standard input, and signals only the direct child (`internal/execution/adapters`, `internal/system/oslauncher.go`). This is acceptable for 002's human-written instructions. It is recorded for the deferred autonomous mode and for a separate 002 hardening task (D-12).

## Required Amendments

**Approved and applied (2026-11-19).** German Munoz, repository owner, approved Amendments 1–4 ("si, apruebo las enmiendas 1 a 4, aplícalas"). That identity is a declared, auditable assertion. Each amended specification records the decision in a Clarifications session dated 2026-11-19. The line numbers below are those of the amended files.

1. **003 FR-013** (`specs/003-agent-role-selection/spec.md:128`), appended: "An explicit, human-initiated Feature 007 guidance request MAY apply the fixed mapping (FR-003), the availability determination (FR-005) and the mapping consistency check (FR-009) by reference, with identical semantics, as information that is not a role selection outcome. It MUST NOT invoke this capability, persist, or produce any effect beyond the guidance report."
2. **004 FR-017** (`specs/004-workflow-state/spec.md:145`), appended: "An explicit, human-initiated Feature 007 guidance request MAY apply this feature's recognition, state-determination and terminality rules by reference, with identical semantics. It MUST NOT perform transition evaluation, invoke either capability, persist, or produce any effect beyond the guidance report."
3. **005 FR-024** (`specs/005-sdd-workflow/spec.md:273`), appended: "An explicit, human-initiated Feature 007 guidance request MAY apply the Required Artifacts Rule and the Next-Step Table by reference, and report this feature's outcome unchanged. It MUST NOT report a target Workflow State, invoke this capability, persist, or produce any effect beyond the guidance report."
4. **Cross-references** in 005 (lines 48 and 321) and 006 (lines 130 and 552): renamed Feature 007 to "Workflow Orchestration (guided report; autonomous mode deferred)".

## Release Gates

| Gate | Pass condition |
|---|---|
| G-SPEC (guided) | D-01 to D-04, D-14 and D-15 recorded and committed (done, `1b85862`); Amendments 1–4 approved and applied (done, `f1bbaa5`); final independent review recorded (done: GO with conditions, findings S-27 to S-30 fixed in revision 3); owner confirmation of the recorded decisions in a version-controlled review |
| G-IMPLEMENT (guided) | Separate human authorization for the guided slice |

A future autonomous mode needs its own specification or amendment. It must pass its own G-SPEC, G-CONTRACT and G-ISOLATION before any autonomous capability exists.

## Out of Scope

- Any autonomous capability.
- Executing agents from guidance.
- Deriving or suggesting a target Workflow State or any Human Gate field.
- Automatic Git operations, pull requests, merges or pushes.
- Native Windows.
- An authenticated approver identity.

---

## Deferred: Autonomous Mode Design Inputs *(NOT normative, NOT ratified)*

This section preserves, inside the repository, the design inputs a future autonomous specification must start from. It records the independent review's findings on them. Nothing here is a requirement of Feature 007. Nothing here may be implemented until a later specification or amendment makes it normative and passes its own gates.

### Design inputs (from the source draft, revised)

- **AD-01 Plan.** An immutable plan (ORCH-001), identified by a digest computed outside the agent plane, binding:
  - the feature and input snapshot;
  - the steps, roles and agents;
  - the permission profile;
  - the permitted and forbidden effects;
  - the limits and remediation budget;
  - the stop rules;
  - the isolation profile.
- **AD-02 Authorization.** Bound to the exact plan digest and snapshot. It is issued only through an interactive terminal, by typing the digest, with a declared actor label (D-04). It grants no transition, gate, Git operation or out-of-plan effect. Residual boundary (review S-14): any process that can allocate a pseudo-terminal could imitate this, so the control only matters for agents that are actually inside the isolation boundary.
- **AD-03 Selection.** 003's mapping is used by reference. A change, an unavailable agent or a changed executable invalidates the plan.
- **AD-04 Process results.** Process success is never review success, a transition or acceptance.
- **AD-05 Run states.** Run states stay separate from 004, and are never stored in a Workflow State Record.
- **AD-06 Isolation.** Isolation is enforced by the operating system or an equivalent boundary, and verified before every launch.
- **AD-07 Protected from writes** (extended by review S-07). The agent plane can never create, change or remove any of these, including through symbolic or hard links, descendants, inherited handles or recursive `ai-dev`:
  - Workflow State Records;
  - Human Gate evidence, wherever it is stored;
  - governing artifacts (specifications, plans, task lists, the constitution);
  - the trusted store;
  - the controller's executable and policy;
  - Git metadata;
  - agent configuration, hook and instruction files that the same or another agent, or a later human session, would load automatically.
- **AD-08 Privileged channels.** The agent plane never receives a terminal, the caller's standard input, the SSH agent, a container-runtime socket, a Git credential helper, or a non-allowlisted environment variable. Network egress and credential access are exactly as D-05 defines.
- **AD-09 Agent options.** Options come only from an argument boundary tested per provider and version.
- **AD-10 Instructions.** Instructions come from authorized templates. A digest of materialized instructions is not human approval.
- **AD-11 Permission profile.** Only under an approved D-06. An interaction request stops the chain unanswered; how it is detected without standard input is an open contract question (S-20).
- **AD-12 Deadlines.** Every step and run has a deadline. Termination covers the whole process tree; an unconfirmed termination is indeterminate.
- **AD-13 Review verdict.** PASS, FAIL or INCONCLUSIVE, strictly parsed. A missing or malformed verdict is INCONCLUSIVE, and exit 0 never means PASS.
- **AD-14 Read-only review.** Review uses a read-only snapshot verified before and after; any mutation invalidates the review.
- **AD-15 Independence.** An effective PASS requires a different provider from the implementer's (D-08).
- **AD-16 Remediation budget.** Budget is reserved before each attempt, never refunded, and bound to the authorization and snapshot.
- **AD-17 Trusted records.** Written atomically, only by the controller, in a store the agent cannot reach.
- **AD-18 Stop rules** (aligned with the source draft's US4 and US5, review S-20):
  - PASS, INCONCLUSIVE or an exhausted budget;
  - a security violation or an interaction request;
  - unverifiable isolation or a timeout;
  - an invalid review or a scope change;
  - an external Workflow State change or an unknown effect.
- **AD-19 Forbidden operations.** No 006 invocation and no Workflow State or gate write. The agent plane has no Git or container-runtime access. Starting the isolation boundary itself, for example a controller-owned container or VM, is the controller's own act (review S-09).
- **AD-20 Outcomes.** Outcome tokens and run states are separate vocabularies, each fully defined (review S-10). The candidate outcomes are `completed`, `failed`, `cancelled`, `stopped_awaiting_human`, `indeterminate`, `isolation_unavailable`, `agent_unavailable` (to be renamed, because it collides with 003's token) and `authorization_required` (to be defined).
- **AD-21 Retention.** Durable records hold metadata, digests and references only; secrets are redacted or quarantined (D-11).
- **AD-22 Indeterminate runs.** Never resumed automatically. Reconciliation is explicit and rechecks the authorization, the Workflow State, the snapshot and outstanding effects.
- **AD-23 Concurrency.** Concurrent runs under one authorization or snapshot are serialized or refused.
- **AD-24 Platforms.** WSL2 is **not** an autonomous target until it has its own isolation evidence, with Windows interoperability and `/mnt/c` access shown to be blocked (review S-15).

### Conceptual contracts (copied from the source draft; fields marked OPEN stay OPEN)

- **ORCH-001 WorkflowPlan:**
  - `schema_version`, `plan_id`;
  - `feature_id`, `workflow_observation_ref`, `input_snapshot_ref`;
  - `steps[{step_id, kind, role, selection_ref, instruction_template_ref, input_refs, expected_effects, forbidden_effects}]`;
  - `limits{max_total_duration, max_step_duration, max_remediation_attempts}`;
  - `isolation_profile_ref`, `stop_conditions`;
  - `plan_digest`, whose algorithm and canonical payload are OPEN.
- **ORCH-002 ChainAuthorization:**
  - `authorization_id`, `plan_digest`, `input_snapshot_ref`;
  - `human_actor_ref` (a declared label, D-04), `human_decision_evidence_ref`;
  - `allowed_effects`, `forbidden_effects`, `isolation_profile_ref`;
  - `issued_at`, `expires_at`, `revoked_at` (semantics OPEN, including in-flight revocation, review S-21);
  - `budget_total`, `budget_consumed`, `status`, `audit_ref`.
- **ORCH-003 OrchestrationRun:**
  - `schema_version`, `run_id`, `plan_digest`, `authorization_id`, `feature_id`, `controller_ref`;
  - `state`, `started_at`, `updated_at`, `step_evidence_refs`, `last_checkpoint_ref`, `outcome_code`.

  The run states are OPEN. The source draft's `validated` and `awaiting_authorization` were dropped because a run exists only after a valid authorization, and `awaiting_human` became `stopped_awaiting_human` (review S-21).
- **ORCH-004 StepEvidence:**
  - `step_id`, `run_id`, `agent_selection_ref`, `adapter_identity`, `adapter_version`;
  - `instruction_template_digest`, `materialized_instruction_digest`;
  - `input_snapshot_ref`, `output_snapshot_ref`, `started_at`, `ended_at`;
  - `process_outcome`, `exit_code`, `observed_effects`, `sanitized_evidence_refs`, `uncertainty_reason`.
- **ORCH-005 ReviewResult:**
  - `review_id`, `run_id`, `reviewed_snapshot_ref`, `reviewer_selection_ref`, `implementer_identity_ref`;
  - `independence_evidence_ref`, `read_only_verification_ref`;
  - `verdict` ∈ {PASS, FAIL, INCONCLUSIVE}, `findings[{id, severity, artifact_ref, description}]`, `limitations`, `review_evidence_ref`.
- **ORCH-006 TrustedCheckpoint:**
  - `checkpoint_id`, `run_id`, `authorization_id`, `plan_digest`, `sequence` (strictly increasing);
  - `last_confirmed_step_id`, `run_state`, `budget_consumed`;
  - `outstanding_operation_refs`, `observed_effect_refs`;
  - `certainty` ∈ {confirmed, indeterminate}, `written_at`, `previous_checkpoint_ref`.

### Negative test catalog (copied from the source draft and extended)

| ID | Test | Expected |
|---|---|---|
| NEG-001 | Agent edits Human Gate evidence or a Workflow State Record | Denied; security event; stop |
| NEG-002 | Agent invokes `ai-dev workflow approve` with a forged actor | Denied by the trust boundary; no gate |
| NEG-003 | Agent modifies a grant, budget or checkpoint | Denied; no trusted mutation |
| NEG-004 | Agent reaches the SSH socket, tokens or privileged IPC | Denied; stop |
| NEG-005 | Agent mutates Git or reaches a container-runtime socket | Denied; stop |
| NEG-006 | A child process survives a timeout | Indeterminate; no continuation |
| NEG-007 | Provider CLI parses untrusted text starting with `-` as an option | Adapter ineligible; no launch |
| NEG-008 | Reviewer exits 0 with a malformed verdict | INCONCLUSIVE |
| NEG-009 | Reviewer mutates the reviewed snapshot | INCONCLUSIVE; stop |
| NEG-010 | Two runs consume one authorization's budget | Atomic serialization or rejection |
| NEG-011 | Plan changed after the human decision | Authorization invalid |
| NEG-012 | Unknown platform isolation capability | Autonomous launch refused |
| NEG-013 | Agent requests interactive input | Stop without a response |
| NEG-014 | Agent output contains a secret | Quarantine or redact; no raw durable log |
| NEG-015 | Agent invokes `ai-dev` recursively | No gate, authorization or checkpoint change |
| NEG-016 | Same-provider review claims PASS | No effective PASS |
| NEG-017 | Implementer writes agent configuration, hooks or instruction files | Denied or quarantined; never loaded by the reviewer or later steps |
| NEG-018 | Agent reaches Windows interoperability or `/mnt/c` under WSL2 | Denied, or WSL2 refused as a target |

### Open decisions for the future autonomous specification

| ID | Decision | Recommended option |
|---|---|---|
| D-05 | Network and credentials of live agents | Egress allowlist to the provider's endpoints, controlled by the controller; the agent's own credentials read-only |
| D-06 | 002 FR-016 in unattended mode | Amend 002 for a plan-declared permission profile, only under verified isolation |
| D-07 | Isolation mechanism per operating system | Spike: VM or container boundary vs. dedicated user plus OS sandbox, covering nested agent sandboxes and denial inside an allowed tree |
| D-08 | Reviewer independence | An effective PASS needs a different provider; first scope is implement → review → remediate only |
| D-09 | Remediation budget | Default 2, bound to authorization and snapshot, never refunded |
| D-10 | Trusted store | A state root outside the repository, as in Feature 008, protected by D-07 |
| D-11 | Evidence retention | Metadata and digests only |
| D-12 | 002 argument boundary | Separate 002 hardening task: an option separator and tests |
| D-13 | Workspace and effect-import model (review S-08) | Decide where the agent writes, and how its results reach the working tree without a Git operation owned by 008. This blocks G-ISOLATION |

### Amendments a future autonomous specification will need

- **002:** a chain-derived invocation profile covering FR-006, FR-010, FR-016 (with D-06), FR-023, FR-024 and FR-025.
- **003 FR-013:** a deterministic, plan-authorized remediation branch, decided by the human upfront (review S-06).
- **004 FR-017 and 005 FR-024:** state and recommendation reads during an authorized chain (review S-06).
- **006:** a threat-model clarification. Under autonomy, the residual boundary of lines 39–42 becomes a requirement on isolation (AD-07, NEG-001, NEG-002, NEG-015).
