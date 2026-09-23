package box

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// MarkerName is the default name of the file that proves a directory is a Box.
const MarkerName = "dgs-box.yaml"

// MarkerVersion is the Box format this build creates.
const MarkerVersion = 1

// Marker is the content of that file.
//
// It holds the format version and when the Box was made rather than being
// empty. An empty marker can only answer whether this is a Box; one with
// contents can also answer which version made it, which is what a later format
// change needs to know.
type Marker struct {
	Version   int    `yaml:"version"`
	CreatedAt string `yaml:"created_at"`
}

// ErrNotABox is returned when the marker is not there.
//
// Every command refuses to write anything without it, and none of them creates
// it as a side effect — dgs box init is the only thing that writes one. On
// macOS, writing under an unmounted /Volumes mount point silently creates a
// local directory instead, so without this check a NAS that failed to mount
// produces a second, empty, entirely plausible-looking Box on the local disk
// and nothing says so for weeks. A gate any command could open is not a gate.
var ErrNotABox = errors.New("not a Box: no marker file")

// MarkerPath is where the marker lives for a root. The name must be a bare
// filename: a configured path would let the check be pointed somewhere other
// than the tree being written to, which is the one thing it exists to prevent.
func MarkerPath(root, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		name = MarkerName
	}
	if name != filepath.Base(name) || strings.ContainsRune(name, filepath.Separator) {
		return "", fmt.Errorf("marker name %q is a path, not a filename", name)
	}
	return filepath.Join(root, name), nil
}

// ReadMarker reads the marker at root, which is how a command checks that it
// may write there.
func ReadMarker(root, name string) (Marker, error) {
	path, err := MarkerPath(root, name)
	if err != nil {
		return Marker{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Marker{}, fmt.Errorf("%w: %s", ErrNotABox, path)
		}
		return Marker{}, err
	}
	var marker Marker
	if err := yaml.Unmarshal(data, &marker); err != nil {
		return Marker{}, fmt.Errorf("%s: %w", path, err)
	}
	if marker.Version > MarkerVersion {
		return Marker{}, fmt.Errorf("%s: Box version %d is newer than this build reads", path, marker.Version)
	}
	return marker, nil
}

// RequireBox reports nothing and returns an error unless root is a Box. It is
// what every command that writes calls first.
func RequireBox(root, name string) error {
	_, err := ReadMarker(root, name)
	return err
}

// WriteMarker creates the marker, and refuses to touch one that is already
// there. Only dgs box init calls this.
func WriteMarker(root, name string, now time.Time) error {
	path, err := MarkerPath(root, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(Marker{Version: MarkerVersion, CreatedAt: now.Format(time.RFC3339)})
	if err != nil {
		return err
	}
	// Exclusive creation: making a Box out of a directory that is already one
	// would reset the version and the creation date, which are the two things
	// the file exists to remember.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s: already a Box", root)
		}
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
