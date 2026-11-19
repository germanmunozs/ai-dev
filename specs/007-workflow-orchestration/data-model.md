# Data Model: Feature 007 — Guided Mode

Nothing in guided mode is persisted. Every entity below is computed for one request and discarded afterwards.

## Guidance Report

| Field | Type | Present when | Source |
|---|---|---|---|
| `FeatureID` | string, verbatim | always | the request |
| `Outcome` | one of 005's five outcomes | always | `sdd.Evaluate` (R-01) |
| `State` | 004 Workflow State | `step_recommended`, `workflow_terminal`, `artifacts_inconsistent` | `sdd.Evaluate` |
| `Activity` | 005 SDD Activity | `step_recommended` | `sdd.Evaluate` |
| `Role` | 003 Development Role | `step_recommended` | `sdd.Evaluate` |
| `Missing` | ordered list of artifact names | `artifacts_inconsistent` | `sdd.Evaluate` |
| `Agent` | 002 canonical agent identity | `step_recommended` | 003 mapping via `selection.Select` (R-02) |
| `Availability` | `available` \| `unavailable` \| `inconsistent` | `step_recommended` | R-02 |
| `Templates` | ordered list of Command Template | `step_recommended`, `workflow_terminal`, when the identifier is template-safe | F007-FR-005, F007-FR-006 |
| `Statement` | fixed text | always | F007-FR-008 |

**Validation (`Valid`)**:
- Every field that is absent for an outcome is empty, exactly as `sdd.Report.Valid` requires for the underlying 005 fields.
- `Agent` and `Availability` are both set or both empty.
- `Templates` follows the table in F007-FR-005 for the outcome and availability, in order, or is empty when the identifier is not template-safe.
- An invalid report is never rendered. It is an output failure: exit 2 with a stderr diagnostic, as 005 does.

## Command Template

| Field | Type | Rule |
|---|---|---|
| `Kind` | `select` \| `run` \| `transition` \| `approve` \| `state` | fixed per F007-FR-005 |
| `Label` | fixed human label | for example "select the agent for this role (003)" |
| `Words` | ordered list of Word | below |
| `HumanSlots` | ordered list of slot names | the `human` words, in order of appearance |

Each **Word** is one of:
- `fixed(text)`: a literal command word such as `ai-dev`, `workflow`, `--` or `--approver`, rendered as is.
- `token(text)`: a canonical role or agent identity, made of `[a-z-]` only, rendered as is.
- `feature`: the identifier, rendered as one POSIX single-quoted word (R-04).
- `human)name(`: a slot named `instruction`, `approver` or `target-state`, rendered unquoted between inverted parentheses as `)name(` (R-03).

**Templates by outcome:**

| Outcome / availability | Kinds, in order |
|---|---|
| `step_recommended`, `available` | `select`, `run`, `transition`, `approve` |
| `step_recommended`, `unavailable` | `select`, `transition`, `approve` |
| `step_recommended`, `inconsistent` | `transition`, `approve` |
| `workflow_terminal` | `state` |
| any other outcome | none |

**Words by kind:**

| Kind | Words |
|---|---|
| `select` | `ai-dev agent select -- {role}` |
| `run` | `ai-dev agent run -- {agent} )instruction(` |
| `transition` | `ai-dev workflow transition -- {feature} )target-state(` |
| `approve` | `ai-dev workflow approve --approver )approver( -- {feature} )target-state(`, followed in the report by the note that `--reason` is optional (006 FR-010) |
| `state` | `ai-dev workflow state -- {feature}` |

## Template-safe identifier

An identifier is template-safe when none of its runes is in Unicode category Cc or Cf and none is U+2028 or U+2029. Any other identifier gets the full report with no templates (F007-FR-006). The identifier is always valid UTF-8, because `fs.ValidPath` guarantees it before recognition.

## State transitions

None. Guidance has no state, and it never changes 004's Workflow State.
