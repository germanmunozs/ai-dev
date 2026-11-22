# Changelog

Todos los cambios relevantes de ai-dev se registran en este archivo.

El formato sigue [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/) y el proyecto usa [Semantic Versioning](https://semver.org/lang/es/). Mientras la versión mayor sea 0, cualquier versión menor puede cambiar la interfaz de la CLI o los formatos JSON.

Al publicar una versión, la sección `[Unreleased]` pasa a `[X.Y.Z] - AAAA-MM-DD`. El workflow de release usa esa sección como notas del release y falla si no existe.

## [Unreleased]

Primera versión candidata (0.1.0).

### Added

- **001 Environment Doctor:** `ai-dev doctor` comprueba con nueve checks si el entorno (Git, Spec Kit, Claude Code y Codex CLI) y el repositorio (Spec Kit, constitución e integraciones) están listos.
- **002 Coding Agent Execution:** `ai-dev agents` lista los agentes soportados y `ai-dev agent run` ejecuta uno (Claude Code o Codex CLI) con una instrucción explícita, sin elevar permisos ni saltarse su sandbox.
- **003 Agent Role Selection:** `ai-dev agent select` elige el agente adecuado para cada rol del proceso SDD.
- **004 Workflow State:** `ai-dev workflow state` y `ai-dev workflow transition` evalúan el estado del ciclo de vida de una feature y la validez de una transición, sin mutar nada.
- **005 SDD Workflow:** `ai-dev workflow next` recomienda la siguiente actividad SDD (solo lectura).
- **006 Human Gates:** `ai-dev workflow approve` y `ai-dev workflow abandon` registran decisiones humanas explícitas en `workflow-state.json`, con escritura atómica y bloqueo.
- **007 Workflow Orchestration (modo guiado):** `ai-dev workflow guide` muestra el siguiente paso con plantillas de comandos que se pueden pegar en un shell sin riesgo. El modo autónomo queda diferido.
- **008 Git Management:** `ai-dev init` inspecciona, planifica, aplica y reconcilia la inicialización supervisada de un repositorio Git, con autorización explícita, auditoría duradera y recuperación tras crash; `ai-dev git commit validate` valida mensajes Conventional Commits.
- **009 CLI Help and Version:** `ai-dev help`, `ai-dev --help` y `-h` listan los comandos; `ai-dev version` y `--version` informan de la versión, el commit, la versión de Go y la plataforma, en texto o JSON (`ai-dev.version/v1`).
- **011 Agent Detection and Role Configuration:** `ai-dev setup` (asistente), `ai-dev setup show` y `ai-dev setup apply` detectan los agentes del catálogo, eligen con cuáles trabaja el proyecto y asignan uno a cada rol en `.ai-dev/agents.json`; `agent select` y `workflow guide` usan esa asignación e informan su origen (`source`).
- **012 Terminal Cockpit:** `ai-dev tui` abre un cockpit de terminal manejado con el teclado. Muestra el estado de cada feature con el texto de `workflow state` y `workflow guide`, el de `doctor`, `setup show` e `init inspect`, y lanza las acciones existentes (`agent run`, `workflow approve`, `workflow abandon`, `setup`, `init`, `git commit validate`) cediendo la terminal al comando, siempre tras revisar el comando exacto y confirmarlo. Todo el texto no confiable se muestra saneado y la terminal se restaura en cualquier salida.
- Primeras dependencias de terceros: Bubble Tea v2.1.1 y Lip Gloss v2.0.6, fijadas con la regla de los 14 días y revisadas por govulncheck.
- `scripts/install-dev.sh` compila e instala una versión de desarrollo (`v0.1.0-dev+<commit>`) para pruebas manuales, y crea un proyecto sandbox con agentes falsos.
- Releases con archivos para macOS y Linux (amd64 y arm64), `SHA256SUMS` y una attestation de procedencia SLSA firmada para cada archivo.

[Unreleased]: https://github.com/germanmunozs/ai-dev/commits/dev
