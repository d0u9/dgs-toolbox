# Conf configuration

`<config dir>/conf/config.json`:

```json
{
  "root": "~/confgen",
  "secrets": "~/confgen-secrets",
  "export": {
    "dir": "~/Exports"
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `root` | The directory holding `services/` and the inventory files. | empty — the TUI page opens with no root and asks for one |
| `secrets` | The root of the secrets tree, one file per credential. | empty — an instance needing a secret refuses to render |
| `export.dir` | Where the destination form opens, for a folder or an archive. | empty — the home directory |

Paths follow the same rules as [`cred`](cred.md#paths): a leading `~` is the
home directory, `$NAME` and `${NAME}` are environment variables, and the
result must be absolute.

What `dgs conf export` does with these is in
[`apps/conf/export.md`](https://github.com/d0u9/rhumb/blob/master/docs/export.md).
