# Capture Scan TUI Design

## Confirmed scope

- `dgs capture` launches the Capture TUI directly and has no user-facing
  subcommand.
- Capture uses command-owned sessions presented as tabs rather than the
  sequential page workflow used by Photo Import.
- The initial session is `Scan`, rendered as the active `SCAN` tab at the
  left of the shared top bar. `Route` is the second session; see
  [`route.md`](route.md) for its layout, keys, and configuration.
- Scan uses the shared three-column landscape skeleton. All three columns use
  the same workspace background and are rendered as Fieldsets.
- The left quarter starts with the scrollable `CAPTURES` list, which extends
  down to a compact `CAPTURE ROOT` path control pinned to the workspace bottom
  and aligned with the center and right Fieldsets. Enter or a primary click on
  the Root control opens the shared directory-only File Explorer as an overlay.
  This control is shared with Route rather than reimplemented per session, and
  both sessions move to a root chosen in either one.
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
  fields in `ID`, `Created`, `Schema` order, an indented Source group whose rows
  are `App`, `Workflow`, `Device`, `System`, and latitude, longitude, and
  altitude on one compact GPS row. Fields the schema does not describe are never
  presented. The shared anchored Divider separates metadata from
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
  well as Up/Down. All Alt navigation is spatial: `Alt+J`/`Alt+Down`
  from Captures reaches the Capture Root below it, `Alt+L`/`Alt+Right` from the
  Root control or Captures reaches Preview, `Alt+L` from Preview reaches Capture
  Info, and `Alt+J`/`Alt+K` move between the stacked Capture Info and File Info.

Each Capture row carries `●` once the Capture has been organized, and the
organizer's record `organize.json` appears in its file list beside the index.
Capture Info reports the organizing state in an `Organized` group: the Recipe of
the most recent pass, when it ran, how many passes there have been when there is
more than one, and the Actions that pass planned with their targets. A Capture
with no record reads `· Not organized` rather than omitting the group, because
whether a Capture has been handled is a question Scan should always answer. The
record is read once with the Capture, so Scan and Route agree on which Captures
are done. See [`organizer.md`](organizer.md) for the record itself.

A JSON preview is offered in two shapes: `SOURCE` is the file as written,
keeping its key order and formatting, and `TREE` is its structure — nesting,
item counts, and a colour per value type, with keys sorted so two producers'
files read the same way. The legend names the current shape and the choice is
remembered across files. Neither shape substitutes for the other, which is why
both are kept rather than one replacing the other; both are prepared when the
file is read, so switching costs no reload.

`t` switches between them. It belongs to `PREVIEW` and exists nowhere else: a
key that does nothing in three fields out of four is a key whose context the
reader has to remember. The status bar hint follows the same rule — it offers
`t Tree` only for a JSON file, `t Source` only while the tree is shown, and
nothing at all otherwise.

The tree is navigated the way the File Explorer is navigated, because a tree the
reader already knows how to walk should not be walked differently here:
`↑/k` and `↓/j` move, `→/l` expands or steps into what is open, `←/h` collapses
or steps out to the parent, `o` toggles, `O` collapses the parent and focuses
it, `w` collapses everything below the root, and `g`/`G` jump to the ends. A
click selects a row. While the tree is shown these replace the preview's
scrolling keys, and the tree scrolls itself to keep the cursor on screen.

The tree draws itself for one width and one focus state, so it is redrawn
whenever either changes: a row painted for a wider column would run over the
fieldset border, and a cursor painted for a focused field would stay lit after
the focus had left it. Rows are clipped by terminal cells rather than by
counting runes, because a coloured row carries escape sequences that occupy no
cells and a CJK value occupies two per rune.

Indentation is four spaces per level. A container shows `▸` or `▾` and how many
items it holds; collapsed it reads `{ … }` with that count, so a reader can tell
an empty container from one worth opening. An expanded container closes with its
bracket on a line of its own: a reader asked to read JSON should see JSON, and
an opening brace with no closing one reads as truncated however clear the
indentation is. A closing bracket is a line rather than a node — the cursor
steps over it, and clicking one selects the container it closes. The top level opens expanded and
deeper containers start collapsed, so a file opens as an outline rather than as
everything at once.

`place` may provide `locality`, `city`, `region`, and `country` as optional
non-empty strings. Capture Info presents available values as four separate rows
inside an indented `Location` group after GPS. There is no free-form `address`
field: a place is described by its structured parts, and anything that reads as
one name is composed from them. Descriptive fields are accepted only inside
`place`; `coordinates` takes `altitude`,
`longitude`, and `latitude` and nothing else.

A Capture with no `coordinates` shows neither the GPS row nor the Apple, Google,
and Amap links, and a Capture with no `place` shows no `Location` group, rather
than a zero coordinate or an empty group. Embedded CR/LF line breaks are
normalized to ` · ` so every property and the status bar remain one row.
- With `CAPTURES` focused, Space opens the selected Capture directory, index,
  or attachment in macOS Quick Look. This delegates to `qlmanage -p`; Capture
  does not implement its own image rendering or require Chafa.
- `Tab` and `Shift+Tab` move between the Captures list and the Root control.
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

- `schema`, `source`, `id`, and `createdAt` are required. `source` requires
  `app`, `workflow`, and `device`; `workflow` names the producing workflow, for
  example `note` or `photo_note`, and replaces the earlier top-level `type`.
- Location is split in two and both halves are optional. `coordinates` carries
  `altitude`, `longitude`, and `latitude` and nothing else; `place` carries the
  descriptive fields `locality`, `city`, `region`, and `country`, each optional
  but non-empty when present. A Capture may have coordinates, a
  place, both, or neither: taken indoors it may know its city and no
  coordinates, and a bare Capture may know neither.
- The schema describes only what the Capture sessions read. A producer may
  write other fields; they are neither required nor rejected, and never
  presented.
- `attachments` is optional. Every attachment is an object and requires a
  string `kind`. The string value `null` means the item is ignored.
- `payload` is optional. When present it is an object whose workflow-specific
  fields are deliberately unconstrained by the base v1 schema.

## Mock Capture root

`testdata/capture` is a generated Capture root for exercising Scan and Route
without touching real data. It holds eight valid Captures—a `been_here` note with a
text attachment, photo with JPEG and PNG attachments, voice memo with a WAV, a
Capture carrying undescribed producer fields (the only one), a `quick_mark`
with a partly filled place, an ignored `"null"` attachment beside a real
one, a place with no coordinates, and a required-fields-only Capture with no
location at all—plus three directories that must be rejected: `not-a-capture` (no index file),
`invalid-schema` (`"schema": "v2"`), and `invalid-json` (truncated JSON).

`testdata` is not tracked, so the fixture is generated rather than committed:

```bash
go run ./cmd/mockcapture
```

Then point `capture.scan.root` at `testdata/capture`, or start `dgs` from the
repository root and browse to it with the Capture Root control.
