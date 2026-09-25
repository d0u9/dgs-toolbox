// The page viewer: a PDF's pages as pictures down one column, a strip of
// thumbnails beside it, zoom and pan. Pictures come from the server at the
// page size and, once zoomed past it, at the large size, so a zoomed page
// stays sharp without every page costing the large size up front.

const $ = (id) => document.getElementById(id);
const ZOOM_KEY = "dgs-doc-zoom";
const MIN = 0.25, MAX = 6;
// The page picture is 1600 px on its long side; past this many device pixels
// across, the large picture is asked for.
const SHARP = 1500;

// zoom is "width" (fit the column's width), "page" (a whole page in view) or
// a scale of fit-width.
let zoom = remembered();
let sheets = [];

function remembered() {
  try {
    const v = localStorage.getItem(ZOOM_KEY);
    if (v === "width" || v === "page") return v;
    const n = Number(v);
    return n >= MIN && n <= MAX ? n : "width";
  } catch { return "width"; }
}
function remember() {
  try { localStorage.setItem(ZOOM_KEY, String(zoom)); } catch { /* not kept */ }
}

// The width a sheet is at fit-width: the column, less its padding, but not
// wider than reads comfortably.
function fitWidth() {
  const pages = $("pages");
  return Math.max(120, Math.min(pages.clientWidth - 32, 900));
}

// widthFor is a sheet's width in pixels at the current zoom.
function widthFor(sheet) {
  if (zoom === "width") return fitWidth();
  if (zoom === "page") {
    const ratio = Number(sheet.dataset.ratio) || 1.414; // height / width
    return Math.min(fitWidth(), ($("pages").clientHeight - 32) / ratio);
  }
  return fitWidth() * zoom;
}

// scale is the current zoom as a number, for the label and for stepping.
function scale() {
  return sheets.length ? widthFor(sheets[0]) / fitWidth() : 1;
}

// apply sizes every sheet, keeping the point at (x, y) of the column where
// it was when anchor is given.
function apply(anchor) {
  const pages = $("pages");
  const before = pages.scrollHeight, beforeW = pages.scrollWidth;
  const ax = anchor ? anchor.x : pages.clientWidth / 2, ay = anchor ? anchor.y : 0;
  const fx = (pages.scrollLeft + ax) / Math.max(1, beforeW), fy = (pages.scrollTop + ay) / Math.max(1, before);
  for (const sheet of sheets) {
    const width = widthFor(sheet);
    sheet.style.width = width + "px";
    const img = sheet.querySelector("img.page");
    if (width * devicePixelRatio > SHARP && img.dataset.large && img.src !== img.dataset.large) {
      img.src = img.dataset.large;
    }
  }
  if (anchor) {
    pages.scrollLeft = fx * pages.scrollWidth - ax;
    pages.scrollTop = fy * pages.scrollHeight - ay;
  }
  $("zoom-level").textContent = Math.round(scale() * 100) + "%";
  $("fit-width").classList.toggle("on", zoom === "width");
  $("fit-page").classList.toggle("on", zoom === "page");
}

function setZoom(next, anchor) {
  zoom = typeof next === "number" ? Math.min(MAX, Math.max(MIN, next)) : next;
  remember();
  apply(anchor);
}

// show lays out count pages; pictureURL(n, size) names page n's picture.
export function show(count, pictureURL, onMissing) {
  $("viewer").hidden = false;
  const pages = $("pages");
  sheets = [];
  const thumbs = [];
  for (let n = 1; n <= count; n++) {
    const sheet = document.createElement("div");
    sheet.className = "sheet";
    sheet.dataset.page = n - 1;
    const img = document.createElement("img");
    img.className = "page";
    img.alt = "Page " + n;
    img.decoding = "async";
    img.loading = n <= 2 ? "eager" : "lazy";
    img.dataset.large = pictureURL(n, "large");
    // Each page holds a sheet of A4's shape until its picture arrives, so the
    // column and the scrollbar are right before anything has loaded.
    img.style.aspectRatio = "1 / 1.414";
    img.src = pictureURL(n, "page");
    // A page with no picture of its own answers 204, which an image reads
    // as an error.
    img.onerror = onMissing;
    img.onload = () => {
      if (img.naturalWidth) {
        sheet.dataset.ratio = img.naturalHeight / img.naturalWidth;
        img.style.aspectRatio = `${img.naturalWidth} / ${img.naturalHeight}`;
      }
      if (zoom === "page") apply();
    };
    const layer = document.createElement("div");
    layer.className = "text-layer";
    sheet.append(img, layer);
    sheets.push(sheet);

    const thumb = document.createElement("button");
    thumb.type = "button";
    thumb.className = "thumb";
    thumb.title = "Page " + n;
    const small = document.createElement("img");
    small.loading = "lazy";
    small.alt = "";
    small.src = pictureURL(n, "thumb");
    const label = document.createElement("span");
    label.textContent = n;
    thumb.append(small, label);
    thumb.onclick = () => sheet.scrollIntoView({ block: "start", behavior: "smooth" });
    thumbs.push(thumb);
  }
  pages.replaceChildren(...sheets);
  $("rail").replaceChildren(...thumbs);
  $("rail").hidden = count < 2;
  pages.scrollTop = 0;
  pages.scrollLeft = 0;
  apply();
  watch();
  return sheets;
}

export function hide() {
  $("viewer").hidden = true;
  $("rail").hidden = true;
  $("pages").replaceChildren();
  $("rail").replaceChildren();
  sheets = [];
}

// watch marks the page most in view, in the strip and the bar.
let observer = null;
function watch() {
  if (observer) observer.disconnect();
  const seen = new Map();
  observer = new IntersectionObserver((entries) => {
    for (const e of entries) seen.set(e.target, e.intersectionRatio);
    let best = null, most = 0;
    for (const [sheet, ratio] of seen) if (ratio > most) { most = ratio; best = sheet; }
    if (!best) return;
    const n = Number(best.dataset.page);
    $("page-at").textContent = sheets.length > 1 ? `${n + 1} / ${sheets.length}` : "";
    [...$("rail").children].forEach((t, i) => {
      t.classList.toggle("current", i === n);
      if (i === n && !$("rail").hidden) t.scrollIntoView({ block: "nearest" });
    });
  }, { root: $("pages"), threshold: [0, 0.25, 0.5, 0.75, 1] });
  sheets.forEach((s) => observer.observe(s));
}

// Zooming: the buttons, Ctrl or ⌘ with the wheel (a trackpad pinch arrives as
// that), and + − 0 from the keyboard.
const step = (factor, anchor) => setZoom(scale() * factor, anchor);
$("zoom-in").onclick = () => step(1.25);
$("zoom-out").onclick = () => step(0.8);
$("zoom-level").onclick = () => setZoom(1);
$("fit-width").onclick = () => setZoom("width");
$("fit-page").onclick = () => setZoom("page");

$("pages").addEventListener("wheel", (event) => {
  if (!event.ctrlKey && !event.metaKey) return;
  event.preventDefault();
  const box = $("pages").getBoundingClientRect();
  step(Math.exp(-event.deltaY * 0.002), { x: event.clientX - box.left, y: event.clientY - box.top });
}, { passive: false });

document.addEventListener("keydown", (event) => {
  if ($("viewer").hidden || event.target.closest("input, select, textarea")) return;
  if (event.key === "+" || event.key === "=") step(1.25);
  else if (event.key === "-") step(0.8);
  else if (event.key === "0") setZoom(1);
  else return;
  event.preventDefault();
});

// Panning: drag the page. A drag that starts on text selects it instead.
let drag = null;
$("pages").addEventListener("pointerdown", (event) => {
  if (event.button !== 0 || event.target.closest(".text-line")) return;
  const pages = $("pages");
  drag = { x: event.clientX, y: event.clientY, left: pages.scrollLeft, top: pages.scrollTop };
  pages.setPointerCapture(event.pointerId);
  pages.classList.add("panning");
});
$("pages").addEventListener("pointermove", (event) => {
  if (!drag) return;
  $("pages").scrollLeft = drag.left - (event.clientX - drag.x);
  $("pages").scrollTop = drag.top - (event.clientY - drag.y);
});
const endDrag = () => { drag = null; $("pages").classList.remove("panning"); };
$("pages").addEventListener("pointerup", endDrag);
$("pages").addEventListener("pointercancel", endDrag);

new ResizeObserver(() => { if (sheets.length && typeof zoom !== "number") apply(); }).observe($("pages"));
