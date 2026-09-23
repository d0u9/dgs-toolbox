// The map stack: every base map the server lists — the built-in ones, then the
// configured ones — and the contour lines, as one ordered list the user
// reorders by dragging. Any number may be shown at once, each with its own
// opacity, so a transparent overlay (GPS traces, railways, contours) can sit
// above an opaque map. All of it is drawn under the tracks.

const PREFIX = "map-";
import { SortGap, makeGhost } from "./drag.js";

export const CONTOURS = "Contours";

// Adjustments a raster layer takes, each from -1 to 1 with 0 unchanged.
const ADJUST = [
  { key: "saturation", label: "Saturation", title: "Down to grey: coloured lines above stand out" },
  { key: "contrast", label: "Contrast", title: "Down to flatten a busy map, up to sharpen thin lines" },
  { key: "brightness", label: "Brightness", title: "Lighten or darken the whole layer" },
];
const NEUTRAL = { saturation: 0, contrast: 0, brightness: 0 };
const clamp = (value, low, high) => Math.min(high, Math.max(low, value));

// rasterPaint turns a layer's opacity and adjustments into MapLibre paint.
// Brightness raises the darkest tone or lowers the lightest one.
function rasterPaint(item) {
  const { saturation, contrast, brightness } = item.adjust;
  return {
    "raster-opacity": item.opacity,
    "raster-saturation": saturation,
    "raster-contrast": contrast,
    "raster-brightness-min": Math.max(0, brightness),
    "raster-brightness-max": 1 + Math.min(0, brightness),
  };
}

// A vector style is a whole style document — sources, layers, glyphs and
// sprite — not a tile template. Its sources and layers are put into the map
// under the tracks, under this map's own prefix, so the page keeps its style
// and the tracks keep their place above. Its glyphs and sprite are the map's:
// showing two vector styles at once leaves the topmost one's labels.
const styles = new Map(); // url -> the style document, once fetched

async function loadStyle(url) {
  if (!styles.has(url)) {
    const response = await fetch(url);
    if (!response.ok) throw new Error(`${url}: ${response.status}`);
    styles.set(url, await response.json());
  }
  return styles.get(url);
}

function source(urls, tile) {
  return { type: "raster", tiles: urls, tileSize: 256, maxzoom: tile.maxZoom, attribution: tile.attribution || "" };
}

export class MapStack {
  // tiles are the server's maps; contours draws the contour lines; onChange
  // runs after any change the user makes.
  constructor(map, { tiles, contours, onChange }) {
    this.map = map;
    this.contours = contours;
    this.onChange = onChange;
    this.coordinates = "wgs84"; // the system tracks are drawn in
    // Top of the list is drawn on top.
    this.items = [
      { name: CONTOURS, coordinates: "wgs84", visible: false, opacity: 1 },
      // Raster maps take the raster adjustments; a vector style does not,
      // so its row carries no ▸.
      ...tiles.map((tile, i) => ({
        name: tile.name, tile, id: PREFIX + i, coordinates: tile.coordinates,
        visible: i === 0, opacity: 1, ...(tile.style ? {} : { adjust: { ...NEUTRAL } }),
      })),
    ];
    this.expanded = new Set(); // names of rows showing their adjustments
    this.drawnStyles = new Map(); // name -> { sources, layers } of a vector style drawn now
    this.styleBases = new Map(); // "layer/property" -> the opacity the style wrote
  }

  // restore applies a saved stack: [{name, visible, opacity}], top first.
  // Maps are matched by name, so reordering the configuration keeps it; maps
  // new to the configuration keep their place below the saved ones.
  restore(saved) {
    if (!Array.isArray(saved)) return;
    const byName = new Map(this.items.map((item) => [item.name, item]));
    const restored = [];
    for (const entry of saved) {
      const item = byName.get(entry?.name);
      if (!item) continue;
      byName.delete(entry.name);
      item.visible = entry.visible === true;
      if (Number.isFinite(entry.opacity)) item.opacity = clamp(entry.opacity, 0, 1);
      if (item.adjust) {
        for (const { key } of ADJUST) {
          if (Number.isFinite(entry.adjust?.[key])) item.adjust[key] = clamp(entry.adjust[key], -1, 1);
        }
      }
      restored.push(item);
    }
    this.items = [...restored, ...byName.values()];
    // A saved stack can name a map the configuration no longer offers — one
    // taken out of the built-in list, or removed from the tiles file. Its row
    // is simply gone; but if it was the only map shown, nothing would be
    // drawn and every row would read as off, so the first map takes over.
    if (!this.items.some((item) => item.visible && item.tile)) {
      const first = this.items.find((item) => item.tile);
      if (first) first.visible = true;
    }
  }

  toJSON() {
    return this.items.map(({ name, visible, opacity, adjust }) => ({ name, visible, opacity, ...(adjust && { adjust }) }));
  }

  // base is the lowest shown map: GCJ-02's Auto follows it.
  base() {
    return [...this.items].reverse().find((item) => item.visible && item.tile);
  }

  // offset says whether an item is drawn in another system than the tracks,
  // and so sits several hundred metres off them in China.
  offset(item) {
    return item.coordinates !== this.coordinates;
  }

  // apply brings the map in line with the list.
  apply() {
    const firstOther = this.map.getStyle().layers.find((layer) => !this.owns(layer.id))?.id;
    // Bottom to top, each moved just under the tracks, stacks them in order.
    for (const item of [...this.items].reverse()) {
      if (item.name === CONTOURS) {
        this.contours.show(item.visible, item.opacity);
        for (const id of this.contours.layerIds()) this.map.moveLayer(id, firstOther);
        continue;
      }
      if (item.tile.style) {
        this.applyStyle(item, firstOther);
        continue;
      }
      const ids = [item.id, `${item.id}-overlay`];
      if (!item.visible) {
        for (const id of ids) {
          if (this.map.getLayer(id)) this.map.removeLayer(id);
          if (this.map.getSource(id)) this.map.removeSource(id);
        }
        continue;
      }
      if (!this.map.getSource(item.id)) {
        this.map.addSource(item.id, source(item.tile.urls, item.tile));
        this.map.addLayer({ id: item.id, type: "raster", source: item.id }, firstOther);
        // The overlay repeats no attribution: the base already credits the provider.
        if (item.tile.overlay?.length) {
          this.map.addSource(ids[1], { ...source(item.tile.overlay, item.tile), attribution: "" });
          this.map.addLayer({ id: ids[1], type: "raster", source: ids[1] }, firstOther);
        }
      }
      for (const id of ids) {
        if (!this.map.getLayer(id)) continue;
        for (const [property, value] of Object.entries(rasterPaint(item))) this.map.setPaintProperty(id, property, value);
        this.map.moveLayer(id, firstOther);
      }
    }
  }

  // applyStyle draws a vector style's own layers under the tracks, or takes
  // them out again. The style is fetched once and kept, so switching back to
  // it costs nothing.
  applyStyle(item, under) {
    const drawn = this.drawnStyles.get(item.name);
    if (!item.visible) {
      if (!drawn) return;
      for (const id of drawn.layers) {
        if (this.map.getLayer(id)) this.map.removeLayer(id);
      }
      for (const id of drawn.sources) {
        if (this.map.getSource(id)) this.map.removeSource(id);
      }
      this.drawnStyles.delete(item.name);
      return;
    }
    if (drawn) {
      // Already drawn: keep it under the tracks and at the opacity asked for.
      for (const id of drawn.layers) {
        if (this.map.getLayer(id)) this.map.moveLayer(id, under);
      }
      this.styleOpacity(item, drawn.layers);
      return;
    }
    // The fetch settles later; the stack is applied again when it does, so
    // the style lands wherever the list has put it by then.
    loadStyle(item.tile.style).then((style) => {
      if (!item.visible || this.drawnStyles.has(item.name)) return;
      const prefix = `${item.id}-`;
      const sources = [];
      for (const [name, definition] of Object.entries(style.sources || {})) {
        const id = prefix + name;
        if (!this.map.getSource(id)) this.map.addSource(id, definition);
        sources.push(id);
      }
      if (style.glyphs) this.map.setGlyphs(style.glyphs);
      if (style.sprite && typeof style.sprite === "string") this.map.setSprite(style.sprite);
      const layers = [];
      const firstOther = this.map.getStyle().layers.find((layer) => !this.owns(layer.id))?.id;
      for (const layer of style.layers || []) {
        // The background layer comes too: it is the style's own paper, and
        // covering what is under it is what an opaque map does.
        const drawnLayer = { ...layer, id: prefix + layer.id };
        if (layer.source) drawnLayer.source = prefix + layer.source;
        this.map.addLayer(drawnLayer, firstOther);
        layers.push(drawnLayer.id);
      }
      this.drawnStyles.set(item.name, { sources, layers });
      this.styleOpacity(item, layers);
    }).catch(() => {
      // A style that cannot be fetched draws nothing; the row stays, so it
      // can be tried again by hiding and showing it.
    });
  }

  // styleOpacity fades a vector style as a whole. Only a layer whose opacity
  // is a plain number is faded; one carrying an expression is left as its
  // author wrote it.
  styleOpacity(item, layers) {
    const properties = {
      background: ["background-opacity"], fill: ["fill-opacity"], line: ["line-opacity"],
      symbol: ["text-opacity", "icon-opacity"],
      circle: ["circle-opacity", "circle-stroke-opacity"], "fill-extrusion": ["fill-extrusion-opacity"],
      raster: ["raster-opacity"], hillshade: ["hillshade-exaggeration"],
    };
    for (const id of layers) {
      const layer = this.map.getLayer(id);
      if (!layer) continue;
      for (const property of properties[layer.type] || []) {
        const base = this.styleBase(id, property);
        if (base == null) continue;
        this.map.setPaintProperty(id, property, base * item.opacity);
      }
    }
  }

  // styleBase is a layer's opacity as the style wrote it, remembered the
  // first time it is faded so fading twice does not compound.
  styleBase(id, property) {
    const key = `${id}/${property}`;
    if (!this.styleBases.has(key)) {
      let value = 1;
      try {
        const written = this.map.getPaintProperty(id, property);
        if (typeof written === "number") value = written;
        else if (written !== undefined) value = null; // an expression: left alone
      } catch {
        value = null;
      }
      this.styleBases.set(key, value);
    }
    return this.styleBases.get(key);
  }

  owns(id) {
    return id.startsWith(PREFIX) || this.contours.layerIds().includes(id);
  }

  // ---- the panel ----

  // render draws the list into root: a drag handle, a checkbox, the name and
  // an opacity slider per row.
  render(root) {
    this.root = root;
    root.replaceChildren(...this.items.map((item, index) => this.row(item, index)));
  }

  row(item, index) {
    const row = document.createElement("li");
    row.className = "layer-row";
    row.classList.toggle("off", !item.visible);

    const handle = document.createElement("span");
    handle.className = "layer-handle";
    handle.textContent = "⋮⋮";
    handle.title = "Drag to reorder: the top of the list is drawn on top";

    const label = document.createElement("label");
    label.className = "layer-name";
    const check = document.createElement("input");
    check.type = "checkbox";
    check.checked = item.visible;
    check.addEventListener("change", () => this.change(() => (item.visible = check.checked)));
    const name = document.createElement("span");
    name.textContent = item.name;
    label.append(check, name);
    if (item.coordinates === "gcj02") {
      const tag = document.createElement("small");
      tag.className = "layer-tag";
      tag.textContent = "GCJ-02";
      label.append(tag);
    }
    if (item.visible && this.offset(item)) {
      const warn = document.createElement("span");
      warn.className = "layer-warn";
      warn.textContent = "⚠";
      warn.title = `Drawn in ${item.coordinates === "gcj02" ? "GCJ-02" : "WGS-84"} while the tracks are ${this.coordinates === "gcj02" ? "GCJ-02" : "WGS-84"}: in China it sits several hundred metres off them`;
      label.append(warn);
    }

    const slider = document.createElement("input");
    slider.type = "range";
    slider.className = "layer-opacity";
    slider.min = "0";
    slider.max = "100";
    slider.step = "5";
    slider.value = String(Math.round(item.opacity * 100));
    slider.title = `Opacity ${slider.value}%`;
    slider.addEventListener("input", () => {
      item.opacity = Number(slider.value) / 100;
      slider.title = `Opacity ${slider.value}%`;
      this.apply();
    });
    slider.addEventListener("change", () => this.onChange());

    handle.addEventListener("pointerdown", (event) => this.drag(event, index));

    const main = document.createElement("div");
    main.className = "layer-main";
    main.append(handle, label, slider);
    row.append(main);
    if (!item.adjust) {
      main.append(Object.assign(document.createElement("span"), { className: "layer-expand-space" }));
      return row;
    }
    const open = this.expanded.has(item.name);
    const adjusted = ADJUST.some(({ key }) => item.adjust[key] !== 0);
    const expand = document.createElement("button");
    expand.className = "layer-expand";
    expand.classList.toggle("adjusted", adjusted);
    expand.textContent = open ? "▾" : "▸";
    expand.title = adjusted ? "Adjustments (changed)" : "Adjustments: saturation, contrast, brightness";
    expand.setAttribute("aria-expanded", String(open));
    expand.addEventListener("click", () => {
      if (open) this.expanded.delete(item.name);
      else this.expanded.add(item.name);
      this.render(this.root);
    });
    main.append(expand);
    if (open) row.append(this.adjustments(item));
    return row;
  }

  // adjustments are a row's saturation, contrast and brightness sliders.
  adjustments(item) {
    const box = document.createElement("div");
    box.className = "layer-adjust";
    for (const { key, label, title } of ADJUST) {
      const line = document.createElement("label");
      line.title = title;
      const name = document.createElement("span");
      name.textContent = label;
      const slider = document.createElement("input");
      slider.type = "range";
      slider.min = "-100";
      slider.max = "100";
      slider.step = "5";
      slider.value = String(Math.round(item.adjust[key] * 100));
      const value = document.createElement("output");
      const show = () => (value.textContent = `${slider.value > 0 ? "+" : ""}${slider.value}`);
      show();
      slider.addEventListener("input", () => {
        item.adjust[key] = Number(slider.value) / 100;
        show();
        this.apply();
      });
      slider.addEventListener("change", () => this.change(() => {}));
      slider.addEventListener("dblclick", () => this.change(() => (item.adjust[key] = 0)));
      line.append(name, slider, value);
      box.append(line);
    }
    const reset = document.createElement("button");
    reset.className = "text-button";
    reset.textContent = "Reset";
    reset.title = "Back to the layer as published";
    reset.addEventListener("click", () => this.change(() => (item.adjust = { ...NEUTRAL })));
    box.append(reset);
    return box;
  }

  // drag moves a row by its handle: the rows it passes slide aside to open
  // the gap it would land in, as in the workspace.
  drag(event, from) {
    event.preventDefault();
    const handle = event.currentTarget;
    handle.setPointerCapture(event.pointerId);
    const rows = [...this.root.children];
    const gap = new SortGap(rows, [rows[from]]);
    const ghost = makeGhost(rows[from], event.clientX, event.clientY);
    let to = from;
    const move = (e) => {
      ghost.follow(e.clientX, e.clientY);
      to = gap.slot(e.clientY);
      gap.open(to);
    };
    const end = () => {
      handle.removeEventListener("pointermove", move);
      handle.removeEventListener("pointerup", end);
      handle.removeEventListener("pointercancel", end);
      ghost.remove();
      gap.close();
      const target = to > from ? to - 1 : to;
      if (target === from) return this.render(this.root);
      this.change(() => {
        const [moved] = this.items.splice(from, 1);
        this.items.splice(target, 0, moved);
      });
    };
    handle.addEventListener("pointermove", move);
    handle.addEventListener("pointerup", end);
    handle.addEventListener("pointercancel", end);
  }

  change(mutate) {
    mutate();
    this.apply();
    this.render(this.root);
    this.onChange();
  }
}
