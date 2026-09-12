# Examples

Working configurations, one folder per thing you might be setting up. Each is a
complete `dgs-config.json` holding only the keys that folder is about, so a
folder can be read on its own and the keys copied into whatever you already
have.

| Folder | What it sets up |
| --- | --- |
| [`shell/`](shell) | The top bar: which system metrics are shown. |
| [`capture-scan/`](capture-scan) | Capture's root and index filename — the least you need for Scan to list something. |
| [`capture-obsidian/`](capture-obsidian) | Capture writing into an Obsidian vault: the daily note, the running list of places, map links, name mappings, plus Recipes, workflow descriptions and templates. |
| [`photo-import/`](photo-import) | Photo Import's source, destination and state file. |

Every key is documented in [`docs/configuration/`](../docs/configuration), and
a test loads each of these files, so an example that stops being valid fails
the build rather than misleading someone.

## Using one

Configuration is looked for in `$XDG_CONFIG_HOME/dgs-toolbox/dgs-config.json`,
or `~/.config/dgs-toolbox/dgs-config.json` when that variable is unset. Copy a
folder's contents there:

```bash
mkdir -p ~/.config/dgs-toolbox
cp -R examples/capture-obsidian/. ~/.config/dgs-toolbox/
```

Or run against one without installing it, which is also how to try a second
configuration next to your own:

```bash
dgs capture -c examples/capture-obsidian/dgs-config.json
```

Paths in these files are examples — `~/Vaults/personal`, `/Volumes/SD/DCIM` —
and are meant to be replaced. `~` is expanded.

## What sits beside the file

`config_dir` defaults to the folder the configuration was loaded from, so
everything Capture reads from disk sits beside it, laid out by command:

```text
capture-obsidian/
  dgs-config.json
  capture/
    recipes/       # one YAML file per Recipe
    workflows/     # one file per workflow: where it keeps each field
    templates/     # daily-note.md, and any compiled-in template you replace
```

`capture-obsidian/` shows both. The Recipes this version ships are written into
a directory of your own with `dgs capture --export-recipes`; the one here is an
extra, to show what a hand-written Recipe looks like.
