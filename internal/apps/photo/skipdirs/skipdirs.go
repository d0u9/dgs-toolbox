// Package skipdirs names directories that photo tools never read as holding
// photos: trash and recycle bins, filesystem snapshots, and NAS or macOS
// metadata stores. A photo found only there is not a durable copy, and one
// kept there is not a photo to import or organize.
package skipdirs

import "strings"

// names compare case-insensitively.
var names = map[string]struct{}{
	".trashes": {}, ".trash": {}, "$recycle.bin": {}, "#recycle": {}, ".recycle": {},
	"@recycle": {}, "#snapshot": {}, ".snapshot": {}, ".snapshots": {}, ".zfs": {},
	"@recently-snapshot": {}, "@eadir": {}, ".@__thumb": {}, ".spotlight-v100": {},
	".fseventsd": {}, ".temporaryitems": {}, ".documentrevisions-v100": {},
}

// Skip reports a directory name to leave out. Freedesktop trash directories
// are ".Trash-<uid>".
func Skip(name string) bool {
	lower := strings.ToLower(name)
	if _, ok := names[lower]; ok {
		return true
	}
	return strings.HasPrefix(lower, ".trash-")
}
