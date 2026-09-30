package timeline

import (
	"strings"
	"testing"
	"time"
)

func TestMarker(t *testing.T) {
	day := time.Date(2026, 9, 24, 17, 0, 0, 0, time.UTC)
	if got := Marker(day, "周四"); got != "- *2026-09-24 周四*" {
		t.Fatalf("Marker = %q", got)
	}
}

// A new day goes on top with its marker; the same day joins the marker already
// there; frontmatter and callout stay above; no blank line splits the list.
func TestPrepend(t *testing.T) {
	preamble := "---\ncssclasses:\n  - timeline\n---\n\n> [!note]- 说明\n> 一行。"
	content := preamble + "\n\n- *2026-09-24 周四*\n- b\n"

	same := Prepend(content, "- *2026-09-24 周四*", "- a")
	if want := preamble + "\n\n- *2026-09-24 周四*\n- a\n- b\n"; same != want {
		t.Fatalf("same day =\n%q\nwant\n%q", same, want)
	}
	next := Prepend(content, "- *2026-09-25 周五*", "- c")
	if want := preamble + "\n\n- *2026-09-25 周五*\n- c\n- *2026-09-24 周四*\n- b\n"; next != want {
		t.Fatalf("new day =\n%q\nwant\n%q", next, want)
	}
	if empty := Prepend("", "- *2026-09-25 周五*", "- c"); empty != "- *2026-09-25 周五*\n- c\n" {
		t.Fatalf("empty = %q", empty)
	}
}

// Past years leave, grouped by year in the order met; this year stays; text
// before the first marker is kept.
func TestArchive(t *testing.T) {
	content := "> [!note] x\n\nlead\n" +
		"- *2026-01-02 周五*\n- new\n" +
		"- *2025-12-31 周三*\n- old\n    - detail\n" +
		"- *2024-05-01 周三*\n- older\n" +
		"- *2025-01-01 周三*\n- old too\n"
	remaining, moved := Archive(content, "2026")
	if want := "> [!note] x\n\nlead\n- *2026-01-02 周五*\n- new\n"; remaining != want {
		t.Fatalf("remaining =\n%q\nwant\n%q", remaining, want)
	}
	if len(moved) != 2 || moved[0].Year != "2025" || moved[1].Year != "2024" {
		t.Fatalf("moved = %+v", moved)
	}
	if want := "- *2025-12-31 周三*\n- old\n    - detail\n- *2025-01-01 周三*\n- old too"; moved[0].Text != want {
		t.Fatalf("2025 =\n%q\nwant\n%q", moved[0].Text, want)
	}
}

func TestArchiveNothingToMove(t *testing.T) {
	content := "- *2026-01-02 周五*\n- new\n"
	remaining, moved := Archive(content, "2026")
	if remaining != content || moved != nil {
		t.Fatalf("remaining = %q, moved = %+v", remaining, moved)
	}
}

func TestMerge(t *testing.T) {
	header := Header{CSSClass: "timeline", Title: "时间线", Source: "Timeline.md"}
	moving := Year{Year: "2025", Text: "- *2025-12-31 周三*\n- old"}

	started := Merge("", header, moving)
	for _, want := range []string{"  - timeline\n", "> [!note]- 2025 年的时间线\n", "从 Timeline.md 归档", "- old\n"} {
		if !strings.Contains(started, want) {
			t.Fatalf("new archive lacks %q:\n%s", want, started)
		}
	}
	existing := header.Render("2025") + "\n- *2025-06-01 周日*\n- earlier\n"
	merged := Merge(existing, header, moving)
	if !strings.HasSuffix(merged, "- *2025-12-31 周三*\n- old\n- *2025-06-01 周日*\n- earlier\n") {
		t.Fatalf("merged =\n%s", merged)
	}
	if strings.Count(merged, "[!note]") != 1 {
		t.Fatalf("header written twice:\n%s", merged)
	}
}
