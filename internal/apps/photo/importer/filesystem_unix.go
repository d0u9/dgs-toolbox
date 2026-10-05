//go:build darwin || linux

package importer

import (
	"os"
	"syscall"
)

// deviceOf returns the filesystem device a file lives on.
func deviceOf(info os.FileInfo) (uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(stat.Dev), true
}
