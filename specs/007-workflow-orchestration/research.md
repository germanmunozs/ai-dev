# Research: Feature 007 — Guided Mode

**Scope**: guided mode only (F007-FR-001 to F007-FR-015, SC-001 to SC-007). The deferred autonomous section of the specification is not normative and is not planned here.

## R-01 Reusing 004, 005 and 003 "by reference" in code

- **Decision**: The guidance domain calls the existing domain functions in-process:
  - `sdd.Evaluate(root, featureID)`, which carries 004's recognition, state and terminality rules and 005's Required Artifacts Rule and Next-Step Table;
  - `selection.NewSelector(agent.NewRegistry(executor)).Select(role)`, which carries 003's fixed mapping, its availability determination (002 FR-002) and the FR-009 consistency check.

  It never invokes the `ai-dev` commands of those features, and adds no rule of its own for any of them.
- **Rationale**: This is how 006 reused 004 and 005: `internal/gate` calls `workflow.EvaluateTransition` and `sdd.MissingArtifacts` directly. Amendments 1–3 permit exactly this rule reuse, with identical semantics. Calling the domain functions guarantees identical semantics by construction, which makes SC-001 and SC-003 hold structurally.
- **Alternatives considered**:
  - Reimplementing the tables: rejected, because it creates a second source of truth.
  - Exporting `selection.mappedAgent`: rejected, because it changes 003's public API without need, and `Select` already exposes the mapped identity in `MappedAgentID` and `SelectedAgentID`.
  - Spawning `ai-dev workflow next` and `ai-dev agent select` as subprocesses: rejected, because it invokes the capabilities, which F007-FR-007 forbids, and starts processes, which SC-002 forbids.

## R-02 Mapping 003's selection report to availability

- **Decision**:

  | 003 outcome | Availability | Agent identity reported |
  |---|---|---|
  | `role_selected` | `available` | `SelectedAgentID` |
  | `agent_unavailable` | `unavailable` | `MappedAgentID` |
  | `selection_inconsistent` | `inconsistent` | `MappedAgentID` |

  A `role_unsupported` outcome cannot occur, because 005's roles are canonical. If it does occur, it is an internal error rendered as an output failure (exit 2), never a domain outcome.
- **Rationale**: F007-FR-004 defines exactly these three values. The 003 outcome itself is not reported, so guidance has no role-selection outcome of its own.
- **Alternatives considered**: Reporting the 003 outcome token verbatim. Rejected, because the specification forbids a role-selection outcome in guidance.

## R-03 Rendering human slots so that an unfilled paste does nothing (S-27, R7-01)

- **Decision**: Every human slot (`{instruction}`, `{approver}`, `{target-state}`) is rendered as an **unquoted name between inverted parentheses**, for example `)target-state(`. The feature identifier is rendered as one single-quoted word after `--` (R-04). The canonical tokens `{role}` and `{agent}` are rendered bare, because they contain only `[a-z0-9-]`.
- **Rationale**: a `)` that closes nothing is a syntax error in every POSIX-like shell and option set measured. The shell rejects the whole line while parsing, before any expansion, redirection or execution, so no command runs and no file is created. Interactively it reports the error at once and does not wait for continuation lines.

  A parenthesized `(target-state)`, the first choice, is **not** enough. It is a syntax error in sh, dash and bash, but zsh can read it as a glob group or glob qualifier. The independent implementation review (R7-01) found it, and it was confirmed on 2026-11-19: with `emulate sh`, `emulate ksh`, `setopt shglob`, `setopt kshglob` or `setopt nobareglobqual nonomatch`, the unfilled `run` template launched `ai-dev agent run -- claude-code '(instruction)'`.

  Measured on 2026-11-19 with a sentinel `ai-dev` on `PATH` and a file named after each slot in the working directory:
  - **Shells and options:** sh, dash, bash (default and `extglob`), and zsh 5.9 (`-f` default, `emulate sh`, `emulate ksh`, `setopt shglob`, `setopt shglob nonomatch`, `setopt kshglob`, `setopt nobareglobqual nonomatch`).
  - **Paste modes:** both with `-c` and interactively, the latter followed by another command line.

  | Slot form | Result |
  |---|---|
  | `'<target-state>'` (quoted) | Command runs: rejected |
  | `{target-state}` | Command runs: rejected |
  | `<target-state>` | Parsed as redirections, so the effect depends on which files exist, and `>` can create one: rejected |
  | `(target-state)` | No run in sh, dash, bash and default zsh, but **runs** in zsh under the options above: rejected (R7-01) |
  | `\|target-state\|` | Pipes into a command: runs: rejected |
  | `)target-state(` | No run and no file change in every shell, option set and paste mode tested; the next interactive line is not swallowed: chosen |

  A filled template, such as `--approver 'Ana' -- 'a;b $(x)' 'specified'`, passes the identifier as exactly one argument in every shell. A human who edits only the inside of a slot and leaves the parentheses still gets a syntax error, so a partial edit also fails closed.
- **Alternatives considered**:
  - A `false &&` prefix: rejected, because it fails open once the human removes it but leaves a slot unfilled.
  - A `#` comment prefix: rejected, because it hides the template and has the same failure mode.
  - Quoted placeholders: rejected for the reason found in review S-27.
  - `x)` or `(x))`: just as safe, but they read like typos and invite a "fix" back to `(x)`.
- **Residual**: shells that are not POSIX-like (fish, PowerShell) are out of scope, because native Windows is unsupported. The paste tests cover every mode above. zsh is required in CI, and dash is used where it is installed (SC-004).

## R-04 Quoting the feature identifier

- **Decision**: Render the identifier as `'…'`, with each embedded `'` written as `'\''`, always after `--`. Render no template when the identifier contains a rune of Unicode category Cc or Cf, U+2028 or U+2029 (`unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)`), and render canonical tokens bare only when they are `[a-z0-9-]` (the report is otherwise invalid).
- **Rationale**: POSIX single quotes suppress every expansion; only `'` needs escaping. `--` stops option parsing in all six commands, which use Go's `flag.FlagSet.Parse`; the independent review verified this from the code. Identifiers are already valid UTF-8 (`fs.ValidPath`).
- **Alternatives considered**:
  - Double quotes: rejected, because `$`, `` ` `` and `\` still expand.
  - Refusing every metacharacter: rejected, because legitimate names would lose their templates.

## R-05 Human-readable layout

- **Decision**:
  - **First line:** the same `ai-dev: outcome=… feature=%q …` form as 005, with `feature=` escaped by Go `%q` (004 FR-028), extended with `agent=` and `availability=` for `step_recommended`, and ending with the fixed statement.
  - **Template lines:** each template is a line `  <label>: <command>`.
  - **Optional reason:** a separate line states that `--reason` is optional for approval.
- **Rationale**: The first line keeps the pattern of 002–006. Unlike 005 FR-017, the specification does not require one line, and templates need their own lines to be copyable.
- **Alternatives considered**: One very long line. Rejected, because it is not copyable.

## R-06 Machine-readable report

- **Decision**: One JSON object per request, with `schema_version: "ai-dev.workflow-guide/v1"` (the Feature 008 pattern). Field names reuse 005's where they overlap (`feature_id`, `outcome`, `current_state`, `activity`, `role`, `missing_artifacts`). Templates are objects `{kind, label, command, human_slots}`. `command` is the exact rendered string, and `human_slots` lists the slot names that remain to be filled. See `contracts/workflow-guide.md`.
- **Rationale**: Automation can find every field without parsing text (SC-007), and the slots are explicit.
- **Alternatives considered**: Templates as argv arrays. Rejected, because the specified artifact is a shell-pasteable template; the slot list gives automation the structure.

## R-07 Proving SC-002 (no process, no write, no network)

- **Decision**:
  - **Processes:** the guidance domain receives only an `agent.Registry` built over the existing `system.Executor`. Its tests use a fake executor that records calls and fails the test on any `Run`; only `LookPath` is allowed.
  - **Writes:** a content digest of the fixture tree is taken before and after each request.
  - **Network and process APIs:** a structural boundary test forbids `os/exec`, `net`, `net/http`, `internal/system` launch types, and the `execution`, `gate`, `gatestore`, `instruction` and `gitmanager` packages.
  - **Standard input:** the command receives no stdin; a fixture reader fails the test if it is read.
- **Rationale**: Each part of SC-002 is shown either structurally or by a fixture, without relying on the environment.
- **Alternatives considered**: An OS-level network sandbox for the tests. Rejected as disproportionate (constitution III), because the code has no network-capable import.

## R-08 Command surface

- **Decision**: Route `ai-dev workflow guide` in `internal/app/app.go`, next to `next`, `state` and `transition`. Leave the `workflow` usage line and the top-level usage line unchanged. Parse `--format text|json` and exactly one positional identifier, with the same usage failures as `workflow next` (005 FR-019).
- **Rationale**: F007-FR-001 and F007-FR-013 require it. 005 and 006 set the precedent of not listing their subcommands in the `workflow` usage line.
- **Alternatives considered**: A top-level `ai-dev guide`. Rejected by D-03.
