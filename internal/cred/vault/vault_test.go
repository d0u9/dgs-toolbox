package vault

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"filippo.io/age/armor"
	"golang.org/x/crypto/ssh"
)

func encrypt(t *testing.T, armored bool, recipients ...age.Recipient) []byte {
	t.Helper()
	var out bytes.Buffer
	var dst io.Writer = &out
	var armorWriter io.WriteCloser
	if armored {
		armorWriter = armor.NewWriter(&out)
		dst = armorWriter
	}
	w, err := age.Encrypt(dst, recipients...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("secret")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if armorWriter != nil {
		if err := armorWriter.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return out.Bytes()
}

type sshKey struct {
	identity  age.Identity
	recipient age.Recipient
	tag       Tag
}

func newSSHKey(t *testing.T) sshKey {
	t.Helper()
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := agessh.NewEd25519Identity(private)
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := agessh.NewEd25519Recipient(sshPublic)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := TagOf(string(ssh.MarshalAuthorizedKey(sshPublic)))
	if !ok {
		t.Fatal("no tag")
	}
	return sshKey{identity: identity, recipient: recipient, tag: tag}
}

func inspect(data []byte, ring Keyring) Report {
	return InspectReader(bytes.NewReader(data), ring)
}

func TestInspectStatuses(t *testing.T) {
	mine, _ := age.GenerateX25519Identity()
	other, _ := age.GenerateX25519Identity()
	protected := newSSHKey(t)
	host := newSSHKey(t)
	scrypt, err := age.NewScryptRecipient("pass")
	if err != nil {
		t.Fatal(err)
	}
	scrypt.SetWorkFactor(10)
	ring := Keyring{
		Openers:   []Opener{{Name: "other", Identity: other}, {Name: "mine", Identity: mine}},
		Protected: []Tagged{{Name: "~/.ssh/id_locked", Tag: protected.tag}},
		Hosts:     []Tagged{{Name: "nas · host key", Tag: host.tag}},
	}

	report := inspect(encrypt(t, false, mine.Recipient(), host.recipient), ring)
	if report.Status != Decryptable || report.OpenedBy != "mine" || report.Armored {
		t.Errorf("decryptable: %+v", report)
	}
	if len(report.Stanzas) != 2 || report.Stanzas[0].Type != "X25519" || !report.Stanzas[1].HasTag {
		t.Errorf("stanzas: %+v", report.Stanzas)
	}
	if len(report.Hosts) != 1 || report.Hosts[0] != "nas · host key" {
		t.Errorf("hosts: %+v", report.Hosts)
	}

	if report := inspect(encrypt(t, true, mine.Recipient()), ring); report.Status != Decryptable || !report.Armored {
		t.Errorf("armored: %+v", report)
	}
	if report := inspect(encrypt(t, false, protected.recipient), ring); report.Status != NeedsPassphrase || len(report.Protected) != 1 {
		t.Errorf("needs passphrase: %+v", report)
	}
	if report := inspect(encrypt(t, false, scrypt), ring); report.Status != Passphrase {
		t.Errorf("passphrase: %+v", report)
	}
	stranger, _ := age.GenerateX25519Identity()
	if report := inspect(encrypt(t, false, stranger.Recipient()), ring); report.Status != NotForThisMachine {
		t.Errorf("not for this machine: %+v", report)
	}
	if report := inspect(encrypt(t, false, mine.Recipient()), Keyring{}); report.Status != NotForThisMachine {
		t.Errorf("no identities: %+v", report)
	}
}

func TestInspectDamaged(t *testing.T) {
	mine, _ := age.GenerateX25519Identity()
	ring := Keyring{Openers: []Opener{{Name: "mine", Identity: mine}}}
	good := encrypt(t, false, mine.Recipient())

	// Flip a character of the MAC: the identity unwraps, the header fails.
	end := bytes.Index(good, []byte("\n--- ")) + 5
	tampered := append([]byte(nil), good...)
	if tampered[end] == 'A' {
		tampered[end] = 'B'
	} else {
		tampered[end] = 'A'
	}

	for name, data := range map[string][]byte{
		"not age":   []byte("hello world, this is not an age file at all"),
		"empty":     nil,
		"truncated": good[:30],
		"bad mac":   tampered,
	} {
		if report := inspect(data, ring); report.Status != Damaged || report.Message == "" {
			t.Errorf("%s: %+v", name, report)
		}
	}
}

func TestInspectUnsupported(t *testing.T) {
	header := "age-encryption.org/v1\n-> piv-p256 abcd\nAAAA\n--- AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n"
	report := inspect([]byte(header+"payload"), Keyring{})
	if report.Status != Unsupported || len(report.Stanzas) != 1 || report.Stanzas[0].Type != "piv-p256" {
		t.Errorf("%+v", report)
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()
	write := func(path string) {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.age")
	write("servers/nas.age")
	write("servers/deep/vps.age")
	write("README.md")
	write("notes.age.txt")
	write(".git/objects/x.age")
	write("servers/.hidden.age")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "linked.age"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "linked.age"), filepath.Join(root, "linked-file.age")); err != nil {
		t.Fatal(err)
	}

	listing, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, file := range listing.Files {
		paths = append(paths, file.Path)
	}
	if got := strings.Join(paths, ","); got != "a.age,linked-file.age,servers/deep/vps.age,servers/nas.age" {
		t.Errorf("paths %s", got)
	}
	if _, err := List(filepath.Join(root, "absent")); err == nil {
		t.Error("listed a missing folder")
	}
}
