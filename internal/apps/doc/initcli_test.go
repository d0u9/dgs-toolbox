package doc

import (
	"bytes"
	"dgs-toolbox/internal/doc/tree"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/config"
)

func TestInitAction(t *testing.T) {
	root := filepath.Join(t.TempDir(), "docs")
	var out bytes.Buffer
	if err := initAction(nil, &out, []string{root}, nil, config.Default()); err != nil {
		t.Fatal(err)
	}
	if err := testStore(t, root).Require(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("id_card.yaml")) {
		t.Errorf("output = %s", out.String())
	}
	if err := initAction(nil, &out, []string{root}, nil, config.Default()); err == nil {
		t.Fatal("second init succeeded")
	}
}

func TestInitInPlaceFlag(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if err := initAction(nil, &out, []string{root}, map[string]string{"layout": "in-place"}, config.Default()); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(root, tree.MetaDir, tree.MetaMarker))
	if err != nil || !strings.Contains(string(marker), "layout: in-place") {
		t.Fatalf("marker: %s %v", marker, err)
	}
	store, err := tree.Open(root)
	if err != nil || store.Sidecar("alice") != filepath.Join(root, tree.MetaDir, "items", "alice.dgs-item.yaml") {
		t.Fatal("open", err)
	}
	rejected := filepath.Join(t.TempDir(), "new-tree")
	if err := initAction(nil, &out, []string{rejected}, map[string]string{"layout": "future"}, config.Default()); err == nil {
		t.Fatal("unknown layout accepted")
	}
	if _, err := os.Stat(rejected); !os.IsNotExist(err) {
		t.Fatal("invalid layout wrote a directory")
	}
}
