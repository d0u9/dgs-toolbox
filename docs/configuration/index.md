# Configuration

Everything `dgs` can be told, in one place. This directory is the reference:
one document per part of the toolbox, each listing its keys, what they mean,
and what they default to. Design documents explain *why* a setting exists and
link here; this is what a reader consults to find out *what* to write.

- [The shell](shell.md) — where the configuration itself lives, and the top bar.
- [Capture](capture.md) — the Capture root, where Captures are archived or
  rejected, Recipes, templates, the vault
  the Obsidian Actions write to, and how reminders are written. The Actions a Recipe may name are catalogued
  in [`apps/capture/actions.md`](../apps/capture/actions.md).
- [Photo](photo.md) — Photo Import's state file and default paths.
- [Box](box.md) — the Box `dgs box` archives scans into, the inbox it takes
  them from, its discardable local cache, the trash, amounts and time zones,
  and per-type lifetime overrides. The types a lifetime may name are
  catalogued in [`apps/box/types.md`](../apps/box/types.md).
- [Doc](doc.md) — the document trees `dgs doc` opens, its local cache, and where its page listens.
- [Geo](geo.md) — where the GPX web server listens, the folder it browses, and its base maps.
- [Conf](conf.md) — the generator root `dgs conf` reads, holding the services
  and the inventory, its secrets tree, and where the destination form opens.
- [Credentials](cred.md) — `credentials.json`, a file of its own: where
  identities are searched for, where the recipient folder is, and the vault.

Working files to copy from are in [`examples/`](../../examples), one folder per
thing you might be setting up. A test loads every one of them, so an example
that stops being valid fails the build rather than the reader.

## Where the configuration lives

`dgs` reads one JSON file. It is looked for in this order, and the first answer
wins:

1. `--config` / `-c <path>` on the command line.
2. `DGS_TOOLBOX_CONFIG` in the environment.
3. `$XDG_CONFIG_HOME/dgs-toolbox/dgs-config.json`, or
   `~/.config/dgs-toolbox/dgs-config.json` when that variable is unset.

One default location rather than a list tried in turn: "which file am I
editing?" should not have an answer that depends on which files exist. It is
the folder the reader's other tools already keep their files in, and the name
is the one `--export-config` writes, so a file copied out of it is found under
the name it already has.

`dgs-toolbox/` is the whole configuration, not only the file: `config_dir`
defaults to the folder the file was found in, so the recipes and templates sit
beside it and the lot moves, copies or goes under version control as one thing.

```text
~/.config/dgs-toolbox/
  dgs-config.json
  capture/
    recipes/     # dgs capture --init writes the shipped ones here
    workflows/   # one file per workflow: where it keeps each field
    mappings/    # one file per translation table
    templates/
```

A file that does not exist is not an error: every setting has a default, or is
empty and the command that needs it says so. An unknown key *is* an error —
the file is rejected rather than half-applied, so a typo cannot silently leave
a setting at its default.

`dgs --export-config [path]` writes a complete default file to start from. It
never overwrites an existing one. See [the shell](shell.md) for the details.

Everything a command reads from disk rather than from that file — Recipes,
templates — lives under `config_dir`, laid out by command. That layout is in
[the shell](shell.md#the-configuration-directory).

## Every key, at a glance

| Key | Part | Default |
| --- | --- | --- |
| `config_dir` | [shell](shell.md#config_dir) | the directory the configuration file was loaded from |
| `tui.top_bar.disk` | [shell](shell.md#the-top-bar) | `true` |
| `tui.top_bar.network` | [shell](shell.md#the-top-bar) | `true` |
| `tui.top_bar.cpu` | [shell](shell.md#the-top-bar) | `true` |
| `tui.top_bar.time` | [shell](shell.md#the-top-bar) | `true` |
| `capture.scan.root` | [capture](capture.md#the-capture-root) | empty — Scan opens with no root |
| `capture.scan.index_file` | [capture](capture.md#the-capture-root) | `index.json` |
| `capture.archive.root` | [capture](capture.md#where-captures-go) | empty — Archive refuses to archive and names this key |
| `capture.archive.reject` | [capture](capture.md#where-captures-go) | empty — Scan and Archive refuse to reject and name this key |
| `capture.obsidian.vault` | [capture](capture.md#the-vault) | empty — the Obsidian Actions refuse to run |
| `capture.gpx.directory` | [capture](capture.md#daily-gpx) | empty — the GPX Action refuses to run |
| `capture.obsidian.daily_note` | [capture](capture.md#the-daily-note) | empty — the daily Action refuses to run |
| `capture.obsidian.section` | [capture](capture.md#the-daily-note) | `Captured{{with .Device}} - {{.}}{{end}}` |
| `capture.obsidian.images.folder` | [capture](capture.md#pictures-in-the-daily-note) | `assets/{{.Note}}` |
| `capture.obsidian.images.max_side` | [capture](capture.md#pictures-in-the-daily-note) | `2048` |
| `capture.obsidian.images.quality` | [capture](capture.md#pictures-in-the-daily-note) | `80` |
| `capture.obsidian.location_note` | [capture](capture.md#the-location-note) | empty — the location Action refuses to run |
| `capture.obsidian.location_archive` | [capture](capture.md#the-location-note) | empty — nothing is archived |
| `capture.apple.reminders.list` | [capture](capture.md#reminders) | empty — Reminders' own default list |
| `capture.apple.reminders.radius` | [capture](capture.md#reminders) | `150` |
| `photo.import.state_file` | [photo](photo.md) | `.dgs-state` |
| `photo.import.source` | [photo](photo.md) | empty — the repository's mock path |
| `photo.import.destination` | [photo](photo.md) | empty — the repository's mock path |
| `box.root` | [box](box.md) | empty — the command asks for a folder |
| `box.inbox` | [box](box.md) | empty — Import opens with no inbox |
| `box.marker` | [box](box.md) | `dgs-box.yaml` |
| `box.state_file` | [box](box.md) | `dgs-box-state.json` |
| `box.cache_dir` | [box](box.md) | empty — the user cache directory |
| `box.workers` | [box](box.md) | `4` |
| `box.currency` | [box](box.md) | empty — every amount names its currency |
| `box.timezone` | [box](box.md) | empty — the machine's zone |
| `box.web.port` | [box](box.md) | `8766` |
| `box.preview.keep` | [box](box.md) | `30` |
| `box.trash.keep` | [box](box.md) | `90` |
| `doc.root` | [doc](doc.md) | empty — the working directory |
| `doc.trees` | [doc](doc.md) | empty — the one tree `doc.root` names |
| `doc.cache_dir` | [doc](doc.md) | empty — the user cache directory |
| `doc.date_order` | [doc](doc.md) | `DMY` |
| `doc.expiring_within_days` | [doc](doc.md) | `90` |
| `doc.targets` | [doc](doc.md) | — no longer read; Targets are in each tree's `targets.yaml` |
| `doc.web.port` | [doc](doc.md) | `8767` |
| `geo.gpx.host` | [geo](geo.md) | `127.0.0.1` |
| `geo.gpx.port` | [geo](geo.md) | `8765` |
| `geo.gpx.root` | [geo](geo.md) | empty — the home directory |
| `geo.gpx.tiles` | [geo](geo.md) | empty — the built-in maps only |
| `conf.root` | [conf](conf.md) | empty — the page opens with no root and asks for one |
| `conf.secrets` | [conf](conf.md) | empty — an instance needing a secret refuses to render |
| `conf.export.dir` | [conf](conf.md) | empty — the home directory |
| `identities` (`credentials.json`) | [cred](cred.md) | empty — no identities |
| `recipients` (`credentials.json`) | [cred](cred.md) | empty — no recipients |
| `new_identity_dir` (`credentials.json`) | [cred](cred.md) | `~/.config/age` |
| `close_after` (`credentials.json`) | [cred](cred.md) | `5m` |
| `vault` (`credentials.json`) | [cred](cred.md) | empty — the page asks for a folder |
| `archive_skip` (`credentials.json`) | [cred](cred.md) | the built-in list of operating system, file manager and git junk |

## Keeping this current

A new configuration file, or a new key in one, is not finished until it is
written down here. See the rule in [`AGENTS.md`](../../AGENTS.md).
