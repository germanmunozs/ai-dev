# Data Model: Agent Role Selection

**Feature**: 003-agent-role-selection
**Date**: 2026-09-28
**Revised**: 2026-09-28 (remediation pass — replaces the mutable-map Selection Policy
representation with an unexported exhaustive-switch function, and updates `Selector` to
remove the `policy` field entirely; see `plan.md` "Remediation Summary" findings 3 and 6)

This domain is introduced by this feature. `internal/role` is the vendor-neutral role
model; `internal/selection` is the policy boundary connecting roles to canonical
supported-agent identities — it is not itself described as vendor-neutral, since
resolving role identity into vendor-specific agent identity is its purpose (see
`plan.md` "Terminology"). This document mirrors the spec's Key Entities section with
concrete Go shapes, in the same style as
`specs/002-coding-agent-execution/data-model.md`. It reuses, and does not redefine,
`internal/agent`'s `ID`, `Agent`, `Availability`, `AgentStatus`, and `Registry` — see
that package's own data model for those shapes.

## Development Role (`internal/role`)

```go
package role

// Role is a canonical development-role identifier. It is a domain concept independent
// of any coding agent (FR-002) — this package has no dependency on internal/agent.
type Role string

const (
    Specification  Role = "specification"
    Planning       Role = "planning"
    Implementation Role = "implementation"
    Review         Role = "review"
    Remediation    Role = "remediation"
)

// Recognized reports whether value is exactly (case-sensitively) one of the five fixed
// canonical roles, with no normalization, aliasing, abbreviation, or fuzzy matching
// (FR-002). A false result carries no Role value.
func Recognized(value string) (Role, bool)
```

- **Invariants**: Exactly five `Role` values exist, fixed at these five identifiers
  (FR-002, Assumptions). Matching is exact-string, case-sensitive (`"Implementation"` is
  not recognized — Edge Cases).
- **Extensibility**: A future role is one more `Role` constant plus one more entry in
  `Recognized`'s backing set; it does not change the meaning of any already-recognized
  role (FR-019) and, because `internal/selection`'s policy is keyed by `Role`, a role
  with no policy entry simply cannot resolve to `role_selected`/`agent_unavailable`
  through the existing `Selector` logic without a corresponding policy entry also being
  added — see `internal/selection.policy` below.

## Selection Policy (`internal/selection`)

The fixed, built-in, deterministic role-to-agent mapping (FR-003) is represented as an
unexported, exhaustive `switch` **function**, not a map or any other mutable value. This
guarantees there is no data structure anywhere in the codebase — production or test —
that could be edited, replaced, persisted, or read generically from outside this file
(see `plan.md` "Selection Policy Immutability" for the full rationale):

```go
package selection

// mappedAgent returns the fixed production mapping's target agent identity for a
// canonical role (FR-003). It has no exported accessor, no setter, and no way to be
// constructed or parameterized differently — the feature provides no capability to
// view, edit, persist, override, score, or dynamically configure it (FR-003).
func mappedAgent(r role.Role) (agent.ID, bool) {
    switch r {
    case role.Specification:
        return agent.ClaudeCode, true
    case role.Planning:
        return agent.ClaudeCode, true
    case role.Implementation:
        return agent.CodexCLI, true
    case role.Review:
        return agent.ClaudeCode, true
    case role.Remediation:
        return agent.CodexCLI, true
    default:
        return "", false
    }
}
```

- **Invariants**: Exactly the five entries in FR-003's table, every value drawn from
  `internal/agent`'s existing `ID` constants (`agent.ClaudeCode`, `agent.CodexCLI`) — no
  new agent identifier is introduced by this feature. The `default` branch is
  unreachable in production (every `role.Role` value `role.Recognized` can produce is one
  of the five `case`s), retained only so the switch is exhaustive and defensive, mirroring
  `internal/report`'s existing defensive-validation style elsewhere in this codebase.
- **Not evidence of suitability**: The mapping's existence and values MUST NOT be
  represented as a claim that either agent is inherently better suited to any role
  (FR-003) — this is a documentation/reporting discipline, not a data constraint.

## Selector (`internal/selection`)

```go
// Selector resolves one Role Selection Request against the fixed production mapping and
// a supported-agent registry. It never launches, authenticates, probes the network, or
// otherwise invokes any coding agent (FR-004) — it holds no system.Executor, no
// system.Launcher, and no execution.Runner, so it has no path to do so.
type Selector struct {
    registry agent.Registry // the only field; no policy value is ever held here
}

// NewSelector builds a Selector against the given registry. Select always resolves
// through the fixed, unexported mappedAgent function — there is no way to supply a
// substitute policy through this or any other exported constructor.
func NewSelector(registry agent.Registry) Selector

// Select resolves exactly one supplied role value (verbatim, as the caller supplied it)
// to a Role Selection Report.
func (s Selector) Select(suppliedRole string) Report

// resolveAgainstRegistry determines the outcome for an already-recognized role and its
// mapped target agent identity, given the registry's current state. It takes the target
// explicitly (rather than looking it up itself), which is what makes it directly
// testable with a deliberately unsupported target — see "Selector — test seam" below.
func resolveAgainstRegistry(suppliedRole string, requested role.Role, target agent.ID, registry agent.Registry) Report
```

`Select`'s body is exactly: recognize the role (§ below); if recognized, call
`mappedAgent(requested)` for the target, then return
`resolveAgainstRegistry(suppliedRole, requested, target, s.registry)`.

- **Determinism**: For unchanged `suppliedRole` and registry-observable local state,
  `Select` returns an identical `Report` on every call (FR-011) — it is a pure function
  of its inputs (the fixed `mappedAgent` function, plus the caller's `suppliedRole` and
  `registry`), with no internal state and no randomness/scoring.
- **Selector — test seam**: `resolveAgainstRegistry` is unexported but takes its target
  as an explicit parameter, so `internal/selection`'s own in-package tests
  (`selector_test.go`, `package selection`) call it directly with a deliberately
  unsupported target (e.g., `agent.ID("not-a-real-agent")`) to exercise
  `selection_inconsistent` — without constructing a `Selector` with any non-production
  policy, because no such construction is possible (there is no `policy` field on
  `Selector` at all). See `research.md` §4 and `plan.md` "Selection Policy Immutability".

## Role Selection Outcome / Report (`internal/selection`)

```go
type Outcome string

const (
    RoleSelected          Outcome = "role_selected"
    RoleUnsupported       Outcome = "role_unsupported"
    AgentUnavailable      Outcome = "agent_unavailable"
    SelectionInconsistent Outcome = "selection_inconsistent"
)

// Report is the result of one recognized Role Selection Request, in both human- and
// machine-readable presentations (FR-010, FR-012).
type Report struct {
    SuppliedRole    string    // exactly as supplied by the caller, always present (FR-007, FR-012)
    RequestedRole   role.Role // canonical role; empty/absent only for RoleUnsupported (FR-007)
    SelectedAgentID agent.ID  // present only when Outcome == RoleSelected (FR-006, FR-012)
    MappedAgentID   agent.ID  // present only when Outcome == AgentUnavailable (FR-008) or
                               // SelectionInconsistent (FR-009); absent otherwise (FR-012)
    Outcome         Outcome
}
```

- **Invariants**: `Outcome` is always exactly one of the four declared constants
  (FR-010). Per-outcome field presence is exactly:

  | Outcome                  | `SuppliedRole` | `RequestedRole` | `SelectedAgentID` | `MappedAgentID` |
  |---------------------------|:---:|:---:|:---:|:---:|
  | `role_selected`            | ✓ | ✓ | ✓ | — |
  | `role_unsupported`          | ✓ | — | — | — |
  | `agent_unavailable`          | ✓ | ✓ | — | ✓ |
  | `selection_inconsistent`      | ✓ | ✓ | — | ✓ |

  This table is enforced twice: by `Selector.Select`'s construction (it only ever sets
  the fields that apply to the branch it takes) and, defensively, by
  `internal/report`'s writers before rendering (mirroring
  `executionreport.go`'s `validateExecutionReport` pattern) — an internal defect that
  produced an inconsistent `Report` value fails loudly at render time rather than
  emitting a malformed or misleading report.
- **No selected/mapped ambiguity**: `SelectedAgentID` and `MappedAgentID` are never both
  non-empty on the same `Report` — a selected agent is never also reported as merely
  "mapped" (FR-006 vs. FR-008/FR-009 are mutually exclusive by construction).

## Relationships

```
role.Recognized       --validates-->  a supplied string --into-->  role.Role (or role_unsupported)
selection.mappedAgent  --maps-->       role.Role --to-->            agent.ID (the Selection Policy)
agent.Registry.List()   --supplies--> current agent.AgentStatus set (existence + availability)
selection.resolveAgainstRegistry --combines--> mappedAgent's target + Registry.List() scan --produces--> Report
```

- One `Select` call maps to at most one `agent.Registry.List()` call and produces
  exactly one `Report` (mirrors 002's "one Request -> one Report" invariant, adapted:
  here there is no process invocation at all). There is no persisted history between
  invocations (spec Assumptions) — `Report` is a return value, not a stored record.

## State Transitions (Role Selection Outcome)

There is no mutable state machine — a `Report` is produced once, synchronously, inside
`Select`. The decision points that lead to each terminal `Outcome` are:

| Decision point                                                              | Terminal Outcome |
|-------------------------------------------------------------------------------|-------------------|
| No role value supplied, or whitespace-only, or extra args/unknown flags (usage) | *(usage failure — no Report; handled in `internal/app`, not `Selector`)* |
| Supplied role value not one of the five canonical roles                        | `role_unsupported` |
| Recognized role; policy's target `agent.ID` absent from `Registry.List()`'s set | `selection_inconsistent` |
| Recognized role; target agent present but `Availability == Unavailable`         | `agent_unavailable` |
| Recognized role; target agent present and `Availability == Available`           | `role_selected` |
