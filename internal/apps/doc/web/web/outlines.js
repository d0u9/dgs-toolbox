// Outlines: the Items grouped into a tree of folders, each counting the PDFs
// beneath it. The server groups them; this draws the tree and lists the PDFs
// of the folder picked.
import { $, api, el, loadState, post, label, frame, say, statusBar } from "/common.js";
import { codeEditor } from "/ui/codeedit.js";

statusBar.setHints("<kbd>Ctrl</kbd>+<kbd>S</kbd> save", { html: true });

const FOLDER = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.75 3.25h4.5l1.5 1.5h6.5v8H1.75z" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/><path d="M1.75 6.25h12.5" stroke="currentColor" stroke-width="1.25"/></svg>';

let state = { templates: [], items: [] };
let outlines = [];
let example = "";
let editing = null; // the saved name of the Outline shown, or "" for a new one
let saved = ""; // the text as last loaded or saved, to tell an edit
let grouping = null; // the server's answer for the text in the editor
let picked = ""; // the path of the folder picked; "" is the whole Outline
const closed = new Set(); // folder paths drawn shut
const UNPLACED = "\u0000unplaced";

const editor = codeEditor($("yaml"), { onChange: changed, onSave: save });

function list() {
  $("count").textContent = outlines.length;
  const row = (name, sub, selected, onclick) => el("li", { className: selected ? "selected" : "", onclick },
    el("span", { className: "template-name" }, name), el("span", { className: "template-sub mono" }, sub));
  $("outlines").replaceChildren(...outlines.map((o) => row(o.name, o.layout, o.name === editing, () => leave() && open(o.name))),
    ...(editing === "" ? [row("new outline", "not saved yet", true)] : []));
}

// leave asks before an unsaved edit is dropped.
function leave() {
  return editor.value === saved || confirm("Drop the unsaved changes to this Outline?");
}

function open(name, text) {
  editing = name;
  const o = outlines.find((x) => x.name === name);
  saved = o ? o.data : example;
  editor.value = text ?? saved;
  picked = "";
  closed.clear();
  history.replaceState(null, "", name ? "#" + encodeURIComponent(name) : location.pathname + location.search);
  $("title").textContent = o ? o.name : "New Outline";
  $("file").textContent = "outlines/" + (o ? o.name : "<name>") + ".yaml";
  $("delete").hidden = !o;
  if (!o) editMode(true);
  say($("message"), "");
  list();
  changed();
}

function editMode(on) {
  $("editor-row").hidden = !on;
  $("save").hidden = !on;
  $("edit").setAttribute("aria-pressed", String(on));
  $("edit").textContent = on ? "Done" : "Edit";
}

let pending = 0;
// changed regroups the text in the editor, saved or not, a moment after the
// last keystroke.
function changed() {
  $("dirty").hidden = editor.value === saved;
  clearTimeout(pending);
  pending = setTimeout(regroup, 250);
}

async function regroup() {
  try {
    grouping = await post("/api/outlines/group", { data: editor.value });
    say($("message"), "");
  } catch (err) {
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
  $("tree").replaceChildren(...(rows.length ? rows : [el("p", { className: "muted outline-empty" }, root ? "No Item matches." : "")]));
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
    f.revision > 1 || (grouping && editor.value.includes("selection: all")) ? el("span", { className: "muted" }, " · revision " + f.revision) : null,
    extra ? el("div", { className: "template-sub" }, extra) : null);
  if (picked === UNPLACED) {
    const missing = grouping.missing;
    $("folder").textContent = "Not placed";
    $("folder-count").textContent = missing.length;
    $("files").replaceChildren(...missing.map((m) => link(m, "lacks " + m.keys.join(", "))));
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
  const answer = await (await fetch(api("/api/outlines"))).json();
  outlines = answer.outlines;
  example = answer.example;
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
}

async function save() {
  try {
    const o = await post("/api/outlines", { previous: editing || "", data: editor.value });
    await reload();
    open(o.name);
    say($("message"), "Saved.");
  } catch (err) {
    say($("message"), err.message, true);
  }
}

$("new").onclick = () => leave() && open("");
$("edit").onclick = () => editMode($("editor-row").hidden);
$("save").onclick = save;
$("delete").onclick = async () => {
  if (!editing || !confirm("Delete the Outline " + editing + "? Its file is removed; no Item changes.")) return;
  try {
    await post("/api/outlines/delete", { name: editing });
    await reload();
    saved = editor.value;
    open(outlines.length ? outlines[0].name : "");
  } catch (err) {
    say($("message"), err.message, true);
  }
};
window.addEventListener("beforeunload", (event) => { if (editor.value !== saved) event.preventDefault(); });

reload().then(() => {
  const wanted = decodeURIComponent(location.hash.slice(1));
  editMode(false);
  open(outlines.some((o) => o.name === wanted) ? wanted : outlines.length ? outlines[0].name : "");
}).catch((err) => {
  state.error = err.message;
  frame(state);
});
