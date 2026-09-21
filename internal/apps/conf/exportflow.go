// This file is the TUI half of exporting: marking nodes, users and instances
// on the inspect page, and the form, confirmation and write that follow `x`.
// The rendering and writing are export.go's, shared with `dgs conf export`;
// only how the instances are chosen differs. See
// docs/apps/conf/export.md#the-page.
package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/target"
	"dgs-toolbox/internal/tui/clipboard"
	tuiconfirm "dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/scrolllist"
	"dgs-toolbox/internal/tui/text"
	"dgs-toolbox/internal/tui/tristate"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The export form's fields and the two formats it offers.
const (
	fieldFormat    = "format"
	fieldDest      = "dest"
	fieldZipName   = "zip-name"
	fieldOverwrite = "overwrite"

	formatShow   = "Show"
	formatFolder = "Folder"
	formatZip    = "Zip"

	// defaultZipName is the archive written when the destination names a
	// directory rather than a .zip file.
	defaultZipName = "conf-export.zip"

	// planRows is how many file paths the confirmation lists before it
	// counts the rest.
	planRows = 12
)

// visibleFields are the form's rows: Show writes nothing, so it has no
// destination to choose.
func (f *exportFlow) visibleFields() []string {
	if f.form.Value(fieldFormat) == formatShow {
		return []string{fieldFormat}
	}
	if f.form.Value(fieldFormat) == formatZip {
		return []string{fieldFormat, fieldDest, fieldZipName, fieldOverwrite}
	}
	return []string{fieldFormat, fieldDest, fieldOverwrite}
}

type exportStage int

const (
	exportForm exportStage = iota
	exportConfirm
	exportRunning
	// exportShow is the rendered files on screen, one at a time, instead of
	// written anywhere.
	exportShow
)

// exportFlow is one export in progress, from the form to the write.
type exportFlow struct {
	stage     exportStage
	instances []string
	form      form.Model
	dialog    tuiconfirm.Model
	// err is why the form could not go on, shown under it.
	err error
	// Fixed once the form is accepted, so the confirmation asks about the
	// write that is carried out.
	zip       bool
	where     string
	overwrite bool

	// owner is the one person every instance is for, when there is one. Only
	// then is Show offered: it is the export a person pastes into a client.
	owner string

	// picking is the File Explorer open over the form, choosing the
	// destination directory.
	picking bool
	picker  fileexplorer.Model

	// files, shown and scroll are the Show stage: every rendered file, the
	// one on screen, and how far down it is scrolled.
	files  []exportFile
	shown  int
	scroll int
	// copied is the outcome of the last c, shown under the file.
	copied string
}

// exportCopiedMsg reports a copy to the clipboard.
type exportCopiedMsg struct {
	what string
	err  error
}

// exportDoneMsg reports the write, successful or not.
type exportDoneMsg struct {
	files int
	where string
	err   error
}

// markedItems draws the checkbox beside each row's name. rows says which instances each row stands for.
func (m InspectModel) markedItems(items []scrolllist.Item, rows [][]string) []scrolllist.Item {
	out := make([]scrolllist.Item, len(items))
	for i, item := range items {
		var instances []string
		if i < len(rows) {
			instances = rows[i]
		}
		box := tristate.Box(len(instances), tristate.Count(instances, m.marked))
		// The box sits beside the name, after the row's indent, branch and
		// fold marker, so it keeps the tree's depth rather than lining up
		// every row in one column.
		name := strings.TrimLeft(item.Label, treePrefix)
		item.Label = item.Label[:len(item.Label)-len(name)] + box + " " + name
		out[i] = item
	}
	return out
}

// treePrefix is every character a row's label may start with before its
// name: indent, tree branches and the fold marker.
const treePrefix = " ▾▸├└─│"

// rowInstances is what the row under the cursor stands for on a tab that
// marks, and false on one that does not.
func (m InspectModel) rowInstances() ([]string, bool) {
	var rows [][]string
	switch m.tab {
	case tabNodes:
		rows = m.nodeRowInstances
	case tabUsers:
		rows = m.userRowInstances
	default:
		return nil, false
	}
	cursor := m.list.Cursor()
	if cursor < 0 || cursor >= len(rows) {
		return nil, false
	}
	return rows[cursor], true
}

// toggleMark marks every instance the row under the cursor stands for, or
// unmarks them all when every one already is — the same rule a tri-state box
// follows everywhere else.
func (m *InspectModel) toggleMark() {
	instances, ok := m.rowInstances()
	if !ok || len(instances) == 0 {
		return
	}
	m.setMarked(instances, tristate.Count(instances, m.marked) < len(instances))
	m.refresh()
}

// toggleMarkAll marks every instance the tab shows, or clears every mark
// when the tab is already fully marked.
func (m *InspectModel) toggleMarkAll() {
	var rows [][]string
	switch m.tab {
	case tabNodes:
		rows = m.nodeRowInstances
	case tabUsers:
		rows = m.userRowInstances
	default:
		return
	}
	// Folded rows still stand for what they hold, so every row together is
	// the whole tab.
	seen := map[string]bool{}
	var all []string
	for _, row := range rows {
		for _, instance := range row {
			if !seen[instance] {
				seen[instance] = true
				all = append(all, instance)
			}
		}
	}
	if tristate.Count(all, m.marked) == len(all) {
		m.marked = map[string]bool{}
	} else {
		m.setMarked(all, true)
	}
	m.refresh()
}

func (m *InspectModel) setMarked(instances []string, marked bool) {
	for _, instance := range instances {
		if marked {
			m.marked[instance] = true
		} else {
			delete(m.marked, instance)
		}
	}
}

// markedCount is how many instances are marked.
func (m InspectModel) markedCount() int { return len(m.marked) }

// exportSelection is what `x` exports: every marked instance, or, with
// nothing marked, whatever the row under the cursor stands for.
func (m InspectModel) exportSelection() []string {
	var instances []string
	if len(m.marked) > 0 {
		for instance := range m.marked {
			instances = append(instances, instance)
		}
	} else if row, ok := m.rowInstances(); ok {
		instances = append(instances, row...)
	} else if item, ok := m.list.Selected(); ok {
		// An instance row on Services is one deployment; export that.
		if kind, id, _ := strings.Cut(item.ID, ":"); kind == "inst" {
			instances = []string{id}
		}
	}
	sort.Strings(instances)
	return instances
}

// startExport opens the export form for the current selection.
func (m *InspectModel) startExport() {
	instances := m.exportSelection()
	if len(instances) == 0 {
		m.notice = "nothing to export here"
		return
	}
	m.notice = ""
	owner := m.singleOwner(instances)
	formats, format := []string{formatFolder, formatZip}, formatFolder
	if owner != "" {
		formats, format = []string{formatShow, formatFolder, formatZip}, formatShow
	}
	m.export = &exportFlow{
		stage:     exportForm,
		instances: instances,
		owner:     owner,
		form: form.New(
			form.Field{ID: fieldFormat, Kind: form.Radio, Label: "Format", Options: formats, Value: format},
			form.Field{ID: fieldDest, Kind: form.Path, Label: "Destination", Value: m.exportDir},
			form.Field{ID: fieldZipName, Kind: form.Text, Label: "ZIP file name", Value: defaultZipName},
			form.Field{ID: fieldOverwrite, Kind: form.Checkbox, Label: "Replace files already there"},
		),
	}
}

// singleOwner is the person every instance is for — the owner of the device
// it hangs off, or the user it was rendered for — or "" when they are for
// more than one, or any is for a machine nobody owns.
func (m InspectModel) singleOwner(instances []string) string {
	ownerOf := map[string]string{}
	for _, n := range m.nodes {
		owner := n.owner
		if n.user {
			owner = n.key
		}
		for _, inst := range n.instances {
			ownerOf[inst.name] = owner
		}
	}
	owner := ""
	for i, instance := range instances {
		o := ownerOf[instance]
		if o == "" || (i > 0 && o != owner) {
			return ""
		}
		owner = o
	}
	return owner
}

// updateExportMsg hands what is not a key to the File Explorer while it is
// open: its directory reads arrive as messages.
func (m InspectModel) updateExportMsg(msg tea.Msg) (InspectModel, tea.Cmd) {
	flow := m.export
	if flow == nil || !flow.picking {
		return m, nil
	}
	picker, _, cmd := flow.picker.Update(msg)
	flow.picker = picker
	return m, cmd
}

// updateExport handles a key while the export flow is open.
func (m InspectModel) updateExport(msg tea.KeyMsg) (InspectModel, tea.Cmd) {
	flow := m.export
	key := msg.String()
	if flow.picking {
		if key == "esc" && !flow.picker.HasDialog() {
			flow.picking = false
			return m, nil
		}
		picker, selected, cmd := flow.picker.Update(msg)
		flow.picker = picker
		if selected != "" {
			flow.form.SetValue(fieldDest, selected)
			flow.picking = false
			flow.err = nil
		}
		return m, cmd
	}
	switch flow.stage {
	case exportRunning:
		return m, nil
	case exportShow:
		return m.updateShow(key)
	case exportConfirm:
		var decision tuiconfirm.Decision
		flow.dialog, decision = flow.dialog.Update(key)
		switch decision {
		case tuiconfirm.Cancelled:
			flow.stage = exportForm
		case tuiconfirm.Confirmed:
			flow.stage = exportRunning
			return m, m.runExport(*flow)
		}
		return m, nil
	}

	if !flow.form.IsActive() {
		switch key {
		case "esc":
			m.export = nil
			return m, nil
		case form.PrimaryActionKey:
			if flow.form.Value(fieldFormat) == formatShow {
				m.showExport()
			} else {
				m.planExport()
			}
			return m, nil
		case "enter", " ":
			if flow.form.Focused(fieldDest) {
				return m, m.openPicker()
			}
		}
	}
	if !flow.form.HandleInteraction(key) {
		flow.form.UpdateNavigationWithin(flow.visibleFields(), key)
	}
	flow.err = nil
	return m, nil
}

// openPicker opens the File Explorer on the destination, or on the directory
// a .zip destination sits in.
func (m InspectModel) openPicker() tea.Cmd {
	flow := m.export
	start := expandHome(strings.TrimSpace(flow.form.Value(fieldDest)))
	if strings.EqualFold(filepath.Ext(start), ".zip") {
		start = filepath.Dir(start)
	}
	if info, err := os.Stat(start); err != nil || !info.IsDir() {
		start = expandHome(m.exportDir)
	}
	w, h := m.pickerSize()
	flow.picker = fileexplorer.New(start, w-4, h-6, fileexplorer.WithFilter(fileexplorer.Directories()))
	flow.picking = true
	return flow.picker.Init()
}

func (m InspectModel) pickerSize() (width, height int) {
	return max(24, min(90, m.width-4)), max(10, m.height-2)
}

// showExport renders every instance and puts the first on screen. Nothing is
// written; the files exist only in memory, and on screen.
func (m *InspectModel) showExport() {
	flow := m.export
	if err := m.brokenIn(flow.instances); err != nil {
		flow.err = err
		return
	}
	files, err := m.renderer().renderAll(flow.instances)
	if err != nil {
		flow.err = err
		return
	}
	flow.files, flow.shown, flow.scroll, flow.copied = files, 0, 0, ""
	flow.stage = exportShow
}

// updateShow is the keys of the Show stage: switch file, scroll, copy.
func (m InspectModel) updateShow(key string) (InspectModel, tea.Cmd) {
	flow := m.export
	switch key {
	case "esc":
		flow.stage = exportForm
		flow.files = nil
	case "tab", "right", "l":
		flow.shown = (flow.shown + 1) % len(flow.files)
		flow.scroll, flow.copied = 0, ""
	case "shift+tab", "left", "h":
		flow.shown = (flow.shown - 1 + len(flow.files)) % len(flow.files)
		flow.scroll, flow.copied = 0, ""
	case "down", "j":
		flow.scroll++
	case "up", "k":
		flow.scroll = max(0, flow.scroll-1)
	case "c":
		f := flow.files[flow.shown]
		if len(f.Bytes) > clipboard.MaxSize {
			flow.copied = fmt.Sprintf("! %s is larger than %s, more than a terminal clipboard takes", f.Path, plural(clipboard.MaxSize, "byte"))
			return m, nil
		}
		copyText, text, what := m.copy, string(f.Bytes), filepath.Base(f.Path)
		return m, func() tea.Msg {
			if err := copyText(text); err != nil {
				return exportCopiedMsg{err: err}
			}
			return exportCopiedMsg{what: what}
		}
	}
	return m, nil
}

// finishCopy shows the outcome of a copy under the file.
func (m *InspectModel) finishCopy(msg exportCopiedMsg) {
	if m.export == nil {
		return
	}
	if msg.err != nil {
		m.export.copied = "! Copy failed: " + msg.err.Error()
		return
	}
	m.export.copied = "copied " + msg.what + " · it stays on the clipboard until replaced"
}

// planExport renders everything the form asks for and, when that works,
// moves on to the confirmation naming each file. Nothing is written here.
func (m *InspectModel) planExport() {
	flow := m.export
	dest := strings.TrimSpace(flow.form.Value(fieldDest))
	if dest == "" {
		flow.err = fmt.Errorf("give a destination")
		return
	}
	if err := m.brokenIn(flow.instances); err != nil {
		flow.err = err
		return
	}
	flow.zip = flow.form.Value(fieldFormat) == formatZip
	flow.overwrite = flow.form.Checked(fieldOverwrite)
	flow.where = expandHome(dest)
	if flow.zip {
		name, err := zipFileName(flow.form.Value(fieldZipName))
		if err != nil {
			flow.err = err
			return
		}
		flow.where = exportDestination(flow.where, name)
	}

	files, err := m.renderer().renderAll(flow.instances)
	if err != nil {
		flow.err = err
		return
	}
	var existing []string
	if flow.zip {
		if _, err := os.Lstat(flow.where); err == nil {
			existing = []string{flow.where}
		}
	} else {
		existing = existingOf(files, flow.where)
	}
	if len(existing) > 0 && !flow.overwrite {
		flow.err = fmt.Errorf("%s already %s; tick Replace to overwrite %s",
			plural(len(existing), "file"), exists(len(existing)), them(len(existing)))
		return
	}

	flow.dialog = tuiconfirm.New(tuiconfirm.Config{
		Title:        "EXPORT",
		Message:      fmt.Sprintf("Write %s to %s?", plural(len(files), "file"), flow.where),
		Detail:       exportPlan(files, existing, flow.zip) + "\n\nThese are plaintext, with every credential in them.",
		ConfirmLabel: "Export",
		CancelLabel:  "Back",
	})
	flow.stage = exportConfirm
}

// brokenIn refuses a selection holding a target that cannot be rendered,
// naming each one, as the command line does.
func (m InspectModel) brokenIn(instances []string) error {
	want := map[string]bool{}
	for _, instance := range instances {
		want[instance] = true
	}
	var broken []string
	for _, t := range target.List(m.l.inv, m.l.derived) {
		if t.Broken != "" && want[t.Instance] {
			broken = append(broken, fmt.Sprintf("%s: %s", t.Instance, t.Broken))
		}
	}
	if len(broken) == 0 {
		return nil
	}
	return fmt.Errorf("%s cannot be rendered: %s", plural(len(broken), "target"), strings.Join(broken, "; "))
}

// exportPlan lists the files an export writes, marking those it replaces.
func exportPlan(files []exportFile, existing []string, zip bool) string {
	replacing := map[string]bool{}
	for _, path := range existing {
		replacing[path] = true
	}
	var lines []string
	for i, f := range files {
		if i == planRows {
			lines = append(lines, fmt.Sprintf("… and %d more", len(files)-planRows))
			break
		}
		suffix := ""
		if replacing[f.Path] {
			suffix = "  (overwrites)"
		}
		lines = append(lines, f.Path+suffix)
	}
	if zip && len(existing) > 0 {
		lines = append(lines, "", "The archive already there is replaced.")
	}
	return strings.Join(lines, "\n")
}

func zipFileName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("give a ZIP file name without a directory path")
	}
	if !strings.EqualFold(filepath.Ext(name), ".zip") {
		name += ".zip"
	}
	return name, nil
}

// exportDestination accepts an existing .zip destination while letting the
// form's file name replace its base name.
func exportDestination(dest, name string) string {
	if strings.EqualFold(filepath.Ext(dest), ".zip") {
		if name == defaultZipName {
			return dest
		}
		return filepath.Join(filepath.Dir(dest), name)
	}
	return filepath.Join(dest, name)
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}

func (m InspectModel) renderer() renderer {
	return renderer{l: m.l, rootPath: m.rootPath, secretsDir: m.secretsDir}
}

// runExport writes the export the confirmation named, off the update loop.
func (m InspectModel) runExport(flow exportFlow) tea.Cmd {
	r := m.renderer()
	return func() tea.Msg {
		var err error
		if flow.zip {
			err = r.ExportZip(flow.instances, flow.where, flow.overwrite)
		} else {
			err = r.ExportFolder(flow.instances, flow.where, flow.overwrite)
		}
		return exportDoneMsg{files: len(flow.instances), where: flow.where, err: err}
	}
}

// finishExport closes the flow on the write's outcome. A successful export
// clears the marks: they were the question, and it is answered.
func (m *InspectModel) finishExport(msg exportDoneMsg) {
	if msg.err != nil {
		if m.export != nil {
			m.export.stage = exportForm
			m.export.err = msg.err
		}
		return
	}
	m.export = nil
	m.marked = map[string]bool{}
	m.notice = fmt.Sprintf("exported %s to %s", plural(msg.files, "file"), msg.where)
	m.refresh()
}

// exportView draws the open export flow over the page.
func (m InspectModel) exportView() string {
	flow := m.export
	switch {
	case flow.picking:
		return m.pickerView()
	case flow.stage == exportConfirm:
		return flow.dialog.ViewSize(min(88, m.width-4), m.height)
	case flow.stage == exportShow:
		return m.showView()
	}
	width := max(24, min(72, m.width-4))
	lines := []string{
		titleStyle.Render("EXPORT") + mutedStyle.Render(" · "+plural(len(flow.instances), "target")),
		"",
		flow.form.ViewFocusedWidth(flow.visibleFields(), true, width-4),
		"",
	}
	switch {
	case flow.stage == exportRunning:
		lines = append(lines, mutedStyle.Render("writing…"))
	case flow.err != nil:
		lines = append(lines, brokenStyle.Render(flow.err.Error()))
	case flow.form.Value(fieldFormat) == formatShow:
		lines = append(lines, mutedStyle.Render("Show puts "+flow.owner+"'s files on screen to read or copy; nothing is written"))
	default:
		lines = append(lines, mutedStyle.Render("Enter on Destination chooses a directory · Zip uses the file name above"))
	}
	return exportFrame.Width(width - 2).Render(strings.Join(lines, "\n"))
}

var exportFrame = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)

// pickerView is the File Explorer choosing the destination.
func (m InspectModel) pickerView() string {
	flow := m.export
	w, h := m.pickerSize()
	inner := max(1, w-4)
	selected := flow.picker.SelectedPath()
	if notice := flow.picker.Notice(); notice != "" {
		selected = notice
	}
	body := strings.Join([]string{
		titleStyle.Render("EXPORT · DESTINATION"),
		mutedStyle.Render(ansi.Truncate(selected, inner, "…")),
		"",
		flow.picker.View(),
		mutedStyle.Render(ansi.Truncate(flow.picker.Hint(), inner, "…")),
	}, "\n")
	return exportFrame.Width(w - 2).MaxHeight(h).Render(body)
}

// showView is one rendered file on screen, with where it sits among the rest.
func (m InspectModel) showView() string {
	flow := m.export
	w, h := m.pickerSize()
	inner := max(1, w-4)
	f := flow.files[flow.shown]
	header := titleStyle.Render(ansi.Truncate(f.Path, max(1, inner-8), "…")) +
		mutedStyle.Render(fmt.Sprintf("  %d/%d", flow.shown+1, len(flow.files)))
	lines := text.Wrapped(strings.TrimRight(string(f.Bytes), "\n"), inner)
	room := max(1, h-8)
	scroll := min(flow.scroll, max(0, len(lines)-room))
	visible := lines[scroll:min(len(lines), scroll+room)]
	footer := mutedStyle.Render("Plaintext, with every credential in it · it stays in this terminal's scrollback")
	if flow.copied != "" {
		footer = flow.copied
	}
	body := strings.Join([]string{
		header,
		"",
		text.Fit(strings.Join(visible, "\n"), room, inner),
		"",
		ansi.Truncate(footer, inner, "…"),
	}, "\n")
	return exportFrame.Width(w - 2).Render(body)
}
