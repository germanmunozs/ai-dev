-- End to end: the ask action with the ai-dev binary built from this
-- branch and fake agents (SC-004, SC-005). TestNvimPluginAsk in
-- cmd/ai-dev/nvimplugin_test.go sets AI_DEV_E2E_BIN and FAKE_AGENT_LOG and
-- puts only fake agents and a fake sh on PATH; without them this file has
-- no tests.
local h = require("helpers")

local bin, agent_log = os.getenv("AI_DEV_E2E_BIN"), os.getenv("FAKE_AGENT_LOG")
if not bin or not agent_log then
  return {}
end

local T = {}

local agents_json = vim.json.encode({
  schema_version = "ai-dev.agents/v1",
  agents = { "claude-code", "gemini-cli" },
  roles = {
    specification = "claude-code",
    planning = "gemini-cli",
    implementation = "claude-code",
    review = "gemini-cli",
    remediation = "claude-code",
  },
})

local cases = {
  { name = "-x $(touch pwned) `id`;|&.txt", content = "plain\n", range = nil },
  { name = "naïve ünï 日本.md", content = "one\ntwo\n", range = { 1, 2 } },
  { name = "src/'q' \"d\" !h.go", content = "rm -rf / $HOME `id`\n\27[31mred\t'q' \"d\"\n", range = { 1, 2 } },
  { name = "--help", content = "-v\n", range = { 1, 1 } },
}

local instruction = "-n Explain $HOME `id` ; echo pwned | sh\n'quoted' \"double\" \\back !bang"

local function agent_calls()
  return h.fake_calls(agent_log)
end

function T.the_agent_receives_exactly_the_composed_instruction()
  for i, case in ipairs(cases) do
    local root = h.tmpdir()
    h.write(root .. "/.ai-dev/agents.json", agents_json .. "\n")
    h.write(root .. "/" .. case.name, case.content)
    vim.cmd.cd(vim.fn.fnameescape(root))
    vim.cmd.edit({ args = { root .. "/" .. case.name }, magic = { file = false, bar = false } })
    require("ai-dev").setup({ cmd = bin })
    local ref = assert(require("ai-dev.reference").from_buffer(0, case.range))
    local want = assert(require("ai-dev.reference").compose(instruction, ref, case.range ~= nil))

    h.stub_ui(case.range and { 1, 1, instruction, 2 } or { 1, 1, instruction })
    local confirms = h.stub_confirm(1)
    local before = #agent_calls()
    local actions = require("ai-dev.actions")
    actions.ask(case.range)
    h.wait_for(function()
      return actions.state("ask") == nil
    end, case.name, 15000)

    local calls = agent_calls()
    h.eq(before + 1, #calls, case.name .. ": one agent run")
    h.eq({ "--print", "--", want }, calls[#calls], case.name)

    -- The preview showed the same instruction, sanitized line by line.
    local preview = confirms[1].preview
    local shown = {}
    local inside = false
    for _, line in ipairs(preview) do
      if line == "Command:" then
        inside = false
      end
      if inside then
        shown[#shown + 1] = line:sub(3)
      end
      if line == "Instruction:" then
        inside = true
      end
    end
    local sanitized = vim.tbl_map(require("ai-dev.sanitize").text, vim.split(want, "\n", { plain = true }))
    h.eq(sanitized, shown, case.name .. ": preview")
    h.eq(nil, vim.uv.fs_stat(root .. "/pwned"), "nothing the values hold ran")
    require("ai-dev").teardown()
    vim.cmd("silent! tabonly!")
    h.truthy(i > 0)
  end
end

return T
