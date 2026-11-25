# Implementation Plan: Neovim Integration

**Branch**: `feature/014-neovim-integration` | **Date**: 2026-11-24 | **Spec**: [spec.md](spec.md)

## Summary

Two halves, joined only through ai-dev's public commands.

- **ai-dev → Neovim:** a new command, `ai-dev open [--line N] [--column N] [--format text|json] -- <path>`.
  - It validates that the path resolves to a regular file inside the project.
  - When `$NVIM` names a parent instance, it opens the file there through a minimal standard-library msgpack-RPC client. The client sends a constant Lua chunk whose values travel as RPC arguments, and opens the file with `nvim_cmd` with file-name expansion disabled (R-02).
  - Otherwise it runs `nvim` in the foreground through the existing `system.Launcher`.
  - The cockpit gains "open in editor" actions that run this command through its existing hand-over (A3).
- **Neovim → ai-dev:** a Lua plugin in `nvim/`, released with ai-dev.
  - Its views are built only from the JSON outputs of `workflow guide`, `doctor`, `setup show` and `version`, run with `vim.system`.
  - Its actions run `ai-dev agent run`, `workflow approve`, `workflow abandon` and `tui` in a Neovim terminal (`jobstart` with an argument list), after a preview and an explicit confirmation.
  - It writes nothing, keeps no state, and starts no process other than ai-dev.

## Technical Context

- **Languages and dependencies:**
  - **Go:** 1.27.4, standard library only. No new module.
  - **Lua:** the plugin targets Neovim 0.11.7 and later (R-01), using only Neovim's runtime, with no Lua dependency.
  - **CI tool:** Neovim, pinned by version and SHA-256 (R-08).
- **Storage:** none.
  - `ai-dev open` writes nothing.
  - The plugin writes no file and keeps nothing between sessions (F014-FR-022).
- **Testing:**
  - `internal/editor`:
    - path validation: traversal, absolute paths, symbolic links leaving the project, directories, missing files, spaces and Unicode;
    - msgpack: encode and decode tables, and a fuzz target for the decoder;
    - endpoint checks: a regular file, a directory, a symbolic link to a socket, a TCP address, and a socket whose owner differs, the owner being injected;
    - the RPC client against an in-process fake server: not Neovim, version below 0.11.7, no answer, an error response, an oversized response;
    - a test that the Lua chunk passes `magic = {file = false, bar = false}`.
  - `internal/app`:
    - `ai-dev open` outcomes, exit codes and JSON;
    - the foreground argument vector through a fake `Launcher`;
    - the cockpit's "open in editor" actions: availability and argument vector;
    - catalog, help, usage and completion for `open` (A1, A2).
  - **Real Neovim** (skipped without Neovim unless `AI_DEV_REQUIRE_NVIM=1`):
    - a headless Neovim started with `--listen`, opened by the built binary, with the buffer and cursor checked through the RPC client (SC-001);
    - hostile file names opened literally.
  - **Lua tests** (`nvim/tests/`, run in headless Neovim by `nvim/tests/run.lua`, driven from `go test`):
    - views against a fake ai-dev that prints fixture JSON (SC-003);
    - references, previews, confirmations and refusals;
    - sanitizer and quoting vectors shared with the Go suites (SC-007);
    - every command and preset mapping run without confirming (SC-006);
    - preset prefix against the LazyVim v16.0.2 fixture (R-06);
    - configuration validation;
    - health check;
    - removal.
  - **End-to-end** (Go test → headless Neovim → built ai-dev → fake agents):
    - the fake agent receives exactly the confirmed instruction (SC-004, SC-005).
- **Target platforms:** macOS and Linux. `ai-dev open` on other platforms refuses (`platform_unsupported`); the plugin is untested there.
- **Project type:** CLI plus an editor plugin in the same repository.
- **Performance:** SC-001 (1 s to open in the parent); SC-008 (views never block editing for more than 100 ms; 50 features within 2 s, with at most 8 ai-dev processes at a time).
- **Constraints:**
  - Existing commands, outputs and cockpit behavior stay unchanged except A1–A3.
  - Argument vectors only, never a shell.
  - Untrusted text is always sanitized before display.

## Constitution Check

| Principle | Status |
|---|---|
| I. Specification First | PASS: spec approved by the owner on 2026-11-24, with Amendments A1–A3 applied (`590af8d`) |
| II. Architecture Before Framework | PASS: no framework. Each transport follows from a requirement: the JSON outputs that exist (FR-007), the parent instance or a foreground editor (FR-003, FR-004), and argument vectors (FR-014) |
| III. Simplicity and YAGNI | PASS with justified complexity: an own msgpack-RPC subset (about 250 lines) instead of `neovim/go-client`, because four messages do not justify a dependency (R-02). The simpler alternative, `nvim --remote`, was rejected because it builds Ex text that expands file names (R-02). There is no Lua test framework (R-08) and no daemon, socket of ai-dev's own, or registry |
| IV. Testable by Design | PASS: pure Go units; a fake RPC server; real headless Neovim in CI on 2 versions × 2 platforms; Lua tests in headless Neovim; end-to-end with fake agents |
| V. Security by Design | PASS: threats and controls in R-10. They include the Ex file-name expansion found by probing (R-02), endpoint ownership (R-03), the residual of unchecked peer credentials, and the size limits (R-07). No new Go dependency; Neovim pinned by SHA-256 in CI |
| VI. Observability by Design | PASS: every view shows its evaluation time; every action shows its exact command and exit code; `ai-dev open` reports its outcome, reason and target; the health check reports versions and compatibility |
| VII. Reproducibility | PASS: Neovim, LazyVim (as a fixture) and every external fact are pinned to versions and digests; tests run with `go test` |
| VIII. AI-Assisted Engineering | PASS: independent review before the PR, as in 012 and 013 |
| IX. Human Authority | PASS: decisions are recorded only by the existing commands, after a preview and an explicit confirmation of that one action, with no human value prefilled. The Neovim tests run inside the required `gates` job, so no branch-protection change is needed |

Post-design re-check: PASS. The only changes to existing behavior are `open` in the catalog, help and usage (A1, A2), and the cockpit's "open in editor" actions (A3).

## Design

- **`internal/editor`** (new; standard library only):
  - `location.go`: `Resolve(root, path string, line, column int) (Location, error)`.
    - It refuses absolute paths.
    - It resolves both root and target with `filepath.EvalSymlinks`, and requires the target under the root and a regular file.
    - Reasons: `absolute_path`, `outside_project`, `not_found`, `not_regular_file`.
  - `msgpack.go`: the encoder and decoder subset of R-02.
  - `rpc.go`: `Dial(endpoint) (*Client, error)`, `Client.Call(method string, params ...any) (any, error)`, with the timeouts of R-02.
  - `endpoint_unix.go` / `endpoint_other.go`: `CheckEndpoint(addr string) error` (R-03), with the owner check behind a variable for tests.
  - `open.go`:
    - `OpenInParent(client, loc)`: checks the version with `nvim_get_api_info`, then sends `nvim_exec_lua(openChunk, {abs, line, column})`;
    - `openChunk`: a constant;
    - `ForegroundArgs(loc) []string`.
- **`internal/app/open.go`** (new): `RunOpen`.
  - It parses arguments; a line or column below 1, or a missing path, exits 2 with the usage line.
  - It resolves the location against the current directory.
  - It chooses the target: `$NVIM` → parent; else terminals and `nvim` on PATH → foreground; else `editor_unavailable`.
  - It writes the text or JSON report (`contracts/open.md`) and maps outcomes to exit codes.
- **`internal/app/{app,catalog,help,completion}.go`:** route `open`; add the catalog entry with help and completion (`--line`, `--column`, `--format`, file names after `--`); `open` in the top-level usage and the general help (A1, A2).
- **`internal/cockpit`:**
  - `FeatureRow` gains `Files []string`: the raw names of the regular files directly in the feature's directory (no symbolic links or subdirectories), filled by `internal/app`.
  - `actions.go` gains "open spec", "open plan" and "open tasks", each available only when the file exists and needing no value, and "open file…", with one typed `file` field whose hint lists the feature's regular files and whose value must be one of them. Each runs `ai-dev open -- <path>` through the existing review, confirmation and hand-over (A3, contracts/open.md).
  - No other area, key or action changes.
- **`nvim/`** (new; `contracts/plugin.md`):
  - `lua/ai-dev/init.lua`: `setup(opts)`, the `:AiDev` command with subcommands and completion.
  - `lua/ai-dev/config.lua`: defaults and validation (F014-FR-020).
  - `lua/ai-dev/cli.lua`: runs ai-dev with `vim.system` (argument list, `cwd` = Neovim's working directory, text mode); decodes JSON; checks schemas and versions (R-09).
  - `lua/ai-dev/views.lua`: features, environment and agents in read-only scratch buffers, with their evaluation time; buffer-local keys inside those buffers only.
  - `lua/ai-dev/reference.lua`: the Code Reference (data-model.md), with the project check, the unsaved-buffer refusal, the SHA-256 digest of the saved file read from disk, and the size limits (R-07).
  - `lua/ai-dev/actions.lua`: the five actions; collects values with `vim.ui.input` and `vim.ui.select` with no default; shows the preview; confirms; checks the digest again; runs `jobstart(argv, {term = true, cwd = root})` in a new tab; shows the exit code and refreshes the open views.
  - `lua/ai-dev/sanitize.lua` and `lua/ai-dev/quote.lua`: the same rules as `cockpit.Sanitize` and `report.ShellInertQuote`.
  - `lua/ai-dev/health.lua`: `:checkhealth ai-dev`.
  - `lua/ai-dev/lazyvim.lua`: the preset, with mappings under `<leader>i`.
  - `tests/run.lua`, `tests/*_test.lua`, `tests/fixtures/`.
  - Shared vectors: `testdata/sanitize-vectors.json` and `testdata/quote-vectors.json` at the repository root, read by both the Go and the Lua tests.
- **`.github/workflows/ci.yml`:** the existing job `gates` (`ubuntu-latest`, `macos-latest`) gains two steps.
  - Install Neovim 0.11.7 and 0.12.6: download the pinned archives, check the SHA-256 of R-08, and extract each into its own directory.
  - Run the Neovim-dependent Go tests once per version, with that version's `bin` first on `PATH` and `AI_DEV_REQUIRE_NVIM=1`.
  - No new job and no change to branch protection (owner decision, 2026-11-24). The other jobs are unchanged.
- **Documentation:**
  - README and README.es: `ai-dev open`, the plugin (installation with the lazy.nvim spec of R-05, the LazyVim preset, configuration, supported versions, privacy boundary, troubleshooting, removal), and the architecture.
  - CHANGELOG and CHANGELOG.es, under `[Unreleased]`.
  - SECURITY: the `$NVIM` trust model and its residual.

## Owner decisions on the plan (2026-11-24)

German Munoz, repository owner, confirmed these on 2026-11-24 ("si, opción A y confirmo la 1 y la 2"), choosing among options presented with alternatives. This is a declared, unauthenticated assertion.

- **F014-FR-019, "no key mappings" without the preset (interpretation 1, confirmed).** The plan reads it as no **global** mappings and no mappings in the person's buffers. The plugin's own read-only view buffers have buffer-local keys (`r`, `<CR>`, `n`, `a`, `x`, `q`). They exist only in those buffers and disappear with them. Without them a view could not be navigated from the keyboard.
- **F014-FR-002, "an absolute path" (interpretation 2, confirmed).** Every absolute path is refused, even one inside the project. Callers (the cockpit, the person) pass project-relative paths. This keeps the reports free of absolute paths (F014-FR-025).
- **Where the Neovim tests run in CI (option A).** They run inside the existing required job `gates`, not in a separate job, so they block a merge without any change to branch protection (R-08). Rejected: a separate job made required, which needs a branch-protection change after its first run, and a separate optional job, whose failure would not block a merge.

After `/speckit-analyze`, the owner approved on 2026-11-24 ("si, apruebo I3-A, A1 y U1, aplica los cambios"), with the same caveat:

- **F014-FR-006, "each artifact" (I3-A).** Besides "open spec", "open plan" and "open tasks", the cockpit offers "open file…" for any regular file of the feature's directory. It uses a typed field validated against the listed files, since the cockpit has no selection input (contracts/open.md).
- **`--` is required by `ai-dev open` (A1).** A path without `--` is a usage failure, as the cockpit and the plugin always pass it.
- **LazyVim installation is a manual check (U1).** The automated tests check the preset against the pinned list of LazyVim v16.0.2 leader mappings, not an installed LazyVim, because CI installs no third-party plugins over the network. quickstart.md has the manual step, done by the owner.

## Project Structure

```text
specs/014-neovim-integration/
  plan.md, research.md, data-model.md, quickstart.md, contracts/open.md, contracts/plugin.md
internal/editor/{location,msgpack,rpc,endpoint_unix,endpoint_other,open}.go, *_test.go   NEW
internal/app/open.go, open_test.go, open_nvim_test.go                                     NEW
internal/app/{app,catalog,help,completion,cockpit}.go          MODIFIED (route, catalog, usage, artifacts)
internal/cockpit/{data,actions}.go                             MODIFIED (Files, "open" actions)
nvim/lua/ai-dev/{init,config,cli,views,reference,actions,sanitize,quote,health,lazyvim}.lua   NEW
nvim/tests/run.lua, nvim/tests/*_test.lua, nvim/tests/fixtures/                           NEW
cmd/ai-dev/nvimplugin_test.go                                  NEW (drives the Lua and end-to-end tests)
internal/testutil/nvim/nvim.go, nvim_test.go                   NEW (test helper: finds Neovim, skips or fails)
testdata/{sanitize,quote}-vectors.json                         NEW (shared by Go and Lua)
.github/workflows/ci.yml                                       MODIFIED (Neovim steps in gates)
tests pinning usage, general help, catalog, completion          UPDATED (A1, A2)
README.md, README.es.md, CHANGELOG.md, CHANGELOG.es.md, SECURITY.md   UPDATED
```

## Quality Gates

| Gate | How | Pass condition |
|---|---|---|
| Format | `gofmt -l .` | no output |
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test -race ./...`; in `gates`, `go test -race -run '^TestNvim' ./...` once per version with `AI_DEV_REQUIRE_NVIM=1` | exit 0; no Neovim test skipped in `gates` |
| Fuzz | `go test -fuzz FuzzDecode -fuzztime 20s ./internal/editor` in CI's existing fuzz step | no failure |
| Static analysis | `staticcheck ./...` for linux and darwin (CI `static-analysis`) | exit 0 |
| Vulnerabilities | `govulncheck ./...` (CI `static-analysis`) | no reachable vulnerability |
| Dependencies | `go.mod` unchanged; Neovim archives match their pinned SHA-256 | pass |
| Build | `go build ./cmd/ai-dev` for darwin, linux, windows, freebsd (existing cross-build) | exit 0 |
| Plugin | Lua tests on Neovim 0.11.7 and 0.12.6, macOS and Linux | all pass |
| Performance | SC-001 and SC-008 measured in tests | 1 s; 100 ms; 2 s |
| Regression | suites of Features 001–013 | pass with only A1–A3 expectations updated (SC-010) |
