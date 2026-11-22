# Research: Coding Agent Execution

**Feature**: 002-coding-agent-execution
**Date**: 2026-09-27

This document resolves every technical unknown needed before design. Each entry is a
decision, not an open question — the spec's clarification session already closed the
behavioral ambiguities; what remains here is implementation-shape research.

## 1. Process streaming and cancellation mechanism

- **Decision**: Use `os/exec.CommandContext` with `Cmd.Cancel` set to send `os.Interrupt`
  (SIGINT) to the child and `Cmd.WaitDelay` set to a short fixed grace period (e.g. 5s)
  after which the stdlib force-kills the process group leader. `Cmd.Stdout`/`Cmd.Stderr`
  are wired directly to the destination `io.Writer`s so the OS pipes data through as the
  child produces it — no buffering layer in `ai-dev`.
- **Rationale**: `Cmd.Cancel` and `Cmd.WaitDelay` (stdlib since Go 1.20) are exactly the
  "forward cancellation, then guarantee the child has stopped" primitive FR-025 requires,
  with no external dependency. Wiring `Stdout`/`Stderr` directly (rather than reading and
  re-writing in a loop) is what makes "observable before exit" (FR-011, SC-009) true by
  construction: the kernel pipe delivers bytes as soon as the child flushes them.
- **Alternatives considered**:
  - Manual `signal.Notify` + `Process.Signal` + a separate hard-kill timer: reimplements
    what `Cmd.Cancel`/`WaitDelay` already does; rejected as unnecessary complexity (YAGNI).
  - Reading child stdout/stderr into buffers and re-emitting after completion: violates
    the live-streaming requirement outright; rejected.

## 2. Distinguishing normal exit, abnormal termination, and handled cancellation

- **Decision**: `system.Handle.Wait` returns an `ExitStatus` containing `Canceled`,
  `Exited`, and `ExitCode`. `OSLauncher` records `Canceled` only when its `Cmd.Cancel`
  function successfully delivers `os.Interrupt` to an active process. The runner treats
  `Canceled == true` as `execution_failed` before considering `Exited` or asking the
  vendor adapter to classify the exit code. If cancellation is observed only after the
  process has completed naturally, interrupt delivery reports that the process is done,
  `Canceled` remains false, and the natural `Exited`/`ExitCode` result is preserved.
  Otherwise, a process that did not exit normally is `execution_failed`; a normal exit
  is handed to the vendor adapter for `agent_succeeded`/`agent_failed` classification.
- **Rationale**: Recording successful interrupt delivery distinguishes handled
  cancellation from both natural completion and unrelated abnormal termination. Giving
  that recorded cancellation precedence prevents a process that exits after handling an
  interrupt from being mistaken for a vendor-reported result, while preserving a natural
  result when cancellation races with an already-completed process. The mechanism uses
  `Cmd.Cancel`, `os.ErrProcessDone`, and `os.ProcessState` without a new dependency and
  matches the `ExitStatus` lifecycle in `contracts/adapter-interface.md`.
- **Alternatives considered**: Treating any non-zero wait error as `execution_failed`
  was rejected — it would misclassify a normal non-zero exit (the agent's own reported
  failure) as an `ai-dev` failure, violating FR-013(b).

## 3. Availability discovery

- **Decision**: Reuse the existing `system.Executor.LookPath` method (already used by
  `internal/checks.ToolCheck` for the doctor's tool checks) to determine agent
  availability. No new discovery mechanism is introduced.
- **Rationale**: FR-002 defines "available" as exactly what `LookPath` already checks
  (standard executable lookup, no auth/network/readiness probing). Reusing the existing
  `system.Executor` interface avoids a parallel discovery path and keeps `internal/checks`
  and `internal/doctor` completely untouched.
- **Alternatives considered**: A new `agent`-specific probing interface — rejected as
  duplication of an already-correct, already-tested mechanism.

## 4. Vendor invocation mode (Claude Code / Codex CLI)

- **Decision**: Each adapter invokes its CLI's own non-interactive, single-instruction
  execution mode — the mode each vendor already documents for scripted/one-shot use
  (for example, a "print"/"exec"-style mode that takes the instruction, runs to
  completion in the current working directory, and exits with a status) rather than
  their default interactive REPL. No flag that skips, disables, or auto-answers the
  vendor's own approval or sandbox prompts is passed. Concrete flag names are pinned
  during implementation (`tasks.md`) against each CLI's own `--help` output at that
  time, since exact flags are explicitly out of scope for the spec (see spec
  Assumptions) and may need to be reconfirmed against a specific installed version.
- **Rationale**: An execution request must have an observable termination (so `ai-dev`
  can classify an outcome and exit deterministically); the default interactive REPL of
  either tool has no such deterministic termination point for a single instruction.
  A one-shot/print mode satisfies "run against the current project with exactly the
  supplied instruction" (FR-010) while still surfacing the vendor's own prompts through
  the passed-through stdio (FR-016), since these modes do not disable approval behavior
  — they only avoid entering a persistent chat loop.
  Not calling this a `NEEDS CLARIFICATION` for the plan: the constitution's Simplicity
  principle and the spec's own assumptions place exact flag selection in the
  implementation, not the architecture. The plan's contracts (see `contracts/`) fix the
  adapter *interface*, not the flags.
- **Alternatives considered**:
  - Driving the interactive REPL via a pseudo-TTY and scripted input: far higher
    complexity, fights the "minimum required arguments" and "native behavior" goals;
    rejected.
  - Hard-coding flags today without re-verification: rejected as a correctness risk the
    plan should not paper over; deferred to task-time verification instead.

## 5. Exit-code-to-outcome classification default

- **Decision**: Both initial adapters use the conventional rule "exit code `0` →
  `agent_succeeded`; any other normal exit code → `agent_failed`," expressed as each
  adapter's own `Classify` method rather than shared logic in the generic execution
  model.
- **Rationale**: Both CLIs follow the common Unix convention. Implementing this as a
  per-adapter method (not a shared default) is what satisfies goal 1 and goal 10: a
  future vendor with a different convention (e.g., structured output requiring
  content inspection) implements its own `Classify` with no change to the generic
  runner, the `Outcome` values, or already-supported adapters.
- **Alternatives considered**: A shared default classification function in the generic
  model with per-vendor overrides — rejected; it would put vendor-shaped knowledge
  (exit-code convention) inside the "vendor-neutral" layer, contradicting goal 1.

## 6. Interactive-stdin detection

- **Decision**: Detect whether standard input is a terminal using
  `os.Stdin.Stat()` and testing `Mode()&os.ModeCharDevice != 0` — true stdlib, no
  dependency.
- **Rationale**: This is the same technique the Go standard library documents for this
  purpose and requires no new module (satisfies goal 9's dependency preference). It is
  behind a small seam (`instruction` package accepts a pre-computed `bool` rather than
  probing `os.Stdin` itself) so tests never need a real terminal.
- **Alternatives considered**: `golang.org/x/term.IsTerminal` — rejected as an
  unnecessary external dependency for a check the standard library already permits.

## 7. Machine-readable serialization

- **Decision**: `encoding/json`, following the exact structural pattern already
  established by `internal/report/json.go` (a private wire struct with `json` tags,
  encoded with `json.NewEncoder`).
- **Rationale**: Consistency with the existing, already-reviewed doctor report pattern;
  no new dependency; automated callers get the same one-object-per-line encoding style
  they already get from `ai-dev doctor --format json`.
- **Alternatives considered**: None warranted — this is a direct continuation of an
  established, working pattern.

## 8. Command dispatch structure

- **Decision**: Extend the existing flat `switch`-style dispatch in `internal/app`'s
  top-level `Run` function with two more recognized shapes (`agents ...` and
  `agent run ...`), following the same `flag.NewFlagSet` + manual usage-error pattern
  already used for `doctor`. No CLI framework is introduced.
- **Rationale**: Directly satisfies goal 9 ("do not introduce a CLI framework merely for
  this feature if the existing CLI structure can satisfy the requirements cleanly") —
  three subcommands with simple flag sets is exactly what the existing structure already
  handles.
- **Alternatives considered**: Adopting `spf13/cobra` or similar — rejected; no
  requirement (subcommand nesting, shell completion, help generation beyond what's
  already hand-rolled) justifies the new dependency.

## 9. New system-boundary interface vs. extending `system.Executor`

- **Decision**: Add a new interface, `system.Launcher`, alongside the existing
  `system.Executor` (which is left completely unchanged). `Launcher` is responsible for
  starting a process with live-streamed stdio and a waitable handle that reports normal
  vs. abnormal termination; `Executor` continues to serve only `LookPath` and the
  captured-output `Run` used by doctor checks.
- **Rationale**: `Executor.Run` returns fully captured `(string, error)` — it cannot
  express live streaming, and every existing caller (`internal/checks`, `internal/app`
  doctor tests) is written against that captured-output contract. Extending `Executor`
  with new methods would force every existing fake/implementation to grow methods it
  never uses, for no behavioral benefit, and would touch files this feature has no
  reason to touch. A separate, narrow interface keeps the doctor's contract and tests
  byte-for-byte untouched (goal 11) while giving execution exactly the seam it needs
  (goal 8: fakeable launch failure / success / failure / cancellation).
- **Alternatives considered**: Extending `system.Executor` — rejected for the reasons
  above. A single mega-interface covering both use cases — rejected as an unjustified
  abstraction over two genuinely different responsibilities (inspect vs. supervise).

## 10. Where usage failure ends and `execution_failed` begins

- **Decision**: A usage failure is exactly: unrecognized subcommand, unknown/invalid
  flags, or a positional-argument shape that does not even identify which agent is being
  targeted (e.g., `agent run` with no agent-id token at all, or more positional tokens
  than the grammar allows). Once an agent-id token is present — regardless of whether
  that ID is supported — the request is "recognized" and every subsequent problem
  (unsupported ID, unavailable agent, missing/empty/whitespace-only instruction,
  ambiguous dual-source instruction) is reported as `execution_failed` through the
  normal report path, per FR-013(c) ("ai-dev rejected the recognized execution
  request").
- **Rationale**: This is a direct, literal reading of FR-026: syntax/option-parsing
  errors that prevent recognizing a request are usage failures outside the outcome
  model; FR-007–FR-009 are given as the recognized-request validation failures that
  become `execution_failed`. The ambiguous-instruction rule (FR-005) is the same kind of
  post-recognition validation failure as FR-007–FR-009 (it rejects an otherwise
  well-formed, recognized request), so it is classified the same way for consistency;
  nothing in the spec's outcome model (FR-013) leaves room for a fourth bucket.
- **Alternatives considered**: Treating dual-source ambiguity as a usage failure —
  rejected because the request *is* recognized (a specific agent-id was named); the spec
  reserves usage failures for requests that cannot even be recognized (FR-026), and
  FR-018 requires the machine-readable report to expose "the requested agent identifier
  exactly as supplied" for exactly this kind of rejected-but-recognized request.

## 11. Regression protection for the existing `doctor` contract

- **Decision**: `internal/checks`, `internal/doctor`, `internal/report/json.go`,
  `internal/report/text.go`, and `RunDoctor` are not modified. The top-level `Run`
  dispatcher gains a `system.Launcher` and an `io.Reader` (stdin) parameter to support
  the new subcommands; every existing call site (`cmd/ai-dev/main.go`,
  `internal/app/doctor_test.go`, `cmd/ai-dev/main_test.go`) is updated mechanically to
  pass the two new arguments, with no change to any assertion or expected value in those
  tests. The existing `cmd/ai-dev/main_test.go` subprocess contract test and
  `internal/app/doctor_test.go` unit tests continue to run unmodified in assertions and
  must continue to pass as-is — this is the regression gate for goal 11.
- **Rationale**: `Run`'s signature is the only touch point the new subcommands need; it
  is an internal Go function, not a published API, so a mechanical, additive signature
  change (two new parameters) is the minimum-blast-radius way to add the capability
  without duplicating the existing usage/dispatch scaffolding in a second entry point.
- **Alternatives considered**: A second top-level entry point (e.g. `RunAgents`) called
  directly from `main.go` instead of extending `Run` — rejected; it would duplicate the
  shared usage-error handling and give `main.go` two independent dispatch paths to keep
  in sync, which is more surface area, not less.
