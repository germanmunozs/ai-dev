# Quickstart: Agent Detection and Role Configuration

## Prerequisites

- Go as declared in `go.mod`; macOS or Linux.
- `go build -o "$PWD/ai-dev" ./cmd/ai-dev`, then work in an empty temporary directory.
- To check that no agent is started, put fake `claude` and `codex` scripts first on `PATH` that append to a log file and exit 1. The log must stay empty in every step.

## Read-only view

| Command | Expected |
|---|---|
| `ai-dev setup show` | `outcome=not_configured source=built_in`, both agents, the built-in roles; exit 0 |
| `ai-dev setup show --format json` | one `ai-dev.setup/v1` object; exit 0 |
| `ai-dev agent select implementation` | `… agent=codex-cli source=built_in` (as today plus `source`) |

## Non-interactive configuration

```sh
ai-dev setup apply --agent claude-code --agent codex-cli \
  --role specification=claude-code --role planning=claude-code \
  --role implementation=claude-code --role review=claude-code --role remediation=codex-cli
# outcome=configuration_written; .ai-dev/config.json in canonical form (contracts/config-file.md)
ai-dev agent select implementation      # agent=claude-code source=project
# same apply again                       # outcome=configuration_unchanged
# apply with a different role set        # outcome=configuration_exists, exit 1; with --replace: written
ai-dev setup apply --agent claude-code --role specification=codex-cli ...   # configuration_rejected, exit 1
```

## Invalid configuration

```sh
printf '{"schema_version":"ai-dev.config/v1","agents":["gemini"],"roles":{}}' > .ai-dev/config.json
ai-dev setup show                        # configuration_invalid, reasons unknown_agent "gemini", missing_role …; exit 1
ai-dev agent select review               # outcome=configuration_invalid source=project; exit 1
ln -sf /etc/hosts .ai-dev/config.json; ai-dev setup show   # file_not_regular
```

## Assistant

- `ai-dev setup </dev/null` → `interactive_terminal_required`, exit 2, nothing written.
- In a terminal, `ai-dev setup`: choose agents, assign roles, answer `no` → `setup_cancelled`, the directory is unchanged; answer `yes` → `configuration_written`.
- `TestSetupWizardInPTY` drives the same flow in a pseudo-terminal.

## Help and completion

`ai-dev setup --help`, `ai-dev help setup apply`; after `eval "$(ai-dev completion zsh)"`, Tab after `ai-dev setup apply --role ` offers the ten `ROLE=AGENT` pairs.
