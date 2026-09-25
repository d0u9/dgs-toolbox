// What both doc pages share: the tree's state, the fields form, and how an
// Item is named. The server decides everything; the pages show what it said.

export const $ = (id) => document.getElementById(id);

export function el(tag, props, ...children) {
  const node = Object.assign(document.createElement(tag), props || {});
  node.append(...children.filter((c) => c !== null && c !== undefined && c !== false));
  return node;
}

export const size = (n) => n < 1024 ? n + " B" : n < 1048576 ? (n / 1024).toFixed(0) + " KB" : (n / 1048576).toFixed(1) + " MB";

export async function loadState() {
  const response = await fetch("/api/state");
  return response.json();
}

export async function post(url, body) {
  const response = await fetch(url, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
  });
  const answer = await response.json();
  if (!response.ok) throw new Error(answer.error || response.statusText);
  return answer;
}

export const templateOf = (state, type) => state.templates.find((t) => t.type === type);

// An Item is named by its type and its distinguishing fields — what makes it
// the one it is.
export function label(state, item) {
  const t = templateOf(state, item.type);
  const keys = t ? t.fields.filter((f) => f.distinguishing).map((f) => f.key) : Object.keys(item.fields);
  const parts = keys.map((k) => item.fields[k]).filter(Boolean);
  return item.type + (parts.length ? " · " + parts.join(" · ") : "");
}

export function inputFor(field, value, placeholder) {
  return el("label", { className: "form-field" },
    el("span", {}, field.key, field.required ? el("span", { className: "req" }, " *") : null),
    el("input", { name: field.key, value, placeholder, spellcheck: false, autocomplete: "off" }));
}

export const fieldsOf = (container) => Object.fromEntries(
  [...container.querySelectorAll("input")].map((input) => [input.name, input.value]));

// The top bar and the banner every page has.
export function frame(state) {
  $("root").textContent = state.root || "";
  $("banner").hidden = state.tree !== false;
  $("error").hidden = !state.error;
  $("error").textContent = state.error || "";
}

export function say(node, text, error) {
  node.className = error ? "message error" : "message";
  node.textContent = text;
}

// The text read off a PDF, drawn into the panel under the side bar. Reading
// can take a few seconds a page, so the answer is dropped when another PDF
// was picked in the meantime.
let textAsked = 0;
export async function showText(query, onAnswer) {
  const asked = ++textAsked;
  const box = $("text");
  box.hidden = false;
  say($("text-message"), "Reading the text…");
  $("text-body").textContent = "";
  let answer;
  try {
    const response = await fetch("/api/text?" + new URLSearchParams(query));
    answer = await response.json();
    if (!response.ok) throw new Error(answer.error || response.statusText);
  } catch (err) {
    answer = { error: err.message, suggestions: {} };
  }
  if (asked !== textAsked) return;
  if (answer.error) {
    say($("text-message"), answer.error, !answer.error.includes("needs macOS"));
  } else if (!answer.text) {
    say($("text-message"), "No text found.");
  } else {
    const recognised = (answer.pages || []).some((p) => p.source === "recognised");
    say($("text-message"), recognised ? "Recognised from the page: check it against the preview." : "From the PDF's own text.");
    $("text-body").textContent = answer.text;
  }
  if (onAnswer) onAnswer(answer);
}

// showPreview draws a PDF as pictures of its pages, which scroll smoothly,
// and falls back to the browser's viewer when a page is not a scan. query
// names the PDF as the API does; viewer is its URL for the viewer.
let previewAsked = 0;
export async function showPreview(query, viewer) {
  const asked = ++previewAsked;
  $("empty").hidden = true;
  const useViewer = () => {
    if (asked !== previewAsked) return;
    $("pages").hidden = true;
    $("pages").replaceChildren();
    $("frame").src = viewer;
    $("frame").hidden = false;
  };
  let info;
  try {
    const response = await fetch("/api/pages?" + new URLSearchParams(query));
    info = await response.json();
  } catch {
    info = { count: 0 };
  }
  if (asked !== previewAsked) return;
  if (!info.count) return useViewer();
  $("frame").hidden = true;
  $("frame").removeAttribute("src");
  const pages = [];
  for (let n = 1; n <= info.count; n++) {
    const img = el("img", {
      className: "page", alt: "Page " + n, loading: n <= 2 ? "eager" : "lazy", decoding: "async",
      src: "/api/page?" + new URLSearchParams({ ...query, n, v: info.digest }),
    });
    // A page with no picture of its own answers 204, which an image reads as
    // an error: the viewer draws the whole document instead.
    img.onerror = useViewer;
    pages.push(img);
  }
  $("pages").replaceChildren(...pages);
  $("pages").scrollTop = 0;
  $("pages").hidden = false;
}
