package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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
	// Type checks the value and picks the control the page shows. Empty is
	// FieldText.
	Type FieldType `yaml:"type,omitempty" json:"type,omitempty"`
	// Options are a FieldSelect's values, and only its.
	Options []string `yaml:"options,omitempty" json:"options,omitempty"`
}

// FieldType is what a field's value is.
type FieldType string

const (
	FieldText FieldType = "text"
	// FieldDate is YYYY-MM-DD.
	FieldDate FieldType = "date"
	// FieldSelect is one of the field's Options.
	FieldSelect FieldType = "select"
	// FieldItem is another Item's ID.
	FieldItem FieldType = "item"
)

// Template is one type's fields and defaults.
type Template struct {
	Type     string            `yaml:"type" json:"type"`
	Kind     Kind              `yaml:"kind" json:"kind"`
	Fields   []Field           `yaml:"fields" json:"fields"`
	Defaults map[string]string `yaml:"defaults" json:"defaults"`
	// IgnoreDates are dates, YYYY-MM-DD, never suggested for a date field: a
	// birthday printed on every page of a person's documents.
	IgnoreDates []string `yaml:"ignore_dates,omitempty" json:"ignore_dates,omitempty"`
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
		switch f.Type {
		case "", FieldText, FieldDate, FieldItem:
			if len(f.Options) > 0 {
				return fmt.Errorf("type %s: key %s: options belong to a select field", t.Type, f.Key)
			}
		case FieldSelect:
			if len(f.Options) == 0 {
				return fmt.Errorf("type %s: key %s: a select field needs options", t.Type, f.Key)
			}
		default:
			return fmt.Errorf("type %s: key %s: type %q is not text, date, select or item", t.Type, f.Key, f.Type)
		}
		if f.Distinguishing && !f.Required {
			return fmt.Errorf("type %s: key %s distinguishes documents, so it must be required", t.Type, f.Key)
		}
	}
	for key, value := range t.Defaults {
		if !seen[key] {
			return fmt.Errorf("type %s: default for %s, which is not a field", t.Type, key)
		}
		if err := t.field(key).check(value); err != nil {
			return fmt.Errorf("type %s: default for %s: %w", t.Type, key, err)
		}
	}
	for _, d := range t.IgnoreDates {
		if !validDate(d) {
			return fmt.Errorf("type %s: ignore_dates: %q is not a date written YYYY-MM-DD", t.Type, d)
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
		t, err := ParseTemplate(data)
		if err != nil {
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

// ParseTemplate reads one Template file's content and validates it. A key the
// Template does not know is refused, so a misspelt one is not silently lost.
func ParseTemplate(data []byte) (Template, error) {
	var t Template
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&t); err != nil {
		return Template{}, err
	}
	return t, t.Validate()
}

func (t Template) field(key string) Field {
	for _, f := range t.Fields {
		if f.Key == key {
			return f
		}
	}
	return Field{Key: key}
}

// check reports whether value suits the field's type. An Item field is
// checked only for its shape here; that the Item exists is checked against
// the tree.
func (f Field) check(value string) error {
	switch f.Type {
	case FieldDate:
		if !validDate(value) {
			return fmt.Errorf("%s: %q is not a date written YYYY-MM-DD", f.Key, value)
		}
	case FieldSelect:
		for _, o := range f.Options {
			if o == value {
				return nil
			}
		}
		return fmt.Errorf("%s: %q is not one of %s", f.Key, value, strings.Join(f.Options, ", "))
	case FieldItem:
		if !idPattern.MatchString(value) {
			return fmt.Errorf("%s: %q is not an Item ID", f.Key, value)
		}
	}
	return nil
}

var idPattern = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
