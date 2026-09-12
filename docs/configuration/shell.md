# Shell configuration

What the shell itself reads: where the rest of the configuration lives, and
what the top bar shows. The design behind it is in [`tui.md`](../tui.md).

```json
{
  "config_dir": "",
  "tui": {
    "top_bar": {
      "disk": true,
      "network": true,
      "cpu": true,
      "time": true
    }
  }
}
```

## The configuration file

One JSON file, found by `--config` / `-c`, then `DGS_TOOLBOX_CONFIG`, then
`<user configuration directory>/dgs/config.json`. `--config` deliberately
bypasses the environment variable, so a run can point at a whole configuration
without changing the shell it was launched from.

A missing file leaves every setting at its default. An unknown key is rejected:
the file is refused with the name of the key rather than applied in part.

`dgs --export-config` writes `dgs-config.json` into the working directory and
prints its absolute path. An optional positional path writes elsewhere — an
existing directory receives `dgs-config.json`, any other path is used exactly.
It never overwrites an existing file. The result is an editable template: move
it to the global location, or point `DGS_TOOLBOX_CONFIG` at it.

## `config_dir`

Settings that are values live in the configuration file. Everything a command
reads *from disk* — Recipes, templates, whatever a later command needs — lives
under this directory. Empty means the directory the configuration file was
loaded from, so `--config` selects a whole configuration and not only one file
of it.

### The configuration directory

dgs is a toolbox, so the directory is laid out **by command**, and a command
asks for its own corner rather than for a path of its own in the configuration
file:

```text
<config_dir>/
  capture/
    recipes/
    templates/
  photo/
    …
```

A command's directory may still be moved somewhere else by naming it — see
`capture.recipes` and `capture.templates` in [Capture](capture.md#recipes-and-templates).

## The top bar

Each key removes one fixed-width cell and its separator when set to `false`.
All four default to `true`; omitting a key preserves that default. The
breadcrumb is structural and cannot be disabled.

| Key | What it shows |
| --- | --- |
| `tui.top_bar.disk` | disk read and write rates |
| `tui.top_bar.network` | network up and down rates |
| `tui.top_bar.cpu` | CPU utilization |
| `tui.top_bar.time` | the local clock |

With `disk`, `network` and `cpu` all `false`, the shell does not start the
system-counter sampler at all.
