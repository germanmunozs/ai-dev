local h = require("helpers")

local T = {}

-- project makes a new project the working directory and opens a saved file
-- holding lines in it.
local function project(lines, name)
  local root = h.tmpdir()
  vim.cmd.cd(vim.fn.fnameescape(root))
  local path = root .. "/" .. (name or "src/main.go")
  h.write(path, table.concat(lines, "\n") .. "\n")
  vim.cmd.edit({ args = { path }, magic = { file = false, bar = false } })
  return root, path
end

function T.a_saved_file_gives_path_range_and_digest()
  local reference = require("ai-dev.reference")
  project({ "hello" })
  local ref = assert(reference.from_buffer(0, nil))
  h.eq("src/main.go", ref.path)
  h.eq(nil, ref.range)
  -- sha256 of "hello\n"
  h.eq("5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03", ref.digest)
  ref = assert(reference.from_buffer(0, { 1, 1 }))
  h.eq({ 1, 1 }, ref.range)
end

function T.the_digest_is_of_the_bytes_on_disk()
  local reference = require("ai-dev.reference")
  local _, path = project({ "a" })
  h.write(path, "a\r\nb\0c")
  vim.cmd("edit!")
  local ref = assert(reference.from_buffer(0, nil))
  -- shasum -a 256 of the same bytes
  h.eq("6253d1ec42d765356e50ad56cd81bf2802afb3f7810a75a6927a97c95e3b374a", ref.digest)
end

function T.a_path_through_a_link_inside_the_project_is_relative_to_the_real_root()
  local reference = require("ai-dev.reference")
  local root = h.tmpdir()
  h.write(root .. "/real/a.txt", "x\n")
  vim.uv.fs_symlink(root .. "/real", root .. "/alias")
  vim.cmd.cd(vim.fn.fnameescape(root))
  vim.cmd.edit({ args = { root .. "/alias/a.txt" }, magic = { file = false, bar = false } })
  h.eq("real/a.txt", assert(reference.from_buffer(0, nil)).path)
end

function T.refusals()
  local reference = require("ai-dev.reference")
  local function refused(reason, range)
    local ref, why = reference.from_buffer(0, range)
    h.eq(nil, ref, reason)
    h.eq(reason, why)
  end

  vim.cmd.enew()
  refused("no_file")

  local root = project({ "a", "b" })
  vim.bo.buftype = "nofile"
  refused("no_file")
  vim.bo.buftype = ""

  vim.api.nvim_buf_set_lines(0, 0, 1, false, { "changed" })
  refused("unsaved")

  local outside = h.tmpdir() .. "/o.txt"
  h.write(outside, "o\n")
  vim.cmd.edit({ args = { outside }, magic = { file = false, bar = false } })
  refused("outside_project")

  vim.uv.fs_symlink(outside, root .. "/link.txt")
  vim.cmd.edit({ args = { root .. "/link.txt" }, magic = { file = false, bar = false } })
  refused("outside_project")

  -- A buffer named after a directory (netrw would replace an edited one).
  vim.fn.mkdir(root .. "/dir", "p")
  vim.cmd.enew()
  vim.api.nvim_buf_set_name(0, root .. "/dir")
  vim.bo.modified = false
  refused("not_regular_file")
end

function T.a_terminal_buffer_is_no_file()
  local reference = require("ai-dev.reference")
  vim.cmd.terminal()
  local ref, why = reference.from_buffer(0, nil)
  h.eq(nil, ref)
  h.eq("no_file", why)
end

function T.selected_text_is_at_most_32_kib()
  local reference = require("ai-dev.reference")
  local line = ("x"):rep(1023)
  local lines = {}
  for i = 1, 32 do
    lines[i] = line
  end
  project(lines)
  -- 32 lines of 1023 bytes plus their newlines is exactly 32,768 bytes.
  local ref = assert(reference.from_buffer(0, { 1, 32 }))
  h.eq(32768, #ref:text())
  h.truthy(reference.compose("why?", ref, true))

  lines[33] = ""
  project(lines, "big.txt")
  -- One more line (just its newline) makes 32,769 bytes.
  ref = assert(reference.from_buffer(0, { 1, 33 }))
  local text, why = reference.compose("why?", ref, true)
  h.eq(nil, text)
  h.eq("selection_too_large", why)
  h.truthy(reference.compose("why?", ref, false), "without the text it is accepted")
end

function T.the_instruction_is_at_most_64_kib()
  local reference = require("ai-dev.reference")
  project({ "a" })
  local ref = assert(reference.from_buffer(0, nil))
  local tail = #("\n\nReference: src/main.go (sha256 " .. ref.digest .. ")")
  h.truthy(reference.compose(("i"):rep(65536 - tail), ref, false), "65,536 bytes")
  local text, why = reference.compose(("i"):rep(65537 - tail), ref, false)
  h.eq(nil, text)
  h.eq("instruction_too_large", why)
  text, why = reference.compose(("i"):rep(65537), nil, false)
  h.eq(nil, text)
  h.eq("instruction_too_large", why, "without a reference")
end

function T.compose_gives_the_documented_text()
  local reference = require("ai-dev.reference")
  project({ "one", "two \27[31m\t", "three" })
  local ref = assert(reference.from_buffer(0, { 2, 3 }))
  local d = ref.digest
  h.eq("Explain\n\nReference: src/main.go, lines 2-3 (sha256 " .. d .. ")", reference.compose("Explain", ref, false))
  h.eq(
    "Explain\n\nReference: src/main.go, lines 2-3 (sha256 " .. d .. ")\n"
      .. "--- begin selection ---\ntwo \27[31m\t\nthree\n--- end selection ---",
    reference.compose("Explain", ref, true),
    "selected text unchanged"
  )
  ref = assert(reference.from_buffer(0, nil))
  h.eq("Explain\n\nReference: src/main.go (sha256 " .. d .. ")", reference.compose("Explain", ref, false))
  h.eq("Explain", reference.compose("Explain", nil, false))
end

function T.verify_reports_a_change_on_disk()
  local reference = require("ai-dev.reference")
  local _, path = project({ "a" })
  local ref = assert(reference.from_buffer(0, { 1, 1 }))
  h.eq(true, ref:verify())
  h.write(path, "b\n")
  local ok, why = ref:verify()
  h.eq(false, ok)
  h.eq("changed", why)
  vim.uv.fs_unlink(path)
  ok, why = ref:verify()
  h.eq(false, ok)
  h.eq("changed", why)
end

return T
