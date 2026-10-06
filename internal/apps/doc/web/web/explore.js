// Explore: an Outline's tree, browsed as in a file manager. Its rules' PDFs
// and its Snapshots, each Snapshot a folder of its name, every folder
// counting the PDFs beneath it; picking a folder lists its PDFs beside the
// tree, picking a PDF opens it there. Export writes the same tree into a
// folder you choose. Outlines are made on the Outlines page, from rules
// and Snapshots made on theirs; nothing here changes one.
import { $, address, keep, scrollBack, api, el, loadState, post, frame, say, planNodes, showPreview, clearPreview, label, exportOutline, nameTree, outlineTitle } from "/common.js";
import { outlineTree, find, findFile, folderFiles, pdfRow, unplaced, UNPLACED, resizable } from "/outlinetree.js";
import { outlineView } from "/outlineview.js";

let state = { templates: [], items: [] };
let outlines = [];
let shown = null; // the Outline drawn
let target = null; // what Export writes: shown
let grouping = null;
let picture = null; // the PDF previewed
const back = history.state || {}; // what the page showed when it was left, on coming Back
const unscroll = scrollBack(["tree", "files", "view-tree", "view-info"]);

const tree = outlineTree($("tree"), () => state, { onPick: files, empty: () =>
  shown && !shown.rules.length && !(shown.snapshots || []).length ? "The Outline has no rule and no Snapshot yet." : "The Outline selects no PDFs." });
resizable(document.querySelector(".outline-main"), "dgs-doc-explore-tree", { after: false });

function list() {
  $("count").textContent = outlines.length;
  nameTree($("outlines"), outlines.map((o) => ({ name: o.name, note: o.rules.length + (o.snapshots || []).length, title: outlineTitle(o), o })), { selected: shown ? shown.name : "", onPick: (e) => open(e.o) });
  $("no-outlines").hidden = outlines.length > 0;
}

async function open(o) {
  shown = target = o;
  address("#" + encodeURIComponent(o.name));
  $("title").textContent = o.name;
  $("description").textContent = o.about || "";
  $("edit").href = api("/outlines/") + "#" + encodeURIComponent(o.name);
  $("export-button").disabled = !o.rules.length && !(o.snapshots || []).length;
  tree.reset("outline:" + o.name);
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
  $("view-button").disabled = !grouping;
  if (view.open) grouping ? view.show(o.name, grouping.root) : leaveView();
}

// View reads the Outline shown as a folder, and nothing else. It is in the
// address, ?view, so a link or a reload opens on it.
const view = outlineView({ state: () => state, preview, onLeave: () => leaveView(),
  explain: (file) => post("/api/outlines/explain", { outline: shown, item: file.item }) });
function enterView() {
  if (!shown || !grouping) return;
  view.show(shown.name, grouping.root);
  const url = new URL(location.href);
  url.searchParams.set("view", "");
  address(url);
}
function leaveView() {
  view.hide();
  const url = new URL(location.href);
  url.searchParams.delete("view");
  address(url);
  files(tree.picked);
}
$("view-button").addEventListener("click", enterView);

// files lists what is picked: a PDF, a folder's PDFs, or the whole Outline's.
function files(picked) {
  keep({ picked });
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
// folder chosen in the dialog; a run that does not check is listed beside
// the tree.
$("export-button").addEventListener("click", () => exportOutline(state, target, {
  say: (text, bad) => say($("message"), text, bad),
  blocked: (plan) => {
    preview(null);
    $("folder").textContent = "Not exported";
    $("folder-count").textContent = "";
    $("files").replaceChildren(el("li", {}, ...planNodes(state, plan)));
  },
}));

(async () => {
  state = await loadState();
  frame(state);
  const answer = await (await fetch(api("/api/defs/outlines"))).json();
  outlines = answer.outlines;
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
  for (const a of document.querySelectorAll('a[href^="/outlines/"]')) a.href = api("/outlines/") + a.hash;
  const wanted = decodeURIComponent(location.hash.slice(1));
  const o = outlines.find((x) => x.name === wanted) || outlines[0];
  if (o) {
    await open(o);
    if (back.picked && o.name === wanted) { tree.pick(back.picked); files(tree.picked); }
    if (new URLSearchParams(location.search).has("view")) {
      enterView();
      if (back.viewing && o.name === wanted) view.pick(back.viewing);
    }
    if (o.name === wanted) unscroll();
  }
})().catch((err) => {
  state.error = err.message;
  frame(state);
});
