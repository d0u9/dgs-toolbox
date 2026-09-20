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
  dir: (path) => getJSON("api/dir" + (path ? `?path=${encodeURIComponent(path)}` : "")),
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
  // removeAdded takes a track added from another file out again.
  removeAdded: (path, index) => sendJSON("DELETE", "api/added", { path, index }),
  // saveAs writes a GPX with the tracks added to it into a new file.
  saveAs: (path, target) => postJSON("api/save-as", { path, target }),
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
