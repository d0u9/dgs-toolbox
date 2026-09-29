package tree

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Shape is what a date field holds: one day, or a span of two.
type Shape string

const (
	ShapeDay  Shape = "day"
	ShapeSpan Shape = "span"
)

// Shapes is the shapes a date field allows. It is written as one shape or a
// list of them; left empty, a date field is one day.
type Shapes []Shape

func (s *Shapes) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*s = Shapes{Shape(node.Value)}
		return nil
	}
	var list []Shape
	if err := node.Decode(&list); err != nil {
		return err
	}
	*s = list
	return nil
}

// allows reports whether a value of shape suits the field. An empty set is
// one day.
func (s Shapes) allows(shape Shape) bool {
	if len(s) == 0 {
		return shape == ShapeDay
	}
	return slices.Contains(s, shape)
}

// SpanSeparator joins a span's two days: 2025-01-01/2025-12-31. An end not
// known is left empty, 2025-01-01/ or /2030-03-01, but not both.
const SpanSeparator = "/"

// SplitSpan reads a date field's value as its start and end: a span's two
// days, or one day as both. ok is false for anything else.
func SplitSpan(value string) (start, end string, ok bool) {
	if a, b, found := strings.Cut(value, SpanSeparator); found {
		if a == "" && b == "" || a != "" && !validDate(a) || b != "" && !validDate(b) || a != "" && b != "" && b < a {
			return "", "", false
		}
		return a, b, true
	}
	if !validDate(value) {
		return "", "", false
	}
	return value, value, true
}

// ValueGroup is a name for some of a select field's values: utility for
// 水, 电 and 气. A condition on the group matches each value in it.
type ValueGroup struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// ValueGroups keeps the groups in the order they are written.
type ValueGroups []ValueGroup

func (g *ValueGroups) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: values maps each group to its values", node.Line)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		var values []string
		if err := node.Content[i+1].Decode(&values); err != nil {
			return err
		}
		*g = append(*g, ValueGroup{Name: node.Content[i].Value, Values: values})
	}
	return nil
}

// Group answers the values a group holds, when name is one of the field's
// groups.
func (f Field) Group(name string) ([]string, bool) {
	for _, g := range f.Values {
		if g.Name == name {
			return g.Values, true
		}
	}
	return nil, false
}

// Is reports whether t is typ or descends from it.
func (t Template) Is(typ string) bool {
	return slices.Contains(t.Lineage, typ)
}

// Resolve gives each Template the fields, defaults and ignored dates of the
// types it extends, and validates the result. A type may add fields, and of
// an inherited one make it required or distinguishing, narrow a date's shape
// and give its own description and patterns; nothing else. Names,
// description, kind and anchor are a type's own.
func Resolve(raw []Template) ([]Template, error) {
	byType := map[string]Template{}
	for _, t := range raw {
		if _, dup := byType[t.Type]; dup {
			return nil, fmt.Errorf("type %s is defined twice", t.Type)
		}
		byType[t.Type] = t
	}
	done := map[string]Template{}
	var resolve func(typ string, seen []string) (Template, error)
	resolve = func(typ string, seen []string) (Template, error) {
		if t, ok := done[typ]; ok {
			return t, nil
		}
		if slices.Contains(seen, typ) {
			return Template{}, fmt.Errorf("type %s extends itself: %s", typ, strings.Join(append(seen, typ), " > "))
		}
		t := byType[typ]
		t.Lineage = []string{t.Type}
		if t.Extends != "" {
			if _, ok := byType[t.Extends]; !ok {
				return Template{}, fmt.Errorf("type %s extends %s, which is no Template", t.Type, t.Extends)
			}
			parent, err := resolve(t.Extends, append(seen, typ))
			if err != nil {
				return Template{}, err
			}
			if t, err = inherit(parent, t); err != nil {
				return Template{}, err
			}
		}
		if err := t.Validate(); err != nil {
			return Template{}, err
		}
		done[typ] = t
		return t, nil
	}
	out := make([]Template, 0, len(raw))
	for _, t := range raw {
		r, err := resolve(t.Type, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out, nil
}

// inherit is child with parent's fields first, then its own new ones.
func inherit(parent, child Template) (Template, error) {
	own := map[string]Field{}
	for _, f := range child.Fields {
		own[f.Key] = f
	}
	var fields []Field
	for _, p := range parent.Fields {
		f := p
		if c, ok := own[p.Key]; ok {
			var err error
			if f, err = override(child.Type, p, c); err != nil {
				return Template{}, err
			}
			delete(own, p.Key)
		}
		fields = append(fields, f)
	}
	for _, c := range child.Fields {
		if _, ok := own[c.Key]; ok {
			fields = append(fields, c)
		}
	}
	child.Fields = fields
	defaults := map[string]string{}
	for k, v := range parent.Defaults {
		defaults[k] = v
	}
	for k, v := range child.Defaults {
		defaults[k] = v
	}
	if len(defaults) > 0 {
		child.Defaults = defaults
	}
	child.IgnoreDates = append(slices.Clone(parent.IgnoreDates), child.IgnoreDates...)
	child.Lineage = append([]string{child.Type}, parent.Lineage...)
	return child, nil
}

// override is the inherited field p as child type typ narrows it with c.
func override(typ string, p, c Field) (Field, error) {
	rest := c
	rest.Required, rest.Distinguishing, rest.Description = false, false, ""
	rest.Shape, rest.Pattern, rest.Patterns = nil, "", nil
	if rest.Type == p.Type {
		rest.Type = ""
	}
	if !reflect.DeepEqual(rest, Field{Key: c.Key}) {
		return Field{}, fmt.Errorf("type %s: key %s is inherited: it may be made required or distinguishing, narrowed in shape and described, not changed", typ, c.Key)
	}
	f := p
	f.Required = p.Required || c.Required
	f.Distinguishing = p.Distinguishing || c.Distinguishing
	if c.Description != "" {
		f.Description = c.Description
	}
	if c.Pattern != "" || len(c.Patterns) > 0 {
		f.Pattern, f.Patterns = c.Pattern, c.Patterns
	}
	if len(c.Shape) > 0 {
		for _, s := range c.Shape {
			if !p.Shape.allows(s) {
				return Field{}, fmt.Errorf("type %s: key %s: shape %s is not one its parent allows", typ, c.Key, s)
			}
		}
		f.Shape = c.Shape
	}
	return f, nil
}

// cleanDate checks a date field's value against its shapes.
func (f Field) cleanDate(value string) (string, error) {
	start, end, ok := SplitSpan(value)
	span := strings.Contains(value, SpanSeparator)
	switch {
	case !ok && span:
		return "", fmt.Errorf("%s: %q is not a span written YYYY-MM-DD/YYYY-MM-DD, its end not before its start", f.Key, value)
	case !ok:
		return "", fmt.Errorf("%s: %q is not a date written YYYY-MM-DD", f.Key, value)
	case span && !f.Shape.allows(ShapeSpan):
		return "", fmt.Errorf("%s: %q is a span, and the field holds one day", f.Key, value)
	case !span && !f.Shape.allows(ShapeDay):
		return "", fmt.Errorf("%s: %q is one day, and the field holds a span: start/end", f.Key, value)
	}
	if span {
		return start + SpanSeparator + end, nil
	}
	return start, nil
}
