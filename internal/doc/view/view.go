// Package view is one rule of an Outline: which Items it selects, and the
// path each selected PDF has in the Outline's tree and in an export of it.
// It computes a plan and writes nothing; Outlines keep their rules, and the
// rules are in docs/apps/doc/index.md.
package view

import (
	"dgs-toolbox/internal/doc/country"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
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
	Query map[string]Values `yaml:"query,omitempty" json:"query"`
	// Exclude leaves out a PDF matching any of its conditions: a key's
	// value among those listed, or for `tags` a tag of the revision.
	Exclude map[string]Values `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	// QueryTypes and ExcludeTypes limit a condition, by its key, to Items of
	// the types listed: an Item of another type is not asked it.
	QueryTypes   map[string]Values `yaml:"query_types,omitempty" json:"query_types,omitempty"`
	ExcludeTypes map[string]Values `yaml:"exclude_types,omitempty" json:"exclude_types,omitempty"`
	// Skip names Items the rule leaves out, whatever else it selects.
	Skip      []string  `yaml:"skip,omitempty" json:"skip,omitempty"`
	Selection Selection `yaml:"selection" json:"selection"`
	// Shared also selects an Item shared with someone the query's owner
	// accepts, as if they owned it.
	Shared bool   `yaml:"shared,omitempty" json:"shared,omitempty"`
	Layout string `yaml:"layout" json:"layout"`
	// Default, when set, is written for a key an Item lacks.
	Default *string `yaml:"default,omitempty" json:"default,omitempty"`
	// Dedupe is empty (refuse) or DedupeNumber.
	Dedupe string `yaml:"dedupe,omitempty" json:"dedupe,omitempty"`
	// Order lists, per numbered name, the names {#} numbers from 01, in
	// order. A numbered name is named by what follows its {#}, as written:
	// {#}-{name|type:zh}{-level?}.{ext} is numbered from the order named
	// {name|type:zh}{-level?}.
	Order map[string][]string `yaml:"order,omitempty" json:"order,omitempty"`
	// Numbers sets, per order and name in it, the number that name gets in
	// place of the next: the names after it count on from there, so
	// setting the third of four to 6 numbers them 1, 2, 6, 7.
	Numbers map[string]map[string]int `yaml:"numbers,omitempty" json:"numbers,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// For is conditions as an Item of type t is asked them: those types limits
// to other types are left out.
func For(conditions, types map[string]Values, t string) map[string]Values {
	if len(types) == 0 {
		return conditions
	}
	out := make(map[string]Values, len(conditions))
	for key, values := range conditions {
		if limit, ok := types[key]; ok && !slices.Contains(limit, t) {
			continue
		}
		out[key] = values
	}
	return out
}

// Validate reports the first thing wrong with v.
func (v View) Validate() error {
	if !namePattern.MatchString(v.Name) {
		return fmt.Errorf("view %q: use lowercase letters, digits, _ and -", v.Name)
	}
	if v.Selection != Head && v.Selection != All {
		return fmt.Errorf("view %s: selection %q is neither head nor all", v.Name, v.Selection)
	}
	for i, id := range v.Skip {
		if strings.TrimSpace(id) == "" || slices.Contains(v.Skip[:i], id) {
			return fmt.Errorf("view %s: skip lists %q twice or empty", v.Name, id)
		}
	}
	for _, scope := range []struct {
		name       string
		types      map[string]Values
		conditions map[string]Values
	}{{"query_types", v.QueryTypes, v.Query}, {"exclude_types", v.ExcludeTypes, v.Exclude}} {
		for key, types := range scope.types {
			if _, ok := scope.conditions[key]; !ok || len(types) == 0 {
				return fmt.Errorf("view %s: %s %q names no condition, or no type", v.Name, scope.name, key)
			}
		}
	}
	for _, conditions := range []map[string]Values{v.Query, v.Exclude} {
		for key := range conditions {
			field, contains := SplitKey(key)
			if !keyPattern.MatchString(field) {
				return fmt.Errorf("view %s: condition %q: a key, or a key and%s", v.Name, key, Contains)
			}
			if contains && field == StatusKey {
				return fmt.Errorf("view %s: status is matched whole, not by%s", v.Name, Contains)
			}
		}
	}
	if v.Shared && len(v.Query["owner"]) == 0 {
		return fmt.Errorf("view %s: shared needs an owner in the query", v.Name)
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
	for key, set := range v.Numbers {
		list, ok := v.Order[key]
		if !ok {
			return fmt.Errorf("view %s: numbers %s: order has no list for it", v.Name, key)
		}
		for name, n := range set {
			if place(list, name) == 0 || n < 1 {
				return fmt.Errorf("view %s: numbers %s: %s must be in the order and numbered from 1", v.Name, key, name)
			}
		}
		numbers := Numbered(list, set)
		for i := 1; i < len(numbers); i++ {
			if numbers[i] <= numbers[i-1] {
				return fmt.Errorf("view %s: numbers %s: %s would be %d, not after %s's %d", v.Name, key, list[i], numbers[i], list[i-1], numbers[i-1])
			}
		}
	}
	for _, segment := range layout {
		for _, part := range segment {
			if part.Counter && len(v.Order[part.Of]) == 0 {
				return fmt.Errorf("view %s: {#} numbers %s, so order needs a list for %s", v.Name, part.Of, part.Of)
			}
		}
	}
	return nil
}

// Part is a piece of a layout: fixed text, a key, or {#}. A key written
// {key:format} is a country written in that format — zh, en, alpha2 or
// alpha3 — whatever form the Item keeps it in. Keys written {a|b} are
// alternatives: the first one an Item has is written. A part ending in ?
// is optional and may carry text around its key, {-key?}: an Item with the
// value gets the text and the value, one without gets nothing.
//
// {#} writes the place, as 01, of the name the rest of its folder or file
// name makes, in the View's order named that rest as written. The rest
// leaves out the text straight after {#}, its separator, and a file's
// .{ext}: in {#}-{name}{-level?}.{ext} it is {name}{-level?}, so each
// name and level together is numbered.
type Part struct {
	Text   string `json:"text,omitempty"`
	Key    string `json:"key,omitempty"`
	Format string `json:"format,omitempty"`
	// Counter is {#}. Of is the rest it numbers as written, which names
	// its order, and From and To are that rest's parts in the segment.
	Counter bool   `json:"counter,omitempty"`
	Of      string `json:"of,omitempty"`
	From    int    `json:"-"`
	To      int    `json:"-"`
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
	if strings.HasSuffix(inner, "#") {
		return "", "", "", fmt.Errorf("layout %q: {%s?}: an optional key is not numbered", layout, written)
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

// oldNumbered finds the ways a numbered key was once written: {inner}#,
// {key#:format} and {inner#}, all now {#}-{inner}.
var (
	oldAfter  = regexp.MustCompile(`\{([^{}?#]*)\}#`)
	oldInside = regexp.MustCompile(`\{([a-z0-9_-]+)#:([a-z0-9]+)\}`)
	oldKey    = regexp.MustCompile(`\{([^{}?#]+)#\}`)
)

// Rewrite writes a layout's numbered keys the one way they are written now,
// {#}-{key}, and reports whether anything changed. {key#}, {a|b#}, {a|b}#
// and {key#:format} were once written.
func Rewrite(layout string) (string, bool) {
	out, _ := rewrite(layout)
	return out, out != layout
}

// rewrite is Rewrite, with the order each numbered key was numbered from
// named against the order it is numbered from now.
func rewrite(layout string) (string, map[string]string) {
	segments := strings.Split(layout, "/")
	renamed := map[string]string{}
	for i, segment := range segments {
		segment = oldInside.ReplaceAllString(segment, "{$1:$2#}")
		segment = oldAfter.ReplaceAllString(segment, "{$1#}")
		m := oldKey.FindStringSubmatch(segment)
		if m == nil || strings.Contains(segment, "{#}") {
			segments[i] = segment
			continue
		}
		old := m[1]
		if !strings.Contains(old, "|") {
			old, _, _ = strings.Cut(old, ":")
		}
		segment = strings.Replace(segment, m[0], "{#}-{"+m[1]+"}", 1)
		segments[i] = segment
		if parsed, err := Parse(segment); err == nil {
			for _, part := range parsed[0] {
				if part.Counter {
					renamed[old] = part.Of
				}
			}
		}
	}
	return strings.Join(segments, "/"), renamed
}

// Upgrade is v with its layout rewritten as Rewrite does and each order
// renamed after the numbered name it now numbers.
func Upgrade(v View) (View, bool) {
	layout, renamed := rewrite(v.Layout)
	if layout == v.Layout {
		return v, false
	}
	v.Layout = layout
	if len(v.Order) > 0 {
		order := map[string][]string{}
		for key, values := range v.Order {
			if _, moved := renamed[key]; !moved {
				order[key] = values
			}
		}
		for old, now := range renamed {
			if values, ok := v.Order[old]; ok {
				order[now] = values
			}
		}
		v.Order = order
	}
	v.Numbers = maps.Clone(v.Numbers)
	for old, now := range renamed {
		if set, ok := v.Numbers[old]; ok {
			delete(v.Numbers, old)
			v.Numbers[now] = set
		}
	}
	return v, true
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
		var written []string // each part as the layout writes it
		rest := segment
		for rest != "" {
			open := strings.IndexByte(rest, '{')
			closing := strings.IndexByte(rest, '}')
			if open < 0 {
				if closing >= 0 {
					return nil, fmt.Errorf("layout %q: } without {", layout)
				}
				parts = append(parts, Part{Text: rest})
				written = append(written, rest)
				break
			}
			if closing >= 0 && closing < open {
				return nil, fmt.Errorf("layout %q: } without {", layout)
			}
			if open > 0 {
				parts = append(parts, Part{Text: rest[:open]})
				written = append(written, rest[:open])
			}
			rest = rest[open+1:]
			end := strings.IndexByte(rest, '}')
			if end < 0 {
				return nil, fmt.Errorf("layout %q: { without }", layout)
			}
			inner := rest[:end]
			written = append(written, "{"+inner+"}")
			if inner == "#" {
				parts = append(parts, Part{Counter: true})
				rest = rest[end+1:]
				continue
			}
			optional := strings.HasSuffix(inner, "?")
			var prefix, suffix string
			if optional {
				var err error
				if prefix, inner, suffix, err = optionalParts(layout, strings.TrimSuffix(inner, "?")); err != nil {
					return nil, err
				}
			}
			if strings.HasSuffix(inner, "#") {
				return nil, fmt.Errorf("layout %q: {%s}: number the whole name with {#}, as {#}-{%s}", layout, inner, strings.TrimSuffix(inner, "#"))
			}
			var choices []Part
			for _, written := range strings.Split(inner, "|") {
				choice, err := parseKey(layout, written)
				if err != nil {
					return nil, err
				}
				choices = append(choices, choice)
			}
			part := choices[0]
			part.Or = choices[1:]
			rest = rest[end+1:]
			if strings.HasPrefix(rest, "#") {
				return nil, fmt.Errorf("layout %q: {%s}#: number the whole name with {#}, as {#}-{%s}", layout, inner, inner)
			}
			if optional {
				part.Optional, part.Prefix, part.Suffix = true, prefix, suffix
			}
			parts = append(parts, part)
		}
		if err := counted(layout, parts, written); err != nil {
			return nil, err
		}
		out = append(out, parts)
	}
	return out, nil
}

// counted finds what a segment's {#} numbers: the parts after it, less
// the text straight after it and a trailing .{ext}. A segment has one {#}
// at most, and it numbers at least one key.
func counted(layout string, parts []Part, written []string) error {
	at := -1
	for i, p := range parts {
		if !p.Counter {
			continue
		}
		if at >= 0 {
			return fmt.Errorf("layout %q: one {#} to a folder or file name", layout)
		}
		at = i
	}
	if at < 0 {
		return nil
	}
	from, to := at+1, len(parts)
	if from < to && parts[from].Key == "" {
		from++
	}
	if to > from && parts[to-1].Key == "ext" {
		to--
		if to > from && parts[to-1].Text == "." {
			to--
		}
	}
	keyed := false
	for _, p := range parts[from:to] {
		keyed = keyed || p.Key != ""
	}
	if !keyed {
		return fmt.Errorf("layout %q: {#} numbers the keys after it, and there is none", layout)
	}
	parts[at].From, parts[at].To = from, to
	parts[at].Of = strings.Join(written[from:to], "")
	return nil
}

// parseKey reads one key as written between { and }: key or key:format.
func parseKey(layout, written string) (Part, error) {
	if strings.Contains(written, "#") {
		return Part{}, fmt.Errorf("layout %q: {%s}: number the whole name with {#}, as {#}-{key}", layout, written)
	}
	key, format, _ := strings.Cut(written, ":")
	if !keyPattern.MatchString(key) {
		return Part{}, fmt.Errorf("layout %q: {%s} is not a key", layout, written)
	}
	if key == "type" && strings.Contains(written, ":") && format != "zh" && format != "en" {
		return Part{}, fmt.Errorf("layout %q: {%s}: a type is written zh or en", layout, written)
	}
	if strings.Contains(written, ":") && !country.Format(format).Valid() {
		return Part{}, fmt.Errorf("layout %q: {%s}: a country is written zh, en, alpha2 or alpha3", layout, written)
	}
	return Part{Key: key, Format: format}, nil
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
