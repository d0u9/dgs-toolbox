package photo

var actionHelp = map[string]string{
	"organize": `Move photos in a folder into capture-date folders (YYYYMMDD by default).

Examples:
  dgs photo organize /path/to/photos --dry-run
  dgs photo organize /path/to/photos --recursive --format YYYY/MM/DD
  dgs photo organize /path/to/photos --dest /path/to/sorted

The folder argument is required. Default folder format is YYYYMMDD.
--dry-run prints the plan without moving files; --recursive includes children
but skips hidden directories. --dest changes the parent of the date folders.
Existing date folders require confirmation unless --yes is given.
This operation moves files; it is not JPEG conversion or a backup.`,
}
