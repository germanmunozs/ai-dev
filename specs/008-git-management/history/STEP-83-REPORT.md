# Informe del Paso 83 — Corrección focalizada de N-18 y N-19

Fecha: 2026-10-22  
Feature: 008 — Git Management  
Fase: 1

## A. Estado general

**SUCCESS**

N-18 y N-19 quedaron corregidos dentro del alcance autorizado. Se preservaron N-09, N-10, N-13 y N-14. No se inició ni se cerró automáticamente ninguna fase.

## B. Preflight

- Repositorio: `/Users/german/Developer/ai-dev`.
- Rama inicial: `feature/008-git-management`.
- HEAD inicial: `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- Índice Git: intacto.
- Diff rastreado inicial: vacío.
- Archivos sin seguimiento: 34 conocidos, todos preservados.
- Informes históricos: presentes e intactos.
- Documentos normativos: sin cambios.
- Código productivo: sin cambios.
- Fases 2–12: sin implementación anticipada.

Todos los hashes iniciales coincidieron con los valores autorizados:

| Archivo | SHA-256 inicial |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `ba2e0df9c584ee1178cffbefe0598367b37ecf44a32bb98977bc0859dedf234c` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `a69bc64e99a336969d57937e66bf714a28b6084fe0acbc949e73000dbb0a32f6` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `bbb12ccb4d6642dfef01d9510c93ef6f32fc5f07ce90b7c1d00fdc5994179383` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

## C. Archivos modificados

- `internal/gitmanager/testhelpers_test.go`: pruebas de captura inmediata, resultados inyectados de `Getpgid` y seam seguro de terminación.
- `internal/gitmanager/sentinel_pty_posix_test.go`: fallback restringido de PGID e inyección de la operación de señalización.
- `internal/gitmanager/sentinel_pty_other_test.go`: ajuste indispensable para conservar las firmas y la compatibilidad multiplataforma.

No se crearon ni eliminaron archivos durante la corrección. Este informe fue creado posteriormente por autorización humana expresa.

## D. N-18 — Captura segura y determinista del PGID

### Causa raíz

En macOS, `Getpgid` puede devolver `ESRCH` para un líder que terminó inmediatamente pero todavía no fue recolectado. La captura realizada justo después de `Start` podía por ello producir un falso error.

### Solución

`Getpgid` permanece como verificación normal. El fallback `PGID = PID` solo se permite cuando se cumplen simultáneamente estas condiciones:

- El proceso no es nil y su PID es mayor que 1.
- Se conserva el mismo `SysProcAttr` utilizado por `Start`.
- `Setpgid == true`.
- `Pgid == 0`.
- El error de la consulta es exactamente `ESRCH`.
- Todavía no se ha llamado a `Wait`, `wait4` ni `Release`.

La implementación del toolchain Go confirma, tanto para BSD/macOS como para Linux, que `Pgid == 0` asigna al hijo su propio PID como PGID. El hijo ejecuta `setpgid(0,0)` antes de `exec`; si esta preparación falla, el error se propaga y `Start` no tiene éxito. Por tanto, un `Start` exitoso bajo esos atributos demuestra que inicialmente `PGID == PID`.

### Errores no ocultados

- `EPERM` y cualquier error distinto de `ESRCH` se propagan.
- Un PGID observado distinto del PID se rechaza.
- El fallback se rechaza si falta `Setpgid` o si `Pgid` no es cero.
- La validación de atributos ocurre antes de consultar el PGID.

### Evidencia

- Captura normal: PASS.
- Fallback `ESRCH` determinista mediante inyección: PASS.
- Error inesperado: propagado correctamente.
- PGID discordante: rechazado.
- Ausencia de atributos aislados: rechazada.
- 50 procesos sintéticos de salida inmediata: PASS.
- Repetición controlada de 10 rondas: 500 procesos inmediatos, PASS.

Las pruebas de salida inmediata lanzan exclusivamente el propio binario de test y no envían señales.

## E. N-19 — Prueba segura por construcción

Se separó la validación de la operación real mediante un seam que recibe una función de terminación inyectada:

- `groupID <= 1` se rechaza antes de invocar el callback.
- La función real sigue ejecutando `syscall.Kill(-groupID, SIGKILL)` solamente después de validar.
- Las pruebas de `-1`, `0` y `1` utilizan un callback inerte, nunca `kill(2)`.
- El callback recibe cero invocaciones para los tres identificadores peligrosos.
- Un PGID válido de prueba llega correctamente al callback.
- Las rutas positivas reales siguen cubiertas por los fixtures PTY aislados.

### Prueba negativa

Si se eliminara accidentalmente el guard, el callback inerte registraría una invocación y la prueba fallaría. Incluso bajo esa regresión simulada:

- No se enviaría ninguna señal real.
- PID 1 no sería afectado.
- El grupo del proceso de test no sería afectado.
- Ningún proceso ajeno sería afectado.

La prueba no depende de privilegios ni del comportamiento de `kill(2)` del sistema operativo.

## F. Preservación de N-09, N-10, N-13 y N-14

### N-09

- No se introdujo ninguna goroutine lectora.
- Se conserva `select(2)` antes de `read(2)`.
- Se mantiene `wait4(WNOHANG)`.
- Se preservan los plazos de 10 segundos y 2 segundos.
- No existen esperas indefinidas nuevas.

### N-10

- El PGID se captura antes de cualquier recolección.
- Mientras el PTY permanece abierto, el líder no se recolecta.
- La señal de grupo ocurre antes de `wait4/Release`.
- No existen señales posteriores al reap.
- El fallback `ESRCH` solo opera mientras el PID permanece sin recolectar y reservado.

### N-13

- PID y PGID menores o iguales a 1 siguen rechazados.
- No existe ruta a `kill(1)`, `kill(0)` o `kill(-1)`.
- Los errores relevantes siguen propagándose.

### N-14

- Límite natural independiente de 5 segundos.
- Verificación de limpieza de 2 segundos.
- Marcador de salida natural preservado.
- Prueba negativa de terminación desactivada preservada.
- Cero procesos sintéticos residuales al finalizar.

## G. Pruebas y calidad

Todas las verificaciones usaron `GOFLAGS=-mod=readonly`, `GOCACHE` y `TMPDIR` fuera del repositorio.

| Verificación | Resultado |
|---|---|
| `gofmt` sobre los tres archivos | PASS |
| Pruebas focalizadas N-18/N-19 | PASS |
| 500 procesos de salida inmediata | PASS |
| Pruebas focalizadas de pager y PTY | PASS |
| Repetición del pager `-count=3` | PASS |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -count=1 ./internal/gitmanager` | PASS |
| `go test -race -count=1 ./internal/gitmanager` | PASS, 59,965 s |
| `go test -count=1 ./...` | PASS |
| Procesos sintéticos residuales | 0 |

Compilación cruzada de tests:

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

## H. Regresiones de Fase 1

- N-01: conversión segura de `unsafe.Pointer` preservada.
- N-02: timeout y limpieza preservados.
- F-01: M1–M11 detectadas, 11/11.
- F-02: editor, pager, askpass, SSH, GPG, fsmonitor y filter pasan con witness y marcador.
- F-05: lectura normal, agotamiento, error parcial e `io.ReadFull` pasan.
- F-06: resultados parciales, concurrencia, clock y copias defensivas pasan, incluido `-race`.
- T001–T006: sin regresiones.
- `internal/app`: composición inerte confirmada por la suite completa.

## I. Integridad final

### Hashes finales

| Archivo | SHA-256 final |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `d5454856f637c3c56a6b1c9ca593f4998d7c41d5a3da1cca5401609cae491602` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `fabccb5ee3b3c122fab3a34c787a8b1653915f729ca296c96f1acce54e12b170` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `d04d457d2057ae20cf2749984c69df6b9f39bae1b357fa4abaad351bfbc8c9ee` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

### Estado final

- Rama: `feature/008-git-management`.
- HEAD: `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- Índice y diff rastreado: vacíos.
- Los 34 archivos sin seguimiento conocidos permanecieron preservados durante la corrección; este informe es la única adición posterior autorizada.
- Documentos normativos intactos.
- Código productivo intacto.
- `go.mod` y `go.sum` intactos.
- Sin dependencias nuevas.
- Sin operaciones Git mutantes sobre `ai-dev`.
- Las operaciones Git de prueba se limitaron a repositorios temporales aislados.
- Sin commits, ramas, merges, rebases, tags o pushes.
- No se ejecutó `/speckit.implement`.
- No se inició ni se cerró ninguna fase.

## J. Hallazgos y decisiones pendientes

N-15, N-16, N-17 y N-20–N-22 permanecen fuera de alcance, junto con F-03, F-04 y F-07–F-10. Este paso no cierra automáticamente la Fase 1 ni autoriza el inicio de la Fase 2.

No continuar automáticamente con otro paso.
