# Quickstart: Command Help and Shell Completion

## Prerequisites

- Go as declared in `go.mod`; bash and zsh for the completion checks.
- `go build -o ai-dev ./cmd/ai-dev`.

## Help

| Command | Expected |
|---|---|
| `./ai-dev workflow state --help` | the command's help on stdout; exit 0 |
| `./ai-dev help init plan` | `init plan` help, listing `--strategy` with its four values; exit 0 |
| `./ai-dev workflow -h` | the six workflow commands; exit 0 |
| `./ai-dev init --help </dev/null` | assistant help and the four subcommands; exit 0; no terminal needed |
| `echo x \| ./ai-dev agent run --help` | help; stdin is not read; exit 0 |
| `./ai-dev help nope` | `Usage: ai-dev help [command...]` on stderr; exit 2 |
| `./ai-dev workflow state f --help` | unchanged: the command's own behavior with feature id `f` and an extra argument (usage failure, exit 2) |

Usage-line check: for each command, the usage line in its help equals the first stderr line of `./ai-dev <command> --no-such-option`. See contracts/help.md.

## Completion

```sh
./ai-dev completion bash > /tmp/ai-dev.bash
bash --norc --noprofile -c 'source /tmp/ai-dev.bash; COMP_WORDS=(ai-dev init plan --strategy ""); COMP_CWORD=4; _ai_dev; echo "${COMPREPLY[@]}"'
# basic git-flow github-flow trunk-based
/bin/bash --norc --noprofile -c '…same…'    # macOS bash 3.2
./ai-dev completion zsh | zsh -n            # syntax check
./ai-dev completion fish; echo $?           # 2
```

Interactive check: `eval "$(./ai-dev completion zsh)"` (or `bash`), then press Tab after `ai-dev wo`. `TestZshCompletionInRealShell` drives the same check in a pseudo-terminal with the real `compinit` and `_files`.
