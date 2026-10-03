# Configuration

Everything `dgs` can be told, in one place. This directory is the reference:
one document per part of the toolbox, each listing its keys, what they mean,
and what they default to. Design documents explain *why* a setting exists and
link here; this is what a reader consults to find out *what* to write.

- [The shell](shell.md) — where the configuration lives, the top bar, and
  the `web` and `cache` keys every command shares the shape of.
- [Capture](capture.md) — the Capture root, where Captures are archived or
  rejected, Recipes, templates, the vault
  the Obsidian Actions write to, and how reminders are written. The Actions a Recipe may name are catalogued
  in [`apps/capture/actions.md`](../apps/capture/actions.md).
- [Timelines](timelines.md) — the file in the vault defining every running
  list, read by the capture Actions and the dgs-toolbox Obsidian plugin alike.
- [Photo](photo.md) — Photo Import paths and Photo Encode defaults.
- [Box](box.md) — the Box `dgs box` archives scans into, the inbox it takes
  them from, its discardable local cache, the trash, amounts and time zones.
  The types it knows are catalogued in [`apps/box/types.md`](../apps/box/types.md).
- [Doc](doc.md) — the document trees `dgs doc` opens, its local cache, and where its page listens.
- [Geo](geo.md) — where the GPX web server listens, the folder it browses, its base maps, routing and contours.
- [Conf](conf.md) — the generator root `dgs conf` reads, holding the services
  and the inventory, its secrets tree, and where the destination form opens.
- [Plugins](plugins.md) — the Obsidian vaults `dgs plugins` installs the
  plugins dgs carries into, and the folders in them its scripts and
  templates go.
- [Credentials](cred.md) — where identities are searched for, where the
  recipient folder is, and the vault.

Working configurations to copy from are in [`examples/`](../../examples), one
folder per thing you might be setting up. A test loads every one of them, so an
example that stops being valid fails the build rather than the reader.

## Where the configuration lives

The configuration is a directory, one folder per command, each holding its own
`config.json`. The directory is looked for in this order, and the first answer
wins:

1. `--config` / `-c <dir>` on the command line.
2. `DGS_TOOLBOX_CONFIG` in the environment.
3. `$XDG_CONFIG_HOME/dgs-toolbox/`, or `~/.config/dgs-toolbox/` when that
   variable is unset.

One default location rather than a list tried in turn: "which file am I
editing?" should not have an answer that depends on which files exist.

```text
~/.config/dgs-toolbox/
  shell/config.json
  capture/config.json
  capture/recipes/     # dgs capture --init writes the shipped ones here
  capture/workflows/   # one file per workflow: where it keeps each field
  capture/mappings/    # one file per translation table
  capture/templates/
  photo/config.json
  box/config.json
  doc/config.json
  geo/config.json
  geo/gpx/tiles.json
  conf/config.json
  plugins/config.json
  cred/config.json
```

The commands are independent, and so is their configuration. A command reads
only its own folder. A file with a mistake in it refuses only its own command,
naming the file and the key; the others run as usual. Only a refused
`shell/config.json` stops `dgs`, because the shell reads it first. A command
that one day leaves the toolbox takes its folder with it unchanged.

A file that does not exist is not an error: every setting has a default, or is
empty and the command that needs it says so. An unknown key *is* an error — the
file is rejected rather than half-applied, so a typo cannot silently leave a
setting at its default.

The single-file layout this replaced — `dgs-config.json` and
`credentials.json` at the top of the directory — is not read. Either file being
there stops `dgs` with a message naming it, so settings someone wrote never
quietly stop applying.

`dgs --export-config [dir]` writes a complete default `config.json` for every
command. It never overwrites an existing file. See [the shell](shell.md) for
the details.

## Shared shapes

Two keys mean the same thing in every command that has them:

- `web` — a list of the command's web servers, each `{"name", "host", "port"}`,
  named by use. See [the shell](shell.md#web-servers).
- `cache.dir` — the command's discardable local cache. See
  [the shell](shell.md#local-caches).

## Every key, at a glance

Keys are written from the top of their part's file, `<config dir>/<part>/config.json`.

| Part | Key | Default |
| --- | --- | --- |
| [shell](shell.md#the-top-bar) | `top_bar.disk` | `true` |
| [shell](shell.md#the-top-bar) | `top_bar.network` | `true` |
| [shell](shell.md#the-top-bar) | `top_bar.cpu` | `true` |
| [shell](shell.md#the-top-bar) | `top_bar.time` | `true` |
| [capture](capture.md#the-capture-root) | `scan.root` | empty — Scan opens with no root |
| [capture](capture.md#the-capture-root) | `scan.index_file` | `index.json` |
| [capture](capture.md#where-captures-go) | `archive.root` | empty — Archive refuses to archive and names this key |
| [capture](capture.md#where-captures-go) | `archive.reject` | empty — Scan and Archive refuse to reject and name this key |
| [capture](capture.md#daily-gpx) | `gpx.root` | empty — the GPX Action refuses to run |
| [capture](capture.md#the-vault) | `obsidian.vault` | empty — the Obsidian Actions refuse to run |
| [capture](capture.md#the-daily-note) | `obsidian.daily.note` | empty — the daily Action refuses to run |
| [capture](capture.md#the-daily-note) | `obsidian.daily.section` | `Captured{{with .Device}} - {{.}}{{end}}` |
| [capture](capture.md#pictures-in-the-daily-note) | `obsidian.daily.images.folder` | `assets/{{.Note}}` |
| [capture](capture.md#pictures-in-the-daily-note) | `obsidian.daily.images.max_side` | `2048` |
| [capture](capture.md#pictures-in-the-daily-note) | `obsidian.daily.images.quality` | `80` |
| [capture](capture.md#timelines) | `obsidian.timelines` | empty — the timeline Actions refuse to run |
| [timelines](timelines.md) | `version` | none — must be `1` |
| [timelines](timelines.md) | `timelines.<name>.note` | none — required |
| [timelines](timelines.md) | `timelines.<name>.archive` | empty — the note's own folder |
| [timelines](timelines.md) | `timelines.<name>.split` | `year` |
| [timelines](timelines.md) | `timelines.<name>.cssclass` | empty |
| [timelines](timelines.md) | `timelines.<name>.title` | empty |
| [timelines](timelines.md) | `timelines.<name>.template` | empty — `timeline-entry.md`, else `location-entry.md` |
| [timelines](timelines.md) | `timelines.<name>.contentRequired` | `false` |
| [capture](capture.md#reminders) | `apple.reminders.list` | empty — Reminders' own default list |
| [capture](capture.md#reminders) | `apple.reminders.radius` | `150` |
| [photo](photo.md) | `import.state_file` | `.dgs-state` |
| [photo](photo.md) | `encode.source` | empty |
| [photo](photo.md) | `encode.destination` | empty |
| [photo](photo.md) | `encode.quality` | `82` |
| [photo](photo.md) | `encode.max_side` | `800` |
| [photo](photo.md) | `encode.ppi` | `240` |
| [photo](photo.md) | `encode.background` | `#ffffff` |
| [photo](photo.md) | `encode.recursive` | `false` |
| [photo](photo.md) | `encode.preserve_gps` | `false` |
| [photo](photo.md) | `encode.max_pixels` | `40000000` |
| [photo](photo.md) | `import.source` | empty — the repository's mock path |
| [photo](photo.md) | `import.destination` | empty — the repository's mock path |
| [box](box.md) | `root` | empty — the command asks for a folder |
| [box](box.md) | `inbox` | empty — Import opens with no inbox |
| [box](box.md) | `marker` | `dgs-box.yaml` |
| [box](box.md) | `state_file` | `dgs-box-state.json` |
| [box](box.md) | `workers` | `4` |
| [box](box.md) | `currency` | empty — every amount names its currency |
| [box](box.md) | `timezone` | empty — the machine's zone |
| [box](box.md) | `cache.dir` | empty — `dgs/box` under the user cache directory |
| [box](box.md) | `web` | `browse` on `127.0.0.1:8766`; host fixed |
| [box](box.md) | `preview.keep_days` | `30` |
| [box](box.md) | `trash.keep_days` | `90` |
| [doc](doc.md) | `trees` | empty — one tree, the working directory |
| [doc](doc.md) | `date_order` | `DMY` |
| [doc](doc.md) | `expiring_within_days` | `90` |
| [doc](doc.md) | `cache.dir` | empty — `dgs/doc` under the user cache directory |
| [doc](doc.md) | `web` | `pages` on `127.0.0.1:8767`; host fixed |
| [geo](geo.md) | `web` | `gpx` on `127.0.0.1:8765`; host settable |
| [geo](geo.md) | `gpx.root` | empty — the home directory |
| [geo](geo.md) | `gpx.tiles` | empty — the built-in maps only |
| [geo](geo.md) | `gpx.amap_key` | empty — OpenStreetMap routing only |
| [geo](geo.md) | `gpx.dem.url` | AWS's public Terrarium tiles |
| [geo](geo.md) | `gpx.dem.encoding` | `terrarium` |
| [geo](geo.md) | `gpx.dem.max_zoom` | `15` |
| [conf](conf.md) | `root` | empty — the TUI page opens with no root and asks for one |
| [conf](conf.md) | `secrets` | empty — an instance needing a secret refuses to render |
| [conf](conf.md) | `services` | empty — only the built-in definitions |
| [conf](conf.md) | `install_root` | empty — `/srv/rhumb` |
| [conf](conf.md) | `label_prefix` | empty — `rhumb` |
| [conf](conf.md) | `export.dir` | empty — the home directory |
| [plugins](plugins.md) | `obsidian.vaults` | empty — every run names a vault with `--vault` |
| [plugins](plugins.md) | `obsidian.folders.public` | `99 Toolkit/91 Scripts/01 DGS/00 Public` |
| [plugins](plugins.md) | `obsidian.folders.quickadd` | `99 Toolkit/91 Scripts/01 DGS/02 QuickAdd` |
| [plugins](plugins.md) | `obsidian.folders.templates` | `99 Toolkit/01 Templates/01 DGS` |
| [plugins](plugins.md) | `obsidian.timelines` | `99 Toolkit/timelines.json` |
| [cred](cred.md) | `identities` | empty — no identities |
| [cred](cred.md) | `recipients` | empty — no recipients |
| [cred](cred.md) | `new_identity_dir` | `~/.config/age` |
| [cred](cred.md) | `close_after` | `5m` |
| [cred](cred.md) | `vault` | empty — the TUI asks for a folder |
| [cred](cred.md) | `archive_skip` | the built-in list of operating system, file manager and git junk |

## Keeping this current

A new part, or a new key in one, is not finished until it is
written down here. See the rule in [`AGENTS.md`](../../AGENTS.md).
