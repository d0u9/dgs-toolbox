// Snapshots: a fixed subtree of PDFs, each at its path with the revision it
// had. One is taken from a rule — what the rule places now — or begun
// empty, and edited by hand after in its tree: folders made, PDFs added to
// the folder picked, dragged into another, renamed, set to another revision
// or taken out. An Outline puts it in its tree as a folder of its name; the
// Outlines page says where.
import { $, api, el, loadState, post, label, frame, say } from "/common.js";
import { fileTree } from "/ui/filetree.js";
import { resizable } from "/outlinetree.js";

let state = { templates: [], items: [] };
let snapshots = [];
let rules = [];
let editing = null; // the saved name of the Snapshot shown
let draft = null; // the Snapshot as edited: name, about, files, folders
let saved = "";
let picked = ""; // the file's path, or a folder's with a trailing "/"
let dragging = ""; // the path being dragged, as picked is
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
  $("add-to").textContent = target() ? target() + "/" : "the top";
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
  }) : el("p", { className: "muted outline-empty" }, "No PDF yet: add one."));
  details();
}

// reveal shows a PDF of the Snapshot in Finder, as its revision has it.
function reveal(f) {
  const r = (itemOf(f.item)?.revisions || []).find((x) => ref(x) === f.revision);
  post("/api/reveal", { item: f.item, digest: r ? r.digest : "" }).catch((err) => alert(err.message));
}

function lostOf() {
  return new Map(((snapshots.find((s) => s.name === editing) || {}).lost || []).map((l) => [l.item + " " + l.revision, l.why]));
}

// renamer is a name field that renames path when it is left or Enter is
// pressed.
function renamer(path) {
  const folder = isFolder(path);
  const old = folder ? path.slice(0, -1) : path;
  const input = el("input", { className: "mono", value: baseOf(old), spellcheck: false, autocomplete: "off" });
  let gone = false; // renamed already: the field is being redrawn
  const done = () => {
    const name = input.value.trim();
    if (gone || name === baseOf(old)) return;
    gone = true;
    if (name.includes("/")) { input.value = baseOf(old); gone = false; return tried("A name holds no /: drag it into a folder instead"); }
    const why = relocate(path, join(dirOf(old), name));
    if (why) { input.value = baseOf(old); gone = false; }
    tried(why);
  };
  input.addEventListener("change", done);
  input.addEventListener("keydown", (event) => { if (event.key === "Enter") { event.preventDefault(); done(); } });
  return el("label", { className: "vf" }, el("span", { className: "vf-label" }, "Name"), input);
}

// details draws the controls for what is picked in the tree.
function details() {
  const box = $("picked");
  if (!picked) {
    $("picked-head").textContent = "Selected";
    return box.replaceChildren(el("p", { className: "muted" }, "Pick a PDF or folder in the tree to rename, move or remove it."));
  }
  if (isFolder(picked)) {
    const old = picked.slice(0, -1);
    const inside = draft.files.filter((f) => within(f.path, old));
    $("picked-head").textContent = "Folder";
    return box.replaceChildren(el("p", { className: "mono muted" }, old + "/"), renamer(picked),
      el("div", { className: "checks" },
        el("span", { className: "muted" }, inside.length + (inside.length === 1 ? " PDF" : " PDFs") + " in it"),
        el("button", { type: "button", className: "button small-button", textContent: "Remove folder", onclick: () => {
          if (inside.length && !confirm("Take the folder " + old + " and its " + inside.length + " PDFs out of the Snapshot? Their Items stay.")) return;
          draft.files = draft.files.filter((f) => !within(f.path, old));
          draft.folders = draft.folders.filter((d) => low(d) !== low(old) && !within(d, old));
          picked = dirOf(old) ? dirOf(old) + "/" : "";
          if (!folders().includes(dirOf(old)) && dirOf(old)) draft.folders.push(dirOf(old));
          changed();
        } })));
  }
  const f = draft.files.find((x) => x.path === picked);
  const item = itemOf(f.item);
  const revs = item ? item.revisions || [] : [];
  const choice = el("select", { title: "The revision kept", onchange: () => { f.revision = choice.value; changed(); } },
    ...revs.map((r, n) => el("option", { value: ref(r), selected: ref(r) === f.revision },
      "revision " + (n + 1) + (ref(r) === ref(revs[revs.length - 1] || {}) ? " (latest)" : "") + (r.added ? " · " + r.added.slice(0, 10) : ""))),
    ...(revs.some((r) => ref(r) === f.revision) ? [] : [el("option", { value: f.revision, selected: true }, "gone")]));
  const why = lostOf().get(f.item + " " + f.revision);
  $("picked-head").textContent = "PDF";
  box.replaceChildren(el("p", { className: "mono muted" }, f.path), renamer(picked),
    el("label", { className: "vf" }, el("span", { className: "vf-label" }, "Revision"), choice),
    el("p", {}, item ? el("a", { href: api("/browse/") + "#" + f.item }, label(state, item)) : f.item,
      why ? el("span", { className: "message error" }, " · " + why) : null),
    el("div", { className: "checks" }, el("button", { type: "button", className: "button small-button", textContent: "Remove", onclick: () => {
      const dir = dirOf(f.path);
      draft.files = draft.files.filter((x) => x !== f);
      if (dir && !folders().includes(dir)) draft.folders.push(dir);
      picked = dir ? dir + "/" : "";
      changed();
    } })));
}

// filters fills each filter with the values the Items have, keeping what
// is chosen.
function filters() {
  const fill = (id, all, values) => {
    const was = $(id).value;
    $(id).replaceChildren(el("option", { value: "" }, all), ...[...new Set(values.filter(Boolean))].sort().map((v) => el("option", { value: v }, v)));
    $(id).value = [...$(id).options].some((o) => o.value === was) ? was : "";
  };
  fill("add-type", "Any type", state.items.map((i) => i.type));
  fill("add-tag", "Any tag", state.items.flatMap((i) => i.tags || []));
  fill("add-field", "Any field", state.items.filter(ofType).flatMap((i) => Object.keys(i.fields)));
  const key = $("add-field").value;
  fill("add-value", key ? "Any " + key : "Pick a field first", key ? state.items.filter(ofType).map((i) => i.fields[key]) : []);
  $("add-value").disabled = !key;
}

const ofType = (i) => !$("add-type").value || i.type === $("add-type").value;

// choices lists the Items to add, narrowed by the search words, which match
// any field, and by the filters, each ticked to add; an Item the Snapshot
// has already says so.
function choices() {
  const words = $("add-search").value.toLowerCase().split(/\s+/).filter(Boolean);
  const have = new Set(draft ? draft.files.map((f) => f.item) : []);
  const [type, tag, key, value] = ["add-type", "add-tag", "add-field", "add-value"].map((id) => $(id).value);
  const kept = new Set([...$("add-list").querySelectorAll("input:checked")].map((b) => b.value));
  const sorted = state.items.filter((i) => (i.revisions || []).length)
    .filter((i) => (!type || i.type === type) && (!key || (value ? i.fields[key] === value : !!i.fields[key])) &&
      (!tag || (i.tags || []).includes(tag)) && ($("add-retired").checked || !i.retired) && (!$("add-fresh").checked || !have.has(i.id)))
    .map((i) => ({ i, name: label(state, i), all: [i.type, ...Object.values(i.fields), ...(i.tags || [])].join(" ").toLowerCase() }))
    .filter(({ all }) => words.every((w) => all.includes(w)))
    .sort((a, b) => a.name.localeCompare(b.name));
  $("add-list").replaceChildren(...(sorted.length ? sorted.map(({ i, name }) => el("label", { className: "pick" },
    el("input", { type: "checkbox", value: i.id, checked: kept.has(i.id) }), " ", el("span", {}, name),
    i.retired ? el("span", { className: "muted" }, " · retired") : null,
    have.has(i.id) ? el("span", { className: "muted" }, " · in it") : null)) : [el("p", { className: "muted" }, "No Item matches.")]));
  ticked();
}

function ticked() {
  const boxes = [...$("add-list").querySelectorAll("input")];
  const n = boxes.filter((b) => b.checked).length;
  $("add-all").checked = !!boxes.length && n === boxes.length;
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
for (const id of ["add-tag", "add-value", "add-fresh", "add-retired"]) $(id).addEventListener("change", choices);
for (const id of ["add-type", "add-field"]) $(id).addEventListener("change", () => { filters(); choices(); });
$("add-all").addEventListener("change", () => {
  $("add-list").querySelectorAll("input").forEach((b) => { b.checked = $("add-all").checked; });
  ticked();
});
$("add-list").addEventListener("change", ticked);
$("add").onclick = () => {
  const dir = target();
  for (const box of $("add-list").querySelectorAll("input:checked")) {
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
  choices();
  changed();
};
$("new-folder").onclick = () => {
  const dir = target();
  let path = join(dir, "New folder"), n = 2;
  while (taken(path)) path = join(dir, "New folder " + n++);
  draft.folders.push(path);
  if (dir) closed.delete(dir + "/");
  picked = path + "/";
  changed();
  $("picked").querySelector("input")?.select();
};
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
