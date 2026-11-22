# Research: Agent Role Selection

**Feature**: 003-agent-role-selection
**Date**: 2026-09-28
**Revised**: 2026-09-28 (remediation pass — see `plan.md` "Remediation Summary" for the
full itemized disposition; this revision corrects §1's dispatcher description, §4's test
seam, §7's dependency description, and terminology throughout, per remediation findings
2, 3, 5, and 6)

This document resolves every technical unknown needed before design. The spec's
clarification session already closed every behavioral ambiguity (five clarifications,
recorded in `spec.md`); what remains here is implementation-shape research, in the same
style as `specs/002-coding-agent-execution/research.md`.

## 1. Command shape and actual current dispatcher

- **Decision**: `ai-dev agent select [--format text|json] <role>`, dispatched from
  inside the existing `if args[0] == "agent"` branch of `internal/app/doctor.go`'s `Run`.
- **Correction to an earlier draft of this research**: an earlier draft stated this would
  be "a new `case "select":` inside the existing ... branch, beside the existing
  `case "run":`", implying the current dispatcher already uses a `switch`/`case`
  structure. It does not. The current implementation is a single `if` condition:

  ```go
  if args[0] == "agent" {
      if len(args) < 2 || args[1] != "run" {
          fmt.Fprintln(stderr, "Usage: ai-dev agent run [--format text|json] <agent-id> [instruction]")
          return 2
      }
      return RunAgentRun(ctx, args[2:], executor, launcher, stdin, readerIsInteractive(stdin), "", stdout, stderr)
  }
  ```

  There is no `switch` anywhere in `internal/app/doctor.go`'s `Run` — the whole function
  is a flat sequence of `if` checks (`args[0] == "agents"`, `args[0] == "agent"`, then a
  fallback `args[0] != "doctor"` check). See `plan.md`'s "Dispatcher Change" section for
  the corrected, smallest additive change to this exact shape.
- **Rationale**: The spec's own Assumptions section gives this exact command shape
  ("conceptually resembling `ai-dev agent select <role>`") as illustrative, leaving the
  concrete syntax to the plan as long as observable behavior matches. Grouping it under
  the existing `agent` subcommand (rather than a new top-level verb) mirrors `agent run`
  precisely and requires no change to `Run`'s existing top-level dispatch structure
  (`doctor` / `agents` / `agent`), keeping the "no fallback/no new orchestration"
  constraint visible in the dispatcher itself: `select` and `run` are two independent,
  unrelated leaves under `agent`, not steps of one workflow.
- **Alternatives considered**:
  - A new top-level `ai-dev select <role>` verb — rejected; it would suggest a
    role-first workflow entry point distinct from agent concerns, which risks implying
    the kind of "automatic SDD phase selection" the spec explicitly excludes.
  - `ai-dev agent select --role <role>` (flag instead of positional) — rejected; every
    existing subcommand in this repo (`agent run <agent-id>`) uses a required concept as
    a bare positional, and `--format` is the only flag precedent.
  - Restructuring the dispatcher into a `switch` while adding `select` — rejected as an
    unrelated refactor; the smallest additive change keeps the existing `if`-chain idiom
    and adds one more independent check for `select`, rather than changing the control
    structure `run`'s existing tests already exercise (see `plan.md`'s Constitution III
    / YAGNI assessment and the remediation instruction not to perform an unrelated
    dispatcher refactor).

## 2. Package boundaries: `internal/role` vs `internal/selection` vs `internal/agent`

- **Decision**: Three packages, each with one responsibility:
  - `internal/role` — the **vendor-neutral role model**: the Development Role domain
    concept only (five canonical identifiers, exact-match recognition). No dependency on
    `internal/agent`. No vendor-specific identity (`claude-code`, `codex-cli`) ever
    appears in this package.
  - `internal/selection` — the **policy boundary connecting roles to canonical
    supported-agent identities**: the fixed Selection Policy (`mappedAgent`, an
    unexported exhaustive switch — see §4) and the `Selector` that produces a `Report`.
    Depends on both `internal/role` and `internal/agent`, but is not depended on by
    either. This package is deliberately *not* described as "vendor-neutral" — unlike
    `internal/role`, it is the one place in this feature that legitimately knows about
    both role identity and vendor-specific agent identity, because resolving one into
    the other is its entire purpose.
  - `internal/agent` — the **generic canonical coding-agent model**, completely
    unmodified from 002; reused as-is.
- **Rationale**: Directly satisfies the architecture goal "keep role identity separate
  from agent identity" (a `role.Role` value has zero knowledge of `agent.ID`) and "keep
  selection policy separate from execution" (`internal/selection` has no dependency on,
  and is not depended on by, `internal/execution` — reinforced further by `plan.md`'s
  "Dependency Boundary" section, which also removes `system.Executor`/`system.Launcher`
  from `RunAgentSelect` itself). This is the same shape 002 already used successfully:
  `internal/agent` (identity) is separate from `internal/execution` (lifecycle/policy for
  *running* an agent); `internal/role` (identity) is separate from `internal/selection`
  (policy for *choosing* an agent) is the same pattern one level up.
- **Alternatives considered**:
  - Putting the role identifiers directly inside `internal/selection` (no separate
    `internal/role` package) — rejected; it would make "a Development Role is a domain
    concept independent of any coding agent" (FR-002) an assertion rather than a
    structural fact, and would make a future standalone role-listing capability (noted
    as plausible future work in the spec) harder to add without churn.
  - Putting the fixed policy and `Selector` inside `internal/agent` — rejected; it would
    make the generic canonical agent catalog aware of an unrelated concept (development
    roles), which nothing in 001/002 requires and which the spec's separation goal
    forbids.

## 3. Reusing `agent.Registry` without adding a new method

- **Decision**: `selection.Selector.Select` calls `agent.Registry.List()` exactly once
  per request and scans the returned `[]agent.AgentStatus` for the policy's target
  `agent.ID`. If no entry matches that ID, the outcome is `selection_inconsistent`
  (the mapping target is not part of the registry's fixed set); if a matching entry's
  `Availability` is `agent.Available`, the outcome is `role_selected`; otherwise
  `agent_unavailable`.
- **Rationale**: `agent.Registry.List()` already enumerates the complete fixed
  supported-agent set with point-in-time availability — precisely what FR-005 requires
  ("the same local-availability determination used by the supported-agent model").
  Scanning `List()`'s result (rather than calling `Resolve` and then a second lookup, the
  pattern `execution.Runner.Run` originally used) collapses "is this ID part of the
  registry" and "is it available" into one pass over one already-existing call, which is
  both the smallest implementation surface and gives `selection_inconsistent` an exact,
  direct definition: *the policy's target ID does not appear in `Registry.List()`'s
  output* — matching FR-009's "not part of the current supported-agent model" precisely,
  with no separate existence check that could disagree with the availability check.
- **Alternatives considered**:
  - Adding a new `agent.Registry.IsAvailable(id) (available, found bool)` method —
    rejected; `List()` already exposes everything needed in one call, and 002 deliberately
    kept `Registry`'s surface to exactly `List`/`Resolve` (data-model.md "not a plugin
    system" note); adding a method for one caller is unjustified surface growth.
  - Calling `Resolve` first (existence) and `List` second (availability), as
    `execution.Runner` does — rejected as redundant for this feature: `Runner` needed
    `Resolve` separately because it also needs the resolved `Agent` value (for
    `DisplayLabel`/adapter lookup by ID) independent of availability; `Selector` only ever
    needs the ID and its availability, which one `List()` scan already gives it.

## 4. A seam for testing `selection_inconsistent` without a mutable production policy

- **Decision**: The fixed production mapping is `mappedAgent(r role.Role) (agent.ID,
  bool)` in `policy.go` — an unexported, exhaustive `switch` over the five canonical
  roles, not a map, not a struct field, and not something any constructor can be given a
  substitute for (see `plan.md`'s "Selection Policy Immutability" for the full code
  shape). `Selector.Select` calls `mappedAgent` to get the role's target, then calls a
  second unexported helper, `resolveAgainstRegistry(suppliedRole string, requested
  role.Role, target agent.ID, registry agent.Registry) Report`, which determines the
  outcome purely from the target it is given and the registry's current state.
  `internal/selection`'s own white-box test file (`selector_test.go`, `package
  selection`) calls `resolveAgainstRegistry` directly with a deliberately unsupported
  target (e.g., `agent.ID("not-a-real-agent")`) to exercise `selection_inconsistent`
  deterministically — without touching `mappedAgent` or the production policy at all.
- **Correction to an earlier draft of this research**: an earlier draft proposed
  constructing a `Selector{policy: testPolicy, registry: ...}` struct literal with a
  test-only policy map, which implied `Selector` held a `map[role.Role]agent.ID` field
  that *could*, in principle, be constructed with something other than the production
  mapping. This revision removes that field entirely — there is no map anywhere in
  `internal/selection`, and no `Selector` field ever holds anything but the caller's
  `agent.Registry`. The test seam is a pure function (`resolveAgainstRegistry`) that
  takes its target as an explicit argument, which is both a strictly smaller seam and
  removes any possibility (even a same-package one) of constructing a `Selector` with a
  non-production policy.
- **Rationale**: This satisfies "the production mapping must not be represented as
  something mutable, even for testing" directly — there is no mutable or substitutable
  policy value anywhere, in production or in test code; the seam works purely because
  `resolveAgainstRegistry` is a pure function of its explicit arguments. It requires no
  new exported type, constructor, flag, or capability — satisfying FR-003's "MUST NOT
  provide any capability to ... override ... the mapping": there is no way to reach this
  test call from `RunAgentSelect`, `main.go`, or any CLI argument, because
  `resolveAgainstRegistry` is unexported and only ever called by `Select` (with the real
  `mappedAgent` result) or by `internal/selection`'s own test file (with an explicit
  test-chosen target).
- **Alternatives considered**:
  - The `Selector{policy: testPolicy, ...}` struct-literal seam described above —
    rejected in this revision; while still package-private and still incapable of being
    reached from outside `internal/selection`, it required a `map[role.Role]agent.ID`
    field to exist on the production type at all, which is a larger surface than
    necessary once the "look up the target" and "resolve the target against the
    registry" responsibilities are split into two functions.
  - An exported `NewSelectorWithPolicy(policy, registry)` constructor — rejected; even
    though unused by any CLI path, an exported override constructor reads as exactly the
    "capability to ... override ... the mapping" FR-003 forbids, and invites future
    misuse from outside the package.
  - Constructing an `agent.Registry` whose fake `system.Executor` simply reports the
    policy's real target (`codex-cli`/`claude-code`) as never found — rejected as the
    *only* mechanism; that only reaches `agent_unavailable`, never
    `selection_inconsistent`, because the ID would still be part of the registry's fixed
    set (per FR-009's own distinction, restated in User Story 3, Acceptance Scenario 2).

## 5. Report stream placement: stdout, not the stderr split used by `agent run`

- **Decision**: Both the human-readable and machine-readable role-selection report are
  written to **stdout** (mirroring `ai-dev agents`), not split against stderr the way
  `agent run` splits its final report from live agent stdio.
- **Rationale**: `agent run`'s stderr placement exists solely to keep its one final
  report line out of the way of the *launched agent's own* live stdout/stderr streams
  (contracts/agent-run.md). Role selection launches no process and produces no
  agent-originated stream to protect stdout from — structurally it is a pure report
  command, exactly like `agents`, whose report already goes to stdout. Using stdout also
  makes the machine-readable contract simpler to consume (`ai-dev agent select --format
  json implementation` on stdout alone, no stream-splitting caveat to document).
- **Alternatives considered**: Matching `agent run`'s stderr-for-report convention for
  superficial "family" consistency — rejected; that convention is a solution to a
  specific problem (isolating `ai-dev`'s own report from a live subprocess's output) that
  does not exist here, and copying it would only add an unnecessary, undocumented-reason
  deviation from the simpler `agents` precedent for a command in the same "no process
  involved" shape.

## 6. Exit codes

- **Decision**: Reuse the exact three-tier convention already established by `doctor`,
  `agents`, and `agent run`: usage failure → `2`; recognized request, non-success
  outcome → `1`; recognized request, success outcome (`role_selected`) → `0`. A
  report-write failure (distinct from any of the four outcomes — see `plan.md`'s "Report
  Validation / Write Failure") follows the existing pure-report-command convention
  (`RunAgentsList`/`RunDoctor`) and returns `2`.
- **Rationale**: FR-017 explicitly defers to "an existing, already-approved ai-dev
  compatibility contract" for exit codes and introduces no new numeric requirement. The
  0/1/2 convention is exactly that existing, already-approved contract — applying it
  is not introducing a new implementation-specific code, it is reusing the one that
  already governs every other `ai-dev` subcommand. Reusing `RunAgentsList`/`RunDoctor`'s
  own write-failure exit code (`2`) for the same failure category, rather than inventing
  a distinct code, is the same reuse discipline applied one level further.
- **Alternatives considered**: A distinct exit code per non-`role_selected` outcome
  (e.g., `3` for `role_unsupported`, `4` for `agent_unavailable`, `5` for
  `selection_inconsistent`) — rejected; FR-017 only requires success-vs-not-success to be
  distinguishable by the "successful completion iff `role_selected`" rule, and the report
  itself (not the exit code) is the channel FR-010/FR-012 assign responsibility for
  distinguishing the four outcomes to. Inventing new codes would be an unrequested,
  implementation-specific addition FR-017 explicitly says not to introduce. Similarly, a
  distinct exit code for a report-write failure (e.g., matching `agent run`'s `1` instead
  of `agents`/`doctor`'s `2`) was considered and rejected — `agent select` is structurally
  a pure-report command like `agents`/`doctor`, not a live-process command like `agent
  run`, so it follows that convention instead (see `plan.md`'s "Report Validation / Write
  Failure").

## 7. Regression protection and `RunAgentSelect`'s actual dependencies

- **Decision**: `RunDoctor`, `RunAgentsList`, `RunAgentRun`, every file under
  `internal/checks`/`internal/doctor`, `internal/agent`, `internal/execution`, and
  `internal/report`'s existing `json.go`/`text.go`/`agentlist.go`/`executionreport.go`
  are not modified. `internal/app/doctor.go`'s `Run` gains exactly one more branch inside
  its existing `if args[0] == "agent"` block (see §1's correction and `plan.md`'s
  "Dispatcher Change" for the exact, corrected shape); the existing `run` handling, the
  block's existing usage-error fallback structure, and every other branch are otherwise
  unchanged. No `app.Run`, `RunAgentsList`, or `RunAgentRun` function signature changes,
  so `cmd/ai-dev/main.go` requires no edit at all (contrast with 002, which had to thread
  two new parameters through `Run` for the launcher/stdin it needed — this feature needs
  neither).
- **Correction to an earlier draft of this research**: an earlier draft stated
  "`RunAgentSelect` only needs a `system.Executor` (to build an `agent.Registry`) and the
  two output streams `Run` already has in scope." This is corrected: `RunAgentSelect`
  itself never receives a `system.Executor` at all. `app.Run` — which already holds the
  `system.Executor` it needs for `doctor`/`agents`/`agent run` — constructs the
  `agent.Registry` itself (via the existing `agent.NewRegistry(executor)` constructor,
  exactly as `RunAgentsList` and `RunAgentRun` already do) and passes only that
  `agent.Registry` value into `RunAgentSelect`. See `plan.md`'s "Dependency Boundary" for
  the full call-chain diagram and rationale (remediation finding 2).
- **Rationale**: `RunAgentSelect` only needs an `agent.Registry` (read-only, already
  discovery-scoped) and the two output streams `Run` already has in scope — it has no new
  dependency to thread through the call chain beyond that, so the minimum-blast-radius
  change is a single new dispatch branch, not a signature change. The existing
  `internal/app/doctor_test.go`, `internal/app/agents_test.go`,
  `internal/app/agentrun_test.go`, and `cmd/ai-dev/main_test.go` assertions are the
  regression gate: they must continue to pass unmodified, and the plan's task list must
  add new, additive test cases rather than edit existing ones. See `plan.md`'s
  "Regression Protection" for the full enumerated list (doctor check count, `agents`
  behavior, `agent run` behavior, canonical agent IDs, `Registry` availability semantics,
  execution outcomes/stream behavior).
- **Zero-process-execution regression proof, both mechanisms**: two independent
  execution mechanisms exist in this codebase — `system.Executor.Run` and
  `system.Launcher.Launch`. With `RunAgentSelect` taking neither a `system.Executor` nor
  a `system.Launcher`, both are structurally unreachable from `RunAgentSelect` itself.
  - `system.Launcher.Launch` is proven unreachable purely by parameter absence, and this
    is additionally confirmed behaviorally at the `app.Run` dispatch boundary: a shared
    fake `system.Launcher` (the same instance `agent run`'s own tests already use) is
    asserted to have a launch count of `0` after every `agent select` invocation across
    all outcomes, while the same fake's count is shown to increase for an `agent run`
    call in the same test file (proving the fake can detect a launch at all).
  - `system.Executor.Run` is unreachable from `RunAgentSelect` for the same
    parameter-absence reason, and is additionally already covered by
    `internal/agent/registry_test.go`'s own existing `recordingExecutor.runs` assertion
    (proving `agent.Registry.List()` — the only I/O `Select` performs — never calls
    `Run`). This plan adds one more `internal/app`-level test reusing that same
    `recordingExecutor` pattern, wired into `Run`'s `executor` parameter, to confirm the
    zero-`Run`-calls property also holds at the application dispatch boundary, not only
    inside `internal/agent`'s own package tests.
  This is the "one mechanism already made unreachable by the existing design, document
  and retain a boundary-level proof rather than adding unnecessary abstraction" treatment
  the remediation review asked for, applied to both mechanisms symmetrically, since this
  revision's dependency-boundary fix (finding 2) makes both equally unreachable by
  construction.
- **Alternatives considered**: Restructuring `Run`'s dispatch into a lookup table or a
  small internal command registry while adding `select` — rejected as an unjustified
  abstraction (Constitution III, planning's "do not introduce unnecessary abstractions");
  the existing flat `if` arms remain easy to read and the existing tests already exercise
  the flat structure directly (see §1's correction).

## 8. Machine-readable serialization

- **Decision**: `encoding/json`, following the exact structural pattern already
  established by `internal/report/executionreport.go` and `agentlist.go`: a private wire
  struct with `json` tags, encoded with `json.NewEncoder`. Fields that must be "absent"
  per outcome (FR-012) use `omitempty` on string-typed wire fields (an empty string
  encodes as a fully omitted key, not `""`), rather than pointers or a variant-shaped
  payload.
- **Rationale**: Consistency with the existing, already-reviewed report pattern; no new
  dependency; `omitempty` on plain strings is the simplest mechanism that satisfies "the
  machine-readable report exposes ... without requiring the reader to parse free-form
  text and without introducing unnecessary serialization detail" (FR-012) — the reader
  checks key presence, not a sentinel value.
- **Alternatives considered**: Pointer fields (`*string`) to distinguish "absent" from
  "empty" — rejected; every "absent" field in this feature's outcome model
  (`RequestedRole`, `SelectedAgentID`, `MappedAgentID`) is a role or agent identifier that
  is never legitimately an empty string when present, so a plain string with `omitempty`
  is unambiguous and avoids nil-pointer handling in the writer for no behavioral gain.

## 9. No new dependency

- **Decision**: stdlib only (`flag`, `strings`, `fmt`, `io`, `encoding/json`) — no
  `go.mod` change.
- **Rationale**: Nothing in this feature's scope (parsing one positional argument,
  building a map lookup, writing text/JSON) requires anything beyond what 001/002 already
  use. This is also directly checked by the "Quality Gates" security/scope audit gate in
  `plan.md` (no new third-party dependency).
- **Alternatives considered**: None warranted.
