package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeConfig lays body out as a configuration directory: each top-level key
// of body is a part, written to <dir>/<part>/config.json.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	var parts map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &parts); err != nil {
		t.Fatalf("test body: %v", err)
	}
	dir := t.TempDir()
	for name, raw := range parts {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, PartFile), raw, 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	return dir
}

// LoadPath loads dir and reports the first refused part as the error, as a
// command would refuse to start.
func LoadPath(dir string) (Config, error) {
	config, err := LoadDir(dir)
	if err != nil {
		return config, err
	}
	for _, p := range parts {
		if err := config.PartErr(p.name); err != nil {
			return config, err
		}
	}
	return config, nil
}

func TestWebEntriesNameTheServersACommandHas(t *testing.T) {
	for _, body := range []string{
		`{"geo":{"web":[{"name":"pages","port":9000}]}}`,
		`{"geo":{"web":[{"name":"gpx"},{"name":"gpx"}]}}`,
		`{"geo":{"web":[{"name":"gpx","port":70000}]}}`,
		`{"geo":{"web":{"port":9000}}}`,
	} {
		if _, err := LoadPath(writeConfig(t, body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}

	config, err := LoadPath(writeConfig(t, `{"geo": {"web": [{"name": "gpx", "host": "0.0.0.0", "port": 9001}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := config.GeoGPXAddr(); got != "0.0.0.0:9001" {
		t.Errorf("GeoGPXAddr = %q", got)
	}
	if got := (Config{}).GeoGPXAddr(); got != "127.0.0.1:8765" {
		t.Errorf("default GeoGPXAddr = %q", got)
	}
}
