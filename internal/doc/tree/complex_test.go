package tree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInPlaceTrashCountsReferencesAndNeverDeletesBytes(t *testing.T) {
	st := inPlaceStore(t)
	ctx := context.Background()
	source := filepath.Join(st.Root(), "paper.pdf")
	if err := os.WriteFile(source, []byte("synthetic shared document"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := st.Import(ctx, importRequest(source, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	req := importRequest(source, "bob")
	req.Reuse = first.ID
	second, err := st.Import(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Trash(first.ID, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("shared file moved", err)
	}
	if _, err := st.GetItem(first.ID); err == nil {
		t.Fatal("sidecar still live")
	}
	archived, err := st.Trash(second.ID, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("last reference did not move to trash")
	}
	bytes, err := os.ReadFile(filepath.Join(archived, second.CurrentDigest()+".pdf"))
	if err != nil || string(bytes) != "synthetic shared document" {
		t.Fatal("trash lost bytes", err)
	}
	if _, err := os.Stat(filepath.Join(archived, filepath.Base(st.Sidecar(second.ID)))); err != nil {
		t.Fatal("trash lost sidecar", err)
	}
}

func TestInPlaceSecondRevisionKeepsRecordedFileAndDeletesOne(t *testing.T) {
	st := inPlaceStore(t)
	ctx := context.Background()
	firstPath := filepath.Join(st.Root(), "first.pdf")
	if err := os.WriteFile(firstPath, []byte("synthetic first version"), 0600); err != nil {
		t.Fatal(err)
	}
	req := importRequest(firstPath, "alice")
	first, err := st.Import(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	incoming := filepath.Join(t.TempDir(), "second.pdf")
	if err := os.WriteFile(incoming, []byte("synthetic second version"), 0600); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(st.Root(), "editions")
	if err := os.Mkdir(folder, 0755); err != nil {
		t.Fatal(err)
	}
	second, err := st.AddRevisionWithMetadata(ctx, first.ID, incoming, req.Template, nil, RevisionMetadata{Folder: folder}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Revisions) != 2 {
		t.Fatal(second)
	}
	for index, r := range second.Revisions {
		expected := "editions/second.pdf"
		if index == 0 {
			expected = "first.pdf"
		}
		if r.Path != expected {
			t.Fatal("revision path", r.Path)
		}
		if _, err := st.ReadPDF(second, r.Digest); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatal("recorded object moved")
	}
	if _, err := os.Stat(filepath.Join(folder, first.ID)); !os.IsNotExist(err) {
		t.Fatal("unexpected Item folder", err)
	}
	if data, err := os.ReadFile(incoming); err != nil || string(data) != "synthetic second version" {
		t.Fatal("source changed", err)
	}
	kept, trash, err := st.TrashRevision(first.ID, first.Current(), time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(kept.Revisions) != 1 || kept.CurrentDigest() != second.CurrentDigest() {
		t.Fatal("wrong revision removed")
	}
	if _, err := os.Stat(filepath.Join(trash, first.CurrentDigest()+".pdf")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReadPDF(kept, kept.CurrentDigest()); err != nil {
		t.Fatal("remaining PDF moved", err)
	}
}

func TestInPlaceReplacementKeepsBothOriginalFiles(t *testing.T) {
	st := inPlaceStore(t)
	ctx := context.Background()
	tpl := Template{Type: "card", Kind: KindDocument, Fields: []Field{{Key: "owner", Required: true, Distinguishing: true}, {Key: "country", Required: true, Distinguishing: true}, {Key: "number"}}}
	firstPath := filepath.Join(st.Root(), "old.pdf")
	nextPath := filepath.Join(st.Root(), "new.pdf")
	for p, data := range map[string]string{firstPath: "synthetic old card", nextPath: "synthetic renewed card"} {
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	req := ImportRequest{Source: firstPath, Template: tpl, Fields: map[string]string{"owner": "alice", "country": "AU", "number": "1"}, Now: time.Unix(0, 0)}
	old, err := st.Import(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	req.Source = nextPath
	req.Fields["number"] = "2"
	req.Replaces = old.ID
	next, err := st.Import(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	old, err = st.Supersede(old.ID, next.ID, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !old.Retired || old.SupersededBy != next.ID {
		t.Fatal(old)
	}
	for _, p := range []string{firstPath, nextPath} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal("replacement moved file", err)
		}
	}
}

func TestInPlaceTrashRefusesChangedContentBeforeAnyMove(t *testing.T) {
	st := inPlaceStore(t)
	source := filepath.Join(st.Root(), "paper.pdf")
	if err := os.WriteFile(source, []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	item, err := st.Import(context.Background(), importRequest(source, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("synthetic changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Trash(item.ID, time.Unix(1, 0)); err == nil {
		t.Fatal("changed file deleted")
	}
	if _, err := st.GetItem(item.ID); err != nil {
		t.Fatal("sidecar moved on refused delete")
	}
}

func TestInPlaceRevisionNameConflictLeavesFilesAndSidecarUnchanged(t *testing.T) {
	st := inPlaceStore(t)
	ctx := context.Background()
	source := filepath.Join(st.Root(), "first.pdf")
	if err := os.WriteFile(source, []byte("synthetic first"), 0600); err != nil {
		t.Fatal(err)
	}
	req := importRequest(source, "alice")
	first, err := st.Import(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	incoming := filepath.Join(t.TempDir(), "second.pdf")
	if err := os.WriteFile(incoming, []byte("synthetic second"), 0600); err != nil {
		t.Fatal(err)
	}
	occupied := filepath.Join(st.Root(), "second.pdf")
	if err := os.WriteFile(occupied, []byte("synthetic unrelated occupant"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddRevisionWithMetadata(ctx, first.ID, incoming, req.Template, nil, RevisionMetadata{Folder: st.Root()}, time.Unix(1, 0)); err == nil {
		t.Fatal("occupied destination accepted")
	}
	kept, err := st.GetItem(first.ID)
	if err != nil || len(kept.Revisions) != 1 || kept.Revisions[0].Path != "first.pdf" {
		t.Fatal("sidecar changed", kept, err)
	}
	if bytes, err := os.ReadFile(source); err != nil || string(bytes) != "synthetic first" {
		t.Fatal("original move not reversed", err)
	}
	if bytes, err := os.ReadFile(occupied); err != nil || string(bytes) != "synthetic unrelated occupant" {
		t.Fatal("existing destination changed", err)
	}
}

func TestInPlaceRevisionSuffixCopiesBesideCurrentFile(t *testing.T) {
	st := inPlaceStore(t)
	ctx := context.Background()
	if err := os.Mkdir(filepath.Join(st.Root(), "books"), 0o755); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(st.Root(), "books", "a.pdf")
	if err := os.WriteFile(firstPath, []byte("synthetic first edition"), 0600); err != nil {
		t.Fatal(err)
	}
	req := importRequest(firstPath, "alice")
	first, err := st.Import(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	incoming := filepath.Join(t.TempDir(), "xxx.PDF")
	if err := os.WriteFile(incoming, []byte("synthetic second edition"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := st.AddRevisionWithMetadata(ctx, first.ID, incoming, req.Template, nil, RevisionMetadata{Suffix: "-1"}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Revisions[1].Path; got != "books/a-1.pdf" {
		t.Fatal("path", got)
	}
	if b, err := os.ReadFile(filepath.Join(st.Root(), "books", "a-1.pdf")); err != nil || string(b) != "synthetic second edition" {
		t.Fatal("copy", err)
	}
	if _, err := os.Stat(incoming); err != nil {
		t.Fatal("source moved", err)
	}
	third := filepath.Join(t.TempDir(), "yyy.pdf")
	if err := os.WriteFile(third, []byte("synthetic third edition"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddRevisionWithMetadata(ctx, first.ID, third, req.Template, nil, RevisionMetadata{Suffix: "/x"}, time.Unix(2, 0)); err == nil {
		t.Fatal("separator accepted")
	}
	inside := filepath.Join(st.Root(), "books", "inside.pdf")
	if err := os.WriteFile(inside, []byte("synthetic inside edition"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddRevisionWithMetadata(ctx, first.ID, inside, req.Template, nil, RevisionMetadata{Suffix: "-2"}, time.Unix(3, 0)); err == nil {
		t.Fatal("suffix on a file already inside the tree accepted")
	}
}
