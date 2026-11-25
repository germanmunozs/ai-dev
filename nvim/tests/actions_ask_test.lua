local h = require("helpers")

local T = {}

local digest_hello = "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"

-- start makes the views fixture project the working directory, opens a
-- saved file holding "hello" in it, and sets up the plugin with a fake
-- ai-dev whose `agent run` exits with code 7.
local function start(opts)
  local root = h.fixture_project("views")
  h.write(root .. "/src/main.go", (opts and opts.content) or "hello\n")
  vim.cmd.edit({ args = { root .. "/src/main.go" }, magic = { file = false, bar = false } })
  local fake, log = h.fixture_ai_dev(h.tmpdir(), { otherwise = "exit 7" })
  require("ai-dev").setup(vim.tbl_extend("force", { cmd = fake }, (opts and opts.setup) or {}))
  return log, fake, root
end

local function runs(log)
  return vim.tbl_filter(function(call)
    return call[1] == "agent" and call[2] == "run"
  end, h.fake_calls(log))
end

-- ask runs the action and waits until it has asked for confirmation (or
-- stopped), then until a started terminal job has ended.
local function ask(range)
  local actions = require("ai-dev.actions")
  actions.ask(range)
  h.wait_for(function()
    return actions.state("ask") == nil or actions.state("ask") == "ended"
  end, "the ask action")
end

function T.by_role_asks_mode_role_instruction_then_previews_and_runs()
  local log, fake = start()
  local asked = h.stub_ui({ 1, 1, "Explain this" })
  local confirms = h.stub_confirm(1)
  local jobs = h.spy_jobstart()
  local notes = h.capture_notify()
  ask(nil)

  h.eq("select", asked[1].kind)
  h.eq({ "by role", "from the catalog" }, asked[1].items)
  h.eq("select", asked[2].kind)
  h.eq({ "specification", "planning", "implementation", "review", "remediation" }, asked[2].items)
  h.eq("input", asked[3].kind)
  h.eq(nil, asked[3].opts.default, "no default instruction")
  h.eq(3, #asked, "no include-text question without a range")

  local instruction = "Explain this\n\nReference: src/main.go (sha256 " .. digest_hello .. ")"
  h.truthy(vim.tbl_contains(h.fake_calls(log), function(c)
    return vim.deep_equal(c, { "agent", "select", "--format", "json", "--", "specification" })
  end, { predicate = true }), "agent select by role")
  h.eq({ { "agent", "run", "--", "claude-code", instruction } }, runs(log))

  h.eq(1, #confirms)
  h.eq({ "Run this command?", "&Run\n&Cancel", 2 }, confirms[1].args)
  local preview = table.concat(confirms[1].preview, "\n")
  h.truthy(preview:find("Agent: claude-code", 1, true), "agent in preview")
  h.truthy(preview:find("Reference: src/main.go (whole file)", 1, true), "reference in preview")
  h.truthy(preview:find("Digest: sha256 " .. digest_hello, 1, true), "digest in preview")
  h.truthy(preview:find("Selected text: not included", 1, true), "text choice in preview")
  h.truthy(preview:find("  Explain this\n  \n  Reference: src/main.go (sha256 " .. digest_hello .. ")", 1, true), "instruction in preview")
  local quoted = require("ai-dev.quote").argv({ fake, "agent", "run", "--", "claude-code", instruction })
  h.truthy(preview:find(quoted, 1, true), "command in preview")
  h.eq(nil, h.preview_lines(), "preview closed")

  h.eq(1, #jobs)
  h.eq("table", type(jobs[1].cmd), "jobstart receives a list")
  h.eq({ fake, "agent", "run", "--", "claude-code", instruction }, jobs[1].cmd)
  h.eq(true, jobs[1].opts.term)
  h.eq(vim.fn.getcwd(), jobs[1].opts.cwd)
  h.eq(2, vim.fn.tabpagenr("$"), "a new tab")
  h.truthy(vim.tbl_contains(notes, function(n)
    return n.msg == "ai-dev exited with code 7"
  end, { predicate = true }), "exit code shown")
end

function T.from_the_catalog_lists_the_agents()
  local log = start()
  local asked = h.stub_ui({ 2, 4, "Why?" })
  h.stub_confirm(1)
  ask(nil)
  local items = vim.tbl_map(function(item)
    return type(item) == "table" and item.id or item
  end, asked[2].items)
  local agents = vim.tbl_map(function(a)
    return a.id
  end, h.fixture("agents").agents)
  h.eq(agents, items)
  h.eq("gemini-cli", runs(log)[1][4])
end

function T.a_range_asks_whether_to_include_the_text()
  local log = start({ content = "one\ntwo\nthree\n" })
  local asked = h.stub_ui({ 1, 1, "Explain", 2 })
  local confirms = h.stub_confirm(1)
  ask({ 2, 3 })
  h.eq("select", asked[4].kind)
  h.eq({ "do not include the selected text", "include the selected text" }, asked[4].items)
  local digest = vim.fn.sha256("one\ntwo\nthree\n")
  local instruction = "Explain\n\nReference: src/main.go, lines 2-3 (sha256 " .. digest .. ")\n"
    .. "--- begin selection ---\ntwo\nthree\n--- end selection ---"
  h.eq(instruction, runs(log)[1][5])
  local preview = table.concat(confirms[1].preview, "\n")
  h.truthy(preview:find("Reference: src/main.go, lines 2-3", 1, true))
  h.truthy(preview:find("Selected text: included", 1, true))
end

function T.cancel_or_escape_runs_nothing()
  for _, answer in ipairs({ 2, 0 }) do
    local log = start()
    h.stub_ui({ 1, 1, "Explain" })
    local confirms = h.stub_confirm(answer)
    local jobs = h.spy_jobstart()
    ask(nil)
    h.eq(1, #confirms)
    h.eq({}, runs(log), "confirm " .. answer)
    h.eq({}, jobs)
    require("ai-dev").teardown()
  end
end

function T.cancelling_any_input_runs_nothing()
  local answers = { { nil }, { 1, nil }, { 1, 1, nil }, { 1, 1, "" }, { 2, nil } }
  for _, list in ipairs(answers) do
    local log = start()
    h.stub_ui(list)
    local confirms = h.stub_confirm(1)
    ask(nil)
    h.eq({}, confirms, vim.inspect(list))
    h.eq({}, runs(log))
    require("ai-dev").teardown()
  end
end

function T.cancelling_the_include_text_question_runs_nothing()
  local log = start({ content = "a\nb\n" })
  h.stub_ui({ 1, 1, "Explain", nil })
  local confirms = h.stub_confirm(1)
  ask({ 1, 2 })
  h.eq({}, confirms)
  h.eq({}, runs(log))
end

function T.a_role_without_an_available_agent_stops_with_its_outcome()
  local log = start()
  h.stub_ui({ 1, 3 })
  local confirms = h.stub_confirm(1)
  local notes = h.capture_notify()
  ask(nil)
  h.eq({}, confirms)
  h.eq({}, runs(log))
  h.match("agent_unavailable", notes[#notes].msg)
end

function T.a_file_changed_after_the_preview_is_refused()
  local log, _, root = start()
  h.stub_ui({ 1, 1, "Explain" })
  local notes = h.capture_notify()
  local confirms = {}
  vim.fn.confirm = function(...)
    confirms[#confirms + 1] = { ... }
    h.write(root .. "/src/main.go", "changed\n")
    return 1
  end
  ask(nil)
  h.eq(1, #confirms)
  h.eq({}, runs(log), "nothing runs")
  h.truthy(vim.tbl_contains(notes, function(n)
    return n.msg:find("changed", 1, true) ~= nil
  end, { predicate = true }), "changed notified")
  local again = table.concat(h.preview_lines() or {}, "\n")
  h.truthy(again:find("Refused (changed)", 1, true), "the preview is shown again")
  h.truthy(again:find("Agent: claude-code", 1, true))
end

function T.refusals_of_the_reference_stop_before_any_question()
  start()
  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "edited" })
  local asked = h.stub_ui({ 1, 1, "x" })
  local notes = h.capture_notify()
  ask(nil)
  h.eq({}, asked)
  h.match("unsaved", notes[#notes].msg)
end

function T.only_one_ask_runs_at_a_time()
  local root = h.fixture_project("views")
  h.write(root .. "/a.txt", "hello\n")
  vim.cmd.edit({ args = { root .. "/a.txt" }, magic = { file = false, bar = false } })
  local fake, log = h.fixture_ai_dev(h.tmpdir(), { otherwise = "sleep 0.5" })
  require("ai-dev").setup({ cmd = fake })
  local actions = require("ai-dev.actions")
  h.stub_ui({ 1, 1, "one", 1, 1, "two" })
  local notes = h.capture_notify()
  h.stub_confirm(1)
  local file_win = vim.api.nvim_get_current_win()
  actions.ask(nil)
  h.wait_for(function()
    return actions.state("ask") == "running"
  end)
  vim.api.nvim_set_current_win(file_win)
  actions.ask(nil)
  h.match("already running", notes[#notes].msg)
  h.wait_for(function()
    return actions.state("ask") == nil
  end)
  h.eq(1, #runs(log))
end

function T.values_reach_ai_dev_unchanged_as_single_arguments()
  local hostile = "-a b 'c' \"d\" $HOME `id` ;e|f\nnext line"
  local root = h.fixture_project("views")
  local name = "-x $y `z`;'q'.txt"
  h.write(root .. "/" .. name, "hello\n")
  vim.cmd.edit({ args = { root .. "/" .. name }, magic = { file = false, bar = false } })
  local fake, log = h.fixture_ai_dev(h.tmpdir(), { otherwise = "exit 0" })
  require("ai-dev").setup({ cmd = fake })
  h.stub_ui({ 1, 1, hostile })
  h.stub_confirm(1)
  ask(nil)
  local want = hostile .. "\n\nReference: " .. name .. " (sha256 " .. digest_hello .. ")"
  h.eq({ { "agent", "run", "--", "claude-code", want } }, runs(log))
end

function T.the_open_views_are_refreshed_when_the_command_ends()
  start()
  local views = require("ai-dev.views")
  local refreshed = 0
  local real = views.refresh_all
  views.refresh_all = function()
    refreshed = refreshed + 1
  end
  h.stub_ui({ 1, 1, "x" })
  h.stub_confirm(1)
  ask(nil)
  views.refresh_all = real
  h.eq(1, refreshed)
end

function T.the_terminal_follows_the_configuration()
  for _, case in ipairs({ { "split", 2, 1 }, { "vsplit", 2, 1 } }) do
    start({ setup = { terminal = case[1] } })
    h.stub_ui({ 1, 1, "x" })
    h.stub_confirm(1)
    ask(nil)
    h.eq(case[2], #vim.api.nvim_tabpage_list_wins(0), case[1] .. " windows")
    h.eq(case[3], vim.fn.tabpagenr("$"), case[1] .. " tabs")
    require("ai-dev").teardown()
    vim.cmd("silent! only!")
  end
end

return T
