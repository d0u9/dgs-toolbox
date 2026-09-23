package doctype

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNamesAreSlugsAndDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, entry := range All() {
		if seen[entry.Name] {
			t.Errorf("%s is registered twice", entry.Name)
		}
		seen[entry.Name] = true
		if entry.Name == "" || entry.Label == "" {
			t.Errorf("%+v has an empty name or label", entry)
		}
		for _, r := range entry.Name {
			if (r < 'a' || r > 'z') && r != '-' {
				t.Errorf("%q is not a lowercase ASCII slug; it goes into a sidecar and has to be greppable on any machine", entry.Name)
				break
			}
		}
	}
}

func TestIntakeKeysDoNotCollide(t *testing.T) {
	byKey := map[rune]string{}
	for _, entry := range All() {
		if entry.Key == 0 {
			t.Errorf("%s has no intake key", entry.Name)
			continue
		}
		if other, taken := byKey[entry.Key]; taken {
			t.Errorf("%s and %s both answer to %q", other, entry.Name, entry.Key)
		}
		byKey[entry.Key] = entry.Name
	}
}

func TestPermanentIsNotTheSameAsExpiringOnTheDay(t *testing.T) {
	ticket, _ := Lookup("ticket")
	if ticket.Permanent() {
		t.Error("a ticket stub expires on the event date; it is not permanent")
	}
	if *ticket.Lifetime != 0 {
		t.Errorf("ticket lifetime = %d days, want 0", *ticket.Lifetime)
	}
	contract, _ := Lookup("contract")
	if !contract.Permanent() {
		t.Error("a contract has no default expiry")
	}
}

func TestEveryKeepsakeIsPermanent(t *testing.T) {
	for _, entry := range All() {
		if entry.Nature == Keepsake && !entry.Permanent() {
			t.Errorf("%s is a keepsake with a lifetime of %d days; a keepsake is kept because its owner wants it, so it has no end", entry.Name, *entry.Lifetime)
		}
	}
}

func TestLookupKeepsAnUnknownNameUnknown(t *testing.T) {
	if _, known := Lookup("tax-notice"); known {
		t.Error("Lookup invented a type")
	}
	if entry, known := Lookup("receipt"); !known || entry.Label == "" {
		t.Errorf("Lookup(receipt) = %+v, %v", entry, known)
	}
}

// TestTypesDocumentNamesEveryType is the catalogue rule from AGENTS.md: the
// document is what someone reads to find out what a Box can hold, so a type
// that exists only in the code is a type nobody knows about. The test cannot
// check that a description is still true; that part belongs to the change.
func TestTypesDocumentNamesEveryType(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test's own directory")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "apps", "box", "types.md")
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the catalogue document: %v", err)
	}
	text := string(document)
	for _, entry := range All() {
		if !strings.Contains(text, "`"+entry.Name+"`") {
			t.Errorf("docs/apps/box/types.md does not name %q — add it there in the same change", entry.Name)
		}
	}
}
