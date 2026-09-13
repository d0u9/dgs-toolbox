// Thin wrappers over the server API. Every track value is computed by the
// server; the page only draws what it receives.

async function getJSON(url) {
  const response = await fetch(url);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `${response.status} ${response.statusText}`);
  return body;
}

async function postJSON(url, body) {
  const response = await fetch(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
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
  focus: (path, stops) => postJSON("api/focus", { path: path || "", stopDistance: stops?.distance, stopDuration: stops?.duration }),
  reveal: (path) => postJSON("api/reveal", { path }),
};

function stopQuery(stops) {
  if (!stops) return "";
  return `&stopDistance=${encodeURIComponent(stops.distance)}&stopDuration=${encodeURIComponent(stops.duration)}`;
}
