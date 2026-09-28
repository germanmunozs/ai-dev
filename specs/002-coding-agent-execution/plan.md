# Implementation Plan: Coding Agent Execution

**Branch**: `feature/002-coding-agent` | **Date**: 2026-09-27 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-coding-agent-execution/spec.md`

## Summary

Add a vendor-neutral capability to list the coding agents `ai-dev` supports with their
local availability (`ai-dev agents`), and to explicitly run one named agent against a
single instruction with live-streamed output and one final, unambiguous outcome
(`ai-dev agent run <agent-id> [instruction]`). The generic execution lifecycle
(validate → resolve → launch → stream → supervise → cancel → wait → classify → report)
lives in a new vendor-neutral `internal/execution` package; everything vendor-specific
(command construction, exit-code interpretation) lives behind a small `Adapter`
interface with one implementation each for Claude Code (`claude-code`) and Codex CLI
(`codex-cli`). Process supervision (live stdio, cancellation, abnormal-termination
detection) is a new `system.Launcher` interface built on `os/exec` primitives already in
the standard library (`Cmd.Cancel`, `Cmd.WaitDelay`), added beside the existing
`system.Executor` without changing it. The existing `doctor` command, its nine-check
contract, and every file under `internal/checks`/`internal/doctor`/`internal/report`
(doctor's JSON/text writers) are untouched; only `internal/app`'s top-level dispatcher
gains two new recognized subcommand shapes and two new constructor parameters
(a `system.Launcher` and stdin), threaded through from `cmd/ai-dev/main.go`.

## Technical Context

**Language/Version**: Go 1.27.1 (matches existing `go.mod`; no change)

**Primary Dependencies**: Go standard library only (`os/exec`, `context`, `flag`,
`encoding/json`, `os/signal`, `io`) — no new module dependency (architecture goal 9)

**Storage**: N/A — no persistence; each invocation is independent (spec Assumptions)

**Testing**: `go test` with the standard library `testing` package, following the
existing repo convention: per-package unit tests with fakes
(`internal/agent`, `internal/instruction`, `internal/execution`), a real-subprocess test
for the process-supervision boundary (`internal/system`, mirroring the existing
`cmd/ai-dev/main_test.go` fake-`PATH` shell-script pattern), and `internal/app`
CLI-contract tests mirroring the existing `TestRunFormatAndUsageContract` style

**Target Platform**: Same as existing `ai-dev` — POSIX-oriented CLI (darwin/linux);
Windows-specific shell-script subprocess tests are skipped the same way the existing
`cmd/ai-dev/main_test.go` skips them (`runtime.GOOS == "windows"`)

**Project Type**: Single Go module CLI (matches existing `cmd/`, `internal/` layout —
no new top-level project structure)

**Performance Goals**: Not a performance-sensitive feature; the only latency-relevant
requirement is "live" streaming, defined behaviorally in FR-011/SC-009 (a flushed marker
observable before the fixture process is released), not by a numeric threshold

**Constraints**: Must not elevate agent permissions, bypass approval/sandbox behavior,
or introduce credentials/secrets into `ai-dev`'s own output (FR-015–FR-017); must not
change the observable `doctor` command or its nine checks (goal 11); must not add a
dynamic plugin system, CLI framework, or workflow orchestration (goals 1, 9, 10)

**Scale/Scope**: Two initial vendor adapters (`claude-code`, `codex-cli`); one execution
request maps to at most one process invocation (FR-024); no concurrency/coordination
requirements between separate `ai-dev` invocations (spec Edge Cases)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design below.*

| Principle | Assessment |
|---|---|
| I. Specification First | Spec is approved, clarified, and 16/16 on its quality checklist; this plan does not alter its behavioral contract. PASS |
| II. Architecture Before Framework | Architecture (vendor-neutral core + adapters + a narrow process-supervision boundary) follows directly from the spec's own vendor-neutral requirements (FR-004, FR-020–FR-022) and goal 1, not from a preferred tool. PASS |
| III. Simplicity and YAGNI | No plugin system, no CLI framework, no dynamic routing; two adapters implemented as two small structs; one new interface (`system.Launcher`) justified by a genuine capability gap in the existing `system.Executor` (research.md §9), not speculative extensibility. PASS |
| IV. Testable by Design | Every FR has a corresponding test seam (contracts' "Test seams" sections); the plan requires real-subprocess coverage where fakes cannot prove the behavior (live streaming, SC-009). PASS |
| V. Security by Design | FR-015–FR-017 are enforced by construction (Adapter contract explicitly forbids permission-elevating flags; Runner never touches agent output content when building its own report) and are review-checked per adapter. PASS |
| VI. Observability by Design | Not a backend/distributed system; the feature's own "observability" requirement is the live-streaming contract itself, addressed above. N/A beyond that. |
| VII. Reproducibility and Automation | Quality gates below are all scripted, version-controlled commands; no manual step affects correctness. PASS |
| VIII. AI-Assisted Engineering | This plan is itself a version-controlled artifact produced ahead of implementation, per the standard Spec Kit flow. PASS |
| IX. Human Authority | Plan does not assert any human approval; it is a proposal awaiting the normal review before `tasks`/implementation proceed. PASS |

No violations requiring a Complexity Tracking entry.

## Project Structure

### Documentation (this feature)

```text
specs/002-coding-agent-execution/
├── plan.md              # This file ($speckit-plan command output)
├── research.md          # Phase 0 output ($speckit-plan command)
├── data-model.md         # Phase 1 output ($speckit-plan command)
├── quickstart.md         # Phase 1 output ($speckit-plan command)
├── contracts/             # Phase 1 output ($speckit-plan command)
│   ├── agents-list.md
│   ├── agent-run.md
│   └── adapter-interface.md
└── tasks.md              # Phase 2 output ($speckit-tasks command - NOT created by $speckit-plan)
```

### Source Code (repository root)

```text
cmd/ai-dev/
├── main.go                  # MODIFIED: wire system.OSLauncher{} + os.Stdin into app.Run
└── main_test.go              # MODIFIED (mechanical): new app.Run params; unmodified assertions

internal/app/
├── doctor.go                  # MODIFIED: RunDoctor untouched; top-level Run gains new dispatch branches + params
├── doctor_test.go              # MODIFIED (mechanical): new app.Run params; unmodified assertions
├── agents.go                   # NEW: RunAgentsList — assembles agent.Registry, chooses text/json rendering
├── agents_test.go               # NEW
├── agentrun.go                  # NEW: RunAgentRun — instruction resolution + execution.Runner + reporting
└── agentrun_test.go              # NEW

internal/agent/
├── agent.go                     # NEW: Agent, ID, Availability, AgentStatus
├── registry.go                   # NEW: Registry (List, Resolve), fixed two-entry agent list
├── agent_test.go                  # NEW
└── registry_test.go                # NEW

internal/instruction/
├── instruction.go                 # NEW: Resolve(arg, hasArg, stdin, stdinIsInteractive) (string, error)
└── instruction_test.go             # NEW

internal/execution/
├── outcome.go                      # NEW: Outcome type + three constants, Report struct
├── request.go                       # NEW: Request struct
├── adapter.go                        # NEW: Adapter interface
├── runner.go                          # NEW: Runner (lifecycle orchestration)
├── runner_test.go                      # NEW: fake system.Launcher-based lifecycle tests
└── adapters/
    ├── claude.go                        # NEW: claude-code Adapter (Command, Classify)
    ├── claude_test.go                    # NEW
    ├── codex.go                           # NEW: codex-cli Adapter (Command, Classify)
    └── codex_test.go                       # NEW

internal/system/
├── executor.go                     # UNCHANGED
├── executor_test.go                  # UNCHANGED
├── launcher.go                        # NEW: Launcher, Spec, Handle, ExitStatus interfaces
├── oslauncher.go                       # NEW: OSLauncher (os/exec + Cmd.Cancel/WaitDelay)
└── oslauncher_test.go                    # NEW: real-subprocess tests incl. SC-009 live-marker test

internal/report/
├── json.go                          # UNCHANGED (doctor's report only)
├── text.go                           # UNCHANGED (doctor's report only)
├── agentlist.go                       # NEW: text + JSON rendering for Agent Listing
├── agentlist_test.go                   # NEW
├── executionreport.go                   # NEW: text (final-outcome line) + JSON rendering for Execution Outcome
└── executionreport_test.go               # NEW

internal/checks/                       # UNCHANGED
internal/doctor/                        # UNCHANGED
```

**Structure Decision**: Single Go module, extending the existing flat `cmd/` +
`internal/<domain>` layout from feature 001 — no new top-level directory, no
web/mobile/multi-project split (none of those apply to a CLI). Two new domain packages
(`internal/agent`, `internal/execution` with an `adapters` sub-package) mirror the
existing `internal/doctor` + `internal/checks` split: one package for the generic domain
model/lifecycle, one for the pluggable-but-fixed implementations. `internal/instruction`
is its own package because the spec explicitly requires validating it independently of
vendor execution (goal 4); `internal/system` gains a sibling interface
(`Launcher`) beside the untouched `Executor`, per research.md §9.

## Complexity Tracking

*No entries — Constitution Check recorded no violations requiring justification.*
