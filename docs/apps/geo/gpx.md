# GPX

## Scope

This document owns decisions specific to `dgs geo gpx`. Read
[`../../tui.md`](../../tui.md) first for shared shell behavior.

GPX is the first command of the Geo app. Nothing here is a requirement for other
apps or commands.

## The iron rule

**A GPX dgs did not write is never written to.** Everything done to such a file
— cleaning, fills, cuts, names, added tracks, route plans — is kept in the
sidecar beside it. The recording itself keeps the bytes it was recorded with,
for as long as it is on disk.

This holds whatever the user asks for, and it has no exception for a command
that would be convenient:

- No command rewrites a recording in place, not to convert it, not to tidy it,
  not to migrate its sidecar into it.
- No command deletes or replaces a recording. Discarding a sidecar deletes the
  sidecar only.
- Anything that wants the edits inside a GPX writes a **new** file and leaves
  the source alone: the server creates a file that is not there and never
  writes over one.
- A GPX dgs wrote — its `creator` is this program — is ours to write: its
  names go into the file and its state into the file's `<extensions>`. That is
  the only kind of file dgs writes into.

The reason is that a recording cannot be made again. A sidecar can: every
setting in it can be entered a second time. So the file that cannot be
recovered is the one nothing may touch, and the cost of the rule — a copy on
disk rather than a file converted in place — is paid every time without asking.

`TestARecordingIsNeverWritten` pins it: every endpoint that changes anything
runs against a recording, and the recording's bytes have to come out the same.

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
| `/api/config` | The ways of travel the routers offer (`ways`: `id`, `label`, `service`), the base maps — built-in first, then configured — each with its coordinate system, the folder to open at, and `places`: the folders the save dialog lists down its side, each `name` and `path`. |
| `/ui/files/dir?path=&ext=&hidden=` | The shared file dialog's listing, mounted by `internal/webfile` rather than this app: one folder's folders and files, narrowed to the suffixes `ext` repeats. Documented in [`docs/web.md`](../../web.md). |
| `/ui/files/places` | The folders the shared file dialog lists down its side. |
| `/api/track?path=` | One GPX file as parallel per-point arrays: position, segment, distance, elevation, speed, time; plus its statistics, stops and parts — each track (as a range of those points, marked when added from another file), route and waypoint — its cleaning: the sidecar's settings, what each point was removed by, the recorded positions when cleaning moved any, and counts; its segments (`pieces`), saved cuts and proposed cuts; its fills; how many tracks were added to it, and `ours` when `dgs` wrote the file. `stopDistance` (metres) and `stopDuration` (seconds) override the stop thresholds. `coordinates=gcj02` returns positions, stop centres and bounds converted for GCJ-02 maps. |
| `PUT /api/clean` | Writes cleaning settings and manual removals to the companion sidecar of an external recording, or the embedded extension of a dgs GPX. Settings left out keep their defaults; an edit that does nothing clears the stored state. |
| `PUT /api/segments` | Writes cuts and segment names to the companion sidecar or embedded dgs state. |
| `PUT /api/part-name` | Names one `<trk>`, `<rte>` or `<wpt>`, keyed `t0`, `r0`, `w0`. The name is held under `names` until the edit is saved, which writes it into a GPX `dgs` wrote and into the sidecar of any other file. A track added here carries its name into the GPX it is written to. |
| `POST /api/waypoint` | Adds a named standalone `<wpt>` at WGS-84 `lat`, `lon` to an existing dgs-created GPX, or to a new GPX not yet saved. It is held under `waypoints` until the edit is saved. Files from other creators are refused. |
| `PUT /api/waypoint` | Puts the waypoint `key` names at WGS-84 `lat`, `lon`, for a pin dropped in the wrong place. Held under `moved` until the edit is saved. Only a GPX `dgs` wrote, or one carried beside a file, is moved; a recorded waypoint is refused. |
| `PUT /api/part-order` | Puts a file's tracks, routes or waypoints in the order `keys` gives, `kind` being `track`, `route` or `waypoint`; the keys are every key of that kind, as the page lists them. Held under `order` until the edit is saved, which puts the elements in that order inside the GPX. Only a GPX `dgs` wrote, or a new one not saved yet, is reordered; a recording is refused. A track's points go with it, so every point index the edits keep is moved to where its point now lies. |
| `DELETE /api/part` | Takes the `<trk>`, `<rte>` or `<wpt>` `key` names out of a file: out of what is held beside the file at once, and out of a GPX `dgs` wrote when the edit is saved, recorded under `deleted`. A track goes with the edits, cuts and fills on its points. Any other file is refused. |
| `POST /api/part/copy` | Copies the part `key` names into the GPX `target` names, as the page shows it, and takes it out of this file when `move`. Only a GPX `dgs` wrote, or a new one not yet saved, is copied into; only a file `dgs` wrote is moved out of. |
| `POST /api/segments/write` | Writes chosen segments of a track, as cleaned, one `<trk>` each: added to another GPX's sidecar (mode `add`), or into a new GPX it will not overwrite (mode `create`). The source is refused. |
| `POST /api/fill/route` | Asks a router for the road between two kept points of a track, with a way of travel `/api/config` lists under `ways`. Returns the route in WGS-84 and as drawn. Nothing is saved. |
| `POST /api/fill` | Records a route between two points in the companion sidecar or embedded dgs state, inserted after the first; later indices move along. |
| `POST /api/route/leg` | Asks the router for the road between two waypoints of a planned route, `from` and `to` as `[lon, lat]` in WGS-84, with profile `car`, `bike` or `foot`. Nothing is saved. |
| `POST /api/route/save` | Writes a planned route into a new GPX it will not overwrite — one `<trk>` of its legs, and a `<rte>` of its waypoints when `writeRte` — and keeps the editable plan in the GPX extension. |
| `DELETE /api/fill` | Removes a fill by its place in the list; its points go and the recorded ones come back. |
| `POST /api/draft` | Starts a new, empty GPX in memory, named by `name`, and answers its path, `draft:<n>/<name>.gpx`. Every other call takes that path as it takes a file's. |
| `DELETE /api/sidecar` | Discards companion sidecar or embedded dgs edit state. The recorded tracks and standalone waypoints remain. |
| `POST /api/save-as` | Writes the current edited tracks into a new GPX it will not overwrite: only points kept by cleaning, at their edited positions, including fills and added tracks. Cuts and editable plans are embedded in the new file; no sidecar is needed. |
| `GET /api/unsaved` | The files holding edits that are not on disk, so a reloaded page shows the same marks again. |
| `POST /api/save` | Writes the unsaved edits of the paths given — or of every file holding any, when none is given — each where it belongs: the sidecar beside a recording, the file itself for a GPX `dgs` wrote. Answers what was saved. |
| `DELETE /api/pending` | Drops unsaved edits, so the files read as they are on disk again. A new GPX has nothing on disk to go back to and is refused. |
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

- **Layout.** A top bar with the time zone selector, the Layers button and the GCJ-02 selector; a sidebar
  with the workspace; the map; and, under the map, the
  focused track's profile.
- **Resizable.** Every boundary between panes is a drag handle: sidebar width,
  map against profile, and elevation chart against speed chart. Sizes are
  remembered per browser.
- **No folder view.** The page never lists folders of its own: the file dialog
  is the one place folders are browsed, so the whole sidebar is the workspace.
  The folder the dialogs open at starts at `geo.gpx.root` (or `--dir`) and
  becomes the folder of the last file opened. A folder the browser remembers is
  used only while `geo.gpx.root` is unchanged: after the configuration changes,
  the dialog opens at the new root. The server reads the configuration when it
  starts, so a change takes effect after restarting `dgs`.
- **Workspace.** The tracks opened, kept together whatever
  folder they came from — the set later milestones edit and search. A row
  carries only what is looked at or reached for all the time: the eye, the
  reveal button and the mark saying who wrote the file. Everything done to a
  file is in its context menu. A filter box narrows the list by words in the
  name or path; the eye beside it acts on the tracks it lists.
  The header counts shown against total. A file with a sidecar says *sidecar*
  in its row (*sidecar unreadable* when it cannot be read).
- **The eye.** Showing and hiding are one state, so one control carries it:
  an open eye where the track is drawn, an eye struck through where it is not.
  It is the same control on a file's row, on one of its tracks, routes or
  waypoints, over the tracks the filter lists, and over a file's tracks,
  routes or waypoints as a group: while any of them is shown it hides them,
  and once none is it shows them.
- **Who wrote the file.** Beside the eye, a mark says whether dgs wrote this
  GPX — filled for a file dgs wrote, hollow for any other. It decides where a
  later edit goes: inside the file itself, or into a sidecar beside it.
- **Editing is not saving.** Nothing an edit does reaches the disk until it
  is saved. What was done to a file — cleaning, removals, cuts, names, fills,
  added tracks, parts deleted or copied in, a waypoint moved, a route's plan —
  is held in the `dgs` process serving this
  page, over what the file's sidecar says, so the page reads as if it were
  saved while the disk still holds the recording as it was. A file holding
  such edits carries a dot after its name and reads *edited, not saved*; its
  menu offers **Save**, which writes them where they belong — the sidecar
  beside a recording, the file itself for a GPX `dgs` wrote — and **Revert**,
  which drops them after asking. The workspace header carries a **Save**
  button while any file is edited, which saves every one of them. Several
  picked files save and revert together.

  The cost is stated where it is paid: unsaved edits live in one `dgs` process
  and one page. They go when `dgs` stops. What is bought is that an edit can
  be taken back whole at any point, and that a half-finished edit never
  reaches the disk.
- **Closing an edited file.** *Close* asks first when the file holds unsaved
  edits, or when it is a new GPX never saved — closing is the one way edits
  go without being seen again, because the file is not reopened with them.
  Nothing on disk changes either way.
- **Leaving the page.** Reloading or closing the tab asks first, while the
  workspace holds anything. A reload throws away what only the page holds —
  the tools open, the route being planned, the timeline's view — and starts
  the workspace again from what was saved, so the question is there to stop a
  reflex `Cmd-R` from leaving the screen and the disk saying different things.
- **Picking several files.** Rows are picked out as a file manager picks
  them: a plain click picks one, `Cmd`/`Ctrl` adds or takes one away, `Shift`
  reaches from the row last picked to this one over the rows the filter lists,
  and `Esc` clears the lot. Picking rows is not looking at one — only a plain
  click moves the map and the profile to a file, so the focused row and the
  picked rows are drawn differently and mean different things.
- **The context menu.** A right click on a file's row offers *Save* and
  *Revert* for what is edited but not on disk, then *Save as…* and
  *Convert to a dgs GPX…*, then discarding its sidecar or its embedded edits,
  then *Close*, which takes it out of the workspace and changes nothing on
  disk. A right click on one of its tracks, routes or waypoints offers
  *Rename…*, then *Copy to…* and *Move to…*, then *Move on the map…* for a
  waypoint, then *Delete*; **What is done to one part** says what each of them
  reaches. A right click
  on a row outside the picked rows picks that row first, as a file manager
  does; on one of several picked rows, the menu offers what can be done to
  each of them without naming a file — saving, reverting, showing, hiding and
  closing them.
  Saving and converting name a new file one at a time, so they stay on a
  single row's menu. The menu
  itself is the shared one of [`docs/web.md`](../../web.md); it offers nothing
  that is not also reachable another way.
- **Convert to a dgs GPX.** A recording keeps what is done to it in a sidecar
  because the recorded file is not ours to write. *Convert to a dgs GPX…*
  copies it into a new GPX dgs writes instead: the points as the page shows
  them, the names given here, and the cuts and route plans inside the copy, so
  the copy needs no sidecar and later edits go into the file itself. The copy
  is named and placed in the shared save dialog, the recording is not written,
  and an existing file is never replaced. The copy takes the recording's place
  in the workspace, open as it was; the recording stays on disk. A file dgs
  already wrote has nothing to convert. This is *Save as…* under the name of
  what it is for, and it writes the same file.
- **Show in file manager.** A track row has a button, on hover, that reveals
  the file in Finder or Explorer. It appears only when the
  page is opened on the machine running `dgs`: from another machine it would
  open windows on a screen nobody there can see.
- **Dropping files.** GPX files dragged from Finder or Explorer onto the
  sidebar are opened in the workspace, as *Open…* opens them. The browser
  hands over no path for a dropped file, but the file manager puts the files'
  URLs on the drag and those carry the path — so, like the rest of opening by
  path, this works only when the page is opened on the machine the files are
  on. Anything dropped that is not a GPX on this machine is refused with a
  line in the sidebar.

  Which browser the page is opened in decides whether dropping works at all.
  Safari and Firefox put the file URLs on a Finder or Explorer drag, so the
  path is there to read. Chrome and the browsers built on it put only the
  files themselves and withhold where they came from, and a page cannot ask;
  there, dropping says so and names *Open…*, which reaches the same files by
  path. This is a limit of the browser, not something the page can work
  around — which is why *Open…* is the way that always works and dropping is
  the shortcut.
- **Opening files.** *Open…* in the Workspace header opens the shared file
  dialog ([`docs/web.md`](../../web.md)), which reaches a GPX anywhere on the
  machine: several files at once. Only GPX files are listed and only a file can
  be answered — a folder is stepped into, never chosen — so `GPX files` is the
  one filter. Each chosen file is added to the workspace, shown; one already
  there is shown and focused instead. Nothing on disk changes. The folder they
  came from is where the next dialog opens.
- **Choosing where to save.** Everything that writes a new file — a planned
  route, a new GPX, *Save as…*, and *Save as a new GPX…* on the Segments tab — opens the
  shared file dialog's save mode ([`docs/web.md`](../../web.md)) instead of a
  typed path: the same browser as *Open*, with a name field, *New Folder*, and
  F2 to rename. `.gpx` is added when it is left off. It opens at the folder the
  file belongs to — the last folder opened from, or the one beside the file being saved —
  and falls back to the folder `dgs` opened at, `geo.gpx.root` or the home
  directory, when that folder is gone. The server writes a new file and never
  writes over one, so the dialog refuses a name that is already taken rather
  than offering to replace it. The full path it returns is what the server is
  asked to write.
- **Names.** A track is named by its file name. The name inside a GPX file is
  often only a recorder's timestamp; the file name is the one the reader chose.
  File names run long, so the sidebar reads a size smaller, a name wraps to two
  lines (breaking anywhere, since names mix words, dates and CJK) before it is
  cut, details sit on a line of their own, and row buttons appear over the
  row's end on hover rather than reserving width. The full path is the row's
  tooltip.
- **What a file holds.** A GPX file may hold several tracks, planned routes
  and waypoints. Every workspace row that holds anything has a disclosure (▶,
  at the row's end so rows start at the left edge) that lists them beneath it
  — a file of one track too, so that track can be shown on its own, framed and
  renamed, whoever recorded it. The bar that shows or hides them all appears
  only when there is more than one; a single part shows and hides from its own
  row. The list holds: tracks with
  their distance and segment count, routes (drawn dashed) with their length,
  waypoints (drawn as dots) with their description and elevation. Each has its
  own show/hide, and a bar above the list shows or hides them all at once, or
  all of one kind — every track, route or waypoint; clicking one frames it on the map, and clicking a waypoint, in
  the list or on the map, opens its name, description, elevation and time.
  Which parts are hidden and which rows are expanded are remembered. Profile,
  stops and timeline still cover all the file's tracks together, but hatch the
  ranges of hidden tracks on both charts and the timeline, and a stop inside a
  hidden track has no marker on the map.
- **Renaming a part.** Every part row has `✎`, on hover, which asks for a new
  name for that `<trk>`, `<rte>` or `<wpt>`. The name is held beside the file
  under `names`, keyed `t0`, `r0`, `w0`, until the edit is saved; saving puts
  it where it belongs, which follows who wrote the file: into a GPX `dgs`
  wrote, and into the sidecar of any other file, which is never written. A
  track added here carries its name into the GPX it is written to. The dialog
  says which of the two will happen before the name is typed.
- **What is done to one part.** Renaming is not all a part row's context menu
  offers:

  - *Delete* takes that `<trk>`, `<rte>` or `<wpt>` out. What is carried
    beside the file — a track added from another GPX, a part copied in — goes
    as it came. A part the GPX itself holds goes out of the file when the edit
    is saved, and *Revert* puts it back until then, so the menu asks first.
    Only a GPX `dgs` wrote is taken out of: a recording keeps every part it
    was recorded with, and the menu says so where it is greyed out.
  - *Copy to…* and *Move to…* ask for another GPX of the workspace and put the
    part in it, as the page shows the part: a track with the points cleaning
    kept, at their edited positions, fills included. Only a GPX `dgs` wrote,
    or a new one not saved yet, is copied into, so nothing is written into a
    recording; a recording's own parts are copied out of it freely, and moved
    out of nothing. *Move* is a copy and a delete, and each file holds its
    half of the change until it is saved.
  - *Move on the map…* is for a waypoint pinned in the wrong place. The
    waypoint becomes a pin that is dragged to where it belongs; `Esc` leaves
    it where it was. Where it is dropped is read in the system the map draws
    in and kept in WGS-84. Only a waypoint of a GPX `dgs` wrote, or one
    carried beside a file, is moved: a recorded waypoint is where it was
    recorded.

- **A waypoint on the map.** The pointer over a waypoint grows it a little and
  thickens its ring, so a dot answers the pointer before it is clicked; a
  click opens what the list's click opens. A right click on it offers what the
  waypoint alone takes — *Rename…*, *Move to a new position…* and *Delete* —
  the same commands as its row, greyed out for the same reasons. Elsewhere on
  the map the right click is the map's own.

- **Dragging rows.** There is no drag handle, because a row carries its own
  controls. With a mouse, pressing a row and moving it a few pixels takes hold
  of it; with a finger or pen, the press must first hold still briefly, since
  moving at once is a scroll. Until then the press is a click like any other.
  A copy of the held row follows the pointer, and the rows it passes slide
  aside to open the gap it would land in; the map list sorts the same way. `Esc`
  during a drag leaves everything as it was. Where it lands says what it does:

  - A **file** over another file lists the workspace in that order. The order
    is the page's own — no file is written — and is remembered with the
    session. The up and down arrows on a file's row move it one step among
    the rows listed, for the same order without dragging.
  - A **part** dropped inside its own file does nothing. A track, route or
    waypoint is put in order among those of its kind by the ▲ and ▼ on its
    row, one place per click. The order is held beside the file, keyed as the
    names are, and goes into the GPX itself when the file is saved. Only a
    GPX `dgs` wrote, or a new one not saved yet, shows the arrows, and only
    on a kind it holds more than one of; a recording keeps the order it was
    recorded in. A track's points go with it, and so do the edits on them:
    the cleaning, cuts and fills are kept by point index, so every index is
    moved to where its point now lies, and a removed stretch that spanned two
    tracks put apart parts in two.
  - A **part** over another file is moved into that GPX, or copied into it
    while `Ctrl` or `Cmd` is held. It is the same work as *Move to…* and
    *Copy to…*, under the same rule: nothing is written into a recording, and
    nothing is taken out of one. A drop that would do either is refused, marks
    the row red, and says why in the status bar.

  A part keeps its key while another part is deleted — the second `<trk>` of a
  file stays `t1` when the first goes — so a name or an edit given to it still
  names it. The keys are counted afresh once the deletion is written and the
  file on disk holds one part fewer.
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
- **Maps.** OpenStreetMap, OpenFreeMap Fiord (a dark blue-grey vector style,
  whose roads still read under a coloured track), Gaode (街道), Gaode Satellite
  (imagery with Gaode's road and place names above it) and Esri World Imagery
  are built in and need no key, followed by the maps in `geo.gpx.tiles` and then those in
  `<config_dir>/geo/gpx/tiles.json` — `{"tiles": [...]}`, each entry as in
  `geo.gpx.tiles` — which keeps a long list out of the configuration file. A
  tiles file that cannot be read keeps the server from starting, saying why. Gaode's tile
  addresses are undocumented and could change; the rest are public services.
- **GCJ-02.** Gaode draws in GCJ-02, so a WGS-84 track sits several hundred
  metres off it. The selector's *Auto* converts what is drawn — tracks, stops,
  the inspected point and the stop circle — whenever the lowest shown map is a GCJ-02
  one, and says whether it is on (and when no workspace track is in China,
  where it would change nothing). *On* and *Off* override it, for a
  configured map marked wrongly or to compare. The server converts
  (`gcj02.FromWGS84`); distances, speeds and stops are measured in WGS-84
  whatever is drawn. The choice is remembered.
- **Credits.** The map's attribution is folded into the ⓘ in its corner, so
  the map itself has the page; the pointer on it, or a click, opens it. The
  fold is a stylesheet rule rather than script: MapLibre opens the credits
  again on every attribution change, and answering that in script would mean
  writing to the element the map is writing to.
- **Layers.** The *Layers* button in the top bar opens every map — the built-in
  and configured ones — and the contour lines as one list. Any number may be
  shown at once, each with its own opacity, so a transparent overlay (OSM GPS
  traces, OpenRailwayMap, contours) sits over an opaque map, or two imageries
  blend. Dragging a row by its handle reorders the list; the top is drawn on
  top, and everything under the tracks. The button names the lowest shown map,
  which GCJ-02's *Auto* follows; a shown layer in the other system than the
  tracks carries a ⚠, as it sits several hundred metres off them in China.
  A **vector style** — OpenFreeMap Fiord — is a map like any other in the
  list: its own sources and layers are put under the tracks, under that map's
  prefix, so the page keeps its own style and the tracks keep their place
  above. Its opacity fades every layer of it that carries a plain opacity; a
  layer whose opacity is an expression is left as its author wrote it. It
  carries no saturation, contrast or brightness — those are raster paint — so
  its row has no ▸. The map has one set of glyphs and one sprite, so showing
  two vector styles at once leaves the labels of the one drawn last. A style
  that cannot be fetched draws nothing and keeps its row, to be tried again by
  hiding and showing it.
  A map row's ▸ opens its saturation, contrast and brightness (−100 to +100,
  MapLibre's raster paint; double-click a slider or *Reset* to clear): a grey,
  lightened OpenStreetMap under the GPS traces lets the traces stand out. The
  ▸ turns blue while a map is adjusted. The list — order, what is shown,
  opacities and adjustments — is remembered by name. A remembered map the
  configuration no longer offers is simply gone from the list; if it was the
  only one shown, the first map takes over, so the list never reads as all
  off with nothing drawn.
- **Contours** are drawn in the browser from raster elevation tiles with
  maplibre-contour (vendored, BSD-3): thin every 10–200 m and bold every
  50–1000 m, closer as the map zooms in, from zoom 9. `geo.gpx.dem` names the
  tiles — `{"url": "https://…/{z}/{x}/{y}.png", "encoding": "terrarium" |
  "mapbox", "max_zoom": 15}` — and defaults to AWS's public Terrarium tiles,
  which need no key but can be slow or unreachable from mainland China; point
  it at a mirror or another terrain-RGB service there. The elevation is
  WGS-84. The lines carry no elevation labels: the map style has no glyphs.
- **3D relief.** **3D** over the map, `D`, or the switch at the foot of the
  *Layers* panel reads the same
  elevation tiles as terrain, so the map tilts to 60° and the ground rises
  under the tracks. It comes with a hillshade drawn from the same tiles, under
  the tracks — terrain alone tilts the ground but barely shows it, since a
  slope is read by the way it is lit — and the map may then be tilted by hand
  to 80°. The vertical stretch starts at 2.5× — real scale reads flat at the
  zooms a walk is looked at — and the slider beside the switch takes it from
  1× to 8×, the shading following it. Terrain tiles are asked for no deeper
  than zoom 12, where the public Terrarium tiles thin out; a deeper view
  stretches that one, which reads better than the flat ground a missing tile
  would give. Only the drawing changes: a position, a distance and a speed are
  what they were. Turning it off levels the map again. The stretch is
  remembered; whether the relief is on is not, so the page opens flat.
- **Time zone.** GPX times are UTC. Every time on the page — the start in the
  profile header, the inspected point — is shown in the time zone chosen in the
  top bar, with its UTC offset. It defaults to the browser's zone and is
  remembered.
- **Stops.** The focused track's stops are drawn as numbered markers.
  Clicking one opens its arrival, departure and duration. Pointing at a stop,
  on the map or the timeline, draws a thin dashed circle of radius D around
  it, so the threshold can be judged against the place. Stops › *Map*, beside the
  thresholds, hides the numbered markers from the map while the timeline keeps
  its stop blocks for jumping to them; it is remembered. Cursor › *Snap*
  makes the timeline's cursor stick to a stop's arrival or departure, a cut, a
  hidden stretch's edge or the track's ends within 10 px, holding there until
  the pointer moves 18 px away; off, the cursor follows the pointer freely
  while the map and charts show the nearest point. It is remembered too.
  Stops › *Timeline* turned off draws no stops on it: the bar shows
  only the stretches with data and the empty time between them — a stretch
  ends where the segment changes or no point came for a minute (or ten times
  the usual interval, if longer) — and Snap sticks to those stretches' edges.
  It is remembered. Axis › *Compress* draws every stop (while Stops › Timeline is on) and every
  empty stretch at a fixed 24 px of the whole track's width, dashed at its
  edges and labelled with the times at both ends, so moving time takes the
  bar; the overview, zoom and pan follow the same axis. It is remembered and
  starts off. Axis › *No empty* leaves empty stretches out entirely: each shows as
  a dashed line with the times on either side, while stops — even one with
  nothing recorded during it — keep their width
  (or Compress's). It is remembered and starts on. The thresholds — at
  least a time T within a distance D — are set beside the timeline and apply
  to every shown track.
- **Timeline.** Above the charts, the focused track's clock time from first to
  last point: moving periods in the track colour, stops as numbered dark
  blocks, and hatched gaps where nothing was recorded. Moving the pointer along
  it, or dragging, marks the point recorded nearest that moment on the map and
  the charts; pointing at the map or a chart marks the timeline. Clicking a
  stop opens it on the map. A file without time says so in its place.
- **Timeline zoom.** The wheel, or a trackpad pinch, zooms the timeline about
  the pointer, redrawn at most once a frame however fast a trackpad sends
  events. Its
  narrowest view is the shortest recorded time in which this track travels one
  metre; Shift-wheel, a sideways scroll or Option-drag (Alt-drag off a Mac)
  pans. One wheel gesture stays either a zoom or a pan throughout, even when
  macOS changes the delta axis or omits Shift on later frames. A mouse wheel
  counting in lines pans as far as a trackpad; double-click shows the whole
  track again. A strip above it always keeps its layout space, so zooming does
  not move the charts; while zoomed it marks the stretch shown — drag it to
  pan, click beside it to jump — and the end labels gain
  seconds, and ticks mark round times in the page's time zone, from days down
  to seconds. The charts zoom with it: the timeline's stretch shows on them as
  the distance those points cover, and zooming a chart shows on the timeline
  as the time the points in its distance range span. A stop has no distance,
  so a stretch of mostly stop shows on the charts as little more than a point.
- **Profile.** The focused track has an elevation chart and a speed chart
  (km/h) against distance, with its distance, duration, ascent and descent. A
  file without elevation or without time says so in place of that chart.
- **Charts on demand.** Toggles in the profile header show or hide each chart
  on its own. With both hidden the profile shrinks to its header and the
  timeline, and the map takes the space. With both shown they sit side by side
  by default, or one above the other, splitting the space evenly until the divider is
  dragged; each arrangement remembers its split as a proportion, so a taller or
  wider profile keeps both charts readable.
- **Inspecting a point.** Moving the pointer along the focused track on the map
  marks the nearest point on the map and draws a marker line at the same place
  on both charts; moving along a chart marks the point on the map. A readout
  shows distance, elevation, speed and local time of that point.
- **Zoom.** Dragging across a chart or scrolling on it zooms the distance axis,
  no narrower than one metre; double-click resets. Both charts always show the
  same range. *Fit* (⤢) in the profile header puts the whole track back
  across both charts and the timeline, and frames on the map the points the
  timeline shows: kept, in shown tracks, without the file's routes and waypoints.
- **Folding a panel away.** Each panel carries the button that hides it — the
  workspace `‹`, the charts `▾`, the edit panel `›` — and the map then carries
  a small tab at that edge to bring it back, so nothing is hidden without a
  way back in the same place. `W`, `E` and `I` do the same from the keyboard.
  ⛶ over the map, or `M`, is **map only**: every panel folds away at once, and
  pressing it again puts back the ones that were open. A folded panel is
  remembered. The charts' tab is there whenever they are folded; the edit
  panel's appears in Edit and Route, the modes that have one.
- **Remembered.** The browser remembers, per browser, the folder the dialogs
  open at, the workspace with each track's colour and visibility, the focus, the
  time zone, the layers, the pane sizes, the folded panels, the stop
  thresholds, and which charts
  are shown and how, and restores them on reload.

## Cleaning

*Edit*, in the top bar beside *Browse*, opens the inspector — a column on the
right of the page, dragged wider or narrower — for the focused track. Its
*Changes* tab lists removals by hand and fills along the road; *Automatic
cleaning* holds the filters. A tool bar on the map's corner picks the tool:
select, *Range* (`R`), *Lasso* (`L`) and *Fill along the road* (`F`); `Esc`
leaves it. Under a divider it carries *Compare with the recording*, a toggle
rather than a tool, so the inspector's header holds only the track's name. A line at the map's foot says what a click does with the tool in
use.
Nothing is removed until a tool is used or a filter is switched on. Settings
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
| By hand | Ranges removed on the timeline or as a stop, and points removed with the lasso. They go first, so they cannot sway a filter. | — |
| Sudden jumps (`spikes`) | A point reached and left faster than the max speed; a point the speed rose into and fell out of faster than the max acceleration; a point the path turns back at by the turn angle, standing at least the min jump from both neighbours and further from each than they are apart; a run of up to 5 points entered and left too fast while passing straight by is not. | max speed 360 km/h, max acceleration 15 m/s², turn back 160°, min jump 30 m |
| Speed drift (`hampel`) | A point whose speed from the previous point is above the median of its neighbours' by the threshold × scaled MAD (never less than the min spread). Only too fast counts. Each pass removes the worst in its neighbourhood and measures again, so the point after a false fix is not removed with it. Optionally stricter, down to a quarter, for fixes with HDOP above 2 or fewer than 6 satellites. | 7 neighbours each side, threshold 3, min spread 3.6 km/h, stricter for poor fixes |
| Position jitter (`kalman`) | Moves points: a constant-velocity Kalman filter and RTS smoother over east and north, a fix counting as fix error × HDOP metres uncertain. Smoothing restarts at a segment or after a pause. | fix error 5 m, acceleration 1 m/s², restart after 60 s |
| Scribbles in stops | Inside each stop found on what is left, keeps the points where it was entered and left and the one recorded midway, moved to the stop's centre, and removes the rest. | within 50 m, at least 5 min |

Points filled in along the road (below) are never removed or moved by a
filter, and the recorded points a fill replaces are removed before any of
them, coloured light blue.

A max speed of 360 km/h removes a flight; raise it for tracks recorded in the
air.

Distances, speeds, statistics, stops, the profile and the TUI summary are all
measured on what cleaning kept, at its moved positions. The track's line skips
removed points. *Compare with the recording*, on the map's tool bar, on by
default and remembered, draws the recording under it as a thin dashed line with removed points as dots
coloured by what removed them: spike red, drift orange, stop grey, by hand
purple.

**Removing by hand.** Every removal is one edit, listed in the panel newest
first with what it removed and when. *Undo*, or ⌘Z (Ctrl+Z off a Mac) while
the panel is open and no field has the keyboard, takes back the latest; any
edit's `×` restores just that one.

- **Range.** With *Range* on, dragging across the timeline removes the points
  in that stretch; zoom the timeline to place it to the second. Or click the
  track on the map at the start — it is marked — and then at the end. The
  track breaks there by default. The first Esc forgets a start not yet
  finished; the next turns *Range* off.
- **Remove this stop**, in a stop's popup on the map, removes every point of
  the stop, joined across.
- **Join or break.** Each removed range is hatched on the timeline and listed
  in the panel with its time span. *Break* leaves no line and no distance
  across it, and starts a new `<trkseg>` when written, as a pause in the
  recording would. *Join* draws a straight line across and counts its
  distance. Clicking a range on the timeline switches it or restores its
  points; so do its row's selector and `×`; clicking the row frames it.
  Removed stretches are also drawn on the map, along where they were
  recorded, as a pale purple band with a dashed centre.
- **Lasso.** With *Lasso* on, dragging on the map draws a shape; the focused
track's kept points inside it are removed as one edit. Holding Option (Alt
off a Mac) while drawing takes the points inside it out of earlier lasso
edits instead. Lasso removals are
always joined across. Map panning is off while the lasso is on; Esc or the
button turns it off, and *Restore* returns them all. Points in hidden tracks
are left alone.

**Filling along the road.** For a stretch recorded badly or not at all — a
tunnel, a phone that lost the signal — the panel's *Fill along the road* asks a
public OSRM router for the road instead. Nothing is filled automatically: a
fill is made only this way, on request.

- Choose the way of travel — car, bicycle or foot — and pick *Fill along the
  road* on the tool bar.
  Click the track on the map where the stretch starts (it is marked), then
  where it ends. Only those two positions, as cleaning left them, are sent:
  cars to `router.project-osrm.org`, bicycles and walking to
  `routing.openstreetmap.de`. The profile is remembered.
- The route is drawn dashed with its length. *Use this route* inserts its
  points between the two ends and removes the recorded points between them;
  *Discard*, Esc or picking again forgets it. A route that fails — no road,
  no network — says why in the panel.
- Filled points get times spread by distance between the two ends' times and
  elevations interpolated the same way, and join the track across a pause in
  the recording. They are banded light blue on the map, and written with
  `<src>dgs-toolbox: filled along the road</src>`.
- Each fill is listed with its time span, length and points; clicking it
  frames it, `×` removes it and brings back the recorded points. A fill
  cannot cross another.

The route is stored in the sidecar as coordinates, so a fill never needs the
router again.

## Routers

Filling along the road and planning a route ask a router for the road
between two points. Every way of travel names the service that routes it:

- **OSRM** — car, bicycle, foot — on OpenStreetMap, from the public servers
  (the OSRM project's for cars, FOSSGIS's for bicycles and walking). No key.
- **Amap (高德)** — car, bicycle, e-bike, foot — with its Web Service route
  planning (v5), on Amap's own map data, the most complete for China. Offered
  only when `geo.gpx.amap_key` holds a Web Service key. Amap works in GCJ-02:
  the server converts the two points before asking and the route back to
  WGS-84 after, so what is saved is WGS-84 like the rest (the conversion is
  good to about 1–2 m). The key stays on the server; the page never sees it,
  and an error never repeats it.

Only the two points go to the service. The selects group the ways by service.

## Planning a route

*Route*, in the top bar, plans a route by hand rather than from a recording:
no track needs to be open. The inspector holds the plan; the map takes the
clicks.

- **Waypoints.** Clicking the map adds a waypoint at the end. A click within
  40 px of the first or last point of a shown track in the workspace snaps to
  it, so a route starts or ends where a track does. Waypoints are numbered
  markers — the start green, the end black — dragged to move them and
  double-clicked to remove them; a small circle midway along each leg is
  dragged to insert a waypoint there. The inspector lists them, each with `×`,
  and clicking one centres the map on it.
- **Legs.** Between each two waypoints a leg goes by car, bicycle or foot —
  asked of the public OSRM routers, which receive only its two waypoints — or
  straight, drawn dashed. *New legs* sets the way of the next one; each leg's
  own way changes in the list. Moving or removing a waypoint asks again only
  for the legs beside it; a leg that fails says why and offers *Retry*. The
  road starts where the router meets the road, so a leg joins it to its
  waypoints in a straight line.
- **Summary and tools.** The total length, the number of waypoints and, when
  every leg follows the road, the router's time. *Reverse* turns the route
  round, *Close loop* adds the start again at the end, *Clear* starts over.
  The plan on the page is remembered by the browser until it is cleared.
- **Saving.** *Save as GPX…* opens the save dialog for a new file — an
  existing one is never replaced — and writes one `<trk>` of the legs joined,
  its points marked `<src>dgs-toolbox: planned route</src>`, and a `<rte>` of
  the waypoints when *Also write the waypoints as a <rte>* is on. No times or
  elevations are written. The file is added to the workspace, and the plan —
  waypoints, and each leg's way and points — is kept in its sidecar as `plan`.
- **Editing a saved route.** With a file planned here focused, the inspector
  offers *Edit its route*: the plan loads, named after the file with ` 2`, and
  is saved again as a new GPX.

## Segments

In Edit, the inspector's *Segments* tab — or the *Cut* tool (`C`) on the map's
tool bar, which opens it — cuts the focused track. While it is shown, clicks
on the track and the timeline cut, and the other tools are off; picking one of
them goes back to *Changes*. A cut is a point
where one segment ends and the next begins; both segments hold that point, so
nothing is lost between them. Segments are cut from what cleaning kept.

- **Proposed cuts.** Every stop proposes a cut at the point recorded midway
  through it, shown on the timeline as a dashed mark. Clicking one cuts there;
  *Cut at every stop* takes them all.
- **Editing cuts.** On the Segments tab, pointing at the focused track on the
  map marks the point a cut would fall on, and clicking cuts there; clicking
  the timeline, stop blocks included, cuts at the point under the pointer,
  while dragging it still scrubs. Cuts are yellow handles on the timeline. drag a handle to move the
  cut, double-click it to remove it, or use × beside the segment that starts at
  that cut. The first segment has no cut before it. *Clear cuts* removes them all. Cuts are
  also marked on the map.
- **Colours.** While the tab is shown each segment is drawn over the track in
  a colour of its own, neighbours always different, with the same colour on
  its number in the table and as a strip along the foot of the timeline.
- **The list.** One row per segment, two lines: its box, colour and number
  with its name, then its time span, duration and distance under them. A bar
  over the list chooses all or none and says how many are chosen. Nothing is
  laid out in columns, so a narrow inspector hides nothing.
- **Names.** A segment is named by its time span in the page's time zone —
  `2026-09-06 09:00–09:26`, or with both dates when it crosses midnight —
  until a name is typed. A name belongs to the segment starting at its point:
  moving that cut keeps it; removing it merges the segment into the one before,
  and the name goes. Pointing at a segment highlights it on the map; clicking
  it frames it.
- **Actions.** Everything done to the segments is in one section under the
  table: *Cut*, holding *Cut at every stop* and *Clear cuts* with the number
  of cuts, then *Save segments to*, holding the targets with the number of
  segments chosen.
- **Saving.** Chosen segments (all, by default) are written as cleaned, one
  `<trk>` each named as shown, with their points' elevation, time, HDOP and
  satellites, a `<trkseg>` per recorded segment. A target is always a GPX
  `dgs` manages — one in the workspace, or one it creates:
  - **a GPX in the workspace**, picked from the list beside *Add n segments*,
    each segment its own track after that file's tracks. This is how part of
    one recording is moved into another file. The GPX that gained a track
    opens in the workspace to list what it now holds. The GPX on disk is not
    written: the added tracks are kept in its sidecar, as points, and it shows
    them at once — listed under it as *added from* their file, each with `×`
    to take it out again, and its row marked *added, not saved*. They can be
    cleaned, cut and filled like the file's own; or
  - **a new GPX**, named in the save dialog *Save as a new GPX…* opens, with
    `<source> segments.gpx` beside the source offered; an existing file is
    never replaced. The file written joins the workspace, shown, so its
    segments are there to see at once.

  No GPX already on disk is written, whichever is chosen.

**A new GPX.** *New GPX*, in the Workspace header, asks for a name and adds an
empty GPX to the workspace. It is not written anywhere: it lives in the memory
of the running `dgs`, so it survives reloading the page but not quitting `dgs`.
Segments are added to it from any track's Segments tab, as to any workspace GPX,
and it can be cleaned, cut and filled the same way. Its row says *new, not
saved* and, once it holds a track, has *Save…*, which opens the save dialog —
at the last folder opened from, named `<name>.gpx` — and writes it there,
never replacing an existing file. The saved file then takes its place.

**Saving as a new GPX.** A workspace GPX with edits has *Save as…* on its
row. It opens the save dialog beside the file, named `<name> edited.gpx`, and
never replaces an existing file. The new GPX holds a snapshot of its tracks:
only the points cleaning kept, at their edited positions, including fills and
tracks added from other files. Saved cuts and editable route plans are
embedded in the new file, which has `creator="dgs-toolbox"` and needs no
sidecar. The original GPX is not written.

**Adding a standalone waypoint.** In Edit, choose ⚑ and click the map. The
tool needs no focused track — it writes into a GPX the dialog asks for — so it
stays available when the other tools, which work on the focused track, are
not. The dialog asks for a name, optional description, and destination among
the open GPX files created by dgs and the new GPX files not yet saved. A file
from another creator is never changed in place; save it as a new dgs GPX
first. A new GPX is not on disk yet, so its waypoints are kept with it in
memory, under `waypoints` in what it holds instead of a sidecar, and are
written into the file it is saved as. The coordinate is WGS-84 even when a
GCJ-02 map is displayed. This is a `<wpt>`, not a route-planning waypoint.

**Embedded edit state.** New dgs GPX files identify themselves with the
standard root `creator` attribute. Subsequent edits are stored directly in
their GPX `<extensions>` as dgs state rather than in a new companion file.
Save as… merges an external GPX and its sidecar into a new standalone dgs GPX.
The extension is not encryption or
authentication: it is readable by dgs and ignored by ordinary GPX readers.

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
    "edits": [
      { "kind": "lasso", "points": [118, 119, 120], "join": true, "at": 1788657200000 },
      { "kind": "range", "first": 900, "last": 1256, "join": false, "at": 1788657300000 },
      { "kind": "stop", "first": 2100, "last": 2390, "join": true, "at": 1788657400000 }
    ]
  },
  "segments": {
    "cuts": [2339, 4159],
    "names": [{ "start": 2339, "name": "逛西湖" }]
  },
  "fills": [
    { "first": 3000, "last": 3140, "profile": "car", "route": [[120.1501, 30.2502], [120.1512, 30.2515]], "at": 1788657500000 }
  ],
  "names": { "t1": "Evening ride" },
  "order": { "w": ["w2", "w0", "w1"] },
  "added": [
    { "name": "Phone, tunnel", "from": "/trips/phone.gpx", "at": 1788657600000,
      "segments": [[{ "lon": 120.16, "lat": 30.26, "ele": 12.5, "time": 1788650000000 }]] }
  ]
}
```

Units are the SI ones the filters use: metres, seconds, m/s, m/s². An edit's
`points` (a lasso's), `first` and `last` (a range's or stop's, both removed),
`cuts`, a name's `start` and a fill's `first` and `last` are point indices in
file order, counting every track and segment, then the added tracks, with every
fill's route counted where it is inserted; `at` is when the edit was made, in
Unix milliseconds. A fill's route (`[lon, lat]`, WGS-84) takes the indices
right after `first`; the recorded points after it up to `last` are the ones it
replaces. Inserting a fill, or removing a fill or an added track, moves every
later index; what pointed only at points that went goes with them. `order` is the parts of one kind — `t` for tracks, `r` for routes, `w` for waypoints — in the
order the page put them, keyed as `names` is. It is taken into the GPX itself
when the file is saved, so only a GPX `dgs` wrote, or a new one not saved yet,
holds one. The indices above count the tracks in that order — the file's
own tracks and the added ones together — so putting tracks in another order
moves every index with its point, and saving writes the tracks in that order,
where the indices still find them. Unlike the
rest of the sidecar, fills and added tracks hold track data, since neither can
be found again from the source. A cut
on a point cleaning later removes moves to the next kept point. Sidecars from
before edits kept `removed` and `ranges` lists instead; they load as edits and
are written back as edits. Settings
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

- **Sources are read-only.** Cleaning, cutting, gap filling and tracks added
  from other files are recorded in a sidecar file beside the source GPX and
  applied on top of the source, so every change can be undone or re-run with
  other parameters. A GPX changed by added tracks is saved as a new file.
  Every point knows which rule removed it.
- **The map is on the web page; the TUI is the entry point.** The TUI chooses
  files, shows summaries, starts the server and exports. Anything that needs a
  map or a timeline is on the page.
- **Only GPX files** are read, for now: tracks, routes and waypoints.
- **Export** writes one GPX file holding one `<trk>` per segment, into a folder
  the user chooses.
- **The sidecar** is a small JSON file stored next to the source GPX. It holds
  what was done: cleaning parameters, points removed by hand, cuts, segment
  names and gap fills — and, until saved, tracks added from other files.
  Opening a GPX that has one resumes the work.

### Map and coordinates

- OpenStreetMap, OpenFreeMap Fiord, Gaode, Gaode Satellite and Esri World
  Imagery are built in.
  Further tile sources are listed in `geo.gpx.tiles` or the tiles file, each with a name, a URL
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

Done, in part: see *Segments* above — cuts proposed at stops and edited on the
timeline, named segments, and writing them into a new or another GPX.
Deferred, not yet designed further:

- Splitting into days at an overnight rest, below, and the track's local time
  zone it needs.
- Default names from the segment's endpoints (place names).
- Export to a folder the user chooses, from the TUI, as the principles
  describe.

The original plan:

- Every stop is a candidate cut.
- **A day ends at an overnight rest**: a stop that overlaps the night window
  (default 00:00–06:00) and lasts at least a minimum rest (default 3 h). Without
  one, the day continues past midnight. Night is judged in the track's local
  time, not UTC.
- Cuts are proposed, then moved, added or removed on the timeline. Each segment
  is named, defaulting to its day and endpoints.
- Export as described above.

### M3.1 — Precise cutting and editing

Done: see *Timeline zoom*, *Segments* and *Removing by hand* above.

Key hints on the page name the keys of the system it runs on: ⌥ Option, ⇧
Shift and ⌘ on a Mac; Alt, Shift and Ctrl elsewhere.

A single file, edited precisely. It is also the ground M4 stands on: every
source is cleaned on its own before it is combined.

- **Timeline zoom.** The wheel zooms the timeline about the pointer; Alt-drag
  or Shift-wheel pans; double-click resets. An overview strip above it shows
  the zoomed range and drags to pan, while its space remains reserved when it
  is inactive. Ticks follow the zoom, from dates and hours down to seconds. The
  timeline and the charts zoom together, matched by the points both show, with
  a shared minimum spatial range of one metre. Zoomed in, cuts and ranges snap
  to single points.
- **Cutting on the map.** In cut mode, pointing at the track marks the point a
  cut would fall on; clicking cuts there. Shift-click on the timeline stays.
- **Edit replaces Clean.** The panel keeps the automatic filters and gathers
  removal by hand: the lasso; dragging a range on the timeline to remove the
  continuous points in it; *Remove this stop* on a stop's popup and timeline
  block. Removed ranges are marked on the timeline; clicking one offers
  *Join*, *Break* and *Restore*.
- **Join or break.** Each removed range chooses whether the points either side
  of it are joined — a straight line, counted in distance — or broken, like a
  pause in the recording: no line, no distance, and a new `<trkseg>` when
  written. A range removed on the timeline breaks by default; points removed
  with the lasso join, as now.
- **Ranges in the sidecar.** Removal by hand is stored as ranges of point
  indices, each with its join or break; the list of single indices written
  until now still loads.

### M4 — Combine sources

Done: see *Filling along the road* and *Segments* above.

A trip is often recorded by more than one device: a GPS left in the car while
a watch records the hike, or a phone that loses the signal in a tunnel. M4
combines them by hand, not by rules:

- **Moving a stretch between files.** Cut the stretch out of one file's track,
  and add it to another GPX in the workspace, as a track of its own. The
  receiving file is changed only in its sidecar, and saved as a new GPX.
- **Filling from the road, on request.** Between two points picked on a track,
  a route from a public OSRM router (OpenStreetMap, WGS-84, no key; only the
  two end points are sent) replaces what was recorded. Its points get times
  spread by distance and elevation interpolated, are previewed before they are
  used, marked as filled and stored as coordinates.

Dropped from the earlier plan, as combining stays a choice made by hand: a
merge project file, a timeline lane per source, suggesting which source to
trust. Deferred: shifting a source's clock. Routes drawn by hand come back as
M6.

### M5 — Room to edit

Edit and Cut are small panels floating over the map's corner: removals, the
filters and fills scroll in one narrow column, the tools are scattered chips,
and the two panels hide each other. The map, the timeline, the charts and the
workspace stay as they are; the editing moves out of the map.

- **Modes.** The top bar switches between *Browse*, *Edit* and *Route* (M6).
- **Inspector.** A column on the right, 300 px wide by default and dragged
  wider, replaces the floating panels. In Edit it has three tabs:
  - *Changes*: removals by hand and fills in one list, newest first; a row
    frames its points on the map and the timeline.
  - *Automatic cleaning*: the filters and their settings.
  - *Segments*: Cut as a table — name, time, distance, chosen — with writing
    into a new GPX or adding to another below it. Switching tabs keeps the
    tool in use.
- **Tool bar.** On the map's corner: select, range, lasso, cut, fill along the
  road, each with a key. The one in use is highlighted, and a line along the
  map's foot says what a click does and how to leave it.

Steps, each usable on its own: the mode bar and the inspector with the Edit
panel moved into it as two tabs, and the tool bar; then Cut as the Segments
tab. Both done: see *Cleaning* and *Segments*.

### M6 — Plan a route

A route drawn from nothing, not from a recording: a document of its own, with
no track to start from.

- Clicking the map adds a waypoint; waypoints are dragged to move them, and a
  handle on a leg inserts one. A click near the end of a track shown in the
  workspace snaps to it.
- Each leg between two waypoints follows the road — car, bicycle or foot,
  asked of OSRM — or is a straight line. Moving a waypoint asks again only for
  the legs either side of it.
- The inspector lists the waypoints with each leg's way and length, the total
  length, reverse and close the loop.
- Kept as a draft in memory, as a new GPX is now; saved as a new GPX with a
  `<trk>`, and optionally a `<rte>` of the waypoints. The waypoints and legs are
  kept in the sidecar, so the route opens to be changed again.
- Tracks in the workspace can be shown faintly as a reference to draw along.
- Times estimated from an average speed are optional; elevation needs a DEM
  source and is left for later.

Done: see *Planning a route*. Routing between two places off a track moved
here from filling, which again only fills a gap between two points of a
track. Not yet: showing tracks faintly as a reference, estimated times and
elevation.

### Later

Map matching to roads (HMM, Newson & Krumm 2009), 3D flyover, other source
formats such as FIT.
