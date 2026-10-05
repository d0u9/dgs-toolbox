package photo

const appHelp = `Photo tools: import originals or export JPEG derivatives.

Examples:
  dgs photo
  dgs photo import --help
  dgs photo import /path/to/card --dest /path/to/library --dest /path/to/backup
  dgs photo encode --help
  dgs photo organize /path/to/photos --dry-run

Without a subcommand, open the Photo picker. Import without a source and Encode
open terminal workspaces. Import with a source and Organize are CLI actions
that return after printing their results.`
