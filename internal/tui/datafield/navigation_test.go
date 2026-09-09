package datafield

import "testing"

func TestSpatialNavigation(t *testing.T) {
	n := New(Field{ID: "left", Row: 0, Col: 0}, Field{ID: "top", Row: 0, Col: 1}, Field{ID: "bottom", Row: 1, Col: 1})
	n.Set("top")
	for key, want := range map[string]string{"alt+h": "left", "alt+j": "bottom"} {
		n.Set("top")
		if !n.Move(key) || n.Current() != want {
			t.Fatalf("%s focused %q, want %q", key, n.Current(), want)
		}
	}
}

func TestMouseFocusUsesRegisteredBounds(t *testing.T) {
	n := New(Field{ID: "left"}, Field{ID: "right"})
	n.SetBounds("left", Bounds{X: 0, Y: 0, Width: 20, Height: 10})
	n.SetBounds("right", Bounds{X: 21, Y: 0, Width: 20, Height: 10})
	if !n.FocusAt(25, 4) || n.Current() != "right" {
		t.Fatalf("focused %q", n.Current())
	}
	if n.FocusAt(20, 4) {
		t.Fatal("gap unexpectedly focused a field")
	}
}

func TestHitAtDoesNotChangeFocus(t *testing.T) {
	n := New(Field{ID: "left"}, Field{ID: "right"})
	n.SetBounds("left", Bounds{X: 0, Y: 0, Width: 20, Height: 10})
	n.SetBounds("right", Bounds{X: 21, Y: 0, Width: 20, Height: 10})
	n.Set("right")
	if got := n.HitAt(2, 3); got != "left" {
		t.Fatalf("hit %q, want left", got)
	}
	if got := n.Current(); got != "right" {
		t.Fatalf("hit changed focus to %q", got)
	}
}

func TestMovePrefersAdjacentColumnOverSameRow(t *testing.T) {
	// The Capture Scan layout: a root control above the captures list, a
	// full-height preview, and stacked capture/file info on the right.
	nav := New(
		Field{ID: "root", Row: 0, Col: 0},
		Field{ID: "captures", Row: 1, Col: 0},
		Field{ID: "preview", Row: 0, Col: 1},
		Field{ID: "capture-info", Row: 0, Col: 2},
		Field{ID: "file-info", Row: 1, Col: 2},
	)
	for _, tc := range []struct{ from, key, want string }{
		{"captures", "alt+right", "preview"},
		{"root", "alt+right", "preview"},
		{"preview", "alt+right", "capture-info"},
		{"file-info", "alt+left", "preview"},
		{"captures", "alt+up", "root"},
	} {
		nav.Set(tc.from)
		nav.Move(tc.key)
		if got := nav.Current(); got != tc.want {
			t.Errorf("%s + %s = %s, want %s", tc.from, tc.key, got, tc.want)
		}
	}
}
