package organizer

import (
	"path/filepath"
	"strings"
)

// Settings are the paths the Actions write to. They are declared here rather
// than read from the configuration package, so the organizer stays free of the
// application's configuration shape and a test can name a directory without
// building a config file.
//
// Nothing here is discovered. An Obsidian vault does keep its own daily note
// settings, and reading them looked like saving the reader a duplicate — but a
// vault says where the plugin in use puts notes, not where this tool should
// write them, and the two are not the same question. Guessing wrongly writes a
// Capture into a file nobody was looking at, so what is not configured is an
// error rather than a guess.
type Settings struct {
	// ObsidianVault is an absolute path. Empty means no vault has been
	// configured, and the Obsidian Actions refuse to run.
	ObsidianVault string
	// DailyNote is where a day's note lives, relative to the vault, as a
	// template over the date: "00 Daily Log/{{.Year}}/{{.Date}}.md".
	DailyNote string
	// LocationsFolder is relative to the vault.
	LocationsFolder string
	// DailySection is the heading a Capture is appended under. Its own section
	// rather than the end of the note, so what this tool writes stays
	// distinguishable from what the reader wrote.
	DailySection string
	// TemplateDir holds every template that is not compiled in, by filename:
	// the daily note's, and any that replaces a compiled-in default. One place
	// to look, rather than a path in the configuration for each.
	TemplateDir string
}

// DefaultSettings are the values that have a sensible default. The paths do
// not: where a reader keeps their notes is not something to assume.
func DefaultSettings() Settings {
	return Settings{LocationsFolder: "Locations", DailySection: DefaultDailySection}
}

// DefaultDailySection is the heading this tool writes under.
const DefaultDailySection = "DGS"

func (s Settings) section() string {
	if strings.TrimSpace(s.DailySection) == "" {
		return DefaultDailySection
	}
	return strings.TrimSpace(s.DailySection)
}

func (s Settings) locations() string {
	if s.LocationsFolder == "" {
		return "Locations"
	}
	return s.LocationsFolder
}

// vaultPath resolves a vault-relative target to where it will actually be
// written. It reports false when no vault is configured.
func (s Settings) vaultPath(target string) (string, bool) {
	if s.ObsidianVault == "" || target == "" {
		return "", false
	}
	return filepath.Join(s.ObsidianVault, target), true
}
