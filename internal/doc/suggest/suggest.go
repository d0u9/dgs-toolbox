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
	for _, f := range t.Fields {
		if f.Pattern == "" {
			continue
		}
		pattern, err := regexp.Compile(f.Pattern)
		if err != nil {
			continue
		}
		match := pattern.FindStringSubmatch(text)
		if match == nil {
			continue
		}
		value := match[0]
		if len(match) > 1 {
			value = match[1]
		}
		if value = strings.TrimSpace(value); value != "" {
			out[f.Key] = value
		}
	}
	return out
}

// All is Fields for each Template, keyed by type. A type nothing matched is
// left out.
func All(templates []tree.Template, text string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, t := range templates {
		if fields := Fields(t, text); len(fields) > 0 {
			out[t.Type] = fields
		}
	}
	return out
}
