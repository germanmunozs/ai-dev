# Quickstart: Agent Role Selection

**Feature**: 003-agent-role-selection
**Revised**: 2026-09-28 (remediation pass — adds the full Quality Gates list, and
corrects references to the removed mutable `policy` map; see `plan.md` "Remediation
Summary" findings 1 and 3)
**Revised again**: 2026-09-29 (second remediation pass, finding I3 — adds explicit
`internal/app`-level `selection_inconsistent` coverage via a direct `RunAgentSelect` call
with an empty `agent.Registry{}`, and clarifies that this is application-level coverage,
not public-CLI reachability; see `plan.md` "Final Remediation Summary (I3 + Corrections)")

This validates the feature end-to-end once implemented. Every deterministic check below
runs without a real Claude Code or Codex CLI install (your actual `PATH` decides
`available`/`unavailable`, exactly like `ai-dev agents` already does); a "real tool" pass
is included as an optional final step. `selection_inconsistent` is **not** reachable
through the *public CLI* under the approved production mapping — see step 6 — but it
does have dedicated, executable `internal/app`-level test coverage (see the `internal/app`
bullet in step 2), which calls `RunAgentSelect` directly rather than going through
argument parsing.

## Prerequisites

- Go toolchain matching `go.mod` (`go 1.27.1`).
- No special environment setup required — this feature performs no process supervision.

## 1. Build

```sh
go build ./...
```

Expected: builds cleanly, no new dependency added to `go.mod` (research.md §9).

## 2. Quality gates

```sh
gofmt -l .
go vet ./...
go test ./...
go test -count=1 ./...
go build ./cmd/ai-dev
git diff --check
```

Expected (see `plan.md` "Quality Gates" for the full pass-condition table): `gofmt -l .`
prints nothing; `go vet ./...` completes with no diagnostics; both `go test ./...` and
`go test -count=1 ./...` pass every test; `go build ./cmd/ai-dev` builds cleanly with no
new `go.mod` dependency; `git diff --check` reports no whitespace errors. None of these
require a live Claude Code or OpenAI/Codex CLI service — every test uses a fake/local
`system.Executor`/`system.Launcher`.

`go test ./...` includes, at minimum:

- `internal/role`: exact-match recognition for all five canonical roles, case-sensitivity
  (`"Implementation"` rejected), and rejection of arbitrary strings
  (`contracts/selection-policy.md`).
- `internal/selection`: production-path coverage of all five `mappedAgent` cases against
  the real `internal/agent` registry (SC-010's "0% under the approved mapping" half); the
  controlled white-box seam for `selection_inconsistent` — `resolveAgainstRegistry`
  called directly with an explicit, deliberately unsupported target agent identity, not
  through any substitutable production policy (`contracts/selection-policy.md`); a
  determinism test (same inputs, repeated calls, identical `Report`).
- `internal/report`: text/JSON rendering for all four outcomes (including
  `selection_inconsistent`, exercised by constructing a `selection.Report` value
  directly, independent of the CLI), asserting exact per-outcome field presence/absence
  (`contracts/agent-select.md`'s table) and canonical outcome values. This layer covers
  rendering and the field contract only — stream placement (stdout/stderr routing) and
  process-completion semantics (the exit code) are properties of `RunAgentSelect`, not of
  the renderer, and are covered separately by the `internal/app` tests below.
- `internal/app`: usage-failure boundary tests (no role, whitespace-only role, extra
  args, bad flags/format) asserting zero registry discovery and zero report emission;
  outcome tests for `role_selected`/`role_unsupported`/`agent_unavailable` dispatched
  through `Run` (the public-CLI path); a **direct `RunAgentSelect` call with an empty
  `agent.Registry{}`** covering `selection_inconsistent` in both text and JSON modes —
  outcome, role/mapped-agent presence, no selected-agent identity, stdout-only stream
  placement, empty stderr, and exit code `1` (see `plan.md` "Application-Level
  `selection_inconsistent` Coverage"); a report-write-failure test (always-erroring
  `io.Writer`) asserting exit `2` and no outcome reclassification; the
  never-launches/never-executes regression tests (covering both `system.Launcher.Launch`
  and `system.Executor.Run`, for the three publicly-dispatched outcomes, plus the
  structural proof that covers `selection_inconsistent` unconditionally) described in
  `contracts/selection-policy.md`; deterministic filesystem non-mutation checks against a
  controlled temporary directory, never a real repository.
- **Regression**: `internal/app/doctor_test.go`, `internal/app/agents_test.go`,
  `internal/app/agentrun_test.go`, and `cmd/ai-dev/main_test.go`'s existing assertions
  are unchanged and still pass (research.md §7; `plan.md` "Regression Protection").

A security/scope audit (manual review against the checklist in `plan.md` "Quality
Gates") confirms no new third-party dependency, no execution capability reachable from
selection, no authentication/network probe, no permission/sandbox/approval change, no
secret/environment-value output, and none of routing/retry/fallback/chaining/background
execution/persistence/dynamic plugin/MCP behavior.

## 3. Manual validation — a role whose mapped agent is available

Ensure at least one of `claude`/`codex` is on your `PATH` (check with `./ai-dev agents`
first). Pick a role mapped to whichever is available (see the table in
`spec.md`'s FR-003), for example `specification` if `claude` is available:

```sh
./ai-dev agent select specification
./ai-dev agent select --format json specification
```

Expected (text): `ai-dev: outcome=role_selected role=specification agent=claude-code`.
Expected (json): `{"supplied_role":"specification","requested_role":"specification","selected_agent_id":"claude-code","outcome":"role_selected"}`.
`echo $?` is `0`. Running either command twice in a row without changing your
environment gives identical output (FR-011/SC-002).

## 4. Manual validation — an unrecognized role

```sh
./ai-dev agent select Specification   # wrong case
./ai-dev agent select nonsense
```

Expected: `ai-dev: outcome=role_unsupported role="Specification"` (and similarly for
`nonsense`); `echo $?` is `1`; no `agent=` field appears in either line or in the JSON
form (no `requested_role`/`selected_agent_id`/`mapped_agent_id` keys).

## 5. Manual validation — a recognized role whose mapped agent is unavailable

Pick a role mapped to whichever of `claude`/`codex` is **not** on your `PATH` (per
`./ai-dev agents`'s `unavailable` row):

```sh
./ai-dev agent select implementation   # if codex is not on PATH
```

Expected: `ai-dev: outcome=agent_unavailable role=implementation agent=codex-cli`;
`echo $?` is `1`.

## 6. `selection_inconsistent` — not naturally reachable through the public CLI

Under the approved production mapping (FR-003) together with the current
`internal/agent` registry, no CLI input reaches `selection_inconsistent` — this is by
design (FR-009, SC-010) and is not a gap in this quickstart. `selection_inconsistent` is
**not made naturally reachable through the public CLI**, and this quickstart introduces
no flag, environment variable, or configuration file that reproduces it from the command
line — none should be added, since a CLI-reachable way to force it would itself be a
"capability to ... configure the mapping," which FR-003 forbids. No production mapping
override or configuration capability is introduced anywhere in this feature to make it
reachable.

Instead, it has the already-approved three-layer test coverage model:

- **`internal/selection`**: the controlled semantic `selection_inconsistent` condition —
  `internal/selection`'s own unit test (`contracts/selection-policy.md`) calls the
  unexported `resolveAgainstRegistry` helper directly with a deliberately unsupported
  target agent identity, without touching the production `mappedAgent` function or any
  mutable policy value (none exists). This layer verifies the semantic outcome only.
- **`internal/report`**: text and machine-readable rendering/field validation —
  constructing a `SelectionInconsistent` `Report` value directly and asserting the
  rendered text line and JSON object match the field-presence contract in
  `contracts/agent-select.md`. This layer verifies rendering and the field contract only.
- **`internal/app`**: direct `RunAgentSelect` invocation with `agent.Registry{}` (the Go
  zero value — no exported test hook, no CLI configuration) for both text and JSON
  modes, verifying stdout/stderr stream placement and unsuccessful process completion
  (exit code `1`) — the properties the two layers above do not cover. See `plan.md`
  "Application-Level `selection_inconsistent` Coverage" for the full test path.

## 7. Manual validation — usage failures

```sh
./ai-dev agent select                       # missing role
./ai-dev agent select "   "                  # whitespace-only role
./ai-dev agent select implementation extra   # too many positionals
./ai-dev agent select --format yaml implementation
```

Expected for each: exit code `2`, usage text on stderr, empty stdout, and no
`outcome=`/`"outcome"` anywhere in the output — these never reach the role-selection
outcome model (FR-015, FR-016).

## 8. Manual validation — never launches an agent

```sh
./ai-dev agent select implementation
ps aux | grep -E 'claude|codex' # (before/after comparison, or use your platform's process list)
```

Expected: no `claude`/`codex` process is ever started by any `agent select` invocation,
regardless of outcome — contrast with `./ai-dev agent run <agent-id> <instruction>`,
which does start one (see `specs/002-coding-agent-execution/quickstart.md` §4).

## 9. Manual validation — filesystem is untouched

```sh
git status --porcelain
./ai-dev agent select review
git status --porcelain
```

Expected: identical output before and after (FR-024, SC-009).

## References

- Command shape, outcome table, exit codes, field-presence table:
  `contracts/agent-select.md`.
- Role/policy/Selector internal contracts and the controlled `selection_inconsistent`
  test seam: `contracts/selection-policy.md`.
- Entity shapes: `data-model.md`.
- Why each implementation choice was made: `research.md`.
