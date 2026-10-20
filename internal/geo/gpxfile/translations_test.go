package gpxfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTranslationsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.gpx")
	en := map[string]string{"en": "West Lake & co"}
	err := CreateNamed(path, File{Name: "西湖", Lang: "zh", Translations: map[string]string{"en": "Hangzhou"}},
		[]Waypoint{{Name: "塔", Translations: en, CaptureID: "c1"}},
		[]Route{{Name: "路", Translations: en, Points: []Point{{}}}},
		[]Track{{Name: "轨迹", Translations: en, Segments: []Segment{{Points: []Point{{}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Name != "西湖" || file.Lang != "zh" || file.Translations["en"] != "Hangzhou" {
		t.Fatalf("file: %q %v", file.Name, file.Translations)
	}
	for _, got := range []map[string]string{file.Waypoints[0].Translations, file.Routes[0].Translations, file.Tracks[0].Translations} {
		if !reflect.DeepEqual(got, en) {
			t.Fatalf("translations %v, want %v", got, en)
		}
	}
	if file.Waypoints[0].CaptureID != "c1" || file.Waypoints[0].Name != "塔" {
		t.Fatalf("waypoint %+v", file.Waypoints[0])
	}
}

// SetTranslations replaces, adds and removes translations in place, keeping
// the rest of each element, and leaves no empty <extensions> behind.
func TestSetTranslations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.gpx")
	err := CreateAll(path, "", []Waypoint{{Name: "w", CaptureID: "c1"}, {Name: "v"}},
		[]Route{{Name: "r", Points: []Point{{}}}},
		[]Track{{Name: "t", Segments: []Segment{{Points: []Point{{}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	both := map[string]string{"en": "One", "zh": "一"}
	for _, step := range []struct {
		part  Part
		index int
	}{{PartFile, 0}, {PartTrack, 0}, {PartRoute, 0}, {PartWaypoint, 0}, {PartWaypoint, 1}} {
		if err := SetTranslations(path, step.part, step.index, "zh-Hans", both); err != nil {
			t.Fatal(step.part, err)
		}
		// Setting again replaces rather than adds.
		if err := SetTranslations(path, step.part, step.index, "zh-Hans", both); err != nil {
			t.Fatal(step.part, err)
		}
	}
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range []map[string]string{file.Translations, file.Tracks[0].Translations, file.Routes[0].Translations, file.Waypoints[0].Translations, file.Waypoints[1].Translations} {
		if !reflect.DeepEqual(got, both) {
			t.Fatalf("translations %v, want %v", got, both)
		}
	}
	for _, lang := range []string{file.Lang, file.Tracks[0].Lang, file.Routes[0].Lang, file.Waypoints[0].Lang, file.Waypoints[1].Lang} {
		if lang != "zh-Hans" {
			t.Fatalf("lang %q", lang)
		}
	}
	if file.Waypoints[0].CaptureID != "c1" || len(file.Tracks[0].Segments) != 1 || len(file.Routes[0].Points) != 1 || file.Tracks[0].Name != "t" {
		t.Fatalf("other content lost: %+v", file)
	}
	for _, step := range []struct {
		part  Part
		index int
	}{{PartFile, 0}, {PartTrack, 0}, {PartRoute, 0}, {PartWaypoint, 0}, {PartWaypoint, 1}} {
		if err := SetTranslations(path, step.part, step.index, "", nil); err != nil {
			t.Fatal(step.part, err)
		}
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "dgs:name") || strings.Contains(string(data), "dgs:lang") || strings.Contains(string(data), "<extensions></extensions>") {
		t.Fatalf("translations left behind:\n%s", data)
	}
	file, err = Open(path)
	if err != nil || file.Waypoints[0].CaptureID != "c1" {
		t.Fatalf("capture id lost: %v %+v", err, file)
	}
}

// A route added to a file whose waypoint carries <extensions> goes before the
// tracks, not into that waypoint.
func TestAddRouteAfterWaypointExtensions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.gpx")
	if err := CreateAll(path, "", []Waypoint{{Name: "w", Translations: map[string]string{"en": "W"}}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := AddRoute(path, Route{Name: "r", Points: []Point{{}}}); err != nil {
		t.Fatal(err)
	}
	file, err := Open(path)
	if err != nil || len(file.Routes) != 1 || len(file.Waypoints) != 1 || file.Waypoints[0].Translations["en"] != "W" {
		t.Fatalf("%v %+v", err, file)
	}
}

func TestSetTranslationsRefusesOtherFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.gpx")
	os.WriteFile(path, []byte(`<gpx version="1.1" creator="other"><trk><name>t</name></trk></gpx>`), 0o644)
	if err := SetTranslations(path, PartTrack, 0, "", map[string]string{"en": "x"}); err == nil {
		t.Fatal("a recording was written")
	}
}

// Only dgs's namespace counts, and the last of one language wins, whatever
// the case of its tag.
func TestParseTranslations(t *testing.T) {
	file, err := Parse(strings.NewReader(`<gpx version="1.1" creator="dgs-toolbox" xmlns="http://www.topografix.com/GPX/1/1"><wpt lat="1" lon="1"><name>w</name><extensions>` +
		`<x:name xmlns:x="urn:other" xml:lang="fr">no</x:name>` +
		`<dgs:name xmlns:dgs="urn:dgs-toolbox:gpx:1" xml:lang="EN">first</dgs:name>` +
		`<dgs:name xmlns:dgs="urn:dgs-toolbox:gpx:1" xml:lang="en"> second </dgs:name>` +
		`<dgs:name xmlns:dgs="urn:dgs-toolbox:gpx:1">no tag</dgs:name>` +
		`<dgs:name xmlns:dgs="urn:dgs-toolbox:gpx:1" lang="fr">not xml:lang</dgs:name>` +
		`<x:lang xmlns:x="urn:other">fr</x:lang><dgs:lang xmlns:dgs="urn:dgs-toolbox:gpx:1"> zh </dgs:lang>` +
		`</extensions></wpt></gpx>`))
	if err != nil {
		t.Fatal(err)
	}
	if file.Waypoints[0].Lang != "zh" {
		t.Fatalf("lang %q", file.Waypoints[0].Lang)
	}
	if got := file.Waypoints[0].Translations; !reflect.DeepEqual(got, map[string]string{"en": "second"}) {
		t.Fatalf("translations %v", got)
	}
}

// Parts added to a file whose only <extensions> is the metadata's go after
// the metadata, not into it.
func TestAddPartsAfterMetadataExtensions(t *testing.T) {
	for name, add := range map[string]func(string) error{
		"waypoint": func(path string) error { return AddWaypoint(path, Waypoint{Name: "w"}) },
		"route":    func(path string) error { return AddRoute(path, Route{Name: "r", Points: []Point{{}}}) },
		"track": func(path string) error {
			return Append(path, []Track{{Name: "t", Segments: []Segment{{Points: []Point{{}}}}}})
		},
	} {
		path := filepath.Join(t.TempDir(), "a.gpx")
		if err := CreateNamed(path, File{Name: "n", Lang: "zh"}, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := add(path); err != nil {
			t.Fatal(name, err)
		}
		file, err := Open(path)
		if err != nil || len(file.Waypoints)+len(file.Routes)+len(file.Tracks) != 1 || file.Lang != "zh" {
			data, _ := os.ReadFile(path)
			t.Fatalf("%s: %v\n%s", name, err, data)
		}
	}
}
