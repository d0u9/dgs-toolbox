// Cases: a matter being dealt with and the Items it has needed so far. The
// server keeps each Case as a file and decides every change; the page shows
// the Case and asks for one change at a time.
import { $, api, el, loadState, post, label, planNodes, frame, say } from "/common.js";
import { openFile } from "/ui/filedialog.js";
import { guardByName } from "/ui/confirm.js";

let state = { templates: [], items: [], expiry: {} };
let cases = [];
let current = null; // the Case shown, as the server last answered it
const FOLDER_KEY = "dgs-doc-case-folder";
let folder = "";
let planned = null;
const armDelete = guardByName($("confirm"), $("delete"), "");

const byId = () => Object.fromEntries(state.items.map((i) => [i.id, i]));
const day = (s) => s ? new Date(s).toLocaleDateString() : "";
const titleOf = (c) => c.title || c.name;

async function load(wanted) {
  state = await loadState();
  frame(state);
  const answer = await (await fetch(api("/api/cases"))).json();
  cases = answer.cases;
  if (answer.error) { $("error").hidden = false; $("error").textContent = answer.error; }
  $("item-options").replaceChildren(...state.items.map((i) => el("option", { value: label(state, i) + " — " + i.id })));
  const name = wanted ?? decodeURIComponent(location.hash.slice(1));
  const found = cases.find((c) => c.name === name) || (name === "" && !location.hash ? cases[0] : null);
  if (found) show(found);
  else if (!cases.length) showNew();
  else show(cases[0]);
}

// The list: open Cases first, then the archived ones.
function list() {
  $("count").textContent = cases.length;
  const row = (c) => {
    const open = c.needs.filter((n) => !n.item).length;
    return el("li", { className: current && c.name === current.name ? "selected" : "", onclick: () => show(c) },
      el("span", { className: "template-name case-list-title" }, titleOf(c)),
      open ? el("span", { className: "badge badge-soon", title: open + " still needed" }, open + " needed") : el("span", {}),
      el("span", { className: "template-sub" }, c.entries.length + (c.entries.length === 1 ? " Item" : " Items") + " · " +
        (c.status === "archived" ? "archived " + day(c.archived) : "opened " + day(c.opened))));
  };
  const group = (title, members) => members.length ? el("section", { className: "view-group-list" },
    el("div", { className: "group-head" }, el("span", { className: "group-name" }, title), el("span", { className: "muted" }, String(members.length))),
    el("ul", { className: "template-list" }, ...members.map(row))) : null;
  $("cases").replaceChildren(...[group("Open", cases.filter((c) => c.status === "open")),
    group("Archived", cases.filter((c) => c.status === "archived"))].filter(Boolean));
}

function showNew() {
  current = null;
  history.replaceState(null, "", location.pathname + location.search);
  $("case").hidden = true;
  $("new-form").hidden = false;
  $("new-title").value = "";
  $("new-name").value = "";
  $("new-file").textContent = "…";
  say($("new-message"), "");
  list();
  $("new-title").focus();
}

// slug makes a name from a title: lowercase ASCII words joined by -, with
// today's year in front, so a title in Chinese still gets a usable start.
function slug(title) {
  const words = title.toLowerCase().normalize("NFKD").replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
  return new Date().getFullYear() + (words ? "-" + words : "-");
}
let nameTouched = false;
$("new-title").addEventListener("input", () => {
  if (!nameTouched) $("new-name").value = slug($("new-title").value);
  $("new-file").textContent = $("new-name").value || "…";
});
$("new-name").addEventListener("input", () => {
  nameTouched = true;
  $("new-file").textContent = $("new-name").value || "…";
});
$("new").addEventListener("click", () => { nameTouched = false; showNew(); });
$("new-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    const c = await post("/api/cases/change", { op: "new", name: $("new-name").value.trim(), title: $("new-title").value });
    await load(c.name);
  } catch (error) {
    say($("new-message"), error.message, true);
  }
});

function show(c) {
  current = c;
  history.replaceState(null, "", location.pathname + location.search + "#" + encodeURIComponent(c.name));
  $("new-form").hidden = true;
  $("case").hidden = false;
  const archived = c.status === "archived";
  $("case").classList.toggle("archived", archived);
  $("title").value = c.title || "";
  $("title").placeholder = c.name;
  $("title").disabled = archived;
  $("notes").value = c.notes || "";
  $("notes").disabled = archived;
  $("layout").value = c.layout || "";
  $("layout").disabled = archived;
  $("file").textContent = "cases/" + c.name + ".yaml";
  $("status").textContent = archived ? "archived" : "open";
  $("status").className = "badge " + (archived ? "badge-permanent" : "badge-valid");
  $("archive").textContent = archived ? "Reopen" : "Archive…";
  $("archive").title = archived ? "Open the Case again to change it" : "Done: record which revision of each Item was used, and keep it as it is";
  $("dates").textContent = "Opened " + day(c.opened) + (c.archived ? " · archived " + day(c.archived) : "");
  $("confirm-name").textContent = c.name;
  armDelete(c.name);
  say($("message"), "");
  drawEntries();
  drawNeeds();
  exportStale();
  list();
}

// change asks the server for one change and shows the Case it answers.
async function change(request, done) {
  try {
    const c = await post("/api/cases/change", { name: current.name, ...request });
    cases = cases.map((x) => x.name === c.name ? c : x);
    show(c);
    if (done) say($("message"), done);
  } catch (error) {
    say($("message"), error.message, true);
  }
}

function expiryBadge(id) {
  const e = state.expiry[id];
  if (!e || !e.state || e.state === "none") return null;
  const text = { expired: "expired", soon: "expires soon", valid: "valid", permanent: "permanent" }[e.state] || e.state;
  return el("span", { className: "badge badge-" + e.state, title: e.date ? "Expires " + e.date : "" }, text);
}

function drawEntries() {
  const items = byId();
  const archived = current.status === "archived";
  $("item-count").textContent = current.entries.length ? String(current.entries.length) : "";
  const rows = current.entries.map((e) => {
    const item = items[e.item];
    const gone = current.missing.includes(e.item);
    const revision = item && e.digest ? item.revisions.findIndex((r) => (r.id || r.digest) === e.digest) + 1 : 0;
    const sub = [e.note, "added " + day(e.added)];
    if (archived && e.digest) sub.push(revision ? "handed over: revision " + revision + (item.head && item.head !== e.digest ? " (HEAD has moved on)" : "") : "revision " + e.digest.slice(0, 8));
    return el("li", {},
      el("div", { className: "row-main" },
        item ? el("a", { href: api("/browse/") + "#" + e.item }, label(state, item)) : el("span", { className: "gone" }, e.item + " — no longer in the tree"),
        gone && item ? el("span", { className: "sub gone" }, "the revision recorded is no longer in the tree") : null,
        el("span", { className: "sub" }, sub.filter(Boolean).join(" · "))),
      el("div", { className: "row-tools" }, item ? expiryBadge(e.item) : null,
        archived ? null : el("button", { type: "button", className: "button small-button", textContent: "Remove",
          title: "Take it out of the Case. The Item stays in the tree.", onclick: () => change({ op: "remove", item: e.item }) })));
  });
  if (!rows.length) rows.push(el("li", { className: "empty" }, archived ? "No Items." : "No Items yet: add them below as they are asked for, or from an Item on Browse."));
  $("entries").replaceChildren(...rows);
}

// itemFrom reads the Item picked in a datalist input: the ID after the dash.
function itemFrom(text) {
  const id = text.split(" — ").pop().trim();
  return state.items.some((i) => i.id === id) ? id : "";
}

function drawNeeds() {
  const items = byId();
  const archived = current.status === "archived";
  const open = current.needs.filter((n) => !n.item).length;
  $("need-count").textContent = current.needs.length ? open + " of " + current.needs.length + " open" : "";
  const rows = current.needs.map((n, i) => {
    const met = n.item && items[n.item];
    let tools;
    if (archived) tools = null;
    else if (n.item) tools = el("button", { type: "button", className: "button small-button", textContent: "Drop", title: "No longer asked for", onclick: () => change({ op: "drop", need: i }) });
    else {
      const pick = el("input", { placeholder: "Met by…", className: "case-meet", autocomplete: "off" });
      pick.setAttribute("list", "item-options");
      pick.addEventListener("change", () => {
        const id = itemFrom(pick.value);
        if (id) change({ op: "meet", need: i, item: id }, "Met, and the Item put in the Case.");
        else say($("message"), "Pick an Item from the list.", true);
      });
      tools = el("span", { className: "row-tools" }, pick,
        el("button", { type: "button", className: "button small-button", textContent: "Drop", title: "No longer asked for", onclick: () => change({ op: "drop", need: i }) }));
    }
    return el("li", { className: n.item ? "met" : "" },
      el("div", { className: "row-main" }, el("span", {}, (n.item ? "✓ " : "○ ") + n.text),
        el("span", { className: "sub" }, "asked " + day(n.added) + (n.item ? " · met " + day(n.met) + " by " + (met ? label(state, met) : n.item) : ""))),
      el("div", { className: "row-tools" }, tools));
  });
  if (!rows.length && !archived) rows.push(el("li", { className: "empty" }, "Nothing asked for that the tree lacks."));
  $("needs").replaceChildren(...rows);
}

$("add-form").addEventListener("submit", (event) => {
  event.preventDefault();
  const id = itemFrom($("add-item").value);
  if (!id) { say($("message"), "Pick an Item from the list.", true); return; }
  change({ op: "add", item: id, note: $("add-note").value }).then(() => { $("add-item").value = ""; $("add-note").value = ""; });
});
$("need-form").addEventListener("submit", (event) => {
  event.preventDefault();
  change({ op: "need", text: $("need-text").value }).then(() => { $("need-text").value = ""; });
});

// Title, notes and names are saved as they are left.
function edit() {
  if (!current || current.status === "archived") return;
  if ($("title").value === (current.title || "") && $("notes").value === (current.notes || "") && $("layout").value === (current.layout || "")) return;
  change({ op: "edit", title: $("title").value, notes: $("notes").value, layout: $("layout").value.trim() }, "Saved.");
}
$("title").addEventListener("change", edit);
$("notes").addEventListener("change", edit);
$("layout").addEventListener("change", edit);
$("title").addEventListener("keydown", (event) => { if (event.key === "Enter") $("title").blur(); });

$("archive").addEventListener("click", () => {
  if (current.status === "archived") return change({ op: "reopen" }, "Reopened.");
  const open = current.needs.filter((n) => !n.item).length;
  const question = "Archive " + titleOf(current) + "?\n\nThe revision of each Item used now is recorded, and the Case stays as it is until reopened." +
    (open ? "\n\n" + open + " need" + (open === 1 ? " is" : "s are") + " still open." : "");
  if (confirm(question)) change({ op: "archive" }, "Archived.");
});

$("delete").addEventListener("click", async () => {
  try {
    await post("/api/cases/change", { op: "delete", name: current.name });
    await load("");
  } catch (error) {
    say($("message"), error.message, true);
  }
});

// Export: the Case's Items, as it takes them, into a folder.
function setFolder(path) {
  folder = path;
  $("folder").textContent = path ? "‎" + path + "‎" : "Choose folder…";
  $("folder").classList.toggle("muted", !path);
  $("choose").title = path || "Choose the folder to export to";
  $("dry-run").disabled = !path;
}
function exportStale() {
  planned = null;
  $("run").disabled = true;
  $("export-plan").replaceChildren();
  say($("export-message"), "");
  let kept = "";
  try { kept = localStorage.getItem(FOLDER_KEY + ":" + (current ? current.name : "")) || ""; } catch { /* none */ }
  setFolder(kept);
}
$("choose").addEventListener("click", async () => {
  const chosen = await openFile({
    title: "Export the Case to",
    message: "Choose the folder the Case's PDFs are copied into. An export removes only files it wrote there itself.",
    folders: true, writable: true, confirm: "Choose",
    folder: folder || state.root, fallbacks: [state.root, ""],
  });
  if (!chosen) return;
  const path = Array.isArray(chosen) ? chosen[0] : chosen;
  try { localStorage.setItem(FOLDER_KEY + ":" + current.name, path); } catch { /* not kept */ }
  planned = null;
  $("run").disabled = true;
  $("export-plan").replaceChildren();
  setFolder(path);
});
$("dry-run").addEventListener("click", async () => {
  if (!folder) return;
  planned = null;
  $("run").disabled = true;
  say($("export-message"), "Checking…");
  const request = { name: current.name, folder };
  try {
    const answer = await post("/api/cases/export/plan", request);
    $("export-plan").replaceChildren(...planNodes(state, answer));
    const changes = answer.jobs.reduce((n, j) => n + j.plan.add.length + j.plan.replace.length + j.plan.remove.length, 0);
    if (!answer.ready) say($("export-message"), "Not exported: fix what is listed below first.", true);
    else if (!changes) say($("export-message"), "Up to date: nothing to write.");
    else {
      say($("export-message"), "No conflicts. " + changes + " change" + (changes === 1 ? "" : "s") + " to write.");
      planned = request;
      $("run").disabled = false;
    }
  } catch (error) {
    say($("export-message"), error.message, true);
  }
});
$("run").addEventListener("click", async () => {
  if (!planned) return;
  $("run").disabled = true;
  say($("export-message"), "Exporting: checking again, then copying and reading back…");
  try {
    const answer = await post("/api/cases/export", planned);
    const r = answer.results[0].result;
    const missed = answer.results[0].history_error;
    say($("export-message"), "Exported: " + r.written + " written, " + r.removed + " removed, " + r.kept + " unchanged." +
      (missed ? " History not recorded: " + missed : ""));
    $("export-plan").replaceChildren();
  } catch (error) {
    say($("export-message"), error.message, true);
  } finally {
    planned = null;
  }
});

load();
