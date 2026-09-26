// Views: build a layout from keys and see, as it is typed, the tree an
// export of it would write. The server computes the plan; this draws it.
import { $, api, el, loadState, post, label, planNodes, templateOf, inputFor, fieldsOf, fieldsAt, frame, say, targetFolder, rememberTargetFolder } from "/common.js";
import { fileTree } from "/ui/filetree.js";
import { openFile } from "/ui/filedialog.js";

let state = { templates: [], items: [] };
let views = [];
let keys = [];
let editing = null; // the saved name of the View in the form, or "" for a new one

const blank = () => ({ name: "", query: {}, selection: "head", layout: "{owner}/{type}.{ext}" });

async function load() {
  state = await loadState();
  frame(state);
  const answer = await (await fetch(api("/api/views"))).json();
  views = answer.views;
  keys = answer.keys;
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
  const wanted = decodeURIComponent(location.hash.slice(1));
  // #new:<target> opens a new View going to that Target.
  if (wanted.startsWith("new:")) return open({ ...blank(), target: wanted.slice(4) }, "");
  const found = views.find((v) => v.name === wanted) || views[0];
  open(found || blank(), found ? found.name : "");
}

// The Views, grouped under the Target each names, each group with the folder
// it goes to this time and its own Export; Views naming none come last.
function list() {
  $("count").textContent = views.length;
  const row = (v) => el("li", { className: v.name === editing ? "selected" : "", onclick: () => open(v, v.name) },
    el("span", { className: "template-name" }, v.name),
    el("span", { className: "badge" }, v.selection === "all" ? "all" : "HEAD"),
    el("span", { className: "template-sub mono" }, v.layout));
  const names = [...new Set([...views.map((v) => v.target || ""), ...targets.map((t) => t.name)])]
    .sort((a, b) => (a === "") - (b === "") || a.localeCompare(b));
  const groups = names.map((name) => {
    const members = views.filter((v) => (v.target || "") === name);
    const known = targets.find((t) => t.name === name);
    const where = name && known ? folderFor(name) : "";
    return el("section", { className: "view-group-list" },
      el("div", { className: "group-head" },
        el("span", { className: "group-name", title: known && known.about ? known.about : "" }, name ? name : "No Target"),
        el("span", { className: "muted" }, name && !known ? "not in targets.yaml" : members.length + (members.length === 1 ? " View" : " Views")),
        name && known && members.length ? el("button", { type: "button", className: "button small-button", textContent: "Export…",
          title: "Check " + name + "'s Views together, then export them",
          onclick: () => exportTargets({ targets: [name] }, name) }) : null),
      name && known ? el("button", { type: "button", className: "target-folder" + (where ? "" : " unset"),
        title: (known.about ? known.about + "\n" : "") + "Where " + name + " goes this time. Click to choose another folder.",
        onclick: () => chooseFolder(name) },
        el("span", { className: "export-path" + (where ? "" : " muted") }, where ? "\u200e" + where + "\u200e" : "Choose a folder…"),
        where && where !== known.default ? el("span", { className: "target-flag" }, "this machine") : null) : null,
      el("ul", { className: "template-list" }, ...members.map(row)));
  });
  if (editing === "") groups.unshift(el("ul", { className: "template-list" }, el("li", { className: "selected" },
    el("span", { className: "template-name" }, "new view"), el("span", { className: "template-sub" }, "not saved yet"))));
  $("views").replaceChildren(...groups);
}

function open(v, name) {
  editing = name;
  history.replaceState(null, "", name ? "#" + encodeURIComponent(name) : location.pathname + location.search);
  $("name").value = v.name;
  const types = (v.query && v.query.type) || [];
  $("types").replaceChildren(...state.templates.map((t) => el("label", {},
    el("input", { type: "checkbox", value: t.type, checked: types.includes(t.type), onchange: changed }), " " + t.type)));
  $("conditions").replaceChildren(...Object.entries(v.query || {})
    .filter(([k]) => k !== "type").map(([k, values]) => condition(k, values.join(", "))));
  document.querySelector(`input[name=selection][value=${v.selection || "head"}]`).checked = true;
  $("layout").value = v.layout;
  $("use-default").checked = v.default !== undefined && v.default !== null;
  $("default").value = $("use-default").checked ? v.default : "none";
  $("dedupe").checked = v.dedupe === "number";
  drawTargets(v.target || "");
  $("delete").hidden = !name;
  closeSuggest();
  say($("message"), "");
  say($("export-message"), "");
  exportStale();
  chips();
  list();
  changed();
}

function condition(key, values) {
  const pick = el("select", { onchange: changed },
    ...keys.filter((k) => !["type", "revision", "ext", "id"].includes(k)).map((k) => el("option", { value: k, selected: k === key }, k)));
  const row = el("div", { className: "condition" }, pick, " is ",
    el("input", { value: values || "", placeholder: "jane, tom", spellcheck: false, autocomplete: "off", oninput: changed }),
    el("button", { type: "button", className: "tool", title: "Remove", textContent: "×", onclick: () => { row.remove(); changed(); } }));
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
  const fields = keys.filter((k) => !BUILT_IN[k]);
  const countries = [...countryKeys()];
  $("keys").replaceChildren(...[
    group("Fields", ...fields.map((k) => chip(k, "{" + k + "}", describe(k)))),
    group("Every PDF", ...keys.filter((k) => BUILT_IN[k]).map((k) => chip(k, "{" + k + "}", describe(k))), chip("/", "/", "a folder")),
    ...countries.map((k) => group(k + " as", ...Object.entries(FORMATS).map(([f, example]) =>
      chip(":" + f, "{" + k + ":" + f + "}", `{${k}:${f}} writes ${example}`)))),
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
    suggestions = countryKeys().has(key)
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

$("layout").addEventListener("input", () => { active = 0; suggest(); });
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

function current() {
  const query = {};
  const types = [...$("types").querySelectorAll("input:checked")].map((i) => i.value);
  if (types.length) query.type = types;
  for (const row of $("conditions").children) {
    const values = row.querySelector("input").value.split(",").map((s) => s.trim()).filter(Boolean);
    if (values.length) query[row.querySelector("select").value] = values;
  }
  const v = {
    name: $("name").value.trim(), query,
    selection: document.querySelector("input[name=selection]:checked").value,
    layout: $("layout").value.trim(),
  };
  if ($("use-default").checked) v.default = $("default").value;
  if ($("dedupe").checked) v.dedupe = "number";
  if ($("target").value) v.target = $("target").value;
  return v;
}

let asked = 0;
let timer = 0;
function changed() {
  clearTimeout(timer);
  timer = setTimeout(preview, 150);
}

async function preview() {
  const mine = ++asked;
  const v = current();
  let plan;
  try {
    plan = await post("/api/views/plan", { ...v, name: v.name || "preview" });
  } catch (error) {
    if (mine !== asked) return;
    $("summary").textContent = "";
    $("problems").replaceChildren(el("p", { className: "message error" }, error.message));
    $("tree").replaceChildren();
    return;
  }
  if (mine !== asked) return;
  const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
  const name = (id) => byId[id] ? label(state, byId[id]) : id;
  const link = (id) => el("a", { href: api("/browse/") + "#" + id }, name(id));
  $("summary").textContent = plan.files.length + (plan.files.length === 1 ? " file" : " files");
  const problems = [];
  if (plan.missing.length) {
    // One form per Item: its revisions share the fields that are missing.
    const byItem = new Map();
    for (const m of plan.missing) {
      const seen = byItem.get(m.item) || { keys: new Set(), fields: new Set(), digests: new Set() };
      seen.digests.add(m.digest);
      m.keys.forEach((k) => seen.keys.add(k));
      m.fields.forEach((f) => seen.fields.add(f));
      byItem.set(m.item, seen);
    }
    problems.push(el("div", { className: "problem" },
      el("strong", {}, plan.missing.length + " PDF" + (plan.missing.length === 1 ? " lacks" : "s lack") + " a key the layout uses"),
      byItem.size > 1 ? fillAll(byItem) : null,
      el("ul", { className: "fill-list" }, ...[...byItem].map(([id, m]) => fill(id, m, link)))));
  }
  if (plan.clashes.length) {
    problems.push(el("div", { className: "problem" },
      el("strong", {}, plan.clashes.length + " path" + (plan.clashes.length === 1 ? " is" : "s are") + " wanted by more than one PDF"),
      el("ul", {}, ...plan.clashes.map((c) => el("li", {}, el("span", { className: "mono" }, c.path), " — ",
        ...c.files.flatMap((f, i) => [i ? ", " : "", link(f.item)]))))));
  }
  if (!problems.length && plan.files.length) {
    problems.push(el("p", { className: "message ok" }, "Complete: every selected PDF has a path of its own."));
  }
  $("problems").replaceChildren(...problems);
  $("tree").replaceChildren(plan.files.length ? drawTree(plan.files, name) : el("p", { className: "muted" }, "The View selects no PDFs."));
}

// fill is one Item lacking keys, with a field to type each in. A key no
// field of its Template supplies is only named: the layout must change.
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
      state = await loadState();
      preview();
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
  const draw = () => {
    const selected = shared.find((w) => w.field.key === pick.value);
    const types = new Set(selected.items.map(({ item }) => item.type));
    slot.replaceChildren(inputFor(selected.field, "", "", state, undefined,
      types.size === 1 ? [...types][0] : undefined));
  };
  pick.onchange = draw;
  draw();
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
      state = await loadState();
      preview();
    } catch (error) {
      state = await loadState();
      say(message, error.message, true);
    }
  };
  return form;
}

// drawTree nests the plan's paths into folders, folders first.
function drawTree(files, name) {
  return fileTree(files, {
    href: (f) => api("/browse/") + "#" + f.item,
    fileExtra: (f) => el("span", {}, name(f.item)),
  });
}

$("form").addEventListener("input", changed);
$("add-condition").addEventListener("click", () => {
  $("conditions").append(condition(keys.find((k) => k === "owner") || keys[0], ""));
  changed();
});
$("new").addEventListener("click", () => open(blank(), ""));
$("form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const v = current();
  try {
    await post("/api/views", { view: v, previous: editing });
    say($("message"), "Saved views/" + v.name + ".yaml.");
    const answer = await (await fetch(api("/api/views"))).json();
    views = answer.views;
    editing = v.name;
    history.replaceState(null, "", "#" + encodeURIComponent(v.name));
    $("delete").hidden = false;
    await loadTargets();
  } catch (error) {
    say($("message"), error.message, true);
  }
});
$("delete").addEventListener("click", async () => {
  if (!editing || !confirm("Delete the View " + editing + "?\n\nIts file under views/ is removed; exports already written stay.")) return;
  try {
    await post("/api/views/delete", { name: editing });
    views = views.filter((v) => v.name !== editing);
    open(views[0] || blank(), views[0] ? views[0].name : "");
    await loadTargets();
  } catch (error) {
    say($("message"), error.message, true);
  }
});

// Export. A View with a Target is exported with every View naming that
// Target, into its folder; one without is exported alone into a folder
// chosen in the shared dialog. Either way the server checks the whole run —
// missing keys, clashes within and between Views, files in the way, Targets
// overlapping — and a run with any is not written.
let targets = [];
let folder = ""; // the folder chosen for a View without a Target
let planned = null; // the request the check shown was for
const FOLDER_KEY = "dgs-doc-export-folder";

async function loadTargets() {
  const answer = await (await fetch(api("/api/targets"))).json();
  targets = answer.targets || [];
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
  try { setFolder(folder || localStorage.getItem(FOLDER_KEY) || ""); } catch { setFolder(folder); }
  $("export-all").disabled = !targets.some((t) => t.views.length);
  drawTargets($("target").value);
  list();
}

// A Target goes to the folder last chosen for it on this machine, else its
// own folder from targets.yaml.
function folderFor(name) {
  const t = targets.find((x) => x.name === name);
  return t ? targetFolder(state, t) : "";
}
const chosenFolders = () => Object.fromEntries(targets.map((t) => [t.name, folderFor(t.name)]).filter(([, f]) => f));

async function chooseFolder(name) {
  const t = targets.find((x) => x.name === name);
  const chosen = await openFile({
    title: "Export " + name + " to",
    message: (t.about ? t.about + ". " : "") + "Choose the folder " + name + "'s Views are written into. An export removes only files it wrote there itself.",
    folders: true, writable: true, confirm: "Choose",
    folder: folderFor(name) || state.root, fallbacks: [state.root, ""],
  });
  if (!chosen) return;
  const path = Array.isArray(chosen) ? chosen[0] : chosen;
  rememberTargetFolder(state, name, path);
  if (path !== t.default && confirm("Make " + path + " " + name + "'s own folder, in the tree's targets.yaml?\n\nOK: every machine starts from it.\nCancel: only this browser remembers it.")) {
    await saveTargets(targets.map((x) => x.name === name ? { ...plain(x), folder: path } : plain(x)));
  }
  exportStale();
  list();
  targetNote();
}

const plain = (t) => ({ name: t.name, about: t.about || "", folder: t.folder || "" });
async function saveTargets(list) {
  try {
    const answer = await post("/api/targets", { targets: list });
    targets = answer.targets;
  } catch (error) {
    say($("message"), error.message, true);
  }
}

// drawTargets fills the Target select and says where the View goes.
function drawTargets(selected) {
  const names = targets.map((t) => t.name);
  if (selected && !names.includes(selected)) names.push(selected);
  $("target").replaceChildren(el("option", { value: "" }, "None: choose a folder when exporting"),
    ...names.map((n) => el("option", { value: n, selected: n === selected }, n)),
    el("option", { value: NEW_TARGET }, "+ New Target…"));
  $("target").value = selected;
  targetNote();
}
const NEW_TARGET = "\u0000new";

function targetNote() {
  const name = $("target").value;
  const t = targets.find((x) => x.name === name);
  const others = t ? t.views.filter((v) => v !== editing) : [];
  const where = t ? folderFor(name) : "";
  $("target-note").textContent = !name ? "Exported on its own, to a folder you pick."
    : !t ? name + " is not in the tree's targets.yaml."
    : (t.about ? t.about + " — " : "") + (where || "no folder yet: one is chosen when exporting") +
      (others.length ? " — exported together with " + others.join(", ") + "." : " — the only View there.");
  const saved = views.find((v) => v.name === editing);
  const target = saved && saved.target;
  $("export-folder-row").hidden = !!target;
  $("export-scope").textContent = target ? "with the Target " + target : "this View alone";
  $("dry-run").textContent = target ? "Check " + target : "Check";
  $("dry-run").disabled = target ? false : !folder;
}
$("target").addEventListener("change", async () => {
  if ($("target").value !== NEW_TARGET) return targetNote();
  const name = (prompt("A name for the new Target: lowercase letters, digits, _ and -.\nWhat it is for can be said next.") || "").trim();
  if (!name) return drawTargets("");
  const about = (prompt("What is " + name + " for? (optional)", "") || "").trim();
  await saveTargets([...targets.map(plain), { name, about, folder: "" }]);
  drawTargets(targets.some((t) => t.name === name) ? name : "");
  changed();
  if (targets.some((t) => t.name === name)) await chooseFolder(name);
});

function setFolder(path) {
  folder = path;
  $("folder").textContent = path ? "\u200e" + path + "\u200e" : "Choose folder…";
  $("folder").classList.toggle("muted", !path);
  $("choose").title = path || "Choose the folder to export to";
  try { if (path) localStorage.setItem(FOLDER_KEY, path); } catch { /* not kept */ }
  targetNote();
}

$("choose").addEventListener("click", async () => {
  const chosen = await openFile({
    title: "Export to",
    message: "Choose the folder the View is written into. An export removes only files it wrote there itself.",
    folders: true,
    writable: true,
    confirm: "Choose",
    folder: folder || state.root,
    fallbacks: [state.root, ""],
  });
  if (!chosen) return;
  setFolder(Array.isArray(chosen) ? chosen[0] : chosen);
  exportStale();
});

function exportStale() {
  planned = null;
  $("run").disabled = true;
  $("run").textContent = "Export";
  $("export-plan").replaceChildren();
  say($("export-message"), "");
}

function saved() {
  const v = views.find((x) => x.name === editing);
  return v && JSON.stringify(normal(v)) === JSON.stringify(normal(current()));
}
const normal = (v) => ({ name: v.name, query: v.query || {}, selection: v.selection || "head", layout: v.layout,
  default: v.default ?? null, dedupe: v.dedupe || "", target: v.target || "" });

// The Export button beside the form: the View's Target, or the View alone.
function dryRun() {
  if (!saved()) { exportStale(); say($("export-message"), "Save the View first: an export writes the saved Views.", true); return; }
  const v = views.find((x) => x.name === editing);
  if (v.target) exportTargets({ targets: [v.target] }, v.target);
  else if (!folder) say($("export-message"), "Choose a folder to export to first.", true);
  else exportTargets({ view: editing, folder }, "this View");
}

// exportTargets checks a run and shows it; Export writes it when nothing
// stops it.
async function exportTargets(request, what) {
  exportStale();
  $("export").scrollIntoView({ block: "nearest" });
  if (editing && !saved()) say($("export-message"), "The View in the form has unsaved changes: the saved one is checked.");
  $("export-scope").textContent = what === "this View" ? "this View alone" : what === "all" ? "every Target" : "with the Target " + what;
  say($("export-message"), "Checking…");
  let answer;
  try {
    answer = await post("/api/export/plan", { ...request, folders: chosenFolders() });
  } catch (error) {
    say($("export-message"), error.message, true);
    return;
  }
  drawPlan(answer);
  const changes = answer.jobs.reduce((n, j) => n + j.plan.add.length + j.plan.replace.length + j.plan.remove.length, 0);
  if (!answer.ready) say($("export-message"), "Not exported: fix what is listed below first. Nothing is written until everything checks.", true);
  else if (!changes) say($("export-message"), "Up to date: nothing to write.");
  else {
    say($("export-message"), "No conflicts. " + changes + " change" + (changes === 1 ? "" : "s") + " to write.");
    planned = request;
    $("run").disabled = false;
    $("run").textContent = "Export " + (what === "this View" ? "" : what === "all" ? "all" : what);
  }
}

// drawPlan shows each Target: its problems first, then what would change.
function drawPlan(answer) {
  $("export-plan").replaceChildren(...planNodes(state, answer));
}

async function run() {
  if (!planned) return;
  const request = planned;
  $("run").disabled = true;
  $("dry-run").disabled = true;
  say($("export-message"), "Exporting: checking again, then copying and reading back…");
  try {
    const answer = await post("/api/export", { ...request, folders: chosenFolders() });
    say($("export-message"), "Exported. " + answer.results.map((r) => (r.name || r.path) + ": " + r.result.written + " written, " +
      r.result.removed + " removed, " + r.result.kept + " unchanged" +
      (r.history_error ? " (history not recorded: " + r.history_error + ")" : "")).join("; ") + ".");
    $("export-plan").replaceChildren();
    await loadTargets();
  } catch (error) {
    say($("export-message"), error.message, true);
  } finally {
    planned = null;
    targetNote();
  }
}

$("dry-run").addEventListener("click", dryRun);
$("run").addEventListener("click", run);
$("export-all").addEventListener("click", () => exportTargets({ all: true }, "all"));
$("form").addEventListener("input", exportStale);

load().then(loadTargets);
