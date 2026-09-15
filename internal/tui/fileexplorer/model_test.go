package fileexplorer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestDirectoryTreeExpandsLazilyAndKeepsContext(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	if err := os.MkdirAll(filepath.Join(a, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "b"), 0o755); err != nil {
		t.Fatal(err)
	}

	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	if len(tree.visible) != 3 {
		t.Fatalf("initial visible nodes = %d, want root, a, and b", len(tree.visible))
	}

	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	tree, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyRight})
	if cmd == nil {
		t.Fatal("expanding an unloaded directory did not request a directory read")
	}
	tree, _, _ = tree.Update(cmd())
	if len(tree.visible) != 4 {
		t.Fatalf("expanded visible nodes = %d, want root, a, child, and b", len(tree.visible))
	}
	want := []string{filepath.Base(root), "a", "child", "b"}
	for i, name := range want {
		got := tree.visible[i].name
		if i == 0 {
			got = filepath.Base(tree.visible[i].path)
		}
		if got != name {
			t.Fatalf("visible node %d = %q, want %q", i, got, name)
		}
	}
}

func TestDirectoryTreeSupportsToggleAndParentClose(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.MkdirAll(filepath.Join(parent, "child"), 0o755); err != nil {
		t.Fatal(err)
	}

	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	tree, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("o did not request children for an unloaded directory")
	}
	tree, _, _ = tree.Update(cmd())
	if len(tree.visible) != 3 || tree.visible[2].depth != 2 {
		t.Fatalf("o did not expose the indented child: %#v", tree.visible)
	}

	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if len(tree.visible) != 2 || tree.visible[1].expanded {
		t.Fatal("second o did not toggle the directory closed")
	}
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	if len(tree.visible) != 2 {
		t.Fatalf("O left %d nodes visible, want root and parent", len(tree.visible))
	}
	if tree.visible[1].expanded || tree.selected != 1 {
		t.Fatal("O did not collapse and focus the current node's parent")
	}
}

func TestDirectoryTreeCollapsesRecursively(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}

	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	tree, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	tree, _, _ = tree.Update(cmd())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	tree, _, cmd = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	tree, _, _ = tree.Update(cmd())
	if len(tree.visible) != 4 {
		t.Fatalf("setup exposed %d nodes, want the complete four-level tree", len(tree.visible))
	}

	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if len(tree.visible) != 1 || tree.selected != 0 {
		t.Fatalf("w left %d visible nodes at selection %d", len(tree.visible), tree.selected)
	}
	for node := tree.root; len(node.children) > 0; node = node.children[0] {
		if node.expanded {
			t.Fatalf("w left directory %q expanded", node.name)
		}
	}
}

func TestDirectoryFilterHidesFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	tree := New(root, 60, 12, WithFilter(Directories()))
	tree, _, _ = tree.Update(tree.Init()())
	if tree.FilterLabel() != "DIR" {
		t.Fatalf("filter label = %q, want DIR", tree.FilterLabel())
	}
	if len(tree.visible) != 2 || tree.visible[1].name != "album" {
		t.Fatalf("directory filter exposed unexpected entries: %#v", tree.visible)
	}
}

func TestExtensionFilterShowsDirectoriesAndMatchingFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.JPG", "second.png", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tree := New(root, 60, 12, WithFilter(Extensions("jpg", ".png")))
	tree, _, _ = tree.Update(tree.Init()())
	if tree.FilterLabel() != "JPG, PNG" {
		t.Fatalf("filter label = %q, want JPG, PNG", tree.FilterLabel())
	}
	want := []string{displayPath(root), "album", "first.JPG", "second.png"}
	if len(tree.visible) != len(want) {
		t.Fatalf("visible entries = %d, want %d", len(tree.visible), len(want))
	}
	for i, name := range want {
		if tree.visible[i].name != name {
			t.Fatalf("visible entry %d = %q, want %q", i, tree.visible[i].name, name)
		}
	}

	tree, selected, _ := tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	if selected != "" {
		t.Fatal("moving onto a directory unexpectedly selected it")
	}
	tree, selected, _ = tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selected != "" {
		t.Fatal("file filter allowed a directory to be selected")
	}
	tree, selected, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	if selected != "" {
		t.Fatal("moving onto a file unexpectedly selected it")
	}
	_, selected, _ = tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selected != filepath.Join(root, "first.JPG") {
		t.Fatalf("selected path = %q, want matching JPG", selected)
	}
}

func TestAllFilesFilterLabel(t *testing.T) {
	tree := New(t.TempDir(), 40, 10, WithFilter(AllFiles()))
	if tree.FilterLabel() != "ALL FILES" {
		t.Fatalf("filter label = %q, want ALL FILES", tree.FilterLabel())
	}
}

func TestCreateRenameDeleteAndRefresh(t *testing.T) {
	root := t.TempDir()
	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())

	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if !tree.CapturesText() {
		t.Fatal("a did not open the new-folder input")
	}
	tree.editor.SetValue("created")
	tree, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tree, _, _ = tree.Update(cmd())
	created := filepath.Join(root, "created")
	if info, err := os.Stat(created); err != nil || !info.IsDir() {
		t.Fatalf("a did not create directory: %v", err)
	}

	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	tree.editor.SetValue("renamed")
	tree, _, cmd = tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tree, _, _ = tree.Update(cmd())
	renamed := filepath.Join(root, "renamed")
	if _, err := os.Stat(renamed); err != nil {
		t.Fatalf("r did not rename directory: %v", err)
	}

	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !tree.HasDialog() {
		t.Fatal("d did not open delete confirmation")
	}
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if _, err := os.Stat(renamed); err != nil {
		t.Fatal("n unexpectedly deleted directory")
	}
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	tree, _, cmd = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	tree, _, _ = tree.Update(cmd())
	if _, err := os.Stat(renamed); !os.IsNotExist(err) {
		t.Fatalf("confirmed d left directory in place: %v", err)
	}

	external := filepath.Join(root, "external")
	if err := os.Mkdir(external, 0o755); err != nil {
		t.Fatal(err)
	}
	tree, _, cmd = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	tree, _, _ = tree.Update(cmd())
	if len(tree.visible) != 2 || tree.visible[1].name != "external" {
		t.Fatalf("R did not refresh filesystem entries: %#v", tree.visible)
	}
}

func TestRootCannotBeRenamedOrDeleted(t *testing.T) {
	tree := New(t.TempDir(), 40, 10)
	tree, _, _ = tree.Update(tree.Init()())
	for _, key := range []rune{'r', 'd'} {
		tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		if tree.HasDialog() {
			t.Fatalf("%c opened an operation for the explorer root", key)
		}
	}
}

func TestOperationPromptKeepsTreeVisibleAndPreservesExpansion(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.MkdirAll(filepath.Join(parent, "existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	tree, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	tree, _, _ = tree.Update(cmd())

	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	view := tree.View()
	if !strings.Contains(view, "existing") || !strings.Contains(view, "NEW FOLDER") {
		t.Fatalf("operation input replaced the tree instead of appearing below it:\n%s", view)
	}
	if got := lipgloss.Height(view); got != 12 {
		t.Fatalf("operation view height = %d, want 12", got)
	}

	tree.editor.SetValue("created")
	tree, _, cmd = tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tree, _, _ = tree.Update(cmd())
	parentNode := tree.find(parent)
	if parentNode == nil || !parentNode.expanded {
		t.Fatal("creating a directory lost the parent's expanded state")
	}
	if tree.find(filepath.Join(parent, "existing")) == nil || tree.find(filepath.Join(parent, "created")) == nil {
		t.Fatal("creating a directory lost existing tree context")
	}
}

func TestPathInputCompletesOneDirectoryLevelAndRespectsFilter(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "photos")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"beach.jpg", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(parent, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tree := New(root, 60, 12, WithFilter(Extensions("jpg")))
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !tree.CapturesText() || !strings.Contains(tree.View(), "PATH") {
		t.Fatal("/ did not open path input")
	}
	if tree.editor.Value() != "" {
		t.Fatalf("path input started with %q, want empty", tree.editor.Value())
	}
	tree.editor.SetValue(filepath.Join(root, "ph"))
	tree.completions = nil
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := tree.editor.Value(); got != parent+string(filepath.Separator) {
		t.Fatalf("directory completion = %q, want %q", got, parent+string(filepath.Separator))
	}
	tree.editor.SetValue(filepath.Join(parent, "be"))
	tree.completions = nil
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := tree.editor.Value(); got != filepath.Join(parent, "beach.jpg") {
		t.Fatalf("file completion = %q, want beach.jpg", got)
	}
	// A single candidate is completed outright, so Enter goes to it.
	tree, selected, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selected != "" || cmd == nil {
		t.Fatalf("path input selected %q instead of returning to the tree", selected)
	}
	tree, _, _ = tree.Update(cmd())
	if tree.SelectedPath() != filepath.Join(parent, "beach.jpg") {
		t.Fatalf("focused path = %q, want beach.jpg", tree.SelectedPath())
	}
	_, selected, _ = tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selected != filepath.Join(parent, "beach.jpg") {
		t.Fatalf("tree selection = %q, want beach.jpg", selected)
	}
}

func TestEscapeFromPathInputPreservesTree(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.MkdirAll(filepath.Join(parent, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	tree, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	tree, _, _ = tree.Update(cmd())
	parentNode := tree.find(parent)
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if tree.action != actionNone || !parentNode.expanded || tree.find(filepath.Join(parent, "child")) == nil {
		t.Fatal("Esc from path input did not preserve the original tree")
	}
}

func TestPathCompletionFallsBackToFuzzyAndCyclesBothDirections(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"1-foo", "2-far", "unrelated"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	tree.editor.SetValue(filepath.Join(root, "f"))
	tree.completions = nil

	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	if len(tree.completions) != 2 {
		t.Fatalf("fuzzy completion returned %d candidates, want 2", len(tree.completions))
	}
	if got := tree.editor.Value(); got != filepath.Join(root, "f") {
		t.Fatalf("opening the grid changed the input to %q", got)
	}
	if view := tree.View(); !strings.Contains(view, "1-foo/") || !strings.Contains(view, "2-far/") || !strings.Contains(view, "2 matches") {
		t.Fatalf("completion grid does not show both candidates:\n%s", view)
	}
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	first := tree.editor.Value()
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	second := tree.editor.Value()
	if second == first {
		t.Fatal("second Tab did not advance to the next completion")
	}
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := tree.editor.Value(); got != first {
		t.Fatalf("Shift+Tab returned %q, want first completion %q", got, first)
	}
}

func TestPathPasteReplacesExistingValueAndTrimsTrailingSpace(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "pasted")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if tree.editor.Value() != "" {
		t.Fatalf("path input started with %q, want empty", tree.editor.Value())
	}

	pasted := target + "   \n"
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(pasted), Paste: true})
	if got := tree.editor.Value(); got != target {
		t.Fatalf("pasted value = %q, want replacement %q", got, target)
	}
	tree, selected, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selected != "" || cmd == nil {
		t.Fatalf("pasted path selected %q instead of navigating", selected)
	}
	tree, _, _ = tree.Update(cmd())
	_, selected, _ = tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selected != target {
		t.Fatalf("selected tree path = %q, want %q", selected, target)
	}
}

func TestEmptyPathTabOffersFilteredEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12, WithFilter(Extensions("jpg")))
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	if len(tree.completions) != 2 {
		t.Fatalf("empty completion returned %v, want directory and matching file", tree.completions)
	}
	for _, completion := range tree.completions {
		if strings.Contains(completion, "notes.txt") {
			t.Fatalf("completion ignored filter: %v", tree.completions)
		}
	}
}

func TestMouseClickExpandsAndDoubleClickSelects(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	tree, selected, cmd := tree.Update(tea.MouseMsg{X: 4, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if selected != "" || cmd == nil || tree.SelectedPath() != child {
		t.Fatal("single click did not focus and expand directory")
	}
	tree, _, _ = tree.Update(cmd())
	_, selected, _ = tree.Update(tea.MouseMsg{X: 4, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if selected != child {
		t.Fatalf("double click selected %q, want %q", selected, child)
	}
}

func TestSeparateMouseClicksToggleDirectory(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, cmd := tree.Update(tea.MouseMsg{X: 4, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	tree, _, _ = tree.Update(cmd())
	tree.lastClickAt = time.Now().Add(-time.Second)
	tree, selected, _ := tree.Update(tea.MouseMsg{X: 4, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if selected != "" || tree.find(child).expanded {
		t.Fatal("a separate second click did not collapse the directory")
	}
}

func TestFileRowsShowHumanReadableSize(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), make([]byte, 1536), 0o644); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12, WithFilter(Extensions("jpg")))
	tree, _, _ = tree.Update(tree.Init()())
	if view := tree.View(); !strings.Contains(view, "photo.jpg") || !strings.Contains(view, "1.5 KB") {
		t.Fatalf("file row does not show its size:\n%s", view)
	}
}

func TestMinusMovesExplorerRootUpAndFocusesPreviousRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "Downloads")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())

	tree, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	if cmd == nil {
		t.Fatal("- did not request the parent directory")
	}
	tree, _, _ = tree.Update(cmd())
	if tree.root.path != parent {
		t.Fatalf("root after - = %q, want %q", tree.root.path, parent)
	}
	if tree.SelectedPath() != root {
		t.Fatalf("selection after - = %q, want previous root %q", tree.SelectedPath(), root)
	}
}

func TestMinusAtFilesystemRootDoesNothing(t *testing.T) {
	root := string(filepath.Separator)
	tree := New(root, 40, 10)
	updated, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	if cmd != nil || updated.root.path != root {
		t.Fatal("- moved above the filesystem root")
	}
}

func TestEqualsMakesFocusedDirectoryTheExplorerRoot(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})
	tree, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	tree, _, _ = tree.Update(cmd())
	parentNode := tree.find(parent)

	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'='}})
	if tree.root.path != parent || tree.SelectedPath() != parent {
		t.Fatalf("root after = = %q at %q, want %q", tree.root.path, tree.SelectedPath(), parent)
	}
	if tree.root.parent != nil || tree.root.depth != 0 {
		t.Fatal("new explorer root retained its previous parent or depth")
	}
	if !parentNode.expanded || tree.find(child) == nil || tree.find(child).depth != 1 {
		t.Fatal("= did not preserve and rebase the existing subtree")
	}
}

func TestEqualsLoadsAndExpandsOneLevel(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12)
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyDown})

	tree, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'='}})
	if cmd == nil {
		t.Fatal("= did not load the new root's first level")
	}
	tree, _, _ = tree.Update(cmd())
	if tree.root.path != parent || !tree.root.expanded {
		t.Fatal("= did not make the focused directory an expanded root")
	}
	if len(tree.visible) != 2 || tree.visible[1].path != child || tree.visible[1].expanded {
		t.Fatalf("= did not expand exactly one level: %#v", tree.visible)
	}
}

func TestAllFilesSelectsFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12, WithFilter(AllFiles()))
	tree, _, _ = tree.Update(tree.Init()())

	// Enter chooses the focused directory rather than expanding it.
	tree.focusPath(filepath.Join(root, "folder"))
	if _, selected, _ := tree.Update(tea.KeyMsg{Type: tea.KeyEnter}); selected != filepath.Join(root, "folder") {
		t.Fatalf("enter on a directory selected %q", selected)
	}
	// o still expands it without choosing.
	if _, selected, _ := tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}); selected != "" {
		t.Fatalf("o selected %q", selected)
	}
	tree.focusPath(filepath.Join(root, "file.txt"))
	if _, selected, _ := tree.Update(tea.KeyMsg{Type: tea.KeyEnter}); selected != filepath.Join(root, "file.txt") {
		t.Fatalf("enter on a file selected %q", selected)
	}
}

func TestHiddenEntriesToggle(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".ssh", "visible"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	names := func(tree Model) string {
		var found []string
		for _, node := range tree.visible[1:] {
			found = append(found, node.name)
		}
		return strings.Join(found, ",")
	}
	tree := New(root, 60, 12, WithFilter(AllFiles()))
	tree, _, _ = tree.Update(tree.Init()())
	if got := names(tree); got != "visible" || tree.FilterLabel() != "ALL FILES" {
		t.Fatalf("default: %q %q", got, tree.FilterLabel())
	}
	tree, _, cmd := tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	tree, _, _ = tree.Update(cmd())
	if got := names(tree); got != ".ssh,visible" || tree.FilterLabel() != "ALL FILES · HIDDEN" || !tree.ShowsHidden() {
		t.Fatalf("after H: %q %q", got, tree.FilterLabel())
	}

	shown := New(root, 60, 12, WithFilter(Directories()), WithHidden(true))
	shown, _, _ = shown.Update(shown.Init()())
	if got := names(shown); got != ".ssh,visible" {
		t.Fatalf("WithHidden: %q", got)
	}
}

func TestPathToHiddenFileShowsHidden(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".secret")
	if err := os.WriteFile(hidden, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 12, WithFilter(AllFiles()))
	tree, _, _ = tree.Update(tree.Init()())
	tree.editor.SetValue(hidden)
	tree, _, err := tree.acceptPath()
	if err != nil || !tree.ShowsHidden() {
		t.Fatalf("accept %v, hidden %v", err, tree.ShowsHidden())
	}
}

func TestTabCompletesCommonPrefixThenOpensGrid(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"id_ed25519", "id_ed25519.pub", "known_hosts"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "sockets"), 0o755); err != nil {
		t.Fatal(err)
	}
	tree := New(root, 60, 16, WithFilter(AllFiles()))
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	tree.editor.SetValue(filepath.Join(root, "i"))
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := tree.editor.Value(); got != filepath.Join(root, "id_ed25519") || len(tree.completions) != 0 {
		t.Fatalf("common prefix: %q, grid %v", got, tree.completions)
	}
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	if len(tree.completions) != 2 {
		t.Fatalf("second Tab did not open the grid: %v", tree.completions)
	}

	// A trailing separator lists the whole directory, directories first.
	tree.closeCompletions()
	tree.editor.SetValue(root + string(filepath.Separator))
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	if len(tree.completions) != 4 || !strings.HasSuffix(tree.completions[0], "sockets"+string(filepath.Separator)) {
		t.Fatalf("listing: %v", tree.completions)
	}

	// Typing narrows the open grid.
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if len(tree.completions) != 1 || !strings.HasSuffix(tree.completions[0], "known_hosts") {
		t.Fatalf("typing did not narrow the grid: %v", tree.completions)
	}

	// Esc closes the grid and stays in path input.
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(tree.completions) != 0 || tree.action != actionPath {
		t.Fatalf("esc: grid %v, action %v", tree.completions, tree.action)
	}
}

func TestGridArrowsAndDescend(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(root, name, "inside-"+name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tree := New(root, 60, 16, WithFilter(AllFiles()))
	tree, _, _ = tree.Update(tree.Init()())
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	tree.editor.SetValue(root + string(filepath.Separator))
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRight})
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRight})
	if tree.completionIndex != 1 || !strings.Contains(tree.editor.Value(), "beta") {
		t.Fatalf("arrows: index %d, input %q", tree.completionIndex, tree.editor.Value())
	}
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if len(tree.completions) != 1 || !strings.Contains(tree.completions[0], "inside-beta") {
		t.Fatalf("/ did not descend: %v", tree.completions)
	}
	if strings.Contains(tree.editor.Value(), "//") {
		t.Fatalf("descending doubled the separator: %q", tree.editor.Value())
	}
	tree, _, _ = tree.Update(tea.KeyMsg{Type: tea.KeyTab})
	tree, selected, _ := tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selected != "" || len(tree.completions) != 0 || !strings.HasSuffix(tree.editor.Value(), "inside-beta"+string(filepath.Separator)) {
		t.Fatalf("enter accepted %q, grid %v, selected %q", tree.editor.Value(), tree.completions, selected)
	}
}
