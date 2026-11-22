# Contract: `ai-dev workflow approve` / `ai-dev workflow abandon`

Implements the spec's Human Gate capability (FR-001–FR-013 and FR-026–FR-031). The design rationale
is in [research.md §8–§10](../research.md).

## Usage

```text
ai-dev workflow approve [--format text|json] --approver <identity> [--reason <text>] <feature-id> <target-state>
ai-dev workflow abandon [--format text|json] --approver <identity> --reason <text> <feature-id> <target-state>
```

- The verb is the decision type: `approve` means `approval`, and `abandon` means `abandonment`.
- Exactly two positional arguments follow the flags: `<feature-id>`, then `<target-state>`.
- `--format` accepts `text` (the default) or `json`.
- `--approver` is required and may be set only once. It is never read from the environment, from
  git, or from the OS user.
- `--reason`:
  - may be set at most once;
  - is optional for `approve` and required for `abandon`;
  - when set, it must not be empty or whitespace-only.
- No standard input is read, there is no prompt, and the command never waits (FR-005).

## Steps 1–2: usage failures (exit 2, empty stdout, one stderr usage line, zero filesystem access, zero store calls)

| # | Condition |
|---|---|
| 1 | Unparseable flags, an unknown flag, `--format` not `text`/`json`, a positional count ≠ 2 |
| 1 | `<feature-id>` or `<target-state>` empty or whitespace-only |
| 1 | `--approver` absent, given twice, or empty or whitespace-only |
| 1 | `--reason` given twice, or set to an empty or whitespace-only value (D-1) |
| 1 | `abandon` without `--reason` |
| 2 | `abandon` with `<target-state>` ≠ exactly `abandoned` (including `Abandoned`, `planned`, `reviewing`) (C-13) |
| 2 | `approve` with `<target-state>` exactly `abandoned` (C-13) |

The usage line is the verb's own line shown above. Step 2 conditions are additionally explained on
stderr: `abandonment requires target "abandoned"`, or `use "ai-dev workflow abandon" to abandon a
feature`. Stdout stays empty.

## Outcomes (steps 3–9)

| Outcome | Produced by | Exit code |
|---|---|---|
| `target_unsupported` | 004, step 3 (reachable only by `approve`) | 1 |
| `feature_unknown` | 004, step 4 | 1 |
| `state_invalid` | 004, step 5 | 1 |
| `transition_invalid` | 004, step 6 (terminal origin, skip, backward move, same state, replay) | 1 |
| `artifacts_missing` | 005 rule, step 7 (`approve` only) | 1 |
| `state_conflict` | store, step 8: an active competing holder of the feature directory's lock, the feature directory replaced, or the state no longer equal to the evaluated state | 1 |
| `persistence_failed` | store, step 8 or 9: evidence that cannot be preserved, a write or rename failure, an ambiguous lock error, or an unsupported platform (spec FR-037) | 1 |
| `transition_applied` | store, step 9 commit | 0 |

Exit 2 is also returned for a report or output failure (FR-031). After `transition_applied`, exit 2
means that the decision **was** committed. Re-observe it with `ai-dev workflow state`.

## Text format (stdout, one line, `%q` for feature, approver, supplied target, and reason)

```text
ai-dev: outcome=transition_applied feature="<id>" decision=<d> approver="<a>" from=<s> to=<t> [reason="<r>"] note="transition applied on the decision asserted by the named approver; approver identity is not verified"
ai-dev: outcome=transition_invalid feature="<id>" decision=<d> approver="<a>" from=<s> target=<t> note="no transition was applied"
ai-dev: outcome=state_conflict feature="<id>" decision=<d> approver="<a>" from=<s> target=<t> note="no transition was applied"
ai-dev: outcome=persistence_failed feature="<id>" decision=<d> approver="<a>" from=<s> target=<t> note="no transition was applied"
ai-dev: outcome=artifacts_missing feature="<id>" decision=approval approver="<a>" from=<s> target=<t> missing=<a,b> note="no transition was applied"
ai-dev: outcome=target_unsupported feature="<id>" decision=approval approver="<a>" target="<as supplied>" note="no transition was applied"
ai-dev: outcome=feature_unknown feature="<id>" decision=<d> approver="<a>" note="no transition was applied"
ai-dev: outcome=state_invalid feature="<id>" decision=<d> approver="<a>" note="no transition was applied"
```

No report, in either form, makes an affirmative identity claim (FR-033, SC-009). A test rejects the
substrings `authenticated`, `verified approver`, `identity verified`, and `approved by`. The only
identity wording is the fixed negation "approver identity is not verified". The tokens `approval`
and `approved` appear only as decision and state values.

## JSON format (stdout, exactly one object)

The keys are `feature_id`, `outcome`, `decision`, and `approver` (always present), plus the
following:

| Outcome | `from_state` | `supplied_target` | `target` | `reason` | `missing_artifacts` |
|---|---|---|---|---|---|
| `transition_applied` | ✓ | ✓ | ✓ | iff supplied | — |
| `transition_invalid`, `state_conflict`, `persistence_failed` | ✓ | ✓ | ✓ | — | — |
| `artifacts_missing` | ✓ | ✓ | ✓ | — | ✓ (non-empty) |
| `target_unsupported` | — | ✓ | — | — | — |
| `feature_unknown`, `state_invalid` | — | — | — | — | — |

The key names follow 004's transition report (`supplied_target`, `target`) and 005's
(`missing_artifacts`).

## Stderr

The following go to stderr:
- usage lines;
- the step 2 explanation;
- report or output failure (`write workflow gate report: <err>`);
- internal errors (an invalid report, or a nil store);
- store warnings, after the report: a post-commit directory flush or descriptor close failure
  (`ai-dev: warning: committed, but …`), or the unsupported-platform notice
  (`ai-dev: warning: human gate persistence is supported only on macOS and Linux`), which accompanies
  `persistence_failed`.

macOS and Linux are the only supported persistence platforms. Windows, iOS, Android, and every
other platform take the fail-closed fallback described above. Native Windows runtime validation is
deferred under H-7; this does not change the required `persistence_failed` behavior or claim Windows
support.

Warnings never change the exit code.

## Test seams

- `internal/app`: `runWorkflowGate(args, root, store, now, stdout, stderr)` is called with a
  recording `fs.FS` and a recording fake `Store`. Every usage row is checked for zero filesystem
  and store calls. Every outcome is checked in both text and JSON.
- The C-13 matrix covers: `approve`/`abandon` × {`abandoned`, `Abandoned`, `planned`, `reviewing`}.
- Output-failure tests:
  - an always-failing writer after `Committed` gives exit 2, with the store called exactly once and
    the record committed;
  - a Unix subprocess with a closed stdout pipe gives exit 2, not 141.
- `cmd/ai-dev`: a subprocess contract test runs the real binary against a temp repository, taking a
  feature from `draft` through to `approved` and checking `workflow state` / `workflow next` after
  each step.
