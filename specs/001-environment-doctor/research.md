# Phase 0 Research: Environment Doctor

**Feature**: `001-environment-doctor` | **Date**: 2026-09-27

This document resolves every open technical question needed to design `ai-dev doctor`
without contradicting the technology-agnostic feature spec. The human sponsor has already
approved the top-level technology direction (Go CLI, stdlib-first, no CLI/TUI framework,
JSON for machine output); this document records the *rationale* and the remaining
implementation-level decisions that direction implies.

## 1. Language, toolchain, and target platform

- **Decision**: Go (toolchain installed in the dev environment: `go1.27.1 darwin/arm64`).
  Module declares a `go` directive pinning the minimum language version used.
- **Rationale**: Approved direction. Go produces a single static binary (easy distribution
  for a dev-machine CLI), has a capable standard library (`os/exec`, `encoding/json`,
  `flag`, `io/fs`, `testing`) that covers every need of this feature without external
  dependencies, and its toolchain (`gofmt`, `go vet`, `go test`, `go build`) directly
  supplies the required quality gates.
- **Alternatives considered**: Python (rich stdlib but requires a packaged interpreter/venv
  story for distribution; weaker static-binary story) and Rust (excellent binaries but a
  heavier learning/build curve than this feature's scope justifies). Both rejected as
  unnecessary given the approved direction and Simplicity/YAGNI (Principle III).
- **Target platform**: macOS on Apple Silicon (`darwin/arm64`) is the only officially
  supported/tested target for this feature, per the approved direction. Domain code
  (`internal/doctor`, `internal/checks`'s pure logic) avoids OS-specific APIs so a future
  Linux/Windows target is a build-and-test exercise, not a redesign; this is a portability
  *side effect*, not a requirement being implemented now (no cross-platform CI or code path
  is added in this feature).

## 2. CLI argument handling: stdlib `flag`, no framework

- **Decision**: Use the standard library `flag` package. `ai-dev` is a single-purpose binary
  invoked as `ai-dev doctor`; the only optional input is an output-format selector.
- **Rationale**: `flag` fully covers "one subcommand, one optional enum flag." A CLI
  framework (Cobra, urfave/cli, kong, …) would add a dependency and abstraction with no
  capability `flag` lacks for this surface. Principle II (Architecture Before Framework)
  requires a framework to be justified by a concrete need; none exists here. If a future
  feature adds many subcommands, shared flag groups, or shell-completion requirements, that
  feature's plan can revisit this decision with its own justification.
- **Alternatives considered**: Cobra/urfave-cli (rejected: no functional gap they close for
  a single flag), hand-rolled arg parsing without `flag` (rejected: reinvents what the
  stdlib already provides correctly, e.g. `-h`/`--help`, `-format=x` vs `-format x`).

## 3. Terminal UI: plain text, no TUI framework

- **Decision**: Human-readable output is plain formatted text written with `fmt`/
  `text/tabwriter`, no colors, no interactive elements, no TUI framework (bubbletea, etc.).
- **Rationale**: The spec's terminal-output requirement (FR-007) is satisfied by legible,
  well-structured static text. A doctor command is a one-shot, non-interactive report;
  nothing here needs rendering loops, redraws, or terminal capability negotiation.
- **Alternatives considered**: A TUI framework — explicitly excluded by the approved
  direction and unjustified by any requirement.

## 4. Machine-readable format: JSON

- **Decision**: The machine-readable report is JSON, selected via `ai-dev doctor --format
  json` (default `--format text`), encoded with the standard library `encoding/json`.
- **Rationale**:
  - Ubiquitous, dependency-free parsing on the consuming side (any CI system, script
    language, or future orchestrator) with zero extra tooling.
  - `encoding/json` is stdlib, keeping the "no unnecessary dependency" constraint intact.
  - A typed Go struct with an explicit field order/naming gives every check a stable,
    documented shape (see `contracts/doctor-report.schema.json`), which directly satisfies
    FR-008's "stable identifier" and "independent of wording" requirements.
  - **Trade-offs accepted**: JSON has no native comments and is slightly more verbose than
    a line-oriented format (e.g. TSV) for trivial `grep`-based consumption; a caller that
    only wants a boolean must still parse structured data rather than reading a single
    exit-code-shaped line. This is judged acceptable because (a) the exit code alone
    already answers the boolean "ready or not" question (see §6), so JSON is only needed
    when a caller wants per-check detail, and (b) every mainstream scripting environment
    has a trivial JSON parser, so the marginal cost over TSV/CSV is negligible while the
    structural guarantees (stable keys, nesting, explicit null-vs-empty) are stronger.
  - **`--format` flag over an implicit `--json` boolean**: `--format <text|json>` is chosen
    over a `--json` boolean so a future additional machine format (e.g. `--format
    ndjson`) doesn't require a second, differently-shaped flag; today only two values are
    implemented. Not a speculative abstraction — it costs nothing beyond a string-enum flag
    conditional and directly serves FR-008's "explicitly selectable" requirement.
- **Alternatives considered**: YAML (rejected: needs a third-party parser to be robust,
  contradicting stdlib-first; humans rarely consume the machine-readable path anyway), a
  flat key=value/TSV format (rejected: does not naturally express nested
  explanation/remediation strings without escaping rules), plain text with a parseable
  marker convention (rejected: exactly the "wording-dependent parsing" FR-008 rules out).

## 5. Ready/Not-Ready ↔ process exit status

- **Decision**: `ai-dev doctor` exits `0` when the overall result is Ready and a non-zero
  code (`1`) when it is Not Ready, for both `--format text` and `--format json`. A usage
  error (e.g., an invalid `--format` value) exits `2`, the conventional Go `flag`-package
  usage-error code, before any check runs.
- **Rationale**: Directly implements FR-006 and the approved direction ("Ready must result
  in successful process completion and Not Ready in unsuccessful process completion") in
  the only way a POSIX process can signal a binary outcome to a calling script — the exit
  code — independent of `--format`, so an automated caller never has to parse output just to
  learn whether it is safe to proceed (SC-007).
- **Alternatives considered**: Reserving distinct non-zero codes per failure category
  (rejected: the spec explicitly declines to assign implementation-specific numeric status
  codes per check, FR-006; per-check detail belongs in the report body, not the exit code).

## 6. Separating process/filesystem inspection from domain aggregation

- **Decision**: Three layers, each independently testable:
  1. `internal/doctor` — the readiness domain model: `Check` interface, `CheckResult`,
     `Status`, `Report`, and the pure aggregation rule (Ready iff every result is Pass).
     Contains **no** reference to Git, Spec Kit, Claude, or Codex.
  2. `internal/checks` — concrete `Check` implementations for the nine checks, built from a
     small number of generic, reusable mechanisms (tool lookup, Git working-tree detection,
     filesystem-manifest validation, constitution content validation). This is where tool
     identity lives.
  3. `internal/system` — narrow interfaces around the two real-world effects a check can
     have: running/locating an external process (`Executor`) and reading repository files
     (`io/fs.FS`, a standard-library interface — no bespoke filesystem interface is
     introduced). Real implementations wrap `os/exec` and `os.DirFS`; tests substitute an
     in-memory fake `Executor` and `testing/fstest.MapFS`.
- **Rationale**: This is precisely the separation the approved direction and Principle IV
  (Testable by Design) require: every check's *decision logic* can be unit-tested against
  canned process/filesystem outcomes without installing or uninstalling real tools, and the
  domain aggregation rule (used by every check) is tested once, generically, without
  knowing what a "Git" or "Codex" even is.
- **Alternatives considered**: One flat package with checks calling `os/exec` and `os`
  directly (rejected: untestable without real tools on the test machine, and violates
  Principle IV); a full plugin/registry system for checks (rejected: no current
  requirement needs dynamic loading — see §7 — and it would violate Principle III).

## 7. Extensibility for future coding-agent integrations (FR-002)

- **Decision**: A single generic mechanism, `checks.ManifestIntegrationCheck`, is
  parameterized by an integration name (`"claude"`, `"codex"`, …), the repository-relative
  manifest path convention (`.specify/integrations/<name>.manifest.json`), and the
  installation-record path (`.specify/integration.json`). Adding a future coding agent is
  adding one more constructed value with a new name in the orchestration wiring
  (`internal/app`) — no change to `internal/doctor`, `internal/checks`'s types, or the
  aggregation logic.
- **Rationale**: FR-002 explicitly requires this extensibility ("Each additional
  coding-agent integration added ... in the future MUST be represented by its own
  independent check"), so building one reusable, data-driven mechanism instead of two
  copy-pasted Claude/Codex implementations is justified by a *stated requirement*, not
  speculative generality (Principle III's stated exception). It stops short of a plugin
  system: no dynamic loading, no reflection-based discovery, no configuration file
  drives which checks exist — the list of checks run is still a fixed Go slice built in
  `internal/app`, satisfying "avoid premature plugin systems."
- **Alternatives considered**: Hand-written, near-duplicate `ClaudeIntegrationCheck` and
  `CodexIntegrationCheck` types (rejected: pure duplication of identical validation logic
  differing only by name/path, which the data-driven mechanism removes without adding
  configurability that wasn't requested); a plugin system with a check registry loaded from
  a config file (rejected: no requirement calls for runtime-discoverable checks; explicitly
  out of scope per the approved direction).

## 8. Shared file-manifest validation reused by three checks

- **Decision**: The Spec Kit initialization check (#6) and the two integration checks (#8,
  #9) share one low-level helper: given a filesystem root and a manifest JSON file
  (`{"files": {"<relative path>": "<sha256>", ...}, ...}`), verify the manifest itself is
  present/parses, then verify every listed relative path exists and is non-empty. The
  initialization check applies this to `.specify/integrations/speckit.manifest.json`
  (the record of the *core* scaffold: its scripts, templates, and `.gitignore` — governed
  by FR-014, which explicitly excludes coding-agent integration files from this check).
  Each integration check applies the same helper to `.specify/integrations/<name>
  .manifest.json`, and additionally requires the name to appear in
  `.specify/integration.json`'s `installed_integrations` list (this second condition is
  what makes it "a valid installation record" per FR-016, distinguishing an integration
  check from the plain scaffold check).
- **Rationale**: Both `.specify/integrations/*.manifest.json` files already exist in this
  repository today with exactly this `files: {path: sha256}` shape (confirmed by
  inspecting `speckit.manifest.json`, `claude.manifest.json`, `codex.manifest.json`), so
  this is not a speculative schema — it is the schema Spec Kit itself already writes.
  Reading required paths from the manifest (rather than hard-coding a file list in Go
  source) keeps the check correct if the scaffold's file set changes, without a code
  change — a direct, low-cost readability/maintenance win, not new configurability that
  was never asked for.
- **Declared-structural-requirement scope (FR-016)**: "exists and is non-empty" is the
  structural requirement enforced — the manifest's recorded sha256 hashes are **not**
  used for exact-content comparison. This directly implements FR-016's explicit
  requirement that "permitted file customizations MUST NOT cause failure" and "exact
  byte-for-byte equality with originally installed content is not required." A future
  check could add stronger structural rules (e.g., required frontmatter in a skill file)
  if a future spec calls for it; none does today.
- **Alternatives considered**: Hard-coded lists of required paths per integration
  (rejected: duplicates data the repository already records once, and drifts silently if
  the manifest changes); hash-equality validation (rejected: explicitly forbidden by
  FR-016's customization allowance).

## 9. Git-repository check (#5): ask Git itself, via the `Executor` abstraction

- **Decision**: Detect "inside a Git working tree" by invoking `git rev-parse
  --is-inside-work-tree` through the same `system.Executor` abstraction `ToolCheck` (#1)
  uses for `LookPath` (research.md §6). A zero exit status with trimmed stdout equal to
  `true` is Pass; a non-zero exit status, a `git`-not-found error, or any other output is
  Fail.
- **Rationale**: This is Git's own authoritative membership test — it walks upward through
  parent directories exactly as every real `git` invocation does, so it correctly
  recognizes a nested subdirectory of a working tree, a linked worktree, and a submodule
  (whose `.git` is a *file* pointing elsewhere), none of which a shallow "does `.git` exist
  directly in the current directory" filesystem check can reliably distinguish. This
  directly satisfies FR-013's literal requirement — "recognized by Git as being inside a
  Git working tree" — rather than only approximating it with a filesystem heuristic. The
  check remains fully unit-testable per Principle IV: tests substitute an in-memory fake
  `Executor` (the same fake used by `ToolCheck`'s tests) to exercise the Pass path and every
  Fail path (non-zero exit, command not found, malformed/unexpected output) without any
  real Git repository, real `git` binary, or filesystem fixture.
- **Trade-off / accepted coupling**: this check's Pass path now depends on the `git` binary
  actually being invocable at runtime, so in an environment with no `git` on `PATH`, both
  `git-tool` (#1) and `git-repository` (#5) report Fail independently and for related
  reasons. This is judged correct, not duplicative: each check answers a different question
  a user needs distinguished ("is Git installed at all" vs. "is this directory inside a
  repository"), and FR-004 requires every check to run and report regardless of any other
  check's outcome. This is a *runtime* coupling only — unit tests substitute a fake
  `Executor`, so check #5's tests never depend on check #1 passing or on any real tool
  installation.
- **Alternatives considered**:
  - Checking for a `.git` entry via `io/fs.FS` rooted at the working directory (the
    approach originally selected in this section) — **rejected on architecture review**:
    it only recognizes the exact directory `ai-dev doctor` is invoked from, not any
    subdirectory of a working tree, and cannot recognize a linked worktree or submodule the
    way `git` itself does. It does not satisfy FR-013's "recognized by Git" wording; it only
    approximates it, and incorrectly for the subdirectory and worktree cases above.
  - `git rev-parse --show-toplevel` (rejected: also proves working-tree membership, but
    returns a path that a boolean check would then have to discard; `--is-inside-work-tree`'s
    bare `true`/`false` output is the more direct fit for a Pass/Fail decision and needs no
    path handling).

## 10. Constitution completeness heuristic (#7)

- **Decision**: `.specify/memory/constitution.md` passes when it (a) exists and is
  non-empty, (b) contains no unresolved bracketed template placeholder tokens matching
  `\[[A-Z][A-Z0-9_]*\]` (the exact convention used by
  `.specify/templates/constitution-template.md`, e.g. `[PROJECT_NAME]`,
  `[PRINCIPLE_1_NAME]`), and (c) contains both a `**Version**:` and a `**Ratified**:`
  marker followed by a non-blank value, matching the convention already used at the bottom
  of the real `constitution.md` in this repository.
- **Known limitation**: The placeholder regex is intentionally heuristic. Legitimate
  all-uppercase bracketed prose such as `[TODO]` or `[FAQ]` also matches and may therefore
  be interpreted as an unresolved placeholder.
- **Rationale**: This is a structural/textual check, matching FR-015 and the Assumptions
  section's explicit statement that substantive content quality is out of scope. The regex
  and markers are taken directly from the template and the current constitution file
  actually present in this repository, not invented independently.
- **Alternatives considered**: Full Markdown parsing/AST diffing against the template
  (rejected: much higher implementation cost for no additional verifiable requirement —
  FR-015 only asks for the four listed structural conditions).

## 11. Quality gates (constitution-mandated)

Every gate below is declared as required by this plan; none is deferred.

| Gate | Command | Pass condition |
|---|---|---|
| Formatting | `gofmt -l .` | Empty output (no files listed as needing reformatting) |
| Static analysis | `go vet ./...` | Exits `0` with no reported diagnostics |
| Automated tests | `go test ./...` | Exits `0`; all unit tests pass; every check's Pass and Fail path is covered via fakes (no real Git/Claude/Codex/Spec Kit installation required for tests to run or pass) |
| Build | `GOOS=darwin GOARCH=arm64 go build ./...` and `go build ./...` | Both exit `0`, producing a runnable `ai-dev` binary. On the reference Apple Silicon macOS environment these commands currently target the same platform; the explicit target documents the supported platform and does not provide cross-platform validation. |

`golangci-lint` or another third-party linter is deliberately **not** introduced: `go vet`
is stdlib-shipped and sufficient for this feature's size and risk; adding a linting
dependency without a demonstrated gap `go vet` leaves open would violate the "no
unnecessary dependency" instruction. This can be revisited in a future feature if a
concrete class of bug survives `go vet` in practice.

No database, network, or persistent-state gate applies (Storage: N/A). No
authentication-related gate applies (explicitly out of scope). Observability
(Principle VI) is not applicable: this is a one-shot local CLI, not a backend or
distributed system.

## Summary of resolved unknowns

All "NEEDS CLARIFICATION" technical-context items are resolved by the approved direction
plus the decisions above:

| Item | Resolution |
|---|---|
| Language/Version | Go (module pins the installed toolchain's minimum, `go1.27`) |
| Primary Dependencies | Standard library only |
| Storage | N/A |
| Testing | `go test` (stdlib `testing`), fakes for `Executor` and `fstest.MapFS` for filesystem |
| Target Platform | macOS, Apple Silicon (`darwin/arm64`) |
| Project Type | Single CLI project |
| Performance Goals | Interactive-speed only (no numeric SLA beyond "sub-second to a couple seconds," dominated by a handful of subprocess `LookPath`/version invocations) |
| Constraints | Read-only (no writes to the repository or environment), no network calls, no daemon/persistent state, deterministic given unchanged inputs (FR-018) |
| Scale/Scope | Single command, nine fixed checks, one invocation per run |
