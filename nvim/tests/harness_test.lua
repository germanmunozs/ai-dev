local h = require("helpers")

local T = {}

function T.each_test_starts_in_a_fresh_state()
  h.eq(1, #vim.api.nvim_list_wins(), "windows")
  h.eq("", vim.api.nvim_buf_get_name(0), "buffer name")
end

function T.fake_ai_dev_records_each_argument()
  local dir = h.tmpdir()
  local cmd, log = h.fake_ai_dev(dir, "echo fixture")
  local out = vim.system({ cmd, "a b", "-x", "line\nbreak", "" }, { text = true }):wait()
  h.eq(0, out.code)
  h.eq("fixture\n", out.stdout)
  h.eq({ { "a b", "-x", "line\nbreak", "" } }, h.fake_calls(log))
end

return T
