local h = require("helpers")

local T = {}

-- The cockpit's fields (internal/cockpit/actions.go), pinned here so the
-- plugin asks the same values with the same labels and hints.
local cockpit = {
  next = { { "instruction", "passed to the agent as one argument" } },
  approve = {
    { "approver", "your name, recorded as declared" },
    { "target state", "draft, specified, planned, tasked, implemented, approved or abandoned" },
    { "reason", "optional" },
  },
  abandon = { { "approver", "your name, recorded as declared" }, { "reason", nil } },
}

local function start(otherwise)
  h.fixture_project("views")
  local fake, log = h.fixture_ai_dev(h.tmpdir(), { otherwise = otherwise or "exit 0" })
  require("ai-dev").setup({ cmd = fake })
  return fake, log
end

-- commands returns the logged calls that are not reads.
local function commands(log)
  return vim.tbl_filter(function(call)
    return (call[1] == "agent" and call[2] == "run") or (call[1] == "workflow" and call[2] ~= "guide") or call[1] == "tui"
  end, h.fake_calls(log))
end

local function wait_idle(name)
  local actions = require("ai-dev.actions")
  h.wait_for(function()
    return actions.state(name) == nil
  end, name)
end

-- on_row opens the features view, puts the cursor on id's row and presses
-- key.
local function on_row(id, key)
  local views = require("ai-dev.views")
  views.open("features")
  local buf = vim.api.nvim_get_current_buf()
  h.wait_for(function()
    return views.state(buf) == "complete"
  end)
  for lnum, line in ipairs(h.buf_lines(buf)) do
    if vim.startswith(line, id .. "  ") then
      vim.api.nvim_win_set_cursor(0, { lnum, 0 })
    end
  end
  vim.api.nvim_feedkeys(key, "x", false)
end

local function prompts(asked)
  return vim.tbl_map(function(a)
    return { a.kind, a.opts.prompt, a.opts.default }
  end, asked)
end

local function label(field)
  return field[2] and ("%s (%s): "):format(field[1], field[2]) or (field[1] .. ": ")
end

function T.approve_asks_the_cockpit_fields_and_runs_the_exact_command()
  local fake, log = start()
  local asked = h.stub_ui({ "Ana López", "planned", "" })
  local confirms = h.stub_confirm(1)
  on_row("02-specified", "a")
  wait_idle("approve")
  h.eq(vim.tbl_map(function(f)
    return { "input", label(f), nil }
  end, cockpit.approve), prompts(asked))
  h.eq({ { "workflow", "approve", "--approver", "Ana López", "--", "02-specified", "planned" } }, commands(log))
  local preview = table.concat(confirms[1].preview, "\n")
  h.truthy(preview:find(require("ai-dev.quote").argv({ fake, "workflow", "approve", "--approver", "Ana López", "--", "02-specified", "planned" }), 1, true))
  h.truthy(preview:find("Feature: 02-specified", 1, true))
end

function T.approve_with_a_reason_passes_it_before_the_separator()
  local _, log = start()
  h.stub_ui({ "-a", "-planned", "--reason x" })
  h.stub_confirm(1)
  on_row("03-planned", "a")
  wait_idle("approve")
  h.eq({ { "workflow", "approve", "--approver", "-a", "--reason", "--reason x", "--", "03-planned", "-planned" } }, commands(log))
end

function T.abandon_requires_a_reason()
  local _, log = start()
  local asked = h.stub_ui({ "Ana", "" })
  local confirms = h.stub_confirm(1)
  local notes = h.capture_notify()
  on_row("02-specified", "x")
  wait_idle("abandon")
  h.eq(vim.tbl_map(function(f)
    return { "input", label(f), nil }
  end, cockpit.abandon), prompts(asked))
  h.eq({}, confirms)
  h.eq({}, commands(log))
  h.match("the reason is required", notes[#notes].msg)

  h.stub_ui({ "Ana", "out of scope" })
  h.stub_confirm(1)
  on_row("02-specified", "x")
  wait_idle("abandon")
  h.eq({ { "workflow", "abandon", "--approver", "Ana", "--reason", "out of scope", "--", "02-specified", "abandoned" } }, commands(log))
end

function T.next_runs_the_agent_of_the_next_step()
  local fake, log = start()
  local asked = h.stub_ui({ "Plan it" })
  local confirms = h.stub_confirm(1)
  on_row("02-specified", "n")
  wait_idle("next")
  h.eq({ { "input", label(cockpit.next[1]), nil } }, prompts(asked))
  h.eq({ { "agent", "run", "--", "gemini-cli", "Plan it" } }, commands(log))
  local preview = table.concat(confirms[1].preview, "\n")
  h.truthy(preview:find("Agent: gemini-cli", 1, true))
  h.truthy(preview:find("Instruction:\n  Plan it", 1, true))
  h.truthy(preview:find(require("ai-dev.quote").argv({ fake, "agent", "run", "--", "gemini-cli", "Plan it" }), 1, true))
end

function T.tui_goes_through_the_preview_and_confirmation()
  local fake, log = start()
  local confirms = h.stub_confirm(2)
  require("ai-dev.actions").tui()
  wait_idle("tui")
  h.eq(1, #confirms)
  h.eq({}, commands(log), "Cancel runs nothing")
  h.stub_confirm(1)
  local jobs = h.spy_jobstart()
  require("ai-dev.actions").tui()
  wait_idle("tui")
  h.eq({ fake, "tui" }, jobs[1].cmd)
  h.eq({ { "tui" } }, commands(log))
end

function T.unavailable_actions_show_their_reason_and_start_nothing()
  local _, log = start()
  local cases = {
    { "06-approved", "n", "the feature is in a terminal state" },
    { "06-approved", "a", "the feature is in a terminal state" },
    { "07-abandoned", "x", "the feature is in a terminal state" },
    { "04-tasked", "n", "the agent of the next step is unavailable" },
    { "08-invalid", "n", "no step is recommended (state_invalid)" },
    { "09-inconsistent", "n", "no step is recommended (artifacts_inconsistent)" },
    { "08-invalid", "a", "the feature has no recognized state (state_invalid)" },
    { "08-invalid", "x", "the feature has no recognized state (state_invalid)" },
  }
  for _, case in ipairs(cases) do
    local asked = h.stub_ui({ "x", "y", "z" })
    local notes = h.capture_notify()
    on_row(case[1], case[2])
    h.eq({}, asked, case[1] .. " " .. case[2])
    h.match(vim.pesc(case[3]), notes[#notes] and notes[#notes].msg, case[1] .. " " .. case[2])
  end
  h.eq({}, commands(log))
end

function T.typed_elsewhere_the_feature_is_asked_from_the_list()
  local _, log = start()
  local asked = h.stub_ui({ 2, "Ana", "planned", "" })
  h.stub_confirm(1)
  vim.cmd("AiDev approve")
  wait_idle("approve")
  h.eq("select", asked[1].kind)
  h.eq("01-draft", asked[1].items[1])
  h.eq(nil, asked[1].opts.default, "nothing preselected")
  h.eq({ { "workflow", "approve", "--approver", "Ana", "--", "02-specified", "planned" } }, commands(log))
end

function T.a_failed_command_shows_its_exit_code_and_nothing_else()
  start("echo refused >&2; exit 1")
  h.stub_ui({ "Ana", "approved", "" })
  h.stub_confirm(1)
  local notes = h.capture_notify()
  on_row("02-specified", "a")
  wait_idle("approve")
  local messages = vim.tbl_map(function(n)
    return n.msg
  end, notes)
  h.eq({ "ai-dev exited with code 1" }, messages)
end

return T
