// Rules: making a rule — which PDFs it picks and the path each has there —
// with the tree it makes redrawn beside the form as it changes. A rule is
// the tree's: every Outline naming it, and every Snapshot taken from it
// after, sees the change. Outlines put rules and Snapshots together.
import { $, api, el, loadState, post, label, templateOf, inputFor, fieldsOf, fieldsAt, frame, say } from "/common.js";
import * as which from "/layoutform.js";
import { outlineTree, unplaced } from "/outlinetree.js";

// PREVIEW is the name the rule is planned under.
const PREVIEW = "preview";
let state = { templates: [], items: [] };
let rules = [];
let used = {}; // the Outlines using each rule
let editing = null; // the saved name of the rule shown, or "" for a new one
let draft = null; // the rule as the form has it
let saved = ""; // the rule as last loaded or saved, to tell an edit
let grouping = null; // the server's answer for the draft
const tree = outlineTree($("tree"), () => state, { empty: () => "Give the rule a path to see the tree." });

const copy = (o) => JSON.parse(JSON.stringify(o));
const text = (r) => JSON.stringify({ name: r.name, query: r.query || {}, selection: r.selection || "head", shared: !!r.shared, exclude: r.exclude || {}, query_types: r.query_types || {}, exclude_types: r.exclude_types || {}, skip: r.skip || [], layout: r.layout,
  default: r.default ?? null, dedupe: r.dedupe || "", order: r.order || null });
const blank = () => {
  let n = 1;
  while (rules.some((r) => r.name === "rule-" + n)) n++;
  return { name: "rule-" + n, query: {}, selection: "head", layout: "{owner}/{type}.{ext}" };
};

function list() {
  $("count").textContent = rules.length;
  const row = (name, sub, selected, onclick) => el("li", { className: selected ? "selected" : "", onclick },
    el("span", { className: "template-name" }, name), el("span", { className: "template-sub" }, sub));
  $("rules").replaceChildren(...rules.map((r) => row(r.name, (used[r.name] || []).length ? "in " + used[r.name].join(", ") : "in no Outline",
    r.name === editing, () => leave() && open(r.name))),
    ...(editing === "" ? [row("new rule", "not saved yet", true)] : []));
}

// sync takes the form into the draft.
function sync() {
  const { order, ...picked } = which.read();
  draft = { name: $("name").value.trim(), ...picked };
  if ($("use-default").checked) draft.default = $("default").value;
  if ($("dedupe").checked) draft.dedupe = "number";
  if (order) draft.order = order;
}

// leave asks before an unsaved edit is dropped.
function leave() {
  if (editing === null) return true;
  sync();
  return text(draft) === saved || confirm("Drop the unsaved changes to this rule?");
}

function open(name) {
  editing = name;
  const r = copy(rules.find((x) => x.name === name) || blank());
  $("name").value = r.name;
  which.fill(r);
  $("use-default").checked = r.default !== undefined && r.default !== null;
  $("default").value = $("use-default").checked ? r.default : "none";
  $("dedupe").checked = r.dedupe === "number";
  sync(); // the form's own reading, so an untouched rule is not an edit
  saved = text(draft);
  tree.reset();
  history.replaceState(null, "", name ? "#" + encodeURIComponent(name) : location.pathname + location.search);
  $("title").textContent = name || "New rule";
  $("delete").hidden = !name;
  $("take").hidden = !name;
  $("take").href = api("/snapshots/") + "#take=" + encodeURIComponent(name);
  const also = name ? used[name] || [] : [];
  $("rule-shared").hidden = !also.length;
  $("rule-shared").textContent = also.length ? "Used by " + also.join(", ") + ": saving changes it there too." : "";
  say($("message"), "");
  say($("form-message"), "");
  list();
  changed();
}

let pending = 0;
let asked = 0;
// changed regroups the draft a moment after the last change.
function changed() {
  sync();
  $("dirty").hidden = editing === null || text(draft) === saved;
  clearTimeout(pending);
  pending = setTimeout(regroup, 200);
}

async function regroup() {
  const mine = ++asked;
  if (!draft.layout) {
    grouping = null;
    return draw();
  }
  try {
    const answer = await post("/api/outlines/group", { name: PREVIEW, rules: [{ ...copy(draft), name: PREVIEW }] });
    if (mine !== asked) return;
    grouping = answer;
    say($("message"), "");
  } catch (err) {
    if (mine !== asked) return;
    grouping = null;
    say($("message"), err.message, true);
  }
  draw();
}

function draw() {
  const root = grouping && grouping.root;
  $("total").textContent = root ? root.count + (root.count === 1 ? " PDF" : " PDFs") : "";
  tree.show(grouping);
  problems();
}

// problems lists what the rule cannot place. A PDF lacking a field gets a
// form to fill it; a numbered value with no place in the rule's order gets
// a button to number it last.
function problems() {
  const lost = unplaced(grouping);
  if (!lost.length) return $("unplaced").replaceChildren();
  const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
  const link = (id) => el("a", { href: api("/browse/") + "#" + id }, byId[id] ? label(state, byId[id]) : id);
  const here = PREVIEW;
  const numbered = which.numberedKeys();
  const rows = [];
  const lacking = new Map(); // Item: the keys, fields and revisions it lacks
  for (const m of lost) {
    const li = el("li", {}, link(m.item));
    // A numbered key the Item has a value for lacks only a place in the
    // rule's order: that is fixed in the order, not in the Item.
    const unordered = m.keys && m.view === here
      ? m.keys.filter((k) => numbered.includes(k)).map((k) => [k, byId[m.item] && which.orderValue(byId[m.item], k)]).filter(([, v]) => v)
      : [];
    const absent = m.keys ? m.keys.filter((k) => !unordered.some(([u]) => u === k)) : [];
    const why = !m.keys ? m.why : [
      absent.length ? "lacks " + absent.join(", ") : "",
      ...unordered.map(([k, v]) => "has no number for " + k + " " + v),
    ].filter(Boolean).join("; ");
    li.append(" " + why + " ");
    for (const [key, value] of unordered) li.append(el("button", { type: "button", className: "small", textContent: "Number " + value + " last",
      onclick: () => which.numberLast(key, value) }));
    if (m.view === here) li.append(" ", el("button", { type: "button", className: "small", textContent: "Leave out",
      title: "Leave this Item out of the rule; it is listed under Leave out Items, to put back", onclick: () => which.skipItem(m.item) }));
    if (m.keys) {
      const keys = m.keys.filter((k) => !(m.view === here && numbered.includes(k) && byId[m.item] && which.orderValue(byId[m.item], k)));
      const fields = m.fields.filter((f) => !m.keys.includes(f) || keys.includes(f));
      if (fields.length) {
        const seen = lacking.get(m.item) || { keys: new Set(), fields: new Set(), digests: new Set() };
        seen.digests.add(m.digest);
        keys.forEach((k) => seen.keys.add(k));
        fields.forEach((f) => seen.fields.add(f));
        lacking.set(m.item, seen);
        continue;
      }
    }
    rows.push(li);
  }
  $("unplaced").replaceChildren(el("details", { className: "problem", open: lost.length <= 8 },
    el("summary", {}, el("strong", {}, lost.length + " PDF" + (lost.length === 1 ? " is" : "s are") + " not placed")),
    lacking.size > 1 ? fillAll(lacking) : null,
    el("ul", { className: "fill-list" }, ...rows, ...[...lacking].map(([id, m]) => fill(id, m, link)))));
}

async function refill() {
  state = await loadState();
  which.setup({ state });
  regroup();
}

// fill is one Item lacking keys, with a field to type each in. A key no
// field of its Template supplies is only named: the path must change.
function fill(id, m, link) {
  const item = state.items.find((i) => i.id === id);
  const t = item && templateOf(state, item.type);
  const known = t ? t.fields.filter((f) => m.fields.has(f.key)) : [];
  const unknown = [...m.fields].filter((f) => !known.some((k) => k.key === f));
  const message = el("span", { className: "message" });
  const form = el("form", { className: "fill" },
    ...known.map((f) => inputFor(f, "", "", state, id)),
    known.length ? el("button", { className: "small", type: "submit", textContent: "Save" }) : null,
    message);
  form.onsubmit = async (event) => {
    event.preventDefault();
    const typed = Object.fromEntries(Object.entries(fieldsOf(form)).filter(([, v]) => v !== ""));
    if (!Object.keys(typed).length) return;
    try {
      // Each lacking revision gets the value: a per_revision field is its own.
      for (const digest of m.digests) {
        await post("/api/fields", { item: id, digest, fields: { ...fieldsAt(item, digest), ...typed } });
      }
      await refill();
    } catch (error) {
      say(message, error.message, true);
    }
  };
  return el("li", {}, link(id), " — missing ", el("span", { className: "mono" }, [...m.keys].join(", ")),
    unknown.length ? el("span", { className: "muted" }, " · " + (item ? item.type : "its Template") + " has no field " +
      unknown.map((f) => f === "issued_at" ? "issued_at (year, month and date come from it)" : f).join(", ")) : null,
    " ", el("button", { type: "button", className: "small", textContent: "Leave out", title: "Leave this Item out of the rule instead",
      onclick: () => which.skipItem(id) }),
    form);
}

// fillAll sets one value on every listed Item that lacks the field and whose
// Template has it.
function fillAll(byItem) {
  const wanting = {};
  for (const [id, m] of byItem) {
    const item = state.items.find((i) => i.id === id);
    const t = item && templateOf(state, item.type);
    for (const f of t ? t.fields.filter((f) => m.fields.has(f.key)) : []) {
      (wanting[f.key] ||= { field: f, items: [] }).items.push({ item, digests: m.digests });
    }
  }
  const shared = Object.values(wanting).filter((w) => w.items.length > 1);
  if (!shared.length) return null;
  const message = el("span", { className: "message" });
  const pick = el("select", {}, ...shared.map((w) => el("option", { value: w.field.key }, w.field.key + " on " + w.items.length + " Items")));
  const slot = el("span");
  const drawSlot = () => {
    const selected = shared.find((w) => w.field.key === pick.value);
    const types = new Set(selected.items.map(({ item }) => item.type));
    slot.replaceChildren(inputFor(selected.field, "", "", state, undefined, types.size === 1 ? [...types][0] : undefined));
  };
  pick.onchange = drawSlot;
  drawSlot();
  const form = el("form", { className: "fill fill-all" }, el("span", {}, "Set "), pick, slot,
    el("button", { className: "small", type: "submit", textContent: "Set on all" }), message);
  form.onsubmit = async (event) => {
    event.preventDefault();
    const value = fieldsOf(form)[pick.value];
    if (!value) return;
    try {
      for (const { item, digests } of shared.find((w) => w.field.key === pick.value).items) {
        for (const digest of digests) {
          await post("/api/fields", { item: item.id, digest, fields: { ...fieldsAt(item, digest), [pick.value]: value } });
        }
      }
      await refill();
    } catch (error) {
      await refill();
      say(message, error.message, true);
    }
  };
  return form;
}

async function reload() {
  state = await loadState();
  frame(state);
  const answer = await (await fetch(api("/api/outlines"))).json();
  which.setup({ state, keys: answer.keys, countries: answer.countries || {}, onChange: changed });
  rules = answer.rules || [];
  used = answer.used || {};
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
  return answer;
}

$("form").addEventListener("input", (event) => { if (!event.target.closest(".fill")) changed(); });
$("form").addEventListener("submit", async (event) => {
  event.preventDefault();
  sync();
  try {
    await post("/api/rules", { rule: draft, previous: editing || "" });
    const name = draft.name;
    await reload();
    open(name);
    say($("form-message"), "Saved rules/" + name + ".yaml.");
  } catch (err) {
    say($("form-message"), err.message, true);
  }
});
$("new").onclick = () => leave() && open("");
$("delete").onclick = async () => {
  if (!editing) return;
  const also = used[editing] || [];
  if (also.length) return say($("form-message"), "Used by " + also.join(", ") + ": take it out of them on the Outlines page first.", true);
  if (!confirm("Delete the rule " + editing + "? Its file under rules/ is removed; no Item changes, and Snapshots taken from it stay.")) return;
  try {
    await post("/api/rules/delete", { name: editing });
    await reload();
    editing = null;
    open(rules.length ? rules[0].name : "");
  } catch (err) {
    say($("form-message"), err.message, true);
  }
};
window.addEventListener("beforeunload", (event) => { if (editing !== null) { sync(); if (text(draft) !== saved) event.preventDefault(); } });

reload().then(() => {
  const wanted = decodeURIComponent(location.hash.slice(1));
  open(wanted === "new" ? "" : rules.some((r) => r.name === wanted) ? wanted : rules.length ? rules[0].name : "");
}).catch((err) => {
  state.error = err.message;
  frame(state);
});
