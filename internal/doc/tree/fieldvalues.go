package tree

import (
	"dgs-toolbox/internal/doc/country"
	"sort"
	"strings"
)

// FieldValues lists distinct saved values for one type and country across revisions, including
// retired Items. Sidecars remain the only source; empty values are omitted.
func FieldValues(items []Item, typ, nation, key string) []string {
	seen := map[string]bool{}
	normalized, valid := country.Normalize(nation, country.Format("alpha2"))
	add := func(fields map[string]string) {
		candidate, ok := country.Normalize(fields["country"], country.Format("alpha2"))
		if value := strings.TrimSpace(fields[key]); valid && ok && candidate == normalized && value != "" {
			seen[value] = true
		}
	}
	for _, item := range items {
		if item.Type == typ {
			add(item.Fields)
		}
		for _, revision := range item.Revisions {
			revisionType := revision.Type
			if revisionType == "" {
				revisionType = item.Type
			}
			if revisionType == typ {
				add(item.FieldsAt(revision.Ref()))
			}
		}
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
