// Package check verifies a doc tree against its sidecars and changes nothing:
// every revision's PDF is there and still hashes to its name, every PDF under
// items/ is named by a sidecar, and every sidecar parses, names a known
// Template and has a HEAD among its revisions.
package check

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"dgs-toolbox/internal/doc/tree"
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

	entries, err := os.ReadDir(filepath.Join(root, tree.ItemsDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return report, err
	}
	type pending struct{ path, digest string }
	var reads []pending
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, tree.ItemsDir, entry.Name())
		named := map[string]bool{}
		sidecar := filepath.Join(dir, tree.SidecarName)
		data, err := os.ReadFile(sidecar)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			add(BadSidecar, sidecar, "no sidecar in this Item folder")
		case err != nil:
			return report, err
		default:
			var item tree.Item
			if err := yaml.Unmarshal(data, &item); err != nil {
				add(BadSidecar, sidecar, err.Error())
				break
			}
			report.Items++
			if item.ID != entry.Name() {
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
				if rev.Snapshot && len(rev.ID) == 64 && tree.SnapshotID(rev.Type, rev.Fields, rev.Digest) != rev.ID {
					add(BadSidecar, sidecar, "snapshot "+short(rev.ID)+" does not match its content")
				}
				if rev.Digest != "" {
					named[rev.Digest+".pdf"] = true
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
				path := filepath.Join(dir, rev.Digest+".pdf")
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
		files, err := os.ReadDir(dir)
		if err != nil {
			return report, err
		}
		for _, f := range files {
			if !f.IsDir() && strings.EqualFold(filepath.Ext(f.Name()), ".pdf") && !named[f.Name()] {
				add(Unnamed, filepath.Join(dir, f.Name()), "no revision names this PDF")
			}
		}
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
	sort.SliceStable(report.Problems, func(i, j int) bool { return report.Problems[i].Path < report.Problems[j].Path })
	return report, nil
}

func short(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}
