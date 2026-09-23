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
import { revealButton } from "./reveal.js";
import { CHEVRON, DIAMOND, DIAMOND_OPEN } from "./icons.js";
import { RoutePlanner } from "./route.js";
import { chooseDialog, confirmDialog, promptDialog, waypointDialog } from "./dialog.js";
import { openFile, saveFile } from "/ui/filedialog.js";
import { openMenu } from "/ui/menu.js";
import * as status from "/ui/statusbar.js";
import { splitter, shareSplitter } from "./splitter.js";
import { Terrain, DEFAULT_EXAGGERATION } from "./terrain.js";
import { longPressDrag, SortGap } from "./drag.js";

// Okabe-Ito-derived hues: distinct under the common red/green colour-vision
// deficiencies, with the darker variants retained for contrast on the map.
const PALETTE = ["#0072b2", "#d55e00", "#009e73", "#cc79a7", "#e69f00", "#56b4e9", "#6f4c9b", "#882255", "#117733", "#332288"];
const STORAGE_KEY = "dgs-gpx-browser";
const FILL_SNAP = 40; // pixels within which a fill tool click snaps to a track's end

const $ = (id) => document.getElementById(id);

// The status bar is the page's one place for news, so it exists before any
// code that could report some.
status.mount();

// What every dialog of this page filters by, opening and saving alike. Only
// GPX is offered: nothing else can be added to the workspace or written.
const GPX_FILTERS = [{ label: "GPX files", extensions: [".gpx"] }];

const state = {
  config: null,
  folder: "", // the folder the file dialogs open at: the last one a file came from
  tracks: new Map(), // the workspace: path -> { id, track, color, visible }
  focus: null, // path of the focused track
  // The rows picked out for a command that acts on several files at once.
  // The focused track is not the selection: focus is what the map and the
  // profile draw, the selection is what the next command acts on.
  selection: new Set(),
  anchor: null, // the row a Shift click reaches back to
  nextId: 0,
  gcj: "auto", // GCJ-02 conversion: "auto" follows the base map, or "on" / "off"
  coordinates: "wgs84", // the system tracks are drawn in now
  cleanOpen: false, // edit mode: the inspector is open for the focused track
  routeOpen: false, // route mode: planning a route by hand in the inspector
  editTab: "changes", // the inspector's tab in edit mode: "changes", "auto" or "segments"
  compare: true, // the recording drawn under a cleaned track
  lasso: false,
  waypointTool: false,
  rangeTool: false, // dragging on the timeline removes a range
  rangeStart: null, // with the range tool, the start clicked on the track
  // Filling along the road: the tool is on, the profile, the start clicked
  // { index, position } — index null for a place off the track — and the
  // route previewed { path, ends, route, display, distance }.
  fill: { active: false, profile: "car", start: null, preview: null, busy: false, error: null },
  timelineStops: true, // stops on the timeline; off, only data and empty time
  timelineSnap: true, // the timeline's cursor sticks to stops, cuts and ends
  timelineCompress: false, // stops and empty time drawn narrow on the timeline
  timelineHideEmpty: true, // empty time left out of the timeline
  stopNumbers: true, // numbered stop markers on the map; the timeline always has them
  stops: { distance: 50, duration: 300 }, // stay-point thresholds: metres, seconds
  charts: { elevation: true, speed: true, layout: "side" }, // layout: "stacked" | "side"
  // The 3D relief. Its stretch is remembered; whether it is on is not — the
  // page opens flat, and the relief is asked for when it is wanted.
  terrain: { on: false, exaggeration: DEFAULT_EXAGGERATION },
  // Panels folded away by hand. Each is the user's choice; the panel still
  // has to have something to show to appear at all.
  hidden: { sidebar: false, profile: false, inspector: false },
  restoring: true, // while the saved session is replayed, do not overwrite it
};

let waypointPopup;
// mapReady settles once the map and its layers exist; a file opened before
// that waits for it.
let resolveMapReady;
const mapReady = new Promise((resolve) => (resolveMapReady = resolve));
let cleanPanel, cleanOverlay, lasso, cutPanel, cutOverlay, planner;
let stack;
let terrain;
let terrainToggle = () => {}; // the 3D switch, from the key as well as the buttons
let map, layers, profile, stopMarkers, timeline, chartSplit;

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
      folder: state.folder,
      configRoot: state.config.root,
      maps: stack?.toJSON(),
      timeZone: $("timezone").value,
      tracks: [...state.tracks].map(([path, entry]) => ({
        path, color: entry.color, visible: entry.visible, hiddenParts: [...entry.hiddenParts], expanded: entry.expanded,
      })),
      focus: state.focus,
      stops: state.stops,
      charts: state.charts,
      terrain: { exaggeration: state.terrain.exaggeration },
      hiddenPanels: state.hidden,
      stopNumbers: state.stopNumbers,
      timelineSnap: state.timelineSnap,
      timelineCompress: state.timelineCompress,
      timelineHideEmpty: state.timelineHideEmpty,
      timelineStops: state.timelineStops,
      cleanOpen: state.cleanOpen,
      editTab: state.editTab,
      routeOpen: state.routeOpen,
      route: planner?.toJSON(),
      compare: state.compare,
      fillProfile: state.fill.profile,
      gcj: state.gcj,
    }));
  } catch {
    // Storage can be unavailable (private windows); the page works without it.
  }
}

// ---- where the dialogs open, and the message line ----

// setFolder remembers the folder the file dialogs open at. It is not shown:
// the dialog is the only place folders are browsed.
function setFolder(folder) {
  if (!folder || folder === state.folder) return;
  state.folder = folder;
  save();
}

// showMessage is the page's one line of news: what was saved, renamed or
// refused. It goes to the shared status bar, so a message reads the same
// wherever on the page it came from.
function showMessage(text, isError) {
  status.show(text, { error: isError });
}

// applyStatus writes the parts of the status bar that are not news: the mode
// on the left and the focused track on the right.
function applyStatus() {
  const mode = state.routeOpen ? "Route" : state.cleanOpen ? "Edit" : "Browse";
  status.setState(mode);
  const entry = focusedEntry();
  status.setHints(entry ? entry.track.name || basename(entry.track.path) : "");
}

// ---- workspace: the tracks opened, each shown or hidden ----

function nextColor() {
  const used = new Set([...state.tracks.values()].map((entry) => entry.color));
  return PALETTE.find((color) => !used.has(color)) || PALETTE[state.tracks.size % PALETTE.length];
}

const drawable = (entry) => entry.track.parts.length > 0;
const visibleEntries = () => [...state.tracks.values()].filter((entry) => entry.visible);

// addTrack puts a file in the workspace, shown. A file already there is shown
// and focused instead.
async function addTrack(path, { color, visible = true, restoring = false, hiddenParts = [], expanded = false } = {}) {
  await mapReady;
  if (state.tracks.has(path)) {
    setVisible(path, true);
    setFocus(path);
    return true;
  }
  let track;
  try {
    track = await api.track(path, state.stops, state.coordinates);
  } catch (error) {
    showMessage(error.message, true);
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
  state.selection.delete(path);
  if (state.anchor === path) state.anchor = null;
  if (state.focus === path) focusNextVisible();
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
  renderTracks();
  save();
}

// ---- selecting rows: what a command acts on ----

// select picks rows out of the workspace the way a file manager does: a plain
// click picks one, Cmd or Ctrl adds or takes away one, and Shift reaches from
// the row last picked to this one, over the rows the filter lists.
function select(path, { toggle = false, range = false } = {}) {
  const paths = filteredPaths();
  if (range && state.anchor && paths.includes(state.anchor)) {
    const from = paths.indexOf(state.anchor), to = paths.indexOf(path);
    const [first, last] = from < to ? [from, to] : [to, from];
    if (!toggle) state.selection.clear();
    for (const between of paths.slice(first, last + 1)) state.selection.add(between);
  } else if (toggle) {
    if (state.selection.has(path)) state.selection.delete(path);
    else state.selection.add(path);
    state.anchor = path;
  } else {
    state.selection = new Set([path]);
    state.anchor = path;
  }
}

// selectionFor is what a command on this row acts on: the selection when the
// row is in it, and the row alone otherwise — a right click on a row outside
// the selection picks that row first, as a file manager does.
function selectionFor(path) {
  if (!state.selection.has(path)) select(path);
  return [...state.selection].filter((selected) => state.tracks.has(selected));
}

function clearSelection() {
  state.selection.clear();
  state.anchor = null;
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
  applyCharts();
  applyCleanPanel();
  applyCutPanel();
  applyStatus();
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
  const wanted = open || state.routeOpen;
  $("inspector").hidden = !wanted || state.hidden.inspector;
  $("inspector-splitter").hidden = $("inspector").hidden;
  $("show-inspector").hidden = !(wanted && state.hidden.inspector);
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
    state.waypointTool = false;
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
  applyStatus();
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
    state.waypointTool = false;
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
  state.waypointTool = tool === "waypoint" ? !state.waypointTool : false;
  if (tool === "range") setRangeTool(!state.rangeTool);
  else if (tool === "lasso") setLasso(!state.lasso);
  else if (tool === "fill") setFillTool(!state.fill.active);
  else {
    setLasso(false);
    setRangeTool(false);
    setFillTool(false);
  }
  applyEditTools();
}

// setCompare draws the untouched recording under the cleaned track, or stops.
function setCompare(on) {
  state.compare = on;
  const entry = focusedEntry();
  cleanOverlay.show(on && entry ? entry.track : null, entry ? hiddenRanges(entry) : []);
  applyEditTools();
  save();
}

// applyEditTools shows the tool bar in edit mode, the tool in use, and a line
// on the map saying what a click does now.
function applyEditTools() {
  const open = state.cleanOpen && Boolean(focusedEntry()) && !state.routeOpen;
  $("map-tools").hidden = !open;
  const tool = cutting() ? "cut" : state.waypointTool ? "waypoint" : state.rangeTool ? "range" : state.lasso ? "lasso" : state.fill.active ? "fill" : "select";
  for (const name of ["select", "range", "lasso", "fill", "cut", "waypoint"]) {
    $(`tool-${name}`).setAttribute("aria-pressed", String(open && tool === name));
  }
  $("tool-compare").setAttribute("aria-pressed", String(open && state.compare));
  $("tool-compare").disabled = !open;
  const esc = "<kbd>Esc</kbd>";
  let html = "";
  if (open && tool === "range") {
    html = state.rangeStart == null
      ? `<strong>Range</strong> · click the track where the removal starts, or drag on the timeline · ${esc} to stop`
      : `<strong>Range</strong> · now click where it ends · ${esc} forgets the start`;
  } else if (open && tool === "waypoint") {
    html = `<strong>Waypoint</strong> · click the map, then choose a dgs-created GPX and name the point · ${esc} to stop`;
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
    html = "Tools: <kbd>R</kbd> range · <kbd>L</kbd> lasso · <kbd>F</kbd> fill along the road · <kbd>C</kbd> cut · ⚑ waypoint";
  }
  // The line lives in the middle of the status bar, where the TUI shell puts
  // the same summary, and the map keeps its whole surface for the track.
  status.setSummary(html);
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

async function addWaypointAt(event) {
  // A new GPX not yet saved takes a waypoint too: it is kept with the draft
  // until the draft is written.
  const targets = [...state.tracks].filter(([path, entry]) => entry.track.ours || entry.track.draft)
    .map(([path, entry]) => ({ path, name: entry.track.name }));
  if (!targets.length) {
    showMessage("Open a GPX created by dgs, or start a new one, before adding a waypoint; other recordings are never overwritten.", true);
    return;
  }
  const [lon, lat] = planner.fromDisplay([event.lngLat.lng, event.lngLat.lat]);
  const answer = await waypointDialog(targets, [lon, lat]);
  if (!answer) return;
  try {
    await api.addWaypoint(answer.path, answer.name, answer.description, lon, lat);
    await reloadEntry(answer.path);
    renderTracks();
    if (state.focus === answer.path) setFocus(answer.path, { fit: false });
    showMessage(`Added ${answer.name}`);
  } catch (error) {
    showMessage(error.message, true);
  }
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

// ---- opening files ----

// fileDrop takes GPX files dragged from the file manager onto the workspace
// and opens them there. The browser hands over no path for a dropped file,
// but the file manager also puts the files' URLs on the drag, and it is those
// the page reads — so this works only when the page runs on the machine the
// files are on, as the rest of opening by path does.
//
// Which browser this is decides whether it works at all. Safari and Firefox
// put the file URLs on a Finder or Explorer drag; Chrome and the browsers
// built on it put only the files themselves, whose paths a page may not see.
// There is nothing the page can do about that, so it says which it got rather
// than failing the same way for two different reasons.
function fileDrop(element) {
  const urls = (transfer) => transfer?.getData("text/uri-list") || transfer?.getData("text/x-moz-url") || transfer?.getData("text/plain") || "";
  const paths = (transfer) => urls(transfer)
    .split(/[\r\n]+/)
    .map((line) => line.trim())
    .filter((line) => line.startsWith("file://"))
    .map((line) => {
      try {
        return decodeURIComponent(new URL(line).pathname);
      } catch {
        return "";
      }
    })
    .filter((path) => /\.gpx$/i.test(path));

  const holdsFiles = (event) => [...(event.dataTransfer?.types || [])].some((type) => type === "Files" || type === "text/uri-list");

  element.addEventListener("dragover", (event) => {
    if (!holdsFiles(event)) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "copy";
    element.classList.add("dropping");
  });
  for (const name of ["dragleave", "dragend"]) {
    element.addEventListener(name, (event) => {
      if (name === "dragleave" && element.contains(event.relatedTarget)) return;
      element.classList.remove("dropping");
    });
  }
  element.addEventListener("drop", async (event) => {
    if (!holdsFiles(event)) return;
    event.preventDefault();
    element.classList.remove("dropping");
    const dropped = paths(event.dataTransfer);
    if (!dropped.length) {
      // A drag carrying files but no URL is the browser withholding the path:
      // the files are there, their names are there, where they are is not.
      const files = [...(event.dataTransfer?.files || [])];
      const gpx = files.filter((file) => /\.gpx$/i.test(file.name));
      if (gpx.length) {
        showMessage(`This browser does not tell the page where ${gpx.length === 1 ? gpx[0].name : "dropped files"} came from. Use Open… instead, or open this page in Safari or Firefox, which do.`, true);
      } else if (files.length) {
        showMessage("Only GPX files are opened here.", true);
      } else {
        showMessage("Drop GPX files from this machine's file manager", true);
      }
      return;
    }
    setFolder(dropped[0].replace(/[\\/][^\\/]*$/, ""));
    for (const path of dropped) await addTrack(path, { visible: true });
    save();
  });
}

// openFiles browses for GPX files anywhere on this machine and puts every
// chosen one in the workspace. Only GPX files can be chosen — a folder is
// stepped into, never answered — and the folder they came from is where the
// next dialog opens.
async function openFiles() {
  const folder = state.folder || state.config.root || "";
  const chosen = await openFile({
    title: "Open GPX",
    message: "Choose one or more GPX files. They are added to the workspace; nothing on disk changes.",
    folders: false, // only a file is answered: a folder in the listing is stepped into
    folder,
    fallbacks: [state.config.root || "", ""],
    filters: GPX_FILTERS,
    multiple: true,
    places: state.config.places || null,
  });
  if (!chosen || !chosen.length) return;
  setFolder(chosen[0].replace(/[\\/][^\\/]*$/, ""));
  for (const path of chosen) await addTrack(path, { visible: true });
  save();
}

// ---- planning a route ----

// saveRoute writes the planned route into a new GPX, chosen by path, and adds
// it to the workspace.
async function saveRoute() {
  const root = (state.folder || state.config.root || "").replace(/[\\/]$/, "");
  const folder = planner.source ? planner.source.replace(/[\\/][^\\/]*$/, "") : root;
  const target = await saveFile({
    title: "Save the route",
    message: "Choose the folder of the new GPX and name it. An existing file is not replaced; the waypoints are kept beside it, so the route can be changed and saved again.",
    folder,
    fallbacks: [state.config.root || "", ""],
    filters: GPX_FILTERS,
    places: state.config.places || null,
    name: `${planner.name}.gpx`,
    // The server writes a new file and refuses to write over one, so the
    // dialog refuses a name that is taken rather than offering to replace it.
    replace: false,
  });
  if (!target) return;
  try {
    const { path } = await api.saveRoute({ target, name: planner.name, plan: planner.toJSON(), writeRte: planner.writeRte });
    planner.source = path;
    showMessage(`Saved ${basename(path)}; it is in the workspace.`);
    if (state.tracks.has(path)) await reloadEntry(path);
    else await addTrack(path, { visible: true });
  } catch (error) {
    showMessage(error.message, true);
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
  status.clear();
  const box = planner.bounds();
  if (box) fitBox(box);
  save();
}

// ---- tracks added from other files, saved into a new GPX ----

// saveAs writes the workspace GPX as it is currently edited into a new file.
async function saveAs(path, { convert = false } = {}) {
  const entry = state.tracks.get(path);
  const dot = path.toLowerCase().lastIndexOf(".gpx");
  // What a draft holds: tracks, routes and waypoints, each counted only when
  // it has any, so a GPX of waypoints alone reads correctly.
  const counted = (n, one, many) => (n ? `${n} ${n === 1 ? one : many}` : "");
  const held = [
    counted(entry.track.parts.filter((part) => part.kind === "track").length, "track", "tracks"),
    counted(entry.track.parts.filter((part) => part.kind === "route").length, "route", "routes"),
    counted(entry.track.parts.filter((part) => part.kind === "waypoint").length, "waypoint", "waypoints"),
  ].filter(Boolean).join(", ") || "nothing yet";
  const root = (state.folder || state.config.root || "").replace(/[\\/]$/, "");
  const common = {
    fallbacks: [state.config.root || "", ""],
    filters: GPX_FILTERS,
    places: state.config.places || null,
    replace: false, // the server writes a new file and never writes over one
  };
  const target = await saveFile(entry.track.draft
    ? {
      ...common,
      title: "Save the new GPX",
      message: `${entry.track.name} holds ${held}, saved as they are being edited. Choose the folder to save it in and name it; an existing file is not replaced.`,
      folder: root,
      name: `${entry.track.name}.gpx`,
    }
    : {
      ...common,
      title: convert ? "Convert to a dgs GPX" : "Save as standalone GPX",
      message: convert
        ? `${entry.track.name} is copied into a new GPX dgs writes: the points as shown here, the names given here, and the cuts and route plans inside the file, so the copy needs no sidecar. ${basename(path)} itself is not written, and an existing file is not replaced.`
        : `${entry.track.name} is saved as a new self-contained GPX: edited points, fills, added tracks, names, cuts and route plan. No sidecar is needed. ${basename(path)} itself is not written, and an existing file is not replaced.`,
      folder: path.replace(/[\\/][^\\/]*$/, "") || root,
      name: `${basename(dot > 0 ? path.slice(0, dot) : path)}${convert ? " dgs" : " edited"}.gpx`,
    });
  if (!target) return;
  try {
    const result = await api.saveAs(path, target);
    const { color, visible, expanded } = entry;
    removeTrack(path);
    // The saved file takes the original's place, open as it was: its tracks
    // stay listed, to show one at a time and to rename.
    await addTrack(result.path, { color, visible, expanded });
    showMessage(convert ? `${basename(result.path)} is a dgs GPX; it needs no sidecar` : `Saved ${basename(result.path)}`);
  } catch (error) {
    showMessage(error.message, true);
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
    showMessage(error.message, true);
  }
}

// discardSidecar clears either a companion sidecar or embedded dgs edit state.
async function discardSidecar(path) {
  const entry = state.tracks.get(path);
  if (!entry) return;
  const embedded = entry.track.clean.embedded;
  const ok = await confirmDialog({
    title: embedded ? "Discard embedded edits?" : "Discard the sidecar?",
    message: `Everything done to ${entry.track.name} goes: removals, filter settings, cuts, segment names, fills and tracks added to it. ${embedded ? "The dgs extension in the GPX is removed; its tracks and waypoints remain." : "The GPX itself is not changed."} This cannot be undone.`,
    detail: embedded ? path : entry.track.clean.sidecar,
    confirm: "Discard",
    danger: true,
  });
  if (!ok) return;
  try {
    await api.discardSidecar(path);
    await reloadEntry(path);
  } catch (error) {
    showMessage(error.message, true);
    return;
  }
  if (state.focus === path) setFocus(path, { fit: false });
  else renderTracks();
}

// ---- unsaved edits: held in this dgs until they are saved ----

// Editing a GPX changes nothing on disk. What was done to a file is held in
// the dgs process serving this page, over what the file's sidecar says, and
// written only here. The cost is stated where it is paid: unsaved edits live
// in one dgs, so they go when dgs stops, and the page asks before it is
// reloaded or closed while any file holds some.

// dirtyPaths are the workspace files holding edits that are not on disk.
const dirtyPaths = () => [...state.tracks].filter(([, entry]) => entry.track.unsaved).map(([path]) => path);

// saveFiles writes the files' edits where each belongs: into the sidecar
// beside a recording, into the GPX itself when dgs wrote it. No paths saves
// every file holding unsaved edits.
async function saveFiles(paths = dirtyPaths()) {
  if (paths.length === 0) return;
  try {
    const { saved } = await api.saveFiles(paths);
    for (const path of saved) await reloadEntry(path);
    showMessage(saved.length === 1 ? `Saved ${basename(saved[0])}` : `Saved ${saved.length} files`);
  } catch (error) {
    showMessage(error.message, true);
  }
  // Saving changes what the file's row says about it — a sidecar now on disk,
  // the dot gone — and the Edit panels read the same file, so both redraw.
  if (state.focus && paths.includes(state.focus)) setFocus(state.focus, { fit: false });
  else renderTracks();
}

// revertFiles drops unsaved edits after asking: nothing on disk changes, and
// what is dropped cannot be got back.
async function revertFiles(paths = dirtyPaths()) {
  if (paths.length === 0) return;
  const names = paths.map((path) => state.tracks.get(path)?.track.name || basename(path));
  const ok = await confirmDialog({
    title: paths.length === 1 ? "Revert the file?" : `Revert ${paths.length} files?`,
    message: `Everything done since ${paths.length === 1 ? "it was" : "they were"} last saved goes: removals, filter settings, cuts, segment names, fills and tracks added. Nothing on disk changes. This cannot be undone.`,
    detail: names.join("\n"),
    confirm: "Revert",
    danger: true,
  });
  if (!ok) return;
  try {
    await api.revertFiles(paths);
    for (const path of paths) await reloadEntry(path);
  } catch (error) {
    showMessage(error.message, true);
    return;
  }
  if (state.focus && paths.includes(state.focus)) setFocus(state.focus, { fit: false });
  else renderTracks();
}

// closeTracks takes files out of the workspace. A file holding unsaved edits
// is asked about first, because closing it is the one way its edits are lost
// without being seen again: the file is not reopened with them.
async function closeTracks(paths) {
  const dirty = paths.filter((path) => state.tracks.get(path)?.track.unsaved);
  const drafts = paths.filter((path) => state.tracks.get(path)?.track.draft);
  if (dirty.length || drafts.length) {
    const ok = await confirmDialog({
      title: "Close without saving?",
      message: drafts.length
        ? "A new GPX that was never saved is not on disk at all: closing it drops it and everything added to it."
        : "Their edits are not on disk. Closing drops them; the files on disk are not changed.",
      detail: [...new Set([...drafts, ...dirty])].map((path) => state.tracks.get(path)?.track.name || basename(path)).join("\n"),
      confirm: "Close",
      danger: true,
    });
    if (!ok) return;
    if (dirty.length) await api.revertFiles(dirty).catch(() => {});
  }
  for (const path of paths) removeTrack(path);
  clearSelection();
  renderTracks();
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
    showMessage(error.message, true);
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
    cutPanel.show(entry.track, { timeZone: $("timezone").value, workspace: [...state.tracks.keys()], folder: state.folder || state.config.root, home: state.config.root || "", places: state.config.places || [] });
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
  // A new GPX joins the workspace, so its segments are there to see at once.
  if (request.target.mode === "create" && !state.tracks.has(result.path)) {
    await addTrack(result.path, { visible: true });
    save();
    return result;
  }
  // A workspace track written into shows what it gained.
  if (state.tracks.has(result.path)) {
    const entry = state.tracks.get(result.path);
    await reloadEntry(result.path);
    // A GPX that gained a track opens to list what it now holds, so the
    // tracks added to it are there to see, hide and rename at once.
    if (expandable(entry)) entry.expanded = true;
    renderTracks();
    save();
  }
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
      showMessage(error.message, true);
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
  // Saving every edited file at once is worth a control of its own; it is
  // there only while there is something to save.
  const dirty = dirtyPaths();
  const saveAll = $("save-all");
  saveAll.hidden = dirty.length === 0;
  saveAll.textContent = dirty.length === 1 ? "Save" : `Save ${dirty.length}`;
  saveAll.title = dirty.length === 1 ? "Write the edited file's edits to disk" : "Write every edited file's edits to disk";
  const anyShown = paths.some((path) => state.tracks.get(path).visible);
  const toggleAll = $("toggle-all");
  toggleAll.innerHTML = anyShown ? EYE : EYE_OFF;
  toggleAll.disabled = paths.length === 0;
  const title = anyShown ? "Hide every listed track" : "Show every listed track";
  toggleAll.title = title;
  toggleAll.setAttribute("aria-label", title);
  renderCoordinates();
}

// expandable: every file that holds anything lists its parts under its row —
// a file of one track too, so that track can be shown on its own, framed and
// renamed, whoever recorded it.
const expandable = (entry) => entry.track.parts.length > 0;

// The eye that shows or hides something on the map. Showing and hiding are
// one state, so one control carries it: the eye open when it is drawn, the
// eye struck through when it is not.
const EYE = `<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M1 8s2.6-4.2 7-4.2S15 8 15 8s-2.6 4.2-7 4.2S1 8 1 8z"/><circle cx="8" cy="8" r="1.9"/></svg>`;
const EYE_OFF = `<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M1 8s2.6-4.2 7-4.2S15 8 15 8s-2.6 4.2-7 4.2S1 8 1 8z"/><circle cx="8" cy="8" r="1.9"/><path d="M2.5 13.5 13.5 2.5"/></svg>`;

// iconButton is a button whose face is an icon rather than text.
function iconButton(className, icon, title, onClick) {
  const element = button(className, "", title, onClick);
  element.innerHTML = icon;
  return element;
}

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
  li.className = "track-row" + (state.focus === path ? " focused" : "") + (state.selection.has(path) ? " selected" : "")
    + (entry.track.unsaved ? " unsaved" : "") + (entry.visible ? "" : " hidden-track");
  // A name too long for the sidebar is cut with an ellipsis, so the tooltip
  // carries the whole name as well as where the file is.
  li.title = entry.track.name && entry.track.name !== path ? `${entry.track.name}\n${path}` : path;

  // The disclosure sits at the row's end, so rows start flush at the left.
  const disclosure = expandable(entry)
    && iconButton("disclosure", CHEVRON, entry.expanded ? "Collapse" : "List its tracks, routes and waypoints", () => {
      entry.expanded = !entry.expanded;
      renderTracks();
      save();
    });
  if (disclosure) {
    disclosure.classList.toggle("expanded", entry.expanded);
    li.classList.add("expandable");
  }

  const eye = iconButton("eye", entry.visible ? EYE : EYE_OFF, entry.visible ? "Hide on the map" : "Show on the map", () => setVisible(path, !entry.visible));
  eye.setAttribute("aria-pressed", String(entry.visible));

  const color = document.createElement("input");
  color.type = "color";
  color.className = "color";
  color.value = entry.color;
  color.title = "The colour this track is drawn in; click to change it";
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
    track.clean.error ? "dgs state unreadable" : track.clean.sidecar ? "sidecar" : track.clean.embedded ? "embedded edits" : "",
    track.draft ? `new, not saved${track.added ? "" : " — add segments from Segments, in Edit"}` : track.added ? `${track.added} added, not saved` : "",
    track.unsaved ? "edited, not saved" : "",
  ];

  // The row carries only what is looked at or reached for all the time:
  // showing the file on the map, where it is on this machine, and whether
  // dgs wrote it. Everything done to the file is in its context menu.
  const actions = document.createElement("span");
  actions.className = "actions";
  if (track.draft || track.added) li.classList.add("changed");
  if (track.clean.sidecar) li.classList.add("has-sidecar");
  actions.append(oursMark(track));
  if (state.config.canReveal && !track.draft) actions.append(revealButton(path));
  actions.append(eye);

  li.append(color, label(track.name, details), actions);
  if (disclosure) li.append(disclosure);
  // A right click on a file offers what is done to the file as a whole:
  // writing it out, then taking it out of the workspace.
  li.addEventListener("contextmenu", (event) => {
    const chosen = selectionFor(path);
    renderTracks();
    if (chosen.length > 1) {
      openMenu(event, manyMenu(chosen));
      return;
    }
    openMenu(event, [
    [{
      label: "Save",
      title: track.unsaved
        ? (track.ours ? "Write the edits into this GPX, which dgs wrote" : "Write the edits into the sidecar beside this GPX")
        : "Nothing is edited that is not on disk",
      disabled: !track.unsaved,
      onSelect: () => saveFiles([path]),
    }, {
      label: "Revert",
      title: track.unsaved ? "Drop the unsaved edits: the file reads as it is on disk again" : "Nothing is edited that is not on disk",
      disabled: !track.unsaved,
      danger: true,
      onSelect: () => revertFiles([path]),
    }],
    [{
      label: track.draft ? "Save…" : "Save as…",
      title: track.draft ? "Save this new GPX to a file" : "Save the edited GPX and dgs state as one standalone file",
      disabled: Boolean(track.draft && track.parts.length === 0),
      onSelect: () => saveAs(path),
    }, {
      label: "Convert to a dgs GPX…",
      title: track.ours
        ? "This GPX is already one dgs wrote"
        : "Copy this file into a new GPX dgs writes, which keeps its edits inside itself and needs no sidecar",
      disabled: Boolean(track.draft || track.ours),
      onSelect: () => saveAs(path, { convert: true }),
    }],
    [
      track.clean.sidecar && {
        label: "Discard the sidecar…",
        title: `Discard the sidecar ${track.clean.sidecar}: the GPX reads as recorded again`,
        danger: true,
        onSelect: () => discardSidecar(path),
      },
      track.clean.embedded && {
        label: "Discard the edits…",
        title: "Discard dgs edits embedded in this GPX",
        danger: true,
        onSelect: () => discardSidecar(path),
      },
    ],
    [{ label: "Close", title: "Remove from the workspace; nothing on disk changes", onSelect: () => closeTracks([path]) }],
    ]);
  });
  // Clicking a hidden track shows it: only a shown track can be focused.
  li.addEventListener("click", (event) => {
    const toggle = event.metaKey || event.ctrlKey;
    select(path, { toggle, range: event.shiftKey });
    // Picking several rows out is not looking at one: only a plain click
    // moves the map and the profile to a file.
    if (toggle || event.shiftKey) {
      renderTracks();
      return;
    }
    if (!entry.visible) setVisible(path, true);
    setFocus(path);
  });
  bindRowDrag(li, { kind: "file", path });
  return li;
}

// oursMark says whether dgs wrote this GPX. A file dgs wrote keeps what is
// done to it inside itself; any other file keeps it in a sidecar beside it,
// so which one it is decides what a later edit writes.
function oursMark(track) {
  const mark = document.createElement("span");
  mark.className = "ours-mark" + (track.ours ? " ours" : "");
  mark.innerHTML = track.ours ? DIAMOND : DIAMOND_OPEN;
  const title = track.draft
    ? "A new GPX, not saved yet; dgs writes it"
    : track.ours
      ? "dgs wrote this GPX: what is done to it is kept inside the file"
      : "dgs did not write this GPX: what is done to it is kept in a sidecar beside it";
  mark.title = title;
  mark.setAttribute("aria-label", title);
  return mark;
}

// manyMenu is what is offered for several files at once: what can be done to
// each of them without naming a file. Saving and converting name a new file
// one at a time, so they are on a single row's menu only.
function manyMenu(paths) {
  const files = `${paths.length} files`;
  const shown = paths.filter((path) => state.tracks.get(path).visible).length;
  const dirty = paths.filter((path) => state.tracks.get(path).track.unsaved);
  return [
    [{
      label: dirty.length ? `Save ${dirty.length} of them` : "Save",
      title: dirty.length ? "Write their edits where each belongs" : "None of them is edited beyond what is on disk",
      disabled: dirty.length === 0,
      onSelect: () => saveFiles(dirty),
    }, {
      label: dirty.length ? `Revert ${dirty.length} of them` : "Revert",
      title: dirty.length ? "Drop their unsaved edits: they read as they are on disk again" : "None of them is edited beyond what is on disk",
      disabled: dirty.length === 0,
      danger: true,
      onSelect: () => revertFiles(dirty),
    }],
    [{
      label: shown ? `Hide ${files}` : `Show ${files}`,
      title: shown ? "Hide them on the map; they stay in the workspace" : "Show them on the map",
      onSelect: () => {
        for (const path of paths) setVisible(path, shown === 0);
      },
    }],
    [{
      label: `Close ${files}`,
      title: "Remove them from the workspace; nothing on disk changes",
      onSelect: () => closeTracks(paths),
    }],
  ];
}

const PART_GLYPH = { track: "〰", route: "┅", waypoint: "⚑" };

function partRow(path, entry, part) {
  const hidden = entry.hiddenParts.has(part.key);
  const li = document.createElement("li");
  li.className = "part-row" + (hidden || !entry.visible ? " hidden-track" : "");
  li.title = `${part.name}${part.description ? ` — ${part.description}` : ""}`;

  const eye = iconButton("eye", hidden ? EYE_OFF : EYE, hidden ? "Show this part" : "Hide this part", () => setPartHidden(path, part.key, !hidden));
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
  const actions = document.createElement("span");
  actions.className = "actions";
  const ours = Boolean(entry.track.ours || entry.track.draft);
  // Tracks, routes and waypoints of a GPX dgs wrote are put in order a step
  // at a time, each among its own kind.
  const kin = entry.track.parts.filter((one) => one.kind === part.kind).map((one) => one.key);
  if (ours && kin.length > 1) {
    const keys = kin;
    const at = keys.indexOf(part.key);
    const up = button("order", "▲", `Move this ${part.kind} up`, () => reorderParts({ kind: part.kind, path, key: part.key }, keys[at - 1]));
    const down = button("order", "▼", `Move this ${part.kind} down`, () => reorderParts({ kind: part.kind, path, key: part.key }, keys[at + 2] ?? null));
    up.disabled = at <= 0;
    down.disabled = at >= keys.length - 1;
    actions.append(up, down);
  }
  actions.append(eye);
  li.append(glyph, label(part.name, details), actions);
  if (part.added) li.classList.add("added");
  // A right click on a track, route or waypoint offers what is done to that
  // part alone.
  // A part of a GPX dgs wrote is ours to take out; so is one carried beside a
  // recording, which the recording itself never holds.
  const takeable = ours || Boolean(part.added);
  const targets = copyTargets(path);
  const kind = part.kind;
  li.addEventListener("contextmenu", (event) => openMenu(event, [
    [{ label: "Rename…", title: "Rename this part", onSelect: () => renamePart(path, entry, part) }],
    [
      {
        label: "Copy to…",
        title: targets.length ? `Copy this ${kind} into another GPX dgs wrote` : "Open or start a GPX dgs wrote to copy into",
        disabled: targets.length === 0,
        onSelect: () => copyPartTo(path, part, targets, false),
      },
      {
        label: "Move to…",
        title: !takeable
          ? "This GPX was not written by dgs, so nothing is taken out of it; copy the part instead"
          : targets.length ? `Copy this ${kind} into another GPX dgs wrote and take it out of this one` : "Open or start a GPX dgs wrote to move it into",
        disabled: targets.length === 0 || !takeable,
        onSelect: () => copyPartTo(path, part, targets, true),
      },
    ],
    kind === "waypoint" ? [{
      label: "Move on the map…",
      title: takeable ? "Drag the pin to where the waypoint belongs" : "This GPX was not written by dgs, so its waypoints stay where they were recorded",
      disabled: !takeable,
      onSelect: () => startWaypointMove(path, part),
    }] : [],
    [{
      label: "Delete",
      title: takeable
        ? (part.added ? `Take this ${kind} out again` : `Take this ${kind} out of the GPX when the edit is saved`)
        : "This GPX was not written by dgs, so nothing is taken out of it",
      danger: true,
      disabled: !takeable,
      onSelect: () => deletePart(path, entry, part),
    }],
  ]));
  li.addEventListener("click", () => showPart(path, part));
  bindRowDrag(li, { kind: part.kind, path, key: part.key });
  return li;
}

// ---- dragging rows in the workspace ----
//
// A row is taken hold of by pressing it and holding still; there is no drag
// handle, so the row keeps its controls. What a drop does depends on what is
// held and where it lands:
//   a file over another file      the workspace is listed in that order
//   a part over another file      it is moved into that GPX, or copied into
//                                 it while Ctrl or Cmd is held
// A GPX dgs did not write is never written to, so a drop on one is refused
// and says why; so is a drop that would take a part out of one.

let dragRow = null; // what is held: { kind, path, key, row }
let dragGap = null; // the SortGap sliding the other rows aside

function bindRowDrag(li, held) {
  li.dataset.path = held.path;
  li.dataset.kind = held.kind;
  if (held.key) li.dataset.key = held.key;
  longPressDrag(li, {
    onStart: () => {
      dragRow = { ...held, row: li };
      const rows = [...$("track-list").children].filter((row) => !row.classList.contains("drag-ghost"));
      // A file is carried with every row under it, down to the next file.
      let carried = [li];
      if (held.kind === "file") {
        const first = rows.indexOf(li);
        let end = first + 1;
        while (end < rows.length && rows[end].dataset.kind !== "file") end++;
        carried = rows.slice(first, end);
      }
      dragGap = new SortGap(rows, carried);
      li.classList.add("dragging");
      document.body.classList.add("row-dragging");
    },
    onMove: (x, y, event) => markDrop(dropAt(x, y, event)),
    onDrop: (x, y, event) => {
      const target = dropAt(x, y, event);
      const held = dragRow;
      endRowDrag();
      applyDrop(held, target);
    },
    onCancel: endRowDrag,
  });
}

function endRowDrag() {
  dragRow = null;
  dragGap?.close();
  dragGap = null;
  document.body.classList.remove("row-dragging");
  for (const row of $("track-list").children) {
    row.classList.remove("dragging", "drop-above", "drop-below", "drop-into", "drop-refused");
  }
}

// copyModifier says whether a part is copied rather than moved: Cmd on a Mac,
// Ctrl elsewhere, either of which reads as a copy here.
const copyModifier = (event) => Boolean(event && (event.metaKey || event.ctrlKey));

// dropAt reads what a drop at a screen point would do. It answers
//   { kind: "files", before }              put in that order, before that row
//   { kind: "into", path, copy }          moved or copied into that file
//   { kind: "refused", reason, row }      nothing, and why
// or null when the point is nowhere the drop means anything.
function dropAt(x, y, event) {
  const held = dragRow;
  if (!held) return null;
  // The rows slide while one is carried, so the point is read against where
  // they lay when the drag began.
  const list = $("track-list").getBoundingClientRect();
  if (x < list.left || x > list.right) return null;
  const hit = dragGap?.at(y);
  if (!hit || !hit.row.dataset.path) return null;
  const { row, above } = hit;
  const targetPath = row.dataset.path;
  const targetEntry = state.tracks.get(targetPath);
  if (!targetEntry) return null;

  if (held.kind === "file") {
    // A file lands between files; the rows of a file's parts belong to it.
    if (targetPath === held.path) return null;
    return { kind: "files", before: above ? targetPath : afterFile(targetPath) };
  }

  const heldEntry = state.tracks.get(held.path);
  if (!heldEntry) return null;
  // Inside its own file a part is put in order by its arrows, not dragged.
  if (targetPath === held.path) return null;


  const copy = copyModifier(event);
  if (!writable(targetEntry)) {
    return { kind: "refused", row, reason: `${targetEntry.track.name} was not written by dgs, so nothing is written into it.` };
  }
  if (!copy && !takeable(heldEntry, held.key)) {
    return { kind: "refused", row, reason: `${heldEntry.track.name} was not written by dgs, so nothing is taken out of it; hold Ctrl or Cmd to copy instead.` };
  }
  return { kind: "into", path: targetPath, copy };
}

// writable says whether a GPX is one dgs wrote, or a new one not saved yet:
// only those are written into.
const writable = (entry) => Boolean(entry.track.ours || entry.track.draft);

// takeable says whether a part may be taken out of its file: out of a GPX dgs
// wrote, or one carried beside a recording until it is saved.
function takeable(entry, key) {
  const part = entry.track.parts.find((one) => one.key === key);
  return writable(entry) || Boolean(part?.added);
}

// afterFile is the file listed after this one, or null at the end of the list.
function afterFile(path) {
  const paths = [...state.tracks.keys()];
  return paths[paths.indexOf(path) + 1] ?? null;
}

// afterPart is the key of the part of that kind listed after this one, or
// null at the end.
function afterPart(entry, kind, key) {
  const keys = entry.track.parts.filter((part) => part.kind === kind).map((part) => part.key);
  return keys[keys.indexOf(key) + 1] ?? null;
}

// markDrop shows where the drop lands: the rows slide aside to open the gap
// between them, the whole file is outlined when a part lands in another file,
// and a refused row turns red.
function markDrop(target) {
  const rows = [...$("track-list").children].filter((row) => !row.classList.contains("drag-ghost"));
  for (const row of rows) row.classList.remove("drop-into", "drop-refused");
  if (!target || target.kind === "refused" || target.kind === "into") dragGap?.open(null);
  if (!target) return;
  if (target.kind === "refused") {
    target.row?.classList.add("drop-refused");
    return;
  }
  if (target.kind === "into") {
    for (const row of rows) {
      if (row.dataset.path === target.path) row.classList.add("drop-into");
    }
    return;
  }
  dragGap?.open(slotIndex(rows, target));
}

// slotIndex is the index of the row a reorder would go before.
function slotIndex(rows, target) {
  const before = target.before;
  if (target.kind === "files") {
    if (before == null) return rows.length;
    return rows.findIndex((one) => one.dataset.path === before && one.dataset.kind === "file");
  }
  const path = dragRow?.path;
  if (before != null) return rows.findIndex((one) => one.dataset.key === before && one.dataset.path === path);
  // The end of the parts of that kind: just after the last of them.
  let last = -1;
  rows.forEach((one, i) => {
    if (one.dataset.path === path && one.dataset.kind === dragRow.kind) last = i;
  });
  return last + 1;
}

// applyDrop carries the drop out.
async function applyDrop(held, target) {
  if (!held || !target) return;
  if (target.kind === "refused") {
    showMessage(target.reason, true);
    return;
  }
  if (target.kind === "files") {
    reorderFiles(held.path, target.before);
    return;
  }
  await dropIntoFile(held, target);
}

// reorderFiles lists the workspace in another order. The order is the page's
// own: it is remembered with the rest of the session and changes no file.
function reorderFiles(path, before) {
  const paths = [...state.tracks.keys()].filter((one) => one !== path);
  const at = before == null ? paths.length : paths.indexOf(before);
  paths.splice(at < 0 ? paths.length : at, 0, path);
  const entries = new Map(state.tracks);
  state.tracks = new Map(paths.map((one) => [one, entries.get(one)]));
  renderTracks();
  save();
}

// reorderParts puts the routes or waypoints of one file in another order. The
// order is held beside the file and written into the GPX when it is saved.
async function reorderParts(held, before) {
  const entry = state.tracks.get(held.path);
  if (!entry) return;
  const keys = entry.track.parts.filter((part) => part.kind === held.kind).map((part) => part.key);
  const without = keys.filter((key) => key !== held.key);
  const at = before == null ? without.length : without.indexOf(before);
  without.splice(at < 0 ? without.length : at, 0, held.key);
  if (without.join() === keys.join()) return;
  try {
    await api.orderParts(held.path, held.kind, without);
    await reloadEntry(held.path);
    renderTracks();
    if (state.focus === held.path) setFocus(held.path, { fit: false });
  } catch (error) {
    showMessage(error.message, true);
  }
}

// dropIntoFile moves a part into another GPX of the workspace, or copies it
// there when Ctrl or Cmd was held.
async function dropIntoFile(held, target) {
  const entry = state.tracks.get(held.path);
  const part = entry?.track.parts.find((one) => one.key === held.key);
  if (!part) return;
  try {
    await api.copyPart(held.path, held.key, target.path, !target.copy);
    await reloadEntry(target.path);
    if (!target.copy) await reloadEntry(held.path);
    renderTracks();
    if (state.focus && (state.focus === held.path || state.focus === target.path)) setFocus(state.focus, { fit: false });
    const name = state.tracks.get(target.path)?.track.name || basename(target.path);
    showMessage(`${target.copy ? "Copied" : "Moved"} ${part.name} to ${name}`);
  } catch (error) {
    showMessage(error.message, true);
  }
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
    showMessage(`Renamed to ${name}`);
  } catch (error) {
    showMessage(error.message, true);
  }
}

// copyTargets are the GPX files of the workspace a part may be copied into:
// those dgs wrote and those not saved yet, other than the file it is in. A
// recording is never written into.
function copyTargets(path) {
  return [...state.tracks]
    .filter(([other, entry]) => other !== path && (entry.track.ours || entry.track.draft))
    .map(([other, entry]) => ({ value: other, label: `${entry.track.name} — ${other}` }));
}

// copyPartTo copies one part into another GPX of the workspace, and takes it
// out of this one when it is moved rather than copied.
async function copyPartTo(path, part, targets, move) {
  const target = await chooseDialog({
    title: `${move ? "Move" : "Copy"} ${part.name}`,
    message: move
      ? "The part is copied into that GPX and taken out of this one. Neither file is written until it is saved."
      : "The part is copied into that GPX, as this page shows it. Neither file is written until it is saved.",
    options: targets,
    confirm: move ? "Move" : "Copy",
  });
  if (!target) return;
  try {
    await api.copyPart(path, part.key, target, move);
    await reloadEntry(target);
    if (move) await reloadEntry(path);
    renderTracks();
    if (state.focus && (state.focus === path || state.focus === target)) setFocus(state.focus, { fit: false });
    showMessage(`${move ? "Moved" : "Copied"} ${part.name} to ${basename(target)}`);
  } catch (error) {
    showMessage(error.message, true);
  }
}

// deletePart takes one track, route or waypoint out of a file. A part the GPX
// itself holds goes out of the file when the edit is saved, so it is asked
// for first; one carried beside the file goes as it came.
async function deletePart(path, entry, part) {
  if (!part.added) {
    const ok = await confirmDialog({
      title: `Delete ${part.name}`,
      message: `The ${part.kind} is taken out of ${entry.track.name} when the edit is saved. Until then, Revert puts it back.`,
      confirm: "Delete",
      danger: true,
    });
    if (!ok) return;
  }
  try {
    await api.deletePart(path, part.key);
    await reloadEntry(path);
    renderTracks();
    if (state.focus === path) setFocus(path, { fit: false });
    showMessage(`Deleted ${part.name}`);
  } catch (error) {
    showMessage(error.message, true);
  }
}

// A waypoint pinned in the wrong place is moved by hand: the pin becomes a
// draggable one, and where it is dropped is where the waypoint goes. Escape
// leaves it where it was. Only a GPX dgs wrote is moved in.
let movingWaypoint = null;

function startWaypointMove(path, part) {
  cancelWaypointMove();
  const element = document.createElement("div");
  element.className = "waypoint-move";
  const marker = new maplibregl.Marker({ element, draggable: true }).setLngLat(part.points[0]).addTo(map);
  movingWaypoint = { path, key: part.key, marker };
  showMessage(`Drag ${part.name} to where it belongs; Esc leaves it where it was.`);
  waypointPopup.remove();
  marker.on("dragend", async () => {
    const at = marker.getLngLat();
    const [lon, lat] = planner.fromDisplay([at.lng, at.lat]);
    cancelWaypointMove();
    try {
      await api.moveWaypoint(path, part.key, lon, lat);
      await reloadEntry(path);
      renderTracks();
      if (state.focus === path) setFocus(path, { fit: false });
      showMessage(`Moved ${part.name}`);
    } catch (error) {
      showMessage(error.message, true);
    }
  });
}

function cancelWaypointMove() {
  if (!movingWaypoint) return;
  movingWaypoint.marker.remove();
  movingWaypoint = null;
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
    const allHidden = keys.every((key) => entry.hiddenParts.has(key));
    const toggle = iconButton("mini", allHidden ? EYE_OFF : EYE, `${allHidden ? "Show" : "Hide"} ${text.toLowerCase()}`, () => setPartsHidden(path, keys, !allHidden));
    box.append(name, toggle);
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
// fitCharts shows the whole of the focused track again: both charts and the
// timeline drop whatever stretch was zoomed into, and the map frames it.
function fitCharts() {
  const entry = focusedEntry();
  if (!entry) return;
  timeline.setView(null);
  profile.setRange(null);
  // Frame the line the timeline shows: its kept points, not the file's
  // routes, waypoints or hidden tracks, which would pull the frame aside.
  const { track } = entry;
  const hidden = hiddenRanges(entry);
  let w = Infinity, s = Infinity, e = -Infinity, n = -Infinity;
  track.points.forEach(([lon, lat], i) => {
    if (track.removed[i] || hidden.some(([first, last]) => i >= first && i <= last)) return;
    w = Math.min(w, lon); s = Math.min(s, lat); e = Math.max(e, lon); n = Math.max(n, lat);
  });
  if (w <= e) fitBox([[w, s], [e, n]]);
  else fit([track]);
}

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

// waypointAt is the waypoint drawn under a screen point, as { path, entry,
// part }, or null. A few pixels around the point count, so a small dot is
// still found by hand.
function waypointAt(point, radius = 6) {
  const ids = layers.waypointLayers();
  if (!ids.length) return null;
  const box = [[point.x - radius, point.y - radius], [point.x + radius, point.y + radius]];
  const hit = map.queryRenderedFeatures(box, { layers: ids })[0];
  if (!hit) return null;
  for (const [path, entry] of state.tracks) {
    if (entry.id !== hit.layer.metadata.track) continue;
    const part = entry.track.parts.find((p) => p.key === hit.properties.part);
    if (part) return { path, entry, part };
  }
  return null;
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
      // A waypoint under the pointer answers it: it grows a little and the
      // pointer says it can be opened.
      const found = waypointAt(point);
      layers.setHover(found ? found.entry.id : null, found ? found.part.key : null);
      // In cut mode the pointer says a click cuts at the marked point.
      map.getCanvas().style.cursor = found ? "pointer"
        : index >= 0 ? (cutting() ? "copy" : "crosshair") : state.fill.active || state.routeOpen ? "crosshair" : "";
      inspect(index);
      profile.setIndex(index >= 0 ? index : null);
    });
  });
  map.on("mouseout", () => {
    pending = null;
    inspect(null);
    profile.setIndex(null);
    layers.setHover(null, null);
  });
  // A right click on a waypoint offers what is done to that waypoint: its
  // name, and where it is pinned. Elsewhere the map keeps its own menu.
  map.on("contextmenu", (event) => {
    const found = waypointAt(event.point);
    if (!found) return;
    event.preventDefault();
    const { path, entry, part } = found;
    const takeable = Boolean(entry.track.ours || entry.track.draft || part.added);
    openMenu(event.originalEvent, [
      [{ label: "Rename…", title: "Rename this waypoint", onSelect: () => renamePart(path, entry, part) }],
      [{
        label: "Move to a new position…",
        title: takeable ? "Drag the pin to where the waypoint belongs" : "This GPX was not written by dgs, so its waypoints stay where they were recorded",
        disabled: !takeable,
        onSelect: () => startWaypointMove(path, part),
      }],
      [{
        label: "Delete",
        title: takeable ? "Take this waypoint out of the GPX when the edit is saved" : "This GPX was not written by dgs, so nothing is taken out of it",
        danger: true,
        disabled: !takeable,
        onSelect: () => deletePart(path, entry, part),
      }],
    ]);
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
    if (state.cleanOpen && state.waypointTool) {
      addWaypointAt(event);
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
    const found = waypointAt(event.point);
    if (found) {
      openWaypoint(found.entry, found.part);
      return;
    }
    const box = [[event.point.x - 4, event.point.y - 4], [event.point.x + 4, event.point.y + 4]];
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
    handle: $("sidebar-splitter"), target: sidebar, axis: "x", min: 220,
    max: () => window.innerWidth - 320, key: "sidebar",
  });
  splitter({
    handle: $("profile-splitter"), target: $("profile"), invert: true, min: 180,
    max: () => document.querySelector(".workspace").clientHeight - 120, key: "profile",
  });
  splitter({
    handle: $("inspector-splitter"), target: $("inspector"), axis: "x", invert: true, min: 300,
    max: () => Math.min(640, window.innerWidth - 560), key: "inspector",
  });
  chartSplit = shareSplitter({
    handles: { y: $("chart-splitter"), x: $("chart-splitter-x") },
    first: $("row-elevation"), second: $("row-speed"), container: $("charts"),
    axis: () => (state.charts.layout === "side" ? "x" : "y"), min: 100, key: "chart-share",
  });
}

// ---- charts: each can be hidden; both show stacked or side by side ----

function applyCharts() {
  if (!chartSplit) return; // before the splitters are bound there is nothing to lay out
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
  $("chart-fit").hidden = charts.hidden;
  $("layout-toggles").hidden = !both;
  $("profile").classList.toggle("no-charts", charts.hidden);
  $("profile-splitter").hidden = !state.focus || charts.hidden;
  // Folded away by hand: the charts go, and the map carries the tab that
  // brings them back.
  if (state.hidden.profile) {
    $("profile").hidden = true;
    $("profile-splitter").hidden = true;
  } else {
    // Unfolded again: the charts come back if there is a track to chart.
    // Only Profile.show fills them, so an empty profile stays away.
    $("profile").hidden = !state.focus;
  }
  // The tab is there whenever the charts are folded, focused track or not:
  // the reader folded them away, so the way back belongs to them, not to
  // whether there is something to chart right now.
  $("show-profile").hidden = !state.hidden.profile;
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
  // Fit puts the whole track back across both charts and the timeline, for a
  // look at all of it after zooming into a stretch.
  $("chart-fit").addEventListener("click", fitCharts);
  $("toggle-elevation").addEventListener("click", () => change({ elevation: !state.charts.elevation }));
  $("toggle-speed").addEventListener("click", () => change({ speed: !state.charts.speed }));
  $("layout-stacked").addEventListener("click", () => change({ layout: "stacked" }));
  $("layout-side").addEventListener("click", () => change({ layout: "side" }));
  applyCharts();
}

// ---- folding the panels away ----
//
// Each panel carries the button that hides it; the map then carries a tab at
// that edge to bring it back. *Map only* folds every panel away at once and
// remembers what was open, so pressing it again puts the page back as it was.

function applyPanels() {
  const sidebar = document.querySelector(".sidebar");
  sidebar.hidden = state.hidden.sidebar;
  $("sidebar-splitter").hidden = state.hidden.sidebar;
  $("show-workspace").hidden = !state.hidden.sidebar;
  // The charts and the edit panel are hidden by what the page is doing as
  // well — no focused track, Browse mode — so each is folded where it is
  // decided, and the tab only appears when the panel would otherwise show.
  applyCharts();
  applyCleanPanel();
  $("map-only").setAttribute("aria-pressed", String(mapOnly()));
  // The map and the charts measure themselves; a folded chart has nothing to
  // measure, so it is left alone until it comes back.
  map?.resize();
  if (!$("profile").hidden) profile?.resize();
}

// mapOnly says whether every panel that could show is folded away.
const mapOnly = () => state.hidden.sidebar && state.hidden.profile && state.hidden.inspector;

function hidePanel(name, hide) {
  state.hidden[name] = hide;
  applyPanels();
  save();
}

// setMapOnly folds every panel away, or puts back the ones that were open.
let panelsBeforeMapOnly = null;

function setMapOnly(on) {
  if (on) {
    panelsBeforeMapOnly = { ...state.hidden };
    state.hidden = { sidebar: true, profile: true, inspector: true };
  } else {
    state.hidden = panelsBeforeMapOnly && !Object.values(panelsBeforeMapOnly).every(Boolean)
      ? panelsBeforeMapOnly
      : { sidebar: false, profile: false, inspector: false };
    panelsBeforeMapOnly = null;
  }
  applyPanels();
  save();
}

function bindPanels(saved) {
  if (saved && typeof saved === "object") {
    for (const name of ["sidebar", "profile", "inspector"]) state.hidden[name] = saved[name] === true;
  }
  $("hide-workspace").addEventListener("click", () => hidePanel("sidebar", true));
  $("show-workspace").addEventListener("click", () => hidePanel("sidebar", false));
  $("hide-profile").addEventListener("click", () => hidePanel("profile", true));
  $("show-profile").addEventListener("click", () => hidePanel("profile", false));
  $("hide-inspector").addEventListener("click", () => hidePanel("inspector", true));
  $("show-inspector").addEventListener("click", () => hidePanel("inspector", false));
  $("map-only").addEventListener("click", () => setMapOnly(!mapOnly()));
  applyPanels();
}

// ViewControl sits under the compass: buttons that tilt the map and turn it
// by a fixed step, for those without a right-drag or two fingers to hand.
// Holding a button keeps it going.
const TILT_STEP = 10; // degrees of pitch per press
const TURN_STEP = 15; // degrees of bearing per press
class ViewControl {
  onAdd(map) {
    this.map = map;
    const box = document.createElement("div");
    box.className = "maplibregl-ctrl maplibregl-ctrl-group view-control";
    const buttons = [
      ["↥", "Tilt up — look further toward the horizon", () => ({ pitch: Math.min(map.getMaxPitch(), map.getPitch() + TILT_STEP) })],
      ["↧", "Tilt down — look straight down", () => ({ pitch: Math.max(0, map.getPitch() - TILT_STEP) })],
      ["↺", "Turn left", () => ({ bearing: map.getBearing() - TURN_STEP })],
      ["↻", "Turn right", () => ({ bearing: map.getBearing() + TURN_STEP })],
    ];
    for (const [glyph, title, next] of buttons) {
      const button = document.createElement("button");
      button.type = "button";
      button.title = title;
      button.setAttribute("aria-label", title);
      button.innerHTML = `<span aria-hidden="true">${glyph}</span>`;
      let repeat = null;
      const step = () => map.easeTo({ ...next(), duration: 200 });
      const stop = () => { clearInterval(repeat); repeat = null; };
      button.addEventListener("pointerdown", (event) => {
        if (event.button !== 0) return;
        step();
        stop();
        repeat = setInterval(step, 200);
      });
      for (const name of ["pointerup", "pointerleave", "pointercancel"]) button.addEventListener(name, stop);
      // The keyboard presses through click, which a pointer press already did.
      button.addEventListener("click", (event) => { if (event.detail === 0) step(); });
      box.appendChild(button);
    }
    this.box = box;
    return box;
  }
  onRemove() {
    this.box.remove();
  }
}

// bindTerrain wires the 3D relief: one switch and its vertical stretch.
function bindTerrain(saved) {
  if (Number.isFinite(saved?.exaggeration)) {
    state.terrain.exaggeration = Math.min(8, Math.max(1, saved.exaggeration));
  }
  const toggle = $("terrain-toggle");
  const slider = $("terrain-exaggeration");
  const value = $("terrain-exaggeration-value");
  // The relief is turned on from the map itself as well: it is looked at far
  // more often than the rest of the layer stack, so it has a button of its
  // own beside the map, and the stretch stays in the panel.
  const button = $("terrain-3d");
  const show = () => {
    toggle.checked = state.terrain.on;
    slider.value = String(state.terrain.exaggeration);
    value.textContent = `${state.terrain.exaggeration}×`;
    toggle.closest(".layer-terrain").classList.toggle("off", !state.terrain.on);
    button.setAttribute("aria-pressed", String(state.terrain.on));
    button.title = state.terrain.on
      ? `3D relief on at ${state.terrain.exaggeration}× — click to level the map (D)`
      : "3D relief — tilt the map and raise the ground (D)";
  };
  const setOn = (on) => {
    state.terrain.on = on;
    terrain.show(on);
    show();
  };
  toggle.addEventListener("change", () => setOn(toggle.checked));
  button.addEventListener("click", () => setOn(!state.terrain.on));
  terrainToggle = () => setOn(!state.terrain.on);
  slider.addEventListener("input", () => {
    state.terrain.exaggeration = Number(slider.value);
    terrain.setExaggeration(state.terrain.exaggeration);
    show();
  });
  slider.addEventListener("change", save);
  show();
}

// The layers popover opens under its button and closes on a click outside it.
function bindLayersPanel() {
  const panel = $("layers-panel");
  const toggle = $("layers-toggle");
  // place keeps the popover inside the window: it starts under the button's
  // left edge and moves left as far as the window's right edge asks.
  const place = () => {
    if (panel.hidden) return;
    panel.style.left = "0px";
    const margin = 8;
    const box = panel.getBoundingClientRect();
    const over = box.right - (window.innerWidth - margin);
    if (over > 0) panel.style.left = `${-Math.min(over, box.left - margin)}px`;
  };
  const open = (on) => {
    panel.hidden = !on;
    toggle.setAttribute("aria-expanded", String(on));
    place();
  };
  toggle.addEventListener("click", () => open(panel.hidden));
  window.addEventListener("resize", place);
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

  // A remembered folder holds only while geo.gpx.root is what it was when it
  // was remembered: changing the configuration opens at the new root.
  const folderStillApplies = saved.folder && saved.configRoot === state.config.root;
  state.folder = folderStillApplies ? saved.folder : state.config.root || "";
  $("open-gpx").addEventListener("click", openFiles);
  fileDrop(document.querySelector(".sidebar"));
  bindSplitters();
  bindCharts(saved.charts);

  map = new maplibregl.Map({
    container: "map",
    style: { version: 8, sources: {}, layers: [] },
    center: [120.15, 30.25],
    zoom: 9,
    // The credits fold into an ⓘ in the corner; a click opens them.
    attributionControl: { compact: true },
  });
  map.addControl(new maplibregl.NavigationControl(), "top-right");
  map.addControl(new ViewControl(), "top-right");
  map.addControl(new maplibregl.ScaleControl({ unit: "metric" }), "bottom-left");
  await new Promise((resolve) => map.once("load", resolve));

  stack.map = map;
  stack.contours = new Contours(map, state.config.dem);
  // The hillshade goes under the tracks: above every base map, below the
  // first layer the map stack does not own.
  stack.apply();
  renderStack();
  layers = new TrackLayers(map);
  // The hillshade goes under the tracks: above every base map, below the
  // first layer the map stack does not own.
  terrain = new Terrain(map, state.config.dem, {
    under: () => map.getStyle().layers.find((layer) => !stack.owns(layer.id) && layer.id !== "terrain-hillshade")?.id,
  });
  bindTerrain(saved.terrain);
  terrain.setExaggeration(state.terrain.exaggeration);
  bindPanels(saved.hiddenPanels);
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
  });
  lasso = new Lasso(map, { onDone: lassoDone, onCancel: () => setLasso(false) });
  if (saved.cleanOpen === true) state.cleanOpen = true;
  if (saved.compare === false) state.compare = false;
  cleanPanel.ways = state.config.ways || [];
  if (cleanPanel.ways.some((way) => way.id === saved.fillProfile)) state.fill.profile = saved.fillProfile;
  $("tool-compare").addEventListener("click", () => setCompare(!state.compare));
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
  for (const name of ["select", "range", "lasso", "fill", "cut", "waypoint"]) $(`tool-${name}`).addEventListener("click", () => setTool(name));
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
      if (movingWaypoint) { cancelWaypointMove(); showMessage("The waypoint stays where it was."); }
      if (state.selection.size) { clearSelection(); renderTracks(); }
      if (state.waypointTool) { state.waypointTool = false; applyEditTools(); }
      if (state.rangeStart != null) setRangeStart(null); // first Esc forgets a half-picked range
      else setRangeTool(false);
      if (state.fill.start != null || state.fill.preview) setFill({ start: null, preview: null, error: null });
      else setFillTool(false);
    }
    // Undo the latest removal by hand, unless typing in a field.
    const typing = event.target.closest?.("input, select, textarea");
    const plain = !event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey;
    // Folding the panels away is the page's own key, wherever it is: it works
    // in Browse, Edit and Route.
    if (!typing && plain) {
      const key = event.key.toLowerCase();
      const panel = { w: "sidebar", e: "profile", i: "inspector" }[key];
      if (panel) {
        event.preventDefault();
        hidePanel(panel, !state.hidden[panel]);
        return;
      }
      if (key === "m") {
        event.preventDefault();
        setMapOnly(!mapOnly());
        return;
      }
      if (key === "d") {
        event.preventDefault();
        terrainToggle();
        return;
      }
    }
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
  if (saved.timelineCompress === true) state.timelineCompress = true;
  $("timeline-compress").checked = state.timelineCompress;
  timeline.setCompress(state.timelineCompress);
  $("timeline-compress").addEventListener("change", () => {
    state.timelineCompress = $("timeline-compress").checked;
    timeline.setCompress(state.timelineCompress);
    save();
  });
  if (saved.timelineHideEmpty === false) state.timelineHideEmpty = false;
  $("timeline-hide-empty").checked = state.timelineHideEmpty;
  timeline.setHideEmpty(state.timelineHideEmpty);
  $("timeline-hide-empty").addEventListener("change", () => {
    state.timelineHideEmpty = $("timeline-hide-empty").checked;
    timeline.setHideEmpty(state.timelineHideEmpty);
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
  $("new-gpx").addEventListener("click", newGPX);
  $("save-all").addEventListener("click", () => saveFiles());
  $("fit-all").addEventListener("click", () => fit(visibleEntries().map((entry) => entry.track)));
  $("track-filter").addEventListener("input", renderTracks);
  // Showing and hiding every listed track is one state, so one eye carries
  // it: while any is shown it hides them, and once none is it shows them.
  // Reloading the page throws away what only the page holds — the tools open,
  // the route being planned, the timeline's view — and starts the workspace
  // again from what was saved. The browser asks first, so a reflex Cmd-R does
  // not leave what is on screen and what is on disk saying different things.
  window.addEventListener("beforeunload", (event) => {
    if (state.restoring || state.tracks.size === 0) return;
    event.preventDefault();
    event.returnValue = ""; // older browsers need this to ask at all
  });
  $("toggle-all").addEventListener("click", () => setAllVisible(!filteredPaths().some((path) => state.tracks.get(path).visible)));
  // The profile appearing or disappearing changes the map's height.
  new ResizeObserver(() => map.resize()).observe($("map"));

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
  applyStatus();
  state.restoring = false;
  save();
}

start().catch((error) => showMessage(error.message, true));
