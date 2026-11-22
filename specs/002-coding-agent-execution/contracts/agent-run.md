# Contract: `ai-dev agent run`

**Satisfies**: FR-004 through FR-019, FR-023 through FR-026, SC-003 through SC-009

## Command

```text
ai-dev agent run [--format text|json] <agent-id> [instruction]
```

- `<agent-id>` is required and positional. `instruction` is an optional second
  positional. `--format` defaults to `text` and, like `doctor` and `agents`, must
  precede the positionals (stdlib `flag` parsing stops at the first non-flag token).
- Instruction, if not given as the second positional, is read from standard input
  (FR-005).

## Recognizing a request (usage failure vs. execution outcome)

A **usage failure** (exit `2`, usage text on stderr, stdout empty, no registry lookup,
no process launched, no `Report` emitted — FR-026) occurs only when the command shape
itself cannot be recognized:

- Missing `<agent-id>` entirely.
- More than two positional tokens after flags.
- Unknown flags, or `--format` given a value other than `text`/`json`.

Once an `<agent-id>` token is present, the request is **recognized**, and every further
problem below is an `execution_failed` **outcome** (reported through the normal
Report path — see `contracts/adapter-interface.md` for the lifecycle), not a usage
failure:

| Condition                                                        | Outcome |
|--------------------------------------------------------------------|---------|
| `<agent-id>` not one of the supported IDs                          | `execution_failed` — "unsupported agent" (FR-007) |
| Supported `<agent-id>`, but not locally available                  | `execution_failed` — "agent unavailable" (FR-008) |
| No instruction from either source, or it is empty/whitespace-only  | `execution_failed` — "instruction required" (FR-009) |
| Instruction present in **both** the positional and stdin           | `execution_failed` — "ambiguous instruction source" (FR-005) |
| Stdin is the only possible source and it is an interactive terminal| `execution_failed` — "instruction required" (fails immediately, no blocking read; FR-005) |
| Process fails to launch                                            | `execution_failed` |
| Human interruption (handled)                                       | `execution_failed` (FR-025) |
| Process terminates abnormally (signal/crash)                       | `execution_failed` |
| Process exits normally, adapter classifies success                 | `agent_succeeded` |
| Process exits normally, adapter classifies failure                 | `agent_failed` |

## Streaming and final report — human-readable mode (default)

- Agent stdout → `ai-dev` stdout, byte-for-byte, no labels, live as produced.
- Agent stderr → `ai-dev` stderr, byte-for-byte, no labels, live as produced.
- Order is preserved within each stream; no ordering is guaranteed between the two
  streams (FR-011).
- After the process ends (or cancellation completes), exactly one final record is
  written to **stderr**, visibly delimited from any preceding agent stderr output,
  identifying itself as the `ai-dev` outcome and displaying exactly one canonical value:

  ```text
  ai-dev: outcome=execution_failed agent=codex-cli reason="agent unavailable: codex not found on PATH"
  ```

  (`reason=` is omitted when `Outcome == agent_succeeded` and `Explanation` is empty.)

## Streaming and final report — machine-readable mode (`--format json`)

- **All** underlying-agent output (both its stdout and its stderr) is written to
  `ai-dev`'s **stderr**, live, as produced — never to stdout (FR-011, FR-018).
- `ai-dev`'s **stdout** carries exactly one JSON object, written only after the
  invocation finishes, and nothing else:

  ```json
  {"agent_id": "codex-cli", "outcome": "execution_failed", "explanation": "agent unavailable: codex not found on PATH"}
  ```

  - `agent_id`: the identifier exactly as supplied by the caller (FR-018) — even when
    unsupported.
  - `outcome`: exactly one of `agent_succeeded`, `agent_failed`, `execution_failed`.
  - `explanation`: present (non-empty) for both failure outcomes; may be an empty string
    for `agent_succeeded`.

## Exit codes

| Condition                                    | Exit code |
|------------------------------------------------|-----------|
| Usage failure                                   | `2` |
| Recognized request, outcome `agent_succeeded`   | `0` |
| Recognized request, outcome `agent_failed`      | `1` |
| Recognized request, outcome `execution_failed`  | `1` |

This is FR-019: success-vs-failure process completion is exactly `agent_succeeded` vs.
everything else.

## Cancellation (SIGINT while an agent is running)

1. `ai-dev` forwards cancellation to the running agent process (sends its native
   interrupt signal).
2. `ai-dev` waits for the agent process to actually stop (bounded grace period, then a
   forced stop) before proceeding.
3. If `ai-dev` can still produce output at that point, it emits `execution_failed`
   through the normal Report path above and exits with an unsuccessful code.
4. If a second, forced termination signal prevents `ai-dev` from running its own
   handler at all, no final Report is promised — this is the one case FR-025
   explicitly does not require reporting for.

## Permissions and security (applies to every outcome)

- No flag, environment variable, or configuration change that elevates the agent's
  permissions, disables its sandbox, or auto-approves/suppresses its prompts is ever
  passed (FR-015, FR-016). Any prompt the agent itself surfaces reaches the developer
  unchanged through the passed-through stdio.
- `ai-dev` never introduces, appends, or exposes a credential, token, or secret in
  either mode's output (FR-017) — it forwards the agent's own streams unmodified and
  only adds the fixed-shape outcome record/JSON object described above, neither of
  which ever contains agent output text.

## Test seams this contract implies

- Instruction-source validation, independent of any process: argument-only,
  stdin-only, both (ambiguous), neither, whitespace-only, interactive-stdin-with-no-arg
  — all unit-testable against `internal/instruction` with fake readers/booleans, no
  subprocess involved.
- Unsupported agent / unavailable agent: fake `agent.Registry` / fake
  `system.Executor`, no process launched.
- Process launch failure, agent success, agent failure, cancellation: fake
  `system.Launcher` in `internal/execution` unit tests — deterministic, no real
  subprocess.
- Stdout/stderr routing and machine-readable stdout isolation: assembled at the
  `internal/app` level with a fake `system.Launcher` writing distinguishable content to
  each stream.
- Live streaming observable before exit (SC-009): one real-subprocess test in
  `internal/system` using a controllable fixture script that writes and flushes a
  marker to each stream, then blocks until the test releases it — the test asserts the
  marker is observed through the destination writer before releasing/exiting the
  fixture.
