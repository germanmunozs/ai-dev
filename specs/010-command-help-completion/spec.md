# Feature Specification: Command Help and Shell Completion

**Feature Branch**: `feature/010-command-help-completion`

**Feature ID**: `010`

**Created**: 2026-11-20

**Status**: Approved for planning. The owner authorized drafting it on 2026-11-20 ("si, autorizo todo y escribe la especificación de la 010"), from review finding P2-11, and approved it as written the same day ("si, apruebo la 010 y abre el bugfix"). This is a declared, unauthenticated assertion.

**Input**: Feature 009 added a general help. A user still cannot ask one command for its options: `ai-dev workflow state --help` is a usage failure (exit 2), and `ai-dev help workflow` is too. The only way to learn a command's arguments is to get them wrong. There is also no shell completion.

## Clarifications

### Session 2026-11-20

- Q: Where is `--help` recognized? → A: Only as the first argument after a complete command path, or a complete group path. Anywhere else it keeps its current meaning, so an agent instruction, a feature identifier or an option value is never reinterpreted as a request for help.
- Q: Which shells get completion? → A: bash and zsh, the POSIX shells the project already supports for the 007 guide. fish and PowerShell are out of scope.
- Q: Does this change Feature 009? → A: Yes, explicitly. F009-FR-004, under which `help` with an argument is a usage failure, is amended so that `help <command path>` is valid. F009-FR-005 is amended so that the top-level usage line names `completion`. Every other usage text and exit code stays unchanged.

### Session 2026-11-21 (Amendment 1: the `workflow` group usage line)

- **Defect.** The `workflow` group usage line, written for an unknown or missing `workflow` subcommand, still read `Usage: ai-dev workflow <state|transition> [options]`. Features 005, 006 and 007 added `next`, `approve`, `abandon` and `guide` and kept the line unchanged on purpose, and F010-FR-010 froze it again (research.md recorded it as found, out of scope).
- **Authority record.** German Munoz, repository owner, asked for this fix on 2026-11-21 in the words "si, bórrala y corrige la línea de uso de workflow". This is a declared, unauthenticated assertion.
- Q: What does the line say now? → A: `Usage: ai-dev workflow <state|transition|next|guide|approve|abandon> [options]`, the six subcommands in catalog order. The exit code (2), the stream (stderr) and the cases that print it do not change.
- Q: How is drift prevented? → A: A test requires each group usage line (`agent` and `workflow`) to name exactly the subcommands the catalog routes under that group.
- The usage expectations that Features 004–007 and 010 fixed in their tests are updated to the new line. No other usage text, behavior or exit code changes.

### Session 2026-11-21 (amended by Feature 011, A3)

- Feature 011 (`specs/011-agent-setup/spec.md`), approved by the owner on 2026-11-21 ("si, apruebo la 011, haz el commit y ejecuta /speckit-plan"), amends F010-FR-010. This is a declared, unauthenticated assertion. Once Feature 011 is implemented, the top-level usage line is `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|version|help|completion> [options]`, and the catalog holds `setup`, `setup show` and `setup apply` (F011-FR-020).

### Session 2026-11-21 (amended by Feature 012, A1)

- Feature 012 (`specs/012-terminal-cockpit/spec.md`), approved by the owner on 2026-11-21 ("si, apruebo la 012, haz el commit y ejecuta /speckit-plan"), amends F010-FR-010. This is a declared, unauthenticated assertion. Once Feature 012 is implemented, the top-level usage line is `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|version|help|completion> [options]`, and the catalog holds `tui` (F012-FR-020).

### Session 2026-11-24 (amended by Feature 014, A1)

- Feature 014 (`specs/014-neovim-integration/spec.md`), approved by the owner on 2026-11-24 ("si, apruebo la 014, haz el commit y ejecuta /speckit-plan"), amends F010-FR-010. This is a declared, unauthenticated assertion. Once Feature 014 is implemented, the top-level usage line is `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|open|version|help|completion> [options]`, and the catalog holds `open` (F014-FR-026).

### Session 2026-11-26 (amended by Feature 015, A1)

- Feature 015 (`specs/015-watch/spec.md`), approved by the owner on 2026-11-26 ("si, apruebo la 015, haz el commit y ejecuta /speckit-plan"), amends F010-FR-010. This is a declared, unauthenticated assertion. Once Feature 015 is implemented, the top-level usage line is `Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|open|watch|version|help|completion> [options]`, and the catalog holds `watch` (F015-FR-023).

### Session 2026-11-25 (Amendment 2: compound values in help)

- **Finding.** L5 of the Feature 013 review: the help of `setup apply --role` listed every `ROLE=AGENT` pair, 40 with eight agents, and the line grows by five values per agent added.
- **Authority record.** German Munoz, repository owner, asked for this change on 2026-11-25 in the words "haz la ayuda compacta de --role". This is a declared, unauthenticated assertion.
- Q: How does help state the allowed values of `--role`? → A: As the two fixed sets the pair is made of, once each: `[values: ROLE: <roles>; AGENT: <agents>]`, in role-list and catalog order. This clarifies F010-FR-002 ("its allowed values when they are fixed") for a value that combines two fixed sets; every other option keeps its full list.
- Q: Does completion change? → A: No. Completion still offers every `ROLE=AGENT` pair (F010-FR-007, F011-FR-020).
- No other help text, usage line, behavior or exit code changes.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Learn one command's options (Priority: P1)

A user knows which command they need but not its arguments. They ask that command for help instead of getting it wrong first.

**Independent Test**: For each command, run `ai-dev <command> --help`, `-h` and `ai-dev help <command>`. Compare the usage line with the one the command prints on a usage failure.

**Acceptance Scenarios**:

1. **Given** any command path, **When** `ai-dev <path> --help`, `ai-dev <path> -h` or `ai-dev help <path>` runs, **Then** standard output holds that command's help: its description, its exact usage line, its arguments and options, what it can change, and the exit codes. Standard error is empty and the exit code is 0.
2. **Given** a group path such as `ai-dev workflow --help` or `ai-dev help git commit`, **Then** the help lists the group's commands with their descriptions, and exits 0.
3. **Given** `ai-dev init --help`, **Then** the help describes the interactive assistant and lists `inspect`, `plan`, `apply` and `reconcile`. No terminal is needed, and nothing is inspected.
4. **Given** `--help` anywhere other than the first argument after the path, as in `ai-dev workflow approve --approver --help f planned` or `ai-dev agent run claude-code --help`, **Then** the command behaves exactly as it does today.
5. **Given** `ai-dev help` with an unknown path, or with an argument after a complete command path, **Then** it is a usage failure: exit 2, the help usage line on standard error, nothing on standard output.

### User Story 2 - Complete commands in the shell (Priority: P2)

A user who types `ai-dev` often wants the shell to complete commands, options and fixed values.

**Independent Test**: In bash and zsh, load the script `ai-dev completion bash|zsh` prints, then complete partial command lines.

**Acceptance Scenarios**:

1. **Given** `ai-dev completion bash` or `ai-dev completion zsh`, **Then** standard output holds a completion script for that shell, and the exit code is 0.
2. **Given** the loaded script, **When** the user completes `ai-dev wo`, `ai-dev workflow `, `ai-dev workflow state --f` or `ai-dev init plan --strategy `, **Then** the shell offers `workflow`, the workflow commands, `--format`, and the four strategies, respectively.
3. **Given** an option that takes a file or a directory, **Then** the shell offers file names, as it does by default.
4. **Given** `ai-dev completion` with no shell, an unknown shell or an extra argument, **Then** it is a usage failure (exit 2).

### Edge Cases

- `ai-dev --help <path>` and `ai-dev -h <path>` behave as `ai-dev help <path>`, because 009 routes them to `help`.
- `ai-dev help help` and `ai-dev help completion` show those commands' own help.
- `ai-dev agent run --help` shows help and reads no standard input, even when standard input is a pipe.
- `ai-dev <path> -- --help` is not a request for help.
- The usage line of `workflow approve` and `workflow abandon` differs by verb; each command's help shows its own.
- macOS ships bash 3.2. The bash script must work there as well as in current bash.

## Requirements *(mandatory)*

### Functional Requirements

- **F010-FR-001**: For every command path and every group path in the command catalog, `ai-dev <path> --help`, `ai-dev <path> -h` and `ai-dev help <path>` MUST write that path's help to standard output, write nothing to standard error, and exit 0. Group paths are `agent`, `workflow`, `init`, `git` and `git commit`. `init` is both a command and a group.
- **F010-FR-002**: A command's help MUST contain:
  - its one-line description, the same as in the general help;
  - its usage line, byte-identical to the line the command writes on a usage failure;
  - each positional argument and each option, with its value placeholder, its allowed values when they are fixed, and a one-line description;
  - what the command can change: nothing, `workflow-state.json`, the repository and project configuration, the plan file, or whatever the agent does;
  - the exit-code convention.

  A group's help MUST list each command of the group with its description.
- **F010-FR-003**: `--help` and `-h` MUST be recognized only as the first argument after a complete command or group path. Anywhere else, including after `--`, as an option value, or after a positional argument, the command's current behavior MUST remain unchanged.
- **F010-FR-004**: Producing help MUST NOT run the command: it MUST NOT start a process, read standard input, read or write files or durable state, require a terminal, or contact the network.
- **F010-FR-005**: `ai-dev help` with no argument keeps the 009 general help. That help lists `completion`, and says how to get one command's help. `ai-dev help` with an unknown path, or with any argument after a complete path, MUST exit 2. It MUST write only `Usage: ai-dev help [command...]` to standard error, and nothing to standard output. This amends F009-FR-004.
- **F010-FR-006**: `ai-dev completion bash` and `ai-dev completion zsh` MUST write a completion script to standard output and exit 0. A missing or unknown shell, or an extra argument, MUST exit 2. It MUST write only `Usage: ai-dev completion bash|zsh` to standard error, and nothing to standard output.
- **F010-FR-007**: The completion script MUST complete:
  - command and group paths;
  - the options of the command being typed, including `--help`;
  - the fixed values of options and positional arguments: formats, strategies, shells, agent identifiers and roles;
  - file names, for options that take a file or a directory.

  It MUST NOT complete anything else.
- **F010-FR-008**: The completion script MUST be static. Completing never runs ai-dev or any other program, and never reads project files or state. The script MUST contain no value taken from the environment, and MUST be byte-identical across runs for the same build. The bash script MUST work in bash 3.2 and later; the zsh script in zsh 5 and later.
- **F010-FR-009**: The general help, each command's help and both completion scripts MUST come from one command catalog. Every catalogued path MUST be routed. Every option the catalog lists for a command MUST be accepted by that command, and every option the command accepts MUST be in the catalog.
- **F010-FR-010**: The top-level usage line MUST be `Usage: ai-dev <doctor|agents|agent|workflow|init|git|version|help|completion> [options]`. This amends F009-FR-005. The `workflow` group usage line MUST be `Usage: ai-dev workflow <state|transition|next|guide|approve|abandon> [options]`, and each group usage line MUST name exactly the subcommands routed under that group (Amendment 1, 2026-11-21). Every other command's behavior, usage text and exit codes MUST remain unchanged.

### Key Entities

- **Command Catalog**: the ordered list of command and group paths. Each entry holds a description, a usage line, its arguments and options with any fixed values, and what the command can change. It is fixed at build time; nothing is persisted.

## Success Criteria *(mandatory)*

- **SC-001**: 100% of the command and group paths in the catalog answer `--help`, `-h` and `help <path>` with exit 0. None of them reads standard input or needs a terminal.
- **SC-002**: For 100% of commands, the usage line in the help is identical to the usage line of a usage failure.
- **SC-003**: Both completion scripts load without errors in bash 3.2, current bash and zsh 5. They complete every catalogued command path, and the fixed values of each option that has them.
- **SC-004**: For 100% of commands, the options the catalog lists equal the options the command accepts.
- **SC-005**: The regression suites of Features 001–009 pass. The only updated expectations are the top-level usage line, the general help text, `help` with an argument, and the `workflow` group usage line (Amendment 1), as F010-FR-005 and F010-FR-010 require.

## Assumptions

- Help is plain English text, like the rest of the CLI's output. A JSON form of the help is out of scope.
- Completion is static by design. Completing feature identifiers from `specs/`, or anything else read at completion time, is out of scope.
- Installing the scripts is documented in the README (for example `source <(ai-dev completion bash)`). ai-dev never writes shell startup files.
- The option-injection defect found while drafting this spec is separate work, outside this feature: an `agent run` instruction that starts with `-` reaches the agent as one of its options. F010-FR-003 keeps `ai-dev agent run claude-code --help` as an instruction, whatever Feature 002 does with it.
