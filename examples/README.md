# Examples

Working configurations, one folder per thing you might be setting up. Each is a
configuration directory holding only the parts and keys that folder is about —
`capture/config.json`, `box/config.json` — so a folder can be read on its own
and its files copied into whatever you already have.

| Folder | What it sets up |
| --- | --- |
| [`shell/`](shell) | The top bar: which system metrics are shown. |
| [`capture-scan/`](capture-scan) | Capture's root and index filename — the least you need for Scan to list something — plus the two folders Archive files Captures into. |
| [`capture-obsidian/`](capture-obsidian) | Capture writing into an Obsidian vault: the daily note, the running list of places, map links, name mappings, plus Recipes, workflow descriptions and templates. |
| [`capture-gpx/`](capture-gpx) | Capture positions as timed waypoints in daily GPX files. |
| [`capture-reminders/`](capture-reminders) | Reminders at a place: the list and radius, a workflow keeping its position in the payload, and a Recipe that reminds you on arriving. |
| [`photo-import/`](photo-import) | Photo Import's source, destination and state file. |
| [`box/`](box) | `dgs box`'s Box root and inbox, and the currency and time zone new entries default to. |
| [`doc/`](doc) | `dgs doc`'s document tree and the port its page listens on. |
| [`doc-trees/`](doc-trees) | `dgs doc` over two trees, papers and books. |
| [`conf/`](conf) | `dgs conf export`'s `conf/config.json`, plus a complete generator root — `services/`, `nodes/`, `users.yaml`, `routes.yaml`, `networks.yaml` — matching [`rhumb docs/inventory.md`](https://github.com/d0u9/rhumb/blob/master/docs/inventory.md)'s worked example. |
| [`plugins/`](plugins) | The Obsidian vault `dgs plugins` installs into. |
| [`cred/`](cred) | `dgs cred`'s `cred/config.json`, and a recipient folder with two hosts and a group. |

Every key is documented in [`docs/configuration/`](../docs/configuration), and
a test loads each of these files, so an example that stops being valid fails
the build rather than misleading someone.

## Using one

Configuration is looked for in `$XDG_CONFIG_HOME/dgs-toolbox/`, or
`~/.config/dgs-toolbox/` when that variable is unset. Copy a folder's contents
there:

```bash
mkdir -p ~/.config/dgs-toolbox
cp -R examples/capture-obsidian/. ~/.config/dgs-toolbox/
```

Or run against one without installing it, which is also how to try a second
configuration next to your own:

```bash
dgs capture -c examples/capture-obsidian
```

Paths in these files are examples — `~/Vaults/personal`, `/Volumes/SD/DCIM` —
and are meant to be replaced. `~` is expanded.

## What sits beside the file

Each command's folder holds its `config.json` and everything else it reads
from disk:

```text
capture-obsidian/
  capture/
    config.json
    recipes/       # one YAML file per Recipe
    workflows/     # one file per workflow: where it keeps each field
    templates/     # daily-note.md, and any compiled-in template you replace
```

`capture-obsidian/` shows both. The Recipes this version ships are written into
a directory of your own with `dgs capture --init`; the one here is an
extra, to show what a hand-written Recipe looks like.
