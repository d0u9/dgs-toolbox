// Drag handles that resize the pane beside them. Sizes are remembered per
// browser; the layout works without them.

const PREFIX = "dgs-gpx-size-";

// splitter makes handle resize target along axis ("y": height, "x": width).
// invert is for a target after the handle, which grows as the handle moves
// back. max is a function so it can follow the window.
export function splitter({ handle, target, axis = "y", invert = false, min = 60, max = () => Infinity, key }) {
  const property = axis === "y" ? "height" : "width";
  const clamp = (size) => Math.round(Math.max(min, Math.min(max(), size)));
  const apply = (size) => {
    target.style[property] = `${clamp(size)}px`;
    target.style.flex = "none";
  };

  const saved = Number(read(key));
  if (saved) apply(saved);

  handle.addEventListener("pointerdown", (event) => {
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
      write(key, Math.round(target.getBoundingClientRect()[property]));
    };
    handle.addEventListener("pointermove", move);
    handle.addEventListener("pointerup", up);
    handle.addEventListener("pointercancel", up);
  });

  // Keep a remembered size inside a window that got smaller.
  window.addEventListener("resize", () => {
    if (target.style[property]) apply(parseFloat(target.style[property]));
  });
}

function read(key) {
  try {
    return localStorage.getItem(PREFIX + key);
  } catch {
    return null;
  }
}

function write(key, value) {
  try {
    localStorage.setItem(PREFIX + key, String(value));
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
