// Import: open a folder from anywhere, see its PDFs as the tree of folders
// they are in, and take them in one at a time. The folder is only read.
import { $, keep, scrollBack, api, el, size, loadState, post, templateOf, label, fieldInputs, fieldsOf, tagUses, frame, say, showText, showPreview, showSource, clearPreview } from "/common.js";
import { openFile } from "/ui/filedialog.js";
import { splitter } from "/ui/splitter.js";
import { fileTree } from "/ui/filetree.js";
import { iconSvg } from "/ui/icons.js";

// The import panel is as wide as the reader drags it, and never so narrow
// that its fields do not fit.
splitter({ handle: $("side-splitter"), target: $("side"), axis: "x", invert: true, min: 320,
  max: () => Math.max(320, window.innerWidth - 480), key: "dgs-doc-size-import-side" });

const FOLDER_KEY = "dgs-doc-import-folder";
const HIDE_KEY = "dgs-doc-import-hide-kept";
let state = { templates: [], items: [] };
let dir = "";
let outputFolder = "";
let files = [];
let selected = "";
// pages are more images after the selected one, each the next page of the
// PDF they become together.
let pages = [];
let noPDF = false;
// What the text of the picked PDF suggests, by type: { type: { key: value } }.
let suggestions = {};
let selectedTemplate = "";
// typeChosen is set once the reader picks a Template for this PDF; a type
// proposed from the text never overrides that.
let typeChosen = false;
// The type the first page most looks like, marked in the list.
let suggestedType = "";
// Past this many Templates the list gets a filter.
const FILTER_FROM = 6;
const closed = new Set();
const tags = window.tagField($("tags"), { known: () => tagUses(state.items), placeholder: "Add tags" });
const revisionTags = window.tagField($("revision-tags"), { known: () => tagUses(state.items), placeholder: "Add tags" });
let metadataItem = "";

function remembered() {
  try { return localStorage.getItem(FOLDER_KEY) || ""; } catch { return ""; }
}
function remember(folder) {
  try { localStorage.setItem(FOLDER_KEY, folder); } catch { /* not kept */ }
}

try { $("hide-kept").checked = localStorage.getItem(HIDE_KEY) === "1"; } catch { /* not kept */ }
$("hide-kept").onchange = () => {
  try { localStorage.setItem(HIDE_KEY, $("hide-kept").checked ? "1" : ""); } catch { /* not kept */ }
  render();
};

function render() {
  frame(state);
  document.querySelector(".import-work").classList.toggle("manual-entry", noPDF);
  $("cancel-without-pdf").hidden = !noPDF;
  $("dir").textContent = dir;
  $("dir").title = dir;
  const kept = files.filter((f) => f.item).length;
  $("counts").textContent = dir ? `${files.length} files · ${kept} in tree` : "";
  $("counts").title = $("counts").textContent;
  // Hidden imported PDFs leave the tree, except the one being looked at.
  const shown = $("hide-kept").checked ? files.filter((f) => !f.item || f.path === selected) : files;
  $("tree").replaceChildren(fileTree(shown, {
    closed, selected, onPick: (f) => pick(f.path),
    mark: (f) => f.item ? "Imported: already in the tree" : "",
    fileExtra: (f) => el("span", {}, size(f.size) + " · " + new Date(f.modified).toLocaleDateString(),
      el("button", { type: "button", className: "ft-reveal", title: "Show in Finder", "aria-label": "Show in Finder", innerHTML: iconSvg("square-arrow-out-up-right"),
        onclick: (event) => { event.stopPropagation(); post("/api/reveal", { dir, path: f.path }).catch((err) => alert(err.message)); } })),
    folderExtra: (path, under) => {
      const kept = under.filter((f) => f.item).length;
      return el("span", { className: "numeric" }, kept ? `${kept}/${under.length} in tree` : String(under.length));
    },
  }));
  if (dir && files.length === 0) say($("list-message"), state.capabilities.all_files?"No files in this folder or under it.":"No PDFs or supported images in this folder or under it.");
  const file = files.find((f) => f.path === selected);
  $("side").hidden = !file && !noPDF;
  $("side-splitter").hidden = $("side").hidden;
  $("import").hidden = (!noPDF && (!file || (!!file.item && !state.capabilities.reuse_files))) || !state.tree;
  $("copy-note").hidden = noPDF;
  $("without-pdf").hidden = !!state.capabilities.reuse_files;
  $("destination-field").hidden = !state.capabilities.import_folder || noPDF;
  if (state.capabilities.import_folder) $("copy-note").textContent = "Files already inside this tree stay in place. External files are copied to the chosen folder and verified before publication. Images become a PDF in an Item folder. Existing content is reused only after confirmation.";
  $("keep-original-field").hidden = noPDF || !file || !file.image || (!!file.item && !state.capabilities.reuse_files);
  if (!$("keep-original-field").hidden) drawPages();
  $("import-title").textContent = noPDF ? "New without PDF" : file?.item ? (state.capabilities.reuse_files ? "New Item from stored content" : "Already in the tree") : "Import";
  $("kept").hidden = !file || !file.item;
  if (file && file.item) {
    const item = state.items.find((i) => i.id === file.item);
    $("kept-link").textContent = item ? label(state, item) : file.item;
    $("kept-link").href = api("/browse/") + "#" + file.item;
  }
}

// source names the file picked, and the images of its other pages, as a
// preview asks for them.
function source(path) {
  return pages.length ? { dir, path, pages: JSON.stringify(pages) } : { dir, path };
}

function pick(path) {
  noPDF = false;
  if (path !== selected) {
    $("notes").value = "";
    tags.set([]);
    $("shared").replaceChildren();
    metadataItem = "";
    pages = [];
    say($("import-message"), "");
    typeChosen = false;
    $("type-hint").hidden = true;
    load(path);
  }
  selected = path;
  keep({ selected });
  render();
  drawFields();
}

// load shows the file picked, with its other pages, and reads its text.
function load(path) {
 if(! /\.pdf$/i.test(path)&&!files.find(f=>f.path===path)?.image){
   clearPreview();suggestions={};say($("list-message"),"This file has no PDF pages or searchable page text.");return;
 }

  const query = source(path);
  const shown = JSON.stringify(query);
  showPreview(query, api("/api/source/file?" + new URLSearchParams(query)));
  suggestions = {};
  showText(query, (answer) => {
    if (selected !== path || JSON.stringify(source(path)) !== shown) return;
    if (answer.types && answer.types.length) proposeType(answer);
    // An earlier page's value stands: a document says what it is first.
    for (const [type, found] of Object.entries(answer.suggestions || {})) {
      suggestions[type] = { ...found, ...(suggestions[type] || {}) };
    }
    suggest();
  }).then(() => { if (selected === path) readAhead(path); });
}

// drawPages lists the images that become the PDF, the picked one first;
// a later one moves up or down or leaves. Any other image of the folder
// not yet in the tree can be added after them.
function drawPages() {
  const name = (p) => p.split("/").pop();
  const changed = () => { load(selected); render(); };
  const move = (i, by) => { const [p] = pages.splice(i, 1); pages.splice(i + by, 0, p); changed(); };
  $("image-pages").replaceChildren(el("li", {}, el("span", { textContent: name(selected), title: selected })),
    ...pages.map((p, i) => el("li", {}, el("span", { textContent: name(p), title: p }),
      el("button", { type: "button", className: "text-button", innerHTML: iconSvg("arrow-up"), title: "Earlier", disabled: i === 0, onclick: () => move(i, -1) }),
      el("button", { type: "button", className: "text-button", innerHTML: iconSvg("arrow-down"), title: "Later", disabled: i === pages.length - 1, onclick: () => move(i, 1) }),
      el("button", { type: "button", className: "text-button", innerHTML: iconSvg("x"), title: "Not a page of this PDF", onclick: () => { pages.splice(i, 1); changed(); } }))));
  const folder = (p) => p.slice(0, p.lastIndexOf("/") + 1);
  const others = files.filter((f) => f.image && (state.capabilities.reuse_files || !f.item) && f.path !== selected && !pages.includes(f.path))
    .sort((a, b) => (folder(b.path) === folder(selected)) - (folder(a.path) === folder(selected)) || a.path.localeCompare(b.path));
  $("add-page").hidden = others.length === 0;
  $("add-page").replaceChildren(el("option", { value: "", textContent: "+ Add a page…" }),
    ...others.map((f) => el("option", { value: f.path, textContent: f.path })));
  $("add-page").onchange = () => { if ($("add-page").value) { pages.push($("add-page").value); changed(); } };
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

$("template-filter").addEventListener("input", () => drawFields());
// Coming back to the filter selects what is in it, so the next type is
// typed over the last rather than after it. The click that focuses it
// would otherwise put the caret back and drop the selection.
let selectOnUp = false;
$("template-filter").addEventListener("focus", () => { $("template-filter").select(); selectOnUp = true; });
$("template-filter").addEventListener("mouseup", (event) => {
  if (selectOnUp) event.preventDefault();
  selectOnUp = false;
});
// Up and down move the picked Template, whether the keys are in the filter
// or on the list: in the filter the caret stays there, so typing goes on,
// and the blue row is the one picked. On the list, up from the top goes
// back to the filter. The filter is kept while the keys move; Enter, Space
// or a click on the picked one empties it.
const templateChoices = () => [...$("template").querySelectorAll(".template-choice")];
function choose(type, focus = true) {
  typeChosen = true;
  if (selectedTemplate !== type) {
    selectedTemplate = type;
    drawFields();
  }
  const picked = $("template").querySelector(".selected");
  if (focus) picked?.focus();
  else picked?.scrollIntoView({ block: "nearest" });
}
const typeOf = (button) => button.querySelector("strong").textContent;
$("template-filter").addEventListener("keydown", (event) => {
  if (event.key !== "ArrowDown" && event.key !== "ArrowUp" && event.key !== "Enter") return;
  const choices = templateChoices();
  if (!choices.length) return;
  event.preventDefault();
  const at = choices.findIndex((c) => c.classList.contains("selected"));
  if (event.key === "Enter") {
    // One Template besides the picked one matches: Enter takes it.
    const others = choices.filter((c) => !c.classList.contains("selected"));
    (others.length === 1 ? others[0] : choices[Math.max(at, 0)]).click();
    return;
  }
  const next = at < 0 ? 0 : Math.max(0, Math.min(at + (event.key === "ArrowDown" ? 1 : -1), choices.length - 1));
  choose(typeOf(choices[next]), false);
});
$("template").addEventListener("keydown", (event) => {
  if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
  const choices = templateChoices();
  const at = choices.indexOf(document.activeElement);
  if (at < 0) return;
  event.preventDefault();
  const next = at + (event.key === "ArrowDown" ? 1 : -1);
  if (next < 0) { if (!$("template-filter").hidden) $("template-filter").focus(); }
  else choose(typeOf(choices[Math.min(next, choices.length - 1)]));
});

function drawFields() {
  if (!templateOf(state, selectedTemplate)) selectedTemplate = state.templates[0]?.type || "";
  // One line per Template: its type, what it is for, and a mark on the one
  // the page suggests. A filter narrows a long list; the chosen one stays.
  $("template-filter").hidden = state.templates.length <= FILTER_FROM;
  const words = $("template-filter").hidden ? [] : $("template-filter").value.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const shown = state.templates.filter((t) => t.type === selectedTemplate ||
    words.every((w) => (t.type + " " + (t.description || "")).toLowerCase().includes(w)));
  $("template").replaceChildren(...shown.map((choice) => {
    const button = el("button", { type: "button", title: (choice.description || choice.type) + " (" + choice.kind + ")", className: "template-choice" + (choice.type === selectedTemplate ? " selected" : "") },
      el("strong", {}, choice.type), el("small", {}, choice.description || choice.kind),
      choice.type === suggestedType ? el("span", { className: "template-suggested" }, "suggested") : null);
    button.setAttribute("aria-pressed", String(choice.type === selectedTemplate));
    // A pick empties the filter, so the whole list is back for the next.
    button.onclick = () => {
      const filtered = $("template-filter").value !== "";
      $("template-filter").value = "";
      if (filtered) selectedTemplate = "";
      choose(choice.type);
    };
    return button;
  }));
  const t = templateOf(state, selectedTemplate);
  $("import-button").disabled = !t;
  if (!t) {
    $("into-field").hidden = true;
    $("tags-field").hidden = true;
    $("revision-tags-field").hidden = true;
    $("notes-field").hidden = true;
    $("fields").replaceChildren(el("p", { className: "message" }, "No Templates in the tree's templates folder."));
    return;
  }
  const into = $("into");
  const was = into.value;
  const documents = t.kind === "document" ? state.items.filter((i) => i.type === t.type) : [];
  into.replaceChildren(el("option", { value: "" }, "A new item"),
    ...documents.map((i) => el("option", { value: i.id }, "Add to " + label(state, i))));
  if (documents.some((i) => i.id === was)) into.value = was;
  $("into-field").hidden = documents.length === 0;
  $("into-choices").replaceChildren(...[{ id: "" }, ...documents].map((item) => {
    const value = item.id;
    const identity = t.fields.filter((f) => f.distinguishing && item.fields?.[f.key])
      .map((f) => ({ key: f.key, value: item.fields[f.key] }));
    // A name says what the document is even where the Template does not
    // count it among what distinguishes one: a lease and its condition
    // report share owner, issuer and category.
    const named = !identity.some((f) => f.key === "card_name" || f.key === "name") && item.fields?.name
      ? { key: "name", value: item.fields.name } : null;
    const primary = named || identity.find((f) => f.key === "card_name" || f.key === "name")
      || identity[identity.length - 1];
    const detail = identity.filter((f) => f !== primary).map((f) => f.value).join(" · ");
    const button = el("button", {
      type: "button", className: "template-choice into-choice" + (into.value === value ? " selected" : ""), title: value ? label(state, item) : "Create a separate Item",
    }, el("strong", {}, value ? (primary?.value || item.id) : "New item"),
    el("small", {}, value ? detail : "Separate document"));
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
  // A new revision is the owner's word, asked each time; a record has none.
  if ($("new-revision-field").hidden !== !adding) $("new-revision").checked = false;
  $("new-revision-field").hidden = !adding;
  if (metadataItem !== into.value) {
    const item = state.items.find((i) => i.id === into.value);
    $("notes").value = item?.notes || "";
    tags.set(item?.tags || []);
    revisionTags.set([]);
    metadataItem = into.value;
  }
  // Every revision can have tags of its own, beside the Item's, which are
  // shown too. A new Item's first revision also gets the Item's tags.
  $("revision-tags-field").hidden = noPDF;
  $("tags-field").hidden = false;
  $("notes-field").hidden = false;
  $("import-button").textContent = adding ? "Add to item" : (noPDF ? "Create item" : "Import");
  // A new revision is asked only what changes with it: the fields the
  // Template marks per_revision. The rest belong to the Item it joins.
  const asked = adding ? t.fields.filter((f) => f.per_revision) : t.fields;
  $("fields").replaceChildren(...fieldInputs(asked, null, adding ? null : t.defaults, state, undefined, t.type));
  if (adding && !asked.length) {
    $("fields").append(el("p", { className: "message" }, "Nothing is asked: " + t.type +
      " marks no field per_revision. To keep a renewed card's own number and expiry, add per_revision: true to them on the Templates page."));
  }
  for (const input of $("fields").querySelectorAll(".field-input")) {
    if (["owner", "country"].includes(input.name)) { input.addEventListener("input", drawReplaces); input.addEventListener("change", drawReplaces); }
    // The owner chosen is left out of the people it is shared with.
    if (input.name === "owner") for (const on of ["input", "change"]) input.addEventListener(on, () => drawShared(t, adding));
    input.addEventListener("input", () => { input.classList.remove("suggested"); showSource(null); });
    const source = (reveal) => input.classList.contains("suggested") && showSource((suggestions[selectedTemplate] || {})[input.name], reveal);
    input.addEventListener("focus", () => source(true));
    input.addEventListener("mouseenter", () => source(false));
    input.addEventListener("blur", () => showSource(null));
    input.addEventListener("mouseleave", () => document.activeElement !== input && showSource(null));
  }
  suggest();
  drawReplaces();
  drawShared(t, adding);
}

// suggest fills the empty fields the text has a value for, marked so they
// read as proposals. A field the reader typed in is never replaced.
function suggest() {
  const found = suggestions[selectedTemplate] || {};
  for (const input of $("fields").querySelectorAll(".field-input")) {
    const value = found[input.name] && found[input.name].value;
    if (value && (input.value === "" || input.classList.contains("suggested"))) {
      input.value = value;
      input.dispatchEvent(new Event("change", { bubbles: true }));
      input.classList.add("suggested");
      input.title = "Suggested from the text. Hover to highlight the source; focus this field to locate it.";
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
  if (suggestedType !== answer.type) {
    suggestedType = answer.type;
    drawFields();
  }
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
  say($("import-message"), noPDF ? "Saving…" : "Copying and reading back…");
  try {
    if ($("into").value && !$("into-field").hidden) {
      revisionTags.commit();
      await post("/api/revisions", { folder: outputFolder, keep_original: $("keep-original").checked, pages: noPDF ? [] : pages, no_pdf: noPDF, dir, path: selected, item: $("into").value, fields: fieldsOf($("fields")), notes: $("notes").value, tags: tags.get(),
        revision_tags: revisionTags.get(), new_revision: $("new-revision").checked });
      $("new-revision").checked = false;
    } else {
      const body = { replaces: $("replaces-field").hidden ? "" : $("replaces").value, keep_original: $("keep-original").checked, pages: noPDF ? [] : pages, no_pdf: noPDF, dir, path: selected, type: selectedTemplate, fields: fieldsOf($("fields")), notes: $("notes-field").hidden ? "" : $("notes").value, tags: tags.get(),
        revision_tags: noPDF ? [] : (revisionTags.commit(), revisionTags.get()), shared_with: sharedWith() };
      body.folder = outputFolder;
      let saved;
      if (state.capabilities.import_folder) {
        const response = await fetch(api("/api/import"), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
        saved = await response.json();
        if (!response.ok) {
          if (!saved.reuse) throw new Error(saved.error || response.statusText);
          if (!confirm("Same content is already stored at " + saved.reuse.path + ".\n\nCreate a new Item referencing this file? No file will be copied.")) {
            say($("import-message"), "Reuse cancelled. Nothing was imported."); return;
          }
          saved = await post("/api/import", { ...body, reuse: saved.reuse.item });
        }
      } else saved = await post("/api/import", body);
      if (saved.warning) { state = await loadState(); render(); say($("import-message"), saved.warning, true); return; }
    }
    const done = selected;
    state = await loadState();
    if (noPDF) {
      render(); drawFields();
      say($("import-message"), "Saved without PDF. Add a scan later from Browse.");
      return;
    }
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

// drawShared offers a new Item's sharing: the owner field's other options,
// the owner chosen left out. Ticks survive a redraw.
function drawShared(t, adding) {
  const options = (t.fields.find((f) => f.key === "owner")?.options) || [];
  const owner = $("fields").querySelector('[name="owner"], [data-key="owner"]')?.value || "";
  const ticked = sharedWith();
  const people = options.filter((p) => p !== owner);
  $("shared-field").hidden = adding || !people.length;
  $("shared").replaceChildren(...people.map((p) => el("label", {},
    el("input", { type: "checkbox", value: p, checked: ticked.includes(p) }), " " + p)));
}
const sharedWith = () => $("shared-field").hidden ? [] : [...$("shared").querySelectorAll("input:checked")].map((i) => i.value);

$("cancel-without-pdf").onclick = () => {
  noPDF = false; clearPreview(); $("empty").textContent = "Open a folder, then pick a PDF."; render();
};

$("without-pdf").onclick = () => {
  noPDF = true; selected = ""; $("into").value = ""; suggestions = {}; suggestedType = ""; typeChosen = true;
  $("notes").value = ""; tags.set([]); revisionTags.set([]); $("shared").replaceChildren(); metadataItem = "";
  clearPreview(); $("type-hint").hidden = true;
  $("empty").textContent = "Enter the details now. Attach a PDF later from Browse.";
  render(); drawFields();
};

const back = history.state || {}; // what the page showed when it was left, on coming Back
const unscroll = scrollBack(["tree"]);
loadState().then(async (s) => {
  state = s;
  $("keep-original").checked = s.keepOriginal !== false;
  render();
  const into = new URLSearchParams(location.search).get("into");
  if (state.capabilities.without_file && new URLSearchParams(location.search).has("no_pdf")) {
    $("without-pdf").click();
    const item = state.items.find((i) => i.id === into);
    if (item) { selectedTemplate = item.type; drawFields(); $("into").value = item.id; drawFields(); }
  } else {
    const last = remembered();
    if (last) await open(last);
    // Coming Back, the file picked before is picked again.
    if (back.selected && files.some((f) => f.path === back.selected)) pick(back.selected);
    unscroll();
  }
}).catch((err) => {
  state.error = err.message;
  render();
});

let replacesRequest = 0;
async function drawReplaces() {
  const request = ++replacesRequest;
  const visible = (state.capabilities.replacement_any_type || selectedTemplate === "visa") && (!$("into").value || $("into-field").hidden);
  $("replaces-field").hidden = !visible;
  const previous = $("replaces").value;
  $("replaces").replaceChildren(el("option", { value: "" }, "None — keep existing visas in use"));
  if (!visible) return;
  const fields = fieldsOf($("fields"));
  if(state.capabilities.replacement_any_type){
    const candidates=state.items.filter(i=>i.type===selectedTemplate&&!i.retired&&!i.superseded_by&&i.fields.owner===fields.owner&&i.fields.country===fields.country);
    $("replaces").append(...candidates.map(i=>el("option",{value:i.id},label(state,i))));
    if(candidates.some(i=>i.id===previous))$("replaces").value=previous;
    return;
  }
  if (!fields.owner || !fields.country) return;
  try {
    const response = await fetch(api("/api/supersession-candidates?" + new URLSearchParams({ owner: fields.owner, country: fields.country })));
    const candidates = await response.json();
    if (request !== replacesRequest) return;
    if (!response.ok) throw new Error(candidates.error);
    $("replaces").append(...candidates.map((i) => el("option", { value: i.id }, [i.fields.visa_type, i.fields.number || i.id, i.fields.issued].filter(Boolean).join(" · "))));
    if (candidates.some((i) => i.id === previous)) $("replaces").value = previous;
  } catch (err) { if (request === replacesRequest) say($("import-message"), err.message, true); }
}
$("choose-destination").onclick = async () => {
  const chosen = await openFile({ title: "Store imported files", message: "Choose a folder inside this doc tree.", folders: true, folder: outputFolder || state.root, fallbacks: [state.root] });
  if (!chosen) return;
  outputFolder = Array.isArray(chosen) ? chosen[0] : chosen;
  $("destination").textContent = outputFolder;
};
