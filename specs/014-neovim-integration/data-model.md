# Data Model: Neovim Integration

No persistent data. Every entity exists only for one command run or one plugin action.

## Editor Location (Go, `internal/editor.Location`)

| Field | Rule |
|---|---|
| `Rel` | Path relative to the project, as given, used only for display and reports |
| `Abs` | `EvalSymlinks(root)/…` after `EvalSymlinks(target)`; must lie under the resolved root |
| `Line` | integer ≥ 1; absent = 1 |
| `Column` | integer ≥ 1; absent = 1 |

Refusal reasons, in check order:
1. `absolute_path`: the given path is absolute.
2. `not_found`: the target does not resolve.
3. `outside_project`: the resolved target is not under the resolved root.
4. `not_regular_file`: the target is a directory, device, socket or pipe.

Neovim clamps a line or column beyond the end of the file (spec Edge Cases).

## Editor Target (Go)

| Value | Condition | Behavior |
|---|---|---|
| `parent` | `$NVIM` is non-empty | `CheckEndpoint`, `Dial`, version check, `nvim_exec_lua(openChunk, {Abs, Line, Column})`; never a fallback |
| `foreground` | no `$NVIM`; stdin and stdout are terminals; `nvim` on PATH | `nvim "+call cursor(Line, Column)" -- Abs`, terminal inherited |
| none | otherwise | `editor_unavailable` |

Endpoint refusal reasons:
- `not_local_socket`: not an absolute path, or a TCP address;
- `not_socket`: a file, directory or symbolic link;
- `not_owned`;
- `no_answer`: dial or call timeout, or closed connection;
- `not_neovim`: the API info has the wrong shape;
- `version_unsupported`: below 0.11.7;
- `editor_error`: Neovim returned an error, reported with its message sanitized.

Editor unavailable reasons:
- `no_terminal`;
- `nvim_not_found`;
- `platform_unsupported`.

## Open Report (Go, JSON `ai-dev.open/v1`)

See [contracts/open.md](contracts/open.md).

## Code Reference (Lua)

| Field | Rule |
|---|---|
| `path` | `vim.fs.relpath(realpath(root), realpath(file))`; refused when nil (outside the project) |
| `range` | `{first, last}` 1-based lines from the visual selection, or absent for the whole file |
| `digest` | `sha256` of the saved file's bytes read from disk |
| `text` | the selected lines, only when the person chose to include them; at most 32 KiB |

Refusals:
- `no_file`: the buffer has no name, or its `buftype` is not empty;
- `unsaved`: the buffer is modified;
- `stale`: the buffer is not modified but no longer matches the file on disk (changed outside Neovim and not reloaded), so its lines would not match the digest (independent review F1);
- `outside_project`;
- `not_regular_file`;
- `selection_too_large`;
- `instruction_too_large`: the composed instruction exceeds 64 KiB;
- `changed`: the digest differs on confirmation.

The composed instruction is the person's text, a blank line, and then:

```text
Reference: <path>, lines <first>-<last> (sha256 <digest>)
```

When text is included, the selected lines follow, between `--- begin selection ---` and `--- end selection ---`. The person sees this exact text in the preview.

## Plugin Action (Lua)

| Field | Content |
|---|---|
| `name` | `ask`, `next`, `approve`, `abandon`, `tui` |
| `available`, `reason` | the conditions of F014-FR-017 (as F012-FR-011); `ask` and `tui` are always available when ai-dev is compatible |
| `fields` | the values asked, none prefilled: `ask` (agent by role or catalog, instruction, include text yes/no), `next` (instruction), `approve` (approver, target state, reason optional), `abandon` (approver, reason required; the target state is `abandoned`, as in the cockpit). The fields, labels and hints are the cockpit's (`internal/cockpit/actions.go`) |
| `argv` | the argument vector, starting with the configured ai-dev path |
| `preview` | the sanitized, quoted command line, plus the reference block for `ask` and `next` with a reference |

State of one action: `collecting` → `previewing` → (`declined` | `confirmed` → `running` → `ended(exit code)`). One confirmation moves at most one action from `previewing` to `running`. Input received while `running` starts nothing.

## Plugin Configuration (Lua)

| Option | Default | Validation |
|---|---|---|
| `cmd` | `"ai-dev"` | non-empty string; resolved with `exepath` |
| `concurrency` | `8` | integer 1–16 |
| `terminal` | `"tab"` | `"tab"`, `"split"` or `"vsplit"` |
| `preset` | `false` | boolean; `true` sets the mappings of `ai-dev.lazyvim` for configurations without lazy.nvim |

An unknown option or an invalid value is reported once with `vim.notify` at warning level, and the default is used for that option.
