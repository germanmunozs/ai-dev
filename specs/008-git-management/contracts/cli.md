# CLI Contract — Feature 008 Git Management

This contract is subordinate to [spec.md](../spec.md). Unknown options, invalid combinations, extra positional arguments, malformed values, and omitted mandatory values are usage errors unless the specification assigns a domain reason.

## 1. Interactive assistant

```text
ai-dev init [--project-dir PATH]
```

- Requires an interactive terminal before inspection, planning, authorization, locks, audit, or mutation.
- Displays all four strategies with no preselection.
- Uses the same inspector, planner, authorization issuer, and apply engine as non-interactive commands.
- Displays canonical plan digest and requires written confirmation bound to it.
- Resolves and displays the external plan destination and requires explicit confirmation before the external write.
- Generates a fresh authorization UUID through the injected cryptographic random source only after confirmation, displays it with the digest/scope, and passes it to the same issuance engine; it is never inferred from the plan digest.
- Confirmation is supervised authorization, not authenticated human identity.
- No TTY: exit 2, `interactive_terminal_required`, zero records/mutations.

## 2. Inspect

```text
ai-dev init inspect [--project-dir PATH] [--format text|json]
```

- Default format: `text`.
- Read-only with respect to project, repository, index, refs, config, and audit store.
- Success exits 0 even when reporting a repository kind that later plan/apply cannot support.
- Invalid usage exits 2.
- A report contains warnings/incompatibilities but no mutation authority.

## 3. Plan

```text
ai-dev init plan
  --project-dir PATH
  --strategy basic|git-flow|github-flow|trunk-based
  [--initialize-repository]
  [--initial-branch NAME]
  [--git-flow-dialect avh|git-flow-next]
  [--production-branch NAME]
  [--development-branch NAME]
  [--development-base-oid ALGORITHM:HEX]
  [--trunk-branch NAME]
  [--feature-prefix PREFIX]
  [--bugfix-prefix PREFIX]
  [--release-prefix PREFIX]
  [--hotfix-prefix PREFIX]
  [--support-prefix PREFIX]
  [--version-tag-prefix PREFIX]
  [--initial-commit]
  [--initial-commit-message-file FILE]
  [--include PATH]...
  [--author-name NAME]
  [--author-email EMAIL]
  [--committer-name NAME]
  [--committer-email EMAIL]
  [--author-date RFC3339]
  [--committer-date RFC3339]
  --output FILE
  [--format text|json]
```

Rules:

- No default strategy; `trunk` and `trunk_based` are invalid aliases.
- Literal `HEAD` is not a valid initial branch and returns `invalid_strategy`, exit 2, during planning.
- Required branch arguments depend on strategy exactly as FR-181 defines.
- Git Flow requires explicit dialect and complete required parameters for new configuration.
- `--development-base-oid` is valid only for Git Flow on an existing repository with history and an absent requested development ref. Its presence explicitly requests creation, and it must equal an accessible commit at the frozen production tip. It is immutable-intent, canonical-plan and authorization-operation scope. In that exact repository state, omission returns `strategy_conflict`, exit 1, and plans no implicit ref.
- Selecting any strategy plans `.ai-dev/config.json`; there is no `--write-config`.
- `--initial-commit` requires exact message, identity/time inputs after explicit resolution, mandatory config file, and literal approved includes.
- Plan output is an explicit external write only. It must be outside every project/worktree/Git area, exclusive-created `0600`, durable, and never overwritten.
- Success exits 0. Domain refusal exits 1. Missing branch/invalid strategy/output path errors assigned by spec exit 2.

Strategy option matrix:

| Strategy | Required | Prohibited |
|---|---|---|
| `basic` | new repo: `--initial-branch` | dialect, production/development/trunk/prefixes, development base |
| `github-flow` | new repo: primary `--initial-branch` | dialect, production/development/trunk/prefixes, development base |
| `trunk-based` | `--trunk-branch`; it is also initial for new repo | separate initial value, all Git Flow options |
| `git-flow` | production/development; new config: dialect and complete prefixes; new repo initial=production | trunk; development base except exact existing-history case |

A syntactically malformed development base, algorithm mismatch, or context-forbidden use returns `invalid_strategy`, exit 2. A well-formed OID naming a nonexistent/inaccessible commit, or one unequal to the frozen production tip, returns `strategy_conflict`, exit 1. `missing_required_branch`, exit 2, is reserved for an omitted required branch-name option, not an omitted development base. No OID is inferred. Drift after a valid plan uses c-01 (`plan_stale` or `concurrent_change`) rather than reclassification as usage or strategy error.

## 4. Apply — issue and consume

```text
ai-dev init apply
  --plan FILE
  --confirm-plan sha256:DIGEST
  --issue-authorization
  --authorization-id UUID
  --authorized-by TEXT
  [--authorization-expires-at RFC3339]
  [--confirm-initial-commit]
  [--approve-file 'PATH|MODE|sha256:BYTES_DIGEST|blob:BLOB_OID']...
  [--approve-ignored-file 'PATH|MODE|sha256:BYTES_DIGEST|blob:BLOB_OID']...
  [--format text|json]
```

- This is the only non-interactive issuance mode.
- `--authorized-by` is a declaration, not authentication.
- Initial commit requires `--confirm-initial-commit` plus one exact approval per file; the set must exactly equal the plan allowlist.
- Ignored files use only `--approve-ignored-file`.
- Insufficient issuance data: `refused/authorization_input_incomplete`, exit 1.
- Existing authorization ID: `refused/authorization_id_exists`, exit 1, no overwrite.

## 5. Apply — consume existing

```text
ai-dev init apply
  --plan FILE
  --confirm-plan sha256:DIGEST
  --authorization-id UUID
  [--format text|json]
```

- Uses the stored exact scope and approvals.
- Rejects issuance-only flags and cannot reissue, widen, or replace authority.
- The plan file is untrusted; operations are regenerated and compared canonically.
- A later invocation against `claimed` with durable `mutation_intent`, `mutation_started`, or `state_indeterminate` writes its own attempt/result audit and returns `refused/authorization_consumed`, exit 1, without planned mutation or transition of the original authorization. The original owner follows the distinct proven-CAS-failure versus unverifiable-CAS mapping in the authorization contract.

## 6. Conventional Commit validator

```text
ai-dev git commit validate (--message TEXT | --file FILE) [--format text|json]
```

- Exactly one of `--message` or `--file`.
- No Git invocation, repository, strategy, authorization, or mutation.
- Valid message: exit 0. Invalid message: exit 1. Usage/output error: exit 2.

## 7. Exit codes

| Exit | Meaning |
|---:|---|
| 0 | Successful inspect/plan; apply `applied` or `already_applied` with empty `secondary_failures[]`; valid Conventional Commit. |
| 1 | Domain refusal/failure; apply `refused`, `failed`, `partially_applied`, `state_indeterminate`; any non-empty `secondary_failures[]`; invalid Conventional Commit. |
| 2 | Invalid usage; no TTY; invalid strategy/missing required branch; plan output exists/unsafe; output-delivery failure. |

Exit 2 after a durable/mutating result does not imply zero effects.

Bounded-input reasons are fixed: excess plan JSON/structure/member/string/path/count is `plan_invalid`; excess configuration/attribute/ignore source is `attributes_incompatible`; excess authorized-file or aggregate authorized bytes is `repository_state_incompatible`; and excess commit-message bytes is `commit_message_invalid`. A captured stdout/stderr overflow uses the reason of the source/operation being evaluated. Exact-boundary values remain valid. When discovered after mutation, the causal reason is retained and status remains effect-based.

An ambient system/global/XDG `core.sharedRepository` value is reported but not replayed and does not reject by itself. A repository-local/worktree or deliberately replayed value capable of altering planned permissions/effects is `configuration_conflict`, exit 1, before the affected mutation.

## 8. Result envelopes

- Inspect JSON is `GitInspectOutput` and includes the complete redacted inspection plus warnings/incompatibilities.
- Plan JSON is `GitPlanOutput` and includes digest, external path, strategy report, ordered operations and every exact normal/ignored approval tuple.
- Plan refusal JSON is `GitPlanRejectionOutput` with `status=refused`, one closed-catalogue `reason`, `exit_code` 1 or 2, redacted `findings[]`, `warnings[]`, and `retryable=false`; it contains no partial plan or sensitive source data.
- Apply JSON is `GitApplyOutput` containing one `GitApplyResult`. Its `reason` is primary; structured `secondary_failures[]` is identical to durable audit. Secret findings contain only reversible path and category.
- JSON emits exactly one object to stdout; diagnostics go to stderr. Text is a validated rendering of the same model.

## 9. Output isolation

- JSON mode emits exactly one valid JSON object to stdout and diagnostics only to stderr.
- Text mode emits one stable result block/line suitable for a human but does not claim authenticated approval.
- Rendering validates the result before writing.
- Output failure preserves durable state and reports `output_failed` according to the specification.
- No output contains secrets, raw environment, credentials, unredacted remote userinfo, or file contents.

## 10. Authority exclusions

No CLI form authorizes push, fetch, pull, remote changes, merge, rebase, tags, branch delete/rename, checkout/switch, reset, stash, gc, amend, Git Flow finish, Workflow State mutation, Human Gate decision, PR creation, or container use.
