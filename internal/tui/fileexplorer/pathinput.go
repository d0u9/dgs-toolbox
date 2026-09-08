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

func (t *Model) completePath(reverse bool) {
	if len(t.completions) == 0 {
		matches, err := t.pathCompletions(t.editor.Value())
		if err != nil {
			t.notice = err.Error()
			return
		}
		t.completions = matches
		if reverse {
			t.completionIndex = len(matches) - 1
		}
	} else if reverse {
		t.completionIndex = (t.completionIndex - 1 + len(t.completions)) % len(t.completions)
	} else {
		t.completionIndex = (t.completionIndex + 1) % len(t.completions)
	}
	if len(t.completions) == 0 {
		t.notice = "no completions"
		return
	}
	t.notice = ""
	t.editor.SetValue(t.completions[t.completionIndex])
	t.editor.CursorEnd()
}

func (t *Model) acceptCompletion() bool {
	if len(t.completions) == 0 {
		return false
	}
	t.editor.SetValue(t.completions[t.completionIndex])
	t.editor.CursorEnd()
	t.completions = nil
	t.completionIndex = 0
	t.notice = ""
	return true
}

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
	matches := make([]string, 0)
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
		matches = append(matches, completed+completionScoreSeparator+fmt.Sprint(score))
	}
	sort.Slice(matches, func(i, j int) bool {
		leftPath, leftScore := splitScoredCompletion(matches[i])
		rightPath, rightScore := splitScoredCompletion(matches[j])
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return strings.ToLower(leftPath) < strings.ToLower(rightPath)
	})
	for index := range matches {
		matches[index], _ = splitScoredCompletion(matches[index])
	}
	return matches, nil
}

func trimPathInput(value string) string {
	return strings.TrimRightFunc(value, unicode.IsSpace)
}

const completionScoreSeparator = "\x00"

func splitScoredCompletion(value string) (string, int) {
	parts := strings.SplitN(value, completionScoreSeparator, 2)
	if len(parts) != 2 {
		return value, 0
	}
	var score int
	_, _ = fmt.Sscan(parts[1], &score)
	return parts[0], score
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
