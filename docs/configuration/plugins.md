# Plugins configuration

`<config dir>/plugins/config.json`:

```json
{
  "obsidian": {
    "vaults": [
      "~/Library/Mobile Documents/iCloud~md~obsidian/Documents/Notes"
    ]
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `obsidian.vaults` | The Obsidian vaults `dgs plugins` checks and installs into when a run names none with `--vault`. Each is a vault's own folder, the one holding `.obsidian/`. | empty — every run must name a vault with `--vault` |

Paths follow the same rules as [`cred`](cred.md#paths): a leading `~` is the
home directory, `$NAME` and `${NAME}` are environment variables, and the
result must be absolute.

What `dgs plugins` does with them is in [`apps/plugins/`](../apps/plugins/index.md).
