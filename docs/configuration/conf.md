# Conf configuration

`<config dir>/conf/config.json`:

```json
{
  "root": "~/confgen",
  "secrets": "~/confgen-secrets",
  "services": "~/confgen-deploy",
  "install_root": "/srv/example",
  "label_prefix": "example",
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
| `install_root` | Where a Linux host bundle is installed when its instance writes no `deploy.dir`: `<install_root>/<service>`. A macOS bundle with no `deploy.dir` stays where it was unpacked. The same as `rhumb deploy build --install-root`. | empty — `/srv/rhumb` |
| `label_prefix` | The start of the name the service manager knows an instance by, `<label_prefix>.<node>.<instance>`: the systemd unit and the launchd label and plist. Letters, digits, `-` and `_`. The same as `rhumb deploy build --label-prefix`. | empty — `rhumb` |
| `export.dir` | Where the destination form opens, for a folder or an archive. | empty — the home directory |

Paths follow the same rules as [`cred`](cred.md#paths): a leading `~` is the
home directory, `$NAME` and `${NAME}` are environment variables, and the
result must be absolute.

What `dgs conf export` does with these is in
[`apps/conf/export.md`](https://github.com/d0u9/rhumb/blob/master/docs/export.md).

`services`, `install_root` and `label_prefix` are read by every way to a
bundle: `dgs conf bundle <export-dir>`, which is `rhumb deploy build`;
`dgs conf export --bundle`; and the inspect page's Bundle format. On the first
two, `--services`, `--install-root` and `--label-prefix` override them for one
run; `--services` on `dgs conf inspect` does too.

`dgs conf bundle-gc` finds launchd entries under `label_prefix`, or its own
`--label-prefix`: run it with the prefix the bundles were built with. A bundle
of the same instance built under another prefix is rebuilt only when replacing
was confirmed — `--overwrite`, or Replace on the inspect page — and a macOS
bundle never is: it is installed where it is, so `ctl uninstall` it first.
