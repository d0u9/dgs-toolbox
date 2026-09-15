# Credentials configuration

`dgs cred` does not read `dgs-config.json`. Its settings are in
`credentials.json`, in the same folder: `$XDG_CONFIG_HOME/dgs-toolbox/`, or
`~/.config/dgs-toolbox/` when that variable is unset.

```json
{
  "identities": ["~/.age", "~/.ssh"],
  "recipients": "$DOT_CONF_DIR/recipients"
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `identities` | Directories searched recursively for private keys on this machine. Files are recognised by content; files over 128 KiB are skipped. A directory that does not exist is reported and skipped, so one file can be shared by machines that do not all have every directory. | empty — no identities |
| `recipients` | The folder holding `hosts/` and `groups/`. | empty — no recipients |

## Paths

In every path, a leading `~` is the home directory, and `$NAME` or `${NAME}`
is replaced by that environment variable. A variable that is not set is an
error naming it, rather than an empty string: `$DOT_CONF_DIR/recipients` with
the variable unset would otherwise quietly become `/recipients`. After
expansion a path must be absolute; relative paths are refused.

Unknown keys are an error. A missing `credentials.json` is not: `dgs cred`
opens with no identities and no recipients, and says where the file is looked
for.

What is inside the recipient folder, and how identities are recognised, is in
[`apps/cred/keys.md`](../apps/cred/keys.md).
