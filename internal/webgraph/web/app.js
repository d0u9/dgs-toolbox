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
  return darken(colourFor(kind));
}

function darken(hex) {
  const n = parseInt(hex.slice(1), 16);
  const darker = [(n >> 16) & 255, (n >> 8) & 255, n & 255]
    .map((c) => Math.round(c * 0.72).toString(16).padStart(2, "0"))
    .join("");
  return `#${darker}`;
}

// Filters are the caller's switches: an element tagged with a filter is shown
// while that filter is on. filterOn holds the ones switched on, and a line
// can be coloured by its first tag instead of its kind, since the filters
// are usually what a reader is tracing.
let filterOn = new Set();
let filterColour = new Map();
let colourByFilter = false;

function edgeColour(kind, tags) {
  if (colourByFilter && tags && tags.length && filterColour.has(tags[0])) return darken(filterColour.get(tags[0]));
  return lineColourFor(kind);
}

// The union of the tags on a set of lines, for a line standing for them.
function unionTags(into, tags) {
  for (const t of tags || []) if (!into.includes(t)) into.push(t);
  return into;
}

const lineStyles = ["solid", "dashed", "dotted"];
function lineStyleFor(kind) {
  if (!kind) return "solid";
  return lineStyles[indexFor(kind) % lineStyles.length];
}

function showError(message) {
  const box = document.getElementById("error");
  box.textContent = message;
  box.hidden = !message;
}

function elements(graph) {
  return [...(graph.groups || []).map(g => ({...g, isGroup: true})),
    ...(graph.nodes || []).map(n => ({...n, parent: n.group})),
  ].map(n => ({...n, detail: n.detail || "", tags: n.tags || [], colour: colourFor(n.kind), textColour: textOn(colourFor(n.kind))}));
}

let graph, all = [], scene, selected = null;
let view = "network", level = "overview", includeDeclarations = false;
const layouts = new Map();
const elk = new ELK();
let renderRequest = 0;
const svg = d3.select("#graph").append("svg").attr("aria-label", "Connectivity graph");
const world = svg.append("g");
const zoom = d3.zoom().scaleExtent([0.03, 8]).on("zoom", event => world.attr("transform", event.transform));
svg.call(zoom).on("dblclick.zoom", null).on("click", event => {
  if (event.target === svg.node()) select(null);
});

function visibleTags(data) {
  return !(data.tags || []).length || data.tags.some(tag => filterOn.has(tag));
}

function project(scope) {
  if (scope === true) scope = "overview";
  if (scope === false) scope = "full";
  const originals = new Map(all.map(n => [n.id, n]));
  const representatives = new Map();
  for (const n of all) {
    let at = n, representative = n;
    while (at) {
      if (scope === "overview" && at.collapse) representative = at;
      if (scope === "services" && at.summary) { representative = at; break; }
      at = originals.get(at.parent);
    }
    representatives.set(n.id, representative.id);
  }
  const nodes = all.filter(n => representatives.get(n.id) === n.id && (includeDeclarations || !n.contextOnly)).map(n => {
    const summary = scope === "overview" && n.collapse || scope === "services" && n.summary;
    return {...n, isGroup: n.isGroup && !summary, opens: summary ? n.id : undefined};
  });
  const byID = new Map(nodes.map(n => [n.id, n]));
  const shown = n => visibleTags(n) && (!byID.has(n.parent) || shown(byID.get(n.parent)));
  const shownIDs = new Set(nodes.filter(shown).map(n => n.id));
  const edges = [];
  const merged = new Map();
  for (const [i, edge] of (graph.edges || []).entries()) {
    if (!visibleTags(edge)) continue;
    const source = representatives.get(edge.from), target = representatives.get(edge.to);
    if (!shownIDs.has(source) || !shownIDs.has(target)) continue;
    if (scope === "services" && view !== "network" && (byID.get(source).isGroup || byID.get(target).isGroup)) continue;
    const aggregate = scope !== "full" && scope !== "medium";
    if (aggregate && source === target && edge.from !== edge.to) continue;
    const key = `${source.length}:${source}${target}`;
    if (aggregate && merged.has(key)) {
      const e = merged.get(key); e.weight++; e.labels.push(edge.label || "");
      unionTags(e.tags, edge.tags); e.label = `${e.weight} connections`;
      if (e.kind !== (edge.kind || "")) e.kind = "";
    } else {
      const e = {...edge, source, target, id: `edge:${i}`, label: edge.label || "", labels: [edge.label || ""], kind: edge.kind || "", tags: [...(edge.tags || [])], weight: 1};
      edges.push(e); if (aggregate) merged.set(key, e);
    }
  }
  if (view === "network" && scope === "services") {
    for (const node of nodes) if (byID.get(node.parent)?.region) {
      node.regions = [node.parent];
      node.parent = byID.get(node.parent).parent;
    }
    for (const edge of edges.filter(e => e.membership)) {
      const node = byID.get(edge.source);
      node.regions ||= [];
      if (!node.regions.includes(edge.target)) node.regions.push(edge.target);
    }
    for (let i = edges.length-1; i >= 0; i--) if (edges[i].membership) edges.splice(i, 1);
  }
  if (view === "network" && scope !== "overview" && scope !== "services") {
    const references = new Map();
    for (const edge of edges.filter(e => e.membership)) {
      const source = byID.get(edge.source), network = byID.get(edge.target);
      if (!network.isGroup || source.id === network.id) continue;
      const key = JSON.stringify([source.id, network.id]);
      if (!references.has(key)) {
        const id = `reference:${key}`;
        const reference = {...source, id, parent: network.id, isGroup: false,
          reference: source.id, opens: undefined, summary: false, collapse: false,
          detail: `Reference · ${network.label}`,
          tooltip: `Same component as ${source.label} in ${byID.get(source.parent)?.label || "its primary group"}.\n${source.detail}`,
          tags: [],
        };
        references.set(key, reference); nodes.push(reference); byID.set(id, reference); shownIDs.add(id);
      }
      edge.membershipLabel = network.label;
      edge.target = references.get(key).id;
      edge.label = edge.label || `Member of ${network.label}`;
    }
  }
  const needed = new Set([...edges.flatMap(e => [e.source, e.target]), ...nodes.flatMap(n => n.regions || [])]);
  // Empty container declarations cannot occupy a layout slot on their own.
  for (const n of nodes) if (!n.isGroup && shownIDs.has(n.id)) needed.add(n.id);
  for (const id of [...needed]) {
    for (let n = byID.get(id); n; n = byID.get(n.parent)) needed.add(n.id);
  }
  return {nodes: nodes.filter(n => shownIDs.has(n.id) && needed.has(n.id)), edges};
}

function geometry(scope) {
  const key = JSON.stringify([view, scope, includeDeclarations, [...filterOn].sort()]);
  if (!layouts.has(key)) layouts.set(key, buildGeometry(scope));
  return layouts.get(key);
}

async function buildGeometry(scope) {
  const model = project(scope);
  const byID = new Map(model.nodes.map(n => [n.id, n]));
  for (const n of model.nodes) n.regionOverlay = view === "network" && scope === "services" && n.region;
  const roots = [];
  const degree = d3.rollup(model.edges.flatMap(e => [e.source, e.target]), ids => ids.length, id => id);
  for (const n of model.nodes) {
    n.displayLabel = view === "network" && scope === "services" && !n.isGroup ? n.label.split("/").at(-1) : n.label;
    n.membershipLabel = (n.regions || []).map(id => byID.get(id)?.label || id).join(" · ");
    n.children = []; n.w = Math.max(110, Math.min(240, (n.displayLabel || "").length * 7 + 28)); n.h = Math.max(52, (degree.get(n.id) || 0)*10+20);
    if (n.membershipLabel) n.w = Math.max(n.w, 180, n.membershipLabel.length*6+28);
  }
  for (const n of model.nodes) {
    const parent = byID.get(n.parent);
    if (parent) parent.children.push(n); else roots.push(n);
  }
  if (view !== "network") {
    radialGeometry(model, roots);
  } else if (model.nodes.length) {
    await networkGeometry(model, roots, byID);
    membershipRegions(model);
  }
  model.edges.forEach(e => { e.source = byID.get(e.source); e.target = byID.get(e.target); });
  routeLanes(model.edges);
  return model;
}

async function networkGeometry(model, roots, byID) {
  function node(n) {
    const value = {id: n.id, width: n.w, height: n.h};
    if (n.children.length) {
      const children = n.children.filter(c => !c.regionOverlay);
      if (children.some(c => c.regions?.length)) {
        const buckets = d3.group(children, c => [...(c.regions || [])].sort().join("\u0000"));
        value.children = Array.from(buckets.values(), (members, index) => ({
          id:`layout-bucket:${n.id}:${index}`, children:members.map(node),
          layoutOptions:{"elk.padding":"[top=72,left=72,bottom=72,right=72]", "elk.spacing.nodeNode":"64"},
        }));
      } else value.children = children.map(node);
      value.layoutOptions = {"elk.padding": "[top=36,left=24,bottom=24,right=24]"};
    }
    return value;
  }
  const result = await elk.layout({
    id: "layout-root", children: roots.filter(n => !n.regionOverlay).map(node),
    edges: model.edges.map(e => ({id: e.id, sources: [e.source], targets: [e.target]})),
    layoutOptions: {
      "elk.algorithm": "layered", "elk.direction": "RIGHT",
      "elk.edgeRouting": "ORTHOGONAL", "elk.hierarchyHandling": "INCLUDE_CHILDREN",
      "elk.spacing.nodeNode": model.nodes.some(n => n.regionOverlay) ? "64" : "40", "elk.layered.spacing.nodeNodeBetweenLayers": model.nodes.some(n => n.regionOverlay) ? "220" : "90",
      "elk.layered.spacing.edgeNodeBetweenLayers": "24", "elk.spacing.edgeNode": "20",
      "elk.spacing.edgeEdge": "12", "elk.layered.spacing.edgeEdgeBetweenLayers": "12",
      "elk.layered.mergeEdges": "false", "elk.randomSeed": "1",
    },
  });
  const routes = new Map(model.edges.map(e => [e.id, e]));
  const origins = new Map();
  function read(container, ox = 0, oy = 0) {
    for (const n of container.children || []) {
      const value = byID.get(n.id), x = ox+n.x, y = oy+n.y;
      origins.set(n.id, {x,y});
      if (value) { value.x = x+n.width/2; value.y = y+n.height/2; value.w = n.width; value.h = n.height; }
      else if (n.id.startsWith("layout-bucket:")) model.membershipBuckets.push({
        x:x+n.width/2,y:y+n.height/2,w:n.width-100,h:n.height-100,
        regions:byID.get(n.children[0]?.id)?.regions || [],
      });
      read(n, x, y);
    }
    for (const e of container.edges || []) {
      const edge = routes.get(e.id);
      const origin = origins.get(e.container) || {x:0,y:0};
      const ex = origin.x, ey = origin.y;
      edge.points = (e.sections || []).flatMap(section => [section.startPoint, ...(section.bendPoints || []), section.endPoint]
        .map(p => ({x: ex+p.x, y: ey+p.y})));
      if (edge.points.length < 2) throw new Error("Layout did not route a connection");
    }
  }
  model.membershipBuckets = [];
  read(result);
  for (const e of model.edges) if (!e.points) throw new Error("Layout omitted a connection");
  model.bounds = model.edges.flatMap(e => e.points.map(p => ({...p, w: 0, h: 0})));
}

// Contours permit overlaps at shared components; non-members are cut out.
function membershipRegions(model) {
  model.regions = [];
  const names = [...new Set((graph.groups || []).filter(g => g.region).map(g => g.label))];
  for (const region of model.nodes.filter(n => n.regionOverlay)) {
    const members = model.nodes.filter(n => (n.regions || []).includes(region.id));
    if (!members.length) continue;
    const siblings = model.nodes.filter(n => n.regionOverlay && n.parent === region.parent);
    const padding = 22 + siblings.indexOf(region)*14, step = 4;
    const x0 = d3.min(members, n => n.x-n.w/2)-padding-step;
    const y0 = d3.min(members, n => n.y-n.h/2)-padding-step;
    const width = Math.ceil((d3.max(members, n => n.x+n.w/2)+padding+step-x0)/step);
    const height = Math.ceil((d3.max(members, n => n.y+n.h/2)+padding+step-y0)/step);
    const areas = model.membershipBuckets.filter(b => b.regions.includes(region.id));
    const others = model.nodes.filter(n => !n.isGroup && !(n.regions || []).includes(region.id));
    const inside = (n, x, y, pad) => Math.abs(x-n.x) <= n.w/2+pad && Math.abs(y-n.y) <= n.h/2+pad;
    const values = new Uint8Array(width*height);
    for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
      const px = x0+x*step, py = y0+y*step;
      values[y*width+x] = (areas.length ? areas : members).some(n => inside(n,px,py,padding)) && !others.some(n => inside(n,px,py,8)) ? 1 : 0;
    }
    const contour = d3.contours().size([width,height]).thresholds([.5])(values)[0];
    const projection = d3.geoIdentity().scale(step).translate([x0,y0]);
    region.path = d3.geoPath(projection)(contour);
    region.w = width*step; region.h = height*step;
    region.x = x0+region.w/2; region.y = y0+region.h/2;
    region.contour = contour; region.grid = {x:x0,y:y0,step};
    region.labels = contour.coordinates.flatMap(polygon => {
      const member = members.find(n => d3.polygonContains(polygon[0], [(n.x-x0)/step,(n.y-y0)/step]));
      return member ? [{label:region.label, x:member.x-member.w/2-padding+4, y:member.y-member.h/2-padding+14}] : [];
    });
    region.regionColour = filterColour.get(region.colorFilter) || paletteAt(names.indexOf(region.label));
    model.regions.push(region);
    model.bounds.push({x:x0+region.w/2,y:y0+region.h/2,w:region.w,h:region.h});
  }
}

function radialGeometry(model, roots) {
  const root = d3.hierarchy({id: "", label: graph.title || "Graph", isGroup: true, children: roots});
  root.sort((a, b) => d3.ascending(a.data.label, b.data.label) || d3.ascending(a.data.id, b.data.id));
  const radius = Math.max(240, root.leaves().length * 24 / (2 * Math.PI));
  d3.cluster().size([2*Math.PI, radius]).separation((a, b) => a.parent === b.parent ? 1 : 2)(root);
  model.hierarchy = root;
  model.treeLinks = root.links();
  root.each(n => {
    n.data.angle = n.x; n.data.radius = n.y; n.data.hierarchy = n;
    n.data.x = Math.sin(n.x)*n.y; n.data.y = -Math.cos(n.x)*n.y;
    n.data.w = n.data.isGroup ? 16 : 12; n.data.h = n.data.w;
  });
  model.sectors = [];
  if (view === "ring-services") for (const machine of model.nodes.filter(n => n.collapse && n.isGroup)) {
    const leaves = machine.hierarchy.leaves().filter(n => !n.data.isGroup);
    if (!leaves.length) continue;
    const from = d3.min(leaves, n => n.x), to = d3.max(leaves, n => n.x);
    const labelWidth = Math.min(240, Math.max(...leaves.map(n => n.data.label.length * 7 + 30)));
    const sector = {id: machine.id, label: machine.label, from: from-.025, to: to+.025, inner: radius-24, outer: radius+labelWidth, angle: (from+to)/2};
    sector.path = d3.arc().innerRadius(sector.inner).outerRadius(sector.outer).startAngle(sector.from).endAngle(sector.to)();
    sector.x = Math.sin(sector.angle)*(radius-38); sector.y = -Math.cos(sector.angle)*(radius-38);
    model.sectors.push(sector);
  }
  const outer = radius + Math.max(40, ...model.nodes.filter(n => !n.isGroup).map(n => Math.min(240, n.label.length*7+24)));
  model.bounds = [{x: -outer, y: -outer, w: 0, h: 0}, {x: outer, y: outer, w: 0, h: 0}];
}

// Opposite directions share a pair, so each connection gets a distinct lane.
function routeLanes(edges) {
  const pairs = new Map();
  for (const e of edges) {
    const [a, b] = [e.source.id, e.target.id].sort();
    const key = `${a.length}:${a}${b}`;
    if (!pairs.has(key)) pairs.set(key, []);
    pairs.get(key).push(e);
  }
  for (const group of pairs.values()) group.forEach((e, i) => { e.lane = i-(group.length-1)/2; e.loop = i; });

}

// Circular fillets stay on the routed channel; a spline can cut through boxes.
function networkRoute(e) {
  const points = e.points;
  let path = `M${points[0].x},${points[0].y}`;
  for (let i = 1; i < points.length-1; i++) {
    const a = points[i-1], b = points[i], c = points[i+1];
    const incoming = Math.hypot(b.x-a.x, b.y-a.y), outgoing = Math.hypot(c.x-b.x, c.y-b.y);
    if (!incoming || !outgoing) continue;
    const radius = Math.min(10, incoming/2, outgoing/2);
    const x = b.x+(a.x-b.x)*radius/incoming, y = b.y+(a.y-b.y)*radius/incoming;
    const tx = b.x+(c.x-b.x)*radius/outgoing, ty = b.y+(c.y-b.y)*radius/outgoing;
    path += `L${x},${y}Q${b.x},${b.y} ${tx},${ty}`;
  }
  const last = points[points.length-1]; path += `L${last.x},${last.y}`;
  let longest = 0, midpoint = points[0];
  for (let i = 1; i < points.length; i++) {
    const a = points[i-1], b = points[i], length = Math.hypot(b.x-a.x, b.y-a.y);
    if (length > longest) { longest = length; midpoint = {x: (a.x+b.x)/2, y: (a.y+b.y)/2}; }
  }
  return {path, x: midpoint.x, y: midpoint.y};
}

function radialRoute(e) {
  if (e.source === e.target) {
    const n = e.source, r = 20 + e.loop*8;
    return `M${n.x},${n.y}c${r},${-r} ${-r},${-r} 0,-8`;
  }
  const points = e.source.hierarchy.path(e.target.hierarchy).map(n => [n.x, n.y]);
  const lane = e.lane*12;
  for (let i = 1; i < points.length-1; i++) {
    points[i] = [points[i][0] + lane/Math.max(80, points[i][1]), Math.max(12, points[i][1] + Math.abs(lane))];
  }
  // Stop before the dot so the arrowhead stays visible above the hierarchy.
  points[0][1] -= 8; points[points.length-1][1] -= 10;
  return d3.lineRadial().angle(p => p[0]).radius(p => p[1]).curve(d3.curveBundle.beta(.72))(points);
}

function scopeNow() { return view === "ring" ? "overview" : view === "ring-services" ? "services" : level; }
async function render(fit = false) {
  if (!graph) return;
  const request = ++renderRequest;
  document.getElementById("graph").setAttribute("aria-busy", "true");
  document.getElementById("layout-state").hidden = false;
  let next;
  try { next = await geometry(scopeNow()); }
  catch (error) { if (request === renderRequest) { showError(`Layout failed: ${error.message}`); document.getElementById("graph").setAttribute("aria-busy", "false"); document.getElementById("layout-state").hidden = true; } return false; }
  if (request !== renderRequest) return false;
  scene = next;
  showError("");
  const byID = new Map(scene.nodes.map(n => [n.id, n]));
  const nodes = scene.nodes, edges = scene.edges, radial = view !== "network";
  world.selectAll("*").remove();
  world.append("defs").append("marker").attr("id", "arrow").attr("viewBox", "0 -5 10 10").attr("refX", 9).attr("markerWidth", 6).attr("markerHeight", 6).attr("orient", "auto").append("path").attr("d", "M0,-5L10,0L0,5").attr("fill", "context-stroke");
  if (radial) {
    world.append("g").selectAll("path").data(scene.sectors).join("path").attr("class", "host-sector").attr("d", n => n.path)
      .attr("role", "button").attr("tabindex", 0).attr("aria-label", n => `${n.label} services`)
      .on("click", (event, n) => { event.stopPropagation(); select(byID.get(n.id)); })
      .on("keydown", (event, n) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); select(byID.get(n.id)); } });
    world.append("g").selectAll("path").data(scene.treeLinks).join("path").attr("class", "tree-link")
      .attr("d", d3.linkRadial().angle(n => n.x).radius(n => n.y));
    world.append("g").selectAll("text").data(scene.sectors).join("text").attr("class", "host-label")
      .attr("x", n => n.x).attr("y", n => n.y).text(n => n.label);
  }
  const regions = world.append("g").selectAll("g").data(scene.regions || []).join("g").attr("class", "membership-region");
  regions.append("path").attr("d", n => n.path).attr("fill", n => n.regionColour).attr("stroke", n => n.regionColour);
  regions.selectAll("text").data(n => n.labels.map(label => ({...label, colour:n.regionColour}))).join("text")
    .attr("x", n => n.x).attr("y", n => n.y).attr("fill", n => n.colour).text(n => n.label);
  const groups = world.append("g").selectAll("g").data(radial ? [] : nodes.filter(n => n.isGroup && !n.regionOverlay)).join("g").attr("class", "group");
  groups.attr("tabindex", 0).attr("role", "button").attr("aria-label", n => `${n.label}. Select to show connections.`)
    .on("dblclick", (event, n) => { event.stopPropagation(); frame([n]); })
    .on("click", (event, n) => { event.stopPropagation(); select(n); })
    .on("keydown", (event, n) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); select(n); } });
  groups.append("rect").attr("rx", 10).style("fill-opacity", scene.regions?.length ? 0 : null);
  groups.append("text").attr("class", "group-label").text(n => n.label);
  const lines = world.append("g").selectAll("g").data(edges).join("g").attr("class", radial ? "edge radial-edge" : "edge");
  lines.append("path").attr("class", "connection").attr("fill", "none").attr("marker-end", "url(#arrow)").attr("stroke", e => edgeColour(e.kind, e.tags))
    .attr("stroke-width", e => Math.min(4, 1.25 + Math.log2(e.weight || 1)*.5)).attr("stroke-dasharray", e => ({solid: null, dashed: "7 4", dotted: "2 4"})[lineStyleFor(e.kind)]);
  lines.append("path").attr("class", "edge-hit").attr("fill", "none").attr("stroke", "transparent").attr("stroke-width", 14);
  lines.append("title").text(e => `${e.source.label} → ${e.target.label}: ${e.labels.filter(Boolean).join(", ") || e.label}`);
  lines.append("text").attr("class", "edge-label").text(e => e.label);
  const shapes = world.append("g").selectAll("g").data(radial ? nodes : nodes.filter(n => !n.isGroup)).join("g").attr("class", n => `node${radial ? " radial-node" : ""}${n.isGroup ? " branch-node" : ""}${n.reference ? " reference-node" : ""}`)
    .attr("tabindex", 0).attr("role", "button").attr("aria-label", n => `${n.label}. ${n.detail}. Select to show connections.`)
    .on("click", (event, n) => { event.stopPropagation(); select(n); })
    .on("keydown", (event, n) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); select(n); } })
    .on("dblclick", async (event, n) => { event.stopPropagation(); if (n.opens && !radial) { level = "full"; if (!await render()) return; const target = scene.nodes.find(c => c.id === n.opens); if (target) frame([target]); } else frame([n]); });
  if (radial) {
    shapes.append("circle").attr("class", "node-hit").attr("r", 12).attr("fill", "transparent");
    shapes.append("circle").attr("r", n => n.isGroup ? 3 : 5).attr("fill", n => n.colour);
    shapes.append("text").attr("fill", "#1a1a1a").text(n => n.isGroup && scene.sectors.some(s => s.id === n.id) ? "" : shorten(n.label, 250))
      .attr("transform", n => `rotate(${n.angle*180/Math.PI-90+(n.angle >= Math.PI ? 180 : 0)})`)
      .attr("x", n => n.angle >= Math.PI ? -10 : 10).attr("dy", ".32em").style("text-anchor", n => n.angle >= Math.PI ? "end" : "start");
  } else {
    shapes.append("rect").attr("rx", n => 4 + indexFor(n.kind) % 4 * 4).attr("fill", n => n.colour);
    shapes.append("text").attr("fill", n => n.textColour).text(n => shorten(n.displayLabel || n.label, n.w));
    shapes.append("text").attr("class", "node-detail").attr("fill", n => n.textColour).text(n => level === "medium" ? "" : shorten(level === "services" && n.regions?.length ? n.membershipLabel : n.detail, n.w));

  }
  shapes.append("title").text(n => n.tooltip || n.detail || n.label);
  function draw() {
    groups.attr("transform", n => `translate(${n.x},${n.y})`);
    groups.select("rect").attr("x", n => -n.w/2).attr("y", n => -n.h/2).attr("width", n => n.w).attr("height", n => n.h);
    groups.select("text").attr("x", n => -n.w/2 + 12).attr("y", n => -n.h/2 + 20);
    shapes.attr("transform", n => `translate(${n.x},${n.y})`);
    if (!radial) {
      shapes.select("rect").attr("x", n => -n.w/2).attr("y", n => -n.h/2).attr("width", n => n.w).attr("height", n => n.h);
      shapes.select("text").attr("y", -2); shapes.select(".node-detail").attr("y", 15);
    }
    lines.selectAll("path").attr("d", e => radial ? radialRoute(e) : networkRoute(e).path);
    lines.select("text").attr("x", e => radial ? (e.source.x+e.target.x)/4 : networkRoute(e).x).attr("y", e => (radial ? (e.source.y+e.target.y)/4 : networkRoute(e).y)-6);
  }
  draw();
  selected = selected ? byID.get(selected.id) || null : null;
  highlight(); buttons();
  document.getElementById("empty").hidden = nodes.length > 0;
  document.getElementById("ring-help").hidden = !radial;
  document.getElementById("graph").setAttribute("aria-busy", "false");
  document.getElementById("layout-state").hidden = true;
  if (fit) frame();
  return true;
}

function shorten(text, width) {
  const count = Math.floor((width-20)/6);
  return text.length > count ? text.slice(0, count-1) + "…" : text;
}
function select(n) { selected = n; highlight(); }
function highlight() {
  const keep = new Set();
  function include(n) { keep.add(n.id); n.children.forEach(include); if (n.regionOverlay) for (const member of scene.nodes) if ((member.regions || []).includes(n.id)) keep.add(member.id); }
  if (selected) {
    include(selected);
    const canonical = scene.nodes.find(n => n.id === (selected.reference || selected.id));
    if (canonical && canonical !== selected) include(canonical);
    for (const n of scene.nodes) if (n.reference === (selected.reference || selected.id)) keep.add(n.id);
  }
  const direct = new Set(keep);
  if (selected) for (const e of scene.edges) {
    if (direct.has(e.source.id) || direct.has(e.target.id)) { keep.add(e.source.id); keep.add(e.target.id); }
  }
  world.selectAll(".membership-region").style("opacity", n => selected && !scene.nodes.some(member => direct.has(member.id) && member.regions?.includes(n.id)) ? .2 : 1);
  world.selectAll(".node").classed("faded", n => selected && !keep.has(n.id)).classed("selected", n => selected && n.id === selected.id);
  world.selectAll(".group").classed("selected", n => selected && n.id === selected.id);
  world.selectAll(".edge").classed("faded", e => selected && !direct.has(e.source.id) && !direct.has(e.target.id))
    .classed("active", e => selected && (direct.has(e.source.id) || direct.has(e.target.id)));
  world.selectAll(".edge-label").style("display", selected ? "none" : null);
  const box = document.getElementById("selection"); box.hidden = !selected;
  const connections = selected ? scene.edges.filter(e => direct.has(e.source.id) || direct.has(e.target.id)) : [];
  box.textContent = selected ? [selected.label, selected.kind, selected.detail, selected.tooltip,
    ...(selected.regions || []).map(id => `Network: ${scene.nodes.find(n => n.id === id)?.label || id}`),
    ...connections.map(e => e.membershipLabel ? `${e.source.label} · member of ${e.membershipLabel}` : `${e.source.label} → ${e.target.label}: ${e.labels.filter(Boolean).join(", ") || e.label}`)].filter(Boolean).join("\n") : "";
}
function frame(nodes = scene?.nodes || []) {
  if (!nodes.length) return;
  if (nodes === scene.nodes) nodes = [...nodes, ...scene.bounds];
  const width = svg.node().clientWidth, height = svg.node().clientHeight;
  const x0 = d3.min(nodes, n => n.x-n.w/2), x1 = d3.max(nodes, n => n.x+n.w/2);
  const y0 = d3.min(nodes, n => n.y-n.h/2), y1 = d3.max(nodes, n => n.y+n.h/2);
  const scale = Math.max(.03, Math.min(3, (width-64)/(x1-x0 || 1), (height-64)/(y1-y0 || 1)));
  svg.call(zoom.transform, d3.zoomIdentity.translate(width/2, height/2).scale(scale).translate(-(x0+x1)/2, -(y0+y1)/2));
}
function buttons() {
  document.getElementById("view-state").textContent = view === "network" ? "Network" : view === "ring" ? "Ring · hosts" : "Ring · services";
  document.querySelectorAll("[data-view]").forEach(button => button.setAttribute("aria-pressed", button.dataset.view === view));
  document.querySelectorAll("[data-level]").forEach(button => { button.setAttribute("aria-pressed", button.dataset.level === level); button.disabled = view !== "network"; });
}
function applyFilters() { render(true); }
function recolourEdges() { render(); }
function drawFilters(graph) {
  const filters = graph.filters || [];
  filterColour = new Map(filters.map((f, i) => [f.id, paletteAt(i)]));
  filterOn = new Set(filters.map((f) => f.id));
  const box = document.getElementById("filters");
  if (filters.length === 0) {
    box.hidden = true;
    return;
  }
  const esc = (v) => String(v).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
  const rows = filters.map((f) =>
    `<label class="filter"><input type="checkbox" data-filter="${esc(f.id)}" checked>` +
    `<span class="swatch" style="background:${filterColour.get(f.id)}"></span>` +
    `<span>${esc(f.label)}${f.detail ? `<small>${esc(f.detail)}</small>` : ""}</span></label>`
  );
  box.innerHTML = `<h2 class="panel-head">${esc(graph.filterTitle || "Filters")}</h2>${rows.join("")}` +
    `<label class="filter colour-by"><input type="checkbox" data-colour-by${colourByFilter ? " checked" : ""}><span>Colour lines by these</span></label>`;
  box.hidden = false;
  box.querySelectorAll("input[data-filter]").forEach((input) => {
    input.addEventListener("change", () => {
      if (input.checked) filterOn.add(input.dataset.filter);
      else filterOn.delete(input.dataset.filter);
      applyFilters();
    });
  });
  box.querySelector("input[data-colour-by]").addEventListener("change", (event) => {
    colourByFilter = event.target.checked;
    recolourEdges();
  });
}

function drawLegend(graph) {
  const aside = document.getElementById("key");
  const kinds = new Map();
  for (const g of graph.groups || []) if (g.kind) kinds.set(g.kind, (graph.legend || {})[g.kind] || "");
  for (const n of graph.nodes || []) if (n.kind) kinds.set(n.kind, (graph.legend || {})[n.kind] || "");
  for (const e of graph.edges || []) if (e.kind) kinds.set(e.kind, (graph.legend || {})[e.kind] || "");
  if (kinds.size === 0 && !(graph.filters || []).length) {
    aside.hidden = false;
    document.getElementById("legend").replaceChildren();
    return;
  }
  const esc = v => String(v).replace(/[&<>"]/g, c => ({"&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;"})[c]);
  const rows = [...kinds].map(([kind, meaning]) =>
    `<dt><span class="swatch" style="background:${colourFor(kind)}"></span>${esc(kind)}</dt><dd>${esc(meaning)}</dd>`
  );
  document.getElementById("legend").innerHTML = `<h2 class="panel-head">Key</h2><dl>${rows.join("")}</dl>`;
  aside.hidden = false;
}



async function load() {
  showError("");
  try {
    const response = await fetch("api/graph");
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || response.statusText);
    graph = data;
  } catch (error) { showError(error.message); return; }
  document.title = `${graph.title || "Graph"} · dgs`;
  document.getElementById("title").textContent = graph.title || "Graph";
  document.getElementById("counts").textContent = `${(graph.nodes || []).length} nodes · ${(graph.edges || []).length} edges`;
  drawFilters(graph); drawLegend(graph);
  all = elements(graph); layouts.clear(); selected = null; scene = null;
  await render(true); search();
}
function search() {
  const query = document.getElementById("search").value.trim().toLowerCase();
  const results = document.getElementById("results"); results.replaceChildren(); results.hidden = !query;
  if (!graph || !query) return;
  const matches = all.filter(n => `${n.label} ${n.detail}`.toLowerCase().includes(query));
  if (!matches.length) results.textContent = "No matches";
  for (const match of matches.slice(0, 30)) {
    const button = document.createElement("button"); button.className = "text-button"; button.textContent = match.label;
    button.onclick = async () => {
      const visible = scene?.nodes.find(n => n.id === match.id);
      if (visible) { select(visible); frame([visible]); return; }
      if (match.contextOnly) { includeDeclarations = true; document.getElementById("declarations").checked = true; }
      const byID = new Map(all.map(n => [n.id, n]));
      for (let n = match; n; n = byID.get(n.parent)) for (const tag of n.tags) filterOn.add(tag);
      document.querySelectorAll("input[data-filter]").forEach(input => { input.checked = filterOn.has(input.dataset.filter); });
      if (!await render()) return;
      let n = scene.nodes.find(n => n.id === match.id);
      if (!n) {
        view = "network"; level = "full";
        if (!await render()) return;
        n = scene.nodes.find(n => n.id === match.id);
      }
      if (n) { select(n); frame([n]); }
    };
    results.append(button);
  }
}
document.getElementById("search").addEventListener("input", search);
document.getElementById("reload").onclick = load;
document.getElementById("fit").onclick = () => frame();
document.getElementById("relayout").onclick = () => { layouts.clear(); render(true); };
document.querySelectorAll("[data-level]").forEach(button => { button.onclick = () => { level = button.dataset.level; render(true); }; });
document.querySelectorAll("[data-view]").forEach(button => { button.onclick = () => { view = button.dataset.view; render(true); }; });
document.getElementById("declarations").onchange = event => { includeDeclarations = event.target.checked; render(true); };
document.getElementById("zoom-in").onclick = () => svg.call(zoom.scaleBy, 1.4);
document.getElementById("zoom-out").onclick = () => svg.call(zoom.scaleBy, 1/1.4);
document.addEventListener("keydown", event => {
  if (event.target.closest("input, textarea, select")) return;
  if (event.key === "Escape") select(null);
  if (event.key === "+" || event.key === "=") svg.call(zoom.scaleBy, 1.4);
  if (event.key === "-") svg.call(zoom.scaleBy, 1/1.4);
  if (event.key.toLowerCase() === "f") frame();
});
new ResizeObserver(() => { if (scene) svg.attr("viewBox", `0 0 ${svg.node().clientWidth} ${svg.node().clientHeight}`); }).observe(document.getElementById("graph"));
load();
