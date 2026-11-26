# Contract: `ai-dev watch` (`ai-dev.watch/v1`)

## Usage

```text
Usage: ai-dev watch [--format text|json] [--feature <id>]... [--debounce <duration>] [--heartbeat <duration>] [--environment-interval <duration>] [--once]
```

| Option | Default | Allowed | Notes |
|---|---|---|---|
| `--format` | `text` | `text`, `json` | as every command |
| `--feature` | all features | a feature identifier, repeatable | validated with 004's rule (`workflow.FeatureDir`); a structurally invalid one is a usage error |
| `--debounce` | `250ms` | 50 ms to 5 s | a Go duration (`300ms`, `1s`) |
| `--heartbeat` | `15s` | 5 s to 300 s | |
| `--environment-interval` | `60s` | `0`, or 5 s to 3600 s | `0` disables revalidation after the snapshot |
| `--once` | off | | emit the snapshot only, then exit 0 |

A usage error writes the usage line to standard error, emits no event and exits 2.

## Envelope

Every event is one JSON object on one line, terminated by `\n`, with its members in this order:

```json
{"schema_version":"ai-dev.watch/v1","seq":1,"type":"snapshot","time":"2026-11-26T10:00:00.000Z","project":"/Users/ana/dev/app","data":{}}
```

| Member | Type | Rule |
|---|---|---|
| `schema_version` | string | always `ai-dev.watch/v1` |
| `seq` | integer | 1 for the first emitted event, then +1 for each emitted event of the process |
| `type` | string | one of the types below |
| `time` | string | RFC 3339, UTC, milliseconds, `Z` |
| `project` | string | the absolute path of the project directory |
| `data` | object | by type |

## Shared objects

- **Feature**:
  - shape: `{"feature": <id>, "state": <S>, "artifacts": [<Artifact>...], "next": <N>}`;
  - `<S>` is the exact object that `ai-dev workflow state --format json <id>` prints;
  - `<N>` is the exact object that `ai-dev workflow next --format json <id>` prints.
- **Artifact**:
  - shape: `{"name": "spec.md"|"plan.md"|"tasks.md", "status": "present"|"absent"|"not_regular_file", "size": <bytes>, "modified": <RFC 3339 UTC, nanoseconds>}`;
  - `size` and `modified` appear only when `status` is `present`;
  - the list always holds the three names, in that order;
  - a symbolic link is `not_regular_file`, and it is never followed.
- **Setup**: the exact object that `ai-dev setup show --format json` prints (`ai-dev.setup/v1`). It holds the outcome, the source, the agents with their availability, and the roles.
- **Environment**: the exact object that `ai-dev doctor --format json` prints (`overall`, `checks`).
- **HumanGate**:
  - shape: `{"decision": "approval"|"abandonment", "approver", "from_state", "to_state", "decided_at", "reason"?}`;
  - it is the last entry of the record's `human_gates`, as recorded, with `reason` present only when recorded.

## Event types

| `type` | `data` |
|---|---|
| `snapshot` | `{"features": [<Feature>...], "setup": <Setup>, "environment": <Environment>}`. Features are in name order; with `--feature`, only the named ones, and a named feature that does not exist appears with `feature_unknown` in `state` and `next` and empty artifacts |
| `feature_added` | `<Feature>` |
| `feature_removed` | `{"feature": <id>}` |
| `workflow_state_changed` | `{"feature", "from": <state or null>, "to": <state or null>, "outcome": "state_reported"\|"state_invalid", "state": <S>, "human_gate": <HumanGate or null>}`. `from` and `to` are null when that side is not a recognized state. `human_gate` is null unless a new history entry appeared since the previous observation and its `to_state` equals `to` (F015-FR-008) |
| `artifacts_changed` | `{"feature", "artifacts": [<Artifact>...]}` |
| `next_step_changed` | `{"feature", "next": <N>}` |
| `agents_config_changed` | `{"setup": <Setup>}` |
| `environment_changed` | `{"checks": [{"id", "from": "pass"\|"fail"\|null, "to": "pass"\|"fail"\|null, "remediation"}...], "environment": <Environment>}`. Only the checks whose status changed; null for a check that appeared or disappeared |
| `heartbeat` | `{}` |
| `resync_required` | `{"reason": "queue_overflow"\|"watch_overflow"}`. A `snapshot` always follows |
| `watch_stopped` | `{"reason": "signal"\|"project_gone"\|"watch_unavailable"\|"internal_error", "detail"?, "remedy"?}`. `detail` names the cause for `watch_unavailable` and `internal_error`; `remedy` says how to raise the limit or fix the cause for `watch_unavailable` |

Emission rules:

- The first event is `snapshot`, unless watching cannot be established before it. In that case the only event is `watch_stopped` with `watch_unavailable`.
- An event is emitted only when its computed result differs from the previous one.
  - For one feature, the order is `workflow_state_changed`, `artifacts_changed`, `next_step_changed`, each only when it changed.
  - `feature_added` and `feature_removed` replace the per-feature events for that feature.
- `--feature` limits feature events only. `agents_config_changed`, `environment_changed`, `heartbeat`, `resync_required` and `watch_stopped` are unaffected.
- `watch_stopped` is the last event. After `--once` there is none.

## Compatibility

Within `ai-dev.watch/v1`:

- New members and new event types may be added, and clients must ignore what they do not know.
- The embedded command objects follow their own schemas.
- Removing or renaming a member, or changing its meaning, requires `ai-dev.watch/v2`.

## Text format

One line per event, for debugging. Every string that comes from the project is in the shell-inert quoted form of 004 FR-028.

```text
ai-dev: event=snapshot seq=1 time=2026-11-26T10:00:00.000Z project="/Users/ana/dev/app" features=3 setup=configured environment=ready
ai-dev: event=workflow_state_changed seq=4 time=2026-11-26T10:02:08.120Z feature="004-login" from=implemented to=approved outcome=state_reported human_gate=approval approver="ana@example.com" reason="Revisado"
ai-dev: event=workflow_state_changed seq=5 time=2026-11-26T10:03:00.004Z feature="005-x" from=planned to=tasked outcome=state_reported human_gate=none note="no recorded human gate for this change"
ai-dev: event=artifacts_changed seq=6 time=2026-11-26T10:03:10.500Z feature="005-x" spec.md=present plan.md=present tasks.md=absent
ai-dev: event=watch_stopped seq=9 time=2026-11-26T10:05:00.000Z reason=watch_unavailable detail="inotify watch limit reached (fs.inotify.max_user_watches)" remedy="sudo sysctl fs.inotify.max_user_watches=524288"
```

## Exit codes

| Code | When |
|---|---|
| 0 | stopped by `SIGINT` or `SIGTERM` (after `watch_stopped`, or after waiting 2 s to write it); `--once` after the snapshot |
| 1 | `watch_unavailable` (including an unreadable project directory at start, an unsupported platform, or a limit reached), `project_gone` |
| 2 | a usage error (no event); an internal failure, after `watch_stopped` with `internal_error` when it can still be written; standard output closed or unwritable (no further event) |

## Read-only

- `watch` writes, creates, deletes, renames and locks nothing in the project.
- It starts no process other than the doctor checks' `git rev-parse --is-inside-work-tree`.
- It never runs a coding agent.
