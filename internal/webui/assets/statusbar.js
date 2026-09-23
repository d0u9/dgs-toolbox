// The shared status bar: the one row along the bottom of a page where every
// piece of news lands. It is the web half of the TUI shell's status bar, and
// carries the same three parts — state on the left, the contextual summary
// and the news in the middle, key hints on the right — so a reader looks in
// one place whichever face of dgs they are using.
//
// What belongs here is a one-off event: what was saved, renamed or refused.
// The lasting state of one thing — a track that has no sidecar, a route leg
// that would not route — stays beside that thing, where it can carry its own
// Retry.
//
// The bar is one row and never wraps. A message longer than the row is cut
// with an ellipsis and kept whole in the title, so the pointer still reads it.

let bar = null; // the one status bar on the page, or null

// mount puts the bar at the end of a parent, body by default, and returns it.
// A page calls this once, after its own markup. Calling it again returns the
// bar already there.
export function mount(parent = document.body) {
  if (bar) return bar;
  bar = document.createElement("footer");
  bar.className = "statusbar";
  bar.setAttribute("role", "status");
  bar.setAttribute("aria-live", "polite");
  for (const part of ["state", "message", "hints"]) {
    const span = document.createElement("span");
    span.className = `status-${part}`;
    bar.append(span);
  }
  parent.append(bar);
  return bar;
}

// setState writes the left part: what the page is doing or showing now, such
// as the mode it is in. It is not news and never clears itself. With html set
// the text is markup the page built itself, never anything a user typed.
export function setState(text, { html = false } = {}) {
  write("state", text, html);
}

// The middle holds one line at a time, and two things want it: the summary of
// what is on screen now, and the last thing that happened. News wins while it
// is the newest thing the reader has been told; the summary comes back when
// the news is cleared or the summary itself changes, because a summary
// changing means the reader has just done something else.
let summary = ""; // the standing line, as HTML
let message = null; // the news over it, or null

// setSummary writes the middle part's standing line: what the page is showing
// or what the tool in hand does now, such as what a click on the map would
// do. It may carry <strong> and <kbd>, because a key is named as a key.
export function setSummary(html) {
  summary = html || "";
  message = null;
  middle();
}

// show writes the middle part: the last thing that happened. An error is
// drawn in the error colour. A message stays until the next one replaces it,
// because the reader may have been looking elsewhere when it arrived.
export function show(text, { error = false } = {}) {
  message = text ? { text, error: Boolean(error) } : null;
  middle();
}

// middle draws whichever of the two the middle part is showing now.
function middle() {
  if (!bar) return;
  const span = bar.querySelector(".status-message");
  span.classList.toggle("error", Boolean(message?.error));
  if (message) {
    span.textContent = message.text;
    span.title = message.text;
    return;
  }
  span.innerHTML = summary;
  if (summary) span.title = span.textContent;
  else span.removeAttribute("title");
}

// showError is show with the error colour, for the common case.
export function showError(text) {
  show(text, { error: true });
}

// clear takes the news away; the middle goes back to its standing summary.
export function clear() {
  show("");
}

// setHints writes the right part: the keys or the reading that belong to what
// is on screen now. html is as for setState.
export function setHints(text, { html = false } = {}) {
  write("hints", text, html);
}

function write(part, text, html = false) {
  if (!bar) return null;
  const span = bar.querySelector(`.status-${part}`);
  if (html) span.innerHTML = text || "";
  else span.textContent = text || "";
  if (text) span.title = span.textContent;
  else span.removeAttribute("title");
  return span;
}
