# CLI Contract: `ai-dev doctor`

**Feature**: `001-environment-doctor`

This is the externally observable interface of the command. Anything described here is a
compatibility surface: stable check IDs, output shapes, and exit codes must not change
meaning without a deliberate, documented decision in a future feature.

## Invocation

```text
ai-dev doctor [--format text|json]
```

- No positional arguments.
- `--format` is optional. Valid values: `text` (default), `json`.
- An unrecognized flag or an invalid `--format` value is a usage error: the process prints
  a usage message to stderr and exits `2` without running any check.
- The command performs a full, read-only assessment of the environment and the current
  working directory's repository state (see `data-model.md` for the checks run: nine, eight since Feature 013 Amendment A2). It
  makes no network calls, writes nothing to disk, and starts no background process.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Overall result is Ready (every check Passed). |
| `1` | Overall result is Not Ready (at least one check Failed). |
| `2` | Usage error (bad flags); no checks were run. |

This holds identically for `--format text` and `--format json` (FR-006, Success Criterion
SC-007): an automated caller never needs to parse output to learn whether it is safe to
proceed.

## `--format text` (default): human-readable output

Written to stdout. Exact spacing/column widths are not part of the contract (may improve
over time); the following elements are:

- Every check is listed with: its `Name`, `Area`, and result (`PASS`/`FAIL`).
- Every `FAIL` result is followed by its `Explanation` and its `Remediation`, both fully
  legible without consulting external documentation (FR-007).
- A final overall line states `Ready` or `Not Ready`.

Example (illustrative, not a byte-exact contract):

```text
Environment
  [PASS] Git
  [PASS] Spec Kit
  [PASS] Claude Code
  [FAIL] Codex CLI
        Codex CLI (codex) could not be found on PATH.
        Install the Codex CLI and ensure it is available on PATH.

Repository
  [PASS] Current Git Repository
  [PASS] Spec Kit Initialization
  [PASS] Project Constitution
  [PASS] Claude Spec Kit Integration
  [FAIL] Codex Spec Kit Integration
        Codex integration file .agents/skills/speckit-plan/SKILL.md is missing.
        Reinstall the Codex Spec Kit integration.

Overall: Not Ready
```

## `--format json`: machine-readable output

Written to stdout as a single JSON object, UTF-8, no trailing newline requirement beyond
what `encoding/json` + a final `\n` naturally produce. Schema:
`contracts/doctor-report.schema.json`.

Top-level shape:

```json
{
  "overall": "ready",
  "checks": [
    {
      "id": "git-tool",
      "name": "Git",
      "area": "Environment",
      "status": "pass",
      "explanation": "",
      "remediation": ""
    }
  ]
}
```

- `overall`: `"ready"` or `"not_ready"`. `"ready"` iff every `checks[].status == "pass"`.
- `checks`: always exactly the checks listed in `data-model.md`'s table, in that order,
  every invocation (FR-002, FR-017) — never fewer, never more, regardless of environment or
  repository state.
- `checks[].id`: stable identifier from `data-model.md`. A program MUST be able to key off
  this field instead of `name` or `explanation` wording (FR-008).
- `checks[].area`: exactly `"Environment"` or `"Repository"`, with this casing. These
  values are part of the machine-readable compatibility contract; automation may use them
  for grouping, while `checks[].id` remains the stable identity of an individual check.
- `checks[].status`: `"pass"` or `"fail"`.
- `checks[].explanation` / `checks[].remediation`: empty strings when `status == "pass"`;
  non-empty when `status == "fail"` (FR-005).

No other top-level or per-check fields are guaranteed by this contract; additional
optional fields MAY be added in a backward-compatible way in a future feature (existing
keys will not be renamed or repurposed without a major, documented change).

## Stability guarantees

- The nine `id` values in `data-model.md` are permanent identifiers. A future feature that
  adds a tenth coding-agent-integration check adds a new `id`; it does not renumber or
  rename the existing nine.
  *(Amended by Feature 013, A2, approved by the owner on 2026-11-22: `claude-code-tool` and
  `codex-cli-tool` are retired and replaced by `coding-agent-tool`, which passes when any
  catalog agent is available. Doctor reports eight checks. The other seven identifiers are
  unchanged and keep this guarantee, and so does `coding-agent-tool`. When it passes, its
  `name` is `Coding agent: <ids>`; `explanation` and `remediation` stay empty, as this
  contract requires for a passing check. Automation keys off `id`, never `name`.)*
- Exit code meanings (`0`/`1`/`2`) do not change.
- `--format text` prose (the `Explanation`/`Remediation` wording) is *not* part of the
  contract and may be reworded for clarity without being a breaking change — this is why
  FR-008 requires the JSON path for anything automation depends on.
