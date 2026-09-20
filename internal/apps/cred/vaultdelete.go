package cred

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/cred/record"
	"dgs-toolbox/internal/tui/confirm"

	tea "github.com/charmbracelet/bubbletea"
)

// vaultDeleteFlow is the confirmation in front of removing a file from the
// vault. A vault file is the only copy of what is in it, so it goes to the
// trash rather than away: the keys page deletes an identity the same way, and
// the one mistake this page can make that nothing else undoes is worth a step
// backwards.
type vaultDeleteFlow struct {
	name string
	// paths are what is moved: the .age file, and its recipient record when it
	// has one. The record names hosts in plaintext and is of no use once the
	// file it describes is gone.
	paths   []string
	dialog  confirm.Model
	running bool
}

type vaultDeletedMsg struct {
	// trashed is where each path went, in the order they were moved.
	trashed []string
	err     error
}

// startVaultDelete begins deleting the file under the cursor.
func (m *vaultModel) startVaultDelete() tea.Cmd {
	if m.open != nil {
		// The file under the cursor is the one decrypted into memory. Moving
		// it while its contents are on screen leaves a page describing a file
		// that is no longer there.
		m.notice = "! Close the file first (esc), then delete it."
		return nil
	}
	file, _, ok := m.selectedFile()
	if !ok {
		if _, isDir := m.selectedDir(); isDir {
			m.notice = "! A folder is not deleted here; delete its files one by one."
			return nil
		}
		return nil
	}
	full := filepath.Join(m.root, filepath.FromSlash(file.Path))
	flow := &vaultDeleteFlow{name: path.Base(file.Path), paths: []string{full}}
	var also string
	if recordPath := record.PathFor(full); exists(recordPath) {
		flow.paths = append(flow.paths, recordPath)
		also = " Its recipient record " + filepath.Base(recordPath) + " goes with it."
	}
	flow.dialog = confirm.New(confirm.Config{
		Title:        "DELETE",
		Message:      fmt.Sprintf("Move %s to the trash?", file.Path),
		Detail:       "The vault is the only copy of what is inside it, unless this folder is in git or a backup." + also,
		ConfirmLabel: "Move to trash",
		CancelLabel:  "Cancel",
	})
	m.del = flow
	return nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func (m vaultModel) updateDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok || m.del.running {
		return m, nil
	}
	dialog, decision := m.del.dialog.Update(key.String())
	m.del.dialog = dialog
	switch decision {
	case confirm.Cancelled:
		m.del = nil
	case confirm.Confirmed:
		m.del.running = true
		return m, m.runDelete()
	}
	return m, nil
}

func (m vaultModel) runDelete() tea.Cmd {
	paths, trash := m.del.paths, m.trash
	return func() tea.Msg {
		msg := vaultDeletedMsg{}
		for _, p := range paths {
			where, err := trash(p)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.trashed = append(msg.trashed, where)
		}
		return msg
	}
}

func (m vaultModel) finishDelete(msg vaultDeletedMsg) (tea.Model, tea.Cmd) {
	flow := m.del
	m.del = nil
	if msg.err != nil {
		// A record left behind after its file moved is said plainly: the
		// folder is not in the state either outcome describes.
		m.notice = "! Delete failed: " + msg.err.Error()
		if len(msg.trashed) > 0 {
			m.notice = fmt.Sprintf("! Moved %s to the trash, but the rest failed: %v", flow.name, msg.err)
		}
	} else {
		where := ""
		if len(msg.trashed) > 0 {
			where = " to " + tilde(filepath.Dir(msg.trashed[0]))
		}
		m.notice = fmt.Sprintf("Moved %s%s", strings.Join(baseNames(flow.paths), " and "), where)
	}
	scan := m.scan(m.root)
	return m, scan
}

func baseNames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}

func (m vaultModel) deleteView() string {
	return m.del.dialog.View(min(96, m.width-4))
}
