# AGENTS.md

## Current goal

Build the Photo Import workflow for `dgs`, including its real, integrity-verified Processing engine. Keep the remaining Photo commands as demo domains and GPX as an empty foundation.

Photo Import may perform real file operations only after the user starts Processing. Its highest-priority contract is that a destination file is not published under its final name until an independent destination readback matches the Source SHA-256 digest. Follow the confirmed algorithm and unresolved boundaries in [`docs/apps/photo/import.md`](docs/apps/photo/import.md).

Shared interface decisions live in [`docs/tui.md`](docs/tui.md). Read it before changing any TUI or interactive command flow. Component-specific documents are linked from that shared design, including [`docs/file-explorer.md`](docs/file-explorer.md), [`docs/parameter-controls.md`](docs/parameter-controls.md), and [`docs/data-fields.md`](docs/data-fields.md); follow the links relevant to the component being changed. App-specific designs live under `docs/apps/<app>/`, including [`docs/apps/photo/import.md`](docs/apps/photo/import.md), and must not be promoted into shared requirements. Update design documents incrementally only when a decision is confirmed, and do not fill undecided sections speculatively.

## Configuration

Every setting `dgs` reads is documented in [`docs/configuration/`](docs/configuration/), one document per part of the toolbox with an index at [`docs/configuration/index.md`](docs/configuration/index.md). It is a reference, not a design document: what a key is called, what it means, and what it defaults to.

Adding a configuration file, or a key to one, is not finished until that reference says so in the same change. Update the part's document and the index table together — a key in one and not the other is worse than a key in neither, because the index is what a reader trusts to be complete. Renaming, removing, or changing the default of a key is the same change. Design documents keep explaining why a setting exists and link here for what to write, rather than repeating the list and drifting from it.

[`examples/`](examples) holds a working configuration per feature, and a test loads every one of them: renaming or removing a key breaks the examples that use it, and fixing them is part of the same change rather than something a reader discovers by copying one.

Capture's Actions are the same rule in the other direction: adding one, renaming one, or changing what one writes or requires is not finished until [`docs/apps/capture/actions.md`](docs/apps/capture/actions.md) says so in the same change. It is the catalogue someone writes a Recipe against, and an Action it does not name is one they have no way to know about. A test fails when the document does not name every registered Action; it cannot check that the description is still true, so that part is on the change.

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
dgs geo
dgs geo gpx
```

A single `dgs` process has only one active leaf command at a time. It does not display or run multiple command workspaces simultaneously.

Startup behavior:

- A direct root leaf such as `dgs demo` opens its TUI directly.
- A leaf command such as `dgs photo import` opens its TUI directly.
- An incomplete command such as `dgs` or `dgs photo` opens a command picker scoped to the available choices.
- Selecting a command replaces the picker with that command's TUI.
- Leaving a command returns to the command picker so the user can choose again.
- Starting another command creates a fresh command model; the previous command does not remain active in the background.

Photo commands other than Photo Import remain mock demonstrations of this model. GPX is an empty foundation: a TUI plus a local web server for map work, described in [`docs/apps/geo/gpx.md`](docs/apps/geo/gpx.md).

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

The workspace belongs to the picker or the active command. For command demos, show only a large identifying label such as `PHOTO IMPORT` or `PHOTO ENCODE`.

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

## Implementation scope

Continue to demonstrate:

- The Cobra command hierarchy.
- Global and domain-scoped command pickers.
- Direct entry into a leaf command.
- Returning from a command to the picker.
- The shared three-region layout.
- Photo placeholder command screens and the empty GPX command.

Implement Photo Import Processing with bounded worker concurrency, same-directory `.dgs-part` files, SHA-256 source hashing during copy, independent destination readback, verified atomic publication, whole-file retry, and a versioned `.dgs-state` file. Keep this logic outside the TUI model and cover it with filesystem tests.

Do not implement real Photo Encode or GPX behavior, databases, plugin loading, or speculative shared infrastructure. Do not claim stronger durability than the user-space/filesystem API boundary documented for Photo Import.
