local h = require("helpers")

local T = {}

-- contracts/plugin.md, LazyVim preset.
local want_keys = {
  { "<leader>if", "<cmd>AiDev features<cr>", desc = "ai-dev features" },
  { "<leader>id", "<cmd>AiDev doctor<cr>", desc = "ai-dev doctor" },
  { "<leader>ia", "<cmd>AiDev agents<cr>", desc = "ai-dev agents" },
  { "<leader>ii", "<cmd>AiDev ask<cr>", desc = "ai-dev ask" },
  { "<leader>ii", ":AiDev ask<cr>", mode = "x", desc = "ai-dev ask (selection)" },
  { "<leader>it", "<cmd>AiDev tui<cr>", desc = "ai-dev tui" },
}

function T.the_preset_keys_are_the_documented_ones()
  h.eq(want_keys, require("ai-dev.lazyvim").keys)
end

function T.no_preset_key_collides_with_a_lazyvim_mapping()
  local lazyvim = {}
  for line in io.lines(h.fixtures_dir() .. "/lazyvim-v16.0.2-leader-keys.txt") do
    if line ~= "" and not vim.startswith(line, "#") then
      lazyvim[#lazyvim + 1] = line
    end
  end
  h.truthy(#lazyvim > 300, "fixture read: " .. #lazyvim)
  for _, key in ipairs(require("ai-dev.lazyvim").keys) do
    for _, taken in ipairs(lazyvim) do
      h.truthy(key[1] ~= taken, key[1] .. " equals a LazyVim mapping")
      h.truthy(not vim.startswith(key[1], taken), key[1] .. " has the LazyVim mapping " .. taken .. " as a prefix")
      h.truthy(not vim.startswith(taken, key[1]), key[1] .. " is a prefix of the LazyVim mapping " .. taken)
    end
  end
end

-- readme_keys extracts the keys table of the preset block between the
-- markers of a README and evaluates it.
local function readme_keys(file)
  local text = assert(h.read(h.repo_root() .. "/" .. file), file)
  local block = text:match("<!%-%- ai%-dev%-preset:begin %-%->(.-)<!%-%- ai%-dev%-preset:end %-%->")
  h.truthy(block, file .. ": preset markers")
  local start = block:find("keys%s*=%s*{")
  h.truthy(start, file .. ": keys table")
  local open = block:find("{", start, true)
  local depth, finish = 0, nil
  for i = open, #block do
    local c = block:sub(i, i)
    if c == "{" then
      depth = depth + 1
    elseif c == "}" then
      depth = depth - 1
      if depth == 0 then
        finish = i
        break
      end
    end
  end
  h.truthy(finish, file .. ": closing brace")
  return assert(loadstring("return " .. block:sub(open, finish)))()
end

function T.the_readme_preset_blocks_equal_the_module()
  local keys = require("ai-dev.lazyvim").keys
  h.eq(keys, readme_keys("README.md"), "README.md")
  h.eq(keys, readme_keys("README.es.md"), "README.es.md")
end

local function keymaps()
  return { n = vim.api.nvim_get_keymap("n"), x = vim.api.nvim_get_keymap("x") }
end

function T.setup_with_preset_sets_the_mappings_and_teardown_removes_them()
  vim.g.mapleader = " "
  local before = keymaps()
  local plugin = require("ai-dev")
  plugin.setup({ preset = true })
  for _, key in ipairs(want_keys) do
    local map = vim.fn.maparg(vim.keycode(key[1]), key.mode or "n", false, true)
    h.eq(key[2], map.rhs, key[1] .. " " .. (key.mode or "n"))
    h.eq(key.desc, map.desc)
  end
  plugin.teardown()
  h.eq(before, keymaps(), "teardown removes the preset mappings")
  plugin.setup({})
  h.eq(before, keymaps(), "no mapping without the preset")
end

function T.the_which_key_group_is_defined_only_with_which_key()
  local added = {}
  package.preload["which-key"] = function()
    return {
      add = function(spec)
        added[#added + 1] = spec
      end,
    }
  end
  require("ai-dev").setup({ preset = true })
  h.eq({ { { "<leader>i", group = "ai-dev" } } }, added)
  require("ai-dev").teardown()
  package.preload["which-key"], package.loaded["which-key"] = nil, nil

  require("ai-dev").setup({ preset = true }) -- no which-key: no error
  require("ai-dev").setup({})
  h.eq(1, #added)
end

return T
