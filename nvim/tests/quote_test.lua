local h = require("helpers")

local T = {}

-- The vectors record the Go quoting's output (F014-FR-023, SC-007).
function T.arg_equals_the_go_quoting()
  local quote = require("ai-dev.quote")
  local list = vim.json.decode(h.read(h.repo_root() .. "/testdata/quote-vectors.json"))
  h.truthy(#list > 0, "vectors")
  for _, v in ipairs(list) do
    h.eq(v.want, quote.arg(v.input), v.name)
  end
end

function T.arg_escapes_invalid_utf8_bytes()
  local quote = require("ai-dev.quote")
  h.eq('"a\\xffb"', quote.arg("a\255b"))
end

function T.argv_joins_quoted_arguments_with_one_space()
  local quote = require("ai-dev.quote")
  h.eq('"ai-dev" "agent" "run" "--" "x" "a \\$b"', quote.argv({ "ai-dev", "agent", "run", "--", "x", "a $b" }))
  h.eq('"a \\$b"', quote.argv({ "a $b" }))
  h.eq("", quote.argv({}))
end

return T
