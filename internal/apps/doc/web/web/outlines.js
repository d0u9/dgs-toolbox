// Outlines: making the rules an Outline groups Items by, in the form a View
// is made in, with its tree redrawn beside it as the form changes. Looking
// through the result is the Explore page's.
import { $, api, el, loadState, post, label, frame, say } from "/common.js";
import * as which from "/layoutform.js";
import { outlineTree } from "/outlinetree.js";

let state = { templates: [], items: [] };
let outlines = [];
let editing = null; // the saved name of the Outline shown, or "" for a new one
let saved = ""; // the Outline as last loaded or saved, to tell an edit
let grouping = null; // the server's answer for the Outline in the form
const tree = outlineTree($("tree"), { empty: () => current().layout ? "" : "Click a key to add the first level." });

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

function draw() {
  const root = grouping && grouping.root;
  $("total").textContent = root ? root.count + (root.count === 1 ? " PDF" : " PDFs") : "";
  tree.show(grouping);
  unplaced();
}

// unplaced lists the PDFs the levels cannot place. A numbered level an Item
// has a value for lacks only a place in its order, numbered last from here.
function unplaced() {
  const missing = (grouping && grouping.missing) || [];
  if (!missing.length) return $("unplaced").replaceChildren();
  const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
  const numbered = which.numberedKeys();
  $("unplaced").replaceChildren(el("details", { className: "problem", open: missing.length <= 8 },
    el("summary", {}, el("strong", {}, missing.length + " PDF" + (missing.length === 1 ? " is" : "s are") + " not placed")),
    el("ul", { className: "fill-list" }, ...missing.map((m) => {
      const item = byId[m.item];
      const li = el("li", {}, el("a", { href: api("/browse/") + "#" + m.item }, item ? label(state, item) : m.item),
        " lacks ", el("span", { className: "mono" }, m.keys.join(", ")), " ");
      for (const key of m.keys.filter((k) => numbered.includes(k))) {
        const value = item && which.orderValue(item, key);
        if (value) li.append(el("button", { type: "button", className: "small", textContent: "Number " + value + " last",
          onclick: () => which.numberLast(key, value) }));
      }
      return li;
    }))));
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
  open(wanted === "new" ? "" : outlines.some((o) => o.name === wanted) ? wanted : outlines.length ? outlines[0].name : "");
}).catch((err) => {
  state.error = err.message;
  frame(state);
});
