// Cutting the focused track into segments: the panel listing them with their
// names, saving the chosen ones into a GPX open in the workspace or into a new
// GPX named in the save dialog; and the map overlay marking cuts and the
// segment pointed at. The server finds the segments and writes the files; this
// module edits cuts and names.

import * as format from "./format.js";
import { saveFile } from "/ui/filedialog.js";
import * as status from "/ui/statusbar.js";

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
    this.addPath = "";
    this.timeZone = format.browserTimeZone();
    this.workspace = []; // other GPX paths in the workspace, to add segments to
    // Chosen segments per GPX path, so leaving a track and coming back keeps
    // the choice: path -> { selected, known, createPath }.
    this.choices = new Map();
  }

  // remember keeps the shown track's choice, for when it is shown again.
  remember() {
    if (!this.track) return;
    this.choices.set(this.track.path, {
      selected: new Set(this.selected),
      known: new Set(this.known || []),
      createPath: this.createPath,
    });
  }

  // folder is where a new file from a track not yet on disk goes by default;
  // home is where the save dialog falls back to when that folder is gone.
  show(track, { timeZone, workspace, folder, home = "", places = [] }) {
    const changed = !this.track || this.track.path !== track.path;
    if (changed) this.remember();
    this.track = track;
    this.timeZone = timeZone;
    this.workspace = workspace.filter((path) => path !== track.path);
    this.home = home;
    this.places = places;
    const kept = changed ? this.choices.get(track.path) : null;
    if (changed && !kept) {
      this.selected = new Set(track.pieces.map((piece) => piece.first));
      this.createPath = defaultCreatePath(track, folder);
    } else {
      if (kept) {
        this.selected = kept.selected;
        this.known = kept.known;
        this.createPath = kept.createPath || defaultCreatePath(track, folder);
      }
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
    this.remember();
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
    if (!this.track.cuts.includes(index)) return;
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
    head.append(title);
    parts.push(head);

    const hint = document.createElement("p");
    hint.className = "cut-hint";
    hint.textContent = "Click the track or timeline to cut there.";
    hint.title = "Remove one cut with × beside the segment it starts; drag a timeline cut to move it. Clear cuts removes them all. Zoom the timeline with the wheel for finer placement.";
    parts.push(hint);

    parts.push(this.list());

    parts.push(this.actions());
    this.root.replaceChildren(...parts);
  }

  // list shows the segments as rows of two lines: chosen, colour, number and
  // name, then its time span, duration and distance. Everything is stacked, so
  // a narrow inspector never hides a column.
  list() {
    const { track } = this;
    const wrap = document.createElement("div");
    wrap.className = "cut-list";
    const bar = document.createElement("div");
    bar.className = "cut-list-bar";
    const all = document.createElement("input");
    all.type = "checkbox";
    all.className = "cut-check";
    const chosen = track.pieces.filter((piece) => this.selected.has(piece.first)).length;
    all.checked = chosen === track.pieces.length;
    all.indeterminate = chosen > 0 && chosen < track.pieces.length;
    all.id = "cut-all";
    all.addEventListener("change", () => {
      this.selected = all.checked ? new Set(track.pieces.map((piece) => piece.first)) : new Set();
      this.render();
    });
    const label = document.createElement("label");
    label.htmlFor = "cut-all";
    label.textContent = all.checked ? "Choose none" : "Choose all";
    const count = document.createElement("span");
    count.className = "clean-count";
    count.textContent = `${chosen} of ${track.pieces.length} chosen`;
    bar.append(all, label, count);
    wrap.append(bar);
    track.pieces.forEach((piece, i) => wrap.append(this.row(piece, i + 1)));
    return wrap;
  }

  row(piece, number) {
    const item = document.createElement("div");
    item.className = "cut-row";
    item.title = `${piece.points} points — click to show it on the map`;

    const top = document.createElement("div");
    top.className = "cut-row-top";
    const check = document.createElement("input");
    check.type = "checkbox";
    check.className = "cut-check";
    check.checked = this.selected.has(piece.first);
    check.title = "Write this segment";
    check.addEventListener("change", () => {
      if (check.checked) this.selected.add(piece.first);
      else this.selected.delete(piece.first);
      this.render();
    });
    const badge = document.createElement("span");
    badge.className = "cut-number";
    badge.textContent = String(number);
    badge.style.background = pieceColor(number - 1);
    const name = document.createElement("input");
    name.className = "cut-name";
    name.value = piece.name;
    name.placeholder = defaultName(piece, number, this.timeZone);
    name.title = "Name — empty uses its time span";
    name.addEventListener("change", () => this.rename(piece, name.value));
    name.addEventListener("click", (event) => event.stopPropagation());
    top.append(check, badge, name);
    if (number > 1 && this.track.cuts.includes(piece.first)) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "cut-remove icon-button";
      button.textContent = "×";
      button.title = `Remove cut before segment ${number}`;
      button.setAttribute("aria-label", button.title);
      button.addEventListener("click", (event) => {
        event.stopPropagation();
        this.removeCut(piece.first);
      });
      top.append(button);
    }
    item.append(top);

    const meta = document.createElement("div");
    meta.className = "cut-row-meta";
    const facts = [];
    if (piece.start != null) {
      const from = format.clock(piece.start, this.timeZone, { seconds: false });
      const to = format.clock(piece.end, this.timeZone, { seconds: false });
      facts.push(from.slice(0, 10) === to.slice(0, 10) ? `${from.slice(11)}–${to.slice(11)}` : `${from.slice(5)} – ${to.slice(5)}`);
      facts.push(format.duration((piece.end - piece.start) / 1000) || "0m");
    }
    facts.push(format.distance(piece.distance));
    meta.textContent = facts.join(" · ");
    item.append(meta);

    item.addEventListener("mouseenter", () => this.onHover(piece));
    item.addEventListener("mouseleave", () => this.onHover(null));
    item.addEventListener("click", (event) => {
      if (event.target !== check) this.onShow(piece);
    });
    return item;
  }

  // actions holds everything done to the segments: cutting them, and writing
  // the chosen ones out.
  actions() {
    const { track } = this;
    const section = document.createElement("section");
    section.className = "clean-filter enabled cut-actions";
    const chosen = track.pieces.filter((piece) => this.selected.has(piece.first));

    const cutHead = document.createElement("div");
    cutHead.className = "clean-filter-head";
    const cutTitle = document.createElement("strong");
    cutTitle.textContent = "Cut";
    const cutCount = document.createElement("span");
    cutCount.className = "clean-count";
    cutCount.textContent = `${track.cuts.length} cut${track.cuts.length === 1 ? "" : "s"}`;
    cutHead.append(cutTitle, cutCount);
    const candidates = track.cutCandidates.filter((cut) => !track.cuts.includes(cut));
    const cutTools = document.createElement("div");
    cutTools.className = "cut-tools";
    cutTools.append(
      textButton("Cut at every stop", candidates.length === 0, () => {
        this.onCuts([...new Set([...track.cuts, ...track.cutCandidates])].sort((a, b) => a - b), this.names());
      }),
      textButton("Clear cuts", track.cuts.length === 0, () => this.onCuts([], this.names().filter((name) => name.start === 0))),
    );
    section.append(cutHead, cutTools);

    const heading = document.createElement("div");
    heading.className = "clean-filter-head cut-group";
    const title = document.createElement("strong");
    title.textContent = "Save segments to";
    heading.append(title);
    section.append(heading);

    const plural = chosen.length === 1 ? "" : "s";
    const segments = () => chosen.map((piece) => ({
      first: piece.first,
      last: piece.last,
      name: piece.name || defaultName(piece, track.pieces.indexOf(piece) + 1, this.timeZone),
    }));

    // A GPX open in the workspace: the segments become tracks in its sidecar.
    const row = document.createElement("div");
    row.className = "cut-target";
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
      addPath.title = this.addPath;
    });
    const add = document.createElement("button");
    add.type = "button";
    add.className = "button primary cut-go";
    add.textContent = `Add ${chosen.length} segment${plural}`;
    add.title = "Add the chosen segments to that GPX, each its own track";
    add.disabled = chosen.length === 0 || !this.addPath;
    add.addEventListener("click", () => this.write(segments(), { mode: "add", path: this.addPath }, add));
    // The select keeps the row to itself: a path is what the reader has to
    // read in full, and the button that acts on it goes under it.
    row.append(addPath);
    section.append(row, add);

    // Or a GPX that does not exist yet, named in the save dialog.
    const saveAs = document.createElement("button");
    saveAs.type = "button";
    saveAs.className = "button cut-save-as";
    saveAs.textContent = `Save as a new GPX…`;
    saveAs.title = "Choose the folder and name of a new GPX holding the chosen segments";
    saveAs.disabled = chosen.length === 0;
    saveAs.addEventListener("click", async () => {
      const target = await saveFile({
        title: "Save the segments as a new GPX",
        message: "Choose the folder of the new GPX and name it. An existing file is not replaced.",
        folder: this.createPath.replace(/[\\/][^\\/]*$/, ""),
        fallbacks: [this.home || "", ""],
        filters: [{ label: "GPX files", extensions: [".gpx"] }],
        places: this.places || null,
        name: basename(this.createPath) || "segments.gpx",
        replace: false, // the server writes a new file and never writes over one
      });
      if (!target) return;
      this.createPath = target;
      await this.write(segments(), { mode: "create", path: target }, saveAs);
    });
    section.append(saveAs);

    const foot = document.createElement("p");
    foot.className = "clean-foot";
    foot.textContent = "Each segment becomes one <trk>, as cleaned. No GPX already on disk is written: a GPX segments are added to is saved as a new file.";
    section.append(foot);
    return section;
  }

  // write sends the chosen segments to their target and says what happened.
  async write(segments, target, button) {
    button.disabled = true;
    status.show(target.mode === "add" ? "Adding…" : "Writing…");
    try {
      const result = await this.onWrite({ segments, target });
      const tracks = `${result.tracks} track${result.tracks === 1 ? "" : "s"}`;
      status.show(target.mode === "add"
        ? `Added ${tracks} to ${basename(result.path)}. Save it as a new GPX from its row in the workspace.`
        : `Wrote ${tracks} to ${basename(result.path)}`);
    } catch (error) {
      status.showError(error.message);
    }
    this.render();
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
  button.type = "button";
  button.className = "button";
  button.textContent = text;
  button.disabled = disabled;
  button.addEventListener("click", onClick);
  return button;
}

const basename = (path) => path.split(/[\\/]/).pop();

function empty() {
  return { type: "FeatureCollection", features: [] };
}
