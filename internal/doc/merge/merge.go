// Package merge brings another tree's Items, Templates and Views into a tree:
// a sub-tree made with init, or a Target read back through its export
// manifest. There is no common state to compare against, so Items are matched
// by what they are — the same ID, the same PDF for a record, the same type
// and distinguishing fields for a document — and anything both sides changed
// differently is a Conflict the owner resolves before anything is written.
package merge

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"time"

	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

// Template is a Template with the file it was read from, so a new one is
// copied as it was written, comments included.
type Template struct {
	tree.Template
	Data []byte
}

// Source is what is merged in.
type Source struct {
	Items     []tree.Item
	Templates []Template
	Views     []view.View
	// PDF is where a revision's PDF is read from.
	PDF func(item, digest string) string
}

// FromTree reads a tree to merge in.
func FromTree(root string) (Source, error) {
	if err := tree.Require(root); err != nil {
		return Source{}, err
	}
	items, err := tree.LoadItems(root)
	if err != nil {
		return Source{}, err
	}
	parsed, err := tree.LoadTemplates(root)
	if err != nil {
		return Source{}, err
	}
	var templates []Template
	for _, t := range parsed {
		data, err := os.ReadFile(filepath.Join(root, tree.TemplatesDir, t.Type+".yaml"))
		if err != nil {
			return Source{}, err
		}
		templates = append(templates, Template{t, data})
	}
	views, err := view.Load(root)
	if err != nil {
		return Source{}, err
	}
	return Source{Items: items, Templates: templates, Views: views,
		PDF: func(id, digest string) string { return tree.PDFPath(root, id, digest) }}, nil
}

// FromTarget reads a Target back through its manifest: an Item for every ID
// it records, with the revisions exported. A Target carries no Templates or
// Views, and revisions it did not export are not in it.
func FromTarget(target string, now time.Time) (Source, error) {
	m, err := export.ReadManifest(target)
	if err != nil {
		return Source{}, err
	}
	if len(m.Files) == 0 {
		return Source{}, fmt.Errorf("%s records no exported files", filepath.Join(target, export.ManifestName))
	}
	entries := append([]export.Entry(nil), m.Files...)
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Item != entries[j].Item {
			return entries[i].Item < entries[j].Item
		}
		return entries[i].Revision < entries[j].Revision
	})
	paths := map[string]string{}
	var items []tree.Item
	for _, e := range entries {
		paths[e.Item+"/"+e.Digest] = filepath.Join(target, filepath.FromSlash(e.Path))
		if len(items) == 0 || items[len(items)-1].ID != e.Item {
			items = append(items, tree.Item{ID: e.Item, Type: e.Type, Kind: tree.Kind(e.Kind), Fields: maps.Clone(e.Fields)})
		}
		it := &items[len(items)-1]
		if hasDigest(*it, e.Digest) {
			continue
		}
		it.Revisions = append(it.Revisions, tree.Revision{Digest: e.Digest, Added: now.Format(time.RFC3339), Source: e.Path})
		if e.Head {
			it.Head = e.Digest
		}
	}
	// Each entry has the fields its revision had. The Item keeps HEAD's, and
	// a revision keeps what differs from them as its own.
	byRevision := map[string]map[string]string{}
	for _, e := range entries {
		byRevision[e.Item+"/"+e.Digest] = e.Fields
	}
	for i := range items {
		it := &items[i]
		if it.Kind == tree.KindDocument && it.Head == "" {
			it.Head = it.Current()
		}
		it.Fields = maps.Clone(byRevision[it.ID+"/"+it.Current()])
		for r := range it.Revisions {
			own := map[string]string{}
			for k, v := range byRevision[it.ID+"/"+it.Revisions[r].Digest] {
				if it.Fields[k] != v {
					own[k] = v
				}
			}
			if len(own) > 0 {
				it.Revisions[r].Fields = own
			}
		}
	}
	return Source{Items: items, PDF: func(id, digest string) string { return paths[id+"/"+digest] }}, nil
}
