# Feature Specification: Agent Detection and Role Configuration

**Feature Branch**: `feature/011-agent-setup`

**Feature ID**: `011`

**Created**: 2026-11-21

**Status**: Approved for planning. The owner asked for this capability on 2026-11-21 ("en el momento de activar o instalar ai-dev este debe detectar todos los agentes que se tengan instalados en el sistema, para poder elegir con cuales trabajar y configurar su rol"), authorized drafting the same day ("si, abre la rama y escribe la especificación de la 011"), and approved it as written, including the proposed defaults and Required Amendments A1–A4, the same day ("si, apruebo la 011, haz el commit y ejecuta /speckit-plan"). This is a declared, unauthenticated assertion. Implementation still needs the plan and tasks.

**Input**: Today ai-dev knows two coding agents, Claude Code and Codex CLI, and checks on every command whether each one is installed. Each of the five development roles goes to a fixed agent written into ai-dev (003 FR-003): specification, planning and review to `claude-code`; implementation and remediation to `codex-cli`. A team cannot see which agents ai-dev detects in one place, cannot choose which of them to work with, and cannot change which agent each role uses. Feature 003 left that as a possible future feature.

## Clarifications

### Session 2026-11-21

- **Authority record.** German Munoz, repository owner, chose the three options below on 2026-11-21, from proposals presented with alternatives. This is a declared, unauthenticated assertion.
- Q: What does "every installed agent" mean? → A: **Extensible catalog.** ai-dev detects the agents of its known catalog, and only by looking up each agent's executable. Today the catalog holds Claude Code and Codex CLI. Each new agent joins the catalog with its own execution support, in a separate decision. Executables outside the catalog are never detected, guessed or run.
- Q: When are agents detected and the configuration written? → A: **Only on an explicit command,** started by a person: an interactive assistant and a non-interactive form. Nothing happens automatically on installation or on first use, and without a configuration ai-dev behaves exactly as it does today.
- Q: Where is the configuration kept? → A: **Per project,** in one file inside the project, meant to be versioned with it, so everyone working on the project uses the same role assignment and its changes are reviewable.
- **Proposed defaults, part of the approval.** An agent that is not installed can be chosen, with a visible warning, so a team can configure an agent some members have not installed yet. Every role must have an agent; there is no unassigned role. Choosing agents does not restrict `ai-dev agent run`, which keeps running any catalog agent a person names explicitly (002 is unchanged).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See which agents ai-dev detects and how roles are assigned (Priority: P1)

A developer starting on a project wants to know which coding agents ai-dev can find on this machine, and which agent each role uses in this project, before running anything.

**Why this priority**: Every other story builds on a trustworthy, read-only view of detected agents and the role assignment in effect. It is useful alone and cannot change anything.

**Independent Test**: In projects with no configuration, a valid configuration and an invalid one, and with each agent installed or not, run `ai-dev setup show` in text and JSON. Check that nothing is written and no agent is started.

**Acceptance Scenarios**:

1. **Given** a project without a configuration, **When** `ai-dev setup show` runs, **Then** it lists every catalog agent with its availability (`available` or `unavailable`), reports outcome `not_configured`, and shows the built-in role assignment with source `built_in`. Exit code 0.
2. **Given** a project with a valid configuration, **When** `ai-dev setup show` runs, **Then** it lists every catalog agent with its availability and whether the project has chosen it, reports outcome `configured`, and shows the configured role assignment with source `project`. A chosen agent that is unavailable carries a warning. Exit code 0.
3. **Given** a project whose configuration is invalid, **When** `ai-dev setup show` runs, **Then** it reports outcome `configuration_invalid` with the reasons, shows no role assignment, and exits 1. It never falls back to the built-in assignment.
4. **Given** `--format json`, **Then** standard output holds exactly one JSON object with the same content, and agent output, warnings and prompts never mix into it.

---

### User Story 2 - Choose agents and assign roles with an assistant (Priority: P1)

A developer runs one interactive command, sees the agents ai-dev detected, picks the ones the project works with, assigns an agent to each role, reviews the result and confirms before anything is written.

**Why this priority**: This is the capability the owner asked for. It depends only on the detection of Story 1.

**Independent Test**: Drive `ai-dev setup` in a pseudo-terminal through choosing, assigning, declining and confirming. Check the file only appears after an explicit confirmation, and that no agent process ever starts.

**Acceptance Scenarios**:

1. **Given** an interactive terminal, **When** `ai-dev setup` runs, **Then** it lists every catalog agent with its availability, asks which agents the project works with, then asks for an agent for each of the five roles, offering only the chosen agents. Choosing an unavailable agent shows a warning and is allowed.
2. **Given** an existing valid configuration, **Then** the assistant starts from it, so the developer can keep or change each answer.
3. **Given** the developer has answered, **Then** the assistant shows the complete resulting configuration and asks for confirmation. The default answer is no. Only an explicit yes writes the configuration. Any other answer, end of input or an interrupt writes nothing and exits 1 with outcome `setup_cancelled`.
4. **Given** the result equals the existing configuration, **Then** nothing is written and the outcome is `configuration_unchanged`.
5. **Given** no interactive terminal, **When** `ai-dev setup` runs, **Then** it fails before detecting anything, writes nothing, says to use `ai-dev setup apply`, and exits 2.

---

### User Story 3 - Configure without an assistant (Priority: P2)

A developer or a script writes the configuration in one command, with every choice explicit, for example to reproduce a team setting or in automation.

**Why this priority**: Scripts and reproducible setups need it, but the assistant covers the person at a terminal first.

**Independent Test**: Run `ai-dev setup apply` with complete, incomplete and contradictory options, with and without an existing configuration, and check the outcomes, exit codes and the file.

**Acceptance Scenarios**:

1. **Given** `ai-dev setup apply --agent claude-code --agent codex-cli --role specification=claude-code --role planning=claude-code --role implementation=codex-cli --role review=claude-code --role remediation=codex-cli` in a project without a configuration, **Then** the configuration is written and the outcome is `configuration_written`. Exit code 0.
2. **Given** an option set that leaves a role without an agent, assigns a role twice, assigns a role to an agent that is not chosen, names an agent outside the catalog or a role outside the five, **Then** the outcome is `configuration_rejected` with every reason, nothing is written, and the exit code is 1.
3. **Given** an existing configuration that differs from the requested one, **When** `--replace` is absent, **Then** the outcome is `configuration_exists`, nothing is written, and the exit code is 1. With `--replace`, it is replaced.
4. **Given** a chosen agent that is unavailable, **Then** the configuration is written and the report carries a warning naming it.

---

### User Story 4 - Role selection and guidance follow the project's assignment (Priority: P1)

Once a project has a configuration, `ai-dev agent select <role>` and `ai-dev workflow guide` use it, so the team's choice takes effect without any other step.

**Why this priority**: Without it, a configuration would have no effect.

**Independent Test**: With no configuration, a configuration that differs from the built-in assignment, and an invalid configuration, run `agent select` for each role and `workflow guide` for a feature at each step. Compare outcomes and sources.

**Acceptance Scenarios**:

1. **Given** no configuration, **Then** `agent select` and `workflow guide` report exactly what they report today, plus the source `built_in`.
2. **Given** a valid configuration that assigns `implementation` to `claude-code`, **When** `ai-dev agent select implementation` runs, **Then** the outcome is `role_selected` with agent `claude-code` and source `project` when Claude Code is available, and `agent_unavailable` with source `project` when it is not.
3. **Given** an invalid configuration, **Then** `agent select` reports outcome `configuration_invalid` with exit code 1, and `workflow guide` reports the agent availability `configuration_invalid` without naming an agent. Neither falls back to the built-in assignment.

### Edge Cases

- **Configuration file present but empty, not JSON, of an unknown version, with unknown or missing fields, or over the size limit**: `configuration_invalid`, listing the reason. ai-dev never repairs, migrates or rewrites it on its own.
- **The configuration file or its directory is a symbolic link, or not a regular file or directory**: `configuration_invalid` when reading; `persistence_failed` when writing, and nothing is written through the link.
- **The configuration names an agent that was in the catalog of a newer ai-dev**: `configuration_invalid`, naming the unknown agent. It is never ignored or dropped.
- **Two `setup` writes in the same project at the same time**: each replaces the file atomically. The file always holds one complete configuration, the last one written. Coordinating concurrent writers beyond that is out of scope.
- **The project directory is not writable, the filesystem cannot replace a file atomically, or the platform is unsupported**: `persistence_failed`, exit 1, and the previous configuration, if any, is untouched.
- **A configuration in the project is not committed to Git**: it still takes effect. ai-dev never stages or commits it.
- **A chosen agent has no role**: allowed; it stays in the chosen list.
- **`--format json` with the assistant**: a usage failure (exit 2). The assistant is text only.
- **`ai-dev setup show` outside a project with a configuration**: there is no project concept beyond the current directory, so it reports `not_configured`.

## Requirements *(mandatory)*

### Functional Requirements

**Catalog and detection**

- **F011-FR-001**: The system MUST detect agents only from its built-in agent catalog, the supported-agent registry of Feature 002. Detection MUST determine availability exactly as 002 FR-002 does, by locating the agent's executable, and MUST NOT start the agent, run it with any argument, read its configuration or credentials, or contact the network.
- **F011-FR-002**: The system MUST NOT detect, list or run any executable outside the catalog, by name, pattern or any other heuristic.
- **F011-FR-003**: Adding an agent to the catalog MUST make it appear in detection, in the assistant, in `setup apply` and in the configuration, without changing any command, option, outcome or report structure of this feature (consistent with 002 FR-021).

**Project configuration**

- **F011-FR-004**: The project configuration MUST be one file at `.ai-dev/config.json` in the current directory. It MUST hold exactly:
  - a schema identifier, `ai-dev.config/v1`;
  - the list of chosen agents: one or more distinct catalog agent identifiers;
  - the role assignment: exactly one chosen agent for each of the five roles of 003 (`specification`, `planning`, `implementation`, `review`, `remediation`).
- **F011-FR-005**: A configuration MUST be valid only when it is a regular file, not a symbolic link, inside a `.ai-dev` directory that is not a symbolic link; it is at most 64 KiB; it is well-formed JSON with exactly the fields of F011-FR-004 and no other; its schema identifier is `ai-dev.config/v1`; every agent identifier is in the catalog; and every role is assigned to a chosen agent. Matching is exact and case-sensitive, with no aliasing or normalization. Any other content MUST be reported as `configuration_invalid` with each reason.
- **F011-FR-006**: The configuration MUST NOT be able to name an executable, a path, a command, an argument, an environment variable, a URL or any other value outside the closed sets of catalog identifiers and role names. A configuration received with a cloned repository can therefore choose only among agents ai-dev already knows how to run.
- **F011-FR-007**: Writing a configuration MUST replace the file atomically: a reader sees the previous complete file or the new complete file, never a partial one. The system MUST create `.ai-dev/` if it is missing, MUST NOT write through or replace a symbolic link, and MUST NOT write any other file. Writing MUST be supported on macOS and Linux local filesystems; elsewhere, or when atomic replacement cannot be established, the outcome MUST be `persistence_failed` and nothing is written. The written file MUST be byte-identical for the same configuration (fixed field order, agents in catalog order, roles in the order of F011-FR-004, a trailing newline).
- **F011-FR-008**: The system MUST NOT stage, commit or otherwise touch Git for the configuration.

**Commands**

- **F011-FR-009**: `ai-dev setup show [--format text|json]` MUST report every catalog agent with its availability and, when a valid configuration exists, whether it is chosen; the outcome (`not_configured`, `configured` or `configuration_invalid` with reasons); and the role assignment in effect with its source (`built_in` or `project`), except for `configuration_invalid`, which reports none. A chosen, unavailable agent MUST carry a warning. It MUST write nothing.
- **F011-FR-010**: `ai-dev setup` MUST run an interactive assistant that requires an interactive terminal on standard input and standard output. Without one it MUST fail before detection, write nothing, name `ai-dev setup apply` as the alternative, and exit 2. The assistant MUST:
  - list every catalog agent with its availability;
  - ask which agents to choose, at least one;
  - ask for the agent of each of the five roles, in the order of F011-FR-004, offering only chosen agents;
  - start from the existing configuration when it is valid, and from no preselection otherwise;
  - warn, without refusing, when a chosen agent is unavailable;
  - show the complete resulting configuration and ask for confirmation, defaulting to no.

  Only an explicit yes MAY write. Any other answer, end of input or an interrupt MUST write nothing and report `setup_cancelled`. An invalid existing configuration MUST be reported and the assistant MUST start with no preselection; the file is replaced only after confirmation.
- **F011-FR-011**: `ai-dev setup apply` MUST take the chosen agents as one or more `--agent AGENT` options and the role assignment as exactly five `--role ROLE=AGENT` options, plus `--replace` and `--format text|json`. It MUST NOT prompt. Its outcomes MUST be:
  - `configuration_written` when it writes a new or replacing configuration;
  - `configuration_unchanged` when the requested configuration equals the existing one, and nothing is written;
  - `configuration_rejected` when the options do not form a valid configuration under F011-FR-005, listing every reason, and nothing is written;
  - `configuration_exists` when a different configuration, valid or not, exists and `--replace` is absent, and nothing is written;
  - `persistence_failed` when writing fails, leaving any previous file untouched.

  A chosen, unavailable agent MUST NOT prevent writing and MUST appear as a warning.
- **F011-FR-012**: Exit codes MUST follow the 0/1/2 convention:
  - 0 for `not_configured`, `configured`, `configuration_written` and `configuration_unchanged`;
  - 1 for `configuration_invalid`, `configuration_rejected`, `configuration_exists`, `setup_cancelled` and `persistence_failed`;
  - 2 for usage failures and output failures.
- **F011-FR-013**: No command of this feature MAY start a coding agent, change an agent's permissions, sandbox, approval behavior, authentication or configuration, or contact the network. Detection and writing happen only inside the explicit `setup` command a person runs; no other command, installation step or first use MAY trigger them.
- **F011-FR-014**: In JSON mode, standard output MUST hold exactly one JSON object, with schema identifier `ai-dev.setup/v1`. Warnings and prompts MUST NOT appear on standard output in JSON mode. No output MAY contain a credential, token, secret or environment value; agent executable paths MUST NOT be reported.

**Effect on role selection and guidance**

- **F011-FR-015**: When a valid project configuration exists, role selection (003) and guidance (007) MUST use its role assignment instead of the built-in one, with the same availability rules. Without a configuration they MUST use the built-in assignment of 003 FR-003, with every outcome unchanged.
- **F011-FR-016**: When the project configuration is invalid, role selection MUST report the outcome `configuration_invalid` (exit 1), and guidance MUST report the agent availability `configuration_invalid` without naming an agent. Neither MAY fall back to the built-in assignment, guess an agent, or modify the file.
- **F011-FR-017**: Role-selection reports and guidance reports MUST state the source of the assignment, `built_in` or `project`, in text and in JSON.
- **F011-FR-018**: `selection_inconsistent` (003 FR-009) MUST remain reserved for a mismatch between the built-in assignment and the catalog. A project configuration naming an unknown agent is `configuration_invalid`, never `selection_inconsistent`.
- **F011-FR-019**: `ai-dev agents` and `ai-dev agent run` MUST remain unchanged. Choosing agents MUST NOT restrict which catalog agent a person can run explicitly.

**Help and completion**

- **F011-FR-020**: `setup`, `setup show` and `setup apply` MUST appear in the command catalog of Feature 010, so they have per-command help and shell completion, including completion of agent identifiers and `ROLE=` prefixes. The top-level usage line MUST become `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|version|help|completion> [options]`, amending F010-FR-010.

### Required Amendments

**Approved and applied (2026-11-21)** with this specification. Each amended specification records the decision in a Clarifications session dated 2026-11-21. Until Feature 011 is implemented, the amended features behave as before, because no project configuration can exist yet.

| # | Specification | Change |
|---|---|---|
| A1 | 003 Agent Role Selection | FR-003 and the Selection Policy: the built-in assignment applies when the project has no configuration; a valid 011 configuration replaces it. FR-011: the configuration joins the determinism inputs. FR-026: the 011 configuration is the one permitted mapping-management capability. New outcome `configuration_invalid` and a `source` field in text and JSON reports (F011-FR-016, F011-FR-017). |
| A2 | 007 Workflow Orchestration | F007-FR-004: the mapped agent comes from the assignment in effect (F011-FR-015); new availability value `configuration_invalid`; the report states the source. F007-FR-009: the configuration joins the determinism inputs. |
| A3 | 010 Command Help and Completion | F010-FR-010: the top-level usage line gains `setup` (F011-FR-020). |
| A4 | 009 CLI Help and Version | The general help lists `setup`, `setup show` and `setup apply`. |
| — | 002 Coding Agent Execution | No requirement changes. FR-021 already allows new catalog agents; F011-FR-019 keeps listing and execution unchanged. |

### Key Entities

- **Agent Catalog**: the built-in, ordered set of agents ai-dev knows how to detect and run (002 registry). Each entry has a canonical identifier, a display label and an executable name. Fixed at build time.
- **Agent Detection Result**: for each catalog agent, its availability now (`available` or `unavailable`). Never persisted.
- **Project Configuration**: the chosen agents and the agent for each role, with a schema identifier, held in `.ai-dev/config.json`. Contains only catalog identifiers and role names.
- **Role Assignment in Effect**: the mapping from each role to one agent, with its source: `project` when a valid configuration exists, `built_in` when none exists, and none when the configuration is invalid.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can see every detected agent and the role assignment in effect with one read-only command, and configure a project with the assistant in under 2 minutes.
- **SC-002**: Across tests for every command and outcome, 0 coding-agent processes are started and 0 network connections are made.
- **SC-003**: In 100% of cancellation paths (declining, end of input, interrupt, no terminal), the project directory is byte-identical before and after.
- **SC-004**: 100% of invalid configurations in a fixture set covering each rule of F011-FR-005 are reported as `configuration_invalid` with the matching reason, and none leads to a selected agent.
- **SC-005**: Without a configuration, the regression suites of Features 001–010 pass with only the expectations the Required Amendments name updated: the `source` field, the top-level usage line, the general help and the catalog.
- **SC-006**: Two runs of `setup apply` with the same options produce byte-identical files, and the second reports `configuration_unchanged`.
- **SC-007**: A configuration naming an agent outside the catalog is never run: 100% of such cases are `configuration_invalid`.

## Assumptions

- The project is the current directory, as for every other ai-dev command. Looking for `.ai-dev/` in parent directories is out of scope.
- A per-user configuration, and a user default that a project overrides, are out of scope (owner decision: per project).
- Adding a new agent to the catalog (for example another coding CLI) is a separate decision with its own specification of how that agent is run safely; this feature only makes detection and configuration ready for it.
- ai-dev is distributed as a single binary with no installation step, so "on installation" means "when a person runs `ai-dev setup`". Package-manager hooks are out of scope.
- The configuration file is meant to be committed; ai-dev does not decide that for the team (F011-FR-008).
- Who may change the configuration is governed by the project's own review process (pull requests), not by ai-dev. A configuration change is visible in Git history.
- Text output is English, like the rest of the CLI.
