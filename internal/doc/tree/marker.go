// Package tree is a dgs doc tree on disk: its marker, its Templates, and the
// Items kept in it as sidecars beside their PDFs. The layout is in
// docs/apps/doc/index.md.
package tree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// MarkerName is the file that proves a folder is a doc tree.
const MarkerName = "dgs-doctree.yaml"

// MarkerVersion is the tree format this build creates.
const MarkerVersion = 1

// TemplatesDir and ItemsDir are the two folders a tree keeps.
const (
	TemplatesDir = "templates"
	ItemsDir     = "items"
)

// Marker is the content of the marker file.
type Marker struct {
	Version   int    `yaml:"version"`
	CreatedAt string `yaml:"created_at"`
}

// ErrNotATree is returned when the marker is missing. Nothing writes into a
// folder without one, and only Init creates it: a NAS that failed to mount must
// not quietly become a second, empty tree on the local disk.
var ErrNotATree = errors.New("not a doc tree: no marker file")

// ReadMarker reads root's marker.
func ReadMarker(root string) (Marker, error) {
	path := filepath.Join(root, MarkerName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Marker{}, fmt.Errorf("%w: %s", ErrNotATree, path)
		}
		return Marker{}, err
	}
	var marker Marker
	if err := yaml.Unmarshal(data, &marker); err != nil {
		return Marker{}, fmt.Errorf("%s: %w", path, err)
	}
	if marker.Version > MarkerVersion {
		return Marker{}, fmt.Errorf("%s: tree version %d is newer than this build reads", path, marker.Version)
	}
	return marker, nil
}

// Require returns an error unless root is a doc tree.
func Require(root string) error {
	_, err := ReadMarker(root)
	return err
}

// Init makes root a doc tree: it writes the marker and, when there is no
// Template yet, the example one. It refuses a folder that is already a tree.
func Init(root string, now time.Time) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(Marker{Version: MarkerVersion, CreatedAt: now.Format(time.RFC3339)})
	if err != nil {
		return err
	}
	path := filepath.Join(root, MarkerName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s: already a doc tree", root)
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
	if err := file.Close(); err != nil {
		return err
	}
	templates := filepath.Join(root, TemplatesDir)
	if err := os.MkdirAll(templates, 0o755); err != nil {
		return err
	}
	existing, err := filepath.Glob(filepath.Join(templates, "*.yaml"))
	if err != nil || len(existing) > 0 {
		return err
	}
	return os.WriteFile(filepath.Join(templates, "id_card.yaml"), []byte(ExampleTemplate), 0o644)
}

// ExampleTemplate is what Init writes into an empty templates folder, so a new
// tree has something to import with and a file to copy for the next type.
const ExampleTemplate = `# A Template: the fields asked for when a PDF of this type is imported.
# Copy this file to add a type; the file name is the type.
type: id_card
kind: document            # document: revisions and HEAD; record: one PDF
fields:
  - key: owner
    required: true
    distinguishing: true  # type + the distinguishing fields are unique
  - key: country
    type: country         # cn, CHN, China, 中国 are all kept as one
    format: zh            # zh 中国, en China, alpha2 CN, alpha3 CHN
    required: true
    distinguishing: true
  - key: number
    pattern: '(\d{17}[\dXx])'   # suggested from the recognised text
  - key: expires
    pattern: '[-－—–一~～至]\s*(\d{4}[.\-/]\d{2}[.\-/]\d{2}|长期)'   # 一 — － as recognised
defaults: {}
`
