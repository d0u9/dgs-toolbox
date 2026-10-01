package geo

const gpxHelp = `Browse GPX tracks in the terminal and a local map page.

Examples:
  dgs geo gpx --dir /path/to/tracks
  dgs geo gpx --port 9000
  dgs geo gpx --host 0.0.0.0 --port 9000

--dir overrides the initial browsing folder (default home directory).
The server uses geo configuration; built-in host/port defaults are
127.0.0.1:8765. Binding 0.0.0.0 allows other hosts to reach it.
The terminal workspace shows the map address and track controls.

A GPX dgs did not write is never rewritten. Its edits go into the companion
sidecar. To put edited tracks inside a GPX, save a new file; existing recordings
are kept. GPX files created by dgs may hold their edit state internally.`
