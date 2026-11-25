local h = require("helpers")

local T = {}

-- SC-008: with 50 features and an ai-dev that takes 50 ms per call, the
-- command returns control at once and the view is complete within 2 s.
function T.features_view_returns_at_once_and_completes_within_two_seconds()
  local root = h.tmpdir()
  for i = 1, 50 do
    vim.fn.mkdir(("%s/specs/f%02d"):format(root, i), "p")
  end
  vim.cmd.cd(vim.fn.fnameescape(root))
  local fake = h.fake_ai_dev(h.tmpdir(), table.concat({
    "sleep 0.05",
    'if [ "$1" = version ]; then echo \'{"schema_version":"ai-dev.version/v1","version":"v0.3.0"}\'; exit 0; fi',
    'printf \'{"schema_version":"ai-dev.workflow-guide/v1","feature_id":"%s","outcome":"step_recommended","current_state":"draft","templates":[],"statement":""}\' "$6"',
  }, "\n"))
  require("ai-dev").setup({ cmd = fake })
  local views = require("ai-dev.views")

  local start = vim.uv.hrtime()
  vim.cmd("AiDev features")
  local returned_ms = (vim.uv.hrtime() - start) / 1e6
  h.truthy(returned_ms < 100, ("returned after %.0f ms"):format(returned_ms))

  local buf = vim.api.nvim_get_current_buf()
  local complete = vim.wait(2000, function()
    return views.state(buf) == "complete"
  end, 5)
  local total_ms = (vim.uv.hrtime() - start) / 1e6
  h.truthy(complete, ("complete after %.0f ms"):format(total_ms))
  local lines = h.buf_lines(buf)
  h.eq(51, #lines)
  h.eq("f50  draft  -  -  -", lines[51])
end

return T
