// Snapshots: a fixed subtree of PDFs, each at its path with the revision it
// had. One is taken from a rule — what the rule places now — or begun
// empty, and edited by hand after in its tree: folders made, PDFs added to
// the folder picked, dragged into another, renamed, set to another revision
// or taken out. An Outline puts it in its tree as a folder of its name; the
// Outlines page says where.
import { $, api, el, loadState, post, label, frame, say } from "/common.js";
import { fileTree } from "/ui/filetree.js";
import { openMenu } from "/ui/menu.js";
import { resizable } from "/outlinetree.js";

let state = { templates: [], items: [] };
let snapshots = [];
let rules = [];
let editing = null; // the saved name of the Snapshot shown
let draft = null; // the Snapshot as edited: name, about, files, folders
let saved = "";
let picked = ""; // the file's path, or a folder's with a trailing "/"
let dragging = ""; // the path being dragged, as picked is
let renaming = ""; // the path whose name is being typed in the tree, as picked is
const closed = new Set();
resizable(document.querySelector(".outline-main"), "dgs-doc-snapshots-tree");

const copy = (o) => JSON.parse(JSON.stringify(o));
const text = (s) => JSON.stringify({ name: s.name, about: s.about || "", files: s.files.map((f) => [f.path, f.item, f.revision]),
  folders: prune(s).sort() });
const itemOf = (id) => state.items.find((i) => i.id === id);
const ref = (r) => r.id || r.digest;
const low = (p) => p.toLowerCase();
const dirOf = (p) => p.includes("/") ? p.slice(0, p.lastIndexOf("/")) : "";
const baseOf = (p) => p.slice(p.lastIndexOf("/") + 1);
const join = (dir, name) => dir ? dir + "/" + name : name;
const isFolder = (p) => p.endsWith("/");
const within = (p, dir) => low(p).startsWith(low(dir) + "/");

// folders is every folder of the draft: those made by hand and those its
// PDFs are in.
function folders() {
  const out = new Set(draft.folders);
  for (const f of draft.files) for (let d = dirOf(f.path); d; d = dirOf(d)) out.add(d);
  for (const d of draft.folders) for (let p = dirOf(d); p; p = dirOf(p)) out.add(p);
  return [...out];
}

// prune is the folders worth writing: those nothing else puts in the tree.
function prune(s) {
  return s.folders.filter((d) => !s.files.some((f) => within(f.path, d)) && !s.folders.some((e) => within(e, d)));
}

// taken reports whether path is a PDF's or a folder's already.
const taken = (path) => draft.files.some((f) => low(f.path) === low(path)) || folders().some((d) => low(d) === low(path));

// target is the folder PDFs and folders are added to: the one picked, or
// the one the picked PDF is in.
const target = () => isFolder(picked) ? picked.slice(0, -1) : dirOf(picked);

function list() {
  $("count").textContent = snapshots.length;
  const row = (name, sub, selected, onclick) => el("li", { className: selected ? "selected" : "", onclick },
    el("span", { className: "template-name" }, name), el("span", { className: "template-sub" }, sub));
  $("snapshots").replaceChildren(...snapshots.map((s) => row(s.name,
    (s.about ? s.about + " · " : "") + (s.used.length ? "in " + s.used.join(", ") : "in no Outline"),
    s.name === editing, () => leave() && open(s.name))),
    ...(editing === "" ? [row("new snapshot", "not taken yet", true)] : []));
}

// relocate moves the PDF or folder at from to the path to, with everything
// in the folder, and answers why not when it cannot.
function relocate(from, to) {
  const folder = isFolder(from);
  const old = folder ? from.slice(0, -1) : from;
  if (!to || to.split("/").some((p) => !p || p === "." || p === "..")) return "Not a name";
  if (low(to) === low(old)) return "";
  if (folder && within(to, old)) return "A folder cannot go inside itself";
  if (taken(to) && low(to) !== low(old)) return to + " is there already";
  const move = (p) => low(p) === low(old) ? to : within(p, old) ? to + p.slice(old.length) : p;
  draft.files.forEach((f) => { f.path = move(f.path); });
  draft.folders = draft.folders.map(move);
  if (folder && draft.folders.every((d) => low(d) !== low(to))) draft.folders.push(to);
  const left = dirOf(old);
  if (left && !folders().some((d) => low(d) === low(left))) draft.folders.push(left);
  if (picked === from || (folder && within(picked, old))) picked = folder ? move(picked.slice(0, -1)) + "/" : move(picked);
  return "";
}

function tried(why) {
  say($("message"), why, !!why);
  changed();
}

function pick(path) {
  picked = picked === path ? "" : path;
  draw();
}

// draw redraws the tree, the picked row's controls and where PDFs go.
function draw() {
  if (picked && !(isFolder(picked) ? folders().some((d) => d + "/" === picked) : draft.files.some((f) => f.path === picked))) picked = "";
  const count = draft.files.length;
  $("total").textContent = count + (count === 1 ? " PDF" : " PDFs");
  const lost = lostOf();
  const host = $("tree");
  host.replaceChildren(draft.files.length || draft.folders.length ? fileTree(draft.files, {
    closed, selected: picked, folders: folders().map((d) => d + "/"),
    onPick: (f) => pick(f.path),
    onPickFolder: (path) => pick(path),
    fileClass: (f) => lost.has(f.item + " " + f.revision) ? "snap-lost" : "",
    folderExtra: (_, files) => el("span", { className: "numeric" }, String(files.length)),
    fileExtra: (f) => el("button", { type: "button", className: "ft-reveal", title: "Show in Finder", "aria-label": "Show in Finder", textContent: "↗",
      onclick: (event) => { event.stopPropagation(); reveal(f); } }),
    decorate: (row, { kind, path }) => {
      // Picked without a redraw, which would scroll and so shut the menu.
      row.addEventListener("contextmenu", (event) => {
        picked = path;
        host.querySelectorAll(".ft-row.selected").forEach((r) => r.classList.remove("selected"));
        row.classList.add("selected");
        details();
        menu(event, path);
      });
      if (path === renaming) return inline(row, path);
      row.draggable = true;
      row.addEventListener("dragstart", (event) => { dragging = path; event.dataTransfer.setData("text/plain", path); event.dataTransfer.effectAllowed = "move"; });
      row.addEventListener("dragend", () => { dragging = ""; host.querySelectorAll(".drop").forEach((r) => r.classList.remove("drop")); host.classList.remove("drop"); });
      if (kind !== "folder") return;
      row.addEventListener("dragover", (event) => {
        if (!dragging || dragging === path) return;
        event.preventDefault();
        event.stopPropagation();
        row.classList.add("drop");
      });
      row.addEventListener("dragleave", () => row.classList.remove("drop"));
      row.addEventListener("drop", (event) => {
        event.preventDefault();
        event.stopPropagation();
        row.classList.remove("drop");
        const from = event.dataTransfer.getData("text/plain") || dragging;
        if (!from || from === path) return;
        closed.delete(path);
        tried(relocate(from, join(path.slice(0, -1), baseOf(isFolder(from) ? from.slice(0, -1) : from))));
      });
    },
  }) : el("p", { className: "muted outline-empty" }, "No PDF yet: add one, or right-click here for a folder."));
  // The picked row keeps the keyboard across a redraw, so Enter renames it.
  const focus = document.activeElement;
  if (picked && !renaming && (!focus || focus === document.body || host.contains(focus))) host.querySelector(".ft-row.selected")?.focus({ preventScroll: true });
  details();
}

// inline turns row's name into a field, as Finder does: Enter or leaving
// it renames, Escape keeps the name.
function inline(row, path) {
  const old = isFolder(path) ? path.slice(0, -1) : path;
  const input = el("input", { className: "mono ft-rename", value: baseOf(old), spellcheck: false, autocomplete: "off" });
  let done = false;
  const finish = (keep) => {
    if (done) return;
    done = true;
    renaming = "";
    const name = input.value.trim();
    if (keep || !name || name === baseOf(old)) return draw();
    if (name.includes("/")) return tried("A name holds no /: drag it into a folder instead");
    tried(relocate(path, join(dirOf(old), name)));
  };
  input.addEventListener("click", (event) => event.stopPropagation());
  input.addEventListener("keydown", (event) => {
    event.stopPropagation();
    if (event.key === "Enter") { event.preventDefault(); finish(false); }
    if (event.key === "Escape") { event.preventDefault(); finish(true); }
  });
  input.addEventListener("blur", () => finish(false));
  row.querySelector(".ft-name").replaceChildren(input);
  requestAnimationFrame(() => {
    input.focus();
    const dot = isFolder(path) ? -1 : input.value.lastIndexOf(".");
    input.setSelectionRange(0, dot > 0 ? dot : input.value.length);
  });
}

// rename starts typing a new name for path in the tree.
function rename(path) {
  renaming = path;
  picked = path;
  for (let d = dirOf(isFolder(path) ? path.slice(0, -1) : path); d; d = dirOf(d)) closed.delete(d + "/");
  draw();
}

// newFolder makes an untitled folder in dir and names it in the tree.
function newFolder(dir) {
  let path = join(dir, "untitled folder"), n = 2;
  while (taken(path)) path = join(dir, "untitled folder " + n++);
  draft.folders.push(path);
  if (dir) closed.delete(dir + "/");
  changed();
  rename(path + "/");
}

// remove takes the PDF or folder at path out of the Snapshot; a folder goes
// with its PDFs, asked first. The folder it was in stays.
function remove(path) {
  const folder = isFolder(path);
  const old = folder ? path.slice(0, -1) : path;
  const inside = folder ? draft.files.filter((f) => within(f.path, old)) : [];
  if (inside.length && !confirm("Take the folder " + old + " and its " + inside.length + " PDFs out of the Snapshot? Their Items stay.")) return;
  draft.files = draft.files.filter((f) => f.path !== old && !within(f.path, old));
  draft.folders = draft.folders.filter((d) => low(d) !== low(old) && !within(d, old));
  const dir = dirOf(old);
  if (dir && !folders().includes(dir)) draft.folders.push(dir);
  picked = dir ? dir + "/" : "";
  changed();
}

// menu is the right-click menu on a row, or with no path on the space
// around them.
function menu(event, path) {
  const folder = !path || isFolder(path);
  const dir = !path ? "" : folder ? path.slice(0, -1) : dirOf(path);
  const f = path && !folder ? draft.files.find((x) => x.path === path) : null;
  openMenu(event, [
    [{ label: "New Folder", onSelect: () => newFolder(folder ? dir : dirOf(path)) },
      { label: "Add PDFs " + (dir ? "to " + baseOf(dir) : "to the top") + "…", onSelect: () => {
        picked = dir ? dir + "/" : ""; draw(); addDialog(); } }],
    path ? [{ label: "Rename", onSelect: () => rename(path) }, f ? { label: "Show in Finder", onSelect: () => reveal(f) } : null] : [],
    path ? [{ label: folder ? "Remove Folder" : "Remove", danger: true, onSelect: () => remove(path) }] : [],
  ]);
}

// reveal shows a PDF of the Snapshot in Finder, as its revision has it.
function reveal(f) {
  const r = (itemOf(f.item)?.revisions || []).find((x) => ref(x) === f.revision);
  post("/api/reveal", { item: f.item, digest: r ? r.digest : "" }).catch((err) => alert(err.message));
}

function lostOf() {
  return new Map(((snapshots.find((s) => s.name === editing) || {}).lost || []).map((l) => [l.item + " " + l.revision, l.why]));
}

// details draws the controls for what is picked in the tree.
function details() {
  $("add-to").textContent = $("add-to-title").textContent = target() ? target() + "/" : "the top";
  const box = $("picked");
  // A folder, and a PDF's name and place, are edited in the tree; only
  // what the tree cannot show is here.
  box.parentElement.hidden = !picked || isFolder(picked);
  if (box.parentElement.hidden) return box.replaceChildren();
  const f = draft.files.find((x) => x.path === picked);
  const item = itemOf(f.item);
  const revs = item ? item.revisions || [] : [];
  const choice = el("select", { title: "The revision kept", onchange: () => { f.revision = choice.value; changed(); } },
    ...revs.map((r, n) => el("option", { value: ref(r), selected: ref(r) === f.revision },
      "revision " + (n + 1) + (ref(r) === ref(revs[revs.length - 1] || {}) ? " (latest)" : "") + (r.added ? " · " + r.added.slice(0, 10) : ""))),
    ...(revs.some((r) => ref(r) === f.revision) ? [] : [el("option", { value: f.revision, selected: true }, "gone")]));
  const why = lostOf().get(f.item + " " + f.revision);
  box.replaceChildren(el("p", { className: "mono muted" }, f.path),
    el("label", { className: "vf" }, el("span", { className: "vf-label" }, "Revision"), choice),
    el("p", {}, item ? el("a", { href: api("/browse/") + "#" + f.item }, label(state, item)) : f.item,
      why ? el("span", { className: "message error" }, " · " + why) : null));
}

// The filters are remembered in this browser, so the next Snapshot opens
// with the same ones.
const FILTERS = ["add-search", "add-owner", "add-country", "add-type", "add-tag", "add-field", "add-value", "add-fresh", "add-retired"];
const FILTERS_KEY = "dgs-doc-snapshots-filters";
let recalled = null; // the filters remembered, until the lists are filled
let recalledOnce = false;
function remember() {
  try { localStorage.setItem(FILTERS_KEY, JSON.stringify(Object.fromEntries(FILTERS.map((id) => [id, $(id).type === "checkbox" ? $(id).checked : $(id).value])))); } catch { /* not kept */ }
}
function recall() {
  try { recalled = JSON.parse(localStorage.getItem(FILTERS_KEY) || "null"); } catch { recalled = null; }
  if (!recalled) return;
  for (const id of ["add-search", "add-fresh", "add-retired"]) if (id in recalled) {
    if ($(id).type === "checkbox") $(id).checked = !!recalled[id]; else $(id).value = recalled[id];
  }
}

// filters fills each filter with the values the Items have, keeping what
// is chosen.
function filters() {
  const fill = (id, all, values) => {
    const was = recalled && id in recalled ? recalled[id] : $(id).value;
    $(id).replaceChildren(el("option", { value: "" }, all), ...[...new Set(values.filter(Boolean))].sort().map((v) => el("option", { value: v }, v)));
    $(id).value = [...$(id).options].some((o) => o.value === was) ? was : "";
  };
  fill("add-owner", "Any owner", state.items.map((i) => i.fields.owner));
  fill("add-country", "Any country", state.items.map((i) => i.fields.country));
  fill("add-type", "Any type", state.items.map((i) => i.type));
  fill("add-tag", "Any tag", state.items.flatMap((i) => i.tags || []));
  fill("add-field", "Any field", state.items.filter(ofType).flatMap((i) => Object.keys(i.fields)));
  const key = $("add-field").value;
  fill("add-value", key ? "Any " + key : "Pick a field first", key ? state.items.filter(ofType).map((i) => i.fields[key]) : []);
  $("add-value").disabled = !key;
  recalled = null;
}

const ofType = (i) => !$("add-type").value || i.type === $("add-type").value;

// The list's columns, as in Finder's list view: Name always, then the keys
// chosen by a right click on the header; a click on a header sorts by it,
// again the other way. Both are remembered in this browser.
const COLUMNS_KEY = "dgs-doc-snapshots-columns";
let columns = ["type", "owner", "country"];
let sortBy = { key: "", down: false }; // "" is Name
let widths = {}; // px per column, "" for Name
try { const kept = JSON.parse(localStorage.getItem(COLUMNS_KEY) || "null"); if (kept) ({ columns, sortBy, widths = {} } = kept); } catch { /* defaults */ }
const keep = () => { try { localStorage.setItem(COLUMNS_KEY, JSON.stringify({ columns, sortBy, widths })); } catch { /* not kept */ } };
let moving = ""; // the column whose header is being dragged
let fitList = () => {}; // sizes the list's columns to the dialog
new ResizeObserver(() => fitList()).observe($("add-list"));

// addDialog opens Add PDFs over the page.
function addDialog() {
  details();
  choices();
  $("add-dialog").showModal();
  $("add-search").focus();
}
const HEADS = { type: "Type", tags: "Tags", added: "Added" };
const cell = (i, key) => key === "type" ? i.type : key === "tags" ? (i.tags || []).join(", ") :
  key === "added" ? ((i.revisions || []).at(-1)?.added || "").slice(0, 10) : i.fields[key] || "";

// columnMenu offers every key an Item has, ticked when shown.
function columnMenu(event) {
  const keys = [...new Set(state.items.flatMap((i) => Object.keys(i.fields)))].sort();
  const item = (key) => ({ label: (columns.includes(key) ? "✓ " : "\u2003") + (HEADS[key] || key), onSelect: () => {
    columns = columns.includes(key) ? columns.filter((c) => c !== key) : [...columns, key];
    if (!columns.includes(sortBy.key)) sortBy = { key: "", down: false };
    keep();
    choices();
  } });
  openMenu(event, [["type", "tags", "added"].map(item), keys.map(item)]);
}

// choices lists the Items to add, narrowed by the search words, which match
// any field, and by the filters, each ticked to add; an Item the Snapshot
// has already says so.
function choices() {
  const words = $("add-search").value.toLowerCase().split(/\s+/).filter(Boolean);
  const have = new Set(draft ? draft.files.map((f) => f.item) : []);
  const [owner, country, type, tag, key, value] = ["add-owner", "add-country", "add-type", "add-tag", "add-field", "add-value"].map((id) => $(id).value);
  remember();
  const kept = new Set([...$("add-list").querySelectorAll("tbody input:checked")].map((b) => b.value));
  const by = (x) => sortBy.key ? cell(x.i, sortBy.key) : x.name;
  const sorted = state.items.filter((i) => (i.revisions || []).length)
    .filter((i) => (!type || i.type === type) && (!owner || i.fields.owner === owner) && (!country || i.fields.country === country) &&
      (!key || (value ? i.fields[key] === value : !!i.fields[key])) &&
      (!tag || (i.tags || []).includes(tag)) && ($("add-retired").checked || !i.retired) && (!$("add-fresh").checked || !have.has(i.id)))
    .map((i) => ({ i, name: label(state, i), all: [i.type, ...Object.values(i.fields), ...(i.tags || [])].join(" ").toLowerCase() }))
    .filter(({ all }) => words.every((w) => all.includes(w)))
    .sort((a, b) => (by(a).localeCompare(by(b), undefined, { numeric: true }) || a.name.localeCompare(b.name)) * (sortBy.down ? -1 : 1));
  if (!sorted.length) return $("add-list").replaceChildren(el("p", { className: "muted" }, "No Item matches.")), ticked();
  const all = el("input", { type: "checkbox", id: "add-all", title: "Tick all shown", onchange: () => {
    $("add-list").querySelectorAll("tbody input").forEach((b) => { b.checked = all.checked; });
    ticked();
  } });
  const head = (k, text) => {
    const th = el("th", { className: sortBy.key === k ? "sorted" : "", title: "Click to sort; drag to move; right-click for columns",
      onclick: () => { sortBy = sortBy.key === k ? { key: k, down: !sortBy.down } : { key: k, down: false }; keep(); choices(); } },
      el("span", { className: "snap-th" }, text, sortBy.key === k ? (sortBy.down ? " ˅" : " ˄") : ""));
    // The edge sizes the column, as in Finder.
    const grip = el("span", { className: "snap-grip", title: "Drag to size; double-click for the default" });
    grip.addEventListener("click", (event) => event.stopPropagation());
    grip.addEventListener("dblclick", (event) => { event.stopPropagation(); delete widths[k]; keep(); choices(); });
    grip.addEventListener("pointerdown", (event) => {
      event.preventDefault();
      event.stopPropagation();
      const col = table.querySelector(`col[data-key="${CSS.escape(k)}"]`);
      const from = event.clientX, start = parseFloat(col.style.width);
      grip.setPointerCapture(event.pointerId);
      const move = (e) => { widths[k] = Math.max(48, Math.round(start + e.clientX - from)); col.style.width = widths[k] + "px"; fit(); };
      const up = () => { grip.removeEventListener("pointermove", move); grip.removeEventListener("pointerup", up); keep(); };
      grip.addEventListener("pointermove", move);
      grip.addEventListener("pointerup", up);
    });
    th.append(grip);
    if (k) {
      th.draggable = true;
      th.addEventListener("dragstart", (event) => { moving = k; event.dataTransfer.setData("text/plain", k); event.dataTransfer.effectAllowed = "move"; });
      th.addEventListener("dragend", () => { moving = ""; });
      th.addEventListener("dragover", (event) => { if (moving && moving !== k) { event.preventDefault(); th.classList.add("drop"); } });
      th.addEventListener("dragleave", () => th.classList.remove("drop"));
      th.addEventListener("drop", (event) => {
        event.preventDefault();
        const from = event.dataTransfer.getData("text/plain") || moving;
        if (!from || from === k) return;
        const rest = columns.filter((c) => c !== from);
        rest.splice(rest.indexOf(k) + (columns.indexOf(from) < columns.indexOf(k) ? 1 : 0), 0, from);
        columns = rest;
        keep();
        choices();
      });
    }
    return th;
  };
  const width = (k) => (widths[k] || (k ? 130 : 300)) + "px";
  const table = el("table", { className: "snap-table" },
    el("colgroup", {}, el("col", { style: "width: 32px" }), ...["", ...columns].map((k) => { const c = el("col", { style: "width: " + width(k) }); c.dataset.key = k; return c; }),
      el("col", { style: "width: 36px" })),
    el("thead", { oncontextmenu: columnMenu }, el("tr", {}, el("th", { className: "snap-tick" }, all), head("", "Name"),
      ...columns.map((k) => head(k, HEADS[k] || k)),
      el("th", { className: "snap-plus", title: "Add a column", onclick: (event) => { event.stopPropagation(); columnMenu(event); } }, "+"))),
    el("tbody", {}, ...sorted.map(({ i, name }) => {
      const box = el("input", { type: "checkbox", value: i.id, checked: kept.has(i.id) });
      const tr = el("tr", { className: (kept.has(i.id) ? "ticked" : "") + (i.retired ? " retired" : ""),
        onclick: (event) => { if (event.target !== box) { box.checked = !box.checked; } tr.classList.toggle("ticked", box.checked); ticked(); } },
        el("td", { className: "snap-tick" }, box),
        el("td", { title: name }, name, have.has(i.id) ? el("span", { className: "muted" }, " · in it") : null),
        ...columns.map((k) => el("td", { title: cell(i, k) }, cell(i, k))), el("td"));
      return tr;
    })));
  // A fixed table is as wide as its columns, so a column sized narrower
  // leaves room at the right rather than widening another.
  // Each column is the width it was dragged to, and its edge follows the
  // pointer; nothing else moves. What room is left goes to the empty
  // column after the last, as in Finder, so the rows still reach the side.
  const fit = () => {
    const cols = [...table.querySelectorAll("col")];
    const filler = cols.at(-1);
    const total = cols.slice(0, -1).reduce((n, c) => n + parseFloat(c.style.width), 0);
    const room = Math.max(36, $("add-list").clientWidth - total - 2);
    filler.style.width = room + "px";
    table.style.width = total + room + "px";
  };
  $("add-list").replaceChildren(table);
  fit();
  fitList = fit;
  ticked();
}

function ticked() {
  const boxes = [...$("add-list").querySelectorAll("tbody input")];
  const n = boxes.filter((b) => b.checked).length;
  if ($("add-all")) $("add-all").checked = !!boxes.length && n === boxes.length;
  $("add").disabled = !n;
  $("add").textContent = n ? "Add " + n : "Add";
}

// leave asks before an unsaved edit is dropped.
function leave() {
  return !draft || text(draft) === saved || confirm("Drop the unsaved changes to this Snapshot?");
}

function open(name) {
  editing = name;
  const s = snapshots.find((x) => x.name === name);
  $("form").hidden = !s;
  $("take-form").hidden = !!s;
  $("tree-tools").hidden = !s;
  $("dirty").hidden = true;
  say($("message"), "");
  say($("form-message"), "");
  closed.clear();
  picked = "";
  list();
  if (!s) {
    draft = null;
    history.replaceState(null, "", location.pathname + location.search);
    $("title").textContent = "New Snapshot";
    $("total").textContent = "";
    $("tree").replaceChildren();
    return;
  }
  history.replaceState(null, "", "#" + encodeURIComponent(name));
  draft = copy({ name: s.name, about: s.about || "", taken: s.taken, rule: s.rule, outline: s.outline, node: s.node, folder: s.folder, files: s.files, folders: s.folders || [] });
  $("name").value = draft.name;
  $("about").value = draft.about;
  $("origin").textContent = "Taken " + s.taken.replace("T", " ").slice(0, 16) + (s.rule ? " from the rule " + s.rule : s.outline ? " from " + s.outline + (s.node ? " / " + s.node : "") : ", begun empty") +
    ". " + (s.used.length ? "In " + s.used.join(", ") + "." : "In no Outline yet: add it on the Outlines page.");
  $("title").textContent = name;
  saved = text(draft);
  if (!recalledOnce) { recalledOnce = true; recall(); }
  filters();
  choices();
  draw();
  if (s.lost.length) say($("message"), s.lost.length + " of its PDFs are no longer in the tree: an Outline with it is not exported until they are removed or replaced.", true);
}

function changed() {
  $("dirty").hidden = text(draft) === saved;
  draw();
}

async function reload() {
  state = await loadState();
  frame(state);
  const [snaps, outlines] = await Promise.all([(await fetch(api("/api/snapshots"))).json(), (await fetch(api("/api/outlines"))).json()]);
  snapshots = snaps.snapshots || [];
  rules = outlines.rules || [];
  const error = snaps.error || outlines.error;
  if (error) { $("error").hidden = false; $("error").textContent = error; }
  $("take-rule").replaceChildren(el("option", { value: "" }, "nothing: begin it empty"),
    ...rules.map((r) => el("option", { value: r.name }, "the rule " + r.name + " · " + r.layout)));
}

$("name").addEventListener("input", () => { draft.name = $("name").value.trim(); changed(); });
$("about").addEventListener("input", () => { draft.about = $("about").value.trim(); changed(); });
$("add-search").addEventListener("input", choices);
for (const id of ["add-owner", "add-country", "add-tag", "add-value", "add-fresh", "add-retired"]) $(id).addEventListener("change", choices);
for (const id of ["add-type", "add-field"]) $(id).addEventListener("change", () => { filters(); choices(); });
$("add-open").onclick = () => addDialog();
$("add-cancel").onclick = () => $("add-dialog").close();
$("add").onclick = () => {
  const dir = target();
  for (const box of $("add-list").querySelectorAll("tbody input:checked")) {
    const item = itemOf(box.value);
    const revs = item.revisions;
    const name = label(state, item).replace(/[\/\\:]+/g, "-");
    let path = join(dir, name + ".pdf"), n = 2;
    while (taken(path)) path = join(dir, name + "_" + String(n++).padStart(2, "0") + ".pdf");
    draft.files.push({ path, item: item.id, revision: ref(revs[revs.length - 1]) });
  }
  draft.folders = draft.folders.filter((d) => low(d) !== low(dir));
  if (dir) closed.delete(dir + "/");
  $("add-list").querySelectorAll("input:checked").forEach((b) => { b.checked = false; });
  $("add-dialog").close();
  choices();
  changed();
};
$("new-folder").onclick = () => newFolder(target());
// Around the rows, as in Finder: a click picks nothing, so what is made
// next goes to the top, and a right click offers what goes there.
$("tree").addEventListener("click", (event) => { if (!event.target.closest(".ft-row") && picked) { picked = ""; draw(); } });
$("tree").addEventListener("contextmenu", (event) => { if (!event.target.closest(".ft-row") && draft) {
  picked = "";
  $("tree").querySelectorAll(".ft-row.selected").forEach((r) => r.classList.remove("selected"));
  details();
  menu(event, "");
}  });
// Keys on a picked row, as in Finder: Enter renames, Cmd-Delete removes,
// Shift-Cmd-N makes a folder.
$("tree").addEventListener("keydown", (event) => {
  if (!draft || renaming) return;
  const cmd = event.metaKey || event.ctrlKey;
  if (event.key === "Enter" && picked && !cmd) { event.preventDefault(); event.stopPropagation(); rename(picked); }
  else if (event.key === "Backspace" && cmd && picked) { event.preventDefault(); remove(picked); }
  else if (event.key.toLowerCase() === "n" && cmd && event.shiftKey) { event.preventDefault(); newFolder(target()); }
}, true);
// A PDF or folder dropped beside the folders goes to the top.
$("tree").addEventListener("dragover", (event) => { if (dragging) { event.preventDefault(); $("tree").classList.add("drop"); } });
$("tree").addEventListener("dragleave", (event) => { if (event.target === $("tree")) $("tree").classList.remove("drop"); });
$("tree").addEventListener("drop", (event) => {
  event.preventDefault();
  $("tree").classList.remove("drop");
  const from = event.dataTransfer.getData("text/plain") || dragging;
  if (from) tried(relocate(from, baseOf(isFolder(from) ? from.slice(0, -1) : from)));
});
$("form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    await post("/api/snapshots", { snapshot: { ...draft, folders: prune(draft) }, previous: editing });
    const name = draft.name;
    await reload();
    open(name);
    say($("form-message"), "Saved snapshots/" + name + ".yaml.");
  } catch (err) {
    say($("form-message"), err.message, true);
  }
});
$("take-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    const taken = await post("/api/snapshots/take", { name: $("take-name").value.trim(), about: $("take-about").value.trim(), rule: $("take-rule").value });
    await reload();
    open(taken.name);
  } catch (err) {
    say($("take-message"), err.message, true);
  }
});
$("new").onclick = () => leave() && open("");
$("delete").onclick = async () => {
  const s = snapshots.find((x) => x.name === editing);
  if (!s) return;
  if (s.used.length) return say($("form-message"), "In " + s.used.join(", ") + ": take it out of them on the Outlines page first.", true);
  if (!confirm("Delete the Snapshot " + s.name + "? Its file moves into the tree's trash; the Items and anything exported stay.")) return;
  try {
    await post("/api/snapshots/delete", { name: s.name });
    await reload();
    draft = null;
    open(snapshots.length ? snapshots[0].name : "");
  } catch (err) {
    say($("form-message"), err.message, true);
  }
};
window.addEventListener("beforeunload", (event) => { if (draft && text(draft) !== saved) event.preventDefault(); });

reload().then(() => {
  const wanted = decodeURIComponent(location.hash.slice(1));
  if (wanted.startsWith("take=")) {
    open("");
    $("take-rule").value = wanted.slice(5);
    $("take-name").value = wanted.slice(5) + "-" + new Date().toISOString().slice(0, 10);
    return;
  }
  open(snapshots.some((s) => s.name === wanted) ? wanted : snapshots.length ? snapshots[0].name : "");
}).catch((err) => {
  state.error = err.message;
  frame(state);
});
