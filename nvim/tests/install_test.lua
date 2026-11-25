-- The plugin writes no file, starts only the configured ai-dev, and shows
-- no environment value (F014-FR-018, FR-022, FR-025, US5).
-- TestNvimPluginInstall sets AI_DEV_TEST_SECRET and AI_DEV_MARKER_DIR and
-- puts marker executables first on PATH; without them this file has no
-- tests.
local h = require("helpers")

local secret, markers = os.getenv("AI_DEV_TEST_SECRET"), os.getenv("AI_DEV_MARKER_DIR")
if not secret or not markers then
  return {}
end

local T = {}

-- listing describes every entry under dir with its type, size and
-- modification time.
local function listing(dir)
  local entries = {}
  local function walk(path, rel)
    local handle = vim.uv.fs_scandir(path)
    while handle do
      local name, type = vim.uv.fs_scandir_next(handle)
      if not name then
        break
      end
      local full, child = path .. "/" .. name, rel .. "/" .. name
      local stat = vim.uv.fs_lstat(full)
      entries[#entries + 1] = ("%s %s %d %d.%d"):format(child, type, stat.size, stat.mtime.sec, stat.mtime.nsec)
      if type == "directory" then
        walk(full, child)
      end
    end
  end
  walk(dir, "")
  table.sort(entries)
  return entries
end

function T.views_and_actions_write_nothing_start_only_ai_dev_and_show_no_secret()
  local home = vim.env.HOME
  local root = h.fixture_project("views")
  h.write(root .. "/notes.txt", "one\ntwo\n")
  local fake, log = h.fixture_ai_dev(h.tmpdir(), { otherwise = "exit 0" })
  local before_home, before_project = listing(home), listing(root)

  local shown = {}
  vim.notify = function(msg)
    shown[#shown + 1] = msg
  end
  local confirms = h.stub_confirm(1)
  vim.ui.input = function(_, on_confirm)
    on_confirm("value")
  end
  vim.ui.select = function(items, _, on_choice)
    on_choice(items[1], 1)
  end
  require("ai-dev").setup({ cmd = fake, preset = true })
  local views, actions = require("ai-dev.views"), require("ai-dev.actions")
  local function idle()
    h.wait_for(function()
      for _, name in ipairs({ "ask", "next", "approve", "abandon", "tui" }) do
        if actions.state(name) ~= nil then
          return false
        end
      end
      return true
    end, "actions", 10000)
  end

  for _, kind in ipairs({ "features", "doctor", "agents" }) do
    views.open(kind)
    local buf = vim.api.nvim_get_current_buf()
    h.wait_for(function()
      return views.state(buf) == "complete"
    end)
    vim.list_extend(shown, h.buf_lines(buf))
  end
  for _, key in ipairs({ "n", "a", "x" }) do
    views.open("features")
    local view = vim.api.nvim_get_current_buf()
    h.wait_for(function()
      return views.state(view) == "complete"
    end)
    vim.api.nvim_win_set_cursor(0, { 3, 0 }) -- 02-specified
    vim.api.nvim_feedkeys(key, "x", false)
    idle()
    vim.cmd("silent! tabonly! | silent! only!")
  end
  vim.cmd.edit({ args = { root .. "/notes.txt" }, magic = { file = false, bar = false } })
  actions.ask({ 1, 2 })
  idle()
  vim.cmd("silent! tabonly! | silent! only!")
  actions.tui()
  idle()
  vim.cmd("silent! tabonly! | silent! only!")
  vim.cmd("silent checkhealth ai-dev")
  vim.list_extend(shown, h.buf_lines(0))
  for _, c in ipairs(confirms) do
    vim.list_extend(shown, c.preview or {})
  end
  require("ai-dev").teardown()

  h.eq(5, #confirms, "every action reached its confirmation")
  local started = {}
  for _, call in ipairs(h.fake_calls(log)) do
    started[call[1]] = true
  end
  h.truthy(started.agent and started.workflow and started.tui, "the actions ran ai-dev")
  h.eq({}, vim.fn.glob(markers .. "/*.ran", false, true), "only the configured ai-dev was started")
  h.eq(before_home, listing(home), "nothing written under HOME")
  h.eq(before_project, listing(root), "nothing written in the project")
  local text = table.concat(shown, "\n")
  for name, value in pairs({ HOME = home, NVIM = vim.env.NVIM, AI_DEV_TEST_SECRET = secret }) do
    if value and value ~= "" then
      h.eq(nil, text:find(value, 1, true), name .. " shown")
    end
  end
end

return T
