// Merge: bring a sub-tree, or an export read back, into this tree. The
// server plans; the page shows the plan, takes a choice for each conflict,
// and asks the server to merge, which plans again before writing.
import { $, el, loadState, post, label, frame, say } from "/common.js";
import { openFile } from "/ui/filedialog.js";

let state = { templates: [], items: [] };
let dir = "";
let plan = null;
const choices = {};

const short = (digest) => digest ? digest.slice(0, 12) : "none";
const fields = (f) => Object.entries(f || {}).map(([k, v]) => k + ": " + v).join("  ") || "no fields";
const named = (id) => {
  const item = state.items.find((i) => i.id === id);
  return item ? el("a", { href: "/browse/#" + id }, label(state, item)) : id;
};
const revision = (id, digest) => {
  const item = state.items.find((i) => i.id === id);
  const n = item ? item.revisions.findIndex((r) => r.digest === digest) : -1;
  return (n >= 0 ? "revision " + (n + 1) : "a revision new here") + " (" + short(digest) + ")";
};

function section(title, rows) {
  return rows.length ? el("section", { className: "merge-section" },
    el("h2", { className: "panel-head" }, title + " ", el("span", { className: "count numeric" }, String(rows.length))),
    el("ul", { className: "merge-rows" }, ...rows)) : null;
}

function side(conflict, which) {
  const value = which === "ours" ? conflict.ours : conflict.theirs;
  switch (conflict.kind) {
    case "fields": {
      // Only the keys the two sides hold differently.
      const other = which === "ours" ? conflict.theirs : conflict.ours;
      const keys = [...new Set([...Object.keys(value || {}), ...Object.keys(other || {})])].filter((k) => (value || {})[k] !== (other || {})[k]).sort();
      return keys.map((k) => k + ": " + ((value || {})[k] ?? "(none)")).join("  ");
    }
    case "head": return "HEAD at " + revision(conflict.item, value);
    default: return el("pre", { className: "mono" }, JSON.stringify(value, null, 2));
  }
}

function conflictRow(c) {
  const title = c.kind === "template" ? "Template " + c.type
    : c.kind === "view" ? "View " + c.name
    : [named(c.item), c.kind === "fields" ? " — fields differ" : " — HEAD moved on both sides"];
  const option = (which, text) => el("label", { className: "merge-choice" },
    el("input", { type: "radio", name: c.id, value: which, checked: choices[c.id] === which,
      onchange: () => { choices[c.id] = which; ready(); } }),
    el("span", {}, el("strong", {}, text), " ", side(c, which)));
  return el("li", {}, el("div", {}, ...[].concat(title)),
    option("ours", "Keep this tree's:"), option("theirs", "Take theirs:"));
}

function draw() {
  $("dir").textContent = dir ? dir + (plan ? " · " + (plan.from === "tree" ? "a sub-tree" : "an export Target") : "") : "";
  if (!plan) { $("plan").replaceChildren(); ready(); return; }
  const parts = [
    section("Cannot merge", plan.problems.map((p) => el("li", { className: "error" }, p))),
    section("Conflicts — choose a side", plan.conflicts.map(conflictRow)),
    section("New Items", plan.new.map((c) => el("li", {}, c.type + " · " + fields(c.fields),
      el("span", { className: "sub" }, c.digests.length + (c.digests.length === 1 ? " PDF" : " PDFs"))))),
    section("Changed Items", plan.changed.map((c) => el("li", {}, named(c.item),
      el("span", { className: "sub" }, [
        c.digests && c.digests.length ? c.digests.length + " new revision" + (c.digests.length === 1 ? "" : "s") : "",
        c.head ? "HEAD moves to " + short(c.head) : "",
        c.notes ? "notes added" : "",
      ].filter(Boolean).join(" · "))))),
    section("New Templates", plan.templates.map((t) => el("li", {}, t))),
    section("New Views", plan.views.map((v) => el("li", {}, v))),
    plan.same ? el("p", { className: "muted merge-intro" }, plan.same + " Item" + (plan.same === 1 ? " is" : "s are") + " already here as they are") : null,
  ].filter(Boolean);
  $("plan").replaceChildren(...parts);
  ready();
}

function ready() {
  const nothing = plan && !plan.new.length && !plan.changed.length && !plan.templates.length && !plan.views.length && !plan.conflicts.length;
  const open = plan ? plan.conflicts.filter((c) => !choices[c.id]).length : 0;
  $("merge").disabled = !plan || plan.problems.length > 0 || open > 0 || nothing;
  if (!plan) return;
  if (plan.problems.length) say($("message"), "Resolve what cannot be merged in the folder first.", true);
  else if (nothing) say($("message"), "Nothing to merge: this tree already holds all of it.");
  else if (open) say($("message"), open + " conflict" + (open === 1 ? "" : "s") + " left to choose.");
  else say($("message"), "Ready. PDFs are copied in and read back before the sidecars name them.");
}

async function planFor(folder) {
  say($("message"), "Reading…");
  try {
    plan = await post("/api/merge/plan", { dir: folder });
    dir = folder;
    for (const k of Object.keys(choices)) delete choices[k];
  } catch (error) {
    plan = null;
    dir = folder;
    draw();
    say($("message"), error.message, true);
    return;
  }
  draw();
}

$("open").onclick = async () => {
  const chosen = await openFile({
    title: "Open folder to merge",
    message: "Choose a sub-tree or an export Target. It is only read.",
    folders: true,
    folder: dir || state.root,
    fallbacks: [state.root, ""],
  });
  if (chosen) await planFor(Array.isArray(chosen) ? chosen[0] : chosen);
};

$("merge").onclick = async () => {
  $("merge").disabled = true;
  say($("message"), "Merging: copying and reading back…");
  try {
    const result = await post("/api/merge", { dir, choices });
    state = await loadState();
    await planFor(dir);
    say($("message"), "Merged: " + result.items + " Item" + (result.items === 1 ? "" : "s") + " written, " + result.pdfs + " PDF" + (result.pdfs === 1 ? "" : "s") + " copied, " +
      result.templates + " Template" + (result.templates === 1 ? "" : "s") + ", " + result.views + " View" + (result.views === 1 ? "" : "s") + ".");
  } catch (error) {
    say($("message"), error.message, true);
    ready();
  }
};

loadState().then((s) => { state = s; frame(state); draw(); }).catch((err) => { state.error = err.message; frame(state); });
