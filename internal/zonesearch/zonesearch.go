// Package zonesearch finds IANA time zones from a few typed letters: "syd"
// finds Australia/Sydney, "aus" finds every zone in Australia (and Austria's),
// "chi" finds China's zone as well as Chicago and Chile's.
//
// The catalogue is tzdb's own zone.tab and iso3166.tab, compiled in, so the
// search needs nothing from the machine it runs on. Both files are in the
// public domain. zone.tab gives each zone one country, which is what a person
// searching by country means.
package zonesearch

import (
	_ "embed"
	"sort"
	"strings"
	"sync"
)

//go:embed zone.tab
var zoneTab string

//go:embed iso3166.tab
var countryTab string

// DefaultLimit is how many matches Search returns when asked for none in
// particular: enough for every zone of Australia at once.
const DefaultLimit = 20

// Zone is one entry of the catalogue.
type Zone struct {
	Name    string `json:"zone"`    // "Australia/Sydney"
	Country string `json:"country"` // "Australia"
	Code    string `json:"code"`    // "AU"
	Comment string `json:"comment"` // "New South Wales (most areas)"
}

var (
	loadOnce  sync.Once
	catalogue []Zone
)

// All is every zone in the catalogue, plus UTC, sorted by name.
func All() []Zone {
	loadOnce.Do(func() { catalogue = parse(zoneTab, countryTab) })
	return catalogue
}

func parse(zones, countries string) []Zone {
	names := map[string]string{}
	for _, line := range strings.Split(countries, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		code, name, found := strings.Cut(line, "\t")
		if found {
			names[code] = strings.TrimSpace(name)
		}
	}
	out := []Zone{{Name: "UTC", Country: "Coordinated Universal Time"}}
	for _, line := range strings.Split(zones, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		zone := Zone{Name: fields[2], Code: fields[0], Country: names[fields[0]]}
		if len(fields) > 3 {
			zone.Comment = fields[3]
		}
		out = append(out, zone)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Search returns the zones matching query, best first, at most limit of them
// (DefaultLimit when limit is not positive). Every word of the query must
// match; a word matches the start of a word in the zone's name, its country's
// name or its comment, or the country code exactly. A match on the city
// ranks above a match on the country, and both above one in the comment.
// An empty query matches nothing.
func Search(query string, limit int) []Zone {
	if limit <= 0 {
		limit = DefaultLimit
	}
	terms := words(query)
	if len(terms) == 0 {
		return nil
	}
	type scored struct {
		zone  Zone
		score int
	}
	var found []scored
	for _, zone := range All() {
		total := 0
		for _, term := range terms {
			best := score(zone, term)
			if best == 0 {
				total = 0
				break
			}
			total += best
		}
		if total > 0 {
			found = append(found, scored{zone, total})
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].score != found[j].score {
			return found[i].score > found[j].score
		}
		return found[i].zone.Name < found[j].zone.Name
	})
	if len(found) > limit {
		found = found[:limit]
	}
	out := make([]Zone, len(found))
	for i, match := range found {
		out[i] = match.zone
	}
	return out
}

// score is how well one query word matches a zone, zero for not at all.
func score(zone Zone, term string) int {
	city := zone.Name
	if slash := strings.LastIndex(city, "/"); slash >= 0 {
		city = city[slash+1:]
	}
	switch {
	case strings.EqualFold(zone.Name, term):
		return 100
	case startsAnyWord(city, term):
		return 40
	case strings.EqualFold(zone.Code, term):
		return 30
	case startsAnyWord(zone.Country, term):
		return 25
	case startsAnyWord(zone.Name, term):
		return 20
	case startsAnyWord(zone.Comment, term):
		return 10
	case strings.Contains(strings.ToLower(zone.Name), term):
		return 5
	}
	return 0
}

func startsAnyWord(text, term string) bool {
	for _, word := range words(text) {
		if strings.HasPrefix(word, term) {
			return true
		}
	}
	return false
}

// words lowercases text and splits it on everything that is not a letter or
// a digit, so "America/Port_of_Spain" is america, port, of, spain.
func words(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127)
	})
}
