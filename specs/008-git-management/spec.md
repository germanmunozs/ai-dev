# Feature Specification: Git Management and Repository Initialization

**Feature Branch**: `feature/008-git-management`

**Feature ID**: `008`

**Created**: 2026-10-10

**Status**: Draft — Step 57 corrections applied; pending independent specification review

**Input**: Formalize the approved Git Management contract for supervised macOS/Linux initialization without implementation.

## Normative Basis and Precedence

This specification formalizes the recovered and integrity-verified contract chain. The applicable order is: Step 32 (F008-FR-001–137), Step 34 (replacements plus F008-FR-138–161), Step 36 (replacements plus F008-FR-162–177), Step 38 (F008-FR-178–188), Step 40 (F008-FR-189–192), and the approved Step 41 amendments N-E and P-1–P-4 incorporated here. Later explicit replacements prevail; all other guarantees remain cumulative.

Subsequent human-approved amendments are cumulative: Step 55 P-06 refines F008-FR-046; Step 55 P-07 refines the release/result obligations including F008-FR-169; Step 55-B B-01 removes the incorrect F008-FR-036 citation from `concurrent_change` while preserving F008-FR-164, and B-02 clarifies that derived envelope field `plan_digest` cannot digest itself; Step 57 decisions N-01, N-02 and N-03 refine release failures, existing-history development-base validation, and safe in-process configuration/ignore evaluation. The human approver is the project-responsible user who expressly authorized each amendment in this conversation. These entries record the decisions without asserting a signature, authenticated identity, approval timestamp, or separate durable artifact that is not available.

| Source | SHA-256 |
|---|---|
| Step 32 contract | `a93eed5bfa57841481fb7dd12e837e3c979d0dab789469ca96ba6aa209755dfa` |
| Step 34 contract | `086e70a60311e1757ced34d79dee242a9519eae4d7deec405adee56d7c7421c3` |
| Step 36 contract | `730e7388cf5af254fadd11a745ab1056867a6dbd4957f1b813ff7a1e907fef3a` |
| Step 38 errata | `d39fd7b80972ceb5a7945dde985873b2f8cb431f8e0f0b5c3faf7dbe1802f7a5` |
| Step 40 microerrata | `00bd6e6a0df83736257f29e65a63d66708edc1cf825f82c8389ab43d49dcd204` |
| Step 41 verification | `457db64a7355037ccad93876ca4392dd91fdb63ecd1dab21be963ca2fa7191f7` |
| FR-001–192 traceability matrix | `45002cbf9ae11a8669ac33b42945ab1365e27d1e4ed9a8f294a5728c5200ae32` |

The source documents are recovery evidence, not runtime dependencies. This `spec.md` contains the complete active requirement ledger. Feature 007 remains reserved for Workflow Orchestration. Feature 008 may expose only a read-only `GitStrategyReport` to it.

## Scope and Authority Boundaries

Feature 008 provides supervised Git inspection, planning, authorization and initialization on supported local macOS and Linux filesystems. It does not implement autonomous orchestration, network operations, remotes, push, merge, rebase, tags, pull requests, Git Flow finish, container use, Workflow State transitions, or Human Gate decisions. Windows plan/apply remains unsupported and fail-closed.

A supervised confirmation is not authentication resistant to another process running as the same operating-system user. Git authorization, plan integrity, declared operator identity, audit evidence and Feature 006 Human Gate authority are separate concepts.

## User Scenarios & Testing

### User Story 1 — Inspect a project safely (Priority: P1)

A developer can inspect a physical project directory and receive a complete Git/repository/strategy report without changing the project, repository, index, refs, configuration or audit store.

**Independent Test**: Run inspect against absent, normal, bare, nested, linked-worktree and submodule states under hostile Git configuration and verify a report plus zero project mutations.

**Acceptance Scenarios**:

1. **Given** a project without Git, **When** the developer runs `ai-dev init inspect`, **Then** the command reports `repository_absent`, project identity and toolchain capabilities without creating `.git`.
2. **Given** hostile hooks, filters, fsmonitor, pagers, editors, credential helpers or `GIT_*`, **When** inspect runs, **Then** none execute and the index remains byte-identical.

### User Story 2 — Build a reviewable initialization plan (Priority: P1)

A developer explicitly selects Basic Git, Git Flow, GitHub Flow or Trunk-Based Development and exports a deterministic, canonical plan outside the project without a default strategy or project mutation.

**Independent Test**: Generate plans for all four strategies, regenerate them from frozen intent and verify canonical equality, explicit branches, complete operations/effects and safe external output.

**Acceptance Scenarios**:

1. **Given** no selected strategy, **When** planning is requested, **Then** planning refuses rather than inferring or preselecting one.
2. **Given** an output path inside the project or an alias to it, **When** planning is requested, **Then** it returns reason `unsafe_output_path` and writes nothing.

### User Story 3 — Apply an explicitly authorized plan (Priority: P1)

A developer can explicitly issue or consume a durable Git authorization bound to one canonical plan, one physical project identity and an exact mutation scope; every attempt is audited before any business result.

**Independent Test**: Exercise issuance, consumption, expiry, mismatch, repeat application, concurrent claims and crash recovery while verifying the authorization state machine and append-only attempt records.

**Acceptance Scenarios**:

1. **Given** complete supervised authorization inputs, **When** apply uses `--issue-authorization`, **Then** an exclusive durable `issued` record exists before any project mutation.
2. **Given** two concurrent apply processes for one authorization, **When** both attempt to claim it, **Then** exactly one may progress and the loser returns an audited non-mutating result.

### User Story 4 — Initialize a new repository deterministically (Priority: P1)

A developer may authorize creation of a new SHA-1, files-ref repository with an explicit initial branch, no inherited template and a frozen Git toolchain.

**Independent Test**: Apply the exact authorized init operation under hostile `init.templateDir`, `init.defaultBranch`, `init.defaultRefFormat`, shims and environment selectors and verify the resulting repository format.

**Acceptance Scenarios**:

1. **Given** a compatible frozen toolchain, **When** initialization is authorized, **Then** the exact argv in F008-FR-190 is used.
2. **Given** a missing required init capability, **When** planning occurs, **Then** it refuses with reason `git_capability_missing` before mutation.

### User Story 5 — Create an optional safe initial commit (Priority: P1)

A developer may separately authorize an initial commit whose exact files, modes, bytes, blob digests, identities, timestamps and message are frozen; the index ends coherent with the committed tree.

**Independent Test**: Authorize a literal allowlist, complete global preflight before the first mutation, build the commit without filters/hooks/signing and verify CAS, index and status postconditions.

**Acceptance Scenarios**:

1. **Given** one unapproved, changed, ignored, suspicious or transformation-incompatible file, **When** apply validates the commit, **Then** it rejects before any blob and, where applicable, before any mutation.
2. **Given** CAS succeeds but controlled index installation fails, **When** apply closes, **Then** the commit remains and the durable result is `partially_applied` without rollback.

### User Story 6 — Configure or report Git strategy (Priority: P2)

A developer can record one explicit strategy in `.ai-dev/config.json`; Git Flow additionally supports explicit AVH or git-flow-next profiles without making detected Git Flow mandatory.

**Independent Test**: Plan each strategy in repositories with complete, partial and contradictory `gitflow.*` configuration and verify non-Git-Flow selections preserve those keys.

**Acceptance Scenarios**:

1. **Given** existing valid Git Flow keys, **When** GitHub Flow is selected, **Then** the plan is valid and leaves all `gitflow.*` keys unchanged.
2. **Given** Git Flow without sufficient history and no initial commit, **When** apply succeeds, **Then** strategy configuration is recorded and reason `development_branch_pending_initial_history` is reported without creating development.

### User Story 7 — Validate Conventional Commits independently (Priority: P2)

A developer can validate a message against Conventional Commits 1.0.0 without a repository, Git invocation or selected strategy.

**Independent Test**: Validate ordinary, breaking, footer and invalid messages and verify stable text/JSON results and exit codes.

**Acceptance Scenarios**:

1. **Given** valid types containing uppercase characters, **When** validation runs, **Then** the message is accepted; ai-dev generated suggestions still use lowercase types.
2. **Given** `BREAKING CHANGE`, `BREAKING-CHANGE`, lowercase `breaking-change`, and `Refs #123`, **When** messages are validated, **Then** validity and recognized breaking-change semantics are reported separately.

### User Story 8 — Reconcile interrupted work safely (Priority: P2)

A developer can inspect an indeterminate attempt, identify verified effects and continue only through a new canonical plan and new supervised authorization, preserving immutable prior records.

**Independent Test**: Simulate a crash after claim or mutation start and verify no old authorization is reactivated, retried, repaired or silently cleaned up.

**Acceptance Scenarios**:

1. **Given** a durable mutation intent without a terminal result, **When** apply is invoked again, **Then** it reports `state_indeterminate` and requires supervised reconciliation.
2. **Given** reconciliation completed, **When** a new plan is authorized, **Then** it links to prior IDs without modifying historical records.

### Edge Cases

- Ancestor repositories, nested repositories, bare repositories, linked worktrees and submodules.
- Unborn, attached, detached and invalid HEAD; absent or conflicting history.
- Existing/stale/foreign locks, replaced `index.lock`, and concurrent ref/index/config changes.
- APFS case folding and Unicode NFC/NFD behavior; non-UTF-8 paths; `core.precomposeunicode` and `core.ignorecase`.
- Attributes from repository, info, global, system and configured files; `core.autocrlf`, encodings and external filters.
- Git shims, changed `DEVELOPER_DIR`, changed capabilities and unsupported init options.
- Ignored files, files missed by `git diff`, secrets in the last allowlisted file and metadata changes during reads.
- Audit storage unavailable before mutation or failing after mutation.
- Existing output file, symlink/alias output, unsupported filesystem, dubious ownership and Windows.

## Requirements

### Normative Functional Requirements

The 192 identifiers below are stable. “Paso” annotations identify provenance, not priority. Requirements with later explicit text already contain the applicable replacement. Step 41 amendments are integrated into F008-FR-045, 085, 181, 182, 189, 190 and 191. F008-FR-035 is harmonized by the later canonical identifier in F008-FR-187; F008-FR-169 is narrowed for `index.lock` by F008-FR-184. These are explicit later-source harmonizations, not new guarantees.

#### F008-FR-001 (Paso 32)

El sistema MUST proporcionar `ai-dev init`, `ai-dev init inspect`, `ai-dev init plan` y `ai-dev init apply`.

#### F008-FR-002 (Paso 32)

`ai-dev init` MUST ser un asistente interactivo humano supervisado.

#### F008-FR-003 (Paso 34)

Without an interactive terminal, `ai-dev init` MUST fail before performing any inspection, planning, authorization, or apply operation. It MUST emit no plan, acquire no apply lock, create no audit record, and mutate nothing.

#### F008-FR-004 (Paso 32)

El asistente MUST mostrar las cuatro estrategias sin preseleccionar ninguna.

#### F008-FR-005 (Paso 32)

El usuario MUST seleccionar expresamente una estrategia.

#### F008-FR-006 (Paso 32)

El asistente MUST mostrar el plan completo y exigir confirmación escrita vinculada al digest.

#### F008-FR-007 (Paso 32)

La confirmación escrita MUST tratarse como autorización supervisada, no como autenticación humana.

#### F008-FR-008 (Paso 32)

Wizard, inspect, plan y apply MUST utilizar el mismo inspector, planner y apply engine.

#### F008-FR-009 (Paso 32)

Una Git Authorization MUST ser independiente de Feature 006.

#### F008-FR-010 (Paso 34)

Coding-agent outcomes, role-selection outcomes, workflow recommendations, transition evaluations, Human Gate decisions, exit codes, artifact presence, and any other automated signal MUST NOT constitute or imply Git authorization.

#### F008-FR-011 (Paso 32)

Inspect MUST ser read-only respecto del proyecto, repositorio, índice, refs, configuración y audit store.

#### F008-FR-012 (Paso 32)

Todas las invocaciones Git, incluidas inspect y plan, MUST usar un entorno controlado.

#### F008-FR-013 (Paso 32)

Inspect y plan MUST fijar `GIT_OPTIONAL_LOCKS=0`.

#### F008-FR-014 (Paso 32)

Inspect y plan MUST impedir fsmonitor, untracked-cache mutations, hooks, filtros, editores, pagers, credential helpers y comandos externos definidos por configuración.

#### F008-FR-015 (Paso 36)

`ai-dev init inspect` and `ai-dev init plan` MUST NOT refresh, rewrite, create, lock, or otherwise mutate the repository index. Read-only Git invocations used by those commands MUST set `GIT_OPTIONAL_LOCKS=0` and MUST reject any execution mode that cannot preserve this guarantee.

#### F008-FR-016 (Paso 32)

Variables `GIT_*` heredadas MUST eliminarse; solo pueden reintroducirse valores controlados y documentados.

#### F008-FR-017 (Paso 32)

El inspector MUST identificar el binario Git mediante path físico, filesystem identity, SHA-256, versión y capabilities.

#### F008-FR-018 (Paso 32)

Apply MUST rechazar si cambia la identidad o capability set del binario.

#### F008-FR-019 (Paso 32)

Dubious ownership MUST reportarse y MUST NOT corregirse mediante `safe.directory`.

#### F008-FR-020 (Paso 32)

El inspector MUST resolver el root físico sin seguir componentes ambiguos o symlinks no autorizados.

#### F008-FR-021 (Paso 32)

La identidad inicial del proyecto MUST incluir exclusivamente el path físico canónico y los valores device e inode del directorio físico abierto, representados según F008-FR-150, F008-FR-171 y F008-FR-186. Su digest MUST usar la fórmula normativa de ProjectIdentity; MUST NOT incorporar mtime, ctime, tamaño, entry count, estado Git, configuración ni contenido.

#### F008-FR-022 (Paso 32)

El inspector MUST distinguir repositorio ausente, normal, bare, linked worktree, submodule, nested, ancestor, inaccesible y ambiguo.

Inspect MUST report an inaccessible or ambiguous repository without mutation. Plan and apply MUST reject either state with `GitApplyResult.status=refused`, `GitApplyResult.reason=repository_state_incompatible`, and exit `1`; they MUST NOT treat it as repository absence, silently select a repository, or initialize over it. This reason concerns an unverifiable repository state, not a recognized but unsupported repository kind under F008-FR-023.

#### F008-FR-023 (Paso 32)

Linked worktrees y submódulos MUST poder inspeccionarse, pero plan/apply MUST rechazarlos.

#### F008-FR-024 (Paso 32)

Los conjuntos de paths MUST obtenerse y compararse sin parsing ambiguo, usando NUL delimiters donde Git lo permita.

#### F008-FR-025 (Paso 32)

Inspect MUST enumerar staged, unstaged, untracked, ignored, conflicted y cambios de modo/tipo.

#### F008-FR-026 (Paso 32)

Archivos ignored MUST inspeccionarse explícitamente cuando aparezcan en una allowlist; no basta `git diff`. Ignore status MUST be evaluated in process from bounded, descriptor-verified system, global, XDG, repository and per-directory sources, including applicable `core.excludesFile`, `.git/info/exclude`, root-to-leaf `.gitignore` files, precedence, negation, directory-relative matching and escapes. Source identities and digests MUST enter the precondition snapshot. Unsupported, unreadable, ambiguous or non-equivalent semantics MUST reject conservatively before mutation; a Git subprocess MUST NOT be allowed to execute configuration in order to decide ignore status.

#### F008-FR-027 (Paso 32)

Remotos y configuración MUST redactar credenciales, tokens y userinfo.

#### F008-FR-028 (Paso 32)

La mera presencia de `.gitattributes` MUST NOT causar rechazo.

#### F008-FR-029 (Paso 32)

El inspector MUST determinar, sin ejecutar filtros, los atributos efectivos relevantes para cada archivo candidato.

#### F008-FR-030 (Paso 34)

The inspector MUST evaluate every transformation that may cause the authorized working-tree bytes, stored blob bytes, prepared index, or ordinary Git status to disagree. This includes repository attributes and the effective values, from every Git configuration scope, of `core.autocrlf`, `core.eol`, `core.safecrlf`, `core.checkRoundtripEncoding`, `core.attributesFile`, `filter.*`, `working-tree-encoding`, `ident`, `text`, `eol`, `crlf`, and any other configured conversion mechanism.

#### F008-FR-031 (Paso 32)

Un atributo explícitamente neutral, como `-filter` o `-text`, MAY considerarse compatible.

#### F008-FR-032 (Paso 32)

Si una conversión efectiva no puede demostrarse byte-identidad sin ejecutar código configurado, inspect MUST advertir y plan/apply MUST rechazar.

#### F008-FR-033 (Paso 32)

El sistema MUST NOT ejecutar clean/smudge filters, textconv, external diff, merge drivers o filter-process.

#### F008-FR-034 (Paso 32)

Configuraciones externas de atributos no legibles o ambiguas MUST impedir plan/apply.

#### F008-FR-035 (Paso 32)

Las únicas estrategias reconocidas MUST ser `basic`, `git-flow`, `github-flow` y `trunk-based`. El alias histórico `trunk` MUST ser rechazado conforme a F008-FR-187.

#### F008-FR-036 (Paso 34)

The system MUST NOT define, preselect, recommend as mandatory, or infer a default Git strategy.

#### F008-FR-037 (Paso 34)

The system MUST NOT infer Basic Git, GitHub Flow, or Trunk-Based Development solely from branch names, remote names, or repository history.

#### F008-FR-038 (Paso 32)

Git Flow existente MUST reconocerse mediante perfiles AVH y git-flow-next.

#### F008-FR-039 (Paso 36)

Partial, mixed, contradictory, or unrecognized `gitflow.*` configuration MUST produce `strategy_conflict` only when the user explicitly selects or adopts Git Flow. When Git basic, GitHub Flow, or Trunk-Based Development is selected, existing `gitflow.*` keys MUST be reported as informational, MUST remain unmodified, and MUST NOT by themselves invalidate the selected strategy.

#### F008-FR-040 (Paso 32)

Para una configuración nueva, el dialecto AVH o git-flow-next MUST suministrarse explícitamente.

#### F008-FR-041 (Paso 36)

When Git Flow is selected, the initialization plan MUST enumerate every `gitflow.*` key and exact value that would be written. Apply MUST write those keys in the planned order, MUST revalidate each key immediately before writing it, and MUST stop after the first failed write. The report MUST identify every key successfully written before the failure.

#### F008-FR-042 (Paso 32)

En un repositorio nuevo Git Flow, initial branch MUST coincidir con production branch.

#### F008-FR-043 (Paso 32)

Con commit inicial exitoso, development branch MAY crearse mediante un ref CAS explícitamente planificado, apuntando al commit inicial.

#### F008-FR-044 (Paso 32)

Sin commit inicial, se MUST registrar la estrategia Git Flow, pero no crear development branch.

#### F008-FR-045 (Paso 32)

El reporte anterior MUST declarar el reason canónico `development_branch_pending_initial_history`. El nombre histórico `development_branch_pending_initial_commit` queda sustituido sin cambiar la condición.

#### F008-FR-046 (Paso 32)

En repositorios existentes, crear una development branch ausente requiere base OID, operación y autorización explícitas. `ai-dev init plan` MUST accept the explicit option `--development-base-oid ALGORITHM:HEX` only with `--strategy git-flow`, existing reachable history, and an absent requested development ref. Presence of the option explicitly requests that creation; absence MUST NOT permit the planner to infer an OID from production.

The supplied algorithm MUST equal the existing repository object format, the OID MUST resolve to an accessible commit, and it MUST equal the production-ref tip frozen by the same inspection. The immutable intent and precondition snapshot MUST carry the supplied base OID, object format, production ref and frozen production-tip OID. The derived operation MUST carry the target development ref, `expected-old=absent`, exact base OID, object format, production ref/tip, exact CAS argv, postconditions and deterministic `operation_id`; all are canonical-plan and `plan_digest` covered.

The `GitAuthorization` MUST explicitly include that `operation_id`. Apply MUST revalidate the object, production tip and target-ref absence immediately before the CAS. Initial incompatibility, pre-mutation snapshot drift and post-revalidation concurrency retain the c-01 precedence `repository_state_incompatible` → `plan_stale` → `concurrent_change`.

For an existing repository with history, selected Git Flow and an absent requested development ref, omission of `--development-base-oid` MUST return `strategy_conflict`, exit `1`, and MUST NOT infer or plan a development ref. Syntactically malformed `ALGORITHM:HEX`, an object algorithm incompatible with the repository, or use in a forbidden CLI context MUST be an input error with `invalid_strategy`, exit `2`. A well-formed OID whose commit is nonexistent or inaccessible, or whose value differs from the frozen production tip, MUST return `strategy_conflict`, exit `1`. Only a well-formed, accessible OID equal to that frozen tip may produce the explicit target-absent CAS operation. A later change from a valid authorized plan is classified only by c-01, never retroactively as input error or strategy incompatibility.

#### F008-FR-047 (Paso 32)

git-flow-next MUST reconocer y preservar claves `.base` asociadas a topic branches, sin confundirlas con las ramas base de la estrategia.

#### F008-FR-048 (Paso 32)

Feature 008 MUST NOT configurar `core.hooksPath`, instalar hooks o definir hooks Git Flow.

#### F008-FR-049 (Paso 32)

Feature 008 MUST NOT ejecutar `git flow`.

#### F008-FR-050 (Paso 32)

Un archivo entregado mediante `--plan` MUST tratarse como input no confiable.

#### F008-FR-051 (Paso 32)

El parser MUST aplicar límites antes de asignar o retener contenido: plan JSON 8 MiB, nesting 32, 16,384 members/items, 1 MiB per string, 4,096 authorized files, 8,192 derived operations, 16,384 postconditions in total across the plan-level list and every operation's expected postconditions, 4,096 bytes per path subject also to platform `NAME_MAX`, 64 MiB per authorized file, 512 MiB aggregate authorized bytes, 1 MiB commit-message bytes, 1 MiB per configuration/attribute/ignore source, 16 MiB aggregate such sources, and 4 MiB for each captured stdout or stderr stream. Reaching a limit exactly is valid. Exceeding plan JSON, structural, member, string, authorized-file-count, operation-count, postcondition-count or path limits MUST reject as `plan_invalid`, exit `1`. Exceeding a configuration, attribute or ignore source limit MUST reject as `attributes_incompatible`; an authorized-file or aggregate-authorized-byte limit MUST reject as `repository_state_incompatible`; and a commit-message limit MUST reject as `commit_message_invalid`, all before authorization issuance or mutation. Captured stdout/stderr overflow MUST use the closed-catalogue reason governing the source or operation being evaluated and MUST NOT be substituted with a generic reason. Implementations MUST stream or bounded-spool content and MUST NOT multiply these limits through unbounded copies. If a limit is first exceeded after mutation has begun, the causal reason remains the one just specified while status follows F008-FR-166/F008-FR-189.

#### F008-FR-052 (Paso 32)

Campos desconocidos, tipos inválidos, duplicados o representaciones no canónicas MUST rechazarse.

#### F008-FR-053 (Paso 32)

`ordered_operations` del archivo MUST NOT despacharse directamente.

#### F008-FR-054 (Paso 34)

Apply MUST extract only a schema-valid immutable intent. The intent MUST carry every frozen value required to reproduce the plan, including identities, timestamps, offsets, exact message bytes, object format, branch names, Git Flow dialect and keys, strategy configuration, and the complete per-file allowlist.

#### F008-FR-055 (Paso 32)

Apply MUST efectuar una inspección fresca y regenerar localmente el plan mediante el planner legítimo.

#### F008-FR-056 (Paso 34)

Apply MUST regenerate the plan from the validated immutable intent and a fresh precondition inspection. It MUST compare the canonical covered fields defined by this contract. The canonical form MUST exclude exactly `plan_id`, `generated_at`, `expires_at`, `inspection_id`, `inspected_at`, and audit timestamps. `plan_digest` is a derived field of the plan envelope, is not a plan field in that canonical form, and MUST NOT participate in the calculation of its own digest. Every other plan field MUST be digest-covered.

#### F008-FR-057 (Paso 32)

Operaciones, config keys, refs o paths que el planner legítimo no pueda producir MUST rechazarse.

#### F008-FR-058 (Paso 32)

Cualquier drift MUST producir `plan_stale` antes de la primera mutación.

For precedence purposes, this requirement applies during `apply` when the fresh inspection or canonical-plan regeneration differs from the authorized precondition snapshot before the first mutation. It MUST then return `GitApplyResult.status=refused`, `GitApplyResult.reason=plan_stale`, and exit `1`, with no blob or planned mutation. A repository condition already incompatible when a plan is generated or initially validated is governed instead by F008-FR-112 and `repository_state_incompatible`; a change detected only after an immediately successful operation-level revalidation is governed by F008-FR-164 and `concurrent_change`.

#### F008-FR-059 (Paso 32)

Los registros operativos MUST almacenarse fuera del repositorio en el user state directory de `ai-dev`.

#### F008-FR-060 (Paso 32)

En macOS, el root lógico MUST derivarse del user application state directory; en Linux, de `XDG_STATE_HOME` o su fallback estándar.

#### F008-FR-061 (Paso 32)

Directorios de auditoría MUST usar permisos equivalentes a `0700`; registros, `0600`.

#### F008-FR-062 (Paso 32)

Antes de la primera mutación MUST escribirse un intent record con creación exclusiva, fsync del archivo y fsync del directorio.

#### F008-FR-063 (Paso 32)

Si el intent record no puede preservarse durablemente, apply MUST rechazar sin mutar.

#### F008-FR-064 (Paso 32)

Antes de cada operación mutante MUST escribirse y sincronizarse un operation-started record.

#### F008-FR-065 (Paso 32)

Después de cada operación MUST escribirse un completed, failed o indeterminate record.

#### F008-FR-066 (Paso 32)

Si el registro posterior falla después de una mutación, el sistema MUST detenerse y reportar `GitApplyResult.status=state_indeterminate` con `GitApplyResult.reason=audit_failed_after_mutation`; no existe un estado durable de autorización llamado `audit_failed_after_mutation`.

#### F008-FR-067 (Paso 32)

Los registros MUST excluir contenidos de archivos, mensajes que contengan secretos, credenciales, entorno y URLs sin redacción.

#### F008-FR-068 (Paso 32)

Los registros MAY contener hashes, paths autorizados, refs, outcomes y metadatos no sensibles.

#### F008-FR-069 (Paso 32)

Los registros son evidencia operativa, no prueba criptográfica de identidad ni aprobación constitucional.

#### F008-FR-070 (Paso 32)

El lock MUST residir fuera del repositorio y derivarse de device/inode del root físico, no solo de su path textual.

#### F008-FR-071 (Paso 32)

Locks activos MUST causar rechazo; archivos de lock existentes sin lock activo MUST poder reutilizarse, pero no eliminarse arbitrariamente.

#### F008-FR-072 (Paso 32)

La estrategia MUST persistirse en `.ai-dev/config.json`.

#### F008-FR-073 (Paso 32)

El archivo MUST contener schema version, project ID, estrategia y parámetros normalizados.

#### F008-FR-074 (Paso 32)

El archivo MUST ser versionable y no contener credenciales.

#### F008-FR-075 (Paso 32)

Una configuración existente MUST validarse sin seguir symlinks.

#### F008-FR-076 (Paso 32)

La escritura MUST usar temp exclusivo en el mismo directorio, fsync, expected-content digest, rename atómico y fsync del directorio.

#### F008-FR-077 (Paso 32)

Filesystems que no garanticen las primitivas requeridas MUST impedir apply.

#### F008-FR-078 (Paso 32)

Una configuración divergente MUST producir `configuration_conflict`, sin overwrite.

#### F008-FR-079 (Paso 32)

Repetir una configuración ya equivalente MUST ser no-op verificable.

#### F008-FR-080 (Paso 32)

El commit inicial MUST ser opcional.

#### F008-FR-081 (Paso 32)

Debe existir una autorización específica para el commit y una autorización individual para cada archivo.

#### F008-FR-082 (Paso 32)

`.ai-dev/config.json` MUST formar parte del commit inicial.

#### F008-FR-083 (Paso 32)

Cada archivo autorizado MUST registrar path, modo, SHA-256 de bytes, blob OID, ignored status y aprobación individual.

#### F008-FR-084 (Paso 32)

Un booleano global `initial_commit_authorized` MUST NOT ser suficiente.

#### F008-FR-085 (Paso 36)

Any authorized initial-commit file that produces a high-risk secret finding MUST be rejected before blob creation or repository mutation. The rejection MUST identify the affected path and finding category without recording the detected secret or sensitive content in the audit record.

“Aplicable” en este requisito significa que el plan contiene el grupo opcional de commit inicial. En ese caso, el preflight global de todos los archivos autorizados, incluido el contenido previsto de `.ai-dev/config.json` generado en memoria, MUST completarse antes de la primera mutación del intento. En un proyecto aún no inicializado, la inspección MUST operar sobre el root físico y los bytes previstos sin crear `.git`, configuración ni objetos; si una garantía solo pudiera evaluarse después de mutar, plan/apply MUST fallar de forma segura.

#### F008-FR-086 (Paso 32)

Allowlist MUST ser literal; globs, directorios, `git add .`, `git add -A` y `git add -u` están prohibidos.

#### F008-FR-087 (Paso 32)

Paths absolutos, traversal, componentes symlink, controles, NUL y paths fuera del root MUST rechazarse.

#### F008-FR-088 (Paso 32)

Se MUST detectar colisiones por case-folding y normalización Unicode.

#### F008-FR-089 (Paso 32)

Solo archivos regulares MUST ser admisibles.

#### F008-FR-090 (Paso 32)

Archivos ignored MUST mostrarse expresamente y requerir confirmación individual reforzada.

#### F008-FR-091 (Paso 32)

Un escaneo integrado MUST detectar secretos de alto riesgo conocidos.

#### F008-FR-092 (Paso 32)

Toda sospecha MUST impedir el commit hasta que un plan posterior ya no la contenga.

#### F008-FR-093 (Paso 32)

El sistema MUST declarar que el scanner no garantiza detección perfecta.

#### F008-FR-094 (Paso 32)

Repositorios nuevos MUST inicializarse explícitamente con object format SHA-1.

#### F008-FR-095 (Paso 32)

Repositorios existentes MUST conservar su object format.

#### F008-FR-096 (Paso 32)

Nombre y email de author y committer MUST congelarse en el plan.

#### F008-FR-097 (Paso 32)

El planner MAY leer identidad Git existente, pero MUST mostrar origen y valores y permitir edición explícita.

#### F008-FR-098 (Paso 32)

Nombres MUST ser UTF-8 no vacíos y no contener NUL, CR/LF, controles ni `<`/`>`.

#### F008-FR-099 (Paso 32)

Emails MUST cumplir un formato seguro sin controles, espacios, CR/LF ni delimitadores de cabecera.

#### F008-FR-100 (Paso 32)

Author date y committer date, incluidos offset y zona efectiva, MUST congelarse en el plan.

#### F008-FR-101 (Paso 32)

Solo timestamps operativos de auditoría MAY capturarse durante apply; no afectan el commit OID.

#### F008-FR-102 (Paso 32)

El mensaje MUST ser UTF-8 válido, sin NUL ni CR, con LF como único salto de línea.

#### F008-FR-103 (Paso 32)

Los bytes almacenados del mensaje MUST ser exactamente los bytes mostrados y aprobados.

#### F008-FR-104 (Paso 34)

The approved commit message MUST be valid UTF-8, contain no NUL or CR, use LF for every line ending, and end with exactly one LF. The displayed, approved, digested, supplied to Git, and stored message bytes MUST be identical.

#### F008-FR-105 (Paso 32)

El mensaje MUST pasar Conventional Commits 1.0.0.

#### F008-FR-106 (Paso 32)

Tipos con mayúsculas MUST aceptarse si cumplen la gramática.

#### F008-FR-107 (Paso 32)

Mensajes generados o sugeridos por `ai-dev` MUST usar tipos en minúsculas.

#### F008-FR-108 (Paso 32)

Firma de commit MUST deshabilitarse; el commit no puede contener `gpgsig` implícito.

#### F008-FR-109 (Paso 32)

Hooks, templates, filtros y editores MUST permanecer fuera del flujo.

#### F008-FR-110 (Paso 32)

Los blobs MUST construirse desde bytes abiertos de forma segura y mediante hashing sin filtros.

#### F008-FR-111 (Paso 32)

El tree MUST contener exactamente los paths, modos y blob OIDs autorizados.

#### F008-FR-112 (Paso 32)

Antes de construir objetos se MUST verificar que HEAD sea unborn, el ref inicial no exista, no haya historia, el índice esté vacío y no existan conflictos.

When the plan contains the optional initial-commit group, existing reachable history, an existing initial ref, a non-empty index, index conflicts, or any HEAD state other than `unborn` MUST make that group inapplicable and MUST reject before blob creation or any project/repository mutation with `GitApplyResult.status=refused`, `GitApplyResult.reason=repository_state_incompatible`, and exit `1`. Existing history does not by itself prohibit an otherwise valid plan that omits the initial-commit group.

`repository_state_incompatible` has precedence only when the incompatible condition already exists while generating or initially validating the plan. Once an authorized compatible snapshot exists, a mismatch found by the fresh pre-mutation apply validation is `plan_stale` under F008-FR-058 rather than a reclassification of the repository as initially incompatible.

#### F008-FR-113 (Paso 32)

El ref inicial MUST actualizarse mediante compare-and-swap desde ref inexistente al commit autorizado.

#### F008-FR-114 (Paso 32)

Un CAS fallido MUST dejar ref e índice sin cambios y reportar los posibles objetos inalcanzables.

#### F008-FR-115 (Paso 32)

Después de CAS exitoso, el índice normal MUST sincronizarse exactamente con el tree autorizado.

#### F008-FR-116 (Paso 32)

La sincronización del índice MUST ser una operación explícita, planificada y autorizada.

#### F008-FR-117 (Paso 32)

Antes de instalar el índice se MUST adquirir `index.lock` de forma exclusiva y verificar que el índice aún coincide con el estado vacío autorizado.

#### F008-FR-118 (Paso 32)

Un índice preparado MUST construirse sin filtros y verificarse contra el tree autorizado.

#### F008-FR-119 (Paso 32)

La instalación MUST usar el protocolo de lock/rename compatible con Git y MUST NOT sobrescribir cambios concurrentes.

#### F008-FR-120 (Paso 32)

Si CAS tuvo éxito pero el índice falla, el commit permanece aplicado y el resultado MUST ser `partially_applied`.

#### F008-FR-121 (Paso 36)

After the initial-reference compare-and-swap succeeds, apply MUST NOT automatically roll back, reset, amend, delete, or replace the created reference or commit if index installation or final verification fails. The result MUST be `partially_applied` and MUST require supervised reconciliation.

#### F008-FR-122 (Paso 32)

Tras éxito se MUST verificar HEAD, ref, commit, parent count, tree, índice y status.

#### F008-FR-123 (Paso 32)

`git diff --cached HEAD` MUST estar vacío para los paths autorizados.

#### F008-FR-124 (Paso 34)

Authorized paths MUST NOT appear staged, unstaged, type-changed, or mode-changed after successful commit and index synchronization.

#### F008-FR-125 (Paso 32)

Archivos preexistentes no autorizados MUST conservar su estado previo, incluidos untracked.

#### F008-FR-126 (Paso 32)

Si commit e índice fueron aplicados pero el reporte falla, el resultado de proceso MUST indicar output failure sin revertir efectos.

#### F008-FR-127 (Paso 32)

Una authorization ID completada y cuyas postcondiciones aún se cumplen MUST responder `already_applied`, sin mutar.

#### F008-FR-128 (Paso 32)

Una authorization ID partial, failed o indeterminate MUST NOT reintentarse automáticamente.

#### F008-FR-129 (Paso 32)

Recuperación MUST requerir inspección, plan y autorización nuevos.

#### F008-FR-130 (Paso 34)

Feature 008 MUST NOT invoke a Git subcommand, helper, hook, executable, transport, URL, or API capable of contacting a remote service as part of inspect, plan, validation, or apply.

#### F008-FR-131 (Paso 32)

Credential helpers, askpass, SSH commands y terminal prompts MUST deshabilitarse.

#### F008-FR-132 (Paso 36)

A repository whose ownership cannot be accepted without changing `safe.directory` or another user/system Git configuration MUST be reported as `dubious_ownership`. Inspect MAY report the condition, but plan and apply MUST reject it. Feature 008 MUST NOT modify `safe.directory`.

#### F008-FR-133 (Paso 32)

macOS y Linux en filesystem local compatible serán las plataformas iniciales.

#### F008-FR-134 (Paso 32)

La matriz obligatoria de aceptación MUST incluir macOS/APFS y Linux/ext4.

#### F008-FR-135 (Paso 32)

Windows MAY ejecutar inspect limitado, pero plan/apply MUST devolver `platform_unsupported` antes de mutar.

#### F008-FR-136 (Paso 36)

Plan and apply MUST fail closed on Windows and on filesystems for which the required identity, locking, durability, atomic-replacement, and compare-and-swap guarantees cannot be established. Inspect MAY report available information with an explicit unsupported-platform warning and MUST NOT claim native validation.

#### F008-FR-137 (Paso 32)

Feature 008 MUST preservar el comportamiento de Features 001–006.

#### F008-FR-138 (Paso 34)

Feature 008 MUST NOT initialize a repository when the requested project directory is already contained in an ancestor Git repository, unless a future separately specified feature defines an explicit nested-repository mode. MVP inspect reports `ancestor_repository`; plan and apply reject.

#### F008-FR-139 (Paso 34)

Git configuration includes, conditional includes, global configuration, and system configuration MAY be inspected as relevant environmental input, but MUST NOT constitute authoritative evidence that the project selected a Git strategy. Strategy evidence is limited to valid local repository configuration and `.ai-dev/config.json`.

#### F008-FR-140 (Paso 34)

Every mutating operation MUST appear in the plan with its exact type, ordered position, preconditions, affected paths, refs and configuration keys, expected postconditions, failure outcomes, partial effects, and recovery instruction.

#### F008-FR-141 (Paso 34)

Apply MUST NOT perform any mutation that is absent from `derived_operations`. Helper actions that write objects, configuration, refs, index, locks, audit records, or project files are operations for this purpose and MUST be represented.

#### F008-FR-142 (Paso 34)

Apply MUST stop before initiating the next planned mutation when any validation, audit write, precondition, operation, postcondition, or output-preparation step fails.

#### F008-FR-143 (Paso 36)

After a failed mutation, Feature 008 MUST NOT perform automatic rollback, retry, repair, garbage collection, reset, restore, amend, mutation fallback, strategy substitution, or cleanup of pre-existing repository resources. Releasing temporary resources created and exclusively owned by the current apply attempt is governed only by F008-FR-169 and is not considered rollback or repair.

#### F008-FR-144 (Paso 34)

Feature 008 MUST NOT create, modify, replace, stage specially, interpret as authorization, or delete any Workflow State Record or Human Gate evidence. Such files may only be included in a Git commit through the same explicit per-file authorization applicable to ordinary files, and committing them MUST NOT be reported as applying or approving a Human Gate.

For the initial commit MVP, the recommended policy is to exclude pre-existing workflow-state and gate-evidence files unless a later specification explicitly covers that case.

#### F008-FR-145 (Paso 34)

Feature 007 MAY consume `GitStrategyReport` as read-only information. The report MUST NOT convey Git authorization, mutation capability, reusable credentials, an apply plan, or permission to invoke Feature 008 apply.

#### F008-FR-146 (Paso 34)

Selecting Basic Git, GitHub Flow, or Trunk-Based Development MUST remain valid when a complete or partial `gitflow.*` configuration exists. Selecting a non-Git-Flow strategy MUST NOT create, modify, normalize, delete, or adopt `gitflow.*` keys. Those keys are reported as informational warnings and MUST NOT by themselves cause `strategy_conflict`.

#### F008-FR-147 (Paso 34)

Feature 008 MUST provide `ai-dev git commit validate` as a strictly read-only Conventional Commits 1.0.0 validator. It MUST NOT require a Git repository, invoke Git, inspect strategy configuration, or mutate files.

#### F008-FR-148 (Paso 34)

Effective conversion settings from every applicable configuration scope MUST be frozen in the precondition snapshot. If any effective setting may cause ordinary Git comparison to transform an authorized path differently from its approved bytes or stored blob, plan and apply MUST return `attributes_incompatible`.

#### F008-FR-149 (Paso 34)

Each `gitflow.*` write MUST be represented as a separate ordered operation. The plan MUST declare that the group is non-atomic. On the first failed write, apply MUST stop and report `partially_applied` if any earlier key was written. It MUST NOT remove or revert previously written keys automatically.

#### F008-FR-150 (Paso 34)

The project identity digest MUST be computed from the canonical physical project path plus filesystem device and inode only. It MUST NOT include mtime, ctime, entry count, directory size, Git state, or repository contents.

#### F008-FR-151 (Paso 34)

When stable device and inode identity cannot be obtained, or the filesystem does not preserve them with the required semantics, inspect MUST report `project_identity_unavailable`; plan and apply MUST reject.

#### F008-FR-152 (Paso 36)

`plan_id`, `plan_digest`, `authorization_id`, and `apply_id` MUST be distinct values with distinct purposes. `authorization_id` MUST NOT be derived solely from `plan_digest`. An authorization record MUST bind its `authorization_id` to exactly one canonical `plan_digest`, one `project_identity`, one immutable authorization scope, one issue time, and an optional expiry time.

#### F008-FR-153 (Paso 36)

An authorization MAY support validation attempts before mutation, but MUST authorize at most one mutation sequence. A refusal before the first repository or project mutation MUST NOT consume the authorization. Immediately before the first mutation, apply MUST durably transition the authorization to `mutation_started`. Once that transition succeeds, the authorization MUST NOT authorize another mutation attempt.

#### F008-FR-154 (Paso 36)

Every invocation of `ai-dev init apply` that passes minimal input size and schema limits MUST receive a new `apply_id`. Before returning any business result, including `already_applied`, `plan_stale`, `authorization_missing`, `authorization_mismatch`, `authorization_expired`, `authorization_consumed`, or `state_indeterminate`, apply MUST durably create an attempt record associated with that `apply_id`. Failure to create and durably synchronize the attempt record MUST return `audit_unavailable` and MUST prevent all repository or project mutations.

#### F008-FR-155 (Paso 34)

On macOS, the inspector MUST identify both the launcher selected by PATH and the effective Apple/Xcode Git toolchain. It MUST resolve the effective executable and `git --exec-path` under a controlled, recorded `DEVELOPER_DIR`/xcode-select context. The launcher hash alone is insufficient.

#### F008-FR-156 (Paso 36)

The planner MUST choose and freeze whether Git will be invoked through a launcher or through a directly identified executable. The plan MUST record the invocation path, invocation mode, SHA-256 where applicable, reported Git version, `git --exec-path`, required capabilities, and every environment selector that affects resolution, including `DEVELOPER_DIR` on macOS. Apply MUST use exactly the frozen invocation mode and MUST reject any mismatch as `plan_stale`. Silent fallback to another launcher, executable, developer directory, exec path, or Git implementation is prohibited.

### Requisitos añadidos

#### F008-FR-157 (Paso 34)

The prepared index MUST be constructed outside the repository using a controlled `GIT_INDEX_FILE`, fixed index format version 2, `core.splitIndex=false`, `core.fsmonitor=false`, `core.untrackedCache=false`, and settings that prevent sparse or shared index dependencies.

#### F008-FR-158 (Paso 34)

Before installation, the prepared index MUST be parsed and verified to contain exactly the authorized tree entries, use the approved version, contain no `link` extension, and reference no `sharedindex` file.

#### F008-FR-159 (Paso 34)

Non-interactive authorization issuance during apply MUST require one explicit file approval argument for every initial-commit file. The approved set MUST equal the plan allowlist exactly. Missing, duplicate, additional, mismatched, or differently classified approvals MUST reject before authorization creation or mutation. Consumption of an existing authorization MUST use its already durable exact approval set and MUST NOT accept new approval arguments that would reissue, widen or replace that scope.

#### F008-FR-160 (Paso 34)

An ignored file MUST use the distinct ignored-file approval form. A normal file approval MUST NOT authorize an ignored path.

#### F008-FR-161 (Paso 34)

`.ai-dev/config.json` MUST be present in both the initial-commit allowlist and the explicit per-file approvals. Its authorized mode MUST be `100644`.

#### F008-FR-162 (Paso 36)

GitInspection MUST report HEAD as exactly one of `unborn`, `attached`, `detached`, or `invalid`. It MUST separately report whether reachable commit history exists and, when HEAD is attached, the exact symbolic reference bytes.

An `invalid` HEAD MUST be reported by inspect without mutation and MUST cause plan/apply to reject with `GitApplyResult.status=refused`, `GitApplyResult.reason=repository_state_incompatible`, and exit `1`. `attached` or `detached` HEAD is not automatically invalid, but remains incompatible with an authorized initial-commit group under F008-FR-112.

#### F008-FR-163 (Paso 36)

Every Git process MUST be invoked using an exact argument vector without an intervening shell. Repository paths and pathspecs MUST follow `--` wherever the Git command accepts it. `GIT_LITERAL_PATHSPECS=1` MUST be set for every invocation that accepts pathspecs. User-controlled bytes MUST NOT be interpolated into shell commands.

#### F008-FR-164 (Paso 36)

Immediately before each planned mutation, apply MUST revalidate that operation’s complete precondition set against current state. This includes `git init`, each Git configuration-key write, `.ai-dev/config.json`, each reference update, authorization transition to `mutation_started`, prepared-index installation, and normal-index replacement. Any mismatch MUST stop execution before that mutation with `concurrent_change` or `plan_stale`, as applicable.

The reason is `plan_stale` when the mismatch is first observed while comparing current state with the authorized snapshot before the mutation sequence starts, as required by F008-FR-058. The reason is `concurrent_change` when the relevant precondition was successfully revalidated immediately for an operation and state then changes before that operation can complete its guarded mutation. If no mutation has started, the result MUST be `status=refused`, reason `concurrent_change`, exit `1`, with no blob or planned mutation. If any mutation has started, `concurrent_change` remains the causal reason but status MUST be selected conservatively under F008-FR-166 and F008-FR-189 from all verified or unverifiable effects.

#### F008-FR-165 (Paso 36)

`prohibited_operations` MUST contain, at minimum: push, fetch, pull, merge, rebase, tag creation or deletion, remote addition or modification, branch deletion or rename, reset, checkout or switch, stash, garbage collection, amend, automatic restore, submodule mutation, linked-worktree mutation, Git Flow hooks, and `git flow feature finish`. The planner MAY add stricter prohibitions but MUST NOT omit this minimum set.

#### F008-FR-166 (Paso 36)

Every apply attempt that performs one or more mutations MUST perform a fresh controlled inspection and verify every planned `expected_postcondition`, not only initial-commit postconditions. Before mutation, a missing, contradictory, or unverifiable postcondition MUST produce `refused`. After mutation, if every effect is verifiable and at least one planned effect exists but the complete planned result was not achieved, the status MUST be `partially_applied`. If any applied, absent, or planned effect cannot be verified, the status MUST instead be `state_indeterminate`. F008-FR-189 and P-2 control classification whenever uncertainty exists.

#### F008-FR-167 (Paso 36)

Apply MUST compare the authorization’s stored `plan_digest` with the exact digest supplied through `--confirm-plan`, and MUST compare its stored `project_identity` with the freshly verified project identity. A mismatch MUST return `authorization_mismatch`, MUST be audited, and MUST NOT consume an authorization that has not begun mutations.

#### F008-FR-168 (Paso 36)

If an authorization or earlier apply attempt contains a durable `mutation_intent` without a conclusive result, its original durable authorization state MUST remain either `claimed` with that intent or `mutation_started`, exactly as last durably verified. An authorization already in terminal `state_indeterminate` MUST remain there. None of these consumed conditions may be downgraded, released, reassigned, implicitly reauthorized, or reused for mutation. Supervised reconciliation MUST write new linked records and MUST NOT transition or overwrite the original authorization.

A subsequent `apply` invocation concerning a `claimed` authorization with durable intent, `mutation_started` without a conclusive result, or terminal `state_indeterminate` MUST receive its own `apply_id` and durable attempt/result records, perform no planned project, repository, reference, index, configuration, object, or authorization-state mutation, and return `GitApplyResult.status=refused`, `GitApplyResult.reason=authorization_consumed`, and exit `1`. The attempt audit MUST record the unchanged prior authorization state and that the prior outcome remains unresolved; those mandatory external audit writes do not constitute a retry of the authorized mutation sequence.

Unrelated future plans and authorizations remain possible. Recovery MUST require supervised reconciliation under F008-FR-180; automatic retry is prohibited.

#### F008-FR-169 (Paso 36)

An apply plan MUST include a release protocol for the apply lock, `index.lock`, configuration temporary file, and prepared-index file created by that attempt. Release MUST verify ownership using the current `apply_id`, an unguessable creation token where the resource format permits one, and the recorded filesystem identity. For `index.lock`, F008-FR-184 is controlling: ownership MUST use exclusive creation, the retained descriptor, device, inode and owning `apply_id`, and MUST NOT embed a token in the Git index. It MUST NOT delete or replace a pre-existing or foreign resource. Release MUST NOT alter refs, commits, repository configuration, project configuration, or an installed index.

Every failed release of an owned resource MUST create its own deterministic `SecondaryFailure` entry, including when it is the only failure. Entries MUST be ordered by the planned release ordinal and resource role and MUST preserve `resource_role`, `ownership_verified`, `failure_class`, `phase`, `code` and the durable audit-record digest without exposing a secret, token, descriptor, temporary path or other sensitive identifier. When no earlier primary cause exists, `reason` MUST be `resource_release_failed`; otherwise the existing primary reason MUST remain unchanged. Any non-empty `secondary_failures[]` MUST return exit `1`, including with `status=applied`. Status MUST remain determined exclusively from verified effects, and the conclusive durable result and rendered output MUST contain the same ordered entries.

#### F008-FR-170 (Paso 36)

The Git Flow production and development branch names MUST be different. For git-flow-next, neither name may be byte-equal to `feature`, `bugfix`, `release`, `hotfix`, or `support`, because those names occupy the `gitflow.branch.<name>.*` namespace used for topic definitions. Invalid names MUST be rejected during planning.

#### F008-FR-171 (Paso 36)

`device`, `inode`, and every integer whose absolute value exceeds `9007199254740991` MUST be represented in canonical-plan JSON as base-10 strings matching `0|[1-9][0-9]*`, with no sign for non-negative values, no leading zeroes, and no exponent notation. They MUST NOT be encoded as JSON numbers.

#### F008-FR-172 (Paso 36)

The canonical `plan_digest` MUST cover exactly `schema_version`, complete immutable intent, complete precondition snapshot, all derived operations, all expected postconditions, all prohibited operations, all partial-effect points and recovery policy.

Immutable intent MUST include `project_id`, strategy, dialect, branch names, object format, toolchain identity, author/committer identities and timestamps, exact commit-message bytes, configuration digest, literal allowlist, per-file path/mode/content/blob digests, ignored status, initial-commit choice and required capabilities. The frozen `project_id` MUST reproduce exactly the intended `.ai-dev/config.json` bytes.

The precondition snapshot MUST include project/repository identity, HEAD/history, refs, relevant effective configuration with origin and scope, index state, worktree identities/digests, project configuration identity, Git Flow state, attribute/conversion sources, worktree/submodule status, filesystem/lock state and toolchain selectors.

The digest MUST exclude exactly `plan_id`, `generated_at`, `expires_at`, `inspection_id`, `inspected_at` and audit timestamps. No excluded field may affect authority, operations, preconditions or postconditions.

#### F008-FR-173 (Paso 36)

Each `operation_id` MUST be derived deterministically from the operation’s zero-based ordinal and the SHA-256 of the RFC 8785 canonical JSON representation of its operation kind, exact argument vector, target identity, preconditions, expected effects and partial-effect classification. Regenerating the same canonical plan MUST produce byte-identical operation IDs.

#### F008-FR-174 (Paso 36)

Plan regeneration MUST require byte-for-byte equality of RFC 8785 canonical plan forms. Semantic similarity is insufficient. Paths MUST be represented and compared using their observed filesystem byte sequences. Feature 008 MUST NOT silently normalize, fold, or equate Unicode NFC and NFD forms.

#### F008-FR-175 (Paso 36)

Inspection MUST account for repository `.gitattributes`, `$GIT_DIR/info/attributes`, `core.attributesFile`, the default global attributes file, system attributes, `.gitignore`, `$GIT_DIR/info/exclude`, `core.excludesFile`, system/global/XDG/local configuration, `core.autocrlf`, `core.eol`, `core.safecrlf`, `core.checkRoundtripEncoding`, configured attribute filters/encodings and any other relevant transformation or ignore source.

Effective settings and ignores MUST be evaluated in process from the user’s actual frozen HOME, XDG and system scopes, with origin, identity and digest captured, while preventing configured commands, filters, helpers, hooks and fsmonitor from executing. Git subprocess profiles MUST use an explicitly documented HOME/XDG/`GIT_CONFIG_NOSYSTEM` policy and command-line overrides derived only from the safely parsed configuration; they MUST NOT silently erase configuration that changes ordinary `status` or ignored classification. If exact equivalence cannot be established without executing external code, inspect MUST warn and plan/apply MUST reject with `attributes_incompatible`.

#### F008-FR-176 (Paso 36)

A repository with `extensions.worktreeConfig=true` is incompatible with Feature 008 plan/apply in the MVP and MUST be rejected. Feature 008 MUST NOT override this extension using `-c`, environment variables, or temporary configuration. Inspect MAY report it read-only.

#### F008-FR-177 (Paso 36)

A path containing the byte `|` MUST be rejected during planning because it cannot be represented unambiguously by the approved CLI authorization format. The rejection MUST occur before plan authorization and MUST NOT be deferred to the apply parser.

#### F008-FR-178 (Paso 38)

Non-interactive `ai-dev init apply` MAY issue and immediately consume a new `GitAuthorization` only when the caller explicitly supplies `--issue-authorization`. Issuance MUST use exclusive creation and MUST bind the authorization identifier, exact `--confirm-plan` digest, freshly verified project identity, complete authorization scope, declared operator identity, individual file approvals, ignored-file approvals, issue time and optional expiry time. The initial durable state MUST be `issued`. The authorization record MUST be durably synchronized before any project or repository mutation.

El alcance exacto incluye `plan_digest`, `project_identity`, todos los `operation_id`, estrategia y dialecto, ramas/refs, configuración, decisión de commit inicial, aprobaciones por archivo y caducidad opcional.

`--authorized-by` es una declaración supervisada, no autenticación criptográfica ni prueba de identidad humana.

Sin `--issue-authorization`, apply requiere una autorización existente y no puede sintetizarla. Datos insuficientes producen `authorization_input_incomplete`. Si el ID ya existe, produce `authorization_id_exists`, sin sobrescribir, ampliar, fusionar, reemitir ni consumir silenciosamente el registro.

#### F008-FR-179 (Paso 38)

Every authorization state transition MUST be a durable compare-and-swap over the authorization identifier, current state, state version and owning `apply_id`. A transition MUST fail without overwrite if any expected value differs.

```text
issued
  └── claimed
        ├── issued                 clean release before mutation
        ├── claimed                verified orphan-claim reassignment
        └── mutation_started
              ├── applied
              ├── failed
              ├── partially_applied
              └── state_indeterminate
```

Reglas:

1. `issued → claimed`: bajo apply lock, tras relectura y CAS durable.
2. `claimed → issued`: solo por rechazo limpio, mismo owner, sin intención ni mutación.
3. `claimed(old) → claimed(new)`: solo claim huérfano probado, lock previo libre, sin `mutation_started`, inspección fresca sin efectos y auditoría append-only.
4. `claimed → mutation_started`: durable inmediatamente antes de la primera mutación.
5. `mutation_started → applied | failed | partially_applied | state_indeterminate`: estados terminales; `failed` solo es válido bajo las condiciones globales de F008-FR-189.

Está prohibido volver desde un estado terminal, saltar de `issued` a `mutation_started`, transferir `mutation_started`, sobrescribir terminales o reducir la versión. Fallos al persistir antes de mutar detienen la operación; un fallo terminal posterior deja `mutation_started` y se clasifica como indeterminado.

For the first mutation, the owner MUST durably publish `mutation_intent`, then durably CAS `claimed → mutation_started`, then durably publish that operation's `operation_started`, and only then invoke the mutation. If `mutation_intent` cannot be persisted, no mutation may start and the same owner MAY release `claimed → issued` only after proving that no durable intent and no effect exists. Once intent is durable, `claimed` is consumed and non-reassignable. If the owner proves durably that the CAS did not occur, it MUST stop before Git mutation and return `refused/audit_unavailable`, exit `1`, leaving the authorization `claimed` with intent. If the owner cannot determine reliably whether the CAS occurred, it MUST stop before any further Git mutation and return `state_indeterminate/audit_failed_after_mutation`, exit `1`; this expresses authorization/audit uncertainty and is not evidence that Git mutated. A failed `operation_started` write after a verified CAS has the same no-retry/no-further-mutation rule and conservative classification. No such failure permits automatic retry, reassignment or mutation with that authorization.

The `GitAuthorization` MUST carry `terminal_result_digest` only through the terminal CAS. Although the apply lock is released before that CAS under the P-07 protocol, only the current `owner_apply_id` may transition its `mutation_started` state. The CAS MUST compare authorization ID, expected `mutation_started` state, expected state version and owner; on success it atomically binds `terminal_result_digest` to the already durable prepared-result digest. It MUST NOT compare against a prior terminal digest, because none exists before closure. Later attempts MAY verify the bound digest. A reconciler or a different authorization MUST NOT write or replace the original result. CAS failure or an unverifiable CAS outcome leaves the original result non-conclusive and is reported conservatively under F008-FR-166/F008-FR-189.

#### F008-FR-180 (Paso 38)

`state_indeterminate` MUST apply only to the affected `authorization_id`, `apply_id`, operation scope and project identity. It MUST NOT permanently prohibit unrelated future plans or authorizations for the project.

Único procedimiento permitido:

1. inspección fresca;
2. identificar efectos aplicados, ausentes o no verificables;
3. crear plan canónico nuevo;
4. obtener autorización supervisada nueva.

La nueva autorización registra `previous_authorization_id`, `previous_apply_id`, `previous_plan_digest` y razón `state_indeterminate`. La anterior sigue terminal e inmutable. No se permite rollback, retry, repair, limpieza arbitraria ni reutilización.

#### F008-FR-181 (Paso 38)

Every branch name required by the selected strategy MUST be an explicit, validated plan input. Feature 008 MUST NOT infer branch names from `init.defaultBranch` and MUST NOT assume `main`, `master`, `develop`, `trunk` or any other conventional name.

- Basic: una rama inicial explícita.
- GitHub Flow: una primary branch explícita, también inicial.
- Trunk-Based Development: una trunk branch explícita, también inicial; sin ramas auxiliares permanentes.
- Git Flow: producción y desarrollo explícitas; inicial igual a producción; development solo con historia suficiente.

Repositorios nuevos usan `git init --initial-branch=<explicit-name> -- <project-path>`. Si la toolchain congelada no lo garantiza, `git_capability_missing`. Los nombres se validan sin crear refs ni modificar el proyecto.

The literal branch name `HEAD` MUST be rejected during planning as `invalid_strategy`, exit `2`, before authorization, `mutation_started`, or `git init`; passing full-ref syntax validation alone is insufficient for an initial branch.

El argv exacto de F008-FR-190, ampliado con `--ref-format=files`, MUST prevalecer sobre cualquier ejemplo abreviado de este requisito.

#### F008-FR-182 (Paso 38)

`ai-dev init plan --output FILE` MUST write only to an explicitly selected location outside the physical project root, outside its worktree, outside every relevant Git directory or common directory, and outside inspected linked-worktree or submodule Git directories.

Antes de escribir debe resolver proyecto y ancestros físicos del destino, rechazar symlinks/aliases hacia áreas prohibidas, rechazar destino existente, verificar creación exclusiva y permisos. Se crea con modo `0600`, sin overwrite y con sincronización durable cuando esté soportada. Un destino incompatible produce `unsafe_output_path` sin crear destino ni temporal.

Exportar fuera del proyecto es escritura externa explícita, no modificación del repositorio. El asistente muestra el destino resuelto y requiere confirmación.

Un destino ya existente MUST producir reason `output_exists`; un destino dentro de un área prohibida, alcanzable por symlink/alias hacia ella o cuya seguridad no pueda demostrarse MUST producir reason `unsafe_output_path`. Ambos rechazos ocurren sin escritura.

#### F008-FR-183 (Paso 38)

Inspection MUST preserve a reversible representation of every observed path. Plan and apply MUST reject any allowlisted path for which equivalence cannot be guaranteed among filesystem bytes, Git path presentation, tree entry, index entry and ordinary `git status`.

El snapshot congela `core.precomposeunicode` efectivo y previsto post-init, `core.ignorecase`, sensibilidad a mayúsculas, comportamiento Unicode, encoding reversible y colisiones.

- UTF-8: preservar bytes exactos.
- NFC/NFD: no normalizar ni equiparar silenciosamente.
- Si `precomposeunicode` cambia bytes: `path_normalization_incompatible`.
- Si ignorecase/filesystem colisionan: `path_collision`.
- Inspect puede reportar no UTF-8 en binario reversible.
- El MVP rechaza allowlist no UTF-8 con `path_unrepresentable`.
- Todo rechazo ocurre antes de blob, CAS o mutación.
- Si no puede predecirse el comportamiento post-init, fail closed.

#### F008-FR-184 (Paso 38)

`index.lock` MUST be created using exclusive creation and retained through its open file descriptor. Ownership MUST be established from the successful exclusive creation, recorded device and inode, open descriptor identity and owning `apply_id`.

No se inserta token en `index.lock`. Antes de rename/unlink se verifica por descriptor que device/inode coinciden. Locks preexistentes o reemplazados no se borran.

#### F008-FR-185 (Paso 38)

`operation_id`, `derived_operations` and canonical plan arguments MUST exclude `apply_id`, authorization state versions, ownership tokens, process identifiers, audit timestamps and concrete temporary paths allocated only at apply time.

Los temporales se representan por roles (`apply_lock`, `prepared_index`, `index_lock`, `config_temporary`, `audit_temporary`). Paths/tokens concretos solo van al registro de intento.

#### F008-FR-186 (Paso 38)

Device and inode values MUST always use the same schema type: unsigned base-10 strings with no leading zeroes. They MUST never change between JSON number and string representations according to magnitude.

#### F008-FR-187 (Paso 38)

The sole canonical identifier for Trunk-Based Development MUST be `trunk-based` in CLI arguments, canonical plans, audit records, reports and `.ai-dev/config.json`. Aliases MUST be rejected rather than silently normalized.

#### F008-FR-188 (Paso 38)

The initial-commit procedure in this erratum is the sole normative sequence. Earlier abbreviated or differently ordered descriptions MUST be interpreted through this sequence and MUST NOT define an alternative implementation path.

#### F008-FR-189 (Paso 40)

After an authorization has reached `mutation_started`, it MAY transition to `failed` only when the attempted operation has stopped and a fresh controlled inspection proves that no planned effect from the entire apply attempt was applied. `failed` MUST be terminal and MUST permanently consume the authorization.

If absence of all planned effects cannot be demonstrated, the terminal state MUST be `state_indeterminate`. If one or more planned effects are verified, but the complete planned result was not achieved, the terminal state MUST be `partially_applied`.

A terminal state, including `failed`, MUST NOT transition to `issued`, `claimed`, or `mutation_started`. The stored authorization state, durable audit result and `GitApplyResult.status` MUST correspond according to the normative result mapping. If a post-mutation audit or terminal-transition write fails, the durable authorization MAY remain `mutation_started`; the returned status MUST be `state_indeterminate` with reason `audit_failed_after_mutation`. A `failed` result MUST satisfy F008-FR-128.

Un fallo anterior a `mutation_started` es un rechazo, no consume la autorización y no usa `failed`, excepto que una autorización `claimed` con `mutation_intent` durable ya está consumida conforme a F008-FR-168/F008-FR-179 aunque no demuestre una mutación Git.

La clasificación MUST considerar todos los efectos del intento: cero efectos demostrados corresponde a status `failed`; uno o más efectos verificados sin resultado completo corresponde a `partially_applied`; efectos no verificables corresponden a `state_indeterminate`. Nunca se clasifica atendiendo solo a los blobs.

#### F008-FR-190 (Paso 40)

For a new SHA-1 repository, the authorized Git initialization operation MUST use exactly this argument vector:

```text
git
init
--template=
--object-format=sha1
--ref-format=files
--initial-branch=<explicit-branch-name>
--
<physical-project-path>
```

It MUST execute through the frozen Git toolchain and controlled Git environment. `init.templateDir`, template directories, template hooks, `init.defaultBranch`, aliases, shell expansion and ambient object-format defaults MUST NOT alter the operation. No template content or template hook may be copied. The initial branch and SHA-1 object format MUST be explicit.

If the frozen toolchain does not support all required options and semantics, planning MUST reject with `git_capability_missing`. Existing repositories MUST NOT be reinitialized and MUST preserve their detected object format.

Es una operación futura planificada y autorizada, no una instrucción para ejecutar `git init` durante esta revisión.

La ausencia de soporte para `--ref-format=files` o cualquier otra capacidad obligatoria MUST producir reason `git_capability_missing`.

#### F008-FR-191 (Paso 40)

The sole initial-commit procedure governed by F008-FR-188 MUST contain two ordered phases. Phase 1 MUST complete successfully for every allowlisted file before Phase 2 creates any Git blob.

#### Phase 1 — Preflight completo

Para todos los archivos, incluida `.ai-dev/config.json`, apply MUST:

1. resolver el path literal;
2. abrir sin seguir symlinks;
3. verificar tipo regular y modo por descriptor;
4. leer bytes desde el descriptor;
5. verificar identidad y metadata antes y después;
6. comparar SHA-256 autorizado;
7. calcular/comparar blob digest sin escribir;
8. ejecutar secret screening;
9. validar atributos y conversiones;
10. retener bytes aprobados o representación inmutable equivalente.

Cualquier fallo rechaza el grupo. Ningún blob, árbol, commit, ref o índice puede crearse hasta que todos los archivos pasen Phase 1.

#### Phase 2 — Construcción

Antes del primer blob, apply MUST ejecutar una segunda barrera que revalide identidades y metadata de todos los archivos. Una diferencia detiene con cero blobs.

Después MUST:

1. crear blobs sin filtros desde exactamente los bytes aprobados;
2. verificar sus digests;
3. construir el árbol exacto;
4. crear el commit congelado;
5. ejecutar el CAS autorizado;
6. instalar el índice después del CAS;
7. verificar todas las postcondiciones.

Rollback y retry automáticos siguen prohibidos. Objetos inalcanzables creados durante Phase 2 permanecen un riesgo aceptado.

Phase 1 MUST ejecutarse antes de la primera mutación del intento cuando el plan contenga el grupo de commit inicial, incluido sobre los bytes previstos de `.ai-dev/config.json`; los bytes y metadatos MUST revalidarse antes del primer blob. Ningún blob puede crearse antes de completar satisfactoriamente la validación global.

#### F008-FR-192 (Paso 40)

The initial-commit procedure defined by F008-FR-188 and F008-FR-191 MUST be represented as one explicitly authorized operation group inside `derived_operations`. It MUST NOT persist the terminal apply result internally.

When Git Flow is selected and the plan authorizes creation of the development branch, the development-reference CAS MUST be a separate, explicitly authorized operation following the initial-commit group.

Orden:

1. ejecutar el commit group;
2. verificar sus postcondiciones;
3. revalidar precondiciones de development;
4. crear development por CAS;
5. verificar sus postcondiciones;
6. continuar al único cierre terminal de apply.

La operación debe contener ref, target commit, estado previo/resultante, `operation_id` y clasificación parcial.

Si falla el CAS después del commit: detener, conservar commit/ref inicial/índice, no rollback/retry, retornar `partially_applied` y auditar el grupo exitoso y el CAS fallido.

Sin historia suficiente no se crea development. Se elimina el cierre terminal interno duplicado.

## Normative Git Flow Profiles

These profiles make F008-FR-038, F008-FR-040, F008-FR-041 and F008-FR-149 verifiable. They are written only when Git Flow and the corresponding explicit dialect are selected. Every key/value is a separate, ordered, non-atomic planned operation with its own precondition and postcondition.

### AVH profile

```text
gitflow.branch.master=<production-branch>
gitflow.branch.develop=<development-branch>
gitflow.prefix.feature=<feature-prefix>
gitflow.prefix.bugfix=<bugfix-prefix>
gitflow.prefix.release=<release-prefix>
gitflow.prefix.hotfix=<hotfix-prefix>
gitflow.prefix.support=<support-prefix>
gitflow.prefix.versiontag=<version-tag-prefix>
```

The AVH profile MUST NOT write `gitflow.path.hooks` or `core.hooksPath`.

### git-flow-next profile

```text
gitflow.version=1.0
gitflow.initialized=true

gitflow.branch.<production>.type=base
gitflow.branch.<production>.upstreamstrategy=none
gitflow.branch.<production>.downstreamstrategy=none
gitflow.branch.<production>.autoupdate=false
gitflow.branch.<production>.tag=false

gitflow.branch.<development>.type=base
gitflow.branch.<development>.parent=<production>
gitflow.branch.<development>.upstreamstrategy=merge
gitflow.branch.<development>.downstreamstrategy=merge
gitflow.branch.<development>.autoupdate=true
gitflow.branch.<development>.tag=false

gitflow.branch.feature.type=topic
gitflow.branch.feature.parent=<development>
gitflow.branch.feature.startpoint=<development>
gitflow.branch.feature.upstreamstrategy=merge
gitflow.branch.feature.downstreamstrategy=rebase
gitflow.branch.feature.prefix=<feature-prefix>
gitflow.branch.feature.autoupdate=false
gitflow.branch.feature.tag=false
gitflow.branch.feature.worktree=false

gitflow.branch.bugfix.type=topic
gitflow.branch.bugfix.parent=<development>
gitflow.branch.bugfix.startpoint=<development>
gitflow.branch.bugfix.upstreamstrategy=merge
gitflow.branch.bugfix.downstreamstrategy=rebase
gitflow.branch.bugfix.prefix=<bugfix-prefix>
gitflow.branch.bugfix.autoupdate=false
gitflow.branch.bugfix.tag=false
gitflow.branch.bugfix.worktree=false

gitflow.branch.release.type=topic
gitflow.branch.release.parent=<production>
gitflow.branch.release.startpoint=<development>
gitflow.branch.release.upstreamstrategy=merge
gitflow.branch.release.downstreamstrategy=merge
gitflow.branch.release.prefix=<release-prefix>
gitflow.branch.release.autoupdate=false
gitflow.branch.release.tag=true
gitflow.branch.release.worktree=false
gitflow.branch.release.tagprefix=<version-tag-prefix>

gitflow.branch.hotfix.type=topic
gitflow.branch.hotfix.parent=<production>
gitflow.branch.hotfix.startpoint=<production>
gitflow.branch.hotfix.upstreamstrategy=merge
gitflow.branch.hotfix.downstreamstrategy=rebase
gitflow.branch.hotfix.prefix=<hotfix-prefix>
gitflow.branch.hotfix.autoupdate=false
gitflow.branch.hotfix.tag=true
gitflow.branch.hotfix.worktree=false
gitflow.branch.hotfix.tagprefix=<version-tag-prefix>

gitflow.branch.support.type=topic
gitflow.branch.support.parent=<production>
gitflow.branch.support.startpoint=<production>
gitflow.branch.support.upstreamstrategy=none
gitflow.branch.support.downstreamstrategy=none
gitflow.branch.support.prefix=<support-prefix>
gitflow.branch.support.autoupdate=false
gitflow.branch.support.tag=false
gitflow.branch.support.worktree=false
```

Instance keys of the form `gitflow.branch.<topic-branch>.base=<base-branch>` MUST be recognized and preserved during inspection/adoption but MUST NOT be created by general initialization. Selecting a non-Git-Flow strategy leaves every `gitflow.*` key byte/value-equivalent and reports `detected_git_flow_not_selected`.

## Consolidated Data Contracts

### GitInspection

A read-only observation containing physical project identity (`path`, decimal-string `device`, decimal-string `inode`), repository kind, Git/common directories, HEAD state and history, refs, index/worktree state, remotes with credentials redacted, Git Flow evidence, conversion/attribute sources, linked-worktree/submodule status, filesystem capabilities and frozen Git toolchain identity. It records warnings and fail-closed incompatibilities without conveying mutation authority.

`PhysicalProjectIdentity.digest` MUST be:

```text
sha256(
  canonical physical path UTF-8
  || NUL
  || unsigned device decimal
  || NUL
  || unsigned inode decimal
)
```

The identity excludes timestamps, size, entry count, `.git`, HEAD, configuration and contents, and therefore remains stable across `git init`.

### GitInitializationPlan

Schema `ai-dev.git-init-plan/v1`, UTF-8 JSON canonicalized by RFC 8785. Its digest covers exactly `schema_version`, immutable intent, complete precondition snapshot, deterministic derived operations, expected postconditions, prohibited operations, partial-effect points and recovery policy. It excludes exactly `plan_id`, `generated_at`, `expires_at`, `inspection_id`, `inspected_at` and audit timestamps. Unknown fields, duplicate keys and floating-point values are rejected. Device, inode and unsafe-range integers are canonical decimal strings. Runtime IDs, tokens and temporary paths do not enter operation IDs or the plan digest.

### GitAuthorization

A durable record with unique `authorization_id`, canonical `plan_digest`, physical `project_identity`, exact operation/file/config/ref scope, declared operator identity, individual normal/ignored file approvals, issue time, optional expiry, state version, owner `apply_id` when claimed, terminal `terminal_result_digest` when conclusively closed, and immutable history. `--authorized-by` is a supervised declaration, not authenticated identity.

State machine:

```text
issued
  └─ claimed
      ├─ issued                  clean release before mutation
      ├─ claimed                 verified orphan reassignment
      └─ mutation_started
          ├─ applied
          ├─ failed              only zero effects proven globally
          ├─ partially_applied   one or more verified effects
          └─ state_indeterminate effects cannot be established
```

Every transition is a durable compare-and-swap over ID, state, version and owner. Terminal states never transition. A pre-mutation refusal does not consume authorization; once mutation starts, it cannot authorize another mutation sequence.

### GitApplyAttempt and GitAuditRecord

Each minimally valid apply invocation receives a fresh `apply_id` and a durable attempt record before any business return. Audit data is outside the repository, uses restrictive permissions and excludes secrets/content. It records operation starts/results, authorization transitions, rejected results, release failures and terminal outcome. An intent lacking a conclusive result is indeterminate and requires reconciliation.

Normative record glossary:

- **Authorization record**: the independently durable `GitAuthorization`, including state/version and its immutable transition history.
- **Apply-attempt record**: created once per minimally valid invocation before any business result; it owns `apply_id` and never grants authority.
- **Mutation-intent record**: the write-ahead record required by F008-FR-062 immediately before the first mutation; historical “intent record” and “audit intent” references mean this record, not the apply-attempt record.
- **Operation-started record**: durable write-ahead record immediately before one planned mutating operation.
- **Operation-result record**: durable completed, failed or indeterminate observation for that operation.
- **Resource-release-started/result records**: write-ahead and outcome records for release of one resource owned by the current attempt. They identify only resource role, operation ID, ownership verdict and non-sensitive outcome.
- **Prepared terminal result**: an immutable, durably synchronized result containing status, primary reason, verified effects and every secondary failure. It is not conclusive until the authorization CAS references its digest.
- **Final audit result**: the prepared terminal result whose digest is referenced by a verifiably successful terminal authorization CAS. It is distinct from process output delivery. An unreferenced prepared result is non-conclusive and MUST NOT be interpreted as applied.

### GitApplyResult

`status` is the outcome category and MUST be one of `applied`, `already_applied`, `refused`, `failed`, `partially_applied`, or `state_indeterminate`. `reason` is the single primary specific cause, MAY be absent for an uncomplicated success, and MUST NOT substitute for status. `secondary_failures[]` records additional failures that did not replace the primary cause and MUST NOT alter effect-based status.

Every `SecondaryFailure` MUST contain exactly `code`, `phase`, `resource_role`, optional `operation_id`, `failure_class`, `ownership_verified`, and optional `audit_record_digest`. The digest MUST identify the latest relevant durable release record when one exists and MUST be absent when persistence of that record failed; an implementation MUST NOT invent a digest. For Feature 008 MVP its `code` MUST be `resource_release_failed`, `phase` MUST be `resource_release`, and `resource_role` MUST be one of `apply_lock`, `index_lock`, `config_temporary`, or `prepared_index`. It MUST NOT contain a concrete temporary path, token, descriptor, secret or file content. The durable final audit result and rendered `GitApplyResult` MUST contain byte-equivalent secondary-failure data.

Every release failure MUST append one independent `SecondaryFailure` in deterministic planned-release order, including when release is the only failure. If release is the only failure, `reason` MUST be `resource_release_failed`; if a primary cause already exists, that cause remains in `reason`. Any non-empty `secondary_failures[]` MUST return exit `1`, including when `status=applied`; status remains determined only by verified effects under F008-FR-166 and F008-FR-189. Failure to persist a release audit record is itself represented for that resource with `ownership_verified` reflecting the last proven ownership and with its available durable audit linkage; if the corresponding effect or terminal CAS cannot be verified, the overall status is `state_indeterminate` and the primary reason is `audit_failed_after_mutation`.

| `GitApplyResult.status` | Durable `GitAuthorization` state | Required interpretation |
|---|---|---|
| `applied` | `applied` | This attempt completed all authorized mutations and verified all postconditions. An informational reason such as `development_branch_pending_initial_history` MAY accompany success when the omitted operation was never authorized/applicable. |
| `already_applied` | existing `applied` | A later attempt verified the prior applied postconditions and performed zero mutations; no new authorization transition occurs. |
| `refused` | no authorization; unchanged pre-existing state; `claimed → issued` after clean release; or unchanged consumed `claimed` with durable intent | No mutation started in this invocation. The authorization is not consumed unless it was already terminal/consumed or carried durable intent before this result. A later invocation against `claimed` with durable intent, `mutation_started`, or `state_indeterminate` returns `authorization_consumed`, leaves the original state unchanged and performs no planned mutation. The original owner that proves its intent-to-state CAS did not occur returns `audit_unavailable` and leaves consumed `claimed` unchanged. |
| `failed` | `failed` | Mutation had started, fresh inspection proves zero planned effects from the entire attempt, and F008-FR-189 is satisfied. |
| `partially_applied` | `partially_applied` | Every relevant effect is verifiable, at least one effect exists, and the complete authorized result was not achieved. |
| `state_indeterminate` | `state_indeterminate`; `mutation_started` when terminal CAS/audit could not be made durable; or last-verified `claimed` with durable intent when the owner cannot determine whether the CAS occurred | At least one relevant effect or the authorization/audit transition cannot be verified. For last-verified `claimed`, this status does not assert Git mutation. A new supervised reconciliation is required. |

`audit_failed_after_mutation` is a `reason` paired with `status=state_indeterminate`; it is not an authorization state or a separate status. `resource_release_failed` and `secondary_failures[]` MUST NOT alter effect-based status. Output delivery failure after a durably verified result uses reason `output_failed`, preserves the durable status/state, and returns exit 2.

If the terminal CAS syscall outcome is not verifiable, the current invocation MUST return `status=state_indeterminate` and `reason=audit_failed_after_mutation` even if a later controlled read discovers that the durable authorization reached `applied`, `failed`, `partially_applied`, or `state_indeterminate`. That later discovery does not rewrite the original process result or prepared record; supervised reconciliation records the observed durable state in a new audit attempt.

The closed reason vocabulary is: `interactive_terminal_required`, `invalid_strategy`, `missing_required_branch`, `repository_absent`, `ancestor_repository`, `repository_kind_unsupported`, `repository_state_incompatible`, `worktree_config_unsupported`, `project_identity_unavailable`, `plan_invalid`, `plan_stale`, `concurrent_change`, `authorization_input_incomplete`, `authorization_id_exists`, `authorization_missing`, `authorization_mismatch`, `authorization_expired`, `authorization_consumed`, `audit_unavailable`, `audit_failed_after_mutation`, `resource_release_failed`, `lock_conflict`, `git_capability_missing`, `development_branch_pending_initial_history`, `output_exists`, `unsafe_output_path`, `output_failed`, `operation_failed`, `attributes_incompatible`, `path_unrepresentable`, `path_normalization_incompatible`, `path_collision`, `sensitive_content_suspected`, `commit_message_invalid`, `identity_invalid`, `platform_unsupported`, `filesystem_unsupported`, `dubious_ownership`, `strategy_conflict`, and `configuration_conflict`. Adding a new reason requires a later normative amendment; implementations MUST NOT silently substitute reason strings.

The normative rejection mapping is closed and MUST be applied as follows. Unless the table explicitly assigns exit `2`, each rejection is a domain refusal with `GitApplyResult.status=refused` and exit `1`. A rejection after mutations have begun MUST instead use the effect-based status rules in F008-FR-166 and F008-FR-189 while retaining the applicable causal reason.

Reason precedence for repository state is mandatory and phase-based:

1. An incompatible condition present while generating or initially validating a plan uses `repository_state_incompatible` under F008-FR-112.
2. During `apply`, drift between current state and the authorized precondition snapshot detected before the first mutation uses `plan_stale` under F008-FR-058.
3. After an operation’s immediate preconditions have been successfully revalidated, a subsequent concurrent change uses `concurrent_change` under F008-FR-164. Before mutation it is `status=refused`, exit `1`; after mutation begins, effect-based status classification controls without changing the causal reason.

These reasons MUST NOT be selected merely by matching the final observed state; the lifecycle phase in which the incompatibility or change is first established controls. All three remain in the closed reason vocabulary and retain exit `1` for pre-mutation refusal.

| Rejection category | Required `reason` | Governing requirements / boundary |
|---|---|---|
| Interactive assistant without a terminal | `interactive_terminal_required` | F008-FR-003; exit `2` |
| Unknown/noncanonical strategy, invalid development-base usage, literal initial branch `HEAD`, or required branch option absent | `invalid_strategy` or `missing_required_branch` | F008-FR-035–037, F008-FR-046, F008-FR-181; exit `2` |
| Repository absent when initialization was not authorized | `repository_absent` | F008-FR-022 and plan intent |
| Nested initialization or an ancestor repository would be captured | `ancestor_repository` | F008-FR-138 |
| Bare repository, linked worktree, or submodule used for plan/apply | `repository_kind_unsupported` | F008-FR-022–023; inspect remains read-only and reports the detected kind |
| Repository is inaccessible or ambiguous; HEAD is invalid; an interrupted merge, rebase, cherry-pick, revert, bisect or am is in progress (`sequencer_state` other than `none`, including `unknown`), whatever operations the plan contains; or an initial-commit group initially encounters reachable history, an existing initial ref, a non-empty index, index conflicts, or non-unborn HEAD | `repository_state_incompatible` | F008-FR-022, F008-FR-112, F008-FR-162 and case O-34; the condition exists at plan generation/initial validation and rejection occurs before blobs or mutation |
| `extensions.worktreeConfig=true` | `worktree_config_unsupported` | F008-FR-176 |
| Physical project identity cannot be established safely | `project_identity_unavailable` | F008-FR-021, F008-FR-150–151 |
| Untrusted plan has invalid schema, unknown fields, noncanonical data, forbidden/impossible operations, or cannot be produced by the legitimate planner | `plan_invalid` | F008-FR-050–057, F008-FR-140–141, F008-FR-165, F008-FR-172–174 |
| Plan/JSON structure, members, strings, path, authorized-file count, operation count, or postcondition count exceeds F008-FR-051 | `plan_invalid` | Exact boundary is valid; one unit/byte over rejects before authorization/mutation |
| Configuration, attribute, or ignore source exceeds its per-source or aggregate bound; or bounded output overflows while evaluating that source | `attributes_incompatible` | F008-FR-028–034, F008-FR-051, F008-FR-175 |
| Authorized file or aggregate authorized bytes exceed their bounds | `repository_state_incompatible` | F008-FR-051; initial validation, before authorization/mutation |
| Commit message exceeds its byte bound | `commit_message_invalid` | F008-FR-051, F008-FR-104–107; before authorization/mutation |
| Captured stdout/stderr exceeds its bound | Reason for the source/operation being evaluated | F008-FR-051; no generic substitution, and post-mutation status remains effect-based |
| An effective, deliberately replayed or repository-local `core.sharedRepository` value can alter planned permissions/effects | `configuration_conflict` | The ambient system/global/XDG value alone is not effective under the private subprocess profile and is not rejected |
| During apply, a valid authorized plan no longer matches its digest, regenerated canonical form, toolchain, or frozen precondition snapshot before the first mutation | `plan_stale` | F008-FR-054–058, F008-FR-156, F008-FR-167 |
| State changes after the applicable operation preconditions were successfully revalidated immediately | `concurrent_change` | F008-FR-164; pre-mutation refusal is exit `1`, while post-mutation status follows effect classification |
| Authorization issuance/lookup/scope/expiry/consumption failure | The matching `authorization_input_incomplete`, `authorization_id_exists`, `authorization_missing`, `authorization_mismatch`, `authorization_expired`, or `authorization_consumed` | F008-FR-152–154, F008-FR-159–160, F008-FR-167–168, F008-FR-178–180 |
| Audit cannot be durably created before mutation | `audit_unavailable` | F008-FR-059–064, F008-FR-154 |
| Audit/result persistence fails after mutation | `audit_failed_after_mutation` | F008-FR-066, F008-FR-189; status is `state_indeterminate` |
| Current operation cannot exclusively acquire its apply/config/index lock because a lock exists or ownership conflicts | `lock_conflict` | F008-FR-070–071, F008-FR-117, F008-FR-169, F008-FR-184 |
| A resource created by this attempt cannot be safely released, or has been replaced | `resource_release_failed` | F008-FR-169, F008-FR-184; it does not change effect-based status |
| Frozen Git lacks a mandatory capability | `git_capability_missing` | F008-FR-017–018, F008-FR-155–158, F008-FR-190 |
| Git Flow development ref is intentionally deferred for lack of initial history | `development_branch_pending_initial_history` | F008-FR-043–045; informational on otherwise successful apply |
| Output destination exists, is unsafe, or final delivery fails | `output_exists`, `unsafe_output_path`, or `output_failed` | F008-FR-182 and CLI output contract; exit `2` |
| Planned operation fails without a more specific reason | `operation_failed` | F008-FR-039, F008-FR-142–143 and effect-based classification |
| Attributes, conversions, or filters prevent byte/status equivalence | `attributes_incompatible` | F008-FR-028–034, F008-FR-148, F008-FR-175 |
| Path bytes are non-reversible/invalid, Unicode representation is incompatible, or paths collide | `path_unrepresentable`, `path_normalization_incompatible`, or `path_collision` | F008-FR-024, F008-FR-087–090, F008-FR-174, F008-FR-177, F008-FR-183 |
| Secret screening reports a high-risk finding | `sensitive_content_suspected` | F008-FR-085, F008-FR-091–092, F008-FR-191 |
| Approved commit message violates the canonical or Conventional Commits contract | `commit_message_invalid` | F008-FR-102–107; standalone validator invalidity also exits `1` |
| Author or committer name/email is absent or unsafe | `identity_invalid` | F008-FR-096–100 |
| Unsupported OS or filesystem semantics | `platform_unsupported` or `filesystem_unsupported` | F008-FR-131–136 |
| Ownership is dubious | `dubious_ownership` | F008-FR-019 |
| Selected strategy cannot be reconciled with its required branch/configuration contract | `strategy_conflict` | F008-FR-038–049, F008-FR-146, F008-FR-170 |
| Existing project configuration differs from the authorized configuration | `configuration_conflict` | F008-FR-072–079 |

### GitStrategyReport

A read-only description of explicitly selected strategy, detected informational evidence, dialect/branches, compatibility, pending dependencies and warnings. It MUST declare `grants_mutation_authority=false` and convey neither credentials, plans nor authorization.

### ConventionalCommitValidation

A read-only result containing syntax validity, parsed type/scope/description/body/footers, breaking-change recognition and diagnostics. Validity and recognition of a breaking change are separate. The validator accepts Conventional Commits 1.0.0 types with uppercase characters; ai-dev-generated messages use lowercase types.

## CLI Contract

`--project-dir` is the canonical project-root option established by Steps 32 and 34. The abbreviated `--path` spelling shown in Step 36 is not a second interface or alias and MUST be rejected as unknown unless a future amendment defines it.

### Interactive assistant and inspect

```text
ai-dev init [--project-dir PATH]
ai-dev init inspect [--project-dir PATH] [--format text|json]
```

`ai-dev init` requires an interactive terminal, has no preselected strategy, displays the canonical plan and uses the same inspector/planner/apply engine as non-interactive commands. Without an interactive terminal it returns reason `interactive_terminal_required`, exit 2 and zero mutation.

### Plan

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

Selecting any strategy makes the planned atomic write of `.ai-dev/config.json` mandatory; `--write-config` does not exist. Required branch options are strategy-specific under F008-FR-181. `--development-base-oid` has exactly the F008-FR-046 meaning and is prohibited for non-Git-Flow strategies, new repositories, repositories without history, or an already-existing requested development ref. A missing production/development/trunk/initial branch required by the selected strategy returns `missing_required_branch`, exit 2. A syntactically malformed development OID, wrong algorithm, or context-forbidden use returns `invalid_strategy`, exit 2. In existing history with selected Git Flow and absent development, omission, an inaccessible/nonexistent commit, or a well-formed OID unequal to the frozen production tip returns `strategy_conflict`, exit 1. `trunk` and `trunk_based` return `invalid_strategy`, exit 2. `plan --output` is an explicit external write, not a project/repository mutation. `output_exists` and `unsafe_output_path` both return exit 2 without creating a destination or temporary file.

### Apply — issue and immediately consume

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

This is the only non-interactive issuance mode. It requires the exact per-file approval arguments whenever the plan includes an initial commit. `--authorized-by` is a declaration, not technical authentication.

### Apply — consume an existing authorization

```text
ai-dev init apply
  --plan FILE
  --confirm-plan sha256:DIGEST
  --authorization-id UUID
  [--format text|json]
```

This mode uses and verifies the approvals already stored in the authorization. It MUST reject issuance-only flags and MUST NOT reissue, expand or replace the stored scope.

### Conventional Commit validator

```text
ai-dev git commit validate (--message TEXT | --file FILE) [--format text|json]
```

`--message` and `--file` are mutually exclusive. The validator invokes no Git command and requires no repository or strategy.

### Exit codes

| Exit | Commands/results |
|---:|---|
| `0` | Successful inspect or plan; apply status `applied` or `already_applied` with empty `secondary_failures[]`; valid Conventional Commit. |
| `1` | Domain refusal/failure, including apply `refused`, `failed`, `partially_applied` or `state_indeterminate`, and any apply result with non-empty `secondary_failures[]`; invalid Conventional Commit. |
| `2` | Invalid usage, `interactive_terminal_required`, `invalid_strategy`, `missing_required_branch`, `output_exists`, `unsafe_output_path`, or output delivery failure. |

An exit 2 after mutations or a durable applied result MUST NOT be interpreted as proof of zero effects. No CLI option grants network, remote, merge or Human Gate authority.

## Definitive Apply Sequence

1. Validate minimal input size/schema limits.
2. Generate `apply_id` and durably record the attempt; failure returns `audit_unavailable` with no mutation.
3. Resolve current physical project identity and locate `authorization_id`.
4. If explicitly requested, exclusively issue the fully scoped authorization in `issued`; reject insufficient data or existing ID.
5. Bind authorization to `--confirm-plan`, project identity, scope, approvals and expiry; classify missing, mismatch, expired, consumed, applied, partial or indeterminate and durably audit any refusal. If the original authorization is `claimed` with durable `mutation_intent`, `mutation_started` without a conclusive result, or terminal `state_indeterminate`, leave that durable state unchanged and stop this later attempt with `status=refused`, `reason=authorization_consumed`, exit `1`, and no planned mutation.
6. Validate the exact normal/ignored per-file approval set.
7. Perform controlled fresh inspection; extract only immutable intent from the untrusted plan; regenerate through the legitimate planner and require byte-identical canonical form and confirmed digest.
8. If the plan contains the initial-commit group, complete the global preflight of every authorized file and intended in-memory `.ai-dev/config.json` before the first mutation. No blob may exist before global validation.
9. Acquire the external apply lock; reread authorization and preconditions; claim by durable CAS.
10. Before the first explicit operation, durably publish `mutation_intent`, CAS `claimed → mutation_started`, then durably publish `operation_started`; only then execute once. For every later operation, revalidate immediately, durably publish `operation_started`, execute once without fallback, verify postconditions and stop on first failure. An intent-write failure permits a clean same-owner `claimed → issued` release only after proving no durable intent/effect. After durable intent, the authorization is consumed: a proven failed CAS returns `refused/audit_unavailable`; an unverifiable CAS returns `state_indeterminate/audit_failed_after_mutation`; both exit `1`, prohibit every later Git mutation/retry, preserve the last verified durable state, and require linked supervised reconciliation.
11. If authorized, execute the single initial-commit operation group defined below.
12. If Git Flow development creation is authorized, verify the commit group, revalidate development-ref preconditions, execute the separate CAS and verify it.
13. Freshly reinspect and verify every planned postcondition.
14. Classify all effects and compute a provisional status/primary reason; durably record release intent and one result or failure for each owned resource, append one deterministic `SecondaryFailure` for every release failure, and release the apply lock last; prepare and synchronize exactly one immutable terminal result containing the complete ordered set; then CAS the authorization to the matching terminal state and `terminal_result_digest`. Only the current owner may perform this transition, and the result becomes conclusive only after that CAS is verified.
15. Return `GitApplyResult`; no automatic rollback, retry, repair, substitution or arbitrary cleanup occurs.

Crash semantics for step 14 are conservative. A crash before or during resource release leaves the authorization `mutation_started` and the missing conclusive result requires supervised reconciliation. A crash after apply-lock release but before the prepared terminal record or terminal CAS remains safe because no process may reclaim `mutation_started`; later apply attempts only audit `authorization_consumed` and mutate nothing. A crash after the prepared record but before CAS leaves an explicitly non-conclusive record. A verifiably successful CAS references an already durable record, so no later audit write is required to establish the terminal outcome. CAS failure or unverifiable CAS outcome returns `state_indeterminate/audit_failed_after_mutation`, never retries, and never reuses the authorization.

## Deterministic Repository Initialization

For a new repository the exact authorized argv is:

```text
git init --template= --object-format=sha1 --ref-format=files --initial-branch=<explicit-branch-name> -- <physical-project-path>
```

It runs without a shell under the controlled frozen toolchain/environment. It neutralizes templates and ambient defaults. Existing repositories are never reinitialized and retain object format. Missing support for any mandatory option or semantic yields reason `git_capability_missing` before mutation. This exact F008-FR-190 form prevails over abbreviated examples in F008-FR-181.

## Single Initial-Commit Flow

The optional commit is one authorized operation group inside `derived_operations` and never closes apply internally.

**Phase 1 — global preflight before first mutation**: whenever the plan contains the initial-commit group, generate intended config bytes in memory; for every allowlisted file resolve literal path, open without following links, verify regular type/mode and metadata before/after, read bytes by descriptor, compare authorized SHA-256 and calculated blob digest without writing, run high-risk secret screening, evaluate attributes/conversions and retain immutable approved bytes. Any failure stops with no blob. In a repository-absent project, checks use the physical root and in-memory intent without creating `.git`; inability to guarantee a check fails closed.

**Phase 2 — construction**: revalidate all file identities/metadata; create blobs without filters from retained bytes; verify digests; construct the exact tree; create one unsigned commit with frozen identities/dates and the approved UTF-8 message ending in exactly one LF; CAS the initial ref; acquire and verify owned `index.lock`; build an autonomous version-2 index without split/shared dependencies; install it without overwriting concurrent changes; verify HEAD/ref/commit/tree/index/status and preservation of unauthorized paths.

A CAS failure leaves refs/index unchanged and reports unreachable objects. A post-CAS index/verification failure is `partially_applied`; no ref rollback occurs. Result classification considers all effects of the entire attempt, not only blobs.

For Git Flow, the development-ref CAS is a distinct later operation. It runs only after the commit group succeeds and its postconditions are verified. Failure leaves the initial commit/ref/index, returns `partially_applied` and is audited. Without sufficient history, development is not created and reason `development_branch_pending_initial_history` is reported.

## Prepared Index and `index.lock` Protocol

The prepared index MUST be constructed in private external temporary storage through controlled `GIT_INDEX_FILE` with:

```text
core.splitIndex=false
core.fsmonitor=false
core.untrackedCache=false
index.version=2
index.skipHash=false
```

Sparse index, shared index, normal-index reads, filters, worktree updates and optional locks are prohibited. A repository with `extensions.worktreeConfig=true` is rejected under F008-FR-176 rather than overridden.

Before installation, verification MUST prove `DIRC` magic, version 2, exact entry count, exact tree paths/modes/blob OIDs, no `link` extension, no shared-index reference, a valid checksum and no additional entry.

The sole `index.lock` protocol is:

1. Confirm the planned absence/precondition of `index.lock`.
2. Create `.git/index.lock` exclusively without following symlinks and retain its descriptor.
3. Record descriptor-derived device, inode, mode, owner and `apply_id`; do not embed a token in the Git index.
4. Keep the descriptor open.
5. Revalidate the normal index against the authorized snapshot.
6. Build and verify the autonomous prepared index outside the repository.
7. Copy the verified prepared-index bytes to the retained descriptor.
8. `fsync` the descriptor.
9. `fstat` and verify device/inode and ownership immediately before installation.
10. Rename the owned lock to `index` using the controlled Git-compatible protocol.
11. `fsync` the Git directory.
12. Verify the installed index and ordinary status.
13. If abandoning before rename, unlink only the inode proven to be the current attempt’s exclusively created lock; a missing, replaced or foreign lock remains untouched and yields reason `resource_release_failed`.

## Acceptance Criteria Catalogue

All cases below are proposed specification tests; **zero Feature 008 tests have been executed**.

- **AC-001 Read-only inspection**: hostile Git config, fsmonitor, filters, hooks, credentials, pager/editor and `GIT_*` cause no execution or project/index change.
- **AC-002 Canonical plan**: regenerated plan is byte-identical under RFC 8785; excluded metadata changes do not affect digest; frozen intent changes do.
- **AC-003 Per-file approval**: a missing, extra, duplicate or mismatched normal/ignored approval refuses before mutation.
- **AC-004 Authorization issuance**: CLI issuance persists complete `issued` scope before mutation; existing ID and insufficient input refuse without overwrite.
- **AC-005 Authorization concurrency**: two apply processes cannot consume one authorization; terminal state cannot return to claimed.
- **AC-006 Crash/reconciliation**: orphan claim and mutation intent follow the defined safe paths. A later invocation against `claimed` with durable intent returns `refused/authorization_consumed`; the original owner returns `refused/audit_unavailable` when it proves the CAS did not occur and `state_indeterminate/audit_failed_after_mutation` when the CAS outcome is unverifiable. Every case exits `1`, performs no later Git mutation/retry, preserves the original record and requires new linked plan/authorization for reconciliation.
- **AC-007 Deterministic init**: hostile template/default branch/default ref format do not alter SHA-1/files-ref init; unsupported capability yields `git_capability_missing`; existing repo is not reinitialized.
- **AC-008 Global preflight**: a secret in the last allowlisted file creates zero blobs and, where preflight is applicable before all mutations, zero project effects.
- **AC-009 Effect classification**: zero effects proven across the entire attempt yield `failed`; fully verifiable earlier effects with an incomplete result yield `partially_applied`; any unverifiable effect yields `state_indeterminate`.
- **AC-010 Commit/index coherence**: successful commit leaves exact tree/index and no artificial changes; CAS-to-index failure is partial and auditable.
- **AC-011 Git Flow**: AVH/git-flow-next profiles, reserved names, partial config, non-Git-Flow selection with existing keys, no-history pending reason and later development CAS behave as specified.
- **AC-012 Paths**: shell metacharacters remain literal; `|`, traversal, symlinks, ambiguous NFC/NFD, incompatible precompose/case behavior and non-reversible non-UTF-8 paths refuse before mutation.
- **AC-013 Output safety**: existing output yields `output_exists`; project/Git-dir/symlink alias or unverifiable destination yields `unsafe_output_path`; neither writes.
- **AC-014 Attributes**: repository/info/global/system sources and config conversions are considered without executing external filters; incompatible cases refuse.
- **AC-015 Toolchain/index**: changed launcher/effective executable/exec-path/capabilities/`DEVELOPER_DIR`, worktreeConfig or split/shared index behavior fails closed.
- **AC-016 Conventional Commits**: `Refs #123`, `BREAKING CHANGE`, `BREAKING-CHANGE`, lowercase `breaking-change`, uppercase types and invalid messages distinguish validity from breaking recognition.
- **AC-017 Locks**: only resources created and still owned by the current attempt are released; a replaced or foreign `index.lock` is never removed.
- **AC-018 Audit failures**: unavailable pre-mutation audit blocks mutation; post-mutation audit failure reports `audit_failed_after_mutation` without rollback.
- **AC-019 Platforms**: macOS/APFS and Linux/ext4 cover supported acceptance; Windows and unsupported filesystems fail closed without claiming validation.
- **AC-020 Regression/authority**: Features 001–006 behavior, Workflow State and Human Gate evidence remain unchanged; Feature 007 receives only non-authoritative read-only strategy information.
- **AC-021 Result/state mapping**: every apply status maps to the durable authorization state exactly as specified; post-mutation audit failure returns `state_indeterminate/audit_failed_after_mutation` and never invents an authorization state. A new invocation against an original authorization in `claimed` with durable intent, `mutation_started`, or `state_indeterminate` creates its own audit attempt, returns `refused/authorization_consumed`, exit `1`, performs zero planned mutations, and leaves the original state unchanged. Owner CAS outcomes use the distinct mappings in AC-006.
- **AC-022 CLI contract**: every documented option is accepted only in its mode; `--path` is rejected; exit codes and the closed rejection-category-to-reason mapping match the CLI tables. Bare repositories, linked worktrees, submodules, worktree configuration, secrets, locks, malformed plans, invalid messages and invalid identities return their specified reasons.
- **AC-023 Git Flow profiles**: AVH writes exactly eight listed keys and neither hooks key; git-flow-next writes exactly the listed profile, preserves topic `.base` instance keys, and reports any partial write.
- **AC-024 Project identity**: identity is stable across `git init`, follows the documented digest formula, serializes device/inode as decimal strings, and changes when physical path/device/inode changes.
- **AC-025 Prepared index**: `index.skipHash=false`, checksum verification and every step of the 13-step ownership protocol are observable; a replaced lock is never removed.
- **AC-026 Audit records**: attempt, mutation-intent, operation-started, operation-result and final records are distinguishable, ordered and durable according to their definitions.
- **AC-027 Identity and message validation**: unsafe names/emails, noncanonical message bytes and implicit signing are rejected or neutralized before object creation as required.
- **AC-028 Complete initial-commit flow**: secret in the last allowlisted file yields zero blobs; successful flow creates exact tree/index; development CAS occurs later; exactly one terminal result is persisted.
- **AC-029 Repository-state compatibility (O-34)**: an initial-commit plan against reachable history, an existing initial ref, non-unborn or invalid HEAD, a non-empty index, or index conflicts returns `status=refused`, `reason=repository_state_incompatible`, exit `1`, and creates no blob or mutation. An inaccessible or ambiguous repository returns the same result without being treated as absent. Any plan against a repository with an interrupted sequencer operation (merge, rebase, cherry-pick, revert, bisect, am or an unclassifiable sequencer state) returns the same result, while inspect still reports the observed `sequencer_state`. Existing history remains inspectable and does not block an otherwise valid plan that omits the initial-commit group.
- **AC-030 Repository-reason precedence (c-01)**: the same observable repository condition produces `repository_state_incompatible` when present at plan generation/initial validation, `plan_stale` when first detected as drift from the authorized snapshot during apply before its first mutation, and `concurrent_change` when state changes only after successful immediate operation-level revalidation. The first two and a pre-mutation concurrent change return `status=refused`, exit `1`, create no blob and perform no planned mutation; a concurrent change after mutation begins retains that reason and uses conservative effect-based status classification.
- **AC-031 Existing-history development creation**: `--development-base-oid` is accepted only for Git Flow on an existing repository with history and an absent requested development ref. Omission returns `strategy_conflict`/exit `1`; malformed syntax, wrong algorithm or forbidden context returns `invalid_strategy`/exit `2`; inaccessible/nonexistent commit or mismatch with frozen production returns `strategy_conflict`/exit `1`; only the exact accessible production tip produces the canonical, explicitly authorized target-absent CAS. Later drift and a losing CAS use c-01 without inference or fallback.
- **AC-032 Terminal release result**: exercise (a) successful authorized operations with a sole release failure, (b) a primary operation failure followed by release failure, (c) two distinct release failures, (d) audit failure during release, and (e) crash after apply-lock release but before terminal CAS. Every release failure appears as its own deterministic structured entry in both durable/result output; release-only uses primary `resource_release_failed`; an earlier cause is preserved; any non-empty set exits `1` without changing effect-based status; audit uncertainty is conservative; an unreferenced result never becomes conclusive and authorization is never reused.

### Requirement-to-Acceptance Traceability

Every row defines proposed tests only; none has been executed.

| Requirements | Acceptance criteria | Required observable result |
|---|---|---|
| F008-FR-001–010 | AC-020, AC-022 | Four command families exist; TTY/default/authority boundaries reject without mutation. |
| F008-FR-011–027 | AC-001, AC-019, AC-022, AC-029 | Inspection is side-effect free; repository states and redacted inputs are reported exactly, and inaccessible or ambiguous repositories fail closed for plan/apply. |
| F008-FR-028–034 | AC-014 | Effective transformations are evaluated without executing filters; uncertainty rejects. |
| F008-FR-035–049 | AC-011, AC-023 | Exactly four canonical strategies; Git Flow selection/profile/branch rules and nonselection behavior match the contract. |
| F008-FR-050–058 | AC-002, AC-030 | Untrusted plan operations never dispatch directly; canonical regeneration/digest drift rejects with phase-correct reason precedence. |
| F008-FR-059–071 | AC-005, AC-006, AC-018, AC-026 | Audit records and locks are durable, ordered, private and non-authoritative. |
| F008-FR-072–079 | AC-011 | Configuration is mandatory, credential-free, atomically written or a verified no-op/conflict. |
| F008-FR-080–093 | AC-003, AC-008, AC-028 | Optional commit requires exact individual approvals and complete secret screening before blobs/mutation. |
| F008-FR-094–111 | AC-007, AC-016, AC-027 | Object format, identity, dates, message, validator, signing and exact object bytes are reproducible. |
| F008-FR-112–129 | AC-009, AC-010, AC-025, AC-028–030 | Initial-commit repository-state preconditions, reason precedence and CAS/index/status/idempotency outcomes follow exact classification. |
| F008-FR-130–137 | AC-019, AC-020 | No network; supported-platform claims and regressions remain bounded. |
| F008-FR-138–161 | AC-002, AC-003, AC-011, AC-020, AC-023, AC-024 | Nested repos, explicit operations, strategy freedom, stable identity, toolchain, approvals and index guarantees are preserved. |
| F008-FR-162–177 | AC-001, AC-002, AC-009, AC-012, AC-014, AC-015, AC-022, AC-025, AC-029–030 | HEAD, argv, revalidation, postconditions, reason precedence, JSON, Unicode, attributes and path separator rules are exact. |
| F008-FR-178–188 | AC-004–006, AC-012–013, AC-017, AC-021–026 | Issuance, CAS states, reconciliation, branches, output, path/index ownership and identifiers are verified. |
| F008-FR-189–192 | AC-007–010, AC-021, AC-028 | Terminal classification, deterministic init, global preflight and later Git Flow development CAS use one closure. |

P-06/P-07 precision: F008-FR-046 is additionally verified by AC-031; F008-FR-059–069, F008-FR-126, F008-FR-128, F008-FR-166, F008-FR-168–169, F008-FR-179–180, F008-FR-184 and F008-FR-189 are additionally verified by AC-032.

Mandatory precision cases include: concurrent loser returns `already_applied` after verifying winner postconditions; missing branch, invalid strategy, existing output and unsafe output return exit 2 with their canonical reasons; O-34 existing history makes only the initial-commit group inapplicable and returns `refused`/`repository_state_incompatible`/exit `1`; non-empty or conflicted index, invalid HEAD, and inaccessible or ambiguous repository initially use that same reason; equivalent drift against an authorized snapshot uses `plan_stale`; a change after successful immediate revalidation uses `concurrent_change`, with effect-based status if mutation began; `trunk` and `trunk_based` are rejected; an inode above 2^53 remains a decimal string; unsafe identity headers and implicit signing are rejected; `Refs #123` and all three breaking-footer forms distinguish message validity from breaking recognition.

## Key Entities

- **PhysicalProjectIdentity**: canonical physical path plus stable decimal-string device/inode; excludes timestamps and contents.
- **GitToolIdentity**: launcher/effective executable mode, paths/digests, version, exec-path, capabilities and environment selectors.
- **ImmutableIntent**: all human-approved strategy, branch, configuration, commit, identity/message/time and allowlist choices.
- **PreconditionSnapshot**: complete state whose drift invalidates application.
- **AuthorizedFile**: literal reversible path, mode, bytes digest, blob digest, ignored status and individual approval.
- **DerivedOperation**: deterministic operation ID, exact argv/target, preconditions, effects, postconditions and partial-effect classification.
- **GitAuthorization**: durable single-sequence mutation authority separate from Human Gates.
- **GitApplyAttempt/GitApplyResult**: one invocation’s immutable audit identity and terminal outcome/cause.

## Success Criteria

- **SC-001**: Inspection and planning produce zero project/repository/index/ref/config mutations across every acceptance fixture.
- **SC-002**: Every plan/apply path requires exactly one of four explicit strategy identifiers; no default is selected or inferred.
- **SC-003**: 100% of planned mutations are enumerated with preconditions, effects and postconditions; no unplanned mutation is observable.
- **SC-004**: Two concurrent apply attempts cannot both start mutations under one authorization.
- **SC-005**: Every minimally valid apply invocation has a durable `apply_id` record before any business result.
- **SC-006**: An authorized successful initial commit has exact tree/index equality and no ai-dev-induced working-tree differences.
- **SC-007**: Secret/allowlist/attribute failures create no blobs; global preflight failures create no earlier mutation whenever preflight is applicable.
- **SC-008**: All supported initialization fixtures use explicit SHA-1, files refs, empty template and explicit initial branch; no ambient default changes the result.
- **SC-009**: Every apply outcome follows the normative status/authorization-state table; `reason` remains a separate cause, and post-mutation audit failure is represented as `status=state_indeterminate` with reason `audit_failed_after_mutation`.
- **SC-010**: macOS/APFS and Linux/ext4 acceptance suites pass before support is claimed; Windows remains explicitly unsupported for plan/apply.
- **SC-011**: Conventional Commit validation accepts/rejects the normative corpus without invoking Git or requiring a repository.
- **SC-012**: Regression verification demonstrates no authority or behavior change to Features 001–006 and no mutation authority conveyed to Feature 007.

## Assumptions, Dependencies, and Residual Risks

- Git is available with the frozen capabilities required by the selected plan; absence fails closed.
- Supported roots are local macOS/APFS or Linux/ext4 filesystems with required identity, locking, fsync, atomic replacement and CAS semantics.
- Processes with the same OS-user permissions can imitate supervised input; the future protected Git Authorization Broker is out of scope.
- Secret scanning can have false negatives and does not promise complete detection.
- Failed construction may leave unreachable Git objects.
- A successful ref CAS can be followed by index/audit failure, producing an explicit partial state.
- Git Flow multi-key writes are not atomic.
- A filesystem remount may change device/inode and safely invalidate prior plans/authorization correlation.
- A TOCTOU window remains between final revalidation and system calls against equally privileged concurrent processes.
- Git provenance is frozen and compared but not cryptographically certified by a trusted authority.
- Future Git Flow configurations or filesystems may be unrecognized and therefore rejected.
- An `issued` authorization without `expires_at` remains consumable until it is claimed, terminal, or otherwise invalidated by precondition drift; operators must use expiration when bounded lifetime is required.
- A failed final synchronization of an external plan artifact may leave a partial external file; it grants no authority and must not be consumed as a valid canonical plan.
- Future Git or macOS Unicode behavior may make a previously representable path incompatible; plan/apply fail closed rather than normalizing silently.
- After the apply lock is released and before the owner-only terminal CAS is verified, another apply using a distinct authorization may acquire the lock. The original attempt remains protected by its `mutation_started` owner/version CAS and atomic digest binding; the later attempt must detect any repository drift, but the cross-authorization window is not an atomic multi-record transaction.

## Traceability Summary

- F008-FR-001–137 originate in Step 32 with the explicit replacements recorded in the source matrix.
- F008-FR-138–161 originate in Step 34.
- F008-FR-162–177 originate in Step 36.
- F008-FR-178–188 originate in Step 38.
- F008-FR-189–192 originate in Step 40.
- Step 41 N-E and P-1–P-4 are integrated, respectively, into deterministic init, global preflight, global effect classification, precedence and status/reason coding.
- Step 55 P-06 and P-07 were expressly approved by the project-responsible user and refine F008-FR-046 plus the terminal release/result protocol. Step 55-B B-01/B-02 were expressly approved microcorrections to the `concurrent_change` citation and self-digest wording. Step 57 N-01/N-02/N-03 were expressly approved decisions governing release-failure enumeration/exit, development-base outcomes, and safe in-process configuration/ignore evaluation. No authenticated approver identity, signature or approval timestamp is asserted.
- Step 59 expressly approves the limit/reason mapping, consumed `claimed`-with-intent semantics, effective `core.sharedRepository` treatment, safe lock-identity reuse, terminal-digest binding, and fixed private HOME/XDG policy. It does not assert authenticated identity, signature, or an approval timestamp.
- The F008-FR-035 strategy spelling is explicitly harmonized by later F008-FR-187; F008-FR-169 `index.lock` ownership is explicitly narrowed by later F008-FR-184.
- The complete AVH/git-flow-next profiles, complete CLI, result-state mapping, audit glossary, prepared-index protocol and FR-to-acceptance matrix are normative supporting contracts for the stable FR-001–192 ledger.
- No proposed acceptance test is represented as executed, and this specification does not authorize implementation.
