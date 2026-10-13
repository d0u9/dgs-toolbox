// Package layout places a doc tree's files. It only computes paths: reading,
// writing and renaming stay in package tree.
package layout

import (
	"golang.org/x/text/unicode/norm"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

// SidecarName is a by-Item sidecar's file name.
const SidecarName = "item.dgs-item.yaml"

// An Item is what a layout needs of one: its ID and, for each PDF digest, the
// fields of the newest snapshot holding it. A layout built from fields reads
// them; by Item reads only the ID.
type Item struct {
	ID     string
	Fields map[string]map[string]string
	Paths  map[string]string
}

// A Move renames one file, both paths relative to the root.
type Move struct{ From, To string }

// A Layout places an Item's files under a tree's root. Every path is
// relative to the root.
type Layout interface {
	// Sidecar is where the Item's sidecar is.
	Sidecar(id string) string
	// PDF is where the Item's PDF of this digest belongs.
	PDF(item Item, digest string) string
	// Image is where an original page image of that PDF belongs.
	Image(item Item, pdf, image, ext string) string
	// Sidecars lists every sidecar in the tree.
	Sidecars(tree fs.FS) ([]string, error)
	// Moves are the renames that take before's files to where after places
	// them. By Item, there are never any.
	Moves(before, after Item) []Move
	// Owned tells whether a path is one the layout places, so check can
	// report the rest as loose.
	Owned(rel string, items ...Item) bool
	ReportsLooseFiles() bool
}

// Folders is a Layout that gives every Item a folder of its own, holding all
// its files and nothing else's. Deleting such an Item moves the folder whole.
type Folders interface {
	Folder(id string) string
}

type byItem struct{}

// ByItem is the default layout: items/<id>/ holding the sidecar, the PDFs and
// the original images.
func ByItem() Layout { return byItem{} }

func (byItem) Folder(id string) string    { return filepath.Join("items", id) }
func (b byItem) Sidecar(id string) string { return filepath.Join(b.Folder(id), SidecarName) }
func (b byItem) PDF(item Item, digest string) string {
	return filepath.Join(b.Folder(item.ID), digest+".pdf")
}
func (b byItem) Image(item Item, _, image, ext string) string {
	return filepath.Join(b.Folder(item.ID), image+"."+ext)
}
func (byItem) Moves(Item, Item) []Move { return nil }

func (byItem) Sidecars(tree fs.FS) ([]string, error) {
	found, err := fs.Glob(tree, path.Join("items", "*", SidecarName))
	if err != nil {
		return nil, err
	}
	out := found[:0]
	for _, f := range found {
		// A hidden folder is an import still being written.
		if !strings.HasPrefix(path.Base(path.Dir(f)), ".") {
			out = append(out, filepath.FromSlash(f))
		}
	}
	return out, nil
}

func (byItem) Owned(rel string, _ ...Item) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	return strings.HasPrefix(rel, "items/")
}

// RecordedPaths records files where their owner keeps them; it has no Item folders.
func RecordedPaths() Layout { return inPlace{} }

type inPlace struct{}

func (inPlace) Sidecar(id string) string            { return filepath.Join("items", id+".dgs-item.yaml") }
func (inPlace) PDF(item Item, digest string) string { return filepath.FromSlash(item.Paths[digest]) }
func (inPlace) Image(item Item, _ string, image, _ string) string {
	return filepath.FromSlash(item.Paths[image])
}
func (inPlace) Moves(Item, Item) []Move { return nil }
func (inPlace) Sidecars(tree fs.FS) ([]string, error) {
	found, err := fs.Glob(tree, "items/*.dgs-item.yaml")
	out := found[:0]
	for _, f := range found {
		if !strings.HasPrefix(path.Base(f), ".") {
			out = append(out, filepath.FromSlash(f))
		}
	}
	return out, err
}
func (inPlace) Owned(rel string, items ...Item) bool {
	rel = norm.NFC.String(filepath.ToSlash(rel))
	for _, item := range items {
		for _, p := range item.Paths {
			if norm.NFC.String(p) == rel {
				return true
			}
		}
	}
	return false
}

func (byItem) ReportsLooseFiles() bool  { return true }
func (inPlace) ReportsLooseFiles() bool { return false }
