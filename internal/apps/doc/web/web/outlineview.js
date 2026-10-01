// View: one Outline read as a folder is in Finder, and nothing else on the
// page. Its folders down the left, the folder picked in the middle — its
// folders, then its PDFs, as cards or a list — and the PDF picked on the
// right, its pages above what the tree knows of it. Nothing here changes
// anything: making and exporting are Explore's, and PDFs the Outline cannot
// place are not shown.
//
//   const view = outlineView({ state, preview, onLeave });
//   view.show(name, root)   draws the Outline named, root its grouping's root
//
// preview(file) shows a PDF's pages in the page's reader, or none for null;
// the reader is moved into the view while it is open and back when it is
// left. onLeave is called when the reader asks to leave: Esc, or the button.
import { $, api, el, label, fieldsAt, tagsAt } from "/common.js";
import { fileTree } from "/ui/filetree.js";
import { find, findFile } from "/outlinetree.js";

const FOLDER = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.75 3.25h4.5l1.5 1.5h6.5v8H1.75z" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/><path d="M1.75 6.25h12.5" stroke="currentColor" stroke-width="1.25"/></svg>';
const LAYOUT_KEY = "dgs-doc-view-layout";

export function outlineView({ state, preview, onLeave }) {
  let name = "";
  let root = null;
  let at = ""; // the folder shown in the middle
  let picked = ""; // the folder or PDF picked in it, by path
  let layout = "grid";
  try { layout = localStorage.getItem(LAYOUT_KEY) === "list" ? "list" : "grid"; } catch { /* the default */ }
  const closed = new Set(); // the folders drawn shut on the left
  let home = null; // where the reader sits when the view is shut

  const item = (f) => state().items.find((i) => i.id === f.item);
  const isFile = (path) => !!findFile(root, path);
  const entries = () => {
    const node = find(root, at);
    return node ? [...node.children.map((c) => ({ kind: "folder", path: c.path, node: c })), ...node.files.map((f) => ({ kind: "file", path: f.path, file: f }))] : [];
  };
  const base = (path) => path.slice(path.lastIndexOf("/") + 1);
  const thumbURL = (f) => api("/api/page?" + new URLSearchParams({ item: f.item, digest: f.digest, n: 1, size: "thumb", v: f.digest }));

  function go(path, pick = "") {
    at = path;
    picked = pick;
    draw();
  }
  function pick(path) {
    picked = path;
    drawMiddle();
    drawSide();
  }
  function up() {
    if (!at) return;
    const was = at;
    go(at.includes("/") ? at.slice(0, at.lastIndexOf("/")) : "", was);
  }
  function enter(entry) {
    if (!entry) return;
    if (entry.kind === "folder") go(entry.path);
    else quickLook(!document.body.classList.contains("quicklook"));
  }
  function quickLook(on) {
    if (on && !isFile(picked)) return;
    document.body.classList.toggle("quicklook", on);
  }

  function drawTree() {
    const folders = [];
    const walk = (n) => { for (const c of n.children) { folders.push(c.path + "/"); walk(c); } };
    if (root) walk(root);
    $("view-tree").replaceChildren(fileTree([], {
      closed, folders,
      selected: at ? at + "/" : "",
      onPickFolder: (path) => go(path.slice(0, -1)),
      folderExtra: (path) => el("span", { className: "numeric" }, String(find(root, path.slice(0, -1))?.count ?? "")),
    }));
  }

  function drawCrumbs() {
    const parts = at ? at.split("/") : [];
    $("view-crumbs").replaceChildren(
      el("button", { type: "button", className: "text-button crumb", onclick: () => go("") }, name),
      ...parts.flatMap((p, n) => [el("span", { className: "crumb-sep", "aria-hidden": "true" }, "›"),
        el("button", { type: "button", className: "text-button crumb", onclick: () => go(parts.slice(0, n + 1).join("/")) }, p)]));
    const node = find(root, at);
    $("view-count").textContent = node ? node.count + (node.count === 1 ? " PDF" : " PDFs") : "";
  }

  function drawMiddle() {
    const list = entries();
    for (const b of $("view-layout").querySelectorAll("button")) b.setAttribute("aria-pressed", String(b.dataset.layout === layout));
    const host = $("view-items");
    host.className = "view-items " + (layout === "list" ? "view-list" : "view-grid");
    const choose = (entry) => ({ onclick: () => pick(entry.path), ondblclick: () => enter(entry) });
    if (!list.length) return host.replaceChildren(el("p", { className: "muted" }, "Nothing in this folder."));
    if (layout === "list") {
      host.replaceChildren(el("table", { className: "items-table" },
        el("thead", {}, el("tr", {}, ...["Name", "Type", "Owner", "Expiry", "Added"].map((t) => el("th", {}, t)))),
        el("tbody", {}, ...list.map((entry) => {
          const tr = el("tr", { className: "table-row" + (entry.path === picked ? " card-selected" : ""), ...choose(entry) });
          if (entry.kind === "folder") {
            tr.append(el("td", { className: "view-name" }, icon(), base(entry.path)), el("td", { className: "muted" }, "folder"),
              el("td", {}), el("td", {}), el("td", { className: "numeric muted" }, entry.node.count + " PDFs"));
          } else {
            const i = item(entry.file), fields = i ? fieldsAt(i, entry.file.digest) : {};
            tr.append(el("td", { className: "view-name mono" }, base(entry.path)), el("td", { className: "mono" }, i ? i.type : ""),
              el("td", {}, fields.owner || ""), el("td", {}, expiry(i)), el("td", { className: "numeric" }, added(i, entry.file)));
          }
          return tr;
        }))));
    } else {
      host.replaceChildren(...list.map((entry) => {
        const tile = el("div", { className: "view-tile" + (entry.path === picked ? " selected" : ""), title: base(entry.path), ...choose(entry) });
        if (entry.kind === "folder") {
          const pic = el("div", { className: "view-tile-pic view-folder" });
          pic.innerHTML = FOLDER;
          tile.append(pic, el("div", { className: "view-tile-name" }, base(entry.path)), el("div", { className: "view-tile-sub numeric" }, entry.node.count + " PDFs"));
        } else {
          const pic = el("div", { className: "view-tile-pic" });
          const img = el("img", { loading: "lazy", alt: "", src: thumbURL(entry.file) });
          const none = () => { pic.classList.add("no-picture"); pic.dataset.type = item(entry.file)?.type || "PDF"; };
          img.addEventListener("error", none, { once: true });
          img.addEventListener("load", () => { if (!img.naturalWidth) none(); }, { once: true });
          pic.append(img);
          tile.append(pic, el("div", { className: "view-tile-name mono" }, base(entry.path)), expiry(item(entry.file)) || el("div", { className: "view-tile-sub" }));
        }
        return tile;
      }));
    }
    host.querySelector(".selected, .card-selected")?.scrollIntoView({ block: "nearest" });
  }

  function drawSide() {
    const file = findFile(root, picked);
    preview(file);
    $("view-side").classList.toggle("has-file", !!file);
    if (!file) {
      const node = find(root, isFile(picked) ? at : picked || at);
      return $("view-info").replaceChildren(el("h3", {}, node && node.path ? base(node.path) : name),
        el("p", { className: "muted" }, node ? node.count + (node.count === 1 ? " PDF" : " PDFs") + (node.snapshot ? " · the Snapshot " + node.snapshot + ", fixed as it was taken" : "") : ""),
        el("p", { className: "muted view-hint" }, "Pick a PDF to see it. Space shows it large."));
    }
    const i = item(file);
    const fields = i ? fieldsAt(i, file.digest) : {};
    const t = i && state().templates.find((x) => x.type === i.type);
    const order = t ? t.fields.map((f) => f.key) : Object.keys(fields);
    const rows = [...order, ...Object.keys(fields).filter((k) => !order.includes(k))].filter((k) => fields[k]);
    const value = (k) => {
      const f = t && t.fields.find((x) => x.key === k);
      const other = f && f.type === "item" && state().items.find((x) => x.id === fields[k]);
      return other ? label(state(), other) : fields[k];
    };
    const tags = i ? tagsAt(i, file.digest) : [];
    $("view-info").replaceChildren(
      el("h3", {}, i ? label(state(), i) : file.item),
      el("p", { className: "view-meta" }, el("span", { className: "mono" }, i ? i.type : ""), " ", expiry(i),
        file.revision > 1 ? el("span", { className: "muted" }, " revision " + file.revision) : null),
      el("p", { className: "mono view-path" }, file.path),
      tags.length ? el("p", { className: "view-tags" }, ...tags.map((tag) => el("span", { className: "tag" }, tag))) : null,
      el("dl", { className: "view-fields" }, ...rows.flatMap((k) => [el("dt", {}, k), el("dd", {}, value(k))])));
  }

  function icon() {
    const span = el("span", { className: "ft-icon" });
    span.innerHTML = FOLDER;
    return span;
  }
  function expiry(i) {
    const e = i && (state().expiry || {})[i.id];
    if (!e) return null;
    const text = { expired: "expired " + (e.date || ""), soon: "expires in " + e.days + (e.days === 1 ? " day" : " days"), valid: "valid to " + (e.date || ""), permanent: "no end date" }[e.state];
    return text ? el("span", { className: "badge badge-" + e.state }, text) : null;
  }
  function added(i, f) {
    const r = i && i.revisions.find((x) => (x.id || x.digest) === f.digest || x.digest === f.digest);
    return r && r.added ? new Date(r.added).toLocaleDateString() : "";
  }

  function draw() {
    drawTree();
    drawCrumbs();
    drawMiddle();
    drawSide();
  }

  // The keys a Finder reader expects: up and down move, right or Enter go
  // into a folder, left or Backspace up out of it, Space shows the PDF large.
  function onKey(event) {
    if (!document.body.classList.contains("viewing")) return;
    if (event.target instanceof Element && event.target.closest("input, select, textarea")) return;
    // A button keeps its own Enter and Space.
    if (event.target instanceof Element && event.target.closest("button") && (event.key === "Enter" || event.key === " ")) return;
    const list = entries();
    const n = list.findIndex((e) => e.path === picked);
    if (event.key === "Escape") {
      event.preventDefault();
      if (document.body.classList.contains("quicklook")) quickLook(false); else onLeave();
    } else if (event.key === " ") {
      event.preventDefault();
      quickLook(!document.body.classList.contains("quicklook"));
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const next = list[Math.max(0, Math.min(list.length - 1, n < 0 ? 0 : n + (event.key === "ArrowDown" ? 1 : -1)))];
      if (next) pick(next.path);
    } else if (event.key === "ArrowRight" || event.key === "Enter") {
      if (list[n] && list[n].kind === "folder") { event.preventDefault(); enter(list[n]); }
      else if (event.key === "Enter" && list[n]) { event.preventDefault(); quickLook(true); }
    } else if (event.key === "ArrowLeft" || event.key === "Backspace") {
      event.preventDefault();
      quickLook(false);
      up();
    }
  }
  document.addEventListener("keydown", onKey);

  for (const b of $("view-layout").querySelectorAll("button")) {
    b.onclick = () => {
      layout = b.dataset.layout;
      try { localStorage.setItem(LAYOUT_KEY, layout); } catch { /* not kept */ }
      drawMiddle();
    };
  }
  $("view-leave").onclick = () => onLeave();
  $("view-quicklook").onclick = () => quickLook(false);

  return {
    // show opens the view on the Outline named, root its grouping's root.
    show(outline, answerRoot) {
      if (!document.body.classList.contains("viewing")) {
        const reader = $("preview");
        home = { parent: reader.parentNode, next: reader.nextSibling };
        $("view-preview").append(reader);
        document.body.classList.add("viewing");
        $("view").hidden = false;
      }
      if (outline !== name) { closed.clear(); at = ""; picked = ""; }
      name = outline;
      root = answerRoot;
      $("view-title").textContent = name;
      if (!find(root, at)) { at = ""; picked = ""; }
      draw();
    },
    // hide shuts the view and puts the reader back where it was.
    hide() {
      if (!document.body.classList.contains("viewing")) return;
      quickLook(false);
      document.body.classList.remove("viewing");
      $("view").hidden = true;
      if (home) home.parent.insertBefore($("preview"), home.next);
      preview(null);
    },
    get open() { return document.body.classList.contains("viewing"); },
  };
}
