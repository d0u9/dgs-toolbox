// Browse: the Items kept in the tree, their fields, revisions and HEAD.
import { splitter } from "/ui/splitter.js";
import { openMenu } from "/ui/menu.js";
import { $, api, el, loadState, post, templateOf, label, inputFor, fieldsOf, fieldsAt, currentFields, tagUses, frame, say, showText, showPreview, clearPreview } from "/common.js";

let state = { templates: [], items: [] };
let selected = null; // {id, digest}
let editing = "";
let notesOf = "";
let tagsOf = "";
const detailTags = window.tagField($("detail-tags"), { known: () => tagUses(state.items), placeholder: "Add tags" });
const filterTags = window.tagField($("filter-tags"), {
  known: () => tagUses(state.items), only: true, placeholder: "any", onChange: () => render(),
});
// Items whose text matched the filter, by ID, with the line it matched on.
let textHits = new Map();
let unread = 0;

const words = () => $("filter").value.trim().toLowerCase().split(/\s+/).filter(Boolean);
const matches = (text) => words().every((w) => text.toLowerCase().includes(w));
const VIEW_KEY = "dgs-doc-browse-view";

// The keys that tell documents apart — owner, country — each get a filter,
// since they are what a person looks for first.
function filterKeys() {
  const keys = [];
  for (const t of state.templates) {
    for (const f of t.fields) if (f.distinguishing && !keys.includes(f.key)) keys.push(f.key);
  }
  return keys;
}

// filters draws a select per filter key with the values the Items have,
// keeping what was chosen.
function drawFilters() {
  const keep = (select, values) => {
    const was = select.value;
    select.replaceChildren(el("option", { value: "" }, "any"), ...values.map((v) => el("option", { value: v }, v)));
    select.value = values.includes(was) ? was : "";
  };
  keep($("filter-type"), [...new Set(state.items.map((i) => i.type))].sort());
  const group = $("field-filters");
  const keys = filterKeys();
  for (const label of [...group.querySelectorAll("label[data-key]")]) if (!keys.includes(label.dataset.key)) label.remove();
  for (const key of keys) {
    let select = group.querySelector(`select[data-key="${key}"]`);
    if (!select) {
      select = el("select", { onchange: render });
      select.dataset.key = key;
      const label = el("label", { className: "filter" }, el("span", {}, key[0].toUpperCase() + key.slice(1)), select);
      label.dataset.key = key;
      group.append(label);
    }
    keep(select, [...new Set(state.items.map((i) => currentFields(i)[key]).filter(Boolean))].sort());
  }
}

function shownItems() {
  const type = $("filter-type").value, kind = $("filter-kind").value, exp = $("filter-expiry").value;
  const requiredTags = filterTags.get();
  const byKey = [...$("field-filters").querySelectorAll("select[data-key]")].filter((s) => s.value);
  const items = state.items.filter((i) => {
    const fields = currentFields(i);
    return (!type || i.type === type) && (!kind || i.kind === kind) &&
      (!exp || expiryOf(i).state === exp) &&
      byKey.every((s) => fields[s.dataset.key] === s.value) &&
      requiredTags.every((tag) => (i.tags || []).includes(tag)) &&
      (textHits.has(i.id) || matches(label(state, i) + " " + Object.values(fields).join(" ") + " " + (i.tags || []).join(" ")));
  });
  const sort = $("view-sort").value, dir = $("view-direction").value === "asc" ? 1 : -1;
  const added = (i) => (i.revisions[i.revisions.length - 1] || {}).added || "";
  // No expiry sorts after any date, whichever way round.
  const exp_ = (i) => { const e = expiryOf(i); return e.date || (e.state === "permanent" ? "9999" : ""); };
  const keyOf = { added, expiry: exp_, name: (i) => label(state, i), type: (i) => i.type + " " + label(state, i) }[sort];
  return items.sort((x, y) => {
    const a = keyOf(x), b = keyOf(y);
    if (sort === "expiry" && (!a || !b)) return !a && !b ? 0 : !a ? 1 : -1;
    return a < b ? -dir : a > b ? dir : 0;
  });
}

const expiryOf = (item) => (state.expiry || {})[item.id] || { state: "none" };

// expiryBadge says where an Item's expiry stands, coloured by how urgent.
function expiryBadge(item) {
  const e = expiryOf(item);
  const text = {
    expired: "expired " + (e.date || ""),
    soon: e.days === 0 ? "expires today" : "expires in " + e.days + (e.days === 1 ? " day" : " days"),
    valid: "valid to " + (e.date || ""),
    permanent: "no end date",
  }[e.state];
  return text ? el("span", { className: "badge badge-" + e.state, title: e.date || "" }, text) : null;
}

const headOf = (item) => item.head || (item.revisions[item.revisions.length - 1] || {}).digest;
const thumbURL = (item) => { const d = headOf(item); return api(`/api/page?item=${encodeURIComponent(item.id)}&digest=${d}&n=1&size=thumb&v=${d}`); };

// thumb is the first page of HEAD, or the type's name where the PDF has no
// picture of its page (one made on a computer).
// noPicture remembers the pages that have no picture, so a redraw does not
// ask for them again.
const noPicture = new Set();
function thumb(item) {
  const wrap = el("div", { className: "card-thumb-wrap" });
  const url = thumbURL(item);
  const none = () => { noPicture.add(url); wrap.classList.add("no-picture"); wrap.dataset.type = item.type; };
  if (noPicture.has(url)) {
    none();
    return wrap;
  }
  const img = el("img", { className: "card-thumb", loading: "lazy", alt: "", src: url });
  img.addEventListener("error", none, { once: true });
  img.addEventListener("load", () => { if (!img.naturalWidth) none(); }, { once: true });
  wrap.append(img);
  return wrap;
}

// The fields a card shows under its title: what is not already in the name.
function details(item) {
  const t = templateOf(state, item.type);
  const named = new Set(t ? t.fields.filter((f) => f.distinguishing).map((f) => f.key) : []);
  return Object.entries(currentFields(item)).filter(([k]) => !named.has(k) && !["expires", "expiry", "expires_at", "expiry_date", "valid_until"].includes(k))
    .map(([k, v]) => k + ": " + shown(item, k, v)).join(" · ");
}

function card(item) {
  const t = templateOf(state, item.type);
  const named = t ? t.fields.filter((f) => f.distinguishing).map((f) => currentFields(item)[f.key]).filter(Boolean) : [];
  const li = el("li", { className: "card state-" + expiryOf(item).state, onclick: () => open(item), ondblclick: () => { open(item); openReader(); } },
    thumb(item),
    el("div", { className: "card-body" },
      el("p", { className: "card-title", title: label(state, item) }, named.join(" · ") || item.type),
      el("p", { className: "card-meta" }, el("span", { className: "card-type" }, item.type),
        item.revisions.length > 1 ? el("span", { title: "Revisions" }, item.revisions.length + " revisions") : null),
      el("p", { className: "card-details", title: details(item) }, details(item)),
      textHits.get(item.id) ? el("p", { className: "card-snippet" }, textHits.get(item.id)) : null,
      el("p", { className: "card-badges" }, expiryBadge(item))));
  if (selected && selected.id === item.id) li.classList.add("card-selected");
  return li;
}

function row(item, keys) {
  const fields = currentFields(item);
  const e = expiryOf(item);
  const tr = el("tr", { className: "table-row state-" + e.state, onclick: () => open(item), ondblclick: () => { open(item); openReader(); } },
    el("td", { className: "table-thumb" }, thumb(item)),
    el("td", {}, item.type),
    ...keys.map((k) => el("td", {}, fields[k] ? shown(item, k, fields[k]) : "")),
    el("td", {}, expiryBadge(item) || el("span", { className: "muted" }, "—")),
    el("td", { className: "numeric" }, String(item.revisions.length)),
    el("td", { className: "numeric" }, new Date((item.revisions[item.revisions.length - 1] || {}).added || 0).toLocaleDateString()));
  if (selected && selected.id === item.id) tr.classList.add("card-selected");
  return tr;
}

function open(item) {
  pick(item.id, headOf(item));
}

// The picture at the top of the detail panel: one page of the picked
// revision at a time, or the type's name where the PDF has no picture of it.
let shownPage = { id: "", digest: "", n: 1, count: 1 };
async function showImage(id, digest) {
  shownPage = { id, digest, n: 1, count: 1 };
  drawPage();
  try {
    const info = await (await fetch(api("/api/pages?" + new URLSearchParams({ item: id, digest })))).json();
    if (shownPage.id === id && shownPage.digest === digest) {
      shownPage.count = Math.max(1, info.count || 1);
      drawPage();
    }
  } catch { /* one page is shown */ }
}
function drawPage() {
  const { id, digest, n, count } = shownPage;
  const item = state.items.find((i) => i.id === id);
  const frame = $("detail-frame");
  frame.classList.remove("no-picture");
  frame.dataset.type = item ? item.type : "";
  $("detail-image").src = api(`/api/page?item=${encodeURIComponent(id)}&digest=${digest}&n=${n}&size=page&v=${digest}`);
  $("page-label").textContent = n + " / " + count;
  $("page-prev").disabled = n <= 1;
  $("page-next").disabled = n >= count;
}
const turn = (by) => {
  const n = Math.min(shownPage.count, Math.max(1, shownPage.n + by));
  if (n !== shownPage.n) { shownPage.n = n; drawPage(); }
};
$("page-prev").onclick = () => turn(-1);
$("page-next").onclick = () => turn(1);
$("detail-image").addEventListener("error", () => $("detail-frame").classList.add("no-picture"));
$("detail-image").addEventListener("load", () => { if (!$("detail-image").naturalWidth) $("detail-frame").classList.add("no-picture"); });

// The reader shows the picked revision in place of the cards, the detail
// panel still beside it. It is a step in the browser's history, so Back
// leaves the reader rather than the page.
function openReader() {
  if (!selected) return;
  const item = state.items.find((i) => i.id === selected.id);
  $("reader-title").textContent = item ? label(state, item) : "";
  if (!document.body.classList.contains("reading")) history.pushState({ reader: true }, "", location.hash || location.pathname + location.search);
  document.body.classList.add("reading");
  $("preview").hidden = false;
  showPreview({ item: selected.id, digest: selected.digest },
    api("/api/revision?item=" + encodeURIComponent(selected.id) + "&digest=" + encodeURIComponent(selected.digest)));
}
function hideReader() {
  document.body.classList.remove("reading");
  $("preview").hidden = true;
  clearPreview();
}
function closeReader() {
  if (history.state && history.state.reader) history.back();
  else hideReader();
}
window.addEventListener("popstate", () => { if (document.body.classList.contains("reading")) hideReader(); });
$("open-reader").onclick = openReader;
$("detail-frame").onclick = openReader;
$("reader-back").onclick = closeReader;

function render() {
  frame(state);
  if (state.soonDays) $("soon-option").textContent = "expires within " + state.soonDays + " days";
  drawFilters();
  const items = shownItems();
  $("count").textContent = items.length === state.items.length ? items.length + " Items" : items.length + " of " + state.items.length + " Items";
  $("none").hidden = state.items.length > 0 || !state.tree;
  $("nothing").hidden = !state.items.length || items.length > 0;
  $("filters-clear").hidden = !filtering();
  $("filter-tags").closest(".filter").classList.toggle("filter-active", filterTags.get().length > 0);
  for (const s of document.querySelectorAll(".filters select")) s.closest(".filter").classList.toggle("filter-active", !!s.value && !s.id.startsWith("view-"));
  $("unread").hidden = !unread;
  $("unread-count").textContent = unread + (unread === 1 ? " Item's text is" : " Items' text is") + " not read yet, so searching cannot find " + (unread === 1 ? "it." : "them.");
  const list = $("view-layout").value === "list";
  $("grid").hidden = list;
  $("table").hidden = !list;
  if (list) {
    const keys = filterKeys();
    $("table-head").replaceChildren(el("th", {}), el("th", {}, "Type"), ...keys.map((k) => el("th", {}, k)),
      el("th", {}, "Expiry"), el("th", {}, "Revisions"), el("th", {}, "Added"));
    $("table-body").replaceChildren(...items.map((i) => row(i, keys)));
  } else {
    $("grid").replaceChildren(...items.map(card));
  }
  const item = selected && state.items.find((i) => i.id === selected.id);
  $("side").hidden = !item;
  $("side-splitter").hidden = !item;
  if (item) detail(item);
}

const filtering = () => $("filter").value.trim() || filterTags.get().length || [...document.querySelectorAll(".filters select")].some((s) => s.value && !s.id.startsWith("view-"));

$("filters-clear").onclick = () => {
  $("filter").value = "";
  filterTags.set([]);
  for (const s of document.querySelectorAll(".filters select")) if (!s.id.startsWith("view-")) s.value = "";
  textHits = new Map();
  render();
  searchText();
};
for (const id of ["filter-type", "filter-expiry", "filter-kind", "view-layout", "view-sort", "view-direction"]) {
  $(id).addEventListener("change", () => {
    try { localStorage.setItem(VIEW_KEY, JSON.stringify({ layout: $("view-layout").value, sort: $("view-sort").value, direction: $("view-direction").value })); } catch { /* not kept */ }
    render();
  });
}
try {
  const v = JSON.parse(localStorage.getItem(VIEW_KEY) || "{}");
  if (v.layout) $("view-layout").value = v.layout;
  if (v.sort) $("view-sort").value = v.sort;
  if (v.direction) $("view-direction").value = v.direction;
} catch { /* the defaults stand */ }

// Closing the detail gives the cards the width back.
function close() {
  selected = null;
  history.replaceState(null, "", location.pathname + location.search);
  closeReader();
  render();
}
$("close").onclick = close;
document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape" || event.target.closest("input, textarea, select")) return;
  if (document.body.classList.contains("reading")) closeReader();
  else if (selected) close();
});

// The picture is as tall as the reader drags it.
splitter({ handle: $("picture-splitter"), target: $("detail-frame"), axis: "y", min: 120,
  max: () => Math.max(160, window.innerHeight - 280), key: "dgs-doc-size-browse-picture" });

// The detail panel is as wide as the reader drags it, and never so narrow
// that its fields do not fit.
splitter({ handle: $("side-splitter"), target: $("side"), axis: "x", invert: true, min: 320,
  max: () => Math.max(320, window.innerWidth - 480), key: "dgs-doc-size-browse-side" });

// shown is a field's value as a person reads it: a linked Item by its name.
function shown(item, key, value) {
  const t = templateOf(state, item.type);
  const f = t && t.fields.find((x) => x.key === key);
  const other = f && f.type === "item" && state.items.find((i) => i.id === value);
  return other ? label(state, other) : value;
}

function pick(id, digest) {
  if (!selected || selected.id !== id || selected.digest !== digest) {
    showImage(id, digest);
    showText({ item: id, digest });
    if (!$("preview").hidden) showPreview({ item: id, digest }, api("/api/revision?item=" + encodeURIComponent(id) + "&digest=" + encodeURIComponent(digest)));
  }
  if (!selected || selected.id !== id) {
    $("similar").replaceChildren();
    say($("similar-message"), "");
  }
  selected = { id, digest };
  history.replaceState(null, "", "#" + id);
  render();
}

function detail(item) {
  $("detail-head").textContent = label(state, item);
  $("change-type").href = api("/change-type/?item=" + encodeURIComponent(item.id));
  const history = [...(item.history || [])];
  for (const r of item.revisions) if (!history.some((event) => event.action === "import" || event.action === "import_revision" ? event.digest === r.digest : false))
    history.push({ at: r.added, action: "import", digest: r.digest });
  history.sort((a, b) => (b.at || "").localeCompare(a.at || ""));
  $("history").replaceChildren(...history.map((event) => {
    const title = { import: "Imported", import_revision: "Added revision", edit_fields: "Changed fields", edit_notes: "Changed notes",
      edit_tags: "Changed tags", make_head: "Made HEAD", delete_revision: "Deleted revision", change_type: "Changed type", export: "Exported" }[event.action] || event.action;
    const changes = Object.entries(event.changes || {}).map(([key, pair]) => `${key}: ${pair[0] || "∅"} → ${pair[1] || "∅"}`).join("; ");
    const oldFields = event.action === "change_type" ? Object.entries(event.previous_fields || {}).map(([key, value]) => `${key}: ${value}`).join("; ") : "";
    const newFields = event.action === "change_type" ? Object.entries(event.new_fields || {}).map(([key, value]) => `${key}: ${value}`).join("; ") : "";
    const oldRevisions = event.action === "change_type" ? Object.entries(event.previous_revisions || {}).map(([digest, values]) =>
      `${digest.slice(0, 8)} (${Object.entries(values || {}).map(([key, value]) => `${key}: ${value}`).join("; ")})`).join("; ") : "";
    const newRevisions = event.action === "change_type" ? Object.entries(event.new_revisions || {}).map(([digest, values]) =>
      `${digest.slice(0, 8)} (${Object.entries(values || {}).map(([key, value]) => `${key}: ${value}`).join("; ")})`).join("; ") : "";
    return el("li", {}, el("strong", {}, title), el("span", { className: "sub" }, new Date(event.at).toLocaleString(),
      event.from_type ? ` · ${event.from_type} → ${event.to_type}` : "", event.digest ? " · " + event.digest.slice(0, 8) : "",
      event.target ? " · " + event.target : "", event.view ? " · " + event.view : ""),
      changes ? el("span", { className: "sub", title: changes }, changes) : null,
      oldFields ? el("span", { className: "sub", title: oldFields }, "Previous fields: " + oldFields) : null,
      newFields ? el("span", { className: "sub", title: newFields }, "New fields: " + newFields) : null,
      oldRevisions ? el("span", { className: "sub", title: oldRevisions }, "Previous revision fields: " + oldRevisions) : null,
      newRevisions ? el("span", { className: "sub", title: newRevisions }, "New revision fields: " + newRevisions) : null);
  }));
  const n = item.revisions.findIndex((r) => r.digest === selected.digest) + 1;
  $("detail-rev").textContent = item.kind === "document"
    ? "Revision " + n + " of " + item.revisions.length + (selected.digest === item.head ? " · HEAD" : "")
    : "Record";
  const t = templateOf(state, item.type);
  // The fields as the picked revision has them: its per_revision values are
  // its own, so picking the old card shows the old card's number.
  const fields = fieldsAt(item, selected.digest);
  if (editing !== item.id + selected.digest + JSON.stringify(fields)) {
    editing = item.id + selected.digest + JSON.stringify(fields);
    say($("edit-message"), "");
    const perRevision = t && t.fields.some((f) => f.per_revision) && item.revisions.length > 1;
    const n = item.revisions.findIndex((r) => r.digest === selected.digest) + 1;
    $("edit-fields").replaceChildren(el("p", { className: "kind" }, item.kind,
      perRevision ? " · fields marked ↻ are revision " + n + "'s own" : ""),
      ...(t ? t.fields : Object.keys(fields).map((key) => ({ key }))).map((f) => {
        const input = inputFor(f, fields[f.key] || "", "", state, item.id);
        if (perRevision && f.per_revision) input.firstChild.append(el("span", { className: "muted", title: "Kept with each revision" }, " ↻"));
        return input;
      }));
    $("save-button").disabled = !t;
  }
  drawCases(item);
  if (notesOf !== item.id + "\n" + (item.notes || "")) {
    notesOf = item.id + "\n" + (item.notes || "");
    $("notes").value = item.notes || "";
    say($("notes-message"), "");
  }
  if (tagsOf !== item.id + "\n" + JSON.stringify(item.tags || [])) {
    tagsOf = item.id + "\n" + JSON.stringify(item.tags || []);
    detailTags.set(item.tags || []);
    say($("tags-message"), "");
  }
  $("revisions").replaceChildren(...item.revisions.slice().reverse().map((r) => {
    const isHead = r.digest === item.head;
    const revisionMenu = (event) => openMenu(event, [
      item.kind === "document" && !isHead ? [{ label: "Make HEAD", onSelect: () => makeHead(item.id, r.digest) }] : [],
      [{ label: "Delete revision…", danger: true, disabled: item.revisions.length < 2,
        title: item.revisions.length < 2 ? "Delete the Item to remove its last revision" : "Move this revision's PDF to trash",
        onSelect: () => deleteRevision(item.id, r.digest) }],
    ]);
    const li = el("li", { className: "rev", onclick: () => pick(item.id, r.digest),
      oncontextmenu: revisionMenu },
      el("div", { className: "rev-text" },
        el("span", {}, new Date(r.added).toLocaleDateString(), isHead ? el("span", { className: "tag" }, "HEAD") : null),
        el("span", { className: "sub", title: r.digest }, (r.source ? r.source + " · " : "") + r.digest.slice(0, 8))),
      item.kind === "document" && !isHead ? el("button", {
        className: "button", type: "button", textContent: "Make HEAD",
        onclick: (event) => { event.stopPropagation(); makeHead(item.id, r.digest); },
      }) : null,
      el("button", { className: "tool", type: "button", textContent: "⋯", title: "Revision actions", "aria-label": "Revision actions",
        onclick: (event) => { event.stopPropagation(); revisionMenu(event); } }));
    if (selected.digest === r.digest) li.classList.add("selected");
    return li;
  }));
}

async function makeHead(id, digest) {
  say($("head-message"), "");
  try {
    await post("/api/head", { item: id, digest });
    await reload();
    pick(id, digest);
  } catch (err) {
    say($("head-message"), err.message, true);
  }
}

async function deleteRevision(id, digest) {
  const item = state.items.find((i) => i.id === id);
  if (!item || item.revisions.length < 2) return;
  const n = item.revisions.findIndex((r) => r.digest === digest) + 1;
  if (!confirm(`Delete revision ${n} of ${label(state, item)}?\n\nIts PDF moves into the tree's trash folder. The PDF you imported from is not touched.`)) return;
  say($("head-message"), "");
  try {
    await post("/api/revisions/delete", { item: id, digest });
    const keep = selected?.id === id && selected.digest !== digest ? selected.digest : "";
    selected = null;
    await reload();
    const remaining = state.items.find((i) => i.id === id);
    if (remaining) pick(id, keep || headOf(remaining));
    say($("head-message"), "Revision moved to trash.");
  } catch (err) { say($("head-message"), err.message, true); }
}

$("edit").onsubmit = async (event) => {
  event.preventDefault();
  try {
    await post("/api/fields", { item: selected.id, digest: selected.digest, fields: fieldsOf($("edit-fields")) });
    await reload();
    say($("edit-message"), "Saved.");
  } catch (err) {
    say($("edit-message"), err.message, true);
  }
};

// The Cases an Item is in, and putting it in an open one.
let cases = [];
async function loadCases() {
  try {
    cases = (await (await fetch(api("/api/cases"))).json()).cases || [];
  } catch {
    cases = [];
  }
}
let casesOf = "";
function drawCases(item) {
  const key = item.id + JSON.stringify(cases.map((c) => [c.name, c.status, c.entries.length]));
  if (casesOf === key) return;
  casesOf = key;
  say($("cases-message"), "");
  const holding = cases.filter((c) => c.entries.some((e) => e.item === item.id));
  $("item-cases").replaceChildren(...(holding.length ? holding.map((c) => el("a", {
    className: "chip", href: api("/cases/") + "#" + encodeURIComponent(c.name),
    title: c.status === "archived" ? "Archived" : "Open" }, (c.status === "archived" ? "▣ " : "") + (c.title || c.name)))
    : [el("span", { className: "muted" }, "In no Case.")]));
  const open = cases.filter((c) => c.status === "open" && !holding.includes(c));
  $("to-case").replaceChildren(el("option", { value: "" }, open.length ? "Add to a Case…" : "No open Case to add to"),
    ...open.map((c) => el("option", { value: c.name }, c.title || c.name)));
  $("to-case").disabled = $("to-case-add").disabled = $("to-case-note").disabled = !open.length;
}
$("to-case-add").onclick = async () => {
  const name = $("to-case").value;
  if (!name || !selected) return;
  try {
    await post("/api/cases/change", { op: "add", name, item: selected.id, note: $("to-case-note").value });
    $("to-case-note").value = "";
    await loadCases();
    casesOf = "";
    const item = state.items.find((i) => i.id === selected.id);
    if (item) drawCases(item);
    say($("cases-message"), "Added.");
  } catch (err) {
    say($("cases-message"), err.message, true);
  }
};

$("notes-form").onsubmit = async (event) => {
  event.preventDefault();
  try {
    await post("/api/notes", { item: selected.id, notes: $("notes").value });
    await reload();
    say($("notes-message"), "Saved.");
  } catch (err) {
    say($("notes-message"), err.message, true);
  }
};

$("tags-form").onsubmit = async (event) => {
  event.preventDefault();
  detailTags.commit();
  try {
    await post("/api/tags", { item: selected.id, tags: detailTags.get() });
    await reload();
    say($("tags-message"), "Saved.");
  } catch (err) {
    say($("tags-message"), err.message, true);
  }
};

$("delete").onclick = async () => {
  const item = selected && state.items.find((i) => i.id === selected.id);
  if (!item) return;
  const n = item.revisions.length;
  if (!confirm(`Delete ${label(state, item)}?\n\nIts ${n === 1 ? "PDF" : n + " PDFs"} and fields move into the tree's trash folder. The PDF you imported from is not touched.`)) return;
  try {
    const answer = await post("/api/items/delete", { item: item.id });
    selected = null;
    history.replaceState(null, "", location.pathname + location.search);
    await reload();
    clearPreview();
    say($("list-message"), "Deleted. It is in " + answer.trash + ".");
  } catch (err) {
    say($("delete-message"), err.message, true);
  }
};

// searchText asks the server which Items' text holds every word typed.
let searchTimer = 0;
let searchAsked = 0;
function searchText() {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(async () => {
    const mine = ++searchAsked;
    const q = $("filter").value.trim();
    try {
      const answer = await (await fetch(api("/api/search?q=" + encodeURIComponent(q)))).json();
      if (mine !== searchAsked) return;
      textHits = new Map((answer.hits || []).map((h) => [h.id, h.snippet]));
      unread = answer.unread || 0;
      render();
    } catch { /* the filter on fields still works */ }
  }, 200);
}

$("filter").oninput = () => { render(); searchText(); };

$("read-all").onclick = async () => {
  try {
    const answer = await post("/api/read-all", {});
    $("unread-count").textContent = answer.queued + " queued to be read; search again in a moment.";
    $("read-all").hidden = true;
  } catch (err) {
    $("unread-count").textContent = err.message;
  }
};

$("more").onclick = async () => {
  const id = selected && selected.id;
  if (!id) return;
  say($("similar-message"), "Comparing…");
  try {
    const answer = await (await fetch(api("/api/similar?item=" + encodeURIComponent(id)))).json();
    if (!selected || selected.id !== id) return;
    const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
    $("similar").replaceChildren(...answer.hits.filter((h) => byId[h.id]).map((h) => el("li", {
      onclick: () => { const it = byId[h.id]; pick(it.id, it.head || it.revisions[0].digest); },
    }, label(state, byId[h.id]), el("span", { className: "sub" }, Math.round(h.score * 100) + "% alike"))));
    say($("similar-message"), answer.hits.length ? "" : "Nothing alike among the Items whose text has been read.");
  } catch (err) {
    say($("similar-message"), err.message, true);
  }
};

async function reload() {
  [state] = await Promise.all([loadState(), loadCases()]);
  render();
}

reload().then(() => {
  searchText();
  const wanted = state.items.find((i) => i.id === location.hash.slice(1));
  if (wanted) pick(wanted.id, wanted.head || wanted.revisions[0].digest);
}).catch((err) => {
  state.error = err.message;
  render();
});
