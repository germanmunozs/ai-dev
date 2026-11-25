-- Read-only views built only from ai-dev's JSON (Feature 014, US2,
-- contracts/plugin.md Views). Every string from ai-dev or from a file name
-- passes through sanitize.text before it is shown (F014-FR-023).
local cli = require("ai-dev.cli")
local sanitize = require("ai-dev.sanitize").text

local M = {}

-- clock returns the evaluation time shown on a view's first line.
M.clock = function()
  return os.date("%Y-%m-%d %H:%M:%S")
end

-- views holds each open view by buffer: kind, state ("loading" or
-- "complete"), generation, compatible, and rows (feature rows by line).
local views = {}

local titles = { features = "ai-dev features", doctor = "ai-dev doctor", agents = "ai-dev agents" }

local function cfg()
  return require("ai-dev").config()
end

local function dash(v)
  return v == nil and "-" or sanitize(v)
end

local function set_lines(buf, lines)
  if not vim.api.nvim_buf_is_valid(buf) then
    return
  end
  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false
end

-- finish shows the lines of a complete view, unless a refresh started since.
local function finish(buf, generation, lines, rows, compatible)
  local view = views[buf]
  if not view or view.generation ~= generation then
    return
  end
  view.state, view.rows, view.compatible = "complete", rows or {}, compatible
  table.insert(lines, 1, ("%s  (evaluated %s)"):format(titles[view.kind], M.clock()))
  set_lines(buf, lines)
end

local function feature_ids(root)
  local specs = root .. "/specs"
  local stat = vim.uv.fs_lstat(specs)
  if not stat or stat.type ~= "directory" then
    return nil
  end
  local ids = {}
  for name, type in vim.fs.dir(specs) do
    if type == "directory" then
      ids[#ids + 1] = name
    end
  end
  table.sort(ids)
  return ids
end

-- feature_row is `<id>  <current_state | outcome>  <activity>  <role>
-- <agent> (<availability>, <source>)`, with "-" for a missing field.
local function feature_row(g)
  local status = g.outcome
  if g.outcome == "step_recommended" or g.outcome == "workflow_terminal" then
    status = g.current_state
  end
  local agent = dash(g.agent)
  if g.agent_availability ~= nil or g.source ~= nil then
    agent = ("%s (%s, %s)"):format(agent, dash(g.agent_availability), dash(g.source))
  end
  return table.concat({ sanitize(g.feature_id), dash(status), dash(g.activity), dash(g.role), agent }, "  ")
end

-- read decodes one ai-dev result; it returns the table or nil and a message.
local function read(r, schema, shape)
  if r.error then
    return nil, r.error
  end
  local t, msg = cli.decode(r.stdout, schema)
  if t and shape then
    local ok, why = cli.check_shape(shape, t)
    if not ok then
      return nil, why
    end
  end
  if not t and r.code ~= 0 then
    msg = ("%s (exit code %d)"):format(msg, r.code)
  end
  return t, msg
end

local loaders = {}

function loaders.features(done)
  local ids = feature_ids(vim.fn.getcwd())
  if ids == nil then
    return done({ "no specs/ directory in the working directory" })
  end
  if #ids == 0 then
    return done({ "no features in specs/" })
  end
  local results, pending = {}, #ids
  for i, id in ipairs(ids) do
    cli.run(cfg(), { "workflow", "guide", "--format", "json", "--", id }, function(r)
      results[i] = r
      pending = pending - 1
      if pending > 0 then
        return
      end
      local lines, rows = {}, {}
      for j, result in ipairs(results) do
        local g, msg = read(result, "ai-dev.workflow-guide/v1")
        if not g then
          return done({ sanitize(msg) }, nil, false)
        end
        g.feature_id = g.feature_id or ids[j]
        lines[j] = feature_row(g)
        rows[j + 1] = { id = ids[j], guide = g }
      end
      done(lines, rows, true)
    end)
  end
end

function loaders.doctor(done)
  cli.run(cfg(), { "doctor", "--format", "json" }, function(r)
    local t, msg = read(r, nil, "doctor")
    if not t then
      return done({ sanitize(msg) }, nil, false)
    end
    local lines = { "overall: " .. sanitize(t.overall) }
    for _, check in ipairs(t.checks) do
      lines[#lines + 1] = ("%s  %s  %s"):format(sanitize(check.status), sanitize(check.id), sanitize(check.name))
    end
    done(lines, nil, true)
  end)
end

function loaders.agents(done)
  cli.run(cfg(), { "setup", "show", "--format", "json" }, function(r)
    local t, msg = read(r, "ai-dev.setup/v1")
    if not t then
      return done({ sanitize(msg) }, nil, false)
    end
    local lines = { ("outcome: %s (source %s)"):format(dash(t.outcome), dash(t.source)), "agents:" }
    for _, a in ipairs(t.agents or {}) do
      lines[#lines + 1] = ("  %s  %s  %s%s"):format(dash(a.id), dash(a.label), dash(a.availability), a.chosen and "  chosen" or "")
    end
    lines[#lines + 1] = "roles:"
    for _, role in ipairs(t.roles or {}) do
      lines[#lines + 1] = ("  %s  %s"):format(dash(role.role), dash(role.agent))
    end
    for _, w in ipairs(t.warnings or {}) do
      lines[#lines + 1] = "warning: " .. sanitize(w)
    end
    for _, reason in ipairs(t.reasons or {}) do
      lines[#lines + 1] = "reason: " .. dash(type(reason) == "table" and reason.code or reason)
    end
    done(lines, nil, true)
  end)
end

-- load runs the view's commands and shows the result; nothing runs again
-- until the person refreshes (F014-FR-008).
local function load(buf)
  local view = views[buf]
  view.generation = view.generation + 1
  view.state = "loading"
  local generation = view.generation
  local function done(lines, rows, compatible)
    finish(buf, generation, lines, rows, compatible)
  end
  set_lines(buf, { ("%s  (loading…)"):format(titles[view.kind]) })
  cli.ensure_compatible(cfg(), function(c)
    if not c.compatible then
      return done({ sanitize(c.message) }, nil, false)
    end
    loaders[view.kind](done)
  end)
end

-- feature_under_cursor returns the features row on the cursor's line.
function M.feature_under_cursor(buf)
  local view = views[buf or vim.api.nvim_get_current_buf()]
  if not view or not view.compatible then
    return nil
  end
  return view.rows[vim.api.nvim_win_get_cursor(0)[1]]
end

local function action_key(name)
  return function()
    local buf = vim.api.nvim_get_current_buf()
    local view = views[buf]
    if not view or not view.compatible or view.kind ~= "features" then
      return
    end
    local row = M.feature_under_cursor(buf)
    if row then
      require("ai-dev.actions")[name](row)
    end
  end
end

local function set_keys(buf)
  local function map(lhs, rhs, desc)
    vim.keymap.set("n", lhs, rhs, { buffer = buf, nowait = true, silent = true, desc = desc })
  end
  map("r", function()
    M.refresh(buf)
  end, "ai-dev: refresh")
  map("<CR>", function()
    M.open_artifact(buf)
  end, "ai-dev: open a feature file")
  map("n", action_key("next"), "ai-dev: next step")
  map("a", action_key("approve"), "ai-dev: approve")
  map("x", action_key("abandon"), "ai-dev: abandon")
  map("q", function()
    if vim.api.nvim_buf_is_valid(buf) then
      vim.api.nvim_buf_delete(buf, { force = true })
    end
  end, "ai-dev: close")
end

-- open shows the view of kind ("features", "doctor" or "agents") in the
-- current window, reusing an open one.
function M.open(kind)
  local name = "ai-dev://" .. kind
  local buf = vim.fn.bufnr("^" .. name .. "$")
  if buf == -1 or not views[buf] then
    buf = vim.api.nvim_create_buf(false, true)
    vim.api.nvim_buf_set_name(buf, name)
    vim.bo[buf].buftype = "nofile"
    vim.bo[buf].bufhidden = "wipe"
    vim.bo[buf].swapfile = false
    vim.bo[buf].modifiable = false
    views[buf] = { kind = kind, generation = 0, rows = {} }
    set_keys(buf)
    vim.api.nvim_create_autocmd("BufWipeout", {
      group = require("ai-dev").group(),
      buffer = buf,
      callback = function()
        views[buf] = nil
      end,
    })
  end
  vim.api.nvim_win_set_buf(0, buf)
  load(buf)
end

-- state returns "loading" or "complete" for an open view, or nil.
function M.state(buf)
  local view = views[buf]
  return view and view.state
end

-- refresh runs the view's commands again.
function M.refresh(buf)
  if views[buf] and vim.api.nvim_buf_is_valid(buf) then
    load(buf)
  end
end

-- refresh_all refreshes every open view, after an action ends.
function M.refresh_all()
  for buf in pairs(views) do
    M.refresh(buf)
  end
end

-- open_artifact lists the regular files of the feature under the cursor
-- and opens the chosen one (F014-FR-009).
function M.open_artifact(buf)
  local view = views[buf]
  if not view or view.kind ~= "features" then
    return
  end
  local row = M.feature_under_cursor(buf)
  if not row then
    return
  end
  local dir = "specs/" .. row.id
  local files = {}
  for name, type in vim.fs.dir(vim.fn.getcwd() .. "/" .. dir) do
    if type == "file" then
      files[#files + 1] = name
    end
  end
  table.sort(files)
  if #files == 0 then
    vim.notify("ai-dev: no files in this feature", vim.log.levels.INFO)
    return
  end
  vim.ui.select(files, { prompt = "Open a file of " .. sanitize(row.id), format_item = sanitize }, function(choice)
    if choice then
      vim.cmd.edit({ args = { dir .. "/" .. choice }, magic = { file = false, bar = false } })
    end
  end)
end

-- close_all wipes every open view; teardown() calls it.
function M.close_all()
  for buf in pairs(views) do
    if vim.api.nvim_buf_is_valid(buf) then
      pcall(vim.api.nvim_buf_delete, buf, { force = true })
    end
  end
  views = {}
end

return M
