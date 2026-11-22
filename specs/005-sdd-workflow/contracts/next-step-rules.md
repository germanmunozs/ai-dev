# Contract: Next-Step Table and Required Artifacts rule (`internal/sdd`)

This contract describes the plan-level representation of FR-007 to FR-012. Both rules are
**code, not data**. They are unexported exhaustive `switch` functions, with no map, slice literal,
or exported variable that could be mutated, listed, persisted, or configured. This follows the
pattern of 003's `mappedAgent` and 004's `Allowed`.

## `nextStep(state workflow.State) (Activity, role.Role, bool)`

```go
switch state {
case workflow.Draft:       return ActivitySpecify, role.Specification, true
case workflow.Specified:   return ActivityPlan, role.Planning, true
case workflow.Planned:     return ActivityTasks, role.Planning, true
case workflow.Tasked:      return ActivityImplement, role.Implementation, true
case workflow.Implemented: return ActivityReview, role.Review, true
default:                   return "", "", false
}
```

- **What it does not return:** no target state, next state, or transition. It never references
  `workflow.Allowed`, `EvaluateTransition`, or `TransitionReport` (FR-003). The import and
  identifier boundary test enforces this.
- **Table miss:** the evaluator reaches the table through the unexported seam
  `evaluate(root, id, lookup)`, and production passes `nextStep`. If a non-terminal state that 004
  reports validly has no entry, the result is an FR-020 output failure of the "cannot be
  rendered" subcase: exit 2, empty stdout, stderr diagnostic (research §6). There is no invented
  activity or role, and no outcome. FR-007 makes this unreachable while the approved table is
  complete.
- **Terminal states:** the evaluator never decides terminality from this table. It uses
  `StateReport.Terminal` from 004 before reaching step 6. The table merely has no terminal entries.
- **Required tests:**
  - for each of the 7 canonical states, `ok == !workflow.Terminal(state)`;
  - the 5 rows match exactly;
  - `role.Remediation` is never returned;
  - each returned role satisfies `role.Recognized`;
  - activity tokens are disjoint from state tokens.

## `requiredArtifacts(state workflow.State) []string`

```go
switch state {
case workflow.Specified:                                       return []string{SpecArtifact}
case workflow.Planned:                                         return []string{SpecArtifact, PlanArtifact}
case workflow.Tasked, workflow.Implemented, workflow.Approved: return []string{SpecArtifact, PlanArtifact, TasksArtifact}
default: /* draft, abandoned */                                return nil
}
```

The function returns a freshly allocated slice on every call, so no caller can corrupt the rule.

## `missingArtifacts(root fs.FS, featureID string, state workflow.State) []string`

1. Call `required := requiredArtifacts(state)`. If it is empty, return `nil`. No filesystem call
   is made.
2. Call `dir, ok := workflow.FeatureDir(featureID)`. If `!ok`, return `required`. This is
   defensive and unreachable after `state_reported`.
3. Call `entries, err := fs.ReadDir(root, dir)`. If `err != nil`, return `required`, because no
   entry could be inspected.
4. Build the set of names `e.Name()` for which `e.Type().IsRegular()` holds.
5. Return the names in `required` that are absent from that set, keeping canonical order.

- **Calls it never makes:** `Open`, `Stat`, `Lstat`, `ReadLink`, or `ReadFile` on any artifact
  path. Exactly one `ReadDir` is made, on `dir`.
- **Case:** `Plan.md`, `SPEC.md`, and `Tasks.md` never match, on any host (research §3).
- **Symlinks:** a symlink entry has `fs.ModeSymlink` in `Type()`, so it is not regular and counts
  as missing. It is never followed.
- **Directories and other non-regular entries:** counted as missing.
- **Content:** never read. A regular file with unreadable content counts as present.
- **"Unreadable" (human planning decision, research §3):** within 005, this means unreadable or
  uninspectable through the metadata-only `ReadDir` inspection. It never means inability to read
  artifact contents. Failure to list or inspect safely → missing. File-byte permissions are never
  probed.

## Full outcome matrix (with required artifacts present)

| State | Step 4 | Step 5 | Step 6 | Outcome |
|---|---|---|---|---|
| `draft` | pass (nothing required, no I/O) | not terminal | `specify` / `specification` | `step_recommended` |
| `specified` | needs `spec.md` | not terminal | `plan` / `planning` | `step_recommended` |
| `planned` | needs `spec.md`, `plan.md` | not terminal | `tasks` / `planning` | `step_recommended` |
| `tasked` | needs all three | not terminal | `implement` / `implementation` | `step_recommended` |
| `implemented` | needs all three | not terminal | `review` / `review` | `step_recommended` |
| `approved` | needs all three | terminal | — | `workflow_terminal` |
| `abandoned` | pass (nothing required, no I/O) | terminal | — | `workflow_terminal` |

Whenever any required artifact is missing, step 4 yields `artifacts_inconsistent` with the
missing list. That includes `approved`. `abandoned` and `draft` can never be inconsistent.
