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
    this.showStops = true; // off, the bar shows only where there is data and where not
    this.compress = false; // stops and empty time drawn FOLD_PIXELS wide instead of to scale
    this.hideEmpty = false; // empty time drawn with no width
    this.axis = null; // with compress, the piecewise-linear map from time to axis position
    this.axisKey = null;
    this.snap = true; // the cursor sticks to stops, cuts and ends; else follows the pointer
    this.stuck = null; // the point index the cursor is stuck to while snapping
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
    this.menu.className = "menu";
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
      this.stuck = null;
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
    this.stuck = null; // an index into the previous track means nothing here
    this.timed = track.time.map((t, i) => (t == null || track.removed[i] ? -1 : i)).filter((i) => i >= 0)
      .sort((a, b) => track.time[a] - track.time[b]);
    this.full = this.timed.length < 2 ? null : [track.time[this.timed[0]], track.time[this.timed[this.timed.length - 1]]];
    this.minSpan = this.full ? this.computeMinimumSpan() : 1;
    this.axisKey = null;
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
    this.updateAxis();
    this.range = this.clampView(this.view) || this.full;
    const [start, end] = this.range;
    const blocks = [];

    for (const [from, to, width] of this.axis?.folds || []) {
      const fold = this.block("timeline-fold" + (width ? "" : " empty"), from, to);
      fold.title = `${format.clock(from, this.timeZone, { seconds: true })} – ${format.clock(to, this.timeZone, { seconds: true })} · ${format.duration((to - from) / 1000)} ${width ? "compressed" : "empty, hidden"}`;
      blocks.push(fold);
    }
    if (this.showStops) {
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
    } else {
      for (const [from, to] of this.dataSpans()) {
        const block = this.block("timeline-moving", from, to, { background: this.color });
        block.title = `Data ${format.clock(from, this.timeZone, { seconds: true })} – ${format.clock(to, this.timeZone, { seconds: true })}`;
        blocks.push(block);
      }
    }
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
    if (this.showStops) {
      const count = track.stops.length === 1 ? "1 stop" : `${track.stops.length} stops`;
      legend.textContent = `${count} · ${format.duration(stopped(track) / 1000) || "0m"} stopped`;
    } else {
      const spans = this.dataSpans();
      const empty = this.full[1] - this.full[0] - spans.reduce((sum, [from, to]) => sum + (to - from), 0);
      const count = spans.length === 1 ? "1 stretch" : `${spans.length} stretches`;
      legend.textContent = `${count} of data · ${format.duration(empty / 1000) || "0m"} empty`;
    }
    const endLabel = document.createElement("span");
    endLabel.textContent = format.clock(end, this.timeZone, { seconds: zoomed });
    this.labels.replaceChildren(startLabel, legend, endLabel);
  }

  // setCompress draws stops and empty time narrow, so only moving time keeps
  // its scale, or with false draws all time to scale.
  setCompress(compress) {
    this.compress = compress;
    this.stuck = null;
    this.axisKey = null;
    if (this.track) this.render();
  }

  // setHideEmpty draws empty time with no width at all, only a dashed line
  // with the times on either side, or with false as Compress says.
  setHideEmpty(hide) {
    this.hideEmpty = hide;
    this.stuck = null;
    this.axisKey = null;
    if (this.track) this.render();
  }

  // updateAxis builds the map from time to axis position. Compressing, every
  // stop (while stops are shown) and every stretch without data longer than a
  // fold takes FOLD_PIXELS of the whole track's width; hiding empty time, a
  // stretch without data outside any stop takes none. The rest keeps its scale.
  updateAxis() {
    const width = this.bar.clientWidth || 600;
    const key = `${this.compress}|${this.hideEmpty}|${this.showStops}|${width}`;
    if (key === this.axisKey) return;
    this.axisKey = key;
    this.axis = null;
    if (!(this.compress || this.hideEmpty) || !this.full) return;
    const intervals = []; // [from, to, empty]
    const spans = this.dataSpans();
    for (let k = 1; k < spans.length; k++) intervals.push([spans[k - 1][1], spans[k][0], true]);
    // A stop is never hidden as empty time, even when nothing was recorded
    // during it: it keeps its time to scale, or Compress's width.
    if (this.showStops) for (const stop of this.track.stops) intervals.push([stop.arrival, stop.departure, false]);
    intervals.sort((a, b) => a[0] - b[0]);
    const merged = [];
    for (const [from, to, empty] of intervals) {
      const last = merged[merged.length - 1];
      if (last && from <= last[1]) {
        last[1] = Math.max(last[1], to);
        last[2] = last[2] && empty;
      } else if (to > from) merged.push([from, to, empty]);
    }
    const hidden = merged.filter(([, , empty]) => empty && this.hideEmpty);
    let narrow = merged.filter(([, , empty]) => (empty ? this.compress && !this.hideEmpty : this.compress));
    const total = this.full[1] - this.full[0];
    const length = (list) => list.reduce((sum, [from, to]) => sum + (to - from), 0);
    // Narrow folds together take at most half the bar; a fold of size f makes
    // pixels / width = f / (kept + n f).
    let pixels = 0, size = 0;
    const sizeFor = (list) => {
      pixels = Math.max(2, Math.min(FOLD_PIXELS, (width / 2) / Math.max(1, list.length)));
      const kept = total - length(hidden) - length(list);
      return kept > 0 ? (pixels * kept) / (width - pixels * list.length) : total / Math.max(1, list.length);
    };
    if (narrow.length) {
      size = sizeFor(narrow);
      narrow = narrow.filter(([from, to]) => to - from > size);
      if (narrow.length) size = sizeFor(narrow);
    }
    const folds = [...hidden.map(([from, to]) => [from, to, 0]), ...narrow.map(([from, to]) => [from, to, size])]
      .sort((a, b) => a[0] - b[0]);
    if (!folds.length) return;
    const times = [this.full[0]], positions = [0];
    let at = 0, previous = this.full[0];
    for (const [from, to, width] of folds) {
      at += from - previous;
      times.push(from, to);
      positions.push(at, at + width);
      at += width;
      previous = to;
    }
    times.push(this.full[1]);
    positions.push(at + this.full[1] - previous);
    this.axis = { folds, times, positions };
  }

  // toAxis is a time's position along the axis; fromAxis its inverse. Without
  // compression both are the time itself; beyond the track they keep scale.
  toAxis(time) {
    return this.axis ? interpolate(this.axis.times, this.axis.positions, time) : time;
  }

  fromAxis(position) {
    return this.axis ? interpolate(this.axis.positions, this.axis.times, position) : position;
  }

  // fraction is where a time falls across the view, 0 at its start, 1 at its end.
  fraction(time) {
    const from = this.toAxis(this.range[0]), to = this.toAxis(this.range[1]);
    return (this.toAxis(time) - from) / (to - from);
  }

  timeAt(fraction) {
    const from = this.toAxis(this.range[0]), to = this.toAxis(this.range[1]);
    return this.fromAxis(from + fraction * (to - from));
  }

  // axisView turns [from, to] axis positions into a view in time.
  axisView(from, to) {
    return [this.fromAxis(from), this.fromAxis(to)];
  }

  // setShowStops shows stops and moving periods, or with false only the
  // stretches with data and the empty time between them.
  setShowStops(show) {
    this.showStops = show;
    this.stuck = null;
    if (!show) this.onStopHover(null);
    if (this.track) this.render();
  }

  // dataSpans are [from, to] times where points were recorded: a new stretch
  // starts wherever a segment changes or no point came for DATA_GAP, or ten
  // times the usual interval if longer.
  dataSpans() {
    const { track, timed } = this;
    const intervals = [];
    for (let k = 1; k < timed.length; k++) intervals.push(track.time[timed[k]] - track.time[timed[k - 1]]);
    const usual = intervals.length ? [...intervals].sort((a, b) => a - b)[intervals.length >> 1] : 0;
    const gap = Math.max(DATA_GAP, usual * 10);
    const spans = [];
    let from = track.time[timed[0]];
    for (let k = 1; k < timed.length; k++) {
      const a = timed[k - 1], b = timed[k];
      if (intervals[k - 1] > gap || track.segments[a] !== track.segments[b]) {
        spans.push([from, track.time[a]]);
        from = track.time[b];
      }
    }
    spans.push([from, track.time[timed[timed.length - 1]]]);
    return spans;
  }

  block(className, from, to, style = {}) {
    const element = document.createElement("div");
    element.className = className;
    const left = this.fraction(from);
    element.style.left = `${left * 100}%`;
    element.style.width = `${(this.fraction(to) - left) * 100}%`;
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
    // Compressed, the view keeps its width along the axis rather than in time.
    const fullStart = this.toAxis(this.full[0]), fullEnd = this.toAxis(this.full[1]);
    const start = this.toAxis(view[0]), end = this.toAxis(view[1]);
    const span = Math.min(fullEnd - fullStart, Math.max(this.minSpan, end - start));
    const from = Math.max(fullStart, Math.min(start, fullEnd - span));
    const to = from + span;
    if (to >= fullEnd && from <= fullStart) return null;
    return this.axisView(from, to);
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
      const from = this.toAxis(this.range[0]), to = this.toAxis(this.range[1]);
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
        this.changeView(this.axisView(from + shift, from + shift + lockedSpan));
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
        this.wheelAnchor = { clientX, fraction, time: this.fromAxis(from + fraction * span) };
      }
      const anchor = { ...this.wheelAnchor, time: this.toAxis(this.wheelAnchor.time) };
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
      this.changeView(this.axisView(
        anchor.time - anchor.fraction * nextSpan,
        anchor.time + (1 - anchor.fraction) * nextSpan,
      ));
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
      const startRange = this.range.map((t) => this.toAxis(t));
      try {
        element.setPointerCapture(event.pointerId);
      } catch {
        // The pan still follows the pointer while it stays over the element.
      }
      const move = (e) => {
        const shift = (e.clientX - startX) * perPixel();
        this.changeView(this.axisView(startRange[0] + shift, startRange[1] + shift));
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
      drag(bar, () => -(this.toAxis(this.range[1]) - this.toAxis(this.range[0])) / bar.getBoundingClientRect().width)(event);
    });
    // Dragging the overview's window moves it across the whole track.
    this.window.addEventListener("pointerdown", (event) => {
      if (!this.full) return;
      drag(this.window, () => (this.toAxis(this.full[1]) - this.toAxis(this.full[0])) / overview.getBoundingClientRect().width)(event);
    });
    // Clicking the overview outside the window centres the view there.
    overview.addEventListener("pointerdown", (event) => {
      if (event.target !== overview || !this.full) return;
      const rect = overview.getBoundingClientRect();
      const fullStart = this.toAxis(this.full[0]), fullEnd = this.toAxis(this.full[1]);
      const at = fullStart + ((event.clientX - rect.left) / rect.width) * (fullEnd - fullStart);
      const half = (this.toAxis(this.range[1]) - this.toAxis(this.range[0])) / 2;
      this.changeView(this.axisView(at - half, at + half));
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
    const fullStart = this.toAxis(this.full[0]), fullEnd = this.toAxis(this.full[1]);
    const span = fullEnd - fullStart;
    const from = this.toAxis(this.range[0]), to = this.toAxis(this.range[1]);
    this.window.style.left = `${((from - fullStart) / span) * 100}%`;
    this.window.style.width = `${Math.max(0.5, ((to - from) / span) * 100)}%`;
  }

  // renderTicks marks round times in the page's time zone across the view.
  // Compressed, each fold is labelled with the times at its two ends first,
  // round times fill the stretches kept to scale, and a label that would
  // overlap one already placed is left out.
  renderTicks() {
    const [from, to] = this.range;
    const width = this.bar.clientWidth || 600;
    const along = this.toAxis(to) - this.toAxis(from);
    const wanted = along / Math.max(2, width / 90); // about one tick per 90 px
    const step = TICK_STEPS.find((s) => s >= wanted) || TICK_STEPS[TICK_STEPS.length - 1];
    const withSeconds = step < MINUTE;
    const placed = []; // [left, right] pixels taken by labels
    const labels = [];
    const fits = (left, right) => left >= 0 && right <= width && placed.every(([a, b]) => right + 6 <= a || left >= b + 6);
    // align: 0.5 centres the label on its time; 1 ends it there; 0 starts it there.
    const add = (t, text, align) => {
      const x = this.fraction(t) * width;
      const size = text.length * 7;
      const left = x - size * align;
      if (!fits(left, left + size)) return;
      placed.push([left, left + size]);
      const tick = document.createElement("span");
      tick.className = "timeline-tick";
      tick.style.left = this.percent(t);
      tick.style.transform = `translateX(${-align * 100}%)`;
      tick.textContent = text;
      labels.push(tick);
    };
    const short = (t, seconds) => format.clock(t, this.timeZone, { seconds }).slice(11);
    const folds = (this.axis?.folds || []).filter(([a, b]) => b > from && a < to);
    for (const [a, b] of folds) {
      if (a > from) add(a, short(a, withSeconds), 1);
      if (b < to) add(b, short(b, withSeconds), 0);
    }
    // Round in local time: shift by the zone's offset, round, shift back.
    // Ticks too near an end would be cut off; the end labels above say those times.
    const margin = (to - from) * 0.03;
    const stretches = [];
    let at = from;
    for (const [a, b] of folds) {
      if (a > at) stretches.push([at, a]);
      at = Math.max(at, b);
    }
    if (at < to) stretches.push([at, to]);
    for (const [a, b] of stretches) {
      const offset = format.offsetMillis(a, this.timeZone);
      const first = Math.ceil((a + offset) / step) * step - offset;
      for (let t = first, n = 0; t <= b && n < 60; t += step, n++) {
        if (!this.axis && (t < from + margin || t > to - margin)) continue;
        const clock = format.clock(t, this.timeZone, { seconds: withSeconds });
        const time = clock.slice(11);
        // A day's first tick, or every tick a day or more apart, shows the date.
        add(t, step >= DAY || time.startsWith("00:00") ? clock.slice(5, 10) : time, 0.5);
      }
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
    return `${this.fraction(time) * 100}%`;
  }

  // setSnap turns the cursor's stickiness on or off.
  setSnap(snap) {
    this.snap = snap;
    this.stuck = null;
  }

  // indexAt is the point recorded nearest the moment at a screen x, or null.
  // Snapping, a stop's arrival or departure, a cut, a hidden stretch's edge or
  // the track's end within SNAP_GRAB pixels wins, and holds until the pointer
  // is SNAP_RELEASE pixels away.
  indexAt(clientX) {
    if (!this.track || this.timed.length < 2) return null;
    const rect = this.bar.getBoundingClientRect();
    const x = Math.max(rect.left, Math.min(rect.right, clientX));
    const toX = (index) => rect.left + this.fraction(this.track.time[index]) * rect.width;
    const index = this.nearest(this.timeAt((x - rect.left) / rect.width));
    if (!this.snap) return index;
    if (this.stuck != null && Math.abs(toX(this.stuck) - clientX) <= SNAP_RELEASE) return this.stuck;
    this.stuck = null;
    let best = null, bestDistance = SNAP_GRAB;
    for (const target of this.snapTargets()) {
      const distance = Math.abs(toX(target) - clientX);
      if (distance <= bestDistance) [best, bestDistance] = [target, distance];
    }
    if (best == null) return index;
    this.stuck = best;
    return best;
  }

  // snapTargets are the point indices the cursor sticks to.
  snapTargets() {
    const { track, timed } = this;
    const targets = [timed[0], timed[timed.length - 1]];
    if (this.showStops) for (const stop of track.stops) targets.push(this.nearest(stop.arrival), this.nearest(stop.departure));
    else for (const [from, to] of this.dataSpans()) targets.push(this.nearest(from), this.nearest(to));
    for (const [first, last] of this.hiddenRanges || []) targets.push(first, last);
    if (this.cutting) targets.push(...this.cutting.cuts, ...this.cutting.candidates);
    return targets.filter((i) => i != null && track.time[i] != null && !track.removed[i]);
  }

  // scrub finds the point recorded nearest the moment under the pointer.
  scrub(event) {
    if (!this.track || this.timed.length < 2) return;
    if (event.type === "pointermove" && event.buttons === 0 && event.pointerType !== "mouse") return;
    const index = this.indexAt(event.clientX);
    this.onScrub(index);
    this.setIndex(index);
    if (this.snap || index == null) return;
    // Not snapping, the cursor stays under the pointer between points.
    const rect = this.bar.getBoundingClientRect();
    const fraction = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width));
    this.cursor.hidden = false;
    this.cursor.style.left = `${fraction * 100}%`;
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
    this.cursor.style.left = this.percent(time);
  }
}

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const DATA_GAP = 60 * 1000; // milliseconds without a point that count as empty time
const FOLD_PIXELS = 24; // width of a compressed stop or empty stretch across the whole track
const SNAP_GRAB = 10; // pixels within which the snapping cursor sticks to a target
const SNAP_RELEASE = 18; // pixels the pointer must move away to free it
const MIN_DISTANCE = 1; // metres; shared spatial floor with the profile charts
const TICK_STEPS = [
  SECOND, 2 * SECOND, 5 * SECOND, 10 * SECOND, 15 * SECOND, 30 * SECOND,
  MINUTE, 2 * MINUTE, 5 * MINUTE, 10 * MINUTE, 15 * MINUTE, 30 * MINUTE,
  HOUR, 2 * HOUR, 3 * HOUR, 6 * HOUR, 12 * HOUR, DAY, 2 * DAY, 7 * DAY,
];

function stopped(track) {
  return track.stops.reduce((sum, stop) => sum + (stop.departure - stop.arrival), 0);
}

// interpolate maps x through the piecewise-linear function with knots xs → ys,
// both ascending; outside the knots it continues at a slope of one.
function interpolate(xs, ys, x) {
  const last = xs.length - 1;
  if (x <= xs[0]) return ys[0] + (x - xs[0]);
  if (x >= xs[last]) return ys[last] + (x - xs[last]);
  let low = 0, high = last;
  while (high - low > 1) {
    const mid = (low + high) >> 1;
    if (xs[mid] <= x) low = mid;
    else high = mid;
  }
  const span = xs[high] - xs[low];
  return span > 0 ? ys[low] + ((x - xs[low]) / span) * (ys[high] - ys[low]) : ys[low];
}
