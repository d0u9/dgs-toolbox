package location

import (
	"errors"
	"testing"
)

func TestFormat(t *testing.T) {
	v := "local"
	utc := "utc"
	values := map[string]*string{"time": &utc, "time_local": &v}
	if got := Format("%time_local %time %address %unknown", values); got != "local utc  %unknown" {
		t.Fatal(got)
	}
	v = "%time"
	if got := Format("%time_local", values); got != "%time" {
		t.Fatal("replacement expanded recursively", got)
	}
}
func TestNeedsPlacemark(t *testing.T) {
	for _, key := range fields {
		want := false
		for _, p := range fields[8:] {
			want = want || p == key
		}
		if got := NeedsPlacemark("%" + key); got != want {
			t.Errorf("%s: %v", key, got)
		}
	}
}
func TestBridgeFailure(t *testing.T) {
	old := bridge
	defer func() { bridge = old }()
	bridge = func(bool) ([]byte, error) { return []byte(`{"error":"denied","diagnostics":["status"]}`), nil }
	result, err := Get(false)
	if err == nil || err.Error() != "denied" || len(result.Diagnostics) != 1 {
		t.Fatal(result, err)
	}
	bridge = func(bool) ([]byte, error) { return nil, ErrUnavailable }
	if _, err = Get(false); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	bridge = func(bool) ([]byte, error) { return []byte(`{"values":{}}`), nil }
	if _, err = Get(false); err == nil {
		t.Fatal("accepted incomplete data")
	}
}
