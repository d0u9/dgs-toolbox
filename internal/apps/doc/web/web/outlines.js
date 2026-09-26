// Outlines: making an Outline — its name, the folder it is exported to, and
// its rules, each picking PDFs and naming the path each has — with the tree
// the rules make redrawn beside the form as it changes. A rule is the
// tree's, not the Outline's: one already made is added as it is, and
// editing it changes it in every Outline using it. Looking through the
// result and exporting it is the Explore page's.
import { $, api, el, loadState, post, label, templateOf, inputFor, fieldsOf, fieldsAt, frame, say } from "/common.js";
import { openFile } from "/ui/filedialog.js";
import * as which from "/layoutform.js";
import { outlineTree, unplaced } from "/outlinetree.js";

let state = { templates: [], items: [] };
let outlines = [];
let rules = []; // every rule in the tree
let used = {}; // the Outlines using each rule, as saved
let origins = []; // each draft rule's saved name, or "" for one made here
let editing = null; // the saved name of the Outline shown, or "" for a new one
let draft = null; // the Outline as the form has it, every rule included
let rule = 0; // the rule in the form
let saved = ""; // the Outline as last loaded or saved, to tell an edit
let grouping = null; // the server's answer for the draft
const tree = outlineTree($("tree"), () => state, { empty: () => "Add a rule with a path to see the tree." });

const newRule = (n) => ({ name: "rule-" + n, query: {}, selection: "head", layout: "{owner}/{type}.{ext}" });
const blank = () => {
  let n = 1;
  while (rules.some((r) => r.name === "rule-" + n)) n++;
  return { name: "", about: "", folder: "", rules: [newRule(n)] };
};
const copy = (o) => JSON.parse(JSON.stringify(o));
const text = (o) => JSON.stringify({ name: o.name, about: o.about || "", folder: o.folder || "",
  rules: (o.rules || []).map((r) => ({ name: r.name, query: r.query || {}, selection: r.selection || "head", layout: r.layout,
    default: r.default ?? null, dedupe: r.dedupe || "", order: r.order || null })) });

function list() {
  $("count").textContent = outlines.length;
  const row = (name, sub, selected, onclick) => el("li", { className: selected ? "selected" : "", onclick },
    el("span", { className: "template-name" }, name), el("span", { className: "template-sub" }, sub));
  $("outlines").replaceChildren(...outlines.map((o) => row(o.name, o.about || o.rules.map((r) => r.name).join(", "), o.name === editing, () => leave() && open(o.name))),
    ...(editing === "" ? [row("new outline", "not saved yet", true)] : []));
}

// readRule is the rule in the form.
function readRule() {
  const { order, ...picked } = which.read();
  const r = { name: $("rule-name").value.trim(), ...picked };
  if ($("use-default").checked) r.default = $("default").value;
  if ($("dedupe").checked) r.dedupe = "number";
  if (order) r.order = order;
  return r;
}

// sync takes the form into the draft.
function sync() {
  draft.name = $("name").value.trim();
  draft.about = $("about").value.trim();
  if (draft.rules.length) draft.rules[rule] = readRule();
}

function fillRule() {
  const r = draft.rules[rule];
  const on = !!r;
  for (const id of ["rule-name", "rule-delete", "layout", "add-condition", "use-default", "default", "dedupe"]) $(id).disabled = !on;
  const shown = r || newRule(1);
  $("rule-name").value = on ? r.name : "";
  which.fill(shown);
  $("use-default").checked = on && r.default !== undefined && r.default !== null;
  $("default").value = $("use-default").checked ? r.default : "none";
  $("dedupe").checked = on && r.dedupe === "number";
  tabs();
}

function tabs() {
  const others = rules.filter((r) => !origins.includes(r.name));
  const existing = el("select", { className: "chip", title: "Add a rule another Outline uses, or one no Outline uses now",
    onchange: () => {
      const r = rules.find((x) => x.name === existing.value);
      if (!r) return;
      sync();
      draft.rules.push(copy(r));
      origins.push(r.name);
      pickRule(draft.rules.length - 1);
      changed();
    } }, el("option", { value: "" }, "+ Existing rule…"),
    ...others.map((r) => el("option", { value: r.name }, r.name + ((used[r.name] || []).length ? " · " + used[r.name].join(", ") : " · unused"))));
  existing.hidden = !others.length;
  const unused = others.filter((r) => !(used[r.name] || []).length);
  $("rules").replaceChildren(...draft.rules.map((r, i) => el("button", { type: "button", className: "chip" + (i === rule ? " on" : ""),
    textContent: r.name || "unnamed", title: r.layout, onclick: () => pickRule(i) })),
    el("button", { type: "button", className: "chip", textContent: "+ Rule", title: "Make a new rule: more PDFs in the same tree",
      onclick: () => {
        sync();
        let n = draft.rules.length + 1;
        while (draft.rules.some((r) => r.name === "rule-" + n) || rules.some((r) => r.name === "rule-" + n)) n++;
        draft.rules.push(newRule(n));
        origins.push("");
        pickRule(draft.rules.length - 1);
        changed();
      } }),
    existing,
    ...(unused.length ? [el("select", { className: "chip", title: "Delete a rule no Outline uses",
      onchange: (event) => dropUnused(event.target) }, el("option", { value: "" }, "Delete unused rule…"),
      ...unused.map((r) => el("option", { value: r.name }, r.name)))] : []));
  shared();
}

// shared says which other saved Outlines the rule shown is in: an edit
// saved here changes it there too.
function shared() {
  const from = origins[rule];
  const also = from ? (used[from] || []).filter((o) => o !== editing) : [];
  $("rule-shared").hidden = !also.length;
  $("rule-shared").textContent = also.length ? "Also used by " + also.join(", ") + ": saving changes it there too." : "";
}

async function dropUnused(select) {
  const name = select.value;
  select.value = "";
  if (!name || !confirm("Delete the rule " + name + "? No Outline uses it; its file under rules/ is removed.")) return;
  try {
    await post("/api/rules/delete", { name });
    const answer = await (await fetch(api("/api/outlines"))).json();
    rules = answer.rules;
    used = answer.used;
    tabs();
  } catch (err) {
    say($("form-message"), err.message, true);
  }
}

function pickRule(i) {
  sync();
  rule = i;
  fillRule();
  problems();
}

function folderShown() {
  $("folder-path").textContent = draft.folder ? "‎" + draft.folder + "‎" : "None: chosen when exporting";
  $("folder-path").classList.toggle("muted", !draft.folder);
  $("folder-clear").disabled = !draft.folder;
}

// leave asks before an unsaved edit is dropped.
function leave() {
  if (editing === null) return true;
  sync();
  return text(draft) === saved || confirm("Drop the unsaved changes to this Outline?");
}

function open(name) {
  editing = name;
  draft = copy(outlines.find((x) => x.name === name) || blank());
  delete draft.default;
  origins = draft.rules.map((r) => name ? r.name : "");
  rule = 0;
  $("name").value = draft.name;
  $("about").value = draft.about || "";
  folderShown();
  fillRule();
  sync(); // the form's own reading, so an untouched Outline is not an edit
  saved = text(draft);
  tree.reset();
  history.replaceState(null, "", name ? "#" + encodeURIComponent(name) : location.pathname + location.search);
  $("title").textContent = name || "New Outline";
  $("delete").hidden = !name;
  $("explore").hidden = !name;
  $("explore").href = api("/explore/") + "#" + encodeURIComponent(name);
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
  tabs();
  $("dirty").hidden = editing === null || text(draft) === saved;
  clearTimeout(pending);
  pending = setTimeout(regroup, 200);
}

async function regroup() {
  const mine = ++asked;
  const o = { ...copy(draft), name: draft.name || "preview" };
  o.rules = o.rules.filter((r) => r.layout);
  if (!o.rules.length) {
    grouping = null;
    return draw();
  }
  try {
    const answer = await post("/api/outlines/group", o);
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

// problems lists what the rules cannot place. A PDF lacking a field gets a
// form to fill it; a numbered value with no place in the rule's order gets
// a button to number it last; a PDF of another rule switches to that rule.
function problems() {
  const lost = unplaced(grouping);
  if (!lost.length) return $("unplaced").replaceChildren();
  const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
  const link = (id) => el("a", { href: api("/browse/") + "#" + id }, byId[id] ? label(state, byId[id]) : id);
  const here = draft.rules[rule] && draft.rules[rule].name;
  const numbered = which.numberedKeys();
  const rows = [];
  const lacking = new Map(); // Item: the keys, fields and revisions it lacks
  for (const m of lost) {
    const li = el("li", {}, m.view && m.view !== here ? el("button", { type: "button", className: "small", textContent: m.view,
      title: "Show this rule", onclick: () => pickRule(draft.rules.findIndex((r) => r.name === m.view)) }) : null,
      m.view && m.view !== here ? " " : null, link(m.item), " " + m.why + " ");
    if (m.keys && m.view === here) {
      for (const key of m.keys.filter((k) => numbered.includes(k))) {
        const value = byId[m.item] && which.orderValue(byId[m.item], key);
        if (value) li.append(el("button", { type: "button", className: "small", textContent: "Number " + value + " last",
          onclick: () => which.numberLast(key, value) }));
      }
    }
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
  outlines = answer.outlines;
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
    const fresh = draft.rules.filter((r, i) => !origins[i]).map((r) => r.name);
    const renamed = Object.fromEntries(draft.rules.map((r, i) => [origins[i], r.name]).filter(([from, to]) => from && from !== to));
    await post("/api/outlines", { outline: draft, previous: editing || "", fresh, renamed });
    const name = draft.name;
    await reload();
    open(name);
    say($("form-message"), "Saved outlines/" + name + ".yaml.");
  } catch (err) {
    say($("form-message"), err.message, true);
  }
});
$("rule-delete").onclick = () => {
  sync();
  const r = draft.rules[rule];
  if (!r || !confirm("Remove the rule " + (r.name || "unnamed") + " from this Outline? Other Outlines keep it; one made here and not saved is gone.")) return;
  draft.rules.splice(rule, 1);
  origins.splice(rule, 1);
  rule = Math.max(0, rule - 1);
  fillRule();
  changed();
};
$("folder").onclick = async () => {
  const chosen = await openFile({ title: "Export " + (draft.name || "the Outline") + " to", folders: true, writable: true, confirm: "Choose",
    message: "The folder every machine exports this Outline to unless another is chosen on Explore. It is kept in the Outline's file.",
    folder: draft.folder || state.root, fallbacks: [state.root, ""] });
  if (!chosen) return;
  draft.folder = Array.isArray(chosen) ? chosen[0] : chosen;
  folderShown();
  changed();
};
$("folder-clear").onclick = () => { draft.folder = ""; folderShown(); changed(); };
$("new").onclick = () => leave() && open("");
$("delete").onclick = async () => {
  if (!editing || !confirm("Delete the Outline " + editing + "? Its file under outlines/ is removed; no Item changes, and nothing exported is touched.")) return;
  try {
    await post("/api/outlines/delete", { name: editing });
    await reload();
    editing = null;
    open(outlines.length ? outlines[0].name : "");
  } catch (err) {
    say($("form-message"), err.message, true);
  }
};
window.addEventListener("beforeunload", (event) => { if (editing !== null) { sync(); if (text(draft) !== saved) event.preventDefault(); } });

reload().then((answer) => {
  const wanted = decodeURIComponent(location.hash.slice(1));
  open(wanted === "new" ? "" : outlines.some((o) => o.name === wanted) ? wanted : outlines.length ? outlines[0].name : "");
  if (answer.migrated && answer.migrated.length) say($("message"), "Outlines brought up to date: " + answer.migrated.join(", ") + ". Their rules are under rules/; old Views and Targets, if any, are in migrated/.");
}).catch((err) => {
  state.error = err.message;
  frame(state);
});
