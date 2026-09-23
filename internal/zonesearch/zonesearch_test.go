package zonesearch_test

import (
	"testing"
	"time"

	"dgs-toolbox/internal/zonesearch"
)

func names(zones []zonesearch.Zone) map[string]int {
	out := map[string]int{}
	for i, zone := range zones {
		out[zone.Name] = i
	}
	return out
}

func TestCityPrefixComesFirst(t *testing.T) {
	got := zonesearch.Search("syd", 0)
	if len(got) == 0 || got[0].Name != "Australia/Sydney" {
		t.Fatalf("got %+v", got)
	}
}

func TestCountryFindsItsZones(t *testing.T) {
	found := names(zonesearch.Search("aus", 40))
	for _, want := range []string{"Australia/Sydney", "Australia/Melbourne", "Australia/Perth", "Europe/Vienna"} {
		if _, ok := found[want]; !ok {
			t.Errorf("aus: missing %s", want)
		}
	}
}

func TestChiFindsChinaChicagoAndChile(t *testing.T) {
	found := names(zonesearch.Search("chi", 40))
	for _, want := range []string{"Asia/Shanghai", "America/Chicago", "America/Santiago"} {
		if _, ok := found[want]; !ok {
			t.Errorf("chi: missing %s", want)
		}
	}
}

func TestEveryWordMustMatch(t *testing.T) {
	got := zonesearch.Search("australia mel", 0)
	if len(got) != 1 || got[0].Name != "Australia/Melbourne" {
		t.Fatalf("got %+v", got)
	}
	if got := zonesearch.Search("   ", 0); got != nil {
		t.Errorf("blank: %+v", got)
	}
}

func TestLimit(t *testing.T) {
	if got := zonesearch.Search("a", 3); len(got) != 3 {
		t.Fatalf("got %d", len(got))
	}
}

// Every zone offered must be one this build can load, or choosing it would
// be refused on save.
func TestEveryZoneLoads(t *testing.T) {
	for _, zone := range zonesearch.All() {
		if _, err := time.LoadLocation(zone.Name); err != nil {
			t.Errorf("%s: %v", zone.Name, err)
		}
	}
}
