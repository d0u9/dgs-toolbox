# Capture Archive TUI Design

## Confirmed scope

- `Archive` is the third Capture session, rendered as an `ARCHIVE` tab beside
  `SCAN` and `ROUTE` in the shared top bar. The three are one Capture command
  and only one is visible at a time; `[` and `]` walk them in the order a
  Capture passes through them — it is looked at, organized, and then put away.
- Archive purpose: take Captures out of the Capture root and put them where
  they belong. Filing has two answers, so the session has two folders: a
  Capture that has been organized is **archived**, and one that is not worth
  keeping is **rejected**. Neither is a deletion — a rejected Capture is moved
  out of the way, not destroyed — and neither changes what is inside the
  Capture: the directory is relocated whole, `index.json` and `organize.json`
  with it.
- A move happens as soon as it is asked for, with no dialog in front of it,
  because every move can be taken back: `R` in either destination returns the
  Capture to the Capture root. A confirmation buys nothing an undo does not
  already give, and it costs a keystroke on every Capture in a session whose
  whole job is going through a list of them.
- Archiving is not a Recipe Action. An Action writes a Capture somewhere; this
  moves the Capture itself, and one that has been moved is no longer in the
  root the other sessions read. Keeping it out of the Actions means no Recipe
  can quietly retire a Capture as a side effect of organizing it.
- Scan rejects a Capture too, with the same `Backspace` into the same folder —
  see [`scan.md`](scan.md). An accident is recognised the first time a Capture
  is looked at, and this session is where it is taken back: a Capture Scan sent
  away appears in `REJECT` here, and `R` returns it to the Capture root. Scan
  has its own three-row undo for what it rejected in this run; this session is
  where anything older than that is taken back.
- A Capture that has not been organized is refused the archive: the archive is
  where handled Captures are kept, and one that was never organized would
  arrive there having had nothing done to it. Any Capture can be rejected,
  because deciding a Capture is not worth organizing is exactly the case
  rejection exists for.
- The session shares the Capture root, the index filename, and the one scan
  result with Scan and Route; `R` in `CAPTURES` refreshes all three, and a root
  chosen in any session moves all three. The two destination folders are
  Archive's own, read with the same rule the Capture root is read by — a
  Capture is a Capture wherever it sits — so what they hold is listed rather
  than remembered from this session's own moves.

## Layout

The shared centre-wide three-column skeleton, left `1/4`, centre `1/2`, right
`1/4`. The middle column is what is being read — what this Capture is and what
was done with it — while the columns either side are lists of Captures.

```text
CAPTURES              CAPTURE                             ARCHIVE
                                                          ~/Capture Archive
  1 2026-09-09        Capture  2026-09-09 16:34:35 +10      1 2026-09-09
      Shortcut …      Source   Shortcut · been_here             Shortcut …
─ ◆ ORGANIZED ────    Sitting  Capture root
  2 2026-09-09        Folder   ~/Captures/aaaa-xxxxx       REJECT
      Shortcut …                                           ~/Capture Rejected
                      ── ◆ ORGANIZED ──────────────────      1 2026-09-08
                      2026-09-09 17:02  ·  Location + …          Shortcut …
                        ✓ location.append  06 Location…
                        ✓ daily.append     Daily/2026-…
── CAPTURE ROOT ──                                     ╭────────╮ ╭────────╮
  ~/Captures                                           │Archive │ │Reject ⌫│
                                                       │› Keep  │ │› Aside │
                                                       ╰────────╯ ╰────────╯
```

- The left column repeats Scan's and Route's arrangement: the scrollable
  `CAPTURES` list extends down to the same compact `CAPTURE ROOT` control
  pinned to the workspace bottom, opening the same directory-only File Explorer
  overlay. The list is arranged as Route arranges it — the Captures still to
  organize first, then a labelled `ORGANIZED` rule — because only the run below
  the rule can be archived.
- The right column is the two destinations, `ARCHIVE` above and `REJECT` below,
  each naming its folder and then listing what that folder holds. The folder is
  written above the list rather than put in a control: it is configuration, so
  it is something to read here and not something to change here.
- The three lists are drawn in one of two views, swapped with `v`. The index
  view leads with when the Capture was taken and carries what produced it
  underneath; the folder view leads with the directory name and carries the
  timestamp underneath. A Capture is identified by its index by default,
  because a generated folder name such as `aaaa-xxxxx` says nothing about what
  is in it — but that name is what every other tool on the machine calls it, so
  a reader comparing this screen with a Finder window, a backup or a terminal
  needs the other view too. Neither view drops the other fact: a row answering
  only one of the two questions would send the reader across for every Capture.
  The view belongs to the session rather than to one column, so a Capture moved
  between columns is not renamed by arriving. Route offers the same switch on
  its own Capture list, and the two are one implementation: a Capture is named
  the same way in both, and `v` means the same thing in each.
- A Capture is in exactly one of the three lists, and a move is watched leaving
  one and arriving in the other. That is the whole state of the session: it
  keeps no marks of its own, because nothing is pending.
- A Capture flagged as wrong in Route — see [`route.md`](route.md) — carries
  `⚑` and leads its group in `CAPTURES`, and the centre pane says when it was
  flagged. It is the one decision already made, waiting on a keystroke. It is
  refused the archive, as one nobody organized is, and its `Archive` control
  reads `Flagged`: keeping it would file a mistake with the Captures that were
  handled. `Backspace` rejects it; unflagging it in Route is how it is archived
  after all. The flag travels with the directory, so it still shows in
  `REJECT`.
- The centre `CAPTURE` pane follows whichever list has focus, so it is always
  about the row the next keystroke would move: what the Capture is, which of
  the three folders it is sitting in, and its organizing runs most recent
  first, each Action with its outcome — `✓` ran, `·` had nothing to do, `✗`
  failed — and the target it wrote. A Capture nobody has organized says so
  there, rather than showing an empty group.
- The centre pane also shows the Capture's payload as indented JSON and its
  recorded position. `Shift+Up`/`Shift+Down` or the wheel over the pane scroll
  long details. A position links to Apple Maps and `o` opens it from the
  keyboard; the map option is absent when the index has no coordinates.
- The two controls under the right column are what can be done to the row under
  the cursor: `Archive  a` and `Reject  ⌫` from `CAPTURES`, and the single
  `Restore  R` from either destination, which is the one thing left to do
  there. Two rather than one, because filing has two answers and a single
  button would have to be aimed before it was pressed. A control is lit only
  when pressing it would move something, and says why not when it would not —
  `Not organized`, `No folder`, `· none` — so it answers "can I do this to this
  row?" without it being tried.

## Keyboard behavior

- `CAPTURES`: `↑/k` and `↓/j` move, `h/l` pan, `g g`/`G` jump to the ends. `a`
  or `A` archives the Capture under the cursor and `Backspace`/`Delete` rejects
  it, both at once and on disk. Either case of the letter archives: nothing
  else in this session is typed with a modifier, and a Shift that is on by
  accident should not turn the one key that files a Capture into nothing at
  all. `R` refreshes, as it does in every Capture session.
- `ARCHIVE` and `REJECT`: the same movement keys, and `R` returns the Capture
  under the cursor to the Capture root. A move is taken back where it landed,
  which is the row the reader is looking at when they decide it was wrong. It
  is the same letter as the refresh key it replaces in these two columns,
  because a list of filed Captures is not a scan and has nothing to re-read
  that the move itself did not already update.
- `a` and `Backspace` mean the same thing wherever the cursor is, so a Capture
  in the wrong folder is moved to the right one without going back through the
  Capture root: `a` in `REJECT` archives it, `Backspace` in `ARCHIVE` rejects
  it. Neither key is offered where it would mean "leave it where it is".
- Backspace and Delete stay inside the session rather than falling through to
  the shell, so a key held down while working through a list cannot leave the
  command.
- `Tab`/`Shift+Tab` walk the three lists. The `CAPTURE ROOT` is not in the ring
  — it is a setting rather than a step — and `Alt+Arrow`/`Alt+h j k l` still
  reach it below `CAPTURES`, as they do in Route.
- A primary click selects a row and focuses its Fieldset, the wheel scrolls the
  list under the pointer without changing focus, and a double click does what
  that column's own key does: archive from `CAPTURES`, restore from either
  destination. A move is a change on disk, so a stray single click never makes
  one. One click presses a button, since a button is pressed rather than
  selected.
- What became of the last move is reported in the status bar, where it was
  asked for, rather than in a dialog nobody opened: `aaaa-xxxxx → archive`, or
  `! aaaa-xxxxx: a directory of that name is already archived`.
- Nothing is renamed on the way out. The directory name is what the index and
  the record refer to each other by, so a Capture reads at its destination
  exactly as it read in the root, and a name already taken there is refused
  rather than merged or suffixed: two Captures sharing a folder name is a
  question only the reader can answer.
- The move is a rename where the filesystem allows one and a copy followed by a
  removal where it does not. A copy that fails part way through removes what it
  had written and leaves the Capture where it was, so a failed move is a
  Capture still in one piece in one place.

## Configuration

```json
{
  "capture": {
    "scan": { "root": "~/Captures", "index_file": "index.json" },
    "archive": {
      "root": "~/Capture Archive",
      "reject": "~/Capture Rejected"
    }
  }
}
```

`capture.archive.root` is where organized Captures are kept and
`capture.archive.reject` where the rest are set aside. Both are configuration
and are not chosen in the session: where an installation files things is
decided once, and a picker for it would be a control used on the first run and
never again. A folder that has not been set is not an error — its pane says
which key to write, and the move that would have used it is refused naming that
same key. A folder that does not exist yet is created by the first move into
it. Every key is listed in
[`configuration/capture.md`](../../configuration/capture.md).

Work deferred out of this session is listed in [`roadmap.md`](roadmap.md).

## Path display

The Archive and Reject folder paths wrap onto as many rows as needed above
their lists, using the full inner width. The lists use the remaining rows,
including for scrolling and mouse hit testing. The centre Capture pane puts
`Folder` on its own line and wraps the path below it, preserving the final
directory name rather than clipping it.

If a terminal cannot hold a complete destination path and one Capture row,
Archive shows its resize prompt instead of clipping the path or overflowing.
