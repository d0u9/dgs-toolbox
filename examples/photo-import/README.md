# Photo Import example

The `photo/config.json` paths are synthetic placeholders; choose source and
destination directories for the session before starting Processing.

Incremental starts enabled in Parameters. It checks size and SHA-256 against
files throughout the destination, including capture-date folders, and copies
only files without a confirmed existing copy. Checks can still read old files
from both locations. Same-name files with different bytes follow the selected
conflict policy; a conflict Skip is not proof of a backup.

State is whole-file recovery bookkeeping, not a permanent import catalogue.
Its configured name must start with `.`; keep `.dgs-state` unless a different
hidden state filename is needed. Unrecognized existing contents are preserved
and reported as a state error; content checks still run.
Deleting `.dgs-state` does not lose incremental matching. Existing matches stay
where they are; optional date organization moves newly published photos and
verified old copies that still mirror their Source folders.
Incremental is a session checkbox, so there is no additional JSON key here.

## Command-line copies

Run without the TUI by providing a source and repeating `--dest`:

```sh
dgs photo import /path/to/card --dest /path/to/library --dest /path/to/backup
```

Each destination must already exist, so an unmounted disk is refused rather
than filled on the local disk. Each target independently checks for existing
content, including date folders.
Use `--ext JPG --ext DNG` to select extensions and `--organize` to opt into
capture-date organization. `--move` deletes a source only after every target
copy passes fresh checks. Partial failures retain successful copies and state,
keep affected Move sources, and return a nonzero status.

A destination must support exclusive rename or hard links for safe publication;
otherwise import fails and keeps Source. Directory-sync failures also prevent
new copies from authorizing source deletion.
