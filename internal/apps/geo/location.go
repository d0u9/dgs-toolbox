package geo

import (
	"dgs-toolbox/internal/desktop/location"
	"dgs-toolbox/internal/tui"
	"encoding/json"
	"fmt"
	"io"
)

func locationAction() tui.Action {
	return tui.Action{ID: "location", Description: "Print this device's current location (macOS).", Flags: []tui.ActionFlag{
		{Name: "json", Shorthand: "j", Bool: true, Usage: "output all location and address fields as JSON"},
		{Name: "format", Shorthand: "f", Default: location.DefaultFormat, Usage: "format with %latitude, %longitude, %altitude, %direction, %speed, %h_accuracy, %v_accuracy, %time, %address, %name, %isoCountryCode, %country, %postalCode, %administrativeArea, %subAdministrativeArea, %locality, %subLocality, %thoroughfare, %subThoroughfare, %region, %timeZone, %time_local"},
		{Name: "verbose", Shorthand: "v", Bool: true, Usage: "print system capabilities and authorization diagnostics"},
		{Name: "version", Bool: true, Usage: "print location command version"},
	}, Run: runLocation}
}

var getLocation = location.Get

func runLocation(_ io.Reader, out io.Writer, _ []string, flags map[string]string) error {
	if flags["version"] == "true" {
		_, err := fmt.Fprintln(out, "dgs geo location version 1")
		return err
	}
	format := flags["format"]
	result, err := getLocation(flags["json"] == "true" || location.NeedsPlacemark(format))
	if flags["verbose"] == "true" {
		for _, line := range result.Diagnostics {
			if _, e := fmt.Fprintln(out, line); e != nil {
				return e
			}
		}
	}
	if err != nil {
		return err
	}
	if flags["json"] == "true" {
		return json.NewEncoder(out).Encode(result.Values)
	}
	_, err = fmt.Fprintln(out, location.Format(format, result.Values))
	return err
}
