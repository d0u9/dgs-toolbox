package view

import (
	"dgs-toolbox/internal/doc/country"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"dgs-toolbox/internal/doc/tree"
)

// DateField is the field year, month and date are derived from.
const DateField = "issued_at"

// File is one PDF a View places.
type File struct {
	Path   string `json:"path"`
	Item   string `json:"item"`
	Digest string `json:"digest"`
	// Revision counts from 1, in the order the Item's revisions were added.
	Revision int `json:"revision"`
}

// Missing is a selected PDF the layout cannot name, and the keys it lacks.
type Missing struct {
	Item     string   `json:"item"`
	Digest   string   `json:"digest"`
	Revision int      `json:"revision"`
	Keys     []string `json:"keys"`
	// Fields are the Item fields that, filled in, supply Keys.
	Fields []string `json:"fields"`
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

// Matches reports whether item passes every condition of query.
func Matches(query map[string]Values, item tree.Item) bool {
	for key, accepted := range query {
		if len(accepted) == 0 {
			continue
		}
		value := item.Fields[key]
		if key == "type" {
			value = item.Type
		}
		found := false
		for _, want := range accepted {
			if value == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

var isoDate = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})`)

// KeysOf is every key a layout can use for one revision of item: its fields,
// `id`, `type`, `kind`, `revision`, `ext`, and `year`, `month` and `date`
// when DateField holds a date.
func KeysOf(item tree.Item, revision int) map[string]string {
	keys := map[string]string{}
	for k, v := range item.Fields {
		if v != "" {
			keys[k] = v
		}
	}
	keys["id"] = item.ID
	keys["type"] = item.Type
	keys["kind"] = string(item.Kind)
	keys["revision"] = strconv.Itoa(revision)
	keys["ext"] = "pdf"
	if m := isoDate.FindStringSubmatch(item.Fields[DateField]); m != nil {
		keys["year"], keys["month"], keys["date"] = m[1], m[2], m[1]+"-"+m[2]+"-"+m[3]
	}
	return keys
}

// FieldsFor is the Item fields that supply keys, in order and once each:
// year, month and date come from DateField, every other key is a field of
// its own name.
func FieldsFor(keys []string) []string {
	var fields []string
	seen := map[string]bool{}
	for _, k := range keys {
		if k == "year" || k == "month" || k == "date" {
			k = DateField
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

// Build computes v's plan over items. Items are taken in ID order and a
// document's revisions in the order they were added, so the same state always
// gives the same plan, numbering included. Paths are compared ignoring case,
// as the file systems a Target usually lives on do.
func Build(v View, items []tree.Item) (Plan, error) {
	layout, err := Parse(v.Layout)
	if err != nil {
		return Plan{}, err
	}
	sorted := append([]tree.Item(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	plan := Plan{Files: []File{}, Missing: []Missing{}, Clashes: []Clash{}}
	var placed []File
	for _, item := range sorted {
		if !Matches(v.Query, item) {
			continue
		}
		current := item.Current()
		for i, rev := range item.Revisions {
			if v.Selection == Head && rev.Digest != current {
				continue
			}
			keys := KeysOf(item, i+1)
			name, lacking := render(layout, keys, v.Default)
			if len(lacking) > 0 {
				plan.Missing = append(plan.Missing, Missing{Item: item.ID, Digest: rev.Digest, Revision: i + 1, Keys: lacking, Fields: FieldsFor(lacking)})
				continue
			}
			placed = append(placed, File{Path: name, Item: item.ID, Digest: rev.Digest, Revision: i + 1})
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
		groups, order = map[string][]int{}, nil
		for i, f := range placed {
			k := strings.ToLower(f.Path)
			if _, ok := groups[k]; !ok {
				order = append(order, k)
			}
			groups[k] = append(groups[k], i)
		}
	}
	for _, k := range order {
		members := groups[k]
		if len(members) == 1 {
			plan.Files = append(plan.Files, placed[members[0]])
			continue
		}
		clash := Clash{Path: placed[members[0]].Path}
		for _, i := range members {
			clash.Files = append(clash.Files, placed[i])
		}
		plan.Clashes = append(plan.Clashes, clash)
	}
	sort.Slice(plan.Files, func(i, j int) bool { return plan.Files[i].Path < plan.Files[j].Path })
	return plan, nil
}

// render fills a layout from keys. It returns the keys it lacked, when there
// is no default to stand in for them.
func render(layout Layout, keys map[string]string, fallback *string) (string, []string) {
	var lacking []string
	segments := make([]string, len(layout))
	for s, parts := range layout {
		var b strings.Builder
		for _, part := range parts {
			if part.Key == "" {
				b.WriteString(part.Text)
				continue
			}
			value, ok := keys[part.Key]
			if !ok {
				if fallback == nil {
					if !contains(lacking, part.Key) {
						lacking = append(lacking, part.Key)
					}
					continue
				}
				value = *fallback
			} else if part.Format != "" {
				// A value that names no country is written as it is.
				if kept, ok := country.Normalize(value, country.Format(part.Format)); ok {
					value = kept
				}
			}
			b.WriteString(Clean(value))
		}
		segments[s] = b.String()
	}
	return strings.Join(segments, "/"), lacking
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

func pad(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
