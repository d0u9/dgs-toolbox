package verifiedcopy

import (
	"fmt"
	"os"
)

// CheckRoot optionally binds a selected directory to its observed identity.
// Its modification time may change as files are added; its identity may not.
// A selected root may be a symlink, but its target must remain the same.
func CheckRoot(root string, expected os.FileInfo) error {
	if expected == nil {
		return nil
	}
	current, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("destination root unavailable: %w", err)
	}
	if !current.IsDir() || !expected.IsDir() || !os.SameFile(current, expected) {
		return fmt.Errorf("destination root identity changed: %s", root)
	}
	return nil
}
