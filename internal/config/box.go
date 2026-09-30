package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/money"
)

// Box configures dgs box: the Box scans are archived into, the inbox they are
// taken from, the discardable local cache, and the few numbers that are not
// facts about a document. See docs/configuration/box.md.
//
// What is deliberately not here is as much a decision as what is. The type
// catalogue and its lifetimes are registered in code, because a type name goes
// into a sidecar and has to mean the same thing on another machine. The
// thumbnail sizes are fixed, because a size that could change would invalidate
// every image already written. The listening host is fixed to a loopback one,
// because a Box holds identity and medical documents.
type Box struct {
	// Root is the Box. Nothing is written unless Marker is found there, so an
	// unmounted NAS is refused rather than turned into a second, local Box.
	// Empty means the command asks for a folder.
	Root string `json:"root"`
	// Inbox is the folder Import reads new scans from. Empty means Import opens
	// with no inbox.
	Inbox string `json:"inbox"`
	// Marker is the name of the file at Root that proves it is a Box. A bare
	// filename, not a path.
	Marker string `json:"marker"`
	// StateFile is where Import keeps its resumable state, inside the inbox. A
	// bare filename, not a path.
	StateFile string `json:"state_file"`
	// Cache is where the discardable index and the thumbnails live.
	Cache Cache `json:"cache"`
	// Workers is how many whole files Import copies and verifies at once. Zero
	// means DefaultBoxWorkers.
	Workers int `json:"workers"`
	// Web lists the servers by use; the only one is "browse", the box pages.
	Web     []Web      `json:"web"`
	Preview BoxPreview `json:"preview"`
	Trash   BoxTrash   `json:"trash"`
	// Currency is the ISO 4217 code an amount typed with no currency is taken
	// to be in. Empty means every amount has to name its own.
	Currency string `json:"currency"`
	// Timezone is the IANA zone name a newly entered event date is assumed to
	// be in. An offset such as +10:00 is refused: it is what a zone happened to
	// be at one moment, so storing it loses daylight saving. Empty means the
	// machine's zone.
	Timezone string `json:"timezone"`
}

// Cache is where a command keeps its discardable local cache. It is outside
// what the cache describes on purpose: a cache belongs to one machine, and a
// database file on a network filesystem is a way to lose data.
type Cache struct {
	// Dir is the cache directory. Empty means <user cache dir>/dgs/<command>.
	Dir string `json:"dir"`
}

// BoxPreview is how long the large previews are worth keeping. Grid thumbnails
// are a few KB and are never dropped, so they have no setting.
type BoxPreview struct {
	// KeepDays is how many days an unused preview is kept. Zero keeps
	// previews indefinitely, so nil rather than zero means the default.
	KeepDays *int `json:"keep_days"`
}

// BoxTrash is how long the trash is worth keeping. box never removes a file:
// emptying trash/ is done by hand, and this only decides what view advises.
type BoxTrash struct {
	// KeepDays is how many days the trash is worth keeping. Zero shows no
	// advice, so nil rather than zero means the default.
	KeepDays *int `json:"keep_days"`
}

// The defaults for the numbers a Box does not get from a document.
const (
	// DefaultBoxMarker is visible on purpose. Finder hides dotfiles, and a Box
	// whose metadata is invisible looks like a folder of PDFs with nothing
	// beside it — the opposite of a sidecar meant to outlive the tool.
	DefaultBoxMarker = "dgs-box.yaml"
	// DefaultBoxStateFile is Import's resumable state, in the inbox.
	DefaultBoxStateFile = "dgs-box-state.json"
	// DefaultBoxWorkers is a handful, which is faster than one over a network
	// filesystem and faster than many.
	DefaultBoxWorkers = 4
	// BoxBrowseServer is the web entry of the box pages.
	BoxBrowseServer = "browse"
	// DefaultBoxWebHost is not configurable. See Box.
	DefaultBoxWebHost = "127.0.0.1"
	// DefaultBoxWebPort sits next to the GPX server's.
	DefaultBoxWebPort = 8766
	// DefaultBoxPreviewKeepDays is how long an unused 1600 px preview is kept.
	DefaultBoxPreviewKeepDays = 30
	// DefaultBoxTrashKeepDays is long because a NAS without snapshots has no
	// other undo.
	DefaultBoxTrashKeepDays = 90
)

var boxServers = []server{{name: BoxBrowseServer, host: DefaultBoxWebHost, port: DefaultBoxWebPort}}

// BoxRoot and BoxInbox are box.root and box.inbox, already expanded by
// LoadDir. Both are empty until configured.
func (c Config) BoxRoot() string  { return c.Box.Root }
func (c Config) BoxInbox() string { return c.Box.Inbox }

// BoxMarker is the filename that proves a folder is a Box.
func (c Config) BoxMarker() string {
	if c.Box.Marker == "" {
		return DefaultBoxMarker
	}
	return c.Box.Marker
}

// BoxStateFile is the name Import keeps its resumable state under.
func (c Config) BoxStateFile() string {
	if c.Box.StateFile == "" {
		return DefaultBoxStateFile
	}
	return c.Box.StateFile
}

// BoxWorkers is how many files Import works on at once.
func (c Config) BoxWorkers() int {
	if c.Box.Workers <= 0 {
		return DefaultBoxWorkers
	}
	return c.Box.Workers
}

// BoxWebAddr is the host:port the box pages listen on. The host is always a
// loopback address.
func (c Config) BoxWebAddr() string { return webAddr(c.Box.Web, boxServers, BoxBrowseServer) }

// BoxPreviewKeepDays is how many days an unused preview is kept; zero keeps
// them indefinitely.
func (c Config) BoxPreviewKeepDays() int {
	if c.Box.Preview.KeepDays == nil {
		return DefaultBoxPreviewKeepDays
	}
	return *c.Box.Preview.KeepDays
}

// BoxTrashKeepDays is how many days the trash is worth keeping; zero means
// view offers no advice.
func (c Config) BoxTrashKeepDays() int {
	if c.Box.Trash.KeepDays == nil {
		return DefaultBoxTrashKeepDays
	}
	return *c.Box.Trash.KeepDays
}

// BoxCurrency is the currency an amount with no code is read in, upper-cased.
// Empty means an amount must name its own, which is what money.Parse does with
// an empty default.
func (c Config) BoxCurrency() string {
	return strings.ToUpper(strings.TrimSpace(c.Box.Currency))
}

// BoxZone is the zone a newly entered event date is assumed to be in: the
// configured IANA zone, or the machine's own.
func (c Config) BoxZone() (*time.Location, error) {
	return box.LoadZone(c.Box.Timezone)
}

// BoxCacheDir is where the discardable index and thumbnails for one Box live:
// <cache dir>/<a digest of the root path>. The root is part of the path rather
// than of the contents so that two Boxes, and two machines, never share a
// cache; and the cache is outside the Box so that nothing on a network
// filesystem is ever written to as a database.
//
// An empty cache.dir uses the operating system's user cache directory.
func (c Config) BoxCacheDir(root string) (string, error) {
	base := c.Box.Cache.Dir
	if base == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("locate the user cache directory: %w", err)
		}
		base = filepath.Join(userCache, "dgs", "box")
	}
	if root == "" {
		return base, nil
	}
	return filepath.Join(base, box.CacheKey(root)), nil
}

// checkBox checks what has to be refused when the file is read rather than
// when the first scan is filed: a mistyped currency, an offset written where a
// zone belongs, a path written where a filename belongs. It expands the paths.
func checkBox(c *Config) error {
	settings := c.Box
	for key, name := range map[string]string{
		"marker":     firstNonEmpty(settings.Marker, DefaultBoxMarker),
		"state_file": firstNonEmpty(settings.StateFile, DefaultBoxStateFile),
	} {
		if filepath.Base(name) != name || name == "." || name == ".." {
			return fmt.Errorf("%s must be a filename, got %q", key, name)
		}
	}
	if code := strings.ToUpper(strings.TrimSpace(settings.Currency)); code != "" && !money.Known(code) {
		return fmt.Errorf("currency %q is not a currency this build knows the decimals of", code)
	}
	if settings.Workers < 0 {
		return fmt.Errorf("workers cannot be negative, got %d", settings.Workers)
	}
	if settings.Preview.KeepDays != nil && *settings.Preview.KeepDays < 0 {
		return fmt.Errorf("preview.keep_days cannot be negative, got %d", *settings.Preview.KeepDays)
	}
	if settings.Trash.KeepDays != nil && *settings.Trash.KeepDays < 0 {
		return fmt.Errorf("trash.keep_days cannot be negative, got %d", *settings.Trash.KeepDays)
	}
	if err := checkWeb(settings.Web, boxServers); err != nil {
		return err
	}
	if settings.Timezone != "" {
		// The zone database is loaded only when a zone is named.
		if _, err := box.LoadZone(settings.Timezone); err != nil {
			return fmt.Errorf("timezone: %w", err)
		}
	}
	return expandAll(map[string]*string{
		"root": &c.Box.Root, "inbox": &c.Box.Inbox, "cache.dir": &c.Box.Cache.Dir,
	})
}

func firstNonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
