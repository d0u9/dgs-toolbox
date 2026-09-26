// Package snapshot is a doc tree's Snapshots. A Snapshot is an Outline's
// tree, or one folder of it, as it was at one moment: each PDF's path, Item
// and revision, recorded so it is browsed and exported the same however the
// rules or the Items change after. It copies no PDF: the revisions stay in
// their Items, and one that leaves the tree is reported, never replaced.
// The rules are in docs/apps/doc/index.md.
package snapshot

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"

	"gopkg.in/yaml.v3"
)

// Dir is the folder under a tree's root that holds one file per Snapshot.
const Dir = "snapshots"

// File is one PDF of a Snapshot.
type File struct {
	// Path is where it is in the Snapshot, relative to the folder taken.
	Path string `yaml:"path" json:"path"`
	Item string `yaml:"item" json:"item"`
	// Revision is the revision's ID, or for a legacy one its PDF's digest.
	Revision string `yaml:"revision" json:"revision"`
	// Rule is the Outline's rule that placed it, for a reader.
	Rule string `yaml:"rule,omitempty" json:"rule,omitempty"`
}

// Snapshot is one file under snapshots/.
type Snapshot struct {
	Name   string `yaml:"name" json:"name"`
	About  string `yaml:"about,omitempty" json:"about,omitempty"`
	Folder string `yaml:"folder,omitempty" json:"folder,omitempty"`
	// Taken is when it was taken, RFC 3339.
	Taken string `yaml:"taken" json:"taken"`
	// Outline and Node are what it was taken from: the Outline, and the
	// folder of its tree, empty for the whole tree.
	Outline string `yaml:"outline" json:"outline"`
	Node    string `yaml:"node,omitempty" json:"node,omitempty"`
	Files   []File `yaml:"files" json:"files"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate reports the first thing wrong with s.
func (s Snapshot) Validate() error {
	if !namePattern.MatchString(s.Name) {
		return fmt.Errorf("snapshot %q: use lowercase letters, digits, _ and -", s.Name)
	}
	if s.Folder != "" && !strings.HasPrefix(s.Folder, "~") && !filepath.IsAbs(s.Folder) {
		return fmt.Errorf("snapshot %s: the folder %q is neither absolute nor under ~", s.Name, s.Folder)
	}
	if _, err := time.Parse(time.RFC3339, s.Taken); err != nil {
		return fmt.Errorf("snapshot %s: taken %q is not a time", s.Name, s.Taken)
	}
	seen := map[string]bool{}
	for _, f := range s.Files {
		if f.Path == "" || f.Item == "" || f.Revision == "" {
			return fmt.Errorf("snapshot %s: a file lacks its path, Item or revision", s.Name)
		}
		if filepath.IsAbs(f.Path) || f.Path != filepath.ToSlash(filepath.Clean(f.Path)) || strings.HasPrefix(f.Path, "../") {
			return fmt.Errorf("snapshot %s: %q is not a path inside it", s.Name, f.Path)
		}
		if seen[strings.ToLower(f.Path)] {
			return fmt.Errorf("snapshot %s: two files at %s", s.Name, f.Path)
		}
		seen[strings.ToLower(f.Path)] = true
	}
	return nil
}

// Take records the PDFs of g under the folder node, "" for all of them,
// with paths relative to it. A grouping with PDFs it cannot place is
// refused: a Snapshot is what an export would have written.
func Take(name, about string, o outline.Outline, g outline.Grouping, node string, items []tree.Item, now time.Time) (Snapshot, error) {
	if !g.Complete() {
		return Snapshot{}, fmt.Errorf("the Outline %s has PDFs it cannot place: place them first", o.Name)
	}
	s := Snapshot{Name: name, About: about, Taken: now.Format(time.RFC3339), Outline: o.Name, Node: node, Files: []File{}}
	byID := map[string]tree.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	for _, f := range g.Files {
		rel := f.Path
		if node != "" {
			if !strings.HasPrefix(f.Path, node+"/") {
				continue
			}
			rel = strings.TrimPrefix(f.Path, node+"/")
		}
		it, ok := byID[f.Item]
		if !ok || f.Revision < 1 || f.Revision > len(it.Revisions) {
			return Snapshot{}, fmt.Errorf("%s: no revision %d of %s", f.Path, f.Revision, f.Item)
		}
		s.Files = append(s.Files, File{Path: rel, Item: f.Item, Revision: it.Revisions[f.Revision-1].Ref(), Rule: f.View})
	}
	if len(s.Files) == 0 {
		return Snapshot{}, fmt.Errorf("no PDF under %q to take", node)
	}
	return s, s.Validate()
}

// Lost is a file of a Snapshot whose Item or revision the tree no longer
// has, or whose revision has no PDF.
type Lost struct {
	File
	Why string `json:"why"`
}

// Resolved is a Snapshot's files as the tree has them now: the tree of
// those still there, and what is lost.
type Resolved struct {
	Files []view.File   `json:"files"`
	Lost  []Lost        `json:"lost"`
	Root  *outline.Node `json:"root"`
}

// Resolve finds each file's revision in items. Each file found is marked
// with the Snapshot's name as its rule, since an export owns it by that.
func Resolve(s Snapshot, items []tree.Item) Resolved {
	byID := map[string]tree.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	out := Resolved{Files: []view.File{}, Lost: []Lost{}}
	for _, f := range s.Files {
		it, ok := byID[f.Item]
		if !ok {
			out.Lost = append(out.Lost, Lost{f, "the Item is no longer in the tree"})
			continue
		}
		n := -1
		for i, r := range it.Revisions {
			if r.Ref() == f.Revision {
				n = i
			}
		}
		switch {
		case n < 0:
			out.Lost = append(out.Lost, Lost{f, "the revision is no longer in the Item"})
		case it.Revisions[n].Digest == "":
			out.Lost = append(out.Lost, Lost{f, "the revision has no PDF"})
		default:
			out.Files = append(out.Files, view.File{Path: f.Path, Item: f.Item, Digest: it.Revisions[n].Digest, Revision: n + 1, View: s.Name})
		}
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	out.Root = outline.Nest(out.Files)
	return out
}

// Parse reads one Snapshot file, refusing keys it does not know.
func Parse(data []byte) (Snapshot, error) {
	var s Snapshot
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&s); err != nil {
		return Snapshot{}, err
	}
	return s, s.Validate()
}

// Path is where the Snapshot named name is kept.
func Path(root, name string) string { return filepath.Join(root, Dir, name+".yaml") }

// Load reads every snapshots/*.yaml under root, sorted by name.
func Load(root string) ([]Snapshot, error) {
	paths, err := filepath.Glob(filepath.Join(root, Dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	out := make([]Snapshot, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		s, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if name := strings.TrimSuffix(filepath.Base(path), ".yaml"); name != s.Name {
			return nil, fmt.Errorf("%s: name is %s, so the file should be %s.yaml", path, s.Name, s.Name)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Save writes s. Create refuses a name an Outline or another Snapshot has;
// otherwise s replaces the Snapshot of its name, whose files it must keep:
// only the name's file, about and folder may change after it is taken.
// Previous, when it names another Snapshot, is removed after.
func Save(root, previous string, s Snapshot, create bool) error {
	if err := s.Validate(); err != nil {
		return err
	}
	taken := func(name string) bool {
		_, err := os.Stat(Path(root, name))
		return err == nil
	}
	if _, err := os.Stat(filepath.Join(root, outline.Dir, s.Name+".yaml")); err == nil {
		return fmt.Errorf("an Outline is named %s: choose another name", s.Name)
	}
	if create || previous != s.Name {
		if taken(s.Name) {
			return fmt.Errorf("a Snapshot is named %s already", s.Name)
		}
	}
	if !create {
		from := previous
		if from == "" {
			from = s.Name
		}
		data, err := os.ReadFile(Path(root, from))
		if err != nil {
			return fmt.Errorf("no Snapshot named %s", from)
		}
		old, err := Parse(data)
		if err != nil {
			return err
		}
		if old.Taken != s.Taken || old.Outline != s.Outline || old.Node != s.Node || !sameFiles(old.Files, s.Files) {
			return fmt.Errorf("the Snapshot %s is taken: only its name, about and folder change", from)
		}
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	if err := write(filepath.Join(root, Dir), s.Name+".yaml", data); err != nil {
		return err
	}
	if !create && previous != "" && previous != s.Name {
		return os.Remove(Path(root, previous))
	}
	return nil
}

func sameFiles(a, b []File) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// write puts data at dir/name through a temporary file and a rename.
func write(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".snapshot-*.dgs-part")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), filepath.Join(dir, name))
}

// Trash moves a Snapshot's file into the tree's trash, and answers where it
// went. Its Items stay where they are.
func Trash(root, name string, now time.Time) (string, error) {
	if !namePattern.MatchString(name) {
		return "", fmt.Errorf("snapshot %q: not a snapshot name", name)
	}
	from := Path(root, name)
	if _, err := os.Lstat(from); err != nil {
		return "", fmt.Errorf("no Snapshot named %s", name)
	}
	dir := filepath.Join(root, tree.TrashDir, Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	to := filepath.Join(dir, name+"-"+now.UTC().Format("20060102T150405Z")+".yaml")
	if _, err := os.Lstat(to); err == nil {
		return "", fmt.Errorf("%s is already in the trash", to)
	}
	return to, os.Rename(from, to)
}
