# Plugins

`dgs plugins` installs the plugins dgs carries for other applications, and
says what is installed where. So far they are all for Obsidian:

| Plugin | What it is | Where it goes |
| --- | --- | --- |
| `obsidian/dgs-toolbox` | The Obsidian plugin that writes timelines with the same code as `dgs capture`, and styles them. | every configuration folder's `plugins/dgs-toolbox/` |
| `obsidian/public` | The scripts Templater, QuickAdd and the plugin share: location, weather, map links, the daily log. | [`obsidian.folders.public`](../../configuration/plugins.md) |
| `obsidian/quickadd` | The QuickAdd scripts: quick note, idea, copy coordinates. | `obsidian.folders.quickadd` |
| `obsidian/templates` | The Templater templates: the daily log, inserting and updating location, weather and dates. | `obsidian.folders.templates` |

```bash
dgs plugins status
dgs plugins install [<plugin>...] [--vault <dir>]...
dgs plugins update  [<plugin>...] [--vault <dir>]... [--force]
dgs plugins uninstall <plugin>... [--vault <dir>]... [--force]
dgs plugins bootstrap [--vault <dir>]... [--from <dir>]
dgs plugins export <dir> [--vault <dir>] [--force]
```

A plugin is named `<host>/<id>` — `obsidian/dgs-toolbox` — or by its id alone
when only one host has a plugin of that id. With none named, every plugin dgs
carries is meant (except by `uninstall`, which must be told).

## Why dgs carries them

A plugin that does what a dgs command does has to agree with it, byte for
byte: the Obsidian plugin and `dgs capture` both edit a timeline note, and a
difference between them is a corrupted note. So the plugin is built from the
same commit as dgs — its engine is `internal/timeline` compiled to
WebAssembly — and dgs carries it compiled in. There is nothing to pin and no
second repository to keep in step, and one `dgs` file is still all there is to
install ([One binary](../../../AGENTS.md#one-binary)).

## Setting up a vault

`bootstrap` is everything a vault needs, for someone starting from nothing:

1. It installs every plugin, as `install` does.
2. It writes the **starting files** the vault lacks: the timelines file, and the
   dgs-toolbox plugin's settings pointing at it. These are yours from then on —
   `bootstrap` never writes over one, and `update` never touches them.
3. It lists what is left to do by hand, in `.obsidian/`: Templater and
   QuickAdd installed and switched on, Templater's script files folder set to
   the Public folder and its template folder holding the templates, a folder
   template giving the daily log its template, and a QuickAdd macro choice
   for each QuickAdd script, none pointing at a script that is gone. dgs does
   not install other people's plugins, nor write their settings: Obsidian
   rewrites those from memory while it runs.

Run it again until it says nothing is left; it changes nothing that is already
right.

To hand your starting files to someone, `export <dir>` copies them from your
vault, as they are, into a folder; `bootstrap --from <dir>` on theirs takes
each from that folder instead of the one dgs carries. Notes are never part of
it.

Only `.obsidian/` is set up and checked. A configuration folder for a phone
gets the plugin, but its settings are its owner's to keep.

## Hosts and places

A host is the application a plugin extends. Each host says where it keeps
plugins; everything after that — comparing, writing, removing — is the same for
every host (`internal/plugins`).

- **obsidian** — every configuration folder of each vault: a folder at the
  vault's top whose name starts with a dot and which holds `app.json`. A vault
  has `.obsidian/`, and may have more, such as `.obsidian-mobile/` for a phone;
  Obsidian reads only the one it is set to, so a plugin is installed into each.
  The scripts and templates go into folders of the vault itself, once.
  The vaults are `--vault`, or [`plugins.obsidian.vaults`](../../configuration/plugins.md).

## What is installed

Every install writes `.dgs-install.json` beside the plugin's files: the plugin,
the dgs that wrote it, when, and the SHA-256 of every file. Its **version** is
a digest of those files, so two installs of the same files have the same
version whichever dgs wrote them, and a dgs rebuilt without changing the plugin
does not call it out of date.

`status` compares each place with what this dgs carries:

| State | Meaning |
| --- | --- |
| `ok` | The files this dgs carries. |
| `outdated` | What another dgs installed, untouched since. `update` replaces it. |
| `modified` | An install whose files were changed or removed since, named in the table. Left alone unless `--force`. |
| `unmanaged` | A folder dgs did not install: there is no record. `install --force` replaces it; `uninstall` never touches it. |
| `missing` | Nothing installed. The host's settings for the plugin may still be there. |

`ON` says whether the host has the plugin switched on, read from Obsidian's
`community-plugins.json`. dgs never writes that file: Obsidian rewrites it from
memory while it runs, and switching a plugin on is the reader's to do.

## Writing

- Each file is written beside its final name and renamed over it. The record
  is written last, so an install cut short reads as `modified`, not `ok`.
- A file an earlier install wrote that this one does not carry is removed.
- A file dgs is about to write for the first time, where a different one is
  already, is someone else's: the install stops and names it, unless `--force`.
  The vault folders may hold your own files beside dgs's; `uninstall` leaves
  them, and the folder with them.
- The scripts name the folders they were installed into — a QuickAdd script
  loads the Public scripts from theirs — so the sources say `{{dgs:public}}`,
  `{{dgs:quickadd}}`, `{{dgs:templates}}` and `{{dgs:timelines}}`, and the
  install writes the configured paths in their place. Changing a folder makes
  every file dgs carries a new version.
- Nothing else in the plugin's folder is touched. Obsidian keeps a plugin's
  settings there as `data.json`; it survives `update` and `uninstall`, so
  installing again finds them.
- Obsidian loads a plugin when it starts or when it is switched on. After
  `install` or `update`, reload Obsidian, or switch the plugin off and on.

## The Obsidian plugin

The sources of everything are in `internal/plugins/bundled/obsidian/`, one
folder per plugin, and `seeds/` for the starting files. The scripts and
templates are edited there and installed; the vault's copies are dgs's.

The plugin's sources are in `dgs-toolbox/`: `manifest.json`,
`engine.js` and `main.js`, plain JavaScript checked with `// @ts-check`, so
building dgs needs no JavaScript toolchain. `make plugins` adds what is
generated — `timeline.wasm`, and Go's `wasm_exec.js` — and `make install` runs
it first. A dgs built without it says so and installs nothing.

What it does: the command *Insert timeline entry* writes one entry onto any
of the timelines the vault's timelines file defines
([timelines](../../configuration/timelines.md)), and into the day's log, in
one go. Every time it shows or reads carries its offset. Where the device is,
the address, the map links and the append to the daily log are the Public
scripts — named in the plugin's settings — loaded through their `quickAddShim.js`, so the vault has one implementation of
each whoever calls it. An entry from `dgs capture` brings its position with it.

The command *Tidy timeline* rewrites the chosen timelines — as many as are
ticked, the open note's to start with — into that shape: the running note and
every `<year>.md` in its archive folder read together, every way an entry has
been written by hand or by an older script recognised, written back days
newest first and entries latest first, an entry read twice kept once, the
current year in the note and the rest in their years' archives. It is the
engine's `tidy` operation (`internal/timeline/tidy.go`), so it agrees with
inserting and archiving. Nothing is made up — an entry with no time keeps its
line, one with no date goes under 无日期 at the top — and a line that is not
part of the list stops the tidy, naming it, rather than being dropped.
*Preview* says what would change and writes nothing.

Obsidian loads a single script, so the installed `main.js` is `wasm_exec.js`,
`engine.js` and `main.js` one after another. `make test-timeline` loads the
first two the same way and runs the shared timeline cases through them.

`styles.css` styles the form, and draws every note with `cssclasses: dgs-timeline`
and the daily log's places section as a timeline, in reading view: the blocks under
the heading set as Log heading, which the plugin marks `.dgs-daily-places` as
they render, with 去过哪里 also found by name for a vault where the plugin is
off. Coordinates and map links are found by their links, not by their place
under the entry, so a timeline whose entries carry only a line of text is not
drawn as a place. It used to
be a CSS snippet each vault kept; as part of the plugin it is on whenever the
plugin is, and updated with it. Notes with `cssclasses: captures`
(the ideas inbox) get only its quieter timestamps.
