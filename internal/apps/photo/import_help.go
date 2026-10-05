package photo

const importHelp = `Examples:
  # Open the auxiliary terminal workspace.
  dgs photo import

  # Copy into one destination; incremental checks are enabled by default.
  dgs photo import /path/to/card --dest /path/to/library

  # Keep a complete copy in each destination.
  dgs photo import /path/to/card --dest /path/to/library --dest /path/to/backup

  # Preview selected files and destination inventories without changing files.
  dgs photo import /path/to/card --dest /path/to/library --dest /path/to/backup --dry-run

  # Organize JPG/RAW by capture date in every destination.
  dgs photo import /path/to/card --dest /path/to/library --dest /path/to/backup --organize

  # Import only JPG and DNG (both extensions must occur on the card).
  dgs photo import /path/to/card --dest /path/to/library --ext JPG --ext DNG

  # Keep different-content photos with the same name by adding safe suffixes.
  dgs photo import /path/to/card --dest /path/to/library --duplicates keep-both

  # Use two whole-file workers per destination.
  dgs photo import /path/to/card --dest /path/to/library --workers 2

  # Delete sources only after all destinations hold independently verified copies.
  dgs photo import /path/to/card --dest /path/to/library --dest /path/to/backup --move

  # Remove import state only after a fully successful import and organization.
  dgs photo import /path/to/card --dest /path/to/library --organize --delete-state

  # Disable content matching; use filename conflicts and verified resume state.
  dgs photo import /path/to/card --dest /path/to/library --incremental=false

  # Quote directory paths containing spaces.
  dgs photo import "/path/to/SD Card" --dest "/path/to/Photo Library"

  # Use a chosen configuration directory.
  dgs --config /path/to/config-directory photo import`
