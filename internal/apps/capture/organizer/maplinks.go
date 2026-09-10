package organizer

import (
	"fmt"
	"strings"
)

// MapLink is one map service's link to a position.
type MapLink struct {
	Name  string
	Short string
	URL   string
}

// Coordinates are handed to every provider as WGS-84, which is what a device
// records. The Chinese services use shifted systems, so the source system is
// declared and they convert: without it a position lands a few hundred metres
// off inside China, and identically everywhere else.
var mapProviders = []struct {
	name  string
	short string
	build func(latitude, longitude, label string) string
}{
	{"Apple 地图", "Apple", func(latitude, longitude, label string) string {
		return "https://maps.apple.com/?ll=" + latitude + "," + longitude + query("q", label)
	}},
	{"高德地图", "高德", func(latitude, longitude, label string) string {
		return "https://uri.amap.com/marker?position=" + longitude + "," + latitude +
			"&coordinate=wgs84" + query("name", label)
	}},
	{"Google 地图", "Google", func(latitude, longitude, _ string) string {
		return "https://www.google.com/maps/search/?api=1&query=" + latitude + "," + longitude
	}},
	{"百度地图", "百度", func(latitude, longitude, label string) string {
		return "https://api.map.baidu.com/marker?location=" + latitude + "," + longitude +
			"&coord_type=wgs84&output=html" + query("title", label)
	}},
	{"OpenStreetMap", "OSM", func(latitude, longitude, _ string) string {
		return fmt.Sprintf("https://www.openstreetmap.org/?mlat=%s&mlon=%s#map=17/%s/%s",
			latitude, longitude, latitude, longitude)
	}},
}

func query(name, label string) string {
	if label == "" {
		return ""
	}
	return "&" + name + "=" + escapeURIComponent(label)
}

// MapLinks builds the links for a position, in the order asked for. wanted is a
// comma-separated list of names or short names; empty means every provider.
// A name nothing matches is left out rather than guessed at.
func MapLinks(latitude, longitude, label, wanted string) []MapLink {
	if latitude == "" || longitude == "" {
		return nil
	}
	var links []MapLink
	add := func(index int) {
		provider := mapProviders[index]
		links = append(links, MapLink{
			Name:  provider.name,
			Short: provider.short,
			URL:   provider.build(latitude, longitude, label),
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
			if provider.name == name || provider.short == name {
				add(index)
				break
			}
		}
	}
	return links
}
