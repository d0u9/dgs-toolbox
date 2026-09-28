package tree

import (
	"dgs-toolbox/internal/doc/country"
	"sort"
	"strings"
)

// FieldValues lists distinct saved values for one type and country across revisions, including
// retired Items. Sidecars remain the only source; empty values are omitted.
func FieldValues(items []Item, typ, nation, key string) []string {
	normalized, valid := country.Normalize(nation, country.Format("alpha2"))
	if !valid {
		return []string{}
	}
	return typeValues(items, typ, key, func(fields map[string]string) bool {
		candidate, ok := country.Normalize(fields["country"], country.Format("alpha2"))
		return ok && candidate == normalized
	})
}

// TypeValues lists distinct saved values of key for one type, whatever the
// country, across revisions and retired Items; empty values are omitted.
func TypeValues(items []Item, typ, key string) []string {
	return typeValues(items, typ, key, func(map[string]string) bool { return true })
}

func typeValues(items []Item, typ, key string, keep func(map[string]string) bool) []string {
	seen := map[string]bool{}
	add := func(fields map[string]string) {
		if value := strings.TrimSpace(fields[key]); value != "" && keep(fields) {
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
