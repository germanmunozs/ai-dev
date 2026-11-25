-- Assertions and fakes shared by the plugin tests (research R-08).
local M = {}

local function fail(msg, level)
  error(msg, (level or 1) + 2)
end

function M.eq(want, got, what)
  if not vim.deep_equal(want, got) then
    fail(("%sexpected %s, got %s"):format(what and (what .. ": ") or "", vim.inspect(want), vim.inspect(got)))
  end
end

function M.truthy(value, what)
  if not value then
    fail((what or "value") .. " is not truthy: " .. vim.inspect(value))
  end
end

function M.match(pattern, text, what)
  if type(text) ~= "string" or not text:find(pattern) then
    fail(("%s%s does not match %q"):format(what and (what .. ": ") or "", vim.inspect(text), pattern))
  end
end

-- tmpdir returns a new empty directory with its symbolic links resolved.
function M.tmpdir()
  local dir = vim.fn.tempname()
  vim.fn.mkdir(dir, "p")
  return vim.uv.fs_realpath(dir)
end

function M.write(path, content, mode)
  vim.fn.mkdir(vim.fn.fnamemodify(path, ":h"), "p")
  local f = assert(io.open(path, "wb"))
  f:write(content)
  f:close()
  if mode then
    vim.uv.fs_chmod(path, mode)
  end
end

function M.read(path)
  local f = io.open(path, "rb")
  if not f then
    return nil
  end
  local content = f:read("*a")
  f:close()
  return content
end

-- fake_ai_dev writes an executable /bin/sh script named ai-dev in dir. It
-- records each call in $AI_DEV_FAKE_LOG as CALL, then each argument, then
-- END, each followed by a NUL byte, and then runs script_body. It returns
-- the script's path and the log path, and sets $AI_DEV_FAKE_LOG.
function M.fake_ai_dev(dir, script_body)
  local path = dir .. "/ai-dev"
  local log = dir .. "/ai-dev.log"
  M.write(path, table.concat({
    "#!/bin/sh",
    -- One printf, so concurrent calls append whole records.
    "printf '%s\\0' CALL \"$@\" END >> \"$AI_DEV_FAKE_LOG\"",
    script_body or "",
    "",
  }, "\n"), 493) -- 0755
  M.write(log, "")
  vim.env.AI_DEV_FAKE_LOG = log
  return path, log
end

-- fake_calls returns the argument lists recorded by fake_ai_dev.
function M.fake_calls(log)
  local calls, current = {}, nil
  for field in vim.gsplit(M.read(log) or "", "\0", { plain = true }) do
    if current == nil then
      if field == "CALL" then
        current = {}
      end
    elseif field == "END" then
      calls[#calls + 1] = current
      current = nil
    else
      current[#current + 1] = field
    end
  end
  return calls
end

-- stub_ui answers vim.ui.input and vim.ui.select from answers, in order. An
-- input answer is a string (nil cancels); a select answer is the chosen
-- item's 1-based index (nil cancels). It returns the list of prompts asked,
-- each with the options and items it received.
function M.stub_ui(answers)
  local asked, i = {}, 0
  local function next_answer()
    i = i + 1
    return answers[i]
  end
  vim.ui.input = function(opts, on_confirm)
    asked[#asked + 1] = { kind = "input", opts = opts }
    on_confirm(next_answer())
  end
  vim.ui.select = function(items, opts, on_choice)
    asked[#asked + 1] = { kind = "select", opts = opts, items = items }
    local index = next_answer()
    on_choice(index and items[index] or nil, index)
  end
  return asked
end

-- preview_lines returns the lines of the plugin's open preview window, or
-- nil when none is open.
function M.preview_lines()
  for _, win in ipairs(vim.api.nvim_list_wins()) do
    local buf = vim.api.nvim_win_get_buf(win)
    if vim.api.nvim_buf_get_name(buf):find("ai%-dev://preview$") then
      return vim.api.nvim_buf_get_lines(buf, 0, -1, false)
    end
  end
end

-- stub_confirm makes vim.fn.confirm return choice (a number, or a list
-- answered in order) and records each call's arguments, with the preview
-- shown at that moment as .preview.
function M.stub_confirm(choice)
  local calls = {}
  vim.fn.confirm = function(...)
    calls[#calls + 1] = { args = { ... }, preview = M.preview_lines() }
    if type(choice) == "table" then
      return choice[#calls]
    end
    return choice
  end
  return calls
end

-- spy_jobstart wraps vim.fn.jobstart, recording each call's arguments; the
-- runner restores it. It returns the list of calls.
function M.spy_jobstart()
  local calls, real = {}, vim.fn.jobstart
  vim.fn.jobstart = function(cmd, opts)
    calls[#calls + 1] = { cmd = cmd, opts = opts }
    return real(cmd, opts)
  end
  return calls
end

-- capture_notify records every vim.notify call as {msg, level}.
function M.capture_notify()
  local notes = {}
  vim.notify = function(msg, level)
    notes[#notes + 1] = { msg = msg, level = level }
  end
  return notes
end

-- repo_root is the repository root (the parent of nvim/).
function M.repo_root()
  local here = debug.getinfo(1, "S").source:sub(2)
  return vim.fn.fnamemodify(vim.fn.fnamemodify(here, ":p"), ":h:h:h")
end

function M.fixtures_dir()
  return M.repo_root() .. "/nvim/tests/fixtures"
end

-- fixture returns the decoded output saved for a fixture of index.json.
function M.fixture(name)
  return vim.json.decode(M.read(M.fixtures_dir() .. "/" .. name .. ".json"), { luanil = { object = true } })
end

-- fixture_project creates the project of projects.json named name in a new
-- directory, makes it Neovim's working directory, and returns its path.
function M.fixture_project(name)
  local projects = vim.json.decode(M.read(M.fixtures_dir() .. "/projects.json"))
  local root = M.tmpdir()
  for path, content in pairs(projects[name]) do
    M.write(root .. "/" .. path, content)
  end
  vim.fn.mkdir(root, "p")
  vim.cmd.cd(vim.fn.fnameescape(root))
  return root
end

-- sh_quote quotes a value for the fake scripts written by these helpers;
-- the plugin itself never builds a shell string.
function M.sh_quote(value)
  return "'" .. value:gsub("'", "'\\''") .. "'"
end

-- fixture_ai_dev writes a fake ai-dev (see fake_ai_dev) that answers each
-- argument vector of fixtures/index.json for the project opts.project
-- (default "views") with its saved output and exit code, `version --format
-- json` with version (default v0.3.0), and the entries of opts.extra
-- ({args, stdout, code}), which come first. Any other call runs the shell
-- code opts.otherwise, by default an error and exit 2. opts.delay makes
-- each answer wait that many seconds. It returns the script's path and the
-- log path.
function M.fixture_ai_dev(dir, opts)
  opts = opts or {}
  local entries = {}
  for _, e in ipairs(opts.extra or {}) do
    local file = ("%s/extra-%d.out"):format(dir, #entries + 1)
    M.write(file, e.stdout or "")
    entries[#entries + 1] = { args = e.args, file = file, code = e.code or 0 }
  end
  local version = ('{"schema_version":"ai-dev.version/v1","version":"%s"}\n'):format(opts.version or "v0.3.0")
  M.write(dir .. "/version.out", version)
  entries[#entries + 1] = { args = { "version", "--format", "json" }, file = dir .. "/version.out", code = 0 }
  local index = vim.json.decode(M.read(M.fixtures_dir() .. "/index.json"))
  for _, e in ipairs(index) do
    if e.project == (opts.project or "views") then
      entries[#entries + 1] = { args = e.args, file = M.fixtures_dir() .. "/" .. e.name .. ".json", code = e.exit_code }
    end
  end
  local body = {}
  if opts.delay then
    body[#body + 1] = ("sleep %s"):format(opts.delay)
  end
  for _, e in ipairs(entries) do
    local test = { ('[ "$#" -eq %d ]'):format(#e.args) }
    for i, a in ipairs(e.args) do
      test[#test + 1] = ('[ "${%d}" = %s ]'):format(i, M.sh_quote(a))
    end
    body[#body + 1] = ("if %s; then cat %s; exit %d; fi"):format(table.concat(test, " && "), M.sh_quote(e.file), e.code)
  end
  body[#body + 1] = opts.otherwise or 'echo "fake ai-dev: no fixture for: $*" >&2; exit 2'
  return M.fake_ai_dev(dir, table.concat(body, "\n"))
end

-- buf_lines returns the lines of buf (default the current buffer).
function M.buf_lines(buf)
  return vim.api.nvim_buf_get_lines(buf or 0, 0, -1, false)
end

-- wait_for waits up to ms (default 5000) for predicate, failing otherwise.
function M.wait_for(predicate, what, ms)
  if not vim.wait(ms or 5000, predicate, 10) then
    fail("timed out waiting for " .. (what or "condition"))
  end
end

return M
