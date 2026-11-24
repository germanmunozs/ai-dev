# Research: Neovim Integration

Phase 0 of the plan for Feature 014, done on 2026-11-24 without running any coding agent. Sources are Neovim's official documentation and release metadata at the cited tags, the lazy.nvim and LazyVim sources at the cited tags, and probes run with the locally installed Neovim 0.12.5 (`nvim --headless --clean -l <probe>`). Downloading and running the Neovim 0.11.7 binary locally was not permitted in this session, so 0.11.7 facts come from its documentation at tag `v0.11.7` and are verified in CI (R-08).

## R-01 Minimum supported Neovim version

- **Decision:** Neovim **0.11.7** or later. CI tests **0.11.7** (minimum) and **0.12.6** (latest stable, published 2026-10-09).
- **What the plugin and `ai-dev open` need, and since when:**
  - `vim.system()`: present in the 0.11.7 Lua reference (`lua.txt`), added in 0.10.
  - `jobstart(..., {term = true})`: "jobstart() gained the "term" flag" (`news.txt` at v0.11.7). It replaces `termopen()`, deprecated in 0.11. This sets the minimum.
  - `vim.fs.relpath()`: new in 0.11 (`news.txt` at v0.11.7).
  - `vim.health.start/ok/warn/error`, `vim.json`, `sha256()`: present in 0.11.
  - `$NVIM` in the environment of terminal jobs: probed on 0.12.5. Inside a `jobstart(..., {term = true})` job, `$NVIM` equals `v:servername`. Documented at v0.11.7 (`vvars.txt`, `*$NVIM*`): "$NVIM is set to v:servername by terminal and jobstart(), and is thus a hint that the current environment is a child (direct subprocess) of Nvim."
  - RPC: `nvim_get_api_info` (since 0.1.0), `nvim_exec_lua` (since 0.5.0, RPC only), `nvim_cmd` (since 0.8.0), from `api.txt` at v0.11.7.
- **Why 0.11.7 and not 0.11.0:** under the evidence rule of Feature 013 (R-01), what is not tested is not claimed. 0.11.7 is the last 0.11 release (2026-03-28), and CI tests it.
- **Alternatives:** 0.10 (needs `termopen()` and an own `relpath`; two code paths for a release line no longer maintained); 0.12 only (excludes distributions still on 0.11).

## R-02 How `ai-dev open` talks to the parent instance

- **Decision:** a minimal **msgpack-RPC client written with the standard library** in `internal/editor`. **No new Go dependency.**
  - It encodes only requests `[0, msgid, method, params]` with arrays, strings, integers, booleans and nil.
  - It decodes responses `[1, msgid, error, result]` with the msgpack types Neovim returns: nil, bool, int and uint families, float, str, bin, array, map and ext.
  - It reads at most 1 MiB and rejects anything else.
- **Calls, all structured:**
  1. `nvim_get_api_info` checks that the peer is Neovim: a 2-tuple whose metadata map has `version.major/minor/patch` at least 0.11.7.
  2. `nvim_exec_lua(CHUNK, [abs_path, line, column])` runs a **constant** Lua chunk compiled into ai-dev. The path and position travel as RPC values, never inside the code text. The chunk:
     - chooses the window: if the current window shows a terminal buffer, it uses the previous window when that one is not a terminal, else the first non-terminal window in the tab page, else a new split above;
     - opens the file with `nvim_cmd({cmd = "edit", args = {path}, magic = {file = false, bar = false}}, {})`, or `split` when the chosen window's buffer has unsaved changes and `'hidden'` is off;
     - sets the cursor clamped to the buffer, and focuses the window.
- **Probe finding (security):** `nvim_cmd` **expands file names by default**. With `args = {'a %b #c $HOME \`x\` ;d|e.txt'}` and no `magic`, `:edit` fails with `E499` because it expands `%`. Backquotes in an expanded file name run a shell command. With `magic = {file = false, bar = false}` the same name opens literally (probed on 0.12.5). `magic` is mandatory, and a test pins it.
- **Timeouts:** connect 500 ms; each call 1 s; the whole open 2 s. A timeout is "no answer" (F014-FR-003).
- **Alternatives:**
  - `nvim --server $NVIM --remote ...`: it builds Ex text and expands names.
  - `nvim_command("edit " .. path)`: a command string.
  - `github.com/neovim/go-client`: a new dependency of about 2,000 lines for four messages; rejected under the dependency policy.

## R-03 Accepting the `$NVIM` endpoint

- **Decision:** before dialing, `ai-dev open` checks `$NVIM` and refuses it unless:
  - it is an absolute path;
  - `Lstat` reports a **socket** (not a symbolic link, file or directory);
  - the socket is **owned by the current user** (`Stat_t.Uid == os.Getuid()`, available in the standard `syscall` package on darwin and linux).

  A TCP address (`host:port`, used with `--listen 127.0.0.1:…`) is refused as "not a local socket". The checks live in `endpoint_unix.go` (`//go:build darwin || linux`). Other platforms refuse with `platform_unsupported`, keeping the existing Windows cross-build gate green.
- **Rationale:** Neovim creates its sockets in a per-user directory. Probed on macOS: `…/T/nvim.german/` is `drwx------`. A process that can change `$NVIM` already runs as the user (spec Assumptions).
- **Residual, documented:** peer credentials after `connect` (`SO_PEERCRED` / `LOCAL_PEERCRED`) are not checked; the standard library has no portable call for them on darwin. The window between `Lstat` and `connect` needs write access to the user's own socket directory. `nvim_get_api_info` then confirms the peer speaks Neovim's API.

## R-04 Foreground editor

- **Decision:** without `$NVIM`, and with standard input and output being terminals, run `nvim` from the search path through the existing `system.Launcher` with inherited standard streams: `nvim "+call cursor(L, C)" -- <path>`.
  - `L` and `C` are the validated integers, formatted by ai-dev; they are the only Ex text and never come from a person's text.
  - `--` makes the path a file name even if it starts with `-` or `+`.
  - The exit code is Neovim's: 0 means opened.
- **Alternative:** start Neovim with `--listen` and use R-02. It needs a temporary socket and a wait for readiness, for no gain.

## R-05 Installing from a subdirectory with lazy.nvim

- **Finding:** lazy.nvim (v11.17.5) has no subdirectory option. Its documentation for packer's `rtp` says to use `config = function(plugin) vim.opt.rtp:append(plugin.dir .. "/custom-rtp") end`.
- **Decision:** the plugin lives in `nvim/` (`nvim/lua/ai-dev/…`). The documented specification is:

  ```lua
  { "germanmunozs/ai-dev", version = "*", cmd = "AiDev",
    config = function(plugin, opts)
      vim.opt.rtp:append(plugin.dir .. "/nvim")
      require("ai-dev").setup(opts)
    end }
  ```

  - `version = "*"` pins to the latest release tag.
  - `cmd = "AiDev"` loads it on first use; lazy.nvim's stub command then runs the real one.
  - The plugin defines nothing at load time (no `plugin/` directory): everything is in `setup()`, which F014-FR-019 and F014-FR-022 need anyway.
- **Cost, accepted:** lazy.nvim clones the whole ai-dev repository (about 4 MB) to install the plugin.

## R-06 LazyVim preset prefix

- **Decision:** prefix **`<leader>i`**, against **LazyVim v16.0.2** (2026-11-08, the latest release, 16 days old).
- **Evidence:** every `"<leader>…"` string in `lua/lazyvim` at v16.0.2.
  - Default specs use the first keys `- . < ? K L S \` b c d f g l n q s u w x |`.
  - Extras add `" , / : D E G H R a e h m o p r t`. The AI extras take `<leader>a`.
  - `i` is free in both.
- **Test:** the list of LazyVim v16.0.2 leader mappings is stored as a fixture with the command that produced it. A plugin test fails if a preset mapping equals one of them or has one of them as a prefix.

## R-07 Size limit for a code reference

- **Decision:**
  - included selected text: at most **32 KiB** (32,768 bytes);
  - the complete instruction (the person's text, the reference and any included text): at most **64 KiB**.
- **Rationale:** Feature 002 passes the instruction to every catalog agent as **one argument** (013 R-03). Linux limits one argument to `MAX_ARG_STRLEN` = 131,072 bytes, and 64 KiB leaves room. 32 KiB is several hundred lines, enough for a function, small enough to read in the preview.

## R-08 Testing the plugin and installing Neovim in CI

- **Plugin tests:**
  - **Harness:** Lua tests run in headless Neovim with `nvim --headless --clean -l nvim/tests/run.lua`. `run.lua` is a runner of about 60 lines (assertions, per-test isolation, exit code). No test framework (plenary, busted, mini.test) is added.
  - **Isolation:** `vim.ui.input`, `vim.ui.select` and the confirmation are replaced per test, and ai-dev is a fake script that records its argument vector.
  - **End-to-end:** a Go test builds ai-dev, puts fake agents on the search path, runs the Lua end-to-end tests, and checks what the fake agent received (SC-004, SC-005).
- **`ai-dev open` tests:**
  - **Go unit tests:** path validation, msgpack encode and decode with a fuzz target, and endpoint checks.
  - **In-process fake RPC server:** covers a peer that is not Neovim, an old version, no answer and error responses.
  - **Real Neovim:** a Go integration test starts a real headless Neovim with `--listen <socket>`, runs `ai-dev open`, and queries the buffer name and cursor through the same RPC client (SC-001).
- **Skipping:** tests that need Neovim skip when it is absent, unless `AI_DEV_REQUIRE_NVIM=1`, which CI sets.
- **CI install:**
  - **Where (owner decision, 2026-11-24, option A):** inside the existing required job `gates`, on `ubuntu-latest` and `macos-latest`. Two steps install both versions into separate directories, and the Neovim-dependent tests run once per version with that version first on `PATH` and `AI_DEV_REQUIRE_NVIM=1`. Because `gates` is already a required check on `dev` and `main`, these tests block a merge without any change to branch protection.
  - **Download:** it fetches `nvim-linux-x86_64.tar.gz` or `nvim-macos-arm64.tar.gz` from the GitHub release and checks a pinned SHA-256 before extracting.
  - **Pinned digests:** from the GitHub release asset `digest` field on 2026-11-24.

    | Version | Archive | SHA-256 |
    |---|---|---|
    | v0.11.7 | linux-x86_64 | `38a7c6317f94503841096c00e8fde05ef04b9472fc9d7d62b6e033cecd6f7991` |
    | v0.11.7 | macos-arm64 | `9c84686ce84bbab725ff3d88ca3ac1d6da5a72128b68d9dd8094e34d4049a126` |
    | v0.12.6 | linux-x86_64 | `474430d53e6264f6d6dd18db42d6dc9df3a1b56ca9e88a325bbf860e1a811d87` |
    | v0.12.6 | macos-arm64 | `1dbd148222b051ba6c307c4d17a318ed083df018a4f6a5a0ce46955ffc9ab87b` |
  - **Cost, accepted:** `gates` takes a few minutes longer, and a failed Neovim download fails the gate until it is re-run.
  - **Rejected:** a separate `neovim` job. As a required check it needs a branch-protection change, made only after the job has run once, or every PR waits for a check that never reports. As an optional check, a failure would not block a merge.
- **Alternatives:** distribution packages (unpinned, often older than 0.11); a setup action (a new third-party action); a separate job (above).

## R-09 Plugin compatibility with ai-dev

- **Decision:** the plugin declares a minimum ai-dev version, **v0.3.0**: this feature adds a command, so its release is a minor one.
  - It reads `ai-dev version --format json` (`ai-dev.version/v1`) and accepts that minimum or later below v1.0.0.
  - A development build (a pseudo-version or `-dev+`) is accepted, and the health check warns about it.
  - Each JSON the plugin reads must carry the schema it knows: `ai-dev.workflow-guide/v1` and `ai-dev.setup/v1`.
  - Three outputs it reads have no schema field, so the plugin checks their shape and refuses anything else:
    - `doctor --format json`: `overall`, and `checks[]` with `id`, `name` and `status`;
    - `agents --format json`: `agents[]` with `id`, `display_label` and `availability`;
    - `agent select --format json`: `outcome`, `selected_agent_id` and `source`.
- **Listing features:** no ai-dev command lists features. The plugin lists the non-symbolic-link directories under `specs/` (Feature 004's rule) and runs `ai-dev workflow guide --format json -- <id>` for each, at most 8 at a time. An invalid name is reported as ai-dev reports it (`feature_unknown`), not hidden (F014-FR-007).

## R-10 Threats

| Threat | Control | Requirement |
|---|---|---|
| Shell or option injection through names, selections or instructions | Argument vectors only (`vim.system`, `jobstart` with a list, `system.Launcher`); `--` before positional values | F014-FR-002, 014 |
| Ex file-name expansion (`%`, `#`, backquotes run a shell) | `nvim_cmd` with `magic = {file = false, bar = false}`; constant Lua chunk; values as RPC arguments | F014-FR-003, 005 |
| A foreign or spoofed endpoint | Absolute path, `Lstat` socket, owner is the current user, Neovim API check, version check | F014-FR-003 |
| Escape from the project | Both root and target resolved with `EvalSymlinks`; target must stay under root; absolute paths refused; regular files only | F014-FR-002, 009, 011 |
| Unsaved or unconfirmed content leaving the editor | Modified buffers refused; text included only when chosen in the preview; digest recomputed on confirmation | F014-FR-011, 012, 015 |
| Terminal injection from untrusted strings | One Lua sanitizer, the same rules as the cockpit's, with shared test vectors checked by both the Go and the Lua suites | F014-FR-023 |
| Arguments too long | Limits of R-07 | F014-FR-011 |
| Supply chain | No new Go module; Neovim in CI pinned by SHA-256; plugin installed from this repository at a release tag | constitution V |
