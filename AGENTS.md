# AGENTS.md

## Current goal

Build a small Go demo to settle the CLI and TUI structure for `dgs`.

Use Photo and GPX as demo domains. Do not implement real photo or GPX behavior yet.

Shared interface decisions live in [`docs/tui.md`](docs/tui.md). Read it before changing any TUI or interactive command flow. Component-specific documents are linked from that shared design, including [`docs/file-explorer.md`](docs/file-explorer.md), [`docs/parameter-controls.md`](docs/parameter-controls.md), and [`docs/data-fields.md`](docs/data-fields.md); follow the links relevant to the component being changed. App-specific designs live under `docs/apps/<app>/`, including [`docs/apps/photo/import.md`](docs/apps/photo/import.md), and must not be promoted into shared requirements. Update design documents incrementally only when a decision is confirmed, and do not fill undecided sections speculatively.

## Technology

- Go
- Cobra
- Bubble Tea
- Bubbles and Lip Gloss only when useful

## Command model

Commands are hierarchical:

```text
dgs
dgs demo
dgs photo
dgs photo import
dgs photo encode
dgs gpx
dgs gpx import
dgs gpx inspect
```

A single `dgs` process has only one active leaf command at a time. It does not display or run multiple command workspaces simultaneously.

Startup behavior:

- A direct root leaf such as `dgs demo` opens its TUI directly.
- A leaf command such as `dgs photo import` opens its TUI directly.
- An incomplete command such as `dgs` or `dgs photo` opens a command picker scoped to the available choices.
- Selecting a command replaces the picker with that command's TUI.
- Leaving a command returns to the command picker so the user can choose again.
- Starting another command creates a fresh command model; the previous command does not remain active in the background.

Photo and GPX are only mock demonstrations of this model.

## Shared TUI shell

Every picker and command screen uses three vertical regions:

```text
Top bar        exactly one terminal row
Workspace      all remaining rows
Status bar     exactly one terminal row
```

The top bar has two regions:

- Left: the current command path or picker context.
- Right: the current local time.

The workspace belongs to the picker or the active command. For command demos, show only a large identifying label such as `PHOTO IMPORT` or `GPX INSPECT`.

The status bar has three regions:

- Left: command or application state.
- Center: contextual summary.
- Right: contextual key hints.

The top and bottom bars must remain one row. On narrow terminals, shorten or omit content instead of wrapping.

## Shell and command boundary

The shell owns:

- The single top-level Bubble Tea program.
- Command selection.
- Switching between the picker and one active command.
- The top bar and status bar layout.
- Window sizing and command lifecycle.

Each leaf command owns:

- Its workspace model and rendering.
- The status values and key hints it contributes.
- Its internal interaction.

Leaf commands expose Bubble Tea models/components and do not create nested `tea.Program` instances.

At a command's root, `Esc` returns to the command picker. In the picker, `Esc` exits. More detailed navigation and cancellation behavior will be designed later.

## Demo scope

Implement only enough to demonstrate:

- The Cobra command hierarchy.
- Global and domain-scoped command pickers.
- Direct entry into a leaf command.
- Returning from a command to the picker.
- The shared three-region layout.
- Photo and GPX placeholder command screens.

Do not implement real importing, encoding, file operations, databases, persistence, background tasks, plugin loading, or speculative shared infrastructure.

Stop after the demo works so the interaction design can be reviewed before adding more rules.
