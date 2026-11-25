-- End to end: approve and abandon through the plugin with the ai-dev binary
-- built from this branch (SC-004). TestNvimPluginDecisions in
-- cmd/ai-dev/nvimplugin_test.go sets AI_DEV_E2E_BIN, AI_DEV_E2E_PROJECT
-- and AI_DEV_E2E_OUT, types the same commands by hand on a copy of the
-- project, and compares; without them this file has no tests.
local h = require("helpers")

local bin = os.getenv("AI_DEV_E2E_BIN")
local project, out = os.getenv("AI_DEV_E2E_PROJECT"), os.getenv("AI_DEV_E2E_OUT")
if not bin or not project or not out then
  return {}
end

local T = {}

-- The decisions, in the order TestNvimPluginDecisions types them by hand.
local steps = {
  { feature = "01-draft", key = "a", answers = { "Ana", "specified", "" } },
  { feature = "02-specified", key = "x", answers = { "Ana", "out of scope" } },
  { feature = "03-planned", key = "a", answers = { "Ana", "tasked", "no tasks yet" } },
}

-- terminal_output returns the text the command printed in its terminal,
-- without the trailing blank lines of the terminal's screen.
local function terminal_output(buf)
  h.wait_for(function()
    return table.concat(h.buf_lines(buf), "") ~= ""
  end, "the command's output")
  vim.wait(100) -- let the last output reach the buffer
  local lines = {}
  for _, line in ipairs(h.buf_lines(buf)) do
    if not line:find("^%[Process exited") then
      lines[#lines + 1] = (line:gsub("%s+$", ""))
    end
  end
  while lines[#lines] == "" do
    lines[#lines] = nil
  end
  return table.concat(lines, "\n") .. "\n"
end

function T.decisions_equal_the_commands_typed_by_hand()
  vim.o.columns = 1000 -- so the terminal wraps no line
  vim.cmd.cd(vim.fn.fnameescape(project))
  require("ai-dev").setup({ cmd = bin })
  local views, actions = require("ai-dev.views"), require("ai-dev.actions")
  local results = {}
  for _, step in ipairs(steps) do
    views.open("features")
    local view = vim.api.nvim_get_current_buf()
    h.wait_for(function()
      return views.state(view) == "complete"
    end, "features view", 15000)
    for lnum, line in ipairs(h.buf_lines(view)) do
      if vim.startswith(line, step.feature .. "  ") then
        vim.api.nvim_win_set_cursor(0, { lnum, 0 })
      end
    end
    h.stub_ui(step.answers)
    h.stub_confirm(1)
    local notes = h.capture_notify()
    vim.api.nvim_feedkeys(step.key, "x", false)
    local name = step.key == "a" and "approve" or "abandon"
    h.wait_for(function()
      return actions.state(name) == nil
    end, name, 15000)
    local output = terminal_output(vim.api.nvim_get_current_buf())
    local code = tonumber(notes[#notes].msg:match("^ai%-dev exited with code (%d+)$"))
    h.truthy(code, "exit code shown: " .. vim.inspect(notes))
    results[#results + 1] = { output = output, exit_code = code }
    vim.cmd("silent! tabonly!")
  end
  h.write(out, vim.json.encode(results))
end

return T
