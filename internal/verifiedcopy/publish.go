package verifiedcopy

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// Publish gives the verified temporary file its final name without ever
// replacing a file already there.
//
// A plain rename replaces silently, and a check before it leaves a window in
// which another file can appear and be destroyed. So publication asks the
// filesystem itself to refuse: an exclusive rename where the platform has one,
// otherwise a hard link, which fails when the name exists. Only a filesystem
// that supports neither — some network mounts — falls back to check-then-rename,
// which is as much as the user-space API can promise there.
func Publish(temporary, final string) error {
	err := renameExclusive(temporary, final)
	if err == nil || errors.Is(err, os.ErrExist) || !unsupported(err) {
		return err
	}
	if err := os.Link(temporary, final); err == nil {
		// The final name now holds the verified file. A temporary name left
		// behind is removed as stale by the next copy of this name.
		_ = os.Remove(temporary)
		return nil
	} else if errors.Is(err, os.ErrExist) || !unsupported(err) {
		return err
	}
	if _, err := os.Lstat(final); err == nil {
		return fmt.Errorf("%w: %s", os.ErrExist, final)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporary, final)
}

// unsupported reports an error that means "this filesystem cannot do that",
// as opposed to a failure of the operation itself.
func unsupported(err error) bool {
	return errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOSYS) ||
		errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EXDEV) ||
		errors.Is(err, errNoExclusiveRename)
}

var errNoExclusiveRename = errors.New("exclusive rename is not available on this platform")
