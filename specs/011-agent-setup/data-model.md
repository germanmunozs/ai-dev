# Data Model: Agent Detection and Role Configuration

## Agent Catalog (existing, 002)

| Field | Meaning | Rules |
|---|---|---|
| `id` | Canonical identifier | `claude-code`, `codex-cli`; fixed at build time |
| `label` | Display label | `Claude Code`, `OpenAI Codex CLI` |
| executable | Name looked up on `PATH` | Never reported |

Order: catalog order, as in `ai-dev agents`.

## Agent Detection Result

| Field | Meaning |
|---|---|
| `id`, `label` | From the catalog |
| `availability` | `available` or `unavailable`, by executable lookup only |
| `chosen` | True when a valid project configuration lists the agent; false otherwise |

Never persisted.

## Project Configuration (`.ai-dev/agents.json`)

| Field | Type | Rules |
|---|---|---|
| `schema_version` | string | Exactly `ai-dev.agents/v1` |
| `agents` | array of strings | One or more; distinct; each a catalog identifier |
| `roles` | object | Exactly the keys `specification`, `planning`, `implementation`, `review`, `remediation`; each value one of `agents` |

- No other field, no `null`, no duplicate key, no trailing data; at most 64 KiB; a regular file inside a `.ai-dev` directory, neither a symbolic link.
- Canonical form: research R-04.
- States as loaded: `absent`, `valid`, `invalid` (with reasons, contracts/config-file.md).

## Role Assignment in Effect

| Source | When | Mapping |
|---|---|---|
| `built_in` | No configuration | 003 FR-003 |
| `project` | Valid configuration | The configuration's `roles` |
| (none) | Invalid configuration | No mapping; recognized roles yield `configuration_invalid` |

## Outcomes

| Command | Outcomes | Exit |
|---|---|---|
| `setup show` | `not_configured`, `configured` | 0 |
| | `configuration_invalid` | 1 |
| `setup apply` | `configuration_written`, `configuration_unchanged` | 0 |
| | `configuration_rejected`, `configuration_exists`, `persistence_failed` | 1 |
| `setup` (assistant) | `configuration_written`, `configuration_unchanged` | 0 |
| | `setup_cancelled`, `persistence_failed` | 1 |
| `agent select` | 003 outcomes, plus `configuration_invalid` | 0 only for `role_selected`; otherwise 1 |
| `workflow guide` | 007 outcomes; availability gains `configuration_invalid` | unchanged (F007-FR-011) |

Usage and output failures exit 2 everywhere.

## Relationships

- Every role's agent is one of the chosen agents; a chosen agent may have no role.
- Detection is independent of the configuration: an unavailable chosen agent is valid and produces the warning `chosen_agent_unavailable`.
- The configuration never affects `ai-dev agents` or `ai-dev agent run` (F011-FR-019).
