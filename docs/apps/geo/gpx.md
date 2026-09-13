# GPX

## Scope

This document owns decisions specific to `dgs geo gpx`. Read
[`../../tui.md`](../../tui.md) first for shared shell behavior.

GPX is the first command of the Geo app. Nothing here is a requirement for other
apps or commands.

## Two surfaces

Much map work — panning, zooming, drawing on a track — cannot be done in a
terminal. GPX therefore has two surfaces:

- **The TUI**, opened by `dgs geo gpx`. It is the entry point and owns the
  process.
- **A local web page**, served by the same process. Map interaction lives here.

The web server starts when the command opens and stops when `dgs` exits. It
starts once per process: leaving GPX and opening it again reuses the running
server rather than starting a second one.

The page, its assets and its libraries (MapLibre GL for the map, uPlot for the
charts) are embedded in the binary: the page cannot drift from the build that
serves it, and it loads without a CDN. Only map tiles come from the network.

The server computes every track value — distance, speed, statistics — with the
shared packages under `internal/geo`, and the page draws what it receives. The
server exposes the API under `/api/`; everything else is the page.

| Path | Purpose |
| --- | --- |
| `/` | The web page. |
| `/api/health` | Answers `{"ok":true}`. |
| `/api/config` | The base maps — built-in first, then configured — each with its coordinate system, and the folder to open at. |
| `/api/dir?path=` | The folders and `.gpx` files of a folder. Hidden entries are left out. |
| `/api/track?path=` | One GPX file as parallel per-point arrays: position, segment, distance, elevation, speed, time; plus its statistics, stops and parts — each track (as a range of those points), route and waypoint — and its cleaning: the sidecar's settings, what each point was removed by, the recorded positions when cleaning moved any, and counts. `stopDistance` (metres) and `stopDuration` (seconds) override the stop thresholds. `coordinates=gcj02` returns positions, stop centres and bounds converted for GCJ-02 maps. |
| `PUT /api/clean` | Writes a track's cleaning settings, and the points removed by hand, to its sidecar. Settings left out keep their defaults; a cleaning that does nothing removes the sidecar. |
| `POST /api/focus` | The page reports its focused track and stop thresholds, so the TUI can summarise it. An empty path clears it. |
| `POST /api/reveal` | Shows a file or folder in this machine's file manager. Refused unless the request comes from this machine. `/api/config` says whether the page may offer it. |

## The TUI

The TUI shows the page's URL and the folder the browser opens at, or why the
server failed to start. Below them it summarises the track focused on the page
— its name, duration, distance and stop count — updating as the focus or the
stop thresholds change. Several browsers share one summary: the last to report
wins.

| Key | Action |
| --- | --- |
| `o` | Open the page in the default browser. |
| `esc` | Back to the command picker. |
| `q` | Quit. |

## The GPX browser

The page's first function: browse a folder of GPX files and look at them on a
map. It is the interface later milestones build on.

- **Layout.** A top bar with the time zone, base map and GCJ-02 selectors; a sidebar
  with the folder tree and the workspace; the map; and, under the map, the
  focused track's profile.
- **Collapsible panels.** Folders and Workspace each fold to their header by
  clicking it. With one folded, the other takes the whole sidebar; the divider
  between them returns when both are open, at its remembered position.
- **Resizable.** Every boundary between panes is a drag handle: sidebar width,
  folder tree against workspace, map against profile, and elevation chart
  against speed chart. Sizes are remembered per browser.
- **Folder tree.** The sidebar shows a tree rooted at `geo.gpx.root` (or
  `--dir`), so GPX files in sibling folders can be shown together. Clicking a
  folder expands or collapses it; a folder's contents load when it is first
  expanded. `↑` makes the parent the root, keeping what is expanded; a
  folder's `⤓` (on hover) makes it the root, so a deep folder is not indented
  far; `⌂` returns to the configured root; `↻` reloads. A root the browser
  remembers is used only while `geo.gpx.root` is unchanged: after the
  configuration changes, the page opens at the new root. The server reads the
  configuration when it starts, so a change takes effect after restarting
  `dgs`. Clicking a file adds it to the workspace, shown and focused;
  clicking a file already there shows and focuses it. A file in the workspace
  carries its track colour: a filled dot when shown, a ring when hidden.
- **Workspace.** The tracks picked from the tree, kept together whatever
  folder they came from — the set later milestones edit and search. Each row
  shows or hides its track on the map (◉ / ○) without leaving the workspace,
  and `×` removes it. A filter box narrows the list by words in the name or
  path; the show-all and hide-all buttons act on the tracks it lists. The
  header counts shown against total.
- **Show in file manager.** Folder, file and track rows have a button, on
  hover, that reveals the item in Finder or Explorer. It appears only when the
  page is opened on the machine running `dgs`: from another machine it would
  open windows on a screen nobody there can see.
- **Names.** A track is named by its file name. The name inside a GPX file is
  often only a recorder's timestamp; the file name is the one the reader chose.
  File names run long, so the sidebar reads a size smaller, a name wraps to two
  lines (breaking anywhere, since names mix words, dates and CJK) before it is
  cut, details sit on a line of their own, and row buttons appear over the
  row's end on hover rather than reserving width. The full path is the row's
  tooltip.
- **What a file holds.** A GPX file may hold several tracks, planned routes
  and waypoints. A workspace row that holds more than one track, or any route
  or waypoint, has a disclosure (▶, at the row's end so rows start at the left
  edge) that lists them beneath it: tracks with
  their distance and segment count, routes (drawn dashed) with their length,
  waypoints (drawn as dots) with their description and elevation. Each has its
  own show/hide, and a bar above the list shows or hides them all at once, or
  all of one kind — every track, route or waypoint; clicking one frames it on the map, and clicking a waypoint, in
  the list or on the map, opens its name, description, elevation and time.
  Which parts are hidden and which rows are expanded are remembered. Profile,
  stops and timeline still cover all the file's tracks together, but hatch the
  ranges of hidden tracks on both charts and the timeline, and a stop inside a
  hidden track has no marker on the map.
- **Several tracks at once.** Every shown workspace track is drawn in its own colour,
  picked from a palette and changeable from its row. A track's recorded
  segments are drawn apart, so a gap in the recording is not bridged by a
  straight line.
- **One focus.** Exactly one shown track is focused — the one later milestones
  edit. It is drawn above the others, wider and with a casing; the others are
  thinner and translucent. Adding a track focuses it; clicking a row, or
  another track's line on the map, moves the focus. A hidden track cannot be
  focused: hiding the focused track passes the focus to the first shown one,
  and clicking a hidden row shows it. Focusing a track fits the
  map to it; *Fit all* fits every shown track. Fitting frames the bulk of a
  track's points (`track.CoreBounds`), so a few stray fixes — a stale first fix
  a thousand kilometres away — do not zoom the map out to a corner.
- **Base maps.** OpenStreetMap, Gaode (街道), Gaode Satellite (imagery with
  Gaode's road and place names above it) and Esri World Imagery are built in
  and need no key, followed by the maps in `geo.gpx.tiles`. Gaode's tile
  addresses are undocumented and could change; the rest are public services.
- **GCJ-02.** Gaode draws in GCJ-02, so a WGS-84 track sits several hundred
  metres off it. The selector's *Auto* converts what is drawn — tracks, stops,
  the inspected point and the stop circle — whenever the base map is a GCJ-02
  one, and says whether it is on (and when no workspace track is in China,
  where it would change nothing). *On* and *Off* override it, for a
  configured map marked wrongly or to compare. The server converts
  (`gcj02.FromWGS84`); distances, speeds and stops are measured in WGS-84
  whatever is drawn. The choice is remembered.
- **Time zone.** GPX times are UTC. Every time on the page — the start in the
  profile header, the inspected point — is shown in the time zone chosen in the
  top bar, with its UTC offset. It defaults to the browser's zone and is
  remembered.
- **Stops.** The focused track's stops are drawn as numbered markers.
  Clicking one opens its arrival, departure and duration. Pointing at a stop,
  on the map or the timeline, draws a thin dashed circle of radius D around
  it, so the threshold can be judged against the place. *On map*, beside the
  thresholds, hides the numbered markers from the map while the timeline keeps
  its stop blocks for jumping to them; it is remembered. The thresholds — at
  least a time T within a distance D — are set beside the timeline and apply
  to every shown track.
- **Timeline.** Above the charts, the focused track's clock time from first to
  last point: moving periods in the track colour, stops as numbered dark
  blocks, and hatched gaps where nothing was recorded. Moving the pointer along
  it, or dragging, marks the point recorded nearest that moment on the map and
  the charts; pointing at the map or a chart marks the timeline. Clicking a
  stop opens it on the map. A file without time says so in its place.
- **Profile.** The focused track has an elevation chart and a speed chart
  (km/h) against distance, with its distance, duration, ascent and descent. A
  file without elevation or without time says so in place of that chart.
- **Charts on demand.** Toggles in the profile header show or hide each chart
  on its own. With both hidden the profile shrinks to its header and the
  timeline, and the map takes the space. With both shown they sit one above
  the other or side by side, splitting the space evenly until the divider is
  dragged; each arrangement remembers its split as a proportion, so a taller or
  wider profile keeps both charts readable.
- **Inspecting a point.** Moving the pointer along the focused track on the map
  marks the nearest point on the map and draws a marker line at the same place
  on both charts; moving along a chart marks the point on the map. A readout
  shows distance, elevation, speed and local time of that point.
- **Zoom.** Dragging across a chart or scrolling on it zooms the distance axis;
  double-click resets. Both charts always show the same range.
- **Remembered.** The browser remembers, per browser, the folder, the
  workspace with each track's colour and visibility, the focus, the time zone, the base map, the root,
  the expanded folders, which sidebar panels are folded, the stop thresholds, and which charts are shown and
  how, and restores them on reload.

## Cleaning

The page's *Clean* chip, in the profile header, opens a panel over the map for
the focused track. Nothing is cleaned until a filter is switched on. Settings
are saved at once to the track's sidecar (see *The sidecar* below); the source
GPX is never written. Filters run in this order, each seeing only what the
ones before kept.

Cleaning applies to the whole file: every `<trk>` of it, joined in file order,
under one set of settings in one sidecar. Routes and waypoints are not cleaned.
Spikes, drift and smoothing stay within a recorded segment; a stop may span
two. Hiding a track in the file's list hides it from the map and the lasso,
not from the filters.

| Filter | Removes or moves | Settings (defaults) |
| --- | --- | --- |
| By hand | Points removed with the lasso. They go first, so they cannot sway a filter. | — |
| Sudden jumps (`spikes`) | A point reached and left faster than the max speed; a point the speed rose into and fell out of faster than the max acceleration; a point the path turns back at by the turn angle, standing at least the min jump from both neighbours and further from each than they are apart; a run of up to 5 points entered and left too fast while passing straight by is not. | max speed 360 km/h, max acceleration 15 m/s², turn back 160°, min jump 30 m |
| Speed drift (`hampel`) | A point whose speed from the previous point is above the median of its neighbours' by the threshold × scaled MAD (never less than the min spread). Only too fast counts. Each pass removes the worst in its neighbourhood and measures again, so the point after a false fix is not removed with it. Optionally stricter, down to a quarter, for fixes with HDOP above 2 or fewer than 6 satellites. | 7 neighbours each side, threshold 3, min spread 3.6 km/h, stricter for poor fixes |
| Position jitter (`kalman`) | Moves points: a constant-velocity Kalman filter and RTS smoother over east and north, a fix counting as fix error × HDOP metres uncertain. Smoothing restarts at a segment or after a pause. | fix error 5 m, acceleration 1 m/s², restart after 60 s |
| Scribbles in stops | Inside each stop found on what is left, keeps the points where it was entered and left and the one recorded midway, moved to the stop's centre, and removes the rest. | within 50 m, at least 5 min |

A max speed of 360 km/h removes a flight; raise it for tracks recorded in the
air.

Distances, speeds, statistics, stops, the profile and the TUI summary are all
measured on what cleaning kept, at its moved positions. The track's line skips
removed points. *Compare with the recording*, on by default and remembered,
draws the recording under it as a thin dashed line with removed points as dots
coloured by what removed them: spike red, drift orange, stop grey, by hand
purple.

**Lasso.** With *Lasso* on, dragging on the map draws a shape; the focused
track's kept points inside it are removed by hand. Holding Alt (⌥) while
drawing restores points removed by hand instead. Map panning is off while the
lasso is on; Esc or the button turns it off. *Restore all removed by hand*
clears the list. Points in hidden tracks are left alone.

### The sidecar

Beside `walk.gpx` the page writes `walk.gpx.dgs.json`:

```json
{
  "version": 1,
  "clean": {
    "spikes": { "enabled": true, "maxSpeed": 100, "maxAcceleration": 15, "turnAngle": 160, "minJump": 30 },
    "drift": { "enabled": false, "halfWindow": 7, "threshold": 3, "minDeviation": 1, "quality": true },
    "smooth": { "enabled": false, "positionSigma": 5, "acceleration": 1, "maxGap": 60 },
    "stops": { "enabled": false, "distance": 50, "duration": 300 },
    "removed": [118, 119, 120]
  }
}
```

Units are the SI ones the filters use: metres, seconds, m/s, m/s². `removed`
holds point indices in file order, counting every track and segment. Settings
missing from a sidecar keep their defaults, so an older one still loads; one
that cannot be read is reported in the panel and the track is shown uncleaned.
A sidecar from a newer version is not read. It is replaced in one step, never
left half written. Editing the source GPX afterwards can shift the indices of
points removed by hand.

Speed at a point is the distance travelled over the 30 seconds centred on it,
divided by the time taken (`track.Speeds`), so GPS jitter between neighbouring
points does not read as changes of pace. Ascent and descent ignore changes
under 3 m.

Stops are found by stay-point detection (`stops.Detect`): from an anchor point,
the points that follow within D of it form a stay, and a stay lasting at least
T is a stop; neighbouring stops whose centres are closer than D are merged.
Defaults are D = 50 m, wide enough for GPS wander indoors, and T = 5 min, long
enough that traffic lights are not stops. Points without time are skipped. A
stop may span a pause between recorded segments: a recorder switched off at a
hotel and on again at its door stayed there.

| Key | Action |
| --- | --- |
| `o` | Open the page in the default browser. |
| `esc` | Back to the command picker. |
| `q` | Quit. |

## Where the server listens

By default the server listens on `127.0.0.1:8765`, reachable only from this
machine. The address is set by `geo.gpx.host` and `geo.gpx.port` in the
configuration (see [`configuration/geo.md`](../../configuration/geo.md)) and
overridden for one run by flags:

```text
dgs geo gpx --host 0.0.0.0 --port 9000
```

- `--host` must be an IP address (or `localhost`). `0.0.0.0` listens on every
  interface so other machines can open the page.
- `--port` must be between 1 and 65535.
- An invalid value is refused before the TUI opens.

When the server listens on every interface, the TUI shows the URL at this
machine's LAN address, since that is the one another machine can use.

If the address is taken, the TUI shows the error. The server does not fall back
to another port: a URL that changes by itself is one another machine cannot
find.

The server has no authentication. Listening on `0.0.0.0` exposes the page to
everyone on the network — including folder names and GPX files anywhere the
account running `dgs` can read.

## Roadmap

The confirmed direction for GPX. Each milestone ships on its own; later ones
build on the data model of earlier ones.

### Principles

- **Sources are read-only.** Cleaning, cutting and gap filling are recorded in a
  sidecar file beside the source GPX and applied on top of the source, so every change can be undone
  or re-run with other parameters. Every point knows which rule removed it and
  which file it came from.
- **The map is on the web page; the TUI is the entry point.** The TUI chooses
  files, shows summaries, starts the server and exports. Anything that needs a
  map or a timeline is on the page.
- **Only GPX files** are read, for now: tracks, routes and waypoints.
- **Export** writes one GPX file holding one `<trk>` per segment, into a folder
  the user chooses.
- **The sidecar** is a small JSON file stored next to the source GPX. It holds
  no track data, only what was done: cleaning parameters, points removed by
  hand, cuts, segment names and gap fills. Opening a GPX that has one resumes
  the work.

### Map and coordinates

- OpenStreetMap, Gaode, Gaode Satellite and Esri World Imagery are built in.
  Further tile sources are listed in `geo.gpx.tiles`, each with a name, a URL
  template and its coordinate system; the page offers them after the built-in
  ones.
- Data is always WGS-84, as GPX is. The **GCJ-02 selector** (done, see above) converts what is drawn —
  track and stops — for base maps that use China's GCJ-02 system. It changes
  display only: cleaning, segmentation and export stay WGS-84. The conversion
  is the widely published reverse-engineered one (accurate to about 1–2 m);
  points outside China are not offset.

### M1 — Open a GPX and show where you stopped

Done: see *The GPX browser* above.

- Parse tracks with time, position, elevation and, when present, HDOP and
  satellite count.
- Detect stops with **stay-point detection** (Li et al., 2008): points that stay
  within a distance D for at least a time T form a stop; neighbouring stops
  closer than D are merged.
- The page draws the track and numbered stops, each with arrival, departure and
  duration, above a timeline of moving and stopped periods. Scrubbing the
  timeline moves a marker on the map.
- The TUI shows duration, distance and stop count.

### M2 — Clean

Done: see *Cleaning* above.

Filters run in order. Each can be toggled and tuned, and the page compares
before and after.

| Problem | Method |
| --- | --- |
| Sudden jumps (A→B→A spikes) | Physically impossible speed or acceleration between neighbours; a near-180° turn to a distant point and back. |
| Speed drift | A **Hampel filter** (rolling median and MAD) on speed, weighting points by HDOP and satellite count when present. |
| Position drift and jitter | A constant-velocity **Kalman filter with RTS smoothing**. It moves points rather than removing them. |
| Scribbles inside buildings | The stops from M1: points inside a stop collapse to its centre, keeping the entry and exit points. |
| Anything left | Lasso selection on the map to remove points by hand. |

### M3 — Segment, split into days, name

- Every stop is a candidate cut.
- **A day ends at an overnight rest**: a stop that overlaps the night window
  (default 00:00–06:00) and lasts at least a minimum rest (default 3 h). Without
  one, the day continues past midnight. Night is judged in the track's local
  time, not UTC.
- Cuts are proposed, then moved, added or removed on the timeline. Each segment
  is named, defaulting to its day and endpoints.
- Export as described above.

### M4 — Fill gaps from another GPX

- A time gap or a distance jump between neighbours is marked on the timeline.
- Open a second GPX, optionally clean it with M2, pick the part that overlaps the
  gap, preview it on the map and splice it in. Every spliced point records the
  file it came from.

### Later

Map matching to roads (HMM, Newson & Krumm 2009), 3D flyover, other source
formats such as FIT.
