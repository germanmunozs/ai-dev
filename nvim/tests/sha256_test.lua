local h = require("helpers")

local T = {}

function T.known_vectors()
  local sha256 = require("ai-dev.sha256")
  -- FIPS 180-4 examples, and `printf 'a\0b\n' | shasum -a 256`.
  h.eq("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", sha256.lua(""))
  h.eq("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", sha256.lua("abc"))
  h.eq(
    "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1",
    sha256.lua("abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq")
  )
  h.eq("3a100994c4e38751871e6e8eef9adad2b20177fdeaf650daacdcd74f4c9421e3", sha256.hex("a\0b\n"))
end

-- The Lua digest equals Neovim's for every length around the padding
-- boundaries and for a long input spanning many blocks.
function T.matches_vim_fn_sha256()
  local sha256 = require("ai-dev.sha256")
  for len = 0, 200 do
    local s = {}
    for i = 1, len do
      s[i] = string.char(1 + (i * 37 + len) % 255)
    end
    local bytes = table.concat(s)
    h.eq(vim.fn.sha256(bytes), sha256.lua(bytes), ("length %d"):format(len))
  end
  local long = ("0123456789abcdef\255\128"):rep(4000)
  h.eq(vim.fn.sha256(long), sha256.lua(long), "long input")
end

return T
