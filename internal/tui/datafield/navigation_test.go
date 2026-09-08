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
