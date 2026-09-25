// Templates: each type's file, edited as written. The server parses and
// checks it; a type Items use keeps its name and kind.
import { $, api, el, loadState, post, frame, say } from "/common.js";
import { codeEditor, highlight } from "/ui/codeedit.js";

let state = {};
let templates = [];
let example = "";
let editing = null; // the type being edited, or "" for a new one
let saved = ""; // the text as last loaded or saved, to tell an edit

const editor = codeEditor($("yaml"), { onChange: dirty, onSave: save });

function dirty() {
  $("dirty").hidden = editor.value === saved;
}

function render() {
  frame(state);
  $("count").textContent = templates.length;
  const row = (type, kind, sub, selected, onclick) => el("li", { className: selected ? "selected" : "", onclick },
    el("span", { className: "template-name" }, type),
    kind ? el("span", { className: "badge" }, kind) : null,
    el("span", { className: "template-sub" }, sub));
  $("templates").replaceChildren(...templates.map((t) => {
    const fields = (state.templates || []).find((x) => x.type === t.type)?.fields.length ?? 0;
    return row(t.type, t.kind, `${fields} field${fields === 1 ? "" : "s"} · ${t.items === 1 ? "1 Item" : t.items + " Items"}`,
      t.type === editing, () => leave() && open(t.type));
  }), ...(editing === "" ? [row("new template", "", "not saved yet", true)] : []));
  const t = templates.find((x) => x.type === editing);
  const file = t ? "templates/" + t.type + ".yaml" : "templates/<type>.yaml";
  $("title").textContent = t ? t.type : "New Template";
  $("file").textContent = file;
  $("file-again").textContent = file;
  $("danger").hidden = !t;
  $("confirm-name").textContent = t ? t.type : "";
  $("confirm").value = "";
  $("confirm").disabled = !!(t && t.items);
  $("delete").disabled = true;
  $("delete-note").textContent = t && t.items
    ? `${t.items === 1 ? "1 Item uses" : t.items + " Items use"} it: delete ${t.items === 1 ? "that Item" : "them"} first.`
    : "Its file moves into the tree's trash folder; nothing is erased.";
}

// leave asks before an unsaved edit is dropped.
function leave() {
  return editor.value === saved || confirm("Drop the unsaved changes to this Template?");
}

function open(type) {
  editing = type;
  const t = templates.find((x) => x.type === type);
  saved = t ? t.data : example.replace("type: id_card", "type: new_type");
  editor.value = saved;
  dirty();
  say($("message"), "");
  history.replaceState(null, "", type ? "#" + type : location.pathname);
  render();
}

async function reload() {
  state = await loadState();
  const answer = await (await fetch(api("/api/templates"))).json();
  if (answer.error) throw new Error(answer.error);
  templates = answer.templates;
  example = answer.example;
  $("example").replaceChildren(...example.split("\n").map((line) => el("div", {}, ...highlight(line))));
  render();
}

async function save() {
  try {
    const t = await post("/api/templates", { previous: editing || "", data: editor.value });
    await reload();
    open(t.type);
    say($("message"), "Saved.");
  } catch (err) {
    say($("message"), err.message, true);
  }
}

$("new").onclick = () => leave() && open("");
$("save").onclick = save;
$("confirm").oninput = () => { $("delete").disabled = $("confirm").value.trim() !== editing; };
$("delete").onclick = async () => {
  if (!editing || $("confirm").value.trim() !== editing) return;
  try {
    const answer = await post("/api/templates/delete", { type: editing });
    await reload();
    saved = editor.value;
    open(templates.length ? templates[0].type : "");
    say($("message"), "Deleted: it is in " + answer.trash + ".");
  } catch (err) {
    say($("message"), err.message, true);
  }
};
window.addEventListener("beforeunload", (event) => { if (editor.value !== saved) event.preventDefault(); });

reload().then(() => {
  const wanted = location.hash.slice(1);
  open(templates.some((t) => t.type === wanted) ? wanted : templates.length ? templates[0].type : "");
}).catch((err) => {
  state.error = err.message;
  frame(state);
});
