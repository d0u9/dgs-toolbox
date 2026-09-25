// Browse: the Items kept in the tree, their fields, revisions and HEAD.
import { $, el, loadState, post, templateOf, label, inputFor, fieldsOf, frame, say, showText, showPreview } from "/common.js";

let state = { templates: [], items: [] };
let selected = null; // {id, digest}
let editing = "";
let notesOf = "";
// Items whose text matched the filter, by ID, with the line it matched on.
let textHits = new Map();
let unread = 0;

const words = () => $("filter").value.trim().toLowerCase().split(/\s+/).filter(Boolean);
const matches = (text) => words().every((w) => text.toLowerCase().includes(w));

function render() {
  frame(state);
  const items = state.items.filter((i) => textHits.has(i.id) || matches(label(state, i) + " " + Object.values(i.fields).join(" ")));
  $("count").textContent = items.length;
  $("none").hidden = state.items.length > 0 || !state.tree;
  $("unread").hidden = !unread;
  $("unread-count").textContent = unread + (unread === 1 ? " Item's text is" : " Items' text is") + " not read yet, so searching cannot find " + (unread === 1 ? "it." : "them.");
  $("items").replaceChildren(...items.map((item) => {
    const li = el("li", { onclick: () => pick(item.id, item.head || item.revisions[0].digest) },
      label(state, item),
      item.revisions.length > 1 ? el("span", { className: "tag" }, item.revisions.length + " revisions") : null,
      el("span", { className: "sub" }, Object.entries(item.fields).map(([k, v]) => k + ": " + shown(item, k, v)).join("  ")),
      textHits.get(item.id) ? el("span", { className: "sub snippet" }, textHits.get(item.id)) : null);
    if (selected && selected.id === item.id) li.className = "selected";
    return li;
  }));
  const item = selected && state.items.find((i) => i.id === selected.id);
  $("side").hidden = !item;
  if (item) detail(item);
}

// shown is a field's value as a person reads it: a linked Item by its name.
function shown(item, key, value) {
  const t = templateOf(state, item.type);
  const f = t && t.fields.find((x) => x.key === key);
  const other = f && f.type === "item" && state.items.find((i) => i.id === value);
  return other ? label(state, other) : value;
}

function pick(id, digest) {
  if (!selected || selected.id !== id || selected.digest !== digest) {
    showPreview({ item: id, digest }, "/api/revision?item=" + encodeURIComponent(id) + "&digest=" + encodeURIComponent(digest));
    showText({ item: id, digest });
  }
  if (!selected || selected.id !== id) {
    $("similar").replaceChildren();
    say($("similar-message"), "");
  }
  selected = { id, digest };
  history.replaceState(null, "", "#" + id);
  render();
}

function detail(item) {
  $("detail-head").textContent = label(state, item);
  const t = templateOf(state, item.type);
  if (editing !== item.id + JSON.stringify(item.fields)) {
    editing = item.id + JSON.stringify(item.fields);
    say($("edit-message"), "");
    $("edit-fields").replaceChildren(el("p", { className: "kind" }, item.kind),
      ...(t ? t.fields : Object.keys(item.fields).map((key) => ({ key }))).map((f) => inputFor(f, item.fields[f.key] || "", "", state, item.id)));
    $("save-button").disabled = !t;
  }
  if (notesOf !== item.id + "\n" + (item.notes || "")) {
    notesOf = item.id + "\n" + (item.notes || "");
    $("notes").value = item.notes || "";
    say($("notes-message"), "");
  }
  $("revisions").replaceChildren(...item.revisions.slice().reverse().map((r) => {
    const isHead = r.digest === item.head;
    const li = el("li", { onclick: () => pick(item.id, r.digest) },
      item.kind === "document" && !isHead ? el("button", {
        className: "small", type: "button", textContent: "Make HEAD",
        onclick: (event) => { event.stopPropagation(); makeHead(item.id, r.digest); },
      }) : null,
      new Date(r.added).toLocaleString(),
      isHead ? el("span", { className: "tag" }, "HEAD") : null,
      el("span", { className: "sub" }, (r.source ? r.source + " · " : "") + r.digest.slice(0, 12)));
    if (selected.digest === r.digest) li.className = "selected";
    return li;
  }));
}

async function makeHead(id, digest) {
  say($("head-message"), "");
  try {
    await post("/api/head", { item: id, digest });
    await reload();
    pick(id, digest);
  } catch (err) {
    say($("head-message"), err.message, true);
  }
}

$("edit").onsubmit = async (event) => {
  event.preventDefault();
  try {
    await post("/api/fields", { item: selected.id, fields: fieldsOf($("edit-fields")) });
    await reload();
    say($("edit-message"), "Saved.");
  } catch (err) {
    say($("edit-message"), err.message, true);
  }
};

$("notes-form").onsubmit = async (event) => {
  event.preventDefault();
  try {
    await post("/api/notes", { item: selected.id, notes: $("notes").value });
    await reload();
    say($("notes-message"), "Saved.");
  } catch (err) {
    say($("notes-message"), err.message, true);
  }
};

// searchText asks the server which Items' text holds every word typed.
let searchTimer = 0;
let searchAsked = 0;
function searchText() {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(async () => {
    const mine = ++searchAsked;
    const q = $("filter").value.trim();
    try {
      const answer = await (await fetch("/api/search?q=" + encodeURIComponent(q))).json();
      if (mine !== searchAsked) return;
      textHits = new Map((answer.hits || []).map((h) => [h.id, h.snippet]));
      unread = answer.unread || 0;
      render();
    } catch { /* the filter on fields still works */ }
  }, 200);
}

$("filter").oninput = () => { render(); searchText(); };

$("read-all").onclick = async () => {
  try {
    const answer = await post("/api/read-all", {});
    $("unread-count").textContent = answer.queued + " queued to be read; search again in a moment.";
    $("read-all").hidden = true;
  } catch (err) {
    $("unread-count").textContent = err.message;
  }
};

$("more").onclick = async () => {
  const id = selected && selected.id;
  if (!id) return;
  say($("similar-message"), "Comparing…");
  try {
    const answer = await (await fetch("/api/similar?item=" + encodeURIComponent(id))).json();
    if (!selected || selected.id !== id) return;
    const byId = Object.fromEntries(state.items.map((i) => [i.id, i]));
    $("similar").replaceChildren(...answer.hits.filter((h) => byId[h.id]).map((h) => el("li", {
      onclick: () => { const it = byId[h.id]; pick(it.id, it.head || it.revisions[0].digest); },
    }, label(state, byId[h.id]), el("span", { className: "sub" }, Math.round(h.score * 100) + "% alike"))));
    say($("similar-message"), answer.hits.length ? "" : "Nothing alike among the Items whose text has been read.");
  } catch (err) {
    say($("similar-message"), err.message, true);
  }
};

async function reload() {
  state = await loadState();
  render();
}

reload().then(() => {
  searchText();
  const wanted = state.items.find((i) => i.id === location.hash.slice(1));
  if (wanted) pick(wanted.id, wanted.head || wanted.revisions[0].digest);
}).catch((err) => {
  state.error = err.message;
  render();
});
