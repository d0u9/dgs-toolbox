// Templates: each type's file, edited as written. The server parses and
// checks it; a type Items use keeps its name and kind.
import { $, el, loadState, post, frame, say } from "/common.js";

let state = {};
let templates = [];
let example = "";
let editing = null; // the type being edited, or "" for a new one

function render() {
  frame(state);
  $("count").textContent = templates.length;
  $("templates").replaceChildren(...templates.map((t) => {
    const li = el("li", { onclick: () => open(t.type) }, t.type,
      el("span", { className: "sub" }, t.kind + " · " + (t.items === 1 ? "1 Item" : t.items + " Items")));
    if (t.type === editing) li.className = "selected";
    return li;
  }), ...(editing === "" ? [el("li", { className: "selected" }, "New Template")] : []));
  const t = templates.find((x) => x.type === editing);
  $("file").textContent = t ? "templates/" + t.type + ".yaml" : "templates/<type>.yaml";
  $("delete").hidden = !t;
  $("delete").disabled = !!(t && t.items);
  $("delete").title = t && t.items ? "Items of this type exist: delete them first." : "";
}

function open(type) {
  editing = type;
  const t = templates.find((x) => x.type === type);
  $("yaml").value = t ? t.data : example.replace("type: id_card", "type: new_type");
  say($("message"), "");
  history.replaceState(null, "", type ? "#" + type : location.pathname);
  render();
}

async function reload() {
  state = await loadState();
  const answer = await (await fetch("/api/templates")).json();
  if (answer.error) throw new Error(answer.error);
  templates = answer.templates;
  example = answer.example;
  render();
}

$("new").onclick = () => open("");
$("save").onclick = async () => {
  try {
    const t = await post("/api/templates", { previous: editing || "", data: $("yaml").value });
    await reload();
    open(t.type);
    say($("message"), "Saved templates/" + t.type + ".yaml.");
  } catch (err) {
    say($("message"), err.message, true);
  }
};
$("delete").onclick = async () => {
  if (!editing || !confirm(`Delete the Template ${editing}?\n\nIts file moves into the tree's trash folder.`)) return;
  try {
    const answer = await post("/api/templates/delete", { type: editing });
    await reload();
    open(templates.length ? templates[0].type : "");
    say($("message"), "Deleted. It is in " + answer.trash + ".");
  } catch (err) {
    say($("message"), err.message, true);
  }
};

reload().then(() => {
  const wanted = location.hash.slice(1);
  open(templates.some((t) => t.type === wanted) ? wanted : templates.length ? templates[0].type : "");
}).catch((err) => {
  state.error = err.message;
  frame(state);
});
