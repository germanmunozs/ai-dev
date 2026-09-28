# Data Model: Coding Agent Execution

**Feature**: 002-coding-agent-execution
**Date**: 2026-09-27

This is the vendor-neutral domain shared by the listing and execution capabilities. It
mirrors the spec's Key Entities section with concrete Go shapes. Vendor-specific detail
(command construction, exit-code conventions) is deliberately excluded — see
`contracts/adapter-interface.md`.

## Coding Agent (`internal/agent`)

```go
package agent

type ID string

const (
    ClaudeCode ID = "claude-code"
    CodexCLI   ID = "codex-cli"
)

// Agent is a vendor-neutral description of one coding agent ai-dev knows how to work
// with. It carries no availability state — availability is a point-in-time query
// result (see AgentStatus), not a property of the agent itself.
type Agent struct {
    ID           ID     // immutable canonical identifier (FR-004, FR-022)
    DisplayLabel string // human-facing label only; never used for selection or reports
    binary       string // executable name used for local discovery (unexported: a
                         // vendor-neutral discovery *mechanism* over a vendor-specific
                         // *value*, analogous to internal/checks.ToolCheck's binary field)
}
```

- **Invariants**: `ID` is immutable and stable across releases (FR-004). Exactly two
  `Agent` values exist initially: `claude-code` (binary `claude`) and `codex-cli`
  (binary `codex`) — the same binary names `internal/app.RunDoctor` already uses for its
  `claude-code-tool`/`codex-cli-tool` checks, kept consistent on purpose.
- **Extensibility**: A future agent is one more `Agent` value in the registry's static
  list (see below) plus one more `Adapter` implementation. No field, method, or existing
  `Agent` value changes.

## Availability (`internal/agent`)

```go
type Availability uint8

const (
    Unavailable Availability = iota
    Available
)

// AgentStatus is one Agent paired with its point-in-time local discovery result.
type AgentStatus struct {
    Agent        Agent
    Availability Availability
}
```

- **Derivation**: `Availability` is computed by calling `system.Executor.LookPath` on
  `Agent.binary` at query time (FR-002) — never cached, never asserting auth or
  readiness (spec Assumptions).
- **Determinism**: Given no change to the local environment, repeated queries return the
  same `Availability` for the same `Agent` (FR-003) — guaranteed because the computation
  is a pure function of `LookPath`'s current result, with no internal state.

## Registry (`internal/agent`)

```go
// Registry enumerates every supported Agent and resolves one by ID.
type Registry struct {
    agents   []Agent // fixed, package-level list; registration order is report order
    executor system.Executor
}

func NewRegistry(executor system.Executor) Registry
func (r Registry) List() []AgentStatus        // every supported agent, stable order (FR-001, FR-003)
func (r Registry) Resolve(id ID) (Agent, bool) // FR-004; false means "unsupported"
```

- **Not a plugin system**: `agents` is a fixed literal slice inside the package, not a
  runtime-registered/discovered set (goal 1, goal 10's YAGNI clause). Adding an agent
  means adding one literal entry, not implementing a discovery/plugin mechanism.

## Execution Request (`internal/execution`)

```go
type Request struct {
    AgentID     agent.ID // as supplied by the caller, verbatim — may be unsupported
    Instruction string   // fully resolved, unmodified instruction text (FR-010)
}
```

- **Construction**: Built only after instruction-source resolution succeeds (see
  `internal/instruction` in `contracts/agent-run.md`); a `Request` value therefore
  always carries non-empty, unambiguous instruction text. Agent-ID *validity* is
  intentionally not enforced at construction — resolving/validating the ID against the
  `Registry` is the `Runner`'s job, because an unsupported ID must still produce a
  reportable `execution_failed` outcome that echoes the requested ID (FR-018), not a
  construction-time panic or a swallowed error.

## Execution Outcome / Report (`internal/execution`)

```go
type Outcome string

const (
    AgentSucceeded  Outcome = "agent_succeeded"
    AgentFailed     Outcome = "agent_failed"
    ExecutionFailed Outcome = "execution_failed"
)

// Report is the result of one recognized execution request, in both human- and
// machine-readable presentations (FR-013, FR-014, FR-018).
type Report struct {
    RequestedAgentID string  // exactly as supplied (FR-018), even if unsupported
    Outcome          Outcome
    Explanation      string  // required (non-empty) when Outcome != AgentSucceeded (FR-014); may be empty on success
}
```

- **Invariants**: `Outcome` is always exactly one of the three stable values. Code in
  this repository constructs reports with the three declared constants, and the final
  report writers validate the value before emitting either representation (a defined Go
  string type can still be explicitly constructed with another value).
  `RequestedAgentID` is always the string the caller supplied, never normalized/altered,
  so an unsupported ID still round-trips into the machine-readable report unchanged
  (FR-018's "an unsupported identifier MUST NOT be represented as a supported Coding
  Agent" — enforced by `RequestedAgentID` being a plain `string`, not an `agent.ID`
  resolved against the registry).

## Relationships

```
Registry --lists/resolves--> Agent --paired with--> Availability  == AgentStatus
Request  --targets (by ID)-->  Agent (via Registry.Resolve, may fail)
Runner   --executes-->        Request --produces-->  Report (Outcome + explanation)
```

- One `Request` maps to at most one process invocation and produces exactly one
  `Report` (FR-024). There is no persisted history between invocations (spec
  Assumptions) — `Report` is a return value, not a stored record.

## State Transitions (Execution Outcome)

There is no mutable state machine on `Report` itself — a `Report` is produced once, at
the end of the lifecycle described in `contracts/adapter-interface.md`. The lifecycle
states that lead to each terminal `Outcome` are:

| Lifecycle point                                   | Terminal Outcome   |
|----------------------------------------------------|---------------------|
| Unrecognized subcommand/flags                       | *(usage failure — no Report)* |
| Recognized request, unsupported `AgentID`           | `execution_failed`  |
| Recognized request, agent unavailable               | `execution_failed`  |
| Recognized request, missing/empty/ambiguous instruction | `execution_failed` |
| Process failed to launch                            | `execution_failed`  |
| Human-initiated cancellation (handled)               | `execution_failed`  |
| Process terminated abnormally (signal/crash)         | `execution_failed`  |
| Process exited normally, adapter classifies success  | `agent_succeeded`   |
| Process exited normally, adapter classifies failure  | `agent_failed`       |
