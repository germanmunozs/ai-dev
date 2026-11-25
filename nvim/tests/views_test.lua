local h = require("helpers")

local T = {}

local ESC = "\27"

-- start sets up the plugin with a fake ai-dev serving the fixtures in the
-- fixture project named project, and returns the fake's log path.
local function start(project, opts)
  local root = h.fixture_project(project)
  opts = vim.tbl_extend("force", { project = project }, opts or {})
  local fake, log = h.fixture_ai_dev(h.tmpdir(), opts)
  require("ai-dev").setup({ cmd = fake, concurrency = opts.concurrency })
  return log, root
end

-- open opens a view and waits until it is complete.
local function open(kind)
  local views = require("ai-dev.views")
  views.open(kind)
  local buf = vim.api.nvim_get_current_buf()
  h.wait_for(function()
    return views.state(buf) == "complete"
  end, kind .. " view")
  return buf
end

local function guide_calls(log)
  local ids = {}
  for _, call in ipairs(h.fake_calls(log)) do
    if call[1] == "workflow" then
      h.eq({ "workflow", "guide", "--format", "json", "--" }, vim.list_slice(call, 1, 5))
      ids[#ids + 1] = call[6]
    end
  end
  return ids
end

local function dash(v)
  return v == nil and "-" or v
end

-- row is the features row of contracts/plugin.md for one guide report.
local function row(g)
  local status = (g.outcome == "step_recommended" or g.outcome == "workflow_terminal") and g.current_state or g.outcome
  local agent = dash(g.agent)
  if g.agent_availability or g.source then
    agent = ("%s (%s, %s)"):format(agent, dash(g.agent_availability), dash(g.source))
  end
  return table.concat({ g.feature_id, status, dash(g.activity), dash(g.role), agent }, "  ")
end

local feature_ids = {
  "01-draft", "02-specified", "03-planned", "04-tasked", "05-implemented",
  "06-approved", "07-abandoned", "08-invalid", "09-inconsistent", "10-" .. ESC .. "[31mred",
}

function T.features_lists_feature_directories_and_reads_each_guide()
  local log, root = start("views", { concurrency = 3 })
  vim.uv.fs_symlink(root .. "/specs/01-draft", root .. "/specs/50-link")
  h.write(root .. "/specs/README.md", "not a feature\n")
  local views, cli = require("ai-dev.views"), require("ai-dev.cli")
  local max_active = 0
  views.open("features")
  local buf = vim.api.nvim_get_current_buf()
  h.wait_for(function()
    max_active = math.max(max_active, cli.active())
    return views.state(buf) == "complete"
  end)
  local called = guide_calls(log)
  table.sort(called) -- concurrent processes may log in any order
  h.eq(feature_ids, called, "one guide per directory, links and files excluded")
  h.truthy(max_active <= 3, "at most concurrency processes: " .. max_active)
end

function T.features_rows_show_the_guide_fields()
  start("views")
  local buf = open("features")
  local lines = h.buf_lines(buf)
  local names = {
    "guide-01-draft", "guide-02-specified", "guide-03-planned", "guide-04-tasked", "guide-05-implemented",
    "guide-06-approved", "guide-07-abandoned", "guide-08-invalid", "guide-09-inconsistent",
  }
  for i, name in ipairs(names) do
    h.eq(row(h.fixture(name)), lines[i + 1], name)
  end
  h.eq("02-specified  specified  plan  planning  gemini-cli (available, project)", lines[3])
  h.eq("04-tasked  tasked  implement  implementation  codex-cli (unavailable, project)", lines[5])
  h.eq("06-approved  approved  -  -  -", lines[7])
  h.eq("08-invalid  state_invalid  -  -  -", lines[9])
  h.eq("09-inconsistent  artifacts_inconsistent  -  -  -", lines[10])
  h.eq(11, #lines)
end

function T.features_row_for_an_unknown_feature_shows_its_outcome()
  local _, root = start("views")
  vim.fn.mkdir(root .. "/specs/99-removed", "p")
  local lines = h.buf_lines(open("features"))
  h.eq("99-removed  feature_unknown  -  -  -", lines[#lines])
end

function T.features_row_for_an_invalid_configuration()
  start("invalid-config")
  local lines = h.buf_lines(open("features"))
  h.eq("01-draft  draft  specify  specification  - (configuration_invalid, project)", lines[2])
end

function T.hostile_names_and_messages_are_shown_escaped()
  local doctor = vim.json.encode({
    overall = "not_ready",
    checks = { { id = "x" .. ESC .. "]52;c;aGk=\7", name = "N\226\128\174ame", status = "fail" } },
  })
  start("views", { extra = { { args = { "doctor", "--format", "json" }, stdout = doctor, code = 1 } } })
  local features = table.concat(h.buf_lines(open("features")), "\n")
  h.eq(nil, features:find(ESC, 1, true), "no escape byte in the features view")
  h.truthy(features:find("10-\\x1b[31mred  draft", 1, true), "escaped name")
  local checks = table.concat(h.buf_lines(open("doctor")), "\n")
  h.eq(nil, checks:find(ESC, 1, true), "no escape byte in the doctor view")
  h.eq(nil, checks:find("\7", 1, true), "no BEL in the doctor view")
  h.truthy(checks:find("x\\x1b]52;c;aGk=\\x07", 1, true), "escaped check id")
  h.truthy(checks:find("N\\u202eame", 1, true), "escaped bidi override")
end

function T.doctor_view_shows_overall_and_each_check()
  start("no-specs")
  local lines = h.buf_lines(open("doctor"))
  local report = h.fixture("doctor")
  h.eq("overall: " .. report.overall, lines[2])
  for i, check in ipairs(report.checks) do
    h.eq(("%s  %s  %s"):format(check.status, check.id, check.name), lines[i + 2])
  end
  h.eq(#report.checks + 2, #lines)
end

function T.agents_view_shows_agents_and_roles()
  start("views")
  local lines = h.buf_lines(open("agents"))
  local setup = h.fixture("setup-show")
  local want = { ("outcome: %s (source %s)"):format(setup.outcome, setup.source), "agents:" }
  for _, a in ipairs(setup.agents) do
    want[#want + 1] = ("  %s  %s  %s%s"):format(a.id, a.label, a.availability, a.chosen and "  chosen" or "")
  end
  want[#want + 1] = "roles:"
  for _, r in ipairs(setup.roles) do
    want[#want + 1] = ("  %s  %s"):format(r.role, r.agent)
  end
  for _, w in ipairs(setup.warnings) do
    want[#want + 1] = "warning: " .. w
  end
  h.eq(want, vim.list_slice(lines, 2))
end

function T.agents_view_shows_an_invalid_configuration()
  start("invalid-config")
  local lines = h.buf_lines(open("agents"))
  h.eq("outcome: configuration_invalid (source -)", lines[2])
  h.eq("reason: malformed_json", lines[#lines])
end

function T.first_line_shows_the_evaluation_time_and_r_refreshes()
  local log = start("views")
  local views = require("ai-dev.views")
  views.clock = function()
    return "2026-11-24 10:00:00"
  end
  local buf = open("features")
  h.eq("ai-dev features  (evaluated 2026-11-24 10:00:00)", h.buf_lines(buf)[1])
  local before = #guide_calls(log)
  views.clock = function()
    return "2026-11-24 10:05:00"
  end
  vim.api.nvim_feedkeys("r", "x", false)
  h.wait_for(function()
    return views.state(buf) == "complete" and h.buf_lines(buf)[1]:find("10:05:00", 1, true) ~= nil
  end, "refresh")
  h.eq(before * 2, #guide_calls(log), "refresh re-runs every command")
end

function T.views_are_read_only_scratch_buffers_with_local_keys()
  start("views")
  local global = vim.api.nvim_get_keymap("n")
  local other = vim.api.nvim_get_current_buf()
  local buf = open("features")
  h.eq("nofile", vim.bo[buf].buftype)
  h.eq(false, vim.bo[buf].modifiable)
  h.eq("wipe", vim.bo[buf].bufhidden)
  local keys = {}
  for _, map in ipairs(vim.api.nvim_buf_get_keymap(buf, "n")) do
    keys[#keys + 1] = map.lhs
  end
  table.sort(keys)
  h.eq({ "<CR>", "a", "n", "q", "r", "x" }, keys)
  h.eq({}, vim.api.nvim_buf_get_keymap(other, "n"), "no key in another buffer")
  h.eq(global, vim.api.nvim_get_keymap("n"), "no global key (owner decision 1)")
  vim.api.nvim_feedkeys("q", "x", false)
  h.eq(false, vim.api.nvim_buf_is_valid(buf), "q closes the view")
end

local function assert_incompatible(opts, project, kind, pattern)
  local log = start(project or "views", opts)
  local asked = h.stub_ui({})
  local confirms = h.stub_confirm(1)
  local buf = open(kind or "features")
  local text = table.concat(h.buf_lines(buf), "\n")
  h.match(pattern, text, "message")
  local calls = #h.fake_calls(log)
  vim.api.nvim_win_set_cursor(0, { #h.buf_lines(buf), 0 })
  for _, key in ipairs({ "\r", "n", "a", "x" }) do
    vim.api.nvim_feedkeys(key, "x", false)
  end
  h.eq(calls, #h.fake_calls(log), "no action key runs ai-dev")
  h.eq({}, asked, "no action key asks anything")
  h.eq({}, confirms)
end

function T.incompatible_version_is_shown_and_disables_actions()
  assert_incompatible({ version = "v0.2.1" }, nil, nil, "v0%.2%.1 is not supported; supported: >= v0%.3%.0, < v1%.0%.0")
end

function T.unknown_schema_is_shown()
  local v2 = '{"schema_version":"ai-dev.workflow-guide/v2","feature_id":"01-draft","outcome":"step_recommended"}'
  assert_incompatible(
    { extra = { { args = { "workflow", "guide", "--format", "json", "--", "01-draft" }, stdout = v2 } } },
    nil,
    nil,
    "ai%-dev%.workflow%-guide/v2; supported: ai%-dev%.workflow%-guide/v1"
  )
end

function T.wrong_doctor_shape_is_shown()
  assert_incompatible(
    { extra = { { args = { "doctor", "--format", "json" }, stdout = '{"overall":"ready"}' } } },
    "no-specs",
    "doctor",
    "unsupported ai%-dev doctor output"
  )
end

function T.non_json_output_is_shown()
  assert_incompatible(
    { extra = { { args = { "setup", "show", "--format", "json" }, stdout = "Error: boom\n", code = 2 } } },
    nil,
    "agents",
    "did not print JSON"
  )
end

function T.missing_ai_dev_is_shown()
  h.fixture_project("views")
  require("ai-dev").setup({ cmd = "/nonexistent/ai-dev" })
  local buf = open("features")
  h.match("not found", table.concat(h.buf_lines(buf), "\n"))
end

function T.a_project_without_specs_says_so()
  start("no-specs")
  local lines = h.buf_lines(open("features"))
  h.eq("no specs/ directory in the working directory", lines[2])
end

-- enter_on_feature opens the features view and presses <CR> on id's row.
local function enter_on_feature(id, answers)
  local buf = open("features")
  local asked = h.stub_ui(answers)
  for lnum, line in ipairs(h.buf_lines(buf)) do
    if vim.startswith(line, id .. "  ") then
      vim.api.nvim_win_set_cursor(0, { lnum, 0 })
    end
  end
  vim.api.nvim_feedkeys("\r", "x", false)
  return buf, asked
end

function T.enter_lists_the_feature_files_and_opens_the_chosen_one_literally()
  local _, root = start("views")
  local dir = root .. "/specs/03-planned"
  local hostile = "-a %b #c `touch x` $HOME.md"
  h.write(dir .. "/" .. hostile, "hostile\n")
  h.write(root .. "/outside.md", "outside\n")
  vim.uv.fs_symlink(root .. "/outside.md", dir .. "/link.md")
  vim.fn.mkdir(dir .. "/sub", "p")
  local view, asked = enter_on_feature("03-planned", { 1 })
  h.eq(1, #asked)
  h.eq("select", asked[1].kind)
  h.eq({ hostile, "plan.md", "spec.md", "workflow-state.json" }, asked[1].items, "regular files, sorted")
  h.eq(1, #vim.api.nvim_list_wins(), "the current window")
  local name = vim.api.nvim_buf_get_name(0)
  h.eq(dir .. "/" .. hostile, vim.uv.fs_realpath(name) or name, "opened literally")
  h.eq({ "hostile" }, h.buf_lines(0))
  h.eq(nil, vim.uv.fs_stat(root .. "/x"), "backquotes not run")
  h.eq(false, vim.api.nvim_buf_is_valid(view), "the view is replaced")
end

function T.enter_with_a_cancelled_selection_opens_nothing()
  start("views")
  local view, asked = enter_on_feature("02-specified", { nil })
  h.eq(1, #asked)
  h.eq(view, vim.api.nvim_get_current_buf())
end

function T.no_command_runs_again_without_a_keypress()
  local log = start("views")
  open("features")
  local calls = #h.fake_calls(log)
  vim.wait(2000)
  h.eq(calls, #h.fake_calls(log), "no background polling (F014-FR-008)")
end

return T
