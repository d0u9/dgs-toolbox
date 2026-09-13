// Cutting the focused track into segments: the panel listing them with their
// names, and writing chosen segments into another GPX or a new one; and the
// map overlay marking cuts and the segment pointed at. The server finds the
// segments and writes the files; this module edits cuts and names.

import * as format from "./format.js";
import { confirmDialog } from "./dialog.js";

const SEGMENT = "cut-segment";
const CUTS = "cut-points";

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
  // onShow(piece) point at a segment; onWrite(request) writes segments and
  // resolves to the server's answer.
  constructor({ root, onCuts, onHover, onShow, onWrite }) {
    Object.assign(this, { root, onCuts, onHover, onShow, onWrite });
    this.track = null;
    this.selected = new Set(); // first point index of the chosen segments
    this.mode = "create";
    this.appendPath = "";
    this.message = null;
    this.timeZone = format.browserTimeZone();
    this.workspace = []; // other GPX paths in the workspace, for appending
  }

  show(track, { timeZone, workspace }) {
    const changed = !this.track || this.track.path !== track.path;
    this.track = track;
    this.timeZone = timeZone;
    this.workspace = workspace.filter((path) => path !== track.path);
    if (changed) {
      this.selected = new Set(track.pieces.map((piece) => piece.first));
      this.message = null;
      this.createPath = defaultCreatePath(track.path);
    } else {
      // Segments that still exist stay chosen; new ones are chosen too.
      const known = this.known || new Set();
      for (const piece of track.pieces) if (!known.has(piece.first)) this.selected.add(piece.first);
      const firsts = new Set(track.pieces.map((piece) => piece.first));
      for (const first of this.selected) if (!firsts.has(first)) this.selected.delete(first);
    }
    this.known = new Set(track.pieces.map((piece) => piece.first));
    if (!this.workspace.includes(this.appendPath)) this.appendPath = this.workspace[0] || "";
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

    const list = document.createElement("ul");
    list.className = "cut-list";
    track.pieces.forEach((piece, i) => list.append(this.row(piece, i + 1)));
    parts.push(list);

    parts.push(this.writer());
    this.root.replaceChildren(...parts);
  }

  row(piece, number) {
    const li = document.createElement("li");
    li.className = "cut-row";
    const check = document.createElement("input");
    check.type = "checkbox";
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
    const body = document.createElement("span");
    body.className = "label";
    const name = document.createElement("input");
    name.className = "cut-name";
    name.value = piece.name;
    name.placeholder = defaultName(piece, number, this.timeZone);
    name.title = "Name — empty uses its time span";
    name.addEventListener("change", () => this.rename(piece, name.value));
    name.addEventListener("click", (event) => event.stopPropagation());
    const meta = document.createElement("span");
    meta.className = "meta";
    const duration = piece.start != null ? format.duration((piece.end - piece.start) / 1000) : "";
    meta.textContent = [format.distance(piece.distance), duration, `${piece.points} points`].filter(Boolean).join(" · ");
    body.append(name, meta);
    li.append(check, badge, body);
    li.addEventListener("mouseenter", () => this.onHover(piece));
    li.addEventListener("mouseleave", () => this.onHover(null));
    li.addEventListener("click", (event) => {
      if (event.target !== check) this.onShow(piece);
    });
    return li;
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
    heading.append(title, textButton(chosen.length === track.pieces.length ? "Choose none" : "Choose all", false, () => {
      this.selected = chosen.length === track.pieces.length ? new Set() : new Set(track.pieces.map((piece) => piece.first));
      this.render();
    }));
    section.append(heading);

    const create = this.choice("create", "Into a new GPX");
    const createPath = document.createElement("input");
    createPath.className = "cut-path";
    createPath.value = this.createPath;
    createPath.title = "Full path of the new file; an existing file is not replaced";
    createPath.addEventListener("input", () => (this.createPath = createPath.value));
    createPath.addEventListener("focus", () => this.setMode("create", false));

    const append = this.choice("append", "Appended to a GPX in the workspace");
    const appendPath = document.createElement("select");
    appendPath.className = "cut-path";
    appendPath.disabled = this.workspace.length === 0;
    appendPath.append(...(this.workspace.length
      ? this.workspace.map((path) => new Option(basename(path), path))
      : [new Option("No other GPX in the workspace", "")]));
    appendPath.value = this.appendPath;
    appendPath.title = this.appendPath;
    appendPath.addEventListener("change", () => {
      this.appendPath = appendPath.value;
      this.setMode("append");
    });
    if (!this.workspace.length) append.querySelector("input").disabled = true;

    const go = document.createElement("button");
    go.className = "chip active cut-go";
    go.textContent = `Write ${chosen.length} segment${chosen.length === 1 ? "" : "s"}`;
    const target = this.mode === "append" ? this.appendPath : this.createPath.trim();
    go.disabled = chosen.length === 0 || !target;
    go.addEventListener("click", async () => {
      // Appending changes a GPX file the reader has: say so and wait for a yes.
      if (this.mode === "append") {
        const yes = await confirmDialog({
          title: "Change this GPX file?",
          message: `${chosen.length} track${chosen.length === 1 ? "" : "s"} will be added to the end of this file. The file is rewritten on disk; what it already holds is kept, but there is no undo.`,
          detail: target,
          confirm: "Append and save",
          danger: true,
        });
        if (!yes) return;
      }
      go.disabled = true;
      this.message = { text: "Writing…" };
      const segments = chosen.map((piece) => ({
        first: piece.first,
        last: piece.last,
        name: piece.name || defaultName(piece, track.pieces.indexOf(piece) + 1, this.timeZone),
      }));
      try {
        const result = await this.onWrite({ segments, target: { mode: this.mode, path: target } });
        this.message = { text: `${this.mode === "append" ? "Appended" : "Wrote"} ${result.tracks} track${result.tracks === 1 ? "" : "s"} to ${basename(result.path)}` };
      } catch (error) {
        this.message = { text: error.message, error: true };
      }
      this.render();
    });

    section.append(create, createPath, append, appendPath, go);
    if (this.message) {
      const note = document.createElement("p");
      note.className = "clean-foot" + (this.message.error ? " error" : "");
      note.textContent = this.message.text;
      section.append(note);
    }
    const foot = document.createElement("p");
    foot.className = "clean-foot";
    foot.textContent = "Each segment becomes one <trk>, as cleaned. The source GPX is never written.";
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
    map.addSource(SEGMENT, { type: "geojson", data: empty() });
    map.addSource(CUTS, { type: "geojson", data: empty() });
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
  }
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

function defaultCreatePath(source) {
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
