package cred

import (
	"errors"
	"fmt"
	"strings"

	"dgs-toolbox/internal/gitrepo"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/pageactions"

	tea "github.com/charmbracelet/bubbletea"
)

type updateStage int

const (
	// updateFetching is the remote being asked what it has; updateConfirm
	// names what would arrive, updateRunning brings it in.
	updateFetching updateStage = iota
	updateConfirm
	updateRunning
)

// updateShown is how many incoming commits the confirmation lists before it
// counts the rest.
const updateShown = 6

// updateFlow is `fetch`, then a fast-forward of the branch the vault folder is
// on. Fetching is safe whatever the working tree holds, so it runs first and
// what it finds is what the dialog is about.
//
// Only a fast-forward is offered. The vault is encrypted files, which no one
// can resolve a conflict in, so an update that could leave the working tree
// half-merged is not one this page makes: a branch that has moved on both
// sides is reported and resolved in a terminal.
type updateFlow struct {
	stage  updateStage
	result gitrepo.FetchResult
	dialog confirm.Model
	err    string
}

type gitFetchedMsg struct {
	result gitrepo.FetchResult
	err    error
}

type gitUpdatedMsg struct {
	snap gitrepo.Snapshot
	err  error
}

// startUpdate fetches and, when something is waiting, asks before taking it.
func (m *vaultModel) startUpdate() tea.Cmd {
	if m.root == "" {
		m.notice = "! No vault folder yet."
		return nil
	}
	m.update = &updateFlow{stage: updateFetching}
	root, runner := m.root, m.gitRunner
	return func() tea.Msg {
		result, err := runner.Fetch(root)
		return gitFetchedMsg{result: result, err: err}
	}
}

func (m vaultModel) finishFetch(msg gitFetchedMsg) (tea.Model, tea.Cmd) {
	if m.update == nil {
		return m, nil
	}
	switch {
	case errors.Is(msg.err, gitrepo.ErrNotInstalled):
		m.update = nil
		m.notice = "! git is not installed, so the vault folder cannot be updated from here."
		return m, nil
	case errors.Is(msg.err, gitrepo.ErrNotARepository):
		m.update = nil
		m.notice = "! " + tilde(m.root) + " is not in a git repository."
		return m, nil
	case msg.err != nil:
		m.update = nil
		m.notice = "! Fetch failed: " + msg.err.Error()
		return m, nil
	}
	snap := msg.result.Snapshot
	switch {
	case msg.result.Note != "":
		m.update = nil
		m.notice = "Fetched: " + msg.result.Note
		return m, nil
	case snap.Behind == 0 && snap.Ahead > 0:
		m.update = nil
		m.notice = fmt.Sprintf("Up to date with %s; %s to publish with p", snap.Upstream, plural(snap.Ahead, "commit"))
		return m, nil
	case snap.Behind == 0:
		m.update = nil
		m.notice = "Up to date with " + snap.Upstream
		return m, nil
	case snap.Ahead > 0:
		// Both sides moved. Fetching has already happened, so nothing is lost
		// by stopping here, and nothing this page does can join the two.
		m.update = nil
		m.notice = fmt.Sprintf("! Diverged: %s here, %s on %s. Merge or rebase in a terminal.",
			plural(snap.Ahead, "commit"), plural(snap.Behind, "commit"), snap.Upstream)
		return m, nil
	}
	flow := m.update
	flow.result = msg.result
	flow.stage = updateConfirm
	flow.dialog = confirm.New(confirm.Config{
		Title:        "UPDATE",
		Message:      fmt.Sprintf("Fast-forward %s to %s, %s?", snap.Branch, snap.Upstream, plural(snap.Behind, "commit")),
		Detail:       incomingDetail(msg.result),
		ConfirmLabel: "Update",
		CancelLabel:  "Cancel",
	})
	return m, nil
}

// incomingDetail says what arrives: the commits by subject, and the files they
// change, which is the part that matters for a vault.
func incomingDetail(result gitrepo.FetchResult) string {
	var parts []string
	var subjects []string
	for i, c := range result.Commits {
		if i == updateShown {
			subjects = append(subjects, fmt.Sprintf("and %s more", plural(len(result.Commits)-updateShown, "commit")))
			break
		}
		subjects = append(subjects, c.Subject)
	}
	if len(subjects) > 0 {
		parts = append(parts, strings.Join(subjects, "; "))
	}
	if len(result.Files) > 0 {
		parts = append(parts, "Files: "+changeSummary(result.Files))
	}
	parts = append(parts, "Only a fast-forward is made: nothing here is merged, rebased or stashed.")
	return strings.Join(parts, " · ")
}

func (m vaultModel) updateUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	flow := m.update
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch flow.stage {
	case updateFetching:
		if key.String() == "esc" {
			m.update = nil
		}
		return m, nil
	case updateRunning:
		return m, nil
	}
	dialog, decision := flow.dialog.Update(key.String())
	flow.dialog = dialog
	switch decision {
	case confirm.Cancelled:
		m.update = nil
	case confirm.Confirmed:
		flow.stage = updateRunning
		root, runner := m.root, m.gitRunner
		return m, func() tea.Msg {
			snap, err := runner.FastForward(root)
			return gitUpdatedMsg{snap: snap, err: err}
		}
	}
	return m, nil
}

func (m vaultModel) finishUpdate(msg gitUpdatedMsg) (tea.Model, tea.Cmd) {
	flow := m.update
	m.update = nil
	switch {
	case errors.Is(msg.err, gitrepo.ErrDiverged):
		m.notice = "! The branch moved here while updating; merge or rebase in a terminal."
	case msg.err != nil:
		m.notice = "! Update failed: " + msg.err.Error()
	default:
		m.notice = fmt.Sprintf("Updated to %s (%s)", msg.snap.Upstream, plural(len(flow.result.Commits), "commit"))
	}
	// The files on disk may have changed either way, so the folder is read
	// again rather than trusted.
	scan := m.scan(m.root)
	return m, scan
}

func (m vaultModel) updateView() string {
	flow := m.update
	if flow.stage == updateConfirm {
		return flow.dialog.View(min(96, m.width-4))
	}
	w, h := pickerSize(m.width, m.height)
	inner := max(1, w-4)
	lines := []string{titleStyle.Render("UPDATE VAULT"), ""}
	if flow.stage == updateRunning {
		lines = append(lines, titleStyle.Render("Fast-forwarding…"))
	} else {
		lines = append(lines, mutedStyle.Render("Asking "+updateRemote(flow)+" what it has…"))
	}
	if flow.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+flow.err), inner)...)
	}
	return modalBox(lines, pageactions.Footer(inner, "esc Cancel", ""), w, h)
}

func updateRemote(flow *updateFlow) string {
	if flow.result.Snapshot.Remote != "" {
		return flow.result.Snapshot.Remote
	}
	return "the remote"
}

func (m vaultModel) updateStatus() (string, string) {
	switch m.update.stage {
	case updateFetching:
		return "UPDATE · FETCHING", "esc Cancel"
	case updateRunning:
		return "UPDATE · RUNNING", ""
	}
	return "UPDATE · CONFIRM", "←→ Choose  ↵ Confirm  esc Cancel"
}
