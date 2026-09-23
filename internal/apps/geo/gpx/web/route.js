// Planning a route by hand: waypoints clicked on the map, and between each two
// a leg that follows the road — asked of the router — or goes straight. The
// planner keeps the plan in WGS-84 and draws it in the map's system; the panel
// lists the waypoints and legs and saves the plan as a new GPX.

import * as format from "./format.js";
import { fromWGS84, toWGS84 } from "./gcj02.js";

const LEGS = "plan-legs";
const COLOR = "#cc79a7";

// STRAIGHT is the way of a leg that goes in a straight line; the others are
// the routers' ways, as /api/config lists them: { id, label, service }.
const STRAIGHT = { id: "line", label: "Straight", service: "" };

export class RoutePlanner {
  // api routes legs; onChange() is called after the plan changes, to remember
  // it; onSave() saves it; onEdit(path) loads the plan of a file in the
  // workspace; gcj() says whether the map draws in GCJ-02; ways are the
  // routers' ways of travel.
  constructor(map, { root, api, gcj, ways = [], onChange, onSave, onEdit }) {
    Object.assign(this, { map, root, api, gcj, onChange, onSave, onEdit });
    this.ways = [...ways, STRAIGHT];
    this.name = "New route";
    this.way = "foot"; // the way of a new leg
    this.writeRte = false;
    this.waypoints = []; // [lon, lat], WGS-84
    this.legs = []; // { way, route, distance, duration, status: "ok" | "busy" | "error", error }
    this.source = null; // the workspace file the plan was loaded from
    this.focused = null; // { path, name, plan } of the focused file, when it holds a plan
    this.active = false;
    this.markers = [];
    this.handles = [];
    map.addSource(LEGS, { type: "geojson", data: { type: "FeatureCollection", features: [] } });
    map.addLayer({
      id: `${LEGS}-casing`,
      type: "line",
      source: LEGS,
      layout: { "line-join": "round", "line-cap": "round" },
      paint: { "line-color": "#ffffff", "line-width": 7, "line-opacity": 0.9 },
    });
    // Legs along the road are solid; straight ones dashed.
    const paint = {
      "line-color": ["match", ["get", "status"], "error", "#b33c00", COLOR],
      "line-width": 4,
      "line-opacity": ["match", ["get", "status"], "busy", 0.45, 1],
    };
    map.addLayer({ id: LEGS, type: "line", source: LEGS, filter: ["!=", ["get", "way"], "line"], layout: { "line-join": "round", "line-cap": "round" }, paint });
    map.addLayer({ id: `${LEGS}-line`, type: "line", source: LEGS, filter: ["==", ["get", "way"], "line"], layout: { "line-join": "round" }, paint: { ...paint, "line-dasharray": [1.5, 1.2] } });
  }

  // ---- positions ----

  display(point) {
    return this.gcj() ? fromWGS84(point) : point;
  }

  fromDisplay(point) {
    return this.gcj() ? toWGS84(point) : point;
  }

  // ---- editing ----

  setActive(active) {
    this.active = active;
    this.draw();
    if (active) this.render();
    else this.root.replaceChildren();
  }

  // add puts a waypoint at the end, [lon, lat] in WGS-84.
  add(point) {
    this.waypoints.push(point);
    if (this.waypoints.length > 1) {
      this.legs.push(this.newLeg(this.way));
      this.route(this.legs.length - 1);
    }
    this.changed();
  }

  insert(legIndex, point) {
    const way = this.legs[legIndex].way;
    this.waypoints.splice(legIndex + 1, 0, point);
    this.legs.splice(legIndex, 1, this.newLeg(way), this.newLeg(way));
    this.route(legIndex);
    this.route(legIndex + 1);
    this.changed();
  }

  move(index, point) {
    this.waypoints[index] = point;
    if (index > 0) this.route(index - 1);
    if (index < this.legs.length) this.route(index);
    this.changed();
  }

  remove(index) {
    this.waypoints.splice(index, 1);
    if (this.legs.length === 0) {
      // nothing to join
    } else if (index === 0) {
      this.legs.shift();
    } else if (index >= this.legs.length) {
      this.legs.pop();
    } else {
      // The legs either side of it become one, going the way of the first.
      this.legs.splice(index - 1, 2, this.newLeg(this.legs[index - 1].way));
      this.route(index - 1);
    }
    this.changed();
  }

  setWay(legIndex, way) {
    this.legs[legIndex].way = way;
    this.route(legIndex);
    this.changed();
  }

  reverse() {
    this.waypoints.reverse();
    this.legs.reverse();
    for (const leg of this.legs) if (leg.route) leg.route = [...leg.route].reverse();
    this.changed();
  }

  // closeLoop adds the start again as the last waypoint.
  closeLoop() {
    if (this.waypoints.length > 1) this.add([...this.waypoints[0]]);
  }

  clear() {
    this.waypoints = [];
    this.legs = [];
    this.source = null;
    this.changed();
  }

  newLeg(way) {
    return { way, route: null, distance: 0, duration: null, status: "busy", error: null };
  }

  // route finds leg i's way between its waypoints: a straight line at once, or
  // the road from the router. A later request for the same leg wins.
  async route(i) {
    const leg = this.legs[i];
    const from = this.waypoints[i], to = this.waypoints[i + 1];
    const token = (leg.token = (leg.token || 0) + 1);
    leg.route = [from, to];
    leg.distance = metres(from, to);
    leg.duration = null;
    leg.error = null;
    if (leg.way === "line") {
      leg.status = "ok";
      return;
    }
    leg.status = "busy";
    try {
      const answer = await this.api.routeLeg(leg.way, from, to);
      if (leg.token !== token || !this.legs.includes(leg)) return;
      const road = answer.route;
      // The road starts and ends on the road; the leg reaches its waypoints.
      leg.route = [from, ...road, to].filter((p, k, all) => k === 0 || p[0] !== all[k - 1][0] || p[1] !== all[k - 1][1]);
      leg.distance = answer.distance + metres(from, road[0]) + metres(road[road.length - 1], to);
      leg.duration = answer.duration;
      leg.status = "ok";
    } catch (error) {
      if (leg.token !== token || !this.legs.includes(leg)) return;
      leg.status = "error";
      leg.error = error.message;
    }
    this.changed();
  }

  changed() {
    this.draw();
    this.render();
    this.onChange();
  }

  // ---- remembered between visits, and loaded from a saved route ----

  toJSON() {
    return {
      name: this.name,
      way: this.way,
      writeRte: this.writeRte,
      source: this.source,
      waypoints: this.waypoints,
      legs: this.legs.map(({ way, route, distance, duration, status }) => ({ way, route: status === "ok" ? route : null, distance, duration })),
    };
  }

  // load takes a remembered plan, or a saved one ({ waypoints, legs: [{ way, route }] }).
  load(saved) {
    if (!saved || !Array.isArray(saved.waypoints)) return;
    this.name = saved.name || this.name;
    if (this.ways.some((way) => way.id === saved.way)) this.way = saved.way;
    this.writeRte = Boolean(saved.writeRte);
    this.source = saved.source || null;
    this.waypoints = saved.waypoints;
    this.legs = (saved.legs || []).slice(0, Math.max(0, this.waypoints.length - 1)).map((leg) => ({
      way: leg.way,
      route: leg.route,
      distance: leg.distance ?? routeMetres(leg.route || []),
      duration: leg.duration ?? null,
      status: leg.route ? "ok" : "busy",
      error: null,
    }));
    while (this.legs.length < this.waypoints.length - 1) this.legs.push(this.newLeg(this.way));
    this.legs.forEach((leg, i) => { if (!leg.route) this.route(i); });
    this.draw();
    this.render();
  }

  // setFocused offers to edit the plan of the focused file, if it holds one.
  setFocused(path, track) {
    this.focused = track?.plan ? { path, name: track.name, plan: track.plan } : null;
    if (this.active) this.render();
  }

  // ---- map ----

  draw() {
    const { map } = this;
    const features = this.active
      ? this.legs.map((leg, i) => ({
        type: "Feature",
        properties: { way: leg.way, status: leg.status },
        geometry: { type: "LineString", coordinates: (leg.route || [this.waypoints[i], this.waypoints[i + 1]]).map((p) => this.display(p)) },
      }))
      : [];
    map.getSource(LEGS).setData({ type: "FeatureCollection", features });
    for (const layer of [`${LEGS}-casing`, LEGS, `${LEGS}-line`]) map.moveLayer(layer);
    for (const marker of [...this.markers, ...this.handles]) marker.remove();
    this.markers = [];
    this.handles = [];
    if (!this.active) return;

    this.waypoints.forEach((point, i) => {
      const element = document.createElement("div");
      element.className = "plan-marker" + (i === 0 ? " start" : i === this.waypoints.length - 1 ? " end" : "");
      element.textContent = String(i + 1);
      element.title = "Drag to move · double-click to remove";
      element.addEventListener("dblclick", (event) => {
        event.stopPropagation();
        event.preventDefault();
        this.remove(i);
      });
      const marker = new maplibregl.Marker({ element, draggable: true }).setLngLat(this.display(point)).addTo(map);
      marker.on("drag", () => this.preview(i, marker.getLngLat()));
      marker.on("dragend", () => {
        const { lng, lat } = marker.getLngLat();
        this.move(i, this.fromDisplay([lng, lat]));
      });
      this.markers.push(marker);
    });

    // A handle midway along each leg inserts a waypoint when dragged.
    this.legs.forEach((leg, i) => {
      const line = leg.route || [this.waypoints[i], this.waypoints[i + 1]];
      const element = document.createElement("div");
      element.className = "plan-marker handle";
      element.title = "Drag to add a waypoint here";
      const handle = new maplibregl.Marker({ element, draggable: true }).setLngLat(this.display(midway(line))).addTo(map);
      handle.on("dragend", () => {
        const { lng, lat } = handle.getLngLat();
        this.insert(i, this.fromDisplay([lng, lat]));
      });
      this.handles.push(handle);
    });
  }

  // preview draws the legs either side of a waypoint being dragged as straight lines.
  preview(index, lngLat) {
    const moved = [lngLat.lng, lngLat.lat];
    const features = this.legs.map((leg, i) => {
      let coordinates = (leg.route || [this.waypoints[i], this.waypoints[i + 1]]).map((p) => this.display(p));
      let status = leg.status;
      if (i === index - 1) [coordinates, status] = [[this.display(this.waypoints[i]), moved], "busy"];
      if (i === index) [coordinates, status] = [[moved, this.display(this.waypoints[i + 1])], "busy"];
      return { type: "Feature", properties: { way: leg.way, status }, geometry: { type: "LineString", coordinates } };
    });
    this.map.getSource(LEGS).setData({ type: "FeatureCollection", features });
  }

  // bounds is the [[west, south], [east, north]] box of the plan as drawn.
  bounds() {
    const points = this.legs.flatMap((leg) => leg.route || []).concat(this.waypoints).map((p) => this.display(p));
    if (!points.length) return null;
    const lons = points.map((p) => p[0]), lats = points.map((p) => p[1]);
    return [[Math.min(...lons), Math.min(...lats)], [Math.max(...lons), Math.max(...lats)]];
  }

  // ---- panel ----

  get distance() {
    return this.legs.reduce((sum, leg) => sum + (leg.distance || 0), 0);
  }

  // ready says whether the plan can be saved: routed legs, two waypoints or more.
  get ready() {
    return this.waypoints.length > 1 && this.legs.every((leg) => leg.status === "ok");
  }

  render() {
    if (!this.active) return;
    const parts = [];

    const head = document.createElement("div");
    head.className = "clean-head route-head";
    const name = document.createElement("input");
    name.className = "route-name";
    name.value = this.name;
    name.placeholder = "Name the route";
    name.setAttribute("aria-label", "Route name");
    name.addEventListener("change", () => {
      this.name = name.value.trim() || "New route";
      this.onChange();
    });
    head.append(name);
    parts.push(head);

    const summary = document.createElement("div");
    summary.className = "route-summary";
    const total = document.createElement("strong");
    total.textContent = format.distance(this.distance);
    const counts = document.createElement("span");
    counts.className = "muted";
    const n = this.waypoints.length;
    const roads = this.legs.length > 0 && this.legs.every((leg) => leg.duration != null);
    counts.textContent = [
      `${n} waypoint${n === 1 ? "" : "s"}`,
      roads ? `about ${format.duration(this.legs.reduce((sum, leg) => sum + leg.duration, 0))} by the router` : "",
    ].filter(Boolean).join(" · ");
    summary.append(total, counts);
    parts.push(summary);

    const tools = document.createElement("div");
    tools.className = "route-tools";
    const way = document.createElement("label");
    way.className = "muted";
    way.textContent = "New legs ";
    way.append(waySelect(this.ways, this.way, (value) => {
      this.way = value;
      this.onChange();
    }));
    tools.append(
      way,
      textButton("Reverse", n < 2, () => this.reverse()),
      textButton("Close loop", n < 2, () => this.closeLoop()),
      textButton("Clear", n === 0, () => this.clear()),
    );
    parts.push(tools);

    const body = document.createElement("div");
    body.className = "inspector-body";
    if (this.focused && this.focused.path !== this.source) {
      const edit = document.createElement("p");
      edit.className = "route-edit";
      edit.append(`${this.focused.name} was planned here. `);
      edit.append(textButton("Edit its route", false, () => this.onEdit(this.focused)));
      body.append(edit);
    }
    if (n === 0) {
      const hint = document.createElement("p");
      hint.className = "cut-hint";
      hint.textContent = "Click the map to add the first waypoint. Each next click adds a leg to it, along the road or straight. A click near the end of a track in the workspace snaps to it.";
      body.append(hint);
    } else {
      body.append(this.list());
    }
    parts.push(body);

    const foot = document.createElement("div");
    foot.className = "route-foot";
    const rte = document.createElement("label");
    rte.className = "clean-check";
    const check = document.createElement("input");
    check.type = "checkbox";
    check.checked = this.writeRte;
    check.addEventListener("change", () => {
      this.writeRte = check.checked;
      this.onChange();
    });
    rte.append(check, " Also write the waypoints as a <rte>");
    const saveButton = document.createElement("button");
    saveButton.className = "chip active";
    saveButton.textContent = "Save as GPX…";
    saveButton.disabled = !this.ready;
    saveButton.title = this.ready ? "Write the route into a new GPX file" : "Two waypoints or more, every leg routed";
    saveButton.addEventListener("click", () => this.onSave());
    foot.append(rte, saveButton);
    parts.push(foot);
    this.root.replaceChildren(...parts);
  }

  list() {
    const list = document.createElement("ol");
    list.className = "route-list";
    this.waypoints.forEach((point, i) => {
      const row = document.createElement("li");
      row.className = "route-waypoint";
      const number = document.createElement("span");
      number.className = "plan-marker static" + (i === 0 ? " start" : i === this.waypoints.length - 1 ? " end" : "");
      number.textContent = String(i + 1);
      const label = document.createElement("button");
      label.className = "route-label";
      label.title = "Show it on the map";
      label.textContent = i === 0 ? "Start" : i === this.waypoints.length - 1 ? "End" : `Waypoint ${i + 1}`;
      const where = document.createElement("span");
      where.className = "meta";
      where.textContent = `${point[1].toFixed(5)}, ${point[0].toFixed(5)}`;
      label.append(where);
      label.addEventListener("click", () => this.map.easeTo({ center: this.display(point), duration: 400 }));
      const remove = document.createElement("button");
      remove.className = "remove";
      remove.textContent = "×";
      remove.title = "Remove this waypoint";
      remove.addEventListener("click", () => this.remove(i));
      row.append(number, label, remove);
      list.append(row);

      const leg = this.legs[i];
      if (!leg) return;
      const legRow = document.createElement("li");
      legRow.className = "route-leg " + leg.status;
      const select = waySelect(this.ways, leg.way, (value) => this.setWay(i, value));
      const meta = document.createElement("span");
      meta.className = "meta";
      if (leg.status === "busy") meta.textContent = "Routing…";
      else if (leg.status === "error") {
        meta.textContent = leg.error;
        meta.title = leg.error;
      } else meta.textContent = format.distance(leg.distance);
      legRow.append(select, meta);
      if (leg.status === "error") legRow.append(textButton("Retry", false, () => this.route(i).then(() => this.changed())));
      list.append(legRow);
    });
    return list;
  }
}

// waySelect picks a way of travel, grouped by the service that routes it.
export function waySelect(ways, value, onChange) {
  const select = document.createElement("select");
  select.className = "range-join";
  const groups = new Map();
  for (const way of ways) {
    const option = new Option(way.label, way.id);
    if (!way.service) {
      select.append(option);
      continue;
    }
    if (!groups.has(way.service)) {
      const group = document.createElement("optgroup");
      group.label = way.service;
      groups.set(way.service, group);
      select.append(group);
    }
    groups.get(way.service).append(option);
  }
  if (!ways.some((way) => way.id === value)) select.append(new Option(`${value} (not offered)`, value));
  select.value = value;
  select.addEventListener("change", () => onChange(select.value));
  return select;
}

function textButton(text, disabled, onClick) {
  const button = document.createElement("button");
  button.className = "text-button";
  button.textContent = text;
  button.disabled = disabled;
  button.addEventListener("click", onClick);
  return button;
}

// metres between two [lon, lat] positions, along the great circle.
export function metres(a, b) {
  const rad = Math.PI / 180;
  const dLat = (b[1] - a[1]) * rad, dLon = (b[0] - a[0]) * rad;
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(a[1] * rad) * Math.cos(b[1] * rad) * Math.sin(dLon / 2) ** 2;
  return 2 * 6371008.8 * Math.asin(Math.min(1, Math.sqrt(h)));
}

function routeMetres(route) {
  let sum = 0;
  for (let i = 1; i < route.length; i++) sum += metres(route[i - 1], route[i]);
  return sum;
}

// midway is the point halfway along a line, by distance.
function midway(line) {
  const half = routeMetres(line) / 2;
  let walked = 0;
  for (let i = 1; i < line.length; i++) {
    const step = metres(line[i - 1], line[i]);
    if (walked + step >= half && step > 0) {
      const t = (half - walked) / step;
      return [line[i - 1][0] + (line[i][0] - line[i - 1][0]) * t, line[i - 1][1] + (line[i][1] - line[i - 1][1]) * t];
    }
    walked += step;
  }
  return line[0];
}
