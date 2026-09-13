// The map stack: every base map the server lists — the built-in ones, then the
// configured ones — and the contour lines, as one ordered list the user
// reorders by dragging. Any number may be shown at once, each with its own
// opacity, so a transparent overlay (GPS traces, railways, contours) can sit
// above an opaque map. All of it is drawn under the tracks.

const PREFIX = "map-";
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
      ...tiles.map((tile, i) => ({ name: tile.name, tile, id: PREFIX + i, coordinates: tile.coordinates, visible: i === 0, opacity: 1, adjust: { ...NEUTRAL } })),
    ];
    this.expanded = new Set(); // names of rows showing their adjustments
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

  // drag moves a row by its handle: a line shows where it will land.
  drag(event, from) {
    event.preventDefault();
    const handle = event.currentTarget;
    handle.setPointerCapture(event.pointerId);
    const rows = [...this.root.children];
    rows[from].classList.add("dragging");
    let to = from;
    const mark = () => {
      rows.forEach((row) => row.classList.remove("drop-above", "drop-below"));
      if (to === from || to === from + 1) return;
      if (to < rows.length) rows[to].classList.add("drop-above");
      else rows[rows.length - 1].classList.add("drop-below");
    };
    const move = (e) => {
      to = rows.findIndex((row) => {
        const box = row.getBoundingClientRect();
        return e.clientY < box.top + box.height / 2;
      });
      if (to < 0) to = rows.length;
      mark();
    };
    const end = () => {
      handle.removeEventListener("pointermove", move);
      handle.removeEventListener("pointerup", end);
      handle.removeEventListener("pointercancel", end);
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
