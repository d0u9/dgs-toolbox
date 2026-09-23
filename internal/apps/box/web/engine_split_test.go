package web_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	boxweb "dgs-toolbox/internal/apps/box/web"
)

func split(documents ...boxweb.SplitDocument) *[]boxweb.SplitDocument { return &documents }

func text(value string) *string { return &value }

// A split is written into the sidecar and the file is left whole: the Box
// holds one scan with the same digest it arrived with.
func TestFilingASplitKeepsTheFileWhole(t *testing.T) {
	engine, root, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0001.pdf", threePages(t))
	engine.Rescan()
	pending := engine.Pending()
	digest := pending[0].Digest

	draft, err := engine.Apply(boxweb.Edit{
		Digest: digest,
		Documents: split(
			boxweb.SplitDocument{Pages: "1-2", Type: "receipt", Total: "12.50"},
			boxweb.SplitDocument{Pages: "2"},
		),
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if draft.Unassigned != "3" {
		t.Errorf("unassigned: %q", draft.Unassigned)
	}

	filed, err := engine.File(digest, boxweb.Edit{Tags: &[]string{"tax"}, IgnoredPages: text("3")})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if filed.Digest != digest || len(filed.Documents) != 2 || filed.Unassigned != "" {
		t.Fatalf("filed: %+v", filed)
	}
	if filed.Documents[0].Total != "AUD 12.50" {
		t.Errorf("total: %q", filed.Documents[0].Total)
	}

	var sidecars []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".dgs-doc.yaml") {
			sidecars = append(sidecars, path)
		}
		return nil
	})
	if len(sidecars) != 1 {
		t.Fatalf("sidecars: %v", sidecars)
	}
	data, _ := os.ReadFile(sidecars[0])
	for _, line := range []string{"pages: 1-2", "ignored_pages: \"3\"", "type: unsorted"} {
		if !strings.Contains(string(data), line) {
			t.Errorf("sidecar lacks %q:\n%s", line, data)
		}
	}
}

func TestASplitCannotOverrideTheFile(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0001.pdf", threePages(t))
	engine.Rescan()
	digest := engine.Pending()[0].Digest

	_, err := engine.File(digest, boxweb.Edit{
		Type:      text("statement"),
		Documents: split(boxweb.SplitDocument{Pages: "1-3", Type: "receipt"}),
	})
	if err == nil {
		t.Fatal("a document overrode the file's type")
	}
	if _, err := engine.Apply(boxweb.Edit{Digest: digest, Documents: split(boxweb.SplitDocument{Pages: "1-4"})}); err == nil {
		t.Fatal("a page past the end was accepted")
	}
}

// A pile scanned together: every document has its own type, date, zone and
// description, and the file says only what they share.
func TestEachDocumentDescribesItself(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0001.pdf", threePages(t))
	engine.Rescan()
	digest := engine.Pending()[0].Digest

	filed, err := engine.File(digest, boxweb.Edit{
		Tags: &[]string{"pile"},
		Documents: split(
			boxweb.SplitDocument{Pages: "1", Type: "receipt", Description: "coffee", EventDate: "2024-03-01", EventZone: "Asia/Tokyo", Total: "JPY 500"},
			boxweb.SplitDocument{Pages: "2-3", Type: "letter", Description: "from Mum", EventDate: "2019-12-24", EventZone: "Australia/Sydney"},
		),
	})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	first, second := filed.Documents[0], filed.Documents[1]
	if first.Type != "receipt" || first.EventZone != "Asia/Tokyo" || first.Total != "JPY 500" || first.Description != "coffee" {
		t.Errorf("first: %+v", first)
	}
	if second.Type != "letter" || second.EventDate != "2019-12-24" || second.EventZone != "Australia/Sydney" {
		t.Errorf("second: %+v", second)
	}
	if _, err := engine.Apply(boxweb.Edit{Digest: digest, Documents: split(boxweb.SplitDocument{Pages: "1", EventZone: "Mars/Olympus"})}); err == nil {
		t.Error("an unknown zone was accepted")
	}
}

// A long PDF split halfway is not lost to a reload: the draft is kept in the
// inbox's state and read back by the next engine.
func TestAHalfDoneSplitSurvivesARestart(t *testing.T) {
	engine, root, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0001.pdf", threePages(t))
	engine.Rescan()
	digest := engine.Pending()[0].Digest
	if _, err := engine.Apply(boxweb.Edit{
		Digest:    digest,
		Type:      text("unsorted"),
		Documents: split(boxweb.SplitDocument{Pages: "1-2", Type: "receipt", Description: "coffee"}),
	}); err != nil {
		t.Fatal(err)
	}

	again, err := boxweb.NewEngine(boxweb.Settings{Root: root, Inbox: inbox, CacheDir: t.TempDir(), Currency: "AUD"})
	if err != nil {
		t.Fatal(err)
	}
	pending := again.Pending()
	if len(pending) != 1 || len(pending[0].Documents) != 1 || pending[0].Documents[0].Description != "coffee" {
		t.Fatalf("after restart: %+v", pending)
	}
	if pending[0].Unassigned != "3" {
		t.Errorf("unassigned: %q", pending[0].Unassigned)
	}
}
