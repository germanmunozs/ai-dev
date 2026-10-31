# Informe del Paso 77 — Corrección focalizada N-01 y N-02

## A. Estado general

**SUCCESS**

N-01 y N-02 quedaron corregidos y verificados. No se inició la Fase 2 ni se modificaron documentos normativos, interfaces productivas o dependencias.

## B. Preflight

- Repositorio: `/Users/german/Developer/ai-dev`.
- Rama inicial/final: `feature/008-git-management`.
- HEAD inicial/final: `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- Índice Git: intacto.
- Cambios rastreados inesperados: ninguno.
- Informes históricos: conservados.
- Trabajo anticipado de Fases 2–12: no detectado.

Todos los hashes iniciales coincidieron:

| Archivo | SHA-256 inicial |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/testhelpers_test.go` | `bc7eac0ae0ae07b68dd8ea31c6457a55131e6ea1751e9bb531353423b2ffa11a` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `1c5e873f64026b02732f07c42d3fa78c6cf5915d6818308159745a29f0297dad` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `fcbdeaee3e3ddb7b3c7051b2f306dbad28b2657ebbdcf6b4cee8b6540066ea0b` |

## C. Cambios

### Archivos modificados

- `internal/gitmanager/sentinel_pty_posix_test.go` para N-01 y control de grupos de procesos de N-02.
- `internal/gitmanager/testhelpers_test.go` para timeout, cancelación, limpieza y pruebas de N-02.
- `internal/gitmanager/sentinel_pty_other_test.go` únicamente para mantener compatibilidad de compilación en las plataformas no POSIX cubiertas.

### Archivos creados o eliminados durante la corrección

Ninguno.

`STEP-77-REPORT.md` fue creado posteriormente por autorización humana expresa para conservar este informe.

## D. N-01 — Seguridad de memoria PTY

La interfaz de `pseudoTerminalIOCTL` ahora recibe `unsafe.Pointer`, evitando transportar direcciones como `uintptr` entre funciones.

La única conversión ocurre directamente en la expresión de llamada:

```go
syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(argument))
```

Inmediatamente después se ejecuta `runtime.KeepAlive(argument)`, garantizando que la referencia permanezca viva durante la llamada al sistema.

También se preservaron:

- Los build tags originales para macOS y Linux.
- Los IOCTL y el comportamiento PTY existentes.
- El cierre del master ante fallos.
- La ausencia de dependencias externas.

La prueba de apertura y cierre explícito del par PTY pasó.

## E. N-02 — Timeout y limpieza del pager

Se añadió:

- Timeout normal específico de 10 segundos, suficientemente amplio para CI.
- Timeout de 500 ms únicamente en escenarios sintéticos bloqueados.
- Grupo de procesos aislado mediante `Setpgid`.
- Terminación del grupo completo mediante `SIGKILL`.
- Recolección obligatoria del proceso Git mediante `Wait`.
- Cierre del slave PTY después de `Start`.
- Cierre del master PTY durante cancelación y finalización.
- Espera obligatoria de la goroutine lectora antes de retornar.
- Error identificable mediante `errors.Is(err, errPagerTimeout)`.
- Limpieza preventiva de descendientes residuales incluso tras una salida normal.

Pruebas añadidas:

- Pager normal con dos ejecuciones sucesivas.
- Pager que no termina.
- Descendiente que mantiene abierto el descriptor PTY.
- Marcador de ejecución correctamente registrado.
- Proceso directo recolectado.
- Descendientes eliminados y comprobados por PID.
- Lector finalizado sin fuga de goroutine.

Los tres escenarios se repitieron cinco veces sin fallos.

## F. Regresiones

- F-01: matriz M1–M11 detectada 11/11.
- Grafo permitido: aceptado sin falsos positivos.
- F-02: las siete sentinelas ejecutaron sus helpers y registraron witness y marcador:
  - editor;
  - pager;
  - askpass;
  - SSH;
  - GPG;
  - fsmonitor;
  - clean filter.
- F-05: lectura normal, agotamiento, error inyectado e `io.ReadFull` pasan.
- F-06: resultados parciales, sincronización, clock y copias defensivas pasan.

## G. Calidad

Todas las verificaciones usaron `GOFLAGS=-mod=readonly` y `GOCACHE=/tmp/ai-dev-step77-go-cache`.

| Verificación | Resultado |
|---|---|
| `gofmt -l` | Sin salida |
| Pruebas PTY/pager focalizadas | PASS |
| Cancelación, limpieza y repetición `-count=5` | PASS |
| `go test -count=1 ./internal/gitmanager` | PASS |
| `go test -race -count=1 ./internal/gitmanager` | PASS |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -count=1 ./...` | PASS |
| Linux amd64 | Compila |
| Linux arm64 | Compila |
| Linux ppc64le | Compila |
| Linux mips64 | Compila |
| Windows amd64 | Compila |
| FreeBSD amd64 | Compila |
| macOS amd64 | Compila |

Las operaciones Git de prueba se ejecutaron exclusivamente en repositorios temporales aislados, con HOME/XDG temporales y sin usar credenciales, remotos ni configuración global del usuario.

## H. Integridad final

### Hashes finales

| Archivo | SHA-256 final |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |
| `internal/gitmanager/testhelpers_test.go` | `526778f4a665abd3d61de7d4560e40b153ea1b7a73e7ae85d708569a1cd6fff3` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `37627b93e9757b0b712ccc7f9b2e8135e1180b0e482e19370b0d19409440e74a` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `53d7c0563f3df7323c84bbc4a5e29937a8ae002dd83b7bc90fcc05505c5c1274` |

### Estado final del repositorio

- Rama y HEAD sin cambios.
- Índice y diff de archivos rastreados vacíos.
- Permanecen los mismos 28 archivos sin seguimiento del preflight, más este informe autorizado.
- Sin commits, ramas, merges, rebases, pushes ni tags.
- Sin cambios fuera del alcance autorizado.
- No se ejecutó `/speckit.implement`.
- No se inició la Fase 2.

## I. Pendientes

Continúan abiertos deliberadamente:

- N-03 y N-04 como deuda técnica conocida.
- F-03 y F-04.
- F-07–F-10 y N-05–N-08 según el informe 76A.

Cualquier trabajo de Fase 2 requiere una nueva autorización humana.

No continuar automáticamente con otro paso.
