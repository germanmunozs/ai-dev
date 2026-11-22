# Contract: `ai-dev workflow next`

This contract implements the spec's next-step evaluation capability: FR-001, FR-013 to FR-021,
and SC-010/SC-011.

## Usage

```text
ai-dev workflow next [--format text|json] <feature-id>
```

- `--format` takes `text` (the default) or `json`. Any other value is a usage failure.
- Exactly one positional argument, `<feature-id>`, is required after flag parsing. Flags must come
  before the positional argument (Go `flag` semantics, identical to `workflow state`).
- The command never reads stdin, prompts, or waits for input (FR-021).

## Usage failures (FR-019): precedence step 1

| Condition | Behavior |
|---|---|
| No `<feature-id>` | `Usage: ai-dev workflow next [--format text|json] <feature-id>` to stderr, empty stdout, exit `2` |
| `<feature-id>` empty or whitespace-only | same |
| Extra positional args | same |
| Unknown flag | same |
| `--format` not `text` or `json` | same |

A usage failure makes zero `fs.FS` calls (no `Open`, `Lstat`, `ReadLink`, or `ReadDir`) and
mutates nothing.

A non-empty, non-whitespace but structurally invalid ID is **not** a usage failure. Examples:
`..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`. Such an ID is a recognized request whose outcome
is `feature_unknown`, determined by 004 with zero filesystem access.

The existing group diagnostic `Usage: ai-dev workflow <state|transition> [options]` (for
`ai-dev workflow` or `ai-dev workflow bogus`) is unchanged; see research §7.

## Outcomes and exit codes (FR-013, FR-014, FR-018)

| Step | Condition | Outcome | Exit |
|---|---|---|---|
| 2 | 004 does not recognize the feature (absent, structurally invalid, symlinked dir, or `fs.ReadLinkFS` unavailable) | `feature_unknown` | 1 |
| 3 | 004 classifies the record as invalid | `state_invalid` | 1 |
| 4 | One or more required artifacts missing | `artifacts_inconsistent` | 1 |
| 5 | The state is one that 004 defines as terminal (`StateReport.Terminal`) | `workflow_terminal` | 0 |
| 6 | Non-terminal state, artifacts present | `step_recommended` | 0 |
| — | Output failure (FR-020): the report cannot be rendered, written, or delivered (subcases below) | *(no outcome)* | 2 |

There are exactly two non-outcome results: a usage failure (exit 2, step 1) and an output failure
(exit 2, FR-020). An output failure has two subcases. Neither is a domain outcome, and neither is a
separate exit-code category:
- **Cannot be written or delivered:** the stdout write returns an error, including `EPIPE` from a
  real broken pipe (research §11).
- **Cannot be rendered:** `Report.Valid()` fails before writing. The only known cause is the
  impossible Next-Step Table miss: a non-terminal state that reached step 6, so `step_recommended`
  is determined, but has no activity/role (research §6). FR-007 makes this unreachable while the
  approved table is complete.

Exit `0` means only that the evaluation completed successfully. Callers MUST read `outcome`
(`step_recommended` versus `workflow_terminal`) to learn whether an activity exists. Exit `0` is
never approval, authorization, or a transition, and it never distinguishes `approved` from
`abandoned`.

## Streams (FR-017)

| Situation | stdout | stderr |
|---|---|---|
| Any of the five outcomes, text or JSON | exactly one report | empty |
| Usage failure | empty | one usage line |
| Output failure: cannot be written or delivered | **not guaranteed**; callers rely on the exit code (FR-020) | `write workflow next report: <error>` (when possible) |
| Output failure: cannot be rendered | empty (nothing is written) | `ai-dev: internal error: workflow next report cannot be rendered for feature <%q id>` |

The "cannot be rendered" subcase has its own diagnostic text, but it is contractually an FR-020
output failure. It never invents an activity or role, and it is never reported as
`artifacts_inconsistent`, `state_invalid`, or any other outcome.

**Broken pipe:** within `workflow next`, a write to a broken stdout or stderr pipe returns an
error instead of terminating the process. Without that handling, the Go runtime would kill the
process with SIGPIPE (shell status 141). As a result, a broken stdout pipe yields exit 2, and a
usage failure with a broken stderr pipe also yields exit 2. This handling is scoped to
`workflow next` only (research §11). Every other command, including `workflow state` and
`workflow transition`, keeps its current behavior.

## Text format (`--format text`, the default)

There is exactly one newline-terminated line per report. `<id>` is rendered with Go `%q`, so the
supplied ID is escaped and quoted (004 FR-028). Every other value is a canonical token from a
closed set.

```text
ai-dev: outcome=step_recommended feature=<id> state=<state> activity=<activity> role=<role> note="recommendation only; does not authorize this activity or any transition"
ai-dev: outcome=workflow_terminal feature=<id> state=<state>
ai-dev: outcome=artifacts_inconsistent feature=<id> state=<state> missing=<name>[,<name>...]
ai-dev: outcome=feature_unknown feature=<id>
ai-dev: outcome=state_invalid feature=<id>
```

The following hold for every line:
- It contains no target or next state, transition result, agent identity, availability, selection
  or execution outcome, or text asserting approval or authorization (FR-016).
- `state=approved` on a `workflow_terminal` line is the observed current state only.
- The disclaimer appears only on `step_recommended`. Its wording is fixed and contains no
  Workflow State token.
- The outcome is always the first field, `ai-dev: outcome=<token> `. Outside the `%q`-quoted
  feature value, no line other than `step_recommended` contains `outcome=step_recommended`.

## JSON format (`--format json`)

There is exactly one JSON object on stdout, followed by `\n`, with no other content. The
`feature_id` and `outcome` keys are always present. Other keys follow FR-016. A prohibited key is
**absent**: it is never `null`, `""`, or `[]`.

| Outcome | `current_state` (string) | `activity` (string) | `role` (string) | `missing_artifacts` (array of string) |
|---|---|---|---|---|
| `step_recommended` | required | required | required | absent |
| `workflow_terminal` | required | absent | absent | absent |
| `artifacts_inconsistent` | required | absent | absent | required, non-empty, canonical order |
| `feature_unknown` | absent | absent | absent | absent |
| `state_invalid` | absent | absent | absent | absent |

Examples:

```json
{"feature_id":"005-sdd-workflow","outcome":"step_recommended","current_state":"specified","activity":"plan","role":"planning"}
{"feature_id":"005-sdd-workflow","outcome":"workflow_terminal","current_state":"approved"}
{"feature_id":"005-sdd-workflow","outcome":"artifacts_inconsistent","current_state":"planned","missing_artifacts":["plan.md"]}
{"feature_id":"../x","outcome":"feature_unknown"}
{"feature_id":"005-sdd-workflow","outcome":"state_invalid"}
```

No other key is ever emitted. That rules out any agent, approval, authorization,
`authorized`, `approved_by`, `gate`, `target`, `next_state`, or transition key. JSON string
escaping makes `feature_id` injection-safe.

**`feature_id` fidelity (inherited Go/004 behavior, research §8)**:
- For a feature ID that is valid UTF-8, the decoded `feature_id` equals the supplied ID exactly.
  `<`, `>`, and `&` are escaped as `<`, `>`, and `&`, which decode back unchanged.
- For a feature ID containing invalid UTF-8, `encoding/json` replaces each invalid byte with
  U+FFFD, so the value is not a byte-for-byte copy. 004's JSON reports behave the same way.
- 005 adds no custom encoding.

## Determinism (FR-022)

Identical arguments, record content, and feature-directory entries produce byte-identical stdout
and identical exit codes. The artifact check produces identical results on case-sensitive and
case-insensitive hosts. Feature recognition and record lookup inherit 004's host behavior
unchanged.

## Test seams

- Every row of every table above has a test. Each test asserts the exact stdout, the exact stderr,
  and the exit code.
- Usage-failure tests wrap the root in a recording `fs.FS` (`Open`, `Lstat`, `ReadLink`,
  `ReadDir`) and assert zero calls.
- Output-failure tests (in-process) use an always-erroring `io.Writer` and expect exit `2`, a
  stderr diagnostic, and no retry (one write attempt).
- Broken-pipe test (real binary, Unix, research §11): stdout is an `os.Pipe` whose read end is
  closed before start.
  - It expects the process not to be signaled, exit `2`, and `write workflow next report:` on
    stderr.
  - A usage failure with a broken stderr pipe also exits `2`.
  - As an isolation control, `workflow state` under the same broken pipe is still terminated by
    SIGPIPE.
- "Cannot be rendered" test (in-process, through the `runWorkflowNext` seam): exit `2`, empty
  stdout, and the internal-error diagnostic.
- A UTF-8 fidelity test covers two IDs:
  - a valid-UTF-8 ID containing `<`, `>`, `&`, and non-ASCII letters round-trips exactly;
  - an ID containing byte `0xff` yields `feature_id` with U+FFFD in JSON, and `\xff` in the `%q`
    text form. This pins the documented, inherited limitation.
- JSON tests decode into `map[string]any` and compare the exact key set and value types per
  outcome.
- A text injection test uses IDs containing `\n`, `"`, `\\`, spaces, and the literal
  `outcome=step_recommended`. For each, it asserts three things:
  - the output is exactly one newline-terminated line;
  - the line begins with `ai-dev: outcome=feature_unknown feature="`;
  - after removing the `%q`-rendered ID, the remainder contains exactly one `outcome=` token.
