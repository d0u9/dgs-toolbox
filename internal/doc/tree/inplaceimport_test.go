package tree

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
	"golang.org/x/text/unicode/norm"
)

func importFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	found := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			digest, err := FileDigest(p)
			if err != nil {
				return err
			}
			found[filepath.ToSlash(rel)] = digest
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func importRequest(source, owner string) ImportRequest {
	return ImportRequest{Source: source, Template: Template{Type: "paper", Kind: KindDocument, Fields: []Field{{Key: "owner", Required: true, Distinguishing: true}}}, Fields: map[string]string{"owner": owner}, Now: time.Unix(0, 0)}
}

func TestInPlaceImportRecordsCopiesAndConfirmsReuse(t *testing.T) {
	st := inPlaceStore(t)
	book := filepath.Join(st.Root(), norm.NFD.String("café.pdf"))
	if err := os.WriteFile(book, []byte("synthetic PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(book)
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.Import(context.Background(), importRequest(book, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	if item.Revisions[0].Path != "café.pdf" {
		t.Fatal("recorded path", item.Revisions[0].Path)
	}
	after, err := os.Stat(book)
	if err != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("original changed", err)
	}
	if len(importFiles(t, st.Root())) != 4 {
		t.Fatal("in-place import copied a file")
	}
	outside := filepath.Join(t.TempDir(), "copy.pdf")
	if err := os.WriteFile(outside, []byte("synthetic PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	untouched := importFiles(t, st.Root())
	request := importRequest(outside, "bob")
	_, err = st.Import(context.Background(), request)
	var offer *ReuseFile
	if !errors.As(err, &offer) || offer.Item != item.ID || offer.Path != "café.pdf" {
		t.Fatalf("offer: %v", err)
	}
	if !reflect.DeepEqual(untouched, importFiles(t, st.Root())) {
		t.Fatal("offer wrote files")
	}
	request.Reuse = offer.Item
	shared, err := st.Import(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if shared.ID == item.ID || shared.Revisions[0].Path != item.Revisions[0].Path || shared.CurrentDigest() != item.CurrentDigest() {
		t.Fatal("not shared")
	}
	if len(importFiles(t, st.Root())) != len(untouched)+1 {
		t.Fatal("reuse wrote an object")
	}
	copies := filepath.Join(st.Root(), "Copies")
	if err := os.Mkdir(copies, 0755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "new.pdf")
	if err := os.WriteFile(source, []byte("different synthetic PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	request = importRequest(source, "carol")
	request.Folder = copies
	copied, err := st.Import(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Revisions[0].Path != "Copies/new.pdf" {
		t.Fatal("copy path", copied.Revisions[0].Path)
	}
	original, _ := os.ReadFile(source)
	result, err := os.ReadFile(st.PDF(copied, copied.CurrentDigest()))
	if err != nil || !bytes.Equal(original, result) {
		t.Fatal("copy", err)
	}
	// Occupied names stay unchanged, even when their content differs.
	if err := os.WriteFile(source, []byte("third synthetic PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	request.Fields = map[string]string{"owner": "dave"}
	untouched = importFiles(t, st.Root())
	if _, err := st.Import(context.Background(), request); !errors.Is(err, verifiedcopy.ErrDestinationExists) {
		t.Fatalf("collision: %v", err)
	}
	if !reflect.DeepEqual(untouched, importFiles(t, st.Root())) {
		t.Fatal("collision changed tree")
	}
}

func TestInPlacePhotoImportHasNoStagingBeforeConfirmation(t *testing.T) {
	st := inPlaceStore(t)
	source, data := docImage(t, 130)
	before := importFiles(t, st.Root())
	input, err := st.prepareInput(context.Background(), []string{source})
	if err != nil {
		t.Fatal(err)
	}
	defer input.cleanup()
	if input.generated == nil || !reflect.DeepEqual(before, importFiles(t, st.Root())) {
		t.Fatal("conversion wrote into tree")
	}
	request := importRequest(source, "alice")
	request.Folder = st.Root()
	request.KeepOriginal = true
	item, err := st.Import(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	r := item.Revisions[0]
	if r.Path != "scan.pdf" || len(r.Originals) != 1 {
		t.Fatalf("photo paths: %+v", r)
	}
	original, err := os.ReadFile(st.OriginalPath(item, r.Digest, r.Originals[0]))
	if err != nil || !bytes.Equal(original, data) {
		t.Fatal("original", err)
	}
	before = importFiles(t, st.Root())
	request.Fields = map[string]string{"owner": "bob"}
	_, err = st.Import(context.Background(), request)
	var offer *ReuseFile
	if !errors.As(err, &offer) {
		t.Fatalf("photo offer: %v", err)
	}
	if !reflect.DeepEqual(before, importFiles(t, st.Root())) {
		t.Fatal("photo offer wrote files")
	}
	request.Reuse = offer.Item
	reused, err := st.Import(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if reused.Revisions[0].Path != r.Path || !reflect.DeepEqual(reused.Revisions[0].Originals, r.Originals) {
		t.Fatal("photo reuse changed provenance")
	}
	for name := range importFiles(t, st.Root()) {
		if strings.HasSuffix(name, ".dgs-part") || strings.HasPrefix(name, "items/.") {
			t.Fatal("staging left behind", name)
		}
	}
}

func TestInPlaceImportRejectsUnsafePathsAndRollsBackOnlyNewFiles(t *testing.T) {
	for _, kind := range []string{"missing-folder", "outside", "reserved", "symlink", "sidecar", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			st := inPlaceStore(t)
			source, _ := docImage(t, 50)
			request := importRequest(source, "alice")
			request.Folder = st.Root()
			request.KeepOriginal = true
			before := importFiles(t, st.Root())
			ctx := context.Background()
			switch kind {
			case "missing-folder":
				request.Folder = ""
			case "outside":
				request.Folder = t.TempDir()
			case "reserved":
				request.Folder = filepath.Join(st.Root(), MetaDir, ItemsDir)
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(st.Root(), "escape")); err != nil {
					t.Fatal(err)
				}
				request.Folder = filepath.Join(st.Root(), "escape")
				// WalkDir records symlinks as files; snapshot before adding this link.
			case "sidecar":
				if err := os.Remove(filepath.Join(st.Root(), MetaDir, ItemsDir)); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(st.Root(), MetaDir, ItemsDir), []byte("occupied"), 0600); err != nil {
					t.Fatal(err)
				}
				before = importFiles(t, st.Root())
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if _, err := st.Import(ctx, request); err == nil {
				t.Fatal("accepted unsafe or failing import")
			}
			if kind != "symlink" && !reflect.DeepEqual(before, importFiles(t, st.Root())) {
				t.Fatal("failed import left files or removed original data")
			}
			if kind == "symlink" {
				entries, err := os.ReadDir(st.Root())
				if err != nil || len(entries) != 2 {
					t.Fatal("symlink import wrote files", err)
				}
			}
		})
	}
}

func TestInPlacePhotoImportArchivesInternalOriginalWithoutChangingSource(t *testing.T) {
	st := inPlaceStore(t)
	_, data := docImage(t, 140)
	inside := filepath.Join(st.Root(), "page.png")
	if err := os.WriteFile(inside, data, 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(inside)
	if err != nil {
		t.Fatal(err)
	}
	request := importRequest(inside, "alice")
	request.Folder = st.Root()
	request.KeepOriginal = true
	item, err := st.Import(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	r := item.Revisions[0]
	if len(r.Originals) != 1 || r.Originals[0].Path != "page/page.png" {
		t.Fatal("internal original was relocated")
	}
	after, err := os.Stat(inside)
	if err != nil || !os.SameFile(original, after) || !original.ModTime().Equal(after.ModTime()) {
		t.Fatal("internal original changed", err)
	}
	// An external pathname alias to an internal PDF is still recorded in place.
	alias := filepath.Join(t.TempDir(), "alias.pdf")
	if err := os.Symlink(st.PDF(item, r.Digest), alias); err != nil {
		t.Fatal(err)
	}
	request = importRequest(alias, "bob")
	_, err = st.Import(context.Background(), request)
	var offer *ReuseFile
	if !errors.As(err, &offer) || offer.Item != item.ID {
		t.Fatalf("alias offer: %v", err)
	}
}

func TestInPlaceCreatesItemWithoutPDF(t *testing.T) {
	root := t.TempDir()
	if err := InitWithLayout(root, time.Now(), "in-place"); err != nil {
		t.Fatal(err)
	}
	st, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Capabilities().WithoutFile {
		t.Fatal("in-place tree offers no creation without a PDF")
	}
	tpl := Template{Type: "trip", Kind: KindDocument, Fields: []Field{{Key: "owner", Required: true, Distinguishing: true}, {Key: "name", Required: true, Distinguishing: true}}}
	item, err := st.CreateWithoutPDF(ImportRequest{Template: tpl, Fields: map[string]string{"owner": "alice", "name": "Trip"}, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	items, err := st.LoadItems()
	if err != nil || len(items) != 1 || items[0].ID != item.ID {
		t.Fatal(items, err)
	}
}

func TestImportListingNameIndexIsDiscardedBetweenScans(t *testing.T) {
	root := t.TempDir()
	if err := InitWithLayout(root, time.Unix(0, 0), "in-place"); err != nil {
		t.Fatal(err)
	}
	st, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"first.pdf", "second.pdf"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
		files, err := st.ImportFiles(root)
		if err != nil || len(files) != i+1 {
			t.Fatalf("listing %d: %v %v", i, files, err)
		}
		if st.sourceNames != nil {
			t.Fatal("listing index retained beyond its scan")
		}
	}
}
