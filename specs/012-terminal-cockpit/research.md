# Research: Terminal Cockpit

## R-01 Terminal library and dependency policy

- **Decision (owner, 2026-11-21):** Bubble Tea and Lip Gloss, major version 2 (`charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`).
- **Why v2:** it is the maintained line. Measured on 2026-11-21 with `go list -m all` in a probe module: v2 pulls 22 modules, 3 of them pseudo-versions (`charmbracelet/ultraviolet`, `charmbracelet/x/exp/golden`, `golang.org/x/exp`); v1.3.10 with lipgloss v1.1.0 pulls 25 modules, 7 of them pseudo-versions.
- **14-day rule:** pin the newest release that is at least 14 days old on the day the dependency is added, and record its date. On 2026-11-21: bubbletea v2.1.2 (2026-11-18) is too new, v2.1.1 (2026-10-28) qualifies; lipgloss v2.1.1 (2026-11-15) and v2.1.0 (2026-11-09) are too new, v2.0.6 (2026-08-11) qualifies (v2.1.0 qualifies from 2026-11-23). Newer releases are taken later by an explicit dependency update, never implicitly.
- **Integrity:** `go.sum` pins every module; `go mod verify` runs in the gates; CI keeps `GOTOOLCHAIN=local` and installs tools by exact version.
- **Vulnerabilities:** `govulncheck` in the `static-analysis` job, pinned by the same rule (v1.9.0, 2026-10-11, today). A reachable vulnerability fails the job.
- **Licenses:** the Charm modules are MIT; the transitive modules must be MIT, BSD or Apache-2.0. Checked from each module's LICENSE file in the module cache when the dependency is added, and recorded in tasks.md.
- **Benefit:** raw input with key decoding, resize events, alternate screen, synchronized rendering, Unicode width, colour downsampling and terminal restoration on exit and panic, all maintained upstream.
- **Cost and trade-offs:** the project's first supply chain (22 modules); the build graph grows; upgrades become a recurring task.
- **Simpler alternative considered:** a hand-written layer over termios (`terminal_posix.go` already reads terminal attributes). Rejected by the owner: it would mean writing and testing key decoding, resize, width tables and redraw, which is more code and more terminal bugs than the dependencies it avoids.
- **Not adopted:** Bubbles (widgets). The forms need one single-line input; writing it keeps a third module family out.

## R-02 Views reuse the commands' own reports

- **Decision:** each read-only area shows the text report its command prints, produced in process by the same functions and renderers: Environment = `RunDoctor` + `report.WriteText`; Agents = the setup report of `setup show`; Repository = `inspectProject` + `report.WriteGitInspectionText`; a feature's detail = `report.WriteWorkflowStateText` and `report.WriteGuideText`. The Features list is a table of fields taken from `workflow.EvaluateState`, `sdd.Evaluate` and `guide.Evaluate`.
- **Rationale:** SC-002 holds by construction, and no second layout of the same facts can drift from its command.
- **Feature listing:** directories under `specs/` for which `workflow.FeatureExists` holds (no symbolic link), in name order. Names are untrusted and are sanitized.
- **Alternatives considered:** new structured layouts per area (rejected: duplication, drift); running the commands and capturing their output (rejected: needs process launching for reads and gives nothing more).

## R-03 Actions hand the terminal to the command

- **Decision:** `Runner.Command(args)` returns a `tea.ExecCommand` implemented in `internal/app` over `system.Launcher`: `Spec{Name: os.Executable(), Args: args, Stdin/Stdout/Stderr: the terminal}`. Bubble Tea releases the terminal, the command runs exactly as if typed, and when it exits the adapter prints `ai-dev tui: the command exited with code N. Press Enter to return.` and reads one line before Bubble Tea takes the terminal back (F012-FR-010).
- **Argument vectors:** values are passed as separate arguments in `--name=value` form, and positional values after `--`, so no value can become an option (contracts/cockpit.md). There is no shell anywhere.
- **Why `system.Launcher`:** it is the existing process boundary; `os/exec` stays confined to `system` and `gitprocess`, and the cockpit package never sees a process API.
- **Context:** the command is launched with a context that is never cancelled by the cockpit. Only the terminal's own interrupt reaches it, so it keeps its documented interrupt behavior (R-04).

## R-04 Signals

- **Decision:** the program runs with Bubble Tea's signal handler disabled. `RunCockpit` registers `SIGINT` and `SIGTERM` itself and forwards them as messages:
  - while the cockpit owns the terminal, either signal quits it, restoring the terminal (in raw mode `Ctrl-C` arrives as a key and quits too);
  - while a command runs, `SIGINT` is ignored by the cockpit, since the terminal delivered it to the command in the foreground, and `SIGTERM` is remembered and honored as soon as the command returns.
- **Context:** `cmd/ai-dev/main.go` wraps the context in `signal.NotifyContext(..., os.Interrupt)`, so a `SIGINT` meant for the command in the foreground also cancels the context `RunCockpit` receives. `RunCockpit` therefore runs the program, the loaders and the launches on `context.WithoutCancel(ctx)`; its own signal forwarding is the only way an external signal reaches the cockpit.
- **Exit codes:** `q` or `Ctrl-C` as a key: 0. An external `SIGINT` while the cockpit owns the terminal: 130, as `ai-dev init`. `SIGTERM`: 143. The terminal is restored first in every case.
- **Rationale:** without this, `main`'s interrupt context would also cancel the launched command and preempt its own interrupt handling, and the cockpit could not resume after the command.
- **Process group:** a `SIGINT` typed while a command runs reaches the whole foreground process group: the cockpit (which ignores it), the `ai-dev` child and anything it started (which handle it as they do outside the cockpit).

## R-05 Terminal injection

- **Decision:** `Sanitize` maps every rune that is a C0 or C1 control, DEL, or a bidirectional control (U+061C, U+200E, U+200F, U+202A–U+202E, U+2066–U+2069) to a visible escape (`\x1b`, `\u202e`), and replaces invalid UTF-8 with U+FFFD. A tab is expanded to spaces up to the next multiple of 8 columns, as a terminal would, so the tab-separated columns of the reports stay aligned. Reports are split into lines before sanitizing, so no newline reaches it. Its output is the `Text` type; views take `Text` for every untrusted string, so a forgotten call does not compile.
- **Coverage:** feature names, every line of every reused report, values of the agents configuration (already quoted by the report, sanitized again), action command previews, and any error message.
- **Verification:** unit tests per class of sequence (CSI, OSC including OSC 52 and OSC 8 hyperlinks, DCS, bidirectional overrides, C1 controls); a pseudo-terminal test with a hostile feature name asserts the raw terminal output never contains the sequence.

## R-06 Double submission and spoofed actions

- **Decision:** a review shows the exact argument vector; only `Enter` on the review's confirm button runs it, once. The model enters a `running` state before returning the exec command, and leaves it only on the exec callback; confirmations in any other state do nothing. After returning, the review is closed, so a repeated `Enter` cannot reach it.
- **SC-006 test:** for every area and state, every key of the keymap and every pair of keys is sent to the model; the fake runner must record no command unless the sequence passed through the review's confirm.

## R-07 Performance

- **Decision:** loading happens in `tea.Cmd`s, one per area, concurrently; update and view are pure and cheap. A benchmark with 50 synthetic features asserts update plus view under 50 ms per key. Environment and Repository, which start Git and version probes, show "evaluating…" until done and never block input.

## R-08 Testing the terminal

- **Model:** unit tests call `Update` and `View` directly (no `teatest`, which would add a module).
- **Terminal:** the existing `openPTY` helper drives the built binary as a session leader. Terminal restoration is checked by reading the terminal attributes of the slave before opening and after exit, and by checking the alternate screen was left (`\x1b[?1049l` seen) and the cursor shown.
