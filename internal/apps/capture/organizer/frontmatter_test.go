package organizer

import "testing"

func TestAddToListProperty(t *testing.T) {
	cases := []struct{ name, note, want string }{
		{"no frontmatter", "# Day\n", "---\nk:\n  - b\n---\n\n# Day\n"},
		{"key missing", "---\ndate: x\n---\n", "---\ndate: x\nk:\n  - b\n---\n"},
		{"empty flow list", "---\nk: []\nz: 1\n---\n", "---\nk:\n  - b\nz: 1\n---\n"},
		{"empty value", "---\nk:\nz: 1\n---\n", "---\nk:\n  - b\nz: 1\n---\n"},
		{"flow list", "---\nk: [a, \"c\"]\n---\n", "---\nk:\n  - a\n  - c\n  - b\n---\n"},
		{"block list", "---\nk:\n  - a\nz: 1\n---\n", "---\nk:\n  - a\n  - b\nz: 1\n---\n"},
		{"already listed", "---\nk:\n- b\n---\n", "---\nk:\n- b\n---\n"},
		{"unclosed", "---\nk: a\n", "---\nk: a\n"},
	}
	for _, c := range cases {
		if got := addToListProperty(c.note, "k", "b"); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
