-- Test runner for the ai-dev Neovim plugin (Feature 014, research R-08).
-- Run with: nvim --headless --clean -l nvim/tests/run.lua
-- Each nvim/tests/*_test.lua returns a table of test functions by name
-- (names starting with "_" are helpers, not tests);
-- $AI_DEV_LUA_TESTS, comma-separated, limits the run to the files named
-- (a name with a slash is a path, otherwise a file in nvim/tests/).

local script = vim.fn.fnamemodify(arg[0], ":p")
local tests_dir = vim.fn.fnamemodify(script, ":h")
local plugin_dir = vim.fn.fnamemodify(tests_dir, ":h")
vim.opt.rtp:prepend(plugin_dir)
package.path = tests_dir .. "/?.lua;" .. package.path

local files = {}
local only = os.getenv("AI_DEV_LUA_TESTS") or ""
if only ~= "" then
  for name in vim.gsplit(only, ",", { plain = true, trimempty = true }) do
    files[#files + 1] = name:find("/", 1, true) and name or (tests_dir .. "/" .. name)
  end
else
  files = vim.fn.glob(tests_dir .. "/*_test.lua", false, true)
  table.sort(files)
end

local saved = {
  input = vim.ui.input,
  select = vim.ui.select,
  confirm = vim.fn.confirm,
  jobstart = vim.fn.jobstart,
  -- Notifications a test does not capture are shown on their own lines.
  notify = function(msg)
    io.stdout:write("  [notify] ", tostring(msg):gsub("\n", "\n  "), "\n")
  end,
  cwd = vim.fn.getcwd(),
}

-- reset gives every test the state of a fresh Neovim: one window, one
-- empty buffer, the original working directory and UI functions, and no
-- running job.
local function reset()
  local plugin = package.loaded["ai-dev"]
  if plugin and plugin.teardown then
    pcall(plugin.teardown)
  end
  vim.ui.input, vim.ui.select = saved.input, saved.select
  vim.fn.confirm, vim.fn.jobstart, vim.notify = saved.confirm, saved.jobstart, saved.notify
  for _, chan in ipairs(vim.api.nvim_list_chans()) do
    if chan.mode == "terminal" then
      pcall(vim.fn.jobstop, chan.id)
    end
  end
  vim.cmd("silent! tabonly! | silent! only!")
  vim.cmd("enew!")
  local keep = vim.api.nvim_get_current_buf()
  for _, buf in ipairs(vim.api.nvim_list_bufs()) do
    if buf ~= keep then
      pcall(vim.api.nvim_buf_delete, buf, { force = true })
    end
  end
  vim.cmd.cd(vim.fn.fnameescape(saved.cwd))
end

local function out(line)
  io.stdout:write(line, "\n")
end

local failed, ran = 0, 0
for _, file in ipairs(files) do
  local short = vim.fn.fnamemodify(file, ":t")
  local ok, suite = pcall(dofile, file)
  if not ok or type(suite) ~= "table" then
    failed = failed + 1
    out(("FAIL %s (load): %s"):format(short, ok and "does not return a table" or suite))
  else
    -- A name starting with "_" is a helper another test file may use.
    local names = vim.tbl_filter(function(name)
      return not vim.startswith(name, "_")
    end, vim.tbl_keys(suite))
    table.sort(names)
    for _, name in ipairs(names) do
      reset()
      ran = ran + 1
      local passed, err = xpcall(suite[name], debug.traceback)
      if passed then
        out(("ok %s %s"):format(short, name))
      else
        failed = failed + 1
        out(("FAIL %s %s: %s"):format(short, name, err))
      end
    end
  end
end
reset()

out(("%d tests, %d failed"):format(ran, failed))
vim.cmd("cquit " .. (failed > 0 and 1 or 0))
