package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlock(t *testing.T) {
	h := Host{Alias: "server1", HostName: "203.0.113.10", User: "root", Port: "2222", IdentityFile: "~/.ssh/keys/server1"}
	if err := h.Check(); err != nil {
		t.Fatal(err)
	}
	want := "Host server1\n    HostName 203.0.113.10\n    User root\n    Port 2222\n    IdentityFile ~/.ssh/keys/server1\n    IdentitiesOnly yes\n"
	if got := h.Block(); got != want {
		t.Errorf("block:\n%s", got)
	}
	short := Host{Alias: "web", HostName: "example.com", IdentityFile: "/k"}.Block()
	if strings.Contains(short, "User") || strings.Contains(short, "Port") {
		t.Errorf("empty fields written:\n%s", short)
	}
	for name, bad := range map[string]Host{
		"alias":    {Alias: ".hidden", HostName: "h"},
		"spaces":   {Alias: "a b", HostName: "h"},
		"hostname": {Alias: "a"},
		"quote":    {Alias: "a", HostName: `h"x`},
		"port":     {Alias: "a", HostName: "h", Port: "70000"},
	} {
		if bad.Check() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestIncludes(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".ssh", "config")
	if ok, err := Includes(path, IncludePattern, home); ok || err != nil {
		t.Errorf("missing file: %v %v", ok, err)
	}
	os.MkdirAll(filepath.Dir(path), 0o700)
	for body, want := range map[string]bool{
		"Host a\n  User b\n":                             false,
		"include ~/.ssh/config.d/*\n":                    true,
		"Include=" + home + "/.ssh/config.d/*\n":         true,
		"Include ~/.ssh/other/* \"~/.ssh/config.d/*\"\n": true,
		"# Include ~/.ssh/config.d/*\n":                  false,
	} {
		os.WriteFile(path, []byte(body), 0o600)
		if ok, _ := Includes(path, IncludePattern, home); ok != want {
			t.Errorf("%q: %v", body, ok)
		}
	}
}

func TestWithInclude(t *testing.T) {
	if got := string(WithInclude(nil, IncludePattern)); got != "Include ~/.ssh/config.d/*\n" {
		t.Errorf("empty: %q", got)
	}
	if got := string(WithInclude([]byte("Host a\n"), IncludePattern)); got != "Include ~/.ssh/config.d/*\n\nHost a\n" {
		t.Errorf("prepend: %q", got)
	}
}
