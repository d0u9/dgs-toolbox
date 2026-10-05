//go:build darwin

package importer

import (
	"golang.org/x/sys/unix"
)

// mountSource names what is mounted at path, such as //user@host/share, so a
// share mounted twice under two names is recognised as one filesystem.
func mountSource(path string) string {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return ""
	}
	return unix.ByteSliceToString(stat.Mntfromname[:])
}
