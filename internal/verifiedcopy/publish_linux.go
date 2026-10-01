//go:build linux

package verifiedcopy

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameExclusive(from, to string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE); err != nil {
		return &os.LinkError{Op: "renameat2", Old: from, New: to, Err: err}
	}
	return nil
}
