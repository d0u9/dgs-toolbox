package cred

import (
	"fmt"
	"strings"

	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/pageactions"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// hostCommentFlow edits the owner's note about a host: whose machine it is and
// where it stands, which its name alone does not say. It changes only the
// comment field of the host file.
type hostCommentFlow struct {
	host       recipients.Host
	input      textinput.Model
	dialog     confirm.Model
	confirming bool
	err        string
}

type hostCommentSavedMsg struct {
	host string
	file string
	err  error
}

func (m *keysModel) startHostComment() {
	m.notice = ""
	if m.snap.settings.Recipients == "" {
		m.notice = "! Set recipients in credentials.json first"
		return
	}
	host, ok := m.selectedHost()
	if !ok {
		return
	}
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 200
	input.SetValue(host.Comment)
	input.Focus()
	input.CursorEnd()
	m.hostComment = &hostCommentFlow{host: host, input: input}
}

func (m keysModel) updateHostComment(msg tea.Msg) (tea.Model, tea.Cmd) {
	flow := m.hostComment
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
			root, host := m.snap.settings.Recipients, flow.host.Name
			comment := strings.TrimSpace(flow.input.Value())
			return m, func() tea.Msg {
				file, err := recipients.SetComment(root, host, comment)
				return hostCommentSavedMsg{host: host, file: file, err: err}
			}
		}
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.hostComment = nil
		return m, nil
	case "enter":
		if strings.TrimSpace(flow.input.Value()) == flow.host.Comment {
			m.hostComment = nil
			return m, nil
		}
		action := "Set"
		if flow.host.Comment != "" {
			action = "Change"
		}
		if strings.TrimSpace(flow.input.Value()) == "" {
			action = "Remove"
		}
		flow.dialog = confirm.New(confirm.Config{
			Title:        "HOST COMMENT",
			Message:      fmt.Sprintf("%s the comment of %s?", action, flow.host.Name),
			Detail:       "Only the comment changes; the host keeps its keys and their fields. The recipient folder is shared, so every machine reading it sees the comment.",
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

func (m keysModel) finishHostComment(msg hostCommentSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.hostComment.confirming = false
		m.hostComment.err = "Could not save the comment: " + msg.err.Error()
		return m, nil
	}
	m.hostComment = nil
	m.notice = "Saved the comment of " + msg.host + " in " + msg.file
	m.selectHost = msg.host
	return m, m.reload()
}

func (m keysModel) hostCommentView() string {
	flow := m.hostComment
	if flow.confirming {
		return flow.dialog.ViewSize(min(88, m.width-4), m.height)
	}
	w, h := pickerSize(m.width, m.height)
	inner := max(1, w-4)
	lines := []string{
		titleStyle.Render("HOST COMMENT · " + strings.ToUpper(flow.host.Name)),
		"",
		mutedStyle.Render("Whose machine it is, where it stands: what the name does not say."),
		"",
		"› " + flow.input.View(),
		"",
		mutedStyle.Render("An empty comment removes it."),
	}
	if flow.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+flow.err), inner)...)
	}
	return modalBox(lines, pageactions.Footer(inner, "esc Cancel", pageactions.Inline("Continue", true)), w, h)
}

func (m keysModel) hostCommentStatus() tui.Status {
	return tui.Status{Left: "HOST COMMENT", Center: m.hostComment.host.Name, Right: "↵ Continue  esc Cancel"}
}
