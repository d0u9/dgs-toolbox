package vaultmove

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func entry(t *testing.T, root, rel string) {
	t.Helper()
	write(t, filepath.Join(root, filepath.FromSlash(rel)), "age")
	write(t, filepath.Join(root, filepath.FromSlash(rel))+".json", `{"version":1}`)
}

func TestMoveRenamesFileAndRecord(t *testing.T) {
	root := t.TempDir()
	entry(t, root, "top.age")

	result, err := Move(root, "top.age", "servers/nas.age")
	if err != nil {
		t.Fatal(err)
	}
	if result.To != "servers/nas.age" || result.Record != "servers/nas.age.json" {
		t.Errorf("result %+v", result)
	}
	if strings.Join(result.CreatedDirs, ",") != "servers" {
		t.Errorf("created %v", result.CreatedDirs)
	}
	for _, name := range []string{"servers/nas.age", "servers/nas.age.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "top.age")); err == nil {
		t.Error("source still there")
	}
}

func TestMoveWithoutRecord(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "lone.age"), "age")
	result, err := Move(root, "lone.age", "kept.age")
	if err != nil || result.Record != "" {
		t.Fatalf("result %+v err %v", result, err)
	}
}

func TestMoveRefusals(t *testing.T) {
	root := t.TempDir()
	entry(t, root, "top.age")
	entry(t, root, "servers/nas.age")

	for name, test := range map[string]struct{ from, to, fragment string }{
		"exists":     {"top.age", "servers/nas.age", "already exists"},
		"same":       {"top.age", "top.age", "already there"},
		"suffix":     {"top.age", "top.txt", "must end in .age"},
		"hidden":     {"top.age", ".top.age", "starting with a dot"},
		"folder":     {"top.age", "a/b/../../../out.age", "leaves the vault"},
		"hidden dir": {"top.age", ".git/top.age", "is hidden"},
		"missing":    {"gone.age", "other.age", "no such file"},
	} {
		if _, err := Move(root, test.from, test.to); err == nil || !strings.Contains(err.Error(), test.fragment) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A refused move leaves both entries where they were.
	for _, name := range []string{"top.age", "top.age.json", "servers/nas.age", "servers/nas.age.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestMoveRefusesLeavingTheVault(t *testing.T) {
	root := t.TempDir()
	entry(t, root, "top.age")
	if _, err := Move(root, "top.age", "../escaped.age"); err == nil {
		t.Fatal("escape accepted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escaped.age")); err == nil {
		t.Error("file left the vault")
	}
}
