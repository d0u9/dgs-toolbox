// The part of the Outlines form that makes one rule: types, conditions,
// revisions, a layout built from key chips, and a Numbering list per
// numbered key. The page owns the rest of
// its form and what a change redraws; the server computes the result.
import { $, el, currentFields } from "/common.js";

let state = { templates: [], items: [] };
let keys = [];
let countries = {}; // each country's every form: its alpha-3 code
let orders = {}; // per numbered key, its order as typed: "alex, emma"
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
    .filter(([k]) => k !== "type").map(([k, values]) => condition(k, values)));
  document.querySelector(`input[name=selection][value=${v.selection || "head"}]`).checked = true;
  $("layout").value = v.layout;
  orders = Object.fromEntries(Object.entries(v.order || {}).map(([k, list]) => [k, list.join(", ")]));
  drawOrder();
  closeSuggest();
  chips();
}

// read is the form's query, selection, layout and order.
export function read() {
  const query = {};
  const types = [...$("types").querySelectorAll("input:checked")].map((i) => i.value);
  if (types.length) query.type = types;
  for (const row of $("conditions").children) {
    const values = [...row.querySelectorAll(".checks input:checked")].map((i) => i.value);
    if (values.length) query[row.querySelector("select").value] = values;
  }
  const out = { query, selection: document.querySelector("input[name=selection]:checked").value, layout: $("layout").value.trim() };
  const order = {};
  for (const key of numberedKeys()) {
    const list = (orders[key] || "").split(",").map((s) => s.trim()).filter(Boolean);
    if (list.length) order[key] = list;
  }
  if (Object.keys(order).length) out.order = order;
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

// same reports whether a and b are one value: one country however each is
// written, or else equal ignoring case.
const same = (a, b) => (countries[a] && countries[a] === countries[b]) || a.toLowerCase() === b.toLowerCase();

// chosenTypes is the types ticked, or every type when none is: a rule
// without types selects them all.
function chosenTypes() {
  const ticked = [...$("types").querySelectorAll("input:checked")].map((i) => i.value);
  return ticked.length ? ticked : state.templates.map((t) => t.type);
}

// narrow redraws what depends on the types: the field chips and each
// condition's values, keeping only what the chosen types have.
function narrow() {
  chips();
  for (const row of $("conditions").children) row.redraw();
  drawOrder();
  changed();
}

// condition is one query key and the values it accepts, picked from those
// the Items hold, so a value is never typed in a form no Item uses. A saved
// value is ticked as the held value it is one with — CHN as 中国 — and one
// no Item holds any more is still listed, ticked.
function condition(key, values) {
  const choices = el("div", { className: "checks" });
  const draw = (saved) => {
    const types = chosenTypes();
    const held = [...new Set(state.items.filter((item) => types.includes(item.type)).map((item) => currentFields(item)[pick.value]).filter(Boolean))];
    const all = [...held, ...saved.filter((v) => !held.some((h) => same(h, v)))].sort((a, b) => a.localeCompare(b));
    choices.replaceChildren(...(all.length ? all.map((v) => el("label", {},
      el("input", { type: "checkbox", value: v, checked: saved.some((s) => same(s, v)), onchange: changed }), " " + v))
      : [el("span", { className: "template-sub", textContent: "no Item has one" })]));
  };
  const pick = el("select", { onchange: () => { draw([]); changed(); } },
    ...keys.filter((k) => !["type", "revision", "ext", "id"].includes(k)).map((k) => el("option", { value: k, selected: k === key }, k)));
  const row = el("div", { className: "condition" },
    el("div", { className: "condition-head" }, pick,
      el("button", { type: "button", className: "tool", title: "Remove", textContent: "×", onclick: () => { row.remove(); changed(); } })),
    choices);
  row.redraw = () => draw([...choices.querySelectorAll("input:checked")].map((i) => i.value));
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
    group("Every PDF", ...keys.filter((k) => BUILT_IN[k]).map((k) => chip(k, "{" + k + "}", describe(k))), chip("/", "/", "a folder")),
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

// The orders the layout numbers from, each once: a key written {key#},
// {key#:format} or {key}#, or alternatives written {a|b}#, named a|b.
export const numberedKeys = () => [...new Set([...$("layout").value.matchAll(/\{([^{}]*)\}(#?)/g)].flatMap(([, inner, after]) => {
  if (inner.includes("|")) return after ? [inner] : [];
  const [, key, hash] = /^([a-z0-9_-]+)(#?)/.exec(inner) || [];
  return key && (hash || after) ? [key] : [];
}))];

// An Item's value for one order: the first alternative it has. A type:zh or
// type:en is its Template's name.
export function orderValue(item, key) {
  for (const choice of key.split("|")) {
    const [name, format] = choice.split(":");
    const value = name === "type"
      ? (format ? (state.templates.find((t) => t.type === item.type)?.names || {})[format] : item.type)
      : item.fields && item.fields[name];
    if (value) return value;
  }
  return "";
}

// One row per order: the values it may have, in the order they are numbered
// from 01. An order new to the layout starts with the values Items have.
function drawOrder() {
  const rows = numberedKeys().map((key) => {
    if (orders[key] === undefined) {
      orders[key] = [...new Set(state.items.map((i) => orderValue(i, key)).filter(Boolean))].sort().join(", ");
    }
    const box = el("div", { className: "order" });
    const list = () => orders[key].split(",").map((s) => s.trim()).filter(Boolean);
    const set = (values) => { orders[key] = values.join(", "); draw(); changed(); };
    let dragged = -1;
    const draw = () => {
      const values = list();
      const unlisted = [...new Set(state.items.filter((i) => chosenTypes().includes(i.type)).map((i) => orderValue(i, key)).filter(Boolean))]
        .filter((v) => !values.some((w) => same(v, w))).sort();
      const move = (i, j) => { const next = [...values]; next.splice(j, 0, ...next.splice(i, 1)); set(next); };
      const act = (title, text, onclick, disabled) => el("button", { type: "button", className: "order-act", title, textContent: text, disabled, onclick });
      box.replaceChildren(...[
        el("ol", { className: "order-list" }, ...values.map((v, i) => {
          const li = el("li", {
            draggable: true,
            ondragstart: (event) => { dragged = i; event.dataTransfer.effectAllowed = "move"; li.classList.add("dragging"); },
            ondragend: () => li.classList.remove("dragging"),
            ondragover: (event) => { event.preventDefault(); li.classList.add("drop"); },
            ondragleave: () => li.classList.remove("drop"),
            ondrop: (event) => { event.preventDefault(); li.classList.remove("drop"); if (dragged >= 0 && dragged !== i) move(dragged, i); dragged = -1; },
          },
            el("span", { className: "order-grip", textContent: "⋮⋮", "aria-hidden": "true" }),
            el("span", { className: "order-number", textContent: String(i + 1).padStart(2, "0") }),
            el("span", { className: "order-value", textContent: v, title: v }),
            el("span", { className: "order-acts" },
              act("Up", "↑", () => move(i, i - 1), i === 0),
              act("Down", "↓", () => move(i, i + 1), i === values.length - 1),
              act("Remove", "×", () => set(values.filter((_, k) => k !== i)))));
          return li;
        })),
        unlisted.length ? el("div", { className: "order-add" }, el("span", { className: "order-add-label", textContent: "Not numbered" }),
          ...unlisted.map((v) => el("button", { type: "button", className: "order-chip", textContent: "+ " + v, title: "Number it last", onclick: () => set([...values, v]) })))
          : null,
      ].filter(Boolean));
    };
    draw();
    return el("div", { className: "condition" }, el("code", {}, key.includes("|") ? "{" + key + "}#" : "{" + key + "#}"), box);
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

