// A modal confirmation, for anything that changes a file the reader already
// has. It says exactly what will change and waits for a clear yes.

import { api } from "./api.js";
import * as format from "./format.js";

// waypointDialog asks for the destination and details before writing a point.
export function waypointDialog(entries, coordinates) {
  return new Promise((resolve) => {
    const dialog = document.createElement("dialog");
    dialog.className = "dialog";
    const heading = document.createElement("h3");
    heading.textContent = "Add waypoint";
    const place = document.createElement("p");
    place.textContent = `${coordinates[1].toFixed(6)}, ${coordinates[0].toFixed(6)} (WGS-84)`;
    const form = document.createElement("form");
    form.method = "dialog";
    const target = document.createElement("select");
    target.className = "cut-path prompt-input";
    for (const entry of entries) {
      const option = document.createElement("option");
      option.value = entry.path;
      option.textContent = `${entry.name} — ${entry.path}`;
      target.append(option);
    }
    const name = document.createElement("input");
    name.className = "cut-path prompt-input";
    name.placeholder = "Waypoint name";
    name.required = true;
    const description = document.createElement("input");
    description.className = "cut-path prompt-input";
    description.placeholder = "Description (optional)";
    const buttons = document.createElement("div");
    buttons.className = "dialog-buttons";
    const cancel = document.createElement("button");
    cancel.type = "button";
    cancel.className = "text-button";
    cancel.textContent = "Cancel";
    const save = document.createElement("button");
    save.type = "submit";
    save.className = "chip active";
    save.textContent = "Add";
    buttons.append(cancel, save);
    form.append(target, name, description, buttons);
    dialog.append(heading, place, form);
    let answer = null;
    cancel.addEventListener("click", () => dialog.close());
    form.addEventListener("submit", () => {
      if (name.value.trim()) answer = { path: target.value, name: name.value.trim(), description: description.value.trim() };
    });
    dialog.addEventListener("close", () => { dialog.remove(); resolve(answer); });
    document.body.append(dialog);
    dialog.showModal();
    name.focus();
  });
}

// promptDialog asks for one line of text, such as a file path, and resolves
// to it, or to null when cancelled.
export function promptDialog({ title, message, value = "", confirm = "OK", cancel = "Cancel" }) {
  return new Promise((resolve) => {
    const dialog = document.createElement("dialog");
    dialog.className = "dialog";
    const heading = document.createElement("h3");
    heading.textContent = title;
    const text = document.createElement("p");
    text.textContent = message;
    const form = document.createElement("form");
    form.method = "dialog";
    const input = document.createElement("input");
    input.className = "cut-path prompt-input";
    input.value = value;
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
    form.append(input, buttons);
    dialog.append(heading, text, form);
    let answer = null;
    no.addEventListener("click", () => dialog.close());
    form.addEventListener("submit", () => {
      if (input.value.trim()) answer = input.value.trim();
    });
    dialog.addEventListener("close", () => {
      dialog.remove();
      resolve(answer);
    });
    document.body.append(dialog);
    dialog.showModal();
    input.focus();
    input.select();
  });
}

// confirmDialog resolves true only when the confirm button is pressed.
export function confirmDialog({ title, message, detail, confirm = "Continue", cancel = "Cancel", danger = false }) {
  return new Promise((resolve) => {
    const dialog = document.createElement("dialog");
    dialog.className = "dialog";
    const heading = document.createElement("h3");
    heading.textContent = title;
    const text = document.createElement("p");
    text.textContent = message;
    dialog.append(heading, text);
    if (detail) {
      const code = document.createElement("code");
      code.className = "dialog-detail";
      code.textContent = detail;
      dialog.append(code);
    }
    const buttons = document.createElement("div");
    buttons.className = "dialog-buttons";
    const no = document.createElement("button");
    no.className = "text-button";
    no.textContent = cancel;
    const yes = document.createElement("button");
    yes.className = "chip active" + (danger ? " danger" : "");
    yes.textContent = confirm;
    buttons.append(no, yes);
    dialog.append(buttons);
    let answer = false;
    no.addEventListener("click", () => dialog.close());
    yes.addEventListener("click", () => {
      answer = true;
      dialog.close();
    });
    dialog.addEventListener("close", () => {
      dialog.remove();
      resolve(answer);
    });
    document.body.append(dialog);
    dialog.showModal();
    no.focus(); // Enter does not confirm by accident
  });
}

// The save dialog's columns, and how the list is sorted. The choice is kept
// between dialogs, as a file manager keeps it.
const SAVE_COLUMNS = [
  { key: "name", label: "Name" },
  { key: "modified", label: "Date Modified" },
  { key: "size", label: "Size" },
];
let saveSort = { key: "name", ascending: true };

// saveDialog chooses where to write a new file: a folder is browsed rather
// than typed, and only the file name is entered. It resolves to the full
// path, or to null when cancelled.
export function saveDialog({ title, message, folder, fallback = "", places = [], name, confirm = "Save", cancel = "Cancel", extension = ".gpx" }) {
  return new Promise((resolve) => {
    const separator = folder.includes("\\") && !folder.includes("/") ? "\\" : "/";
    const dialog = document.createElement("dialog");
    dialog.className = "dialog save-dialog";
    const heading = document.createElement("h3");
    heading.textContent = title;
    const text = document.createElement("p");
    text.textContent = message;

    const bar = document.createElement("div");
    bar.className = "save-bar";
    const up = document.createElement("button");
    up.type = "button";
    up.className = "text-button";
    up.textContent = "↑";
    up.title = "The folder above";
    const where = document.createElement("code");
    where.className = "save-where";
    bar.append(up, where);

    // The places down the side — home, the folders under it, mounted disks —
    // and the folder being browsed, side by side.
    const body = document.createElement("div");
    body.className = "save-body";
    const sidebar = document.createElement("ul");
    sidebar.className = "save-places";
    const pane = document.createElement("div");
    pane.className = "save-listing";
    // The column headers: clicking one sorts by it, clicking it again turns
    // the order round, as the marker ^ or v says.
    const header = document.createElement("div");
    header.className = "save-header";
    const headings = {};
    for (const column of SAVE_COLUMNS) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = `save-heading save-cell-${column.key}`;
      const label = document.createElement("span");
      label.textContent = column.label;
      const marker = document.createElement("span");
      marker.className = "save-marker";
      button.append(label, marker);
      button.addEventListener("click", () => {
        saveSort = saveSort.key === column.key
          ? { key: column.key, ascending: !saveSort.ascending }
          : { key: column.key, ascending: column.key === "name" };
        render();
      });
      headings[column.key] = marker;
      header.append(button);
    }
    const list = document.createElement("ul");
    list.className = "save-list";
    pane.append(header, list);
    body.append(sidebar, pane);
    const problem = document.createElement("p");
    problem.className = "save-problem";
    problem.hidden = true;

    const form = document.createElement("form");
    form.method = "dialog";
    const input = document.createElement("input");
    input.className = "cut-path prompt-input";
    input.value = name;
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
    form.append(input, buttons);
    dialog.append(heading, text, bar, body, problem, form);

    let here = folder;
    let parent = "";
    let entries = { dirs: [], files: [] };

    // A row per folder and per GPX file: its name, when it was last written,
    // and a file's size. Clicking a folder opens it; clicking a file takes
    // its name, so a new file is named after one already there.
    const row = (entry, kind) => {
      const item = document.createElement("li");
      const button = document.createElement("button");
      button.type = "button";
      button.className = kind === "folder" ? "save-folder" : "save-file";
      button.title = kind === "folder" ? entry.path : "Use this name";
      const name = document.createElement("span");
      name.className = "save-cell-name";
      name.textContent = entry.name;
      const modified = document.createElement("span");
      modified.className = "save-cell-modified";
      modified.textContent = entry.modified ? when(entry.modified) : "";
      const size = document.createElement("span");
      size.className = "save-cell-size";
      size.textContent = kind === "folder" ? "" : format.fileSize(entry.size || 0);
      button.append(name, modified, size);
      button.addEventListener("click", () => {
        if (kind === "folder") return open(entry.path);
        input.value = entry.name;
        input.focus();
      });
      item.append(button);
      return item;
    };

    // render draws the folder that is open, sorted as the headers say.
    // Folders stay above files, as a file manager keeps them.
    const render = () => {
      list.textContent = "";
      for (const column of SAVE_COLUMNS) {
        headings[column.key].textContent = saveSort.key === column.key ? (saveSort.ascending ? "^" : "v") : "";
      }
      const sorted = (rows) => [...rows].sort(compare(saveSort));
      for (const entry of sorted(entries.dirs)) list.append(row(entry, "folder"));
      for (const entry of sorted(entries.files)) list.append(row(entry, "file"));
      if (!entries.dirs.length && !entries.files.length) {
        const item = document.createElement("li");
        item.className = "save-empty";
        item.textContent = "Empty";
        list.append(item);
      }
    };
    // A folder that cannot be read falls back once — to the folder the page
    // opened at, and then to the home directory the server answers with.
    const open = async (path, ...rest) => {
      list.textContent = "";
      problem.hidden = true;
      let listing;
      try {
        listing = await api.dir(path);
      } catch (error) {
        if (rest.length) return open(rest[0], ...rest.slice(1));
        problem.textContent = error.message;
        problem.hidden = false;
        return;
      }
      here = listing.path;
      parent = listing.parent || "";
      where.textContent = here;
      up.disabled = !parent;
      for (const item of sidebar.children) {
        item.firstChild.classList.toggle("here", item.firstChild.dataset.path === here);
      }
      entries = { dirs: listing.dirs, files: listing.files };
      render();
    };

    for (const spot of places) {
      const item = document.createElement("li");
      const button = document.createElement("button");
      button.type = "button";
      button.className = "save-place";
      button.textContent = spot.name;
      button.title = spot.path;
      button.dataset.path = spot.path;
      button.addEventListener("click", () => open(spot.path));
      item.append(button);
      sidebar.append(item);
    }
    sidebar.hidden = places.length === 0;

    up.addEventListener("click", () => parent && open(parent));
    let answer = null;
    no.addEventListener("click", () => dialog.close());
    form.addEventListener("submit", () => {
      let chosen = input.value.trim();
      if (!chosen) return;
      if (extension && !chosen.toLowerCase().endsWith(extension)) chosen += extension;
      answer = `${here.replace(/[\\/]$/, "")}${separator}${chosen}`;
    });
    dialog.addEventListener("close", () => {
      dialog.remove();
      resolve(answer);
    });
    document.body.append(dialog);
    dialog.showModal();
    open(folder || fallback, fallback, "");
    input.focus();
    const dot = input.value.toLowerCase().lastIndexOf(extension);
    input.setSelectionRange(0, dot > 0 ? dot : input.value.length);
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
