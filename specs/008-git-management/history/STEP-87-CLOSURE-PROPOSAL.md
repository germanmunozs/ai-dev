# Informe del Paso 87 — Propuesta de cierre formal de la Fase 1 (Feature 008)

Fecha: 2026-10-23
Modo: revisión de gobernanza, read-only. Ningún archivo del proyecto fue modificado durante la revisión. Este archivo se creó después, a petición expresa del usuario. **Es una propuesta; no cierra la Fase 1 ni acepta deuda.**

## A. Resumen ejecutivo

La Fase 1 (T001–T006) está **técnicamente lista para proponer su cierre**, pero no está cerrada. Se volvió a ejecutar la verificación: `gofmt`, `build`, `vet`, `go test -race` de los dos paquetes de Feature 008 y la suite completa (17 paquetes `ok`), con 0 procesos residuales. El código real coincide con los hashes de referencia y con el informe del Paso 86.

Hay cuatro actos distintos, y ninguno se ha ejecutado:

1. **Preparación técnica:** cumplida.
2. **Aceptación de contratos y deuda:** pendiente de decisión humana.
3. **Adopción de la línea base:** pendiente. Hoy todo es contenido verificado, no versionado.
4. **Cierre formal:** pendiente.

**Veredicto: READY TO PROPOSE CLOSURE**, con las condiciones de las secciones K y L.

## B. Preflight e integridad

- **Rama y HEAD:** `feature/008-git-management`, `a61ae2ffbb591a6282636805a2c35c81246b2d5c`. Ese commit pertenece a Feature 006; no hay commits de Feature 008.
- **Índice y diff:** vacíos.
- **Archivos sin seguimiento:** 38, inventariados abajo.
- **Hashes:** los siete archivos de referencia coinciden. `ports.go`, `doc.go`, `internal/app/gitmanagement.go` e `internal/app/gitmanagement_test.go` también mantienen sus hashes de pasos anteriores.
- **Manifiesto SHA-256 de todo el repositorio:** idéntico antes y después de la revisión.
- **Ejecución:** uid 501, sin `go.sum`, `go.mod` sin cambios. Sin operaciones Git mutantes.
- **Informes del Paso 72:** no existe `STEP-72-*.md`. Las fuentes disponibles son los Pasos 73–75 y 76A–86. Queda como pendiente de trazabilidad.

### Inventario de los 38 archivos sin seguimiento

- **Especificación (11):** `spec.md`, `plan.md`, `tasks.md`, `research.md`, `data-model.md`, `quickstart.md`, `checklists/requirements.md` y 4 contratos (`apply-strategy`, `authorization-audit`, `cli`, `inspection-plan`).
- **Código de Fase 1 (13):**
  - `internal/gitmanager/`: `doc.go`, `ports.go`, `boundary_test.go`, `fakes_test.go`, `testhelpers_test.go`, `sentinel_pty_posix_test.go`, `sentinel_pty_other_test.go`, `testdata/hostile-git/README.txt`.
  - `internal/gitprocess/doc.go`, `internal/gitstore/doc.go`, `internal/conventionalcommit/doc.go`.
  - `internal/app/gitmanagement.go` y `gitmanagement_test.go`.
- **Informes en la raíz (14):** `STEP-73` a `STEP-86`, incluidos 76A y 78–86.

## C. Matriz T001–T006

`tasks.md` tiene 0 casillas marcadas y 118 sin marcar. No se tocó ninguna.

| Tarea | Evidencia de implementación | Evidencia de pruebas | Estado | Observaciones |
|---|---|---|---|---|
| T001 | Cuatro `doc.go`; los paquetes siguen el DAG de `plan.md` | Compilan; `go vet` limpio | Cumplida | Paquetes vacíos por diseño. |
| T002 | `ports.go`: `GitRunner`, `GitStore`, `Filesystem`, `Clock`, `RandomSource`, `Reporter` | Aserciones de interfaz en `fakes_test.go` | Cumplida con observaciones | Forma de los puertos pendiente: F-03 y F-04. |
| T003 | `boundary_test.go` | `TestFeature008ImportBoundaries`, matriz M1–M11, grafo base sin falsos positivos | Cumplida con desviación | El texto de T003 dice que solo `gitprocess` importa `os/exec`. La implementación añade una excepción en `gitmanager` para `testhelpers_test.go` (harness de sentinelas, solo test). Es consistente con T004, pero no está escrita en `tasks.md`. |
| T004 | `testhelpers_test.go`, `testdata/hostile-git/README.txt` | Siete sentinelas ejecutadas con Git real, más pruebas de pager, PTY y limpieza | Cumplida con desviación de ubicación | Los dos `sentinel_pty_*_test.go` no figuran en T004, pero se crearon con autorización expresa (Paso 75). |
| T005 | `fakes_test.go` | Inyección de fallos, `io.ReadFull`, resultado parcial más error, concurrencia, copias defensivas | Cumplida | Pasa con `-race`. |
| T006 | `internal/app/gitmanagement.go` y su test | `TestGitManagementCompositionIsInertForExistingCommands` | Cumplida con observaciones | Dependencias nil, sin servicio "no disponible" explícito (F-07); sin test de que `ai-dev git` siga sin reconocerse (F-08). |

**Alcance:** ninguna tarea de Fase 2 fue implementada. Los únicos archivos no test son `ports.go`, `gitmanagement.go` y los `doc.go`.

## D. Contratos de seguridad

### S-01: ciclo de vida del proceso

- **Secuencia:** `Start` → captura de PGID (con `Setpgid` y `Pgid=0`, más probe `Signal(0)`) → supervisión (`wait4(WNOHANG)`) → señalización de grupo → recolección y `Release`.
- **Garantías demostradas:** `os.Process.Wait()` y `Release()` quedan detectados (probados con procesos reales). Un `wait4` crudo se detectó mientras el PID no fue reutilizado.
- **Supuesto de contrato:** no existe un reaper externo y concurrente que recolecte al hijo sin pasar por `os.Process`. Si se incumpliera y el PID se reutilizara, el probe `kill(pid,0)` tendría éxito sobre un ajeno; el helper aceptaría una identidad errónea.
- **Alcance:** solo infraestructura de pruebas.
- **Verificado solo en macOS:** en Linux, `os.Process` usa pidfd y debería ser más fuerte, pero no se ejecutó.

### S-02: reserva POSIX de PID/PGID

- **Garantía demostrada:** el líder no se recolecta mientras el lector PTY siga abierto, y la señal de grupo va antes de `wait4` y `Release`. No hay señales posteriores a la recolección.
- **Supuesto POSIX:** un líder zombie sin recolectar mantiene reservado su PID y por tanto su PGID.
- **Limitación:** los tests de proceso y PTY se ejecutaron nativamente solo en macOS/arm64; el resto de plataformas son compilación cruzada. FreeBSD y Windows usan el stub no POSIX.

### S-03: aislamiento de señales

- **Garantía demostrada:** `groupID <= 1` se rechaza antes de `kill`, y la prueba negativa usa un callback inerte; quitar el guard hace fallar la prueba sin señalar nada.
- **Señales reales:** solo a procesos y grupos sintéticos propios. El registro de destinos en las ejecuciones mostró `unsafe_targets=0`.
- **Matiz:** la función real acepta cualquier `groupID > 1` (N-20), pero solo se la llama con el PGID capturado.

### S-04: límites temporales

- **Garantía demostrada:** timeout de 10 s y limpieza de 2 s. Sin goroutine lectora, 2→2 goroutines y 5→5 descriptores en los cuatro escenarios, 0 procesos residuales.
- **Limitación:** caudal de lectura de unos 200 KB/s (N-16).

### S-05: infraestructura inerte

- **Garantía demostrada:** los archivos no test son solo `ports.go`, `gitmanagement.go` y los `doc.go`. `os/exec` aparece únicamente en `boundary_test.go` (dentro de cadenas de prueba) y `testhelpers_test.go`.
- **Composición:** `GitManagementDependencies` es un struct de interfaces sin cableado y sin rutas CLI. El único otro uso de Git en el repositorio es el `git-tool` de `doctor.go`, previo a Feature 008.
- No hay permisos de mutación ni ejecución de planes; los puertos solo se implementan como fakes.

## E. Inventario de deuda técnica

Severidades tomadas de los informes originales; no se asignó ninguna nueva.

### 1. Deuda informativa o de infraestructura de pruebas

| ID | Sev. | Descripción | Origen | Recomendación |
|---|---|---|---|---|
| N-05 | INFO | Skip silencioso del pager si no hay PTY | 76A | Aceptar |
| N-06 | INFO | Sentinela `credential` generada y nunca disparada | 76A | Aceptar |
| N-07 | INFO | ioctl en linux/ppc y mips sin verificar | 76A | Aceptar |
| N-08 | INFO | `testhelpers_test.go` sin build tag para Windows | 76A | Aceptar |
| N-11 | INFO | Dos mutaciones sin cobertura (limpieza preventiva, `master.Close`) | 78 | Aceptar |
| N-12 | INFO | Stub no POSIX: sin comprobación real de limpieza | 78 | Aceptar |
| N-15 | LOW | `Fd()` borra `O_NONBLOCK`; la afirmación "no bloqueante" es falsa, aunque `select` previo lo hace seguro | 80 | Aceptar |
| N-16 | LOW | Caudal ~200 KB/s; más de ~2 MiB en 10 s da falso timeout | 80 | Aceptar |
| N-17 | INFO | `readerFinished=true` incondicional en rutas de timeout | 80 | Aceptar |
| N-20 | INFO | La terminación acepta cualquier `groupID > 1` | 82 | Aceptar |
| N-21 | INFO | Carrera de microsegundos al matar por PID leído de archivo | 82 | Aceptar |
| N-22 | INFO | `pollPseudoTerminalProcess` sin guard `Pid <= 0`; rama muerta | 82 | Aceptar |
| N-27 | INFO | Probe basado en PID en macOS (límite de S-01) | 86 | Aceptar con S-01 |
| N-28 | INFO | Guards nil detectados por pánico, no por fallo limpio | 86 | Aceptar |

### 2. Deuda arquitectónica del detector (T003)

| ID | Sev. | Descripción | Recomendación |
|---|---|---|---|
| N-03 | LOW | La comprobación indirecta no tiene prueba propia | Aceptar, o resolver antes de que adaptadores dependan del detector |
| N-04 | LOW | `syscall.Syscall(SYS_EXECVE…)` y `plugin` no se detectan; los adaptadores no tienen lista de imports permitidos | Resolver antes de T016/T018, o decidir su alcance |

### 3. Riesgos de seguridad que requieren contrato explícito

S-01 y S-02 (sección D).

### 4. Hallazgos que pueden bloquear tareas futuras

| ID | Sev. | Descripción | Resolver antes de |
|---|---|---|---|
| F-03 | MINOR | `GitInvocation` con `Executable`, `Args`, `Environment` sueltos | T016 y T018 |
| F-04 | MINOR | `GitStore` y `Filesystem` como sobres genéricos con cadenas; `Reporter` recibe bytes ya renderizados | T008, T021 y T023 |
| F-07 | OBS | Dependencias "no disponibles" son todo nil | Fase 12 (según Paso 74) |
| F-08 | OBS | Sin test de que `ai-dev git …` siga sin reconocerse | Fase 12 |
| F-09 | OBS | Fixture `.git` vacío; `writeFile` acepta `..` | Aceptar |
| F-10 | OBS | Informes `STEP-*.md` en la raíz fuera de la lista autorizada | Decidir su destino al adoptar la línea base |

### Pendiente de trazabilidad

No existe N-24 (el Paso 84 saltó de N-23 a N-25) y no hay informe del Paso 72. Los demás identificadores se reconstruyen desde los informes de los Pasos 76A–86.

## F. Análisis F-03: `GitInvocation`

**Hecho del código real** (`ports.go`):

```go
type GitInvocation struct {
	Executable string; Args []string; Directory string
	Environment []string; Stdin []byte
	StdoutLimit, StderrLimit int64
}
```

`plan.md:88` dice que el runner debe recibir "una operación tipada, un `GitToolIdentity` congelado, argv exacto, directorio físico, stdin controlado, límites y un perfil de entorno", y que el runner construye el entorno desde una lista de permitidos (T018 y T019). T008 define `GitToolIdentity`, y T016 y T018 viven en `gitprocess`.

- **A. Riesgo:** con `Environment` y `Executable` libres, el llamador puede inyectar variables `GIT_*` y rutas arbitrarias. La seguridad pasaría a depender de que todos los llamadores respeten la convención.
- **B. ¿Aceptable en Fase 1?** Sí: no hay runner ni llamadores productivos.
- **C. Garantías previas a un runner real:** operación de un conjunto cerrado, identidad del ejecutable congelada, entorno derivado por el runner (no aportado por el llamador), argv exacto verificado contra la matriz P-04 y límites de salida.
- **D/E. Alternativas:**
  1. **Mantener la estructura actual.** Barato hoy; traslada toda la disciplina a `gitprocess` y la prueba solo con tests. Riesgo alto de evolución.
  2. **Descriptor cerrado:** `{Operation (enum), Tool GitToolIdentity, Directory, Profile, Stdin, Limits}`, sin `Executable` ni `Environment`. Un runner único construye argv y entorno. Más seguro; exige que `GitToolIdentity` exista (T008). Reescribe `ports.go` y `fakes_test.go`, ambos pequeños y sin llamadores productivos.
  3. **Una operación tipada por método** (un método por entrada de P-04). La más fuerte; la más verbosa; acopla el puerto a la matriz completa.
- **F. Aprobación humana y planes canónicos:** las alternativas 2 y 3 permiten que el digest del plan cubra las operaciones y el perfil de entorno, no cadenas libres.
- **G. Impacto de cambiarlo en Fase 2:** el puerto lo usan hoy solo fakes y un struct de composición, así que el costo ahora es mínimo. Cambiarlo después de T016–T019 obligaría a reescribir tests y adaptador.
- **H. Dependencia con T016:** T016 prueba "exact-argv/no-shell, environment allowlist"; con la forma actual esas pruebas no podrían impedir que un llamador aporte su propio entorno.

**Recomendación para decisión humana (no adoptada):** alternativa 2 (o 3), decidida antes de T016 y alineada con T008. Si se prefiere no tocar Fase 1, registrar F-03 como bloqueante explícito de T016.

## G. Análisis F-04: `GitStore`, `Filesystem` y `Reporter`

**Hecho del código real:** `GitStore.Execute(ctx, GitStoreRequest{Operation, Key, Version, Payload})`, `Filesystem.Execute(ctx, FilesystemRequest{Operation string, Path string, Data, Mode, Limit})` y `Reporter.Report(ctx, ReportRequest{Kind, Format, Payload []byte})`.

- **Riesgos de capacidad excesiva:** `Operation` es una cadena libre; el puerto no distingue lectura de mutación. `Path` es una ruta, mientras `plan.md:101` y `:239` exigen resolución basada en descriptores, identidad dispositivo/inodo y operaciones sin seguir enlaces. Un adaptador podría exponer cualquier operación sin pasar por el Safety Kernel.
- **Colisión con T021:** T021 define el puerto de filesystem en `internal/gitmanager/filesystem.go`, mientras `ports.go` ya declara `Filesystem`. Habrá que reconciliar dos definiciones.
- **Reporter:** el plan atribuye el renderizado a `internal/report` (`plan.md:77`); el puerto actual recibe bytes ya renderizados.
- **Relación con T016, T021, T023:** T021 (filesystem) y T022/T023 (escritura de registros, CAS, auditoría append-only) fijan las operaciones reales. La autorización durable y el CAS no caben en un sobre `Operation+Payload` sin perder la separación entre planificación read-only y mutaciones autorizadas.

### Alternativas

| Alternativa | Beneficios | Costos | Riesgos | Impacto en calendario |
|---|---|---|---|---|
| 1. Mantener temporalmente | Sin trabajo ahora; la Fase 1 queda como está | Se pospone la decisión | Los adaptadores podrían consolidar el sobre genérico | Ninguno ahora; mayor retrabajo en T021–T023 |
| 2. Endurecer antes de los adaptadores | Prohíbe operaciones desconocidas y rutas libres con poco cambio | Cambio medio en `ports.go` y fakes | Un endurecimiento parcial puede quedar a medio camino respecto de la forma final | Pequeño: antes de T021 |
| 3. Rediseñar con descriptores tipados y capacidades acotadas | Encaja con descriptores, identidad de inodo y CAS del plan | Mayor; depende de los tipos de T008 | Rediseñar antes de conocer los tipos puede obligar a rehacerlo | Medio: debe hacerse junto con T008, antes de T021 |

**Recomendación para decisión humana (no adoptada):** alternativa 3 aplicada al mismo tiempo que T008 (donde nacen los tipos), o alternativa 2 como paso intermedio. En cualquier caso, la decisión debe estar tomada antes de T016 y T021, y se debe reconciliar la duplicidad `Filesystem` / `filesystem.go`.

## H. Línea base técnica y estado de versionamiento

| Concepto | Estado |
|---|---|
| Rama / HEAD | `feature/008-git-management` / `a61ae2f` (Feature 006) |
| Archivos rastreados modificados | Ninguno |
| Archivos sin seguimiento | 38 (11 de especificación, 13 de código, 14 informes) |
| Hashes verificados | Sí |
| Evidencia de pruebas | Reproducida: build, vet, race, suite completa |
| Evidencia de auditoría | Pasos 74, 76A, 78, 80, 82, 84 y 86 |
| `tasks.md` | 0 de 118 casillas marcadas |

### Hashes de referencia verificados

| Archivo | SHA-256 |
|---|---|
| `specs/008-git-management/tasks.md` | `614386e5f3e4a3388292d5026dab021b87dc14bb8ad6cdb7707874fe38e8f8e4` |
| `specs/008-git-management/spec.md` | `8e09095d8bb33b7fbd7a74e4f4340226fc520501667deb8436c66a0a50f54304` |
| `internal/gitmanager/testhelpers_test.go` | `13af569ae6a032c31a7d5fe69b35d456758a10a3457f956ec0404da7e6aba213` |
| `internal/gitmanager/sentinel_pty_posix_test.go` | `68e07ad0741b6131f3d242ac67748ef2aa782310bfcfab8ea31e5991d0e955c1` |
| `internal/gitmanager/sentinel_pty_other_test.go` | `ca30e5029eb70cb737ecdbdb79426e5b09812bfae13d179052c567c19d09d4f0` |
| `internal/gitmanager/boundary_test.go` | `d70a291a379e85c37a68ff5ac31983602a340d9b4762b4bd385a3f3cf8b0c10c` |
| `internal/gitmanager/fakes_test.go` | `7292d7cf40a598753eb9182be0c4d31b87b988796e21e2e3d09690af15d05986` |

### Tres situaciones que no son equivalentes

- **Línea base verificada:** sí, por contenido y hashes.
- **Línea base versionada:** **no.** Todo está sin seguimiento; si se pierde el directorio de trabajo, no hay copia en Git.
- **Fase cerrada:** **no.** Requiere una decisión humana formal registrada.

Para una línea base reproducible hacen falta: decidir qué archivos entran (en particular, qué se hace con los 14 `STEP-*.md`, ver F-10), incorporarlos mediante un proceso autorizado y registrar el commit. No se ejecutó ni se preparó ninguna de esas operaciones.

## I. Matriz C-01 a C-14

| Criterio | Resultado | Evidencia |
|---|---|---|
| C-01 T001–T006 implementadas y verificadas | PASS WITH CONDITIONS | Sección C; desviaciones de T003/T004 por aceptar |
| C-02 Matriz F-01 | PASS | M1–M11 detectadas; mismo hash de `boundary_test.go` desde el Paso 75 |
| C-03 Sentinelas F-02 | PASS | 7/7 con Git real; witness y marcador |
| C-04 Fakes y RandomSource | PASS | `fakes_test.go` sin cambios desde el Paso 75; pasa con `-race` |
| C-05 Infraestructura inerte | PASS | Sección D (S-05) |
| C-06 N-09, N-10, N-13, N-14, N-18, N-19, N-23 sin regresión | PASS WITH CONDITIONS | Paso 86; sujeto a S-01 y S-02 |
| C-07 N-25 y N-26 | PASS | Paso 86; 10 mutaciones |
| C-08 Suite completa y race | PASS | Reproducido |
| C-09 Integridad | PASS | Manifiesto idéntico; sin `go.sum`; ningún normativo cambiado |
| C-10 Contratos documentados y pendientes de aceptación | PASS WITH CONDITIONS | S-01 a S-05; falta la aceptación humana |
| C-11 Deuda inventariada y pendiente de aceptación | PASS WITH CONDITIONS | Sección E; N-24 y el Paso 72 sin trazabilidad |
| C-12 Línea base reproducible y versionamiento explícito | PASS WITH CONDITIONS | Verificada sí, versionada no |
| C-13 F-03/F-04 identificadas y planificadas antes de T016 | PASS WITH CONDITIONS | Identificadas y analizadas; la planificación depende de decisión humana |
| C-14 Ninguna Fase 2 iniciada | PASS | Solo `doc.go` en `gitprocess`, `gitstore` y `conventionalcommit` |

Ningún criterio es FAIL o NOT VERIFIED.

## J. Riesgos residuales y dependencias futuras

- S-01 y S-02 dependen de supuestos de sistema operativo; la ejecución real solo se probó en macOS/arm64.
- F-03 y F-04 deben resolverse antes de T016 (y F-04 también antes de T021 y T023).
- N-04 conviene tratarlo antes de que los adaptadores de `gitprocess` y `gitstore` existan.
- La duplicidad `Filesystem` (`ports.go`) / `filesystem.go` (T021).
- Riesgo de pérdida de trabajo mientras la línea base siga sin versionar.
- El texto de T003 no recoge la excepción del harness de sentinelas.

## K. Decisiones humanas pendientes

1. Aceptar o rechazar los contratos S-01 a S-05.
2. Aceptar las deudas informativas de la tabla 1 y decidir sobre N-03 y N-04.
3. Elegir alternativa para F-03 y para F-04, y cuándo aplicarla.
4. Decidir si se aceptan las desviaciones de T003 y T004 o se ajusta `tasks.md`.
5. Decidir cómo versionar la línea base (qué archivos, y qué hacer con los `STEP-*.md`).
6. Decidir si se registra el cierre de la Fase 1 y quién lo hace.
7. Decidir la autorización de la Fase 2.

## L. Veredicto final

**READY TO PROPOSE CLOSURE.**

- **Preparación técnica:** cumplida.
- **Aceptación de deuda y contratos:** pendiente.
- **Adopción de la línea base:** pendiente.
- **Cierre formal:** pendiente.

Ninguno se ejecuta automáticamente.

## M. Borrador de texto de aceptación humana: NO APROBADO

> **Borrador, no aprobado. Requiere revisión y decisión del responsable del proyecto.**
>
> Yo, [responsable del proyecto], habiendo revisado los informes de auditoría de la Fase 1 de Feature 008 y la propuesta del Paso 87, decido:
>
> 1. Aceptar los contratos de seguridad S-01 a S-05 en los términos descritos [con las siguientes modificaciones: ___].
> 2. Aceptar como deuda técnica temporal los hallazgos [lista] y mantener abiertos [lista] como condición para [tarea].
> 3. Resolver F-03 mediante la alternativa [1 / 2 / 3] antes de T016, y F-04 mediante la alternativa [1 / 2 / 3] antes de T021.
> 4. Aceptar / ajustar las desviaciones de T003 y T004.
> 5. Adoptar como línea base de la Fase 1 el contenido verificado con los hashes indicados, y versionarlo mediante el siguiente procedimiento: [___].
> 6. Declarar / no declarar cerrada la Fase 1 con fecha [___].
> 7. Autorizar / no autorizar el inicio de la Fase 2 en los términos [___].
>
> Fecha: ___  Firma: ___

---

Este documento no cierra la Fase 1, no adopta la línea base, no acepta deuda técnica ni autoriza la Fase 2, la modificación de `tasks.md` o la creación de commits.
