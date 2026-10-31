# Informe PASO 85 — Corrección focalizada de N-23, N-25 y N-26

## A. Estado general: SUCCESS

N-23, N-25 y N-26 fueron corregidos dentro del alcance autorizado. No se inició la Fase 2 ni se cerró formalmente la Fase 1.

## B. Preflight

- Repositorio: `/Users/german/Developer/ai-dev`
- Rama: `feature/008-git-management`
- HEAD: `a61ae2ffbb591a6282636805a2c35c81246b2d5c`
- UID: `501`; pruebas no ejecutadas como root.
- Índice Git: vacío.
- Diff rastreado: vacío.
- Archivos sin seguimiento iniciales: 36, preservados.
- Informes históricos: preservados.
- `go.sum`: no existía y no fue creado.
- Todos los hashes iniciales coincidieron con los esperados.

### Hashes iniciales verificados

| Archivo | SHA-256 inicial |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `d5454856f637c3c56a6b1c9ca593f4998d7c41d5a3da1cca5401609cae491602` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `fabccb5ee3b3c122fab3a34c787a8b1653915f729ca296c96f1acce54e12b170` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `d04d457d2057ae20cf2749984c69df6b9f39bae1b357fa4abaad351bfbc8c9ee` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

## C. Archivos modificados

- `internal/gitmanager/testhelpers_test.go`
- `internal/gitmanager/sentinel_pty_posix_test.go`
- `internal/gitmanager/sentinel_pty_other_test.go`, únicamente para compilación multiplataforma.

Durante la corrección no se crearon ni eliminaron archivos dentro del repositorio. Este informe es el único archivo adicional creado posteriormente por autorización humana expresa.

## D. N-23 — Fallback con PID recolectado

### Causa raíz

Ante `Getpgid(...)=ESRCH`, el helper confiaba únicamente en `Setpgid=true`, `Pgid=0` y reutilizaba `Process.Pid`, aunque `Wait()` ya hubiese recolectado al proceso.

### Solución elegida

- Se añadió un probe inyectable del ciclo de vida.
- La implementación real usa `os.Process.Signal(0)`, no `syscall.Kill(pid, 0)`.
- `os.Process` conserva su estado de ciclo de vida y devuelve `os.ErrProcessDone` después de `Wait`, sin señalizar un PID potencialmente reutilizado.
- `Release()` continúa siendo rechazado mediante `Pid == -1`.
- El fallback `PID == PGID` solo se permite cuando:
  - PID es mayor que 1.
  - `Setpgid=true`.
  - `Pgid=0`.
  - El proceso no fue recolectado ni liberado.
  - `Getpgid` devuelve exactamente `ESRCH`.

### Justificación de seguridad

Una consulta directa `kill(pid, 0)` distingue procesos existentes de inexistentes, pero no acredita identidad histórica si el PID fue reutilizado. En cambio, `os.Process.Signal(0)` consulta primero el estado de ciclo de vida del mismo objeto `os.Process` y se sincroniza con `Process.Wait`. Después de `Wait`, devuelve `os.ErrProcessDone` antes de emitir una señal al sistema operativo.

La secuencia controlada permanece como `Start → captura de identidad → supervisión`. No existe un reaper concurrente en esa ruta. Una recolección externa mediante `syscall.Wait4`, concurrente y ajena al helper, no forma parte del contrato soportado.

### Evidencia

- Proceso vivo no recolectado: PASS.
- Proceso terminado/zombie no recolectado: PASS.
- Proceso recolectado mediante `Wait`: rechazado con `os.ErrProcessDone`.
- Proceso liberado mediante `Release`: rechazado antes de ejecutar probes.
- `ESRCH` determinista: PASS.
- Error inesperado `EPERM`: propagado.
- PGID discordante: rechazado.
- Ausencia de `Setpgid`: rechazada.
- `Pgid != 0`: rechazado.
- No se enviaron señales peligrosas ni se actuó sobre grupos ajenos.

## E. N-25 — Cobertura directa de entradas inválidas

Se añadieron casos focalizados para:

- Proceso nil.
- PID `-2`.
- PID `-1`.
- PID `0`.
- PID `1`.
- Atributos nil.
- `Setpgid=false`.
- `Pgid=7`.
- Función de lookup nil.
- Probe de ciclo de vida nil.

Cada caso comprueba que las funciones inyectadas de identidad no sean invocadas.

### Prueba negativa

Se retiró deliberadamente el guard en una copia temporal aislada. La prueba falló inmediatamente en el caso de proceso nil. No se ejecutó ninguna señal real y no se afectaron PID 1, el grupo del proceso de test ni procesos externos.

La matriz también detectaría de forma segura la eliminación parcial del guard de PID: los callbacks inyectados serían observados o el fallback aceptaría la entrada, haciendo fallar la prueba sin utilizar la terminación real.

## F. N-26 — Prueba de salida inmediata

Se eliminó `TestPseudoTerminalProcessGroupCaptureImmediateExit`, que lanzaba 50 procesos sin observar `ESRCH`.

Fue reemplazada por:

- Cobertura determinista de `ESRCH` mediante inyección.
- Integración con un proceso vivo.
- Integración con un proceso terminado pero no recolectado.
- Casos explícitos posteriores a `Wait` y `Release`.

### Comparación de duración

| Medición | Antes | Paso 85 |
|---|---:|---:|
| Paquete con `-race` | aproximadamente 59,965–61 s | 12,40 s |
| Captura focalizada con `-race` | No aislada | 9,41 s |
| 10 repeticiones focalizadas | 500 procesos en el diseño anterior | 0,52 s |

El nombre y la intención de las nuevas pruebas corresponden a las rutas ejercitadas. La cobertura de `ESRCH` ya no depende de temporización ni de que el sistema operativo produzca intermitentemente esa respuesta.

## G. Preservación de correcciones previas

- N-09: PASS; lectura PTY supervisada y esperas acotadas.
- N-10: PASS; no se señaliza después de recolectar o liberar al líder.
- N-13: PASS; PID y PGID peligrosos continúan rechazados.
- N-14: PASS; la prueba negativa de limpieza sigue siendo no vacua.
- N-18: PASS; fallback `ESRCH` preservado para líderes no recolectados.
- N-19: PASS; seam de señalización seguro y sin estado global mutable.
- N-01: sin regresión en la conversión directa para la llamada al sistema.
- N-02: sin regresión en timeout y limpieza del pager.

## H. Pruebas y calidad

| Verificación | Resultado |
|---|---|
| `gofmt` | PASS; sin archivos pendientes |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -count=1 ./internal/gitmanager` | PASS; 7,57 s |
| `go test -race -count=1 ./internal/gitmanager` | PASS; 12,40 s reales, 11,81 s reportados por Go |
| `go test -count=1 ./...` | PASS; 8,27 s reales |
| Pruebas focalizadas N-23/N-25/N-26 | PASS |
| 10 repeticiones focalizadas | PASS; 0,52 s |
| Pager y PTY | PASS |
| Procesos residuales sintéticos | Ninguno detectado |

### Compilación cruzada

| Plataforma | Resultado |
|---|---|
| linux/amd64 | PASS |
| linux/arm64 | PASS |
| linux/ppc64le | PASS |
| linux/mips64 | PASS |
| darwin/amd64 | PASS |
| freebsd/amd64 | PASS |
| windows/amd64 | PASS |

La compilación cruzada no constituye ejecución nativa. Los tests POSIX y PTY se ejecutaron nativamente en macOS/arm64.

## I. Regresiones generales de Fase 1

- F-01: matriz M1–M11 detectada, 11/11.
- F-02: editor, pager, askpass, SSH, GPG, fsmonitor y clean filter, 7/7.
- F-05: lectura, reproducibilidad, agotamiento, error parcial e `io.ReadFull`, PASS.
- F-06: resultados parciales, concurrencia, clock y copias defensivas, PASS.
- T001–T006: sin regresiones observadas.
- `internal/app`: suite PASS; composición continúa inerte.

## J. Integridad final

### Hashes finales

| Archivo | SHA-256 final |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `13af569ae6a032c31a7d5fe69b35d456758a10a3457f956ec0404da7e6aba213` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `68e07ad0741b6131f3d242ac67748ef2aa782310bfcfab8ea31e5991d0e955c1` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `ca30e5029eb70cb737ecdbdb79426e5b09812bfae13d179052c567c19d09d4f0` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

### Estado final

- Rama: `feature/008-git-management`.
- HEAD: `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- Índice Git intacto.
- Diff de archivos rastreados vacío.
- Código productivo intacto.
- Documentos normativos intactos.
- Dependencias intactas.
- Sin cambios en `go.mod`; `go.sum` continúa ausente.
- Sin operaciones Git mutantes.
- Sin commits, ramas, merges, rebases, tags ni pushes.
- Sin implementación de Fase 2.
- Los artefactos temporales de compilación y mutación fueron eliminados fuera del repositorio.

## K. Hallazgos y decisiones pendientes

N-15, N-16, N-17 y N-20–N-22 continúan fuera del alcance de esta corrección. También permanecen pendientes F-03, F-04 y F-07–F-10.

No se cerró formalmente la Fase 1. No se inició la Fase 2. El trabajo queda detenido a la espera de nueva autorización humana.
