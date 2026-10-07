// ViewGestures owns the map's view gestures, the same under every tool:
//
//   pan              Space+drag always; a plain drag when the tool does not
//                    claim the drag; arrow keys
//   zoom             wheel or pinch, at the pointer; + and -
//   rotate and tilt  right-button drag; Shift+arrow keys
//
// Everything else belongs to the tools: left click and, when a tool claims
// it, left drag; double-click; Shift, Alt/Option and Cmd/Ctrl with a click or
// drag; a right click without a drag. So the map's own double-click zoom and
// Shift+drag box zoom stay off.
export class ViewGestures {
  constructor(map) {
    this.map = map;
    this.toolDrag = false; // whether the tool in use claims the left-button drag
    this.space = false; // Space is held
    this.over = false; // the pointer is over the map
    this.panning = false;
    map.doubleClickZoom.disable();
    map.boxZoom.disable();
    const container = map.getCanvasContainer();
    container.addEventListener("pointerenter", () => { this.over = true; });
    container.addEventListener("pointerleave", () => { this.over = false; });
    window.addEventListener("keydown", (event) => this.key(event, true));
    window.addEventListener("keyup", (event) => this.key(event, false));
    window.addEventListener("blur", () => this.setSpace(false));
    // Capture on the outer container, ahead of the tools' and the map's own handling.
    map.getContainer().addEventListener("pointerdown", (event) => this.down(event), true);
  }

  // setToolDrag says whether the tool in use claims the left-button drag. A
  // tool that does takes plain drags; Space+drag still pans.
  setToolDrag(claimed) {
    this.toolDrag = claimed;
    if (claimed) this.map.dragPan.disable();
    else this.map.dragPan.enable();
  }

  key(event, down) {
    if (event.code !== "Space") return;
    // Space pans only from over the map, and never while typing.
    if (down && (!this.over || event.target.closest?.("input, textarea, select, [contenteditable]"))) return;
    if (!down && !this.space) return;
    event.preventDefault();
    this.setSpace(down);
  }

  setSpace(held) {
    this.space = held;
    this.map.getCanvasContainer().classList.toggle("view-pan-ready", held);
  }

  down(event) {
    if (!this.space || event.button !== 0 || !this.map.getCanvasContainer().contains(event.target)) return;
    // The pan is this gesture alone: no tool and no map handler sees the press.
    event.preventDefault();
    event.stopImmediatePropagation();
    const container = this.map.getCanvasContainer();
    container.classList.add("view-panning");
    this.panning = true;
    try {
      container.setPointerCapture(event.pointerId);
    } catch {
      // Without capture the pan still ends when the pointer is released over the map.
    }
    let last = [event.clientX, event.clientY];
    const move = (e) => {
      this.map.panBy([last[0] - e.clientX, last[1] - e.clientY], { duration: 0 });
      last = [e.clientX, e.clientY];
    };
    const swallow = (e) => { e.stopImmediatePropagation(); };
    const up = () => {
      container.removeEventListener("pointermove", move);
      container.removeEventListener("pointerup", up);
      container.removeEventListener("pointercancel", up);
      container.classList.remove("view-panning");
      this.panning = false;
      // The click that ends a pan is not a tool's click.
      this.map.getContainer().addEventListener("click", swallow, { capture: true, once: true });
      setTimeout(() => this.map.getContainer().removeEventListener("click", swallow, true));
    };
    container.addEventListener("pointermove", move);
    container.addEventListener("pointerup", up);
    container.addEventListener("pointercancel", up);
  }
}
