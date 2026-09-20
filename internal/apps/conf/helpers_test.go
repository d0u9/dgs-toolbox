package conf

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// press sends each key as runes — enough for the single-character keys
// (space, x, o, w, g) these tests use. Named keys (arrows, home, end) go
// through pressKey instead.

// openHolders expands every node and unmanaged user. A holder starts folded,
// so a test that reads an instance row opens the tree first, the way a reader
// does with `l`.
func openHolders(m InspectModel) InspectModel {
	for _, n := range m.nodes {
		n.expanded = true
	}
	m.refresh()
	return m
}
