// The focused track's stops on the map: a numbered marker at each, opening a
// popup with its arrival, departure and duration, and the radius the stop was
// found within, drawn while a stop is pointed at. The server found the stops.

import * as format from "./format.js";

const RANGE = "stop-range";

export class StopMarkers {
  // onSelect(index) is called when a stop's marker is clicked; onRemove(index)
  // when its popup's Remove this stop is.
  constructor(map, { onSelect, onRemove }) {
    this.map = map;
    this.onSelect = onSelect;
    this.onRemove = onRemove;
    this.markers = [];
    this.popup = new maplibregl.Popup({ closeButton: true, closeOnClick: true, offset: 14, maxWidth: "260px" });
    this.track = null;
    this.timeZone = format.browserTimeZone();
    this.onMap = true;
    map.addSource(RANGE, { type: "geojson", data: { type: "FeatureCollection", features: [] } });
    map.addLayer({
      id: RANGE,
      type: "line",
      source: RANGE,
      paint: { "line-color": "#3b3b38", "line-width": 1, "line-opacity": 0.55, "line-dasharray": [3, 2] },
    });
  }

  // show places the markers; a stop inside a hidden range of points has none.
  show(track, color, hiddenRanges = []) {
    this.clear();
    this.track = track;
    this.markers = track.stops.map((stop, i) => {
      const element = document.createElement("button");
      element.className = "stop-marker";
      element.classList.toggle("off-map", !this.onMap);
      element.hidden = hiddenRanges.some(([first, last]) => stop.first <= last && stop.last >= first);
      element.textContent = String(i + 1);
      element.style.borderColor = color;
      element.title = stopTitle(i, stop, this.timeZone);
      element.addEventListener("mouseenter", () => this.highlight(i));
      element.addEventListener("mouseleave", () => this.highlight(null));
      element.addEventListener("click", (event) => {
        event.stopPropagation();
        this.onSelect(i);
      });
      return new maplibregl.Marker({ element }).setLngLat(stop.center).addTo(this.map);
    });
  }

  // setOnMap shows or hides the numbered markers; popups and the range circle,
  // opened from the timeline, still work.
  setOnMap(onMap) {
    this.onMap = onMap;
    for (const marker of this.markers) marker.getElement().classList.toggle("off-map", !onMap);
  }

  setTimeZone(timeZone) {
    this.timeZone = timeZone;
    if (!this.track) return;
    this.markers.forEach((marker, i) => {
      marker.getElement().title = stopTitle(i, this.track.stops[i], timeZone);
    });
    if (this.popup.isOpen() && this.open != null) this.openPopup(this.open);
  }

  // openPopup shows one stop's times and highlights its marker.
  openPopup(index) {
    const stop = this.track?.stops[index];
    if (!stop) return;
    this.open = index;
    this.markers.forEach((marker, i) => marker.getElement().classList.toggle("selected", i === index));
    const body = document.createElement("div");
    body.className = "stop-popup";
    const title = document.createElement("strong");
    title.textContent = `Stop ${index + 1}`;
    body.append(title, ...stopLines(stop, this.timeZone).map((text) => {
      const line = document.createElement("div");
      line.textContent = text;
      return line;
    }));
    const remove = document.createElement("button");
    remove.className = "text-button";
    remove.textContent = "Remove this stop";
    remove.title = "Remove every point of the stop by hand, joining the track across it";
    remove.addEventListener("click", () => {
      this.popup.remove();
      this.onRemove(index);
    });
    body.append(remove);
    this.popup.setLngLat(stop.center).setDOMContent(body).addTo(this.map);
    this.popup.once("close", () => {
      if (this.open !== index) return;
      this.open = null;
      this.markers[index]?.getElement().classList.remove("selected");
    });
  }

  // highlight draws the stop-distance circle around one stop, or none.
  highlight(index) {
    const stop = index == null ? null : this.track?.stops[index];
    const features = stop ? [circle(stop.center, this.track.stopParams.distance)] : [];
    this.map.getSource(RANGE).setData({ type: "FeatureCollection", features });
  }

  clear() {
    this.map.getSource(RANGE).setData({ type: "FeatureCollection", features: [] });
    for (const marker of this.markers) marker.remove();
    this.markers = [];
    this.open = null;
    this.popup.remove();
    this.track = null;
  }
}

// circle is a ring of radius metres around [lon, lat], for drawing only.
function circle([lon, lat], metres, steps = 72) {
  const dLat = (metres / 6371008.8) * (180 / Math.PI);
  const dLon = dLat / Math.cos((lat * Math.PI) / 180);
  const ring = [];
  for (let i = 0; i <= steps; i++) {
    const angle = (i / steps) * 2 * Math.PI;
    ring.push([lon + dLon * Math.cos(angle), lat + dLat * Math.sin(angle)]);
  }
  return { type: "Feature", properties: {}, geometry: { type: "LineString", coordinates: ring } };
}

export function stopLines(stop, timeZone) {
  return [
    `Arrived ${format.clock(stop.arrival, timeZone)}`,
    `Left ${format.clock(stop.departure, timeZone)}`,
    `Stayed ${format.duration(stop.duration) || "under a minute"}`,
  ];
}

export function stopTitle(index, stop, timeZone) {
  return [`Stop ${index + 1}`, ...stopLines(stop, timeZone)].join("\n");
}
