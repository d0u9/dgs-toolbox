package seal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"dgs-toolbox/internal/cred/record"
)

func write(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func decrypt(t *testing.T, path string, identity age.Identity) []byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	r, err := age.Decrypt(file, identity)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSealFile(t *testing.T) {
	dir := t.TempDir()
	mine, _ := age.GenerateX25519Identity()
	other, _ := age.GenerateX25519Identity()
	source := filepath.Join(dir, "src", "id_ed25519")
	write(t, source, "private key", 0o600)
	vault := filepath.Join(dir, "vault")
	if err := os.Mkdir(vault, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(vault, DestinationName(source, false))

	result, err := Seal(Request{
		Source:      source,
		Destination: destination,
		Recipients: []Recipient{
			{PublicKey: mine.Recipient().String(), Host: "laptop", Description: "Main"},
			{PublicKey: other.Recipient().String(), Host: "nas", Description: "Main"},
		},
		Verify: []age.Identity{mine},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.Archived || filepath.Base(result.Path) != "id_ed25519.age" {
		t.Errorf("result %+v", result)
	}
	for _, identity := range []age.Identity{mine, other} {
		if got := decrypt(t, destination, identity); string(got) != "private key" {
			t.Errorf("decrypted %q", got)
		}
	}
	rec, err := record.Read(result.RecordPath)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Archive != "" || len(rec.Recipients) != 2 || rec.Recipients[1].Host != "nas" {
		t.Errorf("record %+v", rec)
	}
	entries, _ := os.ReadDir(vault)
	if len(entries) != 2 {
		t.Errorf("vault holds %v", entries)
	}
	if data, _ := os.ReadFile(source); string(data) != "private key" {
		t.Error("source changed")
	}

	if _, err := Seal(Request{Source: source, Destination: destination, Recipients: []Recipient{{PublicKey: mine.Recipient().String()}}}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second seal: %v", err)
	}
}

func TestSealFolder(t *testing.T) {
	dir := t.TempDir()
	mine, _ := age.GenerateX25519Identity()
	folder := filepath.Join(dir, "nas-keys")
	write(t, filepath.Join(folder, "id_ed25519"), "key", 0o600)
	write(t, filepath.Join(folder, ".ssh", "config"), "Host nas", 0o644)
	destination := filepath.Join(dir, DestinationName(folder, true))

	result, err := Seal(Request{Source: folder, Destination: destination, Recipients: []Recipient{{PublicKey: mine.Recipient().String()}}, Verify: []age.Identity{mine}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Archived || filepath.Base(destination) != "nas-keys.tar.gz.age" {
		t.Errorf("result %+v", result)
	}
	if rec, _ := record.Read(result.RecordPath); rec.Archive != record.ArchiveTarGz {
		t.Errorf("record archive %q", rec.Archive)
	}

	gz, err := gzip.NewReader(bytes.NewReader(decrypt(t, destination, mine)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]*tar.Header{}
	contents := map[string]string{}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = header
		data, _ := io.ReadAll(tr)
		contents[header.Name] = string(data)
	}
	for _, name := range []string{"nas-keys/", "nas-keys/.ssh/", "nas-keys/.ssh/config", "nas-keys/id_ed25519"} {
		if entries[name] == nil {
			t.Errorf("archive lacks %s: %v", name, entries)
		}
	}
	if header := entries["nas-keys/id_ed25519"]; header == nil || header.Mode != 0o600 || contents["nas-keys/id_ed25519"] != "key" {
		t.Errorf("id_ed25519 entry %+v %q", header, contents["nas-keys/id_ed25519"])
	}
}

func TestSealRefuses(t *testing.T) {
	dir := t.TempDir()
	mine, _ := age.GenerateX25519Identity()
	stranger, _ := age.GenerateX25519Identity()
	recipients := []Recipient{{PublicKey: mine.Recipient().String()}}
	source := filepath.Join(dir, "file")
	write(t, source, strings.Repeat("x", 100), 0o600)

	linked := filepath.Join(dir, "linked")
	write(t, filepath.Join(linked, "a"), "a", 0o600)
	if err := os.Symlink(source, filepath.Join(linked, "link")); err != nil {
		t.Fatal(err)
	}

	for name, test := range map[string]struct {
		request  Request
		fragment string
	}{
		"no recipients": {Request{Source: source, Destination: filepath.Join(dir, "a.age")}, "no recipients"},
		"bad recipient": {Request{Source: source, Destination: filepath.Join(dir, "b.age"), Recipients: []Recipient{{PublicKey: "nope"}}}, "recipient nope"},
		"too large":     {Request{Source: source, Destination: filepath.Join(dir, "c.age"), Recipients: recipients, MaxSize: 10}, "larger than"},
		"symlink":       {Request{Source: linked, Destination: filepath.Join(dir, "d.age"), Recipients: recipients}, "not a regular file"},
		"not mine":      {Request{Source: source, Destination: filepath.Join(dir, "e.age"), Recipients: recipients, Verify: []age.Identity{stranger}}, "does not decrypt"},
	} {
		_, err := Seal(test.request)
		if err == nil || !strings.Contains(err.Error(), test.fragment) {
			t.Errorf("%s: %v", name, err)
		}
		if _, statErr := os.Stat(test.request.Destination); statErr == nil {
			t.Errorf("%s: destination published", name)
		}
	}
	record := filepath.Join(dir, "f.age.json")
	write(t, record, "{}", 0o644)
	if _, err := Seal(Request{Source: source, Destination: filepath.Join(dir, "f.age"), Recipients: recipients}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("existing record: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), PartSuffix) {
			t.Errorf("part file left behind: %s", entry.Name())
		}
	}
}

func TestSealUnverified(t *testing.T) {
	dir := t.TempDir()
	other, _ := age.GenerateX25519Identity()
	source := filepath.Join(dir, "file")
	write(t, source, "x", 0o600)
	result, err := Seal(Request{Source: source, Destination: filepath.Join(dir, "file.age"), Recipients: []Recipient{{PublicKey: other.Recipient().String()}}})
	if err != nil || result.Verified {
		t.Errorf("result %+v, %v", result, err)
	}
}
