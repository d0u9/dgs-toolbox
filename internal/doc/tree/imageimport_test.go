package tree

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func docImage(t *testing.T, shade uint8) (string, []byte) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 20, 30))
	img.Set(1, 2, color.NRGBA{R: shade, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "scan.png")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path, b.Bytes()
}

func TestImageImportRetentionAndDuplicate(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(map[bool]string{false: "PDF only", true: "retain original"}[keep], func(t *testing.T) {
			root := newTree(t)
			source, data := docImage(t, 120)
			req := ImportRequest{Source: source, Template: idCard(t, root), Fields: map[string]string{"owner": "alice", "country": "AU"}, Now: now, KeepOriginal: keep}
			item, err := testStoreAt(root).Import(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			rev := item.Revisions[0]
			pdf, err := os.ReadFile(testStoreAt(root).PDF(Item{ID: item.ID}, rev.Digest))
			if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
				t.Fatalf("PDF: %v", err)
			}
			if rev.Source != "scan.png" || len(rev.SourceDigests) == 0 || (len(rev.Originals) > 0) != keep {
				t.Fatalf("revision: %+v", rev)
			}
			if keep {
				original, err := os.ReadFile(testStoreAt(root).OriginalPath(item, rev.Digest, rev.Originals[0]))
				if err != nil || !bytes.Equal(original, data) {
					t.Fatalf("original: %v", err)
				}
			}
			original, _ := os.ReadFile(source)
			if !bytes.Equal(original, data) {
				t.Fatal("source changed")
			}
			if _, err := testStoreAt(root).Import(context.Background(), req); !errors.Is(err, ErrDuplicate) {
				t.Fatalf("duplicate: %v", err)
			}
			loaded, err := testStoreAt(root).GetItem(item.ID)
			if err != nil || !slices.Equal(loaded.Revisions[0].SourceDigests, rev.SourceDigests) {
				t.Fatalf("sidecar: %v", err)
			}
		})
	}
}

func TestImageRevisionAndTrashPreserveOriginal(t *testing.T) {
	root := newTree(t)
	tpl := idCard(t, root)
	source, _ := docImage(t, 100)
	item, err := testStoreAt(root).Import(context.Background(), ImportRequest{Source: source, Template: tpl, Fields: map[string]string{"owner": "alice", "country": "AU"}, KeepOriginal: true, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	source, data := docImage(t, 200)
	item, err = testStoreAt(root).AddRevisionWithMetadata(context.Background(), item.ID, source, tpl, nil, RevisionMetadata{KeepOriginal: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	rev, _ := item.Revision(item.Head)
	if len(rev.Originals) == 0 {
		t.Fatal("revision original missing")
	}
	originalPath := testStoreAt(root).OriginalPath(item, rev.Digest, rev.Originals[0])
	item, to, err := testStoreAt(root).TrashRevision(item.ID, rev.Ref(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Revisions) != 1 {
		t.Fatal("revision not removed")
	}
	if _, err := os.Stat(originalPath); !os.IsNotExist(err) {
		t.Fatal("original still outside trash")
	}
	files, err := os.ReadDir(filepath.Dir(to))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".png" {
			kept, _ := os.ReadFile(filepath.Join(filepath.Dir(to), f.Name()))
			found = bytes.Equal(kept, data)
		}
	}
	if !found {
		t.Fatal("original missing from trash")
	}
}

func TestSameImageRevisionCanArchivePreviouslyOmittedOriginal(t *testing.T) {
	root := newTree(t)
	source, data := docImage(t, 80)
	tpl := idCard(t, root)
	item, err := testStoreAt(root).Import(context.Background(), ImportRequest{Source: source, Template: tpl, Fields: map[string]string{"owner": "alice", "country": "AU"}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	item, err = testStoreAt(root).AddRevisionWithMetadata(context.Background(), item.ID, source, tpl, nil, RevisionMetadata{KeepOriginal: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Revisions) != 1 || len(item.Revisions[0].Originals) == 0 {
		t.Fatal("same snapshot did not gain original")
	}
	original, err := os.ReadFile(testStoreAt(root).OriginalPath(item, item.Revisions[0].Digest, item.Revisions[0].Originals[0]))
	if err != nil || !bytes.Equal(original, data) {
		t.Fatalf("original: %v", err)
	}
}

func TestRetainAlternateImageEncoding(t *testing.T) {
	root := newTree(t)
	tpl := idCard(t, root)
	img := image.NewNRGBA(image.Rect(0, 0, 20, 30))
	img.Set(1, 2, color.NRGBA{R: 120, A: 255})
	var a, b bytes.Buffer
	(&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&a, img)
	(&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&b, img)
	if bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("probe inputs must differ")
	}
	dir := t.TempDir()
	first := filepath.Join(dir, "first.png")
	second := filepath.Join(dir, "second.png")
	os.WriteFile(first, a.Bytes(), 0600)
	os.WriteFile(second, b.Bytes(), 0600)
	item, err := testStoreAt(root).Import(context.Background(), ImportRequest{Source: first, Template: tpl, Fields: map[string]string{"owner": "alice", "country": "AU"}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	item, err = testStoreAt(root).AddRevisionWithMetadata(context.Background(), item.ID, second, tpl, nil, RevisionMetadata{KeepOriginal: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	rev, _ := item.Revision(item.Current())
	if len(rev.Originals) == 0 {
		t.Fatal("KeepOriginal=true silently ignored for alternate PNG encoding")
	}
}

func TestAttachSameImageCanRetainOriginal(t *testing.T) {
	root := newTree(t)
	source, _ := docImage(t, 80)
	item, err := testStoreAt(root).Import(context.Background(), ImportRequest{Source: source, Template: idCard(t, root), Fields: map[string]string{"owner": "alice", "country": "AU"}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	item, err = testStoreAt(root).AttachFile(context.Background(), item.ID, item.Current(), []string{source}, true, now)
	if err != nil {
		t.Fatal(err)
	}
	rev, _ := item.Revision(item.Current())
	if len(rev.Originals) == 0 {
		t.Fatal("AttachFile KeepOriginal=true returns without archiving same image")
	}
}

func TestGeneratedPDFReadbackRejectsChangedTemporary(t *testing.T) {
	root := newTree(t)
	source, _ := docImage(t, 30)
	input, err := testStoreAt(root).prepareInput(context.Background(), []string{source})
	if err != nil {
		t.Fatal(err)
	}
	defer input.cleanup()
	if err := os.WriteFile(input.pdf, []byte("damaged PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := input.digest(); err == nil {
		t.Fatal("accepted changed converter output")
	}
}

func TestAlternateEncodingDoesNotReplaceArchivedOriginal(t *testing.T) {
	root := newTree(t)
	tpl := idCard(t, root)
	img := image.NewNRGBA(image.Rect(0, 0, 20, 30))
	img.Set(1, 2, color.NRGBA{R: 120, A: 255})
	var a, b bytes.Buffer
	(&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&a, img)
	(&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&b, img)
	dir := t.TempDir()
	first := filepath.Join(dir, "first.png")
	second := filepath.Join(dir, "second.png")
	os.WriteFile(first, a.Bytes(), 0600)
	os.WriteFile(second, b.Bytes(), 0600)
	item, err := testStoreAt(root).Import(context.Background(), ImportRequest{Source: first, Template: tpl, Fields: map[string]string{"owner": "alice", "country": "AU"}, KeepOriginal: true, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	original := item.Revisions[0].Originals[0]
	if _, err := testStoreAt(root).AddRevisionWithMetadata(context.Background(), item.ID, second, tpl, nil, RevisionMetadata{KeepOriginal: true}, now); err == nil {
		t.Fatal("silently accepted replacement of different archived original")
	}
	item, err = testStoreAt(root).AddRevisionWithMetadata(context.Background(), item.ID, second, tpl, nil, RevisionMetadata{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if item.Revisions[0].Originals[0] != original || len(item.Revisions[0].SourceDigests) != 2 {
		t.Fatal("lost original or input alias")
	}
	retained, err := os.ReadFile(testStoreAt(root).OriginalPath(item, item.Revisions[0].Digest, original))
	if err != nil || !bytes.Equal(retained, a.Bytes()) {
		t.Fatalf("original: %v", err)
	}
}

func TestImagesImportAsOnePDFAndAreKnownAgain(t *testing.T) {
	root := newTree(t)
	tpl := idCard(t, root)
	first, a := docImage(t, 10)
	second, b := docImage(t, 20)
	item, err := testStoreAt(root).Import(context.Background(), ImportRequest{Source: first, Pages: []string{second}, Template: tpl,
		Fields: map[string]string{"owner": "alice", "country": "AU"}, KeepOriginal: true, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	rev := item.Revisions[0]
	if len(rev.Originals) != 2 || len(rev.SourceDigests) != 2 || rev.Source != "scan.png" {
		t.Fatalf("revision: %+v", rev)
	}
	for i, want := range [][]byte{a, b} {
		got, err := os.ReadFile(testStoreAt(root).OriginalPath(item, rev.Digest, rev.Originals[i]))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("original %d: %v", i+1, err)
		}
	}
	// One of the images alone makes another PDF, but is the same image.
	if _, err := testStoreAt(root).Import(context.Background(), ImportRequest{Source: second, Template: tpl,
		Fields: map[string]string{"owner": "bob", "country": "AU"}, Now: now}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("image already in the tree: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, ItemsDir))
	for _, e := range entries {
		if e.Name() != item.ID {
			t.Fatalf("left behind %s", e.Name())
		}
	}
}
