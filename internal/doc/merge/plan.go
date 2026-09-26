package merge

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
	"dgs-toolbox/internal/tag"
	"dgs-toolbox/internal/verifiedcopy"
)

// Change is one Item the merge adds to or changes.
type Change struct {
	// Item is the Item's ID in the tree; From is its ID in the Source, the
	// same for an Item that is new.
	Item   string            `json:"item"`
	From   string            `json:"from"`
	Type   string            `json:"type"`
	Fields map[string]string `json:"fields"`
	// Digests are the revisions added, in the Source's order.
	Digests []string `json:"digests,omitempty"`
	// Head is where HEAD moves, when it does.
	Head string `json:"head,omitempty"`
	// Notes are the notes after the merge, when the Source adds to them:
	// both sides' notes are kept.
	Notes string `json:"notes,omitempty"`
	// Tags add the Source's tags to the matched Item.
	Tags []string `json:"tags,omitempty"`
	// Frequent marks the matched Item frequent because the Source's is.
	Frequent bool `json:"frequent,omitempty"`
}

// Conflict kinds.
const (
	ConflictFields   = "fields"
	ConflictHead     = "head"
	ConflictTemplate = "template"
	ConflictView     = "view"
)

// Conflict is something both sides hold differently. Ours is the tree's,
// Theirs the Source's; resolving picks one.
type Conflict struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Item   string `json:"item,omitempty"`
	From   string `json:"from,omitempty"`
	Type   string `json:"type,omitempty"`
	Name   string `json:"name,omitempty"`
	Ours   any    `json:"ours"`
	Theirs any    `json:"theirs"`
}

// Choices.
const (
	Ours   = "ours"
	Theirs = "theirs"
)

// Plan is everything a merge would do.
type Plan struct {
	New       []Change   `json:"new"`
	Changed   []Change   `json:"changed"`
	Same      int        `json:"same"`
	Templates []string   `json:"templates"`
	Views     []string   `json:"views"`
	Conflicts []Conflict `json:"conflicts"`
	// Problems are what no choice resolves; Apply refuses a plan with any.
	Problems []string `json:"problems"`
}

func hasDigest(item tree.Item, digest string) bool {
	for _, r := range item.Revisions {
		if r.Digest == digest {
			return true
		}
	}
	return false
}

// Compute is what merging src into the tree at root would do.
func Compute(root string, src Source) (Plan, error) {
	plan := Plan{New: []Change{}, Changed: []Change{}, Templates: []string{}, Views: []string{}, Conflicts: []Conflict{}, Problems: []string{}}
	if err := tree.Require(root); err != nil {
		return plan, err
	}
	items, err := tree.LoadItems(root)
	if err != nil {
		return plan, err
	}
	ours, err := tree.LoadTemplates(root)
	if err != nil {
		return plan, err
	}
	templates := map[string]tree.Template{}
	for _, t := range ours {
		templates[t.Type] = t
	}
	for _, t := range src.Templates {
		mine, ok := templates[t.Type]
		switch {
		case !ok:
			plan.Templates = append(plan.Templates, t.Type)
			templates[t.Type] = t.Template
		case !reflect.DeepEqual(mine, t.Template):
			plan.Conflicts = append(plan.Conflicts, Conflict{ID: "template:" + t.Type, Kind: ConflictTemplate, Type: t.Type, Ours: mine, Theirs: t.Template})
		}
	}
	views, err := view.Load(root)
	if err != nil {
		return plan, err
	}
	for _, v := range src.Views {
		var mine *view.View
		for i := range views {
			if views[i].Name == v.Name {
				mine = &views[i]
			}
		}
		switch {
		case mine == nil:
			plan.Views = append(plan.Views, v.Name)
		case !reflect.DeepEqual(*mine, v):
			plan.Conflicts = append(plan.Conflicts, Conflict{ID: "view:" + v.Name, Kind: ConflictView, Name: v.Name, Ours: *mine, Theirs: v})
		}
	}

	byID := map[string]tree.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	claimed := map[string]string{}
	for _, s := range src.Items {
		t, ok := templates[s.Type]
		if !ok {
			plan.Problems = append(plan.Problems, fmt.Sprintf("%s %s: no Template for type %s on either side", s.Type, s.ID, s.Type))
			continue
		}
		m, found := match(t, items, byID, s)
		if !found {
			plan.New = append(plan.New, Change{Item: s.ID, From: s.ID, Type: s.Type, Fields: s.Fields, Digests: digests(s.Revisions), Head: s.Head})
			continue
		}
		if other, ok := claimed[m.ID]; ok {
			plan.Problems = append(plan.Problems, fmt.Sprintf("%s %s and %s both match %s", s.Type, other, s.ID, m.ID))
			continue
		}
		claimed[m.ID] = s.ID
		if m.Type != s.Type || m.Kind != s.Kind {
			plan.Problems = append(plan.Problems, fmt.Sprintf("%s %s is a %s %s here", s.Type, s.ID, m.Kind, m.Type))
			continue
		}
		c := Change{Item: m.ID, From: s.ID, Type: s.Type, Fields: m.Fields}
		conflicts := len(plan.Conflicts)
		for _, r := range s.Revisions {
			if !hasDigest(m, r.Digest) {
				c.Digests = append(c.Digests, r.Digest)
			}
		}
		if s.Kind == tree.KindRecord && len(c.Digests) > 0 {
			plan.Problems = append(plan.Problems, fmt.Sprintf("record %s %s holds another PDF here", s.Type, s.ID))
			continue
		}
		if !maps.Equal(m.Fields, s.Fields) {
			plan.Conflicts = append(plan.Conflicts, Conflict{ID: "fields:" + m.ID, Kind: ConflictFields, Item: m.ID, From: s.ID, Type: s.Type, Ours: m.Fields, Theirs: s.Fields})
		}
		if s.Kind == tree.KindDocument {
			head, conflict := heads(m, s)
			if conflict {
				plan.Conflicts = append(plan.Conflicts, Conflict{ID: "head:" + m.ID, Kind: ConflictHead, Item: m.ID, From: s.ID, Type: s.Type, Ours: m.Current(), Theirs: s.Current()})
			} else if head != m.Current() {
				c.Head = head
			}
		}
		if s.Notes != "" && !strings.Contains(m.Notes, s.Notes) {
			c.Notes = strings.TrimSpace(m.Notes + "\n\n" + s.Notes)
		}
		for _, tag := range s.Tags {
			if !slices.Contains(m.Tags, tag) {
				c.Tags = append(c.Tags, tag)
			}
		}
		c.Frequent = s.Frequent && !m.Frequent
		if len(c.Digests) > 0 || c.Head != "" || c.Notes != "" || len(c.Tags) > 0 || c.Frequent {
			plan.Changed = append(plan.Changed, c)
		} else if len(plan.Conflicts) == conflicts {
			plan.Same++
		}
	}
	return plan, nil
}

// match finds the tree's Item s stands for: the same ID; for a record, one
// holding the same PDF; for a document, the same type and distinguishing
// fields.
func match(t tree.Template, items []tree.Item, byID map[string]tree.Item, s tree.Item) (tree.Item, bool) {
	if m, ok := byID[s.ID]; ok {
		return m, true
	}
	for _, m := range items {
		if m.Type != s.Type || m.Kind != s.Kind {
			continue
		}
		if s.Kind == tree.KindRecord {
			for _, r := range s.Revisions {
				if hasDigest(m, r.Digest) {
					return m, true
				}
			}
		} else if tree.SameDistinguishing(t, m.Fields, s.Fields) {
			return m, true
		}
	}
	return tree.Item{}, false
}

// heads is where a matched document's HEAD goes. A side whose HEAD the other
// has never seen moved it; when only one side did, that one wins. When both
// did and they share a revision, so each could have seen the other's, or both
// point at revisions both hold but differ, it is a conflict. A sub-tree made
// on its own shares nothing: its HEAD is a new revision, and HEAD moves to it.
func heads(ours, theirs tree.Item) (string, bool) {
	o, t := ours.Current(), theirs.Current()
	if o == t {
		return o, false
	}
	theirsMoved := !hasDigest(ours, t)
	oursMoved := !hasDigest(theirs, o)
	switch {
	case theirsMoved && !oursMoved:
		return t, false
	case oursMoved && !theirsMoved:
		return o, false
	case theirsMoved && oursMoved && !shared(ours, theirs):
		return t, false
	}
	return "", true
}

func shared(a, b tree.Item) bool {
	for _, r := range a.Revisions {
		if hasDigest(b, r.Digest) {
			return true
		}
	}
	return false
}

func digests(revs []tree.Revision) []string {
	out := make([]string, len(revs))
	for i, r := range revs {
		out[i] = r.Digest
	}
	return out
}

// Result is what Apply did.
type Result struct {
	Items     int `json:"items"`
	PDFs      int `json:"pdfs"`
	Templates int `json:"templates"`
	Views     int `json:"views"`
}

// Apply carries out plan with a choice for every Conflict. PDFs are copied
// with verifiedcopy before the sidecar naming them is written, so a sidecar
// never names a PDF that is not there. Templates come first, so an Item is
// never written before its type is known.
func Apply(ctx context.Context, root string, src Source, plan Plan, choices map[string]string, now time.Time) (Result, error) {
	var result Result
	if len(plan.Problems) > 0 {
		return result, errors.New("the merge has problems no choice resolves; nothing was changed")
	}
	chosen := map[string]string{}
	for _, c := range plan.Conflicts {
		choice := choices[c.ID]
		if choice != Ours && choice != Theirs {
			return result, fmt.Errorf("no choice for %s; nothing was changed", c.ID)
		}
		chosen[c.ID] = choice
	}
	srcTemplates := map[string]Template{}
	for _, t := range src.Templates {
		srcTemplates[t.Type] = t
	}
	srcViews := map[string]view.View{}
	for _, v := range src.Views {
		srcViews[v.Name] = v
	}
	srcItems := map[string]tree.Item{}
	for _, it := range src.Items {
		srcItems[it.ID] = it
	}

	writeTemplates := append([]string(nil), plan.Templates...)
	writeViews := append([]string(nil), plan.Views...)
	for _, c := range plan.Conflicts {
		if chosen[c.ID] != Theirs {
			continue
		}
		switch c.Kind {
		case ConflictTemplate:
			writeTemplates = append(writeTemplates, c.Type)
		case ConflictView:
			writeViews = append(writeViews, c.Name)
		}
	}
	for _, name := range writeTemplates {
		if err := writeFile(filepath.Join(root, tree.TemplatesDir, name+".yaml"), srcTemplates[name].Data); err != nil {
			return result, err
		}
		result.Templates++
	}
	for _, name := range writeViews {
		if err := view.Save(root, srcViews[name]); err != nil {
			return result, err
		}
		result.Views++
	}

	for _, c := range plan.New {
		s := srcItems[c.From]
		for _, d := range c.Digests {
			if err := copyPDF(ctx, src.PDF(s.ID, d), tree.PDFPath(root, c.Item, d), d, &result); err != nil {
				return result, err
			}
		}
		item := s
		item.Revisions = append([]tree.Revision(nil), s.Revisions...)
		// The Item keeps the history it had in the other tree, and says when
		// it came into this one.
		item.History = append(append([]tree.HistoryEvent(nil), s.History...),
			tree.HistoryEvent{At: now.Format(time.RFC3339), Action: "merge"})
		if err := tree.WriteItem(root, item); err != nil {
			return result, err
		}
		result.Items++
	}

	// Matched Items: new revisions, a moved HEAD, and chosen fields and HEADs.
	touched := map[string]*tree.Item{}
	load := func(id string) (*tree.Item, error) {
		if it, ok := touched[id]; ok {
			return it, nil
		}
		it, _, err := tree.FindItem(root, id)
		if err != nil {
			return nil, err
		}
		touched[id] = &it
		return &it, nil
	}
	var order []string
	// changes is what the merge did to each matched Item, recorded as one
	// merge event in its history.
	changes := map[string]map[string][2]string{}
	change := func(id, key, old, next string) {
		if changes[id] == nil {
			changes[id] = map[string][2]string{}
		}
		changes[id][key] = [2]string{old, next}
	}
	for _, c := range plan.Changed {
		it, err := load(c.Item)
		if err != nil {
			return result, err
		}
		order = append(order, c.Item)
		s := srcItems[c.From]
		if len(c.Digests) > 0 {
			change(c.Item, "revisions", "", strings.Join(c.Digests, ", "))
		}
		for _, d := range c.Digests {
			if err := copyPDF(ctx, src.PDF(s.ID, d), tree.PDFPath(root, c.Item, d), d, &result); err != nil {
				return result, err
			}
			for _, r := range s.Revisions {
				if r.Digest == d {
					it.Revisions = append(it.Revisions, r)
				}
			}
		}
		if c.Head != "" && c.Head != it.Head {
			change(c.Item, "head", it.Head, c.Head)
			it.Head = c.Head
		}
		if c.Notes != "" && c.Notes != it.Notes {
			change(c.Item, "notes", it.Notes, c.Notes)
			it.Notes = c.Notes
		}
		if tags := tag.List(append(it.Tags, c.Tags...)); !slices.Equal(tags, it.Tags) {
			change(c.Item, "tags", strings.Join(it.Tags, ", "), strings.Join(tags, ", "))
			it.Tags = tags
		}
		if c.Frequent && !it.Frequent {
			change(c.Item, "frequent", "", "yes")
			it.Frequent = true
		}
	}
	for _, c := range plan.Conflicts {
		if chosen[c.ID] != Theirs || (c.Kind != ConflictFields && c.Kind != ConflictHead) {
			continue
		}
		it, err := load(c.Item)
		if err != nil {
			return result, err
		}
		order = append(order, c.Item)
		s := srcItems[c.From]
		if c.Kind == ConflictFields {
			for key, value := range s.Fields {
				if it.Fields[key] != value {
					change(c.Item, key, it.Fields[key], value)
				}
			}
			for key, value := range it.Fields {
				if _, ok := s.Fields[key]; !ok {
					change(c.Item, key, value, "")
				}
			}
			it.Fields = maps.Clone(s.Fields)
		} else if head := s.Current(); head != it.Head {
			change(c.Item, "head", it.Head, head)
			it.Head = head
		}
	}
	sort.Strings(order)
	for i, id := range order {
		if i > 0 && order[i-1] == id {
			continue
		}
		if len(changes[id]) > 0 {
			touched[id].History = append(touched[id].History,
				tree.HistoryEvent{At: now.Format(time.RFC3339), Action: "merge", Changes: changes[id]})
		}
		if err := tree.WriteItem(root, *touched[id]); err != nil {
			return result, err
		}
		result.Items++
	}
	return result, nil
}

// copyPDF copies a revision in unless the tree already keeps it there.
func copyPDF(ctx context.Context, from, to, digest string, result *Result) error {
	if have, err := tree.FileDigest(to); err == nil && have == digest {
		return nil
	}
	if from == "" {
		return fmt.Errorf("no PDF for %s", digest)
	}
	got, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{Source: from, Destination: to})
	if err != nil {
		return err
	}
	if got.Digest != digest {
		_ = os.Remove(to)
		return fmt.Errorf("%s does not hash to %s; it changed since it was recorded", from, digest)
	}
	result.PDFs++
	return nil
}

// writeFile replaces path through a temporary file and a rename.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + verifiedcopy.PartSuffix
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
