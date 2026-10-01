package obsidian

import (
	"dgs-toolbox/internal/plugins"

	"os"
	"path/filepath"
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

func TestEveryConfigurationFolderIsATarget(t *testing.T) {
	vault := t.TempDir()
	write(t, filepath.Join(vault, ".obsidian", "app.json"), "{}")
	write(t, filepath.Join(vault, ".obsidian", "community-plugins.json"), `["dgs-toolbox"]`)
	write(t, filepath.Join(vault, ".obsidian-mobile", "app.json"), "{}")
	write(t, filepath.Join(vault, ".git", "HEAD"), "ref")
	write(t, filepath.Join(vault, "notes", "app.json"), "{}")

	targets, err := Targets(vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 3 {
		t.Fatalf("%d targets: %+v", len(targets), targets)
	}
	if files := targets[2]; files.Kind != plugins.KindFiles || files.Dir != vault {
		t.Errorf("the vault's own target: %+v", files)
	}
	targets = targets[:2]
	want := []struct {
		dir     string
		enabled bool
	}{{".obsidian", true}, {".obsidian-mobile", false}}
	for index, target := range targets {
		if target.Dir != filepath.Join(vault, want[index].dir, "plugins") {
			t.Errorf("target %d dir %s", index, target.Dir)
		}
		enabled, err := target.Enabled("dgs-toolbox")
		if err != nil || enabled != want[index].enabled {
			t.Errorf("target %d enabled %v %v", index, enabled, err)
		}
	}
}

func TestAVaultObsidianNeverOpenedIsRefused(t *testing.T) {
	if _, err := Targets(t.TempDir()); err == nil {
		t.Fatal("no error")
	}
}
