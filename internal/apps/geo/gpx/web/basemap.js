// Raster base maps. The server lists them: the built-in ones first, then the
// ones configured in geo.gpx.tiles. A map may carry an overlay drawn above its
// tiles, as road names above satellite imagery.

const BASE = "base";
const OVERLAY = "base-overlay";

function source(urls, tile) {
  return { type: "raster", tiles: urls, tileSize: 256, maxzoom: tile.maxZoom, attribution: tile.attribution || "" };
}

function layers(tile) {
  const list = [{ id: BASE, type: "raster", source: BASE }];
  if (tile.overlay?.length) list.push({ id: OVERLAY, type: "raster", source: OVERLAY });
  return list;
}

function sources(tile) {
  const all = { [BASE]: source(tile.urls, tile) };
  // The overlay repeats no attribution: the base already credits the provider.
  if (tile.overlay?.length) all[OVERLAY] = { ...source(tile.overlay, tile), attribution: "" };
  return all;
}

export function initialStyle(tile) {
  return { version: 8, sources: sources(tile), layers: layers(tile) };
}

// setBaseMap swaps the base tiles under every other layer, keeping the tracks.
export function setBaseMap(map, tile) {
  const style = map.getStyle().layers;
  const firstOther = style.find((layer) => layer.id !== BASE && layer.id !== OVERLAY);
  for (const id of [OVERLAY, BASE]) {
    if (map.getLayer(id)) map.removeLayer(id);
    if (map.getSource(id)) map.removeSource(id);
  }
  for (const [id, spec] of Object.entries(sources(tile))) map.addSource(id, spec);
  for (const layer of layers(tile)) map.addLayer(layer, firstOther?.id);
}
