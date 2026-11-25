# Feature Specification: Neovim Integration

**Feature Branch**: `feature/014-neovim-integration`

**Feature ID**: `014`

**Created**: 2026-11-24

**Status**: Approved for planning. Derived from the owner's draft "Feature 014 — Bidirectional Neovim/LazyVim Integration" (dated 2026-11-20), after a review against the repository on 2026-11-24. The owner authorized drafting on 2026-11-24 ("si, abre la rama y escribe la especificación de la 014"). The owner approved it as written, including the command `ai-dev open` and Required Amendments A1–A3, the same day ("si, apruebo la 014, haz el commit y ejecuta /speckit-plan"). This is a declared, unauthenticated assertion. Implementation still needs the plan and tasks.

**Input**: A developer who writes code in Neovim has to leave the editor to see where a feature stands or to start an agent, and has to copy paths and line numbers by hand to tell the agent what to look at. From the ai-dev side, the cockpit can show a feature but cannot take the developer to its files. This feature connects the two in both directions, over the capabilities ai-dev already has: from Neovim, see the project and the features, open their artifacts, and start the existing actions with an explicit, bounded reference to the code; from ai-dev, open a project file at a line in Neovim. ai-dev stays the source of truth for the workflow, the agents and the human decisions; Neovim stays the editor.

## Clarifications

### Session 2026-11-24

- **Authority record.** German Munoz, repository owner, made the decisions below on 2026-11-24, choosing among options presented with alternatives after the review. This is a declared, unauthenticated assertion.
- **Review findings that shaped this specification.** The draft assumed capabilities that do not exist; they are out of scope here rather than invented by the integration:
  - The draft's "Feature 009 Terminal Cockpit" and its TUI-001–006 contracts are Feature 012, which has no public contracts beyond its commands. Feature 009 is CLI help and version. Feature 012 recorded the Neovim integration as a future feature that must not depend on the cockpit's internals.
  - Feature 002 runs one agent invocation in the foreground and keeps no execution history, evidence, operation identifiers or remote cancellation. It requires the agent's output and interactions to reach the developer unchanged and live, so a captured or rewritten agent-output panel is excluded.
  - Feature 007 has only its guided, read-only mode: no sessions, budgets, pause or resume.
  - Feature 006 has no queue of pending gates, no expiry and no approver authentication; the approver is an auditable assertion.
  - ai-dev produces no diffs, suggested patches or diagnostics: agents edit files directly, under their own sandbox.
  - There is no capability registry. The nearest is the command templates of `ai-dev workflow guide --format json` (schema `ai-dev.workflow-guide/v1`).
- Q: What does the first version cover? → A: **The integration over the existing capabilities.** A Neovim plugin reads the existing JSON outputs (features' state, next step and guidance, environment, agents), opens a feature's artifacts, sends a file or line range as a reference inside the instruction of `ai-dev agent run` after a preview and confirmation, and runs `ai-dev agent run`, `ai-dev workflow approve` and `ai-dev workflow abandon` in a Neovim terminal with the same command a person would type. From ai-dev, a project file opens at a line in Neovim. Operations with status, remote cancellation, diffs, suggested patches, session panels and gate queues are out of scope.
- Q: How does ai-dev reach Neovim? → A: **The parent instance, or a foreground editor.** When ai-dev runs inside a Neovim terminal, it uses the instance that started that terminal, through Neovim's remote interface with structured arguments, never through a command string. Otherwise it starts the editor in the foreground and hands the terminal over to it, as the cockpit does for its actions. It never searches for other instances and never starts an editor the person cannot see.
- Q: What does a reference to code contain by default? → A: **Metadata only**: the path relative to the project, the line range and a digest of the saved file. The agent reads the file itself. A buffer with unsaved changes is refused until it is saved. The selected text is included only when the person chooses it and confirms it in the preview.
- Q: Where does the plugin live? → A: **In this repository**, in its own directory, versioned and released with ai-dev, installable with lazy.nvim from the repository, and tested in headless Neovim in continuous integration. A LazyVim preset is optional; the plugin works without LazyVim.
- **Specification decision, approved by the owner on 2026-11-24 with this specification.** ai-dev reaches Neovim through one new command, `ai-dev open`, and the cockpit's "open in editor" action runs that command through its existing hand-over (F012-FR-008). The cockpit therefore keeps running only ai-dev commands, and the same path is available from the command line and to the plugin.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Open a project file in Neovim from ai-dev (Priority: P1)

A developer in ai-dev, on the command line or in the cockpit, opens one of the project's files at a line in Neovim: in the Neovim that hosts the terminal if there is one, otherwise in a Neovim that takes over the terminal until the developer leaves it.

**Why this priority**: It is the ai-dev → Neovim direction of the integration and the smallest slice that works on its own.

**Independent Test**: With a fixture project and a headless Neovim started as the parent of a terminal, run `ai-dev open` with valid and hostile paths, and check which file and line the parent instance shows; without a parent, check in a pseudo-terminal that the editor runs in the foreground with the same arguments.

**Acceptance Scenarios**:

1. **Given** ai-dev runs in a terminal of a Neovim instance, **When** `ai-dev open --line 12 -- src/main.go` runs, **Then** that instance shows `src/main.go` at line 12 in a window other than the terminal, focuses it, and the command exits 0 at once.
2. **Given** ai-dev does not run inside Neovim and standard input and output are terminals, **When** the same command runs, **Then** Neovim starts in the foreground on that file and line, owns the terminal until the person quits it, and the command exits 0 after it.
3. **Given** a path outside the project, through `..`, an absolute path or a symbolic link that resolves outside the project, a missing file or a directory, **Then** nothing opens, the outcome names the reason, and the command exits 1.
4. **Given** no parent instance and no interactive terminal, or no Neovim on the search path, **Then** nothing starts, the outcome explains what is missing, and the command exits 1; no editor is started in the background.
5. **Given** the cockpit shows a feature, **When** the person chooses "open in editor" on one of its artifacts, **Then** the cockpit runs `ai-dev open` for that artifact through its hand-over, as for its other actions.

---

### User Story 2 - See the project and its features from Neovim (Priority: P1)

A developer in Neovim lists the features with their state, next step and agent, the environment checks and the agents, and opens a feature's artifacts from that list.

**Why this priority**: It is the Neovim → ai-dev direction for reading, and every action of the plugin starts from these views.

**Independent Test**: With fixture projects covering every Workflow State, an unknown feature, an invalid record and an invalid agent configuration, compare each plugin view with the JSON output of the corresponding ai-dev command for the same project.

**Acceptance Scenarios**:

1. **Given** a project with features under `specs/`, **When** the person opens the features view, **Then** each feature is listed with its Workflow State, its next activity, and that activity's role, agent, availability and assignment source, exactly as `ai-dev workflow guide --format json` reports them.
2. **Given** the environment or agents view, **Then** it shows the same checks and readiness as `ai-dev doctor --format json`, and the same agents, availability and role assignment as `ai-dev setup show --format json`.
3. **Given** a feature in the list, **When** the person chooses one of its artifacts (specification, plan, tasks or another file in its directory), **Then** that file opens in the current Neovim.
4. **Given** an invalid, inconsistent or unknown outcome, **Then** the view shows it exactly; no next step, agent or action is invented.
5. **Given** ai-dev is missing, reports an incompatible version, or returns output the plugin cannot read, **Then** the view says so and offers nothing to run.

---

### User Story 3 - Ask an agent about the code I am looking at (Priority: P1)

A developer selects a file or a range of lines in Neovim, writes an instruction, sees exactly what will be sent and to which agent, confirms, and watches the agent work in a Neovim terminal.

**Why this priority**: Pointing an agent at specific code without copying paths by hand is the main value of the editor side.

**Independent Test**: With fake agents on the search path and files whose names and contents hold shell metacharacters, control sequences and Unicode, send references from saved and unsaved buffers, with and without the selected text, and compare the preview, the command run and the instruction the fake agent received.

**Acceptance Scenarios**:

1. **Given** a saved file inside the project and an optional range, **When** the person asks an agent, **Then** the plugin asks for the agent (by role or directly) and the instruction, with nothing prefilled, and shows a preview with the agent, the relative path, the range, the file digest, whether the selected text is included, the complete instruction, and the exact `ai-dev agent run` command.
2. **Given** the preview, **When** the person confirms, **Then** the command runs in a Neovim terminal, and the agent's output and any interaction it asks for reach the person unchanged; when the person declines, nothing runs.
3. **Given** a buffer with unsaved changes, a buffer without a file, or a file outside the project, **Then** the plugin refuses with the reason and sends nothing.
4. **Given** the person chooses to include the selected text, **Then** the preview shows that text and its size, and a selection above the documented limit is refused.
5. **Given** a file name, a selection or an instruction with spaces, quotes, `$`, backquotes, `;`, newlines or a leading `-`, **Then** the agent receives one unmodified instruction, and no part of it is run by a shell or read as an option.

---

### User Story 4 - Record a decision and run the next step from Neovim (Priority: P1)

A developer records an approval or abandons a feature, or runs the agent of a feature's next step, from Neovim, with the same rules and evidence as on the command line.

**Why this priority**: Without it the developer still has to leave the editor to move a feature forward.

**Independent Test**: Approve, abandon and run the next step for fixture features from the plugin, and compare the resulting `workflow-state.json`, output and exit code with the same commands typed by hand; press every plugin key and command without confirming and check that nothing is recorded.

**Acceptance Scenarios**:

1. **Given** a non-terminal feature, **When** the person chooses "approve", **Then** the plugin asks for the approver, the target state and an optional reason with no value prefilled, shows the exact `ai-dev workflow approve` command, and runs it in a Neovim terminal only after an explicit confirmation.
2. **Given** "abandon", **Then** the reason is required, and the same review and confirmation apply.
3. **Given** "run next step" for a feature whose next activity maps to an available agent, **Then** the plugin asks for the instruction and runs `ai-dev agent run` for that agent through the same review and confirmation.
4. **Given** the command refuses or fails, **Then** the plugin shows the command's own outcome and exit code, and never reports success itself.
5. **Given** any key, mapping or command of the plugin, **Then** no approval, abandonment or agent run happens without the review and explicit confirmation of that one action.

---

### User Story 5 - Install, configure and remove the plugin (Priority: P2)

A developer installs the plugin, optionally with the LazyVim preset, configures it, and can disable or remove it without leaving anything behind.

**Why this priority**: Needed to use the integration, but done once.

**Independent Test**: In clean headless Neovim configurations, one plain and one LazyVim at a pinned version, install the plugin by the documented steps, check its commands, keys and health check, then disable and remove it and check that nothing remains.

**Acceptance Scenarios**:

1. **Given** the documented lazy.nvim specification, **When** the person installs the plugin, **Then** its commands are available and its health check reports Neovim, ai-dev and their compatibility.
2. **Given** a plain configuration, **Then** the plugin defines its commands and no key mappings; **given** the LazyVim preset, **Then** it adds key mappings under one documented prefix, none of which replaces a LazyVim default mapping.
3. **Given** a configuration with an unknown option or an invalid value, **Then** the plugin reports it and uses the default for that option.
4. **Given** the plugin is disabled or removed, **Then** no command, mapping, autocommand or file of the plugin remains, and ai-dev and the cockpit behave as before.

### Edge Cases

- **`$NVIM` is set but its endpoint does not answer, is not a Neovim instance, or is not a local socket owned by the current user**: `ai-dev open` refuses with that reason and opens nothing; it does not fall back to the foreground editor, so a stale variable never starts an editor by surprise.
- **Several Neovim instances are running**: only the parent instance named by `$NVIM` is ever used; no other instance is searched for, so there is no ambiguity to resolve.
- **The line is beyond the end of the file, or the column beyond the end of the line**: the editor goes to the last line or column; a line or column below 1 is a usage error (exit 2).
- **The file changes between the preview and the confirmation**: the plugin computes the digest again on confirmation; if it differs, it refuses and shows the preview again.
- **A path, file name, feature name, check message or selected text contains escape sequences, terminal hyperlinks, bidirectional overrides or control characters**: plugin views and previews show them as inert visible text. The instruction sent to the agent carries the selected text unchanged, as the person confirmed it in the preview.
- **Long paths, spaces and Unicode in paths**: they are opened, shown and sent unchanged.
- **Neovim or the plugin is closed while an agent runs in its terminal**: the agent stops with the terminal, as it would if any terminal closed; ai-dev reports what Feature 002 reports, and the plugin claims no outcome it did not see.
- **The project directory differs from Neovim's working directory**: the plugin uses Neovim's working directory as the project, like the current directory for ai-dev; a file outside it is refused.
- **ai-dev returns a JSON schema version the plugin does not know**: the plugin refuses to interpret it and says which version it found and which it supports.
- **The 006 FR-033 residual**: an agent the person runs can itself run `ai-dev workflow approve`. The integration neither adds nor removes this risk; Feature 006 documents it.

## Requirements *(mandatory)*

### Functional Requirements

**ai-dev → Neovim**

- **F014-FR-001**: `ai-dev open [--line N] [--column N] [--format text|json] -- <path>` MUST open a project file in Neovim. Line and column MUST be integers of at least 1; a missing line or column means the start of the file or line.
- **F014-FR-002**: The path MUST resolve, after following every symbolic link, to an existing regular file inside the project directory. A path outside it, missing, or not a regular file MUST be refused with exit 1 and an outcome naming the reason. The path MUST be passed to the editor as one argument and MUST NOT be interpreted by a shell.
- **F014-FR-003**: When `$NVIM` names an endpoint, `ai-dev open` MUST use that instance only, through Neovim's remote interface with structured arguments, MUST open the file in a window other than the terminal that runs ai-dev, MUST focus it, and MUST exit without waiting for the person. It MUST refuse an endpoint that is not a local socket owned by the current user, that does not answer, or that is not Neovim, and MUST NOT then fall back to another editor.
- **F014-FR-004**: Without `$NVIM`, `ai-dev open` MUST start Neovim from the search path in the foreground when standard input and standard output are terminals, hand the terminal over to it, and exit after it ends. Otherwise, or when Neovim is not on the search path, it MUST refuse with exit 1 and start nothing.
- **F014-FR-005**: `ai-dev open` MUST NOT search for Neovim instances, start an editor in the background, or send the editor any command built from text.
- **F014-FR-006**: The cockpit MUST offer "open in editor" for each artifact of a feature it shows, and MUST run `ai-dev open` for that artifact through its existing hand-over (F012-FR-008 to F012-FR-012).

**Neovim → ai-dev: views**

- **F014-FR-007**: The plugin MUST provide views of the features (state, next activity, role, agent, availability, source), the environment checks and the agents, each produced only from the JSON output of the corresponding ai-dev command for Neovim's working directory, and MUST show every outcome those commands can produce, including invalid, inconsistent, unknown and unavailable ones, without inferring or altering any.
- **F014-FR-008**: Views MUST be produced when opened and when the person refreshes, MUST show the time of their last evaluation, and MUST NOT poll in the background or block editing while ai-dev runs.
- **F014-FR-009**: From the features view, the person MUST be able to open any file in that feature's directory in the current Neovim, after the same project and symbolic-link check as F014-FR-002.
- **F014-FR-010**: The plugin MUST check that ai-dev is on the search path (or at a configured path) and reports a compatible version and known JSON schema versions; otherwise it MUST say what it found and what it supports, and offer nothing to run.

**Neovim → ai-dev: references and actions**

- **F014-FR-011**: A code reference MUST contain the path relative to the project, the optional line range and the digest of the saved file, and MUST include the selected text only when the person chooses it in the preview. The plugin MUST refuse a buffer with unsaved changes, a buffer without a file, a file outside the project, and selected text above the documented size limit.
- **F014-FR-012**: Before running any action, the plugin MUST collect the values the command needs with no value of a human decision prefilled (agent, instruction, approver, target state, reason), MUST show a preview with the exact command and, for an agent run, the agent, the reference, whether text is included, and the complete instruction, and MUST run it only after an explicit confirmation of that one action. Declining MUST run nothing.
- **F014-FR-013**: The actions MUST be: run an agent with an instruction and an optional code reference (`ai-dev agent run`), run the agent of a feature's next step (`ai-dev agent run`), approve a feature (`ai-dev workflow approve`), abandon a feature (`ai-dev workflow abandon`), and open the cockpit (`ai-dev tui`). Each MUST run the ai-dev command in a Neovim terminal, so that the command's output and interactions reach the person unchanged.
- **F014-FR-014**: Every value MUST be passed to the command as a separate, unmodified argument or through standard input, never through a shell, so that none can be split or read as an option.
- **F014-FR-015**: On confirmation of an agent run with a reference, the plugin MUST compute the digest again and MUST refuse, showing the preview again, if the file changed since the preview.
- **F014-FR-016**: After a command ends, the plugin MUST show its exit code and outcome, and MUST refresh the open views. It MUST NOT report an outcome other than the command's own.
- **F014-FR-017**: An action MUST be offered only when its command can apply, under the same conditions as F012-FR-011; an unavailable action MUST be shown with its reason and MUST NOT start. One confirmation MUST start at most one command.

**Installation and configuration**

- **F014-FR-018**: The plugin MUST live in its own directory of this repository, be released with ai-dev, and be installable with a documented lazy.nvim specification. It MUST work without LazyVim.
- **F014-FR-019**: Without the LazyVim preset the plugin MUST define commands and no key mappings. The preset MUST add mappings under one documented prefix and MUST NOT replace any default mapping of the pinned LazyVim version it is tested against.
- **F014-FR-020**: Unknown options and invalid values MUST be reported, and the default MUST be used for each.
- **F014-FR-021**: The plugin MUST provide a health check that reports the Neovim version, the ai-dev path and version, and their compatibility.
- **F014-FR-022**: The plugin MUST NOT write any file, keep state between Neovim sessions, contact the network, or start any process other than ai-dev.

**Safety and boundaries**

- **F014-FR-023**: Every string a plugin view or preview shows that comes from files, the environment or command results MUST be shown as inert text: control characters, escape and operating-system-command sequences, terminal hyperlinks and bidirectional overrides MUST be visibly escaped.
- **F014-FR-024**: The plugin and `ai-dev open` MUST hold no authority: no mapping, command or confirmation of theirs approves anything; decisions are recorded only by the ai-dev commands of F014-FR-013.
- **F014-FR-025**: No output, preview or view MAY contain a credential, token, secret or environment value.
- **F014-FR-026**: `open` MUST appear in the command catalog of Feature 010 with per-command help and shell completion. The top-level usage line MUST become `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|open|version|help|completion> [options]`, amending F010-FR-010, and the general help MUST list `open`, amending F009-FR-003 and F009-FR-005. Every other command's behavior, output and exit codes, and every existing cockpit area and action, MUST remain unchanged.
- **F014-FR-027**: The documentation MUST cover installation, configuration, the supported Neovim versions, the privacy boundary (what a reference contains and that the agent may send what it reads to its provider), troubleshooting, and removal.

### Required Amendments

**Approved and applied (2026-11-24)** with this specification. Each amended specification records the decision in a Clarifications session dated 2026-11-24. Until Feature 014 is implemented, the amended features behave as before.

| # | Specification | Change |
|---|---|---|
| A1 | 010 Command Help and Completion | F010-FR-010: the top-level usage line gains `open` (F014-FR-026) |
| A2 | 009 CLI Help and Version | F009-FR-003 and F009-FR-005: the general help and the top-level usage list `open` |
| A3 | 012 Terminal Cockpit | F012-FR-008: one more action, "open in editor" (`ai-dev open`); its other actions, views and keys are unchanged |
| — | 002, 003–008, 011, 013 | No change: the plugin reads their outputs and runs their commands |

### Key Entities

- **Editor Location**: a file of the project, as a path relative to the project directory, with an optional line and column; valid only after the project and symbolic-link check.
- **Code Reference**: an Editor Location with an optional line range, the digest of the saved file, and the selected text only when the person included it.
- **Plugin Action**: one ai-dev command the plugin can run, with its availability and reason, the values it needs from the person, and the exact command it produces.
- **Editor Target**: where `ai-dev open` sends a location: the parent instance named by `$NVIM`, or a foreground Neovim.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With a parent Neovim instance, a file opens at the requested line within 1 second of running `ai-dev open`, in 100% of runs of the test suite.
- **SC-002**: For 100% of the hostile paths in the fixture set (traversal, absolute paths, symbolic links leaving the project, missing files, directories), `ai-dev open` and the plugin open nothing and name the reason.
- **SC-003**: For 100% of the fixture states of Features 001, 004, 005, 007 and 011, each plugin view shows exactly the outcome the corresponding command reports for the same project.
- **SC-004**: For 100% of the plugin's actions, the command run is identical, argument for argument, to the command shown in its preview, and its effects, evidence and exit code equal those of the same command typed by hand.
- **SC-005**: For a fixture set of file names, selections and instructions with shell metacharacters, newlines, leading dashes and Unicode, the fake agent receives exactly the confirmed instruction in 100% of cases, and no shell runs any part of it.
- **SC-006**: In tests that run every plugin command and mapping without confirming, 0 approvals, abandonments or agent runs happen, and 0 bytes of buffer content are sent.
- **SC-007**: For a fixture set of hostile strings (escape, operating-system-command, hyperlink, bidirectional and control sequences), 0 reach the terminal as instructions from a plugin view or preview.
- **SC-008**: Opening a view or refreshing it never blocks editing for more than 100 ms, and a features view of a 50-feature project is complete within 2 seconds.
- **SC-009**: A developer goes from a selected range in Neovim to a running agent with that reference in under 30 seconds, without typing a path or a line number.
- **SC-010**: The regression suites of Features 001–013 pass, with only the expectations named in the Required Amendments updated, and the plugin's tests pass on the minimum supported Neovim version and on the latest stable one.

## Assumptions

- The project is the current directory for ai-dev and Neovim's working directory for the plugin, as for every other ai-dev command; its features are the directories under `specs/`.
- Supported platforms are macOS and Linux. The minimum supported Neovim version is fixed in planning by verification, and documented; Neovim 0.12.5 is installed locally.
- The integration is optional: ai-dev and the cockpit need neither Neovim nor the plugin, and behave as before without them.
- `$NVIM` is set by Neovim in the environment of the processes its terminals start. A process that can change that variable already acts as the same user, so the parent instance is trusted as the user's own; F014-FR-003 rejects endpoints that are not a local socket owned by that user.
- The agent reads the referenced file itself; whether the agent sends what it reads to its provider depends on the agent, not on ai-dev, and the preview and documentation say so.
- The size limit for included selected text and the exact LazyVim prefix are fixed in planning and documented.
- The plugin, its views and its messages are in English, like the rest of the CLI.
- Neovim becomes a tool of continuous integration for the plugin's tests; the plan must declare it and its version under the dependency policy.
- Out of scope, each because no capability exists to present without inventing one: operations with identifiers, status updates and remote cancellation, execution history and evidence (Feature 002 keeps none); orchestration sessions and budgets (Feature 007); a pending-gate queue, gate expiry and approver authentication (Feature 006); diffs, suggested patches and diagnostics produced by ai-dev; a capability registry. Also out of scope: other editors, reimplementing editing, LSP, DAP, Treesitter or plugin management, automatic actions on editor events, and mandatory LazyVim.
