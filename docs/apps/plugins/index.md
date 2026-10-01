# Plugins

`dgs plugins` installs the plugins dgs carries for other applications, and
says what is installed where. So far there is one: `obsidian/dgs-toolbox`, the
Obsidian plugin that writes timelines with the same code as `dgs capture`.

```bash
dgs plugins status
dgs plugins install [<plugin>...] [--vault <dir>]...
dgs plugins update  [<plugin>...] [--vault <dir>]... [--force]
dgs plugins uninstall <plugin>... [--vault <dir>]... [--force]
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

## Hosts and places

A host is the application a plugin extends. Each host says where it keeps
plugins; everything after that — comparing, writing, removing — is the same for
every host (`internal/plugins`).

- **obsidian** — every configuration folder of each vault: a folder at the
  vault's top whose name starts with a dot and which holds `app.json`. A vault
  has `.obsidian/`, and may have more, such as `.obsidian-mobile/` for a phone;
  Obsidian reads only the one it is set to, so a plugin is installed into each.
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
- Nothing else in the plugin's folder is touched. Obsidian keeps a plugin's
  settings there as `data.json`; it survives `update` and `uninstall`, so
  installing again finds them.
- Obsidian loads a plugin when it starts or when it is switched on. After
  `install` or `update`, reload Obsidian, or switch the plugin off and on.

## The Obsidian plugin

Sources are in `internal/plugins/bundled/obsidian/dgs-toolbox/`: `manifest.json`,
`engine.js` and `main.js`, plain JavaScript checked with `// @ts-check`, so
building dgs needs no JavaScript toolchain. `make plugins` adds what is
generated — `timeline.wasm`, and Go's `wasm_exec.js` — and `make install` runs
it first. A dgs built without it says so and installs nothing.

What it does: the command *Insert timeline entry* writes one entry onto any
of the timelines the vault's timelines file defines
([timelines](../../configuration/timelines.md)), and into the day's log, in
one go. Every time it shows or reads carries its offset. Where the device is,
the address, the map links and the append to the daily log are the vault's own
scripts — the Templater user scripts folder, named in the plugin's settings —
loaded through their `quickAddShim.js`, so the vault has one implementation of
each whoever calls it. An entry from `dgs capture` brings its position with it.

Obsidian loads a single script, so the installed `main.js` is `wasm_exec.js`,
`engine.js` and `main.js` one after another. `make test-timeline` loads the
first two the same way and runs the shared timeline cases through them.
