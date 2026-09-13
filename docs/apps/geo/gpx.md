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

The page and its assets are embedded in the binary, so the page cannot drift
from the build that serves it. The server exposes the API under `/api/`;
everything else is the page.

| Path | Purpose |
| --- | --- |
| `/` | The web page. |
| `/api/health` | Answers `{"ok":true}`; the page uses it to show whether it reached the server. |

## Current state

Both surfaces are empty foundations. The TUI shows the page's URL, or why the
server failed to start. The page shows only whether it reached the server.

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
everyone on the network.

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
- **Only GPX files** are read, for now.
- **Export** writes one GPX file holding one `<trk>` per segment, into a folder
  the user chooses.
- **The sidecar** is a small JSON file stored next to the source GPX. It holds
  no track data, only what was done: cleaning parameters, points removed by
  hand, cuts, segment names and gap fills. Opening a GPX that has one resumes
  the work.

### Map and coordinates

- The page uses MapLibre GL. OpenStreetMap is built in and always available.
  Further tile sources are listed in the configuration, each with a name and a
  URL template; the page offers every one of them, alongside OpenStreetMap, to
  choose from.
- Data is always WGS-84, as GPX is. A **GCJ-02 toggle** converts what is drawn —
  track and stops — for base maps that use China's GCJ-02 system. It changes
  display only: cleaning, segmentation and export stay WGS-84. The conversion
  is the widely published reverse-engineered one (accurate to about 1–2 m);
  points outside China are not offset.

### M1 — Open a GPX and show where you stopped

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
