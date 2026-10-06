// Package check verifies a doc tree against its sidecars and changes nothing:
// every revision's PDF is there and still hashes to its name, every PDF under
// items/ is named by a sidecar, and every sidecar parses, names a known
// Template and has a HEAD among its revisions. The files under rules/,
// outlines/, snapshots/ and cases/ must each read, and name only rules,
// Snapshots, Items and revisions the tree has.
package check

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"dgs-toolbox/internal/doc/cases"
	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/snapshot"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

// Kind is what is wrong.
type Kind string

const (
	Missing     Kind = "missing"      // a revision's PDF is not there
	Changed     Kind = "changed"      // a PDF no longer hashes to its name
	Unnamed     Kind = "unnamed"      // a PDF under items/ no sidecar names
	BadSidecar  Kind = "bad-sidecar"  // a sidecar that does not parse, or is not its folder's
	UnknownType Kind = "unknown-type" // a sidecar naming a type with no Template
	BadHead     Kind = "bad-head"     // HEAD is not one of the revisions
	BadFile     Kind = "bad-file"     // a rule, Outline, Snapshot or Case file that does not read
	Dangling    Kind = "dangling"     // a file naming a Snapshot, Item or revision the tree lacks
)

// Problem is one thing wrong, at a path relative to the tree's root.
type Problem struct {
	Kind   Kind
	Path   string
	Detail string
}

// Report is the result of a check.
type Report struct {
	Items    int
	Checked  int
	Bytes    int64
	Problems []Problem
}

// Progress is told of each PDF before it is read, counting from 0.
type Progress func(path string, done, total int)

// Tree checks root. An error is returned only when the check itself cannot
// run — no marker, Templates that do not load, a folder that cannot be read;
// what is wrong with the tree is in the Report.
func Tree(ctx context.Context, root string, progress Progress) (Report, error) {
	var report Report
	if err := tree.Require(root); err != nil {
		return report, err
	}
	templates, err := tree.LoadTemplates(root)
	if err != nil {
		return report, err
	}
	known := map[string]bool{}
	for _, t := range templates {
		known[t.Type] = true
	}
	rel := func(p string) string {
		r, err := filepath.Rel(root, p)
		if err != nil {
			return p
		}
		return filepath.ToSlash(r)
	}
	add := func(kind Kind, path, detail string) {
		report.Problems = append(report.Problems, Problem{Kind: kind, Path: rel(path), Detail: detail})
	}

	store := tree.At(root)
	sidecars, err := store.Sidecars()
	if err != nil {
		return report, err
	}
	// revisions holds, per Item whose sidecar parses, the IDs and digests of
	// its revisions, for the files that name them.
	revisions := map[string]map[string]bool{}
	// named holds every file a sidecar names, as the layout places it.
	named := map[string]bool{}
	type pending struct{ path, digest string }
	var reads []pending
	for _, sidecar := range sidecars {
		data, err := os.ReadFile(sidecar)
		switch {
		case err != nil:
			return report, err
		default:
			var item tree.Item
			if err := yaml.Unmarshal(data, &item); err != nil {
				add(BadSidecar, sidecar, err.Error())
				break
			}
			report.Items++
			revs := map[string]bool{}
			for _, rev := range item.Revisions {
				revs[rev.ID], revs[rev.Digest] = true, true
			}
			delete(revs, "")
			revisions[item.ID] = revs
			if item.ID == "" || store.Sidecar(item.ID) != sidecar {
				add(BadSidecar, sidecar, fmt.Sprintf("id %q is not the folder's name", item.ID))
			}
			if !known[item.Type] {
				add(UnknownType, sidecar, fmt.Sprintf("type %q has no Template", item.Type))
			}
			refs := map[string]bool{}
			for _, rev := range item.Revisions {
				if refs[rev.Ref()] {
					add(BadSidecar, sidecar, "duplicate revision "+rev.Ref())
				}
				refs[rev.Ref()] = true
				for _, o := range rev.Originals {
					original := store.OriginalPath(item, rev.Digest, o)
					if original == "" {
						add(BadSidecar, sidecar, "invalid original image reference")
					} else if !named[original] {
						named[original] = true
						if _, err := os.Stat(original); errors.Is(err, fs.ErrNotExist) {
							add(Missing, original, "original image")
						} else {
							reads = append(reads, pending{original, o.Digest})
						}
					}
				}

				if rev.Snapshot && len(rev.ID) == 64 && tree.SnapshotID(rev.Type, rev.Fields, rev.Digest) != rev.ID {
					add(BadSidecar, sidecar, "snapshot "+short(rev.ID)+" does not match its content")
				}
				if rev.Digest != "" {
					named[store.PDF(item, rev.Digest)] = true
				}
			}
			if item.Head != "" && !refs[item.Head] {
				add(BadHead, sidecar, "HEAD "+short(item.Head)+" is not one of the revisions")
			}
			seenFiles := map[string]bool{}
			for _, rev := range item.Revisions {
				if rev.Digest == "" && rev.ID != "" {
					continue
				}
				path := store.PDF(item, rev.Digest)
				if seenFiles[path] {
					continue
				}
				seenFiles[path] = true
				if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
					add(Missing, path, "revision added "+rev.Added)
					continue
				}
				reads = append(reads, pending{path, rev.Digest})
			}
		}
	}
	if err := loose(root, store, named, add); err != nil {
		return report, err
	}

	for i, r := range reads {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if progress != nil {
			progress(rel(r.path), i, len(reads))
		}
		digest, err := tree.FileDigest(r.path)
		if err != nil {
			return report, err
		}
		if info, err := os.Stat(r.path); err == nil {
			report.Bytes += info.Size()
		}
		report.Checked++
		if digest != r.digest {
			add(Changed, r.path, "now hashes to "+short(digest))
		}
	}
	if err := files(root, revisions, add); err != nil {
		return report, err
	}
	sort.SliceStable(report.Problems, func(i, j int) bool { return report.Problems[i].Path < report.Problems[j].Path })
	return report, nil
}

// loose reports the PDFs the layout places that no sidecar names, and an
// Item's own folder with no sidecar in it.
func loose(root string, store *tree.Store, named map[string]bool, add func(Kind, string, string)) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			// A hidden folder is an import still being written; the trash
			// is checked by no one.
			if p != root && (strings.HasPrefix(d.Name(), ".") || rel == tree.TrashDir) {
				return filepath.SkipDir
			}
			if folder, ok := store.Folder(d.Name()); ok && folder == p {
				if _, err := os.Stat(store.Sidecar(d.Name())); errors.Is(err, fs.ErrNotExist) {
					add(BadSidecar, store.Sidecar(d.Name()), "no sidecar in this Item folder")
				}
			}
			return nil
		}
		if store.Owned(rel) && strings.EqualFold(filepath.Ext(p), ".pdf") && !named[p] {
			add(Unnamed, p, "no revision names this PDF")
		}
		return nil
	})
}

func short(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

// files checks the files under rules/, outlines/, snapshots/ and cases/:
// each must read on its own, and what it names must be in the tree. An
// Outline naming a rule whose file does not read is reported as naming a
// rule the tree lacks, beside that rule's own problem.
func files(root string, revisions map[string]map[string]bool, add func(Kind, string, string)) error {
	glob := func(dir string) ([]string, error) { return tree.At(root).DefinitionFiles(dir) }
	item := func(path, what, id, revision string) {
		revs, ok := revisions[id]
		switch {
		case !ok:
			add(Dangling, path, what+" names Item "+id+", which is not in items/")
		case revision != "" && !revs[revision]:
			add(Dangling, path, what+" names revision "+short(revision)+" of Item "+id+", which it does not have")
		}
	}

	paths, err := glob(outline.RulesDir)
	if err != nil {
		return err
	}
	rules := map[string]view.View{}
	for _, path := range paths {
		r, err := outline.ReadRule(path)
		if err != nil {
			add(BadFile, path, strip(path, err))
			continue
		}
		rules[r.Name] = r
	}

	if paths, err = glob(snapshot.Dir); err != nil {
		return err
	}
	snapshots := map[string]bool{}
	for _, path := range paths {
		s, err := snapshot.ReadFile(path)
		if err != nil {
			add(BadFile, path, strip(path, err))
			continue
		}
		snapshots[s.Name] = true
		for _, f := range s.Files {
			item(path, f.Path, f.Item, f.Revision)
		}
	}

	if paths, err = glob(outline.Dir); err != nil {
		return err
	}
	for _, path := range paths {
		o, err := outline.ReadFile(path, rules)
		if err != nil {
			add(BadFile, path, strip(path, err))
			continue
		}
		for _, m := range o.Snapshots {
			if !snapshots[m.Name] {
				add(Dangling, path, "names Snapshot "+m.Name+", which is not in "+snapshot.Dir+"/")
			}
		}
	}

	if paths, err = glob(cases.Dir); err != nil {
		return err
	}
	for _, path := range paths {
		c, err := cases.ReadFile(path)
		if err != nil {
			add(BadFile, path, strip(path, err))
			continue
		}
		for _, e := range c.Entries {
			item(path, "entry", e.Item, "")
		}
		for _, n := range c.Needs {
			if n.Item != "" {
				item(path, "need "+strconv.Quote(n.Text), n.Item, "")
			}
		}
	}
	return nil
}

// strip drops the path an error begins with, as a Problem carries it.
func strip(path string, err error) string {
	return strings.TrimPrefix(err.Error(), path+": ")
}
