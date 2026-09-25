# Doc configuration

```json
{
  "doc": {
    "root": "/Volumes/nas/Documents",
    "web": {
      "port": 8767
    }
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `doc.root` | The document tree the page opens. A leading `~` is the home directory. `--root` overrides it for one run. | empty — the working directory |
| `doc.web.port` | The port the page listens on. It always listens on `127.0.0.1`. `0` uses the default. `--port` overrides it for one run. | `8767` |

What `dgs doc` does with these is in [`apps/doc/`](../apps/doc/index.md).
