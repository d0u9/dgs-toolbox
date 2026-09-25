# Doc configuration

```json
{
  "doc": {
    "root": "/Volumes/nas/Documents",
    "cache_dir": "",
    "date_order": "DMY",
    "targets": {
      "icloud": "~/Library/Mobile Documents/com~apple~CloudDocs/Documents",
      "google": "~/Google Drive/Documents"
    },
    "web": {
      "port": 8767
    }
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `doc.root` | The document tree the page opens. A leading `~` is the home directory. `--root` overrides it for one run. | empty — the working directory |
| `doc.cache_dir` | Where the discardable cache is kept: the text read off each PDF, by its SHA-256, so a PDF is read once. It is outside the tree and belongs to one machine; deleting it costs a re-read. A leading `~` is the home directory. | empty — `dgs/doc` under the user cache directory (`~/Library/Caches/dgs/doc` on macOS) |
| `doc.date_order` | How a date with day and month both in digits and the year last, such as `03/04/2026`, is read when it is suggested from a PDF's text: `DMY` (3 April) or `MDY` (March 4). Case does not matter. A date whose first or second number is over 12 is read the only way it can be. | `DMY` |
| `doc.targets` | Named local folders Views are exported to, `{"name": "folder"}`. A View names one with `target:`; every View naming a Target is exported into it together. An export writes only there, and removes only files its own manifest, `dgs-export.json`, says it wrote. A leading `~` is the home directory. A Target may not be inside the tree, nor the tree inside it. | empty — no Targets |
| `doc.web.port` | The port the page listens on. It always listens on `127.0.0.1`. `0` uses the default. `--port` overrides it for one run. | `8767` |

What `dgs doc` does with these is in [`apps/doc/`](../apps/doc/index.md).
