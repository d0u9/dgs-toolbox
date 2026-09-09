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

func TestLegacyEnvironmentVariableIsIgnored(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "legacy.json")
	t.Setenv("DGS_CONFIG", legacyPath)
	t.Setenv(EnvPath, "")
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if path == legacyPath {
		t.Fatalf("legacy environment variable still selected %q", path)
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

func TestPhotoImportStateFileDefaultsAndCanBeConfigured(t *testing.T) {
	if got := (Config{}).PhotoImportStateFile(); got != ".dgs-state" {
		t.Fatalf("default state file = %q", got)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"photo":{"import":{"state_file":".photo-import-state"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvPath, path)
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.PhotoImportStateFile(); got != ".photo-import-state" {
		t.Fatalf("configured state file = %q", got)
	}
}

func TestPhotoImportStateFileRejectsPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"photo":{"import":{"state_file":"nested/state"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvPath, path)
	if _, err := Load(); err == nil {
		t.Fatal("state file path was accepted; only a filename is safe")
	}
}

func TestCaptureScanSettingsDefaultAndCanBeConfigured(t *testing.T) {
	root, indexFile := (Config{}).CaptureScanSettings()
	if root != "" || indexFile != "index.json" {
		t.Fatalf("default Capture Scan settings = %q, %q", root, indexFile)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"capture":{"scan":{"root":"/captures","index_file":"capture.json"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	root, indexFile = loaded.CaptureScanSettings()
	if root != "/captures" || indexFile != "capture.json" {
		t.Fatalf("configured Capture Scan settings = %q, %q", root, indexFile)
	}
}

func TestCaptureIndexFileRejectsPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"capture":{"scan":{"index_file":"metadata/index.json"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPath(path); err == nil {
		t.Fatal("Capture index file path was accepted; only a filename is safe")
	}
}

func TestLoadPathOverridesEnvironmentConfig(t *testing.T) {
	directory := t.TempDir()
	environmentPath := filepath.Join(directory, "environment.json")
	explicitPath := filepath.Join(directory, "explicit.json")
	if err := os.WriteFile(environmentPath, []byte(`{"photo":{"import":{"source":"/environment/source"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(explicitPath, []byte(`{"photo":{"import":{"source":"/explicit/source","destination":"/explicit/destination"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvPath, environmentPath)
	loaded, err := LoadPath(explicitPath)
	if err != nil {
		t.Fatal(err)
	}
	source, destination := loaded.PhotoImportPaths()
	if source != "/explicit/source" || destination != "/explicit/destination" {
		t.Fatalf("paths = %q, %q", source, destination)
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
	if loaded.PhotoImportStateFile() != ".dgs-state" {
		t.Fatalf("exported state file = %q", loaded.PhotoImportStateFile())
	}
	if root, indexFile := loaded.CaptureScanSettings(); root != "" || indexFile != "index.json" {
		t.Fatalf("exported Capture Scan settings = %q, %q", root, indexFile)
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
