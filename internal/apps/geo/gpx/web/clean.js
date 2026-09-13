// Cleaning the focused track: the panel of filters and their settings, the
// before/after overlay on the map, and lasso selection for removing points by
// hand. The server runs every filter; this module edits settings, draws the
// result and turns a lasso into point indices on screen.

const ORIGINAL = "clean-original";
const REMOVED = "clean-removed";

// RULE_COLORS tell apart what removed a point.
export const RULE_COLORS = { spike: "#d0021b", drift: "#f5a623", stop: "#7b7b76", manual: "#8e44ad" };

// FILTERS describe the panel: each filter's settings, in the sidecar's names,
// with the unit and the scale between what is typed and what is stored.
const FILTERS = [
  {
    key: "spikes", title: "Sudden jumps", rule: "spike",
    help: "A fix reached and left impossibly fast, or a sharp turn out to a far point and straight back.",
    fields: [
      { key: "maxSpeed", label: "Max speed", unit: "km/h", scale: 3.6 },
      { key: "maxAcceleration", label: "Max acceleration", unit: "m/s²" },
      { key: "turnAngle", label: "Turn back", unit: "°" },
      { key: "minJump", label: "Min jump", unit: "m" },
    ],
  },
  {
    key: "drift", title: "Speed drift", rule: "drift",
    help: "Hampel filter: a speed far above the rolling median of its neighbours.",
    fields: [
      { key: "halfWindow", label: "Neighbours each side", unit: "", step: 1 },
      { key: "threshold", label: "Threshold", unit: "× MAD" },
      { key: "minDeviation", label: "Min spread", unit: "km/h", scale: 3.6 },
      { key: "quality", label: "Stricter for poor HDOP / few satellites", type: "checkbox" },
    ],
  },
  {
    key: "smooth", title: "Position jitter", rule: "moved",
    help: "Kalman filter with RTS smoothing. Moves points instead of removing them.",
    fields: [
      { key: "positionSigma", label: "Fix error", unit: "m" },
      { key: "acceleration", label: "Acceleration", unit: "m/s²" },
      { key: "maxGap", label: "Restart after a pause", unit: "s" },
    ],
  },
  {
    key: "stops", title: "Scribbles in stops", rule: "stop",
    help: "Points inside a stop collapse to its centre, keeping where it was entered and left.",
    fields: [
      { key: "distance", label: "Within", unit: "m" },
      { key: "duration", label: "At least", unit: "min", scale: 1 / 60 },
    ],
  },
];

export class CleanPanel {
  // onChange(params) is called with the new settings after an edit;
  // onLasso(active) when lasso mode is switched; onCompare(on) likewise.
  constructor({ root, onChange, onLasso, onCompare }) {
    Object.assign(this, { root, onChange, onLasso, onCompare });
    this.track = null;
    this.lasso = false;
    this.compare = true;
  }

  show(track) {
    this.track = track;
    this.render();
  }

  hide() {
    this.track = null;
    this.root.replaceChildren();
  }

  setLasso(active) {
    this.lasso = active;
    this.render();
  }

  render() {
    const { track } = this;
    if (!track) return;
    const { params, counts } = track.clean;
    const parts = [];

    const head = document.createElement("div");
    head.className = "clean-head";
    const title = document.createElement("strong");
    title.textContent = "Clean";
    const compare = checkbox("Compare with the recording", this.compare, (on) => {
      this.compare = on;
      this.onCompare(on);
    });
    head.append(title, compare);
    parts.push(head);

    for (const filter of FILTERS) {
      const settings = params[filter.key];
      const section = document.createElement("section");
      section.className = "clean-filter" + (settings.enabled ? " enabled" : "");
      const header = document.createElement("div");
      header.className = "clean-filter-head";
      const toggle = checkbox(filter.title, settings.enabled, (on) => this.edit(filter.key, "enabled", on));
      toggle.title = filter.help;
      const badge = document.createElement("span");
      badge.className = "clean-count";
      const count = counts[filter.rule];
      if (settings.enabled && count) {
        badge.textContent = filter.rule === "moved" ? `${count} moved` : `${count} removed`;
        if (filter.rule !== "moved") badge.style.color = RULE_COLORS[filter.rule];
      }
      header.append(toggle, badge);
      section.append(header);
      if (settings.enabled) {
        const grid = document.createElement("div");
        grid.className = "clean-fields";
        for (const field of filter.fields) grid.append(this.field(filter.key, field, settings[field.key]));
        section.append(grid);
      }
      parts.push(section);
    }

    const manual = document.createElement("section");
    manual.className = "clean-filter enabled";
    const manualHead = document.createElement("div");
    manualHead.className = "clean-filter-head";
    const lasso = document.createElement("button");
    lasso.className = "chip" + (this.lasso ? " active" : "");
    lasso.setAttribute("aria-pressed", String(this.lasso));
    lasso.textContent = this.lasso ? "Lasso on — Esc to stop" : "Lasso";
    lasso.title = "Draw around points on the map to remove them. Hold Alt (⌥) while drawing to restore points removed by hand.";
    lasso.addEventListener("click", () => this.onLasso(!this.lasso));
    const manualCount = document.createElement("span");
    manualCount.className = "clean-count";
    manualCount.style.color = RULE_COLORS.manual;
    manualCount.textContent = params.removed.length ? `${params.removed.length} removed by hand` : "";
    manualHead.append(lasso, manualCount);
    manual.append(manualHead);
    if (params.removed.length) {
      const restore = document.createElement("button");
      restore.className = "text-button";
      restore.textContent = "Restore all removed by hand";
      restore.addEventListener("click", () => this.onChange({ ...params, removed: [] }));
      manual.append(restore);
    }
    parts.push(manual);

    const foot = document.createElement("p");
    foot.className = "clean-foot";
    if (track.clean.error) {
      foot.classList.add("error");
      foot.textContent = `Sidecar not read: ${track.clean.error}`;
    } else {
      foot.textContent = track.clean.sidecar
        ? `Saved beside the source: ${basename(track.clean.sidecar)}`
        : "Nothing cleaned. The source GPX is never changed.";
      foot.title = track.clean.sidecar || "";
    }
    parts.push(foot);
    this.root.replaceChildren(...parts);
  }

  field(filterKey, field, value) {
    const label = document.createElement("label");
    label.className = "clean-field" + (field.type === "checkbox" ? " wide" : "");
    if (field.type === "checkbox") {
      const input = document.createElement("input");
      input.type = "checkbox";
      input.checked = Boolean(value);
      input.addEventListener("change", () => this.edit(filterKey, field.key, input.checked));
      label.append(input, document.createTextNode(` ${field.label}`));
      return label;
    }
    const scale = field.scale || 1;
    const name = document.createElement("span");
    name.textContent = field.label;
    const input = document.createElement("input");
    input.type = "number";
    input.min = "0";
    input.step = field.step || "any";
    input.value = String(round(value * scale));
    input.addEventListener("change", () => {
      const typed = Number(input.value);
      if (!(typed > 0)) {
        input.value = String(round(value * scale));
        return;
      }
      this.edit(filterKey, field.key, field.step === 1 ? Math.round(typed) : typed / scale);
    });
    const unit = document.createElement("span");
    unit.className = "unit";
    unit.textContent = field.unit;
    label.append(name, input, unit);
    return label;
  }

  edit(filterKey, fieldKey, value) {
    const params = structuredClone(this.track.clean.params);
    params[filterKey][fieldKey] = value;
    this.onChange(params);
  }
}

// CleanOverlay draws, under the focused track, the recording as it was: its
// original line and the points cleaning removed, coloured by rule.
export class CleanOverlay {
  constructor(map, beforeLayer) {
    this.map = map;
    map.addSource(ORIGINAL, { type: "geojson", data: empty() });
    map.addSource(REMOVED, { type: "geojson", data: empty() });
    map.addLayer({
      id: ORIGINAL,
      type: "line",
      source: ORIGINAL,
      paint: { "line-color": "#555550", "line-width": 1.2, "line-opacity": 0.55, "line-dasharray": [2, 2] },
    }, beforeLayer);
    map.addLayer({
      id: REMOVED,
      type: "circle",
      source: REMOVED,
      paint: {
        "circle-radius": 3.5,
        "circle-color": ["get", "color"],
        "circle-stroke-color": "#ffffff",
        "circle-stroke-width": 1,
      },
    }, beforeLayer);
  }

  // show draws a track's recording, or nothing for null or an uncleaned track.
  show(track, hiddenRanges = []) {
    const cleaned = track && track.removed.some(Boolean) || track?.original;
    if (!cleaned) {
      this.clear();
      return;
    }
    const hidden = (i) => hiddenRanges.some(([first, last]) => i >= first && i <= last);
    const positions = track.original || track.points;
    const lines = [];
    let current = [];
    positions.forEach((point, i) => {
      if (hidden(i) || (i > 0 && track.segments[i] !== track.segments[i - 1])) {
        if (current.length > 1) lines.push(current);
        current = [];
      }
      if (!hidden(i)) current.push(point);
    });
    if (current.length > 1) lines.push(current);
    this.map.getSource(ORIGINAL).setData({ type: "Feature", properties: {}, geometry: { type: "MultiLineString", coordinates: lines } });
    const features = [];
    track.removed.forEach((rule, i) => {
      if (!rule || hidden(i)) return;
      features.push({ type: "Feature", properties: { color: RULE_COLORS[rule], rule }, geometry: { type: "Point", coordinates: positions[i] } });
    });
    this.map.getSource(REMOVED).setData({ type: "FeatureCollection", features });
  }

  clear() {
    this.map.getSource(ORIGINAL).setData(empty());
    this.map.getSource(REMOVED).setData(empty());
  }
}

// Lasso lets the pointer draw a closed shape over the map. onDone(points,
// { restore }) receives the shape in screen pixels; restore is true when Alt
// was held.
export class Lasso {
  constructor(map, { onDone, onCancel }) {
    Object.assign(this, { map, onDone, onCancel });
    this.active = false;
    const container = map.getCanvasContainer();
    this.svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    this.svg.classList.add("lasso");
    this.path = document.createElementNS("http://www.w3.org/2000/svg", "path");
    this.svg.append(this.path);
    container.append(this.svg);
    this.onKey = (event) => {
      if (event.key === "Escape") this.onCancel();
    };
    this.onDown = (event) => this.down(event);
  }

  setActive(active) {
    if (active === this.active) return;
    this.active = active;
    const container = this.map.getCanvasContainer();
    container.classList.toggle("lassoing", active);
    if (active) {
      this.map.dragPan.disable();
      this.map.boxZoom.disable();
      // Capture on the outer container, ahead of the map's own pointer handling.
      this.map.getContainer().addEventListener("pointerdown", this.onDown, true);
      window.addEventListener("keydown", this.onKey);
    } else {
      this.map.dragPan.enable();
      this.map.boxZoom.enable();
      this.map.getContainer().removeEventListener("pointerdown", this.onDown, true);
      window.removeEventListener("keydown", this.onKey);
      this.path.removeAttribute("d");
    }
  }

  down(event) {
    // Map controls (zoom buttons, attribution) keep working while lassoing.
    if (event.button !== 0 || !this.map.getCanvasContainer().contains(event.target)) return;
    event.preventDefault();
    event.stopPropagation();
    const container = this.map.getCanvasContainer();
    const box = container.getBoundingClientRect();
    const points = [];
    const restore = event.altKey;
    this.svg.classList.toggle("restore", restore);
    try {
      container.setPointerCapture(event.pointerId);
    } catch {
      // Without capture the shape still closes when the pointer is released over the map.
    }
    const add = (e) => {
      points.push([e.clientX - box.left, e.clientY - box.top]);
      this.path.setAttribute("d", `M${points.map((p) => p.join(",")).join("L")}Z`);
    };
    add(event);
    const move = (e) => add(e);
    const up = () => {
      container.removeEventListener("pointermove", move);
      container.removeEventListener("pointerup", up);
      container.removeEventListener("pointercancel", up);
      this.path.removeAttribute("d");
      if (points.length > 2) this.onDone(points, { restore });
    };
    container.addEventListener("pointermove", move);
    container.addEventListener("pointerup", up);
    container.addEventListener("pointercancel", up);
  }
}

// insidePolygon reports whether a screen point lies inside a closed shape.
export function insidePolygon([x, y], polygon) {
  let inside = false;
  for (let i = 0, j = polygon.length - 1; i < polygon.length; j = i++) {
    const [xi, yi] = polygon[i];
    const [xj, yj] = polygon[j];
    if ((yi > y) !== (yj > y) && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi) inside = !inside;
  }
  return inside;
}

function checkbox(text, checked, onChange) {
  const label = document.createElement("label");
  label.className = "clean-check";
  const input = document.createElement("input");
  input.type = "checkbox";
  input.checked = checked;
  input.addEventListener("change", () => onChange(input.checked));
  label.append(input, document.createTextNode(` ${text}`));
  return label;
}

const round = (value) => Math.round(value * 100) / 100;
const basename = (path) => path.split(/[\\/]/).pop();

function empty() {
  return { type: "FeatureCollection", features: [] };
}
