// The focused track's elevation and speed profiles. Both charts share the
// distance axis: zooming one zooms the other, and the inspected point is one
// index shared with the map.

import * as format from "./format.js";

const SYNC_KEY = "gpx-profile";
const MIN_DISTANCE_SPAN = 0.001; // km (one metre)

export class Profile {
  // onHover(index | null) is called when the pointer inspects a point on a chart.
  // onRange([minKm, maxKm] | null) is called when zooming a chart changes the
  // distance shown, null meaning all of it.
  constructor({ root, elevation, speed, name, swatch, stats, readout, onHover, onRange }) {
    this.onRange = onRange;
    this.root = root;
    this.containers = { elevation, speed };
    this.labels = { name, swatch, stats, readout };
    this.onHover = onHover;
    this.charts = [];
    this.track = null;
    this.syncingScale = false;
    this.setRangeTo = null; // [min, max] km last set from outside
    this.timeZone = format.browserTimeZone();
    const observer = new ResizeObserver(() => this.resize());
    observer.observe(elevation);
    observer.observe(speed);
  }

  // hiddenRanges are point index ranges [first, last] shaded as hidden.
  show(track, color, hiddenRanges = []) {
    this.destroy();
    this.track = track;
    this.color = color;
    this.hiddenRanges = hiddenRanges;
    this.root.hidden = false;
    this.labels.name.textContent = track.name;
    this.labels.swatch.style.background = color;
    this.renderStats();
    this.labels.readout.textContent = "";

    const x = track.distance.map((m) => m / 1000);
    const speed = track.speed.map((v) => (v == null ? null : format.kmh(v)));
    this.addChart(this.containers.elevation, x, track.elevation, color, true, "No elevation in this file");
    this.addChart(this.containers.speed, x, speed, color, false, "No time in this file, so no speed");
  }

  setTimeZone(timeZone) {
    this.timeZone = timeZone;
    if (!this.track) return;
    this.renderStats();
    this.showReadout(this.charts[0]?.cursor.idx ?? null);
  }

  renderStats() {
    const { stats } = this.track;
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
    if (this.track) this.show(this.track, color, this.hiddenRanges);
  }

  // setRange shows a distance range in km on every chart, or with null all of
  // it, without reporting back through onRange.
  setRange(range) {
    for (const chart of this.charts) {
      const data = chart.data[0];
      const requested = range || [data[0], data[data.length - 1]];
      const [min, max] = clampDistanceRange(requested[0], requested[1], data[0], data[data.length - 1]);
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
    this.fromOutside = true;
    for (const chart of this.charts) {
      if (index == null) {
        chart.setCursor({ left: -10, top: -10 });
      } else {
        const left = chart.valToPos(this.track.distance[index] / 1000, "x");
        chart.setCursor({ left, top: chart.over.clientHeight / 2 });
      }
    }
    this.fromOutside = false;
    this.showReadout(index);
  }

  addChart(container, x, y, color, filled, emptyText) {
    container.replaceChildren();
    if (!y.some((v) => v != null)) {
      const empty = document.createElement("div");
      empty.className = "empty";
      empty.textContent = emptyText;
      container.append(empty);
      return;
    }
    const chart = new uPlot({
      width: container.clientWidth,
      height: container.clientHeight,
      legend: { show: false },
      cursor: {
        sync: { key: SYNC_KEY },
        drag: { x: true, y: false, setScale: true },
        points: { size: 7, fill: color, stroke: "#fff", width: 2 },
      },
      scales: { x: { time: false } },
      axes: [
        { values: (_, ticks) => ticks.map((v) => `${v} km`), size: 30, grid: { stroke: "#eee" } },
        { size: 48, grid: { stroke: "#eee" } },
      ],
      series: [
        {},
        { stroke: color, width: 1.5, fill: filled ? hexAlpha(color, 0.15) : undefined, spanGaps: true, points: { show: false } },
      ],
      hooks: {
        draw: [(u) => this.shadeHidden(u)],
        setCursor: [(u) => {
          if (this.fromOutside) return;
          const index = u.cursor.idx;
          this.showReadout(index);
          this.onHover(index);
        }],
        setScale: [(u, key) => {
          if (key !== "x" || this.syncingScale) return;
          const data = u.data[0];
          const [min, max] = clampDistanceRange(u.scales.x.min, u.scales.x.max, data[0], data[data.length - 1]);
          this.syncingScale = true;
          if (min !== u.scales.x.min || max !== u.scales.x.max) u.setScale("x", { min, max });
          for (const other of this.charts) if (other !== u) other.setScale("x", { min, max });
          this.syncingScale = false;
          const echo = this.setRangeTo;
          if (echo && Math.abs(echo[0] - min) < 1e-6 && Math.abs(echo[1] - max) < 1e-6) return;
          this.setRangeTo = null;
          const whole = min <= data[0] && max >= data[data.length - 1];
          this.onRange(whole ? null : [min, max]);
        }],
        ready: [(u) => wheelZoom(u)],
      },
    }, [x, y], container);
    chart.over.addEventListener("mouseleave", () => {
      this.showReadout(null);
      this.onHover(null);
    });
    this.charts.push(chart);
  }

  // shadeHidden hatches the distance ranges of hidden tracks over a chart.
  shadeHidden(u) {
    const { ctx, bbox } = u;
    const distance = this.track.distance;
    ctx.save();
    ctx.beginPath();
    ctx.rect(bbox.left, bbox.top, bbox.width, bbox.height);
    ctx.clip();
    for (const [first, last] of this.hiddenRanges) {
      const left = u.valToPos(distance[first] / 1000, "x", true);
      const right = Math.max(left + 2, u.valToPos(distance[last] / 1000, "x", true));
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

// wheelZoom zooms the distance axis around the pointer; the setScale hook
// carries the new range to the other chart.
function wheelZoom(u) {
  const [dataMin, dataMax] = [u.data[0][0], u.data[0][u.data[0].length - 1]];
  u.over.addEventListener("wheel", (event) => {
    event.preventDefault();
    const { min, max } = u.scales.x;
    const at = u.posToVal(event.offsetX, "x");
    const factor = event.deltaY < 0 ? 0.8 : 1.25;
    let nextMin = at - (at - min) * factor;
    let nextMax = at + (max - at) * factor;
    [nextMin, nextMax] = clampDistanceRange(nextMin, nextMax, dataMin, dataMax);
    u.setScale("x", { min: nextMin, max: nextMax });
  }, { passive: false });
}

function clampDistanceRange(min, max, dataMin, dataMax) {
  const full = Math.max(MIN_DISTANCE_SPAN, dataMax - dataMin);
  const upper = Math.max(dataMax, dataMin + MIN_DISTANCE_SPAN);
  const span = Math.min(full, Math.max(MIN_DISTANCE_SPAN, max - min));
  const from = Math.max(dataMin, Math.min(min, upper - span));
  return [from, from + span];
}

function hexAlpha(hex, alpha) {
  const n = parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
}
