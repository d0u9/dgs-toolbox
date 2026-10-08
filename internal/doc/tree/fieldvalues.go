package tree

import (
	"dgs-toolbox/internal/doc/country"
	"sort"
	"strings"
)

// FieldValues lists distinct current values for one type and country, including
// retired Items. Earlier revisions are history, not suggestions: a value
// rewritten everywhere stops being offered. Sidecars remain the only source; empty values are omitted.
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
// country, from current fields of all Items, retired ones too; empty values are omitted.
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
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

// TypeFieldValues returns individual suggestions for a multiple field,
// while preserving commas in ordinary text fields.
func TypeFieldValues(items []Item, typ string, field Field) []string {
	values := TypeValues(items, typ, field.Key)
	if !field.Multiple {
		return values
	}
	seen := map[string]bool{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				seen[part] = true
			}
		}
	}
	values = make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
