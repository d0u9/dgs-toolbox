package cred

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"dgs-toolbox/internal/cred/actions"
	"dgs-toolbox/internal/cred/opened"
	"dgs-toolbox/internal/cred/sshagent"
	"dgs-toolbox/internal/cred/sshconfig"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/pageactions"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type actionStage int

const (
	actionMenu actionStage = iota
	actionForm
	actionFolder
	actionConfirm
	actionRunning
)

const (
	fieldAlias    = "alias"
	fieldLocation = "location"
	fieldHostOn   = "write_host"
	fieldHostName = "hostname"
	fieldUser     = "user"
	fieldPort     = "port"
	fieldInclude  = "include"
	fieldLifetime = "lifetime"
	fieldConfirm  = "confirm"
	fieldName     = "name"
	fieldFolder   = "folder"
	fieldRun      = "run"
)

// actionFlow is running an Action on an entry of the open file, from choosing
// it to the result.
type actionFlow struct {
	stage   actionStage
	entry   opened.Entry
	choices []actions.Action
	menu    scrolllist.Model
	action  actions.Action
	form    form.Model
	ids     []string
	picker  fileexplorer.Model
	plan    actions.Plan
	dialog  confirm.Model
	err     string
}

type actionDoneMsg struct {
	summary string
	err     error
}

func actionEnv(m vaultModel) actions.Env {
	return actions.Env{Home: homeDir(), NewIdentityDir: m.snap.settings.NewIdentityDir}
}

// otherLocation is the Location choice that asks for a folder.
const otherLocation = "Other…"

// visibleIDs are the fields shown now: ssh.install hides the folder unless
// Other is chosen, and the Host fields unless a Host entry is written.
func (m vaultModel) visibleIDs() []string {
	flow := m.action
	if flow.action.ID != actions.SSHInstall {
		return flow.ids
	}
	var ids []string
	for _, id := range flow.ids {
		switch id {
		case fieldFolder:
			if flow.form.Value(fieldLocation) != otherLocation {
				continue
			}
		case fieldHostName, fieldUser, fieldPort, fieldInclude:
			if !flow.form.Checked(fieldHostOn) {
				continue
			}
		}
		ids = append(ids, id)
	}
	return ids
}

// startAction opens the menu of Actions for the selected entry.
func (m *vaultModel) startAction() {
	entry, ok := m.selectedEntry()
	if !ok {
		return
	}
	choices := actions.For(entry.Kind)
	if len(choices) == 0 {
		m.notice = "! No actions for " + string(entry.Kind) + " yet"
		return
	}
	flow := &actionFlow{stage: actionMenu, entry: entry, choices: choices, menu: scrolllist.New()}
	items := make([]scrolllist.Item, len(choices))
	for i, choice := range choices {
		items[i] = scrolllist.Item{ID: string(choice.ID), Label: choice.Label, Detail: "    " + string(choice.ID)}
	}
	flow.menu.SetItems(items)
	m.action = flow
}

// entryLabel names an entry for a heading; the synthetic root is the file.
func (m vaultModel) entryLabel(entry opened.Entry) string {
	if entry.Path == actions.RootPath && m.open != nil {
		return m.open.opened.Name + "/"
	}
	return path.Base(entry.Path)
}

func baseWithoutExt(name string) string {
	base := path.Base(name)
	if ext := path.Ext(base); ext != "" && ext != base {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

func (m *vaultModel) chooseAction(action actions.Action) tea.Cmd {
	flow := m.action
	flow.action, flow.err = action, ""
	name := path.Base(flow.entry.Path)
	if flow.entry.Path == actions.RootPath {
		name = m.open.opened.Name
	}
	var fields []form.Field
	switch action.ID {
	case actions.SSHInstall:
		fields = []form.Field{
			{ID: fieldAlias, Kind: form.Text, Label: "Name", Value: baseWithoutExt(name)},
			{ID: fieldLocation, Kind: form.Option, Label: "Location", Value: actions.DefaultKeyDir, Options: append(append([]string(nil), actions.KeyLocations...), otherLocation)},
			{ID: fieldFolder, Kind: form.Path, Label: "Folder"},
			{ID: fieldHostOn, Kind: form.Checkbox, Label: "Write Host entry in ~/.ssh/config.d", Checked: true},
			{ID: fieldHostName, Kind: form.Text, Label: "Host name"},
			{ID: fieldUser, Kind: form.Text, Label: "User"},
			{ID: fieldPort, Kind: form.Text, Label: "Port"},
		}
		if actions.NeedsInclude(actionEnv(*m)) {
			fields = append(fields, form.Field{ID: fieldInclude, Kind: form.Checkbox, Label: "Add Include ~/.ssh/config.d/* to ~/.ssh/config", Checked: true})
		}
	case actions.SSHAgent:
		fields = []form.Field{
			{ID: fieldLifetime, Kind: form.Text, Label: "Lifetime", Value: actions.DefaultLifetime.String()},
			{ID: fieldConfirm, Kind: form.Checkbox, Label: "Confirm each use"},
		}
	case actions.SSHConfig:
		fields = []form.Field{{ID: fieldName, Kind: form.Text, Label: "Name", Value: baseWithoutExt(name)}}
		if actions.NeedsInclude(actionEnv(*m)) {
			fields = append(fields, form.Field{ID: fieldInclude, Kind: form.Checkbox, Label: "Add Include ~/.ssh/config.d/* to ~/.ssh/config", Checked: true})
		}
	case actions.AgeInstall:
		fields = []form.Field{{ID: fieldName, Kind: form.Text, Label: "File", Value: name}}
	case actions.FileSave, actions.DirSave:
		fields = []form.Field{
			{ID: fieldFolder, Kind: form.Path, Label: "Folder", Value: "~"},
			{ID: fieldName, Kind: form.Text, Label: "Name", Value: name},
		}
	}
	fields = append(fields, form.Field{ID: fieldRun, Kind: form.Button, Label: "Continue"})
	flow.form = form.New(fields...)
	flow.ids = nil
	for _, field := range fields {
		flow.ids = append(flow.ids, field.ID)
	}
	// Start on the field that has to be filled in, editing it.
	if action.ID == actions.SSHInstall {
		flow.form.SetFocusID(fieldHostName)
	}
	if field, ok := firstField(fields, flow.form.FocusedID()); ok && field.Kind == form.Text {
		flow.form.HandleInteraction("enter")
	}
	flow.stage = actionForm
	return nil
}

func firstField(fields []form.Field, id string) (form.Field, bool) {
	for _, field := range fields {
		if field.ID == id {
			return field, true
		}
	}
	return form.Field{}, false
}

func (m vaultModel) updateAction(msg tea.Msg) (tea.Model, tea.Cmd) {
	flow := m.action
	if m.open != nil {
		m.open.lastInput = time.Now()
	}
	if flow.stage == actionFolder {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" && !flow.picker.HasDialog() {
			flow.stage = actionForm
			return m, nil
		}
		picker, selected, cmd := flow.picker.Update(msg)
		flow.picker = picker
		if selected != "" {
			flow.form.SetValue(fieldFolder, tilde(selected))
			flow.stage = actionForm
		}
		return m, cmd
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch flow.stage {
	case actionMenu:
		switch key.String() {
		case "esc":
			m.action = nil
		case "enter":
			if item, ok := flow.menu.Selected(); ok {
				for _, choice := range flow.choices {
					if string(choice.ID) == item.ID {
						return m, m.chooseAction(choice)
					}
				}
			}
		default:
			var pending bool
			moveKey(&flow.menu, key.String(), &pending)
		}
	case actionForm:
		if key.Paste {
			flow.form.Paste(string(key.Runes))
			return m, nil
		}
		return m.updateActionForm(key.String())
	case actionConfirm:
		dialog, decision := flow.dialog.Update(key.String())
		flow.dialog = dialog
		switch decision {
		case confirm.Cancelled:
			flow.stage = actionForm
		case confirm.Confirmed:
			flow.stage = actionRunning
			return m, m.runAction()
		}
	}
	return m, nil
}

func (m vaultModel) updateActionForm(key string) (tea.Model, tea.Cmd) {
	flow := m.action
	if flow.form.IsActive() {
		// An open option menu keeps its own keys; only a text edit moves on.
		if kind, _ := m.focusedActionField(); kind != form.Text {
			flow.form.HandleInteraction(key)
			return m, nil
		}
		switch key {
		case "enter", "tab", "down", "shift+tab", "up":
			flow.form.HandleInteraction("enter")
			direction := "down"
			if key == "shift+tab" || key == "up" {
				direction = "up"
			}
			flow.form.UpdateNavigationWithin(m.visibleIDs(), direction)
			if field, ok := m.focusedActionField(); ok && field == form.Text {
				flow.form.HandleInteraction("enter")
			}
			return m, nil
		}
		flow.form.HandleInteraction(key)
		return m, nil
	}
	switch {
	case key == "esc":
		flow.stage = actionMenu
		return m, nil
	case flow.form.FocusedID() == fieldRun && (key == "enter" || key == " "):
		return m.planAction()
	case flow.form.FocusedID() == fieldFolder && key == "enter":
		start := actionEnv(m).Expand(flow.form.Value(fieldFolder))
		if start == "" {
			start = homeDir()
		}
		flow.picker = pickerFor(start, m.width, m.height, fileexplorer.Directories())
		flow.stage = actionFolder
		return m, flow.picker.Init()
	}
	if flow.form.HandleInteraction(key) {
		return m, nil
	}
	flow.form.UpdateNavigationWithin(m.visibleIDs(), key)
	return m, nil
}

func (m vaultModel) focusedActionField() (form.Kind, bool) {
	id := m.action.form.FocusedID()
	switch id {
	case fieldRun:
		return form.Button, true
	case fieldInclude, fieldConfirm, fieldHostOn:
		return form.Checkbox, true
	case fieldFolder:
		return form.Path, true
	case fieldLocation:
		return form.Option, true
	}
	return form.Text, id != ""
}

func (m vaultModel) value(id string) string {
	return strings.TrimSpace(m.action.form.Value(id))
}

// planAction works out what the Action will write and asks for confirmation.
func (m vaultModel) planAction() (tea.Model, tea.Cmd) {
	flow := m.action
	env := actionEnv(m)
	var plan actions.Plan
	var err error
	config := confirm.Config{Title: strings.ToUpper(flow.action.Label), ConfirmLabel: "Run", CancelLabel: "Back"}
	switch flow.action.ID {
	case actions.SSHInstall:
		keyDir := m.value(fieldLocation)
		if keyDir == otherLocation {
			keyDir = m.value(fieldFolder)
		}
		if keyDir == "" {
			err = fmt.Errorf("choose a folder for Other")
			break
		}
		plan, err = actions.PlanSSHInstall(flow.entry, actions.SSHInstallOptions{
			Name:       m.value(fieldAlias),
			KeyDir:     keyDir,
			WriteHost:  flow.form.Checked(fieldHostOn),
			Host:       sshconfig.Host{HostName: m.value(fieldHostName), User: m.value(fieldUser), Port: m.value(fieldPort)},
			AddInclude: flow.form.Checked(fieldInclude),
		}, env)
	case actions.SSHConfig:
		plan, err = actions.PlanSSHConfig(flow.entry, m.value(fieldName), flow.form.Checked(fieldInclude), env)
	case actions.AgeInstall:
		plan, err = actions.PlanAgeInstall(flow.entry, m.value(fieldName), env)
	case actions.FileSave:
		plan, err = actions.PlanSave(flow.entry, m.value(fieldFolder), m.value(fieldName), env)
	case actions.DirSave:
		plan, err = actions.PlanDirSave(m.open.opened.Entries, flow.entry.Path, m.value(fieldFolder), m.value(fieldName), env)
	case actions.SSHAgent:
		lifetime, parseErr := time.ParseDuration(m.value(fieldLifetime))
		if m.value(fieldLifetime) == "0" {
			lifetime, parseErr = 0, nil
		}
		switch {
		case parseErr != nil || lifetime < 0:
			err = fmt.Errorf("lifetime %q is not a duration such as 1h or 30m", m.value(fieldLifetime))
		case os.Getenv(sshagent.EnvSocket) == "":
			err = fmt.Errorf("%s is not set, so there is no agent to add to", sshagent.EnvSocket)
		default:
			kept := "until the agent stops"
			if lifetime > 0 {
				kept = "for " + lifetime.String()
			}
			config.Message = fmt.Sprintf("Add %s to the ssh-agent %s?", path.Base(flow.entry.Path), kept)
			if flow.form.Checked(fieldConfirm) {
				config.Detail = "The agent will ask before each use. "
			}
			config.Detail += "No file is written."
		}
	}
	if err != nil {
		flow.err = err.Error()
		return m, nil
	}
	if flow.action.ID == actions.DirSave {
		target := tilde(plan.Dirs[0].Path)
		config.Message = fmt.Sprintf("Write %s and %s into %s?",
			plural(len(plan.Writes), "file"), plural(len(plan.Dirs)-1, "folder"), target)
		var notes []string
		if len(plan.Skipped) > 0 {
			notes = append(notes, "Not written: "+strings.Join(plan.Skipped, ", ")+".")
		}
		config.Detail = strings.Join(notes, " ")
	} else if flow.action.ID != actions.SSHAgent {
		var paths []string
		for _, w := range plan.Writes {
			paths = append(paths, tilde(w.Path))
		}
		config.Message = "Write " + strings.Join(paths, ", ") + "?"
		var notes []string
		for _, kept := range plan.Kept {
			notes = append(notes, tilde(kept)+" already holds this key and is kept.")
		}
		if plan.IncludeIn != "" {
			notes = append(notes, "Add “Include "+sshconfig.IncludePattern+"” as the first line of "+tilde(plan.IncludeIn)+".")
		}
		config.Detail = strings.Join(notes, " ")
	}
	flow.plan, flow.err = plan, ""
	flow.dialog = confirm.New(config)
	flow.stage = actionConfirm
	return m, nil
}

func (m vaultModel) runAction() tea.Cmd {
	flow := m.action
	entry, plan := flow.entry, flow.plan
	switch flow.action.ID {
	case actions.SSHAgent:
		lifetime, _ := time.ParseDuration(m.value(fieldLifetime))
		comment := m.open.file.Path + "/" + entry.Path
		confirmUse := flow.form.Checked(fieldConfirm)
		socket := os.Getenv(sshagent.EnvSocket)
		return func() tea.Msg {
			if err := actions.AddToAgent(entry, socket, comment, lifetime, confirmUse); err != nil {
				return actionDoneMsg{err: err}
			}
			return actionDoneMsg{summary: "Added " + path.Base(entry.Path) + " to ssh-agent"}
		}
	}
	id := flow.action.ID
	alias := m.value(fieldAlias)
	writeHost := flow.form.Checked(fieldHostOn)
	return func() tea.Msg {
		defer plan.Clear()
		if err := plan.Apply(); err != nil {
			return actionDoneMsg{err: err}
		}
		var paths []string
		for _, w := range plan.Writes {
			paths = append(paths, tilde(w.Path))
		}
		summary := "Wrote " + strings.Join(paths, ", ")
		if id == actions.DirSave {
			summary = fmt.Sprintf("Wrote %s into %s", plural(len(plan.Writes), "file"), tilde(plan.Dirs[0].Path))
			if len(plan.Skipped) > 0 {
				summary += fmt.Sprintf(" · skipped %d unsafe", len(plan.Skipped))
			}
		}
		if id == actions.SSHInstall && writeHost {
			summary += " · ssh " + alias
		}
		return actionDoneMsg{summary: summary}
	}
}

func (m vaultModel) finishAction(msg actionDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.action.stage = actionForm
		m.action.err = msg.err.Error()
		return m, nil
	}
	m.action = nil
	m.notice = msg.summary
	return m, nil
}

func (m vaultModel) actionView() string {
	flow := m.action
	if flow.stage == actionConfirm {
		return flow.dialog.View(min(96, m.width-4))
	}
	w, h := pickerSize(m.width, m.height)
	inner := max(1, w-4)
	title := "ACTIONS · " + strings.ToUpper(m.entryLabel(flow.entry))
	var lines []string
	switch flow.stage {
	case actionFolder:
		selected := ansi.Truncate(tilde(flow.picker.SelectedPath()), inner, "…")
		return modalStyle.Width(w - 2).Height(h - 2).MaxWidth(w).MaxHeight(h).Render(strings.Join([]string{
			titleStyle.Render(title + " · FOLDER"), mutedStyle.Render(selected), "", flow.picker.View(), mutedStyle.Render(flow.picker.Hint()),
		}, "\n"))
	case actionMenu:
		lines = append(lines, titleStyle.Render(title), mutedStyle.Render(string(flow.entry.Kind)), "")
		list := flow.menu
		list.SetSize(inner, max(1, h-7))
		lines = append(lines, list.View(true, titleStyle, mutedStyle))
		return modalBox(lines, pageactions.Footer(inner, "↑↓ Move · esc Back", pageactions.Inline("Choose", true)), w, h)
	}
	lines = append(lines, titleStyle.Render(strings.ToUpper(flow.action.Label)), mutedStyle.Render(m.entryLabel(flow.entry)+" · "+string(flow.entry.Kind)), "")
	visible := m.visibleIDs()
	lines = append(lines, flow.form.ViewFocusedWidth(visible[:len(visible)-1], true, inner))
	if flow.action.ID == actions.SSHInstall {
		note := "Only the key and its .pub are written."
		if flow.form.Checked(fieldHostOn) {
			note = "Host name is the server's IP address or domain; ssh <name> then connects with this key only."
		}
		lines = append(lines, "")
		lines = append(lines, wrapped(mutedStyle.Render(note), inner)...)
	}
	if flow.stage == actionRunning {
		lines = append(lines, "", titleStyle.Render("Working…"))
	}
	if flow.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+flow.err), inner)...)
	}
	hints := "↑↓ Field · ↵ Edit · esc Back"
	if flow.form.IsActive() {
		hints = "Type to edit · ↵ Next field · esc Undo"
	}
	return modalBox(lines, pageactions.Footer(inner, hints, pageactions.Inline("Continue", flow.form.FocusedID() == fieldRun)), w, h)
}

func (m vaultModel) actionStatus() (string, string) {
	switch m.action.stage {
	case actionMenu:
		return "ACTIONS", "↑↓ Move  ↵ Choose  esc Back"
	case actionFolder:
		return "ACTIONS · FOLDER", m.action.picker.Hint()
	case actionForm:
		return "ACTION · " + strings.ToUpper(string(m.action.action.ID)), ""
	}
	return "ACTION", ""
}
