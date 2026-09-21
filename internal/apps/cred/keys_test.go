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
	"time"

	"dgs-toolbox/internal/cred/keygen"
	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/cred/record"
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

func keyPress(t *testing.T, m keysModel, keys ...string) keysModel {
	t.Helper()
	for _, key := range keys {
		var msg tea.KeyMsg
		switch key {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
		updated, cmd := m.Update(msg)
		m = updated.(keysModel)
		for i := 0; cmd != nil && i < 5; i++ {
			next := cmd()
			if next == nil {
				break
			}
			updated, cmd = m.Update(next)
			m = updated.(keysModel)
		}
	}
	return m
}

func typeInto(t *testing.T, m keysModel, text string) keysModel {
	t.Helper()
	for _, r := range text {
		m = keyPress(t, m, string(r))
	}
	return m
}

func TestGenerateAndRegister(t *testing.T) {
	root := t.TempDir()
	_, edPrivate, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKey(edPrivate, "")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "ssh", "id_ed25519"), string(pem.EncodeToMemory(block)), 0o600)
	ageDir := filepath.Join(root, "age")
	folder := filepath.Join(root, "recipients")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"identities":["`+filepath.Join(root, "ssh")+`","`+ageDir+`"],"recipients":"`+folder+`","new_identity_dir":"`+ageDir+`"}`, 0o600)

	m := loadedModel(t, path)

	// Generate: the form opens editing Host.
	m = keyPress(t, m, "n")
	if m.keyFlow == nil || !m.keyFlow.form.IsActive() {
		t.Fatalf("n did not open the form editing the host")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("laptop\n"), Paste: true})
	m = updated.(keysModel)
	m = keyPress(t, m, "enter")
	if got := m.keyFlow.form.Value(fileFieldID); got != "laptop"+IdentitySuffix {
		t.Errorf("file default %q", got)
	}
	// Enter goes File → Description → the button. The description starts
	// empty and is required.
	m = keyPress(t, m, "enter")
	if got := m.keyFlow.form.Value(descriptionFieldID); got != "" {
		t.Errorf("description default %q", got)
	}
	m = keyPress(t, m, "enter", "enter")
	if m.keyFlow.stage != keyForm || !strings.Contains(m.keyFlow.err, "Description is required") {
		t.Fatalf("empty description: stage %d err %q", m.keyFlow.stage, m.keyFlow.err)
	}
	m = keyPress(t, m, "up", "enter")
	m = typeInto(t, m, "Main laptop key")
	m = keyPress(t, m, "enter", "enter")
	if m.keyFlow.stage != keyConfirm {
		t.Fatalf("stage %d err %q", m.keyFlow.stage, m.keyFlow.err)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "GENERATE KEY") || !strings.Contains(view, "laptop, a new host") {
		t.Errorf("confirm:\n%s", view)
	}
	m = keyPress(t, m, "tab", "enter")
	if m.keyFlow != nil {
		t.Fatalf("flow open: stage %d err %q", m.keyFlow.stage, m.keyFlow.err)
	}
	if !strings.Contains(m.notice, "registered it under laptop") {
		t.Errorf("notice %q", m.notice)
	}
	if info, err := os.Stat(filepath.Join(ageDir, "laptop"+IdentitySuffix)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("identity file: %v", err)
	}
	if summary := m.summary(); summary != "2 identities · 1 unregistered" {
		t.Errorf("after generate: %q", summary)
	}

	// Register the SSH key, which sorts after age/; the host starts as laptop.
	m = keyPress(t, m, "j")
	if identity, _ := m.selectedIdentity(); identity.Kind != "ssh-ed25519" {
		t.Fatalf("selected %+v", identity)
	}
	m = keyPress(t, m, "r")
	if m.keyFlow == nil || m.keyFlow.form.Value(hostFieldID) != "laptop" {
		t.Fatalf("register flow: %+v", m.keyFlow)
	}
	m = keyPress(t, m, "enter")
	m = typeInto(t, m, "SSH")
	m = keyPress(t, m, "enter", "enter", "tab", "enter")
	if m.keyFlow != nil {
		t.Fatalf("register flow open: err %q", m.keyFlow.err)
	}
	if summary := m.summary(); summary != "2 identities" {
		t.Errorf("after register: %q", summary)
	}
	host, ok := m.snap.folder.Host("laptop")
	today := time.Now().Format("2006-01-02")
	if !ok || len(host.Keys) != 2 {
		t.Fatalf("host %+v", host)
	}
	generated, registered := host.Keys[0].Meta, host.Keys[1].Meta
	if generated.Description != "Main laptop key" || generated.Origin != "generated" || generated.Added != today || !strings.HasSuffix(generated.PrivateKey, "laptop"+IdentitySuffix) {
		t.Errorf("generated meta %+v", generated)
	}
	if registered.Description != "SSH" || registered.Origin != "registered" || !strings.HasSuffix(registered.PrivateKey, "id_ed25519") {
		t.Errorf("registered meta %+v", registered)
	}

	// Generating again starts from the host this machine belongs to, and the
	// same file is refused in the form.
	m = keyPress(t, m, "n")
	if m.keyFlow.form.Value(hostFieldID) != "laptop" {
		t.Errorf("host prefill %q", m.keyFlow.form.Value(hostFieldID))
	}
	m = keyPress(t, m, "enter", "enter")
	m = typeInto(t, m, "again")
	m = keyPress(t, m, "enter", "enter")
	if m.keyFlow == nil || !strings.Contains(m.keyFlow.err, "already exists") {
		t.Fatalf("duplicate: %+v", m.keyFlow)
	}
	m = keyPress(t, m, "esc")
	if m.keyFlow != nil {
		t.Error("esc did not close the form")
	}
}

func TestDeleteAndUnregister(t *testing.T) {
	root := t.TempDir()
	ageDir := filepath.Join(root, "age")
	written, err := keygen.Generate(ageDir, "laptop"+IdentitySuffix, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, edPrivate, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKey(edPrivate, "")
	writeFile(t, filepath.Join(root, "ssh", "id_ed25519"), string(pem.EncodeToMemory(block)), 0o600)
	sshPublic := string(ssh.MarshalAuthorizedKey(mustSigner(t, edPrivate).PublicKey()))

	folder := filepath.Join(root, "recipients")
	os.MkdirAll(folder, 0o755)
	if _, err := recipients.AddKey(folder, "laptop", written.PublicKeys[0], recipients.Meta{Description: "age key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := recipients.AddKey(folder, "laptop", sshPublic, recipients.Meta{Description: "ssh key"}); err != nil {
		t.Fatal(err)
	}
	vaultDir := filepath.Join(root, "vault")
	os.MkdirAll(vaultDir, 0o755)
	if err := record.Create(filepath.Join(vaultDir, "only.age.json"), record.Record{Recipients: []record.Recipient{{PublicKey: written.PublicKeys[0]}}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"identities":["`+ageDir+`","`+filepath.Join(root, "ssh")+`"],"recipients":"`+folder+`","vault":"`+vaultDir+`","new_identity_dir":"`+ageDir+`"}`, 0o600)

	m := loadedModel(t, path)
	trashDir := filepath.Join(root, "trash")
	os.MkdirAll(trashDir, 0o755)
	m.trash = func(p string) (string, error) {
		to := filepath.Join(trashDir, filepath.Base(p))
		return to, os.Rename(p, to)
	}

	// The age key is dgs's own: deleted, and its public key removed.
	m = keyPress(t, m, "d")
	if m.deleteFlow == nil || m.deleteFlow.path == "" {
		t.Fatalf("delete flow %+v notice %q", m.deleteFlow, m.notice)
	}
	dialog := m.usageNote(m.deleteFlow.publicKey, true)
	for _, want := range []string{"1 file in the vault is encrypted only to it", "cannot be opened again: only.age."} {
		if !strings.Contains(dialog, want) {
			t.Errorf("usage note lacks %q: %s", want, dialog)
		}
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "DELETE KEY") {
		t.Errorf("dialog not shown:\n%s", view)
	}
	m = keyPress(t, m, "tab", "enter")
	if _, err := os.Stat(written.Path); err == nil {
		t.Error("identity file still in place")
	}
	if _, err := os.Stat(filepath.Join(trashDir, "laptop"+IdentitySuffix)); err != nil {
		t.Error("identity file not in the trash")
	}
	if host, _ := m.snap.folder.Host("laptop"); len(host.Keys) != 1 || host.Keys[0].Description != "ssh key" {
		t.Errorf("host after delete %+v", host)
	}

	// The SSH key is not dgs's: only unregistered, the file kept.
	m = keyPress(t, m, "d")
	if m.deleteFlow == nil || m.deleteFlow.path != "" || !strings.Contains(ansi.Strip(m.View()), "UNREGISTER KEY") {
		t.Fatalf("unregister flow %+v", m.deleteFlow)
	}
	m = keyPress(t, m, "esc")
	if m.deleteFlow != nil {
		t.Fatal("esc did not cancel")
	}
	m = keyPress(t, m, "d", "tab", "enter")
	if _, err := os.Stat(filepath.Join(root, "ssh", "id_ed25519")); err != nil {
		t.Error("ssh key file was removed")
	}
	if host, _ := m.snap.folder.Host("laptop"); len(host.Keys) != 0 {
		t.Errorf("host after unregister %+v", host)
	}
}

func mustSigner(t *testing.T, key any) ssh.Signer {
	t.Helper()
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func TestAddPublicKey(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "recipients")
	os.MkdirAll(folder, 0o755)
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"recipients":"`+folder+`"}`, 0o600)
	_, edPrivate, _ := ed25519.GenerateKey(rand.Reader)
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(mustSigner(t, edPrivate).PublicKey()))) + " root@vps"

	m := keyPress(t, loadedModel(t, path), "]")
	m = keyPress(t, m, "a")
	if m.keyFlow == nil || m.keyFlow.action != actionAdd {
		t.Fatalf("a did not open the add form: notice %q", m.notice)
	}
	paste := func(text string) {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
		m = updated.(keysModel)
	}
	paste("us-lax-digitalocean-linux-01")
	m = keyPress(t, m, "enter")
	paste(line + "\n")
	m = keyPress(t, m, "enter")
	m = typeInto(t, m, "DigitalOcean droplet")
	m = keyPress(t, m, "enter", "enter")
	if m.keyFlow.stage != keyConfirm {
		t.Fatalf("stage %d err %q", m.keyFlow.stage, m.keyFlow.err)
	}
	m = keyPress(t, m, "tab", "enter")
	if m.keyFlow != nil {
		t.Fatalf("flow open: %q", m.keyFlow.err)
	}
	host, ok := m.snap.folder.Host("us-lax-digitalocean-linux-01")
	if !ok || len(host.Keys) != 1 || host.Keys[0].Type != recipients.TypeED25519 || host.Keys[0].Comment != "root@vps" || host.Keys[0].Origin != "added" || host.Keys[0].Description != "DigitalOcean droplet" {
		t.Errorf("host %+v", host)
	}

	// The same key again is refused in the form, and a bad key is explained.
	m = keyPress(t, m, "a")
	m = keyPress(t, m, "enter")
	paste(line)
	m = keyPress(t, m, "enter")
	m = typeInto(t, m, "x")
	m = keyPress(t, m, "enter", "enter")
	if m.keyFlow == nil || !strings.Contains(m.keyFlow.err, "already listed under us-lax-digitalocean-linux-01") {
		t.Fatalf("duplicate: %+v", m.keyFlow)
	}
}

func TestDeleteHost(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "recipients")
	vaultDir := filepath.Join(root, "vault")
	os.MkdirAll(vaultDir, 0o755)
	nasKey, vpsKey := newAgeRecipient(t), newAgeRecipient(t)
	writeFile(t, filepath.Join(folder, "hosts", "nas.json"), `{"keys":[{"public_key":"`+nasKey+`","description":"a"}]}`, 0o644)
	writeFile(t, filepath.Join(folder, "hosts", "vps.json"), `{"keys":[{"public_key":"`+vpsKey+`","description":"b"}]}`, 0o644)
	writeFile(t, filepath.Join(folder, "groups", "g-all.json"), `{"hosts":["nas","vps"]}`, 0o644)
	if err := record.Create(filepath.Join(vaultDir, "nas-only.age.json"), record.Record{Recipients: []record.Recipient{{PublicKey: nasKey}}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"recipients":"`+folder+`","vault":"`+vaultDir+`"}`, 0o600)

	m := keyPress(t, loadedModel(t, path), "]")
	trashDir := filepath.Join(root, "trash")
	os.MkdirAll(trashDir, 0o755)
	m.trash = func(p string) (string, error) {
		to := filepath.Join(trashDir, filepath.Base(p))
		return to, os.Rename(p, to)
	}
	m = keyPress(t, m, "d")
	if m.deleteFlow == nil || m.deleteFlow.host != "nas" {
		t.Fatalf("delete host flow %+v", m.deleteFlow)
	}
	if note := m.usageNoteAny([]string{nasKey}); !strings.Contains(note, "nas-only.age") {
		t.Errorf("usage %q", note)
	}
	m = keyPress(t, m, "tab", "enter")
	if _, err := os.Stat(filepath.Join(trashDir, "nas.json")); err != nil {
		t.Error("host file not in the trash")
	}
	if _, ok := m.snap.folder.Host("nas"); ok || len(m.snap.folder.Problems) != 0 {
		t.Errorf("after delete: %+v %+v", m.snap.folder.Hosts, m.snap.folder.Problems)
	}
	if len(m.snap.folder.Groups) != 1 || strings.Join(m.snap.folder.Groups[0].Hosts, ",") != "vps" {
		t.Errorf("groups %+v", m.snap.folder.Groups)
	}
}

func newAgeRecipient(t *testing.T) string {
	t.Helper()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return identity.Recipient().String()
}

func TestRenameHost(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "recipients")
	key := newAgeRecipient(t)
	writeFile(t, filepath.Join(folder, "hosts", "nas.json"), `{"keys":[{"public_key":"`+key+`","description":"a"}]}`, 0o644)
	writeFile(t, filepath.Join(folder, "hosts", "vps.json"), `{"keys":[{"public_key":"`+newAgeRecipient(t)+`","description":"b"}]}`, 0o644)
	writeFile(t, filepath.Join(folder, "groups", "g-all.json"), `{"hosts":["NAS","vps"]}`, 0o644)
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"recipients":"`+folder+`"}`, 0o600)

	m := keyPress(t, loadedModel(t, path), "]")
	m = keyPress(t, m, "r")
	if m.renameFlow == nil || m.renameFlow.host != "nas" || !m.renameFlow.form.IsActive() {
		t.Fatalf("r did not open the rename form: %+v", m.renameFlow)
	}
	if got := m.renameFlow.form.Value(nameFieldID); got != "nas" {
		t.Errorf("name starts as %q", got)
	}
	// A name another host already has is refused.
	m = keyPress(t, m, "backspace", "backspace", "backspace")
	m = typeInto(t, m, "vps")
	m = keyPress(t, m, "enter", "enter")
	if m.renameFlow.stage != keyForm || !strings.Contains(m.renameFlow.err, "vps is already a host") {
		t.Fatalf("duplicate: stage %d err %q", m.renameFlow.stage, m.renameFlow.err)
	}
	m = keyPress(t, m, "up", "enter", "backspace", "backspace", "backspace")
	m = typeInto(t, m, "nas-01")
	m = keyPress(t, m, "enter", "enter")
	if m.renameFlow.stage != keyConfirm {
		t.Fatalf("stage %d err %q", m.renameFlow.stage, m.renameFlow.err)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "RENAME HOST") || !strings.Contains(view, "hosts/nas-01.json") || !strings.Contains(view, "g-all") {
		t.Errorf("confirm:\n%s", view)
	}
	m = keyPress(t, m, "tab", "enter")
	if m.renameFlow != nil {
		t.Fatalf("flow open: stage %d err %q", m.renameFlow.stage, m.renameFlow.err)
	}
	if !strings.Contains(m.notice, "Renamed nas to nas-01") || !strings.Contains(m.notice, "g-all.json") {
		t.Errorf("notice %q", m.notice)
	}
	host, ok := m.snap.folder.Host("nas-01")
	if !ok || len(host.Keys) != 1 || host.Keys[0].Key != key {
		t.Fatalf("host %+v", host)
	}
	if _, ok := m.snap.folder.Host("nas"); ok {
		t.Error("old name still loads")
	}
	if len(m.snap.folder.Groups) != 1 || strings.Join(m.snap.folder.Groups[0].Hosts, ",") != "nas-01,vps" {
		t.Errorf("groups %+v", m.snap.folder.Groups)
	}
	// The cursor follows the renamed host, which now sorts first.
	if selected, _ := m.selectedHost(); selected.Name != "nas-01" {
		t.Errorf("selected %+v", selected)
	}
}

func TestHostComment(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "recipients")
	writeFile(t, filepath.Join(folder, "hosts", "nas.json"), `{"keys":[{"public_key":"`+newAgeRecipient(t)+`","description":"a"}]}`, 0o644)
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"recipients":"`+folder+`"}`, 0o600)

	m := keyPress(t, loadedModel(t, path), "]")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "none; m adds one") {
		t.Errorf("hosts view:\n%s", view)
	}
	m = keyPress(t, m, "m")
	if m.hostComment == nil || m.hostComment.host.Name != "nas" {
		t.Fatalf("m did not open the comment: %+v", m.hostComment)
	}
	m = typeInto(t, m, "under the desk")
	m = keyPress(t, m, "enter")
	if !m.hostComment.confirming {
		t.Fatal("no confirmation")
	}
	m = keyPress(t, m, "tab", "enter")
	if m.hostComment != nil {
		t.Fatalf("flow open: %+v", m.hostComment)
	}
	host, _ := m.snap.folder.Host("nas")
	if host.Comment != "under the desk" || len(host.Keys) != 1 {
		t.Fatalf("host %+v", host)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "under the desk") {
		t.Errorf("hosts view:\n%s", view)
	}
}
