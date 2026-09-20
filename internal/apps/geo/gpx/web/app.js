// The GPX browser: pick GPX files from a folder, show several at once, and
// study the focused one — its profile, stops and timeline. This module owns
// page state; the others draw.

import { api } from "./api.js";
import * as format from "./format.js";
import { MapStack } from "./basemap.js";
import { Contours } from "./contours.js";
import { TrackLayers } from "./tracks.js";
import { Profile } from "./profile.js";
import { StopMarkers } from "./stops.js";
import { Timeline } from "./timeline.js";
import { CleanPanel, CleanOverlay, Lasso, insidePolygon } from "./clean.js";
import { CutPanel, CutOverlay, pieceBounds } from "./cut.js";
import { FolderTree, revealButton } from "./tree.js";
import { RoutePlanner } from "./route.js";
import { confirmDialog, promptDialog, saveDialog } from "./dialog.js";
import { splitter, shareSplitter } from "./splitter.js";

// Okabe-Ito-derived hues: distinct under the common red/green colour-vision
// deficiencies, with the darker variants retained for contrast on the map.
const PALETTE = ["#0072b2", "#d55e00", "#009e73", "#cc79a7", "#e69f00", "#56b4e9", "#6f4c9b", "#882255", "#117733", "#332288"];
const STORAGE_KEY = "dgs-gpx-browser";
const FILL_SNAP = 40; // pixels within which a fill tool click snaps to a track's end

const $ = (id) => document.getElementById(id);

const state = {
  config: null,
  tracks: new Map(), // the workspace: path -> { id, track, color, visible }
  focus: null, // path of the focused track
  nextId: 0,
  gcj: "auto", // GCJ-02 conversion: "auto" follows the base map, or "on" / "off"
  coordinates: "wgs84", // the system tracks are drawn in now
  cleanOpen: false, // edit mode: the inspector is open for the focused track
  routeOpen: false, // route mode: planning a route by hand in the inspector
  editTab: "changes", // the inspector's tab in edit mode: "changes", "auto" or "segments"
  compare: true, // the recording drawn under a cleaned track
  lasso: false,
  rangeTool: false, // dragging on the timeline removes a range
  rangeStart: null, // with the range tool, the start clicked on the track
  // Filling along the road: the tool is on, the profile, the start clicked
  // { index, position } — index null for a place off the track — and the
  // route previewed { path, ends, route, display, distance }.
  fill: { active: false, profile: "car", start: null, preview: null, busy: false, error: null },
  timelineStops: true, // stops on the timeline; off, only data and empty time
  timelineSnap: true, // the timeline's cursor sticks to stops, cuts and ends
  stopNumbers: true, // numbered stop markers on the map; the timeline always has them
  stops: { distance: 50, duration: 300 }, // stay-point thresholds: metres, seconds
  collapsed: { folders: false, workspace: false }, // sidebar panels folded to their header
  charts: { elevation: true, speed: true, layout: "stacked" }, // layout: "stacked" | "side"
  restoring: true, // while the saved session is replayed, do not overwrite it
};

let waypointPopup;
// mapReady settles once the map and its layers exist; the folder tree works
// before that, so a file clicked early waits for it.
let resolveMapReady;
const mapReady = new Promise((resolve) => (resolveMapReady = resolve));
let cleanPanel, cleanOverlay, lasso, cutPanel, cutOverlay, planner;
let stack;
let map, layers, profile, tree, stopMarkers, timeline, chartSplit;

// ---- persistence: a per-browser convenience, never required ----

function load() {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY)) || {};
  } catch {
    return {};
  }
}

function save() {
  if (state.restoring) return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({
      root: tree.root,
      configRoot: state.config.root,
      expanded: tree.expanded(),
      maps: stack?.toJSON(),
      timeZone: $("timezone").value,
      tracks: [...state.tracks].map(([path, entry]) => ({
        path, color: entry.color, visible: entry.visible, hiddenParts: [...entry.hiddenParts], expanded: entry.expanded,
      })),
      focus: state.focus,
      stops: state.stops,
      charts: state.charts,
      stopNumbers: state.stopNumbers,
      timelineSnap: state.timelineSnap,
      timelineStops: state.timelineStops,
      cleanOpen: state.cleanOpen,
      editTab: state.editTab,
      routeOpen: state.routeOpen,
      route: planner?.toJSON(),
      compare: state.compare,
      fillProfile: state.fill.profile,
      gcj: state.gcj,
      collapsed: state.collapsed,
    }));
  } catch {
    // Storage can be unavailable (private windows); the page works without it.
  }
}

// ---- workspace: the tracks picked from the tree, each shown or hidden ----

function nextColor() {
  const used = new Set([...state.tracks.values()].map((entry) => entry.color));
  return PALETTE.find((color) => !used.has(color)) || PALETTE[state.tracks.size % PALETTE.length];
}

const drawable = (entry) => entry.track.parts.length > 0;
const visibleEntries = () => [...state.tracks.values()].filter((entry) => entry.visible);

// addTrack puts a file in the workspace, shown. A file already there is shown
// and focused instead.
async function addTrack(path, { color, visible = true, row, restoring = false, hiddenParts = [], expanded = false } = {}) {
  row && row.classList.add("loading");
  await mapReady;
  if (state.tracks.has(path)) {
    setVisible(path, true);
    setFocus(path);
    tree.render();
    return true;
  }
  row && row.classList.add("loading");
  let track;
  try {
    track = await api.track(path, state.stops, state.coordinates);
  } catch (error) {
    row && row.classList.remove("loading");
    tree.showMessage(error.message, true);
    return false;
  }
  if (state.tracks.has(path)) return true;
  const entry = { id: state.nextId++, track, color: color || nextColor(), visible, hiddenParts: new Set(hiddenParts), expanded };
  state.tracks.set(path, entry);
  if (drawable(entry)) {
    layers.add(entry.id, track, entry.color);
    layers.setVisible(entry.id, visible);
    layers.setHiddenParts(entry.id, entry.hiddenParts);
  }
  if (visible && (!state.focus || !restoring)) setFocus(path, { fit: false });
  else if (state.focus) layers.setFocus(state.tracks.get(state.focus).id);
  if (!restoring && visible) fit([track]);
  tree.render();
  renderTracks();
  save();
  return true;
}

// redraw shows a track's new data on the map. A file that had nothing to draw
// — a new GPX before anything is added to it — gets its layers now.
function redraw(entry) {
  if (!drawable(entry)) return;
  if (!layers.ids.has(entry.id)) {
    layers.add(entry.id, entry.track, entry.color);
    layers.setVisible(entry.id, entry.visible);
    layers.setHiddenParts(entry.id, entry.hiddenParts);
    if (state.focus) layers.setFocus(state.tracks.get(state.focus)?.id);
    return;
  }
  layers.update(entry.id, entry.track);
}

function removeTrack(path) {
  const entry = state.tracks.get(path);
  if (!entry) return;
  if (layers.ids.has(entry.id)) layers.remove(entry.id);
  state.tracks.delete(path);
  if (state.focus === path) focusNextVisible();
  tree.render();
  renderTracks();
  save();
}

// setVisible shows or hides a track on the map without leaving the workspace.
// Only a shown track can be focused.
function setVisible(path, visible) {
  const entry = state.tracks.get(path);
  if (!entry || entry.visible === visible) return;
  entry.visible = visible;
  if (layers.ids.has(entry.id)) layers.setVisible(entry.id, visible);
  if (!visible && state.focus === path) focusNextVisible();
  else if (visible && !state.focus) setFocus(path, { fit: false });
  tree.render();
  renderTracks();
  save();
}

// setAllVisible shows or hides every track the filter lists.
function setAllVisible(visible) {
  for (const path of filteredPaths()) setVisible(path, visible);
}

function focusNextVisible() {
  const next = [...state.tracks].find(([, entry]) => entry.visible);
  setFocus(next ? next[0] : null, { fit: false });
}

function setFocus(path, { fit: shouldFit = true } = {}) {
  if (state.focus !== path && cleanOverlay && (state.fill.start != null || state.fill.preview)) setFill({ start: null, preview: null, error: null });
  state.focus = path;
  planner?.setFocused(path, path && state.tracks.get(path)?.track);
  $("profile-splitter").hidden = !path || !(state.charts.elevation || state.charts.speed);
  layers.setCursor(null);
  const entry = path && state.tracks.get(path);
  if (entry) {
    layers.setFocus(entry.id);
    const hidden = hiddenRanges(entry);
    profile.show(entry.track, entry.color, hidden);
    stopMarkers.show(entry.track, entry.color, hidden);
    timeline.show(entry.track, entry.color, hidden);
    syncChartsToTimeline(timeline.view);
    cleanOverlay.show(state.compare ? entry.track : null, hidden);
    cleanOverlay.showFills(entry.track);
    if (shouldFit) fit([entry.track]);
  } else {
    cleanOverlay.clear();
    cleanOverlay.showFills(null);
    cleanPanel.hide();
    layers.setFocus(null);
    profile.hide();
    stopMarkers.clear();
    timeline.hide();
  }
  applyCleanPanel();
  applyCutPanel();
  reportFocus();
  renderTracks();
  save();
}

// ---- cleaning: the focused track's filters, compare overlay and lasso ----

function applyCleanPanel() {
  if (!timeline) return;
  const entry = focusedEntry();
  const open = state.cleanOpen && Boolean(entry) && !state.routeOpen;
  const wasOpen = !$("inspector").hidden;
  $("inspector").hidden = !(open || state.routeOpen);
  $("inspector-splitter").hidden = $("inspector").hidden;
  $("clean-panel").hidden = !open;
  $("route-panel").hidden = !state.routeOpen;
  $("mode-browse").setAttribute("aria-pressed", String(!open && !state.routeOpen));
  $("mode-edit").setAttribute("aria-pressed", String(open));
  $("mode-route").setAttribute("aria-pressed", String(state.routeOpen));
  if (planner && planner.active !== state.routeOpen) planner.setActive(state.routeOpen);
  $("mode-edit").disabled = !entry;
  $("mode-edit").title = entry
    ? "Edit the focused track: remove points by hand, fill along the road, clean automatically"
    : "Focus a track in the workspace to edit it";
  if (wasOpen === $("inspector").hidden) map?.resize();
  if (!open) {
    setLasso(false);
    setRangeTool(false);
    setFillTool(false);
    timeline.setEditing(null);
    applyEditTools();
    return;
  }
  cleanPanel.timeZone = $("timezone").value;
  cleanPanel.fill = state.fill;
  cleanPanel.show(entry.track);
  timeline.setEditing({
    edits: entry.track.clean.params.edits,
    rangeTool: state.rangeTool,
    onRange: (first, last) => cleanPanel.addRange(first, last, false),
    onJoin: (index, join) => cleanPanel.setJoin(index, join),
    onRestore: (index) => cleanPanel.restoreEdit(index),
  });
  if (cutting()) timeline.setEditing(null); // the timeline's clicks cut instead
  applyEditTools();
}

// setMode switches between browsing, editing the focused track and planning
// a route.
function setMode(mode) {
  const edit = mode === "edit" && Boolean(focusedEntry());
  const route = mode === "route";
  if (state.cleanOpen === edit && state.routeOpen === route) return;
  state.cleanOpen = edit;
  state.routeOpen = route;
  applyCutPanel();
  applyCleanPanel();
  save();
}

// cutting says whether clicks on the track and the timeline cut: in edit mode,
// on the Segments tab.
const cutting = () => state.cleanOpen && !state.routeOpen && state.editTab === "segments" && Boolean(focusedEntry());

// setEditTab switches the inspector's tab. Segments is the Cut tool, so the
// other tools stop there.
function setEditTab(tab) {
  state.editTab = cleanPanel.tab = tab;
  if (tab === "segments") {
    setLasso(false);
    setRangeTool(false);
    setFillTool(false);
  }
  applyCleanPanel();
  applyCutPanel();
  save();
}

// setTool picks the edit tool in use: "select", "range", "lasso", "fill" or
// "cut". Cut shows the Segments tab; the others leave it.
function setTool(tool) {
  if (tool === "cut") {
    setEditTab("segments");
    return;
  }
  if (state.editTab === "segments") setEditTab("changes");
  if (tool === "range") setRangeTool(!state.rangeTool);
  else if (tool === "lasso") setLasso(!state.lasso);
  else if (tool === "fill") setFillTool(!state.fill.active);
  else {
    setLasso(false);
    setRangeTool(false);
    setFillTool(false);
  }
}

// applyEditTools shows the tool bar in edit mode, the tool in use, and a line
// on the map saying what a click does now.
function applyEditTools() {
  const open = state.cleanOpen && Boolean(focusedEntry()) && !state.routeOpen;
  $("map-tools").hidden = !open;
  const tool = cutting() ? "cut" : state.rangeTool ? "range" : state.lasso ? "lasso" : state.fill.active ? "fill" : "select";
  for (const name of ["select", "range", "lasso", "fill", "cut"]) {
    $(`tool-${name}`).setAttribute("aria-pressed", String(open && tool === name));
  }
  const hint = $("map-hint");
  const esc = "<kbd>Esc</kbd>";
  let html = "";
  if (open && tool === "range") {
    html = state.rangeStart == null
      ? `<strong>Range</strong> · click the track where the removal starts, or drag on the timeline · ${esc} to stop`
      : `<strong>Range</strong> · now click where it ends · ${esc} forgets the start`;
  } else if (open && tool === "cut") {
    html = "<strong>Cut</strong> · click the track or the timeline to cut there; drag a cut on the timeline to move it, double-click it to remove it";
  } else if (open && tool === "lasso") {
    html = `<strong>Lasso</strong> · draw around points to remove them; hold <kbd>${format.keys.alt}</kbd> to restore · ${esc} to stop`;
  } else if (open && tool === "fill") {
    const { fill } = state;
    if (fill.busy) html = "<strong>Fill along the road</strong> · asking the router…";
    else if (fill.preview) html = `<strong>Fill along the road</strong> · the route is dashed on the map: use or discard it in the panel · ${esc} discards`;
    else if (fill.start != null) html = `<strong>Fill along the road</strong> · now click where it ends; near a track's end the click snaps to it · ${esc} forgets the start`;
    else html = `<strong>Fill along the road</strong> · click the track where the stretch starts · ${esc} to stop`;
  } else if (state.routeOpen) {
    html = "<strong>Route</strong> · click the map to add a waypoint · drag a waypoint to move it, double-click to remove it · drag a small circle on a leg to insert one";
  } else if (open) {
    html = "Tools: <kbd>R</kbd> range · <kbd>L</kbd> lasso · <kbd>F</kbd> fill along the road · <kbd>C</kbd> cut";
  }
  hint.innerHTML = html;
  hint.hidden = !html;
}

// pushEdit records one removal by hand on the focused track.
function pushEdit(edit) {
  const entry = focusedEntry();
  if (!entry) return;
  const params = structuredClone(entry.track.clean.params);
  params.edits.push({ ...edit, at: Date.now() });
  saveClean(params);
}

function setLasso(active) {
  if (state.lasso === active) return;
  state.lasso = active;
  if (active) {
    setRangeTool(false);
    setFillTool(false);
  }
  lasso.setActive(active);
  cleanPanel.setLasso(active);
  applyEditTools();
}

function setRangeTool(active) {
  if (state.rangeTool === active) return;
  state.rangeTool = active;
  setRangeStart(null);
  if (active) {
    setLasso(false);
    setFillTool(false);
  }
  cleanPanel.setRangeTool(active);
  applyCleanPanel();
}

// ---- filling a stretch along the road ----

// setFill changes the fill tool's state and shows it in the panel and on the map.
function setFill(patch) {
  Object.assign(state.fill, patch);
  const entry = focusedEntry();
  if ("start" in patch) cleanOverlay.setPending(state.fill.start == null || !entry ? null : state.fill.start.position);
  if ("preview" in patch) cleanOverlay.setPreview(state.fill.preview?.display || null);
  cleanPanel.setFill(state.fill);
  applyEditTools();
}

// setFillTool turns picking a stretch's ends on or off; turning it off forgets
// a route not yet used.
function setFillTool(active) {
  if (state.fill.active === active && (active || (state.fill.start == null && !state.fill.preview))) return;
  if (active) {
    setLasso(false);
    setRangeTool(false);
  }
  setFill({ active, start: null, preview: null, busy: false, error: null });
}

// trackEndNear is the index of the first or last kept point of a shown track
// of entry within radius pixels of a screen point, or -1.
function trackEndNear(entry, screenPoint, radius = FILL_SNAP) {
  const { track } = entry;
  let index = -1, best = radius ** 2;
  for (const part of track.parts) {
    if (part.kind !== "track" || entry.hiddenParts.has(part.key)) continue;
    let first = part.first, last = part.last;
    while (first < last && track.removed[first]) first++;
    while (last > first && track.removed[last]) last--;
    for (const i of [first, last]) {
      if (track.removed[i]) continue;
      const p = map.project(track.points[i]);
      const d = (p.x - screenPoint.x) ** 2 + (p.y - screenPoint.y) ** 2;
      if (d <= best) [index, best] = [i, d];
    }
  }
  return index;
}

// routePointAt is where a click in route mode puts a waypoint, [lon, lat] in
// WGS-84: the end of a shown track in the workspace when near one, else the
// place clicked.
function routePointAt(event) {
  for (const entry of visibleEntries()) {
    const index = trackEndNear(entry, event.point);
    if (index >= 0) return planner.fromDisplay(entry.track.points[index]);
  }
  return planner.fromDisplay([event.lngLat.lng, event.lngLat.lat]);
}

// fillEndAt is the point of the track a click with the fill tool picks: the
// end of a shown track when near one, else the nearest point, or null.
function fillEndAt(entry, event) {
  const { track } = entry;
  let index = trackEndNear(entry, event.point);
  if (index < 0) index = layers.nearest(track, event.point, 24, entry.hiddenParts);
  if (index >= 0) return { index, position: track.points[index] };
  return null;
}

// pickFillEnd takes a point of the track clicked: the start, then the end,
// after which the router is asked for the road between them.
async function pickFillEnd(end) {
  const entry = focusedEntry();
  if (!end || !entry || state.fill.busy) return;
  const { start } = state.fill;
  if (start == null || state.fill.preview) {
    setFill({ start: end, preview: null, error: null });
    return;
  }
  if (start.index === end.index) return;
  const ends = { first: Math.min(start.index, end.index), last: Math.max(start.index, end.index) };
  const path = state.focus;
  setFill({ busy: true, error: null });
  try {
    const answer = await api.routeFill(path, ends, state.fill.profile, state.coordinates);
    if (state.focus !== path) return;
    setFill({ busy: false, start: null, preview: { path, ends, ...answer } });
  } catch (error) {
    setFill({ busy: false, start: null, error: error.message });
  }
}

async function useFill() {
  const { preview, profile } = state.fill;
  if (!preview || preview.path !== state.focus) return;
  const { first, last } = preview.ends;
  setFill({ busy: true });
  await changeFocused((path) => api.saveFill(path, first, last, profile, preview.route));
  setFill({ busy: false, preview: null });
}

// showFill frames a filled stretch on the map.
function showFill(fill) {
  showRange({ kind: "range", first: fill.first, last: fill.last });
}

// ---- planning a route ----

// saveRoute writes the planned route into a new GPX, chosen by path, and adds
// it to the workspace.
async function saveRoute() {
  const root = (tree.root || state.config.root || "").replace(/[\\/]$/, "");
  const folder = planner.source ? planner.source.replace(/[\\/][^\\/]*$/, "") : root;
  const target = await saveDialog({
    title: "Save the route",
    message: "Choose the folder of the new GPX and name it. An existing file is not replaced; the waypoints are kept beside it, so the route can be changed and saved again.",
    folder,
    fallback: state.config.root || "",
    places: state.config.places || [],
    name: `${planner.name}.gpx`,
    confirm: "Save",
  });
  if (!target) return;
  try {
    const { path } = await api.saveRoute({ target, name: planner.name, plan: planner.toJSON(), writeRte: planner.writeRte });
    planner.source = path;
    planner.message = { text: `Saved ${basename(path)}; it is in the workspace.` };
    if (state.tracks.has(path)) await reloadEntry(path);
    else await addTrack(path, { visible: true });
    tree.refresh?.();
  } catch (error) {
    planner.message = { text: error.message, error: true };
  }
  planner.render();
  save();
}

// editRoute loads the plan of a file in the workspace, to change it and save
// it as a new GPX.
async function editRoute({ path, name, plan }) {
  if (planner.waypoints.length && planner.source !== path) {
    const ok = await confirmDialog({
      title: "Replace the route being planned?",
      message: `The route on the map is set aside for the route of ${name}.`,
      confirm: "Edit its route",
    });
    if (!ok) return;
  }
  planner.load({ name: `${name} 2`, way: planner.way, writeRte: planner.writeRte, source: path, waypoints: plan.waypoints, legs: plan.legs });
  planner.message = null;
  const box = planner.bounds();
  if (box) fitBox(box);
  save();
}

// ---- tracks added from other files, saved into a new GPX ----

// saveAs writes the workspace GPX as it is currently edited into a new file.
async function saveAs(path) {
  const entry = state.tracks.get(path);
  const dot = path.toLowerCase().lastIndexOf(".gpx");
  const tracks = `${entry.track.added} track${entry.track.added === 1 ? "" : "s"}`;
  const root = (tree.root || state.config.root || "").replace(/[\\/]$/, "");
  const target = await saveDialog(entry.track.draft
    ? {
      title: "Save the new GPX",
      message: `${entry.track.name} holds ${tracks}, saved as they are being edited. Choose the folder to save it in and name it; an existing file is not replaced.`,
      folder: root,
      fallback: state.config.root || "",
      places: state.config.places || [],
      name: `${entry.track.name}.gpx`,
      confirm: "Save",
    }
    : {
      title: "Save as a new GPX",
      message: `${entry.track.name} is saved as a new file, as it is being edited: the points cleaning kept, the stretches filled in, ${tracks} added to it, and the names given here. ${basename(path)} itself is not written, and an existing file is not replaced.`,
      folder: path.replace(/[\\/][^\\/]*$/, "") || root,
      fallback: state.config.root || "",
      places: state.config.places || [],
      name: `${basename(dot > 0 ? path.slice(0, dot) : path)} edited.gpx`,
      confirm: "Save",
    });
  if (!target) return;
  try {
    const result = await api.saveAs(path, target);
    const { color, visible, expanded } = entry;
    removeTrack(path);
    // The saved file takes the original's place, open as it was: its tracks
    // stay listed, to show one at a time and to rename.
    await addTrack(result.path, { color, visible, expanded });
    tree.refresh();
    tree.showMessage(`Saved ${basename(result.path)}`);
  } catch (error) {
    tree.showMessage(error.message, true);
  }
}

// newGPX starts an empty GPX in memory, shown in the workspace, to add
// segments to from the Segments tab.
async function newGPX() {
  const name = await promptDialog({
    title: "New GPX",
    message: "A new, empty GPX, kept in memory until you save it. Add segments to it from the Segments tab of any track, in Edit. It is lost if dgs exits before it is saved.",
    value: "New trip",
    confirm: "Create",
  });
  if (!name) return;
  try {
    const { path } = await api.newDraft(name);
    await addTrack(path);
  } catch (error) {
    tree.showMessage(error.message, true);
  }
}

// discardSidecar deletes a file's sidecar after asking, so the GPX reads as
// recorded again.
async function discardSidecar(path) {
  const entry = state.tracks.get(path);
  if (!entry) return;
  const ok = await confirmDialog({
    title: "Discard the sidecar?",
    message: `Everything done to ${entry.track.name} goes: removals, filter settings, cuts, segment names, fills and tracks added to it. The GPX itself is not changed. This cannot be undone.`,
    detail: entry.track.clean.sidecar,
    confirm: "Discard",
    danger: true,
  });
  if (!ok) return;
  try {
    await api.discardSidecar(path);
    await reloadEntry(path);
  } catch (error) {
    tree.showMessage(error.message, true);
    return;
  }
  if (state.focus === path) setFocus(path, { fit: false });
  else renderTracks();
}

async function removeAdded(path, index) {
  try {
    await api.removeAdded(path, index);
    await reloadEntry(path);
  } catch (error) {
    tree.showMessage(error.message, true);
    return;
  }
  if (state.focus === path) setFocus(path, { fit: false });
  else renderTracks();
}

const basename = (path) => path.split(/[\\/]/).pop();

// removeStop removes every point of a stop by hand, joined across it.
function removeStop(index) {
  const entry = focusedEntry();
  const stop = entry?.track.stops[index];
  if (!stop) return;
  pushEdit({ kind: "stop", first: stop.first, last: stop.last, join: true });
}

// setRangeStart marks where a range picked on the track begins, or with null
// forgets it.
function setRangeStart(index) {
  state.rangeStart = index;
  const entry = focusedEntry();
  cleanOverlay.setPending(index == null || !entry ? null : entry.track.points[index]);
  applyEditTools();
}

// showRange frames the points of a removal by hand on the map.
function showRange(edit) {
  const { track } = focusedEntry();
  const box = [[Infinity, Infinity], [-Infinity, -Infinity]];
  const indices = edit.kind === "lasso" ? edit.points : Array.from({ length: edit.last - edit.first + 1 }, (_, k) => edit.first + k);
  for (const i of indices) {
    const [lon, lat] = track.points[i];
    box[0] = [Math.min(box[0][0], lon), Math.min(box[0][1], lat)];
    box[1] = [Math.max(box[1][0], lon), Math.max(box[1][1], lat)];
  }
  if (box[0][0] <= box[1][0]) fitBox(box);
}

// saveClean writes the focused track's cleaning and shows the result.
async function saveClean(params) {
  await changeFocused((path) => api.saveClean(path, params));
}

// changeFocused runs a change to the focused track's sidecar, then fetches the
// track again and redraws it.
async function changeFocused(change) {
  const path = state.focus;
  const entry = focusedEntry();
  if (!entry) return;
  try {
    await change(path);
    await reloadEntry(path);
  } catch (error) {
    tree.showMessage(error.message, true);
    return;
  }
  if (state.focus === path) setFocus(path, { fit: false });
  reportFocus();
}

async function reloadEntry(path) {
  const entry = state.tracks.get(path);
  if (!entry) return;
  entry.track = await api.track(path, state.stops, state.coordinates);
  redraw(entry);
}

// ---- cutting: the focused track's segments ----

function applyCutPanel() {
  const entry = focusedEntry();
  const open = cutting();
  if (open) {
    cutPanel.show(entry.track, { timeZone: $("timezone").value, workspace: [...state.tracks.keys()], folder: tree.root || state.config.root, home: state.config.root || "", places: state.config.places || [] });
    cutOverlay.show(entry.track);
    timeline.setCutting({
      cuts: entry.track.cuts,
      candidates: entry.track.cutCandidates,
      onAdd: (index) => cutPanel.addCut(index),
      onMove: (from, to) => cutPanel.moveCut(from, to),
      onRemove: (index) => cutPanel.removeCut(index),
    });
  } else {
    cutPanel.hide();
    cutOverlay.clear();
    timeline.setCutting(null);
  }
}

async function writeSegments(request) {
  const result = await api.writeSegments(state.focus, request);
  // A workspace track written into shows what it gained; a new file appears in the tree.
  if (state.tracks.has(result.path)) {
    const entry = state.tracks.get(result.path);
    await reloadEntry(result.path);
    // A GPX that gained a track opens to list what it now holds, so the
    // tracks added to it are there to see, hide and rename at once.
    if (expandable(entry)) entry.expanded = true;
    renderTracks();
    save();
  }
  tree.refresh();
  return result;
}

// lassoDone removes the kept points inside the shape as one lasso edit, or
// with Option/Alt takes the points inside it out of earlier lasso edits.
function lassoDone(polygon, { restore }) {
  const entry = focusedEntry();
  if (!entry) return;
  const { track } = entry;
  const hidden = hiddenRanges(entry);
  const inside = new Set();
  track.points.forEach((point, i) => {
    if (hidden.some(([first, last]) => i >= first && i <= last)) return;
    const wanted = restore ? track.removed[i] === "manual" : !track.removed[i];
    if (!wanted) return;
    const { x, y } = map.project(point);
    if (insidePolygon([x, y], polygon)) inside.add(i);
  });
  if (!inside.size) return;
  if (!restore) {
    pushEdit({ kind: "lasso", points: [...inside].sort((a, b) => a - b), join: true });
    return;
  }
  const params = structuredClone(track.clean.params);
  params.edits = params.edits
    .map((edit) => (edit.kind === "lasso" ? { ...edit, points: edit.points.filter((i) => !inside.has(i)) } : edit))
    .filter((edit) => edit.kind !== "lasso" || edit.points.length);
  saveClean(params);
}

// reportFocus tells the TUI which track to summarise. The page works without it.
function reportFocus() {
  api.focus(state.focus, state.stops).catch(() => {});
}

// setStopParams finds every workspace track's stops again with new thresholds.
async function setStopParams(stops) {
  state.stops = stops;
  save();
  await reloadTracks();
}

// reloadTracks fetches every workspace track again, with the current stop
// thresholds and coordinate system.
async function reloadTracks() {
  for (const [path, entry] of state.tracks) {
    try {
      entry.track = await api.track(path, state.stops, state.coordinates);
      redraw(entry);
    } catch (error) {
      tree.showMessage(error.message, true);
    }
  }
  if (state.focus) setFocus(state.focus, { fit: false });
  else renderTracks();
}

// ---- coordinates: GPX is WGS-84; GCJ-02 base maps need converted positions ----

function wantedCoordinates() {
  if (state.gcj === "on") return "gcj02";
  if (state.gcj === "off") return "wgs84";
  // Auto follows the base map. Converting a track outside China changes
  // nothing, so no need to look at where the tracks are to decide.
  return stack?.base()?.coordinates === "gcj02" ? "gcj02" : "wgs84";
}

function renderCoordinates() {
  const inChina = [...state.tracks.values()].some((entry) => entry.track.stats.inChina);
  const auto = stack?.base()?.coordinates === "gcj02" ? "on" : "off";
  const note = auto === "on" && state.tracks.size && !inChina ? ", no track in China" : "";
  $("coordinates").options[0].textContent = `Auto (${auto}${note})`;
  $("coordinates").classList.toggle("active", wantedCoordinates() === "gcj02");
}

// renderStack redraws the layer list, whose warnings follow the tracks' system.
function renderStack() {
  stack.coordinates = state.coordinates;
  stack.render($("layer-list"));
  const base = stack.base();
  $("layers-toggle").textContent = `Layers: ${base ? base.name : "none"}`;
}

// syncCoordinates redraws the tracks when the wanted system changed, and says
// in the selector what Auto currently does.
async function syncCoordinates() {
  renderCoordinates();
  const wanted = wantedCoordinates();
  if (wanted === state.coordinates) return;
  state.coordinates = wanted;
  renderStack();
  planner?.draw();
  await reloadTracks();
}

function setColor(path, color) {
  const entry = state.tracks.get(path);
  entry.color = color;
  if (layers.ids.has(entry.id)) layers.setColor(entry.id, color);
  if (state.focus === path) {
    const hidden = hiddenRanges(entry);
    profile.show(entry.track, color, hidden);
    stopMarkers.show(entry.track, color, hidden);
    timeline.show(entry.track, color, hidden);
  }
  tree.render();
  save();
}

// filteredPaths are the workspace tracks whose name or path holds every word
// of the filter box.
function filteredPaths() {
  const words = $("track-filter").value.trim().toLowerCase().split(/\s+/).filter(Boolean);
  return [...state.tracks]
    .filter(([path, entry]) => words.every((word) => `${entry.track.name} ${path}`.toLowerCase().includes(word)))
    .map(([path]) => path);
}

function renderTracks() {
  if (cutPanel && cutting()) cutPanel.setWorkspace([...state.tracks.keys()]);
  const paths = filteredPaths();
  const items = paths.flatMap((path) => {
    const entry = state.tracks.get(path);
    const rows = [trackRow(path, entry)];
    if (entry.expanded && expandable(entry)) {
      // One part shows and hides from its own row; the toolbar is for several.
      if (entry.track.parts.length > 1) rows.push(partsToolbar(path, entry));
      for (const part of entry.track.parts) rows.push(partRow(path, entry, part));
    }
    return rows;
  });
  $("track-list").replaceChildren(...items);
  const total = state.tracks.size, shown = visibleEntries().length;
  $("track-count").textContent = total ? `${shown}/${total} shown` : "";
  $("track-empty").hidden = total > 0;
  $("track-tools").hidden = total === 0;
  $("track-nomatch").hidden = !total || paths.length > 0;
  $("fit-all").hidden = shown < 2;
  renderCoordinates();
}

// expandable: every file that holds anything lists its parts under its row —
// a file of one track too, so that track can be shown on its own, framed and
// renamed, whoever recorded it.
const expandable = (entry) => entry.track.parts.length > 0;

function button(className, text, title, onClick) {
  const element = document.createElement("button");
  element.className = className;
  element.textContent = text;
  element.title = title;
  element.setAttribute("aria-label", title);
  element.addEventListener("click", (event) => {
    event.stopPropagation();
    onClick();
  });
  return element;
}

// label is a row's text: the name, up to two lines, over its details.
function label(name, details) {
  const box = document.createElement("span");
  box.className = "label";
  const title = document.createElement("span");
  title.className = "name";
  title.textContent = name;
  const meta = document.createElement("span");
  meta.className = "meta";
  meta.textContent = details.filter(Boolean).join(" · ");
  box.append(title, meta);
  return box;
}

function trackRow(path, entry) {
  const li = document.createElement("li");
  li.className = "track-row" + (state.focus === path ? " focused" : "") + (entry.visible ? "" : " hidden-track");
  li.title = path;

  // The disclosure sits at the row's end, so rows start flush at the left.
  const disclosure = expandable(entry)
    && button("disclosure", "▶", entry.expanded ? "Collapse" : "List its tracks, routes and waypoints", () => {
      entry.expanded = !entry.expanded;
      renderTracks();
      save();
    });
  if (disclosure) {
    disclosure.classList.toggle("expanded", entry.expanded);
    li.classList.add("expandable");
  }

  const eye = button("eye", entry.visible ? "◉" : "○", entry.visible ? "Hide on the map" : "Show on the map", () => setVisible(path, !entry.visible));
  eye.setAttribute("aria-pressed", String(entry.visible));

  const color = document.createElement("input");
  color.type = "color";
  color.className = "color";
  color.value = entry.color;
  color.title = "Colour";
  color.addEventListener("click", (event) => event.stopPropagation());
  color.addEventListener("input", () => setColor(path, color.value));

  const { track } = entry;
  const count = (kind, word) => {
    const n = track.parts.filter((part) => part.kind === kind).length;
    return n > 1 || (n === 1 && kind !== "track") ? `${n} ${word}${n > 1 ? "s" : ""}` : "";
  };
  const stops = track.stops.length;
  const details = [
    track.points.length ? format.distance(track.stats.distance) : "",
    count("track", "track"), count("route", "route"), count("waypoint", "waypoint"),
    stops ? `${stops} stop${stops > 1 ? "s" : ""}` : "",
    track.clean.error ? "sidecar unreadable" : track.clean.sidecar ? "sidecar" : "",
    track.draft ? `new, not saved${track.added ? "" : " — add segments from Segments, in Edit"}` : track.added ? `${track.added} added, not saved` : "",
  ];

  const actions = document.createElement("span");
  actions.className = "actions";
  if (track.draft) {
    li.classList.add("changed");
    if (track.added) actions.append(button("save-as", "Save…", "Save this new GPX to a file", () => saveAs(path)));
  } else {
    // Anything done to a file — cleaning, fills, cuts, names, added tracks —
    // is kept beside it, so it can be written into a new GPX at any time.
    if (track.added) li.classList.add("changed");
    if (track.added || track.clean.sidecar) {
      actions.append(button("save-as", "Save as…", "Save this GPX as a new file, as it is being edited", () => saveAs(path)));
    }
  }
  if (track.clean.sidecar) {
    li.classList.add("has-sidecar");
    actions.append(button("sidecar", "Discard…", `Discard the sidecar ${track.clean.sidecar}: the GPX reads as recorded again`, () => discardSidecar(path)));
  }
  if (tree.canReveal && !track.draft) actions.append(revealButton(path));
  actions.append(button("remove", "×", "Remove from the workspace", () => removeTrack(path)));

  li.append(eye, color, label(track.name, details));
  if (disclosure) li.append(disclosure);
  li.append(actions);
  // Clicking a hidden track shows it: only a shown track can be focused.
  li.addEventListener("click", () => {
    if (!entry.visible) setVisible(path, true);
    setFocus(path);
  });
  return li;
}

const PART_GLYPH = { track: "〰", route: "┅", waypoint: "⚑" };

function partRow(path, entry, part) {
  const hidden = entry.hiddenParts.has(part.key);
  const li = document.createElement("li");
  li.className = "part-row" + (hidden || !entry.visible ? " hidden-track" : "");
  li.title = `${part.name}${part.description ? ` — ${part.description}` : ""}`;

  const eye = button("eye", hidden ? "○" : "◉", hidden ? "Show this part" : "Hide this part", () => setPartHidden(path, part.key, !hidden));
  const glyph = document.createElement("span");
  glyph.className = "glyph";
  glyph.textContent = PART_GLYPH[part.kind];
  glyph.style.color = entry.color;

  const details = part.kind === "waypoint"
    ? [part.description, part.elevation != null ? `${Math.round(part.elevation)} m` : ""]
    : [
      format.distance(part.distance), part.segments > 1 ? `${part.segments} segments` : "", part.kind === "route" ? "planned" : "",
      part.added ? `added from ${basename(part.added.from)}` : "",
    ];
  li.append(eye, glyph, label(part.name, details));
  const actions = document.createElement("span");
  actions.className = "actions";
  actions.append(button("row-action", "✎", "Rename this part", () => renamePart(path, entry, part)));
  if (part.added) {
    li.classList.add("added");
    actions.append(button("remove", "×", "Take this added track out again", () => removeAdded(path, part.added.index)));
  }
  li.append(actions);
  li.addEventListener("click", () => showPart(path, part));
  return li;
}

// renamePart names one track, route or waypoint. A GPX dgs wrote is renamed
// in the file itself; a recording keeps its new name in the sidecar beside it,
// so the recorded file stays as it was.
async function renamePart(path, entry, part) {
  const inFile = entry.track.ours && part.kind === "track" && !entry.track.draft && !part.added;
  const name = await promptDialog({
    title: `Rename ${part.name}`,
    message: inFile
      ? "The name is written into the GPX itself, which dgs wrote."
      : "The name is kept beside the GPX, in its sidecar; the recorded file is not written.",
    value: part.name,
    confirm: "Rename",
  });
  if (!name) return;
  try {
    await api.renamePart(path, part.key, name);
    await reloadEntry(path);
    // The new name is in the workspace rows, and in the inspector of the
    // focused file, at once: neither is redrawn by reloading alone.
    renderTracks();
    if (state.focus === path) setFocus(path, { fit: false });
    tree.showMessage(`Renamed to ${name}`);
  } catch (error) {
    tree.showMessage(error.message, true);
  }
}

// partsToolbar shows or hides every part of a file at once, or every part of
// one kind: all tracks, all routes, all waypoints.
function partsToolbar(path, entry) {
  const li = document.createElement("li");
  li.className = "parts-toolbar";
  const group = (text, parts) => {
    const box = document.createElement("span");
    box.className = "parts-group";
    const name = document.createElement("span");
    name.textContent = text;
    const keys = parts.map((part) => part.key);
    const allShown = keys.every((key) => !entry.hiddenParts.has(key));
    const allHidden = keys.every((key) => entry.hiddenParts.has(key));
    const show = button("mini", "◉", `Show ${text.toLowerCase()}`, () => setPartsHidden(path, keys, false));
    const hide = button("mini", "○", `Hide ${text.toLowerCase()}`, () => setPartsHidden(path, keys, true));
    show.disabled = allShown;
    hide.disabled = allHidden;
    box.append(name, show, hide);
    return box;
  };
  const { parts } = entry.track;
  li.append(group("All", parts));
  for (const [kind, text] of [["track", "Tracks"], ["route", "Routes"], ["waypoint", "Waypoints"]]) {
    const ofKind = parts.filter((part) => part.kind === kind);
    if (ofKind.length && ofKind.length < parts.length) li.append(group(text, ofKind));
  }
  li.addEventListener("click", (event) => event.stopPropagation());
  return li;
}

function setPartHidden(path, key, hidden) {
  setPartsHidden(path, [key], hidden);
}

function setPartsHidden(path, keys, hidden) {
  const entry = state.tracks.get(path);
  for (const key of keys) {
    if (hidden) entry.hiddenParts.add(key);
    else entry.hiddenParts.delete(key);
  }
  if (layers.ids.has(entry.id)) layers.setHiddenParts(entry.id, entry.hiddenParts);
  // The profile, stops and timeline mark what is hidden.
  if (state.focus === path) setFocus(path, { fit: false });
  else renderTracks();
  save();
}

// hiddenRanges are the point index ranges [first, last] of a file's hidden tracks.
function hiddenRanges(entry) {
  return entry.track.parts
    .filter((part) => part.kind === "track" && entry.hiddenParts.has(part.key))
    .map((part) => [part.first, part.last]);
}

// showPart brings one part of a file into view, showing it if hidden.
function showPart(path, part) {
  const entry = state.tracks.get(path);
  if (!entry.visible) setVisible(path, true);
  if (entry.hiddenParts.has(part.key)) setPartHidden(path, part.key, false);
  if (state.focus !== path) setFocus(path, { fit: false });
  if (part.kind === "waypoint") {
    openWaypoint(entry, part);
    map.easeTo({ center: part.points[0], zoom: Math.max(map.getZoom(), 15), duration: 400 });
  } else {
    fitBox(part.bounds);
  }
}

function openWaypoint(entry, part) {
  const body = document.createElement("div");
  body.className = "stop-popup";
  const title = document.createElement("strong");
  title.textContent = part.name;
  const lines = [
    part.description,
    part.elevation != null ? `Elevation ${Math.round(part.elevation)} m` : "",
    part.time != null ? format.clock(part.time, $("timezone").value) : "",
  ].filter(Boolean).map((text) => {
    const line = document.createElement("div");
    line.textContent = text;
    return line;
  });
  body.append(title, ...lines);
  waypointPopup.setLngLat(part.points[0]).setDOMContent(body).addTo(map);
}

// fitBox frames one [[west, south], [east, north]] box.
function fitBox(box) {
  map.resize();
  const canvas = map.getCanvas();
  const padding = Math.max(8, Math.min(60, canvas.clientWidth / 10, canvas.clientHeight / 10));
  map.fitBounds(box, { padding, maxZoom: 16, duration: 400 });
}

function fit(tracks) {
  const withPoints = tracks.filter((track) => track.parts.length);
  if (!withPoints.length) return;
  const [[w, s], [e, n]] = withPoints.reduce(
    ([[w, s], [e, n]], { stats: { viewBounds: bounds } }) => [
      [Math.min(w, bounds[0][0]), Math.min(s, bounds[0][1])],
      [Math.max(e, bounds[1][0]), Math.max(n, bounds[1][1])],
    ],
    [[Infinity, Infinity], [-Infinity, -Infinity]],
  );
  // The profile may have just appeared; measure the map as it is now.
  map.resize();
  const canvas = map.getCanvas();
  const padding = Math.max(8, Math.min(60, canvas.clientWidth / 10, canvas.clientHeight / 10));
  map.fitBounds([[w, s], [e, n]], { padding, maxZoom: 16, duration: 400 });
}

// ---- map interaction ----

function focusedEntry() {
  return state.focus ? state.tracks.get(state.focus) : null;
}

function inspect(index) {
  const entry = focusedEntry();
  if (!entry || index == null || index < 0) {
    layers.setCursor(null);
    timeline.setIndex(null);
    return;
  }
  layers.setCursor(entry.track.points[index], entry.color);
  timeline.setIndex(index);
}

// ---- the timeline's time and the charts' distance, zoomed together ----

// syncChartsToTimeline shows on the charts the distance a stretch of time covers.
function syncChartsToTimeline(view) {
  const entry = focusedEntry();
  if (!entry) return;
  if (!view) {
    profile.setRange(null);
    return;
  }
  const { track } = entry;
  const first = timeline.nearest(view[0]);
  const last = timeline.nearest(view[1]);
  profile.setRange([track.distance[first] / 1000, track.distance[last] / 1000]);
}

// timeSpan is the [from, to] times of the kept, timed points whose distance
// lies within [min, max] metres, or null when there are none.
function timeSpan(track, min, max) {
  let from = null, to = null;
  for (let i = 0; i < track.distance.length; i++) {
    const d = track.distance[i];
    if (d < min || d > max || track.time[i] == null || track.removed[i]) continue;
    if (from == null || track.time[i] < from) from = track.time[i];
    if (to == null || track.time[i] > to) to = track.time[i];
  }
  return from == null ? null : [from, to];
}

// selectStop opens a stop's times; from the timeline it also brings the stop
// into view.
function selectStop(index, { pan = false } = {}) {
  const stop = focusedEntry()?.track.stops[index];
  if (!stop) return;
  stopMarkers.openPopup(index);
  if (pan && !map.getBounds().contains(stop.center)) map.easeTo({ center: stop.center, duration: 400 });
}

function bindMap() {
  let pending = null;
  map.on("mousemove", (event) => {
    pending = event.point;
    requestAnimationFrame(() => {
      if (!pending) return;
      const point = pending;
      pending = null;
      const entry = focusedEntry();
      const index = entry && entry.track.points.length ? layers.nearest(entry.track, point, 24, entry.hiddenParts) : -1;
      // In cut mode the pointer says a click cuts at the marked point.
      map.getCanvas().style.cursor = index >= 0 ? (cutting() ? "copy" : "crosshair") : state.fill.active || state.routeOpen ? "crosshair" : "";
      inspect(index);
      profile.setIndex(index >= 0 ? index : null);
    });
  });
  map.on("mouseout", () => {
    pending = null;
    inspect(null);
    profile.setIndex(null);
  });
  // Clicking another track's line focuses it.
  // Clicking a waypoint opens it.
  map.on("click", (event) => {
    if (state.lasso) return; // a lasso ends with a click; it selects, it does not focus
    // In route mode a click adds a waypoint; clicks on the route's own markers do not.
    if (state.routeOpen) {
      if (!event.originalEvent.target.closest?.(".plan-marker")) planner.add(routePointAt(event));
      return;
    }
    const focused = focusedEntry();
    // With the fill tool, clicks on the track pick a stretch's start, then its end.
    if (state.fill.active && focused) {
      pickFillEnd(fillEndAt(focused, event));
      return;
    }
    // With the range tool, clicks on the track pick a range's start, then its end.
    if (state.rangeTool && focused) {
      const index = layers.nearest(focused.track, event.point, 24, focused.hiddenParts);
      if (index < 0) return;
      if (state.rangeStart == null) {
        setRangeStart(index);
      } else {
        const first = Math.min(state.rangeStart, index), last = Math.max(state.rangeStart, index);
        setRangeStart(null);
        cleanPanel.addRange(first, last, false);
      }
      return;
    }
    // In cut mode, clicking the focused track cuts it at the point marked.
    if (cutting() && focused.track.points.length) {
      const index = layers.nearest(focused.track, event.point, 24, focused.hiddenParts);
      if (index >= 0) {
        cutPanel.addCut(index);
        return;
      }
    }
    const box = [[event.point.x - 4, event.point.y - 4], [event.point.x + 4, event.point.y + 4]];
    const waypoint = layers.waypointLayers().length && map.queryRenderedFeatures(box, { layers: layers.waypointLayers() })[0];
    if (waypoint) {
      for (const entry of state.tracks.values()) {
        const part = entry.id === waypoint.layer.metadata.track && entry.track.parts.find((p) => p.key === waypoint.properties.part);
        if (part) openWaypoint(entry, part);
      }
      return;
    }
    const layerIds = layers.lineLayers();
    if (!layerIds.length) return;
    const hit = map.queryRenderedFeatures(box, { layers: layerIds })[0];
    if (!hit) return;
    const id = hit.layer.metadata.track;
    for (const [path, entry] of state.tracks) {
      if (entry.id === id && path !== state.focus) setFocus(path, { fit: false });
    }
  });
}

// ---- layout ----

function bindSplitters() {
  const sidebar = document.querySelector(".sidebar");
  splitter({
    handle: $("sidebar-splitter"), target: sidebar, axis: "x", min: 200,
    max: () => window.innerWidth - 320, key: "sidebar",
  });
  splitter({
    handle: $("folder-splitter"), target: document.querySelector(".panel.folder"), min: 80,
    max: () => sidebar.clientHeight - 80, key: "folder",
  });
  splitter({
    handle: $("profile-splitter"), target: $("profile"), invert: true, min: 180,
    max: () => document.querySelector(".workspace").clientHeight - 120, key: "profile",
  });
  splitter({
    handle: $("inspector-splitter"), target: $("inspector"), axis: "x", invert: true, min: 260,
    max: () => Math.min(640, window.innerWidth - 560), key: "inspector",
  });
  chartSplit = shareSplitter({
    handles: { y: $("chart-splitter"), x: $("chart-splitter-x") },
    first: $("row-elevation"), second: $("row-speed"), container: $("charts"),
    axis: () => (state.charts.layout === "side" ? "x" : "y"), min: 100, key: "chart-share",
  });
}

// ---- sidebar panels: each folds to its header ----

function applyPanels() {
  const { folders, workspace } = state.collapsed;
  for (const [id, collapsed] of [["panel-folders", folders], ["panel-workspace", workspace]]) {
    $(id).classList.toggle("collapsed", collapsed);
    $(id).querySelector(".panel-toggle").setAttribute("aria-expanded", String(!collapsed));
  }
  const sidebar = document.querySelector(".sidebar");
  sidebar.classList.toggle("one-open", folders !== workspace);
  sidebar.classList.toggle("none-open", folders && workspace);
}

function bindPanels(saved) {
  if (saved && typeof saved === "object") {
    state.collapsed = { folders: saved.folders === true, workspace: saved.workspace === true };
  }
  for (const [key, id] of [["folders", "panel-folders"], ["workspace", "panel-workspace"]]) {
    // The whole header folds the panel, except the buttons it holds.
    $(id).querySelector("h2").addEventListener("click", (event) => {
      if (event.target.closest("button:not(.panel-toggle)")) return;
      state.collapsed[key] = !state.collapsed[key];
      applyPanels();
      save();
    });
  }
  applyPanels();
}

// ---- charts: each can be hidden; both show stacked or side by side ----

function applyCharts() {
  const { elevation, speed, layout } = state.charts;
  const both = elevation && speed;
  const charts = $("charts");
  charts.hidden = !elevation && !speed;
  charts.classList.toggle("stacked", layout !== "side");
  charts.classList.toggle("side", layout === "side");
  charts.classList.toggle("single", !both);
  chartSplit.apply();
  $("row-elevation").hidden = !elevation;
  $("row-speed").hidden = !speed;
  $("chart-splitter").hidden = !both || layout === "side";
  $("chart-splitter-x").hidden = !both || layout !== "side";
  $("chart-hint").hidden = charts.hidden;
  $("layout-toggles").hidden = !both;
  $("profile").classList.toggle("no-charts", charts.hidden);
  $("profile-splitter").hidden = !state.focus || charts.hidden;
  $("toggle-elevation").setAttribute("aria-pressed", String(elevation));
  $("toggle-speed").setAttribute("aria-pressed", String(speed));
  $("layout-stacked").setAttribute("aria-pressed", String(layout !== "side"));
  $("layout-side").setAttribute("aria-pressed", String(layout === "side"));
}

function bindCharts(saved) {
  if (saved && typeof saved === "object") {
    state.charts = {
      elevation: saved.elevation !== false,
      speed: saved.speed !== false,
      layout: saved.layout === "side" ? "side" : "stacked",
    };
  }
  const change = (patch) => {
    Object.assign(state.charts, patch);
    applyCharts();
    save();
  };
  $("toggle-elevation").addEventListener("click", () => change({ elevation: !state.charts.elevation }));
  $("toggle-speed").addEventListener("click", () => change({ speed: !state.charts.speed }));
  $("layout-stacked").addEventListener("click", () => change({ layout: "stacked" }));
  $("layout-side").addEventListener("click", () => change({ layout: "side" }));
  applyCharts();
}

// The layers popover opens under its button and closes on a click outside it.
function bindLayersPanel() {
  const panel = $("layers-panel");
  const toggle = $("layers-toggle");
  const open = (on) => {
    panel.hidden = !on;
    toggle.setAttribute("aria-expanded", String(on));
  };
  toggle.addEventListener("click", () => open(panel.hidden));
  document.addEventListener("pointerdown", (event) => {
    if (!panel.hidden && !panel.contains(event.target) && !toggle.contains(event.target)) open(false);
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && !panel.hidden) open(false);
  });
}

// ---- start ----

async function start() {
  const saved = load();
  state.config = await api.config();
  stack = new MapStack(null, {
    tiles: state.config.tiles,
    onChange: () => {
      save();
      syncCoordinates();
      renderStack();
    },
  });
  if (saved.maps) stack.restore(saved.maps);
  else {
    // Before the stack, one base map was chosen by name and contours switched on alone.
    const item = stack.items.find((entry) => entry.name === saved.basemap);
    if (item) stack.items.forEach((entry) => (entry.visible = entry === item));
    if (saved.contours === true) stack.items.find((entry) => !entry.tile).visible = true;
  }
  if (["auto", "on", "off"].includes(saved.gcj)) state.gcj = saved.gcj;
  $("coordinates").value = state.gcj;
  state.coordinates = wantedCoordinates();

  // The tree does not wait for the map: tiles can be slow.
  tree = new FolderTree({
    home: state.config.root,
    homeButton: $("folder-home"),
    list: $("folder-list"),
    pathLabel: $("folder-path"),
    upButton: $("folder-up"),
    refreshButton: $("folder-refresh"),
    message: $("folder-message"),
    onFile: (path, row) => addTrack(path, { row }),
    fileState: (path) => state.tracks.get(path),
    onChange: save,
  });
  tree.canReveal = state.config.canReveal;
  // A remembered root holds only while geo.gpx.root is what it was when it was
  // remembered: changing the configuration opens the new root.
  const rootStillApplies = saved.root && saved.configRoot === state.config.root;
  const folderOpened = tree.open(rootStillApplies ? saved.root : state.config.root, rootStillApplies ? saved.expanded || [] : []);
  bindSplitters();
  bindCharts(saved.charts);
  bindPanels(saved.collapsed);

  map = new maplibregl.Map({
    container: "map",
    style: { version: 8, sources: {}, layers: [] },
    center: [120.15, 30.25],
    zoom: 9,
    attributionControl: { compact: false },
  });
  map.addControl(new maplibregl.NavigationControl(), "top-right");
  map.addControl(new maplibregl.ScaleControl({ unit: "metric" }), "bottom-left");
  await new Promise((resolve) => map.once("load", resolve));

  stack.map = map;
  stack.contours = new Contours(map, state.config.dem);
  stack.apply();
  renderStack();
  layers = new TrackLayers(map);
  waypointPopup = new maplibregl.Popup({ offset: 10, maxWidth: "260px" });
  cleanOverlay = new CleanOverlay(map, "cursor");
  cleanPanel = new CleanPanel({
    root: $("clean-panel"),
    onChange: saveClean,
    onLasso: setLasso,
    onRangeTool: setRangeTool,
    onShowRange: showRange,
    onFillTool: setFillTool,
    onProfile: (profile) => {
      setFill({ profile, preview: null });
      save();
    },
    onUseFill: useFill,
    onDiscardFill: () => setFill({ preview: null }),
    onRemoveFill: (index) => changeFocused((path) => api.removeFill(path, index)),
    onShowFill: showFill,
    onTab: setEditTab,
    onCompare: (on) => {
      state.compare = on;
      const entry = focusedEntry();
      cleanOverlay.show(on && entry ? entry.track : null, entry ? hiddenRanges(entry) : []);
      save();
    },
  });
  lasso = new Lasso(map, { onDone: lassoDone, onCancel: () => setLasso(false) });
  if (saved.cleanOpen === true) state.cleanOpen = true;
  if (saved.compare === false) state.compare = false;
  cleanPanel.ways = state.config.ways || [];
  if (cleanPanel.ways.some((way) => way.id === saved.fillProfile)) state.fill.profile = saved.fillProfile;
  cleanPanel.compare = state.compare;
  if (["changes", "auto", "segments"].includes(saved.editTab)) state.editTab = saved.editTab;
  // Cut was a panel of its own; it is now the Segments tab.
  if (saved.cutOpen === true) [state.cleanOpen, state.editTab] = [true, "segments"];
  cleanPanel.tab = state.editTab;
  $("mode-browse").addEventListener("click", () => setMode("browse"));
  $("mode-edit").addEventListener("click", () => setMode("edit"));
  $("mode-route").addEventListener("click", () => setMode("route"));
  planner = new RoutePlanner(map, {
    root: $("route-panel"),
    api,
    gcj: () => state.coordinates === "gcj02",
    ways: state.config.ways || [],
    onChange: save,
    onSave: saveRoute,
    onEdit: editRoute,
  });
  planner.load(saved.route);
  if (saved.routeOpen === true) state.routeOpen = true;
  for (const name of ["select", "range", "lasso", "fill", "cut"]) $(`tool-${name}`).addEventListener("click", () => setTool(name));
  cutOverlay = new CutOverlay(map, "cursor");
  cutPanel = new CutPanel({
    root: cleanPanel.segmentsRoot,
    onCuts: (cuts, names) => changeFocused((path) => api.saveSegments(path, cuts, names)),
    onHover: (piece) => cutOverlay.highlight(focusedEntry()?.track, piece),
    onShow: (piece) => {
      const box = pieceBounds(focusedEntry().track, piece);
      if (box) fitBox(box);
    },
    onWrite: writeSegments,
  });
  profile = new Profile({
    root: $("profile"),
    elevation: $("chart-elevation"),
    speed: $("chart-speed"),
    name: $("profile-name"),
    swatch: $("profile-swatch"),
    stats: $("profile-stats"),
    readout: $("profile-readout"),
    onHover: inspect,
    onRange: (range) => {
      // The charts' distance range, shown as the time those points span.
      const entry = focusedEntry();
      if (!entry) return;
      timeline.setView(range && timeSpan(entry.track, range[0] * 1000, range[1] * 1000));
    },
  });
  stopMarkers = new StopMarkers(map, { onSelect: (index) => selectStop(index), onRemove: removeStop });
  window.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      if (state.rangeStart != null) setRangeStart(null); // first Esc forgets a half-picked range
      else setRangeTool(false);
      if (state.fill.start != null || state.fill.preview) setFill({ start: null, preview: null, error: null });
      else setFillTool(false);
    }
    // Undo the latest removal by hand, unless typing in a field.
    const typing = event.target.closest?.("input, select, textarea");
    const plain = !event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey;
    if (state.cleanOpen && !typing && plain) {
      const tool = { r: "range", l: "lasso", f: "fill", c: "cut" }[event.key.toLowerCase()];
      if (tool) {
        event.preventDefault();
        setTool(tool);
      }
    }
    if (state.cleanOpen && !typing && (event.metaKey || event.ctrlKey) && !event.shiftKey && event.key.toLowerCase() === "z") {
      event.preventDefault();
      cleanPanel.undo();
    }
  });
  timeline = new Timeline({
    root: $("timeline"),
    bar: $("timeline-bar"),
    labels: $("timeline-labels"),
    overview: $("timeline-overview"),
    ticks: $("timeline-ticks"),
    onScrub: (index) => {
      inspect(index);
      profile.setIndex(index);
    },
    onStop: (index) => selectStop(index, { pan: true }),
    onStopHover: (index) => stopMarkers.highlight(index),
    onView: (view) => syncChartsToTimeline(view),
  });
  bindMap();
  applyCleanPanel();
  applyCutPanel();
  resolveMapReady();

  if (saved.stops?.distance > 0 && saved.stops?.duration > 0) state.stops = saved.stops;
  const stopDistance = $("stop-distance"), stopDuration = $("stop-duration");
  stopDistance.value = state.stops.distance;
  stopDuration.value = state.stops.duration / 60;
  const readStopParams = () => {
    const distance = Number(stopDistance.value), minutes = Number(stopDuration.value);
    if (!(distance > 0) || !(minutes > 0)) return;
    setStopParams({ distance, duration: Math.round(minutes * 60) });
  };
  stopDistance.addEventListener("change", readStopParams);
  if (saved.timelineStops === false) state.timelineStops = false;
  $("timeline-stops").checked = state.timelineStops;
  timeline.setShowStops(state.timelineStops);
  $("timeline-stops").addEventListener("change", () => {
    state.timelineStops = $("timeline-stops").checked;
    timeline.setShowStops(state.timelineStops);
    save();
  });
  if (saved.timelineSnap === false) state.timelineSnap = false;
  $("timeline-snap").checked = state.timelineSnap;
  timeline.setSnap(state.timelineSnap);
  $("timeline-snap").addEventListener("change", () => {
    state.timelineSnap = $("timeline-snap").checked;
    timeline.setSnap(state.timelineSnap);
    save();
  });
  if (saved.stopNumbers === false) state.stopNumbers = false;
  $("stop-numbers").checked = state.stopNumbers;
  stopMarkers.setOnMap(state.stopNumbers);
  $("stop-numbers").addEventListener("change", () => {
    state.stopNumbers = $("stop-numbers").checked;
    stopMarkers.setOnMap(state.stopNumbers);
    save();
  });
  stopDuration.addEventListener("change", readStopParams);

  const zones = format.timeZones();
  const zoneSelect = $("timezone");
  zoneSelect.replaceChildren(...zones.map((zone, i) => new Option(i === 0 ? `${zone} (this browser)` : zone, zone)));
  zoneSelect.value = zones.includes(saved.timeZone) ? saved.timeZone : zones[0];
  const setTimeZone = (zone) => {
    profile.setTimeZone(zone);
    stopMarkers.setTimeZone(zone);
    timeline.setTimeZone(zone);
    applyCutPanel(); // default segment names read in the zone
    applyCleanPanel(); // so do removed ranges
  };
  setTimeZone(zoneSelect.value);
  zoneSelect.addEventListener("change", () => {
    setTimeZone(zoneSelect.value);
    save();
  });

  bindLayersPanel();
  $("coordinates").addEventListener("change", () => {
    state.gcj = $("coordinates").value;
    save();
    syncCoordinates();
  });
  $("new-gpx").addEventListener("click", (event) => {
    event.stopPropagation(); // the header folds the panel otherwise
    newGPX();
  });
  $("fit-all").addEventListener("click", () => fit(visibleEntries().map((entry) => entry.track)));
  $("track-filter").addEventListener("input", renderTracks);
  $("show-all").addEventListener("click", () => setAllVisible(true));
  $("hide-all").addEventListener("click", () => setAllVisible(false));
  // The profile appearing or disappearing changes the map's height.
  new ResizeObserver(() => map.resize()).observe($("map"));

  await folderOpened;
  // Earlier versions saved only the shown tracks, as "shown".
  for (const { path, color, visible, hiddenParts, expanded } of saved.tracks || saved.shown || []) {
    await addTrack(path, {
      color, visible: visible !== false, restoring: true,
      hiddenParts: Array.isArray(hiddenParts) ? hiddenParts : [], expanded: expanded === true,
    });
  }
  if (saved.focus && state.tracks.get(saved.focus)?.visible) setFocus(saved.focus, { fit: false });
  await syncCoordinates();
  const all = visibleEntries().map((entry) => entry.track);
  if (all.length) fit(all);
  applyCleanPanel(); // route mode opens the inspector without a focused track
  state.restoring = false;
  save();
}

start().catch((error) => {
  const message = document.createElement("p");
  message.className = "message error";
  message.textContent = error.message;
  document.body.prepend(message);
});
