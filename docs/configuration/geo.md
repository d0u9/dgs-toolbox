# Geo configuration

```json
{
  "geo": {
    "gpx": {
      "host": "127.0.0.1",
      "port": 8765
    }
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `geo.gpx.host` | The IP address the GPX web server listens on. `0.0.0.0` lets other machines open the page. Empty uses the default. `--host` overrides it for one run. | `127.0.0.1` |
| `geo.gpx.port` | The port the GPX web server listens on. `0` uses the default. `--port` overrides it for one run. | `8765` |

What GPX does with these is in [`apps/geo/gpx.md`](../apps/geo/gpx.md).
