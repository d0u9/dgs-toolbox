// Templates: each type's file, edited as written. The server parses and
// checks it; a type Items use keeps its name and kind.
import { $, api, el, loadState, post, frame, say, statusBar } from "/common.js";
import { codeEditor, highlight } from "/ui/codeedit.js";
import { guardByName } from "/ui/confirm.js";
import { fileTree } from "/ui/filetree.js";

const armDelete = guardByName($("confirm"), $("delete"), "");
statusBar.setHints("<kbd>Ctrl</kbd>+<kbd>S</kbd> save", { html: true });

let state = {};
let templates = [];
let example = "";
let editing = null; // the type being edited, or "" for a new one
let saved = ""; // the text as last loaded or saved, to tell an edit
const closed = new Set(); // the type folders drawn shut

const editor = codeEditor($("yaml"), { onChange: dirty, onSave: save });

function dirty() {
  $("dirty").hidden = editor.value === saved;
}

function render() {
  frame(state);
  $("count").textContent = templates.length;
  // The types as a tree of what extends what: a type others extend is a
  // folder, opened by picking its row; one none extends is a file.
  const parents = new Set(templates.flatMap((t) => (t.lineage || []).slice(1)));
  const pathOf = (t) => [...(t.lineage || [t.type])].reverse().join("/") + (parents.has(t.type) ? "/" : "");
  const byPath = new Map(templates.map((t) => [pathOf(t), t]));
  const extra = (t) => t && el("span", { className: "template-meta" },
    t.abstract ? el("span", { className: "badge" }, "abstract") : el("span", { className: "template-sub numeric", title: t.items === 1 ? "1 Item" : t.items + " Items" }, String(t.items)));
  const files = templates.filter((t) => !parents.has(t.type)).map((t) => ({ path: pathOf(t), t }));
  const shown = templates.find((x) => x.type === editing);
  $("templates").replaceChildren(fileTree(files, {
    closed,
    folders: templates.filter((t) => parents.has(t.type)).map(pathOf),
    selected: shown ? pathOf(shown) : "",
    onPick: (f) => leave() && open(f.t.type),
    onPickFolder: (path) => leave() && open(byPath.get(path).type),
    fileExtra: (f) => extra(f.t),
    folderExtra: (path) => extra(byPath.get(path)),
    decorate: (row, { path }) => { const t = byPath.get(path); if (t) row.title = [t.type, t.kind, t.description].filter(Boolean).join(" · "); },
  }), ...(editing === "" ? [el("p", { className: "template-sub new-template" }, "new template · not saved yet")] : []));
  const t = templates.find((x) => x.type === editing);
  const file = t ? "templates/" + t.type + ".yaml" : "templates/<type>.yaml";
  $("title").textContent = t ? t.type : "New Template";
  $("description").textContent = t ? [t.lineage?.length > 1 ? "extends " + t.lineage.slice(1).join(" › ") : "", t.description].filter(Boolean).join(" · ") : "";
  $("file").textContent = file;
  $("file-again").textContent = file;
  $("danger").hidden = !t;
  $("duplicate").hidden = !t;
  $("confirm-name").textContent = t ? t.type : "";
  armDelete(t ? t.type : "");
  $("confirm").disabled = !!(t && t.items);
  $("delete-note").textContent = t && t.items
    ? `${t.items === 1 ? "1 Item uses" : t.items + " Items use"} it: delete ${t.items === 1 ? "that Item" : "them"} first.`
    : "Its file moves into the tree's trash folder; nothing is erased.";
}

// leave asks before an unsaved edit is dropped.
function leave() {
  return editor.value === saved || confirm("Drop the unsaved changes to this Template?");
}

// open shows a Template, or a new one: from the example, or from text given,
// which stays unsaved until Save.
function open(type, text) {
  editing = type;
  const t = templates.find((x) => x.type === type);
  saved = t ? t.data : example.replace("type: id_card", "type: new_type");
  editor.value = text ?? saved;
  dirty();
  say($("message"), "");
  history.replaceState(null, "", type ? "#" + type : location.pathname + location.search);
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

// Duplicate starts a new Template from the one shown, as saved, under a type
// no Template has yet. Nothing is written until Save.
$("duplicate").onclick = () => {
  const from = editing;
  if (!from || !leave()) return;
  let type = from + "_copy";
  for (let n = 2; templates.some((t) => t.type === type); n++) type = from + "_copy" + n;
  const text = /^type:.*$/m.test(saved) ? saved.replace(/^type:.*$/m, "type: " + type) : "type: " + type + "\n" + saved;
  open("", text);
  say($("message"), "A copy of " + from + ": change what differs, then Save.");
};
$("save").onclick = save;
$("delete").onclick = async () => {
  if (!editing || $("confirm").value !== editing) return;
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
