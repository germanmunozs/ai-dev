# Implementation Plan: Agent Detection and Role Configuration

**Branch**: `feature/011-agent-setup` | **Date**: 2026-11-21 | **Spec**: [spec.md](spec.md)

## Summary

Add `ai-dev setup` (interactive assistant), `ai-dev setup show` (read-only) and `ai-dev setup apply` (non-interactive). They detect the catalog agents through the existing 002 registry, and read or write a per-project `.ai-dev/config.json` holding the chosen agents and one agent per role.

Three new pieces carry the feature:

- **`internal/projectconfig`** reads and validates the file. It is pure: it reads through `fs.FS`, follows no symbolic link, and returns a validated `Config`, an absence, or the list of reasons it is invalid (R-02).
- **`internal/configstore`** is the only code that writes the file. It replaces the file atomically, the way `internal/gatestore` replaces `workflow-state.json` (R-03).
- **`selection.Assignment`** is the role assignment in effect. It is either built-in, built from a validated `Config`, or "the project configuration is invalid". Role selection and guidance resolve through it, so a valid configuration replaces the built-in mapping and an invalid one fails closed (R-01).

## Technical Context

- **Language and dependencies:** Go 1.27.4, standard library only.
- **Storage:** one JSON file, `.ai-dev/config.json`, in the current directory. Written only by `internal/configstore`, on macOS and Linux local filesystems.
- **Testing:** `go test`:
  - `projectconfig`: a fixture per validation rule of F011-FR-005, plus canonical encoding round trips;
  - `configstore`: atomic replacement, symbolic links refused, fault injection at each step (as `gatestore` does), unsupported platform;
  - a boundary test: only `configstore` writes the configuration, and only `cmd/ai-dev/main.go` imports `configstore`;
  - `selection` and `guide`: every outcome under each assignment source;
  - `internal/app`: `setup show` and `setup apply` for every outcome and exit code, and the assistant driven through a pseudo-terminal;
  - a subprocess test against the built binary, which also checks that no agent process starts (fake agents on `PATH` that record any invocation).
- **Target platforms:** macOS and Linux for writing. Reading and `setup show` work everywhere; writing elsewhere reports `persistence_failed`.
- **Project type:** CLI.
- **Constraints:**
  - no agent is ever started, and no network is used (F011-FR-013);
  - without a configuration, every 003 and 007 outcome stays as it is today, plus the `source` field (F011-FR-015, F011-FR-017);
  - an invalid configuration never falls back to the built-in mapping (F011-FR-016);
  - the configuration can only contain catalog identifiers and role names (F011-FR-006).
- **Scale:** 2 catalog agents, 5 roles, 3 new command paths and 1 new group.

## Constitution Check

| Principle | Status |
|---|---|
| I. Specification First | PASS: spec approved by the owner on 2026-11-21, with Amendments A1–A4 applied (`a1586d7`) |
| II. Architecture Before Framework | PASS: no framework. Reading, writing and selection are separated by responsibility, reusing the 002 registry, the 003 selector and the 006 write pattern |
| III. Simplicity and YAGNI | PASS: one file, one schema, no lock (the spec accepts last-writer-wins, R-03), no user-level configuration, no migration. `configstore` duplicates about 100 lines of the `gatestore` replace sequence instead of sharing a generalized writer, because `gatestore` is pinned as the sole `workflow-state.json` write path by its boundary tests (R-03) |
| IV. Testable by Design | PASS: every requirement maps to a test (tasks.md). The assistant is tested in a real pseudo-terminal |
| V. Security by Design | PASS: the configuration is untrusted input from a cloned repository. It is size-limited, strictly decoded, closed-set validated, and never names an executable (F011-FR-006). Symbolic links are refused on read and write. Untrusted strings in reasons are rendered in 004's shell-inert quoted form |
| VI. Observability by Design | PASS: every outcome is a stable token in text and JSON; invalid configurations list each reason |
| VII. Reproducibility | PASS: the file is byte-identical for the same configuration (SC-006), and a test enforces it |
| VIII. AI-Assisted Engineering | PASS: independent review before the PR |
| IX. Human Authority | PASS: only a person running `setup` changes the configuration, after an explicit confirmation or explicit options. Changes are reviewed through the project's own pull requests |

Post-design re-check: PASS. The changes to 003 and 007 code are limited to what Amendments A1 and A2 approved: the assignment source, the `configuration_invalid` outcome and availability, and the `source` field.

## Design

- **`internal/projectconfig`** (new):
  - `const FileName = ".ai-dev/config.json"`, `SchemaVersion = "ai-dev.config/v1"`, `MaxSize = 64 << 10`;
  - `type Config` with unexported fields (chosen agents, role assignment); only `Parse` and `New` build one, and both validate;
  - `Load(root fs.FS) (Config, State, []Reason)` with `State` = `Absent | Valid | Invalid` (R-02);
  - `New(agents []agent.ID, roles map[role.Role]agent.ID) (Config, []Reason)` for `setup apply` and the assistant;
  - `(Config) Encode() []byte`, the canonical bytes of R-04; `Agents()`, `AgentFor(role.Role)`, `Chosen(agent.ID)`.
- **`internal/configstore`** (new): `New(dir string) Store`, `Store.Write(data []byte) (Status, []string)`; POSIX implementation with injectable `ops` for fault tests, and an unsupported-platform implementation that always fails (R-03).
- **`internal/agent`**: `Known(id ID) bool` and `Catalog() []Agent`, read-only views of the existing fixed set, so `projectconfig` can validate identifiers without probing availability.
- **`internal/selection`** (A1):
  - `Source` (`built_in`, `project`), `Assignment` with `BuiltIn()`, `FromProject(projectconfig.Config)` and `InvalidProject()`;
  - `NewSelector(registry)` keeps the built-in assignment; `NewSelectorFor(registry, Assignment)` is new;
  - outcome `ConfigurationInvalid`; `Report.Source`;
  - `Valid()` checks a `built_in` report against `mappedAgent` exactly as today, and a `project` report for field presence and catalog membership.
- **`internal/report`**:
  - role selection text gains ` source=<source>`, JSON gains `"source"`; `configuration_invalid` renders `role` and `source` only (contracts/selection-and-guide.md);
  - guide text gains ` source=<source>` after `availability=`, JSON gains `"source"`;
  - new `setup.go`: text and JSON for `setup show` and `setup apply`, schema `ai-dev.setup/v1` (contracts/setup-cli.md).
- **`internal/guide`** (A2): availability `ConfigurationInvalid` with no agent; `Report.Source`; the `configuration_invalid` templates are the transition and approval templates only, as for `inconsistent`.
- **`internal/app`**:
  - `setup.go`: `RunSetupShow`, `RunSetupApply`, with `--agent` and `--role` as repeatable flags;
  - `setup_wizard.go`: `RunSetupWizard`, requiring a terminal on stdin and stdout, cancelling on interrupt (R-05);
  - `agentselect.go` and `workflowguide.go` load the configuration through `projectconfig.Load(root)` and build the selector with the matching `Assignment`;
  - `app.go`: the `setup` route, `Dependencies.ConfigStore`, and `topLevelUsage` gains `setup` (A3);
  - `catalog.go`: `setup`, `setup show`, `setup apply`; `--role` completes the ten `ROLE=AGENT` pairs (R-07);
  - `help.go`: the general help lists the new commands (A4).
- **`cmd/ai-dev/main.go`**: wires `configstore.New(".")`.

## Project Structure

```text
specs/011-agent-setup/
  plan.md, research.md, data-model.md, quickstart.md
  contracts/setup-cli.md, contracts/config-file.md, contracts/selection-and-guide.md
internal/projectconfig/{config.go,load.go,encode.go,quote.go}, *_test.go   NEW (quote.go: the 004 shell-inert quoting, shared with internal/report)
internal/configstore/{store.go,write_posix.go,unsupported.go,new_other.go}, *_test.go, boundary_test.go   NEW
internal/report/quote.go                           MODIFIED (uses the shared quoting, output unchanged)
internal/app/setupwrite.go, setupwrite_test.go     NEW (shared write decision for apply and the assistant)
internal/agent/registry.go                         MODIFIED (Known, Catalog)
internal/selection/{assignment.go,outcome.go,selector.go,validate.go}   MODIFIED/NEW (A1)
internal/guide/{guide.go,templates.go}             MODIFIED (A2)
internal/report/{roleselection.go,guide.go,setup.go}   MODIFIED/NEW
internal/app/{setup.go,setup_wizard.go}            NEW
internal/app/{app.go,agentselect.go,workflowguide.go,catalog.go,help.go}   MODIFIED
cmd/ai-dev/main.go                                 MODIFIED (ConfigStore)
cmd/ai-dev/setup_test.go                           NEW (subprocess, no agent started)
tests pinning 003/007 reports, top-level usage, general help, catalog   UPDATED (A1–A4)
README.md, CHANGELOG.md                            UPDATED
```

## Quality Gates

| Gate | How | Pass condition |
|---|---|---|
| Format | `gofmt -l .` | no output |
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test -race ./...` | exit 0 |
| Static analysis | `staticcheck ./...` (CI job `static-analysis`) | exit 0 |
| Boundary | `configstore` boundary test, `gatestore` and `gitmanager` boundary tests | pass |
| Git minimum | CI job `git-minimum` | pass |
| Security | review of the threat list in research R-02/R-03 during the independent review | no open finding |
| Build | `go build ./cmd/ai-dev` on macOS and Linux (CI `gates`) | exit 0 |
