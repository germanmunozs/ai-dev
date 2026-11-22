# Informe del Paso 89 — Borrador de acta de decisiones de gobernanza (Feature 008, Fase 1)

Fecha: 2026-10-26
Modo: preparación de borrador, read-only. Ningún archivo del proyecto fue modificado durante la elaboración. Este archivo se creó después, a petición expresa del usuario. **Es un borrador para revisión humana: no aprueba, no autoriza y no ejecuta ninguna decisión.**

## A. Resumen ejecutivo

Se preparó el acta `FEATURE-008-GOVERNANCE-DRAFT-01` a partir de los Pasos 86, 87 y 88. Está lista para que el responsable apruebe, rechace o modifique cada decisión por separado.

- **Contenido:** 5 contratos (S-01 a S-05), 4 decisiones de deuda (D-01 a D-04), 2 decisiones de arquitectura (ARCH-F03 y ARCH-F04), 2 desviaciones (DEV-T003 y DEV-T004), la política de versionamiento (BASELINE-008), el procedimiento de adopción (BASELINE-PROCEDURE-008) y el registro de cierre (CLOSURE-008-PHASE1).
- **Estado de todo:** PENDIENTE DE APROBACIÓN HUMANA. No se aprobó nada.
- **Aprobar el acta no ejecuta nada.** La recomendación técnica, la decisión propuesta, la decisión aprobada, la autorización de implementación, la adopción de la línea base, el cierre formal y la autorización de la Fase 2 son siete actos distintos, y ninguno se infiere de otro.
- **Dato nuevo:** el inventario real era de **40** archivos sin seguimiento, y los informes `STEP-*` eran **16**, no 14. A los 14 históricos se sumaron `STEP-87-CLOSURE-PROPOSAL.md` y `STEP-88-GOVERNANCE-PROPOSAL.md`. Esto ajusta la cifra "14" del Paso 88. Con este archivo, el inventario pasa a 41 y los informes a 17.

**Veredicto: READY FOR HUMAN APPROVAL.**

## B. Preflight

| Verificación | Resultado |
|---|---|
| Rama / HEAD | `feature/008-git-management` / `a61ae2ffbb591a6282636805a2c35c81246b2d5c` |
| Commits de Feature 008 | 0 (`a61ae2f..HEAD` vacío) |
| Índice y diff rastreado | Vacíos |
| Archivos sin seguimiento (al verificar) | **40** = 11 especificación + 13 código + 16 informes |
| Hashes | Los siete archivos de referencia coinciden |
| `ports.go` y `gitmanagement.go` | Hashes sin cambios (`e040a07a…`, `68049579…`) |
| `go.sum` | No existe; `go.mod` sin cambios |
| Manifiesto SHA-256 del repositorio | Idéntico al del Paso 88, salvo el archivo del propio Paso 88 |
| `tasks.md` | 0 de 118 casillas marcadas |
| Fase 2 | Sin implementación (solo `doc.go` en `gitprocess`, `gitstore`, `conventionalcommit`) |
| Ejecución | Solo lecturas, uid 501 |

### Hashes de referencia verificados

| Archivo | SHA-256 |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `13af569ae6a032c31a7d5fe69b35d456758a10a3457f956ec0404da7e6aba213` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `68e07ad0741b6131f3d242ac67748ef2aa782310bfcfab8ea31e5991d0e955c1` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `ca30e5029eb70cb737ecdbdb79426e5b09812bfae13d179052c567c19d09d4f0` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

## C. Borrador del acta formal

> # ACTA PROPUESTA DE DECISIONES DE GOBERNANZA — FEATURE 008 / FASE 1
>
> | Campo | Valor |
> |---|---|
> | Identificador | `FEATURE-008-GOVERNANCE-DRAFT-01` |
> | Fecha de elaboración | 2026-10-26 |
> | Responsable de aprobación | Responsable del proyecto |
> | **Estado** | **BORRADOR — NO APROBADO** |
> | Alcance | Decisiones previas al cierre formal de la Fase 1 |
> | Referencias | Informes de los Pasos 86, 87 y 88 |
> | Rama / HEAD verificados | `feature/008-git-management` / `a61ae2ffbb591a6282636805a2c35c81246b2d5c` |
> | Línea base | Verificada por contenido y hashes; **no versionada** |
>
> **Declaración.** Este documento no constituye autorización de ejecución. No acepta contratos ni deuda, no elige alternativas arquitectónicas, no autoriza modificar código ni normativos, no adopta la línea base, no cierra la Fase 1 y no autoriza la Fase 2. Cada decisión se aprueba, rechaza o modifica de forma individual. No hay firmas ni fechas de aprobación.

## D. Contratos S-01 a S-05

Estado de todos: **PENDIENTE DE APROBACIÓN.** Aprobarlos no autoriza modificar código.

### S-01. Ciclo de vida del proceso

- **Texto propuesto:** "En el harness de pruebas de PTY no existe un reaper externo y concurrente que recolecte los procesos sintéticos sin pasar por `os.Process`. El PGID se captura inmediatamente después de `Start`. Ningún proceso se recolecta antes de señalizar su grupo. `os.Process.Signal(0)` se usa como probe de ciclo de vida dentro de este contrato. El alcance es exclusivamente la infraestructura de pruebas."
- **Justificación técnica:** el probe distingue estados de `os.Process` (`Wait()`, `Release()`) sin enviar señales a un PID liberado.
- **Evidencia:** procesos reales en macOS: vivo y zombie aceptados; tras `Wait()`, `Release()` y `wait4` crudo rechazados. 10 mutaciones del capturador detectadas.
- **Limitaciones:** el probe en macOS se basa en PID. Si un reaper externo recolectara al hijo y el PID se reutilizara, `kill(pid, 0)` tendría éxito sobre un proceso ajeno y el helper aceptaría una identidad errónea. En Linux, `os.Process` usa pidfd y es más fuerte, pero no se ejecutó.
- **Riesgo residual:** garantía por orden de llamadas, no por construcción.
- **Alternativas:** rechazar; aceptar con modificaciones; exigir una verificación adicional por plataforma.
- **Efecto de aprobar:** se acepta la limitación como contrato del harness.
- **Efecto de rechazar:** el capturador de PGID debe rediseñarse antes del cierre.
- **Estado:** PENDIENTE DE APROBACIÓN.

### S-02. Reserva POSIX de PID/PGID

- **Texto propuesto:** "Un líder no recolectado conserva el PID y el PGID necesarios para la señalización. La señal de grupo precede a `wait4` y `Release`. No se permite señalización posterior a la recolección. La evidencia nativa corresponde a macOS/arm64. La compilación cruzada no equivale a pruebas nativas."
- **Justificación técnica:** un zombie sin recolectar mantiene reservado su PID, y por tanto su PGID.
- **Evidencia:** el código recolecta solo con el lector PTY cerrado; los tests de timeout lo ejercen. El registro de destinos mostró 0 objetivos inseguros.
- **Limitaciones:** supuesto POSIX no probado en Linux. FreeBSD y Windows usan el stub no POSIX.
- **Riesgo residual:** comportamiento no verificado en otras plataformas.
- **Alternativas:** rechazar; aceptar y exigir evidencia nativa en Linux antes de la evidencia de plataforma (Fase 12).
- **Efecto de aprobar:** se acepta el supuesto con evidencia solo en macOS.
- **Efecto de rechazar:** se requiere ejecutar los tests en Linux antes del cierre.
- **Estado:** PENDIENTE DE APROBACIÓN.

### S-03. Aislamiento de señales

- **Texto propuesto:** "Se rechaza `groupID <= 1` antes de cualquier `kill`. Las señales reales se dirigen únicamente a procesos y grupos sintéticos propios. Las pruebas negativas usan callbacks inertes. Ninguna señal alcanza grupos ajenos. N-20 queda vinculada como deuda."
- **Justificación técnica:** el guard se aplica en un seam separado de la llamada real a `kill`.
- **Evidencia:** eliminar el guard hace fallar la prueba con el callback inerte, sin ninguna señal real.
- **Limitaciones:** la función real acepta cualquier `groupID > 1` (N-20).
- **Riesgo residual:** un llamador futuro podría pasar un grupo distinto del capturado.
- **Alternativas:** rechazar; exigir que la función rechace el grupo del propio proceso de test.
- **Efecto de aprobar:** N-20 queda aceptada como deuda de este contrato.
- **Efecto de rechazar:** N-20 debe corregirse antes del cierre.
- **Estado:** PENDIENTE DE APROBACIÓN.

### S-04. Límites temporales

- **Texto propuesto:** "La supervisión del pager tiene un timeout de 10 segundos y la limpieza otro de 2 segundos. No hay goroutines lectoras bloqueadas ni procesos residuales. La salida del pager de prueba queda limitada por N-16."
- **Justificación técnica:** `select(2)` antes de `read(2)`, `wait4(WNOHANG)` y plazos con temporizadores monótonos.
- **Evidencia:** 2→2 goroutines y 5→5 descriptores en los cuatro escenarios; 0 procesos residuales.
- **Limitaciones:** caudal de unos 200 KB/s. Más de ~2 MiB en 10 s da un falso timeout.
- **Riesgo residual:** un pager de prueba con salida grande falla por timeout.
- **Alternativas:** rechazar; aceptar con otro límite.
- **Efecto de aprobar:** se acepta el límite práctico.
- **Efecto de rechazar:** hay que mejorar el lector PTY.
- **Estado:** PENDIENTE DE APROBACIÓN.

### S-05. Infraestructura inerte

- **Texto propuesto:** "La Fase 1 no tiene runner productivo ni adaptadores productivos, no ejecuta planes automáticamente y no autoriza mutaciones Git."
- **Justificación técnica:** los únicos archivos no test son `ports.go`, `gitmanagement.go` y los `doc.go`.
- **Evidencia:** `os/exec` solo aparece en `testhelpers_test.go` y en cadenas de `boundary_test.go`. `internal/app` no cablea las dependencias.
- **Limitaciones:** ninguna relevante en Fase 1.
- **Riesgo residual:** ninguno conocido.
- **Alternativas:** rechazar.
- **Efecto de aprobar:** se confirma el carácter inerte de la Fase 1.
- **Efecto de rechazar:** revisar el alcance de la Fase 1.
- **Estado:** PENDIENTE DE APROBACIÓN.

## E. Decisiones D-01 a D-04

Estado de todas: **PENDIENTE DE APROBACIÓN.**

### D-01. Deuda informativa y del harness (aceptación temporal)

| ID | Sev. | Condición particular | Revisión |
|---|---|---|---|
| N-05 | INFO | Revisar si CI no dispone de PTY | Fase 12 |
| N-06 | INFO | — | Fase 12 |
| N-07 | INFO | Revisar cuando exista ejecución nativa en Linux | Evidencia de plataforma |
| N-08 | INFO | — | Fase 12 |
| N-11 | INFO | — | Refactor del helper |
| N-12 | INFO | — | Evidencia de plataforma |
| N-15 | LOW | Mantener `select` antes de `read` | Refactor del lector PTY |
| N-16 | LOW | Limitar la salida de los pagers de prueba | Refactor del lector PTY |
| N-17 | INFO | — | Refactor |
| N-20 | INFO | Señalizar únicamente el PGID capturado (ligada a S-03) | Cambios en la señalización |
| N-21 | INFO | — | — |
| N-22 | INFO | — | — |
| N-27 | INFO | Sujeta a S-01 | Igual que S-01 |
| N-28 | INFO | — | — |

La aceptación temporal no equivale a resolución definitiva.

### D-02. N-03 (LOW)

Aceptación temporal con seguimiento. Se revisa cuando se modifique el detector arquitectónico, junto con N-04. Al hacerlo, se añade una prueba propia de la comprobación indirecta.

### D-03. N-04 (LOW)

**Resolver antes de incorporar adaptadores reales** en `gitprocess` y `gitstore`.

- **Cobertura requerida:** el detector debe cubrir mecanismos indirectos de ejecución (`syscall.Syscall(SYS_EXECVE…)`, `plugin`).
- **Lista de imports:** definir una lista explícita de imports permitidos para los adaptadores.
- **Riesgo de bypass:** hoy los adaptadores pueden importar `report` o `gatestore`, o importarse entre sí.
- **Dependencias:** T016 y T020.
- **Verificación antes de desbloquear:** matriz de mutaciones ampliada que detecte esos casos, y prueba que rechace imports no permitidos en los adaptadores.
- **No se implementa en este paso.**

### D-04. F-07 a F-10

- **F-07:** revisión en Fase 12 (servicio "no disponible" explícito).
- **F-08:** revisión en Fase 12 (test de que `ai-dev git …` sigue sin reconocerse).
- **F-09:** aceptación temporal.
- **F-10:** resolver mediante la política de versionamiento (BASELINE-008).

Ninguna se afirma corregida.

## F. Decisión ARCH-F03

- **Estado:** PROPUESTA, PENDIENTE DE APROBACIÓN.
- **Propuesta:** adoptar conceptualmente un descriptor cerrado para `GitInvocation`.
- **Contrato conceptual:**
  - Operación Git tipada, de un conjunto cerrado.
  - `GitToolIdentity` congelado.
  - Directorio físico explícito.
  - Perfil de entorno controlado.
  - Stdin acotado y límites de salida.
  - Argv y entorno los construye únicamente el runner, sin shell.
  - Ni `Executable`, ni `Args`, ni `Environment` arbitrarios aportados por el llamador.
- **Tres clases de operación:**
  1. Lectura.
  2. Plumbing de objetos autorizado.
  3. Mutación.
- **Mutaciones:** el descriptor no constituye autorización. Se exige autorización humana durable, el plan canónico, las comprobaciones del Safety Kernel, el CAS y la auditoría previa. La credencial futura de ejecución no puede ser una vía alternativa para eludir esas garantías.
- **Dependencias:** T008 (tipos), contrato cerrado aprobado antes de T016, e implementación y pruebas en T016–T019.
- **Criterios de aceptación:**
  - `ports.go` no contiene `Executable` ni `Environment` aportados por el llamador.
  - Las pruebas de T016 fallan si un llamador aporta entorno o argv libre.
  - Las operaciones mutantes son rechazadas sin la credencial emitida por el Safety Kernel.
- **Riesgos de implementación:** acoplamiento del puerto al enum de la matriz P-04, y retrabajo de `fakes_test.go` y de las aserciones de interfaz.
- **No elegida ni implementada.**

## G. Decisión ARCH-F04

- **Estado:** PROPUESTA, PENDIENTE DE APROBACIÓN.
- **Propuesta:** adoptar conceptualmente descriptores tipados y capacidades explícitas para `GitStore`, `Filesystem` y `Reporter`.
- **Requisitos:**
  - Separación entre lectura y mutación.
  - Operaciones tipadas, sin `Execute` genérico con `Operation string`.
  - Identidad física de objetos y rutas.
  - Protección contra traversal y symlink bypass.
  - Operaciones de filesystem basadas en descriptores.
  - CAS por versión y propietario.
  - Auditoría append-only.
  - `Reporter` desacoplado del renderizado.
  - **Una única definición del puerto `Filesystem`** (reconciliar `ports.go` con `filesystem.go` de T021).
  - Mutaciones exclusivamente bajo autorización durable.
- **Aclaración:** poseer un descriptor o una credencial técnica no equivale a aprobación humana.
- **Dependencias:** T008 (tipos y contratos), T021 (filesystem), T022–T023 (registros y auditoría). La resolución debe preceder a la construcción de los adaptadores correspondientes.
- **Criterios de aceptación:**
  - No queda ningún método genérico `Execute`.
  - Hay una sola definición de `Filesystem`.
  - Los tests de T021–T023 pasan sin rutas libres.
  - Sin credencial, ninguna mutación es posible.
- **No elegida ni implementada.**

## H. Decisiones DEV-T003 y DEV-T004

### DEV-T003

- **Propuesta:** aceptar una excepción limitada. Solo `internal/gitmanager/testhelpers_test.go` puede importar `os/exec`, exclusivamente para el harness de sentinelas.
- **La excepción:** no aplica a código productivo, no se extiende a otros archivos, no autoriza procesos arbitrarios y no debilita la prohibición de imports productivos.
- **Registro:** debe quedar explícita en la futura documentación de cierre.
- **`tasks.md`:** decidir mediante una autorización posterior si se actualiza.
- **Riesgo:** la excepción es por nombre de archivo (relacionado con N-04).
- **Estado:** PENDIENTE DE APROBACIÓN.

### DEV-T004

- **Propuesta:** aceptar `sentinel_pty_posix_test.go` y `sentinel_pty_other_test.go` como parte del harness, en su ubicación actual.
- **Registro:** su inclusión fue autorizada en el Paso 75.
- **Inventario normativo:** decidir posteriormente si se actualiza.
- **Riesgo:** el inventario de T004 queda incompleto hasta entonces.
- **Estado:** PENDIENTE DE APROBACIÓN.

## I. Decisión BASELINE-008

- **Estado:** PROPUESTA, PENDIENTE DE APROBACIÓN.

### Grupo 1: especificación (11 archivos)

Commit documental independiente, con contenido y hashes preservados. No se modifica `tasks.md` en ese commit.

### Grupo 2: código de Fase 1 (13 archivos)

Commit independiente, sin modificar el código. Sin cambios de F-03 ni F-04. Infraestructura inerte.

### Grupo 3: informes (16 archivos al momento de la verificación)

Se propone la **alternativa B del Paso 88**:

- Preservar todos los informes íntegros.
- Ubicarlos en un subdirectorio histórico de la feature.
- Incorporar un manifiesto SHA-256.
- Usar un commit documental separado.
- No conservar duplicados innecesarios en la raíz tras la futura migración.

**Advertencia.** Mover los informes cambia sus rutas, y el manifiesto es un archivo nuevo. Ambos requieren autorización posterior explícita.

### Rutas locales

Ocho informes contienen rutas absolutas locales, y `STEP-76A-AUDIT-REPORT.md` cita además una ruta de sesión local. Se propone conservarlas para mantener fidelidad y hashes, pero la decisión es del responsable. No son obligatorias de conservar y no se redactan automáticamente.

### Inventario

El número de archivos sin seguimiento debe actualizarse antes de cualquier staging (40 al verificar; 41 con este archivo). No se usa `git add .` ni comodines indiscriminados.

## J. Procedimiento BASELINE-PROCEDURE-008

Ejecución futura, sujeta a autorización separada.

1. Aprobar el acta.
2. Verificar rama y HEAD.
3. Verificar índice y diff.
4. Inventariar archivos.
5. Verificar hashes.
6. Revisar contenido sensible.
7. Confirmar rutas exactas.
8. Versionar la especificación.
9. Versionar el código.
10. Preservar y versionar los informes.
11. Registrar commits y manifiesto.
12. Verificar la integridad posterior.
13. Preparar el registro formal de cierre.
14. Solicitar autorización separada para cerrar la Fase 1.
15. Detenerse antes de la Fase 2.

**La aprobación del acta no autoriza ejecutar esta secuencia.** No se ejecutó staging, commits ni movimientos.

## K. Propuesta CLOSURE-008-PHASE1

Procedimiento posterior, no ejecutado:

- Registrar fecha y responsable.
- Referenciar hashes y commits de la línea base.
- Registrar la aceptación de contratos y la deuda temporal.
- Registrar las decisiones F-03 y F-04 y las desviaciones T003/T004.
- Actualizar los normativos solo con autorización expresa. En Feature 006 se usó una nota de sesión fechada; esto cambia los hashes de `tasks.md` y otros normativos, así que debe declararse de antemano la nueva referencia de hashes.
- Evaluar el marcado de T001–T006.
- Declarar la Fase 1 cerrada solo con aprobación explícita.

Aprobar este borrador no cierra la fase. No se declaró el cierre.

## L. Matriz consolidada de decisiones

| ID | Decisión | Recomendación | Riesgo | Dependencia | Estado |
|---|---|---|---|---|---|
| S-01 | Contrato de ciclo de vida | Aceptar | PID reutilizado en macOS | — | PENDIENTE DE APROBACIÓN HUMANA |
| S-02 | Reserva POSIX | Aceptar | No probado en Linux | S-01 | PENDIENTE DE APROBACIÓN HUMANA |
| S-03 | Aislamiento de señales | Aceptar | N-20 | — | PENDIENTE DE APROBACIÓN HUMANA |
| S-04 | Límites temporales | Aceptar | N-16 | — | PENDIENTE DE APROBACIÓN HUMANA |
| S-05 | Infraestructura inerte | Aceptar | Ninguno conocido | — | PENDIENTE DE APROBACIÓN HUMANA |
| D-01 | Deuda informativa y del harness | Aceptación temporal | Bajo | S-01, S-03, S-04 | PENDIENTE DE APROBACIÓN HUMANA |
| D-02 | N-03 | Aceptación temporal con seguimiento | Bajo | N-04 | PENDIENTE DE APROBACIÓN HUMANA |
| D-03 | N-04 | Resolver antes de adaptadores reales | Bypass arquitectónico | T016, T020 | PENDIENTE DE APROBACIÓN HUMANA |
| D-04 | F-07 a F-10 | Revisar en Fase 12 / aceptar / versionamiento | Bajo | Fase 12, BASELINE-008 | PENDIENTE DE APROBACIÓN HUMANA |
| ARCH-F03 | Descriptor cerrado de `GitInvocation` | Alternativa 2 | Retrabajo de puertos y fakes | T008, antes de T016 | PENDIENTE DE APROBACIÓN HUMANA |
| ARCH-F04 | Puertos tipados y capacidades | Alternativa 3 | Rediseño mayor | T008, T021–T023 | PENDIENTE DE APROBACIÓN HUMANA |
| DEV-T003 | Excepción de `os/exec` en el harness | Aceptar y registrar | Excepción por nombre de archivo | N-04 | PENDIENTE DE APROBACIÓN HUMANA |
| DEV-T004 | Helpers PTY como parte del harness | Aceptar y registrar | Inventario incompleto | — | PENDIENTE DE APROBACIÓN HUMANA |
| BASELINE-008 | Política de versionamiento | Tres grupos; informes por alternativa B | Pérdida de evidencia si se omite | Aprobación del acta | PENDIENTE DE APROBACIÓN HUMANA |
| BASELINE-PROCEDURE-008 | Procedimiento de adopción | Secuencia de 15 pasos | Ejecución sin autorización separada | BASELINE-008 | PENDIENTE DE APROBACIÓN HUMANA |
| CLOSURE-008-PHASE1 | Registro formal de cierre | Procedimiento posterior | Cambia hashes normativos | Línea base versionada | PENDIENTE DE APROBACIÓN HUMANA |

El inicio de la Fase 2 no figura en esta matriz: es una decisión separada y posterior, **no autorizada**.

## M. Criterios A-01 a A-12

| Criterio | Estado |
|---|---|
| A-01 Contratos completos, sin ocultar limitaciones | Preparado en el borrador; pendiente de aprobación |
| A-02 Deuda clasificada sin perder identificadores | Preparado; pendiente de aprobación |
| A-03 N-04 con bloqueo explícito antes de adaptadores | Preparado; pendiente de aprobación |
| A-04 F-03 con contrato cerrado antes de T016 | Preparado; pendiente de aprobación |
| A-05 F-04 con capacidades tipadas antes de T021 | Preparado; pendiente de aprobación |
| A-06 T003/T004 con excepciones limitadas y trazables | Preparado; pendiente de aprobación |
| A-07 Versionamiento que preserva la evidencia | Preparado; pendiente de aprobación |
| A-08 Rutas locales sometidas a decisión humana | Preparado; pendiente de aprobación |
| A-09 Adopción de línea base con autorización separada | Preparado; pendiente de aprobación |
| A-10 Cierre formal con autorización separada | Preparado; pendiente de aprobación |
| A-11 Fase 2 no autorizada | **Verificado** |
| A-12 Ninguna mutación ejecutada | **Verificado** (manifiesto idéntico; sin operaciones Git mutantes) |

Ninguno está incumplido. No se marca como aprobado ningún criterio que dependa de una decisión humana aún no emitida.

## N. Riesgos residuales

- Mientras no se versione, el trabajo de Fase 1 no tiene copia en Git.
- S-01 y S-02 dependen de supuestos de sistema operativo; la ejecución nativa se probó solo en macOS/arm64.
- Si F-03 y F-04 no se deciden antes de T016 y T021, los adaptadores podrían consolidar los puertos genéricos.
- La excepción de T003 es por nombre de archivo y depende de N-04.
- Registrar el cierre cambia el hash de `tasks.md` y otros normativos.
- Los informes contienen rutas locales; redactarlas cambiaría sus hashes.
- El inventario cambia con cada informe nuevo y debe fijarse antes de cualquier staging.
- Trazabilidad incompleta: N-24 no existe y no hay informe del Paso 72.

## O. Recomendación final

Presentar el acta al responsable y que decida cada punto por separado, en este orden:

1. Contratos S-01 a S-05 y deuda D-01 a D-04.
2. ARCH-F03 y ARCH-F04, porque bloquean T016 y T021.
3. DEV-T003 y DEV-T004.
4. BASELINE-008, incluida la decisión sobre las rutas locales.
5. BASELINE-PROCEDURE-008 y CLOSURE-008-PHASE1.

La aprobación del acta no autoriza implementar F-03 o F-04, versionar la línea base, cerrar la Fase 1 ni iniciar la Fase 2. Cada uno de esos actos exige una autorización expresa y separada.

**Veredicto: READY FOR HUMAN APPROVAL.** No implica aprobación del acta.

---

Este documento es un borrador. No aprueba decisiones, no modifica archivos, no adopta la línea base, no mueve informes, no cierra la Fase 1 y no autoriza la Fase 2, la modificación de `tasks.md` ni la creación de commits.
