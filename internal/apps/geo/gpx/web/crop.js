// Cropping a photo before it is encoded. The crop frame stays put and the
// picture moves under it: dragging pans the picture, the wheel or the slider
// zooms it, and dragging a corner of the frame resizes the frame, held to the
// chosen ratio unless it is free. The picture always covers the frame. The
// answer is { ratio, rect } with rect in the picture's own pixels, null when
// the photo is skipped, or { cancelled: true } when adding is given up.
import { formDialog } from "/ui/dialog.js";

export const RATIOS = [["5:4", 5 / 4], ["4:5", 4 / 5], ["1:1", 1], ["4:3", 4 / 3], ["3:2", 3 / 2], ["16:9", 16 / 9], ["free", 0], ["none", -1]];

const MARGIN = 24; // room around the frame, in CSS pixels
const HANDLE = 16; // how near a corner a press takes hold of it
const MIN_FRAME = 40;
const MAX_ZOOM = 4; // CSS pixels per picture pixel

export function cropDialog(bitmap, name, ratio) {
  const choice = document.createElement("select"); choice.className = "field"; choice.setAttribute("aria-label", "Crop ratio");
  for (const [value] of RATIOS) {
    const option = document.createElement("option"); option.value = value;
    option.textContent = value === "free" ? "Free" : value === "none" ? "No crop" : value;
    choice.append(option);
  }
  choice.value = RATIOS.some(([value]) => value === ratio) ? ratio : "5:4";
  const zoom = document.createElement("input"); zoom.type = "range"; zoom.min = 0; zoom.max = 1000; zoom.className = "crop-zoom"; zoom.setAttribute("aria-label", "Zoom");

  const canvas = document.createElement("canvas"); canvas.className = "crop-picture";
  // The dialog's padding is left out of the width; CSS still narrows the
  // canvas, keeping its shape, rather than let the dialog scroll.
  const W = Math.round(Math.min(1300, window.innerWidth * 0.96 - 120));
  const H = Math.round(Math.max(240, Math.min(820, window.innerHeight * 0.92 - 220)));
  const dpr = window.devicePixelRatio || 1;
  canvas.width = W * dpr; canvas.height = H * dpr;
  canvas.style.width = `${W}px`;
  const g = canvas.getContext("2d");
  const iw = bitmap.width, ih = bitmap.height;

  let frame, z, ox, oy; // frame in CSS pixels; the picture drawn at z from (ox, oy)
  const value = () => RATIOS.find(([v]) => v === choice.value)[1];
  const minZoom = () => Math.max(frame.w / iw, frame.h / ih);
  const zoomRange = () => [minZoom(), Math.max(minZoom(), MAX_ZOOM)];
  // keep holds the picture over the frame, however it was moved or zoomed.
  const keep = () => {
    const [lo, hi] = zoomRange();
    z = Math.min(Math.max(z, lo), hi);
    ox = Math.min(frame.x, Math.max(frame.x + frame.w - iw * z, ox));
    oy = Math.min(frame.y, Math.max(frame.y + frame.h - ih * z, oy));
    zoom.value = hi > lo ? Math.round(Math.log(z / lo) / Math.log(hi / lo) * 1000) : 0;
  };
  // zoomAt scales the picture to z = to around one point of the canvas.
  const zoomAt = (to, px, py) => {
    const [lo, hi] = zoomRange();
    const next = Math.min(Math.max(to, lo), hi);
    ox = px - (px - ox) * (next / z);
    oy = py - (py - oy) * (next / z);
    z = next;
    keep();
    draw();
  };
  const reset = () => {
    const r = value(), fit = Math.min((W - 2 * MARGIN) / iw, (H - 2 * MARGIN) / ih);
    let w = iw * fit, h = ih * fit;
    if (r > 0) {
      w = Math.min(W - 2 * MARGIN, (H - 2 * MARGIN) * r);
      h = w / r;
    }
    frame = { x: (W - w) / 2, y: (H - h) / 2, w, h };
    z = minZoom();
    ox = (W - iw * z) / 2; oy = (H - ih * z) / 2;
    keep();
    zoom.disabled = r < 0;
    draw();
  };
  // corners are [x, y, xDirection, yDirection], the directions pointing inward.
  const corners = () => [
    [frame.x, frame.y, 1, 1], [frame.x + frame.w, frame.y, -1, 1],
    [frame.x, frame.y + frame.h, 1, -1], [frame.x + frame.w, frame.y + frame.h, -1, -1],
  ];
  const cornerAt = (x, y) => value() >= 0 && corners().find(([cx, cy]) => Math.abs(x - cx) < HANDLE && Math.abs(y - cy) < HANDLE);
  const draw = () => {
    g.setTransform(dpr, 0, 0, dpr, 0, 0);
    g.fillStyle = "#1a1a1a"; g.fillRect(0, 0, W, H);
    g.drawImage(bitmap, ox, oy, iw * z, ih * z);
    if (value() < 0) return;
    g.fillStyle = "rgb(0 0 0 / 0.6)";
    g.beginPath(); g.rect(0, 0, W, H); g.rect(frame.x, frame.y, frame.w, frame.h); g.fill("evenodd");
    g.strokeStyle = "rgb(255 255 255 / 0.35)"; g.lineWidth = 1;
    g.beginPath();
    for (const t of [1 / 3, 2 / 3]) {
      g.moveTo(frame.x + frame.w * t, frame.y); g.lineTo(frame.x + frame.w * t, frame.y + frame.h);
      g.moveTo(frame.x, frame.y + frame.h * t); g.lineTo(frame.x + frame.w, frame.y + frame.h * t);
    }
    g.stroke();
    g.strokeStyle = "#fff"; g.lineWidth = 2; g.strokeRect(frame.x, frame.y, frame.w, frame.h);
    g.lineWidth = 4;
    for (const [cx, cy, sx, sy] of corners()) {
      g.beginPath(); g.moveTo(cx + 14 * sx, cy); g.lineTo(cx, cy); g.lineTo(cx, cy + 14 * sy); g.stroke();
    }
  };
  const at = (event) => {
    const bounds = canvas.getBoundingClientRect();
    return [(event.clientX - bounds.left) * W / bounds.width, (event.clientY - bounds.top) * H / bounds.height];
  };

  let drag = null;
  canvas.addEventListener("pointerdown", (event) => {
    if (value() < 0) return;
    const [x, y] = at(event);
    const corner = cornerAt(x, y);
    // A corner is resized against the one opposite it, which stays put.
    drag = corner
      ? { anchor: [corner[0] + frame.w * corner[2], corner[1] + frame.h * corner[3]] }
      : { x, y, ox, oy };
    canvas.setPointerCapture(event.pointerId);
    event.preventDefault();
  });
  canvas.addEventListener("pointermove", (event) => {
    const [x, y] = at(event);
    if (!drag) {
      const corner = cornerAt(x, y);
      canvas.style.cursor = corner ? (corner[2] === corner[3] ? "nwse-resize" : "nesw-resize") : value() < 0 ? "default" : "grab";
      return;
    }
    if (drag.anchor) {
      const [ax, ay] = drag.anchor, r = Math.max(0, value()), edge = MARGIN / 2;
      const px = Math.min(W - edge, Math.max(edge, x)), py = Math.min(H - edge, Math.max(edge, y));
      let w = Math.max(MIN_FRAME, Math.abs(px - ax)), h = Math.max(MIN_FRAME, Math.abs(py - ay));
      if (r) {
        if (w / r > h) h = w / r; else w = h * r;
        const room = Math.min((px < ax ? ax - edge : W - edge - ax) / w, (py < ay ? ay - edge : H - edge - ay) / h, 1);
        w *= room; h *= room;
      }
      frame = { x: px < ax ? ax - w : ax, y: py < ay ? ay - h : ay, w, h };
    } else {
      ox = drag.ox + x - drag.x;
      oy = drag.oy + y - drag.y;
      canvas.style.cursor = "grabbing";
    }
    keep();
    draw();
  });
  canvas.addEventListener("pointerup", () => { drag = null; canvas.style.cursor = "grab"; });
  canvas.addEventListener("wheel", (event) => {
    if (value() < 0) return;
    event.preventDefault();
    const [x, y] = at(event);
    zoomAt(z * Math.exp(-event.deltaY * 0.002), x, y);
  }, { passive: false });
  zoom.addEventListener("input", () => {
    const [lo, hi] = zoomRange();
    zoomAt(lo * (hi / lo) ** (zoom.value / 1000), frame.x + frame.w / 2, frame.y + frame.h / 2);
  });
  choice.addEventListener("change", reset);
  reset();

  // Cancel gives up on every photo still to be added, not just this one.
  let cancelled = false;
  const cancel = document.createElement("button"); cancel.type = "button"; cancel.className = "button crop-cancel"; cancel.textContent = "Cancel";
  cancel.addEventListener("click", () => { cancelled = true; cancel.closest("dialog").close(); });
  const head = document.createElement("div"); head.className = "crop-head"; head.append(choice, zoom, cancel);
  return formDialog({
    title: `Crop ${name}`, message: "Drag to move the photo, scroll or use the slider to zoom, drag a corner to resize the crop.",
    fields: [head, canvas], focus: choice, confirm: "Use", cancel: "Skip",
    read: () => {
      if (value() < 0) return { ratio: choice.value, rect: { x: 0, y: 0, w: iw, h: ih } };
      const x = Math.max(0, Math.round((frame.x - ox) / z)), y = Math.max(0, Math.round((frame.y - oy) / z));
      return { ratio: choice.value, rect: { x, y, w: Math.min(iw - x, Math.round(frame.w / z)), h: Math.min(ih - y, Math.round(frame.h / z)) } };
    },
  }).then((answer) => (cancelled ? { cancelled: true } : answer));
}
