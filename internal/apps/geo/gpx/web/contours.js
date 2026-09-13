// Contour lines, drawn in the browser from raster elevation (DEM) tiles by
// maplibre-contour. The DEM is WGS-84. The map stack places and orders them.

const SOURCE = "contours";
const LINES = "contour-lines";
const MAJOR = "contour-lines-major";

// Thresholds per zoom: [minor, major] interval in metres.
const THRESHOLDS = {
  9: [200, 1000],
  11: [100, 500],
  12: [50, 250],
  13: [20, 100],
  14: [10, 50],
};

const OPACITY = { [LINES]: 0.55, [MAJOR]: 0.75 };

export class Contours {
  constructor(map, dem) {
    this.map = map;
    this.dem = dem;
    this.source = null;
  }

  layerIds() {
    return this.map.getLayer(LINES) ? [LINES, MAJOR] : [];
  }

  // show adds or removes the layers, at the given opacity (0–1).
  show(on, opacity = 1) {
    const present = Boolean(this.map.getLayer(LINES));
    if (!on) {
      if (!present) return;
      for (const id of [MAJOR, LINES]) this.map.removeLayer(id);
      this.map.removeSource(SOURCE);
      return;
    }
    if (!present) this.add();
    for (const id of [LINES, MAJOR]) this.map.setPaintProperty(id, "line-opacity", OPACITY[id] * opacity);
  }

  add() {
    if (!this.source) {
      this.source = new mlcontour.DemSource({
        url: this.dem.url,
        encoding: this.dem.encoding,
        maxzoom: this.dem.maxZoom,
        worker: true,
      });
      this.source.setupMaplibre(maplibregl);
    }
    this.map.addSource(SOURCE, {
      type: "vector",
      tiles: [this.source.contourProtocolUrl({ thresholds: THRESHOLDS, contourLayer: "contours", elevationKey: "ele", levelKey: "level" })],
      maxzoom: 15,
    });
    const line = (id, level, width) => ({
      id, type: "line", source: SOURCE, "source-layer": "contours", minzoom: 9,
      filter: ["==", ["get", "level"], level],
      layout: { "line-join": "round" },
      paint: { "line-color": "#8a5a2b", "line-width": width },
    });
    this.map.addLayer(line(LINES, 0, 0.6));
    this.map.addLayer(line(MAJOR, 1, 1.3));
  }
}
