package gpxfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdatePublishesOnlyAfterAllEditsSucceed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	if err := Create(path, "Trip", sampleTracks()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("synthetic edit failure")
	err = Update(path, func(staged string) error {
		if err := Append(staged, sampleTracks()); err != nil {
			return err
		}
		visible, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, visible) {
			t.Fatal("staging published early")
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("edit error: %v", err)
	}
	visible, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, visible) {
		t.Fatal("failed update changed original")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatal("failed update left temporary files")
	}
	if err := Update(path, func(staged string) error { return Append(staged, sampleTracks()) }); err != nil {
		t.Fatal(err)
	}
	parsed, err := Open(path)
	if err != nil || len(parsed.Tracks) != 2 {
		t.Fatal("successful update missing changes", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("update changed permissions")
	}
}

func TestUpdateRefusesRecordingAndInvalidResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.gpx")
	original := []byte(`<gpx creator="recorder"></gpx>`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Update(path, func(string) error { t.Fatal("recording edited"); return nil }); !errors.Is(err, ErrNotOurs) {
		t.Fatal("recording accepted", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := Create(path, "Trip", sampleTracks()); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`<gpx creator="recorder"></gpx>`, `<gpx`} {
		if err := Update(path, func(staged string) error { return os.WriteFile(staged, []byte(invalid), 0600) }); err == nil {
			t.Fatal("invalid result published")
		}
		visible, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(visible, original) {
			t.Fatal("invalid update changed original")
		}
	}
}
