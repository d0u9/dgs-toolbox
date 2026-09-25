// Package suggest finds values for a Template's fields in a document's text,
// by the pattern each field declares. It proposes; it never decides.
package suggest

import (
	"regexp"
	"strings"

	"dgs-toolbox/internal/doc/tree"
)

// Fields returns a value for every field whose pattern matches text: the
// first capture group when the pattern has one, else the whole match, trimmed.
// A field with no pattern, a pattern that does not compile, or an empty match
// is left out.
func Fields(t tree.Template, text string) map[string]string {
	out := map[string]string{}
	for key, m := range Matches(t, text) {
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
func Matches(t tree.Template, text string) map[string]Match {
	out := map[string]Match{}
	for _, f := range t.Fields {
		if f.Pattern == "" {
			continue
		}
		pattern, err := regexp.Compile(f.Pattern)
		if err != nil {
			continue
		}
		at := pattern.FindStringSubmatchIndex(text)
		if at == nil {
			continue
		}
		start, end := at[0], at[1]
		if len(at) > 2 && at[2] >= 0 {
			start, end = at[2], at[3]
		}
		raw := text[start:end]
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		start += strings.Index(raw, value)
		out[f.Key] = Match{Value: value, Start: start, End: start + len(value)}
	}
	return out
}

// All is Matches for each Template, keyed by type. A type nothing matched is
// left out.
func All(templates []tree.Template, text string) map[string]map[string]Match {
	out := map[string]map[string]Match{}
	for _, t := range templates {
		if found := Matches(t, text); len(found) > 0 {
			out[t.Type] = found
		}
	}
	return out
}
