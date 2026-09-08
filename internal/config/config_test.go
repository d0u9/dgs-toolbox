package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingConfigUsesAllTopBarMetrics(t *testing.T) {
	t.Setenv(EnvPath, filepath.Join(t.TempDir(), "missing.json"))
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := config.TopBarVisibility(); !got.Disk || !got.Network || !got.CPU || !got.Time {
		t.Fatalf("visibility = %#v", got)
	}
}

func TestPartialConfigOnlyOverridesNamedMetrics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"tui":{"top_bar":{"network":false,"time":false}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvPath, path)
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	got := config.TopBarVisibility()
	if !got.Disk || got.Network || !got.CPU || got.Time {
		t.Fatalf("visibility = %#v", got)
	}
}

func TestExportDefaultCreatesEditableConfigWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	t.Setenv(EnvPath, path)
	exported, err := ExportDefault(path)
	if err != nil || exported != path {
		t.Fatalf("exported=%q err=%v", exported, err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	visibility := loaded.TopBarVisibility()
	if !visibility.Disk || !visibility.Network || !visibility.CPU || !visibility.Time {
		t.Fatalf("visibility = %#v", visibility)
	}
	if _, err := ExportDefault(path); err == nil {
		t.Fatal("second export overwrote existing config")
	}
}

func TestDefaultExportPathUsesCurrentWorkingDirectory(t *testing.T) {
	t.Setenv(EnvPath, "")
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path, err := ExportPath("")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(directory, ExportFilename) {
		t.Fatalf("export path = %q", path)
	}
}

func TestExportPathAcceptsFileOrExistingDirectory(t *testing.T) {
	directory := t.TempDir()
	path, err := ExportPath(directory)
	if err != nil || path != filepath.Join(directory, ExportFilename) {
		t.Fatalf("directory export path=%q err=%v", path, err)
	}
	want := filepath.Join(directory, "custom.json")
	path, err = ExportPath(want)
	if err != nil || path != want {
		t.Fatalf("file export path=%q err=%v", path, err)
	}
}
