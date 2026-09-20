package conf

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func selectedLabel(t *testing.T, m InspectModel) string {
	t.Helper()
	item, ok := m.list.Selected()
	if !ok {
		t.Fatal("nothing selected")
	}
	return item.Label
}

func pressKey(t *testing.T, m InspectModel, msg tea.KeyMsg) (InspectModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(InspectModel), cmd
}

func TestInspectMark_NodeMarksEveryInstance(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	if !m.list.SelectID("node:srv") {
		t.Fatal("no srv row")
	}
	m = pressInspect(t, m, " ")
	if !strings.HasSuffix(selectedLabel(t, m), "▾ [x] srv") {
		t.Fatalf("srv label = %q, want it marked", selectedLabel(t, m))
	}
	if !m.marked["ss-srv"] {
		t.Fatalf("marked = %v, want ss-srv", m.marked)
	}
	m = pressInspect(t, m, " ")
	if len(m.marked) != 0 {
		t.Fatalf("marked = %v after a second Space, want none", m.marked)
	}
}

func TestInspectMark_PartialAndSharedAcrossTabs(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.setTab(tabUsers)
	m.list.SelectID("user:doug")
	m = pressInspect(t, m, " ")
	if len(m.marked) == 0 {
		t.Fatal("marking doug marked nothing")
	}

	// One of doug's files unmarked leaves doug partly marked.
	var one string
	for instance := range m.marked {
		one = instance
		break
	}
	m.list.SelectID("inst:" + one)
	m = pressInspect(t, m, " ")
	if len(m.marked) > 0 {
		m.list.SelectID("user:doug")
		if !strings.Contains(selectedLabel(t, m), "[-] doug") {
			t.Fatalf("doug label = %q, want partly marked", selectedLabel(t, m))
		}
	}

	// The laptop is the same entry seen from Nodes.
	m.list.SelectID("user:doug")
	m = pressInspect(t, m, " ")
	m.setTab(tabNodes)
	m.list.SelectID("node:laptop")
	if !strings.Contains(selectedLabel(t, m), "[x] laptop") {
		t.Fatalf("laptop label on Nodes = %q, want the mark made on Users", selectedLabel(t, m))
	}
}

func TestInspectMark_AllTogglesTheTab(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m = pressInspect(t, m, "a")
	if len(m.marked) == 0 {
		t.Fatal("a marked nothing")
	}
	if !strings.Contains(m.Status().Center, "marked") {
		t.Fatalf("status centre = %q, want the marked count", m.Status().Center)
	}
	m = pressInspect(t, m, "a")
	if len(m.marked) != 0 {
		t.Fatalf("marked = %v after a second a, want none", m.marked)
	}
}

func TestInspectMark_BoxFollowsTheIndent(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.list.SelectID("inst:ss-srv")
	if got := selectedLabel(t, m); !strings.Contains(got, "─ [ ] ss-srv") {
		t.Fatalf("instance label = %q, want the box after its branch", got)
	}
}

func TestInspectExport_NoMarksTakesTheCursorRow(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.list.SelectID("inst:ss-srv")
	if got := m.exportSelection(); len(got) != 1 || got[0] != "ss-srv" {
		t.Fatalf("exportSelection = %v, want [ss-srv]", got)
	}
}

// runInspectExport marks srv, walks the form and the confirmation, and runs
// the write.
func runInspectExport(t *testing.T, format, dest string) InspectModel {
	t.Helper()
	m := newInspectModel(buildExportableRoot(t))
	m.list.SelectID("node:srv")
	m = pressInspect(t, m, " ")
	m = pressInspect(t, m, "x")
	if m.export == nil {
		t.Fatal("x opened no export")
	}
	m.export.form.SetValue(fieldFormat, format)
	m.export.form.SetValue(fieldDest, dest)
	m = pressInspect(t, m, "n")
	if m.export.stage != exportConfirm {
		t.Fatalf("n did not reach the confirmation: %v", m.export.err)
	}
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m, cmd := pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirming ran nothing")
	}
	next, _ := m.Update(cmd())
	return next.(InspectModel)
}

func TestInspectExport_Folder(t *testing.T) {
	dest := t.TempDir()
	m := runInspectExport(t, formatFolder, dest)
	if m.export != nil || len(m.marked) != 0 {
		t.Fatalf("export still open or marks kept: %+v", m.export)
	}
	if _, err := os.Stat(filepath.Join(dest, "srv", "hysteria2", "us-sfo", "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Status().Center, "exported") {
		t.Fatalf("status centre = %q", m.Status().Center)
	}
}

func TestInspectExport_ZipIntoDirectory(t *testing.T) {
	dest := t.TempDir()
	runInspectExport(t, formatZip, dest)
	r, err := zip.OpenReader(filepath.Join(dest, defaultZipName))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if len(r.File) != 1 || r.File[0].Name != "srv/hysteria2/us-sfo/config.yaml" {
		t.Fatalf("zip holds %v", r.File)
	}
}

func TestInspectExport_CustomZipName(t *testing.T) {
	dest := t.TempDir()
	m := newInspectModel(buildExportableRoot(t))
	m.list.SelectID("node:srv")
	m = pressInspect(t, m, "x")
	m.export.form.SetValue(fieldFormat, formatZip)
	if got := m.export.visibleFields(); len(got) != 4 || got[2] != fieldZipName {
		t.Fatalf("Zip fields = %v", got)
	}
	m.export.form.SetValue(fieldDest, dest)
	m.export.form.SetValue(fieldZipName, "server-config")
	m = pressInspect(t, m, "n")
	want := filepath.Join(dest, "server-config.zip")
	if m.export.stage != exportConfirm || m.export.where != want {
		t.Fatalf("confirmation = %v at %q, want %q: %v", m.export.stage, m.export.where, want, m.export.err)
	}
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m, cmd := pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ := m.Update(cmd())
	m = next.(InspectModel)
	if _, err := zip.OpenReader(want); err != nil {
		t.Fatal(err)
	}
}

func TestInspectExport_RejectsZipPathAsName(t *testing.T) {
	m := newInspectModel(buildExportableRoot(t))
	m.list.SelectID("node:srv")
	m = pressInspect(t, m, "x")
	m.export.form.SetValue(fieldFormat, formatZip)
	m.export.form.SetValue(fieldDest, t.TempDir())
	m.export.form.SetValue(fieldZipName, "other/archive.zip")
	m = pressInspect(t, m, "n")
	if m.export.stage != exportForm || m.export.err == nil {
		t.Fatal("a ZIP name containing a path reached confirmation")
	}
}

func TestExportDestinationKeepsExplicitZipPath(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "existing-name.zip")
	if got := exportDestination(dest, defaultZipName); got != dest {
		t.Fatalf("destination = %q, want %q", got, dest)
	}
}

func TestInspectExport_RefusesToOverwriteUnasked(t *testing.T) {
	dest := t.TempDir()
	runInspectExport(t, formatFolder, dest)

	m := newInspectModel(buildExportableRoot(t))
	m.list.SelectID("inst:us-sfo")
	m = pressInspect(t, m, "x")
	m.export.form.SetValue(fieldDest, dest)
	m = pressInspect(t, m, "n")
	if m.export.stage != exportForm || m.export.err == nil {
		t.Fatal("an export over existing files went on without Replace")
	}
}

func TestInspectExport_EscCloses(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.list.SelectID("node:srv")
	m = pressInspect(t, m, "x")
	if !m.CapturesShellKey("esc") {
		t.Fatal("an open export lets Esc reach the shell")
	}
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.export != nil {
		t.Fatal("Esc left the export open")
	}
}

func TestInspectExport_ShowOneUserAndCopy(t *testing.T) {
	m := newInspectModel(buildExportableRoot(t))
	var copied string
	m.copy = func(text string) error { copied = text; return nil }
	m.width, m.height = 120, 40
	m.setTab(tabUsers)
	if !m.list.SelectID("user:friend-a") {
		t.Fatal("no friend-a row")
	}
	m = pressInspect(t, m, "x")
	if got := m.export.form.Value(fieldFormat); got != formatShow {
		t.Fatalf("format = %q for one person's files, want Show first", got)
	}
	if got := m.export.visibleFields(); len(got) != 1 {
		t.Fatalf("Show asks for %v, want no destination", got)
	}
	m = pressInspect(t, m, "n")
	if m.export.stage != exportShow {
		t.Fatalf("n did not show the files: %v", m.export.err)
	}
	if !strings.Contains(m.View(), "hysteria2://") {
		t.Fatal("the rendered link is not on screen")
	}
	m, cmd := pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd == nil {
		t.Fatal("c copied nothing")
	}
	next, _ := m.Update(cmd())
	m = next.(InspectModel)
	if !strings.HasPrefix(copied, "hysteria2://") {
		t.Fatalf("copied %q", copied)
	}
	if !strings.HasPrefix(m.export.copied, "copied ") {
		t.Fatalf("copy outcome = %q", m.export.copied)
	}
}

func TestInspectExport_ShowOnlyForOnePerson(t *testing.T) {
	m := newInspectModel(buildExportableRoot(t))
	m.list.SelectID("node:srv")
	m = pressInspect(t, m, "x")
	if got := m.export.form.Value(fieldFormat); got != formatFolder {
		t.Fatalf("format = %q for a server, want Folder: Show is for one person", got)
	}
}

func TestInspectExport_DestinationFromFileExplorer(t *testing.T) {
	dest := t.TempDir()
	m := newInspectModel(buildExportableRoot(t))
	m.exportDir = dest
	m.list.SelectID("node:srv")
	m = pressInspect(t, m, "x")
	m.export.form.SetFocusID(fieldDest)
	m, cmd := pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.export.picking {
		t.Fatal("Enter on Destination opened no File Explorer")
	}
	for cmd != nil {
		msg := cmd()
		next, c := m.Update(msg)
		m, cmd = next.(InspectModel), c
	}
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.export.picking {
		t.Fatal("Enter in the File Explorer chose nothing")
	}
	if got := m.export.form.Value(fieldDest); got != dest {
		t.Fatalf("destination = %q, want %q", got, dest)
	}
}

func TestInspectExport_TabClickIgnoredWhileOpen(t *testing.T) {
	m := newInspectModel(buildInspectRoot(t), "")
	m.list.SelectID("node:srv")
	m = pressInspect(t, m, "x")
	next, _ := m.Update(tui.TabSelectedMsg{Index: tabUsers})
	if m = next.(InspectModel); m.tab != tabNodes {
		t.Fatal("a tab click switched the page behind an open export")
	}
}

func TestInspectExport_PickerFollowsResize(t *testing.T) {
	m := newInspectModel(buildExportableRoot(t))
	m.width, m.height = 120, 40
	m.list.SelectID("node:srv")
	m = pressInspect(t, m, "x")
	m.export.form.SetFocusID(fieldDest)
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(InspectModel)
	w, _ := m.pickerSize()
	for _, line := range strings.Split(m.export.picker.View(), "\n") {
		if lipgloss.Width(line) > w-4 {
			t.Fatalf("picker line %d wide after resize to %d: %q", lipgloss.Width(line), w, line)
		}
	}
}

func TestInspectExport_PickerDirectoryRowsStayOnOneLine(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	dest := t.TempDir()
	for _, name := range []string{"Applications", "DaVinci Resolve Media"} {
		if err := os.Mkdir(filepath.Join(dest, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	m := newInspectModel(buildExportableRoot(t))
	m.width, m.height = 100, 30
	m.list.SelectID("node:srv")
	m = pressInspect(t, m, "x")
	m.export.form.SetValue(fieldDest, dest)
	m.export.form.SetFocusID(fieldDest)
	var cmd tea.Cmd
	m, cmd = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd()
	next, _ := m.Update(msg)
	m = next.(InspectModel)
	view := ansi.Strip(m.pickerView())
	for _, name := range []string{"Applications", "DaVinci Resolve Media"} {
		found := false
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, "▸ "+name) {
				found = true
			}
		}
		if !found {
			t.Fatalf("directory %q was split across rows:\n%s", name, view)
		}
	}
}

func TestInspectExport_PickerFrameFitsNarrowTerminal(t *testing.T) {
	m := newInspectModel(buildExportableRoot(t))
	m.width, m.height = 60, 20
	m.list.SelectID("node:srv")
	m = pressInspect(t, m, "x")
	m.export.form.SetFocusID(fieldDest)
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	w, h := m.pickerSize()
	view := m.pickerView()
	if got := lipgloss.Width(view); got != w {
		t.Fatalf("picker width = %d, want %d", got, w)
	}
	if got := lipgloss.Height(view); got != h {
		t.Fatalf("picker height = %d, want %d", got, h)
	}
	if !strings.Contains(strings.Split(view, "\n")[h-1], "╯") {
		t.Fatal("picker bottom border was clipped")
	}
}
