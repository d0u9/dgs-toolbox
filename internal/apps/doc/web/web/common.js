// What both doc pages share: the tree's state, the fields form, and how an
// Item is named. The server decides everything; the pages show what it said.

import { show as showViewer, hide as hideViewer } from "/viewer.js";
import { splitter } from "/ui/splitter.js";
import * as statusBar from "/ui/statusbar.js";
import { suggest } from "/combo.js";
import { fileTree } from "/ui/filetree.js";
import { tooltip } from "/ui/tooltip.js";

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

// What a page shows beyond its address — the PDF picked in a tree, a search
// typed, a list scrolled — is kept in its history entry, so going to another
// page and coming Back returns to it, not to the page's start. keep merges
// values into the entry, kept reads one back, and address changes the
// address without losing them.
export function keep(values) {
  try { history.replaceState({ ...(history.state || {}), ...values }, ""); } catch { /* not kept */ }
}
export function kept(key) {
  return (history.state || {})[key];
}
export function address(url) {
  history.replaceState(history.state, "", url);
}
// keepScroll keeps how far the page, and each element named by id, is
// scrolled when the page is left; restoreScroll puts them back once the
// page has drawn what they hold.
const scroller = (id) => (id ? $(id) : document.scrollingElement);
export function keepScroll(ids = []) {
  // Kept a moment after each scroll: an entry changed while the page is
  // being left is not always kept.
  let timer = 0;
  addEventListener("scroll", () => {
    clearTimeout(timer);
    timer = setTimeout(() => keep({ scroll: Object.fromEntries(["", ...ids].map((id) => [id, scroller(id) ? scroller(id).scrollTop : 0])) }), 150);
  }, { capture: true, passive: true });
}
export function restoreScroll(scroll = kept("scroll")) {
  for (const [id, top] of Object.entries(scroll || {})) if (scroller(id)) scroller(id).scrollTop = top;
}
// scrollBack keeps the scroll of the page and the elements named, and
// returns what a page calls once it has drawn them: the first call puts the
// scroll back as it was left, any later one does nothing.
export function scrollBack(ids = []) {
  keepScroll(ids);
  let scroll = kept("scroll");
  return () => { if (scroll) restoreScroll(scroll); scroll = null; };
}

export function el(tag, props, ...children) {
  const node = Object.assign(document.createElement(tag), props || {});
  node.append(...children.filter((c) => c !== null && c !== undefined && c !== false));
  return node;
}

// No field here is a login, an address or a card: password managers are
// told to leave every one alone, so a field named country or name does not
// open 1Password's or the browser's fill-in menu.
function unfillable(root) {
  const inputs = root.matches?.("input, textarea") ? [root] : root.querySelectorAll?.("input, textarea") || [];
  for (const input of inputs) {
    input.setAttribute("data-1p-ignore", "");
    input.setAttribute("data-lpignore", "true");
    input.setAttribute("data-bwignore", "");
    input.setAttribute("data-form-type", "other");
  }
}
unfillable(document.body);
new MutationObserver((changes) => {
  for (const change of changes) for (const node of change.addedNodes) if (node.nodeType === 1) unfillable(node);
}).observe(document.body, { childList: true, subtree: true });

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

export async function post(url, body, { signal } = {}) {
  const response = await fetch(api(url), {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), signal,
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

// fieldHead is a field's name, marked when required. A field whose Template
// describes it has a ? beside its name: hovering it shows the description,
// clicking it shows it under the name until clicked again.
function fieldHead(field) {
  const name = el("span", { className: "field-name" }, field.key, field.required ? el("span", { className: "req" }, " *") : null);
  if (!field.description) return [name];
  const note = el("span", { className: "field-help", hidden: true }, field.description);
  const toggle = el("button", { type: "button", className: "field-help-toggle", title: field.description, textContent: "?" });
  toggle.setAttribute("aria-label", "About " + field.key);
  toggle.setAttribute("aria-expanded", "false");
  toggle.onclick = (event) => {
    event.preventDefault();
    note.hidden = !note.hidden;
    toggle.setAttribute("aria-expanded", String(!note.hidden));
  };
  name.append(" ", toggle);
  return [name, note];
}

// inputFor is the control a field's type asks for: a date picker, a list of
// options, a list of the tree's other Items, or a line of text. Every value
// control has the class field-input and the field's key as its name.
export function inputFor(field, value, placeholder, state, self, type) {
  const common = { name: field.key, className: "field-input" };
  let control;
  if (field.key === "issuer" && (!field.type || field.type === "text")) {
    control = el("input", { ...common, type: "text", value,
      placeholder: placeholder || "Choose or enter an issuer…", autocomplete: "off", spellcheck: false });
    let known = [];
    const wrapper = el("label", { className: "form-field" },
      ...fieldHead(field), suggest(control, () => known, { arrow: true }));
    const item = state?.items?.find((item) => item.id === self);
    const typ = type || item?.type || "";
    let request = 0;
    const refresh = async () => {
      const generation = ++request;
      const scope = wrapper.parentElement;
      const countryInput = scope?.querySelector('[name="country"]') || wrapper.closest("form")?.querySelector('[name="country"]');
      const nation = countryInput ? countryInput.value || templateOf(state, typ)?.defaults?.country || ""
        : item?.fields?.country || templateOf(state, typ)?.defaults?.country || "";
      if (!typ || !nation) { known = []; return; }
      try {
        const response = await fetch(api("/api/issuers?" + new URLSearchParams({ type: typ, country: nation })));
        if (!response.ok) return;
        const values = await response.json();
        if (generation === request) known = values;
      } catch { /* Free text remains usable when suggestions are unavailable. */ }
    };
    queueMicrotask(() => {
      if (!wrapper.isConnected) return;
      const scope = wrapper.closest("form") || wrapper.parentElement;
      scope.addEventListener("input", (event) => {
        if (wrapper.isConnected && event.target.name === "country") refresh();
      });
      refresh();
    });
    return wrapper;
  } else if (field.suggest && (!field.type || field.type === "text")) {
    control = el("input", { ...common, type: "text", value, placeholder: placeholder || "",
      autocomplete: "off", spellcheck: false });
    const typ = type || state?.items?.find((item) => item.id === self)?.type || "";
    let loaded = null;
    const known = () => loaded ||= typ ? fetch(api("/api/values?" + new URLSearchParams({ type: typ, key: field.key })))
      .then((response) => response.ok ? response.json() : []).catch(() => []) : Promise.resolve([]);
    return el("label", { className: "form-field" }, ...fieldHead(field), suggest(control, known, { arrow: true }));
  } else if (field.type === "revision" || field.type === "item") {
    // An Item, then for a revision field one of its revisions, HEAD when the
    // Item is picked. The value, <item-id> or <item-id>@<revision>, is in a
    // hidden input.
    const withRevision = field.type === "revision";
    control = el("input", { ...common, type: "hidden", value: value || "" });
    const [id, ref] = (value || "").split("@");
    const all = (state ? state.items : []).filter((i) => i.id !== self);
    let items = all, suggested = [], chosen = !!value;
    const pickItem = el("select", {});
    const pickRevision = el("select", { hidden: !withRevision });
    const draw = (want) => {
      const keep = pickItem.value || id;
      pickItem.replaceChildren(el("option", { value: "" }, want && !items.length ? "(no Item matches)" : ""),
        ...items.map((i) => el("option", { value: i.id, selected: i.id === keep },
          label(state, i) + (suggested.includes(i.id) ? " · suggested" : ""))));
      if (keep && !items.some((i) => i.id === keep)) pickItem.value = "";
    };
    // The server answers which Items agree with the field's match and which
    // its within suggests, from the form as it is now: a translation of a
    // driver_licence offers only licences, a bill the tenancy it was in.
    const here = () => {
      const scope = wrapper.closest("form") || wrapper.parentElement;
      return Object.fromEntries([...(scope?.querySelectorAll(".field-input[name]") || [])].map((i) => [i.name, i.value]));
    };
    // A newer question cancels the one still asked; a field no longer on
    // the page stops listening and asks nothing.
    let asking = null;
    const gone = new AbortController();
    const narrow = async () => {
      if (!type || !(field.match || field.within)) return draw(false);
      asking?.abort();
      const mine = asking = new AbortController();
      try {
        const answer = await post("/api/links", { type, key: field.key, self: self || "", fields: here() }, { signal: mine.signal });
        if (mine !== asking) return;
        const offered = new Set(answer.offered);
        items = all.filter((i) => offered.has(i.id));
        suggested = answer.suggested;
      } catch {
        if (mine !== asking || !wrapper.isConnected) return;
        items = all;
        suggested = [];
      }
      const before = pickItem.value;
      draw(true);
      if (!chosen && !pickItem.value && suggested.length === 1) pickItem.value = suggested[0];
      if (pickItem.value !== before) { fill(""); set(); }
    };
    const fill = (keep) => {
      const item = items.find((i) => i.id === pickItem.value) || all.find((i) => i.id === pickItem.value);
      const revs = item ? [...item.revisions].reverse() : [];
      const head = item && (item.head || (revs[0] && (revs[0].id || revs[0].digest)));
      pickRevision.replaceChildren(...revs.map((r) => {
        const rid = r.id || r.digest;
        return el("option", { value: rid, selected: rid === (keep || head) }, revisionName(r, rid === head));
      }));
      if (keep && !revs.some((r) => (r.id || r.digest) === keep)) pickRevision.append(el("option", { value: keep, selected: true }, keep));
      pickRevision.hidden = !withRevision || !item;
    };
    const set = () => {
      control.value = !pickItem.value ? "" : !withRevision ? pickItem.value : pickRevision.value ? pickItem.value + "@" + pickRevision.value : "";
      control.dispatchEvent(new Event("input", { bubbles: true }));
    };
    pickItem.onchange = () => { chosen = true; fill(""); set(); };
    pickRevision.onchange = set;
    const wrapper = el("div", { className: "form-field" }, ...fieldHead(field), pickItem, pickRevision, control);
    draw(false);
    fill(ref);
    queueMicrotask(() => {
      if (!wrapper.isConnected || !(field.match || field.within)) return;
      narrow();
      const watched = [...Object.values(field.match || {}), ...(field.within ? [field.within.date] : [])];
      // Typing a date, or a value filled in for it, sends one question once
      // it pauses, and none when the watched fields are as last asked.
      const ask = () => { const fields = here(); return JSON.stringify(watched.map((k) => fields[k])); };
      let timer = 0, last = ask();
      const changed = (event) => {
        if (!wrapper.isConnected) { gone.abort(); asking?.abort(); clearTimeout(timer); return; }
        if (event.target === control || !watched.includes(event.target.name)) return;
        clearTimeout(timer);
        timer = setTimeout(() => {
          if (!wrapper.isConnected) return;
          const now = ask();
          if (now !== last) { last = now; narrow(); }
        }, 250);
      };
      const scope = wrapper.closest("form") || wrapper.parentElement;
      for (const on of ["input", "change"]) scope.addEventListener(on, changed, { signal: gone.signal });
    });
    return wrapper;
  } else if (field.type === "select" && field.multiple) {
    // Several options: each card toggles, and a hidden input holds them
    // joined in the options' order, as the server keeps them.
    control = el("input", { ...common, type: "hidden", value });
    const picked = new Set(String(value || "").split(",").map((v) => v.trim()).filter(Boolean));
    const cards = el("div", { className: "field-choice-cards", role: "group" });
    cards.setAttribute("aria-label", field.key);
    const buttons = field.options.map((o) => {
      const button = el("button", { type: "button", className: "button field-choice-card" }, o);
      button.setAttribute("aria-pressed", String(picked.has(o)));
      button.onclick = () => {
        picked.has(o) ? picked.delete(o) : picked.add(o);
        button.setAttribute("aria-pressed", String(picked.has(o)));
        control.value = field.options.filter((x) => picked.has(x)).join(", ");
        control.dispatchEvent(new Event("input", { bubbles: true }));
        control.dispatchEvent(new Event("change", { bubbles: true }));
      };
      return button;
    });
    cards.append(...buttons);
    return el("div", { className: "form-field" }, ...fieldHead(field), control, cards);
  } else if (field.type === "select") {
    const choices = field.options.map((o) => [o, o]);
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
        const button = el("button", { type: "button", className: "button field-choice-card" }, text);
        button.onclick = () => {
          control.value = v;
          control.dispatchEvent(new Event("input", { bubbles: true }));
          control.dispatchEvent(new Event("change", { bubbles: true }));
        };
        return button;
      });
      // A required choice not yet made asks for it, so it is not passed over.
      const ask = el("span", { className: "field-choice-ask" }, "Pick one");
      const sync = () => {
        buttons.forEach((button, i) => button.setAttribute("aria-pressed", String(control.value === cardChoices[i][0])));
        const needed = !!field.required && !control.value;
        cards.classList.toggle("needed", needed);
        ask.hidden = !needed;
      };
      cards.append(...buttons, ask);
      control.addEventListener("change", sync);
      sync();
      return el("div", { className: "form-field" },
        ...fieldHead(field),
        control, cards);
    }
  } else if (field.type === "date" && (field.shape || []).includes("span")) {
    // A span is start/end, kept in one hidden value; an end left empty is
    // open, or, where the field also takes one day, makes it that day.
    control = el("input", { ...common, type: "hidden", value });
    const [from, to] = value.includes("/") ? value.split("/") : [value, ""];
    const day = field.shape.includes("day");
    const start = el("input", { type: "date", max: "9999-12-31", value: from });
    const end = el("input", { type: "date", max: "9999-12-31", value: to });
    end.title = day ? "Leave empty for one day" : "Leave empty for no end";
    const set = () => {
      control.value = !start.value && !end.value ? "" : !end.value && day ? start.value : start.value + "/" + end.value;
      control.dispatchEvent(new Event("input", { bubbles: true }));
      control.dispatchEvent(new Event("change", { bubbles: true }));
    };
    start.oninput = end.oninput = set;
    return el("div", { className: "form-field" }, ...fieldHead(field), control,
      el("div", { className: "date-span" }, start, el("span", {}, "–"), end));
  } else {
    control = el("input", { ...common, type: ["date", "month"].includes(field.type) ? field.type : "text",
      value, placeholder: placeholder || (field.type === "country" ? "cn, CHN, China, 中国…" : ""),
      spellcheck: false, autocomplete: "off" });
    // Without a max the browser takes a five-digit year as typed.
    if (field.type === "date") control.max = "9999-12-31";
    if (field.type === "month") control.max = "9999-12";
    if (field.type === "country") {
      control.title = "Pick a country the tree has, or type any code or name: kept as the Template's format says.";
      // The countries Items already have are offered; any other can be typed.
      return el("label", { className: "form-field" }, ...fieldHead(field), suggest(control, () => countriesOf(state), { arrow: true }));
    }
  }
  return el("label", { className: "form-field" },
    ...fieldHead(field), control);
}

// countriesOf is every value a country field of the tree's Items holds,
// once each, sorted.
function countriesOf(state) {
  if (!state) return [];
  const keys = new Set((state.templates || []).flatMap((t) => t.fields.filter((f) => f.type === "country").map((f) => f.key)));
  const seen = new Set();
  for (const item of state.items || []) {
    for (const k of keys) if (item.fields?.[k]) seen.add(item.fields[k]);
  }
  return [...seen].sort();
}

// revisionName is a revision as a picker lists it: when it was added, its
// ID's start, and HEAD when it is.
export const revisionName = (r, head) => (r.added || "").slice(0, 10) + " · " + (r.id || r.digest).slice(0, 8) + (head ? " · HEAD" : "");

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
  // The page answers from a cache of the tree that dgs's own writes keep
  // up to date; a sidecar edited by hand needs a Reload to be seen.
  if (state.tree !== false && !$("tree-reload")) {
    const reload = el("button", { id: "tree-reload", type: "button", className: "button tree-reload",
      title: "Read every sidecar again, for one edited outside dgs" }, "Reload");
    reload.onclick = async () => {
      reload.disabled = true;
      try { await post("/api/items/reload", {}); location.reload(); }
      catch (error) { statusBar.showError(error.message); reload.disabled = false; }
    };
    $("root").after(reload);
  }
  $("banner").hidden = state.tree !== false;
  $("error").hidden = !state.error;
  $("error").textContent = state.error || "";
}

// planNodes draws an export's plan, each Outline in turn: its
// problems first, then what would change.
export function planNodes(state, answer) {
  const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
  const name = (id) => byId[id] ? label(state, byId[id]) : id;
  const list = (title, rows, open) => rows.length ? el("details", { open: open || rows.length <= 12 },
    el("summary", {}, title + " (" + rows.length + ")"), el("ul", {}, ...rows)) : null;
  const line = (...parts) => el("li", {}, ...parts);
  // A name the Item has but its rule's order does not is fixed in the
  // order, not in the Item.
  const missingWhy = (m, path) => {
    const unordered = m.unordered || {};
    const absent = m.keys || [];
    return [
      ...(absent.length ? [" lacks ", path(absent.join(", "))] : []),
      ...Object.entries(unordered).flatMap(([k, v], i) => [absent.length || i ? "; " : " ", "has no number for ", path(v), " in the order ", path(k + (m.unorderedAt?.[k] ? " (children " + m.unorderedAt[k] + ")" : ""))]),
    ];
  };
  const path = (p) => el("span", { className: "mono" }, p);
  const out = [];
  if (answer.problems.length) out.push(el("div", { className: "problem" }, el("strong", {}, "The run"),
    el("ul", {}, ...answer.problems.map((p) => line(p)))));
  for (const j of answer.jobs) {
    const issues = [];
    for (const p of j.problems) issues.push(line(p));
    for (const [view, p] of Object.entries(j.combined.plans)) {
      for (const m of p.missing) issues.push(line(el("strong", {}, view), ": ", el("a", { href: api("/browse/") + "#" + m.item }, name(m.item)),
        ...missingWhy(m, path)));
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
      el("p", { className: "muted job-views" }, "rules " + j.views.join(", ") + " — " + counts),
      issues.length ? el("ul", { className: "job-issues" }, ...issues) : null,
      list("Add", j.plan.add.map((a) => line(path(a.path), el("span", { className: "muted" }, " · " + a.view)))),
      list("Replace", j.plan.replace.map((a) => line(path(a.path), el("span", { className: "muted" }, " · " + a.view)))),
      list("Remove", j.plan.remove.map((a) => line(path(a.path), el("span", { className: "muted" }, " · " + a.view)))),
      list("Left as they are, and forgotten", j.plan.left.map((a) => line(path(a.path), " — " + a.reason)))));
  }
  return out;
}

// An Outline is exported to the folder last chosen for it in this browser,
// else its own folder. The choice is per tree and per Outline; the key is
// the one Targets had, so a Target's choice holds for the Outline it became.
const targetKey = (state, name) => "dgs-doc-target:" + (state.name || "") + ":" + name;
export function outlineFolder(state, t) {
  let chosen = "";
  try { chosen = localStorage.getItem(targetKey(state, t.name)) || ""; } catch { /* none */ }
  return chosen || t.default || "";
}
// rememberOutlineFolder keeps path as this browser's folder for the
// Outline; an empty path forgets it, so the Outline's own folder is used.
export function rememberOutlineFolder(state, name, path) {
  try {
    if (path) localStorage.setItem(targetKey(state, name), path);
    else localStorage.removeItem(targetKey(state, name));
  } catch { /* not kept */ }
}

// nameTree draws a page's list of named things — Outlines, rules,
// Snapshots, Cases — as the file tree Templates draws: one file row each,
// a short note at its right, and what describes it in the row's tooltip
// (/ui/tooltip.js), so
// a row stays one line however long the description. entries are
//   {name, label, folder, note, badge, title}
// label is drawn instead of the name, folder puts the row in a folder of
// that name, note is a number or a word drawn muted at the right, badge one
// drawn as a badge, and title the tooltip's lines. selected is the picked
// entry's name; closed holds the folders drawn shut ("Archived/"). One that
// is new and not saved yet is noted under the tree.
// Folders are drawn in the order their entries first name them, and the
// picked entry's folder is drawn open.
export function nameTree(node, entries, { selected = "", onPick, unsaved = "", closed } = {}) {
  const pathOf = (e) => (e.folder ? e.folder + "/" : "") + e.name;
  const picked = entries.find((e) => e.name === selected);
  if (picked && picked.folder && closed) closed.delete(picked.folder + "/");
  const order = [...new Set(entries.map((e) => e.folder).filter(Boolean))];
  node.replaceChildren(fileTree(entries.map((e) => ({ path: pathOf(e), e })), {
    closed,
    folderOrder: (a, b) => order.indexOf(a) - order.indexOf(b),
    selected: picked ? pathOf(picked) : "",
    onPick: (f) => onPick(f.e),
    fileExtra: (f) => f.e.badge ? el("span", { className: "badge badge-soon" }, f.e.badge)
      : f.e.note !== undefined && f.e.note !== "" ? el("span", { className: "template-sub numeric" }, String(f.e.note)) : null,
    folderExtra: (path, files) => el("span", { className: "template-sub numeric" }, String(files.length)),
    decorate: (row, { file }) => {
      if (!file) return;
      if (file.e.label) row.querySelector(".ft-name").textContent = file.e.label;
      tooltip(row, [file.e.label || file.e.name, ...[].concat(file.e.title || [])]);
    },
  }), ...(unsaved ? [el("p", { className: "template-sub new-template" }, unsaved)] : []));
}

// outlineTitle is what a row's tooltip says of an Outline: what it is for,
// then its rules and Snapshots.
export const outlineTitle = (o) => [o.about,
  o.rules.length ? "rules: " + o.rules.map((r) => r.name).join(", ") : "no rule",
  (o.snapshots || []).length ? "Snapshots: " + o.snapshots.map((m) => m.name).join(", ") : ""];

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
// The browser's own PDF viewer in place of the pages dgs draws, kept per
// viewer: a PDF dgs draws wrongly, a layered scan, can be compared with it.
export const NATIVE_KEY = "dgs-doc-native-pdf";
export function nativePDF() {
  try { return localStorage.getItem(NATIVE_KEY) === "1"; } catch { return false; }
}
export function setNativePDF(on) {
  try { localStorage.setItem(NATIVE_KEY, on ? "1" : "0"); } catch { /* this visit only */ }
}
let lastPreview = null;
// redrawPreview shows the last PDF again, as the viewer now chosen.
export const redrawPreview = () => { if (lastPreview) showPreview(...lastPreview); };

// nativeSwitch is the Browser viewer button: a page's own, as Browse's in
// its reader bar, or else one put above the preview.
function nativeSwitch() {
  let button = document.getElementById("native-pdf");
  if (!button) {
    button = el("button", { id: "native-pdf", className: "chip", type: "button",
      title: "Show the PDF in the browser's own viewer, to compare with the pages dgs draws" }, "Browser viewer");
    const bar = el("div", { className: "native-bar", id: "native-bar" }, button);
    $("viewer").before(bar);
  }
  if (!button.dataset.wired) {
    button.dataset.wired = "1";
    button.addEventListener("click", () => {
      setNativePDF(!nativePDF());
      button.setAttribute("aria-pressed", String(nativePDF()));
      redrawPreview();
    });
  }
  button.setAttribute("aria-pressed", String(nativePDF()));
  return button;
}

export async function showPreview(query, viewer) {
  lastPreview = [query, viewer];
  nativeSwitch();
  if ($("native-bar")) $("native-bar").hidden = false;
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
  if (nativePDF()) return useViewer();
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
  lastPreview = null;
  if (document.getElementById("native-bar")) $("native-bar").hidden = true;
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
  share: "Changed shared with",
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
