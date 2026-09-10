// Package config loads process-wide dgs configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const EnvPath = "DGS_TOOLBOX_CONFIG"
const ExportFilename = "dgs-config.json"

type Config struct {
	TUI     TUI     `json:"tui"`
	Photo   Photo   `json:"photo"`
	Capture Capture `json:"capture"`
	// dir is the directory the configuration was loaded from. Paths that
	// default to sitting beside the configuration resolve against this rather
	// than against the operating system's location, so --config points at a
	// whole configuration and not only at one file of it.
	dir string
}

// Capture configures the Capture command. Route organizes a Capture through
// the Recipes and Actions of the organizer model rather than through configured
// destinations, so it has no settings of its own.
type Capture struct {
	Scan CaptureScan `json:"scan"`
	// Recipes is the directory holding one file per organizer Recipe. Empty
	// means the "recipes" directory beside the configuration file.
	Recipes string `json:"recipes"`
	// Templates holds templates overriding the compiled-in ones by filename.
	// Empty means the "templates" directory beside the configuration file,
	// which is where recipes live too.
	Templates string          `json:"templates"`
	Obsidian  CaptureObsidian `json:"obsidian"`
}

// CaptureObsidian tells the Obsidian Actions where to write. Vault is an
// absolute path; the folders are relative to it, so what an Action plans and
// records stays vault-relative and survives the vault moving.
type CaptureObsidian struct {
	Vault string `json:"vault"`
	// Section is the heading a Capture is written under in a daily note.
	Section   string `json:"section"`
	Locations string `json:"locations"`
}

type CaptureScan struct {
	Root      string `json:"root"`
	IndexFile string `json:"index_file"`
}

type Photo struct {
	Import PhotoImport `json:"import"`
}

type PhotoImport struct {
	StateFile   string `json:"state_file"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type TUI struct {
	TopBar TopBar `json:"top_bar"`
}

type TopBar struct {
	Disk    *bool `json:"disk"`
	Network *bool `json:"network"`
	CPU     *bool `json:"cpu"`
	Time    *bool `json:"time"`
}

type TopBarVisibility struct {
	Disk, Network, CPU, Time bool
}

func boolPointer(value bool) *bool { return &value }

func Default() Config {
	return Config{TUI: TUI{TopBar: TopBar{
		Disk: boolPointer(true), Network: boolPointer(true),
		CPU: boolPointer(true), Time: boolPointer(true),
	}}, Photo: Photo{Import: PhotoImport{StateFile: ".dgs-state"}}, Capture: Capture{
		Scan:     CaptureScan{IndexFile: "index.json"},
		Obsidian: CaptureObsidian{Locations: "Locations"},
	}}
}

func (c Config) CaptureScanSettings() (root, indexFile string) {
	indexFile = c.Capture.Scan.IndexFile
	if indexFile == "" {
		indexFile = "index.json"
	}
	return c.Capture.Scan.Root, indexFile
}

// CaptureTemplatesDir is where Capture reads templates that replace the
// compiled-in ones. Like the Recipe directory it sits beside the configuration
// file by default rather than at a path fixed by the tool.
func (c Config) CaptureTemplatesDir() string {
	return c.captureDir(c.Capture.Templates, "templates")
}

// CaptureRecipesDir is where Capture reads user-defined Recipes. It defaults to
// a directory beside the configuration file, so a user who has a config has a
// place to put Recipes without configuring a second path.
func (c Config) CaptureRecipesDir() string {
	return c.captureDir(c.Capture.Recipes, "recipes")
}

// captureDir resolves a configured directory, falling back to one named beside
// the configuration file that was actually loaded — not beside the one the
// operating system would have chosen. A run with --config points at a whole
// configuration, and its recipes and templates belong to it.
func (c Config) captureDir(configured, name string) string {
	if configured != "" {
		return configured
	}
	dir := c.dir
	if dir == "" {
		path, err := Path()
		if err != nil {
			return ""
		}
		dir = filepath.Dir(path)
	}
	return filepath.Join(dir, name)
}

// CaptureObsidian returns the Obsidian settings as configured. Defaults are
// applied by the organizer, which owns what they are; an unset vault is left
// empty rather than guessed, because writing into a directory nobody named is
// worse than refusing to write.
func (c Config) CaptureObsidian() CaptureObsidian { return c.Capture.Obsidian }

func (c Config) PhotoImportStateFile() string {
	if c.Photo.Import.StateFile == "" {
		return ".dgs-state"
	}
	return c.Photo.Import.StateFile
}

func (c Config) PhotoImportPaths() (source, destination string) {
	return c.Photo.Import.Source, c.Photo.Import.Destination
}

func DefaultTopBarVisibility() TopBarVisibility {
	return TopBarVisibility{Disk: true, Network: true, CPU: true, Time: true}
}

func (c Config) TopBarVisibility() TopBarVisibility {
	visibility := DefaultTopBarVisibility()
	apply := func(value *bool, target *bool) {
		if value != nil {
			*target = *value
		}
	}
	apply(c.TUI.TopBar.Disk, &visibility.Disk)
	apply(c.TUI.TopBar.Network, &visibility.Network)
	apply(c.TUI.TopBar.CPU, &visibility.CPU)
	apply(c.TUI.TopBar.Time, &visibility.Time)
	return visibility
}

func Path() (string, error) {
	if path := os.Getenv(EnvPath); path != "" {
		return path, nil
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "dgs", "config.json"), nil
}

func ExportPath(destination string) (string, error) {
	if destination != "" {
		info, err := os.Stat(destination)
		if err == nil && info.IsDir() {
			return filepath.Join(destination, ExportFilename), nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return destination, nil
	}
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, ExportFilename), nil
}

// Load returns defaults when the global file does not exist. A malformed file
// is reported instead of silently ignoring a user's intended switches.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, fmt.Errorf("resolve config path: %w", err)
	}
	return LoadPath(path)
}

// LoadPath loads an explicit configuration file. It intentionally bypasses
// DGS_TOOLBOX_CONFIG so command-line configuration can override the environment.
func LoadPath(path string) (Config, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{dir: filepath.Dir(path)}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()
	var config Config
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode config %s: %w", path, err)
	}
	stateFile := config.PhotoImportStateFile()
	if filepath.Base(stateFile) != stateFile || stateFile == "." || stateFile == ".." {
		return Config{}, fmt.Errorf("decode config %s: photo.import.state_file must be a filename, got %q", path, stateFile)
	}
	_, indexFile := config.CaptureScanSettings()
	if filepath.Base(indexFile) != indexFile || indexFile == "." || indexFile == ".." {
		return Config{}, fmt.Errorf("decode config %s: capture.scan.index_file must be a filename, got %q", path, indexFile)
	}
	config.dir = filepath.Dir(path)
	return config, nil
}

// ExportDefault writes a complete, editable default file without replacing an
// existing user configuration.
func ExportDefault(destination string) (string, error) {
	path, err := ExportPath(destination)
	if err != nil {
		return "", fmt.Errorf("resolve export path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(Default(), "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode default config: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("config already exists: %s", path)
	}
	if err != nil {
		return "", fmt.Errorf("create config %s: %w", path, err)
	}
	removeIncomplete := true
	defer func() {
		if removeIncomplete {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write config %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("sync config %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close config %s: %w", path, err)
	}
	removeIncomplete = false
	return path, nil
}
