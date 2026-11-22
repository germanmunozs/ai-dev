# Contract: `workflow.FeatureDir` (minimal 004 API extension)

This is an internal Go API contract, not user-facing. The rationale is in
[research.md §2](../research.md).

**Status**: approved as a human planning decision (2026-10-05). The constraints are binding:
- It centralizes only the safe derivation of `specs/<feature-id>`.
- It reuses exactly 004's approved structural validation.
- It performs no I/O.
- It changes none of the following: feature-recognition semantics, state resolution, symlink
  semantics, outcomes, or transition behavior.
- It exposes no second source of truth.
- The refactor of `FeatureExists` and `ResolveRecord` is semantically neutral and protected by
  regression tests.
- 005 uses this API and never reconstructs `specs/<feature-id>` itself.

## Signature

```go
// internal/workflow/record.go
func FeatureDir(featureID string) (dir string, ok bool)
```

## Behavior

| Input | Result |
|---|---|
| `featureID` satisfies the existing `validFeatureID` (`!= "."`, `fs.ValidPath`, contains no `/`) | `path.Join("specs", featureID)`, `true` |
| anything else (`""`, `.`, `..`, `../x`, `a/../../x`, `/etc/passwd`, `a/b`) | `""`, `false` |

- It validates **first**, with `validFeatureID`, and only then builds the path with
  `path.Join("specs", featureID)`. This is the same expression and order the current
  `FeatureExists` uses.
- It performs **no I/O**: no `fs.FS` parameter, no filesystem call.
- It never trims, cleans, or rewrites the input (validation, not sanitization).
- It does **not** assert existence. Existence remains `FeatureExists` / `EvaluateState`.

## Refactor of existing 004 functions (behavior-preserving)

- `FeatureExists` obtains its `Lstat` path from `FeatureDir` and returns `false` when `ok` is
  false. That matches today's early `validFeatureID` return.
- `ResolveRecord` obtains its directory from `FeatureDir` and returns `invalid = true` when `ok`
  is false, matching today's defensive branch. The record path is
  `path.Join(dir, RecordFileName)`.
- No other line of `internal/workflow` changes. `Recognized`, `Terminal`, `Allowed`,
  `stateField`, `EvaluateState`, `EvaluateTransition`, `Valid()`, and all outcomes are untouched.

## Who may call it

- `internal/workflow` itself.
- `internal/sdd`, only after `EvaluateState` returned `state_reported` (precedence step 4,
  FR-006), and in `Report.Valid()` as an I/O-free structural check.

## Regression obligations

- All existing tests in `internal/workflow`, `internal/report`, `internal/app`, and `cmd/ai-dev`
  keep passing. No existing assertion may be removed or weakened. Any change to an existing test
  file must be additive and justified.
- New additive tests cover three things:
  - the input table above;
  - through a recording `fs.ReadLinkFS` wrapper, `FeatureExists(id)` calls `Lstat` exactly on
    `FeatureDir(id)`, and `ResolveRecord(id)` calls `Lstat` and `Open` exactly on
    `FeatureDir(id) + "/workflow-state.json"`;
  - every C-1 invalid ID still produces `feature_unknown` with zero filesystem calls, through
    both `EvaluateState` and `EvaluateTransition`.
