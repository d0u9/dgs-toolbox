package fileexplorer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

func (t Model) inputBase() string {
	node := t.visible[t.selected]
	if node.isDir {
		return node.path
	}
	return node.parent.path
}

func (t Model) resolveInput(value string) string {
	value = trimPathInput(value)
	value = expandInputHome(value)
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(t.inputBase(), value))
}

func expandInputHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

// completePath answers Tab and Shift+Tab. With no candidate grid open, a
// single candidate is completed outright and several are completed to their
// common prefix, as bash does; when that adds nothing, the grid opens without
// changing the input. With the grid open, Tab and Shift+Tab move through it.
func (t *Model) completePath(reverse bool) {
	if len(t.completions) > 0 {
		if reverse {
			t.moveCompletion(-1, true)
		} else {
			t.moveCompletion(1, true)
		}
		return
	}
	matches, err := t.pathCompletions(t.editor.Value())
	if err != nil {
		t.notice = err.Error()
		return
	}
	switch len(matches) {
	case 0:
		t.notice = "no completions"
		return
	case 1:
		t.setInput(matches[0])
		return
	}
	current := trimPathInput(t.editor.Value())
	if common := commonPrefix(matches); len(common) > len(current) && strings.HasPrefix(strings.ToLower(common), strings.ToLower(current)) {
		t.setInput(common)
		return
	}
	t.notice = ""
	t.completions = matches
	t.completionIndex = -1
}

// moveCompletion moves the highlight by delta candidates, wrapping when wrap is
// set, and shows the highlighted candidate in the input.
func (t *Model) moveCompletion(delta int, wrap bool) {
	count := len(t.completions)
	if count == 0 {
		return
	}
	next := t.completionIndex + delta
	switch {
	case t.completionIndex < 0 && delta > 0:
		next = 0
	case t.completionIndex < 0:
		next = count - 1
	case wrap:
		next = (next%count + count) % count
	case next < 0 || next >= count:
		return
	}
	t.completionIndex = next
	t.editor.SetValue(t.completions[next])
	t.editor.CursorEnd()
}

// refilterCompletions lists again for what is now typed, keeping the grid open
// while anything still matches.
func (t *Model) refilterCompletions() {
	matches, err := t.pathCompletions(t.editor.Value())
	if err != nil || len(matches) == 0 {
		t.closeCompletions()
		t.notice = "no completions"
		return
	}
	t.completions = matches
	t.completionIndex = -1
}

// descendCompletion enters the highlighted directory and lists its contents.
func (t *Model) descendCompletion() bool {
	if t.completionIndex < 0 || !strings.HasSuffix(t.completions[t.completionIndex], string(filepath.Separator)) {
		return false
	}
	t.editor.SetValue(t.completions[t.completionIndex])
	t.editor.CursorEnd()
	t.refilterCompletions()
	return true
}

func (t *Model) setInput(value string) {
	t.editor.SetValue(value)
	t.editor.CursorEnd()
	t.closeCompletions()
	t.notice = ""
}

func (t *Model) closeCompletions() {
	t.completions = nil
	t.completionIndex = 0
}

// acceptCompletion closes an open grid, keeping the highlighted candidate in
// the input. It reports whether there was a grid to close.
func (t *Model) acceptCompletion() bool {
	if len(t.completions) == 0 {
		return false
	}
	if t.completionIndex >= 0 {
		t.editor.SetValue(t.completions[t.completionIndex])
		t.editor.CursorEnd()
	}
	t.closeCompletions()
	t.notice = ""
	return true
}

func commonPrefix(values []string) string {
	prefix := values[0]
	for _, value := range values[1:] {
		for !strings.HasPrefix(value, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

// pathCompletions lists the entries of the directory the input names that
// match its last fragment: by prefix, ignoring case, and fuzzily only when
// nothing matches by prefix. Directories come first, each ending in a
// separator.
func (t Model) pathCompletions(value string) ([]string, error) {
	value = trimPathInput(value)
	expanded := expandInputHome(value)
	directory, prefix := filepath.Split(expanded)
	readPath := directory
	if readPath == "" {
		readPath = t.inputBase()
	} else if !filepath.IsAbs(readPath) {
		readPath = filepath.Join(t.inputBase(), readPath)
	}
	entries, err := os.ReadDir(filepath.Clean(readPath))
	if err != nil {
		return nil, err
	}
	type candidate struct {
		completed string
		name      string
		dir       bool
		score     int
		prefixed  bool
	}
	var candidates []candidate
	anyPrefixed := false
	for _, entry := range entries {
		score, matched := completionScore(prefix, entry.Name())
		if !matched {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		directoryEntry := isDirectory(readPath, entry)
		if !directoryEntry && !t.filter.includesFile(entry.Name()) {
			continue
		}
		completed := filepath.Join(directory, entry.Name())
		if directoryEntry {
			completed += string(filepath.Separator)
		}
		if strings.HasPrefix(value, "~/") {
			home, _ := os.UserHomeDir()
			completed = "~" + strings.TrimPrefix(completed, home)
		}
		prefixed := strings.HasPrefix(strings.ToLower(entry.Name()), strings.ToLower(prefix))
		anyPrefixed = anyPrefixed || prefixed
		candidates = append(candidates, candidate{completed: completed, name: entry.Name(), dir: directoryEntry, score: score, prefixed: prefixed})
	}
	if anyPrefixed {
		kept := candidates[:0]
		for _, c := range candidates {
			if c.prefixed {
				kept = append(kept, c)
			}
		}
		candidates = kept
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.dir != right.dir {
			return left.dir
		}
		if !anyPrefixed && left.score != right.score {
			return left.score > right.score
		}
		return strings.ToLower(left.name) < strings.ToLower(right.name)
	})
	matches := make([]string, len(candidates))
	for i, c := range candidates {
		matches[i] = c.completed
	}
	return matches, nil
}

func trimPathInput(value string) string {
	return strings.TrimRightFunc(value, unicode.IsSpace)
}

func completionScore(query, candidate string) (int, bool) {
	queryRunes := []rune(strings.ToLower(query))
	candidateRunes := []rune(strings.ToLower(candidate))
	if len(queryRunes) == 0 {
		return 0, true
	}
	score, queryIndex, previous := 0, 0, -2
	for index, character := range candidateRunes {
		if queryIndex >= len(queryRunes) || character != queryRunes[queryIndex] {
			continue
		}
		score += 10
		if index == previous+1 {
			score += 6
		}
		if index == 0 || strings.ContainsRune("_- .", candidateRunes[index-1]) {
			score += 8
		}
		previous = index
		queryIndex++
	}
	return score - len(candidateRunes), queryIndex == len(queryRunes)
}

func (t Model) acceptPath() (Model, string, error) {
	path := t.resolveInput(t.editor.Value())
	info, err := os.Stat(path)
	if err != nil {
		return t, "", err
	}
	if info.IsDir() {
		t.id = explorerID.Add(1)
		t.root = &directoryNode{path: path, name: displayPath(path), expanded: true, loading: true, isDir: true}
		t.selected = 0
		t.action = actionNone
		t.editor.Blur()
		t.refresh()
		return t, "", nil
	}
	if t.filter.kind == directoryFilter || !t.filter.includesFile(filepath.Base(path)) {
		return t, "", fmt.Errorf("path does not match %s", t.filter.Label())
	}
	// A hidden file would not appear in its parent, so reaching one by path
	// shows hidden entries.
	if strings.HasPrefix(filepath.Base(path), ".") {
		t.showHidden = true
	}
	parent := filepath.Dir(path)
	t.id = explorerID.Add(1)
	t.root = &directoryNode{path: parent, name: displayPath(parent), expanded: true, loading: true, isDir: true}
	t.selected = 0
	t.focusAfterLoad = path
	t.action = actionNone
	t.editor.Blur()
	t.refresh()
	return t, "", nil
}
