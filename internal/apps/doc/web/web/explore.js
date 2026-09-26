// Explore: an Outline's result, or a Snapshot's. An Outline's rules' PDFs
// in the tree they make, each folder counting the PDFs beneath it; picking a
// folder or a PDF lists it beside, and Export writes the same tree into a
// folder. Snapshot keeps the folder picked, or the whole tree, as it is
// now; a Snapshot is drawn and exported as it was taken. Outlines are made
// on the Outlines page; nothing here changes one.
import { $, api, el, loadState, post, frame, say, planNodes, outlineFolder, rememberOutlineFolder } from "/common.js";
import { openFile } from "/ui/filedialog.js";
import { outlineTree, find, findFile, folderFiles, pdfRow, unplaced, UNPLACED } from "/outlinetree.js";

let state = { templates: [], items: [] };
let outlines = [];
let snapshots = [];
let shown = null; // the Outline or Snapshot drawn
let grouping = null;
let planned = false; // a check passed and Export may write it

const isSnapshot = (o) => !!(o && o.taken);
const tree = outlineTree($("tree"), () => state, { onPick: files, empty: () => isSnapshot(shown) ? "Nothing of the Snapshot is left in the tree." :
  shown && !shown.rules.length ? "The Outline has no rule yet." : "The Outline selects no PDFs." });

function list() {
  const item = (o, badge) => el("li", { className: o.name === (shown && shown.name) ? "selected" : "", onclick: () => open(o) },
    el("span", { className: "template-name" }, o.name), el("span", { className: "badge" }, badge),
    el("span", { className: "template-sub" }, o.about || ""));
  $("count").textContent = outlines.length;
  $("outlines").replaceChildren(...outlines.map((o) => item(o, o.rules.length + (o.rules.length === 1 ? " rule" : " rules"))));
  $("empty").hidden = outlines.length > 0;
  $("snap-count").textContent = snapshots.length;
  $("snapshots").replaceChildren(...snapshots.map((o) => item(o, o.taken.slice(0, 10))));
  $("snap-empty").hidden = snapshots.length > 0;
}

async function open(o) {
  shown = o;
  history.replaceState(null, "", "#" + encodeURIComponent(o.name));
  $("title").textContent = o.name;
  $("edit").href = api("/outlines/") + "#" + encodeURIComponent(isSnapshot(o) ? o.outline : o.name);
  $("edit").hidden = isSnapshot(o);
  $("take").hidden = isSnapshot(o);
  $("drop").hidden = !isSnapshot(o);
  $("take-form").hidden = true;
  tree.reset();
  list();
  exportShown();
  if (isSnapshot(o)) {
    grouping = { root: o.root, plans: {}, clashes: [] };
    say($("message"), "Taken " + o.taken.replace("T", " ").slice(0, 16) + " from " + o.outline + (o.node ? " / " + o.node : "") +
      (o.lost.length ? ". " + o.lost.length + " of its PDFs are no longer in the tree." : "."), o.lost.length > 0);
  } else try {
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
  const root = grouping && grouping.root;
  if (picked === UNPLACED) {
    const lost = unplaced(grouping);
    $("folder").textContent = "Not placed";
    $("folder-count").textContent = lost.length;
    $("files").replaceChildren(el("li", { className: "outline-note muted" }, "The rules cannot place these. Fill the field on Browse, or change the rule on the Outlines page."),
      ...lost.map((m) => pdfRow(state, { ...m, path: "" }, el("div", { className: "template-sub" }, (m.view ? m.view + ": " : "") + m.why))));
    return;
  }
  const file = findFile(root, picked);
  if (file) {
    $("folder").textContent = "PDF";
    $("folder-count").textContent = "";
    $("files").replaceChildren(pdfRow(state, file, el("div", { className: "template-sub mono" }, file.path)));
    return;
  }
  const node = find(root, picked);
  if (!picked && isSnapshot(shown) && shown.lost.length) {
    $("folder").textContent = "All PDFs";
    $("folder-count").textContent = node ? node.count : "";
    $("files").replaceChildren(...(node ? folderFiles(state, node) : []),
      el("li", { className: "outline-note muted" }, "Lost: the tree no longer has these."),
      ...shown.lost.map((l) => pdfRow(state, { item: l.item, path: l.path }, el("div", { className: "template-sub" }, l.why))));
    return;
  }
  $("folder").textContent = picked ? picked.split("/").pop() : "All PDFs";
  $("folder-count").textContent = node ? node.count : "";
  $("files").replaceChildren(...(node ? folderFiles(state, node) : []));
}

// Export writes the tree shown into the folder chosen: this browser's
// choice, else the Outline's own. The server checks the whole run first.
function exportShown() {
  planned = false;
  $("export").hidden = !shown;
  if (!shown) return;
  const where = outlineFolder(state, shown);
  $("about").textContent = shown.about || "";
  $("folder-path").textContent = where ? "‎" + where + "‎" : "Choose a folder…";
  $("folder-path").classList.toggle("muted", !where);
  const something = isSnapshot(shown) ? shown.files.length > 0 : shown.rules.length > 0;
  $("dry-run").disabled = !where || !something;
  $("run").disabled = true;
  $("export-plan").replaceChildren();
  say($("export-message"), something ? "" : isSnapshot(shown) ? "Nothing left to export." : "No rule: nothing to export.");
}
const folders = () => ({ [shown.name]: outlineFolder(state, shown) });

$("choose").addEventListener("click", async () => {
  const chosen = await openFile({ title: "Export " + shown.name + " to", folders: true, writable: true, confirm: "Choose",
    message: "Where an export from this browser writes " + shown.name + ". An export removes only files it wrote there itself.",
    folder: outlineFolder(state, shown) || state.root, fallbacks: [state.root, ""] });
  if (!chosen) return;
  const path = Array.isArray(chosen) ? chosen[0] : chosen;
  rememberOutlineFolder(state, shown.name, path === shown.default ? "" : path);
  exportShown();
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
    open(snapshots.find((x) => x.name === taken.name));
  } catch (err) {
    say($("message"), err.message, true);
  }
});
$("drop").addEventListener("click", async () => {
  if (!confirm(`Delete the Snapshot ${shown.name}?\n\nIts file moves into the tree's trash. The Items and anything exported stay.`)) return;
  try {
    await post("/api/snapshots/delete", { name: shown.name });
    await loadSnapshots();
    shown = null;
    const o = outlines[0] || snapshots[0];
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

$("dry-run").addEventListener("click", async () => {
  exportShown();
  say($("export-message"), "Checking…");
  try {
    const answer = await post("/api/export/plan", { outlines: [shown.name], folders: folders() });
    $("export-plan").replaceChildren(...planNodes(state, answer));
    const changes = answer.jobs.reduce((n, j) => n + j.plan.add.length + j.plan.replace.length + j.plan.remove.length, 0);
    if (!answer.ready) say($("export-message"), "Not exported: fix what is listed below first. Nothing is written until everything checks.", true);
    else if (!changes) say($("export-message"), "Up to date: nothing to write.");
    else {
      say($("export-message"), "No conflicts. " + changes + " change" + (changes === 1 ? "" : "s") + " to write.");
      planned = true;
      $("run").disabled = false;
    }
  } catch (error) {
    say($("export-message"), error.message, true);
  }
});
$("run").addEventListener("click", async () => {
  if (!planned) return;
  $("run").disabled = true;
  say($("export-message"), "Exporting: checking again, then copying and reading back…");
  try {
    const answer = await post("/api/export", { outlines: [shown.name], folders: folders() });
    const r = answer.results[0];
    say($("export-message"), "Exported: " + r.result.written + " written, " + r.result.removed + " removed, " + r.result.kept + " unchanged" +
      (r.history_error ? " (history not recorded: " + r.history_error + ")" : "") + ".");
    $("export-plan").replaceChildren();
  } catch (error) {
    say($("export-message"), error.message, true);
  } finally {
    planned = false;
  }
});

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
  const o = [...outlines, ...snapshots].find((x) => x.name === wanted) || outlines[0] || snapshots[0];
  if (o) open(o);
})().catch((err) => {
  state.error = err.message;
  frame(state);
});
