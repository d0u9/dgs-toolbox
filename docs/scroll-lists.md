# Scroll List

## Purpose

`internal/tui/scrolllist` is the shared component for long, selectable collections. It owns viewport position, selection, horizontal pan, line-number width, and rendering; apps provide stable item IDs and display labels.

Rows use one-based, right-aligned line numbers. The width grows with the collection (`1`–`9`, `10`–`99`, and so on), keeping labels vertically aligned. The selected row uses both the shared accent marker and a restrained full-row background highlight when its containing data field is focused.

## Interaction

- `j/k` or Down/Up moves selection and follows it with the viewport.
- `gg` and `G` move to the beginning and end.
- `h/l` or Left/Right pans long labels.
- A primary click selects the row and focuses its containing data field.
- The wheel scrolls the hovered list without changing data-field focus.
- Apps may attach a contextual primary action such as Quick Look to Space.

Inventory headers, fieldset legends, and app-specific row metadata remain outside the component.

# Context Menu

`internal/tui/contextmenu` is the shared anchored-menu component. Apps supply action IDs and labels, open it at a pointer position, and execute the selected action. It supports keyboard movement, Enter, Escape, and primary-click selection. Menus clamp to the workspace edge and render above the underlying workspace without changing its layout.
