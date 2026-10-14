# Photo Import TUI Design

## Scope

This document owns decisions specific to `dgs photo import`. Read [`../../tui.md`](../../tui.md), [`../../parameter-controls.md`](../../parameter-controls.md), and [`../../file-explorer.md`](../../file-explorer.md) first for shared shell and component behavior.

Photo Import is a concrete consumer of the shared design language. Its page composition, labels, parameter groups, defaults, and workflow do not define requirements for other apps or commands.

Incremental import finds existing copies after date organization. An empty Source file has no content to recognise, so it is already present only at its own planned name or where state recorded it, never at an unrelated empty file. It is enabled initially and
can be disabled in Parameters. Both the workspace and CLI support multiple destination copies.

## Command-line import

`dgs photo import` without arguments still opens the existing workspace. A
source argument invokes the non-interactive Action; it does not open a TUI:

```sh
# Copy into one destination; incremental checks are enabled by default.
dgs photo import /path/to/card --dest /path/to/library

# Keep a complete copy in each destination.
dgs photo import /path/to/card --dest /path/to/library --dest /path/to/backup

# Preview selected files and destination inventories without changing files.
dgs photo import /path/to/card --dest /path/to/library --dest /path/to/backup --dry-run

# Organize JPG/RAW by capture date in every destination.
dgs photo import /path/to/card --dest /path/to/library --dest /path/to/backup --organize

# Import only JPG and DNG (both extensions must occur on the card).
dgs photo import /path/to/card --dest /path/to/library --ext JPG --ext DNG

# Keep different-content photos with the same name by adding safe suffixes.
dgs photo import /path/to/card --dest /path/to/library --duplicates keep-both

# Use two whole-file workers per destination.
dgs photo import /path/to/card --dest /path/to/library --workers 2

# Delete sources only after all destinations hold independently verified copies.
dgs photo import /path/to/card --dest /path/to/library --dest /path/to/backup --move

# Remove import state only after a fully successful import and organization.
dgs photo import /path/to/card --dest /path/to/library --organize --delete-state

# Disable content matching; use filename conflicts and verified resume state.
dgs photo import /path/to/card --dest /path/to/library --incremental=false

# Quote directory paths containing spaces.
dgs photo import "/path/to/SD Card" --dest "/path/to/Photo Library"
```

After taking more photos on the same card, rerun the same command. Incremental
checks find verified existing copies even in date folders and copy new or
changed content under the selected duplicate policy. Rerunning also resumes an
interrupted batch at whole-file granularity. No separate resume flag is needed.
For the preview example, remove `--dry-run` to execute the same selection.

At least one `--dest` is required. Each occurrence is a complete directory
path, including commas or spaces; no comma splitting occurs. Every destination
must already exist: a missing one may be the empty mount point of an unmounted
disk, which would silently receive the "backup", and only existing directories
can be compared by identity. Reject source/destination overlap, duplicate or
nested destinations and aliases — including other case or Unicode
normalization spellings of one directory — before writes. Each target's
directory identity, as validation saw it, is pinned for the batch: queued
targets must still be that directory, and transfer, state writes, publication
and Move checks refuse a missing or replaced root instead of recreating a mount
point. A network share that reconnects as a new mount counts as replaced; run
the import again. Each check is one metadata call on the root. These are
filesystem API checks, not protection against an external path change after a
check. Destinations on one filesystem (the same device, or one network share
mounted twice) are accepted with a warning: their copies are not independent of
each other. Each destination independently uses the
existing importer and retains its configured `import.state_file`. Missing or
corrupt state falls back to filesystem content checks when Incremental is on.

Defaults are Copy, Incremental on, duplicate policy `skip`, one whole-file
worker, and all regular files recursively with the same Source exclusions as
the workspace. `--incremental=false`, `--duplicates keep-both`, `--workers`
(1–8) and repeated `--ext` override these for this run. An `--ext` value
holding a comma or space, or matching no Source file, is refused: a misspelt
filter must not read as a successful, empty import. No settings are saved.
Replace remains unavailable. All targets run at once. Workers keep the bounded
whole-file model: each takes one file, prepares it in every target (resume,
incremental comparison, duplicate policy), then reads Source once and writes
every block to each target that needs a copy, each into its own `.dgs-part`.
A Source digest read for comparison is shared by all targets, and the copy is
bound to it. Each target then reads its copy back independently and publishes
only on a match. The slowest target sets the pace. A target that fails a file
loses only its own partial copy; the others continue. Interactive terminals
show a fixed progress region: processed-file count above, for each worker, one
row per destination, with aligned filename (on the first row), destination
number, phase and byte progress columns. A file counts as processed once every
destination has finished it. Bars
use wide muted sage-green dot patterns (dense for completed, sparse for remaining)
that shorten with the terminal, without gradients or background colors.
Text uses terminal defaults; `NO_COLOR` disables the green. Active worker events animate a braille
spinner; successful files show a checkmark and other outcomes an exclamation. A blank
row separates the overall progress from the workers. Destination paths and
completed-target summaries sit below the worker rows and update in place. Workers keep
their rows across files; normal completions update in place instead of
printing a line per file. Each target ends with a compact summary and any
failure or warning details. Import organization also prints counts rather than
a list of successful moves or files without capture dates on a terminal; actual
errors retain their file details. Redirected organization output keeps the list. The overall percentage counts processed files,
including failures; each worker's bar measures bytes in the current phase,
so completed copying alone never means verified completion. Redraws are
throttled, rows fit the terminal width and short terminals show how many
worker rows are hidden. The region clears before logs or exit. Redirected
output and dumb terminals retain plain per-file logs: every destination is
named first, and each file line carries its destination number. Cancellation
stops every target together; unfinished files fail in each, and a batch
cancelled before it starts reports every target as not started. The first Ctrl+C or SIGTERM cancels cleanly; a
second Ctrl+C ends the process even if a read is stuck on a hung mount. Successful copies are kept
when another target fails. Report each target separately and return nonzero
for conflict skips, failed/undetermined transfers, mismatched source content
between targets, state errors, organization failures or cancellation. The
closing summary counts files imported into every destination, source-kept
Moves included and counted apart.

A file is complete only when every target holds a verified copy of the same
content and the copies are distinct files: a hard link in one target to the
copy in another is one copy, not two. A file missing from any target is
`failed`, naming that target and its reason, whatever the operation.

`--move` first copies into all targets, then independently rechecks Source and
every target copy per file, with `--workers` parallel checks. Delete only an
unchanged Source whose size and SHA-256 agree with all copies, which must still
be distinct files. A failed target leaves the file `failed` and its Source in
place; `source-kept` is reserved for a file copied everywhere whose deletion
was refused (an unreadable or changed copy or Source, or a read-only card).
Other fully verified files may complete Move.
Destination state records remain verified Copy evidence for a later retry.

`--organize` explicitly opts into existing capture-date Post-processing in
each target after transfers, with the same pairing, collision and existing
counterpart checks. It organizes newly published and previously unorganized
copies, leaves other existing copies in place, and updates each target's state
paths. A verified Move may already have removed Source before organization;
an organization failure leaves verified copies at their imported paths. A
file whose capture date cannot be read — a RAW container the EXIF reader does
not support (CR3, RAF, ORF) without a JPG, or a JPG without EXIF — stays in
place and is reported as `No capture date`; it does not make the command fail.
Missing files, permission failures and other metadata-read I/O errors remain
failures and make the CLI return nonzero; they are not missing capture dates.

`--dry-run` (`-n`) exposes the read-only review step in the CLI: validate the
same options, list selected Source files and every Destination inventory, and
print the chosen parameters. It creates no directories, temporary files or
state and never deletes Source, even with `--move`. It does not hash file
contents or read capture dates, so it cannot promise final incremental skips,
conflict outcomes or organization paths; execution establishes those.

State is retained by default. `--delete-state` opts into Result cleanup after
all targets, source deletions and requested organization finish successfully.
Failures, cancellation and earlier source-kept Moves retain state for retry.
Cleanup checks that each destination still denotes the selected directory;
a cleanup error returns nonzero without rolling back verified copies.
Rerunning the same command provides whole-file resume; Ctrl+C provides clean
cancellation. Interactive pause, confirmation dialogs, file preview and list
navigation remain conveniences of the TUI rather than import parameters.

## Multiple destinations in the workspace

Directories shows Source followed by every selected destination as its own
numbered path row: Destination 1, Destination 2, and so on. Enter or a primary
click edits that row through the shared directory Explorer. `Add destination`
opens Explorer and appends a visible row without replacing any existing one.
Adding or editing refuses a directory already selected in another row, including
filesystem aliases; Explorer stays open and identifies the existing row.
Backspace on a destination row removes it; the `Remove destination N` button
also names and removes the last focused destination. Removing a row only
changes this session and never removes a directory or its contents. Add and
Remove use shared one-row buttons on one horizontal row, separated from the
path rows by a blank line.
At least one destination remains. Configuration supplies the initial
destinations (`import.dest`); rows added or removed here are session choices,
and Again retains them.

Scan inventories Source once and every destination without writes. Parameters
shows the selected destination's inventory and estimates; `[` and `]` switch
that inventory without changing extension selection. Processing covers all
selected targets. Multiple targets must already exist and pass the same overlap,
alias and nesting checks as the CLI. A single destination retains the existing
new-folder behavior. Same-filesystem targets produce a warning.

Targets run concurrently using `importer.RunMultiple`, sharing the pause/cancellation
controller and bounded whole-file workers. One source stream feeds all destination
writes; each destination verifies its own readback. Processing labels each
destination and worker and preserves their progress across interleaved events. The final verified-source count requires a
matching independent copy in every target. Move deletes each source only after
fresh source and all-destination checks succeed, which the header reports as
`checking copies before deleting sources N/M` once every copy is final; a target failure keeps the
corresponding source. Successful copies in other targets remain available.

Organizing runs outside the TUI update loop, including capture-date reads,
existing-copy verification, date-folder checks, renames and state updates.
The page shows the active destination, current phase and filename, and a
phase-local file progress bar when counts are available. An activity indicator
continues to update while a filesystem operation is waiting; it never invents
completed files or a completion percentage for unmeasured work.

The optional Post-processing choice organizes each started target in turn,
using that target's verified outcomes even if another target failed. Existing
date folders are confirmed separately for each root: Skip declines that
root only, keeps earlier roots' outcomes and goes on to the next. State paths are updated
in every organized target. Result groups file outcomes by destination and
also names incomplete overall outcomes, including source-kept Moves and
unstarted targets. Published bytes count new copies across all destinations.

The Result state-file choice applies to every selected destination. As in the CLI, cleanup keeps every state while any of them still records
an earlier source-kept Move. Unfinished
imports, earlier source-kept records and state errors retain state by default.
Cleanup checks every root's processing-time identity before removing any state,
and rechecks each root at removal; an error keeps Result open. These checks
have the documented filesystem API race boundary. Cleanup never deletes photos.

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
- The copy, readback and verified publication themselves are `internal/verifiedcopy`. The importer keeps what is Photo Import's own: the conflict policy, Move, the pause controller, and the progress events the pages draw. One mechanism means one place where the integrity contract can be read and tested.
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
- When controls, extension choices and the summary cannot fit vertically, show
  the required workspace height instead of hiding controls or overflowing.
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

The summary updates immediately when Operation, Incremental, Extensions, Duplicates, or Parallelism changes. With Incremental enabled it shows eligible files and bytes, mirrored-name conflicts, and files with same-size candidates needing content checks; exact copy and skip counts are established during Processing. With Incremental off it shows the filename-based transfer estimate. Extensions are discovered from the scanned Source inventory, normalized case-insensitively, sorted, and rendered as independent checkboxes; extensions absent from Source are not offered. All discovered extensions begin selected. A leading `[ All ]` action selects every discovered extension again, providing an explicit recovery from an empty filtered Source viewport. Right/l or Enter enters the extension subitems; Up/Down or k/j then moves through All and the extensions, Space or Enter invokes the current row, and Left/h or Esc exits the group. Duplicates opens with either Space or Enter. Changing the selection immediately filters Source, the two sidecar exception inventories, and Summary from the same source of truth. Destination remains complete so existing files and conflicts stay visible. Filename-conflict estimates compare the relative path under each root case-insensitively. Incremental content matching is separate and searches the complete destination tree, including date directories and renamed files. Do not summarize a long inventory with text such as `… 8 files more`; make each inventory a navigable viewport instead. All four inventories use the shared one-based line-number gutter. Within a focused list, `j/k` or Down/Up moves one file, `gg` moves to the first file, `G` moves to the last, and `h/l` or Left/Right pans long paths horizontally. The selected file uses `›` and the viewport follows it. A primary click selects a row and focuses its inventory. Hovering over an inventory and using the mouse wheel scrolls that inventory by three rows without changing focus. Space opens the selected file in macOS Quick Look. Right-clicking a row selects it and opens the shared context menu; Open with default app delegates to macOS `open`. `Alt+h/j/k/l` moves spatially among Parameters, Import summary, and all four inventories. `Esc` returns to Setup. The shared `n` Next Screen shortcut starts Processing without requiring traversal through unchanged parameters.

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
- `photo.import.source` and the `photo.import.dest` array in `photo/config.json` optionally prefill the source and all numbered destination fields. An empty source or destination array retains the repository mock directories for UI testing; users can edit, add or remove destinations in Directories.
- Operation: radio choice between Copy and Move.
- Incremental: checkbox, enabled for each fresh import. Confirm matching content
  anywhere under Destination before deciding whether to copy; disabling it uses
  filename-conflict handling and retained-state recovery alone. It is a session
  control, with no additional configuration key.
- Extensions: dynamic multi-checkbox choices derived from the extensions actually present in Source; all are selected initially.
- Duplicates: filename-conflict policy. Skip leaves the existing destination untouched, Replace intends to publish the verified new file at that path, and Keep both chooses a new unique filename. A name whose metadata cannot be read — a path component that is a file, a folder without permission, a name made too long by its `-N` suffix — fails that file instead of counting as taken, and the search for a free name is bounded. Replace is not the default; its backup and rollback semantics remain to be designed before implementation.
- Parallelism: positive worker count controlling concurrent file transfers; default `1` for predictable removable-media I/O and bounded resource use.
- `Next → Processing`: starts the selected transfers. Replace remains unavailable until its backup and rollback contract is implemented; the screen reports this without starting file operations.

## Processing screen

Processing uses the shared two-column landscape skeleton and keeps vertical stacks shallow. A compact full-width header shows file progress, aggregate progress, active workers, pending files, and failures. Below it, the narrow right column is divided vertically between Next files and Recent results, with Page Actions at bottom-right. The wider left column contains the worker list so filenames, metadata, step tracks, hashes, and progress bars receive the available width.

Each whole-file worker uses the same row formatter and progress-bar style as the CLI. Its first destination row names the worker and file; subsequent destination rows align underneath it and show their own phase and byte counts. Completed rows show a check mark for successful outcomes or an attention marker for failures and source-kept Moves. Bars use the CLI's filled and empty cell pattern, with byte counts retained on narrower panels. Only engine terminal outcomes mark a row complete; copied bytes alone never imply verification. When workers exceed the visible height, scroll complete worker groups with Up/Down or `k/j`, and jump with `g`/Home or `G`/End. Pausing and cancellation retain the shared controller. Progress events remain bounded by `importer.ProgressInterval`.

The aggregate progress bar uses the same cell pattern as the CLI and reserves a fixed right-hand status segment such as `33% │ 4/12 files`. Calculate the bar width from the remaining cells after that segment so the counts remain visible. Like the CLI, aggregate progress counts files whose outcomes are final in every destination; the separate verified count excludes failed or skipped files.

Processing supports 1–8 workers (`importer.MaxWorkers`) and defaults to 1. Each worker handles a different whole file and reports real engine events. It writes same-directory `.dgs-part` files and persists `.dgs-state` when the batch starts and when it ends; while it runs, pending changes are written within two seconds, even while a file is paused. `p` pauses or resumes I/O between bounded chunks. During active Processing, every key that could leave work is guarded: `Esc` asks before returning to Parameters, and `q` or Ctrl+C asks before exiting. Opening the confirmation pauses I/O; choosing No resumes it unless it was already paused. Choosing Yes cancels the batch and waits for workers to stop before navigation or exit. Returning to Parameters re-reads both roots: a Move has removed sources, and every run adds destination files. The shared dialog defaults to No; Tab switches actions, Enter invokes the selected action, and Esc continues processing.

Photo Import status uses the shared three-part bar as follows: the left chip identifies `SCAN · READING`, `PROCESSING · RUNNING`, `PROCESSING · PAUSED`, `PROCESSING · COMPLETE`, or confirmation state; the center keeps the workflow track; and the right side contains only context-valid controls. A paused import replaces `p Pause` with `p Resume`; a completed import no longer advertises pause.

Processing remains visible when it reaches complete so its final worker state can be inspected. Enter or `n` opens Post-processing, whose confirmation defaults to `Skip`.

## Post-processing screen

Post-processing receives newly published files as rename targets and verified
existing counterparts as read-only shot context. An already-present or resumed
file that still stands where Import places files — mirroring its Source folder,
because an earlier run was interrupted or skipped organizing — is a rename
target too, so a shot is not split between that folder and a date folder.
These movable existing files retain their recorded size and SHA-256 and are
revalidated before metadata reads and again after the date-folder confirmation;
a changed file keeps the whole shot at its current paths.
Existing files anywhere else never enter the rename list. Its page first shows a confirmation dialog. `Skip` is the safe
default and performs no metadata reads or renames before continuing to Result.
Choosing `Organize by capture date` closes the dialog and applies these rules:

1. A RAW with a same-directory, case-insensitive filename-stem JPG/JPEG sidecar and that JPG/JPEG both use the sidecar's EXIF capture time.
2. A JPG/JPEG without RAW uses its own EXIF capture time.
3. A RAW without JPG/JPEG uses capture time read directly from the RAW through the existing Go EXIF library.

A selected JPG already present in a date folder can provide capture time and
same-shot ownership for a newly published RAW. Revalidate its bytes before
planning and again after the existing-date-folder confirmation. An unrelated
same-stem member, renamed ambiguous counterpart, or inconsistent date folders
leaves the new file at its verified Processing path and reports organization
unfinished. Existing counterparts are never moved or overwritten. A shot whose
files were published under different names — Keep both renamed one, as
`IMG_0001-2.JPG` beside `IMG_0001.CR2` — is left in place and reported, since
in a date folder it would pair with nothing or with another shot.

The destination is `<Destination>/YYYYMMDD/<filename>`. Post-processing reads the already verified destination file, not a deleted Move source. It uses same-filesystem rename and never overwrites an existing target. A missing/invalid capture time, unsupported RAW container, unsafe date directory, target conflict, or rename failure leaves that file at its published Processing path and reports a per-file failure. Non-photo files are skipped. Collisions are decided per shot — the files of one source folder sharing a filename stem — before anything moves: a shot whose name another shot of the batch also takes in the same date folder, or which that folder already holds under any extension, stays at its published path whole and is reported, so a RAW is never filed beside a different shot's JPG. If one file of a shot fails to move, the rest of that shot stays with it. Before moving any paired RAW, read its sidecar date so processing order cannot invalidate the metadata source. Destination's identity is taken when the moves are planned and checked again before each date folder is made and each file moves: a disk unmounted while the confirmation waits leaves every file in place rather than filling an empty mount point.

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

The full-width header answers whether the batch is trustworthy before showing detail. It repeats Source and Destination, reports the total bytes successfully verified and published, and summarizes whether Post-processing was skipped or how many files were organized, skipped, and failed. Already-present, transfer-skipped and transfer-failed files do not contribute to the published byte count. Results separate verified existing copies from conflict skips and new publications, and show the actual matching destination path. Below it, Result uses the shared two-column skeleton. The narrow left side separates integrity evidence, post-processing outcome, and the final state-file decision. The wider right column owns the complete per-file result inventory and uses the shared numbered, selectable, vertically scrollable list; organized rows include their `YYYYMMDD` directory. Do not mix cleanup controls into the verification summary.

Delete state is selected by default, matching the confirmed lifecycle decision. Its filename is configured by the global configuration key `photo.import.state_file` and defaults to `.dgs-state`. The value must be a filename rather than a path so state cannot escape the selected Destination. The file lives directly inside that root as `<Destination>/<state_file>`; it is not stored in Source or in a global application-data directory. Result renders the configured filename in its cleanup control and confirmation copy. The explanatory copy makes retaining it an audit or diagnostic choice. When deletion is selected, leaving Result through Again or a confirmed exit—including Esc, `q`, Ctrl+C, and the Quit action—must remove the state file before navigation or process termination. If removal fails, remain on Result and report the error rather than silently exiting. When deletion is not selected, every exit path retains the file. `Alt+h/l` moves between the result inventory and state-file actions; list navigation uses the shared arrow/Vim behavior, mouse wheel works while hovering the inventory, and primary click changes focus or activates a control.

Result is terminal: it does not navigate back to Processing. `r` or the Again action starts a fresh Photo Import at Directories while retaining the last Source and Destination paths for convenient review or adjustment. Esc, `q`, Ctrl+C, or the Quit action opens the shared Quit confirmation; none exits immediately. Esc never returns to the previous workflow page.

## Verified-transfer algorithm

The Processing engine implements this integrity contract.

For each file:

1. Resolve the final destination as the Source file's relative path reproduced under the Destination root, creating any missing intermediate directories. Mirroring the hierarchy rather than flattening to the basename keeps files that share a name in different Source subdirectories distinct, so one batch never contains two jobs competing for a single final name or a single `.dgs-part` file. Create a uniquely named temporary file in that same destination directory, for example `.IMG_0001.JPG.482913.dgs-part`, exclusively. A same-directory temporary file allows the final rename to be atomic on the destination filesystem. The random part makes the name belong to one copy: two imports of cards that both hold `IMG_0001.JPG` into one Destination never write, read back or publish through a shared path. Directories below the Destination root are checked without following links and created one level at a time; a symbolic link inside the tree is refused, not written through. The root itself may be a link.
2. Open Source once and stream it sequentially into the temporary file while updating the Source SHA-256 digest in the same pass. Do not perform a separate pre-hash pass over Source.
3. Flush and close the temporary file. Where supported, sync it before verification. Reopening alone would read the pages this process just wrote, so the write and readback handles ask the operating system not to answer from its cache: `F_NOCACHE` on macOS, `POSIX_FADV_DONTNEED` after the sync on Linux. The same request applies to content checks that decide skipping or deleting. A filesystem may ignore it; the integrity guarantee is defined at the user-space/filesystem API boundary and does not attempt to prove physical-media cache behavior.
4. Read the temporary file sequentially and compute the Destination SHA-256 digest. The readback handle must be the same file that was written; a temporary name that has come to denote another file fails the copy. This second destination read is required by the integrity contract and is not considered redundant scanning. An error closing this read-only handle after a complete read is not a failure: the digest is already of every byte, and nothing is pending on the handle. macOS SMB mounts report `EBADF` there.
5. Compare size and digest. On mismatch, keep the final filename absent and report a hard verification failure. A later retry restarts this individual file from byte zero.
6. On a match, apply the Source modification time and permission `0644` to the still-unpublished temporary file and sync its metadata. The unpublished copy is owner-only; a library on a NAS is read by more than its owner. Destination filesystems may reduce timestamp precision. Access time is not treated as durable photo metadata because reading Source changes it on many systems.
7. Confirm the temporary name still denotes the written file, then rename it atomically to the final filename without ever replacing an existing file: `renamex_np(RENAME_EXCL)` on macOS, `renameat2(RENAME_NOREPLACE)` on Linux, a hard link where neither is available, and check-then-rename only on a filesystem that supports none of them. Sync the containing directory where supported before reporting success. The presence of the correct final extension means verification and metadata application succeeded at publication time.
8. For Move, delete that individual Source file immediately after its verified destination has been published successfully, and only while it is still the file that was read: same file identity, size and modification time. A failure processing another file does not roll back already verified and moved files. A Source that cannot be deleted — a write-protected card — or that changed after it was read is kept, and the file is reported `source kept`: the photo is imported, counted as published and offered to Post-processing; only the move is unfinished.

Normal successful operation therefore has the minimum strict-verification I/O profile: one sequential Source read, one Destination write, and one sequential Destination verification read. With several destinations the one Source read feeds every Destination write; each Destination still has its own write and verification read. Hashing is performed during those required streams. Directory inventory must not reread complete file contents.

Incremental comparison is additional I/O when same-size candidates exist:
read Source and independently check candidates, including date subdirectories.
Matching content is Already present and is not newly published or counted in
published bytes. Without a match, a comparison-bound transfer must preserve the
checked source identity and digest before publication. With no candidates, copy
retains the one-source-read profile. Move over an existing match independently
revalidates Destination before deleting the unchanged Source. A filename Skip
or undetermined comparison never authorizes source deletion. An undetermined
result names the candidate that could not be read. A failed candidate read is
not remembered: the next source of that size reads it again, while a digest
once read is shared by every worker.

With Incremental, a Skip means the name holds different content — no copy of
this photo exists anywhere under Destination — so the file is reported
`same name, different content: not imported` and counted apart from other
skips, typically after a camera's counter restarted.

The inventory is taken once before anything is published, so two Source files
of identical content in one batch are both copied; the later run sees both. It
is read again rather than reused from Scan because matching needs each file's
identity and modification time, and the tree may have changed since.

### Resume state

Resume operates at whole-file granularity, not byte or chunk granularity. Interrupted transfers use a versioned, atomically updated import-state file. It records the batch plan and which individual files have completed successfully.

New writes use version 2; versions 1 and 2 are supported on read. In incremental
mode state only supplies candidate hints; deleting it does not lose detection
of already-imported files. Current-run publications and existing-match outcomes
are recorded separately. Older readers reject version 2 rather than interpreting
its new outcomes as successful publications.

At minimum, record:

- Source identity: normalized path, size, modification time, and any available stable file identity.
- Destination temporary and final filenames.
- Hash algorithm and the verified Source/Destination digest for completed files.
- Intended final path and the parameters that affect it.
- Per-file state such as pending, transferring, verified, source-deleted, or failed.

Persist state when the batch starts and when it ends; while it runs, write pending changes within `importer.DefaultStateInterval` (two seconds), even while a file is slow or paused, and never rewrite unchanged state on a tick, so bookkeeping does not grow with the square of the batch or stall workers on a NAS. A crash loses at most that interval of records; each of those files is verified again before it is trusted, or found again by Incremental. Records of earlier batches into the same Destination are kept, so importing another card in between does not cost the first one its resume. If a process stops while one `.dgs-part` is incomplete, that partial file is never resumed internally. On retry, discard or truncate it and transfer that photo again from byte zero. Already published and recorded files are not recopied.

Before skipping an item recorded as complete, confirm that the expected final destination still exists and matches the recorded identity or digest policy. Planning compares only metadata (the destination's size); the digests are read by the workers, with progress, pause and cancellation like any transfer. An interrupted check leaves the earlier record in state; a refuted record imports the file as new under the conflict policy. State is bookkeeping, not evidence: a record is trusted only when its source and destination digests agree, its destination lies under the pinned Destination root with no link on the way, and that destination is a different file from the source. A damaged, edited or foreign record proves nothing and the file is imported anew. Recorded items are matched against each job's planned destination before Keep both chooses a new name, and state keeps that planned destination, so a rerun resumes a Keep both batch instead of copying it again. A Move over a recorded item whose Source still exists and still matches the recorded digest deletes that Source — whether the earlier run was a Copy or a Move that kept its source. Before retrying an incomplete item, confirm that Source still matches its recorded identity. Never rename a leftover partial file merely because its expected byte count has been reached.

The Result screen must offer an explicit choice to delete or retain the import-state file after the batch has reached its terminal state. Delete is the default only when nothing is left to do; a batch with failed, undetermined, unstarted, source-kept or not-imported files keeps its state by default, because without Incremental a later run needs it to finish. The state file also carries earlier batches' records into the same Destination; a source-kept Move among them keeps it by default too, and Result counts those apart. Retaining it supports auditing or later diagnosis; deleting it does not delete imported photos.

### Source and destination entry policy

Import regular files only. Directories are traversal structure rather than transfer items. Hidden Source entries are not camera files and are listed as not scanned: AppleDouble `._` companions carry a photo's extension but hold metadata. Trash, recycle-bin, snapshot and metadata folders (`.Trashes`, `.Trash-<uid>`, `$RECYCLE.BIN`, `#recycle`, `@Recycle`, `#snapshot`, `.snapshot`, `.zfs`, `@eaDir`, `.Spotlight-V100`, `.fseventsd` and the like, in `skipdirs`) are left out on both sides, readable or not, and by `dgs photo organize --recursive`. Source symbolic links are skipped and reported because following one could read outside the selected SD-card root, and its target could change between scan and transfer. FIFOs, sockets, block or character devices, and other special entries are rejected because reading them can block, produce an unbounded stream, or access something other than the scanned file.

Every Move deletion — after publication, after a resumed record, and after an incremental match — first confirms the pinned root is still there; otherwise the source is kept and reported, and a later run finishes it. Every Processing run pins its Destination root the same way, the workspace's included: a root that existed when Processing started, or that Processing created then, is never recreated if it vanishes or is replaced mid-run. A Destination that does not exist is shown in the Import summary as a new folder to be created, because the empty mount point of an unmounted disk looks the same.

Destination path components and an existing final entry must be checked with non-following metadata operations. Do not write through a destination symbolic link. Create temporary files exclusively so an unrelated existing file cannot be silently reused.

Scan never stops at an unreadable folder: it reports it and continues, so nothing after it in lexical order silently disappears from the inventory. An unreadable hidden folder (`.Trashes`, `.Spotlight-V100`) is listed as not scanned; any other unreadable Source entry makes the scan incomplete, the Import summary shows it, and Processing refuses to start until a rescan succeeds, because a Result reporting every scanned file verified must not be read as every file on the card. Skipped links and special files are listed as not scanned. A Destination that does not exist yet is not an error.

All other unreadable Destination subtrees, including hidden ones, block
incremental Processing. A photo found only in a trash or snapshot folder is not
a durable copy: it is imported again, and a Move never deletes its source on
that evidence. Excluded destination links, special files and import artifacts
are listed separately. Source and Destination roots must not overlap or alias
one another in either mode, compared by file identity so a case-insensitive volume cannot hide an
overlap behind another spelling, and a source hard-link alias is not evidence
of an independent copy.

If a regular Source file disappears after Scan, fail that item clearly and continue or stop according to the later batch-failure policy; never create an empty final file. If it changes while being copied, detect the change by comparing size, modification time, and available stable file identity before and after the stream. Discard the unverified temporary result and require a fresh transfer of that image.

### Parallelism

Implement parallelism as a bounded worker pool over files, not as multiple writers to one file. Default to one worker because SD cards and external disks often perform worse under competing reads and writes. Each worker owns at most one Source stream, one destination temporary file per target, and bounded copy buffers. The UI must expose active, verified, failed, and pending counts independently; completion means every published file passed verification.

Concurrency increases throughput only when the source and destination devices benefit from it. It does not relax ordering, temporary-file, sync, or verification rules.

### Storage topology

The transfer engine must behave correctly when Destination is a local USB volume, another disk on the same host, or a NAS mount. Always create `.dgs-part` in the final destination directory so publication never depends on a cross-filesystem rename. Treat rename and sync guarantees as those offered by the mounted filesystem and server at the user-space API boundary; do not claim stronger physical-media durability.

Use sequential streams and bounded buffers for every topology. Do not use direct I/O or a platform-specific cache bypass as a correctness requirement. A local same-device copy, a cross-device copy, and a network copy all receive the same independent destination readback and SHA-256 comparison.

Classify failures rather than collapsing them into verification errors: Source read, Destination write, sync/close, Destination readback, hash mismatch, atomic publish, Source delete, state persistence, and transient network or mount loss are distinct outcomes. Preserve state after a NAS disconnect so a whole-file retry can resume the batch safely.

Parallelism remains an explicit user choice with default 1. Do not silently raise it based on CPU count. Multiple workers operate on different files; they never split one file into concurrent ranges. This avoids excessive seeks on SD cards or disks and avoids multiplying network requests on a NAS unless the user chooses that tradeoff.

### Confirmed failure behavior

- Disk-space preflight is not required. A write may fail when the destination becomes full; preserve batch state, report the affected file, and allow the user to free space and retry. The affected photo restarts from byte zero.
- Move commits per image: publish the verified destination, then immediately delete that Source image. A Source that cannot be deleted is `source kept`, not failed.
- Incomplete `.dgs-part` content is never promoted and is not byte-resumed.
- Failing to persist the import-state file is a bookkeeping failure, not a transfer failure. A file that passed the destination readback comparison and was published under its final name stays complete; the batch reports the state error once, separately from per-file outcomes. Collapsing the two would tell the user a verified photo had failed and would make the obvious remedy—retrying—recopy work that already succeeded. What is actually lost is whole-file resume for a later retry.
- A leftover `.dgs-part` is removed immediately when its transfer fails, including cancellation, rather than retained for diagnosis. The partial file carries no verified information, and retaining it would accumulate unreadable remnants in the Destination inventory. Diagnostic detail belongs in the per-file failure classification and the import-state file. A `.dgs-part` found at the start of a transfer is still treated as stale and removed, covering remnants left by a process that was killed outright.

### Remaining failure and conflict questions

The core Processing engine is implemented. The following decisions must be settled before adding the corresponding conflict handling and recovery enhancements:

- How Replace publishes atomically when a final destination already exists and how the previous file remains recoverable.
- Whether to add a faster, weaker metadata-only mode; the implemented incremental
  mode requires size and SHA-256 agreement for an existing-copy claim.
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
