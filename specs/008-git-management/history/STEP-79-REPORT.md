# Informe del Paso 79 — Corrección focalizada de N-09 y verificación read-only de N-10

## A. Estado general

**SUCCESS**

N-09 quedó corregido y N-10 fue verificado exclusivamente mediante inspección read-only. No se inició la Fase 2.

## B. Preflight

- Repositorio: `/Users/german/Developer/ai-dev`.
- Rama inicial/final: `feature/008-git-management`.
- HEAD inicial/final: `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- Índice y diff rastreado: vacíos.
- Archivos sin seguimiento conocidos: preservados.
- Informes históricos: preservados.
- Fases 2–12: sin implementación anticipada.

Todos los hashes iniciales coincidieron con los valores autorizados:

| Archivo | SHA-256 inicial |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `526778f4a665abd3d61de7d4560e40b153ea1b7a73e7ae85d708569a1cd6fff3` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `53d7c0563f3df7323c84bbc4a5e29937a8ae002dd83b7bc90fcc05505c5c1274` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `37627b93e9757b0b712ccc7f9b2e8135e1180b0e482e19370b0d19409440e74a` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

## C. Archivos modificados

- `internal/gitmanager/testhelpers_test.go`: control no bloqueante, plazos y pruebas de N-09.
- `internal/gitmanager/sentinel_pty_posix_test.go`: primitivas POSIX indispensables para lectura, sondeo, `wait4` y `setsid`.
- `internal/gitmanager/sentinel_pty_other_test.go`: stubs necesarios para conservar la compilación multiplataforma.

No se crearon ni eliminaron archivos durante la corrección.

`STEP-79-REPORT.md` fue creado posteriormente por autorización humana expresa para conservar este informe.

## D. N-09 — Corrección

### Causa raíz

La rama de timeout esperaba indefinidamente `waitDone` y `readDone`. Además, ni `master.Close()` ni `O_NONBLOCK` por sí solos garantizaban interrumpir una lectura ya iniciada mediante `os.File.Read` en macOS.

### Diseño aplicado

- Se eliminó completamente la goroutine lectora.
- Se eliminaron los canales `waitDone` y `readDone`.
- El master PTY se configura como no bloqueante.
- La legibilidad se consulta primero con `select(2)` de plazo cero.
- Solo se llama a `read(2)` cuando el kernel declara el descriptor listo.
- Se realiza como máximo una lectura por iteración.
- El proceso se consulta y recolecta mediante `wait4(WNOHANG)`, sin goroutine de espera.
- El bucle usa sondeo controlado de 5 ms, no sleeps arbitrarios.

### Límites temporales

- Timeout normal: 10 segundos.
- Segundo plazo de limpieza: 2 segundos.
- Si la recolección no termina dentro del segundo plazo, se devuelve un error explícito con `processReaped=false`.
- El cierre del master detiene el lector síncrono; no queda una goroutine bloqueada.

### Escenarios extremos

Pasaron:

- Pager normal.
- Dos ejecuciones consecutivas.
- Pager bloqueado.
- Descendiente que mantiene el PTY dentro del grupo original.
- Descendiente separado mediante `setsid`.
- Fallo simulado de terminación del grupo.
- Marcador y witness.
- Recolección del líder.
- Ausencia final de helpers sintéticos residuales.

Los helpers problemáticos usan el propio binario de test, duran como máximo 1,5 segundos y no ejecutan loops infinitos ni consumen CPU persistentemente.

Limitación inevitable: un proceso en estado kernel no interrumpible podría no ser recolectable. Esa ruta ya no bloquea el helper indefinidamente; vence el segundo plazo y retorna un error explícito.

## E. N-10 — Verificación read-only

N-10 queda **CONFIRMADO COMO RIESGO TEÓRICO LOW**, no reproducido y no corregido.

Existen dos rutas que pueden ejecutar `kill(-pid)` después de reapear al líder:

1. Finalización normal, después de `wait4(WNOHANG)`, en `testhelpers_test.go`.
2. Timeout donde el líder ya terminó, pero el PTY sigue abierto, también en `testhelpers_test.go`.

Para afectar un grupo ajeno tendrían que coincidir:

- El líder original ya fue recolectado.
- El grupo original ya no conserva miembros.
- El PID queda disponible.
- Otro proceso recibe exactamente ese PID.
- Ese proceso crea o lidera un grupo con el mismo identificador.
- `kill(-pid)` ocurre dentro de esa ventana.

Mientras existan miembros del grupo original, el PGID continúa reservado, reduciendo significativamente la probabilidad. No se ejecutaron pruebas de reutilización ni señales contra procesos potencialmente ajenos.

Alternativas no implementadas:

- Señalizar antes de recolectar al líder.
- Eliminar la limpieza preventiva posterior al reap.
- Supervisión dedicada de descendientes.
- Uso de primitivas específicas de plataforma para identidad estable.
- Aislamiento mediante cgroup o subreaper donde esté disponible.

La sustitución de `Wait` por `wait4(WNOHANG)` fue indispensable para N-09, pero conserva deliberadamente la señalización posterior al reap. No se corrigió N-10.

## F. Pruebas

Las verificaciones usaron `GOFLAGS=-mod=readonly` y `GOCACHE`/`TMPDIR` fuera del repositorio.

| Verificación | Resultado |
|---|---|
| `gofmt -l` | Sin salida |
| Pruebas focalizadas finales | PASS |
| Repetición focalizada `-count=3` | PASS |
| `go test -count=1 ./internal/gitmanager` | PASS |
| `go test -race -count=1 ./internal/gitmanager` | PASS |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -count=1 ./...` | PASS |
| Linux amd64 | Compila |
| Linux arm64 | Compila |
| Linux ppc64le | Compila |
| Linux mips64 | Compila |
| macOS amd64 | Compila |
| FreeBSD amd64 | Compila |
| Windows amd64 | Compila |
| Comprobación final de procesos residuales | Ninguno |

Las primeras ejecuciones contenidas detectaron dos problemas y no se ocultaron:

- `os.File.Read` todavía podía bloquear en macOS.
- `SYS_SELECT` no estaba definido en dos arquitecturas Linux.

Ambos se corrigieron antes de la matriz final. El timeout externo de prueba actuó correctamente y los helpers autolimitados no dejaron procesos residuales.

Todas las operaciones Git de prueba se ejecutaron exclusivamente en repositorios temporales aislados, con HOME/XDG temporales y sin utilizar credenciales, remotos ni configuración global del usuario.

## G. Regresiones

- N-01: la conversión de punteros a `uintptr` permanece dentro de `syscall.Syscall`; `runtime.KeepAlive` sigue presente.
- N-02: timeout, terminación y limpieza permanecen operativos.
- F-01: M1–M11 detectadas 11/11.
- F-02: las siete sentinelas pasan.
- F-05: RandomSource pasa.
- F-06: fakes concurrentes pasan con `-race`.
- T001–T006: sin regresiones.
- `internal/app`: composición inerte confirmada por la suite.

## H. Integridad

### Hashes finales

| Archivo | SHA-256 final |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `8abd6ac0d15c178f5cfcf2123e84802a42e97d54fe2d21578f8df23660c17675` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `a078c64dae9db43ca5237cee4ae4d0e6d0afa4516174c7e467b7d6d04f75e266` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `6050be14b5d15fe98ad3a836ca9de288e2b3d551e05cc1710ba0054c6c4fdf10` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

### Estado final

- Rama y HEAD sin cambios.
- Índice y diff rastreado vacíos.
- 30 archivos sin seguimiento antes de crear este informe; el informe es la única adición autorizada posterior.
- `go.mod` sin cambios rastreados; `go.sum` no existe.
- Sin cambios normativos, dependencias ni código productivo.
- Sin commits, ramas, merges, rebases, tags o pushes.
- No se ejecutó `/speckit.implement`.
- No se inició la Fase 2.

## I. Pendientes y decisiones humanas

Permanecen fuera de alcance:

- N-10, confirmado como riesgo teórico LOW.
- N-03, N-04, N-05–N-08 y N-11–N-12.
- F-03, F-04 y F-07–F-10.
- Cualquier cierre formal de la Fase 1.
- Toda autorización para iniciar la Fase 2.

No continuar automáticamente con otro paso.
