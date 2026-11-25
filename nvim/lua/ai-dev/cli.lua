-- Running ai-dev and reading its JSON (Feature 014, F014-FR-010, research
-- R-09). Every call is an argument vector given to vim.system: no shell.
local M = {}

M.supported = ">= v0.3.0, < v1.0.0"
local minimum, limit = { 0, 3, 0 }, { 1, 0, 0 }

-- timeout is how long, in milliseconds, one ai-dev read may take before it
-- is stopped and reported, so a hung command cannot hold a slot forever.
M.timeout = 30000

local active, queue = 0, {}

-- executable returns the configured ai-dev resolved on PATH, or as given.
function M.executable(cfg)
  local path = vim.fn.exepath(cfg.cmd)
  return path ~= "" and path or cfg.cmd
end

-- active returns the number of ai-dev processes running.
function M.active()
  return active
end

local function start_next()
  while active < (queue[1] and queue[1].limit or 0) do
    local job = table.remove(queue, 1)
    active = active + 1
    local done, timer = false, nil
    -- finish reports the first of exit and timeout; the other is ignored.
    local function finish(result)
      if done then
        return
      end
      done = true
      if timer then
        timer:close()
      end
      active = active - 1
      vim.schedule(function()
        job.cb(result)
        start_next()
      end)
    end
    local ok, proc = pcall(vim.system, job.argv, { cwd = job.cwd, text = true }, function(out)
      finish({ code = out.code, stdout = out.stdout or "", stderr = out.stderr or "" })
    end)
    if not ok then
      finish({ stdout = "", stderr = "", error = ("%s not found or not executable: %s"):format(job.argv[1], proc) })
    else
      -- The timer, not vim.system's timeout, releases the slot: a child
      -- that keeps the output open would otherwise delay the exit.
      local timeout = M.timeout
      timer = vim.defer_fn(function()
        timer = nil -- defer_fn closes its own timer once it fires
        pcall(proc.kill, proc, "sigkill")
        finish({ stdout = "", stderr = "", error = ("ai-dev did not answer within %g s and was stopped"):format(timeout / 1000) })
      end, timeout)
    end
  end
end

-- run starts cfg.cmd with args in Neovim's working directory and returns at
-- once; cb receives {code, stdout, stderr} on the main loop, or {error} when
-- the executable cannot start. At most cfg.concurrency run at once.
function M.run(cfg, args, cb)
  local argv = { M.executable(cfg) }
  vim.list_extend(argv, args)
  queue[#queue + 1] = { argv = argv, cwd = vim.fn.getcwd(), cb = cb, limit = cfg.concurrency }
  start_next()
end

-- decode parses one JSON object. With schema, its schema_version must equal
-- schema. It returns the table, or nil and a message naming what was found.
function M.decode(stdout, schema)
  local ok, value = pcall(vim.json.decode, stdout or "", { luanil = { object = true, array = true } })
  if not ok then
    return nil, "ai-dev did not print JSON"
  end
  if type(value) ~= "table" or (vim.islist(value) and #value > 0) then
    return nil, "ai-dev did not print a JSON object"
  end
  if schema then
    local found = value.schema_version
    if found == nil then
      return nil, ("ai-dev output has no schema_version; supported: %s"):format(schema)
    end
    if found ~= schema then
      return nil, ("unsupported ai-dev output schema %s; supported: %s"):format(tostring(found), schema)
    end
  end
  return value
end

local function strings(t, fields)
  if type(t) ~= "table" then
    return false
  end
  for _, f in ipairs(fields) do
    if type(t[f]) ~= "string" then
      return false
    end
  end
  return true
end

local function list_of(t, fields)
  if type(t) ~= "table" or not vim.islist(t) then
    return false
  end
  for _, item in ipairs(t) do
    if not strings(item, fields) then
      return false
    end
  end
  return true
end

local shapes = {
  doctor = function(t)
    return strings(t, { "overall" }) and list_of(t.checks, { "id", "name", "status" })
  end,
  agents = function(t)
    return list_of(t.agents, { "id", "display_label", "availability" })
  end,
  agent_select = function(t)
    return strings(t, { "outcome", "source" })
      and (t.outcome ~= "role_selected" or type(t.selected_agent_id) == "string")
  end,
}

local described = {
  doctor = "overall and checks[] with id, name and status",
  agents = "agents[] with id, display_label and availability",
  agent_select = "outcome, selected_agent_id and source",
}

-- check_shape checks the outputs that carry no schema_version (R-09).
function M.check_shape(kind, t)
  if shapes[kind](t) then
    return true
  end
  return false, ("unsupported ai-dev %s output; expected %s"):format(kind, described[kind])
end

local function less(a, b)
  for i = 1, 3 do
    if a[i] ~= b[i] then
      return a[i] < b[i]
    end
  end
  return false
end

-- compatibility judges a version string: a release from v0.3.0 to below
-- v1.0.0, or a development build (a Go pseudo-version or -dev).
function M.compatibility(version)
  local major, minor, patch, rest = tostring(version):match("^v(%d+)%.(%d+)%.(%d+)(.*)$")
  if not major then
    return { compatible = false, version = version, dev = false, message = ("unrecognized ai-dev version %s; supported: %s"):format(tostring(version), M.supported) }
  end
  local pseudo = "%d%d%d%d%d%d%d%d%d%d%d%d%d%d%-" .. ("%x"):rep(12)
  local dev = rest:find("^%-dev") ~= nil or rest:find(pseudo) ~= nil
  if dev then
    return { compatible = true, version = version, dev = true }
  end
  local v = { tonumber(major), tonumber(minor), tonumber(patch) }
  if less(v, minimum) or not less(v, limit) then
    return { compatible = false, version = version, dev = false, message = ("ai-dev %s is not supported; supported: %s"):format(version, M.supported) }
  end
  return { compatible = true, version = version, dev = false }
end

local compatible = {}

-- ensure_compatible is check_version run once per executable: a compatible
-- result is kept until reset(), an incompatible one is checked again.
function M.ensure_compatible(cfg, cb)
  local exe = M.executable(cfg)
  if compatible[exe] then
    return cb(compatible[exe])
  end
  M.check_version(cfg, function(r)
    if r.compatible then
      compatible[exe] = r
    end
    cb(r)
  end)
end

-- reset forgets the kept compatibility results.
function M.reset()
  compatible = {}
end

-- version_result judges the result of `version --format json`.
local function version_result(r)
  if r.error then
    return { compatible = false, dev = false, message = r.error }
  end
  if r.code ~= 0 then
    return { compatible = false, dev = false, message = ("ai-dev version failed with exit code %d"):format(r.code) }
  end
  local t, msg = M.decode(r.stdout, "ai-dev.version/v1")
  if not t then
    return { compatible = false, dev = false, message = msg }
  end
  return M.compatibility(t.version)
end

-- check_version runs `version --format json`; cb receives the result of
-- compatibility, or {compatible = false, message} when it cannot be read.
function M.check_version(cfg, cb)
  M.run(cfg, { "version", "--format", "json" }, function(r)
    cb(version_result(r))
  end)
end

-- check_version_now is check_version waiting for the answer, for the
-- health check, which Neovim runs synchronously.
function M.check_version_now(cfg)
  local argv = { M.executable(cfg), "version", "--format", "json" }
  local ok, proc = pcall(vim.system, argv, { cwd = vim.fn.getcwd(), text = true })
  if not ok then
    return version_result({ error = ("%s not found or not executable: %s"):format(argv[1], proc) })
  end
  local out = proc:wait(10000)
  return version_result({ code = out.code, stdout = out.stdout or "" })
end

return M
