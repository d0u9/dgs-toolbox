package check

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/tree"
)

func TestInPlaceChecksOnlyReferencedFiles(t *testing.T) {
	root := t.TempDir()
	if err := tree.InitWithLayout(root, time.Unix(0, 0), "in-place"); err != nil {
		t.Fatal(err)
	}
	st, err := tree.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"known.pdf", "unimported.pdf"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("synthetic PDF"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	digest, err := tree.FileDigest(filepath.Join(root, "known.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	item := tree.Item{ID: "alice", Type: "id_card", Kind: tree.KindDocument, Revisions: []tree.Revision{{Digest: digest, Path: "known.pdf"}}}
	if err := st.WriteItem(item); err != nil {
		t.Fatal(err)
	}
	report, err := Tree(context.Background(), st, nil)
	if err != nil || len(report.Problems) != 0 || report.Checked != 1 {
		t.Fatalf("check: %+v %v", report, err)
	}
	if err := os.Remove(filepath.Join(root, "known.pdf")); err != nil {
		t.Fatal(err)
	}
	report, err = Tree(context.Background(), st, nil)
	if err != nil || len(report.Problems) != 1 || report.Problems[0].Kind != Moved || report.Problems[0].Path != "known.pdf" {
		t.Fatalf("moved: %+v %v", report, err)
	}
}

func TestInPlaceChangedCheckIsReadOnlyAndSearchesLostContent(t *testing.T) {
	root := t.TempDir()
	if err := tree.InitWithLayout(root, time.Unix(0, 0), "in-place"); err != nil {
		t.Fatal(err)
	}
	st, err := tree.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "paper.pdf")
	if err := os.WriteFile(source, []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := tree.FileDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	item := tree.Item{ID: "alice", Type: "id_card", Kind: tree.KindDocument, Head: digest, Revisions: []tree.Revision{{Digest: digest, Path: "paper.pdf"}}}
	if err := st.WriteItem(item); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(st.Sidecar(item.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("synthetic edited"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Tree(context.Background(), st, nil)
	if err != nil || len(report.Problems) != 1 || report.Problems[0].Kind != Changed || report.Problems[0].Actual == "" {
		t.Fatalf("changed %+v %v", report, err)
	}
	after, err := os.ReadFile(st.Sidecar(item.ID))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only check changed sidecar")
	}
	problem := report.Problems[0]
	if _, err := st.RecordFileChange(context.Background(), item.ID, digest, problem.Path, problem.Actual, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "backup.pdf"), []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err = Tree(context.Background(), st, nil)
	if err != nil || len(report.Problems) != 1 || report.Problems[0].Kind != Moved || !report.Problems[0].Lost || report.Problems[0].Candidates[0] != "backup.pdf" {
		t.Fatalf("lost backup %+v %v", report, err)
	}
}

func TestInPlaceDirectoryMoveFindsPDFAndRetainedOriginal(t *testing.T) {
	root := t.TempDir()
	if err := tree.InitWithLayout(root, time.Unix(0, 0), "in-place"); err != nil {
		t.Fatal(err)
	}
	st, err := tree.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "old"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"paper.pdf": "synthetic PDF", "page.png": "synthetic retained original"} {
		if err := os.WriteFile(filepath.Join(root, "old", name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	pdf, err := tree.FileDigest(filepath.Join(root, "old", "paper.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	image, err := tree.FileDigest(filepath.Join(root, "old", "page.png"))
	if err != nil {
		t.Fatal(err)
	}
	item := tree.Item{ID: "alice", Type: "id_card", Kind: tree.KindDocument, Head: pdf, Revisions: []tree.Revision{{Digest: pdf, Path: "old/paper.pdf", Originals: []tree.Original{{Digest: image, Path: "old/page.png", Extension: ".png"}}}}}
	if err := st.WriteItem(item); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "old"), filepath.Join(root, "new")); err != nil {
		t.Fatal(err)
	}
	report, err := Tree(context.Background(), st, nil)
	if err != nil || len(report.Problems) != 2 {
		t.Fatalf("directory move %+v %v", report, err)
	}
	for _, p := range report.Problems {
		if p.Kind != Moved || len(p.Candidates) != 1 {
			t.Fatal("missing candidate", p)
		}
		if _, err := st.ConfirmFilePath(context.Background(), p.Item, p.Digest, p.Path, p.Candidates[0], time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
	}
	report, err = Tree(context.Background(), st, nil)
	if err != nil || len(report.Problems) != 0 || report.Checked != 2 {
		t.Fatalf("confirmed %+v %v", report, err)
	}
	saved, err := st.GetItem(item.ID)
	if err != nil || saved.Head != pdf || saved.Revisions[0].Path != "new/paper.pdf" || saved.Revisions[0].Originals[0].Path != "new/page.png" {
		t.Fatal("references or HEAD", saved, err)
	}
}
