// Values: every value one field holds across the tree's Items, with how
// many Items hold it, so a value written two ways — 中国 and China, NSW
// Fair Trading and NSW FT — can be seen and written as one. Fields are the
// Items' current fields; tags are the Item's and its revisions'. Every
// rewrite goes through the same API an edit on Browse does, so each Item's
// history records it.
import { $, api, el, loadState, post, label, frame, say, currentFields, keep, kept } from "/common.js";
import { listTable } from "/ui/listtable.js";

const TAGS = "tags";
let state = { templates: [], items: [] };
let key = kept("key") || "";
const picked = new Set();
// The field list is sorted by name or by how many values each holds;
// clicking a column's header sorts by it, clicking again turns it round.
let keySort = kept("keySort") || { by: "name", desc: false };
// The values are sorted the same way: by value, numbers as numbers, or by
// how many Items hold them.
let valueSort = kept("valueSort") || { by: "name", desc: false };
const natural = (a, b) => a.localeCompare(b, undefined, { numeric: true });
// sortOf is a list table's sort as one of these: { by, desc }.
const sortOf = (sort, set, draw) => ({
  get: () => ({ id: sort().by, down: sort().desc }),
  set: ({ id, down }) => { set({ by: id, desc: down }); draw(); },
});

// Two values are alike when they differ only in case, spaces or
// punctuation: "NSW Fair Trading" and "nsw fair-trading".
const fold = (v) => v.toLowerCase().normalize("NFKC").replace(/[\s\p{P}\p{S}]+/gu, "");

const typeFilter = () => $("filter-type").value;
const items = () => state.items.filter((i) => !typeFilter() || i.type === typeFilter());

// holdings maps each value of field k to the Items holding it.
function holdings(k, list = items()) {
  const held = new Map();
  for (const item of list) {
    const values = k === TAGS
      ? new Set([...(item.tags || []), ...item.revisions.flatMap((r) => r.tags || [])])
      : new Set([currentFields(item)[k]].filter(Boolean));
    for (const v of values) {
      if (!held.has(v)) held.set(v, []);
      held.get(v).push(item);
    }
  }
  return held;
}

// The fields and their values are shared list tables, sorted here: values
// written alike stay together however the list is sorted.
const keyList = listTable($("keys"), {
  key: "dgs-doc-values-keys",
  chooser: false,
  columns: [
    { id: "name", text: "Field", width: 150, cell: ([k]) => k },
    { id: "count", text: "Values", width: 70, down: true, cell: ([, n]) => el("td", { className: "numeric" }, String(n)) },
  ],
  sorting: sortOf(() => keySort, (s) => { keySort = s; keep({ keySort }); }, () => drawKeys()),
  row: ([k], cells) => el("tr", { className: "values-key" + (k === key ? " selected" : ""),
    onclick: () => { key = k; keep({ key }); picked.clear(); draw(); } }, ...cells),
  empty: "",
});
const valueList = listTable($("values"), {
  key: "dgs-doc-values-values",
  chooser: false,
  columns: [
    { id: "name", text: "Value", width: 260, cell: (r) => el("td", { className: "values-value" },
      el("label", {}, r.box,
        el("span", { className: "values-mark", title: r.similar ? "Written more than one way" : "" }, r.similar ? "≈" : ""),
        el("span", { className: "values-text", title: r.v }, r.v))) },
    { id: "types", text: "Templates", width: 220, sortable: false, cell: (r) => el("td", {}, typesOf(r.holders, r.v)) },
    { id: "count", text: "Items", width: 70, down: true, cell: (r) => {
      const count = el("button", { type: "button", className: "holders-count values-count", title: "The Items holding it", textContent: String(r.holders.length) });
      count.onclick = () => showHolders(r.v, r.holders);
      return el("td", { className: "numeric" }, count);
    } },
  ],
  sorting: sortOf(() => valueSort, (s) => { valueSort = s; keep({ valueSort }); }, () => drawValues()),
  row: (r, cells) => el("tr", { className: "values-row" + (r.similar ? " similar" : "") }, ...cells),
  empty: "",
});

function drawKeys() {
  const counts = new Map();
  for (const item of items()) {
    for (const [k, v] of Object.entries(currentFields(item))) {
      if (!v) continue;
      if (!counts.has(k)) counts.set(k, new Set());
      counts.get(k).add(v);
    }
  }
  const keys = [[TAGS, holdings(TAGS).size], ...[...counts].map(([k, s]) => [k, s.size])];
  const dir = keySort.desc ? -1 : 1;
  keys.sort((a, b) => keySort.by === "count"
    ? dir * (a[1] - b[1]) || a[0].localeCompare(b[0])
    : dir * a[0].localeCompare(b[0]));
  if (!keys.some(([k]) => k === key)) key = keys.length > 1 ? keys[1][0] : TAGS;
  $("key-count").textContent = String(keys.length);
  $("title").textContent = key;
  keyList.draw(keys);
}

function drawValues() {
  const held = holdings(key);
  const groups = new Map();
  for (const v of held.keys()) {
    const f = fold(v) || v;
    if (!groups.has(f)) groups.set(f, []);
    groups.get(f).push(v);
  }
  const search = $("value-search").value.trim().toLowerCase();
  const similarOnly = $("filter-similar").getAttribute("aria-pressed") === "true";
  for (const v of [...picked]) if (!held.has(v)) picked.delete(v);
  const rows = [];
  // Values written alike stay together however the list is sorted: a group
  // sorts by its first value, or by the Items all its values hold.
  const total = (values) => values.reduce((n, v) => n + held.get(v).length, 0);
  const dir = valueSort.desc ? -1 : 1;
  const sorted = [...groups].sort((a, b) => valueSort.by === "count"
    ? dir * (total(a[1]) - total(b[1])) || natural(a[0], b[0])
    : dir * natural(a[0], b[0]));
  for (const [f, values] of sorted) {
    const similar = values.length > 1;
    if (similarOnly && !similar) continue;
    if (search && !values.some((v) => v.toLowerCase().includes(search))) continue;
    values.sort((a, b) => held.get(b).length - held.get(a).length || a.localeCompare(b));
    for (const v of values) {
      const box = el("input", { type: "checkbox", checked: picked.has(v) });
      box.onchange = () => { box.checked ? picked.add(v) : picked.delete(v); drawUnify(held); };
      rows.push({ v, box, similar, holders: held.get(v) });
    }
  }
  valueList.draw(rows);
  $("none").hidden = held.size > 0;
  $("count").textContent = held.size + (held.size === 1 ? " value" : " values");
  $("unify-options").replaceChildren(...[...held.keys()].sort().map((v) => el("option", { value: v })));
  drawUnify(held);
}

function drawUnify(held) {
  const n = new Set([...picked].flatMap((v) => held.get(v) || [])).size;
  $("unify-picked").textContent = picked.size
    ? picked.size + (picked.size === 1 ? " value" : " values") + " · " + n + (n === 1 ? " Item" : " Items")
    : "Tick values to write them as one.";
  $("unify-to").disabled = !picked.size;
  // The most held of the ticked values is the likeliest spelling to keep.
  if (picked.size && !$("unify-to").value) {
    $("unify-to").placeholder = [...picked].sort((a, b) => (held.get(b) || []).length - (held.get(a) || []).length)[0];
  }
  $("unify-go").disabled = !picked.size;
}

// byType groups Items by the Template each was made from, the most first.
function byType(holders) {
  const groups = new Map();
  for (const item of holders) {
    if (!groups.has(item.type)) groups.set(item.type, []);
    groups.get(item.type).push(item);
  }
  return [...groups].sort((a, b) => b[1].length - a[1].length || a[0].localeCompare(b[0]));
}

// typesOf is the row's Templates, each a chip that opens that one's Items.
function typesOf(holders, value) {
  return el("span", { className: "values-types" }, ...byType(holders).map(([type, list]) => {
    const chip = el("button", { type: "button", className: "tag values-type", title: "The " + type + " Items holding it" },
      type, list.length > 1 ? el("span", { className: "values-type-n" }, " ×" + list.length) : null);
    chip.onclick = () => showHolders(value, holders, type);
    return chip;
  }));
}

// files is each revision's file, newest last, as Browse lists them.
const files = (item) => [...new Set((item.revisions || []).map((r) => r.source).filter(Boolean))];

function showHolders(value, holders, only = "") {
  $("holders-title").textContent = key + " = " + value;
  const groups = byType(holders).filter(([type]) => !only || type === only);
  $("holders-list").replaceChildren(...groups.map(([type, list]) => {
    const t = state.templates.find((x) => x.type === type);
    return el("section", { className: "holders-group" },
      el("h4", { className: "holders-value" }, el("code", {}, type), el("span", { className: "holders-count" }, String(list.length)),
        t?.description ? el("span", { className: "holders-desc" }, t.description) : null),
      el("ul", { className: "holders-items" }, ...list.map((item) => el("li", {},
        el("a", { href: api("/browse/?read") + "#" + item.id, target: "_blank", rel: "noopener" },
          el("span", { className: "holders-fields" }, label(state, item)),
          el("span", { className: "holders-files" }, ...files(item).map((f) => el("span", { className: "file-chip" }, f))))))));
  }));
  $("holders-dialog").showModal();
}

async function rewrite(event) {
  event.preventDefault();
  const to = $("unify-to").value.trim() || $("unify-to").placeholder;
  const from = [...picked].filter((v) => v !== to);
  if (!to || !from.length) return;
  const held = holdings(key);
  const targets = [...new Set(from.flatMap((v) => held.get(v) || []))];
  if (!confirm(`Rewrite ${key} ${from.map((v) => "“" + v + "”").join(", ")} as “${to}” on ${targets.length} ${targets.length === 1 ? "Item" : "Items"}?`)) return;
  const message = $("unify-message");
  const failed = [];
  let done = 0;
  for (const item of targets) {
    say(message, `Rewriting ${++done} of ${targets.length}…`);
    try {
      if (key === TAGS) {
        const swap = (tags) => [...new Set((tags || []).map((t) => from.includes(t) ? to : t))];
        if ((item.tags || []).some((t) => from.includes(t))) await post("/api/tags", { item: item.id, tags: swap(item.tags) });
        for (const r of item.revisions) {
          if ((r.tags || []).some((t) => from.includes(t))) await post("/api/revision-tags", { item: item.id, digest: r.id || r.digest, tags: swap(r.tags) });
        }
      } else {
        await post("/api/fields", { item: item.id, digest: "", fields: { ...currentFields(item), [key]: to } });
      }
    } catch (error) {
      failed.push(label(state, item) + ": " + error.message);
    }
  }
  picked.clear();
  $("unify-to").value = "";
  state = await loadState();
  draw();
  say(message, failed.length ? failed.length + " not rewritten — " + failed.join("; ") : `Rewrote ${targets.length} ${targets.length === 1 ? "Item" : "Items"}.`, failed.length > 0);
}

function draw() {
  drawKeys();
  drawValues();
}

$("filter-type").onchange = () => { picked.clear(); draw(); };
$("value-search").oninput = drawValues;
$("filter-similar").onclick = () => {
  const on = $("filter-similar").getAttribute("aria-pressed") !== "true";
  $("filter-similar").setAttribute("aria-pressed", String(on));
  drawValues();
};
$("unify").onsubmit = rewrite;

state = await loadState();
frame(state);
$("filter-type").append(...[...new Set(state.items.map((i) => i.type))].sort().map((t) => el("option", { value: t }, t)));
draw();
