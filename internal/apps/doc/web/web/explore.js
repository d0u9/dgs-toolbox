// Explore: an Outline's result. Its rules' PDFs in the tree they make, each
// folder counting the PDFs beneath it; picking a folder or a PDF lists it
// beside, and Export writes the same tree into a folder. Outlines are made
// on the Outlines page; nothing here changes one.
import { $, api, el, loadState, post, frame, say, planNodes, outlineFolder, rememberOutlineFolder } from "/common.js";
import { openFile } from "/ui/filedialog.js";
import { outlineTree, find, findFile, folderFiles, pdfRow, unplaced, UNPLACED } from "/outlinetree.js";

let state = { templates: [], items: [] };
let outlines = [];
let shown = null; // the Outline drawn
let grouping = null;
let planned = false; // a check passed and Export may write it

const tree = outlineTree($("tree"), () => state, { onPick: files, empty: () => shown && !shown.rules.length ? "The Outline has no rule yet." : "The Outline selects no PDFs." });

function list() {
  $("count").textContent = outlines.length;
  $("outlines").replaceChildren(...outlines.map((o) => el("li", { className: o.name === (shown && shown.name) ? "selected" : "", onclick: () => open(o) },
    el("span", { className: "template-name" }, o.name),
    el("span", { className: "badge" }, o.rules.length + (o.rules.length === 1 ? " rule" : " rules")),
    el("span", { className: "template-sub" }, o.about || ""))));
  $("empty").hidden = outlines.length > 0;
}

async function open(o) {
  shown = o;
  history.replaceState(null, "", "#" + encodeURIComponent(o.name));
  $("title").textContent = o.name;
  $("edit").href = api("/outlines/") + "#" + encodeURIComponent(o.name);
  $("edit").hidden = false;
  tree.reset();
  list();
  exportShown();
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
  $("dry-run").disabled = !where || !shown.rules.length;
  $("run").disabled = true;
  $("export-plan").replaceChildren();
  say($("export-message"), shown.rules.length ? "" : "No rule: nothing to export.");
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
  if (answer.migrated && answer.migrated.length) say($("message"), "The tree's Views and Targets are Outlines now: " + answer.migrated.join(", ") + ". The old files are in migrated/.");
  for (const a of document.querySelectorAll('a[href^="/outlines/"]')) a.href = api("/outlines/") + a.hash;
  const wanted = decodeURIComponent(location.hash.slice(1));
  list();
  const o = outlines.find((x) => x.name === wanted) || outlines[0];
  if (o) open(o);
})().catch((err) => {
  state.error = err.message;
  frame(state);
});
