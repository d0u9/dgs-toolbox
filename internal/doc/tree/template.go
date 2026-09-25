package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind is what an Item is: a document, with revisions and HEAD, or a record,
// with one PDF.
type Kind string

const (
	KindDocument Kind = "document"
	KindRecord   Kind = "record"
)

// Field is one key a Template asks for.
type Field struct {
	Key            string `yaml:"key" json:"key"`
	Required       bool   `yaml:"required,omitempty" json:"required"`
	Distinguishing bool   `yaml:"distinguishing,omitempty" json:"distinguishing"`
	// Pattern, when set, is a regular expression (Go's RE2 syntax) that finds
	// the field's value in a document's recognised text: the first capture
	// group when it has one, else the whole match. It only suggests; a person
	// accepts the value by importing.
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
}

// Template is one type's fields and defaults.
type Template struct {
	Type     string            `yaml:"type" json:"type"`
	Kind     Kind              `yaml:"kind" json:"kind"`
	Fields   []Field           `yaml:"fields" json:"fields"`
	Defaults map[string]string `yaml:"defaults" json:"defaults"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate reports the first thing wrong with t.
func (t Template) Validate() error {
	if !namePattern.MatchString(t.Type) {
		return fmt.Errorf("type %q: use lowercase letters, digits, _ and -", t.Type)
	}
	if t.Kind != KindDocument && t.Kind != KindRecord {
		return fmt.Errorf("type %s: kind %q is neither document nor record", t.Type, t.Kind)
	}
	seen := map[string]bool{}
	for _, f := range t.Fields {
		if !namePattern.MatchString(f.Key) {
			return fmt.Errorf("type %s: key %q: use lowercase letters, digits, _ and -", t.Type, f.Key)
		}
		if seen[f.Key] {
			return fmt.Errorf("type %s: key %s is listed twice", t.Type, f.Key)
		}
		seen[f.Key] = true
		if f.Pattern != "" {
			if _, err := regexp.Compile(f.Pattern); err != nil {
				return fmt.Errorf("type %s: key %s: pattern: %w", t.Type, f.Key, err)
			}
		}
		if f.Distinguishing && !f.Required {
			return fmt.Errorf("type %s: key %s distinguishes documents, so it must be required", t.Type, f.Key)
		}
	}
	for key := range t.Defaults {
		if !seen[key] {
			return fmt.Errorf("type %s: default for %s, which is not a field", t.Type, key)
		}
	}
	return nil
}

// LoadTemplates reads every templates/*.yaml under root, sorted by type. A
// file whose name is not its type is refused: the name is how a person finds
// the Template to edit.
func LoadTemplates(root string) ([]Template, error) {
	paths, err := filepath.Glob(filepath.Join(root, TemplatesDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	templates := make([]Template, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var t Template
		decoder := yaml.NewDecoder(strings.NewReader(string(data)))
		decoder.KnownFields(true)
		if err := decoder.Decode(&t); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := t.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if name := strings.TrimSuffix(filepath.Base(path), ".yaml"); name != t.Type {
			return nil, fmt.Errorf("%s: type is %s, so the file should be %s.yaml", path, t.Type, t.Type)
		}
		templates = append(templates, t)
	}
	sort.Slice(templates, func(i, j int) bool { return templates[i].Type < templates[j].Type })
	return templates, nil
}
