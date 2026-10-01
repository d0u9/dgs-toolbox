package organizer

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
		if got := insertUnderSection(c.note, "去过哪里", entry); got != c.want {
			t.Errorf("%s:\ngot  %q\nwant %q", c.name, got, c.want)
		}
	}
}
