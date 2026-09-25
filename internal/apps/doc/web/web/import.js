// Import: open a folder from anywhere, see its PDFs as the tree of folders
// they are in, and take them in one at a time. The folder is only read.
import { $, el, size, loadState, post, templateOf, label, inputFor, fieldsOf, frame, say, showText, showPreview, showSource } from "/common.js";
import { openFile } from "/ui/filedialog.js";

const FOLDER_KEY = "dgs-doc-import-folder";
let state = { templates: [], items: [] };
let dir = "";
let files = [];
let selected = "";
// What the text of the picked PDF suggests, by type: { type: { key: value } }.
let suggestions = {};
const closed = new Set();

function remembered() {
  try { return localStorage.getItem(FOLDER_KEY) || ""; } catch { return ""; }
}
function remember(folder) {
  try { localStorage.setItem(FOLDER_KEY, folder); } catch { /* not kept */ }
}

// files as nested folders: { dirs: Map(name → node), files: [file] }.
function nest(list) {
  const top = { dirs: new Map(), files: [] };
  for (const f of list) {
    const parts = f.path.split("/");
    let node = top;
    for (const part of parts.slice(0, -1)) {
      if (!node.dirs.has(part)) node.dirs.set(part, { dirs: new Map(), files: [] });
      node = node.dirs.get(part);
    }
    node.files.push(f);
  }
  return top;
}

function counts(node) {
  let all = node.files.length, kept = node.files.filter((f) => f.item).length;
  for (const child of node.dirs.values()) {
    const [a, k] = counts(child);
    all += a;
    kept += k;
  }
  return [all, kept];
}

function drawNode(node, prefix, depth) {
  const rows = [];
  for (const [name, child] of [...node.dirs.entries()].sort(([a], [b]) => a.localeCompare(b))) {
    const path = prefix + name + "/";
    const [all, kept] = counts(child);
    const open = !closed.has(path);
    rows.push(el("li", {
      className: "folder", style: `padding-left: ${14 + depth * 16}px`,
      onclick: () => { open ? closed.add(path) : closed.delete(path); render(); },
    }, el("span", { className: "caret" }, open ? "▾" : "▸"), name,
    el("span", { className: "sub-inline numeric" }, kept ? `${kept}/${all} in tree` : String(all))));
    if (open) rows.push(...drawNode(child, path, depth + 1));
  }
  for (const f of node.files) {
    const li = el("li", { className: "file", style: `padding-left: ${30 + depth * 16}px`, onclick: () => pick(f.path) },
      f.path.slice(f.path.lastIndexOf("/") + 1),
      f.item ? el("span", { className: "tag" }, "in tree") : null,
      el("span", { className: "sub" }, size(f.size) + " · " + new Date(f.modified).toLocaleDateString()));
    if (f.path === selected) li.classList.add("selected");
    if (f.item) li.classList.add("done");
    rows.push(li);
  }
  return rows;
}

function render() {
  frame(state);
  $("dir").textContent = dir;
  const kept = files.filter((f) => f.item).length;
  $("counts").textContent = dir ? `${files.length} PDFs · ${kept} in tree` : "";
  $("tree").replaceChildren(el("ul", { className: "rows" }, ...drawNode(nest(files), "", 0)));
  if (dir && files.length === 0) say($("list-message"), "No PDFs in this folder or under it.");
  const file = files.find((f) => f.path === selected);
  $("side").hidden = !file;
  $("import").hidden = !file || !!file.item || !state.tree;
  $("kept").hidden = !file || !file.item;
  if (file && file.item) {
    const item = state.items.find((i) => i.id === file.item);
    $("kept-link").textContent = item ? label(state, item) : file.item;
    $("kept-link").href = "/browse/#" + file.item;
  }
}

function pick(path) {
  if (path !== selected) {
    showPreview({ dir, path }, "/api/source/file?dir=" + encodeURIComponent(dir) + "&path=" + encodeURIComponent(path));
    say($("import-message"), "");
    suggestions = {};
    showText({ dir, path }, (answer) => {
      if (selected !== path) return;
      // An earlier page's value stands: a document says what it is first.
      for (const [type, found] of Object.entries(answer.suggestions || {})) {
        suggestions[type] = { ...found, ...(suggestions[type] || {}) };
      }
      suggest();
    }).then(() => { if (selected === path) readAhead(path); });
  }
  selected = path;
  render();
  drawFields();
}

// readAhead has the next PDFs not yet in the tree read while this one is
// looked at, so their text is there when they are picked.
const AHEAD = 3;
function readAhead(path) {
  const order = files.map((f) => f.path);
  const next = order.slice(order.indexOf(path) + 1)
    .filter((p) => !files.find((f) => f.path === p).item).slice(0, AHEAD);
  if (next.length) post("/api/ahead", { dir, paths: next }).catch(() => {});
}

function drawFields() {
  const select = $("template");
  const current = select.value;
  select.replaceChildren(...state.templates.map((t) => el("option", { value: t.type }, t.type + " (" + t.kind + ")")));
  if (current && templateOf(state, current)) select.value = current;
  const t = templateOf(state, select.value);
  $("import-button").disabled = !t;
  if (!t) {
    $("into-field").hidden = true;
    $("fields").replaceChildren(el("p", { className: "message" }, "No Templates in the tree's templates folder."));
    return;
  }
  const into = $("into");
  const was = into.value;
  const documents = t.kind === "document" ? state.items.filter((i) => i.type === t.type) : [];
  into.replaceChildren(el("option", { value: "" }, "A new item"),
    ...documents.map((i) => el("option", { value: i.id }, "New revision of " + label(state, i))));
  if (documents.some((i) => i.id === was)) into.value = was;
  $("into-field").hidden = documents.length === 0;
  const adding = into.value !== "";
  $("import-button").textContent = adding ? "Add revision" : "Import";
  $("fields").replaceChildren(...(adding ? [] : t.fields.map((f) => inputFor(f, "", (t.defaults || {})[f.key] || ""))));
  for (const input of $("fields").querySelectorAll("input")) {
    input.addEventListener("input", () => { input.classList.remove("suggested"); showSource(null); });
    const source = () => input.classList.contains("suggested") && showSource((suggestions[$("template").value] || {})[input.name]);
    input.addEventListener("focus", source);
    input.addEventListener("mouseenter", source);
    input.addEventListener("blur", () => showSource(null));
    input.addEventListener("mouseleave", () => document.activeElement !== input && showSource(null));
  }
  suggest();
}

// suggest fills the empty fields the text has a value for, marked so they
// read as proposals. A field the reader typed in is never replaced.
function suggest() {
  const found = suggestions[$("template").value] || {};
  for (const input of $("fields").querySelectorAll("input")) {
    const value = found[input.name] && found[input.name].value;
    if (value && (input.value === "" || input.classList.contains("suggested"))) {
      input.value = value;
      input.classList.add("suggested");
      input.title = "Suggested from the text. Hover to see where on the page it was read.";
    }
  }
}

async function open(folder) {
  say($("list-message"), "Reading…");
  const response = await fetch("/api/source?dir=" + encodeURIComponent(folder));
  const answer = await response.json();
  if (!response.ok) {
    say($("list-message"), answer.error || response.statusText, true);
    return;
  }
  say($("list-message"), "");
  if (folder !== dir) {
    selected = "";
    closed.clear();
    $("frame").hidden = true;
    $("pages").hidden = true;
    $("empty").hidden = false;
  }
  dir = answer.dir;
  files = answer.files;
  remember(dir);
  render();
}

$("open").onclick = async () => {
  const chosen = await openFile({
    title: "Open folder",
    message: "Choose a folder. Its PDFs, and those in folders under it, are listed; nothing in it changes.",
    folders: true,
    folder: dir || remembered() || state.root,
    fallbacks: [state.root, ""],
    filters: [{ label: "PDF", extensions: [".pdf"] }],
  });
  if (chosen) await open(Array.isArray(chosen) ? chosen[0] : chosen);
};

$("template").onchange = drawFields;
$("into").onchange = drawFields;

$("import").onsubmit = async (event) => {
  event.preventDefault();
  $("import-button").disabled = true;
  say($("import-message"), "Copying and reading back…");
  try {
    if ($("into").value && !$("into-field").hidden) {
      await post("/api/revisions", { dir, path: selected, item: $("into").value });
    } else {
      await post("/api/import", { dir, path: selected, type: $("template").value, fields: fieldsOf($("fields")) });
    }
    const done = selected;
    state = await loadState();
    await open(dir);
    const order = files.map((f) => f.path);
    const next = order.slice(order.indexOf(done) + 1).concat(order).find((p) => !files.find((f) => f.path === p).item);
    if (next) pick(next);
    else say($("list-message"), "Everything in this folder is in the tree.");
  } catch (err) {
    say($("import-message"), err.message, true);
  } finally {
    $("import-button").disabled = false;
  }
};

loadState().then(async (s) => {
  state = s;
  render();
  const last = remembered();
  if (last) await open(last);
}).catch((err) => {
  state.error = err.message;
  render();
});
