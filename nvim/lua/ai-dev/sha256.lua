-- SHA-256 of a byte string (FIPS 180-4). vim.fn.sha256 is used when it can
-- take the bytes; Neovim 0.11 turns a Lua string holding a NUL into a Blob
-- that vim.fn.sha256 refuses (E976), so those bytes are hashed here.
local bit = require("bit")
local band, bor, bxor, bnot = bit.band, bit.bor, bit.bxor, bit.bnot
local rshift, ror, tobit, tohex = bit.rshift, bit.ror, bit.tobit, bit.tohex

local M = {}

local k = {
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}

-- lua returns the hex digest of bytes, computed in Lua.
function M.lua(bytes)
  local len = #bytes
  -- Padding: 0x80, zeros to 56 mod 64, then the bit length as 64 bits.
  local bits = len * 8
  local tail = { "\128", ("\0"):rep((55 - len) % 64) }
  for i = 7, 0, -1 do
    tail[#tail + 1] = string.char(math.floor(bits / 2 ^ (8 * i)) % 256)
  end
  local msg = bytes .. table.concat(tail)

  local h = { 0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19 }
  local w = {}
  for chunk = 1, #msg, 64 do
    for i = 0, 15 do
      local a, b, c, d = msg:byte(chunk + 4 * i, chunk + 4 * i + 3)
      w[i] = tobit(a * 0x1000000 + b * 0x10000 + c * 0x100 + d)
    end
    for i = 16, 63 do
      local x, y = w[i - 15], w[i - 2]
      local s0 = bxor(ror(x, 7), ror(x, 18), rshift(x, 3))
      local s1 = bxor(ror(y, 17), ror(y, 19), rshift(y, 10))
      w[i] = tobit(w[i - 16] + s0 + w[i - 7] + s1)
    end
    local a, b, c, d, e, f, g, hh = h[1], h[2], h[3], h[4], h[5], h[6], h[7], h[8]
    for i = 0, 63 do
      local s1 = bxor(ror(e, 6), ror(e, 11), ror(e, 25))
      local ch = bxor(band(e, f), band(bnot(e), g))
      local t1 = tobit(hh + s1 + ch + k[i + 1] + w[i])
      local s0 = bxor(ror(a, 2), ror(a, 13), ror(a, 22))
      local maj = bxor(band(a, b), band(a, c), band(b, c))
      local t2 = tobit(s0 + maj)
      hh, g, f, e, d, c, b, a = g, f, e, tobit(d + t1), c, b, a, tobit(t1 + t2)
    end
    h[1], h[2], h[3], h[4] = tobit(h[1] + a), tobit(h[2] + b), tobit(h[3] + c), tobit(h[4] + d)
    h[5], h[6], h[7], h[8] = tobit(h[5] + e), tobit(h[6] + f), tobit(h[7] + g), tobit(h[8] + hh)
  end
  local out = {}
  for i = 1, 8 do
    out[i] = tohex(h[i])
  end
  return table.concat(out)
end

-- hex returns the hex digest of bytes.
function M.hex(bytes)
  if bytes:find("\0", 1, true) then
    return M.lua(bytes)
  end
  return vim.fn.sha256(bytes)
end

return M
