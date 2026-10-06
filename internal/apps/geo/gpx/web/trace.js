// A fine colour core between light and dark borders stays legible on both
// light cartography and dark imagery. State changes weight, never visibility.
export const TRACE_WIDTHS = Object.freeze({
  inactive: [3.8, 2.8, 1.6],
  focused: [4.2, 3.2, 2],
  segment: [4, 3, 1.8],
  selected: [4.8, 3.8, 2.6],
});
export const traceLayers = id => [`${id}-edge`, `${id}-casing`, id];
export function addTrace(map, { id, source, color, state = "inactive", filter, metadata, dashed = false, before }) {
  const widths = TRACE_WIDTHS[state], colors = ["#17212b", "#ffffff", color];
  traceLayers(id).forEach((layer, i) => {
    const paint = { "line-color": colors[i], "line-width": widths[i], "line-opacity": 1 };
    // Dash lengths are pixels, shared by all three layers, rather than each
    // layer's own width. Otherwise the borders fill the core's dash gaps.
    if (dashed) paint["line-dasharray"] = [7 / widths[i], 4 / widths[i]];
    map.addLayer({ id: layer, type: "line", source, ...(filter ? { filter } : {}), ...(i === 2 && metadata ? { metadata } : {}), layout: { "line-join": "round", "line-cap": "round" }, paint }, before);
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
