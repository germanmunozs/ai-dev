local h = require("helpers")

local T = {}

local defaults = { cmd = "ai-dev", concurrency = 8, terminal = "tab", preset = false }

function T.defaults_without_options()
  local notes = h.capture_notify()
  local config = require("ai-dev.config")
  h.eq(defaults, config.resolve(nil))
  h.eq(defaults, config.resolve({}))
  h.eq({}, notes)
end

function T.valid_options_are_kept()
  local notes = h.capture_notify()
  local cfg = require("ai-dev.config").resolve({ cmd = "/opt/ai-dev", concurrency = 16, terminal = "vsplit", preset = true })
  h.eq({ cmd = "/opt/ai-dev", concurrency = 16, terminal = "vsplit", preset = true }, cfg)
  cfg = require("ai-dev.config").resolve({ concurrency = 1, terminal = "split" })
  h.eq(1, cfg.concurrency)
  h.eq("split", cfg.terminal)
  h.eq({}, notes)
end

function T.each_invalid_value_warns_once_and_uses_the_default()
  local config = require("ai-dev.config")
  local cases = {
    { "cmd", "" },
    { "cmd", 3 },
    { "concurrency", 0 },
    { "concurrency", 17 },
    { "concurrency", 1.5 },
    { "concurrency", "8" },
    { "terminal", "float" },
    { "preset", "yes" },
  }
  for _, case in ipairs(cases) do
    local notes = h.capture_notify()
    local cfg = config.resolve({ [case[1]] = case[2], terminal = case[1] == "terminal" and case[2] or "split" })
    h.eq(1, #notes, case[1] .. "=" .. vim.inspect(case[2]))
    h.eq(vim.log.levels.WARN, notes[1].level)
    h.match(case[1], notes[1].msg)
    h.eq(defaults[case[1]], cfg[case[1]], case[1])
    if case[1] ~= "terminal" then
      h.eq("split", cfg.terminal, "a valid option is kept")
    end
  end
end

function T.unknown_option_warns_and_is_ignored()
  local notes = h.capture_notify()
  local cfg = require("ai-dev.config").resolve({ colour = "red", concurrency = 4 })
  h.eq(1, #notes)
  h.eq(vim.log.levels.WARN, notes[1].level)
  h.match("colour", notes[1].msg)
  h.eq(nil, cfg.colour)
  h.eq(4, cfg.concurrency)
end

function T.options_that_are_not_a_table_warn()
  local notes = h.capture_notify()
  h.eq(defaults, require("ai-dev.config").resolve("fast"))
  h.eq(1, #notes)
  h.eq(vim.log.levels.WARN, notes[1].level)
end

return T
