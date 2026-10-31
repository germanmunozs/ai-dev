# Informe del Paso 80 — Auditoría independiente de N-09 y N-10 (Feature 008, Fase 1)

Fecha: 2026-10-21
Modo: auditoría read-only. Ningún archivo del proyecto fue modificado durante la auditoría. Este archivo se creó después, a petición expresa del usuario.

## A. Veredicto

**FAIL.** El cuelgue de N-09 está resuelto, pero la corrección introdujo un defecto nuevo y más grave (N-13). Todos los tests pasan igualmente: el defecto no se ve en la suite porque los helpers de prueba terminan solos antes de que importe si la limpieza funcionó.

- **N-09 (esperas indefinidas):** corregido.
- **N-13, nuevo:** `kill(1, SIGKILL)` y limpieza de descendientes inoperante. Es la razón del FAIL.
- **N-10:** su descripción en el Paso 79 es incorrecta.
- **Cierre de Fase 1:** **NOT READY.**

## B. Preflight

- **Rama y HEAD:** `feature/008-git-management`, `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- **Hashes:** los siete coinciden con los esperados.
- **Git:** índice y diff vacíos, 31 archivos sin seguimiento (STEP-79 se agregó al conjunto previo).
- **Normativos y Fases 2–12:** sin cambios ni implementación anticipada.
- **Alcance del Paso 79:** por mtime, solo cambiaron los tres archivos autorizados. `boundary_test.go`, `fakes_test.go`, `ports.go` e `internal/app` no cambiaron.

## C. Estado de N-09, por mecanismo

| Afirmación del Paso 79 | Resultado |
|---|---|
| Sin goroutine lectora ni `waitDone`/`readDone` | **Verificado.** Goroutines y descriptores antes y después de los escenarios: 2→2 y 5→5. |
| `select(2)` con plazo cero | **Verificado.** `Timeval` en cero, con manejo de `EINTR`. El `FdSet` se manipula con `reflect` para cubrir las distintas firmas por plataforma; funciona con `int32` e `int64` y valida `fd` contra la capacidad del set. |
| PTY master no bloqueante | **Falso en la práctica.** Cada llamada a `master.Fd()` (en `poll` y en `read`) borra `O_NONBLOCK`. Comprobado con `fcntl(F_GETFL)`: false antes, true tras `SetNonblock`, false tras otro `Fd()`. Sigue siendo seguro porque `select` va antes de `read` y hay un solo lector. |
| Una lectura por iteración | **Verificado**, pero limita el caudal (ver abajo). |
| `wait4(WNOHANG)` | **Verificado.** Maneja `EINTR`, `pid==0`, salida normal, error y señal. `Release()` va después de recolectar, sin doble recolección. `ECHILD` persistente no entra en bucle infinito: queda acotado por los plazos. |
| Sondeo de 5 ms | **Verificado.** `time.Ticker` y `time.Timer`, monótonos: el reloj es apropiado. |
| Plazos de 10 s y 2 s | **Verificados.** Entre cada comprobación de plazo solo hay operaciones no bloqueantes. Un fallo de terminación se acotó a unos 2,6 s. |
| Error explícito si no recolecta | **Verificado.** Devuelve `processReaped=false` con `errors.Join(...)`. |

### Caudal de salida abundante (escenario L), medido

| Salida del pager | Tiempo | Resultado |
|---|---|---|
| 64 KiB | 0,66 s | Completa |
| 256 KiB | 1,7 s | Completa |
| 1 MiB | 5,5 s | Completa |
| 4 MiB | 10,1 s | Timeout, 1,98 MB capturados |

El caudal es de unos 200 KB/s. Es un límite de diseño (4 KiB por tick de 5 ms) y falla de forma explícita, no con un cuelgue.

## D. Estado de N-10 (solo lectura, sin provocar reutilización de PID)

La descripción del Paso 79 está equivocada. Describe una carrera teórica de reutilización de PID, pero el defecto real ocurre siempre.

**Mecanismo (N-13):** `pollPseudoTerminalProcess` llama a `process.Release()`, y `Release()` pone `Process.Pid = -1`. Después, `testhelpers_test.go:503` y `:516` llaman a `options.terminateGroup(command.Process.Pid)`. Con `pid = -1`, `syscall.Kill(-pid, SIGKILL)` equivale a **`kill(1, SIGKILL)`**.

Comprobado instrumentando una copia aislada, que imprimió `kill(--1)=operation not permitted`. Esa es la señal a PID 1, rechazada solo porque el usuario no es root.

Las dos rutas siguen presentes:
1. Finalización normal, tras recolectar al líder. Ocurre **en cada ejecución exitosa del pager.**
2. Timeout cuando el líder ya había terminado.

No se encontraron rutas adicionales. La señal que ocurre antes de recolectar sí usa el PID válido.

**Clasificación:**
- **Como estaba descrito (reutilización de PID):** LOW sigue siendo razonable. `wait4(WNOHANG)` además ensancha la ventana entre recolección y señal con salidas grandes, porque el drenaje es lento.
- **Como es en realidad (N-13):** deja de ser LOW.

**Alternativas apropiadas, sin implementar:** capturar el PID antes de `Release()`, señalizar antes de recolectar, o usar identidad estable de plataforma. Futuras correcciones deben tratar N-13 y N-10 juntas antes de depender de esta infraestructura.

## E. Pruebas independientes

Todo con `GOFLAGS=-mod=readonly`, `GOCACHE` y `TMPDIR` fuera del repositorio. Los helpers sintéticos se autolimitan a 1,5 s. Se verificaron 0 procesos residuales al terminar.

| Verificación | Resultado |
|---|---|
| `gofmt -l internal/` | Sin salida |
| `go build ./...` | OK |
| `go vet ./...` | OK |
| Pruebas PTY, pager y sentinelas (`-v`) | PASS, 0 Skips |
| `go test -race -count=1 ./internal/gitmanager` | PASS |
| `-count=5` de pager, PTY y sentinelas | PASS |
| `go test -count=1 ./...` | Todos `ok` |
| Compilación y `go vet` cruzados | 13 combinaciones OK: las 7 documentadas más linux/386, arm, riscv64, s390x, loong64 y darwin/arm64 |

La compilación cruzada no prueba ejecución real en esas plataformas. Solo se ejecutó en darwin/arm64.

### Escenarios A–L

| Escenario | Resultado |
|---|---|
| A, F, G | Pasan. |
| B (pager bloqueado, líder vivo) | La limpieza funciona: el helper queda eliminado en 0,7 s. |
| C (descendiente en el grupo, líder ya terminado) | La limpieza **no se ejecuta**. El descendiente sigue vivo (`pgid` del líder, `ppid=1`) tras el retorno y muere solo a los 1,84 s. El test tarda 2,0 s. |
| D (`setsid`) | Acotado, pero el descendiente sobrevive por diseño. Muere a los 1,5 s. |
| E (fallo simulado de terminación) | Acotado. |
| H | El líder se recolecta. |
| I | El segundo plazo funciona. |
| J | Sin procesos residuales. |
| K | Sin fugas de goroutines ni de descriptores. |
| L | Medido arriba. |

**Diagnóstico confirmado en una copia:** capturando el PID antes de `Release()`, el escenario C baja de 2,00 s a 0,71 s y el descendiente muere de inmediato.

Los tests actuales no distinguen una limpieza que funciona de una que no. Los descendientes terminan solos a los 1,5 s, y `assertProcessExited` espera hasta 5 s. Cuando el líder aún vive, el respaldo `command.Process.Kill()` termina al líder y el resto expira solo. Las afirmaciones del Paso 79 sobre "descendientes eliminados y comprobados por PID" no están demostradas.

No se ejecutaron mutaciones sin contención ni nada que provocara reutilización de PID. Tampoco se ejecutó nada como root.

## F. Regresiones de Fase 1

Sin regresiones:

- **N-01:** `uintptr(argument)` sigue dentro de `syscall.Syscall`, con `runtime.KeepAlive`.
- **F-01:** M1–M11 sigue pasando, con el mismo hash.
- **F-02:** las siete sentinelas ejecutan el helper y dejan witness y marcador.
- **F-05 y F-06:** pasan, incluso con `-race`.
- **T001–T006 e `internal/app`:** inertes. Los puertos y las interfaces productivas no cambiaron.

## G. Hallazgos nuevos

| ID | Sev. | Hallazgo |
|---|---|---|
| **N-13** | **HIGH** | `testhelpers_test.go:503` y `:516`. Tras `Release()`, `Process.Pid == -1`, y `terminatePseudoTerminalProcessGroup(-1)` llama a `syscall.Kill(1, SIGKILL)` (`sentinel_pty_posix_test.go:87`). Se ejecuta en cada pager exitoso. Como usuario normal devuelve `EPERM`, que el código descarta con `_ =`. Si la suite corriera como root, intentaría matar PID 1. En Linux ese kill se ignora dentro del mismo namespace; en macOS root no se verificó ni se probó. Además, anula la limpieza de descendientes cuando el líder ya terminó. Es una regresión del Paso 79: el Paso 77 usaba `Wait()` y conservaba el PID. **Recomendación:** capturar el PID antes de `Release()` y rechazar PIDs ≤ 1 en la función de terminación. |
| N-14 | MEDIUM | Los tests de limpieza son vacuos. Los descendientes se autolimitan a 1,5 s y la comprobación de PID espera 5 s, así que pasan sin importar si la señal llegó. Por eso N-13 pasó desapercibido. **Recomendación:** que el helper dure más que `assertProcessExited` y comprobar que muere antes de su propio límite. |
| N-15 | LOW | La afirmación "master no bloqueante" es incorrecta, porque `Fd()` borra `O_NONBLOCK` en cada llamada. Sigue siendo seguro por el `select` previo. |
| N-16 | LOW | Caudal de lectura de unos 200 KB/s. Más de ~2 MiB en 10 s da un falso timeout. |
| N-17 | INFO | `readerFinished` se pone a `true` incondicionalmente en las rutas de timeout, aunque quede un descendiente con el PTY abierto. |

## H. Riesgos residuales y deuda conocida

Siguen abiertos y sin tocar:

- N-03 y N-04.
- N-05–N-08.
- N-10.
- N-11 y N-12.
- F-03 y F-04.
- F-07–F-10.

## I. Confirmación de integridad

- **Archivos modificados:** ninguno. El manifiesto SHA-256 de todo el repositorio antes y después de la auditoría es idéntico. Los experimentos se hicieron en copias en el scratchpad.
- **Git:** índice y diff vacíos. Sin commits, ramas, tags ni operaciones Git mutantes. Los repositorios de prueba son temporales.
- **Hashes finales (coinciden con los esperados):**

| Archivo | SHA-256 |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `8abd6ac0d15c178f5cfcf2123e84802a42e97d54fe2d21578f8df23660c17675` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `a078c64dae9db43ca5237cee4ae4d0e6d0afa4516174c7e467b7d6d04f75e266` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `6050be14b5d15fe98ad3a836ca9de288e2b3d551e05cc1710ba0054c6c4fdf10` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

- **Fase 2:** no se implementó ni se inició.
- **Informe:** durante la auditoría no se creó dentro del repositorio; este archivo se creó después, a petición expresa.

## J. Recomendación final

**NOT READY** para proponer el cierre técnico de la Fase 1.

Condiciones pendientes de decisión humana:

1. **Corregir N-13 y N-14 antes de cerrar la Fase 1.** Es una corrección mínima: capturar el PID antes de `Release()`, rechazar PIDs ≤ 1 y volver el test de limpieza no vacuo. Tratar N-10 en el mismo paso.
2. Aceptar o corregir N-15 y N-16 como deuda de infraestructura de pruebas.
3. Después, una verificación focalizada. N-09 ya está demostrado, así que no hace falta repetir la auditoría completa.
4. La autorización de la Fase 2 sigue pendiente.

---

Este informe no cierra la Fase 1 ni constituye autorización para iniciar la Fase 2, modificar `tasks.md` o crear commits.
