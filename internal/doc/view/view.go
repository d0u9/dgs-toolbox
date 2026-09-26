// Package view is one rule of an Outline: which Items it selects, and the
// path each selected PDF has in the Outline's tree and in an export of it.
// It computes a plan and writes nothing; Outlines keep their rules, and the
// rules are in docs/apps/doc/index.md.
package view

import (
	"dgs-toolbox/internal/doc/country"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

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

// View is one rule of an Outline. Its name is unique within the Outline; an
// export's manifest records it against every file the rule placed.
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
	// Order lists, per key, the values a numbered key {key#} may have, in
	// the order they are numbered from 01.
	Order map[string][]string `yaml:"order,omitempty" json:"order,omitempty"`
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
	layout, err := Parse(v.Layout)
	if err != nil {
		return fmt.Errorf("view %s: %w", v.Name, err)
	}
	for key, values := range v.Order {
		if len(values) == 0 {
			return fmt.Errorf("view %s: order %s is empty", v.Name, key)
		}
		for i, a := range values {
			if strings.TrimSpace(a) == "" {
				return fmt.Errorf("view %s: order %s has an empty value", v.Name, key)
			}
			for _, b := range values[:i] {
				if sameValue(a, b) {
					return fmt.Errorf("view %s: order %s lists %s and %s, which are one value", v.Name, key, b, a)
				}
			}
		}
	}
	for _, segment := range layout {
		for _, part := range segment {
			if part.Key == "" {
				continue
			}
			if key := part.OrderKey(); part.Numbered && len(v.Order[key]) == 0 {
				return fmt.Errorf("view %s: %s is numbered, so order needs a list for %s", v.Name, part.written(), key)
			}
		}
	}
	return nil
}

// Part is a piece of a layout: fixed text, or a key. A key written
// {key:format} is a country written in that format — zh, en, alpha2 or
// alpha3 — whatever form the Item keeps it in. A key written {key#} is
// prefixed with its value's place in the View's order, as 01-. Keys written
// {a|b} are alternatives: the first one an Item has is written. Alternatives
// are numbered together, {a|b}#, from one order named a|b. A part ending
// in ? is optional and may carry text around its key, {-key?}: an Item
// with the value gets the text and the value, one without gets nothing.
type Part struct {
	Text     string `json:"text,omitempty"`
	Key      string `json:"key,omitempty"`
	Format   string `json:"format,omitempty"`
	Numbered bool   `json:"numbered,omitempty"`
	// Or holds the alternatives after the first, tried in order.
	Or []Part `json:"or,omitempty"`
	// Optional parts write nothing for an Item without the key, and
	// Prefix and Suffix around its value otherwise.
	Optional bool   `json:"optional,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	Suffix   string `json:"suffix,omitempty"`
}

// keyChar is a character that may start or end a key.
func keyChar(r byte) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_'
}

// optionalParts splits an optional part's inner text, the ? removed, into
// the text before its key, the key as written, and the text after it.
func optionalParts(layout, written string) (prefix, inner, suffix string, err error) {
	start, end := 0, len(written)
	for start < end && !keyChar(written[start]) {
		start++
	}
	for end > start && !keyChar(written[end-1]) {
		end--
	}
	prefix, inner, suffix = written[:start], written[start:end], written[end:]
	if inner == "" {
		return "", "", "", fmt.Errorf("layout %q: {%s?} names no key", layout, written)
	}
	if strings.ContainsAny(prefix+suffix, "{}/:|#?") {
		return "", "", "", fmt.Errorf("layout %q: {%s?}: the text around a key may not hold { } / : | # ?", layout, written)
	}
	return prefix, inner, suffix, nil
}

// choices is p and its alternatives, in the order they are tried.
func (p Part) choices() []Part {
	return append([]Part{{Key: p.Key, Format: p.Format}}, p.Or...)
}

// OrderKey names the order a numbered part is numbered from: its key, or
// for alternatives each written as key or key:format, joined by |.
func (p Part) OrderKey() string {
	if len(p.Or) == 0 {
		return p.Key
	}
	var names []string
	for _, c := range p.choices() {
		name := c.Key
		if c.Format != "" {
			name += ":" + c.Format
		}
		names = append(names, name)
	}
	return strings.Join(names, "|")
}

// written is p as a layout writes it when numbered.
func (p Part) written() string {
	if len(p.Or) == 0 {
		return "{" + p.Key + "#}"
	}
	return "{" + p.OrderKey() + "}#"
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
			inner := rest[:end]
			optional := strings.HasSuffix(inner, "?")
			var prefix, suffix string
			if optional {
				var err error
				if prefix, inner, suffix, err = optionalParts(layout, strings.TrimSuffix(inner, "?")); err != nil {
					return nil, err
				}
			}
			var choices []Part
			for _, written := range strings.Split(inner, "|") {
				choice, err := parseKey(layout, written)
				if err != nil {
					return nil, err
				}
				if choice.Numbered && strings.Contains(inner, "|") {
					return nil, fmt.Errorf("layout %q: {%s}: number alternatives together, as {%s}#", layout, inner, strings.ReplaceAll(inner, "#", ""))
				}
				choices = append(choices, choice)
			}
			part := choices[0]
			part.Or = choices[1:]
			rest = rest[end+1:]
			if strings.HasPrefix(rest, "#") {
				// {key}# is {key#}; {a|b}# numbers the alternatives together.
				part.Numbered = true
				rest = rest[1:]
			}
			if optional {
				if part.Numbered {
					return nil, fmt.Errorf("layout %q: {%s%s%s?}: an optional key is not numbered", layout, prefix, inner, suffix)
				}
				part.Optional, part.Prefix, part.Suffix = true, prefix, suffix
			}
			parts = append(parts, part)
		}
		out = append(out, parts)
	}
	return out, nil
}

// parseKey reads one key as written between { and }: key, key#, key:format.
func parseKey(layout, written string) (Part, error) {
	key, format, _ := strings.Cut(written, ":")
	key, numbered := strings.CutSuffix(key, "#")
	if !keyPattern.MatchString(key) {
		return Part{}, fmt.Errorf("layout %q: {%s} is not a key", layout, written)
	}
	if key == "type" && strings.Contains(written, ":") && format != "zh" && format != "en" {
		return Part{}, fmt.Errorf("layout %q: {%s}: a type is written zh or en", layout, written)
	}
	if strings.Contains(written, ":") && !country.Format(format).Valid() {
		return Part{}, fmt.Errorf("layout %q: {%s}: a country is written zh, en, alpha2 or alpha3", layout, written)
	}
	return Part{Key: key, Format: format, Numbered: numbered}, nil
}

// Keys lists the keys a layout uses, each once, in order.
func (l Layout) Keys() []string {
	var keys []string
	seen := map[string]bool{}
	for _, segment := range l {
		for _, part := range segment {
			if part.Key == "" {
				continue
			}
			for _, choice := range part.choices() {
				if !seen[choice.Key] {
					seen[choice.Key] = true
					keys = append(keys, choice.Key)
				}
			}
		}
	}
	return keys
}

// sameValue reports whether a and b are one value: one country however each
// is written, or else equal ignoring case.
func sameValue(a, b string) bool {
	x, okA := country.Normalize(a, country.Alpha3)
	y, okB := country.Normalize(b, country.Alpha3)
	if okA && okB {
		return x == y
	}
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
