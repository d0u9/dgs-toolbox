// Explore: an Outline's result. The Items grouped into its folders, each
// counting the PDFs beneath it; picking a folder lists its PDFs. Outlines
// are made on the Outlines page; nothing here changes one.
import { $, api, el, loadState, post, frame, say } from "/common.js";
import { outlineTree, find, folderFiles, pdfRow, UNPLACED } from "/outlinetree.js";

let state = { templates: [], items: [] };
let outlines = [];
let shown = null; // the Outline drawn
let grouping = null;

const tree = outlineTree($("tree"), { onPick: files, empty: () => "The Outline selects no PDFs." });

function list() {
  $("count").textContent = outlines.length;
  $("outlines").replaceChildren(...outlines.map((o) => el("li", { className: o.name === (shown && shown.name) ? "selected" : "", onclick: () => open(o) },
    el("span", { className: "template-name" }, o.name), el("span", { className: "template-sub mono" }, o.layout))));
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

// files lists the PDFs of the folder picked, or of the whole Outline.
function files(picked) {
  const all = shown && shown.selection === "all";
  if (picked === UNPLACED) {
    $("folder").textContent = "Not placed";
    $("folder-count").textContent = grouping.missing.length;
    $("files").replaceChildren(el("li", { className: "outline-note muted" }, "The levels cannot place these. Fill the field on Browse, or number the value on the Outlines page."),
      ...grouping.missing.map((m) => pdfRow(state, m, all, el("div", { className: "template-sub" }, "lacks " + m.keys.join(", ")))));
    return;
  }
  const node = find(grouping && grouping.root, picked);
  $("folder").textContent = picked ? picked.split("/").pop() : "All PDFs";
  $("folder-count").textContent = node ? node.count : "";
  $("files").replaceChildren(...(node ? folderFiles(state, node, all) : []));
}

(async () => {
  state = await loadState();
  frame(state);
  const answer = await (await fetch(api("/api/outlines"))).json();
  outlines = answer.outlines;
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
  for (const a of document.querySelectorAll('a[href^="/outlines/"]')) a.href = api("/outlines/") + a.hash;
  const wanted = decodeURIComponent(location.hash.slice(1));
  list();
  const o = outlines.find((x) => x.name === wanted) || outlines[0];
  if (o) open(o);
})().catch((err) => {
  state.error = err.message;
  frame(state);
});
