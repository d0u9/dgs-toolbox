// Package outline is a doc tree's Outlines and the rules they use. An
// Outline is a tree of PDFs made by its rules and its Snapshots: each rule
// selects Items and names the path each of their PDFs has, and each
// Snapshot is a fixed subtree put in as a folder at a path the Outline
// gives it. Together they are the tree, browsed and, into a folder chosen
// then, exported. Rules and Snapshots are kept apart from the Outlines,
// under rules/ and snapshots/, so several Outlines use one. The rules are
// in docs/apps/doc/index.md.
package outline

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"dgs-toolbox/internal/doc/snapshot"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"

	"gopkg.in/yaml.v3"
)

// Dir is the folder under a tree's root that holds one file per Outline,
// and RulesDir the one that holds one file per rule.
const (
	Dir      = "outlines"
	RulesDir = "rules"
)

// Outline is one file under outlines/, its rules read from rules/. The file
// names them; an Outline written before rules were shared holds them itself,
// which Load still reads and Migrate splits.
type Outline struct {
	Name string `yaml:"name" json:"name"`
	// About says what it is for: "read on the phone".
	About string `yaml:"about,omitempty" json:"about,omitempty"`
	// Folder is where it is exported unless another is chosen then. A
	// leading ~ is the home directory of the machine exporting. Empty asks
	// every time.
	Folder string `yaml:"folder,omitempty" json:"folder,omitempty"`
	// Rules place the PDFs, each named uniquely within the Outline.
	Rules []view.View `yaml:"rules" json:"rules"`
	// Snapshots are put in the tree, each as a folder of its name.
	Snapshots []Mount `yaml:"snapshots,omitempty" json:"snapshots"`
}

// Mount puts the Snapshot named Name in an Outline's tree, as the folder
// At/Name, or At/As when As is given.
type Mount struct {
	Name string `yaml:"name" json:"name"`
	// At is the folder it goes in, empty for the top.
	At string `yaml:"at,omitempty" json:"at,omitempty"`
	// As is the folder's name, empty for the Snapshot's own name. It is
	// one name: a folder is placed by At.
	As string `yaml:"as,omitempty" json:"as,omitempty"`
}

// Folder is the Snapshot's folder in the tree.
func (m Mount) Folder() string {
	if m.As != "" {
		return path.Join(m.At, m.As)
	}
	return path.Join(m.At, m.Name)
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate reports the first thing wrong with o.
func (o Outline) Validate() error {
	if !namePattern.MatchString(o.Name) {
		return fmt.Errorf("outline %q: use lowercase letters, digits, _ and -", o.Name)
	}
	if o.Folder != "" && !strings.HasPrefix(o.Folder, "~") && !filepath.IsAbs(o.Folder) {
		return fmt.Errorf("outline %s: the folder %q is neither absolute nor under ~", o.Name, o.Folder)
	}
	seen := map[string]bool{}
	for _, r := range o.Rules {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("outline %s: %s", o.Name, strings.Replace(err.Error(), "view ", "rule ", 1))
		}
		if seen[r.Name] {
			return fmt.Errorf("outline %s: two rules are named %s", o.Name, r.Name)
		}
		seen[r.Name] = true
	}
	for _, m := range o.Snapshots {
		if !namePattern.MatchString(m.Name) {
			return fmt.Errorf("outline %s: %q is not a Snapshot's name", o.Name, m.Name)
		}
		if m.At != "" && (path.IsAbs(m.At) || m.At != path.Clean(m.At) || m.At == ".." || strings.HasPrefix(m.At, "../") || strings.Contains(m.At, "\\")) {
			return fmt.Errorf("outline %s: the Snapshot %s's folder %q is not a path inside the tree", o.Name, m.Name, m.At)
		}
		if m.As != "" && (m.As == "." || m.As == ".." || strings.ContainsAny(m.As, "/\\") || strings.TrimSpace(m.As) != m.As) {
			return fmt.Errorf("outline %s: the Snapshot %s's folder name %q is not one name", o.Name, m.Name, m.As)
		}
		if seen[m.Name] {
			return fmt.Errorf("outline %s: %s is named twice among its rules and Snapshots", o.Name, m.Name)
		}
		seen[m.Name] = true
	}
	return nil
}

// Expand is folder with a leading ~ made home.
func Expand(folder, home string) string {
	if folder == "~" {
		return home
	}
	if strings.HasPrefix(folder, "~/") {
		return filepath.Join(home, folder[2:])
	}
	return folder
}

// onDisk is an Outline file: rules by name, or, written before rules were
// shared, whole.
type onDisk struct {
	Name      string      `yaml:"name"`
	About     string      `yaml:"about,omitempty"`
	Folder    string      `yaml:"folder,omitempty"`
	Rules     []yaml.Node `yaml:"rules"`
	Snapshots []Mount     `yaml:"snapshots,omitempty"`
}

// named is how an Outline is written now.
type named struct {
	Name      string   `yaml:"name"`
	About     string   `yaml:"about,omitempty"`
	Folder    string   `yaml:"folder,omitempty"`
	Rules     []string `yaml:"rules"`
	Snapshots []Mount  `yaml:"snapshots,omitempty"`
}

// parse reads one Outline file, refusing keys it does not know: its rules
// found in rules by name, or held whole, when inline is set.
func parse(data []byte, rules map[string]view.View) (o Outline, inline bool, err error) {
	var d onDisk
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&d); err != nil {
		return Outline{}, false, err
	}
	o = Outline{Name: d.Name, About: d.About, Folder: d.Folder, Rules: []view.View{}, Snapshots: d.Snapshots}
	for _, n := range d.Rules {
		if n.Kind == yaml.ScalarNode {
			r, ok := rules[n.Value]
			if !ok {
				return Outline{}, false, fmt.Errorf("outline %s: no rule named %s in %s/", d.Name, n.Value, RulesDir)
			}
			o.Rules = append(o.Rules, r)
			continue
		}
		inline = true
		var r view.View
		if err := decodeStrict(&n, &r); err != nil {
			return Outline{}, false, err
		}
		o.Rules = append(o.Rules, withHead(r))
	}
	return o, inline, o.Validate()
}

func decodeStrict(n *yaml.Node, out any) error {
	data, err := yaml.Marshal(n)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	return decoder.Decode(out)
}

// withHead fills what a rule file may leave out, and writes its layout the
// way layouts are written now, so a rule saved before reads the same.
func withHead(r view.View) view.View {
	r, _ = view.Upgrade(r)
	if r.Selection == "" {
		r.Selection = view.Head
	}
	return r
}

// Parse reads one Outline file that holds its rules whole.
func Parse(data []byte) (Outline, error) {
	o, _, err := parse(data, nil)
	return o, err
}

// LoadRules reads every rules/*.yaml under root, by name. A file whose name
// is not its rule's name is refused.
func LoadRules(root string) (map[string]view.View, error) {
	paths, err := filepath.Glob(filepath.Join(root, RulesDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	out := map[string]view.View{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var r view.View
		decoder := yaml.NewDecoder(strings.NewReader(string(data)))
		decoder.KnownFields(true)
		if err := decoder.Decode(&r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		r = withHead(r)
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if name := strings.TrimSuffix(filepath.Base(path), ".yaml"); name != r.Name {
			return nil, fmt.Errorf("%s: name is %s, so the file should be %s.yaml", path, r.Name, r.Name)
		}
		out[r.Name] = r
	}
	return out, nil
}

// Rules is every rule under root, sorted by name, and the Outlines using
// each.
func Rules(root string) ([]view.View, map[string][]string, error) {
	rules, err := LoadRules(root)
	if err != nil {
		return nil, nil, err
	}
	outlines, err := Load(root)
	if err != nil {
		return nil, nil, err
	}
	used := map[string][]string{}
	for _, o := range outlines {
		for _, r := range o.Rules {
			used[r.Name] = append(used[r.Name], o.Name)
		}
	}
	out := make([]view.View, 0, len(rules))
	for _, r := range rules {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, used, nil
}

// Load reads every outlines/*.yaml under root, sorted by name, each with its
// rules. A file whose name is not its Outline's name is refused.
func Load(root string) ([]Outline, error) {
	outlines, _, err := load(root)
	return outlines, err
}

// load is Load, and the names of the Outlines holding their rules whole.
func load(root string) ([]Outline, map[string]bool, error) {
	rules, err := LoadRules(root)
	if err != nil {
		return nil, nil, err
	}
	paths, err := filepath.Glob(filepath.Join(root, Dir, "*.yaml"))
	if err != nil {
		return nil, nil, err
	}
	out := make([]Outline, 0, len(paths))
	whole := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		o, inline, err := parse(data, rules)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		if name := strings.TrimSuffix(filepath.Base(path), ".yaml"); name != o.Name {
			return nil, nil, fmt.Errorf("%s: name is %s, so the file should be %s.yaml", path, o.Name, o.Name)
		}
		whole[o.Name] = inline
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, whole, nil
}

// Save writes each of o's rules to rules/<name>.yaml, replacing a rule of
// that name — every Outline using it sees the change — then o to
// outlines/<name>.yaml naming them, once o is valid. Previous, when it names
// another Outline, is removed after. Each file is written through a
// temporary file and a rename.
func Save(root, previous string, o Outline) error {
	for i := range o.Rules {
		o.Rules[i] = withHead(o.Rules[i])
	}
	if o.Rules == nil {
		o.Rules = []view.View{}
	}
	if err := o.Validate(); err != nil {
		return err
	}
	file := named{Name: o.Name, About: o.About, Folder: o.Folder, Rules: []string{}, Snapshots: o.Snapshots}
	for _, r := range o.Rules {
		// An Outline names its rules and Snapshots together.
		if _, err := os.Stat(snapshot.Path(root, r.Name)); err == nil {
			return fmt.Errorf("a Snapshot is named %s: choose another name for the rule", r.Name)
		}
		data, err := yaml.Marshal(r)
		if err != nil {
			return err
		}
		if err := write(filepath.Join(root, RulesDir), r.Name+".yaml", data); err != nil {
			return err
		}
		file.Rules = append(file.Rules, r.Name)
	}
	data, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	if err := write(filepath.Join(root, Dir), o.Name+".yaml", data); err != nil {
		return err
	}
	if previous != "" && previous != o.Name {
		return Delete(root, previous)
	}
	return nil
}

// Fresh refuses any of names a rule under root already has: a rule made new
// in one Outline must not replace another Outline's.
func Fresh(root string, names []string) error {
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(root, RulesDir, n+".yaml")); err == nil {
			return fmt.Errorf("a rule is named %s already: add it as it is, or choose another name", n)
		}
	}
	return nil
}

// SaveRule writes r to rules/<name>.yaml, replacing the rule of that name:
// every Outline using it sees the change. With previous empty r is new and
// must not replace a rule; with previous another name, that rule is
// renamed first, in every Outline naming it.
func SaveRule(root, previous string, r view.View) error {
	r = withHead(r)
	if err := r.Validate(); err != nil {
		return errors.New(strings.Replace(err.Error(), "view ", "rule ", 1))
	}
	if _, err := os.Stat(snapshot.Path(root, r.Name)); err == nil {
		return fmt.Errorf("a Snapshot is named %s: choose another name for the rule", r.Name)
	}
	switch {
	case previous == "":
		if err := Fresh(root, []string{r.Name}); err != nil {
			return err
		}
	case previous != r.Name:
		if err := RenameRule(root, previous, r.Name); err != nil {
			return err
		}
	}
	data, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	return write(filepath.Join(root, RulesDir), r.Name+".yaml", data)
}

// RenameRule renames the rule from to to, in its file and in every Outline
// naming it.
func RenameRule(root, from, to string) error {
	if from == to {
		return nil
	}
	if err := (view.View{Name: to, Selection: view.Head, Layout: "x"}).Validate(); err != nil {
		return err
	}
	if err := Fresh(root, []string{to}); err != nil {
		return err
	}
	rules, err := LoadRules(root)
	if err != nil {
		return err
	}
	r, ok := rules[from]
	if !ok {
		return fmt.Errorf("no rule named %s", from)
	}
	outlines, err := Load(root)
	if err != nil {
		return err
	}
	r.Name = to
	data, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	if err := write(filepath.Join(root, RulesDir), to+".yaml", data); err != nil {
		return err
	}
	for _, o := range outlines {
		uses := false
		for i := range o.Rules {
			if o.Rules[i].Name == from {
				o.Rules[i].Name, uses = to, true
			}
		}
		if uses {
			if err := Save(root, "", o); err != nil {
				return err
			}
		}
	}
	return os.Remove(filepath.Join(root, RulesDir, from+".yaml"))
}

// DeleteRule removes a rule no Outline uses.
func DeleteRule(root, name string) error {
	if err := (view.View{Name: name, Selection: view.Head, Layout: "x"}).Validate(); err != nil {
		return err
	}
	_, used, err := Rules(root)
	if err != nil {
		return err
	}
	if len(used[name]) > 0 {
		return fmt.Errorf("the rule %s is used by %s", name, strings.Join(used[name], ", "))
	}
	err = os.Remove(filepath.Join(root, RulesDir, name+".yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no rule named %s", name)
	}
	return err
}

// write puts data at dir/name through a temporary file and a rename.
func write(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".outline-*.dgs-part")
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

// Delete removes an Outline's file. One that is not there is not an error.
func Delete(root, name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("outline %q: not an outline name", name)
	}
	err := os.Remove(filepath.Join(root, Dir, name+".yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Node is one folder: its name, the PDFs directly in it, and its folders.
type Node struct {
	Name string `json:"name"`
	// Snapshot is set on a Snapshot's folder: the Snapshot's name.
	Snapshot string `json:"snapshot,omitempty"`
	// Path is the folder's segments joined by /.
	Path string `json:"path"`
	// Count is every PDF in the folder and beneath it.
	Count int `json:"count"`
	// Files are the PDFs directly in the folder, each with its whole path.
	Files    []view.File `json:"files"`
	Children []*Node     `json:"children"`
}

// Grouping is an Outline's rules and Snapshots planned together, and the
// tree of the PDFs with a path of their own. What the tree lacks is in the
// plans' Missing and Clashes, in Clashes between rules and Snapshots, and
// in Lost, the Snapshots' files the tree no longer has, each at its path
// in the Outline.
type Grouping struct {
	view.Combined
	Lost []snapshot.Lost `json:"lost"`
	Root *Node           `json:"root"`
}

// Complete reports whether nothing stops the tree being exported.
func (g Grouping) Complete() bool { return g.Combined.Complete() && len(g.Lost) == 0 }

// Group plans o's rules together with its Snapshots, found in snapshots,
// and puts every placed PDF in its folder. A Snapshot's files are marked
// with its name, as a rule's are with the rule's. Folders and files sort by
// name, which keeps numbered ones in their order.
func Group(o Outline, snapshots []snapshot.Snapshot, items []tree.Item, names view.TypeNames) (Grouping, error) {
	fixed, lost, err := Mounted(o, snapshots, items)
	if err != nil {
		return Grouping{}, err
	}
	combined, err := view.CombineWith(o.Rules, fixed, items, names)
	if err != nil {
		return Grouping{}, err
	}
	root := Nest(combined.Files)
	for _, m := range o.Snapshots {
		if n := find(root, m.Folder()); n != nil {
			n.Snapshot = m.Name
		}
	}
	return Grouping{Combined: combined, Lost: lost, Root: root}, nil
}

// Mounted is o's Snapshots' files as items have them now, each at its path
// in the Outline, and those the tree has lost. A Snapshot missing from
// snapshots is an error.
func Mounted(o Outline, snapshots []snapshot.Snapshot, items []tree.Item) ([]view.File, []snapshot.Lost, error) {
	byName := map[string]snapshot.Snapshot{}
	for _, s := range snapshots {
		byName[s.Name] = s
	}
	files, lost := []view.File{}, []snapshot.Lost{}
	for _, m := range o.Snapshots {
		s, ok := byName[m.Name]
		if !ok {
			return nil, nil, fmt.Errorf("outline %s: no Snapshot named %s in %s/", o.Name, m.Name, snapshot.Dir)
		}
		r := snapshot.Resolve(s, items)
		for _, f := range r.Files {
			f.Path = m.Folder() + "/" + f.Path
			files = append(files, f)
		}
		for _, l := range r.Lost {
			l.Path = m.Folder() + "/" + l.Path
			lost = append(lost, l)
		}
	}
	return files, lost, nil
}

// find is the folder at p under n, or nil.
func find(n *Node, p string) *Node {
	for _, c := range n.Children {
		if strings.EqualFold(c.Path, p) {
			return c
		}
		if strings.HasPrefix(strings.ToLower(p), strings.ToLower(c.Path)+"/") {
			return find(c, p)
		}
	}
	return nil
}

// SnapshotUsers is the Outlines using each Snapshot, by its name.
func SnapshotUsers(outlines []Outline) map[string][]string {
	used := map[string][]string{}
	for _, o := range outlines {
		for _, m := range o.Snapshots {
			used[m.Name] = append(used[m.Name], o.Name)
		}
	}
	return used
}

// RenameSnapshot renames the Snapshot from to to, in its file and in
// every Outline putting it in its tree.
func RenameSnapshot(root, from, to string) error {
	outlines, err := Load(root)
	if err != nil {
		return err
	}
	list, err := snapshot.Load(root)
	if err != nil {
		return err
	}
	var s *snapshot.Snapshot
	for i := range list {
		if list[i].Name == from {
			s = &list[i]
		}
	}
	if s == nil {
		return fmt.Errorf("no Snapshot named %s", from)
	}
	for _, o := range outlines {
		for _, r := range o.Rules {
			if r.Name == to {
				return fmt.Errorf("the Outline %s has a rule named %s", o.Name, to)
			}
		}
	}
	s.Name = to
	if err := snapshot.Save(root, from, *s, false); err != nil {
		return err
	}
	for _, o := range outlines {
		uses := false
		for i := range o.Snapshots {
			if o.Snapshots[i].Name == from {
				o.Snapshots[i].Name, uses = to, true
			}
		}
		if uses {
			if err := Save(root, "", o); err != nil {
				return err
			}
		}
	}
	return nil
}

// Nest puts files, each at a path of its own, into folders.
func Nest(files []view.File) *Node {
	root := &Node{Files: []view.File{}, Children: []*Node{}}
	for _, f := range files {
		segments := strings.Split(f.Path, "/")
		node := root
		node.Count++
		for i, s := range segments[:len(segments)-1] {
			node = child(node, s, strings.Join(segments[:i+1], "/"))
			node.Count++
		}
		node.Files = append(node.Files, f)
	}
	tidy(root)
	return root
}

func child(n *Node, name, path string) *Node {
	for _, c := range n.Children {
		if strings.EqualFold(c.Name, name) {
			return c
		}
	}
	c := &Node{Name: name, Path: path, Files: []view.File{}, Children: []*Node{}}
	n.Children = append(n.Children, c)
	return c
}

func tidy(n *Node) {
	sort.Slice(n.Children, func(i, j int) bool { return n.Children[i].Name < n.Children[j].Name })
	sort.Slice(n.Files, func(i, j int) bool { return n.Files[i].Path < n.Files[j].Path })
	for _, c := range n.Children {
		tidy(c)
	}
}
