local h = require("helpers")

local T = {}

local function cfg(cmd, concurrency)
  return { cmd = cmd, concurrency = concurrency or 8, terminal = "tab", preset = false }
end

-- run calls cli.run and waits for its callback.
local function run(c, args)
  local result
  require("ai-dev.cli").run(c, args, function(r)
    result = r
  end)
  h.truthy(vim.wait(5000, function()
    return result ~= nil
  end, 10), "callback ran")
  return result
end

local function version_of(stdout, code)
  local dir = h.tmpdir()
  h.write(dir .. "/out.json", stdout)
  local cmd = h.fake_ai_dev(dir, ("cat %q; exit %d"):format(dir .. "/out.json", code or 0))
  local result
  require("ai-dev.cli").check_version(cfg(cmd), function(r)
    result = r
  end)
  h.truthy(vim.wait(5000, function()
    return result ~= nil
  end, 10), "check_version callback")
  return result
end

function T.run_passes_an_argument_vector_and_the_working_directory()
  local dir = h.tmpdir()
  local project = h.tmpdir()
  local cmd, log = h.fake_ai_dev(dir, 'pwd; echo "err" >&2; exit 3')
  vim.cmd.cd(vim.fn.fnameescape(project))
  local args = { "workflow", "guide", "--format", "json", "--", "a b; $(x) `y` 'z'\n-w" }
  local r = run(cfg(cmd), args)
  h.eq(3, r.code)
  h.eq(project .. "\n", r.stdout)
  h.eq("err\n", r.stderr)
  h.eq({ args }, h.fake_calls(log))
end

function T.run_reports_a_missing_executable()
  local r = run(cfg("/nonexistent/ai-dev"), { "version" })
  h.eq(nil, r.code)
  h.match("not found", r.error)
end

function T.run_returns_before_the_command_ends_and_calls_back_on_the_main_loop()
  local dir = h.tmpdir()
  local cmd = h.fake_ai_dev(dir, "sleep 0.3; echo done")
  local start = vim.uv.hrtime()
  local result, in_fast
  require("ai-dev.cli").run(cfg(cmd), { "x" }, function(r)
    in_fast = vim.in_fast_event()
    result = r
  end)
  local returned_ms = (vim.uv.hrtime() - start) / 1e6
  h.truthy(returned_ms < 100, ("run returned after %.0f ms"):format(returned_ms))
  h.eq(nil, result, "no result before the command ends")
  h.truthy(vim.wait(5000, function()
    return result ~= nil
  end, 10))
  h.eq("done\n", result.stdout)
  h.eq(false, in_fast, "callback outside a fast event")
end

function T.run_limits_the_processes_running_at_once()
  local dir = h.tmpdir()
  local cmd = h.fake_ai_dev(dir, "sleep 0.2")
  local cli = require("ai-dev.cli")
  local done, max_active = 0, 0
  for _ = 1, 5 do
    cli.run(cfg(cmd, 2), { "x" }, function()
      done = done + 1
    end)
  end
  h.truthy(vim.wait(5000, function()
    max_active = math.max(max_active, cli.active())
    return done == 5
  end, 5))
  h.eq(2, max_active, "processes at once")
  h.eq(0, cli.active())
end

function T.release_versions_in_range_are_compatible()
  for _, v in ipairs({ "v0.3.0", "v0.9.4" }) do
    local r = version_of(('{"schema_version":"ai-dev.version/v1","version":"%s"}'):format(v))
    h.eq(true, r.compatible, v)
    h.eq(v, r.version)
    h.eq(false, r.dev)
  end
end

function T.release_versions_out_of_range_are_incompatible()
  for _, v in ipairs({ "v0.2.1", "v1.0.0" }) do
    local r = version_of(('{"schema_version":"ai-dev.version/v1","version":"%s"}'):format(v))
    h.eq(false, r.compatible, v)
    h.match(v, r.message)
    h.match(">= v0.3.0, < v1.0.0", r.message)
  end
end

function T.development_builds_are_compatible()
  for _, v in ipairs({ "v0.3.1-dev+abc", "v0.0.0-20261124232317-e2d5f8628584+dirty", "v0.0.0-20261124232317-e2d5f8628584" }) do
    local r = version_of(('{"schema_version":"ai-dev.version/v1","version":"%s"}'):format(v))
    h.eq(true, r.compatible, v)
    h.eq(true, r.dev, v)
  end
end

function T.unusable_version_reports_are_incompatible()
  local cases = {
    { '{"schema_version":"ai-dev.version/v2","version":"v0.3.0"}', 0, "ai%-dev.version/v2" },
    { "not json", 0, "JSON" },
    { '{"schema_version":"ai-dev.version/v1","version":"v0.3.0"}', 2, "exit" },
    { '{"schema_version":"ai-dev.version/v1","version":"latest"}', 0, "latest" },
  }
  for _, case in ipairs(cases) do
    local r = version_of(case[1], case[2])
    h.eq(false, r.compatible, case[1])
    h.match(case[3], r.message, case[1])
  end
  local r
  require("ai-dev.cli").check_version(cfg("/nonexistent/ai-dev"), function(x)
    r = x
  end)
  vim.wait(5000, function()
    return r ~= nil
  end, 10)
  h.eq(false, r.compatible)
  h.match("not found", r.message)
end

function T.decode_checks_the_schema()
  local cli = require("ai-dev.cli")
  local t = cli.decode('{"schema_version":"ai-dev.setup/v1","outcome":"configured"}', "ai-dev.setup/v1")
  h.eq("configured", t.outcome)

  local value, msg = cli.decode('{"schema_version":"ai-dev.workflow-guide/v2"}', "ai-dev.workflow-guide/v1")
  h.eq(nil, value)
  h.match("ai%-dev.workflow%-guide/v2", msg)
  h.match("ai%-dev.workflow%-guide/v1", msg)

  value, msg = cli.decode("{}", "ai-dev.setup/v1")
  h.eq(nil, value)
  h.match("no schema_version", msg)
  h.match("ai%-dev.setup/v1", msg)

  value, msg = cli.decode("[1", "ai-dev.setup/v1")
  h.eq(nil, value)
  h.match("JSON", msg)

  value, msg = cli.decode("[1]", nil)
  h.eq(nil, value)
  h.match("JSON object", msg)
end

function T.check_shape_accepts_known_shapes()
  local cli = require("ai-dev.cli")
  h.truthy(cli.check_shape("doctor", { overall = "ready", checks = { { id = "git", name = "Git", status = "pass" } } }))
  h.truthy(cli.check_shape("doctor", { overall = "ready", checks = {} }))
  h.truthy(cli.check_shape("agents", { agents = { { id = "a", display_label = "A", availability = "available" } } }))
  h.truthy(cli.check_shape("agent_select", { outcome = "role_selected", selected_agent_id = "a", source = "built_in" }))
  h.truthy(cli.check_shape("agent_select", { outcome = "role_unsupported", source = "built_in" }))
end

function T.check_shape_refuses_missing_fields()
  local cli = require("ai-dev.cli")
  local bad = {
    { "doctor", { checks = {} } },
    { "doctor", { overall = "ready" } },
    { "doctor", { overall = "ready", checks = { { name = "Git", status = "pass" } } } },
    { "doctor", { overall = "ready", checks = { { id = "git", status = "pass" } } } },
    { "doctor", { overall = "ready", checks = { { id = "git", name = "Git" } } } },
    { "agents", {} },
    { "agents", { agents = { { display_label = "A", availability = "available" } } } },
    { "agents", { agents = { { id = "a", availability = "available" } } } },
    { "agents", { agents = { { id = "a", display_label = "A" } } } },
    { "agent_select", { selected_agent_id = "a", source = "built_in" } },
    { "agent_select", { outcome = "role_selected", selected_agent_id = "a" } },
    { "agent_select", { outcome = "role_selected", source = "built_in" } },
  }
  for _, case in ipairs(bad) do
    local ok, msg = cli.check_shape(case[1], case[2])
    h.eq(false, ok, case[1] .. " " .. vim.inspect(case[2]))
    h.match(case[1], msg)
  end
end

return T
