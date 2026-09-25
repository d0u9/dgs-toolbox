// Package suggest finds values for a Template's fields in a document's text,
// by the pattern each field declares. It proposes; it never decides.
package suggest

import (
	"regexp"
	"strings"

	"dgs-toolbox/internal/doc/country"
	"dgs-toolbox/internal/doc/dates"
	"dgs-toolbox/internal/doc/tree"
)

// Fields returns a value for every field whose pattern matches text: the
// first capture group when the pattern has one, else the whole match, trimmed.
// A field with no pattern, a pattern that does not compile, or an empty match
// is left out.
func Fields(t tree.Template, text string, order dates.Order) map[string]string {
	out := map[string]string{}
	for key, m := range Matches(t, text, order) {
		out[key] = m.Value
	}
	return out
}

// Match is a value found in the text and where: Start and End are byte
// offsets into the text, around the value itself.
type Match struct {
	Value string `json:"value"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Matches is Fields with where each value was found.
//
// A date field's value is written YYYY-MM-DD, whatever form the text has it
// in, and a pattern's match that is not a date is no suggestion for one. A
// date field with no pattern takes the next date in the text, in text order,
// that no earlier date field took and the Template does not ignore: issue
// dates usually come before expiry dates, as fields usually list them.
func Matches(t tree.Template, text string, order dates.Order) map[string]Match {
	out := map[string]Match{}
	var found []dates.Found
	next := 0
	for _, f := range t.Fields {
		patterns := f.AllPatterns()
		if len(patterns) == 0 {
			if f.Type != tree.FieldDate {
				continue
			}
			if found == nil {
				found = unignored(dates.Find(text, order), t.IgnoreDates)
			}
			if next < len(found) {
				d := found[next]
				next++
				out[f.Key] = Match{Value: d.Value, Start: d.Start, End: d.End}
			}
			continue
		}
		for _, p := range patterns {
			if m, ok := match(f, p, text, order); ok {
				out[f.Key] = m
				break
			}
		}
	}
	return out
}

// match is what one pattern suggests for f in text: the first capture group
// when it has one, else the whole match, kept in the field's form. A match
// that does not fit the field — no country, no date — is no suggestion.
func match(f tree.Field, expression, text string, order dates.Order) (Match, bool) {
	pattern, err := regexp.Compile(expression)
	if err != nil {
		return Match{}, false
	}
	at := pattern.FindStringSubmatchIndex(text)
	if at == nil {
		return Match{}, false
	}
	start, end := at[0], at[1]
	if len(at) > 2 && at[2] >= 0 {
		start, end = at[2], at[3]
	}
	raw := text[start:end]
	value := strings.TrimSpace(raw)
	if value == "" {
		return Match{}, false
	}
	start += strings.Index(raw, value)
	end = start + len(value)
	if f.Type == tree.FieldCountry {
		kept, ok := country.Normalize(value, country.Format(f.Format))
		if !ok {
			return Match{}, false
		}
		value = kept
	}
	if f.Type == tree.FieldDate {
		parsed, ok := dates.Parse(value, order)
		if !ok {
			return Match{}, false
		}
		value = parsed
	}
	return Match{Value: value, Start: start, End: end}, true
}

// All is Matches for each Template, keyed by type. A type nothing matched is
// left out.
func All(templates []tree.Template, text string, order dates.Order) map[string]map[string]Match {
	out := map[string]map[string]Match{}
	for _, t := range templates {
		if found := Matches(t, text, order); len(found) > 0 {
			out[t.Type] = found
		}
	}
	return out
}

func unignored(found []dates.Found, ignore []string) []dates.Found {
	out := []dates.Found{}
	for _, d := range found {
		skip := false
		for _, i := range ignore {
			if d.Value == i {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, d)
		}
	}
	return out
}
