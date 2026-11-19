# Quickstart: validating Feature 007 guided mode

This guide checks the guided mode end to end. It names the scenarios and their expected results. The tests that automate them are listed in `tasks.md`.

## Prerequisites

- macOS or Linux (or WSL2), Go as declared in `go.mod`, and a build of this repository.
- No coding agent needs to be installed. Availability is reported either way, and nothing is ever launched.

```sh
go build -o ai-dev ./cmd/ai-dev
```

## Scenarios

All commands run from the repository root. Each one is read-only: run `git status` afterwards, and it must show no change.

1. **Recommended step (SC-001, SC-003, SC-005).**

   ```sh
   ./ai-dev workflow guide 007-workflow-orchestration
   ```

   Expected:
   - The first line has the same `outcome`, `state`, `activity` and `role` as `./ai-dev workflow next 007-workflow-orchestration`.
   - It adds `agent=` (the 003 mapping for that role) and `availability=`.
   - It ends with the non-authorization `note`.
   - Template lines follow, each slot shown as `(slot)`.
   - Exit code is 0.

2. **Machine-readable report (SC-007).**

   ```sh
   ./ai-dev workflow guide --format json 007-workflow-orchestration
   ```

   Expected: exactly one JSON object, as specified in `contracts/workflow-guide.md`, with `schema_version` `ai-dev.workflow-guide/v1` and a `statement` field.

3. **Equivalence with 005 for every outcome (SC-001).** For a fixture in each of the seven Workflow States, an unknown feature, an invalid record and missing artifacts, compare each fixture's guidance outcome and exit code with `workflow next`. They must be identical.

4. **No effects (SC-002).** Take a content digest of the tree, run guidance, and take the digest again. The two digests must be equal. The automated test also confirms that no `Run` or `Launch` happened, that standard input was not read, and that no network-capable package is imported.

5. **Hostile identifiers (SC-004).** Use fixtures named `a;b`, `$(x)`, `-f`, `it's` and `a b`:
   - **Filled template:** with every slot filled, it is pasted into `sh -c`, `bash -c` and `zsh -c`, with a sentinel `ai-dev` on `PATH`. The sentinel receives the identifier as exactly one argument after `--`.
   - **Unfilled slots:** for each human slot, leaving only that slot unfilled makes the sentinel not run and changes no file.
   - **Format characters:** identifiers containing U+202E or U+200B produce a report without template lines.

6. **No autonomous capability (SC-006, US2).**

   ```sh
   ./ai-dev workflow run x; echo $?
   ./ai-dev orchestrate x; echo $?
   ```

   Expected: the existing usage failures, exit 2, with unchanged usage text.

7. **Regression (SC-006).**

   ```sh
   go test ./...
   ```

   Expected: every existing suite passes unchanged, including the top-level usage test.
