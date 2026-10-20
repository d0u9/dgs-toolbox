package gpx

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
)

// A translation is held like a name, shown at once, and written into a GPX
// dgs wrote when the file is saved; undo takes the save back.
func TestTranslationsHeldThenWrittenIntoOurGPX(t *testing.T) {
	path := ownRenameGPX(t)
	s := newStepper(t, path)
	en := map[string]any{"en": "Walk in English"}
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "散步", "translations": en})
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": "旅行", "translations": map[string]any{"en": "Trip"}})
	shown := s.shown()
	if shown.Translations["en"] != "Trip" || shown.Parts[0].Translations["en"] != "Walk in English" || !shown.Unsaved {
		t.Fatalf("held: %v %v", shown.Translations, shown.Parts[0].Translations)
	}
	if parsed, _ := gpxfile.Open(path); parsed.Tracks[0].Translations != nil {
		t.Fatal("written before save")
	}
	s.ok(http.MethodPost, "/api/save", nil)
	parsed, err := gpxfile.Open(path)
	if err != nil || parsed.Name != "旅行" || parsed.Translations["en"] != "Trip" ||
		parsed.Tracks[0].Name != "散步" || parsed.Tracks[0].Translations["en"] != "Walk in English" {
		t.Fatalf("saved: %+v %v", parsed, err)
	}
	if s.shown().Unsaved {
		t.Fatal("nothing should be held after save")
	}
	// Sending none takes them out.
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "散步", "translations": map[string]any{}})
	s.ok(http.MethodPost, "/api/save", nil)
	if parsed, _ := gpxfile.Open(path); parsed.Tracks[0].Translations != nil {
		t.Fatalf("not taken out: %v", parsed.Tracks[0].Translations)
	}
	s.step(true)
	if parsed, _ := gpxfile.Open(path); parsed.Tracks[0].Translations["en"] != "Walk in English" {
		t.Fatalf("undo did not give the translation back: %v", parsed.Tracks[0].Translations)
	}
}

// Leaving translations out of a rename keeps them; giving back the file's
// own holds nothing.
func TestTranslationsLeftOutOrUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	err := gpxfile.Create(path, "Trip", []gpxfile.Track{{Name: "Walk", Translations: map[string]string{"zh": "散步"}, Segments: []gpxfile.Segment{{Points: []gpxfile.Point{{}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Walk 2"})
	if got := s.shown().Parts[0].Translations; got["zh"] != "散步" {
		t.Fatalf("translations lost: %v", got)
	}
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Walk 2", "translations": map[string]any{"zh": "散步"}})
	if held, _ := pending.get(path); held.Translations != nil {
		t.Fatalf("unchanged translations held: %v", held.Translations)
	}
}

func TestTranslationsRefuseBadInput(t *testing.T) {
	path := ownRenameGPX(t)
	s := newStepper(t, path)
	for _, translations := range []map[string]any{{"english!": "x"}, {"en": "two\nlines"}, {"en": "a", "EN": "b"}} {
		if code, _ := s.call(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "a", "translations": translations}); code != http.StatusBadRequest {
			t.Fatalf("%v = %d", translations, code)
		}
	}
}

// A recording is never written: its translations stay in the sidecar, and
// Save as carries them into the new GPX.
func TestTranslationsOfRecordingGoToSidecarAndSaveAs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rec.gpx")
	recorded := []byte(`<gpx version="1.1" creator="device"><wpt lat="1" lon="1"><name>w</name></wpt><rte><name>r</name><rtept lat="1" lon="1"/></rte><trk><name>t</name><trkseg><trkpt lat="1" lon="1"/><trkpt lat="1.1" lon="1"/></trkseg></trk></gpx>`)
	if err := os.WriteFile(path, recorded, 0o644); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	for _, key := range []string{"f", "t0", "r0", "w0"} {
		s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": key, "name": "名" + key, "translations": map[string]any{"en": "Name " + key}})
	}
	s.ok(http.MethodPost, "/api/save", nil)
	if data, _ := os.ReadFile(path); string(data) != string(recorded) {
		t.Fatal("the recording was written")
	}
	side, _, err := sidecar.Load(path)
	if err != nil || side.Translations["w0"]["en"] != "Name w0" {
		t.Fatalf("sidecar: %+v %v", side.Translations, err)
	}
	target := filepath.Join(dir, "out.gpx")
	s.ok(http.MethodPost, "/api/save-as", map[string]any{"path": path, "target": target})
	out, err := gpxfile.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	got := []map[string]string{out.Translations, out.Tracks[0].Translations, out.Routes[0].Translations, out.Waypoints[0].Translations}
	want := []map[string]string{{"en": "Name f"}, {"en": "Name t0"}, {"en": "Name r0"}, {"en": "Name w0"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("save as: %v", got)
	}
}

// Copying a part carries its translations; deleting one carried beside the
// file moves the later ones' keys down with their translations.
func TestTranslationsFollowCopyAndDelete(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "a.gpx")
	target := filepath.Join(dir, "b.gpx")
	for _, path := range []string{source, target} {
		if err := gpxfile.CreateAll(path, "", nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := gpxfile.CreateAll(source+".tmp", "", []gpxfile.Waypoint{{Name: "a", Translations: map[string]string{"en": "A"}}, {Name: "b"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	os.Rename(source+".tmp", source)
	s := newStepper(t, source)
	t.Cleanup(func() { pending.forget(target) })
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": source, "key": "w1", "name": "b", "translations": map[string]any{"en": "B"}})
	s.ok(http.MethodPost, "/api/part/copy", map[string]any{"path": source, "key": "w0", "target": target})
	s.ok(http.MethodPost, "/api/part/copy", map[string]any{"path": source, "key": "w1", "target": target})
	var shown trackJSON
	get(t, s.server, "/api/track?path="+url.QueryEscape(target), &shown)
	if shown.Parts[1].Translations["en"] != "B" {
		t.Fatalf("copy lost translations: %+v", shown.Parts)
	}
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": target, "key": "w1", "name": "b", "translations": map[string]any{"en": "B2"}})
	s.ok(http.MethodDelete, "/api/part", map[string]any{"path": target, "key": "w0"})
	s.ok(http.MethodPost, "/api/save", map[string]any{"paths": []string{target}})
	out, err := gpxfile.Open(target)
	if err != nil || len(out.Waypoints) != 1 || out.Waypoints[0].Translations["en"] != "B2" {
		t.Fatalf("target: %+v %v", out, err)
	}
	if data, _ := os.ReadFile(target); strings.Count(string(data), "dgs:name") != 2 {
		t.Fatalf("expected one translation element:\n%s", data)
	}
}

// Restoring the GPX's own name restores its own translations too.
func TestRestoreNameRestoresTranslations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	if err := gpxfile.CreateNamed(path, gpxfile.File{Name: "Trip", Translations: map[string]string{"zh": "旅行"}}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": "Hike"})
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": "Hike", "translations": map[string]any{"zh": "徒步"}})
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": ""})
	shown := s.shown()
	if shown.Name != "Trip" || shown.Translations["zh"] != "旅行" {
		t.Fatalf("restored: %q %v", shown.Name, shown.Translations)
	}
}

// The language of a name is held like a name and written with the
// translations; the file's is the default, and a copied part takes the
// language it had as its own.
func TestNameLanguages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.gpx")
	target := filepath.Join(dir, "b.gpx")
	if err := gpxfile.CreateAll(path, "西湖", []gpxfile.Waypoint{{Name: "断桥"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := gpxfile.CreateAll(target, "", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	t.Cleanup(func() { pending.forget(target) })
	if code, _ := s.call(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": "西湖", "lang": "not a tag"}); code != http.StatusBadRequest {
		t.Fatalf("bad tag = %d", code)
	}
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": "西湖", "lang": "zh"})
	shown := s.shown()
	if shown.Lang != "zh" || shown.Parts[0].Lang != "" {
		t.Fatalf("held: file %q part %q", shown.Lang, shown.Parts[0].Lang)
	}
	s.ok(http.MethodPost, "/api/save", nil)
	if parsed, _ := gpxfile.Open(path); parsed.Lang != "zh" || parsed.Waypoints[0].Lang != "" {
		t.Fatalf("saved: %+v", parsed)
	}
	// Giving back what the file says holds nothing.
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": "西湖", "lang": "zh"})
	if held, _ := pending.get(path); held.Langs != nil {
		t.Fatalf("unchanged language held: %v", held.Langs)
	}
	s.ok(http.MethodPost, "/api/part/copy", map[string]any{"path": path, "key": "w0", "target": target})
	s.ok(http.MethodPost, "/api/save", map[string]any{"paths": []string{target}})
	if out, _ := gpxfile.Open(target); out.Lang != "" || out.Waypoints[0].Lang != "zh" {
		t.Fatalf("copied: %+v", out)
	}
	// A part's own language overrides the file's, and "" gives it back.
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "w0", "name": "Broken Bridge", "lang": "en"})
	s.ok(http.MethodPost, "/api/save", nil)
	if parsed, _ := gpxfile.Open(path); parsed.Waypoints[0].Lang != "en" {
		t.Fatalf("own: %+v", parsed.Waypoints[0])
	}
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "w0", "name": "Broken Bridge", "lang": ""})
	s.ok(http.MethodPost, "/api/save", nil)
	if data, _ := os.ReadFile(path); strings.Count(string(data), "dgs:lang") != 2 {
		t.Fatalf("expected only the file's language:\n%s", data)
	}
}

// Confirming the rename dialog as it opened holds no edit: the file stays
// saved and there is nothing to undo.
func TestUnchangedRenameHoldsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	err := gpxfile.CreateNamed(path, gpxfile.File{Name: "旅行", Lang: "zh", Translations: map[string]string{"en": "Trip"}}, nil, nil,
		[]gpxfile.Track{{Name: "Walk", Segments: []gpxfile.Segment{{Points: []gpxfile.Point{{}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": "旅行", "lang": "zh", "translations": map[string]any{"en": "Trip"}})
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Walk", "lang": "", "translations": map[string]any{}})
	if shown := s.shown(); shown.Unsaved || shown.Undo != 0 {
		t.Fatalf("unsaved %v, undo %d", shown.Unsaved, shown.Undo)
	}
	// Renaming away and back again leaves nothing held either.
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Other"})
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": "Walk"})
	if held, _ := pending.get(path); held.Names != nil {
		t.Fatalf("names held: %v", held.Names)
	}
}

// A save that writes the tracks as cleaned keeps the tracks' languages and
// translations, held ones included.
func TestCleanedSaveKeepsTranslations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	points := []gpxfile.Point{{}, {}, {}}
	for i := range points {
		points[i].Lat = float64(i) / 1000
	}
	err := gpxfile.Create(path, "Trip", []gpxfile.Track{
		{Name: "A", Lang: "en", Translations: map[string]string{"zh": "甲"}, Segments: []gpxfile.Segment{{Points: points}}},
		{Name: "B", Segments: []gpxfile.Segment{{Points: points}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t1", "name": "B", "lang": "en", "translations": map[string]any{"zh": "乙"}})
	s.lasso(1)
	s.ok(http.MethodPost, "/api/save", nil)
	parsed, err := gpxfile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Tracks[0].Segments[0].Points) != 2 {
		t.Fatalf("not cleaned: %d points", len(parsed.Tracks[0].Segments[0].Points))
	}
	for i, want := range []string{"甲", "乙"} {
		if trk := parsed.Tracks[i]; trk.Lang != "en" || trk.Translations["zh"] != want {
			t.Fatalf("track %d: %q %v", i, trk.Lang, trk.Translations)
		}
	}
}

// A file and a track without names of their own show the file name and a
// number; confirming those unchanged is no rename either.
func TestUnchangedRenameOfUnnamedHoldsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	if err := gpxfile.Create(path, "", []gpxfile.Track{{Segments: []gpxfile.Segment{{Points: []gpxfile.Point{{}}}}}}); err != nil {
		t.Fatal(err)
	}
	s := newStepper(t, path)
	shown := s.shown()
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "f", "name": shown.Name})
	s.ok(http.MethodPut, "/api/part-name", map[string]any{"path": path, "key": "t0", "name": shown.Parts[0].Name})
	if after := s.shown(); after.Unsaved || after.Undo != 0 || shown.Name != "trip" || shown.Parts[0].Name != "Track 1" {
		t.Fatalf("shown %q %q; unsaved %v undo %d", shown.Name, shown.Parts[0].Name, after.Unsaved, after.Undo)
	}
}

// A part copied into a GPX whose default is its language writes none of its
// own; into one with another default, it keeps it.
func TestCopiedPartInTargetsLanguageWritesNone(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "a.gpx")
	same := filepath.Join(dir, "zh.gpx")
	other := filepath.Join(dir, "en.gpx")
	if err := gpxfile.CreateNamed(source, gpxfile.File{Name: "西湖", Lang: "zh"}, []gpxfile.Waypoint{{Name: "断桥"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	for path, lang := range map[string]string{same: "ZH", other: "en"} {
		if err := gpxfile.CreateNamed(path, gpxfile.File{Name: "t", Lang: lang}, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	s := newStepper(t, source)
	t.Cleanup(func() { pending.forget(same); pending.forget(other) })
	for _, target := range []string{same, other} {
		s.ok(http.MethodPost, "/api/part/copy", map[string]any{"path": source, "key": "w0", "target": target})
	}
	s.ok(http.MethodPost, "/api/save", map[string]any{"paths": []string{same, other}})
	if out, _ := gpxfile.Open(same); out.Waypoints[0].Lang != "" {
		t.Fatalf("same default: %q", out.Waypoints[0].Lang)
	}
	if out, _ := gpxfile.Open(other); out.Waypoints[0].Lang != "zh" {
		t.Fatalf("other default: %q", out.Waypoints[0].Lang)
	}
}
