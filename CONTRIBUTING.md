# Contributing

**English** · [Español](CONTRIBUTING.es.md)

Thank you for your interest in ai-dev. This document summarizes how the project works. For vulnerabilities, follow [SECURITY.md](SECURITY.md) and do not open a public issue.

## Process

The project follows **Spec Kit** and its [constitution](.specify/memory/constitution.md). Every behavior change starts from a specification:

1. `spec.md`, approved by the maintainer;
2. `plan.md` and, when needed, `contracts/` and `data-model.md`;
3. `tasks.md`, with the tests before the implementation;
4. the implementation, with each task marked `[X]` when done;
5. an independent review before the pull request.

Specifications live in `specs/<NNN>-<name>/`. A small change that does not alter behavior, such as documentation or CI, does not need a new feature.

The public documentation is in English (`README.md`, `CHANGELOG.md`, `CONTRIBUTING.md`, `SECURITY.md`) and Spanish (`*.es.md`). A change in one language is made in the other too, in the same pull request.

An AI agent never records or claims a human approval on a person's behalf (Constitution IX). The maintainer's decisions are recorded with their name and their words, as a declared, unauthenticated assertion.

## Branches and pull requests

The project uses Git Flow:
- `dev` is the integration branch and `main` the release branch;
- working branches start from `dev` with the prefix `feature/` or `bugfix/`;
- pull requests target `dev` and are integrated with a merge commit;
- `dev` and `main` are protected: they require a pull request, the checks `gates (macos-latest)`, `gates (ubuntu-latest)`, `git-minimum` and `static-analysis` to pass, and resolved conversations. Force pushes are not allowed.

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/), for example `feat(cli): add version and help commands`.

## Requirements

- Go, at the version `go.mod` declares;
- Git 2.45 or later;
- macOS or Linux. The tests that apply real changes with Git need a temporary directory on APFS (macOS) or local ext4 (Linux); on tmpfs they are skipped.

## Before opening a pull request

```sh
gofmt -l .                               # must list nothing
go vet ./...
go test ./...
go test -race -timeout 60m ./internal/gitstore ./internal/gitmanager ./internal/app ./cmd/ai-dev
go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
```

The govulncheck and staticcheck versions are the ones CI pins. They are not added to `go.mod`.

CI also runs cross-builds, fuzzing and the real-Git suites against Git 2.45.0 (see [`.github/workflows/ci.yml`](.github/workflows/ci.yml)).

Review criteria:
- a new dependency needs a justification and follows the [dependency policy](README.md#third-party-dependencies); today only the cockpit (`internal/cockpit`) uses third-party libraries;
- every functional requirement has its test;
- existing public APIs and usage texts are not changed unless the specification asks for it;
- fail closed: when in doubt, a typed refusal and never an assumed success;
- no secrets, credentials or personal data in the code, the tests or the logs.

## License

By contributing you agree that your contribution is distributed under the [Apache License 2.0](LICENSE), per its section 5.
