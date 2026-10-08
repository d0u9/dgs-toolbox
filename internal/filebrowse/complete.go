package filebrowse

import (
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Complete returns matching folder paths and their common prefix. Relative
// paths start at base; ~ names the current user's home. No shell is executed.
func Complete(path, base string) ([]string, string, error) {
	dir, prefix := filepath.Split(path)
	if path == "~" {
		dir, prefix = "~/", ""
	}
	dir, err := ResolvePath(dir, base)
	if err != nil {
		return nil, "", err
	}
	listing, err := List(dir, Options{Hidden: strings.HasPrefix(prefix, ".")})
	if err != nil {
		return nil, "", err
	}
	matches := []string{}
	for _, entry := range listing.Dirs {
		if strings.HasPrefix(entry.Name, prefix) {
			matches = append(matches, entry.Path+string(os.PathSeparator))
		}
	}
	common := ""
	if len(matches) > 0 {
		common = matches[0]
		for _, match := range matches[1:] {
			for !strings.HasPrefix(match, common) {
				_, n := utf8.DecodeLastRuneInString(common)
				common = common[:len(common)-n]
			}
		}
	}
	return matches, common, nil
}

// ResolvePath expands ~ and resolves a relative path against base.
func ResolvePath(path, base string) (string, error) {
	trailing := strings.HasSuffix(path, string(os.PathSeparator)) || path == ""
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = home + string(os.PathSeparator) + strings.TrimLeft(path[1:], `/\`)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	if trailing && !strings.HasSuffix(path, string(os.PathSeparator)) {
		path += string(os.PathSeparator)
	}
	return path, nil
}
