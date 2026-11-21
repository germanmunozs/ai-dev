# Contract: Help

## Recognition (F010-FR-001, FR-003)

Let `P` be the longest catalog path that prefixes the arguments, and `n` its length.

| Arguments | Result |
|---|---|
| `P --help` or `P -h`, with anything after it | `P`'s help; stdout; exit 0 |
| `help P`, `--help P` or `-h P` | `P`'s help; stdout; exit 0 |
| `help`, `--help`, `-h` (alone) | the general help (009, plus `completion`) |
| `help X` where `X` is not a catalog path, or `help P extra` | `Usage: ai-dev help [command...]` on stderr; exit 2; stdout empty |
| `--help` or `-h` anywhere else (after a positional, as an option value, after `--`) | unchanged behavior of the command |

Examples of unchanged behavior:
- `ai-dev agent run claude-code --help`: the instruction is `--help`.
- `ai-dev workflow approve --approver --help f planned`: the approver is `--help`.
- `ai-dev workflow state -- --help`: the feature id is `--help`.

## Command help layout

```text
ai-dev <path>: <description>

<usage line>

Arguments:
  <name>   <description> [values: a|b]
Options:
  --<name> <value>   <description> [values: a|b] [repeatable]
  -h, --help         Show this help
Changes: <changes>
Exit codes: 0 positive result, 1 negative domain result, 2 usage or internal failure.
```

- `Arguments:` and `Options:` are omitted when empty, except that `-h, --help` is always listed.
- `version` and `help` have no domain result; their exit line reads `0 success, 2 usage failure.`

## Group help layout

```text
ai-dev <path>: <group description>

Commands:
  <path> <word>   <description>
  ...

Run 'ai-dev help <command>' for a command's options.
```

`init` is both: its command help, then its `Commands:` list.

## General help (amends 009)

- It is the 009 text, with `completion` added to the command list.
- The line "Run a command with missing or invalid arguments to see its exact usage." becomes "Run 'ai-dev help <command>' or 'ai-dev <command> --help' for a command's options."

## Guarantees

- Help never starts a process, reads stdin, touches files or state, needs a terminal, or uses the network (F010-FR-004).
- Each command's help usage line equals its usage-failure line (SC-002).
