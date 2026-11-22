# Implementation Plan: Environment Doctor

**Branch**: `001-environment-doctor` | **Date**: 2026-09-27 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-environment-doctor/spec.md`

## Summary

`ai-dev doctor` is a single Go CLI command that runs nine independent, read-only
readiness checks (four tool-availability checks — Git, Spec Kit, Claude Code, Codex CLI —
and five repository checks — Git repository, Spec Kit initialization, project
constitution, Claude integration, Codex integration), aggregates them into one overall
Ready/Not Ready result, and reports the outcome both as legible terminal text and as a
`--format json` machine-readable report with stable per-check identifiers. The process
exit code alone (0 = Ready, 1 = Not Ready) makes the overall result usable by automation
without any output parsing. The design keeps a technology-agnostic readiness domain model
(`internal/doctor`) fully separate from the concrete, tool-specific checks
(`internal/checks`) and from the narrow interfaces that touch the outside world
(`internal/system`), so every check's Pass/Fail logic is unit-testable against fakes
without any real tool installed on the test machine. Full technical rationale is in
`research.md`.

## Technical Context

**Language/Version**: Go, pinned via `go.mod`'s `go` directive to the installed toolchain
baseline (`go1.27.1 darwin/arm64` in the reference dev environment).

**Primary Dependencies**: Standard library only — `flag`, `os/exec`, `io/fs`,
`encoding/json`, `fmt`, `context`, `testing`, `testing/fstest`. No
third-party module is added by this feature (see `research.md` §2–§4 for why no CLI/TUI
framework is introduced).

**Storage**: N/A — no database, cache, or persisted application state. The command only
reads existing repository files (`.specify/**`, integration skill files); it writes
nothing.

**Testing**: `go test ./...` using the standard library `testing` package, table-driven
unit tests per check, an in-memory fake implementing the `system.Executor` interface, and
`testing/fstest.MapFS` for filesystem-backed checks — no real Git/Spec Kit/Claude/Codex
installation is required for the test suite to pass (research.md §6).

**Target Platform**: macOS, Apple Silicon (`darwin/arm64`), the only officially supported
target for this feature. `internal/doctor` and `internal/checks`'s decision logic avoid
OS-specific APIs so a future additional target is a build/test exercise rather than a
redesign, but no other OS is built, tested, or claimed as supported here.

**Project Type**: Single CLI project (`cmd/ai-dev` entry point + `internal/...` packages).

**Performance Goals**: Interactive-speed only; no numeric SLA is meaningful for a one-shot
local command dominated by a handful of `LookPath`/filesystem-stat operations. Informally,
a full run should complete well under a couple of seconds on a typical development
machine.

**Constraints**: Read-only with respect to the repository and environment (no writes, no
mutation of `.specify` state); no network calls; no daemon or background process; no
persistent state between invocations; deterministic output given unchanged inputs
(FR-018); authentication status of Claude Code/Codex is explicitly out of scope
(spec Assumptions).

**Scale/Scope**: One command, nine fixed checks, one invocation per run. Not a concern for
throughput, concurrency, or data volume.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Specification First | PASS | `spec.md` (with resolved clarifications) precedes this plan; this plan precedes any implementation. |
| II. Architecture Before Framework | PASS | No CLI framework or TUI framework is introduced. The standard library's `flag` package and plain text output fully satisfy the single-subcommand, single-optional-flag surface this feature needs; `research.md` §2–§3 records the justification and the alternatives rejected for lack of a demonstrated gap. |
| III. Simplicity and YAGNI | PASS | No plugin system, DI framework, database, daemon, or persistent state is introduced. The one generalized mechanism in this design (`ManifestIntegrationCheck`, shared by the Spec Kit init check and both integration checks) is justified by FR-002's explicit, stated requirement that future coding-agent integrations be addable as independent checks — not by speculative extensibility (`research.md` §7–§8). No Complexity Tracking entry is needed because nothing here is a *violation* requiring justification beyond what is already recorded in research.md. |
| IV. Testable by Design | PASS | Every check's Pass/Fail decision is exercised through a narrow interface (`system.Executor` for process lookup and invocation, `io/fs.FS` for filesystem reads) with fakes and `fstest.MapFS`, so `go test ./...` covers every check's Pass path and every documented Fail path without requiring Git/Spec Kit/Claude/Codex to be installed on the test runner. |
| V. Security by Design | PASS | No secrets are read, stored, or logged. All external process interaction is limited to `LookPath` and fixed, non-user-controlled arguments (no shell interpolation of untrusted input); the command has no write access to the filesystem and makes no network calls. |
| VI. Observability by Design | N/A | This is a one-shot local CLI, not a backend or distributed system; the constitution scopes this principle to backend/distributed-system specs and plans. |
| VII. Reproducibility and Automation | PASS | Build/test/lint/format are the standard `go` toolchain commands, version-pinned via `go.mod`, and documented verbatim in `quickstart.md` and the Quality Gates section below — runnable without any undocumented local state. |
| VIII. AI-Assisted Engineering | PASS | This plan is produced from the version-controlled spec via the standard Specify → Plan → Tasks → Implement flow; the repository's constitution already requires both Claude and Codex integrations to remain intact (Assumptions in spec.md), consistent with this principle's cross-agent-review intent. |
| IX. Human Authority | PASS | The technology direction and architecture expectations in this plan were explicitly supplied and approved by the human sponsor before this plan was written; no approval is fabricated on the human's behalf. |

No violations requiring a Complexity Tracking entry.

## Project Structure

### Documentation (this feature)

```text
specs/001-environment-doctor/
├── plan.md              # This file ($speckit-plan command output)
├── research.md          # Phase 0 output ($speckit-plan command)
├── data-model.md         # Phase 1 output ($speckit-plan command)
├── quickstart.md         # Phase 1 output ($speckit-plan command)
├── contracts/            # Phase 1 output ($speckit-plan command)
│   ├── cli-contract.md
│   └── doctor-report.schema.json
└── tasks.md              # Phase 2 output ($speckit-tasks command - NOT created by $speckit-plan)
```

### Source Code (repository root)

```text
go.mod                        # module root; pins the Go toolchain version

cmd/
└── ai-dev/
    └── main.go                # entry point: wires system.Executor + real filesystem,
                                 # builds the nine Check values, calls internal/app, sets
                                 # os.Exit(code)

internal/
├── app/
│   ├── doctor.go               # orchestration: flag parsing (--format), assembling the
│   │                           #   nine Check instances with their tool-specific names/
│   │                           #   paths, calling doctor.RunAll, choosing the renderer,
│   │                           #   mapping Report -> exit code
│   └── doctor_test.go
├── doctor/
│   ├── check.go                # Check interface, CheckResult, Status, Report, RunAll
│   │                           #   (the technology-agnostic readiness domain model)
│   └── check_test.go
├── checks/
│   ├── tool.go                  # ToolCheck: generic LookPath-based check
│   ├── gitrepo.go                # GitRepoCheck: `git rev-parse --is-inside-work-tree`
│   │                             #   via system.Executor
│   ├── manifest.go               # ValidateManifestFiles: shared function for manifest-driven file
│   │                             #   existence validation (used by speckit.go and
│   │                             #   integration.go)
│   ├── speckit.go                 # Spec Kit initialization check, built on manifest.go
│   ├── constitution.go            # ConstitutionCheck: structural completeness heuristic
│   ├── integration.go             # ManifestIntegrationCheck: manifest.go + installation-
│   │                             #   record membership, reused for Claude and Codex
│   │                             #   (and any future coding agent)
│   └── *_test.go                  # one test file per check, using fakes/fstest.MapFS
├── system/
│   ├── executor.go                # Executor interface + real os/exec-backed implementation
│   └── executor_test.go           # (fakes for use by internal/checks tests live in an
│                                   #   internal testutil file here or alongside each check)
└── report/
    ├── text.go                    # human-readable renderer (FR-007)
    ├── json.go                    # --format json renderer, matches contracts/doctor-report.schema.json
    └── *_test.go
```

**Structure Decision**: Single Go module, single CLI binary (`cmd/ai-dev`). Four
`internal` packages give the four required separations: `doctor` (pure domain model,
technology-agnostic per the approved direction), `checks` (concrete, tool-aware
inspections built from a small number of reusable mechanisms), `system` (the only place
that touches `os/exec`; filesystem access instead uses the standard `io/fs.FS` interface
directly rather than a bespoke wrapper), and `app` (orchestration: turns flags into a
report and an exit code, and is the only package that knows there are exactly nine checks
today). `internal/` is used throughout because nothing here is a reusable library intended
for import by other modules — it is exclusively the implementation of one CLI command.

## Complexity Tracking

*No entries — no Constitution Check violation requires justification beyond what is
already recorded in `research.md` §7–§8 (the one generalized mechanism, justified by
FR-002 itself, not by speculative extensibility).*
