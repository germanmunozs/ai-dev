# Informe del Paso 88 — Propuesta de decisiones de gobernanza y versionamiento (Feature 008, Fase 1)

Fecha: 2026-10-23
Modo: revisión de gobernanza, read-only. Ningún archivo del proyecto fue modificado durante la revisión. Este archivo se creó después, a petición expresa del usuario. **Es una propuesta para decisión humana; no ejecuta ni aprueba ninguna decisión.**

## A. Resumen ejecutivo

Esta es una propuesta para decisión humana. No se ejecutó ninguna decisión, ni se cambió nada en el repositorio.

Se contrastó el estado real con el Paso 87. Coincide, con una diferencia: había **39** archivos sin seguimiento, porque se sumó `STEP-87-CLOSURE-PROPOSAL.md`. Con este informe serán 40.

Recomendaciones principales:

- **Contratos S-01 a S-05:** aceptarlos, con las limitaciones descritas.
- **Deuda:** aceptar temporalmente los grupos A, B y D. F-03 y F-04 no son deuda informativa, sino bloqueos de decisión antes de T016 y T021.
- **F-03:** descriptor cerrado (alternativa 2), decidido antes de T016.
- **F-04:** descriptores tipados y capacidades explícitas (alternativa 3), junto con T008 y antes de T021.
- **T003 y T004:** aceptar las desviaciones y dejarlas registradas por escrito.
- **Versionamiento:** especificación y código en commits separados. Para los informes, **alternativa B**: versionarlos íntegros en una ubicación histórica dentro del directorio de la feature, con un manifiesto SHA-256. No excluirlos sin preservación verificable.

**Veredicto: READY FOR HUMAN DECISION.**

## B. Preflight e integridad

- **Rama y HEAD:** `feature/008-git-management`, `a61ae2ffbb591a6282636805a2c35c81246b2d5c` (Feature 006).
- **Índice y diff:** vacíos. Archivos rastreados modificados: 0.
- **Sin seguimiento:** 39 al momento de la revisión (11 de especificación, 13 de código, 14 informes `STEP-73` a `STEP-86`, y `STEP-87-CLOSURE-PROPOSAL.md`).
- **Hashes:** los siete archivos esperados coinciden.
- **Manifiesto SHA-256 del repositorio:** igual al del Paso 87, salvo el archivo nuevo del propio Paso 87.
- **Normativos, código productivo y dependencias:** sin cambios. Sin implementación de Fase 2.
- **`tasks.md`:** 0 de 118 casillas marcadas.

### Contexto de versionamiento observado en el historial

- Features 001–006 versionan documentos de especificación y código, nunca informes `STEP-*`.
- Feature 006 siguió esta secuencia: `docs: add feature 006 specifications and audit records`, luego `feat: add human-gated workflow transitions`, luego `docs: record feature 006 human implementation gate approval`.
- Ese último commit registró la aprobación humana como una sesión fechada dentro de `spec.md`, `plan.md`, `tasks.md` y otros normativos.
- `.gitignore` solo excluye artefactos de Go y archivos de editor; no afecta a estos archivos.

## C. Contratos S-01 a S-05

Todos quedan en **PENDIENTE DE APROBACIÓN**.

### S-01: ciclo de vida del proceso

- **Texto propuesto:** "En el harness de pruebas de PTY no existe un reaper externo y concurrente que recolecte los procesos sintéticos sin pasar por `os.Process`. El PGID se captura inmediatamente después de `Start`, el líder no se recolecta antes de señalizar su grupo, y `os.Process.Signal(0)` se usa como probe de ciclo de vida dentro de este contrato. El alcance es la infraestructura de pruebas."
- **Evidencia:** probado con procesos reales en macOS. Vivo y zombie se aceptan. Tras `Wait()`, `Release()` y `wait4` crudo se rechazan. Diez mutaciones del capturador se detectaron.
- **Limitaciones:** el probe en macOS se basa en PID. Si un reaper externo recolectara al hijo y el PID se reutilizara, `kill(pid, 0)` tendría éxito sobre un proceso ajeno, el probe diría "no recolectado" y el helper aceptaría una identidad errónea. En Linux, `os.Process` usa pidfd y es más fuerte, pero no se ejecutó.
- **Consecuencias de aceptar:** se acepta una garantía por orden de llamadas y no por construcción, acotada a pruebas.
- **Recomendación:** aceptar.

### S-02: reserva POSIX de PID/PGID

- **Texto propuesto:** "Un líder no recolectado conserva el PID y el PGID necesarios para la señalización. La señal de grupo precede a `wait4`/`Release`, y no se permite señalización posterior a la recolección. Las garantías ejecutadas nativamente corresponden a macOS/arm64; la compilación cruzada no equivale a pruebas nativas."
- **Evidencia:** la secuencia está en el código y los tests de timeout la ejercen. En el registro de destinos, cero objetivos inseguros.
- **Limitaciones:** el supuesto POSIX no se probó en Linux. FreeBSD y Windows usan el stub no POSIX.
- **Consecuencias de aceptar:** se acepta la reserva POSIX como supuesto, con evidencia nativa solo en macOS.
- **Recomendación:** aceptar.

### S-03: aislamiento de señales

- **Texto propuesto:** "Se rechaza `groupID <= 1` antes de cualquier `kill`. Las señales reales se dirigen solo a procesos sintéticos propios. Las pruebas negativas usan callbacks inertes y ninguna señal alcanza grupos ajenos."
- **Evidencia:** quitar el guard del seam hace fallar la prueba con el callback inerte, sin ninguna señal real.
- **Limitaciones:** la función real acepta cualquier `groupID > 1` (N-20), pero solo se la llama con el PGID capturado.
- **Consecuencias de aceptar:** se acepta N-20 como deuda ligada a este contrato.
- **Recomendación:** aceptar.

### S-04: límites temporales

- **Texto propuesto:** "La supervisión del pager tiene un timeout de 10 s y una limpieza de 2 s. No hay goroutines lectoras bloqueadas ni procesos residuales."
- **Evidencia:** 2→2 goroutines y 5→5 descriptores. El escenario `setsid` termina en unos 0,6 s.
- **Limitaciones:** caudal de lectura de unos 200 KB/s (N-16).
- **Consecuencias de aceptar:** los pagers de prueba deben emitir menos de ~2 MiB.
- **Recomendación:** aceptar.

### S-05: infraestructura inerte

- **Texto propuesto:** "La Fase 1 no activa operaciones Git productivas. Los puertos no tienen adaptadores productivos. No hay ejecución automática de planes. La autorización de mutaciones queda fuera del alcance de la Fase 1."
- **Evidencia:** los únicos archivos no test son `ports.go`, `gitmanagement.go` y los `doc.go`. `os/exec` solo aparece en `testhelpers_test.go` y en cadenas de `boundary_test.go`. `internal/app` no cablea el struct de dependencias.
- **Limitaciones:** ninguna relevante en Fase 1.
- **Consecuencias de aceptar:** la autorización de mutaciones queda para las fases siguientes.
- **Recomendación:** aceptar.

## D. Política de deuda técnica

Todas las decisiones están **PENDIENTES DE APROBACIÓN**. Severidades tomadas de los informes originales.

### Grupo A. Deuda informativa o de infraestructura de pruebas

| ID | Severidad | Decisión propuesta | Condición | Punto de revisión |
|---|---|---|---|---|
| N-05 | INFO | Aceptación temporal | Revisar si CI no dispone de PTY | Fase 12 |
| N-06 | INFO | Aceptación temporal | — | Fase 12 |
| N-07 | INFO | Aceptación temporal | Verificar en Linux nativo si se ejecutan tests PTY allí | Evidencia de plataforma (Fase 12) |
| N-08 | INFO | Aceptación temporal | — | Fase 12 |
| N-11 | INFO | Aceptación temporal | — | Cualquier refactor del helper |
| N-12 | INFO | Aceptación temporal | — | Evidencia de plataforma |
| N-15 | LOW | Aceptación temporal | El `select` previo a `read` debe mantenerse | Cualquier refactor del lector PTY |
| N-16 | LOW | Aceptación temporal | Pagers de prueba con salida < ~2 MiB | Cualquier refactor |
| N-17 | INFO | Aceptación temporal | — | Cualquier refactor |
| N-20 | INFO | Aceptación temporal, ligada a S-03 | Solo llamar con el PGID capturado | Cualquier cambio en la señalización |
| N-21 | INFO | Aceptación temporal | — | — |
| N-22 | INFO | Aceptación temporal | — | — |
| N-27 | INFO | Aceptar con S-01 | — | Igual que S-01 |
| N-28 | INFO | Aceptación temporal | — | — |

### Grupo B. Detector arquitectónico

| ID | Severidad | Decisión propuesta | Condición | Punto de revisión |
|---|---|---|---|---|
| N-03 | LOW | Aceptación temporal con seguimiento | Añadir una prueba propia cuando se toque el detector | Con N-04 |
| N-04 | LOW | **Resolver antes de incorporar adaptadores reales** en `gitprocess` y `gitstore` (o justificar otro alcance) | Fijar listas de imports permitidos para los adaptadores | Antes de T016 y T020 |

### Grupo C. Puertos arquitectónicos (no son deuda informativa)

| ID | Severidad | Decisión propuesta | Condición | Punto de revisión |
|---|---|---|---|---|
| F-03 | MINOR | Alternativa 2 (sección E) | Decisión tomada antes de T016 | T008, T016–T019 |
| F-04 | MINOR | Alternativa 3 (sección F) | Decisión tomada antes de T021 | T008, T021–T023 |

Tareas bloqueadas hasta decidir: T016, T017, T018 y T019 (por F-03); T021, T022 y T023 (por F-04).

### Grupo D. Observaciones restantes

| ID | Severidad | Decisión propuesta | Condición | Punto de revisión |
|---|---|---|---|---|
| F-07 | OBS | Revisión en Fase 12, cuando se cablee la composición | Que el servicio "no disponible" sea explícito, no nil | Fase 12 |
| F-08 | OBS | Revisión en Fase 12 | Test de que `ai-dev git …` sigue sin reconocerse | Fase 12 |
| F-09 | OBS | Aceptación temporal | — | Cuando se amplíen los fixtures |
| F-10 | OBS | Resolver con la decisión de versionamiento (sección H) | — | Adopción de la línea base |

### Pendiente de trazabilidad

N-24 nunca existió (el Paso 84 saltó de N-23 a N-25), y no hay informe del Paso 72.

## E. Decisión propuesta F-03 (alternativa 2: descriptor cerrado)

**Estado actual** (`ports.go`): `GitInvocation` tiene `Executable`, `Args`, `Directory`, `Environment`, `Stdin` y límites. `plan.md:88` dice que el runner recibe una operación tipada, un `GitToolIdentity` congelado, argv exacto, directorio físico, stdin, límites y un perfil de entorno.

**Recomendación técnica:** adoptar el descriptor cerrado. Hoy el puerto solo lo usan fakes y un struct de composición, así que cambiarlo cuesta poco. Después de T016–T019 costaría reescribir pruebas y adaptador.

### Contrato conceptual (no implementar)

- **Entrada al runner:** una operación Git enumerada (conjunto cerrado, igual a la matriz P-04), un `GitToolIdentity` congelado, el directorio físico explícito, un perfil de entorno nombrado (`inspect`, `plan`, `apply`, `object`), stdin acotado y límites de salida.
- **Prohibido en el contrato:** `Executable` libre, `Environment []string` arbitrario, argumentos Git libres y cualquier ejecución mediante shell.
- **Quién construye qué:** solo el runner construye argv y entorno (T018/T019), a partir de la operación y el perfil.
- **Read-only vs mutante:** cada operación del enum declara su clase (lectura, plumbing de objetos, mutación).
- **Un descriptor no basta para mutar:** las operaciones mutantes solo pueden ser ejecutadas si el runner recibe un valor que solo el futuro Safety Kernel puede emitir, tras verificar la autorización humana y registrar la auditoría antes de ejecutar. Poseer el descriptor no equivale a poseer la autorización.
- **Safety Kernel:** emite esa credencial; el runner la valida. Quedan fuera de este contrato los detalles de CAS y auditoría (T022/T023).

### Análisis solicitado

1. **T008:** compatible y dependiente. `GitToolIdentity` y los tipos de operación nacen allí.
2. **T016–T019:** encaja con las pruebas de argv exacto, lista blanca de entorno y matriz por filas.
3. **Impacto en `ports.go`:** reemplaza `GitInvocation`; el resto no cambia.
4. **Fakes y tests:** cambian `fakes_test.go` y las aserciones de interfaz. Los tests de PTY no se ven afectados.
5. **Riesgos de acoplamiento:** el puerto queda atado al enum de P-04. Se mitiga manteniendo el enum en `gitmanager` y la construcción de argv en `gitprocess`.
6. **Operación read-only:** operación enumerada con clase "lectura"; el perfil `inspect` o `plan` la ejecuta sin credencial.
7. **Operación mutante:** igual, pero exige la credencial emitida por el Safety Kernel.
8. **Impedir que el descriptor baste:** el tipo de la credencial no puede construirse fuera del Safety Kernel.
9. **Safety Kernel futuro:** emite la credencial, que el runner valida antes de ejecutar.
10. **Prohibido antes de habilitar un runner real:** el runner no debe aceptar ejecutables, entornos ni argumentos aportados por el llamador, ni mutaciones sin credencial.

### Secuencia futura propuesta

1. Registrar la decisión humana (acta).
2. T008: definir `GitToolIdentity`, el enum de operaciones y los perfiles.
3. Actualizar el puerto y los fakes, antes de escribir T016.
4. T016–T019 contra el contrato nuevo.

### Criterios de aceptación

- Ningún campo de `Executable` ni de entorno en el puerto.
- Las pruebas de T016 fallan si un llamador intenta aportar entorno o argv libre.
- Cada operación mutante exige la credencial; sin ella el runner rechaza.

**Decisión humana pendiente.**

## F. Decisión propuesta F-04 (alternativa 3: descriptores tipados y capacidades)

**Estado actual** (`ports.go`):

- `GitStore.Execute(GitStoreRequest{Operation, Key, Version, Payload})`.
- `Filesystem.Execute(FilesystemRequest{Operation, Path, Data, Mode, Limit})`.
- `Reporter.Report(ReportRequest{Kind, Format, Payload})`.

`plan.md:101` y `:239` piden resolución basada en descriptores e identidad de dispositivo/inodo; `plan.md:77` asigna el renderizado a `internal/report`.

**Recomendación técnica:** rediseñar en el momento en que nacen los tipos (T008), antes de T021. Si se prefiere ir por pasos, la alternativa 2 es un intermedio válido, con el riesgo de quedar a medio camino.

### Contrato conceptual (no implementar)

- **Persistencia:** separar la lectura de la autorización (consulta de versión) de las operaciones de mutación (publicación atómica privada, reemplazo con compare-and-swap por versión y propietario, registro de auditoría append-only). Cada una es un método tipado; no existe un sobre `Operation + Payload`.
- **Filesystem de solo lectura:** resolver la raíz física una vez y devolver un manejador con identidad (dispositivo/inodo como cadenas decimales sin signo). Las lecturas acotadas reciben ese manejador más componentes relativos, no una ruta libre. Se rechazan `..`, rutas absolutas y enlaces simbólicos que escapen.
- **Filesystem de mutación:** operaciones enumeradas y acotadas, que solo existen si el llamador porta una credencial de mutación autorizada.
- **Reporter:** recibe valores de dominio tipados; `internal/report` los renderiza.
- **Posesión de descriptor ≠ autorización:** el descriptor de lectura no permite mutar; la mutación exige la credencial.

### Análisis solicitado

1. **T008:** nacen aquí los tipos de resultado e identidad; F-04 depende de ellos.
2. **T021–T023:** definen el filesystem real, los registros y la auditoría; se implementan contra el contrato nuevo.
3. **Compatibilidad con `plan.md`:** alta (descriptores, identidad, CAS, fsync).
4. **Safety Kernel:** consume las credenciales de mutación y emite las de operación.
5. **Planificación read-only vs ejecución autorizada:** la planificación usa solo los puertos de lectura.
6. **Evitar que un puerto genérico eluda la autorización:** eliminar el método genérico `Execute`.
7. **Operaciones tipadas:** resolución de raíz, lectura acotada, publicación privada, reemplazo con CAS, anexión de auditoría, bloqueo de aplicación.
8. **Capacidades explícitas:** lectura vs mutación, y mutación con credencial.
9. **Descriptor ≠ autorización:** ver contrato.
10. **Duplicación del puerto Filesystem:** T021 dice que `internal/gitmanager/filesystem.go` define el puerto, y `ports.go` ya declara `Filesystem`. Regla propuesta: **una sola definición**. Al llegar a T021, retirar `Filesystem` de `ports.go` o reducirlo a un alias, nunca dos interfaces paralelas.

### Secuencia futura propuesta

1. Registrar la decisión humana.
2. T008: tipos de identidad y resultados.
3. Reconciliar `ports.go` y `filesystem.go`, y reemplazar `GitStore`, `Filesystem` y `Reporter` en un solo paso.
4. T021–T023 contra el contrato nuevo.

### Criterios de aceptación

- No queda ningún método genérico `Execute` con cadena de operación.
- Una única definición de `Filesystem`.
- Los tests de T021–T023 pasan sin rutas libres.
- Sin credencial, ninguna mutación es posible.

**Decisión humana pendiente.**

## G. Desviaciones T003 y T004

### T003

- **Hecho:** el texto de T003 dice que solo `gitprocess` importa `os/exec`. La implementación permite además `testhelpers_test.go` de `gitmanager`, por nombre exacto y solo `os/exec`. Cualquier otra API de procesos en ese archivo sigue siendo marcada. El código productivo queda protegido: M2, M6, M8, M9, M10 y M11 se detectan, y los guards sobre lo productivo siguen vigentes.
- **Decisión propuesta:** aceptar la excepción **solo para pruebas** y registrarla de forma explícita.
- **Justificación:** el harness de sentinelas necesita ejecutar Git real para demostrar que los helpers hostiles no se disparan; sin ello F-02 no se podría probar.
- **Riesgo:** la excepción es por nombre de archivo. Un `testhelpers_test.go` modificado podría usar `os/exec` sin otro control (parte de N-04).
- **Consecuencia sobre `tasks.md`:** modificarlo es conveniente para la trazabilidad, no necesario para el funcionamiento. Si se hace, sigue el precedente de Feature 006 (nota de sesión fechada), y cambia el hash normativo.
- **Estado:** PENDIENTE DE APROBACIÓN.

### T004

- **Hecho:** `sentinel_pty_posix_test.go` y `sentinel_pty_other_test.go` no figuran en T004, pero se crearon con autorización expresa (Paso 75).
- **Decisión propuesta:** aceptar como parte legítima del harness, en la misma ubicación.
- **Justificación:** Git solo activa el pager si la salida es una terminal; el helper PTY es indispensable para disparar esa sentinela. Su ubicación junto a `testhelpers_test.go` es adecuada.
- **Riesgo:** el inventario normativo de T004 queda incompleto.
- **Consecuencia:** puede aceptarse sin modificar `tasks.md`, aunque conviene actualizar el inventario junto con T003.
- **Estado:** PENDIENTE DE APROBACIÓN.

## H. Política de versionamiento

Propuesta, no ejecutada.

### Grupo 1: especificación (11 archivos)

**Incorporar los 11 al repositorio**, con contenidos y hashes preservados. No hay razón técnica para excluir ninguno. Es lo que hicieron las Features 001–006. No se modifican las casillas de `tasks.md`.

### Grupo 2: código (13 archivos)

**Incorporar los 13**, sin cambios y sin habilitar infraestructura productiva. No mezclar rediseños de F-03 o F-04 en este commit.

### Grupo 3: informes (14 históricos, más `STEP-87-CLOSURE-PROPOSAL.md` y este informe)

| Alternativa | Reproducibilidad | Trazabilidad | Tamaño y mantenimiento | Riesgo de pérdida | Compatibilidad con la estructura | Necesidad de mover | Efecto en hashes y referencias |
|---|---|---|---|---|---|---|---|
| A. Todos en la raíz | Alta | Alta | ~165 KB, crece con cada paso | Bajo | Mala: la raíz contiene hoy solo código y documentos de proyecto; ningún informe `STEP-*` se versionó antes | No | Ninguno |
| B. Ubicación histórica dedicada dentro de la feature (p. ej. un subdirectorio de auditoría de `specs/008-git-management/`) | Alta | Alta | Igual | Bajo | Buena: sigue el diseño de `specs/<feature>/` | **Sí**, mover los archivos y crear el directorio | Los hashes de contenido no cambian; cambian las rutas. Los informes se citan por nombre, así que siguen siendo localizables |
| C. Fuera de Git, con respaldo externo | Baja | Media | Ninguno en Git | Medio: depende del respaldo | Neutral | No | Ninguno |
| D. Manifiesto en Git y informes completos fuera | Media | Media | Muy bajo en Git | Medio: el manifiesto prueba, no conserva | Buena | No | Ninguno |

**Recomendación: B**, con un manifiesto SHA-256 de los archivos versionado junto a ellos. A es el fallback sin mover nada. No se recomienda C ni D, porque la regla de seguridad exige no perder evidencia de auditoría.

### Revisión de contenido sensible, ya realizada

- No hay credenciales, tokens, claves ni correos.
- 8 informes contienen la ruta absoluta `/Users/german/...`.
- `STEP-76A-AUDIT-REPORT.md` cita una ruta local de sesión (`~/.codex/sessions/.../rollout-...jsonl`).
- Esas referencias no son secretos, pero identifican el entorno local. Decisión humana: conservarlas (recomendado, para no alterar hashes ni la evidencia) o redactarlas (cambia el contenido y los hashes).

Los informes futuros siguen esta misma política.

## I. Procedimiento futuro de adopción de línea base

Propuesto para una autorización posterior. No ejecutable en este paso.

1. Confirmación humana de las decisiones de la matriz (sección J).
2. Preflight: rama, HEAD, índice, hashes de referencia.
3. Inventario definitivo de archivos sin seguimiento, incluidos los informes nuevos.
4. Verificación de hashes de todos los archivos que se versionarán.
5. Clasificación: especificación, código, informes.
6. Revisión de contenido sensible de los informes.
7. Selección explícita de rutas, sin comodines.
8. Staging limitado a esas rutas autorizadas, en grupos separados.
9. Verificación del diff staged contra el inventario.
10. Pruebas finales: build, vet, `-race`, suite completa.
11. Commits de línea base **solo con autorización separada**, uno por grupo, siguiendo la convención del repositorio: `docs:` para especificación, y un tipo adecuado para el código de Fase 1; informes en un commit propio.
12. Registro del identificador de cada commit.
13. Verificación posterior: HEAD, árbol limpio, hashes iguales a los de antes del commit.
14. Registro formal del cierre en un paso aparte, siguiendo el precedente de Feature 006: una nota de sesión fechada en los normativos. Esto cambiará el hash de `tasks.md` y de otros normativos, así que hay que declarar de antemano la nueva referencia de hashes. Marcar T001–T006 también es una modificación de `tasks.md` que requiere autorización propia.
15. Detención antes de iniciar la Fase 2.

**Nota:** no es `git flow feature finish`. La feature completa sigue abierta.

### Cuatro estados distintos, ninguno intercambiable

- **Línea base verificada:** ya cumplida (contenido y hashes).
- **Línea base versionada:** pendiente (pasos 8–13).
- **Cierre formal de la fase:** pendiente (paso 14).
- **Autorización de la Fase 2:** pendiente, y es una decisión separada.

## J. Matriz de decisiones humanas

Todas **PENDIENTE DE APROBACIÓN HUMANA.**

| Decisión | Propuesta | Alternativas | Impacto | Aprobación |
|---|---|---|---|---|
| S-01 | Aceptar el contrato | Rechazar; aceptar con modificaciones | Garantía por orden de llamadas, acotada a pruebas | Pendiente |
| S-02 | Aceptar | Ídem | Supuesto POSIX, ejecución nativa solo en macOS | Pendiente |
| S-03 | Aceptar | Ídem | Aislamiento de señales | Pendiente |
| S-04 | Aceptar | Ídem | Límites temporales | Pendiente |
| S-05 | Aceptar | Ídem | Fase 1 inerte | Pendiente |
| Deuda informativa (Grupo A y D salvo F-10) | Aceptación temporal | Corregir antes de la Fase 2 | Sin riesgo de seguridad conocido | Pendiente |
| N-03 | Aceptación temporal con seguimiento | Corregir ahora | Detector sin prueba propia de la parte indirecta | Pendiente |
| N-04 | Resolver antes de adaptadores reales | Aceptar con alcance distinto | Evita adaptadores sin lista de imports | Pendiente |
| F-03 | Alternativa 2, antes de T016 | Alternativa 1 o 3 | Desbloquea T016–T019 | Pendiente |
| F-04 | Alternativa 3, con T008 y antes de T021 | Alternativa 1 o 2 | Desbloquea T021–T023 | Pendiente |
| T003 | Aceptar la excepción solo para pruebas y registrarla | Corregir la implementación; modificar `tasks.md` | Trazabilidad | Pendiente |
| T004 | Aceptar los helpers PTY como parte del harness | Mover o renombrar; actualizar el inventario | Trazabilidad | Pendiente |
| Versionar especificación | Un commit, 11 archivos | Dividir en varios | Línea base reproducible | Pendiente |
| Versionar código | Un commit, 13 archivos | Combinar con la especificación | Línea base reproducible | Pendiente |
| Preservar informes | Alternativa B más manifiesto | A, C o D | Evidencia de auditoría | Pendiente |
| Procedimiento de adopción | Secuencia de la sección I | Otro orden | Reproducibilidad | Pendiente |
| Registro de cierre formal | Nota de sesión fechada, siguiendo Feature 006 | Otro formato | Cambia hashes normativos | Pendiente |
| Inicio de Fase 2 | Decisión separada, después del cierre | — | — | Pendiente |

## K. Criterios G-01 a G-10

| Criterio | Estado |
|---|---|
| G-01 Contratos S-01 a S-05 aceptados | Técnicamente preparado, **pendiente de aprobación** |
| G-02 Deuda clasificada y aceptada con condiciones | Técnicamente preparado, **pendiente de aprobación** |
| G-03 F-03 resuelta a nivel de decisión, con bloqueo antes de T016 | Técnicamente preparado, **pendiente de aprobación** |
| G-04 F-04 resuelta a nivel de decisión, con dependencias T008/T021/T023 | Técnicamente preparado, **pendiente de aprobación** |
| G-05 Desviaciones T003/T004 | Técnicamente preparado, **pendiente de aprobación** |
| G-06 Política de versionamiento | Técnicamente preparado, **pendiente de aprobación** |
| G-07 Procedimiento de línea base | Técnicamente preparado, **pendiente de aprobación** |
| G-08 Evidencia técnica y hashes preservados | **Cumplido** (hashes e informes verificados) |
| G-09 Ninguna mutación no autorizada | **Cumplido** (sin operaciones Git mutantes; manifiesto idéntico) |
| G-10 Fase 2 no iniciada | **Cumplido** |

Ningún criterio está incumplido. No se marca como cumplido ninguno que dependa de aprobación humana.

## L. Riesgos residuales

- Mientras no se versione, el trabajo de Fase 1 no tiene copia en Git.
- S-01 y S-02 dependen de supuestos de sistema operativo; la ejecución nativa se probó solo en macOS/arm64.
- Si F-03 y F-04 no se deciden antes de T016, los adaptadores consolidarían los puertos genéricos.
- La excepción de T003 es por nombre de archivo.
- Registrar el cierre cambia el hash de `tasks.md` y otros normativos.
- Los informes contienen rutas locales; redactarlas cambiaría sus hashes.
- Trazabilidad incompleta de N-24 y del Paso 72.

## M. Recomendación final

Someter a decisión humana, en este orden:

1. Contratos S-01 a S-05 y política de deuda (secciones C y D).
2. F-03 y F-04 (secciones E y F), porque bloquean T016 y T021.
3. T003 y T004 (sección G).
4. Política de versionamiento y de informes (sección H).
5. Procedimiento de adopción y de registro del cierre (sección I).

Después, con autorización expresa y separada, ejecutar la adopción de la línea base. Solo entonces corresponde decidir sobre el cierre formal y la Fase 2.

**Veredicto: READY FOR HUMAN DECISION.** No equivale al cierre de la Fase 1.

## N. Borrador de acta de decisiones humanas

> **BORRADOR: NO APROBADO. Requiere revisión y decisión del responsable del proyecto.**
>
> Fecha: ___
> Responsable: ___
>
> Habiendo revisado los informes de los Pasos 73 a 88, decido:
>
> 1. **Contratos.** Acepto / no acepto los contratos S-01, S-02, S-03, S-04 y S-05 [con las siguientes modificaciones: ___].
> 2. **Deuda.** Acepto temporalmente los hallazgos [lista]. Mantengo como condición N-03 [y N-04 antes de ___].
> 3. **F-03.** Adopto la alternativa [1 / 2 / 3], a decidir en T008 y aplicar antes de T016.
> 4. **F-04.** Adopto la alternativa [1 / 2 / 3], a decidir en T008 y aplicar antes de T021.
> 5. **T003 y T004.** Acepto / no acepto las desviaciones, y autorizo / no autorizo registrarlas en [lugar].
> 6. **Versionamiento.** Apruebo versionar la especificación (11 archivos) y el código (13 archivos) en commits separados. Para los informes, apruebo la alternativa [A / B / C / D], conservando / redactando las rutas locales.
> 7. **Procedimiento.** Apruebo / modifico la secuencia de adopción de línea base. La ejecución requiere una autorización separada.
> 8. **Cierre.** No declaro cerrada la Fase 1 hasta [___].
> 9. **Fase 2.** No autorizo el inicio de la Fase 2 hasta [___].
>
> Firma: ___

---

Este documento no acepta contratos ni deuda, no elige F-03 ni F-04, no adopta la línea base, no cierra la Fase 1 y no autoriza la Fase 2, la modificación de `tasks.md` ni la creación de commits.
