//go:build darwin

package verifiedcopy

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameExclusive(from, to string) error {
	if err := unix.RenamexNp(from, to, unix.RENAME_EXCL); err != nil {
		return &os.LinkError{Op: "renamex_np", Old: from, New: to, Err: err}
	}
	return nil
}
