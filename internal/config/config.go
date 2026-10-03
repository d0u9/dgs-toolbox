// Package config loads process-wide dgs configuration.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// EnvPath names the configuration directory, as --config does.
const EnvPath = "DGS_TOOLBOX_CONFIG"

// PartFile is the name of each part's file: <config dir>/<part>/config.json.
// Every command has a folder of its own holding its file and whatever else it
// reads from disk, so a command could leave the toolbox taking its folder
// with it, and a mistake in one file refuses only the command it belongs to.
const PartFile = "config.json"

type Config struct {
	Shell   Shell
	Photo   Photo
	Box     Box
	Doc     Doc
	Capture Capture
	Geo     Geo
	Conf    Conf
	Plugins Plugins
	// dir is the configuration directory. Everything a command reads from
	// disk resolves against it, so --config points at a whole configuration.
	dir string
	// errs holds each part whose file was refused, by part name.
	errs map[string]error
}

// Capture configures the Capture command. Route organizes a Capture through
// the Recipes and Actions of the organizer model rather than through configured
// destinations, so it has no settings of its own.
type Capture struct {
	Scan     CaptureScan     `json:"scan"`
	Archive  CaptureArchive  `json:"archive"`
	Obsidian CaptureObsidian `json:"obsidian"`
	Apple    CaptureApple    `json:"apple"`
	GPX      CaptureGPX      `json:"gpx"`
}

// CaptureGPX names the directory for daily Capture waypoint files.
type CaptureGPX struct {
	Root string `json:"root"`
}

// CaptureApple configures the Actions that write into Apple's apps.
type CaptureApple struct {
	Reminders CaptureReminders `json:"reminders"`
}

// CaptureReminders is how the reminder Actions write when a run does not say
// otherwise. List is a Reminders list's title; empty is Reminders' own default
// list. Radius is how close counts as arriving for a reminder at a place, in
// metres; zero is the organizer's default.
type CaptureReminders struct {
	List   string  `json:"list"`
	Radius float64 `json:"radius"`
}

// CaptureObsidian tells the Obsidian Actions where to write. Vault is an
// absolute path; the notes and folders are relative to it, so what an Action
// plans and records stays vault-relative and survives the vault moving. Each
// note an Action writes has an object of its own.
type CaptureObsidian struct {
	Vault string               `json:"vault"`
	Daily CaptureObsidianDaily `json:"daily"`
	// Timelines is the file defining every timeline, relative to the vault.
	// It is in the vault because the Obsidian plugin reads the same file.
	Timelines string `json:"timelines"`
}

// CaptureObsidianDaily is the day's note. Note is where it lives, relative to
// the vault, written as a template over the date: "00 Daily Log/{{.Year}}/
// {{.Date}}.md". It is configured rather than read from the vault, because a
// vault says where the plugin in use puts notes, which is not the same
// question. Section is the heading a Capture is written under, and Images how
// its pictures are written beside the note.
type CaptureObsidianDaily struct {
	Note    string                `json:"note"`
	Section string                `json:"section"`
	Images  CaptureObsidianImages `json:"images"`
}

// CaptureObsidianImages says where a daily note's pictures go and how they are
// re-encoded. Folder is relative to the note's folder, as a template over the
// note; MaxSide is the longest side in pixels and Quality the JPEG quality,
// 1 to 100. Empty or zero is the organizer's default.
type CaptureObsidianImages struct {
	Folder  string `json:"folder"`
	MaxSide int    `json:"max_side"`
	Quality int    `json:"quality"`
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

// Geo configures the Geo app. Web lists its servers by use; the only one is
// "gpx", the GPX page.
type Geo struct {
	Web []Web  `json:"web"`
	GPX GeoGPX `json:"gpx"`
}

// GeoGPX configures the GPX command: the folder the browser opens at, and the
// base maps the page offers. Empty values use the defaults.
type GeoGPX struct {
	Root  string       `json:"root"`
	Tiles []GeoGPXTile `json:"tiles"`
	// AmapKey is an Amap (高德) Web Service key: with it the page routes
	// along Amap's roads as well as OpenStreetMap's.
	AmapKey string `json:"amap_key"`
	// DEM is the elevation tiles the page draws contour lines from.
	DEM GeoGPXDEM `json:"dem"`
}

// GeoGPXDEM is a raster elevation tile source: URL is a template holding {z},
// {x} and {y}, Encoding is "terrarium" or "mapbox" (terrain-RGB). Empty values
// use AWS's public Terrarium tiles, which need no key.
type GeoGPXDEM struct {
	URL      string `json:"url"`
	Encoding string `json:"encoding"`
	MaxZoom  int    `json:"max_zoom"`
}

// Default DEM tiles: Mapzen's Terrarium tiles on AWS Open Data.
const (
	DefaultGeoGPXDEMURL      = "https://s3.amazonaws.com/elevation-tiles-prod/terrarium/{z}/{x}/{y}.png"
	DefaultGeoGPXDEMEncoding = "terrarium"
	DefaultGeoGPXDEMMaxZoom  = 15
)

// GeoGPXDEMSource is geo.gpx.dem with its defaults filled in. A custom URL
// without an encoding is taken as Terrarium, as the default is.
func (c Config) GeoGPXDEMSource() GeoGPXDEM {
	dem := c.Geo.GPX.DEM
	if dem.URL == "" {
		dem.URL = DefaultGeoGPXDEMURL
	}
	if dem.Encoding == "" {
		dem.Encoding = DefaultGeoGPXDEMEncoding
	}
	if dem.MaxZoom == 0 {
		dem.MaxZoom = DefaultGeoGPXDEMMaxZoom
	}
	return dem
}

// GeoGPXTile is a base map the GPX page offers beside the built-in
// built-in maps. URL is a raster tile template holding {z}, {x} and {y}.
// Coordinates names the system the tiles are drawn in: "wgs84" (or empty) or
// "gcj02", as maps published in China are.
type GeoGPXTile struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Attribution string `json:"attribution"`
	MaxZoom     int    `json:"max_zoom"`
	Coordinates string `json:"coordinates"`
}

// GeoGPXTilesFile is the file, in the geo folder of the config directory,
// that lists base maps beside those in gpx.tiles, so a long list does not
// crowd the configuration file:
//
//	{"tiles": [{"name": "…", "url": "https://…/{z}/{x}/{y}.png", "coordinates": "gcj02"}]}
const GeoGPXTilesFile = "tiles.json"

// GeoGPXServer is the web entry of the GPX page. Host 0.0.0.0 lets other
// machines reach it; the default keeps the page local and at a stable URL.
const GeoGPXServer = "gpx"

const (
	DefaultGeoGPXHost = "127.0.0.1"
	DefaultGeoGPXPort = 8765
)

var geoServers = []server{{name: GeoGPXServer, host: DefaultGeoGPXHost, port: DefaultGeoGPXPort, hostSettable: true}}

// Plugins configures dgs plugins: where the plugins dgs carries are installed.
type Plugins struct {
	Obsidian PluginsObsidian `json:"obsidian"`
}

// PluginsObsidian lists the vaults the Obsidian plugins are checked and
// installed in when a run names none. Each is a vault's own folder, the one
// holding its configuration folders; a leading ~ is the home directory.
// Folders and Timelines are paths inside every vault.
type PluginsObsidian struct {
	Vaults    []string               `json:"vaults"`
	Folders   PluginsObsidianFolders `json:"folders"`
	Timelines string                 `json:"timelines"`
}

// PluginsObsidianFolders are the vault folders the scripts and templates dgs
// carries are installed into, each relative to the vault. dgs owns the files
// it writes there and nothing else.
type PluginsObsidianFolders struct {
	// Public is the scripts every caller shares: Templater's user scripts
	// folder, also loaded by the QuickAdd scripts and the plugin.
	Public string `json:"public"`
	// QuickAdd is the scripts QuickAdd choices run.
	QuickAdd string `json:"quickadd"`
	// Templates is the Templater templates, inside Templater's templates folder.
	Templates string `json:"templates"`
}

// The vault paths dgs plugins installs into when plugins/config.json names
// none.
const (
	DefaultPluginsObsidianPublic    = "99 Toolkit/91 Scripts/01 DGS/00 Public"
	DefaultPluginsObsidianQuickAdd  = "99 Toolkit/91 Scripts/01 DGS/02 QuickAdd"
	DefaultPluginsObsidianTemplates = "99 Toolkit/01 Templates/01 DGS"
	DefaultPluginsObsidianTimelines = "99 Toolkit/timelines.json"
)

// Conf configures dgs conf export: where the generator root and its secrets
// are, and where the destination form opens. See
// rhumb docs/export.md#configuration.
type Conf struct {
	// Root is the directory holding one subdirectory per service. Empty means
	// the page opens with no root and asks for one.
	Root string `json:"root"`
	// Secrets is the directory a manifest's secrets file is named relative
	// to. Empty means a service naming a secrets file refuses to render.
	Secrets string `json:"secrets"`
	// Services is a directory of deploy service definitions, consulted
	// before the ones built into dgs when a bundle is built. Empty means
	// only the built-in ones.
	Services string `json:"services"`
	// InstallRoot is where a Linux host bundle whose instance names no
	// deploy.dir is installed, as <install_root>/<service>. Empty means
	// rhumb's default, /srv/rhumb.
	InstallRoot string `json:"install_root"`
	// LabelPrefix begins the systemd unit and launchd label a bundle
	// registers, <label_prefix>.<node>.<instance>. Empty means rhumb.
	LabelPrefix string `json:"label_prefix"`
	// Tool identifies bundle ownership in ctl, command shims and plist keys.
	// Empty means DefaultConfTool.
	Tool   string     `json:"tool"`
	Export ConfExport `json:"export"`
}

// ConfExport is where dgs conf export's destination form opens.
type ConfExport struct {
	// Dir is where the destination form opens, for a folder or an archive.
	// Empty means the home directory.
	Dir string `json:"dir"`
}

type Photo struct {
	Import PhotoImport `json:"import"`
	Encode PhotoEncode `json:"encode"`
}

type PhotoImport struct {
	StateFile   string `json:"state_file"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// Shell configures what every command shares: the top bar.
type Shell struct {
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

func intPointer(value int) *int { return &value }

func Default() Config {
	return Config{Shell: Shell{TopBar: TopBar{
		Disk: boolPointer(true), Network: boolPointer(true),
		CPU: boolPointer(true), Time: boolPointer(true),
	}}, Photo: Photo{Import: PhotoImport{StateFile: ".dgs-state"}, Encode: DefaultPhotoEncode()}, Box: Box{
		Marker: DefaultBoxMarker, StateFile: DefaultBoxStateFile,
		Workers: DefaultBoxWorkers, Web: defaultWeb(boxServers),
		Preview: BoxPreview{KeepDays: intPointer(DefaultBoxPreviewKeepDays)},
		Trash:   BoxTrash{KeepDays: intPointer(DefaultBoxTrashKeepDays)},
	}, Doc: Doc{Trees: map[string]string{}, Web: defaultWeb(docServers)}, Capture: Capture{
		Scan: CaptureScan{IndexFile: "index.json"},
	}, Geo: Geo{Web: defaultWeb(geoServers), GPX: GeoGPX{Tiles: []GeoGPXTile{}}},
		Plugins: Plugins{Obsidian: PluginsObsidian{
			Vaults: []string{},
			Folders: PluginsObsidianFolders{
				Public:    DefaultPluginsObsidianPublic,
				QuickAdd:  DefaultPluginsObsidianQuickAdd,
				Templates: DefaultPluginsObsidianTemplates,
			},
			Timelines: DefaultPluginsObsidianTimelines,
		}}}
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

// At is the default configuration read from dir: a Config for a directory
// whose files are not to be loaded, as a test building one by hand wants.
func At(dir string) Config {
	config := Default()
	config.dir = dir
	return config
}

// Dir is the configuration directory: the one loaded, or the one this run
// resolves to for a Config built without loading.
func (c Config) Dir() string {
	if c.dir != "" {
		return c.dir
	}
	dir, err := Dir()
	if err != nil {
		return ""
	}
	return dir
}

// AppDir is one command's folder in it, holding its config.json and
// everything else it reads from disk. Commands are given a folder each
// because they are independent: two of them both wanting "templates" is the
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
// would make that answer three paths to go and look up. --config moves the
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

// CaptureReminders returns the reminder settings as configured; the organizer
// applies the defaults, as it does for Obsidian's.
func (c Config) CaptureReminders() CaptureReminders { return c.Capture.Apple.Reminders }

func (c Config) PhotoImportStateFile() string {
	if c.Photo.Import.StateFile == "" {
		return ".dgs-state"
	}
	return c.Photo.Import.StateFile
}

// GeoGPXAddr is the host:port the GPX web server listens on.
func (c Config) GeoGPXAddr() string { return webAddr(c.Geo.Web, geoServers, GeoGPXServer) }

// GeoGPXRoot is the folder the GPX browser opens at: geo.gpx.root, or the home
// directory when it is empty. A leading ~ is the home directory.
// GeoGPXTilesPath is <config dir>/geo/gpx/tiles.json.
func (c Config) GeoGPXTilesPath() string {
	app := c.AppDir("geo")
	if app == "" {
		return ""
	}
	return filepath.Join(app, "gpx", GeoGPXTilesFile)
}

// GeoGPXTiles is every configured base map: geo.gpx.tiles, then the tiles
// file's. A missing tiles file adds none.
func (c Config) GeoGPXTiles() ([]GeoGPXTile, error) {
	tiles := append([]GeoGPXTile(nil), c.Geo.GPX.Tiles...)
	path := c.GeoGPXTilesPath()
	if path == "" {
		return tiles, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return tiles, nil
	}
	if err != nil {
		return tiles, fmt.Errorf("open tiles %s: %w", path, err)
	}
	defer file.Close()
	var listed struct {
		Tiles []GeoGPXTile `json:"tiles"`
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&listed); err != nil {
		return tiles, fmt.Errorf("decode tiles %s: %w", path, err)
	}
	if err := validateTiles(listed.Tiles); err != nil {
		return tiles, fmt.Errorf("decode tiles %s: %w", path, err)
	}
	return append(tiles, listed.Tiles...), nil
}

// validateTiles checks each tile has a name, a URL template holding {z}, {x}
// and {y}, and a known coordinate system.
func validateTiles(tiles []GeoGPXTile) error {
	for index, tile := range tiles {
		if tile.Name == "" {
			return fmt.Errorf("tiles[%d] has no name", index)
		}
		for _, placeholder := range []string{"{z}", "{x}", "{y}"} {
			if !strings.Contains(tile.URL, placeholder) {
				return fmt.Errorf("tiles[%d] (%s) url must contain %s", index, tile.Name, placeholder)
			}
		}
		if tile.Coordinates != "" && tile.Coordinates != "wgs84" && tile.Coordinates != "gcj02" {
			return fmt.Errorf("tiles[%d] (%s) coordinates must be wgs84 or gcj02, got %q", index, tile.Name, tile.Coordinates)
		}
	}
	return nil
}

func (c Config) GeoGPXRoot() string {
	home, _ := os.UserHomeDir()
	root := c.Geo.GPX.Root
	switch {
	case root == "":
		return home
	case root == "~":
		return home
	case strings.HasPrefix(root, "~/"):
		return filepath.Join(home, root[2:])
	}
	return root
}

func (c Config) PhotoImportPaths() (source, destination string) {
	return c.Photo.Import.Source, c.Photo.Import.Destination
}

// ConfRoot, ConfSecrets, ConfServices and ConfInstallRoot are conf.root,
// conf.secrets, conf.services and conf.install_root, already expanded by
// LoadDir; ConfLabelPrefix is conf.label_prefix. These are empty until
// configured.
func (c Config) ConfRoot() string        { return c.Conf.Root }
func (c Config) ConfSecrets() string     { return c.Conf.Secrets }
func (c Config) ConfServices() string    { return c.Conf.Services }
func (c Config) ConfInstallRoot() string { return c.Conf.InstallRoot }
func (c Config) ConfLabelPrefix() string { return c.Conf.LabelPrefix }

// DefaultConfTool is conf.tool when it is empty: the executable that builds
// the bundles.
const DefaultConfTool = "dgs"

// ConfTool is conf.tool, else DefaultConfTool.
func (c Config) ConfTool() string {
	if c.Conf.Tool == "" {
		return DefaultConfTool
	}
	return c.Conf.Tool
}

// ConfExportDir is where the destination form opens: conf.export.dir, or the
// home directory when it is empty.
func (c Config) ConfExportDir() string {
	if c.Conf.Export.Dir != "" {
		return c.Conf.Export.Dir
	}
	home, _ := os.UserHomeDir()
	return home
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
	apply(c.Shell.TopBar.Disk, &visibility.Disk)
	apply(c.Shell.TopBar.Network, &visibility.Network)
	apply(c.Shell.TopBar.CPU, &visibility.CPU)
	apply(c.Shell.TopBar.Time, &visibility.Time)
	return visibility
}

// EnvXDGConfigHome is the XDG base directory the configuration is looked for
// in.
const EnvXDGConfigHome = "XDG_CONFIG_HOME"

// XDGDirName is the toolbox's own folder inside the XDG configuration
// directory: the whole configuration, one folder a reader can move, copy or
// keep under version control as a unit.
const XDGDirName = "dgs-toolbox"

// Dir is the configuration directory this run reads: the environment variable
// when it is set, and otherwise the one default location. One rather than a
// list tried in turn, because "which file am I editing?" should not be a
// question with an answer that depends on which files exist.
func Dir() (string, error) {
	if dir := os.Getenv(EnvPath); dir != "" {
		return dir, nil
	}
	return DefaultDir()
}

// DefaultDir is <XDG config home>/dgs-toolbox, falling back to ~/.config when
// the variable is unset. The XDG directory rather than the operating system's
// own: it is the folder the reader already keeps their other tools' files in,
// and on macOS the operating system's answer is a Library path nobody edits by
// choice.
func DefaultDir() (string, error) {
	directory := os.Getenv(EnvXDGConfigHome)
	if directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		directory = filepath.Join(home, ".config")
	}
	return filepath.Join(directory, XDGDirName), nil
}

// Load reads the configuration directory this run resolves to.
func Load() (Config, error) {
	dir, err := Dir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve config directory: %w", err)
	}
	return LoadDir(dir)
}

// LoadDir reads every part's file under dir. A missing directory or file is
// not an error: every setting has a default. It bypasses DGS_TOOLBOX_CONFIG so
// --config can override the environment.
//
// The error it returns is for what spoils every command: dir being a file, a
// file of the layout this one replaced, or a broken shell part, which the
// shell itself reads. A broken file of one command is kept for PartErr and
// refuses only that command, because the commands are independent and one
// mistake should not take the others down with it.
func LoadDir(dir string) (Config, error) {
	config := Default()
	config.dir = dir
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		return Config{}, fmt.Errorf("config %s is a file: --config and %s name the configuration directory, one folder per command", dir, EnvPath)
	}
	for _, legacy := range []string{"dgs-config.json", "credentials.json"} {
		path := filepath.Join(dir, legacy)
		if _, err := os.Stat(path); err == nil {
			return Config{}, fmt.Errorf("%s is the old single-file configuration and is no longer read: move each part into %s, as docs/configuration/index.md lays out, and remove it", path, filepath.Join(dir, "<command>", PartFile))
		}
	}
	for _, p := range parts {
		if p.name == "cred" {
			// dgs cred reads its file itself, each time it refreshes.
			continue
		}
		if err := loadPart(&config, dir, p); err != nil {
			if p.name == "shell" {
				return Config{}, err
			}
			if config.errs == nil {
				config.errs = map[string]error{}
			}
			config.errs[p.name] = err
		}
	}
	return config, nil
}

// PartErr is why the part command's file was refused, or nil. A command whose
// file was refused does not start.
func (c Config) PartErr(part string) error { return c.errs[part] }

// PartPath is <config dir>/<part>/config.json.
func (c Config) PartPath(part string) string {
	app := c.AppDir(part)
	if app == "" {
		return ""
	}
	return filepath.Join(app, PartFile)
}

func loadPart(config *Config, dir string, p part) error {
	path := filepath.Join(dir, p.name, PartFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open config %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(p.target(config)); err != nil {
		return fmt.Errorf("decode config %s: %w", path, err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("decode config %s: content after the object", path)
	}
	if p.check != nil {
		if err := p.check(config); err != nil {
			return fmt.Errorf("decode config %s: %w", path, err)
		}
	}
	return nil
}

// part is one command's file: where it decodes to, and what is refused when
// the file is read rather than when the setting is first used.
type part struct {
	name   string
	target func(*Config) any
	check  func(*Config) error
}

var parts = []part{
	{"shell", func(c *Config) any { return &c.Shell }, nil},
	{"capture", func(c *Config) any { return &c.Capture }, checkCapture},
	{"photo", func(c *Config) any { return &c.Photo }, checkPhoto},
	{"box", func(c *Config) any { return &c.Box }, checkBox},
	{"doc", func(c *Config) any { return &c.Doc }, checkDoc},
	{"geo", func(c *Config) any { return &c.Geo }, checkGeo},
	{"conf", func(c *Config) any { return &c.Conf }, checkConf},
	{"plugins", func(c *Config) any { return &c.Plugins }, checkPlugins},
	{"cred", nil, nil},
}

func checkPhoto(c *Config) error {
	if err := c.Photo.Encode.validate(); err != nil {
		return err
	}
	if err := expandAll(map[string]*string{"encode.source": &c.Photo.Encode.Source, "encode.destination": &c.Photo.Encode.Destination}); err != nil {
		return err
	}
	stateFile := c.PhotoImportStateFile()
	if filepath.Base(stateFile) != stateFile || stateFile == "." || stateFile == ".." {
		return fmt.Errorf("import.state_file must be a filename, got %q", stateFile)
	}
	return nil
}

func checkCapture(c *Config) error {
	_, indexFile := c.CaptureScanSettings()
	if filepath.Base(indexFile) != indexFile || indexFile == "." || indexFile == ".." {
		return fmt.Errorf("scan.index_file must be a filename, got %q", indexFile)
	}
	return nil
}

func checkGeo(c *Config) error {
	if err := validateTiles(c.Geo.GPX.Tiles); err != nil {
		return fmt.Errorf("gpx.%w", err)
	}
	return checkWeb(c.Geo.Web, geoServers)
}

func checkConf(c *Config) error {
	return expandAll(map[string]*string{
		"root": &c.Conf.Root, "secrets": &c.Conf.Secrets, "services": &c.Conf.Services,
		"install_root": &c.Conf.InstallRoot,
		"export.dir":   &c.Conf.Export.Dir,
	})
}

func checkPlugins(c *Config) error {
	for index := range c.Plugins.Obsidian.Vaults {
		if c.Plugins.Obsidian.Vaults[index] == "" {
			return fmt.Errorf("obsidian.vaults[%d] is empty", index)
		}
		if err := expandAll(map[string]*string{
			fmt.Sprintf("obsidian.vaults[%d]", index): &c.Plugins.Obsidian.Vaults[index],
		}); err != nil {
			return err
		}
	}
	obsidian := c.Plugins.Obsidian
	for key, value := range map[string]string{
		"obsidian.folders.public": obsidian.Folders.Public, "obsidian.folders.quickadd": obsidian.Folders.QuickAdd,
		"obsidian.folders.templates": obsidian.Folders.Templates, "obsidian.timelines": obsidian.Timelines,
	} {
		if err := checkVaultPath(value); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	folders := map[string]string{"public": obsidian.Folders.Public, "quickadd": obsidian.Folders.QuickAdd, "templates": obsidian.Folders.Templates}
	for name, folder := range folders {
		for other, inside := range folders {
			if name != other && (folder == inside || strings.HasPrefix(inside, folder+"/")) {
				return fmt.Errorf("obsidian.folders.%s %q holds obsidian.folders.%s %q; each needs a folder of its own", name, folder, other, inside)
			}
		}
	}
	return nil
}

// checkVaultPath refuses a path that is not a plain path inside a vault:
// written with forward slashes, relative, and not climbing out or into a
// hidden folder, where Obsidian would not see it.
func checkVaultPath(value string) error {
	if value == "" {
		return errors.New("must not be empty")
	}
	if strings.Contains(value, "\\") || path.IsAbs(value) || path.Clean(value) != value {
		return fmt.Errorf("must be a clean path relative to the vault, written with /, got %q", value)
	}
	for _, segment := range strings.Split(value, "/") {
		if strings.HasPrefix(segment, ".") {
			return fmt.Errorf("must not climb out of the vault or into a hidden folder, got %q", value)
		}
	}
	return nil
}

// expandAll expands each non-empty path in place, naming the key that fails.
func expandAll(fields map[string]*string) error {
	home, _ := os.UserHomeDir()
	for key, value := range fields {
		if *value == "" {
			continue
		}
		expanded, err := ExpandPath(*value, os.LookupEnv, home)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		*value = expanded
	}
	return nil
}

// ExportDefault writes every part's complete default file under dir, the
// working directory when it is empty. A file that already exists is left as it
// is and reported as skipped, so exporting into a configuration in use only
// adds what it lacks.
func ExportDefault(dir string) (written, skipped []string, err error) {
	if dir == "" {
		if dir, err = os.Getwd(); err != nil {
			return nil, nil, err
		}
	}
	defaults := Default()
	for _, p := range parts {
		var value any
		if p.target != nil {
			value = p.target(&defaults)
		} else {
			value = DefaultCredentials()
		}
		path := filepath.Join(dir, p.name, PartFile)
		ok, err := writeNew(path, value)
		if err != nil {
			return written, skipped, err
		}
		if ok {
			written = append(written, path)
		} else {
			skipped = append(skipped, path)
		}
	}
	return written, skipped, nil
}

// writeNew writes value as indented JSON to path unless a file is there
// already, which it reports as false rather than as an error.
func writeNew(path string, value any) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode default config: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create config %s: %w", path, err)
	}
	removeIncomplete := true
	defer func() {
		if removeIncomplete {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return false, fmt.Errorf("write config %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return false, fmt.Errorf("sync config %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close config %s: %w", path, err)
	}
	removeIncomplete = false
	return true, nil
}
