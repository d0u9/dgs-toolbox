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
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// MarkerName is the file that proves a folder is a by-Item doc tree.
const MarkerName = "dgs-doctree.yaml"

// MetaDir holds everything an in-place tree keeps — its marker (MetaMarker),
// change mark, sidecars, definitions and trash — so the root holds only the
// owner's files. A by-Item tree keeps them at the root.
const (
	MetaDir    = "_dgs"
	MetaMarker = "tree.yaml"
)

// metaOf is where root's tree keeps its metadata: MetaDir when its marker is
// there, else the root. Both markers present is an error.
func metaOf(root string) (string, error) {
	_, errMeta := os.Stat(filepath.Join(root, MetaDir, MetaMarker))
	_, errRoot := os.Stat(filepath.Join(root, MarkerName))
	switch {
	case errMeta == nil && errRoot == nil:
		return "", fmt.Errorf("%s: both %s and %s exist; keep one", root, MarkerName, filepath.Join(MetaDir, MetaMarker))
	case errMeta == nil:
		return filepath.Join(root, MetaDir), nil
	}
	return root, nil
}

func markerPath(meta, root string) string {
	if meta != root {
		return filepath.Join(meta, MetaMarker)
	}
	return filepath.Join(root, MarkerName)
}

// MarkerVersion is the tree format this build creates.
const MarkerVersion = 1

// TemplatesDir and ItemsDir are the two folders a tree keeps.
const (
	TemplatesDir = "templates"
	ItemsDir     = "items"
)

// Outline formats a marker names: Doc 1 rules built from path templates, or
// folder skeletons.
const (
	OutlinesPaths   = "paths"
	OutlinesFolders = "folders"
)

// Marker is the content of the marker file.
type Marker struct {
	Version   int     `yaml:"version"`
	CreatedAt string  `yaml:"created_at"`
	Layout    *string `yaml:"layout,omitempty"`
	Outlines  string  `yaml:"outlines,omitempty"`
}

// ErrNotATree is returned when the marker is missing. Nothing writes into a
// folder without one, and only Init creates it: a NAS that failed to mount must
// not quietly become a second, empty tree on the local disk.
var ErrNotATree = errors.New("not a doc tree: no marker file")

// ReadMarker reads root's marker.
func readMarker(root string) (Marker, error) {
	meta, err := metaOf(root)
	if err != nil {
		return Marker{}, err
	}
	path := markerPath(meta, root)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Marker{}, fmt.Errorf("%w: %s", ErrNotATree, path)
		}
		return Marker{}, err
	}
	// A yaml.Node distinguishes an omitted layout from an explicitly empty or
	// null one. Only the scalar in-place name is supported.
	var raw struct {
		Version   int       `yaml:"version"`
		CreatedAt string    `yaml:"created_at"`
		Layout    yaml.Node `yaml:"layout"`
		Outlines  yaml.Node `yaml:"outlines"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Marker{}, fmt.Errorf("%s: %w", path, err)
	}
	marker := Marker{Version: raw.Version, CreatedAt: raw.CreatedAt}
	if raw.Layout.Kind != 0 {
		name := ""
		marker.Layout = &name
		if raw.Layout.Kind == yaml.ScalarNode && raw.Layout.Tag == "!!str" {
			*marker.Layout = raw.Layout.Value
		}
	}
	if raw.Outlines.Kind != 0 {
		// 1 and 2 are the names early in-place trees were written with.
		v, tag := raw.Outlines.Value, raw.Outlines.Tag
		if raw.Outlines.Kind != yaml.ScalarNode {
			v = ""
		}
		switch {
		case tag == "!!str" && v == OutlinesFolders, tag == "!!int" && v == "2":
			marker.Outlines = OutlinesFolders
		case tag == "!!str" && v == OutlinesPaths, tag == "!!int" && v == "1":
			marker.Outlines = OutlinesPaths
		default:
			return Marker{}, fmt.Errorf("outlines must be %s or %s", OutlinesPaths, OutlinesFolders)
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
	if err == nil {
		var selected layout.Layout
		selected, err = selectLayout(marker)
		if err == nil {
			_, folders := selected.(layout.Folders)
			if st.OutlineVersion() != outlineVersion(marker) {
				return fmt.Errorf("tree Outline version changed after opening; reopen the tree")
			}
			if st.InPlace() == folders {
				return fmt.Errorf("tree layout changed after opening; reopen the tree")
			}
		}
	}
	return err
}

// Init creates a by-Item tree. Report observers receive paths after success.
func Init(root string, now time.Time, report ...func(string)) error {
	return InitWithLayout(root, now, "", report...)
}

// InitWithLayout creates a tree with a layout selected once at initialization.
// Empty selects by-Item; in-place is the only other supported layout.
func InitWithLayout(root string, now time.Time, name string, report ...func(string)) error {
	return InitWithOptions(root, now, name, 1, report...)
}

// InitWithOptions chooses storage and Outline format independently.
func InitWithOptions(root string, now time.Time, name string, outlines int, report ...func(string)) error {
	if outlines != 1 && outlines != 2 {
		return fmt.Errorf("outlines must be 1 or 2")
	}
	marker := Marker{Version: MarkerVersion, CreatedAt: now.Format(time.RFC3339)}
	if outlines == 2 {
		marker.Outlines = OutlinesFolders
	}
	if name != "" {
		marker.Layout = &name
	}
	if _, err := selectLayout(marker); err != nil {
		return err
	}
	var before []Template
	if len(report) > 0 {
		before, _ = at(root).LoadTemplates()
	}
	if _, err := metaOf(root); err != nil {
		return err
	}
	meta := root
	if name == "in-place" {
		meta = filepath.Join(root, MetaDir)
		if _, err := os.Stat(filepath.Join(root, MarkerName)); err == nil {
			return fmt.Errorf("%s: already a doc tree", root)
		}
	} else if _, err := os.Stat(filepath.Join(root, MetaDir, MetaMarker)); err == nil {
		return fmt.Errorf("%s: already a doc tree", root)
	}
	if err := os.MkdirAll(meta, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(marker)
	if err != nil {
		return err
	}
	path := markerPath(meta, root)
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
	if name == "in-place" {
		if err := os.MkdirAll(filepath.Join(meta, ItemsDir), 0755); err != nil {
			return err
		}
	}
	templates := filepath.Join(meta, TemplatesDir)
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
	example := ExampleTemplate
	if name == "in-place" {
		example = strings.Replace(example, "type: id_card\n", "type: id_card\nlocked: true              # changed identity papers must be restored, not revised\n", 1)
	}
	if err := os.WriteFile(filepath.Join(templates, "id_card.yaml"), []byte(example), 0o644); err != nil {
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
