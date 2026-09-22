// The file dialog every dgs page opens and saves through: one folder browser,
// as Finder and Explorer have one, so a reader learns it once. It is shared
// because nothing in it is a page's own work — places down the side, a sorted
// listing in columns, a filter over the file types, and one selection coming
// back.
//
// It talks to the endpoints internal/webfile mounts, under /ui/files/. A page
// that mounts them gets the dialog with no code of its own.

const ENDPOINT = "/ui/files/";

// The columns, and how the list is sorted. The choice is kept between
// dialogs, as a file manager keeps it.
const COLUMNS = [
  { key: "name", label: "Name" },
  { key: "modified", label: "Date Modified" },
  { key: "size", label: "Size" },
];
let sortBy = { key: "name", ascending: true };

// Whether entries a file manager keeps out of sight are shown. Also kept
// between dialogs, because it is a way of working rather than one choice.
let showHidden = false;

// The size the corner was last dragged to. The stylesheet gives the dialog a
// share of the window; a reader who wants another one says so once, and every
// dialog after it opens that size. Nothing is kept when it was never dragged.
let size = null;

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
export async function listFolder(path, { extensions = [], hidden = false, endpoint = ENDPOINT } = {}) {
  const query = new URLSearchParams();
  if (path) query.set("path", path);
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
  // rename. Saving asks for it; opening does not, unless the page says so.
  writable = mode === "save",
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
    const up = document.createElement("button");
    up.type = "button";
    up.className = "icon-button";
    up.textContent = "↑";
    up.title = "The folder above";
    up.setAttribute("aria-label", "The folder above");
    const where = document.createElement("code");
    where.className = "file-where";
    const refresh = document.createElement("button");
    refresh.type = "button";
    refresh.className = "icon-button";
    refresh.textContent = "↻";
    refresh.title = "Read this folder again";
    refresh.setAttribute("aria-label", "Read this folder again");
    const newFolder = document.createElement("button");
    newFolder.type = "button";
    newFolder.className = "text-button file-new-folder";
    newFolder.textContent = "New Folder";
    newFolder.title = "Make a folder here";
    bar.append(up, where, refresh);
    if (writable) bar.append(newFolder);
    form.append(bar);

    // Places down the side, the listing beside them.
    const body = document.createElement("div");
    body.className = "file-body";
    const sidebar = document.createElement("ul");
    sidebar.className = "file-places";
    const pane = document.createElement("div");
    pane.className = "file-listing";
    const header = document.createElement("div");
    header.className = "file-header";
    const markers = {};
    for (const column of COLUMNS) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = `file-heading file-cell-${column.key}`;
      const label = document.createElement("span");
      label.textContent = column.label;
      const marker = document.createElement("span");
      marker.className = "file-marker";
      button.append(label, marker);
      button.addEventListener("click", () => {
        sortBy = sortBy.key === column.key
          ? { key: column.key, ascending: !sortBy.ascending }
          : { key: column.key, ascending: column.key === "name" };
        render();
      });
      markers[column.key] = marker;
      header.append(button);
    }
    const list = document.createElement("ul");
    list.className = "file-list";
    list.tabIndex = 0;
    pane.append(header, list);
    body.append(sidebar, pane);
    form.append(body);

    const problem = document.createElement("p");
    problem.className = "file-problem";
    problem.hidden = true;
    form.append(problem);

    // The foot: the name field when saving, the type filter, the hidden
    // toggle, and the two buttons.
    const foot = document.createElement("div");
    foot.className = "file-foot";
    const input = document.createElement("input");
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
    no.className = "text-button";
    no.textContent = cancel;
    const yes = document.createElement("button");
    yes.type = "submit";
    yes.className = "chip active";
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
    const row = (entry, kind, index) => {
      const item = document.createElement("li");
      item.className = `file-row file-${kind}`;
      item.dataset.path = entry.path;
      item.title = entry.path;
      if (chosen.has(entry.path)) item.classList.add("chosen");
      const label = document.createElement("span");
      label.className = "file-cell-name";
      label.textContent = entry.name;
      const modified = document.createElement("span");
      modified.className = "file-cell-modified numeric";
      modified.textContent = when(entry.modified);
      const size = document.createElement("span");
      size.className = "file-cell-size numeric";
      size.textContent = kind === "folder" ? "" : fileSize(entry.size || 0);
      item.append(label, modified, size);
      item.addEventListener("click", (event) => select(index, event));
      item.addEventListener("dblclick", () => {
        if (kind === "folder") open(entry.path);
        else submit();
      });
      return item;
    };

    // select marks a row. Shift extends from the row the selection started
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
        chosen = selectable(entry) || entry.kind === "folder" ? new Set([entry.path]) : new Set();
        anchor = index;
      }
      if (saving && entry.kind === "file") input.value = entry.name;
      draw();
    };

    // selectable says whether confirming on this row would answer with it:
    // a file, or a folder when the dialog asks for one.
    const selectable = (entry) => Boolean(entry) && (pickFolders ? entry.kind === "folder" : entry.kind === "file");

    // render sorts the listing and draws it; draw only repaints the marks,
    // so moving the selection does not rebuild the rows.
    const render = () => {
      for (const column of COLUMNS) {
        markers[column.key].textContent = sortBy.key === column.key ? (sortBy.ascending ? "^" : "v") : "";
      }
      const sorted = (list) => [...list].sort(compare(sortBy));
      rows = [
        ...sorted(entries.dirs).map((entry) => ({ ...entry, kind: "folder" })),
        ...sorted(entries.files).map((entry) => ({ ...entry, kind: "file" })),
      ];
      list.textContent = "";
      rows.forEach((entry, index) => list.append(row(entry, entry.kind, index)));
      if (!rows.length) {
        const item = document.createElement("li");
        item.className = "file-empty";
        item.textContent = "Empty";
        list.append(item);
      }
      draw();
    };

    const draw = () => {
      for (const item of list.children) {
        if (item.dataset.path) item.classList.toggle("chosen", chosen.has(item.dataset.path));
      }
      const ready = saving ? Boolean(input.value.trim()) : [...chosen].some((path) => path);
      yes.disabled = !ready && !(pickFolders && here);
    };

    // open reads a folder and draws it. A folder that cannot be read falls
    // back to the next one the caller offered, and says why when none is left.
    const open = async (path, ...rest) => {
      problem.hidden = true;
      let listing;
      try {
        listing = await listFolder(path, { extensions: extensions(), hidden: showHidden, endpoint });
      } catch (error) {
        if (rest.length) return open(rest[0], ...rest.slice(1));
        list.textContent = "";
        problem.textContent = error.message;
        problem.hidden = false;
        return;
      }
      here = listing.path;
      parent = listing.parent || "";
      where.textContent = here;
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
      const field = document.createElement("input");
      field.type = "text";
      field.className = "file-rename";
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
          say(error.message);
          await open(here);
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
      const item = document.createElement("li");
      item.className = "file-row file-folder";
      const cell = document.createElement("span");
      cell.className = "file-cell-name";
      item.append(cell);
      list.prepend(item);
      item.scrollIntoView({ block: "nearest" });
      askName(cell, "untitled folder", (typedName) => createFolder(here, typedName, { endpoint }));
    };

    // renameChosen renames the one row that is selected, in place.
    const renameChosen = () => {
      if (editing) return;
      const index = rows.findIndex((entry) => chosen.has(entry.path));
      const entry = rows[index];
      if (!entry || chosen.size !== 1) return;
      const item = list.children[index];
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
        if (!(await replaces(full))) return;
        answer = `${here.replace(/[\\/]$/, "")}${separator}${full}`;
      } else {
        const picked = [...chosen];
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
      if (event.target === input || editing) return;
      if (event.key === "F2" && writable) { event.preventDefault(); return renameChosen(); }
      const index = rows.findIndex((entry) => chosen.has(entry.path));
      const move = (to) => {
        event.preventDefault();
        const next = Math.max(0, Math.min(rows.length - 1, to));
        select(next);
        list.children[next]?.scrollIntoView({ block: "nearest" });
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

    let answer = null;
    up.addEventListener("click", () => parent && open(parent));
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
    dialog.addEventListener("close", () => { dialog.remove(); resolve(answer); });

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
    no.className = "text-button";
    no.textContent = "Cancel";
    const yes = document.createElement("button");
    yes.type = "button";
    yes.className = "chip active";
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
