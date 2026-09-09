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

Each column is the result of the selection in the column to its left, so the
work reads left to right and every column narrows the one after it:

```text
CAPTURES              RECIPES            ACTIONS                    FIELDS
                      FindRecipes()      Build() over enabled set   the Action's
                                                                    requirements

  1 ◐               > Location + Daily   [x] ● obsidian.location…   Place name* ___
      → Loc + Daily   Location             Locations/Epping…md      Latitude  -33.76
  2 ○               Daily                [x] ○ obsidian.daily…      Longitude 151.08
  3 ○               Archive                · target unresolved
─ ORGANIZED ─────                        [ ]   capture.archive
  4 ● → Daily ×2
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
- `ACTIONS` shows the Action Plan of the chosen Recipe: one row per Action, in
  the order the Recipe lists them, with the resolved target beneath the Action
  name. Each row carries two independent markers, which are never merged:
  `[x]`/`[ ]` for enabled, and `●`/`○` for whether that Action's requirements
  are met. A disabled row is dimmed, shows no readiness marker and no target,
  because it does not enter the plan at all.

  ```text
    [x] ● obsidian.location.upsert
            Locations/Epping Station.md
    [x] ○ obsidian.daily.append
            Daily/2026-09-09.md
    [ ]   capture.archive
  ```

- `FIELDS` shows the requirements of the Action under the `ACTIONS` cursor —
  not the union across the whole Recipe — so a missing input is attributable to
  the Action that needs it. Values already resolved from the Capture are shown
  as muted read-only rows; unmet requirements are editable slots marked `*`
  when required. Fields the Recipe itself declares are shown under every Action
  and stay required whatever is enabled. It uses the same fixed 12-cell key
  column as Capture Info so values never shift as the cursor changes.
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
- `Enter` advances one column to the right and `Esc` returns one column to the
  left, so the four columns are walked as one progressive selection. Route owns
  `Esc` in every column but `CAPTURES`, where it falls through to the shell and
  leaves the command; a half-made selection is therefore never abandoned by one
  stray `Esc`.
- `CAPTURES`: `↑/k` and `↓/j` move, `h/l` pan, `g g`/`G` jump to the first and
  last Capture, `Enter` moves focus to `RECIPES`, and `u` or `Backspace` clears
  the whole Selection for the current Capture.
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
  `ctrl+s` commits. `Esc` abandons an edit, and from a row it returns to
  `ACTIONS`. Committing does not move focus away, so a value can be revised
  immediately.
- The `RUN` dialog is a preview, not a confirmation: it opens on `x`, reports,
  and closes on `Esc`, `Enter`, or `q`. It is rendered generically from the
  plan — the target each Action resolved, then the value of every requirement
  that Action declares — rather than from per-Action display code, so a new
  Action needs no dialog work to appear in it.

  ```text
  ╭─ RUN ──────────────────────────────────────────────╮
  │ 2026-09-01-note-osaka  ·  Location + Daily         │
  │                                                    │
  │ ● 1 obsidian.location.upsert                       │
  │       target      Locations/Chuo, Osaka.md         │
  │       Place name  Chuo, Osaka                      │
  │ ○ 2 apple.reminders.create                         │
  │       target      · unresolved                     │
  │       Due         · missing                        │
  │                                                    │
  │ Actions are not implemented yet; only              │
  │ organize.json was appended to.                     │
  │ esc Close                                          │
  ╰────────────────────────────────────────────────────╯
  ```

- `x` runs the selected Capture's plan from any DataField once a Recipe is
  chosen. No Action is implemented, so running opens a `RUN` dialog naming each
  enabled Action, the target it resolved, and the value of every field it would
  carry. A blocked Action is listed with `○` and its missing values read
  `· missing`. `Esc` closes the dialog.
- A run whose plan is ready appends to the organizer's record in the Capture
  directory, moves that Capture below the `ORGANIZED` rule, and puts the cursor
  on the next Capture still to handle. Focus returns to `CAPTURES` either way,
  so a run of Captures is worked through without walking back up the columns. A
  blocked plan is shown but records nothing: a Capture counts as handled only
  once its plan could actually run.
- `R` refreshes the Capture list from any DataField.
- Every column is reachable with the pointer as well as the keyboard. A primary
  click selects a row and focuses its Fieldset, and the wheel scrolls the list
  under the pointer without changing focus. Two places where the pointer has an
  unambiguous target act directly: clicking an Action's `[x]` toggles it, and
  clicking an editable `FIELDS` row opens its editor. Choosing a Recipe stays on
  `Enter` — it resets the enabled Action set, so it should not follow from a
  stray click.
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
