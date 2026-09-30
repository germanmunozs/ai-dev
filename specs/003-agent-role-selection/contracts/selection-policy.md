# Contract: Development Role, Selection Policy, and Selector

**Revised**: 2026-09-28 (remediation pass — replaces the mutable-map Selection Policy
and the `Selector{policy: ...}` struct-literal test seam with an unexported
exhaustive-switch function and a `resolveAgainstRegistry` seam; see `plan.md`
"Remediation Summary" findings 3 and 6)
**Revised again**: 2026-09-29 (second remediation pass, finding I3 — corrects the
"Never-launches / never-executes regression proof" section below, which had claimed the
`Run`-dispatch-boundary behavioral proofs cover "all four outcomes"; `selection_inconsistent`
cannot be dispatched through `Run` under the production mapping, so its no-execution
property is covered by the unconditional structural proof instead, reinforced by the new
application-level test in `contracts/agent-select.md`; see `plan.md` "Final Remediation
Summary (I3 + Corrections)")

**Satisfies**: architecture goals (role identity separate from agent identity; selection
policy separate from execution); FR-002, FR-003, FR-006 through FR-011, FR-018, FR-019;
planning requirements on testability and no-execution guarantees

These are internal Go contracts, not user-facing surfaces — but they are the mechanism
that makes "role identity separate from agent identity" and "no capability to view,
edit, override, or configure the mapping" enforceable properties rather than hopes, and
that makes `selection_inconsistent` testable without ever making the production mapping
mutable.

## `role.Role` and `role.Recognized` — the Development Role boundary

```go
package role

type Role string

const (
    Specification  Role = "specification"
    Planning       Role = "planning"
    Implementation Role = "implementation"
    Review         Role = "review"
    Remediation    Role = "remediation"
)

// Recognized reports whether value is exactly one of the five canonical roles above,
// case-sensitively, with no normalization/aliasing/fuzzy matching (FR-002).
func Recognized(value string) (Role, bool)
```

- `internal/role` is the **vendor-neutral role model**: this package imports nothing
  from `internal/agent`. A `Role` value carries no agent-shaped information — this is
  the structural enforcement of "a development role is a distinct domain concept from a
  coding agent" (FR-002).
- **Extensibility property (FR-019)**: adding role N+1 means adding one `Role` constant
  and one more entry in `Recognized`'s backing set. No existing `Role` value, and no
  existing case in `internal/selection`'s `mappedAgent` switch, is touched or
  reinterpreted.

## `selection.mappedAgent` — the Selection Policy

*Corrected from a mutable package-level map to an unexported, exhaustive switch
function; see remediation finding 3.*

```go
package selection

// mappedAgent is the fixed, built-in, deterministic role-to-agent mapping (FR-003),
// expressed as code rather than as a mutable data structure.
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

- `internal/selection` is the **policy boundary connecting roles to canonical
  supported-agent identities** — it is not itself described as vendor-neutral (see
  `plan.md` "Terminology"): it deliberately knows about both `role.Role` and `agent.ID`.
- Unexported, package-level, and — critically — **not a value at all**: there is no map,
  slice, struct field, or other mutable/substitutable data structure representing this
  mapping anywhere in the codebase, in production or in test code. There is no function
  anywhere in this codebase, including in `internal/selection`'s own test files, that
  constructs, mutates, or parameterizes `mappedAgent`'s behavior — it is a fixed switch.
- **Extensibility property (FR-018)**: adding a new supported agent to
  `internal/agent`'s registry requires no change here unless a human decision also
  changes which role maps to it — the two concerns are independent by construction (a
  new `agent.ID` existing does not, by itself, appear in `mappedAgent`).

## `selection.Selector` — the read-only selection lifecycle

```go
package selection

type Selector struct {
    registry agent.Registry // the only field
}

func NewSelector(registry agent.Registry) Selector

func (s Selector) Select(suppliedRole string) Report

// resolveAgainstRegistry is the test seam — see below.
func resolveAgainstRegistry(suppliedRole string, requested role.Role, target agent.ID, registry agent.Registry) Report
```

Lifecycle, entirely inside `Select`, with no agent invocation at any point:

1. **Recognize the role**: `role.Recognized(suppliedRole)` → not recognized →
   `Report{SuppliedRole: suppliedRole, Outcome: RoleUnsupported}`, returned immediately.
   No registry access occurs (FR-007's "MUST NOT attempt any agent availability check").
2. **Look up the policy target**: `mappedAgent(recognizedRole)` → the mapped `agent.ID`
   (always present for one of the five production roles, since `mappedAgent` defines a
   case for all five).
3. **Resolve against the registry**: `resolveAgainstRegistry(suppliedRole,
   recognizedRole, target, s.registry)` scans `registry.List()` for an `AgentStatus`
   whose `Agent.ID` equals `target`:
   - No match found → the target is not part of the current supported-agent registry →
     `Report{..., MappedAgentID: target, Outcome: SelectionInconsistent}` (FR-009).
   - Match found, `Availability == agent.Unavailable` →
     `Report{..., MappedAgentID: target, Outcome: AgentUnavailable}` (FR-008).
   - Match found, `Availability == agent.Available` →
     `Report{..., SelectedAgentID: target, Outcome: RoleSelected}` (FR-006).
4. **Return** the `Report` — the caller (`internal/app.RunAgentSelect`) renders it and
   sets the process exit code (FR-017). Neither `Select` nor `resolveAgainstRegistry`
   ever writes to any stream, launches any process, or mutates any state.

Neither `Selector` nor `resolveAgainstRegistry` contains any branch, flag, or code path
that constructs a `system.Launcher`, a `system.Executor`, an `execution.Runner`, or an
`execution.Adapter` — the guarantee that selection never executes an agent (FR-004) is a
property of this type's dependency graph, not just its test coverage: it is not possible
to reach agent execution from this package without changing this file, and
`internal/selection` imports neither `internal/execution` nor `internal/system`'s
`Launcher`/`Executor` interfaces for execution purposes (only `agent.Registry`, which is
itself read-only with respect to process execution).

## Controlled `selection_inconsistent` test seam (no mutable policy required)

Because `mappedAgent` is a fixed switch with no substitution mechanism, and because its
targets are always present in `internal/agent`'s registry under the approved mapping,
`selection_inconsistent` cannot be reached through any exported, public-facing path — by
design, this matches FR-009's own statement that "under the mapping ... together with the
supported-agent registry ..., this outcome does not occur in normal operation."

`internal/selection`'s own test file (`selector_test.go`, `package selection`, not
`package selection_test`) reaches it deterministically by calling
`resolveAgainstRegistry` **directly**, supplying a deliberately unsupported `target`
value as an explicit argument — without touching `mappedAgent`, without touching any
`policy` value (none exists), and without constructing a `Selector` with anything other
than a normal `agent.Registry`:

```go
func TestResolveAgainstRegistryReportsSelectionInconsistentForAnUnregisteredTarget(t *testing.T) {
    got := resolveAgainstRegistry(
        "implementation",
        role.Implementation,
        agent.ID("not-a-real-agent"), // deliberately absent from the registry
        agent.NewRegistry(fakeExecutor),
    )

    // assert got.Outcome == SelectionInconsistent, got.MappedAgentID == "not-a-real-agent",
    // got.SelectedAgentID == "" (never presented as selected — FR-009)
}
```

- This is white-box, same-package testing of a pure, unexported function — a strictly
  smaller and more restrictive seam than constructing any struct with a substitute
  policy field, because no such field exists to construct. It adds no new exported type,
  constructor, CLI flag, or environment variable, so it creates no capability to "view,
  edit, persist, override, score, or dynamically configure" the production mapping
  (FR-003) — `mappedAgent` is never read, referenced, or altered by this test; the test
  chooses its own `target` value independently of what `mappedAgent` would ever return.
- The production path (`Select` → the real `mappedAgent` → the real `agent.Registry`) is
  exercised by separate tests asserting `role_selected` and `agent_unavailable` only, and
  by a dedicated test asserting that every one of the five production `mappedAgent`
  cases currently resolves against the real `internal/agent` registry (i.e., that
  `selection_inconsistent` is unreachable under the approved mapping — SC-010's "0%
  under the approved production mapping" half).

## Never-launches / never-executes regression proof

*Revised per remediation finding 2 (`RunAgentSelect` no longer receives a
`system.Executor`) and finding 4 (explicit coverage of both execution mechanisms).*

Two independent, mutually reinforcing proofs are required, covering **both** existing
execution mechanisms in this codebase — `system.Executor.Run` and
`system.Launcher.Launch`:

1. **Structural**: `internal/app.RunAgentSelect`'s signature is
   `RunAgentSelect(args []string, registry agent.Registry, stdout, stderr io.Writer)
   int` — no `system.Executor`, no `system.Launcher`, no `context.Context` used for
   process supervision, no `execution.Runner`. A reviewer (human or automated) can
   confirm "this function cannot launch or execute a process" by reading its parameter
   list and import graph alone, without needing to trust a test. This is stronger than
   an earlier draft of this contract, which would have given `RunAgentSelect` a
   `system.Executor` parameter — closing the gap the independent plan review identified
   (see `plan.md` "Dependency Boundary").
2. **Behavioral, at the `app.Run` dispatch boundary** (where the real `system.Executor`
   and `system.Launcher` instances actually live) — **for the three outcomes that are
   actually dispatched through `Run`** (`role_selected`, `role_unsupported`,
   `agent_unavailable`; `selection_inconsistent` cannot be dispatched through `Run` under
   the production mapping — see `contracts/agent-select.md` "Recognizing a request" — so
   it is not, and does not need to be, part of this specific behavioral proof; its
   coverage is addressed separately below):
   - An `internal/app` test dispatches `ai-dev agent select` through `Run` (the same
     top-level dispatcher `agent run` uses), passing `Run` the *same* fake
     `system.Launcher` instance the existing `agent run` tests already use. `Run` holds
     that launcher in scope for the whole call but its `select`-handling branch never
     passes it anywhere (it has no parameter to receive it — see "Dispatcher Change" in
     `plan.md`). The test asserts the shared launcher's launch count is `0` after every
     `agent select` invocation across those three outcomes plus every usage-failure case,
     then — in the same test file, reusing the existing `agent run` assertions —
     confirms that same launcher instance's count does increase for an `agent run` call,
     proving the fake is wired up and capable of detecting a launch, not simply
     unreachable.
   - A second, parallel test wires a `recordingExecutor` (mirroring the one already
     defined in `internal/agent/registry_test.go`) into `Run`'s `executor` parameter and
     asserts its `Run`-call count stays `0` across those same three outcomes and every
     usage-failure case. This reuses, at the application dispatch boundary, the same
     zero-`Run`-calls property `internal/agent/registry_test.go` already proves for
     `agent.Registry.List()` in isolation — giving end-to-end confirmation that
     constructing the registry inside `Run` and passing it to `RunAgentSelect` never
     results in a `system.Executor.Run` call, not just that `Registry.List()` alone
     never makes one.
3. **`selection_inconsistent`'s no-execution coverage**: this outcome's zero-execution
   property does not rely on a `Run`-dispatch-boundary fake at all, and does not need
   one. It follows directly and unconditionally from proof 1 above — `RunAgentSelect`'s
   signature has no `system.Launcher`/`system.Executor` parameter regardless of which
   `agent.Registry` value it is given, including the empty `agent.Registry{}` used by its
   own dedicated application-level test (see `contracts/agent-select.md`
   "Application-Level Coverage for `selection_inconsistent`"). That test does not wire in
   a fake `Launcher`/`Executor` merely to assert a zero-call count the function's
   signature already makes impossible to violate; doing so would add an execution
   dependency to a test whose entire point is that no such dependency exists.

Where `system.Executor.Run` is already proven unreachable by `internal/agent`'s own
existing regression test, this contract reuses that proof rather than re-deriving it
from scratch, and adds only the one additional application-boundary test needed to close
the gap between "the registry itself never calls `Run`" and "the dispatch path that
builds the registry for `select` never ends up calling `Run` either." No new production
abstraction (e.g., a counting wrapper type, an execution-tracking middleware) is
introduced to make this provable — both proofs use plain test-double instances already
consistent with this codebase's existing test style.
