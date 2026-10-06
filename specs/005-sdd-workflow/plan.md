# Implementation Plan: SDD Workflow

**Branch**: `feature/005-sdd-workflow` | **Date**: 2026-10-05 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-sdd-workflow/spec.md` (Human Specification Gate
approved by the human)

## Summary

This plan adds one strictly read-only, non-interactive command:

```text
ai-dev workflow next [--format text|json] <feature-id>
```

The command reports which SDD activity, and which 003 development role, corresponds to a feature's
current Workflow State. It also reports state/artifact inconsistencies and terminal workflows. It
never executes or selects agents, never evaluates or applies transitions, never writes anything,
and never grants or records approval.

A new package, `internal/sdd`, owns only what the spec assigns to 005:
- the five SDD activities;
- the Next-Step Table (state → activity → role, with no target);
- the Required Artifacts rule;
- the five outcomes and the report invariants.

Everything 004 owns is reused through its existing API, unchanged:
- feature recognition and structural feature-ID validation;
- symlinked feature directories;
- record resolution;
- canonical states;
- terminality.

Steps 2–3 call `workflow.EvaluateState` itself. Terminality is read from `StateReport.Terminal`.

004 gets one minimal, I/O-free extension, `workflow.FeatureDir(id) (string, bool)`. Its
`FeatureExists` and `ResolveRecord` adopt it in a behavior-preserving refactor. This gives 005 the
directory 004 recognized, without a second copy of the `specs/<id>` location rule
([research §2](./research.md)).

Artifact presence is decided from a single `fs.ReadDir` of that directory: an exact, byte-equal
name match plus `DirEntry.Type().IsRegular()`. This yields case-exact results on case-insensitive
hosts and never follows symlinks. The approach was verified on this repository's macOS
case-insensitive host ([research §3](./research.md)).

## Technical Context

**Language/Version**: Go 1.27.1 (existing `go.mod`, unchanged)

**Primary Dependencies**: Go standard library only. No new module dependency. By package:
- `internal/sdd`: `io/fs` and `strings`, an exhaustive allow-list (research §4);
- `internal/report`: `encoding/json`, `fmt`, and `io`;
- `internal/app`: `flag`, `fmt`, `io`, and `io/fs`, plus `os`, `os/signal`, and `syscall` only in
  the Unix-tagged `sigpipe_unix.go` (research §11);
- tests: `go/parser`, `go/ast`, and `go/token`.

**Storage**: None written. The command reads the following:
- the 004 Workflow State Record (`specs/<id>/workflow-state.json`), through 004's code;
- one directory listing of `specs/<id>/`.

**Testing**: `go test`, white-box tests inside each package, as in 001–004.
- Fixtures: `testing/fstest.MapFS`, including `fs.ModeSymlink` entries and `ReadLinkFS`.
- Real-filesystem fixtures: `os.DirFS(t.TempDir())`, with real symlinks and permission fixtures.
- New wrappers, all in test files:
  - a recording `fs.FS` (`Open`, `Lstat`, `ReadLink`, `ReadDir`);
  - a case-insensitive wrapper;
  - a fault-injecting `ReadDir` wrapper;
  - an always-erroring `io.Writer`.
- A subprocess contract test builds the binary. It includes a Unix-only real broken-pipe test that
  uses an `os.Pipe` whose read end is closed before the process starts (research §11).
- No live agent or network access is involved.

**Target Platform**: Same as existing `ai-dev`. The semantics are platform-independent. Real
symlink and permission fixtures are skipped on Windows, and when running as root (where
permission denial cannot be produced).

**Project Type**: Single Go module CLI (`cmd/`, `internal/`)

**Performance Goals**: Not performance-sensitive. One request makes at most these filesystem
calls:
- two `Lstat` calls (004);
- one record read (004);
- one `ReadDir` (005).

**Constraints**: All of the following hold:
- Read-only (FR-004), enforced structurally because only `fs.FS` is passed.
- No agent, selection, or execution coupling (FR-002), enforced structurally because no
  executor, launcher, or registry is passed, plus an import-boundary test.
- No transition knowledge (FR-003), enforced by an identifier-boundary test.
- The fixed precedence (FR-014).
- No stdin (FR-021), because no stdin is passed.
- Deterministic output (FR-022).
- 001–004 behavior unchanged (FR-026). No existing test assertion is removed or weakened.
- Output failure, including a real broken pipe, exits 2 (FR-018, FR-020). This is achieved with a
  SIGPIPE → `EPIPE` window scoped to `workflow next` only (research §11).

**Scale/Scope**: The command has these fixed sets:
- 7 states, read from 004;
- 5 activities;
- 4 roles used, out of 003's 5;
- 3 artifact names;
- 5 outcomes;
- 3 exit codes.

## Constitution Check (pre-design)

*GATE: Must pass before Phase 0 research.*

| Principle | Assessment |
|---|---|
| I. Specification First | The spec passed two independent reviews plus a final re-review, and the human approved it at the Human Specification Gate. The plan implements its contract without changing any functional decision. PASS |
| II. Architecture Before Framework | Design follows the spec's entities and the boundaries 001–004 already have. No framework or library choice drives it. PASS |
| III. Simplicity and YAGNI | The plan adds one package, one subcommand, one I/O-free 004 helper, and two switch-based rule functions. There is no persistence, no plugin system, no configuration, and no generic workflow engine. PASS |
| IV. Testable by Design | Every FR and SC maps to a planned test (see Test Strategy). The precedence is visible as straight-line code and is provable with recording-FS call logs. PASS |
| V. Security by Design | Traversal, absolute, nested, invalid, and symlinked IDs are delegated to 004's tested containment. Artifacts are inspected with no-follow `ReadDir` types. There is no write path and no secrets. A security audit gate is defined. PASS |
| VI. Observability by Design | Not a backend or distributed system. The report itself is the observable output. N/A |
| VII. Reproducibility and Automation | All gates are scripted Go toolchain commands. Case-insensitive coverage is simulated so it runs on every CI host. PASS |
| VIII. AI-Assisted Engineering | Same version-controlled artifacts for humans and agents. No agent coupling in the feature itself. PASS |
| IX. Human Authority | The plan asserts no Plan Gate approval. Everything 005 produces is non-authoritative by contract (FR-023). PASS |

No violations, so the Complexity Tracking section stays empty.

## Project Structure

### Documentation (this feature)

```text
specs/005-sdd-workflow/
├── spec.md                          # approved specification
├── checklists/requirements.md       # spec quality checklist
├── plan.md                          # this file
├── research.md                      # Phase 0 decisions
├── data-model.md                    # Phase 1 entities and invariants
├── quickstart.md                    # Phase 1 validation guide
└── contracts/
    ├── workflow-next.md             # CLI contract (usage, outcomes, streams, text, JSON)
    ├── next-step-rules.md           # Next-Step Table and Required Artifacts as code
    └── workflow-featuredir.md       # minimal 004 API extension
```

`tasks.md` is not created by this phase.

### Source Code (repository root)

```text
internal/sdd/                        # NEW: 005-owned domain (imports only workflow, role, stdlib)
├── activity.go                      # Activity type + 5 constants; artifact-name constants
├── rules.go                         # nextStep(), requiredArtifacts(), missingArtifacts()
├── outcome.go                       # Outcome constants, Report struct
├── evaluate.go                      # Evaluate(root, featureID) Report — precedence steps 2–6
├── validate.go                      # Report.Valid() — FR-016 field table as code
├── rules_test.go                    # table exhaustiveness, matrix, exact case, symlink, unreadable
├── evaluate_test.go                 # every outcome, precedence, call-log proofs, determinism
├── validate_test.go                 # every valid report + every single-field mutation
├── casefs_test.go                   # simulated case-insensitive FS + real-host fixture
└── boundary_test.go                 # go/parser import allow-list, alias-resolved selector checks

internal/workflow/
├── record.go                        # MODIFIED: + FeatureDir; FeatureExists/ResolveRecord use it
└── featuredir_test.go               # NEW: FeatureDir table + path-equivalence call-log tests

internal/report/
├── workflownext.go                  # NEW: WriteWorkflowNextText / WriteWorkflowNextJSON
└── workflownext_test.go             # NEW: exact lines, exact JSON key sets, injection safety

internal/app/
├── doctor.go                        # MODIFIED: one `args[1] == "next"` arm in the workflow branch
├── workflownext.go                  # NEW: RunWorkflowNext(args, root, stdout, stderr) int
├── sigpipe_unix.go                  # NEW (//go:build unix): brokenPipeAsError() — scoped SIGPIPE Notify/Stop
├── sigpipe_other.go                 # NEW (//go:build !unix): no-op brokenPipeAsError()
└── workflownext_test.go             # NEW: usage, outcomes, exit codes, streams, mutation, output failure

cmd/ai-dev/
├── workflownext_contract_test.go    # NEW: subprocess contract test against the built binary
└── workflownext_brokenpipe_test.go  # NEW (//go:build unix): real broken-pipe tests + workflow-state isolation control
```

**Structure Decision**: This keeps the existing single-module layout. The new code follows the
layout that 004 established, with domain, report, and app layers in separate packages. Only two
existing production files change:
- `internal/workflow/record.go`: the additive `FeatureDir` and a behavior-preserving refactor;
- `internal/app/doctor.go`: one dispatch arm.

New 005 tests go in new files by default. An existing test file MAY be extended when that is the
best way to prove compatibility or dispatch, for example by adding `workflow next` rows to an
existing table. Such changes must be additive and justified, and no existing assertion may be
removed or weakened (P-05).

## Design

### Dependency boundary

```text
app.Run(ctx, args, executor, launcher, root, stdin, stdout, stderr)
  └─ args = workflow next …  →  RunWorkflowNext(args, root, stdout, stderr)
                                  │  (no executor, launcher, registry, stdin)
                                  └─ sdd.Evaluate(root, featureID)
                                       ├─ workflow.EvaluateState(root, id)    // 004, unchanged
                                       ├─ workflow.FeatureDir(id)             // 004, I/O-free
                                       └─ fs.ReadDir(root, dir)               // 005, one listing
```

`internal/sdd` may import exactly four packages: `io/fs`, `strings`, `ai-dev/internal/workflow`,
and `ai-dev/internal/role`. Every other import fails the test, including `os`, `os/exec`,
`os/signal`, `net`, `syscall`, `path`, `path/filepath`, and every other internal package
(research §4).

`boundary_test.go` parses every non-test file of `internal/sdd` and checks the following:
- **Imports:** only the allow-list.
- **Aliases:** it resolves each file's import aliases, so `wf "ai-dev/internal/workflow"` is
  still recognized as the workflow package. Dot and blank imports of internal packages are
  rejected.
- **Workflow selectors:** it rejects any selector, through any alias, on the workflow package
  that names `Allowed`, `EvaluateTransition`, `TransitionReport`, `TransitionValid`,
  `TransitionInvalid`, or `TargetUnsupported`.
- **Role selectors:** on the role package, it permits only `Role`, `Specification`, `Planning`,
  `Implementation`, and `Review`.
- **No `"specs"` literal.**
- **Self-check:** it runs its own checker against the twelve known-bad snippets listed in
  research §4 and test item 11, plus one known-good snippet. This proves the checker is neither
  vacuous nor over-strict.

Together these make FR-002, FR-003, FR-004, and FR-008 executable gates.

**Permitted 003 dependency (human decision, P-08)**: `internal/sdd` → `internal/role` is a
deliberate, approved exception to the Independent Plan Review prompt's list. It reuses 003's
canonical role vocabulary by reference instead of duplicating it. `internal/role` is pure
vocabulary: one type, constants, and `Recognized`, with no imports. The exception does **not**
allow role selection, role → agent resolution, or importing `selection` or `agent`, nor any
agent execution or copying of 003's mapping.

`RunWorkflowNext` receives no executor, launcher, registry, or stdin. It therefore has no
language-level means to run a process, select an agent, or read input.

### Precedence as control flow (FR-014)

The six steps form the literal top-to-bottom order of two functions, with early returns:

```go
// internal/app/workflownext.go — step 1
func RunWorkflowNext(args []string, root fs.FS, stdout, stderr io.Writer) int {
	return runWorkflowNext(args, root, stdout, stderr, sdd.Evaluate) // test seam (research §6)
}

func runWorkflowNext(args []string, root fs.FS, stdout, stderr io.Writer, evaluate func(fs.FS, string) sdd.Report) int {
	release := brokenPipeAsError() // Unix: process-wide SIGPIPE→EPIPE for the duration of this call; no-op elsewhere (research §11)
	defer release()
	// flag.ContinueOnError parse; NArg()==1; format ∈ {text,json}; TrimSpace(id)!="" — else usage, return 2
	r := evaluate(root, featureID)
	// if !r.Valid(): nothing to stdout; stderr "ai-dev: internal error: … cannot be rendered …"; return 2
	//                (FR-020 output failure — "cannot be rendered")
	// write r (text|json) to stdout; on error (incl. EPIPE): stderr "write workflow next report: …"; return 2
	//                (FR-020 output failure — "cannot be written/delivered")
	// return 0 for StepRecommended/WorkflowTerminal, 1 otherwise
}

// internal/sdd/evaluate.go — steps 2–6
func Evaluate(root fs.FS, featureID string) Report { return evaluate(root, featureID, nextStep) }

func evaluate(root fs.FS, featureID string, lookup func(workflow.State) (Activity, role.Role, bool)) Report {
	sr := workflow.EvaluateState(root, featureID)                 // steps 2–3 (004)
	switch sr.Outcome {
	case workflow.FeatureUnknown:
		return Report{FeatureID: featureID, Outcome: FeatureUnknown}
	case workflow.StateInvalid:
		return Report{FeatureID: featureID, Outcome: StateInvalid}
	case workflow.StateReported:
	default:
		return Report{FeatureID: featureID} // impossible; fails Valid() → output failure "cannot be rendered", exit 2
	}
	if missing := missingArtifacts(root, featureID, sr.State); len(missing) > 0 { // step 4
		return Report{FeatureID: featureID, State: sr.State, Missing: missing, Outcome: ArtifactsInconsistent}
	}
	if sr.Terminal {                                               // step 5 (004 terminality)
		return Report{FeatureID: featureID, State: sr.State, Outcome: WorkflowTerminal}
	}
	activity, r, ok := lookup(sr.State)                            // step 6
	if !ok {
		return Report{FeatureID: featureID} // FR-020 "cannot be rendered": no outcome, no activity/role; unreachable per FR-007 (research §6)
	}
	return Report{FeatureID: featureID, State: sr.State, Activity: activity, Role: r, Outcome: StepRecommended}
}
```

This sketch illustrates the control flow; the exact code belongs to implementation. The two
unexported seams, `evaluate(…, lookup)` and `runWorkflowNext(…, evaluate)`, exist only so the
unreachable "cannot be rendered" output failure can be tested defensively. Production always passes `nextStep` and
`sdd.Evaluate`, so no user-reachable configuration exists. The
precedence is provable behaviorally through recording-FS call logs:

| Result | Calls recorded |
|---|---|
| Usage failure | none |
| Structurally invalid ID | none |
| Absent feature | 1 `Lstat`, 0 `ReadDir` |
| `state_invalid` | feature + record access, 0 `ReadDir` |
| `draft` / `abandoned` | 0 `ReadDir` |
| States with requirements | exactly 1 `ReadDir`, on `specs/<id>` |

### Integration with 004 (reuse, not duplication)

| Concern | Mechanism | 005 code that would duplicate it |
|---|---|---|
| Structural ID validation, traversal, absolute and nested IDs | `EvaluateState` → `FeatureExists` → `validFeatureID` | none |
| Symlinked feature directory | `FeatureExists` no-follow `Lstat` | none |
| Record location, reading, `draft` default, `state_invalid` | `EvaluateState` → `ResolveRecord` | none |
| Canonical states | `workflow.State` constants | none (only used as switch keys) |
| Terminality | `StateReport.Terminal` (computed by `workflow.Terminal`) | none |
| Feature directory path | `workflow.FeatureDir` (new, shared with `FeatureExists` and `ResolveRecord`) | none |
| Transitions | not used at all | none |

### Artifact inspection, exact case, no-follow

See [contracts/next-step-rules.md](./contracts/next-step-rules.md) and
[research §3](./research.md). The approach has five properties:
- One `fs.ReadDir` of the recognized directory.
- Presence requires `entry.Name() == canonical` (byte-exact) and `entry.Type().IsRegular()`.
- No per-artifact lookup is ever made. On case-insensitive hosts a lookup would resolve `Plan.md`
  for `plan.md`; this was observed directly on this repository's host.
- Symlinks, directories, and other non-regular entries are missing. A listing error makes every
  required artifact missing.
- Content is never opened.

### Reporting

The formats are specified in [contracts/workflow-next.md](./contracts/workflow-next.md).
`internal/report/workflownext.go` follows `workflowstate.go`:
- a `Valid()` gate;
- one `%q`-quoted single text line, or one JSON object with an omit-when-prohibited wire struct;
- `missing_artifacts` as a JSON array;
- normal reports always on stdout.

Nothing in the report types can carry an agent, approval, authorization, target, or transition
value, because no such field exists in `sdd.Report`.

### Human Authority preservation (FR-023)

| Separation | Where it is enforced |
|---|---|
| recommendation ≠ authorization ≠ transition ≠ approval | No transition API reachable (boundary test). The `step_recommended` disclaimer is in the text form. No approval or authorization field exists. |
| `agent_succeeded` ≠ workflow state ≠ approval | No execution package imported. Inputs are only the record and the directory listing. |
| artifact presence ≠ transition ≠ approval | Artifacts affect only step 4. `State` always comes from 004 (test: extra artifacts never change the result). |
| exit 0 ≠ approval | The exit code depends only on the outcome. Tests show `approved` and `abandoned` both give 0, with output carrying no approval wording. |
| observed `approved` ≠ approval granted by 005 | `approved` appears only as `current_state`/`state=`. A test scans all outputs for forbidden approval or authorization wording and keys. |

### CLI and dispatcher

`app.Run`'s existing `workflow` branch gains one line:

```go
if len(args) >= 2 && args[1] == "next" { return RunWorkflowNext(args[2:], root, stdout, stderr) }
```

- `Run`'s signature is unchanged.
- The group usage string `Usage: ai-dev workflow <state|transition> [options]` is deliberately
  unchanged byte for byte, by human planning decision ([research §7](./research.md)). A 004
  test also asserts it.
- No Feature 007 concepts are added: no prompts, menus, or interactive mode.

## Quality Gates

All gates are automated, run locally, and need no live agent, service, or network access.

| Gate | Command | Pass condition |
|---|---|---|
| Formatting | `gofmt -l .` | No output |
| Vet | `go vet ./...` | Exit 0, no diagnostics |
| Tests | `go test ./...` | All pass |
| Tests, uncached | `go test -count=1 ./...` | All pass |
| Race | `go test -race ./internal/sdd/... ./internal/app/...` | All pass |
| Build | `go build ./cmd/ai-dev` | Succeeds. `go.mod` and `go.sum` unchanged |
| Whitespace | `git diff --check` | No errors |
| Boundary | `go test ./internal/sdd -run TestPackageBoundary` | Passes: import allow-list `{io/fs, strings, workflow, role}`; alias-resolved forbidden selectors; no `"specs"` literal; the self-check rejects all twelve known-bad snippets and accepts the known-good one |
| Broken pipe (Unix) | `go test ./cmd/ai-dev -run TestWorkflowNextBrokenPipe` | Passes: not signaled; exit 2; isolation control for `workflow state` unchanged |
| Existing-test review | `git diff --diff-filter=M main...HEAD -- '*_test.go'`, run on the committed feature branch | A **review input**, not an automated pass/fail. It lists every modified pre-existing test hunk. The reviewer confirms that each change is additive and justified, and that no existing assertion is removed or weakened. It is not evaluated against the pre-commit working tree. |

### Security / scope audit gate (manual review of the diff)

- No new dependency (`go.mod` and `go.sum` unchanged).
- Feature IDs:
  - Traversal, absolute, nested, structurally invalid, and symlinked-directory IDs all resolve
    through 004 to `feature_unknown`, with zero `ReadDir` calls.
  - No artifact path is built before `state_reported` (FR-006).
- Artifacts:
  - Symlinked artifacts are never followed and count as missing.
  - Unreadable listings make the required artifacts count as missing.
  - Artifact content is never opened. There is no `Open`, `Stat`, `Lstat`, or `ReadFile` call on
    any artifact path, and no permission probing (research §3, human decision).
  - No `"specs"` literal appears in `internal/sdd`; the directory comes only from
    `workflow.FeatureDir`.
- Exact-case artifact names are matched against the listing, never through a lookup.
- No `os` write or create call, and no `os.Remove` call, in any new production file. Only `fs.FS`
  is used.
- No secret, token, credential, or environment value appears in any output or fixture.
- No routing, retry, fallback, chaining, background goroutine, persistence, stdin read, or
  prompt.
- `internal/sdd` imports only `{io/fs, strings, workflow, role}`; the boundary test enforces this.
  `RunWorkflowNext` has no executor or launcher parameter.
- Signal handling:
  - `os/signal` and `syscall` appear only in `internal/app/sigpipe_unix.go`;
  - `brokenPipeAsError` is called only from `runWorkflowNext`;
  - there is no `signal.Ignore`, no `init`-time registration, and no change to `cmd/ai-dev/main.go`;
  - `signal.Stop` is deferred on every return path.

## Test Strategy

Each item lists the test location and what it proves. Spec references are given in brackets.

**Domain: `internal/sdd`**
1. **Table exhaustiveness** (`rules_test.go`) [FR-007, FR-008, SC-002, SC-003]:
   - across all 7 states, `nextStep` is `ok` if and only if `!workflow.Terminal`;
   - the exact 5 rows;
   - every role is `role.Recognized`, and `Remediation` never appears;
   - activity tokens are disjoint from state tokens.
2. **Artifact matrix** (`rules_test.go`) [FR-010, FR-011, SC-004]:
   - `requiredArtifacts` returns the exact rows for all 7 states;
   - every non-empty subset of missing required artifacts, for each state that has requirements,
     produces exactly that missing list in canonical order;
   - the returned slices are fresh copies (mutating one does not affect the next call).
3. **Exact case** (`casefs_test.go`) [FR-010, FR-022, SC-012, US3 AS5]:
   - simulated case-insensitive FS with `Plan.md`, `SPEC.md`, and `Tasks.md` → missing;
   - a self-check proves the wrapper resolves lookups case-insensitively;
   - a real `os.DirFS` temp fixture gives the identical result, with the host's case behavior
     logged.
4. **No-follow and non-regular** (`rules_test.go`) [FR-010]:
   - `MapFS` and real-FS symlinks named `spec.md` (to a valid file, dangling, and to a directory)
     → missing;
   - a directory named `plan.md` → missing;
   - a real-FS test proves the symlink target is never opened (the recording FS sees no access to
     the target).
5. **Unreadable means uninspectable through metadata** (`rules_test.go`) [FR-010; human decision,
   research §3]:
   - a fault-injecting `ReadDir` (error, and partial entries with an error) → all required
     artifacts missing;
   - a real-FS feature directory with mode `0311`, which can be traversed but not listed (skipped
     when running as root or on Windows) → `artifacts_inconsistent`;
   - a regular file with mode `000` → present (content is never read);
   - a recording FS proves that zero `Open` or `ReadFile` calls are made on any artifact path, in
     every fixture.
6. **Every outcome and precedence** (`evaluate_test.go`) [FR-013, FR-014, US1–US3]:
   - one fixture per outcome and per state;
   - precedence pairs:

     | Fixture | Outcome |
     |---|---|
     | invalid record **and** missing artifacts | `state_invalid` |
     | `approved` with an artifact missing | `artifacts_inconsistent` |
     | `abandoned` with no artifacts | `workflow_terminal` |
     | `draft` with no artifacts | `step_recommended` |
     | `specified` with extra plan/tasks | `plan` |

   - recording-FS call logs per outcome, as in the Design section.
7. **Invalid IDs** (`evaluate_test.go`) [FR-005, FR-006, SC-007, US3 AS7–AS8]:
   - `..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`, and a symlinked feature directory →
     `feature_unknown`;
   - zero `ReadDir` calls in every case, and zero filesystem calls for the structurally invalid
     IDs.
8. **004 parity** (`evaluate_test.go`) [FR-005]: for a fixture corpus containing every 004
   `state_invalid` sub-case (SC-010 of 004) and every C-1 ID, 005's `feature_unknown` and
   `state_invalid` results are compared against `workflow.EvaluateState`'s, case by case.
9. **Report invariants** (`validate_test.go`) [FR-016]:
   - every valid report passes;
   - each required field blanked and each prohibited field populated, one at a time, fails;
   - also failing: activity/role inconsistent with state, terminal/non-terminal mismatch,
     unordered or duplicate or non-required missing names, and an unknown outcome.
10. **Determinism** (`evaluate_test.go`) [FR-022, SC-009]: repeated `Evaluate` calls on unchanged
    fixtures return `reflect.DeepEqual` reports.
11. **Boundary** (`boundary_test.go`) [FR-002, FR-003, FR-004, FR-008]:
    - the exhaustive import allow-list `{io/fs, strings, ai-dev/internal/workflow,
      ai-dev/internal/role}`;
    - alias resolution per file; dot and blank internal imports rejected;
    - forbidden workflow selectors (`Allowed`, `EvaluateTransition`, `TransitionReport`, and the
      transition outcome constants), through any alias;
    - role selectors limited to `Role`, `Specification`, `Planning`, `Implementation`, and
      `Review`;
    - no `"specs"` literal;
    - a self-check on known-bad snippets. Each must be rejected; this is the same list as
      research §4:
      1. `os`
      2. `os/exec`
      3. `os/signal`
      4. `net`
      5. `syscall`
      6. the forbidden internal import `ai-dev/internal/selection`
      7. aliased `wf "ai-dev/internal/workflow"` with `wf.Allowed`
      8. aliased `r "ai-dev/internal/role"` with `r.Remediation`
      9. `role.Remediation`
      10. blank `_ "ai-dev/internal/selection"`
      11. dot `. "ai-dev/internal/workflow"`
      12. a `"specs"` string literal
    - a known-good snippet (allowed imports, `workflow.FeatureDir`, `workflow.EvaluateState`,
      `role.Planning`) must be accepted.
11a. **Impossible table miss** (`evaluate_test.go`) [FR-007, FR-013, FR-020; human decision,
    research §6]:
    - `evaluate` with a lookup lacking `planned`, on a valid `planned` fixture with artifacts
      present;
    - the result has no outcome, activity, or role, and fails `Valid()`;
    - the result is not `artifacts_inconsistent`, `state_invalid`, or any other outcome.

**004 regression: `internal/workflow/featuredir_test.go`** [FR-026, SC-013]

12. The `FeatureDir` table, and path equivalence for `FeatureExists` and `ResolveRecord` through a
    recording `ReadLinkFS`. All pre-existing `internal/workflow` tests run unchanged.

**Reporting: `internal/report/workflownext_test.go`** [FR-016, FR-017]

13. Exact text line per outcome:
    - the `%q` rendering;
    - the disclaimer, only on `step_recommended`;
    - `state=approved` only as the observed current state;
    - injection IDs (`\n`, `"`, `\\`, spaces, a literal `outcome=step_recommended`), with exactly
      one line and the outcome first;
    - no forbidden wording (`approv` outside a state value, `authoriz` outside the disclaimer,
      `gate`, `agent`, `target`, `next_state`).
14. Exact JSON per outcome:
    - decoded into `map[string]any`;
    - an exact key set;
    - value types (`missing_artifacts` is `[]any` of strings);
    - prohibited keys absent rather than null;
    - exactly one object with a trailing newline only;
    - UTF-8 fidelity (P-06, research §8): a valid-UTF-8 ID containing `<`, `>`, `&`, and
      non-ASCII letters round-trips exactly. An ID containing byte `0xff` yields U+FFFD in JSON
      `feature_id` and `\xff` in the `%q` text form. This pins the inherited, documented
      limitation.
15. Every invalid report is rejected by both writers before any byte is written.

**CLI: `internal/app/workflownext_test.go`** [FR-017 to FR-021, FR-004]

16. Usage failures: each class → exit 2, exact stderr usage line, empty stdout, zero FS calls.
    `""` and whitespace IDs are usage failures, while `../x` is a recognized request.
17. Outcomes × formats through `Run`:
    - exact stdout, empty stderr;
    - exit 0 for `step_recommended` and `workflow_terminal`, and 1 for the others;
    - a test asserts that `step_recommended` and `workflow_terminal` share exit 0 and differ only
      in `outcome`, so callers must read the outcome;
    - `approved` and `abandoned` give the same exit code.
18. Output failure:
    - an erroring writer → exit 2;
    - one write attempt, with a recorded write count;
    - a stderr diagnostic;
    - no reclassification.
18a. Output failure, "cannot be rendered" subcase, through the `runWorkflowNext` seam:
    - an injected evaluator returns the impossible report → exit 2 (the output-failure code),
      empty stdout, and the `ai-dev: internal error: … cannot be rendered` diagnostic on stderr;
    - no outcome token on either stream;
    - its diagnostic text differs from the "cannot be written" subcase, but the exit code and
      stream rules are identical.
18b. Broken-pipe handling in-process:
    - a test calls `brokenPipeAsError` and its `release` and confirms that `release` can be
      called safely, so `Stop` is always paired;
    - the real behavior is proven only by the real-binary test (item 21), because SIGPIPE
      semantics apply to file descriptors 1 and 2 of a real process.
19. Non-mutation:
    - real temp-tree snapshot before and after, for each outcome (reusing the 004 helper pattern);
    - the dispatcher's fake executor and launcher record zero lookups, runs, and launches;
    - no stdin is read, since `RunWorkflowNext` has no stdin parameter and the `Run` test passes a
      reader that fails the test if it is read.
20. Dispatcher: the `workflow next` routing works. `TestRunWorkflowDispatchUsageFailures` (004,
    unchanged) still passes with the unchanged group usage.

**Process: `cmd/ai-dev/workflownext_contract_test.go`**

21. `workflownext_contract_test.go`: build the binary and run it against a temp repo for each
    outcome and each usage-failure class. Assert exit codes, exact stdout, and stderr.
    `workflownext_brokenpipe_test.go` (`//go:build unix`, runs on macOS and Linux; research §11):
    - stdout is an `os.Pipe` whose read end is closed **before** start, which gives a
      deterministic, real broken pipe;
    - for both `workflow next f` and `workflow next --format json f`: not signaled, exit 2, and
      `write workflow next report:` on stderr;
    - a usage failure with a broken stderr pipe: not signaled, exit 2;
    - isolation control: `workflow state f` under the same broken pipe is still terminated by
      SIGPIPE. This proves the handler is scoped to `workflow next` and 004 is unchanged.
    - It never uses `>&-` (the Go runtime reopens a closed fd as `/dev/null`) or `/dev/full`
      (absent on macOS).

**Regression across 001–004**

22. `go test -count=1 ./...` passes. Compatibility is proven by assertions, not by file
    immutability:
    - the existing 001–004 tests keep every assertion; any change to an existing test file is
      additive and justified;
    - the new `FeatureDir` path-equivalence and C-1 zero-access tests (item 12);
    - the existing dispatcher tests, which assert exact strings for `ai-dev workflow`,
      `workflow bogus`, the top-level usage, and `workflow state`/`workflow transition`;
    - the broken-pipe isolation control (item 21);
    - the existing-test review gate.

## Compatibility and Regression Strategy (001–004)

| Feature | What could regress | Protection |
|---|---|---|
| 001 doctor | `Run` dispatch | Only the `workflow` branch changes; all doctor assertions kept |
| 002 agents and agent run | `Run` dispatch, executor and launcher use | No change to those branches; 005 never receives executor or launcher |
| 003 agent select | role vocabulary | `internal/role` is imported read-only as vocabulary only (P-08) and not modified; all its assertions kept; `selection` and `agent` are not importable from `internal/sdd` (boundary test) |
| 004 workflow state and transition | `record.go` refactor; dispatcher branch; signal handling | Behavior-preserving `FeatureDir` refactor; all existing 004 assertions kept; additive path-equivalence and C-1 tests; group usage string unchanged byte for byte; SIGPIPE handling never reached by `workflow state`/`transition`, proven by the broken-pipe isolation control |
| All commands | process-wide signal state | `brokenPipeAsError` is installed only inside `runWorkflowNext` and released on return; `main.go` unchanged; no `signal.Ignore`, so no inherited disposition for 002's child processes |

## Constitution Check (post-design)

| Principle | Re-assessment |
|---|---|
| I | The design introduces no functional decision beyond the approved spec. The plan-level choices are explicit human planning decisions, each literally compatible with the approved spec: research §2, §3, §6 (now an FR-020 "cannot be rendered" subcase), §7, and the P-08 role dependency. The SIGPIPE handling (§11) implements FR-018/FR-020 for real delivery failure and adds no new behavior. PASS |
| II | The package boundary follows the spec's ownership table. PASS |
| III | One small package, and one I/O-free helper added to 004 instead of duplicated logic. No speculative abstraction. PASS |
| IV | The test items cover every FR and SC, including real broken-pipe delivery failure (FR-020), which a deterministic, macOS-compatible real-binary test proves. The precedence, the boundary, and case behavior are each provable with dedicated seams. PASS |
| V | Security audit gate defined. The exhaustive import allow-list makes "no write, exec, network, or signal capability in `internal/sdd`" structural. No-follow and exact-case design verified empirically. Signal handling is confined to one Unix-tagged file in `internal/app`. PASS |
| VI | N/A |
| VII | Gates are scripted. Case-insensitive behavior is simulated for CI portability. The broken-pipe test is deterministic (reader closed before start) on macOS and Linux. The git-based check is a review input on the committed branch and not a pass/fail claim. PASS |
| VIII | PASS |
| IX | No approval asserted. FR-023 separations are enforced in the design and tests. All four plan-level choices were made by the human, not by the agent. PASS |

No violations, so Complexity Tracking is not needed.

## Human planning decisions (2026-10-05)

The human decided the four plan-level choices flagged in the first planning pass, the P-08 role
dependency, and the P-01 SIGPIPE window. Each decision is binding on implementation and documented in research.

1. **Workflow usage text unchanged** (research §7). 005 does not modify the existing
   `ai-dev workflow` usage text to list `next`. `ai-dev workflow next …` is supported by dispatch
   only. 004's existing usage and diagnostics are preserved byte for byte, except for the minimal
   dispatch arm. No Feature 007 behavior is anticipated.
2. **"Unreadable" means uninspectable through metadata-only inspection** (research §3). 005 never
   opens or reads artifact content. Presence is decided only from one `ReadDir`:
   - an exact canonical name shown as a regular file → present;
   - failure to list or inspect safely → missing;
   - symlinks and directories → missing;
   - file-byte readability → never probed.

   This is literally compatible with FR-010 and the spec's edge case. `spec.md` is unchanged.
3. **Impossible Next-Step Table miss** (research §6; reclassified in the P-02 remediation).
   - It is an **FR-020 output failure, "cannot be rendered" subcase**. It is not a third exit-2
     category: `step_recommended` is determined at step 6, but the report required by FR-015 and
     FR-016 cannot be built.
   - It invents no activity or role, adds no sixth outcome, and is never `artifacts_inconsistent`
     or `state_invalid`.
   - stdout stays empty, the specific diagnostic `ai-dev: internal error: … cannot be rendered`
     goes to stderr, and the exit code is 2.
   - FR-007 makes it unreachable while the approved table is complete.
   - The test seams `evaluate(…, lookup)` and `runWorkflowNext(…, evaluate)` keep it testable
     (test items 11a and 18a).
4. **`workflow.FeatureDir` approved** (research §2,
   [contracts/workflow-featuredir.md](./contracts/workflow-featuredir.md)). It centralizes only
   the safe derivation of `specs/<id>`, reuses 004's validation unchanged, and performs no I/O.
   The refactor is semantically neutral and protected by regression tests. 005 must use it, and
   the boundary test forbids a `"specs"` literal in `internal/sdd`. Its single, canonical
   description throughout the planning artifacts is: validate first with `validFeatureID`, then
   `path.Join("specs", featureID)`; no I/O; `false` for invalid IDs.
5. **`internal/sdd` may import `internal/role`** (P-08; research §4). This is a deliberate
   exception to the Independent Plan Review prompt's import list. It reuses 003's role vocabulary
   by reference, as types and constants only. It does not authorize role selection, role → agent
   resolution, importing `selection` or `agent`, agent execution, or duplicating 003 mappings.

6. **SIGPIPE window confirmed** (P-01; research §11). The temporary SIGPIPE handling is
   installed at the start of `runWorkflowNext` and released by `defer` on return. It covers every
   write the command performs: the stdout report, stderr usage diagnostics, and stderr
   output-failure diagnostics.
   - There is no change to `main.go`, no global handler, no `signal.Ignore`, and no
     installation at startup.
   - 001–004 are not intentionally changed, including 002's process and agent execution.
   - No other command invokes the mechanism.
   - The effect is process-wide but time-limited to the call.
   - Precondition: one `runWorkflowNext` per process, which `main.go` → `app.Run` guarantees.

## Independent Plan Review remediation (2026-10-05)

| Finding | Resolution | Where |
|---|---|---|
| P-01 broken pipe → 141 | Scoped SIGPIPE → `EPIPE` window in `runWorkflowNext` only, using `signal.Notify`/`signal.Stop` under the Unix build tag and a no-op elsewhere. A deterministic real-binary test uses a reader-closed `os.Pipe` and includes a `workflow state` isolation control. | research §11; contract "Broken pipe"; tests 18b, 21 |
| P-02 table-miss classification | Reclassified as the FR-020 "cannot be rendered" output-failure subcase. Behavior unchanged; the separate category was removed from the contract tables. | research §6, §9; contract outcome and stream tables; data-model |
| P-03 `>&-` | Removed. Replaced by the real broken-pipe mechanism, with no `/dev/full`. | quickstart; test 21 |
| P-04 boundary test | Exhaustive allow-list, alias resolution, dot and blank import rejection, role-selector limits, and a self-check. | research §4; test 11; boundary gate |
| P-05 test-file restriction | Removed. Additive, justified test changes are allowed. The git check is now a review input on the committed branch. | Structure Decision; Quality Gates; test 22 |
| P-06 invalid UTF-8 | Documented as an inherited Go/004 limitation, with a pinning test. | research §8; contract JSON; test 14 |
| P-07 FeatureDir wording | `path.Join("specs", featureID)` used everywhere. | research §2; FeatureDir contract |
| P-08 role import | Human-confirmed permitted dependency (vocabulary only), enforced by the boundary test. | research §4; Dependency boundary; decision 5 |

## Complexity Tracking

No Constitution Check violations.
