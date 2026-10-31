# Informe del Paso 75 — Corrección focalizada de Feature 008, Fase 1

## 1. Estado general

**BLOCKED**

El preflight encontró cambios posteriores al Paso 74 cuya procedencia no está documentada. No se modificaron archivos de implementación o pruebas durante este paso.

Evidencia temporal observada:

- `STEP-74-AUDIT-REPORT.md`: `2026-10-13T18:49:35-0300`.
- `internal/gitmanager/sentinel_pty_other_test.go`: `2026-10-13T19:11:59-0300`.
- `internal/gitmanager/sentinel_pty_posix_test.go`: `2026-10-13T19:12:15-0300`.
- `internal/gitmanager/testhelpers_test.go`: `2026-10-13T19:12:49-0300`.
- `internal/gitmanager/fakes_test.go`: `2026-10-13T19:12:49-0300`.
- `internal/gitmanager/boundary_test.go`: `2026-10-13T19:14:10-0300`.

El contenido actual contradice la línea base descrita por la auditoría: ya contiene marcadores con rutas absolutas incrustadas, un detector arquitectónico ampliado, la matriz M1–M11 y fakes corregidos. Además, existen dos helpers PTY no registrados en el estado final del Paso 74.

Por la instrucción de detenerse ante una discrepancia relevante, no corresponde adoptar, modificar ni declarar formalmente cerrados esos cambios sin confirmación humana.

## 2. Preflight

- Repositorio: `/Users/german/Developer/ai-dev` — correcto.
- Rama inicial/final: `feature/008-git-management` — correcta.
- HEAD inicial/final: `a61ae2ffbb591a6282636805a2c35c81246b2d5c` — correcto.
- Cambios rastreados: ninguno.
- Índice: sin cambios.
- T001–T006: confirmadas como alcance de Fase 1.
- `AGENTS.md`: ninguno aplicable.
- Documentos normativos: no fueron modificados.
- Resultado: discrepancia relevante de línea base; preflight bloqueado.

### Archivos sin seguimiento registrados en el preflight

```text
STEP-73-REPORT.md
STEP-74-AUDIT-REPORT.md
internal/app/gitmanagement.go
internal/app/gitmanagement_test.go
internal/conventionalcommit/doc.go
internal/gitmanager/boundary_test.go
internal/gitmanager/doc.go
internal/gitmanager/fakes_test.go
internal/gitmanager/ports.go
internal/gitmanager/sentinel_pty_other_test.go
internal/gitmanager/sentinel_pty_posix_test.go
internal/gitmanager/testdata/hostile-git/README.txt
internal/gitmanager/testhelpers_test.go
internal/gitprocess/doc.go
internal/gitstore/doc.go
specs/008-git-management/checklists/requirements.md
specs/008-git-management/contracts/apply-strategy.md
specs/008-git-management/contracts/authorization-audit.md
specs/008-git-management/contracts/cli.md
specs/008-git-management/contracts/inspection-plan.md
specs/008-git-management/data-model.md
specs/008-git-management/plan.md
specs/008-git-management/quickstart.md
specs/008-git-management/research.md
specs/008-git-management/spec.md
specs/008-git-management/tasks.md
```

## 3. Archivos modificados y creados

### Código, pruebas y documentos normativos

Ninguno fue modificado por el Paso 75.

### Informe

- `STEP-75-REPORT.md`, creado posteriormente por autorización humana expresa para conservar este informe.

Solo se ejecutaron comprobaciones de lectura y una prueba focalizada segura con:

```text
GOCACHE=/tmp/ai-dev-step75-go-cache
GOFLAGS=-mod=readonly
```

Los repositorios Git usados por las pruebas residieron en directorios temporales aislados.

## 4. Evidencia individual de F-01, F-02, F-05 y F-06

| Hallazgo | Evidencia observada en el contenido preexistente | Estado formal |
|---|---|---|
| F-01 | Detector ampliado, caso positivo y matriz M1–M11 pasan | No declarado CLOSED por discrepancia de procedencia |
| F-02 | Siete mecanismos reales escribieron marcador y witness | No declarado CLOSED por discrepancia de procedencia |
| F-05 | Lectura normal, agotamiento, error inyectado e `io.ReadFull` pasan | No declarado CLOSED por discrepancia de procedencia |
| F-06 | Resultado parcial más error, sincronización, agotamiento de clock y copias defensivas pasan | No declarado CLOSED por discrepancia de procedencia |

## 5. Matriz de los 11 casos de violación arquitectónica

La autoprueba preexistente detectó 11 de 11 casos:

| ID | Violación introducida | Resultado esperado | Resultado observado |
|---|---|---|---|
| M1 | `gitmanager` importa `gitprocess` | Detectar | Detectada |
| M2 | `gitmanager` importa `os/exec` | Detectar | Detectada |
| M3 | `gitmanager` alcanza `os/exec` mediante `internal/system` | Detectar | Detectada |
| M4 | `gitmanager` importa `internal/report` | Detectar | Detectada |
| M5 | `gitmanager` importa `internal/gatestore` | Detectar | Detectada |
| M6 | Un subpaquete de `gitmanager` importa `os/exec` | Detectar | Detectada |
| M7 | `conventionalcommit` importa `gitmanager` | Detectar | Detectada |
| M8 | `gitmanager` usa `os.StartProcess` | Detectar | Detectada |
| M9 | `gitmanager` usa `syscall.Exec` | Detectar | Detectada |
| M10 | `gitstore` importa `os/exec` | Detectar | Detectada |
| M11 | Un `_test.go` no autorizado de `gitmanager` importa `os/exec` | Detectar | Detectada |

La autoprueba del grafo permitido también pasó, sin falsos positivos observados.

## 6. Matriz de ejecución real de sentinelas

| Mecanismo | Helper ejecutado | Marcador registrado | Resultado Git |
|---|---:|---:|---|
| `GIT_EDITOR` | Sí | Sí | Éxito |
| `GIT_PAGER` | Sí | Sí | Éxito |
| `GIT_ASKPASS` | Sí | Sí | Éxito |
| `GIT_SSH_COMMAND` | Sí | Sí | Error 128 esperado del helper sintético |
| `gpg.program` | Sí | Sí | Error 128 esperado del helper sintético |
| `core.fsmonitor` | Sí | Sí | Éxito |
| `filter.*.clean` | Sí | Sí | Éxito |

También se observó que:

- Dos ejecuciones producen dos registros append-only.
- Una ejecución confirmada cuyo marcador no puede escribirse se distingue y rechaza explícitamente.
- `gpg.program` contiene únicamente una ruta ejecutable válida, sin argumentos incrustados.

## 7. Pruebas focalizadas

Comando:

```text
GOCACHE=/tmp/ai-dev-step75-go-cache GOFLAGS=-mod=readonly go test -count=1 -v ./internal/gitmanager
```

Resultado:

```text
PASS
ok  ai-dev/internal/gitmanager  5.823s
```

`gofmt -l` sobre los cinco archivos de prueba relevantes no produjo salida.

## 8. Resultado de `go test -race`

No ejecutado. El trabajo se detuvo al confirmar la discrepancia de preflight.

## 9. Resultado de `go vet`

No ejecutado. El trabajo se detuvo al confirmar la discrepancia de preflight.

## 10. Resultado de la suite completa

`go test -count=1 ./...` no fue ejecutado. El trabajo se detuvo al confirmar la discrepancia de preflight.

## 11. Hashes iniciales y finales

| Archivo | SHA-256 inicial | SHA-256 final |
|---|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |

No se modificó ningún documento bajo `specs/`.

## 12. Rama, HEAD y estado Git final

- Rama: `feature/008-git-management`.
- HEAD: `a61ae2ffbb591a6282636805a2c35c81246b2d5c`.
- Archivos rastreados modificados: ninguno.
- Índice: sin cambios.
- El estado sin seguimiento permanece como en el preflight, con la única adición autorizada posterior de `STEP-75-REPORT.md`.

## 13. Confirmación de alcance

- No se modificó `tasks.md`.
- No se modificaron contratos ni interfaces productivas.
- No se ejecutaron operaciones Git mutantes sobre `ai-dev`.
- No se crearon commits, ramas, merges, rebases, pushes ni tags.
- No se inició la Fase 2.
- No se corrigieron F-03, F-04 ni F-07–F-10.
- No se ejecutó `/speckit.implement`.

## 14. Riesgos y decisiones pendientes

Se requiere confirmación humana de si los cambios fechados después del informe del Paso 74, incluidos los helpers PTY, son una corrección autorizada que debe adoptarse como nueva línea base.

Hasta recibir esa confirmación, el Paso 75 permanece **BLOCKED** y no se afirma el cierre formal de F-01, F-02, F-05 o F-06.

No continuar automáticamente con otro paso.
