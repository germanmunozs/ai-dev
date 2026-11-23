# Phase 1 Data Model: Environment Doctor

**Feature**: `001-environment-doctor` | **Date**: 2026-09-27

This is the shape of the readiness domain (package `internal/doctor`), which is
intentionally ignorant of Git, Spec Kit, Claude, and Codex — see `research.md` §6. It also
documents the two external, on-disk record shapes (`internal/checks` reads, never writes,
these) that back the Spec Kit-initialization and coding-agent-integration checks.

## Domain entities (`internal/doctor`)

### `Status`

An enum with exactly two values, used for both a check's result and the report's overall
result.

| Value | Meaning |
|---|---|
| `Pass` | The inspected condition is satisfied. |
| `Fail` | The inspected condition is not satisfied. |

No third value exists (FR-003: "No check may be left unreported, partially reported, or
reported in an ambiguous state"). The Go representation assigns the zero value to `Fail`
and the non-zero value to `Pass`, so an unintentionally uninitialized status fails
conservatively rather than silently passing.

### `CheckResult`

The outcome of running exactly one `Check`.

| Field | Type | Notes |
|---|---|---|
| `ID` | `string` | Stable, kebab-case identifier (contract-level; see `contracts/`). Never changes meaning once shipped. |
| `Name` | `string` | Human-readable name, e.g. `"Claude Code"`. |
| `Area` | `string` | Grouping label, exactly `"Environment"` or `"Repository"`; value and casing are part of the machine-readable compatibility contract. |
| `Status` | `Status` | `Pass` or `Fail`. |
| `Explanation` | `string` | Required and non-empty when `Status == Fail` (FR-005); empty when `Pass`. |
| `Remediation` | `string` | Required and non-empty when `Status == Fail` (FR-005); a concise corrective action, not necessarily an exact command. Empty when `Pass`. |

Validation rule enforced by every `Check` implementation (checked in unit tests, not by the
domain type itself, to keep the domain model free of check-specific knowledge): a `Fail`
result always carries both `Explanation` and `Remediation`.

### `Check` (interface)

```go
type Check interface {
    ID() string
    Run(ctx context.Context) CheckResult
}
```

Exactly nine concrete implementations exist for this feature (see "Concrete checks"
below; eight since Feature 013, A2), constructed and assembled into a slice by the orchestration layer
(`internal/app`), which is the only place that knows there are nine, what they are named,
or in what order they run. `internal/doctor` accepts `[]Check` and never hard-codes a
count.

### `Report`

The complete outcome of one `ai-dev doctor` invocation.

| Field | Type | Notes |
|---|---|---|
| `Checks` | `[]CheckResult` | One entry per executed `Check`, in the order they were run. |
| `Overall` | `Status` | `Pass` (rendered as "Ready") iff every entry in `Checks` is `Pass`; otherwise `Fail` ("Not Ready"). Implements FR-006. |

### `RunAll`

```go
func RunAll(ctx context.Context, checks []Check) Report
```

Pure aggregation: runs every check regardless of any other check's outcome (FR-004), then
derives `Overall`. A failing or panicking individual check must not stop the others — the
plan's implementation task list will require each `Check.Run` implementation to recover
from its own unexpected errors internally and return a `Fail` `CheckResult` rather than
propagating a panic through `RunAll` (no check is allowed to have a side effect on another
check's ability to run).

## Concrete checks (`internal/checks`)

Nine `Check` values are constructed by `internal/app`, each with the stable ID it reports
in `CheckResult.ID`. IDs are part of the machine-readable contract (`contracts/`) and must
not change once released.

| # | Stable ID | Name | Mechanism used | Depends on |
|---|---|---|---|---|
| 1 | `git-tool` | Git | `ToolCheck` (binary `git`) | `system.Executor.LookPath` |
| 2 | `speckit-tool` | Spec Kit | `ToolCheck` (binary `specify`) | `system.Executor.LookPath` |
| 3 | `claude-code-tool` | Claude Code | `ToolCheck` (binary `claude`) | `system.Executor.LookPath` |
| 4 | `codex-cli-tool` | Codex CLI | `ToolCheck` (binary `codex`) | `system.Executor.LookPath` |
| 5 | `git-repository` | Current Git Repository | `GitRepoCheck` (runs `git rev-parse --is-inside-work-tree`) | `system.Executor` |
| 6 | `speckit-initialized` | Spec Kit Initialization | `ValidateManifestFiles` against `.specify/integrations/speckit.manifest.json` | `io/fs.FS` |
| 7 | `constitution` | Project Constitution | `ConstitutionCheck` against `.specify/memory/constitution.md` | `io/fs.FS` |
| 8 | `claude-integration` | Claude Spec Kit Integration | `ManifestIntegrationCheck("claude")` | `io/fs.FS` |
| 9 | `codex-integration` | Codex Spec Kit Integration | `ManifestIntegrationCheck("codex")` | `io/fs.FS` |

*Amended by Feature 013, A2 (approved 2026-11-22):* rows 3 and 4 are replaced by one row, and the checks are renumbered from 4 to 8.

| # | Stable ID | Name | Mechanism used | Depends on |
|---|---|---|---|---|
| 3 | `coding-agent-tool` | `Coding agent`, or `Coding agent: <ids>` when it passes | `CodingAgentCheck` (`agent.NewRegistry(executor).List()`) | `system.Executor.LookPath` |

`claude-code-tool` and `codex-cli-tool` are retired.

`ManifestIntegrationCheck` calls the shared `ValidateManifestFiles` function and adds one
extra condition (the integration's name must appear in the installation record — see
below); this is the
reusable mechanism described in `research.md` §7–§8 that lets a future coding agent be
added as a tenth check without any change to `internal/doctor` or to these types.

## External record shapes (read-only inputs, not owned by this feature)

These are Spec Kit's own on-disk artifacts, already present in this repository. This
feature only reads them; it never creates, writes, or migrates them.

### Spec Kit scaffold/integration manifest — `.specify/integrations/<name>.manifest.json`

```json
{
  "integration": "<name>",
  "version": "<string>",
  "installed_at": "<RFC3339 timestamp>",
  "files": {
    "<repository-relative path>": "<sha256 hex digest>",
    "...": "..."
  }
}
```

Used by check #6 (`name = "speckit"`) for the core scaffold, and by checks #8/#9
(`name = "claude"` / `"codex"`) for each coding-agent integration. `files` is the
authoritative list of paths `ValidateManifestFiles` must confirm exist and are non-empty
(§8 of `research.md`: the sha256 values are not used for equality — customization is
permitted).

### Installed-integrations record — `.specify/integration.json`

```json
{
  "version": "<string>",
  "integration_state_schema": 1,
  "installed_integrations": ["<name>", "..."],
  "integration_settings": { "<name>": { "...": "..." } },
  "integration": "<current default name>",
  "default_integration": "<name>"
}
```

`ManifestIntegrationCheck("<name>")` requires `<name>` to be present in
`installed_integrations` for the check to consider the integration's installation record
valid (FR-016). Absence of the file, a parse failure, or the name missing from the list are
all `Fail` outcomes with an explanation naming which of these occurred.

## State transitions

None. Every check is a stateless, read-only inspection re-evaluated fully on each
invocation (FR-018: identical inputs on repeated invocations yield identical results). No
entity in this feature is created, updated, or persisted by `ai-dev doctor` itself.
