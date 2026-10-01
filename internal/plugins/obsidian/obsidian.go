// Package obsidian finds where an Obsidian vault keeps its plugins. A vault
// can have more than one configuration folder — .obsidian on the desktop and
// another, such as .obsidian-mobile, chosen for a phone — and a plugin is
// installed into each, since Obsidian reads only the one it is set to.
package obsidian

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"dgs-toolbox/internal/plugins"
)

// Host is the name the Obsidian plugins are filed under.
const Host = "obsidian"

// marker is the file every configuration folder holds, which tells one apart
// from any other folder whose name starts with a dot.
const marker = "app.json"

// Targets is every configuration folder of the vault, in name order. A folder
// is one when its name starts with a dot and it holds app.json. A vault with
// none is refused: Obsidian has never opened it.
func Targets(vault string) ([]plugins.Target, error) {
	info, err := os.Stat(vault)
	if err != nil {
		return nil, fmt.Errorf("vault %s: %w", vault, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("vault %s is not a folder", vault)
	}
	entries, err := os.ReadDir(vault)
	if err != nil {
		return nil, err
	}
	var targets []plugins.Target
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !strings.HasPrefix(name, ".") {
			continue
		}
		configDir := filepath.Join(vault, name)
		if _, err := os.Stat(filepath.Join(configDir, marker)); err != nil {
			continue
		}
		targets = append(targets, plugins.Target{
			Host:    Host,
			Place:   filepath.Join(filepath.Base(vault), name),
			Dir:     filepath.Join(configDir, "plugins"),
			Enabled: func(id string) (bool, error) { return enabled(configDir, id) },
		})
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("vault %s has no Obsidian configuration folder: open it in Obsidian once first", vault)
	}
	return targets, nil
}

// enabled reads community-plugins.json, the list of plugins Obsidian turns on.
// It is only read: Obsidian rewrites it from memory while it runs, so a change
// made from outside would be lost, and turning a plugin on is the reader's
// decision in any case.
func enabled(configDir, id string) (bool, error) {
	path := filepath.Join(configDir, "community-plugins.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	return slices.Contains(ids, id), nil
}
