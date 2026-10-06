// Dragging a row by holding it. A row of the workspace carries its own
// controls — an eye, a colour, a disclosure — so it has no drag handle: a
// press held in place takes hold of the row, and until then the press is a
// click like any other. The gesture is here; what a drop means is the
// caller's.

// A mouse takes hold of the row as soon as it travels SLOP pixels: a mouse
// press never scrolls, so there is nothing to tell apart. A finger or pen
// must hold still for HOLD first, since moving at once is a scroll.
const HOLD = 220;
const SLOP = 4;

// longPressDrag binds the gesture to one element.
//   onStart(event)  the row is taken hold of
//   onMove(x, y, event)  the pointer moved while holding it
//   onDrop(x, y, event)  the pointer was let go over that point
//   onCancel()  Escape, a cancelled pointer, or the press never became a hold
// A drag swallows the click that would otherwise follow it, so holding a row
// does not also select it.
export function longPressDrag(element, { onStart, onMove, onDrop, onCancel }) {
  element.addEventListener("pointerdown", (event) => {
    if (event.button !== 0) return;
    // The row's own controls take their press first.
    if (event.target.closest?.("button, input, select, a, .parts-group")) return;
    const startX = event.clientX, startY = event.clientY;
    const mouse = event.pointerType === "mouse";
    let holding = false;
    let ghost = null;
    const take = () => {
      timer = null;
      holding = true;
      // Capture keeps the moves coming to this row even over the map; a
      // pointer already gone leaves nothing to capture.
      try {
        element.setPointerCapture?.(event.pointerId);
      } catch {
        // The press ended before the hold; the drop below finds that too.
      }
      ghost = makeGhost(element, startX, startY);
      onStart?.(event);
    };
    let timer = mouse ? null : setTimeout(take, HOLD);

    const stop = () => {
      if (timer) clearTimeout(timer);
      timer = null;
      ghost?.remove();
      ghost = null;
      element.removeEventListener("pointermove", move);
      element.removeEventListener("pointerup", up);
      element.removeEventListener("pointercancel", cancel);
      document.removeEventListener("keydown", key, true);
    };
    const move = (e) => {
      if (!holding) {
        const travelled = Math.abs(e.clientX - startX) > SLOP || Math.abs(e.clientY - startY) > SLOP;
        if (!travelled) return;
        if (!mouse) {
          // A touch that travels before the hold is a scroll, not a drag.
          stop();
          onCancel?.();
          return;
        }
        take();
      }
      e.preventDefault();
      ghost?.follow(e.clientX, e.clientY);
      onMove?.(e.clientX, e.clientY, e);
    };
    const up = (e) => {
      const dragged = holding;
      stop();
      if (!dragged) return onCancel?.();
      swallowNextClick();
      onDrop?.(e.clientX, e.clientY, e);
    };
    const cancel = () => {
      const dragged = holding;
      stop();
      if (dragged) swallowNextClick();
      onCancel?.();
    };
    const key = (e) => {
      if (e.key !== "Escape" || !holding) return;
      e.preventDefault();
      e.stopPropagation();
      cancel();
    };
    element.addEventListener("pointermove", move);
    element.addEventListener("pointerup", up);
    element.addEventListener("pointercancel", cancel);
    document.addEventListener("keydown", key, true);
  });
}

// makeGhost lifts a copy of the row that follows the pointer, so the hand
// visibly carries what it holds. It ignores the pointer, so what lies under
// it is still what a drop lands on. The copy goes into the row's own list,
// where the list's styles still reach it; placed fixed, it is drawn where the
// pointer is all the same.
export function makeGhost(element, x, y) {
  const box = element.getBoundingClientRect();
  const node = element.cloneNode(true);
  node.removeAttribute("id");
  node.classList.add("drag-ghost");
  Object.assign(node.style, {
    position: "fixed", left: "0", top: "0", margin: "0", zIndex: "1000",
    width: `${box.width}px`, height: `${box.height}px`, boxSizing: "border-box",
    pointerEvents: "none", listStyle: "none",
  });
  const dx = x - box.left, dy = y - box.top;
  const follow = (px, py) => {
    node.style.transform = `translate(${px - dx}px, ${py - dy}px)`;
  };
  follow(x, y);
  (element.parentElement ?? document.body).appendChild(node);
  return { follow, remove: () => node.remove() };
}

function swallowNextClick() {
  const eat = (event) => {
    event.preventDefault();
    event.stopPropagation();
  };
  document.addEventListener("click", eat, { capture: true, once: true });
  // Nothing may be left listening if no click follows the drop at all.
  setTimeout(() => document.removeEventListener("click", eat, { capture: true }), 300);
}

// SortGap is the sorting both lists share: while a row is carried, the rows
// between where it was and where it would land slide aside, so the gap it
// would drop into stands open. The held rows fade out, their copy riding
// under the pointer. Where the pointer points is read against the rows as
// they lay when the drag began, so rows sliding under it do not move the
// target back and forth.
export class SortGap {
  // rows are every row of the list in order; held the contiguous ones carried.
  constructor(rows, held) {
    this.rows = rows;
    this.list = rows[0]?.parentElement ?? null;
    this.from = rows.indexOf(held[0]);
    this.to = this.from + held.length;
    const base = this.origin();
    this.boxes = rows.map((row) => {
      const box = row.getBoundingClientRect();
      return { top: box.top - base, bottom: box.bottom - base };
    });
    this.height = held.length ? this.boxes[this.to - 1].bottom - this.boxes[this.from].top : 0;
    for (const row of rows) row.classList.add("sort-row");
    for (const row of held) row.classList.add("sort-held");
  }

  // origin is where the list starts on screen, less its own scroll, so a
  // point is placed against the rows however the list or the page scrolls.
  origin() {
    if (!this.list) return 0;
    return this.list.getBoundingClientRect().top - this.list.scrollTop;
  }

  // at is the row under a screen height as the rows first lay, and whether
  // the point is in its upper half; null outside the list.
  at(y) {
    const at = y - this.origin();
    const index = this.boxes.findIndex((box) => at >= box.top && at < box.bottom);
    if (index < 0) return null;
    const box = this.boxes[index];
    return { row: this.rows[index], index, above: at < (box.top + box.bottom) / 2 };
  }

  // slot is where the rows would be put for a point: the index of the row
  // they would go before, rows.length for the end.
  slot(y) {
    const at = y - this.origin();
    const index = this.boxes.findIndex((box) => at < (box.top + box.bottom) / 2);
    return index < 0 ? this.rows.length : index;
  }

  // open slides the rows so the gap stands before rows[index]; undefined or
  // null closes it back to where the rows came from.
  open(index) {
    const t = index ?? this.from;
    this.rows.forEach((row, i) => {
      let shift = 0;
      if (t > this.to && i >= this.to && i < t) shift = -this.height;
      if (t < this.from && i >= t && i < this.from) shift = this.height;
      row.style.transform = shift ? `translateY(${shift}px)` : "";
    });
  }

  // close puts every row back as it was.
  close() {
    for (const row of this.rows) {
      row.classList.remove("sort-row", "sort-held");
      row.style.transform = "";
    }
  }
}
