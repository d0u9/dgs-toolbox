package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestDocTargetsSayWhereTheyWent(t *testing.T) {
	_, err := LoadPath(writeConfig(t, `{"doc": {"targets": {"icloud": "~/Docs"}}}`))
	if err == nil || !strings.Contains(err.Error(), "targets.yaml") {
		t.Fatalf("doc.targets: %v", err)
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
	if _, err := LoadPath(writeConfig(t, `{"doc": {"root": "/x", "trees": {"a": "/a"}}}`)); err == nil {
		t.Fatal("root and trees together")
	}
}
