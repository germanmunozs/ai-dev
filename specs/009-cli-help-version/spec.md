# Feature Specification: CLI Help and Version

**Feature Branch**: `feature/009-cli-help-version`

**Feature ID**: `009`

**Created**: 2026-11-20

**Status**: Approved for implementation. The owner decided "empieza con --version y --help" (2026-11-20) after the senior-architecture review (finding P0-4). This is a declared, unauthenticated assertion.

**Input**: The architecture review found that no `--help` or `--version` exists: `ai-dev --help`, `-h`, `help`, `--version` and `version` all print the generic usage line and exit 2. Without a version, nobody can tell which binary produced a report or a failure. The top-level usage line also omits the existing `git` command.

## Clarifications

### Session 2026-11-20

- Q: Where does the version come from? → A: From the build information Go embeds in the binary, read without starting any process. A release build may set an explicit version at link time. A development build reports its module version, as Go stamps it, and its commit.
- Q: Does this change the top-level usage line, which Feature 007 kept unchanged? → A: Yes, explicitly. The line gains `git`, which already existed, plus `version` and `help`. 007's constraint applied to 007's own implementation. Every other usage text stays unchanged.

### Session 2026-11-20 (amended by Feature 010)

- Feature 010 (`specs/010-command-help-completion/spec.md`), approved by the owner on 2026-11-20 ("si, apruebo la 010 y abre el bugfix"), amends two requirements of this feature. This is a declared, unauthenticated assertion.
  - **F009-FR-004:** `help` followed by a catalog command path is valid and prints that path's help (F010-FR-005). Only an unknown path or an argument after a complete path is a usage failure, and its usage line is now `Usage: ai-dev help [command...]`.
  - **F009-FR-005:** the top-level usage line names `completion` too (F010-FR-010).

### Session 2026-11-21 (amended by Feature 011, A4)

- Feature 011 (`specs/011-agent-setup/spec.md`), approved by the owner on 2026-11-21 ("si, apruebo la 011, haz el commit y ejecuta /speckit-plan"), amends this feature. This is a declared, unauthenticated assertion.
  - **F009-FR-003:** the general help lists `setup`, `setup show` and `setup apply`.
  - **F009-FR-005:** the top-level usage line names `setup` too (F011-FR-020).

### Session 2026-11-21 (amended by Feature 012, A2)

- Feature 012 (`specs/012-terminal-cockpit/spec.md`), approved by the owner on 2026-11-21 ("si, apruebo la 012, haz el commit y ejecuta /speckit-plan"), amends this feature. This is a declared, unauthenticated assertion.
  - **F009-FR-003:** the general help lists `tui`.
  - **F009-FR-005:** the top-level usage line names `tui` too (F012-FR-020).

### Session 2026-11-24 (amended by Feature 014, A2)

- Feature 014 (`specs/014-neovim-integration/spec.md`), approved by the owner on 2026-11-24 ("si, apruebo la 014, haz el commit y ejecuta /speckit-plan"), amends this feature. This is a declared, unauthenticated assertion.
  - **F009-FR-003:** the general help lists `open`.
  - **F009-FR-005:** the top-level usage line names `open` too (F014-FR-026).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Know which build is running (Priority: P1)

A developer reporting a problem, or automation checking a deployment, needs to know exactly which build of ai-dev is running.

**Independent Test**: Run `ai-dev --version` and `ai-dev version --format json` on a fresh build and compare the commit with the repository.

**Acceptance Scenarios**:

1. **Given** any build, **When** `ai-dev --version` or `ai-dev version` runs, **Then** one line shows the version, the commit when known, whether the tree was modified, the Go version and the platform. The exit code is 0.
2. **Given** `--format json`, **Then** standard output holds exactly one object with a versioned schema identifier.
3. **Given** an extra argument or an invalid `--format`, **Then** it is a usage failure: exit 2, the usage line on standard error, nothing on standard output.

### User Story 2 - Discover the commands (Priority: P1)

A new user wants to see every command and what it does.

**Independent Test**: Run `ai-dev --help`, `ai-dev -h` and `ai-dev help`, and run each listed command.

**Acceptance Scenarios**:

1. **Given** `ai-dev --help`, `ai-dev -h` or `ai-dev help`, **Then** standard output lists every command with a one-line description, plus the exit-code convention. The exit code is 0.
2. **Given** `ai-dev help` with an extra argument, **Then** it is a usage failure (exit 2).
3. **Given** no command or an unknown one, **Then** the top-level usage line, now naming every command group, goes to standard error with exit 2, as before.

### Edge Cases

- A binary built without embedded build information reports the version `unknown`, with no commit.
- A development build with no link-time version reports the module version Go stamps, for example a pseudo-version or `(devel)`.

## Requirements *(mandatory)*

### Functional Requirements

- **F009-FR-001**: `ai-dev --version` and `ai-dev version [--format text|json]` MUST report:
  - the version: the link-time version if set; otherwise the main module version from the build information; otherwise `unknown`;
  - the VCS revision and commit time when embedded, and whether the tree was modified;
  - the Go version;
  - the platform as `GOOS/GOARCH`.

  The command MUST NOT start a process, read files, read standard input, or contact the network.
- **F009-FR-002**: The text report MUST be one line. The JSON report MUST be one object with `schema_version: "ai-dev.version/v1"`. Unknown values are omitted, except `version`, which is always present.
- **F009-FR-003**: `ai-dev --help`, `ai-dev -h` and `ai-dev help` MUST write the help to standard output and exit 0. The help lists every command group and subcommand with a one-line description, and the exit-code convention. Each listed command MUST exist: running it without arguments never yields the top-level usage.
- **F009-FR-004**: Usage failures of `version` and `help` exit 2, write only their usage line to standard error, and write nothing to standard output. *(Amended by F010-FR-005: `help <command path>` is valid.)*
- **F009-FR-005**: The top-level usage line MUST be `Usage: ai-dev <doctor|agents|agent|workflow|init|git|version|help> [options]`. Every other command's behavior and usage text MUST remain unchanged. *(Amended by F010-FR-010: the line also names `completion`.)*

### Key Entities

- **Build Information**: version, commit, commit time, modified flag, Go version, platform. It is computed per request; nothing is persisted.

## Success Criteria *(mandatory)*

- **SC-001**: In 100% of builds, `ai-dev --version` exits 0 and names the platform and Go version. In a build from a Git checkout, it names the commit.
- **SC-002**: 100% of the commands listed in the help exist, and none of the existing command paths is missing from it.
- **SC-003**: The regression suites of Features 001–008 pass, with only the top-level usage expectations updated as F009-FR-005 requires.

## Assumptions

- Releases set the version at link time (`-ldflags "-X <module>/internal/version.Version=vX.Y.Z"`). The release workflow is separate work (review finding P1-5).
- Per-command help and shell completions are out of scope (review finding P2-11).
