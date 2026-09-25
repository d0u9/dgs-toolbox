// Views: build a layout from keys and see, as it is typed, the tree an
// export of it would write. The server computes the plan; this draws it.
import { $, el, loadState, post, label, templateOf, inputFor, fieldsOf, fieldsAt, frame, say } from "/common.js";
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
  const answer = await (await fetch("/api/views")).json();
  views = answer.views;
  keys = answer.keys;
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
  const wanted = decodeURIComponent(location.hash.slice(1));
  const found = views.find((v) => v.name === wanted) || views[0];
  open(found || blank(), found ? found.name : "");
}

function list() {
  $("count").textContent = views.length;
  $("views").replaceChildren(...views.map((v) => el("li", { className: v.name === editing ? "selected" : "", onclick: () => open(v, v.name) },
    el("span", { className: "template-name" }, v.name),
    el("span", { className: "badge" }, v.selection === "all" ? "all" : "HEAD"),
    el("span", { className: "template-sub mono" }, v.layout))),
  ...(editing === "" ? [el("li", { className: "selected" }, el("span", { className: "template-name" }, "new view"),
    el("span", { className: "template-sub" }, "not saved yet"))] : []));
}

function open(v, name) {
  editing = name;
  history.replaceState(null, "", name ? "#" + encodeURIComponent(name) : location.pathname);
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
  const link = (id) => el("a", { href: "/browse/#" + id }, name(id));
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
  const draw = () => slot.replaceChildren(inputFor(shared.find((w) => w.field.key === pick.value).field, "", "", state));
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
    href: (f) => "/browse/#" + f.item,
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
    const answer = await (await fetch("/api/views")).json();
    views = answer.views;
    editing = v.name;
    history.replaceState(null, "", "#" + encodeURIComponent(v.name));
    $("delete").hidden = false;
    list();
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
  } catch (error) {
    say($("message"), error.message, true);
  }
});

// Export: a dry run of the saved View into a folder, then, when nothing
// blocks it, the export itself, which the server plans again. The folder is
// chosen in the shared dialog; the configured Targets are shortcuts to theirs.
let targets = [];
let folder = ""; // the folder chosen, or "" for none yet
let planned = null; // the request the dry run shown was for
const FOLDER_KEY = "dgs-doc-export-folder";

async function loadTargets() {
  targets = await (await fetch("/api/targets")).json();
  if (!folder) {
    let kept = "";
    try { kept = localStorage.getItem(FOLDER_KEY) || ""; } catch { /* not kept */ }
    setFolder(kept);
  }
  $("targets").replaceChildren(...(targets.length ? [el("span", { className: "muted" }, "Targets"),
    ...targets.map((t) => el("button", {
      type: "button", className: "chip" + (t.path === folder ? " on" : ""), title: t.path + (t.view ? " — last: " + t.view : "") + (t.error ? " — " + t.error : ""),
      textContent: t.name, onclick: () => { setFolder(t.path); exportStale(); },
    }))] : []));
}

function setFolder(path) {
  folder = path;
  $("folder").textContent = path ? "\u200e" + path + "\u200e" : "Choose folder…";
  $("folder").classList.toggle("muted", !path);
  $("choose").title = path || "Choose the folder to export to";
  $("dry-run").disabled = !path;
  try { if (path) localStorage.setItem(FOLDER_KEY, path); } catch { /* not kept */ }
  $("targets").querySelectorAll(".chip").forEach((chip, i) => chip.classList.toggle("on", targets[i]?.path === path));
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
  $("export-plan").replaceChildren();
}

function saved() {
  const v = views.find((x) => x.name === editing);
  return v && JSON.stringify(normal(v)) === JSON.stringify(normal(current()));
}
const normal = (v) => ({ name: v.name, query: v.query || {}, selection: v.selection || "head", layout: v.layout,
  default: v.default ?? null, dedupe: v.dedupe || "" });

async function dryRun() {
  exportStale();
  if (!saved()) { say($("export-message"), "Save the View first: an export writes the saved View.", true); return; }
  if (!folder) { say($("export-message"), "Choose a folder to export to first.", true); return; }
  say($("export-message"), "Reading the folder…");
  let plan;
  try {
    plan = await post("/api/export/plan", { view: editing, folder });
  } catch (error) {
    say($("export-message"), error.message, true);
    return;
  }
  const incomplete = plan.view.missing.length + plan.view.clashes.length;
  const changes = plan.add.length + plan.replace.length + plan.remove.length;
  const section = (title, list, why) => list.length ? el("details", { open: list.length <= 20 },
    el("summary", {}, title + " (" + list.length + ")"),
    el("ul", {}, ...list.map((a) => el("li", { className: "mono" }, a.path, why && a.reason ? el("span", { className: "muted" }, " — " + a.reason) : "")))) : null;
  $("export-plan").replaceChildren(...[
    section("Blocked", plan.blocked, true), section("Add", plan.add), section("Replace", plan.replace),
    section("Remove", plan.remove), section("Left as they are, and forgotten", plan.left, true),
    plan.keep.length ? el("p", { className: "muted" }, plan.keep.length + " already there and unchanged") : null,
  ].filter(Boolean));
  if (incomplete) say($("export-message"), "The View is not complete (above); nothing can be exported yet.", true);
  else if (plan.blocked.length) say($("export-message"), "Some paths are taken by files this export did not write; move them first.", true);
  else if (!changes) say($("export-message"), plan.target + " is up to date.");
  else {
    say($("export-message"), changes + " change" + (changes === 1 ? "" : "s") + " to " + plan.target + ".");
    planned = { view: editing, folder };
    $("run").disabled = false;
  }
}

async function run() {
  if (!planned) return;
  $("run").disabled = true;
  $("dry-run").disabled = true;
  say($("export-message"), "Exporting: copying and reading back…");
  try {
    const result = await post("/api/export", planned);
    say($("export-message"), "Exported: " + result.written + " written, " + result.removed + " removed, " + result.kept + " unchanged.");
    $("export-plan").replaceChildren();
    await loadTargets();
  } catch (error) {
    say($("export-message"), error.message, true);
  } finally {
    planned = null;
    $("dry-run").disabled = !folder;
  }
}

$("dry-run").addEventListener("click", dryRun);
$("run").addEventListener("click", run);
$("form").addEventListener("input", exportStale);

load().then(loadTargets);
