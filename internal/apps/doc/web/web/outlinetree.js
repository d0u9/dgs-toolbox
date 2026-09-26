// An Outline's folders as a tree, each with the count of PDFs beneath it,
// and the PDFs of a folder as a list. The server groups the Items; this
// draws what it answered. Shared by the Explore and Outlines pages.
import { api, el, label } from "/common.js";

const FOLDER = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.75 3.25h4.5l1.5 1.5h6.5v8H1.75z" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/><path d="M1.75 6.25h12.5" stroke="currentColor" stroke-width="1.25"/></svg>';

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

// outlineTree draws into host. With onPick, a folder is picked by clicking
// it and onPick is called with its path ("" when unpicked); without, a click
// opens or shuts it. empty is the text drawn when there is nothing.
export function outlineTree(host, { onPick, empty = () => "" } = {}) {
  const closed = new Set();
  let picked = "";
  let grouping = null;
  const draw = () => {
    const root = grouping && grouping.root;
    const missing = (grouping && grouping.missing) || [];
    if (picked !== UNPLACED && !find(root, picked)) picked = "";
    if (picked === UNPLACED && !missing.length) picked = "";
    const rows = [];
    const walk = (node, depth) => {
      for (const c of node.children) {
        rows.push(row(c.name, c.path, c.count, depth, c.children.length > 0));
        if (!closed.has(c.path)) walk(c, depth + 1);
      }
    };
    if (root) walk(root, 0);
    if (missing.length && onPick) rows.push(row("Not placed", UNPLACED, missing.length, 0, false, true));
    host.replaceChildren(...(rows.length ? rows : [el("p", { className: "muted outline-empty" }, root ? "No Item matches." : empty())]));
  };
  const toggle = (path) => { if (closed.has(path)) closed.delete(path); else closed.add(path); draw(); };
  const row = (name, path, count, depth, parent, unplaced) => {
    const open = !closed.has(path);
    const twist = el("span", { className: "outline-twist", textContent: parent ? (open ? "▾" : "▸") : "",
      onclick: (event) => { event.stopPropagation(); toggle(path); } });
    const icon = el("span", { className: "ft-icon" });
    icon.innerHTML = FOLDER;
    return el("div", { className: "outline-row" + (onPick && picked === path ? " selected" : "") + (unplaced ? " unplaced" : ""),
      role: "treeitem", tabIndex: 0, style: `--depth:${depth}`, title: unplaced ? "PDFs the layout cannot place" : path,
      onclick: () => {
        if (!onPick) return parent && toggle(path);
        picked = picked === path ? "" : path;
        draw();
        onPick(picked);
      },
      onkeydown: (event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); event.currentTarget.click(); } } },
      twist, icon, el("span", { className: "outline-name" }, name), el("span", { className: "outline-count numeric" }, String(count)));
  };
  return {
    show(answer) { grouping = answer; draw(); },
    reset() { closed.clear(); picked = ""; },
    get picked() { return picked; },
  };
}

// pdfRow is one PDF linked to its Item on Browse, with extra beneath it.
export function pdfRow(state, f, all, extra) {
  const item = state.items.find((i) => i.id === f.item);
  return el("li", {}, el("a", { href: api("/browse/") + "#" + f.item }, item ? label(state, item) : f.item),
    all ? el("span", { className: "muted" }, " · revision " + f.revision) : null,
    extra || null);
}

// folderFiles lists the PDFs in node and beneath it, those beneath under
// the folder they are in, relative to node.
export function folderFiles(state, node, all) {
  const out = [];
  const walk = (n) => {
    if (n.files.length) {
      if (n !== node) out.push(el("li", { className: "outline-sub mono" }, n.path.slice(node.path ? node.path.length + 1 : 0)));
      out.push(...n.files.map((f) => pdfRow(state, f, all)));
    }
    n.children.forEach(walk);
  };
  walk(node);
  return out;
}
