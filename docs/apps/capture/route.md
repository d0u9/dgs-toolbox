# Capture Route TUI Design

## Confirmed scope

- `Route` is the second Capture session, rendered as a `ROUTE` tab beside
  `SCAN` in the shared top bar. Both tabs belong to one Capture command; only
  one is visible at a time.
- `[` and `]` move between Capture tabs, and a primary click on a tab in the
  top bar activates it. The keys are ignored while the active session owns
  them, so the Scan file-explorer overlay and any text input keep their
  meaning. Capture does not bind `Ctrl+←`/`Ctrl+→`; macOS reserves those for
  switching Desktops. The remaining global bindings keep their
  usual meaning: `Tab`/`Shift+Tab` cycle selectable controls inside the active
  session and `Alt+Arrow`/`Alt+h j k l` move between DataFields.
- Both sessions share one Capture root, one configured index filename, and one
  scan result. A single load feeds Scan and Route, so `R` in either session
  refreshes both.
- Route purpose: assign each scanned Capture to one configured destination.
  This session records the plan only; nothing is copied, moved, or deleted.

## Layout

Route reuses the shared three-column landscape skeleton with the same widths as
Scan: left `1/4`, center `1/2`, right `1/4`.

- Left `CAPTURES` is a full-height flat list of the valid Captures under the
  current root — no expansion and no attachment children, because routing acts
  on the Capture as a whole. Unassigned rows use `○`, assigned rows use `●`
  followed by ` → <destination name>`. Folder labels keep a trailing path
  separator.
- Center `CAPTURE` is a scrollable summary of the selected Capture:
  `Capture`, `ID`, `Created`, `Workflow`, `Files`, `Attachments`, then an indented
  `Route` group holding the current `Destination` or `· Unassigned`. It uses the
  same fixed 12-cell key column as Capture Info so values never shift as the
  selection changes.
- Right top `DESTINATIONS` lists the configured destinations as `name` plus the
  display path. With no configuration it shows
  `· Configure capture.route.destinations`.
- Right bottom `ROUTE PLAN` shows one row per destination with its assigned
  Capture count, an anchored divider, and the `Unassigned` count.

## Keyboard behavior

- `CAPTURES`: `↑/k` and `↓/j` move, `h/l` pan, `g g`/`G` jump to the first and
  last Capture, `Enter` moves focus to `DESTINATIONS` to choose a target, and
  `u` or `Backspace` clears the current assignment.
- `DESTINATIONS`: `↑/k` and `↓/j` move, `Enter` assigns the focused destination
  to the current Capture, then advances the Capture cursor and returns focus to
  `CAPTURES` so a run of Captures can be routed without leaving the keyboard.
  `Esc` returns to `CAPTURES` without assigning.
- `CAPTURE` and `ROUTE PLAN` are scroll-only DataFields.
- `R` refreshes the Capture list from any DataField.
- `Alt+J`/`Alt+K` cycle `Captures → Capture → Destinations → Route Plan`;
  `Alt+K` walks the same order in reverse. Horizontal Alt navigation remains
  spatial.
- The status bar center shows the selected Capture path, plus ` → <path>` when
  it is routed; with nothing selected it summarizes
  `<routed>/<total> ROUTED · <n> DESTINATIONS`.

## Configuration

```json
{
  "capture": {
    "route": {
      "destinations": [
        { "name": "Notes", "path": "~/Captures/notes" }
      ]
    }
  }
}
```

- A destination without a `path` is ignored; a destination without a `name` is
  labelled by the base name of its path.
- Route has no destinations by default, so the session opens with an empty
  `DESTINATIONS` field until it is configured.
