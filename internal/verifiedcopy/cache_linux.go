//go:build linux

package verifiedcopy

import (
	"os"

	"golang.org/x/sys/unix"
)

// bypassCache has no per-handle equivalent on Linux without O_DIRECT, whose
// alignment rules a plain read loop cannot meet; dropCache does the work.
func bypassCache(*os.File) error { return nil }

// dropCache evicts the file's clean pages (POSIX_FADV_DONTNEED). Called after
// the write has been synced and again on the readback handle, it makes the
// readback come from the filesystem rather than the page cache.
func dropCache(file *os.File) error {
	return unix.Fadvise(int(file.Fd()), 0, 0, unix.FADV_DONTNEED)
}
