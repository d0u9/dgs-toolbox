# Box configuration

```json
{
  "box": {
    "root": "/Volumes/nas/Box",
    "inbox": "~/Scans/inbox",
    "marker": ".dgs-box",
    "state_file": ".dgs-box-state",
    "cache_dir": "",
    "index_file": "index.json",
    "workers": 4,
    "currency": "AUD",
    "timezone": "",
    "web": {
      "port": 8766
    },
    "thumbnail": {
      "size": 300
    },
    "preview": {
      "size": 1600,
      "keep": 30
    },
    "trash": {
      "dir": "trash",
      "keep": 90
    },
    "lifetimes": {
      "receipt": 3650
    }
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `box.root` | The Box. A leading `~` is the home directory. Nothing is written unless `box.marker` is found here, so a NAS that is not mounted is refused rather than turned into a second, local Box. `--root` overrides it for one run. | empty — the command asks for a folder |
| `box.inbox` | The folder `dgs box import` reads new scans from. A leading `~` is the home directory. `--inbox` overrides it for one run, which is how a second source is used without changing the file. | empty — Import opens with no inbox |
| `box.marker` | The name of the file at `box.root` that proves it is a Box. A bare filename, not a path: a configuration giving a path is refused. Only `dgs box init` ever writes it. | `.dgs-box` |
| `box.state_file` | The name of the file Import keeps its resumable state in, inside the inbox. A bare filename, not a path. | `.dgs-box-state` |
| `box.cache_dir` | Where the discardable index and the thumbnails are kept. It is deliberately outside the Box: a cache belongs to one machine, and a database file on a network filesystem is a way to lose data. Empty uses the user cache directory — `~/Library/Caches/dgs/box` on macOS — with one subdirectory per Box root. | empty — the user cache directory |
| `box.index_file` | The name of the index inside the Box's cache directory. A bare filename, not a path. | `index.json` |
| `box.workers` | How many whole files Import copies and verifies at once. A handful is faster than one over a network filesystem, and many are slower than a handful. | `4` |
| `box.currency` | The ISO 4217 code an amount typed with no currency is taken to be in. Empty means an amount must always name its currency. A code this build has no minor-unit exponent for is refused. | empty — every amount names its currency |
| `box.timezone` | The IANA zone name a newly entered event date is assumed to be in, for example `Australia/Sydney`. An offset such as `+10:00` is refused, because an offset recorded now is wrong when the same date is read in another month. Empty uses the machine's current zone. | empty — the machine's zone |
| `box.web.port` | The port the `box` pages listen on. `0` uses the default. `--port` overrides it for one run. The address is always a loopback one and is not configurable: a Box holds identity and medical documents, so reaching it from another machine is a design with access control, not a key. | `8766` |
| `box.thumbnail.size` | The long edge, in pixels, of the grid thumbnail. | `300` |
| `box.preview.size` | The long edge, in pixels, of the preview shown when one scan is open. There is no full-resolution preview; opening the original opens the PDF in the system viewer. | `1600` |
| `box.preview.keep` | How many days an unused preview is kept before it is dropped. Thumbnails are a few KB and are never dropped. `0` keeps previews indefinitely. | `30` |
| `box.trash.dir` | The directory inside `box.root` that discarded scans are moved into, in a subdirectory per day of discard. It must be inside the Box so the move is a same-volume rename. A bare name, not a path. | `trash` |
| `box.trash.keep` | How many days the trash is worth keeping, shown as advice in `dgs box view`. `box` never removes a file: emptying the trash is done by hand. `0` shows no advice. | `90` |
| `box.lifetimes` | Overrides the default lifetime of a type, in days from the event date, for example `{"receipt": 3650}`. `0` makes a type permanent. A type this build does not register is refused. The defaults, and which types deliberately have none, are in [`apps/box/types.md`](../apps/box/types.md). | empty — the catalogue's defaults |

Durations here are counted in days rather than written as `90d`, because Go's
duration syntax has no day unit and `2160h` is not what anyone means.

What `box` does with these is in [`apps/box/index.md`](../apps/box/index.md),
[`apps/box/import.md`](../apps/box/import.md) and
[`apps/box/view.md`](../apps/box/view.md).
