# Changelog

[English](CHANGELOG.md) · **Español**

Todos los cambios relevantes de ai-dev se registran en este archivo.

El formato sigue [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/) y el proyecto usa [Semantic Versioning](https://semver.org/lang/es/). Mientras la versión mayor sea 0, cualquier versión menor puede cambiar la interfaz de la CLI o los formatos JSON.

Este archivo es la traducción de [`CHANGELOG.md`](CHANGELOG.md) y se mantiene en paralelo. Al publicar una versión, la sección `[Unreleased]` de ambos pasa a `[X.Y.Z] - AAAA-MM-DD`. El workflow de release usa la sección de `CHANGELOG.md` como notas del release y falla si no existe.

## [Unreleased]

### Added

- **014 Neovim Integration:**
  - `ai-dev open [--format text|json] [--line N] [--column N] -- <ruta>` abre un archivo del proyecto en una línea en Neovim: en el Neovim padre cuando se ejecuta en una de sus terminales (`$NVIM`), si no en primer plano. Rechaza rutas absolutas, rutas fuera del proyecto y lo que no sea un archivo regular, y un `$NVIM` que no sea un socket local del usuario que responda como Neovim 0.11.7 o posterior, sin alternativa. Su informe JSON es `ai-dev.open/v1`.
  - Las acciones de feature del cockpit añaden "Open spec", "Open plan", "Open tasks" y "Open file…", que ejecutan `ai-dev open` tras la revisión y confirmación de siempre; las acciones existentes no cambian.
  - Un plugin de Neovim en `nvim/`, instalable con lazy.nvim, con un preset opcional de LazyVim bajo `<leader>i`. Muestra features, entorno y agentes a partir del JSON de ai-dev, pregunta a un agente sobre un archivo o una selección con una referencia (ruta, rango de líneas y SHA-256) y el texto solo si se elige, y ejecuta next, approve, abandon y el cockpit en una terminal de Neovim, cada uno tras una vista previa y una confirmación explícita. Necesita Neovim 0.11.7 o posterior y ai-dev v0.3.0 o posterior.

### Changed

- El uso general, la ayuda general y el completado de shell incluyen `open`.
- `scripts/install-dev.sh` marca las versiones de desarrollo como `v0.2.2-dev+<commit>` en lugar de `v0.2.1-dev+<commit>`.

## [0.2.1] - 2026-11-23

### Changed

- `scripts/install-dev.sh` marca las versiones de desarrollo como `v0.2.1-dev+<commit>` en lugar de `v0.1.2-dev+<commit>`.
- Lip Gloss actualizado de v2.0.6 a v2.1.0 (publicado el 2026-11-09, bajo la regla de 14 días).

## [0.2.0] - 2026-11-23

### Added

- **013 Agent Catalog Expansion:** seis agentes de programación más se pueden detectar, configurar para un rol y ejecutar con `ai-dev agent run`: Factory Droid (`factory-droid`), Gemini CLI (`gemini-cli`), Kiro CLI (`kiro-cli`), Mistral Vibe (`mistral-vibe`), OpenCode (`opencode`) y Qwen Code (`qwen-code`). Cada uno cumplió unos criterios de admisión publicados, con evidencia. ai-dev invoca cada uno en su propio modo no interactivo, sin que la instrucción pueda leerse nunca como una opción y sin ninguna opción que relaje sus aprobaciones, permisos o sandbox. El registro de admisión, con cada candidato evaluado, está en `specs/013-agent-catalog/research.md`.

### Changed

- `ai-dev doctor` tiene ocho checks: `claude-code-tool` y `codex-cli-tool` se sustituyen por `coding-agent-tool`, que pasa cuando hay cualquier agente soportado instalado y los nombra (`Coding agent: claude-code, gemini-cli`). Los programas que leen el informe JSON de doctor por identificador de check deben usar el nuevo.
- `ai-dev agents`, `setup show`, el asistente de setup, la ayuda, el autocompletado y el cockpit listan los ocho agentes.
- `scripts/install-dev.sh` marca las versiones de desarrollo como `v0.1.2-dev+<commit>` en lugar de `v0.1.0-dev+<commit>`.

## [0.1.1] - 2026-11-22

### Changed

- La documentación pública (README, CHANGELOG, CONTRIBUTING y SECURITY) está en inglés y en español. El inglés queda en los archivos de siempre y el español pasa a `*.es.md`.

### Fixed

- `scripts/install.sh` fallaba en macOS con un locale UTF-8 (`arch…: unbound variable`): el `sh` de macOS leía el carácter `…` como parte del nombre de la variable.
- Los binarios de un release informaban `modified` en `ai-dev version` porque los archivos ya generados en `dist/` contaban como cambios sin commit; `dist/` queda ignorado.

## [0.1.0] - 2026-11-22

Primera versión.

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
- **010 Command Help and Shell Completion:** `ai-dev <comando> --help`, `-h` y `ai-dev help <comando>` muestran la ayuda de un comando o grupo (uso, argumentos, opciones, qué cambia y códigos de salida); `ai-dev completion bash|zsh` escribe un script de autocompletado estático.
- **011 Agent Detection and Role Configuration:** `ai-dev setup` (asistente), `ai-dev setup show` y `ai-dev setup apply` detectan los agentes del catálogo, eligen con cuáles trabaja el proyecto y asignan uno a cada rol en `.ai-dev/agents.json`; `agent select` y `workflow guide` usan esa asignación e informan su origen (`source`).
- **012 Terminal Cockpit:** `ai-dev tui` abre un cockpit de terminal manejado con el teclado. Muestra el estado de cada feature con el texto de `workflow state` y `workflow guide`, el de `doctor`, `setup show` e `init inspect`, y lanza las acciones existentes (`agent run`, `workflow approve`, `workflow abandon`, `setup`, `init`, `git commit validate`) cediendo la terminal al comando, siempre tras revisar el comando exacto y confirmarlo. Todo el texto no confiable se muestra saneado y la terminal se restaura en cualquier salida.
- Primeras dependencias de terceros: Bubble Tea v2.1.1 y Lip Gloss v2.0.6, fijadas con la regla de los 14 días y revisadas por govulncheck.
- `scripts/install.sh` instala ai-dev desde un release de GitHub sin Go ni Git: verifica `SHA256SUMS` y, si `gh` está instalado, la attestation de procedencia, y solo acepta releases `vX.Y.Z`.
- `scripts/install-dev.sh` compila e instala una versión de desarrollo (`v0.1.0-dev+<commit>`) para pruebas manuales, y crea un proyecto sandbox con agentes falsos.
- Releases con archivos para macOS y Linux (amd64 y arm64), `SHA256SUMS` y una attestation de procedencia SLSA firmada para cada archivo.

[Unreleased]: https://github.com/germanmunozs/ai-dev/compare/v0.2.1...dev
[0.2.1]: https://github.com/germanmunozs/ai-dev/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/germanmunozs/ai-dev/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/germanmunozs/ai-dev/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/germanmunozs/ai-dev/releases/tag/v0.1.0
