# Doc configuration

`<config dir>/doc/config.json`:

```json
{
  "trees": {
    "papers": "/Volumes/nas/Documents",
    "books": "/Volumes/nas/Books"
  },
  "date_order": "DMY",
  "expiring_within_days": 90,
  "cache": {
    "dir": ""
  },
  "web": [
    {"name": "pages", "port": 8767}
  ]
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `trees` | The document trees, `{"name": "folder"}`. With one tree the page opens it; with several — papers, books — the page switches between them from its top bar, and `verify`, `export` and `link` take `--tree <name>`. A name is lowercase letters, digits, `_` and `-`. A leading `~` is the home directory. `--root` opens one folder instead. | empty — one tree, the working directory |
| `date_order` | How a date with day and month both in digits and the year last, such as `03/04/2026`, is read when it is suggested from a PDF's text: `DMY` (3 April) or `MDY` (March 4). Case does not matter. A date whose first or second number is over 12 is read the only way it can be. | `DMY` |
| `expiring_within_days` | How many days before its expiry a document is shown as expiring soon on Browse: its badge, and the "expires within" filter. The expiry is the end of the current revision's `date` when it is a span. `0` uses the default; a negative number is refused. | `90` |
| `cache.dir` | Where the discardable cache is kept: the text read off each PDF, by its SHA-256, so a PDF is read once. And each tree's Items, parsed, so the page's questions do not read every sidecar ([cache](../apps/doc/cache.md)). It is outside the tree and belongs to one machine; deleting it costs a re-read. A leading `~` is the home directory. | empty — `dgs/doc` under the user cache directory |
| `web` | The servers, as [the shell](shell.md#web-servers) describes. `doc` has one, `pages`: the doc page. Its host is always `127.0.0.1` and a host is refused. `--port` overrides its port for one run. | `pages` on `127.0.0.1:8767` |

Export folders are not here: each Outline names its own, and the folder is
chosen when exporting — see [Outlines](../apps/doc/index.md#outlines).

What `dgs doc` does with these is in [`apps/doc/`](../apps/doc/index.md).
