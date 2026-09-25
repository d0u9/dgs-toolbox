// Browse: the Items kept in the tree, their fields, revisions and HEAD.
import { $, el, loadState, post, templateOf, label, inputFor, fieldsOf, frame, say, showText, showPreview } from "/common.js";

let state = { templates: [], items: [] };
let selected = null; // {id, digest}
let editing = "";

const words = () => $("filter").value.trim().toLowerCase().split(/\s+/).filter(Boolean);
const matches = (text) => words().every((w) => text.toLowerCase().includes(w));

function render() {
  frame(state);
  const items = state.items.filter((i) => matches(label(state, i) + " " + Object.values(i.fields).join(" ")));
  $("count").textContent = items.length;
  $("none").hidden = state.items.length > 0 || !state.tree;
  $("items").replaceChildren(...items.map((item) => {
    const li = el("li", { onclick: () => pick(item.id, item.head || item.revisions[0].digest) },
      label(state, item),
      item.revisions.length > 1 ? el("span", { className: "tag" }, item.revisions.length + " revisions") : null,
      el("span", { className: "sub" }, Object.entries(item.fields).map(([k, v]) => k + ": " + v).join("  ")));
    if (selected && selected.id === item.id) li.className = "selected";
    return li;
  }));
  const item = selected && state.items.find((i) => i.id === selected.id);
  $("side").hidden = !item;
  if (item) detail(item);
}

function pick(id, digest) {
  if (!selected || selected.id !== id || selected.digest !== digest) {
    showPreview({ item: id, digest }, "/api/revision?item=" + encodeURIComponent(id) + "&digest=" + encodeURIComponent(digest));
    showText({ item: id, digest });
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
      ...(t ? t.fields : Object.keys(item.fields).map((key) => ({ key }))).map((f) => inputFor(f, item.fields[f.key] || "", "")));
    $("save-button").disabled = !t;
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

$("filter").oninput = render;

async function reload() {
  state = await loadState();
  render();
}

reload().then(() => {
  const wanted = state.items.find((i) => i.id === location.hash.slice(1));
  if (wanted) pick(wanted.id, wanted.head || wanted.revisions[0].digest);
}).catch((err) => {
  state.error = err.message;
  render();
});
