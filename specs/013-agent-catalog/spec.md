# Feature Specification: Agent Catalog Expansion

**Feature Branch**: `feature/013-agent-catalog`

**Feature ID**: `013`

**Created**: 2026-11-22

**Status**: Approved for planning. The owner asked for this capability on 2026-11-22 ("ai-dev debe tener el mayor catalogo posible de agentes de desarrollo"), chose the scope and the amendments from proposals presented with alternatives, authorized drafting the same day ("si, abre la rama y escribe la especificación de la 013"), and approved it as written, including Required Amendments A1 and A2, the same day ("si, apruebo la 013, haz el commit y ejecuta /speckit-plan"). This is a declared, unauthenticated assertion. Implementation still needs the plan and tasks.

**Input**: ai-dev knows two coding agents, Claude Code and Codex CLI. A team that works with any other coding agent (Gemini CLI, GitHub Copilot CLI, Cursor, Qwen Code, Amp and many more) cannot detect it, configure it for a role, or run it through ai-dev. Feature 002 designed the catalog to grow (002 FR-021, SC-008), and Feature 011 made detection and configuration follow the catalog (F011-FR-003), but no agent has been added since. This feature makes the catalog as large as the project's safety guarantees allow, and turns adding an agent into a catalog entry that meets published admission criteria instead of a new feature.

## Clarifications

### Session 2026-11-22

- **Authority record.** German Munoz, repository owner, chose the three options below on 2026-11-22, from proposals presented with alternatives. This is a declared, unauthenticated assertion.
- Q: Which agents join the catalog? → A: **Verified wave.** Every coding agent with a command-line program and a documented non-interactive mode that can be verified against its official documentation. A candidate that cannot meet the guarantees of Feature 002 (the instruction can never be read as an option; ai-dev never relaxes the agent's permissions or sandbox) stays out, with the reason recorded. Adding an agent later is only a catalog entry that meets the admission criteria of this specification, without a new feature. A user-defined ("bring your own") agent is out of scope.
- Q: May the instruction be passed as the value of the agent's prompt option? → A: **Yes, amend 002 FR-010.** The instruction may be passed either as the last argument after an end-of-options marker, as today, or as the value attached to the agent's prompt option in a single argument (for example `--prompt=TEXT`), provided it is verified for that agent that text starting with `-` can never be read as an option.
- Q: What does `ai-dev doctor` check with a large catalog? → A: **One aggregate check.** The Claude Code and Codex CLI tool checks are replaced by a single check that passes when at least one catalog agent is available and names the available ones. Per-agent detail stays in `ai-dev agents`.
- **Research input (2026-11-22).** GitHub Spec Kit 1.0.13, installed locally, defines a non-interactive invocation for 34 command-line agents (one of them, Docker Agent, needs a configuration argument) and lists 7 more without one (editor-only integrations and the generic one). It passes the prompt without end-of-options protection, and for some agents it adds options that relax permissions (`--yolo`, `--force`, `--trust`, `--approve-mcps`, `--always-approve`, `--accept-license`). Those invocations are candidates to verify, not evidence; ai-dev does not copy them.
- **Analysis clarification (2026-11-22).** `/speckit-analyze` found that the research-input counts were wrong (35 and 9 instead of 34 and 7), that Qwen Code's exit-status evidence relied on its upstream instead of its own source, and that the end-to-end test task followed the code it covers. The owner authorized the fixes ("si, aplica las correcciones de todos y haz el commit"). The counts are corrected here; the evidence and the task order are corrected in research.md and tasks.md. No requirement changes.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See every supported agent and which are installed (Priority: P1)

A developer who uses, say, Gemini CLI and GitHub Copilot CLI runs `ai-dev agents` and sees them listed with their availability, next to Claude Code, Codex CLI and every other agent ai-dev supports.

**Why this priority**: Detection is the base of everything else and is read-only. Alone, it already tells a team which of its agents ai-dev can work with.

**Independent Test**: With fake executables on `PATH` for a subset of catalog agents, run `ai-dev agents` in text and JSON. Every catalog agent appears exactly once, in the published order, with `available` exactly for the fakes present. No process is started.

**Acceptance Scenarios**:

1. **Given** fake executables for two new catalog agents on `PATH`, **When** the developer runs `ai-dev agents`, **Then** both are reported `available`, every other catalog agent `unavailable`, and no agent is started.
2. **Given** any environment, **When** `ai-dev agents --format json` runs twice with no change, **Then** both reports list the same agents in the same order with the same availability.
3. **Given** the existing agents `claude-code` and `codex-cli`, **When** the catalog grows, **Then** their identifiers, labels and position at the head of the list do not change.

---

### User Story 2 - Run any catalog agent with an instruction (Priority: P1)

The developer runs `ai-dev agent run gemini-cli "add tests for the parser"` (the identifier is illustrative; the plan fixes each one) and the agent runs in the project in its own non-interactive mode, with its own permissions, exactly as `claude-code` and `codex-cli` run today.

**Why this priority**: This is the capability the owner asked for. Without it, a larger list has no use.

**Independent Test**: For each catalog agent, with a fake executable that records its arguments, run `ai-dev agent run <id> "-x a b"`. The fake receives exactly the published invocation for that agent, the instruction arrives as one argument that the agent cannot read as an option, and no permission-relaxing option is present.

**Acceptance Scenarios**:

1. **Given** a catalog agent that is available, **When** the developer runs it with an instruction, **Then** ai-dev starts exactly that agent's published non-interactive invocation with the instruction unmodified, streams its output, and reports `agent_succeeded` or `agent_failed` from its exit status, as 002 requires.
2. **Given** an instruction that starts with `-` or `--`, such as `--help` or `-x`, **When** any catalog agent is run, **Then** the instruction reaches the agent as its prompt and never as one of its options.
3. **Given** any catalog agent, **When** it is run, **Then** its invocation contains no option, configuration or environment change that relaxes its approvals, permissions, trust or sandbox.
4. **Given** a catalog agent that is not installed, **When** the developer runs it, **Then** the outcome is `execution_failed` with the explanation that it is unavailable, as for the existing agents.

---

### User Story 3 - Configure any catalog agent for a role (Priority: P2)

A team that works with Qwen Code for implementation chooses it with `ai-dev setup` or `ai-dev setup apply`, and role selection, guidance and the cockpit use it.

**Why this priority**: Feature 011 already follows the catalog; this story confirms it works end to end for new agents.

**Independent Test**: Write a configuration that chooses two new catalog agents and assigns them to roles. `agent select`, `workflow guide` and `ai-dev tui` report those agents with source `project`.

**Acceptance Scenarios**:

1. **Given** the assistant, **When** it lists agents, **Then** it offers every catalog agent with its availability.
2. **Given** `setup apply --agent <new id> --role implementation=<new id> …`, **When** it runs, **Then** the configuration is written and role selection returns that agent.
3. **Given** no project configuration, **When** roles are selected, **Then** the built-in assignment of 003 is unchanged (`claude-code` and `codex-cli`).

---

### User Story 4 - Doctor reports agent readiness for any catalog agent (Priority: P2)

A developer whose only agent is Gemini CLI runs `ai-dev doctor` and the agent check passes, naming Gemini CLI, instead of failing because Claude Code and Codex CLI are missing.

**Why this priority**: With a large catalog, requiring two specific agents would report a ready environment as not ready.

**Independent Test**: Run `ai-dev doctor` with no catalog agent, one new agent, and several agents on `PATH`, in text and JSON. Check the single agent check's result, the agents it names, and the total number of checks.

**Acceptance Scenarios**:

1. **Given** no catalog agent installed, **When** doctor runs, **Then** the agent check fails, explains that no supported coding agent was found, and points to `ai-dev agents`.
2. **Given** one or more catalog agents installed, **When** doctor runs, **Then** the agent check passes and names the available agents in catalog order.

---

### User Story 5 - The admission record explains every candidate (Priority: P3)

A maintainer or contributor who wants to add an agent reads the admission record and sees, for every candidate evaluated, whether it was admitted and why, with the evidence: the documentation consulted, the version and the date.

**Why this priority**: It makes the catalog auditable and turns future additions into routine work, but it does not change the product's behavior.

**Independent Test**: Every candidate in the research input appears in the record with a decision and, when excluded, at least one admission criterion it fails. Every admitted agent has evidence for each criterion.

**Acceptance Scenarios**:

1. **Given** the admission record, **When** a reviewer checks any admitted agent, **Then** each admission criterion of F013-FR-003 has a source.
2. **Given** an excluded candidate, **When** a reviewer reads its entry, **Then** the failed criterion and the evidence are stated.

---

### Edge Cases

- **Another program with the same executable name.** Some candidate agents use short, generic executable names that other programs also use (for example a database migration tool or a hosting CLI). Lookup by name would report the wrong program as an available agent, and running it would pass an instruction to an unrelated program. Admission requires a distinctive executable name (F013-FR-003).
- **Instruction that looks like an option.** `--help`, `-p`, `--yolo` or `--` itself as an instruction must reach the agent as its prompt.
- **Empty-looking but valid instruction text** such as `-` keeps the validation of 002 FR-009.
- **Agent whose non-interactive mode refuses actions without permission.** ai-dev still invokes it without relaxing options. The agent's own refusal, and its exit status, are its result (`agent_failed` when it exits non-zero), not an ai-dev failure.
- **Agent whose non-interactive mode needs a mandatory per-project argument** (for example a configuration file). It cannot be admitted: the project configuration may not carry arguments (F011-FR-006).
- **Agent that changes its command-line interface after admission.** The catalog records the version verified. A later incompatible version is outside ai-dev's guarantees until the entry is re-verified; the run reports the agent's own result.
- **Agent installed under a different executable name** (an alias or a wrapper). It is not detected; detection stays an exact executable lookup.
- **Editor-only agents** without a command-line program cannot be run by ai-dev and are recorded as excluded.

## Requirements *(mandatory)*

### Functional Requirements

**Catalog**

- **F013-FR-001**: The catalog MUST contain every candidate agent that meets all admission criteria of F013-FR-003 at the time of planning, verified as F013-FR-004 requires. The candidates are at least the 34 command-line integrations of the research input plus any other coding agent with a command-line program that the planning research finds.
- **F013-FR-002**: Each catalog agent MUST have an immutable canonical identifier, a display label and an executable name, as 002 FR-004 and the Coding Agent entity define. Identifiers MUST be lowercase ASCII letters, digits and single hyphens, distinct from one another, and stable once published. `claude-code` and `codex-cli` and their labels MUST NOT change (002 FR-022).
- **F013-FR-003**: A candidate MUST be admitted only when all of the following hold:
  1. it has a command-line program and an officially documented non-interactive mode that runs one instruction against the current directory and exits;
  2. the instruction can be passed as exactly one argument that the agent's parser cannot read as an option (F013-FR-006);
  3. that mode can be started without any option, configuration or environment change that relaxes approvals, permissions, trust, sandboxing or license acceptance;
  4. it signals its own success with exit status 0 and its own failure with a non-zero status;
  5. it needs no per-project or per-user argument beyond the instruction (for example a configuration file, a profile or a model name);
  6. its executable name is distinctive: no widely distributed program that is not this agent uses that name;
  7. it runs on macOS or Linux, the platforms ai-dev supports.
- **F013-FR-004**: Every admission decision MUST be recorded with its evidence: for each admitted agent, a source for each criterion of F013-FR-003 (official documentation or the agent's own help output), the agent version verified and the date; for each excluded candidate, the criterion it fails and the evidence. Verification MUST NOT run any real agent with an instruction; reading documentation and the agent's help output is allowed.
- **F013-FR-005**: The catalog order MUST be stable: `claude-code` first and `codex-cli` second, as today, followed by every other agent in ascending order of identifier. Every report that lists agents MUST use this order.

**Invocation**

- **F013-FR-006**: Each catalog agent's invocation MUST pass the instruction exactly once, unmodified, in one of two forms, chosen and verified per agent:
  - as the last argument, after an end-of-options marker, as 002 FR-010 requires today; or
  - as the value attached to the agent's prompt option within one argument (for example `--prompt=TEXT`), when it is verified that the agent's parser takes everything after the separator as the value, including text that starts with `-`.

  A form in which the instruction is a separate argument that the parser could read as an option (for example `-p TEXT`) MUST NOT be used. This amends 002 FR-010.
- **F013-FR-007**: No catalog invocation MAY include an option, subcommand, configuration or environment change that relaxes approvals, permissions, trust, sandboxing or license acceptance, or that enables an unrestricted mode (002 FR-015 and FR-016). The invocation MAY include only what the non-interactive mode needs: the subcommand or option that selects it and the instruction. Options that select an output format, a model or a profile MUST NOT be added.
- **F013-FR-008**: Every catalog agent MUST behave exactly as 002 specifies for `claude-code` and `codex-cli`: availability by executable lookup only (002 FR-002), the same commands, outcomes, reports, exit codes, streaming, interruption handling and secret handling (002 FR-005 to FR-026). No vendor-specific command syntax MAY reach the user (002 FR-020).

**Doctor (amends 001)**

- **F013-FR-009**: `ai-dev doctor` MUST replace the Claude Code and Codex CLI tool checks with one check, identifier `coding-agent-tool`, that passes when at least one catalog agent is available and fails otherwise. When it passes, its explanation MUST name the available agents by identifier in catalog order. When it fails, its explanation MUST state that no supported coding agent was found and its corrective action MUST point to `ai-dev agents`. Doctor then evaluates eight checks; every other check, and the readiness rule (Ready only if every check passes), MUST remain unchanged. This amends 001 FR-002, FR-006 and SC-002.

**Configuration, roles, help and cockpit**

- **F013-FR-010**: Feature 011 MUST accept every catalog agent in detection, the assistant, `setup apply` and the configuration, with no change to its commands, outcomes or schema (F011-FR-003). A configuration written before this feature MUST remain valid.
- **F013-FR-011**: The built-in role assignment of 003 FR-003 MUST remain unchanged. A new agent is used for a role only through a project configuration (011).
- **F013-FR-012**: Every place that enumerates agent identifiers — per-command help, shell completion (010) and the cockpit's agent and action views (012) — MUST derive them from the catalog, so they list exactly the catalog agents in catalog order.

**Security**

- **F013-FR-013**: Adding catalog agents MUST NOT add any new way for configuration, a cloned repository or an environment variable to choose an executable, a path or an argument. The only inputs remain a catalog identifier and the instruction (F011-FR-006).

### Required Amendments

**Approved and applied (2026-11-22)** with this specification. Each amended specification records the decision in a Clarifications session dated 2026-11-22. Until Feature 013 is implemented, the amended features behave as before.

| # | Specification | Change |
|---|---|---|
| A1 | 002 Coding Agent Execution | FR-010: the instruction may also be passed as the value attached to the agent's prompt option in one argument, when verified per agent (F013-FR-006). FR-022: unchanged; Claude Code and Codex CLI remain the minimum, with their identifiers. The Coding Agent entity is unchanged. |
| A2 | 001 Environment Doctor | FR-002, FR-006, SC-002 and the edge case on all checks failing: eight checks, where the Claude Code and Codex CLI tool checks become the single `coding-agent-tool` check (F013-FR-009). The Claude and Codex Spec Kit integration checks are unchanged. |
| — | 003, 006, 007, 008, 009 | No requirement changes. |
| — | 010, 011, 012 | No requirement changes: they already follow the catalog (F011-FR-003); F013-FR-012 states the expectation for the agent lists they show. |

### Key Entities

- **Agent Catalog**: the built-in, ordered set of agents ai-dev can detect, configure and run. Each entry has a canonical identifier, a display label, an executable name and a verified non-interactive invocation. Fixed at build time.
- **Catalog Entry Invocation**: for one agent, the subcommand or option that selects its non-interactive mode and the form in which the instruction is passed (after an end-of-options marker, or attached to the prompt option).
- **Admission Record**: for every candidate evaluated, the decision (admitted or excluded), the evidence per admission criterion, the version verified and the date.
- **Coding Agent Readiness**: the doctor's aggregate result: pass with the available agents named, or fail.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of the candidate agents of the research input are classified in the admission record as admitted or excluded, and every exclusion names at least one failed criterion.
- **SC-002**: Every agent admitted is detectable and runnable with the same single command as `claude-code` today; a developer can run any of them with exactly one explicit command.
- **SC-003**: For 100% of catalog agents, tests that pass instructions starting with `-`, `--` and `--help` show the instruction reaching the agent as one argument it cannot read as an option.
- **SC-004**: 0 catalog invocations contain an option, configuration or environment change that relaxes approvals, permissions, trust, sandboxing or license acceptance, checked by a test over the whole catalog.
- **SC-005**: 0 real coding-agent processes are started by the test suites and by the admission research.
- **SC-006**: With only one catalog agent installed, other than Claude Code or Codex CLI, `ai-dev doctor`'s agent check passes and names it.
- **SC-007**: The regression suites of Features 001–012 pass with only the expectations the Required Amendments name updated: the doctor's checks and the agent lists that follow the catalog.
- **SC-008**: Adding one more admissible agent after this feature changes only the catalog, its admission record entry and its tests: no command, outcome, report structure or other feature's requirement changes.

## Assumptions

- The candidates are command-line programs on macOS and Linux. Editor-only agents, and agents that exist only as hosted services without a local command-line program, are out of scope.
- "Officially documented" means the agent vendor's documentation or the agent's own help output. Third-party integrations, including Spec Kit's, are leads to verify, not evidence.
- Authentication, accounts, subscriptions and network access remain the agent's own concern, as in 002; an agent that is installed but not signed in reports its own failure.
- The doctor keeps the Claude and Codex Spec Kit integration checks. A project that uses only other agents may still fail those checks; generalizing the integration checks to the catalog is a separate decision.
- Agent command-line interfaces change over time. The admission record fixes the version verified; keeping entries current after release is ordinary maintenance through the same admission criteria.
