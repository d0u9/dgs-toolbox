package cred

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/record"
	"dgs-toolbox/internal/cred/seal"
	"dgs-toolbox/internal/cred/vault"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/pageactions"

	"filippo.io/age"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type editStage int

const (
	editPassphrase editStage = iota
	editRecipients
	editConfirm
	editRunning
)

// editFlow is changing the recipients of a file already in the vault.
type editFlow struct {
	stage editStage
	file  vault.File
	path  string
	// identity is the protected identity the file needs, and unlocked what its
	// passphrase opened; both nil for a file this machine opens as it is.
	identity *identities.Identity
	unlocked age.Identity
	input    textinput.Model
	// recorded is the file's record, nil when it has none; before are the keys
	// it lists.
	recorded *record.Record
	before   map[string]bool
	choose   *recipientPicker
	dialog   confirm.Model
	err      string
}

type unlockedMsg struct {
	identity age.Identity
	err      error
}

type resealedMsg struct {
	result seal.Result
	count  int
	err    error
}

// startEdit begins changing the recipients of the file under the cursor.
func (m *vaultModel) startEdit() tea.Cmd {
	file, report, ok := m.selectedFile()
	if !ok {
		return nil
	}
	flow := &editFlow{file: file, path: filepath.Join(m.root, filepath.FromSlash(file.Path))}
	switch report.Status {
	case vault.Decryptable:
		flow.stage = editRecipients
	case vault.NeedsPassphrase:
		for i, identity := range m.snap.scan.Identities {
			if identity.Status == identities.Protected && len(report.Protected) > 0 && identityLabelFull(identity) == report.Protected[0] {
				flow.identity = &m.snap.scan.Identities[i]
			}
		}
		if flow.identity == nil {
			m.notice = "! The protected identity for this file is no longer found"
			return nil
		}
		flow.stage = editPassphrase
		flow.input = textinput.New()
		flow.input.Prompt = ""
		flow.input.EchoMode = textinput.EchoPassword
		flow.input.EchoCharacter = '•'
		flow.input.Focus()
	case vault.Passphrase:
		m.notice = "! A file encrypted with a passphrase has no recipients to change"
		return nil
	case vault.Unchecked:
		m.notice = "Still checking this file"
		return nil
	default:
		m.notice = "! Recipients can only be changed for a file this machine opens; this is " + report.Status.String()
		return nil
	}

	var initial map[string]bool
	var outside []record.Recipient
	if rec, err := record.Read(record.PathFor(flow.path)); err == nil {
		flow.recorded = &rec
		flow.before = map[string]bool{}
		initial = map[string]bool{}
		for _, r := range rec.Recipients {
			flow.before[r.PublicKey] = true
			initial[r.PublicKey] = true
			if !m.folderLists(r.PublicKey) {
				outside = append(outside, r)
			}
		}
	}
	flow.choose = newRecipientPicker(m.snap.folder, m.snap.held, initial, outside)
	m.edit = flow
	if flow.stage == editPassphrase {
		return textinput.Blink
	}
	return nil
}

func (m vaultModel) folderLists(publicKey string) bool {
	for _, host := range m.snap.folder.Hosts {
		for _, key := range host.Keys {
			if key.Key == publicKey {
				return true
			}
		}
	}
	return false
}

func (m vaultModel) updateEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	flow := m.edit
	if m.open != nil {
		m.open.lastInput = time.Now()
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch flow.stage {
	case editPassphrase:
		switch key.String() {
		case "esc":
			m.edit = nil
		case "enter":
			passphrase := []byte(flow.input.Value())
			flow.input.Reset()
			identity := *flow.identity
			flow.err = ""
			return m, func() tea.Msg {
				opened, err := identities.Unlock(identity, passphrase)
				clear(passphrase)
				return unlockedMsg{identity: opened, err: err}
			}
		default:
			var cmd tea.Cmd
			flow.input, cmd = flow.input.Update(key)
			return m, cmd
		}
	case editRecipients:
		switch key.String() {
		case "esc":
			m.edit = nil
		case "enter":
			return m.confirmEdit()
		default:
			flow.choose.update(key.String())
		}
	case editConfirm:
		dialog, decision := flow.dialog.Update(key.String())
		flow.dialog = dialog
		switch decision {
		case confirm.Cancelled:
			flow.stage = editRecipients
		case confirm.Confirmed:
			flow.stage = editRunning
			return m, m.runEdit()
		}
	}
	return m, nil
}

func (m vaultModel) finishUnlock(msg unlockedMsg) (tea.Model, tea.Cmd) {
	flow := m.edit
	switch {
	case errors.Is(msg.err, identities.ErrWrongPassphrase):
		flow.err = "Wrong passphrase. Try again."
	case msg.err != nil:
		flow.err = msg.err.Error()
	default:
		flow.unlocked, flow.err = msg.identity, ""
		flow.stage = editRecipients
	}
	return m, nil
}

func describe(r seal.Recipient) string {
	name := r.Host
	if r.Description != "" {
		name += " · " + r.Description
	}
	if name == "" {
		name = shortenText(r.PublicKey)
	}
	return name
}

// confirmEdit checks the new recipients and asks before re-encrypting.
func (m vaultModel) confirmEdit() (tea.Model, tea.Cmd) {
	flow := m.edit
	chosen := flow.choose.chosen()
	switch {
	case m.snap.folder.Errors():
		flow.err = "The recipient folder has errors; fix them in dgs cred keys first."
		return m, nil
	case len(chosen) == 0:
		flow.err = "Check at least one key."
		return m, nil
	}
	after := map[string]bool{}
	var added []string
	for _, r := range chosen {
		after[r.PublicKey] = true
		if flow.before != nil && !flow.before[r.PublicKey] {
			added = append(added, describe(r))
		}
	}
	var removed []string
	if flow.recorded != nil {
		for _, r := range flow.recorded.Recipients {
			if !after[r.PublicKey] {
				removed = append(removed, describe(seal.Recipient{PublicKey: r.PublicKey, Host: r.Host, Description: r.Description}))
			}
		}
		if len(added) == 0 && len(removed) == 0 {
			flow.err = "No change: these are the keys it is encrypted to."
			return m, nil
		}
	}
	flow.err = ""

	var message string
	if flow.recorded == nil {
		names := make([]string, len(chosen))
		for i, r := range chosen {
			names[i] = describe(r)
		}
		message = fmt.Sprintf("Re-encrypt %s for %s? It has no record, so what it was encrypted to before is not known.", path.Base(flow.file.Path), strings.Join(names, ", "))
	} else {
		var changes []string
		if len(added) > 0 {
			changes = append(changes, "add "+strings.Join(added, ", "))
		}
		if len(removed) > 0 {
			changes = append(changes, "remove "+strings.Join(removed, ", "))
		}
		message = fmt.Sprintf("Re-encrypt %s to %s?", path.Base(flow.file.Path), strings.Join(changes, "; "))
	}
	var notes []string
	if !flow.choose.mine() {
		notes = append(notes, "No checked key belongs to this machine: the file cannot be opened or checked here afterwards.")
	}
	if len(removed) > 0 || flow.recorded == nil {
		notes = append(notes, "A removed key can still open any copy of the old version — its own, git history, a backup. If it holds a secret, change the secret too.")
	}
	flow.dialog = confirm.New(confirm.Config{Title: "CHANGE RECIPIENTS", Message: message, Detail: strings.Join(notes, " "), ConfirmLabel: "Re-encrypt", CancelLabel: "Back"})
	flow.stage = editConfirm
	return m, nil
}

func (m vaultModel) runEdit() tea.Cmd {
	flow := m.edit
	request := seal.ResealRequest{Path: flow.path, Recipients: flow.choose.chosen()}
	unlocked, snap := flow.unlocked, m.snap
	return func() tea.Msg {
		var usable []age.Identity
		for _, identity := range snap.scan.Identities {
			if identity.Status == identities.Usable {
				if opened, err := identities.Open(identity); err == nil {
					usable = append(usable, opened)
				}
			}
		}
		if unlocked != nil {
			usable = append(usable, unlocked)
		}
		request.Open, request.Verify = usable, usable
		result, err := seal.Reseal(request)
		return resealedMsg{result: result, count: len(request.Recipients), err: err}
	}
}

func (m vaultModel) finishEdit(msg resealedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.edit.stage = editRecipients
		m.edit.err = msg.err.Error()
		return m, nil
	}
	name := path.Base(m.edit.file.Path)
	m.edit = nil
	state := "verified"
	if !msg.result.Verified {
		state = "unverified"
	}
	m.notice = fmt.Sprintf("Re-encrypted %s for %s (%s)", name, plural(msg.count, "key"), state)
	scan := m.scan(m.root)
	return m, scan
}

func (m vaultModel) editView() string {
	flow := m.edit
	if flow.stage == editConfirm {
		return flow.dialog.View(min(96, m.width-4))
	}
	w, h := pickerSize(m.width, m.height)
	inner := max(1, w-4)
	lines := []string{titleStyle.Render("CHANGE RECIPIENTS · " + strings.ToUpper(path.Base(flow.file.Path)))}
	var footer string
	switch flow.stage {
	case editPassphrase:
		lines = append(lines, "")
		lines = append(lines, wrapped(fmt.Sprintf("%s is encrypted to %s, which is protected. Its passphrase unlocks it to re-encrypt this file.", path.Base(flow.file.Path), identityLabelFull(*flow.identity)), inner)...)
		lines = append(lines, "", "› "+flow.input.View())
		footer = pageactions.Footer(inner, "↵ Unlock · esc Cancel", pageactions.Inline("Unlock", true))
	default:
		note := "Checked as its record lists."
		if flow.recorded == nil {
			note = "No recipient record: what it was encrypted to is not known. This machine's keys are checked."
		}
		lines = append(lines, mutedStyle.Render(note), "")
		lines = append(lines, flow.choose.view(inner, h-9))
		footer = pageactions.Footer(inner, fmt.Sprintf("space Check · esc Cancel · %s checked", plural(len(flow.choose.chosen()), "key")), pageactions.Inline("Continue", true))
		if flow.stage == editRunning {
			lines = append(lines, "", titleStyle.Render("Re-encrypting and checking…"))
		}
	}
	if flow.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+flow.err), inner)...)
	}
	return modalBox(lines, footer, w, h)
}
