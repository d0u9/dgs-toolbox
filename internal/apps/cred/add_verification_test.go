package cred

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/cred/record"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAddOnlyRemoteRecipientIgnoresUnrelatedLocalIdentities(t *testing.T) {
	f := newFixture(t)
	snap := loadSnapshot(f.path)
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	writeFile(t, source, "synthetic content", 0600)
	m := newVaultModel()
	m.root = root
	m.snap = snap
	m.add = &addFlow{source: source, directory: root, name: textinput.New(), choose: newRecipientPicker(snap.folder, snap.held, map[string]bool{f.stranger: true}, nil)}
	m.add.name.SetValue("remote.age")
	msg := m.runSeal()().(sealedMsg)
	if msg.err != nil || msg.result.Verified {
		t.Fatalf("remote-only addition: %+v %v", msg.result, msg.err)
	}
	if _, err := os.Stat(filepath.Join(root, "remote.age")); err != nil {
		t.Fatal(err)
	}
	rec, err := record.Read(filepath.Join(root, "remote.age.json"))
	if err != nil || len(rec.Recipients) != 1 || rec.Recipients[0].PublicKey != f.stranger {
		t.Fatalf("recipients: %+v %v", rec, err)
	}
	// The same selection may create a new file, but cannot replace one.
	before, err := os.ReadFile(msg.result.Path)
	if err != nil {
		t.Fatal(err)
	}
	updated, cmd := m.updateAddName(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(vaultModel)
	if cmd != nil || m.add.stage == addConfirm || !strings.Contains(m.add.err, "verified") {
		t.Fatal("unverifiable replacement reached confirmation")
	}
	after, err := os.ReadFile(msg.result.Path)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatal("refused replacement changed the ciphertext")
	}
	// Resealing cannot remove every identity that could verify its replacement.
	m.snap.folder.Problems = nil
	m.edit = &editFlow{choose: m.add.choose}
	updated, _ = m.confirmEdit()
	m = updated.(vaultModel)
	if !strings.Contains(m.edit.err, "verified") || m.edit.stage == editConfirm {
		t.Fatal("unverifiable recipient change reached confirmation")
	}
}
