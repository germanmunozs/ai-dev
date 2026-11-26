# Feature Specification: Project Event Watch

**Feature Branch**: `feature/015-watch`

**Feature ID**: `015`

**Created**: 2026-11-26

**Status**: Approved for planning. Derived from the owner's draft "Feature 015 — Watch de eventos del proyecto (`ai-dev watch`)" (dated 2026-11-25, proposed from NEO / ai-dev Notch Pet), after a review against the repository on 2026-11-26. The owner authorized drafting on 2026-11-26 ("si, abre la rama y escribe la especificación de la 015"). The owner approved it as written, including Required Amendments A1–A3, the same day ("si, apruebo la 015, haz el commit y ejecuta /speckit-plan"). This is a declared, unauthenticated assertion. Implementation still needs the plan and tasks.

**Input**: A graphical client (NEO), the cockpit or an editor integration that shows a project's state today has two options: run `workflow state`, `doctor` and `setup show` every few seconds, or watch and read `specs/*/workflow-state.json` and `.ai-dev/*.json` itself. The first wastes CPU and battery and adds latency; the second makes every client reimplement how ai-dev reads those files and breaks when their format changes. This feature adds one read-only, long-running command, `ai-dev watch`, that observes the current project and reports each relevant change as it happens, computed by the same capabilities as the existing commands and with their outcomes. An event informs; it never approves, transitions or writes anything.

## Clarifications

### Session 2026-11-26

- **Authority record.** German Munoz, repository owner, accepted the decisions below on 2026-11-26 ("si, abre la rama y escribe la especificación de la 015"), after the review findings were presented with a proposal for each. This is a declared, unauthenticated assertion.
- **Review findings that shaped this specification.**
  - The draft's `workflow_state_changed` carried `outcome: "transition_applied"` and the approver for every state change. `transition_applied` is the result of a Human Gate request (Feature 006), which `watch` never sees; it only sees that the persisted state changed. A state can also change without any Human Gate: a manual edit, `git checkout` or `git pull`. Reporting such a change with an approver and `transition_applied` would present it as an approval, against constitution principle IX.
  - Decisions are recorded as `approval` and `abandonment`, not `approve`. Each Workflow State Record keeps its Human Gate evidence as a history (`human_gates`), each entry with `feature_id`, `decision`, `approver`, `from_state`, `to_state`, `decided_at` and an optional `reason`.
  - No capability reads that evidence today: Workflow State resolution (Feature 004) reads only the `state` member, and the Human Gate store (Feature 006) only writes.
  - Released binaries are built without the platform's C toolchain, so a watching mechanism that needs it is not available as the product is built today.
  - ai-dev rejects unsupported operating systems for durable state, but it does not detect file-system types; the draft's "fails closed on tmpfs, network or FUSE file systems, like the rest" does not hold.
  - Doctor's checks start no coding agent: they look tools and agents up on the search path and run `git rev-parse` for the repository check.
- Q: What does a state-change event report? → A: **The new state, with the outcomes of `workflow state`** (`state_reported` or `state_invalid`), never `transition_applied`. It carries the Human Gate evidence as recorded only when a new entry appeared in the record's history since the previous observation and that entry's `to_state` equals the new state; otherwise it carries no evidence, and the event says the change has no recorded Human Gate.
- Q: How is the evidence read? → A: **Through a new read-only reading of the recorded evidence**, owned by the Human Gate capability (Amendment A3). Writing evidence and resolving the state are unchanged.
- Q: How is read-only enforced? → A: **By a structural test**: the code of `watch` does not depend on the Human Gate store, the configuration store, the Git state store, the Git manager or agent execution at all.
- Q: Which watching mechanism? → A: **Not fixed by this specification.** The plan chooses, by measurement and without a new dependency, between the operating systems' change notifications and an internal file-status check; clients stop polling either way.
- Q: Unsupported file systems? → A: **`watch_unavailable` when watching cannot be established**, without promising to detect the file-system type.
- Q: The draft's ten-minute no-write test and the CPU and memory limits? → A: **A short no-write test in continuous integration; CPU and memory measured manually by the owner.**
- Q: An internal failure after watching started? → A: **`watch_stopped` with reason `internal_error`, exit 2.**
- Q: The draft's open questions? → A: **`--once` is in scope** (emit the snapshot and exit). **`next_step_changed` does not embed `workflow guide`**; a client that needs it runs `ai-dev workflow guide`. **Target release v0.4.0**, the next minor after v0.3.0.

### Session 2026-11-26 (analysis fixes)

- **Authority record.** After `/speckit-analyze`, the owner approved the ten proposed fixes on 2026-11-26 ("si, aplica los diez cambios"). This is a declared, unauthenticated assertion.
- The approved fixes change this specification in five places:
  - `--once` works on every platform (F015-FR-014, F015-FR-016, the unsupported-platform edge case), as the owner decided on the plan.
  - No evidence is attached when the previous observation could not read the history (F015-FR-008).
  - `seq` counts emitted events (F015-FR-002).
  - An unwritable standard output exits 2 (F015-FR-018).
  - SC-001 measures state changes only.
- No requirement is added or removed.

### Session 2026-11-26 (independent review fixes)

- **Authority record.** After the independent review (tasks.md T046), the owner approved the fixes on 2026-11-26 ("si corrige"), with the proposals presented for each finding. This is a declared, unauthenticated assertion.
- Q: What is a `specs` that is a file, a symbolic link, or a directory that cannot be read (review F13)? → A: **Like no `specs/` directory:** the snapshot reports zero features and `watch` keeps running. The cockpit's own listing is unchanged.
- The other fixes change no requirement: they make the implementation meet F015-FR-005, FR-008, FR-014, FR-016, FR-018 and FR-019 as written.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A client learns of a state change at once (Priority: P1)

A graphical client keeps `ai-dev watch --format json` running and reflects a feature's new Workflow State within a second of it being persisted, without polling, and can tell a recorded Human Gate decision from a change that has none.

**Why this priority**: It is the reason for the command; without it there is no value.

**Independent Test**: Start `ai-dev watch --format json` on a fixture project, then, in another process, approve a feature with `ai-dev workflow approve`, submit refused requests, and edit a record by hand; check the events received for each.

**Acceptance Scenarios**:

1. **Given** feature `004-login` in `implemented` and `watch` running, **When** `ai-dev workflow approve` records an approval to `approved`, **Then** `watch` emits exactly one `workflow_state_changed` with `from: "implemented"`, `to: "approved"`, `outcome: "state_reported"`, and the recorded evidence (`decision: "approval"`, approver, `from_state`, `to_state`, `decided_at`, reason if any).
2. **Given** `watch` running, **When** a Human Gate request is refused (for example `transition_invalid`, `artifacts_missing` or `state_conflict`), **Then** `watch` emits no `workflow_state_changed`, because the persisted state did not change.
3. **Given** a feature without `workflow-state.json` (state `draft`), **When** a Human Gate creates the record, **Then** the event reports `from: "draft"`.
4. **Given** `watch` running, **When** the record's `state` is changed by hand, or by checking out another revision, with no new Human Gate entry, **Then** the event reports the new state with no evidence and states that the change has no recorded Human Gate.

---

### User Story 2 - A complete snapshot on connection (Priority: P1)

A client receives a complete picture of the project as the first event, so it needs no other command before it starts listening and has no window between asking and subscribing.

**Why this priority**: Without a snapshot the client must combine several commands and can miss a change between them.

**Independent Test**: Start `watch` on a fixture project with three features in different states and compare its first event with the JSON output of `workflow state`, `workflow next`, `setup show` and `doctor` for the same project.

**Acceptance Scenarios**:

1. **Given** a project, **When** `watch` starts, **Then** the first event is `snapshot` with `seq: 1`, holding every feature with its state, artifacts and next step, the agents with their availability, the role assignment with its source, and the environment checks, each with the same values and outcomes as the corresponding command reports.
2. **Given** a change that happens while the snapshot is being computed, **When** the snapshot is emitted, **Then** the change is either included in it or arrives afterwards as an event with a higher `seq`; it is never lost.
3. **Given** `ai-dev watch --once`, **Then** only the snapshot is emitted and the command exits 0.

---

### User Story 3 - Changes to a feature's artifacts and next step (Priority: P2)

A client learns when a feature's specification, plan or tasks appear, change or disappear, and when that changes the recommended next step.

**Why this priority**: It keeps the client's next-step view current; the state change of US1 matters more.

**Independent Test**: With `watch` running, create, rewrite and delete `spec.md`, `plan.md` and `tasks.md` of fixture features, including several saves in quick succession, and check the events.

**Acceptance Scenarios**:

1. **Given** a feature in `specified`, **When** `plan.md` is created, **Then** `watch` emits `artifacts_changed` with the presence, size and modification time of each artifact.
2. **Given** that the change alters what `workflow next` recommends, **Then** `watch` also emits `next_step_changed` with the same content as `ai-dev workflow next --format json` for that feature.
3. **Given** an editor that saves the same file five times within 200 ms, **Then** `watch` emits one `artifacts_changed`.
4. **Given** a feature directory created or removed under `specs/`, **Then** `watch` emits `feature_added` with the feature's state, artifacts and next step, or `feature_removed`.

---

### User Story 4 - Agent configuration and environment health (Priority: P2)

A client learns when the agent configuration changes or becomes invalid, and when an environment check changes result, so it can show the project's health.

**Why this priority**: Health changes less often than workflow state and can tolerate the revalidation interval.

**Independent Test**: With `watch` running, apply `ai-dev setup apply`, write an invalid `.ai-dev/agents.json`, and remove a fake agent from the search path; check the events.

**Acceptance Scenarios**:

1. **Given** `watch` running, **When** `ai-dev setup apply` changes the role assignment, **Then** `watch` emits `agents_config_changed` with the resulting assignment and its source, as `setup show` reports them.
2. **Given** `.ai-dev/agents.json` becomes invalid, **Then** `watch` emits `agents_config_changed` with `outcome: "configuration_invalid"`, and never falls back silently to the defaults.
3. **Given** a fake agent is removed from the search path, **When** the next environment revalidation runs, **Then** `watch` emits `environment_changed` naming each check whose result changed, with its previous and new result and its remediation.

---

### User Story 5 - Predictable termination and recovery (Priority: P3)

A client can stop `watch`, detect lost events, and recover a consistent state without guessing.

**Why this priority**: Needed for robust clients, but the common path is US1 and US2.

**Independent Test**: Send SIGINT and SIGTERM to a running `watch`; stop reading its output until its queue overflows, then read again; delete the project directory while it runs.

**Acceptance Scenarios**:

1. **Given** `watch` running, **When** it receives SIGINT or SIGTERM, **Then** it emits `watch_stopped` with `reason: "signal"` and exits 0.
2. **Given** a client that sees a gap in `seq`, **When** it starts `watch` again, **Then** it receives a new `snapshot` that replaces all its state.
3. **Given** a client that stops reading and lets the queue fill, **Then** `watch` discards the queued events, emits `resync_required` and then a new `snapshot` once the client reads again, and its memory stays bounded.
4. **Given** the project directory is moved or deleted while `watch` runs, **Then** it emits `watch_stopped` with `reason: "project_gone"` and exits 1.

### Edge Cases

- **No `specs/` directory**: the snapshot reports zero features, and `watch` starts observing `specs/` when it is created. A `specs` that is a regular file, a symbolic link or a directory that cannot be read counts as no `specs/` directory: zero features, and `watch` keeps running.
- **An invalid or unreadable `workflow-state.json`**: that feature reports `state_invalid`, as `workflow state` does; the other features keep being observed and reported.
- **A record replaced atomically by renaming a new file over it**: it is reported as one change, not as a removal followed by an addition.
- **A symbolic link, or an entry that is not a regular file or directory, under `specs/` or `.ai-dev/`**: it is never followed. A feature directory that is a symbolic link is not a feature, as Feature 004 defines; an artifact or record that is one is reported with the reason, as the existing commands report it.
- **The operating system's limit on observed paths is reached** (for example Linux's `fs.inotify.max_user_watches`) **or watching cannot be established for another reason**: `watch` emits `watch_stopped` with `reason: "watch_unavailable"`, names the limit or cause and how to raise or fix it, and exits 1.
- **Watching cannot be established before the snapshot**: the only event is that `watch_stopped`; no snapshot is emitted, so a client never receives a picture it cannot keep current.
- **An unsupported operating system** (including native Windows): `watch_unavailable`, exit 1, as for the other durable-state operations; `--once` still emits the snapshot and exits 0 (F015-FR-014).
- **A feature named by `--feature` that does not exist**: the snapshot lists it with `outcome: "feature_unknown"`, and `feature_added` is emitted if it appears; a structurally invalid identifier is a usage error (exit 2).
- **Two changes to the same feature within one debounce window**: one event per changed result, computed from the final state; intermediate states that no longer hold are not reported.
- **Several Human Gate entries appended within one debounce window**: the event carries the latest entry only if its `to_state` equals the new state.
- **Feature identifiers, approvers, reasons or file names with quotes, `$`, backquotes, newlines, escape sequences or bidirectional overrides**: they are JSON-encoded in `json` format and shown in the existing shell-inert quoted form in `text` format.
- **The 006 FR-033 residual**: an agent the person runs can itself run `ai-dev workflow approve`. `watch` reports the recorded evidence as recorded; it neither adds nor removes this risk.

## Requirements *(mandatory)*

### Functional Requirements

**Command and output**

- **F015-FR-001**: `ai-dev watch [--format text|json] [--feature <id>]... [--debounce <duration>] [--heartbeat <duration>] [--environment-interval <duration>] [--once]` MUST observe the project in the current directory until it is stopped. In `json` format each event MUST be one JSON object on one line, terminated by a newline, and written whole.
- **F015-FR-002**: Every event MUST carry `schema_version` (`"ai-dev.watch/v1"`), `seq` (an integer that starts at 1 and grows by exactly 1 for each emitted event of the process), `type`, `time` (RFC 3339, UTC), `project` (the project directory) and `data`.
- **F015-FR-003**: The event types MUST be those of the table under Event Contract, with the data listed there. Within `ai-dev.watch/v1`, new fields and new event types MAY be added and clients MUST ignore those they do not know; removing or renaming a field, or changing its meaning, MUST require `ai-dev.watch/v2`.
- **F015-FR-004**: In `text` format `watch` MUST print one readable line per event (time, type, feature, summary), meant for debugging, with every string from the project in the shell-inert quoted form of the existing reports.

**Snapshot and changes**

- **F015-FR-005**: The first event MUST be `snapshot`, except when watching cannot be established (F015-FR-016). Every `resync_required` MUST be followed by a new `snapshot`. A change that occurs while a snapshot is computed MUST be included in it or reported after it.
- **F015-FR-006**: Every state, next step, role assignment and check result MUST be computed by the same capabilities as `workflow state`, `workflow next`, `setup show` and `doctor`, with their values and outcomes, and MUST be identical to what those commands report for the same project at the same moment. `watch` MUST NOT interpret project files in any other way.
- **F015-FR-007**: `watch` MUST emit an event only when a computed result changes, not every time a file is touched.
- **F015-FR-008**: `workflow_state_changed` MUST report the previous and new state with the outcome of `workflow state` (`state_reported` or `state_invalid`) and MUST NOT report `transition_applied` or any other Human Gate outcome. It MUST carry the Human Gate evidence, as recorded and unchanged, only when the previous observation read the history, a new entry appeared in it since then, and that entry's `to_state` equals the new state; otherwise, including when the previous observation could not read the history, it MUST carry no evidence and MUST state that the change has no recorded Human Gate.
- **F015-FR-009**: `artifacts_changed` MUST report, for `spec.md`, `plan.md` and `tasks.md`, presence, size and modification time only. `next_step_changed` MUST carry the same content as `ai-dev workflow next --format json` and MUST NOT embed `workflow guide`.
- **F015-FR-010**: Changes MUST be grouped with a debounce set by `--debounce` (default 250 ms, from 50 ms to 5 s).
- **F015-FR-011**: `watch` MUST emit `heartbeat` every `--heartbeat` (default 15 s, from 5 s to 300 s).
- **F015-FR-012**: `watch` MUST re-evaluate the environment checks and the agents' availability every `--environment-interval` (default 60 s, from 5 s to 3600 s; `0` disables revalidation after the snapshot), and the role assignment whenever `.ai-dev/agents.json` changes.
- **F015-FR-013**: `--feature` (repeatable) MUST limit feature events and the snapshot's features to the named features. Agent, environment and lifecycle events are unaffected.
- **F015-FR-014**: `--once` MUST emit the snapshot only and exit 0. It registers no watch, so it MUST work on every platform, including those where watching is unavailable.

**Termination, recovery and exit codes**

- **F015-FR-015**: On SIGINT or SIGTERM `watch` MUST emit `watch_stopped` with `reason: "signal"` and exit 0.
- **F015-FR-016**: When watching cannot be established or is lost, including when an operating-system limit is reached or the platform is unsupported (except with `--once`, F015-FR-014), `watch` MUST emit `watch_stopped` with `reason: "watch_unavailable"`, name the cause and its remedy, and exit 1. When the project directory disappears, it MUST emit `watch_stopped` with `reason: "project_gone"` and exit 1.
- **F015-FR-017**: When output is not being read, `watch` MUST keep at most 1,000 pending events. On overflow it MUST discard them, and when output is read again it MUST emit `resync_required` followed by a new `snapshot`. Its memory MUST NOT grow without bound.
- **F015-FR-018**: Exit codes MUST be: 0 for a stop by signal and for `--once`; 1 for a negative domain result (project directory unreadable, `watch_unavailable`, `project_gone`); 2 for a usage error, for an internal failure before or after watching started, and when standard output becomes unwritable (then no further event is emitted); after watching started, `watch_stopped` with `reason: "internal_error"` MUST precede exit 2.

**Read-only and safety**

- **F015-FR-019**: `watch` MUST be read-only: it MUST NOT write, create, delete, rename or lock any file of the project, including `workflow-state.json`, `.ai-dev/agents.json`, `.ai-dev/config.json` and the Git repository. A structural test MUST verify that its code does not depend on the Human Gate store, the configuration store, the Git state store, the Git manager or agent execution.
- **F015-FR-020**: `watch` MUST NOT run any coding agent, apply any transition, or present an event as an approval. The only processes it MAY start are those the doctor checks already start.
- **F015-FR-021**: Events MUST NOT contain the content of any artifact, credential, token, secret or environment value; only the metadata this specification lists.
- **F015-FR-022**: Every string that comes from the project (feature identifiers, approvers, reasons, file names) MUST be correctly JSON-encoded in `json` format and shown in shell-inert quoted form in `text` format, so that pasting it into a shell runs nothing.
- **F015-FR-023**: `watch` MUST appear in the command catalog of Feature 010 with per-command help and shell completion of its options. The top-level usage line MUST become `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|open|watch|version|help|completion> [options]` (Amendment A1), and the general help MUST list `watch` (Amendment A2). Every other command's behavior, output and exit codes MUST remain unchanged.
- **F015-FR-024**: The documentation MUST cover the command, the event contract with examples, the schema compatibility rules, how a client detects support (`ai-dev watch --help` or the version), and the recovery procedure.

### Event Contract (`ai-dev.watch/v1`)

The exact field names and JSON shapes are fixed in the plan's contract; the event types and their data are:

| `type` | When | `data` |
| --- | --- | --- |
| `snapshot` | At start and after `resync_required` | features (state, artifacts, next step), agents, roles with source, environment checks |
| `feature_added` | A feature directory appears under `specs/` | feature, state, artifacts, next step |
| `feature_removed` | A feature directory disappears | feature |
| `workflow_state_changed` | A feature's computed state changes | feature, from, to, outcome, recorded evidence or none |
| `artifacts_changed` | `spec.md`, `plan.md` or `tasks.md` appears, changes or disappears | feature, artifacts (presence, size, modification time) |
| `next_step_changed` | The `workflow next` result changes | feature, the `workflow next` content |
| `agents_config_changed` | The role assignment or its validity changes | roles, source, outcome |
| `environment_changed` | A check's result changes | checks with id, previous and new result, remediation |
| `heartbeat` | Every `--heartbeat` | empty |
| `resync_required` | Queue overflow or internal inconsistency | reason |
| `watch_stopped` | Before exiting, except after `--once` | reason: `signal`, `project_gone`, `watch_unavailable` or `internal_error` |

Example (illustrative; the plan's contract is authoritative):

```json
{"schema_version":"ai-dev.watch/v1","seq":3,"type":"workflow_state_changed","time":"2026-11-25T14:32:08Z","project":"/Users/ana/dev/app","data":{"feature":"004-login","from":"implemented","to":"approved","outcome":"state_reported","human_gate":{"decision":"approval","approver":"ana@example.com","from_state":"implemented","to_state":"approved","decided_at":"2026-11-25T14:32:07Z","reason":"Revisado"}}}
```

### Required Amendments

**Approved and applied (2026-11-26)** with this specification. Each amended specification records the decision in a Clarifications session dated 2026-11-26. Until Feature 015 is implemented, the amended features behave as before.

| # | Specification | Change |
|---|---|---|
| A1 | 010 Command Help and Completion | F010-FR-010: the top-level usage line gains `watch` (F015-FR-023) |
| A2 | 009 CLI Help and Version | F009-FR-003 and F009-FR-005: the general help and the top-level usage list `watch` |
| A3 | 006 Human Gates | The recorded Human Gate evidence becomes readable, read-only, by ai-dev's reporting capabilities, which present it as recorded and never as a decision of their own. FR-003 (sole write path) and the evidence format are unchanged |
| — | 001–005, 007, 008, 011–014 | No change: `watch` reads through their capabilities and reports their outcomes |

### Key Entities

- **Watch Event**: one unit of output, with the common envelope (`schema_version`, `seq`, `type`, `time`, `project`, `data`).
- **Snapshot**: the complete picture of the project at one moment; it replaces any previous state the client holds.
- **Observed Feature**: a directory `specs/<feature-id>/` that Feature 004 recognizes, with its state, artifacts and next step.
- **Recorded Human Gate Evidence**: the latest entry of a record's Human Gate history, read-only, attached to a state change only under F015-FR-008.
- **Agent Configuration**: the result of `setup show`: the agents, their availability, and the role assignment with its source.
- **Environment Health**: the results of the doctor checks.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With the default debounce, 95% of state changes (Human Gate decisions and edits of a record's `state`) reach the client within 1 second of the record being written, across the test suite.
- **SC-002**: In integration tests, every applied Human Gate produces exactly one `workflow_state_changed` carrying its evidence; every refused request produces none; every state change made without a Human Gate produces one event without evidence (100% of fixture cases).
- **SC-003**: A continuous-integration test that runs `watch` while at least 50 simulated changes are made finds the project tree, other than the simulated changes, byte-identical before and after: 0 writes by `watch`.
- **SC-004**: At rest, on a project with 100 features, `watch` uses under 0.5% of one CPU and under 30 MB of memory, measured manually by the owner and recorded with the tasks.
- **SC-005**: With a client that stops reading, memory stays bounded, and when it reads again it receives `resync_required` followed by a `snapshot`.
- **SC-006**: 100% of emitted lines are valid JSON with schema `ai-dev.watch/v1` and consecutive `seq` values, and for a fixture set of hostile strings 0 control or escape sequences reach the terminal in `text` format.
- **SC-007**: For 100% of fixture states, the snapshot's values equal those of `workflow state`, `workflow next`, `setup show` and `doctor` in JSON for the same project.
- **SC-008**: The regression suites of Features 001–014 pass, with only the expectations named in the Required Amendments updated.

## Assumptions

- The project is the current directory, as for every other ai-dev command; its features are the directories under `specs/` that Feature 004 recognizes.
- Supported platforms are macOS and Linux. The watching mechanism is chosen in planning by measurement and adds no dependency; release builds keep their current form.
- One `watch` process observes one project; a client runs one per project.
- The queue limit of 1,000 events is fixed, not configurable.
- Clients (NEO) require ai-dev v0.4.0 or later for `watch`; with earlier versions they keep polling, and detect support with `ai-dev watch --help` or the version.
- Messages and the text format are in English, like the rest of the CLI.
- Tests use fake agents on the search path; no real coding agent runs.
- Out of scope: reporting `agent run` executions started from other terminals (ai-dev keeps no record of running executions, Feature 002), Git events (commits, branches, `init` authorizations), the Git management configuration `.ai-dev/config.json`, observing several projects in one process, any transport other than standard output (sockets, HTTP, D-Bus), and native Windows.
