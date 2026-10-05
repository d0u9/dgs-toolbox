//go:build !darwin && !linux

package importer

import "os"

// deviceOf is not available on this platform.
func deviceOf(os.FileInfo) (uint64, bool) { return 0, false }
