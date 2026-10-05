//go:build darwin

package verifiedcopy

import (
	"os"

	"golang.org/x/sys/unix"
)

// bypassCache asks macOS not to keep this file's data in the unified buffer
// cache (F_NOCACHE). Set on the copy's write handle, the written pages are not
// retained, so the readback handle — set the same way — reads what the
// filesystem returns rather than memory this process just filled.
func bypassCache(file *os.File) error {
	_, err := unix.FcntlInt(file.Fd(), unix.F_NOCACHE, 1)
	return err
}

// dropCache has nothing to add on macOS: bypassCache already keeps the pages
// out of the cache.
func dropCache(*os.File) error { return nil }
