// Log: every event in the tree's Items' history, newest first, as the server
// orders it. Picking an entry opens its Item on Browse.
import { $, api, el, loadState, label, frame, eventLines, eventTitles } from "/common.js";

let state = { templates: [], items: [] };
let entries = [];

const byId = () => Object.fromEntries(state.items.map((i) => [i.id, i]));

function drawFilters() {
  const keep = (select, options) => {
    const was = select.value;
    select.replaceChildren(el("option", { value: "" }, "any"), ...options.map(([value, text]) => el("option", { value }, text)));
    select.value = options.some(([value]) => value === was) ? was : "";
  };
  const items = byId();
  keep($("filter-action"), [...new Set(entries.map((e) => e.event.action))].sort().map((a) => [a, eventTitles[a] || a]));
  keep($("filter-type"), [...new Set(entries.map((e) => e.type))].sort().map((t) => [t, t]));
  keep($("filter-item"), [...new Set(entries.map((e) => e.item))]
    .map((id) => [id, items[id] ? label(state, items[id]) : id]).sort((a, b) => a[1].localeCompare(b[1])));
}

const filtering = () => $("filter").value.trim() || $("filter-frequent").getAttribute("aria-pressed") === "true" ||
  ["filter-action", "filter-type", "filter-item"].some((id) => $(id).value);

function render() {
  frame(state);
  drawFilters();
  const items = byId();
  const words = $("filter").value.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const action = $("filter-action").value, type = $("filter-type").value, item = $("filter-item").value;
  const frequent = $("filter-frequent").getAttribute("aria-pressed") === "true";
  const shown = entries.filter((e) => {
    if ((action && e.event.action !== action) || (type && e.type !== type) || (item && e.item !== item)) return false;
    if (frequent && !(items[e.item] || {}).frequent) return false;
    if (!words.length) return true;
    const { title, meta, lines } = eventLines(e.event);
    const text = [items[e.item] ? label(state, items[e.item]) : "", e.item, e.type, title, meta, ...lines].join(" ").toLowerCase();
    return words.every((w) => text.includes(w));
  });
  $("count").textContent = shown.length === entries.length ? entries.length + " events" : shown.length + " of " + entries.length + " events";
  $("filters-clear").hidden = !filtering();
  for (const id of ["filter-action", "filter-type", "filter-item"]) $(id).closest(".filter").classList.toggle("filter-active", !!$(id).value);
  $("none").hidden = entries.length > 0 || !state.tree;
  $("nothing").hidden = !entries.length || shown.length > 0;
  $("log").replaceChildren(...shown.map((e) => {
    const { title, meta, lines } = eventLines(e.event);
    const it = items[e.item];
    const [when, ...rest] = meta.split(" · ");
    return el("li", { onclick: () => { location.href = api("/browse/") + "#" + encodeURIComponent(e.item); } },
      el("span", { className: "log-when" }, when),
      el("span", { className: "log-item" }, it && it.frequent ? "★ " : "", it ? label(state, it) : e.item + " (deleted)",
        el("span", { className: "sub-inline" }, e.type)),
      el("span", {}, el("strong", {}, title), rest.length ? el("span", { className: "sub-inline" }, rest.join(" · ")) : null),
      ...lines.map((line) => el("span", { className: "sub", title: line }, line)));
  }));
}

for (const id of ["filter-action", "filter-type", "filter-item"]) $(id).addEventListener("change", render);
$("filter").oninput = render;
$("filter-frequent").onclick = () => {
  const on = $("filter-frequent").getAttribute("aria-pressed") !== "true";
  $("filter-frequent").setAttribute("aria-pressed", String(on));
  render();
};
$("filters-clear").onclick = () => {
  $("filter").value = "";
  $("filter-frequent").setAttribute("aria-pressed", "false");
  for (const id of ["filter-action", "filter-type", "filter-item"]) $(id).value = "";
  render();
};

async function load() {
  const [s, answer] = await Promise.all([loadState(), fetch(api("/api/history")).then((r) => r.json())]);
  state = s;
  if (answer.error && !state.error) state.error = answer.error;
  entries = answer.entries || [];
  render();
  // Browse links here with an Item in the address: show its events.
  const wanted = decodeURIComponent(location.hash.slice(1));
  if (wanted && entries.some((e) => e.item === wanted)) {
    $("filter-item").value = wanted;
    render();
  }
}

load().catch((err) => {
  state.error = err.message;
  render();
});
