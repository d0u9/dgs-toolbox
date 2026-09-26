// What both doc pages share: the tree's state, the fields form, and how an
// Item is named. The server decides everything; the pages show what it said.

import { show as showViewer, hide as hideViewer } from "/viewer.js";
import { splitter } from "/ui/splitter.js";
import * as statusBar from "/ui/statusbar.js";

// The status bar along the bottom, as on every dgs page: the tree on the
// left, what the page shows in the middle, and every message said anywhere
// on the page as news over it.
statusBar.mount();
export { statusBar };

export const $ = (id) => document.getElementById(id);

// The tree a page works on is in its address, ?tree=books, so two tabs can
// hold two trees and a link or a reload keeps its tree. api() puts it on
// every request; none is the first tree.
export const treeName = new URLSearchParams(location.search).get("tree") || "";
export function api(path) {
  if (!treeName) return path;
  return path + (path.includes("?") ? "&" : "?") + "tree=" + encodeURIComponent(treeName);
}
for (const link of document.querySelectorAll("a.topbar-link")) {
  if (treeName) link.href = api(link.getAttribute("href"));
}

export function el(tag, props, ...children) {
  const node = Object.assign(document.createElement(tag), props || {});
  node.append(...children.filter((c) => c !== null && c !== undefined && c !== false));
  return node;
}

// The list down the left of every page is as wide as the reader drags it,
// one width for all the doc pages.
const list = document.querySelector("main.work > section.list");
if (list) {
  const handle = el("div", { className: "splitter col", role: "separator" });
  handle.setAttribute("aria-orientation", "vertical");
  list.after(handle);
  splitter({ handle, target: list, axis: "x", min: 320, max: () => window.innerWidth - 360, key: "dgs-doc-size-list" });
}

export const size = (n) => n < 1024 ? n + " B" : n < 1048576 ? (n / 1024).toFixed(0) + " KB" : (n / 1048576).toFixed(1) + " MB";

export async function loadState() {
  const response = await fetch(api("/api/state"));
  return response.json();
}

export async function post(url, body) {
  const response = await fetch(api(url), {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
  });
  const answer = await response.json();
  if (!response.ok) throw new Error(answer.error || response.statusText);
  return answer;
}

export const templateOf = (state, type) => state.templates.find((t) => t.type === type);

// An Item is named by its type and its distinguishing fields — what makes it
// the one it is.
// fieldsAt is an Item's fields as one revision has them: its own, with the
// revision's per_revision values over them.
export function fieldsAt(item, digest) {
  const rev = (item.revisions || []).find((r) => (r.id || r.digest) === digest);
  return rev?.snapshot ? { ...(rev.fields || {}) } : { ...item.fields, ...((rev && rev.fields) || {}) };
}

// currentFields is fieldsAt the revision that stands for the Item.
export const currentFields = (item) => fieldsAt(item, item.head || (item.revisions.length ? (item.revisions[item.revisions.length - 1].id || item.revisions[item.revisions.length - 1].digest) : ""));

export function label(state, item) {
  const t = templateOf(state, item.type);
  const keys = t ? t.fields.filter((f) => f.distinguishing).map((f) => f.key) : Object.keys(item.fields);
  const parts = keys.map((k) => item.fields[k]).filter(Boolean);
  return item.type + (parts.length ? " · " + parts.join(" · ") : "");
}

// inputFor is the control a field's type asks for: a date picker, a list of
// options, a list of the tree's other Items, or a line of text. Every value
// control has the class field-input and the field's key as its name.
let fieldListID = 0;
export function inputFor(field, value, placeholder, state, self, type) {
  const common = { name: field.key, className: "field-input" };
  let control;
  if (field.key === "issuer" && (!field.type || field.type === "text")) {
    const id = "issuer-options-" + (++fieldListID);
    control = el("input", { ...common, type: "text", value,
      placeholder: placeholder || "Choose or enter an issuer…", autocomplete: "off", spellcheck: false });
    control.setAttribute("list", id);
    const options = el("datalist", { id });
    const wrapper = el("label", { className: "form-field" },
      el("span", {}, field.key, field.required ? el("span", { className: "req" }, " *") : null), control, options);
    const item = state?.items?.find((item) => item.id === self);
    const typ = type || item?.type || "";
    let request = 0;
    const refresh = async () => {
      const generation = ++request;
      options.replaceChildren();
      const scope = wrapper.parentElement;
      const countryInput = scope?.querySelector('[name="country"]') || wrapper.closest("form")?.querySelector('[name="country"]');
      const nation = countryInput ? countryInput.value || templateOf(state, typ)?.defaults?.country || ""
        : item?.fields?.country || templateOf(state, typ)?.defaults?.country || "";
      if (!typ || !nation) return;
      try {
        const response = await fetch(api("/api/issuers?" + new URLSearchParams({ type: typ, country: nation })));
        if (!response.ok) return;
        const values = await response.json();
        if (generation === request && wrapper.isConnected) options.replaceChildren(...values.map((value) => el("option", { value })));
      } catch { /* Free text remains usable when suggestions are unavailable. */ }
    };
    control.addEventListener("focus", refresh);
    queueMicrotask(() => {
      if (!wrapper.isConnected) return;
      const scope = wrapper.closest("form") || wrapper.parentElement;
      scope.addEventListener("input", (event) => {
        if (wrapper.isConnected && event.target.name === "country") refresh();
      });
      refresh();
    });
    return wrapper;
  } else if (field.type === "select" || field.type === "item") {
    const choices = field.type === "select"
      ? field.options.map((o) => [o, o])
      : (state ? state.items : []).filter((i) => i.id !== self).map((i) => [i.id, label(state, i)]);
    const empty = placeholder && field.type === "select" ? "(" + placeholder + ")" : "";
    control = el("select", common, el("option", { value: "" }, empty),
      ...choices.map(([v, text]) => el("option", { value: v, selected: v === value }, text)));
    if (value && !choices.some(([v]) => v === value)) {
      control.append(el("option", { value, selected: true }, value));
    }
    if (field.type === "select" && choices.length > 0 && choices.length <= 5 && (!value || choices.some(([v]) => v === value))) {
      control.hidden = true;
      const cards = el("div", { className: "field-choice-cards", role: "group" });
      cards.setAttribute("aria-label", field.key);
      const cardChoices = field.required ? choices : [["", "Not set"], ...choices];
      const buttons = cardChoices.map(([v, text]) => {
        const button = el("button", { type: "button", className: "field-choice-card" }, text);
        button.onclick = () => {
          control.value = v;
          control.dispatchEvent(new Event("input"));
          control.dispatchEvent(new Event("change"));
        };
        return button;
      });
      const sync = () => buttons.forEach((button, i) => {
        button.setAttribute("aria-pressed", String(control.value === cardChoices[i][0]));
      });
      cards.append(...buttons);
      control.addEventListener("change", sync);
      sync();
      return el("div", { className: "form-field" },
        el("span", {}, field.key, field.required ? el("span", { className: "req" }, " *") : null),
        control, cards);
    }
  } else {
    control = el("input", { ...common, type: ["date", "month"].includes(field.type) ? field.type : "text",
      value, placeholder: placeholder || (field.type === "country" ? "cn, CHN, China, 中国…" : ""),
      spellcheck: false, autocomplete: "off" });
    if (field.type === "country") control.title = "Any code or name: kept as the Template's format says.";
  }
  return el("label", { className: "form-field" },
    el("span", {}, field.key, field.required ? el("span", { className: "req" }, " *") : null), control);
}

export const fieldsOf = (container) => Object.fromEntries(
  [...container.querySelectorAll(".field-input")].map((input) => [input.name, input.value]));

// tagsAt is the tags a revision has: the Item's, which hold for every
// revision, and the revision's own.
export function tagsAt(item, digest) {
  const own = (item.revisions.find((r) => (r.id || r.digest) === digest) || {}).tags || [];
  return [...new Set([...(item.tags || []), ...own])].sort();
}

// tagUses counts, for each tag, the Items that use it anywhere: on the Item
// or on any of its revisions.
export function tagUses(items) {
  const counts = new Map();
  for (const item of items || []) {
    const all = new Set([...(item.tags || []), ...item.revisions.flatMap((r) => r.tags || [])]);
    for (const name of all) counts.set(name, (counts.get(name) || 0) + 1);
  }
  return [...counts].map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
}

// The top bar and the banner every page has.
export function frame(state) {
  $("root").textContent = state.root || "";
  statusBar.setState(state.name ? "tree " + state.name : state.tree === false ? "not a tree" : "doc tree");
  const items = (state.items || []).length;
  const templates = (state.templates || []).length;
  statusBar.setSummary(state.tree === false ? "Run <kbd>dgs doc init</kbd> on this folder first."
    : items + (items === 1 ? " Item" : " Items") + " · " + templates + (templates === 1 ? " Template" : " Templates"));
  if (state.error) statusBar.showError(state.error);
  const trees = state.trees || [];
  if (trees.length > 1 && !$("tree-pick")) {
    const pick = el("select", { id: "tree-pick", className: "tree-pick", title: "The tree this page works on" },
      ...trees.map((t) => el("option", { value: t.name, selected: t.name === state.name }, t.name)));
    pick.onchange = () => {
      const params = new URLSearchParams(location.search);
      params.set("tree", pick.value);
      location.href = location.pathname + "?" + params.toString();
    };
    $("root").before(pick);
  }
  $("banner").hidden = state.tree !== false;
  $("error").hidden = !state.error;
  $("error").textContent = state.error || "";
}

// planNodes draws an export's plan, each Target or folder in turn: its
// problems first, then what would change.
export function planNodes(state, answer) {
  const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
  const name = (id) => byId[id] ? label(state, byId[id]) : id;
  const list = (title, rows, open) => rows.length ? el("details", { open: open || rows.length <= 12 },
    el("summary", {}, title + " (" + rows.length + ")"), el("ul", {}, ...rows)) : null;
  const line = (...parts) => el("li", {}, ...parts);
  const path = (p) => el("span", { className: "mono" }, p);
  const out = [];
  if (answer.problems.length) out.push(el("div", { className: "problem" }, el("strong", {}, "The run"),
    el("ul", {}, ...answer.problems.map((p) => line(p)))));
  for (const j of answer.jobs) {
    const issues = [];
    for (const p of j.problems) issues.push(line(p));
    for (const [view, p] of Object.entries(j.combined.plans)) {
      for (const m of p.missing) issues.push(line(el("strong", {}, view), ": ", el("a", { href: api("/browse/") + "#" + m.item }, name(m.item)),
        " lacks ", path(m.keys.join(", "))));
      for (const c of p.clashes) issues.push(line(el("strong", {}, view), ": ", path(c.path), " is wanted by " + c.files.length + " PDFs"));
    }
    for (const c of j.combined.clashes) issues.push(line(path(c.path), " is wanted by ",
      ...c.files.flatMap((f, i) => [i ? " and " : "", el("strong", {}, f.view), " (", f.path === c.path ? name(f.item) : path(f.path), ")"])));
    for (const a of j.plan.blocked) issues.push(line(path(a.path), " — " + a.reason));
    const counts = [["add", j.plan.add], ["replace", j.plan.replace], ["remove", j.plan.remove], ["unchanged", j.plan.keep]]
      .map(([k, l]) => l.length + " " + k).join(" · ");
    out.push(el("section", { className: "job" + (issues.length ? " job-bad" : "") },
      el("div", { className: "job-head" },
        el("strong", {}, j.name || "Folder"), el("span", { className: "mono muted job-path" }, j.path),
        el("span", { className: "badge " + (issues.length ? "badge-expired" : "badge-valid") }, issues.length ? issues.length + " to fix" : "no conflicts")),
      el("p", { className: "muted job-views" }, j.views.join(", ") + " — " + counts),
      issues.length ? el("ul", { className: "job-issues" }, ...issues) : null,
      list("Add", j.plan.add.map((a) => line(path(a.path), el("span", { className: "muted" }, " · " + a.view)))),
      list("Replace", j.plan.replace.map((a) => line(path(a.path), el("span", { className: "muted" }, " · " + a.view)))),
      list("Remove", j.plan.remove.map((a) => line(path(a.path), el("span", { className: "muted" }, " · " + a.view)))),
      list("Left as they are, and forgotten", j.plan.left.map((a) => line(path(a.path), " — " + a.reason)))));
  }
  return out;
}

// A Target goes to the folder last chosen for it in this browser, else its
// own folder from targets.yaml. The choice is per tree and per Target.
const targetKey = (state, name) => "dgs-doc-target:" + (state.name || "") + ":" + name;
export function targetFolder(state, t) {
  let chosen = "";
  try { chosen = localStorage.getItem(targetKey(state, t.name)) || ""; } catch { /* none */ }
  return chosen || t.default || "";
}
// rememberTargetFolder keeps path as this browser's folder for the Target;
// an empty path forgets it, so the Target's own folder is used again.
export function rememberTargetFolder(state, name, path) {
  try {
    if (path) localStorage.setItem(targetKey(state, name), path);
    else localStorage.removeItem(targetKey(state, name));
  } catch { /* not kept */ }
}

export function say(node, text, error) {
  node.className = error ? "message error" : "message";
  node.textContent = text;
  if (text) statusBar.show(text, { error: !!error });
}

// The text read off a PDF is laid over the pictures of its pages, where it
// was read: invisible until it is hovered or selected, so it can be copied
// straight off the page. Reading can take a few seconds a page, so an answer
// for a PDF no longer shown is dropped.
let textAsked = 0;
let textPages = [];
// showText asks for the pages one at a time, the first first, and lays each
// on the page as it comes, so the first page's text does not wait for the
// last. onAnswer is called with each page's suggestions, in page order.
export async function showText(query, onAnswer) {
  const asked = ++textAsked;
  textPages = [];
  layText();
  say($("text-message"), "Reading the text…");
  const ask = async (n) => {
    try {
      const response = await fetch(api("/api/text?" + new URLSearchParams({ ...query, page: n })));
      const answer = await response.json();
      if (!response.ok) throw new Error(answer.error || response.statusText);
      return answer;
    } catch (err) {
      return { error: err.message };
    }
  };
  const first = await ask(0);
  if (asked !== textAsked) return;
  if (first.error) {
    say($("text-message"), first.error, first.available !== false);
    return;
  }
  const total = Math.min(first.count, first.maxPages);
  // The rest are asked for together, so they queue ahead of reading in
  // advance at once rather than one after another.
  const rest = [];
  for (let n = 1; n < total; n++) rest.push(ask(n));
  const answers = [first];
  let shown = 0;
  const show = (answer, n) => {
    textPages[n] = answer.page;
    layText(n);
    if (onAnswer) onAnswer(answer);
    shown++;
    if (shown < total) say($("text-message"), `Reading the text: ${shown} of ${total} pages…`);
  };
  show(first, 0);
  for (let i = 0; i < rest.length; i++) {
    const answer = await rest[i];
    if (asked !== textAsked) return;
    if (answer.error) continue;
    answers.push(answer);
    show(answer, i + 1);
  }
  const lines = textPages.reduce((sum, p) => sum + (p ? p.lines.length : 0), 0);
  const recognised = answers.some((a) => a.page.source === "recognised");
  say($("text-message"), !lines ? "No text found on the page."
    : (recognised ? "Text recognised on the page" : "The PDF's own text") + ": select it on the page to copy it.");
}

// layText puts each page's lines over its picture. A line is sized to its box:
// the font to its height, then stretched to its width, so a selection covers
// the words it selects.
function layText(only) {
  for (const sheet of $("pages").querySelectorAll(".sheet")) {
    const n = Number(sheet.dataset.page);
    if (only !== undefined && n !== only) continue;
    const layer = sheet.querySelector(".text-layer");
    const page = textPages[n];
    layer.replaceChildren(...(page ? page.lines.map((line, i) => el("span", {
      className: "text-line", textContent: line.text,
      style: `left:${line.box.x * 100}%;top:${line.box.y * 100}%;height:${line.box.h * 100}%`,
    })) : []));
    layer.querySelectorAll(".text-line").forEach((span, i) => {
      span.dataset.line = i;
      span.dataset.w = page.lines[i].box.w;
    });
    fit(sheet);
  }
}

function fit(sheet) {
  const width = sheet.clientWidth;
  if (!sheet.clientHeight) return;
  for (const span of sheet.querySelectorAll(".text-line")) {
    span.style.transform = "";
    span.style.fontSize = Math.max(4, span.clientHeight * 0.85) + "px";
    const natural = span.offsetWidth;
    if (natural > 0) span.style.transform = `scaleX(${(Number(span.dataset.w) * width) / natural})`;
  }
}

const refit = new ResizeObserver((entries) => entries.forEach((e) => fit(e.target)));

// showSource marks the line a suggested value was read from, or none.
export function showSource(at, reveal = false) {
  for (const old of $("pages").querySelectorAll(".text-line.source")) old.classList.remove("source");
  if (!at || at.page < 0) return;
  const span = $("pages").querySelector(`.sheet[data-page="${at.page}"] .text-line[data-line="${at.line}"]`);
  if (!span) return;
  span.classList.add("source");
  if (!reveal) return;
  // Hover must never move the document. Explicit field focus reveals the
  // source inside the PDF pane without scrolling the surrounding form.
  const pages = $("pages");
  const bounds = pages.getBoundingClientRect();
  const line = span.getBoundingClientRect();
  const top = bounds.top + pages.clientTop;
  const left = bounds.left + pages.clientLeft;
  const dy = line.top < top ? line.top - top : Math.max(0, line.bottom - top - pages.clientHeight);
  const dx = line.left < left ? line.left - left : Math.max(0, line.right - left - pages.clientWidth);
  pages.scrollBy({ top: dy, left: dx, behavior: "instant" });
}

// showPreview draws a PDF as pictures of its pages, which scroll smoothly,
// and falls back to the browser's viewer when a page is not a scan. query
// names the PDF as the API does; viewer is its URL for the viewer.
let previewAsked = 0;
export async function showPreview(query, viewer) {
  const asked = ++previewAsked;
  $("empty").hidden = true;
  hideViewer();
  $("frame").hidden = true;
  $("frame").removeAttribute("src");
  const useViewer = () => {
    if (asked !== previewAsked) return;
    hideViewer();
    $("frame").src = viewer;
    $("frame").hidden = false;
  };
  let info;
  try {
    const response = await fetch(api("/api/pages?" + new URLSearchParams(query)));
    info = await response.json();
  } catch {
    info = { count: 0 };
  }
  if (asked !== previewAsked) return;
  if (!info.count) return useViewer();
  $("frame").hidden = true;
  $("frame").removeAttribute("src");
  const sheets = showViewer(info.count,
    (n, size) => api("/api/page?" + new URLSearchParams({ ...query, n, size, v: info.digest })), useViewer);
  sheets.forEach((sheet) => refit.observe(sheet));
  layText();
}

export function clearPreview() {
  previewAsked++;
  textAsked++;
  textPages = [];
  hideViewer();
  $("frame").hidden = true;
  $("frame").removeAttribute("src");
  $("empty").hidden = false;
}

// What each history action is called on the page.
export const eventTitles = {
  supersede: "Superseded by", undo_supersede: "Replacement undone",
  create_without_pdf: "Created without PDF", create_revision_without_pdf: "Added revision without PDF", attach_pdf: "Attached PDF",
  import: "Imported", import_revision: "Added revision", edit_fields: "Changed fields", edit_notes: "Changed notes",
  edit_tags: "Changed tags", edit_revision_tags: "Changed revision tags", make_head: "Made HEAD", delete_revision: "Deleted revision", change_type: "Changed type",
  export: "Exported", merge: "Merged", mark_frequent: "Marked frequent", unmark_frequent: "Unmarked frequent",
  retire: "Retired", unretire: "Back in use", edit_retired_reason: "Changed retired reason",
};

// eventLines is one history event as a title and the lines under it. The
// server keeps and orders the history; this only puts it into words.
export function eventLines(event) {
  const pairs = (values) => Object.entries(values || {}).map(([key, value]) => `${key}: ${value}`).join("; ");
  const revisions = (byDigest) => Object.entries(byDigest || {}).map(([digest, values]) => `${digest.slice(0, 8)} (${pairs(values)})`).join("; ");
  const lines = [];
  const changes = Object.entries(event.changes || {}).map(([key, pair]) => `${key}: ${pair[0] || "∅"} → ${pair[1] || "∅"}`).join("; ");
  if (changes) lines.push(changes);
  if (event.action === "change_type") {
    for (const [name, text] of [["Previous fields", pairs(event.previous_fields)], ["New fields", pairs(event.new_fields)],
      ["Previous revision fields", revisions(event.previous_revisions)], ["New revision fields", revisions(event.new_revisions)]]) {
      if (text) lines.push(name + ": " + text);
    }
  }
  const at = event.at ? new Date(event.at) : null;
  const when = at && !isNaN(at) ? at.toLocaleString() : event.at || "";
  const meta = when + (event.from_type ? ` · ${event.from_type} → ${event.to_type}` : "") + (event.digest ? " · " + event.digest.slice(0, 8) : "") +
    (event.target ? " · " + event.target : "") + (event.view ? " · " + event.view : "");
  return { title: eventTitles[event.action] || event.action, meta, lines };
}
