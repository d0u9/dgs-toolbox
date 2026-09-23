// Thin wrappers over the server API. Every track value is computed by the
// server; the page only draws what it receives.

async function getJSON(url) {
  const response = await fetch(url);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `${response.status} ${response.statusText}`);
  return body;
}

async function postJSON(url, body) {
  return sendJSON("POST", url, body);
}

async function sendJSON(method, url, body) {
  const response = await fetch(url, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  const result = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(result.error || `${response.status} ${response.statusText}`);
  return result;
}

export const api = {
  config: () => getJSON("api/config"),
  // stops is { distance: metres, duration: seconds }, the stay-point thresholds.
  // coordinates is "wgs84" or "gcj02", the system the positions are drawn in.
  track: (path, stops, coordinates = "wgs84") =>
    getJSON(`api/track?path=${encodeURIComponent(path)}&coordinates=${coordinates}` + stopQuery(stops)),
  // saveClean records a track's cleaning settings in the sidecar beside it.
  saveClean: (path, clean) => sendJSON("PUT", "api/clean", { path, clean }),
  // renamePart names one <trk>, <rte> or <wpt> of a file, keyed "t0", "r0", "w0".
  renamePart: (path, key, name) => sendJSON("PUT", "api/part-name", { path, key, name }),
  addWaypoint: (path, name, description, lon, lat) => postJSON("api/waypoint", { path, name, description, lon, lat }),
  // saveSegments records a track's cuts and segment names in its sidecar.
  saveSegments: (path, cuts, names) => sendJSON("PUT", "api/segments", { path, cuts, names }),
  // writeSegments writes chosen segments into a new or another GPX.
  writeSegments: (path, request) => sendJSON("POST", "api/segments/write", { path, ...request }),
  // routeFill asks the router for the road between two points of a track,
  // ends { first, last }; profile is "car", "bike" or "foot". Only the two
  // positions leave the server.
  routeFill: (path, ends, profile, coordinates) => postJSON("api/fill/route", { path, ...ends, profile, coordinates }),
  // saveFill records a previewed route between the two points.
  saveFill: (path, first, last, profile, route) => postJSON("api/fill", { path, first, last, profile, route }),
  removeFill: (path, index) => sendJSON("DELETE", "api/fill", { path, index }),
  // orderParts puts a file's tracks, routes or waypoints in another order,
  // keys in the order wanted; kind is "track", "route" or "waypoint".
  orderParts: (path, kind, keys) => sendJSON("PUT", "api/part-order", { path, kind, keys }),
  // deletePart takes one <trk>, <rte> or <wpt> out of a file: out of the GPX
  // itself when dgs wrote it, and out of what is held beside it otherwise.
  deletePart: (path, key) => sendJSON("DELETE", "api/part", { path, key }),
  // copyPart copies one part into another GPX dgs wrote, and takes it out of
  // this one when move is true.
  copyPart: (path, key, target, move = false) => postJSON("api/part/copy", { path, key, target, move }),
  // moveWaypoint puts a waypoint somewhere else, [lon, lat] in WGS-84.
  moveWaypoint: (path, key, lon, lat) => sendJSON("PUT", "api/waypoint", { path, key, lon, lat }),
  // saveAs writes a GPX with the tracks added to it into a new file.
  saveAs: (path, target) => postJSON("api/save-as", { path, target }),
  // saveFiles writes files' unsaved edits where they belong: into the sidecar
  // beside a recording, or into the GPX itself when dgs wrote it. An empty
  // list saves every file holding unsaved edits.
  saveFiles: (paths = []) => postJSON("api/save", { paths }),
  // revertFiles drops unsaved edits, so the files read as they are on disk
  // again. An empty list reverts every file holding unsaved edits.
  revertFiles: (paths = []) => sendJSON("DELETE", "api/pending", { paths }),
  // unsaved lists the files holding edits that are not on disk yet.
  unsaved: () => getJSON("api/unsaved"),
  // discardSidecar deletes a GPX's sidecar, dropping everything done to it.
  discardSidecar: (path) => sendJSON("DELETE", "api/sidecar", { path }),
  // newDraft starts a new, empty GPX that lives in the server's memory until saved.
  newDraft: (name) => postJSON("api/draft", { name }),
  // routeLeg asks the router for the road between two waypoints, [lon, lat] in WGS-84.
  routeLeg: (profile, from, to) => postJSON("api/route/leg", { profile, from, to }),
  // saveRoute writes a planned route into a new GPX, keeping the plan beside it.
  saveRoute: (request) => postJSON("api/route/save", request),
  focus: (path, stops) => postJSON("api/focus", { path: path || "", stopDistance: stops?.distance, stopDuration: stops?.duration }),
  reveal: (path) => postJSON("api/reveal", { path }),
};

function stopQuery(stops) {
  if (!stops) return "";
  return `&stopDistance=${encodeURIComponent(stops.distance)}&stopDuration=${encodeURIComponent(stops.duration)}`;
}
