// Snapshots: a fixed subtree of PDFs, each at its path with the revision it
// had. One is taken from a rule — what the rule places now — or begun
// empty, and edited by hand after: a PDF added, moved, set to another
// revision or taken out. An Outline puts it in its tree as a folder of its
// name; the Outlines page says where.
import { $, api, el, loadState, post, label, frame, say } from "/common.js";
import { outlineTree } from "/outlinetree.js";

let state = { templates: [], items: [] };
let snapshots = [];
let rules = [];
let editing = null; // the saved name of the Snapshot shown
let draft = null; // the Snapshot as the form has it
let saved = "";
const tree = outlineTree($("tree"), () => state, { empty: () => "No PDF yet: add one." });

const copy = (o) => JSON.parse(JSON.stringify(o));
const text = (s) => JSON.stringify({ name: s.name, about: s.about || "", files: s.files.map((f) => [f.path, f.item, f.revision]) });
const itemOf = (id) => state.items.find((i) => i.id === id);
const ref = (r) => r.id || r.digest;

function list() {
  $("count").textContent = snapshots.length;
  const row = (name, sub, selected, onclick) => el("li", { className: selected ? "selected" : "", onclick },
    el("span", { className: "template-name" }, name), el("span", { className: "template-sub" }, sub));
  $("snapshots").replaceChildren(...snapshots.map((s) => row(s.name,
    (s.about ? s.about + " · " : "") + (s.used.length ? "in " + s.used.join(", ") : "in no Outline"),
    s.name === editing, () => leave() && open(s.name))),
    ...(editing === "" ? [row("new snapshot", "not taken yet", true)] : []));
}

// nest puts the draft's files into folders, as the server nests a tree.
function nest(files) {
  const root = { name: "", path: "", count: 0, files: [], children: [] };
  for (const f of [...files].sort((a, b) => a.path < b.path ? -1 : 1)) {
    const parts = f.path.split("/");
    let node = root;
    node.count++;
    parts.slice(0, -1).forEach((part, i) => {
      const path = parts.slice(0, i + 1).join("/");
      let c = node.children.find((x) => x.path.toLowerCase() === path.toLowerCase());
      if (!c) node.children.push(c = { name: part, path, count: 0, files: [], children: [] });
      node = c;
      node.count++;
    });
    node.files.push({ ...f, view: draft.name });
  }
  const tidy = (n) => { n.children.sort((a, b) => a.name < b.name ? -1 : 1); n.children.forEach(tidy); };
  tidy(root);
  return root;
}

// sync takes the form into the draft.
function sync() {
  if (!draft) return;
  draft.name = $("name").value.trim();
  draft.about = $("about").value.trim();
  draft.files = [...$("files").querySelectorAll(".snap-file")].map((row) => ({
    path: row.querySelector("input").value.trim().replace(/^\/+/, ""), item: row.dataset.item,
    revision: row.querySelector("select").value, ...(row.dataset.rule ? { rule: row.dataset.rule } : {}) }));
}

// fill draws the draft's files, one row each: its path, its revision, and
// the Item it is.
function fill() {
  const lost = new Map(((snapshots.find((s) => s.name === editing) || {}).lost || []).map((l) => [l.item + " " + l.revision, l.why]));
  $("files").replaceChildren(...draft.files.map((f, i) => {
    const item = itemOf(f.item);
    const revs = item ? item.revisions || [] : [];
    const choice = el("select", { title: "The revision kept" },
      ...revs.map((r, n) => el("option", { value: ref(r), selected: ref(r) === f.revision },
        "revision " + (n + 1) + (ref(r) === ref(revs[revs.length - 1] || {}) ? " (latest)" : "") + (r.added ? " · " + r.added.slice(0, 10) : ""))),
      ...(revs.some((r) => ref(r) === f.revision) ? [] : [el("option", { value: f.revision, selected: true }, "gone")]));
    const why = lost.get(f.item + " " + f.revision);
    const row = el("div", { className: "snap-file" + (why ? " lost" : "") },
      el("input", { className: "mono", value: f.path, spellcheck: false, autocomplete: "off" }),
      choice,
      el("button", { type: "button", className: "button small-button", textContent: "Remove",
        onclick: () => { sync(); draft.files.splice(i, 1); fill(); changed(); } }),
      el("div", { className: "template-sub" }, item ? el("a", { href: api("/browse/") + "#" + f.item }, label(state, item)) : f.item,
        why ? " · " + why : ""));
    row.dataset.item = f.item;
    if (f.rule) row.dataset.rule = f.rule;
    return row;
  }));
  $("file-count").textContent = draft.files.length;
}

// leave asks before an unsaved edit is dropped.
function leave() {
  if (!draft) return true;
  sync();
  return text(draft) === saved || confirm("Drop the unsaved changes to this Snapshot?");
}

function open(name) {
  editing = name;
  const s = snapshots.find((x) => x.name === name);
  $("form").hidden = !s;
  $("take-form").hidden = !!s;
  $("dirty").hidden = true;
  say($("message"), "");
  say($("form-message"), "");
  tree.reset();
  list();
  if (!s) {
    draft = null;
    history.replaceState(null, "", location.pathname + location.search);
    $("title").textContent = "New Snapshot";
    $("total").textContent = "";
    tree.show(null);
    return;
  }
  history.replaceState(null, "", "#" + encodeURIComponent(name));
  draft = copy({ name: s.name, about: s.about || "", taken: s.taken, rule: s.rule, outline: s.outline, node: s.node, folder: s.folder, files: s.files });
  $("name").value = draft.name;
  $("about").value = draft.about;
  $("origin").textContent = "Taken " + s.taken.replace("T", " ").slice(0, 16) + (s.rule ? " from the rule " + s.rule : s.outline ? " from " + s.outline + (s.node ? " / " + s.node : "") : ", begun empty") +
    ". " + (s.used.length ? "In " + s.used.join(", ") + "." : "In no Outline yet: add it on the Outlines page.");
  $("title").textContent = name;
  fill();
  sync();
  saved = text(draft);
  draw();
  if (s.lost.length) say($("message"), s.lost.length + " of its PDFs are no longer in the tree: an Outline with it is not exported until they are removed or replaced.", true);
}

function changed() {
  sync();
  $("dirty").hidden = text(draft) === saved;
  draw();
}

function draw() {
  const root = nest(draft.files.filter((f) => f.path));
  $("total").textContent = root.count + (root.count === 1 ? " PDF" : " PDFs");
  tree.show({ root, plans: {}, clashes: [] });
}

function items() {
  const sorted = [...state.items].sort((a, b) => label(state, a).localeCompare(label(state, b)));
  $("add-item").replaceChildren(el("option", { value: "" }, "Add an Item's latest revision…"),
    ...sorted.filter((i) => (i.revisions || []).length).map((i) => el("option", { value: i.id }, label(state, i))));
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
  items();
}

$("form").addEventListener("input", () => changed());
$("add").onclick = () => {
  const item = itemOf($("add-item").value);
  if (!item) return;
  sync();
  const revs = item.revisions;
  const name = label(state, item).replace(/[\/\\:]+/g, "-") + ".pdf";
  let path = name, n = 2;
  while (draft.files.some((f) => f.path.toLowerCase() === path.toLowerCase())) path = name.replace(/\.pdf$/, "_" + String(n++).padStart(2, "0") + ".pdf");
  draft.files.push({ path, item: item.id, revision: ref(revs[revs.length - 1]) });
  $("add-item").value = "";
  fill();
  changed();
};
$("form").addEventListener("submit", async (event) => {
  event.preventDefault();
  sync();
  try {
    await post("/api/snapshots", { snapshot: draft, previous: editing });
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
window.addEventListener("beforeunload", (event) => { if (draft) { sync(); if (text(draft) !== saved) event.preventDefault(); } });

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
