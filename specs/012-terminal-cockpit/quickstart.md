# Quickstart: Terminal Cockpit

## Prerequisites

- macOS or Linux, Go as declared in `go.mod`.
- `go build -o "$PWD/ai-dev" ./cmd/ai-dev`.
- For agent runs, fake `claude` and `codex` scripts first on `PATH` that print their arguments and exit 0.

## Checks

| Step | Expected |
|---|---|
| `ai-dev tui </dev/null` | `interactive_terminal_required`, exit 2 |
| `ai-dev tui x` | `Usage: ai-dev tui`, exit 2 |
| `ai-dev tui` in this repository | Dashboard; `Tab` cycles Features, Environment, Agents, Repository, Help |
| Features | one row per `specs/NNN-*` with state, next step, agent and source; `Enter` shows the same text as `ai-dev workflow state` and `ai-dev workflow guide` |
| Environment / Agents / Repository | the same text as `ai-dev doctor`, `ai-dev setup show`, `ai-dev init inspect` |
| `:` then `appr` | approve actions listed, with availability |
| Run agent on a feature | form, review with the exact command, `Enter` on confirm: the fake agent prints its arguments; exit code shown; `Enter` returns and the view refreshes |
| Approve with `Esc` at the review | nothing runs |
| Resize below 80×24 | size notice; growing restores the view |
| `q`, `Ctrl-C`, `kill -TERM` | terminal restored (`stty -a` unchanged) |
| A feature directory named with `\x1b]52;…` | shown as visible `\x1b]52;…`; the terminal clipboard is untouched |

Automated equivalents: `go test ./internal/cockpit ./internal/app -run Cockpit`.
