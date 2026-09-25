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
		"Tree docs  /Volumes/archived/05-Docs",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not show %q: %s", want, view)
		}
	}
}
