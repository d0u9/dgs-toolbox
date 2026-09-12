package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

// The file-based configuration is laid out by command under one root, and the
// root defaults to the directory of the file that was loaded rather than the
// location the operating system would have chosen: --config points at a whole
// configuration.
func TestCaptureDirectoriesFollowTheLoadedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dgs-config.json")
	if err := os.WriteFile(path, []byte(`{"capture": {"scan": {"index_file": "index.json"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvPath, filepath.Join(t.TempDir(), "elsewhere", "config.json"))

	loaded, err := LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.CaptureRecipesDir(); got != filepath.Join(dir, "capture", "recipes") {
		t.Fatalf("recipes = %q, want them under the command's own directory", got)
	}
	if got := loaded.CaptureTemplatesDir(); got != filepath.Join(dir, "capture", "templates") {
		t.Fatalf("templates = %q, want them under the command's own directory", got)
	}

	if got := loaded.CaptureWorkflowsDir(); got != filepath.Join(dir, "capture", "workflows") {
		t.Fatalf("workflows = %q, want them under the command's own directory", got)
	}

	// The layout is not negotiable: a path for each directory would make
	// "where does this installation keep its configuration?" three answers.
	if err := os.WriteFile(path, []byte(`{"capture": {"recipes": "/somewhere/else"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPath(path); err == nil {
		t.Fatal("a path of its own was accepted, want the unknown key refused")
	}
}

// config_dir moves the whole file-based configuration, and each command reads
// its own corner of it. dgs is a toolbox: two commands both wanting
// "templates" is the normal case, not a collision to work around.
func TestConfigDirIsLaidOutByCommand(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()
	path := filepath.Join(dir, "dgs-config.json")
	body := `{"config_dir": ` + strconv.Quote(root) + `}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Dir(); got != root {
		t.Fatalf("dir = %q, want the configured root", got)
	}
	if got := loaded.AppDir("photo"); got != filepath.Join(root, "photo") {
		t.Fatalf("photo dir = %q", got)
	}
	if got := loaded.CaptureRecipesDir(); got != filepath.Join(root, "capture", "recipes") {
		t.Fatalf("recipes = %q", got)
	}
	if got := loaded.CaptureTemplatesDir(); got != filepath.Join(root, "capture", "templates") {
		t.Fatalf("templates = %q", got)
	}

	if got := loaded.CaptureWorkflowsDir(); got != filepath.Join(root, "capture", "workflows") {
		t.Fatalf("workflows = %q", got)
	}
}

// The XDG directory is where a configuration is kept by hand, so it is looked
// at before the operating system's own location.
func TestXDGConfigIsPreferredOverTheOperatingSystemLocation(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv(EnvPath, "")
	t.Setenv(EnvXDGConfigHome, xdg)

	if err := os.MkdirAll(filepath.Join(xdg, XDGDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	wanted := filepath.Join(xdg, XDGDirName, ExportFilename)
	if got, err := Path(); err != nil || got != wanted {
		t.Fatalf("path = %q, %v, want %q even before the file exists", got, err, wanted)
	}
	if err := os.WriteFile(wanted, []byte(`{"tui":{"top_bar":{"cpu":false}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.TopBarVisibility().CPU {
		t.Fatal("the XDG configuration was not the one loaded")
	}
	if want := filepath.Join(xdg, XDGDirName); config.Dir() != want {
		t.Fatalf("config dir = %q, want %q — the folder holding the whole configuration", config.Dir(), want)
	}
}

// Nothing in the XDG directory falls through to the operating system's
// location, which is where a file written there still works from. The file
// itself is not created: it is the reader's real configuration, and a test has
// no business writing over it.
func TestOperatingSystemLocationFollowsTheXDGOne(t *testing.T) {
	t.Setenv(EnvPath, "")
	t.Setenv(EnvXDGConfigHome, t.TempDir())

	candidates, err := Candidates()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(candidates[0])) != XDGDirName {
		t.Fatalf("XDG candidate = %q, want it in the toolbox's own folder", candidates[0])
	}
	if len(candidates) != 2 || filepath.Base(candidates[1]) != "config.json" || filepath.Base(filepath.Dir(candidates[1])) != "dgs" {
		t.Fatalf("candidates = %q, want the XDG file then the operating system's", candidates)
	}
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if path != candidates[0] {
		t.Fatalf("path = %q, want the first candidate when none exists", path)
	}
}

// The examples are configurations someone will copy, so a key renamed without
// them is a file that fails on their first run and looks like their mistake.
// Loading rejects an unknown key, which is exactly the drift worth catching.
func TestExampleConfigurationsLoad(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "examples")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), ExportFilename)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("examples/%s holds no %s", entry.Name(), ExportFilename)
			continue
		}
		if _, err := LoadPath(path); err != nil {
			t.Errorf("examples/%s: %v", entry.Name(), err)
		}
		found++
	}
	if found == 0 {
		t.Fatal("no example configurations were found")
	}
}
