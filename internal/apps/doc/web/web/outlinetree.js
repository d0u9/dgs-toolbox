// An Outline's tree: its folders, each with the count of PDFs beneath it,
// and its PDFs by the name each has there. The server plans and nests the
// rules; this draws what it answered. Shared by the Explore and Outlines
// pages.
import { api, el, label, post } from "/common.js";
import { fileTree } from "/ui/filetree.js";
import { splitter } from "/ui/splitter.js";

const FOLDER = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.75 3.25h4.5l1.5 1.5h6.5v8H1.75z" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/><path d="M1.75 6.25h12.5" stroke="currentColor" stroke-width="1.25"/></svg>';

const SNAPSHOT = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.75 4.75h3l1.25-1.75h4l1.25 1.75h3v8.5H1.75z" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/><circle cx="8" cy="8.75" r="2.5" fill="none" stroke="currentColor" stroke-width="1.25"/></svg>';

// UNPLACED is the path of the Not placed row.
export const UNPLACED = "\u0000unplaced";

// find is the folder at path under node, node itself for "", or null.
export function find(node, path) {
  if (!path || !node) return node;
  for (const c of node.children) {
    if (c.path === path) return c;
    if (path.startsWith(c.path + "/")) return find(c, path);
  }
  return null;
}

// findFile is the PDF at path under node, or null.
export function findFile(node, path) {
  if (!node || !path) return null;
  const folder = find(node, path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : "");
  return folder ? folder.files.find((f) => f.path === path) || null : null;
}

// unplaced is every PDF the grouping's rules select but cannot place: each
// lacking a key, or wanting a path another PDF wants too; and each of its
// Snapshots' PDFs the tree no longer has.
export function unplaced(grouping) {
  if (!grouping) return [];
  const out = [];
  for (const [rule, plan] of Object.entries(grouping.plans || {})) {
    for (const m of plan.missing) out.push({ ...m, view: rule, why: "lacks " + m.keys.join(", ") });
    for (const c of plan.clashes) for (const f of c.files) out.push({ ...f, view: rule, why: "wants " + c.path + " with another PDF" });
  }
  for (const c of grouping.clashes || []) for (const f of c.files) out.push({ ...f, why: "wants " + c.path + " with a PDF of " + c.files.filter((g) => g !== f).map((g) => g.view).join(", ") });
  for (const l of grouping.lost || []) out.push({ ...l, why: "is at " + l.path + ", but " + l.why });
  return out;
}

// resizable puts a handle on the side of the tree's pane that faces the
// rest of the page, so the reader drags it wider or narrower; the width is
// remembered per page. The pane after the form grows leftward, the pane
// before the list of PDFs rightward.
export function resizable(pane, key, { after = true, fallback = 480 } = {}) {
  const handle = el("div", { className: "splitter col", role: "separator", "aria-orientation": "vertical", title: "Drag to resize; double-click for the default" });
  if (after) pane.before(handle); else pane.after(handle);
  splitter({ handle, target: pane, axis: "x", invert: after, min: 260, fallback, key,
    max: () => Math.max(260, pane.parentElement.clientWidth - 420) });
}

// outlineTree draws into host with the shared file tree. With onPick, a
// row is picked by clicking it and onPick is called with its path ("" when
// unpicked); a folder's chevron opens or shuts it. Without, a click opens or
// shuts a folder and a PDF links to its Item. empty is the text drawn when
// there is nothing. A folder with snapshot set is a Snapshot's, drawn as one
// and shut until opened.
export function outlineTree(host, state, { onPick, empty = () => "" } = {}) {
  const closed = new Set(); // folder paths ("a/b/") drawn shut
  const seen = new Set(); // Snapshot folders already shut once
  let picked = "";
  let grouping = null;
  const name = (id) => { const item = state().items.find((i) => i.id === id); return item ? label(state(), item) : id; };
  // reveal shows the PDF selected in its folder in Finder.
  const reveal = (f) => post("/api/reveal", { item: f.item, digest: f.digest }).catch((err) => alert(err.message));
  const pick = (path) => { picked = picked === path ? "" : path; draw(); onPick(picked); };
  const draw = () => {
    const root = grouping && grouping.root;
    const lost = unplaced(grouping);
    if (picked !== UNPLACED && !find(root, picked) && !findFile(root, picked)) picked = "";
    if (picked === UNPLACED && !lost.length) picked = "";
    const files = [];
    const walk = (node) => {
      if (node.snapshot && !seen.has(node.path)) { seen.add(node.path); closed.add(node.path + "/"); }
      files.push(...node.files);
      node.children.forEach(walk);
    };
    if (root) walk(root);
    const folder = (path) => find(root, path.slice(0, -1));
    const parts = [];
    if (files.length) parts.push(fileTree(files, {
      closed,
      selected: onPick ? (find(root, picked) && picked ? picked + "/" : picked) : "",
      onPick: onPick ? (f) => pick(f.path) : undefined,
      onPickFolder: onPick ? (path) => pick(path.slice(0, -1)) : undefined,
      href: onPick ? undefined : (f) => api("/browse/") + "#" + f.item,
      folderIcon: (path) => folder(path)?.snapshot ? SNAPSHOT : "",
      folderClass: (path) => folder(path)?.snapshot ? "outline-snapshot" : "",
      fileExtra: (f) => el("button", { type: "button", className: "ft-reveal", title: "Show in Finder", "aria-label": "Show in Finder",
        textContent: "↗", onclick: (event) => { event.preventDefault(); event.stopPropagation(); reveal(f); } }),
      folderExtra: (path) => el("span", { className: "numeric" }, String(folder(path)?.count ?? "")),
    }));
    if (lost.length && onPick) {
      const r = el("div", { className: "ft-row unplaced" + (picked === UNPLACED ? " selected" : ""), role: "button", tabIndex: 0,
        title: "PDFs the rules cannot place", onclick: () => pick(UNPLACED),
        onkeydown: (event) => { if (event.key === "Enter") pick(UNPLACED); } },
        el("span", { className: "ft-icon" }), el("span", { className: "ft-name" }, "Not placed"), el("span", { className: "numeric" }, String(lost.length)));
      r.firstChild.innerHTML = FOLDER;
      parts.push(el("div", { className: "file-tree" }, r));
    }
    host.replaceChildren(...(parts.length ? parts : [el("p", { className: "muted outline-empty" }, root ? "No Item matches." : empty())]));
  };
  return {
    show(answer) { grouping = answer; draw(); },
    reset() { closed.clear(); seen.clear(); picked = ""; },
    pick(path) { picked = path; draw(); },
    get picked() { return picked; },
    get root() { return grouping && grouping.root; },
  };
}

// pdfRow is one PDF linked to its Item on Browse, with its name in the
// Outline and extra beneath it.
export function pdfRow(state, f, extra) {
  const item = state.items.find((i) => i.id === f.item);
  return el("li", {}, el("a", { href: api("/browse/") + "#" + f.item }, item ? label(state, item) : f.item),
    f.revision > 1 ? el("span", { className: "muted" }, " · revision " + f.revision) : null,
    f.path ? el("div", { className: "template-sub mono" }, f.path.split("/").pop() + (f.view ? " · " + f.view : "")) : null,
    extra || null);
}

// folderFiles lists the PDFs in node and beneath it, those beneath under
// the folder they are in, relative to node. A Snapshot within node is not
// its PDFs.
export function folderFiles(state, node) {
  const out = [];
  const walk = (n) => {
    if (n.files.length) {
      if (n !== node) out.push(el("li", { className: "outline-sub mono" }, n.path.slice(node.path ? node.path.length + 1 : 0)));
      out.push(...n.files.map((f) => pdfRow(state, f)));
    }
    n.children.filter((c) => !c.snapshot).forEach(walk);
  };
  walk(node);
  return out;
}
