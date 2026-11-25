local h = require("helpers")

local T = {}

-- health runs :checkhealth ai-dev with a fake ai-dev printing version and
-- returns the report's text and the fake's log.
local function health(version, opts)
  local dir = h.tmpdir()
  vim.cmd.cd(vim.fn.fnameescape(dir))
  if opts and opts.specs then
    vim.fn.mkdir(dir .. "/specs", "p")
  end
  local fake, log = h.fake_ai_dev(h.tmpdir(), ([[echo '{"schema_version":"ai-dev.version/v1","version":"%s"}']]):format(version))
  require("ai-dev").setup({ cmd = (opts and opts.cmd) or fake })
  vim.cmd("silent checkhealth ai-dev")
  local text = table.concat(h.buf_lines(0), "\n")
  vim.cmd("silent! tabonly!")
  return text, log, fake
end

function T.a_compatible_ai_dev_is_ok()
  local text, log, fake = health("v0.3.0", { specs = true })
  h.match("OK Neovim " .. vim.pesc(tostring(vim.version())), text)
  h.truthy(text:find("ai-dev: " .. fake, 1, true), "resolved path")
  h.match("OK ai%-dev v0%.3%.0 is compatible", text)
  h.match("specs/ directory found", text)
  h.eq({ { "version", "--format", "json" } }, h.fake_calls(log), "nothing else is run")
end

function T.a_development_build_warns()
  local text = health("v0.0.0-20261124232317-e2d5f8628584+dirty")
  h.match("WARNING ai%-dev v0%.0%.0%-20261124232317%-e2d5f8628584%+dirty is a development build", text)
  h.match("no specs/ directory", text)
end

function T.an_incompatible_or_missing_ai_dev_is_an_error()
  local text = health("v0.2.1")
  h.match("ERROR ai%-dev v0%.2%.1 is not supported; supported: >= v0%.3%.0, < v1%.0%.0", text)
  text = health("v0.3.0", { cmd = "/nonexistent/ai-dev" })
  h.match("ERROR .*not found", text)
end

function T.ai_dev_health_runs_the_check()
  health("v0.3.0")
  vim.cmd("silent AiDev health")
  h.match("ai%-dev v0%.3%.0 is compatible", table.concat(h.buf_lines(0), "\n"))
end

return T
