package capture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/apps/capture/organizer"

	tea "github.com/charmbracelet/bubbletea"
)

// organizedRoot is a Capture root where some of the Captures carry a finished
// organizing record and the rest do not. Archive lists both: only the organized
// ones can be kept, and rejecting one nobody organized is what rejection is for.
func organizedRoot(t *testing.T, organized []string, pending []string) string {
	t.Helper()
	root := routeTestRoot(t, append(append([]string{}, organized...), pending...)...)
	for _, name := range organized {
		run := organizer.Run{
			OrganizedAt: time.Now().Format(time.RFC3339),
			Recipe:      "quick_note",
			RecipeName:  "Quick Note",
			Actions:     []organizer.RecordedAction{{Action: "obsidian.daily.append", Target: "Daily/2026-09-09.md", Executed: true}},
		}
		if _, err := organizer.AppendRun(filepath.Join(root, name), run); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// loadedArchive is the session as it stands once the shared Capture load and
// its own two folder reads have come back.
func loadedArchive(t *testing.T, root, archiveRoot, rejectRoot string) archiveModel {
	t.Helper()
	m := newArchiveModel(root, "index.json", archiveRoot, rejectRoot)
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = sized.(archiveModel)
	loaded, _ := m.Update(loadCaptures(root, "index.json")())
	m = loaded.(archiveModel)
	for destination, folder := range map[archiveDestination]string{
		destinationArchive: archiveRoot, destinationReject: rejectRoot,
	} {
		if cmd := loadArchiveFolder(destination, folder, "index.json"); cmd != nil {
			updated, _ := m.Update(cmd())
			m = updated.(archiveModel)
		}
	}
	return m
}

func press(t *testing.T, m archiveModel, keys ...string) archiveModel {
	t.Helper()
	for _, key := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		switch key {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		}
		updated, _ := m.updateKey(msg)
		m = updated.(archiveModel)
	}
	return m
}

// selectCapture puts the cursor on a Capture by directory name in the list that
// holds it, so a test says which Capture it is acting on rather than how far
// down a column it is.
func selectCapture(t *testing.T, m archiveModel, destination archiveDestination, name string) archiveModel {
	t.Helper()
	path := filepath.Join(m.folderFor(destination), name)
	if !m.listFor(destination).SelectID(archiveItemID(destination, path)) {
		t.Fatalf("capture %q is not listed in %s", name, folderName(destination))
	}
	return m
}

// Either case archives: a reader holds no modifier for anything else here, so
// a Shift left on must not turn the key into nothing.
func TestArchiveAcceptsEitherCaseOfTheArchiveKey(t *testing.T) {
	root := organizedRoot(t, []string{"done-one", "done-two"}, nil)
	archiveRoot := filepath.Join(t.TempDir(), "Archive")
	m := loadedArchive(t, root, archiveRoot, filepath.Join(t.TempDir(), "Reject"))

	m = selectCapture(t, m, destinationCaptures, "done-one")
	m = press(t, m, "a")
	m = selectCapture(t, m, destinationCaptures, "done-two")
	m = press(t, m, "A")

	for _, name := range []string{"done-one", "done-two"} {
		if _, err := os.Stat(filepath.Join(archiveRoot, name)); err != nil {
			t.Fatalf("%s was not archived: %v", name, err)
		}
	}
}

func TestArchiveMovesACaptureAsSoonAsTheKeyIsPressed(t *testing.T) {
	root := organizedRoot(t, []string{"done-one"}, []string{"junk"})
	archiveRoot := filepath.Join(t.TempDir(), "Archive")
	rejectRoot := filepath.Join(t.TempDir(), "Reject")
	m := loadedArchive(t, root, archiveRoot, rejectRoot)

	m = selectCapture(t, m, destinationCaptures, "done-one")
	m = press(t, m, "A")
	if _, err := os.Stat(filepath.Join(archiveRoot, "done-one")); err != nil {
		t.Fatalf("A did not archive the capture: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "done-one")); !os.IsNotExist(err) {
		t.Fatalf("the capture is still in the capture root: %v", err)
	}
	if len(m.archived) != 1 || len(m.entries) != 1 {
		t.Fatalf("lists = %d archived, %d captures; want the row to have moved", len(m.archived), len(m.entries))
	}

	m = selectCapture(t, m, destinationCaptures, "junk")
	m = press(t, m, "backspace")
	if _, err := os.Stat(filepath.Join(rejectRoot, "junk")); err != nil {
		t.Fatalf("backspace did not reject the capture: %v", err)
	}
	if len(m.entries) != 0 {
		t.Fatalf("captures = %d, want both rows moved out", len(m.entries))
	}
}

func TestArchiveRestoresACaptureFromEitherFolder(t *testing.T) {
	root := organizedRoot(t, []string{"done-one"}, []string{"junk"})
	archiveRoot := filepath.Join(t.TempDir(), "Archive")
	rejectRoot := filepath.Join(t.TempDir(), "Reject")
	m := loadedArchive(t, root, archiveRoot, rejectRoot)

	m = selectCapture(t, m, destinationCaptures, "done-one")
	m = press(t, m, "A")
	m = selectCapture(t, m, destinationCaptures, "junk")
	m = press(t, m, "backspace")

	m.fields.Set(archiveFolderField)
	m = selectCapture(t, m, destinationArchive, "done-one")
	m = press(t, m, "R")
	if _, err := os.Stat(filepath.Join(root, "done-one")); err != nil {
		t.Fatalf("R did not restore the archived capture: %v", err)
	}

	m.fields.Set(rejectFolderField)
	m = selectCapture(t, m, destinationReject, "junk")
	m = press(t, m, "R")
	if _, err := os.Stat(filepath.Join(root, "junk")); err != nil {
		t.Fatalf("R did not restore the rejected capture: %v", err)
	}
	if len(m.entries) != 2 || len(m.archived) != 0 || len(m.rejected) != 0 {
		t.Fatalf("lists = %d captures, %d archived, %d rejected; want both back",
			len(m.entries), len(m.archived), len(m.rejected))
	}
}

func TestArchiveRefusesACaptureNobodyOrganized(t *testing.T) {
	root := organizedRoot(t, nil, []string{"junk"})
	archiveRoot := filepath.Join(t.TempDir(), "Archive")
	m := loadedArchive(t, root, archiveRoot, filepath.Join(t.TempDir(), "Reject"))

	m = selectCapture(t, m, destinationCaptures, "junk")
	m = press(t, m, "A")
	if _, err := os.Stat(filepath.Join(archiveRoot, "junk")); !os.IsNotExist(err) {
		t.Fatalf("an unorganized capture was archived: %v", err)
	}
	if !m.notice.bad || !strings.Contains(m.notice.text, "Not organized") {
		t.Fatalf("notice = %q, want it to say why", m.notice.text)
	}
	// The same row still takes a rejection, which is the answer it does have.
	m = press(t, m, "backspace")
	if len(m.rejected) != 1 {
		t.Fatal("an unorganized capture could not be rejected")
	}
}

func TestArchiveSaysWhichConfigurationKeyIsMissing(t *testing.T) {
	root := organizedRoot(t, []string{"done-one"}, nil)
	m := loadedArchive(t, root, "", "")

	m = selectCapture(t, m, destinationCaptures, "done-one")
	m = press(t, m, "A")
	if !strings.Contains(m.notice.text, "capture.archive.root") {
		t.Fatalf("notice = %q, want the archive key named", m.notice.text)
	}
	m = press(t, m, "backspace")
	if !strings.Contains(m.notice.text, "capture.archive.reject") {
		t.Fatalf("notice = %q, want the reject key named", m.notice.text)
	}
	if _, err := os.Stat(filepath.Join(root, "done-one")); err != nil {
		t.Fatalf("the capture should not have moved: %v", err)
	}
}

func TestArchiveReportsACaptureItCouldNotMove(t *testing.T) {
	root := organizedRoot(t, []string{"done-one"}, nil)
	archiveRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(archiveRoot, "done-one"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := loadedArchive(t, root, archiveRoot, t.TempDir())
	m = selectCapture(t, m, destinationCaptures, "done-one")
	m = press(t, m, "A")

	if !m.notice.bad || !strings.Contains(m.notice.text, "already archived") {
		t.Fatalf("notice = %q, want it to say the name was taken", m.notice.text)
	}
	if _, err := os.Stat(filepath.Join(root, "done-one")); err != nil {
		t.Fatalf("a refused capture should stay in the root: %v", err)
	}
}

func TestArchiveButtonsFollowTheFocusedColumn(t *testing.T) {
	root := organizedRoot(t, []string{"done-one"}, nil)
	m := loadedArchive(t, root, filepath.Join(t.TempDir(), "Archive"), filepath.Join(t.TempDir(), "Reject"))
	m = selectCapture(t, m, destinationCaptures, "done-one")

	buttons := m.buttons()
	if len(buttons) != 2 || buttons[0].action != actionArchive || buttons[1].action != actionReject {
		t.Fatalf("captures offers %#v, want Archive and Reject", buttons)
	}
	if !buttons[0].primary || !buttons[1].primary {
		t.Fatal("both controls should be lit for an organized capture with folders configured")
	}

	m = press(t, m, "A")
	m.fields.Set(archiveFolderField)
	m = selectCapture(t, m, destinationArchive, "done-one")
	buttons = m.buttons()
	if len(buttons) != 1 || buttons[0].action != actionRestore {
		t.Fatalf("the archive column offers %#v, want Restore", buttons)
	}
}

func TestArchiveDestinationPanesListWhatTheFoldersHold(t *testing.T) {
	root := organizedRoot(t, []string{"done-one"}, nil)
	archiveRoot, rejectRoot := t.TempDir(), t.TempDir()
	// A Capture already filed before the session opened is read back with the
	// same rule the Capture root is read by.
	existing := organizedRoot(t, []string{"filed-earlier"}, nil)
	if err := os.Rename(filepath.Join(existing, "filed-earlier"), filepath.Join(archiveRoot, "filed-earlier")); err != nil {
		t.Fatal(err)
	}
	m := loadedArchive(t, root, archiveRoot, rejectRoot)

	if len(m.archived) != 1 || m.archived[0].name != "filed-earlier" {
		t.Fatalf("archive pane = %#v, want the capture already filed there", m.archived)
	}
	view := m.View()
	if !strings.Contains(view, "ARCHIVE") || !strings.Contains(view, "REJECT") {
		t.Fatalf("the two destination panes are not both on screen:\n%s", view)
	}
}

func TestCaptureSessionCyclesThreeTabs(t *testing.T) {
	s := newSession()
	for _, want := range []int{1, 2, 0} {
		updated, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
		s = updated.(session)
		if s.active != want {
			t.Fatalf("] moved to tab %d, want %d", s.active, want)
		}
	}
	updated, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	s = updated.(session)
	if s.active != 2 {
		t.Fatalf("[ moved to tab %d, want 2", s.active)
	}
	if tabs := s.Tabs(); len(tabs) != 3 || tabs[2].Label != "Archive" {
		t.Fatalf("tabs = %#v", tabs)
	}
}

// A Capture rejected in Scan has to be in Archive's REJECT column when that tab
// is next looked at, or the only way to take the move back would be to reopen
// the command.
func TestCaptureSessionTellsArchiveAboutAScanRejection(t *testing.T) {
	root := organizedRoot(t, nil, []string{"junk"})
	rejectRoot := filepath.Join(t.TempDir(), "Rejected")
	s := newSessionWithSettings(root, "index.json", filepath.Join(t.TempDir(), "Archive"), rejectRoot,
		organizer.Set{}, organizer.DefaultSettings())
	updated, _ := s.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	s = updated.(session)
	updated, _ = s.Update(loadCaptures(root, "index.json")())
	s = updated.(session)

	if !s.scan.captures.SelectID("capture:" + filepath.Join(root, "junk")) {
		t.Fatal("the capture is not listed in Scan")
	}
	rejected, cmd := s.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	s = rejected.(session)
	if cmd == nil {
		t.Fatal("rejecting in Scan started no reload")
	}
	// Scan is in front, so the folder read has to reach the tab behind it.
	for _, msg := range collect(cmd) {
		updated, _ = s.Update(msg)
		s = updated.(session)
	}
	if len(s.archive.rejected) != 1 || s.archive.rejected[0].name != "junk" {
		t.Fatalf("Archive's reject column = %#v, want the capture Scan sent there", s.archive.rejected)
	}
}

// collect runs a command, flattening a batch into the messages it produced.
func collect(cmd tea.Cmd) []tea.Msg {
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		msgs := make([]tea.Msg, 0, len(batch))
		for _, inner := range batch {
			msgs = append(msgs, collect(inner)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

// The two views answer two questions about the same Capture: what it is, and
// what every other tool on the machine calls it.
func TestArchiveSwitchesBetweenIndexAndFolderViews(t *testing.T) {
	root := organizedRoot(t, []string{"aaaa-xxxxx"}, nil)
	m := loadedArchive(t, root, filepath.Join(t.TempDir(), "Archive"), filepath.Join(t.TempDir(), "Reject"))

	label, detail := m.view.Label(m.entries[0]), m.view.Detail(m.entries[0])
	if !strings.HasPrefix(label, "2026-09-09") || !strings.Contains(detail, "Shortcut") {
		t.Fatalf("index view = %q / %q, want the timestamp with its source under it", label, detail)
	}

	m = press(t, m, "v")
	label, detail = m.view.Label(m.entries[0]), m.view.Detail(m.entries[0])
	if !strings.HasPrefix(label, "aaaa-xxxxx") || !strings.Contains(detail, "2026-09-09") {
		t.Fatalf("folder view = %q / %q, want the directory name with the timestamp under it", label, detail)
	}
	if !strings.Contains(m.View(), "aaaa-xxxxx") {
		t.Fatal("the folder view is not drawn in the column")
	}

	m = press(t, m, "v")
	if label := m.view.Label(m.entries[0]); !strings.HasPrefix(label, "2026-09-09") {
		t.Fatalf("v did not switch back: %q", label)
	}
}
