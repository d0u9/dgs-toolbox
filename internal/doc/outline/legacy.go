package outline

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"dgs-toolbox/internal/doc/view"

	"gopkg.in/yaml.v3"
)

// Before Outlines, a tree kept Views in views/*.yaml, each naming the
// Target it was exported to, and the Targets in targets.yaml. Those files
// are read only to become Outlines.
const (
	LegacyViews   = "views"
	LegacyTargets = "targets.yaml"
	// MigratedDir is where Migrate moves them once their Outlines are
	// written, so nothing is deleted.
	MigratedDir = "migrated"
)

type legacyView struct {
	view.View `yaml:",inline"`
	Target    string `yaml:"target,omitempty"`
}

type legacyTarget struct {
	About  string `yaml:"about,omitempty"`
	Folder string `yaml:"folder,omitempty"`
}

// Legacy is the Outlines a tree's Views and Targets make, sorted by name,
// written nowhere: each Target is an Outline whose rules are the Views
// naming it, and each View naming none is an Outline of that one rule.
func Legacy(root string) ([]Outline, error) {
	var targets map[string]legacyTarget
	path := filepath.Join(root, LegacyTargets)
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&targets); err != nil && err.Error() != "EOF" {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	byName := map[string]*Outline{}
	for name, t := range targets {
		byName[name] = &Outline{Name: name, About: t.About, Folder: t.Folder, Rules: []view.View{}}
	}
	paths, err := filepath.Glob(filepath.Join(root, LegacyViews, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var loose []view.View
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var v legacyView
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&v); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if v.Target == "" {
			loose = append(loose, v.View)
			continue
		}
		o := byName[v.Target]
		if o == nil {
			o = &Outline{Name: v.Target, Rules: []view.View{}}
			byName[v.Target] = o
		}
		o.Rules = append(o.Rules, v.View)
	}
	for _, v := range loose {
		name := v.Name
		for byName[name] != nil {
			name += "-view"
		}
		byName[name] = &Outline{Name: name, Rules: []view.View{v}}
	}
	out := make([]Outline, 0, len(byName))
	for _, o := range byName {
		if err := o.Validate(); err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Migrate brings a tree's Outlines up to date, naming those it wrote:
//
//   - Views and Targets become the Outlines Legacy makes, and views/ and
//     targets.yaml move into migrated/. An Outline or a rule already saved
//     under one of those names stops it before anything is written.
//   - An Outline holding its rules whole has them moved under rules/. A rule
//     whose name another Outline's different rule has is renamed
//     <outline>-<rule>; one the same is shared.
func Migrate(root string) ([]string, error) {
	names, err := migrateLegacy(root)
	if err != nil {
		return names, err
	}
	outlines, whole, err := load(root)
	if err != nil {
		return names, err
	}
	for _, o := range outlines {
		if !whole[o.Name] {
			continue
		}
		rules, err := LoadRules(root)
		if err != nil {
			return names, err
		}
		for i, r := range o.Rules {
			name := r.Name
			for {
				have, ok := rules[name]
				if !ok || same(have, r) {
					break
				}
				name = o.Name + "-" + name
			}
			o.Rules[i].Name = name
		}
		if err := Save(root, "", o); err != nil {
			return names, err
		}
		names = append(names, o.Name)
	}
	return names, nil
}

func same(a, b view.View) bool {
	x, _ := yaml.Marshal(withHead(a))
	y, _ := yaml.Marshal(withHead(b))
	return bytes.Equal(x, y)
}

func migrateLegacy(root string) ([]string, error) {
	outlines, err := Legacy(root)
	if err != nil || len(outlines) == 0 {
		return nil, err
	}
	saved, err := Load(root)
	if err != nil {
		return nil, err
	}
	for _, o := range outlines {
		for _, s := range saved {
			if s.Name == o.Name {
				return nil, fmt.Errorf("the View or Target %s would become an Outline, and outlines/%s.yaml is there already: rename one", o.Name, o.Name)
			}
		}
		for _, r := range o.Rules {
			if err := Fresh(root, []string{r.Name}); err != nil {
				return nil, fmt.Errorf("the View %s would become a rule: %w", r.Name, err)
			}
		}
	}
	var names []string
	for _, o := range outlines {
		if err := Save(root, "", o); err != nil {
			return names, err
		}
		names = append(names, o.Name)
	}
	moved := filepath.Join(root, MigratedDir)
	if err := os.MkdirAll(moved, 0o755); err != nil {
		return names, err
	}
	for _, name := range []string{LegacyViews, LegacyTargets} {
		from := filepath.Join(root, name)
		if _, err := os.Stat(from); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		to := filepath.Join(moved, name)
		if _, err := os.Stat(to); err == nil {
			return names, fmt.Errorf("%s is there already: move it away, then open the tree again", to)
		}
		if err := os.Rename(from, to); err != nil {
			return names, err
		}
	}
	return names, nil
}
