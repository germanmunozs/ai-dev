# Informe del Paso 91 — Acta consolidada de decisiones de gobernanza (Feature 008, Fase 1)

Fecha: 2026-10-26
Modo: consolidación documental, read-only. Ningún archivo del proyecto fue modificado durante la consolidación. Este archivo se creó después, a petición expresa del usuario. **Este documento no está firmado, no está versionado ni incorporado formalmente al repositorio, y no ejecuta ninguna decisión.**

## Corrección previa: el conteo "18" fue un error del Paso 90

El Paso 90 decía "17 de 18 decisiones aprobadas y 1 con aclaración". Es incorrecto. Se verificó la matriz de `STEP-90-DECISIONS-RECORD.md` (sección H):

- **Identificadores únicos:** 17 (S-01 a S-05, D-01 a D-04, ARCH-F03, ARCH-F04, DEV-T003, DEV-T004, BASELINE-008, BASELINE-LOCAL-PATHS, BASELINE-PROCEDURE-008, CLOSURE-008-PHASE1).
- **Estados en esa matriz:** 16 APROBADA EXPRESAMENTE y 1 REQUIERE ACLARACIÓN (BASELINE-LOCAL-PATHS).

El "18" aparece en la línea 111 de `STEP-90-DECISIONS-RECORD.md` ("17 de 18 decisiones expresamente aprobadas") y en el mensaje de ese paso. No existe una decimoctava decisión, y no se inventó. El registro histórico no se modificó; esta corrección queda documentada aquí. Corregirlo en el archivo original requeriría una autorización separada.

**Conteo correcto: 17 decisiones identificadas, 16 aprobadas en el Paso 90 y la 17.ª (BASELINE-LOCAL-PATHS) resuelta después con la alternativa A. Total: 17 de 17 resueltas.**

## Preflight (solo lecturas)

- **Rama y HEAD:** `feature/008-git-management`, `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- **Git:** índice y diff vacíos. 42 archivos sin seguimiento al verificar = 11 de especificación + 13 de código + 18 informes `STEP-*` (73 a 90). Con este archivo serán 43 y 19 informes.
- **Hashes:** `tasks.md` (`614386e5…`), `spec.md` (`8e09095d…`) y los cinco archivos de código de referencia coinciden.
- **`tasks.md`:** sin modificar. No se ejecutaron pruebas ni nada que genere artefactos.

---

# ACTA CONSOLIDADA DE DECISIONES DE GOBERNANZA
## FEATURE 008 — GIT MANAGEMENT / FASE 1

| Campo | Valor |
|---|---|
| Identificador | `FEATURE-008-GOVERNANCE-DECISIONS-01` |
| Fecha de consolidación | 2026-10-26 |
| Responsable | Responsable del proyecto |
| Estado | **DECISIONES HUMANAS COMPLETAS — EJECUCIÓN NO AUTORIZADA** |
| Referencias | `STEP-88-GOVERNANCE-PROPOSAL.md`, `STEP-89-GOVERNANCE-DRAFT.md` (`FEATURE-008-GOVERNANCE-DRAFT-01`), `STEP-90-DECISIONS-RECORD.md` |
| Rama / HEAD | `feature/008-git-management` / `a61ae2ffbb591a6282636805a2c35c81246b2d5c` |
| Línea base | Verificada por contenido y hashes; **no versionada** |

**Este documento no está firmado, no está versionado ni incorporado al repositorio.** No hay firma ni fecha de firma. Consolida decisiones humanas ya emitidas; no ejecuta ninguna.

## Procedencia de la evidencia

- **16 decisiones:** aprobadas con la respuesta literal "aprobar" a cada uno de los cinco bloques del Paso 90, durante esa conversación. Cada bloque presentó las propuestas con sus condiciones, y la respuesta se interpretó como aprobación de lo presentado. Quedan documentadas en `STEP-90-DECISIONS-RECORD.md`.
- **BASELINE-LOCAL-PATHS:** el registro del Paso 90 la dejó en REQUIERE ACLARACIÓN. La alternativa A fue una **decisión humana posterior al Paso 90**, declarada por el responsable en la instrucción del Paso 91 ("«A»", ante la pregunta de qué alternativa aprobaba), y atribuida a la conversación con ChatGPT. **No hay acceso a esa conversación ni se puede verificar de forma independiente.** Se usa la declaración del responsable como evidencia, sin atribuirle una verificación que no existe. `STEP-90-DECISIONS-RECORD.md` no contiene esta aprobación.

## Contratos de seguridad S-01 a S-05: APROBADOS EXPRESAMENTE

La aceptación de estos contratos no autoriza implementar cambios.

- **S-01. Ciclo de vida del proceso.**
  - Ausencia de reaper externo concurrente en el harness.
  - PGID capturado inmediatamente después de `Start`.
  - Señalización antes de recolección.
  - Probe `os.Process.Signal(0)`.
  - Alcance exclusivo de pruebas.
  - Riesgo residual: reutilización de PID en macOS.
- **S-02. Reserva POSIX de PID/PGID.**
  - Líder no recolectado hasta señalizar.
  - Señal antes de `wait4`/`Release`, sin señales posteriores.
  - Evidencia nativa solo en macOS/arm64.
  - Linux no validado nativamente.
- **S-03. Aislamiento de señales.**
  - Rechazo de `groupID <= 1`.
  - Señales reales solo a grupos sintéticos propios.
  - Callbacks inertes en pruebas negativas.
  - N-20 como deuda vinculada.
- **S-04. Límites temporales.**
  - Supervisión de 10 segundos y limpieza de 2 segundos.
  - Sin fugas.
  - Límite práctico de salida del pager por N-16.
- **S-05. Infraestructura inerte.**
  - Sin runner productivo ni adaptadores productivos.
  - Sin ejecución automática de planes.
  - Sin mutaciones Git habilitadas en Fase 1.

## Deuda técnica D-01 a D-04: APROBADAS EXPRESAMENTE

- **D-01.** Aceptación temporal del grupo completo: N-05, N-06, N-07, N-08, N-11, N-12, N-15, N-16, N-17, N-20, N-21, N-22, N-27 y N-28.
  - N-15: conservar `select` antes de `read`.
  - N-16: limitar la salida de los pagers de prueba.
  - N-20: usar solo el PGID capturado.
  - N-27: vinculada a S-01.
  - N-07: revisar con evidencia nativa en Linux.
  - La aceptación temporal no equivale a resolución definitiva.
- **D-02.** N-03 aceptada temporalmente. Añadir una prueba propia de la comprobación indirecta cuando se modifique el detector, junto con N-04.
- **D-03.** N-04 debe resolverse antes de incorporar adaptadores reales en `gitprocess` y `gitstore`.
  - Bloquea T016 y T020.
  - Criterios: detección de ejecución indirecta, restricciones explícitas de imports, matriz de mutaciones ampliada, pruebas negativas de dependencias prohibidas.
  - **N-04 sigue sin resolverse técnicamente.** Aprobar la decisión ni la implementa ni desbloquea tareas.
- **D-04.** F-07 y F-08: revisión en Fase 12. F-09: aceptación temporal. F-10: resolver mediante la política de versionamiento. Ninguna está corregida.

## ARCH-F03: APROBADA EXPRESAMENTE — decisión arquitectónica conceptual

**Alternativa 2: descriptor cerrado para `GitInvocation`.**

- Operaciones Git tipadas, de un conjunto cerrado.
- `GitToolIdentity` congelado, directorio físico explícito y perfil de entorno controlado.
- Stdin y límites de salida acotados.
- Argv y entorno construidos por el runner, sin shell, sin ejecutables ni entornos arbitrarios.
- Operaciones separadas por clase: lectura, plumbing autorizado y mutación.
- Las mutaciones exigen autorización humana durable, plan canónico, Safety Kernel, CAS y auditoría previa. El descriptor no equivale a autorización.
- Dependencias: T008 y T016–T019. El contrato debe estar aprobado antes de T016.
- **No se modificó `ports.go` ni los fakes, y no se implementó nada.**

## ARCH-F04: APROBADA EXPRESAMENTE — decisión arquitectónica conceptual

**Alternativa 3: descriptores tipados y capacidades explícitas.**

- Lectura y mutación separadas; operaciones tipadas; sin `Execute` genérico.
- Identidad física; protección contra traversal y symlinks.
- CAS por versión y propietario; auditoría append-only.
- `Reporter` desacoplado del renderizado.
- Una sola definición de `Filesystem`.
- Mutaciones bajo autorización durable.
- Dependencias: T008 y T021–T023.
- **No se implementó nada.**

## DEV-T003 y DEV-T004: APROBADAS EXPRESAMENTE

- **DEV-T003.** Excepción de `os/exec` exclusivamente en `internal/gitmanager/testhelpers_test.go`. Solo pruebas, sin extensión a código productivo ni a otros archivos, sin autorización para procesos arbitrarios. Se registrará en el cierre.
- **DEV-T004.** Se aceptan `sentinel_pty_posix_test.go` y `sentinel_pty_other_test.go` como parte del harness. Ubicación actual; autorización original del Paso 75.
- Actualizar `tasks.md` o el inventario normativo requiere una autorización separada.

## BASELINE-008: APROBADA EXPRESAMENTE

Política de tres grupos:

- **Grupo 1, especificación (11 archivos):** commit documental independiente, sin modificaciones normativas durante la adopción.
- **Grupo 2, código (13 archivos):** commit independiente, sin rediseñar F-03 ni F-04.
- **Grupo 3, informes históricos:** alternativa B (preservación íntegra, subdirectorio histórico de la feature, manifiesto SHA-256, commit documental independiente, sin duplicados innecesarios en la raíz).

**La aprobación de la política no autoriza mover archivos, crear el manifiesto ni ejecutar commits.** El inventario debe actualizarse antes de cualquier ejecución: al verificar eran 42 archivos sin seguimiento y 18 informes.

## BASELINE-LOCAL-PATHS: APROBADA EXPRESAMENTE — alternativa A

- Conservar las rutas locales originales, el contenido y los hashes SHA-256; no redactar.
- Mantiene la trazabilidad histórica.
- Se reconoce que las rutas locales serán visibles para quienes tengan acceso al repositorio cuando se autorice su versionamiento.
- Procedencia: decisión humana posterior al Paso 90, declarada por el responsable (ver "Procedencia de la evidencia").

## BASELINE-PROCEDURE-008: APROBADA EXPRESAMENTE

Secuencia conceptual: preflight → inventario → verificación de hashes → revisión de contenido sensible → confirmación de rutas exactas → commits separados → manifiesto → verificación posterior → preparación del cierre. **La ejecución sigue pendiente de autorización expresa y separada.**

## CLOSURE-008-PHASE1: APROBADA EXPRESAMENTE — solo el procedimiento futuro

Incluye fecha y responsable; referencias a hashes y commits; registro de contratos, deuda, F-03/F-04 y T003/T004; actualización normativa posterior autorizada; evaluación del marcado de T001–T006; y declaración formal de cierre únicamente mediante autorización explícita. **La Fase 1 permanece abierta.**

## Matriz consolidada (17 decisiones)

| ID | Decisión aprobada | Condiciones | Evidencia de aprobación | Estado |
|---|---|---|---|---|
| S-01 | Contrato de ciclo de vida | Sin reaper externo; PGID tras `Start`; señalizar antes de recolectar; `Signal(0)`; solo pruebas; riesgo de PID en macOS | "aprobar", Bloque I del Paso 90 | APROBADA EXPRESAMENTE |
| S-02 | Reserva POSIX de PID/PGID | Señal antes de `wait4`/`Release`; sin señales posteriores; evidencia macOS/arm64 | "aprobar", Bloque I | APROBADA EXPRESAMENTE |
| S-03 | Aislamiento de señales | `groupID <= 1` rechazado; solo grupos propios; callbacks inertes; N-20 vinculada | "aprobar", Bloque I | APROBADA EXPRESAMENTE |
| S-04 | Límites temporales | 10 s y 2 s; sin fugas; límite por N-16 | "aprobar", Bloque I | APROBADA EXPRESAMENTE |
| S-05 | Infraestructura inerte | Sin runner, adaptadores, planes automáticos ni mutaciones | "aprobar", Bloque I | APROBADA EXPRESAMENTE |
| D-01 | Aceptación temporal de 14 hallazgos | Condiciones de N-15, N-16, N-20, N-27, N-07 | "aprobar", Bloque II | APROBADA EXPRESAMENTE |
| D-02 | N-03 aceptada temporalmente | Prueba propia al modificar el detector, junto con N-04 | "aprobar", Bloque II | APROBADA EXPRESAMENTE |
| D-03 | N-04 antes de adaptadores reales | Bloquea T016 y T020; no implementada | "aprobar", Bloque II | APROBADA EXPRESAMENTE |
| D-04 | F-07/F-08 en Fase 12; F-09 temporal; F-10 por versionamiento | Ninguna corregida | "aprobar", Bloque II | APROBADA EXPRESAMENTE |
| ARCH-F03 | Descriptor cerrado (alternativa 2) | Conceptual; contrato antes de T016; no implementada | "aprobar", Bloque III | APROBADA EXPRESAMENTE (conceptual) |
| ARCH-F04 | Descriptores tipados y capacidades (alternativa 3) | Conceptual; tipos en T008; T021–T023; no implementada | "aprobar", Bloque III | APROBADA EXPRESAMENTE (conceptual) |
| DEV-T003 | Excepción de `os/exec` en `testhelpers_test.go` | Solo pruebas; sin extensión; se registra en el cierre | "aprobar", Bloque IV | APROBADA EXPRESAMENTE |
| DEV-T004 | Helpers PTY como parte del harness | Ubicación actual; autorizados en el Paso 75 | "aprobar", Bloque IV | APROBADA EXPRESAMENTE |
| BASELINE-008 | Tres grupos; informes por alternativa B | No autoriza mover, crear manifiesto ni commits | "aprobar", Bloque V | APROBADA EXPRESAMENTE |
| BASELINE-LOCAL-PATHS | **Alternativa A:** conservar rutas, contenido y hashes | Visibles al versionar | Declaración «A» del responsable, posterior al Paso 90 (no verificada de forma independiente) | APROBADA EXPRESAMENTE |
| BASELINE-PROCEDURE-008 | Secuencia conceptual de adopción | Ejecución con autorización separada | "aprobar", Bloque V | APROBADA EXPRESAMENTE |
| CLOSURE-008-PHASE1 | Procedimiento futuro de cierre | No cierra la Fase 1 | "aprobar", Bloque V | APROBADA EXPRESAMENTE |

## Obligaciones técnicas futuras (aprobadas como condiciones, no ejecutadas)

| Obligación | Bloquea | Estado |
|---|---|---|
| Resolver N-04 (detector y lista de imports de adaptadores) | T016, T020 | Pendiente |
| Contrato cerrado de `GitInvocation` (ARCH-F03) | T008, T016–T019 | Pendiente de implementación |
| Puertos tipados y única definición de `Filesystem` (ARCH-F04) | T008, T021–T023 | Pendiente de implementación |
| Prueba propia de la comprobación indirecta (N-03) | Con N-04 | Pendiente |
| Revisiones de F-07 y F-08 | Fase 12 | Pendiente |

## Autorizaciones que siguen pendientes (cada una, separada y expresa)

- Implementar ARCH-F03, ARCH-F04 y N-04, incluidos los cambios en `ports.go` y en los fakes.
- Actualizar `tasks.md` (T003/T004 o marcar T001–T006) y el inventario normativo de T004.
- Ejecutar la adopción de la línea base: mover informes, crear el manifiesto, staging y commits.
- Cierre formal de la Fase 1.
- **Inicio de la Fase 2: no autorizado.**

## Riesgos residuales

- El trabajo de Fase 1 sigue sin versionar y sin copia en Git.
- S-01 y S-02 dependen del sistema operativo; la ejecución nativa se probó solo en macOS/arm64.
- N-04 está pendiente de resolución técnica.
- F-03 y F-04 están aprobadas conceptualmente, no implementadas.
- La excepción de T003 es por nombre de archivo.
- Las actualizaciones normativas pendientes cambiarán los hashes de los normativos.
- Las rutas locales se conservan por decisión humana (alternativa A) y serán visibles al versionar.
- El inventario de informes es variable (18 al verificar, 19 con este archivo, más los que se generen).
- N-24 no existe y no hay informe del Paso 72.
- El registro del Paso 90 contiene el conteo erróneo "17 de 18"; está documentado arriba, y corregirlo en el archivo original requeriría una autorización separada.
- La aprobación de BASELINE-LOCAL-PATHS descansa en una declaración del responsable que no pudo verificarse de forma independiente.

## Verificaciones finales

- Ningún archivo del proyecto modificado durante la consolidación: HEAD, índice y diff limpios.
- Ningún commit creado ni informe movido. Ningún manifiesto SHA-256 creado.
- `tasks.md` sin cambios (hash `614386e5…`).
- Ninguna implementación iniciada.
- Línea base verificada pero no versionada. Fase 1 abierta. Fase 2 no autorizada.

## Veredicto

**DECISIONS COMPLETE — NOT EXECUTED.**

Las 17 decisiones identificadas tienen aprobación humana inequívoca y no se encontraron contradicciones. La diferencia con el "18" del Paso 90 se debió a un error de conteo, y el conjunto de identificadores no cambia. Este veredicto significa exclusivamente que las decisiones de gobernanza están completas. No significa que haya código implementado, línea base versionada, acta incorporada al repositorio, Fase 1 cerrada ni Fase 2 autorizada.

---

Este documento consolida decisiones humanas. No ejecuta ninguna de ellas, no adopta la línea base, no cierra la Fase 1 y no autoriza la Fase 2, la modificación de `tasks.md` ni la creación de commits.
