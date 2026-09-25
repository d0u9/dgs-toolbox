// Import: open a folder from anywhere, see its PDFs as the tree of folders
// they are in, and take them in one at a time. The folder is only read.
import { $, api, el, size, loadState, post, templateOf, label, inputFor, fieldsOf, tagUses, frame, say, showText, showPreview, showSource, clearPreview } from "/common.js";
import { openFile } from "/ui/filedialog.js";
import { fileTree } from "/ui/filetree.js";

const FOLDER_KEY = "dgs-doc-import-folder";
let state = { templates: [], items: [] };
let dir = "";
let files = [];
let selected = "";
// What the text of the picked PDF suggests, by type: { type: { key: value } }.
let suggestions = {};
let selectedTemplate = "";
// typeChosen is set once the reader picks a Template for this PDF; a type
// proposed from the text never overrides that.
let typeChosen = false;
const closed = new Set();
const tags = window.tagField($("tags"), { known: () => tagUses(state.items), placeholder: "Add tags" });
let metadataItem = "";

function remembered() {
  try { return localStorage.getItem(FOLDER_KEY) || ""; } catch { return ""; }
}
function remember(folder) {
  try { localStorage.setItem(FOLDER_KEY, folder); } catch { /* not kept */ }
}

function render() {
  frame(state);
  $("dir").textContent = dir;
  const kept = files.filter((f) => f.item).length;
  $("counts").textContent = dir ? `${files.length} PDFs · ${kept} in tree` : "";
  $("tree").replaceChildren(fileTree(files, {
    closed, selected, onPick: (f) => pick(f.path),
    mark: (f) => f.item ? "Imported: already in the tree" : "",
    fileExtra: (f) => el("span", {}, size(f.size) + " · " + new Date(f.modified).toLocaleDateString()),
    folderExtra: (path, under) => {
      const kept = under.filter((f) => f.item).length;
      return el("span", { className: "numeric" }, kept ? `${kept}/${under.length} in tree` : String(under.length));
    },
  }));
  if (dir && files.length === 0) say($("list-message"), "No PDFs in this folder or under it.");
  const file = files.find((f) => f.path === selected);
  $("side").hidden = !file;
  $("import").hidden = !file || !!file.item || !state.tree;
  $("kept").hidden = !file || !file.item;
  if (file && file.item) {
    const item = state.items.find((i) => i.id === file.item);
    $("kept-link").textContent = item ? label(state, item) : file.item;
    $("kept-link").href = api("/browse/") + "#" + file.item;
  }
}

function pick(path) {
  if (path !== selected) {
    $("notes").value = "";
    tags.set([]);
    metadataItem = "";
    showPreview({ dir, path }, api("/api/source/file?dir=" + encodeURIComponent(dir) + "&path=" + encodeURIComponent(path)));
    say($("import-message"), "");
    suggestions = {};
    typeChosen = false;
    $("type-hint").hidden = true;
    showText({ dir, path }, (answer) => {
      if (selected !== path) return;
      if (answer.types && answer.types.length) proposeType(answer);
      // An earlier page's value stands: a document says what it is first.
      for (const [type, found] of Object.entries(answer.suggestions || {})) {
        suggestions[type] = { ...found, ...(suggestions[type] || {}) };
      }
      suggest();
    }).then(() => { if (selected === path) readAhead(path); });
  }
  selected = path;
  render();
  drawFields();
}

// readAhead has the next PDFs not yet in the tree read while this one is
// looked at, so their text is there when they are picked.
const AHEAD = 3;
function readAhead(path) {
  const order = files.map((f) => f.path);
  const next = order.slice(order.indexOf(path) + 1)
    .filter((p) => !files.find((f) => f.path === p).item).slice(0, AHEAD);
  if (next.length) post("/api/ahead", { dir, paths: next }).catch(() => {});
}

function drawFields() {
  if (!templateOf(state, selectedTemplate)) selectedTemplate = state.templates[0]?.type || "";
  $("template").replaceChildren(...state.templates.map((choice) => {
    const button = el("button", { type: "button", title: (choice.description || choice.type) + " (" + choice.kind + ")", className: "template-choice" + (choice.type === selectedTemplate ? " selected" : "") },
      el("strong", {}, choice.type), el("small", {}, choice.description || choice.kind));
    button.setAttribute("aria-pressed", String(choice.type === selectedTemplate));
    button.onclick = () => {
      typeChosen = true;
      if (selectedTemplate === choice.type) return;
      selectedTemplate = choice.type;
      drawFields();
      $("template").querySelector(".selected")?.focus();
    };
    return button;
  }));
  const t = templateOf(state, selectedTemplate);
  $("import-button").disabled = !t;
  if (!t) {
    $("into-field").hidden = true;
    $("tags-field").hidden = true;
    $("notes-field").hidden = true;
    $("fields").replaceChildren(el("p", { className: "message" }, "No Templates in the tree's templates folder."));
    return;
  }
  const into = $("into");
  const was = into.value;
  const documents = t.kind === "document" ? state.items.filter((i) => i.type === t.type) : [];
  into.replaceChildren(el("option", { value: "" }, "A new item"),
    ...documents.map((i) => el("option", { value: i.id }, "New revision of " + label(state, i))));
  if (documents.some((i) => i.id === was)) into.value = was;
  $("into-field").hidden = documents.length === 0;
  $("into-choices").replaceChildren(...[{ id: "" }, ...documents].map((item) => {
    const value = item.id;
    const button = el("button", {
      type: "button", className: "into-choice", title: value ? label(state, item) : "Create a separate Item",
    }, el("strong", {}, value ? (item.fields?.name || label(state, item)) : "New item"),
    el("small", {}, value ? [item.fields?.owner, item.fields?.country].filter(Boolean).join(" · ") : "Create a separate document"));
    button.value = value;
    button.setAttribute("aria-pressed", String(into.value === value));
    button.onclick = () => {
      if (into.value === value) return;
      into.value = value;
      drawFields();
      [...$("into-choices").children].find((choice) => choice.value === value)?.focus();
    };
    return button;
  }));
  const adding = into.value !== "";
  if (metadataItem !== into.value) {
    const item = state.items.find((i) => i.id === into.value);
    $("notes").value = item?.notes || "";
    tags.set(item?.tags || []);
    metadataItem = into.value;
  }
  $("tags-field").hidden = false;
  $("notes-field").hidden = false;
  $("import-button").textContent = adding ? "Add revision" : "Import";
  // A new revision is asked only what changes with it: the fields the
  // Template marks per_revision. The rest belong to the Item it joins.
  const asked = adding ? t.fields.filter((f) => f.per_revision) : t.fields;
  $("fields").replaceChildren(...asked.map((f) => inputFor(f, "", adding ? "" : (t.defaults || {})[f.key] || "", state)));
  if (adding && !asked.length) {
    $("fields").append(el("p", { className: "message" }, "Nothing is asked: " + t.type +
      " marks no field per_revision. To keep a renewed card's own number and expiry, add per_revision: true to them on the Templates page."));
  }
  for (const input of $("fields").querySelectorAll(".field-input")) {
    input.addEventListener("input", () => { input.classList.remove("suggested"); showSource(null); });
    const source = () => input.classList.contains("suggested") && showSource((suggestions[selectedTemplate] || {})[input.name]);
    input.addEventListener("focus", source);
    input.addEventListener("mouseenter", source);
    input.addEventListener("blur", () => showSource(null));
    input.addEventListener("mouseleave", () => document.activeElement !== input && showSource(null));
  }
  suggest();
}

// suggest fills the empty fields the text has a value for, marked so they
// read as proposals. A field the reader typed in is never replaced.
function suggest() {
  const found = suggestions[selectedTemplate] || {};
  for (const input of $("fields").querySelectorAll(".field-input")) {
    const value = found[input.name] && found[input.name].value;
    if (value && (input.value === "" || input.classList.contains("suggested"))) {
      input.value = value;
      input.dispatchEvent(new Event("change"));
      input.classList.add("suggested");
      input.title = "Suggested from the text. Hover to see where on the page it was read.";
    }
  }
}

async function open(folder) {
  say($("list-message"), "Reading…");
  const response = await fetch(api("/api/source?dir=" + encodeURIComponent(folder)));
  const answer = await response.json();
  if (!response.ok) {
    say($("list-message"), answer.error || response.statusText, true);
    return;
  }
  say($("list-message"), "");
  if (folder !== dir) {
    selected = "";
    closed.clear();
    clearPreview();
  }
  dir = answer.dir;
  files = answer.files;
  remember(dir);
  render();
}

$("open").onclick = async () => {
  const chosen = await openFile({
    title: "Open folder",
    message: "Choose a folder. Its PDFs, and those in folders under it, are listed; nothing in it changes.",
    folders: true,
    folder: dir || remembered() || state.root,
    fallbacks: [state.root, ""],
    filters: [{ label: "PDF", extensions: [".pdf"] }],
  });
  if (chosen) await open(Array.isArray(chosen) ? chosen[0] : chosen);
};

// proposeType selects the Template the first page most looks like, going by
// the Items already filed, and says why. The reader can always pick another.
function proposeType(answer) {
  const top = answer.types[0];
  const hint = $("type-hint");
  hint.hidden = false;
  if (!answer.type) {
    hint.textContent = "Looks most like " + top.type + " (" + Math.round(top.score * 100) + "%), too little to choose it.";
    return;
  }
  hint.textContent = "Looks like " + answer.type + " (" + Math.round(top.score * 100) + "% like one already filed).";
  if (!typeChosen && selectedTemplate !== answer.type && templateOf(state, answer.type)) {
    selectedTemplate = answer.type;
    drawFields();
    $("template").querySelector(".selected")?.scrollIntoView({ block: "nearest" });
  }
}
$("import").onsubmit = async (event) => {
  event.preventDefault();
  tags.commit();
  $("import-button").disabled = true;
  say($("import-message"), "Copying and reading back…");
  try {
    if ($("into").value && !$("into-field").hidden) {
      await post("/api/revisions", { dir, path: selected, item: $("into").value, fields: fieldsOf($("fields")), notes: $("notes").value, tags: tags.get() });
    } else {
      await post("/api/import", { dir, path: selected, type: selectedTemplate, fields: fieldsOf($("fields")), notes: $("notes-field").hidden ? "" : $("notes").value, tags: tags.get() });
    }
    const done = selected;
    state = await loadState();
    await open(dir);
    const order = files.map((f) => f.path);
    const next = order.slice(order.indexOf(done) + 1).concat(order).find((p) => !files.find((f) => f.path === p).item);
    if (next) pick(next);
    else say($("list-message"), "Everything in this folder is in the tree.");
  } catch (err) {
    say($("import-message"), err.message, true);
  } finally {
    $("import-button").disabled = false;
  }
};

loadState().then(async (s) => {
  state = s;
  render();
  const last = remembered();
  if (last) await open(last);
}).catch((err) => {
  state.error = err.message;
  render();
});
