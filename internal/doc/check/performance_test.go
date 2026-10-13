package check

import (
	"bytes"
	"context"
	"dgs-toolbox/internal/doc/tree"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sharedCheckTree(t testing.TB, count int) *tree.Store {
	t.Helper()
	root := t.TempDir()
	if err := tree.InitWithLayout(root, time.Unix(0, 0), "in-place"); err != nil {
		t.Fatal(err)
	}
	st, err := tree.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "shared.pdf"), bytes.Repeat([]byte("synthetic content"), 1<<18), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := tree.FileDigest(filepath.Join(root, "shared.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		item := tree.Item{ID: fmt.Sprintf("ITEM-%04d", i), Type: "id_card", Kind: tree.KindDocument, Head: digest, Revisions: []tree.Revision{{Digest: digest, Path: "shared.pdf"}}}
		if err := st.WriteItem(item); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func BenchmarkCheckSharedFile(b *testing.B) {
	st := sharedCheckTree(b, 100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Tree(context.Background(), st, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSharedFileCheckedOnceAndEveryChangedReferenceReported(t *testing.T) {
	st := sharedCheckTree(t, 3)
	report, err := Tree(context.Background(), st, nil)
	if err != nil || report.Checked != 1 || len(report.Problems) != 0 {
		t.Fatalf("unchanged: %+v %v", report, err)
	}
	if err := os.WriteFile(filepath.Join(st.Root(), "shared.pdf"), []byte("edited synthetic content"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err = Tree(context.Background(), st, nil)
	if err != nil || report.Checked != 1 || len(report.Problems) != 3 {
		t.Fatalf("changed: %+v %v", report, err)
	}
	for _, problem := range report.Problems {
		if problem.Kind != Changed || problem.Actual == "" || problem.Item == "" {
			t.Fatalf("lost reference: %+v", problem)
		}
	}
	// A subsequent check must read content again, even when its path is unchanged.
	if err := os.WriteFile(filepath.Join(st.Root(), "shared.pdf"), []byte("another synthetic edit"), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := Tree(context.Background(), st, nil)
	if err != nil || again.Problems[0].Actual == report.Problems[0].Actual {
		t.Fatal("stale digest reused across checks")
	}
}

func BenchmarkFindMissingWithUnrelatedFiles(b *testing.B) {
	st := sharedCheckTree(b, 1)
	item, err := st.GetItem("ITEM-0000")
	if err != nil {
		b.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(st.Root(), "shared.pdf"))
	if err != nil {
		b.Fatal(err)
	}
	size := info.Size()
	item.Revisions[0].Size = &size
	if err := st.WriteItem(item); err != nil {
		b.Fatal(err)
	}
	if err := os.Remove(filepath.Join(st.Root(), "shared.pdf")); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if err := os.WriteFile(filepath.Join(st.Root(), fmt.Sprintf("unrelated-%04d.txt", i)), []byte("synthetic"), 0600); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.FindFilePaths(context.Background(), []string{item.CurrentDigest()}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestMissingSearchSizeFilterStillFindsMovedContent(t *testing.T) {
	st := sharedCheckTree(t, 1)
	item, err := st.GetItem("ITEM-0000")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(st.Root(), "shared.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	size := info.Size()
	item.Revisions[0].Size = &size
	if err := st.WriteItem(item); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(st.Root(), "shared.pdf"), filepath.Join(st.Root(), "moved.pdf")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Root(), "unrelated.txt"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Tree(context.Background(), st, nil)
	if err != nil || len(report.Problems) != 1 {
		t.Fatalf("check: %+v %v", report, err)
	}
	p := report.Problems[0]
	if p.Kind != Moved || len(p.Candidates) != 1 || p.Candidates[0] != "moved.pdf" {
		t.Fatalf("candidate: %+v", p)
	}
}
