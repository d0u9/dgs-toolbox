// The part of the Outlines form that makes one rule: types, conditions,
// revisions, a layout built from key chips, and a Numbering list per
// {#}. The page owns the rest of
// its form and what a change redraws; the server computes the result.
import { $, el, currentFields, label } from "/common.js";

let state = { templates: [], items: [] };
let keys = [];
let countries = {}; // each country's every form: its alpha-3 code
let orders = {}; // per numbered name, its order as typed: "alex, emma"
let skip = []; // the IDs of the Items the rule leaves out
let numbers = {}; // per numbered name, the numbers set by hand: { 结婚证: 6 }
let changed = () => {};

// setup gives the form the tree's state, the keys a layout can use, the
// country forms, and what to call on every change. Called again when the
// state is reloaded.
export function setup(options) {
  ({ state = state, keys = keys, countries = countries } = options);
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
  $("layout").value = v.layout;
  orders = Object.fromEntries(Object.entries(v.order || {}).map(([k, list]) => [k, list.join(", ")]));
  numbers = JSON.parse(JSON.stringify(v.numbers || {}));
  drawOrder();
  closeSuggest();
  chips();
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
  const out = { query, selection: document.querySelector("input[name=selection]:checked").value, layout: $("layout").value.trim() };
  if ($("shared")?.checked) out.shared = true;
  if (Object.keys(exclude).length) out.exclude = exclude;
  if (Object.keys(queryTypes).length) out.query_types = queryTypes;
  if (Object.keys(excludeTypes).length) out.exclude_types = excludeTypes;
  if (skip.length) out.skip = [...skip];
  const order = {};
  for (const key of numberedKeys()) {
    const list = (orders[key] || "").split(",").map((s) => s.trim()).filter(Boolean);
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
  return out;
}

// numberLast puts value last in key's order.
export function numberLast(key, value) {
  orders[key] = [...(orders[key] || "").split(",").map((s) => s.trim()).filter(Boolean), value].join(", ");
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
  chips();
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
  const touched = () => { drawOrder(); drawSkip(); changed(); };
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

function describe(key) {
  if (BUILT_IN[key]) return BUILT_IN[key];
  const types = state.templates.filter((t) => t.fields.some((f) => f.key === key)).map((t) => t.type);
  return (countryKeys().has(key) ? "a country, as the Item keeps it" : "a field") + (types.length ? " of " + types.join(", ") : "");
}

// The keys as chips that insert at the caret: fields, then the ones every
// PDF has, then each country field in each form it can be written in.
function chips() {
  const chip = (text, insertText, title) => el("button", {
    type: "button", className: "chip", textContent: text, title,
    onmousedown: (event) => event.preventDefault(),
    onclick: () => insert(insertText),
  });
  const group = (name, ...children) => children.length ? el("div", { className: "key-group" },
    el("span", { className: "key-group-name" }, name), el("div", { className: "key-chips" }, ...children)) : null;
  const types = chosenTypes();
  const held = new Set(state.templates.filter((t) => types.includes(t.type)).flatMap((t) => t.fields.map((f) => f.key)));
  const fields = keys.filter((k) => !BUILT_IN[k] && held.has(k));
  const countries = [...countryKeys()].filter((k) => held.has(k));
  $("keys").replaceChildren(...[
    group("Fields", ...fields.map((k) => chip(k, "{" + k + "}", describe(k)))),
    group("Every PDF", ...keys.filter((k) => BUILT_IN[k]).map((k) => chip(k, "{" + k + "}", describe(k))), chip("/", "/", "a folder"), chip("#", "{#}-", "numbers the rest of this folder or file name, 01-, 02-, in the order below")),
    ...countries.map((k) => group(k + " as", ...Object.entries(FORMATS).map(([f, example]) =>
      chip(":" + f, "{" + k + ":" + f + "}", `{${k}:${f}} writes ${example}`)))),
    group("type as", ...Object.entries(TYPE_FORMATS).map(([f, note]) => chip(":" + f, "{type:" + f + "}", note))),
  ].filter(Boolean));
}

// The suggestions under the layout while the caret is inside a { }: keys
// that start with what is typed, or after a country key's colon, its forms.
let suggestions = [];
let active = 0;

function suggest() {
  const input = $("layout");
  const before = input.value.slice(0, input.selectionStart ?? input.value.length);
  const open = /\{([^{}]*)$/.exec(before);
  if (!open || document.activeElement !== input) { closeSuggest(); return; }
  const typed = open[1];
  const colon = typed.indexOf(":");
  if (colon >= 0) {
    const key = typed.slice(0, colon);
    const part = typed.slice(colon + 1);
    suggestions = key === "type"
      ? Object.entries(TYPE_FORMATS).filter(([f]) => f.startsWith(part)).map(([f, note]) => ({ text: "type:" + f, note }))
      : countryKeys().has(key)
      ? Object.entries(FORMATS).filter(([f]) => f.startsWith(part)).map(([f, example]) => ({ text: key + ":" + f, note: "writes " + example }))
      : [];
  } else {
    suggestions = keys.filter((k) => k.startsWith(typed)).flatMap((k) => [{ text: k, note: describe(k) },
      ...(countryKeys().has(k) && typed === k ? Object.entries(FORMATS).map(([f, example]) => ({ text: k + ":" + f, note: "writes " + example })) : [])]);
  }
  if (!suggestions.length) { closeSuggest(); return; }
  active = Math.min(active, suggestions.length - 1);
  $("suggest").replaceChildren(...suggestions.map((s, i) => el("li", {
    className: i === active ? "active" : "", role: "option",
    onmousedown: (event) => { event.preventDefault(); accept(i); },
  }, el("code", {}, "{" + s.text + "}"), el("span", {}, s.note))));
  $("suggest").hidden = false;
}

function closeSuggest() {
  suggestions = [];
  active = 0;
  $("suggest").hidden = true;
}

function accept(i) {
  const input = $("layout");
  const caret = input.selectionStart;
  const start = input.value.lastIndexOf("{", caret - 1);
  const after = input.value.slice(caret).replace(/^[^{}\/]*\}/, "");
  const text = "{" + suggestions[i].text + "}";
  input.value = input.value.slice(0, start) + text + after;
  input.setSelectionRange(start + text.length, start + text.length);
  closeSuggest();
  changed();
}

$("layout").addEventListener("input", () => { active = 0; suggest(); drawOrder(); });

// The orders the layout numbers from, each once: a folder or file name
// with {#} numbers what follows it, less the text straight after {#} and
// a trailing .{ext}, and its order is named that as written.
export const numberedKeys = () => [...new Set($("layout").value.split(/\/(?![^{}]*\})/).flatMap((segment) => {
  const at = segment.indexOf("{#}");
  if (at < 0) return [];
  const rest = segment.slice(at + 3).replace(/^[^{]*/, "").replace(/\.?\{ext\}$/, "");
  return /\{[^#{}]+\}/.test(rest) ? [rest] : [];
}))];

// An Item's value for one key, {a|b} or {a|b:format}: the first alternative
// it has. A type:zh or type:en is its Template's name.
function keyValue(item, inner) {
  for (const choice of inner.split("|")) {
    const [name, format] = choice.split(":");
    const value = name === "type"
      ? (format ? (state.templates.find((t) => t.type === item.type)?.names || {})[format] : item.type)
      : item.fields && item.fields[name];
    if (value) return value;
  }
  return "";
}

// An Item's name in one order: the order's name, {key}s and {-key?}s, filled
// in. A key it lacks makes no name; an optional one it lacks is left out.
export function orderValue(item, key) {
  let whole = true;
  const name = key.replace(/\{([^{}]*)\}/g, (_, inner) => {
    const optional = inner.endsWith("?");
    const [, pre = "", k, post = ""] = optional ? /^([^a-z0-9_]*)(.*?)([^a-z0-9_]*)\?$/.exec(inner) : [null, "", inner, ""];
    const value = keyValue(item, k);
    if (!value && !optional) whole = false;
    return value ? pre + value + post : "";
  });
  return whole ? name : "";
}

// One row per order: the values it may have, in the order they are numbered
// from 01. An order new to the layout starts with the values Items have.
// numbered is the number each name of an order gets: the next after the
// one before, unless set gives it its own, from which the rest count on.
function numbered(values, set = {}) {
  let n = 0;
  return values.map((v) => {
    const own = Object.entries(set).find(([name]) => same(name, v));
    n = own ? own[1] : n + 1;
    return n;
  });
}

function drawOrder() {
  const rows = numberedKeys().map((key) => {
    if (orders[key] === undefined) {
      orders[key] = [...new Set(selected().map((i) => orderValue(i, key)).filter(Boolean))].sort().join(", ");
    }
    const box = el("div", { className: "order" });
    const list = () => orders[key].split(",").map((s) => s.trim()).filter(Boolean);
    const set = (values) => { orders[key] = values.join(", "); draw(); changed(); };
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
      const counted = numbered(values, numbers[key]);
      const pad = Math.max(2, String(counted[counted.length - 1] || 0).length);
      // renumber sets a name's number by hand, or clears it when it is the
      // next one anyway; a number not after the one before is refused.
      const renumber = (i, input) => {
        const own = { ...(numbers[key] || {}) };
        for (const name of Object.keys(own)) if (same(name, values[i])) delete own[name];
        const typed = parseInt(input.value, 10);
        const next = numbered(values.slice(0, i + 1), own)[i];
        if (input.value.trim() !== "" && typed !== next) {
          if (!(typed >= 1) || (i > 0 && typed <= counted[i - 1])) { input.value = String(counted[i]).padStart(pad, "0"); input.classList.add("invalid"); return; }
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
            el("input", { className: "order-number" + (Object.keys(numbers[key] || {}).some((name) => same(name, v)) ? " set" : ""), type: "text", inputMode: "numeric",
              value: String(counted[i]).padStart(pad, "0"), title: "Its number. Type another to skip some; the ones after count on from it. Clear it to count on from the one before.",
              onchange: (event) => renumber(i, event.target), onkeydown: (event) => { if (event.key === "Enter") { event.preventDefault(); event.target.blur(); } } }),
            el("span", { className: "order-value", textContent: v, title: v }),
            el("span", { className: "order-acts" },
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
  $("order").replaceChildren(...rows);
  $("order-row").hidden = !rows.length;
}
$("layout").addEventListener("click", suggest);
$("layout").addEventListener("blur", closeSuggest);
$("layout").addEventListener("keydown", (event) => {
  if ($("suggest").hidden) return;
  if (event.key === "ArrowDown" || event.key === "ArrowUp") {
    event.preventDefault();
    active = (active + (event.key === "ArrowDown" ? 1 : -1) + suggestions.length) % suggestions.length;
    suggest();
  } else if (event.key === "Enter" || event.key === "Tab") {
    event.preventDefault();
    accept(active);
  } else if (event.key === "Escape") {
    closeSuggest();
  }
});

function insert(text) {
  const input = $("layout");
  const start = input.selectionStart ?? input.value.length;
  const end = input.selectionEnd ?? start;
  input.value = input.value.slice(0, start) + text + input.value.slice(end);
  input.focus();
  input.setSelectionRange(start + text.length, start + text.length);
  changed();
}

