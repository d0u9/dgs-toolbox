// Package outline is a doc tree's Outlines and the rules they use. An
// Outline is a tree of PDFs made by its rules: each rule selects Items and
// names the path each of their PDFs has, and the rules' paths together are
// the tree. The same tree is browsed and, into a folder chosen then,
// exported. A rule is kept apart from the Outlines, under rules/, so several
// Outlines use one. The rules are in docs/apps/doc/index.md.
package outline

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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
	Name   string      `yaml:"name"`
	About  string      `yaml:"about,omitempty"`
	Folder string      `yaml:"folder,omitempty"`
	Rules  []yaml.Node `yaml:"rules"`
}

// named is how an Outline is written now.
type named struct {
	Name   string   `yaml:"name"`
	About  string   `yaml:"about,omitempty"`
	Folder string   `yaml:"folder,omitempty"`
	Rules  []string `yaml:"rules"`
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
	o = Outline{Name: d.Name, About: d.About, Folder: d.Folder, Rules: []view.View{}}
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

func withHead(r view.View) view.View {
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
	// A Snapshot shares the Outlines' names, exported and remembered by
	// them; package snapshot keeps its files in snapshots/.
	if _, err := os.Stat(filepath.Join(root, "snapshots", o.Name+".yaml")); err == nil {
		return fmt.Errorf("a Snapshot is named %s: choose another name", o.Name)
	}
	file := named{Name: o.Name, About: o.About, Folder: o.Folder, Rules: []string{}}
	for _, r := range o.Rules {
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
	// Path is the folder's segments joined by /.
	Path string `json:"path"`
	// Count is every PDF in the folder and beneath it.
	Count int `json:"count"`
	// Files are the PDFs directly in the folder, each with its whole path.
	Files    []view.File `json:"files"`
	Children []*Node     `json:"children"`
}

// Grouping is an Outline's rules planned together, and the tree of the PDFs
// with a path of their own. What the tree lacks is in the plans' Missing and
// Clashes, and in Clashes between rules.
type Grouping struct {
	view.Combined
	Root *Node `json:"root"`
}

// Group plans o's rules together and puts every placed PDF in its folder.
// Folders and files sort by name, which keeps numbered ones in their order.
func Group(o Outline, items []tree.Item, names view.TypeNames) (Grouping, error) {
	combined, err := view.Combine(o.Rules, items, names)
	if err != nil {
		return Grouping{}, err
	}
	return Grouping{Combined: combined, Root: Nest(combined.Files)}, nil
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
