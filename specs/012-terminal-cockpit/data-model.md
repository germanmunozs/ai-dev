# Data Model: Terminal Cockpit

Nothing here is persisted. Everything lives while `ai-dev tui` runs.

## Snapshot (from `Source.LoadFeatures`, `LoadEnvironment`, `LoadAgents`, `LoadRepository`, one concurrent call each per refresh)

| Field | Content | Notes |
|---|---|---|
| `evaluatedAt` | time of the evaluation | shown in the status line |
| `features` | list of Feature Row | name order |
| `environment` | lines of the `doctor` text report | sanitized |
| `agents` | lines of the `setup show` text report | sanitized |
| `repository` | lines of the `init inspect` text report, or the failure | sanitized |
| per area | `loading` / `ready` / `failed` | a failed area shows its error, sanitized, and none of its previous lines |

## Feature Row

| Field | Source |
|---|---|
| `name` | directory under `specs/` (`workflow.FeatureExists`) |
| `state` | `workflow.EvaluateState` outcome and state |
| `next` | `sdd.Evaluate` outcome and activity |
| `role`, `agent`, `availability`, `source` | `guide.Evaluate`, for `step_recommended` only |
| `detail` | lines of the workflow state and guide text reports |
| `terminal` | the state is terminal (no approve, abandon or run) |

## Action

| Field | Meaning |
|---|---|
| `id` | `run-agent`, `approve`, `abandon`, `setup`, `init`, `commit-validate` |
| `title` | shown in lists, palette and review |
| `scope` | `feature` or `project` |
| `changes` | whether it changes state or starts an agent |
| `fields` | the values asked from the person (none prefilled) |
| `available(row)` | rule of F012-FR-011, with a reason when false |
| `argv(row, values)` | the exact argument vector (contracts/cockpit.md) |

## View State

| Field | Values |
|---|---|
| `area` | Dashboard, Features, Environment, Agents, Repository, Help |
| `selection`, `scroll`, `filter` | per area |
| `mode` | `browse`, `detail` (a feature's or an area's actions), `filter`, `palette`, `form`, `review`, `running`, `help`; "too small" overlays any of them |
| `pendingTerm` | a termination signal arrived while `running` |

Transitions: `browse → detail → form → review → running → detail` (or from the palette, back to `browse`); `Esc` goes back one step from `detail`, `form` and `review`; nothing leaves `running` except the exec callback, which shows the exit code in the status line and reloads every area; "too small" overlays any mode and confirms nothing.
