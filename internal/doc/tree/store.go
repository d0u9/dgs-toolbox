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
	root   string
	layout layout.Layout
}

// Open verifies a tree and opens its current by-Item layout.
func Open(root string) (*Store, error) {
	marker, err := readMarker(root)
	if err != nil {
		return nil, err
	}
	if marker.Layout != nil {
		return nil, unsupportedLayout()
	}
	return &Store{root: root, layout: layout.ByItem()}, nil
}

// ErrUnsupportedLayout refuses a marker describing a layout this build cannot use.
var ErrUnsupportedLayout = errors.New("unsupported doc tree layout")

func unsupportedLayout() error {
	return fmt.Errorf("%w: this tree's layout is not supported yet", ErrUnsupportedLayout)
}

// at constructs unchecked by-Item storage only for package-internal tests.
func at(root string) *Store   { return &Store{root: root, layout: layout.ByItem()} }
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
func (s *Store) Staging() string { return filepath.Join(s.root, ItemsDir) }

func (s *Store) Sidecar(id string) string { return filepath.Join(s.root, s.layout.Sidecar(id)) }
func (s *Store) PDF(item Item, digest string) string {
	return filepath.Join(s.root, s.layout.PDF(Placing(item), digest))
}
func (s *Store) Image(item Item, pdf string, o Original) string {
	return filepath.Join(s.root, s.layout.Image(Placing(item), pdf, o.Digest, strings.TrimPrefix(o.Extension, ".")))
}
func (s *Store) Owned(rel string) bool { return s.layout.Owned(rel) }

// Sidecars lists every sidecar in the tree, as full paths.
func (s *Store) Sidecars() ([]string, error) {
	rel, err := s.layout.Sidecars(os.DirFS(s.root))
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rel))
	for i, r := range rel {
		out[i] = filepath.Join(s.root, r)
	}
	return out, nil
}

// Placing is what a layout needs of an Item: its ID and, per PDF digest, the
// fields of the newest snapshot holding it.
func Placing(item Item) layout.Item {
	fields := map[string]map[string]string{}
	for _, r := range item.Revisions {
		if r.Digest != "" {
			fields[r.Digest] = item.FieldsAt(r.Ref())
		}
	}
	return layout.Item{ID: item.ID, Fields: fields}
}

// Items reads every sidecar in this Store's selected layout.
func (s *Store) Items() ([]Item, error) { return s.LoadItems() }
