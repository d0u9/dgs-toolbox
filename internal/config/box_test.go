package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ExportFilename)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestBoxDefaultsWhenNothingIsConfigured(t *testing.T) {
	config, err := LoadPath(writeConfig(t, `{}`))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if got := config.BoxMarker(); got != DefaultBoxMarker {
		t.Errorf("BoxMarker = %q, want %q", got, DefaultBoxMarker)
	}
	if got := config.BoxStateFile(); got != DefaultBoxStateFile {
		t.Errorf("BoxStateFile = %q, want %q", got, DefaultBoxStateFile)
	}
	if got := config.BoxWorkers(); got != DefaultBoxWorkers {
		t.Errorf("BoxWorkers = %d, want %d", got, DefaultBoxWorkers)
	}
	if got := config.BoxWebAddr(); got != "127.0.0.1:8766" {
		t.Errorf("BoxWebAddr = %q, want 127.0.0.1:8766", got)
	}
	if got := config.BoxPreviewKeepDays(); got != DefaultBoxPreviewKeepDays {
		t.Errorf("BoxPreviewKeepDays = %d, want %d", got, DefaultBoxPreviewKeepDays)
	}
	if got := config.BoxTrashKeepDays(); got != DefaultBoxTrashKeepDays {
		t.Errorf("BoxTrashKeepDays = %d, want %d", got, DefaultBoxTrashKeepDays)
	}
	if got := config.BoxCurrency(); got != "" {
		t.Errorf("BoxCurrency = %q, want empty so every amount names its own", got)
	}
	if got := config.BoxRoot(); got != "" {
		t.Errorf("BoxRoot = %q, want empty so the command asks", got)
	}
}

func TestBoxNeverListensBeyondLoopback(t *testing.T) {
	// There is no host key on purpose: a Box holds identity and medical
	// documents, so reaching it from another machine is a design with access
	// control rather than a setting.
	config, err := LoadPath(writeConfig(t, `{"box": {"web": {"port": 9000}}}`))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if got := config.BoxWebAddr(); got != "127.0.0.1:9000" {
		t.Errorf("BoxWebAddr = %q, want 127.0.0.1:9000", got)
	}
	if _, err := LoadPath(writeConfig(t, `{"box": {"web": {"host": "0.0.0.0"}}}`)); err == nil {
		t.Error("a host key was accepted; box.web has only a port")
	}
}

func TestBoxKeepDaysTellZeroApartFromUnset(t *testing.T) {
	// Zero is a meaningful answer for both — keep previews forever, advise
	// nothing about the trash — so an unset key cannot be spelled as zero.
	config, err := LoadPath(writeConfig(t, `{"box": {"preview": {"keep": 0}, "trash": {"keep": 0}}}`))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if got := config.BoxPreviewKeepDays(); got != 0 {
		t.Errorf("BoxPreviewKeepDays = %d, want 0 to mean forever", got)
	}
	if got := config.BoxTrashKeepDays(); got != 0 {
		t.Errorf("BoxTrashKeepDays = %d, want 0 to mean no advice", got)
	}
}

func TestBoxRefusesWhatWouldBeWrongLater(t *testing.T) {
	for _, test := range []struct{ name, body, wants string }{
		{"a currency with unknown decimals", `{"box": {"currency": "ZZZ"}}`, "box.currency"},
		{"a path where a filename belongs", `{"box": {"marker": "meta/dgs-box.yaml"}}`, "box.marker"},
		{"a path in the state filename", `{"box": {"state_file": "../state.json"}}`, "box.state_file"},
		{"an offset instead of a zone", `{"box": {"timezone": "+10:00"}}`, "box.timezone"},
		{"a zone that is not one", `{"box": {"timezone": "Middle/Earth"}}`, "box.timezone"},
		{"negative workers", `{"box": {"workers": -1}}`, "box.workers"},
		{"negative preview days", `{"box": {"preview": {"keep": -1}}}`, "box.preview.keep"},
		{"negative trash days", `{"box": {"trash": {"keep": -5}}}`, "box.trash.keep"},
		{"a key that does not exist", `{"box": {"lifetimes": {"receipt": 3650}}}`, "lifetimes"},
	} {
		_, err := LoadPath(writeConfig(t, test.body))
		if err == nil {
			t.Errorf("%s: accepted %s", test.name, test.body)
			continue
		}
		if !strings.Contains(err.Error(), test.wants) {
			t.Errorf("%s: error %q does not name %q", test.name, err, test.wants)
		}
	}
}

func TestBoxAcceptsACurrencyWithoutTwoDecimals(t *testing.T) {
	for _, code := range []string{"AUD", "aud", " JPY ", "KWD"} {
		config, err := LoadPath(writeConfig(t, `{"box": {"currency": "`+code+`"}}`))
		if err != nil {
			t.Errorf("LoadPath with currency %q: %v", code, err)
			continue
		}
		if got := config.BoxCurrency(); got != strings.ToUpper(strings.TrimSpace(code)) {
			t.Errorf("BoxCurrency = %q for %q", got, code)
		}
	}
}

func TestBoxZone(t *testing.T) {
	config, err := LoadPath(writeConfig(t, `{"box": {"timezone": "Asia/Tokyo"}}`))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	zone, err := config.BoxZone()
	if err != nil {
		t.Fatalf("BoxZone: %v", err)
	}
	if zone.String() != "Asia/Tokyo" {
		t.Errorf("BoxZone = %q, want Asia/Tokyo", zone)
	}
	empty, err := LoadPath(writeConfig(t, `{}`))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if zone, err := empty.BoxZone(); err != nil || zone == nil {
		t.Errorf("BoxZone with nothing configured = %v, %v; want the machine's zone", zone, err)
	}
}

func TestBoxPathsAreExpanded(t *testing.T) {
	t.Setenv("BOX_TEST_ROOT", "/Volumes/nas/Box")
	config, err := LoadPath(writeConfig(t, `{"box": {"root": "$BOX_TEST_ROOT", "inbox": "~/Scans/inbox"}}`))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if got := config.BoxRoot(); got != "/Volumes/nas/Box" {
		t.Errorf("BoxRoot = %q, want the expanded value", got)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory to expand against")
	}
	if got, want := config.BoxInbox(), filepath.Join(home, "Scans", "inbox"); got != want {
		t.Errorf("BoxInbox = %q, want %q", got, want)
	}
}

func TestBoxCacheDirIsOutsideTheBoxAndPerBox(t *testing.T) {
	config, err := LoadPath(writeConfig(t, `{"box": {"root": "/Volumes/nas/Box", "cache_dir": "/tmp/box-cache"}}`))
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	first, err := config.BoxCacheDir("/Volumes/nas/Box")
	if err != nil {
		t.Fatalf("BoxCacheDir: %v", err)
	}
	if !strings.HasPrefix(first, "/tmp/box-cache/") {
		t.Errorf("BoxCacheDir = %q, want it under the configured cache directory", first)
	}
	if strings.HasPrefix(first, "/Volumes/nas/Box") {
		t.Errorf("BoxCacheDir = %q, but a cache must never live inside the Box", first)
	}
	// A trailing separator is the same Box, and a different Box is a different
	// cache.
	same, err := config.BoxCacheDir("/Volumes/nas/Box/")
	if err != nil {
		t.Fatalf("BoxCacheDir: %v", err)
	}
	if same != first {
		t.Errorf("BoxCacheDir differs for the same root written two ways: %q and %q", first, same)
	}
	other, err := config.BoxCacheDir("/Volumes/nas/Other")
	if err != nil {
		t.Fatalf("BoxCacheDir: %v", err)
	}
	if other == first {
		t.Error("two Boxes share one cache directory")
	}
}

func TestBoxDefaultConfigIsTheDocumentedOne(t *testing.T) {
	defaults := Default()
	if defaults.Box.Marker != DefaultBoxMarker || defaults.Box.StateFile != DefaultBoxStateFile {
		t.Errorf("Default() box filenames = %q, %q", defaults.Box.Marker, defaults.Box.StateFile)
	}
	if defaults.Box.Currency != "" {
		t.Errorf("Default() box.currency = %q, want empty: an exported default should not guess a country", defaults.Box.Currency)
	}
	for _, name := range []string{defaults.Box.Marker, defaults.Box.StateFile} {
		if strings.HasPrefix(name, ".") {
			t.Errorf("%q is a hidden file; nothing a person needs in a Box is hidden", name)
		}
	}
}
