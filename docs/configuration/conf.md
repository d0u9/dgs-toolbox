# Conf configuration

`<config dir>/conf/config.json`:

```json
{
  "root": "~/confgen",
  "secrets": "~/confgen-secrets",
  "services": "~/confgen-deploy",
  "export": {
    "dir": "~/Exports"
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `root` | The directory holding `services/` and the inventory files. | empty — the TUI page opens with no root and asks for one |
| `secrets` | The root of the secrets tree, one file per credential. | empty — an instance needing a secret refuses to render |
| `services` | A directory of deploy service definitions, one `<service>.yaml` each, consulted before the built-in ones when a bundle is built. The same as `rhumb deploy build --services`. | empty — only the built-in definitions |
| `export.dir` | Where the destination form opens, for a folder or an archive. | empty — the home directory |

Paths follow the same rules as [`cred`](cred.md#paths): a leading `~` is the
home directory, `$NAME` and `${NAME}` are environment variables, and the
result must be absolute.

What `dgs conf export` does with these is in
[`apps/conf/export.md`](https://github.com/d0u9/rhumb/blob/master/docs/export.md).

`services` is read by every way to a bundle: `dgs conf bundle <export-dir>`,
which is `rhumb deploy build`; `dgs conf export --bundle`; and the inspect
page's Bundle format. `--services` on the first two, and on `dgs conf inspect`,
overrides it for one run.
