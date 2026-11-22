# Informe del Paso 86 — Auditoría independiente final de Fase 1 (Feature 008)

Fecha: 2026-10-22
Modo: auditoría read-only. Ningún archivo del proyecto fue modificado durante la auditoría. Este archivo se creó después, a petición expresa del usuario.

## A. Veredicto

**PASS WITH CONDITIONS.** N-23, N-25 y N-26 están corregidos y se reprodujeron las verificaciones. No se encontraron regresiones. Quedan limitaciones de contrato y deuda conocida que solo el responsable puede aceptar.

| Hallazgo | Estado |
|---|---|
| N-23 | Verificado, con un supuesto de contrato |
| N-25 | Verificado |
| N-26 | Verificado |
| N-09 | Sin regresión |
| N-10 | Sin regresión |
| N-13 | Sin regresión |
| N-14 | Sin regresión |
| N-18 | Sin regresión |
| N-19 | Sin regresión |

**Para proponer el cierre formal de la Fase 1: READY**, con las condiciones de la sección K. No se declara cerrada la fase.

## B. Preflight

- **Rama y HEAD:** `feature/008-git-management`, `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- **Hashes:** los siete coinciden con los esperados.
- **Git:** índice y diff vacíos, 37 archivos sin seguimiento (36 previos más `STEP-85-REPORT.md`).
- **Ejecución:** uid 501 (no root). `go.sum` sigue sin existir.
- **Alcance del Paso 85:** por mtime, solo cambiaron `testhelpers_test.go`, `sentinel_pty_posix_test.go` y `sentinel_pty_other_test.go`. `boundary_test.go`, `fakes_test.go`, `ports.go` e `internal/app` no cambiaron.
- **Normativos, código productivo y dependencias:** intactos.
- **Fases 2–12:** sin implementación anticipada.

## C. N-23: ciclo de vida de `os.Process` y probe

### Lo que hace el código

`pseudoTerminalProcessGroupIDWithLookup` ahora recibe además un probe de ciclo de vida. Lo ejecuta **antes** de `Getpgid`, en todos los casos, no solo ante `ESRCH`. La implementación real es `process.Signal(0)` (`sentinel_pty_posix_test.go:130`). El fallback `PGID = PID` solo se acepta si el probe pasa.

### Respuestas con evidencia del toolchain (Go 1.27.1, `os/exec_unix.go` y `exec.go`)

- **A. ¿Distingue de forma fiable recolectado de no recolectado?** Solo para las vías que pasan por `os.Process`. `Wait()` marca el estado `statusDone` y `Release()` marca `statusReleased` y pone `Pid = -1`. Una recolección externa con `syscall.Wait4` no actualiza ese estado.
- **B. ¿Puede `Signal(0)` hacer una syscall tras `Wait()`?** No. `pidSignal` devuelve `ErrProcessDone` desde el estado, antes de llamar a `kill`. Tras `Release()` devuelve `errProcessReleased`, y además el helper rechaza antes por `Pid = -1`.
- **C. ¿Carrera entre comprobar y señalar?** `pidSignal` toma `sigMu.RLock` durante la comprobación y la señal, y `pidWait` toma el bloqueo de escritura antes de recolectar. El `os.Process` mismo está cubierto. El helper no usa `os.Process.Wait`: recolecta con `wait4` y llama a `Release`, y lo hace después de capturar el PGID.
- **D. ¿Puede dar éxito para un proceso recolectado externamente?** Depende. Se probó con `syscall.Wait4` directo sobre un hijo propio: el probe devolvió `ErrProcessDone`, porque `kill(pid,0)` dio `ESRCH`. Pero si ese PID se reutilizara, `kill(pid,0)` tendría éxito sobre un ajeno y el probe diría "no recolectado". No se provocó reutilización de PID.
- **E. ¿Depende de que no haya un reaper externo?** Sí, y el Paso 85 lo declara como contrato.
- **F. ¿Nueva ventana de reutilización?** No. Se ejecuta inmediatamente después de `Start`, antes de cualquier recolección.
- **G. ¿Garantía por construcción u orden de llamadas?** Mixta:
  - Por construcción: `Wait()` y `Release()` sobre ese `os.Process`.
  - Por orden y contrato: ningún reaper externo, y que la función se llame justo después de `Start`.
- **H. Proceso zombie antes del probe:** probado en macOS, pasa. `kill(zombie,0)` tiene éxito, `Getpgid` real da `ESRCH` y el helper devuelve el PID.
- **I. `Release()` antes del probe:** rechazado antes de invocar los probes (`Pid = -1`).
- **J. ¿Preserva N-10, N-13 y N-18?** Sí. No se añadió ninguna señal real nueva en el flujo (el probe usa señal 0 sobre un hijo propio sin recolectar). Los demás puntos de señalización son los mismos.

### Pruebas reales con `os.Process` (macOS, hijos propios)

| Estado | Resultado |
|---|---|
| Vivo | `group == pid`, sin error |
| Zombie | `group == pid`, sin error |
| Tras `Wait()` | Rechazado: `os: process already finished` |
| Tras `Release()` | Rechazado: `unsafe process ID` (`pid=-1`) |
| Tras `wait4` crudo | Rechazado: `os: process already finished` |

### Límites verificados vs inferidos

- **Verificado en macOS:** todo lo anterior.
- **Inferido, sin poder probarlo aquí:** en Linux, `os.Process` usa pidfd, que es resistente a la reutilización de PID. Que `pidfd_send_signal` sobre un zombie tenga éxito se deduce de la documentación, no se ejecutó.

## D. N-25: cobertura de validaciones

Los diez casos pedidos están presentes en `TestPseudoTerminalProcessGroupCaptureRejectsInvalidInputs`: proceso nil, PID `-2`, `-1`, `0`, `1`, atributos nil, `Setpgid=false`, `Pgid=7`, lookup nil y probe nil. Cada caso comprueba el rechazo y que no se llamó a ningún probe. Ninguno usa señales reales: el lookup y el probe están inyectados.

### Mutaciones en copia aislada

| Mutación | Resultado |
|---|---|
| Quitar guard nil | Detectada (por pánico) |
| Quitar `Pid <= 1` | Detectada en 4 casos |
| `Pid < 1` (acepta PID 1) | Detectada |
| Quitar validación de atributos | Detectada |
| Quitar solo `Pgid != 0` | Detectada |
| Quitar solo `Setpgid` | Detectada |
| Quitar validación de lookup/probe nil | Detectada (por pánico) |
| Quitar la llamada al probe | Detectada |
| Probe real → siempre `nil` | Detectada |
| Quitar el fallback `ESRCH` | Detectada |

(La mutación "probe tras el lookup" no compiló, así que no es una medición válida. El orden queda cubierto por el caso `reaped_lifecycle`, que exige que el lookup no se llame.)

La suite detecta todas las regresiones relevantes. Los guards nil se detectan por pánico, no por fallo limpio.

## E. N-26: calidad y rendimiento de pruebas

- El test de 50 procesos fue eliminado.
- `ESRCH` se cubre de forma determinista por inyección, con un proceso real zombie y con `wait4` crudo.
- Los nombres corresponden al comportamiento: vivo, zombie, `Wait` y `Release` están cubiertos.
- No hay `time.Sleep` en los archivos de producción de tests de estas rutas, y no dependen de timing frágil.

### Rendimiento reproducido

| Medición | Resultado |
|---|---|
| Paquete con `-race` | 12,15 s (el Paso 84 midió ~61 s; el Paso 85 reporta 12,4 s) |
| Paquete sin `-race` | 7,6 s |
| 30 repeticiones de la captura | 0,76 s |

La mejora de unos 49 s es reproducible.

## F. Regresiones N-09, N-10, N-13, N-14, N-18 y N-19

- **N-09:** estructura intacta, sin goroutine lectora; `select(2)` antes de `read(2)`; `wait4(WNOHANG)`; plazos de 10 s y 2 s. Sin fugas: **2→2 goroutines y 5→5 descriptores** en normal, bloqueado, descendiente y `setsid`.
- **N-10:** el líder no se recolecta mientras el PTY siga abierto, y la señal de grupo va antes de `wait4` y `Release`. No hay señales posteriores al reap. El supuesto POSIX sigue vigente: un líder no recolectado reserva su PID/PGID.
- **N-13:** `groupID <= 1` rechazado antes de `kill`. Inventario de señalizaciones, todas ya conocidas: `Kill(-grupo)` con guard, `Process.Kill()` del líder sin recolectar (2 sitios), `Kill` del PID sintético con rechazo de `<= 1`, `Kill(pid, 0)` y el nuevo `Signal(0)`. Ninguno es nuevo. Los grupos señalizados son siempre el creado para la prueba, distinto del grupo del test.
- **N-14:** el límite natural de 5 s, la verificación en 2 s y el marcador siguen. La mutación "terminación no-op" falla `TerminatesBlockedProcess` y `ClosesPTYHeldByDescendant` en 2,7 s. Los descendientes del grupo mueren (vivos al instante, muertos a los 300 ms). El `setsid` sobrevive, como corresponde. 0 procesos residuales.
- **N-18:** fallback `ESRCH` seguro, con `Setpgid=true` y `Pgid=0`. `EPERM` se propaga y el PGID discordante se rechaza.
- **N-19:** la mutación que elimina el guard del seam hace fallar el test con el callback inerte, sin ninguna señal real.

## G. Pruebas

Todo con `GOFLAGS=-mod=readonly`, `GOCACHE` y `TMPDIR` fuera del repositorio.

| Verificación | Resultado |
|---|---|
| `gofmt -l internal/` | Sin salida |
| `go build ./...` | OK |
| `go vet ./...` | OK |
| Focalizadas de pager, PTY, sentinelas, grupo y captura (`-v`) | PASS, 0 Skips |
| `go test -count=1 ./internal/gitmanager` | PASS |
| `go test -race -count=1 ./internal/gitmanager` | PASS |
| `-count=10` de pager y grupo | PASS |
| `go test -count=1 ./...` | Todos `ok` |
| Compilación y `go vet` cruzados | OK en las 7 plataformas |
| Procesos residuales | 0 |

Solo se ejecutó en darwin/arm64. La compilación cruzada no prueba ejecución nativa. FreeBSD usa el stub no POSIX: allí los tests hacen Skip.

## H. Regresiones generales de Fase 1

Sin regresiones:

- **N-01:** el uso de `unsafe.Pointer` no cambió.
- **N-02:** timeout y limpieza siguen acotados.
- **F-01:** M1–M11 pasa, con el mismo hash.
- **F-02:** las siete sentinelas siguen dejando witness y marcador.
- **F-05 y F-06:** pasan, incluido `-race`.
- **T001–T006 e `internal/app`:** inertes. `ports.go` y `internal/app/gitmanagement.go` mantienen sus hashes, y no hay infraestructura nueva ni tareas de Fase 2.

## I. Hallazgos nuevos

Ninguno bloqueante ni de severidad relevante.

| ID | Sev. | Hallazgo |
|---|---|---|
| N-27 | INFO | El probe de N-23 en macOS usa PIDs: no es a prueba de recolección externa más reutilización de PID. Es la limitación D/E ya descrita, que el Paso 85 delimita como contrato. En Linux, `os.Process` usa pidfd y es más fuerte, pero no se ejecutó. |
| N-28 | INFO | La detección de los guards nil (M1 y M7) es por pánico y no por fallo limpio del test. No hay impacto de seguridad. |

## J. Integridad

- **Archivos modificados:** ninguno. El manifiesto SHA-256 del repositorio antes y después es idéntico. Las mutaciones y sondeos se hicieron en copias del scratchpad.
- **Hashes finales (coinciden con los esperados):**

| Archivo | SHA-256 |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `13af569ae6a032c31a7d5fe69b35d456758a10a3457f956ec0404da7e6aba213` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `68e07ad0741b6131f3d242ac67748ef2aa782310bfcfab8ea31e5991d0e955c1` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `ca30e5029eb70cb737ecdbdb79426e5b09812bfae13d179052c567c19d09d4f0` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

- **Git:** índice y diff vacíos. Sin operaciones Git mutantes, sin commits, ramas, tags ni pushes.
- **Normativos, código productivo y dependencias:** intactos. No hay `go.sum`.
- **Fase 2:** no se implementó ni se inició.
- **Informe:** durante la auditoría no se creó dentro del repositorio; este archivo se creó después, a petición expresa.

## K. Recomendación

**READY** para proponer el cierre formal de la Fase 1. No se declara cerrada, ni se acepta deuda en nombre del responsable. Decisiones humanas necesarias:

1. Aceptar el contrato de N-23: no hay reaper externo concurrente. Esto se suma al supuesto POSIX de N-10.
2. Aceptar N-27 y N-28 como informativos.
3. Aceptar como deuda técnica conocida, sin corregir:
   - N-03 y N-04.
   - N-05–N-08.
   - N-11 y N-12.
   - N-15–N-17.
   - N-20–N-22.
   - F-03 y F-04.
   - F-07–F-10.
4. Antes de pedir la Fase 2: adoptar formalmente la línea base (código sin seguimiento, sin commit) y decidir F-03 y F-04 (forma de los puertos), que deben resolverse antes de T016.

---

Este informe no cierra la Fase 1 ni constituye autorización para iniciar la Fase 2, modificar `tasks.md` o crear commits.
