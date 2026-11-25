local h = require("helpers")

local T = {}

local modes = { "n", "v", "x", "i" }

local function keymaps()
  local all = {}
  for _, mode in ipairs(modes) do
    all[mode] = vim.api.nvim_get_keymap(mode)
  end
  return all
end

local function has_group()
  return (pcall(vim.api.nvim_get_autocmds, { group = "AiDev" }))
end

local function fresh_modules()
  for name in pairs(package.loaded) do
    if name == "ai-dev" or name:find("^ai%-dev%.") then
      package.loaded[name] = nil
    end
  end
end

function T.nothing_is_defined_before_setup()
  fresh_modules()
  local before = keymaps()
  for _, name in ipairs({ "ai-dev", "ai-dev.config", "ai-dev.cli", "ai-dev.sanitize", "ai-dev.quote" }) do
    require(name)
  end
  h.eq(nil, vim.api.nvim_get_commands({})["AiDev"], ":AiDev before setup")
  h.eq(false, has_group(), "AiDev augroup before setup")
  h.eq(before, keymaps(), "keymaps")
end

function T.setup_defines_the_command_with_completion_and_no_mapping()
  local before = keymaps()
  require("ai-dev").setup({})
  local command = vim.api.nvim_get_commands({})["AiDev"]
  h.truthy(command, ":AiDev after setup")
  h.eq("?", command.nargs)
  h.truthy(has_group(), "AiDev augroup")
  h.eq({ "features", "doctor", "agents", "ask", "next", "approve", "abandon", "tui", "health" }, vim.fn.getcompletion("AiDev ", "cmdline"))
  h.eq({ "features" }, vim.fn.getcompletion("AiDev fe", "cmdline"))
  h.eq(before, keymaps(), "no mapping (owner decision 1, F014-FR-019)")
end

function T.subcommands_dispatch_to_their_modules()
  local calls = {}
  package.loaded["ai-dev.views"] = {
    open = function(kind)
      calls[#calls + 1] = { "views", kind }
    end,
  }
  package.loaded["ai-dev.actions"] = {
    ask = function(range)
      calls[#calls + 1] = { "ask", range }
    end,
    tui = function()
      calls[#calls + 1] = { "tui" }
    end,
  }
  require("ai-dev").setup({})
  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "1", "2", "3", "4" })
  vim.cmd("AiDev features")
  vim.cmd("AiDev doctor")
  vim.cmd("AiDev agents")
  vim.cmd("AiDev ask")
  vim.cmd("2,3AiDev ask")
  vim.cmd("AiDev tui")
  h.eq({
    { "views", "features" },
    { "views", "doctor" },
    { "views", "agents" },
    { "ask", nil },
    { "ask", { 2, 3 } },
    { "tui" },
  }, calls)
  package.loaded["ai-dev.views"], package.loaded["ai-dev.actions"] = nil, nil
end

function T.unknown_or_missing_subcommand_notifies_an_error_and_runs_nothing()
  local calls = 0
  package.loaded["ai-dev.views"] = {
    open = function()
      calls = calls + 1
    end,
  }
  require("ai-dev").setup({})
  local notes = h.capture_notify()
  vim.cmd("AiDev bogus")
  vim.cmd("AiDev")
  h.eq(2, #notes)
  for _, note in ipairs(notes) do
    h.eq(vim.log.levels.ERROR, note.level)
    h.match("features", note.msg)
  end
  h.match("bogus", notes[1].msg)
  h.eq(0, calls)
  package.loaded["ai-dev.views"] = nil
end

function T.setup_keeps_the_resolved_configuration()
  local plugin = require("ai-dev")
  plugin.setup({ concurrency = 3 })
  h.eq(3, plugin.config().concurrency)
  h.eq("ai-dev", plugin.config().cmd)
end

function T.teardown_removes_everything_setup_defined()
  local before = keymaps()
  local plugin = require("ai-dev")
  plugin.setup({})
  plugin.setup({}) -- a second setup replaces the first
  plugin.teardown()
  h.eq(nil, vim.api.nvim_get_commands({})["AiDev"])
  h.eq(false, has_group())
  h.eq(before, keymaps())
  plugin.teardown() -- a second teardown is harmless
end

return T
