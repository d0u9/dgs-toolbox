package sidecar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWithoutSidecarGivesDefaults(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	file, found, err := Load(source)
	if err != nil || found || file.Clean.Active() || file.Clean.Spikes.MaxSpeed != 100 {
		t.Fatalf("file %+v found %v err %v", file, found, err)
	}
}

func TestSaveAndLoad(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	file, _, _ := Load(source)
	file.Clean.Spikes.Enabled = true
	file.Clean.Spikes.MaxSpeed = 250
	file.Clean.Removed = []int{3, 4}
	if err := Save(source, file); err != nil {
		t.Fatal(err)
	}
	back, found, err := Load(source)
	if err != nil || !found || !back.Clean.Spikes.Enabled || back.Clean.Spikes.MaxSpeed != 250 || len(back.Clean.Removed) != 2 {
		t.Fatalf("back %+v found %v err %v", back, found, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(source))
	if len(entries) != 1 || entries[0].Name() != "walk.gpx.dgs.json" {
		t.Fatalf("directory holds %v", entries)
	}
}

func TestPartialSidecarKeepsDefaults(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	if err := os.WriteFile(PathFor(source), []byte(`{"version":1,"clean":{"spikes":{"enabled":true}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	file, found, err := Load(source)
	if err != nil || !found || !file.Clean.Spikes.Enabled || file.Clean.Spikes.MaxSpeed != 100 || file.Clean.Drift.HalfWindow != 7 {
		t.Fatalf("file %+v", file.Clean)
	}
}

func TestSavingNothingRemovesTheSidecar(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	file, _, _ := Load(source)
	file.Clean.Removed = []int{1}
	_ = Save(source, file)
	file.Clean.Removed = nil
	if err := Save(source, file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PathFor(source)); !os.IsNotExist(err) {
		t.Fatalf("sidecar still there: %v", err)
	}
}

func TestLoadRefusesBrokenOrNewer(t *testing.T) {
	source := filepath.Join(t.TempDir(), "walk.gpx")
	for _, body := range []string{"not json", `{"version":99}`} {
		_ = os.WriteFile(PathFor(source), []byte(body), 0o644)
		if _, _, err := Load(source); err == nil {
			t.Fatalf("%q loaded", body)
		}
	}
}
