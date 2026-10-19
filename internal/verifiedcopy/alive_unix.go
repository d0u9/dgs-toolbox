//go:build unix

package verifiedcopy

import (
	"errors"
	"syscall"
)

// processAlive reports whether pid names a running process. Permission
// denied means it runs as another user: alive.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
