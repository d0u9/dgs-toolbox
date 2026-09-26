// Package outline is a doc tree's Outlines. An Outline is a tree of PDFs
// made by its rules: each rule selects Items and names the path each of
// their PDFs has, and the rules' paths together are the tree. The same tree
// is browsed and, into a folder chosen then, exported. The rules are in
// docs/apps/doc/index.md.
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

// Dir is the folder under a tree's root that holds one file per Outline.
const Dir = "outlines"

// Outline is one file under outlines/.
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

// Parse reads one Outline file, refusing keys it does not know.
func Parse(data []byte) (Outline, error) {
	var o Outline
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&o); err != nil {
		return Outline{}, err
	}
	for i := range o.Rules {
		if o.Rules[i].Selection == "" {
			o.Rules[i].Selection = view.Head
		}
	}
	return o, o.Validate()
}

// Load reads every outlines/*.yaml under root, sorted by name. A file whose
// name is not its Outline's name is refused.
func Load(root string) ([]Outline, error) {
	paths, err := filepath.Glob(filepath.Join(root, Dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	out := make([]Outline, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		o, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if name := strings.TrimSuffix(filepath.Base(path), ".yaml"); name != o.Name {
			return nil, fmt.Errorf("%s: name is %s, so the file should be %s.yaml", path, o.Name, o.Name)
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Save writes o to outlines/<name>.yaml through a temporary file and a
// rename, once it is valid. Previous, when it names another Outline, is
// removed after.
func Save(root, previous string, o Outline) error {
	for i := range o.Rules {
		if o.Rules[i].Selection == "" {
			o.Rules[i].Selection = view.Head
		}
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
	data, err := yaml.Marshal(o)
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
