package photo

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/verifiedcopy"
)

// profileModel builds Setup with two profiles over synthetic folders: "card"
// with every directory there, "gone" whose source is missing.
func profileModel(t *testing.T, card config.PhotoImportProfile) (importModel, string) {
	t.Helper()
	root := t.TempDir()
	writeTree(t, filepath.Join(root, "card"), "DCIM/IMG_0001.JPG", "DCIM/IMG_0001.CR3", "DCIM/IMG_0002.MOV")
	writeTree(t, filepath.Join(root, "library"), ".keep")
	writeTree(t, filepath.Join(root, "backup"), ".keep")
	card.Source = filepath.Join(root, "card")
	card.Dest = []string{filepath.Join(root, "library"), filepath.Join(root, "backup")}
	global := config.Default()
	global.Photo.Import.Profiles = map[string]config.PhotoImportProfile{
		"card": card,
		"gone": {Source: filepath.Join(root, "absent"), Dest: []string{filepath.Join(root, "library")}},
	}
	model := New().Commands[0].NewWithConfig(global).(importModel)
	model.width, model.height = 120, 40
	return model, root
}

func press(t *testing.T, m importModel, keys ...tea.KeyMsg) (importModel, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, key := range keys {
		var next tea.Model
		next, cmd = m.Update(key)
		m = next.(importModel)
	}
	return m, cmd
}

var (
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyUp    = tea.KeyMsg{Type: tea.KeyUp}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyTab   = tea.KeyMsg{Type: tea.KeyTab}
	keyBack  = tea.KeyMsg{Type: tea.KeyShiftTab}
)

func scanned(t *testing.T, m importModel) importModel {
	t.Helper()
	targets := make([]scanSummary, len(m.destinationRoots()))
	for i, root := range m.destinationRoots() {
		targets[i] = scanDirectories([pathFieldCount]string{m.paths[sourceField], root})
	}
	next, _ := m.Update(scanDoneMsg{generation: m.scanGeneration, summary: targets[0], targets: targets})
	return next.(importModel)
}

func TestSetupOpensOnFirstProfileWithItsDirectories(t *testing.T) {
	model, root := profileModel(t, config.PhotoImportProfile{})
	if !model.profileFocus || model.profileIndex != 0 || model.profileNames[0] != "card" {
		t.Fatalf("focus %v index %d", model.profileFocus, model.profileIndex)
	}
	want := []string{displayPath(filepath.Join(root, "card")), displayPath(filepath.Join(root, "library")), displayPath(filepath.Join(root, "backup"))}
	if got := append([]string{model.paths[sourceField]}, model.destinationRoots()...); !slices.Equal(got, want) {
		t.Fatalf("directories %v, want %v", got, want)
	}
	view := model.View()
	for _, text := range []string{"Profiles", "card", "2 destinations", "gone", "not there:", "Manual", "Choose a profile or directories"} {
		if !strings.Contains(view, text) {
			t.Errorf("Setup lacks %q", text)
		}
	}
	if model.Status().Left != "READY · PROFILE" {
		t.Fatalf("status %q", model.Status().Left)
	}
}

func TestProfileCursorFillsDirectoriesAndManualKeepsThem(t *testing.T) {
	model, root := profileModel(t, config.PhotoImportProfile{})
	model, _ = press(t, model, keyDown)
	if model.paths[sourceField] != displayPath(filepath.Join(root, "absent")) || len(model.destinationRoots()) != 1 {
		t.Fatal("moving to gone did not show its paths")
	}
	model, _ = press(t, model, keyDown, keyDown)
	if model.profileIndex != 2 || model.paths[sourceField] != displayPath(filepath.Join(root, "absent")) {
		t.Fatal("Manual must keep the directories shown and stop at the end")
	}
	model, _ = press(t, model, keyUp, keyUp)
	if model.profileIndex != 0 || len(model.destinationRoots()) != 2 {
		t.Fatal("moving back did not restore card's paths")
	}
	// Enter on Manual goes into Directories.
	model, _ = press(t, model, keyDown, keyDown, keyEnter)
	if model.profileFocus || !model.controls.Focused(sourceID) {
		t.Fatal("Manual did not move into Directories")
	}
}

func TestMissingProfileDirectoryDoesNotScan(t *testing.T) {
	model, root := profileModel(t, config.PhotoImportProfile{})
	model, cmd := press(t, model, keyDown, keyEnter)
	if model.stage != setupStage || cmd != nil || !strings.Contains(model.View(), filepath.Base(filepath.Join(root, "absent"))+" is not there") {
		t.Fatalf("stage %v cmd %v", model.stage, cmd != nil)
	}
}

func TestProfileScansAtOnceAndPresetsParameters(t *testing.T) {
	incremental := false
	model, _ := profileModel(t, config.PhotoImportProfile{Ext: []string{"jpg", ".CR3"}, Duplicates: "keep-both", Workers: 3, Incremental: &incremental, Organize: true})
	model, cmd := press(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if model.stage != scanStage || cmd == nil || model.activeProfile != "card" {
		t.Fatalf("n did not scan card: stage %v profile %q", model.stage, model.activeProfile)
	}
	model = scanned(t, model)
	if model.stage != parameterStage {
		t.Fatal("scan did not open Parameters")
	}
	if got := model.controls.Values(extensionsID); !slices.Equal(got, []string{"CR3", "JPG"}) {
		t.Errorf("extensions %v", got)
	}
	if model.controls.Value(duplicatesID) != "Keep both" || model.controls.Value(parallelID) != "3" || model.controls.Checked(incrementalID) {
		t.Errorf("duplicates %q workers %q incremental %v", model.controls.Value(duplicatesID), model.controls.Value(parallelID), model.controls.Checked(incrementalID))
	}
	if !strings.Contains(model.Status().Left, "card") {
		t.Errorf("status %q does not name the profile", model.Status().Left)
	}
	next, _ := model.startPostProcessing()
	model = next.(importModel)
	if _, decision := model.postprocessDialog.Update("enter"); decision != confirm.Confirmed {
		t.Fatal("organize did not preset the Organize choice")
	}
}

func TestChangedPathMakesTheImportManual(t *testing.T) {
	model, root := profileModel(t, config.PhotoImportProfile{Duplicates: "keep-both"})
	model, _ = press(t, model, keyTab)
	if model.profileFocus || !model.controls.Focused(sourceID) {
		t.Fatal("Tab did not move into Directories")
	}
	model.paths[sourceField] = displayPath(filepath.Join(root, "library"))
	model.rebuildDirectoryControls()
	next, _ := model.startScan()
	model = scanned(t, next.(importModel))
	if model.activeProfile != "" || model.controls.Value(duplicatesID) != "Skip" {
		t.Fatalf("profile %q duplicates %q", model.activeProfile, model.controls.Value(duplicatesID))
	}
	next, _ = model.startPostProcessing()
	if _, decision := next.(importModel).postprocessDialog.Update("enter"); decision != confirm.Cancelled {
		t.Fatal("Manual must start on Skip")
	}
}

func TestShiftTabFromSourceReturnsToProfiles(t *testing.T) {
	model, _ := profileModel(t, config.PhotoImportProfile{})
	model, _ = press(t, model, keyTab, keyBack)
	if !model.profileFocus {
		t.Fatal("Shift+Tab from Source did not return to Profiles")
	}
}

func TestClickingAProfileRowChoosesIt(t *testing.T) {
	model, _ := profileModel(t, config.PhotoImportProfile{})
	model.profileFocus = false
	lines := strings.Split(model.View(), "\n")
	for y, line := range lines {
		if strings.Contains(line, "gone") && strings.Contains(line, "not there") {
			next, _ := model.clickSetup(strings.Index(line, "gone"), y)
			model = next.(importModel)
			if !model.profileFocus || model.profileIndex != 1 {
				t.Fatalf("click on row %d chose %d", y, model.profileIndex)
			}
			return
		}
	}
	t.Fatal("gone row not drawn")
}

func TestSetupWithoutProfilesIsUnchanged(t *testing.T) {
	model := New().Commands[0].NewWithConfig(config.Default()).(importModel)
	if model.profileFocus || strings.Contains(model.View(), "Profiles") || !model.controls.Focused(sourceID) {
		t.Fatal("Setup without profiles changed")
	}
}

func TestProfileWithCheckRenameLocksDestinationsWhileProcessing(t *testing.T) {
	model, root := profileModel(t, config.PhotoImportProfile{AllowCheckRename: true})
	model, _ = press(t, model, keyEnter)
	model = scanned(t, model)
	if !strings.Contains(model.Status().Left, "CHECK-RENAME") {
		t.Fatalf("status %q does not warn", model.Status().Left)
	}
	next, _ := model.startProcessing()
	model = next.(importModel)
	lock := filepath.Join(root, "library", verifiedcopy.LockName)
	if _, err := os.Stat(lock); err != nil {
		t.Fatal("destination not locked while processing", err)
	}
	model.Close()
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("lock left after close")
	}
}

func TestProfileRecheckedWhenChosenAfterCardIsInserted(t *testing.T) {
	model, root := profileModel(t, config.PhotoImportProfile{})
	if model.profileMissing["gone"] == "" {
		t.Fatal("missing source not noted")
	}
	writeTree(t, filepath.Join(root, "absent"), "IMG_0001.JPG")
	model, cmd := press(t, model, keyDown, keyEnter)
	if model.stage != scanStage || cmd == nil {
		t.Fatalf("inserted card not scanned: stage %v", model.stage)
	}
}

func TestLeavingParametersReleasesTheLock(t *testing.T) {
	model, root := profileModel(t, config.PhotoImportProfile{AllowCheckRename: true})
	model, _ = press(t, model, keyEnter)
	model = scanned(t, model)
	release, err := allowCheckRename(io.Discard, []string{filepath.Join(root, "library")})
	if err != nil {
		t.Fatal(err)
	}
	model.releaseCheckRename = release
	model, _ = press(t, model, tea.KeyMsg{Type: tea.KeyEsc})
	if model.stage != setupStage || model.releaseCheckRename != nil {
		t.Fatal("Esc to Setup kept the lock")
	}
	if _, err := os.Stat(filepath.Join(root, "library", verifiedcopy.LockName)); !os.IsNotExist(err) {
		t.Fatal("lock file left")
	}
}

func TestCheckRenameIsAParameterInManualImports(t *testing.T) {
	model, root := profileModel(t, config.PhotoImportProfile{})
	if model.controls.Checked(checkRenameID) {
		t.Fatal("check-rename on by default")
	}
	model, _ = press(t, model, keyTab)
	next, _ := model.startScan()
	model = scanned(t, next.(importModel))
	model.controls.SetChecked(checkRenameID, true)
	if !strings.Contains(model.Status().Left, "CHECK-RENAME") {
		t.Fatal("status does not warn")
	}
	next, _ = model.startProcessing()
	model = next.(importModel)
	if _, err := os.Stat(filepath.Join(root, "library", verifiedcopy.LockName)); err != nil {
		t.Fatal("manual check-rename did not lock", err)
	}
	model.Close()
}
