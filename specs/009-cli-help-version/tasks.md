# Tasks: CLI Help and Version

**Input**: `spec.md` and `plan.md` in this directory. **Tests**: mandatory (constitution IV), written before the code.

## Phase 1: Version (US1)

- [X] T001 [US1] Tests in `internal/version/version_test.go` for `fromBuildInfo`, with these cases:
  - no build information → `unknown`;
  - `(devel)` with VCS settings → commit, time and modified;
  - a module pseudo-version;
  - the link-time `Version` overriding the module version.

  Platform and Go version always come from `runtime`.
- [X] T002 [US1] Implement `internal/version/version.go` (the `Version` variable, `Info`, `Read`, `fromBuildInfo`). It must use no `os`, `os/exec` or `net` import, and a structural test asserts this.
- [X] T003 [P] [US1] Tests in `internal/report/version_test.go`:
  - the exact text line;
  - the exact JSON object with `schema_version` `ai-dev.version/v1`;
  - omission of unknown fields, while `version` is always present.
- [X] T004 [US1] Implement `internal/report/version.go`.
- [X] T005 [US1] Tests in `internal/app/help_test.go` for `RunVersion`:
  - text and JSON, exit 0;
  - usage failures (an extra argument, `--format yaml`, an unknown flag): exit 2, the usage line on standard error, nothing on standard output;
  - dispatch of both `ai-dev version` and `ai-dev --version`.
- [X] T006 [US1] Implement `internal/app/version.go` and the routing in `internal/app/app.go`.

## Phase 2: Help (US2)

- [X] T007 [US2] Tests in `internal/app/help_test.go`:
  - `--help`, `-h` and `help` print the same text on standard output, exit 0, with nothing on standard error;
  - `help extra` is a usage failure;
  - **routing test (SC-002):** for every entry of `helpCommands`, running its path with no further arguments must not print the top-level usage line;
  - every top-level routed group (`doctor`, `agents`, `agent`, `workflow`, `init`, `git`, `version`, `help`) appears in the help.
- [X] T008 [US2] Implement `internal/app/help.go` and its routing.
- [X] T009 [US2] Replace both literal top-level usage lines in `internal/app/app.go` with `topLevelUsage`, set to `Usage: ai-dev <doctor|agents|agent|workflow|init|git|version|help> [options]`. Update the tests that pin the old line (F009-FR-005), and nothing else.

## Phase 3: Integration and polish

- [X] T010 Subprocess test in `cmd/ai-dev/version_test.go`. It builds the binary and runs `--version`, `version --format json` and `--help`, asserting exit codes and output shape, and the commit when the build has VCS information.
- [X] T011 README: the command reference lists `version` and `help`; the build section shows how to set a release version with `-ldflags`.
- [X] T012 Gates:
  - `gofmt -l .`;
  - `go vet ./...` and `GOOS=linux go vet ./...`;
  - `go test -count=1 ./...`;
  - `go test -race -count=3` on `internal/version`, `internal/report`, `internal/app` and `cmd/ai-dev`.
- [X] T013 PR to `dev` with CI green (branch protection requires it). The owner decides the merge.
  - **Done 2026-11-20.** PR #10 passed all three required checks and was merged into `dev` as `f30e87c`, under the owner's authorization "si, autorizo el commit, los push, los PR y los merges".
