# Contract: Completion

## Command (F010-FR-006)

| Arguments | Result |
|---|---|
| `completion bash` | the bash script on stdout; exit 0 |
| `completion zsh` | the zsh script on stdout; exit 0 |
| `completion`, `completion fish`, `completion bash extra` | `Usage: ai-dev completion bash|zsh` on stderr; exit 2; stdout empty |

## What is completed (F010-FR-007)

| Typed | Offered |
|---|---|
| `ai-dev <partial>` | top-level words: `doctor agents agent workflow init git version help completion` |
| `ai-dev <group> <partial>` | that group's next words (for `init`, its subcommands and its options) |
| `ai-dev <command> -<partial>` | that command's options, plus `--help` |
| `ai-dev <command> --format <partial>` | `text json` |
| `ai-dev init plan --strategy <partial>` | `basic git-flow github-flow trunk-based` |
| `ai-dev agent run <partial>` | `claude-code codex-cli` |
| `ai-dev agent select <partial>` | the five roles |
| `ai-dev completion <partial>` | `bash zsh` |
| `ai-dev help <partial>` | the next catalog words |
| a file or directory option's value | file names (`compgen -f` in bash, `_files` in zsh) |
| anything else, such as `ai-dev workflow state <partial>` | nothing, and no file-name fallback |

## Static script (F010-FR-008)

- The script is generated from the catalog only, with no environment value.
- It runs no program while completing: only shell builtins (`compgen`, `compadd`) and the shell's own file completion. File-name completion lists names and never reads file contents.
- It is byte-identical across runs for the same build.
- Every embedded word matches `^[A-Za-z0-9._|-]+$`.
- **bash:** it runs in bash 3.2 or later, and is registered with `complete -o filenames -F _ai_dev ai-dev`, without `-o default`.
- **zsh:** it runs in zsh 5 or later, as a `#compdef ai-dev` function. When evaluated or sourced, it registers itself with `compdef _ai_dev ai-dev`. When autoloaded from `fpath` (under any name), it completes on the first Tab.
- **bash `--option=value`:** bash 4 and later split it at `=` (`COMP_WORDBREAKS`); the script joins the words back, so positionals are counted correctly, and a value typed after `=` completes as that option's value.
- **Known limitation (bash):** `-o filenames` makes bash append `/` to a candidate that also names a directory in the working directory, for example `workflow/` when a `workflow` directory exists. bash 3.2 has no `compopt` to apply it to file values only, and dropping it would leave file names with spaces unescaped.

## Installation (documented in the README)

```sh
# bash (~/.bashrc); eval also works in bash 3.2, whose source <(...) reads nothing
eval "$(ai-dev completion bash)"
# zsh (~/.zshrc, after compinit), or save as _ai-dev in an fpath directory
eval "$(ai-dev completion zsh)"
```

ai-dev never writes shell startup files.
