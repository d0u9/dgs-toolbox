package cred

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/cred/record"
	"dgs-toolbox/internal/cred/seal"

	"filippo.io/age"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestChangeRecipients(t *testing.T) {
	f := newOpenFixture(t, "0")
	nas, _ := age.GenerateX25519Identity()
	vps, _ := age.GenerateX25519Identity()
	gone, _ := age.GenerateX25519Identity()
	folder := filepath.Join(filepath.Dir(f.path), "recipients")
	writeFile(t, filepath.Join(folder, "hosts", "laptop.json"), `{"keys":[{"public_key":"`+f.mine.Recipient().String()+`","description":"Main"}]}`, 0o644)
	writeFile(t, filepath.Join(folder, "hosts", "nas.json"), `{"keys":[{"public_key":"`+nas.Recipient().String()+`","description":"NAS"}]}`, 0o644)
	writeFile(t, filepath.Join(folder, "hosts", "vps.json"), `{"keys":[{"public_key":"`+vps.Recipient().String()+`","description":"VPS"}]}`, 0o644)
	writeFile(t, f.path, `{"identities":["`+filepath.Join(filepath.Dir(f.path), "keys")+`"],"recipients":"`+folder+`","vault":"`+f.vaultDir+`","close_after":"0"}`, 0o600)

	source := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, source, "the secret", 0o600)
	path := filepath.Join(f.vaultDir, "secret.txt.age")
	if _, err := seal.Seal(seal.Request{Source: source, Destination: path, Recipients: []seal.Recipient{
		{PublicKey: f.mine.Recipient().String(), Host: "laptop", Description: "Main"},
		{PublicKey: nas.Recipient().String(), Host: "nas", Description: "NAS"},
		{PublicKey: gone.Recipient().String(), Host: "old-server", Description: "retired"},
	}}); err != nil {
		t.Fatal(err)
	}

	m := newVaultModel()
	m.path = f.path
	m = runVault(t, m)
	m = keys(t, m, "e")
	if m.edit == nil || m.edit.stage != editRecipients {
		t.Fatalf("edit flow %+v notice %q", m.edit, m.notice)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"[x] laptop", "[x] nas", "[ ] vps", "[x] old-server · retired", "not in the recipient folder"} {
		if !strings.Contains(view, want) {
			t.Errorf("picker lacks %q:\n%s", want, view)
		}
	}

	// Unchanged is refused.
	m = keys(t, m, "enter")
	if m.edit.stage != editRecipients || !strings.Contains(m.edit.err, "No change") {
		t.Fatalf("unchanged: stage %d err %q", m.edit.stage, m.edit.err)
	}

	// Rows: laptop, Main, nas, NAS, vps, VPS, old-server. Uncheck nas, check
	// vps, uncheck the retired key.
	m = keys(t, m, "j", "j", " ", "j", "j", " ", "j", "j", " ")
	m = keys(t, m, "enter")
	if m.edit.stage != editConfirm {
		t.Fatalf("stage %d err %q", m.edit.stage, m.edit.err)
	}
	dialog := strings.Join(strings.Fields(ansi.Strip(m.edit.dialog.View(300))), " ")
	for _, want := range []string{"add vps · VPS", "remove nas · NAS,", "old-server · retired?", "can still open any copy of the old"} {
		if !strings.Contains(dialog, want) {
			t.Errorf("dialog lacks %q: %s", want, dialog)
		}
	}
	m = keys(t, m, "tab", "enter")
	if m.edit != nil {
		t.Fatalf("edit still open: %q", m.edit.err)
	}
	if !strings.Contains(m.notice, "Re-encrypted secret.txt.age for 2 keys (verified)") {
		t.Errorf("notice %q", m.notice)
	}
	for identity, want := range map[*age.X25519Identity]bool{vps: true, nas: false, gone: false} {
		file, _ := os.Open(path)
		_, err := age.Decrypt(file, identity)
		file.Close()
		if (err == nil) != want {
			t.Errorf("identity %s opens: %v, want %v", identity.Recipient(), err == nil, want)
		}
	}
	rec, _ := record.Read(record.PathFor(path))
	if len(rec.Recipients) != 2 || rec.Updated == nil {
		t.Errorf("record %+v", rec)
	}
}

func TestChangeRecipientsWithoutRecord(t *testing.T) {
	f := newOpenFixture(t, "0")
	encryptTo(t, filepath.Join(f.vaultDir, "bare.age"), []byte("x"), f.mine.Recipient())
	m := newVaultModel()
	m.path = f.path
	m = runVault(t, m)
	m = keys(t, m, "e")
	if m.edit == nil || m.edit.recorded != nil {
		t.Fatalf("edit %+v", m.edit)
	}
	if !strings.Contains(ansi.Strip(m.View()), "No recipient record") {
		t.Error("missing record not said")
	}
	m = keys(t, m, "esc")
	if m.edit != nil {
		t.Error("esc did not cancel")
	}
	_ = tea.KeyMsg{}
}
