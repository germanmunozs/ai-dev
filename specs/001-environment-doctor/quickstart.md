# Quickstart: Validating Environment Doctor

**Feature**: `001-environment-doctor`

This is a validation guide, not an implementation guide — it proves the delivered feature
satisfies the spec's acceptance scenarios (`spec.md`) once implemented. It assumes the
module lives at the repository root with the entry point at `cmd/ai-dev` (see `plan.md`
"Project Structure").

## Prerequisites

- Go toolchain matching the version pinned in `go.mod` (`go1.27.1 darwin/arm64` in the
  reference environment).
- Run all commands from the repository root (`/Users/german/Developer/ai-dev`). The
  filesystem-backed repository checks (`speckit-initialized`, `constitution`,
  `claude-integration`, `codex-integration`) inspect paths relative to the current working
  directory directly, so they require running from the repository root. `git-repository` is
  the exception: it delegates to `git rev-parse --is-inside-work-tree` (research.md §9),
  which recognizes any subdirectory of a working tree — running from the root is still
  recommended for consistency with the other checks, but not required for this one.

## Build

```bash
go build -o bin/ai-dev ./cmd/ai-dev
```

Expected: exits `0`, produces `bin/ai-dev`.

The module pins Go 1.27.1 in `go.mod`. Confirm the active toolchain before validating:

```bash
go version
```

Expected: reports `go1.27.1` (the supported reference target is `darwin/arm64`).

## Required quality gates

Run these commands from the repository root in order:

```bash
gofmt -l .
go vet ./...
go test ./...
GOOS=darwin GOARCH=arm64 go build ./...
go build ./...
```

Pass conditions:

- `gofmt -l .` prints nothing.
- `go vet ./...` exits `0` without diagnostics.
- `go test ./...` exits `0`; all packages pass without relying on real Git, Spec Kit,
  Claude Code, or Codex CLI installations.
- Both build commands exit `0`. On the reference Apple Silicon macOS environment they are
  currently equivalent: both target `darwin/arm64`. The explicit `GOOS`/`GOARCH` command
  documents the currently supported platform; it does not provide cross-platform
  validation. The plain command verifies the native build.

To run the command without writing a binary first:

```bash
go run ./cmd/ai-dev doctor
go run ./cmd/ai-dev doctor --format json
```

The text command must list all nine checks and an overall result. The JSON command must
emit a single report object with exactly nine ordered checks. Ready exits `0`, Not Ready
exits `1`, and invalid usage exits `2`.

## Scenario: User Story 1 — fully-ready environment and repository (P1)

This repository, as it stands on branch `001-environment-doctor`, already has Git, Spec
Kit, Claude Code, and Codex CLI installed and discoverable, `.specify` initialized, a
completed constitution, and both Claude and Codex Spec Kit integrations installed — i.e. it
is expected to be a Ready fixture once the checks are implemented against it.

```bash
./bin/ai-dev doctor
echo "exit code: $?"
```

Expected:
- All nine checks print as `PASS`.
- Final line reads `Overall: Ready`.
- Exit code `0`.

Repeat-run determinism (Acceptance Scenario 1.2):

```bash
./bin/ai-dev doctor --format json > /tmp/run1.json
./bin/ai-dev doctor --format json > /tmp/run2.json
diff /tmp/run1.json /tmp/run2.json
```

Expected: no diff (with no environment/repository change between runs).

## Scenario: User Story 2 — diagnosing one broken prerequisite (P2)

Pick one prerequisite, break it in isolation, confirm only its check fails, then restore it.

**Missing constitution** (Acceptance Scenario 2.1):

```bash
mv .specify/memory/constitution.md /tmp/constitution.md.bak
./bin/ai-dev doctor
```

Expected: `constitution` check reports `FAIL` with an explanation that the file is missing
and a corrective action; the other eight checks still run and report their prior results
unchanged.

Restore:

```bash
mv /tmp/constitution.md.bak .specify/memory/constitution.md
```

**Incomplete Codex integration** (Acceptance Scenario 2.2):

```bash
mv .agents/skills/speckit-plan/SKILL.md /tmp/speckit-plan.SKILL.md.bak
./bin/ai-dev doctor
```

Expected: `codex-integration` reports `FAIL`, naming the missing file; `claude-integration`
still reports its own (independent) result, unaffected.

Restore:

```bash
mv /tmp/speckit-plan.SKILL.md.bak .agents/skills/speckit-plan/SKILL.md
```

**Codex CLI not discoverable** (Acceptance Scenario 2.3): run doctor with a `PATH` that
excludes the directory containing `codex`:

```bash
CODEX_DIR=$(dirname "$(command -v codex)")
./bin/ai-dev doctor --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["overall"])'  # sanity: currently "ready"
PATH=$(echo "$PATH" | tr ':' '\n' | grep -vFx "$CODEX_DIR" | paste -sd: -) ./bin/ai-dev doctor
```

Expected: `codex-cli-tool` reports `FAIL` ("could not be found"); `git-tool`,
`speckit-tool`, and `claude-code-tool` still report their own results.

## Scenario: User Story 3 — automation gating (P3)

```bash
./bin/ai-dev doctor --format json | python3 -m json.tool
echo "exit code: $?"
```

Expected (fully-ready fixture): `"overall": "ready"`, every `checks[].status == "pass"`,
process exit code `0`.

Then, with any one prerequisite broken (reuse a step above):

```bash
./bin/ai-dev doctor --format json
echo "exit code: $?"
```

Expected: `"overall": "not_ready"`, the broken check's `status == "fail"`, process exit code
`1` — determinable by a script from `overall` and each `checks[].id`/`status` alone, with no
free-text parsing (validates FR-008, SC-006, SC-007). Validate the shape against
`contracts/doctor-report.schema.json` with any JSON Schema validator available.

## Edge cases worth a manual pass

- Run from a directory outside any Git repository (e.g. `/tmp`): `git-tool`/`speckit-tool`/
  `claude-code-tool`/`codex-cli-tool` still run and report their real results; the five
  repository-dependent checks all `FAIL` with a "no repository/scaffold found" style
  explanation. Overall: Not Ready.
- Run in a repository with `.specify` entirely absent: `speckit-initialized` fails with an
  explanation that the scaffold is missing, and does not crash even though
  `.specify/integrations/*.manifest.json` cannot be read.
- Run with every prerequisite broken at once (empty directory, no tools on `PATH`): all nine
  checks still complete and report their own `FAIL`; process does not panic or hang; exit
  code `1`.

## Cleanup

Ensure no scenario above left the working tree modified:

```bash
git status --porcelain
```

Expected: no unexpected changes (every scenario above restores what it moved).
