# Quickstart: validating Feature 014

Never run a real coding agent: every scenario uses the fake agents of the test suites. The scenarios assume a build from this branch (`go build -o /tmp/ai-dev-014 ./cmd/ai-dev`) and Neovim 0.11.7 or later.

## Automated

```sh
go test ./internal/editor/... ./internal/app/... ./internal/cockpit/...
AI_DEV_REQUIRE_NVIM=1 go test -run '^TestNvim' -v ./... | grep -c -- '--- SKIP'   # expect 0
nvim --headless --clean -l nvim/tests/run.lua      # Lua tests alone
```

Expected: all pass. With `AI_DEV_REQUIRE_NVIM=1`, no test reports `SKIP` for a missing Neovim.

## Manual

1. **Open from a Neovim terminal (US1, SC-001).** In Neovim, `:terminal`, then run `/tmp/ai-dev-014 open --line 20 -- README.md`. README.md opens at line 20 in another window, and the command ends at once with exit 0.
2. **Foreground (US1).** In a plain terminal, `/tmp/ai-dev-014 open -- README.md` runs Neovim in the foreground; `:q` returns with exit 0.
3. **Refusals (US1, SC-002).** Each of these opens nothing and exits 1 with its reason:
   - `open -- ../outside`;
   - `open -- /etc/hosts`;
   - `open -- specs`;
   - a symbolic link to a file outside the project;
   - `NVIM=/tmp/nothing open -- README.md`.
   Usage: `open README.md` (no `--`) opens nothing and exits 2 with the usage line.
4. **Cockpit (A3).** `ai-dev tui` → Features → a feature → "Open spec" → review → confirm. Inside a Neovim terminal it opens in the parent; elsewhere in the foreground. Then "Open file…": the hint lists the feature's files (at most 20); a name that is not a regular file of the feature is refused by the form.
5. **Plugin views (US2).** Install with the lazy.nvim block of contracts/plugin.md, pointing `dir` to this checkout, and set `opts = { cmd = "/tmp/ai-dev-014" }`. `:AiDev features` lists each feature with the fields of `workflow guide --format json`; `<CR>` opens a feature's file; `r` refreshes and updates the time.
6. **Ask an agent (US3).** With a fake agent first on PATH, select lines and run `:'<,'>AiDev ask`. Check:
   - the preview shows the path, the range, the digest and the instruction, and Cancel runs nothing;
   - Run starts the terminal, and the fake agent records the exact instruction;
   - modify the buffer without saving and try again: it is refused (`unsaved`).
7. **Decisions (US4).** In the features view, `a` on a fixture feature, fill the values, and confirm. `workflow-state.json` equals the one from typing the same `ai-dev workflow approve` command by hand.
8. **LazyVim (US5, owner's manual check, U1).** In a LazyVim v16.0.2 configuration, add the preset block of contracts/plugin.md with `dir` pointing to this checkout. Check that which-key shows the `ai-dev` group under `<leader>i`, that `:map <leader>i` lists only the five preset mappings, and that LazyVim's own mappings still work.
9. **Health and removal (US5).** `:checkhealth ai-dev` reports both versions and compatibility. Remove the specification and restart: `:AiDev` no longer exists.
