package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExpandPath(t *testing.T) {
	env := map[string]string{"DOT": "/Users/d/.dot", "EMPTY": "", "REL": "relative"}
	lookup := func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	}
	for input, want := range map[string]string{
		"/abs/path":            "/abs/path",
		"~":                    "/home/d",
		"~/.ssh":               "/home/d/.ssh",
		"$DOT/conf/recipients": "/Users/d/.dot/conf/recipients",
		"${DOT}conf":           "/Users/d/.dotconf",
		"/a/../b/":             "/b",
	} {
		got, err := ExpandPath(input, lookup, "/home/d")
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", input, got, err, want)
		}
	}
	for input, fragment := range map[string]string{
		"":             "empty",
		"relative/dir": "not an absolute path",
		"~other/x":     "not an absolute path",
		"$REL/x":       "not an absolute path",
		"$MISSING/x":   "MISSING is not set",
		"$EMPTY/x":     "EMPTY is not set",
		"${DOT/x":      "unclosed",
		"/a/$/b":       "variable name",
		"/a/${1x}":     "variable name",
	} {
		if _, err := ExpandPath(input, lookup, "/home/d"); err == nil || !strings.Contains(err.Error(), fragment) {
			t.Errorf("%q: want error containing %q, got %v", input, fragment, err)
		}
	}
}

func TestLoadCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFilename)

	if _, found, err := LoadCredentials(path); found || err != nil {
		t.Fatalf("missing file: found %v, %v", found, err)
	}

	t.Setenv("CRED_TEST_ROOT", "/srv/conf")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	body := `{"identities":["~/.ssh","/opt/keys"],"recipients":"$CRED_TEST_ROOT/recipients","vault":"~/vault"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials, found, err := LoadCredentials(path)
	if err != nil || !found {
		t.Fatalf("load: found %v, %v", found, err)
	}
	if strings.Join(credentials.Identities, ",") != filepath.Join(home, ".ssh")+",/opt/keys" {
		t.Errorf("identities %q", credentials.Identities)
	}
	if credentials.NewIdentityDir != DefaultNewIdentityDir() {
		t.Errorf("new identity dir %q", credentials.NewIdentityDir)
	}
	if credentials.Vault != filepath.Join(home, "vault") {
		t.Errorf("vault %q", credentials.Vault)
	}
	if credentials.Recipients != "/srv/conf/recipients" {
		t.Errorf("recipients %q", credentials.Recipients)
	}

	for body, fragment := range map[string]string{
		`{"identity_dirs":[]}`:           "unknown field",
		`{"recipients":"relative"}`:      "recipients",
		`{"vault":"relative"}`:           "vault",
		`{"new_identity_dir":"rel"}`:     "new_identity_dir",
		`{"close_after":"soon"}`:         "close_after",
		`{"identities":["$CRED_UNSET"]}`: "identities[0]",
		`{} {}`:                          "content after",
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadCredentials(path); err == nil || !strings.Contains(err.Error(), fragment) {
			t.Errorf("%s: want error containing %q, got %v", body, fragment, err)
		}
	}

	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if credentials, found, err := LoadCredentials(path); err != nil || !found || len(credentials.Identities) != 0 || credentials.Recipients != "" || credentials.NewIdentityDir != DefaultNewIdentityDir() {
		t.Errorf("empty file: %+v %v %v", credentials, found, err)
	}
}

func TestIdleClose(t *testing.T) {
	for value, want := range map[string]time.Duration{"": DefaultCloseAfter, "0": 0, "90s": 90 * time.Second} {
		if got := (Credentials{CloseAfter: value}).IdleClose(); got != want {
			t.Errorf("%q: %v, want %v", value, got, want)
		}
	}
}

// TestArchiveSkip pins the three states of the key: absent means the seal
// package's own default, a list replaces it, and [] skips nothing.
func TestArchiveSkip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CredentialsFilename)
	load := func(body string) Credentials {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		credentials, _, err := LoadCredentials(path)
		if err != nil {
			t.Fatal(err)
		}
		return credentials
	}
	if skip := load(`{}`).Skip(); skip != nil {
		t.Errorf("absent key: %v", skip)
	}
	if skip := load(`{"archive_skip":[".DS_Store","*.tmp"]}`).Skip(); strings.Join(skip, ",") != ".DS_Store,*.tmp" {
		t.Errorf("list: %v", skip)
	}
	if skip := load(`{"archive_skip":[]}`).Skip(); skip == nil || len(skip) != 0 {
		t.Errorf("empty list: %v", skip)
	}
}
