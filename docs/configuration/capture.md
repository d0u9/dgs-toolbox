# Capture configuration

What Scan lists and what Route's Actions write. The sessions themselves are
described in [`apps/capture/scan.md`](../apps/capture/scan.md) and
[`apps/capture/route.md`](../apps/capture/route.md); the Recipe and Action
model is in [`apps/capture/organizer.md`](../apps/capture/organizer.md).

```json
{
  "capture": {
    "scan": {
      "root": "~/Captures",
      "index_file": "index.json"
    },
    "recipes": "",
    "templates": "",
    "mappings": {
      "country": { "China": "🇨🇳_中国" }
    },
    "obsidian": {
      "vault": "~/Vaults/personal",
      "daily_note": "00 Daily Log/{{.Year}}/{{.Date}}.md",
      "section": "Captured{{with .Device}} - {{.}}{{end}}",
      "location_note": "88 Inbox/06 Locations.md",
      "location_archive": "88 Inbox/06 Locations",
      "map_services": "Apple, 高德, Google",
      "coordinate_choice": "Copy Coordinates (lng, lat)"
    }
  }
}
```

Paths beginning with `~` are expanded. Everything under `obsidian` other than
`vault` is relative to the vault, so what an Action plans, writes and records
survives the vault moving.

## The Capture root

| Key | Meaning | Default |
| --- | --- | --- |
| `capture.scan.root` | The directory Scan lists Captures from. It can also be chosen in the session. | empty — Scan opens with no root |
| `capture.scan.index_file` | The filename that makes a folder a Capture. A bare filename, not a path: a configuration giving a path is refused. | `index.json` |

## Recipes and templates

Both default to Capture's corner of the configuration directory —
`<config_dir>/capture/recipes` and `<config_dir>/capture/templates` — so
neither has to be named. A path is for a directory that lives somewhere of its
own, a vault's templates kept with the vault, say.

| Key | Meaning | Default |
| --- | --- | --- |
| `capture.recipes` | One file per Recipe, and the whole set — see [Recipes](#recipes) below. A missing or empty directory means Route offers nothing until `dgs capture --export-recipes` has been run; a Recipe file that fails to load takes only itself out of the Set. | `<config_dir>/capture/recipes` |
| `capture.templates` | Templates read by filename. One replaces the compiled-in template of the same name; `daily-note.md` has no compiled-in default and must be here for a missing daily note to be created. | `<config_dir>/capture/templates` |

The filenames the templates directory is read by:

| Filename | What it shapes | Compiled-in default |
| --- | --- | --- |
| `daily-entry.md` | One Capture as it is written into a daily note. | yes |
| `location-entry.md` | One Capture as it is written into the running list of places. | yes |
| `daily-note.md` | A day's note, when the day has none yet. | no — a missing one refuses rather than inventing a note |

## Recipes

A Recipe is one YAML file in the Recipe directory, named by its id:
`work_daily.yaml` defines `work_daily`. Nothing is compiled in — the directory
is the whole set — so a new installation starts by laying the shipped copies
down:

```bash
dgs capture --export-recipes
```

That writes them into the configured Recipe directory and skips any file
already there, so running it again after an upgrade adds what is new without
touching what you have edited. From then on they are ordinary files: edit one,
rename it, or delete it.

The Actions a Recipe may name are catalogued in
[`apps/capture/actions.md`](../apps/capture/actions.md), and
`dgs capture --actions` prints that catalogue from the binary in front of you.
`dgs capture --recipes` lists what actually loaded, where each Recipe came
from, and any file that was rejected with its reason. Route lists candidates in
the order the files sort in, which is changed by renaming them.

```yaml
# ~/.config/dgs-toolbox/capture/recipes/work_daily.yaml
name: Work Daily

match:
  workflows: [been_here, quick_mark]
  requires_any: [latitude, city]

fields:
  - id: project
    label: Project
    required: true

actions:
  - id: obsidian.daily.append
  - id: obsidian.location.append
    enabled: false
```

An unknown key is an error rather than an ignored line, and so is naming an
Action this binary does not carry: a Recipe that silently does less than it
says is worse than one that refuses to load. The full shape is in
[`apps/capture/organizer.md`](../apps/capture/organizer.md#recipe-files).

### Switching one off

`enabled: false` at the top of a Recipe file takes it out of what Route offers
without deleting it. The file stays and keeps saying what it does, the report
lists it as `· disabled`, and removing that one line brings it back — which
deleting the file does not.

`enabled: false` on one action inside a Recipe is the smaller switch: the
Action stays in the Recipe and is still listed in `ACTIONS`, it is simply not
ticked when the Recipe is chosen.

## Mappings

`capture.mappings` translates a value on its way into a note, by table name:
what a Capture recorded against what this vault files it under. A template
reads one with `{{mapped "country" .Country}}`; a value the table does not
mention comes back as it was, because a mapping says how some names are
written here, not which names are allowed.

```json
{
  "capture": {
    "mappings": {
      "country": { "Australia": "🇦🇺_Australia", "China": "🇨🇳_中国" },
      "weekday": { "Monday": "周一" }
    }
  }
}
```

Table names are the templates' business rather than the configuration's. Two
are read by what ships today: `country`, by the entry templates, and
`weekday`, by the location note's date marker.

## The vault

| Key | Meaning | Default |
| --- | --- | --- |
| `capture.obsidian.vault` | An absolute path to the Obsidian vault. Nothing is discovered from it: a vault says where the plugin in use puts notes, which is not the same question as where this tool should write. | empty — the Obsidian Actions refuse to run rather than guess |

## The daily note

| Key | Meaning | Default |
| --- | --- | --- |
| `capture.obsidian.daily_note` | Where a day's note lives, relative to the vault, as a template over the date: `{{.Year}}`, `{{.Month}}`, `{{.Day}}`, `{{.Date}}`. | empty — the daily Action refuses to run |
| `capture.obsidian.section` | The heading a Capture is appended under. A template over the Capture, so a heading can name the device or app the entries under it came from. It may carry its own hashes — `## Captured` asks for a second-level heading — and is otherwise a first-level one. One run can override it from the `section` parameter in `FIELDS`. | `Captured{{with .Device}} - {{.}}{{end}}` |

## The location note

| Key | Meaning | Default |
| --- | --- | --- |
| `capture.obsidian.location_note` | The running list of places, newest first, relative to the vault. | empty — the location Action refuses to run |
| `capture.obsidian.location_archive` | The folder a year that has rolled over is moved into, as `<archive>/<year>.md`. | empty — nothing is archived and the list grows without limit |

## Map links and coordinates

| Key | Meaning | Default |
| --- | --- | --- |
| `capture.obsidian.map_services` | Which map links an entry carries, by name or short name, comma-separated, in the order they are written. A name nothing matches is left out rather than guessed at. | empty — every service, in the order below |
| `capture.obsidian.coordinate_choice` | The vault command a coordinate links to, which puts it on the clipboard. | empty — the coordinates are written unlinked |

The services, by name and short name: `Apple 地图` / `Apple`, `高德地图` /
`高德`, `Google 地图` / `Google`, `百度地图` / `百度`, `OpenStreetMap` / `OSM`.
Coordinates are handed to every service as WGS-84, which is what a device
records; the Chinese services are told the source system and convert.
