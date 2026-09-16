package cred

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/cred/seal"

	"filippo.io/age"
	"filippo.io/age/agessh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/crypto/ssh"
)

type openFixture struct {
	path     string
	vaultDir string
	mine     *age.X25519Identity
}

func newOpenFixture(t *testing.T, closeAfter string) openFixture {
	t.Helper()
	root := t.TempDir()
	mine, _ := age.GenerateX25519Identity()
	writeFile(t, filepath.Join(root, "keys", "age.txt"), mine.String()+"\n", 0o600)
	vaultDir := filepath.Join(root, "vault")
	os.MkdirAll(vaultDir, 0o755)
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"identities":["`+filepath.Join(root, "keys")+`"],"vault":"`+vaultDir+`","close_after":"`+closeAfter+`"}`, 0o600)
	return openFixture{path: path, vaultDir: vaultDir, mine: mine}
}

func encryptTo(t *testing.T, path string, plaintext []byte, recipient age.Recipient) {
	t.Helper()
	var out bytes.Buffer
	w, err := age.Encrypt(&out, recipient)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(plaintext)
	w.Close()
	writeFile(t, path, out.String(), 0o644)
}

func selectFile(t *testing.T, m vaultModel, name string) vaultModel {
	t.Helper()
	for i := 0; i < 20; i++ {
		if file, _, ok := m.selectedFile(); ok && filepath.Base(file.Path) == name {
			return m
		}
		m = vaultKeys(t, m, "j")
	}
	t.Fatalf("no file %s", name)
	return m
}

func TestOpenArchiveAndMask(t *testing.T) {
	f := newOpenFixture(t, "0")
	folder := filepath.Join(t.TempDir(), "nas-keys")
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKey(private, "")
	writeFile(t, filepath.Join(folder, "id_ed25519"), string(pem.EncodeToMemory(block)), 0o600)
	writeFile(t, filepath.Join(folder, "notes.txt"), "line one\n\x1b[31mred\x1b[0m\n", 0o644)
	if _, err := seal.Seal(seal.Request{Source: folder, Destination: filepath.Join(f.vaultDir, "nas-keys.tar.gz.age"), Recipients: []seal.Recipient{{PublicKey: f.mine.Recipient().String(), Host: "laptop", Description: "Main"}}}); err != nil {
		t.Fatal(err)
	}

	m := newVaultModel()
	m.path = f.path
	m = runVault(t, m)
	m = selectFile(t, m, "nas-keys.tar.gz.age")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.open == nil {
		t.Fatalf("not opened: notice %q", m.notice)
	}
	if got := strings.Join(m.entryPaths(), ","); got != "nas-keys,nas-keys/id_ed25519,nas-keys/notes.txt" {
		t.Errorf("entries %s", got)
	}
	if path := m.CommandPath(); len(path) != 1 || path[0] != "nas-keys.tar.gz.age" {
		t.Errorf("breadcrumb %v", path)
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{"CONTENTS", "For laptop · Main", "▾ nas-keys.tar.gz/", "▾ nas-keys/", "⚿ id_ed25519", "● nas-keys.tar.gz.age"} {
		if !strings.Contains(screen, want) {
			t.Errorf("open view lacks %q:\n%s", want, screen)
		}
	}

	// The private key is masked until v. The first row is the whole archive,
	// the second the folder inside it.
	m = vaultKeys(t, m, "j", "j")
	screen = ansi.Strip(m.View())
	if !strings.Contains(screen, "Content hidden") || strings.Contains(screen, "BEGIN OPENSSH") || !strings.Contains(screen, "ssh-ed25519") {
		t.Errorf("masked preview:\n%s", screen)
	}
	m = vaultKeys(t, m, "v")
	if !strings.Contains(ansi.Strip(m.View()), "BEGIN OPENSSH PRIVATE KEY") {
		t.Error("v did not show the key")
	}

	// Text is shown with its escape sequences made harmless.
	m = vaultKeys(t, m, "j")
	raw := m.View()
	if strings.Contains(raw, "\x1b[31mred") || !strings.Contains(ansi.Strip(raw), "·[31mred") {
		t.Errorf("escape sequence reached the view")
	}

	// c copies text, and refuses the private key.
	var copied []string
	m.copy = func(text string) error {
		copied = append(copied, text)
		return nil
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if len(copied) != 1 || !strings.HasPrefix(copied[0], "line one\n") || !strings.Contains(m.notice, "Copied notes.txt") {
		t.Errorf("copy text: %q notice %q", copied, m.notice)
	}
	m = vaultKeys(t, m, "k")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if len(copied) != 1 || !strings.Contains(m.notice, "Private keys are not copied") {
		t.Errorf("copy key: %q notice %q", copied, m.notice)
	}
	m = vaultKeys(t, m, "j")

	// Esc closes and zeroes the plaintext.
	data := m.open.opened.Entries[1].Data
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.open != nil || !bytes.Equal(data, make([]byte, len(data))) {
		t.Errorf("esc did not close and clear")
	}
}

func TestOpenPassphraseFile(t *testing.T) {
	f := newOpenFixture(t, "0")
	scrypt, _ := age.NewScryptRecipient("correct horse")
	scrypt.SetWorkFactor(10)
	encryptTo(t, filepath.Join(f.vaultDir, "secret.txt.age"), []byte("the secret"), scrypt)

	m := newVaultModel()
	m.path = f.path
	m = runVault(t, m)
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.prompt == nil {
		t.Fatalf("no prompt: notice %q", m.notice)
	}
	m = typeText(t, m, "wrong")
	if strings.Contains(ansi.Strip(m.View()), "wrong") {
		t.Error("passphrase shown in clear")
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.prompt == nil || !strings.Contains(m.prompt.err, "Wrong passphrase") {
		t.Fatalf("wrong passphrase: %+v", m.prompt)
	}
	m = typeText(t, m, "correct horse")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.prompt != nil || m.open == nil || string(m.open.opened.Entries[0].Data) != "the secret" {
		t.Fatalf("not opened: prompt %+v", m.prompt)
	}
}

func TestOpenWithProtectedSSHKey(t *testing.T) {
	f := newOpenFixture(t, "0")
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKeyWithPassphrase(private, "", []byte("unlock me"))
	keysDir := filepath.Join(filepath.Dir(f.path), "keys")
	writeFile(t, filepath.Join(keysDir, "id_locked"), string(pem.EncodeToMemory(block)), 0o600)
	signer, _ := ssh.NewSignerFromKey(private)
	recipient, _ := agessh.NewEd25519Recipient(signer.PublicKey())
	encryptTo(t, filepath.Join(f.vaultDir, "server.age"), []byte("root password"), recipient)

	m := newVaultModel()
	m.path = f.path
	m = runVault(t, m)
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.prompt == nil || m.prompt.identity == nil {
		t.Fatalf("no prompt for the protected key: notice %q", m.notice)
	}
	m = typeText(t, m, "nope")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.prompt == nil || !strings.Contains(m.prompt.err, "Wrong passphrase") {
		t.Fatalf("wrong: %+v", m.prompt)
	}
	m = typeText(t, m, "unlock me")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.open == nil || string(m.open.opened.Entries[0].Data) != "root password" {
		t.Fatalf("not opened: %+v", m.prompt)
	}
}

func TestIdleCloseAndShellClose(t *testing.T) {
	f := newOpenFixture(t, "1m")
	encryptTo(t, filepath.Join(f.vaultDir, "a.age"), []byte("aaa"), f.mine.Recipient())

	m := newVaultModel()
	m.path = f.path
	m = runVault(t, m)
	updated, cmd := m.Update(m.startOpenForTest())
	m = updated.(vaultModel)
	if m.open == nil || cmd == nil {
		t.Fatal("not opened, or no idle tick")
	}
	data := m.open.opened.Entries[0].Data
	m.open.lastInput = time.Now().Add(-2 * time.Minute)
	updated, _ = m.Update(idleTickMsg{token: m.open.token})
	m = updated.(vaultModel)
	if m.open != nil || !strings.Contains(m.notice, "after 1m0s without input") || !bytes.Equal(data, []byte{0, 0, 0}) {
		t.Errorf("idle close: open %v notice %q", m.open != nil, m.notice)
	}

	updated, _ = m.Update(m.startOpenForTest())
	m = updated.(vaultModel)
	data = m.open.opened.Entries[0].Data
	m.Close()
	if !bytes.Equal(data, []byte{0, 0, 0}) {
		t.Error("Close did not clear")
	}
}
