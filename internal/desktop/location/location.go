// Package location obtains a device location through the operating system.
package location

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

var ErrUnavailable = errors.New("location needs dgs built on macOS with cgo")

const DefaultFormat = "%latitude %longitude"
const TimeoutSeconds = 10

var fields = []string{"latitude", "longitude", "altitude", "direction", "speed", "h_accuracy", "v_accuracy", "time", "address", "name", "isoCountryCode", "country", "postalCode", "administrativeArea", "subAdministrativeArea", "locality", "subLocality", "thoroughfare", "subThoroughfare", "region", "timeZone", "time_local"}

type Result struct {
	Values      map[string]*string `json:"values"`
	Diagnostics []string           `json:"diagnostics"`
	Error       string             `json:"error"`
}

var bridge = native

func Get(placemark bool) (Result, error) {
	data, err := bridge(placemark)
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err = json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	if result.Error != "" {
		return result, errors.New(result.Error)
	}
	for _, key := range fields {
		if _, ok := result.Values[key]; !ok {
			return result, errors.New("location returned incomplete data")
		}
	}
	return result, nil
}
func NeedsPlacemark(format string) bool {
	for _, field := range fields[8:] {
		if strings.Contains(format, "%"+field) {
			return true
		}
	}
	return false
}
func Format(format string, values map[string]*string) string {
	keys := append([]string(nil), fields...)
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	pairs := []string{}
	for _, key := range keys {
		value := ""
		if values[key] != nil {
			value = *values[key]
		}
		pairs = append(pairs, "%"+key, value)
	}
	return strings.NewReplacer(pairs...).Replace(format)
}
