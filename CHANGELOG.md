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
- Releases con archivos para macOS y Linux (amd64 y arm64), `SHA256SUMS` y una attestation de procedencia SLSA firmada para cada archivo.

[Unreleased]: https://github.com/germanmunozs/ai-dev/commits/dev
