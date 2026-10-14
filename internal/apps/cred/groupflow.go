package cred

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/pageactions"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const groupHostsField = "group-hosts"

type groupMember struct {
	name, detail string
	checked      bool
}

type groupFlow struct {
	draft                         *recipients.GroupDraft
	rename                        bool
	form                          form.Model
	members                       []groupMember
	list                          scrolllist.Model
	selecting, running, pendingGG bool
	before                        []groupMember
	err                           string
}

type groupSavedMsg struct {
	name, file string
	err        error
}

func (m keysModel) selectedGroupDraft() (recipients.GroupDraft, error) {
	kind, index, ok := m.selectedIndex(tabGroups)
	if !ok || kind != "group" {
		return recipients.GroupDraft{}, fmt.Errorf("select a group first")
	}
	return recipients.ReadGroup(m.snap.settings.Recipients, m.snap.folder.Groups[index].Name)
}

func (m *keysModel) startGroupFlow(action string) {
	if m.snap.settings.Recipients == "" {
		m.notice = "! Set recipients in cred/config.json first"
		return
	}
	if m.snap.folderErr != nil {
		m.notice = "! " + m.snap.folderErr.Error()
		return
	}
	flow := &groupFlow{rename: action == "r", list: scrolllist.New()}
	if action != "n" {
		draft, err := m.selectedGroupDraft()
		if err != nil {
			m.notice = "! " + err.Error()
			return
		}
		flow.draft = &draft
	}
	fields := []form.Field{}
	if flow.draft == nil || flow.rename {
		name := "g-"
		if flow.draft != nil {
			name = flow.draft.Name
		}
		fields = append(fields, form.Field{ID: nameFieldID, Kind: form.Text, Label: "Name", Value: name})
	}
	if !flow.rename {
		// Start with every raw entry: even duplicate or unresolved names survive
		// unless their own checkbox is explicitly cleared.
		if flow.draft != nil {
			for _, name := range flow.draft.Hosts {
				flow.members = append(flow.members, groupMember{name: name, checked: true})
			}
		}
		for _, host := range m.snap.folder.Hosts {
			if !slices.ContainsFunc(flow.members, func(row groupMember) bool { return strings.EqualFold(row.name, host.Name) }) {
				flow.members = append(flow.members, groupMember{name: host.Name})
			}
		}
		seen := map[string]bool{}
		for i := range flow.members {
			row := &flow.members[i]
			row.detail = "! unavailable: host is missing or has errors"
			if host, ok := m.snap.folder.Host(row.name); ok {
				row.detail = plural(len(host.Keys), "key")
				if len(host.Keys) == 0 {
					row.detail += " · ! no keys"
				}
			}
			if seen[strings.ToLower(row.name)] {
				row.detail += " · ! repeated member"
			}
			seen[strings.ToLower(row.name)] = true
		}
		fields = append(fields, form.Field{ID: groupHostsField, Kind: form.Path, Label: "Hosts"})
	}
	label := "Save"
	if flow.rename {
		label = "Rename"
	}
	fields = append(fields, form.Field{ID: continueFieldID, Kind: form.Button, Label: label})
	flow.form = form.New(fields...)
	flow.refresh()
	if flow.draft == nil || flow.rename {
		flow.form.HandleInteraction("enter")
	}
	m.groupFlow = flow
}

func (f *groupFlow) refresh() {
	items := make([]scrolllist.Item, len(f.members))
	count := 0
	for i, row := range f.members {
		mark := " "
		if row.checked {
			mark = "x"
			count++
		}
		name := row.name
		if recipients.CheckHostName(name) != nil {
			name = strconv.Quote(name)
		}
		items[i] = scrolllist.Item{ID: strconv.Itoa(i), Label: "[" + mark + "] " + name, Detail: row.detail}
	}
	f.list.SetItems(items)
	f.form.SetValue(groupHostsField, plural(count, "host")+" selected")
}

func (f *groupFlow) fields() []string {
	ids := []string{}
	if f.draft == nil || f.rename {
		ids = append(ids, nameFieldID)
	}
	if !f.rename {
		ids = append(ids, groupHostsField)
	}
	return append(ids, continueFieldID)
}

func (f *groupFlow) toggle() {
	if item, ok := f.list.Selected(); ok {
		index, _ := strconv.Atoi(item.ID)
		f.members[index].checked = !f.members[index].checked
		f.refresh()
	}
}

func (m keysModel) updateGroupFlow(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.groupFlow
	if f.running {
		return m, nil
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		return m.groupMouse(mouse)
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	k := key.String()
	if f.selecting {
		switch k {
		case " ":
			f.toggle()
		case "enter":
			f.selecting = false
		case "esc":
			f.members = slices.Clone(f.before)
			f.refresh()
			f.selecting = false
		default:
			moveKey(&f.list, k, &f.pendingGG)
		}
		return m, nil
	}
	if f.form.IsActive() {
		if key.Paste {
			f.form.Paste(string(key.Runes))
		} else {
			f.form.HandleInteraction(k)
		}
		return m, nil
	}
	switch {
	case k == "esc":
		m.groupFlow = nil
	case (k == "enter" || k == " ") && f.form.FocusedID() == groupHostsField:
		f.before = slices.Clone(f.members)
		f.selecting = true
	case (k == "enter" || k == " ") && f.form.FocusedID() == continueFieldID || k == form.PrimaryActionKey:
		return m.saveGroup()
	default:
		if !f.form.HandleInteraction(k) {
			f.form.UpdateNavigationWithin(f.fields(), k)
		}
	}
	return m, nil
}

func (m keysModel) saveGroup() (tea.Model, tea.Cmd) {
	f := m.groupFlow
	name := strings.TrimSpace(f.form.Value(nameFieldID))
	if f.draft != nil && !f.rename {
		name = f.draft.Name
	}
	if err := recipients.CheckGroupName(name); err != nil {
		f.err = err.Error()
		return m, nil
	}
	hosts := []string{}
	for _, row := range f.members {
		if row.checked {
			hosts = append(hosts, row.name)
		}
	}
	root := m.snap.settings.Recipients
	f.err, f.running = "", true
	return m, func() tea.Msg {
		var file string
		var err error
		switch {
		case f.draft == nil:
			file, err = recipients.CreateGroup(root, name, hosts)
		case f.rename:
			file, err = recipients.RenameGroup(root, *f.draft, name)
		default:
			file, err = recipients.SaveGroup(root, *f.draft, hosts)
		}
		return groupSavedMsg{name: name, file: file, err: err}
	}
}

func (m keysModel) finishGroup(msg groupSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.groupFlow.running = false
		m.groupFlow.err = msg.err.Error()
		return m, nil
	}
	m.groupFlow = nil
	m.selectGroup = msg.name
	m.notice = "Saved " + msg.name + " in " + filepath.ToSlash(msg.file)
	return m, m.reload()
}

func (m keysModel) groupFlowView() string {
	f := m.groupFlow
	w, h := pickerSize(m.width, m.height)
	inner := w - 4
	title := "NEW GROUP"
	if f.draft != nil {
		title = "EDIT GROUP · " + f.draft.Name
	}
	if f.rename {
		title = "RENAME GROUP · " + f.draft.Name
	}
	if f.selecting {
		f.list.SetSize(inner, max(1, h-7))
		f.list.Move(0)
		lines := []string{titleStyle.Render("GROUP HOSTS"), "", f.list.View(true, titleStyle, mutedStyle), "", mutedStyle.Render("Unavailable entries stay until unchecked.")}
		return modalBox(lines, pageactions.Footer(inner, "Space Toggle · esc Undo", pageactions.Inline("Apply", true)), w, h)
	}
	ids := f.fields()
	lines := []string{titleStyle.Render(ansi.Truncate(title, inner, "…")), "", f.form.ViewFocusedWidth(ids[:len(ids)-1], true, inner), ""}
	lines = append(lines, wrapped(mutedStyle.Render("Existing encrypted files keep their recipients."), inner)...)
	if f.running {
		lines = append(lines, titleStyle.Render("Saving…"))
	}
	if f.err != "" {
		lines = append(lines, wrapped(warnStyle.Render("! "+f.err), inner)...)
	}
	hints := "↑↓ Field · ↵ Edit · n Save · esc Cancel"
	if f.form.IsActive() {
		hints = "Type to edit · ↵ Apply · esc Undo"
	}
	label := "Save"
	if f.rename {
		label = "Rename"
	}
	return modalBox(lines, pageactions.Footer(inner, hints, pageactions.Inline(label, f.form.FocusedID() == continueFieldID)), w, h)
}

func (m keysModel) groupFlowStatus() tui.Status {
	f := m.groupFlow
	if f.running {
		return tui.Status{Left: "SAVING GROUP"}
	}
	if f.selecting {
		return tui.Status{Left: "SELECT HOSTS", Right: "↑↓ Move  Space Toggle  ↵ Apply  esc Undo"}
	}
	if f.form.IsActive() {
		return tui.Status{Left: "EDIT GROUP", Right: "↵ Apply  esc Undo"}
	}
	return tui.Status{Left: "GROUP", Right: "↑↓ Field  ↵ Edit  n Save  esc Cancel"}
}

func (m keysModel) groupMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	f := m.groupFlow
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	w, h := pickerSize(m.width, m.height)
	x, y := msg.X-(m.width-w)/2-1, msg.Y-(m.height-h)/2-1
	if x < 0 || x >= w-2 || y < 0 || y >= h-2 {
		return m, nil
	}
	if f.selecting {
		f.list.SetSize(w-4, max(1, h-7))
		f.list.Move(0)
		if y == h-3 && x >= w-4-len("Apply")-7 && x < w-4 && msg.Button == tea.MouseButtonLeft {
			f.selecting = false
			return m, nil
		}
		if y >= 2 && y < h-5 {
			switch msg.Button {
			case tea.MouseButtonLeft:
				if f.list.SelectRow(y - 2) {
					f.toggle()
				}
			case tea.MouseButtonWheelUp:
				f.list.Scroll(-3)
			case tea.MouseButtonWheelDown:
				f.list.Scroll(3)
			}
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	label := "Save"
	if f.rename {
		label = "Rename"
	}
	if y == h-3 && x >= w-4-len(label)-7 && x < w-4 {
		if f.form.IsActive() {
			f.form.HandleInteraction("enter")
		}
		return m.saveGroup()
	}
	ids := f.fields()
	if id, ok := f.form.Click(ids[:len(ids)-1], x, y-2); ok && id == groupHostsField {
		f.before = slices.Clone(f.members)
		f.selecting = true
	}
	return m, nil
}
