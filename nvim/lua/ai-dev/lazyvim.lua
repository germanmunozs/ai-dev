-- The LazyVim preset mappings (Feature 014, R-06). A plain table that
-- requires nothing: the README's lazy.nvim block holds the same table
-- literally, and a test keeps the two equal.
return {
  keys = {
    { "<leader>if", "<cmd>AiDev features<cr>", desc = "ai-dev features" },
    { "<leader>id", "<cmd>AiDev doctor<cr>", desc = "ai-dev doctor" },
    { "<leader>ia", "<cmd>AiDev agents<cr>", desc = "ai-dev agents" },
    { "<leader>ii", "<cmd>AiDev ask<cr>", desc = "ai-dev ask" },
    { "<leader>ii", ":AiDev ask<cr>", mode = "x", desc = "ai-dev ask (selection)" },
    { "<leader>it", "<cmd>AiDev tui<cr>", desc = "ai-dev tui" },
  },
  group = { "<leader>i", group = "ai-dev" },
}
