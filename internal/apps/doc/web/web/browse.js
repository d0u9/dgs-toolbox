// Browse: the Items kept in the tree, their fields, revisions and HEAD.
import { openFile } from "/ui/filedialog.js";
import { splitter } from "/ui/splitter.js";
import { openMenu } from "/ui/menu.js";
import { $, api, el, loadState, post, templateOf, label, inputFor, fieldsOf, fieldsAt, currentFields, tagUses, tagsAt, frame, say, showText, showPreview, clearPreview, eventLines } from "/common.js";

let state = { templates: [], items: [] };
let selected = null; // {id, digest}
let editing = "";
let notesOf = "";
let tagsOf = "";
const detailTags = window.tagField($("detail-tags"), { known: () => tagUses(state.items), placeholder: "Add tags" });
const revisionTags = window.tagField($("revision-tags"), { known: () => tagUses(state.items), placeholder: "Add tags" });
let revisionTagsOf = "";
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
  // The list can sort by any of its columns, so Sort offers them too.
  const sortSelect = $("view-sort"), wantSort = sortSelect.dataset.want || sortSelect.value;
  for (const o of [...sortSelect.options]) if (o.dataset.column) o.remove();
  for (const [value, text] of [...filterKeys().map((k) => ["field:" + k, k]), ["revisions", "revisions"]]) {
    const o = el("option", { value }, text);
    o.dataset.column = "1";
    sortSelect.append(o);
  }
  sortSelect.value = wantSort;
  if (sortSelect.value !== wantSort) sortSelect.value = "added";
  delete sortSelect.dataset.want;
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
  const type = $("filter-type").value, kind = $("filter-kind").value, exp = $("filter-expiry").value, use = $("filter-use").value;
  const requiredTags = filterTags.get();
  const byKey = [...$("field-filters").querySelectorAll("select[data-key]")].filter((s) => s.value);
  const items = state.items.filter((i) => {
    const fields = currentFields(i);
    return (!onlyFrequent() || i.frequent) && (!use || (use === "retired") === !!i.retired) && (!type || i.type === type) && (!kind || i.kind === kind) &&
      (!exp || expiryOf(i).state === exp) &&
      byKey.every((s) => fields[s.dataset.key] === s.value) &&
      // An Item has the tags its HEAD has: its own and HEAD's.
      requiredTags.every((tag) => tagsAt(i, headOf(i)).includes(tag)) &&
      (textHits.has(i.id) || matches(label(state, i) + " " + Object.values(fields).join(" ") + " " + tagsAt(i, headOf(i)).join(" ")));
  });
  const sort = $("view-sort").value, dir = $("view-direction").value === "asc" ? 1 : -1;
  const added = (i) => (i.revisions[i.revisions.length - 1] || {}).added || "";
  // No expiry sorts after any date, whichever way round.
  const exp_ = (i) => { const e = expiryOf(i); return e.date || (e.state === "permanent" ? "9999" : ""); };
  const keyOf = sort.startsWith("field:") ? (i) => shown(i, sort.slice(6), currentFields(i)[sort.slice(6)] || "")
    : { added, expiry: exp_, name: (i) => label(state, i), type: (i) => i.type + " " + label(state, i),
      revisions: (i) => i.revisions.length }[sort] || added;
  // Retired Items go last and frequent ones first, whatever the sort; the
  // sort orders each part.
  return items.sort((x, y) => {
    if (!!x.retired !== !!y.retired) return x.retired ? 1 : -1;
    if (!!x.frequent !== !!y.frequent) return x.frequent ? -1 : 1;
    const a = keyOf(x), b = keyOf(y);
    // No value sorts after any value, whichever way round.
    if ((sort === "expiry" || sort.startsWith("field:")) && (!a || !b)) return !a && !b ? 0 : !a ? 1 : -1;
    if (typeof a === "string") return a.localeCompare(b, undefined, { numeric: true }) * dir;
    return a < b ? -dir : a > b ? dir : 0;
  });
}

const expiryOf = (item) => (state.expiry || {})[item.id] || { state: "none" };

// retiredBadge marks an Item no longer used, its reason on hover.
const retiredBadge = (item) => item.retired
  ? el("span", { className: "badge badge-retired", title: item.retired_reason || "No longer used" }, item.superseded_by ? "superseded" : "retired") : null;

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

const hasPDF = (item, ref) => !!item?.revisions.find((r) => (r.id || r.digest) === ref)?.digest;
const headOf = (item) => item.head || ((item.revisions[item.revisions.length - 1] || {}).id || (item.revisions[item.revisions.length - 1] || {}).digest);
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
  if (!hasPDF(item, headOf(item))) {
    wrap.classList.add("no-picture"); wrap.dataset.type = "No PDF"; return wrap;
  }
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
  const li = el("li", { className: "card state-" + expiryOf(item).state + (item.retired ? " retired" : ""), onclick: () => open(item), ondblclick: () => { open(item); openReader(); } },
    thumb(item), star(item),
    el("div", { className: "card-body" },
      el("p", { className: "card-title", title: label(state, item) }, named.join(" · ") || item.type),
      el("p", { className: "card-meta" }, el("span", { className: "card-type" }, item.type),
        item.revisions.length > 1 ? el("span", { title: "Revisions" }, item.revisions.length + " revisions") : null),
      el("p", { className: "card-details", title: details(item) }, details(item)),
      textHits.get(item.id) ? el("p", { className: "card-snippet" }, textHits.get(item.id)) : null,
      el("p", { className: "card-badges" }, retiredBadge(item), expiryBadge(item))));
  if (selected && selected.id === item.id) li.classList.add("card-selected");
  return li;
}

// star marks an Item frequent, or unmarks it, from its card or row.
function star(item) {
  const on = !!item.frequent;
  const button = el("button", { type: "button", className: "star" + (on ? " on" : ""), textContent: on ? "★" : "☆",
    title: on ? "Frequent: click to unmark" : "Mark frequent", onclick: (event) => { event.stopPropagation(); setFrequent(item, !on); },
    ondblclick: (event) => event.stopPropagation() });
  button.setAttribute("aria-pressed", String(on));
  button.setAttribute("aria-label", "Frequent");
  return button;
}

// The list's columns: each can be made wider or narrower by dragging its
// right edge, remembered per column, and a click on a sortable one sorts by
// it, again to turn the order round.
const COLUMN_KEY = "dgs-doc-browse-column-";
const columnWidth = (id, fallback) => { try { return Number(localStorage.getItem(COLUMN_KEY + id)) || fallback; } catch { return fallback; } };
function drawHead(keys) {
  const columns = [
    { id: "thumb", text: "", width: 56, fixed: true },
    { id: "frequent", text: "★", title: "Frequent", width: 36, fixed: true },
    { id: "type", text: "Type", sort: "type", width: 140 },
    ...keys.map((k) => ({ id: "field:" + k, text: k, sort: "field:" + k, width: 140 })),
    { id: "expiry", text: "Expiry", sort: "expiry", width: 150 },
    { id: "revisions", text: "Revisions", sort: "revisions", width: 90 },
    { id: "added", text: "Added", sort: "added", width: 110 },
  ];
  const sort = $("view-sort").value, asc = $("view-direction").value === "asc";
  const cols = columns.map((c) => el("col", {}));
  const total = () => cols.reduce((sum, col) => sum + parseFloat(col.style.width), 0);
  const fit = () => { $("table").style.width = total() + "px"; };
  columns.forEach((c, n) => { cols[n].style.width = columnWidth(c.id, c.width) + "px"; });
  $("table-cols").replaceChildren(...cols);
  fit();
  $("table-head").replaceChildren(...columns.map((c, n) => {
    const th = el("th", { title: c.title || "" });
    if (c.sort) {
      const on = sort === c.sort;
      th.classList.add("sortable");
      th.setAttribute("aria-sort", on ? (asc ? "ascending" : "descending") : "none");
      th.append(el("button", { type: "button", className: "th-sort", onclick: () => sortBy(c.sort) },
        c.text, el("span", { className: "th-arrow" }, on ? (asc ? "▲" : "▼") : "")));
    } else th.append(c.text);
    if (!c.fixed) {
      const handle = el("div", { className: "th-resize", role: "separator", onclick: (event) => event.stopPropagation() });
      handle.setAttribute("aria-orientation", "vertical");
      handle.setAttribute("aria-label", "Width of " + c.text);
      th.append(handle);
      splitter({ handle, target: th, axis: "x", min: 48, key: COLUMN_KEY + c.id, fallback: c.width,
        set: (size) => { cols[n].style.width = size + "px"; fit(); } });
    }
    return th;
  }));
}
function sortBy(column) {
  if ($("view-sort").value === column) $("view-direction").value = $("view-direction").value === "asc" ? "desc" : "asc";
  else {
    $("view-sort").value = column;
    $("view-direction").value = column === "added" || column === "revisions" ? "desc" : "asc";
  }
  saveView();
  render();
}

function row(item, keys) {
  const fields = currentFields(item);
  const e = expiryOf(item);
  const tr = el("tr", { className: "table-row state-" + e.state + (item.retired ? " retired" : ""), onclick: () => open(item), ondblclick: () => { open(item); openReader(); } },
    el("td", { className: "table-thumb" }, thumb(item)),
    el("td", { className: "table-star" }, star(item)),
    el("td", {}, item.type),
    ...keys.map((k) => el("td", {}, fields[k] ? shown(item, k, fields[k]) : "")),
    el("td", {}, retiredBadge(item), expiryBadge(item) || (item.retired ? null : el("span", { className: "muted" }, "—"))),
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
  if (!hasPDF(state.items.find((i) => i.id === id), digest)) return;
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
  const attached = hasPDF(item, digest);
  $("detail-picture").classList.toggle("without-pdf", !attached);
  $("picture-splitter").hidden = !attached;
  $("detail-frame").hidden = !attached;
  $("page-prev").hidden = !attached;
  $("page-next").hidden = !attached;
  $("open-reader").hidden = !attached;
  $("detail-image").hidden = !attached;
  $("open-reader").disabled = !attached;
  if (!attached) {
    frame.classList.add("no-picture"); frame.dataset.type = "No PDF";
    $("page-label").textContent = "No attachment";
    $("page-prev").disabled = true; $("page-next").disabled = true;
    return;
  }
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
  if (!hasPDF(item, selected.digest)) return;
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
  const list = layout === "list";
  $("grid").hidden = list;
  $("table").hidden = !list;
  if (list) {
    const keys = filterKeys();
    drawHead(keys);
    $("table-body").replaceChildren(...items.map((i) => row(i, keys)));
  } else {
    $("grid").replaceChildren(...items.map(card));
  }
  const item = selected && state.items.find((i) => i.id === selected.id);
  $("side").hidden = !item;
  $("side-splitter").hidden = !item;
  if (item) detail(item);
}

const filtering = () => onlyFrequent() || $("filter").value.trim() || filterTags.get().length || [...document.querySelectorAll(".filters select")].some((s) => s.value && !s.id.startsWith("view-"));

$("filters-clear").onclick = () => {
  $("filter").value = "";
  $("filter-frequent").setAttribute("aria-pressed", "false");
  saveView();
  filterTags.set([]);
  for (const s of document.querySelectorAll(".filters select")) if (!s.id.startsWith("view-")) s.value = "";
  textHits = new Map();
  render();
  searchText();
};
function saveView() {
  try {
    localStorage.setItem(VIEW_KEY, JSON.stringify({ layout, sort: $("view-sort").value,
      direction: $("view-direction").value, frequent: onlyFrequent() }));
  } catch { /* not kept */ }
}
// Cards or list: two buttons, the pressed one is how the Items are shown.
let layout = "grid";
function setLayout(value) {
  layout = value === "list" ? "list" : "grid";
  for (const b of $("view-layout").querySelectorAll("button")) b.setAttribute("aria-pressed", String(b.dataset.layout === layout));
}
for (const b of $("view-layout").querySelectorAll("button")) {
  b.onclick = () => {
    setLayout(b.dataset.layout);
    saveView();
    render();
  };
}
for (const id of ["filter-type", "filter-expiry", "filter-kind", "filter-use", "view-sort", "view-direction"]) {
  $(id).addEventListener("change", () => {
    saveView();
    render();
  });
}
try {
  const v = JSON.parse(localStorage.getItem(VIEW_KEY) || "{}");
  if (v.frequent) $("filter-frequent").setAttribute("aria-pressed", "true");
  if (v.layout) setLayout(v.layout);
  if (v.sort) { $("view-sort").value = v.sort; $("view-sort").dataset.want = v.sort; }
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
    if (hasPDF(state.items.find((i) => i.id === id), digest)) showText({ item: id, digest });
    else { clearPreview(); say($("text-message"), ""); }
    if (hasPDF(state.items.find((i) => i.id === id), digest) && !$("preview").hidden) showPreview({ item: id, digest }, api("/api/revision?item=" + encodeURIComponent(id) + "&digest=" + encodeURIComponent(digest)));
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
  drawHistory(item);
  drawRetired(item);
  $("history-all").href = api("/log/") + "#" + encodeURIComponent(item.id);
  $("frequent").setAttribute("aria-pressed", String(!!item.frequent));
  $("frequent").textContent = item.frequent ? "★ Frequent" : "☆ Frequent";
  $("attach-pdf").hidden = false;
  $("attach-pdf").title = "Save the PDF with these fields as a new revision";
  $("attach-pdf").textContent = hasPDF(item, selected.digest) ? "Replace PDF…" : "Attach PDF…";
  $("new-without-pdf").hidden = item.kind !== "document";
  $("new-without-pdf").href = api("/import/?no_pdf=1&into=" + encodeURIComponent(item.id));
  const n = item.revisions.findIndex((r) => (r.id || r.digest) === selected.digest) + 1;
  $("detail-rev").textContent = item.kind === "document" || item.revisions.length > 1
    ? "Revision " + n + " of " + item.revisions.length + (selected.digest === item.head ? " · HEAD" : "")
    : "Record";
  const chosen = item.revisions.find((r) => (r.id || r.digest) === selected.digest);
  const t = templateOf(state, chosen?.type || item.type);
  // The fields as the picked revision has them: its per_revision values are
  // its own, so picking the old card shows the old card's number.
  const fields = fieldsAt(item, selected.digest);
  if (editing !== item.id + selected.digest + JSON.stringify(fields)) {
    editing = item.id + selected.digest + JSON.stringify(fields);
    say($("edit-message"), "");
    $("edit-fields").replaceChildren(
      ...(t ? t.fields : Object.keys(fields).map((key) => ({ key }))).map((f) =>
        inputFor(f, fields[f.key] || "", "", state, item.id, t?.type || item.type)));
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
  // Revision tags belong to the selected snapshot, with or without a PDF.
  const rev = item.revisions.find((r) => (r.id || r.digest) === selected.digest);
  $("revision-tags-form").hidden = !rev;
  if (rev && revisionTagsOf !== item.id + (rev.id || rev.digest) + JSON.stringify(rev.tags || [])) {
    revisionTagsOf = item.id + (rev.id || rev.digest) + JSON.stringify(rev.tags || []);
    revisionTags.set(rev.tags || []);
    const n = item.revisions.indexOf(rev) + 1;
    $("revision-tags-label").textContent = "Revision " + n + "'s own" + ((rev.id || rev.digest) === item.head ? " (HEAD)" : "");
    say($("revision-tags-message"), "");
  }
  $("revisions").replaceChildren(...item.revisions.slice().reverse().map((r) => {
    const isHead = (r.id || r.digest) === item.head;
    const revisionMenu = (event) => openMenu(event, [
      !isHead ? [{ label: "Make HEAD", onSelect: () => makeHead(item.id, (r.id || r.digest)) }] : [],
      [{ label: "Delete revision…", danger: true, disabled: item.revisions.length < 2,
        title: item.revisions.length < 2 ? "Delete the Item to remove its last revision" : (r.digest ? "Move this revision's PDF to trash" : "Save this revision's metadata in trash"),
        onSelect: () => deleteRevision(item.id, (r.id || r.digest)) }],
    ]);
    const li = el("li", { className: "rev", onclick: () => pick(item.id, (r.id || r.digest)),
      oncontextmenu: revisionMenu },
      el("div", { className: "rev-text" },
        el("span", {}, new Date(r.added).toLocaleDateString(), isHead ? el("span", { className: "tag" }, "HEAD") : null),
        el("span", { className: "sub", title: (r.id || r.digest) }, r.digest ? (r.source ? r.source + " · " : "") + r.digest.slice(0, 8) : "No PDF"),
        (r.tags || []).length ? el("span", { className: "rev-tags" }, ...r.tags.map((t) => el("span", { className: "tag" }, t))) : null),
      !isHead ? el("button", {
        className: "button", type: "button", textContent: "Make HEAD",
        onclick: (event) => { event.stopPropagation(); makeHead(item.id, (r.id || r.digest)); },
      }) : null,
      el("button", { className: "tool", type: "button", textContent: "⋯", title: "Revision actions", "aria-label": "Revision actions",
        onclick: (event) => { event.stopPropagation(); revisionMenu(event); } }));
    if (selected.digest === (r.id || r.digest)) li.classList.add("selected");
    return li;
  }));
}

// The picked Item's history, newest first, as the server orders it. It is
// asked for again only when the Item's history has changed.
let historyOf = "";
async function drawHistory(item) {
  const key = item.id + "\n" + (item.history || []).length + "\n" + item.revisions.length;
  if (historyOf === key) return;
  historyOf = key;
  try {
    const answer = await (await fetch(api("/api/history?item=" + encodeURIComponent(item.id)))).json();
    if (historyOf !== key) return;
    $("history").replaceChildren(...(answer.entries || []).map((entry) => {
      const { title, meta, lines } = eventLines(entry.event);
      return el("li", {}, el("strong", {}, title), el("span", { className: "sub" }, meta),
        ...lines.map((line) => el("span", { className: "sub", title: line }, line)));
    }));
  } catch (err) {
    historyOf = "";
    $("history").replaceChildren(el("li", { className: "muted" }, err.message));
  }
}

// The Retire section: set an Item aside as no longer used, with an optional
// reason, or put it back in use.
let retiredOf = "";
function drawRetired(item) {
  drawSupersession(item);
  const key = item.id + "\n" + !!item.retired + "\n" + (item.retired_reason || "");
  if (retiredOf === key) return;
  retiredOf = key;
  $("retire-reason").value = item.retired_reason || "";
  $("retire-note").textContent = item.retired
    ? "Retired: shown faded and last on Browse. Exports still include it."
    : "Keep it, but set it aside: no longer used, though it has not expired. It is shown faded and last; exports are not affected.";
  $("retire").textContent = item.retired ? "Save reason" : "Retire";
  $("unretire").hidden = !item.retired;
  say($("retire-message"), "");
}
async function setRetired(retired) {
  if (!selected) return;
  try {
    await post("/api/retired", { item: selected.id, retired, reason: retired ? $("retire-reason").value : "" });
    await reload();
    say($("retire-message"), retired ? "Retired." : "Back in use.");
  } catch (err) {
    say($("retire-message"), err.message, true);
  }
}
$("retire-form").onsubmit = (event) => { event.preventDefault(); setRetired(true); };
$("unretire").onclick = () => setRetired(false);

async function setFrequent(item, frequent) {
  try {
    await post("/api/frequent", { item: item.id, frequent });
    await reload();
  } catch (err) {
    say($("list-message"), err.message, true);
  }
}
$("frequent").onclick = () => {
  const item = selected && state.items.find((i) => i.id === selected.id);
  if (item) setFrequent(item, !item.frequent);
};

// Only frequent Items, when the chip is pressed; remembered with the view.
$("filter-frequent").onclick = () => {
  const on = $("filter-frequent").getAttribute("aria-pressed") !== "true";
  $("filter-frequent").setAttribute("aria-pressed", String(on));
  saveView();
  render();
};
const onlyFrequent = () => $("filter-frequent").getAttribute("aria-pressed") === "true";

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
  const n = item.revisions.findIndex((r) => (r.id || r.digest) === digest) + 1;
  const consequence = "Its data is kept in the tree's trash. A PDF still used by another snapshot stays in place.";
  if (!confirm(`Delete revision ${n} of ${label(state, item)}?\n\n${consequence}`)) return;
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
    const saved = await post("/api/fields", { item: selected.id, digest: selected.digest, fields: fieldsOf($("edit-fields")) });
    await reload();
    pick(saved.id, headOf(saved));
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

$("revision-tags-form").onsubmit = async (event) => {
  event.preventDefault();
  revisionTags.commit();
  try {
    await post("/api/revision-tags", { item: selected.id, digest: selected.digest, tags: revisionTags.get() });
    await reload();
    say($("revision-tags-message"), "Saved.");
  } catch (err) {
    say($("revision-tags-message"), err.message, true);
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
      onclick: () => { const it = byId[h.id]; pick(it.id, headOf(it)); },
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
  if (wanted) pick(wanted.id, headOf(wanted));
}).catch((err) => {
  state.error = err.message;
  render();
});

$("attach-pdf").onclick = async () => {
  if (!selected) return;
  const target = { ...selected };
  const count = state.items.find((i) => i.id === target.id).revisions.length;
  const chosen = await openFile({ title: "Save PDF as a new revision", folder: state.root,
    filters: [{ label: "PDF", extensions: [".pdf"] }] });
  if (!chosen) return;
  const path = Array.isArray(chosen) ? chosen[0] : chosen;
  const slash = path.lastIndexOf("/");
  try {
    const saved = await post("/api/attach", { item: target.id, digest: target.digest, dir: path.slice(0, slash) || "/", path: path.slice(slash + 1) });
    state = await loadState(); selected = null; pick(saved.id, headOf(saved));
    say($("edit-message"), saved.revisions.length > count ? "Saved as a new revision. Earlier snapshots preserved." : "Content already kept; no new revision.");
  } catch (err) { say($("edit-message"), err.message, true); }
};

let supersessionRequest = 0;
let predecessorID = "";
async function drawSupersession(item) {
  const request = ++supersessionRequest;
  const previous = state.items.find((i) => i.superseded_by === item.id);
  const next = state.items.find((i) => i.id === item.superseded_by);
  predecessorID = item.superseded_by ? item.id : previous?.id || "";
  $("supersession-links").replaceChildren();
  for (const [title, linked, id] of [["Superseded by", next, item.superseded_by], ["Replaces", previous, previous?.id]]) {
    if (id) $("supersession-links").append(el("p", {}, title + ": ", el("a", { href: "#" + id, onclick: (event) => { if (linked) { event.preventDefault(); open(linked); } } }, linked ? label(state, linked) : id + " (not in this tree)")));
  }
  $("undo-supersession").hidden = !predecessorID;
  $("retire-form").hidden = !!item.superseded_by;
  $("supersession-form").hidden = true;
  if (item.type !== "visa" || item.retired || previous) return;
  try {
    const response = await fetch(api("/api/supersession-candidates?" + new URLSearchParams({owner:item.fields.owner || "",country:item.fields.country || "",exclude:item.id})));
    const candidates = await response.json();
    if (request !== supersessionRequest || selected?.id !== item.id) return;
    if (!response.ok) throw new Error(candidates.error);
    $("supersession-choice").replaceChildren(el("option", {value:""}, "Choose an existing visa…"), ...candidates.map((i) => el("option", {value:i.id}, [i.fields.visa_type, i.fields.number || i.id, i.fields.issued].filter(Boolean).join(" · "))));
    $("supersession-form").hidden = !candidates.length;
  } catch (err) { if (request === supersessionRequest) say($("retire-message"),err.message,true); }
}
$("supersession-form").onsubmit = async (event) => {
  event.preventDefault();
  if (!selected || !$("supersession-choice").value) return;
  try { await post("/api/supersession", {item:$("supersession-choice").value,replacement:selected.id}); await reload(); }
  catch (err) { say($("retire-message"),err.message,true); }
};
$("undo-supersession").onclick = async () => {
  if (!predecessorID) return;
  try { await post("/api/supersession", {item:predecessorID}); await reload(); }
  catch (err) { say($("retire-message"),err.message,true); }
};
