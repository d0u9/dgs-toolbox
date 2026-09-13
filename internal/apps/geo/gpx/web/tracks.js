// Track layers on the map. Each workspace file draws its recorded tracks as
// lines (the focused file on top with a casing), its planned routes as dashed
// lines and its waypoints as dots; any part can be hidden on its own. A marker
// shows the point being inspected.

const CURSOR = "cursor";

export class TrackLayers {
  constructor(map) {
    this.map = map;
    this.ids = new Set();
    this.hidden = new Set(); // files hidden as a whole
    this.hiddenParts = new Map(); // file id -> keys of hidden parts
    this.focused = null;
    map.addSource(CURSOR, { type: "geojson", data: emptyCollection() });
    map.addLayer({
      id: CURSOR,
      type: "circle",
      source: CURSOR,
      paint: {
        "circle-radius": 6,
        "circle-color": ["get", "color"],
        "circle-stroke-color": "#ffffff",
        "circle-stroke-width": 2.5,
      },
    });
  }

  // lineLayers are the ids clicks on the map are tested against to focus a file.
  lineLayers() {
    return [...this.ids].flatMap((id) => [lineId(id), routeId(id)]);
  }

  // waypointLayers are the ids clicks are tested against to open a waypoint.
  waypointLayers() {
    return [...this.ids].map((id) => waypointId(id));
  }

  add(id, track, color) {
    const { map } = this;
    map.addSource(sourceId(id), { type: "geojson", data: geometry(track) });
    const kind = (name) => ["==", ["get", "kind"], name];
    map.addLayer({
      id: casingId(id),
      type: "line",
      source: sourceId(id),
      filter: kind("track"),
      layout: { "line-join": "round", "line-cap": "round", visibility: "none" },
      paint: { "line-color": "#ffffff", "line-width": 8 },
    }, CURSOR);
    map.addLayer({
      id: routeId(id),
      type: "line",
      source: sourceId(id),
      filter: kind("route"),
      metadata: { track: id },
      layout: { "line-join": "round" },
      paint: { "line-color": color, "line-width": 2, "line-opacity": 0.8, "line-dasharray": [2, 1.5] },
    }, CURSOR);
    map.addLayer({
      id: lineId(id),
      type: "line",
      source: sourceId(id),
      filter: kind("track"),
      metadata: { track: id },
      layout: { "line-join": "round", "line-cap": "round" },
      paint: { "line-color": color, "line-width": 3, "line-opacity": 0.8 },
    }, CURSOR);
    map.addLayer({
      id: waypointId(id),
      type: "circle",
      source: sourceId(id),
      filter: kind("waypoint"),
      metadata: { track: id },
      paint: { "circle-radius": 5, "circle-color": "#ffffff", "circle-stroke-color": color, "circle-stroke-width": 2.5 },
    }, CURSOR);
    this.ids.add(id);
  }

  // update redraws a file whose positions changed, as when converted to GCJ-02.
  update(id, track) {
    this.map.getSource(sourceId(id))?.setData(geometry(track));
  }

  remove(id) {
    const { map } = this;
    for (const layer of layerIds(id)) map.removeLayer(layer);
    map.removeSource(sourceId(id));
    this.ids.delete(id);
    this.hidden.delete(id);
    this.hiddenParts.delete(id);
    if (this.focused === id) this.focused = null;
  }

  setColor(id, color) {
    this.map.setPaintProperty(lineId(id), "line-color", color);
    this.map.setPaintProperty(routeId(id), "line-color", color);
    this.map.setPaintProperty(waypointId(id), "circle-stroke-color", color);
  }

  // setVisible shows or hides a whole file; a hidden file keeps its layers.
  setVisible(id, visible) {
    if (visible) this.hidden.delete(id);
    else this.hidden.add(id);
    this.applyVisibility(id);
  }

  // setHiddenParts hides some of a file's tracks, routes and waypoints by key.
  setHiddenParts(id, keys) {
    this.hiddenParts.set(id, [...keys]);
    const shown = ["!", ["in", ["get", "part"], ["literal", [...keys]]]];
    for (const [layer, kind] of [[casingId(id), "track"], [lineId(id), "track"], [routeId(id), "route"], [waypointId(id), "waypoint"]]) {
      this.map.setFilter(layer, ["all", ["==", ["get", "kind"], kind], shown]);
    }
  }

  applyVisibility(id) {
    const visible = !this.hidden.has(id);
    for (const layer of [lineId(id), routeId(id), waypointId(id)]) {
      this.map.setLayoutProperty(layer, "visibility", visible ? "visible" : "none");
    }
    this.map.setLayoutProperty(casingId(id), "visibility", visible && this.focused === id ? "visible" : "none");
  }

  // setFocus draws one file above the others, wider and opaque.
  setFocus(id) {
    const { map } = this;
    this.focused = id;
    for (const other of this.ids) {
      const focused = other === id;
      this.applyVisibility(other);
      map.setPaintProperty(lineId(other), "line-width", focused ? 4.5 : 3);
      map.setPaintProperty(lineId(other), "line-opacity", focused ? 1 : 0.55);
      map.setPaintProperty(routeId(other), "line-opacity", focused ? 0.9 : 0.5);
    }
    if (id != null && this.ids.has(id)) {
      for (const layer of layerIds(id)) map.moveLayer(layer, CURSOR);
    }
  }

  setCursor(position, color) {
    const data = position
      ? { type: "Feature", properties: { color }, geometry: { type: "Point", coordinates: position } }
      : emptyCollection();
    this.map.getSource(CURSOR).setData(data);
  }

  // nearest finds the index of the track point closest to a screen point,
  // within radius pixels, or -1, skipping points of hidden tracks and points
  // cleaning removed. Points are first filtered by the geographic box around
  // the pointer so a long track does not project every point.
  nearest(track, screenPoint, radius = 24, hiddenParts = new Set()) {
    const { map } = this;
    const corners = [
      map.unproject([screenPoint.x - radius, screenPoint.y - radius]),
      map.unproject([screenPoint.x + radius, screenPoint.y + radius]),
    ];
    const west = Math.min(corners[0].lng, corners[1].lng), east = Math.max(corners[0].lng, corners[1].lng);
    const south = Math.min(corners[0].lat, corners[1].lat), north = Math.max(corners[0].lat, corners[1].lat);
    const skipped = track.parts.filter((part) => part.kind === "track" && hiddenParts.has(part.key));
    let best = -1, bestDistance = radius * radius;
    const points = track.points;
    for (let i = 0; i < points.length; i++) {
      const [lon, lat] = points[i];
      if (lon < west || lon > east || lat < south || lat > north) continue;
      if (track.removed[i] || skipped.some((part) => i >= part.first && i <= part.last)) continue;
      const p = map.project(points[i]);
      const d = (p.x - screenPoint.x) ** 2 + (p.y - screenPoint.y) ** 2;
      if (d <= bestDistance) {
        bestDistance = d;
        best = i;
      }
    }
    return best;
  }
}

const sourceId = (id) => `track-${id}`;
const lineId = (id) => `track-${id}-line`;
const casingId = (id) => `track-${id}-casing`;
const routeId = (id) => `track-${id}-route`;
const waypointId = (id) => `track-${id}-waypoint`;
const layerIds = (id) => [casingId(id), routeId(id), lineId(id), waypointId(id)];

function emptyCollection() {
  return { type: "FeatureCollection", features: [] };
}

// geometry is one feature per part. A track draws each recorded segment
// separately, so a gap in the recording is not bridged by a straight line.
function geometry(track) {
  const features = [];
  for (const part of track.parts) {
    const properties = { part: part.key, kind: part.kind, name: part.name };
    if (part.kind === "track") {
      const lines = [];
      let current = [];
      // Points cleaning removed are left out; the line joins what was kept.
      let previous = -1;
      for (let i = part.first; i <= part.last; i++) {
        if (track.removed[i]) continue;
        if (previous >= 0 && track.segments[i] !== track.segments[previous]) {
          lines.push(current);
          current = [];
        }
        current.push(track.points[i]);
        previous = i;
      }
      if (current.length) lines.push(current);
      features.push({ type: "Feature", properties, geometry: { type: "MultiLineString", coordinates: lines } });
    } else if (part.kind === "route") {
      features.push({ type: "Feature", properties, geometry: { type: "LineString", coordinates: part.points } });
    } else {
      features.push({ type: "Feature", properties, geometry: { type: "Point", coordinates: part.points[0] } });
    }
  }
  return { type: "FeatureCollection", features };
}
