# Conf configuration

```json
{
  "conf": {
    "root": "~/confgen",
    "secrets": "~/confgen-secrets",
    "export": {
      "dir": "~/Exports"
    }
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `conf.root` | The directory holding one subdirectory per service. | empty — the page opens with no root and asks for one |
| `conf.secrets` | The directory a manifest's `secrets` file is named relative to. | empty — a service naming a secrets file refuses to render |
| `conf.export.dir` | Where the destination form opens, for a folder or an archive. | empty — the home directory |

Paths follow the same rules as [`cred`](cred.md#paths): a leading `~` is the
home directory, `$NAME` and `${NAME}` are environment variables, and the
result must be absolute.

What `dgs conf export` does with these is in
[`apps/conf/export.md`](../apps/conf/export.md).
