package tree

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenLegacyMarkerAndInitBytes(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	if err := Init(root, now); err != nil {
		t.Fatal(err)
	}
	// This is the exact marker serialization before the optional layout field.
	want := []byte("version: 1\ncreated_at: \"2026-10-07T12:00:00Z\"\n")
	before, err := os.ReadFile(filepath.Join(root, MarkerName))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(want) {
		t.Fatalf("Init marker = %q, want %q", before, want)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	item := Item{ID: "alice"}
	if store.Sidecar(item.ID) != filepath.Join(root, "items", "alice", SidecarName) {
		t.Fatal("by-Item sidecar")
	}
	if store.PDF(item, "digest") != filepath.Join(root, "items", "alice", "digest.pdf") {
		t.Fatal("by-Item PDF")
	}
	if folder, ok := store.Folder(item.ID); !ok || folder != filepath.Join(root, "items", "alice") {
		t.Fatal("by-Item folder")
	}
	if store.Image(item, "digest", Original{Digest: "image", Extension: ".jpg"}) != filepath.Join(root, "items", "alice", "image.jpg") {
		t.Fatal("by-Item image")
	}
	after, err := os.ReadFile(filepath.Join(root, MarkerName))
	if err != nil || string(after) != string(before) {
		t.Fatal("Open changed marker")
	}
}

func TestOpenRefusesEverySuppliedLayout(t *testing.T) {
	for _, field := range []string{
		"layout:\n  path: '{date:YYYY}/{date:MM}'\n  file: '{name}-{digest:12}.{ext}'\n",
		"layout: {}\n",
		"layout: null\n",
		"layout: future-layout\n",
		"layout: {name: in-place}\n",
	} {
		t.Run(strings.TrimSpace(field), func(t *testing.T) {
			root := t.TempDir()
			data := []byte("version: 1\ncreated_at: '2026-10-07T12:00:00Z'\n" + field)
			if err := os.WriteFile(filepath.Join(root, MarkerName), data, 0o644); err != nil {
				t.Fatal(err)
			}
			store, err := Open(root)
			if store != nil || !errors.Is(err, ErrUnsupportedLayout) || !strings.Contains(err.Error(), "layout is not supported yet") {
				t.Fatalf("Open = %v, %v", store, err)
			}
			after, readErr := os.ReadFile(filepath.Join(root, MarkerName))
			if readErr != nil || string(after) != string(data) {
				t.Fatal("Open changed unsupported marker")
			}
		})
	}
}

func TestMarkerRejectsInvalidOutlineVersionsWithoutWriting(t *testing.T) {
	for _, value := range []string{"0", "3", "null", "'2'", "{}", "skeleton"} {
		root := t.TempDir()
		data := []byte("version: 1\noutlines: " + value + "\n")
		if err := os.WriteFile(filepath.Join(root, MarkerName), data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(root); err == nil {
			t.Fatal("invalid outlines accepted", value)
		}
		after, err := os.ReadFile(filepath.Join(root, MarkerName))
		if err != nil || string(after) != string(data) {
			t.Fatal("marker rewritten", err)
		}
	}
}

func TestInPlaceKeepsMetadataUnderMetaDir(t *testing.T) {
	root := t.TempDir()
	if err := InitWithLayout(root, time.Unix(0, 0), "in-place"); err != nil {
		t.Fatal(err)
	}
	st, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.WriteChangeMark(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != MetaDir {
		t.Fatal("root holds more than", MetaDir, entries, err)
	}
	if st.Sidecar("a") != filepath.Join(root, MetaDir, ItemsDir, "a.dgs-item.yaml") {
		t.Fatal("sidecar", st.Sidecar("a"))
	}
	if _, err := cleanRecorded(MetaDir + "/x.pdf"); err == nil {
		t.Fatal("recorded path inside", MetaDir)
	}
	if err := os.WriteFile(filepath.Join(root, MarkerName), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root); err == nil {
		t.Fatal("both markers accepted")
	}
}
