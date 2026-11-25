-- The plugin's options (Feature 014, data-model.md Plugin Configuration).
local M = {}

M.defaults = { cmd = "ai-dev", concurrency = 8, terminal = "tab", preset = false }

local rules = {
  cmd = {
    "a non-empty string",
    function(v)
      return type(v) == "string" and v ~= ""
    end,
  },
  concurrency = {
    "an integer from 1 to 16",
    function(v)
      return type(v) == "number" and v == math.floor(v) and v >= 1 and v <= 16
    end,
  },
  terminal = {
    '"tab", "split" or "vsplit"',
    function(v)
      return v == "tab" or v == "split" or v == "vsplit"
    end,
  },
  preset = {
    "a boolean",
    function(v)
      return type(v) == "boolean"
    end,
  },
}

local function warn(msg)
  vim.notify("ai-dev: " .. msg, vim.log.levels.WARN)
end

-- resolve returns the defaults with each valid option of opts applied. An
-- unknown option or an invalid value is reported once, and the default is
-- kept for that option (F014-FR-020). cmd is resolved with exepath when the
-- plugin runs it, not here, so a later PATH change is seen.
function M.resolve(opts)
  local cfg = vim.deepcopy(M.defaults)
  if opts == nil then
    return cfg
  end
  if type(opts) ~= "table" then
    warn("options must be a table; using the defaults")
    return cfg
  end
  local names = vim.tbl_keys(opts)
  table.sort(names, function(a, b)
    return tostring(a) < tostring(b)
  end)
  for _, name in ipairs(names) do
    local rule, value = rules[name], opts[name]
    if rule == nil then
      warn(("unknown option %s ignored"):format(tostring(name)))
    elseif rule[2](value) then
      cfg[name] = value
    else
      warn(("option %s must be %s; using %s"):format(name, rule[1], vim.inspect(M.defaults[name])))
    end
  end
  return cfg
end

return M
