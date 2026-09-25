# Doc configuration

```json
{
  "doc": {
    "root": "/Volumes/nas/Documents",
    "cache_dir": "",
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
| `doc.web.port` | The port the page listens on. It always listens on `127.0.0.1`. `0` uses the default. `--port` overrides it for one run. | `8767` |

What `dgs doc` does with these is in [`apps/doc/`](../apps/doc/index.md).
