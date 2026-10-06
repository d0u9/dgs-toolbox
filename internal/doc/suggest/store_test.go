package suggest

import (
	"dgs-toolbox/internal/doc/tree"
	"errors"
	"testing"
	"time"
)

// testStore opens a constructed synthetic fixture, initializing its marker
// when the original fixture supplied definition files without a marker.
func testStore(t testing.TB, root string) *tree.Store {
	t.Helper()
	store, err := tree.Open(root)
	if errors.Is(err, tree.ErrNotATree) {
		if err := tree.Init(root, time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
		store, err = tree.Open(root)
	}
	if err != nil {
		t.Fatal(err)
	}
	return store
}
