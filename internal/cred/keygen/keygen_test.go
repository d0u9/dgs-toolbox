package keygen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"dgs-toolbox/internal/cred/identities"
)

func TestGenerate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "age")
	now := time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC)
	written, err := Generate(dir, "laptop.txt", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(written.PublicKeys) != 1 || !strings.HasPrefix(written.PublicKeys[0], "age1") {
		t.Fatalf("written %+v", written)
	}
	data, err := os.ReadFile(written.Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "# created: 2026-09-15T14:00:00Z\n# public key: "+written.PublicKeys[0]+"\nAGE-SECRET-KEY-1") {
		t.Errorf("content:\n%s", text)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, written.Path: 0o600} {
		if info, _ := os.Stat(path); info.Mode().Perm() != want {
			t.Errorf("%s mode %04o, want %04o", path, info.Mode().Perm(), want)
		}
	}
	scan := identities.Discover([]string{dir}, identities.Options{})
	if len(scan.Identities) != 1 || scan.Identities[0].Public.Key != written.PublicKeys[0] {
		t.Errorf("not discovered: %+v", scan)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}

	if _, err := Generate(dir, "laptop.txt", now); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second generate: %v", err)
	}
	if again, _ := os.ReadFile(written.Path); string(again) != text {
		t.Error("existing identity changed")
	}
}

func TestImport(t *testing.T) {
	base := t.TempDir()
	first, _ := age.GenerateX25519Identity()
	second, _ := age.GenerateX25519Identity()
	source := filepath.Join(base, "keys.txt")
	content := "# mine\n" + first.String() + "\n" + second.String() + "\n"
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "age")
	written, err := Import(source, dir, "imported.txt")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(written.PublicKeys, ",") != first.Recipient().String()+","+second.Recipient().String() {
		t.Errorf("keys %v", written.PublicKeys)
	}
	if data, _ := os.ReadFile(written.Path); string(data) != content {
		t.Errorf("content changed:\n%s", data)
	}
	if info, _ := os.Stat(written.Path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %04o", info.Mode().Perm())
	}
}

func TestImportRefuses(t *testing.T) {
	base := t.TempDir()
	identity, _ := age.GenerateX25519Identity()
	for name, test := range map[string]struct{ body, fragment string }{
		"empty":     {"# nothing\n", "no age identity"},
		"garbage":   {identity.String() + "\nnot a key\n", "line 2"},
		"plugin":    {"AGE-PLUGIN-YUBIKEY-1QQQQ\n", "plugin"},
		"ssh":       {"-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n", "SSH keys belong"},
		"text file": {"hello\n", "no age identity"},
	} {
		source := filepath.Join(base, name)
		if err := os.WriteFile(source, []byte(test.body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Import(source, filepath.Join(base, "age"), name+".txt"); err == nil || !strings.Contains(err.Error(), test.fragment) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, bad := range []string{"", "a/b", ".hidden", ".."} {
		if _, err := Generate(filepath.Join(base, "age"), bad, time.Now()); err == nil {
			t.Errorf("name %q accepted", bad)
		}
	}
}
