//go:build darwin

package capture

import (
	"os"
	"syscall"
	"time"
)

func fileCreatedAt(info os.FileInfo) (time.Time, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec), true
}
