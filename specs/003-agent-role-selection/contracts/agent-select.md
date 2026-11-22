# Contract: `ai-dev agent select`

**Revised**: 2026-09-28 (remediation pass — updates the "Guarantees" and "Test seams"
sections for `RunAgentSelect`'s corrected `agent.Registry`-based signature, adds explicit
report-write-failure handling, and covers both execution mechanisms in the never-launches
regression proof; see `plan.md` "Remediation Summary" findings 2, 4, and 7. The CLI
surface itself — command shape, outcome table, human/machine report contracts, and exit
codes — is unchanged.)
**Revised again**: 2026-09-29 (second remediation pass, finding I3 — adds a concrete
application-level test path for `selection_inconsistent`, and corrects two overclaims
about outcome/no-execution coverage reaching "all four outcomes" through the public CLI
or its `Run` dispatch boundary; see `plan.md` "Final Remediation Summary (I3 +
Corrections)". The CLI surface itself is unchanged by this pass either.)

**Satisfies**: FR-001 through FR-017, FR-020 through FR-027, SC-001 through SC-010

## Command

```text
ai-dev agent select [--format text|json] <role>
```

- `<role>` is required and positional — exactly one token. `--format` defaults to
  `text` and, like every other `ai-dev` subcommand, must precede the positional (stdlib
  `flag` parsing stops at the first non-flag token).
- There is no other flag, option, or positional this command recognizes.

## Recognizing a request (usage failure vs. role-selection outcome)

A **usage failure** (exit `2`, usage text on stderr, stdout empty, no role selection
attempted, no availability check performed, no `Report` emitted — FR-015) occurs when:

- No `<role>` token is supplied at all.
- The supplied `<role>` token is empty or consists only of whitespace (FR-016 — this is
  a missing role, not `role_unsupported`).
- More than one positional token is supplied after flags (FR-014).
- Unknown flags, or `--format` given a value other than `text`/`json`.

Once a single, non-empty, non-whitespace `<role>` token is present, the request is a
**recognized Role Selection Request**, and every further determination below is a
role-selection **outcome** (reported through the normal Report path — see
`contracts/selection-policy.md`), never a usage failure:

| Condition                                                                    | Outcome |
|--------------------------------------------------------------------------------|---------|
| `<role>` is not one of the five canonical roles (exact, case-sensitive match)   | `role_unsupported` (FR-007) |
| `<role>` recognized; its mapped agent is absent from the supported-agent registry (test-only condition — see below) | `selection_inconsistent` (FR-009) |
| `<role>` recognized; its mapped agent is a supported agent that is not currently locally available | `agent_unavailable` (FR-008) |
| `<role>` recognized; its mapped agent is a supported agent that is currently locally available | `role_selected` (FR-006) |

`selection_inconsistent` does not occur under the approved production policy (FR-003)
together with the current `internal/agent` registry — it is a reserved integrity-failure
outcome. It is never reachable through this CLI command's own argument-parsing/dispatch
path (`Run` → a real, executor-backed `agent.Registry`) in a normal build, and this
command provides no flag or environment variable that could make it reachable. It is,
however, exercised deterministically at two levels below the public CLI surface: (1)
`internal/selection`'s own controlled unit test (`resolveAgainstRegistry`, called
directly with an explicit unsupported target — see `contracts/selection-policy.md`), and
(2) a concrete, executable `internal/app`-level test that calls `RunAgentSelect` directly
with a controlled empty `agent.Registry{}` (the Go zero value), covering the outcome's
stream placement and process-completion semantics at the actual application boundary the
CLI dispatches into — see "Application-Level Coverage for `selection_inconsistent`"
below and `plan.md` "Application-Level `selection_inconsistent` Coverage" for the full
test path.

## Human-readable output (`--format text`, default)

Written to **stdout**; nothing else is written to stdout. Exactly one line, always
displaying the exact canonical outcome value (FR-027):

```text
ai-dev: outcome=role_selected role=implementation agent=codex-cli
ai-dev: outcome=role_unsupported role="Implementation"
ai-dev: outcome=agent_unavailable role=implementation agent=codex-cli
ai-dev: outcome=selection_inconsistent role=implementation agent=codex-cli
```

- `role_selected`: `role=` is the canonical requested role; `agent=` is the selected
  agent's canonical identity.
- `role_unsupported`: `role=` is the originally supplied value exactly as given
  (quoted, since it is arbitrary caller input) — no `agent=` field is present, because
  no canonical requested role or agent identity exists for this outcome (FR-007).
- `agent_unavailable` / `selection_inconsistent`: `role=` is the canonical requested
  role; `agent=` is the mapped/target agent identity — never presented as a selected
  agent (FR-008, FR-009).
- No failure outcome's line resembles or could be mistaken for the `role_selected` line
  (FR-010).

## Machine-readable output (`--format json`)

Written to **stdout**; nothing else is written to stdout. One JSON object, encoded the
same way every other `ai-dev` report is encoded (`json.NewEncoder`, one object):

```json
{"supplied_role": "implementation", "requested_role": "implementation", "selected_agent_id": "codex-cli", "outcome": "role_selected"}
{"supplied_role": "Implementation", "outcome": "role_unsupported"}
{"supplied_role": "implementation", "requested_role": "implementation", "mapped_agent_id": "codex-cli", "outcome": "agent_unavailable"}
{"supplied_role": "implementation", "requested_role": "implementation", "mapped_agent_id": "codex-cli", "outcome": "selection_inconsistent"}
```

- `supplied_role`: always present — the role value exactly as originally supplied
  (FR-012).
- `requested_role`: present only when the supplied value is one of the five canonical
  roles; the JSON key is entirely absent (not `null`, not `""`) for `role_unsupported`
  (FR-007, FR-012).
- `selected_agent_id`: present only when `outcome` is `role_selected`; absent otherwise
  (FR-006, FR-012).
- `mapped_agent_id`: present only when `outcome` is `agent_unavailable` or
  `selection_inconsistent`; absent otherwise (FR-008, FR-009, FR-012).
- `outcome`: exactly one of `role_selected`, `role_unsupported`, `agent_unavailable`,
  `selection_inconsistent` (FR-010).
- An automated caller determines every fact SC-006 requires from key presence and value
  alone — no free-form text parsing.

## Exit codes

| Condition                                        | Exit code |
|-----------------------------------------------------|-----------|
| Usage failure                                        | `2` |
| Recognized request, outcome `role_selected`           | `0` |
| Recognized request, outcome `role_unsupported`         | `1` |
| Recognized request, outcome `agent_unavailable`         | `1` |
| Recognized request, outcome `selection_inconsistent`     | `1` |

This is FR-017: successful completion if and only if `role_selected`; every other
recognized outcome and every usage failure is unsuccessful completion. No new
implementation-specific exit code is introduced beyond the existing 0/1/2 convention
already used by `doctor`, `agents`, and `agent run` (research.md §6).

## Application-Level Coverage for `selection_inconsistent`

*Addresses remediation finding I3: this outcome previously had no concrete, executable
test proving the contracts above (stream placement, exit code) actually hold at the
application boundary `RunAgentSelect` itself — as opposed to the outcome-determination
logic (`internal/selection`) or the report-rendering logic (`internal/report`) in
isolation.*

Because the public CLI cannot construct `selection_inconsistent` under the production
mapping (see "Recognizing a request" above), this contract is instead verified by an
`internal/app` test that calls `RunAgentSelect` directly with a controlled, empty
`agent.Registry{}` (the Go zero value of the already-exported `agent.Registry` type — no
change to `internal/agent`, no new exported constructor, no CLI flag; see `plan.md`
"Application-Level `selection_inconsistent` Coverage" for why this value deterministically
yields `selection_inconsistent` for any of the five canonical roles). Both output modes
are verified against this contract's own tables above:

- **Text mode** (`RunAgentSelect([]string{"implementation"}, agent.Registry{}, &stdout,
  &stderr)`): the rendered line's outcome is exactly `selection_inconsistent`; `role=` is
  the canonical requested role; `agent=` is the mapped target identity (`codex-cli`), per
  the "agent_unavailable / selection_inconsistent" row above — never presented as
  selected; `stdout` contains exactly that one line and nothing else; `stderr` is empty
  (the report was written successfully); the returned exit code is `1`, from the Exit
  codes table above — no new code is introduced.
- **JSON mode** (`RunAgentSelect([]string{"--format", "json", "implementation"},
  agent.Registry{}, &stdout, &stderr)`): `stdout` contains exactly one valid JSON object
  with `"outcome":"selection_inconsistent"`, `"requested_role":"implementation"`,
  `"mapped_agent_id":"codex-cli"`, and no `"selected_agent_id"` key, per the
  Machine-readable output table above; `stderr` is empty; the returned exit code is `1`.

This is application-level coverage of a production code path (`RunAgentSelect`'s own
logic and stream handling), not a claim that the public CLI can reach this state — see
the corrected description in "Recognizing a request" above and in "Test seams" below.

## Guarantees that hold regardless of outcome

- **No agent invocation** (FR-004, FR-020, SC-005): `RunAgentSelect` never constructs or
  receives a `system.Launcher`, a `system.Executor`, or an `execution.Runner` — its
  signature is `RunAgentSelect(args []string, registry agent.Registry, stdout, stderr
  io.Writer) int`. It receives only a preconstructed, read-only `agent.Registry` (built
  by `app.Run` from the executor it already holds — see `plan.md` "Dependency
  Boundary"). There is no code path from this command to launching, authenticating, or
  reconfiguring any coding agent.
- **No filesystem mutation** (FR-024, SC-009): `RunAgentSelect` only reads local
  executable discoverability through the supplied `agent.Registry` (itself backed by
  `system.Executor.LookPath`, a read-only lookup, at construction time in `app.Run`) and
  writes to the caller-supplied `stdout`/`stderr` streams. It performs no file creation,
  modification, or deletion.
- **Report-write failure is not a fifth outcome**: if producing or writing the final
  report for an already-determined outcome fails, `ai-dev` completes unsuccessfully
  (exit `2`, following the same convention `RunAgentsList`/`RunDoctor` already use for
  their own report-write failures), a diagnostic is written to stderr when possible, and
  the failure is never reclassified as `selection_inconsistent` or any other of the four
  outcomes — see `plan.md` "Report Validation / Write Failure".
- **No further automatic action** (FR-023, SC-008): for any outcome other than
  `role_selected`, the command ends at reporting. It never attempts another agent,
  another mapping, or a retry within the same invocation.
- **No network/auth probes** (FR-005, FR-021): availability comes only from
  `agent.Registry.List()`, which performs local executable lookup only — identical to
  the mechanism 002 already established and left unchanged.
- **No secrets in output** (FR-022): the report contains only role and agent
  identifiers and the fixed outcome vocabulary.

## Test seams this contract implies

- Usage-failure boundary: no role token, whitespace-only role token, extra positional
  tokens, unknown flag, invalid `--format` value — each asserted to perform zero
  `agent.Registry` discovery and emit no `Report` (mirrors
  `TestRunAgentsListUsageFailuresDoNotDiscover` /
  `TestRunAgentRunSeparatesUsageFailuresFromRecognizedExecutionFailures`).
- Three outcomes reachable through the public CLI's own dispatch path: a fake-backed
  `agent.Registry` (constructed the same way `internal/agent/registry_test.go` already
  constructs one, via a `recordingExecutor` or equivalent) supplied via `Run` for
  `role_selected`/`role_unsupported`/`agent_unavailable`. `selection_inconsistent` is not
  reachable through that same dispatch path under the production mapping (FR-009,
  SC-010; `quickstart.md` §6), so it is covered by three separate, accurately-scoped
  seams instead: `internal/selection`'s own controlled seam (`resolveAgainstRegistry`
  called directly with an explicit unsupported target — outcome-determination logic in
  isolation), `internal/report`'s direct `Report{Outcome: SelectionInconsistent}`
  construction (rendering logic in isolation), and a direct `RunAgentSelect` call with an
  empty `agent.Registry{}` (the actual application boundary — see "Application-Level
  Coverage for `selection_inconsistent`" above). The third of these is `internal/app`-
  level coverage, calling the same production function the CLI calls, even though the
  public CLI's own argument-parsing path cannot construct this state.
- Determinism: identical `suppliedRole` + fixed fake-registry state queried twice yields
  byte-identical text/JSON output (SC-002).
- Machine-readable isolation and field-presence assertions: stdout contains only the one
  JSON object; per-outcome key presence/absence matches the table above (SC-006).
- Report-write failure: an always-erroring `io.Writer` passed as `stdout` to
  `RunAgentSelect`, asserting exit code `2`, a stderr diagnostic, and that no outcome
  (in particular not `selection_inconsistent`) is reported — see "Guarantees that hold
  regardless of outcome" above and `plan.md` "Report Validation / Write Failure".
- Never-launches / never-executes regression, covering both execution mechanisms: a
  fake `system.Launcher` and a `recordingExecutor` wired at the `internal/app` `Run`
  dispatch boundary (the same instances `agent run`'s own tests already use), asserting
  zero `Launch` calls and zero `Run` calls across every `agent select` invocation
  dispatched through `Run` (`role_selected`, `role_unsupported`, `agent_unavailable`, and
  every usage failure), alongside the existing `agent run` launch/run-count assertions
  for comparison. `selection_inconsistent`'s zero-execution property is covered instead
  by the unconditional structural proof (`RunAgentSelect`'s signature has no
  `system.Launcher`/`system.Executor` parameter, regardless of which `agent.Registry` it
  is given, including the empty one used by its own application-level test) — see
  `contracts/selection-policy.md` "Never-launches / never-executes regression proof" for
  the full detail.
- Deterministic filesystem non-mutation: a controlled temporary directory/repository
  snapshot compared before and after representative `role_selected`, `role_unsupported`,
  `agent_unavailable`, and usage-failure invocations — never against a real repository.
- Regression: `internal/app/doctor_test.go`, `internal/app/agents_test.go`,
  `internal/app/agentrun_test.go`, and `cmd/ai-dev/main_test.go`'s existing assertions
  are unchanged and continue to pass — see `plan.md` "Regression Protection".
