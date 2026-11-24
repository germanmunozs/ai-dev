# Tasks: Neovim Integration

**Input**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/open.md`, `contracts/plugin.md` and `quickstart.md` in this directory.

**Tests**: mandatory (constitution IV), written before the code they cover. Each test task names the requirement or success criterion it proves. **No test may start a real coding agent**: every agent is a fake executable on a private `PATH`, and the plugin's ai-dev is either a fake script that records its argument vector or the binary built from this branch. Tests that need Neovim call one helper that skips when `nvim` is missing or older than 0.11.7, and fails instead when `AI_DEV_REQUIRE_NVIM=1` (R-08). Every Go test that needs Neovim is named `TestNvim…`, so CI selects them with `-run '^TestNvim'`.

**Owner decisions applied** (plan.md, 2026-11-24): no global mappings and no mappings in the person's buffers without the preset (buffer-local keys only in the plugin's own views); every absolute path is refused by `ai-dev open`; the Neovim tests run inside the required `gates` job; after `/speckit-analyze`: the cockpit's "open file…" (I3-A), `--` required by `ai-dev open` (A1), and LazyVim installation as the owner's manual check (U1).

## Phase 1: Setup

- [ ] T001 Confirm the baseline on `feature/014-neovim-integration`: run `gofmt -l .`, `go vet ./...`, `go test ./...` and `nvim --version | head -1`; record the result here. List every test that pins the top-level usage line, the general help's command list, the command catalog or the completion command list, with `git grep -n -E 'setup\|tui\|version\|help\|completion>|"tui"' -- '*_test.go'`, and the cockpit tests that pin the list of actions (`internal/cockpit/actions_test.go`, `actions_model_test.go`). Record the list here; only these expectations may change (A1–A3, SC-010).

## Phase 2: Foundational (blocks every story)

**Shared vectors and the Neovim test helper**

- [ ] T002 [P] Create `testdata/sanitize-vectors.json` and `testdata/quote-vectors.json` at the repository root (plan.md Design). Each is an array of `{"name", "input", "want"}`. Sanitize vectors cover: ESC CSI (`\x1b[31m`), OSC 8 hyperlink (`\x1b]8;;http://x\x07t\x1b]8;;\x07`), OSC 52, C1 controls (`\u009b`), bidirectional overrides (`‮`, `⁦`), NUL, `\r`, a tab, a newline, plain ASCII, and Unicode text that must pass unchanged. Quote vectors cover: empty string, space, `'`, `"`, `$HOME`, backquotes, `;`, `|`, a newline, a leading `-`, Unicode. Fill each `want` by running the existing `cockpit.Sanitize` (its `String()` form) and `report.ShellInertQuote`, so the files record current Go behavior.
- [ ] T003 [P] Go test `internal/cockpit/sanitize_vectors_test.go` and `internal/report/quote_vectors_test.go`: read the T002 files (path found by walking up to `go.mod`) and assert every vector against `cockpit.Sanitize` and `report.ShellInertQuote` (F014-FR-023, SC-007). These pass at once; they pin the Go side that the Lua side must equal.
- [ ] T004 [P] Create `internal/testutil/nvim/nvim.go` (test-only helper package, imported only from `_test.go` files): `Require(t testing.TB) string` returns the path of `nvim` from `PATH`; when `nvim` is absent, or `nvim --version` reports a version below 0.11.7, it calls `t.Skip` with that reason; with `AI_DEV_REQUIRE_NVIM=1` it calls `t.Fatal` in both cases instead. Add `internal/testutil/nvim/nvim_test.go` covering four branches with fake `nvim` scripts on a private `PATH`: absent, too old, valid, and too old with `AI_DEV_REQUIRE_NVIM=1` (checked through a fake `testing.TB` that records `Skip` and `Fatal`).

**Lua harness**

- [ ] T005 Create `nvim/tests/run.lua` (R-08, about 60 lines, no framework): it puts `nvim/` on `'runtimepath'`, loads every `nvim/tests/*_test.lua` (or only the files named in `$AI_DEV_LUA_TESTS`, comma-separated), runs each test function in `pcall` with a fresh state (wipes buffers other than the first, restores `vim.ui.input`, `vim.ui.select`, `vim.fn.confirm`, `vim.notify`, and calls `require("ai-dev").teardown()` when loaded), prints `ok <file> <name>` or `FAIL <file> <name>: <error>`, and exits with `vim.cmd.cquit(n_failed > 0 and 1 or 0)`. Provide `nvim/tests/helpers.lua` with `eq`, `truthy`, `match`, `fake_ai_dev(dir, script_body)` (writes an executable `/bin/sh` script that appends each argument as `ARG:<arg>` to `$AI_DEV_FAKE_LOG` and prints a fixture), `stub_ui(answers)` and `stub_confirm(choice)`.
- [ ] T006 Create `cmd/ai-dev/nvimplugin_test.go` with `runLua(t, files ...string)`: `nvim.Require(t)`; run `nvim --headless --clean -l nvim/tests/run.lua` from the repository root with `AI_DEV_LUA_TESTS` set, a 60 s timeout, and a private `HOME`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME` and `XDG_STATE_HOME` under `t.TempDir()` so no person's configuration loads; fail with the runner's output on non-zero exit. Add `TestNvimLuaHarness` that runs a `nvim/tests/harness_test.lua` with one passing test, and checks (through a second temporary test file) that a failing Lua test makes `runLua` fail.

**Plugin core modules** (needed by US2–US5)

- [ ] T007 [P] Lua tests `nvim/tests/sanitize_test.lua` and `nvim/tests/quote_test.lua`, written before T008: read `testdata/sanitize-vectors.json` and `testdata/quote-vectors.json` with `vim.json.decode` and assert every vector against `require("ai-dev.sanitize").text` and `require("ai-dev.quote").arg`; add `quote.argv(list)` joining quoted arguments with one space (F014-FR-023, SC-007).
- [ ] T008 Implement `nvim/lua/ai-dev/sanitize.lua` and `nvim/lua/ai-dev/quote.lua` with the same rules as `internal/cockpit/sanitize.go` and `internal/report/quote.go` (read both before writing). Wire T007 into `cmd/ai-dev/nvimplugin_test.go` as `TestNvimLuaSanitizeQuote`.
- [ ] T009 [P] Lua test `nvim/tests/config_test.lua`, written before T010 (F014-FR-020, data-model.md Plugin Configuration): defaults are `cmd = "ai-dev"`, `concurrency = 8`, `terminal = "tab"`, `preset = false`; `preset` must be a boolean; `cmd` must be a "non-empty string; resolved with `exepath`"; `concurrency` an "integer 1–16"; `terminal` one of `"tab"`, `"split"`, `"vsplit"`; an unknown option and each invalid value (`cmd = ""`, `concurrency = 0`, `17`, `1.5`, `"8"`, `terminal = "float"`, `preset = "yes"`) produce one `vim.notify` at `vim.log.levels.WARN` naming the option, and the default is used for that option while valid options are kept.
- [ ] T010 Implement `nvim/lua/ai-dev/config.lua`: `config.resolve(opts) -> cfg` with the rules of T009.
- [ ] T011 [P] Lua test `nvim/tests/cli_test.lua`, written before T012 (F014-FR-010, R-09), with a fake ai-dev from `helpers.fake_ai_dev`:
  - `cli.run(cfg, args, cb)` calls `vim.system({cfg.cmd, unpack(args)}, {cwd = vim.fn.getcwd(), text = true}, …)`, never a string; the fake log shows the exact arguments;
  - `cli.check_version`: `{"schema_version":"ai-dev.version/v1","version":"v0.3.0"}` and `v0.9.4` are compatible; `v0.2.1` and `v1.0.0` are incompatible with a message naming found and supported (`>= v0.3.0, < v1.0.0`); `v0.3.1-dev+abc` and a Go pseudo-version are compatible with `dev = true`; a wrong schema, non-JSON output, a non-zero exit and a missing executable are each incompatible with their own message;
  - `cli.decode(stdout, schema)` refuses a missing or different `schema_version` and names both versions; `cli.check_shape("doctor", t)`, `("agents", t)` and `("agent_select", t)` accept the shapes of R-09 and refuse a table missing any listed field;
  - the callback runs on the main loop (`vim.schedule`), and the call returns before the fake ai-dev exits (it sleeps 0.3 s), so editing is not blocked (F014-FR-008).
- [ ] T012 Implement `nvim/lua/ai-dev/cli.lua` per T011, with a semaphore limiting running processes to `cfg.concurrency`.
- [ ] T013 [P] Lua test `nvim/tests/init_test.lua`, written before T014 (contracts/plugin.md Command, Removal): before `setup()`, `:AiDev` does not exist and requiring the modules defines no command, autocommand or mapping; after `setup({})`, `:AiDev` exists with completion `features doctor agents ask tui health`; an unknown subcommand notifies an error and runs nothing; `vim.api.nvim_get_keymap(mode)` for `n`, `v`, `x`, `i` is identical before and after `setup()` (owner decision 1, F014-FR-019); `teardown()` removes the command and the `AiDev` autocommand group and leaves the keymaps as before.
- [ ] T014 Implement `nvim/lua/ai-dev/init.lua`: `setup(opts)` stores the resolved config, creates the `AiDev` augroup and the `:AiDev` user command (`nargs = "?"`, `range = true`, completion), dispatching to `views`, `actions` and `health` modules loaded lazily; `teardown()`. No `plugin/` directory (R-05). Wire T009, T011 and T013 into `cmd/ai-dev/nvimplugin_test.go` as `TestNvimLuaCore`.

**Checkpoint:** the Lua harness runs from `go test`, shared vectors pin both sanitizers, and the plugin loads without defining anything until `setup()`.

## Phase 3: User Story 1, open a project file in Neovim from ai-dev (P1) 🎯 MVP

**Goal**: `ai-dev open` opens a validated project file in the parent Neovim or in a foreground Neovim, and the cockpit offers it. **Independent test**: fixture project, hostile paths, fake RPC server, fake `Launcher`, and a real headless Neovim started with `--listen`.

**Location and endpoint**

- [ ] T015 [P] [US1] Tests `internal/editor/location_test.go`, written before T016 (F014-FR-002, SC-002, data-model.md Editor Location). With a temporary project (resolved through `filepath.EvalSymlinks`, since macOS `/var` is a link):
  - accepted: `a.txt`, `dir/b.go`, `./dir/../a.txt`, `with space.txt`, `ünï-código.md`, `-leading.txt`, a symbolic link inside the project to a file inside it; `Rel` is the given path, `Abs` the resolved one, `Line`/`Column` default to 1;
  - refused in this order: `absolute_path` for any absolute path, including one inside the project (owner decision 2); `not_found` for a missing file and a dangling link; `outside_project` for `../outside`, `dir/../../outside` and a link to a file outside; `not_regular_file` for a directory, a FIFO (`syscall.Mkfifo`), and a link to a directory;
  - a project root that is itself a symbolic link works.
- [ ] T016 [US1] Implement `internal/editor/location.go`: `type Location struct{ Rel, Abs string; Line, Column int }`, `type PathError struct{ Reason string }`, `Resolve(root, path string, line, column int) (Location, error)` per T015, standard library only. Comment with F014-FR-002.
- [ ] T017 [P] [US1] Tests `internal/editor/endpoint_unix_test.go` (`//go:build darwin || linux`), written before T018 (F014-FR-003, R-03): `CheckEndpoint` returns `not_local_socket` for `""`, `relative.sock`, `127.0.0.1:6666`, `[::1]:6666`; `not_socket` for a regular file, a directory, and a symbolic link to a real socket; `not_owned` for a real socket when the injected `ownerOf` returns a different uid; nil for a real Unix socket created with `net.Listen("unix", …)` in a short temporary directory.
- [ ] T018 [US1] Implement `internal/editor/endpoint_unix.go` (`Lstat`, `ModeSocket`, `Stat_t.Uid == os.Getuid()` behind `var ownerOf`) and `internal/editor/endpoint_other.go` (`//go:build !darwin && !linux`, returns `platform_unsupported`). Use the reason constants of data-model.md.

**msgpack-RPC**

- [ ] T019 [P] [US1] Tests `internal/editor/msgpack_test.go`, written before T020 (R-02): encode `[0, 1, "nvim_get_api_info", []]` and `[0, 2, "nvim_exec_lua", ["chunk", ["/p a", 12, 1]]]` to the exact bytes (fixint, uint8/16/32, int families, fixstr/str8/16/32, fixarray/array16/32, nil, bool); decode every type Neovim returns (nil, bool, positive and negative int families, uint64, float32/64, str, bin, array, map, fixext/ext8/16/32 as an opaque value); reject truncated input, a length above the 1 MiB limit, nesting deeper than 32, and a map key that is not a string or integer. Add `FuzzDecode` with the encoded samples as seeds; it must never panic and never allocate beyond the limit.
- [ ] T020 [US1] Implement `internal/editor/msgpack.go` per T019.
- [ ] T021 [P] [US1] Tests `internal/editor/open_test.go`, written before T022, with an in-process fake server on a Unix socket in `t.TempDir()` (F014-FR-003, F014-FR-005, SC-001, R-02):
  - `OpenInParent` sends exactly two requests: `nvim_get_api_info` with no params, then `nvim_exec_lua` with params `[openChunk, [Abs, Line, Column]]`; the path is never inside the chunk text;
  - the API info version 0.11.7 and 0.12.6 is accepted; 0.11.6, 0.10.4 and a missing `version` map give `version_unsupported` and `not_neovim`; a non-array result gives `not_neovim`;
  - a server that accepts and never answers gives `no_answer` within 1.2 s per call and 2 s overall; a refused connection gives `no_answer`; an error response `[1, id, [0, "E499: …\x1b[31m"], nil]` gives `editor_error` with the message sanitized; a response above 1 MiB gives `no_answer` without reading the rest; a mismatched message id is ignored until the right one arrives;
  - `openChunk` contains `magic = { file = false, bar = false }` and `nvim_cmd`, reads its values only from `...` (the `nvim_exec_lua` arguments), and contains none of `nvim_command`, `vim.cmd`, `vim.fn.system`, `vim.system`, `execute(` or `loadstring`; T029 proves with real Neovim that a hostile name opens literally;
  - `ForegroundArgs(Location{Abs: "/p/-x b", Line: 12, Column: 3})` is exactly `["+call cursor(12, 3)", "--", "/p/-x b"]`.
- [ ] T022 [US1] Implement `internal/editor/rpc.go` (`Dial(endpoint string, timeout time.Duration) (*Client, error)`, `Client.Call(method string, params ...any) (any, error)`, `Close`) and `internal/editor/open.go` (`OpenInParent`, `openChunk` constant as specified in R-02 and contracts/open.md step 3: window choice, `nvim_cmd` with `edit` or `split`, clamped cursor, focus; `ForegroundArgs`).

**Command**

- [ ] T023 [P] [US1] Tests `internal/app/open_test.go`, written before T024 (contracts/open.md, F014-FR-001 to FR-005, FR-025), injecting the environment, terminal detection, `exec.LookPath`, the dialer and `system.Launcher`:
  - usage failures exit 2 with the usage line on standard error and no other output: no path, two paths, any path without `--` before it (`open README.md`, `open -x`; A1), `--line 0`, `--line -1`, `--line x`, `--column 0`, an unknown option, `--format yaml`;
  - each `path_refused` reason exits 1 with the text of contracts/open.md on standard error and the JSON `{"schema_version":"ai-dev.open/v1","outcome":"path_refused","reason":…,"path":…}` on standard output with `--format json`;
  - `$NVIM` set and each endpoint reason (fake checker and fake dialer) exits 1 as `editor_refused`, and neither the launcher nor `LookPath` is called (no fallback);
  - no `$NVIM`: not a terminal → `editor_unavailable`/`no_terminal`; terminals but no `nvim` → `nvim_not_found`; terminals and `nvim` → the launcher receives `nvim` with `ForegroundArgs` and inherited streams; its exit 0 → `opened`/`foreground` with no text output, and with `--format json` the report `{"schema_version":"ai-dev.open/v1","outcome":"opened","target":"foreground",…}` printed after the launcher returns; exit 3 → `editor_failed` exit 1, with `"target":"foreground","exit_code":3` in JSON;
  - parent success → `Opened <rel> at line L, column C in the parent Neovim.` and JSON with `target":"parent"`;
  - no report contains the project's absolute path, `$HOME`, or the value of `$NVIM` (assert with `strings.Contains` on every output above);
  - a path with an escape sequence is shown escaped.
- [ ] T024 [US1] Implement `internal/app/open.go` (`RunOpen`) per T023, reusing `flags.go` conventions and the terminal detection of `terminal_posix.go`; route `open` in `internal/app/app.go`.
- [ ] T025 [P] [US1] Update and extend the catalog, help, usage and completion tests, written before T026 (A1, A2, F014-FR-026): in every file listed by T001, the top-level usage becomes `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|open|version|help|completion> [options]`; `internal/app/catalog_test.go` expects `open` with help line `Changes: nothing in the project; opens the file in Neovim`; `internal/app/help_test.go` expects `open` in the general help and `ai-dev help open` / `ai-dev open --help` to print the usage of contracts/open.md; `internal/app/completion_test.go` expects `open` among commands, `--line --column --format` as its options, and file completion after `--`. Change nothing else in those files.
- [ ] T026 [US1] Implement the catalog entry, help text and completion for `open` in `internal/app/catalog.go`, `help.go`, `completion.go`, and the new `topLevelUsage` in `app.go`.

**Cockpit (A3)**

- [ ] T027 [P] [US1] Tests in `internal/cockpit/actions_test.go` and `internal/app/cockpit_test.go`, written before T028 (F014-FR-006, A3, contracts/open.md Cockpit actions): `Actions()` keeps every existing action in its order and adds "Open spec", "Open plan", "Open tasks" and "Open file…" after them (I3-A);
  - "Open spec", "Open plan" and "Open tasks" are available only when `row.Files` contains `spec.md` (resp. `plan.md`, `tasks.md`), with the reason `no spec.md in this feature` (resp. `no plan.md…`, `no tasks.md…`) otherwise; they take no input;
  - "Open file…" is available when `row.Files` is non-empty, with the reason `no files in this feature` otherwise; it has one required field `file` with nothing prefilled, whose hint lists `row.Files` sorted and sanitized, at most 20 names followed by `… and N more` (with 25 files the hint shows 20 and `… and 5 more`, and the 25th file is accepted); a value not equal to one of them is refused in the review with `not a file of this feature` and starts nothing (including `../x`, `sub/y`, an absolute path and a name differing only in case);
  - without a selected feature, the four use the existing reason of feature actions;
  - `Argv` is exactly `["open", "--", "specs/<id>/<file>"]` with the raw `ID` and file name, also for names with spaces and a leading `-`;
  - `internal/app` fills `Files` with the names of the regular files directly in `specs/<id>/`, excluding symbolic links and subdirectories.

  Existing action tests are unchanged.
- [ ] T028 [US1] Implement `FeatureRow.Files []string` (raw names, used only as arguments and, sanitized, in the hint) in `internal/cockpit/data.go`, the four actions and the `file` validation in `internal/cockpit/actions.go`, and filling `Files` in `internal/app/cockpit.go`. Reuse the existing field input; add no input kind. No other area, key or action changes.

**Real Neovim**

- [ ] T029 [US1] Integration test `TestNvimOpen` in `internal/app/open_nvim_test.go` (F014-FR-003, SC-001, SC-002): `nvim.Require(t)`; build ai-dev to `t.TempDir()`; start `nvim --headless --clean --listen <tmp>/n.sock` with a fixture project as working directory; wait for the socket; run the binary with `NVIM=<sock>`:
  - `open --line 3 --column 2 -- "a %b #c $HOME \`x\` ;d|e.txt"` exits 0 within 1 s, and through `editor.Client` the current buffer name ends with that literal name and the cursor is `(3, 1)` (0-based column); no file named `x` output appears (backquotes not run);
  - `--line 9999` on a 5-line file puts the cursor on line 5;
  - when the current window shows a terminal buffer (opened beforehand with `editor.Client` calling `nvim_exec_lua` on a constant test chunk that runs `vim.cmd.terminal()`), the file opens in another window and that window is focused;
  - `NVIM=<tmp>/missing.sock` exits 1 `editor_refused`/`not_socket`; a TCP `--listen 127.0.0.1:0` address is `not_local_socket`;
  - hostile paths of T015 open nothing and Neovim's buffer list is unchanged;
  - with a second headless Neovim listening on another socket, `open` with `NVIM` naming the first leaves the second's buffer list and current buffer unchanged (Edge Case "several Neovim instances", F014-FR-005).
  Run it locally against Neovim 0.12.5 and record the result here.

**Checkpoint:** `ai-dev open` and the cockpit's "open" actions work on their own; this is the MVP.

## Phase 4: User Story 2, see the project and its features from Neovim (P1)

**Goal**: features, environment and agents views built only from ai-dev's JSON. **Independent test**: fake ai-dev printing fixture JSON captured from the real binary for fixture projects.

- [ ] T030 [US2] Create the fixtures `nvim/tests/fixtures/`: run the binary built from this branch in a controlled environment: `PATH` holding only a private directory with fake `git`, `specify` and one fake executable per catalog agent (a chosen subset present, so availability varies), plus `/usr/bin:/bin` for the fake scripts' interpreter; a private `HOME` and `XDG_*`; fixture projects copied to `t.TempDir()`; no environment variable of the person passed through. Use fixture projects that cover every Workflow State, a feature without state, an invalid record, an unknown feature name, a project without `specs/`, an invalid agent configuration, and `doctor` with failing checks; save each JSON output, and record the command line that produced it and the environment above in `nvim/tests/fixtures/README.md`. Add `cmd/ai-dev/nvimfixtures_test.go` that regenerates them in memory under the same environment and fails if any saved fixture differs, so the fixtures cannot drift from the real outputs and give the same result on macOS, Linux and CI (SC-003). If `doctor` reports something that cannot be controlled (such as a real system version), compare its fixture by shape and by each check's `id` and `status` only, and record that here.
- [ ] T031 [P] [US2] Lua tests `nvim/tests/views_test.lua`, written before T032 (F014-FR-007, FR-008, FR-010, US2 scenarios 1, 2, 4, 5, SC-003), with a fake ai-dev that serves the T030 fixtures by its arguments:
  - `:AiDev features` lists the non-symbolic-link directories under `specs/` (a symbolic link directory is not listed) and runs `workflow guide --format json -- <id>` for each, never more than `concurrency` at once (the fake records start and end times);
  - each row is `<id>  <current_state | outcome>  <activity>  <role>  <agent> (<availability>, <source>)` with `-` for a missing field, matching the fixture field by field; an invalid record and `feature_unknown` rows show their outcome and no activity or agent;
  - `:AiDev doctor` shows `overall` and each check `id`, `name`, `status`; `:AiDev agents` shows the agents and role assignments of `setup show --format json`;
  - the first line shows the evaluation time; `r` re-runs the commands and updates the time;
  - the buffer has `buftype=nofile`, `modifiable=false`, `bufhidden=wipe`; the keys `r`, `<CR>`, `n`, `a`, `x`, `q` exist only buffer-locally in that buffer (owner decision 1);
  - an incompatible version, an unknown schema (`ai-dev.workflow-guide/v2`), a wrong doctor shape, non-JSON output and a missing ai-dev each show a message naming what was found and supported, and no action key does anything;
  - a feature name and a check message holding the T002 hostile strings appear escaped (F014-FR-023);
  - after a view is complete, no ai-dev process starts again for 2 s without a keypress (no background polling, F014-FR-008).
- [ ] T032 [US2] Implement `nvim/lua/ai-dev/views.lua` per T031.
- [ ] T033 [P] [US2] Lua test in `nvim/tests/views_test.lua`, written before T034 (F014-FR-009, US2 scenario 3): `<CR>` on a feature lists, through `vim.ui.select`, the regular files of `specs/<id>/` sorted, and opens the chosen one in the current window with a command that does not expand names (`vim.cmd.edit({args = {path}, magic = {file = false, bar = false}})`); a file name with `%`, `#`, backquotes and a leading `-` opens literally; a symbolic link in the feature directory pointing outside the project is not listed; cancelling the selection opens nothing.
- [ ] T034 [US2] Implement the artifact opening in `nvim/lua/ai-dev/views.lua`.
- [ ] T035 [US2] Performance test in `nvim/tests/views_perf_test.lua` (SC-008): with a fake ai-dev that sleeps 50 ms per call and 50 features, `:AiDev features` returns control in less than 100 ms (measured with `vim.uv.hrtime`), and the view is complete within 2 s (`vim.wait`). Wire T031, T033 and T035 into `cmd/ai-dev/nvimplugin_test.go` as `TestNvimLuaViews`.

**Checkpoint:** the views are complete and read-only.

## Phase 5: User Story 3, ask an agent about the code I am looking at (P1)

**Goal**: a file or range becomes a bounded reference in the instruction of `ai-dev agent run`, after a preview and confirmation. **Independent test**: fake ai-dev and, end to end, the real binary with fake agents.

- [ ] T036 [P] [US3] Lua tests `nvim/tests/reference_test.lua`, written before T037 (F014-FR-011, FR-015, data-model.md Code Reference, US3 scenarios 3, 4):
  - a saved file gives `path` from `vim.fs.relpath(realpath(root), realpath(file))`, `range` `{first, last}` from a visual selection or none, and `digest` equal to `vim.fn.sha256` of the bytes read from disk (compare with a known SHA-256 of a fixture);
  - refusals: `no_file` (unnamed buffer, `buftype=terminal`), `unsaved` (modified buffer), `outside_project` (a file outside `getcwd()`, and a link inside pointing outside), `not_regular_file`, `selection_too_large` for 32,769 bytes ("at most 32 KiB") while 32,768 is accepted, `instruction_too_large` above 64 KiB (65,536 bytes);
  - `compose(instruction, ref, include_text)` gives the person's text, a blank line, and `Reference: <path>, lines <first>-<last> (sha256 <digest>)`, followed, only when text is included, by the selected lines between `--- begin selection ---` and `--- end selection ---`; the selected text keeps its control characters unchanged in the instruction;
  - `ref:verify()` returns `changed` after the file is rewritten on disk.
- [ ] T037 [US3] Implement `nvim/lua/ai-dev/reference.lua` per T036.
- [ ] T038 [P] [US3] Lua tests `nvim/tests/actions_ask_test.lua`, written before T039 (F014-FR-012 to FR-017, FR-024, US3 scenarios 1, 2, 5, SC-004, SC-006), with stubbed `vim.ui` and confirmation:
  - `:AiDev ask` asks first for "by role" or "from the catalog" with no default; by role it runs `agent select --format json -- <role>` and uses `selected_agent_id`; from the catalog it lists `agents --format json`; then it asks for the instruction with no default text, then whether to include the selected text (only with a range);
  - the preview floating window shows the agent, the relative path, the range, the digest, whether text is included, the complete composed instruction (sanitized for display) and `quote.argv(argv)`; `argv` is exactly `{cmd, "agent", "run", "--", <agent-id>, <instruction>}`;
  - confirmation uses `vim.fn.confirm("Run this command?", "&Run\n&Cancel", 2)`; answering 2, 0 (Esc) or cancelling any input runs nothing and the fake log stays empty;
  - on Run, the digest is recomputed; when the file changed, it notifies `changed`, shows the preview again, and runs nothing;
  - on Run with an unchanged file, `jobstart` receives a list (not a string) with `term = true` and `cwd = getcwd()` in a new tab (or `split`/`vsplit` per config); on exit it notifies `ai-dev exited with code N` with the fake's code and refreshes open views;
  - pressing Run twice or calling the action while one is `running` starts one command only;
  - an instruction or file name with spaces, quotes, `$`, backquotes, `;`, newlines and a leading `-` reaches the fake ai-dev as one argument, unchanged.
- [ ] T039 [US3] Implement `nvim/lua/ai-dev/actions.lua` with the shared flow (collect → preview → confirm → run → ended) of data-model.md Plugin Action, and the `ask` action; wire `:AiDev ask` with and without a range in `init.lua`.
- [ ] T040 [US3] End-to-end test `TestNvimPluginAsk` in `cmd/ai-dev/nvimplugin_test.go` (SC-004, SC-005): build ai-dev; put fake agents for every catalog executable on a private `PATH` (each writes its arguments, one per line with a length prefix, to `$FAKE_AGENT_LOG`); configure a project with a role assignment; run `nvim/tests/e2e_ask_test.lua` with `cmd` = the built binary and stubbed confirmation = Run, for files and selections from a fixture set holding shell metacharacters, newlines, leading dashes and Unicode; assert the fake agent received exactly the composed instruction the preview showed, and no shell was started (fake `sh` first on `PATH` that records any call and fails the test). Wire T036 and T038 as `TestNvimLuaAsk`.

**Checkpoint:** references and the `ask` action work end to end with fake agents.

## Phase 6: User Story 4, record a decision and run the next step from Neovim (P1)

**Goal**: `next`, `approve` and `abandon` through the same review and confirmation. **Independent test**: compare `workflow-state.json`, output and exit code with the same commands typed by hand.

- [ ] T041 [P] [US4] Lua tests `nvim/tests/actions_decide_test.lua`, written before T042 (F014-FR-012, FR-013, FR-016, FR-017, US4 scenarios 1–5), with a fake ai-dev:
  - fields, labels and hints equal the cockpit's for approve, abandon and next (read them from `internal/cockpit/actions.go` and pin them in the test); no value is prefilled;
  - `approve` argv is `{cmd, "workflow", "approve", "--approver", a, "--", feature, state}`, plus `"--reason", r` before `--` only when a reason is given; `abandon` requires a reason and gives `{cmd, "workflow", "abandon", "--approver", a, "--reason", r, "--", feature, "abandoned"}`; `next` gives `{cmd, "agent", "run", "--", <guide.agent>, <instruction>}`; `tui` gives `{cmd, "tui"}` and goes through the same preview and confirmation, and Cancel runs nothing;
  - availability follows F012-FR-011 (terminal feature, no recommended step, agent unavailable, incompatible ai-dev), and an unavailable action shows its reason and starts nothing;
  - `n`, `a`, `x` in the features view act on the feature under the cursor; typed elsewhere, `:AiDev` for these actions asks for the feature from the list with nothing preselected;
  - a non-zero exit is shown as `ai-dev exited with code N`, and the plugin never reports success itself.
- [ ] T042 [US4] Implement the `next`, `approve`, `abandon` and `tui` actions in `nvim/lua/ai-dev/actions.lua` and the view keys in `views.lua`.
- [ ] T043 [US4] No-confirmation sweep `nvim/tests/no_confirm_test.lua` (F014-FR-024, SC-006): with a fake ai-dev that logs every call, run every `:AiDev` subcommand and every view key (the preset mappings are swept by T049, after T048) with all inputs answered and the confirmation answered Cancel, then with Esc; assert 0 calls of `agent run`, `workflow approve` and `workflow abandon`, and that no buffer content appears in the log.
- [ ] T044 [US4] End-to-end test `TestNvimPluginDecisions` in `cmd/ai-dev/nvimplugin_test.go` (SC-004): on two copies of a fixture project, approve and abandon a feature through `nvim/tests/e2e_decide_test.lua` with the built binary, and type the same commands by hand on the other copy; `workflow-state.json` (ignoring the recorded time), the output and the exit code are equal; a refused approval gives the same exit code and the plugin shows it. Wire T041 and T043 as `TestNvimLuaDecisions`.

**Checkpoint:** every action of F014-FR-013 runs only after its own confirmation.

## Phase 7: User Story 5, install, configure and remove the plugin (P2)

**Goal**: health check, LazyVim preset, configuration and clean removal. **Independent test**: clean headless configurations.

- [ ] T045 [P] [US5] Lua test `nvim/tests/health_test.lua`, written before T046 (F014-FR-021, contracts/plugin.md Health check): `:checkhealth ai-dev` reports the Neovim version as ok (≥ 0.11.7), the resolved ai-dev path, the ai-dev version with ok, a warning for a development build, an error for an incompatible or missing ai-dev, and an informational line for `specs/` in the working directory; nothing is run besides `version --format json`.
- [ ] T046 [US5] Implement `nvim/lua/ai-dev/health.lua` and `:AiDev health`.
- [ ] T047 [P] [US5] Create `nvim/tests/fixtures/lazyvim-v16.0.2-leader-keys.txt`, one `<leader>…` string per line, with the producing command in its header comment (R-06: every `"<leader>…"` string in `lua/lazyvim` at tag v16.0.2, default specs and extras). Lua test `nvim/tests/lazyvim_test.lua`, written before T048 (F014-FR-019): `require("ai-dev.lazyvim").keys` is exactly `<leader>if`, `<leader>id`, `<leader>ia`, `<leader>ii` (normal and visual), `<leader>it` with the commands of contracts/plugin.md; no key equals a fixture line, has one as a prefix, or is a prefix of one; the which-key group is defined only when a stub `which-key` module is loadable; the `keys` block of the lazy.nvim preset in `README.md` and `README.es.md` (between the markers `<!-- ai-dev-preset:begin -->` and `<!-- ai-dev-preset:end -->`), equals the table of `ai-dev.lazyvim` (C1): the text between the markers is extracted, the table constructor after `keys =` is taken up to its matching closing brace, and `loadstring("return " .. table_text)()` gives the table compared.
- [ ] T048 [US5] Implement `nvim/lua/ai-dev/lazyvim.lua` (a plain table that requires nothing else) and `setup({ preset = true })` setting those mappings with `vim.keymap.set`; `teardown()` also removes them when they were set. Add to `README.md` and `README.es.md` a minimal "Neovim plugin" section holding the LazyVim preset block between `<!-- ai-dev-preset:begin -->` and `<!-- ai-dev-preset:end -->`, so that T047 passes; T052 writes the rest of the documentation around it without changing the block.
- [ ] T049 [US5] Extend `nvim/tests/no_confirm_test.lua` (F014-FR-024, SC-006): with `setup({ preset = true })`, run every preset mapping in normal and visual mode with the confirmation answered Cancel and then Esc; 0 calls of `agent run`, `workflow approve` and `workflow abandon`.
- [ ] T050 [US5] Installation and removal test `nvim/tests/install_test.lua` plus `TestNvimPluginInstall` in `cmd/ai-dev/nvimplugin_test.go` (US5 scenarios 1, 4, F014-FR-018, FR-022): with an init file that does what the documented lazy.nvim `config` does (`vim.opt.rtp:append(<repo>/nvim)`; `require("ai-dev").setup({})`) and no plugin manager (no network in tests), `:AiDev` exists and `:checkhealth ai-dev` runs; starting Neovim again with an init file without the plugin leaves no `:AiDev`, no `AiDev` augroup and no mapping; `teardown()` in the same session gives the same result. While running every view and action with the fake ai-dev, the file system under the private `HOME` and the project is unchanged (compare a listing with sizes and modification times before and after), and the fake records that only the configured `cmd` was started; no view, preview or notification contains the value of `$HOME`, `$NVIM` or a sentinel environment variable `AI_DEV_TEST_SECRET` set for the run (F014-FR-025). Wire T045, T047 and the extended no-confirmation sweep as `TestNvimLuaInstall`.

**Checkpoint:** the plugin installs, reports its health and leaves nothing behind.

## Phase 8: Polish and cross-cutting concerns

- [ ] T051 Update `.github/workflows/ci.yml` job `gates` (R-08, owner decision A): after the existing test step, add "Install Neovim" that downloads `nvim-linux-x86_64.tar.gz` (ubuntu) or `nvim-macos-arm64.tar.gz` (macOS) for v0.11.7 and v0.12.6 from `https://github.com/neovim/neovim/releases/download/<tag>/`, checks each against the SHA-256 of research.md R-08 with `sha256sum -c`/`shasum -a 256 -c`, and extracts each into `$RUNNER_TEMP/nvim-<version>`; and "Neovim tests" that, for each version, checks `nvim --version` first, then runs `PATH="$RUNNER_TEMP/nvim-<version>/bin:$PATH" AI_DEV_REQUIRE_NVIM=1 go test -race -run '^TestNvim' ./...`. Add `FuzzDecode` of `./internal/editor` to the existing fuzz step. No new job; other jobs unchanged.
- [ ] T052 [P] Documentation in `README.md` and `README.es.md` (F014-FR-027): `ai-dev open` with its outcomes; the plugin's installation with the lazy.nvim block of contracts/plugin.md, the LazyVim preset block written by T048 (unchanged), configuration options and defaults, supported Neovim versions (0.11.7 and later, tested on 0.11.7 and 0.12.6), the privacy boundary (a reference holds path, range and digest; text only when chosen; the agent reads the file and may send what it reads to its provider), troubleshooting (`:checkhealth ai-dev`, `$NVIM` refusals), that closing Neovim ends an agent running in its terminal, removal; `nvim/` in the architecture section.
- [ ] T053 [P] `CHANGELOG.md` and `CHANGELOG.es.md` under `[Unreleased]`: `ai-dev open`, the cockpit's "open" actions, the Neovim plugin, `open` in usage and help (A1, A2). `SECURITY.md`: the `$NVIM` trust model, the unchecked peer credentials residual (R-03), and the Ex expansion control (R-02).
- [ ] T054 Run the quality gates of plan.md: `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `AI_DEV_REQUIRE_NVIM=1 go test -run '^TestNvim' -v ./...` on local Neovim 0.12.5 with no `--- SKIP` in its output, `go test -run '^$' -fuzz FuzzDecode -fuzztime 20s ./internal/editor`, `staticcheck ./...` with `GOOS=linux` and `GOOS=darwin`, `govulncheck ./...`, the cross-build for darwin, linux, windows and freebsd, and `git diff --exit-code go.mod go.sum`. Record each result here; state that Neovim 0.11.7 is verified only in CI.
- [ ] T055 Walk through quickstart.md's manual scenarios that can run without a real agent and record the result of each; mark those left to the owner: the foreground editor in a real terminal, and the LazyVim v16.0.2 installation of quickstart.md step 8 (U1).
- [ ] T056 Independent review of the whole branch before the PR (constitution VIII), against spec.md, plan.md and contracts/; record here the implementing agent, the independent reviewer, the findings and the fixes (constitution VIII). The PR is opened only with the owner's authorization.

## Dependencies

- Phase 1 → Phase 2 → stories.
- **US1** (Phase 3) needs only T001–T004 (shared vectors and the Neovim helper); it does not need the Lua harness or modules.
- **US2** needs Phase 2. **US3** needs US2's `views.lua` refresh hook (T032) and `cli.lua`. **US4** needs T039's action flow. **US5** needs T014; its sweep of the preset mappings (T049) extends T043 and runs after T048.
- Phase 8 after the stories it documents; T052 keeps the preset block written by T048; T054–T056 last.

## Parallel examples

- Phase 2: T002, T004, T007, T009, T011, T013 in parallel (different files); then T003, T008, T010, T012, T014.
- US1: T015, T017, T019, T021, T023, T025, T027 in parallel; implementations T016, T018, T020, T022 in parallel after their tests; T024 after T016–T022; T029 last.
- US2–US5: each story's test tasks marked [P] in parallel; US5's T045 and T047 can run in parallel with US3 and US4.

## Implementation strategy

1. **MVP:** Phases 1–3. `ai-dev open` and the cockpit's actions are useful without the plugin; stop and validate with T029.
2. Add US2 (read-only views), then US3 (ask with a reference), then US4 (decisions), then US5 (health, preset, removal), each validated by its checkpoint.
3. Phase 8: CI, documentation, gates, quickstart, independent review.
