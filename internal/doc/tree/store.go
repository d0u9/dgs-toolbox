package tree

import (
	"dgs-toolbox/internal/doc/layout"

	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store owns filesystem paths and operations for one opened tree.
type Store struct {
	// Scoped to one source listing; never retained by the opened Store.
	sourceNames map[string]map[string][]string
	root        string
	meta        string // where the marker, sidecars, definitions and trash are
	layout      layout.Layout
	outlines    int
}

// Open verifies a tree and selects the layout its marker names.
func Open(root string) (*Store, error) {
	marker, err := readMarker(root)
	if err != nil {
		return nil, err
	}
	l, err := selectLayout(marker)
	if err != nil {
		return nil, err
	}
	meta, err := metaOf(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: root, meta: meta, layout: l, outlines: outlineVersion(marker)}, nil
}

// ErrUnsupportedLayout refuses a marker describing a layout this build cannot use.
var ErrUnsupportedLayout = errors.New("unsupported doc tree layout")

func unsupportedLayout() error {
	return fmt.Errorf("%w: this tree's layout is not supported yet", ErrUnsupportedLayout)
}

// at constructs unchecked by-Item storage only for package-internal tests.
func at(root string) *Store   { return &Store{root: root, meta: root, layout: layout.ByItem()} }
func (s *Store) Root() string { return s.root }

// Folder is the Item's own folder, when the layout gives each Item one.
func (s *Store) Folder(id string) (string, bool) {
	f, ok := s.layout.(layout.Folders)
	if !ok {
		return "", false
	}
	return filepath.Join(s.root, f.Folder(id)), true
}

// Staging is the folder an import is assembled in before it is published,
// on the same volume as the Items.
func (s *Store) Staging() string { return filepath.Join(s.meta, ItemsDir) }

// Sidecar paths are relative to the metadata folder, which by Item is the root.
func (s *Store) Sidecar(id string) string { return filepath.Join(s.meta, s.layout.Sidecar(id)) }
func (s *Store) PDF(item Item, digest string) string {
	rel := s.layout.PDF(Placing(item), digest)
	if s.InPlace() {
		p, err := s.recordedPath(rel)
		if err != nil {
			return ""
		}
		return p
	}
	return filepath.Join(s.root, rel)
}
func (s *Store) Image(item Item, pdf string, o Original) string {
	rel := s.layout.Image(Placing(item), pdf, o.Digest, strings.TrimPrefix(o.Extension, "."))
	if s.InPlace() {
		p, err := s.recordedPath(rel)
		if err != nil {
			return ""
		}
		return p
	}
	return filepath.Join(s.root, rel)
}
func (s *Store) Owned(rel string, items ...Item) bool {
	placing := make([]layout.Item, len(items))
	for i, item := range items {
		placing[i] = Placing(item)
	}
	return s.layout.Owned(rel, placing...)
}

// Sidecars lists every sidecar in the tree, as full paths.
func (s *Store) Sidecars() ([]string, error) {
	rel, err := s.layout.Sidecars(os.DirFS(s.meta))
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rel))
	for i, r := range rel {
		out[i] = filepath.Join(s.meta, r)
	}
	return out, nil
}

// Placing is what a layout needs of an Item: its ID and, per PDF digest, the
// fields of the newest snapshot holding it.
func Placing(item Item) layout.Item {
	fields := map[string]map[string]string{}
	paths := map[string]string{}
	for _, r := range item.Revisions {
		if r.Digest != "" {
			fields[r.Digest] = item.FieldsAt(r.Ref())
			paths[r.Digest] = r.Path
			for _, o := range r.Originals {
				paths[o.Digest] = o.Path
			}
		}
	}
	return layout.Item{ID: item.ID, Fields: fields, Paths: paths}
}

// Items reads every sidecar in this Store's selected layout.
func (s *Store) Items() ([]Item, error) { return s.LoadItems() }

func selectLayout(marker Marker) (layout.Layout, error) {
	if marker.Layout == nil {
		return layout.ByItem(), nil
	}
	if *marker.Layout == "in-place" {
		return layout.RecordedPaths(), nil
	}
	return nil, unsupportedLayout()
}

// InPlace reports whether files have recorded paths and may be shared.
func (s *Store) InPlace() bool {
	if s == nil {
		return false
	}
	_, ok := s.layout.(layout.Folders)
	return !ok
}

// RequireFileWrites refuses attachment changes, deletion and merge until their
// in-place implementations exist. New-Item imports use Require instead.
func (s *Store) RequireFileWrites() error {
	if err := s.Require(); err != nil {
		return err
	}
	if s.InPlace() {
		return fmt.Errorf("%w: in-place attachment changes and merge are not supported yet", ErrUnsupportedOperation)
	}
	return nil
}

func outlineVersion(m Marker) int {
	if m.Outlines == OutlinesFolders {
		return 2
	}
	return 1
}

// OutlineVersion is independent of storage layout; old markers select 1.
func (s *Store) OutlineVersion() int {
	if s != nil && s.outlines == 2 {
		return 2
	}
	return 1
}

// Capabilities describe available operations, never a layout name.
type Capabilities struct {
	FileChanges        bool `json:"file_changes"`
	ImportFolder       bool `json:"import_folder"`
	AllFiles           bool `json:"all_files"`
	ReuseFiles         bool `json:"reuse_files"`
	SharedFiles        bool `json:"shared_files"`
	AttachmentChanges  bool `json:"attachment_changes"`
	WithoutFile        bool `json:"without_file"`
	Merge              bool `json:"merge"`
	ReplacementAnyType bool `json:"replacement_any_type"`
}

func (s *Store) Capabilities() Capabilities {
	recorded := s.InPlace()
	return Capabilities{FileChanges: recorded, ImportFolder: recorded, AllFiles: recorded, ReuseFiles: recorded, SharedFiles: recorded, AttachmentChanges: !recorded, WithoutFile: true, Merge: !recorded, ReplacementAnyType: recorded}
}
func (s *Store) ReportsLooseFiles() bool { return s.layout.ReportsLooseFiles() }

var ErrUnsupportedOperation = errors.New("operation unsupported by this storage layout")
