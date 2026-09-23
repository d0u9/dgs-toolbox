package box

import (
	"fmt"
	"strings"
	"time"
	// The IANA zone database is compiled in rather than read from the host.
	// A Box records the zone an event date was read in by name — Asia/Tokyo,
	// not +09:00, because an offset recorded today is wrong when the same date
	// is read in another month — and dgs is one file that has to work when it
	// is copied to a machine with no /usr/share/zoneinfo. It costs about 450 KB.
	_ "time/tzdata"
)

// LoadZone finds an IANA zone by name: Australia/Sydney, Asia/Tokyo. An empty
// name is the machine's current zone, which is what an unset box.timezone
// means.
//
// An offset such as "+10:00" is refused rather than accepted as a zone. An
// offset is what a zone happened to be at one moment; storing it loses daylight
// saving, so the same date read in another month is off by an hour.
func LoadZone(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.Local, nil
	}
	if looksLikeOffset(name) {
		return nil, fmt.Errorf("zone %q is an offset: write an IANA name such as Australia/Sydney, so daylight saving is not lost", name)
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("unknown time zone %q", name)
	}
	return location, nil
}

func looksLikeOffset(name string) bool {
	if name == "UTC" || name == "Local" {
		return false
	}
	if strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") {
		return true
	}
	upper := strings.ToUpper(name)
	for _, prefix := range []string{"UTC+", "UTC-", "GMT+", "GMT-"} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

// ZoneName is the name to record for a location: what it is called, or empty
// for the machine's own zone, which is recorded by its name too so that a
// sidecar written here still says where it was written.
func ZoneName(location *time.Location) string {
	if location == nil {
		return ""
	}
	return location.String()
}
