# Photo Import TUI Design

## Scope

This document owns decisions specific to `dgs photo import`. Read [`../../tui.md`](../../tui.md), [`../../parameter-controls.md`](../../parameter-controls.md), and [`../../file-explorer.md`](../../file-explorer.md) first for shared shell and component behavior.

Photo Import is a concrete consumer of the shared design language. Its page composition, labels, parameter groups, defaults, and workflow do not define requirements for other apps or commands.

## Product scenario and integrity contract

The primary scenario is importing camera files from an SD card into durable storage. Data integrity is the highest-priority requirement: a destination file must not be published under its final photo or video filename until the bytes read from Source and the bytes independently read back from Destination produce the same cryptographic hash.

This contract describes integrity at publication time. It cannot promise that a file will never be corrupted after import; later bit rot and external modification require a separate audit feature.

Use SHA-256 by default because it is cryptographically strong, available in the Go standard library, and normally I/O-bound for this workload. Keep the algorithm identified in temporary state so a future version can add another algorithm without making old state ambiguous.

## Confirmed design summary

- Keep two main screens: Directories first, then the scanned Parameters workspace.
- Directories places Source and Destination on separate rows inside one fieldset and uses repository mock folders by default.
- A cancellable, read-only progress transition scans both directories before Parameters is shown.
- Parameters is an app-specific edge-to-edge split: Source and Destination inventories fill the left side; Parameters and a bottom-anchored Summary fill the right side.
- Shared DataField focus uses `Alt+h/j/k/l` and primary clicks. Child controls remain operable by keyboard and mouse.
- Source and Destination use the shared numbered scroll-list component, including full-row selection, Vim navigation, hover-wheel scrolling, Quick Look, and a reusable right-click menu.
- Extension filters are derived from Source, use dynamic multi-checkboxes plus `[ All ]`, and immediately filter Source and Summary while Destination remains unfiltered.
- The status bar shows the complete workflow and emphasizes the current step; the top breadcrumb shows the more detailed local state.
- The 100-cell preferred configuration width is shared, but this second-screen split and its `max(50 cells, 40%)` right pane are Photo Import decisions only.

## Confirmed workflow

The status workflow is:

1. Directories
2. Parameters
3. Processing
4. Result

The implemented interaction includes Directories, Parameters, a landscape Processing preview, and a landscape Result preview. A read-only Scan transition connects the first two screens. Processing advances simulated worker states only; no copy, hash, rename, state-file cleanup, or delete behavior is implemented yet.

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


                                                                   Next →  n
                                                                   › Parameters
```

The current composition is specific to Photo Import:

- Source and Destination occupy separate rows on the first screen.
- Do not connect the paths with an animated data-flow line.
- Setup focuses Source and Destination; the shared Page Actions component is fixed at the workspace bottom-right.
- `n` or the `Next → Parameters` action begins the same read-only scan transition.

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
2. Prominent title: `Choose directories`.
3. One concise explanatory sentence.

Use two rows of top breathing room on a comfortably sized Setup terminal, one on a medium-height terminal, and none when height is constrained. Leave additional space between the introduction and the first fieldset.

## Parameters

- Source: directory selected through File Explorer.
- Destination: directory selected through File Explorer.
- Operation: radio choice between Copy and Move.
- Extensions: dynamic multi-checkbox choices derived from the extensions actually present in Source; all are selected initially.
- Duplicates: filename-conflict policy. Skip leaves the existing destination untouched, Replace intends to publish the verified new file at that path, and Keep both chooses a new unique filename. Replace is not the default; its backup and rollback semantics remain to be designed before implementation.
- Parallelism: positive worker count controlling concurrent file transfers; default `1` for predictable removable-media I/O and bounded resource use.
- Organize by EXIF date: checkbox controlling whether imported files are placed in date-based subdirectories.
- `Next → Processing`: opens a simulated, side-effect-free Processing screen.

## Processing screen

Processing is designed for a landscape terminal: use width aggressively and keep vertical stacks shallow. A compact full-width header shows file progress, aggregate progress, active workers, pending files, and failures. Below it, a wide Worker activity list occupies roughly two thirds of the workspace; the remaining third is divided vertically between Next files and Recent results.

Each Worker owns one file and exposes a distinct state: Copying, Verifying, Publishing, or Idle. Each compact worker entry includes the current filename, file size, phase progress bar, and Source/Destination hash state. It also uses a stable three-step track—`COPY → VERIFY → PUBLISH`—where completed steps use `✓`, the active step uses `●`, and forthcoming steps use `○`; this makes both completed and remaining work visible without relying only on a phase label. Render only configured workers; do not create placeholder worker cards to fill a landscape viewport. When workers exceed the visible height, keep them in one vertically scrollable list instead of laying them out side by side. Up/Down or `k/j` scrolls that list, while `g`/Home and `G`/End jump to its beginning or end. Do not display `MATCH` until independent destination verification has completed. Overall completion counts verified and published files, not merely copied bytes.

The aggregate progress bar reserves a fixed right-hand status segment such as `42% │ 4/12 verified`. Calculate the bar width from the remaining cells after that segment, rather than stretching it to the fieldset width and truncating the right-hand information.

The preview supports 1–8 workers and defaults to 1. It simulates state transitions so layout and density can be evaluated, explicitly labels itself `NO FILES ARE CHANGED`, and never creates `.dgs-part` or `.dgs-state` files. `p` pauses or resumes the preview. During Processing, every key that could leave active work is guarded: `Esc` asks before returning to Parameters, and `q` or Ctrl+C asks before exiting. While the confirmation is open, worker scheduling is frozen. The shared dialog defaults to No; Tab switches actions, Enter invokes the selected action, and Esc continues processing.

Photo Import status uses the shared three-part bar as follows: the left chip identifies `SCAN · READING`, `PROCESSING · RUNNING`, `PROCESSING · PAUSED`, `PROCESSING · COMPLETE`, or confirmation state; the center keeps the workflow track; and the right side contains only context-valid controls. A paused import replaces `p Pause` with `p Resume`; a completed preview no longer advertises pause.

## Result screen

Processing remains visible when it reaches complete so its final worker state can be inspected. Enter or `n` then opens Result. Result uses a landscape summary rather than another progress surface:

```text
╭─ Import result ─────────────────────────────────────────────────────────────╮
│ VERIFIED PREVIEW · NO FILES WERE CHANGED                                   │
│ 41/41 files passed Source and Destination SHA-256 comparison               │
│ Published 2.4 GB   Skipped 0   Failed 0                                    │
╰────────────────────────────────────────────────────────────────────────────╯

╭─ Verified files ───────────────────────────╮ ╭─ Verification summary ──────╮
│ 41 verified files · 2.4 GB                 │ │ ✓ Source stream hashed       │
│ › 1 ✓ IMG_0001.JPG · 24 MB · SHA-256 MATCH │ │ ✓ Destination read back     │
│   2 ✓ IMG_0002.DNG  · 51 MB · SHA-256 MATCH│ │ ✓ SHA-256 matched            │
│                                             │ │ ✓ Published after verify     │
│                                             │ ╰──────────────────────────────╯
│                                             │ ╭─ State file ─────────────────╮
│                                             │ │ › [x] Delete .dgs-state      │
╰─────────────────────────────────────────────╯ ╰──────────────────────────────╯

                                      ← Prev  esc        Next →  n
                                      Processing          › Commands
```

The full-width header answers whether the batch is trustworthy before showing detail. The left pane owns the complete per-file result inventory and uses the shared numbered, selectable, vertically scrollable list. The right side separates integrity evidence from the final state-file decision. Do not mix cleanup controls into the verification summary.

Delete `.dgs-state` is selected by default, matching the confirmed lifecycle decision. The explanatory copy makes retaining it an audit or diagnostic choice. In the side-effect-free preview, toggling or finishing changes no files. `Alt+h/l` moves between the result inventory and state-file actions; list navigation uses the shared arrow/Vim behavior, mouse wheel works while hovering the inventory, and primary click changes focus or activates a control. `Esc` returns to the completed Processing view.

## Proposed verified-transfer algorithm

This section records the current implementation direction; transfer behavior is not implemented yet.

For each file:

1. Resolve the final destination and create a uniquely named temporary file in that same destination directory, for example `.IMG_0001.JPG.dgs-part`. A same-directory temporary file allows the final rename to be atomic on the destination filesystem.
2. Open Source once and stream it sequentially into the temporary file while updating the Source SHA-256 digest in the same pass. Do not perform a separate pre-hash pass over Source.
3. Flush and close the temporary file. Where supported, sync it before verification. The integrity guarantee is defined at the user-space/filesystem API boundary; it does not attempt to prove physical-media cache behavior.
4. Read the temporary file sequentially and compute the Destination SHA-256 digest. This second destination read is required by the integrity contract and is not considered redundant scanning.
5. Compare size and digest. On mismatch, keep the final filename absent and report a hard verification failure. A later retry restarts this individual file from byte zero.
6. On a match, rename the temporary file atomically to the final filename. Sync the containing directory where supported before reporting success. The presence of the correct final extension means verification succeeded at publication time.
7. For Move, delete that individual Source file immediately after its verified destination has been published successfully. A failure processing another file does not roll back already verified and moved files.

Normal successful operation therefore has the minimum strict-verification I/O profile: one sequential Source read, one Destination write, and one sequential Destination verification read. Hashing is performed during those required streams. Directory inventory and EXIF metadata collection must not reread complete file contents.

### Resume state

Resume operates at whole-file granularity, not byte or chunk granularity. Interrupted transfers use a versioned, atomically updated import-state file. It records the batch plan and which individual files have completed successfully. At minimum, record:

- Source identity: normalized path, size, modification time, and any available stable file identity.
- Destination temporary and final filenames.
- Hash algorithm and the verified Source/Destination digest for completed files.
- Intended final path and the parameters that affect it.
- Per-file state such as pending, transferring, verified, source-deleted, or failed.

Persist state only at file-state transitions so bookkeeping does not amplify data I/O. If a process stops while one `.dgs-part` is incomplete, that partial file is never resumed internally. On retry, discard or truncate it and transfer that photo again from byte zero. Already published and recorded files are not recopied.

Before skipping an item recorded as complete, confirm that the expected final destination still exists and matches the recorded identity or digest policy. Before retrying an incomplete item, confirm that Source still matches its recorded identity. Never rename a leftover partial file merely because its expected byte count has been reached.

The Result screen must offer an explicit choice to delete or retain the import-state file after the batch has reached its terminal state. Delete is the default. Retaining it supports auditing or later diagnosis; deleting it does not delete imported photos.

### Source and destination entry policy

Import regular files only. Directories are traversal structure rather than transfer items. Source symbolic links are skipped and reported because following one could read outside the selected SD-card root, and its target could change between scan and transfer. FIFOs, sockets, block or character devices, and other special entries are rejected because reading them can block, produce an unbounded stream, or access something other than the scanned file.

Destination path components and an existing final entry must be checked with non-following metadata operations. Do not write through a destination symbolic link. Create temporary files exclusively so an unrelated existing file cannot be silently reused.

If a regular Source file disappears after Scan, fail that item clearly and continue or stop according to the later batch-failure policy; never create an empty final file. If it changes while being copied, detect the change by comparing size, modification time, and available stable file identity before and after the stream. Discard the unverified temporary result and require a fresh transfer of that image.

### Parallelism

Implement parallelism as a bounded worker pool over files, not as multiple writers to one file. Default to one worker because SD cards and external disks often perform worse under competing reads and writes. Each worker owns at most one Source stream, one destination temporary file, and bounded copy buffers. The UI must expose active, verified, failed, and pending counts independently; completion means every published file passed verification.

Concurrency increases throughput only when the source and destination devices benefit from it. It does not relax ordering, temporary-file, sync, or verification rules.

### Storage topology

The transfer engine must behave correctly when Destination is a local USB volume, another disk on the same host, or a NAS mount. Always create `.dgs-part` in the final destination directory so publication never depends on a cross-filesystem rename. Treat rename and sync guarantees as those offered by the mounted filesystem and server at the user-space API boundary; do not claim stronger physical-media durability.

Use sequential streams and bounded buffers for every topology. Do not use direct I/O or a platform-specific cache bypass as a correctness requirement. A local same-device copy, a cross-device copy, and a network copy all receive the same independent destination readback and SHA-256 comparison.

Classify failures rather than collapsing them into verification errors: Source read, Destination write, sync/close, Destination readback, hash mismatch, atomic publish, Source delete, and transient network or mount loss are distinct outcomes. Preserve state after a NAS disconnect so a whole-file retry can resume the batch safely.

Parallelism remains an explicit user choice with default 1. Do not silently raise it based on CPU count. Multiple workers operate on different files; they never split one file into concurrent ranges. This avoids excessive seeks on SD cards or disks and avoids multiplying network requests on a NAS unless the user chooses that tradeoff.

### Confirmed failure behavior

- Disk-space preflight is not required. A write may fail when the destination becomes full; preserve batch state, report the affected file, and allow the user to free space and retry. The affected photo restarts from byte zero.
- Move commits per image: publish the verified destination, then immediately delete that Source image.
- Incomplete `.dgs-part` content is never promoted and is not byte-resumed.

### Remaining failure and conflict questions

The following decisions must be settled before implementation:

- How Replace publishes atomically when a final destination already exists and how the previous file remains recoverable.
- Whether an existing destination with the same size and SHA-256 is treated as already imported without rewriting it.
- How to identify a Source file reliably when SD-card timestamps are coarse or unreliable.
- Whether a leftover `.dgs-part` is removed immediately on failure or retained until the next retry for diagnosis.

Example status values:

```text
Left:    READY
Center:  Directories › Parameters › Processing › Result
Right:   arrows/hjkl  n Next  ↵ Select  ? Help
```

After the scan:

```text
Left:    PARAMETERS
Center:  Directories › Parameters › Processing › Result
Right:   click / alt+hjkl Focus  space Preview  right-click Menu
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

- Review layout and file grouping.
- Duplicate and conflict presentation.
- Real transfer progress and error recovery behavior beyond the Processing preview.
- Cancellation and confirmation.
- Partial-success, empty, warning, and error variants of Result.
- Narrow-terminal layout beyond shared shrinking behavior.
- Photo Encode screens.

Do not fill these gaps speculatively.
