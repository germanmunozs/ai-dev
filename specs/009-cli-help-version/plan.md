# Implementation Plan: CLI Help and Version

**Branch**: `feature/009-cli-help-version` | **Date**: 2026-11-20 | **Spec**: [spec.md](spec.md)

## Summary

Add `ai-dev version` and `ai-dev help` with their `--version`, `--help` and `-h` aliases, and make the top-level usage line name every command group.

The version comes from `runtime/debug.ReadBuildInfo`, plus an optional link-time variable. The help is a fixed text whose command list is checked against the router by a test.

## Technical Context

- **Language and dependencies:** Go 1.27.1, standard library only.
- **Storage:** none.
- **Testing:** `go test`, covering:
  - unit tests of the build-information mapping, with injected `debug.BuildInfo` values;
  - rendering tests;
  - command tests;
  - a test that every help entry routes to its own command;
  - a subprocess test against a built binary.
- **Platforms:** macOS and Linux, as the rest of the CLI. The command is platform-neutral.
- **Constraints:** no process, file or network access for `version` (F009-FR-001); only the top-level usage line changes (F009-FR-005).

## Constitution Check

| Principle | Status |
|---|---|
| I. Specification First | PASS: spec approved by the owner's decision |
| III. Simplicity | PASS: two small commands, no dependency, no persistence |
| IV. Testable | PASS: every requirement maps to a test (tasks.md) |
| V. Security | PASS: read-only. The version reports only build metadata, with no paths or environment values |
| IX. Human Authority | PASS: no authority involved |

## Design

- **`internal/version`:**
  - `Version` is the link-time variable;
  - `Info{Version, Commit, CommitTime, Modified, GoVersion, Platform}`;
  - `Read()`, which uses `debug.ReadBuildInfo`;
  - `fromBuildInfo(*debug.BuildInfo, bool)`, pure and testable.
- **`internal/report/version.go`:** `WriteVersionText` and `WriteVersionJSON` (`ai-dev.version/v1`).
- **`internal/app/version.go`:** `RunVersion(args, stdout, stderr)`, with flag parsing as in the other commands.
- **`internal/app/help.go`:** `RunHelp(args, stdout, stderr)`, and `helpCommands`, the ordered list of command paths and descriptions that the routing test reads.
- **`internal/app/app.go`:**
  - routes `--version`/`version` and `--help`/`-h`/`help`;
  - one `topLevelUsage` constant replaces the two literal usage lines.

## Project Structure

```text
internal/version/version.go, version_test.go        NEW
internal/report/version.go, version_test.go         NEW
internal/app/version.go, help.go, help_test.go      NEW
internal/app/app.go                                 MODIFIED (routing, usage constant)
cmd/ai-dev/version_test.go                          NEW (subprocess)
tests pinning the old top-level usage               UPDATED (F009-FR-005)
README.md                                           UPDATED (command reference)
```
