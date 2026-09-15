package cred

import (
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/recipients"
)

// snapshot is everything the keys page shows, read from disk at once so a
// reload replaces all of it together.
type snapshot struct {
	path        string
	found       bool
	settingsErr error
	settings    config.Credentials
	folder      recipients.Folder
	folderErr   error
	scan        identities.Scan
	// held are the public keys this machine has an identity for.
	held map[string]bool
}

func loadSnapshot(path string) snapshot {
	snap := snapshot{path: path, held: map[string]bool{}}
	if path == "" {
		resolved, err := config.CredentialsPath()
		if err != nil {
			snap.settingsErr = err
			return snap
		}
		snap.path = resolved
	}
	snap.settings, snap.found, snap.settingsErr = config.LoadCredentials(snap.path)
	if snap.settingsErr != nil {
		return snap
	}
	if snap.settings.Recipients != "" {
		snap.folder, snap.folderErr = recipients.Load(snap.settings.Recipients)
	}
	snap.scan = identities.Discover(snap.settings.Identities, identities.Options{})
	for _, identity := range snap.scan.Identities {
		if identity.Public.Key != "" {
			snap.held[identity.Public.Key] = true
		}
	}
	return snap
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// tilde shortens a path under the home directory for display.
func tilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}
