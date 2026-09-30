# Geo configuration

`<config dir>/geo/config.json`:

```json
{
  "web": [
    {"name": "gpx", "host": "127.0.0.1", "port": 8765}
  ],
  "gpx": {
    "root": "~/Tracks",
    "tiles": [
      {
        "name": "OpenTopoMap",
        "url": "https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png",
        "attribution": "© OpenStreetMap contributors, SRTM | © OpenTopoMap",
        "max_zoom": 17,
        "coordinates": "wgs84"
      }
    ],
    "amap_key": "",
    "dem": {
      "url": "",
      "encoding": "",
      "max_zoom": 0
    }
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `web` | The servers, as [the shell](shell.md#web-servers) describes. Geo has one, `gpx`: the GPX page. Its host may be set: `0.0.0.0` lets other machines open the page. `--host` and `--port` override it for one run. | `gpx` on `127.0.0.1:8765` |
| `gpx.root` | The folder the GPX browser opens at. A leading `~` is the home directory. `--dir` overrides it for one run. | empty — the home directory |
| `gpx.tiles` | Base maps the page offers after the built-in ones (OpenStreetMap, OpenFreeMap Fiord, Gaode, Gaode Satellite, Esri World Imagery), in this order. More can be listed in `geo/gpx/tiles.json`, as `{"tiles": [...]}`, so a long list does not crowd this file. | empty — the built-in maps only |
| `gpx.tiles[].name` | The name shown in the base map selector. Required. | — |
| `gpx.tiles[].url` | A raster tile URL template. It must contain `{z}`, `{x}` and `{y}`; `{s}` is replaced by the subdomains `a`, `b` and `c`. A configuration missing a placeholder is refused. | — |
| `gpx.tiles[].attribution` | The credit shown on the map. Tile providers usually require one. | empty |
| `gpx.tiles[].max_zoom` | The deepest zoom the provider serves. | `19` |
| `gpx.tiles[].coordinates` | The system the tiles are drawn in: `wgs84`, or `gcj02` for maps published in China (Gaode, Tencent). The page's GCJ-02 *Auto* converts tracks for a `gcj02` map. Any other value is refused. | `wgs84` |
| `gpx.amap_key` | An Amap (高德) Web Service key. With it the page routes along Amap's roads as well as OpenStreetMap's. | empty — OpenStreetMap routing only |
| `gpx.dem.url` | The raster elevation tiles contour lines are drawn from, a template holding `{z}`, `{x}` and `{y}`. | AWS's public Terrarium tiles |
| `gpx.dem.encoding` | How those tiles encode height: `terrarium` or `mapbox` (terrain-RGB). | `terrarium` |
| `gpx.dem.max_zoom` | The deepest zoom the elevation tiles are served at. `0` uses the default. | `15` |

What GPX does with these is in [`apps/geo/gpx.md`](../apps/geo/gpx.md).
