package tree

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
)

func TestInPlaceFileMovesConfirmAllSnapshotsButNotOtherItems(t *testing.T) {
	st := inPlaceStore(t)
	ctx := context.Background()
	source := filepath.Join(st.Root(), "paper.pdf")
	if err := os.WriteFile(source, []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	item, err := st.Import(ctx, importRequest(source, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	digest := item.CurrentDigest()
	request := importRequest(source, "bob")
	request.Reuse = item.ID
	shared, err := st.Import(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	item.saveSnapshot(item.Type, map[string]string{"owner": "alice", "note": "correction"}, digest, "paper.pdf", item.Current(), time.Unix(1, 0))
	if err := st.WriteItem(item); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(st.Root(), norm.NFD.String("café.pdf"))
	if err := os.Rename(source, moved); err != nil {
		t.Fatal(err)
	}
	before := importFiles(t, st.Root())
	found, err := st.FindFilePaths(ctx, []string{digest})
	if err != nil || !reflect.DeepEqual(found[digest], []string{"café.pdf"}) {
		t.Fatalf("found %v: %v", found, err)
	}
	if !reflect.DeepEqual(before, importFiles(t, st.Root())) {
		t.Fatal("search wrote files")
	}
	saved, err := st.ConfirmFilePath(ctx, item.ID, digest, "paper.pdf", "café.pdf", time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range saved.Revisions {
		if r.Path != "café.pdf" {
			t.Fatal("snapshot path unchanged")
		}
	}
	other, err := st.GetItem(shared.ID)
	if err != nil || other.Revisions[0].Path != "paper.pdf" {
		t.Fatal("other Item was changed", err)
	}
	if _, err := st.ConfirmFilePath(ctx, item.ID, digest, "paper.pdf", "café.pdf", time.Unix(3, 0)); err == nil {
		t.Fatal("stale sidecar accepted")
	}
	if err := os.WriteFile(moved, []byte("synthetic changed bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	untouched := importFiles(t, st.Root())
	if _, err := st.ConfirmFilePath(ctx, shared.ID, digest, "paper.pdf", "café.pdf", time.Unix(3, 0)); err == nil {
		t.Fatal("changed candidate accepted")
	}
	if !reflect.DeepEqual(untouched, importFiles(t, st.Root())) {
		t.Fatal("rejected confirmation wrote files")
	}
}

func TestInPlaceChangedFilesRespectLocksAndKeepLostHistory(t *testing.T) {
	st := inPlaceStore(t)
	ctx := context.Background()
	source := filepath.Join(st.Root(), "paper.pdf")
	if err := os.WriteFile(source, []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	request := importRequest(source, "alice")
	request.Template.Locked = true
	item, err := st.Import(ctx, request)
	if err != nil || !item.Locked {
		t.Fatal("Template lock default", err)
	}
	old := item.CurrentDigest()
	if err := os.WriteFile(source, []byte("synthetic edited"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := FileDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	before := importFiles(t, st.Root())
	if _, err := st.RecordFileChange(ctx, item.ID, old, "paper.pdf", next, time.Unix(1, 0)); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatal("locked change", err)
	}
	if !reflect.DeepEqual(before, importFiles(t, st.Root())) {
		t.Fatal("locked check wrote")
	}
	locked := false
	if _, err := st.SetMetadata(item.ID, "", Metadata{Locked: &locked}, time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	changed, err := st.RecordFileChange(ctx, item.ID, old, "paper.pdf", next, time.Unix(3, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Revisions) != 2 || changed.CurrentDigest() != next || !changed.Revisions[0].Lost || changed.Revisions[0].Digest != old || changed.Revisions[1].Path != "paper.pdf" || changed.Revisions[1].Paper != changed.Head {
		t.Fatalf("changed %+v", changed)
	}
	if _, err := st.ReadPDF(changed, old); err == nil {
		t.Fatal("lost revision previews changed bytes")
	}
	if _, err := st.ReadPDF(changed, next); err != nil {
		t.Fatal(err)
	}
	before = importFiles(t, st.Root())
	if _, err := st.RecordFileChange(ctx, item.ID, old, "paper.pdf", next, time.Unix(4, 0)); err == nil {
		t.Fatal("same change recorded twice")
	}
	if !reflect.DeepEqual(before, importFiles(t, st.Root())) {
		t.Fatal("repeat wrote")
	}
	// Editing fields preserves the lost marker along with the original digest.
	corrected := changed.Clone()
	corrected.saveSnapshot(changed.Type, map[string]string{"owner": "alice", "note": "old correction"}, old, "paper.pdf", changed.Revisions[0].Ref(), time.Unix(5, 0))
	if !corrected.Revisions[len(corrected.Revisions)-1].Lost {
		t.Fatal("correction forgot lost file")
	}
	// Accepting the loss keeps the revision and its digest, still lost.
	if _, err := st.AcceptLostFile(ctx, item.ID, next, time.Unix(5, 0)); err == nil {
		t.Fatal("accepted a file that is not lost")
	}
	accepted, err := st.AcceptLostFile(ctx, item.ID, old, time.Unix(5, 0))
	if err != nil {
		t.Fatal(err)
	}
	if r := accepted.Revisions[0]; !r.Lost || !r.LossAccepted || r.Digest != old || accepted.Head != changed.Head {
		t.Fatalf("accepted %+v", accepted)
	}
	if _, err := st.ReadPDF(accepted, old); err == nil {
		t.Fatal("accepted loss previews changed bytes")
	}
	// A backup found elsewhere revives every old snapshot only after confirmation.
	backup := filepath.Join(st.Root(), "backup.pdf")
	if err := os.WriteFile(backup, []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := st.ConfirmFilePath(ctx, item.ID, old, "paper.pdf", "backup.pdf", time.Unix(6, 0))
	if err != nil {
		t.Fatal(err)
	}
	if restored.Head != changed.Head || restored.Revisions[0].Lost || restored.Revisions[0].LossAccepted {
		t.Fatal("restore changed HEAD or kept lost marker")
	}
	if _, err := st.ReadPDF(restored, old); err != nil {
		t.Fatal(err)
	}
}

func TestInPlaceFileSearchAndConfirmationRejectUnsafeTargets(t *testing.T) {
	st := inPlaceStore(t)
	ctx := context.Background()
	source := filepath.Join(st.Root(), "paper.pdf")
	if err := os.WriteFile(source, []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	item, err := st.Import(ctx, importRequest(source, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	digest := item.CurrentDigest()
	outside := filepath.Join(t.TempDir(), "outside.pdf")
	if err := os.WriteFile(outside, []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(st.Root(), "escape.pdf")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(st.Root(), TrashDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Root(), TrashDir, "copy.pdf"), []byte("synthetic original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	found, err := st.FindFilePaths(ctx, []string{digest})
	if err != nil || len(found[digest]) != 0 {
		t.Fatal("unsafe candidates", found, err)
	}
	for _, p := range []string{"escape.pdf", "trash/copy.pdf", "../outside.pdf", MarkerName} {
		if _, err := st.ConfirmFilePath(ctx, item.ID, digest, "paper.pdf", p, time.Unix(1, 0)); err == nil {
			t.Fatal("unsafe confirmation", p)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := st.FindFilePaths(cancelled, []string{digest}); err == nil {
		t.Fatal("cancelled scan")
	}
	if _, err := st.RecordFileChange(cancelled, item.ID, digest, "paper.pdf", digest, time.Unix(1, 0)); err == nil {
		t.Fatal("cancelled write")
	}
}

func TestMovedFileSearchSkipsOtherSizesAndFallsBackForOldSidecars(t *testing.T) {
	st := inPlaceStore(t)
	source := filepath.Join(st.Root(), "paper.pdf")
	data := []byte("synthetic moved file")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	item, err := st.Import(context.Background(), importRequest(source, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	if item.Revisions[0].Size == nil || *item.Revisions[0].Size != int64(len(data)) {
		t.Fatal("missing imported size")
	}
	if err := os.Rename(source, filepath.Join(st.Root(), "moved.pdf")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		name := filepath.Join(st.Root(), strings.Repeat("x", i+1)+".txt")
		if err := os.WriteFile(name, []byte(strings.Repeat("other", i+10)), 0000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(name, 0600)
	}
	blocked := filepath.Join(st.Root(), "x.txt")
	if _, err := os.ReadFile(blocked); err == nil {
		t.Skip("environment bypasses file permissions")
	}
	found, err := st.FindFilePaths(context.Background(), []string{item.CurrentDigest()})
	if err != nil || len(found[item.CurrentDigest()]) != 1 || found[item.CurrentDigest()][0] != "moved.pdf" {
		t.Fatal(found, err)
	}
	item.Revisions[0].Size = nil
	if err := st.WriteItem(item); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FindFilePaths(context.Background(), []string{item.CurrentDigest()}); err == nil {
		t.Fatal("old sidecar did not hash other-size candidates")
	}
}
