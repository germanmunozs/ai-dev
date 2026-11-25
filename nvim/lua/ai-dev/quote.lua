-- The escaped, quoted form of a value shown in a command preview, with the
-- rules of report.ShellInertQuote (internal/report/quote.go): Go's
-- double-quoted form with every '$', '`' and '!' backslash-escaped, so a
-- pasted preview runs nothing the value contains. testdata/quote-vectors.json
-- pins both (F014-FR-023, SC-007).
local sanitize = require("ai-dev.sanitize")

local M = {}

local short = { [7] = "\\a", [8] = "\\b", [9] = "\\t", [10] = "\\n", [11] = "\\v", [12] = "\\f", [13] = "\\r" }

local ranges = {
  -- Separators other than the ASCII space.
  { 0x00a0, 0x00a0 }, { 0x1680, 0x1680 }, { 0x2000, 0x200a }, { 0x2028, 0x2029 },
  { 0x202f, 0x202f }, { 0x205f, 0x205f }, { 0x3000, 0x3000 },
  -- Format characters.
  { 0x00ad, 0x00ad }, { 0x0600, 0x0605 }, { 0x061c, 0x061c }, { 0x06dd, 0x06dd },
  { 0x070f, 0x070f }, { 0x0890, 0x0891 }, { 0x08e2, 0x08e2 }, { 0x180e, 0x180e },
  { 0x200b, 0x200f }, { 0x202a, 0x202e }, { 0x2060, 0x2064 }, { 0x2066, 0x206f },
  { 0xfeff, 0xfeff }, { 0xfff9, 0xfffb }, { 0x110bd, 0x110bd }, { 0x110cd, 0x110cd },
  { 0x13430, 0x1343f }, { 0x1bca0, 0x1bca3 }, { 0x1d173, 0x1d17a }, { 0xe0001, 0xe0001 },
  { 0xe0020, 0xe007f },
  -- Private use and noncharacters.
  { 0xe000, 0xf8ff }, { 0xfffe, 0xffff }, { 0xf0000, 0x10ffff },
}

-- printable approximates Go's unicode.IsPrint for code points of U+0080 and
-- above: C1 controls, separators other than the space, format characters,
-- private use and noncharacters are not printable. Unassigned code points,
-- which Go also escapes, are shown as they are; they carry no control.
local function printable(r)
  if r <= 0x9f then
    return false
  end
  for _, range in ipairs(ranges) do
    if r >= range[1] and r <= range[2] then
      return false
    end
  end
  return true
end

-- arg returns the quoted form of one value.
function M.arg(value)
  local s = tostring(value)
  local out, i, n = { '"' }, 1, #s
  while i <= n do
    local r, size = sanitize.next_rune(s, i)
    local piece
    if r == nil then
      piece = ("\\x%02x"):format(s:byte(i))
    elseif r == 0x22 or r == 0x5c or r == 0x24 or r == 0x60 or r == 0x21 then
      piece = "\\" .. string.char(r)
    elseif r < 0x80 then
      piece = (r >= 0x20 and r < 0x7f) and string.char(r) or short[r] or ("\\x%02x"):format(r)
    elseif printable(r) then
      piece = s:sub(i, i + size - 1)
    elseif r < 0x10000 then
      piece = ("\\u%04x"):format(r)
    else
      piece = ("\\U%08x"):format(r)
    end
    out[#out + 1] = piece
    i = i + size
  end
  out[#out + 1] = '"'
  return table.concat(out)
end

-- argv returns the quoted values of list joined with one space.
function M.argv(list)
  local quoted = {}
  for i, value in ipairs(list) do
    quoted[i] = M.arg(value)
  end
  return table.concat(quoted, " ")
end

return M
