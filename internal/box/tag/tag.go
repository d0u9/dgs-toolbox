// Package tag is how a Box spells its tags and what it has already used.
//
// Tags are free-form, so the only thing that keeps them useful is that the
// same word is spelled the same way every time. Normalize is that spelling:
// lower case, with runs of spaces joined by a hyphen, so "Japan 2019" and
// "japan-2019" are one tag rather than two that a filter tells apart.
package tag

import (
	"sort"
	"strings"
	"unicode"
)

// Normalize is one tag in its canonical spelling, or "" when nothing is left.
func Normalize(tag string) string {
	fields := strings.FieldsFunc(strings.ToLower(tag), unicode.IsSpace)
	return strings.Join(fields, "-")
}

// List normalizes every tag and drops the empty and the repeated, keeping the
// first occurrence's position.
func List(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := Normalize(raw)
		if tag == "" {
			continue
		}
		if _, repeated := seen[tag]; repeated {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

// Use is one tag and how many records carry it.
type Use struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Count is every tag across records, each record counted once per tag however
// often it repeats it, most used first and then by name. A record is the list
// of tags one thing carries; spellings that normalize alike are one tag.
func Count(records [][]string) []Use {
	counts := map[string]int{}
	for _, record := range records {
		for _, tag := range List(record) {
			counts[tag]++
		}
	}
	uses := make([]Use, 0, len(counts))
	for name, count := range counts {
		uses = append(uses, Use{Name: name, Count: count})
	}
	sort.Slice(uses, func(i, j int) bool {
		if uses[i].Count != uses[j].Count {
			return uses[i].Count > uses[j].Count
		}
		return uses[i].Name < uses[j].Name
	})
	return uses
}
