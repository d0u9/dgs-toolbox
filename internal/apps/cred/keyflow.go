package cred

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/keygen"
	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/pageactions"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type keyAction int

const (
	actionGenerate keyAction = iota
	actionImport
	actionRegister
	actionAdd
)

type keyStage int

const (
	keySource keyStage = iota
	keyForm
	keyConfirm
	keyRunning
)

// IdentitySuffix names a new identity file. It is not .txt, which reads like a
// note that is safe to throw away.
const IdentitySuffix = ".agekey"

const (
	hostFieldID        = "host"
	fileFieldID        = "file"
	descriptionFieldID = "description"
	publicKeyFieldID   = "public_key"
	continueFieldID    = "continue"
)

// keyFlow is generating, importing or registering a key, from the form to the
// result. It lives on the keys page as an overlay.
type keyFlow struct {
	action keyAction
	stage  keyStage
	// source is the file being imported; identity the one being registered.
	source   string
	identity identities.Identity

	picker fileexplorer.Model
	form   form.Model
	// defaults are the file name and description last filled in for the host,
	// so they follow the host while they have not been edited.
	defaultFile        string
	defaultDescription string
	dialog             confirm.Model
	err                string
}

type keyDoneMsg struct {
	written  keygen.Written
	host     string
	hostFile string
	err      error
}

func (a keyAction) verb() string {
	return [...]string{"Generate", "Import", "Register", "Add"}[a]
}

func (m *keysModel) startKeyFlow(action keyAction) tea.Cmd {
	m.notice = ""
	switch {
	case m.snap.settingsErr != nil || !m.snap.found:
		m.notice = "! credentials.json is needed first"
		return nil
	case m.snap.settings.Recipients == "":
		m.notice = "! Set recipients in credentials.json first"
		return nil
	case m.snap.folderErr != nil:
		m.notice = "! " + m.snap.folderErr.Error()
		return nil
	}
	flow := &keyFlow{action: action, stage: keyForm}
	switch action {
	case actionRegister:
		identity, ok := m.selectedIdentity()
		if !ok || identity.Public.Key == "" || len(identities.Matches(identity, m.snap.folder)) > 0 {
			m.notice = "! Select an unregistered identity to register it"
			return nil
		}
		flow.identity = identity
	case actionImport:
		flow.stage = keySource
		flow.picker = pickerFor(homeDir(), m.width, m.height, fileexplorer.AllFiles())
	}
	host := m.thisHost()
	if action == actionAdd {
		// Adding is for another machine, most often the host under the cursor.
		host = ""
		if selected, ok := m.selectedHost(); ok {
			host = selected.Name
		}
	}
	fields := []form.Field{{ID: hostFieldID, Kind: form.Text, Label: "Host", Value: host}}
	switch action {
	case actionGenerate, actionImport:
		fields = append(fields, form.Field{ID: fileFieldID, Kind: form.Text, Label: "File"})
	case actionAdd:
		fields = append(fields, form.Field{ID: publicKeyFieldID, Kind: form.Text, Label: "Public key"})
	}
	fields = append(fields,
		form.Field{ID: descriptionFieldID, Kind: form.Text, Label: "Description"},
		form.Field{ID: continueFieldID, Kind: form.Button, Label: action.verb()},
	)
	flow.form = form.New(fields...)
	// The host is asked for every time, so the form opens editing it.
	flow.form.HandleInteraction("enter")
	m.keyFlow = flow
	m.followHost()
	if action == actionImport {
		return flow.picker.Init()
	}
	return nil
}

// thisHost is the host this machine's identities already belong to, when there
// is exactly one.
func (m keysModel) thisHost() string {
	found := map[string]bool{}
	for _, identity := range m.snap.scan.Identities {
		for _, match := range identities.Matches(identity, m.snap.folder) {
			found[match.Host] = true
		}
	}
	if len(found) != 1 {
		return ""
	}
	for host := range found {
		return host
	}
	return ""
}

func (m keysModel) fieldIDs() []string {
	switch m.keyFlow.action {
	case actionRegister:
		return []string{hostFieldID, descriptionFieldID, continueFieldID}
	case actionAdd:
		return []string{hostFieldID, publicKeyFieldID, descriptionFieldID, continueFieldID}
	}
	return []string{hostFieldID, fileFieldID, descriptionFieldID, continueFieldID}
}

// followHost refills the file name from the host while it still holds what was
// last filled in.
func (m *keysModel) followHost() {
	flow := m.keyFlow
	if flow.action != actionGenerate && flow.action != actionImport {
		return
	}
	host := strings.TrimSpace(flow.form.Value(hostFieldID))
	file := "key" + IdentitySuffix
	if host != "" {
		file = host + IdentitySuffix
	}
	if flow.form.Value(fileFieldID) == flow.defaultFile {
		flow.form.SetValue(fileFieldID, file)
		flow.defaultFile = file
	}
}

// meta is what is recorded with the key besides the description: how it came,
// when, and where its private key lives when this machine knows.
func (m keysModel) meta() recipients.Meta {
	flow := m.keyFlow
	_, file, description := m.keyValues()
	meta := recipients.Meta{Description: description, Added: time.Now().Format("2006-01-02")}
	switch flow.action {
	case actionGenerate:
		meta.Origin, meta.PrivateKey = recipients.OriginGenerated, tilde(filepath.Join(m.snap.settings.NewIdentityDir, file))
	case actionImport:
		meta.Origin, meta.PrivateKey = recipients.OriginImported, tilde(filepath.Join(m.snap.settings.NewIdentityDir, file))
	case actionRegister:
		meta.Origin, meta.PrivateKey = recipients.OriginRegistered, identityLabelFull(flow.identity)
	case actionAdd:
		meta.Origin = recipients.OriginAdded
	}
	return meta
}

func identityLabelFull(identity identities.Identity) string {
	label := tilde(identity.Path)
	if identity.Line > 0 {
		label += fmt.Sprintf(":%d", identity.Line)
	}
	return label
}

func (m keysModel) updateKeyFlow(msg tea.Msg) (tea.Model, tea.Cmd) {
	flow := m.keyFlow
	if flow.stage == keySource {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" && !flow.picker.HasDialog() {
			m.keyFlow = nil
			return m, nil
		}
		picker, selected, cmd := flow.picker.Update(msg)
		flow.picker = picker
		if selected == "" {
			return m, cmd
		}
		if info, err := os.Stat(selected); err != nil || info.IsDir() {
			flow.err = "Choose an age identity file, not a folder."
			return m, cmd
		}
		flow.source, flow.err = selected, ""
		flow.stage = keyForm
		return m, cmd
	}
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
		return m.updateKeyForm(key.String())
	case keyConfirm:
		dialog, decision := flow.dialog.Update(key.String())
		flow.dialog = dialog
		switch decision {
		case confirm.Cancelled:
			flow.stage = keyForm
		case confirm.Confirmed:
			flow.stage = keyRunning
			return m, m.runKeyFlow()
		}
	}
	return m, nil
}

func (m keysModel) updateKeyForm(key string) (tea.Model, tea.Cmd) {
	flow := m.keyFlow
	if flow.form.IsActive() {
		// Enter, Tab and the arrows keep the edit and go on to the next or
		// previous field, editing it too, so the form is filled in one pass.
		switch key {
		case "enter", "tab", "down", "shift+tab", "up":
			flow.form.HandleInteraction("enter")
			m.followHost()
			direction := "down"
			if key == "shift+tab" || key == "up" {
				direction = "up"
			}
			ids := m.fieldIDs()
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
		if !flow.form.IsActive() {
			m.followHost()
		}
		return m, nil
	}
	if key == "esc" {
		if flow.action == actionImport {
			flow.stage = keySource
		} else {
			m.keyFlow = nil
		}
		return m, nil
	}
	if flow.form.FocusedID() == continueFieldID && (key == "enter" || key == " ") {
		if err := m.checkKeyForm(); err != nil {
			flow.err = err.Error()
			return m, nil
		}
		flow.err = ""
		flow.dialog = confirm.New(m.keyConfirmConfig())
		flow.stage = keyConfirm
		return m, nil
	}
	if flow.form.HandleInteraction(key) {
		return m, nil
	}
	flow.form.UpdateNavigationWithin(m.fieldIDs(), key)
	return m, nil
}

func (m keysModel) keyValues() (host, file, description string) {
	f := m.keyFlow.form
	return strings.TrimSpace(f.Value(hostFieldID)), strings.TrimSpace(f.Value(fileFieldID)), strings.TrimSpace(f.Value(descriptionFieldID))
}

func (m keysModel) checkKeyForm() error {
	host, file, description := m.keyValues()
	if host == "" {
		return errors.New("Host is required.")
	}
	if err := recipients.CheckHostName(host); err != nil {
		return err
	}
	if description == "" {
		return errors.New("Description is required.")
	}
	switch m.keyFlow.action {
	case actionRegister:
		return nil
	case actionAdd:
		text := strings.TrimSpace(m.keyFlow.form.Value(publicKeyFieldID))
		if text == "" {
			return errors.New("Public key is required.")
		}
		key, err := recipients.ParsePublicKey(text)
		if err != nil {
			return err
		}
		for _, listed := range m.snap.folder.Hosts {
			for _, k := range listed.Keys {
				if k.Key == key.Key {
					return fmt.Errorf("The key is already listed under %s.", listed.Name)
				}
			}
		}
		return nil
	}
	if file == "" || file != filepath.Base(file) || strings.HasPrefix(file, ".") {
		return errors.New("File must be a single file name not starting with a dot.")
	}
	if _, err := os.Lstat(filepath.Join(m.snap.settings.NewIdentityDir, file)); err == nil {
		return fmt.Errorf("%s already exists.", tilde(filepath.Join(m.snap.settings.NewIdentityDir, file)))
	}
	return nil
}

// identityDirListed reports whether the new identity directory is one of the
// directories identities are searched in.
func (m keysModel) identityDirListed() bool {
	want := resolved(m.snap.settings.NewIdentityDir)
	for _, dir := range m.snap.settings.Identities {
		if resolved(dir) == want {
			return true
		}
	}
	return false
}

func resolved(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return filepath.Clean(path)
}

func (m keysModel) keyConfirmConfig() confirm.Config {
	host, file, description := m.keyValues()
	existing := "a new host"
	if found, ok := m.snap.folder.Host(host); ok {
		existing = "the existing host " + found.Name
		host = found.Name
	}
	path := tilde(filepath.Join(m.snap.settings.NewIdentityDir, file))
	config := confirm.Config{Title: strings.ToUpper(m.keyFlow.action.verb()) + " KEY", CancelLabel: "Back", ConfirmLabel: m.keyFlow.action.verb()}
	meta := m.meta()
	notes := []string{fmt.Sprintf("Described as “%s”; recorded as %s on %s.", description, meta.Origin, meta.Added)}
	switch m.keyFlow.action {
	case actionGenerate:
		config.Message = fmt.Sprintf("Write a new age identity to %s and register its public key under %s, %s?", path, host, existing)
		notes = append(notes, "The private key will exist only on this machine.")
	case actionImport:
		config.Message = fmt.Sprintf("Copy %s to %s and register its keys under %s, %s?", tilde(m.keyFlow.source), path, host, existing)
	case actionRegister:
		config.Message = fmt.Sprintf("Register %s under %s, %s?", identityLabelFull(m.keyFlow.identity), host, existing)
	case actionAdd:
		key, _ := recipients.ParsePublicKey(m.keyFlow.form.Value(publicKeyFieldID))
		config.Message = fmt.Sprintf("Add the %s key %s under %s, %s? This machine holds no private key for it.", key.Type, shortKey(key), host, existing)
	}
	if (m.keyFlow.action == actionGenerate || m.keyFlow.action == actionImport) && !m.identityDirListed() {
		notes = append(notes, tilde(m.snap.settings.NewIdentityDir)+" is not in identities in credentials.json, so the key will not be found until it is added.")
	}
	config.Detail = strings.Join(notes, " ")
	return config
}

func (m keysModel) runKeyFlow() tea.Cmd {
	flow := m.keyFlow
	host, file, _ := m.keyValues()
	if found, ok := m.snap.folder.Host(host); ok {
		host = found.Name
	}
	settings := m.snap.settings
	meta := m.meta()
	return func() tea.Msg {
		var written keygen.Written
		var err error
		switch flow.action {
		case actionGenerate:
			written, err = keygen.Generate(settings.NewIdentityDir, file, time.Now())
		case actionImport:
			written, err = keygen.Import(flow.source, settings.NewIdentityDir, file)
		case actionRegister:
			written = keygen.Written{Path: flow.identity.Path, PublicKeys: []string{flow.identity.Public.Key}}
		case actionAdd:
			written = keygen.Written{PublicKeys: []string{strings.TrimSpace(flow.form.Value(publicKeyFieldID))}}
		}
		if err != nil {
			return keyDoneMsg{err: err}
		}
		var hostFile string
		for i, key := range written.PublicKeys {
			keyMeta := meta
			if len(written.PublicKeys) > 1 {
				keyMeta.Description = fmt.Sprintf("%s (%d of %d)", meta.Description, i+1, len(written.PublicKeys))
			}
			if hostFile, err = recipients.AddKey(settings.Recipients, host, key, keyMeta); err != nil {
				if flow.action == actionGenerate || flow.action == actionImport {
					err = fmt.Errorf("%s was written, but registering it failed: %w", tilde(written.Path), err)
				}
				return keyDoneMsg{written: written, host: host, err: err}
			}
		}
		return keyDoneMsg{written: written, host: host, hostFile: hostFile}
	}
}

func (m keysModel) finishKeyFlow(msg keyDoneMsg) (tea.Model, tea.Cmd) {
	flow := m.keyFlow
	if msg.err != nil {
		flow.stage = keyForm
		flow.err = msg.err.Error()
		return m, nil
	}
	m.keyFlow = nil
	switch flow.action {
	case actionRegister, actionAdd:
		m.notice = fmt.Sprintf("Added under %s in %s", msg.host, filepath.ToSlash(msg.hostFile))
	default:
		m.notice = fmt.Sprintf("Wrote %s and registered it under %s · it exists only on this machine, so encrypt important files to another host too", tilde(msg.written.Path), msg.host)
	}
	return m, m.reload()
}

func (m keysModel) keyFlowView() string {
	flow := m.keyFlow
	if flow.stage == keyConfirm {
		return flow.dialog.ViewSize(min(88, m.width-4), m.height)
	}
	w, h := pickerSize(m.width, m.height)
	inner := max(1, w-4)
	title := strings.ToUpper(flow.action.verb()) + " KEY"
	var lines []string
	if flow.stage == keySource {
		selected := ansi.Truncate(tilde(flow.picker.SelectedPath()), inner, "…")
		lines = append(lines, titleStyle.Render(title+" · an age identity file"), mutedStyle.Render(selected), "", flow.picker.View(), mutedStyle.Render(flow.picker.Hint()))
	} else {
		switch flow.action {
		case actionImport:
			lines = append(lines, titleStyle.Render(title), mutedStyle.Render("From "+ansi.Truncate(tilde(flow.source), inner-5, "…")))
		case actionRegister:
			lines = append(lines, titleStyle.Render(title), mutedStyle.Render(identityLabelFull(flow.identity)), mutedStyle.Render(ansi.Truncate(flow.identity.Public.Key, inner, "…")))
		case actionAdd:
			lines = append(lines, titleStyle.Render(title), mutedStyle.Render("A public key from another machine: paste an age1… key or an ssh-ed25519 / ssh-rsa line"))
		default:
			lines = append(lines, titleStyle.Render(title), mutedStyle.Render("A new age X25519 identity in "+tilde(m.snap.settings.NewIdentityDir)))
		}
		ids := m.fieldIDs()
		lines = append(lines, "", flow.form.ViewFocusedWidth(ids[:len(ids)-1], true, inner), "")
		var hosts []string
		for _, host := range m.snap.folder.Hosts {
			hosts = append(hosts, host.Name)
		}
		if len(hosts) > 0 {
			lines = append(lines, wrapped(mutedStyle.Render("Existing hosts: "+strings.Join(hosts, ", ")), inner)...)
		} else {
			lines = append(lines, mutedStyle.Render("No hosts yet; the host is created."))
		}
		if flow.stage == keyRunning {
			lines = append(lines, "", titleStyle.Render("Writing…"))
		}
	}
	if flow.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+flow.err), inner)...)
	}
	if flow.stage == keySource {
		return modalStyle.Width(w - 2).Height(h - 2).MaxWidth(w).MaxHeight(h).Render(strings.Join(lines, "\n"))
	}
	hints := "↑↓ Field · ↵ Edit · esc Back"
	if flow.form.IsActive() {
		hints = "Type to edit · ↵ Next field · esc Undo"
	}
	button := pageactions.Inline(flow.action.verb(), flow.form.FocusedID() == continueFieldID)
	return modalBox(lines, pageactions.Footer(inner, hints, button), w, h)
}

func (m keysModel) keyFlowStatus() tui.Status {
	flow := m.keyFlow
	left := strings.ToUpper(flow.action.verb()) + " KEY"
	switch flow.stage {
	case keySource:
		return tui.Status{Left: left, Center: "IDENTITY FILE", Right: flow.picker.Hint()}
	case keyForm:
		if flow.form.IsActive() {
			return tui.Status{Left: left, Right: "↵ Apply  esc Cancel"}
		}
		return tui.Status{Left: left, Right: "↑↓ Field  ↵ Edit  esc Back"}
	}
	return tui.Status{Left: left}
}
