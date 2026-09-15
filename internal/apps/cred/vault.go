package cred

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/vault"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/datafield"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/overlay"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// inspectWorkers is how many headers are read at once.
const inspectWorkers = 4

var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}).
	Background(lipgloss.AdaptiveColor{Light: "#EAF4F4", Dark: "#243447"})

// vaultListedMsg is a folder listed and the keyring its headers are checked
// against. results delivers the reports as they are made.
type vaultListedMsg struct {
	generation  int
	snap        snapshot
	root        string
	listing     vault.Listing
	err         error
	keyProblems []string
	results     <-chan inspected
}

type inspected struct {
	index  int
	report vault.Report
}

type inspectedMsg struct {
	generation int
	inspected
	done bool
}

// vaultModel is dgs cred vault's browsing page.
type vaultModel struct {
	width, height int
	path          string

	loaded bool
	snap   snapshot
	// root is the folder shown; empty until one is configured or chosen.
	root        string
	listing     vault.Listing
	listErr     error
	keyProblems []string
	reports     []vault.Report
	pending     int

	generation int
	cancel     context.CancelFunc
	results    <-chan inspected

	list      scrolllist.Model
	collapsed map[string]bool
	fields    datafield.Navigator
	pendingGG bool
	notice    string

	picking bool
	picker  fileexplorer.Model
	// add is the add flow while it is open.
	add *addFlow
}

func newVaultModel() vaultModel {
	return vaultModel{
		list:      scrolllist.New(),
		collapsed: map[string]bool{},
		fields:    datafield.New(datafield.Field{ID: listField, Row: 0, Col: 0}),
	}
}

func (m vaultModel) Init() tea.Cmd { return m.scan("") }

// scan lists root — or, when root is empty, the configured vault — and starts
// checking headers. A newer scan makes the results of an older one ignored.
func (m *vaultModel) scan(root string) tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	m.generation++
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	generation, credentialsPath := m.generation, m.path
	return func() tea.Msg {
		snap := loadSnapshot(credentialsPath)
		msg := vaultListedMsg{generation: generation, snap: snap, root: root}
		if msg.root == "" {
			msg.root = snap.settings.Vault
		}
		if snap.settingsErr != nil || msg.root == "" {
			return msg
		}
		msg.listing, msg.err = vault.List(msg.root)
		if msg.err != nil {
			return msg
		}
		ring, problems := keyring(snap)
		msg.keyProblems = problems
		msg.results = inspectAll(ctx, msg.root, msg.listing.Files, ring)
		return msg
	}
}

// keyring opens the usable identities and collects the tags of protected SSH
// identities and of the hosts' SSH keys.
func keyring(snap snapshot) (vault.Keyring, []string) {
	var ring vault.Keyring
	var problems []string
	for _, identity := range snap.scan.Identities {
		name := tilde(identity.Path)
		if identity.Line > 0 {
			name += ":" + strconv.Itoa(identity.Line)
		}
		switch identity.Status {
		case identities.Usable:
			opened, err := identities.Open(identity)
			if err != nil {
				problems = append(problems, name+": "+err.Error())
				continue
			}
			ring.Openers = append(ring.Openers, vault.Opener{Name: name, Identity: opened})
		case identities.Protected:
			if tag, ok := vault.TagOf(identity.Public.Key); ok {
				ring.Protected = append(ring.Protected, vault.Tagged{Name: name, Tag: tag})
			}
		}
	}
	for _, host := range snap.folder.Hosts {
		for _, key := range host.Keys {
			if tag, ok := vault.TagOf(key.Key); ok {
				ring.Hosts = append(ring.Hosts, vault.Tagged{Name: host.Name + " · " + key.Description, Tag: tag})
			}
		}
	}
	return ring, problems
}

// inspectAll reads headers inspectWorkers at a time. The channel is buffered
// for every file, so workers never block on a page that has stopped reading.
func inspectAll(ctx context.Context, root string, files []vault.File, ring vault.Keyring) <-chan inspected {
	results := make(chan inspected, len(files))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range inspectWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				results <- inspected{index: index, report: vault.Inspect(filepath.Join(root, filepath.FromSlash(files[index].Path)), ring)}
			}
		}()
	}
	go func() {
		defer func() {
			close(jobs)
			wg.Wait()
			close(results)
		}()
		for index := range files {
			select {
			case <-ctx.Done():
				return
			case jobs <- index:
			}
		}
	}()
	return results
}

func waitInspected(generation int, results <-chan inspected) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-results
		return inspectedMsg{generation: generation, inspected: result, done: !ok}
	}
}

func (m vaultModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		if m.picking {
			w, h := pickerSize(m.width, m.height)
			m.picker.SetSize(w-2, h-6)
		}
		return m, nil
	case vaultListedMsg:
		// Init scans from a copy of the model, so its generation is not stored;
		// only a scan older than the one on screen is ignored.
		if msg.generation < m.generation {
			return m, nil
		}
		m.generation = msg.generation
		m.loaded, m.snap, m.root = true, msg.snap, msg.root
		m.listing, m.listErr, m.keyProblems = msg.listing, msg.err, msg.keyProblems
		m.reports = make([]vault.Report, len(msg.listing.Files))
		m.pending = len(msg.listing.Files)
		m.results = msg.results
		m.rebuild()
		if msg.results == nil {
			return m, nil
		}
		return m, waitInspected(msg.generation, msg.results)
	case inspectedMsg:
		if msg.generation != m.generation || msg.done {
			return m, nil
		}
		if msg.index < len(m.reports) {
			m.reports[msg.index] = msg.report
			m.pending--
		}
		m.rebuild()
		return m, waitInspected(msg.generation, m.results)
	}

	if sealed, ok := msg.(sealedMsg); ok && m.add != nil {
		return m.finishAdd(sealed)
	}
	if m.add != nil {
		return m.updateAdd(msg)
	}
	if m.picking {
		return m.updatePicker(msg)
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.updateKey(msg.String())
	case tea.MouseMsg:
		return m.updateMouse(msg)
	}
	return m, nil
}

func (m vaultModel) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" && !m.picker.HasDialog() {
		m.picking = false
		return m, nil
	}
	picker, selected, cmd := m.picker.Update(msg)
	m.picker = picker
	if selected == "" {
		return m, cmd
	}
	m.picking = false
	m.collapsed = map[string]bool{}
	m.list = scrolllist.New()
	m.layout()
	scan := m.scan(selected)
	return m, tea.Batch(cmd, scan)
}

func (m vaultModel) updateKey(key string) (tea.Model, tea.Cmd) {
	m.notice = ""
	switch key {
	case "R":
		scan := m.scan(m.root)
		return m, scan
	case "a":
		cmd := m.startAdd()
		return m, cmd
	case "o":
		start := m.root
		if start == "" {
			start = "~"
		}
		w, h := pickerSize(m.width, m.height)
		m.picker = fileexplorer.New(expandTilde(start), w-2, h-6, fileexplorer.WithFilter(fileexplorer.Directories()))
		m.picking = true
		return m, m.picker.Init()
	case "enter", "l", "right":
		if dir, ok := m.selectedDir(); ok {
			if key == "enter" {
				m.collapsed[dir] = !m.collapsed[dir]
			} else {
				m.collapsed[dir] = false
			}
			m.rebuild()
			return m, nil
		}
		if key == "enter" {
			if _, _, ok := m.selectedFile(); ok {
				m.notice = "Opening a file is not built yet"
			}
		}
		return m, nil
	case "h", "left":
		if dir, ok := m.selectedDir(); ok && !m.collapsed[dir] {
			m.collapsed[dir] = true
			m.rebuild()
			return m, nil
		}
		// Otherwise go to the containing directory.
		if parent := m.selectedParent(); parent != "" {
			m.list.SelectID("dir:" + parent)
		}
		return m, nil
	}
	moveKey(&m.list, key, &m.pendingGG)
	return m, nil
}

func (m vaultModel) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.fields.HitAt(msg.X, msg.Y) != listField || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.list.Scroll(-3)
	case tea.MouseButtonWheelDown:
		m.list.Scroll(3)
	case tea.MouseButtonLeft:
		m.notice = ""
		m.list.SelectRow(msg.Y - 1)
	}
	return m, nil
}

// rebuild lays the listing out as a tree, directories first, and keeps the
// cursor on the entry it was on.
func (m *vaultModel) rebuild() {
	previous := ""
	if item, ok := m.list.Selected(); ok {
		previous = item.ID
	}
	type node struct {
		dirs  map[string]*node
		files []int
	}
	root := &node{dirs: map[string]*node{}}
	for index, file := range m.listing.Files {
		current := root
		parts := strings.Split(file.Path, "/")
		for _, part := range parts[:len(parts)-1] {
			next, ok := current.dirs[part]
			if !ok {
				next = &node{dirs: map[string]*node{}}
				current.dirs[part] = next
			}
			current = next
		}
		current.files = append(current.files, index)
	}
	var items []scrolllist.Item
	var walk func(n *node, prefix string, depth int)
	walk = func(n *node, prefix string, depth int) {
		indent := strings.Repeat("  ", depth)
		names := make([]string, 0, len(n.dirs))
		for name := range n.dirs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			dir := path.Join(prefix, name)
			marker := "▾ "
			if m.collapsed[dir] {
				marker = "▸ "
			}
			items = append(items, scrolllist.Item{ID: "dir:" + dir, Label: indent + marker + name + "/"})
			if !m.collapsed[dir] {
				walk(n.dirs[name], dir, depth+1)
			}
		}
		for _, index := range n.files {
			label := indent + statusMark(m.reports[index].Status) + " " + path.Base(m.listing.Files[index].Path)
			items = append(items, scrolllist.Item{ID: "file:" + strconv.Itoa(index), Label: label})
		}
	}
	walk(root, "", 0)
	entries := len(items)
	for i, p := range m.problems() {
		items = append(items, scrolllist.Item{ID: "problem:" + strconv.Itoa(i), Label: "! " + p.path})
	}
	m.list.SetItems(items)
	m.list.SetDivider(entries, "PROBLEMS")
	if previous != "" {
		m.list.SelectID(previous)
	}
	m.layout()
}

func (m vaultModel) problems() []problem {
	var found []problem
	for _, p := range m.listing.Problems {
		found = append(found, problem{severity: "warning", path: p.Path, message: p.Message})
	}
	for _, p := range m.keyProblems {
		found = append(found, problem{severity: "warning", path: "identity", message: p})
	}
	return found
}

func statusMark(status vault.Status) string {
	switch status {
	case vault.Decryptable:
		return "✓"
	case vault.NeedsPassphrase, vault.Passphrase:
		return "⚿"
	case vault.NotForThisMachine, vault.Unsupported:
		return "✗"
	case vault.Damaged:
		return "!"
	}
	return "·"
}

func (m vaultModel) selected() (kind string, value string, ok bool) {
	item, found := m.list.Selected()
	if !found {
		return "", "", false
	}
	kind, value, ok = strings.Cut(item.ID, ":")
	return kind, value, ok
}

func (m vaultModel) selectedDir() (string, bool) {
	kind, value, ok := m.selected()
	return value, ok && kind == "dir"
}

func (m vaultModel) selectedFile() (vault.File, vault.Report, bool) {
	kind, value, ok := m.selected()
	index, err := strconv.Atoi(value)
	if !ok || kind != "file" || err != nil || index >= len(m.listing.Files) {
		return vault.File{}, vault.Report{}, false
	}
	return m.listing.Files[index], m.reports[index], true
}

func (m vaultModel) selectedParent() string {
	kind, value, ok := m.selected()
	if !ok {
		return ""
	}
	if kind == "file" {
		if file, _, found := m.selectedFile(); found {
			value = file.Path
		}
	}
	if parent := path.Dir(value); parent != "." {
		return parent
	}
	return ""
}

func (m vaultModel) tooSmall() bool { return m.width < minWidth || m.height < minHeight }

func (m vaultModel) columns() (int, int) {
	left := min(90, (m.width-1)/3)
	return left, m.width - 1 - left
}

func (m *vaultModel) layout() {
	if m.tooSmall() {
		return
	}
	left, _ := m.columns()
	m.list.SetSize(max(1, left-4), max(1, m.height-2))
	m.fields.SetBounds(listField, datafield.Bounds{X: 0, Y: 0, Width: left, Height: m.height})
}

func pickerSize(width, height int) (int, int) {
	return max(20, min(96, width-4)), max(8, min(30, height-2))
}

func (m vaultModel) View() string {
	if m.tooSmall() {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, mutedStyle.Render("Resize terminal for Credentials Vault"))
	}
	left, right := m.columns()
	workspace := lipgloss.JoinHorizontal(lipgloss.Top, m.leftColumn(left), " ", m.rightColumn(right))
	if m.add != nil {
		return overlay.Place(workspace, m.addView(), m.width, m.height)
	}
	if m.picking {
		return overlay.Place(workspace, m.pickerView(), m.width, m.height)
	}
	return workspace
}

func (m vaultModel) message() string {
	switch {
	case !m.loaded:
		return "· Loading…"
	case m.snap.settingsErr != nil:
		return "! " + m.snap.settingsErr.Error()
	case m.root == "":
		return "· No vault folder. Press o to choose one, or set vault in credentials.json."
	case m.listErr != nil:
		return "! " + m.listErr.Error()
	case len(m.listing.Files) == 0 && len(m.problems()) == 0:
		return "· No .age files in " + tilde(m.root)
	}
	return ""
}

func (m vaultModel) leftColumn(width int) string {
	inner := width - 4
	var content string
	if message := m.message(); message != "" {
		content = strings.Join(wrapped(mutedStyle.Render(message), inner), "\n")
	} else {
		content = m.list.View(true, titleStyle, mutedStyle)
	}
	legend := "VAULT"
	if m.root != "" {
		legend += " · " + tilde(m.root)
	}
	return fieldset.ViewFocused(legend, fit(content, m.height-2, inner), width, true)
}

func (m vaultModel) rightColumn(width int) string {
	inner := width - 4
	empty := fieldset.View("DETAIL", fit("", m.height-2, inner), width)
	if m.message() != "" {
		return empty
	}
	kind, value, ok := m.selected()
	if !ok {
		return empty
	}
	var legend string
	var lines []string
	switch kind {
	case "dir":
		count := 0
		for _, file := range m.listing.Files {
			if strings.HasPrefix(file.Path, value+"/") {
				count++
			}
		}
		legend = strings.ToUpper(path.Base(value))
		lines = []string{property("Folder", value), property("Files", strconv.Itoa(count))}
	case "problem":
		index, _ := strconv.Atoi(value)
		p := m.problems()[index]
		legend = "PROBLEM"
		lines = append(wrapped(property("Path", p.path), inner), "")
		lines = append(lines, wrapped(p.message, inner)...)
	case "file":
		file, report, _ := m.selectedFile()
		legend = strings.ToUpper(path.Base(file.Path))
		lines = m.fileDetail(file, report, inner)
	}
	return fieldset.View(legend, fit(strings.Join(lines, "\n"), m.height-2, inner), width)
}

func (m vaultModel) fileDetail(file vault.File, report vault.Report, width int) []string {
	lines := wrapped(property("Path", file.Path), width)
	lines = append(lines,
		property("Size", humanSize(file.Size)),
		property("Modified", file.ModTime.Local().Format("2006-01-02 15:04")),
	)
	status := statusMark(report.Status) + " " + report.Status.String()
	if report.Status == vault.Unchecked {
		status = "· checking…"
	}
	lines = append(lines, "", property("Status", status))
	switch {
	case report.OpenedBy != "":
		lines = append(lines, wrapped(property("Opens", report.OpenedBy), width)...)
	case len(report.Protected) > 0:
		lines = append(lines, wrapped(property("Needs", strings.Join(report.Protected, ", ")+" (passphrase)"), width)...)
	case report.Message != "":
		lines = append(lines, wrapped(warnStyle.Render(property("Problem", report.Message)), width)...)
	}
	if report.Armored {
		lines = append(lines, property("Format", "armored"))
	}
	if len(report.Stanzas) > 0 {
		lines = append(lines, "", titleStyle.Render("Recipients in the header"))
		counts := map[string]int{}
		var order []string
		for _, stanza := range report.Stanzas {
			if counts[stanza.Type] == 0 {
				order = append(order, stanza.Type)
			}
			counts[stanza.Type]++
		}
		for _, kind := range order {
			lines = append(lines, fmt.Sprintf("%s ×%d", kind, counts[kind]))
		}
	}
	if len(report.Hosts) > 0 {
		lines = append(lines, "", titleStyle.Render("Hosts by SSH key"))
		for _, host := range report.Hosts {
			lines = append(lines, host)
		}
		lines = append(lines, wrapped(mutedStyle.Render("SSH keys only, matched by a four-byte tag; X25519 recipients are not shown."), width)...)
	}
	return lines
}

func humanSize(size int64) string {
	switch {
	case size < 1<<10:
		return fmt.Sprintf("%d B", size)
	case size < 1<<20:
		return fmt.Sprintf("%.1f KiB", float64(size)/(1<<10))
	default:
		return fmt.Sprintf("%.1f MiB", float64(size)/(1<<20))
	}
}

func (m vaultModel) pickerView() string {
	w, h := pickerSize(m.width, m.height)
	headerWidth := max(1, w-4)
	header := titleStyle.Render("FILE EXPLORER · VAULT")
	selected := ansi.Truncate(tilde(m.picker.SelectedPath()), headerWidth, "…")
	if notice := m.picker.Notice(); notice != "" {
		selected = ansi.Truncate(notice, headerWidth, "…")
	}
	body := lipgloss.JoinVertical(lipgloss.Left, header, mutedStyle.Render(selected), "", m.picker.View(), mutedStyle.Render(m.picker.Hint()))
	return modalStyle.Width(w - 2).Height(h - 2).MaxWidth(w).MaxHeight(h).Render(body)
}

func (m vaultModel) CapturesShellKey(key string) bool {
	if m.add != nil {
		switch m.add.stage {
		case addSource, addDirectory:
			return key == "esc" || (key == "q" && m.add.picker.CapturesText())
		case addName:
			return key == "esc" || key == "q" || key == "backspace"
		}
		return key == "esc" || key == "q"
	}
	return m.picking && (key == "esc" || (key == "q" && m.picker.CapturesText()))
}

func (m vaultModel) Status() tui.Status {
	if m.add != nil {
		left, right := m.addStatus()
		return tui.Status{Left: left, Center: tilde(m.add.directory), Right: right}
	}
	if m.picking {
		return tui.Status{Left: "BROWSE", Center: "VAULT FOLDER", Right: m.picker.Hint()}
	}
	left := "VAULT"
	if m.pending > 0 {
		left = "VAULT · CHECKING"
	}
	center := m.notice
	if center == "" {
		center = m.summary()
	}
	right := "↑↓ Move  a Add  o Folder  R Rescan"
	if _, ok := m.selectedDir(); ok {
		right = "↑↓ Move  ↵ Fold  a Add  o Folder  R Rescan"
	}
	return tui.Status{Left: left, Center: center, Right: right}
}

func (m vaultModel) summary() string {
	if m.message() != "" {
		return ""
	}
	decryptable := 0
	for _, report := range m.reports {
		if report.Status == vault.Decryptable {
			decryptable++
		}
	}
	parts := []string{plural(len(m.listing.Files), "file"), fmt.Sprintf("%d decryptable", decryptable)}
	if m.pending > 0 {
		parts = append(parts, fmt.Sprintf("%d checking", m.pending))
	}
	return strings.Join(parts, " · ")
}

func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home := homeDir(); home != "" {
			return home + p[1:]
		}
	}
	return p
}
