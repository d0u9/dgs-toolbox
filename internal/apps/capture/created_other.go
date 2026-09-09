//go:build !darwin

package capture

import (
	"os"
	"time"
)

func fileCreatedAt(os.FileInfo) (time.Time, bool) { return time.Time{}, false }
