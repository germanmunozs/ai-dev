# Contract: the Neovim plugin

## Installation (lazy.nvim, R-05)

```lua
{
  "germanmunozs/ai-dev",
  version = "*",          -- latest release tag
  cmd = "AiDev",
  config = function(plugin, opts)
    vim.opt.rtp:append(plugin.dir .. "/nvim")
    require("ai-dev").setup(opts)
  end,
  opts = {},              -- see Configuration
}
```

LazyVim preset: the same specification plus a literal `keys` table. It must be literal: lazy.nvim evaluates the specification before `config` puts `nvim/` on the runtime path, and before the first clone, so `require("ai-dev.lazyvim")` would fail there.

```lua
keys = {
  { "<leader>if", "<cmd>AiDev features<cr>", desc = "ai-dev features" },
  { "<leader>id", "<cmd>AiDev doctor<cr>",   desc = "ai-dev doctor" },
  { "<leader>ia", "<cmd>AiDev agents<cr>",   desc = "ai-dev agents" },
  { "<leader>ii", "<cmd>AiDev ask<cr>",      desc = "ai-dev ask" },
  { "<leader>ii", ":AiDev ask<cr>", mode = "x", desc = "ai-dev ask (selection)" },
  { "<leader>it", "<cmd>AiDev tui<cr>",      desc = "ai-dev tui" },
},
```

`lua/ai-dev/lazyvim.lua` holds the same table; a test checks that the README block equals it. Without lazy.nvim: put `nvim/` on `'runtimepath'` and call `require("ai-dev").setup()`, or `setup({ preset = true })` to set the same mappings.

## Command

`:AiDev {subcommand}`, with completion of subcommands.

| Subcommand | Effect |
|---|---|
| `features` | Opens the features view |
| `doctor` | Opens the environment view |
| `agents` | Opens the agents view |
| `ask` | In normal mode, a reference to the whole file; with a range (`:'<,'>AiDev ask`), to those lines. Starts the `ask` action |
| `tui` | Opens `ai-dev tui` in a Neovim terminal |
| `health` | Runs `:checkhealth ai-dev` |

`next`, `approve` and `abandon` act on the feature under the cursor in the features view. Typed elsewhere, they ask for the feature from the list of features, with nothing preselected.

## Views

Read-only scratch buffers (`buftype=nofile`, `modifiable=false`, `bufhidden=wipe`), with one line per item and the evaluation time on the first line. Every string from ai-dev or from file names passes through `sanitize.lua`. Buffer-local keys work in those buffers only:

| Key | Effect |
|---|---|
| `r` | Refresh |
| `<CR>` | Features view: list the feature's files and open the chosen one (F014-FR-009). Elsewhere: nothing |
| `n`, `a`, `x` | `next`, `approve`, `abandon` for the feature under the cursor, through the preview and confirmation |
| `q` | Close the view |

Features row: `<id>  <current_state | outcome>  <activity>  <role>  <agent> (<availability>, <source>)`. These are exactly the fields of `ai-dev.workflow-guide/v1`; a missing field is shown as `-`, and nothing is inferred.

## Actions

Every action:
1. collects its fields with `vim.ui.input` or `vim.ui.select`, with no default;
2. shows the preview in a floating window: the quoted command line, plus, for `ask` and `next`, the agent, the reference, whether text is included, and the complete instruction;
3. asks `vim.fn.confirm("Run this command?", "&Run\n&Cancel", 2)`. The default is Cancel; only `Run` continues;
4. for a reference, computes the digest again and refuses with `changed` if it differs;
5. opens a terminal (configured as `tab`, `split` or `vsplit`) and runs `jobstart(argv, {term = true, cwd = root, on_exit = …})`;
6. on exit, shows `ai-dev exited with code N` with `vim.notify` (the terminal keeps the command's own output), and refreshes the open views.

Argument vectors (the configured `cmd` first):

| Action | argv after `cmd` |
|---|---|
| `ask` | `agent run -- <agent-id> <instruction>`, where the agent comes from `ai-dev agent select --format json -- <role>` or from the catalog list |
| `next` | `agent run -- <guide.agent> <instruction>` |
| `approve` | `workflow approve --approver <a> [--reason <r>] -- <feature> <state>` |
| `abandon` | `workflow abandon --approver <a> --reason <r> -- <feature> abandoned` |
| `tui` | `tui` |

## Compatibility (R-09)

On first use and in the health check, `cmd version --format json` must report `ai-dev.version/v1` with a version ≥ v0.3.0 and < v1.0.0, or a development build. Each JSON read must carry its known schema; the outputs without a schema field (`doctor`, `agents`, `agent select`) must have the shapes of R-09. Otherwise every view shows the incompatibility and no action is offered.

## Health check (`:checkhealth ai-dev`)

It reports:
- the Neovim version, ok when ≥ 0.11.7;
- the resolved ai-dev path;
- the ai-dev version and its compatibility, with a warning for a development build;
- whether Neovim's working directory has a `specs/` directory, for information only.

## Configuration

See data-model.md, Plugin Configuration.

## LazyVim preset (`ai-dev.lazyvim`, R-06)

| Keys | Effect |
|---|---|
| `<leader>if` | `:AiDev features` |
| `<leader>id` | `:AiDev doctor` |
| `<leader>ia` | `:AiDev agents` |
| `<leader>ii` | `:AiDev ask`; in visual mode with the selection |
| `<leader>it` | `:AiDev tui` |

The same mappings as the literal block of Installation. A which-key group named `ai-dev` is defined only if which-key is present. No preset key replaces a LazyVim v16.0.2 default mapping.

## Removal

Removing the specification from lazy.nvim and restarting leaves nothing: the plugin writes no file, and defines its command, highlights and autocommands only inside `setup()`. In the same session, `require("ai-dev").teardown()` removes the command, the preset keys and the plugin's autocommand group, and the tests use it to check SC for US5.
