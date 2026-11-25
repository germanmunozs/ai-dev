-- Rendering of untrusted text as inert text, with the rules of the cockpit's
-- Sanitize (internal/cockpit/sanitize.go); testdata/sanitize-vectors.json
-- pins both (F014-FR-023, SC-007).
local M = {}

local byte = string.byte

-- next_rune decodes the UTF-8 sequence at byte i of s with Go's rules: it
-- returns the code point and its length, or nil and 1 for a byte that does
-- not start a valid sequence (overlong forms and surrogates included).
function M.next_rune(s, i)
  local c = byte(s, i)
  if c < 0x80 then
    return c, 1
  end
  local size, min, lo, hi
  if c >= 0xc2 and c <= 0xdf then
    size, min = 2, c - 0xc0
  elseif c >= 0xe0 and c <= 0xef then
    size, min = 3, c - 0xe0
  elseif c >= 0xf0 and c <= 0xf4 then
    size, min = 4, c - 0xf0
  else
    return nil, 1
  end
  -- The accepted range of the second byte excludes overlong forms,
  -- surrogates and code points above U+10FFFF.
  lo, hi = 0x80, 0xbf
  if c == 0xe0 then
    lo = 0xa0
  elseif c == 0xed then
    hi = 0x9f
  elseif c == 0xf0 then
    lo = 0x90
  elseif c == 0xf4 then
    hi = 0x8f
  end
  local r = min
  for k = 1, size - 1 do
    local b = byte(s, i + k)
    if b == nil or b < (k == 1 and lo or 0x80) or b > (k == 1 and hi or 0xbf) then
      return nil, 1
    end
    r = r * 64 + (b - 0x80)
  end
  return r, size
end

-- bidi reports the bidirectional formatting characters that can reorder
-- what the screen shows (Trojan Source).
function M.bidi(r)
  return r == 0x061c or r == 0x200e or r == 0x200f or (r >= 0x202a and r <= 0x202e) or (r >= 0x2066 and r <= 0x2069)
end

-- text renders s as inert text: every C0 control and DEL becomes \xHH,
-- every C1 control and bidirectional control becomes \uHHHH, invalid UTF-8
-- becomes U+FFFD, and a tab expands to spaces up to the next multiple of 8
-- columns. nil gives "", and a number its decimal form.
function M.text(s)
  if s == nil then
    return ""
  end
  s = tostring(s)
  local out, column, i, n = {}, 0, 1, #s
  while i <= n do
    local r, size = M.next_rune(s, i)
    local piece, width
    if r == 0x09 then
      width = 8 - column % 8
      piece = (" "):rep(width)
    elseif r == nil then
      piece, width = "\239\191\189", 1
    elseif r < 0x20 or r == 0x7f then
      piece, width = ("\\x%02x"):format(r), 4
    elseif (r >= 0x80 and r <= 0x9f) or M.bidi(r) then
      piece, width = ("\\u%04x"):format(r), 6
    else
      piece, width = s:sub(i, i + size - 1), 1
    end
    out[#out + 1] = piece
    column = column + width
    i = i + size
  end
  return table.concat(out)
end

return M
