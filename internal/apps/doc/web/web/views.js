// Views: build a layout from keys and see, as it is typed, the tree an
// export of it would write. The server computes the plan; this draws it.
import { $, el, loadState, post, label, frame, say } from "/common.js";

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
  $("views").replaceChildren(...views.map((v) => {
    const li = el("li", { onclick: () => open(v, v.name) }, v.name, el("span", { className: "sub mono" }, v.layout));
    if (v.name === editing) li.className = "selected";
    return li;
  }));
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
  say($("message"), "");
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

// The keys a layout can use, as chips that insert themselves at the caret.
function chips() {
  $("keys").replaceChildren(el("span", { className: "muted" }, "Insert "), ...keys.map((k) => el("button", {
    type: "button", className: "chip", textContent: k,
    onmousedown: (event) => event.preventDefault(),
    onclick: () => insert("{" + k + "}"),
  })), el("button", { type: "button", className: "chip", textContent: "/", onmousedown: (e) => e.preventDefault(), onclick: () => insert("/") }));
}

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
    problems.push(el("div", { className: "problem" },
      el("strong", {}, plan.missing.length + " PDF" + (plan.missing.length === 1 ? " lacks" : "s lack") + " a key the layout uses"),
      el("ul", {}, ...plan.missing.map((m) => el("li", {}, link(m.item),
        m.revision > 1 || v.selection === "all" ? " · revision " + m.revision : "",
        " — missing ", el("span", { className: "mono" }, m.keys.join(", ")))))));
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

// drawTree nests the plan's paths into folders, folders first.
function drawTree(files, name) {
  const root = { dirs: {}, files: [] };
  for (const f of files) {
    const parts = f.path.split("/");
    let node = root;
    for (const dir of parts.slice(0, -1)) node = node.dirs[dir] ||= { dirs: {}, files: [] };
    node.files.push({ leaf: parts[parts.length - 1], file: f });
  }
  const draw = (node) => el("ul", {},
    ...Object.keys(node.dirs).sort().map((d) => el("li", { className: "dir" },
      el("details", { open: true }, el("summary", {}, d + "/"), draw(node.dirs[d])))),
    ...node.files.map(({ leaf, file }) => el("li", { className: "file" },
      el("a", { href: "/browse/#" + file.item, title: "Open in Browse" }, leaf),
      el("span", { className: "sub-inline" }, name(file.item)))));
  return draw(root);
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
  if (!editing || !confirm("Delete the View " + editing + "? Its file under views/ is removed.")) return;
  try {
    await post("/api/views/delete", { name: editing });
    views = views.filter((v) => v.name !== editing);
    open(views[0] || blank(), views[0] ? views[0].name : "");
  } catch (error) {
    say($("message"), error.message, true);
  }
});

load();
