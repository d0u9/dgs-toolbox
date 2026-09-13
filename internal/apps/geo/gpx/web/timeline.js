// The focused track's timeline: moving and stopped periods against clock time,
// with gaps where nothing was recorded. Scrubbing it inspects the point at that
// moment. The periods come from the server's stops; this only lays them out.

import * as format from "./format.js";
import { stopTitle } from "./stops.js";

export class Timeline {
  // onScrub(index | null) is called while the pointer moves over the timeline;
  // onStop(index) when a stop is clicked; onStopHover(index | null) when the
  // pointer enters or leaves one.
  constructor({ root, bar, labels, onScrub, onStop, onStopHover }) {
    Object.assign(this, { root, bar, labels, onScrub, onStop, onStopHover });
    this.track = null;
    this.timed = []; // indices of timed points, in time order
    this.timeZone = format.browserTimeZone();
    this.cursor = document.createElement("div");
    this.cursor.className = "timeline-cursor";
    this.cursor.hidden = true;

    bar.addEventListener("pointerdown", (event) => {
      if (event.target.closest(".timeline-stop")) return;
      bar.setPointerCapture(event.pointerId);
      this.scrub(event);
    });
    bar.addEventListener("pointermove", (event) => this.scrub(event));
    bar.addEventListener("pointerleave", (event) => {
      if (event.buttons) return; // still dragging: pointer capture keeps the scrub
      this.setIndex(null);
      this.onScrub(null);
    });
  }

  // hiddenRanges are point index ranges [first, last] drawn as hidden.
  show(track, color, hiddenRanges = []) {
    this.track = track;
    this.color = color;
    this.hiddenRanges = hiddenRanges;
    // Points cleaning removed are not scrubbed to.
    this.timed = track.time.map((t, i) => (t == null || track.removed[i] ? -1 : i)).filter((i) => i >= 0)
      .sort((a, b) => track.time[a] - track.time[b]);
    this.render();
  }

  hide() {
    this.track = null;
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
    const start = track.time[this.timed[0]];
    const end = track.time[this.timed[this.timed.length - 1]];
    this.range = [start, end];
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
      block.addEventListener("click", () => this.onStop(i));
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
    bar.replaceChildren(...blocks, this.cursor);

    const startLabel = document.createElement("span");
    startLabel.textContent = format.clock(start, this.timeZone, { seconds: false });
    const legend = document.createElement("span");
    legend.className = "timeline-legend";
    const count = track.stops.length === 1 ? "1 stop" : `${track.stops.length} stops`;
    legend.textContent = `${count} · ${format.duration(stopped(track) / 1000) || "0m"} stopped`;
    const endLabel = document.createElement("span");
    endLabel.textContent = format.clock(end, this.timeZone, { seconds: false });
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

  // scrub finds the point recorded nearest the moment under the pointer.
  scrub(event) {
    if (!this.track || this.timed.length < 2) return;
    if (event.type === "pointermove" && event.buttons === 0 && event.pointerType !== "mouse") return;
    const rect = this.bar.getBoundingClientRect();
    const fraction = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width));
    const [start, end] = this.range;
    const index = this.nearest(start + fraction * (end - start));
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
    this.cursor.hidden = false;
    this.cursor.style.left = `${((time - start) / (end - start)) * 100}%`;
  }
}

function stopped(track) {
  return track.stops.reduce((sum, stop) => sum + (stop.departure - stop.arrival), 0);
}
