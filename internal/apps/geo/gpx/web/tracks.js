import { addTrace, setTraceState, setTraceColor, traceLayers, directionColor, addDirection, TRACE_WIDTHS } from "./trace.js";

// Track layers on the map. Each workspace file draws its recorded tracks as
// lines (the focused file on top with a casing), its planned routes as dashed
// lines and its waypoints as dots; any part can be hidden on its own. A marker
// shows the point being inspected.

const CURSOR = "cursor";
export const CAMERA_IMAGE = "waypoint-camera";

// cameraImage draws Lucide's camera in white, at twice the size it is shown,
// for the map to place over a waypoint with photos. Path2D draws it at once,
// so a style that loses its images can be given it again when it asks.
export function cameraImage() {
  const size = 40, canvas = document.createElement("canvas");
  canvas.width = canvas.height = size;
  const g = canvas.getContext("2d");
  g.scale(size / 24, size / 24);
  g.strokeStyle = "#fff"; g.lineWidth = 2.2; g.lineCap = g.lineJoin = "round";
  g.stroke(new Path2D("M13.997 4a2 2 0 0 1 1.76 1.05l.486.9A2 2 0 0 0 18.003 7H20a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2h1.997a2 2 0 0 0 1.759-1.048l.489-.904A2 2 0 0 1 10.004 4z"));
  g.beginPath(); g.arc(12, 13, 3, 0, Math.PI * 2); g.stroke();
  return g.getImageData(0, 0, size, size);
}

export class TrackLayers {
  constructor(map) {
    this.map = map;
    this.ids = new Set();
    this.hidden = new Set(); // files hidden as a whole
    this.hiddenParts = new Map(); // file id -> keys of hidden parts
    this.focused = null;
    this.pointsVisible = false;
    this.joinGaps = false;
    this.hover = null; // the waypoint under the pointer, { id, key }
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
    // promoteId makes a part's key its feature id, so a waypoint under the
    // pointer is marked by feature state rather than redrawn.
    map.addSource(sourceId(id), { type: "geojson", data: geometry(track), promoteId: "part" });
    const kind = (name) => ["==", ["get", "kind"], name];
    addTrace(map, { id: lineId(id), source: sourceId(id), color, filter: kind("track"), metadata: { track: id }, before: CURSOR });
    addTrace(map, { id: routeId(id), source: sourceId(id), color, filter: kind("route"), metadata: { track: id }, dashed: true, before: CURSOR });
    map.addLayer({
      id: waypointId(id),
      type: "circle",
      source: sourceId(id),
      filter: kind("waypoint"),
      metadata: { track: id },
      paint: {
        // A waypoint under the pointer grows a little and thickens its ring:
        // enough to answer the pointer, not enough to move the map's reading.
        "circle-radius": ["case", ["boolean", ["feature-state", "hover"], false], withPhotos(11, 7), withPhotos(9, 5)],
        // A waypoint with photos is filled with the file's colour and ringed
        // in white, the reverse of the plain dot.
        "circle-color": withPhotos(color, "#ffffff"),
        "circle-stroke-color": withPhotos("#ffffff", color),
        "circle-stroke-width": ["case", ["boolean", ["feature-state", "hover"], false], 3.5, 2.5],
      },
    });
    // A waypoint with photos carries a camera over its dot.
    map.addLayer({
      id: photoId(id),
      type: "symbol",
      source: sourceId(id),
      filter: ["all", kind("waypoint"), ["boolean", ["get", "photos"], false]],
      metadata: { track: id },
      layout: { "icon-image": CAMERA_IMAGE, "icon-size": 0.5, "icon-allow-overlap": true, "icon-ignore-placement": true },
    });
    map.addLayer({ id: pointId(id), type: "circle", source: sourceId(id),
      filter: kind("track-point"), metadata: { track: id },
      layout: { visibility: this.pointsVisible ? "visible" : "none" },
      paint: { "circle-radius": 2.5, "circle-color": color, "circle-stroke-color": "#ffffff", "circle-stroke-width": 0.75 },
    }, CURSOR);
    addDirection(map, { id: directionId(id), source: sourceId(id), color, filter: kind("track"), before: CURSOR });
    map.addLayer({ id: gapId(id), type: "line", source: sourceId(id), filter: kind("track-gap"),
      layout: { visibility: this.joinGaps ? "visible" : "none" },
      paint: { "line-color": color, "line-width": 1.5 },
    }, CURSOR);
    // The thin link has no core to sit on, so its arrows are drawn in the
    // file's own colour, wider than the line and spaced further apart.
    map.addLayer({ id: gapDirectionId(id), type: "symbol", source: sourceId(id), filter: kind("track-gap"),
      layout: {
        visibility: this.joinGaps ? "visible" : "none",
        "symbol-placement": "line", "symbol-spacing": GAP_ARROW_SPACING,
        "icon-image": "track-direction", "icon-rotation-alignment": "map",
        "icon-keep-upright": false, "icon-size": GAP_ARROW_SIZE / 12,
        "icon-allow-overlap": true, "icon-ignore-placement": true,
      },
      paint: { "icon-color": color },
    }, CURSOR);
    this.ids.add(id);
    if (this.pointsVisible) this.applyTrackStyle(id);
  }

  // update redraws a file whose positions changed, as when converted to GCJ-02.
  update(id, track, hidden = []) {
    this.map.getSource(sourceId(id))?.setData(geometry(track, hidden));
  }

  remove(id) {
    const { map } = this;
    for (const layer of layerIds(id)) map.removeLayer(layer);
    map.removeSource(sourceId(id));
    this.ids.delete(id);
    this.hidden.delete(id);
    this.hiddenParts.delete(id);
    if (this.focused === id) this.focused = null;
    if (this.hover?.id === id) this.hover = null;
  }

  // setHover marks one waypoint as the one under the pointer, or clears the
  // mark with a null key. Only one waypoint is marked at a time.
  setHover(id, key) {
    if (this.hover && (this.hover.id !== id || this.hover.key !== key)) {
      if (this.map.getSource(sourceId(this.hover.id))) {
        this.map.setFeatureState({ source: sourceId(this.hover.id), id: this.hover.key }, { hover: false });
      }
      this.hover = null;
    }
    if (key == null || id == null || !this.ids.has(id)) return;
    this.map.setFeatureState({ source: sourceId(id), id: key }, { hover: true });
    this.hover = { id, key };
  }

  setColor(id, color) {
    setTraceColor(this.map, lineId(id), color);
    this.map.setPaintProperty(gapId(id), "line-color", color);
    this.map.setPaintProperty(gapDirectionId(id), "icon-color", color);
    this.map.setPaintProperty(directionId(id), "icon-color", directionColor(color));
    setTraceColor(this.map, routeId(id), color);
    this.map.setPaintProperty(pointId(id), "circle-color", color);
    this.map.setPaintProperty(waypointId(id), "circle-color", withPhotos(color, "#ffffff"));
    this.map.setPaintProperty(waypointId(id), "circle-stroke-color", withPhotos("#ffffff", color));
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
    for (const [layer, kind] of [...traceLayers(lineId(id)).map(layer => [layer, "track"]), ...traceLayers(routeId(id)).map(layer => [layer, "route"]), [waypointId(id), "waypoint"], [pointId(id), "track-point"], [directionId(id), "track"], [gapId(id), "track-gap"], [gapDirectionId(id), "track-gap"]]) {
      this.map.setFilter(layer, ["all", ["==", ["get", "kind"], kind], shown]);
    }
    this.map.setFilter(photoId(id), ["all", ["==", ["get", "kind"], "waypoint"], ["boolean", ["get", "photos"], false], shown]);
  }

  applyVisibility(id) {
    const visible = !this.hidden.has(id);
    for (const layer of layerIds(id)) {
      this.map.setLayoutProperty(layer, "visibility", visible && (layer !== pointId(id) || this.pointsVisible) && ((layer !== gapId(id) && layer !== gapDirectionId(id)) || this.joinGaps) ? "visible" : "none");
    }
  }

  setJoinGaps(on) {
    this.joinGaps = on;
    for (const id of this.ids) this.applyVisibility(id);
  }

  setPointsVisible(on) {
    this.pointsVisible = on;
    for (const id of this.ids) {
      this.applyVisibility(id);
      this.applyTrackStyle(id);
    }
  }

  applyTrackStyle(id) {
    const state = id === this.focused ? "focused" : "inactive";
    setTraceState(this.map, lineId(id), state, { points: this.pointsVisible });
    this.map.setLayoutProperty(directionId(id), "icon-size", (this.pointsVisible ? 1 : TRACE_WIDTHS[state][1]) / 12);
    // With one track of a file selected, its other tracks step back, so the
    // line the pointer follows is the one that stands out.
    const key = this.selectedPart?.id === id ? this.selectedPart.key : null;
    const opacity = key == null ? 1 : ["case", ["==", ["get", "part"], key], 1, UNSELECTED_OPACITY];
    for (const layer of traceLayers(lineId(id))) this.map.setPaintProperty(layer, "line-opacity", opacity);
    this.map.setPaintProperty(directionId(id), "icon-opacity", opacity);
  }

  // setSelectedPart marks one track of a file as selected, or with a null
  // key none.
  setSelectedPart(id, key) {
    const previous = this.selectedPart?.id;
    this.selectedPart = key == null ? null : { id, key };
    for (const other of new Set([previous, id])) if (other != null && this.ids.has(other)) this.applyTrackStyle(other);
  }

  // setFocus draws one file above the others, wider and opaque.
  setFocus(id) {
    const { map } = this;
    this.focused = id;
    for (const other of this.ids) {
      const focused = other === id;
      this.applyVisibility(other);
      this.applyTrackStyle(other);
      setTraceState(map, routeId(other), focused ? "focused" : "inactive", { dashed: true });
    }
    if (id != null && this.ids.has(id)) {
      for (const layer of layerIds(id).filter(layer => layer !== waypointId(id) && layer !== photoId(id))) map.moveLayer(layer, CURSOR);
      map.moveLayer(waypointId(id));
      map.moveLayer(photoId(id));
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
  // cleaning removed. The pointer may rest on the line between two sparse
  // points: a kept step within radius answers whichever end is nearer the
  // pointer's foot on it. Steps wholly to one side of the geographic box
  // around the pointer are skipped so a long track does not project every
  // point. Recording breaks are never treated as steps.
  nearest(track, screenPoint, radius = 24, hiddenParts = new Set(), { preferred = -1, hiddenRanges = [] } = {}) {
    const { map } = this;
    const corners = [
      map.unproject([screenPoint.x - radius, screenPoint.y - radius]),
      map.unproject([screenPoint.x + radius, screenPoint.y + radius]),
    ];
    const west = Math.min(corners[0].lng, corners[1].lng), east = Math.max(corners[0].lng, corners[1].lng);
    const south = Math.min(corners[0].lat, corners[1].lat), north = Math.max(corners[0].lat, corners[1].lat);
    const skipped = track.parts.filter((part) => part.kind === "track" && hiddenParts.has(part.key));
    const points = track.points;
    const hidden = (i) => hiddenRanges.some(([first, last]) => i >= first && i <= last);
    const visible = (i) => !track.removed[i] && !hidden(i) && !skipped.some((part) => i >= part.first && i <= part.last);
    const inside = ([lon, lat]) => lon >= west && lon <= east && lat >= south && lat <= north;
    const apart = ([lon1, lat1], [lon2, lat2]) =>
      (lon1 < west && lon2 < west) || (lon1 > east && lon2 > east) || (lat1 < south && lat2 < south) || (lat1 > north && lat2 > north);
    let best = -1, bestDistance = radius * radius;
    const offer = (i, d) => {
      if (d < bestDistance || (d === bestDistance && (best < 0 || preferred < 0 || Math.abs(i - preferred) < Math.abs(best - preferred)))) { bestDistance = d; best = i; }
    };
    let previous = -1;
    for (let i = 0; i < points.length; i++) {
      if (!visible(i)) {
        if (!track.removed[i] || hidden(i)) previous = -1; // a hidden part breaks the line
        continue;
      }
      if (previous >= 0 && track.segments[i] === track.segments[previous] && !apart(points[previous], points[i])) {
        const a = map.project(points[previous]), b = map.project(points[i]);
        const dx = b.x - a.x, dy = b.y - a.y, length = dx * dx + dy * dy;
        const t = length ? Math.max(0, Math.min(1, ((screenPoint.x - a.x) * dx + (screenPoint.y - a.y) * dy) / length)) : 0;
        const d = (a.x + t * dx - screenPoint.x) ** 2 + (a.y + t * dy - screenPoint.y) ** 2;
        offer(t < 0.5 ? previous : i, d);
      } else if (inside(points[i])) {
        const p = map.project(points[i]);
        offer(i, (p.x - screenPoint.x) ** 2 + (p.y - screenPoint.y) ** 2);
      }
      previous = i;
    }
    return best;
  }
}

const sourceId = (id) => `track-${id}`;
const lineId = (id) => `track-${id}-line`;
const routeId = (id) => `track-${id}-route`;
const waypointId = (id) => `track-${id}-waypoint`;
const pointId = (id) => `track-${id}-points`;
const directionId = (id) => `track-${id}-direction`;
const photoId = (id) => `track-${id}-photo`;
const gapId = (id) => `track-${id}-gap`;
const gapDirectionId = (id) => `track-${id}-gap-direction`;
const UNSELECTED_OPACITY = 0.3; // a selected track's siblings in its file
const GAP_ARROW_SIZE = 8; // pixels across an arrow on a gap link
const GAP_ARROW_SPACING = 60; // pixels between arrows on a gap link
const layerIds = (id) => [...traceLayers(lineId(id)), ...traceLayers(routeId(id)), waypointId(id), photoId(id), pointId(id), directionId(id), gapId(id), gapDirectionId(id)];

function emptyCollection() {
  return { type: "FeatureCollection", features: [] };
}

// geometry is one feature per part. A track draws each recorded segment
// separately, so a gap in the recording is not bridged by a straight line.
function geometry(track, hidden = []) {
  const features = [];
  for (const part of track.parts) {
    const properties = { part: part.key, kind: part.kind, name: part.name, photos: Boolean(part.photos?.length) };
    if (part.kind === "track") {
      const lines = [], gaps = [];
      let current = [];
      // Points cleaning removed are left out; the line joins what was kept.
      let previous = -1;
      for (let i = part.first; i <= part.last; i++) {
        if (hidden.some(([first, last]) => i >= first && i <= last)) {
          if (current.length > 1) lines.push(current);
          current = []; previous = -1;
          continue;
        }
        if (track.removed[i]) continue;
        if (previous >= 0 && track.segments[i] !== track.segments[previous]) {
          gaps.push([track.points[previous], track.points[i]]);
          lines.push(current);
          current = [];
        }
        current.push(track.points[i]);
        previous = i;
      }
      if (current.length) lines.push(current);
      features.push({ type: "Feature", properties, geometry: { type: "MultiLineString", coordinates: lines } });
      features.push({ type: "Feature", properties: { ...properties, kind: "track-gap" }, geometry: { type: "MultiLineString", coordinates: gaps } });
      features.push({ type: "Feature", properties: { ...properties, kind: "track-point" }, geometry: { type: "MultiPoint", coordinates: lines.flat() } });
    } else if (part.kind === "route") {
      features.push({ type: "Feature", properties, geometry: { type: "LineString", coordinates: part.points } });
    } else {
      features.push({ type: "Feature", properties, geometry: { type: "Point", coordinates: part.points[0] } });
    }
  }
  return { type: "FeatureCollection", features };
}

function withPhotos(yes, no) {
  return ["case", ["boolean", ["get", "photos"], false], yes, no];
}
