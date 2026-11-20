# Spike D-07: Isolation Mechanism for a Future Autonomous Mode

**Status**: research input, NOT normative and NOT ratified. It informs open decision D-07 (and D-05, D-06, D-13) of the spec's deferred autonomous section. It grants no capability: autonomous mode stays blocked until its own G-SPEC, G-CONTRACT and G-ISOLATION pass.

**Date**: 2026-11-20. **Author**: Claude Opus 5.5, under the owner's request to analyze D-07. **Method**:
- **What ran:** shell probes only, with no coding agent, against a scratch layout outside the repository.
- **What did not run:** no agent was launched, no credential was read and no network endpoint was contacted.

## Question

What boundary can stop agents in an autonomous chain from writing governing state, reaching credentials or privileged channels, and planting files that later sessions load? The boundary is enforced by the operating system (AD-06) and verified before every launch. The governing state is Workflow State Records, Human Gate evidence, specs, the trusted store and Git metadata.

## Measured: macOS Seatbelt (`sandbox-exec`)

**Environment and layout:**
- **Environment:** macOS 27.2.1, arm64.
- **Layout:**
  - a repository with `specs/f/workflow-state.json`, `spec.md` and `.git/config`;
  - a trusted store;
  - an agent workspace.
- **Profile:** `(allow default)`, then `(deny file-write*)` except under the workspace and `/dev/null`, then `(deny network*)`.

| # | Probe | Result | Catalog |
|---|---|---|---|
| T1 | Write inside the workspace | allowed | — |
| T1 | Append to the Workflow State Record / `.git/config` / trusted store | **denied** | NEG-001, NEG-003, NEG-005 |
| T1 | Delete `spec.md`; rename the record into the workspace | **denied** | NEG-001 |
| T2 | Symlink in the workspace to the record, then write through it | **denied** | AD-07 |
| T3 | Hard link in the workspace to the record | **denied** (link creation refused) | AD-07 |
| T4 | Apply a nested sandbox from inside the profile | **denied**: `sandbox_apply: Operation not permitted` | D-07 nesting |
| T5 | Write through a file descriptor opened **before** the sandbox and inherited | **allowed**: the record was modified | AD-08 |
| T6 | Write `~/.claude/...` outside the workspace | **denied** | NEG-017 |
| T6 | Write `.claude/settings.json` **inside** the workspace | **allowed** | NEG-017 |
| T7 | `ssh-add -l` against `$SSH_AUTH_SOCK` | **denied** | NEG-004 |
| T7 | HTTP to `127.0.0.1` (works outside the sandbox: 200) | **denied** | D-05 |
| T7 | Allow only `localhost:<port>`; that port, then another port | 200; **denied** | D-05 |
| T7 | Allow `api.anthropic.com:443` | **rejected by the profile language**: "host must be * or localhost" | D-05 |
| T8 | `ai-dev workflow approve --approver forged-by-agent` from inside | `persistence_failed`, exit 1; state still `draft` | NEG-002, NEG-015 |

## Findings

1. **The file-system boundary holds for paths.** Writes, deletes, renames, symlink traversal and hard-link creation into protected trees are all refused. A recursive `ai-dev workflow approve` cannot persist, so NEG-002 and NEG-015 reduce to the file-system rule, without trusting the caller's identity.
2. **Inherited descriptors bypass it (T5).** Seatbelt checks access when a file is opened, not when it is written. The controller must therefore give the agent pipes, never files, as standard input, output and error, and no other descriptor. Go's `os/exec` already marks its own descriptors close-on-exec and passes only `Stdin`, `Stdout`, `Stderr` and `ExtraFiles`, but this needs its own negative test.
3. **Agents' own sandboxes cannot nest (T4).** Codex and Claude Code each apply a Seatbelt profile on macOS, and inside ours that fails. Under a host Seatbelt boundary, each agent would have to run with its own sandbox disabled, with ours replacing it. That is exactly the 002 FR-016 question of D-06, and it makes our profile the only line of defense.
4. **Network allowlisting by host is impossible in Seatbelt (T7).** It filters only `localhost` or `*`, plus a port. D-05's provider-only egress therefore needs a controller-owned egress proxy on a localhost port. The sandbox allows only that port, and the proxy allowlists provider hosts by CONNECT/SNI. Unix sockets, including the SSH agent, are refused by `deny network*`.
5. **Agent-loaded files inside the workspace stay writable (T6).** NEG-017 needs one of two things:
   - an explicit deny list inside the workspace (`.claude/`, `.codex/`, `CLAUDE.md`, `AGENTS.md`, `.git/hooks`, editor and tool configuration);
   - a quarantine step that diffs the workspace before the reviewer or a later step loads anything.

   Which deny-list precedence SBPL gives inside an allowed subtree still has to be measured.
6. **Credentials are an open problem on macOS.** Claude Code keeps its login in the Keychain, which needs Mach lookups to `securityd`. Allowing them exposes whatever that user's Keychain grants. Denying them breaks agent authentication unless D-05 switches to per-run API keys. This was not measured.
7. **Seatbelt's future is uncertain.** `sandbox-exec` is deprecated, although Chromium, Bazel, Codex and Claude Code still rely on it. A release gate built on it carries that platform risk.

## Options for D-07

| Option | Strength | Costs and risks |
|---|---|---|
| **A. Host Seatbelt** (macOS) plus Landlock or bubblewrap (Linux) | No new dependency; fast; measured path rules hold | Two different mechanisms to prove. Agents' sandboxes cannot nest (F3), and there is no host allowlist (F4). Keychain (F6) and inherited descriptors (F2) need handling. Seatbelt is deprecated (F7). Linux userns may be restricted (for example, Ubuntu's AppArmor policy) |
| **B. Controller-owned Linux container or VM** (Podman machine on macOS, rootless containers on Linux) | One Linux boundary for both platforms. The repository is mounted read-only and the workspace is a separate writable copy, so D-13's import becomes an explicit diff. No host Keychain, SSH agent or terminal inside. Agents' Linux sandboxes may work inside (not yet measured) | New runtime dependency (constitution III) and slower start. Agent credentials must be injected per run (D-05). The container runtime socket must never reach the agent (AD-19, NEG-005) |
| **C. Dedicated OS user plus Seatbelt** | Isolates Keychain and home directory by construction | Needs administrator setup and two-user permissions on the repository; still has F3–F5 |

## Recommendation

1. **Pursue option B as the G-ISOLATION candidate.** It gives a boundary the controller can verify the same way on both platforms. It removes host credentials and channels by construction rather than by deny lists, and it turns D-13 into a reviewable diff import.
2. **Keep option A as defense in depth on macOS.** Treat it as a fallback, not as the gate.
3. **Do not use WSL2** until AD-24's own evidence exists.

## Measurements still required before G-ISOLATION

1. **Option B on Podman** (`podman-machine-default`): run every NEG test inside a container with these settings:
   - the repository mounted read-only;
   - a writable workspace;
   - no runtime socket;
   - network only through the controller's proxy.

   This needs the owner's authorization to start the machine.
2. **Linux:** Landlock ABI version and bubblewrap availability on the CI runner or a VM, and nesting of the agents' Linux sandboxes inside a container.
3. **Agents, non-interactively, with their own sandbox disabled, inside the boundary.** This needs explicit owner authorization and credentials (D-05, D-06). It has not been done.
4. **Precedence of in-workspace deny rules** (F5) and a negative test for inherited descriptors (F2).

**Decision status:** D-07 stays open. This spike recommends option B, pending measurements 1–3. It does not pass G-ISOLATION.
