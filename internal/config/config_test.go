package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMissingConfigUsesAllTopBarMetrics(t *testing.T) {
	t.Setenv(EnvPath, filepath.Join(t.TempDir(), "missing"))
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := config.TopBarVisibility(); !got.Disk || !got.Network || !got.CPU || !got.Time {
		t.Fatalf("visibility = %#v", got)
	}
}

func TestPartialConfigOnlyOverridesNamedMetrics(t *testing.T) {
	t.Setenv(EnvPath, writeConfig(t, `{"shell":{"top_bar":{"network":false,"time":false}}}`))
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
	loaded, err := LoadPath(writeConfig(t, `{"photo":{"import":{"state_file":".photo-import-state"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.PhotoImportStateFile(); got != ".photo-import-state" {
		t.Fatalf("configured state file = %q", got)
	}
}

func TestPhotoImportStateFileRejectsPaths(t *testing.T) {
	if _, err := LoadPath(writeConfig(t, `{"photo":{"import":{"state_file":"nested/state"}}}`)); err == nil {
		t.Fatal("state file path was accepted; only a filename is safe")
	}
}

func TestCaptureScanSettingsDefaultAndCanBeConfigured(t *testing.T) {
	root, indexFile := (Config{}).CaptureScanSettings()
	if root != "" || indexFile != "index.json" {
		t.Fatalf("default Capture Scan settings = %q, %q", root, indexFile)
	}
	loaded, err := LoadPath(writeConfig(t, `{"capture":{"scan":{"root":"/captures","index_file":"capture.json"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	root, indexFile = loaded.CaptureScanSettings()
	if root != "/captures" || indexFile != "capture.json" {
		t.Fatalf("configured Capture Scan settings = %q, %q", root, indexFile)
	}
}

// The two Archive folders are configuration and nothing else — there is no
// picker for them in the session — so the key names are the whole interface.
func TestCaptureArchiveFoldersDefaultAndCanBeConfigured(t *testing.T) {
	if archive, reject := (Config{}).CaptureArchiveFolders(); archive != "" || reject != "" {
		t.Fatalf("default Archive folders = %q, %q; want neither set", archive, reject)
	}
	loaded, err := LoadPath(writeConfig(t, `{"capture":{"archive":{"root":"/filed","reject":"/set-aside"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if archive, reject := loaded.CaptureArchiveFolders(); archive != "/filed" || reject != "/set-aside" {
		t.Fatalf("configured Archive folders = %q, %q", archive, reject)
	}
}

func TestCaptureObsidianIsOneObjectPerNote(t *testing.T) {
	loaded, err := LoadPath(writeConfig(t, `{"capture":{"obsidian":{"vault":"/v",
		"daily":{"note":"D/{{.Date}}.md","section":"S","images":{"max_side":100}},
		"location":{"note":"L.md","archive":"L"},"timeline":{"note":"T.md"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	o := loaded.CaptureObsidian()
	if o.Daily.Note != "D/{{.Date}}.md" || o.Daily.Section != "S" || o.Daily.Images.MaxSide != 100 ||
		o.Location.Note != "L.md" || o.Location.Archive != "L" || o.Timeline.Note != "T.md" {
		t.Fatalf("obsidian = %+v", o)
	}
	if _, err := LoadPath(writeConfig(t, `{"capture":{"obsidian":{"daily_note":"D.md"}}}`)); err == nil {
		t.Fatal("the old flat key was accepted")
	}
}

func TestCaptureIndexFileRejectsPaths(t *testing.T) {
	if _, err := LoadPath(writeConfig(t, `{"capture":{"scan":{"index_file":"metadata/index.json"}}}`)); err == nil {
		t.Fatal("Capture index file path was accepted; only a filename is safe")
	}
}

func TestLoadDirOverridesEnvironmentConfig(t *testing.T) {
	t.Setenv(EnvPath, writeConfig(t, `{"photo":{"import":{"source":"/environment/source"}}}`))
	loaded, err := LoadPath(writeConfig(t, `{"photo":{"import":{"source":"/explicit/source","destination":"/explicit/destination"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	source, destination := loaded.PhotoImportPaths()
	if source != "/explicit/source" || destination != "/explicit/destination" {
		t.Fatalf("paths = %q, %q", source, destination)
	}
}

// A broken file refuses only the command it belongs to: the commands are
// independent, and a typo in one should not take the others down.
func TestABrokenPartRefusesOnlyItsCommand(t *testing.T) {
	loaded, err := LoadDir(writeConfig(t, `{"box":{"nope":1},"photo":{"import":{"source":"/s"}}}`))
	if err != nil {
		t.Fatalf("LoadDir failed as a whole: %v", err)
	}
	if err := loaded.PartErr("box"); err == nil || !strings.Contains(err.Error(), filepath.Join("box", PartFile)) {
		t.Fatalf("box error = %v, want it to name box/config.json", err)
	}
	if err := loaded.PartErr("photo"); err != nil {
		t.Fatalf("photo refused for box's mistake: %v", err)
	}
	if source, _ := loaded.PhotoImportPaths(); source != "/s" {
		t.Fatalf("photo source = %q", source)
	}
}

// The shell's file is read by the shell itself, so a broken one stops dgs.
func TestABrokenShellPartStopsEverything(t *testing.T) {
	if _, err := LoadDir(writeConfig(t, `{"shell":{"tui":{}}}`)); err == nil {
		t.Fatal("a broken shell file was accepted")
	}
}

// The single-file layout this one replaced is refused by name rather than
// silently ignored: settings someone wrote must not quietly stop applying.
func TestTheOldSingleFileIsRefused(t *testing.T) {
	for _, name := range []string{"dgs-config.json", "credentials.json"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadDir(dir); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: err = %v, want it refused by name", name, err)
		}
	}
}

func TestConfigNamesADirectoryNotAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDir(path); err == nil {
		t.Fatal("a file was accepted as the configuration directory")
	}
}

func TestExportDefaultWritesEveryPartWithoutOverwrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	written, skipped, err := ExportDefault(dir)
	if err != nil || len(skipped) != 0 || len(written) != len(parts) {
		t.Fatalf("written=%v skipped=%v err=%v", written, skipped, err)
	}
	loaded, err := LoadPath(dir)
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
	if got := loaded.BoxWebAddr(); got != "127.0.0.1:8766" {
		t.Fatalf("exported box web = %q", got)
	}
	if _, _, err := LoadCredentials(loaded.CredentialsPath()); err != nil {
		t.Fatalf("exported cred file: %v", err)
	}
	box := filepath.Join(dir, "box", PartFile)
	if err := os.WriteFile(box, []byte(`{"workers": 2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	written, skipped, err = ExportDefault(dir)
	if err != nil || len(written) != 0 || len(skipped) != len(parts) {
		t.Fatalf("second export: written=%v skipped=%v err=%v", written, skipped, err)
	}
	if data, _ := os.ReadFile(box); string(data) != `{"workers": 2}` {
		t.Fatalf("second export overwrote box: %s", data)
	}
}

// Each command reads its own folder of the configuration directory. dgs is a
// toolbox: two commands both wanting "templates" is the normal case, not a
// collision to work around, and a folder is what a command takes with it if
// it leaves.
func TestTheConfigurationIsLaidOutByCommand(t *testing.T) {
	dir := writeConfig(t, `{"capture": {"scan": {"index_file": "index.json"}}}`)
	t.Setenv(EnvPath, filepath.Join(t.TempDir(), "elsewhere"))

	loaded, err := LoadPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	for got, want := range map[string]string{
		loaded.CaptureRecipesDir():   filepath.Join(dir, "capture", "recipes"),
		loaded.CaptureTemplatesDir(): filepath.Join(dir, "capture", "templates"),
		loaded.CaptureWorkflowsDir(): filepath.Join(dir, "capture", "workflows"),
		loaded.AppDir("photo"):       filepath.Join(dir, "photo"),
		loaded.CredentialsPath():     filepath.Join(dir, "cred", PartFile),
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}

	// The layout is not negotiable: a path for each directory would make
	// "where does this installation keep its configuration?" three answers.
	if _, err := LoadPath(writeConfig(t, `{"capture": {"recipes": "/somewhere/else"}}`)); err == nil {
		t.Fatal("a path of its own was accepted, want the unknown key refused")
	}
}

// There is one default location, and it is in the XDG directory whether or not
// anything is there yet: "which file am I editing?" must not have an answer
// that depends on which files exist.
func TestTheDefaultLocationIsTheXDGOne(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv(EnvPath, "")
	t.Setenv(EnvXDGConfigHome, xdg)

	wanted := filepath.Join(xdg, XDGDirName)
	if got, err := Dir(); err != nil || got != wanted {
		t.Fatalf("dir = %q, %v, want %q even before it exists", got, err, wanted)
	}
	shell := filepath.Join(wanted, "shell", PartFile)
	if err := os.MkdirAll(filepath.Dir(shell), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shell, []byte(`{"top_bar":{"cpu":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.TopBarVisibility().CPU {
		t.Fatal("the XDG configuration was not the one loaded")
	}
	if config.Dir() != wanted {
		t.Fatalf("config dir = %q, want %q", config.Dir(), wanted)
	}
	if path, err := CredentialsPath(); err != nil || path != filepath.Join(wanted, "cred", PartFile) {
		t.Fatalf("credentials path = %q, %v", path, err)
	}
}

// With no XDG_CONFIG_HOME the default is the same path under ~/.config, which
// is what that variable means when it is unset.
func TestTheDefaultLocationFallsBackToHomeConfig(t *testing.T) {
	t.Setenv(EnvPath, "")
	t.Setenv(EnvXDGConfigHome, "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	got, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", XDGDirName); got != want {
		t.Fatalf("dir = %q, want %q", got, want)
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
		dir := filepath.Join(root, entry.Name())
		held := false
		for _, p := range parts {
			if _, err := os.Stat(filepath.Join(dir, p.name, PartFile)); err == nil {
				held = true
			}
		}
		if !held {
			t.Errorf("examples/%s holds no <command>/%s", entry.Name(), PartFile)
			continue
		}
		loaded, err := LoadPath(dir)
		if err != nil {
			t.Errorf("examples/%s: %v", entry.Name(), err)
			continue
		}
		if _, _, err := LoadCredentials(loaded.CredentialsPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("examples/%s: %v", entry.Name(), err)
		}
		found++
	}
	if found == 0 {
		t.Fatal("no example configurations were found")
	}
}

func TestGeoGPXTilesAreValidated(t *testing.T) {
	for name, body := range map[string]string{
		"no name":        `{"geo":{"gpx":{"tiles":[{"url":"https://t/{z}/{x}/{y}.png"}]}}}`,
		"no placeholder": `{"geo":{"gpx":{"tiles":[{"name":"t","url":"https://t/{z}/{x}.png"}]}}}`,
		"coordinates":    `{"geo":{"gpx":{"tiles":[{"name":"t","url":"https://t/{z}/{x}/{y}.png","coordinates":"bd09"}]}}}`,
	} {
		if _, err := LoadPath(writeConfig(t, body)); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

func TestGeoGPXTilesFileAddsTiles(t *testing.T) {
	dir := t.TempDir()
	config := Config{dir: dir, Geo: Geo{GPX: GeoGPX{Tiles: []GeoGPXTile{{Name: "inline", URL: "https://i/{z}/{x}/{y}.png"}}}}}
	if tiles, err := config.GeoGPXTiles(); err != nil || len(tiles) != 1 {
		t.Fatalf("without a file: %v %v", tiles, err)
	}
	path := filepath.Join(dir, "geo", "gpx", "tiles.json")
	if config.GeoGPXTilesPath() != path {
		t.Fatalf("path = %q", config.GeoGPXTilesPath())
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(`{"tiles":[{"name":"file","url":"https://f/{z}/{x}/{y}.png","coordinates":"gcj02"}]}`), 0o600)
	tiles, err := config.GeoGPXTiles()
	if err != nil || len(tiles) != 2 || tiles[1].Name != "file" || tiles[1].Coordinates != "gcj02" {
		t.Fatalf("with a file: %v %v", tiles, err)
	}
	os.WriteFile(path, []byte(`{"tiles":[{"name":"bad","url":"https://f/{z}.png"}]}`), 0o600)
	if _, err := config.GeoGPXTiles(); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("invalid file: %v", err)
	}
}

func TestGeoGPXRootExpandsHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := (Config{}).GeoGPXRoot(); got != home {
		t.Fatalf("default root = %q", got)
	}
	config := Config{Geo: Geo{GPX: GeoGPX{Root: "~/tracks"}}}
	if got := config.GeoGPXRoot(); got != filepath.Join(home, "tracks") {
		t.Fatalf("root = %q", got)
	}
}
