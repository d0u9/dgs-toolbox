import { $, api, el, loadState, post, inputFor, fieldsOf, fieldsAt, frame, say } from "/common.js";

const id = new URLSearchParams(location.search).get("item");
let state, item;
$("back").href = api("/browse/") + "#" + encodeURIComponent(id || "");

function original() {
  const section = (title, values) => el("section", {}, el("h3", {}, title),
    ...Object.entries(values || {}).map(([key, value]) => el("p", { className: "change-type-value" },
      el("span", { className: "muted" }, key), el("strong", {}, value))));
  $("original").replaceChildren(section("Type", { type: item.type, kind: item.kind }), section("Item fields", item.fields),
    ...item.revisions.map((r, index) => section("Revision " + (index + 1) + " · " + r.digest.slice(0, 8), fieldsAt(item, r.digest))));
}

function draw() {
  const t = state.templates.find((template) => template.type === $("target").value);
  if (!t) return;
  const oldKeys = new Set([...Object.keys(item.fields || {}), ...item.revisions.flatMap((r) => Object.keys(r.fields || {}))]);
  const nextKeys = new Set(t.fields.map((f) => f.key));
  const dropped = [...oldKeys].filter((key) => !nextKeys.has(key));
  $("dropped").textContent = dropped.length ? "Fields kept in history, removed from current values: " + dropped.join(", ") : "All existing field names occur in the new Template.";
  $("target-fields").replaceChildren(el("h3", {}, "Item fields"),
    ...t.fields.filter((f) => !f.per_revision).map((f) => inputFor(f, item.fields?.[f.key] || "", (t.defaults || {})[f.key] || "", state, item.id)));
  $("target-revisions").replaceChildren(...item.revisions.map((r, index) => {
    const box = el("section", { className: "change-type-revision" }, el("h3", {}, "Revision " + (index + 1) + " · " + r.digest.slice(0, 8)));
    box.dataset.digest = r.digest;
    box.append(...t.fields.filter((f) => f.per_revision).map((f) => inputFor(f, fieldsAt(item, r.digest)[f.key] || "", (t.defaults || {})[f.key] || "", state, item.id)));
    return box;
  }));
}

$("target").onchange = draw;
$("change-form").onsubmit = async (event) => {
  event.preventDefault();
  const revisions = Object.fromEntries([...$("target-revisions").children].map((box) => [box.dataset.digest, fieldsOf(box)]));
  try {
    await post("/api/change-type", { item: id, type: $("target").value, fields: fieldsOf($("target-fields")), revisions });
    location.href = $("back").href;
  } catch (err) { say($("message"), err.message, true); }
};

state = await loadState();
frame(state);
item = state.items.find((entry) => entry.id === id);
if (!item) {
  say($("message"), "Item not found.", true);
  $("save").disabled = true;
} else {
  original();
  const choices = state.templates.filter((t) => t.kind === item.kind && t.type !== item.type);
  $("target").replaceChildren(...choices.map((t) => el("option", { value: t.type }, t.type + (t.description ? " · " + t.description : ""))));
  $("save").disabled = choices.length === 0;
  if (choices.length) draw(); else say($("message"), "No other Template of this kind is available.");
}
