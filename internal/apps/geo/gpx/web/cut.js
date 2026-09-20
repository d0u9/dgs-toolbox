// Cutting the focused track into segments: the panel listing them with their
// names, and writing chosen segments into a new GPX, or adding them to another
// GPX in the workspace; and the
// map overlay marking cuts and the segment pointed at. The server finds the
// segments and writes the files; this module edits cuts and names.

import * as format from "./format.js";
import { saveDialog } from "./dialog.js";

const SEGMENT = "cut-segment";
const CUTS = "cut-points";
const PIECES = "cut-pieces";

// PIECE_COLORS tell neighbouring segments apart: Okabe-Ito hues, in an order
// where each differs strongly from the one before it.
const PIECE_COLORS = ["#e69f00", "#009e73", "#cc79a7", "#0072b2", "#d55e00", "#56b4e9", "#f0e442", "#6f4c9b"];

// pieceColor is the colour of the segment at a place in the list.
export const pieceColor = (position) => PIECE_COLORS[position % PIECE_COLORS.length];

// defaultName names a segment by its time span in a time zone, or by its
// number when it has no time.
export function defaultName(piece, number, timeZone) {
  if (piece.start == null) return `Segment ${number}`;
  const from = format.clock(piece.start, timeZone, { seconds: false });
  const to = format.clock(piece.end, timeZone, { seconds: false });
  return from.slice(0, 10) === to.slice(0, 10) ? `${from}–${to.slice(11)}` : `${from} – ${to}`;
}

export class CutPanel {
  // onCuts(cuts, names) saves cuts and names; onHover(piece | null) and
  // onShow(piece) point at a segment; onWrite(request) writes or adds segments
  // and resolves to the server's answer.
  constructor({ root, onCuts, onHover, onShow, onWrite }) {
    Object.assign(this, { root, onCuts, onHover, onShow, onWrite });
    this.track = null;
    this.selected = new Set(); // first point index of the chosen segments
    this.mode = "create";
    this.addPath = "";
    this.message = null;
    this.timeZone = format.browserTimeZone();
    this.workspace = []; // other GPX paths in the workspace, to add segments to
  }

  // folder is where a new file from a track not yet on disk goes by default;
  // home is where the save dialog falls back to when that folder is gone.
  show(track, { timeZone, workspace, folder, home = "", places = [] }) {
    const changed = !this.track || this.track.path !== track.path;
    this.track = track;
    this.timeZone = timeZone;
    this.workspace = workspace.filter((path) => path !== track.path);
    this.home = home;
    this.places = places;
    if (changed) {
      this.selected = new Set(track.pieces.map((piece) => piece.first));
      this.message = null;
      this.createPath = defaultCreatePath(track, folder);
    } else {
      // Segments that still exist stay chosen; new ones are chosen too.
      const known = this.known || new Set();
      for (const piece of track.pieces) if (!known.has(piece.first)) this.selected.add(piece.first);
      const firsts = new Set(track.pieces.map((piece) => piece.first));
      for (const first of this.selected) if (!firsts.has(first)) this.selected.delete(first);
    }
    this.known = new Set(track.pieces.map((piece) => piece.first));
    if (!this.workspace.includes(this.addPath)) this.addPath = this.workspace[0] || "";
    this.render();
  }

  // setWorkspace follows GPX files added to or removed from the workspace
  // while the panel is shown.
  setWorkspace(workspace) {
    if (!this.track) return;
    const others = workspace.filter((path) => path !== this.track.path);
    if (others.join("\n") === this.workspace.join("\n")) return;
    this.workspace = others;
    if (!others.includes(this.addPath)) this.addPath = others[0] || "";
    this.render();
  }

  hide() {
    this.track = null;
    this.root.replaceChildren();
  }

  names() {
    return this.track.pieces.filter((piece) => piece.name).map((piece) => ({ start: piece.first, name: piece.name }));
  }

  // ---- edits: cuts are point indices; a name belongs to the segment starting at it ----

  addCut(index) {
    const kept = this.track.removed.map((rule, i) => (rule ? -1 : i)).filter((i) => i >= 0);
    if (index <= kept[0] || index >= kept[kept.length - 1] || this.track.cuts.includes(index)) return;
    const cuts = [...new Set([...this.track.cuts, index])].sort((a, b) => a - b);
    this.onCuts(cuts, this.names());
  }

  removeCut(index) {
    const cuts = this.track.cuts.filter((cut) => cut !== index);
    // The segment starting at the cut joins the one before; its name goes.
    this.onCuts(cuts, this.names().filter((name) => name.start !== index));
  }

  moveCut(from, to) {
    const cuts = [...new Set(this.track.cuts.map((cut) => (cut === from ? to : cut)))].sort((a, b) => a - b);
    const names = this.names().map((name) => (name.start === from ? { ...name, start: to } : name));
    if (this.selected.delete(from)) this.selected.add(to);
    this.onCuts(cuts, names);
  }

  rename(piece, name) {
    const names = this.names().filter((entry) => entry.start !== piece.first);
    if (name.trim()) names.push({ start: piece.first, name: name.trim() });
    this.onCuts(this.track.cuts, names);
  }

  render() {
    const { track } = this;
    if (!track) return;
    const parts = [];

    const head = document.createElement("div");
    head.className = "clean-head";
    const title = document.createElement("strong");
    title.textContent = `Segments (${track.pieces.length})`;
    const tools = document.createElement("span");
    tools.className = "cut-tools";
    const candidates = track.cutCandidates.filter((cut) => !track.cuts.includes(cut));
    tools.append(
      textButton("Cut at every stop", candidates.length === 0, () => {
        this.onCuts([...new Set([...track.cuts, ...track.cutCandidates])].sort((a, b) => a - b), this.names());
      }),
      textButton("Clear cuts", track.cuts.length === 0, () => this.onCuts([], this.names().filter((name) => name.start === 0))),
    );
    head.append(title, tools);
    parts.push(head);

    const hint = document.createElement("p");
    hint.className = "cut-hint";
    hint.textContent = "Click the track on the map, or click the timeline, to cut at that point; a dashed mark on the timeline is a stop's proposed cut. Drag a cut to move it, double-click it to remove it. Zoom the timeline with the wheel to place a cut to the second.";
    parts.push(hint);

    parts.push(this.table());

    parts.push(this.writer());
    this.root.replaceChildren(...parts);
  }

  // table lists the segments, one row each: chosen, number, name, time span,
  // duration and distance.
  table() {
    const { track } = this;
    const wrap = document.createElement("div");
    wrap.className = "cut-table-wrap";
    const table = document.createElement("table");
    table.className = "cut-table";
    const head = table.createTHead().insertRow();
    const all = document.createElement("input");
    all.type = "checkbox";
    const chosen = track.pieces.filter((piece) => this.selected.has(piece.first)).length;
    all.checked = chosen === track.pieces.length;
    all.indeterminate = chosen > 0 && chosen < track.pieces.length;
    all.title = all.checked ? "Choose none" : "Choose all";
    all.addEventListener("change", () => {
      this.selected = all.checked ? new Set(track.pieces.map((piece) => piece.first)) : new Set();
      this.render();
    });
    const cells = [[all, "check"], ["#", "num"], ["Name", "name"], ["Time", "time"], ["Duration", "right"], ["Distance", "right"]];
    for (const [content, className] of cells) {
      const th = document.createElement("th");
      th.className = className;
      th.append(content);
      head.append(th);
    }
    const body = table.createTBody();
    track.pieces.forEach((piece, i) => body.append(this.row(piece, i + 1)));
    wrap.append(table);
    return wrap;
  }

  row(piece, number) {
    const tr = document.createElement("tr");
    tr.className = "cut-row";
    tr.title = `${piece.points} points — click to show it on the map`;
    const cell = (className, ...content) => {
      const td = document.createElement("td");
      td.className = className;
      td.append(...content);
      tr.append(td);
      return td;
    };
    const check = document.createElement("input");
    check.type = "checkbox";
    check.checked = this.selected.has(piece.first);
    check.title = "Write this segment";
    check.addEventListener("change", () => {
      if (check.checked) this.selected.add(piece.first);
      else this.selected.delete(piece.first);
      this.render();
    });
    cell("check", check);
    const badge = document.createElement("span");
    badge.className = "cut-number";
    badge.textContent = String(number);
    badge.style.background = pieceColor(number - 1);
    cell("num", badge);
    const name = document.createElement("input");
    name.className = "cut-name";
    name.value = piece.name;
    name.placeholder = defaultName(piece, number, this.timeZone);
    name.title = "Name — empty uses its time span";
    name.addEventListener("change", () => this.rename(piece, name.value));
    name.addEventListener("click", (event) => event.stopPropagation());
    cell("name", name);
    let time = "–";
    if (piece.start != null) {
      const from = format.clock(piece.start, this.timeZone, { seconds: false });
      const to = format.clock(piece.end, this.timeZone, { seconds: false });
      time = from.slice(0, 10) === to.slice(0, 10) ? `${from.slice(11)}–${to.slice(11)}` : `${from.slice(5)} – ${to.slice(5)}`;
    }
    cell("time", time);
    cell("right", piece.start != null ? format.duration((piece.end - piece.start) / 1000) || "0m" : "–");
    cell("right", format.distance(piece.distance));
    tr.addEventListener("mouseenter", () => this.onHover(piece));
    tr.addEventListener("mouseleave", () => this.onHover(null));
    tr.addEventListener("click", (event) => {
      if (event.target !== check) this.onShow(piece);
    });
    return tr;
  }

  writer() {
    const { track } = this;
    const section = document.createElement("section");
    section.className = "clean-filter enabled cut-writer";
    const chosen = track.pieces.filter((piece) => this.selected.has(piece.first));

    const heading = document.createElement("div");
    heading.className = "clean-filter-head";
    const title = document.createElement("strong");
    title.textContent = "Write segments";
    const count = document.createElement("span");
    count.className = "clean-count";
    count.textContent = `${chosen.length} of ${track.pieces.length} chosen`;
    heading.append(title, count);
    section.append(heading);

    const create = this.choice("create", "Into a new GPX");
    const createPath = document.createElement("input");
    createPath.className = "cut-path";
    createPath.value = this.createPath;
    createPath.title = "Full path of the new file; an existing file is not replaced";
    createPath.addEventListener("input", () => (this.createPath = createPath.value));
    createPath.addEventListener("focus", () => this.setMode("create", false));
    const browse = document.createElement("button");
    browse.type = "button";
    browse.className = "text-button cut-browse";
    browse.textContent = "Browse…";
    browse.title = "Choose the folder and name of the new file";
    browse.addEventListener("click", async () => {
      const chosenPath = await saveDialog({
        title: "Write the segments into a new GPX",
        message: "Choose the folder of the new GPX and name it. An existing file is not replaced.",
        folder: this.createPath.replace(/[\\/][^\\/]*$/, ""),
        fallback: this.home || "",
        places: this.places || [],
        name: basename(this.createPath) || "segments.gpx",
        confirm: "Choose",
      });
      if (!chosenPath) return;
      this.createPath = chosenPath;
      this.setMode("create", false);
      this.render();
    });

    const add = this.choice("add", "Added to a GPX in the workspace, each its own track");
    const addPath = document.createElement("select");
    addPath.className = "cut-path";
    addPath.disabled = this.workspace.length === 0;
    addPath.append(...(this.workspace.length
      ? this.workspace.map((path) => new Option(basename(path), path))
      : [new Option("No other GPX in the workspace", "")]));
    addPath.value = this.addPath;
    addPath.title = this.addPath;
    addPath.addEventListener("change", () => {
      this.addPath = addPath.value;
      this.setMode("add");
    });
    if (!this.workspace.length) add.querySelector("input").disabled = true;

    const go = document.createElement("button");
    go.className = "chip active cut-go";
    const plural = chosen.length === 1 ? "" : "s";
    go.textContent = this.mode === "add" ? `Add ${chosen.length} segment${plural}` : `Write ${chosen.length} segment${plural}`;
    const target = this.mode === "add" ? this.addPath : this.createPath.trim();
    go.disabled = chosen.length === 0 || !target;
    go.addEventListener("click", async () => {
      go.disabled = true;
      this.message = { text: "Writing…" };
      const segments = chosen.map((piece) => ({
        first: piece.first,
        last: piece.last,
        name: piece.name || defaultName(piece, track.pieces.indexOf(piece) + 1, this.timeZone),
      }));
      try {
        const result = await this.onWrite({ segments, target: { mode: this.mode, path: target } });
        const tracks = `${result.tracks} track${result.tracks === 1 ? "" : "s"}`;
        this.message = {
          text: this.mode === "add"
            ? `Added ${tracks} to ${basename(result.path)}. Save it as a new GPX from its row in the workspace.`
            : `Wrote ${tracks} to ${basename(result.path)}`,
        };
      } catch (error) {
        this.message = { text: error.message, error: true };
      }
      this.render();
    });

    section.append(create, createPath, browse, add, addPath, go);
    if (this.message) {
      const note = document.createElement("p");
      note.className = "clean-foot" + (this.message.error ? " error" : "");
      note.textContent = this.message.text;
      section.append(note);
    }
    const foot = document.createElement("p");
    foot.className = "clean-foot";
    foot.textContent = "Each segment becomes one <trk>, as cleaned. No GPX already on disk is written: a GPX segments are added to is saved as a new file.";
    section.append(foot);
    return section;
  }

  choice(mode, text) {
    const label = document.createElement("label");
    label.className = "clean-check cut-choice";
    const input = document.createElement("input");
    input.type = "radio";
    input.name = "cut-mode";
    input.checked = this.mode === mode;
    input.addEventListener("change", () => this.setMode(mode));
    label.append(input, document.createTextNode(` ${text}`));
    return label;
  }

  setMode(mode, rerender = true) {
    if (this.mode === mode) return;
    this.mode = mode;
    if (rerender) this.render();
  }
}

// CutOverlay marks the focused track's cuts on the map and highlights the
// segment pointed at in the panel.
export class CutOverlay {
  constructor(map, beforeLayer) {
    this.map = map;
    this.beforeLayer = beforeLayer;
    map.addSource(SEGMENT, { type: "geojson", data: empty() });
    map.addSource(CUTS, { type: "geojson", data: empty() });
    map.addSource(PIECES, { type: "geojson", data: empty() });
    // Segments in their colours, drawn over the focused track while cutting.
    map.addLayer({
      id: PIECES,
      type: "line",
      source: PIECES,
      layout: { "line-join": "round", "line-cap": "round" },
      paint: { "line-color": ["get", "color"], "line-width": 5 },
    }, beforeLayer);
    map.addLayer({
      id: SEGMENT,
      type: "line",
      source: SEGMENT,
      layout: { "line-join": "round", "line-cap": "round" },
      paint: { "line-color": "#ffd400", "line-width": 9, "line-opacity": 0.55 },
    }, beforeLayer);
    map.addLayer({
      id: CUTS,
      type: "circle",
      source: CUTS,
      paint: { "circle-radius": 5, "circle-color": "#ffffff", "circle-stroke-color": "#1d1d1b", "circle-stroke-width": 2.5 },
    }, beforeLayer);
  }

  show(track) {
    if (!track) {
      this.clear();
      return;
    }
    const features = track.cuts.map((cut) => ({ type: "Feature", properties: {}, geometry: { type: "Point", coordinates: track.points[cut] } }));
    this.map.getSource(CUTS).setData({ type: "FeatureCollection", features });
    const pieces = track.pieces.map((piece, i) => ({
      type: "Feature",
      properties: { color: pieceColor(i) },
      geometry: { type: "MultiLineString", coordinates: keptLines(track, piece) },
    }));
    this.map.getSource(PIECES).setData({ type: "FeatureCollection", features: pieces });
    // Above the track lines, which move up when focused; below cuts and the cursor.
    for (const layer of [PIECES, SEGMENT, CUTS, this.beforeLayer]) if (this.map.getLayer(layer)) this.map.moveLayer(layer);
  }

  highlight(track, piece) {
    if (!track || !piece) {
      this.map.getSource(SEGMENT).setData(empty());
      return;
    }
    const line = [];
    for (let i = piece.first; i <= piece.last; i++) if (!track.removed[i]) line.push(track.points[i]);
    this.map.getSource(SEGMENT).setData({ type: "Feature", properties: {}, geometry: { type: "LineString", coordinates: line } });
  }

  clear() {
    this.map.getSource(SEGMENT).setData(empty());
    this.map.getSource(CUTS).setData(empty());
    this.map.getSource(PIECES).setData(empty());
  }
}

// keptLines are a segment's kept points, apart where the recording breaks.
function keptLines(track, piece) {
  const lines = [];
  let current = [], previous = -1;
  for (let i = piece.first; i <= piece.last; i++) {
    if (track.removed[i]) continue;
    if (previous >= 0 && track.segments[i] !== track.segments[previous]) {
      if (current.length > 1) lines.push(current);
      current = [];
    }
    current.push(track.points[i]);
    previous = i;
  }
  if (current.length > 1) lines.push(current);
  return lines;
}

// pieceBounds is the [[west, south], [east, north]] box of a segment's kept points.
export function pieceBounds(track, piece) {
  let west = Infinity, south = Infinity, east = -Infinity, north = -Infinity;
  for (let i = piece.first; i <= piece.last; i++) {
    if (track.removed[i]) continue;
    const [lon, lat] = track.points[i];
    west = Math.min(west, lon); east = Math.max(east, lon);
    south = Math.min(south, lat); north = Math.max(north, lat);
  }
  return west <= east ? [[west, south], [east, north]] : null;
}

function defaultCreatePath(track, folder = "") {
  if (track.draft) return `${folder.replace(/[\\/]$/, "")}/${track.name} segments.gpx`;
  const source = track.path;
  const dot = source.toLowerCase().lastIndexOf(".gpx");
  return `${dot > 0 ? source.slice(0, dot) : source} segments.gpx`;
}

function textButton(text, disabled, onClick) {
  const button = document.createElement("button");
  button.className = "text-button";
  button.textContent = text;
  button.disabled = disabled;
  button.addEventListener("click", onClick);
  return button;
}

const basename = (path) => path.split(/[\\/]/).pop();

function empty() {
  return { type: "FeatureCollection", features: [] };
}
