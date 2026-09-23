// The 3D view: the same elevation (DEM) tiles the contour lines are drawn
// from, read as terrain, so the map is a relief the tracks lie on. The
// exaggeration is vertical only — positions do not move, so a track still
// falls where it was recorded.
//
// Terrain alone tilts the ground but barely shows it: a slope is only read by
// the way it is lit. So the relief comes with a hillshade layer drawn from
// the same tiles, under the tracks, which is what makes a ridge look like a
// ridge from above.

const SOURCE = "terrain-dem";
const HILLSHADE = "terrain-hillshade";

// DEFAULT_EXAGGERATION is how much the relief is stretched vertically. Real
// scale reads flat at the zooms a walk is looked at, so the view starts
// stretched; 1 is true to the ground.
export const DEFAULT_EXAGGERATION = 2.5;

// PITCH is the angle the map tilts to when the view is turned on, and MAX_PITCH
// how far it may be tilted by hand — further than MapLibre's own 60°, because
// a relief is worth looking across.
const PITCH = 60;
const MAX_PITCH = 80;

// DEM_MAX_ZOOM is the deepest zoom terrain tiles are asked for. The public
// Terrarium tiles thin out past it, and a missing tile reads as flat ground,
// so the deeper zooms are stretched from this one instead.
const DEM_MAX_ZOOM = 12;

export class Terrain {
  // dem is the server's elevation source { url, encoding, maxZoom }; under()
  // answers the id of the layer the hillshade goes beneath, so it stays under
  // the tracks.
  constructor(map, dem, { under } = {}) {
    this.map = map;
    this.dem = dem;
    this.under = under;
    this.exaggeration = DEFAULT_EXAGGERATION;
    this.on = false;
  }

  // show turns the relief on or off, tilting the map with it. A map already
  // tilted by hand keeps its angle when the view is turned off.
  show(on, { tilt = true } = {}) {
    const { map } = this;
    this.on = Boolean(on);
    if (!this.on) {
      map.setTerrain(null);
      if (map.getLayer(HILLSHADE)) map.removeLayer(HILLSHADE);
      if (tilt && map.getPitch() > 0) map.easeTo({ pitch: 0, duration: 300 });
      map.setMaxPitch(60);
      return;
    }
    if (!map.getSource(SOURCE)) {
      map.addSource(SOURCE, {
        type: "raster-dem",
        tiles: [this.dem.url],
        tileSize: 256,
        maxzoom: Math.min(this.dem.maxZoom || DEM_MAX_ZOOM, DEM_MAX_ZOOM),
        encoding: this.dem.encoding,
      });
    }
    map.setTerrain({ source: SOURCE, exaggeration: this.exaggeration });
    this.drawHillshade();
    map.setMaxPitch(MAX_PITCH);
    if (tilt && map.getPitch() < 1) map.easeTo({ pitch: PITCH, duration: 400 });
  }

  // drawHillshade adds the shading, or moves it back under the tracks after
  // the layers above it changed.
  drawHillshade() {
    const { map } = this;
    const under = this.under?.();
    if (!map.getLayer(HILLSHADE)) {
      map.addLayer({
        id: HILLSHADE,
        type: "hillshade",
        source: SOURCE,
        paint: {
          "hillshade-exaggeration": this.shading(),
          "hillshade-shadow-color": "#000000",
          "hillshade-highlight-color": "#ffffff",
        },
      }, under);
      return;
    }
    if (under) map.moveLayer(HILLSHADE, under);
  }

  // shading follows the vertical stretch, so turning the relief up shows more
  // of it, but never so much that the map underneath goes dark.
  shading() {
    return Math.min(0.55, 0.1 + this.exaggeration * 0.06);
  }

  // setExaggeration stretches the relief; the value is kept while the view is
  // off, so turning it on again reads the same.
  setExaggeration(value) {
    this.exaggeration = Math.min(8, Math.max(1, Number(value) || DEFAULT_EXAGGERATION));
    if (!this.on) return;
    this.map.setTerrain({ source: SOURCE, exaggeration: this.exaggeration });
    if (this.map.getLayer(HILLSHADE)) {
      this.map.setPaintProperty(HILLSHADE, "hillshade-exaggeration", this.shading());
    }
  }
}
