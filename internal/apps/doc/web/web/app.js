// The M1 page: list the PDFs under the tree, show the one picked. It asks the
// server for nothing but the listing and the file; it changes nothing.
(() => {
  const list = document.getElementById("files");
  const filter = document.getElementById("filter");
  const count = document.getElementById("count");
  const message = document.getElementById("message");
  const frame = document.getElementById("frame");
  const empty = document.getElementById("empty");
  let files = [];
  let selected = "";

  const size = (n) => n < 1024 ? n + " B" : n < 1048576 ? (n / 1024).toFixed(0) + " KB" : (n / 1048576).toFixed(1) + " MB";

  function render() {
    const words = filter.value.trim().toLowerCase().split(/\s+/).filter(Boolean);
    const shown = files.filter((f) => words.every((w) => f.path.toLowerCase().includes(w)));
    count.textContent = shown.length === files.length ? files.length : shown.length + " / " + files.length;
    list.replaceChildren(...shown.map((f) => {
      const li = document.createElement("li");
      const cut = f.path.lastIndexOf("/");
      if (cut >= 0) {
        const dir = document.createElement("span");
        dir.className = "dir";
        dir.textContent = f.path.slice(0, cut + 1);
        li.append(dir);
      }
      li.append(f.path.slice(cut + 1) + " ");
      const meta = document.createElement("span");
      meta.className = "meta";
      meta.textContent = size(f.size) + " · " + new Date(f.modified).toLocaleDateString();
      li.append(meta);
      if (f.path === selected) li.className = "selected";
      li.onclick = () => show(f.path);
      return li;
    }));
    message.textContent = files.length === 0 ? "No PDFs in this folder." : "";
  }

  function show(path) {
    selected = path;
    frame.src = "/api/file?path=" + encodeURIComponent(path);
    frame.hidden = false;
    empty.hidden = true;
    render();
  }

  filter.oninput = render;
  fetch("/api/config").then((r) => r.json()).then((c) => {
    document.getElementById("root").textContent = c.root;
  });
  fetch("/api/files").then(async (r) => {
    const body = await r.json();
    if (!r.ok) throw new Error(body.error || r.statusText);
    files = body;
    render();
  }).catch((err) => {
    message.className = "message error";
    message.textContent = err.message;
  });
})();
