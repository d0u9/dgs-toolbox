# Photo Import TUI Design

## Scope

This document owns decisions specific to `dgs photo import`. Read [`../../tui.md`](../../tui.md), [`../../parameter-controls.md`](../../parameter-controls.md), and [`../../file-explorer.md`](../../file-explorer.md) first for shared shell and component behavior.

Photo Import is a concrete consumer of the shared design language. Its page composition, labels, parameter groups, defaults, and workflow do not define requirements for other apps or commands.

## Proposed flow

The proposed stages are:

1. Setup
2. Scan
3. Review
4. Import
5. Result

Setup, read-only Scan, Parameters, and a lightweight Review placeholder are currently demonstrated. No copy or move behavior is implemented.

## Setup screen

Photo Import uses a directory-only Setup screen. Scanning those directories opens a second screen containing the inventory and import parameters.

```text
  SETUP · PHOTO IMPORT
  Choose directories
  Select the source and destination to scan before configuring the import.

╭─ Directories ─────────────────────────────╮
│ › Source       …/testdata/photo-import/src│
│   Destination  …/testdata/photo-import/dst│
╰───────────────────────────────────────────╯

  [ Scan directories ]
```

The current composition is specific to Photo Import:

- Source and Destination occupy separate rows on the first screen.
- Do not connect the paths with an animated data-flow line.
- Setup focuses Source, Destination, and Scan directories.
- `n` or Scan directories begins the same read-only scan transition.

## Scan transition

After Setup, show a dedicated progress screen while recursively reading Source and Destination. The progress bar remains active during a long scan and the copy explains that no files are being changed. `Esc` cancels the UI transition and returns to Setup; a late result from that scan must be ignored.

The scan collects regular-file names and counts from both roots. It does not read EXIF metadata or perform photo-specific parsing yet.

## Parameters screen

After scanning, divide the entire workspace into two asymmetric panes:

- Left: separate Source and Destination inventories, consuming all width not used by the right pane.
- Right: Parameters above Import summary, flush with the terminal's right edge.
- Keep one terminal cell between panes.
- The right pane uses `max(50 cells, 40% of the workspace)`; 50 cells is its normal minimum.
- Below 63 total columns, replace the panes with a resize prompt instead of overflowing or breaking either fieldset.
- Source occupies the upper half of the full-height left pane and Destination occupies the lower half.
- A strong `━━━ ▼  ▼  ▼ ━━━` directional divider between them communicates Source-to-Destination flow. Keep two cells between arrows so the direction cue remains legible instead of becoming a dense glyph cluster.
- Import summary uses the natural fixed height of its content and is anchored to the bottom-right corner.
- Parameters starts at the top-right and expands through all space above the summary.

This screen does not use the centered 100-cell configuration surface. Its edge-to-edge composition is specific to Photo Import.

```text
╭─ Source ───────────────────────────╮ ╭─ Parameters ─────────────────────╮
│ 24 files · 420 B                   │ │ IMPORT                           │
│ › DCIM/100LEICA/IMG_0001.JPG       │ │ Operation  (●) Copy  ( ) Move    │
│ · DCIM/100LEICA/IMG_0002.JPG       │ │ Extensions [x] DNG               │
│ · DCIM/100LEICA/IMG_0003.DNG       │ │ Duplicates Skip                  │
╰────────────────────────────────────╯ │                                  │
━━━━━━━━━━━━━ ▼  ▼  ▼ ━━━━━━━━━━━━━━━━ │ ── ◆ ─────────────────────────── │
╭─ Destination ──────────────────────╮ │ CLASSIFICATION                   │
│ 11 files · 260 B                   │ │ [x] Organize by EXIF date        │
│ › Existing Album/IMG_0001.JPG      │ ╰──────────────────────────────────╯
│ · Existing Album/IMG_0002.JPG      │ ╭─ Import summary ─────────────────╮
│ · Existing Album/IMG_0999.JPG      │ │ Eligible       24 files          │
│                                    │ │ Duplicates      2 files          │
│                                    │ │ Skipped         2 files          │
│                                    │ │ Will copy      22 files          │
╰────────────────────────────────────╯ ╰──────────────────────────────────╯
```

The summary updates immediately when Operation, Extensions, Duplicates, or Classification changes. Extensions are discovered from the scanned Source inventory, normalized case-insensitively, sorted, and rendered as independent checkboxes; extensions absent from Source are not offered. All discovered extensions begin selected. A leading `[ All ]` action selects every discovered extension again, providing an explicit recovery from an empty filtered Source viewport. Right/l or Enter enters the extension subitems; Up/Down or k/j then moves through All and the extensions, Space or Enter invokes the current row, and Left/h or Esc exits the group. Duplicates opens with either Space or Enter. Changing the selection immediately filters the Source viewport and summary from the same source of truth. Destination remains complete so existing files and conflicts stay visible. Duplicate detection currently compares file basenames case-insensitively. Do not summarize a long inventory with text such as `… 8 files more`; make each inventory a navigable viewport instead. Source and Destination rows use the shared one-based line-number gutter. Within either list, `j/k` or Down/Up moves one file, `gg` moves to the first file, `G` moves to the last, and `h/l` or Left/Right pans long paths horizontally. The selected file uses `›` and the viewport follows it. A primary click selects a row and focuses its inventory. Hovering over either inventory and using the mouse wheel scrolls that inventory by three rows without changing focus. Space opens the selected file in macOS Quick Look. Right-clicking a row selects it and opens the shared context menu; Open with default app delegates to macOS `open`. `Alt+h/j/k/l` moves spatially among Source, Destination, Parameters, and Import summary. `Esc` returns to Setup. The shared `n` Next Screen shortcut invokes Review import without requiring traversal through unchanged parameters.

The shared status-bar stepper shows `Directories › Parameters › Processing › Result`. The active stage is bold and underlined. Scan and the parameter-selection workspace both belong to Parameters until the processing stage is implemented.

The shell breadcrumb includes workflow state: Setup uses `dgs › photo › import › setup`; scanning and the inventory/parameter screen use `dgs › photo › import › scan`; Review uses `dgs › photo › import › review`.

## Introduction region

The Setup workspace begins with a separate introduction region:

1. Quiet eyebrow: `SETUP · PHOTO IMPORT`.
2. Prominent title: `Configure import`.
3. One concise explanatory sentence.

Use two rows of top breathing room on a comfortably sized Setup terminal, one on a medium-height terminal, and none when height is constrained. Leave additional space between the introduction and the first fieldset.

## Parameters

- Source: directory selected through File Explorer.
- Destination: directory selected through File Explorer.
- Operation: radio choice between Copy and Move.
- Extensions: dynamic multi-checkbox choices derived from the extensions actually present in Source; all are selected initially.
- Duplicates: option with Skip, Replace, and Keep both.
- Organize by EXIF date: checkbox controlling whether imported files are placed in date-based subdirectories.
- Review import: preview-only action during the demo.

Example status values:

```text
Left:    READY
Center:  Choose directories to scan
Right:   arrows/hjkl  n Next  ↵ Select  ? Help
```

After the scan:

```text
Left:    PARAMETERS
Center:  4 source · 2 destination
Right:   arrows/hjkl  n Next  ↵ Select  esc Back
```

Selector state uses `SELECT` and text editing uses `EDIT`. The Review placeholder uses `REVIEW`, explicitly states that no files changed, and offers `Esc Back`.

## Demo data

For convenient UI testing, the default paths resolve to these repository-owned mock directories:

```text
testdata/photo-import/src
testdata/photo-import/dst
```

Resolve them to absolute paths at startup so File Explorer can open them directly. Their files are placeholders only; they do not imply implemented photo parsing or import behavior.

The fixture intentionally contains multiple screens of files in both Source and Destination to exercise viewport scrolling, jump navigation, nested folders, mixed extensions, existing destination content, and duplicate names. Keep placeholder contents small; the directory structure and names are the test data.

## Still undesigned

- Scan progress.
- Review layout and file grouping.
- Duplicate and conflict presentation.
- Import progress.
- Cancellation and confirmation.
- Result, empty, warning, and error states.
- Narrow-terminal layout beyond shared shrinking behavior.
- Photo Encode screens.

Do not fill these gaps speculatively.
