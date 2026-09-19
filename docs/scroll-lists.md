# Scroll List

## Purpose

`internal/tui/scrolllist` is the shared component for long, selectable collections. It owns viewport position, selection, horizontal pan, line-number width, and rendering; apps provide stable item IDs and display labels.

Rows use one-based, right-aligned line numbers. The width grows with the collection (`1`–`9`, `10`–`99`, and so on), keeping labels vertically aligned. The selected row uses both the shared accent marker and a restrained full-row background highlight when its containing data field is focused.

### Line numbers

`HideNumbers(true)` drops the number column, and `LabelOffset` shrinks with it
so a row's own controls stay where the app puts them. Use it only for a list
whose rows already carry their own structure: a tree draws depth into the
label, numbers every visible row rather than every node, and renumbers
everything below a fold as it opens and closes, so the column competes with the
structure instead of locating anything. `dgs conf inspect`'s Nodes index is the
current example. A plain collection keeps its numbers — they are what makes a
row easy to point at.

An item may carry an optional second line. When any item in a list has one,
every item occupies two rows: the label, then a muted detail row indented to
the label column. Two-row mode keeps rows uniform so the cursor never changes
height as it moves, halves how many items the viewport holds, scrolls and
selects by item rather than by row — either row of an item selects it — and
moves the `↓ more` marker to the detail row, which is the item's last rendered
line. Use it when one row cannot hold an item's identity at the column width
the screen allocates; Capture Route is the current example.

### Inline details

`InlineDetail(true)` puts the detail beside the label instead of under it,
muted and in a column starting after the longest label, so the details line
up. The item stays one row, so the viewport holds twice as many.

Use it when the detail is a short qualifier rather than a second identity: a
role and a machine, a count, a state. The colour already tells it from the
label, and a row each is a row spent on a phrase. `dgs conf inspect`'s
indexes are the current example — they are trees, where a row per qualifier
halves how much of the tree is on screen.

The label column is capped at two thirds of the width so one long label
cannot leave the details nothing to be drawn in. A label past the cap keeps
its whole text and takes the gap instead: truncating the name of a thing is
worse than losing the alignment of what qualifies it.

A detail wider than the room left ends in `…` rather than mid-word, and one
with fewer than four cells to be drawn in is left out: a cut word such as
`1 creden` reads as a mistake, and the detail pane still says it in full.

## Divider

`SetDivider(index, label)` draws the shared anchored divider above the item at
`index`, labelled,
splitting the list into two runs without turning the rule into an
item: it is never selectable, it does not shift the numbering, and a click on it
selects nothing. It is the same `── ◆ LABEL ───` rule used between sections
elsewhere, so a split list reads as one familiar thing rather than a list with a
line drawn through it. An index at either end of the list removes the rule, because a rule
with nothing on one side of it separates nothing. The rule costs one row from
the viewport whether or not it is currently scrolled into view, so the number of
visible items does not change as the list moves.

Use it where the two runs are the same kind of thing in different states, and
the cursor should walk from one into the other — Capture Route separates the
Captures still to handle from the organized ones this way, rather than putting
them in two data fields the cursor has to leave the column to reach.

## Interaction

- `j/k` or Down/Up moves selection and follows it with the viewport.
- `gg` and `G` move to the beginning and end.
- `h/l` or Left/Right pans long labels.
- A primary click selects the row and focuses its containing data field.
- `LabelOffset` reports how many cells precede an item's label, so an app that
  renders a control into the label — Capture Route's Action checkboxes — can
  tell which part of a row was clicked.
- The wheel scrolls the hovered list without changing data-field focus.
- Apps may attach a contextual primary action such as Quick Look to Space.

Inventory headers, fieldset legends, and app-specific row metadata remain outside the component.

## Context Menu

`internal/tui/contextmenu` is the shared anchored-menu component. Apps supply action IDs and labels, open it at a pointer position, and execute the selected action. It supports keyboard movement, Enter, Escape, and primary-click selection. Menus clamp to the workspace edge and render above the underlying workspace without changing its layout.

Right-clicking a list row may select that row for the contextual action without changing the containing data-field focus. The menu action belongs to the app: Photo Import currently contributes Quick Look on Space and `Open with default app` in the menu, while the shared list and menu contain no macOS-specific behavior.
