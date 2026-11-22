# Implementation Plan: Agent Role Selection

**Branch**: `feature/003-agent-role-selection` | **Date**: 2026-09-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-agent-role-selection/spec.md`

**Remediation note**: This plan was revised after an independent plan review identified
one critical and four important findings (plus two minor notes). That revision addressed
all of them without changing `spec.md`. See "Remediation Summary" at the end of this
document for the itemized disposition.

**Second remediation note (2026-09-29)**: A follow-up independent review of the
remediated plan identified exactly one remaining finding — **I3**: `selection_inconsistent`
lacked a concrete, executable application-level test path proving stream placement and
process completion semantics, and two test-plan statements about outcome/coverage
reachability were inaccurate. This second pass closes I3 and corrects both statements,
plus one report-write-failure wording overclaim, without changing `spec.md`. See "Final
Remediation Summary (I3 + Corrections)" at the end of this document.

## Summary

Add a capability that turns one explicitly requested development role (`specification`,
`planning`, `implementation`, `review`, `remediation`) into a reported selection of which
supported coding agent (from the `internal/agent` registry established by
002-coding-agent-execution) `ai-dev` would use for it right now —
`ai-dev agent select [--format text|json] <role>` — without ever invoking, launching,
authenticating, or reconfiguring any agent.

`internal/role` (new) defines the five canonical role identifiers and exact-match
recognition, with no dependency on any agent concept. `internal/selection` (new) holds
the fixed, built-in role-to-agent Selection Policy — represented as an unexported,
exhaustive `switch` function over the five canonical roles, **not** a mutable
package-level map — and a `Selector` that resolves one role value against that policy and
a supplied `agent.Registry` to produce exactly one of the four canonical outcomes
(`role_selected`, `role_unsupported`, `agent_unavailable`, `selection_inconsistent`).

`internal/app` gains one new file (`agentselect.go`, `RunAgentSelect`). `RunAgentSelect`
takes a **preconstructed `agent.Registry`**, not a `system.Executor`, and not a
`system.Launcher` — it has no dependency on `system.Executor`, `system.Launcher`, or
`internal/execution` at all, so it has no capability to execute a process (see
"Dependency Boundary" below). The existing top-level `app.Run` signature in
`internal/app/doctor.go` is unchanged; `Run`'s `agent` dispatch (currently a single
conditional, `args[1] != "run"` — see "Dispatcher Change" below, not a switch/case as an
earlier draft of this plan incorrectly described) gains one additional, independent
branch for `select` that constructs an `agent.Registry` from the executor `Run` already
holds, then calls `RunAgentSelect(args[2:], registry, stdout, stderr)`. `internal/report`
gains one new file for the role-selection human/machine report.

No existing file's behavior, signature-facing contract, or test assertions for `doctor`,
`agents`, or `agent run` change. The feature strictly implements role → select → report:
`RunAgentSelect` never receives or uses a `system.Launcher`, and cannot reach
`system.Executor.Run` either (it never receives a `system.Executor` — only the read-only
`agent.Registry` built from one) — this is an architectural guarantee, not just a
behavioral one, verified by both the function's parameter list and its import graph.

## Technical Context

**Language/Version**: Go 1.27.1 (matches existing `go.mod`; no change)

**Primary Dependencies**: Go standard library only (`flag`, `strings`, `encoding/json`,
`fmt`, `io`) — no new module dependency, consistent with the existing project and with
planning requirement 10

**Storage**: N/A — no persistence; each invocation is independent (spec Assumptions;
FR-024)

**Testing**: `go test` with the standard library `testing` package, following the
existing repo convention established by 001/002: white-box unit tests in the same
package for `internal/role` and `internal/selection` (mirroring `internal/agent`'s
`agent_test.go`/`registry_test.go` style). The full required coverage (see "Test
Strategy" below) includes, at minimum: all five canonical roles; exact,
case-sensitive recognition; all five fixed mapping entries; all four outcomes
(`role_selected`, `role_unsupported`, `agent_unavailable`, `selection_inconsistent`);
every usage-failure class in `contracts/agent-select.md`; the text report contract; the
machine-readable report contract; stream placement; completion/exit-code semantics; the
"no selected identity where prohibited" and "no fallback" invariants; determinism;
explicit zero-call evidence against both `system.Executor.Run` and
`system.Launcher.Launch`; deterministic filesystem non-mutation evidence; and
lower-level report/finishing tests for all four outcomes (including
`selection_inconsistent`) independent of the CLI. All automated tests use fake/local
discovery (`system.Executor`/`system.Launcher` test doubles) and require no live Claude
Code or OpenAI/Codex CLI service access.

**Target Platform**: Same as existing `ai-dev` — no platform-specific code is needed
(this feature performs no process supervision at all)

**Project Type**: Single Go module CLI (matches existing `cmd/`, `internal/` layout — no
new top-level project structure)

**Performance Goals**: Not a performance-sensitive feature; a role selection request is
one `agent.Registry.List()` call (a handful of `LookPath` lookups) and a report write

**Constraints**: Must not execute, launch, authenticate, or reconfigure any coding agent
regardless of outcome (FR-004, FR-020); must not create/modify/delete repository or
filesystem content (FR-024); must not change the observable behavior of `ai-dev agents`
or `ai-dev agent run` (FR-025); must not introduce routing, scoring, retries, fallback,
chaining, workflow state, or any mapping-management capability (FR-026); must remain
deterministic (FR-011)

**Scale/Scope**: Five fixed roles, two fixed mapped agent identities drawn from the
existing two-entry `agent.Registry` (`claude-code`, `codex-cli`); one request produces
exactly one report, no concurrency/coordination requirements (spec Edge Cases)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design below, and
re-checked again after this remediation (see "Constitution Check — Post-Remediation"
below).*

| Principle | Assessment |
|---|---|
| I. Specification First | Spec is approved (Human Specification Gate explicitly confirmed by the invoking command) and clarified (5/5 clarification questions resolved, checklist complete); this plan does not alter its behavioral contract. PASS |
| II. Architecture Before Framework | Package boundaries (`internal/role`, `internal/selection`) follow directly from the spec's own "role identity distinct from agent identity" and "selection policy distinct from execution" requirements (Key Entities; architecture goal), not from a preferred tool. PASS |
| III. Simplicity and YAGNI | No new abstraction beyond what FR-003/FR-009 require: one fixed policy (an exhaustive switch function, not a map, and not a struct field), one `Selector` reusing an already-constructed `agent.Registry`, one new CLI subcommand reusing the existing dispatch pattern. No plugin system, no routing, no scoring — all explicitly excluded by the spec. PASS |
| IV. Testable by Design | Every FR has a corresponding test seam (contracts' "Test seams" sections); a package-private seam tests an unexported registry-resolution helper with an explicit, deliberately unsupported agent identity — no mutable production policy is introduced to make this possible (see "Selection Policy Immutability" below). PASS |
| V. Security by Design | FR-020–FR-022 are enforced by construction: `RunAgentSelect` has no `system.Launcher` parameter and no `system.Executor` parameter (only a read-only `agent.Registry`), so it cannot start a process by any path; the report writers only ever emit role/agent identifiers and the fixed outcome vocabulary, never environment data. A dedicated security/scope audit gate (see "Quality Gates" below) checks this. PASS |
| VI. Observability by Design | Not a backend/distributed system; N/A beyond the report itself, which is the feature's whole purpose. |
| VII. Reproducibility and Automation | Quality gates below are scripted, version-controlled commands consistent with 001/002 (`gofmt`, `go vet`, `go test`, `go build`) plus a `git diff --check` whitespace gate and a security/scope audit gate; none require a live Claude or OpenAI service. PASS |
| VIII. AI-Assisted Engineering | This plan is itself a version-controlled artifact produced ahead of implementation, per the standard Spec Kit flow. PASS |
| IX. Human Authority | Plan does not assert Human Plan Gate approval; it is a proposal awaiting the normal review before `tasks`/implementation proceed. This remediation likewise does not claim that gate. PASS |

No violations requiring a Complexity Tracking entry.

## Quality Gates

*Addresses remediation finding: Quality Gates (REQUIRED).*

All gates below are automated, run locally against the repository checkout, and require
**no live Claude Code or OpenAI/Codex CLI service access** — they exercise only the Go
toolchain and this repository's own fake-backed test suite (`system.Executor` and
`system.Launcher` test doubles). None of these gates invent new security tooling; each
reuses a standard Go toolchain command or a plain repository-content check.

| Gate | Command | Pass condition |
|---|---|---|
| Formatting | `gofmt -l .` | No output (no file listed as needing formatting). |
| Vet | `go vet ./...` | Successful completion with no diagnostics. |
| Unit/integration tests | `go test ./...` | All tests pass. |
| Tests without cache | `go test -count=1 ./...` | All tests pass without relying on the test result cache. |
| Build | `go build ./cmd/ai-dev` | Successful build, no new `go.mod` dependency introduced. |
| Whitespace | `git diff --check` | No whitespace errors reported. |

### Security / scope audit gate

A review gate (manual code review checklist, run against the diff introduced by this
feature — not a new automated tool) verifying all of the following before merge:

- No new third-party dependency is introduced (`go.mod`/`go.sum` unchanged, or changed
  only by an unrelated toolchain bump).
- `internal/role` and `internal/selection` cannot execute an agent: neither package
  imports `internal/execution`, `internal/execution/adapters`, or references
  `system.Launcher`. `RunAgentSelect`'s parameter list contains no `system.Executor`, no
  `system.Launcher`, and no `context.Context` used for process supervision — only
  `agent.Registry` and the two output streams.
- No authentication step or network probe is introduced anywhere in `internal/role`,
  `internal/selection`, `RunAgentSelect`, or the new `internal/report` file — the only
  I/O is `agent.Registry.List()` (local `LookPath` only, already unmodified from 002) and
  writes to the caller-supplied `stdout`/`stderr`.
- No permission, sandbox, or approval configuration is read, written, or referenced.
- No secret, token, credential, or environment value appears in any code path or test
  fixture added by this feature (the report only ever contains role/agent identifiers
  and the four fixed outcome strings).
- No routing, retry, fallback, agent chaining, background execution, persistence,
  dynamic plugin loading, or MCP-orchestration behavior is introduced — `Selector.Select`
  is a single synchronous, stateless call producing one `Report`.

This gate is satisfied by code review against the criteria above plus the automated
tests in "Test Strategy" below (particularly the zero-call evidence against
`system.Executor.Run` and `system.Launcher.Launch`); it does not require introducing a
new static-analysis tool, linter, or CI job beyond what 001/002 already use.

## Dependency Boundary

*Addresses remediation finding: Remove execution capability from `RunAgentSelect`
(critical).*

The earlier draft of this plan proposed passing `system.Executor` directly to
`RunAgentSelect`. That parameter is too broad: `system.Executor` exposes both
`LookPath` (needed) **and** `Run` (a process-execution capability this feature must never
have — see FR-004, FR-020). This revision removes that dependency entirely.

`RunAgentSelect` receives a **preconstructed `agent.Registry`** instead:

```text
app.Run
  -> construct agent.Registry using existing discovery dependency (the system.Executor
     app.Run already holds, exactly as agents.go and agentrun.go already do via
     agent.NewRegistry(executor))
  -> RunAgentSelect(args, registry, stdout, stderr)
  -> selection.Selector (built from the same registry via selection.NewSelector(registry))
  -> registry.List()   (the only I/O Select performs — local LookPath lookups only)
```

- `app.Run` (in `internal/app/doctor.go`) is the only place in this feature's call chain
  that ever sees a `system.Executor`. It constructs `agent.NewRegistry(executor)` — the
  exact same, already-existing constructor `RunAgentsList` and `RunAgentRun` already use
  — and passes only the resulting `agent.Registry` value into `RunAgentSelect`.
- `RunAgentSelect` (`internal/app/agentselect.go`) has the signature
  `RunAgentSelect(args []string, registry agent.Registry, stdout, stderr io.Writer) int`.
  It has no `system.Executor`, no `system.Launcher`, no `context.Context`, and no
  `internal/execution` import at all. It cannot construct a `system.Launcher.Launch`
  call, and it cannot call `system.Executor.Run` because it never has a value of that
  type in scope.
- `selection.Selector` (`internal/selection/selector.go`) is constructed from that same
  `agent.Registry` via `NewSelector(registry)`. `Select` calls `registry.List()` — which,
  per `internal/agent/registry.go` (unmodified), only ever calls `executor.LookPath`,
  never `executor.Run` — exactly once per request.
- `agent.Registry.List()` itself is completely unmodified from 002; this feature adds no
  new method to `internal/agent`.

**Result**: selection has no capability to execute a process, provable by reading
`RunAgentSelect`'s parameter list and `internal/selection`'s import graph alone, without
needing to trust a test (though the tests in "Test Strategy" below additionally prove it
behaviorally).

## Selection Policy Immutability

*Addresses remediation finding: Fixed production policy must not be mutable (important).*

The earlier draft represented the fixed production role-to-agent mapping as a mutable
package-level `map[role.Role]agent.ID` variable. This revision replaces that with an
unexported, exhaustive `switch` function:

```go
// internal/selection/policy.go
package selection

// mappedAgent returns the fixed production mapping's target agent identity for a
// canonical role. This is the complete FR-003 table, expressed as code, not data —
// there is no map, no slice, no struct literal that could be mutated, and no
// constructor or accessor that exposes it for reading or writing from outside this
// file.
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

- `mappedAgent` is unexported, takes no configuration, and has no way to be constructed
  differently — it is code, not a value that can be replaced, appended to, or read
  generically from outside `internal/selection`.
- The exact mapping is preserved unchanged: specification → `claude-code`, planning →
  `claude-code`, implementation → `codex-cli`, review → `claude-code`, remediation →
  `codex-cli`.
- No configuration file, environment variable, persistence, plugin hook, or exported
  policy-mutation function is introduced anywhere in this feature.

### Test seam for `selection_inconsistent` (no mutable policy required)

The `selection_inconsistent` outcome depends only on whether a given target `agent.ID` is
present in `agent.Registry.List()`'s output — it does not depend on *how* that target was
produced. This revision separates "look up the role's mapped target" (`mappedAgent`,
production-only, never overridden) from "resolve a target agent identity against the
registry" into its own small, unexported, directly testable helper:

```go
// internal/selection/selector.go
package selection

// resolveAgainstRegistry determines the outcome for one already-recognized role and its
// mapped target agent identity, given the current registry state. It takes the target
// explicitly, so it can be tested with a deliberately unsupported identity without ever
// touching mappedAgent or constructing a Selector with a substitute policy.
func resolveAgainstRegistry(suppliedRole string, requested role.Role, target agent.ID, registry agent.Registry) Report
```

`internal/selection`'s own white-box test file (`selector_test.go`, `package selection`)
calls `resolveAgainstRegistry` directly with an explicit, deliberately unsupported target
(for example, `agent.ID("not-a-real-agent")`) and a normal fake-backed `agent.Registry`,
to exercise the `selection_inconsistent` branch deterministically:

```go
func TestResolveAgainstRegistryReportsSelectionInconsistentForAnUnregisteredTarget(t *testing.T) {
    got := resolveAgainstRegistry(
        "implementation", role.Implementation, agent.ID("not-a-real-agent"),
        agent.NewRegistry(fakeExecutor),
    )
    // assert got.Outcome == SelectionInconsistent, got.MappedAgentID == "not-a-real-agent",
    // got.SelectedAgentID == "" (never presented as selected)
}
```

This is the smallest possible seam: no `Selector` struct field holds a policy at all
(production `Select` calls `mappedAgent` directly, then `resolveAgainstRegistry`), so
there is no way to construct a `Selector` with a substitute mapping even by accident —
`selection_inconsistent` is reached in tests purely by choosing what explicit `target`
value to pass to a pure function, never by making the production mapping configurable.
A separate, dedicated production-path test asserts that all five of `mappedAgent`'s real
return values currently resolve against the real `internal/agent` registry (i.e.,
`selection_inconsistent` is unreachable under the approved mapping — SC-010's "0% under
the approved production mapping" half).

## Dispatcher Change

*Addresses remediation finding: Dispatcher description (important) and correction to
research.md.*

**Correction**: An earlier draft of this plan and of `research.md` described the current
`agent` dispatch as already using a `switch`/`case "run"` structure with a new
`case "select":` added beside it. That is not accurate. The current implementation in
`internal/app/doctor.go`'s `Run` is:

```go
if args[0] == "agent" {
    if len(args) < 2 || args[1] != "run" {
        fmt.Fprintln(stderr, "Usage: ai-dev agent run [--format text|json] <agent-id> [instruction]")
        return 2
    }
    return RunAgentRun(ctx, args[2:], executor, launcher, stdin, readerIsInteractive(stdin), "", stdout, stderr)
}
```

This is a single `if` condition combining "missing subcommand" and "unknown subcommand"
into one branch, not a `switch`/`case`. This plan's dispatch change is the smallest
additive change to this exact shape — adding one more subcommand check, not refactoring
the dispatcher's structure:

```go
if args[0] == "agent" {
    if len(args) >= 2 && args[1] == "run" {
        return RunAgentRun(ctx, args[2:], executor, launcher, stdin, readerIsInteractive(stdin), "", stdout, stderr)
    }
    if len(args) >= 2 && args[1] == "select" {
        registry := agent.NewRegistry(executor)
        return RunAgentSelect(args[2:], registry, stdout, stderr)
    }
    fmt.Fprintln(stderr, "Usage: ai-dev agent <run|select> [options]")
    return 2
}
```

Preserved exactly:

- `app.Run`'s existing signature (no new parameter).
- `doctor` dispatch (`args[0] == "doctor"`) — untouched.
- `agents` dispatch (`args[0] == "agents"`) — untouched.
- The existing `agent run` call and its exact arguments (`ctx, args[2:], executor,
  launcher, stdin, readerIsInteractive(stdin), "", stdout, stderr`) — unchanged.
- `args[2:]` handling: unambiguous for both `run` and `select` — each subcommand
  receives everything after its own name, exactly as `run` already does; `select`'s own
  usage validation (missing role, whitespace-only role, extra positionals, unknown
  flags) happens entirely inside `RunAgentSelect`, mirroring how `run`'s own usage
  validation happens entirely inside `RunAgentRun`.
- Missing-subcommand behavior (`ai-dev agent` with nothing after it) and
  unknown-subcommand behavior (`ai-dev agent foo`) remain a single fallback path with
  exit code `2` and a usage message on stderr — the same shape as today. The fallback
  message text is updated from a `run`-only message to one naming both recognized
  subcommands (`run`, `select`), since a message that only ever mentioned `run` would now
  be actively inaccurate for a codebase that also has `select`; this is the one
  intentionally-updated string, not a structural refactor.

This is the only change to `internal/app/doctor.go`. `RunDoctor` and the `doctor`
subcommand's own flag parsing/report-writing block are untouched.

## Terminology

*Addresses remediation finding: Terminology (minor).*

- `internal/role` is the **vendor-neutral role model**: it defines development-role
  identity with zero knowledge of any coding agent.
- `internal/agent` is the **generic canonical coding-agent model** (unchanged from 002):
  it defines coding-agent identity, discoverability, and availability without any
  role-shaped knowledge.
- `internal/selection` is the **policy boundary connecting roles to canonical
  supported-agent identities**. It is *not itself* described as "vendor-neutral" in this
  plan or its supporting documents — it is the layer that deliberately does know about
  both `role.Role` and `agent.ID`, precisely because resolving one into the other is its
  entire purpose. Vendor-specific identities (`claude-code`, `codex-cli`) never leak into
  `internal/role`.

## Report Validation / Write Failure

*Addresses remediation finding: Report validation / write failure (important).*

If `Selector.Select` has already produced one of the four semantic outcomes but
validating or writing that outcome's final report subsequently fails (for example, an
`io.Writer` error), the following holds:

- `ai-dev` completes **unsuccessfully**.
- A diagnostic is written to stderr when possible (the write attempt that failed was, by
  definition, capable of at least attempting a stderr write, mirroring the existing
  `RunAgentsList`/`RunDoctor` pattern of `fmt.Fprintf(stderr, "write ...: %v\n", err)`).
- The report-write failure **MUST NOT** be reclassified as any of the four selection
  outcomes — in particular, it **MUST NOT** become `selection_inconsistent`.
  `selection_inconsistent` remains reserved exclusively for a mapping-target/registry
  mismatch (FR-009); a write failure is unrelated to that condition and must not be
  conflated with it.
- `ai-dev` **MUST NOT** synthesize a replacement selection outcome for this request.
- No agent is launched as a result of this failure (this already follows from
  `RunAgentSelect` never holding a `system.Launcher`).
- This is an **application/reporting failure outside the four semantic selection
  outcomes**, exactly analogous to how `RunAgentsList` and `RunDoctor` already treat
  their own `report.Write*` failures as a distinct category from the report content
  itself.

**Convention used**: `ai-dev agent select` is a pure-report command in the same category
as `ai-dev agents` and `ai-dev doctor` (it never splits its report against a live
subprocess's stdio the way `ai-dev agent run` does — see `research.md` §5). It therefore
follows the existing pure-report-command write-failure convention already used by
`RunAgentsList`/`RunDoctor`: on a report-write error, print a diagnostic to stderr and
return exit code `2`. No fifth selection outcome is introduced; the four-outcome model in
`internal/selection` is unchanged.

**Precision on what is and is not claimed about partially-written output**: the
deterministic test for this convention (an `io.Writer` test double that always errors on
its first `Write` call) verifies zero report bytes reached `stdout` **for that specific,
controlled case** — an always-erroring writer that fails before producing any output.
This plan does **not** claim, as a general contract, that no report bytes can ever have
reached `stdout` for every possible way a write could fail (for example, a writer that
successfully writes part of a multi-write output and then fails partway through). The
approved specification does not require atomic rollback of partially written output, and
this plan does not introduce such a requirement. The general contract that does hold,
regardless of how or where a write fails, is exactly the list in "Report Validation /
Write Failure" above: unsuccessful completion, a diagnostic to stderr when possible, no
synthesized replacement selection outcome, and no reclassification as
`selection_inconsistent` or any other of the four outcomes. Deterministic failing-writer
tests (an always-erroring `io.Writer`) are part of the test strategy below, exercised at
the `internal/app` level around `RunAgentSelect`, and are scoped to proving that specific,
controlled case — not a general atomicity guarantee.

## Test Strategy

*Addresses remediation finding: Test strategy (important).*

This supersedes and extends the abbreviated "Testing" line in Technical Context above
with the full required coverage:

1. **`internal/role`**: all five canonical roles recognized; exact, case-sensitive
   matching (`"Implementation"` rejected); rejection of arbitrary/empty strings; no
   normalization/aliasing.
2. **`internal/selection`**: all five fixed mapping entries resolve correctly via
   `mappedAgent` against the real registry (production-path coverage, and the SC-010
   "0% under the approved mapping" evidence); `resolveAgainstRegistry` unit-tested
   directly for `role_selected`, `agent_unavailable`, and `selection_inconsistent`
   (via an explicit unsupported target — see "Selection Policy Immutability"); a
   determinism test (same inputs, repeated calls, byte-identical `Report`).
3. **`internal/report`** (new `roleselection.go`/`roleselection_test.go`): text and JSON
   rendering for **all four outcomes**, including `selection_inconsistent`, asserting the
   exact per-outcome field-presence table in `contracts/agent-select.md` — exercised by
   constructing `selection.Report` values directly (including a `SelectionInconsistent`
   one), never through the CLI. This layer verifies rendering, canonical outcome values,
   and field presence/absence only. It does **not** verify stream placement (which
   stream a byte reaches) or process-completion semantics (the exit code
   `RunAgentSelect` returns) — those are properties of `RunAgentSelect` itself, not of
   the renderer, and are verified separately by the `internal/app`-level test in item 4
   below (see "Application-Level `selection_inconsistent` Coverage").
4. **`internal/app`** (new `agentselect_test.go`):
   - Every usage-failure class from `contracts/agent-select.md`: no role token,
     whitespace-only role token, extra positional tokens, unknown flags, invalid
     `--format` value — each asserted to perform zero registry discovery and emit no
     report (mirroring `TestRunAgentsListUsageFailuresDoNotDiscover`).
   - **Three of the four outcomes are naturally reachable through the public CLI**
     (`role_selected`, `role_unsupported`, `agent_unavailable`), each dispatched through
     `Run` with a fake-backed `agent.Registry` supplying controlled availability state.
     `selection_inconsistent` is *not* reachable through the public CLI under the
     production mapping — by design (FR-009, SC-010; see `quickstart.md` §6), since the
     production mapping and the production registry are intentionally kept consistent.
     `selection_inconsistent` therefore has three layers of coverage instead, none of
     which requires the public CLI to reach it:
     1. `internal/selection`'s own controlled seam (`resolveAgainstRegistry` called
        directly with an explicit unsupported target — item 2 above);
     2. `internal/report`'s direct construction of a `SelectionInconsistent` `Report`
        value (item 3 above), for the report-rendering contract in isolation;
     3. **A concrete, executable `internal/app`-level test that calls `RunAgentSelect`
        directly** (not through `Run`/the public CLI's argument-parsing path) with a
        controlled **empty `agent.Registry`** — closing the application-level gap for
        stream placement and process-completion semantics. See "Application-Level
        `selection_inconsistent` Coverage" below for the full test path.
   - Text report contract, machine-readable report contract, stream placement (stdout
     only — see `research.md` §5), and completion semantics (exit codes 0/1/2 per
     `contracts/agent-select.md`).
   - "No selected identity where prohibited" and "no fallback": assertions that
     `SelectedAgentID` is absent for every non-`role_selected` outcome, and that no
     outcome other than `role_selected` triggers any further action.
   - Determinism: identical inputs against a fixed fake registry state yield
     byte-identical text/JSON output across repeated calls.
   - Deterministic failing-writer tests: an always-erroring `io.Writer` passed as
     `stdout`, asserting exit code `2`, a stderr diagnostic, and no reclassification into
     any of the four outcomes (per "Report Validation / Write Failure" above).
   - **Zero-process-execution evidence, covering both existing execution mechanisms**:
     - **Structural proof (all four outcomes, unconditionally)**: `RunAgentSelect`'s
       signature (`args []string, registry agent.Registry, stdout, stderr io.Writer`)
       contains no `system.Launcher` and no `system.Executor` at all — this holds
       regardless of what `agent.Registry` value it is given, including the empty one
       used by the `selection_inconsistent` test below. This single structural fact
       already covers all four outcomes; the behavioral proofs below add independent,
       trust-no-single-mechanism confirmation for the three outcomes that are dispatched
       through `Run`.
     - **Behavioral, `system.Launcher.Launch`, for the three publicly-dispatched
       outcomes**: a shared fake `system.Launcher` instance (the same one `agent run`'s
       own tests already use) is wired into `Run`, and a test asserts its launch count
       stays `0` across every `agent select` invocation dispatched through `Run` — i.e.,
       `role_selected`, `role_unsupported`, `agent_unavailable`, and every usage-failure
       case (`selection_inconsistent` is not part of this list, because it cannot be
       dispatched through `Run` with the real executor under the production mapping —
       see the corrected coverage description above) — while the same fake's count is
       proven to increase for an `agent run` call in the same test file, confirming the
       fake actually detects a launch rather than being trivially unreachable.
     - **Behavioral, `system.Executor.Run`, for the same three publicly-dispatched
       outcomes**: `agent.Registry.List()` itself is already proven, by `internal/agent`'s
       own existing `registry_test.go` (`recordingExecutor.runs` assertion), to call only
       `LookPath`, never `Run` — this feature reuses that existing regression proof
       rather than re-deriving it. This plan additionally adds one `internal/app`-level
       test using a `recordingExecutor` (mirroring `internal/agent/registry_test.go`'s
       own fake) wired into `Run`'s `executor` parameter, asserting its `Run` call count
       stays `0` across every `agent select` invocation dispatched through `Run` —
       proving the zero-execution property holds at the same application boundary the
       `Launcher` proof uses, not just inside `internal/agent`.
     - **`selection_inconsistent`'s no-execution evidence does not need a dispatch-boundary
       fake at all**: the "Application-Level `selection_inconsistent` Coverage" test below
       calls `RunAgentSelect` directly with no `system.Launcher`/`system.Executor` in
       scope to begin with (they are simply not parameters of the function under test),
       so its no-execution property follows immediately from the structural proof above.
       No execution dependency is added to that test merely to assert a zero-call count
       that the function's signature already makes impossible to violate.
   - Deterministic filesystem non-mutation evidence: using a controlled temporary
     directory/repository snapshot (for example, `os.MkdirTemp` plus a `git init` fixture
     or a plain file-tree snapshot — no real repository is ever touched), assert the
     snapshot is byte-identical before and after each of: `role_selected`,
     `role_unsupported`, `agent_unavailable`, and a representative usage failure.
     (`selection_inconsistent` is excluded from this CLI-level check because it is not
     reachable through the CLI under the production mapping — see item 3's
     `internal/report`-level coverage instead.)
5. **Regression** (see "Regression Protection" below): `internal/app/doctor_test.go`,
   `internal/app/agents_test.go`, `internal/app/agentrun_test.go`, and
   `cmd/ai-dev/main_test.go`'s existing assertions are unchanged and continue to pass
   unmodified; only additive dispatcher test cases are introduced.

All tests in this feature use fake/local `system.Executor`/`system.Launcher`
implementations (in-memory test doubles, following the existing `recordingExecutor` /
fake-launcher patterns already present in this repository's test suite) and require no
live Claude Code or OpenAI/Codex CLI service access.

## Application-Level `selection_inconsistent` Coverage

*Addresses remediation finding I3 (the one open finding of the follow-up independent
review): `selection_inconsistent` lacked a concrete, executable application-level test
path proving stream placement and process-completion semantics — the `internal/selection`/
`internal/report` seams from earlier remediation cover the outcome model and the report
renderer in isolation, but not `RunAgentSelect` itself, the actual function the public CLI
calls.*

**Seam used**: `RunAgentSelect` already receives a preconstructed `agent.Registry` (see
"Dependency Boundary") rather than a `system.Executor`. `agent.Registry` is a concrete
Go struct with unexported fields (`agents []Agent`, `executor system.Executor`) and
exactly one exported constructor, `NewRegistry(executor)`, which always populates
`agents` with the fixed, two-entry production set. There is no exported way to construct
a `Registry` holding a different `agents` slice from outside `internal/agent` — and this
plan does not add one (that would be exactly the kind of new production/test hook this
remediation must avoid).

There is, however, an **already-available, zero-cost seam that requires no change to
`internal/agent` at all**: the Go zero value of an exported struct type is always
constructible from any package, even when its fields are unexported, as long as no field
is named explicitly — `agent.Registry{}` is legal, ordinary Go syntax from
`internal/app`'s own test file. Its zero-valued `agents` field is `nil`, and
`Registry.List()`'s implementation (`internal/agent/registry.go`, unmodified) is:

```go
func (r Registry) List() []AgentStatus {
    statuses := make([]AgentStatus, 0, len(r.agents))
    for _, supported := range r.agents { // ranges zero times over a nil slice
        ...
    }
    return statuses
}
```

Ranging over a `nil` slice in Go performs zero iterations and never dereferences the
zero-valued (`nil`) `executor` field, so `agent.Registry{}.List()` deterministically
returns an empty, non-nil `[]AgentStatus{}` — a registry that, by construction, contains
no supported agent identities at all. For any of the five canonical roles, its fixed
`mappedAgent` target (`claude-code` or `codex-cli`) is therefore always absent from that
empty list, so `resolveAgainstRegistry` deterministically returns `SelectionInconsistent`
for every role, regardless of which role is chosen.

This satisfies every constraint on the seam:

- **No change to `internal/agent`**: `agent.Registry`'s definition, fields, and
  constructor are untouched; `agent.Registry{}` is ordinary use of the type's existing,
  already-exported zero value, not a new capability added to the package.
- **No exported test hook, no CLI configuration**: the test calls `RunAgentSelect`
  directly as a Go function, exactly the same way `internal/app/agentrun_test.go`
  already calls `RunAgentRun` directly; no new flag, environment variable, or exported
  constructor is introduced anywhere.
- **No mutation of the production fixed mapping**: `mappedAgent` is not touched, called
  with a substitute, or bypassed — the test exercises the real, production `mappedAgent`
  function end-to-end through the real `Select`/`resolveAgainstRegistry` path; only the
  *registry* input is a controlled empty value, exactly mirroring how the existing
  `agent_unavailable`/`role_selected` application-level tests already supply a
  controlled, non-default `agent.Registry` via a fake executor.
- **No new production abstraction**: nothing is added to `internal/app`,
  `internal/selection`, or `internal/agent` to make this possible — the seam is a test
  file calling an already-planned, already-exported constructor pattern (`T{}`) against
  an already-planned function signature.

### Test path — human-readable mode

An `internal/app/agentselect_test.go` test:

1. Constructs `stdout`/`stderr` as separate, empty `*bytes.Buffer` values.
2. Calls `RunAgentSelect([]string{"implementation"}, agent.Registry{}, &stdout, &stderr)`
   directly (any of the five canonical roles works identically; `implementation` is used
   for consistency with this contract's other examples).
3. Asserts:
   - The outcome rendered to `stdout` is exactly `selection_inconsistent`.
   - The canonical requested role (`implementation`) is present in the rendered line.
   - The mapped target agent identity (`codex-cli`, per FR-003's fixed table) is present.
   - No selected-agent identity (`agent=` presented as a *selection*, as opposed to a
     mapped/target identity) is present — matching the same "never presented as
     selected" invariant already proven for the `internal/selection`-level seam.
   - `stdout` is exactly the one report line (stream placement: stdout only, per
     `research.md` §5 — no split against stderr).
   - `stderr` is empty (`stderr.Len() == 0`): the semantic report was written
     successfully, so no diagnostic is emitted — this is distinct from, and MUST NOT be
     confused with, the separate report-write-failure test, which deliberately supplies
     a failing writer instead (see "Report Validation / Write Failure" below).
   - The `int` returned by `RunAgentSelect` is the existing "recognized request,
     non-`role_selected` outcome" exit code (`1`, per `contracts/agent-select.md`'s exit
     code table) — process completion is unsuccessful, and no new numeric exit code is
     introduced for this outcome.

### Test path — machine-readable mode

A second test in the same file:

1. Constructs fresh `stdout`/`stderr` buffers.
2. Calls `RunAgentSelect([]string{"--format", "json", "implementation"}, agent.Registry{},
   &stdout, &stderr)`.
3. Asserts:
   - `stdout` contains exactly one syntactically valid JSON object (decodes cleanly with
     `json.Unmarshal`/`json.Decoder`, and nothing else is present in the buffer).
   - `"outcome"` is exactly `"selection_inconsistent"`.
   - `"requested_role"` is present and equals `"implementation"`.
   - `"mapped_agent_id"` is present and equals `"codex-cli"`.
   - `"selected_agent_id"` is absent (key not present at all, per the existing
     `omitempty` field-presence contract in `contracts/agent-select.md`).
   - `stderr` is empty, for the same reason as the human-readable case.
   - The returned exit code is `1`, the same already-planned completion semantics as the
     human-readable case — `--format json` does not change completion semantics.

### Relationship to the rest of the test strategy

This test path is additive to, and does not replace, the existing `internal/selection`-
level (`resolveAgainstRegistry`) and `internal/report`-level (direct `Report{Outcome:
SelectionInconsistent}` construction) coverage from "Test Strategy" above — those remain
the right place to test the outcome-determination logic and the report-rendering logic
in isolation. This new test closes the one remaining gap: proving that `RunAgentSelect`
itself — the actual function `app.Run`'s dispatcher calls — produces the correct stream
placement and process-completion behavior for `selection_inconsistent`, without relying
on the public CLI's argument-parsing path to construct that state (which it structurally
cannot do under the production mapping — see "Test Strategy" item 4's corrected
description above).

## Regression Protection

*Addresses remediation finding: Regression protection (important).*

This feature must not change the observable behavior of anything built by 001 or 002.
The regression suite already in the repository is the gate; this plan adds only
narrowly additive dispatcher tests rather than rewriting any 002 test:

- **Exactly nine ordered doctor checks**: `internal/app/doctor_test.go`'s existing
  assertions over `RunDoctor`'s check order/count are unchanged and continue to pass;
  `RunDoctor` itself is not modified by this feature.
- **`ai-dev agents` behavior**: `internal/app/agents_test.go`'s existing assertions are
  unchanged; `RunAgentsList` is not modified.
- **`ai-dev agent run` behavior**: `internal/app/agentrun_test.go`'s existing assertions
  are unchanged; `RunAgentRun` is not modified, and the dispatcher change in "Dispatcher
  Change" above preserves its exact call and arguments.
- **Canonical agent IDs**: `agent.ClaudeCode` / `agent.CodexCLI` and
  `internal/agent/agent_test.go`'s assertions are unchanged; `internal/agent/agent.go` is
  not modified.
- **`Registry` availability semantics**: `internal/agent/registry_test.go`'s existing
  assertions (including its zero-`Run`-calls assertion) are unchanged;
  `internal/agent/registry.go` is not modified, and no new method is added to it.
- **Existing execution outcomes and stream behavior**: `internal/execution/runner_test.go`
  and the `internal/report` execution-report tests (`executionreport_test.go`) are
  unchanged; `internal/execution` is not imported by `internal/role`, `internal/selection`,
  or `internal/app/agentselect.go`.
- **`cmd/ai-dev/main_test.go`**: unchanged; `main.go`'s call to `app.Run` needs no new
  argument because `app.Run`'s signature does not change.

## Project Structure

### Documentation (this feature)

```text
specs/003-agent-role-selection/
├── plan.md              # This file ($speckit-plan command output; revised by remediation)
├── research.md          # Phase 0 output (revised by remediation)
├── data-model.md        # Phase 1 output (revised by remediation)
├── quickstart.md        # Phase 1 output (revised by remediation)
├── contracts/           # Phase 1 output (revised by remediation)
│   ├── agent-select.md
│   └── selection-policy.md
├── checklists/
│   └── requirements.md  # Already present (Human Specification Gate input) — unchanged
└── tasks.md             # Phase 2 output ($speckit-tasks command - NOT created by $speckit-plan)
```

### Source Code (repository root)

```text
cmd/ai-dev/
├── main.go                    # UNCHANGED — app.Run's signature does not change
└── main_test.go                # UNCHANGED assertions; MAY gain new dispatch cases (additive only)

internal/app/
├── doctor.go                    # MODIFIED: the existing "agent" branch's single `if
│                                 #   len(args) < 2 || args[1] != "run"` condition gains
│                                 #   one more independent "select" branch (see
│                                 #   "Dispatcher Change" above); RunDoctor and every
│                                 #   other branch are byte-for-byte unchanged
├── doctor_test.go                # UNCHANGED (regression gate)
├── agents.go / agents_test.go     # UNCHANGED (regression gate)
├── agentrun.go / agentrun_test.go  # UNCHANGED (regression gate)
├── agentselect.go                  # NEW: RunAgentSelect(args []string, registry
│                                    #   agent.Registry, stdout, stderr io.Writer) int —
│                                    #   usage validation, Selector wiring, text/json
│                                    #   rendering, exit-code mapping. No system.Executor,
│                                    #   system.Launcher, or context.Context parameter.
└── agentselect_test.go              # NEW

internal/role/
├── role.go                      # NEW: Role type, five canonical constants, Recognized(value)
└── role_test.go                  # NEW

internal/selection/
├── outcome.go                    # NEW: Outcome type + four constants, Report struct
├── policy.go                      # NEW: unexported mappedAgent(role.Role) (agent.ID,
│                                    #   bool) — an exhaustive switch, not a map (see
│                                    #   "Selection Policy Immutability" above)
├── selector.go                     # NEW: Selector (NewSelector, Select) plus the
│                                    #   unexported resolveAgainstRegistry helper
└── selector_test.go                 # NEW: includes the controlled selection_inconsistent
                                      #   seam via resolveAgainstRegistry with an explicit
                                      #   unsupported target (never touches mappedAgent)

internal/report/
├── json.go / text.go               # UNCHANGED (doctor's report only)
├── agentlist.go / agentlist_test.go   # UNCHANGED (regression gate)
├── executionreport.go / executionreport_test.go # UNCHANGED (regression gate)
├── roleselection.go                    # NEW: text + JSON rendering for Role Selection Outcome
└── roleselection_test.go                # NEW — covers all four outcomes directly,
                                           #   including selection_inconsistent

internal/agent/                     # UNCHANGED — Registry, Agent, Availability all reused as-is
internal/execution/                 # UNCHANGED — no execution path is exercised by this feature
internal/checks/, internal/doctor/  # UNCHANGED
```

**Structure Decision**: Single Go module, extending the existing flat `cmd/` +
`internal/<domain>` layout from 001/002 — no new top-level directory. Two new, narrow
domain packages mirror the existing `internal/agent` + `internal/execution` split:
`internal/role` is the vendor-neutral role-identity model (analogous to `internal/agent`
being the generic canonical coding-agent model), and `internal/selection` is the policy
boundary connecting roles to canonical supported-agent identities (analogous to
`internal/execution` being the execution *lifecycle* model) — kept as a clearly separate
package from `internal/execution` precisely because the spec requires selection policy to
stay separate from execution (architecture goal; see "Terminology" above).
`internal/agent` is reused unmodified: `agent.Registry.List()` (already used by both
`internal/app`'s listing command and `internal/execution.Runner`'s availability check) is
reused a third time by `internal/selection.Selector`, with no new method added to
`internal/agent`.

## Complexity Tracking

*No entries — Constitution Check recorded no violations requiring justification, before
or after this remediation.*

## Constitution Check — Post-Remediation

*Addresses remediation finding: Constitution (required re-run).*

| Principle | Assessment |
|---|---|
| I. Specification First | Unchanged from the pre-remediation check: `spec.md` is untouched by this remediation, and this plan continues to implement exactly its behavioral contract. PASS |
| II. Architecture Before Framework | Strengthened: the Dependency Boundary section now makes "selection policy separate from execution" a provable property of `RunAgentSelect`'s parameter list (no `system.Executor`/`system.Launcher`), not just of `Selector`'s internals. PASS |
| III. Simplicity and YAGNI | The switch-based policy and the `resolveAgainstRegistry` seam are each smaller than the map-based/struct-field alternatives they replace (no map, no extra struct field, no exported override constructor). No new abstraction was added to satisfy this remediation — the changes are strict simplifications. PASS |
| IV. Testable by Design | `selection_inconsistent` remains fully testable via `resolveAgainstRegistry` at the `internal/selection` level, with an even smaller seam than the prior `Selector{...}` literal-construction approach, and without any production configurability. *(Updated by the second remediation pass, 2026-09-29 — see "Constitution Check — Final Remediation" below: this is no longer the only layer of testability for `selection_inconsistent`; an application-level test now calls `RunAgentSelect` directly with the Go zero value `agent.Registry{}`, closing the gap the follow-up review identified for stream-placement and process-completion coverage.)* PASS |
| V. Security by Design | Strengthened: `RunAgentSelect` now has no `system.Executor` reference at all (previously it would have), closing the gap the independent review identified. The new security/scope audit gate in "Quality Gates" makes this an explicit, checked property rather than an implicit one. PASS |
| VI. Observability by Design | Unchanged; N/A beyond the report itself. |
| VII. Reproducibility and Automation | Strengthened: the Quality Gates section now enumerates every gate explicitly, including `go test -count=1 ./...`, `go build ./cmd/ai-dev`, and `git diff --check`, none of which require live service access. PASS |
| VIII. AI-Assisted Engineering | Unchanged: this remediation is itself a version-controlled artifact produced ahead of implementation/tasks. PASS |
| IX. Human Authority | Unchanged: this remediation does not claim Human Plan Gate approval, does not generate tasks, and does not implement code. PASS |

No complexity exception is required by this remediation — every change either removes a
capability (`system.Executor` from `RunAgentSelect`) or replaces one representation with
a strictly simpler one (switch function instead of mutable map); nothing added new
surface area beyond what the nine findings required.

## Remediation Summary

Planning artifacts changed by this remediation: `plan.md`, `research.md`,
`data-model.md`, `contracts/agent-select.md`, `contracts/selection-policy.md`,
`quickstart.md`. `spec.md` and `checklists/requirements.md` are unchanged. No `tasks.md`
was created or modified. No code was implemented. The Human Plan Gate is not claimed as
approved by this remediation.

| # | Finding | Disposition |
|---|---|---|
| 1 | Quality Gates missing | Added "Quality Gates" section (six automated gates + security/scope audit gate; no live-service requirement stated) |
| 2 | `RunAgentSelect` too broad (critical) | `RunAgentSelect` now takes `agent.Registry`, not `system.Executor`; see "Dependency Boundary" |
| 3 | Mutable production policy | Replaced with unexported exhaustive-switch `mappedAgent`; test seam is `resolveAgainstRegistry` with an explicit unsupported target — see "Selection Policy Immutability" |
| 4 | Test strategy underspecified | Full enumerated coverage added — see "Test Strategy" |
| 5 | Dispatcher description inaccurate | Corrected to the actual `if args[1] != "run"` conditional; smallest additive change documented — see "Dispatcher Change" |
| 6 | Terminology (`internal/selection` called "vendor-neutral") | Corrected — see "Terminology" |
| 7 | Report validation / write failure undocumented | Documented as an application/reporting failure outside the four outcomes, using the existing pure-report-command convention — see "Report Validation / Write Failure" |
| 8 | Regression protection | Explicit enumerated list added — see "Regression Protection" |
| 9 | Constitution re-run | Re-run in full — see "Constitution Check — Post-Remediation" |

This remediation does not implement code, does not generate `tasks.md`, does not modify
`internal/constitution` or any constitution document, does not configure extension hooks,
and does not claim Human Plan Gate approval.

## Constitution Check — Final Remediation (2026-09-29)

*Re-run specifically for Principle IV per the second remediation pass, addressing
finding I3. The other eight principles are unaffected by this pass — see the assessments
in "Constitution Check — Post-Remediation" above, all of which remain PASS and are not
restated here.*

| Principle | Assessment |
|---|---|
| IV. Testable by Design | `selection_inconsistent` now has three independent, accurately-described layers of coverage instead of two: (1) `internal/selection`'s `resolveAgainstRegistry` seam (outcome-determination logic in isolation); (2) `internal/report`'s direct `Report{Outcome: SelectionInconsistent}` construction (rendering contract in isolation); (3) `internal/app`'s direct `RunAgentSelect` call with the Go zero value `agent.Registry{}` (the actual application boundary — stream placement and process-completion semantics). No new production abstraction, exported constructor, CLI flag, or test hook was added to reach layer 3 — it uses `agent.Registry`'s pre-existing, already-exported zero value, exactly as legal from any Go package as it was before this remediation. PASS |

No complexity exception is required: layer 3 adds a test file, not a production capability. `internal/agent`, `internal/selection`, and `internal/app`'s production code are all unmodified by this pass — only `plan.md`, `contracts/agent-select.md`, `contracts/selection-policy.md`, and `quickstart.md` change (`research.md` and `data-model.md` were unchanged by this pass — see "Final Remediation Summary (I3 + Corrections)" below).

## Final Remediation Summary (I3 + Corrections)

A follow-up independent review of the remediated plan (the "Remediation Summary" above)
found the remediation's critical finding and three of its four important findings, plus
both minor findings, correctly and durably resolved. It identified exactly **one**
remaining finding, plus two inaccurate test-plan statements and one wording overclaim
that needed correcting. Using the reviewer's own labels, mapped onto this plan's earlier
finding numbers for traceability:

| Reviewer label | Corresponds to | Status after this pass |
|---|---|---|
| C1 (critical) | Finding 2 — `RunAgentSelect` too broad | **Intact**, unchanged by this pass |
| I1 (important) | Finding 3 — mutable production policy | **Intact**, unchanged by this pass |
| I2 (important) | Finding 4 — test strategy (general structure) | **Intact**, extended (not contradicted) by this pass |
| I3 (important) | *New* — `selection_inconsistent` lacked a concrete, executable application-level test path | **Resolved by this pass** — see "Application-Level `selection_inconsistent` Coverage" |
| I4 (important) | Finding 7 — report validation / write failure | **Intact in substance**; one wording overclaim corrected by this pass (see below) |
| M1 (minor) | Finding 6 — terminology | **Intact**, unchanged by this pass |
| M2 (minor) | Finding 5 — dispatcher description | **Intact**, unchanged by this pass |

Findings 1 (Quality Gates), 8 (Regression Protection), and 9 (Constitution re-run) from
the original nine-item disposition table were procedural completeness requirements, not
defects, and remain satisfied unchanged — they are not part of the reviewer's C1/I1–I4/
M1–M2 defect classification.

### I3 — resolved

`selection_inconsistent` now has a concrete, executable `internal/app`-level test path:
`RunAgentSelect` is called directly with a controlled, empty `agent.Registry{}` (the Go
zero value — no change to `internal/agent`, no exported test hook, no CLI configuration,
no mutation of the production fixed mapping). Both human-readable and machine-readable
modes are covered, verifying outcome, canonical-role presence, mapped-target-agent
presence, selected-agent-identity absence, stdout-only stream placement, an empty stderr
for the successful-write case, and unsuccessful process completion using the
already-planned exit code (`1`) — no new numeric exit code was introduced. Full detail in
"Application-Level `selection_inconsistent` Coverage" above.

### Two inaccurate coverage claims — corrected

1. **"All four outcomes reachable from the CLI"**: corrected. The public CLI (argument
   parsing → `Run` → a real executor-backed registry) naturally reaches only three
   outcomes: `role_selected`, `role_unsupported`, `agent_unavailable`.
   `selection_inconsistent` is verified through the controlled, direct `RunAgentSelect`
   application-level test described above, precisely because the production fixed
   mapping and the production registry are intentionally kept consistent (FR-009,
   SC-010) — see "Test Strategy" item 4's corrected description.
2. **"No-execution coverage across all four outcomes" (dispatch-boundary phrasing)**:
   corrected. The `Run`-dispatch-boundary fake-`Launcher`/fake-`Executor` behavioral
   proofs cover exactly the three outcomes (plus usage failures) that are actually
   dispatched through `Run`. `selection_inconsistent`'s no-execution property does not
   need, and does not get, a dispatch-boundary fake — it follows immediately and
   unconditionally from the structural proof (`RunAgentSelect`'s signature has no
   `system.Launcher`/`system.Executor` parameter at all, regardless of which
   `agent.Registry` value it is given). See "Test Strategy"'s corrected
   "Zero-process-execution evidence" subsection and `contracts/selection-policy.md`'s
   "Never-launches / never-executes regression proof" section.

### Report-write-failure wording — corrected

"With no report having reached stdout" (a universal claim about every possible
write-failure mode) is replaced with a precise distinction: the deterministic
always-erroring-writer test verifies zero report bytes reached `stdout` *for that
specific, controlled case*; the general contract (unsuccessful completion, a stderr
diagnostic when possible, no synthesized replacement outcome, no reclassification as
`selection_inconsistent`) holds regardless of failure mode, without claiming atomic
rollback of partially-written output — which the approved specification does not
require and this plan does not introduce. See "Report Validation / Write Failure" above.

### Confirmations

- No new production abstraction or test-only production hook was introduced by this
  pass: `agent.Registry{}` is the pre-existing zero value of an already-exported type;
  no field, method, constructor, flag, or environment variable was added anywhere.
- `spec.md` and `checklists/requirements.md` are unchanged by this pass.
- No `tasks.md` was created or modified; no code was implemented; the constitution
  document itself and extension hooks are untouched; the Human Plan Gate is not claimed
  as approved.
- Planning artifacts changed by this pass: `plan.md`, `contracts/agent-select.md`,
  `contracts/selection-policy.md`, `quickstart.md`. `research.md` and `data-model.md`
  contained none of the inaccurate statements this pass corrects and needed no change.

This plan is ready for a further independent plan re-review.
