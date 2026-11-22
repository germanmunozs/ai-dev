# Informe de preparación — Paso 93

## Feature 008 — Git Management / Fase 1

Fecha: 2026-10-31

Naturaleza: preparación exclusivamente. Este informe no autoriza ni ejecuta la adopción de la línea base. Debe permanecer fuera de los tres commits previstos salvo autorización humana posterior y específica.

## A. Veredicto

**READY WITH CONDITIONS**

La adopción puede autorizarse posteriormente. No se encontró ninguna condición de bloqueo.

Condiciones pendientes antes del commit de código:

1. Autorizar la validación ejecutable con `go test` y `go vet`, usando cachés y temporales fuera del repositorio.
2. Repetir el preflight y los hashes inmediatamente antes del staging.
3. Autorizar expresamente el Momento B: creación de directorio, movimientos, manifiesto, staging y tres commits.

Durante la preparación no se modificó el índice, no se crearon commits y no se modificó `tasks.md`.

## B. Preflight

| Comprobación | Resultado |
|---|---|
| Repositorio | `/Users/german/Developer/ai-dev` |
| Rama | `feature/008-git-management` — correcta |
| HEAD | `a61ae2ffbb591a6282636805a2c35c81246b2d5c` — correcto |
| Rama `develop` | Apunta al mismo HEAD |
| Índice | Limpio |
| Archivos rastreados | Sin modificaciones |
| Commits propios de Feature 008 | Ninguno; `develop..HEAD` está vacío |
| Operaciones Git activas | Ninguna: merge, rebase, cherry-pick, revert y bisect ausentes |
| Archivos no ignorados de la línea base | Exactamente los 43 esperados |
| Archivos ignorados | `.idea/*` y `.specify/feature.json`; excluidos |
| `tasks.md` | Sin modificación; hash esperado |
| Paso 92 | Localizado en `/Users/german/Developer/STEP-92-BASELINE-ADOPTION-PLAN.md`; fuera del repositorio y del alcance |

No apareció ningún informe posterior al Paso 92 dentro del repositorio durante el preflight. Este informe del Paso 93 fue creado posteriormente por autorización específica y queda excluido de los tres commits previstos.

### Git Flow

- Rama de desarrollo: `develop`.
- Rama de producción: `master`.
- Base configurada para Feature 008: `develop`.
- Rama actual y `develop`: mismo HEAD.
- `master`: `7cf0ce17bef89a5e2f1508b4ec061ea31737614f`.

La referencia contextual anterior a `main` no coincide con la configuración real. Esto no bloquea los tres commits locales, pero debe resolverse antes de un futuro merge, PR o `git flow feature finish`.

## C. Inventario

### Grupo 1 — Especificación: 11 archivos

```text
specs/008-git-management/spec.md
specs/008-git-management/plan.md
specs/008-git-management/tasks.md
specs/008-git-management/research.md
specs/008-git-management/data-model.md
specs/008-git-management/quickstart.md
specs/008-git-management/checklists/requirements.md
specs/008-git-management/contracts/apply-strategy.md
specs/008-git-management/contracts/authorization-audit.md
specs/008-git-management/contracts/cli.md
specs/008-git-management/contracts/inspection-plan.md
```

### Grupo 2 — Código: 13 archivos

```text
internal/app/gitmanagement.go
internal/app/gitmanagement_test.go
internal/conventionalcommit/doc.go
internal/gitprocess/doc.go
internal/gitstore/doc.go
internal/gitmanager/doc.go
internal/gitmanager/ports.go
internal/gitmanager/boundary_test.go
internal/gitmanager/fakes_test.go
internal/gitmanager/testhelpers_test.go
internal/gitmanager/sentinel_pty_posix_test.go
internal/gitmanager/sentinel_pty_other_test.go
internal/gitmanager/testdata/hostile-git/README.txt
```

### Grupo 3 — Informes: 19 archivos

```text
STEP-73-REPORT.md
STEP-74-AUDIT-REPORT.md
STEP-75-REPORT.md
STEP-76A-AUDIT-REPORT.md
STEP-77-REPORT.md
STEP-78-AUDIT-REPORT.md
STEP-79-REPORT.md
STEP-80-AUDIT-REPORT.md
STEP-81-REPORT.md
STEP-82-AUDIT-REPORT.md
STEP-83-REPORT.md
STEP-84-AUDIT-REPORT.md
STEP-85-REPORT.md
STEP-86-AUDIT-REPORT.md
STEP-87-CLOSURE-PROPOSAL.md
STEP-88-GOVERNANCE-PROPOSAL.md
STEP-89-GOVERNANCE-DRAFT.md
STEP-90-DECISIONS-RECORD.md
STEP-91-GOVERNANCE-DECISIONS.md
```

La línea base consta de **43 archivos sin seguimiento**. El futuro manifiesto será un archivo adicional creado únicamente durante el Momento B. `STEP-92-BASELINE-ADOPTION-PLAN.md` y este informe quedan fuera de la línea base.

## D. Integridad

Los siete hashes de referencia coinciden:

| Archivo | SHA-256 |
|---|---|
| `tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `testhelpers_test.go` | `13af569ae6a032c31a7d5fe69b35d456758a10a3457f956ec0404da7e6aba213` |
| `sentinel_pty_posix_test.go` | `68e07ad0741b6131f3d242ac67748ef2aa782310bfcfab8ea31e5991d0e955c1` |
| `sentinel_pty_other_test.go` | `ca30e5029eb70cb737ecdbdb79426e5b09812bfae13d179052c567c19d09d4f0` |
| `boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

Los 19 hashes de informes coinciden íntegramente con la tabla del Paso 92.

## E. Compatibilidad con Spec Kit

La ruta propuesta es compatible:

```text
specs/008-git-management/history/
```

Comprobaciones:

- Spec Kit admite subdirectorios dentro de una feature; ya reconoce específicamente `contracts/` y `checklists/`.
- Los scripts de prerrequisitos descubren documentos mediante nombres fijos: `spec.md`, `plan.md`, `tasks.md`, `research.md`, `data-model.md`, `quickstart.md` y `contracts/`.
- No existe una búsqueda recursiva general sobre todos los Markdown de la feature.
- La revisión de checklists se limita a `FEATURE_DIR/checklists/`; no alcanza `history/`.
- `history/` no será incluido en `AVAILABLE_DOCS`.
- La ruta todavía no existe y no colisiona con un artefacto normativo conocido.
- Los nombres `STEP-*` no se confunden con archivos canónicos de Spec Kit.

Resultado: **sin colisión ni interpretación normativa automática identificada**.

## F. Validación del código

### Ejecutado durante Momento A

Se ejecutó `gofmt -l` sobre los 12 archivos Go del grupo de código. La salida fue vacía: todos están formateados.

No se utilizó `gofmt -w`.

### No ejecutado por falta de autorización

No se ejecutaron pruebas, `go vet`, descargas, instalación de dependencias ni procesos sintéticos.

Propuesta para una validación autorizada:

```bash
validation_root="$(mktemp -d /private/tmp/ai-dev-step93.XXXXXX)"
mkdir -p "$validation_root/gocache" "$validation_root/gotmp"

GOCACHE="$validation_root/gocache" \
GOTMPDIR="$validation_root/gotmp" \
go test -count=1 -timeout=60s \
  ./internal/app \
  ./internal/conventionalcommit \
  ./internal/gitmanager \
  ./internal/gitprocess \
  ./internal/gitstore

GOCACHE="$validation_root/gocache" \
GOTMPDIR="$validation_root/gotmp" \
go vet \
  ./internal/app \
  ./internal/conventionalcommit \
  ./internal/gitmanager \
  ./internal/gitprocess \
  ./internal/gitstore
```

Los tests de `gitmanager` crean únicamente repositorios, scripts y procesos sintéticos dentro de `t.TempDir()`. No deben operar sobre el repositorio principal. Después de la validación deberá confirmarse nuevamente que los archivos de la línea base y sus hashes no cambiaron.

## G. Plan exacto de commits

### Commit 1 — Especificación

```bash
git add -- \
  specs/008-git-management/spec.md \
  specs/008-git-management/plan.md \
  specs/008-git-management/tasks.md \
  specs/008-git-management/research.md \
  specs/008-git-management/data-model.md \
  specs/008-git-management/quickstart.md \
  specs/008-git-management/checklists/requirements.md \
  specs/008-git-management/contracts/apply-strategy.md \
  specs/008-git-management/contracts/authorization-audit.md \
  specs/008-git-management/contracts/cli.md \
  specs/008-git-management/contracts/inspection-plan.md

git commit -m "docs: add feature 008 git management specification"
```

Antes del commit se verificará que el índice contenga exactamente esas 11 rutas.

### Commit 2 — Código

```bash
git add -- \
  internal/app/gitmanagement.go \
  internal/app/gitmanagement_test.go \
  internal/conventionalcommit/doc.go \
  internal/gitprocess/doc.go \
  internal/gitstore/doc.go \
  internal/gitmanager/doc.go \
  internal/gitmanager/ports.go \
  internal/gitmanager/boundary_test.go \
  internal/gitmanager/fakes_test.go \
  internal/gitmanager/testhelpers_test.go \
  internal/gitmanager/sentinel_pty_posix_test.go \
  internal/gitmanager/sentinel_pty_other_test.go \
  internal/gitmanager/testdata/hostile-git/README.txt

git commit -m "feat: add feature 008 git management phase 1 foundation"
```

Antes del commit se verificará que el índice contenga exactamente esas 13 rutas. No se cambiarán fakes ni se implementarán F-03/F-04.

### Commit 3 — Archivo histórico

Primero se creará `history/` y se moverán los 19 archivos usando nombres explícitos. No se usará `STEP-*.md` para efectuar el movimiento.

Después se creará `SHA256SUMS`, se comprobarán sus 19 entradas y se hará staging explícito de las 20 rutas de destino: 19 informes más el manifiesto.

Mensaje:

```bash
git commit -m "docs: archive feature 008 phase 1 audit and governance reports"
```

En ningún punto se utilizarán `git add .`, `git add -A` ni staging indiscriminado de un directorio.

## H. Manifiesto propuesto

Contenido exacto, sin escribirlo todavía en `history/`:

```text
1317388847eb0277aa7b5925a7172ab96578e4428f71763ed519bd4cf1c4086d  STEP-73-REPORT.md
d264076489afa1506abe5a86712ab5622af123807f140bda3805ac2f0e1f3c43  STEP-74-AUDIT-REPORT.md
097f887f775c6d7945ec0f8323e49e6b182de727730e78f510f2c7b15f9022f1  STEP-75-REPORT.md
03b3294ac0828aaa636e2f360be78a37e3a2b474a0bf68757344fe44b27d293e  STEP-76A-AUDIT-REPORT.md
a4c4f06eb2ca2a35c5d5800e734b4a2f553f8ff2a4a8288f3325d03401532de3  STEP-77-REPORT.md
499d7d4f596c6a286d41638288ec1f77f976ac1018102aab3781f40ca4a0279d  STEP-78-AUDIT-REPORT.md
b0542386a0d75859498df1ae03af11b899ecc9621905166d3ba1e97bb98e8a4d  STEP-79-REPORT.md
066b36fc8ad68481fd8515a672e93f8c093a5a8c039395279ce3f455b102cd48  STEP-80-AUDIT-REPORT.md
24d88d1f7157c9c3550de4f94e92f939ee5cf295bfc2d9ee1507aef5a65cea6b  STEP-81-REPORT.md
8517cd99b2e1139c18954b8995d94aba2f4c9c559f369263964e70b85b29cc95  STEP-82-AUDIT-REPORT.md
2b95ba8fd39361898e5f6d58f4f4df1c33bb011b425058402bd21f22426ab065  STEP-83-REPORT.md
9c90529d0fd446b14aa88b213b9b88d8b8c07ffe1b80b1c0e1ee20f2bfddab03  STEP-84-AUDIT-REPORT.md
f3be35927658555e377371ab06521ea44dc6fd7d3bd95fc558a730398b6f53e9  STEP-85-REPORT.md
cba1369c6f79f57915963021054c107801b58786b8c029ef94125cb353a3ba90  STEP-86-AUDIT-REPORT.md
529735b62547a0353491a322f5141d933dfc33f5b29e98dc15e082262d174a3a  STEP-87-CLOSURE-PROPOSAL.md
7f89525a896ec269149cb11996cfc657b1a1fcccbce3252b5e6edbcc3845f4ec  STEP-88-GOVERNANCE-PROPOSAL.md
9bb007c7b9bb1ff30ca75c1f29a2dcca4b1071f61a1f6e8722f3132a66ee31f7  STEP-89-GOVERNANCE-DRAFT.md
0a0c50121fc3f5502f7cc9b19cb1dc3f745b0f5418c22d259475b7f91a9f22d1  STEP-90-DECISIONS-RECORD.md
da7f5ca2015be0aae7d15b67723baa50a16cc4313acdd59dd43a667abe286f68  STEP-91-GOVERNANCE-DECISIONS.md
```

Verificación prevista después del movimiento y después del commit:

```bash
cd specs/008-git-management/history
shasum -a 256 -c SHA256SUMS
```

Resultado esperado: 19 entradas `OK`.

## I. Riesgos y condiciones

- La búsqueda focalizada de secretos en los 19 informes no encontró credenciales reales, tokens, claves privadas ni contraseñas. Las coincidencias fueron texto de gobernanza.
- Existen rutas locales, incluida una referencia a una sesión bajo `~/.codex`; su conservación fue aprobada expresamente.
- `STEP-92-BASELINE-ADOPTION-PLAN.md` permanece fuera del repositorio y queda excluido.
- `STEP-93-PREPARATION-REPORT.md` es posterior al inventario aprobado y queda excluido de los tres commits.
- Los tres conjuntos de staging son disjuntos.
- Ninguno de los 43 archivos de la línea base está ignorado.
- No se incluirán `.idea/*`, `.specify/feature.json` ni archivos ajenos a Feature 008.
- La discrepancia `main` frente a `develop`/`master` deberá atenderse antes de finalizar Git Flow, no antes de estos commits locales.
- Cualquier cambio de rama, HEAD, inventario o hash antes de ejecutar obliga a detener el procedimiento.

## J. Autorización requerida

Una instrucción humana posterior deberá autorizar concretamente:

1. Ejecutar `go test` y `go vet` con temporales fuera del repositorio.
2. Crear `specs/008-git-management/history/`.
3. Mover los 19 informes enumerados, por rutas explícitas.
4. Crear y verificar `SHA256SUMS`.
5. Ejecutar staging explícito y crear los tres commits, en ese orden.
6. Repetir las verificaciones posteriores a cada commit.

## STOP

No se crearon directorios históricos, manifiestos, movimientos, staging ni commits. No se modificó `tasks.md`. Fase 1 sigue abierta y Fase 2 no fue iniciada.
