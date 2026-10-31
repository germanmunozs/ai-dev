# Inspection and Plan Contract

## 1. Inspector port

Conceptual Go boundary:

```text
Inspector.Inspect(context, InspectRequest) -> GitInspection | error
```

`InspectRequest` contains only project path, output mode, and explicitly controlled tool/environment inputs. Inspection must not receive a mutation store or authorization.

Postconditions:

- Project/repository/index/ref/config/audit bytes are unchanged.
- No optional lock, hook, filter, fsmonitor, helper, pager, editor, prompt, network command, or configured executable runs.
- Physical project identity, tool identity, repository kind, HEAD/history, refs, index/worktree, configuration origins, attributes/conversions, Git Flow evidence, and filesystem capabilities are explicit.
- Secrets and remote userinfo are redacted before returning.

## 2. Planner port

```text
Planner.Plan(context, ImmutableIntent, GitInspection) -> GitInitializationPlan | GitApplyResult
Planner.Regenerate(context, ImmutableIntent, GitInspection) -> GitInitializationPlan | GitApplyResult
```

The planner is deterministic. It cannot read human identity/timestamps/message/strategy not present in `ImmutableIntent`. `Regenerate` produces the same covered canonical bytes when preconditions are unchanged.

During apply, the caller first compares fresh inspection with the authorized snapshot. An incompatibility caused by that drift is translated to `plan_stale` before `Regenerate`; the planner does not reclassify it as an initial `repository_state_incompatible`. Only a condition already present while generating/initially validating a plan uses the latter.

## 3. Plan schema and canonical form

- Schema: `ai-dev.git-init-plan/v1`.
- Encoding: UTF-8 JSON, canonicalized under RFC 8785.
- Covered fields: schema version, complete immutable intent, complete precondition snapshot, derived operations, expected postconditions, prohibited operations, partial-effect points, recovery policy.
- Excluded exactly: plan ID, generated/expiry times, inspection ID/time, audit timestamps.
- Excluded fields cannot affect authority or execution.
- Floats, duplicate/unknown members, ambiguous integers, and noncanonical values are invalid.
- Device/inode and integers outside IEEE-754 safe range are decimal strings.
- Regeneration compares canonical bytes, not decoded semantic equivalence.
- Limits are checked before allocation. Exact boundaries pass. Plan JSON/structure/member/string/path/count excess is `plan_invalid`; configuration/attribute/ignore source excess is `attributes_incompatible`; authorized-file/aggregate-byte excess is `repository_state_incompatible`; commit-message excess is `commit_message_invalid`. Captured-output excess uses the closed reason for the source or operation being evaluated. Checks occur before authorization/mutation whenever the input is available, and later discovery retains the causal reason with effect-based status.

## 4. Derived operation contract

Every mutation and mutation-supporting write is explicit. Each operation records:

- deterministic ID and order;
- operation kind and exact argv when Git is involved;
- target identities;
- paths/refs/config keys;
- complete preconditions;
- expected effects and postconditions;
- failure and partial-effect classifications;
- recovery instruction;
- logical temporary-resource roles.

No runtime apply ID, PID, token, audit time, or concrete temporary path enters the canonical operation. File-provided operations are never dispatched directly.

## 5. Safe output contract

Before exporting a plan:

1. resolve physical project and all prohibited Git/worktree/submodule areas;
2. resolve destination parent without unsafe symlinks/aliases;
3. prove destination is outside all prohibited areas;
4. reject an existing destination;
5. exclusive-create with mode `0600`;
6. write canonical complete bytes;
7. fsync file and, when supported, parent directory.

Unsafe destination returns `unsafe_output_path`; existing destination returns `output_exists`; both exit 2 and create no destination/temp file.

The destination parent must have a supported production filesystem profile. tmpfs and every non-APFS/non-ext4 profile reject; an allowed destination completes exclusive-create/file-sync/parent-sync capability checks as part of the explicit external write.

## 6. Reason precedence c-01

The classifier uses the phase where the condition is first established:

| Phase | Reason | Result before mutation |
|---|---|---|
| Plan generation or initial plan validation finds repository state intrinsically incompatible | `repository_state_incompatible` | `refused`, exit 1, zero blob/mutation |
| Apply fresh inspection/regeneration differs from authorized snapshot before first mutation | `plan_stale` | `refused`, exit 1, zero blob/mutation |
| State changes after successful immediate operation revalidation | `concurrent_change` | Before mutation: `refused`, exit 1. After mutation: retain reason and classify all effects. |

Final state similarity does not override phase precedence.

## 7. Strategy planning

- `basic`: explicit initial branch.
- `github-flow`: explicit primary/initial branch.
- `trunk-based`: explicit trunk/initial branch; no permanent auxiliary branches.
- `git-flow`: explicit production/development, initial=production for a new repository, explicit AVH or git-flow-next dialect.

Existing-history creation of a missing development ref additionally requires explicit `--development-base-oid ALGORITHM:HEX`. It must match repository object format, an accessible commit and the frozen production tip. The target-absent CAS is a distinct derived operation and its deterministic ID must be authorization-scoped; omission never causes OID inference.

For selected Git Flow with existing history and an absent development ref, omission is `strategy_conflict`/exit 1. Malformed syntax, an incompatible algorithm or forbidden context is `invalid_strategy`/exit 2. A nonexistent/inaccessible commit or mismatch with frozen production is `strategy_conflict`/exit 1. Only the exact valid production tip derives the operation; later drift follows c-01.

Detected Git Flow is informational unless Git Flow is selected/adopted. Non-Git-Flow selection never modifies `gitflow.*`. Git Flow operations use the exact profiles in `spec.md`, preserve instance `.base`, and expose every multi-key partial point.

Complete existing profiles are adopted only when every requested value is equal; those keys become verified no-ops. Empty profiles may be initialized. Partial/mixed/unrecognized or complete-but-different selected profiles are `strategy_conflict` and are not normalized.

## 8. In-process configuration, attribute, and ignore evaluation

Before Git status/diff or any initial-commit mutation, the inspector parses bounded configuration/attribute sources in process. Precedence is system, explicit/default global, root-to-leaf `.gitattributes`, then existing-repository `info/attributes`. New repositories have no local config/info source; expected post-init values are supplied from immutable intent/profile.

The parser supports approved Git matching, escapes, macros and states and returns ordered source digests, matched rules, effective values and a byte-identity proof per candidate. It invokes no Git/filter/hook/helper. Unknown grammar, include cycle/depth above 16, source above 1 MiB, aggregate sources above 16 MiB, unreadable/ambiguous input, active configured code or unproved conversion returns `attributes_incompatible`. Differential Git fixtures are test oracles only for compatible inert cases and never execute configured code.

The same bounded reader resolves effective system, global, XDG and local Git configuration without invoking Git configuration helpers or include commands. It evaluates ignore precedence in process across `core.excludesFile`, `.git/info/exclude`, and root-to-leaf `.gitignore` files, including negation, directory-only and anchored rules, recursive matching and escapes. It freezes every applicable source identity and digest plus the winning rule per candidate in the snapshot. An unreadable source, unsupported include/conditional syntax, ambiguous byte representation or mismatch with the supported inert Git semantics rejects before mutation. Ambient system/global/XDG `core.sharedRepository` is recorded but is not replayed into the private subprocess environment; only a repository-local/worktree or explicitly replayed value capable of changing planned permissions/effects is `configuration_conflict`.

Read-only Git subprocesses run only after that proof. `<state-root>` already includes the single final `git` segment and resolves to `$HOME/Library/Application Support/ai-dev/state/git` on macOS or absolute `$XDG_STATE_HOME/ai-dev/git` (fallback `$HOME/.local/state/ai-dev/git`) on Linux. The host resolves it from the real user context before building the subprocess environment; relative XDG state is invalid and real HOME/XDG are not exposed to Git. The frozen profile sets HOME to `<state-root>/environment/nonexistent-home-v1` and XDG_CONFIG_HOME to `<state-root>/environment/nonexistent-xdg-v1`, sets `GIT_CONFIG_NOSYSTEM=1` and null global/system config paths, and uses sorted exact `-c` replay of every supported semantic value required for ordinary status/ignore equivalence, including a descriptor-verified `core.excludesFile` path. The resolved root, exact reversible path bytes and profile version are snapshot/canonical-plan/digest/operation-ID inputs, and execution must match them byte-for-byte. Their private common parent is validated, the leaves are verified absent before/after every command, and the frozen toolchain must prove allowed commands do not create them; they are never per-attempt resources. Real HOME/XDG/system paths are inputs only to the in-process reader and remain frozen in the snapshot. Real-user `safe.directory` entries are not replayed or treated as proof of ownership: the inspector performs its own physical ownership check, and a controlled Git refusal remains `dubious_ownership`. If a value cannot be replayed while neutralizing executable configuration, the command is not run. Git status/diff output is compared with the in-process classification and any difference is `attributes_incompatible`. Global-only ignored files therefore retain ignored classification and require the ignored-file approval form.

## 9. Prohibited operation minimum

The plan always lists push, fetch, pull, merge, rebase, tag mutation, remote mutation, branch delete/rename, reset, checkout/switch, stash, gc, amend, automatic restore, submodule/linked-worktree mutation, Git Flow hooks, and `git flow feature finish` as prohibited.
