package cred

import (
	"bytes"
	"filippo.io/age"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListRecipients(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"hosts", "groups"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Generate a synthetic public key; never read the user's configuration.
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	write("hosts/host-a.json", `{"keys":[{"public_key":"`+identity.Recipient().String()+`","description":"Test key"}]}`)
	write("groups/g-team.json", `{"hosts":["host-a"]}`)
	path := filepath.Join(root, "config.json")
	write("config.json", `{"recipients":"`+filepath.ToSlash(root)+`"}`)
	for _, kind := range []string{"hosts", "groups"} {
		var out bytes.Buffer
		if err := listRecipients(&out, path, kind); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "host-a") {
			t.Fatalf("%s: %s", kind, out.String())
		}
		if kind == "groups" && !strings.Contains(out.String(), "g-team") {
			t.Fatal(out.String())
		}
	}
	write("hosts/broken.json", `{`)
	var out bytes.Buffer
	if err := listRecipients(&out, path, "hosts"); err == nil || !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("error: %v", err)
	}
	write("config.json", `{}`)
	if err := listRecipients(&out, path, "hosts"); err == nil {
		t.Fatal("missing recipients accepted")
	}
}
