# Implementation Plan: Terminal Cockpit

**Branch**: `feature/012-terminal-cockpit` | **Date**: 2026-11-21 | **Spec**: [spec.md](spec.md)

## Summary

`ai-dev tui` opens a keyboard-first cockpit. Its read-only areas show **the same text reports the commands print**, sanitized for the terminal: `doctor` for Environment, `setup show` for Agents, `init inspect` for Repository, and the 004/005/007 reports for each feature. They are produced in process by the functions the commands already use, so a view cannot diverge from its command (SC-002). Actions hand the terminal to the existing `ai-dev` command, launched through the existing `system.Launcher` boundary, after an explicit review; the cockpit then waits for a key and refreshes.

The cockpit itself (`internal/cockpit`) is presentation only: a Bubble Tea model over plain data. It imports no domain package, writes no file, starts no process of its own, and holds no authority. `internal/app` composes the data and the command runner, as it composes every other command.

## Technical Context

- **Language and dependencies:** Go 1.27.4. **New, pinned:** `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2`, the project's first third-party modules (research R-01). Versions follow the 14-day rule of R-01; today that is bubbletea v2.1.1 (2026-10-28) and lipgloss v2.0.6 (2026-08-11).
- **Storage:** none. The cockpit keeps nothing between runs (F012-FR-017).
- **Testing:** `go test`:
  - `internal/cockpit`: model update and view tests with fake data sources and a fake runner (navigation, palette, forms, availability, double confirmation, size notice, every key of the keymap for SC-006), sanitizer tests (SC-005), and a benchmark of update and view with 50 features (SC-001);
  - `internal/app`: snapshot composition equals the commands' own reports for the same fixtures (SC-002), and each action's argument vector (SC-003);
  - pseudo-terminal tests of the built binary: open, navigate, run a fake command, return, quit by `q`, `Ctrl-C` and `SIGTERM`, and terminal state restored (SC-004); hostile names never reach the terminal raw (SC-005);
  - boundary test: `internal/cockpit` imports neither `os/exec`, `net`, `syscall`, `internal/system` nor any domain or persistence package, and calls no write-capable function.
- **Target platforms:** macOS and Linux terminals. Elsewhere `ai-dev tui` refuses (no terminal detection), exit 2.
- **Project type:** CLI.
- **Constraints:** no new behavior in any existing command; actions only through the commands; untrusted text always sanitized; terminal always restored.
- **Scale:** up to about 50 features; six areas; six actions.

## Constitution Check

| Principle | Status |
|---|---|
| I. Specification First | PASS: spec approved by the owner on 2026-11-21, Amendments A1 and A2 applied (`8bfa59c`) |
| II. Architecture Before Framework | PASS: the framework is confined to `internal/cockpit`; domain and command code is untouched, and the cockpit consumes plain data. The library choice follows from the spec's terminal requirements (resize, raw input, alternate screen, restoration, Unicode width), not the reverse |
| III. Simplicity and YAGNI | PASS with justified complexity: two pinned dependencies (22 modules in total) instead of a hand-written terminal layer; benefit, cost, trade-offs and the simpler alternative are in R-01. No Bubbles widgets: the single-line input the forms need is about 60 lines. Views reuse the existing text renderers instead of new layouts |
| IV. Testable by Design | PASS: model logic is pure and tested without a terminal; pseudo-terminal tests cover what only a terminal can show |
| V. Security by Design | PASS: threats in R-05 and R-06 (terminal injection, action spoofing, double submission, signal handling, supply chain). Dependencies pinned with checksums, 14-day rule, `govulncheck` in CI, licenses checked (R-01) |
| VI. Observability by Design | PASS: every view shows its evaluation time; every action shows its exact command and its exit code |
| VII. Reproducibility | PASS: `go.sum` pins every module; CI installs nothing unpinned |
| VIII. AI-Assisted Engineering | PASS: independent review before the PR |
| IX. Human Authority | PASS: decisions are recorded only by the commands, after a review and an explicit confirmation of that one action; the cockpit prefills no human decision |

Post-design re-check: PASS. The only change to existing behavior is the catalog, help and top-level usage gaining `tui` (A1, A2).

## Design

- **`internal/cockpit`** (new, depends on Bubble Tea, Lip Gloss and the standard library only):
  - `model.go`: the `tea.Model` with areas, selection, filter, palette, form, review and "returned" states (data-model.md);
  - `keys.go`: the default keymap of F012-FR-013;
  - `actions.go`: the six actions as data, each with its argument template and its form fields (contracts/cockpit.md);
  - `sanitize.go`: `Sanitize(string) string` and the `Text` type it alone produces; views accept only `Text` for untrusted strings (R-05);
  - `view.go`: layout, the size notice below 80×24, and the help;
  - `input.go`: a single-line text input;
  - ports: `Source` (`Load(ctx) Snapshot`, one call per refresh, each area loaded concurrently) and `Runner` (`Command(args []string) tea.ExecCommand`).
- **`internal/app/cockpit.go`** (new): `RunCockpit(ctx, args, deps, stdin, stdout, stderr)`:
  - refuses unless stdin and stdout are terminals (as `setup`), exit 2;
  - builds the `Source` from existing functions: `RunDoctor` + `report.WriteText`; `loadAssignment` + the setup report; `inspectProject` + `report.WriteGitInspectionText`; feature listing (`specs/*` filtered by `workflow.FeatureExists`) with `workflow.EvaluateState`, `sdd.Evaluate`, `guide.Evaluate` and `report.WriteGuideText`;
  - builds the `Runner` over `system.Launcher` with `os.Executable()`, inheriting the terminal (R-03);
  - runs the program without Bubble Tea's own signal handler and forwards `SIGINT`/`SIGTERM` itself (R-04).
- **`internal/app/app.go`, `catalog.go`, `help.go`**: route `tui`; catalog entry; top-level usage gains `tui` (A1, A2).
- **`.github/workflows/ci.yml`**: a pinned `govulncheck` step in `static-analysis`.

## Project Structure

```text
specs/012-terminal-cockpit/
  plan.md, research.md, data-model.md, quickstart.md, contracts/cockpit.md
internal/cockpit/{model,keys,actions,sanitize,view,input}.go, *_test.go, boundary_test.go   NEW
internal/app/cockpit.go, cockpit_test.go, cockpit_pty_test.go                            NEW
internal/app/{app,catalog,help}.go                 MODIFIED (route, catalog, usage)
go.mod, go.sum                                     MODIFIED (two pinned modules)
.github/workflows/ci.yml                           MODIFIED (govulncheck)
tests pinning top-level usage, general help, catalog, completion   UPDATED (A1, A2)
README.md, CHANGELOG.md, SECURITY.md               UPDATED (cockpit; dependency policy)
```

## Quality Gates

| Gate | How | Pass condition |
|---|---|---|
| Format | `gofmt -l .` | no output |
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test -race ./...` | exit 0 |
| Static analysis | `staticcheck ./...` for linux and darwin (CI `static-analysis`) | exit 0 |
| Vulnerabilities | `govulncheck ./...` (CI `static-analysis`) | no reachable vulnerability |
| Dependencies | `go mod verify`; versions satisfy R-01's 14-day rule; licenses permissive (R-01) | pass, recorded in tasks.md |
| Boundary | cockpit boundary test, existing boundary tests | pass |
| Performance | benchmark of SC-001 | under 50 ms per key with 50 features |
| Build | `go build ./cmd/ai-dev` for darwin, linux, windows | exit 0 |
