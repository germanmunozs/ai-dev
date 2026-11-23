# Changelog

**English** · [Español](CHANGELOG.es.md)

All notable changes to ai-dev are recorded in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/). While the major version is 0, any minor version may change the CLI interface or the JSON formats.

[`CHANGELOG.es.md`](CHANGELOG.es.md) is the Spanish translation of this file and is kept in step with it. When a version is published, the `[Unreleased]` section of both becomes `[X.Y.Z] - YYYY-MM-DD`. The release workflow uses this file's section as the release notes and fails if it is missing.

## [Unreleased]

### Added

- **013 Agent Catalog Expansion:** six more coding agents can be detected, configured for a role and run with `ai-dev agent run`: Factory Droid (`factory-droid`), Gemini CLI (`gemini-cli`), Kiro CLI (`kiro-cli`), Mistral Vibe (`mistral-vibe`), OpenCode (`opencode`) and Qwen Code (`qwen-code`). Each one met published admission criteria with evidence. ai-dev invokes each in its own non-interactive mode, with the instruction never readable as an option and with no option that relaxes its approvals, permissions or sandbox. The admission record, with every candidate evaluated, is in `specs/013-agent-catalog/research.md`.

### Changed

- `ai-dev doctor` has eight checks: `claude-code-tool` and `codex-cli-tool` are replaced by `coding-agent-tool`, which passes when any supported agent is installed and names them (`Coding agent: claude-code, gemini-cli`). Programs that read the doctor JSON report by check identifier must use the new identifier.
- `ai-dev agents`, `setup show`, the setup assistant, help, shell completion and the cockpit list the eight agents.
- `scripts/install-dev.sh` marks development builds as `v0.1.2-dev+<commit>` instead of `v0.1.0-dev+<commit>`.

## [0.1.1] - 2026-11-22

### Changed

- The public documentation (README, CHANGELOG, CONTRIBUTING and SECURITY) is in English and Spanish. English stays in the usual files and Spanish moves to `*.es.md`.

### Fixed

- `scripts/install.sh` failed on macOS under a UTF-8 locale (`arch…: unbound variable`): macOS `sh` read the `…` character as part of the variable name.
- Release binaries reported `modified` in `ai-dev version` because the archives already written to `dist/` counted as uncommitted changes; `dist/` is now ignored.

## [0.1.0] - 2026-11-22

First release.

### Added

- **001 Environment Doctor:** `ai-dev doctor` runs nine checks to tell whether the environment (Git, Spec Kit, Claude Code and Codex CLI) and the repository (Spec Kit, constitution and integrations) are ready.
- **002 Coding Agent Execution:** `ai-dev agents` lists the supported agents and `ai-dev agent run` runs one (Claude Code or Codex CLI) with an explicit instruction, without raising permissions or bypassing its sandbox.
- **003 Agent Role Selection:** `ai-dev agent select` chooses the right agent for each role in the SDD process.
- **004 Workflow State:** `ai-dev workflow state` and `ai-dev workflow transition` evaluate a feature's lifecycle state and whether a transition is valid, without changing anything.
- **005 SDD Workflow:** `ai-dev workflow next` recommends the next SDD activity (read-only).
- **006 Human Gates:** `ai-dev workflow approve` and `ai-dev workflow abandon` record explicit human decisions in `workflow-state.json`, with an atomic write and locking.
- **007 Workflow Orchestration (guided mode):** `ai-dev workflow guide` shows the next step with command templates that are safe to paste into a shell. Autonomous mode is deferred.
- **008 Git Management:** `ai-dev init` inspects, plans, applies and reconciles the supervised initialization of a Git repository, with explicit authorization, a durable audit trail and crash recovery; `ai-dev git commit validate` validates Conventional Commits messages.
- **009 CLI Help and Version:** `ai-dev help`, `ai-dev --help` and `-h` list the commands; `ai-dev version` and `--version` report the version, commit, Go version and platform, as text or JSON (`ai-dev.version/v1`).
- **010 Command Help and Shell Completion:** `ai-dev <command> --help`, `-h` and `ai-dev help <command>` show the help for one command or group (usage, arguments, options, what it changes and exit codes); `ai-dev completion bash|zsh` writes a static completion script.
- **011 Agent Detection and Role Configuration:** `ai-dev setup` (wizard), `ai-dev setup show` and `ai-dev setup apply` detect the catalog's agents, choose which ones the project works with and assign one to each role in `.ai-dev/agents.json`; `agent select` and `workflow guide` use that assignment and report its origin (`source`).
- **012 Terminal Cockpit:** `ai-dev tui` opens a keyboard-driven terminal cockpit. It shows each feature's state with the text of `workflow state` and `workflow guide`, plus that of `doctor`, `setup show` and `init inspect`, and launches the existing actions (`agent run`, `workflow approve`, `workflow abandon`, `setup`, `init`, `git commit validate`) by handing the terminal to the command, always after the exact command is reviewed and confirmed. All untrusted text is shown sanitized and the terminal is restored on every exit.
- First third-party dependencies: Bubble Tea v2.1.1 and Lip Gloss v2.0.6, pinned under the 14-day rule and checked by govulncheck.
- `scripts/install.sh` installs ai-dev from a GitHub release without Go or Git: it verifies `SHA256SUMS` and, when `gh` is installed, the provenance attestation, and accepts only `vX.Y.Z` releases.
- `scripts/install-dev.sh` builds and installs a development version (`v0.1.0-dev+<commit>`) for manual testing, and creates a sandbox project with fake agents.
- Releases with archives for macOS and Linux (amd64 and arm64), `SHA256SUMS` and a signed SLSA provenance attestation for each archive.

[Unreleased]: https://github.com/germanmunozs/ai-dev/compare/v0.1.1...dev
[0.1.1]: https://github.com/germanmunozs/ai-dev/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/germanmunozs/ai-dev/releases/tag/v0.1.0
