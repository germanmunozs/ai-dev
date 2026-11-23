# Research: Agent Catalog Expansion

All verification was done on 2026-11-22. No coding agent was run with an instruction (F013-FR-004, SC-005). The evidence comes from:
- the vendor's documentation;
- the agent's own `--help` (Gemini CLI 0.70.0 and GitHub Copilot CLI 1.0.114 are installed on the maintainer's machine);
- the agent's published package or release, downloaded into a scratch directory and inspected without running it;
- the agent's public source, when it has one.

## R-01 Evidence policy

A criterion of F013-FR-003 counts as met only with a source for it. When the source is silent and the agent's code is closed, the criterion is **unverified**. Unverified is not met: the candidate stays out, and its entry names the missing evidence, so a later verification can admit it without a new feature (F013-FR-001, SC-008).

| Criterion | Accepted evidence |
|---|---|
| 1. Documented non-interactive mode | Vendor documentation or `--help` |
| 2. Instruction cannot be read as an option | (a) The vendor documents `--` before the prompt; or (b) the documented prompt form (positional after a boolean mode flag, or a string-valued long option) together with the argument parser identified in the published package or source, and that parser's behavior checked (R-03) |
| 3. No relaxing option needed | The documentation or source shows that the non-interactive mode, started with no extra option, denies, asks about or refuses actions that need approval, and that auto-approval is an explicit opt-in (R-02) |
| 4. Exit status | The documentation or source shows 0 for the agent's own completion and non-zero for its failure |
| 5. No extra argument | The documented invocation needs only the mode selector and the instruction. Authentication through the agent's own login, configuration or environment is allowed (002 Assumptions) |
| 6. Distinctive executable | No Homebrew formula, or other widely distributed package, installs a different program under the same name (R-04) |
| 7. macOS or Linux | Vendor documentation or published platform packages |

## R-02 Criterion 3: a mode that approves by itself is a relaxed mode

- **Decision:** 002 FR-016 forbids invoking an agent so that it "enable[s] an unrestricted execution mode". A non-interactive mode that, by default, approves actions its interactive mode would ask about **is** such a mode, even when ai-dev adds no option. A mode whose default is to deny or ask is accepted. An agent whose interactive mode already approves everything natively (no approval system at all) is judged by the same rule: the non-interactive mode must not be more permissive than the interactive one.
- **Rationale:** otherwise `ai-dev agent run` would silently give a person's instruction more authority than the same agent typed by hand. That is exactly what FR-015 and FR-016 exclude.
- **Consequence:** Cline, Crush, Continue (`cn`) and Kimi Code are excluded (R-05).

## R-03 Criterion 2: how the parsers were checked

- **Identification:** parser signatures found in the published code:
  - `yargs-parser` / `populate--` / `halt-at-non-option` (yargs);
  - `commander` with `allowUnknownOption` and `passThroughOptions`;
  - `clap_builder` (Rust clap);
  - `com/github/ajalt/clikt` (Kotlin Clikt);
  - otherwise, the hand-written parser itself was read.
- **Behavior check:** the libraries were run directly, in a scratch Node project with `yargs-parser@21`, `yargs@18` and `commander@14`, over the instructions `-x a b`, `--help`, `--yolo`, `--`, `-`, `--prompt=y`, `a=b`, `123` and the empty string:
  - **yargs-parser:** `--prompt=<text>` gives exactly `<text>` for a string option, with and without `unknown-options-as-args` and `halt-at-non-option`.
  - **commander:** `--print -- <text>` gives the positional `<text>`, and `--prompt=<text>` gives the option value `<text>`. Both also hold for a subcommand with `passThroughOptions` and `allowUnknownOption`, the configuration Factory Droid uses.
  - **yargs commands:** an argument after `--` does **not** fill a command's positional parameter (`run [message..]`). It goes to `argv["--"]` (with `populate--`) or `argv._`. A yargs agent that takes a positional prompt is safe with `--` only when its code reads `argv["--"]`. OpenCode does (R-05).
  - **Rust clap and Kotlin Clikt** both end option parsing at `--` and bind `--name=value` verbatim. These are documented properties of the libraries; their test runs were not repeated here.
- **Residual:** an agent may pre-process its arguments before its parser runs. The Cursor CLI does, and fails for that reason. For the admitted agents, the code that was read shows no such step before the parser. A later version could add one; F013-FR-004 records the version verified.

## R-04 Criterion 6: name collisions

`brew info` was run for every candidate executable. These names are installed by other, widely distributed programs:

| Executable | Other program |
|---|---|
| `copilot` | AWS Copilot CLI (Amazon ECS and Fargate), Homebrew formula `copilot` |
| `goose` | pressly/goose, database migrations, formula `goose` |
| `amp` | Amp text editor, formula `amp` |
| `forge` | Foundry's `forge` (Ethereum toolchain) and formula `forge` (arrayfire) |
| `grok` | jordansissel/grok, formula `grok` |
| `bob` | Neovim version manager, formula `bob` |
| `dsh` | Dancer's distributed shell, formula `dsh` |

Not collisions:
- **`gemini`:** the Homebrew cask of that name is a GUI application, MacPaw Gemini, with no command-line binary.
- **`kimi`, `muse`, `zcode` and `codebuddy`:** the casks of those names are GUI applications.
- **`kiro-cli`, `droid`, `junie`, `opencode`, `aider` and `cline`:** the Homebrew entries are the agents themselves.

## R-05 Admission record

**Owner decisions on this record (2026-11-22).** German Munoz, repository owner, chose the following in the words "mantén el criterio 6 y deja los 13 pendientes". This is a declared, unauthenticated assertion.
- Criterion 6 stays as written. GitHub Copilot CLI, Amp, Goose, Forge, Grok Build, IBM Bob and DeepSeek Harness stay out.
- The 13 candidates with missing evidence stay out of this feature. They are left for a later extension under the same criteria (SC-008).

Legend: ✓ met with evidence · ✗ fails · ? unverified (counts as not met, R-01).

### Admitted

| Identifier | Label | Executable | Version verified | Invocation (instruction = `I`) |
|---|---|---|---|---|
| `claude-code` | Claude Code | `claude` | 2.1.311 (002 Amendment 1) | `claude --print -- I` (unchanged) |
| `codex-cli` | OpenAI Codex CLI | `codex` | 0.166.0 (002 Amendment 1) | `codex exec -- I` (unchanged) |
| `factory-droid` | Factory Droid | `droid` | 1.17.2 (npm `@factory/cli`) | `droid exec -- I` |
| `gemini-cli` | Gemini CLI | `gemini` | 0.70.0 (installed) | `gemini --prompt=I` |
| `kiro-cli` | Kiro CLI | `kiro-cli` | 3.5.0 (release archive) | `kiro-cli chat --no-interactive -- I` |
| `mistral-vibe` | Mistral Vibe | `vibe` | 3.1.4 (GitHub release) | `vibe --prompt=I` |
| `opencode` | OpenCode | `opencode` | 1.19.6 (npm `opencode-ai`) | `opencode run -- I` |
| `qwen-code` | Qwen Code | `qwen` | 0.28.4 (npm `@qwen-code/qwen-code`) | `qwen --prompt=I` |

Evidence for each new agent, by criterion:

- **Factory Droid:**
  1. "Usage: droid exec [options] [prompt]" (docs.factory.com/cli/droid-exec/overview) ✓
  2. `commander` with `passThroughOptions` in the darwin-arm64 binary, checked in R-03 ✓
  3. "By default the run is read-only"; `--auto low|medium|high` is the opt-in ✓
  4. "exits 0 when the run completes and non-zero when it stops early" ✓
  5. ✓
  6. Homebrew cask `droid` is this agent ✓
  7. macOS and glibc Linux ✓
- **Gemini CLI:**
  1. "Use -p/--prompt for non-interactive (headless) mode" (`gemini --help`, geminicli.com/docs/cli/headless) ✓
  2. `-p, --prompt [string]`; yargs-parser in the installed bundle; `--prompt=I` checked in R-03 ✓
  3. Policy engine: an `ask_user` decision "In non-interactive mode, this is treated as `deny`"; default approval mode is `default` ✓
  4. Exit codes 0, 1, 42 and 53 (headless reference) ✓
  5. ✓
  6. The `gemini` cask has no binary ✓
  7. Homebrew and npm on macOS and Linux ✓
  - Note: the documentation says Antigravity CLI replaces Gemini CLI for unpaid tiers from 2026-06-18. Gemini CLI is still published (0.70.0) and is admitted as such; a later removal would be handled as catalog maintenance.
- **Kiro CLI:**
  1. `kiro-cli chat --no-interactive "<prompt>"` (kiro.dev/docs/cli/headless) ✓
  2. Positional prompt; `clap_builder` in the 3.5.0 binary; `--` ends options ✓
  3. Untrusted tool calls are denied: "tool permission approval is not supported in non-interactive mode"; `--trust-all-tools` and `--trust-tools` are opt-in ✓
  4. 0 success, 1 failure, 3 and 4 specific failures, non-zero when stdout closes ✓
  5. Headless authentication is the agent's own `KIRO_API_KEY` ✓
  6. Cask `kiro-cli` is this agent ✓
  7. macOS universal archive; Linux in the documented CI example ✓
- **Mistral Vibe:**
  1. "Use the `--prompt` (`-p`) flag to run a single task and exit" (docs.mistral.ai/vibe/code/cli/work-with-cli) ✓
  2. Rust clap `#[arg(short = 'p', long = "prompt", num_args = 0..=1)]` (`vibe/cli-rust/src/cli.rs`); `--prompt=I` binds verbatim ✓
  3. "Nobody can answer an approval prompt, so Vibe denies any tool call that needs one"; `--auto-approve` is opt-in ✓
  4. 0 finished, 1, 2 and 3 failures ✓
  5. ✓
  6. No other `vibe` package found ✓
  7. macOS and Linux releases ✓
  - Note: earlier documentation said programmatic mode auto-approved by default. Version 3.1.4 is the version verified.
- **OpenCode:**
  1. `opencode run` "designed for scripts, CI jobs" (opencode.ai/docs/cli) ✓
  2. `run [message..]` in yargs; the v1.19.6 source builds the message from `[...args.message, ...(args["--"] || [])]` (`packages/opencode/src/cli/cmd/run.ts`), and the binary contains `populate--` ✓
  3. Without `--auto`, `--yolo` or `--dangerously-skip-permissions`, a `permission.asked` event is answered "auto-rejecting" (same file) ✓
  4. `process.exit(1)` on every error path of `run` ✓
  5. ✓
  6. Formula `opencode` is this agent ✓
  7. darwin and linux packages ✓
- **Qwen Code:**
  1. "Use the `--prompt` (or `-p`) flag to run in headless mode" (qwenlm.github.io/qwen-code-docs) ✓
  2. yargs-parser in the 0.28.4 bundle; string option `prompt` ✓
  3. "when running headless commands, Ask Permissions Mode is the default"; `--yolo` and `--approval-mode` are opt-in ✓
  4. In v0.28.4, `packages/cli/src/llm.tsx` ends a headless run with `process.exit(exitCode)`, where `runNonInteractive` (`nonInteractiveCli.ts`) returns 0 on completion, 1 on failure and 130 on interruption; the documentation adds failure codes 53 and 55 ✓
  5. ✓
  6. No other `qwen` package found ✓
  7. npm on macOS and Linux ✓

### Excluded, failing a criterion

| Candidate | Executable | Fails | Evidence |
|---|---|---|---|
| GitHub Copilot CLI | `copilot` | 6 | AWS Copilot CLI installs `copilot` (R-04). Otherwise strong: `-p/--prompt`, commander, requests needing approval denied ("Without any permission options, requests that need approval are denied") |
| Amp | `amp` | 6 | Amp text editor (R-04) |
| Goose | `goose` | 6 | pressly/goose (R-04) |
| Forge | `forge` | 6 | Foundry `forge` (R-04) |
| Grok Build | `grok` | 6, 3 | jordansissel/grok (R-04); Spec Kit adds `--always-approve` |
| IBM Bob | `bob` | 6, 3 | Neovim `bob` (R-04); Spec Kit adds `--trust --accept-license` |
| DeepSeek Harness | `dsh` | 6 | Dancer's shell (R-04) |
| Cursor CLI | `cursor-agent` | 2 | Cursor forum report: the positional prompt is scanned for `-p`/`-P` and `--` is ignored. Without `--force`, edits are only proposed (acceptable), but criterion 2 fails |
| Pi Coding Agent | `pi` | 2 | Hand-written parser (`dist/cli/args.js`, 0.73.1): no `--`. An argument starting with `-` is a flag, and one starting with `@` is a file. An instruction like `-x` can never be the message |
| SHAI | `shai` | 2 | The prompt is documented only on standard input (`echo … \| shai`). 002 kept standard input for the agent's terminal (Amendment 1 residual) |
| Aider | `aider` | 4 | `--message` returns normally after `coder.run` (`aider/main.py`, v0.86.0), so a failed run exits 0 |
| Cline | `cline` | 3 | Headless "all actions are auto-approved"; `--auto-approve` defaults to `true` |
| Crush | `crush` | 3 | `crush run` "does not prompt for permissions" (upstream PR evidence) |
| Continue CLI | `cn` | 3 | The headless default policy allows Bash (CVE-2026-76072 advisory) |
| Kimi Code | `kimi` | 3 | "non-interactive mode uses auto permission by default"; older `--print` "implicitly enables --yolo" |
| Docker Agent | `docker-agent` | 5 | Needs an agent configuration reference argument |
| Tabnine CLI | `tabnine` | 1 | No flag reference for `-p`. The documentation says CLI users are being moved to a plugin for OpenCode |
| ZCode | `zcode` | 1 | No documented non-interactive flag for the official binary |
| Alquimia AI | `alquimia` | 1 | No documentation found |
| Firebender, Lingma, Trae, Zed, Kilo Code (IDE) | — | 1 | Editor-only in Spec Kit, with no command-line program |
| Generic | — | — | Out of scope by the owner's decision (spec Clarifications) |

### Excluded, evidence missing (candidates for a later verification)

| Candidate | Executable | Unverified | What is known |
|---|---|---|---|
| Devin for Terminal | `devin` | 4 | `devin -p -- …` documented (criterion 2 ✓); `-p` denies tool calls that need approval (changelog) |
| Auggie CLI | `auggie` | 4 | `auggie --print "…"`, commander; `ask-user` "Without an interactive UI, the call is denied" |
| Qoder CLI | `qodercli` | 4 | `-p` print, commander; "any operation that requires confirmation defaults to deny" |
| CodeBuddy Code | `codebuddy` | 4 | `-p`, commander; operations needing authorization are blocked without `-y` |
| Junie | `junie` | 3, 4 | Positional prompt, Clikt (criterion 2 ✓); headless approval behavior and exit codes undocumented |
| Rovo Dev | `acli rovodev run` | 2, 4 | `acli` parser not identified; normal mode "asks permission" (implied) |
| iFlow CLI | `iflow` | 3 | `--prompt`, yargs; default approval in non-interactive runs undocumented |
| Kilo Code CLI | `kilo` | 2, 3 | `kilo run [message..]`, fork of OpenCode; its handling of `--` and of asks was not read |
| Antigravity CLI | `agy` | 2, 3 | Official headless page not readable; parser not identified; reports of output loss off a terminal |
| Muse Code | `muse` | 1 | Beta; official documentation not reachable, only third-party guides |
| Hermes Agent | `hermes` | 1 | `hermes chat -q` only in third-party guides |
| Command Code | `command-code` | 1, 3 | `-p` documented; permission default and executable name unclear |
| Oh My Pi | `omp` | 2 | Derived from Pi, whose parser fails criterion 2; its own parser was not read |

## R-06 Design: the catalog as data

- **Decision:**
  - `internal/agent` keeps the identity part of each entry (identifier, label, executable) in the ordered catalog of F013-FR-005.
  - `internal/execution/adapters` replaces the two vendor types with one data-driven adapter, `Invocation{Executable, Mode []string, Form, Option string}`, and a table `Invocations` keyed by agent identifier.
  - `Form` is `AfterEndOfOptions` (`<mode…> -- I`) or `AttachedOption` (`<mode…> <Option>=I`).
  - `Classify` is the shared 0 → `agent_succeeded`, other → `agent_failed` rule that both existing adapters already use.
  - `internal/app/agentrun.go` builds the runner's adapter map from the table.
- **Rationale:**
  - Adding an agent becomes one catalog line and one table line, with the tests of R-07 (SC-008).
  - The existing argument vectors are reproduced exactly: `claude --print -- I` and `codex exec -- I`.
  - The `Runner`, the `Adapter` interface and every report stay as they are (F013-FR-008).
- **Why not merge the invocation into `internal/agent`:** that package is documented as vendor-neutral and "without execution semantics"; selection, setup and the cockpit depend on it and need no invocation data.
- **Alternatives considered:** one Go type per agent, as today (rejected: duplication, and the shape is the same for every agent); a data file read at run time (rejected: the catalog is fixed at build time, and a file would be a new input that could name an executable, against F013-FR-013).

## R-07 Tests that guard the catalog

- **Drift:** every catalog agent has exactly one invocation and every invocation belongs to a catalog agent. The invocation's executable equals the catalog executable.
- **Order:** `claude-code`, `codex-cli`, then ascending identifiers (F013-FR-005). Identifiers match `^[a-z0-9]+(-[a-z0-9]+)*$` and are distinct (F013-FR-002).
- **Instruction safety (SC-003):** for every agent and for the instructions `-x a b`, `--help`, `-h`, `--version`, `-p`, `--yolo`, `--dangerously-skip-permissions`, `--`, `-` and `a=b`:
  - with `AfterEndOfOptions`, the last argument is the instruction and the only `--` comes right before it;
  - with `AttachedOption`, the last argument is `<Option>=` followed by the instruction, and no `--` appears.
- **No relaxing option (SC-004):** every argument other than the instruction is checked, case-insensitively, against a denylist: `yolo`, `dangerous`, `skip-permission`, `bypass`, `trust`, `approve`, `allow-all`, `auto`, `force`, `full-auto`, `accept-license`, `sandbox`, `permission`. Only the mode selectors in the table may appear (F013-FR-007).
- **Fake-executable runs:** for every agent, a fake executable on `PATH` records its arguments and `ai-dev agent run <id> "-x a b"` reproduces the table exactly (SC-002, SC-003).

## R-08 Doctor's aggregate check

- **Decision:** `checks.NewCodingAgentCheck(executor)`, identifier `coding-agent-tool`, name "Coding agent", area `Environment`, evaluated over `agent.NewRegistry(executor).List()`.
  - **Pass:** the explanation is `Available: claude-code, gemini-cli`, using identifiers in catalog order.
  - **Fail:** the explanation is `No supported coding agent was found on PATH.` and the remediation is `Install a supported coding agent; run ai-dev agents to see the list.`
  - A panic during lookup fails the check, as the existing tool check does.
- **Position:** third, where `claude-code-tool` was. `codex-cli-tool` is removed. Doctor reports eight checks.
- **Rationale:** F013-FR-009. Readiness stays "every check passes".
