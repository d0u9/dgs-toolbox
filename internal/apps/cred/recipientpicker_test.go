package cred

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRecipientPickerUsesSingleRowsWithInlineDetails(t *testing.T) {
	f := newFixture(t)
	p := newRecipientPicker(loadSnapshot(f.path).folder, nil, map[string]bool{}, nil)
	p.list.SetSize(180, len(p.rows)+1)
	lines := strings.Split(ansi.Strip(p.list.View(true, titleStyle, mutedStyle)), "\n")
	if len(lines) != len(p.rows)+1 {
		t.Fatalf("entries use more than one row: %d rows for %d entries and a divider", len(lines), len(p.rows))
	}
	for i := range p.rows {
		item, _ := p.list.ItemAt(i)
		found := false
		for _, line := range lines {
			if strings.Contains(line, item.Label) && strings.Contains(line, item.Detail) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("label and detail do not share a row: %q / %q", item.Label, item.Detail)
		}
	}
}
