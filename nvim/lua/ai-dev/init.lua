-- The ai-dev Neovim plugin (Feature 014, contracts/plugin.md). Requiring a
-- module defines nothing; setup() defines the :AiDev command and the AiDev
-- autocommand group, and teardown() removes them. There is no plugin/
-- directory, so nothing loads before setup() (R-05).
local M = {}

local state = { cfg = nil, group = nil, preset = nil }

local subcommands = { "features", "doctor", "agents", "ask", "next", "approve", "abandon", "tui", "health" }

local handlers = {
  features = function()
    require("ai-dev.views").open("features")
  end,
  doctor = function()
    require("ai-dev.views").open("doctor")
  end,
  agents = function()
    require("ai-dev.views").open("agents")
  end,
  ask = function(range)
    require("ai-dev.actions").ask(range)
  end,
  next = function()
    require("ai-dev.actions").next()
  end,
  approve = function()
    require("ai-dev.actions").approve()
  end,
  abandon = function()
    require("ai-dev.actions").abandon()
  end,
  tui = function()
    require("ai-dev.actions").tui()
  end,
  health = function()
    vim.cmd.checkhealth("ai-dev")
  end,
}

local function dispatch(opts)
  local name = opts.fargs[1]
  local handler = name and handlers[name]
  if not handler then
    local found = name and ("unknown subcommand %s; "):format(name) or "missing subcommand; "
    vim.notify("ai-dev: " .. found .. "use one of " .. table.concat(subcommands, ", "), vim.log.levels.ERROR)
    return
  end
  handler(opts.range > 0 and { opts.line1, opts.line2 } or nil)
end

-- setup resolves opts and defines the command. A second call replaces the
-- first.
function M.setup(opts)
  M.teardown()
  state.cfg = require("ai-dev.config").resolve(opts)
  state.group = vim.api.nvim_create_augroup("AiDev", { clear = true })
  vim.api.nvim_create_user_command("AiDev", dispatch, {
    nargs = "?",
    range = true,
    desc = "ai-dev",
    complete = function(lead)
      return vim.tbl_filter(function(s)
        return vim.startswith(s, lead)
      end, subcommands)
    end,
  })
  if state.cfg.preset then
    -- The mappings of ai-dev.lazyvim, for configurations without lazy.nvim
    -- (F014-FR-019); never set otherwise.
    local preset = require("ai-dev.lazyvim")
    state.preset = {}
    for _, key in ipairs(preset.keys) do
      local mode = key.mode or "n"
      vim.keymap.set(mode, key[1], key[2], { desc = key.desc, silent = true })
      state.preset[#state.preset + 1] = { mode, key[1] }
    end
    local ok, wk = pcall(require, "which-key")
    if ok and type(wk) == "table" and wk.add then
      wk.add({ preset.group })
    end
  end
end

-- config returns the configuration resolved by setup().
function M.config()
  return state.cfg or require("ai-dev.config").resolve(nil)
end

-- group returns the AiDev autocommand group, or nil before setup().
function M.group()
  return state.group
end

-- teardown closes the plugin's views and preview, forgets the actions'
-- state, and removes the command, the preset mappings and the autocommand
-- group.
function M.teardown()
  local views, cli = package.loaded["ai-dev.views"], package.loaded["ai-dev.cli"]
  local actions = package.loaded["ai-dev.actions"]
  if views and views.close_all then
    views.close_all()
  end
  if actions and actions.reset then
    actions.reset()
  end
  if cli and cli.reset then
    cli.reset()
  end
  for _, key in ipairs(state.preset or {}) do
    pcall(vim.keymap.del, key[1], key[2])
  end
  state.preset = nil
  if state.group then
    pcall(vim.api.nvim_del_augroup_by_id, state.group)
    state.group = nil
  end
  pcall(vim.api.nvim_del_user_command, "AiDev")
  state.cfg = nil
end

return M
