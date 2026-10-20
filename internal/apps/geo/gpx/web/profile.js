import { uiPx } from "./density.js";
// The focused track's elevation and speed profiles. Both charts share one
// x axis — distance along the track, or time — so zooming one zooms the
// other, and the inspected point is one index shared with the map. Ranges go
// in and out as the file's point indices at their ends, whichever axis is
// shown: distance stays the same across a break in the recording, so two
// points far apart in place and time can share one.

import * as format from "./format.js";

const SYNC_KEY = "gpx-profile";
// The narrowest stretch a chart zooms to, in the axis's own unit.
const MIN_SPAN = { distance: 0.001, time: 1 }; // one metre in km; one second

export class Profile {
  // onHover(index | null) is called when the pointer inspects a point on a chart.
  // onRange([first, last] | null, { user, centre }) is called when a chart's
  // stretch changes, as the point indices at its ends, null meaning all of
  // it; centre is the point in the middle of the chart. user says the
  // pointer did it — a wheel or a drag — rather than a chart being built,
  // which shows all of the track.
  // onMenu(event, index) opens the menu at the point under a right click;
  // onJump(index) shows a double-clicked point on the map.
  constructor({ root, elevation, speed, name, swatch, stats, readout, onHover, onRange, onMenu, onJump }) {
    this.onRange = onRange;
    this.onMenu = onMenu;
    this.onJump = onJump;
    this.root = root;
    this.containers = { elevation, speed };
    this.labels = { name, swatch, stats, readout };
    this.onHover = onHover;
    this.charts = [];
    this.track = null;
    this.syncingScale = false;
    this.setRangeTo = null; // [min, max] on the x axis, last set from outside
    this.axis = "distance"; // or "time"
    this.indices = []; // the file's point index of each chart point, in chart order
    this.byFile = []; // the same indices in file order
    this.chartAt = new Map(); // a charted file index's chart point
    this.timeZone = format.browserTimeZone();
    const observer = new ResizeObserver(() => this.resize());
    observer.observe(elevation);
    observer.observe(speed);
  }

  // hiddenRanges are point index ranges [first, last] shaded as hidden. part,
  // a track of the file as { name, first, last }, limits the charts to its
  // points; indices in and out stay the file's.
  show(track, color, hiddenRanges = [], part = null) {
    this.destroy();
    this.track = track;
    this.part = part;
    this.offset = part ? part.first : 0;
    const end = part ? part.last + 1 : track.distance.length;
    this.color = color;
    this.hiddenRanges = hiddenRanges;
    this.root.hidden = false;
    this.labels.name.textContent = part?.name || track.name;
    this.labels.swatch.style.background = color;
    this.renderStats();
    this.labels.readout.textContent = "";

    // On the time axis a point without time has no place, so it is left out.
    const time = this.axis === "time";
    this.byFile = [];
    for (let i = this.offset; i < end; i++) if (!time || track.time[i] != null) this.byFile.push(i);
    if (time && this.byFile.length < 2) this.byFile = [];
    // A chart's x values must rise. Distance does in file order; time need
    // not, when a file's tracks were recorded in another order than they are
    // kept, so on the time axis the points are charted in time order.
    this.indices = time ? [...this.byFile].sort((a, b) => track.time[a] - track.time[b]) : this.byFile;
    this.chartAt = new Map(this.indices.map((index, k) => [index, k]));
    const x = this.indices.map((i) => (time ? track.time[i] / 1000 : track.distance[i] / 1000));
    const elevation = this.indices.map((i) => track.elevation[i]);
    const speed = this.indices.map((i) => (track.speed[i] == null ? null : format.kmh(track.speed[i])));
    const noTime = "No time in this file, so no time axis";
    this.addChart(this.containers.elevation, x, elevation, color, true, time ? noTime : "No elevation in this file", (v) => `${Math.round(v)} m`);
    this.addChart(this.containers.speed, x, speed, color, false, time ? noTime : "No time in this file, so no speed", (v) => `${v.toFixed(1)} km/h`);
  }

  // setAxis puts distance or time across the charts, keeping the stretch
  // they are zoomed into.
  setAxis(axis) {
    if (axis === this.axis) return;
    const range = this.shownRange();
    this.axis = axis;
    this.redraw(range);
  }

  // redraw builds the charts again and zooms them back into range.
  redraw(range) {
    if (!this.track) return;
    this.show(this.track, this.color, this.hiddenRanges, this.part);
    if (range) this.setRange(range);
  }

  // shownRange is the stretch the charts are zoomed into, as point indices,
  // or null when they show all of it.
  shownRange() {
    const chart = this.charts[0];
    if (!chart) return null;
    const data = chart.data[0];
    const { min, max } = chart.scales.x;
    if (min == null || max == null || (min <= data[0] && max >= data[data.length - 1])) return null;
    return this.indexRange(chart, min, max);
  }

  // chartIndex is the chart point of the file's point index, or of the
  // charted point nearest it in the file, the later on a tie.
  chartIndex(index) {
    const { byFile } = this;
    let low = 0, high = byFile.length - 1;
    while (low < high) {
      const mid = (low + high) >> 1;
      if (byFile[mid] < index) low = mid + 1;
      else high = mid;
    }
    if (low > 0 && index - byFile[low - 1] < byFile[low] - index) low--;
    return this.chartAt.get(byFile[low]) ?? 0;
  }

  // xRange is the x range the points first to last of the file take on a
  // chart. On the time axis they need not be in time order, so it spans
  // every one of them.
  xRange(chart, [first, last]) {
    if (this.axis !== "time") return [this.xOf(chart, first), this.xOf(chart, last)];
    const xs = this.byFile.filter((i) => i >= first && i <= last).map((i) => this.xOf(chart, i));
    if (!xs.length) return [this.xOf(chart, first), this.xOf(chart, last)].sort((a, b) => a - b);
    return [Math.min(...xs), Math.max(...xs)];
  }

  // indexRange is the stretch a chart's x range [min, max] shows, as the
  // first and last point indices of the file charted in it.
  indexRange(u, min, max) {
    const data = u.data[0];
    let from = 0;
    while (from < data.length - 1 && data[from] < min) from++;
    let to = data.length - 1;
    while (to > from && data[to] > max) to--;
    let first = Infinity, last = -Infinity;
    for (let k = from; k <= to; k++) {
      first = Math.min(first, this.indices[k]);
      last = Math.max(last, this.indices[k]);
    }
    return [first, last];
  }

  // xOf is where a point of the file falls on the x axis.
  xOf(chart, index) {
    return chart.data[0][this.chartIndex(index)];
  }

  setTimeZone(timeZone) {
    this.timeZone = timeZone;
    if (!this.track) return;
    // The time axis is labelled in the zone, so it is drawn again.
    if (this.axis === "time") this.redraw(this.shownRange());
    this.renderStats();
    this.showReadout(this.fileIndex(this.charts[0]?.cursor.idx));
  }

  // fileIndex is the file's point index of a chart's data index.
  fileIndex(index) {
    return index == null || index < 0 ? null : this.indices[index] ?? null;
  }

  renderStats() {
    const { stats } = this.part ? { stats: partStats(this.track, this.part) } : this.track;
    const start = stats.start ? Date.parse(stats.start) : null;
    this.labels.stats.textContent = [
      start != null ? `${format.clock(start, this.timeZone, { seconds: false })} ${format.zoneOffset(start, this.timeZone)}` : "",
      format.distance(stats.distance),
      format.duration(stats.duration),
      stats.ascent ? `↑ ${Math.round(stats.ascent)} m` : "",
      stats.descent ? `↓ ${Math.round(stats.descent)} m` : "",
    ].filter(Boolean).join(" · ");
  }

  hide() {
    this.destroy();
    this.track = null;
    this.root.hidden = true;
  }

  setColor(color) {
    if (this.track) this.show(this.track, color, this.hiddenRanges, this.part);
  }

  // setRange shows the stretch between two of the file's point indices on
  // every chart, or with null all of it, without reporting back through
  // onRange.
  setRange(range) {
    for (const chart of this.charts) {
      const data = chart.data[0];
      const requested = range ? this.xRange(chart, range) : [data[0], data[data.length - 1]];
      const [min, max] = clampRange(requested[0], requested[1], data[0], data[data.length - 1], MIN_SPAN[this.axis]);
      // uPlot commits a scale in a microtask, so its setScale hook runs after
      // this returns. Remember the range set here to recognise that echo;
      // reporting it would snap the timeline to recorded points mid-gesture.
      this.setRangeTo = [min, max];
      chart.setScale("x", { min, max });
    }
  }

  // setIndex moves the chart cursor to a point chosen elsewhere (the map).
  setIndex(index) {
    if (!this.track) return;
    if (this.part && (index < this.part.first || index > this.part.last)) index = null;
    this.fromOutside = true;
    for (const chart of this.charts) {
      if (index == null) {
        chart.setCursor({ left: -10, top: -10 });
      } else {
        const left = chart.valToPos(this.xOf(chart, index), "x");
        chart.setCursor({ left, top: chart.over.clientHeight / 2 });
      }
    }
    this.fromOutside = false;
    this.showReadout(index);
  }

  // formatValue(y) is the text shown beside the cursor point.
  addChart(container, x, y, color, filled, emptyText, formatValue) {
    container.replaceChildren();
    if (!y.some((v) => v != null)) {
      const empty = document.createElement("div");
      empty.className = "empty";
      empty.textContent = emptyText;
      container.append(empty);
      return;
    }
    const type = getComputedStyle(container);
    const axisFont = `${type.getPropertyValue("--font-size-xs").trim() || "11px"} ${type.fontFamily}`;
    const chart = new uPlot({
      width: container.clientWidth,
      height: container.clientHeight,
      legend: { show: false },
      cursor: {
        sync: { key: SYNC_KEY },
        // Holding and dragging pans (see dragPan); the wheel zooms.
        drag: { x: false, y: false, setScale: false },
        // A double-click shows the point on the map instead of zooming out;
        // Fit shows the whole track.
        bind: { dblclick: () => null },
        points: { size: uiPx(7), fill: color, stroke: "#fff", width: uiPx(2) },
      },
      scales: { x: { time: this.axis === "time" } },
      // Times are labelled in the page's time zone, not the browser's.
      tzDate: (seconds) => uPlot.tzDate(new Date(seconds * 1000), this.timeZone),
      axes: [
        this.axis === "time"
          ? { font: axisFont, size: uiPx(30), grid: { stroke: "#eee" } }
          : { font: axisFont, values: (_, ticks) => ticks.map((v) => `${v} km`), size: uiPx(30), grid: { stroke: "#eee" } },
        { font: axisFont, size: uiPx(48), grid: { stroke: "#eee" } },
      ],
      series: [
        {},
        { stroke: color, width: 1.5, fill: filled ? hexAlpha(color, 0.15) : undefined, spanGaps: true, points: { show: false } },
      ],
      hooks: {
        draw: [(u) => this.shadeHidden(u)],
        setCursor: [(u) => {
          placeValue(u, formatValue);
          if (this.fromOutside) return;
          const index = this.fileIndex(u.cursor.idx);
          this.showReadout(index);
          this.onHover(index);
        }],
        setScale: [(u, key) => {
          if (key !== "x" || this.syncingScale) return;
          const user = Boolean(u.byUser);
          const data = u.data[0];
          const [min, max] = clampRange(u.scales.x.min, u.scales.x.max, data[0], data[data.length - 1], MIN_SPAN[this.axis]);
          this.syncingScale = true;
          if (min !== u.scales.x.min || max !== u.scales.x.max) u.setScale("x", { min, max });
          for (const other of this.charts) if (other !== u) other.setScale("x", { min, max });
          this.syncingScale = false;
          const echo = this.setRangeTo;
          if (echo && Math.abs(echo[0] - min) < 1e-6 && Math.abs(echo[1] - max) < 1e-6) return;
          this.setRangeTo = null;
          const whole = min <= data[0] && max >= data[data.length - 1];
          const centre = this.fileIndex(u.valToIdx((min + max) / 2));
          this.onRange(whole ? null : this.indexRange(u, min, max), { user, centre });
        }],
        ready: [(u) => { wheelZoom(u, MIN_SPAN[this.axis]); dragPan(u); }],
      },
    }, [x, y], container);
    // Axes and canvas margins are part of the chart's hover surface too.
    chart.root.addEventListener("pointermove", event => {
      if (event.buttons || chart.over.contains(event.target)) return;
      const box = chart.over.getBoundingClientRect();
      const index = this.fileIndex(chart.posToIdx(Math.max(0, Math.min(box.width, event.clientX - box.left))));
      this.setIndex(index);
      this.onHover(index);
    });
    chart.root.addEventListener("dblclick", event => {
      const box = chart.over.getBoundingClientRect();
      const index = this.fileIndex(chart.posToIdx(Math.max(0, Math.min(box.width, event.clientX - box.left))));
      if (index != null) this.onJump?.(index);
    });
    chart.root.addEventListener("contextmenu", event => {
      event.preventDefault();
      const box = chart.over.getBoundingClientRect();
      const index = this.fileIndex(chart.posToIdx(Math.max(0, Math.min(box.width, event.clientX - box.left))));
      if (index != null) this.onMenu?.(event, index);
    });
    chart.root.addEventListener("mouseleave", () => {
      this.showReadout(null);
      this.onHover(null);
    });
    this.charts.push(chart);
    document.fonts?.ready.then(() => { if (this.charts.includes(chart)) chart.redraw(); });
  }

  // shadeHidden hatches the distance ranges of hidden tracks over a chart.
  shadeHidden(u) {
    const { ctx, bbox } = u;
    ctx.save();
    ctx.beginPath();
    ctx.rect(bbox.left, bbox.top, bbox.width, bbox.height);
    ctx.clip();
    for (const [first, last] of this.hiddenRanges) {
      const ends = [u.valToPos(this.xOf(u, first), "x", true), u.valToPos(this.xOf(u, last), "x", true)];
      const left = Math.min(...ends);
      const right = Math.max(left + 2, ...ends);
      ctx.save();
      ctx.beginPath();
      ctx.rect(left, bbox.top, right - left, bbox.height);
      ctx.clip();
      ctx.fillStyle = "rgba(246, 246, 244, 0.72)";
      ctx.fillRect(left, bbox.top, right - left, bbox.height);
      ctx.strokeStyle = "rgba(140, 140, 135, 0.4)";
      ctx.lineWidth = 1;
      ctx.beginPath();
      for (let x = left - bbox.height; x < right; x += 8) {
        ctx.moveTo(x, bbox.top + bbox.height);
        ctx.lineTo(x + bbox.height, bbox.top);
      }
      ctx.stroke();
      ctx.restore();
    }
    ctx.restore();
  }

  showReadout(index) {
    const track = this.track;
    if (!track || index == null || index < 0) {
      this.labels.readout.textContent = "";
      return;
    }
    const parts = [format.distance(track.distance[index])];
    if (track.elevation[index] != null) parts.push(`${Math.round(track.elevation[index])} m`);
    if (track.speed[index] != null) parts.push(`${format.kmh(track.speed[index]).toFixed(1)} km/h`);
    if (track.time[index] != null) parts.push(format.clock(track.time[index], this.timeZone));
    this.labels.readout.textContent = parts.join(" · ");
  }

  resize() {
    for (const chart of this.charts) {
      const container = chart.root.parentElement;
      if (container.clientWidth && container.clientHeight) {
        chart.setSize({ width: container.clientWidth, height: container.clientHeight });
      }
    }
  }

  destroy() {
    for (const chart of this.charts) chart.destroy();
    this.charts = [];
    for (const container of Object.values(this.containers)) container.replaceChildren();
  }
}

// placeValue labels the cursor point with its value, on the side of the point
// with more room.
function placeValue(u, formatValue) {
  let label = u.over.querySelector(".cursor-value");
  if (!label) {
    label = document.createElement("div");
    label.className = "cursor-value";
    u.over.append(label);
  }
  const index = u.cursor.idx;
  const value = index == null ? null : u.data[1][index];
  const left = u.cursor.left;
  if (value == null || left == null || left < 0) {
    label.hidden = true;
    return;
  }
  const top = u.valToPos(value, "y");
  label.textContent = formatValue(value);
  label.hidden = false;
  const width = u.over.clientWidth;
  const height = u.over.clientHeight;
  label.style.left = left > width / 2 ? "" : `${left + 8}px`;
  label.style.right = left > width / 2 ? `${width - left + 8}px` : "";
  label.style.top = `${Math.min(Math.max(top - 20, 0), height - 18)}px`;
}

// wheelZoom zooms the x axis around the pointer, no narrower than minSpan;
// the setScale hook carries the new range to the other chart.
function wheelZoom(u, minSpan) {
  const [dataMin, dataMax] = [u.data[0][0], u.data[0][u.data[0].length - 1]];
  u.over.addEventListener("wheel", (event) => {
    event.preventDefault();
    const { min, max } = u.scales.x;
    const at = u.posToVal(event.offsetX, "x");
    const factor = event.deltaY < 0 ? 0.8 : 1.25;
    let nextMin = at - (at - min) * factor;
    let nextMax = at + (max - at) * factor;
    [nextMin, nextMax] = clampRange(nextMin, nextMax, dataMin, dataMax, minSpan);
    userScale(u, { min: nextMin, max: nextMax });
  }, { passive: false });
}

// dragPan moves the x axis with the pointer while it is held on a chart, the
// distance or time under it staying under it. A press that moves less than
// PAN_START pixels stays a click.
function dragPan(u) {
  u.over.addEventListener("pointerdown", (event) => {
    if (event.button !== 0) return;
    const startX = event.clientX;
    const { min, max } = u.scales.x;
    const data = u.data[0];
    let dragging = false;
    const move = (e) => {
      // Released outside the chart before the drag began: no pointerup came.
      if (!(e.buttons & 1)) { up(); return; }
      if (!dragging && Math.abs(e.clientX - startX) < PAN_START) return;
      if (!dragging) {
        dragging = true;
        try { u.over.setPointerCapture(event.pointerId); } catch { /* follows while over the chart */ }
      }
      const shift = -((e.clientX - startX) / u.over.clientWidth) * (max - min);
      // The span stays as it is; the ends stop at the data's.
      const from = Math.max(data[0], Math.min(min + shift, data[data.length - 1] - (max - min)));
      userScale(u, { min: from, max: from + (max - min) });
    };
    const up = () => {
      u.over.removeEventListener("pointermove", move);
      u.over.removeEventListener("pointerup", up);
      u.over.removeEventListener("pointercancel", up);
    };
    u.over.addEventListener("pointermove", move);
    u.over.addEventListener("pointerup", up);
    u.over.addEventListener("pointercancel", up);
  });
}

const PAN_START = 3; // pixels

// userScale sets the x scale for the pointer. uPlot reports a scale from a
// microtask; the mark is taken down in one queued after it, so the report
// sees it.
function userScale(u, scale) {
  u.byUser = true;
  u.setScale("x", scale);
  queueMicrotask(() => { u.byUser = false; });
}

function clampRange(min, max, dataMin, dataMax, minSpan) {
  const full = Math.max(minSpan, dataMax - dataMin);
  const upper = Math.max(dataMax, dataMin + minSpan);
  const span = Math.min(full, Math.max(minSpan, max - min));
  const from = Math.max(dataMin, Math.min(min, upper - span));
  return [from, from + span];
}

function hexAlpha(hex, alpha) {
  const n = parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
}

// partStats are the stats shown for one track of a file: its start, distance
// and duration. Climb is the file's measure and is left out.
function partStats(track, { first, last }) {
  const times = track.time.slice(first, last + 1).filter((t) => t != null);
  return {
    start: times.length ? new Date(times[0]).toISOString() : null,
    distance: track.distance[last] - track.distance[first],
    duration: times.length ? (times[times.length - 1] - times[0]) / 1000 : 0,
  };
}
