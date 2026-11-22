# Quickstart: Coding Agent Execution

**Feature**: 002-coding-agent-execution

This validates the feature end-to-end once implemented. It does not require a real
Claude Code or Codex CLI install for the deterministic checks (fixtures stand in for
both); a "real tool" pass is included as an optional final step.

## Prerequisites

- Go toolchain matching `go.mod` (`go 1.27.1`).
- POSIX shell (fixture scripts are `sh`, matching the existing pattern in
  `cmd/ai-dev/main_test.go`); Windows CI paths that skip shell-script subprocess tests
  follow the same `runtime.GOOS == "windows"` skip already used there.

## 1. Build

```sh
go build ./...
```

Expected: builds cleanly, no new dependency added to `go.mod` (per architecture goal 9,
`go.mod` should still show only the toolchain line, no `require` block, unless a
documented exception was approved).

## 2. Unit and fixture tests

```sh
gofmt -l .
go vet ./...
go test ./...
```

Expected: `gofmt -l .` prints nothing (no unformatted files); `go vet` and `go test`
both pass, including:

- `internal/agent`: registry listing/resolution, availability via fake `system.Executor`
  (contracts/agents-list.md test seams).
- `internal/instruction`: every instruction-source case in
  `contracts/agent-run.md` ("Recognizing a request" table), independent of any process.
- `internal/execution`: `Runner` lifecycle against a fake `system.Launcher` — launch
  failure, agent success, agent failure, abnormal termination, cancellation
  (contracts/adapter-interface.md).
- `internal/system`: real-subprocess tests for `OSLauncher`, including the live-marker
  test that proves output is observable before a controllable fixture process is
  released (SC-009).
- `internal/app`: usage-contract tests for `agents` and `agent run` mirroring the
  existing `TestRunFormatAndUsageContract` style, plus stdout/stderr routing and
  machine-readable stdout-isolation assertions for both CLI modes.
- **Regression**: `internal/app/doctor_test.go` and `cmd/ai-dev/main_test.go`'s existing
  assertions are unchanged and still pass — the nine-check doctor contract is untouched
  (research.md §11).

## 3. Manual validation — listing

```sh
./ai-dev agents
./ai-dev agents --format json
```

Expected: both `claude-code` and `codex-cli` are listed every time, each with an
`available`/`unavailable` result matching whether `claude`/`codex` are actually on your
`PATH`; running twice in a row without changing your environment gives identical output
(FR-003 / SC-002).

## 4. Manual validation — execution, human-readable

Pick whichever of `claude-code`/`codex-cli` shows `available` above (or install one).

```sh
./ai-dev agent run claude-code "Explain what this repository does in one paragraph."
```

Expected: the agent's own output streams live to your terminal (stdout for its stdout,
stderr for its stderr); after it finishes, a clearly delimited final line appears on
stderr containing exactly `agent_succeeded` or `agent_failed`; `echo $?` shows `0` only
in the `agent_succeeded` case.

## 5. Manual validation — execution, machine-readable

```sh
./ai-dev agent run --format json claude-code "Explain what this repository does in one paragraph." 2>/tmp/agent-stderr.log 1>/tmp/agent-report.json
cat /tmp/agent-report.json
```

Expected: `/tmp/agent-report.json` contains exactly one JSON object with `agent_id`,
`outcome`, `explanation`; `/tmp/agent-stderr.log` contains the agent's live output (both
its stdout and stderr, interleaved by arrival time) and nothing that looks like the
JSON report.

## 6. Manual validation — stdin instruction

```sh
echo "List the top-level directories in this project." | ./ai-dev agent run codex-cli
```

Expected: equivalent behavior to supplying the same text as a command argument
(FR-005).

## 7. Manual validation — failure paths

```sh
./ai-dev agent run not-a-real-agent "hello"        # expect execution_failed, "unsupported agent"
./ai-dev agent run codex-cli                        < /dev/null   # expect execution_failed, "instruction required"
./ai-dev agent run codex-cli "arg text" <<< "stdin text"          # expect execution_failed, "ambiguous instruction source"
```

For each: exit code `1`, no `agent_succeeded` anywhere in the output, and the final
outcome's explanation matches the noted reason.

## 8. Manual validation — cancellation

```sh
./ai-dev agent run claude-code "Do something that takes a while."
# press Ctrl+C partway through
```

Expected: the agent process stops (verify with `ps` / no lingering child process after
`ai-dev` exits); `ai-dev` reports `execution_failed` on stderr if it exits normally
after handling the interrupt, and exits with a non-zero code.

## References

- Command shapes and every outcome/exit-code table: `contracts/agents-list.md`,
  `contracts/agent-run.md`.
- Entity shapes: `data-model.md`.
- Why each implementation choice was made: `research.md`.
