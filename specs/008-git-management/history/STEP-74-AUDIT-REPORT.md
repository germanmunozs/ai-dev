# Informe del Paso 74 — Auditoría independiente de Feature 008, Fase 1 (T001–T006)

Fecha: 2026-10-13
Modo: auditoría read-only (ningún archivo del proyecto modificado durante la auditoría).

## 1. Veredicto

| Punto | Resultado |
|---|---|
| Veredicto | **PASS WITH CONDITIONS** |
| ¿Listo para pedir autorización de la Fase 2? | **NOT READY** hasta corregir F-01 y F-02. Después basta una verificación focalizada; no hace falta repetir la auditoría completa. |

El código compila, todas las pruebas pasan, no se salió del alcance y no hay ninguna operación Git mutante. Sin embargo, las dos redes de seguridad que la Fase 2 va a usar directamente tienen defectos comprobados:

- **Prueba de límites (T003):** deja pasar 8 de 11 maneras realistas de saltarse el diseño.
- **Sentinelas hostiles (T004):** varias no registran nada cuando Git las ejecuta, así que una prueba que compruebe "no se ejecutaron" pasaría aunque sí se hubieran ejecutado.

## 2. Preflight: PASS

- **Repositorio:** `/Users/german/Developer/ai-dev`
- **Rama:** `feature/008-git-management`
- **HEAD:** `a61ae2ffbb591a6282636805a2c35c81246b2d5c`. No hay commits posteriores.
- **SHA-256 `tasks.md`:** `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` (coincide)
- **SHA-256 `spec.md`:** `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` (coincide)
- **Estado Git:** solo archivos sin seguimiento. `git diff` y `git diff --cached` vacíos. 0 casillas marcadas en `tasks.md`.

## 3. Estado de cada tarea

| Tarea | Estado | Motivo |
|---|---|---|
| T001 | PASS | Los 4 paquetes solo tienen `doc.go`. La documentación coincide con el grafo de dependencias de `plan.md:64` y no hay código adelantado. |
| T002 | PASS con observaciones | Los 6 puertos existen, están del lado del consumidor, solo usan la biblioteca estándar y ninguno muta nada. Su forma no coincide del todo con el plan (F-03, F-04). |
| T003 | **CONDICIONAL** | F-01 |
| T004 | **CONDICIONAL** | F-02 |
| T005 | PASS con observaciones menores | F-05, F-06 |
| T006 | PASS | Composición inerte. `doctor.go` sin cambios y ningún comando `git` registrado. |

## 4. Resultados de las pruebas

Caché de Go fuera del repositorio (scratchpad) y `GOFLAGS=-mod=readonly`.

| Verificación | Resultado |
|---|---|
| `go build` de los paquetes afectados | OK |
| `go vet` de los paquetes afectados | OK |
| `gofmt -l` | Sin salida |
| Pruebas focalizadas con `-race` (gitmanager y los 3 paquetes vacíos) | PASS, 3 pruebas |
| `TestGitManagementCompositionIsInertForExistingCommands` | PASS |
| `go test -count=1 ./...` | Todos los paquetes `ok` |
| Mutaciones de la prueba de límites (copia fuera del repositorio) | 3 detectadas, 8 no detectadas (ver F-01) |
| Ejecución real de las sentinelas (repo temporal, HOME aislado, `GIT_CONFIG_NOSYSTEM=1`) | Ver F-02 |

## 5. Hallazgos

### F-01 · MAJOR · T003 · `internal/gitmanager/boundary_test.go:15-37`

**Defecto.** La prueba usa una lista de prohibidos: solo mira 3 importaciones exactas, solo importaciones directas y solo archivos que no son de prueba. No verifica que los paquetes de Feature 008 importen únicamente lo permitido, ni las dependencias indirectas. No incluye la comprobación negativa con código fuente sintético que sí tienen `internal/sdd/boundary_test.go:124` e `internal/gatestore/boundary_test.go`.

**Evidencia (mutaciones).** Todas compilan y la prueba sigue pasando:

- M3: `gitmanager` importa `internal/system`, que usa `os/exec`.
- M4: `gitmanager` importa `internal/report` (prohibido por `plan.md:64`).
- M5: `gitmanager` importa `internal/gatestore` (acoplamiento con Human Gates).
- M6: un subpaquete de `gitmanager` importa `os/exec`.
- M7: `conventionalcommit` importa `gitmanager` (deja de ser hoja).
- M8 y M9: `os.StartProcess` y `syscall.Exec`.
- M11: un `_test.go` importa `os/exec`.

Sí se detectan M1 (`gitmanager` → `gitprocess`), M2 (`gitmanager` → `os/exec`) y M10 (`gitstore` → `os/exec`).

**Riesgo.** `plan.md:64` afirma que el grafo "is enforced by a structural import test", y esta prueba es el único control. En la Fase 2 se implementan `gitprocess` y `gitstore` (T016–T023), justo cuando este control más importa.

**Corrección mínima.**

- `gitmanager` solo puede importar la biblioteca estándar y `conventionalcommit`.
- `conventionalcommit` solo puede importar la biblioteca estándar.
- Comprobar dependencias indirectas (`go list -deps` o recorrido del árbol) para `os/exec`, `gitprocess` y `gitstore`.
- Recorrer subdirectorios.
- Prohibir `os.StartProcess` y `syscall.Exec*`.
- Añadir autoprueba negativa con código fuente infractor, como en `sdd`.
- Decidir explícitamente si los `_test.go` quedan excluidos.

### F-02 · MAJOR · T004 · `internal/gitmanager/testhelpers_test.go:22, 28-33, 38`

**Defecto.** El script escribe en `"$1"`, pero las sentinelas de entorno no le pasan la ruta del marcador. Comprobado ejecutando Git real:

- **`GIT_EDITOR`:** el marcador no se escribe; además el script sobrescribió el mensaje del commit con "invoked".
- **`GIT_PAGER`:** el marcador no se escribe.
- **`gpg.program` (`"cmd marker"`):** Git no lo ejecuta mediante shell; no corre y el marcador no se escribe.
- **`GIT_ASKPASS` y `GIT_SSH_COMMAND`:** no ejecutados; por el mismo mecanismo `$1` sería el prompt o el host, no se registraría nada y se crearía un archivo en el directorio de trabajo.
- **`core.fsmonitor` y `filter.*.clean`:** el marcador sí se escribe.

**Riesgo.** Las comprobaciones de "marcador ausente" en T016 pasarían aunque el helper se hubiera ejecutado: falsos negativos de seguridad.

**Corrección mínima.**

- Escribir en el script la ruta absoluta del marcador y usar `>>` en vez de depender de `$1`.
- Configurar `gpg.program` con la ruta del script, sin argumentos.
- Añadir una autoprueba que invoque cada sentinela como lo haría Git y compruebe que el marcador aparece.

### F-03 · MINOR · T002 · `internal/gitmanager/ports.go:12-26`

**Defecto.** `GitInvocation` recibe `Executable`, `Args` y `Environment` sueltos. Según `plan.md:87` y `plan.md:169`, el runner debe recibir una operación tipada, un `GitToolIdentity` congelado y un perfil de entorno, y construir el entorno él mismo desde una lista de permitidos (T018 y T019, en `gitprocess`). Con la forma actual, quien llama puede inyectar cualquier entorno.

**Calificación.** Pendiente legítimo (esos tipos llegan en T008), pero el campo `Environment` libre contradice el diseño.

**Corrección.** Alinear el puerto antes de T016 y registrar la decisión.

### F-04 · MINOR · T002 · `internal/gitmanager/ports.go:38-108`

**Defecto.**

- `GitStore` y `Filesystem` son sobres genéricos `Execute(Operation string, Payload []byte)`.
- `Filesystem` trabaja con rutas; el plan exige resolución basada en descriptores (`plan.md:101` y `:239`). Además T021 definirá el puerto de filesystem en `gitmanager/filesystem.go`, con riesgo de dos definiciones solapadas.
- `Reporter` recibe bytes ya renderizados; el renderizado corresponde a `internal/report` (`plan.md:77`).

**Corrección.** Marcarlos como provisionales o sustituirlos por métodos tipados en T008, T021 y T023.

### F-05 · MINOR · T005 · `internal/gitmanager/fakes_test.go:77-90`

**Defecto.**

- Un `fakeRandomSource` con valores por defecto tiene `ErrAt=0` y falla en la primera lectura.
- Al agotarse los bytes devuelve `(0, nil)`, por lo que `io.ReadFull` entraría en bucle infinito (deducido del código de `io.ReadAtLeast`).

**Corrección.** Devolver `io.ErrUnexpectedEOF` al agotarse y usar un constructor o un `ErrAt` opcional.

### F-06 · MINOR · T005 · `internal/gitmanager/fakes_test.go:18-100`

**Defecto.**

- Con error inyectado siempre se devuelve un resultado vacío: no se puede simular efecto parcial + error (p. ej. `Applied=true` con fallo de fsync), necesario para probar que no se declaran efectos falsos.
- Sin mutex: las pruebas de concurrencia de la Fase 3 con `-race` darían carreras.
- `fakeClock` devuelve en silencio la hora cero al agotarse.
- Las copias defensivas no tienen prueba.

Ninguno de estos cambios afecta a los contratos.

### F-07 · OBSERVATION · T006 · `internal/app/gitmanagement.go:16-18`

El conjunto de dependencias "no disponible" es una estructura con todos los campos nil, usada solo por pruebas. `plan.md:81` pide un servicio *explícitamente* no disponible; con interfaces nil el código futuro entraría en pánico en lugar de fallar de forma controlada. Resolver al conectar la composición (Fase 12).

### F-08 · OBSERVATION · T006 · `internal/app/gitmanagement_test.go:18-28`

La prueba no comprueba que `ai-dev git …` siga sin reconocerse (salida 2). La inercia sí queda confirmada por inspección: el despacho de `internal/app/doctor.go:43-95` no cambió.

### F-09 · OBSERVATION · T004

- `newRepositoryFixture` crea un `.git` vacío, no un repositorio real.
- `writeFile` acepta rutas con `..`.
- `filter.hostile` no hace nada sin un `.gitattributes` que lo asocie, y el helper no lo crea.

### F-10 · OBSERVATION · alcance

- `STEP-73-REPORT.md`, en la raíz del repositorio, queda fuera de la lista de archivos autorizados (artefacto de informe).
- Su sección de estado Git final no se incluye a sí mismo.
- Afirma "Riesgos inmediatos: ninguno", lo que F-01 y F-02 contradicen.
- La regla sobre `os/exec` se limita a los paquetes de Feature 008, porque `internal/system` ya lo usa legítimamente. Interpretación razonable, pero conviene documentarla en la prueba.

**Revisión de alcance:** sin problemas. No se adelantó trabajo de las Fases 2–12, no hay dependencias nuevas, no se modificaron documentos normativos, no hay rutas de mutación Git y no hay commits ni pushes.

## 6. Riesgos residuales

- Con F-01 abierto, los adaptadores de la Fase 2 pueden romper el grafo de dependencias sin que falle ninguna prueba.
- Con F-02 abierto, las pruebas de T016 sobre helpers, editor, pager, firma y askpass pueden pasar sin comprobar nada.
- La forma provisional de los puertos (F-03, F-04) obligará a rehacerlos en T008, T018 y T021, con riesgo de consolidar sobres genéricos basados en cadenas.

## 7. Archivos revisados

- **Código de Fase 1:** los 11 archivos creados en el Paso 73:
  - `internal/app/gitmanagement.go`, `internal/app/gitmanagement_test.go`
  - `internal/conventionalcommit/doc.go`, `internal/gitprocess/doc.go`, `internal/gitstore/doc.go`
  - `internal/gitmanager/doc.go`, `ports.go`, `boundary_test.go`, `testhelpers_test.go`, `fakes_test.go`, `testdata/hostile-git/README.txt`
- **Código existente:** `internal/app/doctor.go`, `internal/sdd/boundary_test.go`, `internal/gatestore/boundary_test.go`.
- **Especificación:** `tasks.md` (Fases 1 y 2), `plan.md` (arquitectura, matriz de Git, estructura de paquetes), búsquedas en `research.md`, `data-model.md` y `contracts/*`, y `STEP-73-REPORT.md`.

## 8. Estado final del repositorio (al cierre de la auditoría)

Rama `feature/008-git-management`, HEAD `a61ae2ffbb591a6282636805a2c35c81246b2d5c`. Hashes de `tasks.md` y `spec.md` sin cambios. Estado Git idéntico al del preflight. Ningún archivo modificado durante la auditoría; las copias temporales usadas para mutaciones y sentinelas se eliminaron.

Nota: este archivo (`STEP-74-AUDIT-REPORT.md`) se creó después de la auditoría, a petición expresa del usuario, y es el único archivo nuevo sin seguimiento añadido por el Paso 74.

## 9. Siguiente paso recomendado

Autorizar un paso de corrección acotado a la Fase 1 que resuelva F-01 y F-02 (opcionalmente F-05 y F-06, baratos de arreglar). Después, verificación focalizada: repetir la batería de mutaciones y la ejecución real de las sentinelas. Con eso, solicitar la autorización humana de la Fase 2 y registrar en ella la decisión sobre la forma tipada de los puertos (F-03 y F-04) antes de T016.
