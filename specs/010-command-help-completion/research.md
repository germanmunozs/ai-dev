# Research: Command Help and Shell Completion

## R-01 Where help is recognized

- **Decision:** In `RunWithDependencies`, before any routing, find the longest catalog path that prefixes the arguments. If the very next argument is `--help` or `-h`, render that path's help and return 0.
- **Rationale:**
  - One place implements F010-FR-003, so no command can drift.
  - No command code runs, no dependency is touched and standard input is never read (F010-FR-004).
  - The first-position rule falls out naturally: `agent run claude-code --help` has a positional argument after the path, so it is not intercepted.
  - `init -h`, which today starts the wizard, is intercepted before `RunGitInit` sees it.
- **Alternatives considered:**
  - Each command handling `flag.ErrHelp` itself. Rejected: 16 places, and Go's flag package accepts `-h` anywhere among the flags, which would break F010-FR-003.
  - A help flag defined on every flag set. Rejected for the same reason.

## R-02 Catalog shape and usage lines

- **Decision:** One `[]commandSpec` in `internal/app/catalog.go`.
  - Each usage string refers to the command's existing usage text. Inline literals are extracted into constants with byte-identical content; dynamic ones (`workflow approve` and `workflow abandon`) use one function shared by the command and the catalog.
  - The catalog keeps the existing usage-failure line even where it is odd. `init inspect` prints the `init` group usage on failure today, and F010-FR-010 forbids changing that, so its help shows the line it really prints. A test compares each catalog usage line with the command's real usage-failure output (SC-002).
- **Rationale:** a single source of truth (F010-FR-009) without changing any text.
- **Found, out of scope:** the `workflow` group usage line still says `<state|transition>`, though the group has six commands. Changing it is a usage-text change that F010-FR-010 excludes. A group's help lists all six commands, so a user who asks for help sees them all.

## R-03 Proving catalog options equal command options

- **Decision:**
  - Every command builds its flag set with `newFlagSet(name)`, which behaves exactly as today's inline construction (`ContinueOnError`, output discarded).
  - A package-level `flagSetObserver`, nil in production, lets a test record each set built.
  - The test calls each command with a single undefined option. The command defines all its flags, fails to parse and returns 2 before doing anything else. The test then compares `VisitAll` of the recorded set with the catalog.
- **Rationale:**
  - Static analysis cannot see the options `init plan` defines in loops (branch names, prefixes and commit fields).
  - A runtime observation of the real flag set is exact.
  - The hook is a nil function in production, and the commands' behavior is unchanged.
- **Alternatives considered:**
  - Parsing the source with `go/ast`. Rejected: it misses loop-defined flags.
  - Declaring the options only in the catalog and building the flag sets from it. Rejected: an invasive rewrite of 16 commands with typed values, for no user benefit.

## R-04 Completion scripts

- **Decision:** Render both scripts from the catalog as static text, with no runtime calls.
  - **bash:** one function registered with `complete -o default -F _ai_dev ai-dev`. It uses only bash 3.2 features: no associative arrays and no `compopt`. Words come from `COMP_WORDS` and `COMP_CWORD`, and candidates are filtered with `compgen -W`. For a file or directory option it returns no candidates, so `-o default` falls back to file names.
  - **zsh:** a `#compdef ai-dev` function that uses `words` and `CURRENT`, `compadd` for candidates and `_files` for file options. It can be loaded with `source <(ai-dev completion zsh)` (registered with `compdef`), or installed in `fpath`.
  - Candidate lists are embedded literals. Every embedded word is checked to match `^[A-Za-z0-9._|-]+$`, so nothing needs quoting and nothing can expand.
- **Rationale:** F010-FR-008 (static, byte-identical, no program runs), and the macOS default bash.
- **Alternatives considered:**
  - zsh `bashcompinit` reusing the bash script. Rejected: it gives poorer zsh behavior, and it is an extra runtime dependency on a zsh module.
  - Dynamic completion that calls `ai-dev __complete`. Rejected by the spec's Assumptions.
- **Testing:**
  - **bash:** the test sources the script in `bash --norc --noprofile`, sets `COMP_WORDS` and `COMP_CWORD`, calls `_ai_dev` and prints `COMPREPLY`. It runs with `/bin/bash` (3.2 on macOS) and with the `bash` on `PATH`.
  - **zsh:** the test defines stub `compadd`, `_files` and `compdef` functions that print their arguments, sets `words` and `CURRENT`, and calls the function.
  - A shell that is missing skips its test locally. CI has both shells on both runners.

## R-05 Help text format

- **Decision:** Plain text, ending with a newline:
  - a header `ai-dev <path>: <description>`;
  - a blank line, then the usage line;
  - an `Arguments:` section and an `Options:` section, aligned, with fixed values shown as `text|json`;
  - a `Changes:` line;
  - an `Exit codes:` line.

  A group shows its header and a `Commands:` list. `init` shows its command help, then its `Commands:` list.
- **Rationale:** it mirrors the 009 general help and is readable at 80 columns.
