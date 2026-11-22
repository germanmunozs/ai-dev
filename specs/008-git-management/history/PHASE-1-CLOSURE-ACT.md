# Acta definitiva de cierre de Fase 1

## Feature 008 — Git Management

## 1. Identificación

| Campo | Valor |
|---|---|
| Proyecto | `ai-dev` |
| Feature | 008 — Git Management |
| Fase | 1 — Setup |
| Rama | `feature/008-git-management` |
| Fecha de cierre | 2026-10-31 |
| Responsable | Responsable del proyecto, mediante autorización humana explícita registrada en la ejecución del Paso 97 |
| Alcance | T001–T006 y desviaciones aprobadas DEV-T003 y DEV-T004 |

No se consigna una identidad personal ni una firma porque no fueron confirmadas como parte de esta autorización.

## 2. Declaración de cierre

**«Se declara formalmente cerrada la Fase 1 — Setup de Feature 008 — Git Management, exclusivamente respecto de las tareas T001–T006 y las desviaciones aprobadas».**

Esta declaración se emite después de la autorización humana explícita recibida para ejecutar el Paso 97. El cierre es administrativo y documental; no declara terminada Feature 008, no completa tareas posteriores y no autoriza iniciar Fase 2.

## 3. Tareas completadas

| Tarea | Estado |
|---|---|
| T001 | Completada |
| T002 | Completada |
| T003 | Completada con DEV-T003 |
| T004 | Completada con DEV-T004 |
| T005 | Completada |
| T006 | Completada |

El estado normativo consta en `specs/008-git-management/tasks.md`. Las tareas T007–T118 permanecen pendientes y no forman parte de este cierre.

## 4. Línea base y regularización

La línea base y su regularización están registradas en los cinco commits siguientes:

| Alcance | Commit |
|---|---|
| Especificación de Feature 008 | `0ac06bae557abd01786861a833dffc0cf5fd982c` |
| Fundación de código de Fase 1 | `69563d22cf9bafd8fa442113f2522ebf67f7da31` |
| Informes históricos y manifiesto inicial | `1dbf8a89de3d304249a33e96e8e899b96f2c8d23` |
| Regularización normativa de T001–T006 | `05e827ee7ed8308a381c8fdb876b61a8469112d3` |
| Evidencia documental y propuesta de cierre | `9e9e7de74197fb77fc420b25a0fdefb7a3dd38e9` |

El SHA del commit que incorpora esta acta no se incluye porque no existía al elaborar el documento.

## 5. Validación técnica

Este cierre utiliza evidencia técnica ya aprobada; el Paso 97 no volvió a ejecutar las pruebas.

### Evidencia del Paso 94 — 2026-10-31

Documentada en `specs/008-git-management/history/STEP-94-EXECUTION-REPORT.md`:

- `gofmt -l`: correcto, sin salida.
- `go test -count=1 -timeout=60s`: aprobado para los paquetes de Fase 1.
- `go vet`: aprobado.
- Dependencias sin descargas, mediante `GOPROXY=off` y `GOSUMDB=off`.
- Siete hashes de referencia conservados.
- Diecinueve informes históricos verificados.

### Evidencia del Paso 96 — 2026-10-31

Documentada en el informe externo `STEP-96-REGULARIZATION-EXECUTION-REPORT.md` y respaldada por los commits de regularización:

- `gofmt -l`: correcto, sin salida.
- `go test -count=1 -timeout=60s`: aprobado.
- `go vet`: aprobado.
- Código y pruebas sin modificaciones durante la regularización.
- Manifiesto histórico ampliado y verificado con 21 de 21 entradas correctas.

Estas referencias registran evidencia anterior; no representan pruebas nuevas ejecutadas durante el Paso 97.

## 6. Contratos de seguridad

Las decisiones S-01 a S-05 permanecen aprobadas con sus límites originales:

- **S-01 — Ciclo de vida del proceso.** Sin reaper externo concurrente; PGID capturado inmediatamente después de `Start`; señalización antes de recolección; probe `os.Process.Signal(0)`; alcance exclusivo de pruebas. Permanece el riesgo residual de reutilización de PID en macOS.
- **S-02 — Reserva POSIX de PID/PGID.** Señalización antes de `wait4` o `Release`, sin señales posteriores a la recolección. La evidencia nativa corresponde a macOS/arm64; Linux no ha sido validado nativamente.
- **S-03 — Aislamiento de señales.** Rechazo de `groupID <= 1`; señales reales limitadas a procesos y grupos sintéticos propios; callbacks inertes en pruebas negativas; N-20 permanece vinculada.
- **S-04 — Límites temporales.** Supervisión de 10 segundos y limpieza de 2 segundos, sin fugas; la salida de pagers permanece limitada por N-16.
- **S-05 — Infraestructura inerte.** Sin runner o adaptadores productivos, ejecución automática de planes ni mutaciones Git habilitadas en Fase 1.

El cierre no amplía estos contratos ni sus excepciones.

## 7. Deuda técnica aceptada

Las decisiones D-01 a D-04 permanecen vigentes:

- **D-01.** Aceptación temporal de N-05, N-06, N-07, N-08, N-11, N-12, N-15, N-16, N-17, N-20, N-21, N-22, N-27 y N-28 bajo las condiciones aprobadas. No equivale a resolución definitiva.
- **D-02.** N-03 está aceptada temporalmente. Deberá añadirse una prueba propia de la comprobación indirecta cuando se modifique el detector, junto con N-04.
- **D-03.** N-04 permanece pendiente y bloquea T016 y T020. Debe resolverse antes de incorporar adaptadores reales en `gitprocess` y `gitstore`.
- **D-04.** F-07 y F-08 están diferidas a Fase 12; F-09 tiene aceptación temporal; F-10 fue tratada mediante la política de versionamiento.

**N-04 debe resolverse antes de implementar T016/T020.**

## 8. Decisiones arquitectónicas

- **ARCH-F03:** alternativa 2, descriptor cerrado para `GitInvocation`.
- **ARCH-F04:** alternativa 3, descriptores tipados y capacidades explícitas.

Ambas decisiones están aprobadas conceptualmente, pero todavía no están implementadas. El cierre de Fase 1 no autoriza su implementación ni modifica sus dependencias.

## 9. Desviaciones

### DEV-T003

Excepción limitada de importación `os/exec` en:

```text
internal/gitmanager/testhelpers_test.go
```

Su alcance se limita a infraestructura sintética de pruebas. No se extiende a código productivo ni a otros archivos. Permanece como riesgo residual la dependencia del nombre exacto del archivo.

### DEV-T004

Harness de pruebas autorizado:

```text
internal/gitmanager/sentinel_pty_posix_test.go
internal/gitmanager/sentinel_pty_other_test.go
```

La autorización original corresponde al Paso 75 y su alcance es exclusivamente de pruebas.

## 10. Decisiones de línea base y cierre

Se conservan sin reapertura ni ampliación:

- **BASELINE-008:** adopción en tres grupos independientes.
- **BASELINE-LOCAL-PATHS, alternativa A:** conservación íntegra de rutas locales y hashes.
- **BASELINE-PROCEDURE-008:** procedimiento controlado de adopción y verificación.
- **CLOSURE-008-PHASE1:** cierre formal sujeto a evidencia y autorización humana separada, satisfecha para este acto mediante el Paso 97.

Las 17 decisiones de gobernanza permanecen resueltas y vigentes.

## 11. Riesgos residuales

- La validación nativa de procesos POSIX fue realizada en macOS/arm64.
- La validación nativa en Linux permanece pendiente.
- Persiste el riesgo residual de reutilización de PID.
- La excepción DEV-T003 depende del nombre exacto del archivo.
- Las rutas locales se preservan conforme a BASELINE-LOCAL-PATHS, alternativa A.
- Se conservan observaciones menores de formato documental, incluidos espacios finales preexistentes no bloqueantes.
- F-03 y F-04 permanecen pendientes de implementación.
- N-04 permanece pendiente antes de T016 y T020.

## 12. Alcance del cierre

- Fase 1 — Setup queda formalmente cerrada.
- Feature 008 continúa abierta y en desarrollo.
- T007–T118 permanecen pendientes.
- F-03/F-04 permanecen pendientes de implementación.
- N-04 permanece pendiente de resolución antes de T016/T020.
- Fase 2 no fue iniciada ni autorizada.
- No se ejecutaron operaciones remotas.
- No se ejecutó push, merge, rebase ni integración Git Flow.
- No se declara implementada ninguna capacidad productiva de Git Management fuera de T001–T006.

## 13. Trazabilidad

Este cierre se sustenta en:

- `specs/008-git-management/tasks.md`.
- `specs/008-git-management/spec.md`.
- `specs/008-git-management/plan.md`.
- `specs/008-git-management/history/PHASE-1-CLOSURE-PROPOSAL.md`.
- `specs/008-git-management/history/STEP-91-GOVERNANCE-DECISIONS.md`.
- `specs/008-git-management/history/STEP-93-PREPARATION-REPORT.md`.
- `specs/008-git-management/history/STEP-94-EXECUTION-REPORT.md`.
- El informe externo `STEP-96-REGULARIZATION-EXECUTION-REPORT.md`, no incorporado al repositorio por este cierre.
- `specs/008-git-management/history/SHA256SUMS`, con 21 entradas válidas.
- Los cinco commits de línea base y regularización enumerados en esta acta.

No se incluyen firmas ficticias ni se atribuyen aprobaciones distintas de la autorización humana explícita recibida para el Paso 97.
