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
export function setTraceState(map, id, state, { dashed = false, points = false } = {}) {
  traceLayers(id).forEach((layer, i) => {
    const width = points ? [2, 1][i] : TRACE_WIDTHS[state][i];
    map.setPaintProperty(layer, "line-width", width);
    map.setPaintProperty(layer, "line-opacity", 1);
    map.setPaintProperty(layer, "line-dasharray", points ? [3 / width, 3 / width] : dashed ? [7 / width, 4 / width] : null);
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

// Opposite lightness stays legible where complementary hues do not.
export function directionColor(color) {
  const weights = [0.299, 0.587, 0.114];
  if (typeof color === "string" && /^#[0-9a-f]{6}$/i.test(color)) {
    const rgb = [1, 3, 5].map(i => parseInt(color.slice(i, i + 2), 16));
    return rgb.reduce((sum, value, i) => sum + value * weights[i], 0) >= 128 ? "#1a1a1a" : "#ffffff";
  }
  return ["case", [">=", ["+", ...weights.map((weight, i) => ["*", weight, ["at", i, ["to-rgba", color]]])], 128], "#1a1a1a", "#ffffff"];
}

// Chevrons follow geometry order and sit inside the visible colour core.
export function addDirection(map, { id, source, color, filter, before, width = TRACE_WIDTHS.inactive[1] }) {
  map.addLayer({ id, type: "symbol", source, ...(filter ? { filter } : {}),
    layout: {
      "symbol-placement": "line", "symbol-spacing": 24,
      "icon-image": "track-direction", "icon-rotation-alignment": "map",
      // The thick chevron spans 12 px including its stroke.
      "icon-keep-upright": false, "icon-size": width / 12,
      "icon-allow-overlap": true, "icon-ignore-placement": true,
    },
    paint: { "icon-color": directionColor(color) },
  }, before);
}
