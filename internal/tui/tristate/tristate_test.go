package tristate

import "testing"

func TestBox(t *testing.T) {
	for _, c := range []struct {
		total, checked int
		want           string
	}{
		{0, 0, "[ ]"},
		{3, 0, "[ ]"},
		{3, 1, "[-]"},
		{3, 3, "[x]"},
	} {
		if got := Box(c.total, c.checked); got != c.want {
			t.Errorf("Box(%d, %d) = %q, want %q", c.total, c.checked, got, c.want)
		}
	}
}

func TestCount(t *testing.T) {
	checked := map[string]bool{"a": true, "c": true, "d": false}
	if got := Count([]string{"a", "b", "c", "d"}, checked); got != 2 {
		t.Fatalf("Count = %d, want 2", got)
	}
}
