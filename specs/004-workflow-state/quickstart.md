# Quickstart: Workflow State

Runnable validation that `ai-dev workflow state` and `ai-dev workflow transition` satisfy the
approved spec contract. See `contracts/workflow-state.md`, `contracts/workflow-transition.md`, and
`contracts/workflow-record.md` for the full field-level contracts this guide exercises rather than
restates.

## Prerequisites

- Go 1.27.1 toolchain (matches `go.mod`).
- A scratch directory outside this repository (never run these steps against your real working
  tree, since step 4 intentionally creates fixture files).

## Setup

```sh
cd /path/to/ai-dev
go build -o /tmp/ai-dev-004 ./cmd/ai-dev

WORK=$(mktemp -d)
mkdir -p "$WORK/specs/001-sample-feature"
cd "$WORK"
```

## 1. Report state for a feature with no record — expect `draft`

```sh
/tmp/ai-dev-004 workflow state 001-sample-feature
# ai-dev: outcome=state_reported feature="001-sample-feature" state=draft terminal=false
echo $?   # 0
```

## 2. Report state for an unrecognized feature — expect `feature_unknown`

```sh
/tmp/ai-dev-004 workflow state does-not-exist
# ai-dev: outcome=feature_unknown feature="does-not-exist"
echo $?   # 1
```

## 3. Report state for a feature with a valid, non-default record

```sh
printf '{"state":"planned"}' > specs/001-sample-feature/workflow-state.json
/tmp/ai-dev-004 workflow state 001-sample-feature
# ai-dev: outcome=state_reported feature="001-sample-feature" state=planned terminal=false

/tmp/ai-dev-004 workflow state --format json 001-sample-feature
# {"feature_id":"001-sample-feature","state":"planned","terminal":false,"outcome":"state_reported"}
```

## 4. Report state for a feature with a malformed record — expect `state_invalid`, never `draft`

```sh
printf '{"state":"Planned"}' > specs/001-sample-feature/workflow-state.json   # wrong case
/tmp/ai-dev-004 workflow state 001-sample-feature
# ai-dev: outcome=state_invalid feature="001-sample-feature"
echo $?   # 1

printf '' > specs/001-sample-feature/workflow-state.json   # empty file, not valid JSON
/tmp/ai-dev-004 workflow state 001-sample-feature
# ai-dev: outcome=state_invalid feature="001-sample-feature"

printf '{}' > specs/001-sample-feature/workflow-state.json   # valid JSON, absent state field
/tmp/ai-dev-004 workflow state 001-sample-feature
# ai-dev: outcome=state_invalid feature="001-sample-feature"
```

Restore a valid record before continuing:

```sh
printf '{"state":"planned"}' > specs/001-sample-feature/workflow-state.json
```

## 5. Evaluate a valid transition

```sh
/tmp/ai-dev-004 workflow transition 001-sample-feature tasked
# ai-dev: outcome=transition_valid feature="001-sample-feature" current=planned target=tasked
echo $?   # 0
```

## 6. Evaluate an invalid transition (skip-ahead)

```sh
/tmp/ai-dev-004 workflow transition 001-sample-feature approved
# ai-dev: outcome=transition_invalid feature="001-sample-feature" current=planned target=approved
echo $?   # 1
```

## 7. Evaluate a terminal-origin transition

```sh
printf '{"state":"abandoned"}' > specs/001-sample-feature/workflow-state.json
/tmp/ai-dev-004 workflow transition 001-sample-feature specified
# ai-dev: outcome=transition_invalid feature="001-sample-feature" current=abandoned target=specified
```

## 8. The two formerly-ambiguous precedence combinations (FR-029)

Unsupported target wins over feature existence — restore the valid record first, then try a
nonexistent feature with a bad target:

```sh
/tmp/ai-dev-004 workflow transition does-not-exist reviewing
# ai-dev: outcome=target_unsupported feature="does-not-exist" target="reviewing"
```

Unsupported target wins over record validity — make the record malformed, then supply a
non-canonical target:

```sh
printf 'not json' > specs/001-sample-feature/workflow-state.json
/tmp/ai-dev-004 workflow transition 001-sample-feature reviewing
# ai-dev: outcome=target_unsupported feature="001-sample-feature" target="reviewing"
```

Both commands report `target_unsupported`, never `feature_unknown`/`state_invalid` — matching
`contracts/workflow-transition.md`'s precedence table. (Proving the record is never *read* for
these two cases requires the recording-`fs.FS` unit test in `internal/workflow/evaluate_test.go`;
this black-box CLI step can only observe the correct outcome, not the absence of an `Open` call.)

## 9. Machine-readable mode and filesystem non-mutation

```sh
# Capture the complete scratch-tree state immediately before the command under test. The archives
# live outside "$WORK", so creating them is not itself a mutation of the observed tree.
tar -cf /tmp/ai-dev-004-before.tar .

/tmp/ai-dev-004 workflow transition --format json 001-sample-feature reviewing
# {"feature_id":"001-sample-feature","supplied_target":"reviewing","outcome":"target_unsupported"}

tar -cf /tmp/ai-dev-004-after.tar .
cmp /tmp/ai-dev-004-before.tar /tmp/ai-dev-004-after.tar
echo $?   # 0: the command created, modified, and deleted nothing
```

## Cleanup

```sh
cd /
rm -rf "$WORK"
rm -f /tmp/ai-dev-004-before.tar /tmp/ai-dev-004-after.tar
```
