package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
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
// every image already written. The listening address is fixed to a loopback
// one, because a Box holds identity and medical documents.
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
	// CacheDir is where the discardable index and the thumbnails live. It is
	// outside the Box on purpose: a cache belongs to one machine, and a
	// database file on a network filesystem is a way to lose data. Empty means
	// the user cache directory.
	CacheDir string `json:"cache_dir"`
	// Workers is how many whole files Import copies and verifies at once. Zero
	// means DefaultBoxWorkers.
	Workers int        `json:"workers"`
	Web     BoxWeb     `json:"web"`
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

// BoxWeb is where the box pages listen. Only the port is configurable.
type BoxWeb struct {
	// Port is the port to listen on. Zero means DefaultBoxWebPort.
	Port int `json:"port"`
}

// BoxPreview is how long the large previews are worth keeping. Grid thumbnails
// are a few KB and are never dropped, so they have no setting.
type BoxPreview struct {
	// Keep is how many days an unused preview is kept. Zero keeps previews
	// indefinitely, so nil rather than zero means the default.
	Keep *int `json:"keep"`
}

// BoxTrash is how long the trash is worth keeping. box never removes a file:
// emptying trash/ is done by hand, and this only decides what view advises.
type BoxTrash struct {
	// Keep is how many days the trash is worth keeping. Zero shows no advice,
	// so nil rather than zero means the default.
	Keep *int `json:"keep"`
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

// BoxRoot and BoxInbox are box.root and box.inbox, already expanded by
// LoadPath. Both are empty until configured.
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
func (c Config) BoxWebAddr() string {
	port := c.Box.Web.Port
	if port == 0 {
		port = DefaultBoxWebPort
	}
	return net.JoinHostPort(DefaultBoxWebHost, strconv.Itoa(port))
}

// BoxPreviewKeepDays is how many days an unused preview is kept; zero keeps
// them indefinitely.
func (c Config) BoxPreviewKeepDays() int {
	if c.Box.Preview.Keep == nil {
		return DefaultBoxPreviewKeepDays
	}
	return *c.Box.Preview.Keep
}

// BoxTrashKeepDays is how many days the trash is worth keeping; zero means
// view offers no advice.
func (c Config) BoxTrashKeepDays() int {
	if c.Box.Trash.Keep == nil {
		return DefaultBoxTrashKeepDays
	}
	return *c.Box.Trash.Keep
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
// An empty box.cache_dir uses the operating system's user cache directory.
func (c Config) BoxCacheDir(root string) (string, error) {
	base := c.Box.CacheDir
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

// validateBox checks what has to be refused when the file is read rather than
// when the first scan is filed: a mistyped currency, an offset written where a
// zone belongs, a path written where a filename belongs.
func validateBox(settings Box, path string) error {
	for key, name := range map[string]string{
		"box.marker":     firstNonEmpty(settings.Marker, DefaultBoxMarker),
		"box.state_file": firstNonEmpty(settings.StateFile, DefaultBoxStateFile),
	} {
		if filepath.Base(name) != name || name == "." || name == ".." {
			return fmt.Errorf("decode config %s: %s must be a filename, got %q", path, key, name)
		}
	}
	if code := strings.ToUpper(strings.TrimSpace(settings.Currency)); code != "" && !money.Known(code) {
		return fmt.Errorf("decode config %s: box.currency %q is not a currency this build knows the decimals of", path, code)
	}
	if settings.Workers < 0 {
		return fmt.Errorf("decode config %s: box.workers cannot be negative, got %d", path, settings.Workers)
	}
	if settings.Preview.Keep != nil && *settings.Preview.Keep < 0 {
		return fmt.Errorf("decode config %s: box.preview.keep is a number of days and cannot be negative, got %d", path, *settings.Preview.Keep)
	}
	if settings.Trash.Keep != nil && *settings.Trash.Keep < 0 {
		return fmt.Errorf("decode config %s: box.trash.keep is a number of days and cannot be negative, got %d", path, *settings.Trash.Keep)
	}
	return nil
}

// validateBoxZone is separate because it loads the zone database, which the
// rest of the checks do not need.
func validateBoxZone(configured Box, path string) error {
	if configured.Timezone == "" {
		return nil
	}
	if _, err := box.LoadZone(configured.Timezone); err != nil {
		return fmt.Errorf("decode config %s: box.timezone: %w", path, err)
	}
	return nil
}

func firstNonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
