package cred

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/cred/seal"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// openServerFolder seals a folder holding an SSH key and a config file, and
// opens it on the vault page with the key selected. HOME is a temporary
// directory, so nothing touches the real ~/.ssh.
func openServerFolder(t *testing.T) (vaultModel, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	f := newOpenFixture(t, "0")
	folder := filepath.Join(t.TempDir(), "server1")
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKey(private, "")
	writeFile(t, filepath.Join(folder, "server1"), string(pem.EncodeToMemory(block)), 0o600)
	writeFile(t, filepath.Join(folder, "notes.txt"), "Host server1\n", 0o644)
	if _, err := seal.Seal(seal.Request{Source: folder, Destination: filepath.Join(f.vaultDir, "server1.tar.gz.age"), Recipients: []seal.Recipient{{PublicKey: f.mine.Recipient().String()}}}); err != nil {
		t.Fatal(err)
	}
	m := newVaultModel()
	m.path = f.path
	m = runVault(t, m)
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.open == nil {
		t.Fatalf("not opened: %q", m.notice)
	}
	// Entries: server1/, server1/notes.txt, server1/server1.
	m = vaultKeys(t, m, "j", "j")
	if entry, _ := m.selectedEntry(); entry.Path != "server1/server1" {
		t.Fatalf("selected %q", entry.Path)
	}
	return m, home
}

func key(name string) tea.KeyMsg {
	switch name {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

func keys(t *testing.T, m vaultModel, names ...string) vaultModel {
	t.Helper()
	for _, name := range names {
		m = send(t, m, key(name))
	}
	return m
}

func paste(t *testing.T, m vaultModel, text string) vaultModel {
	t.Helper()
	return send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
}

func TestInstallSSHKeyFromVault(t *testing.T) {
	m, home := openServerFolder(t)
	m = keys(t, m, "enter")
	if m.action == nil || m.action.stage != actionMenu || len(m.action.choices) != 3 {
		t.Fatalf("menu %+v", m.action)
	}
	m = keys(t, m, "enter")
	if m.action.stage != actionForm || m.action.form.FocusedID() != fieldHostName || !m.action.form.IsActive() {
		t.Fatalf("form did not open editing the host name: %+v", m.action.form.FocusedID())
	}
	// Continuing without a host name is refused.
	m = keys(t, m, "enter", "enter", "enter", "down", "enter")
	if m.action.stage != actionForm || !strings.Contains(m.action.err, "host name is required") {
		t.Fatalf("empty host name: stage %d err %q", m.action.stage, m.action.err)
	}
	m.action.form.SetFocusID(fieldHostName)
	m = keys(t, m, "enter")
	m = paste(t, m, "203.0.113.10")
	m = keys(t, m, "enter")
	for _, r := range "root" {
		m = keys(t, m, string(r))
	}
	m = keys(t, m, "enter", "enter", "down", "enter")
	if m.action.stage != actionConfirm {
		t.Fatalf("stage %d err %q", m.action.stage, m.action.err)
	}
	if !strings.Contains(m.action.dialog.View(200), "Include") {
		t.Error("confirmation does not mention the Include line")
	}
	m = keys(t, m, "tab", "enter")
	if m.action != nil {
		t.Fatalf("action open: %q", m.action.err)
	}
	if !strings.Contains(m.notice, "ssh server1") {
		t.Errorf("notice %q", m.notice)
	}
	conf, err := os.ReadFile(filepath.Join(home, ".ssh", "config.d", "server1.conf"))
	if err != nil || !strings.Contains(string(conf), "HostName 203.0.113.10") || !strings.Contains(string(conf), "User root") {
		t.Errorf("conf %q %v", conf, err)
	}
	if config, _ := os.ReadFile(filepath.Join(home, ".ssh", "config")); !strings.HasPrefix(string(config), "Include ~/.ssh/config.d/*") {
		t.Errorf("config %q", config)
	}
	if info, err := os.Stat(filepath.Join(home, ".ssh", "keys", "server1")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("key: %v", err)
	}
}

func TestAddSSHKeyToAgentFromVault(t *testing.T) {
	m, _ := openServerFolder(t)
	dir, _ := os.MkdirTemp("/tmp", "agent")
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	keyring := agent.NewKeyring()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(keyring, conn)
		}
	}()
	t.Setenv("SSH_AUTH_SOCK", socket)

	m = keys(t, m, "enter", "j", "enter")
	if m.action.action.ID != "ssh.agent" {
		t.Fatalf("chose %s", m.action.action.ID)
	}
	m = keys(t, m, "enter", "down", "enter")
	if m.action.stage != actionConfirm {
		t.Fatalf("stage %d err %q", m.action.stage, m.action.err)
	}
	m = keys(t, m, "tab", "enter")
	listed, _ := keyring.List()
	if m.action != nil || len(listed) != 1 || listed[0].Comment != "server1.tar.gz.age/server1/server1" {
		t.Errorf("agent keys %+v, action %+v, notice %q", listed, m.action, m.notice)
	}
}

func TestSaveTextFromVault(t *testing.T) {
	m, home := openServerFolder(t)
	m = vaultKeys(t, m, "k")
	m = keys(t, m, "enter")
	if len(m.action.choices) != 2 {
		t.Fatalf("text choices %+v", m.action.choices)
	}
	m = keys(t, m, "j", "enter")
	if m.action.action.ID != "file.save" {
		t.Fatalf("chose %s", m.action.action.ID)
	}
	// Folder starts at ~ and is not a text field, so down moves past it.
	m = keys(t, m, "down", "down", "enter")
	if m.action.stage != actionConfirm {
		t.Fatalf("stage %d err %q", m.action.stage, m.action.err)
	}
	m = keys(t, m, "tab", "enter")
	if data, err := os.ReadFile(filepath.Join(home, "notes.txt")); err != nil || string(data) != "Host server1\n" {
		t.Errorf("saved %q %v, notice %q", data, err, m.notice)
	}
}

func TestInstallSSHKeyIntoSSHDirWithoutHostEntry(t *testing.T) {
	m, home := openServerFolder(t)
	m = keys(t, m, "enter", "enter")
	// Leave the host name edit, choose ~/.ssh, and turn the Host entry off.
	m = keys(t, m, "esc")
	m.action.form.SetFocusID(fieldLocation)
	m = keys(t, m, "enter", "down", "enter")
	if got := m.action.form.Value(fieldLocation); got != "~/.ssh" {
		t.Fatalf("location %q", got)
	}
	m = keys(t, m, "down")
	if m.action.form.FocusedID() != fieldHostOn {
		t.Fatalf("focus %s; folder should be hidden", m.action.form.FocusedID())
	}
	m = keys(t, m, "enter")
	for _, id := range m.visibleIDs() {
		if id == fieldHostName || id == fieldInclude {
			t.Errorf("%s still shown without a Host entry", id)
		}
	}
	m = keys(t, m, "down", "enter")
	if m.action.stage != actionConfirm {
		t.Fatalf("stage %d err %q", m.action.stage, m.action.err)
	}
	m = keys(t, m, "tab", "enter")
	if _, err := os.Stat(filepath.Join(home, ".ssh", "server1")); err != nil {
		t.Errorf("key not in ~/.ssh: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "server1.pub")); err != nil {
		t.Error("no .pub")
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "config.d")); err == nil {
		t.Error("config.d written without a Host entry")
	}
	if strings.Contains(m.notice, "ssh server1") {
		t.Errorf("notice suggests ssh alias: %q", m.notice)
	}
}

func TestInstallSSHKeyElsewhere(t *testing.T) {
	m, home := openServerFolder(t)
	m = keys(t, m, "enter", "enter", "esc")
	m.action.form.SetFocusID(fieldLocation)
	m = keys(t, m, "enter", "down", "down", "enter", "down")
	if m.action.form.FocusedID() != fieldFolder {
		t.Fatalf("Other did not show the folder: focus %s", m.action.form.FocusedID())
	}
	m.action.form.SetValue(fieldFolder, "~/Secrets/ssh")
	m.action.form.SetFocusID(fieldHostName)
	m = keys(t, m, "enter")
	m = paste(t, m, "example.com")
	m = keys(t, m, "enter", "enter", "enter", "down", "enter", "tab", "enter")
	if _, err := os.Stat(filepath.Join(home, "Secrets", "ssh", "server1")); err != nil {
		t.Errorf("key not in the chosen folder: %v; notice %q action %+v", err, m.notice, m.action)
	}
	conf, _ := os.ReadFile(filepath.Join(home, ".ssh", "config.d", "server1.conf"))
	if !strings.Contains(string(conf), "IdentityFile ~/Secrets/ssh/server1") {
		t.Errorf("conf %q", conf)
	}
}
