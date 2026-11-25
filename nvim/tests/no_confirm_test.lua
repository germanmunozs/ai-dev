-- Without a confirmation of Run, no key, mapping or command of the plugin
-- runs an agent or records a decision (F014-FR-024, SC-006).
local h = require("helpers")

local T = {}

local secret = "BUFFER-CONTENT-7f3a"

-- answer_everything answers every input with a value and every selection
-- with its last item, so each flow reaches its confirmation.
local function answer_everything()
  vim.ui.input = function(_, on_confirm)
    on_confirm("sweep value")
  end
  vim.ui.select = function(items, _, on_choice)
    on_choice(items[#items], #items)
  end
end

local function decisions(log)
  return vim.tbl_filter(function(call)
    return (call[1] == "agent" and call[2] == "run")
      or (call[1] == "workflow" and (call[2] == "approve" or call[2] == "abandon"))
      or call[1] == "tui"
  end, h.fake_calls(log))
end

local function idle()
  local actions = require("ai-dev.actions")
  h.wait_for(function()
    for _, name in ipairs({ "ask", "next", "approve", "abandon", "tui" }) do
      if actions.state(name) ~= nil then
        return false
      end
    end
    return true
  end, "every action to stop")
end

-- open_file opens a saved project file holding the secret line.
local function open_file(root)
  h.write(root .. "/notes.txt", secret .. "\n" .. secret .. "\n")
  vim.cmd.edit({ args = { root .. "/notes.txt" }, magic = { file = false, bar = false } })
end

-- sweep runs every subcommand and every view key with confirm answering
-- choice, and returns the fake's log. extra runs further keys (the preset
-- mappings, T049) after the plugin is set up with opts.
local function sweep(choice, opts, extra)
  local root = h.fixture_project("views")
  local fake, log = h.fixture_ai_dev(h.tmpdir(), { otherwise = "exit 0" })
  require("ai-dev").setup(vim.tbl_extend("force", { cmd = fake }, opts or {}))
  answer_everything()
  local confirms = h.stub_confirm(choice)
  h.capture_notify()

  for _, sub in ipairs({ "features", "doctor", "agents", "next", "approve", "abandon", "tui", "health" }) do
    vim.cmd("silent AiDev " .. sub)
    idle()
    vim.cmd("silent! tabonly! | silent! only!")
  end
  open_file(root)
  vim.cmd("AiDev ask")
  idle()
  vim.cmd("1,2AiDev ask")
  idle()

  local views = require("ai-dev.views")
  views.open("features")
  local buf = vim.api.nvim_get_current_buf()
  h.wait_for(function()
    return views.state(buf) == "complete"
  end)
  for lnum = 1, #h.buf_lines(buf) do
    for _, key in ipairs({ "n", "a", "x", "r" }) do
      if vim.api.nvim_get_current_buf() ~= buf then
        break
      end
      h.wait_for(function()
        return views.state(buf) == "complete"
      end)
      vim.api.nvim_win_set_cursor(0, { lnum, 0 })
      vim.api.nvim_feedkeys(key, "x", false)
      idle()
    end
  end
  -- <CR> opens a file without a confirmation: it runs nothing.
  h.wait_for(function()
    return views.state(buf) == "complete"
  end)
  vim.api.nvim_win_set_cursor(0, { 2, 0 })
  vim.api.nvim_feedkeys("\r", "x", false)

  if extra then
    open_file(root)
    extra()
    idle()
  end
  h.truthy(#confirms > 0, "flows reached their confirmation")
  return log, confirms
end

local function assert_nothing_ran(log)
  h.eq({}, decisions(log), "agent run, approve, abandon or tui")
  for _, call in ipairs(h.fake_calls(log)) do
    for _, arg in ipairs(call) do
      h.eq(nil, arg:find(secret, 1, true), "buffer content in an ai-dev call")
    end
  end
end

local base_confirms -- set by cancel_runs_nothing, which sorts first

function T.cancel_runs_nothing()
  local log, confirms = sweep(2)
  assert_nothing_ran(log)
  h.truthy(#confirms >= 9, "confirmations asked: " .. #confirms)
  base_confirms = #confirms
end

function T.escape_runs_nothing()
  local log = sweep(0)
  assert_nothing_ran(log)
end

-- press_preset_mappings presses every mapping of ai-dev.lazyvim, the
-- visual ones on a selection of the open file (T049).
local function press_preset_mappings()
  local file_buf = vim.api.nvim_get_current_buf()
  for _, key in ipairs(require("ai-dev.lazyvim").keys) do
    -- A view replaces the file in its window; show the file again.
    vim.api.nvim_win_set_buf(0, file_buf)
    -- Typed keys do not expand <leader>; mappings do.
    local keys = (vim.g.mapleader or "\\") .. vim.keycode(key[1]:sub(#"<leader>" + 1))
    if key.mode == "x" then
      keys = "ggVj" .. keys
    end
    vim.api.nvim_feedkeys(keys, "x", false)
    idle()
    vim.cmd("silent! tabonly! | silent! only!")
  end
end

function T.preset_mappings_with_cancel_run_nothing()
  local log, confirms = sweep(2, { preset = true }, press_preset_mappings)
  assert_nothing_ran(log)
  -- The two ask mappings and the tui mapping each reach a confirmation.
  h.eq((base_confirms or 25) + 3, #confirms, "confirmations asked")
end

function T.preset_mappings_with_escape_run_nothing()
  local log = sweep(0, { preset = true }, press_preset_mappings)
  assert_nothing_ran(log)
end

return T
