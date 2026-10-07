package outline

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/doc/snapshot"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
	"gopkg.in/yaml.v3"
)

type Assembly struct {
	Rule     string `yaml:"rule,omitempty" json:"rule,omitempty"`
	Snapshot string `yaml:"snapshot,omitempty" json:"snapshot,omitempty"`
	At       string `yaml:"at,omitempty" json:"at,omitempty"`
	As       string `yaml:"as,omitempty" json:"as,omitempty"`
}

type skeletonFile struct {
	Name     string              `yaml:"name"`
	About    string              `yaml:"about,omitempty"`
	Folder   string              `yaml:"folder,omitempty"`
	Defaults view.Defaults       `yaml:"defaults,omitempty"`
	Mount    []Assembly          `yaml:"mount"`
	Pins     map[string]view.Pin `yaml:"pins,omitempty"`
}

func inside(p string) bool {
	return p != "" && !path.IsAbs(p) && p == path.Clean(p) && p != ".." && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\\\x00")
}

func (o Outline) validateSkeleton() error {
	base := Outline{Name: o.Name, About: o.About, Folder: o.Folder}
	if err := base.Validate(); err != nil {
		return err
	}
	if err := o.Defaults.Validate(); err != nil {
		return err
	}
	for _, m := range o.Mount {
		if (m.Rule == "") == (m.Snapshot == "") {
			return fmt.Errorf("mount needs one rule or snapshot")
		}
		name := m.Rule
		if name == "" {
			name = m.Snapshot
		}
		if !namePattern.MatchString(name) {
			return fmt.Errorf("invalid mount name")
		}
		if m.At != "" && !inside(m.At) {
			return fmt.Errorf("mount path is not inside the Outline")
		}
		if m.As != "" && (!inside(m.As) || strings.Contains(m.As, "/")) {
			return fmt.Errorf("mount as must be one folder name")
		}
	}
	for folder, pin := range o.Pins {
		if !inside(folder) {
			return fmt.Errorf("pin path is not inside the Outline")
		}
		used := map[int]bool{}
		for _, n := range pin.Numbers {
			if n < 1 || used[n] {
				return fmt.Errorf("pinned numbers must be positive and unique")
			}
			used[n] = true
		}
	}
	for _, r := range o.Rules {
		if r.Folders == nil {
			return fmt.Errorf("doc 1 rules are not read by folder skeleton Outlines")
		}
		if err := r.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func parseFor(st *tree.Store, data []byte, rules map[string]view.View) (Outline, bool, error) {
	if st.OutlineVersion() != 2 {
		return parse(data, rules)
	}
	var file skeletonFile
	d := yaml.NewDecoder(strings.NewReader(string(data)))
	d.KnownFields(true)
	if err := d.Decode(&file); err != nil {
		return Outline{}, false, err
	}
	if file.Mount == nil {
		return Outline{}, false, fmt.Errorf("doc 2 Outline needs mount; doc 1 Outlines are not read here")
	}
	o := Outline{Name: file.Name, About: file.About, Folder: file.Folder, Defaults: file.Defaults, Mount: file.Mount, Pins: file.Pins}
	seen := map[string]bool{}
	for _, m := range file.Mount {
		if m.Rule != "" && !seen[m.Rule] {
			r, ok := rules[m.Rule]
			if !ok {
				return o, false, fmt.Errorf("no rule named %s", m.Rule)
			}
			o.Rules = append(o.Rules, r)
			seen[m.Rule] = true
		}
	}
	return o, false, o.Validate()
}

func saveSkeleton(st *tree.Store, previous string, o Outline, now time.Time) error {
	if o.Mount == nil {
		return fmt.Errorf("doc 2 Outline needs mount")
	}
	if err := o.Validate(); err != nil {
		return err
	}
	rules, err := LoadRules(st)
	if err != nil {
		return err
	}
	for _, m := range o.Mount {
		if m.Rule != "" {
			if _, ok := rules[m.Rule]; !ok {
				return fmt.Errorf("no rule named %s", m.Rule)
			}
		}
	}
	file := skeletonFile{Name: o.Name, About: o.About, Folder: o.Folder, Defaults: o.Defaults, Mount: o.Mount, Pins: o.Pins}
	return tree.SaveDefinition(st, tree.DefinitionKind[skeletonFile]{Dir: Dir, Name: func(o skeletonFile) string { return o.Name }}, previous, file, now)
}

func groupSkeleton(o Outline, snapshots []snapshot.Snapshot, items []tree.Item, names view.Types, now time.Time) (Grouping, error) {
	out := Grouping{Combined: view.Combined{Plans: map[string]view.Plan{}, Files: []view.File{}, Clashes: []view.Clash{}}, Lost: []snapshot.Lost{}}
	if err := o.Validate(); err != nil {
		return out, err
	}
	rules := map[string]view.View{}
	for _, r := range o.Rules {
		rules[r.Name] = r
	}
	fixed := map[string]snapshot.Snapshot{}
	for _, s := range snapshots {
		fixed[s.Name] = s
	}
	assigned := map[string]bool{}
	folderNumbers := map[string]map[int]string{}
	var folders []string
	for index, m := range o.Mount {
		label := m.Rule
		if label == "" {
			label = m.Snapshot
		}
		planName := fmt.Sprintf("%02d-%s", index+1, label)
		plan := view.Plan{Files: []view.File{}, Missing: []view.Missing{}, Clashes: []view.Clash{}}
		if m.Rule != "" {
			r, ok := rules[m.Rule]
			if !ok {
				return out, fmt.Errorf("no rule named %s", m.Rule)
			}
			at := m.At
			if m.As != "" {
				at = path.Join(at, m.As)
			}
			defaults := r.Defaults
			if defaults.Revision == "" {
				defaults.Revision = o.Defaults.Revision
			}
			if defaults.Valid == "" {
				defaults.Valid = o.Defaults.Valid
			}
			if defaults.File == "" {
				defaults.File = o.Defaults.File
			}
			built, err := view.BuildFolders(r.Folders, items, names, defaults, o.Pins, at, assigned, folderNumbers, now)
			if err != nil {
				return out, err
			}
			plan = built.Plan
			folders = append(folders, built.Folders...)
		} else {
			s, ok := fixed[m.Snapshot]
			if !ok {
				return out, fmt.Errorf("no Snapshot named %s", m.Snapshot)
			}
			resolved := snapshot.Resolve(s, items)
			folder := m.As
			if folder == "" {
				folder = s.Name
			}
			folder = path.Join(m.At, folder)
			folders = append(folders, folder)
			for _, f := range resolved.Files {
				target := ""
				for p, pin := range o.Pins {
					if slices.Contains(pin.Items, f.Item) {
						target = p
					}
				}
				if assigned[f.Item] || target != "" && target != folder {
					continue
				}
				f.Path = path.Join(folder, f.Path)
				f.Node = folder
				plan.Files = append(plan.Files, f)
			}
			for _, f := range plan.Files {
				assigned[f.Item] = true
			}
			for _, l := range resolved.Lost {
				l.Path = path.Join(folder, l.Path)
				out.Lost = append(out.Lost, l)
			}
		}
		for i := range plan.Files {
			plan.Files[i].View = planName
		}
		out.Plans[planName] = plan
		out.Files = append(out.Files, plan.Files...)
	}
	for _, folder := range folders {
		for _, f := range out.Files {
			if strings.EqualFold(folder, f.Path) {
				return out, fmt.Errorf("folder %s conflicts with a file", folder)
			}
		}
	}
	// Reuse the existing file/folder collision check for the assembled files.
	combined, err := view.CombineWith(nil, out.Files, items, names)
	if err != nil {
		return out, err
	}
	out.Files, out.Clashes = combined.Files, combined.Clashes
	out.Root = Nest(out.Files)
	for _, folder := range folders {
		// Empty skeleton folders must remain valid pin/drop targets.
		current := out.Root
		prefix := ""
		for _, name := range strings.Split(folder, "/") {
			if name == "" || name == "." {
				continue
			}
			prefix = path.Join(prefix, name)
			var next *Node
			for _, c := range current.Children {
				if c.Name == name {
					next = c
					break
				}
			}
			if next == nil {
				next = &Node{Name: name, Path: prefix, Files: []view.File{}, Children: []*Node{}}
				current.Children = append(current.Children, next)
			}
			current = next
		}
	}
	var order func(*Node)
	order = func(n *Node) {
		sort.Slice(n.Children, func(i, j int) bool { return n.Children[i].Name < n.Children[j].Name })
		for _, c := range n.Children {
			order(c)
		}
	}
	order(out.Root)
	for _, item := range items {
		if !assigned[item.ID] {
			out.Unplaced = append(out.Unplaced, item.ID)
		}
	}
	for folder := range o.Pins {
		if find(out.Root, folder) == nil {
			return out, fmt.Errorf("pin names a folder the skeleton does not make: %s", folder)
		}
	}
	return out, nil
}
