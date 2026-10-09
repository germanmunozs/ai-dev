# Quickstart: validating `ai-dev workflow approve` / `abandon` (006)

This is a validation guide, not an implementation guide. The command shapes are in
[contracts/workflow-gate.md](./contracts/workflow-gate.md), and the persisted layout is in
[contracts/human-gate-record.md](./contracts/human-gate-record.md).

## Prerequisites

- Go 1.27.1, as declared in `go.mod`.
- macOS or Linux. Human Gate persistence is supported only there ([research §13](./research.md)).
  The POSIX store constraint is `(darwin && !ios) || (linux && !android)`; the fallback constraint
  is `!((darwin && !ios) || (linux && !android))`. On Windows, iOS, Android, and every other
  unsupported platform, `approve` and `abandon` end in `persistence_failed` with no writes after
  steps 1–7, which is covered by structural selection checks, cross-builds, and the
  unsupported-platform unit test.
- A Linux environment, for the required Linux store gate (see §1). For example, a container with Go
  1.27.1.
- Do not run as root; permission-denial fixtures skip themselves under root.

## 1. Automated gates

```sh
gofmt -l .                 # expect: no output
go vet ./...               # expect: exit 0
go build ./...             # expect: exit 0
go test ./...              # expect: ok for every package, including the unchanged 001–005 tests
go test -race -count=20 ./internal/gatestore/ ./internal/gate/   # expect: ok (concurrency)
```

The Windows unsupported-platform gate (T049) uses this three-stage procedure. Save it as a
separate Bash script and execute that script from the repository root with **Bash 4.4 or later**
(macOS `/bin/bash` 3.2 is unsupported). Do not paste the setup lines into an interactive shell.
The module path is `ai-dev` (verified in `go.mod`):

```bash
set -euo pipefail
export GOOS=windows GOARCH=amd64

# Stage 1: Cross-build all production packages.
go build ./...

# Stage 2: Vet all packages except the two with incompatible legacy tests.
all_packages=$(go list ./...)
vet_packages=$(printf '%s\n' "$all_packages" |
  grep -v -x -e 'ai-dev/cmd/ai-dev' -e 'ai-dev/internal/system')
mapfile -t packages <<<"$vet_packages"
test "${#packages[@]}" -eq "$(( $(printf '%s\n' "$all_packages" | wc -l) - 2 ))"

go vet "${packages[@]}"

# Stage 3: Vet Windows-selected production files in the two excluded packages.
for package in ./cmd/ai-dev ./internal/system; do
  file_list=$(go list -f '{{range .GoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}' "$package")
  test -n "$file_list"
  mapfile -t files <<<"$file_list"
  go vet "${files[@]}"
done
```

The original Windows `go vet ./...` failure predates Feature 006 and was independently reproduced
against clean HEAD `666abd7`. Its two incompatible legacy references are `cmd/ai-dev/main_test.go:171`
(`syscall.Kill`) and `internal/system/oslauncher_test.go:164` (`syscall.Kill`). Stage 1 cross-builds
every production package for Windows. Stage 2 vets all other packages, including their
Windows-compatible tests. Stage 3 vets Windows-selected production files in `cmd/ai-dev` and
`internal/system`, selected with `go list` to respect Windows build constraints. The two excluded
legacy test files are outside Feature 006's modification scope under FR-035 and T042. No
production-code validation is waived, and no Feature 006 test that the original Windows command
could analyze is excluded. `set -euo pipefail` makes validation fail if any stage fails. This is a
human-approved amendment to T049's validation procedure, not a change to Feature 006 functional
requirements, security guarantees, or the human authority model; it does not claim Windows
persistence support.

The platform-audit D-1 selection gate additionally uses `go list` and representative
target-compilation checks for macOS, Linux, iOS, Android, and Windows. It must show that only
macOS/Linux select `commit_posix.go` and `lock_posix.go`, while iOS/Android/Windows select
`new_other.go`. For iOS, this verifies package/file selection and compilation; it does not claim a
linked, installable, or runnable iOS executable. Claude's technical review of the corrected
constraints returned PASS as independent evidence, not human authorization. This is not native
Windows validation.

H-7 is **PENDING — DEFERRED** and non-blocking for the current macOS/Linux scope. Reactivate and run
its preserved real-host criteria before distributing or claiming Windows persistence support,
before changing the Windows fallback behavior, or when native Windows validation becomes a release
requirement.

**Required Linux gate**: run the same `go test ./...` and the `-race` line on Linux (ext4, xfs, or
tmpfs) against the exact source tree proposed for merge:

```sh
podman run --rm -v "$PWD":/src:Z -w /src docker.io/library/golang:1.27.1 \
  sh -c 'go test ./... && go test -race -count=20 ./internal/gatestore/ ./internal/gate/'
# expect: ok. Record the output in the PR (the Linux flock semantics were not verified empirically during planning).
```

H-3 first passed for the source tree before D-1. Under platform-audit D-2, the corrected tree then
required reproducible macOS/Linux SHA-256 source manifests and renewed Linux full-suite and race
logs. That renewal was initially pending; the first independent audit returned FAIL with BLK-1,
BLK-1 was corrected, and the follow-up returned `D-2 EVIDENCE PASS` with no blockers. On
2026-10-08, the responsible human accepted the renewed evidence at
`/Users/german/ai-dev-evidence/feature-006/h3-linux-d2-renewal-2026-10-08/`: `SHA256SUMS` SHA-256
`696e623eb98a67058644bf4935de363acf91b20a3382f7073c6cf21c42616b03`; matching 248-entry tree
manifests SHA-256 `595b861b041b8f93b10479432206a8a69ba50d5bd8778bcb2ffd1cb6121aa21f`.
NB-1 through NB-4 and the package's known synchronization-log limitations remain non-blocking.
The `595b861b…` identity describes the accepted snapshot before the documentation changes that
recorded the acceptance and later audit corrections. Go production code and tests remain identical
to that snapshot; the current documentation differs.
This acceptance does not validate Windows or approve the Human Implementation Gate.

In a separate later human decision on 2026-10-08, the responsible human for Feature 006 approved
the implementation for the supported macOS/Linux scope: **Human Implementation Gate:
`APPROVED — macOS/Linux scope`**. H-7 remains `PENDING — DEFERRED`; native Windows behavior remains
unvalidated, no complete multiplatform validation is claimed, and FR-037/SC-014 remain unchanged.
The decision does not approve H-7 or authorize merge or release.

## 2. Manual walk-through in a scratch repository

Run these commands from a scratch copy so that the real `specs/` directory is never touched:

```sh
go build -o /tmp/ai-dev ./cmd/ai-dev
scratch=$(mktemp -d) && mkdir -p "$scratch/specs/900-demo" && cd "$scratch"
touch specs/900-demo/spec.md

/tmp/ai-dev workflow approve --approver alice 900-demo specified; echo "exit=$?"
# expect: outcome=transition_applied … from=draft to=specified … not verified; exit=0
/tmp/ai-dev workflow state 900-demo          # expect: state=specified
cat specs/900-demo/workflow-state.json         # expect: "state": "specified" and one human_gates entry

/tmp/ai-dev workflow approve --approver alice 900-demo specified; echo "exit=$?"
# expect (replay): outcome=transition_invalid … exit=1; file unchanged

/tmp/ai-dev workflow approve --approver alice 900-demo planned; echo "exit=$?"
# expect: outcome=artifacts_missing missing=plan.md, exit=1; file unchanged

touch specs/900-demo/plan.md
/tmp/ai-dev workflow approve --approver alice --reason "plan ok" 900-demo planned   # transition_applied
/tmp/ai-dev workflow next 900-demo           # expect: step_recommended activity=tasks

/tmp/ai-dev workflow abandon --approver bob 900-demo abandoned; echo "exit=$?"
# expect: usage failure (reason required), exit=2, empty stdout
/tmp/ai-dev workflow abandon --approver bob --reason x 900-demo Abandoned; echo "exit=$?"
# expect: usage failure (C-13), exit=2
/tmp/ai-dev workflow approve --approver bob 900-demo abandoned; echo "exit=$?"
# expect: usage failure (C-13), exit=2
/tmp/ai-dev workflow approve --approver bob 900-demo Planned; echo "exit=$?"
# expect: outcome=target_unsupported, exit=1

/tmp/ai-dev workflow abandon --approver bob --reason "superseded" 900-demo abandoned
# expect: transition_applied to=abandoned; human_gates now has 3 entries, the first 2 unchanged
/tmp/ai-dev workflow next 900-demo           # expect: workflow_terminal
```

### Concurrency and lock recovery

There is no lock file to create or delete: the lock is a kernel `flock` on the feature directory,
held only while a commit is running. The behaviors are covered by automated tests, which give
deterministic timing:

```sh
go test ./internal/gatestore/ -run 'Lock|Crash|PostCommit' -v
# expect: ok. The tests assert the following:
#  - an active holder (in-process or another process) gives state_conflict, with nothing written;
#  - after the holding process is SIGKILLed, the next commit succeeds immediately (automatic recovery);
#  - an ambiguous flock error gives persistence_failed, with nothing written and nothing removed;
#  - a kill at any commit step leaves the complete prior or the complete new record, and the next gate succeeds;
#  - a post-commit flush or close failure gives transition_applied plus a warning, and the next gate succeeds.
go test ./cmd/ai-dev/ -run WorkflowGateConcurrent
# expect: ok. Of 8 concurrent ai-dev processes, exactly one gets transition_applied; the others get state_conflict.
```

Manual sanity check, in the scratch repository from above:

```sh
mkdir -p specs/901-demo && touch specs/901-demo/spec.md specs/901-demo/.workflow-state.lock
/tmp/ai-dev workflow approve --approver alice 901-demo specified; echo "exit=$?"
# expect: transition_applied, exit=0 — an unrelated file with the old lock name never blocks a gate
```

### Output failure after commit (macOS/Linux)

A shell pipeline cannot make this check deterministic, because whether the write fails depends on
pipe timing. Run the automated subprocess test, which closes the pipe's read end before the
process starts:

```sh
go test ./cmd/ai-dev/ -run WorkflowGateBrokenPipe
# expect: ok. The test asserts exit 2, the stderr text "write workflow gate report", and that
# `workflow state` afterwards reports the committed target (the commit is not rolled back).
```

## 3. Read-only and authority checks

- `go test ./internal/app/ -run Authority` snapshots a fixture repository and runs every
  001–005 command against it. This includes a fake agent that reports `agent_succeeded` and
  creates `plan.md`. It then asserts that `workflow-state.json` is byte-identical and that no
  `human_gates` entry exists.
- `go test ./internal/gate/ ./internal/gatestore/ -run Boundary` checks the import allow-lists and
  enforces the sole write path.

## 4. Cleanup

```sh
cd / && rm -rf "$scratch" /tmp/ai-dev
```
