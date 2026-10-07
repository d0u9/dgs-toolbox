// Browse: the Items kept in the tree, their fields, revisions and HEAD.
import { openFile } from "/ui/filedialog.js";
import { splitter } from "/ui/splitter.js";
import { openMenu } from "/ui/menu.js";
import { suggest } from "/ui/combo.js";
import { tooltip } from "/ui/tooltip.js";
import { listTable } from "/ui/listtable.js";
import { $, address, keep, scrollBack, tagPills, papersOf, api, el, loadState, post, templateOf, label, fieldInputs, fieldsOf, fieldsAt, currentFields, tagUses, tagsAt, revisionName, frame, say, showText, showPreview, clearPreview, eventLines, reveal, originalsList } from "/common.js";
import { icon, iconSvg } from "/ui/icons.js";

let state = { templates: [], items: [] };
let selected = null; // {id, digest}
// What the page showed when it was left — the filters and the revision
// picked — until it is drawn so again on coming Back.
let back = history.state || null;
let editing = "";
let notesOf = "";
let tagsOf = "";
const detailTags = window.tagField($("detail-tags"), { known: () => tagUses(state.items), placeholder: "Add tags" });
const revisionTags = window.tagField($("revision-tags"), { known: () => tagUses(state.items), placeholder: "Add tags" });
let revisionTagsOf = "";
// Items whose text matched the filter, by ID, with the line it matched on.
let textHits = new Map();
let unread = 0;

const words = () => $("filter").value.trim().toLowerCase().split(/\s+/).filter(Boolean);
const matches = (text) => words().every((w) => text.toLowerCase().includes(w));
const VIEW_KEY = "dgs-doc-browse-view";

// keysOf is the keys that tell a type's documents apart — owner, country —
// which are what a person looks for first.
const keysOf = (type) => (templateOf(state, type)?.fields || []).filter((f) => f.distinguishing).map((f) => f.key);
const fieldOf = (key) => {
  for (const t of state.templates) { const f = t.fields.find((x) => x.key === key); if (f) return f; }
  return null;
};

// tableKeys is the field columns of the list for the Items given, and what
// Sort offers besides its own: the fields every Template requires, then,
// when the Items are all of one type, that type's distinguishing keys in its
// order. A key none of them has a value for is left out.
function tableKeys(items) {
  const types = new Set(items.map((i) => i.type));
  const keys = [...new Set([...(state.mandatory || []), ...(types.size === 1 ? keysOf([...types][0]) : [])])];
  return keys.filter((k) => items.some((i) => currentFields(i)[k]));
}

// The filters are steps taken one after another, as a folder is walked into:
// each is a key and the value chosen for it, and narrows the Items the one
// before it left. The keys are any field the Items left hold, besides type,
// kind, expiry, use and tags; a field linking to another Item is offered by
// that Item's name. Each step is a chip beside the search, so the filters
// stay one row however many there are.
let steps = (back && back.steps) || []; // [{ key, value }]

const EXPIRY = { expired: "expired", soon: "expiring soon", valid: "valid", permanent: "no end date", none: "no expiry field" };
const USE = { active: "in use", retired: "retired" };
const KEY_NAMES = { type: "Type", kind: "Kind", expiry: "Expiry", use: "Use", tags: "Tags" };
const keyName = (key) => KEY_NAMES[key] || (fieldOf(key)?.type === "date" || fieldOf(key)?.type === "month" ? key + " year" : key);

// valuesOf is what an Item holds for a filter key: one value, or for tags
// and an owner it shares with, several. A date or month is held by its year.
function valuesOf(i, key) {
  if (key === "type") return [i.type];
  if (key === "kind") return [i.kind];
  if (key === "expiry") return [expiryOf(i).state];
  if (key === "use") return [i.retired ? "retired" : "active"];
  if (key === "tags") return tagsAt(i, headOf(i));
  const v = currentFields(i)[key];
  if (v === undefined || v === null || v === "") return key === "owner" ? i.shared_with || [] : [];
  const f = fieldOf(key);
  const one = f && (f.type === "date" || f.type === "month") ? String(v).slice(0, 4) : String(v);
  // Filtering by owner shows an Item shared with that person too.
  return key === "owner" ? [one, ...(i.shared_with || [])] : [one];
}

// valueName is how a value is shown: an Item a field points at by its name.
function valueName(key, value) {
  if (key === "expiry") return value === "soon" && state.soonDays ? "expires within " + state.soonDays + " days" : EXPIRY[value] || value;
  if (key === "use") return USE[value] || value;
  return labelOf(key, value);
}

// labelOf is how a value is named in a list: an Item a field points at by
// its name, any other value as kept.
function labelOf(key, value) {
  const f = fieldOf(key);
  const other = f && f.type === "item" && state.items.find((i) => i.id === value);
  return other ? label(state, other) : value;
}

// passes says whether an Item meets the search, Frequent and the first n
// steps, every step when n is left out.
function passes(i, n = steps.length) {
  const fields = currentFields(i);
  return (!onlyFrequent() || i.frequent) &&
    steps.slice(0, n).every((s) => valuesOf(i, s.key).includes(s.value)) &&
    // A linked field matches by the linked Item's name too, so a plate finds
    // every document about that car.
    (textHits.has(i.id) || matches(label(state, i) + " " + Object.entries(fields).map(([k, v]) => v + " " + shown(i, k, v)).join(" ") + " " + tagsAt(i, headOf(i)).join(" ")));
}

// facets is what the Items given can still be told apart by: each key with
// its values and how many Items hold each. A key every Item holds the same
// one value of tells nothing and is left out, as is a value already chosen.
// The keys most Items hold come first, the finer before the coarser; those
// that group nothing last.
function facets(items) {
  const keys = new Map(); // key → Map(value → count)
  const add = (key, value) => {
    if (steps.some((s) => s.key === key && s.value === value)) return;
    if (!keys.has(key)) keys.set(key, new Map());
    const m = keys.get(key);
    m.set(value, (m.get(value) || 0) + 1);
  };
  for (const i of items) {
    const ks = new Set(["type", "kind", "expiry", "use", "tags", ...Object.keys(currentFields(i))]);
    if ((i.shared_with || []).length) ks.add("owner");
    for (const k of ks) for (const v of new Set(valuesOf(i, k))) add(k, v);
  }
  const out = [];
  for (const [key, counts] of keys) {
    if (counts.size === 1 && [...counts.values()][0] === items.length) continue;
    // held: how many Items have the key; alike: how many share a value on
    // average. A key nearly every value of is held by one Item alone —
    // a number, an amount — groups nothing, so it goes last.
    const held = items.filter((i) => valuesOf(i, key).length).length;
    const alike = [...counts.values()].reduce((a, b) => a + b, 0) / counts.size;
    out.push({ key, counts, held, groups: alike >= 1.5 });
  }
  return out.sort((a, b) => b.groups - a.groups || b.held - a.held || b.counts.size - a.counts.size || a.key.localeCompare(b.key));
}

// drawSteps draws the steps as chips: clicking one goes back to that step
// and offers its key again; its × takes only that step away.
function drawSteps() {
  $("filter-steps").replaceChildren(...steps.map((s, n) => {
    // The tag bubble every page draws, from /ui/tags.css: a linked Item by
    // the last part of its name, a plate or an address, the whole on hover.
    const full = keyName(s.key) + ": " + valueName(s.key, s.value);
    const back = () => { steps = steps.slice(0, n); render(); openFacets(s.key); };
    const chip = el("span", { className: "tag-bubble step", tabIndex: 0, role: "button",
      title: full + "\nClick to go back to this step and choose again", onclick: back,
      onkeydown: (event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); back(); } } },
      el("span", {}, el("span", { className: "step-key" }, keyName(s.key) + " "), shortName(s.key, s.value)),
      el("button", { type: "button", className: "tag-remove", title: "Take this filter away", "aria-label": "Take this filter away",
        onclick: (event) => { event.stopPropagation(); steps.splice(n, 1); render(); }, innerHTML: iconSvg("x") }));
    return n ? [el("span", { className: "step-sep", "aria-hidden": "true", innerHTML: iconSvg("chevron-right") }), chip] : [chip];
  }).flat());
}
// shortName is a value as a step names it: a linked Item by the last part
// of its name, which is what tells it apart — a plate, an address.
function shortName(key, value) {
  const name = valueName(key, value);
  const f = fieldOf(key);
  return f && f.type === "item" && name !== value ? name.split(" · ").pop() : name;
}

// The facet dialog: the keys in a table on the left, the values of the one
// picked in a table on the right, each sorted by a click on a column and
// found by the box over it. Choosing a value adds a step and closes it.
let facetKey = "";
let facetRows = []; // the facets of the Items left, as facets answers them
const pct = (n, of) => (of ? Math.round((n / of) * 100) : 0) + "%";
const keyTable = listTable($("facet-keys"), {
  key: "dgs-doc-browse-facet-keys",
  chooser: false,
  columns: [
    { id: "rank", text: "#", width: 40, cell: (f) => el("td", { className: "numeric muted" }, String(f.rank)), value: (f) => f.rank,
      title: "The order suggested: keys most Items hold first, the finer before the coarser, those that group nothing last" },
    { id: "name", text: "Field", width: 150, cell: (f) => keyName(f.key), title: "The field, or type, kind, expiry, use or tags" },
    { id: "held", text: "Items", width: 64, down: true, cell: (f) => el("td", { className: "numeric" }, String(f.held)), value: (f) => f.held, title: "How many of the Items left hold it" },
    { id: "values", text: "Values", width: 64, down: true, cell: (f) => el("td", { className: "numeric" }, String(f.counts.size)), value: (f) => f.counts.size, title: "How many different values they hold" },
  ],
  sort: { id: "rank" },
  row: (f, cells) => el("tr", { className: "facet-key" + (f.key === facetKey ? " selected" : ""),
    onclick: () => { facetKey = f.key; $("facet-value-search").value = ""; drawFacets(); } }, ...cells),
  empty: "No field matches.",
});
const valueTable = listTable($("facet-values"), {
  key: "dgs-doc-browse-facet-values",
  chooser: false,
  columns: [
    { id: "name", text: "Value", width: 320, cell: (v) => el("td", { className: "facet-value", title: v.name }, v.name), value: (v) => v.name },
    { id: "count", text: "Items", width: 64, down: true, cell: (v) => el("td", { className: "numeric" }, String(v.count)), value: (v) => v.count, title: "How many of the Items left hold it" },
    { id: "share", text: "Share", width: 64, down: true, cell: (v) => el("td", { className: "numeric muted" }, pct(v.count, v.of)), value: (v) => v.count },
    { id: "types", text: "Types", width: 200, cell: (v) => el("td", { className: "muted facet-types", title: v.types }, v.types), title: "The types of the Items holding it" },
  ],
  sort: { id: "count", down: true },
  row: (v, cells) => el("tr", { className: "facet-pick", title: "Narrow to " + v.name,
    onclick: () => { steps.push({ key: facetKey, value: v.value }); closeFacets(); render(); } }, ...cells),
  empty: "No value matches.",
});
function openFacets(key = "") {
  facetKey = key;
  $("facet-key-search").value = "";
  $("facet-value-search").value = "";
  if (!$("facets").open) $("facets").showModal();
  drawFacets();
  (key ? $("facet-value-search") : $("facet-key-search")).focus();
}
function closeFacets() { if ($("facets").open) $("facets").close(); }
function drawFacets() {
  if (!$("facets").open) return;
  const pool = state.items.filter((i) => passes(i));
  $("facets-steps").textContent = (steps.length ? steps.map((s) => keyName(s.key) + ": " + valueName(s.key, s.value)).join(" › ") + " · " : "") +
    pool.length + (pool.length === 1 ? " Item" : " Items");
  facetRows = facets(pool).map((f, n) => ({ ...f, rank: n + 1 }));
  $("facet-empty").hidden = facetRows.length > 0;
  // A key is found by its name or by any value it holds.
  const kq = $("facet-key-search").value.trim().toLowerCase();
  const keys = facetRows.filter((f) => !kq || keyName(f.key).toLowerCase().includes(kq) ||
    [...f.counts.keys()].some((v) => valueName(f.key, v).toLowerCase().includes(kq)));
  if (!keys.some((f) => f.key === facetKey)) facetKey = keys[0] ? keys[0].key : "";
  keyTable.draw(keys);
  const picked = facetRows.find((f) => f.key === facetKey);
  $("facet-values-title").textContent = picked ? keyName(picked.key) : "";
  // A key found by one of its values shows those values on the right, until
  // the value box asks for others.
  const keyHit = kq && keyName(facetKey).toLowerCase().includes(kq);
  const vq = ($("facet-value-search").value.trim() || (keyHit ? "" : kq)).toLowerCase();
  const typesOf = (value) => {
    const n = new Map();
    for (const i of pool) if (valuesOf(i, facetKey).includes(value)) n.set(i.type, (n.get(i.type) || 0) + 1);
    return [...n].sort((a, b) => b[1] - a[1]).map(([t, c]) => t + " " + c).join(", ");
  };
  const values = picked ? [...picked.counts].map(([value, count]) => ({ value, count, of: pool.length, name: valueName(facetKey, value) }))
    .filter((v) => !vq || v.name.toLowerCase().includes(vq))
    .map((v) => ({ ...v, types: typesOf(v.value) })) : [];
  valueTable.draw(values);
}
$("filter-add").onclick = () => openFacets();
$("facets-close").onclick = closeFacets;
// A click on the backdrop, outside the dialog's box, closes it.
$("facets").onclick = (event) => {
  const r = $("facets").getBoundingClientRect();
  if (event.target === $("facets") && (event.clientX < r.left || event.clientX > r.right || event.clientY < r.top || event.clientY > r.bottom)) closeFacets();
};
$("facet-key-search").oninput = drawFacets;
$("facet-value-search").oninput = drawFacets;
// Enter takes the first row: the first key on the left, the first value on
// the right.
$("facet-key-search").onkeydown = (event) => { if (event.key === "Enter") $("facet-value-search").focus(); };
$("facet-value-search").onkeydown = (event) => { if (event.key === "Enter") $("facet-values").querySelector("tbody tr.facet-pick")?.click(); };

// drawSort lets the list sort by any of its columns.
function drawSort(pool) {
  const sortSelect = $("view-sort"), wantSort = sortSelect.dataset.want || sortSelect.value;
  for (const o of [...sortSelect.options]) if (o.dataset.column) o.remove();
  // Every field the Items hold, so a column chosen in the list sorts too.
  const columns = [...new Set([...tableKeys(pool), ...pool.flatMap((i) => Object.keys(currentFields(i))).sort()])];
  if (wantSort.startsWith("field:") && !columns.includes(wantSort.slice(6))) columns.push(wantSort.slice(6));
  for (const [value, text] of [...columns.map((k) => ["field:" + k, k]), ["tags", "tags"], ["revisions", "revisions"]]) {
    const o = el("option", { value }, text);
    o.dataset.column = "1";
    sortSelect.append(o);
  }
  sortSelect.value = wantSort;
  if (sortSelect.value !== wantSort) sortSelect.value = "added";
  delete sortSelect.dataset.want;
}

// The order the Items are sorted in, turned round by the button beside Sort.
const direction = () => $("view-direction").dataset.dir === "asc" ? "asc" : "desc";
function setDirection(dir) {
  const b = $("view-direction");
  b.dataset.dir = dir === "asc" ? "asc" : "desc";
  b.innerHTML = iconSvg(b.dataset.dir === "asc" ? "arrow-up" : "arrow-down");
  b.title = b.dataset.dir === "asc" ? "Ascending: click for descending" : "Descending: click for ascending";
  b.setAttribute("aria-label", b.title);
}

function shownItems() {
  const items = state.items.filter((i) => passes(i));
  const sort = $("view-sort").value, dir = direction() === "asc" ? 1 : -1;
  const added = (i) => (i.revisions[i.revisions.length - 1] || {}).added || "";
  // No expiry sorts after any date, whichever way round.
  const exp_ = (i) => { const e = expiryOf(i); return e.date || (e.state === "permanent" ? "9999" : ""); };
  const keyOf = sort.startsWith("field:") ? (i) => shown(i, sort.slice(6), currentFields(i)[sort.slice(6)] || "")
    : { added, expiry: exp_, name: (i) => label(state, i), type: (i) => i.type + " " + label(state, i),
      tags: (i) => tagsAt(i, headOf(i)).join(" "),
      revisions: (i) => papersOf(i).length }[sort] || added;
  // Retired Items go last and frequent ones first, whatever the sort; the
  // sort orders each part.
  return items.sort((x, y) => {
    if (!!x.retired !== !!y.retired) return x.retired ? 1 : -1;
    if (!!x.frequent !== !!y.frequent) return x.frequent ? -1 : 1;
    const a = keyOf(x), b = keyOf(y);
    // No value sorts after any value, whichever way round.
    if ((sort === "expiry" || sort === "tags" || sort.startsWith("field:")) && (!a || !b)) return !a && !b ? 0 : !a ? 1 : -1;
    if (typeof a === "string") return a.localeCompare(b, undefined, { numeric: true }) * dir;
    return a < b ? -dir : a > b ? dir : 0;
  });
}

const expiryOf = (item) => (state.expiry || {})[item.id] || { state: "none" };

// sharedBadge marks an Item shared with someone besides its owner.
const sharedBadge = (item) => (item.shared_with || []).length
  ? el("span", { className: "badge badge-shared", title: "Shared by " + (currentFields(item).owner || "its owner") + " with " + item.shared_with.join(", ") }, "shared") : null;

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

const fileOf=(item,ref)=>item?.revisions.find(r=>(r.id||r.digest)===ref);
const hasFile=(item,ref)=>!!fileOf(item,ref)?.digest;
const hasPDF = (item, ref) => hasFile(item,ref)&&(/\.pdf$/i.test(fileOf(item,ref)?.path||"file.pdf"));
const headOf = (item) => item.head || ((item.revisions[item.revisions.length - 1] || {}).id || (item.revisions[item.revisions.length - 1] || {}).digest);
const thumbURL = (item) => { const d = headOf(item); return api(`/${hasPDF(item,d)?"api/page":"api/thumbnail"}?item=${encodeURIComponent(item.id)}&digest=${d}&n=1&size=thumb&v=${d}`); };

// thumb is the first page of HEAD, or the type's name where the PDF has no
// picture of its page (one made on a computer).
// noPicture remembers the pages that have no picture, so a redraw does not
// ask for them again.
const noPicture = new Set();
function thumb(item) {
  const wrap = el("div", { className: "card-thumb-wrap" });
  const url = thumbURL(item);
  const none = () => { noPicture.add(url); wrap.classList.add("no-picture"); wrap.dataset.type = item.type; };
  if (!hasFile(item, headOf(item))) {
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
        papersOf(item).length > 1 ? el("span", { title: "Revisions" }, papersOf(item).length + " revisions") : null),
      el("p", { className: "card-details", title: details(item) }, details(item)),
      textHits.get(item.id) ? el("p", { className: "card-snippet" }, textHits.get(item.id)) : null,
      el("p", { className: "card-badges" }, sharedBadge(item), retiredBadge(item), expiryBadge(item))));
  if (selected && selected.id === item.id) li.classList.add("card-selected");
  return li;
}

// star marks an Item frequent, or unmarks it, from its card or row.
function star(item) {
  const on = !!item.frequent;
  const button = el("button", { type: "button", className: "star" + (on ? " on" : ""), innerHTML: iconSvg("star"),
    title: on ? "Frequent: click to unmark" : "Mark frequent", onclick: (event) => { event.stopPropagation(); setFrequent(item, !on); },
    ondblclick: (event) => event.stopPropagation() });
  button.setAttribute("aria-pressed", String(on));
  button.setAttribute("aria-label", "Frequent");
  return button;
}

// The list is the shared list table: a column's right edge drags it wider
// or narrower, remembered per column, and a click on a header sorts by it,
// again to turn the order round. Sort and its direction stay the page's,
// since the cards are in the same order.
// tableColumns is the list's columns for the Items shown, in order: the
// thumbnail and star, the Item's name as its card carries it, the type when more than
// one is shown, the field columns tableKeys gives, Expiry when any of them
// has something to say there, when each was added, and its revisions,. Each
// says how its cell is drawn.
// tableColumns is every column the list may show. Item, the star and the
// thumbnail are always there; the rest are chosen from the header's menu
// (right click, or its +): type, any field the Items hold, tags, expiry,
// added and revisions.
const SHOWN = ["type", "field:owner", "field:country", "tags", "expiry", "added", "revisions"];
function tableColumns(items) {
  const capital = (k) => k[0].toUpperCase() + k.slice(1);
  const added = (item) => new Date((item.revisions[item.revisions.length - 1] || {}).added || 0).toLocaleDateString();
  const fields = [...new Set([...(state.mandatory || []), ...items.flatMap((i) => Object.keys(currentFields(i)))])]
    .sort((a, b) => a.localeCompare(b));
  return [
    { id: "thumb", text: "", fixed: true, width: 48, sortable: false, resizable: false, cell: (item) => el("td", { className: "table-thumb" }, thumb(item)) },
    { id: "frequent", text: "Frequent", icon: "star", tip: "Frequent", fixed: true, width: 36, sortable: false, resizable: false, cell: (item) => el("td", { className: "table-star" }, star(item)) },
    // The card's name without its type, which the Type column, or the type
    // filter, already says.
    { id: "name", text: "Item", fixed: true, width: 340, cell: (item) => {
      const name = label(state, item).slice(item.type.length).replace(/^ · /, "") || item.type;
      return el("td", { className: "table-name", title: label(state, item) }, name);
    } },
    { id: "type", text: "Type", width: 130, cell: (item) => el("td", { className: "mono" }, item.type) },
    ...fields.map((k) => ({ id: "field:" + k, text: capital(k), about: fieldOf(k)?.description, width: k === "country" ? 90 : 130, group: "Fields",
      cell: (item) => { const v = currentFields(item)[k]; return el("td", {}, v ? shown(item, k, v) : ""); } })),
    // Tags as the shared pills from /ui/tags.css.
    { id: "tags", text: "Tags", width: 160, value: (item) => tagsAt(item, headOf(item)).join(" "),
      cell: (item) => el("td", { className: "table-tags" }, tagPills(tagsAt(item, headOf(item)))) },
    { id: "expiry", text: "Expiry", width: 150,
      cell: (item) => el("td", {}, sharedBadge(item), retiredBadge(item), expiryBadge(item) || (item.retired ? null : el("span", { className: "muted" }, "—"))) },
    { id: "added", text: "Added", down: true, width: 100, cell: (item) => el("td", { className: "numeric" }, added(item)) },
    { id: "revisions", text: "Rev.", tip: "Revisions", down: true, width: 56, cell: (item) => el("td", { className: "numeric" }, String(papersOf(item).length)) },
  ];
}
let shownColumns = [];
const itemList = listTable($("table"), {
  key: "dgs-doc-browse-columns",
  className: "items-table",
  columns: () => shownColumns,
  shown: SHOWN,
  sorting: {
    get: () => ({ id: $("view-sort").value, down: direction() === "desc" }),
    set: ({ id, down }) => { $("view-sort").value = id; setDirection(down ? "desc" : "asc"); saveView(); render(); },
  },
  head: (th, c) => { if (c.tip || c.about) tooltip(th, [c.tip || c.text, c.about]); },
  row: (item, cells) => row(item, cells),
  empty: "",
});

function row(item, cells) {
  const e = expiryOf(item);
  const tr = el("tr", { className: "table-row state-" + e.state + (item.retired ? " retired" : ""), onclick: () => open(item), ondblclick: () => { open(item); openReader(); } },
    ...cells);
  if (selected && selected.id === item.id) tr.classList.add("selected", "focused");
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
  $("detail-frame").hidden = !hasFile(item,digest);
  $("page-prev").hidden = !attached;
  $("page-next").hidden = !attached;
  $("open-reader").hidden = !attached;
  $("reveal-pdf").hidden = !hasFile(item,digest);
  $("detail-image").hidden = !hasFile(item,digest);
  $("open-reader").disabled = !attached;
  if (!attached) {
    frame.classList.add("no-picture"); frame.dataset.type = hasFile(item,digest)?item.type:"No PDF";
    if(hasFile(item,digest))$("detail-image").src=api("/api/thumbnail?"+new URLSearchParams({item:id,digest}));
    $("page-label").textContent = hasFile(item,digest)?"This file has no PDF pages; show it in Finder.":"No attachment";
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
$("reveal-pdf").onclick = () => reveal({ item: shownPage.id, digest: shownPage.digest });

// drawOriginals lists the images the picked revision's PDF was made from,
// kept beside it, a page each in order; each opens, or shows in Finder.
function drawOriginals(item, rev) {
  const originals = (rev && rev.originals) || [];
  $("originals-group").hidden = originals.length === 0;
  $("originals").replaceChildren(originalsList(item.id, originals));
}

$("detail-frame").onclick = openReader;
$("reader-back").onclick = closeReader;

function render() {
  frame(state);
  $("check-files").hidden = !state.capabilities.file_changes;
  drawSort(state.items.filter((i) => passes(i)));
  const items = shownItems();
  drawSteps();
  drawFacets();
  $("count").textContent = items.length === state.items.length ? items.length + " Items" : items.length + " of " + state.items.length + " Items";
  $("none").hidden = state.items.length > 0 || !state.tree;
  $("nothing").hidden = !state.items.length || items.length > 0;
  $("filters-clear").hidden = !filtering();
  if (!back) {
    keep({ q: $("filter").value, steps });
  }
  $("unread").hidden = !unread;
  $("unread-count").textContent = unread + (unread === 1 ? " Item's text is" : " Items' text is") + " not read yet, so searching cannot find " + (unread === 1 ? "it." : "them.");
  const list = layout === "list";
  $("grid").hidden = list;
  $("table").hidden = !list;
  if (list) {
    shownColumns = tableColumns(items);
    itemList.draw(items);
  } else {
    $("grid").replaceChildren(...items.map(card));
  }
  const item = selected && state.items.find((i) => i.id === selected.id);
  $("side").hidden = !item;
  $("side-splitter").hidden = !item;
  if (item) detail(item);
}

const filtering = () => onlyFrequent() || $("filter").value.trim() || steps.length;

$("filters-clear").onclick = () => {
  $("filter").value = "";
  $("filter-frequent").setAttribute("aria-pressed", "false");
  saveView();
  steps = [];
  textHits = new Map();
  render();
  searchText();
};
function saveView() {
  try {
    localStorage.setItem(VIEW_KEY, JSON.stringify({ layout, sort: $("view-sort").value,
      direction: direction(), frequent: onlyFrequent() }));
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
$("view-sort").addEventListener("change", () => {
  saveView();
  render();
});
$("view-direction").onclick = () => {
  setDirection(direction() === "asc" ? "desc" : "asc");
  saveView();
  render();
};
setDirection("desc");
try {
  const v = JSON.parse(localStorage.getItem(VIEW_KEY) || "{}");
  if (v.frequent) $("filter-frequent").setAttribute("aria-pressed", "true");
  if (v.layout) setLayout(v.layout);
  if (v.sort) { $("view-sort").value = v.sort; $("view-sort").dataset.want = v.sort; }
  if (v.direction) setDirection(v.direction);
} catch { /* the defaults stand */ }

// Closing the detail gives the cards the width back.
function close() {
  selected = null;
  address(location.pathname + location.search);
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
  if (f && f.type === "revision") {
    const [id, ref] = (value || "").split("@");
    const other = state.items.find((i) => i.id === id);
    const r = other && other.revisions.find((x) => (x.id || x.digest) === ref);
    return other ? label(state, other) + " @ " + (r ? revisionName(r, false) : ref) : value;
  }
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
  keep({ digest });
  address("#" + id);
  render();
}

function detail(item) {
  $("detail-head").textContent = label(state, item);
  $("change-type").href = api("/change-type/?item=" + encodeURIComponent(item.id));
  drawHistory(item);
  drawRetired(item);
  drawShared(item);
  $("history-all").href = api("/log/") + "#" + encodeURIComponent(item.id);
  $("locked").hidden = !state.capabilities.file_changes;
  $("locked").setAttribute("aria-pressed", String(!!item.locked));
  $("locked").classList.toggle("on", !!item.locked);
  $("frequent").setAttribute("aria-pressed", String(!!item.frequent));
  $("frequent").classList.toggle("on", !!item.frequent);
  $("delete-note").textContent = state.capabilities.shared_files ? "Moves this sidecar to trash; files stay while another Item references them." : "Moves this Item, with every revision’s PDF, into the tree’s trash folder.";
  $("delete").disabled = false;
  $("attach-pdf").hidden = false;
  $("attach-pdf").disabled = !state.capabilities.attachment_changes;
  $("attach-pdf").title = !state.capabilities.attachment_changes ? "Attachment changes are not supported in in-place trees yet" : "Save the PDF with these fields: a correction, or a new revision when that is ticked";
  $("attach-pdf").textContent = hasPDF(item, selected.digest) ? "Replace PDF…" : "Attach PDF…";
  $("new-without-pdf").hidden = !state.capabilities.without_file || item.kind !== "document";
  $("new-without-pdf").href = api("/import/?no_pdf=1&into=" + encodeURIComponent(item.id));
  const papers = papersOf(item);
  const paper = papers.findIndex((p) => p.some((r) => (r.id || r.digest) === selected.digest));
  const correction = paper >= 0 && (papers[paper][0].id || papers[paper][0].digest) !== selected.digest;
  $("detail-rev").textContent = (item.kind === "document" ? "Revision " + (paper + 1) + " of " + papers.length : "Record")
    + (correction ? " · correction" : "") + (selected.digest === item.head ? " · HEAD" : "");
  // New revision is the owner's word, asked each time; a record has none.
  $("new-revision-field").hidden = item.kind !== "document";
  const chosen = item.revisions.find((r) => (r.id || r.digest) === selected.digest);
  if (chosen?.lost) $("detail-rev").textContent += " · file lost";
  // Saving an older revision's fields branches from it and moves HEAD there.
  const branching = selected.digest !== headOf(item);
  $("save-button").textContent = branching ? "Save as new HEAD" : "Save";
  $("save-button").title = branching ? "Saves a snapshot from this one's values and makes it HEAD; the others are kept" : "";
  const t = templateOf(state, chosen?.type || item.type);
  // The fields as the picked revision has them: its per_revision values are
  // its own, so picking the old card shows the old card's number.
  const fields = fieldsAt(item, selected.digest);
  if (editing !== item.id + selected.digest + JSON.stringify(fields)) {
    editing = item.id + selected.digest + JSON.stringify(fields);
    say($("edit-message"), "");
    $("edit-fields").replaceChildren(
      ...fieldInputs(t ? t.fields : Object.keys(fields).map((key) => ({ key })), fields, null, state, item.id, t?.type || item.type));
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
  drawOriginals(item, rev);
  $("revision-tags-form").hidden = !rev;
  if (rev && revisionTagsOf !== item.id + (rev.id || rev.digest) + JSON.stringify(rev.tags || [])) {
    revisionTagsOf = item.id + (rev.id || rev.digest) + JSON.stringify(rev.tags || []);
    revisionTags.set(rev.tags || []);
    const n = item.revisions.indexOf(rev) + 1;
    $("revision-tags-label").textContent = "Revision " + n + "'s own" + ((rev.id || rev.digest) === item.head ? " (HEAD)" : "");
    say($("revision-tags-message"), "");
  }
  // Newest revision first, each its corrections above the snapshot that
  // began it.
  const rows = papers.flatMap((p, n) => p.map((r, k) => ({ r, n, first: k === 0 }))).reverse();
  $("revisions").replaceChildren(...rows.map(({ r, n, first }) => {
    const isHead = (r.id || r.digest) === item.head;
    const revisionMenu = (event) => openMenu(event, [
      !isHead ? [{ label: "Make HEAD", onSelect: () => makeHead(item.id, (r.id || r.digest)) }] : [],
      item.kind !== "document" ? [] : first
        ? (n > 0 ? [{ label: "Make correction", title: "Make this revision a correction of the one before", onSelect: () => changePaper("correct", item.id, (r.id || r.digest)) }] : [])
        : [{ label: "Make new revision", title: "This is a new paper: begin a revision with it", onSelect: () => changePaper("begin", item.id, (r.id || r.digest)) }],
      r.digest ? [{ label: "Show PDF in Finder", onSelect: () => reveal({ item: item.id, digest: r.digest }) },
        ...(r.originals || []).map((o, n) => ({ label: "Show original " + (n + 1) + " in Finder", onSelect: () => reveal({ item: item.id, original: o.digest }) }))] : [],
      [{ label: "Delete revision…", danger: true, disabled: item.revisions.length < 2,
        title: item.revisions.length < 2 ? "Delete the Item to remove its last revision" : (r.digest ? "Move this revision's PDF to trash" : "Save this revision's metadata in trash"),
        onSelect: () => deleteRevision(item.id, (r.id || r.digest)) }],
    ]);
    const li = el("li", { className: "rev" + (first ? "" : " correction"), onclick: () => pick(item.id, (r.id || r.digest)),
      oncontextmenu: revisionMenu },
      el("div", { className: "rev-text" },
        el("span", {}, el("strong", {}, first ? "Revision " + (n + 1) : "Correction"), " · " + new Date(r.added).toLocaleDateString(), isHead ? el("span", { className: "tag" }, "HEAD") : null),
        el("span", { className: "sub", title: (r.id || r.digest) }, r.digest ? (r.source ? r.source + " · " : "") + r.digest.slice(0, 8) : "No PDF"),
        (r.tags || []).length ? Object.assign(tagPills(r.tags, true), { className: "tag-pills wrap rev-tags" }) : null),
      !isHead ? el("button", {
        className: "button secondary small-button", type: "button", textContent: "Make HEAD",
        onclick: (event) => { event.stopPropagation(); makeHead(item.id, (r.id || r.digest)); },
      }) : null,
      el("button", { className: "tool", type: "button", innerHTML: iconSvg("ellipsis"), title: "Revision actions", "aria-label": "Revision actions",
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
    await post("/api/items/meta", { item: selected.id, retired, reason: retired ? $("retire-reason").value : "" });
    await reload();
    say($("retire-message"), retired ? "Retired." : "Back in use.");
  } catch (err) {
    say($("retire-message"), err.message, true);
  }
}
$("retire-form").onsubmit = (event) => { event.preventDefault(); setRetired(true); };
$("unretire").onclick = () => setRetired(false);

// The Shared with section: the people besides the owner an Item belongs
// to, from the owner field's options.
let sharedOf = "";
function drawShared(item) {
  const key = item.id + "\n" + (item.shared_with || []).join(",") + "\n" + (currentFields(item).owner || "");
  if (sharedOf === key) return;
  sharedOf = key;
  const owner = currentFields(item).owner || "";
  const field = (templateOf(state, item.type)?.fields || []).find((f) => f.key === "owner");
  const people = [...new Set([...(field?.options || []), ...(item.shared_with || [])])].filter((p) => p !== owner);
  $("shared-people").replaceChildren(...(people.length ? people.map((p) => el("label", { className: "pick" },
    el("input", { type: "checkbox", value: p, checked: (item.shared_with || []).includes(p), onchange: saveShared }), " ", p)) :
    [el("span", { className: "muted" }, "No one else is an owner option in this Item's Template.")]));
  say($("shared-message"), "");
}
async function saveShared() {
  if (!selected) return;
  const people = [...$("shared-people").querySelectorAll("input:checked")].map((i) => i.value);
  try {
    await post("/api/items/meta", { item: selected.id, shared_with: people });
    await reload();
    say($("shared-message"), people.length ? "Shared with " + people.join(", ") + "." : "Not shared.");
  } catch (err) {
    sharedOf = "";
    say($("shared-message"), err.message, true);
  }
}

async function setFrequent(item, frequent) {
  try {
    await post("/api/items/meta", { item: item.id, frequent });
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

// changePaper says, after the fact, whether a snapshot began a revision.
async function changePaper(how, id, digest) {
  say($("head-message"), "");
  try {
    await post("/api/revisions/" + how, { item: id, digest });
    await reload();
    pick(id, digest);
    say($("head-message"), how === "begin" ? "Now a revision of its own." : "Now a correction of the revision before.");
  } catch (err) { say($("head-message"), err.message, true); }
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
    const saved = await post("/api/fields", { item: selected.id, digest: selected.digest, fields: fieldsOf($("edit-fields")), new_revision: $("new-revision").checked });
    $("new-revision").checked = false;
    await reload();
    pick(saved.item.id, headOf(saved.item));
    say($("edit-message"), "Saved.");
  } catch (err) {
    say($("edit-message"), err.message, true);
  }
};

// The Cases an Item is in, and putting it in an open one.
let cases = [];
async function loadCases() {
  try {
    cases = (await (await fetch(api("/api/defs/cases"))).json()).cases || [];
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
    title: c.status === "archived" ? "Archived" : "Open" }, c.status === "archived" ? icon("archive") : "", c.title || c.name))
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
    await post("/api/defs/cases", { op: "add", name, item: selected.id, note: $("to-case-note").value });
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
    await post("/api/items/meta", { item: selected.id, notes: $("notes").value });
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
    await post("/api/items/meta", { item: selected.id, digest: selected.digest, revision_tags: revisionTags.get() });
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
    await post("/api/items/meta", { item: selected.id, tags: detailTags.get() });
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
    address(location.pathname + location.search);
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

// Coming Back, the filters are set as they were before anything is drawn.
if (back) {
  $("filter").value = back.q || "";
}
const unscroll = scrollBack(["items-pane"]);

async function reload() {
  [state] = await Promise.all([loadState(), loadCases()]);
  render();
}

reload().then(() => {
  const was = back;
  back = null;
  searchText();
  const wanted = state.items.find((i) => i.id === location.hash.slice(1));
  if (wanted) {
    const requested=new URLSearchParams(location.search).get("digest");
    pick(wanted.id,requested&&wanted.revisions.some(r=>(r.id||r.digest)===requested)?requested:was&&wanted.revisions.some(r=>(r.id||r.digest)===was.digest)?was.digest:headOf(wanted));
  }
  else render();
  // ?read opens the picked Item straight in the reader, as a link from
  // another page asks.
  if (wanted && new URLSearchParams(location.search).has("read")) openReader();
  unscroll();
}).catch((err) => {
  state.error = err.message;
  render();
});

$("attach-pdf").onclick = async () => {
  if (!selected) return;
  const target = { ...selected };
  const count = state.items.find((i) => i.id === target.id).revisions.length;
  const chosen = await openFile({ title: $("new-revision").checked ? "Save PDF as a new revision" : "Save PDF as a correction", folder: state.root,
    filters: [{ label: "PDF", extensions: [".pdf"] }] });
  if (!chosen) return;
  const path = Array.isArray(chosen) ? chosen[0] : chosen;
  const slash = path.lastIndexOf("/");
  try {
    const fresh = $("new-revision").checked;
    const saved = await post("/api/attach", { item: target.id, digest: target.digest, dir: path.slice(0, slash) || "/", path: path.slice(slash + 1), new_revision: fresh });
    $("new-revision").checked = false;
    state = await loadState(); selected = null; pick(saved.item.id, headOf(saved.item));
    say($("edit-message"), saved.item.revisions.length === count ? "Content already kept; nothing new saved." : fresh ? "Saved as a new revision. Earlier ones preserved." : "Saved as a correction. Earlier snapshots preserved.");
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

$("locked").onclick = async () => {
  const item = selected && state.items.find((i) => i.id === selected.id);
  if (!item) return;
  try { await post("/api/items/meta", { item: item.id, locked: !item.locked }); await reload(); }
  catch (e) { say($("edit-message"), e.message, true); }
};

$("file-check-close").onclick = () => $("file-check-report").close();
$("check-files").onclick = async () => {
  $("check-files").disabled = true;
  $("file-check-report").showModal();
  $("file-problems").replaceChildren();
  say($("file-check-summary"), "Checking recorded files… Unlocked changes will be recorded as new revisions.");
  try {
    const result = await post("/api/files/check", {});
    await reload();
    const problems = result.report.Problems || [];
    say($("file-check-summary"), (result.recorded?.length || 0) + " revisions recorded; " + problems.length + " files or definitions need attention.");
    $("file-problems").replaceChildren(...problems.map((p) => {
      const item = state.items.find((i) => i.id === p.item);
      const row = el("li", {}, el("p", {}, (item ? label(state, item) + " · " : "") + p.Path + ": " + p.Detail));
      for (const path of p.candidates || []) {
        const button = el("button", { className: "button secondary", type: "button", textContent: "Use " + path });
        button.onclick = async () => {
          button.disabled = true;
          try {
            await post("/api/files/confirm", { item: p.item, digest: p.digest, previous: p.Path, path });
            row.replaceChildren(el("p", {}, "Recorded " + path));
            await reload();
          } catch (e) { button.disabled = false; say($("file-check-summary"), e.message, true); }
        };
        row.append(button);
      }
      return row;
    }));
  } catch (e) { say($("file-check-summary"), e.message, true); }
  finally { $("check-files").disabled = false; }
};
