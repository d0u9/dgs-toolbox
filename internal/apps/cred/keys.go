package cred

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/datafield"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/scrolllist"

	"github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	tabIdentities = iota
	tabHosts
	tabGroups
	tabCount
)

const (
	listField = "list"
	keysField = "keys"

	minWidth  = 60
	minHeight = 12
	// labelWidth aligns the property names on the right.
	labelWidth = 9
	// fullKeyRows is what the Hosts key pane keeps under its list for the
	// selected key written out whole.
	fullKeyRows = 4
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"})
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"})
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"})
)

type loadedMsg struct{ snap snapshot }

type copiedMsg struct {
	what string
	err  error
}

// problem is a list entry that belongs to no loaded identity, host or group.
type problem struct {
	severity string
	path     string
	message  string
}

// keysModel is dgs cred keys: a read-only view of the identities on this
// machine and the recipient folder.
type keysModel struct {
	width, height int
	// path is the credentials.json to read; empty means the default location.
	path   string
	loaded bool
	snap   snapshot

	tab      int
	lists    [tabCount]scrolllist.Model
	problems [tabCount][]problem
	// messages replaces a tab's list when there is nothing to list, saying why.
	messages [tabCount]string
	keys     scrolllist.Model
	// keysHost is the host the key list was built for, so moving the cursor
	// within the same host keeps the key selection.
	keysHost string

	fields    datafield.Navigator
	pendingGG bool
	notice    string
	copy      func(string) error
}

func newKeysModel() keysModel {
	return keysModel{
		fields: datafield.New(datafield.Field{ID: listField, Row: 0, Col: 0}, datafield.Field{ID: keysField, Row: 0, Col: 1}),
		copy:   copyOSC52,
		keys:   scrolllist.New(),
		lists:  [tabCount]scrolllist.Model{scrolllist.New(), scrolllist.New(), scrolllist.New()},
	}
}

// copyOSC52 asks the terminal to put text on the clipboard. It is written to
// stderr, which is the same terminal, so it does not interleave with the frames
// Bubble Tea writes to stdout.
func copyOSC52(text string) error {
	_, err := osc52.New(text).WriteTo(os.Stderr)
	return err
}

func (m keysModel) Init() tea.Cmd { return m.reload() }

func (m keysModel) reload() tea.Cmd {
	path := m.path
	return func() tea.Msg { return loadedMsg{snap: loadSnapshot(path)} }
}

func (m keysModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil
	case loadedMsg:
		m.snap, m.loaded = msg.snap, true
		m.rebuild()
		return m, nil
	case copiedMsg:
		if msg.err != nil {
			m.notice = "! Copy failed: " + msg.err.Error()
		} else {
			m.notice = "Copied " + msg.what
		}
		return m, nil
	case tui.TabSelectedMsg:
		if msg.Index >= 0 && msg.Index < tabCount {
			m.setTab(msg.Index)
		}
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg.String())
	case tea.MouseMsg:
		return m.updateMouse(msg)
	}
	return m, nil
}

func (m *keysModel) setTab(tab int) {
	m.tab = tab
	m.fields.Set(listField)
	m.pendingGG = false
	m.notice = ""
	m.syncKeys()
}

func (m keysModel) updateKey(key string) (tea.Model, tea.Cmd) {
	m.notice = ""
	switch key {
	case "[":
		m.setTab((m.tab + tabCount - 1) % tabCount)
		return m, nil
	case "]":
		m.setTab((m.tab + 1) % tabCount)
		return m, nil
	case "R":
		return m, m.reload()
	case "c":
		return m, m.copySelected()
	case "tab", "shift+tab":
		if m.tab == tabHosts {
			if m.fields.Current() == listField {
				m.fields.Set(keysField)
			} else {
				m.fields.Set(listField)
			}
		}
		return m, nil
	}
	if m.fields.Move(key) {
		if m.tab != tabHosts {
			m.fields.Set(listField)
		}
		return m, nil
	}
	if m.fields.Current() == keysField {
		moveKey(&m.keys, key, &m.pendingGG)
		return m, nil
	}
	moveKey(&m.lists[m.tab], key, &m.pendingGG)
	m.syncKeys()
	return m, nil
}

func moveKey(list *scrolllist.Model, key string, pendingGG *bool) {
	if key != "g" {
		*pendingGG = false
	}
	switch key {
	case "up", "k":
		list.Move(-1)
	case "down", "j":
		list.Move(1)
	case "left", "h":
		list.Pan(-4)
	case "right", "l":
		list.Pan(4)
	case "G", "end":
		list.Last()
	case "home":
		list.First()
	case "g":
		if *pendingGG {
			list.First()
		}
		*pendingGG = !*pendingGG
	}
}

func (m keysModel) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	hit := m.fields.HitAt(msg.X, msg.Y)
	if hit == keysField && m.tab != tabHosts {
		return m, nil
	}
	list := &m.lists[m.tab]
	top := 0
	if hit == keysField {
		list = &m.keys
		top = m.keysTop()
	}
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		if hit == "" {
			return m, nil
		}
		delta := 3
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -3
		}
		list.Scroll(delta)
	case tea.MouseButtonLeft:
		if hit == "" {
			return m, nil
		}
		m.fields.FocusAt(msg.X, msg.Y)
		m.notice = ""
		if list.SelectRow(msg.Y-top-1) && hit == listField {
			m.syncKeys()
		}
	}
	return m, nil
}

// copySelected copies the public key of the selected identity, or of the key
// under the cursor in a host.
func (m keysModel) copySelected() tea.Cmd {
	text, what := "", ""
	switch m.tab {
	case tabIdentities:
		if identity, ok := m.selectedIdentity(); ok && identity.Public.Key != "" {
			text, what = identity.Public.Key, "public key of "+filepath.Base(identity.Path)
		}
	case tabHosts:
		if key, ok := m.selectedKey(); ok {
			text, what = key.Key, "public key: "+key.Description
		}
	}
	if text == "" {
		return nil
	}
	copyText := m.copy
	return func() tea.Msg { return copiedMsg{what: what, err: copyText(text)} }
}

// rebuild turns a snapshot into the three lists.
func (m *keysModel) rebuild() {
	snap := m.snap
	m.problems = [tabCount][]problem{}
	m.messages = [tabCount]string{}
	// A reload keeps each tab's cursor on the entry it was on.
	var previous [tabCount]string
	for tab := range m.lists {
		if item, ok := m.lists[tab].Selected(); ok {
			previous[tab] = item.ID
		}
	}

	switch {
	case snap.settingsErr != nil:
		for tab := range m.messages {
			m.messages[tab] = "! " + snap.settingsErr.Error()
		}
	case !snap.found:
		for tab := range m.messages {
			m.messages[tab] = "· No credentials.json at " + tilde(snap.path)
		}
	}

	// Identities.
	var items []scrolllist.Item
	for i, identity := range snap.scan.Identities {
		items = append(items, scrolllist.Item{ID: "identity:" + strconv.Itoa(i), Label: identityLabel(identity), Detail: m.identitySummary(identity)})
	}
	for _, p := range snap.scan.Problems {
		m.problems[tabIdentities] = append(m.problems[tabIdentities], problem{severity: "warning", path: tilde(p.Path), message: p.Message})
	}
	if m.messages[tabIdentities] == "" && len(items) == 0 && len(m.problems[tabIdentities]) == 0 {
		if len(snap.settings.Identities) == 0 {
			m.messages[tabIdentities] = "· credentials.json names no identity directories"
		} else {
			m.messages[tabIdentities] = "· No identities found"
		}
	}
	m.setItems(tabIdentities, items)

	// Hosts and groups, and the folder's problems that belong to neither.
	attached := map[string]bool{}
	for _, host := range snap.folder.Hosts {
		attached[host.File] = true
	}
	for _, group := range snap.folder.Groups {
		attached[group.File] = true
	}
	for _, p := range snap.folder.Problems {
		if attached[p.File] {
			continue
		}
		tab := tabHosts
		if strings.HasPrefix(filepath.ToSlash(p.File), recipients.GroupsDir+"/") || filepath.ToSlash(p.File) == recipients.GroupsDir {
			tab = tabGroups
		}
		m.problems[tab] = append(m.problems[tab], problem{severity: p.Severity.String(), path: p.File, message: p.Message})
	}
	folderMessage := ""
	switch {
	case snap.settings.Recipients == "":
		folderMessage = "· credentials.json names no recipient folder"
	case snap.folderErr != nil:
		folderMessage = "! " + snap.folderErr.Error()
	}

	items = nil
	for i, host := range snap.folder.Hosts {
		label := host.Name
		if len(m.warningsFor(host.File)) > 0 {
			label = "! " + label
		}
		items = append(items, scrolllist.Item{ID: "host:" + strconv.Itoa(i), Label: label})
	}
	if m.messages[tabHosts] == "" {
		m.messages[tabHosts] = folderMessage
		if folderMessage == "" && len(items) == 0 && len(m.problems[tabHosts]) == 0 {
			m.messages[tabHosts] = "· No hosts in " + tilde(filepath.Join(snap.folder.Root, recipients.HostsDir))
		}
	}
	m.setItems(tabHosts, items)

	items = nil
	for i, group := range snap.folder.Groups {
		label := group.Name
		if len(m.warningsFor(group.File)) > 0 {
			label = "! " + label
		}
		items = append(items, scrolllist.Item{ID: "group:" + strconv.Itoa(i), Label: label})
	}
	if m.messages[tabGroups] == "" {
		m.messages[tabGroups] = folderMessage
		if folderMessage == "" && len(items) == 0 && len(m.problems[tabGroups]) == 0 {
			m.messages[tabGroups] = "· No groups in " + tilde(filepath.Join(snap.folder.Root, recipients.GroupsDir))
		}
	}
	m.setItems(tabGroups, items)
	for tab := range m.lists {
		m.lists[tab].SelectID(previous[tab])
	}
	m.keysHost = ""
	m.syncKeys()
	m.layout()
}

// setItems sets a tab's entries followed by its problems under a divider.
func (m *keysModel) setItems(tab int, items []scrolllist.Item) {
	if m.messages[tab] != "" {
		items = nil
		m.problems[tab] = nil
	}
	entries := len(items)
	twoRow := tab == tabIdentities
	for i, p := range m.problems[tab] {
		item := scrolllist.Item{ID: "problem:" + strconv.Itoa(i), Label: "! " + filepath.Base(p.path)}
		if twoRow {
			item.Detail = p.message
		}
		items = append(items, item)
	}
	m.lists[tab].SetItems(items)
	m.lists[tab].SetDivider(entries, "PROBLEMS")
}

func (m keysModel) warningsFor(file string) []recipients.Problem {
	var found []recipients.Problem
	for _, p := range m.snap.folder.Problems {
		if p.File == file {
			found = append(found, p)
		}
	}
	return found
}

// identityLabel names an identity in the list by its file name: a full path is
// cut off before the part that tells files apart. The detail shows the path.
func identityLabel(identity identities.Identity) string {
	label := filepath.Base(identity.Path)
	if identity.Line > 0 {
		label += ":" + strconv.Itoa(identity.Line)
	}
	return label
}

func (m keysModel) identitySummary(identity identities.Identity) string {
	parts := []string{identity.Kind, identity.Status.String()}
	if identity.Public.Key != "" {
		if matches := identities.Matches(identity, m.snap.folder); len(matches) > 0 {
			parts = append(parts, matches[0].Host)
		} else {
			parts = append(parts, "unregistered")
		}
	}
	return strings.Join(parts, " · ")
}

// selectedIndex returns the kind and index of the entry under the list cursor.
func (m keysModel) selectedIndex(tab int) (string, int, bool) {
	item, ok := m.lists[tab].Selected()
	if !ok {
		return "", 0, false
	}
	kind, number, found := strings.Cut(item.ID, ":")
	index, err := strconv.Atoi(number)
	return kind, index, found && err == nil
}

func (m keysModel) selectedIdentity() (identities.Identity, bool) {
	kind, index, ok := m.selectedIndex(tabIdentities)
	if !ok || kind != "identity" || index >= len(m.snap.scan.Identities) {
		return identities.Identity{}, false
	}
	return m.snap.scan.Identities[index], true
}

func (m keysModel) selectedHost() (recipients.Host, bool) {
	kind, index, ok := m.selectedIndex(tabHosts)
	if !ok || kind != "host" || index >= len(m.snap.folder.Hosts) {
		return recipients.Host{}, false
	}
	return m.snap.folder.Hosts[index], true
}

func (m keysModel) selectedKey() (recipients.Key, bool) {
	host, ok := m.selectedHost()
	if !ok {
		return recipients.Key{}, false
	}
	cursor := m.keys.Cursor()
	if cursor < 0 || cursor >= len(host.Keys) {
		return recipients.Key{}, false
	}
	return host.Keys[cursor], true
}

// syncKeys rebuilds the Hosts key list when the selected host changes.
func (m *keysModel) syncKeys() {
	host, ok := m.selectedHost()
	if !ok {
		m.keys.SetItems(nil)
		m.keysHost = ""
		if m.fields.Current() == keysField {
			m.fields.Set(listField)
		}
		return
	}
	if host.Name == m.keysHost {
		return
	}
	m.keysHost = host.Name
	items := make([]scrolllist.Item, len(host.Keys))
	for i, key := range host.Keys {
		label := key.Description
		if m.snap.held[key.Key] {
			label += "  ● this machine"
		}
		items[i] = scrolllist.Item{ID: strconv.Itoa(i), Label: label, Detail: string(key.Type) + "  " + keyText(key.PublicKey)}
	}
	m.keys = scrolllist.New()
	m.keys.SetItems(items)
	m.layout()
}

// keyText is how a key is shown in a row: an SSH key's fingerprint, an age
// key whole.
func keyText(key recipients.PublicKey) string {
	if key.Fingerprint != "" {
		return key.Fingerprint
	}
	return key.Key
}

func (m keysModel) columns() (int, int) {
	left := min(90, (m.width-1)/3)
	return left, m.width - 1 - left
}

func (m keysModel) tooSmall() bool { return m.width < minWidth || m.height < minHeight }

// hostHeader is the part of the Hosts right column above the key list.
func (m keysModel) hostHeader(width int) []string {
	host, ok := m.selectedHost()
	if !ok {
		return nil
	}
	var groups []string
	for _, group := range m.snap.folder.Groups {
		for _, member := range group.Hosts {
			if member == host.Name {
				groups = append(groups, group.Name)
			}
		}
	}
	lines := []string{property("File", filepath.ToSlash(host.File)), property("Groups", orNone(strings.Join(groups, ", ")))}
	for _, p := range m.warningsFor(host.File) {
		lines = append(lines, wrapped(warnStyle.Render("! "+p.Message), width)...)
	}
	return lines
}

func (m keysModel) headerHeight() int {
	_, right := m.columns()
	return min(len(m.hostHeader(max(1, right-4)))+2, m.height/2)
}

// keysTop is the row the key pane's fieldset starts on.
func (m keysModel) keysTop() int { return m.headerHeight() }

func (m *keysModel) layout() {
	if m.tooSmall() {
		return
	}
	left, right := m.columns()
	listHeight := max(1, m.height-2)
	for tab := range m.lists {
		m.lists[tab].SetSize(max(1, left-4), listHeight)
	}
	m.fields.SetBounds(listField, datafield.Bounds{X: 0, Y: 0, Width: left, Height: m.height})
	top := m.keysTop()
	m.keys.SetSize(max(1, right-4), max(1, m.height-top-2-fullKeyRows-1))
	m.fields.SetBounds(keysField, datafield.Bounds{X: left + 1, Y: top, Width: right, Height: m.height - top})
}

func (m keysModel) View() string {
	if m.tooSmall() {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, mutedStyle.Render("Resize terminal for Credentials Keys"))
	}
	left, right := m.columns()
	return lipgloss.JoinHorizontal(lipgloss.Top, m.leftColumn(left), " ", m.rightColumn(right))
}

func (m keysModel) leftColumn(width int) string {
	inner := width - 4
	legend := [tabCount]string{"IDENTITIES", "HOSTS", "GROUPS"}[m.tab]
	focused := m.fields.Current() == listField
	var content string
	switch {
	case !m.loaded:
		content = mutedStyle.Render("· Loading…")
	case m.messages[m.tab] != "":
		content = strings.Join(wrapped(mutedStyle.Render(m.messages[m.tab]), inner), "\n")
	default:
		content = m.lists[m.tab].View(focused, titleStyle, mutedStyle)
	}
	return fieldset.ViewFocused(legend, fit(content, m.height-2, inner), width, focused)
}

func (m keysModel) rightColumn(width int) string {
	inner := width - 4
	if !m.loaded || m.messages[m.tab] != "" {
		return fieldset.View("DETAIL", fit("", m.height-2, inner), width)
	}
	kind, index, ok := m.selectedIndex(m.tab)
	if !ok {
		return fieldset.View("DETAIL", fit("", m.height-2, inner), width)
	}
	if kind == "problem" {
		p := m.problems[m.tab][index]
		lines := append([]string{property("Severity", p.severity)}, wrapped(property("Path", p.path), inner)...)
		lines = append(lines, "")
		lines = append(lines, wrapped(p.message, inner)...)
		return fieldset.View("PROBLEM", fit(strings.Join(lines, "\n"), m.height-2, inner), width)
	}
	switch m.tab {
	case tabIdentities:
		identity := m.snap.scan.Identities[index]
		return fieldset.View(strings.ToUpper(filepath.Base(identity.Path)), fit(strings.Join(m.identityDetail(identity, inner), "\n"), m.height-2, inner), width)
	case tabHosts:
		host := m.snap.folder.Hosts[index]
		top := m.headerHeight()
		header := fieldset.View(strings.ToUpper(host.Name), fit(strings.Join(m.hostHeader(inner), "\n"), top-2, inner), width)
		return header + "\n" + m.keysPane(width, m.height-top)
	default:
		group := m.snap.folder.Groups[index]
		return fieldset.View(strings.ToUpper(group.Name), fit(strings.Join(m.groupDetail(group, inner), "\n"), m.height-2, inner), width)
	}
}

func (m keysModel) keysPane(width, height int) string {
	inner := width - 4
	focused := m.fields.Current() == keysField
	listRows := max(1, height-2-fullKeyRows-1)
	lines := []string{fit(m.keys.View(focused, titleStyle, mutedStyle), listRows, inner), ""}
	if key, ok := m.selectedKey(); ok {
		lines = append(lines, wrapped(mutedStyle.Render(key.Key), inner)...)
	}
	return fieldset.ViewFocused("KEYS", fit(strings.Join(lines, "\n"), height-2, inner), width, focused)
}

func (m keysModel) identityDetail(identity identities.Identity, width int) []string {
	path := tilde(identity.Path)
	if identity.Line > 0 {
		path += ":" + strconv.Itoa(identity.Line)
	}
	lines := append(wrapped(property("Path", path), width), property("Type", identity.Kind))
	status := identity.Status.String()
	if identity.Message != "" {
		status += " — " + identity.Message
	}
	lines = append(lines, wrapped(property("Status", status), width)...)
	if identity.Public.Key != "" {
		matches := identities.Matches(identity, m.snap.folder)
		if len(matches) == 0 {
			lines = append(lines, warnStyle.Render(property("Host", "unregistered")))
			lines = append(lines, wrapped(mutedStyle.Render("  Add this public key to "+recipients.HostsDir+"/<name>.json in the recipient folder."), width)...)
		}
		for _, match := range matches {
			lines = append(lines, property("Host", match.Host+" · "+match.Key.Description))
		}
	}
	for _, warning := range identity.Warnings {
		lines = append(lines, wrapped(warnStyle.Render("! "+warning), width)...)
	}
	if identity.Public.Key != "" {
		lines = append(lines, "", titleStyle.Render("Public key"))
		lines = append(lines, wrapped(identity.Public.Key, width)...)
		if identity.Public.Fingerprint != "" {
			lines = append(lines, mutedStyle.Render(identity.Public.Fingerprint))
		}
	}
	return lines
}

func (m keysModel) groupDetail(group recipients.Group, width int) []string {
	lines := []string{property("File", filepath.ToSlash(group.File)), ""}
	for _, p := range m.warningsFor(group.File) {
		lines = append(lines, wrapped(warnStyle.Render("! "+p.Message), width)...)
	}
	lines = append(lines, titleStyle.Render(fmt.Sprintf("Hosts (%d)", len(group.Hosts))))
	if len(group.Hosts) == 0 {
		lines = append(lines, mutedStyle.Render("· No hosts"))
	}
	for _, name := range group.Hosts {
		host, _ := m.snap.folder.Host(name)
		lines = append(lines, fmt.Sprintf("%s  %s", name, mutedStyle.Render(plural(len(host.Keys), "key"))))
	}
	return lines
}

func (m keysModel) Status() tui.Status {
	left := [tabCount]string{"IDENTITIES", "HOSTS", "GROUPS"}[m.tab]
	if !m.loaded {
		return tui.Status{Left: "LOADING"}
	}
	center := m.notice
	if center == "" {
		center = m.summary()
	}
	right := "↑↓ Move  [ ] Tab  R Reload"
	switch {
	case m.tab == tabHosts && m.fields.Current() == keysField:
		left = "KEYS"
		right = "↑↓ Move  c Copy  tab Hosts"
	case m.tab == tabHosts:
		right = "↑↓ Move  tab Keys  [ ] Tab  R Reload"
	case m.tab == tabIdentities:
		if identity, ok := m.selectedIdentity(); ok && identity.Public.Key != "" {
			right = "↑↓ Move  c Copy  [ ] Tab  R Reload"
		}
	}
	return tui.Status{Left: left, Center: center, Right: right}
}

func (m keysModel) summary() string {
	switch m.tab {
	case tabIdentities:
		unregistered := 0
		for _, identity := range m.snap.scan.Identities {
			if identity.Public.Key != "" && len(identities.Matches(identity, m.snap.folder)) == 0 {
				unregistered++
			}
		}
		parts := []string{plural(len(m.snap.scan.Identities), "identity")}
		if unregistered > 0 {
			parts = append(parts, fmt.Sprintf("%d unregistered", unregistered))
		}
		return strings.Join(parts, " · ")
	case tabHosts:
		return m.folderSummary(plural(len(m.snap.folder.Hosts), "host"))
	default:
		return m.folderSummary(plural(len(m.snap.folder.Groups), "group"))
	}
}

func (m keysModel) folderSummary(count string) string {
	errors, warnings := 0, 0
	for _, p := range m.snap.folder.Problems {
		if p.Severity == recipients.Error {
			errors++
		} else {
			warnings++
		}
	}
	parts := []string{count}
	if errors > 0 {
		parts = append(parts, plural(errors, "error"))
	}
	if warnings > 0 {
		parts = append(parts, plural(warnings, "warning"))
	}
	return strings.Join(parts, " · ")
}

func (m keysModel) Tabs() []tui.Tab {
	return []tui.Tab{
		{Label: "Identities", Active: m.tab == tabIdentities},
		{Label: "Hosts", Active: m.tab == tabHosts},
		{Label: "Groups", Active: m.tab == tabGroups},
	}
}

func property(name, value string) string {
	return mutedStyle.Render(fmt.Sprintf("%-*s", labelWidth, name)) + value
}

func orNone(value string) string {
	if value == "" {
		return mutedStyle.Render("none")
	}
	return value
}

func plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	if strings.HasSuffix(noun, "ty") {
		return fmt.Sprintf("%d %sies", count, strings.TrimSuffix(noun, "y"))
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// wrapped breaks text into lines of width cells, breaking inside words when it
// has to, since a public key has no spaces to break at.
func wrapped(text string, width int) []string {
	return strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
}

func fit(content string, height, width int) string {
	lines := strings.Split(content, "\n")
	lines = lines[:min(len(lines), max(1, height))]
	for len(lines) < max(1, height) {
		lines = append(lines, strings.Repeat(" ", max(1, width)))
	}
	return strings.Join(lines, "\n")
}
