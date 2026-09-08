package scrolllist

import "testing"

func TestNumberedViewportNavigationAndMouseSelection(t *testing.T) {
	m := New()
	m.SetSize(30, 3)
	m.SetItems([]Item{{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"}})
	m.Scroll(2)
	if m.Top() != 1 || !m.SelectRow(2) || m.Cursor() != 3 {
		t.Fatalf("top=%d cursor=%d", m.Top(), m.Cursor())
	}
}
