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
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"

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

func TestReseal(t *testing.T) {
	dir := t.TempDir()
	mine, _ := age.GenerateX25519Identity()
	removed, _ := age.GenerateX25519Identity()
	added, _ := age.GenerateX25519Identity()
	source := filepath.Join(dir, "secret")
	write(t, source, "the secret", 0o600)
	path := filepath.Join(dir, "vault", "secret.age")
	os.MkdirAll(filepath.Dir(path), 0o755)
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if _, err := Seal(Request{Source: source, Destination: path, Recipients: []Recipient{{PublicKey: mine.Recipient().String()}, {PublicKey: removed.Recipient().String(), Host: "old"}}, Now: created}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)

	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	result, err := Reseal(ResealRequest{
		Path:       path,
		Open:       []age.Identity{mine},
		Recipients: []Recipient{{PublicKey: mine.Recipient().String(), Host: "laptop"}, {PublicKey: added.Recipient().String(), Host: "new"}},
		Verify:     []age.Identity{mine},
		Now:        now,
	})
	if err != nil || !result.Verified {
		t.Fatalf("%+v %v", result, err)
	}
	after, _ := os.ReadFile(path)
	if bytes.Equal(before, after) {
		t.Error("file unchanged")
	}
	if got := decrypt(t, path, added); string(got) != "the secret" {
		t.Errorf("added recipient reads %q", got)
	}
	if file, _ := os.Open(path); file != nil {
		if _, err := age.Decrypt(file, removed); err == nil {
			t.Error("removed recipient still opens the new file")
		}
		file.Close()
	}
	rec, _ := record.Read(result.RecordPath)
	if !rec.Created.Equal(created) || rec.Updated == nil || !rec.Updated.Equal(now) || len(rec.Recipients) != 2 || rec.Recipients[1].Host != "new" {
		t.Errorf("record %+v", rec)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 2 {
		t.Errorf("left behind: %v", entries)
	}
}

func TestResealKeepsArmorAndRefuses(t *testing.T) {
	dir := t.TempDir()
	mine, _ := age.GenerateX25519Identity()
	stranger, _ := age.GenerateX25519Identity()
	path := filepath.Join(dir, "a.age")
	var out bytes.Buffer
	aw := armor.NewWriter(&out)
	w, _ := age.Encrypt(aw, mine.Recipient())
	w.Write([]byte("armored"))
	w.Close()
	aw.Close()
	write(t, path, out.String(), 0o640)

	recipients := []Recipient{{PublicKey: mine.Recipient().String()}}
	if _, err := Reseal(ResealRequest{Path: path, Open: []age.Identity{mine}, Recipients: recipients, Verify: []age.Identity{mine}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !bytes.HasPrefix(data, []byte(armorIntro)) {
		t.Error("armor not kept")
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Errorf("mode %04o", info.Mode().Perm())
	}
	if rec, err := record.Read(record.PathFor(path)); err != nil || rec.Updated == nil {
		t.Errorf("record created for an unrecorded file: %+v %v", rec, err)
	}

	before, _ := os.ReadFile(path)
	if _, err := Reseal(ResealRequest{Path: path, Open: []age.Identity{stranger}, Recipients: recipients}); err == nil || !strings.Contains(err.Error(), "does not decrypt") {
		t.Errorf("wrong identity: %v", err)
	}
	if _, err := Reseal(ResealRequest{Path: path, Open: []age.Identity{mine}, Recipients: []Recipient{{PublicKey: stranger.Recipient().String()}}, Verify: []age.Identity{mine}}); err == nil {
		t.Error("replaced with a file this machine cannot check")
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Error("a refused reseal changed the file")
	}
}

func TestSealPassphrase(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "notes.txt")
	write(t, source, "secret notes", 0o600)
	destination := filepath.Join(dir, "notes.txt.age")
	result, err := Seal(Request{Source: source, Destination: destination, Passphrase: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified {
		t.Error("a passphrase file is checked with the passphrase")
	}
	identity, _ := age.NewScryptIdentity("correct horse")
	if got := decrypt(t, destination, identity); string(got) != "secret notes" {
		t.Errorf("decrypted %q", got)
	}
	rec, err := record.Read(record.PathFor(destination))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Encryption != record.EncryptionPassphrase || len(rec.Recipients) != 0 {
		t.Errorf("record %+v", rec)
	}

	mine, _ := age.GenerateX25519Identity()
	_, err = Seal(Request{Source: source, Destination: filepath.Join(dir, "both.age"), Passphrase: "p",
		Recipients: []Recipient{{PublicKey: mine.Recipient().String()}}})
	if err == nil {
		t.Error("a passphrase with recipients was accepted")
	}
}

func TestResealPassphrase(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "notes.txt")
	write(t, source, "secret notes", 0o600)
	path := filepath.Join(dir, "notes.txt.age")
	if _, err := Seal(Request{Source: source, Destination: path, Passphrase: "old"}); err != nil {
		t.Fatal(err)
	}
	wrong, _ := age.NewScryptIdentity("wrong")
	if _, err := Reseal(ResealRequest{Path: path, Open: []age.Identity{wrong}, Passphrase: "new"}); err == nil {
		t.Fatal("the wrong passphrase re-encrypted the file")
	}
	old, _ := age.NewScryptIdentity("old")
	result, err := Reseal(ResealRequest{Path: path, Open: []age.Identity{old}, Passphrase: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified {
		t.Error("not checked with the new passphrase")
	}
	file, _ := os.Open(path)
	defer file.Close()
	if _, err := age.Decrypt(file, old); err == nil {
		t.Error("the old passphrase still opens the file")
	}
	fresh, _ := age.NewScryptIdentity("new")
	if got := decrypt(t, path, fresh); string(got) != "secret notes" {
		t.Errorf("decrypted %q", got)
	}
	rec, err := record.Read(record.PathFor(path))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Encryption != record.EncryptionPassphrase || rec.Updated == nil || len(rec.Recipients) != 0 {
		t.Errorf("record %+v", rec)
	}
}

// TestArchiveSkipsJunk pins that the default patterns leave out what an
// operating system and git write, keep the dotfiles a folder is added for,
// and that an empty list archives everything.
func TestArchiveSkipsJunk(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "nas-keys")
	write(t, filepath.Join(folder, "id_ed25519"), "key", 0o600)
	write(t, filepath.Join(folder, ".env"), "TOKEN=1", 0o600)
	write(t, filepath.Join(folder, ".DS_Store"), "junk", 0o644)
	write(t, filepath.Join(folder, "._id_ed25519"), "junk", 0o644)
	write(t, filepath.Join(folder, "thumbs.db"), "junk", 0o644)
	write(t, filepath.Join(folder, "desktop.ini"), "junk", 0o644)
	write(t, filepath.Join(folder, ".git", "config"), "junk", 0o644)
	write(t, filepath.Join(folder, "sub", ".DS_Store"), "junk", 0o644)

	names, err := Count(folder, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 6 {
		t.Errorf("counted %v", names)
	}

	data, skipped, err := Archive(folder, DefaultMaxSize, nil)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 6 {
		t.Errorf("skipped %d", skipped)
	}
	entries := archiveNames(t, data)
	for _, name := range []string{"nas-keys/id_ed25519", "nas-keys/.env", "nas-keys/sub/"} {
		if !entries[name] {
			t.Errorf("archive lacks %s: %v", name, entries)
		}
	}
	for _, name := range []string{"nas-keys/.DS_Store", "nas-keys/._id_ed25519", "nas-keys/thumbs.db", "nas-keys/desktop.ini", "nas-keys/.git/", "nas-keys/.git/config", "nas-keys/sub/.DS_Store"} {
		if entries[name] {
			t.Errorf("archive holds %s", name)
		}
	}

	data, skipped, err = Archive(folder, DefaultMaxSize, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 {
		t.Errorf("skipped %d with no patterns", skipped)
	}
	if entries = archiveNames(t, data); !entries["nas-keys/.DS_Store"] {
		t.Errorf("empty patterns skipped something: %v", entries)
	}
}

func archiveNames(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatal(err)
		}
		names[header.Name] = true
	}
}
