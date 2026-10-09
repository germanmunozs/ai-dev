# Contract: `sdd.MissingArtifacts` (minimal 005 API extension)

This is an internal Go API contract, not a user-facing one. The rationale is in
[research.md §2](../research.md).

## Signature

```go
// internal/sdd/rules.go
func MissingArtifacts(root fs.FS, featureID string, state workflow.State) []string
```

## Behavior

The body is exactly `return missingArtifacts(root, featureID, state)`. It therefore inherits 005's
approved semantics with no change:
- It uses the Required Artifacts rows of 005 FR-010, through the existing unexported
  `requiredArtifacts`.
- It performs a single `fs.ReadDir` of `workflow.FeatureDir(featureID)`.
- An artifact counts as present only on an exact, byte-equal, case-sensitive name match **and**
  when `DirEntry.Type().IsRegular()` holds. Symlinks are never followed.
- A `ReadDir` error, or a structurally invalid ID, makes every required artifact count as missing.
- The result lists artifacts in the fixed order `spec.md`, `plan.md`, `tasks.md`. It is `nil` when
  the state requires nothing (`draft`, `abandoned`).

## Constraints (binding)

- It adds no import to `internal/sdd`, and the existing `boundary_test.go` allow-lists stay
  unchanged.
- `Evaluate`, `nextStep`, `requiredArtifacts`, `missingArtifacts`, `Report.Valid`, and all outcomes
  are untouched.
- It is a read-only function: no writes, and no extra filesystem calls beyond those
  `missingArtifacts` already makes.
- **Callers**: only `internal/gate`, only for `approval`, only after `workflow.EvaluateTransition`
  returned `transition_valid`, and only with the **explicit target** state.

## Regression obligations

- All existing `internal/sdd` tests pass unchanged.
- New additive tests:
  - an equivalence table: for every canonical state and every directory fixture already used in
    `rules_test.go`, `MissingArtifacts` equals `missingArtifacts`;
  - through a recording `fs.FS`, a `MissingArtifacts` call performs exactly one `ReadDir` on
    `FeatureDir(id)`, and no `Open`, `Lstat`, or `ReadLink`.
