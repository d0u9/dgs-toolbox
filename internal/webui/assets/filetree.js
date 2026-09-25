// The file tree every dgs page draws a set of paths with: folders first, a
// folder and file icon on each row, and a guide line down each open folder
// so the depth reads at a glance. It draws and reports clicks; what a row
// means stays with the page.
//
//   const tree = fileTree(files, {
//     closed,       Set of folder paths ("a/b/") drawn shut; changed in place
//     selected,     the path drawn as picked
//     onPick,       called with the file whose row was clicked
//     fileExtra,    (file) → a node or null drawn after the name
//     folderExtra,  (path, files) → a node or null drawn after a folder name
//     fileClass,    (file) → extra class names for the row, or ""
//     mark,         (file) → a title for a check drawn after the name, or ""
//     href,         (file) → a link for the name instead of onPick
//   });
//
// files are objects with a slash-separated path. Changing closed or selected
// redraws nothing: the page draws the tree again, as it does anything else.

const FOLDER = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.75 3.25h4.5l1.5 1.5h6.5v8H1.75z" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/><path d="M1.75 6.25h12.5" stroke="currentColor" stroke-width="1.25"/></svg>';
const FILE = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3.25 1.75h6l3.5 3.5v9h-9.5z M9.25 1.75v3.5h3.5" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/></svg>';
const CHECK = '<svg viewBox="0 0 16 16"><circle cx="8" cy="8" r="6.5" fill="currentColor"/><path d="M5 8.2l2 2 4-4.2" fill="none" stroke="#fff" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>';

function nest(files) {
  const top = { dirs: new Map(), files: [] };
  for (const file of files) {
    const parts = file.path.split("/");
    let node = top;
    for (const part of parts.slice(0, -1)) {
      if (!node.dirs.has(part)) node.dirs.set(part, { dirs: new Map(), files: [] });
      node = node.dirs.get(part);
    }
    node.files.push(file);
  }
  return top;
}

function under(node) {
  let all = [...node.files];
  for (const child of node.dirs.values()) all = all.concat(under(child));
  return all;
}

function icon(svg) {
  const span = document.createElement("span");
  span.className = "ft-icon";
  span.innerHTML = svg;
  return span;
}

function row(kind, name, title) {
  const div = document.createElement("div");
  div.className = "ft-row ft-" + kind;
  div.title = title;
  div.append(icon(kind === "folder" ? FOLDER : FILE));
  const label = document.createElement("span");
  label.className = "ft-name";
  label.textContent = name;
  div.append(label);
  return div;
}

export function fileTree(files, options = {}) {
  const { closed = new Set(), selected = "", onPick, fileExtra, folderExtra, fileClass, mark, href } = options;
  const draw = (node, prefix) => {
    const ul = document.createElement("ul");
    ul.className = "ft-list";
    for (const [name, child] of [...node.dirs.entries()].sort(([a], [b]) => a.localeCompare(b))) {
      const path = prefix + name + "/";
      const li = document.createElement("li");
      const open = !closed.has(path);
      const r = row("folder", name, path);
      r.setAttribute("role", "button");
      r.setAttribute("aria-expanded", String(open));
      r.tabIndex = 0;
      const extra = folderExtra && folderExtra(path, under(child));
      if (extra) r.append(extra);
      const toggle = () => {
        if (closed.has(path)) closed.delete(path); else closed.add(path);
        const shut = closed.has(path);
        r.setAttribute("aria-expanded", String(!shut));
        sub.hidden = shut;
      };
      r.addEventListener("click", toggle);
      r.addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); toggle(); } });
      const sub = draw(child, path);
      sub.hidden = !open;
      li.append(r, sub);
      ul.append(li);
    }
    for (const file of node.files) {
      const li = document.createElement("li");
      const name = file.path.slice(file.path.lastIndexOf("/") + 1);
      const r = row("file", "", file.path);
      const label = r.querySelector(".ft-name");
      if (href) {
        const a = document.createElement("a");
        a.href = href(file);
        a.textContent = name;
        label.append(a);
      } else {
        label.textContent = name;
      }
      if (fileClass) { const c = fileClass(file); if (c) r.classList.add(...c.split(" ")); }
      if (file.path === selected) r.classList.add("selected");
      const why = mark && mark(file);
      if (why) {
        const check = icon(CHECK);
        check.className = "ft-mark";
        check.title = why;
        check.setAttribute("aria-label", why);
        label.after(check);
      }
      const extra = fileExtra && fileExtra(file);
      if (extra) r.append(extra);
      if (onPick) {
        r.tabIndex = 0;
        r.addEventListener("click", () => onPick(file));
        r.addEventListener("keydown", (e) => { if (e.key === "Enter") onPick(file); });
      }
      li.append(r);
      ul.append(li);
    }
    return ul;
  };
  const root = document.createElement("div");
  root.className = "file-tree";
  root.append(draw(nest(files), ""));
  return root;
}
