package skipdirs

import "testing"

func TestSkip(t *testing.T) {
	for _, name := range []string{".Trashes", ".Trash", ".Trash-1000", "#recycle", "#snapshot", "@eaDir", "$RECYCLE.BIN", ".snapshot", ".zfs", "@Recycle", ".Spotlight-V100", ".fseventsd"} {
		if !Skip(name) {
			t.Errorf("%s not skipped", name)
		}
	}
	for _, name := range []string{"DCIM", "20261003", "recycle", "snapshot-trip", ".hidden-library", "Trash photos"} {
		if Skip(name) {
			t.Errorf("%s skipped", name)
		}
	}
}
