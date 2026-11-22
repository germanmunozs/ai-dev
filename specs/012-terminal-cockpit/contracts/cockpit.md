# Contract: `ai-dev tui`

## Command

| Case | Behavior | Exit |
|---|---|---|
| stdin and stdout are terminals, no argument | open the cockpit | 0 on `q` or `Ctrl-C`; 130 on an external `SIGINT`; 143 on `SIGTERM` (terminal restored first) |
| not a terminal | stderr `ai-dev: tui: refused: interactive_terminal_required; use the ai-dev commands directly` | 2 |
| any argument | stderr `Usage: ai-dev tui` | 2 |
| the terminal cannot be set up | stderr `ai-dev: tui: <reason>`, terminal restored | 2 |

Catalog: `tui`, description "Open the interactive terminal cockpit (needs a terminal)", changes "nothing by itself; actions run their own commands". Top-level usage: `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|version|help|completion> [options]`.

## Keys (F012-FR-013)

| Key | Effect |
|---|---|
| `↑` `↓` `j` `k` | move |
| `Enter` | open the selected item or choose the selected action |
| `Tab` `Shift-Tab` | next or previous area |
| `/` | filter the current list (`Esc` clears) |
| `:` `Ctrl-P` | palette |
| `?` | help |
| `r` | refresh |
| `Esc` | back or close |
| `q`, `Ctrl-C` | quit (from the top level; `Ctrl-C` from anywhere except a running command) |

Inside a review, the confirm button is focused only by `Tab` and pressed by `Enter`; no other key confirms.

## Actions and argument vectors (F012-FR-008, FR-009)

Values are the person's text, unmodified, each one argument. `<feature>` is the selected feature's directory name.

| Action | Fields | Argument vector after `ai-dev` | Available when |
|---|---|---|---|
| Run the agent of the next step | instruction (required) | `agent run -- <agent> <instruction>` | `step_recommended`, availability `available` |
| Approve | approver (required), target state (required), reason (optional) | `workflow approve --approver=<approver> [--reason=<reason>] -- <feature> <target>` | state recognized and not terminal |
| Abandon | approver (required), reason (required) | `workflow abandon --approver=<approver> --reason=<reason> -- <feature> abandoned` | state recognized and not terminal |
| Configure agents | — | `setup` | always |
| Initialize repository | — | `init` | always |
| Validate a commit message | message (required) | `git commit validate --message=<message>` | always |

The review shows the argument vector with each argument quoted by `report.ShellQuote` (the POSIX single-quote form the guide report already uses), produced by `Runner.Preview` in `internal/app` and sanitized like any other text, and the line "This runs the command above in this terminal. Nothing happens until you confirm."

After the command: `ai-dev tui: the command exited with code N. Press Enter to return.`

## Layout

- Header: `ai-dev tui`, the current area, the evaluation time.
- Body: the area's list and detail.
- Footer: the keys of the current mode.
- Below 80×24: only `Terminal too small: <c>×<r>, need 80×24.`
