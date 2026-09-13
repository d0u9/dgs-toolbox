package geo

import (
	"testing"

	"dgs-toolbox/internal/config"
)

func TestGPXFlagsRejectBadValues(t *testing.T) {
	flags := New().Commands[0].Flags
	global := config.Default()
	for _, flag := range flags {
		if err := flag.Apply(&global, "nope"); err == nil {
			t.Errorf("--%s accepted %q", flag.Name, "nope")
		}
	}
	for name, value := range map[string]string{"host": "0.0.0.0", "port": "9000"} {
		for _, flag := range flags {
			if flag.Name == name {
				if err := flag.Apply(&global, value); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if got := global.GeoGPXAddr(); got != "0.0.0.0:9000" {
		t.Fatalf("addr = %q", got)
	}
	if got := config.Default().GeoGPXAddr(); got != "127.0.0.1:8765" {
		t.Fatalf("default addr = %q", got)
	}
}
