# Security policy

**English** · [Español](SECURITY.es.md)

## Supported versions

Vulnerabilities are fixed only in the latest published release (currently the 0.1.x series) and in the `dev` branch, the active development line. While the major version is 0, a fix ships in a new release and is not backported to earlier versions.

## Reporting a vulnerability

**Do not open a public issue or pull request to report a vulnerability.**

Report it privately with the **Report a vulnerability** form on the repository's [Security](https://github.com/germanmunozs/ai-dev/security) tab (GitHub private vulnerability reporting).

If you can, include:
- the affected version (`ai-dev version --format json`), the operating system and the filesystem;
- the minimal steps to reproduce it;
- the impact you observed or expect.

Do not include real credentials, tokens or personal data in the report.

The maintainer will acknowledge the report and coordinate the fix and the disclosure date with you. The project is maintained by one person, so there are no guaranteed response times.

## Scope

Of particular interest are failures of the guarantees described in the [security model](README.md#security-model):
- an automatic signal that grants authority, approves or mutates without an explicit human approval;
- a Git mutation without the confirmed plan and its issued authorization (`MutationPermit`);
- running hooks, a shell or network access from the Git invocations;
- credentials or secrets that appear in the output or in the durable records;
- paths that follow symbolic links, or writes outside the project or the state store;
- text reports that, pasted into a shell, run or expand anything;
- a partial success reported as success, instead of a typed refusal;
- in the cockpit (`ai-dev tui`), an escape sequence, OSC or bidirectional control that reaches the terminal unescaped, or an action that runs without the review and explicit confirmation;
- in Neovim (`ai-dev open` and the plugin in `nvim/`), a file name, selection or instruction that is expanded by Ex or by a shell, read as an option or split; a `$NVIM` endpoint that is not a socket of the user and is still used; text that reaches the screen unescaped; or an agent run or decision without the preview and explicit confirmation;
- in `ai-dev watch`, a write, lock or rename in the project, or a process other than the doctor check's `git rev-parse`; an event that carries a Human Gate outcome, or presents a change without a recorded decision as one; the content of an artifact in an event; or, in text format, a name that reaches the terminal unescaped;
- a vulnerability in a third-party dependency (Bubble Tea, Lip Gloss and their modules) that is reachable from ai-dev.

Out of scope are the [known limitations](README.md#known-limitations) and the accepted residual risks documented in the specifications (for example REC-06, REC-07 and DUR-01 in [`specs/008-git-management`](specs/008-git-management/)).

**`$NVIM` trust model (Feature 014).** `ai-dev open` trusts `$NVIM` only as far as these checks go: an absolute path whose `Lstat` reports a socket owned by the current user, and a peer that answers Neovim's API with version 0.11.7 or later. A process that can change `$NVIM` already runs as the user. The peer's credentials after connecting are not checked, since the standard library has no portable call for them on macOS; replacing the socket between the check and the connection needs write access to the user's own socket directory ([R-03](specs/014-neovim-integration/research.md)). The file is opened with a constant Lua chunk that receives the path as an argument and calls `nvim_cmd` with file-name expansion off, so `%`, `#` and backquotes in a name are never expanded or run ([R-02](specs/014-neovim-integration/research.md)).

## Third-party dependencies

Dependencies are pinned by version and checksum in `go.mod` and `go.sum`. The newest version published at least 14 days earlier is chosen, and govulncheck runs on every CI run. The full policy is in the [README](README.md#third-party-dependencies).
