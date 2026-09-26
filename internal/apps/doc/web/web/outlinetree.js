// An Outline's tree: its folders, each with the count of PDFs beneath it,
// and its PDFs by the name each has there. The server plans and nests the
// rules; this draws what it answered. Shared by the Explore and Outlines
// pages.
import { api, el, label } from "/common.js";

const FOLDER = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.75 3.25h4.5l1.5 1.5h6.5v8H1.75z" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/><path d="M1.75 6.25h12.5" stroke="currentColor" stroke-width="1.25"/></svg>';
const FILE = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3.25 1.75h6l3.5 3.5v9H3.25z" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/><path d="M9.25 1.75v3.5h3.5" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/></svg>';

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

// outlineTree draws into host. With onPick, a row is picked by clicking it
// and onPick is called with its path ("" when unpicked); without, a click
// opens or shuts a folder and a PDF links to its Item. empty is the text
// drawn when there is nothing. A folder with snapshot set is a Snapshot's,
// drawn as one and shut until opened.
export function outlineTree(host, state, { onPick, empty = () => "" } = {}) {
  const closed = new Set(); // folders toggled from how they start
  const shut = (c) => closed.has(c.path) !== !!c.snapshot;
  let picked = "";
  let grouping = null;
  const name = (id) => { const item = state().items.find((i) => i.id === id); return item ? label(state(), item) : id; };
  const draw = () => {
    const root = grouping && grouping.root;
    const lost = unplaced(grouping);
    if (picked !== UNPLACED && !find(root, picked) && !findFile(root, picked)) picked = "";
    if (picked === UNPLACED && !lost.length) picked = "";
    const rows = [];
    const walk = (node, depth) => {
      for (const c of node.children) {
        rows.push(folderRow(c, depth));
        if (!shut(c)) walk(c, depth + 1);
      }
      for (const f of node.files) rows.push(fileRow(f, depth));
    };
    if (root) walk(root, 0);
    if (lost.length && onPick) rows.push(row({ name: "Not placed", path: UNPLACED, depth: 0, count: lost.length, icon: FOLDER, className: " unplaced", title: "PDFs the rules cannot place" }));
    host.replaceChildren(...(rows.length ? rows : [el("p", { className: "muted outline-empty" }, root ? "No Item matches." : empty())]));
  };
  const toggle = (path) => { if (closed.has(path)) closed.delete(path); else closed.add(path); draw(); };
  const folderRow = (c, depth) => row({ name: c.name, path: c.path, depth, count: c.count, parent: true, open: !shut(c),
    icon: c.snapshot ? SNAPSHOT : FOLDER, className: c.snapshot ? " outline-snapshot" : "",
    title: c.snapshot ? "Snapshot " + c.snapshot + ": fixed as it was taken" : c.path });
  const fileRow = (f, depth) => row({ name: f.path.split("/").pop(), path: f.path, depth, icon: FILE, className: " outline-file",
    title: f.path + "\n" + name(f.item) + (f.view ? " · rule " + f.view : ""), href: api("/browse/") + "#" + f.item });
  const row = ({ name, path, depth, count, icon, parent, open, className = "", title, href }) => {
    const twist = el("span", { className: "outline-twist", textContent: parent ? (open ? "▾" : "▸") : "",
      onclick: (event) => { event.stopPropagation(); if (parent) toggle(path); } });
    const glyph = el("span", { className: "ft-icon" });
    glyph.innerHTML = icon;
    return el("div", { className: "outline-row" + className + (onPick && picked === path ? " selected" : ""),
      role: "treeitem", tabIndex: 0, style: `--depth:${depth}`, title,
      onclick: () => {
        if (!onPick) return parent ? toggle(path) : href && (location.href = href);
        picked = picked === path ? "" : path;
        draw();
        onPick(picked);
      },
      onkeydown: (event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); event.currentTarget.click(); } } },
      twist, glyph, el("span", { className: "outline-name" }, name),
      el("span", { className: "outline-count numeric" }, count !== undefined ? String(count) : ""));
  };
  return {
    show(answer) { grouping = answer; draw(); },
    reset() { closed.clear(); picked = ""; },
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
