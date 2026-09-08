# Parameter Controls Design

## Purpose

Commands that collect parameters use the shared form components in `internal/tui/form`. Read [`tui.md`](tui.md) first for global layout and keyboard conventions. Read [`file-explorer.md`](file-explorer.md) when a path control opens File Explorer.

For forms placed inside multi-region workspaces, read [`data-fields.md`](data-fields.md) for `Alt+h/j/k/l` movement and inactive-field focus presentation.

This document defines reusable controls only. Commands own their page composition, labels, values, workflow, and side effects. App-specific examples belong under `docs/apps/<app>/`; see [`apps/photo/import.md`](apps/photo/import.md).

## Source and destination paths

```text
› Source       /path/to/source
  Destination  /path/to/destination
```

Source and Destination are focusable path controls and pressing `Enter` opens the shared File Explorer. Their placement within a command's workflow belongs to that command's design.

## Parameter region

The shared library provides reusable fieldsets. A fieldset is a fully closed rectangular border with its legend embedded in the top edge, analogous to HTML `fieldset` and `legend`. The shared component lives in `internal/tui/fieldset`.

The shared form component owns field definitions, focus order, type-specific rendering, current display values, and navigation. Each command chooses its own fieldset grouping, page structure, labels, and workflow transitions.

### Fieldset geometry

The `internal/tui/fieldset` component owns border construction. It must:

- Render a complete rectangle with left, right, top, and bottom edges.
- Embed the legend in the top border rather than placing a separate heading above it.
- Produce rows of one consistent terminal-cell width, including ANSI-styled content.
- Truncate content that exceeds the available inner width and pad shorter rows so the right edge remains aligned.
- Accept a resolved width from the caller and shrink content correctly within it.

Commands provide only the legend, rendered child content, and resolved width. They must not draw their own fieldset borders. The shared shell offers a 100-cell preferred configuration width, but each command decides how fieldsets are arranged within its surface.

### Anchored Section Divider

Use the shared Anchored Section Divider to separate semantic sections inside one fieldset without introducing another nested border:

```text

── ◆ ─────────────────────────────────────
```

The component lives in `internal/tui/divider`. Its visual contract is:

- Leave one empty row above the divider so it does not attach to the preceding control.
- Begin with a short rule and a diamond anchor, then extend a quiet rule through the remaining width.
- Give the diamond the shared accent color and keep the line lower contrast.
- Resolve the line to the fieldset's inner width so its right edge remains aligned.
- Use it only between meaningful sections, not between individual fields.

The caller owns the empty row because vertical density may vary by composition; the divider component owns only the one-row rule.

## Control vocabulary

The shared control vocabulary includes:

```text
  Mode            Standard  ▾              Option
  [x] Include children                      Checkbox
  Label           Example                   Text
  Strategy        (●) First   ( ) Second    Radio
  Extensions      [x] JPG                    Multi-checkbox
                  [ ] DNG
  [ Continue ]                              Button
```

Use the same `›` focus marker and accent color as the rest of the TUI. The focused control receives a restrained full-row background highlight, including its marker, label, and value; this makes the current row immediately scannable without adding another border. Keep labels aligned where a label/value structure applies. The fieldset is the grouping boundary; do not wrap every control in its own border. Buttons use brackets to communicate their distinct action role.

## Navigation

Commands define which visible controls participate in a focus order. Within a form, shared navigation is:

```text
Tab / Down / Right / j / l        Move to the next control
Shift+Tab / Up / Left / k / h     Move to the previous control
Enter / Space                     Open a focused Option
```

Navigation wraps at the beginning and end. `Enter` opens File Explorer for paths, opens an option menu, begins text editing, advances a radio choice, or activates a button according to the focused control. `Space` toggles a checkbox. Left/Right or h/l changes a focused radio choice. An open option menu uses Up/Down and Enter; `Esc` cancels it. Text editing uses Enter to apply and `Esc` to restore the previous value.

While editing text, printable keys—including `q` and the Vim navigation letters—belong to the editor instead of form navigation.

Primary-click interaction mirrors the keyboard behavior. Clicking a control focuses and operates it: Checkbox toggles, Option opens its choices, an Option choice applies, Radio selects the clicked choice, Path opens File Explorer, Text begins editing, and Button activates. Clicking a fieldset's padding or border may focus its data field but does not operate a child control.

### Interaction states

Only one control may be focused or actively editing at a time. Opening a selector or editor temporarily suspends form navigation.

- Option: `Enter` or `Space` opens an inline list; Up/Down changes the highlighted candidate; `Enter` applies it; `Esc` restores the previous value.
- Checkbox: `Space` or `Enter` toggles immediately.
- Text: `Enter` begins inline editing and shows a cursor; `Enter` applies; `Esc` restores the previous value.
- Radio: Left/Right or h/l changes the selection; `Space` and `Enter` advance it.
- Multi-checkbox: options are supplied by the owning command and each renders on its own aligned row. It may expose a leading `[ All ]` bulk action. From the parent row, Right/l or Enter enters the subitems. Inside, Up/Down or k/j moves vertically, Space or Enter invokes the current action or checkbox, and Left/h or Esc returns to the parent form. A mouse click invokes the clicked row and enters that group. Internal multi-checkbox position is independent from an Option menu's candidate cursor; clicking another control closes an open Option without applying its unconfirmed candidate.
- Button: `Enter` activates it; the owning command defines the result.
- Path: `Enter` opens File Explorer as a temporary overlay.

The status bar must describe the active interaction instead of continuing to show general navigation hints. Selector state uses `SELECT` and text editing uses `EDIT`; command-specific states belong in that command's design.

### Next-screen shortcut

`n` is the shared **Next Screen** shortcut. In normal form navigation it advances using the current values, invoking the exact same command-owned transition as the screen's primary button without moving focus or changing parameters.

Temporary interactions take precedence: `n` must not advance while a selector, text editor, File Explorer, or another component owns input. A screen with a next transition shows `n Next` in its normal status-bar hints. Screens without a next transition neither advertise nor handle it.

## Component boundary

Reusable controls must not depend on an app or domain package. New commands compose shared field definitions and retain responsibility for domain validation, workflow transitions, and side effects.

Do not add separate navigation implementations for each command. Extend the shared form component when a new generic control behavior is confirmed.
