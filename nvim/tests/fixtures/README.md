# Plugin test fixtures

Each `<name>.json` is the standard output of the ai-dev binary built from this branch for one entry of `index.json`, which records the project, the arguments and the exit code. The projects are in `projects.json`: a map from each file's path to its content. The Lua tests build the same projects, so the files the plugin lists match the outputs it reads.

## How they are produced

`cmd/ai-dev/nvimfixtures_test.go` (`TestPluginFixturesMatchTheBinary`) builds the binary, creates each project of `projects.json` in a temporary directory, and runs each entry of `index.json` there, as `ai-dev <args>`, with this environment and nothing else:

- `PATH` holds one private directory with fake `git`, `specify`, `claude` and `gemini`, each `#!/bin/sh` followed by `exit 0`. So `git rev-parse` prints nothing, and of the catalog agents only Claude Code and Gemini CLI are available;
- `HOME`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME` and `XDG_STATE_HOME` are private temporary directories.

The test fails when an output or exit code differs from the saved one, so the fixtures cannot drift from the real outputs, and they are the same on macOS, Linux and CI. To regenerate them after a deliberate change of an output:

```sh
AI_DEV_UPDATE_FIXTURES=1 go test -count=1 -run TestPluginFixturesMatchTheBinary ./cmd/ai-dev
```

`doctor.json` holds no system version or path, so it is compared exactly like the others.

## What they cover

| Project | Covers |
|---|---|
| `views` | every Workflow State (`01-draft` has no state record), an invalid record (`08-invalid`), missing artifacts (`09-inconsistent`), a feature name holding an escape sequence (`10-…red`), a feature that no longer exists (`99-removed`, `feature_unknown`), a configuration whose implementation agent is unavailable |
| `invalid-config` | an invalid agent configuration, in `workflow guide` and `setup show` |
| `no-specs` | a project without `specs/`; `doctor` with failing checks |

The version report is not a fixture: it depends on the build, so the tests' fake ai-dev prints a fixed `ai-dev.version/v1` report.
