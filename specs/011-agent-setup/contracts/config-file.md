# Contract: `.ai-dev/agents.json`

## Canonical form

```json
{
  "schema_version": "ai-dev.agents/v1",
  "agents": [
    "claude-code",
    "codex-cli"
  ],
  "roles": {
    "specification": "claude-code",
    "planning": "claude-code",
    "implementation": "codex-cli",
    "review": "claude-code",
    "remediation": "codex-cli"
  }
}
```

Two-space indentation and a trailing newline; agents in catalog order; roles in the order shown. ai-dev writes only this form. It reads any formatting of a valid file.

## Validation reasons

Reasons are reported in this order. A file-level reason stops further checks; content reasons are all reported. `<v>` is an untrusted value in 004's shell-inert quoted form.

| Code | When |
|---|---|
| `directory_not_regular` | `.ai-dev` is a symbolic link or not a directory |
| `file_not_regular` | `agents.json` is a symbolic link or not a regular file |
| `cannot_verify_symlinks` | the filesystem view cannot report symbolic links |
| `too_large` | more than 65536 bytes |
| `unreadable` | the file cannot be read |
| `malformed_json` | not one JSON object, or trailing data |
| `duplicate_key <v>` | a key appears twice in the same object |
| `unknown_field <v>` | a top-level field other than the three |
| `missing_field <v>` | `schema_version`, `agents` or `roles` is absent |
| `wrong_type <v>` | a field, an agent or a role value has the wrong JSON type, including `null` |
| `unsupported_schema <v>` | `schema_version` is not `ai-dev.agents/v1` |
| `no_agents` | `agents` is empty |
| `duplicate_agent <v>` | an agent appears twice |
| `unknown_agent <v>` | an agent, in `agents` or `roles`, is not in the catalog |
| `unknown_role <v>` | a key of `roles` is not one of the five |
| `missing_role <v>` | one of the five roles is absent |
| `agent_not_chosen <v>` | a role is assigned to an agent not in `agents`; `<v>` is `role=agent` |

`setup apply` reports the same codes for its options, plus `duplicate_role <v>` when `--role` names a role twice. `duplicate_role` reasons come first, in the order the options were given, followed by the codes above in their table order.

## Read and write rules

- Read: no symbolic link is followed for `.ai-dev` or `agents.json`; read at most 65537 bytes.
- Write: research R-03. ai-dev never stages or commits the file.
