-- A code reference: the project-relative path, the optional line range and
-- the digest of a saved file, and the selected text only when the person
-- includes it (Feature 014, F014-FR-011, FR-015, data-model.md Code
-- Reference). The agent reads the file itself.
local sha256 = require("ai-dev.sha256").hex

local M = {}

M.max_text = 32768 -- 32 KiB of selected text (R-07)
M.max_instruction = 65536 -- 64 KiB for the whole instruction (R-07)

local function read(path)
  local f = io.open(path, "rb")
  if not f then
    return nil
  end
  local bytes = f:read("*a")
  f:close()
  return bytes
end

local function digest_of(path)
  local bytes = read(path)
  return bytes and sha256(bytes)
end

-- matches_buffer reports whether writing buf would give bytes: its lines
-- joined by its file format, with its final end of line, byte order mark
-- and encoding.
local function matches_buffer(buf, bytes)
  local bo = vim.bo[buf]
  local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  if bytes == "" and #lines == 1 and lines[1] == "" then
    return true
  end
  local eol = ({ dos = "\r\n", mac = "\r" })[bo.fileformat] or "\n"
  local text = table.concat(lines, eol) .. (bo.eol and eol or "")
  local enc = bo.fileencoding
  if enc ~= "" and enc ~= "utf-8" then
    text = vim.iconv(text, "utf-8", enc)
  end
  if text and bo.bomb then
    text = "\239\187\191" .. text
  end
  return text == bytes
end

local Reference = {}
Reference.__index = Reference

-- text returns the selected lines, each ending with a newline.
function Reference:text()
  if not self.range then
    return ""
  end
  local lines = vim.api.nvim_buf_get_lines(self.buf, self.range[1] - 1, self.range[2], false)
  return table.concat(lines, "\n") .. "\n"
end

-- verify computes the digest again; it returns false and "changed" when
-- the file on disk differs from the one previewed.
function Reference:verify()
  if digest_of(self.real) ~= self.digest then
    return false, "changed"
  end
  return true
end

-- from_buffer returns the reference to buf (0 for the current buffer) and
-- range ({first, last} or nil for the whole file), or nil and the reason:
-- no_file, unsaved, outside_project, not_regular_file, or stale when the
-- file on disk no longer holds the buffer's lines.
function M.from_buffer(buf, range)
  buf = buf == 0 and vim.api.nvim_get_current_buf() or buf
  local name = vim.api.nvim_buf_get_name(buf)
  if name == "" or vim.bo[buf].buftype ~= "" then
    return nil, "no_file"
  end
  if vim.bo[buf].modified then
    return nil, "unsaved"
  end
  local root = vim.uv.fs_realpath(vim.fn.getcwd())
  local real = vim.uv.fs_realpath(name)
  if not real or not root then
    return nil, "not_regular_file"
  end
  local path = vim.fs.relpath(root, real)
  if path == nil then
    return nil, "outside_project"
  end
  local stat = vim.uv.fs_stat(real)
  if path == "." or not stat or stat.type ~= "file" then
    return nil, "not_regular_file"
  end
  local bytes = read(real)
  if not bytes then
    return nil, "not_regular_file"
  end
  if not matches_buffer(buf, bytes) then
    return nil, "stale"
  end
  return setmetatable({ buf = buf, path = path, real = real, range = range, digest = sha256(bytes) }, Reference)
end

-- compose returns the instruction the agent receives: the person's text, a
-- blank line, the reference line, and, when include_text, the selected
-- lines between markers. It returns nil and selection_too_large or
-- instruction_too_large above the limits of R-07.
function M.compose(instruction, ref, include_text)
  local text = instruction
  if ref then
    local lines = ref.range and (", lines %d-%d"):format(ref.range[1], ref.range[2]) or ""
    text = ("%s\n\nReference: %s%s (sha256 %s)"):format(instruction, ref.path, lines, ref.digest)
    if include_text and ref.range then
      local selected = ref:text()
      if #selected > M.max_text then
        return nil, "selection_too_large"
      end
      text = text .. "\n--- begin selection ---\n" .. selected .. "--- end selection ---"
    end
  end
  if #text > M.max_instruction then
    return nil, "instruction_too_large"
  end
  return text
end

return M
