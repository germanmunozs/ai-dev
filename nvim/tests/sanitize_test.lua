local h = require("helpers")

local T = {}

local function vectors(name)
  return vim.json.decode(h.read(h.repo_root() .. "/testdata/" .. name))
end

-- The vectors record the Go sanitizer's output (F014-FR-023, SC-007).
function T.sanitize_equals_the_go_sanitizer()
  local sanitize = require("ai-dev.sanitize")
  local list = vectors("sanitize-vectors.json")
  h.truthy(#list > 0, "vectors")
  for _, v in ipairs(list) do
    h.eq(v.want, sanitize.text(v.input), v.name)
  end
end

function T.sanitize_replaces_invalid_utf8()
  local sanitize = require("ai-dev.sanitize")
  h.eq("a\239\191\189b", sanitize.text("a\255b"))
  local bad = "\239\191\189"
  h.eq(bad .. bad .. "x", sanitize.text("\226\130x"), "truncated sequence")
  h.eq(bad .. bad .. bad, sanitize.text("\237\160\128"), "surrogate")
  h.eq(bad .. bad, sanitize.text("\192\175"), "overlong")
  h.eq("        " .. bad .. bad .. "      x", sanitize.text("\t\226\130\tx"), "tab columns")
end

function T.sanitize_accepts_nil_and_numbers()
  local sanitize = require("ai-dev.sanitize")
  h.eq("", sanitize.text(nil))
  h.eq("12", sanitize.text(12))
end

return T
