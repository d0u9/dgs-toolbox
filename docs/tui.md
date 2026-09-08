# TUI Design

## Purpose

This is the evolving interface specification and shared design language for all interactive commands in `dgs`. Add decisions incrementally. Undecided details are not requirements.

All command TUIs should follow this document unless a confirmed command-specific requirement says otherwise. Store those command-specific decisions under `docs/apps/<app>/`; see [`apps/photo/import.md`](apps/photo/import.md) for the current concrete example.

## Example command entry

```text
dgs demo
dgs photo
dgs photo import
dgs photo encode
```

- `dgs demo` opens the component gallery directly.
- `dgs photo` opens a picker scoped to Photo commands.
- `dgs photo import` opens Photo Import directly.
- Leaving Photo Import returns to a command picker so another command can be selected.
- Only one command is active at a time.

## Shared screen frame

Every interactive command uses the shared `dgs` shell:

```text
Top bar        exactly one terminal row
Workspace      all remaining rows
Status bar     exactly one terminal row
```

### Top bar

- Left: the current command path, such as `dgs › photo › import`.
- Right: the current local time.
- The row must never wrap.

### Workspace

The active command owns the workspace. The shell supplies the available width and height after reserving the top and bottom rows.

Command configuration surfaces prefer a shared content width of 100 terminal cells, defined by `tui.DefaultContentWidth`. Center that surface in wider terminals and shrink it to fit narrower terminals; never force a 100-cell layout beyond the available viewport.

### Status bar

- Left: current command state.
- Center: workflow steps when a command has a multi-step flow; otherwise a contextual summary.
- Right: contextual key hints.
- The row must never wrap.

A workflow stepper presents the complete sequence with `›` separators and renders the current step in bold with an underline, so progress is not communicated by color alone. Commands provide the labels and current index; the shell owns placement and truncation. Photo Import is the first consumer, not the source of a mandatory workflow for every command.

## Keyboard convention

Wherever arrow-key navigation is available, support the equivalent Vim navigation keys:

```text
Left / h       Move left or toward the parent
Down / j       Move down or to the next item
Up / k         Move up or to the previous item
Right / l      Move right or toward children
```

The exact action remains contextual, but arrow keys and their Vim equivalents must behave identically in navigation mode.

Printable keys belong to the active text input while editing. In text-entry mode, `h`, `j`, `k`, and `l` insert characters and must not trigger navigation. Contextual help and the status bar should show compact hints such as `↑/k`, `↓/j`, `←/h`, and `→/l` where those controls are relevant.

Multi-region workspaces use `Alt+h/j/k/l` for spatial movement between data fields. Read [`data-fields.md`](data-fields.md) before changing field-level focus, spatial navigation, or focused fieldset presentation.

Read [`scroll-lists.md`](scroll-lists.md) before changing numbered viewport lists or contextual right-click menus.

## Command parameter forms

The following form language applies to every command that collects parameters. Command-specific compositions are documented under `docs/apps/<app>/`.

Read [`parameter-controls.md`](parameter-controls.md) before changing reusable controls, fieldsets, focus navigation, or form interaction. Keep detailed component decisions there.

## Shared parameter form language

All command parameters use the same vertical form pattern. Each row contains a label and a value. Labels use a consistent width within the form, and values begin at the same column.

```text
› Input       /path/to/input
  Output      /path/to/output
  [x] Include children
  Mode        Standard
```

Use one focus at a time. Keep the visual vocabulary small:

```text
  Normal
› Focused
! Error
· Disabled or informational
```

Focus uses the `›` marker and one shared accent color. Do not give every row a border or introduce a different focus style for each parameter type. Meaning must remain understandable without color.

### Path values

A path appears as a normal field row:

```text
› Input       /path/to/input
```

Pressing `Enter` opens the shared File Explorer described below. The parameter form remains behind the temporary selector. The owning command configures whether directories or files are selectable.

Choosing or cancelling closes the File Explorer and returns to the parameter form. A later iteration may consider path completion and recent paths; do not add a second path-entry method yet.

### Single-choice values

A single-choice field shows its current value in the form:

```text
› Mode        Standard
```

Pressing `Enter` opens a temporary selection list. Choosing an item returns to the form. Do not change consequential values silently with left/right keys.

### Checkbox values

Display boolean controls consistently as checkboxes:

```text
› [x] Include children
```

Pressing `Space` or `Enter` toggles the value. Do not mix checkboxes with `Yes/No`, `On/Off`, or `True/False` styles across forms.

### Number and text values

Pressing `Enter` edits number and text values in place. Validate the value when editing finishes and keep the error next to its field.

### Multi-choice values

The form shows a compact summary:

```text
› File types  RAW, JPEG
```

Pressing `Enter` opens a temporary checklist. `Space` toggles an item, `Enter` applies the selection, and `Esc` cancels it.

### Required values and errors

An empty required value initially uses a quiet prompt:

```text
› Source      Required
```

After failed validation, show the error at the affected field rather than only in the status bar:

```text
! Input       /path/to/input
               Path is not readable
```

The status bar may summarize the overall state, such as `INVALID` and `1 parameter needs attention`, but it does not replace the local explanation.

### Primary action

The form ends with one primary action using the same focus marker:

```text
  Mode        Standard

› Continue
```

Primary actions use the shared bracketed button presentation, such as `[ Continue ]`. The owning command defines the action label and effect.

## Form keyboard behavior

Use the same baseline controls on parameter screens:

```text
Up / k         Move focus up
Down / j       Move focus down
Tab            Move to next field
Shift+Tab      Move to previous field
Enter          Edit, open, or continue
Space          Toggle a boolean or checklist item
Esc            Cancel the active edit; otherwise return to commands
?              Show contextual help
q              Quit when not editing text
```

While editing text, printable keys—including `q`—belong to the input. Contextual shortcuts apply only when the field is not capturing text.

The status bar shows only controls valid in the current state. For example, edit mode should describe how to accept or cancel the edit rather than showing form-navigation hints.

## File Explorer

Path fields use the shared, reusable File Explorer component. Each command configures its allowed selection type.

Read [`file-explorer.md`](file-explorer.md) before changing the component, its keyboard behavior, filtering, file operations, path completion, or any command that embeds it. Keep detailed Explorer decisions in that document rather than duplicating them here.
