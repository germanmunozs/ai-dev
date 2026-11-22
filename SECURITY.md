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
- a vulnerability in a third-party dependency (Bubble Tea, Lip Gloss and their modules) that is reachable from ai-dev.

Out of scope are the [known limitations](README.md#known-limitations) and the accepted residual risks documented in the specifications (for example REC-06, REC-07 and DUR-01 in [`specs/008-git-management`](specs/008-git-management/)).

## Third-party dependencies

Dependencies are pinned by version and checksum in `go.mod` and `go.sum`. The newest version published at least 14 days earlier is chosen, and govulncheck runs on every CI run. The full policy is in the [README](README.md#third-party-dependencies).
