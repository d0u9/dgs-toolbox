# Capture Route TUI Design

## Confirmed scope

- `Route` is the second Capture session, rendered as a `ROUTE` tab beside
  `SCAN` in the shared top bar. Both tabs belong to one Capture command; only
  one is visible at a time.
- `[` and `]` move between Capture tabs, and a primary click on a tab in the
  top bar activates it. The keys are ignored while the active session owns
  them, so the Scan file-explorer overlay and any text input keep their
  meaning. Capture does not bind `Ctrl+←`/`Ctrl+→`; macOS reserves those for
  switching Desktops. The remaining global bindings keep their
  usual meaning: `Tab`/`Shift+Tab` cycle selectable controls inside the active
  session and `Alt+Arrow`/`Alt+h j k l` move between DataFields.
- Both sessions share one Capture root, one configured index filename, and one
  scan result. A single load feeds Scan and Route, so `R` in either session
  refreshes both, and choosing a new root in either session moves both to it
  and triggers one shared reload.
- Route purpose: organize each scanned Capture by choosing a Recipe, enabling
  the Actions to run, and supplying whatever those Actions still need. This
  session records the plan only; nothing is copied, moved, or deleted, and no
  Action is executed.
- The underlying data abstraction — Recipe, Action, and Missing Fields — lives
  in [`organizer.md`](organizer.md). Route holds no domain logic of its own: it
  calls `FindRecipes`, `MissingFields`, and `Build`, and draws the result. This
  document describes only the TUI over that model.

## Layout

Route uses the shared equal four-column landscape skeleton: four full-height
columns of `1/4` each, one Fieldset per column, left to right `CAPTURES`,
`RECIPES`, `ACTIONS`, `FIELDS`. The four regions are peers in the organizing
task, so none of them is widened over the others. Remainder cells from an
uneven division go to the leftmost columns. Because four columns need more
width than Scan's three, Route shows its resize prompt on narrower terminals
than Scan does.

`CAPTURES` narrows `RECIPES`, and the chosen Recipe fills `ACTIONS` and
`FIELDS`. The last two are peers rather than a further narrowing: `ACTIONS` is
where the plan is shaped and each Action explains itself, `FIELDS` is the one
place everything gets filled in. Moving the `ACTIONS` cursor changes only its
own detail pane; toggling an Action changes what `FIELDS` asks for.

```text
CAPTURES              RECIPES            ACTIONS                    FIELDS
                      candidates         the plan, one row each     what the enabled
                                                                    actions ask for

  1 ◐               > Location + Daily   [x] ● obsidian.location…   Latitude  -33.76
      → Loc + Daily   Location             Locations/Epping…md      Longitude 151.08
  2 ○               Daily              > [x] ○ obsidian.daily…      Created   2026-…
  3 ○               Archive                Daily/2026-09-09.md    ── ◆ MISSING ─────
─ ◆ ORGANIZED ───                        [ ]   capture.archive       Place name* ___
  4 ● → Daily ×2                                                     Note*       ___
                                       ── ◆ obsidian.daily.append ─
                                         needs  createdAt  2026-…
                                                content  · missing
                                         effect Appends one entry
                                                to the day's note
```

- The left column repeats Scan's arrangement: the scrollable `CAPTURES` list
  extends down to the same compact `CAPTURE ROOT` control pinned to the
  workspace bottom and aligned with the other three columns. Enter or a primary
  click on the Root control opens the same directory-only File Explorer overlay
  Scan uses. Route and Scan share one implementation of this control, so the
  path row, the overlay, its `BROWSE` status, and its Esc behavior are identical
  in both sessions.
- Changing the root from Route clears every Selection along with the capture
  list, because Selections are keyed by Capture path under the old root.
- `CAPTURES` is a flat list of the valid Captures under the current root — no
  expansion and no attachment children, because organizing acts on the Capture
  as a whole. Rows with no Recipe chosen use `○`, rows whose plan is ready use
  `●`, and rows with a Recipe but unmet requirements use `◐`.
- The Captures still to handle come first, then a labelled `ORGANIZED` rule,
  then the ones already organized, each run keeping the load order. An
  organized row is labelled by its most recent pass, with `×n` when it has been
  organized more than once. One list
  with a rule through it rather than two fields: the two runs are the same kind
  of thing in different states, and the cursor should walk from the last
  unhandled Capture into the handled ones without leaving the column. The rule
  is not selectable and does not shift the numbering.
- A Route row identifies its Capture by index content rather than by directory
  name, because generated folder names such as `aaaa-xxxxx` say nothing about
  what to do with the Capture. That identity does not fit one quarter-width
  row, so each Capture occupies two: the formatted `createdAt` on the first,
  and `source.app · source.workflow` on the muted second, followed by
  ` → <recipe name>` once a Recipe is chosen.

  ```text
    1 ● 2026-10-10 12:23:24 +11
        app1 · wf  → Location + Daily
  ```

- The timestamp keeps the offset written in the index rather than being
  converted to local time: whole-hour offsets drop their minutes, UTC reads as
  `Z`, and a `createdAt` that is not valid RFC 3339 is shown verbatim so a
  malformed index stays visible. Absent parts are omitted, and a Capture with
  no `createdAt` falls back to its folder name with a trailing path separator.
  The directory itself stays available in the status bar.
- `RECIPES` lists the candidates `FindRecipes` returns for the selected
  Capture, so the list changes as the Capture cursor moves; a `been_here`
  Capture and a `quick_mark` Capture do not offer the same Recipes. Nothing is
  preselected and no candidate is highlighted as recommended — choosing the
  Recipe is the user's decision, and a lone candidate is still confirmed with
  `Enter`. With no candidate it shows `· No recipe matches this capture`.
- A pane below the list describes the candidate under the cursor: what it
  `Runs`, what it `Asks` for beyond its Actions, what it `Matches`, and its
  `Source` — `built-in` or the filename that defined it. It is there because
  choosing a Recipe resets the enabled Action set, so choosing one must not be
  how its content is discovered.
- `ACTIONS` shows the Action Plan of the chosen Recipe: one row per Action, in
  the order the Recipe lists them, with the resolved target beneath the Action
  name. Each row carries two independent markers, which are never merged:
  `[x]`/`[ ]` for enabled, and `●`/`○` for whether that Action's requirements
  are met. A disabled row is dimmed, shows no readiness marker and no target,
  because it does not enter the plan at all.
- Below the list, a pane describes the Action under the cursor: what it `Needs`
  of this Capture, field by field with the value or `· missing`, and what its
  `Effect` is outside the Capture — what it creates, what it changes, what
  running it twice does. The effects are declared on the Action itself, so this
  pane and the generated reference say the same thing.
- Both detail panes share one fixed height, so neither list resizes as the
  cursor moves between entries that describe themselves at different lengths,
  and the two rules line up across the columns.
- A pane is written as headed sections — a quiet header on its own line, its
  items indented under it — rather than as a hanging key column. A key column
  costs width the content needs, and at a quarter of the screen that showed up
  as truncated field names. A single value small enough to sit beside its key
  still does, where a header of its own would waste a row.

  ```text
  ── ◆ obsidian.location.upsert ──────
  Needs
    Place name*  · missing
    Latitude*    34.66939
    Longitude*   135.50122
  Effects
    Creates Locations/<place name>.md,
      or updates it in place when it
      exists
  ```

- Within a section: names are muted and values are not, so the eye lands on the
  content; the name column is sized to the longest name present rather than
  fixed, so a short list is not padded out to fit one it does not contain; a
  wrapped sentence is indented further than its first line, so it cannot be
  mistaken for the next item; and a field value is clipped rather than wrapped,
  because it is a datum rather than a sentence and `FIELDS` carries it in full a
  column away. Fields are named by the same label `FIELDS` uses, so one screen
  never calls one thing two names.
- This pane is where a field is attributed to the Action that asked for it,
  which is why `FIELDS` does not have to repeat the attribution.

  ```text
    [x] ● obsidian.location.upsert
            Locations/Epping Station.md
    [x] ○ obsidian.daily.append
            Daily/2026-09-09.md
    [ ]   capture.archive
  ```

- `FIELDS` is what the **enabled** Actions ask for, deduplicated and split by
  state: the fields that already have a value, then a `MISSING` rule, then the
  ones still to supply. A field two Actions both require appears once, so it is
  filled once and satisfies both. Toggling an Action off takes the fields only
  it required out of both halves; toggling it back on brings them back.
- The split is by state, not by importance: above the rule is what the Capture
  can already answer — from its own data, from a composed value, or from
  something supplied earlier — and below it is the work left. A value the
  Capture itself carries is muted and read-only; a composed or supplied one
  stays editable, so a composed place name can be replaced. It uses the same
  fixed 12-cell key column as Capture Info so values never shift as the cursor
  changes.
- An optional field with no value sits below the rule too, unmarked. Only the
  `*` rows block the plan, so a `MISSING` list of unmarked rows is still ready
  to run.
- The row under the cursor is marked with the shared selected-row style, the
  same one the lists in the other columns use, rather than with a marker
  character alone: a read-only row is muted, and a muted line paints over a
  lone marker.
- At a quarter of the width `FIELDS` renders `text` inline. A `multiline` field
  is a paragraph and does not fit a quarter column, so it is edited in an
  overlay over the workspace, titled by the field. `datetime` and
  `multi_select` do not fit either and are not editable yet; the Recipes that
  need them show the requirement and report the Action as blocked.
- The Capture's own summary — ID, files, attachments — no longer has a column.
  Its resolved values appear in `FIELDS` alongside the values being supplied,
  which is the same Context read from both sides, and its path stays in the
  status bar.

## Keyboard behavior

- `CAPTURE ROOT`: `Enter` opens the File Explorer overlay; `Esc` closes it
  without changing the root.
- `Enter` advances one column to the right, and `Esc`, `Backspace`, or `Delete`
  returns one column to the left, so the four columns are walked as one
  progressive selection. Route owns those keys in every column but `CAPTURES`.
- From `CAPTURES` there is nowhere further left: `Esc` falls through to the
  shell and leaves the command, so a half-made selection is never abandoned by
  one stray press; `Backspace` and `Delete` are inert there, because a key held
  down while walking back should not fall out of the session.
- `CAPTURES`: `↑/k` and `↓/j` move, `h/l` pan, `g g`/`G` jump to the first and
  last Capture, `Enter` moves focus to `RECIPES`, and `u` clears the whole
  Selection for the current Capture. Clearing is `u` alone: `Backspace` walks
  back a column everywhere else, and one key with two meanings on one screen is
  how a Selection gets cleared by accident.
- `RECIPES`: `↑/k` and `↓/j` move, `Enter` chooses the Recipe, enables its
  default Action set, and moves focus to `ACTIONS`. Choosing a different Recipe
  replaces the Action set; enrichment already supplied is kept, since it is
  keyed by field rather than by Recipe.
- `ACTIONS`: `↑/k` and `↓/j` move, `space` toggles the focused Action, and
  `Enter` moves focus to `FIELDS`. Disabling every Action is allowed; the
  Capture then has an empty plan and counts as blocked, never as ready.
- `FIELDS`: `↑/k` and `↓/j` move between rows and `Enter` edits the focused
  editable row. A `text` row is edited in place and committed with `Enter`; a
  `multiline` row opens the overlay, where `Enter` inserts a newline and
  `ctrl+s` commits. `Esc` abandons an edit; `Backspace` and `Delete` edit the
  text while an editor is open and only walk back once it is closed. Committing does not move focus away, so a value can be revised
  immediately.
- The `RUN` dialog is a confirmation, not a report: `x` opens it and nothing has
  happened yet, `Enter` or `y` carries the plan out, and `Esc` or `q` cancels
  with nothing done. The asymmetry decides this — confirming costs one keystroke
  each time, while a mistaken `x` would write into a vault. A blocked plan
  cannot be confirmed and says so instead of offering to run. It is rendered generically from the
  plan — the target each Action resolved, then the value of every requirement
  that Action declares — rather than from per-Action display code, so a new
  Action needs no dialog work to appear in it.

  The dialog reports the Capture it ran, not whichever one the cursor moved on
  to afterwards.

  ```text
  ╭─ RUN ──────────────────────────────────────────────╮
  │ 2026-09-01-note-osaka  ·  Location + Daily         │
  │                                                    │
  │ ● 1 obsidian.location.upsert                       │
  │       target      Locations/Chuo, Osaka.md         │
  │       effect      Creates it, or updates it in…    │
  │       Place name  Chuo, Osaka                      │
  │ ○ 2 apple.reminders.create                         │
  │       target      · unresolved                     │
  │       Due         · missing                        │
  │                                                    │
  │ Nothing has happened yet. No Action is             │
  │ implemented, so running this writes only           │
  │ organize.json.                                     │
  │ ↵ Run   esc Cancel                                 │
  ╰────────────────────────────────────────────────────╯
  ```

- `x` runs the selected Capture's plan from any DataField once a Recipe is
  chosen. No Action is implemented, so running opens a `RUN` dialog naming each
  enabled Action, the target it resolved, and the value of every field it would
  carry. A blocked Action is listed with `○` and its missing values read
  `· missing`. `Esc` closes the dialog.
- Confirming a ready plan appends to the organizer's record in the Capture
  directory, moves that Capture below the `ORGANIZED` rule, and puts the cursor
  on the next Capture still to handle. Focus returns to `CAPTURES`, so a run
  of Captures is worked through without walking back up the columns. A blocked
  plan records nothing: a Capture counts as handled only once its plan could
  actually run.
- `R` refreshes the Capture list from any DataField.
- Every column is reachable with the pointer as well as the keyboard. A primary
  click selects a row and focuses its Fieldset, and the wheel scrolls the list
  under the pointer without changing focus.
- A double click on a row does what `Enter` does in that column, so the pointer
  alone can drive the session: on a Capture it moves to `RECIPES`, on a Recipe
  it chooses it, on an Action it moves to `FIELDS`, and on an editable field row
  it opens its editor. Splitting select from act this way means a stray click
  never chooses a Recipe, which would reset the enabled Action set.
- The exception is an Action's `[x]`: a press on it toggles that Action and does
  not pair into a double click, because toggling twice would mean nothing.
- The four Fieldsets sit in one row, so `Alt+L`/`Alt+Right` and
  `Alt+H`/`Alt+Left` walk `Captures ↔ Recipes ↔ Actions ↔ Fields` without the
  progressive-selection semantics of `Enter`. `Alt+J`/`Alt+Down` from Captures
  reaches the Capture Root below it, and `Alt+K`/`Alt+Up` returns.
  `Tab`/`Shift+Tab` cycle
  `Captures → Capture Root → Recipes → Actions → Fields`.
- The status bar center shows the selected Capture path, plus ` → <recipe>`
  once one is chosen; with nothing selected it summarizes
  `<ready>/<total> READY · <n> BLOCKED`. An already organized Capture counts as
  ready whether or not this session chose its Recipe, so the summary matches
  what the rule in `CAPTURES` shows.

## Configuration

Recipes are not configured per session. They are defined by the organizer model
in [`organizer.md`](organizer.md), and Route renders whichever candidates the
Set returns. The Set is the built-in Recipes with the files in
`capture.recipes` layered over them:

```json
{
  "capture": {
    "scan": { "root": "~/Captures", "index_file": "index.json" },
    "recipes": "~/.config/dgs/recipes"
  }
}
```

An unset `recipes` means the `recipes` directory beside the configuration file.
A missing directory is not an error; it is the normal state of an installation
that has defined no Recipe of its own. The paths Actions write to are not
configurable yet.

Route therefore has no configuration of its own beyond the Capture root and
index filename it shares with Scan. A Capture whose workflow matches no Recipe
shows an empty `RECIPES` column rather than an error — it is simply not
organizable yet.

Work deferred out of this session is listed in [`roadmap.md`](roadmap.md).
