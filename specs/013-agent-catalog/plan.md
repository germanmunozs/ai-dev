# Implementation Plan: Agent Catalog Expansion

**Branch**: `feature/013-agent-catalog` | **Date**: 2026-11-22 | **Spec**: [spec.md](spec.md)

## Summary

Six coding agents join Claude Code and Codex CLI: Factory Droid, Gemini CLI, Kiro CLI, Mistral Vibe, OpenCode and Qwen Code. They are the candidates, out of 44 evaluated besides the two existing agents, that meet every admission criterion with evidence. The admission record is research R-05.

The catalog becomes data:
- `internal/agent` holds identity (identifier, label, executable) in a fixed order;
- one data-driven adapter in `internal/execution/adapters` turns each agent's verified invocation into an argument vector, replacing the two vendor types while reproducing their vectors exactly;
- doctor's two agent tool checks become one aggregate `coding-agent-tool` check (Amendment A2);
- help, completion, setup and the cockpit already derive from `agent.Catalog()` and need no code change.

## Technical Context

- **Language and dependencies:** Go 1.27.4. No new dependency.
- **Storage:** none. The project configuration schema is unchanged.
- **Testing:** `go test`.
  - `internal/agent`: order, identifier syntax, uniqueness, and `claude-code`/`codex-cli` unchanged.
  - `internal/execution/adapters`: table-driven tests over the whole catalog for drift, instruction safety (SC-003) and the denylist of relaxing options (SC-004), plus exact vectors for the two existing agents.
  - `internal/checks`: the aggregate check passes, fails and recovers from a panic.
  - `internal/app` and `cmd/ai-dev`: `agent run` against fake executables for every agent (SC-002); doctor's eight checks; `agents` order; completion and help listing.
- **Target platforms:** macOS and Linux, as before.
- **Project type:** CLI.
- **Constraints:**
  - no real agent is run, by tests or by research (SC-005);
  - no new input can name an executable or argument (F013-FR-013);
  - 002's behavior is unchanged for every agent (F013-FR-008).
- **Scale:** 8 agents now; adding one is two table lines.

No item needs clarification: research R-01 to R-08 resolve the evidence policy, the interpretation of criterion 3, the parser checks, the collisions, the admission record, the design, the guard tests and the doctor text.

## Constitution Check

| Principle | Status |
|---|---|
| I. Specification First | PASS: spec approved by the owner on 2026-11-22, Amendments A1 and A2 applied (`dde42cb`) |
| II. Architecture Before Framework | PASS: domain packages keep their boundaries. Identity stays in `agent`, invocation in `execution/adapters` (R-06) |
| III. Simplicity and YAGNI | PASS: one generic adapter replaces two near-identical types; no plugin system, no runtime catalog file, no new dependency |
| IV. Testable by Design | PASS: the catalog is pure data guarded by table-driven tests; fakes stand in for every agent |
| V. Security by Design | PASS: threats addressed: instruction injected as an option (R-03, SC-003); permission relaxation (R-02, SC-004); executable spoofing by name collision (R-04, criterion 6); configuration naming executables (F013-FR-013, unchanged closed set) |
| VI. Observability by Design | PASS: doctor names the available agents; `agents` lists all with availability |
| VII. Reproducibility | PASS: the catalog is fixed at build time; the versions verified are recorded (R-05) |
| VIII. AI-Assisted Engineering | PASS: independent review before the PR |
| IX. Human Authority | PASS: agents are still run only on an explicit command; no agent is granted authority it does not have when typed by hand (R-02) |

Post-design re-check: PASS. The only changes to existing behavior are those approved: doctor's checks (A2), and the agent lists that follow the catalog.

## Design

- **`internal/agent/registry.go`:**
  - the catalog gains six entries, ordered per F013-FR-005;
  - constants for the new identifiers;
  - an `Executable()` accessor on `Agent`.

  `Catalog`, `Known`, `Resolve` and `List` are unchanged.
- **`internal/execution/adapters`:**
  - `invocation.go` (new): `Invocation`, `Form` (`AfterEndOfOptions`, `AttachedOption`), `Command` and `Classify`;
  - `catalog.go` (new): `Invocations map[agent.ID]Invocation` with the rows of contracts/catalog.md;
  - `claude.go` and `codex.go` are removed. Their tests become rows of the table-driven tests, with the vectors asserted literally.
- **`internal/app/agentrun.go`:** `Adapters` is built from `adapters.Invocations`.
- **`internal/checks/codingagent.go` (new):** the aggregate check of R-08, over `agent.NewRegistry(executor).List()`.
- **`internal/app/doctor.go`:** eight checks, with `coding-agent-tool` third. The comment changes from "nine" to "eight".
- **Documentation:**
  - README and README.es: supported agents, the doctor table (eight checks) and the admission criteria pointer;
  - CHANGELOG and CHANGELOG.es: `[Unreleased]`, Added (six agents) and Changed (doctor check; agent lists).

## Project Structure

### Documentation (this feature)

```text
specs/013-agent-catalog/
├── spec.md
├── plan.md
├── research.md        # R-05 is the admission record
├── data-model.md
├── quickstart.md
├── contracts/
│   └── catalog.md
├── checklists/
│   └── requirements.md
└── tasks.md           # /speckit-tasks
```

### Source Code

```text
internal/agent/registry.go              # catalog entries, order, Executable()
internal/agent/registry_test.go         # order, syntax, existing IDs unchanged
internal/execution/adapters/invocation.go   # new generic adapter
internal/execution/adapters/catalog.go      # new invocation table
internal/execution/adapters/*_test.go       # drift, safety, denylist, exact vectors
internal/execution/adapters/claude.go, codex.go   # removed
internal/checks/codingagent.go(+_test)  # new aggregate check
internal/app/agentrun.go                # adapters from the table
internal/app/doctor.go(+_test)          # eight checks
internal/app/agents_test.go, cmd/ai-dev/main_test.go   # catalog-length expectations
README.md, README.es.md, CHANGELOG.md, CHANGELOG.es.md
```

**Structure Decision:** existing packages only; two new files in `adapters`, one in `checks`.

## Complexity Tracking

No violations to justify.
