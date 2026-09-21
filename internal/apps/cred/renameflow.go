package cred

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/pageactions"

	tea "github.com/charmbracelet/bubbletea"
)

const nameFieldID = "name"

// renameFlow is renaming a host, from the form to the result. It lives on the
// keys page as an overlay.
type renameFlow struct {
	// host is the name the host has now.
	host   string
	stage  keyStage
	form   form.Model
	dialog confirm.Model
	err    string
}

type renamedMsg struct {
	host    string
	name    string
	file    string
	changed []string
	err     error
}

func (m *keysModel) startRename() {
	m.notice = ""
	switch {
	case m.snap.settings.Recipients == "":
		m.notice = "! Set recipients in credentials.json first"
		return
	case m.snap.folderErr != nil:
		m.notice = "! " + m.snap.folderErr.Error()
		return
	}
	host, ok := m.selectedHost()
	if !ok {
		return
	}
	flow := &renameFlow{host: host.Name, stage: keyForm}
	flow.form = form.New(
		form.Field{ID: nameFieldID, Kind: form.Text, Label: "Name", Value: host.Name},
		form.Field{ID: continueFieldID, Kind: form.Button, Label: "Rename"},
	)
	// The name is what the flow is for, so the form opens editing it.
	flow.form.HandleInteraction("enter")
	m.renameFlow = flow
}

func (m keysModel) renameFieldIDs() []string { return []string{nameFieldID, continueFieldID} }

func (m keysModel) updateRenameFlow(msg tea.Msg) (tea.Model, tea.Cmd) {
	flow := m.renameFlow
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch flow.stage {
	case keyForm:
		if key.Paste {
			flow.form.Paste(string(key.Runes))
			return m, nil
		}
		return m.updateRenameForm(key.String())
	case keyConfirm:
		dialog, decision := flow.dialog.Update(key.String())
		flow.dialog = dialog
		switch decision {
		case confirm.Cancelled:
			flow.stage = keyForm
		case confirm.Confirmed:
			flow.stage = keyRunning
			return m, m.runRename()
		}
	}
	return m, nil
}

func (m keysModel) updateRenameForm(key string) (tea.Model, tea.Cmd) {
	flow := m.renameFlow
	if flow.form.IsActive() {
		switch key {
		case "enter", "tab", "down", "shift+tab", "up":
			flow.form.HandleInteraction("enter")
			if key == "shift+tab" || key == "up" {
				flow.form.UpdateNavigationWithin(m.renameFieldIDs(), "up")
				if flow.form.FocusedID() != continueFieldID {
					flow.form.HandleInteraction("enter")
				}
				return m, nil
			}
			flow.form.SetFocusID(continueFieldID)
			return m, nil
		}
		flow.form.HandleInteraction(key)
		return m, nil
	}
	if key == "esc" {
		m.renameFlow = nil
		return m, nil
	}
	if flow.form.FocusedID() == continueFieldID && (key == "enter" || key == " ") {
		if err := m.checkRenameForm(); err != nil {
			flow.err = err.Error()
			return m, nil
		}
		flow.err = ""
		flow.dialog = confirm.New(m.renameConfirmConfig())
		flow.stage = keyConfirm
		return m, nil
	}
	if flow.form.HandleInteraction(key) {
		return m, nil
	}
	flow.form.UpdateNavigationWithin(m.renameFieldIDs(), key)
	return m, nil
}

func (m keysModel) renameValue() string {
	return strings.TrimSpace(m.renameFlow.form.Value(nameFieldID))
}

func (m keysModel) checkRenameForm() error {
	name := m.renameValue()
	flow := m.renameFlow
	if name == "" {
		return errors.New("Name is required.")
	}
	if err := recipients.CheckHostName(name); err != nil {
		return err
	}
	if name == flow.host {
		return errors.New("That is already its name.")
	}
	if !strings.EqualFold(name, flow.host) {
		if existing, ok := m.snap.folder.Host(name); ok {
			return fmt.Errorf("%s is already a host; keys are not merged here.", existing.Name)
		}
	}
	return nil
}

// renameGroups are the groups naming the host, which the rename rewrites.
func (m keysModel) renameGroups() []string {
	var groups []string
	for _, group := range m.snap.folder.Groups {
		for _, member := range group.Hosts {
			if strings.EqualFold(member, m.renameFlow.host) {
				groups = append(groups, group.Name)
			}
		}
	}
	return groups
}

func (m keysModel) renameConfirmConfig() confirm.Config {
	flow := m.renameFlow
	name := m.renameValue()
	message := fmt.Sprintf("Rename %s to %s, as %s?", flow.host, name, filepath.ToSlash(filepath.Join(recipients.HostsDir, name+recipients.FileSuffix)))
	notes := []string{"Its public keys are unchanged, so encrypted files can still be opened."}
	if groups := m.renameGroups(); len(groups) > 0 {
		notes = append(notes, "It is renamed in "+strings.Join(groups, ", ")+" too.")
	}
	notes = append(notes, "The old name is left nowhere: a Recipe or note naming it must be changed by hand.")
	return confirm.Config{
		Title:        "RENAME HOST",
		Message:      message,
		Detail:       strings.Join(notes, " "),
		ConfirmLabel: "Rename",
		CancelLabel:  "Back",
	}
}

func (m keysModel) runRename() tea.Cmd {
	root, host, name := m.snap.settings.Recipients, m.renameFlow.host, m.renameValue()
	return func() tea.Msg {
		file, changed, err := recipients.RenameHost(root, host, name)
		return renamedMsg{host: host, name: name, file: file, changed: changed, err: err}
	}
}

func (m keysModel) finishRename(msg renamedMsg) (tea.Model, tea.Cmd) {
	flow := m.renameFlow
	if msg.err != nil {
		flow.stage = keyForm
		flow.err = msg.err.Error()
		return m, nil
	}
	m.renameFlow = nil
	m.notice = fmt.Sprintf("Renamed %s to %s in %s", msg.host, msg.name, filepath.ToSlash(msg.file))
	if len(msg.changed) > 0 {
		m.notice += " · updated " + strings.Join(msg.changed, ", ")
	}
	// The cursor follows the new name, which sorts elsewhere in the list.
	m.selectHost = msg.name
	return m, m.reload()
}

func (m keysModel) renameFlowView() string {
	flow := m.renameFlow
	if flow.stage == keyConfirm {
		return flow.dialog.ViewSize(min(88, m.width-4), m.height)
	}
	w, h := pickerSize(m.width, m.height)
	inner := max(1, w-4)
	lines := []string{
		titleStyle.Render("RENAME HOST"),
		mutedStyle.Render("Its file, and every group naming it, follow the new name"),
		"",
		flow.form.ViewFocusedWidth([]string{nameFieldID}, true, inner),
		"",
	}
	var hosts []string
	for _, host := range m.snap.folder.Hosts {
		if host.Name != flow.host {
			hosts = append(hosts, host.Name)
		}
	}
	if len(hosts) > 0 {
		lines = append(lines, wrapped(mutedStyle.Render("Other hosts: "+strings.Join(hosts, ", ")), inner)...)
	}
	if flow.stage == keyRunning {
		lines = append(lines, "", titleStyle.Render("Renaming…"))
	}
	if flow.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+flow.err), inner)...)
	}
	hints := "↑↓ Field · ↵ Edit · esc Back"
	if flow.form.IsActive() {
		hints = "Type to edit · ↵ Continue · esc Undo"
	}
	button := pageactions.Inline("Rename", flow.form.FocusedID() == continueFieldID)
	return modalBox(lines, pageactions.Footer(inner, hints, button), w, h)
}

func (m keysModel) renameFlowStatus() tui.Status {
	flow := m.renameFlow
	switch {
	case flow.stage == keyForm && flow.form.IsActive():
		return tui.Status{Left: "RENAME HOST", Right: "↵ Apply  esc Cancel"}
	case flow.stage == keyForm:
		return tui.Status{Left: "RENAME HOST", Right: "↑↓ Field  ↵ Edit  esc Back"}
	}
	return tui.Status{Left: "RENAME HOST"}
}
