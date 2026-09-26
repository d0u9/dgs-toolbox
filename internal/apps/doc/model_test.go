package doc

import (
	"strings"
	"testing"

	docweb "dgs-toolbox/internal/apps/doc/web"
)

func TestModelShowsEveryConfiguredTree(t *testing.T) {
	m := NewModel(docweb.Settings{Trees: []docweb.Tree{
		{Name: "books", Root: "/Volumes/assets/12-PDFs & Books"},
		{Name: "docs", Root: "/Volumes/archived/05-Docs"},
	}})
	view := m.View()
	for _, want := range []string{
		"Tree books  /Volumes/assets/12-PDFs & Books",
		"Tree docs   /Volumes/archived/05-Docs",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not show %q: %s", want, view)
		}
	}
	// Every root starts in the same column.
	column := -1
	for _, line := range strings.Split(view, "\n") {
		at := strings.Index(line, "/Volumes/")
		if at < 0 {
			continue
		}
		if column >= 0 && at != column {
			t.Errorf("roots are not aligned: %s", view)
		}
		column = at
	}
}
