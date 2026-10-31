# Informe del Paso 81 — Corrección de N-13, N-14 y N-10

Fecha: 2026-10-21  
Feature: 008 — Git Management  
Fase: 1

## A. Estado general

**SUCCESS**

N-13, N-14 y N-10 quedaron corregidos dentro del alcance autorizado. No se inició la Fase 2.

## B. Preflight

- Repositorio: `/Users/german/Developer/ai-dev`.
- Rama inicial: `feature/008-git-management`.
- HEAD inicial: `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- Índice Git: intacto.
- Diff rastreado inicial: vacío.
- Archivos sin seguimiento: 31, todos preservados.
- Informes históricos: preservados.
- Fases 2–12: sin implementación anticipada.

Todos los hashes iniciales coincidieron con los valores autorizados:

| Archivo | SHA-256 inicial |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `8abd6ac0d15c178f5cfcf2123e84802a42e97d54fe2d21578f8df23660c17675` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `a078c64dae9db43ca5237cee4ae4d0e6d0afa4516174c7e467b7d6d04f75e266` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `6050be14b5d15fe98ad3a836ca9de288e2b3d551e05cc1710ba0054c6c4fdf10` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

## C. Archivos modificados

- `internal/gitmanager/testhelpers_test.go`: secuencia segura de señalización y recolección, identidad inmutable del grupo y pruebas no vacuas.
- `internal/gitmanager/sentinel_pty_posix_test.go`: obtención y validación segura del PGID y rechazo de identificadores peligrosos.
- `internal/gitmanager/sentinel_pty_other_test.go`: cambio indispensable para mantener la misma validación y firma en la compilación multiplataforma.

No se crearon ni eliminaron archivos durante la corrección. Este informe fue creado posteriormente por autorización humana expresa.

## D. N-13 — Seguridad de señalización

### Causa raíz

`pollPseudoTerminalProcess` ejecutaba `wait4(WNOHANG)` y luego `Process.Release()`. `Release()` cambiaba `Process.Pid` a `-1`, pero dos rutas posteriores reutilizaban ese campo para formar `kill(-pid, SIGKILL)`. Con `pid == -1`, el destino efectivo era PID 1.

### Corrección

- El PGID se captura inmediatamente después de `Start` y antes de cualquier recolección o liberación.
- En POSIX se valida que el proceso no sea nil, que PID y PGID sean mayores que 1 y que `PGID == PID`.
- `terminatePseudoTerminalProcessGroup` rechaza todo identificador menor o igual a 1 antes de invocar `kill(2)`.
- Ninguna ruta usa `command.Process.Pid` después de `Release()`.
- Los errores relevantes de terminación se incorporan al resultado.

### Evidencia

- La prueba `TestProcessGroupTerminationRejectsUnsafeIdentifiers` confirma el rechazo de `-1`, `0` y `1`.
- La inspección estructural final solo encontró `Process.Pid` dentro de la captura y validación inicial del grupo.
- No existe una ruta que pueda construir `kill(1)`, `kill(0)` o `kill(-1)`.
- La terminación real de descendientes del grupo pasa en los escenarios focalizados.

## E. N-14 — Pruebas de limpieza no vacuas

El helper sintético ahora dispone de:

- Un límite natural independiente de 5 segundos.
- Un plazo de verificación de limpieza de 2 segundos.
- Un marcador escrito únicamente al alcanzar el límite natural.
- Una comprobación de que el proceso desaparece antes de dicho límite y sin generar el marcador natural.

La prueba negativa `TestPagerCleanupVerificationRejectsDisabledGroupTermination` sustituye la terminación del grupo por un no-op. El verificador observa que el descendiente sigue vivo después de 250 ms y rechaza la limpieza. El proceso se elimina inmediatamente después mediante su PID sintético conocido.

Además:

- El escenario con descendiente en el grupo verifica una terminación anterior a la autolimpieza.
- El escenario `setsid` verifica que el descendiente separado sobrevive al grupo original y después lo elimina explícitamente.
- La simulación de error comprueba que el PGID entregado a la función de terminación coincide con el PGID capturado y es mayor que 1.
- La desactivación deliberada de la terminación haría fallar las aserciones positivas.

## F. N-10 — Reutilización de PID/PGID

### Secuencia final

1. `Start` crea al líder con `Setpgid`.
2. Se captura y valida inmediatamente el PGID.
3. Mientras el lector PTY siga abierto, el líder no se recolecta.
4. En timeout, el grupo se señala antes de `wait4` y `Release`.
5. Después se termina, si corresponde, y se recolecta al líder.
6. En finalización normal, primero termina el lector, luego se recolecta al líder y no se envían señales posteriores.

Las llamadas a señalización posteriores al reap fueron eliminadas de `triggerPager`. Mientras el líder permanece vivo o zombie sin recolectar, su PID no puede reutilizarse; de ese modo, la identidad que mantiene reservado el grupo continúa siendo válida durante la señalización.

### Riesgos residuales

Un proceso en estado kernel no interrumpible podría exceder el plazo de limpieza. Esa ruta permanece acotada, retorna un error explícito y no intenta señalar usando una identidad liberada. Los descendientes que ejecutan `setsid` ya no pertenecen al grupo original y requieren la contención independiente prevista por el fixture.

## G. Pruebas y calidad

Todas las verificaciones usaron `GOFLAGS=-mod=readonly`, `GOCACHE` y `TMPDIR` fuera del repositorio.

| Verificación | Resultado |
|---|---|
| `gofmt` sobre los tres archivos | PASS, sin archivos pendientes |
| Pruebas focalizadas N-13/N-14/N-10 | PASS |
| Repetición focalizada `-count=3` | PASS |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -count=1 ./internal/gitmanager` | PASS |
| `go test -race -count=1 ./internal/gitmanager` | PASS |
| `go test -count=1 ./...` | PASS |
| Inspección final de helpers sintéticos | 0 procesos residuales |

Compilación cruzada de los tests:

| Plataforma | Resultado |
|---|---|
| linux/amd64 | PASS |
| linux/arm64 | PASS |
| linux/ppc64le | PASS |
| linux/mips64 | PASS |
| darwin/amd64 | PASS |
| freebsd/amd64 | PASS |
| windows/amd64 | PASS |

La compilación cruzada no constituye ejecución nativa. Los tests de procesos y PTY se ejecutaron en macOS/arm64.

## H. Regresiones

- N-01: la conversión de `unsafe.Pointer` a `uintptr` permanece directamente en `syscall.Syscall`, seguida de `runtime.KeepAlive`.
- N-02: timeout, terminación y limpieza permanecen operativos.
- N-09: no se reintrodujeron esperas indefinidas ni goroutines lectoras.
- F-01: M1–M11 detectadas, 11/11.
- F-02: editor, pager, askpass, SSH, GPG, fsmonitor y filter pasan con witness y marcador.
- F-05: lectura normal, agotamiento, error inyectado e `io.ReadFull` pasan.
- F-06: resultados parciales, concurrencia, clock y copias defensivas pasan, incluido `-race`.
- T001–T006: sin regresiones.
- `internal/app`: composición inerte confirmada por la suite completa.

## I. Integridad final

### Hashes finales

| Archivo | SHA-256 final |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `ba2e0df9c584ee1178cffbefe0598367b37ecf44a32bb98977bc0859dedf234c` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `a69bc64e99a336969d57937e66bf714a28b6084fe0acbc949e73000dbb0a32f6` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `bbb12ccb4d6642dfef01d9510c93ef6f32fc5f07ce90b7c1d00fdc5994179383` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

### Estado final

- Rama: `feature/008-git-management`.
- HEAD: `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- Índice y diff rastreado: vacíos.
- Los 31 archivos sin seguimiento conocidos permanecieron preservados durante la corrección; este informe es la única adición posterior autorizada.
- Documentos normativos intactos.
- Código productivo intacto.
- `go.mod` y `go.sum` intactos.
- Sin dependencias nuevas.
- Sin commits, ramas, merges, rebases, tags o pushes.
- Las operaciones Git de prueba se limitaron a repositorios temporales aislados.
- No se ejecutó `/speckit.implement`.
- No se inició la Fase 2.

## J. Hallazgos y decisiones pendientes

Permanecen fuera de alcance N-15, N-16, N-17 y la deuda técnica previamente documentada. Este paso no cierra automáticamente la Fase 1 ni autoriza el inicio de la Fase 2.

No continuar automáticamente con otro paso.
