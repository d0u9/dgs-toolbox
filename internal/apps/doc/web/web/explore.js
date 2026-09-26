// Explore: an Outline's result, or a Snapshot's. An Outline's rules' PDFs
// in the tree they make, each folder counting the PDFs beneath it; picking a
// folder or a PDF lists it beside, and Export writes the same tree into a
// folder you choose. Picking a PDF opens it beside the tree, as a file
// manager does. Snapshot keeps the folder picked, or the whole tree, as it is
// now; a Snapshot is drawn inside its Outline's tree, as a folder beside
// the folder it was taken from, and picking it or anything in it exports
// and deletes the Snapshot as it was taken. Outlines are made on the
// Outlines page; nothing here changes one.
import { $, api, el, loadState, post, frame, say, planNodes, showPreview, clearPreview, label, outlineFolder, rememberOutlineFolder } from "/common.js";
import { openFile } from "/ui/filedialog.js";
import { outlineTree, find, findFile, folderFiles, pdfRow, unplaced, UNPLACED } from "/outlinetree.js";

let state = { templates: [], items: [] };
let outlines = [];
let snapshots = [];
let shown = null; // the Outline drawn, or a Snapshot whose Outline is gone
let target = null; // what Export and Delete act on: shown, or a Snapshot picked in it
let marks = new Map(); // the path of each Snapshot drawn in shown's tree
let said = ""; // shown's own message
let grouping = null;
let picture = null; // the PDF previewed

const isSnapshot = (o) => !!(o && o.taken);
const tree = outlineTree($("tree"), () => state, { onPick: files, empty: () => isSnapshot(shown) ? "Nothing of the Snapshot is left in the tree." :
  shown && !shown.rules.length ? "The Outline has no rule yet." : "The Outline selects no PDFs." });

function list() {
  const item = (o, badge) => el("li", { className: o.name === (shown && shown.name) ? "selected" : "", onclick: () => open(o) },
    el("span", { className: "template-name" }, o.name), el("span", { className: "badge" }, badge),
    el("span", { className: "template-sub" }, o.about || ""));
  $("count").textContent = outlines.length;
  $("outlines").replaceChildren(...outlines.map((o) => item(o, o.rules.length + (o.rules.length === 1 ? " rule" : " rules"))));
  $("no-outlines").hidden = outlines.length > 0;
  // A Snapshot is listed only when its Outline is gone; else it is in the tree.
  const orphans = snapshots.filter((s) => !outlines.some((o) => o.name === s.outline));
  $("snap-count").textContent = orphans.length;
  $("snapshots").replaceChildren(...orphans.map((o) => item(o, o.taken.slice(0, 10))));
  $("snap-head").hidden = !orphans.length;
}

const takenText = (s) => "Taken " + s.taken.replace("T", " ").slice(0, 16) + " from " + s.outline + (s.node ? " / " + s.node : "") +
  (s.lost.length ? ". " + s.lost.length + " of its PDFs are no longer in the tree." : ".");

// open draws o; with snapshot, the Snapshot of that name in it is picked.
async function open(o, snapshot) {
  shown = o;
  target = null;
  marks = new Map();
  $("title").textContent = o.name;
  $("edit").href = api("/outlines/") + "#" + encodeURIComponent(isSnapshot(o) ? o.outline : o.name);
  $("edit").hidden = isSnapshot(o);
  $("take-form").hidden = true;
  tree.reset();
  list();
  if (isSnapshot(o)) {
    grouping = { root: o.root, plans: {}, clashes: [] };
    said = "";
  } else try {
    grouping = await post("/api/outlines/group", o);
    said = "";
  } catch (err) {
    grouping = null;
    said = err.message;
  }
  $("total").textContent = grouping ? grouping.root.count + (grouping.root.count === 1 ? " PDF" : " PDFs") : "";
  const drawn = isSnapshot(o) ? grouping : graft(grouping);
  tree.show(drawn);
  const at = [...marks].find(([, s]) => s.name === snapshot);
  if (at) tree.pick(at[0]);
  files(tree.picked);
}

// graft puts each of shown's Snapshots into a copy of its tree, as a folder
// beside the folder it was taken from, or at the top when that is gone.
function graft(answer) {
  marks = new Map();
  if (!answer) return answer;
  const clone = (n) => ({ ...n, children: n.children.map(clone) });
  const moved = (n, base) => ({ ...n, path: n.path ? base + "/" + n.path : base,
    children: n.children.map((c) => moved(c, base)), files: n.files.map((f) => ({ ...f, path: base + "/" + f.path })) });
  const root = clone(answer.root);
  for (const s of snapshots.filter((s) => s.outline === shown.name)) {
    const at = find(root, s.node) || root;
    const path = (at.path ? at.path + "/" : "") + "@" + s.name;
    at.children = [...at.children, { ...moved(s.root, path), name: s.name, snapshot: s }];
    marks.set(path, s);
  }
  return { ...answer, root };
}

// aim makes t what Export, Delete and the message act on.
function aim(t) {
  if (t === target) return;
  target = t;
  history.replaceState(null, "", "#" + encodeURIComponent(t.name));
  $("take").hidden = isSnapshot(t);
  $("drop").hidden = !isSnapshot(t);
  if (isSnapshot(t)) say($("message"), takenText(t), t.lost.length > 0);
  else say($("message"), said, !!said);
  $("export-button").disabled = !(isSnapshot(t) ? t.files.length : t.rules.length);
}

// files lists what is picked: a PDF, a folder's PDFs, or the whole Outline's.
function files(picked) {
  const root = tree.root;
  const mark = [...marks].find(([p]) => picked === p || picked.startsWith(p + "/"));
  aim(mark ? mark[1] : shown);
  if (picked === UNPLACED) {
    preview(null);
    const lost = unplaced(grouping);
    $("folder").textContent = "Not placed";
    $("folder-count").textContent = lost.length;
    $("files").replaceChildren(el("li", { className: "outline-note muted" }, "The rules cannot place these. Fill the field on Browse, or change the rule on the Outlines page."),
      ...lost.map((m) => pdfRow(state, { ...m, path: "" }, el("div", { className: "template-sub" }, (m.view ? m.view + ": " : "") + m.why))));
    return;
  }
  const file = findFile(root, picked);
  preview(file);
  if (file) {
    $("folder").textContent = "PDF";
    $("folder-count").textContent = "";
    $("files").replaceChildren(pdfRow(state, file, el("div", { className: "template-sub mono" }, file.path)));
    return;
  }
  const node = find(root, picked);
  if ((mark ? picked === mark[0] : !picked) && isSnapshot(target) && target.lost.length) {
    $("folder").textContent = mark ? target.name : "All PDFs";
    $("folder-count").textContent = node ? node.count : "";
    $("files").replaceChildren(...(node ? folderFiles(state, node) : []),
      el("li", { className: "outline-note muted" }, "Lost: the tree no longer has these."),
      ...target.lost.map((l) => pdfRow(state, { item: l.item, path: l.path }, el("div", { className: "template-sub" }, l.why))));
    return;
  }
  $("folder").textContent = mark && picked === mark[0] ? target.name : picked ? picked.split("/").pop() : "All PDFs";
  $("folder-count").textContent = node ? node.count : "";
  $("files").replaceChildren(...(node ? folderFiles(state, node) : []));
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

// Snapshot keeps the folder picked, or the whole tree when none is.
$("take").addEventListener("click", () => {
  const folder = find(grouping && grouping.root, tree.picked) && tree.picked !== UNPLACED ? tree.picked : "";
  $("take-form").hidden = false;
  const slug = (folder ? folder.split("/").pop() : "").toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^[-_]+|[-_]+$/g, "");
  $("take-name").value = (/[a-z]/.test(slug) ? slug : shown.name) + "-" + new Date().toISOString().slice(0, 10);
  $("take-hint").textContent = "Snapshot of " + (folder ? shown.name + " / " + folder : "the whole of " + shown.name) + ": each PDF's revision is kept as it is now.";
  $("take-name").focus();
});
$("take-cancel").addEventListener("click", () => { $("take-form").hidden = true; });
$("take-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const node = find(grouping && grouping.root, tree.picked) && tree.picked !== UNPLACED ? tree.picked : "";
  try {
    const taken = await post("/api/snapshots/take", { outline: shown.name, node, name: $("take-name").value, about: $("take-about").value });
    await loadSnapshots();
    open(shown, taken.name);
  } catch (err) {
    say($("message"), err.message, true);
  }
});
$("drop").addEventListener("click", async () => {
  if (!confirm(`Delete the Snapshot ${target.name}?\n\nIts file moves into the tree's trash. The Items and anything exported stay.`)) return;
  try {
    await post("/api/snapshots/delete", { name: target.name });
    await loadSnapshots();
    const o = isSnapshot(shown) ? outlines[0] || snapshots[0] : shown;
    if (o) open(o); else location.reload();
  } catch (err) {
    say($("message"), err.message, true);
  }
});
async function loadSnapshots() {
  const answer = await (await fetch(api("/api/snapshots"))).json();
  snapshots = answer.snapshots;
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
  list();
}

(async () => {
  state = await loadState();
  frame(state);
  const answer = await (await fetch(api("/api/outlines"))).json();
  outlines = answer.outlines;
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
  if (answer.migrated && answer.migrated.length) say($("message"), "Outlines brought up to date: " + answer.migrated.join(", ") + ". Their rules are under rules/; old Views and Targets, if any, are in migrated/.");
  for (const a of document.querySelectorAll('a[href^="/outlines/"]')) a.href = api("/outlines/") + a.hash;
  const wanted = decodeURIComponent(location.hash.slice(1));
  await loadSnapshots();
  const snap = snapshots.find((x) => x.name === wanted);
  const home = snap && outlines.find((x) => x.name === snap.outline);
  if (home) return open(home, snap.name);
  const o = [...outlines, ...snapshots].find((x) => x.name === wanted) || outlines[0] || snapshots[0];
  if (o) open(o);
})().catch((err) => {
  state.error = err.message;
  frame(state);
});
