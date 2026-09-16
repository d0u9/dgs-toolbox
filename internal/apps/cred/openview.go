package cred

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"dgs-toolbox/internal/cred/actions"
	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/opened"
	"dgs-toolbox/internal/cred/record"
	"dgs-toolbox/internal/cred/vault"
	"dgs-toolbox/internal/tui/datafield"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/pageactions"
	"dgs-toolbox/internal/tui/scrolllist"

	"filippo.io/age"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	contentsField = "contents"
	previewField  = "preview"
)

// openFile is the vault file decrypted into memory, and how it is being looked
// at. It is a pointer on the model so that closing clears the one copy.
type openFile struct {
	path      string
	file      vault.File
	opened    *opened.Opened
	record    *record.Record
	list      scrolllist.Model
	collapsed map[string]bool
	reveal    bool
	scroll    int
	lastInput time.Time
	// token tells this opening's idle ticks from an earlier one's.
	token int
}

// passphrasePrompt asks for a passphrase before a file can be opened.
type passphrasePrompt struct {
	file   vault.File
	report vault.Report
	// identity is the protected SSH identity to unlock; nil for a file
	// encrypted with a passphrase.
	identity *identities.Identity
	input    textinput.Model
	err      string
	working  bool
}

type openedMsg struct {
	path   string
	file   vault.File
	opened *opened.Opened
	record *record.Record
	err    error
	wrong  bool
}

type idleTickMsg struct{ token int }

var openTokens int

// startOpen opens the file under the cursor, asking for a passphrase first when
// it needs one.
func (m *vaultModel) startOpen() tea.Cmd {
	file, report, ok := m.selectedFile()
	if !ok {
		return nil
	}
	path := filepath.Join(m.root, filepath.FromSlash(file.Path))
	switch report.Status {
	case vault.Decryptable:
		snap := m.snap
		return func() tea.Msg {
			var ids []age.Identity
			for _, identity := range snap.scan.Identities {
				if identity.Status == identities.Usable {
					if opened, err := identities.Open(identity); err == nil {
						ids = append(ids, opened)
					}
				}
			}
			return openFileMsg(path, file, ids)
		}
	case vault.Passphrase, vault.NeedsPassphrase:
		prompt := &passphrasePrompt{file: file, report: report}
		if report.Status == vault.NeedsPassphrase {
			for i, identity := range m.snap.scan.Identities {
				if identity.Status == identities.Protected && len(report.Protected) > 0 && identityLabelFull(identity) == report.Protected[0] {
					prompt.identity = &m.snap.scan.Identities[i]
				}
			}
			if prompt.identity == nil {
				m.notice = "! The protected identity for this file is no longer found"
				return nil
			}
		}
		prompt.input = textinput.New()
		prompt.input.Prompt = ""
		prompt.input.EchoMode = textinput.EchoPassword
		prompt.input.EchoCharacter = '•'
		prompt.input.Focus()
		m.prompt = prompt
		return textinput.Blink
	case vault.Unchecked:
		m.notice = "Still checking this file"
	default:
		m.notice = "! Not opened: " + report.Status.String()
	}
	return nil
}

func openFileMsg(path string, file vault.File, ids []age.Identity) openedMsg {
	msg := openedMsg{path: path, file: file}
	msg.opened, msg.err = opened.Open(path, ids, opened.DefaultMaxSize)
	if rec, err := record.Read(record.PathFor(path)); err == nil {
		msg.record = &rec
	}
	return msg
}

func (m vaultModel) updatePrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	prompt := m.prompt
	key, ok := msg.(tea.KeyMsg)
	if !ok || prompt.working {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.prompt = nil
		return m, nil
	case "enter":
		passphrase := prompt.input.Value()
		prompt.input.Reset()
		if passphrase == "" {
			prompt.err = "Type the passphrase."
			return m, nil
		}
		prompt.working, prompt.err = true, ""
		path := filepath.Join(m.root, filepath.FromSlash(prompt.file.Path))
		file, identity := prompt.file, prompt.identity
		return m, func() tea.Msg {
			var id age.Identity
			var err error
			if identity != nil {
				secret := []byte(passphrase)
				id, err = identities.Unlock(*identity, secret)
				clear(secret)
			} else {
				id, err = identities.PassphraseIdentity(passphrase)
			}
			if err != nil {
				return openedMsg{path: path, file: file, err: err, wrong: errors.Is(err, identities.ErrWrongPassphrase)}
			}
			msg := openFileMsg(path, file, []age.Identity{id})
			var noMatch *age.NoIdentityMatchError
			if identity == nil && errors.As(msg.err, &noMatch) {
				msg.wrong = true
			}
			return msg
		}
	}
	var cmd tea.Cmd
	prompt.input, cmd = prompt.input.Update(key)
	return m, cmd
}

// finishOpen shows a decrypted file, or says why it did not open.
func (m vaultModel) finishOpen(msg openedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		if m.prompt != nil {
			m.prompt.working = false
			if msg.wrong {
				m.prompt.err = "Wrong passphrase. Try again."
			} else {
				m.prompt.err = msg.err.Error()
			}
			return m, nil
		}
		m.notice = "! Not opened: " + msg.err.Error()
		return m, nil
	}
	m.prompt = nil
	m.closeOpen()
	openTokens++
	open := &openFile{path: msg.path, file: msg.file, opened: msg.opened, record: msg.record, collapsed: map[string]bool{}, lastInput: time.Now(), token: openTokens}
	open.list = scrolllist.New()
	m.open = open
	m.rebuildContents()
	m.rebuild()
	m.fields.Set(contentsField)
	m.layout()
	return m, m.idleTick()
}

func (m vaultModel) idleTick() tea.Cmd {
	if m.open == nil || m.snap.settings.IdleClose() <= 0 {
		return nil
	}
	token := m.open.token
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return idleTickMsg{token: token} })
}

func (m vaultModel) updateIdle(msg idleTickMsg) (tea.Model, tea.Cmd) {
	if m.open == nil || msg.token != m.open.token {
		return m, nil
	}
	if limit := m.snap.settings.IdleClose(); limit > 0 && time.Since(m.open.lastInput) >= limit {
		name := path.Base(m.open.file.Path)
		m.closeOpen()
		m.notice = fmt.Sprintf("Closed %s after %s without input", name, limit)
		return m, nil
	}
	return m, m.idleTick()
}

// closeOpen discards the open file's plaintext.
func (m *vaultModel) closeOpen() {
	if m.open == nil {
		return
	}
	m.open.opened.Close()
	m.open = nil
	m.fields.Set(listField)
	m.rebuild()
}

// Close is called by the shell when the page is left or dgs quits.
func (m vaultModel) Close() {
	if m.open != nil {
		m.open.opened.Close()
	}
}

func (m *vaultModel) rebuildContents() {
	open := m.open
	previous := ""
	if item, ok := open.list.Selected(); ok {
		previous = item.ID
	}
	var items []scrolllist.Item
	hidden := func(p string) bool {
		if open.collapsed[actions.RootPath] {
			return true
		}
		for dir := path.Dir(p); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if open.collapsed[dir] {
				return true
			}
		}
		return false
	}
	// An archive gets a row standing for the whole of it, so it can be saved in
	// one Action rather than a top-level directory at a time.
	indent := 0
	if root, ok := m.rootEntry(); ok {
		marker := "▾ "
		if open.collapsed[actions.RootPath] {
			marker = "▸ "
		}
		_ = root
		items = append(items, scrolllist.Item{ID: rootItemID, Label: marker + open.opened.Name + "/"})
		indent = 1
	}
	for i, entry := range open.opened.Entries {
		if hidden(entry.Path) {
			continue
		}
		depth := strings.Count(entry.Path, "/") + indent
		indent := strings.Repeat("  ", depth)
		name := path.Base(entry.Path)
		var label string
		switch entry.Kind {
		case opened.KindDirectory:
			marker := "▾ "
			if open.collapsed[entry.Path] {
				marker = "▸ "
			}
			label = indent + marker + name + "/"
		case opened.KindUnsafe:
			label = indent + "! " + entry.Path
		default:
			mark := "  "
			if entry.Kind.Sensitive() {
				mark = "⚿ "
			}
			label = indent + mark + name
		}
		items = append(items, scrolllist.Item{ID: strconv.Itoa(i), Label: label})
	}
	open.list.SetItems(items)
	if previous != "" {
		open.list.SelectID(previous)
	}
}

// rootItemID is the CONTENTS row standing for the whole archive.
const rootItemID = "root"

// rootEntry is the directory entry that stands for the whole archive: it is in
// no archive, so it is made here, named after the vault file. A single file has
// none.
func (m vaultModel) rootEntry() (opened.Entry, bool) {
	if m.open == nil || m.open.opened.Archive == opened.ArchiveNone {
		return opened.Entry{}, false
	}
	return opened.Entry{Path: actions.RootPath, Kind: opened.KindDirectory, Mode: fs.ModeDir | 0o755}, true
}

// moveOverEntry is CONTENTS folding, with the File Explorer's keys: l opens a
// directory, o toggles it, h closes it or goes to its parent, and O closes the
// parent and goes there.
func (m *vaultModel) moveOverEntry(key string) {
	entry, ok := m.selectedEntry()
	if !ok {
		return
	}
	open := m.open
	isDir := entry.Kind == opened.KindDirectory
	switch key {
	case "l", "right":
		if isDir {
			open.collapsed[entry.Path] = false
		}
	case "o":
		if isDir {
			open.collapsed[entry.Path] = !open.collapsed[entry.Path]
		}
	case "h", "left":
		if isDir && !open.collapsed[entry.Path] {
			open.collapsed[entry.Path] = true
			break
		}
		m.selectParent(entry.Path)
	case "O":
		parent := m.parentOf(entry.Path)
		if parent == "" {
			return
		}
		open.collapsed[parent] = true
		m.selectParent(entry.Path)
	}
	m.rebuildContents()
	open.scroll, open.reveal = 0, false
}

// parentOf is the path of the entry holding p, or empty for the topmost row.
func (m vaultModel) parentOf(p string) string {
	if p == actions.RootPath {
		return ""
	}
	parent := path.Dir(p)
	if parent == "." || parent == "/" {
		if _, ok := m.rootEntry(); ok {
			return actions.RootPath
		}
		return ""
	}
	return parent
}

func (m *vaultModel) selectParent(p string) {
	parent := m.parentOf(p)
	if parent == "" {
		return
	}
	if parent == actions.RootPath {
		m.open.list.SelectID(rootItemID)
		return
	}
	for i, entry := range m.open.opened.Entries {
		if entry.Path == parent {
			m.open.list.SelectID(strconv.Itoa(i))
			return
		}
	}
}

func (m vaultModel) selectedEntry() (opened.Entry, bool) {
	if m.open == nil {
		return opened.Entry{}, false
	}
	item, ok := m.open.list.Selected()
	if !ok {
		return opened.Entry{}, false
	}
	if item.ID == rootItemID {
		return m.rootEntry()
	}
	index, err := strconv.Atoi(item.ID)
	if err != nil || index >= len(m.open.opened.Entries) {
		return opened.Entry{}, false
	}
	return m.open.opened.Entries[index], true
}

// updateOpenKey handles keys while a file is open. It reports whether the key
// was used.
func (m vaultModel) updateOpenKey(key string) (tea.Model, tea.Cmd, bool) {
	open := m.open
	open.lastInput = time.Now()
	switch key {
	case "tab", "shift+tab":
		order := []string{listField, contentsField, previewField}
		current := 0
		for i, id := range order {
			if id == m.fields.Current() {
				current = i
			}
		}
		step := 1
		if key == "shift+tab" {
			step = len(order) - 1
		}
		m.fields.Set(order[(current+step)%len(order)])
		return m, nil, true
	case "v":
		open.reveal = !open.reveal
		return m, nil, true
	case "c":
		return m, m.copyEntry(), true
	}
	if m.fields.Move(key) {
		return m, nil, true
	}
	switch m.fields.Current() {
	case contentsField:
		switch key {
		case "esc":
			m.closeOpen()
			return m, nil, true
		case "enter":
			m.startAction()
			return m, nil, true
		case "l", "right", "h", "left", "o", "O":
			m.moveOverEntry(key)
			return m, nil, true
		}
		before := open.list.Cursor()
		moveKey(&open.list, key, &m.pendingGG)
		if open.list.Cursor() != before {
			open.scroll, open.reveal = 0, false
		}
		return m, nil, true
	case previewField:
		switch key {
		case "esc":
			m.closeOpen()
		case "down", "j":
			open.scroll++
		case "up", "k":
			open.scroll = max(0, open.scroll-1)
		case "pgdown", "ctrl+d":
			open.scroll += max(1, m.height/2)
		case "pgup", "ctrl+u":
			open.scroll = max(0, open.scroll-max(1, m.height/2))
		case "g", "home":
			open.scroll = 0
		}
		return m, nil, true
	}
	return m, nil, false
}

// maxCopySize bounds what is copied: terminals cap the OSC 52 sequence, and
// several drop anything much larger without a word.
const maxCopySize = 64 << 10

// copyEntry copies the selected entry's text to the clipboard. Private keys are
// never copied: the clipboard is readable by every program and outlives dgs.
func (m *vaultModel) copyEntry() tea.Cmd {
	entry, ok := m.selectedEntry()
	switch {
	case !ok:
		return nil
	case entry.Kind.Sensitive():
		m.notice = "! Private keys are not copied to the clipboard"
		return nil
	case entry.Kind != opened.KindText && entry.Kind != opened.KindSSHPublic:
		m.notice = "! Only text is copied; this is " + string(entry.Kind)
		return nil
	case entry.Size > maxCopySize:
		m.notice = fmt.Sprintf("! %s is larger than %s, more than a terminal clipboard takes", path.Base(entry.Path), humanSize(maxCopySize))
		return nil
	}
	text, copyText, name := string(entry.Data), m.copy, path.Base(entry.Path)
	size := humanSize(entry.Size)
	return func() tea.Msg {
		if err := copyText(text); err != nil {
			return copiedMsg{err: err}
		}
		return copiedMsg{what: fmt.Sprintf("%s (%s) · it stays on the clipboard until replaced", name, size)}
	}
}

// openColumns is the trailing-wide skeleton: vault and contents a quarter each,
// the preview the rest.
func (m vaultModel) openColumns() (int, int, int) {
	available := max(3, m.width-2)
	left := min(90, available/4)
	center := min(90, available/4)
	return left, center, available - left - center
}

func (m *vaultModel) layoutOpen() {
	left, center, right := m.openColumns()
	m.list.SetSize(max(1, left-4), max(1, m.height-2))
	header := len(m.contentsHeader(center - 4))
	m.open.list.SetSize(max(1, center-4), max(1, m.height-2-header))
	m.fields.SetBounds(listField, datafield.Bounds{X: 0, Y: 0, Width: left, Height: m.height})
	m.fields.SetBounds(contentsField, datafield.Bounds{X: left + 1, Y: 0, Width: center, Height: m.height})
	m.fields.SetBounds(previewField, datafield.Bounds{X: left + center + 2, Y: 0, Width: right, Height: m.height})
}

// contentsHeader names who the file was encrypted to, from its record.
func (m vaultModel) contentsHeader(width int) []string {
	open := m.open
	var lines []string
	if open.record == nil {
		lines = append(lines, mutedStyle.Render("No recipient record"))
	} else {
		var names []string
		for _, r := range open.record.Recipients {
			name := r.Host
			if r.Description != "" {
				name += " · " + r.Description
			}
			if name == "" {
				name = shortenText(r.PublicKey)
			}
			names = append(names, name)
		}
		lines = append(lines, wrapped(mutedStyle.Render("For "+strings.Join(names, ", ")), width)...)
	}
	return append(lines, "")
}

func shortenText(text string) string {
	if len(text) > 24 {
		return text[:14] + "…" + text[len(text)-6:]
	}
	return text
}

func (m vaultModel) openView() string {
	left, center, right := m.openColumns()
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.leftColumn(left), " ",
		m.contentsColumn(center), " ",
		m.previewColumn(right))
}

func (m vaultModel) contentsColumn(width int) string {
	inner := width - 4
	focused := m.fields.Current() == contentsField
	list := m.open.list
	header := m.contentsHeader(inner)
	list.SetSize(max(1, inner), max(1, m.height-2-len(header)))
	content := strings.Join(header, "\n") + "\n" + list.View(focused, titleStyle, mutedStyle)
	return fieldset.ViewFocused("CONTENTS · "+strings.ToUpper(m.open.opened.Name), fit(content, m.height-2, inner), width, focused)
}

func (m vaultModel) previewColumn(width int) string {
	inner := width - 4
	focused := m.fields.Current() == previewField
	entry, ok := m.selectedEntry()
	if !ok {
		return fieldset.ViewFocused("PREVIEW", fit("", m.height-2, inner), width, focused)
	}
	shown, named := entry.Path, path.Base(entry.Path)
	if shown == actions.RootPath {
		named = m.open.opened.Name
		shown = named + "/ · the whole file"
	}
	lines := wrapped(property("Path", shown), inner)
	lines = append(lines, property("Type", string(entry.Kind)))
	if entry.Kind != opened.KindDirectory && entry.Kind != opened.KindUnsafe {
		lines = append(lines,
			property("Size", humanSize(entry.Size)),
			property("Mode", entry.Mode.Perm().String()),
		)
		lines = append(lines, wrapped(property("SHA-256", hex.EncodeToString(entry.SHA256[:])), inner)...)
	}
	lines = append(lines, "")
	body := m.previewBody(entry, inner)
	scroll := min(m.open.scroll, max(0, len(body)-1))
	room := max(1, m.height-2-len(lines))
	end := min(len(body), scroll+room)
	lines = append(lines, body[scroll:end]...)
	legend := "PREVIEW · " + strings.ToUpper(named)
	if len(body) > room {
		legend += fmt.Sprintf(" · %d–%d of %d", scroll+1, end, len(body))
	}
	return fieldset.ViewFocused(legend, fit(strings.Join(lines, "\n"), m.height-2, inner), width, focused)
}

func (m vaultModel) previewBody(entry opened.Entry, width int) []string {
	switch entry.Kind {
	case opened.KindDirectory:
		count := 0
		for _, other := range m.open.opened.Entries {
			if path.Dir(other.Path) == entry.Path {
				count++
			}
		}
		return []string{mutedStyle.Render(plural(count, "entry") + " directly inside")}
	case opened.KindUnsafe:
		return wrapped(warnStyle.Render("! Not extracted: "+entry.Unsafe), width)
	case opened.KindOther:
		return []string{mutedStyle.Render("Binary content, not shown.")}
	}
	if entry.Kind.Sensitive() && !m.open.reveal {
		lines := []string{warnStyle.Render("Content hidden · v to show")}
		for _, identity := range identities.Parse(entry.Data) {
			if identity.Public.Key != "" {
				lines = append(lines, "")
				lines = append(lines, wrapped(property("Public", identity.Public.Key), width)...)
				if identity.Public.Fingerprint != "" {
					lines = append(lines, property("SHA256", strings.TrimPrefix(identity.Public.Fingerprint, "SHA256:")))
				}
			} else if identity.Message != "" {
				lines = append(lines, "", mutedStyle.Render(identity.Message))
			}
		}
		return lines
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSuffix(string(entry.Data), "\n"), "\n") {
		lines = append(lines, wrapped(printable(line), width)...)
	}
	return lines
}

// printable keeps a line of decrypted text from driving the terminal: control
// characters, escape sequences among them, are shown as · rather than sent.
func printable(line string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return '·'
		}
		return r
	}, line)
}

func (m vaultModel) promptView() string {
	prompt := m.prompt
	w, h := pickerSize(m.width, m.height)
	w, h = min(w, 72), min(h, 12)
	inner := max(1, w-4)
	lines := []string{titleStyle.Render("PASSPHRASE"), ""}
	if prompt.identity != nil {
		lines = append(lines, wrapped(fmt.Sprintf("%s is encrypted to %s, which is protected. Its passphrase unlocks it for this file only.", path.Base(prompt.file.Path), identityLabelFull(*prompt.identity)), inner)...)
	} else {
		lines = append(lines, wrapped(fmt.Sprintf("%s is encrypted with a passphrase.", path.Base(prompt.file.Path)), inner)...)
	}
	lines = append(lines, "", "› "+prompt.input.View())
	if prompt.working {
		lines = append(lines, "", titleStyle.Render("Decrypting…"))
	}
	if prompt.err != "" {
		lines = append(lines, "")
		lines = append(lines, wrapped(warnStyle.Render("! "+prompt.err), inner)...)
	}
	return modalBox(lines, pageactions.Footer(inner, "↵ Open · esc Cancel", pageactions.Inline("Open", true)), w, h)
}

func (m vaultModel) openStatus() (string, string, string) {
	open := m.open
	center := path.Base(open.file.Path)
	if limit := m.snap.settings.IdleClose(); limit > 0 {
		left := limit - time.Since(open.lastInput)
		center += fmt.Sprintf(" · closes in %d:%02d without input", int(left.Minutes()), int(left.Seconds())%60)
	}
	if m.notice != "" {
		center = m.notice
	}
	switch m.fields.Current() {
	case contentsField:
		return "OPEN · CONTENTS", center, "↑↓ Move  o Fold  ↵ Actions  c Copy  v Show  tab Next  esc Close"
	case previewField:
		return "OPEN · PREVIEW", center, "↑↓ Scroll  c Copy  v Show  tab Next  esc Close"
	}
	return "OPEN · VAULT", center, "↑↓ Move  ↵ Open  tab Next"
}

// CommandPath names the open file in the breadcrumb.
func (m vaultModel) CommandPath() []string {
	if m.open == nil {
		return nil
	}
	return []string{path.Base(m.open.file.Path)}
}

// entryPaths lists the paths of the open file's entries, for tests.
func (m vaultModel) entryPaths() []string {
	var paths []string
	for _, entry := range m.open.opened.Entries {
		paths = append(paths, entry.Path)
	}
	sort.Strings(paths)
	return paths
}

// contentRowsForTest is the labels CONTENTS shows now, folding included.
func (m vaultModel) contentRowsForTest() []string {
	var rows []string
	for i := 0; ; i++ {
		item, ok := m.open.list.ItemAt(i)
		if !ok {
			return rows
		}
		rows = append(rows, item.Label)
	}
}

// startOpenForTest runs the open command for the file under the cursor.
func (m vaultModel) startOpenForTest() tea.Msg {
	return m.startOpen()()
}
