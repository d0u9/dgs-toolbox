// Outlines: the Items grouped into a tree of folders, each counting the PDFs
// beneath it. The Outline is built in a form, as a View is; the server groups
// the Items and this draws the tree and lists the PDFs of the folder picked.
import { $, api, el, loadState, post, label, frame, say } from "/common.js";
import * as which from "/layoutform.js";

const FOLDER = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.75 3.25h4.5l1.5 1.5h6.5v8H1.75z" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/><path d="M1.75 6.25h12.5" stroke="currentColor" stroke-width="1.25"/></svg>';

let state = { templates: [], items: [] };
let outlines = [];
let editing = null; // the saved name of the Outline shown, or "" for a new one
let saved = ""; // the Outline as last loaded or saved, to tell an edit
let grouping = null; // the server's answer for the Outline in the form
let picked = ""; // the path of the folder picked; "" is the whole Outline
const closed = new Set(); // folder paths drawn shut
const UNPLACED = "\u0000unplaced";

const blank = () => ({ name: "", query: {}, selection: "head", layout: "" });

function list() {
  $("count").textContent = outlines.length;
  const row = (name, sub, selected, onclick) => el("li", { className: selected ? "selected" : "", onclick },
    el("span", { className: "template-name" }, name), el("span", { className: "template-sub mono" }, sub));
  $("outlines").replaceChildren(...outlines.map((o) => row(o.name, o.layout, o.name === editing, () => leave() && open(o.name))),
    ...(editing === "" ? [row("new outline", "not saved yet", true)] : []));
}

// current is the Outline as the form has it.
function current() {
  return { name: $("name").value.trim(), ...which.read() };
}
const text = (o) => JSON.stringify({ name: o.name, query: o.query || {}, selection: o.selection || "head", layout: o.layout, order: o.order || null });

// leave asks before an unsaved edit is dropped.
function leave() {
  return editing === null || text(current()) === saved || confirm("Drop the unsaved changes to this Outline?");
}

function open(name) {
  editing = name;
  const o = outlines.find((x) => x.name === name) || blank();
  $("name").value = o.name;
  which.fill(o);
  saved = text(current());
  picked = "";
  closed.clear();
  history.replaceState(null, "", name ? "#" + encodeURIComponent(name) : location.pathname + location.search);
  $("title").textContent = name || "New Outline";
  $("delete").hidden = !name;
  if (!name) editMode(true);
  say($("message"), "");
  say($("form-message"), "");
  list();
  changed();
}

function editMode(on) {
  $("form").hidden = !on;
  $("edit").setAttribute("aria-pressed", String(on));
  $("edit").textContent = on ? "Done" : "Edit";
}

let pending = 0;
let asked = 0;
// changed regroups the Outline in the form, saved or not, a moment after the
// last change.
function changed() {
  $("dirty").hidden = editing === null || text(current()) === saved;
  clearTimeout(pending);
  pending = setTimeout(regroup, 200);
}

async function regroup() {
  const mine = ++asked;
  const o = current();
  if (!o.layout) {
    grouping = null;
    return draw();
  }
  try {
    const answer = await post("/api/outlines/group", { ...o, name: o.name || "preview" });
    if (mine !== asked) return;
    grouping = answer;
    say($("message"), "");
  } catch (err) {
    if (mine !== asked) return;
    grouping = null;
    say($("message"), err.message, true);
  }
  draw();
}

// find is the node at path, or null.
function find(node, path) {
  if (!path || !node) return node;
  for (const c of node.children) {
    if (c.path === path) return c;
    if (path.startsWith(c.path + "/")) return find(c, path);
  }
  return null;
}

function draw() {
  const root = grouping && grouping.root;
  const missing = (grouping && grouping.missing) || [];
  $("total").textContent = root ? root.count + (root.count === 1 ? " PDF" : " PDFs") : "";
  if (picked !== UNPLACED && !find(root, picked)) picked = "";
  const rows = [];
  const walk = (node, depth) => {
    for (const c of node.children) {
      rows.push(folderRow(c.name, c.path, c.count, depth, c.children.length > 0));
      if (!closed.has(c.path)) walk(c, depth + 1);
    }
  };
  if (root) walk(root, 0);
  if (missing.length) rows.push(folderRow("Not placed", UNPLACED, missing.length, 0, false, true));
  $("tree").replaceChildren(...(rows.length ? rows : [el("p", { className: "muted outline-empty" }, root ? "No Item matches." : current().layout ? "" : "Add a level to see the tree: Edit, then click a key.")]));
  files();
}

function folderRow(name, path, count, depth, parent, unplaced) {
  const open = !closed.has(path);
  const twist = el("span", { className: "outline-twist" + (parent ? "" : " leaf"), textContent: parent ? (open ? "▾" : "▸") : "",
    onclick: (event) => { event.stopPropagation(); if (open) closed.add(path); else closed.delete(path); draw(); } });
  const icon = el("span", { className: "ft-icon" });
  icon.innerHTML = FOLDER;
  return el("div", { className: "outline-row" + (picked === path ? " selected" : "") + (unplaced ? " unplaced" : ""),
    role: "treeitem", tabIndex: 0, style: `--depth:${depth}`, title: unplaced ? "PDFs the layout cannot place" : path,
    onclick: () => { picked = picked === path ? "" : path; draw(); },
    onkeydown: (event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); event.currentTarget.click(); } } },
    twist, icon, el("span", { className: "outline-name" }, name), el("span", { className: "outline-count numeric" }, String(count)));
}

// files lists the PDFs in the folder picked and beneath it, each under the
// folder it is in below the one picked.
function files() {
  const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
  const name = (id) => byId[id] ? label(state, byId[id]) : id;
  const link = (f, extra) => el("li", {}, el("a", { href: api("/browse/") + "#" + f.item }, name(f.item)),
    f.revision > 1 || current().selection === "all" ? el("span", { className: "muted" }, " · revision " + f.revision) : null,
    extra ? el("div", { className: "template-sub" }, extra) : null);
  if (picked === UNPLACED) {
    const missing = grouping.missing;
    $("folder").textContent = "Not placed";
    $("folder-count").textContent = missing.length;
    // A numbered level an Item has a value for lacks only a place in its
    // order: that is fixed here, not in the Item.
    const numbered = which.numberedKeys();
    $("files").replaceChildren(...missing.map((m) => {
      const li = link(m, "lacks " + m.keys.join(", "));
      for (const key of m.keys.filter((k) => numbered.includes(k))) {
        const value = byId[m.item] && which.orderValue(byId[m.item], key);
        if (value) li.append(el("button", { type: "button", className: "small", textContent: "Number " + value + " last",
          onclick: () => which.numberLast(key, value) }));
      }
      return li;
    }));
    return;
  }
  const node = find(grouping && grouping.root, picked);
  $("folder").textContent = picked ? picked.split("/").pop() : "All PDFs";
  $("folder-count").textContent = node ? node.count : "";
  if (!node) return $("files").replaceChildren();
  const out = [];
  const walk = (n) => {
    if (n.files.length) {
      if (n.path !== picked) out.push(el("li", { className: "outline-sub mono" }, n.path.slice(picked ? picked.length + 1 : 0)));
      out.push(...n.files.map((f) => link(f)));
    }
    n.children.forEach(walk);
  };
  walk(node);
  $("files").replaceChildren(...out);
}

async function reload() {
  state = await loadState();
  frame(state);
  const [views, answer] = await Promise.all([
    fetch(api("/api/views")).then((r) => r.json()),
    fetch(api("/api/outlines")).then((r) => r.json()),
  ]);
  which.setup({ state, keys: views.keys, countries: views.countries || {}, onChange: changed });
  outlines = answer.outlines;
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
}

$("form").addEventListener("input", changed);
$("form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const o = current();
  try {
    await post("/api/outlines", { outline: o, previous: editing || "" });
    await reload();
    open(o.name);
    say($("form-message"), "Saved outlines/" + o.name + ".yaml.");
  } catch (err) {
    say($("form-message"), err.message, true);
  }
});
$("new").onclick = () => leave() && open("");
$("edit").onclick = () => editMode($("form").hidden);
$("delete").onclick = async () => {
  if (!editing || !confirm("Delete the Outline " + editing + "? Its file under outlines/ is removed; no Item changes.")) return;
  try {
    await post("/api/outlines/delete", { name: editing });
    await reload();
    editing = null;
    open(outlines.length ? outlines[0].name : "");
  } catch (err) {
    say($("form-message"), err.message, true);
  }
};
window.addEventListener("beforeunload", (event) => { if (editing !== null && text(current()) !== saved) event.preventDefault(); });

reload().then(() => {
  const wanted = decodeURIComponent(location.hash.slice(1));
  editMode(false);
  open(outlines.some((o) => o.name === wanted) ? wanted : outlines.length ? outlines[0].name : "");
}).catch((err) => {
  state.error = err.message;
  frame(state);
});
