package organizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// starterTables are the mapping tables this version ships, loaded the way a
// run loads them.
func starterTables(t *testing.T) map[string]map[string]string {
	t.Helper()
	dir := t.TempDir()
	for _, starter := range StartersOf(StarterMappings) {
		if err := os.WriteFile(filepath.Join(dir, starter.Name), starter.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loaded := LoadMappings(dir)
	if len(loaded.Failures) > 0 {
		t.Fatalf("the shipped mappings do not load: %v", loaded.Failures)
	}
	return loaded.Tables
}

// One file is one table, named after it, and the file is the table itself.
func TestAMappingFileIsTheTable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "country.yaml"), []byte("Australia: 🇦🇺_Australia\nChina: 🇨🇳_中国\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := LoadMappings(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	if got := loaded.Tables["country"]["Australia"]; got != "🇦🇺_Australia" {
		t.Fatalf("country = %q", got)
	}
	settings := DefaultSettings()
	settings.Mappings = loaded.Tables
	// A value the table does not mention comes back as it was.
	if got := settings.Mappings["country"]["Japan"]; got != "" {
		t.Fatalf("Japan = %q, want nothing so the value is written as it was", got)
	}
}

// A bad file fails alone, the way a bad Recipe does.
func TestABadMappingFileFailsAlone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "good.yaml"), []byte("a: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty.yaml"), []byte("# nothing here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := LoadMappings(dir)
	if len(loaded.Failures) != 1 || !strings.Contains(loaded.Failures[0].Err.Error(), "translates nothing") {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	if _, ok := loaded.Tables["good"]; !ok {
		t.Fatal("the good file was dropped with the bad one")
	}
}

// The shipped tables load, and the weekday one the location note asks for is
// among them.
func TestShippedMappingsLoad(t *testing.T) {
	dir := t.TempDir()
	for _, starter := range StartersOf(StarterMappings) {
		if err := os.WriteFile(filepath.Join(dir, starter.Name), starter.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loaded := LoadMappings(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("the shipped mappings do not load: %v", loaded.Failures)
	}
	if got := loaded.Tables["weekday"]["Monday"]; got != "周一" {
		t.Fatalf("weekday = %q", got)
	}
}
