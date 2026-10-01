// The shared tooltip: what describes a thing, shown beside it while the
// pointer rests on it or the keyboard is on it. It replaces the browser's
// title, which comes late, cannot be styled and cuts long text.
//
//   tooltip(node, lines)
//
// lines is a list of strings: the first is drawn as a heading, the rest
// beneath it, one per line; empty ones are left out. The tip opens a moment
// after the pointer comes to rest, at once when another tip has just closed
// so a reader running down a list is not kept waiting, and closes when the
// pointer or focus leaves, on a click, on Escape and on a scroll. Calling it
// again on a node replaces its lines.

const DELAY = 350; // ms the pointer rests before a tip opens
const WARM = 400; // ms after a tip closes during which the next opens at once

let tip = null; // the one tip element, made when first wanted
let owner = null; // the node whose tip is shown
let timer = 0;
let closedAt = 0;
const linesOf = new WeakMap();

export function tooltip(node, lines) {
  const kept = lines.filter(Boolean);
  node.removeAttribute("title");
  if (!kept.length) { linesOf.delete(node); return; }
  const first = !linesOf.has(node);
  linesOf.set(node, kept);
  if (owner === node) draw(node);
  if (!first) return;
  node.addEventListener("pointerenter", () => soon(node));
  node.addEventListener("pointerleave", () => hide(node));
  node.addEventListener("focus", () => soon(node));
  node.addEventListener("blur", () => hide(node));
  node.addEventListener("pointerdown", () => hide(node));
}

function soon(node) {
  clearTimeout(timer);
  if (!linesOf.has(node)) return;
  if (Date.now() - closedAt < WARM) show(node);
  else timer = setTimeout(() => show(node), DELAY);
}

function show(node) {
  if (!node.isConnected || !linesOf.has(node)) return;
  if (!tip) {
    tip = document.createElement("div");
    tip.className = "tooltip";
    tip.setAttribute("role", "tooltip");
    tip.hidden = true;
    document.body.append(tip);
  }
  owner = node;
  draw(node);
  tip.hidden = false;
  place(node);
  listen(true);
}

function draw(node) {
  const [head, ...rest] = linesOf.get(node) || [];
  const line = (cls, text) => { const div = document.createElement("div"); div.className = cls; div.textContent = text; return div; };
  tip.replaceChildren(line("tooltip-head", head), ...rest.map((text) => line("tooltip-line", text)));
}

// place puts the tip beside the node, to its right, or to its left when the
// window has no room there, or below it when neither has; always inside the
// window.
function place(node) {
  const r = node.getBoundingClientRect();
  const { width, height } = tip.getBoundingClientRect();
  const gap = 6, margin = 4;
  let left = r.right + gap, top = r.top;
  if (left + width > window.innerWidth - margin) left = r.left - gap - width;
  if (left < margin) { left = r.left; top = r.bottom + gap; }
  tip.style.left = Math.max(margin, Math.min(left, window.innerWidth - width - margin)) + "px";
  tip.style.top = Math.max(margin, Math.min(top, window.innerHeight - height - margin)) + "px";
}

function hide(node) {
  clearTimeout(timer);
  if (node && owner !== node) return;
  if (owner) closedAt = Date.now();
  owner = null;
  if (tip) tip.hidden = true;
  listen(false);
}

const onScroll = () => hide();
// A page that redraws a list takes the node away under the pointer, and a
// node taken away hears no pointerleave: the next pointer move closes it.
const onOver = (event) => { if (owner && (!owner.isConnected || !owner.contains(event.target))) hide(); };
const onKey = (event) => { if (event.key === "Escape") hide(); };

function listen(on) {
  const method = on ? "addEventListener" : "removeEventListener";
  window[method]("scroll", onScroll, true);
  window[method]("resize", onScroll);
  window[method]("keydown", onKey, true);
  document[method]("pointerover", onOver, true);
}
