package tree

import (
	"dgs-toolbox/internal/doc/country"
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

// AllPatterns is Pattern, when set, then Patterns: every expression that may
// suggest the field's value, in the order they are tried.
func (f Field) AllPatterns() []string {
	var out []string
	if f.Pattern != "" {
		out = append(out, f.Pattern)
	}
	for _, p := range f.Patterns {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

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
	// Patterns are more regular expressions, for a value written more than
	// one way. They are tried after Pattern, in order, and the first to find
	// a value that fits the field suggests it.
	Patterns []string `yaml:"patterns,omitempty" json:"patterns,omitempty"`
	// Type checks the value and picks the control the page shows. Empty is
	// FieldText.
	Type FieldType `yaml:"type,omitempty" json:"type,omitempty"`
	// Options are a FieldSelect's values, and only its.
	Options []string `yaml:"options,omitempty" json:"options,omitempty"`
	// Format is how a FieldCountry is written, and only its: zh (the
	// default), en, alpha2 or alpha3.
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
	// PerRevision keeps the value with each revision of a document rather
	// than with the Item: a renewed card has a new number and expiry, and the
	// old card keeps its own. Adding a revision asks for these fields.
	PerRevision bool `yaml:"per_revision,omitempty" json:"per_revision"`
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
	// FieldCountry is a country, however it is typed — cn, CHN, China,
	// 中国 — kept in the field's Format.
	FieldCountry FieldType = "country"
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
		for _, p := range f.AllPatterns() {
			if _, err := regexp.Compile(p); err != nil {
				return fmt.Errorf("type %s: key %s: pattern %q: %w", t.Type, f.Key, p, err)
			}
		}
		if f.Format != "" && f.Type != FieldCountry {
			return fmt.Errorf("type %s: key %s: format belongs to a country field", t.Type, f.Key)
		}
		if f.Format != "" && !country.Format(f.Format).Valid() {
			return fmt.Errorf("type %s: key %s: format %q is not zh, en, alpha2 or alpha3", t.Type, f.Key, f.Format)
		}
		switch f.Type {
		case "", FieldText, FieldDate, FieldItem, FieldCountry:
			if len(f.Options) > 0 {
				return fmt.Errorf("type %s: key %s: options belong to a select field", t.Type, f.Key)
			}
		case FieldSelect:
			if len(f.Options) == 0 {
				return fmt.Errorf("type %s: key %s: a select field needs options", t.Type, f.Key)
			}
		default:
			return fmt.Errorf("type %s: key %s: type %q is not text, date, select, item or country", t.Type, f.Key, f.Type)
		}
		if f.PerRevision && t.Kind != KindDocument {
			return fmt.Errorf("type %s: key %s: per_revision belongs to a document; a record has one PDF", t.Type, f.Key)
		}
		if f.PerRevision && f.Distinguishing {
			return fmt.Errorf("type %s: key %s names the document, so it cannot change per revision", t.Type, f.Key)
		}
		if f.Distinguishing && !f.Required {
			return fmt.Errorf("type %s: key %s distinguishes documents, so it must be required", t.Type, f.Key)
		}
	}
	for key, value := range t.Defaults {
		if !seen[key] {
			return fmt.Errorf("type %s: default for %s, which is not a field", t.Type, key)
		}
		if _, err := t.field(key).clean(value); err != nil {
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

// Split divides cleaned fields into the Item's and a revision's own.
func (t Template) Split(fields map[string]string) (item, revision map[string]string) {
	item, revision = map[string]string{}, map[string]string{}
	for k, v := range fields {
		if t.field(k).PerRevision {
			revision[k] = v
		} else {
			item[k] = v
		}
	}
	if len(revision) == 0 {
		revision = nil
	}
	return item, revision
}

// RevisionFields is CleanFields over only the per_revision fields: what a
// new revision is asked for.
func RevisionFields(t Template, given map[string]string) (map[string]string, error) {
	only := t
	only.Fields = nil
	for _, f := range t.Fields {
		if f.PerRevision {
			only.Fields = append(only.Fields, f)
		}
	}
	only.Defaults = nil
	for key := range given {
		if !only.has(key) {
			return nil, fmt.Errorf("%s is not a per_revision field of %s", key, t.Type)
		}
	}
	fields, err := CleanFields(only, given)
	if len(fields) == 0 {
		fields = nil
	}
	return fields, err
}

func (t Template) field(key string) Field {
	for _, f := range t.Fields {
		if f.Key == key {
			return f
		}
	}
	return Field{Key: key}
}

// clean reports whether value suits the field's type, and answers it as it
// is kept: a country in the field's Format. An Item field is checked only
// for its shape here; that the Item exists is checked against the tree.
func (f Field) clean(value string) (string, error) {
	switch f.Type {
	case FieldDate:
		if !validDate(value) {
			return "", fmt.Errorf("%s: %q is not a date written YYYY-MM-DD", f.Key, value)
		}
	case FieldSelect:
		for _, o := range f.Options {
			if o == value {
				return value, nil
			}
		}
		return "", fmt.Errorf("%s: %q is not one of %s", f.Key, value, strings.Join(f.Options, ", "))
	case FieldItem:
		if !idPattern.MatchString(value) {
			return "", fmt.Errorf("%s: %q is not an Item ID", f.Key, value)
		}
	case FieldCountry:
		kept, ok := country.Normalize(value, country.Format(f.Format))
		if !ok {
			return "", fmt.Errorf("%s: %q is not a country dgs knows: try its code, such as CN or CHN", f.Key, value)
		}
		return kept, nil
	}
	return value, nil
}

var idPattern = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
