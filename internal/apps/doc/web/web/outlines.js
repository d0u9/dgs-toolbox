// Outlines: making an Outline — its name, the folder it is exported to, the
// rules that place PDFs in its tree and the Snapshots it puts in as
// folders — with the tree they make redrawn beside the form as it changes.
// Rules are made on the Rules page and Snapshots on the Snapshots page;
// both are the tree's, and several Outlines use one. Looking through the
// result and exporting it is the Explore page's.
import { $, api, el, loadState, post, label, frame, say } from "/common.js";
import { openFile } from "/ui/filedialog.js";
import { outlineTree, unplaced, resizable } from "/outlinetree.js";

let state = { templates: [], items: [] };
let outlines = [];
let rules = []; // every rule in the tree
let snapshots = []; // every Snapshot in the tree
let editing = null; // the saved name of the Outline shown, or "" for a new one
let draft = null; // the Outline as the form has it
let saved = ""; // the Outline as last loaded or saved, to tell an edit
let grouping = null; // the server's answer for the draft
const tree = outlineTree($("tree"), () => state, { empty: () => "Pick a rule or add a Snapshot to see the tree." });
resizable(document.querySelector(".outline-main"), "dgs-doc-outlines-tree");

const blank = () => ({ name: "", about: "", folder: "", rules: [], snapshots: [] });
const copy = (o) => JSON.parse(JSON.stringify(o));
const text = (o) => JSON.stringify({ name: o.name, about: o.about || "", folder: o.folder || "",
  rules: o.rules.map((r) => r.name), snapshots: o.snapshots.map((m) => ({ name: m.name, at: m.at || "" })) });

function list() {
  $("count").textContent = outlines.length;
  const row = (name, sub, selected, onclick) => el("li", { className: selected ? "selected" : "", onclick },
    el("span", { className: "template-name" }, name), el("span", { className: "template-sub" }, sub));
  const parts = (o) => [...o.rules.map((r) => r.name), ...(o.snapshots || []).map((m) => m.name)].join(", ");
  $("outlines").replaceChildren(...outlines.map((o) => row(o.name, o.about || parts(o), o.name === editing, () => leave() && open(o.name))),
    ...(editing === "" ? [row("new outline", "not saved yet", true)] : []));
}

// sync takes the form into the draft.
function sync() {
  draft.name = $("name").value.trim();
  draft.about = $("about").value.trim();
  draft.rules = rules.filter((r) => $("rules").querySelector(`input[value="${CSS.escape(r.name)}"]`)?.checked);
  draft.snapshots = [...$("mounts").querySelectorAll(".mount")].map((row) => ({ name: row.dataset.name, at: row.querySelector("input").value.trim().replace(/^\/+|\/+$/g, "") }));
}

// fill draws the draft's rules and Snapshots into the form.
function fill() {
  const on = new Set(draft.rules.map((r) => r.name));
  $("rules").replaceChildren(...(rules.length ? rules.map((r) => el("label", { className: "pick", title: r.layout },
    el("input", { type: "checkbox", value: r.name, checked: on.has(r.name) }), " ", el("span", { className: "mono" }, r.name),
    " ", el("a", { href: api("/rules/") + "#" + encodeURIComponent(r.name), className: "muted", textContent: "edit" }))) :
    [el("p", { className: "muted" }, "No rule yet.")]));
  $("mounts").replaceChildren(...draft.snapshots.map((m) => {
    const s = snapshots.find((x) => x.name === m.name);
    const row = el("div", { className: "mount checks" },
      el("span", { className: "mono mount-name", title: s ? (s.about || "") : "No Snapshot of this name" }, m.name),
      el("span", { className: "muted" }, " in "),
      el("input", { className: "mono", value: m.at || "", placeholder: "the top", spellcheck: false, autocomplete: "off",
        title: "The folder it goes in; it becomes a folder of its name there" }),
      el("button", { type: "button", className: "button small-button", textContent: "Remove",
        onclick: () => { sync(); draft.snapshots = draft.snapshots.filter((x) => x.name !== m.name); fill(); changed(); } }),
      s ? null : el("span", { className: "message error" }, " not in the tree"));
    row.dataset.name = m.name;
    return row;
  }));
  const free = snapshots.filter((s) => !draft.snapshots.some((m) => m.name === s.name) && !rules.some((r) => r.name === s.name));
  $("add-mount").replaceChildren(el("option", { value: "" }, free.length ? "+ Snapshot…" : "No other Snapshot"),
    ...free.map((s) => el("option", { value: s.name }, s.name + " · " + s.files.length + " PDFs" + (s.about ? " · " + s.about : ""))));
  $("add-mount").disabled = !free.length;
}

function folderShown() {
  $("folder-path").textContent = draft.folder ? "‎" + draft.folder + "‎" : "None: chosen when exporting";
  $("folder-path").classList.toggle("muted", !draft.folder);
  $("folder-clear").disabled = !draft.folder;
}

// leave asks before an unsaved edit is dropped.
function leave() {
  if (editing === null) return true;
  sync();
  return text(draft) === saved || confirm("Drop the unsaved changes to this Outline?");
}

function open(name) {
  editing = name;
  draft = copy(outlines.find((x) => x.name === name) || blank());
  draft.snapshots ||= [];
  delete draft.default;
  $("name").value = draft.name;
  $("about").value = draft.about || "";
  folderShown();
  fill();
  sync();
  saved = text(draft);
  tree.reset();
  history.replaceState(null, "", name ? "#" + encodeURIComponent(name) : location.pathname + location.search);
  $("title").textContent = name || "New Outline";
  $("delete").hidden = !name;
  $("explore").hidden = !name;
  $("explore").href = api("/explore/") + "#" + encodeURIComponent(name);
  say($("message"), "");
  say($("form-message"), "");
  list();
  changed();
}

let pending = 0;
let asked = 0;
// changed regroups the draft a moment after the last change.
function changed() {
  sync();
  $("dirty").hidden = editing === null || text(draft) === saved;
  clearTimeout(pending);
  pending = setTimeout(regroup, 200);
}

async function regroup() {
  const mine = ++asked;
  if (!draft.rules.length && !draft.snapshots.length) {
    grouping = null;
    return draw();
  }
  try {
    const answer = await post("/api/outlines/group", { ...copy(draft), name: draft.name || "preview" });
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

function draw() {
  const root = grouping && grouping.root;
  $("total").textContent = root ? root.count + (root.count === 1 ? " PDF" : " PDFs") : "";
  tree.show(grouping);
  problems();
}

// problems lists what the Outline cannot place: a rule's PDF is fixed in
// the rule, or its Item, on their own pages; a clash between a rule and a
// Snapshot is fixed by moving one of them.
function problems() {
  const lost = unplaced(grouping);
  if (!lost.length) return $("unplaced").replaceChildren();
  const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
  const isRule = (name) => draft.rules.some((r) => r.name === name);
  $("unplaced").replaceChildren(el("details", { className: "problem", open: lost.length <= 8 },
    el("summary", {}, el("strong", {}, lost.length + " PDF" + (lost.length === 1 ? " is" : "s are") + " not placed: nothing is exported until they are")),
    el("ul", { className: "fill-list" }, ...lost.map((m) => el("li", {},
      m.view ? el("a", { href: api(isRule(m.view) ? "/rules/" : "/snapshots/") + "#" + encodeURIComponent(m.view), className: "mono" }, m.view) : null,
      m.view ? " " : null,
      el("a", { href: api("/browse/") + "#" + m.item }, byId[m.item] ? label(state, byId[m.item]) : m.item),
      " " + m.why)))));
}

async function reload() {
  state = await loadState();
  frame(state);
  const [answer, snaps] = await Promise.all([(await fetch(api("/api/outlines"))).json(), (await fetch(api("/api/snapshots"))).json()]);
  outlines = answer.outlines;
  rules = answer.rules || [];
  snapshots = snaps.snapshots || [];
  const error = answer.error || snaps.error;
  if (error) { $("error").hidden = false; $("error").textContent = error; }
  return answer;
}

$("form").addEventListener("input", () => changed());
$("add-mount").onchange = () => {
  const name = $("add-mount").value;
  if (!name) return;
  sync();
  draft.snapshots.push({ name, at: "" });
  fill();
  changed();
};
$("form").addEventListener("submit", async (event) => {
  event.preventDefault();
  sync();
  try {
    await post("/api/outlines", { outline: draft, previous: editing || "" });
    const name = draft.name;
    await reload();
    open(name);
    say($("form-message"), "Saved outlines/" + name + ".yaml.");
  } catch (err) {
    say($("form-message"), err.message, true);
  }
});
$("folder").onclick = async () => {
  const chosen = await openFile({ title: "Export " + (draft.name || "the Outline") + " to", folders: true, writable: true, confirm: "Choose",
    message: "The folder every machine exports this Outline to unless another is chosen on Explore. It is kept in the Outline's file.",
    folder: draft.folder || state.root, fallbacks: [state.root, ""] });
  if (!chosen) return;
  draft.folder = Array.isArray(chosen) ? chosen[0] : chosen;
  folderShown();
  changed();
};
$("folder-clear").onclick = () => { draft.folder = ""; folderShown(); changed(); };
$("new").onclick = () => leave() && open("");
$("delete").onclick = async () => {
  if (!editing || !confirm("Delete the Outline " + editing + "? Its file under outlines/ is removed; its rules and Snapshots stay, no Item changes, and nothing exported is touched.")) return;
  try {
    await post("/api/outlines/delete", { name: editing });
    await reload();
    editing = null;
    open(outlines.length ? outlines[0].name : "");
  } catch (err) {
    say($("form-message"), err.message, true);
  }
};
window.addEventListener("beforeunload", (event) => { if (editing !== null) { sync(); if (text(draft) !== saved) event.preventDefault(); } });

reload().then((answer) => {
  const wanted = decodeURIComponent(location.hash.slice(1));
  open(wanted === "new" ? "" : outlines.some((o) => o.name === wanted) ? wanted : outlines.length ? outlines[0].name : "");
  if (answer.migrated && answer.migrated.length) say($("message"), "Brought up to date: " + answer.migrated.join(", ") + ". Rules are under rules/, their layouts written the way they are now; old Views and Targets, if any, are in migrated/.");
}).catch((err) => {
  state.error = err.message;
  frame(state);
});
