-- Actions that run an ai-dev command in a Neovim terminal, each only after
-- its values are collected with nothing prefilled, a preview of the exact
-- command, and an explicit confirmation (Feature 014, F014-FR-012 to
-- FR-017, FR-024, data-model.md Plugin Action). The plugin decides nothing:
-- the command records any decision and reports its own outcome.
local cli = require("ai-dev.cli")
local quote = require("ai-dev.quote")
local reference = require("ai-dev.reference")
local sanitize = require("ai-dev.sanitize").text

local M = {}

-- states holds each action's state while it is active: "collecting",
-- "previewing" or "running". An action that is not active has none.
local states = {}

M.roles = { "specification", "planning", "implementation", "review", "remediation" }

local function cfg()
  return require("ai-dev").config()
end

local function notify(msg, level)
  vim.notify("ai-dev: " .. msg, level or vim.log.levels.INFO)
end

-- state returns the state of the action named name, or nil.
function M.state(name)
  return states[name]
end

local preview_name = "ai-dev://preview"

local function close_preview()
  local buf = vim.fn.bufnr("^" .. preview_name .. "$")
  if buf ~= -1 then
    pcall(vim.api.nvim_buf_delete, buf, { force = true })
  end
end

-- show_preview opens a floating window holding lines; q closes it.
local function show_preview(lines)
  close_preview()
  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_name(buf, preview_name)
  vim.bo[buf].bufhidden = "wipe"
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false
  local width = 20
  for _, line in ipairs(lines) do
    width = math.max(width, vim.fn.strdisplaywidth(line))
  end
  width = math.min(width, math.max(20, vim.o.columns - 4))
  local height = math.min(#lines, math.max(1, vim.o.lines - 6))
  vim.api.nvim_open_win(buf, false, {
    relative = "editor",
    row = math.max(0, math.floor((vim.o.lines - height) / 2) - 1),
    col = math.max(0, math.floor((vim.o.columns - width) / 2)),
    width = width,
    height = height,
    style = "minimal",
    border = "rounded",
    title = " ai-dev ",
  })
  vim.keymap.set("n", "q", close_preview, { buffer = buf, nowait = true })
end

-- preview_lines renders the preview of an action: for an agent run, the
-- agent, the reference, whether text is included and the complete
-- instruction; then the quoted command line. Everything is sanitized.
local function preview_lines(a)
  local lines = { "ai-dev " .. a.name }
  if a.agent then
    lines[#lines + 1] = "Agent: " .. sanitize(a.agent)
  end
  if a.ref then
    local where = a.ref.range and (", lines %d-%d"):format(a.ref.range[1], a.ref.range[2]) or " (whole file)"
    lines[#lines + 1] = "Reference: " .. sanitize(a.ref.path) .. where
    lines[#lines + 1] = "Digest: sha256 " .. a.ref.digest
    lines[#lines + 1] = "Selected text: " .. (a.include_text and "included" or "not included")
  end
  if a.instruction then
    lines[#lines + 1] = "Instruction:"
    for line in vim.gsplit(a.instruction, "\n", { plain = true }) do
      lines[#lines + 1] = "  " .. sanitize(line)
    end
  end
  for _, extra in ipairs(a.details or {}) do
    lines[#lines + 1] = extra
  end
  lines[#lines + 1] = "Command:"
  lines[#lines + 1] = "  " .. sanitize(quote.argv(a.argv))
  return lines
end

local function open_terminal()
  local where = cfg().terminal
  if where == "split" then
    vim.cmd.new()
  elseif where == "vsplit" then
    vim.cmd.vnew()
  else
    vim.cmd.tabnew()
  end
end

-- run starts a.argv in a new terminal; on exit it shows the exit code and
-- refreshes the open views (F014-FR-013, FR-016).
local function run(a)
  states[a.name] = "running"
  open_terminal()
  local ok, job = pcall(vim.fn.jobstart, a.argv, {
    term = true,
    cwd = vim.fn.getcwd(),
    on_exit = function(_, code)
      states[a.name] = nil
      local level = code == 0 and vim.log.levels.INFO or vim.log.levels.WARN
      vim.notify(("ai-dev exited with code %d"):format(code), level)
      local views = package.loaded["ai-dev.views"]
      if views then
        views.refresh_all()
      end
    end,
  })
  if not ok or job <= 0 then
    states[a.name] = nil
    notify("cannot start " .. sanitize(a.argv[1]), vim.log.levels.ERROR)
  end
end

-- confirm shows the preview and asks; only Run continues. A reference is
-- checked again first, and a changed file runs nothing (F014-FR-015).
local function confirm(a)
  states[a.name] = "previewing"
  show_preview(preview_lines(a))
  local choice = vim.fn.confirm("Run this command?", "&Run\n&Cancel", 2)
  close_preview()
  if choice ~= 1 then
    states[a.name] = nil
    notify("nothing was run")
    return
  end
  if a.ref then
    local unchanged = a.ref:verify()
    if not unchanged then
      states[a.name] = nil
      local lines = preview_lines(a)
      table.insert(lines, 1, "Refused (changed): the file changed on disk since this preview. Nothing was run.")
      show_preview(lines)
      notify("changed: " .. sanitize(a.ref.path) .. " changed on disk since the preview; nothing was run", vim.log.levels.WARN)
      return
    end
  end
  run(a)
end

-- begin marks name as collecting, or refuses when it is already active. It
-- then checks that ai-dev is compatible before calling next.
local function begin(name, next)
  if states[name] then
    notify(("the %s action is already running"):format(name), vim.log.levels.WARN)
    return
  end
  states[name] = "collecting"
  cli.ensure_compatible(cfg(), function(c)
    if not c.compatible then
      states[name] = nil
      notify(sanitize(c.message), vim.log.levels.ERROR)
      return
    end
    next()
  end)
end

local function stop(name, msg)
  states[name] = nil
  if msg then
    notify(msg, vim.log.levels.WARN)
  end
end

-- choose_agent asks for the agent by role or from the catalog; cb receives
-- the agent id.
local function choose_agent(name, cb)
  vim.ui.select({ "by role", "from the catalog" }, { prompt = "Choose the agent" }, function(_, mode)
    if mode == 1 then
      vim.ui.select(M.roles, { prompt = "Role" }, function(role)
        if not role then
          return stop(name)
        end
        cli.run(cfg(), { "agent", "select", "--format", "json", "--", role }, function(r)
          local t, msg = cli.decode(r.stdout, nil)
          if t then
            local ok, why = cli.check_shape("agent_select", t)
            if not ok then
              t, msg = nil, why
            end
          end
          if not t then
            return stop(name, sanitize(r.error or msg))
          end
          if t.outcome ~= "role_selected" then
            return stop(name, ("no agent for the role %s (%s)"):format(role, sanitize(t.outcome)))
          end
          cb(t.selected_agent_id)
        end)
      end)
    elseif mode == 2 then
      cli.run(cfg(), { "agents", "--format", "json" }, function(r)
        local t, msg = cli.decode(r.stdout, nil)
        if t then
          local ok, why = cli.check_shape("agents", t)
          if not ok then
            t, msg = nil, why
          end
        end
        if not t then
          return stop(name, sanitize(r.error or msg))
        end
        vim.ui.select(t.agents, {
          prompt = "Agent",
          format_item = function(a)
            return ("%s  %s  %s"):format(sanitize(a.id), sanitize(a.display_label), sanitize(a.availability))
          end,
        }, function(agent)
          if not agent then
            return stop(name)
          end
          cb(agent.id)
        end)
      end)
    else
      stop(name)
    end
  end)
end

-- ask runs an agent with an instruction and a reference to the current
-- file, or to the lines of range (F014-FR-011, FR-013).
function M.ask(range)
  local ref, why = reference.from_buffer(0, range)
  if not ref then
    notify(("cannot refer to this buffer (%s)"):format(why), vim.log.levels.WARN)
    return
  end
  begin("ask", function()
    choose_agent("ask", function(agent)
      vim.ui.input({ prompt = "Instruction: " }, function(instruction)
        if instruction == nil then
          return stop("ask")
        end
        if instruction == "" then
          return stop("ask", "the instruction is required")
        end
        local function finish(include_text)
          local text, refusal = reference.compose(instruction, ref, include_text)
          if not text then
            return stop("ask", ("refused (%s)"):format(refusal))
          end
          confirm({
            name = "ask",
            agent = agent,
            ref = ref,
            include_text = include_text,
            instruction = text,
            argv = { cli.executable(cfg()), "agent", "run", "--", agent, text },
          })
        end
        if not range then
          return finish(false)
        end
        vim.ui.select(
          { "do not include the selected text", "include the selected text" },
          { prompt = "Selected text" },
          function(_, choice)
            if not choice then
              return stop("ask")
            end
            finish(choice == 2)
          end
        )
      end)
    end)
  end)
end

-- The cockpit's fields (internal/cockpit/actions.go): the same labels and
-- hints, nothing prefilled (F014-FR-012).
local state_hint = "draft, specified, planned, tasked, implemented, approved or abandoned"
local approver = { label = "approver", required = true, hint = "your name, recorded as declared" }
M.fields = {
  next = { { label = "instruction", required = true, hint = "passed to the agent as one argument" } },
  approve = { approver, { label = "target state", required = true, hint = state_hint }, { label = "reason", hint = "optional" } },
  abandon = { approver, { label = "reason", required = true } },
}

local changes = {
  next = "starts the agent; whatever the agent does, under its own sandbox",
  approve = "records your decision in workflow-state.json",
  abandon = "records your decision in workflow-state.json",
}

-- available applies F012-FR-011 to the guide report of a feature; it
-- returns false and the reason when the action cannot apply.
function M.available(name, guide)
  local terminal = guide.outcome == "workflow_terminal"
  if name == "next" then
    if terminal then
      return false, "the feature is in a terminal state"
    elseif guide.outcome ~= "step_recommended" then
      return false, ("no step is recommended (%s)"):format(sanitize(guide.outcome))
    elseif guide.agent == nil or guide.agent_availability ~= "available" then
      return false, ("the agent of the next step is %s"):format(sanitize(guide.agent_availability or "unknown"))
    end
  elseif guide.current_state == nil then
    return false, ("the feature has no recognized state (%s)"):format(sanitize(guide.outcome))
  elseif terminal then
    return false, "the feature is in a terminal state"
  end
  return true
end

-- collect asks each field in order; cb receives the values, or nothing
-- happens when the person cancels or leaves a required value empty.
local function collect(name, fields, cb)
  local values = {}
  local function ask_field(i)
    local field = fields[i]
    if not field then
      return cb(values)
    end
    local prompt = field.hint and ("%s (%s): "):format(field.label, field.hint) or (field.label .. ": ")
    vim.ui.input({ prompt = prompt }, function(value)
      if value == nil then
        return stop(name)
      end
      if field.required and value == "" then
        return stop(name, ("the %s is required"):format(field.label))
      end
      values[i] = value
      ask_field(i + 1)
    end)
  end
  ask_field(1)
end

local function argv_for(name, row, values)
  local exe = cli.executable(cfg())
  if name == "next" then
    return { exe, "agent", "run", "--", row.guide.agent, values[1] }
  elseif name == "approve" then
    local argv = { exe, "workflow", "approve", "--approver", values[1] }
    if values[3] ~= "" then
      vim.list_extend(argv, { "--reason", values[3] })
    end
    return vim.list_extend(argv, { "--", row.id, values[2] })
  end
  return { exe, "workflow", "abandon", "--approver", values[1], "--reason", values[2], "--", row.id, "abandoned" }
end

local function feature_action(name, row)
  local ok, why = M.available(name, row.guide)
  if not ok then
    return stop(name, ("%s is not available: %s"):format(name, why))
  end
  collect(name, M.fields[name], function(values)
    confirm({
      name = name,
      agent = name == "next" and row.guide.agent or nil,
      instruction = name == "next" and values[1] or nil,
      details = { "Feature: " .. sanitize(row.id), "Changes: " .. changes[name] },
      argv = argv_for(name, row, values),
    })
  end)
end

-- pick_feature asks for a feature from specs/, with nothing preselected,
-- and reads its guide; cb receives the row {id, guide}.
local function pick_feature(name, cb)
  local ids = {}
  local specs = vim.fn.getcwd() .. "/specs"
  local stat = vim.uv.fs_lstat(specs)
  if stat and stat.type == "directory" then
    for entry, type in vim.fs.dir(specs) do
      if type == "directory" then
        ids[#ids + 1] = entry
      end
    end
  end
  table.sort(ids)
  if #ids == 0 then
    return stop(name, "no features in specs/")
  end
  vim.ui.select(ids, { prompt = "Feature", format_item = sanitize }, function(id)
    if not id then
      return stop(name)
    end
    cli.run(cfg(), { "workflow", "guide", "--format", "json", "--", id }, function(r)
      local guide, msg = cli.decode(r.stdout, "ai-dev.workflow-guide/v1")
      if not guide then
        return stop(name, sanitize(r.error or msg))
      end
      cb({ id = id, guide = guide })
    end)
  end)
end

-- next, approve and abandon act on row ({id, guide} from the features
-- view), or, without one, on a feature the person picks (F014-FR-013).
for _, name in ipairs({ "next", "approve", "abandon" }) do
  M[name] = function(row)
    begin(name, function()
      if row then
        feature_action(name, row)
      else
        pick_feature(name, function(picked)
          feature_action(name, picked)
        end)
      end
    end)
  end
end

-- tui opens the cockpit in a Neovim terminal.
function M.tui()
  begin("tui", function()
    confirm({ name = "tui", argv = { cli.executable(cfg()), "tui" } })
  end)
end

-- reset forgets every action's state; teardown() calls it.
function M.reset()
  states = {}
  close_preview()
end

return M
