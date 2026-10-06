// Cutting the focused track into segments: the panel listing them with their
// names, saving the chosen ones into a GPX open in the workspace or into a new
// GPX named in the save dialog; and the map overlay marking cuts and the
// segment pointed at. The server finds the segments and writes the files; this
// module edits cuts and names.

import * as format from "./format.js";
import { segmentSaveDialog } from "./dialog.js";
import { addTrace, traceLayers } from "./trace.js";
import { saveFile } from "/ui/filedialog.js";
import * as status from "/ui/statusbar.js";
import { promptDialog } from "/ui/dialog.js";
import { openMenu } from "/ui/menu.js";
import { tooltip } from "/ui/tooltip.js";
import { textButton as sharedTextButton } from "/ui/controls.js";
import { rowDoubleClick } from "/ui/selection.js";

const SEGMENT = "cut-segment";
const CUTS = "cut-points";
const PIECES = "cut-pieces";

// PIECE_COLORS tell neighbouring segments apart: Okabe-Ito hues, in an order
// where each differs strongly from the one before it.
const PIECE_COLORS = ["#cf8800", "#00856a", "#b53686", "#0072b2", "#d55e00", "#1685ae", "#9b8400", "#6f4c9b"];

// pieceColor is the colour of the segment at a place in the list.
export const pieceColor = (position) => PIECE_COLORS[position % PIECE_COLORS.length];

// A complete, unchanged track keeps its track name. This is a display
// default, never a saved segment override, so renaming the track follows through.
export function inheritedName(piece, track) {
  if (!track) return "";
  const parts = (track.parts || []).filter(part => part.kind === "track" && part.first === piece.first && part.last === piece.last);
  if (parts.length !== 1 || track.cuts?.some(cut => cut > piece.first && cut < piece.last)) return "";
  for (let i = piece.first; i <= piece.last; i++) {
    if (track.removed?.[i]) return "";
    if (track.original && track.points?.[i]?.some((value, axis) => value !== track.original[i]?.[axis])) return "";
  }
  if (track.fills?.some(fill => fill.first <= piece.last && fill.last >= piece.first)) return "";
  return parts[0].name?.trim() || "";
}

// Otherwise use the time span, or the segment number when there is no time.
export function defaultName(piece, number, timeZone, track) {
  const inherited = inheritedName(piece, track);
  if (inherited) return inherited;
  if (piece.start == null) return `Segment ${number}`;
  const from = format.clock(piece.start, timeZone, { seconds: false });
  const to = format.clock(piece.end, timeZone, { seconds: false });
  return from.slice(0, 10) === to.slice(0, 10) ? `${from}–${to.slice(11)}` : `${from} – ${to}`;
}

export class CutPanel {
  // onCuts(cuts, names) saves cuts and names; onHover(piece | null) and
  // onShow(piece) point at a segment; onWrite(request) writes or adds segments
  // and resolves to the server's answer.
  constructor({ root, onCuts, onHover, onShow, onWrite, onSelection = () => {} }) {
    Object.assign(this, { root, onCuts, onHover, onShow, onWrite, onSelection });
    this.track = null;
    this.selected = new Set(); // first point index of the chosen segments
    this.addPath = "";
    this.timeZone = format.browserTimeZone();
    this.workspace = []; // other GPX paths in the workspace, to add segments to
    // Chosen segments per GPX path, so leaving a track and coming back keeps
    // the choice: path -> { selected, known, createPath, addPath }.
    this.choices = new Map();
    this.doubleClick = rowDoubleClick();
  }

  // remember keeps the shown track's choice, for when it is shown again.
  remember() {
    if (!this.track) return;
    this.choices.set(`${this.track.path}\0${this.scope || ""}`, {
      selected: new Set(this.selected),
      known: new Set(this.known || []),
      createPath: this.createPath,
      addPath: this.addPath,
    });
  }

  // A file's choices and any target pointing at it follow its new name.
  renamePath(from, to) {
    if (this.track?.path === from) this.remember();
    for (const [key, value] of this.choices) {
      if (key.startsWith(`${from}\0`)) {
        this.choices.set(`${to}${key.slice(from.length)}`, value);
        this.choices.delete(key);
      }
    }
    for (const choice of this.choices.values()) if (choice.addPath === from) choice.addPath = to;
    this.workspace = this.workspace.map(path => path === from ? to : path);
    if (this.addPath === from) this.addPath = to;
    if (this.track?.path === from) this.track.path = to;
  }

  // folder is where a new file from a track not yet on disk goes by default;
  // home is where the save dialog falls back to when that folder is gone.
  show(track, { part = null, timeZone, workspace, folder, home = "", places = [] }) {
    const scope = part?.key || "";
    const changed = !this.track || this.track.path !== track.path || this.scope !== scope;
    const allPieces = track.pieces;
    track = { ...track, pieces: part ? allPieces.filter(piece => piece.first >= part.first && piece.last <= part.last) : allPieces, allPieces };
    if (changed) this.remember();
    this.track = track;
    this.scope = scope;
    this.timeZone = timeZone;
    this.workspace = workspace.filter((path) => path !== track.path || track.draft || track.ours);
    this.home = home;
    this.places = places;
    const kept = changed ? this.choices.get(`${track.path}\0${scope}`) : null;
    if (changed) this.addPath = kept?.addPath || "";
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
    if (!this.workspace.includes(this.addPath)) this.addPath = this.workspace.includes(track.path) ? track.path : this.workspace[0] || "";
    this.render();
  }

  // setWorkspace follows GPX files added to or removed from the workspace
  // while the panel is shown.
  setWorkspace(workspace) {
    if (!this.track) return;
    const others = workspace.filter((path) => path !== this.track.path || this.track.draft || this.track.ours);
    if (others.join("\n") === this.workspace.join("\n")) return;
    this.workspace = others;
    if (!others.includes(this.addPath)) this.addPath = others.includes(this.track.path) ? this.track.path : others[0] || "";
    this.render();
  }

  hide() {
    this.remember();
    this.track = null;
    this.root.replaceChildren();
    this.onSelection();
  }

  // Hide unchecked pieces, retaining boundary samples shared with checked pieces.
  hiddenRanges() {
    if (!this.track) return [];
    const checked = this.track.pieces.filter(piece => this.selected.has(piece.first));
    return this.track.pieces.filter(piece => !this.selected.has(piece.first)).map(piece => [
      piece.first + (checked.some(other => other.last === piece.first) ? 1 : 0),
      piece.last - (checked.some(other => other.first === piece.last) ? 1 : 0),
    ]).filter(([first, last]) => first <= last);
  }

  names() {
    return (this.track.allPieces || this.track.pieces).filter((piece) => piece.name).map((piece) => ({ start: piece.first, name: piece.name }));
  }

  // ---- edits: cuts are point indices; a name belongs to the segment starting at it ----

  addCut(index) {
    const kept = this.track.removed.map((rule, i) => (rule ? -1 : i)).filter((i) => i >= 0 && this.track.pieces.some(piece => i >= piece.first && i <= piece.last));
    if (kept.length < 3 || index <= kept[0] || index >= kept[kept.length - 1] || this.track.cuts.includes(index)) return;
    if (this.track.parts?.length && !this.track.parts.some(part => part.kind === "track" && index > part.first && index < part.last)) return;
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
    const part = this.track.parts?.find(part => part.kind === "track" && from > part.first && from < part.last);
    if (part && (to <= part.first || to >= part.last)) return;
    const scope = this.track.parts?.find(part => part.key === this.scope);
    if (scope && (from <= scope.first || from >= scope.last || to <= scope.first || to >= scope.last)) return;
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

    parts.push(this.list());

    parts.push(this.actions());
    this.root.replaceChildren(...parts);
    this.onSelection();
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
      this.doubleClick.reset();
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
    tooltip(bar, ["Select segments", "Click to select; double-click to fit", "Right-click to rename or remove a cut", "Drag a timeline cut to move it"]);
    wrap.append(bar);
    track.pieces.forEach((piece, i) => wrap.append(this.row(piece, i + 1)));
    return wrap;
  }

  row(piece, number) {
    const item = document.createElement("div");
    item.className = "cut-row";
    item.classList.toggle("selected", this.selected.has(piece.first));

    const top = document.createElement("div");
    top.className = "cut-row-top";
    const check = document.createElement("input");
    check.type = "checkbox";
    check.className = "cut-check";
    check.checked = this.selected.has(piece.first);
    check.title = "Show and include this segment";
    check.addEventListener("change", () => {
      this.doubleClick.reset();
      if (check.checked) this.selected.add(piece.first);
      else this.selected.delete(piece.first);
      this.render();
    });
    const badge = document.createElement("span");
    badge.className = "cut-number";
    badge.textContent = String(number);
    item.style.borderLeftColor = pieceColor(number - 1);
    const name = document.createElement("span");
    name.className = "cut-name";
    const fullName = piece.name || defaultName(piece, number, this.timeZone, this.track);
    const from = piece.start == null ? "" : format.clock(piece.start, this.timeZone, { seconds: false });
    const to = piece.end == null ? "" : format.clock(piece.end, this.timeZone, { seconds: false });
    name.textContent = piece.name || inheritedName(piece, this.track) || (from && to && from.slice(0, 10) === to.slice(0, 10) ? `${from.slice(11)}–${to.slice(11)}` : fullName);
    top.append(check, badge, name);
    const distance = document.createElement("span");
    distance.className = "row-distance";
    distance.textContent = format.distance(piece.distance);
    top.append(distance);
    item.append(top);
    tooltip(item, [fullName,
      piece.name && from && to ? `${from} – ${to}` : "",
      piece.start != null && piece.end != null ? format.duration((piece.end - piece.start) / 1000) || "0m" : "",
      `${format.distance(piece.distance)} · ${piece.points} points`,
      "Click to select · double-click to fit · right-click for actions",
    ]);

    item.addEventListener("mouseenter", () => this.onHover(this.selected.has(piece.first) ? piece : null));
    item.addEventListener("mouseleave", () => this.onHover(this.selected.size === 1 ? this.track.pieces.find(piece => this.selected.has(piece.first)) : null));
    item.addEventListener("click", (event) => {
      if (event.target.closest?.("button, input")) return;
      const fit = this.doubleClick(JSON.stringify([trackPath(this.track), piece.first]), event);
      if (event.metaKey || event.ctrlKey) {
        if (this.selected.has(piece.first)) this.selected.delete(piece.first); else this.selected.add(piece.first);
      } else this.selected = new Set([piece.first]);
      this.onShow(piece, { fit });
      this.render();
    });
    item.addEventListener("contextmenu", event => {
      this.doubleClick.reset();
      if (!this.selected.has(piece.first)) {
        this.selected = new Set([piece.first]);
        this.onShow(piece, { fit: false });
        this.render();
      }
      openMenu(event, [[
        { label: "Fit this segment", onSelect: () => this.onShow(piece, { fit: true }) },
        { label: "Use default name", onSelect: () => this.rename(piece, "") },
        { label: "Rename…", onSelect: async () => {
          const name = await promptDialog({ title: "Rename segment", message: "Give this segment a name.", value: fullName, confirm: "Rename" });
          if (name != null) this.rename(piece, name);
        } },
      ], this.track.cuts.includes(piece.first) ? [{ label: "Remove cut before this segment", onSelect: () => this.removeCut(piece.first) }] : []]);
    });
    return item;
  }

  // actions holds everything done to the segments: cutting them, and writing
  // the chosen ones out.
  actions() {
    const { track } = this;
    const section = document.createElement("section");
    section.className = "clean-filter enabled cut-actions compact-controls";
    const chosen = track.pieces.filter((piece) => this.selected.has(piece.first));

    const cutHead = document.createElement("div");
    cutHead.className = "clean-filter-head";
    const cutTitle = document.createElement("strong");
    cutTitle.textContent = "Cut";
    const inScope = index => track.pieces.some(piece => index > piece.first && index <= piece.last);
    const scopedCuts = track.cuts.filter(inScope);
    const cutCount = document.createElement("span");
    cutCount.className = "clean-count";
    cutCount.textContent = `${scopedCuts.length} cut${scopedCuts.length === 1 ? "" : "s"}`;
    cutHead.append(cutTitle, cutCount);
    const candidates = track.cutCandidates.filter((cut) => inScope(cut) && !track.cuts.includes(cut));
    const cutTools = document.createElement("div");
    cutTools.className = "cut-tools";
    cutTools.append(
      textButton("Cut at every stop", candidates.length === 0, () => {
        this.onCuts([...new Set([...track.cuts, ...candidates])].sort((a, b) => a - b), this.names());
      }, "button secondary"),
      textButton("Clear cuts", scopedCuts.length === 0, () => this.onCuts(track.cuts.filter(cut => !inScope(cut)), this.names().filter(name => !scopedCuts.includes(name.start))), "button secondary"),
    );
    section.append(cutHead, cutTools);

    const plural = chosen.length === 1 ? "" : "s";
    const segments = () => chosen.map((piece) => ({
      first: piece.first,
      last: piece.last,
      name: piece.name || defaultName(piece, track.pieces.indexOf(piece) + 1, this.timeZone, track),
    }));

    // A GPX open in the workspace: the segments become tracks in its sidecar.
    const add = document.createElement("button");
    add.type = "button";
    add.className = "button primary cut-go";
    add.textContent = `Add ${chosen.length} segment${plural}`;
    add.title = "Add the chosen segments as separate tracks, or merge them into one named track";
    add.disabled = chosen.length === 0 || !this.addPath;
    add.addEventListener("click", () => this.addSegments(segments(), { mode: "add", path: this.addPath }, add));
    section.append(add);

    // Or a GPX that does not exist yet, named in the save dialog.
    const saveAs = document.createElement("button");
    saveAs.type = "button";
    saveAs.className = "button secondary cut-save-as";
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
      await this.write(segments(), { mode: "create", path: target }, saveAs, track.path);
    });
    section.append(saveAs);

    tooltip(add, ["Add selected segments", "Choose Save to in the Add dialog", "Name one track, or choose separate tracks or a named merge", "Save the target GPX to write the added tracks"]);
    return section;
  }

  // No addition happens before the choice and any required name are confirmed.
  async addSegments(segments, target, button) {
    const path = this.track.path;
    button.disabled = true;
    try {
      const answer = await segmentSaveDialog({ targets: [...(this.workspace || [])], target: target.path, source: path, segments });
      if (!answer) return;
      this.addPath = answer.path;
      target = { mode: "add", path: answer.path };
      if (answer.merge) target = { ...target, merge: true, name: answer.name };
      else if (segments.length === 1) segments = [{ ...segments[0], name: answer.name }];
      await this.write(segments, target, button, path);
    } finally {
      button.disabled = false;
    }
  }

  // write sends the chosen segments to their target and says what happened.
  async write(segments, target, button, path) {
    button.disabled = true;
    status.show(target.mode === "add" ? "Adding…" : "Writing…");
    try {
      const result = await this.onWrite({ path, segments, target });
      const tracks = `${result.tracks} track${result.tracks === 1 ? "" : "s"}`;
      status.show(target.mode === "add"
        ? `Added ${tracks} to ${basename(result.path)}. Save the target GPX from its row in the workspace.`
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
    addTrace(map, { id: PIECES, source: PIECES, color: ["get", "color"], state: "segment", before: beforeLayer });
    addTrace(map, { id: SEGMENT, source: SEGMENT, color: ["get", "color"], state: "selected", before: beforeLayer });
    map.addLayer({
      id: CUTS,
      type: "circle",
      source: CUTS,
      paint: { "circle-radius": 5, "circle-color": "#ffffff", "circle-stroke-color": "#1d1d1b", "circle-stroke-width": 2.5 },
    }, beforeLayer);
  }

  show(track, hidden = []) {
    this.hidden = hidden;
    if (!track) {
      this.clear();
      return;
    }
    const features = track.cuts.filter(cut => track.pieces.some(piece => cut >= piece.first && cut <= piece.last) && !hidden.some(([first, last]) => cut >= first && cut <= last)).map((cut) => ({ type: "Feature", properties: {}, geometry: { type: "Point", coordinates: track.points[cut] } }));
    this.map.getSource(CUTS).setData({ type: "FeatureCollection", features });
    const pieces = track.pieces.map((piece, i) => ({
      type: "Feature",
      properties: { color: pieceColor(i) },
      geometry: { type: "MultiLineString", coordinates: keptLines(track, piece, hidden) },
    }));
    this.map.getSource(PIECES).setData({ type: "FeatureCollection", features: pieces });
    this.highlight(track, this.highlightedTrack === track ? this.highlightedPiece : null);
    // Above the track lines, which move up when focused; below cuts and the cursor.
    for (const layer of [...traceLayers(PIECES), ...traceLayers(SEGMENT), CUTS, this.beforeLayer]) if (this.map.getLayer(layer)) this.map.moveLayer(layer);
  }

  highlight(track, piece) {
    this.highlightedTrack = track;
    this.highlightedPiece = piece;
    if (!track || !piece) {
      this.map.getSource(SEGMENT).setData(empty());
      return;
    }
    const index = track.pieces.findIndex(candidate => candidate.first === piece.first);
    this.map.getSource(SEGMENT).setData({ type: "Feature", properties: { color: pieceColor(Math.max(0, index)) }, geometry: { type: "MultiLineString", coordinates: keptLines(track, piece, this.hidden || []) } });
  }

  clear() {
    this.hidden = [];
    this.highlightedTrack = null;
    this.highlightedPiece = null;
    this.map.getSource(SEGMENT).setData(empty());
    this.map.getSource(CUTS).setData(empty());
    this.map.getSource(PIECES).setData(empty());
  }
}

// keptLines are a segment's kept points, apart where the recording breaks.
function keptLines(track, piece, hidden = []) {
  const lines = [];
  let current = [], previous = -1;
  for (let i = piece.first; i <= piece.last; i++) {
    if (hidden.some(([first, last]) => i >= first && i <= last)) {
      if (current.length > 1) lines.push(current);
      current = []; previous = -1;
      continue;
    }
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

const textButton = (text, disabled, onClick, className = "button") => sharedTextButton(text, disabled, onClick, className);
const trackPath = track => track?.path || "";

const basename = (path) => path.split(/[\\/]/).pop();

function empty() {
  return { type: "FeatureCollection", features: [] };
}
