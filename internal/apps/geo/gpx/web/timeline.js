// The focused track's timeline: moving and stopped periods against clock time,
// with gaps where nothing was recorded. Scrubbing it inspects the point at that
// moment; the wheel zooms it and Option/Alt-drag pans it, with an overview strip and
// ticks that follow. The periods come from the server's stops; this only lays
// them out.

import * as format from "./format.js";
import { pieceColor } from "./cut.js";
import { stopTitle } from "./stops.js";

export class Timeline {
  // onScrub(index | null) is called while the pointer moves over the timeline;
  // onStop(index) when a stop is clicked; onStopHover(index | null) when the
  // pointer enters or leaves one; onView([from, to] | null) when zooming or
  // panning changes the visible time, null meaning the whole track.
  constructor({ root, bar, labels, overview, ticks, onScrub, onStop, onStopHover, onView }) {
    Object.assign(this, { root, bar, labels, overview, ticks, onScrub, onStop, onStopHover, onView });
    this.view = null; // [from, to] in Unix milliseconds, or null for the whole track
    this.window = document.createElement("div");
    this.window.className = "timeline-window";
    overview.append(this.window);
    this.bindZoom();
    bar.title = `Wheel to zoom · ${format.keys.shift}-wheel or ${format.keys.alt}-drag to pan · double-click for the whole track`;
    this.track = null;
    this.timed = []; // indices of timed points, in time order
    this.timeZone = format.browserTimeZone();
    this.cursor = document.createElement("div");
    this.cursor.className = "timeline-cursor";
    this.cursor.hidden = true;
    this.pointerX = null;
    this.wheelMode = null;
    this.wheelAnchor = null; // zoom only; pan never has a pointer anchor
    this.panSpan = null;
    this.wheelAnchorTimer = null;
    this.shiftDown = false;
    document.addEventListener("keydown", (event) => {
      if (event.key === "Shift") this.shiftDown = true;
    });
    document.addEventListener("keyup", (event) => {
      if (event.key === "Shift") this.shiftDown = false;
    });
    window.addEventListener("blur", () => {
      this.shiftDown = false;
      this.clearWheelAnchor();
    });

    this.cutting = null;
    this.editing = null;
    this.menu = document.createElement("div");
    this.menu.className = "range-menu";
    this.menu.hidden = true;
    root.append(this.menu);
    document.addEventListener("pointerdown", (event) => {
      if (!this.menu.hidden && !this.menu.contains(event.target)) this.menu.hidden = true;
    });
    bar.addEventListener("pointerdown", (event) => {
      if (event.target.closest(".timeline-cut, .timeline-candidate, .timeline-removed")) return;
      if (event.altKey) return; // Alt-drag pans; see bindZoom
      if (this.editing?.rangeTool) {
        this.selectRange(event);
        return;
      }
      if (event.target.closest(".timeline-stop") && !this.cutting) return;
      // While cutting, a click cuts at the point under the pointer; a drag
      // still scrubs.
      if (this.cutting) this.clickToCut(event);
      try {
        bar.setPointerCapture(event.pointerId);
      } catch {
        // The scrub still follows the pointer while it stays over the bar.
      }
      this.scrub(event);
    });
    bar.addEventListener("pointermove", (event) => {
      this.pointerX = event.clientX;
      this.scrub(event);
    });
    bar.addEventListener("pointerleave", (event) => {
      if (event.buttons) return; // still dragging: pointer capture keeps the scrub
      this.pointerX = null;
      this.setIndex(null);
      this.onScrub(null);
    });
  }

  // hiddenRanges are point index ranges [first, last] drawn as hidden.
  show(track, color, hiddenRanges = []) {
    if (this.track?.path !== track.path) this.view = null;
    this.track = track;
    this.color = color;
    this.hiddenRanges = hiddenRanges;
    // Points cleaning removed are not scrubbed to.
    this.timed = track.time.map((t, i) => (t == null || track.removed[i] ? -1 : i)).filter((i) => i >= 0)
      .sort((a, b) => track.time[a] - track.time[b]);
    this.full = this.timed.length < 2 ? null : [track.time[this.timed[0]], track.time[this.timed[this.timed.length - 1]]];
    this.minSpan = this.full ? this.computeMinimumSpan() : 1;
    this.render();
  }

  hide() {
    cancelAnimationFrame(this.frame);
    this.frame = null;
    this.clearWheelAnchor();
    this.track = null;
    this.full = null; // nothing to zoom until a track is shown again
    this.onStopHover(null);
    this.bar.replaceChildren();
  }

  setTimeZone(timeZone) {
    this.timeZone = timeZone;
    if (this.track) this.render();
  }

  render() {
    const { track, bar } = this;
    this.cursor.hidden = true;
    if (this.timed.length < 2) {
      const empty = document.createElement("div");
      empty.className = "timeline-empty";
      empty.textContent = "No time in this file, so no timeline";
      bar.replaceChildren(empty);
      this.labels.replaceChildren();
      this.root.classList.add("untimed");
      return;
    }
    this.root.classList.remove("untimed");
    this.range = this.clampView(this.view) || this.full;
    const [start, end] = this.range;
    const blocks = [];

    // A moving block per recorded segment: from its first to its last timed point.
    const spans = new Map();
    for (const i of this.timed) {
      const span = spans.get(track.segments[i]) || [Infinity, -Infinity];
      spans.set(track.segments[i], [Math.min(span[0], track.time[i]), Math.max(span[1], track.time[i])]);
    }
    for (const [from, to] of spans.values()) {
      blocks.push(this.block("timeline-moving", from, to, { background: this.color }));
    }
    track.stops.forEach((stop, i) => {
      const block = this.block("timeline-stop", stop.arrival, stop.departure);
      block.textContent = String(i + 1);
      block.title = stopTitle(i, stop, this.timeZone);
      block.addEventListener("click", () => {
        if (!this.cutting) this.onStop(i); // while cutting, a click cuts instead
      });
      block.addEventListener("mouseenter", () => this.onStopHover(i));
      block.addEventListener("mouseleave", () => this.onStopHover(null));
      blocks.push(block);
    });
    for (const [first, last] of this.hiddenRanges || []) {
      const times = track.time.slice(first, last + 1).filter((t) => t != null);
      if (!times.length) continue;
      const block = this.block("timeline-hidden", Math.min(...times), Math.max(...times));
      block.title = "Hidden";
      blocks.push(block);
    }
    // Manual removals are part of the track, not merely editing chrome. Keep
    // them visible after the Edit panel closes; editing only enables their
    // context menu.
    blocks.push(...this.removedElements());
    if (this.cutting) blocks.push(...this.cutElements());
    bar.replaceChildren(...blocks, this.cursor);

    this.renderTicks();
    this.renderOverview();
    const zoomed = this.isZoomed();
    const startLabel = document.createElement("span");
    startLabel.textContent = format.clock(start, this.timeZone, { seconds: zoomed });
    const legend = document.createElement("span");
    legend.className = "timeline-legend";
    const count = track.stops.length === 1 ? "1 stop" : `${track.stops.length} stops`;
    legend.textContent = `${count} · ${format.duration(stopped(track) / 1000) || "0m"} stopped`;
    const endLabel = document.createElement("span");
    endLabel.textContent = format.clock(end, this.timeZone, { seconds: zoomed });
    this.labels.replaceChildren(startLabel, legend, endLabel);
  }

  block(className, from, to, style = {}) {
    const [start, end] = this.range;
    const element = document.createElement("div");
    element.className = className;
    element.style.left = `${((from - start) / (end - start)) * 100}%`;
    element.style.width = `${((to - from) / (end - start)) * 100}%`;
    Object.assign(element.style, style);
    return element;
  }

  // ---- zoom ----

  isZoomed() {
    return Boolean(this.view && this.full && (this.range[0] > this.full[0] || this.range[1] < this.full[1]));
  }

  // clampView keeps a view inside the track and at least the minimum span wide; null
  // when it covers the whole track.
  clampView(view) {
    if (!view || !this.full) return null;
    const [fullStart, fullEnd] = this.full;
    const span = Math.min(fullEnd - fullStart, Math.max(this.minSpan, view[1] - view[0]));
    const from = Math.max(fullStart, Math.min(view[0], fullEnd - span));
    const to = from + span;
    if (to >= fullEnd && from <= fullStart) return null;
    return [from, to];
  }

  // setView shows a stretch of time, or with null the whole track. It does not
  // report back through onView: whoever calls it already knows.
  setView(view) {
    // This view replaces any the wheel has yet to draw and report.
    cancelAnimationFrame(this.frame);
    this.frame = null;
    this.view = this.clampView(view);
    if (this.track) this.render();
  }

  changeView(view) {
    const next = this.clampView(view);
    const same = next === this.view || (next && this.view && next[0] === this.view[0] && next[1] === this.view[1]);
    if (same) return;
    // A trackpad sends wheel events faster than the screen refreshes, and each
    // redraw also redraws the charts. Take the new view at once, so the next
    // event builds on it, but draw and report it once per frame.
    this.view = next;
    this.range = next || this.full;
    if (this.frame) return;
    this.frame = requestAnimationFrame(() => {
      this.frame = null;
      this.render();
      this.onView(this.view);
    });
  }

  bindZoom() {
    const { bar, overview } = this;
    bar.addEventListener("wheel", (event) => {
      if (!this.full) return;
      event.preventDefault();
      const [from, to] = this.range;
      const span = to - from;
      const rect = bar.getBoundingClientRect();
      // A mouse wheel may count in lines or pages rather than pixels.
      const unit = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? rect.width : 1;
      const dx = event.deltaX * unit, dy = event.deltaY * unit;
      // Lock one event burst to pan or zoom. macOS may report Shift only on the
      // first frame and may alternate deltaX/deltaY later in the same gesture.
      // A trackpad pinch arrives as a wheel event with ctrlKey set.
      const pinch = event.ctrlKey;
      let mode = this.wheelMode;
      if (pinch) mode = "zoom";
      else if (event.shiftKey || this.shiftDown) mode = "pan";
      else if (!mode) mode = Math.abs(dx) > Math.abs(dy) * 1.25 ? "pan" : "zoom";
      if (mode === "pan") {
        this.wheelMode = "pan";
        // The span is immutable for the whole pan. Constructing both ends
        // independently from every wheel frame allowed tiny floating-point
        // and mode-classification changes to alter the overview width.
        // Crucially, panning owns no sticky pointer/time anchor.
        if (this.panSpan == null) {
          this.wheelAnchor = null;
          this.panSpan = span;
        }
        const pan = Math.abs(dx) > Math.abs(dy) ? dx : dy;
        if (!pan) {
          this.keepWheelGesture();
          return;
        }
        const lockedSpan = this.panSpan;
        const shift = (pan / rect.width) * lockedSpan;
        this.changeView([from + shift, from + shift + lockedSpan]);
        this.keepWheelGesture();
        return;
      }
      if (!dy) return;
      this.wheelMode = "zoom";
      this.panSpan = null;
      // WebKit can report a stale wheel-event x during a trackpad gesture.
      // Pointer movement is the authoritative cursor position when available.
      const clientX = this.pointerX ?? event.clientX;
      const fraction = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
      // One trackpad gesture is a burst of wheel events. Preserve both its
      // screen position and the time initially under it for the whole burst;
      // deriving the anchor anew after every render allows rounding and event
      // coordinate noise to make the content sway under a stationary cursor.
      if (!this.wheelAnchor || Math.abs(this.wheelAnchor.clientX - clientX) > 2) {
        this.wheelAnchor = { clientX, fraction, time: from + fraction * span };
      }
      const anchor = this.wheelAnchor;
      // Trackpads send a stream of small deltas while mouse wheels tend to send
      // fewer large ones. An exponential scale makes both continuous and keeps
      // the time under the pointer stationary.
      // Cap momentum spikes and use a gentler scale. A trackpad's later
      // inertial frames often carry much larger deltas than its first frames;
      // without the cap zoom feels slow at first and abruptly fast afterward.
      // Pinch deltas are about a tenth of a two-finger scroll's.
      // Ctrl with a mouse wheel reads as a pinch too; the lower cap keeps its
      // larger deltas from leaping.
      const factor = pinch
        ? Math.exp(Math.max(-10, Math.min(10, dy)) * 0.01)
        : Math.exp(Math.max(-60, Math.min(60, dy)) * 0.001);
      const nextSpan = span * factor;
      this.changeView([
        anchor.time - anchor.fraction * nextSpan,
        anchor.time + (1 - anchor.fraction) * nextSpan,
      ]);
      this.keepWheelGesture();
    }, { passive: false });

    bar.addEventListener("dblclick", (event) => {
      if (event.target.closest(".timeline-stop, .timeline-cut, .timeline-candidate")) return;
      this.clearWheelAnchor();
      this.changeView(null);
    });

    const drag = (element, perPixel) => (event) => {
      event.preventDefault();
      event.stopPropagation();
      const startX = event.clientX;
      const startRange = [...this.range];
      try {
        element.setPointerCapture(event.pointerId);
      } catch {
        // The pan still follows the pointer while it stays over the element.
      }
      const move = (e) => {
        const shift = (e.clientX - startX) * perPixel();
        this.changeView([startRange[0] + shift, startRange[1] + shift]);
      };
      const up = () => {
        element.removeEventListener("pointermove", move);
        element.removeEventListener("pointerup", up);
        element.removeEventListener("pointercancel", up);
      };
      element.addEventListener("pointermove", move);
      element.addEventListener("pointerup", up);
      element.addEventListener("pointercancel", up);
    };
    // Alt-drag on the bar moves the time under the pointer with it.
    bar.addEventListener("pointerdown", (event) => {
      if (!event.altKey || !this.full) return;
      drag(bar, () => -(this.range[1] - this.range[0]) / bar.getBoundingClientRect().width)(event);
    });
    // Dragging the overview's window moves it across the whole track.
    this.window.addEventListener("pointerdown", (event) => {
      if (!this.full) return;
      drag(this.window, () => (this.full[1] - this.full[0]) / overview.getBoundingClientRect().width)(event);
    });
    // Clicking the overview outside the window centres the view there.
    overview.addEventListener("pointerdown", (event) => {
      if (event.target !== overview || !this.full) return;
      const rect = overview.getBoundingClientRect();
      const at = this.full[0] + ((event.clientX - rect.left) / rect.width) * (this.full[1] - this.full[0]);
      const half = (this.range[1] - this.range[0]) / 2;
      this.changeView([at - half, at + half]);
    });
  }

  clearWheelAnchor() {
    clearTimeout(this.wheelAnchorTimer);
    this.wheelAnchorTimer = null;
    this.wheelMode = null;
    this.wheelAnchor = null;
    this.panSpan = null;
  }

  keepWheelGesture() {
    clearTimeout(this.wheelAnchorTimer);
    // Panning needs a longer idle tail: WebKit can omit Shift on later frames,
    // and a chart redraw can leave a noticeable gap between those frames.
    const idle = this.wheelMode === "pan" ? 600 : 180;
    this.wheelAnchorTimer = setTimeout(() => this.clearWheelAnchor(), idle);
  }

  // The minimum time span is the shortest recorded time needed to travel one
  // metre, matching the profile charts' spatial zoom floor.
  // It scans the whole track, so show() computes it once.
  computeMinimumSpan() {
    if (!this.track || this.timed.length < 2) return 1;
    let best = Infinity;
    let right = 1;
    for (let left = 0; left < this.timed.length - 1; left++) {
      if (right <= left) right = left + 1;
      const from = this.timed[left];
      while (right < this.timed.length && this.track.distance[this.timed[right]] - this.track.distance[from] < MIN_DISTANCE) right++;
      if (right >= this.timed.length) break;
      const elapsed = this.track.time[this.timed[right]] - this.track.time[from];
      if (elapsed > 0) best = Math.min(best, elapsed);
    }
    return Number.isFinite(best) ? best : this.full[1] - this.full[0];
  }

  renderOverview() {
    const zoomed = this.isZoomed();
    this.overview.classList.toggle("inactive", !zoomed);
    if (!zoomed) return;
    const [fullStart, fullEnd] = this.full;
    const span = fullEnd - fullStart;
    this.window.style.left = `${((this.range[0] - fullStart) / span) * 100}%`;
    this.window.style.width = `${Math.max(0.5, ((this.range[1] - this.range[0]) / span) * 100)}%`;
  }

  // renderTicks marks round times in the page's time zone across the view.
  renderTicks() {
    const [from, to] = this.range;
    const width = this.bar.clientWidth || 600;
    const wanted = (to - from) / Math.max(2, width / 90); // about one tick per 90 px
    const step = TICK_STEPS.find((s) => s >= wanted) || TICK_STEPS[TICK_STEPS.length - 1];
    // Round in local time: shift by the zone's offset, round, shift back.
    const offset = format.offsetMillis(from, this.timeZone);
    const first = Math.ceil((from + offset) / step) * step - offset;
    const withSeconds = step < MINUTE;
    const labels = [];
    // Ticks too near an end would be cut off; the end labels above say those times.
    const margin = (to - from) * 0.03;
    for (let t = first; t <= to && labels.length < 60; t += step) {
      if (t < from + margin || t > to - margin) continue;
      const tick = document.createElement("span");
      tick.className = "timeline-tick";
      tick.style.left = this.percent(t);
      const clock = format.clock(t, this.timeZone, { seconds: withSeconds });
      const time = clock.slice(11);
      // A day's first tick, or every tick a day or more apart, shows the date.
      tick.textContent = step >= DAY || time.startsWith("00:00") ? clock.slice(5, 10) : time;
      labels.push(tick);
    }
    this.ticks.replaceChildren(...labels);
  }

  // clickToCut cuts where a press is released without moving.
  clickToCut(event) {
    const startX = event.clientX;
    const up = (e) => {
      window.removeEventListener("pointerup", up, true);
      if (Math.abs(e.clientX - startX) > 3 || !this.cutting) return;
      const index = this.indexAt(e.clientX);
      if (index != null) this.cutting.onAdd(index);
    };
    window.addEventListener("pointerup", up, true);
  }

  // setEditing shows removals by hand and lets them be edited, or with null
  // hides them. edits are the sidecar's edits; ranges and stops show as blocks;
  // rangeTool makes dragging select a range to remove; onRange(first, last),
  // onJoin(index, join) and onRestore(index) report edits.
  setEditing(editing) {
    this.editing = editing;
    this.menu.hidden = true;
    this.root.classList.toggle("range-tool", Boolean(editing?.rangeTool));
    if (this.track) this.render();
  }

  removedElements() {
    const edits = this.track?.clean?.params?.edits || [];
    const { onJoin, onRestore } = this.editing || {};
    const elements = [];
    edits.forEach((range, index) => {
      if (range.kind === "lasso") return; // scattered points, shown on the map
      const times = this.track.time.slice(range.first, range.last + 1).filter((t) => t != null);
      if (!times.length) return;
      const from = Math.min(...times), to = Math.max(...times);
      const block = this.block("timeline-removed" + (range.join ? " join" : ""), from, to);
      const description = `${range.kind === "stop" ? "Stop" : "Range"} removed by hand, ${range.join ? "joined across" : "broken"}`;
      block.title = this.editing ? `${description} — click to change or restore` : description;
      if (this.editing) {
        block.addEventListener("click", (event) => {
          event.stopPropagation();
          this.openMenu(event.clientX, [
            [range.join ? "Break here" : "Join across", () => onJoin(index, !range.join)],
            ["Restore these points", () => onRestore(index)],
          ]);
        });
      }
      elements.push(block);
    });
    return elements;
  }

  openMenu(clientX, items) {
    this.menu.replaceChildren(...items.map(([text, action]) => {
      const button = document.createElement("button");
      button.textContent = text;
      button.addEventListener("click", () => {
        this.menu.hidden = true;
        action();
      });
      return button;
    }));
    const box = this.root.getBoundingClientRect();
    const barBox = this.bar.getBoundingClientRect();
    this.menu.hidden = false;
    this.menu.style.left = `${Math.min(clientX - box.left, box.width - this.menu.offsetWidth)}px`;
    this.menu.style.top = `${barBox.bottom - box.top + 2}px`;
  }

  // selectRange drags out a stretch of the timeline and removes its points.
  selectRange(event) {
    event.preventDefault();
    const band = document.createElement("div");
    band.className = "timeline-selection";
    this.bar.append(band);
    const rect = this.bar.getBoundingClientRect();
    const startX = event.clientX;
    try {
      this.bar.setPointerCapture(event.pointerId);
    } catch {
      // Without capture the selection ends when the pointer leaves the bar.
    }
    const clampX = (x) => Math.max(rect.left, Math.min(rect.right, x));
    const draw = (x) => {
      const left = Math.min(startX, clampX(x)) - rect.left;
      band.style.left = `${left}px`;
      band.style.width = `${Math.abs(clampX(x) - startX)}px`;
    };
    draw(startX);
    const move = (e) => {
      draw(e.clientX);
      const index = this.indexAt(e.clientX);
      this.setIndex(index);
      this.onScrub(index);
    };
    const up = (e) => {
      this.bar.removeEventListener("pointermove", move);
      this.bar.removeEventListener("pointerup", up);
      this.bar.removeEventListener("pointercancel", up);
      band.remove();
      if (Math.abs(clampX(e.clientX) - startX) < 3) return;
      const a = this.indexAt(Math.min(startX, e.clientX));
      const b = this.indexAt(Math.max(startX, e.clientX));
      if (a != null && b != null) this.editing.onRange(Math.min(a, b), Math.max(a, b));
    };
    this.bar.addEventListener("pointermove", move);
    this.bar.addEventListener("pointerup", up);
    this.bar.addEventListener("pointercancel", up);
  }

  // setCutting shows cuts on the timeline and lets them be edited, or with
  // null hides them. cuts and candidates are point indices; onAdd(index),
  // onMove(from, to) and onRemove(index) report edits.
  setCutting(cutting) {
    this.cutting = cutting;
    this.root.classList.toggle("cutting", Boolean(cutting));
    if (this.track) this.render();
  }

  cutElements() {
    const { cuts, candidates, onAdd, onMove, onRemove } = this.cutting;
    const elements = [];
    const at = (index) => this.track.time[index];
    // Each segment's time, as a strip in its colour along the bar's foot.
    (this.track.pieces || []).forEach((piece, i) => {
      if (piece.start == null) return;
      const strip = this.block("timeline-piece", piece.start, piece.end, { background: pieceColor(i) });
      strip.title = `Segment ${i + 1}`;
      elements.push(strip);
    });
    for (const index of candidates) {
      if (cuts.includes(index) || at(index) == null) continue;
      const mark = this.mark("timeline-candidate", at(index));
      mark.title = "Proposed cut at a stop — click to cut here";
      mark.addEventListener("click", () => onAdd(index));
      elements.push(mark);
    }
    for (const index of cuts) {
      if (at(index) == null) continue;
      const mark = this.mark("timeline-cut", at(index));
      mark.title = `Cut at ${format.clock(at(index), this.timeZone)} — drag to move, double-click to remove`;
      mark.addEventListener("dblclick", () => onRemove(index));
      mark.addEventListener("pointerdown", (event) => {
        event.preventDefault();
        try {
          mark.setPointerCapture(event.pointerId);
        } catch {
          // Without capture the drag ends when the pointer leaves the handle.
        }
        let target = index;
        const move = (e) => {
          const to = this.indexAt(e.clientX);
          if (to == null) return;
          target = to;
          mark.style.left = this.percent(at(to));
          this.setIndex(to);
          this.onScrub(to);
        };
        const up = () => {
          mark.removeEventListener("pointermove", move);
          mark.removeEventListener("pointerup", up);
          mark.removeEventListener("pointercancel", up);
          if (target !== index) onMove(index, target);
        };
        mark.addEventListener("pointermove", move);
        mark.addEventListener("pointerup", up);
        mark.addEventListener("pointercancel", up);
      });
      elements.push(mark);
    }
    return elements;
  }

  mark(className, time) {
    const element = document.createElement("div");
    element.className = className;
    element.style.left = this.percent(time);
    return element;
  }

  percent(time) {
    const [start, end] = this.range;
    return `${((time - start) / (end - start)) * 100}%`;
  }

  // indexAt is the point recorded nearest the moment at a screen x, or null.
  indexAt(clientX) {
    if (!this.track || this.timed.length < 2) return null;
    const rect = this.bar.getBoundingClientRect();
    const fraction = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
    const [start, end] = this.range;
    return this.nearest(start + fraction * (end - start));
  }

  // scrub finds the point recorded nearest the moment under the pointer.
  scrub(event) {
    if (!this.track || this.timed.length < 2) return;
    if (event.type === "pointermove" && event.buttons === 0 && event.pointerType !== "mouse") return;
    const index = this.indexAt(event.clientX);
    this.setIndex(index);
    this.onScrub(index);
  }

  nearest(millis) {
    const { timed, track } = this;
    let low = 0, high = timed.length - 1;
    while (low < high) {
      const mid = (low + high) >> 1;
      if (track.time[timed[mid]] < millis) low = mid + 1;
      else high = mid;
    }
    if (low > 0 && millis - track.time[timed[low - 1]] < track.time[timed[low]] - millis) low--;
    return timed[low];
  }

  // setIndex moves the cursor to a point chosen elsewhere (map or charts).
  setIndex(index) {
    const time = index == null || !this.track || !this.range ? null : this.track.time[index];
    if (time == null) {
      this.cursor.hidden = true;
      return;
    }
    const [start, end] = this.range;
    this.cursor.hidden = time < start || time > end;
    this.cursor.style.left = `${((time - start) / (end - start)) * 100}%`;
  }
}

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const MIN_DISTANCE = 1; // metres; shared spatial floor with the profile charts
const TICK_STEPS = [
  SECOND, 2 * SECOND, 5 * SECOND, 10 * SECOND, 15 * SECOND, 30 * SECOND,
  MINUTE, 2 * MINUTE, 5 * MINUTE, 10 * MINUTE, 15 * MINUTE, 30 * MINUTE,
  HOUR, 2 * HOUR, 3 * HOUR, 6 * HOUR, 12 * HOUR, DAY, 2 * DAY, 7 * DAY,
];

function stopped(track) {
  return track.stops.reduce((sum, stop) => sum + (stop.departure - stop.arrival), 0);
}
