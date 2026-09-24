// The page: fetch a graph, draw it, and let it be dragged. Everything about
// what is being drawn comes from the API — this file knows about groups,
// nodes, edges and kinds, and nothing about what any of them mean.

// theme is what the drawing paints with, since a canvas cannot read a CSS
// variable. The values match the shared tokens of /ui/tokens.css.
function theme() {
  return { groupFill: "#000000", groupFillOpacity: 0.035, groupLine: "#c2c2c2", groupText: "#636363", edgeText: "#636363", edgeTextBg: "#f7f7f7", selected: "#1a1a1a" };
}

// The Okabe-Ito qualitative palette, which stays distinguishable under
// deuteranopia, protanopia and tritanopia. Ordered so the first kinds a
// caller declares get the pairs that are furthest apart, and so that no two
// neighbours differ in hue alone.
const palette = [
  "#0072B2", // blue
  "#D55E00", // vermillion
  "#009E73", // bluish green
  "#CC79A7", // reddish purple
  "#E69F00", // orange
  "#56B4E9", // sky blue
  "#3A3A3A", // near-black
  "#F0E442", // yellow
];

function paletteAt(i) {
  return palette[i % palette.length];
}

// Colour is never the only difference. A kind also gets a line style and a
// shape from its position in the palette, so the picture still reads when
// the hues do not — printed in grey, or by a reader who cannot tell two of
// them apart. Both lists are shorter than the palette on purpose: they cycle
// against it, so kind 0 and kind 6 differ in shape even though they are the
// closest pair of colours.
const lineStyles = ["solid", "dashed", "dotted"];
// Every shape here is one a line of text fits inside: a diamond or a plain
// hexagon wastes its corners and pushes the label over its own outline, so
// the silhouettes differ along the sides instead.
const nodeShapes = ["round-rectangle", "round-hexagon", "barrel", "cut-rectangle"];

// Shapes that lose width towards their ends need more room around the label
// than a rectangle does, or the text runs into the slanted edge.
const shapePadding = { "round-rectangle": "12px", "round-hexagon": "20px", "barrel": "16px", "cut-rectangle": "16px" };

// indexFor keeps one slot per kind, assigned in the order kinds are first
// seen, so a graph redrawn after a reload keeps its colours.
const kindIndex = new Map();
function indexFor(kind) {
  if (!kindIndex.has(kind)) {
    kindIndex.set(kind, kindIndex.size);
  }
  return kindIndex.get(kind);
}

function colourFor(kind) {
  if (!kind) return "#636363";
  return paletteAt(indexFor(kind));
}

// textOn is black or white, whichever the swatch behind it carries: the
// palette runs from a near-black to a yellow, and one fixed label colour
// would be unreadable on one end of it.
function textOn(hex) {
  const n = parseInt(hex.slice(1), 16);
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((c) => {
    const v = c / 255;
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
  });
  const luminance = 0.2126 * r + 0.7152 * g + 0.0722 * b;
  return luminance > 0.45 ? "#1a1a1a" : "#ffffff";
}

// A line is a thin thing, and the palette's paler hues — the yellow above
// all — are chosen to work as a filled swatch rather than as one. So a line
// takes a darker version of its kind's colour.
function lineColourFor(kind) {
  const hex = colourFor(kind);
  const n = parseInt(hex.slice(1), 16);
  const darker = [(n >> 16) & 255, (n >> 8) & 255, n & 255]
    .map((c) => Math.round(c * 0.72).toString(16).padStart(2, "0"))
    .join("");
  return `#${darker}`;
}

function lineStyleFor(kind) {
  if (!kind) return "solid";
  return lineStyles[indexFor(kind) % lineStyles.length];
}

function shapeFor(kind) {
  if (!kind) return "round-rectangle";
  return nodeShapes[indexFor(kind) % nodeShapes.length];
}

function showError(message) {
  const box = document.getElementById("error");
  box.textContent = message;
  box.hidden = !message;
}

function elements(graph) {
  const out = [];
  for (const g of graph.groups || []) {
    out.push({
      data: {
        id: g.id,
        parent: g.parent || undefined,
        label: g.label,
        detail: g.detail || "",
        kind: g.kind || "",
        colour: colourFor(g.kind),
        isGroup: true,
      },
      classes: g.collapse ? "collapsible" : "",
    });
  }
  for (const n of graph.nodes || []) {
    out.push({
      data: {
        id: n.id,
        parent: n.group || undefined,
        label: n.label,
        detail: n.detail || "",
        tooltip: n.tooltip || "",
        kind: n.kind || "",
        colour: colourFor(n.kind),
        textColour: textOn(colourFor(n.kind)),
        shape: shapeFor(n.kind),
        pad: shapePadding[shapeFor(n.kind)] || "12px",
      },
    });
  }
  const standsFor = collapseMap(graph);

  // One stand-in shape per collapsible box. A compound box is not drawn at
  // all once everything inside it is hidden, so the shut box is a shape of
  // its own rather than the box itself: it carries the box's name, the count
  // of what it holds, and the lines that stand for the lines inside.
  const holds = new Map();
  for (const [id, box] of standsFor) if (!id.startsWith("stand:")) holds.set(box, (holds.get(box) || 0) + 1);
  for (const g of graph.groups || []) {
    if (!g.collapse) continue;
    const inside = (holds.get(g.id) || 0) - 1;
    out.push({
      data: {
        id: standID(g.id),
        // The stand-in sits in whatever box the collapsible one sat in, so
        // the overview still says whose machines these are.
        parent: g.parent || undefined,
        label: g.label,
        detail: g.detail || (inside > 0 ? `${inside} inside` : ""),
        tooltip: g.detail || "",
        kind: g.kind || "",
        colour: colourFor(g.kind),
        textColour: textOn(colourFor(g.kind)),
        shape: "round-rectangle",
        pad: "14px",
        opens: g.id,
      },
      classes: "stand",
    });
  }

  const meta = new Map();
  for (const e of graph.edges || []) {
    const from = standsFor.get(e.from) || "";
    const to = standsFor.get(e.to) || "";
    out.push({
      data: {
        id: `${e.from}->${e.to}:${e.label || ""}`,
        source: e.from,
        target: e.to,
        label: e.label || "",
        kind: e.kind || "",
        colour: lineColourFor(e.kind),
        lineStyle: lineStyleFor(e.kind),
      },
      // An edge inside one collapsed box has nothing to say once the box is
      // shut; one that crosses between boxes is said by the meta edge below.
      classes: from && from === to ? "within" : from || to ? "crossing" : "",
    });
    if (!from && !to) continue;
    if (from === to) continue;
    const s = from ? standID(from) : e.from;
    const t = to ? standID(to) : e.to;
    const key = `${s}|${t}`;
    const agg = meta.get(key) || { source: s, target: t, count: 0, kinds: new Set(), label: "" };
    agg.count += 1;
    agg.kinds.add(e.kind || "");
    agg.label = e.label || agg.label;
    meta.set(key, agg);
  }
  for (const [key, agg] of meta) {
    const kind = agg.kinds.size === 1 ? [...agg.kinds][0] : "";
    out.push({
      data: {
        id: `meta:${key}`,
        source: agg.source,
        target: agg.target,
        // One line standing for many says how many; one standing for one
        // keeps the word the single line carried.
        label: agg.count > 1 ? `${agg.count}` : agg.label,
        weight: agg.count,
        kind: kind,
        colour: lineColourFor(kind),
        lineStyle: lineStyleFor(kind),
      },
      classes: "meta",
    });
  }
  return out;
}

function standID(id) {
  return `stand:${id}`;
}

// collapseMap answers, for every node and group, which collapsible box
// stands for it when the picture is small — the outermost one it sits in, so
// a collapsible box inside another closes with its container rather than
// staying open inside a shut box. Ids not inside one are absent.
function collapseMap(graph) {
  const parent = new Map();
  const collapses = new Set();
  for (const g of graph.groups || []) {
    if (g.parent) parent.set(g.id, g.parent);
    if (g.collapse) collapses.add(g.id);
  }
  for (const n of graph.nodes || []) if (n.group) parent.set(n.id, n.group);
  const standsFor = new Map();
  const resolve = (id) => {
    let box = "";
    for (let at = id; at; at = parent.get(at)) {
      if (collapses.has(at)) box = at;
    }
    return box;
  };
  for (const g of graph.groups || []) {
    const box = resolve(g.id);
    if (box) standsFor.set(g.id, box);
  }
  for (const n of graph.nodes || []) {
    const box = resolve(n.id);
    if (box) standsFor.set(n.id, box);
  }
  return standsFor;
}

function styleSheet() {
  const t = theme();
  return [
  {
    selector: "node",
    style: {
      "background-color": "data(colour)",
      "label": (el) => {
        const detail = el.data("detail");
        return detail ? `${el.data("label")}\n${detail}` : el.data("label");
      },
      "color": "data(textColour)",
      "text-wrap": "wrap",
      "text-valign": "center",
      "text-halign": "center",
      "font-size": 11,
      "line-height": 1.25,
      "text-max-width": 140,
      "padding": "data(pad)",
      "shape": "data(shape)",
      "corner-radius": 8,
      "width": "label",
      "height": "label",
      // A thin outline in the same hue, so a shape on a pale swatch still
      // has an edge against the page.
      "border-width": 1,
      "border-color": "data(colour)",
    },
  },
  {
    selector: ":parent",
    style: {
      "background-color": t.groupFill,
      "background-opacity": t.groupFillOpacity,
      "border-width": 1,
      "border-color": t.groupLine,
      "border-style": "dashed",
      "label": "data(label)",
      "color": t.groupText,
      "text-valign": "top",
      "text-halign": "center",
      "font-size": 12,
      "font-weight": 600,
      "text-margin-y": -4,
      "padding": "22px",
      "shape": "round-rectangle",
      "corner-radius": 12,
    },
  },
  {
    // A group inside a group: drawn solid and tighter, so the outer box
    // still reads as the container of the inner ones.
    selector: ":parent[parent]",
    style: {
      "border-style": "solid",
      "border-color": "data(colour)",
      "background-opacity": 0,
      "font-size": 10,
      "font-weight": 500,
      "padding": "10px",
      "color": "data(colour)",
    },
  },
  {
    selector: "edge",
    style: {
      "width": 1.6,
      "line-color": "data(colour)",
      "line-style": "data(lineStyle)",
      "target-arrow-color": "data(colour)",
      "target-arrow-shape": "triangle",
      "arrow-scale": 1,
      // Orthogonal routing: every segment runs horizontal or vertical, so a
      // dense picture reads as wiring rather than as a bundle of straight
      // lines crossing at arbitrary angles.
      "curve-style": "taxi",
      "taxi-direction": "auto",
      "taxi-turn": "50%",
      "taxi-turn-min-distance": 12,
      "taxi-radius": 8,
      "label": "data(label)",
      "font-size": 10,
      "color": t.edgeText,
      "text-background-color": t.edgeTextBg,
      "text-background-opacity": 0.9,
      "text-background-shape": "roundrectangle",
      "text-background-padding": 3,
      // Upright, because a taxi edge's segments are horizontal or vertical
      // and a rotated label on a vertical one reads sideways.
      "text-rotation": "none",
    },
  },
  {
    selector: "node:selected",
    style: { "border-width": 3, "border-color": t.selected },
  },
  {
    selector: ".faded",
    style: { "opacity": 0.15 },
  },
  // Level of detail. These come last on purpose: a later rule wins, so what
  // the zoom decides overrides how an element is drawn when it is open.
  {
    // Taken out of the picture rather than made invisible: a shut box is
    // meant to be the size of its own name, and an element that still takes
    // up space would keep the box as wide as everything inside it.
    selector: ".lod-hide",
    style: { "display": "none" },
  },
  {
    selector: "node.lod-collapsed",
    style: {
      "background-color": t.groupFill,
      "background-opacity": Math.min(1, t.groupFillOpacity * 4),
      "border-width": 2,
      "border-style": "solid",
      "border-color": t.groupLine,
      "label": "data(label)",
      "text-valign": "center",
      "text-halign": "center",
      "text-margin-y": 0,
      "font-size": 16,
      "font-weight": 600,
      "color": t.groupText,
      // With nothing visible inside it, a box has no contents to take its
      // size from, so it is given one.
      "min-width": 150,
      "min-height": 56,
      "padding": "10px",
      "text-max-width": 170,
      "text-wrap": "wrap",
    },
  },
  {
    // One line of text per shape, and no words on the lines between them.
    selector: "node.lod-terse",
    style: { "label": "data(label)" },
  },
  {
    selector: "edge.lod-terse",
    style: { "label": "" },
  },
  // The ring view. Its boxes say nothing — the shapes are laid out by whose
  // they are, so a box drawn around them would only cover the ring — and its
  // lines are chords, bending towards the middle rather than taking a corner
  // around the outside of it.
  {
    selector: "node.quiet",
    style: { "background-opacity": 0, "border-width": 0, "label": "" },
  },
  {
    // The name of a box, written outside the ring against the stretch of it
    // that box holds, since the box itself is not drawn there.
    selector: "node.ring-label",
    style: {
      "background-opacity": 0,
      "border-width": 0,
      "label": "data(label)",
      "color": t.groupText,
      "font-size": 13,
      "font-weight": 600,
      "text-valign": "center",
      "text-halign": "center",
      "events": "no",
    },
  },
  {
    // A line standing for many is drawn as thickly as it stands for, so the
    // pairs that talk most are the ones the eye lands on.
    selector: "edge.meta",
    style: { "width": (el) => 1.4 + Math.min(4, Math.log2(el.data("weight") || 1)) },
  },
  {
    selector: "edge.chord",
    style: {
      "curve-style": "unbundled-bezier",
      "control-point-distances": "data(bend)",
      "control-point-weights": 0.5,
      "taxi-turn": 0,
    },
  },
  ];
}

// The three levels the picture is drawn at, from small to close:
//
//   overview  collapsible boxes are shut, and the lines between their
//             contents are drawn as one line per pair of boxes.
//   medium    everything is open, with second lines and edge labels off.
//   full      everything, as authored.
//
// The two zoom thresholds carry a margin, so a picture sitting on one of
// them does not flicker between levels as it is nudged.
const levelThresholds = { open: 0.5, words: 0.85 };
const levelMargin = 0.06;

function levelForZoom(zoom, current) {
  let open = levelThresholds.open;
  let words = levelThresholds.words;
  if (current === "overview") open += levelMargin;
  if (current === "medium") { open -= levelMargin; words += levelMargin; }
  if (current === "full") words -= levelMargin;
  if (zoom < open) return "overview";
  if (zoom < words) return "medium";
  return "full";
}

// pinnedLevel is what the button says: a level chosen by hand, or null for
// one that follows the zoom.
let pinnedLevel = null;
let shownLevel = null;

async function applyLevel(level) {
  if (!cy || level === shownLevel) return;
  const from = shownLevel;
  // The scale the reader is looking at, which the switch keeps: the picture
  // changes, the size things are drawn at does not. It is read before the
  // level moves, since which level is showing is what makes a zoom a scale.
  const was = scale();
  shownLevel = level;
  const shut = level === "overview";
  if (shut && from !== "overview") {
    if (!positions.full) {
      savePositions("full");
      spans.full = currentSpan();
    }
  }
  cy.batch(() => {
    cy.elements().removeClass("lod-hide lod-terse");
    if (level === "overview") {
      const shut = cy.nodes(".collapsible");
      shut.union(shut.descendants()).addClass("lod-hide");
      cy.edges(".within").addClass("lod-hide");
      cy.edges(".crossing").addClass("lod-hide");
    } else {
      cy.nodes(".stand").addClass("lod-hide");
      cy.edges(".meta").addClass("lod-hide");
    }
    if (level !== "full") cy.elements().addClass("lod-terse");
  });
  drawDetailButton();
  if (shut) {
    // The box holding whatever was in the middle of the screen, so its
    // stand-in can be put back there once the picture is small.
    const held = from ? middleOf(cy.nodes(":visible").filter((n) => !n.isParent() || n.hasClass("collapsible"))) : null;
    const box = held ? (held.hasClass("collapsible") ? held : held.ancestors(".collapsible").first()) : null;
    if (!restorePositions("overview")) await layOutOverview();
    else spans.overview = spans.overview || currentSpan();
    viewport(zoomAtScale(was, "overview"), box && box.length ? cy.$id(standID(box.id())) : null);
  } else if (from === "overview") {
    const stand = middleOf(cy.nodes(".stand:visible"));
    restorePositions("full");
    viewport(was, stand ? cy.$id(stand.data("opens")) : null);
  }
}

function levelNow() {
  return pinnedLevel || levelForZoom(cy ? scale() : 1, shownLevel);
}

function drawDetailButton() {
  const button = document.getElementById("detail");
  if (!button) return;
  button.disabled = view === "ring";
  button.textContent = pinnedLevel ? `Detail: ${pinnedLevel}` : `Detail: auto (${shownLevel || "full"})`;
}

// The button cycles through pinning each level and back to following the
// zoom, for a reader who wants the small picture on a big screen.
function cycleDetail() {
  // In the ring view the level is not the reader's to choose: the ring is
  // the machine-level picture, and that is what it stays.
  if (view === "ring") return;
  const order = [null, "overview", "medium", "full"];
  pinnedLevel = order[(order.indexOf(pinnedLevel) + 1) % order.length];
  applyLevel(levelNow());
  drawDetailButton();
}

function wireDetail() {
  let queued = false;
  cy.on("zoom", () => {
    if (pinnedLevel || queued || adjusting) return;
    queued = true;
    requestAnimationFrame(() => {
      queued = false;
      applyLevel(levelNow());
    });
  });
}

// The two views. The network is the picture as a picture: shapes wherever
// the forces put them, lines routed around the corners between them. The
// ring is the same graph read as a table of connections: one machine per
// place on a circle, ordered by whose it is, and every line a chord across
// the middle. A ring cannot say where anything is, and it does not try to;
// what it says is who talks to whom, which is the one thing a tangle of a
// hundred shapes hides.
const viewNames = ["network", "ring"];
let view = "network";

// ringPositions places the shut boxes around a circle, ordered by the box
// each of them sits in, so one owner's machines are neighbours on the ring.
function ringPositions() {
  const tops = topShapes().filter((n) => !n.hasClass("ring-label"));
  const order = tops.sort((a, b) => {
    const boxA = a.parent().length ? a.parent().id() : "";
    const boxB = b.parent().length ? b.parent().id() : "";
    if (boxA !== boxB) return boxA < boxB ? -1 : 1;
    return a.data("label") < b.data("label") ? -1 : 1;
  });
  // The circle is as big as its contents need: every shape gets its own
  // width plus a gap of the ring's edge, so nothing on it overlaps.
  const gap = 40;
  let around = 0;
  order.forEach((n) => { around += Math.max(n.outerWidth(), n.outerHeight()) + gap; });
  const radius = Math.max(around / (2 * Math.PI), 200);
  const at = new Map();
  // Where each box's stretch of the ring begins and ends, so its name can be
  // written against the middle of it.
  const arcs = new Map();
  let travelled = 0;
  order.forEach((n) => {
    const step = Math.max(n.outerWidth(), n.outerHeight()) + gap;
    const angle = ((travelled + step / 2) / around) * 2 * Math.PI - Math.PI / 2;
    at.set(n.id(), { x: Math.cos(angle) * radius, y: Math.sin(angle) * radius });
    const box = n.parent().length ? n.parent().id() : "";
    if (box) {
      const arc = arcs.get(box) || { from: angle, to: angle };
      arc.from = Math.min(arc.from, angle);
      arc.to = Math.max(arc.to, angle);
      arcs.set(box, arc);
    }
    travelled += step;
  });
  return { at, arcs, radius };
}

// ringLabels writes each box's name outside the ring, and takes the old ones
// away first, since a ring drawn again is a ring with other arcs.
function ringLabels(arcs, radius) {
  cy.remove(cy.nodes(".ring-label"));
  const out = radius + 90;
  const added = [];
  arcs.forEach((arc, id) => {
    const box = cy.$id(id);
    if (!box.length) return;
    const angle = (arc.from + arc.to) / 2;
    added.push({
      group: "nodes",
      data: { id: `ringlabel:${id}`, label: box.data("label") },
      position: { x: Math.cos(angle) * out, y: Math.sin(angle) * out },
      classes: "ring-label",
      selectable: false,
      grabbable: false,
    });
  });
  cy.add(added);
}

// bendOf is how far a chord leaves the straight line between its ends: the
// distance from the middle of that line to the middle of the ring, which
// puts every chord's belly towards the centre.
function bendOf(edge, centre) {
  const a = edge.source().position();
  const b = edge.target().position();
  const middle = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
  const along = { x: b.x - a.x, y: b.y - a.y };
  const length = Math.hypot(along.x, along.y) || 1;
  // The component of "towards the centre" that is square to the line, which
  // is the number cytoscape wants, signed the way it measures it.
  const across = { x: -along.y / length, y: along.x / length };
  const towards = { x: centre.x - middle.x, y: centre.y - middle.y };
  return Math.round((towards.x * across.x + towards.y * across.y) * 0.55);
}

async function applyView(next) {
  if (!cy || next === view) return;
  view = next;
  drawViewButton();
  if (next === "ring") {
    // The ring is drawn at one level: a hundred ports on a circle is the
    // tangle it was meant to answer, so it shows the machines.
    pinnedLevel = "overview";
    await applyLevel("overview");
    const ring = ringPositions();
    cy.batch(() => {
      ring.at.forEach((p, id) => cy.$id(id).position(p));
      ringLabels(ring.arcs, ring.radius);
    });
    savePositions("ring");
    cy.batch(() => {
      cy.nodes(":parent").addClass("quiet");
      const centre = { x: 0, y: 0 };
      cy.edges(":visible").forEach((e) => {
        e.data("bend", bendOf(e, centre));
        e.addClass("chord");
      });
    });
  } else {
    cy.batch(() => {
      cy.remove(cy.nodes(".ring-label"));
      cy.nodes().removeClass("quiet");
      cy.edges().removeClass("chord");
    });
    pinnedLevel = null;
    restorePositions("overview");
  }
  adjusting = true;
  cy.fit(undefined, 40);
  requestAnimationFrame(() => { adjusting = false; });
  drawDetailButton();
}

function drawViewButton() {
  const button = document.getElementById("view");
  if (button) button.textContent = `View: ${view}`;
}

function cycleView() {
  applyView(viewNames[(viewNames.indexOf(view) + 1) % viewNames.length]);
}

const layout = {
  name: "fcose",
  fit: true,
  quality: "proof",
  animate: false,
  randomize: true,
  nodeSeparation: 120,
  idealEdgeLength: 140,
  nodeRepulsion: 14000,
  // Compounds need their own pull and their own room, or boxes land on top
  // of each other in a picture with many of them.
  gravityCompound: 1.4,
  gravityRangeCompound: 1.2,
  tile: true,
  packComponents: true,
  padding: 30,
};

// The overview is laid out on its own, on a graph of nothing but the shut
// boxes and the lines between them. Keeping the detailed geometry and
// drawing it small does not make it any less tangled: the small picture is
// tangled because it is the big picture seen from further away. So the two
// levels get two sets of positions, both kept, and switching between them
// moves the shapes rather than re-running anything.
const overviewLayout = {
  name: "fcose",
  quality: "proof",
  animate: false,
  randomize: true,
  nodeSeparation: 140,
  idealEdgeLength: 220,
  nodeRepulsion: 20000,
  packComponents: true,
  padding: 30,
};

let cy = null;

// Two remembered geometries, by the id of every leaf shape: the one the full
// layout produced, and the one the overview layout produced. A level change
// puts the shapes back where that level had them.
const positions = { full: null, overview: null, ring: null };

// How wide each of those two pictures is, in the graph's own units. The
// overview is the smaller drawing of the same thing, and the ratio between
// the two is what makes one zoom comparable with the other: a reader looking
// at the overview at twice its own scale is looking at the whole thing at
// twice the scale too, whatever number the viewport happens to hold.
const spans = { full: 0, overview: 0 };

function currentSpan() {
  const box = cy.elements(":visible").boundingBox();
  return Math.max(box.w, box.h, 1);
}

// scale is the zoom as the full picture would count it, so one threshold
// reads the same at either level.
function scale() {
  const zoom = cy.zoom();
  if (shownLevel !== "overview" || !spans.full || !spans.overview) return zoom;
  return zoom * (spans.overview / spans.full);
}

// zoomAtScale is the inverse: the viewport zoom that shows the level named
// at the given scale, so switching levels does not change how big anything
// on screen looks.
function zoomAtScale(value, level) {
  if (level !== "overview" || !spans.full || !spans.overview) return value;
  return value * (spans.full / spans.overview);
}

// adjusting is on while the page moves the viewport itself, so its own
// zooming does not read as the reader asking for another level.
let adjusting = false;

// viewport sets the scale and puts one shape back under the middle of the
// screen: a level change is the same picture at another level of detail, so
// the reader should still be looking at what they were looking at.
function viewport(zoom, onto) {
  adjusting = true;
  cy.zoom({ level: zoom, renderedPosition: { x: cy.width() / 2, y: cy.height() / 2 } });
  if (onto && onto.length) cy.center(onto);
  else cy.center();
  requestAnimationFrame(() => { adjusting = false; });
}

// middleOf answers which of the given shapes is nearest the middle of the
// screen, which is the one the reader is looking at.
function middleOf(candidates) {
  const middle = { x: cy.width() / 2, y: cy.height() / 2 };
  let best = null;
  let bestAway = Infinity;
  candidates.forEach((n) => {
    const at = n.renderedPosition();
    const away = (at.x - middle.x) ** 2 + (at.y - middle.y) ** 2;
    if (away < bestAway) {
      bestAway = away;
      best = n;
    }
  });
  return best;
}

function leafNodes() {
  return cy.nodes().filter((n) => !n.isParent() && !n.hasClass("ring-label"));
}

function savePositions(into) {
  const at = new Map();
  leafNodes().forEach((n) => at.set(n.id(), { ...n.position() }));
  positions[into] = at;
}

function restorePositions(from) {
  const at = positions[from];
  if (!at) return false;
  cy.batch(() => {
    leafNodes().forEach((n) => {
      const p = at.get(n.id());
      if (p) n.position(p);
    });
  });
  return true;
}

// topShapes are what the overview draws: one stand-in per shut box, and
// every shape that sits in none of them.
function topShapes() {
  const stands = cy.nodes(".stand");
  const loose = leafNodes().filter((n) => !n.hasClass("stand") && n.ancestors(".collapsible").length === 0);
  return stands.union(loose);
}

// layOutOverview places the shut boxes on a headless graph of their own and
// copies the result back. Moving a box moves what is inside it, so the
// detailed geometry survives inside each box and only the boxes travel.
function layOutOverview() {
  const tops = topShapes();
  const ids = new Set(tops.map((n) => n.id()));
  const elements = [];
  // The boxes these shapes sit in come along, so the overview groups them
  // the way the picture does rather than scattering one owner's machines.
  const boxes = new Set();
  tops.forEach((n) => n.ancestors().forEach((a) => boxes.add(a.id())));
  for (const id of boxes) {
    const box = cy.$id(id);
    const parent = box.parent();
    elements.push({ data: { id: id, parent: parent.length && boxes.has(parent.id()) ? parent.id() : undefined } });
  }
  // Measured now, with the boxes already shut, so the layout works with the
  // size the overview draws rather than the size the detail needed.
  tops.forEach((n) => {
    const parent = n.parent();
    elements.push({
      data: {
        id: n.id(),
        parent: parent.length && boxes.has(parent.id()) ? parent.id() : undefined,
        w: Math.max(n.outerWidth(), 40),
        h: Math.max(n.outerHeight(), 40),
      },
      position: { ...n.position() },
    });
  });
  // The lines of the overview: the aggregated ones, plus any ordinary line
  // that already runs between two shapes it draws.
  const seen = new Set();
  cy.edges().forEach((e) => {
    const [a, b] = [e.source().id(), e.target().id()];
    if (!ids.has(a) || !ids.has(b) || a === b) return;
    const key = `${a}|${b}`;
    if (seen.has(key)) return;
    seen.add(key);
    elements.push({ data: { id: `o:${key}`, source: a, target: b } });
  });
  const head = cytoscape({
    headless: true,
    // Without its style a headless graph gives every node a size of one
    // pixel, and a layout run on it packs boxes that are hundreds of pixels
    // wide into the same spot.
    styleEnabled: true,
    elements: elements,
    style: [{ selector: "node", style: { width: "data(w)", height: "data(h)" } }],
  });
  return new Promise((resolve) => {
    const run = head.layout(overviewLayout);
    run.one("layoutstop", () => {
      cy.batch(() => {
        head.nodes().forEach((hn) => {
          if (hn.isParent()) return;
          const target = cy.$id(hn.id());
          if (target.length) target.position({ ...hn.position() });
        });
      });
      head.destroy();
      savePositions("overview");
      spans.overview = currentSpan();
      resolve();
    });
    run.run();
  });
}

function drawLegend(graph) {
  const aside = document.getElementById("key");
  const kinds = new Map();
  for (const g of graph.groups || []) if (g.kind) kinds.set(g.kind, (graph.legend || {})[g.kind] || "");
  for (const n of graph.nodes || []) if (n.kind) kinds.set(n.kind, (graph.legend || {})[n.kind] || "");
  for (const e of graph.edges || []) if (e.kind) kinds.set(e.kind, (graph.legend || {})[e.kind] || "");
  if (kinds.size === 0) {
    aside.hidden = true;
    return;
  }
  const rows = [...kinds].map(([kind, meaning]) =>
    `<dt><span class="swatch" style="background:${colourFor(kind)}"></span>${kind}</dt><dd>${meaning}</dd>`
  );
  aside.innerHTML = `<h2>Key</h2><dl>${rows.join("")}</dl>`;
  aside.hidden = false;
}

// Clicking a node dims everything it is not connected to, so one shape's
// reach is readable in a picture too dense to trace by eye. Clicking the
// background clears it.
function wireHighlighting() {
  cy.on("tap", "node", (event) => {
    const node = event.target;
    if (node.isParent()) return;
    const keep = node.closedNeighborhood().union(node.ancestors());
    cy.elements().addClass("faded");
    keep.removeClass("faded");
  });
  cy.on("tap", (event) => {
    if (event.target === cy) cy.elements().removeClass("faded");
  });
}

async function load() {
  showError("");
  let graph;
  try {
    const response = await fetch("api/graph");
    graph = await response.json();
    if (!response.ok) throw new Error(graph.error || response.statusText);
  } catch (err) {
    showError(String(err.message || err));
    return;
  }

  document.title = `${graph.title || "Graph"} · dgs`;
  document.getElementById("title").textContent = graph.title || "Graph";
  const counts = [
    `${(graph.groups || []).length} groups`,
    `${(graph.nodes || []).length} nodes`,
    `${(graph.edges || []).length} edges`,
  ];
  document.getElementById("counts").textContent = counts.join(" · ");

  if (cy) cy.destroy();
  cy = cytoscape({
    container: document.getElementById("graph"),
    elements: elements(graph),
    style: styleSheet(),
    wheelSensitivity: 0.2,
  });
  view = "network";
  positions.ring = null;
  drawViewButton();
  wireHighlighting();
  wireDetail();
  drawLegend(graph);
  // The layout runs after the legend, so the container has its final width
  // before anything is placed, and it is run rather than passed to the
  // constructor so its stop event is there to listen for.
  runLayout();
}

// The graph fills whatever space it is given, so a resized window re-fits
// rather than leaving the picture where the old width put it.
window.addEventListener("resize", () => {
  if (!cy) return;
  cy.resize();
  cy.fit(undefined, 40);
});

function runLayout() {
  if (!cy) return;
  // Showing or hiding the key changes the container's width, and the change
  // lands after this frame; resizing first means the layout and the fit use
  // the width the graph actually has.
  requestAnimationFrame(() => {
    cy.resize();
    const run = cy.layout({ ...layout, eles: cy.elements().difference(cy.elements(".stand, .meta")) });
    run.one("layoutstop", () => {
      cy.resize();
      cy.fit(undefined, 40);
      // The fit decides the zoom, and the zoom decides the level, so the
      // first level is chosen here rather than before anything is placed.
      shownLevel = null;
      applyLevel(levelNow());
    });
    run.run();
  });
}

document.getElementById("reload").addEventListener("click", load);
document.getElementById("fit").addEventListener("click", () => cy && cy.fit(undefined, 30));
document.getElementById("relayout").addEventListener("click", runLayout);
document.getElementById("detail").addEventListener("click", cycleDetail);
document.getElementById("view").addEventListener("click", cycleView);
load();
