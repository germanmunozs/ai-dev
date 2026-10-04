# Phase 0 Research: Workflow State

All items the spec left as explicit plan-level decisions (per spec Assumptions: "Command syntax,
programming language, CLI framework, and machine-readable serialization format are implementation/
plan concerns"). No `NEEDS CLARIFICATION` markers remain in Technical Context — every open question
below has a resolved decision.

## 1. Workflow State Record file name and schema

- **Decision**: A single JSON file, `workflow-state.json`, colocated inside `specs/<feature-id>/`,
  with exactly one field: `{"state": "<string>"}`.
- **Rationale**: FR-027 requires the record to live inside the feature's own spec-kit directory,
  distinct from `spec.md`/`plan.md`/`tasks.md`; a one-field JSON object is the minimum structure
  that can hold "one of seven canonical values, or something that isn't" (FR-006/FR-007), and JSON
  is already this project's report-serialization choice (`internal/report/json.go`,
  `roleselection.go`), so no new serialization convention is introduced. Using `encoding/json`
  (stdlib) keeps "no new dependency" intact.
- **Alternatives considered**: A bare text file containing just the state string (rejected: less
  obviously extensible if a future feature needs to add fields such as a timestamp, and
  inconsistent with every other structured artifact `ai-dev` already produces/consumes in JSON);
  YAML (rejected: no existing YAML dependency in this module, and this project avoids new
  dependencies); embedding state inside `plan.md`/`tasks.md` front-matter (rejected: FR-027
  explicitly requires the record be distinct from those artifacts).

## 2. Feature identifier resolution

- **Decision**: Validate the raw feature identifier before any `path.Join`/`path.Clean` operation.
  It is structurally valid only when `fs.ValidPath(featureID)` is true, `featureID != "."`, and
  `featureID` contains no `/`. This admits exactly one relative feature-directory name and rejects
  empty/invalid UTF-8 values, absolute paths, `.`/`..`, traversal, and nested path components.
  Only after that check may the implementation form `specs/<feature-id>`. A structurally valid
  identifier resolves to an existing feature only when the supplied filesystem implements
  `fs.ReadLinkFS` and its `Lstat` reports that path as a real directory, not a symlink. The
  production `os.DirFS(".")` and the planned `fstest.MapFS` fixtures both implement that interface;
  a filesystem without no-follow inspection support is rejected rather than silently falling
  back to link-following behavior.
- **Rationale**: Matches spec Assumptions exactly ("corresponds to that feature's spec-kit feature
  directory, consistent with the numbered `specs/<NNN-name>` convention"); reuses the same `root
  fs.FS` parameter `RunDoctor` already receives (constructed once in `main.go` via
  `os.DirFS(".")`), so no new way of locating the repository root is introduced. Validating the
  raw value, rather than relying on a joined path to remain invalid, makes containment explicit:
  no accepted identifier can alter or add to the single path component below `specs/`.
- **Alternatives considered**: Requiring `spec.md` to also exist inside that directory (rejected:
  spec.md's existence is not part of the approved "recognized as existing" definition in
  spec.md's Assumptions, and would make a feature's state undeterminable during the brief window
  between `mkdir specs/<id>` and `spec.md` being written by `/speckit-specify`); consulting
  `.specify/feature.json` instead of scanning `specs/` directly (rejected: `feature.json` names
  only the *current* feature being worked on, not an index of every feature that exists, so it
  cannot answer "does `<arbitrary-id>` exist"); cleaning or sanitizing arbitrary paths (rejected:
  it could silently reinterpret caller input and is unnecessary when a three-part raw-value
  predicate enforces one directory name); relying on `fs.ValidPath(path.Join(...))` (rejected:
  `path.Join` can normalize traversal before validation and therefore cannot prove containment).

  A non-empty, non-whitespace identifier that fails this structural rule is a recognized request
  whose feature does not resolve, so its outcome is the specification's existing
  `feature_unknown`, never a new outcome and never a usage failure. For transition evaluation,
  FR-029 still checks candidate-target canonicality first: an unsupported target returns
  `target_unsupported` before feature-identifier validation or any filesystem access.

## 3. CLI command shape

- **Decision**: `ai-dev workflow state [--format text|json] <feature-id>` and
  `ai-dev workflow transition [--format text|json] <feature-id> <target-state>`, dispatched via
  one new `args[0] == "workflow"` branch in `app.Run`, with `state`/`transition` as the two
  recognized subcommands — mirroring the existing `agent <run|select>` two-subcommand shape
  exactly.
- **Rationale**: Consistent with the established `ai-dev <verb> <noun-ish-subcommand>
  [--format ...] <positional args>` shape used by `agent run`/`agent select`; `--format text|json`
  matches the flag name, values, and default (`text`) already used by every existing command.
- **Alternatives considered**: A single `ai-dev workflow <feature-id> [target-state]` command that
  infers reporting vs. evaluation from argument count (rejected: ambiguous and harder to validate
  — a mistyped feature-id that happens to collide with a target-state-shaped string would be
  misclassified; two explicit subcommands match FR-001/FR-010's framing as two separate
  capabilities and avoid inventing an inference rule the spec never defines).

## 4. Proving the FR-029 "no record read" guarantee

- **Decision**: Prove it two ways: (a) structurally, via `EvaluateTransition`'s own control flow
  (candidate-target canonicality returns before `FeatureExists`/`ResolveRecord` are called in the
  source text itself); (b) behaviorally, via a small `fs.FS`-wrapping test double that records
  every `name` passed to `Open` (the one required `fs.FS` method; `fs.Stat`/`fs.ReadFile` fall
  back to it when the optional `fs.StatFS`/`fs.ReadFileFS` interfaces are absent from the
  underlying `fstest.MapFS`), asserting zero recorded calls for a non-canonical target.
- **Rationale**: Mirrors exactly how 003 proved "no execution": a structural proof (parameter list
  / control flow) plus an independent behavioral proof (a recording fake), so the guarantee does
  not rest on trusting either kind of evidence alone.
- **Alternatives considered**: Only the structural proof (rejected: a future refactor could
  silently reorder the `if` statements without any test catching it); only the behavioral proof
  (rejected: weaker without the code-reading guarantee 003's own plan explicitly valued).

## 5. Record-matching reuse to prevent drift (I-1)

- **Decision**: `ResolveRecord` calls the same `Recognized` function `EvaluateTransition` already
  uses to validate a candidate target, rather than re-implementing a second "is this one of the
  seven values" check.
- **Rationale**: The approved spec (FR-006) requires record-content matching to be exactly as
  strict (case-sensitive, no trimming/normalization) as candidate-target matching; reusing one
  function makes it structurally impossible for the two checks to drift apart over time.
- **Alternatives considered**: A separate, record-specific recognition function (rejected: direct
  duplication risk the independent spec review's I-1 finding was specifically about).

## 6. `terminal` field and JSON `omitempty`

- **Decision**: `StateReport`'s wire type uses `*bool` for `terminal`, populated only when the
  outcome is `state_reported`; plain `omitempty` on a `bool` is not used.
- **Rationale**: `terminal: false` is a legitimate, required value for every non-terminal
  `state_reported` result (e.g. `planned`); a plain `bool` with `omitempty` would silently drop the
  key whenever the state happens to be non-terminal, violating FR-019's "required whenever
  determinable" rule for exactly the common case (five of seven states are non-terminal).
- **Alternatives considered**: A plain `bool` without `omitempty` on every outcome (rejected:
  would then render `terminal:false` even for `feature_unknown`/`state_invalid`, where FR-019
  requires the field to be absent, not merely falsy).

## 7. Symlink semantics

- **Decision**: Neither `specs/<feature-id>` nor its `workflow-state.json` may be a symlink.
  Feature existence requires `fs.ReadLinkFS` and uses that interface's `Lstat`, accepting only a
  non-symlink directory. Record resolution uses the same no-follow operation before `fs.ReadFile`;
  an absent record defaults to `draft`, while a symlink or any other non-regular record is
  `state_invalid`. A filesystem without `fs.ReadLinkFS` support does not fall back to `fs.Stat`:
  feature existence is false (and defensive direct record resolution is invalid). Thus the
  feature-controlled path components are not followed outside their declared location.
- **Rationale**: This is the smallest rule that preserves C-1's containment guarantee for the
  local read-only CLI. It requires no general sandbox or path-sanitization framework and fits
  FR-007, which already classifies unreadable or non-usable existing records as `state_invalid`.
  The repository root and its top-level `specs` directory remain the trusted boundary supplied by
  the existing `root fs.FS`. As with the specification's concurrency edge case, no atomic
  protection against a separate process replacing entries between `Lstat` and `ReadFile` is
  required.
- **Alternatives considered**: Following feature-directory symlinks (rejected: could make an
  accepted feature identifier resolve outside `specs/`); following record symlinks (rejected:
  could read a record outside the feature directory); rejecting every filesystem that supports
  symlinks (rejected as disproportionate—the rule applies only to the two resolved entries).

## Output

All `NEEDS CLARIFICATION` items resolved (none existed beyond the above plan-level decisions,
all of which are now fixed). Proceeding to Phase 1.
