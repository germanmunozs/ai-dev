# Research: SDD Workflow (005)

Phase 0 output for [plan.md](./plan.md). The Technical Context had no `NEEDS CLARIFICATION`
items, because the approved specification resolves every product decision. This document records
the plan-level design decisions, each derived from an approved requirement and the existing
001–004 code.

## §1 Reuse of 004 feature recognition, state resolution, and terminality

**Decision**: Steps 2–3 of the precedence (feature recognition, Workflow State determination) are
performed by calling the existing, unchanged `workflow.EvaluateState(root, featureID)`. The 005
evaluator maps its `StateReport` as follows:

| `StateReport.Outcome` | 005 result |
|---|---|
| `feature_unknown` | `feature_unknown` |
| `state_invalid` | `state_invalid` |
| `state_reported` | continue, using `StateReport.State` and `StateReport.Terminal` |

Terminality (step 5) is read from `StateReport.Terminal`, which 004 computes with
`workflow.Terminal`. 005 never compares a state against `approved` or `abandoned` to decide
terminality.

**Rationale**: FR-005 requires "exactly the observable semantics of 004's approved state-reporting
capability". The function that implements that capability is `EvaluateState`, so calling it is the
only way to have zero risk of divergence. It brings with it, unchanged:
- the raw feature-ID containment rule (`validFeatureID`);
- the no-follow feature-directory `Lstat` (symlink → `feature_unknown`);
- the `fs.ReadLinkFS` requirement;
- the record-reading rules (absent → `draft`; every non-exact case → `state_invalid`).

Using `StateReport.Terminal` satisfies N-01 (004 is the normative authority on terminality).

**Alternatives considered**:
- *Call `FeatureExists` and `ResolveRecord` separately.* This is equivalent, but it re-implements
  `EvaluateState`'s two-step sequence in a second place. Rejected under "no second implementation".
- *Re-implement recognition in 005.* This is a forbidden duplicate (FR-005, FR-026). Rejected.

## §2 Locating the feature directory for artifact inspection: minimal 004 API extension

**Decision**: Add one exported, I/O-free function to `internal/workflow/record.go`:

```go
// FeatureDir returns the slash-separated path of featureID's spec-kit feature directory
// relative to root, and whether featureID is structurally valid. It performs no I/O.
func FeatureDir(featureID string) (string, bool)
```

It returns `path.Join("specs", featureID), true` when `validFeatureID(featureID)` holds, and
`"", false` otherwise. `FeatureExists` and `ResolveRecord` are refactored to obtain their paths
from `FeatureDir`. This is a pure refactor with identical behavior. As a result, the `"specs"`
location and the containment predicate each exist in exactly one place.

**Rationale**:
- 005 needs the directory that 004 recognized (FR-006, FR-010).
- Writing `path.Join("specs", id)` in 005 would create a second copy of 004's feature-location
  rule.
- Exporting the private `validFeatureID` would expose a predicate without its path.
- `FeatureDir` gives both together, with no I/O and no new semantics.
- 005 calls it only after `EvaluateState` returned `state_reported`, so FR-006 holds: no artifact
  path exists before recognition and state resolution succeed.

**Human planning decision (2026-10-05)**: This extension is approved with the following
constraints, each of which is binding on implementation. `FeatureDir`:
- centralizes only the safe derivation of `specs/<feature-id>`;
- reuses exactly 004's approved structural validation (`validFeatureID`), unchanged;
- performs no I/O;
- does not change feature-recognition semantics, state resolution, symlink semantics, outcomes,
  or transition behavior;
- exposes no second source of truth.

The `FeatureExists`/`ResolveRecord` refactor must be semantically neutral and protected by
regression tests. 005 MUST use `FeatureDir` and MUST NOT reconstruct `specs/<feature-id>` itself.
A grep-level audit check confirms that no `"specs"` string literal appears in `internal/sdd`.

**Regression protection**:
- Every existing `internal/workflow`, `internal/report`, `internal/app`, and `cmd/ai-dev` test
  must keep passing. No existing assertion may be removed or weakened. Changes to existing test
  files are allowed only when they are additive and justified (P-05 remediation).
- A new, additive test file covers `FeatureDir`:
  - the C-1 invalid IDs (`..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`, `.`, `""`) → `false`;
  - the valid control (`004-workflow-state`) → `specs/004-workflow-state`, `true`.
- A new test asserts that the paths `FeatureExists` and `ResolveRecord` access are exactly
  `FeatureDir(id)` and `FeatureDir(id) + "/workflow-state.json"`, observed through a recording
  `fs.ReadLinkFS` wrapper.

**Alternatives considered**:
- *Leave 004 untouched and duplicate the `"specs"` literal in 005.* Rejected: it creates a second
  source of truth for feature location.
- *Add a 004 function that also lists artifacts.* Rejected: artifact rules are owned by 005 (spec
  boundary table).

## §3 Exact-case, no-follow artifact presence on any host filesystem

**Decision**: Presence is decided from one directory listing of the feature directory, never by
looking up an artifact name:
1. `entries, err := fs.ReadDir(root, dir)`. If `err != nil`, every required artifact counts as
   missing. That includes partial results that come with an error.
2. An artifact is present if and only if two things hold: some `entry.Name()` is byte-for-byte
   equal to the canonical name (`spec.md`, `plan.md`, `tasks.md`), and that entry's
   `entry.Type().IsRegular()` is true.
3. Every other case counts as missing:
   - a case variant (`Plan.md`);
   - a directory;
   - a symbolic link (`Type()` has `fs.ModeSymlink`);
   - any other non-regular type.

No `Open`, `Stat`, `Lstat`, or `ReadFile` call is ever made on an artifact path. Content is never
read.

**Rationale**:
- A name lookup (`Lstat("specs/x/plan.md")`) is resolved case-insensitively by case-insensitive
  filesystems such as the macOS APFS default, and would wrongly find `Plan.md`.
- A directory listing returns the names as stored on disk on every host. An exact string
  comparison against those stored names is therefore host-independent, which satisfies FR-010,
  FR-022, and SC-012.
- `fs.DirEntry.Type()` reports the entry's own type bits without following links. Two
  implementations are relevant:
  - `os.DirFS`: the type comes from the directory entry; when the kernel reports an unknown type,
    Go falls back to `lstat`, never `stat`.
  - `fstest.MapFS`: the type comes from the entry's `Mode`.
- Symlinks are therefore never followed, with no extra syscall.
- One listing replaces up to three lookups.

**Meaning of "unreadable" (human planning decision, 2026-10-05)**: 005 stays metadata- and
existence-only. It MUST NOT open or read the content of `spec.md`, `plan.md`, or `tasks.md` to
decide presence. Within 005's boundary, "unreadable" means **unreadable or uninspectable through
the permitted metadata-only inspection**. That inspection is the single `fs.ReadDir` of the
feature directory and its `DirEntry` name and type. It does **not** mean inability to read an
artifact's contents. Concretely:
- An entry that the metadata-only inspection shows with the exact canonical name, as a regular
  file, counts as **present**.
- Failure to list or inspect the relevant directory or entry safely counts every affected required
  artifact as **missing**. That covers a `ReadDir` error, including partial results returned with
  an error, and an entry whose type cannot be determined.
- Symlinks and directories count as **missing**.
- Readability or permissions of the file bytes are **not probed**. A regular file visible through
  metadata can therefore count as present even if reading its content would fail (for example,
  mode `000`).
- No content read is introduced merely to check permissions.

*Literal compatibility with the approved spec.*
- FR-010 says that an entry "that cannot be inspected because of a filesystem or I/O error" counts
  as missing, and that "Artifact content MUST NOT be read or validated".
- The spec's edge case says the same of an artifact that "cannot be inspected".
- The spec never defines inspection as reading content. Its only content rule *forbids* reading.
- This decision therefore matches the literal text. The approved inspection is metadata-only; an
  entry it cannot inspect is missing; content is never read. No requirement is contradicted, and
  `spec.md` is unchanged.

**Alternatives considered**:
- *`Lstat` on each canonical name, then compare `info.Name()`.* On macOS `Lstat` returns the
  *requested* name, not the stored one, so it cannot detect `Plan.md`. Rejected.
- *Open the parent directory and use `ReadDir(-1)` manually.* This is equivalent to `fs.ReadDir`
  but more code. Rejected.
- *Try to open each artifact to detect unreadable content.* This violates "content MUST NOT be
  read" and is not required. Rejected.

## §4 Package placement and dependency boundary

**Decision**: Create a new package `internal/sdd`. Its non-test files may import exactly these
packages and nothing else.

| Import path | Why |
|---|---|
| `io/fs` | `fs.FS`, `fs.ReadDir`, `fs.DirEntry` (read-only) |
| `strings` | `strings.TrimSpace` in `Report.Valid()` |
| `ai-dev/internal/workflow` | `EvaluateState`, `StateReport`, `State` constants, outcome tokens, `FeatureDir` |
| `ai-dev/internal/role` | the `Role` type and its constants, as vocabulary only (see below) |

Everything else is rejected. That explicitly includes:
- the stdlib packages `os`, `os/exec`, `os/signal`, `net`, `net/http`, `syscall`, `io/ioutil`,
  `path/filepath`, `path`, `unsafe`, and `reflect`;
- the internal packages `agent`, `execution`, `selection`, `system`, `instruction`, `app`,
  `report`, `doctor`, and `checks`.

The allow-list is exhaustive rather than a deny-list. Any stdlib package not listed, including
any package with write, process, network, or link-following capability, fails the test until a
human-reviewed plan change adds it.

**Boundary test design (`internal/sdd/boundary_test.go`, P-04 remediation)**:
- It uses only `go/parser` and `go/ast`; `go/types` adds nothing needed here.
- It parses every non-`_test.go` file in the package directory once, with `parser.ParseFile`
  (full mode).
- **Import allow-list:** every `ImportSpec.Path` (unquoted) must be in the table above.
- **Alias resolution:** for each file, the test builds a map from the local name to the import
  path:
  - an explicit `ImportSpec.Name` is used as given;
  - otherwise the local name is the last path element, which is `workflow` and `role` for the two
    internal imports.
  - Dot imports (`.`) and blank imports (`_`) of any internal package are rejected outright, so no
    identifier can enter unqualified.
- **Forbidden selectors:** the test walks every `*ast.SelectorExpr` whose `X` is an `*ast.Ident`
  that resolves through that map to `ai-dev/internal/workflow`. It fails if `Sel.Name` is
  `Allowed`, `EvaluateTransition`, `TransitionReport`, `TransitionValid`, `TransitionInvalid`, or
  `TargetUnsupported`. For example, `wf "ai-dev/internal/workflow"` followed by `wf.Allowed` is
  caught, because `wf` resolves to the workflow path.
- **No `"specs"` literal:** no `*ast.BasicLit` string literal may equal `"specs"` or begin with
  `"specs/"`. 005 must use `workflow.FeatureDir`.
- **Self-check:** the test runs its checker against small in-memory known-bad source snippets.
  Each must be rejected, which proves the checker is not vacuous. This is the canonical list;
  `plan.md` test item 11 enumerates the same twelve:
  1. `import "os"`
  2. `import "os/exec"`
  3. `import "os/signal"`
  4. `import "net"`
  5. `import "syscall"`
  6. a forbidden internal import: `import "ai-dev/internal/selection"`
  7. an aliased workflow import using transition knowledge:
     `import wf "ai-dev/internal/workflow"` … `wf.Allowed(…)`
  8. an aliased role import using an unauthorized role: `import r "ai-dev/internal/role"` …
     `r.Remediation`
  9. `role.Remediation` through the default `role` name
  10. a blank internal import: `import _ "ai-dev/internal/selection"`
  11. a dot internal import: `import . "ai-dev/internal/workflow"`
  12. a `"specs"` string literal

  A known-good snippet is also run. It uses only the four allowed imports, `workflow.FeatureDir`,
  `workflow.EvaluateState`, and `role.Planning`, and it must be accepted. This guards against an
  over-strict checker.

**Permitted dependency on `internal/role` (human decision, 2026-10-05, P-08)**:
- `internal/sdd` MAY import `ai-dev/internal/role`. This is a deliberate exception to the
  over-restrictive import list in the Independent Plan Review prompt. The reason is that 005 must
  reuse 003's canonical role vocabulary by reference, not duplicate it.
- `internal/role` is used **exclusively** as pure vocabulary: the `Role` type and the constants
  `Specification`, `Planning`, `Implementation`, and `Review`, plus `role.Recognized` in tests.
  The package itself has no imports.
- This does **not** authorize 005 to select roles, resolve role → agent, import `selection` or
  `agent`, execute agents, or duplicate any 003 mapping.
- The boundary test enforces the agent and selection part through the allow-list. It also rejects
  any selector on the `role` alias other than `Role`, `Specification`, `Planning`,
  `Implementation`, and `Review`, which keeps `Remediation` out of production code as well.

**Rationale**:
- This mirrors 004's own package boundary (`internal/workflow` has no agent or role dependency).
- The dependency direction is `internal/sdd → internal/workflow` and `internal/sdd →
  internal/role`. Neither of those packages imports `sdd`, so there is no cycle, and 004 needs no
  knowledge of 005.
- The parser-based test turns the manual audit of 003/004 into an executable gate for FR-002,
  FR-003, FR-004, and FR-008. An exhaustive allow-list makes "no write, exec, network, or signal
  capability in the domain package" structural rather than review-only.

**Alternatives considered**:
- *Put the evaluator inside `internal/workflow`.* Rejected: it would mix 005-owned artifact and
  activity rules into 004's package and blur the boundary.
- *Redeclare role strings in 005.* Rejected: it duplicates 003's vocabulary.

## §5 Next-Step Table and Required Artifacts as code

**Decision**: Both tables are unexported, exhaustive `switch` functions over `workflow.State`.
This follows the pattern of 003's `mappedAgent` and 004's `Allowed`: no map, slice, or struct
literal that could be mutated, exposed, or configured.

- `nextStep(state) (Activity, role.Role, bool)` has exactly five `case` arms (the non-terminal
  states). Its `default` returns `false`. There is no target-state return value.
- `requiredArtifacts(state) []string` returns a freshly allocated slice in canonical order for the
  approved matrix. `draft` and `abandoned` return `nil`.

**Rationale**: FR-007 forbids any capability to view separately, edit, persist, override, score,
or configure the table. Code is the smallest representation that satisfies this, and tests can
check it exhaustively.

## §6 Defensive handling of an impossible table miss

**Decision (confirmed by human planning decision, 2026-10-05)**: The case is a non-terminal state
that 004 has validly reported, but for which the Next-Step Table has no entry. It is a violated
invariant that makes the required report impossible to render. It is never a domain outcome, and
it is classified as an FR-020 output failure (see "Classification" below). 005 MUST NOT:
- invent an activity or a role;
- add a sixth outcome;
- treat it as `artifacts_inconsistent` or `state_invalid`.

**Classification (P-02 remediation, human decision)**: This is an **FR-020 output failure**,
specifically the "cannot be rendered" subcase. It is not a third exit-2 category alongside usage
failure and output failure. The reasoning:
- A canonical non-terminal state that reaches step 6 has already, conceptually, determined
  `step_recommended`. FR-014 defines step 6 as the step that produces `step_recommended`.
- The report that FR-015 and FR-016 require for that outcome needs an activity and a role. When
  the table has no entry, that report cannot be built or rendered.
- FR-020 covers exactly that: "an outcome has been determined but the final report cannot be
  rendered … MUST treat this as an output failure", which FR-018 maps to exit 2.
- No spec text is contradicted and no spec change is needed.

**Unreachability**: FR-007 requires exactly one entry for each non-terminal Workflow State. While
the approved table is complete, which the exhaustiveness test enforces, this branch cannot be
reached. It exists only as a defensive guard against a future table/vocabulary mismatch.

Mechanism (observable behavior unchanged from the human decision):
1. The evaluator returns a `Report` with an empty `Outcome`, and no activity or role. `Report.Valid()`
   rejects it.
2. Before writing anything, `RunWorkflowNext` checks `r.Valid()`. When the check fails, it writes
   nothing to stdout and writes one diagnostic to stderr:
   `ai-dev: internal error: workflow next report cannot be rendered for feature <%q id>`.
   It then exits 2, through the same output-failure exit path as a writer error.
3. The diagnostic text is specific, so a human can tell this subcase apart. Contractually it is
   still an output failure: same exit code, same stream rules, never one of the five outcomes.
4. The writers' own `Valid()` gate stays as defense in depth.

**Test seams (the case is unreachable with 004's seven states)**:
- `internal/sdd`: `Evaluate(root, id)` is a one-line wrapper over the unexported
  `evaluate(root, id, lookup)`, where `lookup` has the signature of `nextStep`. A test passes a
  lookup with no entry for `planned`, using a valid `planned` fixture with its artifacts present.
  It asserts that the returned report has no outcome, no activity, and no role, and that it fails
  `Valid()`.
- `internal/app`: `RunWorkflowNext(args, root, stdout, stderr)` is a one-line wrapper over the
  unexported `runWorkflowNext(args, root, stdout, stderr, evaluate)`. A test injects an evaluator
  that returns the impossible report. It asserts four things:
  - exit 2, which is the output-failure code;
  - empty stdout;
  - the `ai-dev: internal error: … cannot be rendered` diagnostic on stderr;
  - no outcome token anywhere.
- The production wrappers always pass `nextStep` and `sdd.Evaluate`. The seams are unexported, so
  they add no user-reachable configuration and no capability to edit the table.
- An exhaustiveness test enumerates all seven canonical states. It asserts that every
  non-terminal state (`!workflow.Terminal`) has a `nextStep` entry and every terminal state has
  none.

**Rationale**:
- The spec's edge case on new 004 states says this "must be handled by an amendment to this
  feature" and that "005 never guesses an activity".
- No sixth outcome may be added (FR-013).
- Refusing to render follows the existing 004 pattern for a report the model could not have
  produced.

**Alternatives considered**:
- *`panic`.* Rejected: it would crash instead of emitting a stderr diagnostic.
- *Report `state_invalid`.* Rejected: that redefines a 004 outcome's meaning.

## §7 CLI shape and dispatcher change

**Decision**: The command is `ai-dev workflow next [--format text|json] <feature-id>`. It is
reached through one new `args[1] == "next"` arm in the existing `args[0] == "workflow"` branch of
`app.Run`. `Run`'s signature is unchanged.

The existing workflow-group usage diagnostic stays byte-for-byte
`Usage: ai-dev workflow <state|transition> [options]`, because 004's approved test
`TestRunWorkflowDispatchUsageFailures` asserts that exact string (FR-026, SC-013). `workflow next`
has its own usage line, `Usage: ai-dev workflow next [--format text|json] <feature-id>`.

**Rationale**:
- "next" matches the spec's illustrative syntax (`ai-dev workflow next <feature>`).
- Flag parsing reuses the exact `flag.ContinueOnError` pattern of `RunWorkflowState`, so usage
  failures behave identically to 004.
- Leaving the group usage line unchanged keeps 004's asserted usage behavior intact.

**Human planning decision (2026-10-05)**: 005 does **not** modify the existing `ai-dev workflow`
usage text to list `next`. `ai-dev workflow next …` is supported by dispatch only. The existing
004 behavior and usage text, both the group usage line and the `ai-dev workflow` and
`ai-dev workflow bogus` diagnostics, are preserved byte for byte. The only change is the minimal
dispatch arm that recognizes `next`. Discoverability is not a reason to change them, and no
Feature 007 behavior is anticipated. This is not a spec conflict: the spec does not require
discoverability through that diagnostic.

**Alternatives considered**:
- *Update the group usage string and its 004 test.* Rejected without human approval (FR-026).
- *A top-level `ai-dev next` command.* Rejected: it is not under the workflow namespace and would
  change the top-level usage string.

## §8 Report rendering

**Decision**: The writers follow `internal/report/workflowstate.go` exactly: a `Valid()` gate,
one `fmt.Fprintf` line for text, and one `json.Encoder.Encode` for JSON. Normal reports are
written to stdout for every outcome, including failures. That matches 003/004.

The text form has the prefix `ai-dev: outcome=<token> feature=<%q id>`. Only canonical tokens
follow it, plus the fixed recommendation disclaimer for `step_recommended`. JSON keys are
`feature_id`, `outcome`, `current_state`, `activity`, `role`, and `missing_artifacts`. Fields
prohibited for an outcome are absent, never `null` or `""`. Full formats are in
[contracts/workflow-next.md](./contracts/workflow-next.md).

**Rationale**:
- `current_state` matches 004's transition-report key for the same concept and conveys "observed
  current state".
- `%q` quoting is required by 004 FR-028, which FR-017 references.
- Every other text value is drawn from a closed set of canonical tokens, so no supplied byte other
  than the quoted ID ever reaches the line. That makes the line injection-safe.

**JSON `feature_id` and invalid UTF-8 (P-06 remediation; inherited Go/004 behavior)**:
- `encoding/json` coerces string values to valid UTF-8, replacing each invalid byte with U+FFFD.
  It also escapes `<`, `>`, and `&` as `<`, `>`, and `&`, which decode back to the
  same characters.
- For a feature ID that is valid UTF-8, the decoded `feature_id` equals the supplied ID exactly.
- For a feature ID containing invalid UTF-8, the decoded `feature_id` is the U+FFFD-normalized
  form, not a byte-for-byte copy.
- 004's `workflow state` and `workflow transition` JSON have the same behavior. 005 adds no custom
  encoding and does not change 004.
- Such an ID can never name a recognized feature in practice. The outcome is still computed from
  the supplied bytes, typically `feature_unknown`.
- The text form is unaffected: `%q` escapes invalid bytes as `\x..`, losslessly.

## §9 Output failure

**Decision**: FR-020 output failure has exactly two subcases. Both exit 2, and neither is a
domain outcome.

| Subcase | Trigger | stdout | stderr |
|---|---|---|---|
| Cannot be written or delivered | The stdout `Write` returns an error. This includes `EPIPE` from a real broken pipe, which §11 turns into an error. | not guaranteed | `write workflow next report: <err>`, when possible |
| Cannot be rendered | `Report.Valid()` fails before writing, for example the impossible table miss in §6 | empty | `ai-dev: internal error: workflow next report cannot be rendered for feature <%q id>` |

The command makes exactly one write attempt. It does not retry, reclassify the outcome, or fall
back to another output. This matches `RunWorkflowState`'s existing write-error handling.

**Rationale**: This implements FR-020 and the exit-code table in FR-018. It also matches the
existing 004 convention for report-write failures.

## §11 Broken pipe (SIGPIPE) handling, scoped to `workflow next` (P-01 remediation)

**Problem (measured)**: Go's runtime handles a write to a broken pipe on file descriptor 1 or 2
by killing the process with SIGPIPE, unless the program has registered for SIGPIPE with
`os/signal`. The shell reports exit 141 (128+13). This was reproduced on this host for a probe
binary and for the existing `ai-dev workflow state`. FR-018 and FR-020 require exit 2 when the
report cannot be delivered.

**Decision**: The fix lives only in `internal/app`, in two small build-tagged files.

```go
// internal/app/sigpipe_unix.go   (//go:build unix)
// brokenPipeAsError makes writes to a broken pipe on stdout/stderr return EPIPE
// instead of killing the process, until the returned release func is called.
func brokenPipeAsError() (release func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGPIPE)
	return func() { signal.Stop(ch) }
}

// internal/app/sigpipe_other.go  (//go:build !unix)
func brokenPipeAsError() (release func()) { return func() {} }
```

`runWorkflowNext` calls `release := brokenPipeAsError()` and then `defer release()` as its
**first statement**, and releases on return. The window is the lifetime of one `workflow next`
invocation inside `runWorkflowNext`. Within it, the only operations that can write to a pipe are
`workflow next`'s own writes:
- the usage diagnostic;
- the report;
- the "cannot be rendered" diagnostic;
- the write-error diagnostic.

Argument parsing and evaluation perform only reads, through `fs.FS`, and write nothing.

**Human decision (2026-10-05, P-01 window)**: The temporary SIGPIPE handling is installed at the
start of `runWorkflowNext` and released, through `defer`, when `runWorkflowNext` returns. It covers
every write the command itself performs:
- the normal report on stdout;
- usage diagnostics on stderr;
- output-failure diagnostics on stderr.

It is not limited to the report write. These constraints remain binding:
- no change to `cmd/ai-dev/main.go` and no global handler;
- no `signal.Ignore(SIGPIPE)`;
- no installation at startup;
- no intentional change to 001–004, including 002's process and agent execution;
- installed only on entry to `runWorkflowNext` and released only by its `defer`;
- no other command invokes this mechanism.

*Why the window opens at entry and not only around the report write*: FR-019 also requires a
usage failure to exit 2 after writing its diagnostic to stderr. If stderr is a broken pipe, a
window that opened after usage validation would leave that write unprotected, and the process
would die with 141. Opening at entry covers every write `workflow next` performs. Because
evaluation writes nothing, this is effectively the same scope as "the output period of
`workflow next`", and it is still confined to this single command.

**How the write becomes exit 2**:
1. While the channel is registered, the runtime's SIGPIPE path finds that the signal is wanted.
   It delivers the signal to `ch`, which is non-blocking: the buffer holds 1 and extra signals are
   dropped. It does not kill the process.
2. The `write(2)` syscall returns `EPIPE`, so `os.File.Write` returns a `*fs.PathError` wrapping
   `syscall.EPIPE`.
3. `fmt.Fprintf` or `json.Encoder.Encode` returns that error to the report writer, and the writer
   returns it to `runWorkflowNext`.
4. The existing output-failure path (§9) writes `write workflow next report: …` to stderr and
   returns 2.
5. On return, `release` calls `signal.Stop(ch)`. With no channel registered, the runtime restores
   its default SIGPIPE behavior.

**Measured on this host (macOS, Go 1.27.1)**: a child whose stdout was a pipe with the read end
already closed behaved as follows:
- with no handler: killed by SIGPIPE;
- with `Notify`: the write failed with `write /dev/stdout: broken pipe`, and the child exited 2;
- after `Stop`: the next broken-pipe write killed it again, confirming that default behavior is
  restored.

**Scope and isolation**:
- `brokenPipeAsError` is called only from `runWorkflowNext`.
- `cmd/ai-dev/main.go` is unchanged.
- SIGPIPE is never ignored (`signal.Ignore` is not used), and there is no global or `init`
  registration.
- `doctor`, `agents`, `agent run`, `agent select`, `workflow state`, and `workflow transition`
  never reach it, so their behavior is unchanged. That includes 004's current exit-by-SIGPIPE on a
  broken pipe, which 005 does not alter (FR-026).
- `workflow next` starts no child process, so no signal disposition can leak into a child. `Notify`
  also does not change the disposition a child would inherit: it installs a Go handler, not
  `SIG_IGN`. 002's agent execution path is never involved.

**Exact semantics (process-wide, not thread-local)**:
- For file descriptors 1 and 2, Go decides synchronously, at the moment a `write` returns `EPIPE`,
  whether to die. The `os` package calls the runtime's `sigpipe` routine, which returns without
  killing the process if SIGPIPE is currently ignored or currently wanted by some `Notify`
  channel. Otherwise it terminates the process with SIGPIPE.
- An asynchronously delivered kernel SIGPIPE is never fatal to a Go program on its own. The
  runtime only relays it to `Notify` channels, if there are any.
- The guarantee therefore depends only on whether a `Notify` registration exists at the instant
  each write returns. Because `defer release()` runs after every write in `runWorkflowNext`,
  every such write sees the registration.
- `signal.Notify` and `signal.Stop` change **process-wide** state. "Scoped" in this plan means
  *temporally* scoped to one `runWorkflowNext` call. It does not mean thread- or
  goroutine-local.
- After `signal.Stop(ch)` returns, `ch` receives no further signals. If no other channel is
  registered for SIGPIPE, the process returns to Go's default behavior for SIGPIPE: a later
  broken-pipe write on fd 1/2 is fatal (measured above).
- A signal value already buffered in `ch`, or one that arrives during `Stop`, is harmless. Nobody
  reads `ch`, `Notify` never blocks on a full channel, and the channel is garbage-collected.
- No draining of `ch` is needed. Nor is `signal.Reset(SIGPIPE)`, which would be worse: it would
  also remove any other registration for SIGPIPE in the process.

**Precondition: one `runWorkflowNext` per process (documented, not engineered around)**:
- In production, `cmd/ai-dev/main.go` calls `app.Run` exactly once, and `app.Run` dispatches to
  exactly one command. `runWorkflowNext` therefore runs at most once per process, and nothing
  else in that process writes to fd 1/2 concurrently with it.
- No additional locking or reference counting is designed. If several `runWorkflowNext` calls did
  overlap in one process, for example in a future in-process test using `t.Parallel`, each would
  hold its own channel. `os/signal` keeps SIGPIPE wanted while any channel is registered, so each
  call's writes would still get `EPIPE`. One call's `Stop` cannot disable another call's
  registration.
- The only process-wide effect of overlap is that a non-`workflow next` broken-pipe write in that
  same process, during the overlap, would get `EPIPE` instead of being fatal. No existing test
  runs in parallel (there is no `t.Parallel` in the repository) or depends on SIGPIPE death.

**Concurrency**:
- `signal.Notify` and `signal.Stop` are safe for concurrent use and process-wide.
- In production, `workflow next` is the only work in the process.
- In `go test`, tests may run concurrently in the same binary. While a `runWorkflowNext` test holds
  the registration, SIGPIPE on stdout/stderr anywhere in that test process becomes `EPIPE`
  instead of death. No existing test depends on dying from SIGPIPE, so this is harmless.
- `NotifyContext(os.Interrupt)` in `main.go` uses a different signal and is unaffected.
- No goroutine reads `ch`. That is allowed for a buffered channel and introduces no background
  work.

**Platforms without SIGPIPE**: the `!unix` file is a no-op. On those platforms (Windows), writing
to a broken pipe already returns an ordinary error (`ERROR_BROKEN_PIPE` / `ERROR_NO_DATA`), which
takes the same exit-2 path. `syscall.SIGPIPE` is referenced only under the `unix` build tag.

**Test (real binary, Unix, deterministic, no `>&-`, no `/dev/full`)**: the test is in
`cmd/ai-dev/workflownext_brokenpipe_test.go`, with `//go:build unix`.
1. Build `ai-dev` into `t.TempDir()`, as `main_test.go` already does.
2. Create a fixture repository with `specs/f/` (draft).
3. Create `r, w, _ := os.Pipe()` and call `r.Close()` **before** starting the process. The read end
   is gone, so the first write deterministically gets `EPIPE`/SIGPIPE. There is no timing race,
   and it works the same way on macOS and Linux.
4. Run `ai-dev workflow next f`, and then `--format json f`, with `cmd.Stdout = w` and stderr
   captured.
5. Assert that the process was **not** signaled (`WaitStatus.Signaled() == false`), the exit code
   is `2`, and stderr contains `write workflow next report:`.
6. **Broken stderr on usage failure:** run `ai-dev workflow next` with no ID, with
   `cmd.Stderr = w2`, a second pipe whose read end is already closed, and stdout captured. Assert
   that the process was not signaled, exited 2, and wrote nothing to stdout.
7. **Isolation control:** run `ai-dev workflow state f` under the same broken pipe and assert that
   its existing behavior is unchanged: it is terminated by SIGPIPE. This proves the handler did not
   leak to 004. The assertion documents current 004 behavior, so a future, separately approved fix
   to 004 would update this control deliberately.

**Alternatives considered**:
- *`signal.Ignore(syscall.SIGPIPE)` in `main.go`.* This is global, changes 001–004, and an ignored
  disposition is inherited by child processes such as 002's agents. Rejected.
- *Registering in `main.go` for every command.* This changes 004 and others. Rejected.
- *Testing with `>&-`.* The Go runtime reopens a closed fd 1 as `/dev/null`, so the write
  succeeds (measured). Rejected.
- *Testing with `/dev/full`.* It does not exist on macOS. Rejected.

## §10 Case-insensitive host testing

**Decision**: Two complementary tests.

1. **Simulated, always runs.** A test wrapper `caseInsensitiveFS` around `fstest.MapFS` behaves
   like a case-insensitive filesystem:
   - `Open`, `Lstat`, and `ReadLink` resolve names case-insensitively against the stored keys;
   - `ReadDir` returns the stored (case-preserved) names.

   Fixtures with `Plan.md`, `SPEC.md`, and `Tasks.md` must report `artifacts_inconsistent`. A
   companion test confirms that the wrapper really resolves `Lstat("…/plan.md")` to `Plan.md`. That
   proves the fixture would catch a lookup-based implementation.
2. **Real host, always runs.** The same fixtures are built in `t.TempDir()` and evaluated through
   `os.DirFS`.
   - The test probes whether the host is case-insensitive (create `probe`, `Lstat` `PROBE`) and
     records the result with `t.Log`.
   - The expected outcome is identical on both kinds of host, so no skip is needed.
   - On the default macOS developer setup it exercises a real case-insensitive filesystem. On
     Linux CI it exercises a case-sensitive one.

**Rationale**: SC-012 requires identical results on both kinds of host. The simulated test
guarantees case-insensitive coverage on every CI platform. The real-host test guards against
differences between the simulation and `os.DirFS`.
