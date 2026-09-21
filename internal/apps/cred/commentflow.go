package cred

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/cred/record"
	"dgs-toolbox/internal/cred/vault"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/pageactions"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// commentFlow changes the plaintext explanation in an existing recipient
// record. It deliberately never opens or rewrites the encrypted file.
type commentFlow struct {
	file       vault.File
	path       string
	record     record.Record
	input      textinput.Model
	dialog     confirm.Model
	confirming bool
	err        string
}

type commentSavedMsg struct{ err error }

func (m *vaultModel) startComment() tea.Cmd {
	file, _, ok := m.selectedFile()
	if !ok {
		return nil
	}
	full := filepath.Join(m.root, filepath.FromSlash(file.Path))
	rec, err := record.Read(record.PathFor(full))
	if err != nil {
		m.notice = "! This file has no usable recipient record to hold a comment."
		return nil
	}
	input := textinput.New()
	input.Prompt = ""
	input.SetValue(rec.Comment)
	input.Focus()
	m.comment = &commentFlow{file: file, path: full, record: rec, input: input}
	return textinput.Blink
}

func (m vaultModel) updateComment(msg tea.Msg) (tea.Model, tea.Cmd) {
	flow := m.comment
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if flow.confirming {
		dialog, decision := flow.dialog.Update(key.String())
		flow.dialog = dialog
		switch decision {
		case confirm.Cancelled:
			flow.confirming = false
		case confirm.Confirmed:
			comment := strings.TrimSpace(flow.input.Value())
			rec, recordPath := flow.record, record.PathFor(flow.path)
			return m, func() tea.Msg {
				current, err := record.Read(recordPath)
				if err != nil {
					return commentSavedMsg{err: err}
				}
				if current.Comment != rec.Comment {
					return commentSavedMsg{err: fmt.Errorf("comment changed while editing; reopen it to see the latest value")}
				}
				current.Comment = comment
				return commentSavedMsg{err: record.Replace(recordPath, current)}
			}
		}
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.comment = nil
		return m, nil
	case "enter":
		if strings.TrimSpace(flow.input.Value()) == flow.record.Comment {
			m.comment = nil
			return m, nil
		}
		flow.dialog = confirm.New(confirm.Config{
			Title:        "EDIT COMMENT",
			Message:      fmt.Sprintf("Update the comment for %s?", path.Base(flow.file.Path)),
			Detail:       "Only its plaintext recipient record changes; the encrypted file is not opened or re-encrypted.",
			ConfirmLabel: "Save",
			CancelLabel:  "Back",
		})
		flow.confirming = true
		return m, nil
	}
	var cmd tea.Cmd
	flow.input, cmd = flow.input.Update(key)
	return m, cmd
}

func (m vaultModel) finishComment(msg commentSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.comment.confirming = false
		m.comment.err = "Could not save comment: " + msg.err.Error()
		return m, nil
	}
	name := path.Base(m.comment.file.Path)
	m.comment = nil
	m.notice = "Saved comment for " + name
	return m, nil
}

func (m vaultModel) commentView() string {
	flow := m.comment
	if flow.confirming {
		return flow.dialog.ViewSize(min(88, m.width-4), m.height)
	}
	w, h := pickerSize(m.width, m.height)
	inner := max(1, w-4)
	lines := []string{
		titleStyle.Render("EDIT COMMENT · " + strings.ToUpper(path.Base(flow.file.Path))),
		"",
		mutedStyle.Render("Plaintext explanation of what this encrypted file is for."),
		"",
		"› " + flow.input.View(),
	}
	if flow.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+flow.err), inner)...)
	}
	return modalBox(lines, pageactions.Footer(inner, "esc Cancel", pageactions.Inline("Continue", true)), w, h)
}
