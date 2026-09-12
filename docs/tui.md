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

- Left: a tab region reserved for command- or workspace-level tabs. Tabs are
  rendered by the shell from the active command's `Tabs()`; a primary click on
  a tab sends that command a `TabSelectedMsg` carrying the tab index, so the
  shell owns hit testing and the command owns what activating a tab means.
  Commands must not bind `Ctrl+Arrow` for tab switching—macOS reserves it for
  switching Desktops. A tab is never narrower than nine cells; its uppercased
  label is centered in that space and longer labels expand the tab rather than
  being truncated. Tabs are separated by one top-bar cell, and an inactive tab
  carries its own quiet background—distinct from both the active tab and the
  top bar—so every click target is visible. A click in the gap selects no tab.
- Right: breadcrumb, disk read/write rate, network upload/download rate, CPU utilization, and local time—in that exact order.
- A typical right-hand sequence is `dgs › photo › import › processing │ DISK R… W… │ NET ↑… ↓… │ CPU … │ 15:04:05`. The breadcrumb anchors the left edge of this region and the clock anchors its right edge.
- The breadcrumb is never rendered on the left; keeping that area free allows the tab system to grow without competing with navigation context.
- The row must never wrap.

Top-bar telemetry belongs to the shared shell rather than individual commands. Sample cumulative system counters asynchronously with the one-second shell tick and derive per-second rates from consecutive samples. Show `--` until two valid samples exist; a failed sample must not block interaction or replace the last valid values. Disk, Network, CPU, and Time are separate fixed-width cells whose values align right inside their cell; changing digits or units must not move adjacent cells or the breadcrumb. Separate cells with a quiet `│`, for example `dgs › photo › import │ DISK R  4.0M/s W  5.0K/s │ NET ↑  2.0M/s ↓  3.0K/s │ CPU  17% │ 15:04:05`. On narrow screens, remove whole CPU, Network, and Disk cells in that order while always retaining the breadcrumb and the clock when Time is enabled; never show a partially clipped metric or let telemetry change workspace height.

Every key of every part of the toolbox is listed in [`configuration/index.md`](configuration/index.md); this section says what the shell does with its own.

The global configuration controls the visibility of Disk, Network, CPU, and Time independently. All four default to enabled; omitting one key preserves that default. The breadcrumb is structural and cannot be disabled. Load JSON from `$XDG_CONFIG_HOME/dgs-toolbox/dgs-config.json` (`~/.config/dgs-toolbox/dgs-config.json` when that variable is unset), falling back to the operating system's user configuration directory at `dgs/config.json`, or from the path in `DGS_TOOLBOX_CONFIG` when set:

```json
{
  "tui": {
    "top_bar": {
      "disk": true,
      "network": true,
      "cpu": true,
      "time": true
    }
  },
  "photo": {
    "import": {
      "state_file": ".dgs-state",
      "source": "",
      "destination": ""
    }
  },
  "capture": {
    "scan": {
      "root": "",
      "index_file": "index.json"
    }
  }
}
```

App-specific settings remain under their app key. `photo.import.state_file` configures the Photo Import state filename and defaults to `.dgs-state`. `photo.import.source` and `photo.import.destination` optionally replace the repository mock paths shown when those values are empty. Detailed behavior belongs to the Photo Import design rather than this shared TUI document.

Setting any value to `false` removes that entire fixed-width cell and its adjacent separator. If Disk, Network, and CPU are all disabled, the shell does not start the system-counter sampler.

Run `dgs --export-config` to create `dgs-config.json` in the current working directory and print its absolute path. An optional positional path writes elsewhere: `dgs --export-config /path/to/config.json` uses that exact file, while an existing directory receives `dgs-config.json`. The command never overwrites an existing file. Export destinations are explicit and independent of `DGS_TOOLBOX_CONFIG`; the environment variable controls where `dgs` loads configuration. The exported file is an editable template: move it to `~/.config/dgs-toolbox/dgs-config.json` or the operating system's global `dgs/config.json` location, or set `DGS_TOOLBOX_CONFIG` to its path, before launching `dgs`. `dgs -c /path/to/config.json …` (or `--config`) loads that exact configuration file and takes precedence over `DGS_TOOLBOX_CONFIG`.

### The configuration directory

Settings that are values live in the configuration file; everything a command reads from disk—recipes, templates, whatever a later command needs—lives under one directory, `config_dir`. It defaults to the directory the configuration file was loaded from, so `--config` selects a whole configuration and not only one file of it.

dgs is a toolbox, so that directory is laid out **by command**:

```text
<config_dir>/
  capture/
    recipes/
    templates/
  photo/
    …
```

A command asks for its own corner rather than for a path of its own in the configuration file: two commands both wanting `templates` is the normal case, not a collision to work around, and adding a command adds no configuration keys. The layout itself is not configurable: `config_dir` moves all of it at once, and a directory that genuinely belongs elsewhere—templates kept beside the vault they belong to—is a symlink. A path per directory would make "where does this installation keep its configuration?" a question with as many answers as there are directories.

### Workspace

The active command owns the workspace. The shell supplies the available width and height after reserving the top and bottom rows.

The primary target is a landscape, normally maximized terminal. Choose one of four shared column skeletons before composing command-specific Fieldsets and DataFields:

| Skeleton | Variant | Width allocation | Wide-screen cap |
|---|---|---|---|
| One column | Centered | One 100-cell content surface, horizontally centered | 100 cells |
| Two columns | Leading narrow | Left `1/3`, right `2/3` | Left never exceeds 90 cells; right receives the remainder |
| Two columns | Equal | Left `1/2`, right `1/2` | Divide the available width equally |
| Three columns | Center wide | Left `1/4`, center `1/2`, right `1/4` | Left and right never exceed 90 cells each; center receives the remaining width |
| Three columns | Trailing wide | Left `1/4`, center `1/4`, right `1/2` | Left and center never exceed 90 cells each; right receives the remaining width |
| Four columns | Equal | Four columns of `1/4` | Divide the available width equally; remainder cells go to the leftmost columns so no two columns differ by more than one cell |

Ratios are initial allocations, not permission to overflow. On a narrower terminal, subtract inter-column gutters first and shrink columns proportionally within their minimum viable content widths. If a screen cannot remain legible, show its existing resize prompt rather than wrap structural regions or add horizontal scrolling to the whole workspace. Exact gutter width, minimum column widths, and responsive collapse behavior are still undecided.

The one-column 100-cell width is defined by `tui.DefaultContentWidth`. It is a preferred maximum, not a forced terminal width: shrink it to the available viewport while retaining outer breathing room where possible. Two-, three-, and four-column screens use the available landscape width rather than being enclosed inside the centered 100-cell surface. Each command chooses the variant that matches its information hierarchy and records that choice in its app-specific design.

The equal four-column skeleton suits screens whose regions are peers rather than a subject with supporting detail: every column is full height, carries one Fieldset, and none is visually primary. It needs more width than the other skeletons, so a command using it shows its resize prompt earlier.

### Tab and the spatial keys

`Tab`/`Shift+Tab` cycle the *selectable* controls of the active session, which
is not the same as all of them. A field with nothing in it yet — waiting on a
choice made in another field — is skipped, because stopping there tells the
reader only that they are somewhere useless; so is a field that holds a setting
rather than a step of the work, since a ring is walked to get through the work
and a setting in it costs a keystroke every time round. `Alt+Arrow` and `Alt+h j k l` are
spatial and are not gated that way: they name a direction, and the field in that
direction is where it is whether or not it is ready.

### App reports

An app may expose non-interactive reports: what it supports, what it loaded, why
a file was rejected. Each becomes a boolean flag on the app's own command
(`dgs capture --recipes`), writes to stdout, and returns without opening the
TUI. Reports are declared in the app registry beside its commands, so the
command hierarchy stays generic and no app is special-cased in the CLI.

A report is written from the registry it describes rather than from a hand-kept
list, so it cannot drift from the behaviour it documents.

### Status bar

- Left: a compact state chip. It identifies the current command stage and, when relevant, its activity state such as `PROCESSING · RUNNING` or `PROCESSING · PAUSED`.
- Center: workflow steps when a command has a multi-step flow; otherwise a contextual summary.
- Right: only the actions valid in the current state, in descending order of immediacy.
- The row must never wrap.

The state chip takes its natural label width plus a small inset; it must not expand to a fixed percentage of a wide terminal. Each of the three status regions has one cell of left and right padding. Position the center content against the midpoint of the entire terminal row, not the midpoint of the leftover region between the side cells. If asymmetric side content would overlap it, constrain the center content within the available gap. The right hint region is content-sized and flush to the terminal edge. At an extremely narrow width, sacrifice the inset before allowing the bar to wrap or overflow.

The center and right regions share one continuous background color. Nested styled content such as the workflow stepper must restore that background after ANSI resets; terminal-default background patches must not appear around bold or underlined spans.

A shared stepper presents the complete sequence as a stateful track: completed steps use `✓`, the current step uses `●`, and forthcoming steps use `○`, joined by `──`. The current step is bold without an underline; the state glyphs keep progress understandable without relying on color. Commands provide the labels and current index; the shell owns placement and truncation. Reuse this component for both workflow state and smaller staged operations such as `COPY → VERIFY → PUBLISH`. Photo Import is the first consumer, not the source of a mandatory workflow for every command.

The status bar allocates enough central space for a short four-step workflow. On narrow terminals, truncate the stepper rather than wrapping or increasing the status bar height.

## Global key bindings

These bindings form the shared navigation baseline for every picker and command screen. A component may give them a more specific contextual meaning, but it must preserve the same direction and exit behavior:

```text
Arrow keys / h j k l       Move the cursor or selection inside the active component
Alt+Arrow / Alt+h j k l    Move focus between DataFields in that direction
Tab                        Move to the next selectable control
Shift+Tab                  Move to the previous selectable control
<CR>                       Confirm or invoke the focused item
Ctrl+C                     Request exit through the shared confirmation dialog
```

Ctrl+C never terminates `dgs` immediately. It opens the shared safe-default confirmation dialog, with No selected. If work is active, the owning command may pause or cancel it as part of the confirmed exit, but it must still use the shared dialog.

`<CR>` means the Enter/Return key. It confirms the currently focused item, opens it when it owns a child interaction, or invokes its primary action. Tab and Shift+Tab traverse selectable controls in forward and reverse order respectively; they do not replace `Alt+Arrow` or `Alt+h/j/k/l`, which remain the spatial navigation keys between DataFields.

### Movement inside the active component

Wherever arrow-key navigation is available, support the equivalent Vim navigation keys:

```text
Left / h       Move left or toward the parent
Down / j       Move down or to the next item
Up / k         Move up or to the previous item
Right / l      Move right or toward children
```

The exact action remains contextual, but arrow keys and their Vim equivalents must behave identically in navigation mode.

Printable keys belong to the active text input while editing. In text-entry mode, `h`, `j`, `k`, and `l` insert characters and must not trigger navigation. Contextual help and the status bar should show compact hints such as `↑/k`, `↓/j`, `←/h`, and `→/l` where those controls are relevant.

### Movement between DataFields

Multi-region workspaces use `Alt+Arrow` and `Alt+h/j/k/l` for spatial movement between data fields. The direction is literal: Left or `h` selects the nearest field to the left, Down or `j` the nearest below, Up or `k` the nearest above, and Right or `l` the nearest to the right. The distance along the requested axis is what decides: horizontal movement always lands in the adjacent column and vertical movement in the adjacent row, with the other axis only breaking ties between equally close fields. Plain arrow keys and `h/j/k/l` remain inside the currently focused DataField. Read [`data-fields.md`](data-fields.md) before changing field-level focus, spatial navigation, or focused fieldset presentation.

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

Local form actions may use the shared bracketed button presentation. Transitions between workflow pages use the shared two-row Page Actions component, which names both the direction and destination; see [`parameter-controls.md`](parameter-controls.md).

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

An application stage with active work may capture the shell's return and exit keys, including Ctrl+C, to present a confirmation before work is interrupted. This is lifecycle behavior owned by that application stage, not a required layout or interaction for idle screens.

### Confirmation dialog

Consequential interruptions use the shared confirmation component in `internal/tui/confirm`. It renders a compact framed dialog with a title, message, optional detail, and two actions. The negative or safe action is focused by default. `Tab` and `Shift+Tab` switch focus, `Enter` invokes the focused action, and `Esc` always cancels. The selected button carries the visual focus treatment; keyboard hints appear once in the dialog footer and are not duplicated in the global status bar.

The status bar shows only controls valid in the current state. For example, edit mode should describe how to accept or cancel the edit rather than showing form-navigation hints.

Every action that quits `dgs`, including `q`, Ctrl+C, picker exit, and a command-owned Quit button, routes through this shared confirmation component. Do not render a separate inline `y/n` prompt or terminate immediately. The dialog keeps the underlying workspace visible, focuses No by default, and places the right-aligned buttons in safe-to-consequential order: No on the left, Yes on the right. Tab or Shift+Tab switches actions, Enter invokes the focused action, and Esc continues. Active work may provide more specific interruption copy, but it still uses the same component and interaction contract.

### Page actions

Workflow transitions use the shared two-row Page Actions component. Its first row names the action and shortcut; its second row describes the destination. Previous and Next are defaults, not mandatory labels: terminal pages may instead expose explicit actions such as `↻ Again  r / New import` and `Quit  q / Exit dgs`. This keeps the destination visible without misrepresenting a restart or exit as backward/forward navigation.

## File Explorer

Path fields use the shared, reusable File Explorer component. Each command configures its allowed selection type.

Read [`file-explorer.md`](file-explorer.md) before changing the component, its keyboard behavior, filtering, file operations, path completion, or any command that embeds it. Keep detailed Explorer decisions in that document rather than duplicating them here.
