package view

import (
	"fmt"
	"strings"

	"dgs-toolbox/internal/doc/expr"
	"dgs-toolbox/internal/doc/tree"
)

// What became of one revision in one rule: Explanation.Result.
const (
	Placed      = "placed"
	NotSelected = "not selected"
	NotHead     = "not HEAD"
	LeftOut     = "left out"
	Lacking     = "lacking"
	Clashing    = "clash"
	// Fixed is a file put in as it is, by a Snapshot, not by a rule.
	Fixed = "fixed"
)

// Explanation is how one rule placed one revision of an Item, or did not:
// each step Build took, recorded as Build took it, so it cannot tell a
// different story from the plan.
type Explanation struct {
	Rule     string `json:"rule"`
	Item     string `json:"item"`
	Digest   string `json:"digest"`
	Revision int    `json:"revision"`
	Result   string `json:"result"`
	// If is the rule's own condition asked of the revision; nil when the
	// rule has none. SharedAs is the person it held for, when it held only
	// as an Item shared with them.
	If       *expr.Check `json:"if,omitempty"`
	SharedAs string      `json:"sharedAs,omitempty"`
	// Branches are the children asked, level by level, in order, up to
	// the one at each level that took the revision.
	Branches []Branch `json:"branches,omitempty"`
	// Layout is the path and file the revision was written by, and Steps
	// what each of its keys, groups and {#} wrote.
	Layout string `json:"layout,omitempty"`
	Steps  []Step `json:"steps,omitempty"`
	// Path is where it is, when placed or clashing.
	Path    string   `json:"path,omitempty"`
	Lacking []string `json:"lacking,omitempty"`
	// Clash is the other files wanting Path, or a folder of its name.
	Clash []File `json:"clash,omitempty"`

	placed int
}

// Branch is one child asked: children 2.1 is the first child of the
// second. If is nil for the else.
type Branch struct {
	Where    string      `json:"where"`
	If       *expr.Check `json:"if,omitempty"`
	SharedAs string      `json:"sharedAs,omitempty"`
	Took     bool        `json:"took"`
}

// Step is what one part of a layout wrote, as written in the layout.
type Step struct {
	Part string `json:"part"`
	// Key is the alternative used, or the first when none was there.
	Key string `json:"key,omitempty"`
	// Value is what was written; Raw the value before Clean, when Clean
	// changed it.
	Value string `json:"value,omitempty"`
	Raw   string `json:"raw,omitempty"`
	// From is the link field an inherited key came by.
	From    string `json:"from,omitempty"`
	Lacking bool   `json:"lacking,omitempty"`
	Default bool   `json:"default,omitempty"`
	// Left are the keys an optional group lacked, so wrote nothing; Inner
	// its parts.
	Left  []string `json:"left,omitempty"`
	Inner []Step   `json:"inner,omitempty"`
	// A {#}'s: its order and the name it numbers, the name's place in it,
	// or Unordered when it is not there, Unnumbered when the order leaves
	// it so, Skipped when a key it needs is lacking. OrderAt is the child
	// the order is in, 2.1, or "" for the rule.
	Order      string `json:"order,omitempty"`
	OrderAt    string `json:"orderAt,omitempty"`
	Name       string `json:"name,omitempty"`
	Place      int    `json:"place,omitempty"`
	Unordered  bool   `json:"unordered,omitempty"`
	Unnumbered bool   `json:"unnumbered,omitempty"`
	Skipped    bool   `json:"skipped,omitempty"`
}

// String is p as a layout writes it.
func (p Part) String() string {
	switch {
	case p.Counter:
		return "{#" + p.Label + "}"
	case p.End:
		return "{/#}"
	case len(p.Group) > 0:
		var b strings.Builder
		b.WriteString("[")
		if p.Folder {
			b.WriteString("/")
		}
		for _, q := range p.Group {
			b.WriteString(q.String())
		}
		return b.String() + "]"
	case p.Key == "":
		return p.Text
	}
	var keys []string
	for _, c := range p.choices() {
		if c.Format != "" {
			c.Key += ":" + c.Format
		}
		keys = append(keys, c.Key)
	}
	return "{" + strings.Join(keys, "|") + "}"
}

// clashIn marks x clashing when its file is in one of clashes.
func (x *Explanation) clashIn(clashes []Clash) {
	if x.Result != Placed {
		return
	}
	for _, c := range clashes {
		mine := -1
		for i, f := range c.Files {
			if f.Item == x.Item && f.Digest == x.Digest && f.Path == x.Path && (f.View == "" || f.View == x.Rule) {
				mine = i
			}
		}
		if mine < 0 {
			continue
		}
		x.Result = Clashing
		for i, f := range c.Files {
			if i != mine {
				x.Clash = append(x.Clash, f)
			}
		}
		return
	}
}

// Explain is how v places each revision with a PDF of the Item id, or
// why it does not, built as Build builds v's plan.
func Explain(v View, items []tree.Item, types Types, id string) ([]Explanation, error) {
	_, out, err := build(v, items, types, id)
	return out, err
}

// ExplainWith is Explain for every view, planned together as CombineWith
// plans them: a file placed alone in its rule may still clash with
// another rule's, or with one of fixed. Fixed files of the Item are
// explained too, as Fixed, by what placed them.
func ExplainWith(views []View, fixed []File, items []tree.Item, types Types, id string) ([]Explanation, error) {
	var out []Explanation
	for _, f := range fixed {
		if f.Item == id {
			out = append(out, Explanation{Rule: f.View, Item: id, Digest: f.Digest, Revision: f.Revision, Result: Fixed, Path: f.Path})
		}
	}
	for _, v := range views {
		xs, err := Explain(v, items, types, id)
		if err != nil {
			return nil, fmt.Errorf("view %s: %w", v.Name, err)
		}
		out = append(out, xs...)
	}
	combined, err := CombineWith(views, fixed, items, types)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].clashIn(combined.Clashes)
	}
	return out, nil
}

// Text writes x for reading: what each condition asked and found, the
// branch taken, and what each part of the layout wrote.
func (x Explanation) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s, revision %d: %s", x.Rule, x.Revision, x.Result)
	switch x.Result {
	case Placed, Fixed:
		fmt.Fprintf(&b, "\n  at %s", x.Path)
	case Clashing:
		fmt.Fprintf(&b, "\n  at %s, which these want too:", x.Path)
		for _, f := range x.Clash {
			fmt.Fprintf(&b, "\n    %s  %s, revision %d (%s)", f.Path, f.Item, f.Revision, f.View)
		}
	case Lacking:
		fmt.Fprintf(&b, ": %s", strings.Join(x.Lacking, ", "))
	}
	b.WriteString("\n")
	if x.If != nil {
		writeCheck(&b, "if", *x.If, "  ")
	}
	if x.SharedAs != "" {
		fmt.Fprintf(&b, "  as shared with %s\n", x.SharedAs)
	}
	for _, br := range x.Branches {
		took := map[bool]string{true: "takes it", false: "no"}[br.Took]
		if br.If == nil {
			fmt.Fprintf(&b, "  children %s  else  %s\n", br.Where, took)
			continue
		}
		writeCheck(&b, "children "+br.Where+"  "+took+"  if", *br.If, "  ")
	}
	if x.Layout != "" {
		fmt.Fprintf(&b, "  layout %s\n", x.Layout)
	}
	writeSteps(&b, x.Steps, "    ")
	return b.String()
}

// writeCheck writes c under label, one comparison a line: yes or no, the
// comparison, and what its key holds.
func writeCheck(b *strings.Builder, label string, c expr.Check, indent string) {
	mark := map[bool]string{true: "yes", false: "no "}[c.Met]
	if c.Op == "" {
		if label != "" {
			fmt.Fprintf(b, "%s%s\n", indent, label)
			indent += "  "
		}
		held := "nothing"
		if len(c.Held) > 0 {
			held = strings.Join(c.Held, ", ")
		}
		fmt.Fprintf(b, "%s%s  %s   (holds %s)\n", indent, mark, comparison(c.Node), held)
		return
	}
	not := ""
	if c.Not {
		not = "!"
	}
	if label != "" {
		label += " "
	}
	fmt.Fprintf(b, "%s%s%s  %s(%s)\n", indent, label, mark, not, c.Op)
	for _, p := range c.Parts {
		writeCheck(b, "", p, indent+"  ")
	}
}

// comparison is one comparison as a condition writes it.
func comparison(n expr.Node) string {
	var s string
	switch n.Cmp {
	case "has":
		s = "has(" + n.Key + ")"
	case "in":
		s = n.Key + " in [" + strings.Join(n.Values, ", ") + "]"
	case "==":
		if n.Not {
			return n.Key + " != " + strings.Join(n.Values, ", ")
		}
		s = n.Key + " == " + strings.Join(n.Values, ", ")
	default:
		s = n.Key + " " + n.Cmp + " " + strings.Join(n.Values, ", ")
	}
	if n.Not {
		return "!" + s
	}
	return s
}

// writeSteps writes what each part of a layout wrote, a group's inside
// below it.
func writeSteps(b *strings.Builder, steps []Step, indent string) {
	for _, s := range steps {
		fmt.Fprintf(b, "%s%s  %s\n", indent, s.Part, stepText(s))
		writeSteps(b, s.Inner, indent+"  ")
	}
}

// orderOf names a {#}'s order, and the child it is in when not the rule.
func orderOf(s Step) string {
	if s.OrderAt == "" {
		return s.Order
	}
	return s.Order + " (children " + s.OrderAt + ")"
}

func stepText(s Step) string {
	switch {
	case s.Order != "" && s.Skipped:
		return "not numbered: a key it numbers is lacking"
	case s.Order != "" && s.Unordered:
		return fmt.Sprintf("order %s has no %q", orderOf(s), s.Name)
	case s.Order != "" && s.Unnumbered:
		return fmt.Sprintf("order %s: %q is #%d, left unnumbered", orderOf(s), s.Name, s.Place)
	case s.Order != "":
		return fmt.Sprintf("order %s: %q is #%d, numbered %s", orderOf(s), s.Name, s.Place, s.Value)
	case s.Inner != nil || s.Left != nil:
		if len(s.Left) > 0 {
			return "left out: lacks " + strings.Join(s.Left, ", ")
		}
		return fmt.Sprintf("%q", s.Value)
	case s.Lacking:
		return s.Key + " lacking"
	case s.Default:
		return fmt.Sprintf("%s lacking, the default %q", s.Key, s.Value)
	}
	out := fmt.Sprintf("%s = %q", s.Key, s.Value)
	if s.Raw != "" {
		out += fmt.Sprintf(", cleaned from %q", s.Raw)
	}
	if s.From != "" {
		out += ", inherited from " + s.From
	}
	return out
}
