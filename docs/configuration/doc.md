# Doc configuration

```json
{
  "doc": {
    "root": "/Volumes/nas/Documents",
    "cache_dir": "",
    "date_order": "DMY",
    "expiring_within_days": 90,
    "web": {
      "port": 8767
    }
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `doc.root` | The document tree the page opens. A leading `~` is the home directory. `--root` overrides it for one run. | empty — the working directory |
| `doc.trees` | Several trees, `{"name": "folder"}` — papers, books — when there is more than one. The page switches between them from its top bar, and `verify` and `export` take `--tree <name>`. Set this or `doc.root`, not both. A name is lowercase letters, digits, `_` and `-`. A leading `~` is the home directory. `--root` opens one folder instead. | empty — the one tree `doc.root` names |
| `doc.cache_dir` | Where the discardable cache is kept: the text read off each PDF, by its SHA-256, so a PDF is read once. It is outside the tree and belongs to one machine; deleting it costs a re-read. A leading `~` is the home directory. | empty — `dgs/doc` under the user cache directory (`~/Library/Caches/dgs/doc` on macOS) |
| `doc.date_order` | How a date with day and month both in digits and the year last, such as `03/04/2026`, is read when it is suggested from a PDF's text: `DMY` (3 April) or `MDY` (March 4). Case does not matter. A date whose first or second number is over 12 is read the only way it can be. | `DMY` |
| `doc.expiring_within_days` | How many days before its expiry a document is shown as expiring soon on Browse: its badge, and the "expires within" filter. The expiry is the first of the fields `expires`, `expiry`, `expires_at`, `expiry_date`, `valid_until` the current revision has. `0` uses the default; a negative number is refused. | `90` |
| `doc.targets` | No longer read: a configuration holding it is refused, with a message saying where Targets went. Targets are in each tree's `targets.yaml`, and the folder is chosen when exporting — see [Views](../apps/doc/index.md#views). | — |
| `doc.web.port` | The port the page listens on. It always listens on `127.0.0.1`. `0` uses the default. `--port` overrides it for one run. | `8767` |

With several trees, `doc.trees` takes the place of `doc.root`:

```json
{
  "doc": {
    "trees": {
      "papers": "/Volumes/nas/Documents",
      "books": "/Volumes/nas/Books"
    }
  }
}
```

What `dgs doc` does with these is in [`apps/doc/`](../apps/doc/index.md).
