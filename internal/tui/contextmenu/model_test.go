package contextmenu

import "testing"

func TestClickSelectsMenuItem(t *testing.T) {
	m := New(Item{ID: "open", Label: "Open"})
	m.Open(4, 3)
	item, ok := m.Click(5, 4)
	if !ok || item.ID != "open" || m.IsOpen() {
		t.Fatalf("item=%#v ok=%v open=%v", item, ok, m.IsOpen())
	}
}
