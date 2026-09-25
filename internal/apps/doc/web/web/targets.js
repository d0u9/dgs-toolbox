// Targets: where this tree's Views are exported to. The tree keeps them in
// targets.yaml; the server writes it and each View. The page adds, renames and
// deletes Targets, picks their folders, moves Views to them and exports.
import { $, api, el, loadState, post, planNodes, frame, say, targetFolder, rememberTargetFolder } from "/common.js";
import { openFile } from "/ui/filedialog.js";
import { guardByName } from "/ui/confirm.js";

let state = { templates: [], items: [] };
let targets = [];
let views = [];
let current = null; // the name of the Target shown
let planned = null;
const armDelete = guardByName($("confirm"), $("delete"), "");

const plain = (t) => ({ name: t.name, about: t.about || "", folder: t.folder || "" });
const shown = () => targets.find((t) => t.name === current);

async function load(wanted) {
  state = await loadState();
  frame(state);
  const [t, v] = await Promise.all([fetch(api("/api/targets")).then((r) => r.json()), fetch(api("/api/views")).then((r) => r.json())]);
  targets = t.targets || [];
  views = v.views || [];
  const problems = [t.error, v.error].filter(Boolean);
  $("error").hidden = !problems.length;
  $("error").textContent = problems.join("; ");
  const name = wanted ?? decodeURIComponent(location.hash.slice(1));
  const found = targets.find((x) => x.name === name) || targets[0];
  if (found) show(found.name);
  else showNew();
}

function list() {
  $("count").textContent = targets.length;
  $("export-all").disabled = !targets.some((t) => t.views.length);
  $("targets").replaceChildren(...targets.map((t) => {
    const where = targetFolder(state, t);
    return el("li", { className: t.name === current ? "selected" : "", onclick: () => show(t.name) },
      el("span", { className: "template-name" }, t.name),
      el("span", { className: "badge" }, t.views.length + (t.views.length === 1 ? " View" : " Views")),
      el("span", { className: "template-sub" }, t.about || ""),
      el("span", { className: "template-sub mono" + (where ? "" : " gone") }, where || "no folder: chosen when exporting"));
  }));
}

function showNew() {
  current = null;
  history.replaceState(null, "", location.pathname + location.search);
  $("target").hidden = true;
  $("new-form").hidden = false;
  $("new-name").value = "";
  $("new-about").value = "";
  say($("new-message"), "");
  list();
  $("new-name").focus();
}
$("new").addEventListener("click", showNew);

// save writes the whole list and answers whether it was taken.
async function save(list, done) {
  try {
    const answer = await post("/api/targets", { targets: list });
    targets = answer.targets;
    if (done) say($("message"), done);
    return true;
  } catch (error) {
    say($("message"), error.message, true);
    return false;
  }
}

$("new-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const name = $("new-name").value.trim();
  if (targets.some((t) => t.name === name)) { say($("new-message"), "There is a Target named " + name + " already.", true); return; }
  try {
    const answer = await post("/api/targets", { targets: [...targets.map(plain), { name, about: $("new-about").value.trim(), folder: "" }] });
    targets = answer.targets;
    show(name);
    say($("message"), "Added. Choose its own folder below, or leave it empty to choose one each time.");
  } catch (error) {
    say($("new-message"), error.message, true);
  }
});

const pathText = (node, path, empty) => {
  node.textContent = path ? "‎" + path + "‎" : empty;
  node.classList.toggle("muted", !path);
};

function show(name) {
  current = name;
  const t = shown();
  history.replaceState(null, "", location.pathname + location.search + "#" + encodeURIComponent(name));
  $("new-form").hidden = true;
  $("target").hidden = false;
  $("name").value = t.name;
  $("about").value = t.about || "";
  pathText($("default-path"), t.folder || "", "None: chosen each time");
  $("default-clear").disabled = !t.folder;
  const here = targetFolder(state, t);
  pathText($("here-path"), here, "Choose a folder…");
  $("here-clear").disabled = !here || here === t.default;
  $("new-view").href = api("/views/") + "#new:" + encodeURIComponent(name);
  drawViews();
  $("confirm-name").textContent = t.name;
  armDelete(t.name);
  $("confirm").disabled = t.views.length > 0;
  $("delete-note").textContent = t.views.length
    ? "Views export here: move them to another Target first."
    : "Removes the Target from targets.yaml. Nothing already exported is touched.";
  say($("message"), "");
  exportStale();
  list();
}

function drawViews() {
  const t = shown();
  const mine = views.filter((v) => v.target === t.name);
  $("view-count").textContent = String(mine.length);
  const rows = mine.map((v) => el("li", {},
    el("div", { className: "row-main" },
      el("a", { href: api("/views/") + "#" + encodeURIComponent(v.name) }, v.name),
      el("span", { className: "sub mono" }, v.layout)),
    el("div", { className: "row-tools" },
      el("button", { type: "button", className: "button small-button", textContent: "Take out",
        title: "The View stays; it names no Target", onclick: () => retarget(v, "") }))));
  if (!rows.length) rows.push(el("li", { className: "empty" }, "No View exports here yet: add one below, or make a new one."));
  $("views").replaceChildren(...rows);
  const others = views.filter((v) => v.target !== t.name);
  $("move-view").replaceChildren(el("option", { value: "" }, others.length ? "Add an existing View…" : "Every View is here"),
    ...others.map((v) => el("option", { value: v.name }, v.name + (v.target ? "  (now " + v.target + ")" : ""))));
  $("move-view").disabled = $("move").disabled = !others.length;
}

// retarget saves a View naming another Target, or none.
async function retarget(v, name) {
  const next = { ...v, target: name || undefined };
  if (!name) delete next.target;
  try {
    await post("/api/views", { view: next, previous: v.name });
    await load(current);
    say($("message"), name ? v.name + " now exports to " + name + "." : v.name + " names no Target now.");
  } catch (error) {
    say($("message"), error.message, true);
  }
}
$("move").addEventListener("click", () => {
  const v = views.find((x) => x.name === $("move-view").value);
  if (!v) return;
  if (v.target && !confirm(v.name + " exports to " + v.target + " now. Move it to " + current + "?")) return;
  retarget(v, current);
});

$("about").addEventListener("change", () => {
  save(targets.map((t) => t.name === current ? { ...plain(t), about: $("about").value.trim() } : plain(t)), "Saved.").then(() => list());
});

// Renaming adds the new name, moves every View to it, then drops the old.
$("name").addEventListener("change", async () => {
  const from = current;
  const to = $("name").value.trim();
  if (!to || to === from) { $("name").value = from; return; }
  if (targets.some((t) => t.name === to)) { say($("message"), "There is a Target named " + to + " already.", true); $("name").value = from; return; }
  const old = shown();
  if (!await save([...targets.map(plain), { ...plain(old), name: to }])) { $("name").value = from; return; }
  for (const v of views.filter((x) => x.target === from)) {
    try {
      await post("/api/views", { view: { ...v, target: to }, previous: v.name });
    } catch (error) {
      say($("message"), "Renamed only in part: " + error.message, true);
      return load(to);
    }
  }
  await save(targets.filter((t) => t.name !== from).map(plain));
  rememberTargetFolder(state, to, targetFolder(state, old) !== old.default ? targetFolder(state, old) : "");
  rememberTargetFolder(state, from, "");
  await load(to);
  say($("message"), "Renamed; every View naming " + from + " names " + to + " now.");
});
$("name").addEventListener("keydown", (event) => { if (event.key === "Enter") $("name").blur(); });

async function pick(title, message, start) {
  const chosen = await openFile({ title, message, folders: true, writable: true, confirm: "Choose",
    folder: start || state.root, fallbacks: [state.root, ""] });
  if (!chosen) return "";
  return Array.isArray(chosen) ? chosen[0] : chosen;
}
$("default").addEventListener("click", async () => {
  const t = shown();
  const path = await pick(t.name + "'s own folder", "The folder every machine exports " + t.name + " to unless another is chosen. It is kept in targets.yaml.", t.default);
  if (!path) return;
  if (await save(targets.map((x) => x.name === current ? { ...plain(x), folder: path } : plain(x)), "Saved.")) show(current);
});
$("default-clear").addEventListener("click", async () => {
  if (await save(targets.map((x) => x.name === current ? { ...plain(x), folder: "" } : plain(x)), "Its own folder is cleared: one is chosen each time.")) show(current);
});
$("here").addEventListener("click", async () => {
  const t = shown();
  const path = await pick("Export " + t.name + " to", "Where an export from this browser writes " + t.name + ". An export removes only files it wrote there itself.", targetFolder(state, t));
  if (!path) return;
  rememberTargetFolder(state, t.name, path);
  show(current);
});
$("here-clear").addEventListener("click", () => {
  rememberTargetFolder(state, current, "");
  show(current);
});

$("delete").addEventListener("click", async () => {
  if (await save(targets.filter((t) => t.name !== current).map(plain))) {
    rememberTargetFolder(state, current, "");
    await load("");
  }
});

// Export: every View naming the Target, or every Target.
const folders = () => Object.fromEntries(targets.map((t) => [t.name, targetFolder(state, t)]).filter(([, f]) => f));
function exportStale() {
  planned = null;
  $("run").disabled = true;
  $("run").textContent = "Export";
  $("export-plan").replaceChildren();
  say($("export-message"), "");
  $("dry-run").disabled = !shown() || !shown().views.length;
  $("dry-run").textContent = shown() ? "Check " + shown().name : "Check";
}
async function check(request, what) {
  exportStale();
  say($("export-message"), "Checking…");
  try {
    const answer = await post("/api/export/plan", { ...request, folders: folders() });
    $("export-plan").replaceChildren(...planNodes(state, answer));
    const changes = answer.jobs.reduce((n, j) => n + j.plan.add.length + j.plan.replace.length + j.plan.remove.length, 0);
    if (!answer.ready) say($("export-message"), "Not exported: fix what is listed below first. Nothing is written until everything checks.", true);
    else if (!changes) say($("export-message"), "Up to date: nothing to write.");
    else {
      say($("export-message"), "No conflicts. " + changes + " change" + (changes === 1 ? "" : "s") + " to write.");
      planned = request;
      $("run").disabled = false;
      $("run").textContent = "Export " + what;
    }
  } catch (error) {
    say($("export-message"), error.message, true);
  }
}
$("dry-run").addEventListener("click", () => check({ targets: [current] }, current));
$("export-all").addEventListener("click", () => {
  if ($("target").hidden && targets.length) show(targets[0].name);
  check({ all: true }, "all");
  $("export-plan").scrollIntoView({ block: "nearest" });
});
$("run").addEventListener("click", async () => {
  if (!planned) return;
  $("run").disabled = true;
  say($("export-message"), "Exporting: checking again, then copying and reading back…");
  try {
    const answer = await post("/api/export", { ...planned, folders: folders() });
    say($("export-message"), "Exported. " + answer.results.map((r) => (r.name || r.path) + ": " + r.result.written + " written, " +
      r.result.removed + " removed, " + r.result.kept + " unchanged").join("; ") + ".");
    $("export-plan").replaceChildren();
  } catch (error) {
    say($("export-message"), error.message, true);
  } finally {
    planned = null;
  }
});

load();
