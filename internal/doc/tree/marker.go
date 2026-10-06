// Package tree is a dgs doc tree on disk: its marker, its Templates, and the
// Items kept in it as sidecars beside their PDFs. The layout is in
// docs/apps/doc/index.md.
package tree

import (
	"dgs-toolbox/internal/doc/layout"
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
	Version   int             `yaml:"version"`
	CreatedAt string          `yaml:"created_at"`
	Layout    *LayoutPatterns `yaml:"layout,omitempty"`
}

// LayoutPatterns describes a future by-fields layout. Any non-nil value
// is refused by Open until that layout is implemented, including an empty map.
type LayoutPatterns struct {
	Path string `yaml:"path"`
	File string `yaml:"file"`
}

// ErrNotATree is returned when the marker is missing. Nothing writes into a
// folder without one, and only Init creates it: a NAS that failed to mount must
// not quietly become a second, empty tree on the local disk.
var ErrNotATree = errors.New("not a doc tree: no marker file")

// ReadMarker reads root's marker.
func readMarker(root string) (Marker, error) {
	path := filepath.Join(root, MarkerName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Marker{}, fmt.Errorf("%w: %s", ErrNotATree, path)
		}
		return Marker{}, err
	}
	// A yaml.Node distinguishes an omitted layout from an explicitly empty or
	// null one. Every supplied layout is unsupported at this stage.
	var raw struct {
		Version   int       `yaml:"version"`
		CreatedAt string    `yaml:"created_at"`
		Layout    yaml.Node `yaml:"layout"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Marker{}, fmt.Errorf("%s: %w", path, err)
	}
	marker := Marker{Version: raw.Version, CreatedAt: raw.CreatedAt}
	if raw.Layout.Kind != 0 {
		marker.Layout = &LayoutPatterns{}
		if raw.Layout.Kind == yaml.MappingNode {
			_ = raw.Layout.Decode(marker.Layout)
		}
	}
	if marker.Version > MarkerVersion {
		return Marker{}, fmt.Errorf("%s: tree version %d is newer than this build reads", path, marker.Version)
	}
	return marker, nil
}

// Require returns an error unless root is a doc tree.
func (st *Store) Require() error {
	if st == nil {
		return ErrNotATree
	}
	marker, err := readMarker(st.root)
	if err == nil && marker.Layout != nil {
		return unsupportedLayout()
	}
	return err
}

// Init makes root a doc tree: it writes the marker and, when there is no
// Template yet, the example one. It refuses a folder that is already a tree.
// Optional report observers receive the CLI report paths after success.
func Init(root string, now time.Time, report ...func(string)) error {
	var before []Template
	if len(report) > 0 {
		before, _ = (&Store{root: root, layout: layout.ByItem()}).LoadTemplates()
	}
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
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		for _, notify := range report {
			notify(path)
			if len(before) == 0 {
				notify(filepath.Join(templates, "id_card.yaml"))
			}
		}
		return nil
	}
	if err := os.WriteFile(filepath.Join(templates, "id_card.yaml"), []byte(ExampleTemplate), 0o644); err != nil {
		return err
	}
	for _, notify := range report {
		notify(path)
		if len(before) == 0 {
			notify(filepath.Join(templates, "id_card.yaml"))
		}
	}
	return nil
}

// ExampleTemplate is what Init writes into an empty templates folder, so a new
// tree has something to import with and a file to copy for the next type.
const ExampleTemplate = `# A Template: the fields asked for when a PDF of this type is imported.
# Copy this file to add a type; the file name is the type.
type: id_card
description: Identity cards and household registers
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
    per_revision: true    # a renewed card has its own; adding it asks again
    pattern: '(\d{17}[\dXx])'   # suggested from the recognised text
  - key: expires
    per_revision: true
    patterns:             # tried in order; the first date found is suggested
      - '[-－—–一~～至]\s*(\d{4}[.\-/]\d{2}[.\-/]\d{2}|长期)'   # 2016.01.01-2036.01.01; 一 — － as recognised
      - '(?i)(?:expiry|expires|date of expiry|valid until)\W*(\d{1,2}[./-]\d{1,2}[./-]\d{4})'   # Expiry: 03/04/2031
defaults: {}
`
