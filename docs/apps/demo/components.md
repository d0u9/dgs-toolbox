# Component Demo Design

## Scope

`dgs demo` is an interactive gallery for reviewing and tuning reusable TUI components. It is a root-level leaf command, opens directly without an intermediate picker, and still runs inside the shared shell.

## Gallery

The current screen demonstrates:

- Shared 100-cell configuration surface and adaptive shrinking.
- Fieldsets with embedded legends and fully closed borders.
- Anchored Section Divider with its required breathing room.
- Path control and the floating File Explorer overlay.
- Option menu, checkbox, radio group, and text editor.
- Button activation with a side-effect-free event result.
- Shared `n` Next Screen shortcut.
- Spatial `Alt+h/j/k/l` navigation between data fields.
- Primary-button focus switching by clicking any gallery fieldset.
- Navigation, `SELECT`, `EDIT`, `BROWSE`, and normal status-bar states.

The path control defaults to `testdata/demo`, which contains side-effect-free placeholder folders for exercising File Explorer.

Every control is interactive. The Demo must not perform persistent or external side effects. Add future reusable visual components here when they are introduced, but keep app-specific components in their owning app documents and screens.
