// The folder tree: folders expand in place, so GPX files in sibling folders
// can be shown side by side. Folders load when first expanded.

import { api } from "./api.js";
import * as format from "./format.js";

export class FolderTree {
  // fileState(path) is a file's workspace entry ({ color, visible }), or
  // undefined. onFile(path, row) is called when a file is clicked.
  // home is the configured root that ⌂ returns to.
  constructor({ list, pathLabel, upButton, homeButton, refreshButton, message, onFile, fileState, onChange, home }) {
    Object.assign(this, { list, pathLabel, message, onFile, fileState, onChange });
    this.root = null;
    this.nodes = new Map(); // folder path -> { listing, expanded, loading, error }
    this.canReveal = false;
    upButton.addEventListener("click", () => {
      const parent = this.node(this.root).listing?.parent;
      if (parent) this.open(parent, [...this.expanded(), this.root]);
    });
    refreshButton.addEventListener("click", () => this.refresh());
    this.home = home;
    homeButton.addEventListener("click", () => this.open(this.home, this.expanded()));
    this.homeButton = homeButton;
    this.upButton = upButton;
  }

  node(path) {
    if (!this.nodes.has(path)) this.nodes.set(path, { listing: null, expanded: false, loading: false, error: null });
    return this.nodes.get(path);
  }

  expanded() {
    return [...this.nodes].filter(([path, node]) => node.expanded && path !== this.root).map(([path]) => path);
  }

  // open makes path the root and re-expands the given folders beneath it.
  async open(path, expanded = []) {
    this.nodes.clear();
    this.message.hidden = true;
    let listing;
    try {
      listing = await api.dir(path);
    } catch (error) {
      this.showMessage(error.message, true);
      if (this.root) return;
      listing = { path, parent: "", dirs: [], files: [] };
    }
    this.root = listing.path;
    Object.assign(this.node(this.root), { listing, expanded: true });
    const beneath = expanded
      .filter((folder) => folder.startsWith(this.root + "/") || folder.startsWith(this.root + "\\"))
      .sort((a, b) => a.length - b.length);
    await Promise.all(beneath.map((folder) => this.load(folder)));
    for (const folder of beneath) this.node(folder).expanded = true;
    this.render();
    this.onChange();
  }

  async load(path) {
    const node = this.node(path);
    node.loading = true;
    try {
      node.listing = await api.dir(path);
      node.error = null;
    } catch (error) {
      node.error = error.message;
    }
    node.loading = false;
  }

  async toggle(path) {
    const node = this.node(path);
    if (node.expanded) {
      node.expanded = false;
    } else {
      node.expanded = true;
      if (!node.listing) {
        node.loading = true;
        this.render();
        await this.load(path);
      }
    }
    this.render();
    this.onChange();
  }

  // refresh reloads every folder already loaded, keeping what is expanded.
  async refresh() {
    await this.open(this.root, this.expanded());
  }

  showMessage(text, isError) {
    this.message.textContent = text;
    this.message.className = "message" + (isError ? " error" : "");
    this.message.hidden = false;
  }

  render() {
    const root = this.node(this.root).listing;
    this.pathLabel.textContent = this.root || "";
    this.pathLabel.title = this.root || "";
    this.upButton.disabled = !root?.parent;
    this.homeButton.disabled = this.root === this.home;
    const rows = [];
    this.renderFolder(this.root, 0, rows);
    this.list.replaceChildren(...rows);
    if (root && !root.dirs.length && !root.files.length && this.message.hidden) {
      this.showMessage("No folders or GPX files here.", false);
    }
  }

  renderFolder(path, depth, rows) {
    const { listing } = this.node(path);
    if (!listing) return;
    for (const dir of listing.dirs) {
      const node = this.node(dir.path);
      const li = this.row(depth, "▶", dir.name, node.loading ? "…" : "", dir.path, [
        actionButton("⤓", "Make this folder the root", () => this.open(dir.path, this.expanded())),
      ]);
      li.classList.add("folder-row");
      li.classList.toggle("expanded", node.expanded);
      li.addEventListener("click", () => this.toggle(dir.path));
      rows.push(li);
      if (node.expanded) {
        if (node.error) {
          const error = this.row(depth + 1, "", node.error);
          error.classList.add("error-row");
          rows.push(error);
        } else {
          this.renderFolder(dir.path, depth + 1, rows);
        }
      }
    }
    for (const file of listing.files) {
      const entry = this.fileState(file.path);
      const li = this.row(depth, "", file.name, format.fileSize(file.size), file.path);
      li.classList.add("file-row");
      const glyph = li.querySelector(".glyph");
      if (entry) {
        // In the workspace: a filled dot when shown, a ring when hidden.
        li.classList.add("in-workspace");
        const dot = document.createElement("span");
        dot.className = "dot";
        if (entry.visible) dot.style.background = entry.color;
        else dot.style.boxShadow = `inset 0 0 0 2px ${entry.color}`;
        glyph.replaceChildren(dot);
      } else {
        glyph.textContent = "○";
      }
      li.title = `${entry ? "In the workspace — show and focus" : "Add to the workspace"}: ${file.path}`;
      li.addEventListener("click", () => this.onFile(file.path, li));
      rows.push(li);
    }
  }

  row(depth, glyph, name, meta, revealPath, actions = []) {
    const li = document.createElement("li");
    li.style.paddingLeft = `${8 + depth * 16}px`;
    const g = document.createElement("span");
    g.className = "glyph";
    g.textContent = glyph;
    const n = document.createElement("span");
    n.className = "name";
    n.textContent = name;
    li.append(g, n);
    if (meta) {
      const m = document.createElement("span");
      m.className = "meta";
      m.textContent = meta;
      li.append(m);
    }
    if (revealPath && this.canReveal) actions = [...actions, revealButton(revealPath)];
    if (actions.length) {
      const box = document.createElement("span");
      box.className = "actions";
      box.append(...actions);
      li.append(box);
    }
    return li;
  }
}

function actionButton(text, title, onClick) {
  const button = document.createElement("button");
  button.className = "row-action";
  button.textContent = text;
  button.title = title;
  button.setAttribute("aria-label", title);
  button.addEventListener("click", (event) => {
    event.stopPropagation();
    onClick();
  });
  return button;
}

// revealButton opens the file manager at path. It exists only when the page
// runs on the machine serving it.
export function revealButton(path) {
  const button = document.createElement("button");
  button.className = "row-action";
  button.textContent = "⧉";
  button.title = "Show in file manager";
  button.setAttribute("aria-label", "Show in file manager");
  button.addEventListener("click", async (event) => {
    event.stopPropagation();
    try {
      await api.reveal(path);
    } catch (error) {
      button.title = error.message;
    }
  });
  return button;
}
