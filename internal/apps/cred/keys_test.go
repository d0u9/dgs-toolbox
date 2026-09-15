package cred

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/tui"

	"filippo.io/age"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/crypto/ssh"
)

type fixture struct {
	path       string
	registered string
	stranger   string
}

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

// newFixture writes a credentials.json, an identity directory holding one
// registered age key and one unregistered SSH key, and a recipient folder with
// two hosts, a group, and a file left out with an error.
func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	registered, _ := age.GenerateX25519Identity()
	other, _ := age.GenerateX25519Identity()
	_, edPrivate, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKey(edPrivate, "")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "keys", "age.txt"), registered.String()+"\n", 0o600)
	writeFile(t, filepath.Join(root, "keys", "id_ed25519"), string(pem.EncodeToMemory(block)), 0o600)

	folder := filepath.Join(root, "recipients")
	writeFile(t, filepath.Join(folder, "hosts", "nas.json"), `{"keys":[{"public_key":"`+registered.Recipient().String()+`","description":"Main age key"},{"public_key":"`+other.Recipient().String()+`","description":"Spare"}]}`, 0o644)
	writeFile(t, filepath.Join(folder, "hosts", "laptop.json"), `{"keys":[]}`, 0o644)
	writeFile(t, filepath.Join(folder, "hosts", "broken.json"), `{`, 0o644)
	writeFile(t, filepath.Join(folder, "groups", "g-home.json"), `{"hosts":["nas","laptop"]}`, 0o644)

	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"identities":["`+filepath.Join(root, "keys")+`","`+filepath.Join(root, "absent")+`"],"recipients":"`+folder+`"}`, 0o600)
	return fixture{path: path, registered: registered.Recipient().String(), stranger: other.Recipient().String()}
}

func loadedModel(t *testing.T, path string) keysModel {
	t.Helper()
	m := newKeysModel()
	m.path = path
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = updated.(keysModel)
	msg := m.Init()()
	updated, _ = m.Update(msg)
	return updated.(keysModel)
}

func press(t *testing.T, m keysModel, keys ...string) (keysModel, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, key := range keys {
		var msg tea.KeyMsg
		switch key {
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
		var updated tea.Model
		updated, cmd = m.Update(msg)
		m = updated.(keysModel)
	}
	return m, cmd
}

func view(m keysModel) string { return ansi.Strip(m.View()) }

func TestIdentitiesTab(t *testing.T) {
	f := newFixture(t)
	m := loadedModel(t, f.path)
	screen := view(m)
	for _, want := range []string{"IDENTITIES", "age.txt:1", "age · usable · nas", "id_ed25519", "ssh-ed25519 · usable · unregistered", "PROBLEMS", "directory does not exist"} {
		if !strings.Contains(screen, want) {
			t.Errorf("identities view lacks %q:\n%s", want, screen)
		}
	}
	if status := m.Status(); status.Left != "IDENTITIES" || !strings.Contains(status.Center, "2 identities · 1 unregistered") {
		t.Errorf("status %+v", status)
	}
	if !strings.Contains(screen, "Main age key") || !strings.Contains(screen, f.registered) {
		t.Errorf("detail of the first identity missing:\n%s", screen)
	}

	m, _ = press(t, m, "j")
	if screen := view(m); !strings.Contains(screen, "unregistered") || !strings.Contains(screen, "SHA256:") || !strings.Contains(screen, "Add this public key") {
		t.Errorf("unregistered detail:\n%s", screen)
	}
}

func TestHostsTabAndCopy(t *testing.T) {
	f := newFixture(t)
	m := loadedModel(t, f.path)
	var copied []string
	m.copy = func(text string) error {
		copied = append(copied, text)
		return nil
	}

	m, _ = press(t, m, "]")
	if tabs := m.Tabs(); !tabs[1].Active {
		t.Fatalf("tabs %+v", tabs)
	}
	screen := view(m)
	for _, want := range []string{"HOSTS", "! laptop", "nas", "PROBLEMS", "! broken.json"} {
		if !strings.Contains(screen, want) {
			t.Errorf("hosts view lacks %q:\n%s", want, screen)
		}
	}
	if status := m.Status(); !strings.Contains(status.Center, "2 hosts · 1 error · 1 warning") {
		t.Errorf("status %+v", status)
	}

	// laptop is first; move to nas and into its keys.
	m, _ = press(t, m, "j")
	screen = view(m)
	for _, want := range []string{"Groups   g-home", "Main age key  ● this machine", "Spare"} {
		if !strings.Contains(screen, want) {
			t.Errorf("nas view lacks %q:\n%s", want, screen)
		}
	}
	m, _ = press(t, m, "tab", "j")
	if m.fields.Current() != keysField || m.Status().Left != "KEYS" {
		t.Fatalf("focus %s", m.fields.Current())
	}
	m, cmd := press(t, m, "c")
	if cmd == nil {
		t.Fatal("c produced nothing")
	}
	updated, _ := m.Update(cmd())
	m = updated.(keysModel)
	if len(copied) != 1 || copied[0] != f.stranger || !strings.Contains(m.Status().Center, "Copied public key: Spare") {
		t.Errorf("copied %q, status %+v", copied, m.Status())
	}

	m.copy = func(string) error { return errors.New("no terminal") }
	_, cmd = press(t, m, "c")
	updated, _ = m.Update(cmd())
	if status := updated.(keysModel).Status(); !strings.Contains(status.Center, "Copy failed") {
		t.Errorf("failed copy status %+v", status)
	}

	// The problem entry shows the error.
	m, _ = press(t, m, "tab", "j")
	if screen := view(m); !strings.Contains(screen, "PROBLEM") || !strings.Contains(screen, "invalid JSON") {
		t.Errorf("problem detail:\n%s", screen)
	}
}

func TestGroupsTab(t *testing.T) {
	f := newFixture(t)
	m := loadedModel(t, f.path)
	updated, _ := m.Update(tui.TabSelectedMsg{Index: 2})
	m = updated.(keysModel)
	screen := view(m)
	for _, want := range []string{"GROUPS", "g-home", "Hosts (2)", "nas  2 keys", "laptop  0 keys"} {
		if !strings.Contains(screen, want) {
			t.Errorf("groups view lacks %q:\n%s", want, screen)
		}
	}
	if _, cmd := press(t, m, "c"); cmd != nil {
		t.Error("c copies in Groups")
	}
}

func TestMissingCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	m := loadedModel(t, path)
	if screen := view(m); !strings.Contains(screen, "No credentials.json at") {
		t.Errorf("view:\n%s", screen)
	}
	writeFile(t, path, `{"identities":["relative"]}`, 0o600)
	m, cmd := press(t, m, "R")
	updated, _ := m.Update(cmd())
	if screen := view(updated.(keysModel)); !strings.Contains(screen, "not an absolute path") {
		t.Errorf("view after reload:\n%s", screen)
	}
}

func TestNoRecipientFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	writeFile(t, path, `{}`, 0o600)
	m, _ := press(t, loadedModel(t, path), "]")
	if screen := view(m); !strings.Contains(screen, "names no recipient") {
		t.Errorf("view:\n%s", screen)
	}
}

func TestTooSmall(t *testing.T) {
	m := newKeysModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	if !strings.Contains(view(updated.(keysModel)), "Resize terminal") {
		t.Error("no resize prompt")
	}
}
