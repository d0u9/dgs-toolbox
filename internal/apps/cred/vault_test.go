package cred

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func ageFile(t *testing.T, path string, recipients ...age.Recipient) {
	t.Helper()
	var out bytes.Buffer
	w, err := age.Encrypt(&out, recipients...)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("secret"))
	w.Close()
	writeFile(t, path, out.String(), 0o644)
}

// runVault loads the page and delivers every header report.
func runVault(t *testing.T, m vaultModel) vaultModel {
	t.Helper()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = updated.(vaultModel)
	updated, cmd := m.Update(m.Init()())
	m = updated.(vaultModel)
	for cmd != nil {
		updated, cmd = m.Update(cmd())
		m = updated.(vaultModel)
	}
	return m
}

func vaultKeys(t *testing.T, m vaultModel, keys ...string) vaultModel {
	t.Helper()
	for _, key := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		if key == "enter" {
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		}
		updated, _ := m.Update(msg)
		m = updated.(vaultModel)
	}
	return m
}

func TestVaultPage(t *testing.T) {
	root := t.TempDir()
	mine, _ := age.GenerateX25519Identity()
	stranger, _ := age.GenerateX25519Identity()
	writeFile(t, filepath.Join(root, "keys", "age.txt"), mine.String()+"\n", 0o600)
	vaultDir := filepath.Join(root, "vault")
	ageFile(t, filepath.Join(vaultDir, "top.age"), mine.Recipient())
	ageFile(t, filepath.Join(vaultDir, "servers", "nas.age"), stranger.Recipient())
	writeFile(t, filepath.Join(vaultDir, "servers", "broken.age"), "not an age file", 0o644)
	ageFile(t, filepath.Join(vaultDir, ".git", "hidden.age"), mine.Recipient())
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"identities":["`+filepath.Join(root, "keys")+`"],"vault":"`+vaultDir+`"}`, 0o600)

	m := newVaultModel()
	m.path = path
	m = runVault(t, m)
	if m.pending != 0 {
		t.Fatalf("pending %d", m.pending)
	}
	text := stripVault(m)
	for _, want := range []string{"▾ servers/", "! broken.age", "✗ nas.age", "✓ top.age"} {
		if !strings.Contains(text, want) {
			t.Errorf("vault view lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "hidden.age") {
		t.Errorf("hidden directory listed:\n%s", text)
	}
	if status := m.Status(); !strings.Contains(status.Center, "3 files · 1 decryptable") {
		t.Errorf("status %+v", status)
	}

	// servers/ is first: fold it, then the tree hides its files.
	m = vaultKeys(t, m, "enter")
	if text := stripVault(m); strings.Contains(text, "nas.age") || !strings.Contains(text, "▸ servers/") {
		t.Errorf("folded:\n%s", text)
	}
	m = vaultKeys(t, m, "enter", "G")
	text = stripVault(m)
	for _, want := range []string{"✓ decryptable", "Opens", "age.txt:1", "X25519 ×1"} {
		if !strings.Contains(text, want) {
			t.Errorf("top.age detail lacks %q:\n%s", want, text)
		}
	}
	m = vaultKeys(t, m, "k", "k")
	if text := stripVault(m); !strings.Contains(text, "! damaged") || !strings.Contains(text, "not an age file") {
		t.Errorf("broken.age detail:\n%s", text)
	}
}

func TestVaultWithoutFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	writeFile(t, path, `{}`, 0o600)
	m := newVaultModel()
	m.path = path
	m = runVault(t, m)
	if text := stripVault(m); !strings.Contains(text, "No vault folder") {
		t.Errorf("view:\n%s", text)
	}
	m = vaultKeys(t, m, "o")
	if !m.picking || m.Status().Center != "VAULT FOLDER" {
		t.Errorf("o did not open the folder picker")
	}
	if !m.CapturesShellKey("esc") {
		t.Error("esc not kept while picking")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(vaultModel).picking {
		t.Error("esc did not close the picker")
	}
}

func stripVault(m vaultModel) string { return ansi.Strip(m.View()) }
