// The part of the Outlines form that makes one rule: types, conditions,
// revisions, a layout built from key chips, and a Numbering list per
// {#}. The page owns the rest of
// its form and what a change redraws; the server computes the result.
import { $, el, currentFields, label, post } from "/common.js";

let state = { templates: [], items: [] };
let keys = [];
let layouts = {}; // each saved rule's layout, parsed by the server
let countries = {}; // each country's every form: its alpha-3 code
let orders = {}; // per numbered name, its order: a name may hold a comma
let skip = []; // the IDs of the Items the rule leaves out
let numbers = {}; // per numbered name, the numbers set by hand: { 结婚证: 6 }
let unnumbered = {}; // per numbered name, the names left unnumbered: [押金]
let changed = () => {};

// setup gives the form the tree's state, the keys a layout can use, the
// country forms, and what to call on every change. Called again when the
// state is reloaded.
export function setup(options) {
  ({ state = state, keys = keys, countries = countries, layouts = layouts } = options);
  if (options.onChange) changed = options.onChange;
}

// fill shows v's query, selection, layout and order.
export function fill(v) {
  const types = (v.query && v.query.type) || [];
  $("types").replaceChildren(...state.templates.map((t) => el("label", {},
    el("input", { type: "checkbox", value: t.type, checked: types.includes(t.type), onchange: narrow }), " " + t.type)));
  $("conditions").replaceChildren(...Object.entries(v.query || {})
    .filter(([k]) => k !== "type").map(([k, values]) => condition(k, values, false, (v.query_types || {})[k])));
  $("exclude").replaceChildren(...Object.entries(v.exclude || {}).map(([k, values]) => condition(k, values, true, (v.exclude_types || {})[k])));
  skip = [...(v.skip || [])];
  drawSkip();
  document.querySelector(`input[name=selection][value=${v.selection || "head"}]`).checked = true;
  if ($("shared")) $("shared").checked = !!v.shared;
  rows = !v.layout ? DEFAULT_ROWS() : layouts[v.name] && v.layout ? fromLayout(layouts[v.name])
    : v.layout.split("/").map((text) => ({ parts: [{ text }] }));
  active = { row: rows.length - 1, group: -1 };
  drawInherit(v.inherit || []);
  orders = Object.fromEntries(Object.entries(v.order || {}).map(([k, list]) => [k, [...list]]));
  numbers = JSON.parse(JSON.stringify(v.numbers || {}));
  unnumbered = JSON.parse(JSON.stringify(v.unnumbered || {}));
  $("map").value = Object.entries(v.map || {}).map(([k, m]) => k + ": " + Object.entries(m).map(([a, b]) => a + " = " + b).join(", ")).join("\n");
  $("map").classList.remove("invalid");
  $("map-error").textContent = "";
  drawOrder();
  drawPath();
}

// read is the form's query, selection, layout and order.
export function read() {
  const query = {};
  const types = [...$("types").querySelectorAll("input:checked")].map((i) => i.value);
  if (types.length) query.type = types;
  // Two rows of one key and operator are one condition.
  // A condition limited to some types keeps them beside it.
  const gather = (rows, into, types) => {
    for (const [key, values, only] of rows) {
      into[key] = [...new Set([...(into[key] || []), ...values])];
      if (only.length) types[key] = [...new Set([...(types[key] || []), ...only])];
    }
    return into;
  };
  const queryTypes = {}, excludeTypes = {};
  gather(read_("conditions"), query, queryTypes);
  const exclude = gather(read_("exclude"), {}, excludeTypes);
  const out = { query, selection: document.querySelector("input[name=selection]:checked").value, layout: layoutText() };
  if (inherited().length) out.inherit = inherited();
  if ($("shared")?.checked) out.shared = true;
  if (Object.keys(exclude).length) out.exclude = exclude;
  if (Object.keys(queryTypes).length) out.query_types = queryTypes;
  if (Object.keys(excludeTypes).length) out.exclude_types = excludeTypes;
  if (skip.length) out.skip = [...skip];
  const order = {};
  for (const key of numberedKeys()) {
    const list = (orders[key] || []).filter(Boolean);
    if (list.length) order[key] = list;
  }
  if (Object.keys(order).length) out.order = order;
  // A number is kept only for a name still in its order.
  const set = {};
  for (const [key, list] of Object.entries(order)) {
    const kept = Object.fromEntries(Object.entries(numbers[key] || {}).filter(([name]) => list.some((v) => same(v, name))));
    if (Object.keys(kept).length) set[key] = kept;
  }
  if (Object.keys(set).length) out.numbers = set;
  const skipped = {};
  for (const [key, list] of Object.entries(order)) {
    const kept = (unnumbered[key] || []).filter((name) => list.some((v) => same(v, name)));
    if (kept.length) skipped[key] = kept;
  }
  if (Object.keys(skipped).length) out.unnumbered = skipped;
  const map = mapOf($("map").value);
  $("map").classList.toggle("invalid", !map);
  $("map-error").textContent = map ? "" : "Write each line as key: value = as, value = as";
  if (map && Object.keys(map).length) out.map = map;
  return out;
}

// mapOf reads the Map box: a line a key, "tags: network-1 = address01,
// network-2 = address02". Null when a line is not that.
function mapOf(text) {
  const out = {};
  for (const line of text.split("\n").map((s) => s.trim()).filter(Boolean)) {
    const at = line.indexOf(":");
    const key = line.slice(0, at).trim();
    if (at < 1 || !key) return null;
    const pairs = line.slice(at + 1).split(",").map((s) => s.trim()).filter(Boolean);
    for (const pair of pairs) {
      const [from, to, ...rest] = pair.split("=").map((s) => s.trim());
      if (!from || !to || rest.length) return null;
      (out[key] ||= {})[from] = to;
    }
    if (!out[key]) return null;
  }
  return out;
}

// numberLast puts value last in key's order.
export function numberLast(key, value) {
  orders[key] = [...(orders[key] || []), value];
  drawOrder();
  changed();
}

$("add-condition").addEventListener("click", () => {
  $("conditions").append(condition(keys.find((k) => k === "owner") || keys[0], []));
  changed();
});
$("add-exclude").addEventListener("click", () => {
  $("exclude").append(condition("tags", [], true));
  changed();
});

// skipItem leaves an Item out of the rule, whatever else it selects.
export function skipItem(id) {
  if (!skip.includes(id)) skip.push(id);
  drawSkip();
  drawOrder();
  changed();
}

// drawSkip lists the Items left out, each put back with ×, and offers the
// Items the rule selects, so one is picked from those, not from the tree.
function drawSkip() {
  const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
  const unskip = (id) => { skip = skip.filter((x) => x !== id); drawSkip(); drawOrder(); changed(); };
  $("skip-list").replaceChildren(...skip.map((id) => el("li", { title: id },
    el("span", { className: byId[id] ? "" : "muted" }, byId[id] ? label(state, byId[id]) : id + " — no such Item"),
    el("button", { type: "button", className: "tool", title: "Put it back", textContent: "×", onclick: () => unskip(id) }))));
  const offered = selected().sort((a, b) => label(state, a).localeCompare(label(state, b)));
  $("add-skip").replaceChildren(el("option", { value: "" }, offered.length ? "+ Leave out an Item…" : "The rule selects no Item"),
    ...offered.map((i) => el("option", { value: i.id }, label(state, i))));
  $("add-skip").disabled = !offered.length;
}
$("add-skip").addEventListener("change", (event) => {
  const id = event.target.value;
  event.target.value = "";
  if (id) skipItem(id);
});


// same reports whether a and b are one value: one country however each is
// written, or else equal ignoring case.
const same = (a, b) => (countries[a] && countries[a] === countries[b]) || a.toLowerCase() === b.toLowerCase();

// chosenTypes is the types ticked, or every type when none is: a rule
// without types selects them all.
function chosenTypes() {
  const ticked = [...$("types").querySelectorAll("input:checked")].map((i) => i.value);
  return ticked.length ? ticked : state.templates.map((t) => t.type);
}

// selected is the Items the form's query picks: the chosen types, and each
// condition's values when some are ticked, less the Items skipped and those
// an exclusion meets at their current revision. Tags count the Item's and
// any of its revisions'.
function selected() {
  const types = chosenTypes();
  const conditions = read_("conditions");
  const exclusions = read_("exclude");
  const excluded = (item) => exclusions.some(([key, values, only]) => {
    if (only.length && !only.includes(item.type)) return false;
    const [field] = splitKey(key);
    const head = item.head || item.revisions?.[item.revisions.length - 1]?.id || item.revisions?.[item.revisions.length - 1]?.digest;
    const held = field === "tags" ? [...(item.tags || []), ...((item.revisions || []).find((r) => (r.id || r.digest) === head)?.tags || [])]
      : field === "status" ? [item.superseded_by ? "superseded" : "", item.retired || item.superseded_by ? "retired" : ""].filter(Boolean)
      : [field === "type" ? item.type : currentFields(item)[field]].filter(Boolean);
    // A field the Item lacks is the one it inherits, as the server has it.
    if (!held.length && field !== "type" && field !== "tags" && field !== "status") {
      const from = inherited().map((l) => choiceValue(item, l + "." + field)).find(Boolean);
      if (from) held.push(from);
    }
    return accepts(key, held, values);
  });
  return state.items.filter((item) => !skip.includes(item.id) && !excluded(item) && types.includes(item.type) && conditions.every(([key, values, only]) => {
    if (only.length && !only.includes(item.type)) return true;
    const [field] = splitKey(key);
    const held = field === "tags" ? [...(item.tags || []), ...(item.revisions || []).flatMap((r) => r.tags || [])] : [currentFields(item)[field]].filter(Boolean);
    // A rule taking shared Items takes them for the people they are shared with.
    if (field === "owner" && $("shared")?.checked) held.push(...(item.shared_with || []));
    return accepts(key, held, values);
  }));
}

// CONTAINS ends a condition's key that matches a value holding the text,
// ignoring case, rather than one equal to it: "name contains".
const CONTAINS = " contains";
const splitKey = (key) => key.endsWith(CONTAINS) ? [key.slice(0, -CONTAINS.length), true] : [key, false];

// accepts reports whether a held value meets the condition key's values.
function accepts(key, held, values) {
  const [, contains] = splitKey(key);
  return held.some((h) => values.some((v) => contains ? h.toLowerCase().includes(v.toLowerCase()) : same(h, v)));
}

// read_ is the conditions in one list of rows, those with values.
const read_ = (id) => [...$(id).children].map((row) => row.read()).filter(([, values]) => values.length);

// narrow redraws what depends on the types: the field chips and each
// condition's values, keeping only what the chosen types have.
function narrow() {
  drawInherit(inherited());
  drawPath();
  for (const row of $("conditions").children) row.redraw();
  for (const row of $("exclude").children) row.redraw();
  drawSkip();
  drawOrder();
  changed();
}

// condition is one query key, how it matches, and the values it accepts.
// "is" picks values from those the Items hold, so a value is never typed
// in a form no Item uses; a saved value is ticked as the held value it is
// one with — CHN as 中国 — and one no Item holds any more is still listed,
// ticked. "contains" takes typed text, several separated by commas.
// An exclusion may also name a status, which no field holds and which is
// matched whole.
// Ticking types under "for" asks the condition of those types only.
function condition(key, values, exclusion = false, only = []) {
  // What the query picks changes the numbering shown.
  const touched = () => { drawOrder(); drawSkip(); drawPath(); changed(); };
  const [field, contains] = splitKey(key);
  const choices = el("div", { className: "checks" });
  const text = el("input", { className: "mono", spellcheck: false, autocomplete: "off", placeholder: "text, other text", oninput: touched,
    title: "Matches a value holding any of these, ignoring case" });
  const draw = (saved) => {
    op.hidden = pick.value === "status";
    if (op.hidden) op.value = "is";
    if (op.value === "contains") {
      text.value = saved.join(", ");
      return choices.replaceChildren(text);
    }
    const types = chosenTypes();
    const chosen = state.items.filter((item) => types.includes(item.type));
    // Tags are the Items' and their revisions' own, matched per revision.
    const held = pick.value === "status" ? ["superseded", "retired"] : [...new Set(pick.value === "tags" ? chosen.flatMap((item) => [...(item.tags || []), ...(item.revisions || []).flatMap((r) => r.tags || [])])
      : chosen.map((item) => currentFields(item)[pick.value]).filter(Boolean))];
    const all = [...held, ...saved.filter((v) => !held.some((h) => same(h, v)))].sort((a, b) => a.localeCompare(b));
    choices.replaceChildren(...(all.length ? all.map((v) => el("label", {},
      el("input", { type: "checkbox", value: v, checked: saved.some((s) => same(s, v)), onchange: touched }), " " + v))
      : [el("span", { className: "template-sub", textContent: "no Item has one" })]));
  };
  const current = () => op.value === "contains" ? text.value.split(",").map((v) => v.trim()).filter(Boolean)
    : [...choices.querySelectorAll("input:checked")].map((i) => i.value);
  const pick = el("select", { onchange: () => { draw(op.value === "contains" ? current() : []); touched(); } },
    ...[...keys.filter((k) => !["type", "revision", "ext", "id", "tags", "status"].includes(k)), "tags", ...(exclusion ? ["status"] : [])].map((k) => el("option", { value: k, selected: k === field }, k)));
  const op = el("select", { className: "condition-op", title: "is: one of the values ticked; contains: holds any text typed",
    onchange: () => { draw([]); touched(); } },
    el("option", { value: "is", selected: !contains }, "is"), el("option", { value: "contains", selected: contains }, "contains"));
  const row = el("div", { className: "condition" },
    el("div", { className: "condition-head" }, pick, op,
      el("button", { type: "button", className: "tool", title: "Remove", textContent: "×", onclick: () => { row.remove(); touched(); } })),
    choices);
  // The types it is asked of fold away behind a button naming them.
  const scope = el("div", { className: "checks condition-types", hidden: true });
  const ticked = () => [...scope.querySelectorAll("input:checked")].map((i) => i.value);
  const forButton = el("button", { type: "button", className: "condition-for", title: "Ask this condition of some types only",
    onclick: () => { scope.hidden = !scope.hidden; } });
  const named = () => { const t = ticked(); forButton.textContent = t.length ? "for " + t.join(", ") : "for every type"; };
  const drawScope = (saved) => {
    const all = [...new Set([...chosenTypes(), ...saved])];
    forButton.hidden = all.length < 2 && !saved.length;
    scope.replaceChildren(...all.map((t) => el("label", {},
      el("input", { type: "checkbox", value: t, checked: saved.includes(t), onchange: () => { named(); touched(); } }), " " + t)));
    named();
  };
  row.append(forButton, scope);
  row.read = () => [pick.value + (op.value === "contains" ? CONTAINS : ""), current(), forButton.hidden ? [] : ticked()];
  row.redraw = () => { draw(current()); drawScope(ticked()); };
  drawScope(only || []);
  draw(values || []);
  return row;
}

// What each key a layout can use writes. A Template's own field is named
// with the types that have it.
const BUILT_IN = {
  type: "the Template's type, such as id_card",
  kind: "document or record",
  id: "the Item's ID",
  revision: "the revision's number, from 1",
  ext: "the extension: pdf",
  year: "the year of issued_at",
  month: "the month of issued_at",
  date: "issued_at, YYYY-MM-DD",
};
const FORMATS = { zh: "中国", en: "China", alpha2: "CN", alpha3: "CHN" };
// A type is written in its Template's names.
const TYPE_FORMATS = { zh: "the Chinese name, such as 驾驶证", en: "the English name, such as Driver licence" };

const countryKeys = () => new Set(state.templates.flatMap((t) => t.fields.filter((f) => f.type === "country").map((f) => f.key)));

// linkFields is the fields of the chosen types' Templates that link to
// another Item: an Item's keys can be taken from the Item they name.
const linkFields = () => {
  const types = chosenTypes();
  return [...new Set(state.templates.filter((t) => types.includes(t.type))
    .flatMap((t) => t.fields.filter((f) => f.type === "item" || f.type === "revision").map((f) => f.key)))];
};

function describe(key) {
  if (BUILT_IN[key]) return BUILT_IN[key];
  const types = state.templates.filter((t) => t.fields.some((f) => f.key === key)).map((t) => t.type);
  return (countryKeys().has(key) ? "a country, as the Item keeps it" : "a field") + (types.length ? " of " + types.join(", ") : "");
}

// The path is edited part by part, never as text. A row is a folder or,
// last, the file's name: { parts, optional }. An optional row is a folder
// only an Item with every key in it gets, written [/…] after the row
// before. A part is fixed text { text }, a key and its alternatives
// { keys: ["name", "type:zh"] }, the number {#} { counter: true }, or an
// optional group { group: [parts] }, written only when the Item has every
// key in it. The server parses a saved layout into these; the page writes
// them back as the layout it saves.
let rows = [];
let active = { row: 0, group: -1 }; // where a chip adds: a row, or a group in it
const DEFAULT_ROWS = () => [{ parts: [{ keys: ["owner"] }] }, { parts: [{ keys: ["type"] }, { text: "." }, { keys: ["ext"] }] }];

// fromLayout makes rows of a layout the server parsed: a folder group
// ending a name becomes an optional row of its own.
function fromLayout(layout) {
  const part = (p) => p.counter ? { counter: true } : p.end ? { end: true } : p.group ? { group: p.group.map(part) }
    : p.key ? { keys: [p, ...(p.or || [])].map((c) => c.key + (c.format ? ":" + c.format : "")) } : { text: p.text };
  const out = [];
  for (const segment of layout) {
    const last = segment[segment.length - 1];
    if (last && last.folder) {
      out.push({ parts: segment.slice(0, -1).map(part) }, { parts: last.group.map(part), optional: true });
    } else {
      out.push({ parts: segment.map(part) });
    }
  }
  return out;
}

const written = (p) => p.counter ? "{#}" : p.end ? "{/#}" : p.group ? "[" + p.group.map(written).join("") + "]" : p.keys ? "{" + p.keys.join("|") + "}" : p.text;

// layoutText is the rows as the layout the rule saves.
export const layoutText = () => rows.map((row, i) => {
  const inner = row.parts.map(written).join("");
  return row.optional ? "[/" + inner + "]" : (i ? "/" : "") + inner;
}).join("");

// The container a chip adds to: the active row's parts, or a group's.
function target() {
  const row = rows[Math.min(active.row, rows.length - 1)];
  const group = row.parts[active.group];
  return group && group.group ? group.group : row.parts;
}
const inGroup = () => active.group >= 0 && !!rows[active.row]?.parts[active.group]?.group;

function edited() {
  drawPath();
  drawOrder();
  changed();
}

// add puts a part last in the container chips add to; in the file's name,
// before its .{ext}.
function add(part) {
  const list = target();
  let at = list.length;
  if (list === rows[rows.length - 1].parts && list[at - 1]?.keys?.[0] === "ext") at -= list[at - 2]?.text === "." ? 2 : 1;
  list.splice(at, 0, part);
  edited();
}

// heldBy is the field keys the Templates of types have.
const heldBy = (types) => new Set(state.templates.filter((t) => types.includes(t.type)).flatMap((t) => t.fields.map((f) => f.key)));

// linkedTypes is the types the Item a link field names can be: those its
// match ties to a field the query limits, such as of: diploma, or else
// every type.
function linkedTypes(link) {
  const all = state.templates.map((t) => t.type);
  const query = read().query;
  const types = chosenTypes();
  const f = state.templates.filter((t) => types.includes(t.type)).flatMap((t) => t.fields).find((x) => x.key === link);
  const by = f && f.match && f.match.type;
  const wanted = by && query[by];
  return wanted && wanted.length ? all.filter((t) => wanted.includes(t)) : all;
}

// linkKeys is the keys offered from the Item a link field names.
function linkKeys(link) {
  const held = heldBy(linkedTypes(link));
  return [...keys.filter((k) => !BUILT_IN[k] && k !== link && held.has(k)), "type"];
}

// The keys a part may be, for its menus: the chosen types' fields and the
// keys every PDF has, each country field and type in each form, and the
// keys of the Items link fields name, original.level.
function keyOptions() {
  const held = heldBy(chosenTypes());
  const plain = keys.filter((k) => k !== "type" && (BUILT_IN[k] || held.has(k)));
  return [
    ["Keys", [...plain, "type", "type:zh", "type:en"]],
    ["Countries", [...countryKeys()].filter((k) => held.has(k)).flatMap((k) => Object.keys(FORMATS).map((f) => k + ":" + f))],
    ...linkFields().map((l) => ["From " + l, [...linkKeys(l), "type:zh", "type:en", "year", "month", "date"].map((k) => l + "." + k)]),
  ];
}

function keyPart(part, remove) {
  const box = el("span", { className: "path-key", title: "A key. Several are alternatives: the first the Item has is written." });
  const draw = () => {
    const options = keyOptions();
    box.replaceChildren(...part.keys.flatMap((k, i) => {
      const known = options.some(([, list]) => list.includes(k));
      // A click stays in the menu: the row it would reach redraws.
      const pick = el("select", { onclick: (event) => event.stopPropagation(), onmousedown: (event) => event.stopPropagation(), onchange: () => { part.keys[i] = pick.value; edited(); } },
        ...(known ? [] : [el("option", { value: k, selected: true }, k)]),
        ...options.filter(([, list]) => list.length).map(([name, list]) => el("optgroup", { label: name },
          ...list.map((o) => el("option", { value: o, selected: o === k, title: describe(o.split(/[:.]/)[0]) }, o)))));
      const drop = part.keys.length > 1 ? el("button", { type: "button", className: "path-x", title: "Remove this alternative", textContent: "×",
        onclick: (event) => { event.stopPropagation(); part.keys.splice(i, 1); edited(); } }) : null;
      return [i ? el("span", { className: "path-or", textContent: "or" }) : null, pick, drop].filter(Boolean);
    }),
    el("button", { type: "button", className: "path-more", title: "Add an alternative, written when the Item lacks the keys before it", textContent: "+ or",
      onclick: (event) => { event.stopPropagation(); part.keys.push(part.keys[part.keys.length - 1]); edited(); } }),
    el("button", { type: "button", className: "path-x", title: "Remove the key", textContent: "×", onclick: (event) => { event.stopPropagation(); remove(); } }));
  };
  draw();
  return box;
}

// fit sizes a text input to its text: a wide character, 中, takes two
// columns of a narrow one.
function fit(input) {
  const cols = [...input.value].reduce((n, c) => n + (/[\u1100-\uffff]/.test(c) && !/[\uff61-\uffdc]/.test(c) ? 2 : 1), 0);
  input.style.width = `calc(${Math.max(1, cols)}ch + 20px)`;
}

// partEl draws one part of a container; the container's own list is where
// it moves and is removed from.
function partEl(part, list, r, g) {
  const i = list.indexOf(part);
  const remove = () => { list.splice(list.indexOf(part), 1); if (part.group) active = { row: r, group: -1 }; edited(); };
  let node;
  if (part.counter) {
    node = el("span", { className: "path-counter", title: "Numbers the rest of this name, 01, 02, in the order below" }, "#",
      el("button", { type: "button", className: "path-x", title: "Remove the number", textContent: "×", onclick: (event) => { event.stopPropagation(); remove(); } }));
  } else if (part.end) {
    node = el("span", { className: "path-counter", title: "The number stops here: what follows is written but not numbered" }, "/#",
      el("button", { type: "button", className: "path-x", title: "Remove the stop", textContent: "×", onclick: (event) => { event.stopPropagation(); remove(); } }));
  } else if (part.keys) {
    node = keyPart(part, remove);
  } else if (part.group) {
    const at = rows[r].parts.indexOf(part);
    node = el("span", { className: "path-group" + (active.row === r && active.group === at ? " active" : ""),
      title: "Optional: written only when the Item has every key in it; otherwise nothing, its text included",
      onclick: (event) => { event.stopPropagation(); active = { row: r, group: at }; drawPath(); } },
      el("span", { className: "path-group-name", textContent: "if any" }),
      ...(part.group.length ? part.group.map((p) => partEl(p, part.group, r, at)) : [el("span", { className: "muted", textContent: "click a key below" })]),
      el("button", { type: "button", className: "path-x", title: "Remove the optional part", textContent: "×", onclick: (event) => { event.stopPropagation(); remove(); } }));
  } else {
    // Text is typed in place; what would start a key, group or folder is
    // left out, and emptied text goes. An optional folder's own text may
    // hold /: bill/{service} adds both folders or neither.
    const slash = rows[r]?.optional && list === rows[r].parts;
    node = el("input", { className: "path-text", value: part.text,  spellcheck: false, autocomplete: "off",
      title: slash ? "Fixed text; / adds a folder" : "Fixed text", onclick: (event) => event.stopPropagation(),
      oninput: (event) => {
        part.text = event.target.value.replace(slash ? /[{}[\]\\]/g : /[{}[\]/\\]/g, "");
        event.target.value = part.text;
        fit(event.target);
        showText();
        drawOrder();
        changed();
      },
      onchange: () => { if (!part.text) remove(); } });
    fit(node);
  }
  // A part is dragged to another place in its own container.
  node.draggable = !(part.text !== undefined);
  node.dataset.row = r;
  node.dataset.group = g;
  node.addEventListener("dragstart", (event) => { event.stopPropagation(); dragging = { list, part }; event.dataTransfer.effectAllowed = "move"; });
  node.addEventListener("dragover", (event) => { if (dragging && dragging.list === list) { event.preventDefault(); event.stopPropagation(); node.classList.add("drop"); } });
  node.addEventListener("dragleave", () => node.classList.remove("drop"));
  node.addEventListener("drop", (event) => {
    node.classList.remove("drop");
    if (!dragging || dragging.list !== list) return;
    event.preventDefault();
    event.stopPropagation();
    const from = list.indexOf(dragging.part);
    list.splice(from, 1);
    list.splice(list.indexOf(part) + (from <= i ? 1 : 0), 0, dragging.part);
    dragging = null;
    edited();
  });
  return node;
}
let dragging = null;

// showText writes the layout the rows make into its box, unless it is
// being typed in.
function showText() {
  const box = $("layout-text");
  if (document.activeElement === box) return;
  box.value = layoutText();
  box.classList.remove("invalid");
  $("layout-error").textContent = "";
}

// typed takes the layout typed in its box: the server parses it and the
// rows are drawn from it; one it refuses stays, marked, with the reason.
let typing = 0;
async function typed() {
  const box = $("layout-text"), mine = ++typing;
  try {
    const answer = await post("/api/rules/layout", { layout: box.value.trim() });
    if (mine !== typing) return;
    rows = fromLayout(answer.layout);
    active = { row: rows.length - 1, group: -1 };
    box.classList.remove("invalid");
    $("layout-error").textContent = "";
    edited();
  } catch (err) {
    if (mine !== typing) return;
    box.classList.add("invalid");
    $("layout-error").textContent = err.message;
  }
}
// settle takes a layout typed and not yet taken, as Save is pressed; one
// the server refuses stops the save.
export async function settle() {
  const box = $("layout-text");
  if (box.value.trim() === layoutText()) return;
  await typed();
  if (box.classList.contains("invalid")) throw new Error($("layout-error").textContent);
}
$("layout-text").addEventListener("change", () => typed());
$("layout-text").addEventListener("keydown", (event) => { if (event.key === "Enter") { event.preventDefault(); $("layout-text").blur(); } });

// drawPath draws the rows, the one chips add to marked, and the layout
// they make, which is shown and never typed.
function drawPath() {
  if (active.row >= rows.length) active = { row: rows.length - 1, group: -1 };
  const last = rows.length - 1;
  const act = (title, text, onclick, disabled) => el("button", { type: "button", className: "order-act", title, textContent: text, disabled,
    onclick: (event) => { event.stopPropagation(); onclick(); } });
  $("path").replaceChildren(...rows.map((row, r) => {
    const file = r === last;
    const optionalAllowed = r > 0 && !file && !rows[r - 1].optional;
    // Each row sits under the one before, indented a step with a line
    // down from its parent, as the tree is drawn.
    const node = el("div", { className: "path-row" + (row.optional ? " optional" : "") + (active.row === r && active.group < 0 ? " active" : ""),
      onclick: () => { active = { row: r, group: -1 }; drawPath(); } },
      el("span", { className: "path-kind", textContent: file ? "File" : row.optional ? "If any" : "Folder",
        title: file ? "The PDF's name" : row.optional ? "A folder only an Item with every key in it gets" : "A folder" }),
      el("div", { className: "path-parts" }, ...(row.parts.length ? row.parts.map((p) => partEl(p, row.parts, r, -1)) : [el("span", { className: "muted", textContent: "click a key below" })])),
      el("span", { className: "path-acts" },
        file ? null : el("label", { className: "path-optional", title: "Only an Item with every key in this folder gets it; the others stay in the folder before",
          onclick: (event) => event.stopPropagation() },
          el("input", { type: "checkbox", checked: !!row.optional, disabled: !optionalAllowed && !row.optional,
            onchange: (event) => { row.optional = event.target.checked; edited(); } }), " optional"),
        file ? null : act("Up", "↑", () => { [rows[r - 1], rows[r]] = [rows[r], rows[r - 1]]; active = { row: r - 1, group: -1 }; edited(); }, r === 0),
        file ? null : act("Down", "↓", () => { [rows[r + 1], rows[r]] = [rows[r], rows[r + 1]]; active = { row: r + 1, group: -1 }; edited(); }, r + 1 >= last),
        file ? null : act("Remove the folder", "×", () => { rows.splice(r, 1); active = { row: Math.min(r, rows.length - 1), group: -1 }; edited(); })));
    const level = el("div", { className: "path-level" }, node);
    level.style.setProperty("--depth", r);
    return level;
  }), el("button", { type: "button", className: "button path-add", textContent: "+ Folder", title: "Add a folder before the PDF's name",
    onclick: () => { rows.splice(rows.length - 1, 0, { parts: [] }); active = { row: rows.length - 2, group: -1 }; edited(); } }));
  showText();
  chips();
}

// The keys as chips that add to the row or optional part chosen: fields,
// the ones every PDF has, text, a number and an optional part, and each
// country field in each form it can be written in.
function chips() {
  const chip = (text, part, title, disabled) => el("button", {
    type: "button", className: "chip", textContent: text, title, disabled,
    onclick: () => {
      const made = part();
      if (made.counter) {
        // The number starts its name, 01-.
        target().unshift(made, { text: "-" });
        return edited();
      }
      add(made);
      if (made.group) { active = { row: active.row, group: rows[active.row].parts.indexOf(made) }; drawPath(); }
      if (made.text !== undefined) {
        // The new text is focused to type into: the only empty one.
        [...$("path").querySelectorAll(".path-text")].find((input) => input.value === "")?.focus();
      }
    },
  });
  const group = (name, ...children) => children.length ? el("div", { className: "key-group" },
    el("span", { className: "key-group-name" }, name), el("div", { className: "key-chips" }, ...children)) : null;
  const types = chosenTypes();
  const held = heldBy(types);
  const fields = keys.filter((k) => !BUILT_IN[k] && held.has(k));
  const countries = [...countryKeys()].filter((k) => held.has(k));
  const numbered = target().some((p) => p.counter);
  const stopped = target().some((p) => p.end);
  const key = (k) => () => ({ keys: [k] });
  $("keys").replaceChildren(...[
    group("Fields", ...fields.map((k) => chip(k, key(k), describe(k)))),
    group("Every PDF", ...keys.filter((k) => BUILT_IN[k]).map((k) => chip(k, key(k), describe(k)))),
    group("Add", chip("text", () => ({ text: "" }), "Fixed text, such as - or 03-Education"),
      chip("#", () => ({ counter: true }), inGroup() ? "A number goes only in a row, or in an optional folder" : "Numbers the rest of this name, 01-, 02-, in the order below", numbered || inGroup()),
      chip("/#", () => ({ end: true }), "Stops the number: what follows, such as a date, is written but not numbered", !numbered || stopped || inGroup()),
      chip("if any […]", () => ({ group: [] }), "An optional part, written only when the Item has every key in it: -本科 or nothing", inGroup())),
    ...countries.map((k) => group(k + " as", ...Object.entries(FORMATS).map(([f, example]) =>
      chip(":" + f, key(k + ":" + f), `{${k}:${f}} writes ${example}`)))),
    group("type as", ...Object.entries(TYPE_FORMATS).map(([f, note]) => chip(":" + f, key("type:" + f), note))),
    ...linkFields().map((l) => group("from " + l, ...[...linkKeys(l), "type:zh", "type:en"].map((k) => chip(k, key(l + "." + k), "the " + k + " of the Item " + l + " links to")))),
  ].filter(Boolean));
}

// drawInherit offers each link field of the rule's types, ticked when the
// rule takes the keys an Item lacks from the Item it names.
function drawInherit(saved) {
  const all = [...new Set([...linkFields(), ...saved])];
  $("inherit-row").hidden = !all.length;
  $("inherit").replaceChildren(...all.map((l) => el("label", { title: "A key an Item lacks is taken from the Item its " + l + " links to" },
    el("input", { type: "checkbox", value: l, checked: saved.includes(l), onchange: () => { drawOrder(); changed(); } }), " keys it lacks, from its " + l)));
}
const inherited = () => [...$("inherit").querySelectorAll("input:checked")].map((i) => i.value);

// The orders the layout numbers from, each once: a row with {#} numbers
// what follows it up to {/#}, less the text straight after {#} and a trailing .{ext},
// and its order is named that as written.
export const numberedKeys = () => [...new Set(rows.flatMap((row) => {
  const at = row.parts.findIndex((p) => p.counter);
  if (at < 0) return [];
  let rest = row.parts.slice(at + 1);
  const end = rest.findIndex((p) => p.end);
  if (end >= 0) rest = rest.slice(0, end);
  if (rest.length && rest[0].text !== undefined) rest = rest.slice(1);
  if (rest.length && rest[rest.length - 1].keys && rest[rest.length - 1].keys[0] === "ext") {
    rest = rest.slice(0, -1);
    if (rest.length && rest[rest.length - 1].text === ".") rest = rest.slice(0, -1);
  }
  return rest.some((p) => p.keys || p.group) ? [rest.map(written).join("")] : [];
}))];

// An Item's value for one key, {a|b} or {a|b:format}: the first alternative
// it has, or with inherit the first the Items it links to have. A type:zh
// or type:en is its Template's name.
function keyValue(item, inner) {
  for (const choice of inner.split("|")) {
    const value = choiceValue(item, choice) || (choice.includes(".") ? "" : inherited().map((l) => choiceValue(item, l + "." + choice)).find(Boolean));
    if (value) return value;
  }
  return "";
}

function choiceValue(item, choice) {
  const [name, format] = choice.split(":");
  if (name.includes(".")) {
    // original.level: the key of the revision the field original links to.
    const [field, key] = name.split(".");
    const [id, ref] = String((item.fields || {})[field] || "").split("@");
    const other = state.items.find((i) => i.id === id);
    const rev = other && (ref ? other.revisions.find((r) => (r.id || r.digest) === ref) : null);
    const fields = !other ? {} : !rev ? other.fields : rev.snapshot ? rev.fields || {} : { ...other.fields, ...(rev.fields || {}) };
    // original.type:zh is the linked Item's type in a language.
    if (key === "type") return format && other ? (state.templates.find((t) => t.type === other.type)?.names || {})[format] : other?.type;
    return fields[key];
  }
  return name === "type"
    ? (format ? (state.templates.find((t) => t.type === item.type)?.names || {})[format] : item.type)
    : item.fields && item.fields[name];
}

// An Item's name in one order: the order's name, its {key}s and [groups],
// filled in. A key it lacks makes no name; a group it lacks a key of is
// left out.
export function orderValue(item, key) {
  let whole = true;
  const fill = (text) => {
    let ok = true;
    const out = text.replace(/\{([^{}]*)\}/g, (_, inner) => { const v = keyValue(item, inner); if (!v) ok = false; return v || ""; });
    return [out, ok];
  };
  const name = key.replace(/\[([^\]]*)\]|\{([^{}]*)\}/g, (_, group, inner) => {
    if (group !== undefined) { const [out, ok] = fill(group); return ok ? out : ""; }
    const v = keyValue(item, inner);
    if (!v) whole = false;
    return v || "";
  });
  return whole ? name : "";
}

// One row per order: the values it may have, in the order they are numbered
// from 01. An order new to the layout starts with the values Items have.
// numbered is the number each name of an order gets: the next after the
// one before, unless set gives it its own, from which the rest count on.
// A name skip lists is null and takes no number.
function numbered(values, set = {}, skip = []) {
  let n = 0;
  return values.map((v) => {
    if (skip.some((name) => same(name, v))) return null;
    const own = Object.entries(set).find(([name]) => same(name, v));
    n = own ? own[1] : n + 1;
    return n;
  });
}

function drawOrder() {
  const lists = numberedKeys().map((key) => {
    // A key given alternatives or losing them, {level} become
    // {level|original.level} or back, keeps the order it had.
    const plain = (name) => name.replace(/\|[^{}]*?(?=\})/g, "");
    const before = orders[key] === undefined && Object.keys(orders).find((k) => orders[k] && plain(k) === plain(key));
    if (before) {
      orders[key] = orders[before];
      if (numbers[before]) numbers[key] = numbers[before];
      if (unnumbered[before]) unnumbered[key] = unnumbered[before];
    }
    if (orders[key] === undefined) {
      orders[key] = [...new Set(selected().map((i) => orderValue(i, key)).filter(Boolean))].sort();
    }
    const box = el("div", { className: "order" });
    const list = () => orders[key].filter(Boolean);
    const set = (values) => { orders[key] = [...values]; draw(); changed(); };
    let dragged = -1;
    const draw = () => {
      const values = list();
      // Only the values the query's Items have are shown, each with its
      // number in the whole order; the others stay in it, numbered, hidden.
      const held = [...new Set(selected().map((i) => orderValue(i, key)).filter(Boolean))];
      const shown = values.map((v, i) => i).filter((i) => held.some((h) => same(h, values[i])));
      const hidden = values.length - shown.length;
      const unlisted = held.filter((v) => !values.some((w) => same(v, w))).sort();
      // move swaps a shown value with the shown one before or after it.
      const move = (a, b) => { const next = [...values]; [next[a], next[b]] = [next[b], next[a]]; set(next); };
      const act = (title, text, onclick, disabled) => el("button", { type: "button", className: "order-act", title, textContent: text, disabled, onclick });
      const counted = numbered(values, numbers[key], unnumbered[key]);
      const pad = Math.max(2, String(Math.max(0, ...counted)).length);
      // before is the number of the last numbered name before i.
      const before = (i) => counted.slice(0, i).filter((n) => n !== null).pop();
      const skipped = (v) => (unnumbered[key] || []).some((name) => same(name, v));
      // toggle leaves a name unnumbered, or numbers it again.
      const toggle = (v) => {
        unnumbered[key] = skipped(v) ? unnumbered[key].filter((name) => !same(name, v)) : [...(unnumbered[key] || []), v];
        numbers[key] = Object.fromEntries(Object.entries(numbers[key] || {}).filter(([name]) => !same(name, v)));
        draw();
        changed();
      };
      // renumber sets a name's number by hand, or clears it when it is the
      // next one anyway; a number not after the one before is refused.
      const renumber = (i, input) => {
        const own = { ...(numbers[key] || {}) };
        for (const name of Object.keys(own)) if (same(name, values[i])) delete own[name];
        const typed = parseInt(input.value, 10);
        const next = numbered(values.slice(0, i + 1), own, unnumbered[key])[i];
        if (input.value.trim() !== "" && typed !== next) {
          if (!(typed >= 0) || (before(i) !== undefined && typed <= before(i))) { input.value = String(counted[i]).padStart(pad, "0"); input.classList.add("invalid"); return; }
          own[values[i]] = typed;
        }
        numbers[key] = own;
        draw();
        changed();
      };
      box.replaceChildren(...[
        el("ol", { className: "order-list" }, ...shown.map((i, k) => {
          const v = values[i];
          const li = el("li", {
            draggable: true,
            ondragstart: (event) => { dragged = i; event.dataTransfer.effectAllowed = "move"; li.classList.add("dragging"); },
            ondragend: () => li.classList.remove("dragging"),
            ondragover: (event) => { event.preventDefault(); li.classList.add("drop"); },
            ondragleave: () => li.classList.remove("drop"),
            ondrop: (event) => {
              event.preventDefault(); li.classList.remove("drop");
              if (dragged >= 0 && dragged !== i) { const next = [...values]; next.splice(i, 0, ...next.splice(dragged, 1)); set(next); }
              dragged = -1;
            },
          },
            el("span", { className: "order-grip", textContent: "⋮⋮", "aria-hidden": "true" }),
            counted[i] === null
              ? el("span", { className: "order-number unnumbered", textContent: "—", title: "Not numbered: no number, nor the text after {#}" })
              : el("input", { className: "order-number" + (Object.keys(numbers[key] || {}).some((name) => same(name, v)) ? " set" : ""), type: "text", inputMode: "numeric",
                value: String(counted[i]).padStart(pad, "0"), title: "Its number. Type another to skip some; the ones after count on from it. Clear it to count on from the one before.",
                onchange: (event) => renumber(i, event.target), onkeydown: (event) => { if (event.key === "Enter") { event.preventDefault(); event.target.blur(); } } }),
            el("span", { className: "order-value", textContent: v, title: v }),
            el("span", { className: "order-acts" },
              el("button", { type: "button", className: "order-act" + (skipped(v) ? " on" : ""), textContent: "#",
                title: skipped(v) ? "Number it again" : "Leave it unnumbered", onclick: () => toggle(v) }),
              act("Up", "↑", () => move(i, shown[k - 1]), k === 0),
              act("Down", "↓", () => move(i, shown[k + 1]), k === shown.length - 1),
              act("Remove", "×", () => set(values.filter((_, n) => n !== i)))));
          return li;
        })),
        hidden ? el("p", { className: "template-sub", textContent: hidden + (hidden === 1 ? " value no Item this rule picks has is" : " values no Item this rule picks has are") + " kept in the order, hidden: " + values.filter((_, i) => !shown.includes(i)).join(", ") }) : null,
        unlisted.length ? el("div", { className: "order-add" }, el("span", { className: "order-add-label", textContent: "Not numbered" }),
          ...unlisted.map((v) => el("button", { type: "button", className: "order-chip", textContent: "+ " + v, title: "Number it last", onclick: () => set([...values, v]) })))
          : null,
      ].filter(Boolean));
    };
    draw();
    return el("div", { className: "condition" }, el("code", {}, "{#}-" + key), box);
  });
  $("order").replaceChildren(...lists);
  $("order-row").hidden = !lists.length;
}
