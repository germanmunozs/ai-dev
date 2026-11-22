# Contract: Vendor Adapter and System Boundary Interfaces

**Satisfies**: architecture goals 1, 3, 6, 10; FR-020, FR-021, SC-008

These are internal Go contracts, not user-facing surfaces — but they are the mechanism
that makes "add a coding agent without changing the generic model" (goal 10) an
enforceable property rather than a hope. A future adapter implements exactly the
interfaces below; nothing else in `internal/execution` or `internal/agent` changes.

## `system.Launcher` — the process-supervision boundary

```go
package system

type Spec struct {
    Name   string    // executable name/path
    Args   []string  // full argument vector, vendor-constructed
    Dir    string     // working directory (the current project — FR-010)
    Stdin  io.Reader  // caller's validated stdin reader, forwarded to the launched process
    Stdout io.Writer  // destination the caller has already chosen (human or machine mode)
    Stderr io.Writer
}

type ExitStatus struct {
    Exited   bool // true only if the process ran to completion and returned an exit code
    ExitCode int  // meaningful only when Exited is true
    Canceled bool // true when cancellation reached the process while it was active
}

type Handle interface {
    // Wait blocks until the process ends (including cancellation-driven termination)
    // and reports how it ended.
    Wait() (ExitStatus, error)
}

type Launcher interface {
    // Launch starts the process. ctx cancellation must be forwarded to the running
    // process and must guarantee it has stopped before a subsequent Wait() returns.
    Launch(ctx context.Context, spec Spec) (Handle, error)
}
```

- `OSLauncher` (production implementation): `os/exec.CommandContext` +
  `Cmd.Cancel`/`Cmd.WaitDelay` (see `research.md` §1–2).
- This interface knows nothing about agents, instructions, or outcomes — it is the same
  shape whether the process is `claude`, `codex`, or a test fixture script. It sits
  beside the existing `system.Executor` (unchanged) rather than extending it (see
  `research.md` §9).

## `execution.Adapter` — the vendor boundary

```go
package execution

// Adapter owns everything vendor-specific: how to build the command line for one
// instruction, and how to read that vendor's own exit-code convention.
type Adapter interface {
    // Command builds the full invocation for a single instruction against workdir.
    // It MUST NOT include any flag that elevates permissions, bypasses approval, or
    // disables sandboxing (FR-015, FR-016) — this is a review property of each
    // implementation, not something the interface can enforce mechanically.
    Command(workdir, instruction string) (name string, args []string)

    // Classify interprets a *normal* exit (ExitStatus.Exited == true) as this vendor's
    // own reported success or failure. It is never called for abnormal termination —
    // the Runner classifies that as execution_failed unconditionally (research.md §2).
    Classify(exitCode int) Outcome // must return AgentSucceeded or AgentFailed only
}
```

- `claude.Adapter` and `codex.Adapter` are the two initial implementations
  (`internal/execution/adapters`), each a few lines: build the vendor's one-shot
  invocation, map exit code `0` to `AgentSucceeded` and anything else to
  `AgentFailed` (research.md §4–5).
- **Extensibility property (SC-008)**: adding agent N+1 means adding one `Agent` value
  to `agent.Registry`'s literal list and one new `Adapter` implementation, registered in
  the same fixed `map[agent.ID]Adapter` the `Runner` already holds. No change to
  `execution.Outcome`'s three values, `execution.Runner`, `system.Launcher`, or any
  already-supported adapter is required or permitted by this contract.

## `execution.Runner` — the generic lifecycle

```go
type Runner struct {
    Registry agent.Registry
    Launcher system.Launcher
    Adapters map[agent.ID]Adapter
    Stdin    io.Reader // caller stdin passed through after instruction-source validation
}

func (r Runner) Run(ctx context.Context, req Request, workdir string, stdout, stderr io.Writer) Report
```

Lifecycle (goal 3), entirely inside this one method, with no vendor-specific branching
except the two `Adapter` calls:

1. **Resolve agent**: `Registry.Resolve(req.AgentID)` → unsupported → `execution_failed`.
2. **Check availability**: `Registry` availability for the resolved agent → unavailable
   → `execution_failed`.
3. **Build command**: `Adapters[id].Command(workdir, req.Instruction)`.
4. **Launch**: `Launcher.Launch(ctx, spec)` → error → `execution_failed`.
5. **Stream / interact**: implicit — `spec.Stdin` is the caller's validated input
   stream and `spec.Stdout`/`spec.Stderr` are the caller's chosen destinations; the OS
   passes them through while the process runs.
6. **Supervise / handle cancellation**: `handle.Wait()`; `ctx` cancellation is the
   caller's (the CLI signal handler's) responsibility to trigger — the `Runner` itself
   just calls `Wait()` and interprets whatever `ExitStatus` comes back.
7. **Classify outcome**: `ExitStatus.Canceled == true`, `ExitStatus.Exited == false`,
   or a launch/communication error from `Wait` → `execution_failed`; otherwise
   `Adapters[id].Classify(ExitStatus.ExitCode)`. Cancellation observed only after a
   completed `Wait` result does not replace that natural process result.
8. **Report**: returns a `Report` (see `data-model.md`) — the caller (`internal/app`)
   renders it in human or machine form and sets the process exit code (FR-019).

Routing between human-readable and machine-readable mode (FR-011) is **not** the
`Runner`'s concern: the caller decides, before calling `Run`, which writer(s) to pass as
`stdout`/`stderr` in `Spec` (real `os.Stdout`/`os.Stderr` for human mode; the same
`stderr` writer for both in machine mode, per the contract in `agent-run.md`). This
keeps the `Runner` — the generic execution semantics goal 10 protects — free of any
"if machine mode" branching.
