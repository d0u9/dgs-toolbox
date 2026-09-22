package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Box configures dgs box: the Box scans are archived into, the inbox they are
// taken from, the discardable local cache, and the defaults an entry form
// assumes. See docs/configuration/box.md.
type Box struct {
	// Root is the Box. A leading ~ is the home directory. Empty means the
	// command asks for a folder rather than writing into a guessed one.
	Root string `json:"root"`
	// Inbox is the folder dgs box import reads new scans from. A leading ~ is
	// the home directory.
	Inbox string `json:"inbox"`
	// Marker is the name of the file at Root that proves it is a Box. A bare
	// filename, not a path.
	Marker string `json:"marker"`
	// StateFile is the name of Import's resumable state file inside the inbox.
	// A bare filename, not a path.
	StateFile string `json:"state_file"`
	// CacheDir is where the discardable index and the thumbnails are kept,
	// deliberately outside the Box. Empty uses the user cache directory.
	CacheDir string `json:"cache_dir"`
	// IndexFile is the name of the index inside the cache directory. A bare
	// filename, not a path.
	IndexFile string `json:"index_file"`
	// Workers is how many whole files Import copies and verifies at once.
	Workers int `json:"workers"`
	// Currency is the ISO 4217 code an amount typed with no currency is taken
	// to be in. Empty means an amount must always name its currency.
	Currency string `json:"currency"`
	// Timezone is the IANA zone name a newly entered event date is assumed to
	// be in. An offset is refused. Empty uses the machine's zone.
	Timezone string `json:"timezone"`

	Web       BoxWeb       `json:"web"`
	Thumbnail BoxThumbnail `json:"thumbnail"`
	Preview   BoxPreview   `json:"preview"`
	Trash     BoxTrash     `json:"trash"`
	// Lifetimes overrides the default lifetime of a type, in days from the
	// event date. 0 makes a type permanent.
	Lifetimes map[string]int `json:"lifetimes"`
}

// BoxWeb is where the box pages listen. The address is always a loopback one
// and is not configurable.
type BoxWeb struct {
	// Port is the port the box pages listen on. 0 uses the default.
	Port int `json:"port"`
}

// BoxThumbnail is the grid thumbnail's size.
type BoxThumbnail struct {
	// Size is the long edge, in pixels.
	Size int `json:"size"`
}

// BoxPreview is the preview shown when one scan is open.
type BoxPreview struct {
	// Size is the long edge, in pixels.
	Size int `json:"size"`
	// Keep is how many days an unused preview is kept. 0 keeps them
	// indefinitely, so it is a pointer: unset takes the default, 0 is a
	// decision.
	Keep *int `json:"keep"`
}

// BoxTrash is where discarded scans are moved to, inside the Box.
type BoxTrash struct {
	// Dir is the directory inside box.root, a bare name rather than a path, so
	// the move stays a same-volume rename.
	Dir string `json:"dir"`
	// Keep is how many days the trash is worth keeping, shown as advice. box
	// never removes a file. 0 shows no advice, so it is a pointer: unset takes
	// the default, 0 is a decision.
	Keep *int `json:"keep"`
}

const (
	DefaultBoxMarker      = ".dgs-box"
	DefaultBoxStateFile   = ".dgs-box-state"
	DefaultBoxIndexFile   = "index.json"
	DefaultBoxWorkers     = 4
	DefaultBoxWebPort     = 8766
	DefaultBoxThumbnail   = 300
	DefaultBoxPreviewSize = 1600
	DefaultBoxPreviewKeep = 30
	DefaultBoxTrashDir    = "trash"
	DefaultBoxTrashKeep   = 90
)

func intPointer(value int) *int { return &value }

func defaultBox() Box {
	return Box{
		Marker:    DefaultBoxMarker,
		StateFile: DefaultBoxStateFile,
		IndexFile: DefaultBoxIndexFile,
		Workers:   DefaultBoxWorkers,
		Web:       BoxWeb{Port: DefaultBoxWebPort},
		Thumbnail: BoxThumbnail{Size: DefaultBoxThumbnail},
		Preview:   BoxPreview{Size: DefaultBoxPreviewSize, Keep: intPointer(DefaultBoxPreviewKeep)},
		Trash:     BoxTrash{Dir: DefaultBoxTrashDir, Keep: intPointer(DefaultBoxTrashKeep)},
	}
}

// BoxSettings is the Box configuration with every unset key filled in with its
// documented default, so a caller reads one value rather than repeating the
// defaults at each use.
func (c Config) BoxSettings() Box {
	box := c.Box
	filled := defaultBox()
	if box.Marker == "" {
		box.Marker = filled.Marker
	}
	if box.StateFile == "" {
		box.StateFile = filled.StateFile
	}
	if box.IndexFile == "" {
		box.IndexFile = filled.IndexFile
	}
	if box.Workers == 0 {
		box.Workers = filled.Workers
	}
	if box.Web.Port == 0 {
		box.Web.Port = filled.Web.Port
	}
	if box.Thumbnail.Size == 0 {
		box.Thumbnail.Size = filled.Thumbnail.Size
	}
	if box.Preview.Size == 0 {
		box.Preview.Size = filled.Preview.Size
	}
	if box.Preview.Keep == nil {
		box.Preview.Keep = filled.Preview.Keep
	}
	if box.Trash.Dir == "" {
		box.Trash.Dir = filled.Trash.Dir
	}
	if box.Trash.Keep == nil {
		box.Trash.Keep = filled.Trash.Keep
	}
	return box
}

// BoxLifetime is the configured lifetime of a type, in days from the event
// date, and whether the configuration names it at all. A type it does not name
// keeps the catalogue's own default, which the catalogue owns.
func (c Config) BoxLifetime(name string) (int, bool) {
	days, ok := c.Box.Lifetimes[name]
	return days, ok
}

// BoxLocation is the zone a newly entered event date is assumed to be in:
// box.timezone, or the machine's zone when it is empty.
func (c Config) BoxLocation() (*time.Location, error) {
	if c.Box.Timezone == "" {
		return time.Local, nil
	}
	return time.LoadLocation(c.Box.Timezone)
}

// validateBox refuses a Box configuration that cannot mean what it says: a
// path where a bare name belongs, a negative count, an offset where an IANA
// zone belongs. Paths are expanded here too, so every caller reads a real one.
func validateBox(box *Box, home string) error {
	for _, named := range []struct {
		key   string
		value *string
	}{
		{"box.root", &box.Root},
		{"box.inbox", &box.Inbox},
		{"box.cache_dir", &box.CacheDir},
	} {
		if *named.value == "" {
			continue
		}
		expanded, err := ExpandPath(*named.value, os.LookupEnv, home)
		if err != nil {
			return fmt.Errorf("%s: %w", named.key, err)
		}
		*named.value = expanded
	}
	for _, named := range []struct {
		key   string
		value string
	}{
		{"box.marker", box.Marker},
		{"box.state_file", box.StateFile},
		{"box.index_file", box.IndexFile},
		{"box.trash.dir", box.Trash.Dir},
	} {
		if named.value == "" {
			continue
		}
		if filepath.Base(named.value) != named.value || named.value == "." || named.value == ".." {
			return fmt.Errorf("%s must be a filename, got %q", named.key, named.value)
		}
	}
	for _, named := range []struct {
		key   string
		value int
	}{
		{"box.workers", box.Workers},
		{"box.web.port", box.Web.Port},
		{"box.thumbnail.size", box.Thumbnail.Size},
		{"box.preview.size", box.Preview.Size},
	} {
		if named.value < 0 {
			return fmt.Errorf("%s must not be negative, got %d", named.key, named.value)
		}
	}
	for _, named := range []struct {
		key   string
		value *int
	}{
		{"box.preview.keep", box.Preview.Keep},
		{"box.trash.keep", box.Trash.Keep},
	} {
		if named.value != nil && *named.value < 0 {
			return fmt.Errorf("%s must not be negative, got %d", named.key, *named.value)
		}
	}
	if box.Web.Port > 65535 {
		return fmt.Errorf("box.web.port must be a port, got %d", box.Web.Port)
	}
	if box.Currency != "" {
		if len(box.Currency) != 3 || strings.ToUpper(box.Currency) != box.Currency {
			return fmt.Errorf("box.currency must be an ISO 4217 code, got %q", box.Currency)
		}
	}
	if box.Timezone != "" {
		if strings.ContainsAny(box.Timezone, "+-") || box.Timezone == "Local" {
			return fmt.Errorf("box.timezone must be an IANA zone name, got %q", box.Timezone)
		}
		if _, err := time.LoadLocation(box.Timezone); err != nil {
			return fmt.Errorf("box.timezone: %w", err)
		}
	}
	for name, days := range box.Lifetimes {
		if name == "" {
			return fmt.Errorf("box.lifetimes names an empty type")
		}
		if days < 0 {
			return fmt.Errorf("box.lifetimes[%q] must not be negative, got %d", name, days)
		}
	}
	return nil
}
