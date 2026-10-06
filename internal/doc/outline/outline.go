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
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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
	// A rule and a Snapshot may share a name: each is named once among
	// its own kind.
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
	seen = map[string]bool{}
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
			return fmt.Errorf("outline %s: two Snapshots are named %s", o.Name, m.Name)
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

// withHead fills what a rule file may leave out, and names its orders as
// they are named now.
func withHead(r view.View) view.View {
	if r.Selection == "" {
		r.Selection = view.Head
	}
	return r.Upgrade()
}

// Parse reads one Outline file that holds its rules whole.
func Parse(data []byte) (Outline, error) {
	o, _, err := parse(data, nil)
	return o, err
}

// LoadRules reads every rules/*.yaml under root, by name. A file whose name
// is not its rule's name is refused.
func LoadRules(st *tree.Store) (map[string]view.View, error) {
	rules, err := tree.ListDefinitions(st, ruleKind())
	if err != nil {
		return nil, err
	}
	out := map[string]view.View{}
	for _, r := range rules {
		out[r.Name] = r
	}
	return out, nil
}

func ruleKind() tree.DefinitionKind[view.View] {
	return tree.DefinitionKind[view.View]{Dir: RulesDir, Name: func(r view.View) string { return r.Name }, Validate: func(_ *tree.Store, r view.View) error { return r.Validate() }, Decode: func(_ *tree.Store, data []byte) (view.View, error) {
		var r view.View
		d := yaml.NewDecoder(strings.NewReader(string(data)))
		d.KnownFields(true)
		if err := d.Decode(&r); err != nil {
			return r, err
		}
		return withHead(r), nil
	}}
}

// ReadRule reads one rule file: it must parse, be a valid rule, and be named
// for its rule.
func ReadRule(st *tree.Store, path string) (view.View, error) {
	return tree.ReadDefinitionFile(st, ruleKind(), path)
}

// Rules is every rule under root, sorted by name, and the Outlines using
// each.
func Rules(st *tree.Store) ([]view.View, map[string][]string, error) {
	rules, err := LoadRules(st)
	if err != nil {
		return nil, nil, err
	}
	outlines, err := Load(st)
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
func Load(st *tree.Store) ([]Outline, error) {
	outlines, _, err := load(st)
	return outlines, err
}

// load is Load, and the names of the Outlines holding their rules whole.
func load(st *tree.Store) ([]Outline, map[string]bool, error) {
	rules, err := LoadRules(st)
	if err != nil {
		return nil, nil, err
	}
	whole := map[string]bool{}
	out, err := tree.ListDefinitions(st, tree.DefinitionKind[Outline]{Dir: Dir, Name: func(o Outline) string { return o.Name }, Decode: func(_ *tree.Store, data []byte) (Outline, error) {
		o, inline, err := parse(data, rules)
		whole[o.Name] = inline
		return o, err
	}})
	return out, whole, err
}

// ReadFile reads one Outline file, its rules named from rules: it must
// parse, name only rules there, be valid, and be named for its Outline.
func ReadFile(st *tree.Store, path string, rules map[string]view.View) (Outline, error) {
	o, _, err := readFile(st, path, rules)
	return o, err
}

func readFile(st *tree.Store, path string, rules map[string]view.View) (Outline, bool, error) {
	var inline bool
	k := tree.DefinitionKind[Outline]{Dir: Dir, Name: func(o Outline) string { return o.Name }, Decode: func(_ *tree.Store, data []byte) (Outline, error) {
		o, whole, err := parse(data, rules)
		inline = whole
		return o, err
	}}
	o, err := tree.ReadDefinitionFile(st, k, path)
	return o, inline, err
}

// Save writes each of o's rules to rules/<name>.yaml, replacing a rule of
// that name — every Outline using it sees the change — then o to
// outlines/<name>.yaml naming them, once o is valid. Previous, when it names
// another Outline, is moved to the trash after. Each file is written through a
// temporary file and a rename.
func Save(st *tree.Store, previous string, o Outline, now time.Time) error {
	for i := range o.Rules {
		o.Rules[i] = withHead(o.Rules[i])
	}
	if o.Rules == nil {
		o.Rules = []view.View{}
	}
	if err := o.Validate(); err != nil {
		return err
	}
	if err := st.CheckDefinitionSave(Dir, previous, o.Name); err != nil {
		return err
	}
	file := named{Name: o.Name, About: o.About, Folder: o.Folder, Rules: []string{}, Snapshots: o.Snapshots}
	for _, r := range o.Rules {
		data, err := yaml.Marshal(r)
		if err != nil {
			return err
		}
		if err := st.WriteDefinition(RulesDir, r.Name, data); err != nil {
			return err
		}
		file.Rules = append(file.Rules, r.Name)
	}
	return tree.SaveDefinition(st, tree.DefinitionKind[named]{Dir: Dir, Name: func(o named) string { return o.Name }}, previous, file, now)
}

// Fresh refuses any of names a rule under root already has: a rule made new
// in one Outline must not replace another Outline's.
func Fresh(st *tree.Store, names []string) error {
	for _, n := range names {
		exists, err := st.DefinitionExists(RulesDir, n)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("a rule is named %s already: add it as it is, or choose another name", n)
		}
	}
	return nil
}

// SaveRule writes r to rules/<name>.yaml, replacing the rule of that name:
// every Outline using it sees the change. With previous empty r is new and
// must not replace a rule; with previous another name, that rule is
// renamed first, in every Outline naming it.
func SaveRule(st *tree.Store, previous string, r view.View, now time.Time) error {
	r = withHead(r)
	if err := r.Validate(); err != nil {
		return errors.New(strings.Replace(err.Error(), "view ", "rule ", 1))
	}
	switch {
	case previous == "":
		if err := Fresh(st, []string{r.Name}); err != nil {
			return err
		}
	case previous != r.Name:
		if err := RenameRule(st, previous, r.Name, now); err != nil {
			return err
		}
	}
	if previous != "" {
		previous = r.Name
	}
	return tree.SaveDefinition(st, ruleKind(), previous, r, now)
}

// RenameRule renames the rule from to to, in its file and in every Outline
// naming it.
func RenameRule(st *tree.Store, from string, to string, now time.Time) error {
	if from == to {
		return nil
	}
	if err := (view.View{Name: to, Node: view.Node{Selection: view.Head, File: "x"}}).Validate(); err != nil {
		return err
	}
	if err := Fresh(st, []string{to}); err != nil {
		return err
	}
	rules, err := LoadRules(st)
	if err != nil {
		return err
	}
	r, ok := rules[from]
	if !ok {
		return fmt.Errorf("no rule named %s", from)
	}
	outlines, err := Load(st)
	if err != nil {
		return err
	}
	r.Name = to
	if err := tree.SaveDefinition(st, ruleKind(), "", r, now); err != nil {
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
			if err := Save(st, o.Name, o, now); err != nil {
				return err
			}
		}
	}
	_, err = TrashRule(st, from, now)
	return err
}

// TrashRule moves a rule no Outline uses into the tree's trash.
func TrashRule(st *tree.Store, name string, now time.Time) (string, error) {
	if err := (view.View{Name: name, Node: view.Node{Selection: view.Head, File: "x"}}).Validate(); err != nil {
		return "", err
	}
	_, used, err := Rules(st)
	if err != nil {
		return "", err
	}
	if len(used[name]) > 0 {
		return "", fmt.Errorf("the rule %s is used by %s", name, strings.Join(used[name], ", "))
	}
	exists, err := st.DefinitionExists(RulesDir, name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("no rule named %s", name)
	}
	return st.TrashDefinition(RulesDir, name, now)
}

// Trash moves an Outline's file into the tree's trash and answers where it
// went. One that is not there is not an error and answers an empty path.
func Trash(st *tree.Store, name string, now time.Time) (string, error) {
	if !namePattern.MatchString(name) {
		return "", fmt.Errorf("outline %q: not an outline name", name)
	}
	exists, err := st.DefinitionExists(Dir, name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	return st.TrashDefinition(Dir, name, now)
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
func Group(o Outline, snapshots []snapshot.Snapshot, items []tree.Item, names view.Types) (Grouping, error) {
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

// Explain is how o places each PDF of the Item id, or why it does not: by
// each rule, planned with its Snapshots as Group plans them, and by each
// Snapshot holding it.
func Explain(o Outline, snapshots []snapshot.Snapshot, items []tree.Item, names view.Types, id string) ([]view.Explanation, error) {
	fixed, _, err := Mounted(o, snapshots, items)
	if err != nil {
		return nil, err
	}
	return view.ExplainWith(o.Rules, fixed, items, names, id)
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
func RenameSnapshot(st *tree.Store, from string, to string, now time.Time) error {
	outlines, err := Load(st)
	if err != nil {
		return err
	}
	list, err := snapshot.Load(st)
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
	s.Name = to
	if err := snapshot.Save(st, from, *s, false, now); err != nil {
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
			if err := Save(st, o.Name, o, now); err != nil {
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
