// This file is the TUI half of exporting: marking nodes, users and instances
// on the inspect page, and the form, confirmation and write that follow `x`.
// The rendering and writing are export.go's, shared with `dgs conf export`;
// only how the instances are chosen differs. See
// rhumb docs/export.md#the-page.
package conf

import (
	"fmt"
	rcli "github.com/d0u9/rhumb/cli"
	"github.com/d0u9/rhumb/deploy"
	"github.com/d0u9/rhumb/engine"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/tui/clipboard"
	tuiconfirm "dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/scrolllist"
	"dgs-toolbox/internal/tui/text"
	"dgs-toolbox/internal/tui/tristate"
	"github.com/d0u9/rhumb/target"

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
	fieldDownload  = "download"

	formatShow   = "Show"
	formatFolder = "Folder"
	formatZip    = "Zip"
	// formatBundle writes, per instance, what `rhumb deploy build` makes of
	// its export: the files, compose.yaml and a ctl, ready to copy to the
	// machine and install. The export itself is only a step on the way.
	formatBundle = "Bundle"

	// defaultZipName is the archive written when the destination names a
	// directory rather than a .zip file.
	defaultZipName = "conf-export.zip"

	// The Download field's choices: when a bundle's release is fetched.
	downloadPerNode   = "Per node"
	downloadNow       = "At export"
	downloadOnMachine = "On machine"

	// headerTargets is how many targets the form's header names before it
	// counts the rest.
	headerTargets = 3

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
	if f.form.Value(fieldFormat) == formatBundle {
		return []string{fieldFormat, fieldDest, fieldDownload, fieldOverwrite}
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
	// routes narrows a program marked route by route to those routes.
	routes map[string][]string
	form   form.Model
	dialog tuiconfirm.Model
	// err is why the form could not go on, shown under it.
	err error
	// Fixed once the form is accepted, so the confirmation asks about the
	// write that is carried out.
	zip    bool
	bundle bool
	// bundles are the export directories a Bundle builds, one per instance.
	bundles   []string
	download  string
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
	files  []engine.File
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
	// noun is what files counts, "file" when empty.
	noun  string
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

// markedCount is how many instances are marked, a program counting once
// however many of its routes are.
func (m InspectModel) markedCount() int {
	instances, _ := splitUnits(keys(m.marked))
	return len(instances)
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// splitUnits turns marked rows' keys into the instances to render and, for
// a program marked route by route, the routes to render it with.
func splitUnits(units []string) ([]string, map[string][]string) {
	seen := map[string]bool{}
	var instances []string
	var routes map[string][]string
	for _, u := range units {
		instance, route, byRoute := strings.Cut(u, routeSep)
		if !seen[instance] {
			seen[instance] = true
			instances = append(instances, instance)
		}
		if byRoute {
			if routes == nil {
				routes = map[string][]string{}
			}
			routes[instance] = append(routes[instance], route)
		}
	}
	sort.Strings(instances)
	for _, r := range routes {
		sort.Strings(r)
	}
	return instances, routes
}

// exportSelection is what `x` exports: every marked instance, or, with
// nothing marked, whatever the row under the cursor stands for — and, for a
// program marked route by route, the routes it is rendered with.
func (m InspectModel) exportSelection() ([]string, map[string][]string) {
	var instances []string
	if len(m.marked) > 0 {
		instances = keys(m.marked)
	} else if row, ok := m.rowInstances(); ok {
		instances = append(instances, row...)
	} else if item, ok := m.list.Selected(); ok {
		// An instance row on Services is one deployment; export that.
		if kind, id, _ := strings.Cut(item.ID, ":"); kind == "inst" {
			instances = []string{id}
		}
	}
	return splitUnits(instances)
}

// startExport opens the export form for the current selection.
func (m *InspectModel) startExport() {
	instances, routes := m.exportSelection()
	if len(instances) == 0 {
		m.notice = "nothing to export here"
		return
	}
	m.notice = ""
	owner := m.singleOwner(instances)
	formats, format := []string{formatBundle, formatFolder, formatZip}, formatBundle
	if owner != "" {
		formats, format = []string{formatShow, formatBundle, formatFolder, formatZip}, formatShow
	}
	m.export = &exportFlow{
		stage:     exportForm,
		instances: instances,
		routes:    routes,
		owner:     owner,
		form: form.New(
			form.Field{ID: fieldFormat, Kind: form.Radio, Label: "Format", Options: formats, Value: format},
			form.Field{ID: fieldDest, Kind: form.Path, Label: "Destination", Value: m.exportDir},
			form.Field{ID: fieldZipName, Kind: form.Text, Label: "ZIP file name", Value: defaultZipName},
			form.Field{ID: fieldDownload, Kind: form.Radio, Label: "Download", Options: []string{downloadPerNode, downloadOnMachine, downloadNow}, Value: downloadPerNode},
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
	files, err := m.renderer().RenderAll(flow.instances)
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
	flow.bundle = flow.form.Value(fieldFormat) == formatBundle
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

	files, err := m.renderer().RenderAll(flow.instances)
	if err != nil {
		flow.err = err
		return
	}
	if flow.bundle {
		m.planBundles(files)
		return
	}
	var existing []string
	if flow.zip {
		if _, err := os.Lstat(flow.where); err == nil {
			existing = []string{flow.where}
		}
	} else {
		existing = rcli.ExistingOf(files, flow.where)
	}
	if len(existing) > 0 && !flow.overwrite {
		flow.err = fmt.Errorf("%s already %s; tick Replace to overwrite %s",
			plural(len(existing), "file"), rcli.Exists(len(existing)), rcli.Them(len(existing)))
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

// planBundles moves on to the confirmation naming each bundle: one per
// instance whose export has a manifest, at the same relative path an export
// would write it.
func (m *InspectModel) planBundles(files []engine.File) {
	flow := m.export
	flow.bundles = bundleDirs(files)
	if len(flow.bundles) == 0 {
		flow.err = fmt.Errorf("no instance here has a manifest to build a bundle from")
		return
	}
	var existing []string
	for _, dir := range flow.bundles {
		if _, err := os.Lstat(filepath.Join(flow.where, dir)); err == nil {
			existing = append(existing, dir)
		}
	}
	if len(existing) > 0 && !flow.overwrite {
		flow.err = fmt.Errorf("%s already %s; tick Replace to rebuild %s",
			plural(len(existing), "bundle"), rcli.Exists(len(existing)), rcli.Them(len(existing)))
		return
	}
	// Empty leaves it to each bundle's node, and on the machine where the
	// node does not say.
	switch flow.form.Value(fieldDownload) {
	case downloadNow:
		flow.download = deploy.DownloadBuild
	case downloadOnMachine:
		flow.download = deploy.DownloadInstall
	default:
		flow.download = ""
	}
	described, err := describeBundles(files, m.bundleOptions(flow.download))
	if err != nil {
		flow.err = err
		return
	}
	var lines []string
	replacing := map[string]bool{}
	for _, dir := range existing {
		replacing[dir] = true
	}
	for i, dir := range flow.bundles {
		if i == planRows {
			lines = append(lines, fmt.Sprintf("… and %d more", len(flow.bundles)-planRows))
			break
		}
		suffix := ""
		if replacing[dir] {
			suffix = "  (rebuilds)"
		}
		lines = append(lines, dir+"/"+suffix, "  "+described[dir])
	}
	flow.dialog = tuiconfirm.New(tuiconfirm.Config{
		Title:   "EXPORT",
		Message: fmt.Sprintf("Build %s in %s?", plural(len(flow.bundles), "bundle"), flow.where),
		Detail: strings.Join(lines, "\n") +
			"\n\nEach holds ctl and the rendered files, plaintext, with every credential in them." +
			"\nCopy one to its machine and run ./ctl install there.",
		ConfirmLabel: "Build",
		CancelLabel:  "Back",
	})
	flow.stage = exportConfirm
}

// bundleOptions are the deploy options a bundle built here takes: the
// Download field's answer and conf.services.
func (m InspectModel) bundleOptions(download string) deploy.Options {
	return deploy.Options{Download: download, Services: m.servicesDir}
}

// bundleDirs is the export directory of every instance the files hold a
// manifest for.
func bundleDirs(files []engine.File) []string {
	var dirs []string
	for _, f := range files {
		if path.Base(f.Path) == deploy.ManifestFile {
			dirs = append(dirs, path.Dir(f.Path))
		}
	}
	sort.Strings(dirs)
	return dirs
}

// describeBundles says, per bundle directory, how its instance will run and
// where its program comes from, or refuses naming every bundle that cannot
// be built.
func describeBundles(files []engine.File, opt deploy.Options) (map[string]string, error) {
	out := map[string]string{}
	var bad []string
	for _, f := range files {
		if path.Base(f.Path) != deploy.ManifestFile {
			continue
		}
		dir := path.Dir(f.Path)
		m, err := deploy.ParseManifest(f.Bytes)
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s: %v", dir, err))
			continue
		}
		d := deploy.Describe(m, opt)
		if d.Err != nil {
			bad = append(bad, fmt.Sprintf("%s: %v", dir, d.Err))
			continue
		}
		parts := []string{d.Manager}
		if d.Platform != "" {
			parts = append(parts, d.Platform)
		}
		if d.Binary != "" {
			parts = append(parts, d.Binary)
		}
		out[dir] = strings.Join(parts, " · ")
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("%s cannot be built: %s", plural(len(bad), "bundle"), strings.Join(bad, "; "))
	}
	return out, nil
}

// writeBundles exports the instances into a private temporary directory and
// builds each bundle from there. The temporary export holds every credential
// in plaintext, so it is removed whatever happens.
func writeBundles(r engine.Renderer, instances, dirs []string, where string, opt deploy.Options) error {
	tmp, err := os.MkdirTemp("", "dgs-conf-bundle-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := rcli.ExportFolder(r, instances, tmp, false); err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := deploy.Build(filepath.Join(tmp, dir), filepath.Join(where, dir), opt); err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
	}
	return nil
}

// brokenIn refuses a selection holding a target that cannot be rendered,
// naming each one, as the command line does.
func (m InspectModel) brokenIn(instances []string) error {
	want := map[string]bool{}
	for _, instance := range instances {
		want[instance] = true
	}
	var broken []string
	for _, t := range target.List(m.l.Inv, m.l.Derived) {
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
func exportPlan(files []engine.File, existing []string, zip bool) string {
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

func (m InspectModel) renderer() engine.Renderer {
	r := engine.Renderer{Data: m.l, RootPath: m.rootPath, SecretsDir: m.secretsDir}
	if m.export != nil {
		r.Routes = m.export.routes
	}
	return r
}

// runExport writes the export the confirmation named, off the update loop.
func (m InspectModel) runExport(flow exportFlow) tea.Cmd {
	r := m.renderer()
	return func() tea.Msg {
		var err error
		if flow.bundle {
			err = writeBundles(r, flow.instances, flow.bundles, flow.where, m.bundleOptions(flow.download))
			return exportDoneMsg{files: len(flow.bundles), noun: "bundle", where: flow.where, err: err}
		}
		if flow.zip {
			err = rcli.ExportZip(r, flow.instances, flow.where, flow.overwrite)
		} else {
			err = rcli.ExportFolder(r, flow.instances, flow.where, flow.overwrite)
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
	noun := msg.noun
	if noun == "" {
		noun = "file"
	}
	m.notice = fmt.Sprintf("exported %s to %s", plural(msg.files, noun), msg.where)
	if noun == "bundle" {
		m.notice = fmt.Sprintf("built %s in %s · copy each to its machine and run ./ctl install", plural(msg.files, noun), msg.where)
	}
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
	width := max(24, min(96, m.width-4))
	inner := max(1, width-2-2*exportPadX)
	var fields []string
	for _, id := range flow.visibleFields() {
		fields = append(fields, flow.form.ViewFocusedWidth([]string{id}, true, inner))
	}
	lines := []string{
		titleStyle.Render("EXPORT") + mutedStyle.Render(" · "+plural(len(flow.instances), "target")),
		mutedStyle.Render(ansi.Truncate(targetSummary(flow.instances), inner, "…")),
		"",
		strings.Join(fields, "\n\n"),
		"",
	}
	switch {
	case flow.stage == exportRunning:
		lines = append(lines, mutedStyle.Render("writing…"))
	case flow.err != nil:
		lines = append(lines, brokenStyle.Render(flow.err.Error()))
	case flow.form.Value(fieldFormat) == formatBundle && flow.form.Focused(fieldDownload):
		lines = append(lines, mutedStyle.Render("Per node follows each node's download, else on machine · At export puts the release in the bundle"))
	case flow.form.Value(fieldFormat) == formatBundle:
		lines = append(lines, mutedStyle.Render("Bundle builds, per instance, what ./ctl install deploys on its machine"))
	case flow.form.Value(fieldFormat) == formatZip:
		lines = append(lines, mutedStyle.Render("Zip writes every file into one archive · Enter on Destination chooses its directory"))
	case flow.form.Value(fieldFormat) == formatShow:
		lines = append(lines, mutedStyle.Render("Show puts "+flow.owner+"'s files on screen to read or copy; nothing is written"))
	default:
		lines = append(lines, mutedStyle.Render("Folder writes the rendered files · Enter on Destination chooses a directory"))
	}
	hint := lipgloss.NewStyle().Width(inner).Render(lines[len(lines)-1])
	lines[len(lines)-1] = hint
	return exportFrame.Padding(1, exportPadX).Width(width - 2).Render(strings.Join(lines, "\n"))
}

// exportPadX is the export form's side padding, so the form breathes.
const exportPadX = 3

// targetSummary names the first targets and counts the rest.
func targetSummary(instances []string) string {
	if len(instances) <= headerTargets {
		return strings.Join(instances, ", ")
	}
	return strings.Join(instances[:headerTargets], ", ") + fmt.Sprintf(", +%d more", len(instances)-headerTargets)
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
