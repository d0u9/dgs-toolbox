// The part of the Rules form that makes one rule: its if, revisions, a
// layout built from key chips, its children as nested if blocks, and a
// Numbering list per {#}. The page owns the rest of
// its form and what a change redraws; the server computes the result.
import { $, api, el, currentFields, label, post, tagUses } from "/common.js";
import { condition } from "/condition.js";

let state = { templates: [], items: [] };
let keys = [];
let countries = {}; // each country's every form: its alpha-3 code
let previewed = null; // each Item the rule takes to the nodes placing it ("", 2.1), as the server last planned it; null until it has
// top is the rule's own numbering, and each node has its own the same way:
// per order, its names in order (a name may hold a comma), the numbers set
// by hand ({ 结婚证: 6 }), and the names left unnumbered ([押金]).
let top = { order: {}, numbers: {}, unnumbered: {}, unlisted: {} };
let changed = () => {};
// nodes are the rule's children: { if, path, file, default, children,
// exclude, order, numbers, unnumbered } as saved, with ofs, the orders its
// path and file number ({ of, rest }), and error and ifError, as the
// server answered them.
let nodes = [];
const clone = (o) => JSON.parse(JSON.stringify(o || {}));

// setup gives the form the tree's state, the keys a layout can use, the
// country forms, and what to call on every change. Called again when the
// state is reloaded.
export function setup(options) {
  ({ state = state, keys = keys, countries = countries } = options);
  if (options.onChange) changed = options.onChange;
}

// fill shows v's if, selection, file, children and orders. It settles
// once the server has parsed every path and file, when read() stops
// keeping orders nothing numbers.
export function fill(v) {
  previewed = null;
  $("if").value = v.if || "";
  checkIf($("if"), $("if-error"));
  ifBubbles.read();
  document.querySelector(`input[name=selection][value=${v.selection || "head"}]`).checked = true;
  if ($("shared")) $("shared").checked = !!v.shared;
  rows = DEFAULT_ROWS();
  active = { row: rows.length - 1, group: -1 };
  drawInherit(v.inherit || []);
  top = { order: clone(v.order), numbers: clone(v.numbers), unnumbered: clone(v.unnumbered), unlisted: clone(v.unlisted) };
  const take = (n) => ({ if: n.if || "", path: n.path || "", file: n.file || "", default: n.default, children: (n.children || []).map(take),
    exclude: !!n.exclude, order: clone(n.order), numbers: clone(n.numbers), unnumbered: clone(n.unnumbered), unlisted: clone(n.unlisted), ofs: [], error: "", ifError: "" });
  nodes = (v.children || []).map(take);
  drawNodes();
  const parses = every(nodes).map(parseNode);
  drawOrder();
  drawPath();
  // The file goes in its box for the server to read; its rows then replace
  // the default ones.
  root.path = v.path || "";
  $("root-path").value = root.path;
  parses.push(parseNode(root));
  if (v.file) {
    $("layout-text").value = v.file;
    parses.push(typed());
  }
  return Promise.all(parses);
}

// skipItem leaves an Item out, by adding not id is <id> to the rule's if.
export function skipItem(id) {
  const box = $("if"), now = box.value.trim();
  box.value = !now ? "id != " + id : /\|\|/.test(now) ? "(" + now + ") && id != " + id : now + " && id != " + id;
  checkIf(box, $("if-error"));
  ifBubbles.read();
  changed();
}

// setPreviewed tells the form which Items the rule takes and the node
// placing each, so numbering lists their values only.
export function setPreviewed(taken) {
  previewed = taken ? new Map(Object.entries(taken)) : null;
  // A block's lists are drawn again in place, so a field being typed in
  // keeps its focus.
  $("paths")?.querySelectorAll(".order").forEach((box) => box.redraw?.());
  drawOrder();
}

// every is the nodes and all below them, parents first.
const every = (list) => list.flatMap((n) => [n, ...every(n.children)]);

// checkIf has the server read a condition, and marks the box with why not.
let ifAsked = new WeakMap();
async function checkIf(box, out, node) {
  const mine = (ifAsked.get(box) || 0) + 1;
  ifAsked.set(box, mine);
  let error = "";
  if (box.value.trim()) {
    try {
      await post("/api/rules/if", { if: box.value.trim() });
    } catch (err) {
      error = err.message;
    }
  }
  if (ifAsked.get(box) !== mine) return;
  box.classList.toggle("invalid", !!error);
  out.textContent = error;
  if (node) node.ifError = error;
}
$("if").addEventListener("input", () => checkIf($("if"), $("if-error")));

// counters are the orders a parsed layout's {#} number, each with the
// rest it numbers as written.
const counters = (layout) => layout.flat(Infinity).flatMap(function walk(p) {
  return p.counter && p.of ? [{ of: p.of, rest: p.rest }] : p.group ? p.group.flatMap(walk) : [];
});

// parseNode has the server parse a node's path and file, for the orders
// they number.
let parsing = 0;
// root holds the rule's own folders, parsed as a block's are.
const root = { path: "", file: "", children: [], ofs: [], error: "" };
$("root-path").addEventListener("change", () => { root.path = $("root-path").value; changed(); parseNode(root); });
$("root-path-edit").addEventListener("click", () => openEditor({ text: root.path,
  set: (v) => { root.path = v; $("root-path").value = v; parseNode(root); } }));
async function parseNode(n) {
  parsing++;
  n.ofs = [];
  n.error = "";
  for (const text of [n.path, n.file]) {
    if (!text.trim()) continue;
    try {
      n.ofs.push(...counters((await post("/api/rules/layout", { layout: text.trim() })).layout));
    } catch (err) {
      n.error = err.message;
    }
  }
  parsing--;
  if (n === root) {
    $("root-path").classList.toggle("invalid", !!n.error);
    $("root-path-error").textContent = n.error;
  }
  drawNodes();
  drawOrder();
  changed();
}

// drawNodes draws the children as nested blocks, as Scratch draws an if:
// a head with its condition, the path and file it adds, and a slot of
// blocks below it. A block with no condition is the else, and comes last.
let draggingBlock = null; // the block being dragged, { list, n }
// contains says whether list is n's own children or lies below them.
const contains = (n, list) => n.children === list || n.children.some((c) => contains(c, list));
function drawNodes() {
  const box = $("paths");
  if (!box) return;
  const redraw = () => { drawNodes(); changed(); };
  const act = (title, text, onclick, disabled) => el("button", { type: "button", className: "order-act", title, textContent: text, disabled, onclick });
  // icon draws a block's ↑ ↓ × as small line icons.
  const ICONS = { "↑": "M8 13V3M4 7l4-4 4 4", "↓": "M8 3v10M4 9l4 4 4-4", "×": "M4 4l8 8M12 4l-8 8" };
  const icon = (title, text, onclick, disabled) => {
    const b = act(title, "", onclick, disabled);
    b.innerHTML = '<svg viewBox="0 0 16 16" width="12" height="12" aria-hidden="true"><path d="' + ICONS[text] + '" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';
    return b;
  };
  const field = (n, key, label, title) => {
    const input = el("input", { className: "layout-text", value: n[key], spellcheck: false, autocomplete: "off", title,
      placeholder: key === "file" ? "the file name above" : "",
      onchange: () => { n[key] = input.value; changed(); parseNode(n); },
      onkeydown: (event) => { if (event.key === "Enter") { event.preventDefault(); input.blur(); } } });
    return el("label", { className: "block-line" }, el("span", { className: "block-key", textContent: label }), input,
      el("button", { type: "button", className: "path-more", textContent: "Edit…", title: "Build it part by part",
        onclick: () => openEditor({ text: n[key], set: (v) => { n[key] = v; input.value = v; parseNode(n); } }) }));
  };
  const adder = (list) => {
    const hasElse = list.some((n) => !n.if);
    const add = (cond) => () => {
      const n = { if: cond, path: "", file: "", children: [], exclude: false, order: {}, numbers: {}, unnumbered: {}, unlisted: {}, ofs: [], error: "", ifError: "" };
      if (cond && hasElse) list.splice(list.length - 1, 0, n); else list.push(n);
      redraw();
    };
    return el("div", { className: "block-adds" },
      el("button", { type: "button", className: "button block-add", textContent: "+ if", onclick: add("type == " + (state.templates[0]?.type || "x")) }),
      hasElse ? null : el("button", { type: "button", className: "button block-add", textContent: "+ else", onclick: add("") }));
  };
  const blocks = (list, up = [top]) => list.map((n, i) => {
    const isElse = !n.if && i === list.length - 1;
    const cond = el("input", { className: "block-if mono" + (n.ifError ? " invalid" : ""), value: n.if, spellcheck: false, autocomplete: "off",
      placeholder: "type == bill && category == utility", title: "&&, ||, !, (…); ==, !=, in […], ~ (contains), has(key)" });
    const why = el("span", { className: "message error", textContent: n.ifError });
    cond.oninput = () => { n.if = cond.value; changed(); checkIf(cond, why, n); };
    const bubbles = isElse ? null : condition(cond, { keys: condKeys, values: condValues });
    bubbles?.read();
    const out = n.exclude;
    const numbering = Object.keys(n.order).length > 0;
    const idle = !out && !n.path && !n.file && !n.children.length && !numbering && n.default == null;
    const small = (text, title, onclick) => el("button", { type: "button", className: "path-more block-more", textContent: text, title, onclick });
    const body = out
      ? [el("p", { className: "muted block-note", textContent: "What it takes is left out of the tree." }),
        small("Place them", "Place what it takes, as the blocks above do", () => { n.exclude = false; redraw(); })]
      : [
        n.path || n.showPath ? field(n, "path", "folders", "Folders after the ones above")
          : small("+ folders", "Folders after the ones above; without, only the file name differs", () => { n.showPath = true; redraw(); }),
        field(n, "file", "file", "The file name, in place of the one above; empty keeps it"),
        numbering ? el("div", { className: "block-order" },
          ...Object.keys(n.order).map((key) => orderList(n, key, restOf(key, n))),
          small("Number as above", "Drop this block's own numbering: its Items are numbered as the blocks above number theirs", () => {
            n.order = {}; n.numbers = {}; n.unnumbered = {}; n.unlisted = {}; redraw(); }))
          : small("+ numbering", "Number what this block takes on its own, starting from the numbering above", () => { ownNumbering(n, up); redraw(); }),
        idle ? el("p", { className: "muted block-note", textContent: isElse ? "Give it folders or a file name." : "Changes nothing yet: give it folders, a file or numbering, or leave what it takes out." }) : null,
        isElse ? null : small("Leave out", "Leave what it takes out of the tree", () => {
          Object.assign(n, { exclude: true, path: "", file: "", children: [], order: {}, numbers: {}, unnumbered: {}, unlisted: {}, default: undefined, ofs: [] }); redraw(); }),
        n.error ? el("span", { className: "message error", textContent: n.error }) : null,
        ...blocks(n.children, [...up, n]), adder(n.children)];
    const block = el("div", { className: "block" + (isElse ? " else" : "") + (out ? " out" : "") },
      el("div", { className: "block-head" },
        el("span", { className: "block-word", textContent: isElse ? "else" : i ? "else if" : "if" }),
        el("span", { className: "order-acts block-acts" },
          icon("Up: tried before the one above", "↑", () => { [list[i - 1], list[i]] = [list[i], list[i - 1]]; redraw(); }, i === 0 || isElse),
          icon("Down", "↓", () => { [list[i + 1], list[i]] = [list[i], list[i + 1]]; redraw(); }, i >= list.length - 1 || !list[i + 1].if),
          icon("Remove, with the blocks inside it", "×", () => { list.splice(i, 1); redraw(); drawOrder(); })),
        isElse ? null : bubbles.el),
      why,
      el("div", { className: "block-body" }, ...body));
    // Dragged by its head, a block drops before the one it is let go on,
    // in that one's list; never into itself, and never after an else.
    const head = block.firstChild;
    head.draggable = true;
    head.addEventListener("dragstart", (event) => {
      if (event.target !== head) return;
      event.stopPropagation(); draggingBlock = { list, n }; event.dataTransfer.effectAllowed = "move"; block.classList.add("dragging");
    });
    head.addEventListener("dragend", () => block.classList.remove("dragging"));
    const fits = () => draggingBlock && draggingBlock.n !== n && !contains(draggingBlock.n, list) && (draggingBlock.n.if || list === draggingBlock.list && i === list.length - 1);
    head.addEventListener("dragover", (event) => { if (!fits()) return; event.preventDefault(); event.stopPropagation(); head.classList.add("drop"); });
    head.addEventListener("dragleave", () => head.classList.remove("drop"));
    head.addEventListener("drop", (event) => {
      head.classList.remove("drop");
      if (!fits()) return;
      event.preventDefault(); event.stopPropagation();
      const from = draggingBlock; draggingBlock = null;
      from.list.splice(from.list.indexOf(from.n), 1);
      list.splice(list.indexOf(n), 0, from.n);
      redraw();
    });
    return block;
  });
  box.replaceChildren(...blocks(nodes), adder(nodes));
}
// The path editor is a dialog: the rows and key chips build the rule's
// file, or a block's folders or file, whose rows stand in for the file's
// until Done or Cancel puts them back.
let editingPath = null; // the block text being edited, { text, set }; null for the rule's file
let stash = null; // the rows and active part before the dialog opened
async function openEditor(p) {
  stash = { rows: JSON.parse(JSON.stringify(rows)), active: { ...active } };
  editingPath = p || null;
  if (p) {
    try {
      rows = p.text.trim() ? fromLayout((await post("/api/rules/layout", { layout: p.text.trim() })).layout) : [{ parts: [] }];
    } catch (err) {
      stash = null;
      editingPath = null;
      alert(err.message);
      return;
    }
    active = { row: rows.length - 1, group: -1 };
  }
  drawPath();
  $("path-dialog").showModal();
}
function closeEditor(keep) {
  if (!stash) return;
  const p = editingPath;
  const text = layoutText();
  if (p || !keep) { rows = stash.rows; active = stash.active; }
  stash = null;
  editingPath = null;
  if ($("path-dialog").open) $("path-dialog").close();
  if (p && keep) { p.set(text); changed(); }
  edited();
}
$("path-edit")?.addEventListener("click", () => openEditor(null));
$("path-done")?.addEventListener("click", () => closeEditor(true));
$("path-cancel")?.addEventListener("click", () => closeEditor(false));
$("path-dialog")?.addEventListener("close", () => closeEditor(false));
// read is the form's rule, less its name and what the page owns.
export function read() {
  const clean = (n) => {
    const out = {};
    if (n.if.trim()) out.if = n.if.trim();
    if (n.path.trim()) out.path = n.path.trim();
    if (n.file.trim()) out.file = n.file.trim();
    if (n.default !== undefined && n.default !== null) out.default = n.default;
    if (n.children.length) out.children = n.children.map(clean);
    if (n.exclude) out.exclude = true;
    return Object.assign(out, numberingOf(n, Object.keys(n.order)));
  };
  // The box shows the file as the rows make it, and as typed until the
  // server has read it.
  const out = { selection: document.querySelector("input[name=selection]:checked").value, file: $("layout-text").value.trim() || layoutText() };
  if ($("if").value.trim()) out.if = $("if").value.trim();
  if ($("root-path").value.trim()) out.path = $("root-path").value.trim();
  if (nodes.length) out.children = nodes.map(clean);
  if (inherited().length) out.inherit = inherited();
  if ($("shared")?.checked) out.shared = true;
  // While a path is still being parsed its orders are not known yet: keep
  // every order until it is.
  return Object.assign(out, numberingOf(top, parsing ? Object.keys(top.order) : numberedKeys().map((c) => c.of)));
}

// numberingOf is the order, numbers and unnumbered s saves for the orders
// named: an empty order is not kept, nor a number or unnumbered name no
// longer in its order.
function numberingOf(s, names) {
  const out = {};
  const order = {};
  for (const key of names) {
    const list = (s.order[key] || []).filter(Boolean);
    if (list.length) order[key] = list;
  }
  if (Object.keys(order).length) out.order = order;
  const set = {};
  for (const [key, list] of Object.entries(order)) {
    const kept = Object.fromEntries(Object.entries(s.numbers[key] || {}).filter(([name]) => list.some((v) => same(v, name))));
    if (Object.keys(kept).length) set[key] = kept;
  }
  if (Object.keys(set).length) out.numbers = set;
  const skipped = {};
  for (const [key, list] of Object.entries(order)) {
    const kept = (s.unnumbered[key] || []).filter((name) => list.some((v) => same(v, name)));
    if (kept.length) skipped[key] = kept;
  }
  if (Object.keys(skipped).length) out.unnumbered = skipped;
  const unlisted = Object.fromEntries(Object.entries(s.unlisted || {}).filter(([key, v]) => order[key] && v !== ""));
  if (Object.keys(unlisted).length) out.unlisted = unlisted;
  return out;
}

// nodeAt is the child at, as the server names it: 2.1 is the second
// child's first; "" is the rule's own numbering.
const nodeAt = (at) => at ? at.split(".").reduce((n, i) => n && n.children[i - 1], { children: nodes }) : top;

// numberLast puts value last in key's order, in the node at.
export function numberLast(key, value, at = "") {
  const s = nodeAt(at) || top;
  s.order[key] = [...(s.order[key] || []), value];
  drawNodes();
  drawOrder();
  changed();
}

// ownNumbering gives n its own copy of the orders it numbers, or of every
// order of the rule when its own path and file number none, each as the
// nearest block above numbers it.
function ownNumbering(n, up) {
  const names = n.ofs.length ? n.ofs.map((c) => c.of) : numberedKeys().map((c) => c.of);
  for (const key of new Set(names)) {
    const from = [...up].reverse().find((s) => (s.order[key] || []).length) || top;
    n.order[key] = [...(from.order[key] || [])];
    if (from.numbers[key]) n.numbers[key] = { ...from.numbers[key] };
    if (from.unnumbered[key]) n.unnumbered[key] = [...from.unnumbered[key]];
    if ((from.unlisted || {})[key] !== undefined) n.unlisted[key] = from.unlisted[key];
  }
}

// restOf is what the order named numbers, as written: from n's own path
// and file, or else the rule's.
const restOf = (key, n) => ((n && n.ofs.find((c) => c.of === key)) || numberedKeys().find((c) => c.of === key) || { rest: "" }).rest;

// same reports whether a and b are one value: one country however each is
// written, or else equal ignoring case.
const same = (a, b) => (countries[a] && countries[a] === countries[b]) || a.toLowerCase() === b.toLowerCase();

// chosenTypes is every type: a rule's if says which it takes.
const chosenTypes = () => state.templates.map((t) => t.type);

// selected is the Items the rule takes, as the server last planned it, or
// every Item before it has.
// Until the server has planned the rule no Item is known to be taken:
// none is listed, rather than every Item in the tree.
// A block lists only the Items it or a block below it places.
const selected = (s = top) => {
  if (!previewed) return [];
  const at = s === top ? null : whereOf(s);
  return state.items.filter((item) => previewed.has(item.id)
    && (at === null || [...previewed.get(item.id)].some((n) => n === at || n.startsWith(at + "."))));
};
// nameOwn reports whether the Item is named by the rule's own file at
// one of the places it lands: no block on the way there writes its own.
// An order the rule's file numbers lists only those Items; the others
// never print that number.
const nameOwn = (item) => [...previewed.get(item.id)].some((at) => {
  let list = nodes;
  for (const i of at ? at.split(".") : []) {
    const n = list[i - 1];
    if (!n || n.file.trim()) return false;
    list = n.children;
  }
  return true;
});
// whereOf is a block's place as the server names it: 2.1 is the second
// child's first.
const whereOf = (s) => {
  const walk = (list, prefix) => {
    for (const [i, n] of list.entries()) {
      const at = prefix + (i + 1);
      if (n === s) return at;
      const below = walk(n.children, at + ".");
      if (below) return below;
    }
    return "";
  };
  return walk(nodes, "");
};

// What each key a layout can use writes. A Template's own field is named
// with the types that have it.
const BUILT_IN = {
  type: "the Template's type, such as id_card",
  kind: "document or record",
  id: "the Item's ID",
  revision: "the revision's number, from 1",
  ext: "the extension: pdf",
  year: "the year of issued_at",
  month: "the month of issued_at",
  date: "issued_at, YYYY-MM-DD",
};
const FORMATS = { zh: "中国", en: "China", alpha2: "CN", alpha3: "CHN" };
// A type is written in its Template's names.
const TYPE_FORMATS = { zh: "the Chinese name, such as 驾驶证", en: "the English name, such as Driver licence" };

const countryKeys = () => new Set(state.templates.flatMap((t) => t.fields.filter((f) => f.type === "country").map((f) => f.key)));

// linkFields is the fields of the chosen types' Templates that link to
// another Item: an Item's keys can be taken from the Item they name.
const linkFields = () => {
  const types = chosenTypes();
  return [...new Set(state.templates.filter((t) => types.includes(t.type))
    .flatMap((t) => t.fields.filter((f) => f.type === "item" || f.type === "revision").map((f) => f.key)))];
};

function describe(key) {
  if (BUILT_IN[key]) return BUILT_IN[key];
  const types = state.templates.filter((t) => t.fields.some((f) => f.key === key)).map((t) => t.type);
  return (countryKeys().has(key) ? "a country, as the Item keeps it" : "a field") + (types.length ? " of " + types.join(", ") : "");
}

// The path is edited part by part, never as text. A row is a folder or,
// last, the file's name: { parts, optional }. An optional row is a folder
// only an Item with every key in it gets, written [/…] after the row
// before. A part is fixed text { text }, a key and its alternatives
// { keys: ["name", "type:zh"] }, the number {#} { counter: true }, or an
// optional group { group: [parts] }, written only when the Item has every
// key in it. The server parses a saved layout into these; the page writes
// them back as the layout it saves.
let rows = [];
let active = { row: 0, group: -1 }; // where a chip adds: a row, or a group in it
const DEFAULT_ROWS = () => [{ parts: [{ keys: ["owner"] }] }, { parts: [{ keys: ["type"] }, { text: "." }, { keys: ["ext"] }] }];

// fromLayout makes rows of a layout the server parsed: a folder group
// ending a name becomes an optional row of its own.
function fromLayout(layout) {
  const part = (p) => p.counter ? { counter: true, label: p.label } : p.end ? { end: true } : p.group ? { group: p.group.map(part) }
    : p.key ? { keys: [p, ...(p.or || [])].map((c) => c.key + (c.format ? ":" + c.format : "")) } : { text: p.text };
  const out = [];
  for (const segment of layout) {
    const last = segment[segment.length - 1];
    if (last && last.folder) {
      out.push({ parts: segment.slice(0, -1).map(part) }, { parts: last.group.map(part), optional: true });
    } else {
      out.push({ parts: segment.map(part) });
    }
  }
  return out;
}

const written = (p) => p.counter ? "{#" + (p.label || "") + "}" : p.end ? "{/#}" : p.group ? "[" + p.group.map(written).join("") + "]" : p.keys ? "{" + p.keys.join("|") + "}" : p.text;

// layoutText is the rows as the layout the rule saves.
export const layoutText = () => rows.map((row, i) => {
  const inner = row.parts.map(written).join("");
  return row.optional ? "[/" + inner + "]" : (i ? "/" : "") + inner;
}).join("");

// The container a chip adds to: the active row's parts, or a group's.
function target() {
  const row = rows[Math.min(active.row, rows.length - 1)];
  const group = row.parts[active.group];
  return group && group.group ? group.group : row.parts;
}
const inGroup = () => active.group >= 0 && !!rows[active.row]?.parts[active.group]?.group;

function edited() {
  drawPath();
  drawOrder();
  changed();
}

// add puts a part last in the container chips add to; in the file's name,
// before its .{ext}.
function add(part) {
  const list = target();
  let at = list.length;
  if (list === rows[rows.length - 1].parts && list[at - 1]?.keys?.[0] === "ext") at -= list[at - 2]?.text === "." ? 2 : 1;
  list.splice(at, 0, part);
  edited();
}

// heldBy is the field keys the Templates of types have.
const heldBy = (types) => new Set(state.templates.filter((t) => types.includes(t.type)).flatMap((t) => t.fields.map((f) => f.key)));

// linkedTypes is the types the Item a link field names can be: the one its
// match names, such as {type: =tenancy}, or else every type.
function linkedTypes(link) {
  const all = state.templates.map((t) => t.type);
  const f = state.templates.flatMap((t) => t.fields).find((x) => x.key === link);
  const by = f && f.match && f.match.type;
  // A match naming its type itself, {type: =tenancy}, offers that type's keys.
  return by && by.startsWith("=") ? all.filter((t) => t === by.slice(1)) : all;
}

// linkKeys is the keys offered from the Item a link field names.
function linkKeys(link) {
  const held = heldBy(linkedTypes(link));
  return [...keys.filter((k) => !BUILT_IN[k] && k !== link && held.has(k)), "type"];
}

// The keys a part may be, for its menus: the chosen types' fields and the
// keys every PDF has, each country field and type in each form, and the
// keys of the Items link fields name, original.level.
function keyOptions() {
  const held = heldBy(chosenTypes());
  const plain = keys.filter((k) => k !== "type" && (BUILT_IN[k] || held.has(k)));
  return [
    ["Keys", [...plain, "type", "type:zh", "type:en"]],
    // A select field whose values are named in a language: {category:en}.
    ["Named values", [...new Set(state.templates.flatMap((t) => t.fields.filter((f) => held.has(f.key) && f.names).flatMap((f) => Object.keys(f.names).map((l) => f.key + ":" + l))))]],
    ["Countries", [...countryKeys()].filter((k) => held.has(k)).flatMap((k) => Object.keys(FORMATS).map((f) => k + ":" + f))],
    ...linkFields().map((l) => ["From " + l, [...linkKeys(l), "type:zh", "type:en", "year", "month", "date"].map((k) => l + "." + k)]),
  ];
}

function keyPart(part, remove) {
  const box = el("span", { className: "path-key", title: "A key. Several are alternatives: the first the Item has is written." });
  const draw = () => {
    const options = keyOptions();
    box.replaceChildren(...part.keys.flatMap((k, i) => {
      const known = options.some(([, list]) => list.includes(k));
      // A click stays in the menu: the row it would reach redraws.
      const pick = el("select", { onclick: (event) => event.stopPropagation(), onmousedown: (event) => event.stopPropagation(), onchange: () => { part.keys[i] = pick.value; edited(); } },
        ...(known ? [] : [el("option", { value: k, selected: true }, k)]),
        ...options.filter(([, list]) => list.length).map(([name, list]) => el("optgroup", { label: name },
          ...list.map((o) => el("option", { value: o, selected: o === k, title: describe(o.split(/[:.]/)[0]) }, o)))));
      const drop = part.keys.length > 1 ? el("button", { type: "button", className: "path-x", title: "Remove this alternative", textContent: "×",
        onclick: (event) => { event.stopPropagation(); part.keys.splice(i, 1); edited(); } }) : null;
      return [i ? el("span", { className: "path-or", textContent: "or" }) : null, pick, drop].filter(Boolean);
    }),
    el("button", { type: "button", className: "path-more", title: "Add an alternative, written when the Item lacks the keys before it", textContent: "+ or",
      onclick: (event) => { event.stopPropagation(); part.keys.push(part.keys[part.keys.length - 1]); edited(); } }),
    el("button", { type: "button", className: "path-x", title: "Remove the key", textContent: "×", onclick: (event) => { event.stopPropagation(); remove(); } }));
  };
  draw();
  return box;
}

// fit sizes a text input to its text: a wide character, 中, takes two
// columns of a narrow one.
function fit(input) {
  const cols = [...input.value].reduce((n, c) => n + (/[\u1100-\uffff]/.test(c) && !/[\uff61-\uffdc]/.test(c) ? 2 : 1), 0);
  input.style.width = `calc(${Math.max(1, cols)}ch + 20px)`;
}

// partEl draws one part of a container; the container's own list is where
// it moves and is removed from.
function partEl(part, list, r, g) {
  const i = list.indexOf(part);
  const remove = () => { list.splice(list.indexOf(part), 1); if (part.group) active = { row: r, group: -1 }; edited(); };
  let node;
  if (part.counter) {
    node = el("span", { className: "path-counter", title: "Numbers the rest of this name, 01, 02, in the order below" }, "#",
      el("button", { type: "button", className: "path-x", title: "Remove the number", textContent: "×", onclick: (event) => { event.stopPropagation(); remove(); } }));
  } else if (part.end) {
    node = el("span", { className: "path-counter", title: "The number stops here: what follows is written but not numbered" }, "/#",
      el("button", { type: "button", className: "path-x", title: "Remove the stop", textContent: "×", onclick: (event) => { event.stopPropagation(); remove(); } }));
  } else if (part.keys) {
    node = keyPart(part, remove);
  } else if (part.group) {
    const at = rows[r].parts.indexOf(part);
    node = el("span", { className: "path-group" + (active.row === r && active.group === at ? " active" : ""),
      title: "Optional: written only when the Item has every key in it; otherwise nothing, its text included",
      onclick: (event) => { event.stopPropagation(); active = { row: r, group: at }; drawPath(); } },
      el("span", { className: "path-group-name", textContent: "if any" }),
      ...(part.group.length ? part.group.map((p) => partEl(p, part.group, r, at)) : [el("span", { className: "muted", textContent: "click a key below" })]),
      el("button", { type: "button", className: "path-x", title: "Remove the optional part", textContent: "×", onclick: (event) => { event.stopPropagation(); remove(); } }));
  } else {
    // Text is typed in place; what would start a key, group or folder is
    // left out, and emptied text goes. An optional folder's own text may
    // hold /: bill/{service} adds both folders or neither.
    const slash = rows[r]?.optional && list === rows[r].parts;
    node = el("input", { className: "path-text", value: part.text,  spellcheck: false, autocomplete: "off",
      title: slash ? "Fixed text; / adds a folder" : "Fixed text", onclick: (event) => event.stopPropagation(),
      oninput: (event) => {
        part.text = event.target.value.replace(slash ? /[{}[\]\\]/g : /[{}[\]/\\]/g, "");
        event.target.value = part.text;
        fit(event.target);
        showText();
        drawOrder();
        changed();
      },
      onchange: () => { if (!part.text) remove(); } });
    fit(node);
  }
  // A part is dragged to another place in its own container.
  node.draggable = !(part.text !== undefined);
  node.dataset.row = r;
  node.dataset.group = g;
  node.addEventListener("dragstart", (event) => { event.stopPropagation(); dragging = { list, part }; event.dataTransfer.effectAllowed = "move"; });
  node.addEventListener("dragover", (event) => { if (dragging && dragging.list === list) { event.preventDefault(); event.stopPropagation(); node.classList.add("drop"); } });
  node.addEventListener("dragleave", () => node.classList.remove("drop"));
  node.addEventListener("drop", (event) => {
    node.classList.remove("drop");
    if (!dragging || dragging.list !== list) return;
    event.preventDefault();
    event.stopPropagation();
    const from = list.indexOf(dragging.part);
    list.splice(from, 1);
    list.splice(list.indexOf(part) + (from <= i ? 1 : 0), 0, dragging.part);
    dragging = null;
    edited();
  });
  return node;
}
let dragging = null;

// showText writes the layout the rows make into its box, unless it is
// being typed in.
function showText() {
  if ($("path-dialog-text")) $("path-dialog-text").textContent = layoutText();
  if (editingPath) return;
  const box = $("layout-text");
  if (document.activeElement === box) return;
  box.value = layoutText();
  box.classList.remove("invalid");
  $("layout-error").textContent = "";
}

// typed takes the layout typed in its box: the server parses it and the
// rows are drawn from it; one it refuses stays, marked, with the reason.
let typing = 0;
async function typed() {
  const box = $("layout-text"), mine = ++typing;
  parsing++;
  try {
    const answer = await post("/api/rules/layout", { layout: box.value.trim() });
    if (mine !== typing) return;
    rows = fromLayout(answer.layout);
    active = { row: rows.length - 1, group: -1 };
    box.classList.remove("invalid");
    $("layout-error").textContent = "";
    edited();
  } catch (err) {
    if (mine !== typing) return;
    box.classList.add("invalid");
    $("layout-error").textContent = err.message;
  } finally {
    parsing--;
  }
  changed();
}
// settle takes a layout typed and not yet taken, as Save is pressed; one
// the server refuses stops the save.
export async function settle() {
  const bad = [root, ...every(nodes)].find((n) => n.error || n.ifError);
  if (bad) throw new Error(bad.error || bad.ifError);
  if ($("if-error").textContent) throw new Error($("if-error").textContent);
  const box = $("layout-text");
  if (box.value.trim() === layoutText()) return;
  await typed();
  if (box.classList.contains("invalid")) throw new Error($("layout-error").textContent);
}
$("layout-text").addEventListener("change", () => typed());
$("layout-text").addEventListener("keydown", (event) => { if (event.key === "Enter") { event.preventDefault(); $("layout-text").blur(); } });

// drawPath draws the rows, the one chips add to marked, and the layout
// they make, which is shown and never typed.
function drawPath() {
  if (active.row >= rows.length) active = { row: rows.length - 1, group: -1 };
  const last = rows.length - 1;
  const act = (title, text, onclick, disabled) => el("button", { type: "button", className: "order-act", title, textContent: text, disabled,
    onclick: (event) => { event.stopPropagation(); onclick(); } });
  $("path").replaceChildren(...rows.map((row, r) => {
    const file = r === last;
    const optionalAllowed = r > 0 && !file && !rows[r - 1].optional;
    // Each row sits under the one before, indented a step with a line
    // down from its parent, as the tree is drawn.
    const node = el("div", { className: "path-row" + (row.optional ? " optional" : "") + (active.row === r && active.group < 0 ? " active" : ""),
      onclick: () => { active = { row: r, group: -1 }; drawPath(); } },
      el("span", { className: "path-kind", textContent: file ? "File" : row.optional ? "If any" : "Folder",
        title: file ? "The PDF's name" : row.optional ? "A folder only an Item with every key in it gets" : "A folder" }),
      el("div", { className: "path-parts" }, ...(row.parts.length ? row.parts.map((p) => partEl(p, row.parts, r, -1)) : [el("span", { className: "muted", textContent: "click a key below" })])),
      el("span", { className: "path-acts" },
        file ? null : el("label", { className: "path-optional", title: "Only an Item with every key in this folder gets it; the others stay in the folder before",
          onclick: (event) => event.stopPropagation() },
          el("input", { type: "checkbox", checked: !!row.optional, disabled: !optionalAllowed && !row.optional,
            onchange: (event) => { row.optional = event.target.checked; edited(); } }), " optional"),
        file ? null : act("Up", "↑", () => { [rows[r - 1], rows[r]] = [rows[r], rows[r - 1]]; active = { row: r - 1, group: -1 }; edited(); }, r === 0),
        file ? null : act("Down", "↓", () => { [rows[r + 1], rows[r]] = [rows[r], rows[r + 1]]; active = { row: r + 1, group: -1 }; edited(); }, r + 1 >= last),
        file ? null : act("Remove the folder", "×", () => { rows.splice(r, 1); active = { row: Math.min(r, rows.length - 1), group: -1 }; edited(); })));
    const level = el("div", { className: "path-level" }, node);
    level.style.setProperty("--depth", r);
    return level;
  }), el("button", { type: "button", className: "button path-add", textContent: "+ Folder", title: "Add a folder before the PDF's name",
    onclick: () => { rows.splice(rows.length - 1, 0, { parts: [] }); active = { row: rows.length - 2, group: -1 }; edited(); } }));
  showText();
  chips();
}

// The keys as chips that add to the row or optional part chosen: fields,
// the ones every PDF has, text, a number and an optional part, and each
// country field in each form it can be written in.
function chips() {
  const chip = (text, part, title, disabled) => el("button", {
    type: "button", className: "chip", textContent: text, title, disabled,
    onclick: () => {
      const made = part();
      if (made.counter) {
        // The number starts its name, 01-.
        target().unshift(made, { text: "-" });
        return edited();
      }
      add(made);
      if (made.group) { active = { row: active.row, group: rows[active.row].parts.indexOf(made) }; drawPath(); }
      if (made.text !== undefined) {
        // The new text is focused to type into: the only empty one.
        [...$("path").querySelectorAll(".path-text")].find((input) => input.value === "")?.focus();
      }
    },
  });
  const group = (name, ...children) => children.length ? el("div", { className: "key-group" },
    el("span", { className: "key-group-name" }, name), el("div", { className: "key-chips" }, ...children)) : null;
  const types = chosenTypes();
  const held = heldBy(types);
  const fields = keys.filter((k) => !BUILT_IN[k] && held.has(k));
  const countries = [...countryKeys()].filter((k) => held.has(k));
  const numbered = target().some((p) => p.counter);
  const stopped = target().some((p) => p.end);
  const key = (k) => () => ({ keys: [k] });
  $("keys").replaceChildren(...[
    group("Fields", ...fields.map((k) => chip(k, key(k), describe(k)))),
    group("Every PDF", ...keys.filter((k) => BUILT_IN[k]).map((k) => chip(k, key(k), describe(k)))),
    group("Add", chip("text", () => ({ text: "" }), "Fixed text, such as - or 03-Education"),
      chip("#", () => ({ counter: true }), inGroup() ? "A number goes only in a row, or in an optional folder" : "Numbers the rest of this name, 01-, 02-, in the order below", numbered || inGroup()),
      chip("/#", () => ({ end: true }), "Stops the number: what follows, such as a date, is written but not numbered", !numbered || stopped || inGroup()),
      chip("if any […]", () => ({ group: [] }), "An optional part, written only when the Item has every key in it: -本科 or nothing", inGroup())),
    ...countries.map((k) => group(k + " as", ...Object.entries(FORMATS).map(([f, example]) =>
      chip(":" + f, key(k + ":" + f), `{${k}:${f}} writes ${example}`)))),
    group("type as", ...Object.entries(TYPE_FORMATS).map(([f, note]) => chip(":" + f, key("type:" + f), note))),
    ...linkFields().map((l) => group("from " + l, ...[...linkKeys(l), "type:zh", "type:en"].map((k) => chip(k, key(l + "." + k), "the " + k + " of the Item " + l + " links to")))),
  ].filter(Boolean));
}

// drawInherit offers each link field of the rule's types, ticked when the
// rule takes the keys an Item lacks from the Item it names.
function drawInherit(saved) {
  const all = [...new Set([...linkFields(), ...saved])];
  $("inherit-row").hidden = !all.length;
  $("inherit").replaceChildren(...all.map((l) => el("label", { title: "A key an Item lacks is taken from the Item its " + l + " links to" },
    el("input", { type: "checkbox", value: l, checked: saved.includes(l), onchange: () => { drawOrder(); changed(); } }), " keys it lacks, from its " + l)));
}
const inherited = () => [...$("inherit").querySelectorAll("input:checked")].map((i) => i.value);

// The orders the rule numbers from, each once, as { of, rest }: a row with
// {#} numbers what follows it up to {/#}, less the text straight after {#}
// and a trailing .{ext}, the rest; its order is named for the rest's first
// key, or as {#label} names it.
const firstKey = (parts) => parts.map((p) => p.keys ? p.keys[0].split(":")[0] : p.group ? firstKey(p.group) : "").find(Boolean) || "";
export const numberedKeys = () => {
  const all = [...rows.flatMap((row, r) => {
    const at = row.parts.findIndex((p) => p.counter);
    if (at < 0) return [];
    let rest = row.parts.slice(at + 1);
    const end = rest.findIndex((p) => p.end);
    if (end >= 0) rest = rest.slice(0, end);
    if (rest.length && rest[0].text !== undefined) rest = rest.slice(1);
    if (rest.length && rest[rest.length - 1].keys && rest[rest.length - 1].keys[0] === "ext") {
      rest = rest.slice(0, -1);
      if (rest.length && rest[rest.length - 1].text === ".") rest = rest.slice(0, -1);
    }
    return rest.some((p) => p.keys || p.group) ? [{ of: row.parts[at].label || firstKey(rest), rest: rest.map(written).join(""), file: r === rows.length - 1 }] : [];
  }), ...[root, ...every(nodes)].flatMap((n) => n.ofs)];
  return all.filter((c, i) => all.findIndex((d) => d.of === c.of) === i);
};

// An Item's value for one key, {a|b} or {a|b:format}: the first alternative
// it has, or with inherit the first the Items it links to have. A type:zh
// or type:en is its Template's name.
function keyValue(item, inner) {
  for (const choice of inner.split("|")) {
    const value = choiceValue(item, choice) || (choice.includes(".") ? "" : inherited().map((l) => choiceValue(item, l + "." + choice)).find(Boolean));
    if (value) return value;
  }
  return "";
}

function choiceValue(item, choice) {
  const [name, format] = choice.split(":");
  if (name.includes(".")) {
    // original.level: the key of the revision the field original links to.
    const [field, key] = name.split(".");
    const [id, ref] = String((item.fields || {})[field] || "").split("@");
    const other = state.items.find((i) => i.id === id);
    const rev = other && (ref ? other.revisions.find((r) => (r.id || r.digest) === ref) : null);
    const fields = !other ? {} : !rev ? other.fields : rev.snapshot ? rev.fields || {} : { ...other.fields, ...(rev.fields || {}) };
    // original.type:zh is the linked Item's type in a language.
    if (key === "type") return format && other ? (state.templates.find((t) => t.type === other.type)?.names || {})[format] : other?.type;
    return fields[key];
  }
  return name === "type"
    ? (format ? (state.templates.find((t) => t.type === item.type)?.names || {})[format] : item.type)
    : item.fields && item.fields[name];
}

// An Item's name in one order: the rest its {#} numbers, its {key}s and
// [groups] filled in. A key it lacks makes no name; a group it lacks a
// key of is left out.
export function orderValue(item, key) {
  let whole = true;
  const fill = (text) => {
    let ok = true;
    const out = text.replace(/\{([^{}]*)\}/g, (_, inner) => { const v = keyValue(item, inner); if (!v) ok = false; return v || ""; });
    return [out, ok];
  };
  const name = key.replace(/\[([^\]]*)\]|\{([^{}]*)\}/g, (_, group, inner) => {
    if (group !== undefined) { const [out, ok] = fill(group); return ok ? out : ""; }
    const v = keyValue(item, inner);
    if (!v) whole = false;
    return v || "";
  });
  return whole ? name : "";
}

// One row per order: the values it may have, in the order they are numbered
// from 01. An order new to the layout starts with the values Items have.
// numbered is the number each name of an order gets: the next after the
// one before, unless set gives it its own, from which the rest count on.
// A name skip lists is null and takes no number.
function numbered(values, set = {}, skip = []) {
  let n = 0;
  return values.map((v) => {
    if (skip.some((name) => same(name, v))) return null;
    const own = Object.entries(set).find(([name]) => same(name, v));
    n = own ? own[1] : n + 1;
    return n;
  });
}

function drawOrder() {
  const lists = numberedKeys().map(({ of, rest }) => orderList(top, of, rest));
  $("order").replaceChildren(...lists);
  $("order-row").hidden = !lists.length;
}

// unlistedPick is what s's order key gives a value it does not list: no
// place until it is listed, no number, or one number shared by them all.
function unlistedPick(s, key) {
  s.unlisted ||= {};
  const now = s.unlisted[key] ?? "";
  let mode = now === "" ? "" : now === "unnumbered" ? "unnumbered" : "number";
  const number = el("input", { type: "text", inputMode: "numeric", className: "order-unlisted-number",
    value: mode === "number" ? now : "99", "aria-label": "Their number" });
  const save = () => {
    number.hidden = mode !== "number";
    if (mode === "number") {
      const n = parseInt(number.value, 10);
      number.classList.toggle("invalid", !(n >= 0));
      if (!(n >= 0)) return;
      s.unlisted[key] = String(n);
    } else if (mode) s.unlisted[key] = mode;
    else delete s.unlisted[key];
    for (const chip of chips.children) chip.setAttribute("aria-pressed", String(chip.value === mode));
    changed();
  };
  const chip = (value, text, title) => el("button", { type: "button", className: "chip", value, textContent: text, title,
    onclick: () => { mode = value; save(); if (value === "number") number.focus(); } });
  const chips = el("div", { className: "segmented", role: "group", "aria-label": "Values not in the list" },
    chip("", "Not placed", "Missing until added to the list"),
    chip("unnumbered", "No number", "Placed without a number, nor the text straight after {#}"),
    chip("number", "One number", "Placed with one number they all share"));
  for (const c of chips.children) c.setAttribute("aria-pressed", String(c.value === mode));
  number.hidden = mode !== "number";
  number.onchange = save;
  return el("div", { className: "order-unlisted" }, el("span", { textContent: "Not in the list" }), chips, number);
}

// orderList draws the order s keeps for key, rest what it numbers as
// written. An order new to s starts with the names the Items have.
function orderList(s, key, rest) {
  const fileKey = s === top && numberedKeys().some((c) => c.of === key && c.file);
  const takes = () => fileKey ? selected(s).filter(nameOwn) : selected(s);
  const numbers = s.numbers, unnumbered = s.unnumbered;
  if (s.order[key] === undefined && previewed) {
    s.order[key] = [...new Set(takes().map((i) => orderValue(i, rest)).filter(Boolean))].sort();
  }
  const box = el("div", { className: "order" });
  const list = () => (s.order[key] || []).filter(Boolean);
  const set = (values) => { s.order[key] = [...values]; draw(); changed(); };
  let dragged = -1;
  const draw = () => {
    const values = list();
    // Every value in the order is listed with its number; one no Item the
    // query picks has is dimmed, but still holds its number, so a number
    // refused is seen taken.
    const held = [...new Set(takes().map((i) => orderValue(i, rest)).filter(Boolean))];
    const idle = (v) => !held.some((h) => same(h, v));
    // holders is the Items this block takes whose value for key is v.
    const holders = (v) => takes().filter((i) => same(orderValue(i, rest), v));
    const unlisted = held.filter((v) => !values.some((w) => same(v, w))).sort();
    // move swaps a value with the one before or after it.
    const move = (a, b) => { const next = [...values]; [next[a], next[b]] = [next[b], next[a]]; set(next); };
    const act = (title, text, onclick, disabled) => el("button", { type: "button", className: "order-act", title, textContent: text, disabled, onclick });
    const counted = numbered(values, numbers[key], unnumbered[key]);
    const pad = Math.max(2, String(Math.max(0, ...counted)).length);
    // before is the number of the last numbered name before i.
    const before = (i) => counted.slice(0, i).filter((n) => n !== null).pop();
    const skipped = (v) => (unnumbered[key] || []).some((name) => same(name, v));
    // toggle leaves a name unnumbered, or numbers it again.
    const toggle = (v) => {
      unnumbered[key] = skipped(v) ? unnumbered[key].filter((name) => !same(name, v)) : [...(unnumbered[key] || []), v];
      numbers[key] = Object.fromEntries(Object.entries(numbers[key] || {}).filter(([name]) => !same(name, v)));
      draw();
      changed();
    };
    // renumber sets a name's number by hand, or clears it when it is the
    // next one anyway. It may repeat the one before, so two names, a
    // tenancy agreement and its translation, share a number; a number
    // before it is refused.
    const renumber = (i, input) => {
      const own = { ...(numbers[key] || {}) };
      for (const name of Object.keys(own)) if (same(name, values[i])) delete own[name];
      const typed = parseInt(input.value, 10);
      const next = numbered(values.slice(0, i + 1), own, unnumbered[key])[i];
      if (input.value.trim() !== "" && typed !== next) {
        if (!(typed >= 0) || (before(i) !== undefined && typed < before(i))) {
          const at = counted.slice(0, i).lastIndexOf(before(i));
          input.value = String(counted[i]).padStart(pad, "0");
          input.classList.add("invalid");
          input.title = before(i) === undefined ? "A number is 0 or more." : "Not before " + String(before(i)).padStart(pad, "0") + ", which " + values[at] + " holds: type it to share that number.";
          return;
        }
        own[values[i]] = typed;
      }
      numbers[key] = own;
      draw();
      changed();
    };
    box.replaceChildren(...[
      el("ol", { className: "order-list" }, ...values.map((v, i) => {
        const li = el("li", {
          className: idle(v) ? "idle" : "",
          title: idle(v) ? "No Item this rule picks has " + v + "; it keeps its place and number" : "",
          draggable: true,
          ondragstart: (event) => { dragged = i; event.dataTransfer.effectAllowed = "move"; li.classList.add("dragging"); },
          ondragend: () => li.classList.remove("dragging"),
          ondragover: (event) => { event.preventDefault(); li.classList.add("drop"); },
          ondragleave: () => li.classList.remove("drop"),
          ondrop: (event) => {
            event.preventDefault(); li.classList.remove("drop");
            if (dragged >= 0 && dragged !== i) { const next = [...values]; next.splice(i, 0, ...next.splice(dragged, 1)); set(next); }
            dragged = -1;
          },
        },
          el("span", { className: "order-grip", textContent: "⋮⋮", "aria-hidden": "true" }),
          counted[i] === null
            ? el("span", { className: "order-number unnumbered", textContent: "—", title: "Not numbered: no number, nor the text after {#}" })
            : el("input", { className: "order-number" + (Object.keys(numbers[key] || {}).some((name) => same(name, v)) ? " set" : ""), type: "text", inputMode: "numeric",
              value: String(counted[i]).padStart(pad, "0"), title: "Its number. Type another to skip some, or the one before to share it; the ones after count on from it. Clear it to count on from the one before.",
              onchange: (event) => renumber(i, event.target), onkeydown: (event) => { if (event.key === "Enter") { event.preventDefault(); event.target.blur(); } } }),
          el("span", { className: "order-value", textContent: v, title: v }),
          el("span", { className: "order-acts" },
            el("button", { type: "button", className: "order-act" + (skipped(v) ? " on" : ""), textContent: "#",
              title: skipped(v) ? "Number it again" : "Leave it unnumbered", onclick: () => toggle(v) }),
            act("Up", "↑", () => move(i, i - 1), i === 0),
            act("Down", "↓", () => move(i, i + 1), i === values.length - 1),
            act("Remove", "×", () => set(values.filter((_, n) => n !== i)))));
        return li;
      })),
      unlistedPick(s, key),
      unlisted.length ? el("div", { className: "order-add" }, el("span", { className: "order-add-label", textContent: "Not numbered" }),
        ...unlisted.map((v) => el("button", { type: "button", className: "order-chip", textContent: "+ " + v + " · " + holders(v).length,
          title: "Number " + v + " last", onclick: () => set([...values, v]) })),
        el("button", { type: "button", className: "order-act order-show", textContent: "Files…",
          title: "List the Items holding each value not numbered, to find a value written two ways", onclick: () => showHolders(key, unlisted.map((v) => [v, holders(v)])) }))
        : null,
    ].filter(Boolean));
  };
  box.redraw = draw;
  draw();
  return el("div", { className: "condition" }, el("code", { title: "{#} numbers " + rest }, key), box);
}

// showHolders lists, in a dialog, the Items behind each value key leaves
// unnumbered; each opens in Browse.
function showHolders(key, groups) {
  $("holders-title").textContent = "Not numbered: " + key;
  $("holders-list").replaceChildren(...groups.map(([v, items]) => el("section", { className: "holders-group" },
    el("h4", { className: "holders-value" }, el("span", { textContent: v }), el("span", { className: "holders-count", textContent: String(items.length) })),
    el("ul", { className: "holders-items" }, ...items.map((i) => {
      const [type, ...rest] = label(state, i).split(" · ");
      return el("li", {}, el("a", { href: api("/browse/?read") + "#" + encodeURIComponent(i.id), target: "_blank", title: label(state, i) },
        el("span", { className: "holders-type", textContent: type }), el("span", { className: "holders-fields", textContent: rest.join(" · ") })));
    })))));
  $("holders-dialog").showModal();
}

// condKeys is the keys an if can ask of: a layout's, less the ways of
// writing one, and tags and status.
const condKeys = () => [...new Set(["type", "tags", "status", "id", "owner",
  ...keyOptions().flatMap(([, list]) => list).filter((k) => !k.includes(":"))])];

// condValues is the values known for key: a type and those above it, the
// tags in use, a field's options and groups, and what the Items hold.
function condValues(key) {
  if (key === "type" || key.endsWith(".type")) return [...new Set(state.templates.flatMap((t) => [t.type, ...(t.lineage || [])]))].sort();
  if (key === "tags") return tagUses(state.items).map((t) => t.name);
  if (key === "status") return ["superseded", "retired"];
  if (key === "id") return state.items.map((i) => i.id);
  const field = key.split(".").pop();
  const out = new Set();
  for (const t of state.templates) for (const f of t.fields) if (f.key === field) {
    (f.values || []).forEach((g) => out.add(g.name));
    (f.options || []).forEach((o) => out.add(o));
  }
  if (!key.includes(".")) for (const item of state.items) {
    const v = currentFields(item)[key];
    if (v) v.split(", ").forEach((x) => out.add(x));
  }
  return [...out].slice(0, 300);
}
const ifBubbles = condition($("if"), { keys: condKeys, values: condValues, onEdit: () => changed() });
