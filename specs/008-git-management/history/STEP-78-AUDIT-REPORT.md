# Informe del Paso 78 — Auditoría independiente de las correcciones N-01 y N-02 (Feature 008, Fase 1)

Fecha: 2026-10-19
Modo: auditoría read-only. Ningún archivo del proyecto fue modificado durante la auditoría. Este archivo se creó después, a petición expresa del usuario.

## A. Veredicto

**PASS WITH CONDITIONS.** N-01 y N-02 están resueltos en lo que el paso se proponía, con una condición: un descendiente que se escape del grupo de procesos todavía puede colgar el helper (N-09).

## B. Cierre de Fase 1

**READY**, con la condición de aceptar N-09 como deuda conocida o corregirla antes de la Fase 2. Ninguno de los hallazgos nuevos afecta a código productivo.

## C. Estado de N-01 y N-02

### N-01: resuelto

- `uintptr(argument)` se convierte dentro de la propia llamada a `syscall.Syscall` (`sentinel_pty_posix_test.go:72`). Con eso se cumple la regla 4 de `unsafe`.
- Ningún wrapper transporta direcciones como `uintptr`; el parámetro es `unsafe.Pointer`.
- `runtime.KeepAlive` queda después de la llamada y es correcto, aunque redundante con esa regla.
- Los buffers `[128]byte`, `int32` y `uint32` tienen el tamaño y la alineación correctos.
- Los números de ioctl no cambiaron. Siguen válidos para darwin y para linux amd64/arm64.
- El master se cierra si algo falla. El slave lo cierra el llamador.
- Los build tags no cambiaron: los dos archivos son complementarios.
- No hay dependencias nuevas.
- `go vet` pasa en 7 combinaciones de plataforma y arquitectura, y las pruebas compilan en las mismas 7.

### N-02: resuelto en el camino normal, con reserva en el camino de fallo

- Hay timeout de 10 s (500 ms en los escenarios sintéticos).
- Se mata el grupo completo con `Setpgid` y `kill(-pid, SIGKILL)`.
- Siempre se hace `Wait` y se espera al lector.
- Un timeout se distingue de un fallo normal con `errors.Is(err, errPagerTimeout)`.
- Los canales son con búfer, así que no quedan goroutines huérfanas en las rutas probadas.
- Tras 25 repeticiones no quedó ningún proceso huérfano.

## D. Evidencia de pruebas independientes

Todo con `GOFLAGS=-mod=readonly`, `GOCACHE` y `TMPDIR` fuera del repositorio, y Git solo en repositorios temporales.

| Verificación | Resultado |
|---|---|
| `gofmt -l internal/` | Sin salida |
| `go build ./...` | OK |
| `go vet ./...` | OK |
| `go test -count=1 ./...` | Todos `ok` |
| Pruebas PTY, pager y sentinelas (`-v`) | PASS, 0 Skips |
| `-count=25` de pager y PTY | PASS (47 s) |
| `go test -race -count=5 ./internal/gitmanager` | PASS |
| Compilación cruzada (linux amd64/arm64/ppc64le/mips64, darwin/amd64, freebsd/amd64, windows/amd64) | 7 plataformas OK |

### Pruebas de mutación de N-02 (copia aislada)

| Mutación | Resultado |
|---|---|
| Matar solo al líder, no al grupo | Detectada, pero como cuelgue de 60 s, no como fallo limpio |
| Quitar `Setpgid` | Detectada, igual (cuelgue) |
| `SIGTERM` en lugar de `SIGKILL` | Detectada, igual (cuelgue) |
| Quitar la limpieza preventiva tras salida normal | No detectada: ninguna prueba cubre esa rama |
| Quitar `master.Close()` en la ruta de timeout | No confirmado: el parche no llegó a aplicarse. Por lectura del código, el cierre es redundante, porque el lector termina al morir los descendientes |

La prueba con un descendiente que hace `setsid` confirmó el cuelgue en `<-readDone`. Esas mutaciones dejaron tres procesos `while :` en bucle (100 % CPU) y un `python3` propios de la auditoría; fueron eliminados y se verificó que no quedó ninguno.

## E. Confirmación de regresiones

- **F-01:** la matriz M1–M11 sigue detectando las 11 violaciones, sin falsos positivos. `boundary_test.go` mantiene el mismo hash que antes del Paso 77.
- **F-02:** las siete sentinelas (`editor`, `pager`, `askpass`, `ssh`, `gpg`, `fsmonitor`, `filter`) ejecutan el helper y dejan witness y marcador.
- **F-05 y F-06:** `fakes_test.go` no cambió y sus pruebas pasan, incluidas las de carrera.
- **T001–T006 e `internal/app`:** inertes y sin cambios.

## F. Hallazgos nuevos

| ID | Sev. | Hallazgo |
|---|---|---|
| N-09 | LOW | La limpieza solo funciona si el `SIGKILL` al grupo tiene efecto. `<-waitDone` y `<-readDone` (`testhelpers_test.go`, rama del timer) no tienen un segundo plazo. Si un descendiente escapa del grupo con `setsid`, o si el `kill` falla, la prueba se cuelga hasta el timeout global de `go test`. Es la misma clase de bloqueo que N-02, reducida a un caso extremo. Se debe a que `master.Fd()` deja el archivo en modo bloqueante, y por eso `master.Close()` no interrumpe una lectura en curso. |
| N-10 | LOW | Tras reapear al líder (`Wait`), `kill(-pid)` podría alcanzar un grupo ajeno si ese PID se reutilizara como líder de grupo. Es una carrera teórica, pero con consecuencias de señal a un grupo equivocado. Es poco probable porque el pid de un grupo con miembros vivos no se reutiliza. |
| N-11 | INFO | Dos mutaciones (limpieza preventiva y `master.Close()` en la ruta de timeout) no están cubiertas por ninguna prueba. |
| N-12 | INFO | En plataformas no POSIX el helper solo mata al proceso directo y `pseudoTerminalProcessExists` devuelve siempre `false`. Es coherente, porque ahí los tests de PTY hacen Skip, pero las comprobaciones de limpieza serían vacías. |

## G. Riesgos residuales y deuda conocida

- **N-09 y N-10:** descritos arriba.
- **Del Paso 76A:**
  - N-03 y N-04 (detector de límites).
  - N-05 (Skip silencioso si no hay PTY).
  - N-06 (sentinela `credential` sin disparar).
  - N-07 (ioctl en ppc/mips sin verificar).
  - N-08 (sin build tag en Windows).
- **Diseño:** F-03, F-04 y F-07–F-10 siguen abiertos.

## H. Estado final

- **Rama:** `feature/008-git-management`.
- **HEAD:** `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- **Hashes (todos coinciden con los esperados):**

| Archivo | SHA-256 |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |
| `internal/gitmanager/testhelpers_test.go` | `526778f4a665abd3d61de7d4560e40b153ea1b7a73e7ae85d708569a1cd6fff3` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `37627b93e9757b0b712ccc7f9b2e8135e1180b0e482e19370b0d19409440e74a` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `53d7c0563f3df7323c84bbc4a5e29937a8ae002dd83b7bc90fcc05505c5c1274` |

- **Git:** 29 archivos sin seguimiento al cierre de la auditoría (con este informe, 30), cero cambios rastreados, índice y diff vacíos. Sin commits, ramas ni tags.
- **Alcance:** en el Paso 77 solo cambiaron los tres archivos autorizados (por mtime y hash). `ports.go`, `doc.go` y `internal/app` están intactos. No se modificaron documentos normativos ni se añadieron dependencias.
- **No modificado:** el manifiesto SHA-256 del repositorio antes y después de la auditoría es idéntico. Las mutaciones se hicieron en una copia en el scratchpad.

## I. Recomendación para el siguiente paso

Declarar la Fase 1 técnicamente cerrada si se aceptan N-09 y N-10 como deuda. Son defectos de infraestructura de pruebas, acotados por el timeout de `go test`, que solo afloran con un descendiente que escape del grupo.

Antes de pedir la autorización de la Fase 2:

1. Decidir si se adopta formalmente la línea base (decisión pendiente del Paso 76A).
2. Registrar N-03, N-04 y N-09–N-12 como deuda.
3. Mantener F-03 y F-04 (forma de los puertos) como decisión de entrada a la Fase 2, antes de T016.

Alternativa: cerrar N-09 antes con una corrección pequeña, un segundo plazo en las dos esperas.

---

Este informe no constituye adopción de la línea base ni autorización para iniciar la Fase 2, modificar `tasks.md` o crear commits.
