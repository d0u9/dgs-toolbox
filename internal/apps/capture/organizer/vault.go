package organizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// A vault carries its own settings, so the daily note folder and filename
// format are read from it rather than configured a second time here: two
// places to say the same thing is two places to drift apart.
//
// Which settings are authoritative depends on which plugin is creating daily
// notes. The core plugin writes daily-notes.json and can be switched off while
// its file stays behind, so a stale file must not win over the plugin actually
// in use.
const (
	corePluginsFile      = "core-plugins.json"
	communityPluginsFile = "community-plugins.json"
	dailyNotesFile       = "daily-notes.json"
	periodicNotesFile    = "plugins/periodic-notes/data.json"
)

// VaultSettings is what the vault says about its daily notes.
type VaultSettings struct {
	// Folder is relative to the vault.
	Folder string
	// Format is a Moment.js pattern, as Obsidian writes it. Empty means the
	// Obsidian default.
	Format string
	// Source names the file these came from, for a caller that has to explain
	// where a path came from.
	Source string
}

// ReadVaultSettings reads a vault's daily note settings, preferring whichever
// plugin is enabled. An unreadable or absent vault gives the defaults rather
// than an error: the caller finds out it cannot write when it tries.
func ReadVaultSettings(vault string) VaultSettings {
	settings := VaultSettings{Folder: "Daily", Format: ""}
	if vault == "" {
		return settings
	}
	config := filepath.Join(vault, ".obsidian")
	if periodic, ok := readPeriodicNotes(config); ok && periodicEnabled(config) {
		return periodic
	}
	if daily, ok := readDailyNotes(config); ok && corePluginEnabled(config, "daily-notes") {
		return daily
	}
	// Neither plugin is enabled, or neither file is readable. A file that is
	// present is still the best guess at where the notes live.
	if periodic, ok := readPeriodicNotes(config); ok {
		return periodic
	}
	if daily, ok := readDailyNotes(config); ok {
		return daily
	}
	return settings
}

func readPeriodicNotes(config string) (VaultSettings, bool) {
	var document struct {
		Daily struct {
			Format  string `json:"format"`
			Folder  string `json:"folder"`
			Enabled bool   `json:"enabled"`
		} `json:"daily"`
	}
	path := filepath.Join(config, periodicNotesFile)
	if !readJSON(path, &document) || !document.Daily.Enabled {
		return VaultSettings{}, false
	}
	return VaultSettings{Folder: orDefault(document.Daily.Folder, "Daily"), Format: document.Daily.Format, Source: periodicNotesFile}, true
}

func readDailyNotes(config string) (VaultSettings, bool) {
	var document struct {
		Format string `json:"format"`
		Folder string `json:"folder"`
	}
	path := filepath.Join(config, dailyNotesFile)
	if !readJSON(path, &document) {
		return VaultSettings{}, false
	}
	return VaultSettings{Folder: orDefault(document.Folder, "Daily"), Format: document.Format, Source: dailyNotesFile}, true
}

func periodicEnabled(config string) bool {
	var plugins []string
	if !readJSON(filepath.Join(config, communityPluginsFile), &plugins) {
		return false
	}
	for _, plugin := range plugins {
		if plugin == "periodic-notes" {
			return true
		}
	}
	return false
}

func corePluginEnabled(config, name string) bool {
	var plugins map[string]bool
	if readJSON(filepath.Join(config, corePluginsFile), &plugins) {
		return plugins[name]
	}
	// Older vaults list only the enabled plugins.
	var enabled []string
	if readJSON(filepath.Join(config, corePluginsFile), &enabled) {
		for _, plugin := range enabled {
			if plugin == name {
				return true
			}
		}
	}
	return false
}

func readJSON(path string, target any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, target) == nil
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
