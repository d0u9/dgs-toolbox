# Box configuration

```json
{
  "box": {
    "root": "/Volumes/nas/Box",
    "inbox": "~/Scans/inbox",
    "marker": "dgs-box.yaml",
    "state_file": "dgs-box-state.json",
    "cache_dir": "",
    "workers": 4,
    "currency": "AUD",
    "timezone": "",
    "web": {
      "port": 8766
    },
    "preview": {
      "keep": 30
    },
    "trash": {
      "keep": 90
    }
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `box.root` | The Box. A leading `~` is the home directory. Nothing is written unless `box.marker` is found here, so a NAS that is not mounted is refused rather than turned into a second, local Box. `--root` overrides it for one run. | empty — the command asks for a folder |
| `box.inbox` | The folder the `dgs box` intake page reads new scans from. A leading `~` is the home directory. `--inbox` overrides it for one run, which is how a second source is used without changing the file. | empty — intake opens with no inbox |
| `box.marker` | The name of the file at `box.root` that proves it is a Box, and which records the version that created it. A bare filename, not a path: a configuration giving a path is refused. Only `dgs box init` ever writes it, and no other command creates it as a side effect — a gate any command could open is not a gate. | `dgs-box.yaml` |
| `box.state_file` | The name of the file Import keeps its resumable state in, inside the inbox. A bare filename, not a path. | `dgs-box-state.json` |
| `box.cache_dir` | Where the discardable index and the thumbnails are kept. It is deliberately outside the Box: a cache belongs to one machine, and a database file on a network filesystem is a way to lose data. Empty uses the user cache directory — `~/Library/Caches/dgs/box` on macOS — with one subdirectory per Box root. | empty — the user cache directory |
| `box.workers` | How many inbox files Import reads at once. A handful is faster than one over a network filesystem, and many are slower than a handful. Filing itself stays one file at a time, so a failed copy stops that scan and nothing else. | `4` |
| `box.currency` | The ISO 4217 code an amount typed with no currency is taken to be in. Empty means an amount must always name its currency. A code this build has no minor-unit exponent for is refused. | empty — every amount names its currency |
| `box.timezone` | The IANA zone name a newly entered event date is assumed to be in, for example `Australia/Sydney`. An offset such as `+10:00` is refused, because an offset recorded now is wrong when the same date is read in another month. Empty uses the machine's current zone. | empty — the machine's zone |
| `box.web.port` | The port the `box` pages listen on. `0` uses the default. `--port` overrides it for one run. The address is always a loopback one and is not configurable: a Box holds identity and medical documents, so reaching it from another machine is a design with access control, not a key. | `8766` |
| `box.preview.keep` | How many days an unused 1600 px preview is kept before it is dropped. Grid thumbnails are a few KB and are never dropped. `0` keeps previews indefinitely. | `30` |
| `box.trash.keep` | How many days the trash is worth keeping, shown as advice on the `dgs box` browse page. `box` never removes a file: emptying `trash/` is done by hand. `0` shows no advice. | `90` |

Durations here are counted in days rather than written as `90d`, because Go's
duration syntax has no day unit and `2160h` is not what anyone means.

Some names a reader might look for are deliberately not keys. `trash/` inside
the Box, `index.json` inside the cache directory, `dgs-box-log.jsonl` at the
root and the two thumbnail sizes are fixed. A thumbnail size that could be
changed would invalidate every image already on disk and so would need a whole
eviction rule of its own, in exchange for a number nobody sets twice; the
others are names nothing would ever be gained by renaming. They are named
constants with documented defaults in the code, which is what
[`AGENTS.md`](../../AGENTS.md) asks of a parameter — not necessarily a key in
this file.

The type catalogue is not configurable at all — not the names and not the
lifetimes. Types are registered in code, documented in
[`apps/box/types.md`](../apps/box/types.md) and pinned there by a test, and a
lifetime is part of what a type *is*, so it lives where the type lives. A type
name goes into a sidecar and has to mean the same thing on another machine;
splitting half its definition into this file would make that less true for a
number nobody sets twice.

What `box` does with these is in [`apps/box/index.md`](../apps/box/index.md),
[`apps/box/import.md`](../apps/box/import.md) and
[`apps/box/view.md`](../apps/box/view.md).
