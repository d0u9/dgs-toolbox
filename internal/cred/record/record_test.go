package record

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateAndRead(t *testing.T) {
	path := PathFor(filepath.Join(t.TempDir(), "keys.tar.gz.age"))
	if !strings.HasSuffix(path, "keys.tar.gz.age.json") {
		t.Fatalf("path %s", path)
	}
	created := time.Date(2026, 9, 15, 14, 0, 0, 0, time.FixedZone("AEST", 10*3600))
	want := Record{Created: created, Archive: ArchiveTarGz, Recipients: []Recipient{{PublicKey: "age1abc", Host: "nas", Description: "Main"}}}
	if err := Create(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != Version || !got.Created.Equal(created) || got.Archive != ArchiveTarGz || len(got.Recipients) != 1 || got.Recipients[0] != want.Recipients[0] {
		t.Errorf("read %+v", got)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"public_key": "age1abc"`) {
		t.Errorf("file:\n%s", data)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("temporary file left behind: %v", entries)
	}

	if err := Create(path, want); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second create: %v", err)
	}
}

func TestReadRefuses(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"newer":    `{"version":2,"recipients":[]}`,
		"unknown":  `{"version":1,"recipients":[],"extra":true}`,
		"trailing": `{"version":1,"recipients":[]} {}`,
		"broken":   `{`,
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(path); err == nil {
			t.Errorf("%s: read", name)
		}
	}
}

func TestUsing(t *testing.T) {
	vault := t.TempDir()
	create := func(path string, keys ...string) {
		full := filepath.Join(vault, path)
		os.MkdirAll(filepath.Dir(full), 0o755)
		var recipients []Recipient
		for _, key := range keys {
			recipients = append(recipients, Recipient{PublicKey: key})
		}
		if err := Create(PathFor(full), Record{Recipients: recipients}); err != nil {
			t.Fatal(err)
		}
	}
	create("only.age", "age1me")
	create("servers/shared.age", "age1me", "age1nas")
	create("other.age", "age1nas")
	create(".git/hidden.age", "age1me")
	os.WriteFile(filepath.Join(vault, "broken.age.json"), []byte("{"), 0o644)

	usage, err := Using(vault, "age1me")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(usage.Files, ",") != "only.age,servers/shared.age" || strings.Join(usage.Only, ",") != "only.age" || strings.Join(usage.Unreadable, ",") != "broken.age" {
		t.Errorf("usage %+v", usage)
	}
}

func TestUsingAny(t *testing.T) {
	vault := t.TempDir()
	for name, keys := range map[string][]string{"both.age": {"age1a", "age1b"}, "mixed.age": {"age1a", "age1c"}, "none.age": {"age1c"}} {
		var recipients []Recipient
		for _, key := range keys {
			recipients = append(recipients, Recipient{PublicKey: key})
		}
		if err := Create(PathFor(filepath.Join(vault, name)), Record{Recipients: recipients}); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := UsingAny(vault, []string{"age1a", "age1b"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(usage.Files, ",") != "both.age,mixed.age" || strings.Join(usage.Only, ",") != "both.age" {
		t.Errorf("usage %+v", usage)
	}
}
