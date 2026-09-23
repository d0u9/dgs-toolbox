// Editing the focused track: the panel for removing points by hand — with the
// lasso, as ranges on the timeline, or a whole stop — and the automatic
// cleaning filters and their settings; the before/after overlay on the map;
// and lasso selection. The server runs every filter; this module edits settings, draws the
// result and turns a lasso into point indices on screen.

import * as format from "./format.js";
import { waySelect } from "./route.js";

const ORIGINAL = "clean-original";
const REMOVED = "clean-removed";
const MANUAL = "clean-manual"; // stretches removed by hand, as a line
const PENDING = "clean-pending"; // a range's or fill's start, picked on the track
const FILLED = "clean-filled"; // stretches filled in along the road
const PREVIEW = "clean-fill-preview"; // a route not yet used

// RULE_COLORS tell apart what removed a point.
export const RULE_COLORS = { spike: "#d55e00", drift: "#e69f00", stop: "#666666", manual: "#6f4c9b", fill: "#56b4e9" };



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
  // onLasso(active) and onRangeTool(active) when those tools are switched;
  // onShowRange(range) to frame a removed range.
  // Filling along the road: onFillTool(active), onProfile(profile),
  // onUseFill() and onDiscardFill() for the previewed route, onRemoveFill(index)
  // and onShowFill(fill).
  // onTab(tab) is called when the tab is switched.
  constructor({ root, onChange, onLasso, onRangeTool, onShowRange, onFillTool, onProfile, onUseFill, onDiscardFill, onRemoveFill, onShowFill, onTab, timeZone }) {
    Object.assign(this, { root, onChange, onLasso, onRangeTool, onShowRange, onFillTool, onProfile, onUseFill, onDiscardFill, onRemoveFill, onShowFill, onTab });
    this.ways = []; // the routers' ways of travel, from /api/config
    this.tab = "changes"; // "changes": removals by hand and fills; "auto": the filters; "segments": cuts
    // segmentsRoot holds the Segments tab, drawn by the cut panel.
    this.segmentsRoot = document.createElement("div");
    this.segmentsRoot.className = "segments-tab";
    this.track = null;
    this.lasso = false;
    this.rangeTool = false;
    // fill is the fill tool's state: { active, profile, start, preview, busy, error }.
    this.fill = { active: false, profile: "car", start: null, preview: null, busy: false, error: null };
    this.compare = true;
    this.timeZone = timeZone || format.browserTimeZone();
  }

  setRangeTool(active) {
    this.rangeTool = active;
    this.render();
  }

  setFill(fill) {
    this.fill = fill;
    this.render();
  }

  // ---- removal by hand: a list of edits, oldest first ----

  // addEdit records one removal by hand; it is the next one undo takes back.
  addEdit(edit) {
    const params = structuredClone(this.track.clean.params);
    params.edits.push({ ...edit, at: Date.now() });
    this.onChange(params);
  }

  addRange(first, last, join = false) {
    this.addEdit({ kind: "range", first, last, join });
  }

  setJoin(index, join) {
    const params = structuredClone(this.track.clean.params);
    params.edits[index].join = join;
    this.onChange(params);
  }

  // restoreEdit takes back one edit, whenever it was made.
  restoreEdit(index) {
    const params = structuredClone(this.track.clean.params);
    params.edits.splice(index, 1);
    this.onChange(params);
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

  // undo takes back the latest edit.
  undo() {
    const { edits } = this.track?.clean.params || {};
    if (edits?.length) this.restoreEdit(edits.length - 1);
  }

  render() {
    const { track } = this;
    if (!track) return;
    const { params, counts } = track.clean;
    const parts = [];

    const head = document.createElement("div");
    head.className = "clean-head";
    const title = document.createElement("strong");
    title.textContent = track.name;
    title.title = track.name;
    head.append(title);
    parts.push(head);

    const autoRemoved = FILTERS.reduce((sum, filter) => sum + (params[filter.key].enabled && filter.rule !== "moved" ? counts[filter.rule] || 0 : 0), 0);
    const tabs = document.createElement("div");
    tabs.className = "inspector-tabs";
    tabs.setAttribute("role", "tablist");
    for (const [key, text, count] of [
      ["changes", "Changes", params.edits.length + track.fills.length],
      ["auto", "Automatic cleaning", autoRemoved],
      ["segments", "Segments", track.pieces.length > 1 ? track.pieces.length : 0],
    ]) {
      const tab = document.createElement("button");
      tab.className = "inspector-tab" + (this.tab === key ? " active" : "");
      tab.setAttribute("role", "tab");
      tab.setAttribute("aria-selected", String(this.tab === key));
      tab.textContent = text;
      if (count) {
        const badge = document.createElement("span");
        badge.className = "tab-count";
        badge.textContent = count;
        tab.append(badge);
      }
      tab.addEventListener("click", () => this.onTab(key));
      tabs.append(tab);
    }
    parts.push(tabs);

    const body = document.createElement("div");
    body.className = "inspector-body";
    if (this.tab === "changes") {
      body.append(this.manualSection(), this.fillSection());
    } else if (this.tab === "auto") {
      for (const filter of FILTERS) body.append(this.filterSection(filter));
    } else {
      body.append(this.segmentsRoot);
    }
    parts.push(body);

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

  filterSection(filter) {
    const { params, counts } = this.track.clean;
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
    const help = document.createElement("p");
    help.className = "cut-hint";
    help.textContent = filter.help;
    section.append(help);
    if (settings.enabled) {
      const grid = document.createElement("div");
      grid.className = "clean-fields";
      for (const field of filter.fields) grid.append(this.field(filter.key, field, settings[field.key]));
      section.append(grid);
    }
    return section;
  }

  manualSection() {
    const { params, counts } = this.track.clean;
    const section = document.createElement("section");
    section.className = "clean-filter enabled";
    const head = document.createElement("div");
    head.className = "clean-filter-head";
    const title = document.createElement("strong");
    title.textContent = "Remove by hand";
    const count = document.createElement("span");
    count.className = "clean-count";
    count.style.color = RULE_COLORS.manual;
    count.textContent = counts.manual ? `${counts.manual} removed` : "";
    const undo = document.createElement("button");
    undo.className = "text-button";
    undo.textContent = "Undo";
    undo.title = `Take back the latest removal (${format.keys.undo})`;
    undo.disabled = params.edits.length === 0;
    undo.addEventListener("click", () => this.undo());
    head.append(title, count, undo);
    section.append(head);

    if (!params.edits.length) {
      const hint = document.createElement("p");
      hint.className = "cut-hint";
      hint.textContent = "Nothing removed yet. Use Range or Lasso on the map's tool bar, drag on the timeline, or Remove this stop in a stop's popup.";
      section.append(hint);
    }

    if (params.edits.length) {
      const list = document.createElement("ol");
      list.className = "range-list";
      for (let i = params.edits.length - 1; i >= 0; i--) list.append(this.editRow(params.edits[i], i));
      section.append(list);
    }
    return section;
  }

  fillSection() {
    const { track, fill } = this;
    const section = document.createElement("section");
    section.className = "clean-filter enabled";
    const head = document.createElement("div");
    head.className = "clean-filter-head";
    const title = document.createElement("strong");
    title.textContent = "Fill along the road";
    const count = document.createElement("span");
    count.className = "clean-count";
    count.style.color = RULE_COLORS.fill;
    count.textContent = track.fills.length ? `${track.fills.length} filled` : "";
    head.append(title, count);
    section.append(head);

    const tools = document.createElement("div");
    tools.className = "clean-tools";
    const way = document.createElement("span");
    way.className = "muted";
    way.textContent = "Way of travel";
    const profile = waySelect(this.ways, fill.profile, (value) => this.onProfile(value));
    profile.title = "The way of travel the route follows";
    tools.append(way, profile);
    section.append(tools);

    const hint = document.createElement("p");
    hint.className = "cut-hint";
    if (fill.busy) hint.textContent = "Asking the router…";
    else if (fill.error) {
      hint.classList.add("error");
      hint.textContent = fill.error;
    } else if (fill.preview) {
      hint.textContent = "The route is drawn dashed on the map. Use it to replace the points between its ends.";
    } else if (fill.active) {
      hint.textContent = fill.start == null
        ? "Click the track where the stretch starts."
        : "Now click where it ends. Near a track's end, the click snaps to it.";
    }
    else hint.textContent = "Only the two end points are sent, to the service of the way chosen: OSRM on OpenStreetMap, or Amap when a key is set. The route's points get times spread by distance and are marked as filled.";
    section.append(hint);

    if (fill.preview) {
      const box = document.createElement("div");
      box.className = "fill-preview";
      const meta = document.createElement("span");
      meta.className = "meta";
      meta.textContent = `${format.distance(fill.preview.distance)} · ${fill.preview.route.length} points`;
      const use = document.createElement("button");
      use.className = "chip active";
      use.textContent = "Use this route";
      use.addEventListener("click", () => this.onUseFill());
      const discard = document.createElement("button");
      discard.className = "text-button";
      discard.textContent = "Discard";
      discard.addEventListener("click", () => this.onDiscardFill());
      box.append(meta, discard, use);
      section.append(box);
    }

    if (track.fills.length) {
      const list = document.createElement("ol");
      list.className = "range-list";
      for (let i = track.fills.length - 1; i >= 0; i--) list.append(this.fillRow(track.fills[i], i));
      section.append(list);
    }
    return section;
  }

  fillRow(fill, index) {
    const { track } = this;
    const li = document.createElement("li");
    li.className = "range-row";
    const label = document.createElement("span");
    label.className = "label";
    const name = document.createElement("span");
    name.className = "name";
    const from = track.time[fill.first], to = track.time[fill.last];
    const found = this.ways.find((w) => w.id === fill.profile);
    const way = found ? `${found.label} (${found.service})` : fill.profile;
    name.textContent = from != null && to != null
      ? `${way} ${format.clock(from, this.timeZone).slice(5)} – ${format.clock(to, this.timeZone).slice(11)}`
      : `${way}, points ${fill.first}–${fill.last}`;
    const meta = document.createElement("span");
    meta.className = "meta";
    meta.textContent = `${format.distance(fill.distance)} · ${fill.points} points filled`;
    if (fill.at) meta.textContent += ` · ${format.clock(fill.at, this.timeZone).slice(11)}`;
    label.append(name, meta);
    label.title = "Show on the map";
    label.addEventListener("click", () => this.onShowFill(fill));
    const remove = document.createElement("button");
    remove.className = "remove";
    remove.textContent = "×";
    remove.title = "Remove this fill and bring back the recorded points";
    remove.addEventListener("click", () => this.onRemoveFill(index));
    li.append(label, remove);
    return li;
  }

  editRow(edit, index) {
    const { track } = this;
    const li = document.createElement("li");
    li.className = "range-row";
    const label = document.createElement("span");
    label.className = "label";
    const name = document.createElement("span");
    name.className = "name";
    const meta = document.createElement("span");
    meta.className = "meta";
    if (edit.kind === "lasso") {
      name.textContent = "Lasso";
      meta.textContent = `${edit.points.length} points`;
    } else {
      const times = [];
      for (let i = edit.first; i <= edit.last; i++) if (track.time[i] != null) times.push(track.time[i]);
      const span = times.length
        ? `${format.clock(times[0], this.timeZone).slice(5)} – ${format.clock(times[times.length - 1], this.timeZone).slice(11)}`
        : `points ${edit.first}–${edit.last}`;
      name.textContent = `${edit.kind === "stop" ? "Stop" : "Range"} ${span}`;
      meta.textContent = `${edit.last - edit.first + 1} points`;
    }
    if (edit.at) meta.textContent += ` · removed ${format.clock(edit.at, this.timeZone).slice(11)}`;
    label.append(name, meta);
    label.title = "Show on the map";
    label.addEventListener("click", () => this.onShowRange(edit));
    li.append(label);
    if (edit.kind !== "lasso") {
      const join = document.createElement("select");
      join.className = "range-join";
      join.append(new Option("Break", "break"), new Option("Join", "join"));
      join.value = edit.join ? "join" : "break";
      join.title = "Break: no line and no distance across, like a pause. Join: a straight line across.";
      join.addEventListener("change", () => this.setJoin(index, join.value === "join"));
      li.append(join);
    }
    const restore = document.createElement("button");
    restore.className = "remove";
    restore.textContent = "×";
    restore.title = "Restore these points";
    restore.addEventListener("click", () => this.restoreEdit(index));
    li.append(restore);
    return li;
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
    map.addSource(MANUAL, { type: "geojson", data: empty() });
    map.addSource(PENDING, { type: "geojson", data: empty() });
    map.addSource(FILLED, { type: "geojson", data: empty() });
    map.addSource(PREVIEW, { type: "geojson", data: empty() });
    // Filled stretches: a light band under the track's own line.
    map.addLayer({
      id: FILLED,
      type: "line",
      source: FILLED,
      layout: { "line-join": "round", "line-cap": "round" },
      paint: { "line-color": RULE_COLORS.fill, "line-width": 10, "line-opacity": 0.45 },
    }, beforeLayer);
    // Removed stretches: a wide pale band with a dark dashed centre, readable
    // over any base map and apart from the track's own colour.
    map.addLayer({
      id: `${MANUAL}-band`,
      type: "line",
      source: MANUAL,
      layout: { "line-join": "round", "line-cap": "round" },
      paint: { "line-color": RULE_COLORS.manual, "line-width": 9, "line-opacity": 0.25 },
    }, beforeLayer);
    map.addLayer({
      id: MANUAL,
      type: "line",
      source: MANUAL,
      layout: { "line-join": "round" },
      paint: { "line-color": RULE_COLORS.manual, "line-width": 2.5, "line-dasharray": [1.5, 1.2] },
    }, beforeLayer);
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
    map.addLayer({
      id: `${PREVIEW}-casing`,
      type: "line",
      source: PREVIEW,
      layout: { "line-join": "round", "line-cap": "round" },
      paint: { "line-color": "#ffffff", "line-width": 6 },
    });
    map.addLayer({
      id: PREVIEW,
      type: "line",
      source: PREVIEW,
      layout: { "line-join": "round" },
      paint: { "line-color": "#0072b2", "line-width": 3, "line-dasharray": [2, 1.2] },
    });
    map.addLayer({
      id: PENDING,
      type: "circle",
      source: PENDING,
      paint: { "circle-radius": 7, "circle-color": RULE_COLORS.manual, "circle-stroke-color": "#ffffff", "circle-stroke-width": 3 },
    });
  }

  // setPreview draws a route not yet used, or with null clears it.
  setPreview(points) {
    const data = points ? { type: "Feature", properties: {}, geometry: { type: "LineString", coordinates: points } } : empty();
    this.map.getSource(PREVIEW).setData(data);
  }

  // showFills bands the stretches of a track filled in along the road, from
  // the point before each route to the point after it.
  showFills(track) {
    const lines = (track?.fills || []).map((fill) => {
      const line = [track.points[fill.first]];
      for (let i = fill.first + 1; i <= fill.first + fill.points; i++) line.push(track.points[i]);
      line.push(track.points[fill.last]);
      return line;
    });
    this.map.getSource(FILLED).setData({ type: "Feature", properties: {}, geometry: { type: "MultiLineString", coordinates: lines } });
  }

  // setPending marks a range's start picked on the track, or with null clears it.
  setPending(position) {
    const data = position ? { type: "Feature", properties: {}, geometry: { type: "Point", coordinates: position } } : empty();
    this.map.getSource(PENDING).setData(data);
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
    // Runs of points removed by hand, drawn along where they were recorded.
    const runs = [];
    let run = [];
    positions.forEach((point, i) => {
      if (track.removed[i] === "manual" && !hidden(i)) {
        run.push(point);
      } else {
        if (run.length > 1) runs.push(run);
        run = [];
      }
    });
    if (run.length > 1) runs.push(run);
    this.map.getSource(MANUAL).setData({ type: "Feature", properties: {}, geometry: { type: "MultiLineString", coordinates: runs } });
    track.removed.forEach((rule, i) => {
      if (!rule || hidden(i)) return;
      features.push({ type: "Feature", properties: { color: RULE_COLORS[rule], rule }, geometry: { type: "Point", coordinates: positions[i] } });
    });
    this.map.getSource(REMOVED).setData({ type: "FeatureCollection", features });
  }

  clear() {
    this.map.getSource(ORIGINAL).setData(empty());
    this.map.getSource(REMOVED).setData(empty());
    this.map.getSource(MANUAL).setData(empty());
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
