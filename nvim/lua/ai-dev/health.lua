-- :checkhealth ai-dev (Feature 014, F014-FR-021, contracts/plugin.md
-- Health check). It runs nothing but `ai-dev version --format json`.
local M = {}

local minimum = "0.11.7"

function M.check()
  local health = vim.health
  local cli = require("ai-dev.cli")
  local sanitize = require("ai-dev.sanitize").text
  local cfg = require("ai-dev").config()

  health.start("Neovim")
  local version = tostring(vim.version())
  if vim.version.ge(vim.version(), minimum) then
    health.ok(("Neovim %s (supported: %s or later)"):format(version, minimum))
  else
    health.error(("Neovim %s is older than %s"):format(version, minimum))
  end

  health.start("ai-dev")
  health.info("ai-dev: " .. sanitize(cli.executable(cfg)))
  local r = cli.check_version_now(cfg)
  if not r.compatible then
    health.error(sanitize(r.message))
  elseif r.dev then
    health.warn(("ai-dev %s is a development build"):format(sanitize(r.version)))
  else
    health.ok(("ai-dev %s is compatible (supported: %s)"):format(sanitize(r.version), cli.supported))
  end

  health.start("Project")
  local stat = vim.uv.fs_stat(vim.fn.getcwd() .. "/specs")
  if stat and stat.type == "directory" then
    health.info("specs/ directory found in the working directory")
  else
    health.info("no specs/ directory in the working directory")
  end
end

return M
