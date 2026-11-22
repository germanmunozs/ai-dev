# Contract: Workflow State Record

The plan-level concrete representation of spec.md's **Workflow State Record** entity (location
and format deliberately left open by the specification; resolved here per research.md §1).

## Location

`specs/<feature-id>/workflow-state.json`, relative to the repository root `fs.FS` `ai-dev` already
uses for every other repository-relative check (`os.DirFS(".")` in `main.go`).

The raw feature ID is validated before constructing this path: `fs.ValidPath(featureID)` must be
true, the value must not be `.`, and it must contain no `/`. The filesystem must implement
`fs.ReadLinkFS`; its no-follow `Lstat` must report the feature path as a real directory and
`workflow-state.json` as a real regular file. A filesystem without that interface is rejected
rather than falling back to `fs.Stat`; neither entry is followed.

## Schema

```json
{"state": "planned"}
```

A JSON object with exactly one field this feature reads, `state`, a string. The member name is
matched exactly and case-sensitively (human decision, I-1 remediation): `State`, `STATE`, `ſtate`,
or any other variant that equals `state` only under case folding is **not** the `state` field,
and its presence makes the record invalid; an exactly duplicated `state` member is also invalid —
there is no last-key-wins resolution. Any other field present in the object is ignored by this
feature (forward-compatible with a future feature adding fields such as a timestamp or an audit
trail, without requiring this feature's reader to change). Only top-level members are considered.

## Reading rules (FR-005, FR-006, FR-007)

| Condition | Result |
|---|---|
| File does not exist | `State = draft`, `invalid = false` (FR-005 — the only "default" case) |
| File path is a symlink | `invalid = true` (FR-007); the target is not followed |
| File exists but is not regular | `invalid = true` (FR-007) |
| File exists, cannot be opened/read (I/O error other than not-exist) | `invalid = true` (FR-007) |
| File exists, content is not valid JSON | `invalid = true` (FR-007) |
| File exists, valid JSON, but not a single JSON object (e.g. a string, array, or `null`) | `invalid = true` (FR-007) |
| File exists, valid JSON, `state` field absent or `null` | `invalid = true` (FR-007) |
| File exists, valid JSON, a case variant of the member name is present (e.g. `{"State":"approved"}`, `{"STATE":"planned"}`, `{"ſtate":"approved"}`, `{"state":"draft","State":"approved"}`) | `invalid = true` (FR-006/FR-007) — the variant is never read as `state` |
| File exists, valid JSON, the exact `state` member appears more than once (e.g. `{"state":"draft","state":"approved"}`) | `invalid = true` (FR-007) — no last-key-wins |
| File exists, valid JSON, `state` is not a string (e.g. a number) | `invalid = true` (FR-007) |
| File exists, valid JSON, `state` is `""` or whitespace-only | `invalid = true` (FR-007) |
| File exists, valid JSON, `state` is not one of the seven canonical tokens (including wrong case or padded whitespace, e.g. `"Planned"`, `" planned "`) | `invalid = true` (FR-007) |
| File exists, valid JSON, `state` is exactly one of the seven canonical tokens | `State = <that value>`, `invalid = false` |

No case other than "file does not exist" produces `draft`; every other non-exact condition
produces `invalid = true`, never a guess or a silently-substituted value (I-1/I-3).

## Write path

**None.** This feature defines no function, flag, or code path anywhere that creates, writes, or
updates this file (FR-027). `ResolveRecord`'s only parameter capable of touching storage is an
`io/fs.FS`, an interface with no write method — a future feature that adds the ability to apply a
transition will need its own, separately-planned write mechanism; this plan intentionally
introduces none.

## Test seams

- A dedicated `internal/workflow/record_test.go` table test covers every row of the Reading Rules
  table above against an `fstest.MapFS` fixture, for both `FeatureExists`+`ResolveRecord` calls
  directly and through `EvaluateState`/`EvaluateTransition`.
- The same table covers the I-1 member-name rules: `{"State":"approved"}`, `{"STATE":"planned"}`,
  `{"ſtate":"approved"}`, `{"state":"draft","State":"approved"}`, and
  `{"state":"draft","state":"approved"}` each resolve invalid, and an `internal/app` end-to-end
  test confirms `state_invalid` (exit `1`) for both commands in text and JSON.
- A permission-error / non-regular-file case (e.g. a directory named `workflow-state.json`) is
  included to exercise the "I/O error other than not-exist" row distinctly from the
  "malformed JSON" row, per I-3.
- Symlink fixtures prove a symlinked feature directory produces `feature_unknown` and a symlinked
  record produces `state_invalid` without reading either target.
- A wrapper that exposes only `fs.FS`, not `fs.ReadLinkFS`, proves the implementation rejects the
  feature as unknown rather than invoking a link-following fallback.
