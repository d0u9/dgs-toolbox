# Capture Scan TUI Design

## Confirmed scope

- `dgs capture` launches the Capture TUI directly and has no user-facing
  subcommand.
- Capture uses command-owned sessions presented as tabs rather than the
  sequential page workflow used by Photo Import.
- The initial session is `Scan`, rendered as the active `SCAN` tab at the
  left of the shared top bar.
- Scan uses the shared three-column landscape skeleton. All three columns use
  the same workspace background and are rendered as Fieldsets.
- The left quarter starts with a compact `CAPTURE ROOT` path control. Enter or
  a primary click opens the shared directory-only File Explorer as an overlay.
  The scrollable `CAPTURES` list begins immediately below it and extends to
  the workspace bottom, aligned with the center and right Fieldsets.
- `CAPTURES` includes only immediate child directories containing a regular
  file whose name matches the configured index filename and whose contents
  validate against the declared Capture index schema. Invalid JSON, unsupported
  schemas, and schema violations are omitted. Folder labels retain a trailing
  path separator so they read as directories rather than files.
- Capture rows use `▸` when collapsed and `▾` when expanded. Pressing `o` on a
  Capture row toggles it. An expanded Capture shows the configured index file,
  followed by attachment files referenced by the validated index in manifest
  order. Attachments with `kind` equal to the string `null`, missing names,
  unsafe paths, missing files, non-regular files, and duplicates are omitted.
  Scan does not hash attachment contents merely to build this navigation tree.
- In the Captures tree, `Shift+O` collapses the Capture containing the current
  row and returns selection to its folder. `w` collapses every Capture.
- `R` is a Scan-wide refresh key in every DataField. It rescans the current
  Capture root and reapplies the configured index-file and v1 schema filters
  without blocking input, so newly added valid Capture directories appear.
- Scan selects the shared center-wide three-column variant: left `1/4`, center
  `1/2`, and right `1/4`. The center column is a full-height, focusable
  `PREVIEW` Fieldset. Arrow/Vim keys and the mouse wheel scroll long previews;
  horizontal movement pans long lines.
- Selecting a text child loads it asynchronously and displays a fixed
  three-cell line-number gutter. Numbers beyond 999 display as `###`, keeping
  the text column stable. JSON is beautified with two-space indentation before
  numbering. The configured index file is treated as JSON even when its name
  does not end in `.json`. Text previews are limited to 1 MiB.
- JPEG, PNG, and GIF children use `go-termimg`'s Unicode half-block renderer in
  the Preview Fieldset. Images are scaled to the largest size that fits both
  available dimensions while preserving aspect ratio, then centered both
  horizontally and vertically. Capture computes the final terminal-cell bounds
  once using the half-block cell aspect and asks `go-termimg` only to render
  those bounds; it does not stack `go-termimg`'s fit calculation on top of its
  own sizing. This is the portable
  fallback used by Alacritty; Quick Look remains available for a full-quality
  view.
- Text preview loading is debounced for 120 ms and every request carries a
  generation identifier. Selecting a JPEG, PNG, or GIF does not render it;
  Preview displays `Press Enter to preview` while File Info loads independently.
  Enter/CR explicitly starts image rendering. A late result from an older
  selection or terminal size cannot replace the current preview.
- Informational Preview states such as selection prompts, loading messages, and
  `Press Enter to preview` are centered horizontally and vertically.
- The right column is divided vertically. `CAPTURE INFO` shows fixed index
  fields in `ID`, `Created`, `Type`, `Schema` order, an indented Source group, and latitude,
  longitude, and altitude on one compact GPS row. The persisted `isDone` value
  is not presented. The shared anchored Divider separates metadata from
  clickable OSC 8 links for Apple Maps, Google Maps, and Amap. The divider and
  three map rows are pinned to the bottom of Capture Info while metadata above
  them scrolls. Non-null
  attachments form an indented numbered group. `FILE INFO` shows only filesystem
  and media metadata for the selected file: name, MIME type, byte size, and
  creation/modification dates. It does not repeat or flatten JSON content.
  Image files also show decoded dimensions, format, capture time,
  camera maker/model, and lens when those EXIF values exist.
- WAV and MP3 File Info includes decoded audio duration. WAV duration is derived
  from RIFF byte rate and data length; MP3 duration comes from its decoded stream
  length and sample rate.
- Capture Info uses a fixed 12-cell key column across all captures; its keys are
  right-aligned and values always begin at the same column, preventing layout
  movement as selection changes. File Info sizes its key column to its own
  current metadata. Keys are
  the same column. Keys and values share a row, conserving vertical space.
  Capture Info and File Info are independent viewports: focused
  arrow/Vim input and hover-wheel input scroll them without moving focus. The
  Preview and Captures regions also accept hover-wheel scrolling. A visible
  `↑ more` or `↓ more` marker indicates vertically clipped content in every
  scrollable DataField.
- The status bar center shows the complete value of the current item rather than
  a count summary. In the property fields, Up/Down moves the current property;
  selecting a map row therefore exposes its complete URL in the wider status
  region even when the right column truncates its display.
- Capture Info and File Info property rows are selectable by primary click as
  well as Up/Down. Capture Scan gives `Alt+J` and `Alt+K` an explicit cyclic
  field order: `Captures → Preview → Capture Info → File Info`; `Alt+K` walks
  the same order in reverse. Horizontal Alt navigation remains spatial.

Position may provide `locality`, `city`, `region`, and `country` as optional
non-empty strings. Capture Info presents available values as four separate rows
inside an indented `Location` group after GPS. The wire spelling `"region "`
with a trailing space is accepted as a compatibility alias for `region`; the UI
always labels it `Region`. The earlier optional `address` remains accepted as a
legacy fallback when the structured location fields are absent. Embedded CR/LF
line breaks are normalized to ` · ` so every property and the status bar remain
one row.
- With `CAPTURES` focused, Space opens the selected Capture directory, index,
  or attachment in macOS Quick Look. This delegates to `qlmanage -p`; Capture
  does not implement its own image rendering or require Chafa.
- `Tab` and `Shift+Tab` move between the Root control and Captures list.
  `Alt+Arrow` and `Alt+h/j/k/l` move between DataFields. Captures uses the
  shared scroll-list keys and mouse-wheel behavior.
- Scan opens with the `CAPTURES` DataField focused.

## Configuration

```json
{
  "capture": {
    "scan": {
      "root": "",
      "index_file": "index.json"
    }
  }
}
```

- An empty `capture.scan.root` starts at the current working directory.
- `capture.scan.index_file` defaults to `index.json` and must be a filename,
  not a path.

## Capture index v1

The v1 Capture metadata contract is defined by
[`index-v1.schema.json`](index-v1.schema.json).

- `schema`, `source`, `position`, `id`, `isDone`, `createdAt`,
  `type`, and `dir` are required.
- `attachments` is optional. Every attachment is an object and requires a
  string `kind`. The string value `null` means the item is ignored.
- `payload` is optional. When present it is an object whose type-specific fields
  are deliberately unconstrained by the base v1 schema.
- v1 retains the wire field name `systemVesion` and represents `isDone` as the
  strings `true` or `false`. Correcting either representation requires a new
  schema version rather than silently changing v1.
