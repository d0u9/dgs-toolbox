// Package outline is a doc tree's Outlines: a browsing tree of folders an
// Outline's layout groups its Items into, each folder counting the PDFs
// beneath it. It shares a View's query, selection, layout and numbering but
// is never exported. The rules are in docs/apps/doc/index.md.
package outline

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"

	"gopkg.in/yaml.v3"
)

// Dir is the folder under a tree's root that holds one file per Outline.
const Dir = "outlines"

// Outline is one file under outlines/. Its layout names folders only: the
// PDFs are listed in the folder they land in, never named.
type Outline struct {
	Name      string                 `yaml:"name" json:"name"`
	Query     map[string]view.Values `yaml:"query,omitempty" json:"query"`
	Selection view.Selection         `yaml:"selection" json:"selection"`
	Layout    string                 `yaml:"layout" json:"layout"`
	Order     map[string][]string    `yaml:"order,omitempty" json:"order,omitempty"`
}

// fileKey names each PDF inside its folder while planning; it is dropped
// from the path, so the layout's segments are all folders.
const fileKey = "{id}-{revision}"

// asView is o as a View whose last segment names each PDF uniquely.
func (o Outline) asView() view.View {
	return view.View{Name: o.Name, Query: o.Query, Selection: o.Selection, Layout: o.Layout + "/" + fileKey, Order: o.Order}
}

// Validate reports the first thing wrong with o.
func (o Outline) Validate() error {
	if o.Selection == "" {
		o.Selection = view.Head
	}
	err := o.asView().Validate()
	if err != nil {
		return errors.New(strings.Replace(err.Error(), "view ", "outline ", 1))
	}
	return nil
}

// Parse reads one Outline file, refusing keys it does not know.
func Parse(data []byte) (Outline, error) {
	var o Outline
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&o); err != nil {
		return Outline{}, err
	}
	if o.Selection == "" {
		o.Selection = view.Head
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
	if o.Selection == "" {
		o.Selection = view.Head
	}
	if err := o.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(o)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, Dir)
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
	if err := os.Rename(temp.Name(), filepath.Join(dir, o.Name+".yaml")); err != nil {
		return err
	}
	if previous != "" && previous != o.Name {
		return Delete(root, previous)
	}
	return nil
}

// Delete removes an Outline's file. One that is not there is not an error.
func Delete(root, name string) error {
	if strings.ContainsAny(name, `/\`) || name == "" || name == "." || name == ".." {
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
	Count    int         `json:"count"`
	Files    []view.File `json:"files"`
	Children []*Node     `json:"children"`
}

// Grouping is an Outline's folders, and the PDFs it selects but cannot place.
type Grouping struct {
	Root    *Node          `json:"root"`
	Missing []view.Missing `json:"missing"`
}

// Group places every PDF o selects in its folder. Folders sort by name,
// which keeps numbered folders in their order; PDFs sort by Item ID and
// revision.
func Group(o Outline, items []tree.Item, names view.TypeNames) (Grouping, error) {
	if o.Selection == "" {
		o.Selection = view.Head
	}
	plan, err := view.Build(o.asView(), items, names)
	if err != nil {
		return Grouping{}, err
	}
	root := &Node{Files: []view.File{}, Children: []*Node{}}
	files := append([]view.File(nil), plan.Files...)
	// Clashes cannot happen, as each PDF's last segment is its own; a case
	// fold of two folders still lands both in the first one's spelling.
	for _, c := range plan.Clashes {
		files = append(files, c.Files...)
	}
	for _, f := range files {
		segments := strings.Split(f.Path, "/")
		node := root
		node.Count++
		for i, s := range segments[:len(segments)-1] {
			node = child(node, s, strings.Join(segments[:i+1], "/"))
			node.Count++
		}
		f.Path = strings.Join(segments[:len(segments)-1], "/")
		node.Files = append(node.Files, f)
	}
	tidy(root)
	return Grouping{Root: root, Missing: plan.Missing}, nil
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
	sort.Slice(n.Files, func(i, j int) bool {
		a, b := n.Files[i], n.Files[j]
		if a.Item != b.Item {
			return a.Item < b.Item
		}
		return a.Revision < b.Revision
	})
	for _, c := range n.Children {
		tidy(c)
	}
}
