# Informe del Paso 84 — Auditoría independiente de N-18 y N-19 (Feature 008, Fase 1)

Fecha: 2026-10-22
Modo: auditoría read-only. Ningún archivo del proyecto fue modificado durante la auditoría. Este archivo se creó después, a petición expresa del usuario.

## A. Veredicto

**PASS WITH CONDITIONS.** N-18 y N-19 funcionan y no se encontraron regresiones. Quedan tres hallazgos nuevos (LOW/INFO) y una afirmación del Paso 83 que el código no hace cumplir.

| Hallazgo | Estado |
|---|---|
| N-18 | Verificado con reserva (ver sección C) |
| N-19 | Verificado |
| N-09 | Sin regresión |
| N-10 | Sin regresión |
| N-13 | Sin regresión |
| N-14 | Sin regresión |

**Para proponer el cierre técnico de Fase 1: READY**, con las condiciones de la sección J. No se declara cerrada la Fase 1.

## B. Preflight

- **Rama y HEAD:** `feature/008-git-management`, `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- **Hashes:** los siete coinciden con los esperados.
- **Git:** índice y diff vacíos, 35 archivos sin seguimiento (34 previos más `STEP-83-REPORT.md`).
- **Ejecución:** no root (uid 501). No existe `go.sum`, como antes.
- **Alcance del Paso 83:** por mtime, solo cambiaron `testhelpers_test.go`, `sentinel_pty_posix_test.go` y `sentinel_pty_other_test.go`. `boundary_test.go`, `fakes_test.go`, `ports.go` e `internal/app` no cambiaron.
- **Código productivo, normativos y dependencias:** intactos.
- **Fases 2–12:** sin implementación anticipada.

## C. N-18: captura del PGID

### Análisis del fallback (`sentinel_pty_posix_test.go:90-111`)

| Condición declarada | Resultado |
|---|---|
| Proceso no nil y PID > 1 | Verificada en el código. |
| Se conserva el `SysProcAttr` del `Start` | **Parcial.** El puntero lo pasa el llamador y la función no lo ata al proceso. En el único uso real es el mismo objeto asignado a `command.SysProcAttr`. |
| `Setpgid == true` y `Pgid == 0` | Verificada, y se comprueba **antes** de consultar `Getpgid`. |
| `Getpgid` devuelve `ESRCH` | Verificada con `errors.Is`, equivalente a la igualdad exacta para `Errno`. |
| No hubo `Wait`, `wait4` ni `Release` | **No se hace cumplir.** Ver N-23. |
| `EPERM` y otros errores se propagan | Verificada: test y mutación. |
| PGID discordante se rechaza | Verificada: test y mutación. |

### Garantías del toolchain

En Go 1.27.1, `exec_bsd.go:116` (macOS/BSD) y `exec_linux.go:393` ejecutan `setpgid(0, Pgid)` en el hijo antes de `exec`. Si falla, `Start` devuelve error. Con `Pgid == 0`, el hijo toma su propio PID. Por tanto, un `Start` exitoso con esos atributos implica `PGID == PID` al ejecutarse. Esto es una garantía verificada en el código fuente, no una inferencia.

### Seguridad de la identidad

El fallback se invoca inmediatamente después de `Start`, antes de cualquier recolección, así que el líder (vivo o zombie) mantiene reservado el PID. Si el líder hiciera `setsid`, el PGID sería igualmente su PID, y señalar `-PID` solo puede alcanzar un grupo con ese identificador. No hay ventana nueva de reutilización ni camino a grupos ajenos.

### Evidencia

| Prueba | Resultado |
|---|---|
| Mutación: quitar la comprobación de atributos | Detectada. |
| Mutación: tragar todos los errores de lookup | Detectada. |
| Mutación: aceptar PGID discordante | Detectada. |
| Mutación: quitar `Pid <= 1` | **No detectada.** Ver N-25. |
| Sondeo real en macOS (`/usr/bin/true` + 200 ms, 5 veces) | `Getpgid` crudo devuelve `ESRCH` y el helper devuelve el PID correcto, sin error. |
| Entradas inválidas (nil, `-1`, `0`, `1`; atributos nil, vacíos, `Pgid` no cero, sin `Setpgid`) | Todas rechazadas sin llamar al lookup. |
| Prueba de salida inmediata (50 procesos) | **No ejercita el fallback.** Instrumentada, vio `ESRCH` real en **0 de 50** iteraciones: el arranque del binario de test tarda más que `Getpgid`. Ver N-26. |

### Respuesta a la pregunta central

Sí, N-18 elimina la intermitencia sin debilitar N-10 ni N-13 en el uso real. La cobertura determinista del fallback viene de la inyección y del sondeo con un proceso real, no de las "500 salidas inmediatas".

## D. N-19: seam de terminación

### Diseño

`terminatePseudoTerminalProcessGroupWith(groupID, terminate)` valida primero (`groupID <= 1` y callback nil) y solo entonces invoca el callback. `terminatePseudoTerminalProcessGroup` lo compone con el único punto que llama a `syscall.Kill(-validGroupID, SIGKILL)`. Las responsabilidades están separadas correctamente.

- Los tests con `-1`, `0` y `1` usan un callback inerte. Ninguna ruta de esos tests alcanza `syscall.Kill`, y el callback no se invoca con identificadores rechazados.
- Un identificador válido (42) llega al callback.
- No hay variables globales mutables, y la prueba pasa bajo `-race`.
- Los tests positivos con PTY siguen cubriendo la terminación real.

### Prueba negativa independiente

En una copia aislada se eliminó el guard del seam y se ejecutó solo `TestProcessGroupTerminationRejectsUnsafeIdentifiers`. El test falló con `injected terminator called 3 times` y mensajes de que `-1`, `0` y `1` fueron aceptados. El callback es inerte, así que no se envió ninguna señal real: ni a PID 1, ni al grupo del test, ni a procesos ajenos.

### Respuesta a la pregunta central

Sí. N-19 quedó corregido: el test es seguro frente a la regresión que pretende detectar.

### Matices

- `pagerRunOptions.terminateGroup` es un `func(int) error` inyectable que no pasa por el guard. Se usa con funciones inertes o con la versión guardada.
- Persiste el comportamiento de N-20: la función real acepta cualquier `groupID > 1`.

## E. Regresiones N-09, N-10, N-13 y N-14

- **N-09:**
  - Sin goroutine lectora.
  - `select(2)` antes de `read(2)`, `wait4(WNOHANG)`, plazos de 10 s y 2 s.
  - Sin fugas: **2→2 goroutines y 5→5 descriptores** en normal, bloqueado, descendiente y `setsid`.
  - Los escenarios con timeout terminan en unos 0,6 s.
- **N-10:**
  - El PGID se captura antes de cualquier recolección.
  - Con el PTY abierto, el líder no se recolecta (solo se llama a `wait4` cuando `readFinished`).
  - La señal de grupo va antes de `wait4`/`Release`.
  - No hay señales tras el reap.
  - El supuesto POSIX (un líder zombie reserva su PGID) sigue siendo la única condición.
- **N-13:**
  - `groupID <= 1` rechazado antes de `kill`.
  - Los errores se propagan.
  - Los puntos de señalización son los mismos que en el Paso 82, sin sitios nuevos.
- **N-14:**
  - Límite natural de 5 s, verificación en 2 s y marcador natural intactos.
  - La prueba negativa con terminación desactivada pasa a los 250 ms.
  - Tras volver el helper, el descendiente del grupo está muerto a los 300 ms y el `setsid` sigue vivo, como corresponde.
  - 0 procesos residuales.

## F. Pruebas

Todo con `GOFLAGS=-mod=readonly`, `GOCACHE` y `TMPDIR` fuera del repositorio.

| Verificación | Resultado |
|---|---|
| `gofmt -l internal/` | Sin salida |
| `go build ./...` | OK |
| `go vet ./...` | OK |
| Focalizadas de pager, PTY, sentinelas, grupo y captura (`-v`) | PASS, 0 Skips |
| `go test -race -count=1 ./internal/gitmanager` | PASS (61 s) |
| `-count=10` de pager y grupo | PASS |
| `go test -count=1 ./...` | Todos `ok` |
| Compilación y `go vet` cruzados | OK en las 7 plataformas |
| Procesos residuales | 0 |

Solo se ejecutó en darwin/arm64. La compilación cruzada no prueba ejecución nativa. FreeBSD queda en el stub no POSIX: esos tests hacen Skip allí.

## G. Regresiones de Fase 1

Sin regresiones:

- **N-01:** el uso de `unsafe.Pointer` no cambió.
- **N-02:** timeout y limpieza siguen acotados.
- **F-01:** M1–M11 pasa, con el mismo hash.
- **F-02:** las siete sentinelas siguen dejando witness y marcador.
- **F-05 y F-06:** pasan, incluido `-race`.
- **T001–T006 e `internal/app`:** inertes, sin cambios en puertos ni composición.

## H. Hallazgos nuevos

| ID | Sev. | Archivo | Hallazgo y recomendación |
|---|---|---|---|
| N-23 | LOW | `sentinel_pty_posix_test.go:90-111` | El Paso 83 afirma que el fallback exige que no se haya llamado a `Wait` ni `wait4`, pero el código no lo hace cumplir. Sondeado: tras `cmd.Wait()` el PID sigue siendo > 1, `Getpgid` devuelve `ESRCH` y el helper devuelve el PID reapeado como PGID **sin error**. Solo `Release()` lo bloquea, porque pone `Pid = -1`. Hoy no es explotable, porque la función se llama una vez, inmediatamente después de `Start`. Es una garantía por orden de llamada, no por construcción. **Recomendación:** ante `ESRCH`, confirmar que el proceso existe con `kill(pid, 0)` (verificado: un zombie devuelve éxito y un proceso recolectado devuelve `ESRCH`), o documentar el contrato. |
| N-25 | INFO | `testhelpers_test.go`, captura de PGID | No hay test para `PID <= 1` ni para procesos nil en `pseudoTerminalProcessGroupIDWithLookup`. Quitar esa comprobación no hace fallar ninguna prueba. El código es correcto (comprobado con entradas inválidas) y hay un segundo guard en la terminación, pero la rama no está cubierta. |
| N-26 | LOW | `testhelpers_test.go`, `TestPseudoTerminalProcessGroupCaptureImmediateExit` | No ejerce el fallback `ESRCH`: 0 de 50 iteraciones lo observaron, porque lanza el binario de test (decenas de ms) y `Getpgid` corre al instante. Su nombre sugiere una cobertura que no ofrece. Además, 50 procesos suben el tiempo de `-race` de unos 9 s a unos 61 s. **Recomendación:** renombrarla o reducirla; la cobertura determinista ya la dan los casos inyectados. |

## I. Integridad

- **Archivos modificados:** ninguno. El manifiesto SHA-256 del repositorio antes y después es idéntico. Las mutaciones y sondeos se hicieron en copias del scratchpad, sin enviar señales peligrosas.
- **Hashes finales (coinciden con los esperados):**

| Archivo | SHA-256 |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `d5454856f637c3c56a6b1c9ca593f4998d7c41d5a3da1cca5401609cae491602` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `fabccb5ee3b3c122fab3a34c787a8b1653915f729ca296c96f1acce54e12b170` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `d04d457d2057ae20cf2749984c69df6b9f39bae1b357fa4abaad351bfbc8c9ee` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

- **Git:** índice y diff vacíos. Sin commits, ramas, tags ni operaciones Git mutantes. Los repositorios de prueba son temporales.
- **Normativos, código productivo y dependencias:** intactos.
- **Fase 2:** no se implementó ni se inició.
- **Informe:** durante la auditoría no se creó dentro del repositorio; este archivo se creó después, a petición expresa.

## J. Recomendación

**READY** para proponer el cierre técnico de la Fase 1, sujeto a estas decisiones humanas:

1. Aceptar N-23, N-25 y N-26 como deuda de infraestructura de pruebas, o corregirlas antes. N-23 es la única que toca una garantía declarada por el Paso 83.
2. Mantener el supuesto POSIX de N-10: un líder no recolectado reserva su PID y PGID.
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
