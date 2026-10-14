package cred

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dgs-toolbox/internal/cred/recipients"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func groupModel(t *testing.T, members string) keysModel {
	t.Helper()
	root := t.TempDir()
	folder := filepath.Join(root, "recipients")
	writeFile(t, filepath.Join(folder, "hosts", "host-a.json"), `{"keys":[]}`, 0o644)
	writeFile(t, filepath.Join(folder, "hosts", "host-b.json"), `{"keys":[]}`, 0o644)
	writeFile(t, filepath.Join(folder, "groups", "g-team.json"), members, 0o644)
	path := filepath.Join(root, "config.json")
	writeFile(t, path, `{"recipients":"`+folder+`"}`, 0o600)
	return keyPress(t, loadedModel(t, path), "]", "]")
}

func TestGroupCreateEditAndRename(t *testing.T) {
	m := groupModel(t, `{"hosts":["host-a"]}`)
	root := m.snap.settings.Recipients
	m = keyPress(t, m, "n")
	if m.groupFlow == nil || !m.groupFlow.form.IsActive() {
		t.Fatal("new form did not open editing its name")
	}
	m = typeInto(t, m, "new")
	m = keyPress(t, m, "enter", "tab", "enter", " ", "enter")
	if _, err := os.Stat(filepath.Join(root, "groups", "g-new.json")); !os.IsNotExist(err) {
		t.Fatal("wrote before Save")
	}
	m = keyPress(t, m, "n")
	if m.groupFlow != nil {
		t.Fatalf("save: %+v", m.groupFlow)
	}
	draft, err := m.selectedGroupDraft()
	if err != nil || draft.Name != "g-new" || !reflect.DeepEqual(draft.Hosts, []string{"host-a"}) {
		t.Fatalf("created %+v %v", draft, err)
	}
	m = keyPress(t, m, "e", "enter", "down", " ", "enter", "n")
	draft, err = m.selectedGroupDraft()
	if err != nil || !reflect.DeepEqual(draft.Hosts, []string{"host-a", "host-b"}) {
		t.Fatalf("edited %+v %v", draft, err)
	}
	m = keyPress(t, m, "r")
	if m.groupFlow == nil || !m.groupFlow.rename {
		t.Fatal("rename did not open")
	}
	m = typeInto(t, m, "-renamed")
	m = keyPress(t, m, "enter", "n")
	draft, err = m.selectedGroupDraft()
	if err != nil || draft.Name != "g-new-renamed" || len(draft.Hosts) != 2 {
		t.Fatalf("renamed %+v %v", draft, err)
	}
	// The next encryption picker expands the edited and renamed group.
	for _, host := range []string{"host-a", "host-b"} {
		if _, err := recipients.AddKey(root, host, newAgeRecipient(t), recipients.Meta{Description: "generated test key"}); err != nil {
			t.Fatal(err)
		}
	}
	m = keyPress(t, m, "R")
	p := newRecipientPicker(m.snap.folder, nil, map[string]bool{}, nil)
	if len(p.rows) == 0 || p.rows[0].group.Name != "g-new-renamed" {
		t.Fatal("saved group absent from encryption picker")
	}
	p.update(" ")
	chosen := p.chosen()
	if len(chosen) != 2 || chosen[0].Host != "host-a" || chosen[1].Host != "host-b" {
		t.Fatalf("group did not resolve: %+v", chosen)
	}
}

func TestGroupCancelAndWarningsPreserveMembers(t *testing.T) {
	m := groupModel(t, `{"hosts":["host-a","missing","HOST-A"]}`)
	root := m.snap.settings.Recipients
	path := filepath.Join(root, "groups", "g-team.json")
	original, _ := os.ReadFile(path)
	m = keyPress(t, m, "e", "enter")
	screen := view(m)
	for _, want := range []string{"missing", "unavailable", "repeated member", "no keys"} {
		if !strings.Contains(screen, want) {
			t.Errorf("view lacks %q:\n%s", want, screen)
		}
	}
	if !m.CapturesShellKey("esc") || !m.CapturesShellKey("q") {
		t.Fatal("overlay does not capture shell keys")
	}
	m = keyPress(t, m, " ", "esc")
	if !m.groupFlow.members[0].checked {
		t.Fatal("checklist cancel did not restore selection")
	}
	m = keyPress(t, m, "esc")
	if m.groupFlow != nil {
		t.Fatal("cancel left form open")
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(original) {
		t.Fatal("cancel wrote the file")
	}
	m = keyPress(t, m, "e", "n")
	draft, err := m.selectedGroupDraft()
	if err != nil || !reflect.DeepEqual(draft.Hosts, []string{"host-a", "missing", "HOST-A"}) {
		t.Fatalf("lost members: %+v %v", draft, err)
	}
	// An unresolved member may be explicitly removed.
	m = keyPress(t, m, "e", "enter", "down", " ", "enter", "n")
	draft, err = m.selectedGroupDraft()
	if err != nil || !reflect.DeepEqual(draft.Hosts, []string{"host-a", "HOST-A"}) {
		t.Fatalf("explicit removal: %+v %v", draft, err)
	}
}

func TestGroupConflictAndInvalidNameKeepForm(t *testing.T) {
	m := groupModel(t, `{"hosts":["host-a"]}`)
	m = keyPress(t, m, "e", "enter", "down", " ", "enter")
	path := filepath.Join(m.snap.settings.Recipients, "groups", "g-team.json")
	newer := `{"hosts":["host-b"]}`
	writeFile(t, path, newer, 0o644)
	m = keyPress(t, m, "n")
	if m.groupFlow == nil || m.groupFlow.running || !strings.Contains(m.groupFlow.err, "changed") || !m.groupFlow.members[1].checked {
		t.Fatal("conflict discarded the form")
	}
	data, _ := os.ReadFile(path)
	if string(data) != newer {
		t.Fatal("conflict overwritten")
	}
	m = keyPress(t, m, "esc", "n", "backspace", "backspace", "enter", "n")
	if m.groupFlow == nil || !strings.Contains(m.groupFlow.err, "must start") {
		t.Fatal("invalid name accepted")
	}
	m = keyPress(t, m, "esc", "n")
	m = typeInto(t, m, "team")
	m = keyPress(t, m, "enter", "n")
	if m.groupFlow == nil || !strings.Contains(m.groupFlow.err, "already exists") {
		t.Fatal("duplicate name accepted")
	}
}

func TestGroupDeleteChecksChangesAndUsesTrash(t *testing.T) {
	m := groupModel(t, `{"hosts":["host-a"]}`)
	path := filepath.Join(m.snap.settings.Recipients, "groups", "g-team.json")
	calls := 0
	m.trash = func(path string) (string, error) {
		calls++
		to := filepath.Join(t.TempDir(), "g-team.json")
		return to, os.Rename(path, to)
	}
	m = keyPress(t, m, "d", "enter")
	if calls != 0 || m.deleteFlow != nil {
		t.Fatal("delete did not default to Keep")
	}
	m = keyPress(t, m, "d")
	writeFile(t, path, `{"hosts":[]}`, 0o644)
	m = keyPress(t, m, "tab", "enter")
	if calls != 0 || !strings.Contains(m.notice, "changed") {
		t.Fatal("trashed an external edit")
	}
	m = keyPress(t, m, "R", "d", "tab", "enter")
	if calls != 1 || len(m.snap.folder.Groups) != 0 || len(m.snap.folder.Hosts) != 2 {
		t.Fatal("group deletion affected hosts or did not reload")
	}
	m = keyPress(t, m, "n")
	m = typeInto(t, m, "empty")
	m = keyPress(t, m, "enter", "n")
	m.trash = func(string) (string, error) { return "", errors.New("trash unavailable") }
	m = keyPress(t, m, "d", "tab", "enter")
	if !strings.Contains(m.notice, "trash unavailable") || len(m.snap.folder.Groups) != 1 {
		t.Fatal("trash error did not preserve group")
	}
}

func TestGroupChecklistScrollAndMouse(t *testing.T) {
	m := groupModel(t, `{"hosts":[]}`)
	for i := 0; i < 40; i++ {
		writeFile(t, filepath.Join(m.snap.settings.Recipients, "hosts", fmt.Sprintf("node-%02d.json", i)), `{"keys":[]}`, 0o644)
	}
	m = keyPress(t, m, "R", "e", "enter", "G")
	screen := view(m)
	if !strings.Contains(screen, "node-39") || !strings.Contains(screen, "Apply") {
		t.Fatalf("last host or footer hidden:\n%s", screen)
	}
	for _, size := range [][2]int{{60, 12}, {80, 24}, {140, 30}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(keysModel)
		screen = view(m)
		lines := strings.Split(screen, "\n")
		if len(lines) != size[1] {
			t.Errorf("height %d != %d", len(lines), size[1])
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Errorf("line overflow: %s", line)
			}
		}
		if !strings.Contains(screen, "node-39") || !strings.Contains(screen, "Apply") {
			t.Errorf("scroll/footer lost at %v", size)
		}
	}
	m = keyPress(t, m, "home")
	_ = m.View()
	w, h := pickerSize(m.width, m.height)
	mouse := tea.MouseMsg{X: (m.width-w)/2 + 4, Y: (m.height-h)/2 + 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	updated, _ := m.Update(mouse)
	m = updated.(keysModel)
	if !m.groupFlow.members[0].checked {
		t.Fatal("mouse did not toggle member")
	}
	m = keyPress(t, m, "enter")
	// Clicking the footer hints must not save.
	mouse.Y = (m.height-h)/2 + h - 2
	updated, cmd := m.Update(mouse)
	m = updated.(keysModel)
	if cmd != nil || m.groupFlow.running {
		t.Fatal("clicking hints saved the group")
	}
}

func TestGroupProblemCannotBeEdited(t *testing.T) {
	m := groupModel(t, `{`)
	for _, key := range []string{"e", "r", "d"} {
		m = keyPress(t, m, key)
		if m.groupFlow != nil || m.deleteFlow != nil {
			t.Fatalf("%s acts on a problem row", key)
		}
	}
	// A group can be created while an unrelated group has a parse error.
	m = keyPress(t, m, "n")
	m = typeInto(t, m, "valid")
	m = keyPress(t, m, "enter", "n")
	if m.groupFlow != nil {
		t.Fatalf("unrelated error blocked creation: %+v", m.groupFlow)
	}
	if _, err := recipients.ReadGroup(m.snap.settings.Recipients, "g-valid"); err != nil {
		t.Fatal(err)
	}
}

func TestGroupUnavailableNameIsEscaped(t *testing.T) {
	m := groupModel(t, `{"hosts":["bad\nname\u001b[31m"]}`)
	m = keyPress(t, m, "e", "enter")
	item, ok := m.groupFlow.list.Selected()
	if !ok || strings.ContainsAny(item.Label, "\n\x1b") || !strings.Contains(item.Label, `\n`) {
		t.Fatalf("unsafe label %q", item.Label)
	}
	m = keyPress(t, m, "enter", "n")
	draft, err := m.selectedGroupDraft()
	if err != nil || len(draft.Hosts) != 1 || draft.Hosts[0] != "bad\nname\x1b[31m" {
		t.Fatalf("raw name not retained: %+v %v", draft, err)
	}
}
