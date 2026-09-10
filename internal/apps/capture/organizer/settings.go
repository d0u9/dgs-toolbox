package organizer

import (
	"path"
	"strings"
)

// Settings are the paths the Actions write to. They are declared here rather
// than read from the configuration package, so the organizer stays free of the
// application's configuration shape and a test can name a directory without
// building a config file.
type Settings struct {
	// ObsidianVault is an absolute path. Empty means no vault has been
	// configured, and the Obsidian Actions refuse to run rather than guessing
	// a directory to write into.
	ObsidianVault string
	// DailyFolder and LocationsFolder are relative to the vault, so what an
	// Action plans and records stays vault-relative and survives the vault
	// being moved. DailyFormat is the Moment.js pattern the vault names its
	// daily notes with.
	DailyFolder     string
	DailyFormat     string
	LocationsFolder string
	// DailySection is the heading a Capture is appended under. Its own section
	// rather than the end of the note, so what this tool writes stays
	// distinguishable from what the reader wrote.
	DailySection string
	// TemplateDir holds templates that override the compiled-in ones by
	// filename. Empty means the defaults, so the tool writes sensibly before
	// anything is configured.
	TemplateDir string
}

// DefaultSettings are the folder names used when nothing is configured. They
// carry no vault: there is no sensible default for where a user's notes live.
func DefaultSettings() Settings {
	return Settings{DailyFolder: "Daily", LocationsFolder: "Locations", DailySection: DefaultDailySection}
}

// DefaultDailySection is the heading this tool writes under.
const DefaultDailySection = "DGS"

// FromVault fills the folder and filename format from the vault's own
// configuration, so they are not configured a second time here.
func (s Settings) FromVault() Settings {
	vault := ReadVaultSettings(s.ObsidianVault)
	s.DailyFolder, s.DailyFormat = vault.Folder, vault.Format
	return s
}

func (s Settings) section() string {
	if strings.TrimSpace(s.DailySection) == "" {
		return DefaultDailySection
	}
	return strings.TrimSpace(s.DailySection)
}

func (s Settings) daily() string {
	if s.DailyFolder == "" {
		return "Daily"
	}
	return s.DailyFolder
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
	return path.Join(s.ObsidianVault, target), true
}
