# AGENTS.md

## Current goal

- **Photo Import** — the real, integrity-verified Processing engine. Real file
  operations only after the user starts Processing. Highest-priority contract: a
  destination file is not published under its final name until an independent
  destination readback matches the Source SHA-256 digest. Follow
  [`docs/apps/photo/import.md`](docs/apps/photo/import.md). Other Photo commands
  stay demos.
- **GPX** — milestone by milestone per [`docs/apps/geo/gpx.md`](docs/apps/geo/gpx.md).
- **`dgs cred`** — age identities, recipients, encrypted vault, per
  [`docs/apps/cred/`](docs/apps/cred/). Decrypted content stays in memory unless
  an Action the user confirms writes it. A vault file is replaced only after the
  new one decrypts back to the same SHA-256.

## Documents

- [`docs/tui.md`](docs/tui.md) holds shared interface decisions. Read it before
  changing any TUI or interactive flow, and follow its links to the component
  being changed — [`file-explorer.md`](docs/file-explorer.md),
  [`parameter-controls.md`](docs/parameter-controls.md),
  [`data-fields.md`](docs/data-fields.md).
- App designs live under `docs/apps/<app>/` and must not be promoted into shared
  requirements.
- Update a design document only when a decision is confirmed. Never fill an
  undecided section speculatively.

## Configuration and catalogues

Every setting `dgs` reads is documented in
[`docs/configuration/`](docs/configuration/), one document per part, indexed by
[`index.md`](docs/configuration/index.md). It is a reference — name, meaning,
default — not a design document; designs explain why a setting exists and link
here.

Adding, renaming, removing a key, or changing its default, is not finished until
the part's document **and** the index table say so in the same change. A key in
one and not the other is worse than a key in neither: the index is what a reader
trusts to be complete.

[`examples/`](examples) holds a working configuration per feature and a test
loads every one. Fixing examples a key change breaks is part of that change.

The same rule covers two catalogues, each pinned by a test that fails when the
document does not name every registered Action:

- `dgs cred` Actions — [`docs/apps/cred/actions.md`](docs/apps/cred/actions.md).
- Capture Actions — [`docs/apps/capture/actions.md`](docs/apps/capture/actions.md).
  It is what someone writes a Recipe against. The test cannot check that a
  description is still true; that part is on the change.

## Reusable algorithms

The same computation is wanted in more than one place, so every algorithm lives
in its own package, apart from whatever shows its result:

- Domain packages such as `internal/geo`, `internal/geo/gpxfile`,
  `internal/geo/track` — one concern per package, named for what it computes.
- They import no TUI, HTTP, configuration or app code: plain values in, plain
  values out, so a command, a web handler, a report and a test can all call them.
- Parameters (a window, a threshold) are arguments with a named, documented
  default, never constants in a caller.
- Each has tests in its own package pinning behaviour on small constructed inputs.
- Apps and web handlers compose algorithms, never reimplement them. A web page
  draws what the server computed; only on-screen interaction geometry is its own.
- Look for an algorithm to reuse or generalise before writing one. Move an
  app-local helper into a shared package rather than copying it.

## Technology

Go, Cobra, Bubble Tea. Bubbles and Lip Gloss only when useful.

## One binary

`dgs` is the only executable installed, and this is hard: copying that one file
to another machine must be enough to run it. No helper binary, sidecar or script
beside `dgs` or on `PATH` — not as an option either.

Compile in what Go cannot reach directly. Apple Reminders, for example, reach
EventKit through cgo under a `darwin && cgo` tag; other platforms and
`CGO_ENABLED=0` get a fallback that refuses with a clear reason. If a feature
cannot be built this way, ask before designing around it.

## Command model

Commands are hierarchical: `dgs`, then `demo`, `capture`, `photo` (`import`,
`encode`), `geo` (`gpx`), `cred` (`keys`, `vault`), `conf`.

One `dgs` process has exactly one active leaf command. It never runs or displays
two command workspaces at once.

- A leaf — `dgs demo`, `dgs photo import` — opens its TUI directly.
- An incomplete command — `dgs`, `dgs photo` — opens a picker scoped to the
  choices available; selecting replaces the picker with that command's TUI.
- Leaving a command returns to the picker. Starting another builds a fresh
  model; the previous command does not stay alive in the background.
- `Esc` at a command's root returns to the picker; `Esc` in the picker exits.

## Shared TUI shell

Three vertical regions: a top bar of exactly one row, the workspace, a status bar
of exactly one row. Both bars stay one row — on narrow terminals shorten or omit
content, never wrap.

- Top bar: left, the command path or picker context; right, local time.
- Status bar: left, state; center, contextual summary; right, key hints.
- Workspace: the picker's or the active command's. A demo shows only a large
  label such as `PHOTO IMPORT`.

The shell owns the single top-level Bubble Tea program, command selection,
picker/command switching, both bars, window sizing and command lifecycle. A leaf
command owns its workspace model and rendering, the status values and key hints
it contributes, and its internal interaction. Leaf commands expose Bubble Tea
models and never create a nested `tea.Program`.

## Scope

Implement Photo Import Processing with bounded worker concurrency, same-directory
`.dgs-part` files, SHA-256 source hashing during copy, independent destination
readback, verified atomic publication, whole-file retry, and a versioned
`.dgs-state` file. Keep it outside the TUI model and cover it with filesystem
tests.

Do not implement real Photo Encode behavior, GPX beyond the confirmed milestones,
databases, plugin loading, or speculative shared infrastructure. Do not claim
stronger durability than the user-space/filesystem API boundary documented for
Photo Import.
