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
- Directories uses the shared centered one-column skeleton. Scan, Processing, and Result use the shared two-column landscape skeleton. Parameters uses a four-column landscape composition whose middle two columns equally divide the flexible width.
- In the two-column screens, the narrow column begins at one third of the workspace and is capped at 90 cells; the wide column receives the remaining space. Scan puts the narrow column on the left. Processing and Result put it on the right, with Page Actions at its bottom, so the primary content reads first.
- Shared DataField focus uses `Alt+h/j/k/l` and primary clicks. Child controls remain operable by keyboard and mouse.
- Source, Destination, JPG-only, and RAW-only inventories use the shared numbered scroll-list component, including full-row selection, Vim navigation, hover-wheel scrolling, Quick Look, and a reusable right-click menu.
- Extension filters are derived from Source, use dynamic multi-checkboxes plus `[ All ]`, and immediately filter Source and Summary while Destination remains unfiltered.
- The status bar shows the complete workflow and emphasizes the current step; the top breadcrumb shows the more detailed local state.
- Scan repeats the selected endpoints so an incorrect SD-card or storage path can be noticed before parameters are accepted.
- Processing uses bounded whole-file workers and exposes Copy, Verify, and Publish separately. A final photo name is never visible before independent SHA-256 readback succeeds; Source modification time is applied before publication.
- Optional Post-processing organizes verified JPG/RAW files into `YYYYMMDD` directories. It never replaces Processing verification and runs only when explicitly confirmed on the Post-processing page.
- Every quit path uses the shared safe-default confirmation dialog. Active Processing pauses while it is open and cancels cleanly only after confirmation.
- Result is terminal: it summarizes Source, Destination, verified published bytes, skipped files, and failures, then offers only Again or Quit—not navigation back into completed work.
- The internal subdivisions remain Photo Import decisions rather than shared layout rules.

## Confirmed workflow

The status workflow is:

1. Directories
2. Parameters
3. Processing
4. Post-processing
5. Result

The implemented interaction includes Directories, Parameters, real integrity-verified Processing, optional capture-date Post-processing, and a landscape Result screen. A read-only Scan transition connects the first two screens. Processing performs file operations only after the user invokes Next from Parameters.

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

- Source and Destination occupy separate rows on the first screen. Their shared fieldset is compact, adds one row of internal vertical padding, and the complete Setup composition is vertically centered above the bottom actions.
- Do not connect the paths with an animated data-flow line.
- Setup focuses Source and Destination; the shared Page Actions component is fixed at the workspace bottom-right.
- `n` or the `Next → Parameters` action begins the same read-only scan transition.

## Scan transition

After Setup, show a dedicated progress screen while recursively reading Source and Destination. The progress bar remains active during a long scan and the copy explains that no files are being changed. `Esc` cancels the UI transition and returns to Setup; a late result from that scan must be ignored.

Parameters offers `r` Refresh: it re-reads both roots in place, keeps focus and extension choices, and selects only extensions that appeared since the last scan. Use it after changing either folder outside the tool.

The scan collects regular-file names and counts from both roots without reading complete file contents. It uses the two-column skeleton: the left column repeats the exact Source and Destination paths selected on Directories, while the wider right column owns scan state and progress. This allows the user to verify the transfer endpoints while scanning is underway. Long paths truncate within the left column; they never wrap the screen.

## Parameters screen

After scanning, divide the entire workspace into four columns:

- First: Parameters above Import summary. Parameters keeps the existing controls and takes their natural height; Import summary fills the remaining height below it.
- Second: the full-height Source inventory.
- Third: the full-height Destination inventory.
- Fourth: `JPG only · no RAW`, then `RAW only · no JPG`, then Page Actions. The two inventory DataFields equally divide the height remaining above Page Actions.
- The first and fourth columns are supporting widths. The middle two columns freely and equally divide all remaining width after the outer columns and three one-cell gutters are reserved.
- Page Actions keeps only its shared four-row height at bottom-right, including its breathing space and right-edge inset. It does not shorten the other three columns.
- Below 140 total columns, replace the columns with a resize prompt instead of overflowing or wrapping structural regions.
- Each inventory displays its root directory before its count and relative file list.
- The JPG-only and RAW-only inventories classify scanned Source files by matching a case-insensitive filename stem within the same relative directory. This classification reads filenames only; EXIF capture-time behavior is not part of this layout change.

This screen does not use the centered 100-cell configuration surface. Its composition is specific to Photo Import.

```text
╭─ Parameters ─────╮ ╭─ Source files ────╮ ╭─ Destination files ─╮ ╭─ JPG only · no RAW ─╮
│ Operation Copy   │ │ Root /Volumes/SD   │ │ Root /Volumes/Photos│ │ IMG_0003.JPG        │
│ Extensions …     │ │ IMG_0001.JPG       │ │ Existing/IMG_0912…  │ ╰─────────────────────╯
│ Duplicates Skip  │ │ IMG_0001.DNG       │ │                     │ ╭─ RAW only · no JPG ─╮
│ Parallelism 1    │ │ …                  │ │                     │ │ IMG_0004.DNG        │
╰──────────────────╯ │                    │ │                     │ ╰─────────────────────╯
╭─ Import summary ╮ │                    │ │                     │   ← Prev      Next →
│ Eligible 24     │ │                    │ │                     │   Directories Processing
│ Will copy 22    │ │                    │ │                     │
│ Workers 1       │ │                    │ │                     │
╰──────────────────╯ ╰────────────────────╯ ╰─────────────────────╯
```

The summary updates immediately when Operation, Extensions, Duplicates, or Parallelism changes. Extensions are discovered from the scanned Source inventory, normalized case-insensitively, sorted, and rendered as independent checkboxes; extensions absent from Source are not offered. All discovered extensions begin selected. A leading `[ All ]` action selects every discovered extension again, providing an explicit recovery from an empty filtered Source viewport. Right/l or Enter enters the extension subitems; Up/Down or k/j then moves through All and the extensions, Space or Enter invokes the current row, and Left/h or Esc exits the group. Duplicates opens with either Space or Enter. Changing the selection immediately filters Source, the two sidecar exception inventories, and Summary from the same source of truth. Destination remains complete so existing files and conflicts stay visible. Duplicate detection compares the relative path under each root case-insensitively, matching the destination layout resolved by the engine. Do not summarize a long inventory with text such as `… 8 files more`; make each inventory a navigable viewport instead. All four inventories use the shared one-based line-number gutter. Within a focused list, `j/k` or Down/Up moves one file, `gg` moves to the first file, `G` moves to the last, and `h/l` or Left/Right pans long paths horizontally. The selected file uses `›` and the viewport follows it. A primary click selects a row and focuses its inventory. Hovering over an inventory and using the mouse wheel scrolls that inventory by three rows without changing focus. Space opens the selected file in macOS Quick Look. Right-clicking a row selects it and opens the shared context menu; Open with default app delegates to macOS `open`. `Alt+h/j/k/l` moves spatially among Parameters, Import summary, and all four inventories. `Esc` returns to Setup. The shared `n` Next Screen shortcut starts Processing without requiring traversal through unchanged parameters.

The shared status-bar stepper shows `Directories › Parameters › Processing › Post-processing › Result`. The active stage is bold without an underline. Scan and the parameter-selection workspace both belong to Parameters.

The shell breadcrumb includes workflow state: Directories uses `dgs › photo › import › setup`; scanning and the inventory/parameter screen use `dgs › photo › import › scan`; active transfer uses `dgs › photo › import › processing`; and the terminal summary uses `dgs › photo › import › result`.

## Introduction region

The Setup workspace begins with a separate introduction region:

1. Quiet eyebrow: `SETUP · PHOTO IMPORT`.
2. Prominent title: `Choose directories`.
3. One concise explanatory sentence.

Use two rows of top breathing room on a comfortably sized Setup terminal, one on a medium-height terminal, and none when height is constrained. Leave additional space between the introduction and the first fieldset.

## Parameters

- Source: directory selected through File Explorer.
- Destination: directory selected through File Explorer.
- `photo.import.source` and `photo.import.destination` in `dgs-config.json` optionally prefill those two fields. Empty values retain the repository mock directories for UI testing; users can still change either path in Directories.
- Operation: radio choice between Copy and Move.
- Extensions: dynamic multi-checkbox choices derived from the extensions actually present in Source; all are selected initially.
- Duplicates: filename-conflict policy. Skip leaves the existing destination untouched, Replace intends to publish the verified new file at that path, and Keep both chooses a new unique filename. Replace is not the default; its backup and rollback semantics remain to be designed before implementation.
- Parallelism: positive worker count controlling concurrent file transfers; default `1` for predictable removable-media I/O and bounded resource use.
- `Next → Processing`: starts the selected transfers. Replace remains unavailable until its backup and rollback contract is implemented; the screen reports this without starting file operations.

## Processing screen

Processing uses the shared two-column landscape skeleton and keeps vertical stacks shallow. A compact full-width header shows file progress, aggregate progress, active workers, pending files, and failures. Below it, the narrow right column is divided vertically between Next files and Recent results, with Page Actions at bottom-right. The wider left column contains the worker list so filenames, metadata, step tracks, hashes, and progress bars receive the available width.

Each Worker owns one file and exposes a distinct state: Copying, Verifying, Publishing, or Idle. Each compact worker entry includes the current filename, file size, phase progress bar, and Source/Destination hash state. It also uses a stable three-step track—`COPY → VERIFY → PUBLISH`—where completed steps use `✓`, the active step uses `●`, and forthcoming steps use `○`; this makes both completed and remaining work visible without relying only on a phase label. Render only configured workers; do not create placeholder worker cards to fill a landscape viewport. When workers exceed the visible height, keep them in one vertically scrollable list instead of laying them out side by side. Up/Down or `k/j` scrolls that list, while `g`/Home and `G`/End jump to its beginning or end. Do not display `MATCH` until independent destination verification has completed. Overall completion counts verified and published files, not merely copied bytes.

The aggregate progress bar reserves a fixed right-hand status segment such as `42% │ 4/12 verified`. Calculate the bar width from the remaining cells after that segment, rather than stretching it to the fieldset width and truncating the right-hand information.

Processing supports 1–8 workers and defaults to 1. Each worker handles a different whole file and reports real engine events. It writes same-directory `.dgs-part` files and persists `.dgs-state` at file transitions. `p` pauses or resumes I/O between bounded chunks. During active Processing, every key that could leave work is guarded: `Esc` asks before returning to Parameters, and `q` or Ctrl+C asks before exiting. Opening the confirmation pauses I/O; choosing No resumes it unless it was already paused. Choosing Yes cancels the batch and waits for workers to stop before navigation or exit. The shared dialog defaults to No; Tab switches actions, Enter invokes the selected action, and Esc continues processing.

Photo Import status uses the shared three-part bar as follows: the left chip identifies `SCAN · READING`, `PROCESSING · RUNNING`, `PROCESSING · PAUSED`, `PROCESSING · COMPLETE`, or confirmation state; the center keeps the workflow track; and the right side contains only context-valid controls. A paused import replaces `p Pause` with `p Resume`; a completed import no longer advertises pause.

Processing remains visible when it reaches complete so its final worker state can be inspected. Enter or `n` opens Post-processing, whose confirmation defaults to `Skip`.

## Post-processing screen

Post-processing receives only files that Processing reported complete after independent destination readback and publication. Its page first shows a confirmation dialog. `Skip` is the safe default and performs no metadata reads or renames before continuing to Result. Choosing `Organize by capture date` closes the dialog and applies these rules:

1. A RAW with a same-directory, case-insensitive filename-stem JPG/JPEG sidecar and that JPG/JPEG both use the sidecar's EXIF capture time.
2. A JPG/JPEG without RAW uses its own EXIF capture time.
3. A RAW without JPG/JPEG uses capture time read directly from the RAW through the existing Go EXIF library.

The destination is `<Destination>/YYYYMMDD/<filename>`. Post-processing reads the already verified destination file, not a deleted Move source. It uses same-filesystem rename and never overwrites an existing target. A missing/invalid capture time, unsupported RAW container, unsafe date directory, target conflict, or rename failure leaves that file at its published Processing path and reports a per-file failure. Non-photo files are skipped. Before moving any paired RAW, read its sidecar date so processing order cannot invalidate the metadata source.

Organizing runs in two steps. It first reads every capture date without moving anything. If any planned `YYYYMMDD` folder already exists under Destination, a second dialog lists those folders and asks to `Continue` or `Skip`, so a batch never silently mixes into earlier folders. The same check backs `dgs photo organize <folder>`, which organizes the files directly inside one folder and asks `[y/N]` on the terminal. Its flags: `-y/--yes` skips that question, `-n/--dry-run` prints each planned move and changes nothing, `-r/--recursive` includes subfolders (hidden ones skipped; photos already in their date folder are left alone), `--format` sets the folder layout from `YYYY`, `MM`, `DD`, `-`, `_`, `.` and `/` (default `YYYYMMDD`, e.g. `YYYY/MM/DD` nests), and `--dest` creates the date folders under another directory.

Successful relocations update both the in-memory transfer result and the versioned import-state file atomically. State retains the originally planned destination as well as the relocated destination, allowing whole-file resume to recognize and verify a post-processed file instead of copying it again.

The screen reports each original filename, resolved date, metadata source, final path, and failure. When it reaches complete, Enter or `n` opens Result; Esc returns to the completed Processing screen for inspection.

## Result screen

Result uses a landscape summary rather than another progress surface:

```text
╭─ Import result ─────────────────────────────────────────────────────────────╮
│ TRANSFER COMPLETE · VERIFIED BEFORE PUBLISH                                │
│ Source       /Volumes/SD/DCIM                                               │
│ Destination  /Volumes/Photos/Import                                         │
│ 41/41 files passed Source and Destination SHA-256 comparison               │
│ Published 2.4 GB   Skipped 0   Failed 0                                    │
╰────────────────────────────────────────────────────────────────────────────╯

╭─ Verification summary ───╮ ╭─ Verified files ─────────────────────────────╮
│ ✓ Source stream hashed   │ │ 41 verified files · 2.4 GB                  │
│ ✓ Destination read back  │ │ › 1 ✓ IMG_0001.JPG · 24 MB · SHA-256 MATCH  │
│ ✓ SHA-256 matched        │ │   2 ✓ IMG_0002.DNG  · 51 MB · SHA-256 MATCH │
│ ✓ Published after verify │ │                                              │
╰──────────────────────────╯ │                                              │
╭─ State file ─────────────╮ │                                              │
│ › [x] Delete .dgs-state  │ │                                              │
╰──────────────────────────╯ ╰──────────────────────────────────────────────╯

  ↻ Again  r      Quit  q
  New import      › Exit dgs
```

The full-width header answers whether the batch is trustworthy before showing detail. It repeats Source and Destination, reports the total bytes successfully verified and published, and summarizes whether Post-processing was skipped or how many files were organized, skipped, and failed. Transfer-skipped and transfer-failed files do not contribute to the published byte count. Below it, Result uses the shared two-column skeleton. The narrow left side separates integrity evidence, post-processing outcome, and the final state-file decision. The wider right column owns the complete per-file result inventory and uses the shared numbered, selectable, vertically scrollable list; organized rows include their `YYYYMMDD` directory. Do not mix cleanup controls into the verification summary.

Delete state is selected by default, matching the confirmed lifecycle decision. Its filename is configured by the global configuration key `photo.import.state_file` and defaults to `.dgs-state`. The value must be a filename rather than a path so state cannot escape the selected Destination. The file lives directly inside that root as `<Destination>/<state_file>`; it is not stored in Source or in a global application-data directory. Result renders the configured filename in its cleanup control and confirmation copy. The explanatory copy makes retaining it an audit or diagnostic choice. When deletion is selected, leaving Result through Again or a confirmed exit—including Esc, `q`, Ctrl+C, and the Quit action—must remove the state file before navigation or process termination. If removal fails, remain on Result and report the error rather than silently exiting. When deletion is not selected, every exit path retains the file. `Alt+h/l` moves between the result inventory and state-file actions; list navigation uses the shared arrow/Vim behavior, mouse wheel works while hovering the inventory, and primary click changes focus or activates a control.

Result is terminal: it does not navigate back to Processing. `r` or the Again action starts a fresh Photo Import at Directories while retaining the last Source and Destination paths for convenient review or adjustment. Esc, `q`, Ctrl+C, or the Quit action opens the shared Quit confirmation; none exits immediately. Esc never returns to the previous workflow page.

## Verified-transfer algorithm

The Processing engine implements this integrity contract.

For each file:

1. Resolve the final destination as the Source file's relative path reproduced under the Destination root, creating any missing intermediate directories. Mirroring the hierarchy rather than flattening to the basename keeps files that share a name in different Source subdirectories distinct, so one batch never contains two jobs competing for a single final name or a single `.dgs-part` file. Create a uniquely named temporary file in that same destination directory, for example `.IMG_0001.JPG.dgs-part`. A same-directory temporary file allows the final rename to be atomic on the destination filesystem.
2. Open Source once and stream it sequentially into the temporary file while updating the Source SHA-256 digest in the same pass. Do not perform a separate pre-hash pass over Source.
3. Flush and close the temporary file. Where supported, sync it before verification. The integrity guarantee is defined at the user-space/filesystem API boundary; it does not attempt to prove physical-media cache behavior.
4. Read the temporary file sequentially and compute the Destination SHA-256 digest. This second destination read is required by the integrity contract and is not considered redundant scanning.
5. Compare size and digest. On mismatch, keep the final filename absent and report a hard verification failure. A later retry restarts this individual file from byte zero.
6. On a match, apply the Source modification time to the still-unpublished temporary file and sync its metadata. Destination filesystems may reduce timestamp precision. Access time is not treated as durable photo metadata because reading Source changes it on many systems.
7. Rename the temporary file atomically to the final filename. Sync the containing directory where supported before reporting success. The presence of the correct final extension means verification and metadata application succeeded at publication time.
8. For Move, delete that individual Source file immediately after its verified destination has been published successfully. A failure processing another file does not roll back already verified and moved files.

Normal successful operation therefore has the minimum strict-verification I/O profile: one sequential Source read, one Destination write, and one sequential Destination verification read. Hashing is performed during those required streams. Directory inventory must not reread complete file contents.

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

Classify failures rather than collapsing them into verification errors: Source read, Destination write, sync/close, Destination readback, hash mismatch, atomic publish, Source delete, state persistence, and transient network or mount loss are distinct outcomes. Preserve state after a NAS disconnect so a whole-file retry can resume the batch safely.

Parallelism remains an explicit user choice with default 1. Do not silently raise it based on CPU count. Multiple workers operate on different files; they never split one file into concurrent ranges. This avoids excessive seeks on SD cards or disks and avoids multiplying network requests on a NAS unless the user chooses that tradeoff.

### Confirmed failure behavior

- Disk-space preflight is not required. A write may fail when the destination becomes full; preserve batch state, report the affected file, and allow the user to free space and retry. The affected photo restarts from byte zero.
- Move commits per image: publish the verified destination, then immediately delete that Source image.
- Incomplete `.dgs-part` content is never promoted and is not byte-resumed.
- Failing to persist the import-state file is a bookkeeping failure, not a transfer failure. A file that passed the destination readback comparison and was published under its final name stays complete; the batch reports the state error once, separately from per-file outcomes. Collapsing the two would tell the user a verified photo had failed and would make the obvious remedy—retrying—recopy work that already succeeded. What is actually lost is whole-file resume for a later retry.
- A leftover `.dgs-part` is removed immediately when its transfer fails, including cancellation, rather than retained for diagnosis. The partial file carries no verified information, and retaining it would accumulate unreadable remnants in the Destination inventory. Diagnostic detail belongs in the per-file failure classification and the import-state file. A `.dgs-part` found at the start of a transfer is still treated as stale and removed, covering remnants left by a process that was killed outright.

### Remaining failure and conflict questions

The core Processing engine is implemented. The following decisions must be settled before adding the corresponding conflict handling and recovery enhancements:

- How Replace publishes atomically when a final destination already exists and how the previous file remains recoverable.
- Whether an existing destination with the same size and SHA-256 is treated as already imported without rewriting it.
- How to identify a Source file reliably when SD-card timestamps are coarse or unreliable.

Example status values:

```text
Left:    READY
Center:  Directories › Parameters › Processing › Post-processing › Result
Right:   arrows/hjkl  n Next  ↵ Select  ? Help
```

After the scan:

```text
Left:    PARAMETERS
Center:  Directories › Parameters › Processing › Post-processing › Result
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
- Result-stage retry controls beyond starting a fresh import.
- Cancellation and confirmation.
- Partial-success, empty, warning, and error variants of Result.
- Narrow-terminal layout beyond shared shrinking behavior.
- Photo Encode screens.

Do not fill these gaps speculatively.
