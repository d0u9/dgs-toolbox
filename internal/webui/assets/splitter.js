// Drag handles that resize the pane beside them, for every page dgs serves.
// Sizes are remembered per browser under key, the whole localStorage name, so
// each page names its own; the layout works without them.

// splitter makes handle resize target along axis ("y": height, "x": width).
// invert is for a target after the handle, which grows as the handle moves
// back. max is a function so it can follow the window.
//
// fallback, when given, is the size before anything is remembered, and a
// double-click on the handle goes back to it. step is how far an arrow key
// moves a focused handle. set, when given, applies a size in place of the
// target's own width or height — a grid column held in a CSS variable, say;
// the target is still what is measured.
export function splitter({ handle, target, axis = "y", invert = false, min = 60, max = () => Infinity, key, fallback = 0, step = 24, set = null }) {
  const property = axis === "y" ? "height" : "width";
  const clamp = (size) => Math.round(Math.max(min, Math.min(max(), size)));
  let size = 0;
  const apply = (next) => {
    size = clamp(next);
    if (set) set(size);
    else {
      target.style[property] = `${size}px`;
      target.style.flex = "none";
    }
    handle.setAttribute("aria-valuenow", String(size));
    handle.setAttribute("aria-valuemin", String(min));
    if (Number.isFinite(max())) handle.setAttribute("aria-valuemax", String(Math.round(max())));
  };

  const saved = Number(read(key));
  if (saved) apply(saved);
  else if (fallback) apply(fallback);
  if (!handle.hasAttribute("tabindex")) handle.tabIndex = 0;

  handle.addEventListener("pointerdown", (event) => {
    if (event.button !== 0) return;
    event.preventDefault();
    handle.setPointerCapture(event.pointerId);
    handle.classList.add("dragging");
    document.body.classList.add(axis === "y" ? "resizing-y" : "resizing-x");
    const start = axis === "y" ? event.clientY : event.clientX;
    const startSize = target.getBoundingClientRect()[property];
    const move = (moveEvent) => {
      const delta = (axis === "y" ? moveEvent.clientY : moveEvent.clientX) - start;
      apply(startSize + (invert ? -delta : delta));
    };
    const up = () => {
      handle.removeEventListener("pointermove", move);
      handle.removeEventListener("pointerup", up);
      handle.removeEventListener("pointercancel", up);
      handle.classList.remove("dragging");
      document.body.classList.remove("resizing-y", "resizing-x");
      write(key, size || Math.round(target.getBoundingClientRect()[property]));
    };
    handle.addEventListener("pointermove", move);
    handle.addEventListener("pointerup", up);
    handle.addEventListener("pointercancel", up);
  });

  if (fallback) {
    handle.addEventListener("dblclick", () => {
      apply(fallback);
      write(key, size);
    });
  }

  // A focused handle moves with the arrow keys, so the mouse is not needed.
  // The page's own keys are not reached from here: the handle takes them.
  handle.addEventListener("keydown", (event) => {
    const keys = axis === "y" ? { ArrowUp: -1, ArrowDown: 1 } : { ArrowLeft: -1, ArrowRight: 1 };
    const direction = keys[event.key];
    if (!direction) return;
    event.preventDefault();
    event.stopPropagation();
    const now = size || target.getBoundingClientRect()[property];
    apply(now + direction * step * (invert ? -1 : 1));
    write(key, size);
  });

  // Keep a remembered size inside a window that got smaller.
  window.addEventListener("resize", () => {
    if (size) apply(size);
  });
}

function read(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key, value) {
  try {
    localStorage.setItem(key, String(value));
  } catch {
    // Storage can be unavailable; the size simply is not remembered.
  }
}

// shareSplitter divides a container between first and second by a remembered
// share rather than a size, so both keep their proportion as the container
// grows or shrinks. handles maps each axis to its drag handle; axis() says
// which one is in use, and each axis remembers its own share.
export function shareSplitter({ handles, first, second, container, axis, min = 60, key }) {
  const shares = { x: Number(read(`${key}-x`)) || 0.5, y: Number(read(`${key}-y`)) || 0.5 };
  const apply = () => {
    const share = shares[axis()];
    first.style.flex = `${share} 1 0`;
    second.style.flex = `${1 - share} 1 0`;
  };
  for (const [name, handle] of Object.entries(handles)) {
    handle.addEventListener("pointerdown", (event) => {
      event.preventDefault();
      handle.setPointerCapture(event.pointerId);
      handle.classList.add("dragging");
      document.body.classList.add(name === "y" ? "resizing-y" : "resizing-x");
      const property = name === "y" ? "height" : "width";
      const move = (moveEvent) => {
        const box = container.getBoundingClientRect();
        const total = box[property];
        if (total <= 2 * min) return;
        const offset = name === "y" ? moveEvent.clientY - box.top : moveEvent.clientX - box.left;
        shares[name] = Math.max(min, Math.min(total - min, offset)) / total;
        apply();
      };
      const up = () => {
        handle.removeEventListener("pointermove", move);
        handle.removeEventListener("pointerup", up);
        handle.removeEventListener("pointercancel", up);
        handle.classList.remove("dragging");
        document.body.classList.remove("resizing-y", "resizing-x");
        write(`${key}-${name}`, shares[name].toFixed(4));
      };
      handle.addEventListener("pointermove", move);
      handle.addEventListener("pointerup", up);
      handle.addEventListener("pointercancel", up);
    });
  }
  apply();
  return { apply };
}
