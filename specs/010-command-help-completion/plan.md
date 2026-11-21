# Implementation Plan: Command Help and Shell Completion

**Branch**: `feature/010-command-help-completion` | **Date**: 2026-11-20 | **Spec**: [spec.md](spec.md)

## Summary

Add per-command help (`ai-dev <path> --help|-h` and `ai-dev help <path>`) and `ai-dev completion bash|zsh`. All of it is driven by one command catalog in `internal/app`, which replaces the Feature 009 `helpCommands` list.

The router recognizes a help request before it dispatches. If the argument right after a complete catalog path is `--help` or `-h`, it renders that path's help and returns. No command code runs and no dependency is touched.

The completion scripts are rendered from the catalog as static text. A test checks that the options each command really defines equal the options the catalog lists (F010-FR-009). To make that observable, every command builds its flag set through one helper (research R-03).

## Technical Context

- **Language and dependencies:** Go 1.27.4, standard library only.
- **Storage:** none.
- **Testing:** `go test`:
  - catalog unit tests;
  - help rendering;
  - router tests for every catalog path and the negative positions of F010-FR-003;
  - the option-equality test;
  - usage-line equality against each command's real usage failure;
  - completion tests that run the generated scripts in real bash and zsh;
  - a subprocess test against the built binary.
- **Target platforms:** macOS and Linux. Completion targets bash 3.2 or later (the macOS `/bin/bash`) and zsh 5 or later.
- **Project type:** CLI.
- **Constraints:**
  - help never runs the command (F010-FR-004);
  - no usage text or exit code changes, except those the spec amends (F010-FR-005, F010-FR-010);
  - the scripts are static (F010-FR-008).
- **Scale:** 18 command paths, 5 group paths and about 45 options.

## Constitution Check

| Principle | Status |
|---|---|
| I. Specification First | PASS: spec approved by the owner on 2026-11-20 |
| II. Architecture Before Framework | PASS: no framework. The catalog is plain data in `internal/app`, the package that owns the CLI surface |
| III. Simplicity and YAGNI | PASS: one catalog drives three outputs, with no code generation step and no dependency. JSON help and dynamic completion are excluded |
| IV. Testable by Design | PASS: every requirement maps to a test (tasks.md). The scripts are tested by running them in the real shells |
| V. Security by Design | PASS: help and completion are read-only. The scripts run nothing and embed no environment value. `--help` is recognized in one position only, so no instruction or value is ever reinterpreted |
| VI. Observability by Design | PASS: help goes to stdout with exit 0; usage failures keep the exit-2 contract |
| VII. Reproducibility | PASS: the scripts are byte-identical across runs for the same build, and a test enforces it |
| VIII. AI-Assisted Engineering | PASS: independent review before the PR |
| IX. Human Authority | PASS: no authority involved |

Post-design re-check: PASS. The flag-set helper (R-03) is the only change to existing command code. It is mechanical and keeps every flag, default and usage text.

## Design

- **`internal/app/catalog.go`** (new):
  - `commandSpec{path, group, description, usage, args, options, changes}`;
  - `optionSpec{name, value, values, file, description}`;
  - `argSpec{name, values, description}`;
  - `var catalog []commandSpec`, ordered as in the general help;
  - `lookup(args) (spec, consumed)`, which finds the longest catalog path that prefixes `args`.

  `usage` reuses the existing usage constants, which are extracted from the inline literals with no change to their text.
- **`internal/app/help.go`:**
  - `RunHelp` renders the general help from the catalog's commands;
  - `help <path>` renders that path's help;
  - `helpUsage` becomes `Usage: ai-dev help [command...]`;
  - `writeCommandHelp` renders a command, and `writeGroupHelp` a group.
- **`internal/app/app.go`:**
  - before the existing `switch`, `if spec, n := lookup(args); n < len(args) && (args[n] == "--help" || args[n] == "-h")`, render the help and return 0;
  - add the `completion` route;
  - `topLevelUsage` gains `completion`.
- **`internal/app/completion.go`** (new): `RunCompletion(args, stdout, stderr)`, plus `bashScript()` and `zshScript()`, which render static scripts from the catalog (contracts/completion.md).
- **`internal/app/flags.go`** (new): `newFlagSet(name string) *flag.FlagSet` returns a `ContinueOnError` set with discarded output, as every command builds today. A test-only observer records each set (R-03). All 16 `flag.NewFlagSet` call sites switch to it.

## Project Structure

```text
specs/010-command-help-completion/
  plan.md, research.md, data-model.md, quickstart.md
  contracts/help.md, contracts/completion.md
internal/app/catalog.go, catalog_test.go            NEW
internal/app/completion.go, completion_test.go      NEW
internal/app/flags.go                               NEW
internal/app/help.go, help_test.go                  MODIFIED (catalog, help <path>)
internal/app/app.go                                 MODIFIED (interception, completion route, usage)
internal/app/{agents,agentrun,agentselect,version,gitcommit,gitapply,gitinit,
  gitinit_wizard,gitplan,gitreconcile,workflowgate,workflownext,workflowguide,
  workflowstate,workflowtransition}.go              MODIFIED (newFlagSet; usage constants)
cmd/ai-dev/help_test.go                             NEW (subprocess)
tests pinning the top-level usage and general help  UPDATED (F010-FR-005, FR-010)
README.md                                           UPDATED (help, completion install)
```
