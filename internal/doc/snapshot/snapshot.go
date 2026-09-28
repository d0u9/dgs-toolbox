// Package snapshot is a doc tree's Snapshots. A Snapshot is a fixed
// subtree of PDFs: each PDF's path, Item and revision, taken from a rule's
// result at one moment and edited by hand after, so it stays the same
// however the rules or the Items change. An Outline puts it in its tree as
// a folder, as it puts a rule's PDFs. It copies no PDF: the revisions stay
// in their Items, and one that leaves the tree is reported, never replaced.
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

	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"

	"gopkg.in/yaml.v3"
)

// Dir is the folder under a tree's root that holds one file per Snapshot.
const Dir = "snapshots"

// File is one PDF of a Snapshot.
type File struct {
	// Path is where it is in the Snapshot, relative to the Snapshot's folder.
	Path string `yaml:"path" json:"path"`
	Item string `yaml:"item" json:"item"`
	// Revision is the revision's ID, or for a legacy one its PDF's digest.
	Revision string `yaml:"revision" json:"revision"`
	// Rule is the rule that placed it when it was taken, for a reader.
	Rule string `yaml:"rule,omitempty" json:"rule,omitempty"`
}

// Snapshot is one file under snapshots/.
type Snapshot struct {
	Name  string `yaml:"name" json:"name"`
	About string `yaml:"about,omitempty" json:"about,omitempty"`
	// Taken is when it was taken, RFC 3339.
	Taken string `yaml:"taken" json:"taken"`
	// Rule is the rule it was taken from, empty when it was begun empty.
	Rule string `yaml:"rule,omitempty" json:"rule,omitempty"`
	// Outline, Node and Folder are kept from a Snapshot taken before
	// Snapshots stood alone: the Outline and folder it was taken from, and
	// where it was exported. Nothing reads them.
	Outline string `yaml:"outline,omitempty" json:"outline,omitempty"`
	Node    string `yaml:"node,omitempty" json:"node,omitempty"`
	Folder  string `yaml:"folder,omitempty" json:"folder,omitempty"`
	Files   []File `yaml:"files" json:"files"`
	// Folders are folders made by hand, kept even while no PDF is in them.
	// A folder holding a PDF needs no entry; an Outline and an export see
	// only the folders its PDFs are in.
	Folders []string `yaml:"folders,omitempty" json:"folders,omitempty"`
	// Naming is the layout, as a rule's, a PDF added by hand is named by,
	// relative to the folder it is added to. Empty names it by its label.
	Naming string `yaml:"naming,omitempty" json:"naming,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate reports the first thing wrong with s.
func (s Snapshot) Validate() error {
	if !namePattern.MatchString(s.Name) {
		return fmt.Errorf("snapshot %q: use lowercase letters, digits, _ and -", s.Name)
	}
	if _, err := time.Parse(time.RFC3339, s.Taken); err != nil {
		return fmt.Errorf("snapshot %s: taken %q is not a time", s.Name, s.Taken)
	}
	if s.Naming != "" {
		if _, err := view.Parse(s.Naming); err != nil {
			return fmt.Errorf("snapshot %s: naming: %w", s.Name, err)
		}
	}
	seen := map[string]bool{}
	for _, f := range s.Files {
		if f.Path == "" || f.Item == "" || f.Revision == "" {
			return fmt.Errorf("snapshot %s: a file lacks its path, Item or revision", s.Name)
		}
		if !inside(f.Path) {
			return fmt.Errorf("snapshot %s: %q is not a path inside it", s.Name, f.Path)
		}
		if seen[strings.ToLower(f.Path)] {
			return fmt.Errorf("snapshot %s: two files at %s", s.Name, f.Path)
		}
		seen[strings.ToLower(f.Path)] = true
	}
	folders := map[string]bool{}
	for _, d := range s.Folders {
		if !inside(d) {
			return fmt.Errorf("snapshot %s: folder %q is not a path inside it", s.Name, d)
		}
		if seen[strings.ToLower(d)] {
			return fmt.Errorf("snapshot %s: %s is a file and a folder", s.Name, d)
		}
		if folders[strings.ToLower(d)] {
			return fmt.Errorf("snapshot %s: folder %s twice", s.Name, d)
		}
		folders[strings.ToLower(d)] = true
	}
	for _, f := range s.Files {
		for d := f.Path; strings.Contains(d, "/"); {
			d = d[:strings.LastIndex(d, "/")]
			if seen[strings.ToLower(d)] {
				return fmt.Errorf("snapshot %s: %s is a file and a folder", s.Name, d)
			}
		}
	}
	return nil
}

// inside reports whether p is a clean slash-separated path within a
// Snapshot's folder.
func inside(p string) bool {
	return p != "" && p != "." && !filepath.IsAbs(p) && !strings.HasPrefix(p, "/") &&
		p == filepath.ToSlash(filepath.Clean(p)) && p != ".." && !strings.HasPrefix(p, "../")
}

// Take records the PDFs rule places now, at the paths it gives them. A
// rule with PDFs it cannot place is refused: place them first. A nil rule
// begins an empty Snapshot, filled by hand.
func Take(name, about string, rule *view.View, items []tree.Item, names view.TypeNames, now time.Time) (Snapshot, error) {
	s := Snapshot{Name: name, About: about, Taken: now.Format(time.RFC3339), Files: []File{}}
	if rule == nil {
		return s, s.Validate()
	}
	s.Rule = rule.Name
	plan, err := view.Build(*rule, items, names)
	if err != nil {
		return Snapshot{}, err
	}
	if !plan.Complete() {
		return Snapshot{}, fmt.Errorf("the rule %s has PDFs it cannot place: place them first", rule.Name)
	}
	byID := map[string]tree.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	for _, f := range plan.Files {
		it, ok := byID[f.Item]
		if !ok || f.Revision < 1 || f.Revision > len(it.Revisions) {
			return Snapshot{}, fmt.Errorf("%s: no revision %d of %s", f.Path, f.Revision, f.Item)
		}
		s.Files = append(s.Files, File{Path: f.Path, Item: f.Item, Revision: it.Revisions[f.Revision-1].Ref(), Rule: rule.Name})
	}
	if len(s.Files) == 0 {
		return Snapshot{}, fmt.Errorf("the rule %s selects no PDF", rule.Name)
	}
	return s, s.Validate()
}

// Lost is a file of a Snapshot whose Item or revision the tree no longer
// has, or whose revision has no PDF.
type Lost struct {
	File
	Why string `json:"why"`
}

// Resolved is a Snapshot's files as the tree has them now: those still
// there, and what is lost.
type Resolved struct {
	Files []view.File `json:"files"`
	Lost  []Lost      `json:"lost"`
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
		s, err := ReadFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ReadFile reads one Snapshot file: it must parse, be valid, and be named
// for its Snapshot.
func ReadFile(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	s, err := Parse(data)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	if name := strings.TrimSuffix(filepath.Base(path), ".yaml"); name != s.Name {
		return Snapshot{}, fmt.Errorf("%s: name is %s, so the file should be %s.yaml", path, s.Name, s.Name)
	}
	return s, nil
}

// Save writes s. Create refuses a name another Snapshot has, as does a
// rename; a rule or an Outline may share it.
// Previous, when it names another Snapshot, is removed after: package
// outline renames it in the Outlines using it.
func Save(root, previous string, s Snapshot, create bool) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if create || (previous != "" && previous != s.Name) {
		if _, err := os.Stat(Path(root, s.Name)); err == nil {
			return fmt.Errorf("a Snapshot is named %s already", s.Name)
		}
	}
	if !create {
		from := previous
		if from == "" {
			from = s.Name
		}
		if _, err := os.Stat(Path(root, from)); err != nil {
			return fmt.Errorf("no Snapshot named %s", from)
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
