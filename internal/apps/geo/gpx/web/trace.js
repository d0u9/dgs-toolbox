// A broad colour core has one narrow inverse-colour outline.
// State changes weight, never visibility.
export const TRACE_WIDTHS = Object.freeze({
  inactive: [4.8, 3.6],
  focused: [5.2, 4],
  segment: [5, 3.8],
  selected: [6, 4.8],
});
export const traceLayers = id => [`${id}-edge`, id];
export function addTrace(map, { id, source, color, state = "inactive", filter, metadata, dashed = false, before }) {
  const widths = TRACE_WIDTHS[state], colors = [inverseColor(color), color];
  traceLayers(id).forEach((layer, i) => {
    const paint = { "line-color": colors[i], "line-width": widths[i], "line-opacity": 1 };
    // Dash lengths are pixels, shared by all three layers, rather than each
    // layer's own width. Otherwise the borders fill the core's dash gaps.
    if (dashed) paint["line-dasharray"] = [7 / widths[i], 4 / widths[i]];
    map.addLayer({ id: layer, type: "line", source, ...(filter ? { filter } : {}), ...(i === 1 && metadata ? { metadata } : {}), layout: { "line-join": "round", "line-cap": "round" }, paint }, before);
  });
}
export function setTraceState(map, id, state, { dashed = false } = {}) {
  traceLayers(id).forEach((layer, i) => {
    const width = TRACE_WIDTHS[state][i];
    map.setPaintProperty(layer, "line-width", width);
    map.setPaintProperty(layer, "line-opacity", 1);
    if (dashed) map.setPaintProperty(layer, "line-dasharray", [7 / width, 4 / width]);
  });
}

export function inverseColor(color) {
  if (typeof color === "string" && /^#[0-9a-f]{6}$/i.test(color)) {
    return "#" + (0xffffff ^ parseInt(color.slice(1), 16)).toString(16).padStart(6, "0");
  }
  // Segment colours come from each feature, so their outlines do too.
  return ["rgb", ...[0, 1, 2].map(i => ["-", 255, ["at", i, ["to-rgba", color]]])];
}
export function setTraceColor(map, id, color) {
  map.setPaintProperty(`${id}-edge`, "line-color", inverseColor(color));
  map.setPaintProperty(id, "line-color", color);
}
