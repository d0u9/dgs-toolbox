package conf

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/secretstore"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/cred/publish"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/overlay"
	"dgs-toolbox/internal/tui/scrolllist"
	"dgs-toolbox/internal/tui/text"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type migrationRow struct {
	kind, label, source, before, after string
	instance, port, network            string
}

type migrationTable struct {
	l             loaded
	root, secrets string
	planDir       string
	planName      string
	planPath      string
	planDigest    *[32]byte
	dirty         bool
	confirmLeave  bool
	nodes         []inventory.Node
	selectedNode  string
	rows          []migrationRow
	list          scrolllist.Model
	mode          string // plans, nodes, table, report, save, plan-name
	picking       string // open, plan-dir, report-dir
	picker        fileexplorer.Model
	outputDir     string
	form          form.Model
	editRow       int
	report        []byte
	reportScroll  int
	notice        string
}

func newMigrationTable(l loaded, root, secrets string) *migrationTable {
	m := &migrationTable{l: l, root: root, secrets: secrets, planDir: filepath.Join(root, "migrations"), outputDir: filepath.Join(root, "migrations"), list: scrolllist.New(), mode: "plans"}
	m.list.HideNumbers(true)
	for _, node := range l.inv.Nodes {
		if node.Broken == "" {
			m.nodes = append(m.nodes, node)
		}
	}
	m.refreshPlans()
	return m
}

func (m *migrationTable) refreshNodes() {
	items := make([]scrolllist.Item, 0, len(m.nodes))
	for _, node := range m.nodes {
		items = append(items, scrolllist.Item{ID: node.ID, Label: node.ID, Detail: node.Path})
	}
	m.list.SetItems(items)
}

func (m *migrationTable) refreshPlans() {
	names, err := listMigrationPlans(m.planDir)
	if err != nil {
		m.notice = err.Error()
	}
	items := []scrolllist.Item{{ID: "new", Label: "+ New migration", Detail: "Choose a source node"}, {ID: "open", Label: "Open migration file…", Detail: "Browse anywhere"}}
	for _, name := range names {
		items = append(items, scrolllist.Item{ID: "plan:" + name, Label: name, Detail: filepath.Join(m.planDir, name+".yaml")})
	}
	m.list.SetItems(items)
	m.list.First()
}

func (m *migrationTable) editing() bool { return m.form.IsActive() }

func (m *migrationTable) captures(key string) bool {
	if m.picking != "" {
		return key == "esc" || (key == "q" && m.picker.CapturesText())
	}
	if m.editing() {
		return key == "esc" || key == "q"
	}
	return key == "esc" && m.mode != "plans"
}

func (m *migrationTable) openPlan(name string) {
	path, err := migrationPlanPath(m.planDir, name)
	if err != nil {
		m.notice = err.Error()
		return
	}
	m.openPlanPath(path)
}

func (m *migrationTable) openPlanPath(path string) {
	p, digest, err := readMigrationPlan(path)
	if err != nil {
		m.notice = err.Error()
		return
	}
	if err = applyMigrationPlan(m, p); err != nil {
		m.mode = "plans"
		m.refreshPlans()
		m.notice = err.Error()
		return
	}
	m.planName, m.planPath, m.planDigest, m.notice = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), path, &digest, "Opened: "+path
	m.dirty, m.confirmLeave = false, false
}

func (m *migrationTable) savePlan(name string) {
	path, err := migrationPlanPath(m.outputDir, name)
	if err != nil {
		m.notice = err.Error()
		return
	}
	if m.planPath != "" && name == m.planName && filepath.Dir(m.planPath) == m.outputDir {
		path = m.planPath
	}
	var known *[32]byte
	if path == m.planPath {
		known = m.planDigest
	}
	digest, err := writeMigrationPlan(path, migrationPlanFromTable(m), known)
	if err != nil {
		m.notice = err.Error()
		return
	}
	m.planName, m.planPath, m.planDigest, m.notice = name, path, &digest, "Saved: "+path
	m.dirty, m.confirmLeave = false, false
}

func (m *migrationTable) selectNode(id string) {
	if id == m.selectedNode && len(m.rows) != 0 {
		m.mode = "table"
		m.refreshRows()
		m.selectFirstRow()
		return
	}
	var node *inventory.Node
	for i := range m.nodes {
		if m.nodes[i].ID == id {
			node = &m.nodes[i]
			break
		}
	}
	if node == nil {
		return
	}
	m.selectedNode, m.mode, m.report, m.notice = id, "table", nil, ""
	m.dirty, m.confirmLeave = true, false
	m.rows = []migrationRow{{kind: "node", label: "NODE ID", source: node.Path + ": id", before: id, after: id}}
	networks := make([]string, 0, len(node.Networks))
	for name := range node.Networks {
		networks = append(networks, name)
	}
	sort.Strings(networks)
	for _, name := range networks {
		m.rows = append(m.rows,
			migrationRow{kind: "network", label: "NETWORK " + name, source: node.Path + ": networks." + name, before: name, after: name, network: name},
			migrationRow{kind: "address", label: "ADDRESS " + name, source: node.Path + ": networks." + name, before: node.Networks[name], after: node.Networks[name], network: name},
		)
	}
	instances := map[string]bool{}
	var publishedRows []migrationRow
	for _, inst := range node.Instances {
		if inst.Service == "" {
			continue
		}
		instances[inst.ID] = true
		m.rows = append(m.rows, migrationRow{kind: "instance", label: "INSTANCE " + inst.ID, source: inst.Path + ": id", before: inst.ID, after: inst.ID})
		ports := make([]string, 0, len(inst.Ports))
		for name, port := range inst.Ports {
			if port.Published != "" {
				ports = append(ports, name)
			}
		}
		sort.Strings(ports)
		for _, name := range ports {
			publishedRows = append(publishedRows, migrationRow{kind: "published", label: "PUBLISHED " + inst.ID + ":" + name, source: inst.Path + ": ports." + name + ".published", before: inst.Ports[name].Published, after: inst.Ports[name].Published, instance: inst.ID, port: name})
		}
	}
	m.rows = append(m.rows, publishedRows...)
	routes := make([]string, 0, len(m.l.inv.Routes))
	for name, route := range m.l.inv.Routes {
		for _, raw := range route.Hops {
			hop, err := derive.ParseHop(raw)
			if err == nil && instances[hop.Instance] {
				routes = append(routes, name)
				break
			}
		}
	}
	sort.Strings(routes)
	for _, name := range routes {
		m.rows = append(m.rows, migrationRow{kind: "route", label: "ROUTE " + name, source: "routes.yaml: routes." + name, before: name, after: name})
	}
	m.refreshSecretRows()
	m.selectFirstRow()
}

func (m *migrationTable) editableRows() []migrationRow {
	for i, row := range m.rows {
		if row.kind == "secret" {
			return m.rows[:i]
		}
	}
	return m.rows
}

func (m *migrationTable) refreshSecretRows() {
	m.rows = m.editableRows()
	before, after := m.l, m.l
	if m.hasMigrationEdits() {
		cfg := config.Config{}
		cfg.Conf.Root, cfg.Conf.Secrets = m.root, m.secrets
		err := migrateActionSnapshot(nil, io.Discard, []string{"node"}, m.flags(), cfg, func(old, next loaded) bool {
			before, after = old, next
			return true
		})
		if err != nil {
			m.rows = append(m.rows, migrationRow{kind: "secret", label: "SECRET PATHS", before: "preview unavailable", after: err.Error(), source: "Fix the proposed migration, then regenerate the report"})
			m.refreshRows()
			return
		}
	}
	oldPaths := secretstore.ImpliedPaths(before.inv, before.manifests, before.derived)
	newPaths := secretstore.ImpliedPaths(after.inv, after.manifests, after.derived)
	newSet := map[secretstore.Path]bool{}
	for _, path := range newPaths {
		newSet[path] = true
	}
	used := map[secretstore.Path]bool{}
	instanceTo := map[string]string{}
	for _, row := range m.editableRows() {
		if row.kind == "instance" {
			instanceTo[row.before] = row.after
		}
	}
	newNode := m.selectedNode
	for _, row := range m.editableRows() {
		if row.kind == "node" {
			newNode = row.after
			break
		}
	}
	for _, old := range oldPaths {
		candidate := old
		if to := instanceTo[old.Instance]; to != "" {
			candidate.Instance = to
		} else if newNode != m.selectedNode && strings.HasPrefix(old.Instance, m.selectedNode+"-") {
			candidate.Instance = newNode + strings.TrimPrefix(old.Instance, m.selectedNode)
		}
		_, related := instanceTo[old.Instance]
		related = related || strings.HasPrefix(old.Instance, m.selectedNode+"-")
		if newSet[candidate] {
			used[candidate] = true
			if related || candidate != old {
				m.rows = append(m.rows, migrationRow{kind: "secret", label: "SECRET " + old.Instance, before: old.String(), after: candidate.String(), source: "Derived secret path; move manually if changed"})
			}
		} else {
			m.rows = append(m.rows, migrationRow{kind: "secret", label: "SECRET " + old.Instance, before: old.String(), after: "(no implied target)", source: "Derived secret path; review manually"})
		}
	}
	for _, path := range newPaths {
		if used[path] {
			continue
		}
		oldExists := false
		for _, old := range oldPaths {
			if old == path {
				oldExists = true
				break
			}
		}
		if !oldExists {
			m.rows = append(m.rows, migrationRow{kind: "secret", label: "SECRET " + path.Instance, before: "(new)", after: path.String(), source: "Derived secret path; review manually"})
		}
	}
	if len(m.rows) == len(m.editableRows()) {
		m.rows = append(m.rows, migrationRow{kind: "secret", label: "SECRET PATHS", before: "none implied", after: "none implied", source: "No secret path is implied for this migration"})
	}
	m.refreshRows()
}

func (m *migrationTable) hasMigrationEdits() bool {
	for _, row := range m.editableRows() {
		if row.after != row.before {
			return true
		}
	}
	return false
}

func (m *migrationTable) refreshRows() {
	selected := ""
	if item, ok := m.list.Selected(); ok && strings.HasPrefix(item.ID, "row:") {
		selected = item.ID
	}
	items := make([]scrolllist.Item, 0, len(m.rows))
	sections := map[int]string{}
	lastSection := ""
	for i, row := range m.rows {
		section := migrationSection(row.kind)
		if section != lastSection {
			sections[i] = section
			lastSection = section
		}
		items = append(items, scrolllist.Item{ID: fmt.Sprintf("row:%d", i), Label: "  " + row.label, Detail: "    " + row.before + " → " + row.after})
	}
	m.list.SetItems(items)
	m.list.SetDividers(sections)
	if selected != "" {
		m.list.SelectID(selected)
	}
}

func migrationSection(kind string) string {
	switch kind {
	case "node":
		return "NODE"
	case "network", "address":
		return "NETWORKS"
	case "instance":
		return "INSTANCES"
	case "published":
		return "PUBLISHED"
	case "secret":
		return "SECRETS"
	default:
		return "ROUTES"
	}
}

func (m *migrationTable) selectedRow() int {
	index := m.list.Cursor()
	if index < 0 || index >= len(m.rows) {
		return -1
	}
	return index
}

func (m *migrationTable) selectFirstRow() {
	if len(m.rows) > 0 {
		m.list.SelectID("row:0")
	}
}

func (m *migrationTable) moveRow(delta int) {
	m.list.Move(delta)
}

func (m *migrationTable) updateMouse(msg tea.MouseMsg, width, height int) tea.Cmd {
	if m.picking != "" {
		foreground := m.pickerView(width)
		startY := max(0, (height-lipgloss.Height(foreground))/2) + 4
		if msg.Y < startY || msg.Y >= startY+max(6, height-9) {
			return nil
		}
		msg.Y -= startY
		picker, selected, cmd := m.picker.Update(msg)
		m.picker = picker
		if selected != "" {
			m.acceptPicked(selected)
		}
		return cmd
	}
	if m.editing() || msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	left, _ := migrationColumns(width)
	if msg.X <= 0 || msg.X >= left-1 || msg.Y <= 0 || msg.Y >= height-1 {
		return nil
	}
	if !m.list.SelectRow(msg.Y - 1) {
		return nil
	}
	if m.mode == "report" {
		m.mode = "table"
	}
	return nil
}

func (m *migrationTable) openPicker(purpose string, width, height int) tea.Cmd {
	start := m.planDir
	if m.planPath != "" {
		start = filepath.Dir(m.planPath)
	}
	if purpose == "report-dir" && m.outputDir != "" {
		start = m.outputDir
	}
	if info, err := os.Stat(start); err != nil || !info.IsDir() {
		start, _ = os.Getwd()
	}
	filter := fileexplorer.Directories()
	if purpose == "open" {
		filter = fileexplorer.Extensions("yaml", "yml")
	}
	m.picker = fileexplorer.New(start, max(20, min(86, width-8)), max(6, height-9), fileexplorer.WithFilter(filter))
	m.picking = purpose
	return m.picker.Init()
}

func (m *migrationTable) acceptPicked(path string) {
	purpose := m.picking
	m.picking = ""
	switch purpose {
	case "open":
		m.openPlanPath(path)
	case "plan-dir":
		m.outputDir = path
		m.mode = "plan-name"
		m.form = form.New(form.Field{ID: "value", Kind: form.Text, Label: "Plan name", Value: m.planName})
		m.form.HandleInteraction("enter")
	case "report-dir":
		m.outputDir = path
		m.mode = "save"
		m.form = form.New(form.Field{ID: "value", Kind: form.Text, Label: "Report filename", Value: "migration-report.txt"})
		m.form.HandleInteraction("enter")
	}
}

func (m *migrationTable) updateMsg(msg tea.Msg) tea.Cmd {
	if m.picking == "" {
		return nil
	}
	picker, selected, cmd := m.picker.Update(msg)
	m.picker = picker
	if selected != "" {
		m.acceptPicked(selected)
	}
	return cmd
}

func (m *migrationTable) update(msg tea.KeyMsg, width, height int) tea.Cmd {
	key := msg.String()
	if m.picking != "" {
		if key == "esc" && !m.picker.HasDialog() {
			m.picking = ""
			return nil
		}
		return m.updateMsg(msg)
	}
	if m.editing() {
		m.form.HandleInteraction(key)
		if !m.editing() {
			if key == "enter" {
				value := strings.TrimSpace(m.form.Value("value"))
				if m.mode == "save" {
					m.save(filepath.Join(m.outputDir, value))
				} else if m.mode == "plan-name" {
					m.mode = "table"
					m.savePlan(value)
				} else if value != "" && m.editRow >= 0 && m.editRow < len(m.rows) {
					m.rows[m.editRow].after = value
					m.dirty, m.confirmLeave = true, false
					m.report = nil
					m.notice = ""
					m.refreshSecretRows()
				}
			} else if m.mode == "save" || m.mode == "plan-name" {
				if m.mode == "save" {
					m.mode = "report"
				} else {
					m.mode = "table"
				}
			}
		}
		return nil
	}
	if m.mode == "save" {
		m.mode = "report"
	}
	if key != "esc" {
		m.confirmLeave = false
	}
	switch key {
	case "up", "k":
		if m.mode == "report" {
			m.reportScroll = max(0, m.reportScroll-1)
		} else {
			if m.mode == "nodes" || m.mode == "plans" {
				m.list.Move(-1)
			} else {
				m.moveRow(-1)
			}
		}
	case "down", "j":
		if m.mode == "report" {
			m.reportScroll++
		} else {
			if m.mode == "nodes" || m.mode == "plans" {
				m.list.Move(1)
			} else {
				m.moveRow(1)
			}
		}
	case "g", "home":
		if m.mode == "report" {
			m.reportScroll = 0
		} else {
			if m.mode == "nodes" || m.mode == "plans" {
				m.list.First()
			} else {
				m.selectFirstRow()
			}
		}
	case "G", "end":
		if m.mode == "report" {
			m.reportScroll = len(strings.Split(string(m.report), "\n"))
		} else {
			m.list.Last()
			if m.mode != "nodes" && m.mode != "plans" && m.selectedRow() < 0 {
				m.moveRow(-1)
			}
		}
	case "enter":
		if m.mode == "plans" {
			if item, ok := m.list.Selected(); ok {
				if item.ID == "new" {
					m.planName, m.planPath, m.planDigest, m.notice = "", "", nil, ""
					m.selectedNode, m.rows = "", nil
					m.mode = "nodes"
					m.refreshNodes()
				} else if item.ID == "open" {
					return m.openPicker("open", width, height)
				} else if strings.HasPrefix(item.ID, "plan:") {
					m.openPlan(strings.TrimPrefix(item.ID, "plan:"))
				}
			}
		} else if m.mode == "nodes" {
			if item, ok := m.list.Selected(); ok {
				m.selectNode(item.ID)
			}
		} else if m.mode == "table" && len(m.rows) > 0 {
			m.editRow = m.selectedRow()
			if m.editRow < 0 || m.rows[m.editRow].kind == "secret" {
				return nil
			}
			m.form = form.New(form.Field{ID: "value", Kind: form.Text, Label: "New value", Value: m.rows[m.editRow].after})
			m.form.HandleInteraction("enter")
		}
	case "r":
		if m.mode == "table" || m.mode == "report" {
			m.generateReport()
		}
	case "s":
		if m.mode == "table" || m.mode == "report" {
			if m.planPath != "" {
				m.outputDir = filepath.Dir(m.planPath)
				m.savePlan(m.planName)
			} else {
				return m.openPicker("plan-dir", width, height)
			}
		}
	case "S":
		if m.mode == "table" || m.mode == "report" {
			return m.openPicker("plan-dir", width, height)
		}
	case "p":
		if m.mode == "report" {
			return m.openPicker("report-dir", width, height)
		}
	case "esc":
		if m.mode == "report" {
			m.mode = "table"
		} else if m.mode == "table" {
			if m.dirty && !m.confirmLeave {
				m.confirmLeave = true
				m.notice = "Unsaved plan: press s to save or Esc again to discard"
				return nil
			}
			m.dirty, m.confirmLeave = false, false
			m.mode = "plans"
			m.refreshPlans()
			if m.planName != "" {
				m.list.SelectID("plan:" + m.planName)
			}
		} else if m.mode == "nodes" {
			m.mode = "plans"
			m.refreshPlans()
		}
	}
	return nil
}

func (m *migrationTable) generateReport() {
	flags := m.flags()
	cfg := config.Config{}
	cfg.Conf.Root, cfg.Conf.Secrets = m.root, m.secrets
	report, err := buildMigrationReport(flags, cfg)
	if err != nil {
		m.notice = err.Error()
		return
	}
	m.report, m.reportScroll, m.mode, m.notice = report, 0, "report", ""
}

func (m *migrationTable) flags() map[string]string {
	flags := map[string]string{}
	var networks, instances, routes, published []string
	nodeTo := m.selectedNode
	byNetwork := map[string]networkChange{}
	for _, row := range m.rows {
		switch row.kind {
		case "node":
			nodeTo = row.after
		case "network":
			change := byNetwork[row.network]
			change.From, change.To = row.network, row.after
			byNetwork[row.network] = change
		case "address":
			change := byNetwork[row.network]
			change.From = row.network
			if row.after != row.before {
				change.Address = row.after
			}
			byNetwork[row.network] = change
		case "instance":
			if row.after != row.before {
				instances = append(instances, "from="+row.before+",to="+row.after)
			}
		case "route":
			if row.after != row.before {
				routes = append(routes, "from="+row.before+",to="+row.after)
			}
		case "published":
			if row.after != row.before {
				published = append(published, "instance="+row.instance+",port="+row.port+",to="+row.after)
			}
		}
	}
	for _, change := range byNetwork {
		if change.To == "" {
			change.To = change.From
		}
		if change.To != change.From || change.Address != "" {
			spec := "from=" + change.From
			if change.To != change.From {
				spec += ",to=" + change.To
			}
			if change.Address != "" {
				spec += ",address=" + change.Address
			}
			networks = append(networks, spec)
		}
	}
	sort.Strings(networks)
	encNetworks, _ := json.Marshal(networks)
	encInstances, _ := json.Marshal(instances)
	encRoutes, _ := json.Marshal(routes)
	encPublished, _ := json.Marshal(published)
	flags["node"] = "from=" + m.selectedNode + ",to=" + nodeTo
	flags["network"], flags["instance"], flags["route"], flags["published"] = string(encNetworks), string(encInstances), string(encRoutes), string(encPublished)
	return flags
}

func (m *migrationTable) save(path string) {
	m.mode = "report"
	if path == "" {
		return
	}
	if err := publish.Create(path, m.report, 0o600, 0o700); err != nil {
		m.notice = fmt.Sprintf("saving report: %v", err)
		return
	}
	m.notice = "Report saved: " + path
}

func (m *migrationTable) view(width, height int) string {
	if len(m.nodes) == 0 {
		return "No valid node is available for migration."
	}
	left, right := migrationColumns(width)
	contentHeight := max(1, height-2)
	m.list.SetSize(left-4, contentHeight)
	title := "MIGRATION PLANS"
	if m.mode == "nodes" {
		title = "SELECT NODE"
	}
	if m.mode != "nodes" && m.mode != "plans" {
		title = "MIGRATION TABLE"
	}
	leftBox := fieldset.ViewFocused(title, text.Fit(m.list.View(true, titleStyle, mutedStyle), contentHeight, left-4), left, true)
	body, detailTitle := "Choose New migration or Open migration file.\n\nRecent plans: "+m.planDir, "MIGRATION"
	if m.mode == "nodes" {
		body = "Choose a source node with ↑↓ and Enter."
	}
	if m.mode != "nodes" && m.mode != "plans" && len(m.rows) > 0 {
		index := m.selectedRow()
		if index < 0 {
			index = 0
		}
		row := m.rows[index]
		body = "Before:\n    " + row.before + "\nAfter:\n    " + row.after + "\nSource:\n    " + row.source + "\n\nEnter edits. s saves the plan; r builds the report."
		if row.kind == "secret" {
			body = "Before:\n    " + row.before + "\nAfter:\n    " + row.after + "\n\n" + row.source + "\n\nSecret paths are derived and read-only."
		}
		detailTitle = row.label
	}
	if m.editing() {
		body = m.form.ViewFocusedWidth([]string{"value"}, true, right-4) + "\n\nEnter applies; Esc cancels editing."
		detailTitle = "EDIT TARGET"
		if m.mode == "plan-name" {
			detailTitle = "SAVE MIGRATION PLAN"
		}
	} else if m.mode == "report" {
		body = string(m.report)
		detailTitle = "UPGRADE REPORT"
	}
	lines := text.Hanging(body, right-4)
	scroll := 0
	if m.mode == "report" {
		scroll = min(m.reportScroll, max(0, len(lines)-contentHeight))
	}
	rightBox := fieldset.View(detailTitle, text.Fit(strings.Join(lines[scroll:min(len(lines), scroll+contentHeight)], "\n"), contentHeight, right-4), right)
	page := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, " ", rightBox)
	if m.picking != "" {
		return overlay.Place(page, m.pickerView(width), width, height)
	}
	return page
}

func (m *migrationTable) pickerView(width int) string {
	w := max(24, min(90, width-4))
	title := "MIGRATION · OPEN"
	if m.picking != "open" {
		title = "MIGRATION · SAVE DIRECTORY"
	}
	body := title + "\n" + m.picker.SelectedPath() + "\n\n" + m.picker.View() + "\n" + m.picker.Hint()
	return fieldset.View(title, body, w)
}

func migrationColumns(width int) (left, right int) {
	available := max(3, width-1)
	left = min(90, available/3)
	return left, available - left
}

func (m *migrationTable) status() tui.Status {
	center := m.planName
	if center == "" {
		center = m.selectedNode
	}
	if m.dirty {
		center += " * unsaved"
	}
	if m.notice != "" {
		center = m.notice
	}
	right := "↑↓/Click Select  Enter Open  [/] Tabs"
	switch m.mode {
	case "table":
		right = "↑↓/Click Move  Enter Edit  s Save  r Report  Esc Plans"
	case "report":
		right = "↑↓ Scroll  s Save Plan  p Export Report  Esc Table"
	case "save":
		right = "Enter Export  Esc Cancel"
	case "plan-name":
		right = "Enter Save Plan  Esc Cancel"
	}
	if m.editing() && m.mode != "save" {
		right = "Enter Apply  Esc Cancel"
	}
	if m.picking != "" {
		right = m.picker.Hint()
	}
	return tui.Status{Left: "MIGRATE", Center: center, Right: right}
}
