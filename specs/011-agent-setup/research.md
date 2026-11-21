# Research: Agent Detection and Role Configuration

## R-01 How the configured assignment enters role selection

- **Decision:** `selection.Assignment` is a small value with three constructors:
  - `BuiltIn()`, which resolves through the existing unexported `mappedAgent`;
  - `FromProject(projectconfig.Config)`, which takes only a validated `Config`;
  - `InvalidProject()`, under which every recognized role yields `configuration_invalid`.

  `NewSelectorFor(registry, assignment)` resolves through it. `NewSelector(registry)` stays, and means `BuiltIn()`.
- **Rationale:** 003 kept the mapping unreachable so no caller could substitute an arbitrary policy. Amendment A1 permits exactly one substitute: a validated Feature 011 configuration. Accepting only a `projectconfig.Config`, which only `Parse` and `New` can build after validation, keeps that boundary: no code can hand the selector an unvalidated map.
- **Order of checks:** an unrecognized role is `role_unsupported` before the configuration matters, as today. A recognized role under `InvalidProject()` is `configuration_invalid`. Under `FromProject`, an unavailable agent is `agent_unavailable`. `selection_inconsistent` can only arise under `BuiltIn()`, because `FromProject` only holds catalog agents (F011-FR-018).
- **Report validation:** `Report.Valid()` keeps checking a `built_in` report against `mappedAgent`. For a `project` report it checks field presence and that the agent is in the catalog; it cannot know the file, and it does not need to, since the report is rendered right after it is built.
- **Alternatives considered:** an exported mapping table (rejected: reopens what 003 closed); passing `map[role.Role]agent.ID` (rejected: unvalidated input could reach the selector); falling back to the built-in mapping when the file is invalid (rejected by F011-FR-016).

## R-02 Reading and validating the configuration

- **Decision:** `projectconfig.Load(root fs.FS)`:
  1. `Lstat(".ai-dev")` through `fs.ReadLinkFS` (as `internal/workflow` does for the record). Absent: `Absent`. Symbolic link or not a directory: `Invalid`.
  2. `Lstat(".ai-dev/config.json")`. Absent: `Absent`. Symbolic link, not a regular file or larger than 64 KiB: `Invalid`.
  3. Read at most 64 KiB + 1 bytes; more is `too_large` (the file can grow between the two steps).
  4. Decode strictly: one JSON object, no trailing data, no duplicate keys at any level (checked by a token walk, because `encoding/json` keeps the last duplicate silently), exactly the fields `schema_version`, `agents` and `roles`, the right JSON types, no `null`.
  5. Validate: schema `ai-dev.config/v1`; at least one agent; no duplicate agent; every agent in the catalog; exactly the five roles; every role assigned to a chosen agent. Matching is exact and case-sensitive.

  A root that does not implement `fs.ReadLinkFS` is treated as `Invalid` (`cannot_verify_symlinks`), never as trusted.
- **Reasons:** every violation is reported, in a fixed order (contracts/config-file.md). Untrusted values (an unknown agent or role name) appear in 004's shell-inert quoted form, so no reason can break a line or fake an outcome.
- **Threats addressed:**
  - a hostile cloned repository naming an executable or arguments: impossible, the schema has no such field and every value is checked against closed sets (F011-FR-006);
  - a symbolic link pointing outside the project, or at a device: refused before reading;
  - a huge file or deeply nested JSON: bounded by the 64 KiB limit before decoding;
  - ambiguity between two readers of a duplicated key: rejected;
  - an agent known to a newer ai-dev: `unknown_agent`, never silently dropped.
- **Alternatives considered:** `DisallowUnknownFields` alone (rejected: misses duplicate keys and trailing data); searching parent directories (out of scope, spec Assumptions).

## R-03 Writing atomically

- **Decision:** `internal/configstore`, POSIX build tags like `gatestore`:
  1. `os.OpenRoot(dir)`.
  2. `Lstat(".ai-dev")`: if absent, `root.Mkdir(".ai-dev", 0o755)`; if a symbolic link or not a directory, fail.
  3. Open `.ai-dev`, check it is the same file as the path just checked (`os.SameFile`), so a swap between the check and the open is detected.
  4. `Lstat(".ai-dev/config.json")`: a symbolic link or a non-regular file fails; an existing file keeps its permission bits, a new one gets 0o644 (it is meant to be committed).
  5. Write `.ai-dev/.config.<random>.tmp` with `O_CREATE|O_EXCL`, `fsync`, close, `rename` over the file, `fsync` the directory. A failure before `rename` removes the temporary file; a failure of the directory `fsync` after `rename` is a warning, as in `gatestore`.

  Other platforms: always `persistence_failed`, with the warning that writing is supported on macOS and Linux only.
- **No lock.** The spec accepts that concurrent writers end with the last complete file (Edge Cases). The comparison that produces `configuration_unchanged` or `configuration_exists` runs in the command just before writing; a concurrent write in between is the accepted race.
- **Why not reuse `gatestore`:** its boundary tests pin it as the sole `workflow-state.json` write path, and its API is the Human Gate commit. Generalizing it would widen a security-pinned package. About 100 lines are duplicated instead, with their own fault-injection and boundary tests.
- **Boundary test:** no package except `internal/configstore` may combine a write-capable call with a reference to the configuration file name, and only `cmd/ai-dev/main.go` may import `configstore` (the same checker shape as `gatestore/boundary_test.go`).
- **Alternatives considered:** `os.WriteFile` (rejected: not atomic); writing through `os.DirFS` (impossible, read-only).

## R-04 Canonical encoding

- **Decision:** two-space indented JSON with a trailing newline, fields in the order `schema_version`, `agents`, `roles`; agents in catalog order; roles in the order specification, planning, implementation, review, remediation. Roles encode from a struct with one field per role, because `encoding/json` sorts map keys alphabetically.
- **Rationale:** byte-identical output for the same configuration (F011-FR-007, SC-006) and small, readable diffs in pull requests.

## R-05 The interactive assistant

- **Decision:** follow `ai-dev init`'s wizard: numbered choices, the literal `yes` to confirm, nothing written before confirmation. Differences:
  - both standard input and standard output must be terminals (F011-FR-010);
  - lines are read in a goroutine and the read is raced against the context, so Ctrl-C (`signal.NotifyContext` in `main`) cancels with `setup_cancelled` instead of leaving the prompt blocked;
  - when a valid configuration exists, each prompt shows the current answer and an empty answer keeps it; otherwise an empty answer is invalid;
  - an invalid answer (an unknown number or name) ends the assistant with exit 2 and writes nothing, as `init` does.
- **Input forms:** agents as a comma-separated list of numbers or identifiers; each role as one number or identifier among the chosen agents.
- **Alternatives considered:** re-prompting after an invalid answer (rejected for consistency with `init` and fewer paths to test); a full-screen terminal UI (rejected: a dependency or a lot of code for five questions).

## R-06 Detection

- **Decision:** `agent.NewRegistry(executor).List()`, unchanged. It uses `LookPath` only, so detection starts no process (F011-FR-001). Executable paths are never reported (F011-FR-014).
- **Catalog views:** `agent.Catalog()` and `agent.Known(id)` expose the fixed set read-only, so `projectconfig` validates identifiers without an executor.

## R-07 Help and completion

- **Decision:** catalog entries for `setup` (both a command and a group, like `init`), `setup show` and `setup apply`. `--agent` completes the catalog identifiers. `--role` completes the ten `ROLE=AGENT` pairs, generated from the roles and the catalog, so the static script needs no special handling for the `=` inside the value (the bash entry already rejoins it, Feature 010).
- **Alternatives considered:** completing `ROLE=` alone and then the agent (rejected: needs `nospace` handling per shell for one option).

## R-08 Report formats

- **Decision:** text output is line-oriented, like every other ai-dev report: one `ai-dev: outcome=…` line, then one indented line per agent, role, warning or reason. JSON is one object, schema `ai-dev.setup/v1`. Warnings are part of the report in both forms, so JSON mode never prints them elsewhere (F011-FR-014). Details in contracts/setup-cli.md.
- **003 and 007 reports:** `source` is appended at the end of the text line and added as a JSON field. The `ai-dev.workflow-guide/v1` schema identifier stays: the change is additive, and A2 approved it.
