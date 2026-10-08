package view

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"dgs-toolbox/internal/doc/expr"
	"dgs-toolbox/internal/doc/tree"
)

type Defaults struct {
	Revision Selection `yaml:"revision,omitempty" json:"revision,omitempty"`
	Valid    string    `yaml:"valid,omitempty" json:"valid,omitempty"`
	File     string    `yaml:"file,omitempty" json:"file,omitempty"`
}

func (d Defaults) Validate() error {
	if d.Revision != "" && d.Revision != Head && d.Revision != All {
		return fmt.Errorf("revision must be head or all")
	}
	if d.Valid != "" && d.Valid != "today" && d.Valid != "expired" {
		return fmt.Errorf("valid must be today or expired")
	}
	return nil
}
func (d Defaults) inherit(parent Defaults) Defaults {
	if d.Revision == "" {
		d.Revision = parent.Revision
	}
	if d.Valid == "" {
		d.Valid = parent.Valid
	}
	if d.File == "" {
		d.File = parent.File
	}
	return d
}

type Folder struct {
	Name  string `yaml:"name" json:"name"`
	Take  Values `yaml:"take,omitempty" json:"take,omitempty"`
	If    string `yaml:"if,omitempty" json:"if,omitempty"`
	Each  string `yaml:"each,omitempty" json:"each,omitempty"`
	File  string `yaml:"file,omitempty" json:"file,omitempty"`
	Order Values `yaml:"order,omitempty" json:"order,omitempty"`
	// Start is the first {#} of the folders each makes; 0 starts at 1.
	Start    int      `yaml:"start,omitempty" json:"start,omitempty"`
	Defaults Defaults `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Folders  []Folder `yaml:"folders,omitempty" json:"folders,omitempty"`
}

type Pin struct {
	Items   []string       `yaml:"items,omitempty" json:"items,omitempty"`
	Numbers map[string]int `yaml:"numbers,omitempty" json:"numbers,omitempty"`
}

func ValidateFolders(folders []Folder) error {
	for _, f := range folders {
		if f.Name == "" {
			return fmt.Errorf("folder needs a name")
		}
		for _, layout := range []string{f.Name, f.File, f.Defaults.File} {
			if layout == "" {
				continue
			}
			parsed, err := Parse(layout)
			if err != nil {
				return err
			}
			for _, counter := range parsed.Counters() {
				if counter.Label != "" || strings.Contains(layout, "{/#}") {
					return fmt.Errorf("folders support only {#} numbering")
				}
			}
		}
		if err := f.Defaults.Validate(); err != nil {
			return err
		}
		if f.Start < 0 {
			return fmt.Errorf("folder %s: start must not be negative", f.Name)
		}
		if f.If != "" {
			if _, err := expr.Parse(f.If); err != nil {
				return err
			}
		}
		if err := ValidateFolders(f.Folders); err != nil {
			return err
		}
	}
	return nil
}

type Skeleton struct {
	Plan
	Folders []string
}

// BuildFolders fills explicit folders in order. assigned is shared through
// an Outline's mounts; an Item belongs to the first folder that places it.
func BuildFolders(folders []Folder, items []tree.Item, types Types, defaults Defaults, pins map[string]Pin, at string, assigned map[string]bool, folderNumbers map[string]map[int]string, now time.Time) (Skeleton, error) {
	out := Skeleton{Plan: Plan{Files: []File{}, Missing: []Missing{}, Clashes: []Clash{}}}
	if err := ValidateFolders(folders); err != nil {
		return out, err
	}
	if err := defaults.Validate(); err != nil {
		return out, err
	}
	defaults = defaults.inherit(Defaults{Revision: Head, File: "{#}-{name}"})
	byID := map[string]tree.Item{}
	for _, i := range items {
		byID[i.ID] = i
	}
	// Pins override automatic placement throughout the Outline.
	pinned := map[string]string{}
	for folder, pin := range pins {
		for _, id := range pin.Items {
			if old, ok := pinned[id]; ok && old != folder {
				return out, fmt.Errorf("Item %s pinned to more than one folder", id)
			}
			if _, ok := byID[id]; !ok {
				return out, fmt.Errorf("pin names missing Item %s", id)
			}
			pinned[id] = folder
		}
		for name, n := range pin.Numbers {
			if n < 1 {
				return out, fmt.Errorf("number for %s must be positive", name)
			}
		}
	}
	keysOf := func(item tree.Item, n int) map[string]string {
		keys := KeysOf(item, n)
		Follow(keys, byID)
		for lang, name := range types.Names(keys["type"]) {
			keys["type:"+lang] = name
		}
		types.nameValues(keys)
		if keys["name"] == "" {
			keys["name"] = keys["type:zh"]
			if keys["name"] == "" {
				keys["name"] = keys["type"]
			}
		}
		return keys
	}
	renderName := func(layout string, keys map[string]string, number int) (string, []string, error) {
		parsed, err := Parse(strings.ReplaceAll(layout, "{#}", fmt.Sprintf("%02d", number)))
		if err != nil {
			return "", nil, err
		}
		name, lacking := render(parsed, keys, nil)
		if name == "" || path.IsAbs(name) || name != path.Clean(name) || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00") {
			return "", nil, fmt.Errorf("folder/file name is not a relative path")
		}
		return name, lacking, nil
	}
	type choice struct {
		item    tree.Item
		indices []int
		keys    map[string]string
	}
	var walk func([]Folder, string, []tree.Item, Defaults, map[string]string, []expr.Expr) error
	walk = func(list []Folder, parent string, pool []tree.Item, inherited Defaults, context map[string]string, conditions []expr.Expr) error {
		for _, f := range list {
			d := f.Defaults.inherit(inherited)
			if f.File != "" {
				d.File = f.File
			}
			filters := slices.Clone(conditions)
			if f.If != "" {
				e, err := expr.Parse(f.If)
				if err != nil {
					return err
				}
				filters = append(filters, e)
			}
			// take narrows what reaches the folders below, as if does.
			if len(f.Take) > 0 {
				quoted := make([]string, len(f.Take))
				for i, t := range f.Take {
					quoted[i] = strconv.Quote(t)
				}
				e, err := expr.Parse("type in [" + strings.Join(quoted, ", ") + "]")
				if err != nil {
					return err
				}
				filters = append(filters, e)
			}
			type group struct {
				value string
				keys  map[string]string
				items []tree.Item
			}
			groups := []group{{keys: maps.Clone(context), items: pool}}
			if f.Each != "" {
				groups = nil
				if _, isType := types.byType[f.Each]; isType {
					for _, anchor := range pool {
						if anchor.Type != f.Each && !types.Is(anchor.Type, f.Each) {
							continue
						}
						g := group{value: anchor.ID, keys: keysOf(anchor, len(anchor.Revisions))}
						for _, i := range pool {
							if i.ID == anchor.ID || i.CurrentFields()["about"] == anchor.ID {
								g.items = append(g.items, i)
							}
						}
						groups = append(groups, g)
					}
				} else {
					values := map[string][]tree.Item{}
					for _, i := range pool {
						keys := keysOf(i, len(i.Revisions))
						e := env{keys: keys, item: i, types: types}
						for _, v := range e.Values(f.Each) {
							values[v] = append(values[v], i)
						}
					}
					for v, entries := range values {
						keys := maps.Clone(context)
						keys[f.Each] = v
						groups = append(groups, group{value: v, keys: keys, items: entries})
					}
				}
				sort.SliceStable(groups, func(i, j int) bool {
					a, b := slices.Index(f.Order, groups[i].value), slices.Index(f.Order, groups[j].value)
					if a < 0 {
						a = len(f.Order)
					}
					if b < 0 {
						b = len(f.Order)
					}
					if a != b {
						return a < b
					}
					return groups[i].value < groups[j].value
				})
			}
			// number counts only the folders kept, so a dropped one leaves no gap.
			number := max(f.Start, 1)
			for _, g := range groups {
				folder, lacking, err := renderName(f.Name, g.keys, number)
				if err != nil {
					return err
				}
				if len(lacking) > 0 {
					return fmt.Errorf("folder %s lacks %s", f.Name, strings.Join(lacking, ", "))
				}
				folder = path.Join(parent, folder)
				// A folder made by each that ends up holding nothing is dropped;
				// only literal folders appear empty.
				foldersBefore, filesBefore := len(out.Folders), len(out.Files)
				out.Folders = append(out.Folders, folder)
				// The folders below take first; a folder with take keeps what
				// none of them took.
				if err := walk(f.Folders, folder, g.items, d, g.keys, filters); err != nil {
					return err
				}
				var choices []choice
				for _, item := range g.items {
					if assigned[item.ID] {
						continue
					}
					forced := pinned[item.ID] == folder
					if target := pinned[item.ID]; target != "" && !forced {
						continue
					}
					if !forced && len(f.Take) == 0 && !(f.Each != "" && len(f.Folders) == 0) {
						continue
					}
					latest := item.Latest()
					selected := choice{item: item}
					for n, r := range item.Revisions {
						if r.Digest == "" || !latest[r.Ref()] || d.Revision == Head && r.Ref() != item.Current() {
							continue
						}
						keys := keysOf(item, n+1)
						expired := item.SupersededBy != "" || (keys["date.end"] != "" && keys["date.end"] < now.Format("2006-01-02"))
						if d.Valid == "today" && expired || d.Valid == "expired" && !expired {
							continue
						}
						if !forced {
							take := len(f.Take) == 0
							for _, t := range f.Take {
								if keys["type"] == t || types.Is(keys["type"], t) {
									take = true
								}
							}
							if !take {
								continue
							}
							e := env{keys: keys, item: item, types: types, tags: append(slices.Clone(item.Tags), r.Tags...)}
							match := true
							for _, condition := range filters {
								if !condition.Eval(e) {
									match = false
									break
								}
							}
							if !match {
								continue
							}
						}
						selected.indices = append(selected.indices, n)
						selected.keys = keys
					}
					if len(selected.indices) > 0 {
						choices = append(choices, selected)
					}
				}
				pin := pins[folder]
				numberKey := "name"
				parsed, _ := Parse(d.File)
				if counters := parsed.Counters(); len(counters) > 0 && counters[0].Of != "" {
					numberKey = counters[0].Of
				}

				sort.SliceStable(choices, func(i, j int) bool {
					a, b := choices[i], choices[j]
					if x, y := slices.Index(pin.Items, a.item.ID), slices.Index(pin.Items, b.item.ID); x >= 0 || y >= 0 {
						if x < 0 {
							x = len(pin.Items)
						}
						if y < 0 {
							y = len(pin.Items)
						}
						if x != y {
							return x < y
						}
					}
					if len(f.Order) == 1 && a.keys[f.Order[0]] != "" && b.keys[f.Order[0]] != "" {
						if a.keys[f.Order[0]] != b.keys[f.Order[0]] {
							return a.keys[f.Order[0]] < b.keys[f.Order[0]]
						}
					}
					x, y := slices.Index(f.Order, a.keys[numberKey]), slices.Index(f.Order, b.keys[numberKey])
					if x < 0 {
						x = len(f.Order)
					}
					if y < 0 {
						y = len(f.Order)
					}
					if x != y {
						return x < y
					}
					return a.item.ID < b.item.ID
				})
				numbers := map[string]int{}
				used := folderNumbers[folder]
				if used == nil {
					used = map[int]string{}
					folderNumbers[folder] = used
				}
				reserved := map[int]bool{}
				for _, n := range pin.Numbers {
					reserved[n] = true
				}
				for _, c := range choices {
					n := pin.Numbers[c.item.ID]
					if n == 0 {
						n = pin.Numbers[c.keys[numberKey]]
					}
					if n > 0 {
						if old := used[n]; old != "" && old != c.item.ID {
							return fmt.Errorf("folder %s pins number %d twice", folder, n)
						}
						numbers[c.item.ID] = n
						used[n] = c.item.ID
					}
				}
				next := 1
				for _, c := range choices {
					n := numbers[c.item.ID]
					if n == 0 {
						for used[next] != "" || reserved[next] {
							next++
						}
						n = next
						used[n] = c.item.ID
						next++
					}
					for _, index := range c.indices {
						r := c.item.Revisions[index]
						keys := keysOf(c.item, index+1)
						name, lacking, err := renderName(d.File, keys, n)
						if err != nil {
							return err
						}
						if len(lacking) > 0 {
							out.Missing = append(out.Missing, Missing{Item: c.item.ID, Digest: r.Digest, Revision: index + 1, Keys: lacking, Fields: FieldsFor(lacking)})
							continue
						}
						if len(c.indices) > 1 && !strings.Contains(d.File, "{date") && !strings.Contains(d.File, "{year}") {
							date := keys["year"]
							if date == "" {
								date = keys["date.start"]
							}
							if date == "" && len(r.Added) >= 10 {
								date = r.Added[:10]
							}
							if date == "" {
								date = strconv.Itoa(c.item.RevisionNumber(r.Ref()))
							}
							// Same-year rescans still need a distinct date/version suffix.
							duplicates := 0
							for _, other := range c.indices {
								otherKeys := keysOf(c.item, other+1)
								otherDate := otherKeys["year"]
								if otherDate == "" {
									otherDate = otherKeys["date.start"]
								}
								if otherDate == "" && len(c.item.Revisions[other].Added) >= 10 {
									otherDate = c.item.Revisions[other].Added[:10]
								}
								if otherDate == date {
									duplicates++
								}
							}
							if duplicates > 1 {
								date = r.Added
								if date == "" {
									date = keys["date.start"]
								}
								date += "-r" + strconv.Itoa(c.item.RevisionNumber(r.Ref()))
							}
							suffix := "-" + Clean(date)
							if strings.Contains(d.File, "{ext}") && keys["ext"] != "" {
								ext := "." + keys["ext"]
								name = strings.TrimSuffix(name, ext) + suffix + ext
							} else {
								name += suffix
							}
						}
						if !strings.Contains(d.File, "{ext}") {
							if ext := keys["ext"]; ext != "" {
								name += "." + ext
							}
						}
						out.Files = append(out.Files, File{Path: path.Join(folder, name), Item: c.item.ID, Digest: r.Digest, Revision: index + 1, Node: folder, Number: n, NumberName: keys[numberKey]})
					}
					assigned[c.item.ID] = true
				}
				if f.Each != "" && len(out.Files) == filesBefore {
					out.Folders = out.Folders[:foldersBefore]
					continue
				}
				number++
			}
		}
		return nil
	}
	if err := walk(folders, at, items, defaults, map[string]string{}, nil); err != nil {
		return out, err
	}
	out.Files, out.Clashes = separate(out.Files)
	return out, nil
}
