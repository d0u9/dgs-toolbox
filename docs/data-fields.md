# Data Field Navigation

## Purpose

A data field is a major, semantically distinct workspace region such as a result inventory, parameter form, preview, or summary. Multi-region screens use the shared `internal/tui/datafield` navigator so movement follows spatial layout rather than declaration order.

## Keyboard convention

```text
Alt+h / Alt+Left     Focus the nearest data field to the left
Alt+j / Alt+Down     Focus the nearest data field below
Alt+k / Alt+Up       Focus the nearest data field above
Alt+l / Alt+Right    Focus the nearest data field to the right
```

Plain `h/j/k/l` and arrow keys remain owned by controls inside the active data field. Temporary selectors, text editors, and overlays retain their input precedence.

## Mouse convention

A primary-button click anywhere inside a data field's rectangular fieldset switches focus to that field. This includes its legend, border, padding, and content. Clicking the gap between fields leaves focus unchanged. When the hit cell belongs to an interactive child control or list row, the same click may also operate that child according to its component contract. Clicking only the fieldset border or padding changes field focus without triggering a child action.

Scrollable data fields may use hover-based wheel input. The field under the pointer receives the wheel event without becoming focused; focus remains an explicit keyboard-navigation or primary-click action.

The shared shell enables cell-motion mouse events and translates terminal coordinates into workspace coordinates before dispatching them. Each multi-region workspace registers the rendered bounds of its data fields with the shared navigator, so keyboard and mouse update the same focus state and presentation.

Because terminal mouse reporting is required for clicks, context menus, and wheel input, ordinary drag selection may be reserved by the terminal protocol. Components must ignore drag and release events and must not implement mouse capture. Users can use their terminal's mouse-reporting bypass modifier—commonly Shift or Option on macOS terminals—when selecting terminal text. A TUI cannot reliably restore the initial press to the terminal after deciding that a gesture became a long press.

## Focus presentation

Only one data field is active. A focused fieldset uses both a stronger accent border and a `›` prefix in its legend so focus remains understandable without color. Controls inside inactive data fields hide their internal focus marker. When focus enters a field containing controls, focus the first relevant control unless the command has a meaningful position to restore.

Screens register data fields with row and column positions for directional movement and rectangular bounds for mouse hit testing. Directional movement selects the nearest candidate in the requested direction. A direction with no candidate leaves focus unchanged.

The shell breadcrumb may append a command-owned workflow-state segment through the shared command path contributor. This keeps stage context such as `setup`, `scan`, or `review` visible without transferring top-bar ownership to the command.

Single-region screens and non-data workspaces such as command pickers do not need to handle these shortcuts.
