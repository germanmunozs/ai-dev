# Data Model: Project Event Watch

**Feature**: 015 | **Date**: 2026-11-26 | **Plan**: [plan.md](plan.md)

Nothing is persisted. Every entity lives in the memory of one `ai-dev watch` process; the JSON shapes are in [contracts/watch.md](contracts/watch.md).

## Watch Event

| Field | Rule |
|---|---|
| `Type` | one of the eleven types of the contract |
| `Seq` | assigned by the writer at emission; 1, then +1 (R-06) |
| `Time` | the time of emission, UTC |
| `Data` | the type's data; command objects are kept as the commands' bytes (`json.RawMessage`) |

## Observed Feature

The key is the feature identifier, a directory `specs/<id>` that `workflow.FeatureExists` accepts.

| Field | Source |
|---|---|
| `StateJSON` | `report.WriteWorkflowStateJSON(workflow.EvaluateState(root, id))`, through `watch.Source` |
| `State`, `Outcome` | the same state report (for `from`, `to` and `outcome`) |
| `NextJSON` | `report.WriteWorkflowNextJSON(sdd.Evaluate(root, id))`, through `watch.Source` |
| `Artifacts` | `Lstat` of `spec.md`, `plan.md` and `tasks.md`: `present` (regular file: size, modification time), `absent`, `not_regular_file` |
| `HistoryLen` | the number of entries `gate.ReadHistory` returned at the last observation; −1 when unreadable |

Comparisons are made on `StateJSON`, `Artifacts` and `NextJSON`, each separately.

## Project Picture

| Field | Rule |
|---|---|
| `Features` | map from identifier to Observed Feature |
| `SetupJSON` | the bytes of the `setup show` report, through `watch.Source` |
| `Environment` | the doctor report (statuses per check id) and its JSON bytes, through `watch.Source` |
| `Identity` | the device and inode of the project directory at start; a different or missing identity is `project_gone` |

A `snapshot` serializes the whole Project Picture, restricted to `--feature` when it is given.

## Recorded Human Gate Evidence

- `gate.Evidence` as read by `gate.ReadHistory` (R-05).
- It is read-only and never written by `watch`.
- It is attached to `workflow_state_changed` only when all three hold:
  - `HistoryLen` was not −1;
  - `len(history) > HistoryLen`;
  - the last entry's `ToState` equals the new state.

## Change Key and Debounce Window

| Key | Marked by a notification on | Evaluates |
|---|---|---|
| `project` | the project directory | identity (`project_gone`), then `specs` and `setup` |
| `specs` | `specs/` or its creation or removal | the feature listing: `feature_added` and `feature_removed` |
| `feature:<id>` | `specs/<id>/` or a file in it | that Observed Feature |
| `setup` | `.ai-dev/` or `agents.json` | `SetupJSON` |

A window opens at the first mark of a key and closes after `--debounce`; the key is evaluated once when the window closes (R-04).

## Watch Set

The paths the adapter observes, recomputed after each evaluation:

- the directories: the project, `.ai-dev` and `specs` when they exist, and every Observed Feature's directory;
- on macOS only, the files: `.ai-dev/agents.json` and each feature's `workflow-state.json`, `spec.md`, `plan.md` and `tasks.md`, when they are regular files.

A symbolic link is never observed or followed.

## Outbound Queue

- A FIFO of at most 1,000 events.
- On overflow it holds only `resync_required`, and production pauses until the writer has written it (R-06).

## Lifecycle

```text
start → validate options → (--once: snapshot → exit 0)
      → register watch set → snapshot → running
running → notification → window → evaluate → events
running → heartbeat / revalidation tick → events
running → queue overflow → resync_required → (written) → snapshot → running
running → signal → watch_stopped(signal) → exit 0
running → project gone → watch_stopped(project_gone) → exit 1
running → watch lost or limit → watch_stopped(watch_unavailable) → exit 1
any     → internal failure → watch_stopped(internal_error) → exit 2
start   → watching cannot be established → watch_stopped(watch_unavailable) → exit 1
```
