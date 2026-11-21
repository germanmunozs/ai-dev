# Data Model: Command Help and Shell Completion

Everything here is fixed at build time and held in memory; nothing is persisted.

## Command Spec

| Field | Meaning | Rules |
|---|---|---|
| `path` | Words typed after `ai-dev`, for example `init plan` | Unique; every prefix that is not itself a command is a group |
| `group` | True for a group path | Groups: `agent`, `workflow`, `init`, `git`, `git commit`. `init` is both a group and a command |
| `description` | One line | The same text in the general help and in the command's help |
| `usage` | The usage line | Byte-identical to the line the command writes on a usage failure (SC-002); empty for pure groups |
| `args` | Ordered positional arguments | Each has a name, optional fixed values and a description |
| `options` | Options | Equal to the options the command's flag set defines (F010-FR-009) |
| `changes` | What the command can change | `nothing`, `workflow-state.json`, `the repository and its project configuration`, `the plan file only`, or `whatever the agent does, under its own sandbox` |

## Option Spec

| Field | Meaning |
|---|---|
| `name` | Without dashes, as the flag set defines it |
| `value` | Placeholder, empty for a boolean option |
| `values` | Fixed values, for example `text`, `json` |
| `file` | True when the value is a file or directory, so completion offers file names |
| `repeatable` | True for `--include`, `--approve-file` and `--approve-ignored-file` |
| `description` | One line |

## Fixed Values

- `--format`: `text`, `json`.
- `init plan --strategy`: `basic`, `git-flow`, `github-flow`, `trunk-based`.
- `agent run <agent-id>`: `claude-code`, `codex-cli`.
- `agent select <role>`: the five roles.
- `completion <shell>`: `bash`, `zsh`.
- `help <command>`: the catalog paths.

## Relationships

- The general help lists every non-group `path`, plus `completion`.
- A group's help lists the commands whose `path` extends it by one word. `git` lists `git commit`, and `git commit` lists `git commit validate`.
- Completion offers the next words of every path that extends the typed prefix, then the options of the reached command, then the fixed values of the option or argument being completed.
