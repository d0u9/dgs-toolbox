package geo

import (
	"bytes"
	"dgs-toolbox/internal/desktop/location"
	"encoding/json"
	"testing"
)

func TestLocationOutput(t *testing.T) {
	old := getLocation
	defer func() { getLocation = old }()
	lookups := []bool{}
	getLocation = func(lookup bool) (location.Result, error) {
		lookups = append(lookups, lookup)
		lat := "1.000000"
		lon := "2.000000"
		return location.Result{Values: map[string]*string{"latitude": &lat, "longitude": &lon, "address": nil}}, nil
	}
	var out bytes.Buffer
	if err := runLocation(nil, &out, nil, map[string]string{"format": location.DefaultFormat}); err != nil || out.String() != "1.000000 2.000000\n" {
		t.Fatal(out.String(), err)
	}
	out.Reset()
	if err := runLocation(nil, &out, nil, map[string]string{"json": "true"}); err != nil {
		t.Fatal(err)
	}
	var values map[string]*string
	if err := json.Unmarshal(out.Bytes(), &values); err != nil || values["address"] != nil || *values["latitude"] != "1.000000" {
		t.Fatal(out.String(), err)
	}
	out.Reset()
	runLocation(nil, &out, nil, map[string]string{"version": "true"})
	if len(lookups) != 2 || lookups[0] || !lookups[1] {
		t.Fatal(lookups)
	}
}
