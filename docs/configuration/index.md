# Configuration

Everything `dgs` can be told, in one place. This directory is the reference:
one document per part of the toolbox, each listing its keys, what they mean,
and what they default to. Design documents explain *why* a setting exists and
link here; this is what a reader consults to find out *what* to write.

- [The shell](shell.md) — where the configuration itself lives, and the top bar.
- [Capture](capture.md) — the Capture root, Recipes, templates, and the vault
  the Obsidian Actions write to.
- [Photo](photo.md) — Photo Import's state file and default paths.

## Where the configuration lives

`dgs` reads one JSON file. It is looked for in this order, and the first answer
wins:

1. `--config` / `-c <path>` on the command line.
2. `DGS_TOOLBOX_CONFIG` in the environment.
3. `<user configuration directory>/dgs/config.json` — on macOS,
   `~/Library/Application Support/dgs/config.json`.

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
| `capture.recipes` | [capture](capture.md#recipes-and-templates) | `<config_dir>/capture/recipes` |
| `capture.templates` | [capture](capture.md#recipes-and-templates) | `<config_dir>/capture/templates` |
| `capture.mappings` | [capture](capture.md#mappings) | none |
| `capture.obsidian.vault` | [capture](capture.md#the-vault) | empty — the Obsidian Actions refuse to run |
| `capture.obsidian.daily_note` | [capture](capture.md#the-daily-note) | empty — the daily Action refuses to run |
| `capture.obsidian.section` | [capture](capture.md#the-daily-note) | `Captured{{with .Device}} - {{.}}{{end}}` |
| `capture.obsidian.location_note` | [capture](capture.md#the-location-note) | empty — the location Action refuses to run |
| `capture.obsidian.location_archive` | [capture](capture.md#the-location-note) | empty — nothing is archived |
| `capture.obsidian.map_services` | [capture](capture.md#map-links-and-coordinates) | empty — every service, in the order below |
| `capture.obsidian.coordinate_choice` | [capture](capture.md#map-links-and-coordinates) | empty — coordinates are written unlinked |
| `photo.import.state_file` | [photo](photo.md) | `.dgs-state` |
| `photo.import.source` | [photo](photo.md) | empty — the repository's mock path |
| `photo.import.destination` | [photo](photo.md) | empty — the repository's mock path |

## Keeping this current

A new configuration file, or a new key in one, is not finished until it is
written down here. See the rule in [`AGENTS.md`](../../AGENTS.md).
