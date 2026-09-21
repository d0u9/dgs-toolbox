package cred

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/gitrepo"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/pageactions"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type gitStage int

const (
	// gitReading is the working tree being read; gitMessage asks for the
	// commit message, gitConfirm names what will happen, gitRunning does it.
	gitReading gitStage = iota
	gitMessage
	gitConfirm
	gitRunning
)

// gitChangesShown is how many paths the confirmation names before it counts
// the rest. A vault commit is usually one or two files; a long list would
// push the sentence saying what happens off the dialog.
const gitChangesShown = 8

// gitFlow is `add -A`, `commit` and `push` of the folder the vault is in,
// which is the whole of publishing a change made here. It is one flow rather
// than three commands because that is one decision — put what is on this
// machine on the server — and the steps in between are never wanted alone.
type gitFlow struct {
	stage  gitStage
	snap   gitrepo.Snapshot
	input  textinput.Model
	dialog confirm.Model
	err    string
}

type gitStatusMsg struct {
	snap gitrepo.Snapshot
	err  error
}

type gitPublishedMsg struct {
	result gitrepo.Result
	err    error
}

// startGit begins publishing the vault folder. The folder is read first: what
// the dialog has to say — the branch, the remote, the files — is only known
// from the working tree, and a folder that is not in one is refused before
// anything is typed.
func (m *vaultModel) startGit() tea.Cmd {
	if m.root == "" {
		m.notice = "! No vault folder yet."
		return nil
	}
	m.git = &gitFlow{stage: gitReading}
	root, runner := m.root, m.gitRunner
	return func() tea.Msg {
		snap, err := runner.Status(root)
		return gitStatusMsg{snap: snap, err: err}
	}
}

func (m vaultModel) finishGitStatus(msg gitStatusMsg) (tea.Model, tea.Cmd) {
	if m.git == nil {
		return m, nil
	}
	switch {
	case errors.Is(msg.err, gitrepo.ErrNotInstalled):
		m.git = nil
		m.notice = "! git is not installed, so the vault folder cannot be published from here."
		return m, nil
	case errors.Is(msg.err, gitrepo.ErrNotARepository):
		m.git = nil
		m.notice = "! " + tilde(m.root) + " is not in a git repository."
		return m, nil
	case msg.err != nil:
		m.git = nil
		m.notice = "! " + msg.err.Error()
		return m, nil
	}
	flow := m.git
	flow.snap = msg.snap
	if msg.snap.Clean() && msg.snap.Ahead == 0 && msg.snap.Upstream != "" {
		m.git = nil
		m.notice = "Nothing to publish: no change here and nothing the remote lacks."
		return m, nil
	}
	if msg.snap.Detached {
		m.git = nil
		m.notice = "! HEAD is detached: check out a branch before publishing."
		return m, nil
	}
	flow.stage = gitMessage
	flow.input = textinput.New()
	flow.input.Prompt = ""
	flow.input.SetValue(defaultCommitMessage(msg.snap))
	flow.input.CursorEnd()
	flow.input.Focus()
	if msg.snap.Clean() {
		// Nothing to commit: the push is the whole of it, so there is no
		// message to ask for.
		return m.confirmGit()
	}
	return m, textinput.Blink
}

// defaultCommitMessage says what the commit holds, which is what a vault
// commit has to say: the names are already in the tree, and the one thing
// `git log` cannot show without reading it is how much moved.
func defaultCommitMessage(snap gitrepo.Snapshot) string {
	if len(snap.Changes) == 1 {
		return "vault: update " + filepath.Base(snap.Changes[0].Path)
	}
	return fmt.Sprintf("vault: update %s", plural(len(snap.Changes), "file"))
}

func (m vaultModel) updateGit(msg tea.Msg) (tea.Model, tea.Cmd) {
	flow := m.git
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch flow.stage {
	case gitReading, gitRunning:
		if key.String() == "esc" && flow.stage == gitReading {
			m.git = nil
		}
		return m, nil
	case gitConfirm:
		dialog, decision := flow.dialog.Update(key.String())
		flow.dialog = dialog
		switch decision {
		case confirm.Cancelled:
			if flow.snap.Clean() {
				m.git = nil
				return m, nil
			}
			flow.stage = gitMessage
		case confirm.Confirmed:
			flow.stage = gitRunning
			return m, m.runGit()
		}
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.git = nil
		return m, nil
	case "enter":
		return m.confirmGit()
	}
	var cmd tea.Cmd
	flow.input, cmd = flow.input.Update(key)
	return m, cmd
}

// confirmGit names every step that will run, and on what.
func (m vaultModel) confirmGit() (tea.Model, tea.Cmd) {
	flow := m.git
	message := strings.TrimSpace(flow.input.Value())
	if !flow.snap.Clean() && message == "" {
		flow.err = "Type a commit message."
		return m, nil
	}
	flow.err = ""
	flow.input.SetValue(message)

	where := "no remote"
	if flow.snap.Remote != "" {
		where = flow.snap.Remote + "/" + flow.snap.Branch
		if flow.snap.Upstream != "" {
			where = flow.snap.Upstream
		}
	}
	var steps []string
	if !flow.snap.Clean() {
		steps = append(steps, fmt.Sprintf("commit %s", plural(len(flow.snap.Changes), "change")))
	}
	if flow.snap.Ahead > 0 && flow.snap.Remote != "" {
		steps = append(steps, fmt.Sprintf("push %s already committed", plural(flow.snap.Ahead, "commit")))
	}
	if flow.snap.Remote != "" && len(steps) < 2 {
		steps = append(steps, "push "+where)
	}
	var notes []string
	if flow.snap.Remote == "" {
		notes = append(notes, "This repository has no remote, so nothing is pushed; the commit stays on this machine.")
	}
	if flow.snap.Behind > 0 {
		notes = append(notes, fmt.Sprintf("%s is %s ahead of this branch, so the push may be rejected; press f to update first.", where, plural(flow.snap.Behind, "commit")))
	}
	notes = append(notes, "Everything changed under "+tilde(flow.snap.Root)+" is staged, not only the vault folder.")
	if !flow.snap.Clean() {
		notes = append(notes, "Files: "+changeSummary(flow.snap.Changes))
	}
	flow.dialog = confirm.New(confirm.Config{
		Title:        "PUBLISH",
		Message:      fmt.Sprintf("%s on %s?", capitalize(strings.Join(steps, ", then ")), branchName(flow.snap)),
		Detail:       strings.Join(notes, " "),
		ConfirmLabel: "Publish",
		CancelLabel:  "Back",
	})
	flow.stage = gitConfirm
	return m, nil
}

func branchName(snap gitrepo.Snapshot) string {
	if snap.Branch == "" {
		return "this repository"
	}
	return snap.Branch
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// changeSummary names the paths a commit holds, and counts the rest.
func changeSummary(changes []gitrepo.Change) string {
	names := make([]string, 0, len(changes))
	for i, c := range changes {
		if i == gitChangesShown {
			names = append(names, fmt.Sprintf("and %s more", plural(len(changes)-gitChangesShown, "path")))
			break
		}
		names = append(names, c.Path)
	}
	return strings.Join(names, ", ")
}

func (m vaultModel) runGit() tea.Cmd {
	request := gitrepo.PublishRequest{Dir: m.root, Message: strings.TrimSpace(m.git.input.Value())}
	runner := m.gitRunner
	return func() tea.Msg {
		result, err := runner.Publish(request)
		return gitPublishedMsg{result: result, err: err}
	}
}

func (m vaultModel) finishGit(msg gitPublishedMsg) (tea.Model, tea.Cmd) {
	flow := m.git
	if msg.err != nil {
		flow.stage = gitMessage
		flow.err = msg.err.Error()
		if flow.snap.Clean() {
			// There is no message to go back to, so the flow has nothing left
			// to ask and says what happened on the page.
			m.git = nil
			m.notice = "! Publish failed: " + msg.err.Error()
		}
		return m, nil
	}
	m.git = nil
	m.notice = publishedNotice(msg.result)
	scan := m.scan(m.root)
	return m, scan
}

func publishedNotice(result gitrepo.Result) string {
	var parts []string
	if result.Committed {
		parts = append(parts, fmt.Sprintf("Committed %s as %s", plural(result.Staged, "file"), result.Commit))
	}
	if result.Pushed {
		parts = append(parts, "pushed")
	}
	if len(parts) == 0 {
		if result.Note != "" {
			return capitalize(result.Note)
		}
		return "Nothing to do"
	}
	notice := strings.Join(parts, " and ")
	if result.Note != "" && !result.Pushed {
		notice += " (" + result.Note + ")"
	}
	return notice
}

func (m vaultModel) gitView() string {
	flow := m.git
	if flow.stage == gitConfirm {
		return flow.dialog.ViewSize(min(96, m.width-4), m.height)
	}
	w, h := pickerSize(m.width, m.height)
	inner := max(1, w-4)
	lines := []string{titleStyle.Render("PUBLISH VAULT"), ""}
	footer := pageactions.Footer(inner, "esc Cancel", "")
	switch flow.stage {
	case gitReading:
		lines = append(lines, mutedStyle.Render("Reading the working tree…"))
	case gitRunning:
		lines = append(lines, titleStyle.Render("Committing and pushing…"))
	default:
		lines = append(lines, wrapped(fmt.Sprintf("%s on %s, %s.", tilde(flow.snap.Root), branchName(flow.snap), plural(len(flow.snap.Changes), "change")), inner)...)
		lines = append(lines, "", mutedStyle.Render("Commit message"), "› "+flow.input.View())
		footer = pageactions.Footer(inner, "↵ Continue · esc Cancel", pageactions.Inline("Continue", true))
	}
	if flow.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+flow.err), inner)...)
	}
	return modalBox(lines, footer, w, h)
}

func (m vaultModel) gitStatus() (string, string) {
	switch m.git.stage {
	case gitReading:
		return "PUBLISH · READING", "esc Cancel"
	case gitConfirm:
		return "PUBLISH · CONFIRM", ""
	case gitRunning:
		return "PUBLISH · RUNNING", ""
	}
	return "PUBLISH · MESSAGE", "↵ Continue  esc Cancel"
}
