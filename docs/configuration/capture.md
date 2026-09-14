# Capture configuration

What Scan lists, what Route's Actions write, and where Archive puts a Capture
when it is done with. The sessions themselves are
described in [`apps/capture/scan.md`](../apps/capture/scan.md),
[`apps/capture/route.md`](../apps/capture/route.md) and
[`apps/capture/archive.md`](../apps/capture/archive.md); the Recipe and Action
model is in [`apps/capture/organizer.md`](../apps/capture/organizer.md).

```json
{
  "capture": {
    "scan": {
      "root": "~/Captures",
      "index_file": "index.json"
    },
    "archive": {
      "root": "~/Capture Archive",
      "reject": "~/Capture Rejected"
    },
    "obsidian": {
      "vault": "~/Vaults/personal",
      "daily_note": "00 Daily Log/{{.Year}}/{{.Date}}.md",
      "section": "Captured{{with .Device}} - {{.}}{{end}}",
      "location_note": "88 Inbox/06 Locations.md",
      "location_archive": "88 Inbox/06 Locations"
    },
    "apple": {
      "reminders": {
        "list": "Places",
        "radius": 200
      }
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

An empty `capture.scan.root` starts at the current working directory, and
`capture.scan.index_file` must be a filename rather than a path.

## Where Captures go

Archive moves a Capture out of the Capture root into one of two folders: the
archive, for Captures that have been organized, and the reject folder, for the
ones that are not worth keeping. Nothing is deleted — a rejected Capture is
moved out of the way — and nothing is renamed, so a Capture reads at its
destination exactly as it read in the root.

| Key | Meaning | Default |
| --- | --- | --- |
| `capture.archive.root` | The directory organized Captures are kept in. | empty — Archive refuses to archive and names this key |
| `capture.archive.reject` | The directory rejected Captures are set aside in. Scan rejects into it as well, so a Capture created by mistake is sent away the first time it is looked at. | empty — Scan and Archive refuse to reject and name this key |

Both are absolute paths, `~` expanded, and neither has to exist: the first move
into a folder creates it. They are configuration and nothing else — unlike the
Capture root, they cannot be chosen in the session, because where an
installation files things is decided once and a picker for it would be a
control used on the first run and never again. A folder left unset is not an
error: its pane in the session says which key to write, and a move that would
have used it is refused naming that same key.

Nothing stops either folder sitting inside the Capture root, but a Capture
filed there would be scanned again as a Capture still to handle, so they belong
outside it.

## What sits beside the configuration file

Capture reads three directories, and none of them is configurable:

```text
<config_dir>/capture/
  recipes/     one file per Recipe
  workflows/   one file per workflow: where it keeps each field
  mappings/    one file per translation table
  templates/   daily-note.md, and any compiled-in template you replace
```

The layout is the answer to "where does this installation keep its Capture
configuration?", and a path for each would make that answer three paths to go
and look up. `config_dir` moves the whole thing at once, and a directory that
genuinely belongs elsewhere — a vault's templates kept with the vault — is a
symlink.

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
dgs capture --init
```

That writes the Recipes and the workflow descriptions this version ships into
the configuration, skipping any file already there, so running it again after
an upgrade adds what is new without touching what you have edited. From then on
they are ordinary files: edit one, rename it, or delete it.

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
  # Offered only when at least one of these resolves. Any field may be named,
  # including one a workflow file describes.
  requires_any: [coordinates, place.city]

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

## Workflows: where a field is kept

An Action asks for a logical field — `content`, the text that was written —
and a workflow keeps it under whatever key its author chose. One file per
workflow says which, in the workflow directory:

```yaml
# ~/.config/dgs-toolbox/capture/workflows/quick_note.yaml
fields:
  content: payload.text
  tags: payload.labels
```

The filename is the workflow, the way a Recipe's filename is its id. With that
file, a `quick_note` Capture whose payload is
`{"text": "buy milk", "labels": ["errand"]}` answers `content` and `tags`
without anyone typing them, and a Recipe naming `obsidian.daily.append` runs
with nothing missing. Nothing in the Recipe mentions a payload key, and nothing
in `dgs-config.json` does either — workflows arrive one at a time and there is
no end to them, so they are a directory rather than a section of one file that
grows without bound.

### Many workflows, one field

A field may name several keys, tried in order, and one file may answer for a
family of workflows. `"*"` answers for any workflow with no file of its own:

```yaml
# ~/.config/dgs-toolbox/capture/workflows/notes.yaml
workflows: ["*"]

fields:
  content:
    - payload.text
    - payload.body
    - payload.note
```

`quick_note_1` writing `payload.text`, `quick_note_2` writing `payload.body`
and `xxx1` writing `payload.note` all answer `content` from that one file. A
workflow with a file of its own has said where it keeps its fields and is not
read through the general list as well — but two files naming the same workflow
compose field by field, so a family file and a file for one of its members do
not erase each other.

This is keyed by workflow rather than by Action on purpose. Which key holds the
text is a fact about the workflow that wrote the Capture; an Action only ever
asks for `content`, and every Action that asks is answered by the same file.
Keyed by Action it would be repeated for each Action in the Recipe, and it
would come too late — a Recipe's `requires_any` is checked before any Action is
chosen.

### One field out of several keys

Some workflows keep in pieces what an Action asks for as one thing — a title
and a body, say. A source that carries `{{ }}` is a template over the payload
rather than a key in it, so the field is whole by the time an Action asks:

```yaml
# ~/.config/dgs-toolbox/capture/workflows/quick_note.yaml
fields:
  content: "{{.title}}{{with .body}}\n\n{{.}}{{end}}"
```

The payload is the data, so `.title` is `payload.title` and `.meta.heading`
reaches into it. The functions every other template in this toolbox has —
`join`, `trim`, `default`, `indent`, `mapped` — are here too.

A piece that is not always there is guarded with `{{with}}`, which writes
nothing when the key is absent. A key named without a guard, on a Capture that
does not carry it, leaves the whole field missing and Route asks for it in
`FIELDS` — the same answer a path that resolves to nothing gives.

A template that does not parse is a different thing: it is a mistake, so the
file is refused when it loads, naming the field. A template and a plain key can
be listed together, and the first that resolves answers.

### The rules

- **Paths start at `payload`** and may go deeper: `payload.detail.long`. The
  rest of the index has one meaning already and needs no telling. A file with a
  path starting anywhere else is rejected, with the field that is wrong, rather
  than quietly resolving to nothing. A source carrying `{{ }}` is a template
  instead, and is held to parsing rather than to that shape.
- **A bad file fails alone.** The rest of the directory still loads and the
  session still opens. `dgs capture --recipes` ends with a `SOURCES` section
  saying what was read, from where, and what was rejected.
- **A path that leads nowhere leaves the field missing**, which Route asks for
  in `FIELDS` the way it asks for any other. It is not an error: a Capture that
  happens not to carry the key is the ordinary case.
- **A list of strings stays a list**, so `tags` arrives as tags rather than as
  a rendered array. A number or boolean is read as its text.
- **What the reader typed in `FIELDS` wins over the file**, so a Capture whose
  key held the wrong thing is still organized without editing anything.
- **Nothing in the payload is read without a file saying where it is.** The
  workflows this toolbox ships shortcuts for — `been_here`, `photo_note`,
  `quick_mark` — are files like everyone else's, laid down by `dgs capture
  --init`. An installation that has laid nothing down resolves no field from a
  payload and asks for it in `FIELDS`; the fields the index itself gives a
  meaning, `createdAt` and the place and the coordinates, are read as always.

### A position kept in the payload

A workflow whose Capture is about somewhere other than where the phone was
sources `coordinates`, whole:

```yaml
fields:
  coordinates: payload.place          # "lat, lng", or {latitude, longitude} / {lat, lng}
  # coordinates: "{{.y}}, {{.x}}"     # or put together from two keys
```

- **The position is one field.** `coordinates.latitude` and
  `coordinates.longitude` cannot be sourced on their own — the file is refused,
  naming the field — so the two halves can never come from different points.
  The location note, its map links and any Action reading the position all
  read this one answer.
- **A sourced position does not fall back to the index.** When the key is
  missing or is not a position — halves that are not numbers, or off the globe —
  the Capture has no position and Route asks for one in `FIELDS`.
- **The index's altitude and place are then left out**: `Altitude`, `Address`,
  `Place`, the map pin's label, and `place.*` fields describe where the phone
  was. Source `place.name` or the others too when the note should carry them.
- **Coordinates are WGS-84**, as the index's are. Nothing is converted.

## Mappings

What a Capture records and what a vault files it under are not always the same
string: `Australia` against `🇦🇺_Australia`. One file per table, named after it,
and the file is the table:

```yaml
# ~/.config/dgs-toolbox/capture/mappings/country.yaml
Australia: 🇦🇺_Australia
China: 🇨🇳_中国
```

A template asks for one by name:

```gotemplate
country: "{{mapped "country" .Country}}"
```

A value the table does not mention is written as it was — a mapping says how
some names are written here, not which names are allowed, and dropping the rest
would lose what the Capture carried. A table nothing asks for changes nothing:
it is the template that decides where a translation applies, so the same
Capture can be filed one way in the daily note and another in the list of
places.

`dgs capture --init` writes the tables this version ships, currently
`weekday.yaml`, which the location note's date marker asks for.

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

## Reminders

How the reminder Actions write when a run does not say otherwise. Both are
parameters too, so one run can change either in `FIELDS`. The Actions and the
`dgs-reminders` helper they need are described in
[`apps/capture/actions.md`](../apps/capture/actions.md#reminders).

| Key | Meaning | Default |
| --- | --- | --- |
| `capture.apple.reminders.list` | The title of the Reminders list a reminder is written into. A list that does not exist, or a title two lists share, refuses rather than writing somewhere else. | empty — Reminders' own default list |
| `capture.apple.reminders.radius` | How close counts as arriving at, or leaving, a reminder's place, in metres. | `150` |

## Map links and coordinates

These are not configuration. The location entry's template decides which map
services a line carries and which vault command a coordinate links to, because
they are decisions about how that line reads:

```gotemplate
- {{.CopyLink "Copy Coordinates (lng, lat)"}}
- {{.MapLinks "Apple, 高德, Google"}}
```

`MapLinks` takes the services by name or short name, in the order they are
written, and with no argument writes every one of them. A name nothing matches
is left out rather than guessed at. `CopyLink` takes the vault command that
puts a position on the clipboard; with no argument the position is written
unlinked.

The services, by name and short name: `Apple 地图` / `Apple`, `高德地图` /
`高德`, `Google 地图` / `Google`, `百度地图` / `百度`, `OpenStreetMap` / `OSM`.
Coordinates are handed to every service as WGS-84, which is what a device
records; the Chinese services are told the source system and convert.
