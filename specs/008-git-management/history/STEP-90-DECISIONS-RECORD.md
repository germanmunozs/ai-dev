# Informe del Paso 90 — Registro de decisiones humanas sobre el acta FEATURE-008-GOVERNANCE-DRAFT-01

Fecha: 2026-10-26
Modo: revisión de gobernanza, read-only. Ningún archivo del proyecto fue modificado durante el paso. Este archivo se creó después, a petición expresa del usuario.

**Naturaleza de este documento.** Es un registro de las respuestas que el responsable dio durante la conversación del Paso 90, bloque por bloque. **No es el acta definitiva ni una acta firmada.** No contiene firmas ni fechas de aprobación formal. Las aprobaciones aquí registradas **no autorizan ejecutar ninguna acción**: ni modificar código, ni cambiar `ports.go` o los fakes, ni modificar `tasks.md`, ni versionar la línea base, ni cerrar la Fase 1, ni iniciar la Fase 2.

## A. Estado inicial verificado

- **Rama y HEAD:** `feature/008-git-management`, `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- **Git:** índice y diff vacíos. 41 archivos sin seguimiento = 11 de especificación + 13 de código + 17 informes `STEP-*` (verificado con lecturas durante el paso).
- **Hashes:** `tasks.md`, `spec.md` y los cinco archivos de código de referencia coinciden con los esperados.
- **`tasks.md`:** 0 de 118 casillas marcadas.
- **Línea base:** verificada por contenido y hashes; **no versionada**.
- **Fase 1:** abierta. **Fase 2:** no autorizada.
- **Acta de referencia:** `FEATURE-008-GOVERNANCE-DRAFT-01` (Paso 89), estado inicial de todas sus decisiones: PENDIENTE.

## B. Método

Las decisiones se presentaron en cinco bloques, sin avanzar al siguiente hasta recibir respuesta. Cada respuesta del responsable fue la palabra **"aprobar"**. Se interpretó, en cada bloque, como aprobación expresa de las decisiones tal como se presentaron, con las condiciones listadas. La excepción fue BASELINE-LOCAL-PATHS, que pedía elegir entre A, B o C: allí un "aprobar" no indica una opción y se registró como REQUIERE ACLARACIÓN, tal como se advirtió antes de la respuesta.

## C. Bloque I — Contratos de seguridad (respuesta: "aprobar")

| ID | Decisión | Condiciones aprobadas | Estado |
|---|---|---|---|
| S-01 | Ciclo de vida del proceso | Sin reaper externo concurrente sobre los procesos sintéticos del harness; PGID capturado inmediatamente después de `Start`; prohibido recolectar antes de señalizar el grupo; probe `os.Process.Signal(0)`; alcance exclusivo de infraestructura de pruebas; riesgo residual de reutilización de PID en macOS | APROBADA EXPRESAMENTE |
| S-02 | Reserva POSIX de PID/PGID | Señal de grupo antes de `wait4`/`Release`; ninguna señal posterior a la recolección; evidencia nativa solo macOS/arm64; sin asumir validación nativa en Linux | APROBADA EXPRESAMENTE |
| S-03 | Aislamiento de señales | Rechazo de `groupID <= 1`; señales reales solo a procesos y grupos sintéticos propios; callbacks inertes en pruebas negativas; N-20 como deuda temporal vinculada; prohibido ampliar a grupos arbitrarios | APROBADA EXPRESAMENTE |
| S-04 | Límites temporales | Supervisión de 10 s; limpieza de 2 s; sin procesos residuales ni goroutines lectoras bloqueadas; límite práctico de salida del pager por N-16 | APROBADA EXPRESAMENTE |
| S-05 | Infraestructura inerte | Sin runner productivo, adaptadores productivos, ejecución automática de planes ni mutaciones Git habilitadas | APROBADA EXPRESAMENTE |

## D. Bloque II — Deuda técnica (respuesta: "aprobar")

| ID | Decisión | Condiciones aprobadas | Estado |
|---|---|---|---|
| D-01 | Aceptación temporal del grupo completo: N-05, N-06, N-07, N-08, N-11, N-12, N-15, N-16, N-17, N-20, N-21, N-22, N-27, N-28 | N-15: conservar `select` antes de `read`. N-16: limitar la salida de pagers de prueba. N-20: usar solo el PGID capturado. N-27: vinculada a S-01. N-07: revisar con evidencia nativa en Linux. La aceptación temporal no es resolución definitiva. | APROBADA EXPRESAMENTE |
| D-02 | Aceptación temporal de N-03 | Añadir prueba propia de la comprobación indirecta cuando se modifique el detector, junto con N-04 | APROBADA EXPRESAMENTE |
| D-03 | N-04 como condición obligatoria antes de incorporar adaptadores reales en `gitprocess` y `gitstore` | Resolver antes de T016 y T020: detección de ejecución indirecta, restricciones de imports, matriz de mutaciones ampliada, pruebas que rechacen dependencias prohibidas. **No implementa N-04 ni desbloquea T016/T020.** | APROBADA EXPRESAMENTE |
| D-04 | F-07, F-08: revisión en Fase 12. F-09: aceptación temporal. F-10: resolver mediante la política de versionamiento | Ninguna queda corregida | APROBADA EXPRESAMENTE |

## E. Bloque III — Decisiones arquitectónicas (respuesta: "aprobar")

Aprobación **conceptual**. No autoriza cambiar `ports.go`, modificar los fakes, implementar T008, implementar T016–T023, ni iniciar la Fase 2.

### ARCH-F03 — Alternativa 2: descriptor cerrado para `GitInvocation`

- Operaciones Git tipadas, de un conjunto cerrado (matriz P-04).
- `GitToolIdentity` congelado; directorio físico explícito; perfil de entorno controlado (`inspect`, `plan`, `apply`, `object`); stdin y límites de salida acotados.
- Argv y entorno construidos únicamente por el runner; sin shell; sin `Executable`, `Args` ni `Environment` arbitrarios aportados por el llamador.
- Tres clases: lectura, plumbing de objetos autorizado y mutación.
- **El descriptor no equivale a autorización.** Las mutaciones exigen autorización humana durable, plan canónico, comprobaciones del Safety Kernel, CAS y auditoría previa. La credencial técnica futura no puede ser una vía para eludir esas garantías.
- Dependencias: T008 (tipos); contrato aprobado antes de T016; implementación y pruebas en T016–T019.
- **Estado:** APROBADA EXPRESAMENTE (conceptual).

### ARCH-F04 — Alternativa 3: descriptores tipados y capacidades explícitas para `GitStore`, `Filesystem` y `Reporter`

- Separación entre lectura y mutación; métodos tipados; sin `Execute` genérico con `Operation string`.
- Identidad física de archivos y rutas; protección contra traversal y symlinks; operaciones de filesystem basadas en descriptores.
- CAS por versión y propietario; auditoría append-only; `Reporter` desacoplado del renderizado.
- **Una única definición del puerto `Filesystem`** (reconciliar `ports.go` con `filesystem.go` de T021).
- Mutaciones solo bajo autorización durable; poseer un descriptor o credencial técnica no equivale a aprobación humana.
- Dependencias: T008 (tipos); T021–T023 contra este contrato; resolución antes de construir los adaptadores.
- **Estado:** APROBADA EXPRESAMENTE (conceptual).

## F. Bloque IV — Desviaciones T003 y T004 (respuesta: "aprobar")

No autoriza modificar `tasks.md`.

| ID | Decisión | Condiciones aprobadas | Estado |
|---|---|---|---|
| DEV-T003 | Excepción de `os/exec` en el harness de sentinelas | Solo `internal/gitmanager/testhelpers_test.go`, por nombre exacto; solo código de pruebas; sin extensión a otros archivos; sin permisos de ejecución arbitraria; restricciones productivas intactas; se registra en el futuro cierre | APROBADA EXPRESAMENTE |
| DEV-T004 | `sentinel_pty_posix_test.go` y `sentinel_pty_other_test.go` como parte del harness | Autorizados en el Paso 75; ubicación actual; desviación registrada; actualizar el inventario normativo de T004 es una decisión aparte | APROBADA EXPRESAMENTE |

## G. Bloque V — Versionamiento y cierre (respuesta: "aprobar")

Ninguna de estas aprobaciones ejecuta staging, commits, movimientos de informes ni el cierre de la fase.

| ID | Decisión | Condiciones aprobadas | Estado |
|---|---|---|---|
| BASELINE-008 | Política de versionamiento en tres grupos | Grupo 1: 11 archivos de especificación en un commit documental independiente, sin modificar `tasks.md`. Grupo 2: 13 archivos de código en un commit independiente, sin modificar código ni incluir F-03/F-04. Grupo 3: informes por **alternativa B** (preservación íntegra en un subdirectorio histórico de la feature, con manifiesto SHA-256, en un commit documental independiente, sin duplicados en la raíz). Mover informes y crear el manifiesto exigen autorización posterior explícita. Inventario actual: 41 archivos; se actualiza antes de cualquier staging; sin `git add .`. | APROBADA EXPRESAMENTE |
| BASELINE-LOCAL-PATHS | Qué hacer con las rutas locales de los informes | Opciones: **A** conservar rutas originales y hashes; **B** redactar y recalcular hashes, preservando los originales de forma verificable; **C** posponer hasta revisar la visibilidad del repositorio. Verificado: 9 de 17 informes contienen `/Users/german/...`; `STEP-76A-AUDIT-REPORT.md` cita una ruta de sesión local (`~/.codex/...`), y `STEP-88-GOVERNANCE-PROPOSAL.md` la menciona. Sin credenciales, tokens ni correos. | **REQUIERE ACLARACIÓN** (la respuesta "aprobar" no indica A, B ni C) |
| BASELINE-PROCEDURE-008 | Procedimiento futuro de adopción de línea base | Aprobación de decisiones → preflight → inventario → verificación de hashes → revisión de contenido sensible → rutas exactas → commits separados → manifiesto → verificación posterior → preparación del cierre. La ejecución requiere autorización separada; si el inventario cambia, se repiten inventario y hashes. | APROBADA EXPRESAMENTE |
| CLOSURE-008-PHASE1 | Procedimiento futuro de cierre | Registrar fecha y responsable; referenciar commits y hashes; registrar contratos, deuda, decisiones arquitectónicas y desviaciones; actualizar normativos solo con autorización separada (cambia sus hashes; Feature 006 usó una nota de sesión fechada); evaluar el marcado de T001–T006 por separado; pedir autorización expresa para declarar cerrada la Fase 1. **No declara cerrada la Fase 1.** | APROBADA EXPRESAMENTE |

## H. Matriz consolidada

| ID | Propuesta original | Respuesta humana | Estado | Pendientes |
|---|---|---|---|---|
| S-01 | Aceptar ausencia de reaper externo concurrente | "aprobar" | APROBADA EXPRESAMENTE | — |
| S-02 | Aceptar reserva POSIX de PID/PGID | "aprobar" | APROBADA EXPRESAMENTE | — |
| S-03 | Aceptar aislamiento de señales | "aprobar" | APROBADA EXPRESAMENTE | — |
| S-04 | Aceptar límites temporales | "aprobar" | APROBADA EXPRESAMENTE | — |
| S-05 | Aceptar infraestructura inerte | "aprobar" | APROBADA EXPRESAMENTE | — |
| D-01 | Aceptación temporal de 14 hallazgos | "aprobar" | APROBADA EXPRESAMENTE | — |
| D-02 | Aceptación temporal de N-03 | "aprobar" | APROBADA EXPRESAMENTE | — |
| D-03 | N-04 antes de adaptadores reales | "aprobar" | APROBADA EXPRESAMENTE | N-04 sigue sin resolver |
| D-04 | F-07 a F-10 | "aprobar" | APROBADA EXPRESAMENTE | F-10 depende de BASELINE-008 |
| ARCH-F03 | Descriptor cerrado (alternativa 2) | "aprobar" | APROBADA EXPRESAMENTE (conceptual) | No autoriza implementar |
| ARCH-F04 | Descriptores tipados y capacidades (alternativa 3) | "aprobar" | APROBADA EXPRESAMENTE (conceptual) | No autoriza implementar |
| DEV-T003 | Excepción limitada de `os/exec` | "aprobar" | APROBADA EXPRESAMENTE | Actualizar `tasks.md` es aparte |
| DEV-T004 | Helpers PTY como parte del harness | "aprobar" | APROBADA EXPRESAMENTE | Actualizar inventario de T004 es aparte |
| BASELINE-008 | Tres grupos; informes por alternativa B | "aprobar" | APROBADA EXPRESAMENTE | El Grupo 3 depende de BASELINE-LOCAL-PATHS |
| BASELINE-LOCAL-PATHS | A, B o C | "aprobar" | **REQUIERE ACLARACIÓN** | **Elegir A, B o C** |
| BASELINE-PROCEDURE-008 | Secuencia conceptual de adopción | "aprobar" | APROBADA EXPRESAMENTE | No autoriza ejecutar |
| CLOSURE-008-PHASE1 | Procedimiento futuro de cierre | "aprobar" | APROBADA EXPRESAMENTE | No cierra la Fase 1 |

## I. Veredicto

**PARTIAL HUMAN DECISIONS.**

- 17 de 18 decisiones expresamente aprobadas (varias condicionadas o conceptuales, según se indica).
- 1 decisión requiere aclaración: BASELINE-LOCAL-PATHS.
- Ninguna fue rechazada ni pospuesta.

Esto es distinto de DECISIONS COMPLETE, y ni siquiera DECISIONS COMPLETE equivaldría a cerrar la fase.

## J. Qué no ha ocurrido

- No se modificó ningún archivo del proyecto: ni código, ni especificación, ni `tasks.md` (0 de 118 casillas marcadas).
- No se hizo staging, commits, merges, rebases, tags ni pushes. No se ejecutó `git flow feature finish`.
- No se movió ningún informe ni se creó el manifiesto.
- No se implementó ARCH-F03, ARCH-F04 ni N-04. T016, T020 y T021–T023 siguen bloqueadas por esas decisiones.
- La línea base sigue **verificada pero no versionada**. La Fase 1 sigue **abierta**. La Fase 2 **no está autorizada**.

## K. Pendiente

1. **BASELINE-LOCAL-PATHS:** el responsable debe elegir A, B o C.
2. Decisiones separadas, solo si se desean:
   - Actualizar `tasks.md` (para T003/T004 o para marcar T001–T006).
   - Actualizar el inventario normativo de T004.
   - Cuándo ejecutar la adopción de la línea base.
   - El cierre formal de la Fase 1.
   - La autorización de la Fase 2.

Cada uno de esos actos requiere una autorización posterior, explícita y separada.

---

Este documento es un registro de respuestas en la conversación. No es un acta firmada, no autoriza ninguna ejecución, no adopta la línea base, no cierra la Fase 1 y no autoriza la Fase 2, la modificación de `tasks.md` ni la creación de commits.
