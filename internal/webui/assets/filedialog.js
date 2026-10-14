import "/ui/dialog.js";

// The file dialog every dgs page opens and saves through: one folder browser,
// as Finder and Explorer have one, so a reader learns it once. It is shared
// because nothing in it is a page's own work — places down the side, a sorted
// listing in columns, a filter over the file types, and one selection coming
// back.
//
// It talks to the endpoints internal/webfile mounts, under /ui/files/. A page
// that mounts them gets the dialog with no code of its own.

import { listTable } from "/ui/listtable.js";
import { openMenu, closeMenu } from "/ui/menu.js";
import { iconSvg } from "/ui/icons.js";

const ENDPOINT = "/ui/files/";

// The columns, and how the list is sorted. The choice is kept between
// dialogs, as a file manager keeps it.
let sortBy = { key: "name", ascending: true };

// Whether entries a file manager keeps out of sight are shown. Also kept
// between dialogs, because it is a way of working rather than one choice.
let showHidden = false;

// The size the corner was last dragged to. The stylesheet gives the dialog a
// share of the window; a reader who wants another one says so once, and every
// dialog after it opens that size. Nothing is kept when it was never dragged.
let size = null;

// Whether the picked file is shown beside the listing. Kept in the browser,
// as the columns are kept between dialogs, and on until it is turned off.
const PREVIEW_KEY = "dgs-file-dialog-preview";
let previewing = true;
try { previewing = localStorage.getItem(PREVIEW_KEY) !== "off"; } catch { /* on */ }

// The filters a page offers when it names none: everything.
const ALL_FILES = { label: "All files", extensions: [] };

// openFile opens the dialog and resolves to the chosen path — an array of
// paths when multiple is set — or to null when it is cancelled.
//
//   folder      the folder to open at
//   fallbacks   folders to try, in order, when that one cannot be read
//   filters     [{ label, extensions: [".gpx"] }]; the first is chosen
//   multiple    several files may be chosen at once
//   folders     a folder is chosen instead of a file
export function openFile(options = {}) {
  return dialog({ mode: "open", confirm: "Open", title: "Open", ...options });
}

// saveFile asks where to write a file: the same browser, with a name field
// under it, a New Folder button in the bar, and F2 to rename what is there.
// It resolves to the full path, or to null when cancelled. The extension of
// the chosen filter is added when the name is left without one, and a name
// already taken is confirmed before it is answered — the dialog only chooses
// a path, so replacing the file stays the server's work.
export function saveFile(options = {}) {
  return dialog({ mode: "save", confirm: "Save", title: "Save", ...options });
}

// createFolder makes one folder inside another and answers its path.
export async function createFolder(parent, name, { endpoint = ENDPOINT } = {}) {
  const body = await postJSON(`${endpoint}folder`, { parent, name });
  return body.path;
}

// renameEntry gives a file or folder another name where it already is, and
// answers its new path.
export async function renameEntry(path, name, { endpoint = ENDPOINT } = {}) {
  const body = await postJSON(`${endpoint}rename`, { path, name });
  return body.path;
}

// nameTaken says whether a folder already holds this name, and what of:
// { taken, kind } with kind "file" or "dir".
export async function nameTaken(dir, name, { endpoint = ENDPOINT } = {}) {
  const query = new URLSearchParams({ path: dir, name });
  return getJSON(`${endpoint}taken?${query}`);
}

// listFolder reads one folder through the shared endpoint, so a page that
// draws its own tree lists it exactly as the dialog does.
export async function listFolder(path, { extensions = [], hidden = false, base = "", endpoint = ENDPOINT } = {}) {
  const query = new URLSearchParams();
  if (path) query.set("path", path);
  if (base) query.set("base", base);
  for (const ext of extensions) query.append("ext", ext);
  if (hidden) query.set("hidden", "1");
  return getJSON(`${endpoint}dir?${query}`);
}

// loadPlaces reads the places to offer down the dialog's side.
export async function loadPlaces({ endpoint = ENDPOINT } = {}) {
  const body = await getJSON(`${endpoint}places`);
  return body.places || [];
}

async function postJSON(url, body) {
  const response = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const answer = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(answer.error || `${response.status} ${response.statusText}`);
  return answer;
}

async function getJSON(url) {
  const response = await fetch(url);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `${response.status} ${response.statusText}`);
  return body;
}

// dialog builds the browser both modes share. The only differences are what
// a row's click means, and the name field save adds.
function dialog({
  mode,
  title,
  message = "",
  folder = "",
  fallbacks = [],
  filters = [],
  multiple = false,
  folders: pickFolders = false,
  name = "",
  // writable mounts what changes the disk: the New Folder button and F2 to
  // rename. Both modes offer it when the server permits writes.
  writable = true,
  // replace off is for a server that will not write over a file: the dialog
  // then refuses a name that is taken rather than asking to replace it, so it
  // never answers a path the write would be refused for.
  replace = true,
  places = null,
  confirm = "Open",
  cancel = "Cancel",
  endpoint = ENDPOINT,
}) {
  const choices = filters.length ? filters : [ALL_FILES];
  const saving = mode === "save";
  return new Promise((resolve) => {
    const separator = folder.includes("\\") && !folder.includes("/") ? "\\" : "/";
    const dialog = document.createElement("dialog");
    dialog.className = "dialog file-dialog";
    const form = document.createElement("form");
    form.method = "dialog";
    form.className = "file-form";

    const heading = document.createElement("h3");
    heading.className = "file-title";
    heading.textContent = title;
    form.append(heading);
    if (message) {
      const text = document.createElement("p");
      text.className = "file-message";
      text.textContent = message;
      form.append(text);
    }

    // The bar over the listing: the folder above, where we are, and refresh.
    const bar = document.createElement("div");
    bar.className = "file-bar";
    const back = document.createElement("button");
    back.type = "button";
    back.className = "icon-button";
    back.innerHTML = iconSvg("arrow-left");
    back.title = "The folder shown before";
    back.setAttribute("aria-label", "Back");
    back.disabled = true;
    const up = document.createElement("button");
    up.type = "button";
    up.className = "icon-button";
    up.innerHTML = iconSvg("arrow-up");
    up.title = "The folder above";
    up.setAttribute("aria-label", "The folder above");
    const where = document.createElement("input"); where.autocomplete = "off"; where.setAttribute("data-1p-ignore", ""); // not a login: password managers stay out
    where.type = "text";
    where.setAttribute("aria-label", "Folder path");
    where.title = "Type a path; Enter opens it, Tab completes it, Escape restores it.";
    where.className = "file-where";
    where.hidden = true;
    // Where we are reads as a trail of folders, each a button that opens it,
    // as in a desktop file manager; a click beside them edits the path as text.
    const crumbs = document.createElement("div");
    crumbs.className = "file-where file-crumbs";
    crumbs.title = "Click a folder to open it; click beside them to type a path.";
    const drawCrumbs = () => {
      crumbs.replaceChildren();
      let at = "";
      here.split(separator).forEach((name, i) => {
        if (i > 0 && !name) return;
        at = i === 0 ? (name ? name + separator : separator) : at + (at.endsWith(separator) ? "" : separator) + name;
        if (i > 0 || name) {
          const sep = document.createElement("span");
          sep.className = "file-crumb-separator";
          sep.textContent = "›";
          if (crumbs.childElementCount) crumbs.append(sep);
        }
        const crumb = document.createElement("button");
        crumb.type = "button";
        crumb.className = "file-crumb";
        crumb.textContent = name || separator;
        const target = at;
        crumb.addEventListener("click", (event) => { event.stopPropagation(); open(target); });
        crumbs.append(crumb);
      });
      // A deep folder keeps its end in view: that is where the reader is.
      crumbs.scrollLeft = crumbs.scrollWidth;
    };
    const editPath = (editing) => {
      crumbs.hidden = editing;
      where.hidden = !editing;
      if (editing) { where.focus(); where.select(); }
    };
    crumbs.addEventListener("click", () => editPath(true));
    where.addEventListener("blur", () => { ++completionRequest; where.value = here; editPath(false); });
    const editPathButton = document.createElement("button");
    editPathButton.type = "button";
    editPathButton.className = "text-button";
    editPathButton.textContent = "Edit path";
    editPathButton.addEventListener("click", () => editPath(true));
    const refresh = document.createElement("button");
    refresh.type = "button";
    refresh.className = "icon-button";
    refresh.innerHTML = iconSvg("rotate-cw");
    refresh.title = "Read this folder again";
    refresh.setAttribute("aria-label", "Read this folder again");
    const newFolder = document.createElement("button");
    newFolder.type = "button";
    newFolder.className = "text-button file-new-folder";
    newFolder.textContent = "New Folder";
    newFolder.title = "Make a folder here";
    const eye = document.createElement("button");
    eye.type = "button";
    eye.className = "text-button file-preview-toggle";
    eye.textContent = "Preview";
    eye.title = "Show the picked file beside the listing";
    bar.append(back, up, crumbs, where, editPathButton, refresh, eye);
    if (writable) bar.append(newFolder);
    form.append(bar);

    // Places down the side, the listing beside them.
    const body = document.createElement("div");
    body.className = "file-body";
    const sidebar = document.createElement("ul");
    sidebar.className = "file-places";
    const pane = document.createElement("div");
    pane.className = "file-listing";
    // The listing is the shared list table; folders stay before files
    // however it is sorted, so the dialog sorts and the table draws.
    const list = document.createElement("div");
    list.className = "file-list";
    list.tabIndex = 0;
    pane.append(list);
    const cellOf = (className, text) => {
      const td = document.createElement("td");
      td.className = className;
      td.textContent = text;
      return td;
    };
    const table = listTable(list, {
      key: "dgs-file-dialog-list",
      className: "file-table",
      chooser: false,
      columns: [
        { id: "name", text: "Name", width: 260, cell: (entry) => cellOf("file-cell-name", entry.name) },
        { id: "modified", text: "Date Modified", width: 130, down: true, cell: (entry) => cellOf("file-cell-modified numeric", when(entry.modified)) },
        { id: "size", text: "Size", width: 76, down: true,
          cell: (entry) => cellOf("file-cell-size numeric", entry.kind === "folder" ? "" : fileSize(entry.size || 0)) },
      ],
      sorting: {
        get: () => ({ id: sortBy.key, down: !sortBy.ascending }),
        set: ({ id, down }) => { sortBy = { key: id, ascending: !down }; render(); },
      },
      row: (entry, cells) => row(entry, cells),
      empty: "Empty",
    });
    // rowItems are the rows drawn, in the order of rows.
    const rowItems = () => (table.table ? [...table.table.tBodies[0].rows] : []);
    // The preview: the one picked file as it is — a PDF in the browser's
    // own viewer, an image, the start of a text file — or why not.
    const preview = document.createElement("div");
    preview.className = "file-preview";
    body.append(sidebar, pane, preview);
    form.append(body);

    const problem = document.createElement("p");
    problem.className = "file-problem";
    problem.hidden = true;
    form.append(problem);

    // The foot: the name field when saving, the type filter, the hidden
    // toggle, and the two buttons.
    const foot = document.createElement("div");
    foot.className = "file-foot";
    const input = document.createElement("input"); input.autocomplete = "off"; input.setAttribute("data-1p-ignore", ""); // not a login: password managers stay out
    input.type = "text";
    input.className = "file-name";
    input.value = name;
    input.setAttribute("aria-label", "File name");
    if (saving) foot.append(input);

    const filter = document.createElement("select");
    filter.className = "file-filter";
    filter.setAttribute("aria-label", "Show");
    choices.forEach((choice, index) => {
      const option = document.createElement("option");
      option.value = String(index);
      option.textContent = choice.label;
      filter.append(option);
    });
    filter.disabled = choices.length < 2;
    const hidden = document.createElement("label");
    hidden.className = "file-hidden";
    const hiddenBox = document.createElement("input");
    hiddenBox.type = "checkbox";
    hiddenBox.checked = showHidden;
    hidden.append(hiddenBox, document.createTextNode(" Hidden files"));

    const buttons = document.createElement("div");
    buttons.className = "dialog-buttons";
    const no = document.createElement("button");
    no.type = "button";
    no.className = "button";
    no.textContent = cancel;
    const yes = document.createElement("button");
    yes.type = "submit";
    yes.className = "action";
    yes.textContent = confirm;
    buttons.append(no, yes);
    foot.append(filter, hidden, buttons);
    form.append(foot);
    dialog.append(form);

    let here = folder;
    let parent = "";
    let entries = { dirs: [], files: [] };
    let rows = [];        // what is drawn, folders first, in sorted order
    let chosen = new Set(); // paths selected in the listing
    let anchor = -1;      // where a shift-click range starts
    let typed = "";       // the name being typed to jump to a row
    let typedAt = 0;
    let editing = false;  // a name is being typed into the listing itself

    const extensions = () => choices[Number(filter.value)]?.extensions || [];

    // A row per folder and per file: name, when it was last written, and a
    // file's size.
    const row = (entry, cells) => {
      const kind = entry.kind;
      const index = rows.indexOf(entry);
      const item = document.createElement("tr");
      item.className = `file-row file-${kind}`;
      item.dataset.path = entry.path;
      item.title = entry.path;
      if (chosen.has(entry.path)) item.classList.add("selected");
      item.append(...cells);
      item.addEventListener("click", (event) => select(index, event));
      item.addEventListener("dblclick", () => {
        if (kind === "folder") open(entry.path);
        else if (!pickFolders) submit();
      });
      item.addEventListener("contextmenu", (event) => {
        if (!writable || editing) return;
        select(index);
        openMenu(event, [[{ label: "Rename…", onSelect: renameChosen }]]);
      });
      return item;
    };

    // select marks a row. Any row may be marked, to preview a file or step
    // into a folder; pickedPaths keeps only what the dialog answers with. Shift extends from the row the selection started
    // at, and Cmd or Ctrl adds one row, as a file manager does — both only
    // where several files may be chosen.
    const select = (index, event = {}) => {
      const entry = rows[index];
      if (!entry) return;
      const many = multiple && !pickFolders;
      if (many && event.shiftKey && anchor >= 0) {
        const [from, to] = anchor < index ? [anchor, index] : [index, anchor];
        chosen = new Set();
        for (let i = from; i <= to; i++) {
          if (selectable(rows[i])) chosen.add(rows[i].path);
        }
      } else if (many && (event.metaKey || event.ctrlKey)) {
        if (chosen.has(entry.path)) chosen.delete(entry.path);
        else if (selectable(entry)) chosen.add(entry.path);
        anchor = index;
      } else {
        chosen = new Set([entry.path]);
        anchor = index;
      }
      if (saving && entry.kind === "file") input.value = entry.name;
      draw();
    };

    // showPreview draws the picked file in the preview, only when the pick
    // changed, so moving through the listing does not reload what is shown.
    let shown = null;
    const note = (text) => {
      const p = document.createElement("p");
      p.className = "file-preview-note";
      p.textContent = text;
      return p;
    };
    const showPreview = () => {
      eye.setAttribute("aria-pressed", String(previewing));
      preview.hidden = !previewing;
      if (!previewing) { shown = null; preview.textContent = ""; return; }
      const picked = rows.filter((entry) => chosen.has(entry.path));
      const entry = picked.length === 1 ? picked[0] : null;
      const key = entry ? entry.path : picked.length > 1 ? "*" : "";
      if (key === shown) return;
      shown = key;
      preview.textContent = "";
      if (!entry) return preview.append(note(picked.length > 1 ? picked.length + " files picked" : "Pick a file to see it here."));
      if (entry.kind === "folder") return preview.append(note("A folder: open it to see what is in it."));
      const src = `${endpoint}preview?${new URLSearchParams({ path: entry.path })}`;
      if (entry.preview === "pdf") {
        const frame = document.createElement("iframe");
        frame.title = entry.name;
        frame.src = src + "#toolbar=0&navpanes=0&view=FitH";
        preview.append(frame);
      } else if (entry.preview === "image") {
        const image = document.createElement("img");
        image.alt = entry.name;
        image.src = src;
        preview.append(image);
      } else if (entry.preview === "text") {
        const text = document.createElement("pre");
        preview.append(text);
        fetch(src).then((r) => r.ok ? r.text() : Promise.reject(new Error(r.statusText)))
          .then((body) => { if (shown === key) text.textContent = body; })
          .catch((error) => { if (shown === key) { preview.textContent = ""; preview.append(note(error.message)); } });
      } else {
        preview.append(note("No preview for this kind of file."));
      }
    };
    eye.addEventListener("click", () => {
      previewing = !previewing;
      try { localStorage.setItem(PREVIEW_KEY, previewing ? "" : "off"); } catch { /* not kept */ }
      shown = null;
      showPreview();
    });

    // selectable says whether confirming on this row would answer with it:
    // a file, or a folder when the dialog asks for one.
    const selectable = (entry) => Boolean(entry) && (pickFolders ? entry.kind === "folder" : entry.kind === "file");

    // pickedPaths are the marked rows the dialog may answer with. A folder is
    // marked to step into it and to rename it, so a dialog asking for files
    // never answers one: the mark is where the reader is, not what is chosen.
    const pickedPaths = () => rows.filter((entry) => chosen.has(entry.path) && selectable(entry)).map((entry) => entry.path);

    // render sorts the listing and draws it; draw only repaints the marks,
    // so moving the selection does not rebuild the rows.
    const render = () => {
      const sorted = (list) => [...list].sort(compare(sortBy));
      rows = [
        ...sorted(entries.dirs).map((entry) => ({ ...entry, kind: "folder" })),
        ...sorted(entries.files).map((entry) => ({ ...entry, kind: "file" })),
      ];
      table.draw(rows);
      draw();
    };

    const draw = () => {
      for (const item of rowItems()) {
        if (item.dataset.path) item.classList.toggle("selected", chosen.has(item.dataset.path));
      }
      showPreview();
      const ready = saving ? Boolean(input.value.trim()) : pickedPaths().length > 0;
      yes.disabled = !ready && !(pickFolders && here);
    };

    // open reads a folder and draws it. A folder that cannot be read falls
    // back to the next one the caller offered, and says why when none is left.
    // visited are the folders shown before this one, for Back; going back
    // does not add the folder it leaves.
    const visited = [];
    let lastShown = "";
    const open = async (path, ...rest) => {
      problem.hidden = true;
      let listing;
      try {
        listing = await listFolder(path, { extensions: extensions(), hidden: showHidden, base: here, endpoint });
      } catch (error) {
        if (rest.length) return open(rest[0], ...rest.slice(1));
        problem.textContent = error.message;
        problem.hidden = false;
        return;
      }
      if (listing.writable === false) writable = false;
      if (lastShown && listing.path !== lastShown && !open.goingBack) visited.push(lastShown);
      lastShown = listing.path;
      back.disabled = visited.length === 0;
      here = listing.path;
      parent = listing.parent || "";
      where.value = here;
      drawCrumbs();
      newFolder.hidden = !writable;
      up.disabled = !parent;
      chosen = new Set();
      anchor = -1;
      for (const item of sidebar.children) {
        item.firstChild.classList.toggle("here", item.firstChild.dataset.path === here);
      }
      entries = { dirs: listing.dirs, files: listing.files };
      render();
      // The keys act on the listing, so a folder opened from the places or
      // the filter hands the keyboard back to it.
      if (!saving) list.focus();
    };

    const drawPlaces = (list) => {
      sidebar.textContent = "";
      for (const spot of list) {
        const item = document.createElement("li");
        const button = document.createElement("button");
        button.type = "button";
        button.className = "file-place";
        button.textContent = spot.name;
        button.title = spot.path;
        button.dataset.path = spot.path;
        button.classList.toggle("here", spot.path === here);
        button.addEventListener("click", () => open(spot.path));
        item.append(button);
        sidebar.append(item);
      }
      sidebar.hidden = list.length === 0;
    };

    // say puts a reason under the listing, or takes it away.
    const say = (reason) => {
      problem.textContent = reason || "";
      problem.hidden = !reason;
    };

    // askName is the one editor both writing actions use: an input in place
    // of a row's name. Enter or leaving it commits, Esc gives up — and Esc
    // is kept from the dialog, which would otherwise cancel the whole thing.
    // The commit reads the folder again, so what came back from the server is
    // what is drawn.
    const askName = (cell, value, commit) => {
      editing = true;
      const field = document.createElement("input"); field.autocomplete = "off"; field.setAttribute("data-1p-ignore", ""); // not a login: password managers stay out
      field.type = "text";
      field.className = "file-rename";
      field.setAttribute("aria-label", "Rename");
      field.value = value;
      cell.textContent = "";
      cell.append(field);
      let done = false;
      const stop = (renamed) => {
        if (done) return;
        done = true;
        editing = false;
        if (!renamed) render();
      };
      const finish = async () => {
        if (done) return;
        const typedName = field.value.trim();
        if (!typedName || typedName === value) return stop(false);
        if (/[\\/]/.test(typedName)) {
          say("A name cannot hold a path.");
          field.focus();
          return;
        }
        done = true;
        editing = false;
        try {
          const path = await commit(typedName);
          say("");
          await open(here);
          const at = rows.findIndex((entry) => entry.path === path);
          if (at >= 0) select(at);
        } catch (error) {
          await open(here);
          say(error.message);
        }
      };
      field.addEventListener("keydown", (event) => {
        event.stopPropagation();
        if (event.key === "Enter") { event.preventDefault(); finish(); }
        if (event.key === "Escape") { event.preventDefault(); stop(false); }
      });
      field.addEventListener("blur", finish);
      field.focus();
      field.select();
    };

    // makeFolder asks for the name on a row of its own at the top of the
    // listing, where the folder will be once it is made.
    const makeFolder = () => {
      if (editing) return;
      say("");
      const item = document.createElement("tr");
      item.className = "file-row file-folder";
      const cell = document.createElement("td");
      cell.className = "file-cell-name";
      cell.colSpan = 3;
      item.append(cell);
      if (table.table) table.table.tBodies[0].prepend(item);
      else {
        const holder = document.createElement("table");
        holder.className = "list-table file-table";
        holder.createTBody().append(item);
        list.replaceChildren(holder);
      }
      item.scrollIntoView({ block: "nearest" });
      askName(cell, "untitled folder", (typedName) => createFolder(here, typedName, { endpoint }));
    };

    // renameChosen renames the one row that is selected, in place.
    const renameChosen = () => {
      if (editing) return;
      const index = rows.findIndex((entry) => chosen.has(entry.path));
      const entry = rows[index];
      if (!entry || chosen.size !== 1) return;
      const item = rowItems()[index];
      const cell = item?.querySelector(".file-cell-name");
      if (!cell) return;
      say("");
      askName(cell, entry.name, (typedName) => renameEntry(entry.path, typedName, { endpoint }));
    };

    // replaces asks before a save answers a name that is already there. A
    // folder of that name is refused instead: a save never replaces a folder.
    const replaces = async (typedName) => {
      let state;
      try {
        state = await nameTaken(here, typedName, { endpoint });
      } catch (error) {
        say(error.message);
        return false;
      }
      if (!state.taken) return true;
      if (state.kind === "dir") {
        say(`A folder is already called ${typedName}.`);
        return false;
      }
      if (!replace) {
        say(`${typedName} is already in this folder. Name it something else.`);
        return false;
      }
      return confirmReplace(typedName);
    };

    // withExtension is the name the save answers: the chosen filter's first
    // extension is added when the typed name was left without one.
    const withExtension = (typedName) => {
      const exts = extensions();
      if (!exts.length) return typedName;
      if (exts.some((ext) => typedName.toLowerCase().endsWith(dotted(ext).toLowerCase()))) return typedName;
      return typedName + dotted(exts[0]);
    };

    // submit answers with what is chosen. Saving adds the filter's extension
    // when the typed name has none, and confirms a name already taken.
    const submit = async () => {
      if (editing) return;
      if (saving) {
        const typedName = input.value.trim();
        if (!typedName) return;
        if (/[\\/]/.test(typedName)) return say("A name cannot hold a path.");
        const full = withExtension(typedName);
        // ".gpx" alone has no name: it would be a hidden file.
        if (extensions().some((ext) => full.toLowerCase() === dotted(ext).toLowerCase())) return say("Give the file a name.");
        if (!(await replaces(full))) return;
        answer = `${here.replace(/[\\/]$/, "")}${separator}${full}`;
      } else {
        const picked = pickedPaths();
        if (!picked.length) {
          if (pickFolders && here) answer = multiple ? [here] : here;
          else return;
        } else {
          answer = multiple ? picked : picked[0];
        }
      }
      dialog.close();
    };

    // Keys, as a file manager answers them: the arrows move the selection,
    // Right or Enter steps into a folder, Left or Backspace goes up, and
    // typing jumps to the row whose name starts that way.
    const keys = (event) => {
      if (event.target === editPathButton) return;
      if (event.target === input || event.target === where || event.target.closest(".context-menu") || editing) return;
      if (event.key === "F2" && writable) { event.preventDefault(); return renameChosen(); }
      const index = rows.findIndex((entry) => chosen.has(entry.path));
      const move = (to) => {
        event.preventDefault();
        const next = Math.max(0, Math.min(rows.length - 1, to));
        select(next);
        rowItems()[next]?.scrollIntoView({ block: "nearest" });
      };
      switch (event.key) {
        case "ArrowDown": return move(index < 0 ? 0 : index + 1);
        case "ArrowUp": return move(index < 0 ? rows.length - 1 : index - 1);
        case "Home": return move(0);
        case "End": return move(rows.length - 1);
        case "ArrowRight":
          if (rows[index]?.kind === "folder") { event.preventDefault(); open(rows[index].path); }
          return;
        case "ArrowLeft":
        case "Backspace":
          if (parent) { event.preventDefault(); open(parent); }
          return;
        case "Enter":
          if (rows[index]?.kind === "folder" && !pickFolders) { event.preventDefault(); open(rows[index].path); }
          return;
        default:
          break;
      }
      if (event.key.length === 1 && !event.metaKey && !event.ctrlKey && !event.altKey) {
        const now = Date.now();
        typed = now - typedAt > 900 ? event.key : typed + event.key;
        typedAt = now;
        const at = rows.findIndex((entry) => entry.name.toLowerCase().startsWith(typed.toLowerCase()));
        if (at >= 0) move(at);
      }
    };

    let completionRequest = 0;
    // Completion is inline, as in an address bar: what the path can only go on
    // to is added after the caret and selected, so typing on replaces it.
    const completePath = async (tab = false, inline = false) => {
      const value = where.value;
      const request = ++completionRequest;
      try {
        const result = await getJSON(`${endpoint}complete?${new URLSearchParams({ path: value, base: here })}`);
        if (request !== completionRequest || where.value !== value || !dialog.open) return;
        if (inline && result.matches.length && result.completion.length > value.length && result.completion.startsWith(value)) {
          where.value = result.completion;
          where.setSelectionRange(value.length, where.value.length);
        } else if (tab && result.matches.length) {
          where.value = result.completion;
          where.setSelectionRange(where.value.length, where.value.length);
        }
      } catch (error) { if (tab) say(error.message); }
    };
    // Only typing completes: deleting must not bring back what was deleted.
    where.addEventListener("input", (event) => {
      if (event.inputType?.startsWith("insert") && where.selectionEnd === where.value.length) completePath(false, true);
    });
    where.addEventListener("keydown", (event) => {
      event.stopPropagation();
      if (event.key === "Enter") { event.preventDefault(); open(where.value); where.blur(); }
      if (event.key === "Tab" && !event.shiftKey) { event.preventDefault(); completePath(true); }
      if (event.key === "Escape") { event.preventDefault(); ++completionRequest; where.value = here; where.blur(); }
    });
    let answer = null;
    up.addEventListener("click", () => parent && open(parent));
    back.addEventListener("click", async () => {
      if (!visited.length) return;
      open.goingBack = true;
      try { await open(visited.pop()); } finally { open.goingBack = false; }
      back.disabled = visited.length === 0;
    });
    refresh.addEventListener("click", () => open(here));
    // Changing the filter changes the name's extension with it, as a save
    // dialog does: the name stays, the type follows the choice.
    let filterWas = Number(filter.value);
    filter.addEventListener("change", () => {
      if (saving) {
        const old = choices[filterWas]?.extensions || [];
        const now = extensions();
        const typedName = input.value.trim();
        const hit = old.find((ext) => typedName.toLowerCase().endsWith(dotted(ext).toLowerCase()));
        if (typedName && now.length && hit) {
          input.value = typedName.slice(0, -dotted(hit).length) + dotted(now[0]);
        }
      }
      filterWas = Number(filter.value);
      open(here);
    });
    newFolder.addEventListener("click", makeFolder);
    hiddenBox.addEventListener("change", () => { showHidden = hiddenBox.checked; open(here); });
    input.addEventListener("input", draw);
    no.addEventListener("click", () => dialog.close());
    form.addEventListener("submit", (event) => { event.preventDefault(); submit(); });
    dialog.addEventListener("keydown", keys);
    dialog.addEventListener("close", () => { closeMenu(); dialog.remove(); resolve(answer); });

    // The size the reader chose, and the size they choose now.
    if (size) {
      dialog.style.width = size.width;
      dialog.style.height = size.height;
    }
    new ResizeObserver(() => {
      if (dialog.open && dialog.style.width) size = { width: dialog.style.width, height: dialog.style.height };
    }).observe(dialog);

    document.body.append(dialog);
    dialog.showModal();
    (places ? Promise.resolve(places) : loadPlaces({ endpoint }).catch(() => []))
      .then((list) => drawPlaces(list));
    open(folder, ...fallbacks, "");
    if (saving) {
      input.focus();
      const exts = extensions();
      const dot = exts.length ? input.value.toLowerCase().lastIndexOf(exts[0].toLowerCase()) : -1;
      input.setSelectionRange(0, dot > 0 ? dot : input.value.length);
    } else {
      list.focus();
    }
  });
}

// compare orders two entries by the column the header chose. Names are
// compared as the reader reads them, so 2 comes before 10 and case does not
// matter; a missing time or size sorts as the oldest and the smallest.
function compare({ key, ascending }) {
  const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
  const direction = ascending ? 1 : -1;
  return (a, b) => {
    let order = 0;
    if (key === "modified") order = time(a.modified) - time(b.modified);
    else if (key === "size") order = (a.size || 0) - (b.size || 0);
    if (order === 0) order = collator.compare(a.name, b.name) * (key === "name" ? 1 : direction);
    return order * direction;
  };
}

function time(value) {
  const millis = value ? Date.parse(value) : NaN;
  return Number.isNaN(millis) ? 0 : millis;
}

// when is a file's time as a file manager writes it: the date, and the time
// of day in the browser's zone.
function when(value) {
  const millis = time(value);
  if (!millis) return "";
  const date = new Date(millis);
  const pad = (number) => String(number).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

// fileSize is a size in the units a file manager shows, kB upwards in
// thousands, as macOS counts them.
export function fileSize(bytes) {
  if (bytes < 1000) return `${bytes} B`;
  const units = ["kB", "MB", "GB", "TB"];
  let value = bytes / 1000;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit++;
  }
  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[unit]}`;
}

// dotted is an extension as a name ends with it, whether the page wrote
// ".gpx" or "gpx".
function dotted(extension) {
  return extension.startsWith(".") ? extension : `.${extension}`;
}

// confirmReplace asks, over the dialog, before a save answers a name that is
// already a file. It resolves true when the reader says to replace it.
function confirmReplace(name) {
  return new Promise((resolve) => {
    const ask = document.createElement("dialog");
    ask.className = "dialog file-replace";
    const heading = document.createElement("h3");
    heading.textContent = "Replace the file?";
    const text = document.createElement("p");
    text.textContent = `${name} is already in this folder. Saving writes over it.`;
    const buttons = document.createElement("div");
    buttons.className = "dialog-buttons";
    const no = document.createElement("button");
    no.type = "button";
    no.className = "button";
    no.textContent = "Cancel";
    const yes = document.createElement("button");
    yes.type = "button";
    yes.className = "action";
    yes.textContent = "Replace";
    let answer = false;
    no.addEventListener("click", () => ask.close());
    yes.addEventListener("click", () => { answer = true; ask.close(); });
    buttons.append(no, yes);
    ask.append(heading, text, buttons);
    ask.addEventListener("close", () => { ask.remove(); resolve(answer); });
    document.body.append(ask);
    ask.showModal();
    yes.focus();
  });
}
