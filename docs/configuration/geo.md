# Geo configuration

```json
{
  "geo": {
    "gpx": {
      "host": "127.0.0.1",
      "port": 8765,
      "root": "~/Tracks",
      "tiles": [
        {
          "name": "OpenTopoMap",
          "url": "https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png",
          "attribution": "© OpenStreetMap contributors, SRTM | © OpenTopoMap",
          "max_zoom": 17,
          "coordinates": "wgs84"
        }
      ]
    }
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `geo.gpx.host` | The IP address the GPX web server listens on. `0.0.0.0` lets other machines open the page. Empty uses the default. `--host` overrides it for one run. | `127.0.0.1` |
| `geo.gpx.port` | The port the GPX web server listens on. `0` uses the default. `--port` overrides it for one run. | `8765` |
| `geo.gpx.root` | The folder the GPX browser opens at. A leading `~` is the home directory. `--dir` overrides it for one run. | empty — the home directory |
| `geo.gpx.tiles` | Base maps the page offers after the built-in ones (OpenStreetMap, OpenFreeMap Fiord, Gaode, Gaode Satellite, Esri World Imagery), in this order. | empty — the built-in maps only |
| `geo.gpx.tiles[].name` | The name shown in the base map selector. Required. | — |
| `geo.gpx.tiles[].url` | A raster tile URL template. It must contain `{z}`, `{x}` and `{y}`; `{s}` is replaced by the subdomains `a`, `b` and `c`. A configuration missing a placeholder is refused. | — |
| `geo.gpx.tiles[].attribution` | The credit shown on the map. Tile providers usually require one. | empty |
| `geo.gpx.tiles[].max_zoom` | The deepest zoom the provider serves. | `19` |
| `geo.gpx.tiles[].coordinates` | The system the tiles are drawn in: `wgs84`, or `gcj02` for maps published in China (Gaode, Tencent). The page's GCJ-02 *Auto* converts tracks for a `gcj02` map. Any other value is refused. | `wgs84` |

What GPX does with these is in [`apps/geo/gpx.md`](../apps/geo/gpx.md).
