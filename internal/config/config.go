// Package config loads process-wide dgs configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

const EnvPath = "DGS_TOOLBOX_CONFIG"
const ExportFilename = "dgs-config.json"

type Config struct {
	// ConfigDir is where the file-based configuration lives: the recipes,
	// templates and anything else a command reads from disk rather than from
	// this file. dgs is a toolbox, so it is laid out by command —
	// <config dir>/<command>/<what> — and a command asks for its own corner
	// rather than for a path of its own in here. Empty means the directory the
	// configuration file was loaded from.
	ConfigDir string  `json:"config_dir"`
	TUI       TUI     `json:"tui"`
	Photo     Photo   `json:"photo"`
	Capture   Capture `json:"capture"`
	Geo       Geo     `json:"geo"`
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
	Scan     CaptureScan     `json:"scan"`
	Archive  CaptureArchive  `json:"archive"`
	Obsidian CaptureObsidian `json:"obsidian"`
}

// CaptureObsidian tells the Obsidian Actions where to write. Vault is an
// absolute path; the folders are relative to it, so what an Action plans and
// records stays vault-relative and survives the vault moving.
type CaptureObsidian struct {
	Vault string `json:"vault"`
	// DailyNote is where a day's note lives, relative to the vault, written as
	// a template over the date: "00 Daily Log/{{.Year}}/{{.Date}}.md". It is
	// configured rather than read from the vault, because a vault says where
	// the plugin in use puts notes, which is not the same question.
	DailyNote string `json:"daily_note"`
	// Section is the heading a Capture is written under in a daily note.
	Section string `json:"section"`
	// LocationNote is the running list of places, relative to the vault, and
	// LocationArchive the folder a year that has rolled over is moved into.
	LocationNote    string `json:"location_note"`
	LocationArchive string `json:"location_archive"`
}

// CaptureArchive is where an organized Capture is put away. It is a directory
// rather than a rule: what to keep and where to keep it is the reader's
// filing, not something this tool derives from the Capture.
type CaptureArchive struct {
	Root string `json:"root"`
	// Reject is where a Capture that is not worth keeping is set aside. A
	// second folder rather than a deletion: what a rejected Capture was is
	// still a question its own directory answers, and this tool does not
	// destroy what it did not produce.
	Reject string `json:"reject"`
}

type CaptureScan struct {
	Root      string `json:"root"`
	IndexFile string `json:"index_file"`
}

// Geo configures the Geo app.
type Geo struct {
	GPX GeoGPX `json:"gpx"`
}

// GeoGPX is where the GPX web server listens. Host 0.0.0.0 lets other
// machines reach it. Empty values use the defaults.
type GeoGPX struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// DefaultGeoGPXHost and DefaultGeoGPXPort keep the GPX page local and at a
// stable URL.
const (
	DefaultGeoGPXHost = "127.0.0.1"
	DefaultGeoGPXPort = 8765
)

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
		Obsidian: CaptureObsidian{},
	}, Geo: Geo{GPX: GeoGPX{Host: DefaultGeoGPXHost, Port: DefaultGeoGPXPort}}}
}

// CaptureArchiveFolders are the two directories Archive files into: where
// organized Captures are kept, and where the rest are set aside. Empty means
// the session opens without that folder: Archive browses to one rather than
// refusing, because where things are filed is decided once and then rarely
// again.
func (c Config) CaptureArchiveFolders() (archive, reject string) {
	return c.Capture.Archive.Root, c.Capture.Archive.Reject
}

func (c Config) CaptureScanSettings() (root, indexFile string) {
	indexFile = c.Capture.Scan.IndexFile
	if indexFile == "" {
		indexFile = "index.json"
	}
	return c.Capture.Scan.Root, indexFile
}

// Dir is the root of the file-based configuration: the configured directory, or
// the one the configuration file itself was loaded from. A run with --config
// points at a whole configuration, so what it reads from disk belongs to it
// rather than to the location the operating system would have chosen.
func (c Config) Dir() string {
	if c.ConfigDir != "" {
		return c.ConfigDir
	}
	if c.dir != "" {
		return c.dir
	}
	path, err := Path()
	if err != nil {
		return ""
	}
	return filepath.Dir(path)
}

// AppDir is one command's corner of it. Commands are given a corner each
// because they are unrelated: two of them both wanting "templates" is the
// normal case, not a collision to be worked around.
func (c Config) AppDir(app string) string {
	dir := c.Dir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, app)
}

// CaptureTemplatesDir is where Capture reads every template that is not
// compiled in, CaptureWorkflowsDir where it reads what a workflow keeps where,
// and CaptureRecipesDir where it reads the Recipes.
//
// None of the three is configurable. The layout is the answer to "where does
// this installation keep its Capture configuration?", and a path for each
// would make that answer three paths to go and look up. config_dir moves the
// whole thing, and a directory that genuinely belongs elsewhere — a vault's
// templates kept with the vault — is a symlink.
func (c Config) CaptureTemplatesDir() string { return c.captureDir("templates") }

func (c Config) CaptureWorkflowsDir() string { return c.captureDir("workflows") }

func (c Config) CaptureRecipesDir() string { return c.captureDir("recipes") }

// CaptureMappingsDir is where Capture reads the mapping tables: one file per
// table, named after it.
func (c Config) CaptureMappingsDir() string { return c.captureDir("mappings") }

func (c Config) captureDir(name string) string {
	app := c.AppDir("capture")
	if app == "" {
		return ""
	}
	return filepath.Join(app, name)
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

// GeoGPXAddr is the host:port the GPX web server listens on.
func (c Config) GeoGPXAddr() string {
	host, port := c.Geo.GPX.Host, c.Geo.GPX.Port
	if host == "" {
		host = DefaultGeoGPXHost
	}
	if port == 0 {
		port = DefaultGeoGPXPort
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
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

// EnvXDGConfigHome is the XDG base directory the configuration is looked for
// in.
const EnvXDGConfigHome = "XDG_CONFIG_HOME"

// XDGDirName is the toolbox's own folder inside the XDG configuration
// directory. The configuration file sits in it rather than beside it, so the
// whole configuration — the file, the recipes, the workflows, the mappings, the
// templates — is one folder a reader can move, copy or keep under version
// control as a unit.
const XDGDirName = "dgs-toolbox"

// Path is the configuration file this run reads: the environment variable when
// it is set, and otherwise the one default location. One rather than a list
// tried in turn, because "which file am I editing?" should not be a question
// with an answer that depends on which files exist.
func Path() (string, error) {
	if path := os.Getenv(EnvPath); path != "" {
		return path, nil
	}
	return DefaultPath()
}

// DefaultPath is <XDG config home>/dgs-toolbox/dgs-config.json, falling back to
// ~/.config when the variable is unset. The XDG directory rather than the
// operating system's own: it is the folder the reader already keeps their other
// tools' files in, and on macOS the operating system's answer is a Library path
// nobody edits by choice. The filename is the one --export-config writes, so a
// file copied out of it is found under the name it already has.
func DefaultPath() (string, error) {
	directory := os.Getenv(EnvXDGConfigHome)
	if directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		directory = filepath.Join(home, ".config")
	}
	return filepath.Join(directory, XDGDirName, ExportFilename), nil
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
