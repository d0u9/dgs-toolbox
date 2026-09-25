// Package view is a doc tree's Views: which Items a View selects, and the
// path each selected PDF would have in an export. It computes a plan and
// writes nothing but View files; the rules are in docs/apps/doc/index.md.
package view

import (
	"dgs-toolbox/internal/doc/country"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Dir is the folder under a tree's root that holds one file per View.
const Dir = "views"

// Selection is which revisions of a document a View takes.
type Selection string

const (
	// Head takes a document's HEAD only, and a record's one PDF.
	Head Selection = "head"
	// All takes every revision.
	All Selection = "all"
)

// DedupeNumber numbers PDFs that would land on one path, instead of refusing.
const DedupeNumber = "number"

// Values is one query condition's accepted values. In YAML it is one value or
// a list.
type Values []string

// UnmarshalYAML reads a scalar or a sequence.
func (v *Values) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*v = Values{node.Value}
		return nil
	}
	var list []string
	if err := node.Decode(&list); err != nil {
		return err
	}
	*v = list
	return nil
}

// MarshalYAML writes one value as a scalar.
func (v Values) MarshalYAML() (any, error) {
	if len(v) == 1 {
		return v[0], nil
	}
	return []string(v), nil
}

// View is one file under views/.
type View struct {
	Name string `yaml:"name" json:"name"`
	// Query holds, per key, the values an Item's key may have. An Item must
	// match every key. `type` is the Item's type.
	Query     map[string]Values `yaml:"query,omitempty" json:"query"`
	Selection Selection         `yaml:"selection" json:"selection"`
	Layout    string            `yaml:"layout" json:"layout"`
	// Default, when set, is written for a key an Item lacks.
	Default *string `yaml:"default,omitempty" json:"default,omitempty"`
	// Dedupe is empty (refuse) or DedupeNumber.
	Dedupe string `yaml:"dedupe,omitempty" json:"dedupe,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate reports the first thing wrong with v.
func (v View) Validate() error {
	if !namePattern.MatchString(v.Name) {
		return fmt.Errorf("view %q: use lowercase letters, digits, _ and -", v.Name)
	}
	if v.Selection != Head && v.Selection != All {
		return fmt.Errorf("view %s: selection %q is neither head nor all", v.Name, v.Selection)
	}
	if v.Dedupe != "" && v.Dedupe != DedupeNumber {
		return fmt.Errorf("view %s: dedupe %q: leave it out, or use number", v.Name, v.Dedupe)
	}
	if _, err := Parse(v.Layout); err != nil {
		return fmt.Errorf("view %s: %w", v.Name, err)
	}
	return nil
}

// Part is a piece of a layout: fixed text, or a key. A key written
// {key:format} is a country written in that format — zh, en, alpha2 or
// alpha3 — whatever form the Item keeps it in.
type Part struct {
	Text   string `json:"text,omitempty"`
	Key    string `json:"key,omitempty"`
	Format string `json:"format,omitempty"`
}

// Layout is a parsed layout, one list of parts per path segment.
type Layout [][]Part

var keyPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// Parse splits a layout into segments and parts. A layout is relative: no
// leading /, no empty segment, and no segment that is only . or .., so a path
// it makes stays inside the Target.
func Parse(layout string) (Layout, error) {
	if strings.TrimSpace(layout) == "" {
		return nil, errors.New("layout is empty")
	}
	if strings.Contains(layout, `\`) {
		return nil, errors.New(`layout: use / between folders, not \`)
	}
	var out Layout
	for _, segment := range strings.Split(layout, "/") {
		if segment == "" {
			return nil, fmt.Errorf("layout %q: empty folder name (a leading, trailing or doubled /)", layout)
		}
		if segment == "." || segment == ".." {
			return nil, fmt.Errorf("layout %q: %s is not a folder name", layout, segment)
		}
		var parts []Part
		rest := segment
		for rest != "" {
			open := strings.IndexByte(rest, '{')
			closing := strings.IndexByte(rest, '}')
			if open < 0 {
				if closing >= 0 {
					return nil, fmt.Errorf("layout %q: } without {", layout)
				}
				parts = append(parts, Part{Text: rest})
				break
			}
			if closing >= 0 && closing < open {
				return nil, fmt.Errorf("layout %q: } without {", layout)
			}
			if open > 0 {
				parts = append(parts, Part{Text: rest[:open]})
			}
			rest = rest[open+1:]
			end := strings.IndexByte(rest, '}')
			if end < 0 {
				return nil, fmt.Errorf("layout %q: { without }", layout)
			}
			key, format, _ := strings.Cut(rest[:end], ":")
			if !keyPattern.MatchString(key) {
				return nil, fmt.Errorf("layout %q: {%s} is not a key", layout, rest[:end])
			}
			if strings.Contains(rest[:end], ":") && !country.Format(format).Valid() {
				return nil, fmt.Errorf("layout %q: {%s}: a country is written zh, en, alpha2 or alpha3", layout, rest[:end])
			}
			parts = append(parts, Part{Key: key, Format: format})
			rest = rest[end+1:]
		}
		out = append(out, parts)
	}
	return out, nil
}

// Keys lists the keys a layout uses, each once, in order.
func (l Layout) Keys() []string {
	var keys []string
	seen := map[string]bool{}
	for _, segment := range l {
		for _, part := range segment {
			if part.Key != "" && !seen[part.Key] {
				seen[part.Key] = true
				keys = append(keys, part.Key)
			}
		}
	}
	return keys
}

// Load reads every views/*.yaml under root, sorted by name. A file whose name
// is not its View's name is refused.
func Load(root string) ([]View, error) {
	paths, err := filepath.Glob(filepath.Join(root, Dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var v View
		decoder := yaml.NewDecoder(strings.NewReader(string(data)))
		decoder.KnownFields(true)
		if err := decoder.Decode(&v); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := v.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if name := strings.TrimSuffix(filepath.Base(path), ".yaml"); name != v.Name {
			return nil, fmt.Errorf("%s: name is %s, so the file should be %s.yaml", path, v.Name, v.Name)
		}
		views = append(views, v)
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	return views, nil
}

// Save writes v to views/<name>.yaml through a temporary file and a rename.
func Save(root string, v View) error {
	if err := v.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".view-*.dgs-part")
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
	return os.Rename(temp.Name(), filepath.Join(dir, v.Name+".yaml"))
}

// Delete removes a View's file. A View that is not there is not an error.
func Delete(root, name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("view %q: not a view name", name)
	}
	err := os.Remove(filepath.Join(root, Dir, name+".yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
