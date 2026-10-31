# Informe del Paso 76A — Auditoría read-only de procedencia e integridad (Feature 008, Fase 1)

Fecha: 2026-10-19
Modo: auditoría read-only. Ningún archivo del proyecto fue modificado durante la auditoría. Este archivo se creó después, a petición expresa del usuario.

## A. Veredicto

**PASS WITH CONDITIONS.** El código actual es técnicamente sólido. La procedencia está corroborada, no firmada.

## B. Procedencia

### Hechos comprobados (metadatos del sistema de archivos, hora −03:00)

| Hora | Evento |
|---|---|
| 2026-10-13 18:38 | Paso 73 crea los 11 archivos base. |
| 18:49 | Se escribe `STEP-74-AUDIT-REPORT.md`. |
| 19:08:30 | `boundary_test.go` se recrea. |
| 19:09:37 / 19:10:34 | `testhelpers_test.go` y `fakes_test.go` se recrean. |
| 19:11:59 | Nacen `sentinel_pty_posix_test.go` y `sentinel_pty_other_test.go`. |
| 19:12:15 | Modificación del helper PTY posix. |
| 19:12:49 | Última modificación de `testhelpers_test.go` y `fakes_test.go`. |
| 19:14:10 | Última modificación de `boundary_test.go`. |
| 2026-10-19 10:34 | Se escribe `STEP-75-REPORT.md` (BLOCKED). |

`ports.go`, `internal/app/`, los `doc.go` y el README no cambiaron desde las 18:38.

### Evidencia local adicional

Existe un registro de sesión de Codex (`~/.codex/sessions/2026/10/13/rollout-2026-10-13T18-34-34-01a13730-4b95-7d83-944c-edc454219ee1.jsonl`). Solo se leyeron mensajes del usuario, rutas de parches, marcas de tiempo y el mensaje final.

- A las 19:04:24 recibió el prompt del PASO 75, con el mismo texto de autorización.
- Entre 19:07:44 y 19:14:10 aplicó parches únicamente a los cinco archivos de `internal/gitmanager/` que hoy existen.
- Cada marca de tiempo coincide al segundo con el mtime o birth del archivo. El mtime de 19:12:15 corresponde a un `gofmt -w`.
- A las 19:15:33 cerró con un informe **SUCCESS** del Paso 75 que nunca se guardó en un archivo.
- El `STEP-75-REPORT.md` del 19 de octubre vino de otra ejecución, que no conocía ese cierre y declaró BLOCKED.

### Alcance autorizado

El prompt del Paso 75 permitía modificar `boundary_test.go`, `testhelpers_test.go` y `fakes_test.go`. También permitía crear archivos de prueba nuevos bajo `internal/gitmanager/` si eran indispensables. Los dos helpers PTY encajan en esa excepción: Git solo activa el pager si la salida es una terminal. No se tocaron `ports.go`, `internal/app`, `specs/`, `go.mod`, `go.sum` ni otras features.

### Limitaciones

- El registro de Codex es un log local editable, no una firma.
- No se reconstruyó el contenido de los parches contra los hashes actuales; solo coinciden rutas y marcas de tiempo.
- No existen copias de las versiones previas de los archivos.

### Conclusión

La procedencia es **probable y corroborada**: los cambios son el resultado del Paso 75 ejecutado en Codex. No es criptográficamente verificable. Una adopción humana expresa puede regularizar el estado sin falsear el origen.

## C. Integridad

### SHA-256 de los cinco archivos investigados

| Archivo | SHA-256 |
|---|---|
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/testhelpers_test.go` | `bc7eac0ae0ae07b68dd8ea31c6457a55131e6ea1751e9bb531353423b2ffa11a` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `1c5e873f64026b02732f07c42d3fa78c6cf5915d6818308159745a29f0297dad` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `fcbdeaee3e3ddb7b3c7051b2f306dbad28b2657ebbdcf6b4cee8b6540066ea0b` |

### Helpers PTY

- Tienen build tags complementarios y exhaustivos. Para todo lo que no sea darwin o linux, `_other` devuelve error y el subtest hace Skip.
- Compilan en 7 plataformas (linux/amd64, arm64, ppc64le, mips64; windows/amd64; freebsd/amd64; darwin/amd64).
- No usan dependencias externas ni modifican el entorno global. Todo vive en directorios temporales.
- Quince ejecuciones seguidas del pager pasaron sin procesos huérfanos.
- Los números de ioctl coinciden con los de `creack/pty`.

### Alcance

Ningún cambio fuera de alcance: sin Fase 2, sin mutaciones Git, sin dependencias nuevas, `internal/app` inerte, documentos normativos intactos.

## D. Hallazgos

### Estado técnico de los hallazgos previos

| Hallazgo | Estado | Nota |
|---|---|---|
| F-01 | Resuelto técnicamente | Con reservas N-03 y N-04. |
| F-02 | Resuelto técnicamente | Los siete mecanismos escriben witness y marcador. |
| F-05 | Resuelto | `io.ReadFull` no entra en bucle; agotamiento con `ErrUnexpectedEOF`; inyección de error con resultado parcial. |
| F-06 | Resuelto | Mutex, resultado parcial más error, panic explícito al agotarse el clock y copias defensivas. |

### Hallazgos nuevos (ninguno bloqueante)

| ID | Sev. | Hallazgo |
|---|---|---|
| N-01 | LOW | `sentinel_pty_posix_test.go:38,52,56` convierte punteros a `uintptr` y los pasa por la función intermedia `pseudoTerminalIOCTL`. Incumple la regla de `unsafe`, que exige la conversión dentro de la llamada a `syscall.Syscall`. Riesgo teórico; `go vet` no lo detecta. |
| N-02 | LOW | `triggerPager` (`testhelpers_test.go:290-292`) espera con `Wait()` y `<-readDone` sin timeout propio. Si quedara un proceso con el esclavo abierto, bloquearía hasta el timeout global de `go test`. |
| N-03 | LOW | Quitar `checkIndirectManagerDependencies` no hace fallar ninguna de M1–M11: M3 queda cubierta también por la lista blanca directa. La comprobación indirecta no tiene prueba propia. |
| N-04 | LOW | Brechas del detector probadas en copia temporal: `syscall.Syscall(SYS_EXECVE…)` y el import de `plugin` no se detectan. Los adaptadores (`gitprocess`, `gitstore`) pueden importar `report` o `gatestore`, o importarse entre sí; el plan no lo prohíbe de forma explícita. |
| N-05 | INFO | Si el PTY no está disponible, el pager hace Skip silencioso y la prueba global sigue en PASS. Aquí 0 Skips. |
| N-06 | INFO | `credential.helper` se genera (8º script) pero ninguna prueba lo dispara. El requisito era de siete mecanismos. |
| N-07 | INFO | Si los ioctl de linux/ppc o mips difieren, el helper fallaría, haría Skip y pasaría igual. No verificado. |
| N-08 | INFO | `testhelpers_test.go` no tiene build tag; en Windows compila pero los scripts `.sh` no funcionarían. |

### Riesgos residuales

Los anteriores, más F-03, F-04 y F-07–F-10, que siguen abiertos por diseño.

## E. Pruebas

Todo con `GOFLAGS=-mod=readonly`, `GOCACHE` y `TMPDIR` fuera del repositorio.

| Verificación | Resultado |
|---|---|
| `go build ./...` | OK |
| `go vet ./...` | OK |
| `gofmt -l internal/` | Sin salida |
| `go test ./internal/gitmanager -v` | PASS, 0 Skips |
| `go test -race` (5 paquetes de Feature 008) | PASS |
| `go test -race -count=5` (fakes) | PASS |
| `go test -count=1 ./...` | Todos `ok` |
| Compilación cruzada de pruebas | 7 plataformas OK |

### Matriz M1–M11

Las 11 violaciones son detectadas por el detector real (que corre sobre árboles sintéticos) y el árbol base no da falsos positivos. Sensibilidad comprobada saboteando el detector en una copia aislada:

| Sabotaje | Resultado |
|---|---|
| Quitar `StartProcess` | Cae M8 |
| Quitar `syscall.Exec` | Cae M9 |
| Ignorar `_test.go` | Cae M11 |
| Aprobar todo `os/exec` | Caen M2, M6, M10 y M11 |
| Quitar lista blanca y comprobación indirecta | Caen M4, M5 y M7 |

### Matriz de las siete sentinelas

`editor`, `pager`, `askpass`, `ssh`, `gpg`, `fsmonitor` y `filter` ejecutaron el helper real y dejaron witness y marcador (Git real, repositorios temporales, HOME/XDG aislados, sin red). Una autoprueba rechaza el caso de ejecución sin marcador, y dos ejecuciones dejan dos registros append-only.

## F. Recomendación

**APTA CON CONDICIONES.** Es una evaluación técnica; no equivale a autorización humana.

### Condiciones para regularizar la línea base

1. Decisión humana expresa de adopción, indicando que el origen es el Paso 75 ejecutado en Codex (19:04–19:15), corroborado por el log de sesión y no criptográficamente verificable.
2. Guardar el informe SUCCESS que Codex emitió pero no persistió, o declarar `STEP-75-REPORT.md` (BLOCKED) como supersedido.
3. Aceptar o corregir N-01 y N-02 antes de que la Fase 2 dependa de los helpers.
4. Registrar N-03 y N-04 como deuda conocida; decidir si los adaptadores necesitan su propia lista de imports permitidos.
5. Mantener F-03, F-04 y F-07–F-10 para la autorización de la Fase 2.

### Decisiones humanas pendientes

- Adoptar o no la nueva línea base.
- Qué hacer con el informe del Paso 75.
- Si N-01 y N-02 se corrigen antes de la Fase 2.

## G. Estado final

- **Rama:** `feature/008-git-management`.
- **HEAD:** `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- **Git:** 27 archivos sin seguimiento al cierre de la auditoría (26 del preflight más `STEP-75-REPORT.md`); cero cambios rastreados; índice y diff vacíos. Con este informe, 28.
- **Hashes normativos:** `tasks.md` `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` y `spec.md` `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304`, sin cambios.
- **Confirmación:** el manifiesto SHA-256 de todos los archivos antes y después de la auditoría es idéntico. No se hicieron operaciones Git mutantes ni se inició la Fase 2. Las copias y sabotajes estuvieron en el scratchpad, fuera del repositorio. No se tocó `~/.codex`.

---

Este informe no constituye adopción de la línea base ni autorización para iniciar la Fase 2, modificar `tasks.md` o crear commits.
