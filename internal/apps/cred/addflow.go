package cred

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/cred/record"
	"dgs-toolbox/internal/cred/seal"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/pageactions"

	"filippo.io/age"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type addStage int

const (
	addSource addStage = iota
	addRecipients
	addName
	addDirectory
	addConfirm
	addRunning
)

// addFlow is adding a file or folder to the vault, from choosing the source to
// the result. It lives on the vault page as an overlay.
type addFlow struct {
	stage addStage
	// directory is where the file is written; source is what is encrypted.
	directory string
	source    string
	folder    bool

	picker fileexplorer.Model
	choose *recipientPicker

	name        textinput.Model
	onDirectory bool
	dialog      confirm.Model
	err         string
}

type sealedMsg struct {
	result seal.Result
	source string
	err    error
}

func pickerFor(start string, width, height int, filter fileexplorer.Filter) fileexplorer.Model {
	w, h := pickerSize(width, height)
	return fileexplorer.New(start, w-2, h-6, fileexplorer.WithFilter(filter))
}

// startAdd opens the flow at the source step.
func (m *vaultModel) startAdd() tea.Cmd {
	if m.root == "" {
		m.notice = "! Choose a vault folder first"
		return nil
	}
	directory := m.root
	if dir, ok := m.selectedDir(); ok {
		directory = filepath.Join(m.root, filepath.FromSlash(dir))
	} else if file, _, ok := m.selectedFile(); ok {
		directory = filepath.Join(m.root, filepath.FromSlash(path.Dir(file.Path)))
	}
	flow := &addFlow{stage: addSource, directory: directory}
	flow.picker = pickerFor(homeDir(), m.width, m.height, fileexplorer.AllFiles())
	m.add = flow
	return flow.picker.Init()
}

func (m vaultModel) updateAdd(msg tea.Msg) (tea.Model, tea.Cmd) {
	flow := m.add
	switch flow.stage {
	case addSource, addDirectory:
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" && !flow.picker.HasDialog() {
			if flow.stage == addSource {
				m.add = nil
			} else {
				flow.stage = addName
			}
			return m, nil
		}
		picker, selected, cmd := flow.picker.Update(msg)
		flow.picker = picker
		if selected == "" {
			return m, cmd
		}
		if flow.stage == addDirectory {
			flow.directory = selected
			flow.stage = addName
			return m, cmd
		}
		info, err := os.Stat(selected)
		if err != nil {
			flow.err = err.Error()
			return m, cmd
		}
		flow.source, flow.folder = selected, info.IsDir()
		if !flow.folder && info.Size() > seal.DefaultMaxSize {
			flow.err = fmt.Sprintf("%s is larger than %d MiB", tilde(selected), seal.DefaultMaxSize>>20)
			return m, cmd
		}
		flow.err = ""
		flow.choose = newRecipientPicker(m.snap.folder, m.snap.held, nil, nil)
		flow.stage = addRecipients
		return m, cmd
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch flow.stage {
	case addRecipients:
		return m.updateAddRecipients(key.String())
	case addName:
		return m.updateAddName(key)
	case addConfirm:
		dialog, decision := flow.dialog.Update(key.String())
		flow.dialog = dialog
		switch decision {
		case confirm.Cancelled:
			flow.stage = addName
		case confirm.Confirmed:
			flow.stage = addRunning
			return m, m.runSeal()
		}
	}
	return m, nil
}

func shortKey(key recipients.PublicKey) string {
	text := keyText(key)
	if len(text) > 24 {
		return text[:14] + "…" + text[len(text)-6:]
	}
	return text
}

func (m vaultModel) updateAddRecipients(key string) (tea.Model, tea.Cmd) {
	flow := m.add
	switch key {
	case "esc":
		flow.stage = addSource
		return m, nil
	case "enter":
		switch {
		case m.snap.folder.Errors():
			flow.err = "The recipient folder has errors; fix them in dgs cred keys first."
		case len(m.chosenRecipients()) == 0:
			flow.err = "Check at least one key."
		default:
			flow.err = ""
			flow.name = textinput.New()
			flow.name.SetValue(seal.DestinationName(flow.source, flow.folder))
			flow.name.CharLimit = 255
			flow.name.Focus()
			flow.name.CursorEnd()
			flow.onDirectory = false
			flow.stage = addName
			return m, textinput.Blink
		}
		return m, nil
	}
	flow.choose.update(key)
	return m, nil
}

// chosenRecipients are the keys checked in the add flow.
func (m vaultModel) chosenRecipients() []seal.Recipient { return m.add.choose.chosen() }

func (m vaultModel) updateAddName(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	flow := m.add
	switch key.String() {
	case "esc":
		flow.stage = addRecipients
		return m, nil
	case "tab", "shift+tab", "up", "down":
		flow.onDirectory = !flow.onDirectory
		if flow.onDirectory {
			flow.name.Blur()
		} else {
			flow.name.Focus()
		}
		return m, nil
	case "enter":
		if flow.onDirectory {
			flow.picker = pickerFor(flow.directory, m.width, m.height, fileexplorer.Directories())
			flow.stage = addDirectory
			return m, flow.picker.Init()
		}
		if err := m.checkDestination(); err != nil {
			flow.err = err.Error()
			return m, nil
		}
		flow.err = ""
		flow.dialog = confirm.New(m.confirmConfig())
		flow.stage = addConfirm
		return m, nil
	}
	if flow.onDirectory {
		return m, nil
	}
	var cmd tea.Cmd
	flow.name, cmd = flow.name.Update(key)
	return m, cmd
}

func (m vaultModel) destination() string {
	return filepath.Join(m.add.directory, strings.TrimSpace(m.add.name.Value()))
}

func (m vaultModel) checkDestination() error {
	name := strings.TrimSpace(m.add.name.Value())
	switch {
	case name == "" || name == "." || name == ".." || strings.ContainsRune(name, filepath.Separator):
		return errors.New("The name must be a single file name.")
	case !strings.HasSuffix(name, ".age"):
		return errors.New("The name must end in .age, or the vault will not list it.")
	case strings.HasPrefix(name, "."):
		return errors.New("The name must not start with a dot, or the vault will not list it.")
	}
	for _, p := range []string{m.destination(), record.PathFor(m.destination())} {
		if _, err := os.Lstat(p); err == nil {
			return fmt.Errorf("%s already exists.", tilde(p))
		}
	}
	return nil
}

func (m vaultModel) mineChosen() bool { return m.add.choose.mine() }

func (m vaultModel) confirmConfig() confirm.Config {
	var names []string
	for _, r := range m.chosenRecipients() {
		names = append(names, r.Host+" · "+r.Description)
	}
	kind := "file"
	if m.add.folder {
		kind = "folder"
	}
	detail := "For " + strings.Join(names, ", ") + "."
	if !m.mineChosen() {
		detail = "No checked key belongs to this machine: the file cannot be opened or checked here. " + detail
	}
	// The dialog gives each a line, so name the source by its name and the
	// destination within the vault; the full source path was on every step.
	destination := tilde(m.destination())
	if rel, err := filepath.Rel(m.root, m.destination()); err == nil && !strings.HasPrefix(rel, "..") {
		destination = filepath.ToSlash(rel)
	}
	return confirm.Config{
		Title:        "ADD TO VAULT",
		Message:      fmt.Sprintf("Encrypt the %s %s to %s?", kind, filepath.Base(m.add.source), destination),
		Detail:       detail,
		ConfirmLabel: "Encrypt",
		CancelLabel:  "Back",
	}
}

func (m vaultModel) runSeal() tea.Cmd {
	request := seal.Request{Source: m.add.source, Destination: m.destination(), Recipients: m.chosenRecipients()}
	snap := m.snap
	return func() tea.Msg {
		for _, identity := range snap.scan.Identities {
			if identity.Status != identities.Usable {
				continue
			}
			if opened, err := identities.Open(identity); err == nil {
				request.Verify = append(request.Verify, age.Identity(opened))
			}
		}
		result, err := seal.Seal(request)
		return sealedMsg{result: result, source: request.Source, err: err}
	}
}

func (m vaultModel) finishAdd(msg sealedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.add.stage = addName
		m.add.err = msg.err.Error()
		return m, nil
	}
	m.add = nil
	rel, err := filepath.Rel(m.root, msg.result.Path)
	if err != nil {
		rel = tilde(msg.result.Path)
	}
	state := "verified"
	if !msg.result.Verified {
		state = "unverified"
	}
	m.notice = fmt.Sprintf("Added %s (%s) · the plaintext is still at %s", filepath.ToSlash(rel), state, tilde(msg.source))
	scan := m.scan(m.root)
	return m, scan
}

func (m vaultModel) addView() string {
	flow := m.add
	w, h := pickerSize(m.width, m.height)
	inner := max(1, w-4)
	var lines []string
	title := "ADD TO VAULT"
	switch flow.stage {
	case addSource, addDirectory:
		heading := "SOURCE · a file or a folder"
		if flow.stage == addDirectory {
			heading = "DIRECTORY"
		}
		selected := ansi.Truncate(tilde(flow.picker.SelectedPath()), inner, "…")
		if notice := flow.picker.Notice(); notice != "" {
			selected = notice
		}
		lines = append(lines, titleStyle.Render(title+" · "+heading), mutedStyle.Render(selected), "", flow.picker.View(), mutedStyle.Render(flow.picker.Hint()))
	case addRecipients:
		lines = append(lines, titleStyle.Render(title+" · RECIPIENTS"), mutedStyle.Render(ansi.Truncate(tilde(flow.source), inner, "…")), "")
		lines = append(lines, flow.choose.view(inner, h-9))
	case addName, addConfirm, addRunning:
		lines = append(lines, titleStyle.Render(title+" · NAME"), mutedStyle.Render(ansi.Truncate(tilde(flow.source), inner, "…")), "")
		nameMarker, dirMarker := "› ", "  "
		if flow.onDirectory {
			nameMarker, dirMarker = "  ", "› "
		}
		lines = append(lines,
			nameMarker+mutedStyle.Render(fmt.Sprintf("%-10s", "Name"))+flow.name.View(),
			dirMarker+mutedStyle.Render(fmt.Sprintf("%-10s", "Directory"))+ansi.Truncate(tilde(flow.directory), max(1, inner-12), "…"),
			"",
			mutedStyle.Render(plural(len(m.chosenRecipients()), "recipient")),
		)
		if flow.stage == addRunning {
			lines = append(lines, "", titleStyle.Render("Encrypting and checking…"))
		}
	}
	if flow.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+flow.err), inner)...)
	}
	switch flow.stage {
	case addRecipients:
		return modalBox(lines, pageactions.Footer(inner, fmt.Sprintf("space Check · esc Back · %s checked", plural(len(m.chosenRecipients()), "key")), pageactions.Inline("Continue", true)), w, h)
	case addName:
		return modalBox(lines, pageactions.Footer(inner, "tab Field · esc Back", pageactions.Inline("Continue", !flow.onDirectory)), w, h)
	}
	if flow.stage == addConfirm {
		return flow.dialog.View(min(88, m.width-4))
	}
	return modalStyle.Width(w - 2).Height(h - 2).MaxWidth(w).MaxHeight(h).Render(strings.Join(lines, "\n"))
}

// modalBox draws an overlay with its footer on the last row.
func modalBox(lines []string, footer string, w, h int) string {
	rows := max(1, h-2)
	// An entry may itself hold several rows, such as a rendered form.
	lines = strings.Split(strings.Join(lines, "\n"), "\n")
	if len(lines) > rows-1 {
		lines = lines[:rows-1]
	}
	for len(lines) < rows-1 {
		lines = append(lines, "")
	}
	lines = append(lines, footer)
	return modalStyle.Width(w - 2).Height(h - 2).MaxWidth(w).MaxHeight(h).Render(strings.Join(lines, "\n"))
}

func (m vaultModel) addStatus() (string, string) {
	switch m.add.stage {
	case addSource:
		return "ADD · SOURCE", m.add.picker.Hint()
	case addDirectory:
		return "ADD · DIRECTORY", m.add.picker.Hint()
	case addRecipients:
		return "ADD · RECIPIENTS", "↑↓ Move  space Check  ↵ Continue  esc Back"
	case addName:
		return "ADD · NAME", "tab Field  ↵ Continue  esc Back"
	case addConfirm:
		return "ADD · CONFIRM", ""
	}
	return "ADD · RUNNING", ""
}
