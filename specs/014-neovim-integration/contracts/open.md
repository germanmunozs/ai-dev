# Contract: `ai-dev open`

## Usage

```text
Usage: ai-dev open [--format text|json] [--line N] [--column N] -- <path>
```

- `<path>`: a file of the project, relative to the current directory. `--` is required (owner decision, 2026-11-24): everything after it is the path, so a name that starts with `-` is a path, and a path without `--` before it is a usage failure.
- `--line N`, `--column N`: integers ≥ 1. The default for each is 1.
- Help: `Changes: nothing in the project; opens the file in Neovim`.

## Outcomes and exit codes

| Outcome | Exit | When |
|---|---|---|
| `opened` | 0 | the parent instance opened the file, or the foreground Neovim exited 0 |
| `path_refused` | 1 | `absolute_path`, `not_found`, `outside_project`, `not_regular_file` |
| `editor_refused` | 1 | `$NVIM` set but refused: `not_local_socket`, `not_socket`, `not_owned`, `no_answer`, `not_neovim`, `version_unsupported`, `editor_error` |
| `editor_unavailable` | 1 | no `$NVIM` and `no_terminal`, `nvim_not_found` or `platform_unsupported` |
| `editor_failed` | 1 | the foreground Neovim exited non-zero |
| usage failure | 2 | missing `--`, missing path, extra arguments, a line or column that is not an integer ≥ 1, an unknown option; the usage line goes to standard error |

There is no fallback: when `$NVIM` is set and refused, nothing else is tried.

## Text report (standard output for 0; standard error otherwise)

```text
Opened src/main.go at line 12, column 1 in the parent Neovim.
```

```text
Refused: src/../../etc/passwd is outside the project (outside_project).
```

```text
Refused: $NVIM does not name a local socket owned by you (not_owned). Nothing was opened.
```

Paths and Neovim messages are sanitized: control and escape sequences are shown escaped, as in the existing text reports. A foreground run prints nothing after a successful exit, so the terminal shows Neovim's own exit.

## JSON report (`--format json`, standard output)

```json
{"schema_version":"ai-dev.open/v1","outcome":"opened","target":"parent",
 "path":"src/main.go","line":12,"column":1}
```

```json
{"schema_version":"ai-dev.open/v1","outcome":"path_refused","reason":"outside_project",
 "path":"src/../../etc/passwd"}
```

- `target` is `parent` or `foreground`, present only for `opened` and `editor_failed`.
- For a foreground run, the report is printed after Neovim exits: `{"schema_version":"ai-dev.open/v1","outcome":"opened","target":"foreground","path":…,"line":…,"column":…}` for exit 0, and `editor_failed` with `"target":"foreground"` and `"exit_code":N` otherwise. The text format stays silent after a successful foreground run.
- `reason` is present for every refusal.
- `message` (sanitized) is present only for `editor_error`.
- No absolute path, no environment value, and not the value of `$NVIM` ever appear (F014-FR-025). An `absolute_path` refusal does not echo the path typed either: `path` is `""`, and the text report says "the path".

## Parent-instance protocol (R-02)

1. `CheckEndpoint($NVIM)`, then dial the Unix socket (500 ms).
2. `nvim_get_api_info` must return `[channel, {version = {major, minor, patch, …}, …}]` with a version ≥ 0.11.7.
3. `nvim_exec_lua(openChunk, [abs_path, line, column])`. `openChunk` is a constant in ai-dev; it chooses a non-terminal window (previous, first in the tab, or a new split), runs `nvim_cmd({cmd = "edit" | "split", args = {path}, magic = {file = false, bar = false}}, {})`, sets the cursor clamped to the buffer, and focuses the window.
4. Any error response becomes `editor_error`.

Each call times out after 1 s, and the whole protocol after 2 s.

## Foreground (R-04)

`nvim "+call cursor(<line>, <column>)" -- <abs_path>`, launched through `system.Launcher` with standard input, output and error inherited. The two integers are formatted by ai-dev; nothing else is Ex text.

## Cockpit actions (A3)

"Open spec", "Open plan", "Open tasks" and "Open file…" in a feature's detail, after the existing actions (owner decision I3-A, 2026-11-24).
- "Open spec", "Open plan" and "Open tasks" are available when that file exists as a regular file; otherwise the reason is `no <file> in this feature` (for example `no plan.md in this feature`). They take no input.
- "Open file…" is available when the feature's directory holds at least one regular file; otherwise the reason is `no files in this feature`. It takes one required field, `file`, typed like every cockpit field (F012-FR-009: nothing prefilled). Its hint lists the regular files of `specs/<feature>/`, sorted and sanitized; symbolic links and subdirectories are not listed. The hint shows at most 20 names, followed by `… and N more` when there are more. The value must equal the name of one of those regular files exactly, listed in the hint or not, or the form refuses it with `not a file of this feature`, as it refuses a missing required value, and nothing runs. The cockpit gains no new input kind.
- Without a selected feature, the reason is the cockpit's existing one for feature actions.
- Each runs `ai-dev open -- specs/<feature>/<file>` through the review, the confirmation and the hand-over of F012-FR-009 and F012-FR-010.
