# Quickstart: validating `ai-dev workflow next` (005)

This guide runs once implementation is complete. It proves the feature end to end against the
[CLI contract](./contracts/workflow-next.md) and the
[rules contract](./contracts/next-step-rules.md). It does not restate those contracts.

## Prerequisites

- Go 1.27.1 (per `go.mod`).
- A checkout of this repository on the feature branch.
- No coding agent, network access, or credentials are needed. The command never invokes them.

## 1. Automated gates

```sh
gofmt -l .                       # expect no output
go vet ./...
go test -count=1 ./...           # all pass, including every 001–004 assertion
go test -race ./internal/sdd/... ./internal/app/...
go test ./internal/sdd -run TestPackageBoundary        # import allow-list + alias-resolved selectors
go test ./cmd/ai-dev -run TestWorkflowNextBrokenPipe   # Unix (macOS/Linux): real broken pipe → exit 2
go build -o /tmp/ai-dev ./cmd/ai-dev
git diff --check
```

Before merge, on the committed feature branch, review the output of the following command. It is
a review input, not a pass/fail gate. Every listed hunk must be additive and justified, and no
existing assertion may be removed or weakened:

```sh
git diff --diff-filter=M main...HEAD -- '*_test.go'
```

## 2. Manual scenarios against a scratch repository

Create a throwaway tree. Never use the real repository's `specs/` for these scenarios.

```sh
R=$(mktemp -d) && cd "$R" && mkdir -p specs/f
```

Run each row with `/tmp/ai-dev workflow next f`, then with `--format json f`. Check the outcome,
the stream, and the exit code (`echo $?`).

| Setup inside `specs/f/` | Expected outcome | Exit |
|---|---|---|
| empty dir, no record | `step_recommended`, `draft`, `specify`/`specification` | 0 |
| `workflow-state.json` = `{"state":"specified"}`, no `spec.md` | `artifacts_inconsistent`, missing `spec.md` | 1 |
| same, plus `touch spec.md` | `step_recommended`, `plan`/`planning` | 0 |
| `{"state":"planned"}`, `spec.md`, `Plan.md` only | `artifacts_inconsistent`, missing `plan.md` (also on macOS) | 1 |
| `{"state":"tasked"}`, `spec.md`, `plan.md`, `ln -s plan.md tasks.md` | `artifacts_inconsistent`, missing `tasks.md` | 1 |
| `{"state":"approved"}` with all three files | `workflow_terminal`, `state=approved`, no approval wording | 0 |
| `{"state":"abandoned"}`, no files | `workflow_terminal` | 0 |
| `{"state":"Planned"}` | `state_invalid` | 1 |

Also run these:

```sh
/tmp/ai-dev workflow next ../x            # feature_unknown, exit 1
/tmp/ai-dev workflow next /etc/passwd     # feature_unknown, exit 1
ln -s f specs/g && /tmp/ai-dev workflow next g   # feature_unknown, exit 1 (symlinked dir)
/tmp/ai-dev workflow next                 # usage on stderr, empty stdout, exit 2
/tmp/ai-dev workflow next --format yaml f # usage, exit 2
```

### Output failure: real broken pipe (macOS and Linux)

The writer is delayed so that the reader has already closed its end, which makes the stdout write
fail with a broken pipe:

```sh
bash -c '(sleep 0.5; /tmp/ai-dev workflow next f) | (exec 0<&-; sleep 2); echo "next exit=${PIPESTATUS[0]}"'
# expect: stderr "write workflow next report: … broken pipe", next exit=2

bash -c '(sleep 0.5; /tmp/ai-dev workflow state f) | (exec 0<&-; sleep 2); echo "state exit=${PIPESTATUS[0]}"'
# isolation control: unchanged 004 behavior, state exit=141 (terminated by SIGPIPE)
```

This manual check relies on a generous delay. The deterministic proof is the automated
`TestWorkflowNextBrokenPipe`, which closes the pipe's read end before starting the process.
Do not use `>&-`: the Go runtime reopens a closed stdout as `/dev/null`, so the write succeeds and
the exit code is 0. `/dev/full` is not needed and does not exist on macOS.

## 3. Read-only and no-coupling checks

```sh
find "$R" -exec stat -f '%m %N' {} + | sort > /tmp/before   # GNU: stat -c '%Y %n'
/tmp/ai-dev workflow next f; /tmp/ai-dev workflow next --format json f
find "$R" -exec stat -f '%m %N' {} + | sort > /tmp/after
diff /tmp/before /tmp/after        # expect no difference
```

The command needs no `claude` or `codex` binary on `PATH`, and produces the same output when they
are absent.

## 4. Cleanup

```sh
rm -rf "$R" /tmp/ai-dev /tmp/before /tmp/after
```
