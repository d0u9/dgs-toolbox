package timeline

import "testing"

func TestInsertUnderSection(t *testing.T) {
	entry := []string{"- `09:00:00 +10` · here", "    - Australia"}
	cases := []struct{ name, note, want string }{{
		name: "a placeholder gives way to the first entry",
		note: "# 速记\n\n# 去过哪里\n\n1. 无\n",
		want: "# 速记\n\n# 去过哪里\n\n- `09:00:00 +10` · here\n    - Australia\n",
	}, {
		name: "an empty section gets a blank line under its heading",
		note: "# 去过哪里\n\n# 今日活动\n",
		want: "# 去过哪里\n\n- `09:00:00 +10` · here\n    - Australia\n\n# 今日活动\n",
	}, {
		name: "entries already there stay, and the new one follows",
		note: "# 去过哪里\n\n- `08:00:00 +10` · before\n\n# 今日活动\n",
		want: "# 去过哪里\n\n- `08:00:00 +10` · before\n- `09:00:00 +10` · here\n    - Australia\n\n# 今日活动\n",
	}, {
		name: "a placeholder among real entries is an entry",
		note: "# 去过哪里\n\n1. Epping\n2. 无\n",
		want: "# 去过哪里\n\n1. Epping\n2. 无\n- `09:00:00 +10` · here\n    - Australia\n",
	}}
	for _, c := range cases {
		if got := InsertUnderSection(c.note, "去过哪里", entry); got != c.want {
			t.Errorf("%s:\ngot  %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestHeadingText(t *testing.T) {
	cases := []struct {
		line  string
		level int
		text  string
	}{
		{"# 去过哪里", 1, "去过哪里"},
		{"### 去过哪里   ", 3, "去过哪里"},
		{"## 去过哪里 ##", 2, "去过哪里"},
		{"# Learn C#", 1, "Learn C#"},
		{"# #", 1, ""},
		{"  ## indented", 2, "indented"},
		{"#no space", 0, ""},
		{"####### seven", 0, ""},
		{"- # not a heading", 0, ""},
		{"", 0, ""},
	}
	for _, c := range cases {
		if level, text := headingText(c.line); level != c.level || text != c.text {
			t.Errorf("headingText(%q) = %d, %q; want %d, %q", c.line, level, text, c.level, c.text)
		}
	}
}
