# Informe del Paso 82 — Auditoría independiente de N-13, N-14 y N-10 (Feature 008, Fase 1)

Fecha: 2026-10-22
Modo: auditoría read-only. Ningún archivo del proyecto fue modificado durante la auditoría. Este archivo se creó después, a petición expresa del usuario.

## A. Veredicto

**PASS WITH CONDITIONS.** N-13, N-14 y N-10 están corregidos y se verificaron de forma independiente. N-09 sigue resuelto. Quedan hallazgos nuevos menores y supuestos dependientes del sistema operativo que requieren aceptación humana.

| Hallazgo | Estado |
|---|---|
| N-13 | Verificado |
| N-14 | Verificado |
| N-10 | Eliminado en este helper, bajo un supuesto POSIX |
| N-09 | Sin regresión |

**Para proponer el cierre técnico de Fase 1: READY**, con las condiciones de la sección K. No se declara cerrada la Fase 1.

## B. Preflight

- **Rama y HEAD:** `feature/008-git-management`, `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- **Hashes:** los siete coinciden con los esperados.
- **Git:** índice y diff vacíos, 33 archivos sin seguimiento (los 31 previos más STEP-80 y STEP-81).
- **Usuario de ejecución:** uid 501, no root.
- **Alcance del Paso 81:** por mtime, solo cambiaron los tres archivos autorizados (`testhelpers_test.go`, `sentinel_pty_posix_test.go`, `sentinel_pty_other_test.go`). `boundary_test.go`, `fakes_test.go`, `ports.go` e `internal/app` no cambiaron.
- **Fases 2–12:** sin implementación anticipada.

## C. N-13: señalización segura

**Verificado.** Las afirmaciones del Paso 81 coinciden con el código:

- El PGID se captura justo después de `Start` (`testhelpers_test.go:549`, `pseudoTerminalProcessGroupID`).
- Se exige `Pid > 1`, `Getpgid` sin error, `PGID > 1` y `PGID == PID`. Cualquier fallo devuelve error y no señala.
- `terminatePseudoTerminalProcessGroup` rechaza `groupID <= 1` **antes** de `kill(2)`, así que no se puede construir `kill(1)`, `kill(0)` ni `kill(-1)`. Cubre cero, uno, negativos y extremos, porque el guard es `<= 1` en `int`.
- Ninguna ruta usa `Process.Pid` tras `Release()`. Los únicos usos son en la captura inicial y en `Wait4`, que nunca se llama después de recolectar.
- Los errores de terminación se unen al resultado con `errors.Join`.
- Nada depende de ser root.

### Todos los puntos de señalización del repositorio

| Sitio | Qué señala | Seguro porque |
|---|---|---|
| `sentinel_pty_posix_test.go:104` | `kill(-groupID, SIGKILL)` | Tiene guard `<= 1`. |
| `testhelpers_test.go:551` | `Process.Kill()` del líder | El líder no está recolectado. |
| `testhelpers_test.go:595` | `Process.Kill()` del líder | El líder no está recolectado. |
| `testhelpers_test.go:759` | `Kill` del PID sintético | Se rechaza `pid <= 1`. |
| `sentinel_pty_posix_test.go:112` | `kill(pid, 0)` | Es solo comprobación de existencia. |

Se instrumentó `terminatePseudoTerminalProcessGroup` en una copia y se corrió la suite dos veces. Se registraron 6 señalizaciones de grupo, todas con `groupID > 1` y distinto del grupo del propio proceso de test (`unsafe_targets=0`). La ruta normal no emite ninguna señal.

Los argumentos peligrosos se verificaron por inspección estructural del guard y por el test existente. Ver N-19: ese test llama a la función real.

## D. N-14: pruebas no vacuas

**Verificado.**

- **Límite natural:** el holder vive 5 s y escribe un marcador de salida natural solo al terminar ese temporizador. Es independiente de la limpieza.
- **Plazo de verificación:** 2 s, menor que el límite natural. Si el proceso sigue vivo a los 2 s, o si existe el marcador natural, falla.
- **Mutaciones en copia aislada** (sin enviar señales peligrosas):

| Mutación | Resultado |
|---|---|
| Terminación convertida en no-op | Fallan `TerminatesBlockedProcess` y `ClosesPTYHeldByDescendant`, en 2,7 s. Fallos limpios, sin cuelgue. |
| Señalar solo al líder (destino incorrecto, positivo y propio) | Fallan los mismos dos tests. |

- **Prueba negativa del repo** (`RejectsDisabledGroupTermination`): el verificador rechaza la limpieza a los 250 ms, así que no es vacua.
- **Terminación real de descendientes:** tras volver el helper, el holder del escenario C está vivo al instante (zombie por la comprobación `kill 0`) y **muerto a los 300 ms**. El holder `setsid` sigue vivo, como corresponde.
- **Determinismo:** pasa con `-race` y en 10 repeticiones.
- **Aislamiento de `setsid`:** el test comprueba que el holder sobrevive al grupo y lo termina por PID explícito, sin señalar grupos.
- **Residuos:** 0 procesos tras todas las ejecuciones.

## E. N-10: identidad PID/PGID

La secuencia real coincide con la descrita:

1. `Start` con `Setpgid`, y luego captura y validación del PGID.
2. Mientras el lector PTY siga abierto, **el líder no se recolecta** (`testhelpers_test.go:566-573`: solo se llama a `wait4` cuando `readFinished`).
3. En timeout se señala el grupo (`:593`) **antes** de `wait4` y `Release`.
4. Después se recolecta en el bucle de limpieza, acotado por 2 s.
5. En salida normal se cierra el lector, se recolecta y no hay señales posteriores.

### Rutas probadas

- **El líder sale antes que el descendiente:** queda zombie sin recolectar durante la señalización, así que el PGID sigue reservado.
- **Falla `kill`:** el error se une al resultado y el helper sigue acotado.
- **Falla `wait4`:** el plazo de limpieza de 2 s lo acota.
- **Descendiente con `setsid`:** queda fuera del grupo por diseño, y el test lo termina por PID conocido.
- **Señales posteriores a la recolección:** no hay en ninguna ruta. Ninguna señaliza un grupo que ya no pertenece al test.

### Riesgo residual

Pasó de "reducido" a **eliminado en este helper**, pero la garantía depende de un supuesto POSIX: un líder zombie no recolectado mantiene reservado su PID y por tanto su PGID. Es un supuesto estándar en macOS y Linux. No se provocó reutilización de PID ni se señalizaron grupos ajenos, y no se ejecutó nada como root.

## F. N-09: sin regresión

- No hay goroutine lectora ni `waitDone`/`readDone`.
- Se mantiene `select(2)` con plazo cero antes de `read(2)`, `wait4(WNOHANG)`, el sondeo de 5 ms, los plazos de 10 s y 2 s y el error explícito si no se recolecta.
- La nueva secuencia no retrasa indefinidamente la recolección: si el lector nunca termina, el timeout de 10 s dispara la limpieza y la recolección queda dentro de los 2 s siguientes.
- El escenario `setsid` termina en 0,6–0,7 s.
- No hay fugas: 2→2 goroutines y 5→5 descriptores en los cuatro escenarios.

## G. Pruebas

Todo con `GOFLAGS=-mod=readonly`, `GOCACHE` y `TMPDIR` fuera del repositorio.

| Verificación | Resultado |
|---|---|
| `gofmt -l internal/` | Sin salida |
| `go build ./...` | OK |
| `go vet ./...` | OK |
| Tests focalizados de pager, PTY, sentinelas y grupo | PASS, 0 Skips |
| `go test -race -count=1 ./internal/gitmanager` | PASS |
| `-count=10` de pager y grupo | PASS |
| `go test -count=1 ./...` | Todos `ok` |
| Compilación y `go vet` cruzados | OK en las 7 plataformas documentadas |
| Procesos residuales | 0 |

Solo se ejecutó en darwin/arm64. La compilación cruzada no prueba ejecución nativa.

## H. Regresiones de Fase 1

Sin regresiones:

- **N-01:** `unsafe.Pointer` sigue dentro de `syscall.Syscall`, con `KeepAlive`.
- **N-02 y N-09:** timeout y limpieza acotados.
- **F-01:** M1–M11 sigue pasando, con el mismo hash.
- **F-02:** las siete sentinelas dejan witness y marcador.
- **F-05 y F-06:** pasan, incluido `-race`.
- **T001–T006 e `internal/app`:** inertes. Los puertos y las interfaces productivas no cambiaron.

## I. Hallazgos nuevos

| ID | Sev. | Archivo | Hallazgo y recomendación |
|---|---|---|---|
| N-18 | LOW | `sentinel_pty_posix_test.go:90`, `testhelpers_test.go:549` | En macOS, `Getpgid` sobre un líder zombie devuelve `ESRCH` (reproducido con `/usr/bin/true`). Si Git saliera antes de la captura del PGID, el helper devuelve error aunque no haya nada incorrecto. La ventana es de microsegundos y el pager real tarda milisegundos, así que es una posible fuente de intermitencia. En la ruta de error se llama a `command.Process.Wait()` sin plazo, tras un `Kill` al líder, lo que es aceptable. **Recomendación:** tolerar `ESRCH` usando el PID como PGID, que es seguro porque `Setpgid` lo garantiza, o documentarlo. |
| N-19 | LOW | `testhelpers_test.go`, `TestProcessGroupTerminationRejectsUnsafeIdentifiers` | El test llama a la función real con `-1`, `0` y `1`. Hoy es seguro por el guard. Si el guard se regresara, ese test enviaría `kill(0, SIGKILL)` al propio grupo del test, y `kill(1)` o `kill(-1)` a objetivos peores. Ante una regresión del guard, el test mismo sería peligroso. **Recomendación:** inyectar la función de señalización en vez de llamar a `syscall.Kill` real. |
| N-20 | INFO | `sentinel_pty_posix_test.go:100` | `terminatePseudoTerminalProcessGroup` acepta cualquier `groupID > 1`, incluido el grupo del propio proceso de test. No es alcanzable con los llamadores actuales, que solo pasan el PGID capturado. |
| N-21 | INFO | `testhelpers_test.go:759` | `terminateSyntheticProcess` mata por PID leído de un archivo, tras una comprobación de existencia. Hay una carrera de reutilización de PID de microsegundos y, en la práctica, inofensiva. |
| N-22 | INFO | `sentinel_pty_posix_test.go:165` | `pollPseudoTerminalProcess` no protege contra `Pid <= 0`, pero hoy no es alcanzable. Además, la rama `if !waitFinished` del timeout es siempre verdadera: es código muerto. |

## J. Integridad

- **Archivos modificados:** ninguno. El manifiesto SHA-256 del repositorio antes y después es idéntico. Los experimentos se hicieron en copias del scratchpad, que contenían instrumentación y mutaciones.
- **Hashes finales (coinciden con los esperados):**

| Archivo | SHA-256 |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `ba2e0df9c584ee1178cffbefe0598367b37ecf44a32bb98977bc0859dedf234c` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `a69bc64e99a336969d57937e66bf714a28b6084fe0acbc949e73000dbb0a32f6` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `bbb12ccb4d6642dfef01d9510c93ef6f32fc5f07ce90b7c1d00fdc5994179383` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

- **Git:** índice y diff vacíos. Sin commits, ramas, tags ni operaciones Git mutantes. Los repositorios de prueba son temporales.
- **Normativos:** sin cambios.
- **Fase 2:** no se implementó ni se inició.
- **Informe:** durante la auditoría no se creó dentro del repositorio; este archivo se creó después, a petición expresa.

## K. Recomendación

**READY** para proponer el cierre técnico de la Fase 1, sujeto a estas decisiones humanas:

1. Aceptar N-18 y N-19 como deuda de infraestructura de pruebas, o corregirlas antes. Son de baja prioridad.
2. Aceptar el supuesto POSIX de N-10: un líder zombie no recolectado mantiene reservado su PGID.
3. Mantener como deuda conocida:
   - N-03 y N-04.
   - N-05–N-08.
   - N-11 y N-12.
   - N-15–N-17.
   - N-20–N-22.
   - F-03 y F-04.
   - F-07–F-10.
4. Antes de pedir la Fase 2, adoptar la línea base formalmente y decidir F-03 y F-04 (forma de los puertos), que deben resolverse antes de T016.

---

Este informe no cierra la Fase 1 ni constituye autorización para iniciar la Fase 2, modificar `tasks.md` o crear commits.
