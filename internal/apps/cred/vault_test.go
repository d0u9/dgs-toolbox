package cred

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/cred/record"

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
	if _, report, ok := m.selectedFile(); !ok || !strings.HasSuffix(report.OpenedBy, "age.txt:1") {
		t.Errorf("top.age opened by %q", report.OpenedBy)
	}
	for _, want := range []string{"✓ decryptable", "Opens", "X25519 ×1"} {
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

// deliver runs a command and hands its message back, repeatedly, the way the
// Bubble Tea runtime would, skipping batches and blinks it cannot resolve.
func deliver(t *testing.T, m vaultModel, cmd tea.Cmd) vaultModel {
	t.Helper()
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if _, ok := msg.(tea.BatchMsg); ok {
			for _, inner := range msg.(tea.BatchMsg) {
				m = deliver(t, m, inner)
			}
			return m
		}
		if msg == nil {
			return m
		}
		if strings.Contains(fmt.Sprintf("%T", msg), "blink") {
			return m
		}
		var updated tea.Model
		updated, cmd = m.Update(msg)
		m = updated.(vaultModel)
	}
	return m
}

func send(t *testing.T, m vaultModel, msg tea.KeyMsg) vaultModel {
	t.Helper()
	updated, cmd := m.Update(msg)
	return deliver(t, updated.(vaultModel), cmd)
}

func typeText(t *testing.T, m vaultModel, text string) vaultModel {
	return send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
}

func TestAddFlow(t *testing.T) {
	root := t.TempDir()
	mine, _ := age.GenerateX25519Identity()
	other, _ := age.GenerateX25519Identity()
	writeFile(t, filepath.Join(root, "keys", "age.txt"), mine.String()+"\n", 0o600)
	folder := filepath.Join(root, "recipients")
	writeFile(t, filepath.Join(folder, "hosts", "laptop.json"), `{"keys":[{"public_key":"`+mine.Recipient().String()+`","description":"Main"}]}`, 0o644)
	writeFile(t, filepath.Join(folder, "hosts", "nas.json"), `{"keys":[{"public_key":"`+other.Recipient().String()+`","description":"NAS key"}]}`, 0o644)
	writeFile(t, filepath.Join(folder, "groups", "g-all.json"), `{"hosts":["laptop","nas"]}`, 0o644)
	vaultDir := filepath.Join(root, "vault")
	if err := os.MkdirAll(filepath.Join(vaultDir, "servers"), 0o755); err != nil {
		t.Fatal(err)
	}
	ageFile(t, filepath.Join(vaultDir, "servers", "old.age"), mine.Recipient())
	source := filepath.Join(root, "src", "nas-keys")
	writeFile(t, filepath.Join(source, "id_ed25519"), "secret key", 0o600)
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"identities":["`+filepath.Join(root, "keys")+`"],"recipients":"`+folder+`","vault":"`+vaultDir+`"}`, 0o600)

	m := newVaultModel()
	m.path = path
	m = runVault(t, m)

	// The cursor is on servers/, so the file goes there.
	m = typeText(t, m, "a")
	if m.add == nil || m.add.directory != filepath.Join(vaultDir, "servers") {
		t.Fatalf("add flow: %+v", m.add)
	}
	m = typeText(t, m, "/")
	m = typeText(t, m, source)
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.add.stage != addRecipients || !m.add.folder {
		t.Fatalf("stage %d, source %q folder %v, err %q", m.add.stage, m.add.source, m.add.folder, m.add.err)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"[-] g-all", "[x] laptop", "[x] Main", "● this machine", "[ ] nas"} {
		if !strings.Contains(view, want) {
			t.Errorf("recipients lack %q:\n%s", want, view)
		}
	}

	// Check the group: every key under it.
	m = typeText(t, m, " ")
	if len(m.chosenRecipients()) != 2 {
		t.Fatalf("chosen %+v", m.chosenRecipients())
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.add.stage != addName || m.add.name.Value() != "nas-keys.tar.gz.age" {
		t.Fatalf("name stage %d %q err %q", m.add.stage, m.add.name.Value(), m.add.err)
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.add.stage != addConfirm {
		t.Fatalf("confirm stage %d err %q", m.add.stage, m.add.err)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "ADD TO VAULT") || !strings.Contains(view, "laptop · Main") {
		t.Errorf("confirm view:\n%s", view)
	}
	// Tab to Encrypt, Enter, and deliver the seal and the rescan.
	m = send(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.add != nil {
		t.Fatalf("flow still open: stage %d err %q", m.add.stage, m.add.err)
	}
	if !strings.Contains(m.notice, "Added servers/nas-keys.tar.gz.age (verified)") || !strings.Contains(m.notice, "plaintext is still at") {
		t.Errorf("notice %q", m.notice)
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "servers", "nas-keys.tar.gz.age.json")); err != nil {
		t.Error("record not written")
	}
	if text := stripVault(m); !strings.Contains(text, "✓ nas-keys.tar.gz.age") {
		t.Errorf("rescan does not show the new file:\n%s", text)
	}

	// Adding the same again is refused at the name.
	m = typeText(t, m, "a")
	m = typeText(t, m, "/")
	m = typeText(t, m, source)
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.add == nil || m.add.stage != addName || !strings.Contains(m.add.err, "already exists") {
		t.Fatalf("duplicate: %+v", m.add)
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.add != nil {
		t.Errorf("esc did not walk out of the flow: stage %d", m.add.stage)
	}
}

func TestAddFlowPassphrase(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "recipients")
	writeFile(t, filepath.Join(folder, "hosts", "laptop.json"), `{"keys":[]}`, 0o644)
	vaultDir := filepath.Join(root, "vault")
	if err := os.MkdirAll(vaultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "src", "notes.txt")
	writeFile(t, source, "secret notes", 0o600)
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"recipients":"`+folder+`","vault":"`+vaultDir+`"}`, 0o600)

	m := newVaultModel()
	m.path = path
	m = runVault(t, m)
	m = typeText(t, m, "a")
	m = typeText(t, m, "/")
	m = typeText(t, m, source)
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.add == nil || m.add.stage != addRecipients {
		t.Fatalf("add flow: %+v", m.add)
	}
	m = typeText(t, m, "p")
	if m.add.stage != addPassphrase {
		t.Fatalf("stage %d", m.add.stage)
	}
	m = typeText(t, m, "correct horse")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = typeText(t, m, "wrong")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.add.stage != addPassphrase || !strings.Contains(m.add.err, "do not match") {
		t.Fatalf("mismatch accepted: stage %d err %q", m.add.stage, m.add.err)
	}
	if view := ansi.Strip(m.View()); strings.Contains(view, "correct horse") {
		t.Error("the passphrase is shown")
	}
	m = typeText(t, m, "correct horse")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.add.stage != addName || m.addEncryption() != "with a passphrase" {
		t.Fatalf("name stage %d err %q", m.add.stage, m.add.err)
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.add != nil {
		t.Fatalf("flow still open: stage %d err %q", m.add.stage, m.add.err)
	}
	if !strings.Contains(m.notice, "Added notes.txt.age (verified)") {
		t.Errorf("notice %q", m.notice)
	}
	identity, _ := age.NewScryptIdentity("correct horse")
	file, err := os.Open(filepath.Join(vaultDir, "notes.txt.age"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := age.Decrypt(file, identity); err != nil {
		t.Error(err)
	}
}

func TestVaultMove(t *testing.T) {
	root := t.TempDir()
	mine, _ := age.GenerateX25519Identity()
	writeFile(t, filepath.Join(root, "keys", "age.txt"), mine.String()+"\n", 0o600)
	vaultDir := filepath.Join(root, "vault")
	ageFile(t, filepath.Join(vaultDir, "top.age"), mine.Recipient())
	if err := record.Create(filepath.Join(vaultDir, "top.age.json"), record.Record{Recipients: []record.Recipient{{PublicKey: mine.Recipient().String()}}}); err != nil {
		t.Fatal(err)
	}
	ageFile(t, filepath.Join(vaultDir, "servers", "nas.age"), mine.Recipient())
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"identities":["`+filepath.Join(root, "keys")+`"],"vault":"`+vaultDir+`"}`, 0o600)

	m := newVaultModel()
	m.path = path
	m = runVault(t, m)
	// The cursor starts on servers/, so move down to servers/nas.age.
	m = vaultKeys(t, m, "j")
	if file, _, ok := m.selectedFile(); !ok || file.Path != "servers/nas.age" {
		t.Fatalf("selected %+v", file)
	}
	m = vaultKeys(t, m, "r")
	if m.move == nil || m.move.from != "servers/nas.age" {
		t.Fatalf("r did not open the move form: %+v", m.move)
	}
	if got := m.move.form.Value(vaultNameFieldID); got != "nas.age" {
		t.Errorf("name %q folder %q", got, m.move.form.Value(vaultFolderFieldID))
	}
	// A name the vault would not list is refused.
	m = vaultMoveType(t, m, "nas.txt")
	m = vaultKeys(t, m, "enter", "enter", "enter")
	if m.move.stage != keyForm || !strings.Contains(m.move.err, "must end in .age") {
		t.Fatalf("suffix: stage %d err %q", m.move.stage, m.move.err)
	}
	// Rename it and move it to a folder that does not exist yet.
	m = vaultKeys(t, m, "up", "up", "enter")
	m = vaultMoveType(t, m, "nas-01.age")
	m = vaultKeys(t, m, "enter")
	m = vaultMoveType(t, m, "servers/eu")
	m = vaultKeys(t, m, "enter", "enter")
	if m.move.stage != keyConfirm {
		t.Fatalf("stage %d err %q", m.move.stage, m.move.err)
	}
	if view := stripVault(m); !strings.Contains(view, "servers/eu/nas-01.age") || !strings.Contains(view, "is created") {
		t.Errorf("confirm:\n%s", view)
	}
	m = vaultKeys(t, m, "tab", "enter")
	m = deliver(t, m, m.runMove())
	if m.move != nil {
		t.Fatalf("flow open: %+v", m.move)
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "servers", "eu", "nas-01.age")); err != nil {
		t.Fatalf("moved file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "servers", "nas.age")); err == nil {
		t.Error("source still there")
	}
	if !strings.Contains(m.notice, "servers/eu/nas-01.age") {
		t.Errorf("notice %q", m.notice)
	}

	// Renaming carries the record along.
	m = vaultKeys(t, m, "G")
	if file, _, ok := m.selectedFile(); !ok || file.Path != "top.age" {
		t.Fatalf("selected %+v", file)
	}
	m = vaultKeys(t, m, "r")
	m = vaultMoveType(t, m, "keep.age")
	m = vaultKeys(t, m, "enter", "enter", "enter")
	m = vaultKeys(t, m, "tab", "enter")
	m = deliver(t, m, m.runMove())
	for _, name := range []string{"keep.age", "keep.age.json"} {
		if _, err := os.Stat(filepath.Join(vaultDir, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "top.age.json")); err == nil {
		t.Error("record left behind")
	}
}

// vaultMoveType clears the field being edited and types text into it.
func vaultMoveType(t *testing.T, m vaultModel, text string) vaultModel {
	t.Helper()
	for i := 0; i < 64; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = updated.(vaultModel)
	}
	for _, r := range text {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(vaultModel)
	}
	return m
}
