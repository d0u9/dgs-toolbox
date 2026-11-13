package entrytemplate

import (
	"fmt"
	"strings"
)

// CopyLink shows the position as "(latitude, longitude)" and links it to the
// vault's copy command, which puts it on the clipboard the other way round.
// What is read and what is copied deliberately differ: maps want one order and
// the services that take a pasted pair want the other. No position is nothing
// to show, and no command writes the pair unlinked.
func CopyLink(latitude, longitude, choice string) string {
	if latitude == "" || longitude == "" {
		return ""
	}
	shown := fmt.Sprintf("(%s, %s)", latitude, longitude)
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return shown
	}
	return fmt.Sprintf("[%s](obsidian://quickadd?choice=%s&value-coordinates=%s)",
		shown, EscapeURIComponent(choice), EscapeURIComponent(latitude+", "+longitude))
}

// EscapeURIComponent encodes the way a browser's encodeURIComponent does, which
// is what wrote the links already in these notes: a space is %20 rather than +,
// and the characters JavaScript leaves alone are left alone. A + would be read
// back as a literal plus by some parsers, and the command it names would not be
// found.
func EscapeURIComponent(value string) string {
	const unreserved = "-_.!~*'()"
	var out strings.Builder
	for _, b := range []byte(value) {
		switch {
		case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9',
			strings.IndexByte(unreserved, b) >= 0:
			out.WriteByte(b)
		default:
			fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return out.String()
}

// MapLink is one map service's link to a position. Name and Short are what a
// note writes; English is what the TUI shows, since the TUI is English.
type MapLink struct {
	Name    string
	Short   string
	English string
	URL     string
}

// Coordinates are handed to every provider as WGS-84, which is what a device
// records. The Chinese services use shifted systems, so the source system is
// declared and they convert: without it a position lands a few hundred metres
// off inside China, and identically everywhere else.
var mapProviders = []struct {
	name, short, english string
	build                func(latitude, longitude, label string) string
}{
	{"Apple 地图", "Apple", "Apple", func(latitude, longitude, label string) string {
		return "https://maps.apple.com/place?coordinate=" + latitude + "," + longitude + query("name", label)
	}},
	{"高德地图", "高德", "Amap", func(latitude, longitude, label string) string {
		return "https://uri.amap.com/marker?position=" + longitude + "," + latitude +
			"&coordinate=wgs84" + query("name", label)
	}},
	{"Google 地图", "Google", "Google", func(latitude, longitude, _ string) string {
		return "https://www.google.com/maps/search/?api=1&query=" + latitude + "," + longitude
	}},
	{"百度地图", "百度", "Baidu", func(latitude, longitude, label string) string {
		return "https://api.map.baidu.com/marker?location=" + latitude + "," + longitude +
			"&coord_type=wgs84&output=html" + query("title", label)
	}},
	{"OpenStreetMap", "OSM", "OSM", func(latitude, longitude, _ string) string {
		return fmt.Sprintf("https://www.openstreetmap.org/?mlat=%s&mlon=%s#map=17/%s/%s",
			latitude, longitude, latitude, longitude)
	}},
}

func query(name, label string) string {
	if label == "" {
		return ""
	}
	return "&" + name + "=" + EscapeURIComponent(label)
}

// MapLinks builds the links for a position, in the order asked for. wanted is a
// comma-separated list of names, short names or English names; empty means
// every provider. A name nothing matches is left out rather than guessed at.
func MapLinks(latitude, longitude, label, wanted string) []MapLink {
	if latitude == "" || longitude == "" {
		return nil
	}
	var links []MapLink
	add := func(index int) {
		provider := mapProviders[index]
		links = append(links, MapLink{
			Name:    provider.name,
			Short:   provider.short,
			English: provider.english,
			URL:     provider.build(latitude, longitude, label),
		})
	}

	names := strings.Split(wanted, ",")
	if strings.TrimSpace(wanted) == "" {
		for index := range mapProviders {
			add(index)
		}
		return links
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		for index, provider := range mapProviders {
			if provider.name == name || provider.short == name || provider.english == name {
				add(index)
				break
			}
		}
	}
	return links
}

// MapLinksLine is the map services' links on one line, as a note writes them.
func MapLinksLine(latitude, longitude, label, services string) string {
	links := MapLinks(latitude, longitude, label, services)
	parts := make([]string, 0, len(links))
	for _, link := range links {
		parts = append(parts, fmt.Sprintf("[%s](%s)", link.Short, link.URL))
	}
	return strings.Join(parts, " · ")
}
