# TUI Design

## Purpose

This is the evolving interface specification and shared design language for all interactive commands in `dgs`. Add decisions incrementally. Undecided details are not requirements.

All command TUIs should follow this document unless a confirmed command-specific requirement says otherwise. The current concrete example is `dgs photo import`; Photo Encode remains a placeholder for demonstrating command selection.

## Example command entry

```text
dgs photo
dgs photo import
dgs photo encode
```

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

### Status bar

- Left: current command state.
- Center: contextual summary.
- Right: contextual key hints.
- The row must never wrap.

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

## Command parameter forms

The following form language applies to every command that collects parameters. Photo Import is the first example.

## Photo Import example flow

The proposed stages are:

1. Setup
2. Scan
3. Review
4. Import
5. Result

This sequence is provisional. Review each stage before implementing real import behavior.

## Setup screen

The initial screen establishes context before any file operation occurs.

Workspace content:

```text
PHOTO IMPORT

› Source      /Volumes/LEICA
  Destination ~/Pictures/Photos
  Recursive   Yes
  Duplicates  Skip

  Review import

Review the plan before any files are copied.
```

Design intent:

- Keep the first screen quiet and immediately understandable.
- Make the source and destination visible before scanning.
- Use one primary action at the end of the form: `Review import`.
- Scanning must not copy or modify files.
- Do not begin with a dense file browser or multi-panel layout.

Example status values:

```text
Left:    READY
Center:  No source scanned
Right:   Enter Scan   Esc Commands   ? Help   q Quit
```

Exact labels, colors, spacing, and narrow-terminal behavior may be refined during implementation.

## Shared parameter form language

All command parameters use the same vertical form pattern. Each row contains a label and a value. Labels use a consistent width within the form, and values begin at the same column.

```text
› Source      /Volumes/LEICA
  Destination ~/Pictures/Photos
  Recursive   Yes
  Duplicates  Skip
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

Use `Source` for the path being imported from and `Destination` for the path imported files will be placed in. Do not use `Archive`, because importing does not imply an archival workflow.

A path appears as a normal field row:

```text
› Source      /Volumes/LEICA
```

Pressing `Enter` opens the shared File Explorer described below. The parameter form remains behind the temporary selector. Both Photo Import Source and Destination select directories, not individual files.

Choosing or cancelling closes the File Explorer and returns to the parameter form. A later iteration may consider path completion and recent paths; do not add a second path-entry method yet.

### Single-choice values

A single-choice field shows its current value in the form:

```text
› Duplicates  Skip
```

Pressing `Enter` opens a temporary selection list. Choosing an item returns to the form. Do not change consequential values silently with left/right keys.

### Boolean values

Display booleans consistently as `Yes` or `No`:

```text
› Recursive   Yes
```

Pressing `Space` toggles the value. Do not mix `Yes/No`, `On/Off`, `True/False`, and checkbox styles across screens.

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
! Source      /Volumes/LEICA
               Directory is not readable
```

The status bar may summarize the overall state, such as `INVALID` and `1 parameter needs attention`, but it does not replace the local explanation.

### Primary action

The form ends with one primary action using the same focus marker:

```text
  Duplicates  Skip

› Review import
```

Use `Review import`, not `Import`, because the next step scans and presents a plan before any copy begins. Avoid button brackets unless a consistent button language is introduced later.

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

Path fields use the shared, reusable File Explorer component. Photo Import currently configures both Source and Destination for directory selection.

Read [`file-explorer.md`](file-explorer.md) before changing the component, its keyboard behavior, filtering, file operations, path completion, or any command that embeds it. Keep detailed Explorer decisions in that document rather than duplicating them here.

## Photo Import areas still undesigned

- Scan progress.
- Review layout and file grouping.
- Duplicate and conflict presentation.
- Import progress.
- Cancellation and confirmation.
- Result, empty, warning, and error states.
- Narrow-terminal layout.
- Photo Encode screens.

Do not fill these gaps speculatively.
