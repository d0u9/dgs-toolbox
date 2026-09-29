// Package view is one rule of an Outline: which Items it selects, and the
// path each selected PDF has in the Outline's tree and in an export of it.
// It computes a plan and writes nothing; Outlines keep their rules, and the
// rules are in docs/apps/doc/index.md.
package view

import (
	"dgs-toolbox/internal/doc/country"
	"dgs-toolbox/internal/doc/expr"
	"errors"
	"fmt"
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
//
// A rule is a Node: its If picks the Items it places, its Path and File
// place them, and its Children, an if/elif/else chain, place some of them
// deeper.
type View struct {
	Name string `yaml:"name" json:"name"`
	Node `yaml:",inline"`
	// Selection is which revisions of a document the rule takes.
	Selection Selection `yaml:"selection" json:"selection"`
	// Shared also selects an Item shared with someone, as if they owned it:
	// its If is asked with owner as each person it is shared with.
	Shared bool `yaml:"shared,omitempty" json:"shared,omitempty"`
	// Inherit names fields that link to an Item, such as original: a key
	// an Item lacks is taken from the Item its first such field links to
	// that has it, so a translation is placed by its original's level.
	Inherit []string `yaml:"inherit,omitempty" json:"inherit,omitempty"`
	// Dedupe is empty (refuse) or DedupeNumber.
	Dedupe string `yaml:"dedupe,omitempty" json:"dedupe,omitempty"`
	// Order lists, per numbered name, the names {#} numbers from 01, in
	// order. A numbered name is named by what follows its {#}, as written:
	// {#}-{name|type:zh}[-{level}].{ext} is numbered from the order named
	// {name|type:zh}[-{level}]. One order serves the whole rule.
	Order map[string][]string `yaml:"order,omitempty" json:"order,omitempty"`
	// Numbers sets, per order and name in it, the number that name gets in
	// place of the next: the names after it count on from there, so
	// setting the third of four to 6 numbers them 1, 2, 6, 7.
	Numbers map[string]map[string]int `yaml:"numbers,omitempty" json:"numbers,omitempty"`
	// Unnumbered lists, per order, the names {#} leaves unnumbered: their
	// number and the text straight after {#} are not written, and they
	// take no number, so the next name counts on from the one before.
	Unnumbered map[string][]string `yaml:"unnumbered,omitempty" json:"unnumbered,omitempty"`
}

// Node is a rule, or one of its children.
type Node struct {
	// If is the condition, in package expr's language, an Item must meet.
	// A rule without one takes every Item; a child without one is the
	// else, and comes last.
	If string `yaml:"if,omitempty" json:"if,omitempty"`
	// Path is folders, appended to the parent's.
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
	// File is the file name, in place of the parent's.
	File string `yaml:"file,omitempty" json:"file,omitempty"`
	// Default, when set, is written for a key an Item lacks, in place of
	// the parent's.
	Default *string `yaml:"default,omitempty" json:"default,omitempty"`
	// Children place some of what the node takes: the first whose If an
	// Item meets. One none takes the node places itself.
	Children []Node `yaml:"children,omitempty" json:"children,omitempty"`
}

// Out reports whether the node leaves what it takes out of the tree: a
// child with no path, file or children.
func (n Node) Out() bool { return n.Path == "" && n.File == "" && len(n.Children) == 0 }

// walk calls fn on v's node and each below it, parents first, with where
// it is (children 2.1: ) and the layout it places by: its path after its
// parents', then the nearest file. A node that leaves out has none.
func (v View) walk(fn func(where string, n Node, layout string, root bool) error) error {
	var visit func(where string, n Node, path []string, file string, root bool) error
	visit = func(where string, n Node, path []string, file string, root bool) error {
		if n.Path != "" {
			path = append(slices.Clone(path), strings.Trim(n.Path, "/"))
		}
		if n.File != "" {
			file = n.File
		}
		layout := ""
		if root || !n.Out() {
			layout = strings.Join(append(slices.Clone(path), file), "/")
		}
		if err := fn(where, n, layout, root); err != nil {
			return err
		}
		for i, c := range n.Children {
			at := strings.TrimSuffix(where, ": ")
			if at == "" {
				at = "children "
			} else {
				at += "."
			}
			if err := visit(fmt.Sprintf("%s%d: ", at, i+1), c, path, file, false); err != nil {
				return err
			}
		}
		return nil
	}
	return visit("", v.Node, nil, "", true)
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
	if v.File == "" {
		return fmt.Errorf("view %s: a rule needs a file", v.Name)
	}
	if v.Shared {
		e, err := expr.Parse(v.If)
		if err != nil || !slices.Contains(e.Keys(), "owner") {
			return fmt.Errorf("view %s: shared needs an owner in its if", v.Name)
		}
	}
	if v.Dedupe != "" && v.Dedupe != DedupeNumber {
		return fmt.Errorf("view %s: dedupe %q: leave it out, or use number", v.Name, v.Dedupe)
	}
	var counters []Part
	if err := v.walk(func(where string, n Node, layout string, root bool) error {
		if n.If != "" {
			if _, err := expr.Parse(n.If); err != nil {
				return fmt.Errorf("view %s: %sif: %w", v.Name, where, err)
			}
		}
		for i, c := range n.Children {
			if c.If == "" && i != len(n.Children)-1 {
				return fmt.Errorf("view %s: %schild %d has no if, so it is the else and must come last", v.Name, where, i+1)
			}
			if c.If == "" && c.Out() {
				return fmt.Errorf("view %s: %schild %d is an else that leaves everything out; give it a path or file", v.Name, where, i+1)
			}
		}
		if layout == "" {
			return nil
		}
		parsed, err := Parse(layout)
		if err != nil {
			return fmt.Errorf("view %s: %s%w", v.Name, where, err)
		}
		counters = append(counters, parsed.Counters()...)
		return nil
	}); err != nil {
		return err
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
			if place(list, name) == 0 || n < 0 {
				return fmt.Errorf("view %s: numbers %s: %s must be in the order and numbered from 0", v.Name, key, name)
			}
			if place(v.Unnumbered[key], name) > 0 {
				return fmt.Errorf("view %s: numbers %s: %s is unnumbered", v.Name, key, name)
			}
		}
	}
	for key, names := range v.Unnumbered {
		list, ok := v.Order[key]
		if !ok {
			return fmt.Errorf("view %s: unnumbered %s: order has no list for it", v.Name, key)
		}
		for _, name := range names {
			if place(list, name) == 0 {
				return fmt.Errorf("view %s: unnumbered %s: %s is not in the order", v.Name, key, name)
			}
		}
	}
	for key, list := range v.Order {
		numbers := Numbered(list, v.Numbers[key], v.Unnumbered[key])
		last := -1
		for i, n := range numbers {
			if n == Unnumbered {
				continue
			}
			if last >= 0 && n <= numbers[last] {
				return fmt.Errorf("view %s: numbers %s: %s would be %d, not after %s's %d", v.Name, key, list[i], n, list[last], numbers[last])
			}
			last = i
		}
	}
	for _, part := range counters {
		if len(v.Order[part.Of]) == 0 {
			return fmt.Errorf("view %s: {#} numbers %s, so order needs a list for %s", v.Name, part.Of, part.Of)
		}
	}
	for _, link := range v.Inherit {
		if !namePattern.MatchString(link) {
			return fmt.Errorf("view %s: inherit %q: name a field that links to an Item, such as original", v.Name, link)
		}
	}
	return nil
}

// Part is a piece of a layout: fixed text, a key, {#}, or an optional
// group. A key written {key:format} is a country written in that format —
// zh, en, alpha2 or alpha3 — whatever form the Item keeps it in. Keys
// written {a|b} are alternatives: the first one an Item has is written.
//
// A group, [...], is optional: it is written only when the Item has every
// key in it, and otherwise leaves nothing, its text included: [-{degree}]
// writes -本科 or nothing. Groups do not nest. A group starting with /, at
// the end of a folder or file name, is a folder of its own:
// license[/{language}]/x puts a translation in license/en/ and the
// original in license/. [/{#}-{language}] numbers that folder, 10-en, from
// the order named {language}. A folder group may add several folders, all
// or none: [/bill/{service}] puts a bill in bill/水/.
//
// {#} writes the place, as 01, of the name the rest of its folder or file
// name makes, in the View's order named that rest as written. The rest
// leaves out the text straight after {#}, a folder group ending the name,
// and a file's .{ext}: in {#}-{name}[-{level}].{ext} it is
// {name}[-{level}], so each name and level together is numbered.
type Part struct {
	Text   string `json:"text,omitempty"`
	Key    string `json:"key,omitempty"`
	Format string `json:"format,omitempty"`
	// Counter is {#}. Of is the rest it numbers as written, which names
	// its order, and From and To are that rest's parts beside it.
	Counter bool `json:"counter,omitempty"`
	// End is {/#}: the number's rest stops before it, so what follows is
	// written but not numbered. It writes nothing itself.
	End  bool   `json:"end,omitempty"`
	Of   string `json:"of,omitempty"`
	From int    `json:"-"`
	To   int    `json:"-"`
	// Or holds the alternatives after the first, tried in order.
	Or []Part `json:"or,omitempty"`
	// Group holds an optional group's parts, written only when the Item
	// has every key among them.
	Group []Part `json:"group,omitempty"`
	// Folder is a group written [/...]: a folder of its own after the name
	// it ends.
	Folder bool `json:"folder,omitempty"`
}

// keyChar is a character that may start or end a key.
func keyChar(r byte) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_'
}

// choices is p and its alternatives, in the order they are tried.
func (p Part) choices() []Part {
	return append([]Part{{Key: p.Key, Format: p.Format}}, p.Or...)
}

// oldNumbered finds the ways a numbered key was once written: {inner}#,
// {key#:format} and {inner#}, all now {#}-{inner}.
// Layout is a parsed layout, one list of parts per path segment.
type Layout [][]Part

// keyPattern is a key, or a key of the Item a field links to:
// original.level is the level of the revision original names.
var keyPattern = regexp.MustCompile(`^[a-z0-9_-]+(\.[a-z0-9_-]+)?$`)

// Parse splits a layout into segments and parts. A layout is relative: no
// leading /, no empty segment, and no segment that is only . or .., so a path
// it makes stays inside the Target. An optional key written the old way,
// {-key?}, is read as the group it is now.
func Parse(layout string) (Layout, error) {
	if strings.TrimSpace(layout) == "" {
		return nil, errors.New("layout is empty")
	}
	if strings.Contains(layout, `\`) {
		return nil, errors.New(`layout: use / between folders, not \`)
	}
	var out Layout
	for _, segment := range splitSegments(layout) {
		if segment == "" {
			return nil, fmt.Errorf("layout %q: empty folder name (a leading, trailing or doubled /)", layout)
		}
		if segment == "." || segment == ".." {
			return nil, fmt.Errorf("layout %q: %s is not a folder name", layout, segment)
		}
		parts, written, err := parseParts(layout, segment, false)
		if err != nil {
			return nil, err
		}
		for i, p := range parts {
			if p.Folder && (i == 0 || i != len(parts)-1) {
				return nil, fmt.Errorf("layout %q: %s: a group that adds a folder ends a name that has more before it, as license[/{language}]", layout, written[i])
			}
		}
		if err := counted(layout, parts, written); err != nil {
			return nil, err
		}
		out = append(out, parts)
	}
	return out, nil
}

// parseParts reads one folder or file name, or a group's inside, into
// parts, with each part as the layout writes it.
func parseParts(layout, text string, group bool) ([]Part, []string, error) {
	var parts []Part
	var written []string
	rest := text
	for rest != "" {
		at := strings.IndexAny(rest, "{}[]")
		if at < 0 {
			parts = append(parts, Part{Text: rest})
			written = append(written, rest)
			break
		}
		if at > 0 {
			parts = append(parts, Part{Text: rest[:at]})
			written = append(written, rest[:at])
		}
		switch rest[at] {
		case '}':
			return nil, nil, fmt.Errorf("layout %q: } without {", layout)
		case ']':
			return nil, nil, fmt.Errorf("layout %q: ] without [", layout)
		case '[':
			if group {
				return nil, nil, fmt.Errorf("layout %q: a group is not put in another", layout)
			}
			end := strings.IndexByte(rest[at:], ']')
			if end < 0 {
				return nil, nil, fmt.Errorf("layout %q: [ without ]", layout)
			}
			inner := rest[at+1 : at+end]
			if strings.Contains(inner, "[") {
				return nil, nil, fmt.Errorf("layout %q: a group is not put in another", layout)
			}
			rest = rest[at+end+1:]
			body, folder := strings.CutPrefix(inner, "/")
			nested := strings.Contains(body, "/")
			if nested && !folder {
				return nil, nil, fmt.Errorf("layout %q: [%s]: a group holds a / only after one at its start, where it adds folders", layout, inner)
			}
			if nested {
				// [/bill/{service}] adds bill/水 or nothing.
				for _, name := range splitSegments(body) {
					if name == "" || name == "." || name == ".." {
						return nil, nil, fmt.Errorf("layout %q: [%s]: %q is not a folder name", layout, inner, name)
					}
				}
			}
			inside, words, err := parseParts(layout, body, true)
			if err != nil {
				return nil, nil, err
			}
			keyed := false
			for _, p := range inside {
				keyed = keyed || p.Key != ""
				if (p.Counter || p.End) && !folder {
					return nil, nil, fmt.Errorf("layout %q: [%s]: {#} in a group numbers only the folder it adds, as [/{#}-{language}]", layout, inner)
				}
				if (p.Counter || p.End) && nested {
					return nil, nil, fmt.Errorf("layout %q: [%s]: {#} numbers a group that adds one folder, not several", layout, inner)
				}
			}
			if !keyed {
				return nil, nil, fmt.Errorf("layout %q: [%s] holds no key", layout, inner)
			}
			if folder {
				if err := counted(layout, inside, words); err != nil {
					return nil, nil, err
				}
			}
			parts = append(parts, Part{Group: inside, Folder: folder})
			written = append(written, "["+inner+"]")
		case '{':
			end := strings.IndexByte(rest[at:], '}')
			if end < 0 {
				return nil, nil, fmt.Errorf("layout %q: { without }", layout)
			}
			inner := rest[at+1 : at+end]
			if strings.ContainsAny(inner, "{[]") {
				return nil, nil, fmt.Errorf("layout %q: { without }", layout)
			}
			rest = rest[at+end+1:]
			written = append(written, "{"+inner+"}")
			if inner == "#" {
				parts = append(parts, Part{Counter: true})
				continue
			}
			if inner == "/#" {
				parts = append(parts, Part{End: true})
				continue
			}
			if strings.HasSuffix(inner, "#") {
				return nil, nil, fmt.Errorf("layout %q: {%s}: number the whole name with {#}, as {#}-{%s}", layout, inner, strings.TrimSuffix(inner, "#"))
			}
			var choices []Part
			for _, one := range strings.Split(inner, "|") {
				choice, err := parseKey(layout, one)
				if err != nil {
					return nil, nil, err
				}
				choices = append(choices, choice)
			}
			if strings.HasPrefix(rest, "#") {
				return nil, nil, fmt.Errorf("layout %q: {%s}#: number the whole name with {#}, as {#}-{%s}", layout, inner, inner)
			}
			part := choices[0]
			part.Or = choices[1:]
			parts = append(parts, part)
		}
	}
	return parts, written, nil
}

// splitSegments splits a layout at each / outside braces and brackets: the
// / in [/{language}] belongs to its group.
func splitSegments(layout string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(layout); i++ {
		switch layout[i] {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case '/':
			if depth == 0 {
				out = append(out, layout[start:i])
				start = i + 1
			}
		}
	}
	return append(out, layout[start:])
}

// counted finds what a name's {#} numbers: the parts after it, less the
// text straight after it, a folder group ending the name and a trailing
// .{ext}. A name has one {#} at most, and it numbers at least one key.
func counted(layout string, parts []Part, written []string) error {
	at, end := -1, -1
	for i, p := range parts {
		if p.End {
			if at < 0 || end >= 0 {
				return fmt.Errorf("layout %q: {/#} ends the rest one {#} before it numbers", layout)
			}
			end = i
		}
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
	if end >= 0 {
		// {#}-{name}{/#}[-{signed}]: the number stops at {/#}.
		to = end
	}
	if from < to && parts[from].Text != "" {
		from++
	}
	if to > from && parts[to-1].Folder {
		// {#}-{level}[/{language}]: the folder a group adds is not numbered.
		to--
	}
	if to > from && parts[to-1].Key == "ext" {
		to--
		if to > from && parts[to-1].Text == "." {
			to--
		}
	}
	keyed := false
	for _, p := range parts[from:to] {
		keyed = keyed || p.Key != "" || len(p.Group) > 0
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
	if isType(key) && strings.Contains(written, ":") && format != "zh" && format != "en" {
		return Part{}, fmt.Errorf("layout %q: {%s}: a type is written zh or en", layout, written)
	}
	if strings.Contains(written, ":") && !isType(key) && !isDateFormat(format) && !country.Format(format).Valid() {
		return Part{}, fmt.Errorf("layout %q: {%s}: a country is written zh, en, alpha2 or alpha3, a date compact or fy", layout, written)
	}
	return Part{Key: key, Format: format}, nil
}

// Counters is every {#} of the layout, a folder group's included.
func (l Layout) Counters() []Part {
	var out []Part
	var walk func([]Part)
	walk = func(parts []Part) {
		for _, p := range parts {
			if p.Counter {
				out = append(out, p)
			}
			walk(p.Group)
		}
	}
	for _, segment := range l {
		walk(segment)
	}
	return out
}

// Keys lists the keys a layout uses, each once, in order.
func (l Layout) Keys() []string {
	var keys []string
	seen := map[string]bool{}
	var walk func([]Part)
	walk = func(parts []Part) {
		for _, part := range parts {
			walk(part.Group)
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
	for _, segment := range l {
		walk(segment)
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
