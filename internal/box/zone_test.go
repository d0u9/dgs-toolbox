package box

import (
	"testing"
	"time"
)

func TestLoadZoneFindsIANANamesWithoutTheHost(t *testing.T) {
	// The zone database is compiled in, so this has to work on a machine with
	// no /usr/share/zoneinfo — which is the whole point of importing tzdata.
	for _, name := range []string{"Australia/Sydney", "Asia/Tokyo", "Europe/Berlin", "UTC"} {
		location, err := LoadZone(name)
		if err != nil {
			t.Errorf("LoadZone(%q): %v", name, err)
			continue
		}
		if ZoneName(location) != name {
			t.Errorf("LoadZone(%q) is called %q", name, ZoneName(location))
		}
	}
}

func TestLoadZoneRefusesAnOffset(t *testing.T) {
	// An offset is what a zone happened to be at one moment. Sydney is +10:00
	// in June and +11:00 in December, so storing the offset loses the zone.
	for _, name := range []string{"+10:00", "-05:00", "UTC+10", "GMT-5"} {
		if location, err := LoadZone(name); err == nil {
			t.Errorf("LoadZone(%q) = %v, want an error naming daylight saving", name, location)
		}
	}
}

func TestEmptyZoneIsTheMachinesOwn(t *testing.T) {
	location, err := LoadZone("  ")
	if err != nil {
		t.Fatalf("LoadZone(empty): %v", err)
	}
	if location != time.Local {
		t.Errorf("LoadZone(empty) = %v, want the machine's zone", location)
	}
}

func TestLoadZoneRefusesANameThatIsNotAZone(t *testing.T) {
	if location, err := LoadZone("Middle/Earth"); err == nil {
		t.Errorf("LoadZone invented a zone: %v", location)
	}
}

func TestAZoneKeepsDaylightSavingRatherThanOneOffset(t *testing.T) {
	sydney, err := LoadZone("Australia/Sydney")
	if err != nil {
		t.Fatalf("LoadZone: %v", err)
	}
	_, june := time.Date(2019, 6, 1, 12, 0, 0, 0, sydney).Zone()
	_, december := time.Date(2019, 12, 1, 12, 0, 0, 0, sydney).Zone()
	if june == december {
		t.Errorf("Sydney's offset is %d in both June and December; the zone database is not being used", june)
	}
}
