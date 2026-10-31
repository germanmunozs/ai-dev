# Propuesta de cierre de Fase 1

## Feature 008 — Git Management

**Estado: PROPUESTA — FASE 1 NO CERRADA.**

## 1. Identificación

- Feature: 008 — Git Management.
- Fase: 1 — Setup and Shared Test Infrastructure.
- Rama: `feature/008-git-management`.
- Fecha de elaboración: 2026-10-31.
- Naturaleza: propuesta documental sujeta a autorización humana expresa de cierre.

## 2. Línea base

La evidencia versionada de Fase 1 está distribuida en los siguientes commits:

| Alcance | Commit |
|---|---|
| Especificación de Feature 008 | `0ac06bae557abd01786861a833dffc0cf5fd982c` |
| Fundación de código de Fase 1 | `69563d22cf9bafd8fa442113f2522ebf67f7da31` |
| Informes históricos y manifiesto inicial | `1dbf8a89de3d304249a33e96e8e899b96f2c8d23` |
| Regularización normativa de T001–T006 | `05e827ee7ed8308a381c8fdb876b61a8469112d3` |

El commit documental que incorporará esta propuesta no se identifica aquí porque todavía no existía al elaborar el documento.

## 3. Cumplimiento de T001–T006

El archivo `specs/008-git-management/tasks.md`, regularizado en el commit normativo del Paso 96, registra:

- T001: completa.
- T002: completa; los contratos permanecen sujetos a la evolución posterior autorizada para F-03 y F-04.
- T003: completa con la desviación aprobada DEV-T003.
- T004: completa con la desviación aprobada DEV-T004.
- T005: completa.
- T006: completa.

No se marcaron tareas posteriores a T006.

## 4. Validación técnica

El Paso 94 registró la siguiente evidencia, que no fue ejecutada nuevamente durante la elaboración inicial de esta propuesta:

- `gofmt -l`: correcto, sin salida.
- `go test`: aprobado para `internal/app`, `internal/conventionalcommit`, `internal/gitmanager`, `internal/gitprocess` e `internal/gitstore`.
- `go vet`: aprobado para los mismos paquetes.
- Los siete hashes de referencia se conservaron.
- Los diecinueve informes históricos iniciales se verificaron correctamente contra `SHA256SUMS`.

Las verificaciones focalizadas posteriores exigidas por el Paso 96 se registrarán en el informe de ejecución de ese paso; no se presentan aquí como evidencia ya existente.

## 5. Contratos de seguridad

Las decisiones S-01 a S-05 están aprobadas con las siguientes limitaciones:

- S-01 — Ciclo de vida: sin reaper externo concurrente; PGID capturado inmediatamente después de `Start`; señalización antes de recolección; probe `Signal(0)`; alcance exclusivo de pruebas; permanece el riesgo de reutilización de PID en macOS.
- S-02 — Reserva POSIX de PID/PGID: señal antes de `wait4` o `Release`, sin señales posteriores; evidencia nativa limitada a macOS/arm64 y sin validación nativa de Linux.
- S-03 — Aislamiento de señales: rechazo de `groupID <= 1`; señales reales limitadas a procesos y grupos sintéticos propios; callbacks inertes en pruebas negativas; N-20 permanece vinculada.
- S-04 — Límites temporales: supervisión de 10 segundos y limpieza de 2 segundos, sin fugas; salida de pagers limitada por la deuda N-16.
- S-05 — Infraestructura inerte: sin runner ni adaptadores productivos, ejecución automática de planes o mutaciones Git habilitadas en Fase 1.

## 6. Deuda técnica

Las decisiones D-01 a D-04 continúan vigentes:

- D-01: aceptación temporal de N-05, N-06, N-07, N-08, N-11, N-12, N-15, N-16, N-17, N-20, N-21, N-22, N-27 y N-28 bajo sus condiciones aprobadas. No equivale a resolución definitiva.
- D-02: N-03 está aceptada temporalmente; deberá añadirse una prueba propia de la comprobación indirecta cuando se modifique el detector, junto con N-04.
- D-03: N-04 permanece pendiente y bloquea T016 y T020. Debe resolverse antes de incorporar adaptadores reales en `gitprocess` y `gitstore`.
- D-04: F-07 y F-08 están diferidas a Fase 12; F-09 tiene aceptación temporal; F-10 fue tratada mediante la política de versionamiento.

## 7. Arquitectura futura

- ARCH-F03: aprobada conceptualmente mediante la alternativa 2, descriptor cerrado para `GitInvocation`. No está implementada.
- ARCH-F04: aprobada conceptualmente mediante la alternativa 3, descriptores tipados y capacidades explícitas. No está implementada.

Estas decisiones no autorizan iniciar su implementación ni adelantar tareas posteriores.

## 8. Desviaciones

- DEV-T003: excepción de importación `os/exec` exclusivamente en `internal/gitmanager/testhelpers_test.go`, limitada a infraestructura sintética de pruebas y sin extensión a código productivo ni otros archivos. Riesgo residual: la excepción depende del nombre exacto del archivo.
- DEV-T004: el harness de pruebas autorizado incluye `internal/gitmanager/sentinel_pty_posix_test.go` e `internal/gitmanager/sentinel_pty_other_test.go`; su autorización original corresponde al Paso 75 y su alcance es exclusivamente de pruebas.

## 9. Evidencia documental

La evidencia documental comprende:

- Los informes históricos originales de los Pasos 73 a 91.
- `STEP-93-PREPARATION-REPORT.md`.
- `STEP-94-EXECUTION-REPORT.md`.
- `SHA256SUMS`, actualizado con 21 entradas para los informes históricos.
- Esta propuesta, que no se incluye a sí misma en el manifiesto de informes.

El informe del Paso 95 no forma parte de esta incorporación.

## 10. Riesgos residuales

- La validación nativa disponible depende de macOS/arm64.
- La validación nativa en Linux permanece pendiente.
- Permanece el riesgo de reutilización de PID.
- La excepción DEV-T003 depende del nombre del archivo.
- Las rutas locales se conservan por la alternativa A de BASELINE-LOCAL-PATHS.
- Existen espacios finales documentales preexistentes y no bloqueantes, preservados para no alterar la línea base aprobada.
- F-03 y F-04 están pendientes de implementación.
- N-04 permanece pendiente antes de T016 y T020.

## 11. Condiciones para el cierre

El cierre formal de Fase 1 requiere confirmar:

1. Los dos commits del Paso 96 fueron creados y verificados.
2. `tasks.md` quedó regularizado exclusivamente para T001–T006 y DEV-T003/DEV-T004.
3. Los informes 93 y 94 fueron incorporados íntegramente.
4. `SHA256SUMS` contiene 21 entradas válidas.
5. No existen cambios inesperados, modificaciones de código ni tareas posteriores marcadas.
6. Las verificaciones focalizadas del Paso 96 resultaron satisfactorias.
7. El responsable emitió una autorización humana expresa y separada para declarar cerrada la fase.

El cumplimiento técnico de estas condiciones no sustituye la autorización humana de cierre.

## 12. Declaración final

**«Esta propuesta documenta la preparación del cierre de Fase 1. No constituye una declaración de cierre ni autoriza iniciar Fase 2».**
