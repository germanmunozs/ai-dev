# Quickstart: Project Event Watch

**Feature**: 015 | **Date**: 2026-11-26 | **Plan**: [plan.md](plan.md)

Validation scenarios for the owner. They use fake agents only, never a real coding agent. The event shapes are in [contracts/watch.md](contracts/watch.md).

## Prerequisites

- macOS or Linux, with ai-dev built from this branch: `go build -o /tmp/ai-dev-015 ./cmd/ai-dev`.
- A throwaway project: a copy of a fixture, or a new directory with `specs/` holding a few features, at least one in `implemented`.
- Two terminals in that project directory.

## 1. Snapshot (US2)

```sh
/tmp/ai-dev-015 watch --format json --once
```

Expected:

- exactly one line, a `snapshot` with `seq` 1;
- its `features[*].state` and `next` are equal to `ai-dev workflow state --format json <id>` and `ai-dev workflow next --format json <id>`;
- `setup` is equal to `ai-dev setup show --format json`, and `environment` to `ai-dev doctor --format json`;
- exit code 0.

## 2. A Human Gate, a refusal and a manual edit (US1)

Terminal A:

```sh
/tmp/ai-dev-015 watch --format text
```

Terminal B, one at a time:

1. `ai-dev workflow approve --approver you@example.com <implemented-feature> approved`

   Within a second, A shows one `workflow_state_changed` with `outcome=state_reported human_gate=approval approver="you@example.com"`.
2. Repeat the same approval.

   It is refused (the feature is terminal), and A shows no `workflow_state_changed`.
3. Edit another feature's `workflow-state.json` by hand to a different valid `state`.

   A shows a `workflow_state_changed` with `human_gate=none` and the note that the change has no recorded Human Gate.

## 3. Artifacts and next step (US3)

With A running:

- Create `plan.md` in a feature in `specified`. A shows one `artifacts_changed`, then a `next_step_changed`.
- Save a file five times quickly. A shows one `artifacts_changed`.
- `mkdir specs/099-new` shows `feature_added`; `rmdir specs/099-new` shows `feature_removed`.

## 4. Configuration and environment (US4)

With A started as `watch --environment-interval 5s`:

- `ai-dev setup apply …` shows `agents_config_changed`.
- Writing `{` into `.ai-dev/agents.json` shows `agents_config_changed` with outcome `configuration_invalid`.
- Removing a fake agent from `PATH` cannot be done inside a running process, so the automated tests cover that case with a fake executor.

## 5. Termination (US5)

- `Ctrl-C` in A: the last line is `watch_stopped reason=signal`, and `echo $?` prints 0.
- Start A again, then rename the project directory from B: `watch_stopped reason=project_gone`, exit 1.

## 6. Resource use (SC-004, manual by the owner)

1. Create a project with 100 features, each with `spec.md`, `plan.md`, `tasks.md` and `workflow-state.json`.
2. Start `watch --format json > /dev/null`.
3. After it has been idle for 60 s, run `ps -o %cpu,rss -p <pid>` a few times.

Expected: under 0.5% CPU and under 30 MB RSS. Record the values, the date and the platform in `tasks.md`.

## Automated suite

```sh
go test -race ./...
```

It includes:

- the loop tests under `testing/synctest`;
- the adapter tests on the real file system;
- the end-to-end tests of the built binary: SC-001 latency, the SC-003 no-write tree comparison, and the hostile-string vectors;
- the structural dependency test.
