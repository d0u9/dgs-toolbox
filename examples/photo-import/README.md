# Photo Import example

The `photo/config.json` paths are synthetic placeholders; choose source and
destination directories for the session before starting Processing.

Incremental starts enabled in Parameters. It checks size and SHA-256 against
files throughout the destination, including capture-date folders, and copies
only files without a confirmed existing copy. Checks can still read old files
from both locations. Same-name files with different bytes follow the selected
conflict policy; a conflict Skip is not proof of a backup.

State is whole-file recovery bookkeeping, not a permanent import catalogue.
Deleting `.dgs-state` does not lose incremental matching. Existing matches stay
where they are; optional date organization moves newly published photos only.
Incremental is a session checkbox, so there is no additional JSON key here.
