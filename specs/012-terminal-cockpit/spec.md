# Feature Specification: Terminal Cockpit

**Feature Branch**: `feature/012-terminal-cockpit`

**Feature ID**: `012`

**Created**: 2026-11-21

**Status**: Approved for planning. Derived from the owner's draft "Feature 012 — Terminal Cockpit TUI for ai-dev" (dated 2026-11-20), after a review against the repository on 2026-11-21. The owner authorized drafting on 2026-11-21 ("si, abre la rama y escribe la especificación de la 012") and approved it as written, including Required Amendments A1 and A2, the same day ("si, apruebo la 012, haz el commit y ejecuta /speckit-plan"). This is a declared, unauthenticated assertion. Implementation still needs the plan and tasks.

**Input**: ai-dev is a set of commands. Each is good for automation, but a person has to remember which command answers which question, run several to see where a project stands, and copy identifiers between them. The cockpit is one interactive terminal workspace where a person sees the state of the project and its features at a glance, finds every available action from the keyboard, and starts any of them, without weakening any rule the commands already enforce.

## Clarifications

### Session 2026-11-21

- **Authority record.** German Munoz, repository owner, made the decisions below on 2026-11-21, choosing among options presented with alternatives after the review. This is a declared, unauthenticated assertion.
- **Review findings that shaped this specification.** The draft assumed capabilities that do not exist, and they are out of scope here rather than invented by the cockpit:
  - Feature 007 has only its guided, read-only mode: there are no orchestration sessions, plans, checkpoints, budgets, pause or resume.
  - Feature 006 has no queue of pending gates, no gate authentication and no expiry: an approval or abandonment is an explicit command with a declared approver.
  - Feature 008 covers repository initialization and commit-message validation only: there is no branch, status, diff, commit, push, merge or Git Flow finish capability.
  - Feature 002 runs one agent invocation in the foreground and keeps no execution history or evidence store; it requires the agent's output and interactions to reach the developer unchanged.
  - Feature numbers 009, 010 and 011 are taken; the Neovim integration the draft placed in 010 is a future, unnumbered feature.
- Q: How does the cockpit perform an action that changes something or starts an agent? → A: **It hands the terminal to the existing ai-dev command.** The cockpit suspends itself, runs the same command a person would type, in the real terminal, waits for the person to return, and refreshes. Read-only views evaluate the existing capabilities directly. The cockpit therefore enforces no rule of its own and duplicates none, every action leaves exactly the evidence the command leaves, the agent's output reaches the person unchanged (Feature 002), and interactive assistants work as they do today.
- Q: What does the first version cover? → A: **A cockpit over the existing capabilities**: read-only views of the dashboard, environment, features, agents and repository initialization, help, a command palette, and the existing actions launched through their commands. Everything listed under Out of Scope is excluded.
- Q: Can a person approve or abandon a feature from the cockpit? → A: **Yes, through the command.** The person types the approver, the target state and the reason; the cockpit shows the exact command and runs it only after an explicit confirmation. No key approves by itself.
- **Planning constraint (not a user-visible requirement).** The cockpit is built on Bubble Tea and Lip Gloss, pinned to exact versions. They are the project's first third-party dependencies; the plan must justify them under constitution principles III and V and add a dependency vulnerability check to continuous integration.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Open the cockpit and find my way (Priority: P1)

A developer opens one workspace, sees what the project looks like, moves between areas with the keyboard, asks for help, and leaves with the terminal exactly as it was.

**Why this priority**: Every other story happens inside this workspace.

**Independent Test**: In a pseudo-terminal, open the cockpit, visit every area, open the help and the palette, resize the window, and quit by each supported path; check the terminal state is restored.

**Acceptance Scenarios**:

1. **Given** an interactive terminal of at least 80 columns and 24 rows, **When** `ai-dev tui` runs, **Then** the dashboard appears with a summary of every area and the keys to reach them.
2. **Given** any area, **When** the person presses `?`, **Then** the keys of that area and the global keys are listed; `Esc` closes the help.
3. **Given** any area, **When** the person opens the palette (`:` or `Ctrl-P`) and types part of a name, **Then** matching areas and actions are listed; an action that is not available is shown with its reason and cannot be started.
4. **Given** a terminal smaller than 80×24, **Then** the cockpit shows a short notice with the current and the minimum size instead of a broken layout, and recovers when the window grows.
5. **Given** the person quits with `q`, or the cockpit receives an interrupt or termination signal, or fails, **Then** the terminal returns to its previous mode, screen and cursor.
6. **Given** standard input or standard output is not a terminal, **When** `ai-dev tui` runs, **Then** it writes a refusal naming the non-interactive commands, changes nothing, and exits 2.

---

### User Story 2 - See where every feature stands (Priority: P1)

A developer sees each feature of the project with its workflow state, the recommended next step, and the agent that step would use, and opens one feature for details.

**Why this priority**: It replaces running `workflow state`, `workflow next` and `workflow guide` for every feature by hand, which is the main reason for the cockpit.

**Independent Test**: With fixture projects covering every Workflow State, missing artifacts, an invalid record, a terminal feature and an invalid agent configuration, compare each row and detail view with the output of the corresponding commands.

**Acceptance Scenarios**:

1. **Given** a project with features under `specs/`, **When** the Features area opens, **Then** each feature is listed with its Workflow State (Feature 004), its outcome and next activity (Feature 005), and the role, agent, availability and assignment source of that activity (Features 007 and 011).
2. **Given** a selected feature, **When** the person opens it, **Then** the detail shows the same facts plus the missing artifacts, if any, and the actions the guidance offers for that state.
3. **Given** a feature whose record is invalid, whose artifacts are inconsistent or which is unknown, **Then** it shows that outcome exactly, and no next step, agent or action is invented.
4. **Given** the files change while the cockpit is open, **When** the person refreshes (`r`) or returns from an action, **Then** every view is evaluated again, and the time of the last evaluation is visible.

---

### User Story 3 - Start an agent for the next step (Priority: P1)

From a feature, a developer starts the agent the next step needs, gives it an instruction, watches it work in the real terminal, and returns to the cockpit.

**Why this priority**: Running agents is the core of the workflow the cockpit supports.

**Independent Test**: With fake agents on the search path, start an agent from the cockpit, check the command run, its arguments, that its output reached the terminal unchanged, that the cockpit waited for the person, and that it refreshed.

**Acceptance Scenarios**:

1. **Given** a feature whose next step maps to an available agent, **When** the person chooses "run agent", **Then** the cockpit asks for the instruction, shows the exact `ai-dev agent run` command it will run, and runs it only after an explicit confirmation.
2. **Given** the command runs, **Then** the cockpit has handed over the terminal: the agent's output and any interaction it asks for reach the person unchanged, and an interrupt affects that command as it would outside the cockpit.
3. **Given** the command ends, **Then** the cockpit shows its exit code, waits for a key so the output can be read, then restores its own screen and refreshes.
4. **Given** the agent of the next step is unavailable, or the assignment is invalid, **Then** "run agent" is shown unavailable with that reason.
5. **Given** an instruction that begins with `-` or contains any characters, **Then** it reaches the agent as one unmodified instruction, never as an option or as several words.

---

### User Story 4 - Record a human decision (Priority: P1)

A developer who has finished an activity records an approval, or abandons a feature, from the cockpit, with the same rules and evidence as on the command line.

**Why this priority**: Recording decisions is the step that moves a feature forward; without it the cockpit can only watch.

**Independent Test**: Approve and abandon fixture features from the cockpit and compare the resulting `workflow-state.json`, output and exit code with the same commands typed by hand; check that no path writes without the explicit confirmation.

**Acceptance Scenarios**:

1. **Given** a non-terminal feature, **When** the person chooses "approve", **Then** the cockpit asks for the approver, the target state and an optional reason, with no value prefilled, shows the exact `ai-dev workflow approve` command, and runs it only after an explicit confirmation.
2. **Given** "abandon", **Then** the reason is required, and the same review and confirmation apply.
3. **Given** the command refuses (an invalid transition, missing artifacts, a changed state, a persistence failure), **Then** the cockpit shows the command's own outcome after it returns and never reports success itself.
4. **Given** any key or sequence of keys, **Then** no approval or abandonment happens without the review and the explicit confirmation of that one decision.

---

### User Story 5 - Check the environment, agents and repository (Priority: P2)

A developer sees whether the environment is ready, which agents are installed and which agent each role uses, and what an initialization of the repository would find, and starts the corresponding assistants.

**Why this priority**: Useful when setting a project up or diagnosing it, but less frequent than daily feature work.

**Independent Test**: With fixture environments, compare the Environment, Agents and Repository areas with `ai-dev doctor`, `ai-dev setup show` and `ai-dev init inspect`, and start each assistant through the cockpit.

**Acceptance Scenarios**:

1. **Given** the Environment area, **Then** it shows the same checks and readiness as `ai-dev doctor`; a slow check shows a progress indicator and never blocks the keyboard.
2. **Given** the Agents area, **Then** it shows the same agents, availability, chosen agents, role assignment and source as `ai-dev setup show`, including an invalid configuration and its reasons.
3. **Given** the Repository area, **Then** it shows the same findings as `ai-dev init inspect`, and makes no change.
4. **Given** the person chooses "configure agents", "initialize repository" or "validate commit message", **Then** the cockpit runs `ai-dev setup`, `ai-dev init` or `ai-dev git commit validate` through the same hand-over as User Story 3.

### Edge Cases

- **The window shrinks below 80×24 during a review or a form**: the size notice replaces the view; nothing is confirmed or lost, and the form returns when the window grows.
- **A confirmation key is pressed repeatedly**: one confirmation starts one command; further presses while it runs, or after it returns, start nothing.
- **A feature name, path, branch name, agent configuration value or check message contains escape sequences, terminal hyperlinks, clipboard sequences, bidirectional overrides or control characters**: they are shown as inert visible text and never reach the terminal as instructions.
- **The project directory disappears or becomes unreadable**: views report that they cannot be evaluated; the cockpit does not crash and does not show stale data as current.
- **An interrupt arrives while the cockpit owns the terminal**: the cockpit quits and restores the terminal. **An interrupt arrives while a command runs**: it reaches that command, which handles it as it always does, and the cockpit resumes afterwards.
- **The command started from the cockpit cannot be launched**: the cockpit reports the failure after restoring its screen; nothing else happens.
- **No feature exists under `specs/`**: the Features area says so and explains how features are created.
- **A terminal without colour or without Unicode**: every status is also given as a word, so meaning never depends on colour or on symbols alone.
- **The 006 FR-033 residual**: an agent the person runs can itself run `ai-dev workflow approve`. The cockpit neither adds nor removes this risk; Feature 006 documents it.

## Requirements *(mandatory)*

### Functional Requirements

**Entry and terminal**

- **F012-FR-001**: `ai-dev tui` MUST open the cockpit when standard input and standard output are interactive terminals. Otherwise, or with any argument, it MUST change nothing, write a refusal or its usage line to standard error, and exit 2. Leaving the cockpit normally MUST exit 0.
- **F012-FR-002**: The cockpit MUST restore the terminal's mode, screen and cursor when it quits, when it fails, and when it receives an interrupt or termination signal while it owns the terminal.
- **F012-FR-003**: Below 80 columns or 24 rows, the cockpit MUST show a notice with the current and minimum size instead of the layout, and MUST resume when the window is large enough. No input may confirm an action while the notice is shown.

**Views (read-only)**

- **F012-FR-004**: The cockpit MUST provide these areas: Dashboard, Features, Environment, Agents, Repository and Help. The Dashboard MUST summarize the other areas.
- **F012-FR-005**: Every fact a view shows MUST be the result the existing capability produces for the same inputs: Workflow State (004), next step (005), guidance with role, agent, availability and source (007, 011), environment readiness (001), agent availability and assignment (002, 011) and repository inspection (008). The cockpit MUST NOT infer, combine or alter an outcome, and MUST show every outcome those capabilities can produce, including invalid, inconsistent, unknown and unavailable ones.
- **F012-FR-006**: Views MUST be evaluated when the cockpit opens, when the person refreshes, and after every action returns, and MUST show the time of their last evaluation. The cockpit MUST NOT poll in the background.
- **F012-FR-007**: Producing a view MUST NOT write any file, start any coding agent, or contact the network. Evaluating a view MUST NOT block keyboard input; a view still being evaluated MUST say so.

**Actions (through the existing commands)**

- **F012-FR-008**: Every action that changes state or starts an agent MUST run the existing ai-dev command, with the person's terminal handed over to it for its whole run. The actions are:
  - run the agent of a feature's next step (`ai-dev agent run`);
  - approve a feature (`ai-dev workflow approve`) and abandon a feature (`ai-dev workflow abandon`);
  - configure the project's agents (`ai-dev setup`);
  - initialize the repository (`ai-dev init`);
  - validate a commit message (`ai-dev git commit validate`).
- **F012-FR-009**: Before running an action, the cockpit MUST collect the values the command needs from the person, with no value of a human decision prefilled (approver, target state, reason, instruction), MUST show the exact command, and MUST run it only after an explicit confirmation of that one action. Values MUST be passed as separate arguments, unmodified, so that none can be read as an option or split into several.
- **F012-FR-010**: After the command ends, the cockpit MUST show its exit code, wait for a key, restore its screen and refresh every view. The cockpit MUST NOT report an outcome other than the command's own.
- **F012-FR-011**: An action MUST be offered only when its command can apply: running an agent only for a non-terminal feature whose next step maps to an available agent under a valid or absent assignment; approving and abandoning only for a recognized, non-terminal feature. An unavailable action MUST be shown with its reason and MUST NOT start.
- **F012-FR-012**: One confirmation MUST start at most one command. Input received while a command runs or before the cockpit has restored its screen MUST NOT start another.

**Navigation**

- **F012-FR-013**: Every area and action MUST be reachable by keyboard alone. The default keys MUST be: arrows and `j`/`k` to move, `Enter` to open or choose, `Tab`/`Shift-Tab` to change area, `/` to filter the current list, `:` or `Ctrl-P` for the palette, `?` for help, `r` to refresh, `Esc` to go back or close, and `q` to quit from the top level. No single key MUST approve, abandon or start an agent.
- **F012-FR-014**: The palette MUST list every area and every action, filter them by the typed text, show each action's availability and reason, and open the same review as the area does.
- **F012-FR-015**: Returning from an action or from the help MUST bring the person back to the area and item they left.

**Safety and boundaries**

- **F012-FR-016**: Every string the cockpit renders that comes from files, the environment or command results MUST be rendered as inert text: control characters, escape and operating-system-command sequences, terminal hyperlinks and bidirectional overrides MUST be shown visibly escaped and never sent to the terminal as instructions.
- **F012-FR-017**: The cockpit MUST hold no authority of its own. It MUST NOT write any file, keep any configuration or state between runs, or record any decision except by running the commands of F012-FR-008.
- **F012-FR-018**: No output MAY contain a credential, token, secret or environment value.
- **F012-FR-019**: Every status MUST be conveyed by a word as well as by any colour or symbol.
- **F012-FR-020**: `tui` MUST appear in the command catalog of Feature 010 with per-command help and shell completion. The top-level usage line MUST become `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|version|help|completion> [options]`, amending F010-FR-010, and the general help MUST list `tui`, amending F009-FR-003. Every other command's behavior, output and exit codes MUST remain unchanged.

### Required Amendments

**Approved and applied (2026-11-21)** with this specification. Each amended specification records the decision in a Clarifications session dated 2026-11-21. Until Feature 012 is implemented, the amended features behave as before.

| # | Specification | Change |
|---|---|---|
| A1 | 010 Command Help and Completion | F010-FR-010: the top-level usage line gains `tui` (F012-FR-020) |
| A2 | 009 CLI Help and Version | F009-FR-003 and F009-FR-005: the general help and the top-level usage list `tui` |
| — | 002, 004–008, 011 | No change: views use their capabilities unchanged, and actions run their commands |

### Key Entities

- **Area**: one screen of the cockpit (Dashboard, Features, Environment, Agents, Repository, Help), with its list, its detail and the actions it offers.
- **Action**: one existing command the cockpit can run, with a name, whether it changes state or starts an agent, its availability and reason, the values it needs from the person, and the exact command line it produces.
- **View State**: the area, the selected item, the filter and the time of the last evaluation. It exists only while the cockpit runs, and holds no domain fact or authority.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With a project of 50 features, the cockpit responds to each navigation key within 50 ms, and the Features area is complete within 2 seconds of opening.
- **SC-002**: For 100% of the fixture states of Features 001, 002, 004, 005, 007, 008 and 011, each view shows exactly the outcome the corresponding command reports for the same inputs.
- **SC-003**: For 100% of the actions, the command the cockpit runs is identical, argument for argument, to the command shown in its review, and its effects, evidence and exit code equal those of the same command typed by hand.
- **SC-004**: Across every exit path (quit, failure, interrupt, termination signal) in a pseudo-terminal, the terminal's mode, screen and cursor are restored in 100% of runs.
- **SC-005**: For a fixture set of hostile strings (escape, operating-system-command, hyperlink, clipboard, bidirectional and control sequences) in feature names, paths, configuration values and check messages, 0 reach the terminal as instructions.
- **SC-006**: In tests that press every key and combination of the default keymap in every area, 0 approvals, abandonments or agent runs happen without the review and explicit confirmation of that action.
- **SC-007**: A person can go from opening the cockpit to running the agent of a feature's next step in under 30 seconds, without typing a command.
- **SC-008**: The regression suites of Features 001–011 pass, with only the expectations named in the Required Amendments updated.

## Assumptions

- The project is the current directory, as for every other ai-dev command, and its features are the directories under `specs/` that Features 004 and 005 recognize.
- Supported platforms are macOS and Linux terminals with at least 16 colours; the cockpit remains usable without colour or Unicode (F012-FR-019). Windows is not supported: without terminal detection, `ai-dev tui` refuses (F012-FR-001).
- The cockpit is in English, like the rest of the CLI.
- Out of scope, each because no capability exists to present without inventing one: execution history, evidence browsing and detaching from a running agent (Feature 002 keeps none); autonomous orchestration sessions (deferred by Feature 007); Git status, diff, branches, commit, push, merge and Git Flow finish (they need their own domain feature, with the process boundary of Feature 008); a pending-gate queue and gate authentication (Feature 006 has neither). Also out of scope: themes, layouts and configurable keys; an embedded editor; the Neovim/LazyVim integration, a future feature that must not depend on the cockpit's internals.
- `ai-dev setup apply` and the other non-interactive forms remain available on the command line; the cockpit offers the interactive assistant for each task.
