// The doc page: the Items kept in the tree, the loose PDFs beside them, and
// importing a loose PDF with a Template. The server decides everything; the
// page shows what it said.
(() => {
  const $ = (id) => document.getElementById(id);
  let state = { templates: [], items: [], loose: [] };
  let selected = null; // {kind: "loose", path} or {kind: "item", id, digest}

  const size = (n) => n < 1024 ? n + " B" : n < 1048576 ? (n / 1024).toFixed(0) + " KB" : (n / 1048576).toFixed(1) + " MB";
  const el = (tag, props, ...children) => {
    const node = Object.assign(document.createElement(tag), props || {});
    node.append(...children.filter((c) => c !== null && c !== undefined));
    return node;
  };
  const words = () => $("filter").value.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const matches = (text) => words().every((w) => text.toLowerCase().includes(w));
  const template = (type) => state.templates.find((t) => t.type === type);

  // An Item is named by its type and its distinguishing fields — what makes
  // it the one it is.
  function label(item) {
    const t = template(item.type);
    const keys = t ? t.fields.filter((f) => f.distinguishing).map((f) => f.key) : Object.keys(item.fields);
    const parts = keys.map((k) => item.fields[k]).filter(Boolean);
    return item.type + (parts.length ? " · " + parts.join(" · ") : "");
  }

  function render() {
    $("root").textContent = state.root || "";
    $("banner").hidden = state.tree !== false;
    $("error").hidden = !state.error;
    $("error").textContent = state.error || "";

    const items = state.items.filter((i) => matches(label(i) + " " + Object.values(i.fields).join(" ")));
    $("items-head").hidden = $("items").hidden = !state.tree;
    $("items-count").textContent = items.length;
    $("items").replaceChildren(...items.map((item) => {
      const li = el("li", { onclick: () => pick({ kind: "item", id: item.id, digest: item.head || item.revisions[0].digest }) },
        label(item),
        item.kind === "document" && item.revisions.length > 1 ? el("span", { className: "tag" }, item.revisions.length + " revisions") : null,
        el("span", { className: "sub" }, Object.entries(item.fields).map(([k, v]) => k + ": " + v).join("  ")));
      if (selected && selected.kind === "item" && selected.id === item.id) li.className = "selected";
      return li;
    }));

    const loose = state.loose.filter((f) => matches(f.path));
    $("loose-count").textContent = loose.length;
    $("loose-empty").hidden = state.loose.length > 0;
    $("loose").replaceChildren(...loose.map((f) => {
      const cut = f.path.lastIndexOf("/");
      const li = el("li", { onclick: () => pick({ kind: "loose", path: f.path }) },
        cut >= 0 ? el("span", { className: "sub" }, f.path.slice(0, cut + 1)) : null,
        f.path.slice(cut + 1),
        f.item ? el("span", { className: "tag" }, "in tree") : null,
        el("span", { className: "sub" }, size(f.size) + " · " + new Date(f.modified).toLocaleDateString()));
      if (selected && selected.kind === "loose" && selected.path === f.path) li.className = "selected";
      return li;
    }));
    side();
  }

  function pick(choice) {
    const same = selected && JSON.stringify(selected) === JSON.stringify(choice);
    selected = choice;
    if (!same) {
      $("frame").src = choice.kind === "loose"
        ? "/api/file?path=" + encodeURIComponent(choice.path)
        : "/api/revision?item=" + encodeURIComponent(choice.id) + "&digest=" + encodeURIComponent(choice.digest);
      $("import-message").textContent = "";
      if (choice.kind === "loose") drawFields();
    }
    $("frame").hidden = false;
    $("empty").hidden = true;
    render();
  }

  function side() {
    const loose = selected && selected.kind === "loose" ? state.loose.find((f) => f.path === selected.path) : null;
    const item = selected && selected.kind === "item" ? state.items.find((i) => i.id === selected.id) : null;
    const importing = loose && state.tree && !loose.item;
    $("import").hidden = !importing;
    $("detail").hidden = !item && !(loose && loose.item);
    $("side").hidden = $("import").hidden && $("detail").hidden;
    const shown = item || (loose && state.items.find((i) => i.id === loose.item));
    if (shown) detail(shown);
  }

  let editing = "";

  function detail(item) {
    $("detail-head").textContent = label(item);
    const t = template(item.type);
    if (editing !== item.id + JSON.stringify(item.fields)) {
      editing = item.id + JSON.stringify(item.fields);
      $("edit-message").textContent = "";
      $("edit-fields").replaceChildren(el("p", { className: "kind" }, item.kind),
        ...(t ? t.fields : Object.keys(item.fields).map((key) => ({ key }))).map((f) => inputFor(f, item.fields[f.key] || "", "")));
      $("save-button").disabled = !t;
      $("edit").dataset.item = item.id;
    }
    $("revisions").replaceChildren(...item.revisions.slice().reverse().map((r) => {
      const isHead = r.digest === item.head;
      const li = el("li", { onclick: () => pick({ kind: "item", id: item.id, digest: r.digest }) },
        item.kind === "document" && !isHead ? el("button", {
          className: "small", type: "button", textContent: "Make HEAD",
          onclick: (event) => { event.stopPropagation(); makeHead(item.id, r.digest); },
        }) : null,
        new Date(r.added).toLocaleString(),
        isHead ? el("span", { className: "tag" }, "HEAD") : null,
        el("span", { className: "sub" }, (r.source ? r.source + " · " : "") + r.digest.slice(0, 12)));
      if (selected && selected.kind === "item" && selected.digest === r.digest) li.className = "selected";
      return li;
    }));
  }

  function inputFor(f, value, placeholder) {
    return el("label", { className: "field" },
      el("span", {}, f.key, f.required ? el("span", { className: "req" }, " *") : null),
      el("input", { name: f.key, value, placeholder, spellcheck: false, autocomplete: "off" }));
  }

  const fieldsOf = (container) => Object.fromEntries(
    [...container.querySelectorAll("input")].map((input) => [input.name, input.value]));

  async function post(url, body) {
    const response = await fetch(url, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
    });
    const answer = await response.json();
    if (!response.ok) throw new Error(answer.error || response.statusText);
    return answer;
  }

  async function makeHead(id, digest) {
    $("head-message").className = "message";
    $("head-message").textContent = "";
    try {
      await post("/api/head", { item: id, digest });
      await load();
      pick({ kind: "item", id, digest });
    } catch (err) {
      $("head-message").className = "message error";
      $("head-message").textContent = err.message;
    }
  }

  $("edit").onsubmit = async (event) => {
    event.preventDefault();
    $("edit-message").className = "message";
    try {
      await post("/api/fields", { item: $("edit").dataset.item, fields: fieldsOf($("edit-fields")) });
      await load();
      $("edit-message").textContent = "Saved.";
    } catch (err) {
      $("edit-message").className = "message error";
      $("edit-message").textContent = err.message;
    }
  };

  function drawFields() {
    const select = $("template");
    const current = select.value;
    select.replaceChildren(...state.templates.map((t) => el("option", { value: t.type }, t.type + " (" + t.kind + ")")));
    if (current && template(current)) select.value = current;
    const t = template(select.value);
    $("import-button").disabled = !t;
    if (!t) {
      $("fields").replaceChildren(el("p", { className: "message" }, "No Templates in templates/."));
      return;
    }
    const into = $("into");
    const was = into.value;
    const documents = t.kind === "document" ? state.items.filter((i) => i.type === t.type) : [];
    into.replaceChildren(el("option", { value: "" }, "A new item"),
      ...documents.map((i) => el("option", { value: i.id }, "New revision of " + label(i))));
    if (documents.some((i) => i.id === was)) into.value = was;
    $("into-field").hidden = documents.length === 0;
    const adding = into.value !== "";
    $("import-button").textContent = adding ? "Add revision" : "Import";
    $("fields").replaceChildren(...(adding ? [] : t.fields.map((f) => inputFor(f, "", (t.defaults || {})[f.key] || ""))));
  }

  $("template").onchange = drawFields;
  $("into").onchange = drawFields;
  $("filter").oninput = render;
  $("import").onsubmit = async (event) => {
    event.preventDefault();
    $("import-button").disabled = true;
    $("import-message").className = "message";
    $("import-message").textContent = "Copying and reading back…";
    try {
      const body = $("into").value
        ? await post("/api/revisions", { path: selected.path, item: $("into").value })
        : await post("/api/import", { path: selected.path, type: $("template").value, fields: fieldsOf($("fields")) });
      await load();
      pick({ kind: "item", id: body.id, digest: body.head || body.revisions[0].digest });
    } catch (err) {
      $("import-message").className = "message error";
      $("import-message").textContent = err.message;
    } finally {
      $("import-button").disabled = false;
    }
  };

  async function load() {
    const response = await fetch("/api/state");
    state = await response.json();
    render();
  }
  load().catch((err) => {
    state.error = err.message;
    render();
  });
})();
