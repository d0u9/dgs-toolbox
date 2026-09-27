// Explore: an Outline's tree, browsed as in a file manager. Its rules' PDFs
// and its Snapshots, each Snapshot a folder of its name, every folder
// counting the PDFs beneath it; picking a folder lists its PDFs beside the
// tree, picking a PDF opens it there. Export writes the same tree into a
// folder you choose. Outlines are made on the Outlines page, from rules
// and Snapshots made on theirs; nothing here changes one.
import { $, api, el, loadState, post, frame, say, planNodes, showPreview, clearPreview, label, outlineFolder, rememberOutlineFolder } from "/common.js";
import { openFile } from "/ui/filedialog.js";
import { outlineTree, find, findFile, folderFiles, pdfRow, unplaced, UNPLACED, resizable } from "/outlinetree.js";

let state = { templates: [], items: [] };
let outlines = [];
let shown = null; // the Outline drawn
let target = null; // what Export writes: shown
let grouping = null;
let picture = null; // the PDF previewed

const tree = outlineTree($("tree"), () => state, { onPick: files, empty: () =>
  shown && !shown.rules.length && !(shown.snapshots || []).length ? "The Outline has no rule and no Snapshot yet." : "The Outline selects no PDFs." });
resizable(document.querySelector(".outline-main"), "dgs-doc-explore-tree", { after: false });

function list() {
  const parts = (o) => o.rules.length + (o.rules.length === 1 ? " rule" : " rules") +
    ((o.snapshots || []).length ? " · " + o.snapshots.length + (o.snapshots.length === 1 ? " Snapshot" : " Snapshots") : "");
  $("count").textContent = outlines.length;
  $("outlines").replaceChildren(...outlines.map((o) => el("li", { className: o === shown ? "selected" : "", onclick: () => open(o) },
    el("span", { className: "template-name" }, o.name), el("span", { className: "badge" }, parts(o)),
    el("span", { className: "template-sub" }, o.about || ""))));
  $("no-outlines").hidden = outlines.length > 0;
}

async function open(o) {
  shown = target = o;
  history.replaceState(null, "", "#" + encodeURIComponent(o.name));
  $("title").textContent = o.name;
  $("edit").href = api("/outlines/") + "#" + encodeURIComponent(o.name);
  $("export-button").disabled = !o.rules.length && !(o.snapshots || []).length;
  tree.reset();
  list();
  try {
    grouping = await post("/api/outlines/group", o);
    say($("message"), "");
  } catch (err) {
    grouping = null;
    say($("message"), err.message, true);
  }
  $("total").textContent = grouping ? grouping.root.count + (grouping.root.count === 1 ? " PDF" : " PDFs") : "";
  tree.show(grouping);
  files(tree.picked);
}

// files lists what is picked: a PDF, a folder's PDFs, or the whole Outline's.
function files(picked) {
  const root = tree.root;
  if (picked === UNPLACED) {
    preview(null);
    const lost = unplaced(grouping);
    $("folder").textContent = "Not placed";
    $("folder-count").textContent = lost.length;
    $("files").replaceChildren(el("li", { className: "outline-note muted" }, "Nothing is exported until these are placed: fill the field on Browse, change the rule on the Rules page, or the Snapshot on the Snapshots page."),
      ...lost.map((m) => pdfRow(state, { ...m, path: "" }, el("div", { className: "template-sub" }, (m.view ? m.view + ": " : "") + m.why))));
    return;
  }
  const file = findFile(root, picked);
  preview(file);
  if (file) return;
  const node = find(root, picked);
  $("folder").textContent = picked ? picked.split("/").pop() : "All PDFs";
  $("folder-count").textContent = node ? node.count : "";
  $("files").replaceChildren(...(node && node.snapshot ? [el("li", { className: "outline-note muted" }, "The Snapshot ",
    el("a", { href: api("/snapshots/") + "#" + encodeURIComponent(node.snapshot) }, node.snapshot), ": fixed as it was taken.")] : []),
    ...(node ? folderFiles(state, node) : []));
}

// preview shows the PDF picked beside the tree, or nothing.
function preview(file) {
  const on = !!(file && file.digest);
  picture = on ? file : null;
  document.body.classList.toggle("previewing", on);
  $("preview").hidden = !on;
  if (!on) return clearPreview();
  const item = state.items.find((i) => i.id === file.item);
  $("reader-title").textContent = file.path.split("/").pop() + (item ? " · " + label(state, item) : "");
  $("reader-browse").href = api("/browse/") + "#" + file.item;
  const query = { item: file.item, digest: file.digest };
  showPreview(query, api("/api/revision?" + new URLSearchParams(query)));
}

$("reveal").addEventListener("click", async () => {
  if (!picture) return;
  try { await post("/api/reveal", { item: picture.item, digest: picture.digest }); }
  catch (err) { say($("message"), err.message, true); }
});

// The Outlines list folds away, leaving the tree and the PDF beside it.
const LIST_KEY = "dgs-doc-explore-list";
function fold(shut) {
  document.body.classList.toggle("list-shut", shut);
  $("open-list").hidden = !shut;
  try { localStorage.setItem(LIST_KEY, shut ? "shut" : ""); } catch { /* not kept */ }
}
$("shut-list").addEventListener("click", () => fold(true));
$("open-list").addEventListener("click", () => fold(false));
try { fold(localStorage.getItem(LIST_KEY) === "shut"); } catch { /* open */ }

// Export writes what is picked, the Outline or a Snapshot in it, into a
// folder chosen in the dialog: this browser's last one, else the Outline's
// own, is offered. The server checks the whole run before anything is
// written; a run that does not check is listed beside the tree.
$("export-button").addEventListener("click", async () => {
  const t = target;
  const chosen = await openFile({ title: "Export " + t.name + " to", folders: true, writable: true, confirm: "Export",
    message: "Writes " + t.name + " into this folder. An export removes only files it wrote there itself.",
    folder: outlineFolder(state, t) || state.root, fallbacks: [state.root, ""] });
  if (!chosen) return;
  const where = Array.isArray(chosen) ? chosen[0] : chosen;
  rememberOutlineFolder(state, t.name, where === t.default ? "" : where);
  const body = { outlines: [t.name], folders: { [t.name]: where } };
  say($("message"), "Checking " + where + "…");
  try {
    const plan = await post("/api/export/plan", body);
    const count = (k) => plan.jobs.reduce((n, j) => n + j.plan[k].length, 0);
    const [add, replace, remove] = [count("add"), count("replace"), count("remove")];
    if (!plan.ready) {
      preview(null);
      $("folder").textContent = "Not exported";
      $("folder-count").textContent = "";
      $("files").replaceChildren(el("li", {}, ...planNodes(state, plan)));
      return say($("message"), "Not exported: fix what is listed first. Nothing was written.", true);
    }
    if (!add && !replace && !remove) return say($("message"), where + " is up to date: nothing to write.");
    if ((replace || remove) && !confirm(`Export ${t.name} into ${where}?\n\n${add} to add, ${replace} to replace, ${remove} to remove. Only files an export wrote there are replaced or removed.`)) {
      return say($("message"), "Not exported.");
    }
    say($("message"), "Exporting: copying and reading back…");
    const r = (await post("/api/export", body)).results[0];
    say($("message"), "Exported to " + where + ": " + r.result.written + " written, " + r.result.removed + " removed, " + r.result.kept + " unchanged" +
      (r.history_error ? " (history not recorded: " + r.history_error + ")" : "") + ".");
  } catch (err) {
    say($("message"), err.message, true);
  }
});

(async () => {
  state = await loadState();
  frame(state);
  const answer = await (await fetch(api("/api/outlines"))).json();
  outlines = answer.outlines;
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
  for (const a of document.querySelectorAll('a[href^="/outlines/"]')) a.href = api("/outlines/") + a.hash;
  const wanted = decodeURIComponent(location.hash.slice(1));
  const o = outlines.find((x) => x.name === wanted) || outlines[0];
  if (o) open(o);
})().catch((err) => {
  state.error = err.message;
  frame(state);
});
