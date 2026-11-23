# Quickstart: Agent Catalog Expansion

## Prerequisites

- Go as declared in `go.mod`; macOS or Linux.
- `go build -o "$PWD/ai-dev" ./cmd/ai-dev`, then work in a temporary directory.
- Fake agents: for each executable in [contracts/catalog.md](contracts/catalog.md), a script on a private `PATH` directory that prints one line per argument and exits with a code taken from `FAKE_EXIT` (default 0). No real agent is needed or run.

## Checks

| Step | Expected |
|---|---|
| `PATH=<fakes>:/usr/bin:/bin ./ai-dev agents` | the 8 agents in catalog order, all `available` |
| Same with only `gemini` and `vibe` fakes | `gemini-cli` and `mistral-vibe` available, the rest unavailable |
| `./ai-dev agents --format json` twice | identical output |
| `./ai-dev agent run gemini-cli "-x a b"` | the fake prints exactly `--prompt=-x a b`; outcome `agent_succeeded`, exit 0 |
| `./ai-dev agent run opencode --help` (the instruction is `--help`, 010 FR-001) | the fake prints `run`, `--`, `--help`; never opencode's help |
| `FAKE_EXIT=3 ./ai-dev agent run kiro-cli x` | outcome `agent_failed`, exit 1 |
| `./ai-dev agent run qwen-code x` with no `qwen` on `PATH` | `execution_failed`, unavailable |
| `./ai-dev agent run claude-code x` and `codex-cli x` | argument vectors unchanged: `--print -- x`, `exec -- x` |
| `./ai-dev doctor` with no agent on `PATH` | `coding-agent-tool` fails, points to `ai-dev agents`; eight checks |
| `./ai-dev doctor` with only the `vibe` fake | `coding-agent-tool` passes: `Available: mistral-vibe` |
| `./ai-dev setup apply --agent gemini-cli --agent factory-droid --role specification=gemini-cli --role planning=gemini-cli --role implementation=factory-droid --role review=gemini-cli --role remediation=factory-droid`, then `./ai-dev agent select implementation` | `configuration_written`; `agent=factory-droid source=project` |
| `./ai-dev agent select implementation` without a configuration | `agent=codex-cli source=built_in` (unchanged) |
| `./ai-dev agent run --help`, `ai-dev completion zsh` | list the 8 identifiers in catalog order |

Automated equivalents: `go test ./internal/agent ./internal/execution/... ./internal/checks ./internal/app ./cmd/ai-dev`.
