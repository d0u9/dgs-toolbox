// View: one Outline read as a folder is in Finder, and nothing else of
// Explore; the top bar stays. Its folders and PDFs down the left as a tree, the PDF picked in the
// middle, and on the right what the tree knows of it, or of the folder
// picked. Nothing here changes anything: making and exporting are
// Explore's, and PDFs the Outline cannot place are not shown.
//
//   const view = outlineView({ state, preview, onLeave, explain });
//   view.show(name, root)   draws the Outline named, root its grouping's root
//
// preview(file) shows a PDF's pages in the page's reader, or none for null;
// the reader is moved into the view while it is open and back when it is
// left. onLeave is called when the reader asks to leave: Esc, or the button.
// explain(file) answers the server's explanations of how the Outline
// places the file's Item, for Why here.
import { $, el, keep, label, fieldsAt, tagsAt } from "/common.js";
import { fileTree } from "/ui/filetree.js";
import { find, findFile, resizable, whyText } from "/outlinetree.js";

export function outlineView({ state, preview, onLeave, explain }) {
  let name = "";
  let root = null;
  let picked = ""; // the folder ("a/b/") or PDF ("a/b/c.pdf") picked, by tree path
  const closed = new Set(); // the folders drawn shut on the left
  let home = null; // where the reader sits when the view is shut
  let why = false; // Why here is open, and stays open from PDF to PDF
  // Both side panes are dragged wider or narrower, kept in this browser.
  resizable($("view-tree"), "dgs-doc-view-tree", { after: false, fallback: 300, min: 180 });
  resizable($("view-info"), "dgs-doc-view-info", { fallback: 320, min: 220 });

  const item = (f) => state().items.find((i) => i.id === f.item);
  const isFolder = (path) => path === "" || path.endsWith("/");
  const folderOf = (path) => (isFolder(path) ? path.slice(0, -1) : "");
  const base = (path) => path.replace(/\/$/, "").split("/").pop();
  const files = () => {
    const all = [];
    const walk = (n) => { all.push(...n.files); n.children.forEach(walk); };
    if (root) walk(root);
    return all;
  };

  function pick(path) {
    picked = path;
    keep({ viewing: picked });
    draw();
  }
  function quickLook(on) {
    if (on && isFolder(picked)) return;
    document.body.classList.toggle("quicklook", on);
  }
  // rows are the tree's rows as drawn, top to bottom: what up and down walk.
  const rows = () => [...$("view-tree").querySelectorAll(".ft-row[data-path]")];

  function drawTree() {
    const folders = [];
    const walk = (n) => { for (const c of n.children) { folders.push(c.path + "/"); walk(c); } };
    if (root) walk(root);
    $("view-tree").replaceChildren(fileTree(files(), {
      closed, folders,
      selected: picked,
      onPick: (f) => pick(f.path),
      onPickFolder: (path) => pick(path),
      folderExtra: (path) => el("span", { className: "numeric" }, String(find(root, path.slice(0, -1))?.count ?? "")),
      decorate: (row, { path }) => { row.dataset.path = path; },
    }));
    rows().find((r) => r.dataset.path === picked)?.scrollIntoView({ block: "nearest" });
  }

  function drawCrumbs() {
    const parts = picked.replace(/\/$/, "").split("/").filter(Boolean);
    $("view-crumbs").replaceChildren(...parts.flatMap((p) => [el("span", { className: "crumb-sep", "aria-hidden": "true" }, "›"),
      el("span", { className: "crumb" }, p)]));
    $("view-count").textContent = root ? root.count + (root.count === 1 ? " PDF" : " PDFs") : "";
  }

  function drawInfo() {
    const file = isFolder(picked) ? null : findFile(root, picked);
    preview(file);
    $("view").classList.toggle("has-file", !!file);
    if (!file) {
      const node = find(root, folderOf(picked));
      return $("view-info").replaceChildren(el("h3", {}, node && node.path ? base(node.path) : name),
        el("p", { className: "muted" }, node ? node.count + (node.count === 1 ? " PDF" : " PDFs") + (node.snapshot ? " · the Snapshot " + node.snapshot + ", fixed as it was taken" : "") : ""));
    }
    const i = item(file);
    const fields = i ? fieldsAt(i, file.digest) : {};
    const t = i && state().templates.find((x) => x.type === i.type);
    const order = t ? t.fields.map((f) => f.key) : Object.keys(fields);
    const keys = [...order, ...Object.keys(fields).filter((k) => !order.includes(k))].filter((k) => fields[k]);
    const value = (k) => {
      const f = t && t.fields.find((x) => x.key === k);
      const other = f && f.type === "item" && state().items.find((x) => x.id === fields[k]);
      return other ? label(state(), other) : fields[k];
    };
    const tags = i ? tagsAt(i, file.digest) : [];
    const when = added(i, file);
    $("view-info").replaceChildren(
      el("h3", {}, i ? label(state(), i) : file.item),
      el("p", { className: "view-meta" }, el("span", { className: "mono" }, i ? i.type : ""), " ", expiry(i),
        file.revision > 1 ? el("span", { className: "muted" }, " revision " + file.revision) : null),
      el("p", { className: "mono view-path" }, file.path),
      tags.length ? el("p", { className: "view-tags" }, ...tags.map((tag) => el("span", { className: "tag" }, tag))) : null,
      el("dl", { className: "view-fields" }, ...keys.flatMap((k) => [el("dt", {}, k), el("dd", {}, value(k))])),
      when ? el("p", { className: "muted" }, "Added " + when) : null,
      whyHere(file));
  }

  // whyHere is what placed file where it is: the rule's conditions, the
  // branch taken and what each key wrote, as the server explains it. It is
  // asked for only while open.
  function whyHere(file) {
    const body = el("pre", { className: "view-why-text" });
    const box = el("details", { className: "view-why" }, el("summary", {}, "Why here"), body);
    const load = async () => {
      body.textContent = "…";
      try {
        const all = await explain(file);
        if (picked !== file.path) return;
        body.textContent = whyText(all, file);
      } catch (err) {
        body.textContent = err.message;
      }
    };
    box.ontoggle = () => { why = box.open; if (why) load(); };
    box.open = why;
    return box;
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
    drawInfo();
  }

  // The keys a Finder reader expects: up and down move down the tree, right
  // opens a folder, left shuts it or goes to the folder holding the row,
  // Space shows the PDF large.
  function onKey(event) {
    if (!document.body.classList.contains("viewing")) return;
    // A field, or a splitter's own arrows, are not the tree's.
    if (event.target instanceof Element && event.target.closest("input, select, textarea, .splitter")) return;
    // A button keeps its own Enter and Space.
    if (event.target instanceof Element && event.target.closest("button") && (event.key === "Enter" || event.key === " ")) return;
    const list = rows().map((r) => r.dataset.path);
    const n = list.indexOf(picked);
    if (event.key === "Escape") {
      event.preventDefault();
      if (document.body.classList.contains("quicklook")) quickLook(false); else onLeave();
    } else if (event.key === " ") {
      event.preventDefault();
      quickLook(!document.body.classList.contains("quicklook"));
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const next = list[Math.max(0, Math.min(list.length - 1, n < 0 ? 0 : n + (event.key === "ArrowDown" ? 1 : -1)))];
      if (next !== undefined) pick(next);
    } else if (event.key === "ArrowRight" || event.key === "Enter") {
      event.preventDefault();
      if (isFolder(picked) && picked) { closed.delete(picked); draw(); } else quickLook(true);
    } else if (event.key === "ArrowLeft") {
      event.preventDefault();
      quickLook(false);
      if (isFolder(picked) && picked && !closed.has(picked)) { closed.add(picked); return draw(); }
      const path = picked.replace(/\/$/, "");
      if (path.includes("/")) pick(path.slice(0, path.lastIndexOf("/") + 1));
    }
  }
  document.addEventListener("keydown", onKey);

  $("view-leave").onclick = () => onLeave();
  $("view-quicklook").onclick = () => quickLook(false);

  return {
    // show opens the view on the Outline named, root its grouping's root.
    show(outline, answerRoot) {
      if (!document.body.classList.contains("viewing")) {
        const reader = $("preview");
        home = { parent: reader.parentNode, next: reader.nextSibling };
        $("view-preview").prepend(reader);
        document.body.classList.add("viewing");
        $("view").hidden = false;
      }
      if (outline !== name) { closed.clear(); picked = ""; }
      name = outline;
      root = answerRoot;
      $("view-title").textContent = name;
      if (isFolder(picked) ? !find(root, folderOf(picked)) : !findFile(root, picked)) picked = "";
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
    // pick picks the folder ("a/b/") or PDF named, if the Outline has it.
    pick(path) {
      if (isFolder(path) ? find(root, folderOf(path)) : findFile(root, path)) pick(path);
    },
    get open() { return document.body.classList.contains("viewing"); },
  };
}
