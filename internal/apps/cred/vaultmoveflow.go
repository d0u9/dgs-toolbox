package cred

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/cred/vaultmove"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/pageactions"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	vaultNameFieldID   = "name"
	vaultFolderFieldID = "folder"
)

// moveFlow renames a vault file and moves it to another folder of the same
// vault: both are one operation, since both are a rename of the file and of
// the record beside it.
type moveFlow struct {
	// from is the file's path now, relative to the vault root.
	from   string
	stage  keyStage
	form   form.Model
	dialog confirm.Model
	err    string
}

type movedMsg struct {
	result vaultmove.Result
	err    error
}

// startMove opens the flow on the file under the cursor.
func (m *vaultModel) startMove() {
	if m.root == "" {
		m.notice = "! Choose a vault folder first"
		return
	}
	if m.open != nil {
		// As when deleting: the file on screen must not move under the page
		// that describes it.
		m.notice = "! Close the file first (esc), then move it."
		return
	}
	file, _, ok := m.selectedFile()
	if !ok {
		if _, isDir := m.selectedDir(); isDir {
			m.notice = "! A folder is not moved here; move its files one by one."
		}
		return
	}
	folder := path.Dir(file.Path)
	if folder == "." {
		folder = ""
	}
	flow := &moveFlow{from: file.Path, stage: keyForm}
	flow.form = form.New(
		form.Field{ID: vaultNameFieldID, Kind: form.Text, Label: "Name", Value: path.Base(file.Path)},
		form.Field{ID: vaultFolderFieldID, Kind: form.Text, Label: "Folder", Value: folder},
		form.Field{ID: continueFieldID, Kind: form.Button, Label: "Move"},
	)
	flow.form.HandleInteraction("enter")
	m.move = flow
}

func moveFieldIDs() []string {
	return []string{vaultNameFieldID, vaultFolderFieldID, continueFieldID}
}

func (m vaultModel) updateMove(msg tea.Msg) (tea.Model, tea.Cmd) {
	flow := m.move
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
		return m.updateMoveForm(key.String())
	case keyConfirm:
		dialog, decision := flow.dialog.Update(key.String())
		flow.dialog = dialog
		switch decision {
		case confirm.Cancelled:
			flow.stage = keyForm
		case confirm.Confirmed:
			flow.stage = keyRunning
			return m, m.runMove()
		}
	}
	return m, nil
}

func (m vaultModel) updateMoveForm(key string) (tea.Model, tea.Cmd) {
	flow := m.move
	ids := moveFieldIDs()
	if flow.form.IsActive() {
		// As on the keys page: Enter keeps the edit and goes on, so the form
		// is filled in one pass.
		switch key {
		case "enter", "tab", "down", "shift+tab", "up":
			flow.form.HandleInteraction("enter")
			direction := "down"
			if key == "shift+tab" || key == "up" {
				direction = "up"
			}
			if direction == "down" && flow.form.FocusedID() == ids[len(ids)-2] {
				flow.form.SetFocusID(continueFieldID)
				return m, nil
			}
			flow.form.UpdateNavigationWithin(ids, direction)
			if flow.form.FocusedID() != continueFieldID {
				flow.form.HandleInteraction("enter")
			}
			return m, nil
		}
		flow.form.HandleInteraction(key)
		return m, nil
	}
	if key == "esc" {
		m.move = nil
		return m, nil
	}
	if flow.form.FocusedID() == continueFieldID && (key == "enter" || key == " ") {
		if err := m.checkMoveForm(); err != nil {
			flow.err = err.Error()
			return m, nil
		}
		flow.err = ""
		flow.dialog = confirm.New(m.moveConfirmConfig())
		flow.stage = keyConfirm
		return m, nil
	}
	if flow.form.HandleInteraction(key) {
		return m, nil
	}
	flow.form.UpdateNavigationWithin(ids, key)
	return m, nil
}

// moveDestination is where the file goes, relative to the vault root.
func (m vaultModel) moveDestination() string {
	name := strings.TrimSpace(m.move.form.Value(vaultNameFieldID))
	folder := strings.Trim(strings.TrimSpace(m.move.form.Value(vaultFolderFieldID)), "/")
	folder = filepath.ToSlash(folder)
	if folder == "" || folder == "." {
		return name
	}
	return folder + "/" + name
}

func (m vaultModel) checkMoveForm() error {
	name := strings.TrimSpace(m.move.form.Value(vaultNameFieldID))
	if name == "" {
		return errors.New("Name is required.")
	}
	if err := vaultmove.CheckName(name); err != nil {
		return sentence(err)
	}
	folder := strings.Trim(strings.TrimSpace(m.move.form.Value(vaultFolderFieldID)), "/")
	for _, part := range strings.Split(filepath.ToSlash(folder), "/") {
		if part == ".." {
			return errors.New("The folder must stay inside the vault.")
		}
	}
	if err := vaultmove.CheckFolder(folder); err != nil {
		return sentence(err)
	}
	if m.moveDestination() == m.move.from {
		return errors.New("It is already there.")
	}
	for _, file := range m.listing.Files {
		if file.Path == m.moveDestination() {
			return fmt.Errorf("%s already exists.", m.moveDestination())
		}
	}
	return nil
}

// sentence makes a package's lowercase error read as one in the interface.
func sentence(err error) error {
	text := err.Error()
	if text == "" {
		return err
	}
	return errors.New(strings.ToUpper(text[:1]) + text[1:] + ".")
}

// vaultFolders are the folders the vault already has, for the form to list.
func (m vaultModel) vaultFolders() []string {
	seen := map[string]bool{}
	for _, file := range m.listing.Files {
		for dir := path.Dir(file.Path); dir != "." && dir != "/"; dir = path.Dir(dir) {
			seen[dir] = true
		}
	}
	folders := make([]string, 0, len(seen))
	for dir := range seen {
		folders = append(folders, dir)
	}
	sort.Strings(folders)
	return folders
}

func (m vaultModel) moveConfirmConfig() confirm.Config {
	to := m.moveDestination()
	title, verb := "RENAME", "Rename"
	if path.Dir(to) != path.Dir(m.move.from) {
		title, verb = "MOVE", "Move"
	}
	notes := []string{"Its recipient record moves with it, under the new name."}
	if dir := path.Dir(to); dir != "." && !m.hasFolder(dir) {
		notes = append(notes, dir+" does not exist yet and is created.")
	}
	notes = append(notes, "The file itself is not decrypted or re-encrypted, so it opens as before.")
	return confirm.Config{
		Title:        title,
		Message:      fmt.Sprintf("%s %s to %s?", verb, m.move.from, to),
		Detail:       strings.Join(notes, " "),
		ConfirmLabel: verb,
		CancelLabel:  "Back",
	}
}

func (m vaultModel) hasFolder(dir string) bool {
	for _, folder := range m.vaultFolders() {
		if folder == dir {
			return true
		}
	}
	return false
}

func (m vaultModel) runMove() tea.Cmd {
	root, from, to := m.root, m.move.from, m.moveDestination()
	return func() tea.Msg {
		result, err := vaultmove.Move(root, from, to)
		return movedMsg{result: result, err: err}
	}
}

func (m vaultModel) finishMove(msg movedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.move.stage = keyForm
		m.move.err = sentence(msg.err).Error()
		return m, nil
	}
	m.move = nil
	m.notice = fmt.Sprintf("Moved %s to %s", msg.result.From, msg.result.To)
	if msg.result.Record != "" {
		m.notice += " with its record"
	}
	m.selectPath = msg.result.To
	scan := m.scan(m.root)
	return m, scan
}

func (m vaultModel) moveView() string {
	flow := m.move
	if flow.stage == keyConfirm {
		return flow.dialog.ViewSize(min(96, m.width-4), m.height)
	}
	w, h := pickerSize(m.width, m.height)
	inner := max(1, w-4)
	lines := []string{
		titleStyle.Render("RENAME OR MOVE"),
		mutedStyle.Render(flow.from),
		"",
		flow.form.ViewFocusedWidth([]string{vaultNameFieldID, vaultFolderFieldID}, true, inner),
		"",
		mutedStyle.Render("An empty folder is the vault's own folder. A folder that does not exist is created."),
	}
	if folders := m.vaultFolders(); len(folders) > 0 {
		lines = append(lines, wrapped(mutedStyle.Render("Folders: "+strings.Join(folders, ", ")), inner)...)
	}
	if flow.stage == keyRunning {
		lines = append(lines, "", titleStyle.Render("Moving…"))
	}
	if flow.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+flow.err), inner)...)
	}
	hints := "↑↓ Field · ↵ Edit · esc Back"
	if flow.form.IsActive() {
		hints = "Type to edit · ↵ Next field · esc Undo"
	}
	button := pageactions.Inline("Move", flow.form.FocusedID() == continueFieldID)
	return modalBox(lines, pageactions.Footer(inner, hints, button), w, h)
}

func (m vaultModel) moveStatus() tui.Status {
	flow := m.move
	switch {
	case flow.stage == keyForm && flow.form.IsActive():
		return tui.Status{Left: "MOVE", Center: flow.from, Right: "↵ Next field  esc Undo"}
	case flow.stage == keyForm:
		return tui.Status{Left: "MOVE", Center: flow.from, Right: "↑↓ Field  ↵ Edit  esc Back"}
	}
	return tui.Status{Left: "MOVE", Center: flow.from}
}
