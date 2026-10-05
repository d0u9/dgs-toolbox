package view

import (
	"dgs-toolbox/internal/doc/country"
	"dgs-toolbox/internal/doc/dates"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"dgs-toolbox/internal/doc/expr"
	"dgs-toolbox/internal/doc/tree"
)

// DateField is the field year and month are derived from, from its start
// when it is a span.
const DateField = "date"

// File is one PDF a View places.
type File struct {
	Path   string `json:"path"`
	Item   string `json:"item"`
	Digest string `json:"digest"`
	// Revision counts from 1, in the order the Item's revisions were added.
	Revision int `json:"revision"`
	// View is the View that placed the file, set when Views are combined.
	View string `json:"view,omitempty"`
	// Node is the rule node that placed it: "" for the rule, 2.1 for its
	// second child's first.
	Node string `json:"node,omitempty"`
}

// Missing is a selected PDF the layout cannot name, and the keys it lacks.
type Missing struct {
	Item     string   `json:"item"`
	Digest   string   `json:"digest"`
	Revision int      `json:"revision"`
	Keys     []string `json:"keys"`
	// Fields are the Item fields that, filled in, supply Keys.
	Fields []string `json:"fields"`
	// Unordered are the orders the Item has a name for that the order
	// does not list: order to name. UnorderedAt is the node each order
	// is in: "" for the rule, 2.1 for its second child's first.
	Unordered   map[string]string `json:"unordered,omitempty"`
	UnorderedAt map[string]string `json:"unorderedAt,omitempty"`
	// Node is the rule node that would place it, as File's.
	Node string `json:"node,omitempty"`
}

// Clash is a path more than one PDF would land on.
type Clash struct {
	Path  string `json:"path"`
	Files []File `json:"files"`
}

// Plan is where a View puts every PDF it selects. It is complete — an export
// may write it — only when Missing and Clashes are both empty.
type Plan struct {
	Files   []File    `json:"files"`
	Missing []Missing `json:"missing"`
	Clashes []Clash   `json:"clashes"`
}

// Complete reports whether nothing stops the plan being written.
func (p Plan) Complete() bool { return len(p.Missing) == 0 && len(p.Clashes) == 0 }

// TagsKey is the key a condition reads a revision's tags from: the Item's
// and the revision's own.
const TagsKey = "tags"

// StatusKey is the key a condition reads where an Item stands from, rather
// than a field: Status names the values.
const StatusKey = "status"

// Status is what item is besides its fields: `superseded` when a later
// Item replaces it, `retired` when it is no longer used, as a superseded
// Item also is.
func Status(item tree.Item) []string {
	var out []string
	if item.SupersededBy != "" {
		out = append(out, "superseded")
	}
	if item.Retired || item.SupersededBy != "" {
		out = append(out, "retired")
	}
	return out
}

// env is what a condition asks of one revision: its keys as a layout has
// them, its tags and the Item's Status.
type env struct {
	keys  map[string]string
	tags  []string
	item  tree.Item
	types Types
}

func (e env) Values(key string) []string {
	switch key {
	case TagsKey:
		return e.tags
	case StatusKey:
		return Status(e.item)
	}
	v := e.keys[key]
	if v == "" {
		return nil
	}
	if f, ok := e.field(key); ok && f.Multiple {
		return strings.Split(v, tree.MultipleSeparator)
	}
	return []string{v}
}

// Is holds for a value equal to want, as sameValue compares; for a type
// below want; and for a value in the group want of its field.
func (e env) Is(key, value, want string) bool {
	if key == TagsKey {
		return strings.EqualFold(value, want)
	}
	if sameValue(value, want) {
		return true
	}
	if isType(key) {
		return e.types.Is(value, want)
	}
	if f, ok := e.field(key); ok {
		group, _ := f.Group(want)
		for _, v := range group {
			if sameValue(v, value) {
				return true
			}
		}
	}
	return false
}

// field is the Template field key reads: an own field of this type, or
// about.category a field of the linked Item's.
func (e env) field(key string) (tree.Field, bool) {
	typ := e.keys["type"]
	if i := strings.LastIndex(key, "."); i >= 0 {
		typ, key = e.keys[key[:i]+".type"], key[i+1:]
	}
	return e.types.Field(typ, key)
}

var isoDate = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})`)

// KeysOf is every key a layout can use for one revision of item, counting
// from 1: its fields as that revision has them, `id`, `type`, `kind`,
// `revision`, `ext`, and `year`, `month` and `date` when DateField holds a
// date.
func KeysOf(item tree.Item, revision int) map[string]string {
	keys := map[string]string{}
	fields := item.Fields
	if revision >= 1 && revision <= len(item.Revisions) {
		fields = item.FieldsAt(item.Revisions[revision-1].Ref())
	}
	for k, v := range fields {
		if v != "" {
			keys[k] = v
		}
	}
	keys["id"] = item.ID
	keys["type"] = item.Type
	if revision >= 1 && revision <= len(item.Revisions) && item.Revisions[revision-1].Type != "" {
		keys["type"] = item.Revisions[revision-1].Type
	}
	keys["kind"] = string(item.Kind)
	keys["revision"] = strconv.Itoa(revision)
	keys["ext"] = "pdf"
	for k, v := range fields {
		if start, end, ok := tree.SplitSpan(v); ok {
			if start != "" {
				keys[k+".start"] = start
			}
			if end != "" {
				keys[k+".end"] = end
			}
		}
	}
	// year and month are the date's start's, or its end's when the span
	// is open at the start.
	if m := isoDate.FindStringSubmatch(strings.TrimPrefix(fields[DateField], tree.SpanSeparator)); m != nil {
		keys["year"], keys["month"] = m[1], m[2]
	}
	return keys
}

// Follow adds, for each key whose value links to another Item — an Item ID,
// or <item-id>@<revision> — that Item's keys under the key's name:
// original.level is the level of the revision original names, and an Item
// ID alone stands for its current revision. A link to an Item not in items
// adds nothing, so a layout using it finds the key missing.
func Follow(keys map[string]string, items map[string]tree.Item) {
	for key, value := range maps.Clone(keys) {
		id, ref, ok := tree.SplitRevisionLink(value)
		if !ok {
			id, ref = value, ""
		}
		linked, found := items[id]
		if !found {
			continue
		}
		if ref == "" {
			ref = linked.Current()
		}
		for i, rev := range linked.Revisions {
			if rev.Ref() != ref && !(rev.ID == "" && rev.Digest == ref) {
				continue
			}
			for k, v := range KeysOf(linked, i+1) {
				keys[key+"."+k] = v
			}
		}
	}
}

// Inherit gives keys, for each key they lack, the value a link field's
// Item has, as Follow added it: with links [original], a translation
// without a level takes original.level. The first link having it wins.
func Inherit(keys map[string]string, links []string) { inherit(keys, links) }

// inherit is Inherit, returning each key it gave and the link it came by.
func inherit(keys map[string]string, links []string) map[string]string {
	from := map[string]string{}
	for _, link := range links {
		for k, v := range maps.Clone(keys) {
			base, ok := strings.CutPrefix(k, link+".")
			if !ok || strings.Contains(base, ".") {
				continue
			}
			if _, has := keys[base]; !has {
				keys[base] = v
				from[base] = link
			}
		}
	}
	return from
}

// FieldsFor is the Item fields that supply keys, in order and once each:
// year and month come from DateField, every other key is a field of
// its own name.
func FieldsFor(keys []string) []string {
	var fields []string
	seen := map[string]bool{}
	for _, k := range keys {
		if k == "year" || k == "month" {
			k = DateField
		}
		// original.level is filled by the field original.
		k, _, _ = strings.Cut(k, ".")
		if strings.HasPrefix(k, "type:") || strings.Contains(k, "|") || strings.HasPrefix(k, "{") {
			// A type's name comes from its Template, and alternatives and
			// a braced order key such as {about.address} name an order,
			// not an Item field.
			continue
		}
		if !seen[k] {
			seen[k] = true
			fields = append(fields, k)
		}
	}
	return fields
}

// Clean makes one key's value safe as part of a file name: /, \, control
// characters and the characters Windows forbids become _, and leading and
// trailing dots and spaces are trimmed, so a value never adds a folder or
// escapes the Target. A value with nothing left is _.
func Clean(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r < 0x20 || r == 0x7f:
			b.WriteByte('_')
		case strings.ContainsRune(`/\:*?"<>|`, r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), ". ")
	if out == "" {
		return "_"
	}
	return out
}

// Types is what a plan needs of the Templates: each type's names by
// language, its lineage and its fields.
type Types struct {
	byType map[string]tree.Template
}

// TypesOf collects the Types of templates.
func TypesOf(templates []tree.Template) Types {
	out := Types{byType: map[string]tree.Template{}}
	for _, t := range templates {
		out.byType[t.Type] = t
	}
	return out
}

// Names is typ's names by language.
func (t Types) Names(typ string) map[string]string { return t.byType[typ].Names }

// nameValues adds, for each select field of keys' type with names, its
// value in each language: keys["category:en"] = "rent".
func (t Types) nameValues(keys map[string]string) {
	for _, f := range t.byType[keys["type"]].Fields {
		v, ok := keys[f.Key]
		if !ok {
			continue
		}
		for lang := range f.Names {
			keys[f.Key+":"+lang] = f.ValueName(v, lang)
		}
	}
}

// Is reports whether typ is want or below it.
func (t Types) Is(typ, want string) bool {
	tpl, ok := t.byType[typ]
	return ok && tpl.Is(want)
}

// Field is typ's field key.
func (t Types) Field(typ, key string) (tree.Field, bool) {
	for _, f := range t.byType[typ].Fields {
		if f.Key == key {
			return f, true
		}
	}
	return tree.Field{}, false
}

// Build computes v's plan over items. Items are taken in ID order and a
// document's revisions in the order they were added, so the same state always
// gives the same plan, numbering included. Paths are compared ignoring case,
// as the file systems a Target usually lives on do.
func Build(v View, items []tree.Item, types Types) (Plan, error) {
	plan, _, err := build(v, items, types, "")
	return plan, err
}

// build is Build, also explaining each revision with a PDF of the Item
// watch as it goes: what it asked, what it chose, and what it wrote.
func build(v View, items []tree.Item, types Types, watch string) (Plan, []Explanation, error) {
	if err := v.Validate(); err != nil {
		return Plan{}, nil, err
	}
	layouts := map[string]Layout{}
	conditions := map[string]expr.Expr{}
	_ = v.walk(func(_ string, n Node, layout string, _ []Node) error {
		if layout != "" {
			layouts[layout], _ = Parse(layout)
		}
		if n.If != "" {
			conditions[n.If] = expr.MustParse(n.If)
		}
		return nil
	})
	// meets reports whether e meets n's If, and the person it was asked
	// as when only an Item shared with them does, with the env that held.
	meets := func(n Node, e env) (bool, string, env) {
		if n.If == "" {
			return true, "", e
		}
		if conditions[n.If].Eval(e) {
			return true, "", e
		}
		if !v.Shared || n.If != v.If {
			return false, "", e
		}
		for _, person := range e.item.SharedWith {
			shared := e
			shared.keys = maps.Clone(e.keys)
			shared.keys["owner"] = person
			if conditions[n.If].Eval(shared) {
				return true, person, shared
			}
		}
		return false, "", e
	}
	check := func(n Node, e env) *expr.Check {
		if n.If == "" {
			return nil
		}
		c := expr.Explain(conditions[n.If], e)
		return &c
	}
	var explained []*Explanation
	sorted := append([]tree.Item(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	byID := make(map[string]tree.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	plan := Plan{Files: []File{}, Missing: []Missing{}, Clashes: []Clash{}}
	var placed []File
	for _, item := range sorted {
		current := item.Current()
		for i, rev := range item.Revisions {
			if rev.Digest == "" {
				continue
			}
			var x *Explanation
			if item.ID == watch {
				x = &Explanation{Rule: v.Name, Item: item.ID, Digest: rev.Digest, Revision: i + 1}
				explained = append(explained, x)
			}
			if v.Selection == Head && rev.Ref() != current {
				if x != nil {
					x.Result = NotHead
				}
				continue
			}
			keys := KeysOf(item, i+1)
			for lang, name := range types.Names(keys["type"]) {
				keys["type:"+lang] = name
			}
			types.nameValues(keys)
			Follow(keys, byID)
			for k, t := range maps.Clone(keys) {
				// original.type:zh, the linked Item's type in a language.
				if strings.HasSuffix(k, ".type") {
					for lang, name := range types.Names(t) {
						keys[k+":"+lang] = name
					}
				}
			}
			inherited := inherit(keys, v.Inherit)
			e := env{keys: keys, tags: item.TagsAt(rev.Ref()), item: item, types: types}
			ok, as, held := meets(v.Node, e)
			if x != nil {
				x.If, x.SharedAs = check(v.Node, held), as
			}
			if !ok {
				if x != nil {
					x.Result = NotSelected
				}
				continue
			}
			// Down the first child met at each level, keeping the path,
			// file and default the deepest node sets.
			node, path, file, fallback := v.Node, []string{}, v.File, v.Default
			scopes := []scope{{v.Node, ""}}
			if v.Path != "" {
				path = append(path, strings.Trim(v.Path, "/"))
			}
			out, where := false, ""
			for {
				next := -1
				for c, child := range node.Children {
					ok, as, held := meets(child, e)
					if x != nil {
						x.Branches = append(x.Branches, Branch{Where: where + strconv.Itoa(c+1), If: check(child, held), SharedAs: as, Took: ok})
					}
					if ok {
						next = c
						break
					}
				}
				if next < 0 {
					break
				}
				node, where = node.Children[next], where+strconv.Itoa(next+1)+"."
				scopes = append(scopes, scope{node, strings.TrimSuffix(where, ".")})
				if node.Out() {
					out = true
					break
				}
				if node.Path != "" {
					path = append(path, strings.Trim(node.Path, "/"))
				}
				if node.File != "" {
					file = node.File
				}
				if node.Default != nil {
					fallback = node.Default
				}
			}
			if out {
				if x != nil {
					x.Result = LeftOut
				}
				continue
			}
			layout := strings.Join(append(path, file), "/")
			r := renderer{keys: keys, fallback: fallback, scopes: scopes}
			if x != nil {
				r.record, r.from, x.Layout = true, inherited, layout
			}
			name := r.render(layouts[layout])
			if x != nil {
				x.Steps = r.steps
			}
			if len(r.lacking) > 0 || len(r.unordered) > 0 {
				plan.Missing = append(plan.Missing, Missing{Item: item.ID, Digest: rev.Ref(), Revision: i + 1, Keys: r.lacking, Fields: FieldsFor(r.lacking), Unordered: r.unordered, UnorderedAt: r.unorderedAt, Node: strings.TrimSuffix(where, ".")})
				if x != nil {
					x.Result, x.Lacking = Lacking, slices.Clone(r.lacking)
					for _, order := range slices.Sorted(maps.Keys(r.unordered)) {
						x.Lacking = append(x.Lacking, "a number for "+r.unordered[order]+" in order "+order)
					}
				}
				continue
			}
			if x != nil {
				x.Result, x.placed = Placed, len(placed)
			}
			placed = append(placed, File{Path: name, Item: item.ID, Digest: rev.Digest, Revision: i + 1, Node: strings.TrimSuffix(where, ".")})
		}
	}

	groups := map[string][]int{}
	var order []string
	for i, f := range placed {
		k := strings.ToLower(f.Path)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}
	if v.Dedupe == DedupeNumber {
		for _, k := range order {
			for n, i := range groups[k][1:] {
				placed[i].Path = numbered(placed[i].Path, n+1)
			}
		}
	}
	out := make([]Explanation, 0, len(explained))
	for _, x := range explained {
		if x.Result == Placed {
			x.Path = placed[x.placed].Path
		}
	}
	plan.Files, plan.Clashes = separate(placed)
	sort.Slice(plan.Files, func(i, j int) bool { return plan.Files[i].Path < plan.Files[j].Path })
	for _, x := range explained {
		x.clashIn(plan.Clashes)
		out = append(out, *x)
	}
	return plan, out, nil
}

// separate splits files into those with a path of their own and the
// clashes: paths more than one file wants, ignoring case, and a file where
// another needs a folder of the same name.
func separate(files []File) ([]File, []Clash) {
	groups := map[string][]int{}
	var order []string
	for i, f := range files {
		k := strings.ToLower(f.Path)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}
	ok := []File{}
	clashes := []Clash{}
	for _, k := range order {
		members := groups[k]
		// A file whose path is a folder above another file's clashes too.
		var under []int
		for _, other := range order {
			if strings.HasPrefix(other, k+"/") {
				under = append(under, groups[other]...)
			}
		}
		if len(members) == 1 && len(under) == 0 {
			ok = append(ok, files[members[0]])
			continue
		}
		clash := Clash{Path: files[members[0]].Path}
		for _, i := range append(members, under...) {
			clash.Files = append(clash.Files, files[i])
		}
		clashes = append(clashes, clash)
	}
	// A file under a clashing folder is only reported with it.
	blocked := map[string]bool{}
	for _, c := range clashes {
		for _, f := range c.Files {
			blocked[strings.ToLower(f.Path)] = true
		}
	}
	kept := ok[:0]
	for _, f := range ok {
		if !blocked[strings.ToLower(f.Path)] {
			kept = append(kept, f)
		}
	}
	return kept, clashes
}

// Combined is several Views planned into one Target together.
type Combined struct {
	// Plans are each View's own plan, by View name, their files marked with
	// the View.
	Plans map[string]Plan `json:"plans"`
	// Files are every View's files that have a path of their own.
	Files []File `json:"files"`
	// Clashes are paths wanted by files of different Views.
	Clashes []Clash `json:"clashes"`
}

// Complete reports whether every View is complete and no two Views want
// one path.
func (c Combined) Complete() bool {
	for _, p := range c.Plans {
		if !p.Complete() {
			return false
		}
	}
	return len(c.Clashes) == 0
}

// Combine plans views into one Target: each is built alone, then their files
// are put together and checked against each other as Build checks one View.
func Combine(views []View, items []tree.Item, types Types) (Combined, error) {
	return CombineWith(views, nil, items, types)
}

// CombineWith is Combine with fixed files beside the views', each already
// at its path and marked with what placed it, checked against them alike.
func CombineWith(views []View, fixed []File, items []tree.Item, types Types) (Combined, error) {
	out := Combined{Plans: map[string]Plan{}, Files: []File{}, Clashes: []Clash{}}
	all := append([]File{}, fixed...)
	for _, v := range views {
		p, err := Build(v, items, types)
		if err != nil {
			return out, fmt.Errorf("view %s: %w", v.Name, err)
		}
		for _, list := range [][]File{p.Files} {
			for i := range list {
				list[i].View = v.Name
			}
		}
		for i := range p.Clashes {
			for j := range p.Clashes[i].Files {
				p.Clashes[i].Files[j].View = v.Name
			}
		}
		out.Plans[v.Name] = p
		all = append(all, p.Files...)
	}
	out.Files, out.Clashes = separate(all)
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	return out, nil
}

// render fills a layout that numbers nothing from keys. It returns the
// keys it lacked, when there is no default to stand in for them.
func render(layout Layout, keys map[string]string, fallback *string) (string, []string) {
	r := renderer{keys: keys, fallback: fallback}
	return r.render(layout), r.lacking
}

// scope is a node a {#} may take its order from, and where it is: "" for
// the rule, 2.1 for its second child's first.
type scope struct {
	Node
	at string
}

// order is the nearest of r's scopes to list the order named, the deepest
// first.
func (r *renderer) order(name string) (scope, bool) {
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if len(r.scopes[i].Order[name]) > 0 {
			return r.scopes[i], true
		}
	}
	return scope{}, false
}

// render fills layout from r's keys, keeping in r what it lacked.
func (r *renderer) render(layout Layout) string {
	segments := make([]string, len(layout))
	for s, parts := range layout {
		segments[s], _ = r.name(parts, false)
		if segments[s] == "" && allGroups(parts) {
			// Only groups, all left out: the folder would vanish and the
			// path change without anyone asking, so it is lacking.
			for _, part := range parts {
				for _, p := range part.Group {
					if p.Key != "" {
						r.lack(p.choices()[0].Key)
						break
					}
				}
			}
		}
	}
	return strings.Join(segments, "/")
}

// renderer is one render's keys, orders, and what it found lacking.
type renderer struct {
	keys     map[string]string
	fallback *string
	// scopes are the nodes from the rule down to the one placing, where
	// each {#} looks for its order.
	scopes  []scope
	lacking []string
	// unordered is an order the name it made is not in, to the name, and
	// unorderedAt the node each order is in.
	unordered   map[string]string
	unorderedAt map[string]string
	// record keeps in steps what each part wrote; from is the keys
	// Inherit gave, by the link they came by.
	record bool
	from   map[string]string
	steps  []Step
}

func (r *renderer) lack(key string) {
	if !contains(r.lacking, key) {
		r.lacking = append(r.lacking, key)
	}
}

// name writes one folder or file name, or a group's inside. A key the
// Item lacks is lacking, or written as the default; in a group it is
// returned as absent instead, and the group is left out.
func (r *renderer) name(parts []Part, group bool) (string, []string) {
	pieces := make([]string, len(parts))
	steps := make([]*Step, len(parts))
	var absent []string
	counter, short := -1, len(r.lacking)
	for i, part := range parts {
		switch {
		case part.Counter:
			counter = i
		case len(part.Group) > 0:
			outer := r.steps
			r.steps = nil
			text, missing := r.name(part.Group, true)
			inner := r.steps
			r.steps = outer
			if len(missing) == 0 && part.Folder {
				pieces[i] = "/" + text
			} else if len(missing) == 0 {
				pieces[i] = text
			}
			steps[i] = &Step{Value: pieces[i], Left: missing, Inner: inner}
		case part.Key == "":
			pieces[i] = part.Text
		default:
			choice, value, ok := pick(part, r.keys)
			step := &Step{Key: choice.Key, From: r.from[choice.Key]}
			steps[i] = step
			if !ok {
				step.Lacking = true
				if group {
					absent = append(absent, choice.Key)
					continue
				}
				if r.fallback == nil {
					r.lack(choice.Key)
					continue
				}
				value, step.Lacking, step.Default = *r.fallback, false, true
			}
			pieces[i] = Clean(value)
			step.Value = pieces[i]
			if value != pieces[i] {
				step.Raw = value
			}
		}
	}
	if counter >= 0 {
		c := parts[counter]
		s, _ := r.order(c.Of)
		step := &Step{Order: c.Of, OrderAt: s.at, Skipped: true}
		steps[counter] = step
		if len(absent) == 0 && len(r.lacking) == short {
			name := strings.Join(pieces[c.From:c.To], "")
			step.Name, step.Skipped = name, false
			if n := place(s.Order[c.Of], name); n > 0 {
				step.Place = n
				numbers := Numbered(s.Order[c.Of], s.Numbers[c.Of], s.Unnumbered[c.Of])
				if numbers[n-1] == Unnumbered {
					step.Unnumbered = true
					// No number, nor the text straight after it: 物业发票.pdf.
					if counter+1 < c.From {
						pieces[counter+1] = ""
					}
				} else {
					widest := slices.Max(numbers)
					if unlisted, ok := UnlistedNumber(s.Unlisted[c.Of]); ok {
						widest = max(widest, unlisted)
					}
					pieces[counter] = padTo(numbers[n-1], widest)
					step.Value = pieces[counter]
				}
			} else if number, ok := UnlistedNumber(s.Unlisted[c.Of]); ok && s.Unlisted[c.Of] != "" {
				// Not listed, and the order says what such names get.
				if number == Unnumbered {
					step.Unnumbered = true
					if counter+1 < c.From {
						pieces[counter+1] = ""
					}
				} else {
					numbers := Numbered(s.Order[c.Of], s.Numbers[c.Of], s.Unnumbered[c.Of])
					pieces[counter] = padTo(number, max(number, slices.Max(numbers)))
					step.Value = pieces[counter]
				}
			} else {
				step.Unordered = true
				if r.unordered == nil {
					r.unordered, r.unorderedAt = map[string]string{}, map[string]string{}
				}
				r.unordered[c.Of], r.unorderedAt[c.Of] = name, s.at
			}
		}
	}
	if r.record {
		for i, s := range steps {
			if s != nil {
				s.Part = parts[i].String()
				r.steps = append(r.steps, *s)
			}
		}
	}
	return strings.Join(pieces, ""), absent
}

// allGroups reports whether every part is a group.
func allGroups(parts []Part) bool {
	for _, p := range parts {
		if len(p.Group) == 0 {
			return false
		}
	}
	return true
}

// pick is the first of part's choices an Item has, with its value as
// written. When it has none, it is the first choice, and ok is false.
func pick(part Part, keys map[string]string) (choice Part, value string, ok bool) {
	for _, c := range part.choices() {
		key := c.Key
		if isType(key) && c.Format != "" {
			// The type's name in a language, from its Template:
			// type:zh, or original.type:zh for a linked Item's.
			key += ":" + c.Format
		}
		value, ok := keys[key]
		if !ok {
			continue
		}
		start, end, span := tree.SplitSpan(value)
		span = span && strings.Contains(value, tree.SpanSeparator)
		if span && (start == "" || end == "") {
			// A span open at one end is written as the day it has.
			value, span = start+end, false
		}
		switch {
		case c.Format == DateCompact && span:
			return c, dates.Compact(start) + "-" + dates.Compact(end), true
		case c.Format == DateCompact:
			return c, dates.Compact(value), true
		case c.Format == DateFinancialYear:
			return c, dates.FinancialYear(start, dates.DefaultYearStart), true
		case span:
			return c, start + "-" + end, true
		}
		if named, ok := keys[key+":"+c.Format]; ok && c.Format != "" && !isType(c.Key) {
			// A select value in a language, from its field's names.
			return c, named, true
		}
		if c.Format != "" && !isType(c.Key) {
			// A value that names no country is written as it is.
			if kept, ok := country.Normalize(value, country.Format(c.Format)); ok {
				value = kept
			}
		}
		return c, value, true
	}
	first := part.choices()[0]
	if isType(first.Key) && first.Format != "" {
		first.Key += ":" + first.Format
	}
	return first, "", false
}

// A date key's formats in a layout: {start:compact} writes 20100101,
// {date:fy} the financial year, FY2024.
const (
	DateCompact       = "compact"
	DateFinancialYear = "fy"
)

func isDateFormat(format string) bool { return format == DateCompact || format == DateFinancialYear }

// isType reports whether key is an Item's type, its own or a linked one's:
// type, original.type.
func isType(key string) bool {
	return key == "type" || strings.HasSuffix(key, ".type")
}

// numbered puts _NN before the file name's extension.
func numbered(p string, n int) string {
	dir, file := path.Split(p)
	ext := path.Ext(file)
	if ext == file {
		ext = ""
	}
	return dir + strings.TrimSuffix(file, ext) + "_" + pad(n) + ext
}

func pad(n int) string { return padTo(n, 0) }

// padTo writes n with at least two digits, and as many as count has.
func padTo(n, largest int) string {
	width := max(2, len(strconv.Itoa(largest)))
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// Unnumbered is the number Numbered gives a name {#} leaves unnumbered.
const Unnumbered = -1

// Numbered is the number {#} writes for each name of order: the next
// after the one before, counting from 1, unless set gives the name its
// own, from which the names after it count on. A name skip lists is
// Unnumbered and takes no number.
func Numbered(order []string, set map[string]int, skip []string) []int {
	out := make([]int, len(order))
	n := 0
	for i, name := range order {
		if place(skip, name) > 0 {
			out[i] = Unnumbered
			continue
		}
		n++
		for named, m := range set {
			if sameValue(named, name) {
				n = m
				break
			}
		}
		out[i] = n
	}
	return out
}

// place is value's position in order, counting from 1, or 0 when it is not
// there.
func place(order []string, value string) int {
	for i, v := range order {
		if sameValue(v, value) {
			return i + 1
		}
	}
	return 0
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Name renders layout for one revision of item, counting from 1, as Build
// would for a rule with no default: the path, and the keys the Item lacks.
// A layout for one PDF numbers nothing, so {#} is refused.
func Name(layout string, item tree.Item, revision int, types Types) (string, []string, error) {
	parsed, err := Parse(layout)
	if err != nil {
		return "", nil, err
	}
	if len(parsed.Counters()) > 0 {
		return "", nil, fmt.Errorf("layout %q: {#} numbers a rule's PDFs, not one PDF's name", layout)
	}
	keys := KeysOf(item, revision)
	for lang, name := range types.Names(keys["type"]) {
		keys["type:"+lang] = name
	}
	types.nameValues(keys)
	name, lacking := render(parsed, keys, nil)
	return name, lacking, nil
}
