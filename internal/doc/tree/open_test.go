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
