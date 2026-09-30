package config

import (
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/doc/dates"
)

func TestDocDateOrder(t *testing.T) {
	config, err := LoadPath(writeConfig(t, `{}`))
	if err != nil || config.DocDateOrder() != dates.DMY {
		t.Fatalf("default: %v %v", err, config.DocDateOrder())
	}
	config, err = LoadPath(writeConfig(t, `{"doc": {"date_order": "mdy"}}`))
	if err != nil || config.DocDateOrder() != dates.MDY {
		t.Fatalf("mdy: %v %v", err, config.DocDateOrder())
	}
	if _, err := LoadPath(writeConfig(t, `{"doc": {"date_order": "YMD"}}`)); err == nil {
		t.Fatal("YMD accepted")
	}
}

func TestDocOldKeysAreRefused(t *testing.T) {
	for _, body := range []string{
		`{"doc": {"targets": {"icloud": "~/Docs"}}}`,
		`{"doc": {"root": "/x"}}`,
		`{"doc": {"cache_dir": "/x"}}`,
	} {
		if _, err := LoadPath(writeConfig(t, body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestDocTrees(t *testing.T) {
	home, _ := os.UserHomeDir()
	config, err := LoadPath(writeConfig(t, `{"doc": {"trees": {"papers": "~/P", "books": "/B"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	trees := config.DocTrees()
	if len(trees) != 2 || trees[0].Name != "books" || trees[1].Root != filepath.Join(home, "P") {
		t.Fatalf("%+v", trees)
	}
	if _, err := config.DocTreeNamed(""); err == nil {
		t.Fatal("no name chose among two")
	}
	if tr, err := config.DocTreeNamed("papers"); err != nil || tr.Name != "papers" {
		t.Fatalf("%+v %v", tr, err)
	}
	one, err := LoadPath(writeConfig(t, `{"doc": {"trees": {"papers": "/P"}}}`))
	if err != nil || one.DocRoot() != "/P" {
		t.Fatalf("one tree: root %q, %v", one.DocRoot(), err)
	}
	if config.DocRoot() != "" {
		t.Fatalf("two trees: root %q, want none", config.DocRoot())
	}
}
