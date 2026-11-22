# Implementation Plan: Workflow Orchestration — Guided Mode

**Branch**: `feature/007-workflow-orchestration` | **Date**: 2026-11-19 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/007-workflow-orchestration/spec.md` (revision 3, merged to `dev` in PR #1).

**Authorization**: The owner authorized G-IMPLEMENT for the guided slice on 2026-11-19 ("si, autorizo G-IMPLEMENT, ejecuta /speckit-plan para el modo guiado"). This plan covers F007-FR-001 to F007-FR-015 and SC-001 to SC-007 only. The specification's deferred autonomous section is not normative and is not planned.

## Summary

Add `ai-dev workflow guide <feature-id>`, a read-only report for one feature. The report holds:
- 005's next-step outcome, state, activity and role;
- the agent that role maps to under 003, with its 002 availability;
- command templates for the next human actions, with every human-supplied value left as an unquoted `)slot(`, whose unbalanced `)` is a syntax error in every POSIX shell mode measured, so that a pasted, unfilled template executes nothing.

The new domain package composes the existing `sdd` and `selection` domain functions in process. It starts no process, writes nothing, reads no standard input, and changes no existing behavior.

## Technical Context

**Language/Version**: Go 1.27.1, as `go.mod` declares.

**Primary Dependencies**: Standard library only. Internal packages `sdd`, `selection`, `agent`, `role` and `workflow` are reused, and no new third-party module is added.

**Storage**: None. Nothing is persisted.

**Testing**: `go test`. The unit tests use `fstest.MapFS` fixtures and a recording fake executor. Shell-paste tests run the rendered templates through `sh -c`, `bash -c` and `zsh -c` against a sentinel `ai-dev` on `PATH`; zsh is skipped when it is absent. A subprocess end-to-end test runs the built binary.

**Target Platform**: macOS and Linux, including WSL2. Guidance reads only files and `PATH`, so it also builds and runs elsewhere. Templates assume a POSIX-like shell (R-03).

**Project Type**: Single-binary CLI.

**Performance Goals**: Not applicable beyond 005's own cost. A request does a bounded number of `LookPath` calls (two agents) plus 005's reads.

**Constraints**:
- No process start, no write, no standard-input read and no network access (F007-FR-007).
- Identical outcomes and exit codes to 005 (F007-FR-002, F007-FR-011).
- Unchanged usage texts (F007-FR-013).

**Scale/Scope**: One request reports on one feature.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Status |
|---|---|---|
| I. Specification First | The specification is merged in PR #1, G-SPEC (guided) passed with the owner's confirmation, and G-IMPLEMENT is authorized. There are no open ambiguities in scope. | PASS |
| II. Architecture Before Framework | No framework is used. The design follows from the specification's by-reference rule and the 006 precedent (R-01). | PASS |
| III. Simplicity and YAGNI | One small domain package, one renderer and one command. There is no store, no abstraction layer and no autonomous code. Rejected alternatives are recorded in research.md. | PASS |
| IV. Testable by Design | Every SC is mapped to automated tests (Test Strategy), including shell-paste tests for SC-004. | PASS |
| V. Security by Design | The threats are copy-paste command injection, false Human Gate evidence, derived identities and side effects. They are mitigated by R-03, R-04, F007-FR-005 and the boundary test (R-07). No secrets are involved. | PASS |
| VI. Observability by Design | A local CLI with no backend. Diagnostics go to standard error as in 005; no telemetry. | N/A (justified) |
| VII. Reproducibility and Automation | Every gate runs with `go` commands and in the existing CI workflow. | PASS |
| VIII. AI-Assisted Engineering | The implementing and reviewing agents will be recorded. An independent review is planned before merge. | PASS (planned) |
| IX. Human Authority | Guidance never executes, approves or derives a decision field, and the merge stays the owner's decision. | PASS |

**Post-design re-check (after Phase 1):** unchanged. PASS. The design adds no persistence, no process and no authority. The only rendering choice with security weight (R-03) was measured empirically and is enforced by tests.

## Quality Gates

| Gate | How it is run or evidenced | Pass condition |
|---|---|---|
| Format | `gofmt -l .` | No output |
| Vet | `go vet ./...`; `GOOS=linux go vet ./...` | Exit 0 |
| Build | The six-platform cross-build of the CI workflow | Exit 0 |
| Unit and integration tests | `go test -count=1 ./...` | Exit 0; every existing suite unchanged |
| Race | `go test -race -count=3 ./internal/guide ./internal/report ./internal/app ./cmd/ai-dev` | Exit 0, no data race |
| Structural boundary | `internal/guide` boundary test (imports and calls, R-07) | Passes |
| Shell paste (SC-004) | Shell-paste tests in sh and bash, and zsh when present | Every hostile fixture passes |
| CI | The existing `.github/workflows/ci.yml` on the PR, macOS and Ubuntu | All jobs green |
| Independent review | A separate reviewing agent on the implementation diff, recorded in the PR | No blocking or high finding left open |
| Security | Covered by the boundary test, SC-004 and a check that no template or field contains a derived decision value | Passes |

## Project Structure

### Documentation (this feature)

```text
specs/007-workflow-orchestration/
├── spec.md              # merged specification (revision 3)
├── plan.md              # this file
├── research.md          # Phase 0 decisions R-01 to R-08
├── data-model.md        # Guidance Report, Command Template, template-safe identifier
├── quickstart.md        # validation scenarios
├── contracts/
│   └── workflow-guide.md   # command, outputs, exit codes, invariants
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output ($speckit-tasks; not created by $speckit-plan)
```

### Source Code (repository root)

```text
internal/guide/                 # NEW domain: composes sdd and selection; builds templates
├── guide.go                    # Evaluate(root, featureID, selector) Report; Valid
├── templates.go                # Command Template model (words, slots), F007-FR-005 table
├── safety.go                   # template-safe identifier (Cc, Cf, U+2028, U+2029)
├── guide_test.go               # equivalence with sdd for all outcomes; availability mapping
├── templates_test.go           # template sets by outcome and availability
└── boundary_test.go            # allowed and forbidden imports and calls (R-07)

internal/report/
├── guide.go                    # NEW: text and JSON rendering, shell quoting (R-03, R-04)
└── guide_test.go               # rendering; shell-paste tests (SC-004)

internal/app/
├── workflowguide.go            # NEW: RunWorkflowGuide; flags; exit codes; output failure
├── workflowguide_test.go       # usage failures, exit codes, no stdin, no side effects
└── app.go                      # MODIFIED: route `workflow guide`; usage texts unchanged

cmd/ai-dev/
└── workflowguide_test.go       # NEW: subprocess end-to-end; US2 non-existent subcommands
```

**Structure Decision**: The single Go project layout already in use. The domain lives in `internal/<feature>`, rendering in `internal/report`, and command wiring in `internal/app`, as 005 does (`sdd`, `report/workflownext.go`, `app/workflownext.go`).

**Dependency rule**:
- `internal/guide` may import only `io/fs`, `strings`, `unicode` and the internal packages `sdd`, `selection`, `agent`, `role` and `workflow`. Its boundary test enforces this, and also forbids calling any constructor that holds a launcher.
- `internal/app` builds the registry from the injected `system.Executor`, the same one `agents` and `agent select` use, so guidance can reach `LookPath` and nothing else.

## Test Strategy

| Criterion | Tests |
|---|---|
| SC-001 | `guide_test.go`: for MapFS fixtures in all seven states plus unknown, invalid and missing-artifact cases, `guide.Evaluate(...).Outcome` and its fields equal `sdd.Evaluate(...)`. `app` checks that exit codes match `workflow next`. |
| SC-002 | `workflowguide_test.go`: the fixture tree digest is unchanged; a fake executor records zero `Run` calls; standard input is not read; the boundary test shows no `os/exec`, `net` or launcher use. |
| SC-003 | `guide_test.go`: available, unavailable and inconsistent cases (a test registry without the mapped agent); no substitution. |
| SC-004 | `report/guide_test.go`: hostile identifiers `a;b`, `$(x)`, `-f`, `it's`, `a b`, U+202E and U+200B. Filled templates reach a sentinel with exactly one identifier argument. Each human slot left unfilled alone runs nothing and changes no file, in sh, bash and zsh. No derived slot value appears. |
| SC-005 | `report/guide_test.go`: the statement is present in every outcome, in text and JSON. |
| SC-006 | The existing suites; a cmd test that `workflow run` and `orchestrate` stay usage failures with unchanged text. |
| SC-007 | `report/guide_test.go`: the JSON is decoded and every field is checked per outcome. |

## Complexity Tracking

No constitution violations to justify.
