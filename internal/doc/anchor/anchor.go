// Package anchor finds the Item a link field should point to: a bill's
// tenancy, a payslip's employment. An anchor is an Item, often with no PDF,
// that other documents link to; a rule then places them by its fields. It
// proposes; a person, or an action they confirm, writes the link.
package anchor

import (
	"regexp"
	"sort"

	"dgs-toolbox/internal/doc/tree"
)

// Offered is the Items field f of an Item may link to, given the form's
// fields: every Item but self that agrees with f's match. anchors are the
// anchor types, tree.AnchorTypes, for a match on tree.AnchorKey.
func Offered(f tree.Field, self string, fields map[string]string, items []tree.Item, anchors map[string]bool) []tree.Item {
	var out []tree.Item
	for _, it := range items {
		if it.ID == self || !agrees(f, fields, it, anchors) {
			continue
		}
		out = append(out, it)
	}
	return out
}

func agrees(f tree.Field, fields map[string]string, it tree.Item, anchors map[string]bool) bool {
	for key, ours := range f.Match {
		if want := tree.MatchValue(ours, fields); want != "" && tree.Theirs(key, it, anchors) != want {
			return false
		}
	}
	return true
}

// Suggested is those of Offered whose span, by f's within, holds the form's
// date, or its start when it is a span. It is empty when f has no within or
// the form no date.
func Suggested(f tree.Field, self string, fields map[string]string, items []tree.Item, anchors map[string]bool) []tree.Item {
	w := f.Within
	if w == nil || fields[w.Date] == "" {
		return nil
	}
	date := fields[w.Date]
	if start, _, ok := tree.SplitSpan(date); ok {
		date = start
	}
	var out []tree.Item
	for _, it := range Offered(f, self, fields, items, anchors) {
		theirs := it.FieldsAt(it.Current())
		from, to := theirs[w.From], theirs[w.To]
		if w.To == "" {
			// From names a span field: its two ends.
			from, to, _ = tree.SplitSpan(from)
		}
		if Holds(date, from, to) {
			out = append(out, it)
		}
	}
	return out
}

var day = regexp.MustCompile(`^(\d{4}-\d{2})(-\d{2})?`)

// Holds reports whether the span from..to holds date. A date is a day,
// 2011-03-15, or a month, 2011-03, which it holds when they share a day.
// An empty from or to leaves that end open; a value that is no date holds
// nothing.
func Holds(date, from, to string) bool {
	lo, hi, ok := bounds(date)
	if !ok {
		return false
	}
	if from != "" {
		start, _, ok := bounds(from)
		if !ok || hi < start {
			return false
		}
	}
	if to != "" {
		_, end, ok := bounds(to)
		if !ok || lo > end {
			return false
		}
	}
	return true
}

// bounds is a day's or a month's first and last day, compared as text.
func bounds(value string) (string, string, bool) {
	m := day.FindStringSubmatch(value)
	if m == nil {
		return "", "", false
	}
	if m[2] != "" {
		return m[1] + m[2], m[1] + m[2], true
	}
	return m[1] + "-01", m[1] + "-31", true
}

// Proposal is a link an Item lacks and the one Item it should be.
type Proposal struct {
	Item string `json:"item"`
	Key  string `json:"key"`
	// Link is the value to write: an Item ID, or <id>@<revision> for a
	// revision field, its current revision.
	Link string `json:"link"`
}

// Unsure is a link an Item lacks where more than one Item fits.
type Unsure struct {
	Item    string   `json:"item"`
	Key     string   `json:"key"`
	Fitting []string `json:"fitting"`
}

// Proposals is, for every Item's empty link field with a within, the one
// Item whose span holds its date, and, apart, those where several do.
// An Item no span holds is left out: nothing is known of it.
func Proposals(templates []tree.Template, items []tree.Item) ([]Proposal, []Unsure) {
	byType := map[string]tree.Template{}
	for _, t := range templates {
		byType[t.Type] = t
	}
	anchors := tree.AnchorTypes(templates)
	proposals, unsure := []Proposal{}, []Unsure{}
	for _, it := range items {
		t, ok := byType[it.Type]
		if !ok {
			continue
		}
		fields := it.FieldsAt(it.Current())
		for _, f := range t.Fields {
			if f.Within == nil || fields[f.Key] != "" {
				continue
			}
			fit := Suggested(f, it.ID, fields, items, anchors)
			switch {
			case len(fit) == 1:
				link := fit[0].ID
				if f.Type == tree.FieldRevision {
					link += "@" + fit[0].Current()
				}
				proposals = append(proposals, Proposal{Item: it.ID, Key: f.Key, Link: link})
			case len(fit) > 1:
				ids := make([]string, len(fit))
				for i, x := range fit {
					ids[i] = x.ID
				}
				unsure = append(unsure, Unsure{Item: it.ID, Key: f.Key, Fitting: ids})
			}
		}
	}
	sort.Slice(proposals, func(i, j int) bool { return proposals[i].Item < proposals[j].Item })
	sort.Slice(unsure, func(i, j int) bool { return unsure[i].Item < unsure[j].Item })
	return proposals, unsure
}
