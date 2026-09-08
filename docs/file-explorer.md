# File Explorer Design

## Purpose

File Explorer is the shared file and directory selector for every interactive `dgs` command. It is a reusable Bubble Tea component rather than an app-owned workspace.

Read [`tui.md`](tui.md) first for the shared shell, keyboard, status-bar, and parameter-form conventions. This document defines the File Explorer-specific behavior.

The component owns:

- Directory loading and cached tree state.
- Focus, scrolling, disclosure, and keyboard navigation.
- Context filters and selection validation.
- File operations and their temporary input or confirmation state.
- Direct path input and completion.
- Tree-row presentation and colors.

The calling command owns the surrounding field label, opens and sizes the floating selector, configures what may be selected, and receives the selected path. It must not start a nested Bubble Tea program.

## Presentation

Display the Explorer as a floating selector over the calling parameter form. It has a title at the upper left and the active filter at the upper right:

```text
╭────────────────────────────────────────────────────────────╮
│FILE EXPLORER · SOURCE                                  DIR │
│~/Downloads                                                 │
│                                                            │
│› ▾ ~/Downloads                                             │
│    ▸ photos                                                │
│    ▸ scans                                                 │
│                                                            │
│- up  •  = root  •  / path  •  a new  •  d del  •  r ren   │
╰────────────────────────────────────────────────────────────╯
```

Use visible indentation to communicate tree depth. Keep expanded ancestors and siblings visible so the current location remains clear.

The three tree markers have distinct meanings:

```text
›  Keyboard focus
▾  Expanded directory
▸  Collapsed directory
```

An empty loaded directory may use `·`, a loading directory `…`, and a directory that could not be read `!`. Focus and disclosure state must remain understandable without color.

Use an adaptive blue-gray panel instead of a black popup. Teal identifies focus and disclosure, normal paths use high-contrast neutral text, and loading or error states use quiet or semantic colors. Preserve the hierarchy in light and dark terminals.

Use separators instead of adding a rectangle around every internal area. Temporary inputs and confirmations appear at the bottom without replacing the visible tree.

## Context filters

The caller configures what may be selected:

```text
DIR          Directories only
JPG, PNG     Files with the listed extensions
ALL FILES    Any regular file
```

Show this compact label at the upper right of the Explorer.

In `DIR` mode, hide files. In a file mode, keep directories visible for navigation, show only matching files, and allow only matching files to be selected. Extension matching is case-insensitive.

Callers choose the appropriate filter. Command-specific filter choices belong in that command's design document.

## Tree navigation

Arrow keys and Vim keys are equivalent:

```text
Up / k         Move focus to the previous visible node
Down / j       Move focus to the next visible node
Right / l      Expand a directory; if expanded, move to its first child
Left / h       Collapse a directory; if collapsed, move to its parent
o              Toggle the focused directory
O              Collapse and focus the focused node's parent
w              Collapse every directory already present in the tree
-              Move the Explorer root up one directory
=              Make the focused directory the Explorer root
Enter          Choose the focused item when valid for the active filter
Esc            Cancel and return to the calling screen
?              Show contextual help
```

Disclosure commands do not confirm a selection. `w` does not load additional filesystem entries.

### Changing the Explorer root

`-` moves the Explorer root to its parent. After loading, focus the previous root as a child. Repeating `-` can move from a nested starting directory to `/`; at `/` it has no effect.

`=` makes the focused directory the new root. If a file is focused, use its containing directory. Rebase indentation at depth zero and automatically expand the new root by one level. Load direct children asynchronously when necessary, but do not recursively expand descendants.

## File operations

```text
a              Create a directory
d              Delete the focused file or directory
r              Rename the focused file or directory
R              Refresh the filesystem tree
```

`a` opens a name input at the bottom. When a directory is focused, create inside it; when a file is focused, create beside it. `r` opens the same input with the current name. Accept only a single valid path segment.

`d` opens a bottom confirmation naming the target. Only `y` deletes it; `n` or `Esc` cancels. Show confirmation controls in this area once—do not repeat them in the Explorer footer or global status bar.

The Explorer root cannot be renamed or deleted. `R` discards cached entries and reloads the root.

After creating, renaming, or deleting, update the current tree in place. Preserve expanded nodes, scroll position, focus context, and surrounding entries. Do not rebuild the root and force the user to navigate back.

While editing a name, printable keys—including `q` and the Vim navigation letters—belong to the input. `Enter` applies and `Esc` cancels without closing File Explorer.

## Direct path input

`/` opens a shell-like path input at the bottom without replacing the tree. Support absolute paths, paths relative to the focused directory, and `~`.

```text
Tab            Complete or select the next candidate
Shift+Tab      Select the previous candidate
Enter          Select a valid completed path
Esc            Cancel and return to the unchanged tree
```

Completion reads only the directory containing the current path fragment; it never recursively scans the filesystem. Match the final fragment fuzzily and case-insensitively. For example, `/tmp/f<Tab>` may offer both `/tmp/1-foo/` and `/tmp/2-far/`.

Show up to five matching candidates immediately above the input. Further `Tab` presses move forward and `Shift+Tab` moves backward. Append a path separator to directory completions. Show hidden entries only when the fragment begins with `.`. File candidates must obey the active filter.

Pasting replaces the prefilled input instead of appending to it. Trim trailing spaces, newlines, and other whitespace from pasted and submitted paths.

## Loading and state

Load children only when their parent is expanded. Do not recursively scan the filesystem. Retain loaded nodes until the tree is refreshed or its root changes.

Distinguish an empty directory, an in-progress load, and a failed load. Loading and filesystem mutations must not block keyboard handling.

The status bar shows controls for the active Explorer state. While a bottom input or confirmation is active, hide duplicate controls from the global status bar.

## Deferred design

The Explorer does not currently provide recursive fuzzy search, zoxide integration, multi-selection, file previews, bookmarks, or recent locations. Do not add these without a confirmed design change.
